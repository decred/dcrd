// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"container/heap"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/blockchain/v4"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/gcs/v3/blockcf2"
	"github.com/decred/dcrd/txscript/v3"
	"github.com/decred/dcrd/wire"
)

var (
	// zeroHash is the zero value hash (all zeros).  It is defined as a
	// convenience.
	zeroHash chainhash.Hash

	// opTrueScript is a simple public key script that contains the OP_TRUE
	// opcode.  It is defined here to reduce garbage creation.
	opTrueScript = []byte{txscript.OP_TRUE}
)

const (
	// MinHighPriority is the minimum priority value that allows a
	// transaction to be considered high priority.
	MinHighPriority = dcrutil.AtomsPerCoin * 144.0 / 250
)

// Config is a descriptor containing the mining configuration.
type Config struct {
	// Policy houses the policy (configuration parameters) which is used to control
	// the generation of block templates.
	Policy *Policy

	// TxSource represents a source of transactions to consider for inclusion in
	// new blocks.
	TxSource TxSource

	// TimeSource defines the median time source which is used to retrieve the
	// current time adjusted by the median time offset.  This is used when setting
	// the timestamp in the header of new blocks.
	TimeSource blockchain.MedianTimeSource

	// SubsidyCache defines a subsidy cache to use when calculating and validating
	// block and vote subsidies.
	SubsidyCache *standalone.SubsidyCache

	// ChainParams identifies which chain parameters should be used while
	// generating block templates.
	ChainParams *chaincfg.Params

	// BlockManager provides methods for checking if the chain is synced, forcing
	// reorgs, and passing new work to the notification manager.
	BlockManager blockManagerFacade

	// MiningTimeOffset defines the number of seconds to offset the mining
	// timestamp of a block by (positive values are in the past).
	MiningTimeOffset int

	// BestSnapshot defines the function to use to access information about the
	// current best block.  The returned instance should be treated as immutable.
	BestSnapshot func() *blockchain.BestState

	// BlockByHash defines the function to use to search the internal chain block
	// stores and the database in an attempt to find the requested block and return
	// it.  This function should return blocks regardless of whether or not they
	// are part of the main chain.
	BlockByHash func(hash *chainhash.Hash) (*dcrutil.Block, error)

	// CalcNextRequiredDifficulty defines the function to use to calculate the
	// required difficulty for the block after the given block based on the
	// difficulty retarget rules.
	CalcNextRequiredDifficulty func(hash *chainhash.Hash, timestamp time.Time) (uint32, error)

	// CalcStakeVersionByHash defines the function to use to calculate the expected
	// stake version for the block AFTER the provided block hash.
	CalcStakeVersionByHash func(hash *chainhash.Hash) (uint32, error)

	// CheckConnectBlockTemplate defines the function to use to fully validate that
	// connecting the passed block to either the tip of the main chain or its
	// parent does not violate any consensus rules, aside from the proof of work
	// requirement.
	CheckConnectBlockTemplate func(block *dcrutil.Block) error

	// CheckTicketExhaustion defines the function to use to ensure that extending
	// the block associated with the provided hash with a block that contains the
	// specified number of ticket purchases will not result in a chain that is
	// unrecoverable due to inevitable ticket exhaustion.  This scenario happens
	// when the number of live tickets drops below the number of tickets that is
	// needed to reach the next block at which any outstanding immature ticket
	// purchases that would provide the necessary live tickets mature.
	CheckTicketExhaustion func(hash *chainhash.Hash, ticketPurchases uint8) error

	// CheckTransactionInputs defines the function to use to perform a series of
	// checks on the inputs to a transaction to ensure they are valid.
	CheckTransactionInputs func(tx *dcrutil.Tx, txHeight int64, view *blockchain.UtxoViewpoint, checkFraudProof bool, isTreasuryEnabled bool) (int64, error)

	// CheckTSpendHasVotes defines the function to use to check whether the given
	// tspend has enough votes to be included in a block AFTER the specified block.
	CheckTSpendHasVotes func(prevHash chainhash.Hash, tspend *dcrutil.Tx) error

	// CountSigOps defines the function to use to count the number of signature
	// operations for all transaction input and output scripts in the provided
	// transaction.
	CountSigOps func(tx *dcrutil.Tx, isCoinBaseTx bool, isSSGen bool, isTreasuryEnabled bool) int

	// FetchUtxoView defines the function to use to fetch unspent transaction
	// output information.  The returned instance should be treated as immutable.
	FetchUtxoView func(tx *dcrutil.Tx, includeRegularTxns bool) (*blockchain.UtxoViewpoint, error)

	// FetchUtxoViewParentTemplate defines the function to use to fetch unspent
	// transaction output information from the point of view of just having
	// connected the given block, which must be a block template that connects to
	// the parent of the tip of the main chain.  In other words, the given block
	// must be a sibling of the current tip of the main chain.
	//
	// This should typically only be used by mining code when it is unable to
	// generate a template that extends the current tip due to being unable to
	// acquire the minimum required number of votes to extend it.
	//
	// The returned instance should be treated as immutable.
	FetchUtxoViewParentTemplate func(block *wire.MsgBlock) (*blockchain.UtxoViewpoint, error)

	// ForceHeadReorganization defines the function to use to force a
	// reorganization of the block chain to the block hash requested, so long as it
	// matches up with the current organization of the best chain.
	ForceHeadReorganization func(formerBest chainhash.Hash, newBest chainhash.Hash) error

	// IsFinalizedTransaction defines the function to use to determine whether or
	// not a transaction is finalized.
	IsFinalizedTransaction func(tx *dcrutil.Tx, blockHeight int64, blockTime time.Time) bool

	// IsHeaderCommitmentsAgendaActive defines the function to use to determine
	// whether or not the header commitments agenda is active or not for the block
	// AFTER the given block.
	IsHeaderCommitmentsAgendaActive func(prevHash *chainhash.Hash) (bool, error)

	// IsTreasuryAgendaActive defines the function to use to determine if the
	// treasury agenda is active or not for the block AFTER the given block.
	IsTreasuryAgendaActive func(prevHash *chainhash.Hash) (bool, error)

	// MaxTreasuryExpenditure defines the function to use to get the maximum amount
	// of funds that can be spent from the treasury by a set of TSpends for a block
	// that extends the given block hash.  The function should return 0 if it is
	// called on an invalid TVI.
	MaxTreasuryExpenditure func(preTVIBlock *chainhash.Hash) (int64, error)

	// NewUtxoViewpoint defines the function to use to create a new empty unspent
	// transaction output view.
	NewUtxoViewpoint func() *blockchain.UtxoViewpoint

	// TipGeneration defines the function to use to get the entire generation of
	// blocks stemming from the parent of the current tip.
	TipGeneration func() ([]chainhash.Hash, error)

	// ValidateTransactionScripts defines the function to use to validate the
	// scripts for the passed transaction.
	ValidateTransactionScripts func(tx *dcrutil.Tx, utxoView *blockchain.UtxoViewpoint, flags txscript.ScriptFlags) error
}

// TxDesc is a descriptor about a transaction in a transaction source along with
// additional metadata.
type TxDesc struct {
	// Tx is the transaction associated with the entry.
	Tx *dcrutil.Tx

	// Type is the type of the transaction associated with the entry.
	Type stake.TxType

	// Added is the time when the entry was added to the source pool.
	Added time.Time

	// Height is the block height when the entry was added to the source
	// pool.
	Height int64

	// Fee is the total fee the transaction associated with the entry pays.
	Fee int64

	// TotalSigOps is the total signature operations for this transaction.
	TotalSigOps int

	// TxSize is the size of the transaction.
	TxSize int64
}

// TxAncestorStats is a descriptor that stores aggregated statistics for the
// unconfirmed ancestors of a transasction.
type TxAncestorStats struct {
	// Fees is the sum of all fees of unconfirmed ancestors.
	Fees int64

	// SizeBytes is the total size of all unconfirmed ancestors.
	SizeBytes int64

	// TotalSigOps is the total number of signature operations of all ancestors.
	TotalSigOps int

	// NumAncestors is the total number of ancestors for a given transaction.
	NumAncestors int

	// NumDescendants is the total number of descendants that have ancestor
	// statistics tracked for a given transaction.
	NumDescendants int
}

// VoteDesc is a descriptor about a vote transaction in a transaction source
// along with additional metadata.
type VoteDesc struct {
	VoteHash       chainhash.Hash
	TicketHash     chainhash.Hash
	ApprovesParent bool
}

const (
	// generatedBlockVersion is the version of the block being generated for
	// the main network.  It is defined as a constant here rather than using
	// the wire.BlockVersion constant since a change in the block version
	// will require changes to the generated block.  Using the wire constant
	// for generated block version could allow creation of invalid blocks
	// for the updated version.
	generatedBlockVersion = 8

	// generatedBlockVersionTest is the version of the block being generated
	// for networks other than the main and simulation networks.
	generatedBlockVersionTest = 9

	// blockHeaderOverhead is the max number of bytes it takes to serialize
	// a block header and max possible transaction count.
	blockHeaderOverhead = wire.MaxBlockHeaderPayload + wire.MaxVarIntPayload

	// coinbaseFlags is some extra data appended to the coinbase script
	// sig.
	coinbaseFlags = "/dcrd/"

	// kilobyte is the size of a kilobyte.
	kilobyte = 1000
)

// containsTx is a helper function that checks to see if a list of transactions
// contains any of the TxIns of some transaction.
func containsTxIns(txs []*dcrutil.Tx, tx *dcrutil.Tx) bool {
	for _, txToCheck := range txs {
		for _, txIn := range tx.MsgTx().TxIn {
			if txIn.PreviousOutPoint.Hash.IsEqual(txToCheck.Hash()) {
				return true
			}
		}
	}

	return false
}

// blockWithNumVotes is a block with the number of votes currently present
// for that block. Just used for sorting.
type blockWithNumVotes struct {
	Hash     chainhash.Hash
	NumVotes uint16
}

// byNumberOfVotes implements sort.Interface to sort a slice of blocks by their
// number of votes.
type byNumberOfVotes []*blockWithNumVotes

// Len returns the number of elements in the slice.  It is part of the
// sort.Interface implementation.
func (b byNumberOfVotes) Len() int {
	return len(b)
}

