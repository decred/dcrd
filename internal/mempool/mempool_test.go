// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/blockchain/v5"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/internal/mining"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/sign"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/txscript/v4/stdscript"
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
	nextStakeDiff int64
	utxos         *blockchain.UtxoViewpoint
	utxoTimes     map[wire.OutPoint]int64
	blocks        map[chainhash.Hash]*dcrutil.Block
	currentHash   chainhash.Hash
	currentHeight int64
	medianTime    time.Time
	scriptFlags   txscript.ScriptFlags
	tspendMined   map[chainhash.Hash]struct{}
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

// FetchUtxoView loads unspent transaction outputs for the inputs referenced by
// the passed transaction from the point of view of the main chain tip while
// taking into account whether or not the transactions in the regular tree of
// the current tip block should be included or not depending on the provided
// flag.  It also attempts to fetch the utxos for the outputs of the transaction
// so the returned view can be examined for duplicate transactions.
//
// This function is safe for concurrent access however the returned view is NOT.
func (s *fakeChain) FetchUtxoView(tx *dcrutil.Tx, treeValid bool) (*blockchain.UtxoViewpoint, error) {
	s.RLock()
	defer s.RUnlock()

	// All entries are cloned to ensure modifications to the returned view
	// do not affect the fake chain's view.

	// Add entries for the outputs of the tx to the new view.
	msgTx := tx.MsgTx()
	viewpoint := blockchain.NewUtxoViewpoint(nil)
	outpoint := wire.OutPoint{Hash: *tx.Hash(), Tree: tx.Tree()}
	for txOutIdx := range msgTx.TxOut {
		outpoint.Index = uint32(txOutIdx)
		entry := s.utxos.LookupEntry(outpoint)
		viewpoint.Entries()[outpoint] = entry.Clone()
	}

	// Add entries for all of the inputs to the tx to the new view.
	for _, txIn := range msgTx.TxIn {
		entry := s.utxos.LookupEntry(txIn.PreviousOutPoint)
		viewpoint.Entries()[txIn.PreviousOutPoint] = entry.Clone()
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

// HeaderByHash returns the header for the block with the given hash from the
// fake chain instance.  Blocks can be added to the instance with the AddBlock
// function.
func (s *fakeChain) HeaderByHash(hash *chainhash.Hash) (wire.BlockHeader, error) {
	block, ok := s.blocks[*hash]
	if !ok {
		return wire.BlockHeader{}, fmt.Errorf("unable to find block %v in fake "+
			"chain", hash)
	}
	return block.MsgBlock().Header, nil
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
	if !enforce || standalone.IsCoinBaseTx(msgTx, noTreasury) ||
		stake.IsSSGen(msgTx) {
		return sequenceLock, nil
	}

	for txInIndex, txIn := range msgTx.TxIn {
		// Nothing to calculate for this input when relative time locks are
		// disabled for it.
		sequenceNum := txIn.Sequence
		if sequenceNum&wire.SequenceLockTimeDisabled != 0 {
			continue
		}

		utxo := view.LookupEntry(txIn.PreviousOutPoint)
		if utxo == nil {
			str := fmt.Sprintf("output %v referenced from transaction %s:%d "+
				"either does not exist or has already been spent",
				txIn.PreviousOutPoint, tx.Hash(), txInIndex)
			return nil, blockchain.RuleError{
				Err:         blockchain.ErrMissingTxOut,
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

// TSpendMinedOnAncestor returns whether the given tx hash has been marked on
// the fake chain as a tspend that has been mined on an ancestor block.
func (s *fakeChain) TSpendMinedOnAncestor(txh chainhash.Hash) error {
	s.Lock()
	_, tspendMined := s.tspendMined[txh]
	s.Unlock()
	if tspendMined {
		return errors.New("tspend mined")
	}
	return nil
}

// SetTSpendMinedOnAncestor sets the status on whether the given txhash is a
// tspend that has been mined on an ancestor block.
func (s *fakeChain) SetTSpendMinedOnAncestor(txh chainhash.Hash, mined bool) {
	s.Lock()
	if mined {
		s.tspendMined[txh] = struct{}{}
	} else {
		delete(s.tspendMined, txh)
	}
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
	//
	// payScriptVer and payScript are the script version and script to pay the
	// aforementioned payAddr.
	signKey               []byte
	sigType               dcrec.SignatureType
	payAddr               stdaddr.StakeAddress
	payScriptVer          uint16
	payScript             []byte
	chainParams           *chaincfg.Params
	treasuryActive        bool
	autoRevocationsActive bool
	subsidySplitActive    bool

	chain  *fakeChain
	txPool *TxPool
}

// GetScript is the pool harness' implementation of the ScriptDB interface.
// It returns the pool harness' payment redeem script for any address
// passed in.
func (p *poolHarness) GetScript(addr stdaddr.Address) ([]byte, error) {
	return p.payScript, nil
}

// GetKey is the pool harness' implementation of the KeyDB interface.
// It returns the pool harness' signature key for any address passed in.
func (p *poolHarness) GetKey(addr stdaddr.Address) ([]byte, dcrec.SignatureType, bool, error) {
	return p.signKey, p.sigType, true, nil
}

// AddFakeUTXO creates a fake mined utxo for the provided transaction.
func (p *poolHarness) AddFakeUTXO(tx *dcrutil.Tx, blockHeight int64, blockIndex uint32) {
	p.chain.utxos.AddTxOuts(tx, blockHeight, blockIndex, noTreasury)
}

// CreateCoinbaseTx returns a coinbase transaction with the requested number of
// outputs paying an appropriate subsidy based on the passed block height to the
// address associated with the harness.  It automatically uses a standard
// signature script that starts with the required block height.
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
	remainder := int64(totalInput) % int64(numOutputs)

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
			amount += remainder
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
		sigScript, err := sign.SignatureScript(tx, i, p.payScript,
			txscript.SigHashAll, p.signKey, dcrec.STEcdsaSecp256k1, true)
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
		sigScript, err := sign.SignatureScript(tx, 0, p.payScript,
			txscript.SigHashAll, p.signKey, dcrec.STEcdsaSecp256k1, true)
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

// CreateTicketPurchase creates a ticket purchase from the provided spendable
// output.
func (p *poolHarness) CreateTicketPurchase(input spendableOutput, cost int64, mungers ...func(*wire.MsgTx)) (*dcrutil.Tx, error) {
	ticketFee := singleInputTicketSize
	ticketPrice := cost

	// Generate the voting rights, commitment, and change scripts of the ticket.
	voteScriptVer, voteScript := p.payAddr.VotingRightsScript()
	commitScriptVer, commitScript := p.payAddr.RewardCommitmentScript(
		ticketPrice+ticketFee, 0, ticketPrice)
	change := int64(input.amount) - ticketPrice - ticketFee
	changeScriptVer, changeScript := p.payAddr.StakeChangeScript()

	// Generate the ticket purchase.
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  input.outPoint.Hash,
			Index: 0,
			Tree:  wire.TxTreeRegular,
		},
		Sequence:    wire.MaxTxInSequenceNum,
		ValueIn:     int64(input.amount),
		BlockHeight: uint32(p.chain.BestHeight()),
	})

	tx.AddTxOut(newTxOut(ticketPrice, voteScriptVer, voteScript))
	tx.AddTxOut(newTxOut(0, commitScriptVer, commitScript))
	tx.AddTxOut(newTxOut(change, changeScriptVer, changeScript))

	// Perform any transaction munging just before signing.
	for _, f := range mungers {
		f(tx)
	}

	// Sign the ticket purchase.
	sigScript, err := sign.SignatureScript(tx, 0, p.payScript,
		txscript.SigHashAll, p.signKey, dcrec.STEcdsaSecp256k1, true)
	if err != nil {
		return nil, err
	}
	tx.TxIn[0].SignatureScript = sigScript

	return dcrutil.NewTx(tx), nil
}

// CreateTicketPurchaseFromTx creates a ticket purchase spending the first
// output of the provided transaction.
func (p *poolHarness) CreateTicketPurchaseFromTx(sourceTx *dcrutil.Tx, cost int64) (*dcrutil.Tx, error) {
	spend := txOutToSpendableOut(sourceTx, 0, wire.TxTreeRegular)
	return p.CreateTicketPurchase(spend, cost)
}

// newVoteScript generates a voting script from the passed VoteBits, for
// use in a vote.
func newVoteScript(voteBits stake.VoteBits) ([]byte, error) {
	b := make([]byte, 2+len(voteBits.ExtendedBits))
	binary.LittleEndian.PutUint16(b[0:2], voteBits.Bits)
	copy(b[2:], voteBits.ExtendedBits)
	return stdscript.ProvablyPruneableScriptV0(b)
}

// newTxOut returns a new transaction output with the given parameters.
func newTxOut(amount int64, pkScriptVer uint16, pkScript []byte) *wire.TxOut {
	return &wire.TxOut{
		Value:    amount,
		Version:  pkScriptVer,
		PkScript: pkScript,
	}
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
	subsidy := subsidyCache.CalcStakeVoteSubsidyV2(p.chain.BestHeight(),
		p.subsidySplitActive)
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

	// Create payment scripts for the ticket commitments.
	params := p.chainParams
	for i, h160 := range ticketHash160s {
		var addr stdaddr.StakeAddress
		if ticketPayKinds[i] { // P2SH
			addr, _ = stdaddr.NewAddressScriptHashV0FromHash(h160, params)
		} else {
			addr, _ = stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(h160, params)
		}

		scriptVer, script := addr.PayVoteCommitmentScript()
		vote.AddTxOut(newTxOut(voteRewardValues[i], scriptVer, script))
	}

	// Perform any transaction munging just before signing.
	for _, f := range mungers {
		f(vote)
	}

	// Sign the input.
	inputToSign := 1
	redeemTicketScript := ticket.MsgTx().TxOut[0].PkScript
	signedScript, err := sign.SignTxOutput(p.chainParams, vote, inputToSign,
		redeemTicketScript, txscript.SigHashAll, p, p,
		vote.TxIn[inputToSign].SignatureScript, noTreasury)
	if err != nil {
		return nil, err
	}

	vote.TxIn[0].SignatureScript = p.chainParams.StakeBaseSigScript
	vote.TxIn[1].SignatureScript = signedScript

	return dcrutil.NewTx(vote), nil
}

// CreateRevocation creates a revocation using the provided ticket.
func (p *poolHarness) CreateRevocation(ticket *dcrutil.Tx,
	revocationTxFee dcrutil.Amount) (*dcrutil.Tx, error) {

	ticketMsgTx := ticket.MsgTx()
	ticketHash := ticketMsgTx.TxHash()

	// Set the transaction version based on whether the automatic ticket
	// revocations agenda is active or not.
	revocationTxVersion := uint16(1)
	if p.autoRevocationsActive {
		revocationTxVersion = stake.TxVersionAutoRevocations
	}

	// Get the best header bytes.
	bestHeader, err := p.chain.HeaderByHash(p.chain.BestHash())
	if err != nil {
		return nil, err
	}
	bestHeaderBytes, err := bestHeader.Bytes()
	if err != nil {
		return nil, err
	}

	// Create the revocation transaction.
	ticketMinOuts := stake.ConvertToMinimalOutputs(ticketMsgTx)
	revocation, err := stake.CreateRevocationFromTicket(&ticketHash,
		ticketMinOuts, revocationTxFee, revocationTxVersion, p.chainParams,
		bestHeaderBytes, p.autoRevocationsActive)
	if err != nil {
		return nil, err
	}

	// Sign the input unless the automatic ticket revocations agenda is active.
	if !p.autoRevocationsActive {
		inputToSign := 0
		redeemTicketScript := ticketMsgTx.TxOut[0].PkScript
		signedScript, err := sign.SignTxOutput(p.chainParams, revocation,
			inputToSign, redeemTicketScript, txscript.SigHashAll, p, p,
			revocation.TxIn[inputToSign].SignatureScript, noTreasury)
		if err != nil {
			return nil, err
		}
		revocation.TxIn[0].SignatureScript = signedScript
	}

	return dcrutil.NewTx(revocation), nil
}

// SetTreasuryAgendaActive sets whether the treasury agenda should be
// considered active by the mempool.
func (p *poolHarness) SetTreasuryAgendaActive(active bool) {
	p.treasuryActive = active

	// Set or clear the treasury verification flag depending on the state
	// of the agenda.
	scriptFlags, _ := p.chain.StandardVerifyFlags()
	if active {
		scriptFlags |= txscript.ScriptVerifyTreasury

	} else {
		scriptFlags &^= txscript.ScriptVerifyTreasury

	}
	p.chain.SetStandardVerifyFlags(scriptFlags)
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
	signPub := secp256k1.PrivKeyFromBytes(keyBytes).PubKey()

	// Generate associated pay-to-script-hash address and resulting payment
	// script.
	pubKeyBytes := signPub.SerializeCompressed()
	h160 := stdaddr.Hash160(pubKeyBytes)
	payAddr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(h160,
		chainParams)
	if err != nil {
		return nil, nil, err
	}
	payScriptVer, payScript := payAddr.PaymentScript()

	// Create a new fake chain and harness bound to it.
	subsidyCache := standalone.NewSubsidyCache(chainParams)
	chain := &fakeChain{
		utxos:       blockchain.NewUtxoViewpoint(nil),
		utxoTimes:   make(map[wire.OutPoint]int64),
		blocks:      make(map[chainhash.Hash]*dcrutil.Block),
		scriptFlags: BaseStandardVerifyFlags,
		tspendMined: make(map[chainhash.Hash]struct{}),
	}
	var harness *poolHarness
	harness = &poolHarness{
		signKey:      keyBytes,
		sigType:      dcrec.STEcdsaSecp256k1,
		payAddr:      payAddr,
		payScriptVer: payScriptVer,
		payScript:    payScript,
		chainParams:  chainParams,
		chain:        chain,
		txPool: New(&Config{
			Policy: Policy{
				EnableAncestorTracking: true,
				DisableRelayPriority:   true,
				FreeTxRelayLimit:       15.0,
				MaxOrphanTxs:           5,
				MaxOrphanTxSize:        1000,
				MaxSigOpsPerTx:         blockchain.MaxSigOpsPerBlock / 5,
				MinRelayTxFee:          1000, // 1 Atom per byte
				MaxVoteAge: func() uint16 {
					switch chainParams.Net {
					case wire.MainNet, wire.SimNet, wire.RegNet:
						return chainParams.CoinbaseMaturity

					case wire.TestNet3:
						return 1440 // defaultMaximumVoteAge

					default:
						return chainParams.CoinbaseMaturity
					}
				}(),
				StandardVerifyFlags: chain.StandardVerifyFlags,
			},
			ChainParams:           chainParams,
			NextStakeDifficulty:   chain.NextStakeDifficulty,
			FetchUtxoView:         chain.FetchUtxoView,
			BlockByHash:           chain.BlockByHash,
			BestHash:              chain.BestHash,
			BestHeight:            chain.BestHeight,
			HeaderByHash:          chain.HeaderByHash,
			PastMedianTime:        chain.PastMedianTime,
			CalcSequenceLock:      chain.CalcSequenceLock,
			TSpendMinedOnAncestor: chain.TSpendMinedOnAncestor,
			SubsidyCache:          subsidyCache,
			SigCache:              nil,
			AddrIndex:             nil,
			ExistsAddrIndex:       nil,
			OnVoteReceived:        nil,
			IsTreasuryAgendaActive: func() (bool, error) {
				return harness.treasuryActive, nil
			},
			IsAutoRevocationsAgendaActive: func() (bool, error) {
				return harness.autoRevocationsActive, nil
			},
			IsSubsidySplitAgendaActive: func() (bool, error) {
				return harness.subsidySplitActive, nil
			},
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
	harness.chain.utxos.AddTxOuts(coinbase, curHeight+1, wire.NullBlockIndex,
		noTreasury)
	for i := uint32(0); i < numOutputs; i++ {
		outputs = append(outputs, txOutToSpendableOut(coinbase, i, wire.TxTreeRegular))
	}
	harness.chain.SetHeight(int64(chainParams.CoinbaseMaturity) + curHeight)
	harness.chain.SetPastMedianTime(time.Now())

	// Mock the chain best block and state.
	mockBestBlock := *dcrutil.NewBlock(&wire.MsgBlock{})
	mockBestHash := mockBestBlock.Hash()
	harness.chain.blocks[*mockBestHash] = &mockBestBlock
	harness.chain.SetBestHash(mockBestHash)

	return harness, outputs, nil
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
	gotTxStaged := tc.harness.txPool.isTransactionStaged(txHash)
	if gotTxStaged && (gotOrphanPool || gotTxPool) {
		_, file, line, _ := runtime.Caller(1)
		tc.t.Fatalf("%s:%d -- HaveTransaction: tx exists "+
			"in multiple pools. staged: %v, txpool: %v, orphan: %v",
			file, line, gotTxStaged, gotTxPool, gotOrphanPool)
	}
	if wantHaveTx != gotHaveTx && !gotTxStaged {
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
			false, true, 0)
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
		false, false, true, 0)
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
	ticket, err := harness.CreateTicketPurchaseFromTx(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction %v", err)
	}

	// Ensure the ticket purchase is accepted as an orphan.
	acceptedTxns, err := harness.txPool.ProcessTransaction(ticket, true,
		false, true, 0)
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
	_, err = harness.txPool.ProcessTransaction(tx, false, false, true, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)
	testPoolMembership(tc, ticket, false, false)

	// Remove the transaction from the mempool. This causes the ticket
	// in the stage pool to enter the mempool.
	harness.AddFakeUTXO(tx, int64(ticket.MsgTx().TxIn[0].BlockHeight),
		wire.NullBlockIndex)
	harness.txPool.RemoveTransaction(tx, false, noTreasury, noAutoRevocations)
	harness.txPool.MaybeAcceptDependents(tx, noTreasury)

	testPoolMembership(tc, tx, false, false)
	testPoolMembership(tc, ticket, false, true)

	// Add the transaction back to the mempool to ensure it
	// kicks the ticket out to the stage pool.
	harness.AddFakeUTXO(tx, int64(mining.UnminedHeight), wire.NullBlockIndex)
	outpoint := wire.OutPoint{Hash: *tx.Hash(), Tree: wire.TxTreeRegular, Index: 0}
	harness.chain.utxos.LookupEntry(outpoint).Spend()
	_, err = harness.txPool.MaybeAcceptTransaction(tx, false, false)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}

	testPoolMembership(tc, tx, false, true)
	testPoolMembership(tc, ticket, false, false)
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
	ticket, err := harness.CreateTicketPurchaseFromTx(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction: %v", err)
	}

	harness.chain.SetHeight(harness.chainParams.StakeValidationHeight)

	vote, err := harness.CreateVote(ticket)
	if err != nil {
		t.Fatalf("unable to create vote: %v", err)
	}

	// Ensure the vote is rejected because it is an orphan.
	_, err = harness.txPool.ProcessTransaction(vote, false, false, true, 0)
	if !errors.Is(err, ErrOrphan) {
		t.Fatalf("Process Transaction: did not get expected ErrOrphan")
	}
	testPoolMembership(tc, vote, false, false)

	// Ensure the ticket is accepted as an orphan.
	_, err = harness.txPool.ProcessTransaction(ticket, true, false, true, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, ticket, true, false)

	// Ensure the regular tx whose output is spent by the ticket is accepted.
	_, err = harness.txPool.ProcessTransaction(tx, false, false, true, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)

	// Generate a fake mined utxo for the ticket created.
	harness.AddFakeUTXO(ticket, int64(ticket.MsgTx().TxIn[0].BlockHeight),
		wire.NullBlockIndex)

	// Ensure the previously rejected vote is accepted now since all referenced
	// utxos can now be found.
	_, err = harness.txPool.ProcessTransaction(vote, false, false, true, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept orphan transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)
	testPoolMembership(tc, ticket, false, false)
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
	ticket, err := harness.CreateTicketPurchaseFromTx(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction: %v", err)
	}

	harness.chain.SetHeight(harness.chainParams.StakeValidationHeight + 1)

	revocation, err := harness.CreateRevocation(ticket, 0)
	if err != nil {
		t.Fatalf("unable to create revocation: %v", err)
	}

	// Ensure the vote is rejected because it is an orphan.
	_, err = harness.txPool.ProcessTransaction(revocation, false, false, true,
		0)
	if !errors.Is(err, ErrOrphan) {
		t.Fatalf("Process Transaction: did not get expected " +
			"ErrTooManyVotes error")
	}
	testPoolMembership(tc, revocation, false, false)

	// Ensure the ticket is accepted as an orphan.
	_, err = harness.txPool.ProcessTransaction(ticket, true, false, true, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, ticket, true, false)

	// Ensure the regular tx whose output is spent by the ticket is accepted.
	_, err = harness.txPool.ProcessTransaction(tx, false, false, true, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)

	// Generate a fake mined utxos for the ticket created.
	harness.AddFakeUTXO(ticket, int64(ticket.MsgTx().TxIn[0].BlockHeight),
		wire.NullBlockIndex)

	// Ensure the previously rejected revocation is accepted now since all referenced
	// utxos can now be found.
	_, err = harness.txPool.ProcessTransaction(revocation, false, false, true,
		0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept orphan transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)
	testPoolMembership(tc, ticket, false, false)
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
			false, true, 0)
		if err == nil {
			t.Fatalf("ProcessTransaction: did not fail on orphan "+
				"%v when allow orphans flag is false", tx.Hash())
		}
		if !errors.Is(err, ErrOrphan) {
			t.Fatalf("ProcessTransaction: unexpected error -- got %v, want %v",
				err, ErrOrphan)
		}

		// Ensure no transactions were reported as accepted.
		if len(acceptedTxns) != 0 {
			t.Fatalf("ProcessTransaction: reported %d accepted "+
				"transactions from failed orphan attempt",
				len(acceptedTxns))
		}

		// Ensure the transaction is not in the orphan pool, not in the
		// transaction pool, and not reported as available
		testPoolMembership(tc, tx, false, false)
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
			false, true, 0)
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
		true, false, true, 0)
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
			true, 0)
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
			false, true, 0)
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

	harness.txPool.RemoveOrphan(nonChainedOrphanTx, noTreasury, noAutoRevocations)
	testPoolMembership(tc, nonChainedOrphanTx, false, false)
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		testPoolMembership(tc, tx, true, false)
	}

	// Attempt to remove an orphan that has an existing redeemer but itself
	// is not present and ensure the state of all other orphans (including
	// the one that redeems it) are unaffected.
	harness.txPool.RemoveOrphan(chainedTxns[0], noTreasury, noAutoRevocations)
	testPoolMembership(tc, chainedTxns[0], false, false)
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		testPoolMembership(tc, tx, true, false)
	}

	// Remove each orphan one-by-one and ensure they are removed as
	// expected.
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		harness.txPool.RemoveOrphan(tx, noTreasury, noAutoRevocations)
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
			false, true, 0)
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
	harness.txPool.removeOrphan(chainedTxns[1], false, noTreasury,
		noAutoRevocations)
	harness.txPool.mtx.Unlock()
	testPoolMembership(tc, chainedTxns[1], false, false)
	for _, tx := range chainedTxns[2 : maxOrphans+1] {
		testPoolMembership(tc, tx, true, false)
	}

	// Remove the first remaining orphan that starts the orphan chain with
	// the remove redeemer flag set and ensure they are all removed.
	harness.txPool.mtx.Lock()
	harness.txPool.removeOrphan(chainedTxns[2], true, noTreasury,
		noAutoRevocations)
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
			false, true, 0)
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
		true, false, true, 0)
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
		false, false, true, 0)
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
		err          error  // expected error when enforcing seq locks.
	}{{
		name:         "By-height lock with seq == height == 0",
		txVersion:    2,
		sequence:     mustLockTimeToSeq(false, 0),
		heightOffset: 0,
		err:          nil,
	}, {
		// The mempool is for transactions to be included in the next block so
		// sequence locks are calculated based on that point of view.  Thus, a
		// sequence lock of one for an input created at the current height will
		// be satisfied.
		name:         "By-height lock with seq == 1, height == 0",
		txVersion:    2,
		sequence:     mustLockTimeToSeq(false, 1),
		heightOffset: 0,
		err:          nil,
	}, {
		name:         "By-height lock with seq == height == 65535",
		txVersion:    2,
		sequence:     mustLockTimeToSeq(false, 65535),
		heightOffset: 65534,
		err:          nil,
	}, {
		name:         "By-height lock with masked max seq == height",
		txVersion:    2,
		sequence:     0xffffffff &^ seqLockTimeDisabled &^ seqLockTimeIsSecs,
		heightOffset: 65534,
		err:          nil,
	}, {
		name:         "By-height lock with unsatisfied seq == 2",
		txVersion:    2,
		sequence:     mustLockTimeToSeq(false, 2),
		heightOffset: 0,
		err:          ErrSeqLockUnmet,
	}, {
		name:         "By-height lock with unsatisfied masked max sequence",
		txVersion:    2,
		sequence:     0xffffffff &^ seqLockTimeDisabled &^ seqLockTimeIsSecs,
		heightOffset: 65533,
		err:          ErrSeqLockUnmet,
	}, {
		name:       "By-time lock with seq == elapsed == 0",
		txVersion:  2,
		sequence:   mustLockTimeToSeq(true, 0),
		secsOffset: 0,
		err:        nil,
	}, {
		name:       "By-time lock with seq == elapsed == max",
		txVersion:  2,
		sequence:   mustLockTimeToSeq(true, seqIntervalToSecs(65535)),
		secsOffset: int64(seqIntervalToSecs(65535)),
		err:        nil,
	}, {
		name:       "By-time lock with unsatisfied seq == 1024",
		txVersion:  2,
		sequence:   mustLockTimeToSeq(true, seqIntervalToSecs(2)),
		secsOffset: int64(seqIntervalToSecs(1)),
		err:        ErrSeqLockUnmet,
	}, {
		name:       "By-time lock with unsatisfied masked max sequence",
		txVersion:  2,
		sequence:   0xffffffff &^ seqLockTimeDisabled,
		secsOffset: int64(seqIntervalToSecs(65534)),
		err:        ErrSeqLockUnmet,
	}, {
		name:         "Disabled by-height lock with seq == height == 0",
		txVersion:    2,
		sequence:     mustLockTimeToSeq(false, 0) | seqLockTimeDisabled,
		heightOffset: 0,
		err:          nil,
	}, {
		name:         "Disabled by-height lock with unsatisfied sequence",
		txVersion:    2,
		sequence:     mustLockTimeToSeq(false, 2) | seqLockTimeDisabled,
		heightOffset: 0,
		err:          nil,
	}, {
		name:       "Disabled by-time lock with seq == elapsed == 0",
		txVersion:  2,
		sequence:   mustLockTimeToSeq(true, 0) | seqLockTimeDisabled,
		secsOffset: 0,
		err:        nil,
	}, {
		name:      "Disabled by-time lock with unsatisfied seq == 1024",
		txVersion: 2,
		sequence: mustLockTimeToSeq(true, seqIntervalToSecs(2)) |
			seqLockTimeDisabled,
		secsOffset: int64(seqIntervalToSecs(1)),
		err:        nil,
	}, {
		// The following section uses version 1 transactions which are not
		// subject to sequence locks.
		name:         "By-height lock with seq == height == 0 (v1)",
		txVersion:    1,
		sequence:     mustLockTimeToSeq(false, 0),
		heightOffset: 0,
		err:          nil,
	}, {
		name:         "By-height lock with unsatisfied seq == 2 (v1)",
		txVersion:    1,
		sequence:     mustLockTimeToSeq(false, 2),
		heightOffset: 0,
		err:          nil,
	}, {
		name:       "By-time lock with seq == elapsed == 0 (v1)",
		txVersion:  1,
		sequence:   mustLockTimeToSeq(true, 0),
		secsOffset: 0,
		err:        nil,
	}, {
		name:       "By-time lock with unsatisfied seq == 1024 (v1)",
		txVersion:  1,
		sequence:   mustLockTimeToSeq(true, seqIntervalToSecs(2)),
		secsOffset: int64(seqIntervalToSecs(1)),
		err:        nil,
	}, {
		name:         "Disabled by-height lock with seq == height == 0 (v1)",
		txVersion:    1,
		sequence:     mustLockTimeToSeq(false, 0) | seqLockTimeDisabled,
		heightOffset: 0,
		err:          nil,
	}, {
		name:         "Disabled by-height lock with unsatisfied seq (v1)",
		txVersion:    1,
		sequence:     mustLockTimeToSeq(false, 2) | seqLockTimeDisabled,
		heightOffset: 0,
		err:          nil,
	}, {
		name:       "Disabled by-time lock with seq == elapsed == 0 (v1)",
		txVersion:  1,
		sequence:   mustLockTimeToSeq(true, 0) | seqLockTimeDisabled,
		secsOffset: 0,
		err:        nil,
	}, {
		name:      "Disabled by-time lock with unsatisfied seq == 1024 (v1)",
		txVersion: 1,
		sequence: mustLockTimeToSeq(true, seqIntervalToSecs(2)) |
			seqLockTimeDisabled,
		secsOffset: int64(seqIntervalToSecs(1)),
		err:        nil,
	}}

	harness, _, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	baseHeight := harness.chain.BestHeight()
	baseTime := time.Now()
	for i, test := range tests {
		// Create and add a mock utxo at a common base height so updating the
		// mock chain height below will cause sequence locks to be evaluated
		// relative to that height.
		//
		// The output value adds the test index in order to ensure the resulting
		// transaction hash is unique.
		inputMsgTx := wire.NewMsgTx()
		inputMsgTx.AddTxOut(&wire.TxOut{
			PkScript: harness.payScript,
			Value:    1000000000 + int64(i),
		})
		inputTx := dcrutil.NewTx(inputMsgTx)
		harness.AddFakeUTXO(inputTx, baseHeight, wire.NullBlockIndex)
		harness.chain.AddFakeUtxoMedianTime(inputTx, 0, baseTime)

		// Create a transaction which spends from the mock utxo with the details
		// specified in the test data.
		spendableOut := txOutToSpendableOut(inputTx, 0, wire.TxTreeRegular)
		inputs := []spendableOutput{spendableOut}
		tx, err := harness.CreateSignedTx(inputs, 1, func(tx *wire.MsgTx) {
			tx.Version = test.txVersion
			tx.TxIn[0].Sequence = test.sequence
		})
		if err != nil {
			t.Fatalf("unable to create tx: %v", err)
		}

		// Set the mock chain height and median time based on the test data and
		// ensure the transaction is either accepted or rejected as desired.
		secsOffset := time.Second * time.Duration(test.secsOffset)
		harness.chain.SetHeight(baseHeight + test.heightOffset)
		harness.chain.SetPastMedianTime(baseTime.Add(secsOffset))
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, false,
			false, true, 0)
		if !errors.Is(err, test.err) {
			t.Fatalf("%s: unexpected err -- got %v, want %v", test.name, err,
				test.err)
		}

		// Ensure the number of reported accepted transactions and pool
		// membership matches the expected result.
		shouldHaveAccepted := err == nil
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
	ticket, err := harness.CreateTicketPurchaseFromTx(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction: %v", err)
	}

	// Add the ticket outputs as utxos to fake their existence.  Use one after
	// the stake enabled height for the height of the fake utxos to ensure they
	// are mature for the votes cast a stake validation height below.
	harness.chain.SetHeight(harness.chainParams.StakeEnabledHeight + 1)
	harness.chain.utxos.AddTxOuts(ticket, harness.chain.BestHeight(), 0,
		noTreasury)

	// Create enough votes all using the same ticket and voting on different
	// blocks at stake validation height to be able to force rejection due to
	// exceeding the max allowed double spends.
	harness.chain.SetHeight(harness.chainParams.StakeValidationHeight)
	var votes []*dcrutil.Tx
	for i := 0; i < maxVoteDoubleSpends*2; i++ {
		// Ensure each vote is voting on a different block.
		var hash chainhash.Hash
		mockBlock := dcrutil.NewBlock(&wire.MsgBlock{})
		binary.LittleEndian.PutUint32(hash[:4], uint32(i))
		harness.chain.SetBestHash(&hash)
		harness.chain.blocks[hash] = mockBlock

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
			false, true, 0)
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
			false, true, 0)
		if err == nil {
			t.Fatalf("ProcessTransaction: accepted double-spending vote with " +
				"more than max allowed")
		}
		if !errors.Is(err, ErrTooManyVotes) {
			t.Fatalf("Process Transaction: did not get expected " +
				"ErrTooManyVotes error")
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
	harness.txPool.RemoveTransaction(vote, true, noTreasury, noAutoRevocations)
	testPoolMembership(tc, vote, false, false)

	// Add one of the votes that was rejected above due to the pool being at the
	// max allowed and ensure it is accepted now.  Also, ensure it is not in the
	// orphan pool, is in the transaction pool, and is reported as available.
	vote = votes[maxVoteDoubleSpends]
	_, err = harness.txPool.ProcessTransaction(vote, false, false, true, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid vote %v", err)
	}
	testPoolMembership(tc, vote, false, true)

	// Attempt to add another one of the votes and ensure it is rejected due to
	// exceeding the max again.  Also, ensure it is not in the orphan pool, not
	// in the transaction pool, and not reported as available.
	vote = votes[maxVoteDoubleSpends+1]
	_, err = harness.txPool.ProcessTransaction(vote, false, false, true, 0)
	if !errors.Is(err, ErrTooManyVotes) {
		t.Fatalf("Process Transaction: did not get expected " +
			"ErrTooManyVotes error")
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
	ticket, err := harness.CreateTicketPurchaseFromTx(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction: %v", err)
	}

	// Add the ticket outputs as utxos to fake their existence.  Use one after
	// the stake enabled height for the height of the fake utxos to ensure they
	// are matured for the votes cast a stake validation height below.
	harness.chain.SetHeight(harness.chainParams.StakeEnabledHeight + 1)
	harness.chain.utxos.AddTxOuts(ticket, harness.chain.BestHeight(), 0,
		noTreasury)

	// Create a vote that votes on a block at stake validation height.
	hash := chainhash.Hash{0x5c, 0xa1, 0xab, 0x1e}
	mockBlock := dcrutil.NewBlock(&wire.MsgBlock{})
	harness.chain.SetBestHash(&hash)
	harness.chain.SetHeight(harness.chainParams.StakeValidationHeight)
	harness.chain.blocks[hash] = mockBlock
	vote, err := harness.CreateVote(ticket)
	if err != nil {
		t.Fatalf("unable to create vote: %v", err)
	}

	// Add the vote and ensure it is not in the orphan pool, is in the
	// transaction pool, and is reported as available.
	_, err = harness.txPool.ProcessTransaction(vote, false, false, true, 0)
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
	_, err = harness.txPool.ProcessTransaction(dupVote, false, false, true, 0)
	if !errors.Is(err, ErrAlreadyVoted) {
		t.Fatalf("Process Transaction: did not get expected " +
			"ErrTooManyVotes error")
	}
	testPoolMembership(tc, dupVote, false, false)

	// Remove the original vote from the pool and ensure it is not in the orphan
	// pool, not in the transaction pool, and not reported as available.
	harness.txPool.RemoveTransaction(vote, true, noTreasury, noAutoRevocations)
	testPoolMembership(tc, vote, false, false)

	// Add the duplicate vote which should now be accepted.  Also, ensure it is
	// not in the orphan pool, is in the transaction pool, and is reported as
	// available.
	_, err = harness.txPool.ProcessTransaction(dupVote, false, false, true, 0)
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
	_, err = harness.txPool.ProcessTransaction(tx, true, false, true, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept initial tx: %v", err)
	}
	testPoolMembership(tc, tx, false, true)

	// Ensure a second attempt to process the tx is rejected with the correct
	// error and that the transaction remains in the pool.
	_, err = harness.txPool.ProcessTransaction(tx, true, false, true, 0)
	if !errors.Is(err, ErrDuplicate) {
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
	_, err = harness.txPool.ProcessTransaction(orphan, true, false, true, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept orphan tx: %v", err)
	}
	testPoolMembership(tc, orphan, true, false)

	// The second call should fail with the expected ErrDuplicate error.
	_, err = harness.txPool.ProcessTransaction(orphan, true, false, true, 0)
	if !errors.Is(err, ErrDuplicate) {
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
	_, err = harness.txPool.ProcessTransaction(tx, true, false, true, 0)
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

	// Ensure a second attempt to process the tx is rejected with the correct
	// error, that the original transaction remains in the pool and the double
	// spend is not added to the pool.
	_, err = harness.txPool.ProcessTransaction(doubleSpendTx, true, false, true,
		0)
	if !errors.Is(err, ErrMempoolDoubleSpend) {
		t.Fatalf("ProcessTransaction: did not get expected ErrMempoolDoubleSpend")
	}
	testPoolMembership(tc, tx, false, true)
	testPoolMembership(tc, doubleSpendTx, false, false)
}

// TestFetchTransaction ensures that a ticket which spends an output in the
// mempool is returned by FetchTransaction.
func TestFetchTransaction(t *testing.T) {
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
	_, err = harness.txPool.ProcessTransaction(tx, true, false, true, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept initial tx: %v", err)
	}

	// Create a ticket purchase transaction spending the outputs of the
	// prior regular transaction.
	ticket, err := harness.CreateTicketPurchaseFromTx(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction %v", err)
	}

	// Ensure the ticket purchase is accepted into the stage pool.
	_, err = harness.txPool.ProcessTransaction(ticket, true,
		false, true, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid ticket %v", err)
	}

	// ticket should have been accepted but not exist in orphan or main pool.
	testPoolMembership(tc, ticket, false, false)

	// FetchTransaction should still find the ticket, despite it not
	// existing in either pool.
	foundTx, err := harness.txPool.FetchTransaction(ticket.Hash())
	if err != nil {
		t.Fatalf("FetchTransaction: failed to retrieve tx: %v", err)
	}

	if *ticket.Hash() != *foundTx.Hash() {
		t.Fatalf("FetchTransaction: expected ticket %v "+
			"but got %v", ticket.Hash(), foundTx.Hash())
	}
}

// TestRemoveDoubleSpends verifies that a ticket in the stage pool that has a
// double-spent input due to a reorg is removed from the stage pool.
func TestRemoveDoubleSpends(t *testing.T) {
	t.Parallel()

	harness, spendableOuts, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a regular transaction from the first spendable output provided by
	// the harness.
	baseTx, err := harness.CreateTx(spendableOuts[0])
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// Ensure the transaction is accepted to the pool.
	_, err = harness.txPool.ProcessTransaction(baseTx, true, false, true, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept initial tx: %v", err)
	}

	// Create a regular transaction that spends from an input in the
	// baseTx and add it to the mempool.
	baseTxOut := txOutToSpendableOut(baseTx, 0, wire.TxTreeRegular)
	doubleSpendTx, err := harness.CreateTx(baseTxOut)
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// Create a ticket purchase transaction spending the outputs of the
	// base regular transaction.
	ticket, err := harness.CreateTicketPurchaseFromTx(baseTx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction %v", err)
	}

	// Ensure the ticket purchase is accepted as staged.
	_, err = harness.txPool.ProcessTransaction(ticket, true,
		false, true, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid ticket %v", err)
	}

	testPoolMembership(tc, ticket, false, false)

	// FetchTransaction should find the ticket, despite it not
	// existing in either the main or orphan pool.
	_, err = harness.txPool.FetchTransaction(ticket.Hash())
	if err != nil {
		t.Fatalf("FetchTransaction: failed to retrieve tx: %v", err)
	}

	// If a staged transaction double-spends an input due to a reorg,
	// it should be removed from the stage pool.
	tc.harness.txPool.RemoveDoubleSpends(doubleSpendTx, noTreasury,
		noAutoRevocations)

	// FetchTransaction should not be able to retrieve the ticket anymore.
	_, err = harness.txPool.FetchTransaction(ticket.Hash())
	if err == nil {
		t.Fatalf("FetchTransaction: expected dependent transaction %v to not "+
			"exist in pool.", ticket.Hash())
	}
}

// createTSpend creates a treasury spend transaction given the specified
// parameters. A single output is created that pays to a test OP_TRUE P2SH
// script.
func createTSpend(t *testing.T, expiry uint32, tspendAmount, tspendFee int64, piKey []byte) *wire.MsgTx {
	t.Helper()

	msgTx := wire.NewMsgTx()
	msgTx.Version = wire.TxVersionTreasury
	msgTx.Expiry = expiry

	valueIn := tspendAmount + tspendFee

	var opRetScript [1 + 1 + 32]byte
	opRetScript[0] = txscript.OP_RETURN
	opRetScript[1] = txscript.OP_DATA_32
	binary.LittleEndian.PutUint64(opRetScript[2:], uint64(valueIn))
	binary.LittleEndian.PutUint64(opRetScript[25:], uint64(rand.Int63())) // Ensure unique hash.
	msgTx.AddTxOut(wire.NewTxOut(0, opRetScript[:]))

	p2shOpTrueScript, err := hex.DecodeString("a914f5a8302ee8695bf836258b8f2b57b38a0be14e4787")
	if err != nil {
		t.Fatalf("unable to decode p2shOpTrueScript: %v", err)
	}
	script := make([]byte, len(p2shOpTrueScript)+1)
	script[0] = txscript.OP_TGEN
	copy(script[1:], p2shOpTrueScript)
	msgTx.AddTxOut(wire.NewTxOut(tspendAmount, script))

	msgTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         valueIn,
		BlockHeight:     wire.NullBlockHeight,
		BlockIndex:      wire.NullBlockIndex,
		SignatureScript: nil,
	})

	// Calculate TSpend signature without SigHashType.
	sigscript, err := sign.TSpendSignatureScript(msgTx, piKey)
	if err != nil {
		t.Fatalf("unable to sign tspend: %v", err)
	}
	msgTx.TxIn[0].SignatureScript = sigscript

	return msgTx
}

// TestHandlesTSpends verifies that the mempool correctly adds and removes
// valid tspends and limits their total number according to the appropriate
// limits.
func TestHandlesTSpends(t *testing.T) {
	t.Parallel()

	// Pi private key to use when signing tspends.
	piKey, err := hex.DecodeString("62deae1ab2b1ebd96a28c80e870aee325bed359e83d8db2464ef999e616a9eef")
	if err != nil {
		t.Fatal(err)
	}
	piPubKey := secp256k1.PrivKeyFromBytes(piKey).PubKey().SerializeCompressed()

	// Use the mainnet parameters but replace the Pi key to a simnet one so
	// we can sign the tspends.
	net := chaincfg.MainNetParams()
	net.PiKeys = [][]byte{piPubKey, piPubKey}
	harness, _, err := newPoolHarness(net)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Useful constants.
	tvi := net.TreasuryVoteInterval
	mul := net.TreasuryVoteIntervalMultiplier

	// Setup the harness for the test and activate the treasury agenda and
	// set the chain height to an appropriate height to add tspends.
	harness.SetTreasuryAgendaActive(true)
	nextHeight := net.StakeValidationHeight + int64(tvi) - net.StakeValidationHeight%int64(tvi)
	harness.chain.SetHeight(nextHeight - 1)

	// Helper to assert that the TSpendHashes() function returns the
	// correct tspends.
	assertTSpendHashes := func(tspends []*dcrutil.Tx) {
		t.Helper()
		tspendHashes := harness.txPool.TSpendHashes()
		tspendHashesMap := make(map[chainhash.Hash]struct{}, len(tspendHashes))
		for i := 0; i < len(tspendHashes); i++ {
			tspendHashesMap[tspendHashes[i]] = struct{}{}
		}
		for i := 0; i < len(tspends); i++ {
			h := tspends[i].MsgTx().TxHash()
			if _, ok := tspendHashesMap[h]; !ok {
				t.Fatalf("added tspend %d was not returned in TSpendHashes()", i)
			}
			delete(tspendHashesMap, h)
		}
		if len(tspendHashesMap) > 0 {
			t.Fatalf("TSpendHashes() returned extraneous data: %v", tspendHashesMap)
		}
	}

	// Helper that adds and asserts the given tspend was added to the
	// mempool.
	acceptTSpend := func(tx *dcrutil.Tx) {
		t.Helper()
		_, err = harness.txPool.ProcessTransaction(tx, true,
			false, true, 0)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid tspend %v", err)
		}
		testPoolMembership(tc, tx, false, true)
	}

	// Helper that attempts to add and asserts the given tspend is rejected
	// with the given error.
	rejectTSpend := func(tx *dcrutil.Tx, wantErr error) {
		t.Helper()
		_, err = harness.txPool.ProcessTransaction(tx, true, false, true, 0)
		if !errors.Is(err, wantErr) {
			t.Fatalf("Unexpected error while processing rejected tspend. "+
				"want %v, got %#v", wantErr, err)
		}
		testPoolMembership(tc, tx, false, false)
	}

	// Calculate an expiry for the tests such that voting starts at the
	// next block (which happens to be SVH).
	expiry := standalone.CalcTSpendExpiry(nextHeight-int64(tvi), tvi, mul)
	tspendAmount := int64(1e8)
	tspendFee := int64(2550)

	// Create a few valid tspends that can enter the mempool. We'll create
	// one more than the maximum allowed to test for the correct limit.
	const maxTSpends = 7
	const nbTSpends = maxTSpends + 1
	tspends := make([]*dcrutil.Tx, 0, nbTSpends)
	for i := 0; i < nbTSpends; i++ {
		msgTx := createTSpend(t, expiry, tspendAmount, tspendFee, piKey)
		tspends = append(tspends, dcrutil.NewTx(msgTx))
	}

	// Before adding any tspends, TSpendHashes() should not have any
	// hashes.
	assertTSpendHashes(nil)

	// Add the maximum amount of tspends the mempool can hold.
	for _, tx := range tspends[:maxTSpends] {
		acceptTSpend(tx)
	}

	// TSpendHashes() should return the hashes of all tspends so far.
	assertTSpendHashes(tspends[:maxTSpends])

	// The next tspend should be rejected due to exceeding the number of
	// maximum tspends in the mempool.
	rejectTSpend(tspends[maxTSpends], ErrTooManyTSpends)

	// Remove the first tspend from the mempool and assert TSpendHashes()
	// is working as intended.
	harness.txPool.RemoveTransaction(tspends[0], true, true, noAutoRevocations)
	testPoolMembership(tc, tspends[0], false, false)
	assertTSpendHashes(tspends[1:maxTSpends])

	// Add the new tspend.
	acceptTSpend(tspends[maxTSpends])
	assertTSpendHashes(tspends[1 : maxTSpends+1])

	// Remove all tspends from the mempool and ensure TSpendHashes() is
	// empty again.
	for _, tx := range tspends[1 : maxTSpends+1] {
		harness.txPool.RemoveTransaction(tx, true, true, noAutoRevocations)
		testPoolMembership(tc, tx, false, false)
	}
	assertTSpendHashes(nil)

	// Attempt to add a tspend that was already mined on an ancestor block.
	// This should fail.
	harness.chain.SetTSpendMinedOnAncestor(tspends[0].MsgTx().TxHash(), true)
	rejectTSpend(tspends[0], ErrTSpendMinedOnAncestor)

	// Attempt to add a tspend with an incorrect expiry (not tvi+2). This
	// should fail.
	tx := tspends[1].MsgTx()
	tx.Expiry++
	tx.TxIn[0].SignatureScript, err = sign.TSpendSignatureScript(tx, piKey)
	if err != nil {
		t.Fatal(err)
	}
	rejectTSpend(tspends[1], ErrTSpendInvalidExpiry)

	// Attempt to add a tspend with an expiry in the past. This should
	// fail.
	tx = tspends[1].MsgTx()
	tx.Expiry = uint32(tvi)
	tx.TxIn[0].SignatureScript, err = sign.TSpendSignatureScript(tx, piKey)
	if err != nil {
		t.Fatal(err)
	}
	rejectTSpend(tspends[1], ErrExpired)

	// Attempt to add a tspend with an expiry in the distant future. This
	// should fail. Mempool considers a tspend in the "distant future" if
	// the block height where it's supposed to start voting is greater than
	// twice the total voting interval. To generate such a height we just
	// add two tvi*mul interval to the current expiry (which starts voting
	// in the next block).
	tx = tspends[1].MsgTx()
	tx.Expiry = expiry + uint32(tvi*mul*2)
	tx.TxIn[0].SignatureScript, err = sign.TSpendSignatureScript(tx, piKey)
	if err != nil {
		t.Fatal(err)
	}
	rejectTSpend(tspends[1], ErrTSpendInvalidExpiry)

	// Attempt to add a tspend with an invalid signature. This should fail.
	// Since tspends have sigscripts with a fixed format, we reach in and
	// break the signature directly.
	tx = tspends[2].MsgTx()
	tx.TxIn[0].SignatureScript[1] = ^tx.TxIn[0].SignatureScript[1]
	rejectTSpend(tspends[2], blockchain.ErrInvalidPiSignature)

	// Attempt to add a tspend with a valid signature not from a pi key for
	// the current network. This should fail.
	nonPiKey, err := hex.DecodeString("ffff1ab2b1ebd96a28c80e870aee325bed359e83d8db2464ef999e616a0000")
	if err != nil {
		t.Fatal(err)
	}
	tx = tspends[3].MsgTx()
	tx.TxIn[0].SignatureScript, err = sign.TSpendSignatureScript(tx, nonPiKey)
	if err != nil {
		t.Fatal(err)
	}
	rejectTSpend(tspends[3], blockchain.ErrUnknownPiKey)

	// Assert the OnTSpendReceived listener is called when a tspend enters
	// the mempool.
	var callbackReceived bool
	tspendToAdd := tspends[4]
	harness.txPool.cfg.OnTSpendReceived = func(tx *dcrutil.Tx) {
		callbackReceived = true
		if tx.MsgTx().TxHash() != tspendToAdd.MsgTx().TxHash() {
			t.Fatalf("Received wrong tx in callback. want=%s got=%s",
				tspendToAdd.MsgTx().TxHash(), tx.MsgTx().TxHash())
		}
	}
	acceptTSpend(tspends[4])
	if !callbackReceived {
		t.Fatalf("OnTSpendReceived callback was not called")
	}
	harness.txPool.cfg.OnTSpendReceived = nil

	// Assert the tspend can enter the mempool up until the last block it
	// can be mined. Given we know the expiry for the tspend, figure out
	// when voting ends and advance the fake chain to just before that
	// height. The tspend can be mined on the block the vote ends, which is
	// a TVI block.
	_, endVote, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}
	harness.chain.SetHeight(int64(endVote - 1))
	acceptTSpend(tspends[5])
}

