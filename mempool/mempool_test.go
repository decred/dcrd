// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v2"
	"github.com/decred/dcrd/blockchain/standalone"
	"github.com/decred/dcrd/blockchain/v2"
	"github.com/decred/dcrd/blockchain/v2/chaingen"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v2"
	"github.com/decred/dcrd/chaincfg/v2/chainec"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1"
	"github.com/decred/dcrd/dcrutil/v2"
	"github.com/decred/dcrd/mining/v2"
	"github.com/decred/dcrd/txscript/v2"
	"github.com/decred/dcrd/wire"
)

const (
	// singleInputTicketSize is the typical size of a normal P2PKH ticket
	// in bytes when the ticket has one input, rounded up.
	singleInputTicketSize int64 = 300
)

// fakeChain is used by the pool harness to provide generated test utxos and
// a current faked chain height to the pool callbacks.  This, in turn, allows
// transactions to be appear as though they are spending completely valid utxos.
type fakeChain struct {
	sync.RWMutex
	nextStakeDiff  int64
	utxos          *blockchain.UtxoViewpoint
	utxoTimes      map[wire.OutPoint]int64
	blocks         map[chainhash.Hash]*dcrutil.Block
	currentHash    chainhash.Hash
	currentHeight  int64
	medianTime     time.Time
	scriptFlags    txscript.ScriptFlags
	acceptSeqLocks bool
}

// NextStakeDifficulty returns the next stake difficulty associated with the
// fake chain instance.
func (s *fakeChain) NextStakeDifficulty() (int64, error) {
	s.RLock()
	nextStakeDiff := s.nextStakeDiff
	s.RUnlock()
	return nextStakeDiff, nil
}

// SetNextStakeDifficulty sets the next stake difficulty associated with the
// fake chain instance.
func (s *fakeChain) SetNextStakeDifficulty(nextStakeDiff int64) {
	s.Lock()
	s.nextStakeDiff = nextStakeDiff
	s.Unlock()
}

// FetchUtxoView loads utxo details about the input transactions referenced by
// the passed transaction from the point of view of the fake chain.
// It also attempts to fetch the utxo details for the transaction itself so the
// returned view can be examined for duplicate unspent transaction outputs.
//
// This function is safe for concurrent access however the returned view is NOT.
func (s *fakeChain) FetchUtxoView(tx *dcrutil.Tx, treeValid bool) (*blockchain.UtxoViewpoint, error) {
	s.RLock()
	defer s.RUnlock()

	// All entries are cloned to ensure modifications to the returned view
	// do not affect the fake chain's view.

	// Add an entry for the tx itself to the new view.
	viewpoint := blockchain.NewUtxoViewpoint()
	entry := s.utxos.LookupEntry(tx.Hash())
	viewpoint.Entries()[*tx.Hash()] = entry.Clone()

	// Add entries for all of the inputs to the tx to the new view.
	for _, txIn := range tx.MsgTx().TxIn {
		originHash := &txIn.PreviousOutPoint.Hash
		entry := s.utxos.LookupEntry(originHash)
		viewpoint.Entries()[*originHash] = entry.Clone()
	}

	return viewpoint, nil
}

// BlockByHash returns the block with the given hash from the fake chain
// instance.  Blocks can be added to the instance with the AddBlock function.
func (s *fakeChain) BlockByHash(hash *chainhash.Hash) (*dcrutil.Block, error) {
	s.RLock()
	block, ok := s.blocks[*hash]
	s.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unable to find block %v in fake chain",
			hash)
	}
	return block, nil
}

// AddBlock adds a block that will be available to the BlockByHash function to
// the fake chain instance.
func (s *fakeChain) AddBlock(block *dcrutil.Block) {
	s.Lock()
	s.blocks[*block.Hash()] = block
	s.Unlock()
}

// BestHash returns the current best hash associated with the fake chain
// instance.
func (s *fakeChain) BestHash() *chainhash.Hash {
	s.RLock()
	hash := &s.currentHash
	s.RUnlock()
	return hash
}

// SetHash sets the current best hash associated with the fake chain instance.
func (s *fakeChain) SetBestHash(hash *chainhash.Hash) {
	s.Lock()
	s.currentHash = *hash
	s.Unlock()
}

// BestHeight returns the current height associated with the fake chain
// instance.
func (s *fakeChain) BestHeight() int64 {
	s.RLock()
	height := s.currentHeight
	s.RUnlock()
	return height
}

// SetHeight sets the current height associated with the fake chain instance.
func (s *fakeChain) SetHeight(height int64) {
	s.Lock()
	s.currentHeight = height
	s.Unlock()
}

// PastMedianTime returns the current median time associated with the fake chain
// instance.
func (s *fakeChain) PastMedianTime() time.Time {
	s.RLock()
	medianTime := s.medianTime
	s.RUnlock()
	return medianTime
}

// SetPastMedianTime sets the current median time associated with the fake chain
// instance.
func (s *fakeChain) SetPastMedianTime(medianTime time.Time) {
	s.Lock()
	s.medianTime = medianTime
	s.Unlock()
}

// CalcSequenceLock returns the current sequence lock for the passed transaction
// associated with the fake chain instance.
func (s *fakeChain) CalcSequenceLock(tx *dcrutil.Tx, view *blockchain.UtxoViewpoint) (*blockchain.SequenceLock, error) {
	// A value of -1 for each lock type allows a transaction to be included in a
	// block at any given height or time.
	sequenceLock := &blockchain.SequenceLock{MinHeight: -1, MinTime: -1}

	// Sequence locks do not apply if the tx version is less than 2, or the tx
	// is a coinbase or stakebase, so return now with a sequence lock that
	// indicates the tx can possibly be included in a block at any given height
	// or time.
	msgTx := tx.MsgTx()
	enforce := msgTx.Version >= 2
	if !enforce || standalone.IsCoinBaseTx(msgTx) || stake.IsSSGen(msgTx) {
		return sequenceLock, nil
	}

	for txInIndex, txIn := range msgTx.TxIn {
		// Nothing to calculate for this input when relative time locks are
		// disabled for it.
		sequenceNum := txIn.Sequence
		if sequenceNum&wire.SequenceLockTimeDisabled != 0 {
			continue
		}

		utxo := view.LookupEntry(&txIn.PreviousOutPoint.Hash)
		if utxo == nil {
			str := fmt.Sprintf("output %v referenced from transaction %s:%d "+
				"either does not exist or has already been spent",
				txIn.PreviousOutPoint, tx.Hash(), txInIndex)
			return nil, blockchain.RuleError{
				ErrorCode:   blockchain.ErrMissingTxOut,
				Description: str,
			}
		}

		// Calculate the sequence locks from the point of view of the next block
		// for inputs that are in the mempool.
		inputHeight := utxo.BlockHeight()
		if inputHeight == mining.UnminedHeight {
			inputHeight = s.BestHeight() + 1
		}

		// Mask off the value portion of the sequence number to obtain
		// the time lock delta required before this input can be spent.
		// The relative lock can be time based or block based.
		relativeLock := int64(sequenceNum & wire.SequenceLockTimeMask)

		if sequenceNum&wire.SequenceLockTimeIsSeconds != 0 {
			// Ordinarily time based relative locks determine the median time
			// for the block before the one the input was mined into, however,
			// in order to facilitate testing the fake chain instance instead
			// allows callers to directly set median times associated with fake
			// utxos and looks up those values here.
			medianTime := s.FakeUtxoMedianTime(&txIn.PreviousOutPoint)

			// Calculate the minimum required timestamp based on the sum of the
			// past median time and required relative number of seconds.  Since
			// time based relative locks have a granularity associated with
			// them, shift left accordingly in order to convert to the proper
			// number of relative seconds.  Also, subtract one from the relative
			// lock to maintain the original lock time semantics.
			relativeSecs := relativeLock << wire.SequenceLockTimeGranularity
			minTime := medianTime + relativeSecs - 1
			if minTime > sequenceLock.MinTime {
				sequenceLock.MinTime = minTime
			}
		} else {
			// This input requires a relative lock expressed in blocks before it
			// can be spent.  Therefore, calculate the minimum required height
			// based on the sum of the input height and required relative number
			// of blocks.  Also, subtract one from the relative lock in order to
			// maintain the original lock time semantics.
			minHeight := inputHeight + relativeLock - 1
			if minHeight > sequenceLock.MinHeight {
				sequenceLock.MinHeight = minHeight
			}
		}
	}

	return sequenceLock, nil
}

// StandardVerifyFlags returns the standard verification script flags associated
// with the fake chain instance.
func (s *fakeChain) StandardVerifyFlags() (txscript.ScriptFlags, error) {
	return s.scriptFlags, nil
}

// SetStandardVerifyFlags sets the standard verification script flags associated
// with the fake chain instance.
func (s *fakeChain) SetStandardVerifyFlags(flags txscript.ScriptFlags) {
	s.scriptFlags = flags
}

// FakeUtxoMedianTime returns the median time associated with the requested utxo
// from the fake chain instance.
func (s *fakeChain) FakeUtxoMedianTime(prevOut *wire.OutPoint) int64 {
	s.RLock()
	medianTime := s.utxoTimes[*prevOut]
	s.RUnlock()
	return medianTime
}

