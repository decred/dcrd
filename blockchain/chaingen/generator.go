// Copyright (c) 2016-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaingen

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"runtime"
	"sort"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/crypto/rand"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/sign"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/txscript/v4/stdscript"
	"github.com/decred/dcrd/wire"
)

var (
	// hash256prngSeedConst is a constant derived from the hex
	// representation of pi and is used in conjunction with a caller-provided
	// seed when initializing the deterministic lottery prng.
	hash256prngSeedConst = []byte{0x24, 0x3f, 0x6a, 0x88, 0x85, 0xa3, 0x08,
		0xd3}

	// opTrueScript is a simple public key script that contains the OP_TRUE
	// opcode.  It is defined here to reduce garbage creation.
	opTrueScript = []byte{txscript.OP_TRUE}

	// opTrueRedeemScript is the signature script that can be used to redeem
	// a p2sh output to the opTrueScript.  It is defined here to reduce
	// garbage creation.
	opTrueRedeemScript = []byte{txscript.OP_DATA_1, txscript.OP_TRUE}

	// coinbaseSigScript is the signature script used by the tests when
	// creating standard coinbase transactions.  It is defined here to
	// reduce garbage creation.
	coinbaseSigScript = []byte{txscript.OP_0, txscript.OP_0}
)

const (
	// voteBitYes is the specific bit that is set in the vote bits to
	// indicate that the previous block is valid.
	voteBitYes = 0x01
)

// SpendableOut represents a transaction output that is spendable along with
// additional metadata such as the block its in and how much it pays.
type SpendableOut struct {
	prevOut     wire.OutPoint
	blockHeight uint32
	blockIndex  uint32
	amount      dcrutil.Amount
}

// PrevOut returns the outpoint associated with the spendable output.
func (s *SpendableOut) PrevOut() wire.OutPoint {
	return s.prevOut
}

// BlockHeight returns the block height of the block the spendable output is in.
func (s *SpendableOut) BlockHeight() uint32 {
	return s.blockHeight
}

// BlockIndex returns the offset into the block the spendable output is in.
func (s *SpendableOut) BlockIndex() uint32 {
	return s.blockIndex
}

// Amount returns the amount associated with the spendable output.
func (s *SpendableOut) Amount() dcrutil.Amount {
	return s.amount
}

// makeSpendableOutForTxInternal returns a spendable output for the given
// transaction block height, transaction index within the block, transaction
// tree and transaction output index within the transaction.
func makeSpendableOutForTxInternal(tx *wire.MsgTx, blockHeight, txIndex, txOutIndex uint32, tree int8) SpendableOut {
	return SpendableOut{
		prevOut: wire.OutPoint{
			Hash:  *tx.CachedTxHash(),
			Index: txOutIndex,
			Tree:  tree,
		},
		blockHeight: blockHeight,
		blockIndex:  txIndex,
		amount:      dcrutil.Amount(tx.TxOut[txOutIndex].Value),
	}
}

// MakeSpendableOutForTx returns a spendable output for a regular transaction.
func MakeSpendableOutForTx(tx *wire.MsgTx, blockHeight, txIndex, txOutIndex uint32) SpendableOut {
	return makeSpendableOutForTxInternal(tx, blockHeight, txIndex, txOutIndex, wire.TxTreeRegular)
}

// MakeSpendableOutForSTx returns a spendable output for a stake transaction.
func MakeSpendableOutForSTx(tx *wire.MsgTx, blockHeight, txIndex, txOutIndex uint32) SpendableOut {
	return makeSpendableOutForTxInternal(tx, blockHeight, txIndex, txOutIndex, wire.TxTreeStake)
}

// MakeSpendableOut returns a spendable output for the given block, transaction
// index within the block, and transaction output index within the transaction.
func MakeSpendableOut(block *wire.MsgBlock, txIndex, txOutIndex uint32) SpendableOut {
	tx := block.Transactions[txIndex]
	return MakeSpendableOutForTx(tx, block.Header.Height, txIndex, txOutIndex)
}

// MakeSpendableStakeOut returns a spendable stake output for the given block,
// transaction index within the block, and transaction output index within the
// transaction.
func MakeSpendableStakeOut(block *wire.MsgBlock, txIndex, txOutIndex uint32) SpendableOut {
	tx := block.STransactions[txIndex]
	return MakeSpendableOutForSTx(tx, block.Header.Height, txIndex, txOutIndex)
}

// stakeTicket represents a transaction that is an sstx along with the height of
// the block it was mined in and the its index within that block.
type stakeTicket struct {
	tx          *wire.MsgTx
	blockHeight uint32
	blockIndex  uint32
}

// stakeTicketSorter implements sort.Interface to allow a slice of stake tickets
// to be sorted.
type stakeTicketSorter []*stakeTicket

// Len returns the number of stake tickets in the slice.  It is part of the
// sort.Interface implementation.
func (t stakeTicketSorter) Len() int { return len(t) }

// Swap swaps the stake tickets at the passed indices.  It is part of the
// sort.Interface implementation.
func (t stakeTicketSorter) Swap(i, j int) { t[i], t[j] = t[j], t[i] }

// Less returns whether the stake ticket with index i should sort before the
// stake ticket with index j.  It is part of the sort.Interface implementation.
func (t stakeTicketSorter) Less(i, j int) bool {
	iHash := t[i].tx.CachedTxHash()[:]
	jHash := t[j].tx.CachedTxHash()[:]
	return bytes.Compare(iHash, jHash) < 0
}

// spendableOutsSnap houses a snapshot of the spendable outputs state.  It is
// useful to allow callers to reset back to known good points.
type spendableOutsSnap struct {
	spendableOuts     [][]SpendableOut
	prevCollectedHash chainhash.Hash
}

// PowHashAlgorithm defines the supported proof of work hash functions the
// generator can use when generating solved blocks.
type PowHashAlgorithm uint8

const (
	// PHABlake256r14 specifies the original blake256 hashing algorithm with 14
	// rounds that was in effect at initial launch.
	//
	// This is the algorithm the generator will use by default.
	PHABlake256r14 PowHashAlgorithm = iota

	// PHABlake3 specifies the blake3 hashing algorithm introduced by DCP0011.
	PHABlake3
)

// PowDifficultyAlgorithm defines the supported proof of work difficulty
// algorithms the generator can use when generating solved blocks.
type PowDifficultyAlgorithm uint8

const (
	// PDAEma specifies the original exponential moving average proof of work
	// difficulty algorithm that was in effect at initial launch.
	PDAEma PowDifficultyAlgorithm = iota

	// PDAAsert specifies the ASERT proof of work difficulty algorithm
	// introduced by DCP0011.
	PDAAsert
)

// TreasurySemantics defines the supported treasury semantics the generator can
// use when generating solved blocks.
type TreasurySemantics uint8

const (
	// TSOriginal specifies the original treasury semantics that were in effect
	// at initial launch.
	TSOriginal TreasurySemantics = iota

	// TSDCP0006 specifies the decentralized treasury semantics introduced by
	// DCP0006.
	TSDCP0006
)

// Generator houses state used to ease the process of generating test blocks
// that build from one another along with housing other useful things such as
// available spendable outputs and generic payment scripts used throughout the
// tests.
type Generator struct {
	params              *chaincfg.Params
	tip                 *wire.MsgBlock
	tipName             string
	blocks              map[chainhash.Hash]*wire.MsgBlock
	blockHeights        map[chainhash.Hash]uint32
	blocksByName        map[string]*wire.MsgBlock
	blockNames          map[chainhash.Hash]string
	p2shOpTrueAddr      stdaddr.StakeAddress
	p2shOpTrueScriptVer uint16
	p2shOpTrueScript    []byte

	// Used for tracking spendable coinbase outputs.
	spendableOuts     [][]SpendableOut
	prevCollectedHash chainhash.Hash
	spendableOutSnaps map[string]spendableOutsSnap

	// Used for tracking the live ticket pool and revocations.
	originalParents map[chainhash.Hash]chainhash.Hash
	immatureTickets []*stakeTicket
	liveTickets     []*stakeTicket
	wonTickets      map[chainhash.Hash][]*stakeTicket
	expiredTickets  []*stakeTicket
	revokedTickets  map[chainhash.Hash][]*stakeTicket
	missedVotes     map[chainhash.Hash]*stakeTicket

	// Used for tracking different behavior depending on voting agendas.
	powHashAlgo   PowHashAlgorithm
	powDiffAlgo   PowDifficultyAlgorithm
	powDiffAnchor *wire.BlockHeader

	// Used for tracking different treasury semantics depending on voting
	// agendas.
	treasurySemantics TreasurySemantics
}

// MakeGenerator returns a generator instance initialized with the genesis block
// as the tip as well as a cached generic pay-to-script-hash script for OP_TRUE.
func MakeGenerator(params *chaincfg.Params) (Generator, error) {
	// Generate a generic pay-to-script-hash script that is a simple
	// OP_TRUE.  This allows the tests to avoid needing to generate and
	// track actual public keys and signatures.
	p2shOpTrueAddr, err := stdaddr.NewAddressScriptHashV0(opTrueScript, params)
	if err != nil {
		return Generator{}, err
	}
	p2shOpTrueScriptVer, p2shOpTrueScript := p2shOpTrueAddr.PaymentScript()

	genesis := params.GenesisBlock
	genesisHash := genesis.BlockHash()
	return Generator{
		params:              params,
		tip:                 genesis,
		tipName:             "genesis",
		blocks:              map[chainhash.Hash]*wire.MsgBlock{genesisHash: genesis},
		blockHeights:        map[chainhash.Hash]uint32{genesisHash: 0},
		blocksByName:        map[string]*wire.MsgBlock{"genesis": genesis},
		blockNames:          map[chainhash.Hash]string{genesisHash: "genesis"},
		spendableOutSnaps:   make(map[string]spendableOutsSnap),
		p2shOpTrueAddr:      p2shOpTrueAddr,
		p2shOpTrueScriptVer: p2shOpTrueScriptVer,
		p2shOpTrueScript:    p2shOpTrueScript,
		originalParents:     make(map[chainhash.Hash]chainhash.Hash),
		wonTickets:          make(map[chainhash.Hash][]*stakeTicket),
		revokedTickets:      make(map[chainhash.Hash][]*stakeTicket),
		missedVotes:         make(map[chainhash.Hash]*stakeTicket),
		powHashAlgo:         PHABlake256r14,
		powDiffAlgo:         PDAEma,
		treasurySemantics:   TSOriginal,
	}, nil
}

// UsePowHashAlgo specifies the proof of work hashing algorithm the generator
// should use when generating blocks.
//
// It will panic if an unsupported algorithm is provided.
func (g *Generator) UsePowHashAlgo(algo PowHashAlgorithm) {
	switch algo {
	case PHABlake256r14:
	case PHABlake3:
	default:
		panic(fmt.Sprintf("unsupported proof of work hash algorithm %d", algo))
	}

	g.powHashAlgo = algo
}

// UsePowDiffAlgo specifies the proof of work difficulty algorithm the generator
// should use when generating blocks.  It will panic if an unsupported algorithm
// is provided.
//
// The second options parameter depends on the specified algorithm as follows:
//
// PDAEma:
//   - No additional options may be specified.  The function will panic if any
//     are specified.
//
// PDAAsert:
//   - The name of the anchor block to use.  The function will panic if no
//     anchor block is specified or the specified block does not exist.
func (g *Generator) UsePowDiffAlgo(algo PowDifficultyAlgorithm, options ...string) {
	switch algo {
	case PDAEma:
		if len(options) != 0 {
			panic("no options may be specified for the EMA proof of work " +
				"difficulty algorithm")
		}
		g.powDiffAlgo = algo
		g.powDiffAnchor = nil
		return

	case PDAAsert:
		if len(options) != 1 {
			panic("an anchor block must be specified for the ASERT proof of " +
				"work difficulty algorithm")
		}

		anchorName := options[0]
		anchor, ok := g.blocksByName[anchorName]
		if !ok {
			panic(fmt.Sprintf("block name %s does not exist", anchorName))
		}
		g.powDiffAlgo = algo
		g.powDiffAnchor = &anchor.Header
		return
	}

	panic(fmt.Sprintf("unsupported proof of work difficulty algorithm %d", algo))
}

// UseTreasurySemantics specifies the treasury semantics the generator should
// use when generating blocks.  It will panic if an unsupported value is
// provided.
//
// The second options parameter depends on the specified semantics as follows:
//
// TSOriginal:
//   - No additional options may be specified.  The function will panic if any
//     are specified.
//
// TSDCP0006:
//   - No additional options may be specified.  The function will panic if any
//     are specified.
func (g *Generator) UseTreasurySemantics(semantics TreasurySemantics, options ...string) {
	switch semantics {
	case TSOriginal:
		if len(options) != 0 {
			panic("no options may be specified for the original treasury " +
				"semantics")
		}
		g.treasurySemantics = semantics
		return

	case TSDCP0006:
		if len(options) != 0 {
			panic("no options may be specified for the DCP0006 treasury " +
				"semantics")
		}
		g.treasurySemantics = semantics
		return
	}

	panic(fmt.Sprintf("unsupported treasury semantics %d", semantics))
}

// Params returns the chain params associated with the generator instance.
func (g *Generator) Params() *chaincfg.Params {
	return g.params
}

// Tip returns the current tip block of the generator instance.
func (g *Generator) Tip() *wire.MsgBlock {
	return g.tip
}

// TipName returns the name of the current tip block of the generator instance.
func (g *Generator) TipName() string {
	return g.tipName
}

// P2shOpTrueAddr returns the generator p2sh script that is composed with
// a single OP_TRUE.
func (g *Generator) P2shOpTrueAddr() stdaddr.StakeAddress {
	return g.p2shOpTrueAddr
}

// BlockByName returns the block associated with the provided block name.  It
// will panic if the specified block name does not exist.
func (g *Generator) BlockByName(blockName string) *wire.MsgBlock {
	block, ok := g.blocksByName[blockName]
	if !ok {
		panic(fmt.Sprintf("block name %s does not exist", blockName))
	}
	return block
}

// BlockName returns the name associated with the provided block hash.  It will
// panic if the specified block hash does not exist.
func (g *Generator) BlockName(hash *chainhash.Hash) string {
	name, ok := g.blockNames[*hash]
	if !ok {
		panic(fmt.Sprintf("block name for hash %s does not exist", hash))
	}
	return name
}

// BlockByHash returns the block associated with the provided block hash.  It
// will panic if the specified block hash does not exist.
func (g *Generator) BlockByHash(hash *chainhash.Hash) *wire.MsgBlock {
	block, ok := g.blocks[*hash]
	if !ok {
		panic(fmt.Sprintf("block with hash %s does not exist", hash))
	}
	return block
}

// blockHeight returns the block height associated with the provided block hash.
// It will panic if the specified block hash does not exist.
func (g *Generator) blockHeight(hash chainhash.Hash) uint32 {
	height, ok := g.blockHeights[hash]
	if !ok {
		panic(fmt.Sprintf("no block height found for block %s", hash))
	}
	return height
}

// opReturnScript returns a provably-pruneable OP_RETURN script with the
// provided data.
func opReturnScript(data []byte) []byte {
	builder := txscript.NewScriptBuilder()
	script, err := builder.AddOp(txscript.OP_RETURN).AddData(data).Script()
	if err != nil {
		panic(err)
	}
	return script
}

// UniqueOpReturnScript returns a standard provably-pruneable OP_RETURN script
// with a random uint64 encoded as the data.
func UniqueOpReturnScript() []byte {
	rand := rand.Uint64()

	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data[0:8], rand)
	return opReturnScript(data)
}

// calcFullSubsidy returns the full block subsidy for the given block height.
//
// NOTE: This and the other subsidy calculation funcs intentionally are not
// using the blockchain code since the intent is to be able to generate known
// good tests which exercise that code, so it wouldn't make sense to use the
// same code to generate them.
func (g *Generator) calcFullSubsidy(blockHeight uint32) dcrutil.Amount {
	iterations := int64(blockHeight) / g.params.SubsidyReductionInterval
	subsidy := g.params.BaseSubsidy
	for i := int64(0); i < iterations; i++ {
		subsidy *= g.params.MulSubsidy
		subsidy /= g.params.DivSubsidy
	}
	return dcrutil.Amount(subsidy)
}

// calcPoWSubsidy returns the proof-of-work subsidy portion from a given full
// subsidy, block height, and number of votes that will be included in the
// block.
//
// NOTE: This and the other subsidy calculation funcs intentionally are not
// using the blockchain code since the intent is to be able to generate known
// good tests which exercise that code, so it wouldn't make sense to use the
// same code to generate them.
func (g *Generator) calcPoWSubsidy(fullSubsidy dcrutil.Amount, blockHeight uint32, numVotes uint16) dcrutil.Amount {
	const (
		powProportion    = 6
		totalProportions = 10
	)
	powSubsidy := (fullSubsidy * powProportion) / totalProportions
	if int64(blockHeight) < g.params.StakeValidationHeight {
		return powSubsidy
	}

	// Reduce the subsidy according to the number of votes.
	ticketsPerBlock := dcrutil.Amount(g.params.TicketsPerBlock)
	return (powSubsidy * dcrutil.Amount(numVotes)) / ticketsPerBlock
}

// calcPoSSubsidy returns the proof-of-stake subsidy portion for a given block
// height being voted on.
//
// NOTE: This and the other subsidy calculation funcs intentionally are not
// using the blockchain code since the intent is to be able to generate known
// good tests which exercise that code, so it wouldn't make sense to use the
// same code to generate them.
func (g *Generator) calcPoSSubsidy(heightVotedOn uint32) dcrutil.Amount {
	if int64(heightVotedOn+1) < g.params.StakeValidationHeight {
		return 0
	}

	const (
		posProportion    = 3
		totalProportions = 10
	)
	fullSubsidy := g.calcFullSubsidy(heightVotedOn)
	return (fullSubsidy * posProportion) / totalProportions
}