// createTAdd creates a treasury add transaction spending from the given
// harness output and sending back any outstanding change to the given address.
func createTAdd(t *testing.T, spend *spendableOutput, payScript, signKey []byte,
	amount, fee dcrutil.Amount, changeAddr stdaddr.StakeAddress) *wire.MsgTx {
	t.Helper()

	// Calculate change and generate script to deliver it.
	var changeScriptVer uint16
	var changeScript []byte
	change := spend.amount - amount - fee
	if change < 0 {
		t.Fatalf("negative change %v", change)
	}
	if change > 0 {
		changeScriptVer, changeScript = changeAddr.StakeChangeScript()
	}

	// Generate and return the transaction spending from the provided
	// spendable output with the previously described outputs.
	tx := wire.NewMsgTx()
	tx.Version = wire.TxVersionTreasury
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: spend.outPoint,
		Sequence:         wire.MaxTxInSequenceNum,
		ValueIn:          int64(spend.amount),
	})
	tx.AddTxOut(wire.NewTxOut(int64(amount), []byte{txscript.OP_TADD}))
	if len(changeScript) > 0 {
		tx.AddTxOut(newTxOut(int64(change), changeScriptVer, changeScript))
	}

	var err error
	tx.TxIn[0].SignatureScript, err = sign.SignatureScript(tx, 0, payScript,
		txscript.SigHashAll, signKey, dcrec.STEcdsaSecp256k1, true)
	if err != nil {
		t.Fatalf("Unable to sign tadd: %v", err)
	}

	return tx
}