// AddFakeUtxoMedianTime adds a median time to the fake chain instance that will
// be used when querying the median time for the provided transaction and output
// when calculating by-time sequence locks.
func (s *fakeChain) AddFakeUtxoMedianTime(tx *dcrutil.Tx, txOutIdx uint32, medianTime time.Time) {
	s.Lock()
	s.utxoTimes[wire.OutPoint{
		Hash:  *tx.Hash(),
		Index: txOutIdx,
		Tree:  wire.TxTreeRegular,
	}] = medianTime.Unix()
	s.Unlock()
}

// AcceptSequenceLocks returns whether or not the pool harness the fake chain
// is associated with should accept transactions with sequence locks enabled.
func (s *fakeChain) AcceptSequenceLocks() (bool, error) {
	s.RLock()
	acceptSeqLocks := s.acceptSeqLocks
	s.RUnlock()
	return acceptSeqLocks, nil
}

// SetAcceptSequenceLocks sets whether or not the pool harness the fake chain is
// associated with should accept transactions with sequence locks enabled.
func (s *fakeChain) SetAcceptSequenceLocks(accept bool) {
	s.Lock()
	s.acceptSeqLocks = accept
	s.Unlock()
}

// spendableOutput is a convenience type that houses a particular utxo and the
// amount associated with it.
type spendableOutput struct {
	outPoint wire.OutPoint
	amount   dcrutil.Amount
}

// txOutToSpendableOut returns a spendable output given a transaction and index
// of the output to use.  This is useful as a convenience when creating test
// transactions.
func txOutToSpendableOut(tx *dcrutil.Tx, outputNum uint32, tree int8) spendableOutput {
	return spendableOutput{
		outPoint: wire.OutPoint{Hash: *tx.Hash(), Index: outputNum, Tree: tree},
		amount:   dcrutil.Amount(tx.MsgTx().TxOut[outputNum].Value),
	}
}

// poolHarness provides a harness that includes functionality for creating and
// signing transactions as well as a fake chain that provides utxos for use in
// generating valid transactions.
type poolHarness struct {
	// signKey is the signing key used for creating transactions throughout
	// the tests.
	//
	// payAddr is the p2sh address for the signing key and is used for the
	// payment address throughout the tests.
	signKey     *secp256k1.PrivateKey
	payAddr     dcrutil.Address
	payScript   []byte
	chainParams *chaincfg.Params

	chain  *fakeChain
	txPool *TxPool
}

// GetScript is the pool harness' implementation of the ScriptDB interface.
// It returns the pool harness' payment redeem script for any address
// passed in.
func (p *poolHarness) GetScript(addr dcrutil.Address) ([]byte, error) {
	return p.payScript, nil
}

// GetKey is the pool harness' implementation of the KeyDB interface.
// It returns the pool harness' signature key for any address passed in.
func (p *poolHarness) GetKey(addr dcrutil.Address) (chainec.PrivateKey, bool, error) {
	return p.signKey, true, nil
}

// AddFakeUTXO creates a fake mined utxo for the provided transaction.
func (p *poolHarness) AddFakeUTXO(tx *dcrutil.Tx, blockHeight int64) {
	p.chain.utxos.AddTxOuts(tx, blockHeight, wire.NullBlockIndex)
}

// CreateCoinbaseTx returns a coinbase transaction with the requested number of
// outputs paying an appropriate subsidy based on the passed block height to the
// address associated with the harness.  It automatically uses a standard
// signature script that starts with the block height that is required by
// version 2 blocks.
func (p *poolHarness) CreateCoinbaseTx(blockHeight int64, numOutputs uint32) (*dcrutil.Tx, error) {
	// Create standard coinbase script.
	extraNonce := int64(0)
	coinbaseScript, err := txscript.NewScriptBuilder().
		AddInt64(blockHeight).AddInt64(extraNonce).Script()
	if err != nil {
		return nil, err
	}

	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		// Coinbase transactions have no inputs, so previous outpoint is
		// zero hash and max index.
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
		SignatureScript: coinbaseScript,
		Sequence:        wire.MaxTxInSequenceNum,
	})
	totalInput := p.txPool.cfg.SubsidyCache.CalcBlockSubsidy(blockHeight)
	amountPerOutput := totalInput / int64(numOutputs)
	remainder := totalInput - amountPerOutput*int64(numOutputs)
	for i := uint32(0); i < numOutputs; i++ {
		// Ensure the final output accounts for any remainder that might
		// be left from splitting the input amount.
		amount := amountPerOutput
		if i == numOutputs-1 {
			amount = amountPerOutput + remainder
		}
		tx.AddTxOut(&wire.TxOut{
			PkScript: p.payScript,
			Value:    amount,
		})
	}

	return dcrutil.NewTx(tx), nil
}

// CreateSignedTx creates a new signed transaction that consumes the provided
// inputs and generates the provided number of outputs by evenly splitting the
// total input amount.  All outputs will be to the payment script associated
// with the harness and all inputs are assumed to do the same.
//
// Additionally, if one or more munge functions are specified, they will be
// invoked with the transaction prior to signing it.  This provides callers with
// the opportunity to modify the transaction which is especially useful for
// testing.
func (p *poolHarness) CreateSignedTx(inputs []spendableOutput, numOutputs uint32, mungers ...func(*wire.MsgTx)) (*dcrutil.Tx, error) {
	// Calculate the total input amount and split it amongst the requested
	// number of outputs.
	var totalInput dcrutil.Amount
	for _, input := range inputs {
		totalInput += input.amount
	}
	amountPerOutput := int64(totalInput) / int64(numOutputs)
	remainder := int64(totalInput) - amountPerOutput*int64(numOutputs)

	tx := wire.NewMsgTx()
	tx.Expiry = wire.NoExpiryValue
	for _, input := range inputs {
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: input.outPoint,
			SignatureScript:  nil,
			Sequence:         wire.MaxTxInSequenceNum,
			ValueIn:          int64(input.amount),
		})
	}
	for i := uint32(0); i < numOutputs; i++ {
		// Ensure the final output accounts for any remainder that might
		// be left from splitting the input amount.
		amount := amountPerOutput
		if i == numOutputs-1 {
			amount = amountPerOutput + remainder
		}
		tx.AddTxOut(&wire.TxOut{
			PkScript: p.payScript,
			Value:    amount,
		})
	}

	// Perform any transaction munging just before signing.
	for _, f := range mungers {
		f(tx)
	}

	// Sign the new transaction.
	for i := range tx.TxIn {
		sigScript, err := txscript.SignatureScript(tx, i, p.payScript,
			txscript.SigHashAll, p.signKey, true)
		if err != nil {
			return nil, err
		}
		tx.TxIn[i].SignatureScript = sigScript
	}

	return dcrutil.NewTx(tx), nil
}

// CreateTxChain creates a chain of zero-fee transactions (each subsequent
// transaction spends the entire amount from the previous one) with the first
// one spending the provided outpoint.  Each transaction spends the entire
// amount of the previous one and as such does not include any fees.
func (p *poolHarness) CreateTxChain(firstOutput spendableOutput, numTxns uint32) ([]*dcrutil.Tx, error) {
	txChain := make([]*dcrutil.Tx, 0, numTxns)
	prevOutPoint := firstOutput.outPoint
	spendableAmount := firstOutput.amount
	for i := uint32(0); i < numTxns; i++ {
		// Create the transaction using the previous transaction output
		// and paying the full amount to the payment address associated
		// with the harness.
		tx := wire.NewMsgTx()
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: prevOutPoint,
			SignatureScript:  nil,
			Sequence:         wire.MaxTxInSequenceNum,
			ValueIn:          int64(spendableAmount),
		})
		tx.AddTxOut(&wire.TxOut{
			PkScript: p.payScript,
			Value:    int64(spendableAmount),
		})

		// Sign the new transaction.
		sigScript, err := txscript.SignatureScript(tx, 0, p.payScript,
			txscript.SigHashAll, p.signKey, true)
		if err != nil {
			return nil, err
		}
		tx.TxIn[0].SignatureScript = sigScript

		txChain = append(txChain, dcrutil.NewTx(tx))

		// Next transaction uses outputs from this one.
		prevOutPoint = wire.OutPoint{Hash: tx.TxHash(), Index: 0}
	}

	return txChain, nil
}

// CreateTx creates a zero-fee regular transaction from the provided spendable
// output.
func (p *poolHarness) CreateTx(out spendableOutput) (*dcrutil.Tx, error) {
	txns, err := p.CreateTxChain(out, 1)
	if err != nil {
		return nil, err
	}
	return txns[0], err
}

// CreateTicketPurchase creates a ticket purchase spending the first output of
// the provided transaction.
func (p *poolHarness) CreateTicketPurchase(sourceTx *dcrutil.Tx, cost int64) (*dcrutil.Tx, error) {
	ticketfee := dcrutil.Amount(singleInputTicketSize)
	ticketPrice := dcrutil.Amount(cost)

	// Generate the p2sh, commitment and change scripts of the ticket.
	pkScript, err := txscript.PayToSStx(p.payAddr)
	if err != nil {
		return nil, err
	}
	commitScript := chaingen.PurchaseCommitmentScript(p.payAddr,
		ticketPrice+ticketfee, 0, ticketPrice)
	change := dcrutil.Amount(sourceTx.MsgTx().TxOut[0].Value) -
		ticketPrice - ticketfee
	changeScript, err := txscript.PayToSStxChange(p.payAddr)
	if err != nil {
		return nil, err
	}

	// Generate the ticket purchase.
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  *sourceTx.Hash(),
			Index: 0,
			Tree:  wire.TxTreeRegular,
		},
		Sequence:    wire.MaxTxInSequenceNum,
		ValueIn:     sourceTx.MsgTx().TxOut[0].Value,
		BlockHeight: uint32(p.chain.BestHeight()),
	})

	tx.AddTxOut(wire.NewTxOut(int64(ticketPrice), pkScript))
	tx.AddTxOut(wire.NewTxOut(0, commitScript))
	tx.AddTxOut(wire.NewTxOut(int64(change), changeScript))

	// Sign the ticket purchase.
	sigScript, err := txscript.SignatureScript(tx, 0,
		sourceTx.MsgTx().TxOut[0].PkScript, txscript.SigHashAll, p.signKey, true)
	if err != nil {
		return nil, err
	}
	tx.TxIn[0].SignatureScript = sigScript

	return dcrutil.NewTx(tx), nil
}