// calcDevSubsidy returns the dev org subsidy portion from a given full subsidy.
//
// NOTE: This and the other subsidy calculation funcs intentionally are not
// using the blockchain code since the intent is to be able to generate known
// good tests which exercise that code, so it wouldn't make sense to use the
// same code to generate them.
func (g *Generator) calcDevSubsidy(fullSubsidy dcrutil.Amount, blockHeight uint32, numVotes uint16) dcrutil.Amount {
	const (
		devProportion    = 1
		totalProportions = 10
	)
	devSubsidy := (fullSubsidy * devProportion) / totalProportions
	if int64(blockHeight) < g.params.StakeValidationHeight {
		return devSubsidy
	}

	// Reduce the subsidy according to the number of votes.
	ticketsPerBlock := dcrutil.Amount(g.params.TicketsPerBlock)
	return (devSubsidy * dcrutil.Amount(numVotes)) / ticketsPerBlock
}

// calcCoinbaseDevSubsidy returns the dev org subsidy portion from a given full
// subsidy for use in the coinbase transaction using the treasury semantics
// configured for the generator.
//
// This differs from [Generator.calcDevSubsidy] in that this considers how the
// treasury semantics affect the coinbase transaction whereas
// [Generator.calcDevSubsidy] does not.
//
// NOTE: This and the other subsidy calculation funcs intentionally are not
// using the blockchain code since the intent is to be able to generate known
// good tests which exercise that code, so it wouldn't make sense to use the
// same code to generate them.
func (g *Generator) calcCoinbaseDevSubsidy(fullSubsidy dcrutil.Amount, blockHeight uint32, numVotes uint16) dcrutil.Amount {
	if g.treasurySemantics == TSDCP0006 && blockHeight > 1 {
		return 0
	}

	return g.calcDevSubsidy(fullSubsidy, blockHeight, numVotes)
}

// standardCoinbaseOpReturnScript returns a standard script suitable for use as
// the second output of a standard coinbase transaction of a new block.  In
// particular, the serialized data used with the OP_RETURN starts with the block
// height and is followed by 32 bytes which are treated as 4 uint64 extra
// nonces.  This implementation puts a cryptographically random value into the
// final extra nonce position.  The actual format of the data after the block
// height is not defined however this effectively mirrors the actual mining code
// at the time it was written.
func standardCoinbaseOpReturnScript(blockHeight uint32) []byte {
	extraNonce := rand.Uint64()

	data := make([]byte, 36)
	binary.LittleEndian.PutUint32(data[0:4], blockHeight)
	binary.LittleEndian.PutUint64(data[28:36], extraNonce)
	return opReturnScript(data)
}

// newTxOut returns a new transaction output with the given parameters.
func newTxOut(amount int64, pkScriptVer uint16, pkScript []byte) *wire.TxOut {
	return &wire.TxOut{
		Value:    amount,
		Version:  pkScriptVer,
		PkScript: pkScript,
	}
}

// addCoinbaseTxOutputs adds the outputs required for a coinbase to the provided
// transaction based on the given parameters and using the treasury semantics
// configured for the generator.
//
// The provided transaction is assumed to be a coinbase transaction.
//
// See the [Generator.CreateCoinbaseTx] documentation for a breakdown of the
// outputs added to the transaction.
func (g *Generator) addCoinbaseTxOutputs(tx *wire.MsgTx, blockHeight uint32, devSubsidy, powSubsidy dcrutil.Amount) {
	// A coinbase for the original treasury semantics requires the first output
	// to be the developer subsidy.  The subsidy was moved to the stake tree for
	// the decentralized treasury, so there is no output for it in that case.
	if g.treasurySemantics == TSOriginal || blockHeight <= 1 {
		tx.AddTxOut(&wire.TxOut{
			Value:    int64(devSubsidy),
			Version:  g.params.OrganizationPkScriptVersion,
			PkScript: g.params.OrganizationPkScript,
		})
	}

	// The next output is a provably prunable data-only output that is used to
	// ensure the coinbase is unique.
	//
	// It will either be the first output when the [TSDCP0006] treasury
	// semantics are in effect or the second when the [TSOriginal] treasury
	// semantics are in effect.
	tx.AddTxOut(wire.NewTxOut(0, standardCoinbaseOpReturnScript(blockHeight)))

	// Final outputs are the proof-of-work subsidy split into more than one
	// output.  These are in turn used throughout the tests as inputs to
	// other transactions such as ticket purchases and additional spend
	// transactions.
	const numPoWOutputs = 6
	amount := powSubsidy / numPoWOutputs
	for i := 0; i < numPoWOutputs; i++ {
		if i == numPoWOutputs-1 {
			amount = powSubsidy - amount*(numPoWOutputs-1)
		}
		tx.AddTxOut(newTxOut(int64(amount), g.p2shOpTrueScriptVer,
			g.p2shOpTrueScript))
	}
}

// CreateCoinbaseTx returns a coinbase transaction that consists of outputs
// which pay an appropriate subsidy based on the passed block height and number
// of votes to the proof-of-work miner and potentially the dev org use the
// treasury semantics configured for the generator.
//
// For the [TSOriginal] treasury semantics the outputs are:
//   - First output pays the development subsidy portion to the dev org
//   - Second output is a standard provably prunable data-only coinbase output
//   - Third and subsequent outputs pay the pow subsidy portion to the generic
//     OP_TRUE p2sh script hash
//
// For the [TSDCP0006] treasury semantics the outputs are:
//   - First output is a standard provably prunable data-only coinbase output
//   - Second and subsequent outputs pay the pow subsidy portion to the generic
//     OP_TRUE p2sh script hash
func (g *Generator) CreateCoinbaseTx(blockHeight uint32, numVotes uint16) *wire.MsgTx {
	// Calculate the subsidy proportions based on the block height and the
	// number of votes the block will include.
	fullSubsidy := g.calcFullSubsidy(blockHeight)
	devSubsidy := g.calcCoinbaseDevSubsidy(fullSubsidy, blockHeight, numVotes)
	powSubsidy := g.calcPoWSubsidy(fullSubsidy, blockHeight, numVotes)

	// The coinbase transaction version depends on the active treasury
	// semantics.
	tx := wire.NewMsgTx()
	if g.treasurySemantics == TSDCP0006 && blockHeight > 1 {
		tx.Version = wire.TxVersionTreasury
	}
	tx.AddTxIn(&wire.TxIn{
		// Coinbase transactions have no inputs, so previous outpoint is
		// zero hash and max index.
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         int64(devSubsidy + powSubsidy),
		BlockHeight:     wire.NullBlockHeight,
		BlockIndex:      wire.NullBlockIndex,
		SignatureScript: coinbaseSigScript,
	})

	g.addCoinbaseTxOutputs(tx, blockHeight, devSubsidy, powSubsidy)

	return tx
}

// standardTreasurybaseOpReturnScript returns a standard script suitable for use
// as the second output of the treasurybase transaction of a new block.  In
// particular, the serialized data used with the OP_RETURN starts with the block
// height and is followed by 8 bytes of cryptographically random data.
func standardTreasurybaseOpReturnScript(blockHeight uint32) []byte {
	randValue := rand.Uint64()

	data := make([]byte, 12)
	binary.LittleEndian.PutUint32(data[0:4], blockHeight)
	binary.LittleEndian.PutUint64(data[4:12], randValue)
	return opReturnScript(data)
}

// CreateTreasuryBaseTx returns a treasurybase transaction that consists of
// outputs which pay an appropriate subsidy based on the passed block height
// number of votes to the treasury.
//
// Inputs:
//   - A single input with the input value set to the total payout amount
//
// Outputs:
//   - Treasury add output that adds to the treasury account balance
//   - Output that includes the block height and a cryptographically random
//     value to ensure a unique hash
//
// Note that all treasurybase transactions require TxVersionTreasury and
// they must be in the stake transaction tree.
func (g *Generator) CreateTreasuryBaseTx(blockHeight uint32, numVotes uint16) *wire.MsgTx {
	// Calculate the subsidy proportion based on the block height and the number
	// of votes the block will include.
	fullSubsidy := g.calcFullSubsidy(blockHeight)
	trsySubsidy := g.calcDevSubsidy(fullSubsidy, blockHeight, numVotes)

	// Create a treasurybase with expected inputs and outputs.
	tx := wire.NewMsgTx()
	tx.Version = wire.TxVersionTreasury
	tx.AddTxIn(&wire.TxIn{
		// Treasurybase transactions have no inputs, so previous outpoint is
		// zero hash and max index.
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         int64(trsySubsidy),
		BlockHeight:     wire.NullBlockHeight,
		BlockIndex:      wire.NullBlockIndex,
		SignatureScript: nil, // Must be nil by consensus.
	})
	tx.AddTxOut(wire.NewTxOut(int64(trsySubsidy), []byte{txscript.OP_TADD}))
	tx.AddTxOut(wire.NewTxOut(0, standardTreasurybaseOpReturnScript(blockHeight)))
	return tx
}

// CreateTicketPurchaseTx creates a new transaction that spends the provided
// output to purchase a stake submission ticket (sstx) at the given ticket
// price.  Both the ticket and the change will go to a p2sh script that is
// composed with a single OP_TRUE.
//
// The transaction consists of the following outputs:
// - First output is an OP_SSTX followed by the OP_TRUE p2sh script hash
// - Second output is an OP_RETURN followed by the commitment script
// - Third output is an OP_SSTXCHANGE followed by the OP_TRUE p2sh script hash
func (g *Generator) CreateTicketPurchaseTx(spend *SpendableOut, ticketPrice, fee dcrutil.Amount) *wire.MsgTx {
	// The first output is the voting rights address.  This impl uses the
	// standard pay-to-script-hash to an OP_TRUE.
	voteScriptVer, voteScript := g.p2shOpTrueAddr.VotingRightsScript()

	// Generate the commitment script.
	commitScriptVer, commitScript := g.p2shOpTrueAddr.RewardCommitmentScript(
		int64(ticketPrice+fee), 0, int64(ticketPrice))

	// Calculate change and generate script to deliver it.
	change := spend.amount - ticketPrice - fee
	changeScriptVer, changeScript := g.p2shOpTrueAddr.StakeChangeScript()

	// Generate and return the transaction spending from the provided
	// spendable output with the previously described outputs.
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: spend.prevOut,
		Sequence:         wire.MaxTxInSequenceNum,
		ValueIn:          int64(spend.amount),
		BlockHeight:      spend.blockHeight,
		BlockIndex:       spend.blockIndex,
		SignatureScript:  opTrueRedeemScript,
	})
	tx.AddTxOut(newTxOut(int64(ticketPrice), voteScriptVer, voteScript))
	tx.AddTxOut(newTxOut(0, commitScriptVer, commitScript))
	tx.AddTxOut(newTxOut(int64(change), changeScriptVer, changeScript))
	return tx
}

// CreateTreasuryTAddChange creates a new transaction that spends the provided
// output to the treasury.  If the amount minus fee is zero the returned
// transaction does not have a change output.
//
// The transaction consists of the following outputs:
// - First output is an OP_TADD
// - Second output is optional and when used it is an OP_SSTXCHANGE paying to
// the provided changeAddr
func (g *Generator) CreateTreasuryTAddChange(spend *SpendableOut,
	amount, fee dcrutil.Amount, changeAddr stdaddr.StakeAddress) *wire.MsgTx {

	// Calculate change and generate script to deliver it.
	var changeScriptVer uint16
	var changeScript []byte
	change := spend.amount - amount - fee
	if change < 0 {
		panic(fmt.Sprintf("negative change %v", change))
	}
	if change > 0 {
		changeScriptVer, changeScript = changeAddr.StakeChangeScript()
	}

	// Generate and return the transaction spending from the provided
	// spendable output with the previously described outputs.
	tx := wire.NewMsgTx()
	tx.Version = wire.TxVersionTreasury
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: spend.prevOut,
		Sequence:         wire.MaxTxInSequenceNum,
		ValueIn:          int64(spend.amount),
		BlockHeight:      spend.blockHeight,
		BlockIndex:       spend.blockIndex,
		SignatureScript:  opTrueRedeemScript,
	})
	tx.AddTxOut(wire.NewTxOut(int64(amount), []byte{txscript.OP_TADD}))
	if len(changeScript) > 0 {
		tx.AddTxOut(newTxOut(int64(change), changeScriptVer, changeScript))
	}
	return tx
}

// CreateTreasuryTAdd creates a new transaction that spends the provided output
// to the treasury and uses the standard chaingen OP_TRUE P2SH as change address.
func (g *Generator) CreateTreasuryTAdd(spend *SpendableOut, amount, fee dcrutil.Amount) *wire.MsgTx {
	return g.CreateTreasuryTAddChange(spend, amount, fee, g.p2shOpTrueAddr)
}

// AddressAmountTuple wraps address+amount in a tuple for easy parameter
// passing.
type AddressAmountTuple struct {
	Address stdaddr.Address
	Amount  dcrutil.Amount
}

// CreateTreasuryTSpend creates a new transaction that spends treasury funds to
// outputs.
//
// The transaction consists of the following outputs:
// - First input is <signature> <pi pubkey> OP_TSPEND
// - First output is an OP_RETURN <32 byte random data>
// - Second and other outputs are OP_TGEN P2PKH/P2SH accounting.
//
// If an address is specified for a payout then it is used, otherwise the
// standard OP_TRUE p2sh script is used as payout address.
func (g *Generator) CreateTreasuryTSpend(privKey []byte, payouts []AddressAmountTuple, fee dcrutil.Amount, expiry uint32) *wire.MsgTx {
	// Calculate total payout.
	totalPayout := int64(0)
	for _, v := range payouts {
		totalPayout += int64(v.Amount)
	}
	valueIn := int64(fee) + totalPayout

	// OP_RETURN <8 byte LE ValueIn><24 byte random>
	// The TSpend TxIn ValueIn is encoded in the first 8 bytes to ensure
	// that it becomes signed. This is consensus enforced.
	var payload [32]byte
	binary.LittleEndian.PutUint64(payload[0:8], uint64(valueIn))
	rand.Read(payload[8:])
	opretScript := opReturnScript(payload[:])
	msgTx := wire.NewMsgTx()
	msgTx.Version = wire.TxVersionTreasury
	msgTx.Expiry = expiry
	msgTx.AddTxOut(wire.NewTxOut(0, opretScript))

	// OP_TGEN
	for _, v := range payouts {
		addr := g.p2shOpTrueAddr.(stdaddr.Address)
		if v.Address != nil {
			addr = v.Address
		}
		scriptVer, script := addr.PaymentScript()
		tgenScript := make([]byte, len(script)+1)
		tgenScript[0] = txscript.OP_TGEN
		copy(tgenScript[1:], script)
		msgTx.AddTxOut(newTxOut(int64(v.Amount), scriptVer, tgenScript))
	}

	// Treasury spend transactions have no inputs since the funds are
	// sourced from a special account, so previous outpoint is zero hash
	// and max index.
	msgTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         int64(fee) + totalPayout,
		BlockHeight:     wire.NullBlockHeight,
		BlockIndex:      wire.NullBlockIndex,
		SignatureScript: nil,
	})

	// Calculate TSpend signature without SigHashType.
	sigscript, err := sign.TSpendSignatureScript(msgTx, privKey)
	if err != nil {
		panic(err)
	}
	msgTx.TxIn[0].SignatureScript = sigscript

	return msgTx
}

// isTicketPurchaseTx returns whether or not the passed transaction is a stake
// ticket purchase.
//
// NOTE: Like many other functions in this test code, this function
// intentionally does not use the blockchain/stake package code since the intent
// is to be able to generate known good tests which exercise that code, so it
// wouldn't make sense to use the same code to generate them.  It must also be
// noted that this function is NOT robust.  It is the minimum necessary needed
// by the testing framework.
func isTicketPurchaseTx(tx *wire.MsgTx) bool {
	if len(tx.TxOut) == 0 {
		return false
	}
	txOut := tx.TxOut[0]
	scriptVer, script := txOut.Version, txOut.PkScript
	return stdscript.IsStakeSubmissionPubKeyHashScript(scriptVer, script) ||
		stdscript.IsStakeSubmissionScriptHashScript(scriptVer, script)
}

// isVoteTx returns whether or not the passed tx is a stake vote (ssgen).
//
// NOTE: Like many other functions in this test code, this function
// intentionally does not use the blockchain/stake package code since the intent
// is to be able to generate known good tests which exercise that code, so it
// wouldn't make sense to use the same code to generate them.  It must also be
// noted that this function is NOT robust.  It is the minimum necessary needed
// by the testing framework.
func isVoteTx(tx *wire.MsgTx) bool {
	if len(tx.TxOut) < 3 {
		return false
	}
	txOut := tx.TxOut[2]
	scriptVer, script := txOut.Version, txOut.PkScript
	return stdscript.IsStakeGenPubKeyHashScript(scriptVer, script) ||
		stdscript.IsStakeGenScriptHashScript(scriptVer, script)
}

// isRevocationTx returns whether or not the passed tx is a stake ticket
// revocation (ssrtx).
//
// NOTE: Like many other functions in this test code, this function
// intentionally does not use the blockchain/stake package code since the intent
// is to be able to generate known good tests which exercise that code, so it
// wouldn't make sense to use the same code to generate them.  It must also be
// noted that this function is NOT robust.  It is the minimum necessary needed
// by the testing framework.
func isRevocationTx(tx *wire.MsgTx) bool {
	if len(tx.TxOut) == 0 {
		return false
	}
	txOut := tx.TxOut[0]
	scriptVer, script := txOut.Version, txOut.PkScript
	return stdscript.IsStakeRevocationPubKeyHashScript(scriptVer, script) ||
		stdscript.IsStakeRevocationScriptHashScript(scriptVer, script)
}