// TestHandlesTAdds verifies that the mempool correctly adds and removes valid
// tadds.
func TestHandlesTAdds(t *testing.T) {
	t.Parallel()

	net := chaincfg.MainNetParams()
	harness, outs, err := newPoolHarness(net)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Setup the harness for the test and activate the treasury agenda.
	harness.SetTreasuryAgendaActive(true)

	// Addresses to use in the following TAdds.
	addPrivKey := secp256k1.NewPrivateKey(new(secp256k1.ModNScalar).SetInt(2))
	pubKey := addPrivKey.PubKey().SerializeCompressed()
	pubKeyHash := stdaddr.Hash160(pubKey)
	addP2pkhAddr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(pubKeyHash,
		net)
	if err != nil {
		t.Fatal(err)
	}
	addP2shScript := []byte{txscript.OP_NOP, txscript.OP_NOP, txscript.OP_TRUE}
	addP2shAddr, err := stdaddr.NewAddressScriptHashV0(addP2shScript, net)
	if err != nil {
		t.Fatal(err)
	}

	// Helper to create tadds for this test without having to keep passing
	// repeated parameters.
	createTAdd := func(spend *spendableOutput, amount, fee dcrutil.Amount,
		changeAddr stdaddr.StakeAddress) *dcrutil.Tx {
		t.Helper()
		return dcrutil.NewTx(createTAdd(t, spend, harness.payScript,
			harness.signKey, amount, fee, changeAddr))
	}

	// Helper that adds and asserts the given tadd was added to the
	// mempool.
	//
	// Note that this automatically removes the tadd from the mempool after
	// asserting it was correctly added to allow using the same outputs to
	// create a new one.
	acceptTAdd := func(tx *dcrutil.Tx) {
		t.Helper()
		_, err = harness.txPool.ProcessTransaction(tx, true,
			false, true, 0)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid tadd %v", err)
		}
		testPoolMembership(tc, tx, false, true)
		harness.txPool.RemoveTransaction(tx, true, true, noAutoRevocations)
	}

	// Create a few valid tadds that can enter the mempool. Generate a TAdd
	// for some amount ensuring there's change and direct the change to a
	// P2PKH addr.
	taddAmount := outs[0].amount / 2
	taddFee := dcrutil.Amount(2550)
	tadd := createTAdd(&outs[0], taddAmount, taddFee, addP2pkhAddr)
	if len(tadd.MsgTx().TxOut) != 2 {
		t.Fatalf("tadd was not created with change: %#v", tadd)
	}
	acceptTAdd(tadd)

	// Create a TAdd with change to a p2sh address.
	tadd = createTAdd(&outs[0], taddAmount, taddFee, addP2shAddr)
	if len(tadd.MsgTx().TxOut) != 2 {
		t.Fatalf("tadd was not created with change: %#v", tadd)
	}
	acceptTAdd(tadd)

	// Create a TAdd that has no change.
	taddAmount = outs[0].amount - taddFee
	tadd = createTAdd(&outs[0], taddAmount, taddFee, addP2shAddr)
	if len(tadd.MsgTx().TxOut) != 1 {
		t.Fatalf("tadd was created with change: %#v", tadd)
	}
	acceptTAdd(tadd)
}