// newVoteScript generates a voting script from the passed VoteBits, for
// use in a vote.
func newVoteScript(voteBits stake.VoteBits) ([]byte, error) {
	b := make([]byte, 2+len(voteBits.ExtendedBits))
	binary.LittleEndian.PutUint16(b[0:2], voteBits.Bits)
	copy(b[2:], voteBits.ExtendedBits)
	return txscript.GenerateProvablyPruneableOut(b)
}

// CreateVote creates a vote transaction using the provided ticket.  The vote
// will vote on the current best block hash and height associated with the
// harness.
//
// Additionally, if one or more munge functions are specified, they will be
// invoked with the transaction prior to signing it.  This provides callers with
// the opportunity to modify the transaction which is especially useful for
// testing.
func (p *poolHarness) CreateVote(ticket *dcrutil.Tx, mungers ...func(*wire.MsgTx)) (*dcrutil.Tx, error) {
	// Calculate the vote subsidy.
	subsidyCache := p.txPool.cfg.SubsidyCache
	subsidy := subsidyCache.CalcStakeVoteSubsidy(p.chain.BestHeight())
	// Parse the ticket purchase transaction and generate the vote reward.
	ticketPayKinds, ticketHash160s, ticketValues, _, _, _ :=
		stake.TxSStxStakeOutputInfo(ticket.MsgTx())
	voteRewardValues := stake.CalculateRewards(ticketValues,
		ticket.MsgTx().TxOut[0].Value, subsidy)

	// Add the stakebase input.
	vote := wire.NewMsgTx()
	stakebaseOutPoint := wire.NewOutPoint(&chainhash.Hash{}, ^uint32(0),
		wire.TxTreeRegular)
	stakebaseInput := wire.NewTxIn(stakebaseOutPoint, subsidy, nil)
	vote.AddTxIn(stakebaseInput)

	// Add the ticket input.
	spendOut := txOutToSpendableOut(ticket, 0, wire.TxTreeStake)
	ticketInput := wire.NewTxIn(&spendOut.outPoint, int64(spendOut.amount), nil)
	ticketInput.BlockHeight = uint32(p.chain.BestHeight())
	ticketInput.BlockIndex = 5
	vote.AddTxIn(ticketInput)

	// Add the block reference output.
	blockRefScript, _ := txscript.GenerateSSGenBlockRef(*p.chain.BestHash(),
		uint32(p.chain.BestHeight()))
	vote.AddTxOut(wire.NewTxOut(0, blockRefScript))

	// Create the vote script.
	voteBits := stake.VoteBits{Bits: uint16(0xff), ExtendedBits: []byte{}}
	voteScript, err := newVoteScript(voteBits)
	if err != nil {
		return nil, err
	}
	vote.AddTxOut(wire.NewTxOut(0, voteScript))

	// Create P2SH scripts for the ticket outputs.
	for i, hash160 := range ticketHash160s {
		scriptFn := txscript.PayToSSGenPKHDirect
		if ticketPayKinds[i] { // P2SH
			scriptFn = txscript.PayToSSGenSHDirect
		}
		// Error is checking for a nil hash160, just ignore it.
		script, _ := scriptFn(hash160)
		vote.AddTxOut(wire.NewTxOut(voteRewardValues[i], script))
	}

	// Perform any transaction munging just before signing.
	for _, f := range mungers {
		f(vote)
	}

	// Sign the input.
	inputToSign := 1
	redeemTicketScript := ticket.MsgTx().TxOut[0].PkScript
	signedScript, err := txscript.SignTxOutput(p.chainParams, vote, inputToSign,
		redeemTicketScript, txscript.SigHashAll, p,
		p, vote.TxIn[inputToSign].SignatureScript, dcrec.STEcdsaSecp256k1)
	if err != nil {
		return nil, err
	}

	vote.TxIn[0].SignatureScript = p.chainParams.StakeBaseSigScript
	vote.TxIn[1].SignatureScript = signedScript

	return dcrutil.NewTx(vote), nil
}

// CreateRevocation creates a revocation using the provided ticket.
func (p *poolHarness) CreateRevocation(ticket *dcrutil.Tx) (*dcrutil.Tx, error) {
	ticketPurchase := ticket.MsgTx()
	ticketHash := ticketPurchase.TxHash()

	// Parse the ticket purchase transaction and generate the revocation value.
	ticketPayKinds, ticketHash160s, ticketValues, _, _, _ :=
		stake.TxSStxStakeOutputInfo(ticketPurchase)
	revocationValues := stake.CalculateRewards(ticketValues,
		ticketPurchase.TxOut[0].Value, 0)

	// Add the ticket input.
	revocation := wire.NewMsgTx()
	ticketOutPoint := wire.NewOutPoint(&ticketHash, 0, wire.TxTreeStake)
	ticketInput := wire.NewTxIn(ticketOutPoint,
		ticketPurchase.TxOut[ticketOutPoint.Index].Value, nil)
	revocation.AddTxIn(ticketInput)

	// All remaining outputs pay to the output destinations and amounts tagged
	// by the ticket purchase.
	for i, hash160 := range ticketHash160s {
		scriptFn := txscript.PayToSSRtxPKHDirect
		if ticketPayKinds[i] { // P2SH
			scriptFn = txscript.PayToSSRtxSHDirect
		}
		// Error is checking for a nil hash160, just ignore it.
		script, _ := scriptFn(hash160)
		revocation.AddTxOut(wire.NewTxOut(revocationValues[i], script))
	}

	// Sign the input.
	inputToSign := 0
	redeemTicketScript := ticket.MsgTx().TxOut[0].PkScript
	signedScript, err := txscript.SignTxOutput(p.chainParams, revocation, inputToSign,
		redeemTicketScript, txscript.SigHashAll, p,
		p, revocation.TxIn[inputToSign].SignatureScript, dcrec.STEcdsaSecp256k1)
	if err != nil {
		return nil, err
	}

	revocation.TxIn[0].SignatureScript = signedScript

	return dcrutil.NewTx(revocation), nil
}

// newPoolHarness returns a new instance of a pool harness initialized with a
// fake chain and a TxPool bound to it that is configured with a policy suitable
// for testing.  Also, the fake chain is populated with the returned spendable
// outputs so the caller can easily create new valid transactions which build
// off of it.
func newPoolHarness(chainParams *chaincfg.Params) (*poolHarness, []spendableOutput, error) {
	// Use a hard coded key pair for deterministic results.
	keyBytes, err := hex.DecodeString("700868df1838811ffbdf918fb482c1f7e" +
		"ad62db4b97bd7012c23e726485e577d")
	if err != nil {
		return nil, nil, err
	}
	signKey, signPub := secp256k1.PrivKeyFromBytes(keyBytes)

	// Generate associated pay-to-script-hash address and resulting payment
	// script.
	pubKeyBytes := signPub.SerializeCompressed()
	payPubKeyAddr, err := dcrutil.NewAddressSecpPubKey(pubKeyBytes,
		chainParams)
	if err != nil {
		return nil, nil, err
	}
	payAddr := payPubKeyAddr.AddressPubKeyHash()
	pkScript, err := txscript.PayToAddrScript(payAddr)
	if err != nil {
		return nil, nil, err
	}

	// Create a new fake chain and harness bound to it.
	subsidyCache := standalone.NewSubsidyCache(chainParams)
	chain := &fakeChain{
		utxos:       blockchain.NewUtxoViewpoint(),
		utxoTimes:   make(map[wire.OutPoint]int64),
		blocks:      make(map[chainhash.Hash]*dcrutil.Block),
		scriptFlags: BaseStandardVerifyFlags,
	}
	harness := poolHarness{
		signKey:     signKey,
		payAddr:     payAddr,
		payScript:   pkScript,
		chainParams: chainParams,

		chain: chain,
		txPool: New(&Config{
			Policy: Policy{
				MaxTxVersion:         2,
				DisableRelayPriority: true,
				FreeTxRelayLimit:     15.0,
				MaxOrphanTxs:         5,
				MaxOrphanTxSize:      1000,
				MaxSigOpsPerTx:       blockchain.MaxSigOpsPerBlock / 5,
				MinRelayTxFee:        1000, // 1 Satoshi per byte
				StandardVerifyFlags:  chain.StandardVerifyFlags,
				AcceptSequenceLocks:  chain.AcceptSequenceLocks,
			},
			ChainParams:         chainParams,
			NextStakeDifficulty: chain.NextStakeDifficulty,
			FetchUtxoView:       chain.FetchUtxoView,
			BlockByHash:         chain.BlockByHash,
			BestHash:            chain.BestHash,
			BestHeight:          chain.BestHeight,
			PastMedianTime:      chain.PastMedianTime,
			CalcSequenceLock:    chain.CalcSequenceLock,
			SubsidyCache:        subsidyCache,
			SigCache:            nil,
			AddrIndex:           nil,
			ExistsAddrIndex:     nil,
			OnVoteReceived:      nil,
		}),
	}

	// Create a single coinbase transaction and add it to the harness
	// chain's utxo set and set the harness chain height such that the
	// coinbase will mature in the next block.  This ensures the txpool
	// accepts transactions which spend immature coinbases that will become
	// mature in the next block.
	numOutputs := uint32(1)
	outputs := make([]spendableOutput, 0, numOutputs)
	curHeight := harness.chain.BestHeight()
	coinbase, err := harness.CreateCoinbaseTx(curHeight+1, numOutputs)
	if err != nil {
		return nil, nil, err
	}
	harness.chain.utxos.AddTxOuts(coinbase, curHeight+1, wire.NullBlockIndex)
	for i := uint32(0); i < numOutputs; i++ {
		outputs = append(outputs, txOutToSpendableOut(coinbase, i, wire.TxTreeRegular))
	}
	harness.chain.SetHeight(int64(chainParams.CoinbaseMaturity) + curHeight)
	harness.chain.SetPastMedianTime(time.Now())

	return &harness, outputs, nil
}