// VoteCommitmentScript returns a standard provably-pruneable OP_RETURN script
// suitable for use in a vote tx (ssgen) given the block hash and height to vote
// on.
func VoteCommitmentScript(hash chainhash.Hash, height uint32) []byte {
	// The vote commitment consists of a 32-byte hash of the block it is
	// voting on along with its expected height as a 4-byte little-endian
	// uint32.  32-byte hash + 4-byte uint32 = 36 bytes.
	var data [36]byte
	copy(data[:], hash[:])
	binary.LittleEndian.PutUint32(data[32:], height)
	script, err := txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).
		AddData(data[:]).Script()
	if err != nil {
		panic(err)
	}
	return script
}

// voteBlockScript returns a standard provably-pruneable OP_RETURN script
// suitable for use in a vote tx (ssgen) given the block to vote on.
func voteBlockScript(parentBlock *wire.MsgBlock) []byte {
	return VoteCommitmentScript(parentBlock.BlockHash(),
		parentBlock.Header.Height)
}

// voteBitsScript returns a standard provably-pruneable OP_RETURN script
// suitable for use in a vote tx (ssgen) with the appropriate vote bits set
// depending on the provided params.
func voteBitsScript(bits uint16, voteVersion uint32) []byte {
	data := make([]byte, 6)
	binary.LittleEndian.PutUint16(data, bits)
	binary.LittleEndian.PutUint32(data[2:], voteVersion)
	if voteVersion == 0 {
		data = data[:2]
	}
	script, err := txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).
		AddData(data).Script()
	if err != nil {
		panic(err)
	}
	return script
}

// CreateVoteTx returns a new transaction (ssgen) paying an appropriate subsidy
// for the given block height (and the number of votes per block) as well as the
// original commitments.
//
// The transaction consists of the following outputs:
//   - First output is an OP_RETURN followed by the block hash and height
//   - Second output is an OP_RETURN followed by the vote bits
//   - Third and subsequent outputs are the payouts according to the ticket
//     commitments and the appropriate proportion of the vote subsidy.
//
// Note that votes for treasury spends may be added to stake vote transactions
// via [AddTreasurySpendVote], [SetTreasurySpendVote], and the block munge
// functions [AddTreasurySpendYesVotes] and [AddTreasurySpendNoVotes].
func (g *Generator) CreateVoteTx(voteBlock *wire.MsgBlock, ticketTx *wire.MsgTx, ticketBlockHeight, ticketBlockIndex uint32) *wire.MsgTx {
	// Calculate the proof-of-stake subsidy proportion based on the block
	// height.
	posSubsidy := g.calcPoSSubsidy(voteBlock.Header.Height)
	voteSubsidy := posSubsidy / dcrutil.Amount(g.params.TicketsPerBlock)
	ticketPrice := dcrutil.Amount(ticketTx.TxOut[0].Value)

	// The first output is the block (hash and height) the vote is for.
	blockScript := voteBlockScript(voteBlock)

	// The second output is the vote bits.
	voteScript := voteBitsScript(voteBitYes, 0)

	// The third and subsequent outputs pay the original commitment amounts
	// along with the appropriate portion of the vote subsidy.  This impl
	// uses the standard pay-to-script-hash to an OP_TRUE.
	genScriptVer, genScript := g.p2shOpTrueAddr.PayVoteCommitmentScript()

	// Generate and return the transaction with the proof-of-stake subsidy
	// coinbase and spending from the provided ticket along with the
	// previously described outputs.
	ticketHash := ticketTx.CachedTxHash()
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         int64(voteSubsidy),
		BlockHeight:     wire.NullBlockHeight,
		BlockIndex:      wire.NullBlockIndex,
		SignatureScript: g.params.StakeBaseSigScript,
	})
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(ticketHash, 0,
			wire.TxTreeStake),
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         int64(ticketPrice),
		BlockHeight:     ticketBlockHeight,
		BlockIndex:      ticketBlockIndex,
		SignatureScript: opTrueRedeemScript,
	})
	tx.AddTxOut(wire.NewTxOut(0, blockScript))
	tx.AddTxOut(wire.NewTxOut(0, voteScript))
	tx.AddTxOut(newTxOut(int64(voteSubsidy+ticketPrice), genScriptVer, genScript))
	return tx
}

// createVoteTxFromTicket returns a new transaction (ssgen) paying an appropriate subsidy
// for the given block height (and the number of votes per block) as well as the
// original commitments. It requires a stake ticket as a parameter.
func (g *Generator) createVoteTxFromTicket(voteBlock *wire.MsgBlock, ticket *stakeTicket) *wire.MsgTx {
	return g.CreateVoteTx(voteBlock, ticket.tx, ticket.blockHeight, ticket.blockIndex)
}

// CreateRevocationTx returns a new transaction (ssrtx) refunding the ticket
// price for a ticket which either missed its vote or expired.
//
// The transaction consists of the following inputs:
// - The outpoint of the ticket that was missed or expired.
//
// The transaction consists of the following outputs:
// - The payouts according to the ticket commitments.
func (g *Generator) CreateRevocationTx(ticketTx *wire.MsgTx, ticketBlockHeight, ticketBlockIndex uint32) *wire.MsgTx {
	// The outputs pay the original commitment amounts.  This impl uses the
	// standard pay-to-script-hash to an OP_TRUE.
	revokeScrVer, revokeScript := g.p2shOpTrueAddr.PayRevokeCommitmentScript()

	// Generate and return the transaction spending from the provided ticket
	// along with the previously described outputs.
	ticketPrice := ticketTx.TxOut[0].Value
	ticketHash := ticketTx.CachedTxHash()
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(ticketHash, 0,
			wire.TxTreeStake),
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         ticketPrice,
		BlockHeight:     ticketBlockHeight,
		BlockIndex:      ticketBlockIndex,
		SignatureScript: opTrueRedeemScript,
	})
	tx.AddTxOut(newTxOut(ticketPrice, revokeScrVer, revokeScript))
	return tx
}

// CreateRevocationTxFromTicket returns a new transaction (ssrtx) refunding
// the ticket price for a ticket which either missed its vote or expired.
// It requires a stake ticket as a parameter.
func (g *Generator) createRevocationTxFromTicket(ticket *stakeTicket) *wire.MsgTx {
	return g.CreateRevocationTx(ticket.tx, ticket.blockHeight, ticket.blockIndex)
}

// ancestorBlock returns the ancestor block at the provided height by following
// the chain backwards from the given block.  The returned block will be nil
// when a height is requested that is after the height of the passed block.
// Also, a callback can optionally be provided that is invoked with each block
// as it traverses.
func (g *Generator) ancestorBlock(block *wire.MsgBlock, height uint32, f func(*wire.MsgBlock)) *wire.MsgBlock {
	// Nothing to do if the requested height is outside of the valid
	// range.
	if block == nil || height > block.Header.Height {
		return nil
	}

	// Iterate backwards until the requested height is reached.
	for block != nil && block.Header.Height > height {
		block = g.blocks[block.Header.PrevBlock]
		if f != nil && block != nil {
			f(block)
		}
	}

	return block
}

// limitRetarget clamps the passed new difficulty to the old one adjusted by the
// factor specified in the chain parameters.  This ensures the difficulty can
// only move up or down by a limited amount.
func (g *Generator) limitRetarget(oldDiff, newDiff int64) int64 {
	maxRetarget := g.params.RetargetAdjustmentFactor
	switch {
	case newDiff == 0:
		fallthrough
	case (oldDiff / newDiff) > (maxRetarget - 1):
		return oldDiff / maxRetarget
	case (newDiff / oldDiff) > (maxRetarget - 1):
		return oldDiff * maxRetarget
	}

	return newDiff
}

// calcNextRequiredDiffEMA returns the required proof-of-work difficulty for the
// block after the current tip block the generator is associated with using the
// original EMA difficulty algorithm.
//
// An overview of the algorithm is as follows:
//  1. Use the proof-of-work limit for all blocks before the first retarget
//     window
//  2. Use the previous block's difficulty if the next block is not at a
//     retarget interval
//  3. Calculate the ideal retarget difficulty for each window based on the
//     actual timespan of the window versus the target timespan and
//     exponentially weight each difficulty such that the most recent window has
//     the highest weight
//  4. Calculate the final retarget difficulty based on the exponential weighted
//     average and ensure it is limited to the max retarget adjustment factor
func (g *Generator) calcNextRequiredDiffEMA() uint32 {
	// Target difficulty before the first retarget interval is the pow
	// limit.
	nextHeight := g.tip.Header.Height + 1
	windowSize := g.params.WorkDiffWindowSize
	if int64(nextHeight) < windowSize {
		return g.params.PowLimitBits
	}

	// Return the previous block's difficulty requirements if the next block
	// is not at a difficulty retarget interval.
	curDiff := int64(g.tip.Header.Bits)
	if int64(nextHeight)%windowSize != 0 {
		return uint32(curDiff)
	}

	// Calculate the ideal retarget difficulty for each window based on the
	// actual time between blocks versus the target time and exponentially
	// weight them.
	adjustedTimespan := big.NewInt(0)
	tempBig := big.NewInt(0)
	weightedTimespanSum, weightSum := big.NewInt(0), big.NewInt(0)
	targetTimespan := int64(g.params.TargetTimespan)
	targetTimespanBig := big.NewInt(targetTimespan)
	numWindows := g.params.WorkDiffWindows
	weightAlpha := g.params.WorkDiffAlpha
	block := g.tip
	finalWindowTime := block.Header.Timestamp.UnixNano()
	for i := int64(0); i < numWindows; i++ {
		// Get the timestamp of the block at the start of the window and
		// calculate the actual timespan accordingly.  Use the target
		// timespan if there are not yet enough blocks left to cover the
		// window.
		actualTimespan := targetTimespan
		if int64(block.Header.Height) > windowSize {
			for j := int64(0); j < windowSize; j++ {
				block = g.blocks[block.Header.PrevBlock]
			}
			startWindowTime := block.Header.Timestamp.UnixNano()
			actualTimespan = finalWindowTime - startWindowTime

			// Set final window time for the next window.
			finalWindowTime = startWindowTime
		}

		// Calculate the ideal retarget difficulty for the window based
		// on the actual timespan and weight it exponentially by
		// multiplying it by 2^(window_number) such that the most recent
		// window receives the most weight.
		//
		// Also, since integer division is being used, shift up the
		// number of new tickets 32 bits to avoid losing precision.
		//
		//   windowWeightShift = ((numWindows - i) * weightAlpha)
		//   adjustedTimespan = (actualTimespan << 32) / targetTimespan
		//   weightedTimespanSum += adjustedTimespan << windowWeightShift
		//   weightSum += 1 << windowWeightShift
		windowWeightShift := uint((numWindows - i) * weightAlpha)
		adjustedTimespan.SetInt64(actualTimespan)
		adjustedTimespan.Lsh(adjustedTimespan, 32)
		adjustedTimespan.Div(adjustedTimespan, targetTimespanBig)
		adjustedTimespan.Lsh(adjustedTimespan, windowWeightShift)
		weightedTimespanSum.Add(weightedTimespanSum, adjustedTimespan)
		weight := tempBig.SetInt64(1)
		weight.Lsh(weight, windowWeightShift)
		weightSum.Add(weightSum, weight)
	}

	// Calculate the retarget difficulty based on the exponential weighted
	// average and shift the result back down 32 bits to account for the
	// previous shift up in order to avoid losing precision.  Then, limit it
	// to the maximum allowed retarget adjustment factor.
	//
	//   nextDiff = (weightedTimespanSum/weightSum * curDiff) >> 32
	curDiffBig := tempBig.SetInt64(curDiff)
	weightedTimespanSum.Div(weightedTimespanSum, weightSum)
	weightedTimespanSum.Mul(weightedTimespanSum, curDiffBig)
	weightedTimespanSum.Rsh(weightedTimespanSum, 32)
	nextDiff := weightedTimespanSum.Int64()
	nextDiff = g.limitRetarget(curDiff, nextDiff)

	if nextDiff > int64(g.params.PowLimitBits) {
		return g.params.PowLimitBits
	}
	return uint32(nextDiff)
}

// compactToBig converts a compact representation of a whole number N to an
// unsigned 32-bit number.  The representation is similar to IEEE754 floating
// point numbers.
//
// Like IEEE754 floating point, there are three basic components: the sign,
// the exponent, and the mantissa.  They are broken out as follows:
//
//  1. the most significant 8 bits represent the unsigned base 256 exponent
//  2. zero-based bit 23 (the 24th bit) represents the sign bit
//  3. the least significant 23 bits represent the mantissa
//
// Diagram:
//
//	-------------------------------------------------
//	|   Exponent     |    Sign    |    Mantissa     |
//	|-----------------------------------------------|
//	| 8 bits [31-24] | 1 bit [23] | 23 bits [22-00] |
//	-------------------------------------------------
//
// The formula to calculate N is:
//
//	N = (-1^sign) * mantissa * 256^(exponent-3)
//
// This compact form is only used in Decred to encode unsigned 256-bit numbers
// which represent difficulty targets, thus there really is not a need for a
// sign bit, but it is implemented here to stay consistent with bitcoind.
func compactToBig(compact uint32) *big.Int {
	// Extract the mantissa, sign bit, and exponent.
	mantissa := compact & 0x007fffff
	isNegative := compact&0x00800000 != 0
	exponent := uint(compact >> 24)

	// Since the base for the exponent is 256, the exponent can be treated as
	// the number of bytes to represent the full 256-bit number.  So, treat the
	// exponent as the number of bytes and shift the mantissa right or left
	// accordingly.  This is equivalent to:
	// N = mantissa * 256^(exponent-3)
	var bn *big.Int
	if exponent <= 3 {
		mantissa >>= 8 * (3 - exponent)
		bn = big.NewInt(int64(mantissa))
	} else {
		bn = big.NewInt(int64(mantissa))
		bn.Lsh(bn, 8*(exponent-3))
	}

	// Make it negative if the sign bit is set.
	if isNegative {
		bn = bn.Neg(bn)
	}

	return bn
}

// bigToCompact converts a whole number N to a compact representation using an
// unsigned 32-bit number.  The compact representation only provides 23 bits of
// precision, so values larger than (2^23 - 1) only encode the most significant
// digits of the number.  See compactToBig for details.
func bigToCompact(n *big.Int) uint32 {
	// No need to do any work if it's zero.
	if n.Sign() == 0 {
		return 0
	}

	// Since the base for the exponent is 256, the exponent can be treated as
	// the number of bytes.  So, shift the number right or left accordingly.
	// This is equivalent to:
	// mantissa = mantissa / 256^(exponent-3)
	var mantissa uint32
	exponent := uint(len(n.Bytes()))
	if exponent <= 3 {
		mantissa = uint32(n.Bits()[0])
		mantissa <<= 8 * (3 - exponent)
	} else {
		// Use a copy to avoid modifying the caller's original number.
		tn := new(big.Int).Set(n)
		mantissa = uint32(tn.Rsh(tn, 8*(exponent-3)).Bits()[0])
	}

	// When the mantissa already has the sign bit set, the number is too large
	// to fit into the available 23-bits, so divide the number by 256 and
	// increment the exponent accordingly.
	if mantissa&0x00800000 != 0 {
		mantissa >>= 8
		exponent++
	}

	// Pack the exponent, sign bit, and mantissa into an unsigned 32-bit int and
	// return it.
	compact := uint32(exponent<<24) | mantissa
	if n.Sign() < 0 {
		compact |= 0x00800000
	}
	return compact
}