// Swap swaps the elements at the passed indices.  It is part of the
// sort.Interface implementation.
func (b byNumberOfVotes) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

// Less returns whether the block with index i should sort before the block with
// index j.  It is part of the sort.Interface implementation.
func (b byNumberOfVotes) Less(i, j int) bool {
	return b[i].NumVotes < b[j].NumVotes
}

// SortParentsByVotes takes a list of block header hashes and sorts them
// by the number of votes currently available for them in the votes map of
// mempool.  It then returns all blocks that are eligible to be used (have
// at least a majority number of votes) sorted by number of votes, descending.
//
// This function is safe for concurrent access.
func SortParentsByVotes(txSource TxSource, currentTopBlock chainhash.Hash, blocks []chainhash.Hash, params *chaincfg.Params) []chainhash.Hash {
	// Return now when no blocks were provided.
	lenBlocks := len(blocks)
	if lenBlocks == 0 {
		return nil
	}

	// Fetch the vote metadata for the provided block hashes from the
	// mempool and filter out any blocks that do not have the minimum
	// required number of votes.
	minVotesRequired := (params.TicketsPerBlock / 2) + 1
	voteMetadata := txSource.VotesForBlocks(blocks)
	filtered := make([]*blockWithNumVotes, 0, lenBlocks)
	for i := range blocks {
		numVotes := uint16(len(voteMetadata[i]))
		if numVotes >= minVotesRequired {
			filtered = append(filtered, &blockWithNumVotes{
				Hash:     blocks[i],
				NumVotes: numVotes,
			})
		}
	}

	// Return now if there are no blocks with enough votes to be eligible to
	// build on top of.
	if len(filtered) == 0 {
		return nil
	}

	// Blocks with the most votes appear at the top of the list.
	sort.Sort(sort.Reverse(byNumberOfVotes(filtered)))
	sortedUsefulBlocks := make([]chainhash.Hash, 0, len(filtered))
	for _, bwnv := range filtered {
		sortedUsefulBlocks = append(sortedUsefulBlocks, bwnv.Hash)
	}

	// Make sure we don't reorganize the chain needlessly if the top block has
	// the same amount of votes as the current leader after the sort. After this
	// point, all blocks listed in sortedUsefulBlocks definitely also have the
	// minimum number of votes required.
	curVoteMetadata := txSource.VotesForBlocks([]chainhash.Hash{currentTopBlock})
	numTopBlockVotes := uint16(len(curVoteMetadata))
	if filtered[0].NumVotes == numTopBlockVotes && filtered[0].Hash !=
		currentTopBlock {

		// Attempt to find the position of the current block being built
		// from in the list.
		pos := 0
		for i, bwnv := range filtered {
			if bwnv.Hash == currentTopBlock {
				pos = i
				break
			}
		}

		// Swap the top block into the first position. We directly access
		// sortedUsefulBlocks useful blocks here with the assumption that
		// since the values were accumulated from filtered, they should be
		// in the same positions and we shouldn't be able to access anything
		// out of bounds.
		if pos != 0 {
			sortedUsefulBlocks[0], sortedUsefulBlocks[pos] =
				sortedUsefulBlocks[pos], sortedUsefulBlocks[0]
		}
	}

	return sortedUsefulBlocks
}

// BlockTemplate houses a block that has yet to be solved along with additional
// details about the fees and the number of signature operations for each
// transaction in the block.
type BlockTemplate struct {
	// Block is a block that is ready to be solved by miners.  Thus, it is
	// completely valid with the exception of satisfying the proof-of-work
	// requirement.
	Block *wire.MsgBlock

	// Fees contains the amount of fees each transaction in the generated
	// template pays in base units.  Since the first transaction is the
	// coinbase, the first entry (offset 0) will contain the negative of the
	// sum of the fees of all other transactions.
	Fees []int64

	// SigOpCounts contains the number of signature operations each
	// transaction in the generated template performs.
	SigOpCounts []int64

	// Height is the height at which the block template connects to the main
	// chain.
	Height int64

	// ValidPayAddress indicates whether or not the template coinbase pays
	// to an address or is redeemable by anyone.  See the documentation on
	// NewBlockTemplate for details on which this can be useful to generate
	// templates without a coinbase payment address.
	ValidPayAddress bool
}

// mergeUtxoView adds all of the entries in viewB to viewA.  The result is that
// viewA will contain all of its original entries plus all of the entries
// in viewB.  It will replace any entries in viewB which also exist in viewA
// if the entry in viewA is fully spent.
func mergeUtxoView(viewA *blockchain.UtxoViewpoint, viewB *blockchain.UtxoViewpoint) {
	viewAEntries := viewA.Entries()
	for hash, entryB := range viewB.Entries() {
		if entryA, exists := viewAEntries[hash]; !exists ||
			entryA == nil || entryA.IsFullySpent() {
			viewAEntries[hash] = entryB
		}
	}
}

// hashExistsInList checks if a hash exists in a list of hash pointers.
func hashInSlice(h chainhash.Hash, list []chainhash.Hash) bool {
	for i := range list {
		if h == list[i] {
			return true
		}
	}

	return false
}

// txIndexFromTxList returns a transaction's index in a list, or -1 if it
// can not be found.
func txIndexFromTxList(hash chainhash.Hash, list []*dcrutil.Tx) int {
	for i, tx := range list {
		h := tx.Hash()
		if hash == *h {
			return i
		}
	}

	return -1
}

// standardCoinbaseOpReturn creates a standard OP_RETURN output to insert into
// coinbase. This function autogenerates the extranonce. The OP_RETURN pushes
// 12 bytes.
func standardCoinbaseOpReturn(height uint32) ([]byte, error) {
	extraNonce, err := wire.RandomUint64()
	if err != nil {
		return nil, err
	}

	enData := make([]byte, 12)
	binary.LittleEndian.PutUint32(enData[0:4], height)
	binary.LittleEndian.PutUint64(enData[4:12], extraNonce)
	extraNonceScript, err := txscript.GenerateProvablyPruneableOut(enData)
	if err != nil {
		return nil, err
	}

	return extraNonceScript, nil
}

// standardTreasurybaseOpReturn creates a standard OP_RETURN output to insert
// into a treasurybase. This function autogenerates the extranonce. The
// OP_RETURN pushes 12 bytes.
func standardTreasurybaseOpReturn(height uint32) ([]byte, error) {
	extraNonce, err := wire.RandomUint64()
	if err != nil {
		return nil, err
	}

	enData := make([]byte, 12)
	binary.LittleEndian.PutUint32(enData[0:4], height)
	binary.LittleEndian.PutUint64(enData[4:12], extraNonce)
	extraNonceScript, err := txscript.GenerateProvablyPruneableOut(enData)
	if err != nil {
		return nil, err
	}

	return extraNonceScript, nil
}

// calcBlockMerkleRoot calculates and returns a merkle root depending on the
// result of the header commitments agenda vote.  In particular, before the
// agenda is active, it returns the merkle root of the regular transaction tree.
// Once the agenda is active, it returns the combined merkle root for the
// regular and stake transaction trees in accordance with DCP0005.
func calcBlockMerkleRoot(regularTxns, stakeTxns []*wire.MsgTx, hdrCmtActive bool) chainhash.Hash {
	if !hdrCmtActive {
		return standalone.CalcTxTreeMerkleRoot(regularTxns)
	}

	return standalone.CalcCombinedTxTreeMerkleRoot(regularTxns, stakeTxns)
}

// calcBlockCommitmentRootV1 calculates and returns the required v1 block and
// the previous output scripts it references as inputs.
func calcBlockCommitmentRootV1(block *wire.MsgBlock, prevScripts blockcf2.PrevScripter) (chainhash.Hash, error) {
	filter, err := blockcf2.Regular(block, prevScripts)
	if err != nil {
		return chainhash.Hash{}, err
	}
	return blockchain.CalcCommitmentRootV1(filter.Hash()), nil
}

// createCoinbaseTx returns a coinbase transaction paying an appropriate subsidy
// based on the passed block height to the provided address.  When the address
// is nil, the coinbase transaction will instead be redeemable by anyone.
//
// See the comment for NewBlockTemplate for more information about why the nil
// address handling is useful.
func createCoinbaseTx(subsidyCache *standalone.SubsidyCache, coinbaseScript []byte, opReturnPkScript []byte, nextBlockHeight int64, addr dcrutil.Address, voters uint16, params *chaincfg.Params, isTreasuryEnabled bool) (*dcrutil.Tx, error) {
	// Coinbase transactions have no inputs, so previous outpoint is zero hash
	// and max index.
	coinbaseInput := &wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
		Sequence:        wire.MaxTxInSequenceNum,
		BlockHeight:     wire.NullBlockHeight,
		BlockIndex:      wire.NullBlockIndex,
		SignatureScript: coinbaseScript,
	}

	// Block one is a special block that might pay out tokens to a ledger.
	if nextBlockHeight == 1 && len(params.BlockOneLedger) != 0 {
		tx := wire.NewMsgTx()
		tx.Version = 1
		tx.AddTxIn(coinbaseInput)
		tx.TxIn[0].ValueIn = params.BlockOneSubsidy()

		for _, payout := range params.BlockOneLedger {
			tx.AddTxOut(&wire.TxOut{
				Value:    payout.Amount,
				Version:  payout.ScriptVersion,
				PkScript: payout.Script,
			})
		}

		return dcrutil.NewTx(tx), nil
	}

	// Create the script to pay to the provided payment address if one was
	// specified.  Otherwise create a script that allows the coinbase to be
	// redeemable by anyone.
	workSubsidyScript := opTrueScript
	if addr != nil {
		var err error
		workSubsidyScript, err = txscript.PayToAddrScript(addr)
		if err != nil {
			return nil, err
		}
	}

	// Prior to the decentralized treasury agenda, the transaction version must
	// be 1 and there is an additional output that either pays to organization
	// associated with the treasury or a provably pruneable zero-value output
	// script when it is disabled.
	//
	// Once the decentralized treasury agenda is active, the transaction version
	// must be the new expected version and there is no treasury output since it
	// is included in the stake tree instead.
	var txVersion = uint16(1)
	var treasuryOutput *wire.TxOut
	var treasurySubsidy int64
	if !isTreasuryEnabled {
		if params.BlockTaxProportion > 0 {
			// Create the treasury output with the correct subsidy and public
			// key script for the organization associated with the treasury.
			treasurySubsidy = subsidyCache.CalcTreasurySubsidy(nextBlockHeight,
				voters, isTreasuryEnabled)
			treasuryOutput = &wire.TxOut{
				Value:    treasurySubsidy,
				PkScript: params.OrganizationPkScript,
			}
		} else {
			// Treasury disabled.
			treasuryOutput = &wire.TxOut{
				Value:    0,
				PkScript: opTrueScript,
			}
		}
	} else {
		// Set the transaction version to the new version required by the
		// decentralized treasury agenda.
		txVersion = wire.TxVersionTreasury
	}

	// Create a coinbase with expected inputs and outputs.
	//
	// Inputs:
	//  - A single input with input value set to the total payout amount.
	//
	// Outputs:
	//  - Potential treasury output prior to the decentralized treasury agenda
	//  - Output that includes the block height and potential extra nonce used
	//    to ensure a unique hash
	//  - Output that pays the work subsidy to the miner
	workSubsidy := subsidyCache.CalcWorkSubsidy(nextBlockHeight, voters)
	tx := wire.NewMsgTx()
	tx.Version = txVersion
	tx.AddTxIn(coinbaseInput)
	tx.TxIn[0].ValueIn = workSubsidy + treasurySubsidy
	if treasuryOutput != nil {
		tx.AddTxOut(treasuryOutput)
	}
	tx.AddTxOut(&wire.TxOut{
		Value:    0,
		PkScript: opReturnPkScript,
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    workSubsidy,
		PkScript: workSubsidyScript,
	})
	return dcrutil.NewTx(tx), nil
}