// testContext houses a test-related state that is useful to pass to helper
// functions as a single argument.
type testContext struct {
	t       *testing.T
	harness *poolHarness
}

// testPoolMembership tests the transaction pool associated with the provided
// test context to determine if the passed transaction matches the provided
// orphan pool and transaction pool status.  It also further determines if it
// should be reported as available by the HaveTransaction function based upon
// the two flags and tests that condition as well.
func testPoolMembership(tc *testContext, tx *dcrutil.Tx, inOrphanPool, inTxPool bool) {
	txHash := tx.Hash()
	gotOrphanPool := tc.harness.txPool.IsOrphanInPool(txHash)
	if inOrphanPool != gotOrphanPool {
		_, file, line, _ := runtime.Caller(1)
		tc.t.Fatalf("%s:%d -- IsOrphanInPool: want %v, got %v", file,
			line, inOrphanPool, gotOrphanPool)
	}

	gotTxPool := tc.harness.txPool.IsTransactionInPool(txHash)
	if inTxPool != gotTxPool {
		_, file, line, _ := runtime.Caller(1)
		tc.t.Fatalf("%s:%d -- IsTransactionInPool: want %v, got %v",
			file, line, inTxPool, gotTxPool)
	}

	gotHaveTx := tc.harness.txPool.HaveTransaction(txHash)
	wantHaveTx := inOrphanPool || inTxPool
	if wantHaveTx != gotHaveTx {
		_, file, line, _ := runtime.Caller(1)
		tc.t.Fatalf("%s:%d -- HaveTransaction: want %v, got %v", file,
			line, wantHaveTx, gotHaveTx)
	}
}

// TestSimpleOrphanChain ensures that a simple chain of orphans is handled
// properly.  In particular, it generates a chain of single input, single output
// transactions and inserts them while skipping the first linking transaction so
// they are all orphans.  Finally, it adds the linking transaction and ensures
// the entire orphan chain is moved to the transaction pool.
func TestSimpleOrphanChain(t *testing.T) {
	t.Parallel()

	harness, spendableOuts, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a chain of transactions rooted with the first spendable output
	// provided by the harness.
	maxOrphans := uint32(harness.txPool.cfg.Policy.MaxOrphanTxs)
	chainedTxns, err := harness.CreateTxChain(spendableOuts[0], maxOrphans+1)
	if err != nil {
		t.Fatalf("unable to create transaction chain: %v", err)
	}

	// Ensure the orphans are accepted (only up to the maximum allowed so
	// none are evicted).
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, true,
			false, true)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid "+
				"orphan %v", err)
		}

		// Ensure no transactions were reported as accepted.
		if len(acceptedTxns) != 0 {
			t.Fatalf("ProcessTransaction: reported %d accepted "+
				"transactions from what should be an orphan",
				len(acceptedTxns))
		}

		// Ensure the transaction is in the orphan pool, is not in the
		// transaction pool, and is reported as available.
		testPoolMembership(tc, tx, true, false)
	}

	// Add the transaction which completes the orphan chain and ensure they
	// all get accepted.  Notice the accept orphans flag is also false here
	// to ensure it has no bearing on whether or not already existing
	// orphans in the pool are linked.
	acceptedTxns, err := harness.txPool.ProcessTransaction(chainedTxns[0],
		false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid "+
			"orphan %v", err)
	}
	if len(acceptedTxns) != len(chainedTxns) {
		t.Fatalf("ProcessTransaction: reported accepted transactions "+
			"length does not match expected -- got %d, want %d",
			len(acceptedTxns), len(chainedTxns))
	}
	for _, tx := range acceptedTxns {
		// Ensure the transaction is no longer in the orphan pool, is
		// now in the transaction pool, and is reported as available.
		testPoolMembership(tc, tx, false, true)
	}
}

// TestTicketPurchaseOrphan ensures that ticket purchases are orphaned when
// referenced outputs spent are from missing transactions.
func TestTicketPurchaseOrphan(t *testing.T) {
	t.Parallel()

	harness, spendableOuts, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a regular transaction from the first spendable output
	// provided by the harness.
	tx, err := harness.CreateTx(spendableOuts[0])
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// Create a ticket purchase transaction spending the outputs of the
	// prior regular transaction.
	ticket, err := harness.CreateTicketPurchase(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction %v", err)
	}

	// Ensure the ticket purchase is accepted as an orphan.
	acceptedTxns, err := harness.txPool.ProcessTransaction(ticket, true,
		false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid orphan %v", err)
	}
	testPoolMembership(tc, ticket, true, false)

	if len(acceptedTxns) > 0 {
		t.Fatalf("ProcessTransaction: expected zero accepted transactions "+
			"got %v", len(acceptedTxns))
	}

	// Add the regular transaction whose outputs are spent by the ticket purchase
	// and ensure they all get accepted.  Notice the accept orphans flag is also
	// false here to ensure it has no bearing on whether or not already existing
	// orphans in the pool are linked.
	_, err = harness.txPool.ProcessTransaction(tx, false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)
	testPoolMembership(tc, ticket, false, true)
}

// TestVoteOrphan ensures that votes are orphaned when referenced outputs
// spent are from missing transactions.
func TestVoteOrphan(t *testing.T) {
	t.Parallel()

	harness, spendableOuts, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a regular transaction from the first spendable output
	// provided by the harness.
	tx, err := harness.CreateTx(spendableOuts[0])
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// Create a ticket purchase transaction spending the outputs of the
	// prior regular transaction.
	ticket, err := harness.CreateTicketPurchase(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction: %v", err)
	}

	harness.chain.SetHeight(harness.chainParams.StakeValidationHeight)

	vote, err := harness.CreateVote(ticket)
	if err != nil {
		t.Fatalf("unable to create vote: %v", err)
	}

	// Ensure the vote is rejected because it is an orphan.
	_, err = harness.txPool.ProcessTransaction(vote, false, false, true)
	if !IsErrorCode(err, ErrOrphan) {
		t.Fatalf("Process Transaction: did not get expected ErrOrphan")
	}
	testPoolMembership(tc, vote, false, false)

	// Ensure the ticket is accepted as an orphan.
	_, err = harness.txPool.ProcessTransaction(ticket, true, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, ticket, true, false)

	// Ensure the regular tx whose output is spent by the ticket is accepted.
	_, err = harness.txPool.ProcessTransaction(tx, false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)

	// Generate a fake mined utxo for the ticket created.
	harness.AddFakeUTXO(ticket, int64(ticket.MsgTx().TxIn[0].BlockHeight))

	// Ensure the previously rejected vote is accepted now since all referenced
	// utxos can now be found.
	_, err = harness.txPool.ProcessTransaction(vote, false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept orphan transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)
	testPoolMembership(tc, ticket, false, true)
	testPoolMembership(tc, vote, false, true)
}

// TestRevocationOrphan ensures that revocations are orphaned when
// referenced outputs spent are from missing transactions.
func TestRevocationOrphan(t *testing.T) {
	t.Parallel()

	harness, spendableOuts, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a regular transaction from the first spendable output
	// provided by the harness.
	tx, err := harness.CreateTx(spendableOuts[0])
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// Create a ticket purchase transaction spending the outputs of the
	// prior regular transaction.
	ticket, err := harness.CreateTicketPurchase(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction: %v", err)
	}

	harness.chain.SetHeight(harness.chainParams.StakeValidationHeight + 1)

	revocation, err := harness.CreateRevocation(ticket)
	if err != nil {
		t.Fatalf("unable to create revocation: %v", err)
	}

	// Ensure the vote is rejected because it is an orphan.
	_, err = harness.txPool.ProcessTransaction(revocation, false, false, true)
	if !IsErrorCode(err, ErrOrphan) {
		t.Fatalf("Process Transaction: did not get expected " +
			"ErrTooManyVotes error code")
	}
	testPoolMembership(tc, revocation, false, false)

	// Ensure the ticket is accepted as an orphan.
	_, err = harness.txPool.ProcessTransaction(ticket, true, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, ticket, true, false)

	// Ensure the regular tx whose output is spent by the ticket is accepted.
	_, err = harness.txPool.ProcessTransaction(tx, false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)

	// Generate a fake mined utxos for the ticket created.
	harness.AddFakeUTXO(ticket, int64(ticket.MsgTx().TxIn[0].BlockHeight))

	// Ensure the previously rejected revocation is accepted now since all referenced
	// utxos can now be found.
	_, err = harness.txPool.ProcessTransaction(revocation, false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept orphan transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)
	testPoolMembership(tc, ticket, false, true)
	testPoolMembership(tc, revocation, false, true)
}