// calcASERTDiff calculates an absolutely scheduled exponentially weighted
// target difficulty for the given set of parameters using the algorithm defined
// in DCP0011.
//
// The Absolutely Scheduled Exponentially weighted Rising Targets (ASERT)
// algorithm defines an ideal schedule for block issuance and calculates the
// difficulty based on how far the most recent block's timestamp is ahead or
// behind that schedule.
//
// The target difficulty is set exponentially such that it is doubled or halved
// for every multiple of the half life the most recent block is ahead or behind
// the ideal schedule.
//
// The starting difficulty bits parameter is the initial target difficulty all
// calculations use as a reference.  This value is defined on a per-chain basis.
// It must be non-zero and less than or equal to the provided proof of work
// limit or the function will panic.
//
// The time delta is the number of seconds that have elapsed between the most
// recent block and an initial reference timestamp.
//
// The height delta is the number of blocks between the most recent block height
// and an initial reference height.  It must be non-negative or the function
// will panic.
//
// This function is safe for concurrent access.
func calcASERTDiff(startDiffBits uint32, powLimit *big.Int, targetSecsPerBlock,
	timeDelta, heightDelta, halfLife int64) uint32 {

	// Ensure parameter assumptions are not violated.
	//
	// 1. The starting target difficulty must be in the range [1, powLimit]
	// 2. The height to calculate the difficulty for must come after the height
	//    of the reference block
	startDiff := compactToBig(startDiffBits)
	if startDiff.Sign() <= 0 || startDiff.Cmp(powLimit) > 0 {
		panic(fmt.Sprintf("starting difficulty %064x is not in the valid "+
			"range [1, %064x]", startDiff, powLimit))
	}
	if heightDelta < 0 {
		panic(fmt.Sprintf("provided height delta %d is negative", heightDelta))
	}

	// nolint: dupword
	//
	// Calculate the target difficulty by multiplying the provided starting
	// target difficulty by an exponential scaling factor that is determined
	// based on how far ahead or behind the ideal schedule the given time delta
	// is along with a half life that acts as a smoothing factor.
	//
	// Per DCP0011, the goal equation is:
	//
	//   nextDiff = min(max(startDiff * 2^((Δt - Δh*Ib)/halfLife), 1), powLimit)
	//
	// However, in order to avoid the need to perform floating point math which
	// is problematic across languages due to uncertainty in floating point math
	// libs, the formula is implemented using a combination of fixed-point
	// integer arithmetic and a cubic polynomial approximation to the 2^x term.
	//
	// In particular, the goal cubic polynomial approximation over the interval
	// 0 <= x < 1 is:
	//
	//   2^x ~= 1 + 0.695502049712533x + 0.2262697964x^2 + 0.0782318x^3
	//
	// This approximation provides an absolute error margin < 0.013% over the
	// aforementioned interval of [0,1) which is well under the 0.1% error
	// margin needed for good results.  Note that since the input domain is not
	// constrained to that interval, the exponent is decomposed into an integer
	// part, n, and a fractional part, f, such that f is in the desired range of
	// [0,1).  By exponent rules 2^(n + f) = 2^n * 2^f, so the strategy is to
	// calculate the result by applying the cubic polynomial approximation to
	// the fractional part and using the fact that multiplying by 2^n is
	// equivalent to an arithmetic left or right shift depending on the sign.
	//
	// In other words, start by calculating the exponent (x) using 64.16 fixed
	// point and decompose it into integer (n) and fractional (f) parts as
	// follows:
	//
	//       2^16 * (Δt - Δh*Ib)   (Δt - Δh*Ib) << 16
	//   x = ------------------- = ------------------
	//            halfLife              halfLife
	//
	//        x
	//   n = ---- = x >> 16
	//       2^16
	//
	//   f = x (mod 2^16) = x & 0xffff
	//
	// The use of 64.16 fixed point for the exponent means both the integer (n)
	// and fractional (f) parts have an additional factor of 2^16.  Since the
	// fractional part of the exponent is cubed in the polynomial approximation
	// and (2^16)^3 = 2^48, the addition step in the approximation is internally
	// performed using 16.48 fixed point to compensate.
	//
	// In other words, the fixed point formulation of the goal cubic polynomial
	// approximation for the fractional part is:
	//
	//                 195766423245049*f + 971821376*f^2 + 5127*f^3 + 2^47
	//   2^f ~= 2^16 + ---------------------------------------------------
	//                                          2^48
	//
	// Finally, the final target difficulty is calculated using x.16 fixed point
	// and then clamped to the valid range as follows:
	//
	//              startDiff * 2^f * 2^n
	//   nextDiff = ---------------------
	//                       2^16
	//
	//   nextDiff = min(max(nextDiff, 1), powLimit)
	//
	// NOTE: The division by the half life uses Quo instead of Div because it
	// must be truncated division (which is truncated towards zero as Quo
	// implements) as opposed to the Euclidean division that Div implements.
	idealTimeDelta := heightDelta * targetSecsPerBlock
	exponentBig := big.NewInt(timeDelta - idealTimeDelta)
	exponentBig.Lsh(exponentBig, 16)
	exponentBig.Quo(exponentBig, big.NewInt(halfLife))

	// Decompose the exponent into integer and fractional parts.  Since the
	// exponent is using 64.16 fixed point, the bottom 16 bits are the
	// fractional part and the integer part is the exponent arithmetic right
	// shifted by 16.
	frac64 := uint64(exponentBig.Int64() & 0xffff)
	shifts := exponentBig.Rsh(exponentBig, 16).Int64()

	// Calculate 2^16 * 2^(fractional part) of the exponent.
	//
	// Note that a full unsigned 64-bit type is required to avoid overflow in
	// the internal 16.48 fixed point calculation.  Also, the overall result is
	// guaranteed to be positive and a maximum of 17 bits, so it is safe to cast
	// to a uint32.
	const (
		polyCoeff1 uint64 = 195766423245049 // ceil(0.695502049712533 * 2^48)
		polyCoeff2 uint64 = 971821376       // ceil(0.2262697964 * 2^32)
		polyCoeff3 uint64 = 5127            // ceil(0.0782318 * 2^16)
	)
	fracFactor := uint32(1<<16 + (polyCoeff1*frac64+
		polyCoeff2*frac64*frac64+
		polyCoeff3*frac64*frac64*frac64+
		1<<47)>>48)

	// Calculate the target difficulty per the previous discussion:
	//
	//              startDiff * 2^f * 2^n
	//   nextDiff = ---------------------
	//                       2^16
	//
	// Note that by exponent rules 2^n / 2^16 = 2^(n - 16).  This takes
	// advantage of that property to reduce the multiplication by 2^n and
	// division by 2^16 to a single shift.
	//
	// This approach also has the benefit of lowering the maximum magnitude
	// relative to what would be the case when first left shifting by a larger
	// value and then right shifting after.  Since arbitrary precision integers
	// are used for this implementation, it doesn't make any difference from a
	// correctness standpoint, however, it does potentially lower the amount of
	// memory for the arbitrary precision type and can be used to help prevent
	// overflow in implementations that use fixed precision types.
	nextDiff := new(big.Int).Set(startDiff)
	nextDiff.Mul(nextDiff, big.NewInt(int64(fracFactor)))
	shifts -= 16
	if shifts >= 0 {
		nextDiff.Lsh(nextDiff, uint(shifts))
	} else {
		nextDiff.Rsh(nextDiff, uint(-shifts))
	}

	// Limit the target difficulty to the valid hardest and easiest values.
	// The valid range is [1, powLimit].
	if nextDiff.Sign() == 0 {
		// The hardest valid target difficulty is 1 since it would be impossible
		// to find a non-negative integer less than 0.
		nextDiff.SetInt64(1)
	} else if nextDiff.Cmp(powLimit) > 0 {
		nextDiff.Set(powLimit)
	}

	// Convert the difficulty to the compact representation and return it.
	return bigToCompact(nextDiff)
}

// calcNextRequiredDiffASERT returns the required proof-of-work difficulty for
// the block after the current tip block the generator is associated with using
// the ASERT difficulty algorithm defined in DCP0011.
func (g *Generator) calcNextRequiredDiffASERT() uint32 {
	anchor := g.powDiffAnchor
	timeDelta := g.tip.Header.Timestamp.Unix() - anchor.Timestamp.Unix()
	heightDelta := int64(g.tip.Header.Height - anchor.Height)
	return calcASERTDiff(g.params.WorkDiffV2Blake3StartBits, g.params.PowLimit,
		int64(g.params.TargetTimePerBlock.Seconds()), timeDelta, heightDelta,
		g.params.WorkDiffV2HalfLifeSecs)
}

// CalcNextRequiredDifficulty returns the required proof-of-work difficulty
// for the block after the current tip block the generator is associated with
// using the difficulty algorithm configured for the generator.
func (g *Generator) CalcNextRequiredDifficulty() uint32 {
	switch g.powDiffAlgo {
	case PDAEma:
		return g.calcNextRequiredDiffEMA()

	case PDAAsert:
		return g.calcNextRequiredDiffASERT()
	}

	panic(fmt.Sprintf("unsupported proof of work difficulty algorithm %d",
		g.powDiffAlgo))
}

// sumPurchasedTickets returns the sum of the number of tickets purchased in the
// most recent specified number of blocks from the point of view of the provided
// block.
func (g *Generator) sumPurchasedTickets(block *wire.MsgBlock, numToSum int64) int64 {
	var numPurchased, numTraversed int64
	for ; block != nil && numTraversed < numToSum; numTraversed++ {
		numPurchased += int64(block.Header.FreshStake)
		block = g.blocks[block.Header.PrevBlock]
	}

	return numPurchased
}

// estimateSupply returns an estimate of the coin supply for the provided block
// height.  This is primarily used in the stake difficulty algorithm and relies
// on an estimate to simplify the necessary calculations.  The actual total
// coin supply as of a given block height depends on many factors such as the
// number of votes included in every prior block (not including all votes
// reduces the subsidy) and whether or not any of the prior blocks have been
// invalidated by stakeholders thereby removing the PoW subsidy for them.
//
// This function is safe for concurrent access.
func estimateSupply(params *chaincfg.Params, height int64) int64 {
	if height <= 0 {
		return 0
	}

	// Estimate the supply by calculating the full block subsidy for each
	// reduction interval and multiplying it the number of blocks in the
	// interval then adding the subsidy produced by number of blocks in the
	// current interval.
	supply := params.BlockOneSubsidy()
	reductions := height / params.SubsidyReductionInterval
	subsidy := params.BaseSubsidy
	for i := int64(0); i < reductions; i++ {
		supply += params.SubsidyReductionInterval * subsidy

		subsidy *= params.MulSubsidy
		subsidy /= params.DivSubsidy
	}
	supply += (1 + height%params.SubsidyReductionInterval) * subsidy

	// Blocks 0 and 1 have special subsidy amounts that have already been
	// added above, so remove what their subsidies would have normally been
	// which were also added above.
	supply -= params.BaseSubsidy * 2

	return supply
}

// CalcNextReqStakeDifficulty returns the required stake difficulty (aka ticket
// price) for the block after the provided block the generator is associated
// with.  The stake difficulty is calculated based on the algorithm defined in
// DCP0001.
func (g *Generator) CalcNextReqStakeDifficulty(prevBlock *wire.MsgBlock) int64 {
	// Stake difficulty before any tickets could possibly be purchased is
	// the minimum value.
	nextHeight := int64(prevBlock.Header.Height) + 1
	stakeDiffStartHeight := int64(g.params.CoinbaseMaturity) + 1
	if nextHeight < stakeDiffStartHeight {
		return g.params.MinimumStakeDiff
	}

	// Return 0 if the current difficulty is already zero since any scaling
	// of 0 is still 0.  This should never really happen since there is a
	// minimum stake difficulty, but the consensus code checks the condition
	// just in case, so follow suit here.
	curDiff := g.tip.Header.SBits
	if curDiff == 0 {
		return 0
	}

	// Return the previous block's difficulty requirements if the next block
	// is not at a difficulty retarget interval.
	intervalSize := g.params.StakeDiffWindowSize
	if nextHeight%intervalSize != 0 {
		return curDiff
	}

	// Get the pool size and number of tickets that were immature at the
	// previous retarget interval.
	//
	// NOTE: Since the stake difficulty must be calculated based on existing
	// blocks, it is always calculated for the block after a given block, so
	// the information for the previous retarget interval must be retrieved
	// relative to the block just before it to coincide with how it was
	// originally calculated.
	var prevPoolSize int64
	prevRetargetHeight := nextHeight - intervalSize - 1
	prevRetargetBlock := g.ancestorBlock(prevBlock, uint32(prevRetargetHeight), nil)
	if prevRetargetBlock != nil {
		prevPoolSize = int64(prevRetargetBlock.Header.PoolSize)
	}
	ticketMaturity := int64(g.params.TicketMaturity)
	prevImmatureTickets := g.sumPurchasedTickets(prevRetargetBlock,
		ticketMaturity)

	// Return the existing ticket price for the first few intervals to avoid
	// division by zero and encourage initial pool population.
	prevPoolSizeAll := prevPoolSize + prevImmatureTickets
	if prevPoolSizeAll == 0 {
		return curDiff
	}

	// Count the number of currently immature tickets.
	immatureTickets := g.sumPurchasedTickets(prevBlock, ticketMaturity)

	// Calculate and return the final next required difficulty.
	curPoolSizeAll := int64(prevBlock.Header.PoolSize) + immatureTickets

	// Shorter version of various parameter for convenience.
	votesPerBlock := int64(g.params.TicketsPerBlock)
	ticketPoolSize := int64(g.params.TicketPoolSize)

	// nolint: dupword
	//
	// Calculate the difficulty by multiplying the old stake difficulty
	// with two ratios that represent a force to counteract the relative
	// change in the pool size (Fc) and a restorative force to push the pool
	// size towards the target value (Fr).
	//
	// Per DCP0001, the generalized equation is:
	//
	//   nextDiff = min(max(curDiff * Fc * Fr, Slb), Sub)
	//
	// The detailed form expands to:
	//
	//                        curPoolSizeAll      curPoolSizeAll
	//   nextDiff = curDiff * ---------------  * -----------------
	//                        prevPoolSizeAll    targetPoolSizeAll
	//
	//   Slb = b.chainParams.MinimumStakeDiff
	//
	//               estimatedTotalSupply
	//   Sub = -------------------------------
	//          targetPoolSize / votesPerBlock
	//
	// In order to avoid the need to perform floating point math which could
	// be problematic across languages due to uncertainty in floating point
	// math libs, this is further simplified to integer math as follows:
	//
	//                   curDiff * curPoolSizeAll^2
	//   nextDiff = -----------------------------------
	//              prevPoolSizeAll * targetPoolSizeAll
	//
	// Further, the Sub parameter must calculate the denominator first using
	// integer math.
	targetPoolSizeAll := votesPerBlock * (ticketPoolSize + ticketMaturity)
	curPoolSizeAllBig := big.NewInt(curPoolSizeAll)
	nextDiffBig := big.NewInt(curDiff)
	nextDiffBig.Mul(nextDiffBig, curPoolSizeAllBig)
	nextDiffBig.Mul(nextDiffBig, curPoolSizeAllBig)
	nextDiffBig.Div(nextDiffBig, big.NewInt(prevPoolSizeAll))
	nextDiffBig.Div(nextDiffBig, big.NewInt(targetPoolSizeAll))

	// Limit the new stake difficulty between the minimum allowed stake
	// difficulty and a maximum value that is relative to the total supply.
	//
	// NOTE: This is intentionally using integer math to prevent any
	// potential issues due to uncertainty in floating point math libs.  The
	// ticketPoolSize parameter already contains the result of
	// (targetPoolSize / votesPerBlock).
	nextDiff := nextDiffBig.Int64()
	estimatedSupply := estimateSupply(g.params, nextHeight)
	maximumStakeDiff := estimatedSupply / ticketPoolSize
	if nextDiff > maximumStakeDiff {
		nextDiff = maximumStakeDiff
	}
	if nextDiff < g.params.MinimumStakeDiff {
		nextDiff = g.params.MinimumStakeDiff
	}
	return nextDiff
}

// CalcNextRequiredStakeDifficulty returns the required stake difficulty (aka
// ticket price) for the block after the current tip block the generator is
// associated with.
func (g *Generator) CalcNextRequiredStakeDifficulty() int64 {
	return g.CalcNextReqStakeDifficulty(g.tip)
}

// hash256prng is a deterministic pseudorandom number generator that uses a
// 256-bit secure hashing function to generate random uint32s starting from
// an initial seed.
type hash256prng struct {
	seed       chainhash.Hash // Initialization seed
	idx        uint64         // Hash iterator index
	cachedHash chainhash.Hash // Most recently generated hash
	hashOffset int            // Offset into most recently generated hash
}

// newHash256PRNG creates a pointer to a newly created hash256PRNG.
func newHash256PRNG(seed []byte) *hash256prng {
	// The provided seed is initialized by appending a constant derived from
	// the hex representation of pi and hashing the result to give 32 bytes.
	// This ensures the PRNG is always doing a short number of rounds
	// regardless of input since it will only need to hash small messages
	// (less than 64 bytes).
	seedHash := chainhash.HashFunc(append(seed, hash256prngSeedConst...))
	return &hash256prng{
		seed:       seedHash,
		idx:        0,
		cachedHash: seedHash,
	}
}

// State returns a hash that represents the current state of the deterministic
// PRNG.
func (hp *hash256prng) State() chainhash.Hash {
	// The final state is the hash of the most recently generated hash
	// concatenated with both the hash iterator index and the offset into
	// the hash.
	//
	//   hash(hp.cachedHash || hp.idx || hp.hashOffset)
	finalState := make([]byte, len(hp.cachedHash)+4+1)
	copy(finalState, hp.cachedHash[:])
	offset := len(hp.cachedHash)
	binary.BigEndian.PutUint32(finalState[offset:], uint32(hp.idx))
	offset += 4
	finalState[offset] = byte(hp.hashOffset)
	return chainhash.HashH(finalState)
}

// Hash256Rand returns a uint32 random number using the pseudorandom number
// generator and updates the state.
func (hp *hash256prng) Hash256Rand() uint32 {
	offset := hp.hashOffset * 4
	r := binary.BigEndian.Uint32(hp.cachedHash[offset : offset+4])
	hp.hashOffset++

	// Generate a new hash and reset the hash position index once it would
	// overflow the available bytes in the most recently generated hash.
	if hp.hashOffset > 7 {
		// Hash of the seed concatenated with the hash iterator index.
		//   hash(hp.seed || hp.idx)
		data := make([]byte, len(hp.seed)+4)
		copy(data, hp.seed[:])
		binary.BigEndian.PutUint32(data[len(hp.seed):], uint32(hp.idx))
		hp.cachedHash = chainhash.HashH(data)
		hp.idx++
		hp.hashOffset = 0
	}

	// Roll over the entire PRNG by re-hashing the seed when the hash
	// iterator index overflows a uint32.
	if hp.idx > math.MaxUint32 {
		hp.seed = chainhash.HashH(hp.seed[:])
		hp.cachedHash = hp.seed
		hp.idx = 0
	}

	return r
}

// uniformRandom returns a random in the range [0, upperBound) while avoiding
// modulo bias to ensure a normal distribution within the specified range.
func (hp *hash256prng) uniformRandom(upperBound uint32) uint32 {
	if upperBound < 2 {
		return 0
	}

	// (2^32 - (x*2)) % x == 2^32 % x when x <= 2^31
	min := ((math.MaxUint32 - (upperBound * 2)) + 1) % upperBound
	if upperBound > 0x80000000 {
		min = 1 + ^upperBound
	}

	r := hp.Hash256Rand()
	for r < min {
		r = hp.Hash256Rand()
	}
	return r % upperBound
}