// TestStagedTransactionHeight verifies that the height of a transaction
// that moves from the stage pool into the main pool is set to the height it
// was initially added to the mempool, rather than the height it was unstaged.
func TestStagedTransactionHeight(t *testing.T) {
	harness, spendableOuts, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}

	txA, _ := harness.CreateSignedTx([]spendableOutput{
		spendableOuts[0],
	}, 1)

	ticket, err := harness.CreateTicketPurchaseFromTx(txA, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction %v", err)
	}

	allTxns := []*dcrutil.Tx{txA, ticket}
	for index, tx := range allTxns {
		_, err := harness.txPool.ProcessTransaction(tx,
			true, false, true, 0)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid "+
				"transaction at index %d: %v", index, err)
		}
	}

	initialBlockHeight := harness.chain.BestHeight()
	poolTxDescs := harness.txPool.TxDescs()
	if len(poolTxDescs) != 1 {
		t.Fatalf("expected to find exactly one transaction in the mempool but "+
			"got %v", len(poolTxDescs))
	}

	poolTxA := poolTxDescs[0]
	if poolTxA.Height != initialBlockHeight {
		t.Fatalf("expected txA mempool height to be %v but got %v",
			initialBlockHeight, poolTxA.Height)
	}

	// Remove txA, which should bring the ticket out of the stage pool and
	// into the main pool.
	newBlockHeight := initialBlockHeight + 1
	harness.AddFakeUTXO(txA, newBlockHeight, wire.NullBlockIndex)
	harness.chain.SetHeight(newBlockHeight)
	harness.txPool.RemoveTransaction(txA, false, noTreasury, noAutoRevocations)
	harness.txPool.MaybeAcceptDependents(txA, noTreasury)

	poolTxDescs = harness.txPool.TxDescs()
	if len(poolTxDescs) != 1 {
		t.Fatalf("expected to find exactly one transaction in the mempool but "+
			"got %v", len(poolTxDescs))
	}

	poolTransaction := poolTxDescs[0]
	if *poolTransaction.Tx.Hash() != *ticket.Hash() {
		t.Fatalf("expected to find ticket %v in the mempool but got %v",
			ticket.Hash(), poolTransaction.Tx.Hash())
	}

	if poolTransaction.Height != initialBlockHeight {
		t.Fatalf("expected ticket mempool height to be %v but got %v",
			initialBlockHeight, poolTransaction.Height)
	}
}