// createTreasuryBaseTx returns a treasurybase transaction paying an appropriate
// subsidy based on the passed block height to the treasury.
func createTreasuryBaseTx(subsidyCache *standalone.SubsidyCache, nextBlockHeight int64, voters uint16) (*dcrutil.Tx, error) {
	// Create provably pruneable script for the output that encodes the block
	// height used to ensure a unique overall transaction hash.  This is
	// necessary because neither the input nor the output that adds to the
	// treasury account balance are unique for a treasurybase.
	opReturnTreasury, err := standardTreasurybaseOpReturn(uint32(nextBlockHeight))
	if err != nil {
		return nil, err
	}

	// Create a treasurybase with expected inputs and outputs.
	//
	// Inputs:
	//  - A single input with input value set to the total payout amount.
	//
	// Outputs:
	//  - Treasury output that adds to the treasury account balance
	//  - Output that includes the block height to ensure a unique hash
	//
	// Note that all treasurybase transactions require TxVersionTreasury and
	// they must be in the stake transaction tree.
	const withTreasury = true
	trsySubsidy := subsidyCache.CalcTreasurySubsidy(nextBlockHeight, voters,
		withTreasury)
	tx := wire.NewMsgTx()
	tx.Version = wire.TxVersionTreasury
	tx.AddTxIn(&wire.TxIn{
		// Treasurybase transactions have no inputs, so previous outpoint
		// is zero hash and max index.
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
		Sequence:        wire.MaxTxInSequenceNum,
		BlockHeight:     wire.NullBlockHeight,
		BlockIndex:      wire.NullBlockIndex,
		SignatureScript: nil, // Must be nil by consensus.
	})
	tx.TxIn[0].ValueIn = trsySubsidy
	tx.AddTxOut(&wire.TxOut{
		Value:    trsySubsidy,
		Version:  0,
		PkScript: []byte{txscript.OP_TADD},
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    0,
		PkScript: opReturnTreasury,
	})
	retTx := dcrutil.NewTx(tx)
	retTx.SetTree(wire.TxTreeStake)
	return retTx, nil
}

// spendTransaction updates the passed view by marking the inputs to the passed
// transaction as spent.  It also adds all outputs in the passed transaction
// which are not provably unspendable as available unspent transaction outputs.
func spendTransaction(utxoView *blockchain.UtxoViewpoint, tx *dcrutil.Tx, height int64, isTreasuryEnabled bool) {
	for _, txIn := range tx.MsgTx().TxIn {
		originHash := &txIn.PreviousOutPoint.Hash
		originIndex := txIn.PreviousOutPoint.Index
		entry := utxoView.LookupEntry(originHash)
		if entry != nil {
			entry.SpendOutput(originIndex)
		}
	}

	utxoView.AddTxOuts(tx, height, wire.NullBlockIndex, isTreasuryEnabled)
}

// logSkippedDeps logs any dependencies which are also skipped as a result of
// skipping a transaction while generating a block template at the trace level.
func logSkippedDeps(tx *dcrutil.Tx, deps []*TxDesc) {
	if deps == nil {
		return
	}

	for _, item := range deps {
		log.Tracef("Skipping tx %s since it depends on %s\n",
			item.Tx.Hash(), tx.Hash())
	}
}

// minimumMedianTime returns the minimum allowed timestamp for a block building
// on the end of the current best chain.  In particular, it is one second after
// the median timestamp of the last several blocks per the chain consensus
// rules.
func minimumMedianTime(best *blockchain.BestState) time.Time {
	return best.MedianTime.Add(time.Second)
}

// medianAdjustedTime returns the current time adjusted to ensure it is at least
// one second after the median timestamp of the last several blocks per the
// chain consensus rules.
func (g *BlkTmplGenerator) medianAdjustedTime() time.Time {
	// The timestamp for the block must not be before the median timestamp
	// of the last several blocks.  Thus, choose the maximum between the
	// current time and one second after the past median time.  The current
	// timestamp is truncated to a second boundary before comparison since a
	// block timestamp does not support a precision greater than one second.
	best := g.cfg.BestSnapshot()
	newTimestamp := g.cfg.TimeSource.AdjustedTime()
	minTimestamp := minimumMedianTime(best)
	if newTimestamp.Before(minTimestamp) {
		newTimestamp = minTimestamp
	}

	// Adjust by the amount requested from the command line argument.
	newTimestamp = newTimestamp.Add(
		time.Duration(-g.cfg.MiningTimeOffset) * time.Second)

	return newTimestamp
}

// maybeInsertStakeTx checks to make sure that a stake tx is
// valid from the perspective of the mainchain (not necessarily
// the mempool or block) before inserting into a tx tree.
// If it fails the check, it returns false; otherwise true.
func (g *BlkTmplGenerator) maybeInsertStakeTx(stx *dcrutil.Tx, treeValid bool, isTreasuryEnabled bool) bool {
	missingInput := false

	view, err := g.cfg.FetchUtxoView(stx, treeValid)
	if err != nil {
		log.Warnf("Unable to fetch transaction store for "+
			"stx %s: %v", stx.Hash(), err)
		return false
	}
	mstx := stx.MsgTx()
	isSSGen := stake.IsSSGen(mstx, isTreasuryEnabled)
	var isTSpend, isTreasuryBase bool
	if isTreasuryEnabled {
		isTSpend = stake.IsTSpend(mstx)
		isTreasuryBase = stake.IsTreasuryBase(mstx)
	}
	for i, txIn := range mstx.TxIn {
		// Evaluate if this is a stakebase or treasury base input or
		// not. If it is, continue without evaluation of the input.
		if (i == 0 && (isSSGen || isTreasuryBase)) || isTSpend {
			txIn.BlockHeight = wire.NullBlockHeight
			txIn.BlockIndex = wire.NullBlockIndex
			continue
		}

		originHash := &txIn.PreviousOutPoint.Hash
		utxIn := view.LookupEntry(originHash)
		if utxIn == nil {
			missingInput = true
			break
		} else {
			originIdx := txIn.PreviousOutPoint.Index
			txIn.ValueIn = utxIn.AmountByIndex(originIdx)
			txIn.BlockHeight = uint32(utxIn.BlockHeight())
			txIn.BlockIndex = utxIn.BlockIndex()
		}
	}
	return !missingInput
}