// winningTickets returns a slice of tickets that are required to vote for the
// given block being voted on and live ticket pool and the associated underlying
// deterministic prng state hash.
func winningTickets(voteBlock *wire.MsgBlock, liveTickets []*stakeTicket, numVotes uint16) ([]*stakeTicket, chainhash.Hash, error) {
	// Serialize the parent block header used as the seed to the
	// deterministic pseudo random number generator for vote selection.
	var buf bytes.Buffer
	buf.Grow(wire.MaxBlockHeaderPayload)
	if err := voteBlock.Header.Serialize(&buf); err != nil {
		return nil, chainhash.Hash{}, err
	}

	// Ensure the number of live tickets is within the allowable range.
	numLiveTickets := len(liveTickets)
	if numLiveTickets > math.MaxUint32 {
		return nil, chainhash.Hash{}, fmt.Errorf("live ticket pool "+
			"has %d tickets which is more than the max allowed of "+
			"%d", len(liveTickets), uint32(math.MaxUint32))
	}
	if uint32(numVotes) > uint32(numLiveTickets) {
		return nil, chainhash.Hash{}, fmt.Errorf("live ticket pool "+
			"has %d tickets, while %d are needed to vote",
			len(liveTickets), numVotes)
	}

	// Construct list of winners by generating successive values from the
	// deterministic prng and using them as indices into the sorted live
	// ticket pool while skipping any duplicates that might occur.
	prng := newHash256PRNG(buf.Bytes())
	winners := make([]*stakeTicket, 0, numVotes)
	usedOffsets := make(map[uint32]struct{})
	for uint16(len(winners)) < numVotes {
		ticketIndex := prng.uniformRandom(uint32(numLiveTickets))
		if _, exists := usedOffsets[ticketIndex]; !exists {
			usedOffsets[ticketIndex] = struct{}{}
			winners = append(winners, liveTickets[ticketIndex])
		}
	}
	return winners, prng.State(), nil
}

// calcFinalLotteryState calculates the final lottery state for a set of winning
// tickets and the associated deterministic prng state hash after selecting the
// winners.  It is the first 6 bytes of:
//
//	blake256(firstTicketHash || ... || lastTicketHash || prngStateHash)
func calcFinalLotteryState(winners []*stakeTicket, prngStateHash chainhash.Hash) [6]byte {
	data := make([]byte, (len(winners)+1)*chainhash.HashSize)
	for i := 0; i < len(winners); i++ {
		h := winners[i].tx.CachedTxHash()
		copy(data[chainhash.HashSize*i:], h[:])
	}
	copy(data[chainhash.HashSize*len(winners):], prngStateHash[:])
	dataHash := chainhash.HashH(data)

	var finalState [6]byte
	copy(finalState[:], dataHash[0:6])
	return finalState
}

// nextPowerOfTwo returns the next highest power of two from a given number if
// it is not already a power of two.  This is a helper function used during the
// calculation of a merkle tree.
func nextPowerOfTwo(n int) int {
	// Return the number if it's already a power of 2.
	if n&(n-1) == 0 {
		return n
	}

	// Figure out and return the next power of two.
	exponent := uint(math.Log2(float64(n))) + 1
	return 1 << exponent // 2^exponent
}

// hashMerkleBranches takes two hashes, treated as the left and right tree
// nodes, and returns the hash of their concatenation.  This is a helper
// function used to aid in the generation of a merkle tree.
func hashMerkleBranches(left *chainhash.Hash, right *chainhash.Hash) *chainhash.Hash {
	// Concatenate the left and right nodes.
	var hash [chainhash.HashSize * 2]byte
	copy(hash[:chainhash.HashSize], left[:])
	copy(hash[chainhash.HashSize:], right[:])

	newHash := chainhash.HashH(hash[:])
	return &newHash
}

// buildMerkleTreeStore creates a merkle tree from a slice of transactions,
// stores it using a linear array, and returns a slice of the backing array.  A
// linear array was chosen as opposed to an actual tree structure since it uses
// about half as much memory.  The following describes a merkle tree and how it
// is stored in a linear array.
//
// A merkle tree is a tree in which every non-leaf node is the hash of its
// children nodes.  A diagram depicting how this works for Decred transactions
// where h(x) is a blake256 hash follows:
//
//	         root = h1234 = h(h12 + h34)
//	        /                           \
//	  h12 = h(h1 + h2)            h34 = h(h3 + h4)
//	   /            \              /            \
//	h1 = h(tx1)  h2 = h(tx2)    h3 = h(tx3)  h4 = h(tx4)
//
// The above stored as a linear array is as follows:
//
//	[h1 h2 h3 h4 h12 h34 root]
//
// As the above shows, the merkle root is always the last element in the array.
//
// The number of inputs is not always a power of two which results in a
// balanced tree structure as above.  In that case, parent nodes with no
// children are also zero and parent nodes with only a single left node
// are calculated by concatenating the left node with itself before hashing.
// Since this function uses nodes that are pointers to the hashes, empty nodes
// will be nil.
func buildMerkleTreeStore(transactions []*dcrutil.Tx) []*chainhash.Hash {
	// If there's an empty stake tree, return totally zeroed out merkle tree root
	// only.
	if len(transactions) == 0 {
		merkles := make([]*chainhash.Hash, 1)
		merkles[0] = &chainhash.Hash{}
		return merkles
	}

	// Calculate how many entries are required to hold the binary merkle
	// tree as a linear array and create an array of that size.
	nextPoT := nextPowerOfTwo(len(transactions))
	arraySize := nextPoT*2 - 1
	merkles := make([]*chainhash.Hash, arraySize)

	// Create the base transaction hashes and populate the array with them.
	for i, tx := range transactions {
		msgTx := tx.MsgTx()
		txHashFull := msgTx.TxHashFull()
		merkles[i] = &txHashFull
	}

	// Start the array offset after the last transaction and adjusted to the
	// next power of two.
	offset := nextPoT
	for i := 0; i < arraySize-1; i += 2 {
		switch {
		// When there is no left child node, the parent is nil too.
		case merkles[i] == nil:
			merkles[offset] = nil

		// When there is no right child, the parent is generated by
		// hashing the concatenation of the left child with itself.
		case merkles[i+1] == nil:
			newHash := hashMerkleBranches(merkles[i], merkles[i])
			merkles[offset] = newHash

		// The normal case sets the parent node to the hash of the
		// concatenation of the left and right children.
		default:
			newHash := hashMerkleBranches(merkles[i], merkles[i+1])
			merkles[offset] = newHash
		}
		offset++
	}

	return merkles
}

// calcMerkleRoot creates a merkle tree from the slice of transactions and
// returns the root of the tree.
func calcMerkleRoot(txns []*wire.MsgTx) chainhash.Hash {
	utilTxns := make([]*dcrutil.Tx, 0, len(txns))
	for _, tx := range txns {
		utilTxns = append(utilTxns, dcrutil.NewTx(tx))
	}
	merkles := buildMerkleTreeStore(utilTxns)
	return *merkles[len(merkles)-1]
}

// hashToBig converts a chainhash.Hash into a big.Int that can be used to
// perform math comparisons.
func hashToBig(hash *chainhash.Hash) *big.Int {
	// A Hash is in little-endian, but the big package wants the bytes in
	// big-endian, so reverse them.
	buf := *hash
	blen := len(buf)
	for i := 0; i < blen/2; i++ {
		buf[i], buf[blen-1-i] = buf[blen-1-i], buf[i]
	}

	return new(big.Int).SetBytes(buf[:])
}

// IsSolved returns whether or not the header hashes to a value that is less
// than or equal to the target difficulty as specified by its bits field while
// respecting the proof of work hashing algorithm associated with the generator
// state.
func (g *Generator) IsSolved(header *wire.BlockHeader) bool {
	targetDifficulty := compactToBig(header.Bits)
	var hash chainhash.Hash
	switch g.powHashAlgo {
	case PHABlake256r14:
		hash = header.PowHashV1()
	case PHABlake3:
		hash = header.PowHashV2()
	default:
		panic(fmt.Sprintf("unsupported proof of work hash algorithm %d",
			g.powHashAlgo))
	}
	return hashToBig(&hash).Cmp(targetDifficulty) <= 0
}

// solveBlock attempts to find a nonce which makes the passed block header hash
// to a value less than the target difficulty.  When a successful solution is
// found, true is returned and the nonce field of the passed header is updated
// with the solution.  False is returned if no solution exists.
//
// NOTE: This function will never solve blocks with a nonce of 0.  This is done
// so [Generator.NextBlock] can properly detect when a nonce was modified by a
// munge function.
func (g *Generator) solveBlock(header *wire.BlockHeader) bool {
	// sbResult is used by the solver goroutines to send results.
	type sbResult struct {
		found bool
		nonce uint32
	}

	// solver accepts a block header and a nonce range to test. It is
	// intended to be run as a goroutine.
	targetDifficulty := compactToBig(header.Bits)
	quit := make(chan bool)
	results := make(chan sbResult)
	solver := func(hdr wire.BlockHeader, startNonce, stopNonce uint32) {
		// Choose which proof of work hash algorithm to use based on the
		// associated state.
		var powHashFn func() chainhash.Hash
		switch g.powHashAlgo {
		case PHABlake256r14:
			powHashFn = hdr.PowHashV1
		case PHABlake3:
			powHashFn = hdr.PowHashV2
		default:
			panic(fmt.Sprintf("unsupported proof of work hash algorithm %d",
				g.powHashAlgo))
		}

		// We need to modify the nonce field of the header, so make sure
		// we work with a copy of the original header.
		for i := startNonce; i >= startNonce && i <= stopNonce; i++ {
			select {
			case <-quit:
				results <- sbResult{false, 0}
				return
			default:
				hdr.Nonce = i
				hash := powHashFn()
				if hashToBig(&hash).Cmp(targetDifficulty) <= 0 {
					results <- sbResult{true, i}
					return
				}
			}
		}
		results <- sbResult{false, 0}
	}

	startNonce := uint32(1)
	stopNonce := uint32(math.MaxUint32)
	numCores := uint32(runtime.NumCPU())
	noncesPerCore := (stopNonce - startNonce) / numCores
	for i := uint32(0); i < numCores; i++ {
		rangeStart := startNonce + (noncesPerCore * i)
		rangeStop := startNonce + (noncesPerCore * (i + 1)) - 1
		if i == numCores-1 {
			rangeStop = stopNonce
		}
		go solver(*header, rangeStart, rangeStop)
	}
	var foundResult bool
	for i := uint32(0); i < numCores; i++ {
		result := <-results
		if !foundResult && result.found {
			close(quit)
			header.Nonce = result.nonce
			foundResult = true
		}
	}

	return foundResult
}

// ReplaceWithNVotes returns a function that itself takes a block and modifies
// it by replacing the votes in the stake tree with specified number of votes.
//
// NOTE: This must only be used as a munger to [Generator.NextBlock] or it will
// lead to an invalid live ticket pool.  To help safeguard against improper
// usage, it will panic if called with a block that does not connect to the
// current tip block.
func (g *Generator) ReplaceWithNVotes(numVotes uint16) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		// Attempt to prevent misuse of this function by ensuring the
		// provided block connects to the current tip.
		if b.Header.PrevBlock != g.tip.BlockHash() {
			panic(fmt.Sprintf("attempt to replace number of votes "+
				"for block %s with parent %s that is not the "+
				"current tip %s", b.BlockHash(),
				b.Header.PrevBlock, g.tip.BlockHash()))
		}

		// Get the winning tickets for the specified number of votes.
		parentBlock := g.tip
		winners, _, err := winningTickets(parentBlock, g.liveTickets,
			numVotes)
		if err != nil {
			panic(err)
		}

		// Find the existing votes and ensure they are all contiguous since the
		// following logic depends on it.
		voteIdxs := make([]int, 0, b.Header.Voters)
		for txIdx, tx := range b.STransactions {
			if isVoteTx(tx) {
				voteIdxs = append(voteIdxs, txIdx)
			}
		}
		var startVoteIdx, endVoteIdx int
		numExistingVotes := len(voteIdxs)
		if numExistingVotes > 0 {
			startVoteIdx = voteIdxs[0]
			endVoteIdx = voteIdxs[len(voteIdxs)-1]
			if (endVoteIdx-startVoteIdx)+1 != numExistingVotes {
				panic(fmt.Sprintf("attempt to replace number of votes for "+
					"block %s with parent %s that does not have contiguous "+
					"votes", b.BlockHash(), b.Header.PrevBlock))
			}
		}

		// Generate vote transactions for the winning tickets while maintaining
		// their original position in the stake tree.
		numExistingTxns := len(b.STransactions) - numExistingVotes
		stakeTxns := make([]*wire.MsgTx, 0, numExistingTxns+int(numVotes))
		stakeTxns = append(stakeTxns, b.STransactions[:startVoteIdx]...)
		for _, ticket := range winners {
			voteTx := g.createVoteTxFromTicket(parentBlock, ticket)
			stakeTxns = append(stakeTxns, voteTx)
		}

		// Add back the original stake transactions other than the original
		// stake votes that have been replaced.
		stakeTxns = append(stakeTxns, b.STransactions[endVoteIdx+1:]...)

		// Update the block with the new stake transactions and the
		// header with the new number of votes.
		b.STransactions = stakeTxns
		b.Header.Voters = numVotes

		// Recalculate the coinbase amount based on the number of new
		// votes and update the coinbase so that the adjustment in
		// subsidy is accounted for.
		height := b.Header.Height
		fullSubsidy := g.calcFullSubsidy(height)
		devSubsidy := g.calcCoinbaseDevSubsidy(fullSubsidy, height, numVotes)
		powSubsidy := g.calcPoWSubsidy(fullSubsidy, height, numVotes)
		cbTx := b.Transactions[0]
		cbTx.TxIn[0].ValueIn = int64(devSubsidy + powSubsidy)
		cbTx.TxOut = nil
		g.addCoinbaseTxOutputs(cbTx, height, devSubsidy, powSubsidy)
	}
}

// ReplaceVoteBitsN returns a function that itself takes a block and modifies
// it by replacing the vote bits of the vote located at the provided index.  It
// will panic if the stake transaction at the provided index is not already a
// vote.
//
// NOTE: This must only be used as a munger to [Generator.NextBlock] or it will
// lead to an invalid live ticket pool.
func (g *Generator) ReplaceVoteBitsN(voteNum int, voteBits uint16) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		// Attempt to prevent misuse of this function by ensuring the
		// provided stake transaction number is actually a vote.
		stx := b.STransactions[voteNum]
		if !isVoteTx(stx) {
			panic(fmt.Sprintf("attempt to replace non-vote "+
				"transaction #%d for block %s", voteNum,
				b.BlockHash()))
		}

		// Extract the existing vote version.
		existingScript := stx.TxOut[1].PkScript
		var voteVersion uint32
		if len(existingScript) >= 8 {
			voteVersion = binary.LittleEndian.Uint32(existingScript[4:8])
		}

		stx.TxOut[1].PkScript = voteBitsScript(voteBits, voteVersion)
	}
}

// ReplaceVoteBits returns a function that itself takes a block and modifies it
// by replacing the vote bits of the stake transactions.
//
// NOTE: This must only be used as a munger to [Generator.NextBlock] or it will
// lead to an invalid live ticket pool.
func (g *Generator) ReplaceVoteBits(voteBits uint16) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		for stxIdx, stx := range b.STransactions {
			if isVoteTx(stx) {
				g.ReplaceVoteBitsN(stxIdx, voteBits)(b)
			}
		}
	}
}

// ReplaceBlockVersion returns a function that itself takes a block and modifies
// it by replacing the stake version of the header.
func ReplaceBlockVersion(newVersion int32) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		b.Header.Version = newVersion
	}
}

// ReplaceStakeVersion returns a function that itself takes a block and modifies
// it by replacing the stake version of the header.
func ReplaceStakeVersion(newVersion uint32) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		b.Header.StakeVersion = newVersion
	}
}

// ReplaceVoteVersions returns a function that itself takes a block and modifies
// it by replacing the voter version of the stake transactions.
//
// NOTE: This must only be used as a munger to [Generator.NextBlock] or it will
// lead to an invalid live ticket pool.
func ReplaceVoteVersions(newVersion uint32) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		for _, stx := range b.STransactions {
			if isVoteTx(stx) {
				stx.TxOut[1].PkScript = voteBitsScript(
					voteBitYes, newVersion)
			}
		}
	}
}

// ReplaceVotes returns a function that itself takes a block and modifies it by
// replacing the voter version and bits of the stake transactions.
//
// NOTE: This must only be used as a munger to [Generator.NextBlock] or it will
// lead to an invalid live ticket pool.
func ReplaceVotes(voteBits uint16, newVersion uint32) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		for _, stx := range b.STransactions {
			if isVoteTx(stx) {
				stx.TxOut[1].PkScript = voteBitsScript(voteBits,
					newVersion)
			}
		}
	}
}

// treasurySpendVote houses information needed to cast a vote for a treasury
// spend.  It consists of the hash of the treasury spend transaction and
// the associated vote bits to use when voting on it.
type treasurySpendVote struct {
	hash chainhash.Hash
	bits byte
}

// extractTreasurySpendVoteData extracts the raw vote data from the passed
// script if it is a version 0 script that matches the expected form of a
// provably-pruneable treasury spend vote script.  It will return nil otherwise.
func extractTreasurySpendVoteData(scriptVer uint16, script []byte) []byte {
	// A treasury spend vote consists of a provably-pruneable script of the
	// form:
	//  OP_RETURN DATA_PUSH <TV><votes>
	//
	// Each vote is of the form:
	//  <32 byte spend tx hash><1 byte vote>
	//
	// The DATA_PUSH is one of OP_DATA_X or OP_PUSHDATA1 <1 byte size>.
	//
	// First, ensure the script is a provably-pruneable script that is both
	// large enough to have at least the 'TV' discriminator and one vote and has
	// one of the allowed data pushes.
	const discriminatorSize = 2
	const voteSize = chainhash.HashSize + 1
	const minPossibleSize = 2 + discriminatorSize + voteSize
	if scriptVer != 0 || len(script) < minPossibleSize {
		return nil
	}
	if script[0] != txscript.OP_RETURN || script[1] > txscript.OP_PUSHDATA1 {
		return nil
	}

	// Extract the data push and ensure it starts with the 'TV' discriminator.
	tokenizer := txscript.MakeScriptTokenizer(scriptVer, script[1:])
	tokenizer.Next()
	data := tokenizer.Data()
	if !tokenizer.Done() || data[0] != 'T' || data[1] != 'V' {
		return nil
	}

	// Return the raw vote data push.
	return data[2:]
}