// TestOrphanReject ensures that orphans are properly rejected when the allow
// orphans flag is not set on ProcessTransaction.
func TestOrphanReject(t *testing.T) {
	t.Parallel()

	harness, outputs, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a chain of transactions rooted with the first spendable output
	// provided by the harness.
	maxOrphans := uint32(harness.txPool.cfg.Policy.MaxOrphanTxs)
	chainedTxns, err := harness.CreateTxChain(outputs[0], maxOrphans+1)
	if err != nil {
		t.Fatalf("unable to create transaction chain: %v", err)
	}

	// Ensure orphans are rejected when the allow orphans flag is not set.
	for _, tx := range chainedTxns[1:] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, false,
			false, true)
		if err == nil {
			t.Fatalf("ProcessTransaction: did not fail on orphan "+
				"%v when allow orphans flag is false", tx.Hash())
		}
		expectedErr := RuleError{}
		if reflect.TypeOf(err) != reflect.TypeOf(expectedErr) {
			t.Fatalf("ProcessTransaction: wrong error got: <%T> %v, "+
				"want: <%T>", err, err, expectedErr)
		}
		code, extracted := extractRejectCode(err)
		if !extracted {
			t.Fatalf("ProcessTransaction: failed to extract reject "+
				"code from error %q", err)
		}
		if code != wire.RejectDuplicate {
			t.Fatalf("ProcessTransaction: unexpected reject code "+
				"-- got %v, want %v", code, wire.RejectDuplicate)
		}

		if !IsErrorCode(err, ErrOrphan) {
			t.Fatalf("ProcessTransaction: unexpected error code "+
				"-- got %v, want %v", code, ErrOrphan)
		}

		// Ensure no transactions were reported as accepted.
		if len(acceptedTxns) != 0 {
			t.Fatal("ProcessTransaction: reported %d accepted "+
				"transactions from failed orphan attempt",
				len(acceptedTxns))
		}

		// Ensure the transaction is not in the orphan pool, not in the
		// transaction pool, and not reported as available
		testPoolMembership(tc, tx, false, false)
	}
}

// TestOrphanEviction ensures that exceeding the maximum number of orphans
// evicts entries to make room for the new ones.
func TestOrphanEviction(t *testing.T) {
	t.Parallel()

	harness, outputs, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a chain of transactions rooted with the first spendable output
	// provided by the harness that is long enough to be able to force
	// several orphan evictions.
	maxOrphans := uint32(harness.txPool.cfg.Policy.MaxOrphanTxs)
	chainedTxns, err := harness.CreateTxChain(outputs[0], maxOrphans+5)
	if err != nil {
		t.Fatalf("unable to create transaction chain: %v", err)
	}

	// Add enough orphans to exceed the max allowed while ensuring they are
	// all accepted.  This will cause an eviction.
	for _, tx := range chainedTxns[1:] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, true,
			false, true)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid "+
				"orphan %v", err)
		}

		// Ensure no transactions were reported as accepted.
		if len(acceptedTxns) != 0 {
			t.Fatalf("ProcessTransaction: reported %d accepted "+
				"transactions from what should be an orphan",
				len(acceptedTxns))
		}

		// Ensure the transaction is in the orphan pool, is not in the
		// transaction pool, and is reported as available.
		testPoolMembership(tc, tx, true, false)
	}

	// Figure out which transactions were evicted and make sure the number
	// evicted matches the expected number.
	var evictedTxns []*dcrutil.Tx
	for _, tx := range chainedTxns[1:] {
		if !harness.txPool.IsOrphanInPool(tx.Hash()) {
			evictedTxns = append(evictedTxns, tx)
		}
	}
	expectedEvictions := len(chainedTxns) - 1 - int(maxOrphans)
	if len(evictedTxns) != expectedEvictions {
		t.Fatalf("unexpected number of evictions -- got %d, want %d",
			len(evictedTxns), expectedEvictions)
	}

	// Ensure none of the evicted transactions ended up in the transaction
	// pool.
	for _, tx := range evictedTxns {
		testPoolMembership(tc, tx, false, false)
	}
}

// TestExpirationPruning ensures that transactions that expire without being
// mined are removed.
func TestExpirationPruning(t *testing.T) {
	harness, outputs, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create and add a transaction with several outputs that spends the first
	// spendable output provided by the harness and ensure it is not the orphan
	// pool, is in the transaction pool, and is reported as available.
	//
	// These outputs will be used as inputs to transactions with expirations.
	const numTxns = 5
	multiOutputTx, err := harness.CreateSignedTx([]spendableOutput{outputs[0]},
		numTxns)
	if err != nil {
		t.Fatalf("unable to create signed tx: %v", err)
	}
	acceptedTxns, err := harness.txPool.ProcessTransaction(multiOutputTx,
		true, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid tx: %v", err)
	}
	if len(acceptedTxns) != 1 {
		t.Fatalf("ProcessTransaction: reported %d accepted transactions from "+
			"what should be 1", len(acceptedTxns))
	}
	testPoolMembership(tc, multiOutputTx, false, true)

	// Create several transactions such that each transaction has an expiration
	// one block after the previous and the first one expires in the block after
	// the next one.
	nextBlockHeight := harness.chain.BestHeight() + 1
	expiringTxns := make([]*dcrutil.Tx, 0, numTxns)
	for i := 0; i < numTxns; i++ {
		tx, err := harness.CreateSignedTx([]spendableOutput{
			txOutToSpendableOut(multiOutputTx, uint32(i), wire.TxTreeRegular),
		}, 1, func(tx *wire.MsgTx) {
			tx.Expiry = uint32(nextBlockHeight + int64(i) + 1)
		})
		if err != nil {
			t.Fatalf("unable to create signed tx: %v", err)
		}
		expiringTxns = append(expiringTxns, tx)
	}

	// Ensure expiration pruning is working properly by adding each expiring
	// transaction just before the point at which it will expire and advancing
	// the chain so that the transaction becomes expired and thus should be
	// pruned.
	for _, tx := range expiringTxns {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, true, false,
			true)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid tx: %v", err)
		}

		// Ensure the transaction was reported as accepted, is not in the orphan
		// pool, is in the transaction pool, and is reported as available.
		if len(acceptedTxns) != 1 {
			t.Fatalf("ProcessTransaction: reported %d accepted transactions "+
				"from what should be 1", len(acceptedTxns))
		}
		testPoolMembership(tc, tx, false, true)

		// Simulate processing a new block that did not mine any of the txns.
		harness.chain.SetHeight(harness.chain.BestHeight() + 1)

		// Prune any transactions that are now expired and ensure that the tx
		// that was just added was pruned by checking that it is not in the
		// orphan pool, not in the transaction pool, and not reported as
		// available.
		harness.txPool.PruneExpiredTx()
		testPoolMembership(tc, tx, false, false)
	}
}

// TestBasicOrphanRemoval ensure that orphan removal works as expected when an
// orphan that doesn't exist is removed both when there is another orphan that
// redeems it and when there is not.
func TestBasicOrphanRemoval(t *testing.T) {
	t.Parallel()

	const maxOrphans = 4
	harness, spendableOuts, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	harness.txPool.cfg.Policy.MaxOrphanTxs = maxOrphans
	tc := &testContext{t, harness}

	// Create a chain of transactions rooted with the first spendable output
	// provided by the harness.
	chainedTxns, err := harness.CreateTxChain(spendableOuts[0], maxOrphans+1)
	if err != nil {
		t.Fatalf("unable to create transaction chain: %v", err)
	}

	// Ensure the orphans are accepted (only up to the maximum allowed so
	// none are evicted).
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, true,
			false, true)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid "+
				"orphan %v", err)
		}

		// Ensure no transactions were reported as accepted.
		if len(acceptedTxns) != 0 {
			t.Fatalf("ProcessTransaction: reported %d accepted "+
				"transactions from what should be an orphan",
				len(acceptedTxns))
		}

		// Ensure the transaction is in the orphan pool, not in the
		// transaction pool, and reported as available.
		testPoolMembership(tc, tx, true, false)
	}

	// Attempt to remove an orphan that has no redeemers and is not present,
	// and ensure the state of all other orphans are unaffected.
	nonChainedOrphanTx, err := harness.CreateSignedTx([]spendableOutput{{
		amount:   dcrutil.Amount(5000000000),
		outPoint: wire.OutPoint{Hash: chainhash.Hash{}, Index: 0},
	}}, 1, func(tx *wire.MsgTx) {
		tx.Expiry = uint32(harness.chain.BestHeight() + 1)
	})
	if err != nil {
		t.Fatalf("unable to create signed tx: %v", err)
	}

	harness.txPool.RemoveOrphan(nonChainedOrphanTx)
	testPoolMembership(tc, nonChainedOrphanTx, false, false)
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		testPoolMembership(tc, tx, true, false)
	}

	// Attempt to remove an orphan that has an existing redeemer but itself
	// is not present and ensure the state of all other orphans (including
	// the one that redeems it) are unaffected.
	harness.txPool.RemoveOrphan(chainedTxns[0])
	testPoolMembership(tc, chainedTxns[0], false, false)
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		testPoolMembership(tc, tx, true, false)
	}

	// Remove each orphan one-by-one and ensure they are removed as
	// expected.
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		harness.txPool.RemoveOrphan(tx)
		testPoolMembership(tc, tx, false, false)
	}
}