// handleTooFewVoters handles the situation in which there are too few voters on
// of the blockchain. If there are too few voters and a cached parent template to
// work off of is present, it will return a copy of that template to pass to the
// miner.
// Safe for concurrent access.
func (g *BlkTmplGenerator) handleTooFewVoters(nextHeight int64, miningAddress dcrutil.Address, isTreasuryEnabled bool) (*BlockTemplate, error) {
	stakeValidationHeight := g.cfg.ChainParams.StakeValidationHeight

	// Handle not enough voters being present if we're set to mine aggressively
	// (default behavior).
	best := g.cfg.BestSnapshot()
	if nextHeight >= stakeValidationHeight && g.cfg.Policy.AggressiveMining {
		// Fetch the latest block and head and begin working off of it with an
		// empty transaction tree regular and the contents of that stake tree.
		// In the future we should have the option of reading some transactions
		// from this block, too.
		topBlock, err := g.cfg.BlockByHash(&best.Hash)
		if err != nil {
			str := fmt.Sprintf("unable to get tip block %s", best.PrevHash)
			return nil, miningRuleError(ErrGetTopBlock, str)
		}
		tipHeader := &topBlock.MsgBlock().Header

		// Start with a copy of the tip block header.
		var block wire.MsgBlock
		block.Header = *tipHeader

		// Create and populate a new coinbase.
		coinbaseScript := make([]byte, len(coinbaseFlags)+2)
		copy(coinbaseScript[2:], coinbaseFlags)
		opReturnPkScript, err := standardCoinbaseOpReturn(tipHeader.Height)
		if err != nil {
			return nil, err
		}
		coinbaseTx, err := createCoinbaseTx(g.cfg.SubsidyCache, coinbaseScript,
			opReturnPkScript, topBlock.Height(), miningAddress,
			tipHeader.Voters, g.cfg.ChainParams, isTreasuryEnabled)
		if err != nil {
			return nil, err
		}
		block.AddTransaction(coinbaseTx.MsgTx())

		if isTreasuryEnabled {
			treasuryBase, err := createTreasuryBaseTx(g.cfg.SubsidyCache,
				topBlock.Height(), tipHeader.Voters)
			if err != nil {
				return nil, err
			}
			block.AddSTransaction(treasuryBase.MsgTx())
		}

		// Copy all of the stake transactions over.
		for i, stx := range topBlock.STransactions() {
			if i == 0 && isTreasuryEnabled {
				// Skip copying treasurybase.
				continue
			}
			block.AddSTransaction(stx.MsgTx())
		}

		// Set a fresh timestamp.
		ts := g.medianAdjustedTime()
		block.Header.Timestamp = ts

		// If we're on testnet, the time since this last block listed as the
		// parent must be taken into consideration.
		if g.cfg.ChainParams.ReduceMinDifficulty {
			parentHash := topBlock.MsgBlock().Header.PrevBlock

			requiredDifficulty, err := g.cfg.CalcNextRequiredDifficulty(&parentHash, ts)
			if err != nil {
				return nil, miningRuleError(ErrGettingDifficulty,
					err.Error())
			}

			block.Header.Bits = requiredDifficulty
		}

		// Recalculate the size.
		block.Header.Size = uint32(block.SerializeSize())

		bt := &BlockTemplate{
			Block:           &block,
			Fees:            []int64{0},
			SigOpCounts:     []int64{0},
			Height:          int64(tipHeader.Height),
			ValidPayAddress: miningAddress != nil,
		}

		// Calculate the merkle root depending on the result of the header
		// commitments agenda vote.
		prevHash := &tipHeader.PrevBlock
		hdrCmtActive, err := g.cfg.IsHeaderCommitmentsAgendaActive(prevHash)
		if err != nil {
			return nil, err
		}
		header := &block.Header
		header.MerkleRoot = calcBlockMerkleRoot(block.Transactions, block.STransactions, hdrCmtActive)

		// Calculate the stake root or commitment root depending on the result
		// of the header commitments agenda vote.
		var cmtRoot chainhash.Hash
		if hdrCmtActive {
			// Load all of the previous output scripts the block references as
			// inputs since they are needed to create the filter commitment.
			blockUtxos, err := g.cfg.FetchUtxoViewParentTemplate(&block)
			if err != nil {
				str := fmt.Sprintf("failed to fetch inputs when making new "+
					"block template: %v", err)
				return nil, miningRuleError(ErrFetchTxStore, str)
			}

			cmtRoot, err = calcBlockCommitmentRootV1(&block, blockUtxos)
			if err != nil {
				str := fmt.Sprintf("failed to calculate commitment root for "+
					"block when making new block template: %v", err)
				return nil, miningRuleError(ErrCalcCommitmentRoot, str)
			}
		} else {
			cmtRoot = standalone.CalcTxTreeMerkleRoot(block.STransactions)
		}
		header.StakeRoot = cmtRoot

		// Make sure the block validates.
		btBlock := dcrutil.NewBlockDeepCopyCoinbase(&block)
		err = g.cfg.CheckConnectBlockTemplate(btBlock)
		if err != nil {
			str := fmt.Sprintf("failed to check template: %v while "+
				"constructing a new parent", err.Error())
			return nil, miningRuleError(ErrCheckConnectBlock, str)
		}

		return bt, nil
	}

	log.Debugf("Not enough voters on top block to generate " +
		"new block template")

	return nil, nil
}

// BlkTmplGenerator generates block templates based on a given mining policy
// and a transactions source. It also houses additional state required in
// order to ensure the templates adhere to the consensus rules and are built
// on top of the best chain tip or its parent if the best chain tip is
// unable to get enough votes.
//
// See the NewBlockTemplate method for a detailed description of how the block
// template is generated.
type BlkTmplGenerator struct {
	cfg *Config
}

// NewBlkTmplGenerator returns a new block template generator for the given
// policy using transactions from the provided transaction source.
func NewBlkTmplGenerator(cfg *Config) *BlkTmplGenerator {
	return &BlkTmplGenerator{cfg: cfg}
}

// calcFeePerKb returns an adjusted fee per kilobyte taking the provided
// transaction and its ancestors into account.
func calcFeePerKb(txDesc *TxDesc, ancestorStats *TxAncestorStats) float64 {
	txSize := txDesc.Tx.MsgTx().SerializeSize()
	return (float64(txDesc.Fee+ancestorStats.Fees) * float64(kilobyte)) /
		float64(int64(txSize)+ancestorStats.SizeBytes)
}