// isTreasurySpendVoteOutput returns whether or not the passed script is a
// version 0 script that matches the expected form of a provably-pruneable
// treasury spend vote script.
func isTreasurySpendVoteOutput(txOut *wire.TxOut) bool {
	return extractTreasurySpendVoteData(txOut.Version, txOut.PkScript) != nil
}

// setTreasurySpendVotes updates the given transaction, which is expected to be
// a stake vote transaction, to include the given treasury spend votes.
//
// It either replaces an existing treasury spend vote output with a new one or
// adds the optional output when there is not already one.  In either case, the
// resulting output will include only the provided treasury spend votes.
//
// It also modifies the transaction version to ensure it is set to the version
// required to indicate an optional treasury spend vote output is included.
func setTreasurySpendVotes(voteTx *wire.MsgTx, votes []treasurySpendVote) {
	// Treasury spend votes are optional and exist in the final output when
	// present.  Add an output when there is not already an existing one.
	//
	// Also, the overall vote transaction version must be set to the appropriate
	// version when the optional treasury spend votes are present, so set it
	// accordingly.
	finalTxOut := voteTx.TxOut[len(voteTx.TxOut)-1]
	if !isTreasurySpendVoteOutput(finalTxOut) {
		const amount = 0
		const scriptVersion = 0
		finalTxOut = newTxOut(amount, scriptVersion, nil)
		voteTx.AddTxOut(finalTxOut)
	}
	voteTx.Version = wire.TxVersionTreasury

	// A treasury spend vote consists of a provably-pruneable script of the
	// form:
	//  OP_RETURN DATA_PUSH <TV><votes>
	//
	// Each vote is of the form:
	//  <32 byte spend tx hash><1 byte vote>
	voteData := make([]byte, 0, 2+len(votes)*(chainhash.HashSize+1))
	voteData = append(voteData, 'T', 'V')
	for _, vote := range votes {
		voteData = append(voteData, vote.hash[:]...)
		voteData = append(voteData, vote.bits)
	}
	finalTxOut.PkScript = opReturnScript(voteData)
}

// SetTreasurySpendVote updates the given transaction, which is expected to be a
// stake vote transaction, to include a vote for the given treasury spend with
// the provided vote bits.
//
// It either replaces an existing treasury spend vote output with a new one or
// adds the optional output when there is not already one.  In either case, the
// resulting output will include only the provided treasury spend vote.
//
// It will panic if the provided vote transaction is not a stake vote.
func SetTreasurySpendVote(voteTx *wire.MsgTx, tspend *wire.MsgTx, bits byte) {
	// Attempt to prevent misuse of this function by ensuring the provided stake
	// transaction is actually a vote.
	if !isVoteTx(voteTx) {
		panic(fmt.Sprintf("attempt to set treasury spend votes on non-vote "+
			"transaction %s", voteTx.TxHash()))
	}

	setTreasurySpendVotes(voteTx, []treasurySpendVote{{tspend.TxHash(), bits}})
}

// extractTreasurySpendVotes extracts any treasury spend votes from the passed
// transaction which is expected to be a stake vote transaction.  It returns nil
// when there are no votes.
func extractTreasurySpendVotes(voteTx *wire.MsgTx) []treasurySpendVote {
	// Treasury spend votes only apply to vote transactions with at least the
	// treasury version.
	if voteTx.Version < wire.TxVersionTreasury {
		return nil
	}

	// Treasury spend votes are in the final output when present.  Attempt to
	// extract the raw vote data when it is of the correct form.
	finalTxOut := voteTx.TxOut[len(voteTx.TxOut)-1]
	scriptVer, script := finalTxOut.Version, finalTxOut.PkScript
	data := extractTreasurySpendVoteData(scriptVer, script)
	if data == nil {
		return nil
	}

	// The data for each treasury spend vote is of the form:
	//  <32 byte spend tx hash><1 byte vote>
	const voteSize = chainhash.HashSize + 1
	numVotes := len(data) / voteSize
	if numVotes == 0 {
		return nil
	}

	votes := make([]treasurySpendVote, 0, numVotes)
	for i := 0; i < numVotes; i++ {
		var vote treasurySpendVote
		copy(vote.hash[:], data[voteSize*i:])
		vote.bits = data[voteSize*i+chainhash.HashSize]
		votes = append(votes, vote)
	}

	return votes
}

// AddTreasurySpendVote modifies the passed stake vote to include the provided
// vote bits for the given associated treasury spend transaction.  Existing
// treasury spend votes are retained.
//
// It will panic if the provided vote transaction is not a stake vote.
func AddTreasurySpendVote(voteTx *wire.MsgTx, tspend *wire.MsgTx, bits byte) {
	// Attempt to prevent misuse of this function by ensuring the provided stake
	// transaction is actually a vote.
	if !isVoteTx(voteTx) {
		panic(fmt.Sprintf("attempt to add treasury spend vote on non-vote "+
			"transaction %s", voteTx.TxHash()))
	}

	// Append the provided treasury spend votes to add to any existing treasury
	// spend votes and update the stake vote to include them.
	existingVotes := extractTreasurySpendVotes(voteTx)
	numCombined := len(existingVotes) + 1
	combinedVotes := make([]treasurySpendVote, 0, numCombined)
	combinedVotes = append(combinedVotes, existingVotes...)
	combinedVotes = append(combinedVotes, treasurySpendVote{
		hash: tspend.TxHash(),
		bits: bits,
	})
	setTreasurySpendVotes(voteTx, combinedVotes)
}

// addTreasurySpendVotes returns a function that itself takes a block and
// modifies it by setting all stake votes to include the provided vote bits for
// the given associated treasury spend transactions.  Existing treasury spend
// votes are retained.
//
// This is an internal func used to implement exported mungers.
//
// NOTE: It must only be used as a munger to [Generator.NextBlock] or it will
// lead to an invalid live ticket pool.
func addTreasurySpendVotes(votesToAdd []treasurySpendVote) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		for _, stx := range b.STransactions {
			if !isVoteTx(stx) {
				continue
			}

			// Append the provided treasury spend votes to add to any existing
			// treasury spend votes and update the stake vote to include them.
			existingVotes := extractTreasurySpendVotes(stx)
			numCombined := len(existingVotes) + len(votesToAdd)
			combinedVotes := make([]treasurySpendVote, 0, numCombined)
			combinedVotes = append(combinedVotes, existingVotes...)
			combinedVotes = append(combinedVotes, votesToAdd...)
			setTreasurySpendVotes(stx, combinedVotes)
		}
	}
}

// AddTreasurySpendYesVotes returns a function that itself takes a block and
// modifies it by setting all stake votes to include yes votes for the provided
// treasury spend transactions.  Existing treasury spend votes are retained.
//
// NOTE: This must only be used as a munger to [Generator.NextBlock] or it will
// lead to an invalid live ticket pool.
func AddTreasurySpendYesVotes(treasurySpendTxns ...*wire.MsgTx) func(*wire.MsgBlock) {
	// Create slice of treasury spend yes votes to add by the returned munger.
	votesToAdd := make([]treasurySpendVote, 0, len(treasurySpendTxns))
	const treasuryVoteYes = 0x01
	for _, tx := range treasurySpendTxns {
		votesToAdd = append(votesToAdd, treasurySpendVote{
			hash: tx.TxHash(),
			bits: treasuryVoteYes,
		})
	}
	return addTreasurySpendVotes(votesToAdd)
}

// AddTreasurySpendNoVotes returns a function that itself takes a block and
// modifies it by setting all stake votes to include no votes for the provided
// treasury spend transactions.  Existing treasury spend votes are retained.
//
// NOTE: This must only be used as a munger to [Generator.NextBlock] or it will
// lead to an invalid live ticket pool.
func AddTreasurySpendNoVotes(treasurySpendTxns ...*wire.MsgTx) func(*wire.MsgBlock) {
	// Create slice of treasury spend no votes to add by the returned munger.
	votesToAdd := make([]treasurySpendVote, 0, len(treasurySpendTxns))
	const treasuryVoteNo = 0x02
	for _, tx := range treasurySpendTxns {
		votesToAdd = append(votesToAdd, treasurySpendVote{
			hash: tx.TxHash(),
			bits: treasuryVoteNo,
		})
	}
	return addTreasurySpendVotes(votesToAdd)
}

// ReplaceVoteSubsidies returns a function that itself takes a block and
// modifies it by replacing the subsidy of all votes contained in the block.
//
// NOTE: This must only be used as a munger to [Generator.NextBlock] or it will
// lead to an invalid live ticket pool.
func ReplaceVoteSubsidies(newSubsidy dcrutil.Amount) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		for _, stx := range b.STransactions {
			if isVoteTx(stx) {
				stx.TxOut[2].Value -= stx.TxIn[0].ValueIn
				stx.TxIn[0].ValueIn = int64(newSubsidy)
				stx.TxOut[2].Value += int64(newSubsidy)
			}
		}
	}
}

// ReplaceRevocationVersions returns a function that itself takes a block and
// modifies it by replacing the transaction version of all revocations contained
// in the block.
func ReplaceRevocationVersions(newVersion uint16) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		for _, stx := range b.STransactions {
			if isRevocationTx(stx) {
				stx.Version = newVersion
			}
		}
	}
}

// CreateRevocationsForMissedTickets returns a function that itself takes a
// block and modifies it by creating revocations for all tickets that would
// otherwise become missed as of that block.
//
// Note: This is intended to be used when the automatic ticket revocations
// agenda is active.  If the agenda is not active, revocations are not valid
// until the block AFTER they become missed or expired.
func (g *Generator) CreateRevocationsForMissedTickets() func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		// Attempt to prevent misuse of this function by ensuring the provided block
		// connects to the current tip.
		if b.Header.PrevBlock != g.tip.BlockHash() {
			panic(fmt.Sprintf("attempted to create revocations for missed tickets "+
				"for block %s with parent %s that is not the current tip %s",
				b.BlockHash(), b.Header.PrevBlock, g.tip.BlockHash()))
		}

		// Create a map of the ticket hashes of votes in the block for faster
		// lookup.
		const ticketInIdx = 1
		ticketsPerBlock := g.params.TicketsPerBlock
		voteTicketHashes := make(map[chainhash.Hash]struct{}, ticketsPerBlock)
		for _, stx := range b.STransactions {
			if isVoteTx(stx) {
				ticketHash := stx.TxIn[ticketInIdx].PreviousOutPoint.Hash
				voteTicketHashes[ticketHash] = struct{}{}
			}
		}

		// Get the winning tickets for the block.
		prevBlock := g.blocks[b.Header.PrevBlock]
		winners, _, err := winningTickets(prevBlock, g.liveTickets, ticketsPerBlock)
		if err != nil {
			panic(err)
		}

		// Create revocation transactions for all tickets that would become missed
		// as of this block due to not having corresponding vote transaction.
		for _, winningStakeTx := range winners {
			if _, ok := voteTicketHashes[winningStakeTx.tx.TxHash()]; !ok {
				// Create a revocation from the ticket and set the signature script to
				// nil since a non-empty signature script is not allowed when the
				// automatic ticket revocations agenda is active.
				revocationTx := g.createRevocationTxFromTicket(winningStakeTx)
				revocationTx.TxIn[0].SignatureScript = nil

				b.STransactions = append(b.STransactions, revocationTx)
				b.Header.Revocations++
			}
		}
	}
}

// CreateSpendTx creates a transaction that spends from the provided spendable
// output and includes an additional unique OP_RETURN output to ensure the
// transaction ends up with a unique hash.  The public key script is a simple
// OP_TRUE p2sh script which avoids the need to track addresses and signature
// scripts in the tests.  The signature script is the opTrueRedeemScript.
func (g *Generator) CreateSpendTx(spend *SpendableOut, fee dcrutil.Amount) *wire.MsgTx {
	spendTx := wire.NewMsgTx()
	spendTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: spend.prevOut,
		Sequence:         wire.MaxTxInSequenceNum,
		ValueIn:          int64(spend.amount),
		BlockHeight:      spend.blockHeight,
		BlockIndex:       spend.blockIndex,
		SignatureScript:  opTrueRedeemScript,
	})
	spendTx.AddTxOut(wire.NewTxOut(int64(spend.amount-fee), g.p2shOpTrueScript))
	spendTx.AddTxOut(wire.NewTxOut(0, UniqueOpReturnScript()))
	return spendTx
}

// CreateSpendTxForTx creates a transaction that spends from the first output of
// the provided transaction and includes an additional unique OP_RETURN output
// to ensure the transaction ends up with a unique hash.  The public key script
// is a simple OP_TRUE p2sh script which avoids the need to track addresses and
// signature scripts in the tests.  This signature script the
// opTrueRedeemScript.
func (g *Generator) CreateSpendTxForTx(tx *wire.MsgTx, blockHeight, txIndex uint32, fee dcrutil.Amount) *wire.MsgTx {
	spend := MakeSpendableOutForTx(tx, blockHeight, txIndex, 0)
	return g.CreateSpendTx(&spend, fee)
}

// removeTicket removes the passed index from the provided slice of tickets and
// returns the resulting slice.  This is an in-place modification.
func removeTicket(tickets []*stakeTicket, index int) []*stakeTicket {
	copy(tickets[index:], tickets[index+1:])
	tickets[len(tickets)-1] = nil // Prevent memory leak
	tickets = tickets[:len(tickets)-1]
	return tickets
}

// connectLiveTickets updates the live ticket pool for a new tip block by
// removing tickets that are now expired from it, removing the passed winners
// from it, adding any immature tickets which are now mature to it, and
// resorting it.
func (g *Generator) connectLiveTickets(blockHash *chainhash.Hash, height uint32, winners, purchases []*stakeTicket) {
	// Move expired tickets from the live ticket pool to the expired ticket
	// pool.
	ticketMaturity := uint32(g.params.TicketMaturity)
	ticketExpiry := g.params.TicketExpiry
	for i := 0; i < len(g.liveTickets); i++ {
		ticket := g.liveTickets[i]
		liveHeight := ticket.blockHeight + ticketMaturity
		expireHeight := liveHeight + ticketExpiry
		if height >= expireHeight {
			g.liveTickets = removeTicket(g.liveTickets, i)
			g.expiredTickets = append(g.expiredTickets, ticket)

			// This is required because the ticket at the current
			// offset was just removed from the slice that is being
			// iterated, so adjust the offset down one accordingly.
			i--
		}
	}

	// Move winning tickets from the live ticket pool to won tickets pool.
	for i := 0; i < len(g.liveTickets); i++ {
		ticket := g.liveTickets[i]
		for _, winner := range winners {
			if ticket.tx.CachedTxHash() == winner.tx.CachedTxHash() {
				g.liveTickets = removeTicket(g.liveTickets, i)

				// This is required because the ticket at the
				// current offset was just removed from the
				// slice that is being iterated, so adjust the
				// offset down one accordingly.
				i--
				break
			}
		}
	}
	g.wonTickets[*blockHash] = winners

	// Move immature tickets which are now mature to the live ticket pool.
	for i := 0; i < len(g.immatureTickets); i++ {
		ticket := g.immatureTickets[i]
		liveHeight := ticket.blockHeight + ticketMaturity
		if height >= liveHeight {
			g.immatureTickets = removeTicket(g.immatureTickets, i)
			g.liveTickets = append(g.liveTickets, ticket)

			// This is required because the ticket at the current
			// offset was just removed from the slice that is being
			// iterated, so adjust the offset down one accordingly.
			i--
		}
	}

	// Resort the ticket pool now that all live ticket pool manipulations
	// are done.
	sort.Sort(stakeTicketSorter(g.liveTickets))

	// Add new ticket purchases to the immature ticket pool.
	g.immatureTickets = append(g.immatureTickets, purchases...)
}

// addMissedVotes adds any of the passed winning tickets as missed votes if the
// passed block does not cast those votes.
func (g *Generator) addMissedVotes(stakeTxns []*wire.MsgTx, winners []*stakeTicket) {
	// Nothing to do before there are any winning tickets.
	if len(winners) == 0 {
		return
	}

	// Assume all of the winning tickets were missed.
	missedVotes := make(map[chainhash.Hash]*stakeTicket)
	for _, ticket := range winners {
		missedVotes[ticket.tx.TxHash()] = ticket
	}

	// Remove the entries for which the block actually contains votes.
	for _, stx := range stakeTxns {
		// Ignore all stake transactions that are not votes.
		if !isVoteTx(stx) {
			continue
		}

		// Ignore the vote if it is not for one of the winning tickets.
		ticketInput := stx.TxIn[1]
		ticketHash := ticketInput.PreviousOutPoint.Hash
		missedVote, ok := missedVotes[ticketHash]
		if !ok || missedVote.blockHeight != ticketInput.BlockHeight ||
			missedVote.blockIndex != ticketInput.BlockIndex {

			continue
		}

		delete(missedVotes, ticketHash)
	}

	// Add the missed votes to the generator state so future blocks will
	// generate revocations for them.
	for ticketHash, missedVote := range missedVotes {
		g.missedVotes[ticketHash] = missedVote
	}
}

// connectRevocations updates the missed and revoked ticket data structs
// according to the revocations in the passed block.
func (g *Generator) connectRevocations(blockHash *chainhash.Hash, stakeTxns []*wire.MsgTx) {
	for _, stx := range stakeTxns {
		// Ignore all stake transactions that are not revocations.
		if !isRevocationTx(stx) {
			continue
		}

		// Ignore the revocation if it is not for a missed ticket.
		ticketInput := stx.TxIn[0]
		ticketHash := ticketInput.PreviousOutPoint.Hash
		ticket, ok := g.missedVotes[ticketHash]
		if !ok || ticket.blockHeight != ticketInput.BlockHeight ||
			ticket.blockIndex != ticketInput.BlockIndex {

			continue
		}

		// Remove the revoked ticket from the missed votes and add it to the
		// list of tickets revoked by the block.
		delete(g.missedVotes, ticketHash)
		g.revokedTickets[*blockHash] = append(g.revokedTickets[*blockHash],
			ticket)
	}
}