// TestOrphanChainRemoval ensure that orphan chains (orphans that spend outputs
// from other orphans) are removed as expected.
func TestOrphanChainRemoval(t *testing.T) {
	t.Parallel()

	const maxOrphans = 10
	harness, spendableOuts, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	harness.txPool.cfg.Policy.MaxOrphanTxs = maxOrphans
	tc := &testContext{t, harness}

	// Create a chain of transactions rooted with the first spendable output
	// provided by the harness.
	chainedTxns, err := harness.CreateTxChain(spendableOuts[0], maxOrphans+1)
	if err != nil {
		t.Fatalf("unable to create transaction chain: %v", err)
	}

	// Ensure the orphans are accepted (only up to the maximum allowed so
	// none are evicted).
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, true,
			false, true)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid "+
				"orphan %v", err)
		}

		// Ensure no transactions were reported as accepted.
		if len(acceptedTxns) != 0 {
			t.Fatalf("ProcessTransaction: reported %d accepted "+
				"transactions from what should be an orphan",
				len(acceptedTxns))
		}

		// Ensure the transaction is in the orphan pool, not in the
		// transaction pool, and reported as available.
		testPoolMembership(tc, tx, true, false)
	}

	// Remove the first orphan that starts the orphan chain without the
	// remove redeemer flag set and ensure that only the first orphan was
	// removed.
	harness.txPool.mtx.Lock()
	harness.txPool.removeOrphan(chainedTxns[1], false)
	harness.txPool.mtx.Unlock()
	testPoolMembership(tc, chainedTxns[1], false, false)
	for _, tx := range chainedTxns[2 : maxOrphans+1] {
		testPoolMembership(tc, tx, true, false)
	}

	// Remove the first remaining orphan that starts the orphan chain with
	// the remove redeemer flag set and ensure they are all removed.
	harness.txPool.mtx.Lock()
	harness.txPool.removeOrphan(chainedTxns[2], true)
	harness.txPool.mtx.Unlock()
	for _, tx := range chainedTxns[2 : maxOrphans+1] {
		testPoolMembership(tc, tx, false, false)
	}
}

// TestMultiInputOrphanDoubleSpend ensures that orphans that spend from an
// output that is spend by another transaction entering the pool are removed.
func TestMultiInputOrphanDoubleSpend(t *testing.T) {
	t.Parallel()

	const maxOrphans = 4
	harness, outputs, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	harness.txPool.cfg.Policy.MaxOrphanTxs = maxOrphans
	tc := &testContext{t, harness}

	// Create a chain of transactions rooted with the first spendable output
	// provided by the harness.
	chainedTxns, err := harness.CreateTxChain(outputs[0], maxOrphans+1)
	if err != nil {
		t.Fatalf("unable to create transaction chain: %v", err)
	}

	// Start by adding the orphan transactions from the generated chain
	// except the final one.
	for _, tx := range chainedTxns[1:maxOrphans] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, true,
			false, true)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid "+
				"orphan %v", err)
		}
		if len(acceptedTxns) != 0 {
			t.Fatalf("ProcessTransaction: reported %d accepted transactions "+
				"from what should be an orphan", len(acceptedTxns))
		}
		testPoolMembership(tc, tx, true, false)
	}

	// Ensure a transaction that contains a double spend of the same output
	// as the second orphan that was just added as well as a valid spend
	// from that last orphan in the chain generated above (and is not in the
	// orphan pool) is accepted to the orphan pool.  This must be allowed
	// since it would otherwise be possible for a malicious actor to disrupt
	// tx chains.
	doubleSpendTx, err := harness.CreateSignedTx([]spendableOutput{
		txOutToSpendableOut(chainedTxns[1], 0, wire.TxTreeRegular),
		txOutToSpendableOut(chainedTxns[maxOrphans], 0, wire.TxTreeRegular),
	}, 1)
	if err != nil {
		t.Fatalf("unable to create signed tx: %v", err)
	}
	acceptedTxns, err := harness.txPool.ProcessTransaction(doubleSpendTx,
		true, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid orphan %v",
			err)
	}
	if len(acceptedTxns) != 0 {
		t.Fatalf("ProcessTransaction: reported %d accepted transactions "+
			"from what should be an orphan", len(acceptedTxns))
	}
	testPoolMembership(tc, doubleSpendTx, true, false)

	// Add the transaction which completes the orphan chain and ensure the
	// chain gets accepted.  Notice the accept orphans flag is also false
	// here to ensure it has no bearing on whether or not already existing
	// orphans in the pool are linked.
	//
	// This will cause the shared output to become a concrete spend which
	// will in turn must cause the double spending orphan to be removed.
	acceptedTxns, err = harness.txPool.ProcessTransaction(chainedTxns[0],
		false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid tx %v", err)
	}
	if len(acceptedTxns) != maxOrphans {
		t.Fatalf("ProcessTransaction: reported accepted transactions "+
			"length does not match expected -- got %d, want %d",
			len(acceptedTxns), maxOrphans)
	}
	for _, tx := range acceptedTxns {
		// Ensure the transaction is no longer in the orphan pool, is
		// in the transaction pool, and is reported as available.
		testPoolMembership(tc, tx, false, true)
	}

	// Ensure the double spending orphan is no longer in the orphan pool and
	// was not moved to the transaction pool.
	testPoolMembership(tc, doubleSpendTx, false, false)
}

// mustLockTimeToSeq converts the passed relative lock time to a sequence number
// by using LockTimeToSequence.  It only differs in that it will panic if there
// is an error so errors in the source code can be detected.  It will only (and
// must only)  be called with hard-coded, and therefore known good, values.
func mustLockTimeToSeq(isSeconds bool, lockTime uint32) uint32 {
	sequence, err := blockchain.LockTimeToSequence(isSeconds, lockTime)
	if err != nil {
		panic(fmt.Sprintf("invalid lock time in source file: "+
			"isSeconds: %v, lockTime: %d", isSeconds, lockTime))
	}
	return sequence
}

// seqIntervalToSecs converts the passed number of sequence lock intervals into
// the number of seconds it represents.
func seqIntervalToSecs(intervals uint32) uint32 {
	return intervals << wire.SequenceLockTimeGranularity
}