// TestExplicitVersionSemantics ensures the mempool has the following semantics
// in regards to transaction and script versions:
//
// - Rejects new regular and stake txns with an unsupported tx version
// - Rejects new regular and stake txns with an output that has an unsupported
//   script version
// - Accepts new txns that spend an existing regular tx output that has a newer
//   script version that is no longer allowed for new outputs (until/unless
//   explicitly enabled via a future consensus vote)
func TestExplicitVersionSemantics(t *testing.T) {
	t.Parallel()

	// Create a new harness that accepts non-standard transactions.
	harness, outputs, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	txPool := harness.txPool
	txPool.cfg.Policy.AcceptNonStd = true
	tc := &testContext{t, harness}

	// Ensure a regular transaction with the version set to the max value is
	// rejected.  Then verify it is not in the orphan pool, not in the
	// transaction pool, and not reported as available.
	maxVerTx, err := harness.CreateSignedTx([]spendableOutput{outputs[0]}, 1,
		func(tx *wire.MsgTx) {
			tx.Version = ^uint16(0)
		})
	if err != nil {
		t.Fatalf("unable to create signed tx: %v", err)
	}
	_, err = txPool.ProcessTransaction(maxVerTx, false, false, true, 0)
	if !errors.Is(err, blockchain.ErrTxVersionTooHigh) {
		t.Fatalf("did not receive expected err -- got %[1]v (%[1]T)", err)
	}
	testPoolMembership(tc, maxVerTx, false, false)

	// Ensure a stake transaction (ticket purchase) with the version set to the
	// max value is rejected.  Then verify it is not in the orphan pool, not in
	// the transaction pool, and not reported as available.
	ticket, err := harness.CreateTicketPurchase(outputs[0], 40000,
		func(tx *wire.MsgTx) {
			tx.Version = ^uint16(0)
		})
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction %v", err)
	}
	_, err = txPool.ProcessTransaction(ticket, false, false, true, 0)
	if !errors.Is(err, blockchain.ErrTxVersionTooHigh) {
		t.Fatalf("did not receive expected err -- got %[1]v (%[1]T)", err)
	}
	testPoolMembership(tc, ticket, false, false)

	// Ensure a regular transaction with the script version for the first output
	// set to the max value is rejected.  Then verify it is not in the orphan
	// pool, not in the transaction pool, and not reported as available.
	maxScrVerTx, err := harness.CreateSignedTx([]spendableOutput{outputs[0]},
		1, func(tx *wire.MsgTx) {
			tx.TxOut[0].Version = ^uint16(0)
		})
	if err != nil {
		t.Fatalf("unable to create signed tx: %v", err)
	}
	_, err = txPool.ProcessTransaction(maxScrVerTx, false, false, true, 0)
	if !errors.Is(err, blockchain.ErrScriptVersionTooHigh) {
		t.Fatalf("did not receive expected err -- got %[1]v (%[1]T)", err)
	}
	testPoolMembership(tc, maxScrVerTx, false, false)

	// Ensure a stake transaction (ticket purchase) with the script version of
	// an output set to the max value is rejected.  Then verify it is not in the
	// orphan pool, not in the transaction pool, and not reported as available.
	//
	// Note that newer script versions for stake transactions were already
	// disallowed prior to the explicit version upgrades consensus vote, so this
	// just asserts that behavior has not changed.
	ticket2, err := harness.CreateTicketPurchase(outputs[0], 40000,
		func(tx *wire.MsgTx) {
			tx.TxOut[0].Version = ^uint16(0)
		})
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction %v", err)
	}
	_, err = txPool.ProcessTransaction(ticket2, false, false, true, 0)
	if !errors.Is(err, blockchain.ErrScriptVersionTooHigh) {
		t.Fatalf("did not receive expected err -- got %[1]v (%[1]T)", err)
	}
	testPoolMembership(tc, ticket2, false, false)

	// Create and add a mock regular tree utxo with a script version set to the
	// max value so the code below can spend it to prove existing utxos created
	// with script versions that are no longer valid for new outputs remain
	// spendable.
	mockInputMsgTx := wire.NewMsgTx()
	mockInputMsgTx.AddTxOut(&wire.TxOut{
		Value:    1000000000,
		Version:  ^uint16(0),
		PkScript: []byte{txscript.OP_FALSE},
	})
	mockInputTx := dcrutil.NewTx(mockInputMsgTx)
	harness.AddFakeUTXO(mockInputTx, harness.chain.BestHeight(),
		wire.NullBlockIndex)

	// Ensure spending an existing regular transaction output that has a newer
	// script version that is no longer valid for new outputs (until explicitly
	// enabled in the future) is accepted.  Then verify it is not in the orphan
	// pool, is in the transaction pool, and is reported as available.
	spendableOut := txOutToSpendableOut(mockInputTx, 0, wire.TxTreeRegular)
	spendMaxScrVerTx, err := harness.CreateTx(spendableOut)
	if err != nil {
		t.Fatalf("unable to create signed tx: %v", err)
	}
	_, err = txPool.ProcessTransaction(spendMaxScrVerTx, false, false, true, 0)
	if err != nil {
		t.Fatalf("unexpected err: %[1]v (%[1]T)", err)
	}
	testPoolMembership(tc, spendMaxScrVerTx, false, true)

	// NOTE: No attempt to spend a stake output with a script version too high
	// is made because those have never been permitted by consensus.
}