// connectBlockTickets updates the live ticket pool and associated data structs
// by for the passed block.  It will panic if the specified block does not
// connect to the current tip block.
func (g *Generator) connectBlockTickets(b *wire.MsgBlock) {
	// Attempt to prevent misuse of this function by ensuring the provided
	// block connects to the current tip.
	blockHash := b.BlockHash()
	if b.Header.PrevBlock != g.tip.BlockHash() {
		panic(fmt.Sprintf("attempt to connect block %s with parent %s "+
			"that is not the current tip %s", blockHash,
			b.Header.PrevBlock, g.tip.BlockHash()))
	}

	// Get all of the winning tickets for the block.
	numVotes := g.params.TicketsPerBlock
	winners, _, err := winningTickets(g.tip, g.liveTickets, numVotes)
	if err != nil {
		panic(err)
	}

	// Keep track of any missed votes.
	g.addMissedVotes(b.STransactions, winners)

	// Keep track of revocations.
	g.connectRevocations(&blockHash, b.STransactions)

	// Extract the ticket purchases (sstx) from the block.
	var purchases []*stakeTicket
	blockHeight := g.blockHeight(blockHash)
	for txIdx, tx := range b.STransactions {
		if isTicketPurchaseTx(tx) {
			ticket := &stakeTicket{tx, blockHeight, uint32(txIdx)}
			purchases = append(purchases, ticket)
		}
	}

	// Update the live ticket pool and associated data structures.
	g.connectLiveTickets(&blockHash, blockHeight, winners, purchases)
}

// disconnectBlockTickets updates the live ticket pool and associated data
// structs by unwinding the passed block, which must be the current tip block.
// It will panic if the specified block is not the current tip block.
func (g *Generator) disconnectBlockTickets(b *wire.MsgBlock) {
	// Attempt to prevent misuse of this function by ensuring the provided
	// block is the current tip.
	blockHash := b.BlockHash()
	if b != g.tip {
		panic(fmt.Sprintf("attempt to disconnect block %s that is not "+
			"the current tip %s", blockHash, g.tip.BlockHash()))
	}

	// Move tickets revoked by the block back to the list of missed tickets.
	for _, ticket := range g.revokedTickets[blockHash] {
		g.missedVotes[ticket.tx.TxHash()] = ticket
	}

	// Remove any votes missed by the block.
	winners := g.wonTickets[blockHash]
	for _, ticket := range winners {
		delete(g.missedVotes, ticket.tx.TxHash())
	}

	// Remove tickets created in the block from the immature ticket pool.
	blockHeight := g.blockHeight(blockHash)
	for i := 0; i < len(g.immatureTickets); i++ {
		ticket := g.immatureTickets[i]
		if ticket.blockHeight == blockHeight {
			g.immatureTickets = removeTicket(g.immatureTickets, i)

			// This is required because the ticket at the current
			// offset was just removed from the slice that is being
			// iterated, so adjust the offset down one accordingly.
			i--
		}
	}

	// Move tickets that are no longer mature from the live ticket pool to
	// the immature ticket pool.
	prevBlockHeight := blockHeight - 1
	ticketMaturity := uint32(g.params.TicketMaturity)
	for i := 0; i < len(g.liveTickets); i++ {
		ticket := g.liveTickets[i]
		liveHeight := ticket.blockHeight + ticketMaturity
		if prevBlockHeight < liveHeight {
			g.liveTickets = removeTicket(g.liveTickets, i)
			g.immatureTickets = append(g.immatureTickets, ticket)

			// This is required because the ticket at the current
			// offset was just removed from the slice that is being
			// iterated, so adjust the offset down one accordingly.
			i--
		}
	}

	// Move tickets that are no longer expired from the expired ticket pool
	// to the live ticket pool.
	ticketExpiry := g.params.TicketExpiry
	for i := 0; i < len(g.expiredTickets); i++ {
		ticket := g.expiredTickets[i]
		liveHeight := ticket.blockHeight + ticketMaturity
		expireHeight := liveHeight + ticketExpiry
		if prevBlockHeight < expireHeight {
			g.expiredTickets = removeTicket(g.expiredTickets, i)
			g.liveTickets = append(g.liveTickets, ticket)

			// This is required because the ticket at the current
			// offset was just removed from the slice that is being
			// iterated, so adjust the offset down one accordingly.
			i--
		}
	}

	// Add the winning tickets consumed by the block back to the live ticket
	// pool.
	g.liveTickets = append(g.liveTickets, winners...)
	delete(g.wonTickets, blockHash)

	// Resort the ticket pool now that all live ticket pool manipulations
	// are done.
	sort.Sort(stakeTicketSorter(g.liveTickets))
}

// originalParent returns the original block the passed block was built from.
// This is necessary because callers might change the previous block hash in a
// munger which would cause the live ticket pool to be reconstructed improperly.
func (g *Generator) originalParent(b *wire.MsgBlock) *wire.MsgBlock {
	parentHash, ok := g.originalParents[b.BlockHash()]
	if !ok {
		parentHash = b.Header.PrevBlock
	}
	return g.BlockByHash(&parentHash)
}

// SetTip changes the tip of the instance to the block with the provided name.
// This is useful since the tip is used for things such as generating subsequent
// blocks.
func (g *Generator) SetTip(blockName string) {
	// Nothing to do if already the tip.
	if blockName == g.tipName {
		return
	}

	newTip := g.blocksByName[blockName]
	if newTip == nil {
		panic(fmt.Sprintf("tip block name %s does not exist", blockName))
	}

	// Create a list of blocks to disconnect and blocks to connect in order
	// to switch to the new tip.
	var connect, disconnect []*wire.MsgBlock
	oldBranch, newBranch := g.tip, newTip
	for oldBranch != newBranch {
		oldBranchHeight := g.blockHeight(oldBranch.BlockHash())
		newBranchHeight := g.blockHeight(newBranch.BlockHash())
		if oldBranchHeight > newBranchHeight {
			disconnect = append(disconnect, oldBranch)
			oldBranch = g.originalParent(oldBranch)
			continue
		} else if newBranchHeight > oldBranchHeight {
			connect = append(connect, newBranch)
			newBranch = g.originalParent(newBranch)
			continue
		}

		// At this point the two branches have the same height, so add
		// each tip to the appropriate connect or disconnect list and
		// the tips to their previous block.
		disconnect = append(disconnect, oldBranch)
		oldBranch = g.originalParent(oldBranch)
		connect = append(connect, newBranch)
		newBranch = g.originalParent(newBranch)
	}

	// Update the live ticket pool and associated data structs by
	// disconnecting all blocks back to the fork point.
	for _, block := range disconnect {
		g.disconnectBlockTickets(block)
		g.tip = g.originalParent(block)
	}

	// Update the live ticket pool and associated data structs by connecting
	// all blocks after the fork point up to the new tip.  The list of
	// blocks to connect is iterated in reverse order, because it was
	// constructed in reverse, and the blocks need to be connected in the
	// order in which they build the chain.
	for i := len(connect) - 1; i >= 0; i-- {
		block := connect[i]
		g.connectBlockTickets(block)
		g.tip = block
	}

	// Ensure the tip is the expected new tip and set the associated name.
	if g.tip != newTip {
		panic(fmt.Sprintf("tip %s is not expected new tip %s",
			g.tip.BlockHash(), newTip.BlockHash()))
	}
	g.tipName = blockName
}

// updateVoteCommitments updates all of the votes in the passed block to commit
// to the previous block hash and previous height based on the values specified
// in the header.
func updateVoteCommitments(block *wire.MsgBlock) {
	for _, stx := range block.STransactions {
		if !isVoteTx(stx) {
			continue
		}

		stx.TxOut[0].PkScript = VoteCommitmentScript(block.Header.PrevBlock,
			block.Header.Height-1)
	}
}

// NextBlock builds a new block that extends the current tip associated with the
// generator and updates the generator's tip to the newly generated block.
//
// The block will include the following:
//
// 1. A coinbase with the following outputs:
//
//   - When the [TSOriginal] treasury semantics are active, one that pays the
//     required 10% subsidy to the dev org
//   - One that contains a standard coinbase OP_RETURN script
//   - Six that pay the required 60% subsidy to an OP_TRUE p2sh script
//
// 2. When a spendable output is provided, a transaction that spends from the
// provided output with the following outputs:
//
//   - One that pays the inputs amount minus 1 atom to an OP_TRUE p2sh script
//   - One that contains an OP_RETURN output with a random uint64 in order to
//     ensure the transaction has a unique hash
//
// 3. Once the coinbase maturity height has been reached, a ticket purchase
// transaction (sstx) for each provided ticket spendable output with the
// following outputs:
//
//   - One OP_SSTX output that grants voting rights to an OP_TRUE p2sh script
//   - One OP_RETURN output that contains the required commitment and pays
//     the subsidy to an OP_TRUE p2sh script
//   - One OP_SSTXCHANGE output that sends change to an OP_TRUE p2sh script
//
// 4. When the [TSDCP0006] treasury semantics are active, a treasurybase with
// the following outputs:
//
//   - One that contains the required OP_TADD script
//   - One that contains a standard treasurybase OP_RETURN script
//
// 5. Once stake validation height has been reached, 5 vote transactions (ssgen)
// as required according to the live ticket pool and vote selection rules with
// the following outputs:
//
//   - One OP_RETURN followed by the block hash and height being voted on
//   - One OP_RETURN followed by the vote bits
//   - One or more OP_SSGEN outputs with the payouts according to the original
//     ticket commitments
//
// 6. Once stake validation height has been reached, revocation transactions
// (ssrtx) as required according to any missed votes with the following outputs:
//
//   - One or more OP_SSRTX outputs with the payouts according to the original
//     ticket commitments
//
// Additionally, if one or more munge functions are specified, they will be
// invoked with the block prior to solving it.  This provides callers with the
// opportunity to modify the block which is especially useful for testing.
//
// In order to simplify the logic in the munge functions, the following rules
// are applied after all munge functions have been invoked:
//   - All votes will have their commitments updated if the previous hash or
//     height was manually changed after stake validation height has been reached
//   - The merkle root will be recalculated unless it was manually changed
//   - The stake root will be recalculated unless it was manually changed
//   - The size of the block will be recalculated unless it was manually changed
//   - The block will be solved unless the nonce was changed
func (g *Generator) NextBlock(blockName string, spend *SpendableOut, ticketSpends []SpendableOut, mungers ...func(*wire.MsgBlock)) *wire.MsgBlock {
	// Prevent block name collisions.
	if g.blocksByName[blockName] != nil {
		panic(fmt.Sprintf("block name %s already exists", blockName))
	}

	// Calculate the next required stake difficulty (aka ticket price).
	ticketPrice := dcrutil.Amount(g.CalcNextRequiredStakeDifficulty())

	// Generate the appropriate votes and ticket purchases based on the
	// current tip block and provided ticket spendable outputs.
	var ticketWinners []*stakeTicket
	var stakeTxns []*wire.MsgTx
	var finalState [6]byte
	nextHeight := g.tip.Header.Height + 1
	if nextHeight > uint32(g.params.CoinbaseMaturity) {
		// Generate votes once the stake validation height has been
		// reached.
		if int64(nextHeight) >= g.params.StakeValidationHeight {
			// Generate and add the vote transactions for the
			// winning tickets to the stake tree.
			numVotes := g.params.TicketsPerBlock
			winners, stateHash, err := winningTickets(g.tip,
				g.liveTickets, numVotes)
			if err != nil {
				panic(err)
			}
			ticketWinners = winners
			for _, ticket := range winners {
				voteTx := g.createVoteTxFromTicket(g.tip, ticket)
				stakeTxns = append(stakeTxns, voteTx)
			}

			// Calculate the final lottery state hash for use in the
			// block header.
			finalState = calcFinalLotteryState(winners, stateHash)
		}

		// Generate ticket purchases (sstx) using the provided spendable
		// outputs.
		if ticketSpends != nil {
			const ticketFee = dcrutil.Amount(2)
			for i := 0; i < len(ticketSpends); i++ {
				out := &ticketSpends[i]
				purchaseTx := g.CreateTicketPurchaseTx(out,
					ticketPrice, ticketFee)
				stakeTxns = append(stakeTxns, purchaseTx)
			}
		}

		// Generate and add revocations for any missed tickets.
		for _, missedVote := range g.missedVotes {
			revocationTx := g.createRevocationTxFromTicket(missedVote)
			stakeTxns = append(stakeTxns, revocationTx)
		}
	}

	// Count the ticket purchases (sstx), votes (ssgen), and ticket revocations
	// (ssrtx), and calculate the total PoW fees generated by the stake
	// transactions.
	var numVotes uint16
	var numTicketRevocations uint8
	var numTicketPurchases uint8
	var stakeTreeFees dcrutil.Amount
	for _, tx := range stakeTxns {
		switch {
		case isVoteTx(tx):
			numVotes++
		case isTicketPurchaseTx(tx):
			numTicketPurchases++
		case isRevocationTx(tx):
			numTicketRevocations++
		}

		// Calculate any fees for the transaction.
		var inputSum, outputSum dcrutil.Amount
		for _, txIn := range tx.TxIn {
			inputSum += dcrutil.Amount(txIn.ValueIn)
		}
		for _, txOut := range tx.TxOut {
			outputSum += dcrutil.Amount(txOut.Value)
		}
		stakeTreeFees += (inputSum - outputSum)
	}

	// Generate and prepend a treasurybase transaction when the decentralized
	// treasury semantics are in effect.
	if g.treasurySemantics == TSDCP0006 && nextHeight > 1 {
		treasuryBaseTx := g.CreateTreasuryBaseTx(nextHeight, numVotes)
		stakeTxns = append([]*wire.MsgTx{treasuryBaseTx}, stakeTxns...)
	}

	// Create a standard coinbase and spending transaction.
	var regularTxns []*wire.MsgTx
	{
		// Create coinbase transaction for the block with no additional
		// dev or pow subsidy.
		coinbaseTx := g.CreateCoinbaseTx(nextHeight, numVotes)
		regularTxns = []*wire.MsgTx{coinbaseTx}

		// Increase the PoW subsidy to account for any fees in the stake
		// tree.
		coinbaseTx.TxOut[2].Value += int64(stakeTreeFees)

		// Create a transaction to spend the provided utxo if needed.
		if spend != nil {
			// Create the transaction with a fee of 1 atom for the
			// miner and increase the PoW subsidy accordingly.
			fee := dcrutil.Amount(1)
			coinbaseTx.TxOut[2].Value += int64(fee)

			// Create a transaction that spends from the provided
			// spendable output and includes an additional unique
			// OP_RETURN output to ensure the transaction ends up
			// with a unique hash, then add it to the list of
			// transactions to include in the block.  The script is
			// a simple OP_TRUE p2sh script in order to avoid the
			// need to track addresses and signature scripts in the
			// tests.
			spendTx := g.CreateSpendTx(spend, fee)
			regularTxns = append(regularTxns, spendTx)
		}
	}

	// Use a timestamp that is 7/8 of target timespan after the previous
	// block unless this is the first block in which case the current time
	// is used or the proof-of-work difficulty parameters have been adjusted
	// such that it's greater than the max 2 hours worth of blocks that can
	// be tested in which case one second is used.  This helps maintain the
	// retarget difficulty low as needed.  Also, ensure the timestamp is
	// limited to one second precision.
	var ts time.Time
	if nextHeight == 1 {
		ts = time.Now()
	} else {
		if g.params.WorkDiffWindowSize > 7200 {
			ts = g.tip.Header.Timestamp.Add(time.Second)
		} else {
			addDuration := g.params.TargetTimespan * 7 / 8
			ts = g.tip.Header.Timestamp.Add(addDuration)
		}
	}
	ts = time.Unix(ts.Unix(), 0)

	// Create the unsolved block.
	prevHash := g.tip.BlockHash()
	block := wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:      1,
			PrevBlock:    prevHash,
			MerkleRoot:   calcMerkleRoot(regularTxns),
			StakeRoot:    calcMerkleRoot(stakeTxns),
			VoteBits:     1,
			FinalState:   finalState,
			Voters:       numVotes,
			FreshStake:   numTicketPurchases,
			Revocations:  numTicketRevocations,
			PoolSize:     uint32(len(g.liveTickets)),
			Bits:         g.CalcNextRequiredDifficulty(),
			SBits:        int64(ticketPrice),
			Height:       nextHeight,
			Size:         0, // Filled in below.
			Timestamp:    ts,
			Nonce:        0, // To be solved.
			ExtraData:    [32]byte{},
			StakeVersion: 0,
		},
		Transactions:  regularTxns,
		STransactions: stakeTxns,
	}
	block.Header.Size = uint32(block.SerializeSize())

	// Perform any block munging just before solving.  Once stake validation
	// height has been reached, update the vote commitments accordingly if the
	// header height or previous hash was manually changed by a munge function.
	// Also, only recalculate the merkle roots and block size if they weren't
	// manually changed by a munge function.
	curMerkleRoot := block.Header.MerkleRoot
	curStakeRoot := block.Header.StakeRoot
	curSize := block.Header.Size
	curNonce := block.Header.Nonce
	for _, f := range mungers {
		f(&block)
	}
	if block.Header.Height != nextHeight || block.Header.PrevBlock != prevHash {
		if int64(nextHeight) >= g.params.StakeValidationHeight {
			updateVoteCommitments(&block)
		}
	}
	if block.Header.MerkleRoot == curMerkleRoot {
		block.Header.MerkleRoot = calcMerkleRoot(block.Transactions)
	}
	if block.Header.StakeRoot == curStakeRoot {
		block.Header.StakeRoot = calcMerkleRoot(block.STransactions)
	}
	if block.Header.Size == curSize {
		block.Header.Size = uint32(block.SerializeSize())
	}

	// Only solve the block if the nonce wasn't manually changed by a munge
	// function.
	if block.Header.Nonce == curNonce && !g.solveBlock(&block.Header) {
		panic(fmt.Sprintf("unable to solve block at height %d",
			block.Header.Height))
	}

	// Create stake tickets for the ticket purchases (sstx) in the block.  This
	// is done after the mungers to ensure all changes are accurately accounted
	// for.
	var ticketPurchases []*stakeTicket
	for txIdx, tx := range block.STransactions {
		if isTicketPurchaseTx(tx) {
			ticket := &stakeTicket{tx, nextHeight, uint32(txIdx)}
			ticketPurchases = append(ticketPurchases, ticket)
		}
	}

	// Update generator state and return the block.
	blockHash := block.BlockHash()
	if block.Header.PrevBlock != prevHash {
		// Save the original block this one was built from if it was
		// manually changed in a munger so the code which deals with
		// updating the live tickets when changing the tip has access to
		// it.
		g.originalParents[blockHash] = prevHash
	}
	g.addMissedVotes(block.STransactions, ticketWinners)
	g.connectRevocations(&blockHash, block.STransactions)
	g.connectLiveTickets(&blockHash, nextHeight, ticketWinners,
		ticketPurchases)
	g.blocks[blockHash] = &block
	g.blockHeights[blockHash] = nextHeight
	g.blocksByName[blockName] = &block
	g.blockNames[blockHash] = blockName
	g.tip = &block
	g.tipName = blockName
	return &block
}