// TestSequenceLockAcceptance ensures that transactions which involve sequence
// locks are accepted or rejected from the pool as expected.
func TestSequenceLockAcceptance(t *testing.T) {
	t.Parallel()

	// Shorter versions of variables for convenience.
	const seqLockTimeDisabled = wire.SequenceLockTimeDisabled
	const seqLockTimeIsSecs = wire.SequenceLockTimeIsSeconds

	tests := []struct {
		name         string // test description.
		txVersion    uint16 // transaction version.
		sequence     uint32 // sequence number used for input.
		heightOffset int64  // mock chain height offset at which to evaluate.
		secsOffset   int64  // mock median time offset at which to evaluate.
		valid        bool   // whether tx is valid when enforcing seq locks.
	}{
		{
			name:         "By-height lock with seq == height == 0",
			txVersion:    2,
			sequence:     mustLockTimeToSeq(false, 0),
			heightOffset: 0,
			valid:        true,
		},
		{
			// The mempool is for transactions to be included in the next block
			// so sequence locks are calculated based on that point of view.
			// Thus, a sequence lock of one for an input created at the current
			// height will be satisfied.
			name:         "By-height lock with seq == 1, height == 0",
			txVersion:    2,
			sequence:     mustLockTimeToSeq(false, 1),
			heightOffset: 0,
			valid:        true,
		},
		{
			name:         "By-height lock with seq == height == 65535",
			txVersion:    2,
			sequence:     mustLockTimeToSeq(false, 65535),
			heightOffset: 65534,
			valid:        true,
		},
		{
			name:         "By-height lock with masked max seq == height",
			txVersion:    2,
			sequence:     0xffffffff &^ seqLockTimeDisabled &^ seqLockTimeIsSecs,
			heightOffset: 65534,
			valid:        true,
		},
		{
			name:         "By-height lock with unsatisfied seq == 2",
			txVersion:    2,
			sequence:     mustLockTimeToSeq(false, 2),
			heightOffset: 0,
			valid:        false,
		},
		{
			name:         "By-height lock with unsatisfied masked max sequence",
			txVersion:    2,
			sequence:     0xffffffff &^ seqLockTimeDisabled &^ seqLockTimeIsSecs,
			heightOffset: 65533,
			valid:        false,
		},
		{
			name:       "By-time lock with seq == elapsed == 0",
			txVersion:  2,
			sequence:   mustLockTimeToSeq(true, 0),
			secsOffset: 0,
			valid:      true,
		},
		{
			name:       "By-time lock with seq == elapsed == max",
			txVersion:  2,
			sequence:   mustLockTimeToSeq(true, seqIntervalToSecs(65535)),
			secsOffset: int64(seqIntervalToSecs(65535)),
			valid:      true,
		},
		{
			name:       "By-time lock with unsatisfied seq == 1024",
			txVersion:  2,
			sequence:   mustLockTimeToSeq(true, seqIntervalToSecs(2)),
			secsOffset: int64(seqIntervalToSecs(1)),
			valid:      false,
		},
		{
			name:       "By-time lock with unsatisfied masked max sequence",
			txVersion:  2,
			sequence:   0xffffffff &^ seqLockTimeDisabled,
			secsOffset: int64(seqIntervalToSecs(65534)),
			valid:      false,
		},
		{
			name:         "Disabled by-height lock with seq == height == 0",
			txVersion:    2,
			sequence:     mustLockTimeToSeq(false, 0) | seqLockTimeDisabled,
			heightOffset: 0,
			valid:        true,
		},
		{
			name:         "Disabled by-height lock with unsatisfied sequence",
			txVersion:    2,
			sequence:     mustLockTimeToSeq(false, 2) | seqLockTimeDisabled,
			heightOffset: 0,
			valid:        true,
		},
		{
			name:       "Disabled by-time lock with seq == elapsed == 0",
			txVersion:  2,
			sequence:   mustLockTimeToSeq(true, 0) | seqLockTimeDisabled,
			secsOffset: 0,
			valid:      true,
		},
		{
			name:      "Disabled by-time lock with unsatisfied seq == 1024",
			txVersion: 2,
			sequence: mustLockTimeToSeq(true, seqIntervalToSecs(2)) |
				seqLockTimeDisabled,
			secsOffset: int64(seqIntervalToSecs(1)),
			valid:      true,
		},

		// The following section uses version 1 transactions which are not
		// subject to sequence locks.
		{
			name:         "By-height lock with seq == height == 0 (v1)",
			txVersion:    1,
			sequence:     mustLockTimeToSeq(false, 0),
			heightOffset: 0,
			valid:        true,
		},
		{
			name:         "By-height lock with unsatisfied seq == 2 (v1)",
			txVersion:    1,
			sequence:     mustLockTimeToSeq(false, 2),
			heightOffset: 0,
			valid:        true,
		},
		{
			name:       "By-time lock with seq == elapsed == 0 (v1)",
			txVersion:  1,
			sequence:   mustLockTimeToSeq(true, 0),
			secsOffset: 0,
			valid:      true,
		},
		{
			name:       "By-time lock with unsatisfied seq == 1024 (v1)",
			txVersion:  1,
			sequence:   mustLockTimeToSeq(true, seqIntervalToSecs(2)),
			secsOffset: int64(seqIntervalToSecs(1)),
			valid:      true,
		},
		{
			name:         "Disabled by-height lock with seq == height == 0 (v1)",
			txVersion:    1,
			sequence:     mustLockTimeToSeq(false, 0) | seqLockTimeDisabled,
			heightOffset: 0,
			valid:        true,
		},
		{
			name:         "Disabled by-height lock with unsatisfied seq (v1)",
			txVersion:    1,
			sequence:     mustLockTimeToSeq(false, 2) | seqLockTimeDisabled,
			heightOffset: 0,
			valid:        true,
		},
		{
			name:       "Disabled by-time lock with seq == elapsed == 0 (v1)",
			txVersion:  1,
			sequence:   mustLockTimeToSeq(true, 0) | seqLockTimeDisabled,
			secsOffset: 0,
			valid:      true,
		},
		{
			name:      "Disabled by-time lock with unsatisfied seq == 1024 (v1)",
			txVersion: 1,
			sequence: mustLockTimeToSeq(true, seqIntervalToSecs(2)) |
				seqLockTimeDisabled,
			secsOffset: int64(seqIntervalToSecs(1)),
			valid:      true,
		},
	}

	// Run through the tests twice such that the first time the pool is set to
	// reject all sequence locks and the second it is not.
	for _, acceptSeqLocks := range []bool{false, true} {
		harness, _, err := newPoolHarness(chaincfg.MainNetParams())
		if err != nil {
			t.Fatalf("unable to create test pool: %v", err)
		}
		tc := &testContext{t, harness}

		harness.chain.SetAcceptSequenceLocks(acceptSeqLocks)
		baseHeight := harness.chain.BestHeight()
		baseTime := time.Now()
		for i, test := range tests {
			// Create and add a mock utxo at a common base height so updating
			// the mock chain height below will cause sequence locks to be
			// evaluated relative to that height.
			//
			// The output value adds the test index in order to ensure the
			// resulting transaction hash is unique.
			inputMsgTx := wire.NewMsgTx()
			inputMsgTx.AddTxOut(&wire.TxOut{
				PkScript: harness.payScript,
				Value:    1000000000 + int64(i),
			})
			inputTx := dcrutil.NewTx(inputMsgTx)
			harness.AddFakeUTXO(inputTx, baseHeight)
			harness.chain.AddFakeUtxoMedianTime(inputTx, 0, baseTime)

			// Create a transaction which spends from the mock utxo with the
			// details specified in the test data.
			spendableOut := txOutToSpendableOut(inputTx, 0, wire.TxTreeRegular)
			inputs := []spendableOutput{spendableOut}
			tx, err := harness.CreateSignedTx(inputs, 1, func(tx *wire.MsgTx) {
				tx.Version = test.txVersion
				tx.TxIn[0].Sequence = test.sequence
			})
			if err != nil {
				t.Fatalf("unable to create tx: %v", err)
			}

			// Determine if the test data describes a transaction with an
			// enabled sequence lock.
			hasEnabledSeqLock := test.txVersion >= 2 &&
				test.sequence&wire.SequenceLockTimeDisabled == 0

			// Set the mock chain height and median time based on the test data
			// and ensure the transaction is either accepted or rejected as
			// desired.
			secsOffset := time.Second * time.Duration(test.secsOffset)
			harness.chain.SetHeight(baseHeight + test.heightOffset)
			harness.chain.SetPastMedianTime(baseTime.Add(secsOffset))
			acceptedTxns, err := harness.txPool.ProcessTransaction(tx, false,
				false, true)
			switch {
			case !acceptSeqLocks && hasEnabledSeqLock && err == nil:
				t.Fatalf("%s: did not reject tx when seq locks are not allowed",
					test.name)

			case !acceptSeqLocks && !hasEnabledSeqLock && err != nil:
				t.Fatalf("%s: did not accept tx: %v", test.name, err)

			case acceptSeqLocks && test.valid && err != nil:
				t.Fatalf("%s: did not accept tx: %v", test.name, err)

			case acceptSeqLocks && !test.valid && err == nil:
				t.Fatalf("%s: did not reject tx", test.name)

			case acceptSeqLocks && !test.valid && !IsErrorCode(err, ErrSeqLockUnmet):
				t.Fatalf("%s: did not get expected ErrSeqLockUnmet",
					test.name)
			}

			// Ensure the number of reported accepted transactions and pool
			// membership matches the expected result.
			shouldHaveAccepted := (acceptSeqLocks && test.valid) ||
				(!acceptSeqLocks && !hasEnabledSeqLock)
			switch {
			case shouldHaveAccepted:
				// Ensure the transaction was reported as accepted.
				if len(acceptedTxns) != 1 {
					t.Fatalf("%s: reported %d accepted transactions from what "+
						"should be 1", test.name, len(acceptedTxns))
				}

				// Ensure the transaction is not in the orphan pool, in the
				// transaction pool, and reported as available.
				testPoolMembership(tc, tx, false, true)

			case !shouldHaveAccepted:
				if len(acceptedTxns) != 0 {
					// Ensure no transactions were reported as accepted.
					t.Fatalf("%s: reported %d accepted transactions from what "+
						"should have been rejected", test.name, len(acceptedTxns))
				}

				// Ensure the transaction is not in the orphan pool, not in the
				// transaction pool, and not reported as available.
				testPoolMembership(tc, tx, false, false)
			}
		}
	}
}