// TestRevocationsWithAutoRevocationsEnabled validates that revocations are
// accepted and rejected by the mempool as expected with the automatic
// ticket revocations agenda enabled.
func TestRevocationsWithAutoRevocationsEnabled(t *testing.T) {
	t.Parallel()

	harness, spendableOuts, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Enable automatic ticket revocations.
	harness.autoRevocationsActive = true

	// Create a regular transaction from the first spendable output provided by
	// the harness.
	tx, err := harness.CreateTx(spendableOuts[0])
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// Create a ticket purchase transaction spending the outputs of the prior
	// regular transaction.
	ticket, err := harness.CreateTicketPurchaseFromTx(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction: %v", err)
	}

	harness.chain.SetHeight(harness.chainParams.StakeValidationHeight + 1)

	// Ensure the regular tx whose output will be spent by the ticket is accepted.
	_, err = harness.txPool.ProcessTransaction(tx, false, false, true, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)

	// Ensure the ticket is accepted.
	_, err = harness.txPool.ProcessTransaction(ticket, false, false, true, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, ticket, false, false)

	// Add a fake UTXO for the ticket.
	harness.AddFakeUTXO(ticket, int64(ticket.MsgTx().TxIn[0].BlockHeight),
		wire.NullBlockIndex)

	// Create a revocation from the ticket with a non-zero fee.
	zeroFee := dcrutil.Amount(0)
	revocation, err := harness.CreateRevocation(ticket, zeroFee)
	revocation.MsgTx().TxOut[0].Value--
	if err != nil {
		t.Fatalf("unable to create revocation: %v", err)
	}

	// The revocation should be rejected if the transaction does not have zero
	// fee.
	_, err = harness.txPool.ProcessTransaction(revocation, false, false, true,
		0)
	if !errors.Is(err, blockchain.ErrRegTxCreateStakeOut) {
		t.Fatalf("mismatched err -- got %T, want %T", err,
			blockchain.ErrRegTxCreateStakeOut)
	}

	// Create a revocation from the ticket with zero-fee.
	revocation, err = harness.CreateRevocation(ticket, zeroFee)
	if err != nil {
		t.Fatalf("unable to create revocation: %v", err)
	}

	// The revocation should be rejected if the transaction is not version 2.
	revocation.SetVersion(1)
	_, err = harness.txPool.ProcessTransaction(revocation, false, false, true,
		0)
	if !errors.Is(err, blockchain.ErrInvalidRevocationTxVersion) {
		t.Fatalf("mismatched err -- got %T, want %T", err,
			blockchain.ErrInvalidRevocationTxVersion)
	}

	// The revocation should be accepted when it is version 2 with zero fee.
	revocation.SetVersion(stake.TxVersionAutoRevocations)
	_, err = harness.txPool.ProcessTransaction(revocation, false, false, true,
		0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept orphan transaction %v",
			err)
	}
	testPoolMembership(tc, revocation, false, true)

	// The revocation should be removed when a new block is processed.
	nextStakeDiff, err := harness.chain.NextStakeDifficulty()
	if err != nil {
		t.Fatalf("unable to retrieve next stake difficulty: %v", err)
	}
	harness.txPool.PruneStakeTx(nextStakeDiff, harness.chain.BestHeight()+1)
	testPoolMembership(tc, revocation, false, false)
}