// CreateBlockOne generates the first block of the chain with the required
// payouts.  The additional amount parameter can be used to create a block that
// is otherwise a completely valid block one except it adds the extra amount to
// each payout and thus create a block that violates consensus.
func (g *Generator) CreateBlockOne(blockName string, additionalAmount dcrutil.Amount, mungers ...func(*wire.MsgBlock)) *wire.MsgBlock {
	coinbaseTx := wire.NewMsgTx()
	coinbaseTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         0, // Updated below.
		BlockHeight:     wire.NullBlockHeight,
		BlockIndex:      wire.NullBlockIndex,
		SignatureScript: coinbaseSigScript,
	})

	// Add each required output and tally the total payouts for the coinbase
	// in order to set the input value appropriately.
	var totalSubsidy dcrutil.Amount
	for _, payout := range g.params.BlockOneLedger {
		coinbaseTx.AddTxOut(&wire.TxOut{
			Value:    payout.Amount + int64(additionalAmount),
			Version:  payout.ScriptVersion,
			PkScript: payout.Script,
		})

		totalSubsidy += dcrutil.Amount(payout.Amount)
	}
	coinbaseTx.TxIn[0].ValueIn = int64(totalSubsidy)

	// Generate the block with the specially created regular transactions.
	munger := func(b *wire.MsgBlock) {
		b.Transactions = []*wire.MsgTx{coinbaseTx}
	}
	mungers = append([]func(*wire.MsgBlock){munger}, mungers...)
	return g.NextBlock(blockName, nil, nil, mungers...)
}

// UpdateBlockState manually updates the generator state to remove all internal
// map references to a block via its old hash and insert new ones for the new
// block hash.  This is useful if the test code has to manually change a block
// after 'NextBlock' has returned.
func (g *Generator) UpdateBlockState(oldBlockName string, oldBlockHash chainhash.Hash, newBlockName string, newBlock *wire.MsgBlock) {
	// Remove existing entries.
	wonTickets := g.wonTickets[oldBlockHash]
	existingHeight := g.blockHeights[oldBlockHash]
	delete(g.blocks, oldBlockHash)
	delete(g.blockHeights, oldBlockHash)
	delete(g.blocksByName, oldBlockName)
	delete(g.blockNames, oldBlockHash)
	delete(g.wonTickets, oldBlockHash)

	// Add new entries.
	newBlockHash := newBlock.BlockHash()
	g.blocks[newBlockHash] = newBlock
	g.blockHeights[newBlockHash] = existingHeight
	g.blocksByName[newBlockName] = newBlock
	g.blockNames[newBlockHash] = newBlockName
	g.wonTickets[newBlockHash] = wonTickets
}

// OldestCoinbaseOuts removes the oldest set of coinbase proof-of-work outputs
// that was previously saved to the generator and returns the set as a slice.
func (g *Generator) OldestCoinbaseOuts() []SpendableOut {
	outs := g.spendableOuts[0]
	g.spendableOuts = g.spendableOuts[1:]
	return outs
}

// NumSpendableCoinbaseOuts returns the number of proof-of-work outputs that
// were previously saved to the generator but have not yet been collected.
func (g *Generator) NumSpendableCoinbaseOuts() int {
	return len(g.spendableOuts)
}

// saveCoinbaseOutsOriginal adds the proof-of-work outputs of the coinbase tx in
// the passed block to the list of spendable outputs using the original treasury
// semantics in effect at initial launch.
func (g *Generator) saveCoinbaseOutsOriginal(b *wire.MsgBlock) {
	g.spendableOuts = append(g.spendableOuts, []SpendableOut{
		MakeSpendableOut(b, 0, 2),
		MakeSpendableOut(b, 0, 3),
		MakeSpendableOut(b, 0, 4),
		MakeSpendableOut(b, 0, 5),
		MakeSpendableOut(b, 0, 6),
		MakeSpendableOut(b, 0, 7),
	})
	g.prevCollectedHash = b.BlockHash()
}

// saveCoinbaseOutsTreasury adds the proof-of-work outputs of the coinbase tx in
// the passed block to the list of spendable outputs using the decentralized
// treasury semantics introduced by DCP0006.
func (g *Generator) saveCoinbaseOutsTreasury(b *wire.MsgBlock) {
	g.spendableOuts = append(g.spendableOuts, []SpendableOut{
		MakeSpendableOut(b, 0, 1),
		MakeSpendableOut(b, 0, 2),
		MakeSpendableOut(b, 0, 3),
		MakeSpendableOut(b, 0, 4),
		MakeSpendableOut(b, 0, 5),
		MakeSpendableOut(b, 0, 6),
	})
	g.prevCollectedHash = b.BlockHash()
}

// saveCoinbaseOuts adds the proof-of-work outputs of the coinbase tx in the
// passed block to the list of spendable outputs using the treasury semantics
// configured for the generator.
func (g *Generator) saveCoinbaseOuts(b *wire.MsgBlock) {
	switch g.treasurySemantics {
	case TSOriginal:
		g.saveCoinbaseOutsOriginal(b)
		return

	case TSDCP0006:
		g.saveCoinbaseOutsTreasury(b)
		return
	}

	panic(fmt.Sprintf("unsupported treasury semantics %d", g.treasurySemantics))
}

// SaveTipCoinbaseOuts adds the proof-of-work outputs of the coinbase tx in the
// current tip block to the list of spendable outputs using the treasury
// semantics configured for the generator.
func (g *Generator) SaveTipCoinbaseOuts() {
	g.saveCoinbaseOuts(g.tip)
}

// SaveTipCoinbaseOutsWithTreasury adds the proof-of-work outputs of the
// coinbase tx in the current tip block to the list of spendable outputs.
//
// Deprecated: Use [Generator.SaveTipCoinbaseOuts] with [Generator.UseTreasurySemantics] instead.
func (g *Generator) SaveTipCoinbaseOutsWithTreasury() {
	g.saveCoinbaseOutsTreasury(g.tip)
}

// SaveSpendableCoinbaseOuts adds all proof-of-work coinbase outputs starting
// from the block after the last block that had its coinbase outputs collected
// and ending at the current tip using the treasury semantics configured for the
// generator.  This is useful to batch the collection of the outputs once the
// tests reach a stable point so they don't have to manually add them for the
// right tests which will ultimately end up being the best chain.
func (g *Generator) SaveSpendableCoinbaseOuts() {
	// Loop through the ancestors of the current tip until the
	// reaching the block that has already had the coinbase outputs
	// collected.
	var collectBlocks []*wire.MsgBlock
	for b := g.tip; b != nil; b = g.blocks[b.Header.PrevBlock] {
		if b.BlockHash() == g.prevCollectedHash {
			break
		}
		collectBlocks = append(collectBlocks, b)
	}
	for i := range collectBlocks {
		g.saveCoinbaseOuts(collectBlocks[len(collectBlocks)-1-i])
	}
}

// dupSpendableOuts returns a deep copy of the passed spendable outputs.
func dupSpendableOuts(spendableOuts [][]SpendableOut) [][]SpendableOut {
	result := make([][]SpendableOut, 0, len(spendableOuts))
	for _, outs := range spendableOuts {
		outsCopy := make([]SpendableOut, len(outs))
		copy(outsCopy, outs)
		result = append(result, outsCopy)
	}
	return result
}

// SnapshotCoinbaseOuts saves the current state of the spendable coinbase
// outputs with the given snapshot name so that it can be restored later by
// name.  This is primarily useful for tests that want to be able to undo a
// bunch of blocks via invalidation and reset the spendable outputs back to a
// matching known good state.
//
// This will panic if the specified snapshot name already exists.
func (g *Generator) SnapshotCoinbaseOuts(snapName string) {
	if _, ok := g.spendableOutSnaps[snapName]; ok {
		panic(fmt.Sprintf("snapshot name %s already exists", snapName))
	}
	g.spendableOutSnaps[snapName] = spendableOutsSnap{
		spendableOuts:     dupSpendableOuts(g.spendableOuts),
		prevCollectedHash: g.prevCollectedHash,
	}
}

// RestoreCoinbaseOutsSnapshot restores the state of the spendable coinbase
// outputs saved with the given snapshot name.  This is primarily useful for
// tests that want to be able to undo a bunch of blocks via invalidation and
// reset the spendable outputs back to a matching known good state.
//
// This will panic if the specified snapshot name does not exist.
func (g *Generator) RestoreCoinbaseOutsSnapshot(snapName string) {
	snap, ok := g.spendableOutSnaps[snapName]
	if !ok {
		panic(fmt.Sprintf("snapshot name %s does not exist", snapName))
	}
	g.spendableOuts = dupSpendableOuts(snap.spendableOuts)
	g.prevCollectedHash = snap.prevCollectedHash
}

// AssertTipHeight panics if the current tip block associated with the generator
// does not have the specified height.
func (g *Generator) AssertTipHeight(expected uint32) {
	height := g.tip.Header.Height
	if height != expected {
		panic(fmt.Sprintf("height for block %q is %d instead of "+
			"expected %d", g.tipName, height, expected))
	}
}

// countSigOps returns the number of signature operations in a script as
// determined by [txscript.GetSigOpCount] using the treasury semantics
// configured for the generator.
func (g Generator) countSigOps(script []byte, height uint32) int {
	isTreasuryEnabled := g.treasurySemantics == TSDCP0006 && height > 1
	return txscript.GetSigOpCount(script, isTreasuryEnabled)
}

// AssertScriptSigOpsCount panics if the provided script does not have the
// specified number of signature operations using the treasury semantics
// configured for the generator.
func (g *Generator) AssertScriptSigOpsCount(script []byte, expected int) {
	numSigOps := g.countSigOps(script, g.tip.Header.Height)
	if numSigOps != expected {
		_, file, line, _ := runtime.Caller(1)
		panic(fmt.Sprintf("assertion failed at %s:%d: generated number "+
			"of sigops for script is %d instead of expected %d",
			file, line, numSigOps, expected))
	}
}

// countBlockSigOps returns the number of legacy signature operations in the
// scripts in the passed block using the treasury semantics configured for the
// generator.
func (g *Generator) countBlockSigOps(block *wire.MsgBlock) int {
	blockHeight := block.Header.Height
	totalSigOps := 0
	for _, tx := range block.Transactions {
		for _, txIn := range tx.TxIn {
			numSigOps := g.countSigOps(txIn.SignatureScript, blockHeight)
			totalSigOps += numSigOps
		}
		for _, txOut := range tx.TxOut {
			numSigOps := g.countSigOps(txOut.PkScript, blockHeight)
			totalSigOps += numSigOps
		}
	}

	return totalSigOps
}

// AssertTipBlockSigOpsCount panics if the current tip block associated with the
// generator does not have the specified number of signature operations.
func (g *Generator) AssertTipBlockSigOpsCount(expected int) {
	numSigOps := g.countBlockSigOps(g.tip)
	if numSigOps != expected {
		panic(fmt.Sprintf("generated number of sigops for block %q "+
			"(height %d) is %d instead of expected %d", g.tipName,
			g.tip.Header.Height, numSigOps, expected))
	}
}

// AssertTipBlockSize panics if the current tip block associated with the
// generator does not have the specified size when serialized.
func (g *Generator) AssertTipBlockSize(expected int) {
	serializeSize := g.tip.SerializeSize()
	if serializeSize != expected {
		panic(fmt.Sprintf("block size of block %q (height %d) is %d "+
			"instead of expected %d", g.tipName,
			g.tip.Header.Height, serializeSize, expected))
	}
}

// AssertTipBlockNumTxns panics if the number of transactions in the current tip
// block associated with the generator does not match the specified value.
func (g *Generator) AssertTipBlockNumTxns(expected int) {
	numTxns := len(g.tip.Transactions)
	if numTxns != expected {
		panic(fmt.Sprintf("number of txns in block %q (height %d) is "+
			"%d instead of expected %d", g.tipName,
			g.tip.Header.Height, numTxns, expected))
	}
}

// AssertTipBlockHash panics if the current tip block associated with the
// generator does not match the specified hash.
func (g *Generator) AssertTipBlockHash(expected chainhash.Hash) {
	hash := g.tip.BlockHash()
	if hash != expected {
		panic(fmt.Sprintf("block hash of block %q (height %d) is %v "+
			"instead of expected %v", g.tipName,
			g.tip.Header.Height, hash, expected))
	}
}

// AssertTipBlockMerkleRoot panics if the merkle root in header of the current
// tip block associated with the generator does not match the specified hash.
func (g *Generator) AssertTipBlockMerkleRoot(expected chainhash.Hash) {
	hash := g.tip.Header.MerkleRoot
	if hash != expected {
		panic(fmt.Sprintf("merkle root of block %q (height %d) is %v "+
			"instead of expected %v", g.tipName,
			g.tip.Header.Height, hash, expected))
	}
}

// AssertTipBlockStakeRoot panics if the stake root in header of the current
// tip block associated with the generator does not match the specified hash.
func (g *Generator) AssertTipBlockStakeRoot(expected chainhash.Hash) {
	hash := g.tip.Header.StakeRoot
	if hash != expected {
		panic(fmt.Sprintf("stake root of block %q (height %d) is %v "+
			"instead of expected %v", g.tipName,
			g.tip.Header.Height, hash, expected))
	}
}

// AssertTipBlockTxOutOpReturn panics if the current tip block associated with
// the generator does not have an OP_RETURN script for the transaction output at
// the provided tx index and output index.
func (g *Generator) AssertTipBlockTxOutOpReturn(txIndex, txOutIndex uint32) {
	if txIndex >= uint32(len(g.tip.Transactions)) {
		panic(fmt.Sprintf("transaction index %d in block %q "+
			"(height %d) does not exist", txIndex, g.tipName,
			g.tip.Header.Height))
	}

	tx := g.tip.Transactions[txIndex]
	if txOutIndex >= uint32(len(tx.TxOut)) {
		panic(fmt.Sprintf("transaction index %d output %d in block %q "+
			"(height %d) does not exist", txIndex, txOutIndex,
			g.tipName, g.tip.Header.Height))
	}

	txOut := tx.TxOut[txOutIndex]
	if txOut.PkScript[0] != txscript.OP_RETURN {
		panic(fmt.Sprintf("transaction index %d output %d in block %q "+
			"(height %d) is not an OP_RETURN", txIndex, txOutIndex,
			g.tipName, g.tip.Header.Height))
	}
}

// AssertStakeVersion panics if the current tip block associated with the
// generator does not have the specified stake version in the header.
func (g *Generator) AssertStakeVersion(expected uint32) {
	stakeVersion := g.tip.Header.StakeVersion
	if stakeVersion != expected {
		panic(fmt.Sprintf("stake version for block %q is %d instead of "+
			"expected %d", g.tipName, stakeVersion, expected))
	}
}

// AssertBlockVersion panics if the current tip block associated with the
// generator does not have the specified block version in the header.
func (g *Generator) AssertBlockVersion(expected int32) {
	blockVersion := g.tip.Header.Version
	if blockVersion != expected {
		panic(fmt.Sprintf("block version for block %q is %d instead of "+
			"expected %d", g.tipName, blockVersion, expected))
	}
}

// AssertPoolSize panics if the current tip block associated with the generator
// does not indicate the specified pool size.
func (g *Generator) AssertPoolSize(expected uint32) {
	poolSize := g.tip.Header.PoolSize
	if poolSize != expected {
		panic(fmt.Sprintf("pool size for block %q is %d instead of expected "+
			"%d", g.tipName, poolSize, expected))
	}
}

// AssertBlockRevocationTx panics if the current tip block associated with the
// generator does not have a revocation at the specified transaction index
// provided.
func (g *Generator) AssertBlockRevocationTx(b *wire.MsgBlock, txIndex uint32) {
	if !isRevocationTx(b.STransactions[txIndex]) {
		panic(fmt.Sprintf("stake transaction at index %d in block %q is "+
			" not a revocation", txIndex, g.tipName))
	}
}

// AssertTipNumRevocations panics if the number of revocations in header of the
// current tip block associated with the generator does not match the specified
// value.
func (g *Generator) AssertTipNumRevocations(expected uint8) {
	numRevocations := g.tip.Header.Revocations
	if numRevocations != expected {
		panic(fmt.Sprintf("number of revocations in block %q (height "+
			"%d) is %d instead of expected %d", g.tipName,
			g.tip.Header.Height, numRevocations, expected))
	}
}

// AssertTipDisapprovesPrevious panics if the current tip block associated with
// the generator does not disapprove the previous block.
func (g *Generator) AssertTipDisapprovesPrevious() {
	if g.tip.Header.VoteBits&voteBitYes == 1 {
		panic(fmt.Sprintf("block %q (height %d) does not disapprove prev block",
			g.tipName, g.tip.Header.Height))
	}
}