// TestMaxVoteDoubleSpendRejection ensures that votes that spend the same ticket
// while voting on different blocks are accepted to the pool until the maximum
// allowed is reached and rejected afterwards.
func TestMaxVoteDoubleSpendRejection(t *testing.T) {
	t.Parallel()

	harness, spendableOuts, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a regular transaction from the first spendable output provided by
	// the harness.
	tx, err := harness.CreateTx(spendableOuts[0])
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// Create a ticket purchase transaction spending the outputs of the prior
	// regular transaction.
	ticket, err := harness.CreateTicketPurchase(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction: %v", err)
	}

	// Add the ticket outputs as utxos to fake their existence.  Use one after
	// the stake enabled height for the height of the fake utxos to ensure they
	// are mature for the votes cast a stake validation height below.
	harness.chain.SetHeight(harness.chainParams.StakeEnabledHeight + 1)
	harness.chain.utxos.AddTxOuts(ticket, harness.chain.BestHeight(), 0)

	// Create enough votes all using the same ticket and voting on different
	// blocks at stake validation height to be able to force rejection due to
	// exceeding the max allowed double spends.
	harness.chain.SetHeight(harness.chainParams.StakeValidationHeight)
	var votes []*dcrutil.Tx
	for i := 0; i < maxVoteDoubleSpends*2; i++ {
		// Ensure each vote is voting on a different block.
		var hash chainhash.Hash
		binary.LittleEndian.PutUint32(hash[:4], uint32(i))
		harness.chain.SetBestHash(&hash)

		vote, err := harness.CreateVote(ticket)
		if err != nil {
			t.Fatalf("unable to create vote: %v", err)
		}
		votes = append(votes, vote)
	}

	// Add enough of the votes to reach the max allowed while ensuring they are
	// all accepted.
	for _, vote := range votes[:maxVoteDoubleSpends] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(vote, false,
			false, true)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid vote %v", err)
		}

		// Ensure the transaction was reported as accepted.
		if len(acceptedTxns) != 1 {
			t.Fatalf("ProcessTransaction: reported %d accepted transactions from "+
				"what should be 1", len(acceptedTxns))
		}

		// Ensure the transaction is not in the orphan pool, in the transaction
		// pool, and reported as available.
		testPoolMembership(tc, vote, false, true)
	}

	// Attempt to add the remaining votes while ensuring they are all rejected
	// due to exceeding the max allowed double spends across all blocks being
	// voted on.
	for _, vote := range votes[maxVoteDoubleSpends:] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(vote, false,
			false, true)
		if err == nil {
			t.Fatalf("ProcessTransaction: accepted double-spending vote with " +
				"more than max allowed")
		}
		if !IsErrorCode(err, ErrTooManyVotes) {
			t.Fatalf("Process Transaction: did not get expected " +
				"ErrTooManyVotes error code")
		}

		// Ensure no transactions were reported as accepted.
		if len(acceptedTxns) != 0 {
			t.Fatalf("ProcessTransaction: reported %d accepted "+
				"transactions from what should be an orphan",
				len(acceptedTxns))
		}

		// Ensure the transaction is not in the orphan pool, not in the
		// transaction pool, and not reported as available.
		testPoolMembership(tc, vote, false, false)
	}

	// Remove one of the votes from the pool and ensure it is not in the orphan
	// pool, not in the transaction pool, and not reported as available.
	vote := votes[2]
	harness.txPool.RemoveTransaction(vote, true)
	testPoolMembership(tc, vote, false, false)

	// Add one of the votes that was rejected above due to the pool being at the
	// max allowed and ensure it is accepted now.  Also, ensure it is not in the
	// orphan pool, is in the transaction pool, and is reported as available.
	vote = votes[maxVoteDoubleSpends]
	_, err = harness.txPool.ProcessTransaction(vote, false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid vote %v", err)
	}
	testPoolMembership(tc, vote, false, true)

	// Attempt to add another one of the votes and ensure it is rejected due to
	// exceeding the max again.  Also, ensure it is not in the orphan pool, not
	// in the transaction pool, and not reported as available.
	vote = votes[maxVoteDoubleSpends+1]
	_, err = harness.txPool.ProcessTransaction(vote, false, false, true)
	if !IsErrorCode(err, ErrTooManyVotes) {
		t.Fatalf("Process Transaction: did not get expected " +
			"ErrTooManyVotes error code")
	}
	testPoolMembership(tc, vote, false, false)
}

// TestDuplicateVoteRejection ensures that additional votes on the same block
// that spend the same ticket are rejected from the pool as expected.
func TestDuplicateVoteRejection(t *testing.T) {
	t.Parallel()

	harness, spendableOuts, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a regular transaction from the first spendable output provided by
	// the harness.
	tx, err := harness.CreateTx(spendableOuts[0])
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// Create a ticket purchase transaction spending the outputs of the prior
	// regular transaction.
	ticket, err := harness.CreateTicketPurchase(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction: %v", err)
	}

	// Add the ticket outputs as utxos to fake their existence.  Use one after
	// the stake enabled height for the height of the fake utxos to ensure they
	// are matured for the votes cast a stake validation height below.
	harness.chain.SetHeight(harness.chainParams.StakeEnabledHeight + 1)
	harness.chain.utxos.AddTxOuts(ticket, harness.chain.BestHeight(), 0)

	// Create a vote that votes on a block at stake validation height.
	harness.chain.SetBestHash(&chainhash.Hash{0x5c, 0xa1, 0xab, 0x1e})
	harness.chain.SetHeight(harness.chainParams.StakeValidationHeight)
	vote, err := harness.CreateVote(ticket)
	if err != nil {
		t.Fatalf("unable to create vote: %v", err)
	}

	// Add the vote and ensure it is not in the orphan pool, is in the
	// transaction pool, and is reported as available.
	_, err = harness.txPool.ProcessTransaction(vote, false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid vote %v", err)
	}
	testPoolMembership(tc, vote, false, true)

	// Create another vote with a different hash that votes on the same block
	// using the same ticket.
	dupVote, err := harness.CreateVote(ticket, func(tx *wire.MsgTx) {
		voteBits := stake.VoteBits{Bits: uint16(0x03), ExtendedBits: nil}
		voteScript, err := newVoteScript(voteBits)
		if err != nil {
			t.Fatalf("failed to create vote script: %v", err)
		}
		tx.TxOut[1].PkScript = voteScript
	})
	if err != nil {
		t.Fatalf("unable to create vote: %v", err)
	}

	// Attempt to add the duplicate vote and ensure it is rejected.  Also,
	// ensure it is not in the orphan pool, not in the transaction pool, and not
	// reported as available.
	_, err = harness.txPool.ProcessTransaction(dupVote, false, false, true)
	if !IsErrorCode(err, ErrAlreadyVoted) {
		t.Fatalf("Process Transaction: did not get expected " +
			"ErrTooManyVotes error code")
	}
	testPoolMembership(tc, dupVote, false, false)

	// Remove the original vote from the pool and ensure it is not in the orphan
	// pool, not in the transaction pool, and not reported as available.
	harness.txPool.RemoveTransaction(vote, true)
	testPoolMembership(tc, vote, false, false)

	// Add the duplicate vote which should now be accepted.  Also, ensure it is
	// not in the orphan pool, is in the transaction pool, and is reported as
	// available.
	_, err = harness.txPool.ProcessTransaction(dupVote, false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid vote %v", err)
	}
	testPoolMembership(tc, dupVote, false, true)
}

// TestDuplicateTxError ensures that attempting to add a transaction to the
// pool which is an exact duplicate of another transaction fails with the
// appropriate error.
func TestDuplicateTxError(t *testing.T) {
	t.Parallel()

	harness, spendableOuts, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a regular transaction from the first spendable output provided by
	// the harness.
	tx, err := harness.CreateTx(spendableOuts[0])
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// Ensure the transaction is accepted to the pool.
	_, err = harness.txPool.ProcessTransaction(tx, true, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept initial tx: %v", err)
	}
	testPoolMembership(tc, tx, false, true)

	// Ensure a second attempt to process the tx is rejected with the
	// correct error code and that the transaction remains in the pool.
	_, err = harness.txPool.ProcessTransaction(tx, true, false, true)
	if !IsErrorCode(err, ErrDuplicate) {
		t.Fatalf("ProcessTransaction: did get the expected ErrDuplicate")
	}
	testPoolMembership(tc, tx, false, true)

	// Create an orphan transaction to perform the same test but this time
	// in the orphan pool. The orphan tx is the second one in the created
	// chain.
	txs, err := harness.CreateTxChain(txOutToSpendableOut(tx, 0, 0), 2)
	if err != nil {
		t.Fatalf("unable to create orphan chain: %v", err)
	}
	orphan := txs[1]

	// The first call to ProcessTransaction should succeed when enabling
	// orphans.
	_, err = harness.txPool.ProcessTransaction(orphan, true, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept orphan tx: %v", err)
	}
	testPoolMembership(tc, orphan, true, false)

	// The second call should fail with the expected ErrDuplicate error.
	_, err = harness.txPool.ProcessTransaction(orphan, true, false, true)
	if !IsErrorCode(err, ErrDuplicate) {
		t.Fatalf("ProcessTransaction: did not get expected ErrDuplicate")
	}
	testPoolMembership(tc, orphan, true, false)
}

// TestMempoolDoubleSpend ensures that attempting to add a transaction to the
// pool which spends an output already in the mempool fails for the correct
// reason.
func TestMempoolDoubleSpend(t *testing.T) {
	t.Parallel()

	harness, spendableOuts, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a regular transaction from the first spendable output provided by
	// the harness.
	tx, err := harness.CreateTx(spendableOuts[0])
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// Ensure the transaction is accepted to the pool.
	_, err = harness.txPool.ProcessTransaction(tx, true, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept initial tx: %v", err)
	}
	testPoolMembership(tc, tx, false, true)

	// Create a second transaction, spending the same outputs. Create with
	// 2 outputs so that it is a different transaction than the original
	// one.
	doubleSpendTx, err := harness.CreateSignedTx(spendableOuts, 2)
	if err != nil {
		t.Fatalf("unable to create double spend tx: %v", err)
	}

	// Ensure a second attempt to process the tx is rejected with the
	// correct error code, that the original transaction remains in the
	// pool and the double spend is not added to the pool.
	_, err = harness.txPool.ProcessTransaction(doubleSpendTx, true, false, true)
	if !IsErrorCode(err, ErrMempoolDoubleSpend) {
		t.Fatalf("ProcessTransaction: did not get expected ErrMempoolDoubleSpend")
	}
	testPoolMembership(tc, tx, false, true)
	testPoolMembership(tc, doubleSpendTx, false, false)
}