// TestFraudProofHandling validates that the fraud proof of transaction inputs
// is corrected as necessary when entering the mempool under a variety of
// conditions.
func TestFraudProofHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		incorrectTxInAmount      bool
		incorrectTxInBlockHeight bool
		incorrectTxInBlockIndex  bool
	}{{
		name:                     "correct fraud proof",
		incorrectTxInAmount:      false,
		incorrectTxInBlockHeight: false,
		incorrectTxInBlockIndex:  false,
	}, {
		name:                     "incorrect amount",
		incorrectTxInAmount:      true,
		incorrectTxInBlockHeight: false,
		incorrectTxInBlockIndex:  false,
	}, {
		name:                     "incorrect block height",
		incorrectTxInAmount:      false,
		incorrectTxInBlockHeight: true,
		incorrectTxInBlockIndex:  false,
	}, {
		name:                     "incorrect block index",
		incorrectTxInAmount:      false,
		incorrectTxInBlockHeight: false,
		incorrectTxInBlockIndex:  true,
	}, {
		name:                     "incorrect amount and block height",
		incorrectTxInAmount:      true,
		incorrectTxInBlockHeight: true,
		incorrectTxInBlockIndex:  false,
	}, {
		name:                     "incorrect amount and block index",
		incorrectTxInAmount:      true,
		incorrectTxInBlockHeight: false,
		incorrectTxInBlockIndex:  true,
	}, {
		name:                     "incorrect block height and block index",
		incorrectTxInAmount:      false,
		incorrectTxInBlockHeight: true,
		incorrectTxInBlockIndex:  true,
	}, {
		name:                     "incorrect amount, block height, and block index",
		incorrectTxInAmount:      true,
		incorrectTxInBlockHeight: true,
		incorrectTxInBlockIndex:  true,
	}}

	for _, test := range tests {
		harness, spendableOuts, err := newPoolHarness(chaincfg.MainNetParams())
		if err != nil {
			t.Fatalf("%q: unable to create test pool: %v", test.name, err)
		}
		tc := &testContext{t, harness}

		// Create a regular transaction from the first spendable output provided by
		// the harness.
		baseTx, err := harness.CreateTx(spendableOuts[0])
		if err != nil {
			t.Fatalf("%q: unable to create base transaction: %v", test.name, err)
		}

		// Ensure that the transaction is accepted to the pool.
		_, err = harness.txPool.ProcessTransaction(baseTx, true, false, true, 0)
		if err != nil {
			t.Fatalf("%q: failed to process base transaction: %v", test.name, err)
		}

		// Add a fake UTXO for the output of the base transaction.
		utxoAmount := baseTx.MsgTx().TxOut[0].Value
		utxoBlockHeight := harness.chain.BestHeight()
		utxoBlockIndex := uint32(0)
		harness.AddFakeUTXO(baseTx, utxoBlockHeight, utxoBlockIndex)

		// Create a regular transaction that spends an output of the base
		// transaction.
		baseTxOut := txOutToSpendableOut(baseTx, 0, wire.TxTreeRegular)
		tx, err := harness.CreateTx(baseTxOut)
		if err != nil {
			t.Fatalf("%q: unable to create transaction: %v", test.name, err)
		}

		// Set the fraud proof data on the transaction input.  Optionally set some
		// of the fields incorrectly based on the test parameters.
		txIn := tx.MsgTx().TxIn[0]
		txIn.ValueIn = utxoAmount
		if test.incorrectTxInAmount {
			txIn.ValueIn++
		}
		txIn.BlockHeight = uint32(utxoBlockHeight)
		if test.incorrectTxInBlockHeight {
			txIn.BlockHeight++
		}
		txIn.BlockIndex = utxoBlockIndex
		if test.incorrectTxInBlockIndex {
			txIn.BlockIndex++
		}

		// Ensure the regular tx is accepted.
		_, err = harness.txPool.ProcessTransaction(tx, false, false, true, 0)
		if err != nil {
			t.Fatalf("%q: ProcessTransaction: failed to accept valid transaction %v",
				test.name, err)
		}
		testPoolMembership(tc, tx, false, true)

		// Validate that the transaction can be fetched from the mempool by its
		// hash.
		txFromMempool, err := harness.txPool.FetchTransaction(tx.Hash())
		if err != nil {
			t.Fatalf("%q: unable to fetch transaction: %v", test.name, err)
		}

		// Validate that the transaction input amount is correct.
		txInFromMempool := txFromMempool.MsgTx().TxIn[0]
		if txInFromMempool.ValueIn != utxoAmount {
			t.Errorf("%q: unexpected amount -- got %v, want %v", test.name,
				txInFromMempool.ValueIn, utxoAmount)
		}

		// Validate that the transaction input block height is correct.
		if int64(txInFromMempool.BlockHeight) != utxoBlockHeight {
			t.Errorf("%q: unexpected block height -- got %v, want %v", test.name,
				txInFromMempool.BlockHeight, utxoBlockHeight)
		}

		// Validate that the transaction input block index is correct.
		if int64(txInFromMempool.BlockIndex) != int64(utxoBlockIndex) {
			t.Errorf("%q: unexpected block index -- got %v, want %v", test.name,
				txInFromMempool.BlockIndex, utxoBlockIndex)
		}
	}
}