// NewBlockTemplate returns a new block template that is ready to be solved
// using the transactions from the passed transaction source pool and a coinbase
// that either pays to the passed address if it is not nil, or a coinbase that
// is redeemable by anyone if the passed address is nil.  The nil address
// functionality is useful since there are cases such as the getblocktemplate
// RPC where external mining software is responsible for creating their own
// coinbase which will replace the one generated for the block template.  Thus
// the need to have configured address can be avoided.
//
// The transactions selected and included are prioritized according to several
// factors.  First, each transaction has a priority calculated based on its
// value, age of inputs, and size.  Transactions which consist of larger
// amounts, older inputs, and small sizes have the highest priority.  Second, a
// fee per kilobyte is calculated for each transaction.  Transactions with a
// higher fee per kilobyte are preferred.  Finally, the block generation related
// policy settings are all taken into account.
//
// Transactions which only spend outputs from other transactions already in the
// block chain are immediately added to a priority queue which either
// prioritizes based on the priority (then fee per kilobyte) or the fee per
// kilobyte (then priority) depending on whether or not the BlockPrioritySize
// policy setting allots space for high-priority transactions.  Transactions
// which spend outputs from other transactions in the source pool are added to a
// dependency map so they can be added to the priority queue once the
// transactions they depend on have been included.
//
// Once the high-priority area (if configured) has been filled with
// transactions, or the priority falls below what is considered high-priority,
// the priority queue is updated to prioritize by fees per kilobyte (then
// priority).
//
// When the fees per kilobyte drop below the TxMinFreeFee policy setting, the
// transaction will be skipped unless the BlockMinSize policy setting is
// nonzero, in which case the block will be filled with the low-fee/free
// transactions until the block size reaches that minimum size.
//
// Any transactions which would cause the block to exceed the BlockMaxSize
// policy setting, exceed the maximum allowed signature operations per block, or
// otherwise cause the block to be invalid are skipped.
//
// Given the above, a block generated by this function is of the following form:
//
//   -----------------------------------  --  --
//  |      Coinbase Transaction         |   |   |
//  |-----------------------------------|   |   |
//  |                                   |   |   | ----- policy.BlockPrioritySize
//  |   High-priority Transactions      |   |   |
//  |                                   |   |   |
//  |-----------------------------------|   | --
//  |                                   |   |
//  |                                   |   |
//  |                                   |   |--- (policy.BlockMaxSize) / 2
//  |  Transactions prioritized by fee  |   |
//  |  until <= policy.TxMinFreeFee     |   |
//  |                                   |   |
//  |                                   |   |
//  |                                   |   |
//  |-----------------------------------|   |
//  |  Low-fee/Non high-priority (free) |   |
//  |  transactions (while block size   |   |
//  |  <= policy.BlockMinSize)          |   |
//   -----------------------------------  --
//
// Which also includes a stake tree that looks like the following:
//
//   -----------------------------------  --  --
//  |                                   |   |   |
//  |             Votes                 |   |   | --- >= (chaincfg.TicketsPerBlock/2) + 1
//  |                                   |   |   |
//  |-----------------------------------|   | --
//  |                                   |   |   |
//  |            Tickets                |   |   | --- <= chaincfg.MaxFreshStakePerBlock
//  |                                   |   |   |
//  |-----------------------------------|   | --
//  |                                   |   |
//  |          Revocations              |   |
//  |                                   |   |
//   -----------------------------------  --
//
//  This function returns nil, nil if there are not enough voters on any of
//  the current top blocks to create a new block template.
func (g *BlkTmplGenerator) NewBlockTemplate(payToAddress dcrutil.Address) (*BlockTemplate, error) {
	// All transaction scripts are verified using the more strict standard
	// flags.
	scriptFlags, err := g.cfg.Policy.StandardVerifyFlags()
	if err != nil {
		return nil, err
	}

	// Extend the most recently known best block.
	// The most recently known best block is the top block that has the most
	// ssgen votes for it. We only need this after the height in which stake voting
	// has kicked in.
	// To figure out which block has the most ssgen votes, we need to run the
	// following algorithm:
	// 1. Acquire the HEAD block and all of its orphans. Record their block header
	// hashes.
	// 2. Create a map of [blockHeaderHash] --> [mempoolTxnList].
	// 3. for blockHeaderHash in candidateBlocks:
	//		if mempoolTx.StakeDesc == SSGen &&
	//			mempoolTx.SSGenParseBlockHeader() == blockHeaderHash:
	//			map[blockHeaderHash].append(mempoolTx)
	// 4. Check len of each map entry and store.
	// 5. Query the ticketdb and check how many eligible ticket holders there are
	//    for the given block you are voting on.
	// 6. Divide #ofvotes (len(map entry)) / totalPossibleVotes --> penalty ratio
	// 7. Store penalty ratios for all block candidates.
	// 8. Select the one with the largest penalty ratio (highest block reward).
	//    This block is then selected to build upon instead of the others, because
	//    it yields the greater amount of rewards.

	best := g.cfg.BestSnapshot()
	prevHash := best.Hash
	nextBlockHeight := best.Height + 1
	stakeValidationHeight := g.cfg.ChainParams.StakeValidationHeight

	isTreasuryEnabled, err := g.cfg.IsTreasuryAgendaActive(&prevHash)
	if err != nil {
		return nil, err
	}

	var (
		isTVI            bool
		maxTreasurySpend int64
	)
	if isTreasuryEnabled {
		isTVI = standalone.IsTreasuryVoteInterval(uint64(nextBlockHeight),
			g.cfg.ChainParams.TreasuryVoteInterval)
	}

	if nextBlockHeight >= stakeValidationHeight {
		// Obtain the entire generation of blocks stemming from this parent.
		children, err := g.cfg.TipGeneration()
		if err != nil {
			return nil, miningRuleError(ErrFailedToGetGeneration, err.Error())
		}

		// Get the list of blocks that we can actually build on top of. If we're
		// not currently on the block that has the most votes, switch to that
		// block.
		eligibleParents := SortParentsByVotes(g.cfg.TxSource, prevHash, children,
			g.cfg.ChainParams)
		if len(eligibleParents) == 0 {
			log.Debugf("Too few voters found on any HEAD block, " +
				"recycling a parent block to mine on")
			return g.handleTooFewVoters(nextBlockHeight, payToAddress,
				isTreasuryEnabled)
		}

		log.Debugf("Found eligible parent %v with enough votes to build "+
			"block on, proceeding to create a new block template",
			eligibleParents[0])

		// Force a reorganization to the parent with the most votes if we need
		// to.
		if eligibleParents[0] != prevHash {
			for i := range eligibleParents {
				newHead := &eligibleParents[i]
				err := g.cfg.BlockManager.ForceReorganization(prevHash, *newHead)
				if err != nil {
					log.Errorf("failed to reorganize to new parent: %v", err)
					continue
				}

				// Check to make sure we actually have the transactions
				// (votes) we need in the mempool.
				voteHashes := g.cfg.TxSource.VoteHashesForBlock(newHead)
				if len(voteHashes) == 0 {
					return nil, fmt.Errorf("no vote metadata for block %v",
						newHead)
				}

				if exist := g.cfg.TxSource.HaveAllTransactions(voteHashes); !exist {
					continue
				} else {
					prevHash = *newHead
					break
				}
			}
		}

		// Obtain the maximum allowed treasury expenditure.
		if isTreasuryEnabled && isTVI {
			maxTreasurySpend, err = g.cfg.MaxTreasuryExpenditure(&prevHash)
			if err != nil {
				return nil, err
			}
		}
	}

	// Get the current source transactions and create a priority queue to
	// hold the transactions which are ready for inclusion into a block
	// along with some priority related and fee metadata.  Reserve the same
	// number of items that are available for the priority queue.  Also,
	// choose the initial sort order for the priority queue based on whether
	// or not there is an area allocated for high-priority transactions.
	miningView := g.cfg.TxSource.MiningView()
	sourceTxns := miningView.TxDescs()
	sortedByFee := g.cfg.Policy.BlockPrioritySize == 0
	lessFunc := txPQByStakeAndFeeAndThenPriority
	if sortedByFee {
		lessFunc = txPQByStakeAndFee
	}
	priorityQueue := newTxPriorityQueue(len(sourceTxns), lessFunc)
	prioritizedTxns := make(map[chainhash.Hash]struct{}, len(sourceTxns))

	// Create a slice to hold the transactions to be included in the
	// generated block with reserved space.  Also create a utxo view to
	// house all of the input transactions so multiple lookups can be
	// avoided.
	blockTxns := make([]*dcrutil.Tx, 0, len(sourceTxns))
	blockUtxos := g.cfg.NewUtxoViewpoint()

	// Create slices to hold the fees and number of signature operations
	// for each of the selected transactions and add an entry for the
	// coinbase.  This allows the code below to simply append details about
	// a transaction as it is selected for inclusion in the final block.
	// However, since the total fees aren't known yet, use a dummy value for
	// the coinbase fee which will be updated later.
	txFees := make([]int64, 0, len(sourceTxns))
	txFeesMap := make(map[chainhash.Hash]int64)
	txSigOpCounts := make([]int64, 0, len(sourceTxns))
	txSigOpCountsMap := make(map[chainhash.Hash]int64)
	txFees = append(txFees, -1) // Updated once known

	log.Debugf("Considering %d transactions for inclusion to new block",
		len(sourceTxns))
	knownDisapproved := g.cfg.TxSource.IsRegTxTreeKnownDisapproved(&prevHash)

	// Tracks the total number of transactions that depend on another
	// from the tx source.
	totalDescendantTxns := 0
	prioItemMap := make(map[chainhash.Hash]*txPrioItem, len(sourceTxns))

mempoolLoop:
	for _, txDesc := range sourceTxns {
		// A block can't have more than one coinbase or contain
		// non-finalized transactions.
		tx := txDesc.Tx
		msgTx := tx.MsgTx()
		if standalone.IsCoinBaseTx(msgTx, isTreasuryEnabled) {
			log.Tracef("Skipping coinbase tx %s", tx.Hash())
			continue
		}
		if !g.cfg.IsFinalizedTransaction(tx, nextBlockHeight, best.MedianTime) {
			log.Tracef("Skipping non-finalized tx %s", tx.Hash())
			continue
		}

		// Need this for a check below for stake base input, and to check
		// the ticket number.
		isSSGen := txDesc.Type == stake.TxTypeSSGen
		if isSSGen {
			blockHash, blockHeight := stake.SSGenBlockVotedOn(msgTx)
			if !((blockHash == prevHash) &&
				(int64(blockHeight) == nextBlockHeight-1)) {
				log.Tracef("Skipping ssgen tx %s because it does "+
					"not vote on the correct block", tx.Hash())
				continue
			}
		}

		var isTSpend bool
		if isTreasuryEnabled {
			isTSpend = txDesc.Type == stake.TxTypeTSpend
		}

		// Fetch all of the utxos referenced by the this transaction.
		utxos, err := g.cfg.FetchUtxoView(tx, !knownDisapproved)
		if err != nil {
			log.Warnf("Unable to fetch utxo view for tx %s: "+
				"%v", tx.Hash(), err)
			continue
		}

		// Setup dependencies for any transactions which reference
		// other transactions in the mempool so they can be properly
		// ordered below.
		prioItem := &txPrioItem{txDesc: txDesc, txType: txDesc.Type}
		for i, txIn := range tx.MsgTx().TxIn {
			// Evaluate if this is a stakebase input or not. If it is, continue
			// without evaluation of the input.
			// if isStakeBase
			if (i == 0 && isSSGen) || isTSpend {
				continue
			}

			originHash := &txIn.PreviousOutPoint.Hash
			originIndex := txIn.PreviousOutPoint.Index
			utxoEntry := utxos.LookupEntry(originHash)
			if utxoEntry == nil || utxoEntry.IsOutputSpent(originIndex) {
				if !g.cfg.TxSource.HaveTransaction(originHash) {
					log.Tracef("Skipping tx %s because "+
						"it references unspent output "+
						"%s which is not available",
						tx.Hash(), txIn.PreviousOutPoint)
					continue mempoolLoop
				}
			}
		}

		// Calculate the final transaction priority using the input
		// value age sum as well as the adjusted transaction size.  The
		// formula is: sum(inputValue * inputAge) / adjustedTxSize
		prioItem.priority = CalcPriority(tx.MsgTx(), utxos,
			nextBlockHeight)

		// Calculate the fee in Atoms/KB.
		// NOTE: This is a more precise value than the one calculated
		// during calcMinRelayFee which rounds up to the nearest full
		// kilobyte boundary.  This is beneficial since it provides an
		// incentive to create smaller transactions.
		ancestorStats, hasStats := miningView.AncestorStats(tx.Hash())
		prioItem.feePerKB = calcFeePerKb(txDesc, ancestorStats)
		prioItem.fee = txDesc.Fee + ancestorStats.Fees
		prioItemMap[*tx.Hash()] = prioItem
		hasParents := miningView.hasParents(tx.Hash())

		if !hasParents || hasStats {
			heap.Push(priorityQueue, prioItem)
			prioritizedTxns[*tx.Hash()] = struct{}{}
			blockUtxos.AddTxOuts(tx, nextBlockHeight, wire.NullBlockIndex,
				isTreasuryEnabled)
		}

		if hasParents {
			totalDescendantTxns++
		}

		// Merge the referenced outputs from the input transactions to
		// this transaction into the block utxo view.  This allows the
		// code below to avoid a second lookup.
		mergeUtxoView(blockUtxos, utxos)
	}

	log.Tracef("Priority queue len %d, dependers len %d",
		priorityQueue.Len(), totalDescendantTxns)

	// The starting block size is the size of the block header plus the max
	// possible transaction count size, plus the size of the coinbase
	// transaction.
	blockSize := uint32(blockHeaderOverhead)

	// Guesstimate for sigops based on valid txs in loop below. This number
	// tends to overestimate sigops because of the way the loop below is
	// coded and the fact that tx can sometimes be removed from the tx
	// trees if they fail one of the stake checks below the priorityQueue
	// pop loop. This is buggy, but not catastrophic behaviour. A future
	// release should fix it. TODO
	blockSigOps := int64(0)
	totalFees := int64(0)

	numSStx := 0
	numTAdds := 0

	foundWinningTickets := make(map[chainhash.Hash]bool, len(best.NextWinningTickets))
	for _, ticketHash := range best.NextWinningTickets {
		foundWinningTickets[ticketHash] = false
	}

	// Maintain lookup of transactions that have been included in the block
	// template.
	templateTxnMap := make(map[chainhash.Hash]struct{})

	// Choose which transactions make it into the block.
nextPriorityQueueItem:
	for priorityQueue.Len() > 0 {
		// Grab the highest priority (or highest fee per kilobyte
		// depending on the sort order) transaction.
		prioItem := heap.Pop(priorityQueue).(*txPrioItem)
		tx := prioItem.txDesc.Tx
		delete(prioritizedTxns, *tx.Hash())

		if _, exist := templateTxnMap[*tx.Hash()]; exist {
			continue
		}

		// Store if this is an SStx or not.
		isSStx := prioItem.txType == stake.TxTypeSStx

		// Store if this is an SSGen or not.
		isSSGen := prioItem.txType == stake.TxTypeSSGen

		// Store if this is an SSRtx or not.
		isSSRtx := prioItem.txType == stake.TxTypeSSRtx

		var isTSpend, isTAdd bool
		if isTreasuryEnabled {
			// Store if this is an TSpend or not.
			isTSpend = prioItem.txType == stake.TxTypeTSpend

			// Store if this is a TAdd or not
			isTAdd = prioItem.txType == stake.TxTypeTAdd
		}

		// Grab the list of transactions which depend on this one (if any).
		deps := miningView.children(tx.Hash())

		// Skip TSpend if this block is not on a TVI or outside of the
		// Tspend window.
		if isTSpend {
			if !isTVI {
				log.Tracef("Skipping tspend %v because block "+
					"is not on a TVI: %v", tx.Hash(),
					nextBlockHeight)
				continue
			}

			// We are on a TVI, make sure this tspend is within the
			// window.
			exp := tx.MsgTx().Expiry
			if !standalone.InsideTSpendWindow(nextBlockHeight,
				exp, g.cfg.ChainParams.TreasuryVoteInterval,
				g.cfg.ChainParams.TreasuryVoteIntervalMultiplier) {

				log.Tracef("Skipping treasury spend %v at height %d because it "+
					"has an expiry of %d that is outside of the voting window",
					tx.Hash(), nextBlockHeight, exp)
				continue
			}

			// Ensure there are enough votes. Send in a fake block
			// with the proper height.
			err = g.cfg.CheckTSpendHasVotes(prevHash,
				dcrutil.NewTx(tx.MsgTx()))
			if err != nil {
				log.Tracef("Skipping tspend %v because it doesn't have enough "+
					"votes: height %v reason '%v'", tx.Hash(), nextBlockHeight, err)
				continue
			}

			// Ensure this TSpend won't overspend from the
			// treasury. This is currently considering between
			// TSpends by tx priority order, which might not be
			// ideal, but we don't expect this to be triggered for
			// mainnet. This could be improved by sorting approved
			// TSpends either by approval % or by expiry.
			tspendAmount := tx.MsgTx().TxIn[0].ValueIn
			if maxTreasurySpend-tspendAmount < 0 {
				log.Tracef("Skipping tspend %v because it spends "+
					"more than allowed: treasury %d tspend %d",
					tx.Hash(), maxTreasurySpend, tspendAmount)
				continue
			}
			maxTreasurySpend -= tspendAmount
		}

		// Skip if we already have too many TAdds.
		if isTAdd && numTAdds >= blockchain.MaxTAddsPerBlock {
			log.Tracef("Skipping tadd %s because it would exceed "+
				"the max number of tadds allowed in a block",
				tx.Hash())
			logSkippedDeps(tx, deps)
			continue
		}

		// Skip if we already have too many SStx.
		if isSStx && (numSStx >=
			int(g.cfg.ChainParams.MaxFreshStakePerBlock)) {
			log.Tracef("Skipping sstx %s because it would exceed "+
				"the max number of sstx allowed in a block", tx.Hash())
			logSkippedDeps(tx, deps)
			continue
		}

		// Skip if the SStx commit value is below the value required by the
		// stake diff.
		if isSStx && (tx.MsgTx().TxOut[0].Value < best.NextStakeDiff) {
			continue
		}

		// Skip all missed tickets that we've never heard of.
		if isSSRtx {
			ticketHash := &tx.MsgTx().TxIn[0].PreviousOutPoint.Hash

			if !hashInSlice(*ticketHash, best.MissedTickets) {
				continue
			}
		}

		if miningView.isRejected(tx.Hash()) {
			// If the transaction or any of its ancestors have been rejected,
			// discard the transaction.
			continue
		}

		ancestors := miningView.ancestors(tx.Hash())
		ancestorStats, _ := miningView.AncestorStats(tx.Hash())
		oldFee := prioItem.feePerKB
		prioItem.feePerKB = calcFeePerKb(prioItem.txDesc, ancestorStats)

		feeDecreased := oldFee > prioItem.feePerKB
		if feeDecreased && ancestorStats.NumAncestors == 0 {
			// If the fee decreased due to ancestors being included in the
			// template and the transaction has no parents, then enqueue it one
			// more time with an accurate feePerKb.
			heap.Push(priorityQueue, prioItem)
			prioritizedTxns[*tx.Hash()] = struct{}{}
			continue
		}

		if feeDecreased {
			// Skip the transaction if the total feePerKb decreased.
			// This addresses a vulnerability that would allow a low-fee
			// transaction to have an inflated and inaccurate feePerKb based on
			// ancestors that have already been included in the block template.
			// The transaction will be added back to the priority queue when all
			// parent transactions are included in the template.
			continue
		}

		// Enforce maximum block size.  Also check for overflow.
		txSize := uint32(tx.MsgTx().SerializeSize())
		blockPlusTxSize := blockSize + txSize + uint32(ancestorStats.SizeBytes)
		if blockPlusTxSize < blockSize ||
			blockPlusTxSize >= g.cfg.Policy.BlockMaxSize {
			log.Tracef("Skipping tx %s (size %v) because it "+
				"would exceed the max block size; cur block "+
				"size %v, cur num tx %v", tx.Hash(), txSize,
				blockSize, len(blockTxns))
			logSkippedDeps(tx, deps)
			miningView.reject(tx.Hash())
			continue
		}

		// Enforce maximum signature operations per block.  Also check
		// for overflow.
		numSigOps := int64(prioItem.txDesc.TotalSigOps)
		numSigOpsBundle := numSigOps + int64(ancestorStats.TotalSigOps)
		if blockSigOps+numSigOpsBundle < blockSigOps ||
			blockSigOps+numSigOpsBundle > blockchain.MaxSigOpsPerBlock {
			log.Tracef("Skipping tx %s because it would "+
				"exceed the maximum sigops per block", tx.Hash())
			logSkippedDeps(tx, deps)
			miningView.reject(tx.Hash())
			continue
		}

		// Check to see if the SSGen tx actually uses a ticket that is
		// valid for the next block.
		if isSSGen {
			if foundWinningTickets[tx.MsgTx().TxIn[1].PreviousOutPoint.Hash] {
				continue
			}
			msgTx := tx.MsgTx()
			isEligible := false
			for _, sstxHash := range best.NextWinningTickets {
				if sstxHash.IsEqual(&msgTx.TxIn[1].PreviousOutPoint.Hash) {
					isEligible = true
				}
			}

			if !isEligible {
				continue
			}
		}

		// Skip free transactions once the block is larger than the
		// minimum block size, except for stake transactions.
		if sortedByFee &&
			(prioItem.feePerKB < float64(g.cfg.Policy.TxMinFreeFee)) &&
			(tx.Tree() != wire.TxTreeStake) &&
			(blockPlusTxSize >= g.cfg.Policy.BlockMinSize) {

			log.Tracef("Skipping tx %s with feePerKB %.2f "+
				"< TxMinFreeFee %d and block size %d >= "+
				"minBlockSize %d", tx.Hash(), prioItem.feePerKB,
				g.cfg.Policy.TxMinFreeFee, blockPlusTxSize,
				g.cfg.Policy.BlockMinSize)
			logSkippedDeps(tx, deps)
			miningView.reject(tx.Hash())
			continue
		}

		// Prioritize by fee per kilobyte once the block is larger than
		// the priority size or there are no more high-priority
		// transactions.
		if !sortedByFee && (blockPlusTxSize >= g.cfg.Policy.BlockPrioritySize ||
			prioItem.priority <= MinHighPriority) {

			log.Tracef("Switching to sort by fees per "+
				"kilobyte blockSize %d >= BlockPrioritySize "+
				"%d || priority %.2f <= minHighPriority %.2f",
				blockPlusTxSize, g.cfg.Policy.BlockPrioritySize,
				prioItem.priority, MinHighPriority)

			sortedByFee = true
			priorityQueue.SetLessFunc(txPQByStakeAndFee)

			// Put the transaction back into the priority queue and
			// skip it so it is re-prioritized by fees if it won't
			// fit into the high-priority section or the priority is
			// too low.  Otherwise this transaction will be the
			// final one in the high-priority section, so just fall
			// though to the code below so it is added now.
			if blockPlusTxSize > g.cfg.Policy.BlockPrioritySize ||
				prioItem.priority < MinHighPriority {

				heap.Push(priorityQueue, prioItem)
				prioritizedTxns[*tx.Hash()] = struct{}{}
				continue
			}
		}

		txBundle := append(ancestors, prioItem.txDesc)
		for _, bundledTx := range txBundle {
			// Ensure the transaction inputs pass all of the necessary
			// preconditions before allowing it to be added to the block.
			// The fraud proof is not checked because it will be filled in
			// by the miner.
			_, err = g.cfg.CheckTransactionInputs(bundledTx.Tx, nextBlockHeight,
				blockUtxos, false, isTreasuryEnabled)
			if err != nil {
				log.Tracef("Skipping tx %s due to error in "+
					"CheckTransactionInputs: %v", bundledTx.Tx.Hash(), err)
				logSkippedDeps(bundledTx.Tx, deps)
				miningView.reject(bundledTx.Tx.Hash())
				continue nextPriorityQueueItem
			}
			err = g.cfg.ValidateTransactionScripts(bundledTx.Tx, blockUtxos, scriptFlags)
			if err != nil {
				log.Tracef("Skipping tx %s due to error in "+
					"ValidateTransactionScripts: %v", bundledTx.Tx.Hash(), err)
				logSkippedDeps(bundledTx.Tx, deps)
				miningView.reject(bundledTx.Tx.Hash())
				continue nextPriorityQueueItem
			}
		}

		for _, bundledTxDesc := range txBundle {
			bundledTx := bundledTxDesc.Tx
			bundledTxHash := bundledTx.Hash()

			// Spend the transaction inputs in the block utxo view and add
			// an entry for it to ensure any transactions which reference
			// this one have it available as an input and can ensure they
			// aren't double spending.
			spendTransaction(blockUtxos, bundledTxDesc.Tx, nextBlockHeight,
				isTreasuryEnabled)

			// Add the transaction to the block, increment counters, and
			// save the fees and signature operation counts to the block
			// template.
			blockTxns = append(blockTxns, bundledTx)
			blockSize += uint32(bundledTx.MsgTx().SerializeSize())
			bundledTxSigOps := int64(bundledTxDesc.TotalSigOps)
			blockSigOps += bundledTxSigOps

			// Accumulate the SStxs in the block, because only a certain number
			// are allowed.
			if bundledTxDesc.Type == stake.TxTypeSStx {
				numSStx++
			}
			if bundledTxDesc.Type == stake.TxTypeSSGen {
				foundWinningTickets[bundledTx.MsgTx().TxIn[1].PreviousOutPoint.Hash] = true
			}
			if isTreasuryEnabled && bundledTxDesc.Type == stake.TxTypeTAdd {
				numTAdds++
			}

			templateTxnMap[*bundledTxHash] = struct{}{}
			txFeesMap[*bundledTxHash] = bundledTxDesc.Fee
			txSigOpCountsMap[*bundledTxHash] = bundledTxSigOps

			bundledPrioItem := prioItemMap[*bundledTxHash]
			log.Tracef("Adding tx %s (priority %.2f, feePerKB %.2f)",
				bundledTxHash, bundledPrioItem.priority,
				bundledPrioItem.feePerKB)

			// Remove transaction from mining view since it's been added to the
			// block template.
			bundledTxDeps := miningView.children(bundledTxHash)
			miningView.RemoveTransaction(bundledTxHash, false)

			// Add transactions which depend on this one (and also do not
			// have any other unsatisfied dependencies) to the priority
			// queue.
			for _, childTx := range bundledTxDeps {
				childTxHash := childTx.Tx.Hash()
				if _, exist := prioritizedTxns[*childTxHash]; exist {
					continue
				}

				// Add the transaction to the priority queue if there are no
				// more dependencies after this one and the priority item for it
				// already exists.
				if !miningView.hasParents(childTxHash) {
					childPrioItem := prioItemMap[*childTxHash]
					if childPrioItem != nil {
						heap.Push(priorityQueue, childPrioItem)
						prioritizedTxns[*childTxHash] = struct{}{}
					}
				}
			}
		}
	}

	// Build tx list for stake tx.
	blockTxnsStake := make([]*dcrutil.Tx, 0, len(blockTxns))

	// Create a standard coinbase transaction paying to the provided
	// address.  NOTE: The coinbase value will be updated to include the
	// fees from the selected transactions later after they have actually
	// been selected.  It is created here to detect any errors early
	// before potentially doing a lot of work below.  The extra nonce helps
	// ensure the transaction is not a duplicate transaction (paying the
	// same value to the same public key address would otherwise be an
	// identical transaction for block version 1).
	// Decred: We need to move this downwards because of the requirements
	// to incorporate voters and potential voters.
	//
	// NOTE: we have to do this early to deal with stakebase.
	coinbaseScript := []byte{0x00, 0x00}
	coinbaseScript = append(coinbaseScript, []byte(coinbaseFlags)...)

	// Add a random coinbase nonce to ensure that tx prefix hash
	// so that our merkle root is unique for lookups needed for
	// getwork, etc.
	opReturnPkScript, err := standardCoinbaseOpReturn(uint32(nextBlockHeight))
	if err != nil {
		return nil, err
	}

	// Stake tx ordering in stake tree:
	// 1. Stakebase
	// 2. SSGen (votes).
	// 3. SStx (fresh stake tickets).
	// 4. SSRtx (revocations for missed tickets).
	// 5. Stuff treasury payout in stake tree.

	// We have to figure out how many voters we have before adding the
	// stakebase transaction.
	//
	// We should only find at most TicketsPerBlock votes per block, so
	// initialize the slices using that as a hint to avoid reallocations.
	voters := 0
	voteBitsVoters := make([]uint16, 0, g.cfg.ChainParams.TicketsPerBlock)
	votes := make([]*dcrutil.Tx, 0, g.cfg.ChainParams.TicketsPerBlock)

	// After SVH, we should add SSGens (votes) to the stake tree. Since we
	// need to figure out the number of voters before creating the
	// treasuryBase, we add the votes to an aux slice to be processed
	// later.
	if nextBlockHeight >= stakeValidationHeight {
		for _, tx := range blockTxns {
			msgTx := tx.MsgTx()

			if stake.IsSSGen(msgTx, isTreasuryEnabled) {
				txCopy := dcrutil.NewTxDeepTxIns(msgTx)
				if g.maybeInsertStakeTx(txCopy, !knownDisapproved,
					isTreasuryEnabled) {

					vb := stake.SSGenVoteBits(txCopy.MsgTx())
					voteBitsVoters = append(voteBitsVoters, vb)
					votes = append(votes, txCopy)
					voters++
				}
			}

			// Don't let this overflow, although probably it's impossible.
			if voters >= math.MaxUint16 {
				break
			}
		}
	}

	// If the treasury is enabled, it should be the first tx of the stake
	// tree.
	var treasuryBase *dcrutil.Tx
	if isTreasuryEnabled {
		treasuryBase, err = createTreasuryBaseTx(g.cfg.SubsidyCache,
			nextBlockHeight, uint16(voters))
		if err != nil {
			return nil, err
		}
		blockTxnsStake = append(blockTxnsStake, treasuryBase)
	}

	// Add the votes to blockTxnsStake since treasuryBase needs to come
	// first.
	blockTxnsStake = append(blockTxnsStake, votes...)

	// Set votebits, which determines whether the TxTreeRegular of the previous
	// block is valid or not.
	var votebits uint16
	if nextBlockHeight < stakeValidationHeight {
		votebits = uint16(0x0001) // TxTreeRegular enabled pre-staking
	} else {
		// Otherwise, we need to check the votes to determine if the tx tree was
		// validated or not.
		voteYea := 0
		totalVotes := 0

		for _, vb := range voteBitsVoters {
			if dcrutil.IsFlagSet16(vb, dcrutil.BlockValid) {
				voteYea++
			}
			totalVotes++
		}

		if voteYea == 0 { // Handle zero case for div by zero error prevention.
			votebits = uint16(0x0000) // TxTreeRegular disabled
		} else if (totalVotes / voteYea) <= 1 {
			votebits = uint16(0x0001) // TxTreeRegular enabled
		} else {
			votebits = uint16(0x0000) // TxTreeRegular disabled
		}
	}

	// Get the newly purchased tickets (SStx tx) and store them and their
	// number.
	freshStake := 0
	for _, tx := range blockTxns {
		msgTx := tx.MsgTx()
		if tx.Tree() == wire.TxTreeStake && stake.IsSStx(msgTx) {
			// A ticket can not spend an input from TxTreeRegular,
			// since it has not yet been validated.
			if containsTxIns(blockTxns, tx) {
				continue
			}

			// Quick check for difficulty here.
			if msgTx.TxOut[0].Value >= best.NextStakeDiff {
				txCopy := dcrutil.NewTxDeepTxIns(msgTx)
				if g.maybeInsertStakeTx(txCopy, !knownDisapproved,
					isTreasuryEnabled) {

					blockTxnsStake = append(blockTxnsStake, txCopy)
					freshStake++
				}
			}
		}

		// Don't let this overflow.
		if freshStake >= int(g.cfg.ChainParams.MaxFreshStakePerBlock) {
			break
		}
	}

	// Ensure that mining the block would not cause the chain to become
	// unrecoverable due to ticket exhaustion.
	err = g.cfg.CheckTicketExhaustion(&best.Hash, uint8(freshStake))
	if err != nil {
		log.Debug(err)
		return nil, miningRuleError(ErrTicketExhaustion, err.Error())
	}

	// Get the ticket revocations (SSRtx tx) and store them and their
	// number.
	revocations := 0
	for _, tx := range blockTxns {
		if nextBlockHeight < stakeValidationHeight {
			break // No SSRtx should be present before this height.
		}

		msgTx := tx.MsgTx()
		if tx.Tree() == wire.TxTreeStake && stake.IsSSRtx(msgTx) {
			txCopy := dcrutil.NewTxDeepTxIns(msgTx)
			if g.maybeInsertStakeTx(txCopy, !knownDisapproved,
				isTreasuryEnabled) {

				blockTxnsStake = append(blockTxnsStake, txCopy)
				revocations++
			}
		}

		// Don't let this overflow.
		if revocations >= math.MaxUint8 {
			break
		}
	}

	// Insert TAdd/TSpend transactions.
	if isTreasuryEnabled {
		for _, tx := range blockTxns {
			msgTx := tx.MsgTx()
			if tx.Tree() == wire.TxTreeStake && stake.IsTAdd(msgTx) {
				txCopy := dcrutil.NewTxDeepTxIns(msgTx)
				if g.maybeInsertStakeTx(txCopy, !knownDisapproved,
					isTreasuryEnabled) {

					blockTxnsStake = append(blockTxnsStake, txCopy)
					log.Tracef("maybeInsertStakeTx TADD %v ", tx.Hash())
				}
			} else if tx.Tree() == wire.TxTreeStake && stake.IsTSpend(msgTx) {
				txCopy := dcrutil.NewTxDeepTxIns(msgTx)
				if g.maybeInsertStakeTx(txCopy, !knownDisapproved,
					isTreasuryEnabled) {

					blockTxnsStake = append(blockTxnsStake, txCopy)
					log.Tracef("maybeInsertStakeTx TSPEND %v ", tx.Hash())
				}
			}
		}
	}

	coinbaseTx, err := createCoinbaseTx(g.cfg.SubsidyCache, coinbaseScript,
		opReturnPkScript, nextBlockHeight, payToAddress, uint16(voters),
		g.cfg.ChainParams, isTreasuryEnabled)
	if err != nil {
		return nil, err
	}
	coinbaseTx.SetTree(wire.TxTreeRegular)

	numCoinbaseSigOps := int64(g.cfg.CountSigOps(coinbaseTx, true,
		false, isTreasuryEnabled))
	blockSize += uint32(coinbaseTx.MsgTx().SerializeSize())
	blockSigOps += numCoinbaseSigOps
	txFeesMap[*coinbaseTx.Hash()] = 0
	txSigOpCountsMap[*coinbaseTx.Hash()] = numCoinbaseSigOps
	if treasuryBase != nil {
		txFeesMap[*treasuryBase.Hash()] = 0
		n := int64(g.cfg.CountSigOps(treasuryBase, true, false, isTreasuryEnabled))
		txSigOpCountsMap[*treasuryBase.Hash()] = n
	}

	// Build tx lists for regular tx.
	blockTxnsRegular := make([]*dcrutil.Tx, 0, len(blockTxns)+1)

	// Append coinbase.
	blockTxnsRegular = append(blockTxnsRegular, coinbaseTx)

	// Assemble the two transaction trees.
	for _, tx := range blockTxns {
		if tx.Tree() == wire.TxTreeRegular {
			blockTxnsRegular = append(blockTxnsRegular, tx)
		} else if tx.Tree() == wire.TxTreeStake {
			continue
		} else {
			log.Tracef("Error adding tx %s to block; invalid tree", tx.Hash())
			continue
		}
	}

	for _, tx := range blockTxnsRegular {
		fee, ok := txFeesMap[*tx.Hash()]
		if !ok {
			return nil, fmt.Errorf("couldn't find fee for tx %v",
				*tx.Hash())
		}
		totalFees += fee
		txFees = append(txFees, fee)

		tsos, ok := txSigOpCountsMap[*tx.Hash()]
		if !ok {
			return nil, fmt.Errorf("couldn't find sig ops count for tx %v",
				*tx.Hash())
		}
		txSigOpCounts = append(txSigOpCounts, tsos)
	}

	for _, tx := range blockTxnsStake {
		fee, ok := txFeesMap[*tx.Hash()]
		if !ok {
			return nil, fmt.Errorf("couldn't find fee for stx %v",
				*tx.Hash())
		}
		totalFees += fee
		txFees = append(txFees, fee)

		tsos, ok := txSigOpCountsMap[*tx.Hash()]
		if !ok {
			return nil, fmt.Errorf("couldn't find sig ops count for stx %v",
				*tx.Hash())
		}
		txSigOpCounts = append(txSigOpCounts, tsos)
	}

	// Scale the fees according to the number of voters once stake validation
	// height is reached.
	if nextBlockHeight >= stakeValidationHeight {
		totalFees *= int64(voters)
		totalFees /= int64(g.cfg.ChainParams.TicketsPerBlock)
	}

	txSigOpCounts = append(txSigOpCounts, numCoinbaseSigOps)

	// Now that the actual transactions have been selected, update the
	// block size for the real transaction count and coinbase value with
	// the total fees accordingly.
	if nextBlockHeight > 1 {
		blockSize -= wire.MaxVarIntPayload -
			uint32(wire.VarIntSerializeSize(uint64(len(blockTxnsRegular))+
				uint64(len(blockTxnsStake))))
		powOutputIdx := 2
		if isTreasuryEnabled {
			powOutputIdx = 1
		}
		coinbaseTx.MsgTx().TxOut[powOutputIdx].Value += totalFees
		txFees[0] = -totalFees
	}

	// Calculate the required difficulty for the block.  The timestamp
	// is potentially adjusted to ensure it comes after the median time of
	// the last several blocks per the chain consensus rules.
	ts := g.medianAdjustedTime()
	reqDifficulty, err := g.cfg.CalcNextRequiredDifficulty(&prevHash, ts)
	if err != nil {
		return nil, miningRuleError(ErrGettingDifficulty, err.Error())
	}

	// Return nil if we don't yet have enough voters; sometimes it takes a
	// bit for the mempool to sync with the votes map and we end up down
	// here despite having the relevant votes available in the votes map.
	minimumVotesRequired :=
		int((g.cfg.ChainParams.TicketsPerBlock / 2) + 1)
	if nextBlockHeight >= stakeValidationHeight &&
		voters < minimumVotesRequired {
		log.Warnf("incongruent number of voters in mempool " +
			"vs mempool.voters; not enough voters found")
		return g.handleTooFewVoters(nextBlockHeight, payToAddress,
			isTreasuryEnabled)
	}

	// Correct transaction index fraud proofs for any transactions that
	// are chains. maybeInsertStakeTx fills this in for stake transactions
	// already, so only do it for regular transactions.
	for i, tx := range blockTxnsRegular {
		// No need to check any of the transactions in the custom first
		// block.
		if nextBlockHeight == 1 {
			break
		}

		utxs, err := g.cfg.FetchUtxoView(tx, !knownDisapproved)
		if err != nil {
			str := fmt.Sprintf("failed to fetch input utxs for tx %v: %s",
				tx.Hash(), err.Error())
			return nil, miningRuleError(ErrFetchTxStore, str)
		}

		// Copy the transaction and swap the pointer.
		txCopy := dcrutil.NewTxDeepTxIns(tx.MsgTx())
		blockTxnsRegular[i] = txCopy
		tx = txCopy

		for _, txIn := range tx.MsgTx().TxIn {
			originHash := &txIn.PreviousOutPoint.Hash
			utx := utxs.LookupEntry(originHash)
			if utx == nil {
				// Set a flag with the index so we can properly set
				// the fraud proof below.
				txIn.BlockIndex = wire.NullBlockIndex
			} else {
				originIdx := txIn.PreviousOutPoint.Index
				txIn.ValueIn = utx.AmountByIndex(originIdx)
				txIn.BlockHeight = uint32(utx.BlockHeight())
				txIn.BlockIndex = utx.BlockIndex()
			}
		}
	}

	// Fill in locally referenced inputs.
	for i, tx := range blockTxnsRegular {
		// Skip coinbase.
		if i == 0 {
			continue
		}

		// Copy the transaction and swap the pointer.
		txCopy := dcrutil.NewTxDeepTxIns(tx.MsgTx())
		blockTxnsRegular[i] = txCopy
		tx = txCopy

		for _, txIn := range tx.MsgTx().TxIn {
			// This tx was at some point 0-conf and now requires the
			// correct block height and index. Set it here.
			if txIn.BlockIndex == wire.NullBlockIndex {
				idx := txIndexFromTxList(txIn.PreviousOutPoint.Hash,
					blockTxnsRegular)

				// The input is in the block, set it accordingly.
				if idx != -1 {
					originIdx := txIn.PreviousOutPoint.Index
					amt := blockTxnsRegular[idx].MsgTx().TxOut[originIdx].Value
					txIn.ValueIn = amt
					txIn.BlockHeight = uint32(nextBlockHeight)
					txIn.BlockIndex = uint32(idx)
				} else {
					str := fmt.Sprintf("failed find hash in tx list "+
						"for fraud proof; tx in hash %v",
						txIn.PreviousOutPoint.Hash)
					return nil, miningRuleError(ErrFraudProofIndex, str)
				}
			}
		}
	}

	// Choose the block version to generate based on the network.
	blockVersion := int32(generatedBlockVersion)
	if g.cfg.ChainParams.Net != wire.MainNet &&
		g.cfg.ChainParams.Net != wire.SimNet {

		blockVersion = generatedBlockVersionTest
	}

	// Figure out stake version.
	generatedStakeVersion, err := g.cfg.CalcStakeVersionByHash(&prevHash)
	if err != nil {
		return nil, err
	}

	// Create a new block ready to be solved.
	var msgBlock wire.MsgBlock
	msgBlock.Header = wire.BlockHeader{
		Version:   blockVersion,
		PrevBlock: prevHash,
		// MerkleRoot and StakeRoot set below.
		VoteBits:     votebits,
		FinalState:   best.NextFinalState,
		Voters:       uint16(voters),
		FreshStake:   uint8(freshStake),
		Revocations:  uint8(revocations),
		PoolSize:     best.NextPoolSize,
		Timestamp:    ts,
		SBits:        best.NextStakeDiff,
		Bits:         reqDifficulty,
		StakeVersion: generatedStakeVersion,
		Height:       uint32(nextBlockHeight),
		// Size declared below
	}

	for _, tx := range blockTxnsRegular {
		if err := msgBlock.AddTransaction(tx.MsgTx()); err != nil {
			return nil, miningRuleError(ErrTransactionAppend, err.Error())
		}
	}

	totalTreasuryOps := 0
	for _, tx := range blockTxnsStake {
		if err := msgBlock.AddSTransaction(tx.MsgTx()); err != nil {
			return nil, miningRuleError(ErrTransactionAppend, err.Error())
		}
		// While in this loop count treasury operations.
		if isTreasuryEnabled {
			if stake.IsTAdd(tx.MsgTx()) {
				totalTreasuryOps++
			} else if stake.IsTSpend(tx.MsgTx()) {
				totalTreasuryOps++
			}
		}
	}

	// Calculate the merkle root depending on the result of the header
	// commitments agenda vote.
	hdrCmtActive, err := g.cfg.IsHeaderCommitmentsAgendaActive(&prevHash)
	if err != nil {
		return nil, err
	}
	msgBlock.Header.MerkleRoot = calcBlockMerkleRoot(msgBlock.Transactions,
		msgBlock.STransactions, hdrCmtActive)

	// Calculate the stake root or commitment root depending on the result of
	// the header commitments agenda vote.
	var cmtRoot chainhash.Hash
	if hdrCmtActive {
		cmtRoot, err = calcBlockCommitmentRootV1(&msgBlock, blockUtxos)
		if err != nil {
			str := fmt.Sprintf("failed to calculate commitment root for block "+
				"when making new block template: %v", err)
			return nil, miningRuleError(ErrCalcCommitmentRoot, str)
		}
	} else {
		cmtRoot = standalone.CalcTxTreeMerkleRoot(msgBlock.STransactions)
	}
	msgBlock.Header.StakeRoot = cmtRoot

	msgBlock.Header.Size = uint32(msgBlock.SerializeSize())

	// Finally, perform a full check on the created block against the chain
	// consensus rules to ensure it properly connects to the current best
	// chain with no issues.
	block := dcrutil.NewBlockDeepCopyCoinbase(&msgBlock)
	err = g.cfg.CheckConnectBlockTemplate(block)
	if err != nil {
		str := fmt.Sprintf("failed to do final check for check connect "+
			"block when making new block template: %v",
			err.Error())
		return nil, miningRuleError(ErrCheckConnectBlock, str)
	}

	log.Debugf("Created new block template (%d transactions, %d stake "+
		"transactions, %d treasury transactions, %d in fees, %d signature "+
		"operations, %d bytes, target difficulty %064x, stake difficulty %v)",
		len(msgBlock.Transactions), len(msgBlock.STransactions),
		totalTreasuryOps, totalFees, blockSigOps, blockSize,
		standalone.CompactToBig(msgBlock.Header.Bits),
		dcrutil.Amount(msgBlock.Header.SBits).ToCoin())

	blockTemplate := &BlockTemplate{
		Block:           &msgBlock,
		Fees:            txFees,
		SigOpCounts:     txSigOpCounts,
		Height:          nextBlockHeight,
		ValidPayAddress: payToAddress != nil,
	}

	return blockTemplate, nil
}

// UpdateBlockTime updates the timestamp in the passed header to the current
// time while taking into account the median time of the last several blocks to
// ensure the new time is after that time per the chain consensus rules.
//
// Finally, it will update the target difficulty if needed based on the new time
// for the test networks since their target difficulty can change based upon
// time.
func (g *BlkTmplGenerator) UpdateBlockTime(header *wire.BlockHeader) error {
	// The new timestamp is potentially adjusted to ensure it comes after
	// the median time of the last several blocks per the chain consensus
	// rules.
	newTimestamp := g.medianAdjustedTime()
	header.Timestamp = newTimestamp

	// If running on a network that requires recalculating the difficulty,
	// do so now.
	if g.cfg.ChainParams.ReduceMinDifficulty {
		difficulty, err := g.cfg.CalcNextRequiredDifficulty(&header.PrevBlock,
			newTimestamp)
		if err != nil {
			return miningRuleError(ErrGettingDifficulty, err.Error())
		}
		header.Bits = difficulty
	}

	return nil
}