// TestSubsidySplitSemantics ensures the mempool has the following semantics
// in regards to the modified subsidy split agenda:
//
// - Accepts votes with the original subsidy when the agenda is NOT active
// - Rejects votes with the original subsidy when the agenda is active
// - Accepts votes with the modified subsidy when the agenda is active
// - Rejects votes with the modified subsidy when the agenda is NOT active
func TestSubsidySplitSemantics(t *testing.T) {
	t.Parallel()

	harness, outputs, err := newPoolHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	txPool := harness.txPool
	tc := &testContext{t, harness}

	// Create a regular transaction from the first spendable output provided by
	// the harness.
	tx, err := harness.CreateTx(outputs[0])
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// Create a ticket purchase transaction spending the outputs of the prior
	// regular transaction.
	ticket, err := harness.CreateTicketPurchaseFromTx(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction: %v", err)
	}

	// Add the ticket outputs as utxos to fake their existence.  Use one after
	// the stake enabled height for the height of the fake utxos to ensure they
	// are matured for the votes cast at stake validation height below.
	harness.chain.SetHeight(harness.chainParams.StakeEnabledHeight + 1)
	harness.chain.utxos.AddTxOuts(ticket, harness.chain.BestHeight(), 0,
		noTreasury)

	// Create a vote that votes on a block at stake validation height using the
	// proportions required when the modified subsidy split agenda is NOT active.
	harness.subsidySplitActive = false
	hash := chainhash.Hash{0x5c, 0xa1, 0xab, 0x1e}
	mockBlock := dcrutil.NewBlock(&wire.MsgBlock{})
	harness.chain.SetBestHash(&hash)
	harness.chain.SetHeight(harness.chainParams.StakeValidationHeight)
	harness.chain.blocks[hash] = mockBlock
	preDCP0010Vote, err := harness.CreateVote(ticket)
	if err != nil {
		t.Fatalf("unable to create vote: %v", err)
	}

	// Create another vote that votes on a block at stake validation height
	// using the proportions required when the modified subsidy split agenda is
	// active.
	harness.subsidySplitActive = true
	postDCP0010Vote, err := harness.CreateVote(ticket)
	if err != nil {
		t.Fatalf("unable to create vote: %v", err)
	}

	// Attempt to add the vote with the modified subsidy when the agenda is NOT
	// active and ensure it is rejected.  Also, ensure it is not in the orphan
	// pool, not in the transaction pool, and not reported as available.
	harness.subsidySplitActive = false
	_, err = txPool.ProcessTransaction(postDCP0010Vote, false, false, true, 0)
	if !errors.Is(err, blockchain.ErrBadStakebaseAmountIn) {
		t.Fatal("did not get expected ErrBadStakebaseAmountIn error")
	}
	testPoolMembership(tc, postDCP0010Vote, false, false)

	// Attempt to add the vote with the original subsidy when the agenda is NOT
	// active and ensure it is accepted.  Also, ensure it is not in the orphan
	// pool, is in the transaction pool, and is reported as available.
	_, err = txPool.ProcessTransaction(preDCP0010Vote, false, false, true, 0)
	if err != nil {
		t.Fatalf("failed to accept valid vote %v", err)
	}
	testPoolMembership(tc, preDCP0010Vote, false, true)

	// Remove the vote from the pool and ensure it is not in the orphan pool,
	// not in the transaction pool, and not reported as available.
	harness.txPool.RemoveTransaction(preDCP0010Vote, true, noTreasury,
		noAutoRevocations)
	testPoolMembership(tc, preDCP0010Vote, false, false)

	// Attempt to add the vote with the original subsidy when the agenda is
	// active and ensure it is rejected.  Also, ensure it is not in the orphan
	// pool, not in the transaction pool, and not reported as available.
	harness.subsidySplitActive = true
	_, err = txPool.ProcessTransaction(preDCP0010Vote, false, false, true, 0)
	if !errors.Is(err, blockchain.ErrBadStakebaseAmountIn) {
		t.Fatal("did not get expected ErrBadStakebaseAmountIn error")
	}
	testPoolMembership(tc, preDCP0010Vote, false, false)

	// Attempt to add the vote with the modified subsidy when the agenda is
	// active and ensure it is accepted.  Also, ensure it is not in the orphan
	// pool, is in the transaction pool, and is reported as available.
	_, err = txPool.ProcessTransaction(postDCP0010Vote, false, false, true, 0)
	if err != nil {
		t.Fatalf("failed to accept valid vote %v", err)
	}
	testPoolMembership(tc, postDCP0010Vote, false, true)
}
