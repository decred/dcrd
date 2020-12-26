// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"container/heap"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/blockchain/v3"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/gcs/v2/blockcf2"
	"github.com/decred/dcrd/lru"
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

// TxMiningView is a snapshot of the tx source.
type TxMiningView interface {
	// TxDescs returns a slice of mining descriptors for all minable
	// transactions in the source pool.
	TxDescs() []*TxDesc

	// AncestorStats returns the last known ancestor stats for the provided
	// transaction hash, and a boolean indicating whether ancestors are being
	// tracked for it.
	//
	// Calling Ancestors will update the value returned by this function to
	// reflect the newly calculated statistics for those ancestors.
	AncestorStats(txHash *chainhash.Hash) (*TxAncestorStats, bool)

	// Ancestors returns all transactions in the mining view that the provided
	// transaction hash depends on.
	Ancestors(txHash *chainhash.Hash) []*TxDesc

	// HasParents returns true if the provided transaction hash has any
	// ancestors known to the view.
	HasParents(txHash *chainhash.Hash) bool

	// Parents returns a set of transactions in the graph that the provided
	// transaction hash spends from in the view. The order of elements
	// returned is not guaranteed.
	Parents(txHash *chainhash.Hash) []*TxDesc

	// Children returns a set of transactions in the graph that spend
	// from the provided transaction hash. The order of elements
	// returned is not guaranteed.
	Children(txHash *chainhash.Hash) []*TxDesc

	// Remove causes the provided transaction to be removed from the view, if
	// it exists. The updateDescendantStats parameter indicates whether the
	// descendent transactions of the provided txHash should have their ancestor
	// stats updated within the view to account for the removal of this
	// transaction.
	Remove(txHash *chainhash.Hash, updateDescendantStats bool)

	// Reject removes and flags the provided transaction hash and all of its
	// descendants in the view as rejected.
	Reject(txHash *chainhash.Hash)

	// IsRejected checks to see if a transaction that once existed in the view
	// has been rejected.
	IsRejected(txHash *chainhash.Hash) bool
}

// VoteDesc is a descriptor about a vote transaction in a transaction source
// along with additional metadata.
type VoteDesc struct {
	VoteHash       chainhash.Hash
	TicketHash     chainhash.Hash
	ApprovesParent bool
}

// TxSource represents a source of transactions to consider for inclusion in
// new blocks.
//
// The interface contract requires that all of these methods are safe for
// concurrent access with respect to the source.
type TxSource interface {
	// LastUpdated returns the last time a transaction was added to or
	// removed from the source pool.
	LastUpdated() time.Time

	// HaveTransaction returns whether or not the passed transaction hash
	// exists in the source pool.
	HaveTransaction(hash *chainhash.Hash) bool

	// HaveAllTransactions returns whether or not all of the passed
	// transaction hashes exist in the source pool.
	HaveAllTransactions(hashes []chainhash.Hash) bool

	// VoteHashesForBlock returns the hashes for all votes on the provided
	// block hash that are currently available in the source pool.
	VoteHashesForBlock(hash *chainhash.Hash) []chainhash.Hash

	// VotesForBlocks returns a slice of vote descriptors for all votes on
	// the provided block hashes that are currently available in the source
	// pool.
	VotesForBlocks(hashes []chainhash.Hash) [][]VoteDesc

	// IsRegTxTreeKnownDisapproved returns whether or not the regular
	// transaction tree of the block represented by the provided hash is
	// known to be disapproved according to the votes currently in the
	// source pool.
	IsRegTxTreeKnownDisapproved(hash *chainhash.Hash) bool

	// MiningView returns a snapshot of the underlying TxSource.
	MiningView() TxMiningView
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

	// minVotesTimeoutDuration is the duration that must elapse after a new tip
	// block has been received before other variants that also extend the same
	// parent and received later are considered for the base of new templates.
	minVotesTimeoutDuration = time.Second * 3

	// maxVoteTimeoutDuration is the duration elapsed after the minimum number
	// of votes for a new tip block has been received that a new template with
	// less than the maximum number of votes will be generated.
	maxVoteTimeoutDuration = time.Millisecond * 2500 // 2.5 seconds

	// templateRegenSecs is the required number of seconds elapsed with
	// incoming non vote transactions before template regeneration
	// is required.
	templateRegenSecs = 30
)

// txPrioItem houses a transaction along with extra information that allows the
// transaction to be prioritized and track dependencies on other transactions
// which have not been mined into a block yet.
type txPrioItem struct {
	txDesc   *TxDesc
	txType   stake.TxType
	fee      int64
	priority float64
	feePerKB float64
}

// txPriorityQueueLessFunc describes a function that can be used as a compare
// function for a transaction priority queue (txPriorityQueue).
type txPriorityQueueLessFunc func(*txPriorityQueue, int, int) bool

// txPriorityQueue implements a priority queue of txPrioItem elements that
// supports an arbitrary compare function as defined by txPriorityQueueLessFunc.
type txPriorityQueue struct {
	lessFunc txPriorityQueueLessFunc
	items    []*txPrioItem
}

// Len returns the number of items in the priority queue.  It is part of the
// heap.Interface implementation.
func (pq *txPriorityQueue) Len() int {
	return len(pq.items)
}

// Less returns whether the item in the priority queue with index i should sort
// before the item with index j by deferring to the assigned less function.  It
// is part of the heap.Interface implementation.
func (pq *txPriorityQueue) Less(i, j int) bool {
	return pq.lessFunc(pq, i, j)
}

// Swap swaps the items at the passed indices in the priority queue.  It is
// part of the heap.Interface implementation.
func (pq *txPriorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
}

// Push pushes the passed item onto the priority queue.  It is part of the
// heap.Interface implementation.
func (pq *txPriorityQueue) Push(x interface{}) {
	pq.items = append(pq.items, x.(*txPrioItem))
}

// Pop removes the highest priority item (according to Less) from the priority
// queue and returns it.  It is part of the heap.Interface implementation.
func (pq *txPriorityQueue) Pop() interface{} {
	n := len(pq.items)
	item := pq.items[n-1]
	pq.items[n-1] = nil
	pq.items = pq.items[0 : n-1]
	return item
}

// SetLessFunc sets the compare function for the priority queue to the provided
// function.  It also invokes heap.Init on the priority queue using the new
// function so it can immediately be used with heap.Push/Pop.
func (pq *txPriorityQueue) SetLessFunc(lessFunc txPriorityQueueLessFunc) {
	pq.lessFunc = lessFunc
	heap.Init(pq)
}

// stakePriority is an integer that is used to sort stake transactions
// by importance when they enter the min heap for block construction.
// 2 is for votes (highest), followed by 1 for tickets (2nd highest),
// followed by 0 for regular transactions and revocations (lowest).
type stakePriority int

const (
	regOrRevocPriority stakePriority = iota
	ticketPriority
	votePriority
)

// stakePriority assigns a stake priority based on a transaction type.
func txStakePriority(txType stake.TxType) stakePriority {
	prio := regOrRevocPriority
	switch txType {
	case stake.TxTypeSSGen:
		prio = votePriority
	case stake.TxTypeSStx:
		prio = ticketPriority
	}

	return prio
}

// compareStakePriority compares the stake priority of two transactions.
// It uses votes > tickets > regular transactions or revocations. It
// returns 1 if i > j, 0 if i == j, and -1 if i < j in terms of stake
// priority.
func compareStakePriority(i, j *txPrioItem) int {
	iStakePriority := txStakePriority(i.txType)
	jStakePriority := txStakePriority(j.txType)

	if iStakePriority > jStakePriority {
		return 1
	}
	if iStakePriority < jStakePriority {
		return -1
	}
	return 0
}

// txPQByStakeAndFee sorts a txPriorityQueue by stake priority, followed by
// fees per kilobyte, and then transaction priority.
func txPQByStakeAndFee(pq *txPriorityQueue, i, j int) bool {
	// Sort by stake priority, continue if they're the same stake priority.
	cmp := compareStakePriority(pq.items[i], pq.items[j])
	if cmp == 1 {
		return true
	}
	if cmp == -1 {
		return false
	}

	// Using > here so that pop gives the highest fee item as opposed
	// to the lowest.  Sort by fee first, then priority.
	if pq.items[i].feePerKB == pq.items[j].feePerKB {
		return pq.items[i].priority > pq.items[j].priority
	}

	// The stake priorities are equal, so return based on fees
	// per KB.
	return pq.items[i].feePerKB > pq.items[j].feePerKB
}

// txPQByStakeAndFeeAndThenPriority sorts a txPriorityQueue by stake priority,
// followed by fees per kilobyte, and then if the transaction type is regular
// or a revocation it sorts it by priority.
func txPQByStakeAndFeeAndThenPriority(pq *txPriorityQueue, i, j int) bool {
	// Sort by stake priority, continue if they're the same stake priority.
	cmp := compareStakePriority(pq.items[i], pq.items[j])
	if cmp == 1 {
		return true
	}
	if cmp == -1 {
		return false
	}

	bothAreLowStakePriority :=
		txStakePriority(pq.items[i].txType) == regOrRevocPriority &&
			txStakePriority(pq.items[j].txType) == regOrRevocPriority

	// Use fees per KB on high stake priority transactions.
	if !bothAreLowStakePriority {
		return pq.items[i].feePerKB > pq.items[j].feePerKB
	}

	// Both transactions are of low stake importance. Use > here so that
	// pop gives the highest priority item as opposed to the lowest.
	// Sort by priority first, then fee.
	if pq.items[i].priority == pq.items[j].priority {
		return pq.items[i].feePerKB > pq.items[j].feePerKB
	}

	return pq.items[i].priority > pq.items[j].priority
}

// newTxPriorityQueue returns a new transaction priority queue that reserves the
// passed amount of space for the elements.  The new priority queue uses the
// less than function lessFunc to sort the items in the min heap. The priority
// queue can grow larger than the reserved space, but extra copies of the
// underlying array can be avoided by reserving a sane value.
func newTxPriorityQueue(reserve int, lessFunc func(*txPriorityQueue, int, int) bool) *txPriorityQueue {
	pq := &txPriorityQueue{
		items: make([]*txPrioItem, 0, reserve),
	}
	pq.SetLessFunc(lessFunc)
	return pq
}

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

// mergeUtxoView adds all of the entries in view to viewA.  The result is that
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
	best := g.chain.BestSnapshot()
	newTimestamp := g.timeSource.AdjustedTime()
	minTimestamp := minimumMedianTime(best)
	if newTimestamp.Before(minTimestamp) {
		newTimestamp = minTimestamp
	}

	// Adjust by the amount requested from the command line argument.
	newTimestamp = newTimestamp.Add(
		time.Duration(-g.miningTimeOffset) * time.Second)

	return newTimestamp
}

// maybeInsertStakeTx checks to make sure that a stake tx is
// valid from the perspective of the mainchain (not necessarily
// the mempool or block) before inserting into a tx tree.
// If it fails the check, it returns false; otherwise true.
func (g *BlkTmplGenerator) maybeInsertStakeTx(stx *dcrutil.Tx, treeValid bool, isTreasuryEnabled bool) bool {
	missingInput := false

	view, err := g.chain.FetchUtxoView(stx, treeValid)
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
	stakeValidationHeight := g.chainParams.StakeValidationHeight

	// Handle not enough voters being present if we're set to mine aggressively
	// (default behavior).
	best := g.chain.BestSnapshot()
	if nextHeight >= stakeValidationHeight && g.policy.AggressiveMining {
		// Fetch the latest block and head and begin working off of it with an
		// empty transaction tree regular and the contents of that stake tree.
		// In the future we should have the option of reading some transactions
		// from this block, too.
		topBlock, err := g.chain.BlockByHash(&best.Hash)
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
		coinbaseTx, err := createCoinbaseTx(g.subsidyCache, coinbaseScript,
			opReturnPkScript, topBlock.Height(), miningAddress,
			tipHeader.Voters, g.chainParams, isTreasuryEnabled)
		if err != nil {
			return nil, err
		}
		block.AddTransaction(coinbaseTx.MsgTx())

		if isTreasuryEnabled {
			treasuryBase, err := createTreasuryBaseTx(g.subsidyCache,
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
		if g.chainParams.ReduceMinDifficulty {
			parentHash := topBlock.MsgBlock().Header.PrevBlock

			requiredDifficulty, err :=
				g.chain.CalcNextRequiredDifficulty(&parentHash, ts)
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
		hdrCmtActive, err := g.chain.IsHeaderCommitmentsAgendaActive(prevHash)
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
			blockUtxos, err := g.chain.FetchUtxoViewParentTemplate(&block)
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
		err = g.chain.CheckConnectBlockTemplate(btBlock)
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

// blockManagerFacade provides the mining package with a subset of
// the methods originally defined in the blockManager.
type blockManagerFacade interface {
	ForceReorganization(formerBest, newBest chainhash.Hash) error
	IsCurrent() bool
	NotifyWork(templateNtfn *TemplateNtfn)
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
	policy           *Policy
	txSource         TxSource
	sigCache         *txscript.SigCache
	subsidyCache     *standalone.SubsidyCache
	chainParams      *chaincfg.Params
	chain            *blockchain.BlockChain
	blockManager     blockManagerFacade
	timeSource       blockchain.MedianTimeSource
	miningTimeOffset int
}

// NewBlkTmplGenerator returns a new block template generator for the given
// policy using transactions from the provided transaction source.
func NewBlkTmplGenerator(policy *Policy, txSource TxSource,
	timeSource blockchain.MedianTimeSource, sigCache *txscript.SigCache,
	subsidyCache *standalone.SubsidyCache, chainParams *chaincfg.Params,
	chain *blockchain.BlockChain, blockManager blockManagerFacade,
	miningTimeOffset int) *BlkTmplGenerator {

	return &BlkTmplGenerator{
		policy:           policy,
		txSource:         txSource,
		sigCache:         sigCache,
		subsidyCache:     subsidyCache,
		chainParams:      chainParams,
		chain:            chain,
		blockManager:     blockManager,
		timeSource:       timeSource,
		miningTimeOffset: miningTimeOffset,
	}
}

// calcFeePerKb returns an adjusted fee per kilobyte taking the provided
// transaction and its ancestors into account.
func calcFeePerKb(txDesc *TxDesc, ancestorStats *TxAncestorStats) float64 {
	txSize := txDesc.Tx.MsgTx().SerializeSize()
	if ancestorStats.Fees < 0 || ancestorStats.SizeBytes < 0 {
		return (float64(txDesc.Fee) * float64(kilobyte)) / float64(txSize)
	}
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
	scriptFlags, err := g.policy.StandardVerifyFlags()
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

	best := g.chain.BestSnapshot()
	prevHash := best.Hash
	nextBlockHeight := best.Height + 1
	stakeValidationHeight := g.chainParams.StakeValidationHeight

	isTreasuryEnabled, err := g.chain.IsTreasuryAgendaActive(&prevHash)
	if err != nil {
		return nil, err
	}

	var (
		isTVI            bool
		maxTreasurySpend int64
	)
	if isTreasuryEnabled {
		isTVI = standalone.IsTreasuryVoteInterval(uint64(nextBlockHeight),
			g.chainParams.TreasuryVoteInterval)
	}

	if nextBlockHeight >= stakeValidationHeight {
		// Obtain the entire generation of blocks stemming from this parent.
		children, err := g.chain.TipGeneration()
		if err != nil {
			return nil, miningRuleError(ErrFailedToGetGeneration, err.Error())
		}

		// Get the list of blocks that we can actually build on top of. If we're
		// not currently on the block that has the most votes, switch to that
		// block.
		eligibleParents := SortParentsByVotes(g.txSource, prevHash, children,
			g.chainParams)
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
				err := g.blockManager.ForceReorganization(prevHash, *newHead)
				if err != nil {
					log.Errorf("failed to reorganize to new parent: %v", err)
					continue
				}

				// Check to make sure we actually have the transactions
				// (votes) we need in the mempool.
				voteHashes := g.txSource.VoteHashesForBlock(newHead)
				if len(voteHashes) == 0 {
					return nil, fmt.Errorf("no vote metadata for block %v",
						newHead)
				}

				if exist := g.txSource.HaveAllTransactions(voteHashes); !exist {
					continue
				} else {
					prevHash = *newHead
					break
				}
			}
		}

		// Obtain the maximum allowed treasury expenditure.
		if isTreasuryEnabled && isTVI {
			maxTreasurySpend, err = g.chain.MaxTreasuryExpenditure(&prevHash)
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
	miningView := g.txSource.MiningView()
	sourceTxns := miningView.TxDescs()
	sortedByFee := g.policy.BlockPrioritySize == 0
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
	blockUtxos := blockchain.NewUtxoViewpoint(g.chain)

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
	knownDisapproved := g.txSource.IsRegTxTreeKnownDisapproved(&prevHash)

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
		if !blockchain.IsFinalizedTransaction(tx, nextBlockHeight,
			best.MedianTime) {
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
		utxos, err := g.chain.FetchUtxoView(tx, !knownDisapproved)
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
				if !g.txSource.HaveTransaction(originHash) {
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
		hasParents := miningView.HasParents(tx.Hash())

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
		deps := miningView.Children(tx.Hash())

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
				exp, g.chainParams.TreasuryVoteInterval,
				g.chainParams.TreasuryVoteIntervalMultiplier) {

				log.Tracef("Skipping treasury spend %v at height %d because it "+
					"has an expiry of %d that is outside of the voting window",
					tx.Hash(), nextBlockHeight, exp)
				continue
			}

			// Ensure there are enough votes. Send in a fake block
			// with the proper height.
			err = g.chain.CheckTSpendHasVotes(prevHash,
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
			int(g.chainParams.MaxFreshStakePerBlock)) {
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

		if miningView.IsRejected(tx.Hash()) {
			// If the transaction or any of its ancestors have been rejected,
			// discard the transaction.
			continue
		}

		ancestors := miningView.Ancestors(tx.Hash())
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
			blockPlusTxSize >= g.policy.BlockMaxSize {
			log.Tracef("Skipping tx %s (size %v) because it "+
				"would exceed the max block size; cur block "+
				"size %v, cur num tx %v", tx.Hash(), txSize,
				blockSize, len(blockTxns))
			logSkippedDeps(tx, deps)
			miningView.Reject(tx.Hash())
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
			miningView.Reject(tx.Hash())
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
			(prioItem.feePerKB < float64(g.policy.TxMinFreeFee)) &&
			(tx.Tree() != wire.TxTreeStake) &&
			(blockPlusTxSize >= g.policy.BlockMinSize) {

			log.Tracef("Skipping tx %s with feePerKB %.2f "+
				"< TxMinFreeFee %d and block size %d >= "+
				"minBlockSize %d", tx.Hash(), prioItem.feePerKB,
				g.policy.TxMinFreeFee, blockPlusTxSize,
				g.policy.BlockMinSize)
			logSkippedDeps(tx, deps)
			miningView.Reject(tx.Hash())
			continue
		}

		// Prioritize by fee per kilobyte once the block is larger than
		// the priority size or there are no more high-priority
		// transactions.
		if !sortedByFee && (blockPlusTxSize >= g.policy.BlockPrioritySize ||
			prioItem.priority <= MinHighPriority) {

			log.Tracef("Switching to sort by fees per "+
				"kilobyte blockSize %d >= BlockPrioritySize "+
				"%d || priority %.2f <= minHighPriority %.2f",
				blockPlusTxSize, g.policy.BlockPrioritySize,
				prioItem.priority, MinHighPriority)

			sortedByFee = true
			priorityQueue.SetLessFunc(txPQByStakeAndFee)

			// Put the transaction back into the priority queue and
			// skip it so it is re-prioritized by fees if it won't
			// fit into the high-priority section or the priority is
			// too low.  Otherwise this transaction will be the
			// final one in the high-priority section, so just fall
			// though to the code below so it is added now.
			if blockPlusTxSize > g.policy.BlockPrioritySize ||
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
			_, err = blockchain.CheckTransactionInputs(g.subsidyCache,
				bundledTx.Tx, nextBlockHeight, blockUtxos, false, g.chainParams,
				isTreasuryEnabled)
			if err != nil {
				log.Tracef("Skipping tx %s due to error in "+
					"CheckTransactionInputs: %v", bundledTx.Tx.Hash(), err)
				logSkippedDeps(bundledTx.Tx, deps)
				miningView.Reject(bundledTx.Tx.Hash())
				continue nextPriorityQueueItem
			}
			err = blockchain.ValidateTransactionScripts(bundledTx.Tx,
				blockUtxos, scriptFlags, g.sigCache)
			if err != nil {
				log.Tracef("Skipping tx %s due to error in "+
					"ValidateTransactionScripts: %v", bundledTx.Tx.Hash(), err)
				logSkippedDeps(bundledTx.Tx, deps)
				miningView.Reject(bundledTx.Tx.Hash())
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
			bundledTxDeps := miningView.Children(bundledTxHash)
			miningView.Remove(bundledTxHash, false)

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
				if !miningView.HasParents(childTxHash) {
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
	voteBitsVoters := make([]uint16, 0, g.chainParams.TicketsPerBlock)
	votes := make([]*dcrutil.Tx, 0, g.chainParams.TicketsPerBlock)

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
		treasuryBase, err = createTreasuryBaseTx(g.subsidyCache,
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
		if freshStake >= int(g.chainParams.MaxFreshStakePerBlock) {
			break
		}
	}

	// Ensure that mining the block would not cause the chain to become
	// unrecoverable due to ticket exhaustion.
	err = g.chain.CheckTicketExhaustion(&best.Hash, uint8(freshStake))
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

	coinbaseTx, err := createCoinbaseTx(g.subsidyCache, coinbaseScript,
		opReturnPkScript, nextBlockHeight, payToAddress, uint16(voters),
		g.chainParams, isTreasuryEnabled)
	if err != nil {
		return nil, err
	}
	coinbaseTx.SetTree(wire.TxTreeRegular)

	numCoinbaseSigOps := int64(blockchain.CountSigOps(coinbaseTx, true,
		false, isTreasuryEnabled))
	blockSize += uint32(coinbaseTx.MsgTx().SerializeSize())
	blockSigOps += numCoinbaseSigOps
	txFeesMap[*coinbaseTx.Hash()] = 0
	txSigOpCountsMap[*coinbaseTx.Hash()] = numCoinbaseSigOps
	if treasuryBase != nil {
		txFeesMap[*treasuryBase.Hash()] = 0
		n := int64(blockchain.CountSigOps(treasuryBase, true, false,
			isTreasuryEnabled))
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
		totalFees /= int64(g.chainParams.TicketsPerBlock)
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
	reqDifficulty, err := g.chain.CalcNextRequiredDifficulty(&prevHash, ts)
	if err != nil {
		return nil, miningRuleError(ErrGettingDifficulty, err.Error())
	}

	// Return nil if we don't yet have enough voters; sometimes it takes a
	// bit for the mempool to sync with the votes map and we end up down
	// here despite having the relevant votes available in the votes map.
	minimumVotesRequired :=
		int((g.chainParams.TicketsPerBlock / 2) + 1)
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

		utxs, err := g.chain.FetchUtxoView(tx, !knownDisapproved)
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
	if g.chainParams.Net != wire.MainNet && g.chainParams.Net != wire.SimNet {
		blockVersion = generatedBlockVersionTest
	}

	// Figure out stake version.
	generatedStakeVersion, err := g.chain.CalcStakeVersionByHash(&prevHash)
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
	hdrCmtActive, err := g.chain.IsHeaderCommitmentsAgendaActive(&prevHash)
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
	err = g.chain.CheckConnectBlockTemplate(block)
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
	if g.chainParams.ReduceMinDifficulty {
		difficulty, err := g.chain.CalcNextRequiredDifficulty(&header.PrevBlock,
			newTimestamp)
		if err != nil {
			return miningRuleError(ErrGettingDifficulty, err.Error())
		}
		header.Bits = difficulty
	}

	return nil
}

// regenEventType represents the type of a template regeneration event message.
type regenEventType int

// Constants for the type of template regeneration event messages.
const (
	// rtReorgStarted indicates a chain reorganization has been started.
	rtReorgStarted regenEventType = iota

	// rtReorgDone indicates a chain reorganization has completed.
	rtReorgDone

	// rtBlockAccepted indicates a new block has been accepted to the block
	// chain which does not necessarily mean it was added to the main chain.
	// That case is rtBlockConnected.
	rtBlockAccepted

	// rtBlockConnected indicates a new block has been connected to the main
	// chain.
	rtBlockConnected

	// rtBlockDisconnected indicates the current tip block of the best chain has
	// been disconnected.
	rtBlockDisconnected

	// rtVote indicates a new vote has been received.  It applies to all votes
	// and therefore may or may not be relevant.
	rtVote

	// rtTemplateUpdated indicates the current template associated with the
	// generator has been updated.
	rtTemplateUpdated

	// rtForceRegen indicates the template should be regenerated even if
	// it's not yet time for it to be regenerated.
	rtForceRegen
)

// TemplateUpdateReason represents the type of a reason why a template is
// being updated.
type TemplateUpdateReason int

// Constants for the type of template update reasons.
const (
	// TURNewParent indicates the associated template has been updated because
	// it builds on a new block as compared to the previous template.
	TURNewParent TemplateUpdateReason = iota

	// TURNewVotes indicates the associated template has been updated because a
	// new vote for the block it builds on has been received.
	TURNewVotes

	// TURNewTxns indicates the associated template has been updated because new
	// non-vote transactions are available and have potentially been included.
	TURNewTxns

	// turUnknown indicates the associated template has either been updated due
	// to an error or cleared for a chain reorg.  It is only used internally to
	// the background template generator.
	turUnknown
)

// TemplateNtfn represents a notification of a new template along with the
// reason it was generated.  It is sent to subscribers on the channel obtained
// from the TemplateSubscription instance returned by Subscribe.
type TemplateNtfn struct {
	Template *BlockTemplate
	Reason   TemplateUpdateReason
}

// templateUpdate defines a type which is used to signal the regen event handler
// that a new template and relevant error have been associated with the
// generator.
type templateUpdate struct {
	template *BlockTemplate
	err      error
}

// regenEvent defines an event which will potentially result in regenerating a
// block template and consists of a regen event type as well as associated data
// that depends on the type as follows:
//  - rtReorgStarted:      nil
//  - rtReorgDone:         nil
//  - rtBlockAccepted:     *dcrutil.Block
//  - rtBlockConnected:    *dcrutil.Block
//  - rtBlockDisconnected: *dcrutil.Block
//  - rtVote:              *dcrutil.Tx
//  - rtTemplateUpdated:   templateUpdate
type regenEvent struct {
	reason regenEventType
	value  interface{}
}

// BgBlkTmplGenerator provides facilities for asynchronously generating block
// templates in response to various relevant events and allowing clients to
// subscribe for updates when new templates are generated as well as access the
// most recently-generated template in a concurrency-safe manner.
//
// An example of some of the events that trigger a new block template to be
// generated are modifications to the current best chain, receiving relevant
// votes, and periodic timeouts to allow inclusion of new transactions.
//
// The templates are generated based on a given block template generator
// instance which itself is based on a given mining policy and transaction
// source.  See the NewBlockTemplate method for a detailed description of how
// the block template is generated.
//
// The background generation makes use of three main goroutines -- a regen event
// queue to allow asynchronous non-blocking signalling, a regen event handler to
// process the aforementioned queue and react accordingly, and a subscriber
// notification controller.  In addition, the templates themselves are generated
// in their own goroutines with a cancellable context.
//
// A high level overview of the semantics are as follows:
// - Ignore all vote handling when prior to stake validation height
// - Generate templates building on the current tip at startup with a fall back
//   to generate a template on its parent if the current tip does not receive
//   enough votes within a timeout
//   - Continue monitoring for votes on any blocks that extend said parent to
//     potentially switch to them and generate a template building on them when
//     possible
// - Generate new templates building on new best chain tip blocks once they have
//   received the minimum votes after a timeout to provide the additional votes
//   an opportunity to propagate, except when it is an intermediate block in a
//   chain reorganization
//   - In the event the current tip fails to receive the minimum number of
//     required votes, monitor side chain blocks which are siblings of it for
//     votes in order to potentially switch to them and generate a template
//     building on them when possible
// - Generate new templates on blocks disconnected from the best chain tip,
//   except when it is an intermediate block in a chain reorganization
// - Generate new templates periodically when there are new regular transactions
//   to include
// - Bias templates towards building on the first seen block when possible in
//   order to prevent PoW miners from being able to gain an advantage through
//   vote withholding
// - Schedule retries in the rare event template generation fails
// - Allow clients to subscribe for updates every time a new template is
//   successfully generated along with a reason why it was generated
// - Provide direct access to the most-recently generated template
//   - Block while generating new templates that will make the current template
//     stale (e.g. new parent or new votes)
type BgBlkTmplGenerator struct {
	wg   sync.WaitGroup
	quit chan struct{}

	// These fields are provided by the caller when the generator is created and
	// are either independently safe for concurrent access or do not change after
	// initialization.
	//
	// chain is the blockchain instance that is used to build the block and
	// validate the block templates.
	//
	// tg is a block template generator instance that is used to actually create
	// the block templates the background block template generator stores.
	//
	// allowUnsyncedMining indicates block templates should be created even when
	// the chain is not fully synced.
	//
	// maxVotesPerBlock is the maximum number of votes per block and comes from
	// the chain parameters.  It is defined separately for convenience.
	//
	// minVotesRequired is the minimum number of votes required for a block to
	// be built on.  It is derived from the chain parameters and is defined
	// separately for convenience.
	chain               *blockchain.BlockChain
	tg                  *BlkTmplGenerator
	allowUnsyncedMining bool
	miningAddrs         []dcrutil.Address
	maxVotesPerBlock    uint16
	minVotesRequired    uint16

	// These fields deal with providing a stream of template updates to
	// subscribers.
	//
	// subscriptions tracks all template update subscriptions.  It is protected
	// for concurrent access by subscriptionMtx.
	//
	// notifySubscribers delivers template updates to the separate subscriber
	// notification goroutine so it can in turn asynchronously deliver
	// notifications to all subscribers.
	subscriptionMtx   sync.Mutex
	subscriptions     map[*TemplateSubscription]struct{}
	notifySubscribers chan *TemplateNtfn
	notifiedParents   lru.Cache

	// These fields deal with the template regeneration event queue.  This is
	// implemented as a concurrent queue with immediate passthrough when
	// possible to ensure the order of events is maintained and the related
	// callbacks never block.
	//
	// queueRegenEvent either immediately forwards regen events to the
	// regenEventMsgs channel when it would not block or adds the event to a
	// queue that is processed asynchronously as soon as the receiver becomes
	// available.
	//
	// regenEventMsgs delivers relevant regen events to which the generator
	// reacts to the separate regen goroutine so it can in turn asynchronously
	// process the events and regenerate templates as needed.
	queueRegenEvent chan regenEvent
	regenEventMsgs  chan regenEvent

	// staleTemplateWg is used to allow template retrieval to block callers when
	// a new template that will make the current template stale is being
	// generated.  Stale, in this context, means either the parent has changed
	// or there are new votes available.
	staleTemplateWg sync.WaitGroup

	// These fields track the current best template and are protected by the
	// template mutex.  The template will be nil when there is a template error
	// set.
	templateMtx    sync.Mutex
	template       *BlockTemplate
	templateReason TemplateUpdateReason
	templateErr    error

	// These fields are used to provide the ability to cancel a template that
	// is in the process of being asynchronously generated in favor of
	// generating a new one.
	//
	// cancelTemplate is a function which will cancel the current template that
	// is in the process of being asynchronously generated.  It will have no
	// effect if no template generation is in progress.  It is protected for
	// concurrent access by cancelTemplateMtx.
	cancelTemplateMtx sync.Mutex
	cancelTemplate    func()
}

// NewBgBlkTmplGenerator initializes a background block template generator with
// the provided parameters.  The returned instance must be started with the Run
// method to allowing processing.
func NewBgBlkTmplGenerator(tg *BlkTmplGenerator, addrs []dcrutil.Address, allowUnsynced bool) *BgBlkTmplGenerator {
	return &BgBlkTmplGenerator{
		quit:                make(chan struct{}),
		chain:               tg.chain,
		tg:                  tg,
		allowUnsyncedMining: allowUnsynced,
		miningAddrs:         addrs,
		maxVotesPerBlock:    tg.chainParams.TicketsPerBlock,
		minVotesRequired:    (tg.chainParams.TicketsPerBlock / 2) + 1,
		subscriptions:       make(map[*TemplateSubscription]struct{}),
		notifySubscribers:   make(chan *TemplateNtfn),
		notifiedParents:     lru.NewCache(3),
		queueRegenEvent:     make(chan regenEvent),
		regenEventMsgs:      make(chan regenEvent),
		cancelTemplate:      func() {},
	}
}

// UpdateBlockTime updates the timestamp in the passed header to the current
// time while taking into account the median time of the last several blocks to
// ensure the new time is after that time per the chain consensus rules.
//
// Finally, it will update the target difficulty if needed based on the new time
// for the test networks since their target difficulty can change based upon
// time.
func (g *BgBlkTmplGenerator) UpdateBlockTime(header *wire.BlockHeader) error {
	return g.tg.UpdateBlockTime(header)
}

// sendQueueRegenEvent sends the provided regen event on the internal queue
// regen event channel while respecting the quit channel.  The allows orderly
// shutdown when the generator is shutdown.
func (g *BgBlkTmplGenerator) sendQueueRegenEvent(event regenEvent) {
	select {
	case g.queueRegenEvent <- event:
	case <-g.quit:
	}
}

// setCurrentTemplate sets the current template and error associated with the
// background block template generator and notifies the regen event handler
// about the update.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) setCurrentTemplate(template *BlockTemplate, reason TemplateUpdateReason, err error) {
	g.templateMtx.Lock()
	g.template, g.templateReason, g.templateErr = template, reason, err
	g.templateMtx.Unlock()

	tplUpdate := templateUpdate{template: template, err: err}
	g.sendQueueRegenEvent(regenEvent{rtTemplateUpdated, tplUpdate})
}

// currentTemplate returns the current template associated with the background
// template generator along with the associated reason and error.
//
// NOTE: The returned template and block that it contains MUST be treated as
// immutable since they are shared by all callers.
//
// NOTE: The returned template might be nil even if there is no error.  It is
// the responsibility of the caller to properly handle nil templates.
//
// This function differs from the exported version in that it also returns the
// reason associated with the template that is used in notifications.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) currentTemplate() (*BlockTemplate, TemplateUpdateReason, error) {
	g.staleTemplateWg.Wait()
	g.templateMtx.Lock()
	template, reason, err := g.template, g.templateReason, g.templateErr
	g.templateMtx.Unlock()
	return template, reason, err
}

// CurrentTemplate returns the current template associated with the background
// template generator along with any associated error.
//
// NOTE: The returned template and block that it contains MUST be treated as
// immutable since they are shared by all callers.
//
// NOTE: The returned template might be nil even if there is no error.  It is
// the responsibility of the caller to properly handle nil templates.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) CurrentTemplate() (*BlockTemplate, error) {
	template, _, err := g.currentTemplate()
	return template, err
}

// TemplateSubscription defines a subscription to receive block template updates
// from the background block template generator.  The caller must call Stop on
// the subscription when it is no longer needed to free resources.
//
// NOTE: Notifications are dropped to make up for slow receivers to ensure
// notifications to other subscribers, as well as senders, are not blocked
// indefinitely.  Since templates are typically only generated infrequently and
// receives must fall several templates behind before new ones are dropped, this
// should not affect callers in practice, however, if a caller wishes to
// guarantee that no templates are being dropped, they will need to ensure the
// channel is always processed quickly.
type TemplateSubscription struct {
	g     *BgBlkTmplGenerator
	privC chan *TemplateNtfn
}

// C returns a channel that produces a stream of block templates as each new
// template is generated.  Successive calls to C return the same channel.
//
// NOTE: Notifications are dropped to make up for slow receivers.  See the
// template subscription type documentation for more details.
func (s *TemplateSubscription) C() <-chan *TemplateNtfn {
	return s.privC
}

// Stop prevents any future template updates from being delivered and
// unsubscribes the associated subscription.
//
// NOTE: The channel is not closed to prevent a read from the channel succeeding
// incorrectly.
func (s *TemplateSubscription) Stop() {
	s.g.subscriptionMtx.Lock()
	delete(s.g.subscriptions, s)
	s.g.subscriptionMtx.Unlock()
}

// publishTemplateNtfn sends the provided template notification on the channel
// associated with the subscription.
func (s *TemplateSubscription) publishTemplateNtfn(templateNtfn *TemplateNtfn) {
	// Make use of a non-blocking send along with the buffered channel to allow
	// notifications to be dropped to make up for slow receivers.
	select {
	case s.privC <- templateNtfn:
	default:
	}
}

// notifySubscribersHandler updates subscribers with newly created block
// templates.
//
// This must be run as a goroutine.
func (g *BgBlkTmplGenerator) notifySubscribersHandler(ctx context.Context) {
	for {
		select {
		case templateNtfn := <-g.notifySubscribers:
			g.tg.blockManager.NotifyWork(templateNtfn)

			g.subscriptionMtx.Lock()
			for subscription := range g.subscriptions {
				subscription.publishTemplateNtfn(templateNtfn)
			}
			g.subscriptionMtx.Unlock()

		case <-ctx.Done():
			g.wg.Done()
			return
		}
	}
}

// Subscribe subscribes a client for block template updates.  The returned
// template subscription contains functions to retrieve a channel that produces
// the stream of block templates and to stop the stream when the caller no
// longer wishes to receive new templates.
//
// The current template associated with the background block template generator,
// if any, is immediately sent to the returned subscription stream.
func (g *BgBlkTmplGenerator) Subscribe() *TemplateSubscription {
	// Create the subscription with a buffered channel that is large enough to
	// handle twice the number of templates that can be induced due votes in
	// order to provide a reasonable amount of buffering before dropping
	// notifications due to a slow receiver.
	maxVoteInducedRegens := g.maxVotesPerBlock - g.minVotesRequired + 1
	c := make(chan *TemplateNtfn, maxVoteInducedRegens*2)
	subscription := &TemplateSubscription{
		g:     g,
		privC: c,
	}
	g.subscriptionMtx.Lock()
	g.subscriptions[subscription] = struct{}{}
	g.subscriptionMtx.Unlock()

	// Send existing valid template immediately.
	template, reason, err := g.currentTemplate()
	if err == nil && template != nil {
		subscription.publishTemplateNtfn(&TemplateNtfn{template, reason})
	}

	return subscription
}

// regenQueueHandler immediately forwards items from the regen event queue
// channel to the regen event messages channel when it would not block or adds
// the event to an internal queue to be processed as soon as the receiver
// becomes available.  This ensures that queueing regen events never blocks
// despite how busy the regen handler might become during a burst of events.
//
// This must be run as a goroutine.
func (g *BgBlkTmplGenerator) regenQueueHandler(ctx context.Context) {
	var q []regenEvent
	var out, dequeue chan<- regenEvent = g.regenEventMsgs, nil
	skipQueue := out
	var next regenEvent
	for {
		select {
		case n := <-g.queueRegenEvent:
			// Either send to destination channel immediately when skipQueue is
			// non-nil (queue is empty) and reader is ready, or append to the
			// queue and send later.
			select {
			case skipQueue <- n:
			default:
				q = append(q, n)
				dequeue = out
				skipQueue = nil
				next = q[0]
			}

		case dequeue <- next:
			copy(q, q[1:])
			q = q[:len(q)-1]
			if len(q) == 0 {
				dequeue = nil
				skipQueue = out
			} else {
				next = q[0]
			}

		case <-ctx.Done():
			g.wg.Done()
			return
		}
	}
}

// regenHandlerState houses the state used in the regen event handler goroutine.
// It is separated from the background template generator to ensure it is only
// available within the scope of the goroutine.
type regenHandlerState struct {
	// isReorganizing indicates the chain is currently undergoing a
	// reorganization and therefore the generator should not attempt to create
	// new templates until the reorganization has completed.
	isReorganizing bool

	// These fields are used to implement a periodic regeneration timeout that
	// can be reset at any time without needing to create a new one and the
	// associated extra garbage.
	//
	// regenTimer is an underlying timer that is used to implement the timeout.
	//
	// regenChanDrained indicates whether or not the channel for the regen timer
	// has already been read and is used when resetting the timer to ensure the
	// channel is drained when the timer is stopped as described in the timer
	// documentation.
	//
	// lastGeneratedTime specifies the timestamp the current template was
	// generated.
	regenTimer        *time.Timer
	regenChanDrained  bool
	lastGeneratedTime int64

	// These fields are used to control the various generation states when a new
	// block that requires votes has been received.
	//
	// awaitingMinVotesHash is selectively set when a new tip block has been
	// received that requires votes until the minimum number of required votes
	// has been received.
	//
	// maxVotesTimeout is selectively enabled once the minimum number of
	// required votes for the current tip block has been received and is
	// disabled once the maximum number of votes has been received.  This
	// effectively sets a timeout to give the remaining votes an opportunity to
	// propagate prior to forcing a template with less than the maximum number
	// of votes.
	awaitingMinVotesHash *chainhash.Hash
	maxVotesTimeout      <-chan time.Time

	// These fields are used to handle detection of side chain votes and
	// potentially reorganizing the chain to a variant of the current tip when
	// it is unable to obtain the minimum required votes.
	//
	// awaitingSideChainMinVotes houses the known blocks that build from the
	// same parent as the current tip and will only be selectively populated
	// when none of the current possible tips have the minimum number of
	// required votes.
	//
	// trackSideChainsTimeout is selectively enabled when a new tip block has
	// been received in order to give the minimum number of required votes
	// needed to build a block template on it an opportunity to propagate before
	// attempting to find any other variants that extend the same parent as the
	// current tip with enough votes to force a reorganization.  This ensures the
	// first block that is seen is chosen to build templates on so long as it
	// receives the minimum required votes in order to prevent PoW miners from
	// being able to gain an advantage through vote withholding.  It is disabled
	// if the minimum number of votes is received prior to the timeout.
	awaitingSideChainMinVotes map[chainhash.Hash]struct{}
	trackSideChainsTimeout    <-chan time.Time

	// failedGenRetryTimeout is selectively enabled in the rare case a template
	// fails to generate so it can be regenerated again after a delay.  A
	// template should never fail to generate in practice, however, future code
	// changes might break that assumption and thus it is important to handle
	// the case properly.
	failedGenRetryTimeout <-chan time.Time

	// These fields track the block and height that the next template to be
	// generated will build on.  This may not be the same as the current tip in
	// the case it has not yet received the minimum number of required votes
	// needed to build a template on it.
	//
	// baseBlockHash is the hash of the block the next template to be generated
	// will build on.
	//
	// baseBlockHeight is the height of the block identified by the base block
	// hash.
	baseBlockHash   chainhash.Hash
	baseBlockHeight uint32
}

// makeRegenHandlerState returns a regen handler state that is ready to use.
func makeRegenHandlerState() regenHandlerState {
	regenTimer := time.NewTimer(math.MaxInt64)
	regenTimer.Stop()
	return regenHandlerState{
		regenTimer:                regenTimer,
		regenChanDrained:          true,
		awaitingSideChainMinVotes: make(map[chainhash.Hash]struct{}),
	}
}

// stopRegenTimer stops the regen timer while ensuring to read from the timer's
// channel in the case the timer already expired which can happen due to the
// fact the stop happens in between channel reads.   This behavior is well
// documented in the Timer docs.
//
// NOTE: This function must not be called concurrent with any other receives on
// the timer's channel.
func (state *regenHandlerState) stopRegenTimer() {
	t := state.regenTimer
	if !t.Stop() && !state.regenChanDrained {
		<-t.C
	}
	state.regenChanDrained = true
}

// resetRegenTimer resets the regen timer to the given duration while ensuring
// to read from the timer's channel in the case the timer already expired which
// can happen due to the fact the reset happens in between channel reads.   This
// behavior is well documented in the Timer docs.
//
// NOTE: This function must not be called concurrent with any other receives on
// the timer's channel.
func (state *regenHandlerState) resetRegenTimer(d time.Duration) {
	state.stopRegenTimer()
	state.regenTimer.Reset(d)
	state.regenChanDrained = false
}

// clearSideChainTracking removes all tracking for minimum required votes on
// side chain blocks as well as clears the associated timeout that must
// transpire before said tracking is enabled.
func (state *regenHandlerState) clearSideChainTracking() {
	for hash := range state.awaitingSideChainMinVotes {
		delete(state.awaitingSideChainMinVotes, hash)
	}
	state.trackSideChainsTimeout = nil
}

// genTemplateAsync cancels any asynchronous block template that is already
// currently being generated and launches a new goroutine to asynchronously
// generate a new one with the provided reason.  It also handles updating the
// current template and error associated with the generator with the results in
// a concurrent safe fashion and, in the case a successful template is
// generated, notifies the subscription handler goroutine with the new template.
func (g *BgBlkTmplGenerator) genTemplateAsync(ctx context.Context, reason TemplateUpdateReason) {
	// Cancel any other templates that might currently be in the process of
	// being generated and create a new context that can be cancelled for the
	// new template that is about to be generated.
	g.cancelTemplateMtx.Lock()
	g.cancelTemplate()
	ctx, g.cancelTemplate = context.WithCancel(ctx)
	g.cancelTemplateMtx.Unlock()

	// Ensure that attempts to retrieve the current template block until the
	// new template is generated when it is because the parent has changed or
	// new votes are available in order to avoid handing out a template that
	// is guaranteed to be stale soon after.
	blockRetrieval := reason == TURNewParent || reason == TURNewVotes
	if blockRetrieval {
		g.staleTemplateWg.Add(1)
	}
	go func(ctx context.Context, reason TemplateUpdateReason, blockRetrieval bool) {
		if blockRetrieval {
			defer g.staleTemplateWg.Done()
		}

		// Pick a mining address at random and generate a block template that
		// pays to it.
		prng := rand.New(rand.NewSource(time.Now().Unix()))
		payToAddr := g.miningAddrs[prng.Intn(len(g.miningAddrs))]
		template, err := g.tg.NewBlockTemplate(payToAddr)
		// NOTE: err is handled below.
		if err != nil {
			log.Tracef("NewBlockTemplate: %v", err)
		}

		// Don't update the state or notify subscribers when the template
		// generation was cancelled.
		if ctx.Err() != nil {
			return
		}

		// Update the current template state with the results and notify
		// subscribed clients of the new template so long as it's valid.
		if err != nil {
			reason = turUnknown
		}
		g.setCurrentTemplate(template, reason, err)
		if err == nil && template != nil {
			// It is possible for a new vote to show up while the template for
			// a new parent is still being generated which causes that template
			// to be canceled in favor of the the new one with the vote.  So,
			// ensure the first notification sent for a new parent has that
			// reason.
			header := &template.Block.Header
			if reason == TURNewVotes {
				if !g.notifiedParents.Contains(header.PrevBlock) {
					reason = TURNewParent
				}
			}
			if reason == TURNewParent {
				g.notifiedParents.Add(header.PrevBlock)
			}

			// Ensure the goroutine exits cleanly during shutdown.
			select {
			case <-ctx.Done():
				return

			case g.notifySubscribers <- &TemplateNtfn{template, reason}:
			}
		}
	}(ctx, reason, blockRetrieval)
}

// curTplHasNumVotes returns whether or not the current template is valid,
// builds on the provided hash, and contains the specified number of votes.
func (g *BgBlkTmplGenerator) curTplHasNumVotes(votedOnHash *chainhash.Hash, numVotes uint16) bool {
	g.templateMtx.Lock()
	template, err := g.template, g.templateErr
	g.templateMtx.Unlock()
	if template == nil || err != nil {
		return false
	}
	if template.Block.Header.PrevBlock != *votedOnHash {
		return false
	}
	return template.Block.Header.Voters == numVotes
}

// numVotesForBlock returns the number of votes on the provided block hash that
// are known.
func (g *BgBlkTmplGenerator) numVotesForBlock(votedOnBlock *chainhash.Hash) uint16 {
	return uint16(len(g.tg.txSource.VoteHashesForBlock(votedOnBlock)))
}

// handleBlockConnected handles the rtBlockConnected event by either immediately
// generating a new template building on the block when it will still be prior
// to stake validation height or selectively setting up timeouts to give the
// votes a chance to propagate once the template will be at or after stake
// validation height.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleBlockConnected(ctx context.Context, state *regenHandlerState, block *dcrutil.Block, chainTip *blockchain.BestState) {
	// Clear all vote tracking when the current chain tip changes.
	state.awaitingMinVotesHash = nil
	state.clearSideChainTracking()

	// Nothing more to do if the connected block is not the current chain tip.
	// This can happen in rare cases such as if more than one new block shows up
	// while generating a template.  Due to the requirement for votes later in
	// the chain, it should almost never happen in practice once the chain has
	// progressed that far, however, it is required for correctness.  It is also
	// worth noting that it happens more frequently earlier in the chain before
	// voting starts, particularly in simulation networks with low difficulty.
	blockHeight := block.MsgBlock().Header.Height
	blockHash := block.Hash()
	if int64(blockHeight) != chainTip.Height || *blockHash != chainTip.Hash {
		return
	}

	// Generate a new template immediately when it will be prior to stake
	// validation height which means no votes are required.
	newTemplateHeight := blockHeight + 1
	if newTemplateHeight < uint32(g.tg.chainParams.StakeValidationHeight) {
		state.stopRegenTimer()
		state.failedGenRetryTimeout = nil
		state.baseBlockHash = *blockHash
		state.baseBlockHeight = blockHeight
		g.genTemplateAsync(ctx, TURNewParent)
		return
	}

	// At this point the template will be at or after stake validation height,
	// and therefore requires the inclusion of votes on the previous block to be
	// valid.

	// Generate a new template immediately when the maximum number of votes
	// for the block are already known.
	numVotes := g.numVotesForBlock(blockHash)
	if numVotes >= g.maxVotesPerBlock {
		state.stopRegenTimer()
		state.failedGenRetryTimeout = nil
		state.baseBlockHash = *blockHash
		state.baseBlockHeight = blockHeight
		g.genTemplateAsync(ctx, TURNewParent)
		return
	}

	// Update the state so the next template generated will build on the block
	// and set a timeout to give the remaining votes an opportunity to propagate
	// when the minimum number of required votes for the block are already
	// known.  This provides a balance between preferring to generate block
	// templates with max votes and not waiting too long before starting work on
	// the next block.
	if numVotes >= g.minVotesRequired {
		state.stopRegenTimer()
		state.failedGenRetryTimeout = nil
		state.baseBlockHash = *blockHash
		state.baseBlockHeight = blockHeight
		state.maxVotesTimeout = time.After(maxVoteTimeoutDuration)
		return
	}

	// Mark the state as waiting for the minimum number of required votes needed
	// to build a template on the block to be received and set a timeout to give
	// them an opportunity to propagate before attempting to find any other
	// variants that extend the same parent with enough votes to force a
	// reorganization.  This ensures the first block that is seen is chosen to
	// build templates on so long as it receives the minimum required votes in
	// order to prevent PoW miners from being able to gain an advantage through
	// vote withholding.
	//
	// Also, the regen timer for the current template is stopped since chances
	// are high that the votes will be received and it is ideal to avoid
	// regenerating a template that will likely be stale shortly.  The regen
	// timer is reset after the timeout if needed.
	state.stopRegenTimer()
	state.awaitingMinVotesHash = blockHash
	state.trackSideChainsTimeout = time.After(minVotesTimeoutDuration)
}

// handleBlockDisconnected handles the rtBlockDisconnected event by immediately
// generating a new template based on the new tip since votes for it are
// either necessarily already known due to being included in the block being
// disconnected or not required due to moving before stake validation height.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleBlockDisconnected(ctx context.Context, state *regenHandlerState, block *dcrutil.Block, chainTip *blockchain.BestState) {
	// Clear all vote tracking when the current chain tip changes.
	state.awaitingMinVotesHash = nil
	state.clearSideChainTracking()

	// Nothing more to do if the current chain tip is not the block prior to the
	// block that was disconnected.  This can happen in rare cases such as when
	// forcing disconnects via block invalidation.  In practice, disconnects
	// happen as a result of chain reorganizations and thus this code will not
	// be executed, however, it is required for correctness.
	prevHeight := block.MsgBlock().Header.Height - 1
	prevHash := &block.MsgBlock().Header.PrevBlock
	if int64(prevHeight) != chainTip.Height || *prevHash != chainTip.Hash {
		return
	}

	// NOTE: The block being disconnected necessarily has votes for the block
	// that is becoming the new tip and they should ideally be extracted here to
	// ensure they are available for use when building the template.  However,
	// the underlying template generator currently relies on pulling the votes
	// out of the mempool and performs this task itself.  In the future, the
	// template generator should ideally accept the votes to include directly.

	// Generate a new template building on the new tip.
	state.stopRegenTimer()
	state.failedGenRetryTimeout = nil
	state.baseBlockHash = *prevHash
	state.baseBlockHeight = prevHeight
	g.genTemplateAsync(ctx, TURNewParent)
}

// handleBlockAccepted handles the rtBlockAccepted event by establishing vote
// tracking for the block when it is a variant that extends the same parent as
// the current tip, the current tip does not have the minimum number of required
// votes, and the initial timeout to provide them an opportunity to propagate
// has already expired.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleBlockAccepted(_ context.Context, state *regenHandlerState, block *dcrutil.Block, chainTip *blockchain.BestState) {
	// Ignore side chain blocks while still waiting for the side chain tracking
	// timeout to expire.  This provides a bias towards the first block that is
	// seen in order to prevent PoW miners from being able to gain an advantage
	// through vote withholding.
	if state.trackSideChainsTimeout != nil {
		return
	}

	// Ignore side chain blocks when building on it would produce a block prior
	// to stake validation height which means no votes are required and
	// therefore no additional handling is necessary.
	blockHeight := block.MsgBlock().Header.Height
	newTemplateHeight := blockHeight + 1
	if newTemplateHeight < uint32(g.tg.chainParams.StakeValidationHeight) {
		return
	}

	// Ignore side chain blocks when the current tip already has enough votes
	// for a template to be built on it.  This ensures the first block that is
	// seen is chosen to build templates on so long as it receives the minimum
	// required votes in order to prevent PoW miners from being able to gain an
	// advantage through vote withholding.
	if state.awaitingMinVotesHash == nil {
		return
	}

	// Ignore blocks that are prior to the current tip.
	if blockHeight < uint32(chainTip.Height) {
		return
	}

	// Ignore main chain tip block since it is handled by the connect path.
	blockHash := block.Hash()
	if *blockHash == chainTip.Hash {
		return
	}

	// Ignore side chain blocks when the current template is already building on
	// the current tip or the accepted block is not a sibling of the current
	// best chain tip.
	alreadyBuildingOnCurTip := state.baseBlockHash == chainTip.Hash
	acceptedPrevHash := &block.MsgBlock().Header.PrevBlock
	if alreadyBuildingOnCurTip || *acceptedPrevHash != chainTip.PrevHash {
		return
	}

	// Setup tracking for votes on the block.
	state.awaitingSideChainMinVotes[*blockHash] = struct{}{}
}

// handleVote handles the rtVote event by determining if the vote is for a block
// the current state is monitoring and reacting accordingly.  At a high level,
// this entails either establishing a timeout once the minimum number of
// required votes for the current tip have been received to provide the
// remaining votes an opportunity to propagate, regenerating the current
// template as a result of the vote, or potentially reorganizing the chain to a
// new tip that has enough votes in the case the current tip is unable to obtain
// the required votes.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleVote(ctx context.Context, state *regenHandlerState, voteTx *dcrutil.Tx, chainTip *blockchain.BestState) {
	votedOnHash, _ := stake.SSGenBlockVotedOn(voteTx.MsgTx())

	// The awaiting min votes hash is selectively set once a block is connected
	// such that a new template that builds on it will be at or after stake
	// validation height until the minimum number of votes required to build a
	// template are received.
	//
	// Update the state so the next template generated will build on the current
	// tip once at least the minimum number of required votes for it has been
	// received and either set a timeout to give the remaining votes an
	// opportunity to propagate if the maximum number of votes is not already
	// known or generate a new template immediately when they are.  This
	// provides a balance between preferring to generate block templates with
	// max votes and not waiting too long before starting work on the next
	// block.
	minVotesHash := state.awaitingMinVotesHash
	if minVotesHash != nil && votedOnHash == *minVotesHash {
		numVotes := g.numVotesForBlock(minVotesHash)
		log.Debugf("Received vote %s for tip block %s (%d total)",
			voteTx.Hash(), minVotesHash, numVotes)
		if numVotes >= g.minVotesRequired {
			// Ensure the next template generated builds on the tip and clear
			// all vote tracking to lock the current tip in now that it
			// has the minimum required votes.
			state.stopRegenTimer()
			state.failedGenRetryTimeout = nil
			state.baseBlockHash = *minVotesHash
			state.baseBlockHeight = uint32(chainTip.Height)
			state.awaitingMinVotesHash = nil
			state.clearSideChainTracking()

			// Generate a new template immediately when the maximum number of
			// votes for the block are already known.
			if numVotes >= g.maxVotesPerBlock {
				g.genTemplateAsync(ctx, TURNewParent)
				return
			}

			// Set a timeout to give the remaining votes an opportunity to
			// propagate.
			state.maxVotesTimeout = time.After(maxVoteTimeoutDuration)
		}
		return
	}

	// Generate a template on new votes for the block the current state is
	// configured to build the next block template on when either the maximum
	// number of votes is received for it or once the minimum number of required
	// votes has been received and the propagation delay timeout that is started
	// upon receipt of said minimum votes has expired.
	//
	// Note that the base block hash is only updated to the current tip once it
	// has received the minimum number of required votes, so this will continue
	// to detect votes for the parent of the current tip prior to the point the
	// new tip has received enough votes.
	//
	// This ensures new templates that include the new votes are generated
	// immediately upon receiving the maximum number of votes as well as any
	// additional votes that arrive after the initial timeout.
	if votedOnHash == state.baseBlockHash {
		// Avoid regenerating the current template if it is already building on
		// the expected block and already has the maximum number of votes.
		if g.curTplHasNumVotes(&votedOnHash, g.maxVotesPerBlock) {
			state.maxVotesTimeout = nil
			return
		}

		numVotes := g.numVotesForBlock(&votedOnHash)
		log.Debugf("Received vote %s for current template %s (%d total)",
			voteTx.Hash(), votedOnHash, numVotes)
		if numVotes >= g.maxVotesPerBlock || state.maxVotesTimeout == nil {
			// The template needs to be updated due to a new parent the first
			// time it is generated and due to new votes on subsequent votes.
			// The max votes timeout is only non-nil before the first time it is
			// generated.
			tplUpdateReason := TURNewVotes
			if state.maxVotesTimeout != nil {
				tplUpdateReason = TURNewParent
			}

			// Cancel the max votes timeout (if set).
			state.maxVotesTimeout = nil

			state.stopRegenTimer()
			state.failedGenRetryTimeout = nil
			g.genTemplateAsync(ctx, tplUpdateReason)
			return
		}
	}

	// Reorganize to an alternative chain tip when it receives at least the
	// minimum required number of votes in the case the current chain tip does
	// not receive the minimum number of required votes within an initial
	// timeout period.
	//
	// Note that the potential side chain blocks to consider are only populated
	// in the aforementioned case.
	if _, ok := state.awaitingSideChainMinVotes[votedOnHash]; ok {
		numVotes := g.numVotesForBlock(&votedOnHash)
		log.Debugf("Received vote %s for side chain block %s (%d total)",
			voteTx.Hash(), votedOnHash, numVotes)
		if numVotes >= g.minVotesRequired {
			err := g.chain.ForceHeadReorganization(chainTip.Hash, votedOnHash)
			if err != nil {
				return
			}

			// Prevent votes on other tip candidates from causing reorg again
			// since the new chain tip has enough votes.
			state.clearSideChainTracking()
			return
		}
	}
}

// handleTemplateUpdate handles the rtTemplateUpdate event by updating the state
// accordingly.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleTemplateUpdate(state *regenHandlerState, tplUpdate templateUpdate) {
	// Schedule a template regen if it failed to generate for some reason.  This
	// should be exceedingly rare in practice.
	if tplUpdate.err != nil && state.failedGenRetryTimeout == nil {
		state.failedGenRetryTimeout = time.After(time.Second)
		return
	}
	if tplUpdate.template == nil {
		return
	}

	// Ensure the base block details match the template.
	state.baseBlockHash = tplUpdate.template.Block.Header.PrevBlock
	state.baseBlockHeight = tplUpdate.template.Block.Header.Height - 1

	// Update the state related to template regeneration due to new regular
	// transactions.
	state.lastGeneratedTime = time.Now().Unix()
	state.resetRegenTimer(templateRegenSecs * time.Second)
}

// handleForceRegen handles the rtForceRegen event by initiating the generation
// of a new template.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleForceRegen(ctx context.Context, state *regenHandlerState) {
	// Ignore requests to force regeneration if the minimum amount of votes
	// has been received and it's just waiting for the last ones to arrive.
	// The template will be regenerated shortly in that case.
	if state.maxVotesTimeout != nil {
		return
	}

	state.stopRegenTimer()
	state.failedGenRetryTimeout = nil
	g.genTemplateAsync(ctx, turUnknown)
}

// handleRegenEvent handles all regen events by determining the event reason and
// reacting accordingly.  For example, it calls the appropriate associated event
// handler for the events that have one and prevents templates from being
// generating in the middle of reorgs.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleRegenEvent(ctx context.Context, state *regenHandlerState, event regenEvent) {
	// Handle chain reorg messages up front since all of the following logic
	// only applies when not in the middle of reorganizing.
	switch event.reason {
	case rtReorgStarted:
		// Ensure that attempts to retrieve the current template block until the
		// new template after the reorg is generated.
		g.staleTemplateWg.Add(1)

		// Mark the state as reorganizing.
		state.isReorganizing = true

		// Stop all timeouts and clear all vote tracking.
		state.stopRegenTimer()
		state.failedGenRetryTimeout = nil
		state.awaitingMinVotesHash = nil
		state.maxVotesTimeout = nil
		state.clearSideChainTracking()

		// Clear the current template and associated base block for the next
		// generated template.
		g.setCurrentTemplate(nil, turUnknown, nil)
		state.baseBlockHash = zeroHash
		state.baseBlockHeight = 0
		return

	case rtReorgDone:
		state.isReorganizing = false

		// Treat the tip block as if it was just connected when a reorganize
		// finishes so the existing code paths are run.
		//
		// An error should be impossible here since the request is for the block
		// the chain believes is the current tip which means it must exist.
		chainTip := g.chain.BestSnapshot()
		tipBlock, err := g.chain.BlockByHash(&chainTip.Hash)
		if err != nil {
			g.setCurrentTemplate(nil, turUnknown, err)
		} else {
			g.handleBlockConnected(ctx, state, tipBlock, chainTip)
		}

		g.staleTemplateWg.Done()
		return
	}

	// Do not generate block templates when the chain is in the middle of
	// reorganizing.
	if state.isReorganizing {
		return
	}

	// Do not generate block templates when the chain is not synced unless
	// specifically requested to.
	if !g.allowUnsyncedMining && !g.tg.blockManager.IsCurrent() {
		return
	}

	chainTip := g.chain.BestSnapshot()
	switch event.reason {
	case rtBlockConnected:
		block := event.value.(*dcrutil.Block)
		g.handleBlockConnected(ctx, state, block, chainTip)

	case rtBlockDisconnected:
		block := event.value.(*dcrutil.Block)
		g.handleBlockDisconnected(ctx, state, block, chainTip)

	case rtBlockAccepted:
		block := event.value.(*dcrutil.Block)
		g.handleBlockAccepted(ctx, state, block, chainTip)

	case rtVote:
		voteTx := event.value.(*dcrutil.Tx)
		g.handleVote(ctx, state, voteTx, chainTip)

	case rtTemplateUpdated:
		tplUpdate := event.value.(templateUpdate)
		g.handleTemplateUpdate(state, tplUpdate)

	case rtForceRegen:
		g.handleForceRegen(ctx, state)
	}
}

// tipSiblingsSortedByVotes returns all blocks other than the current tip block
// that also extend its parent sorted by the number of votes each has in
// descending order.
func (g *BgBlkTmplGenerator) tipSiblingsSortedByVotes(state *regenHandlerState) []*blockWithNumVotes {
	// Obtain all of the current blocks that extend the same parent as the
	// current tip.  The error is ignored here because it is deprecated.
	generation, _ := g.chain.TipGeneration()

	// Nothing else to consider if there is only a single block which will be
	// the current tip itself.
	if len(generation) <= 1 {
		return nil
	}

	siblings := make([]*blockWithNumVotes, 0, len(generation)-1)
	for i := range generation {
		hash := &generation[i]
		if *hash == *state.awaitingMinVotesHash {
			continue
		}

		numVotes := g.numVotesForBlock(hash)
		siblings = append(siblings, &blockWithNumVotes{
			Hash:     *hash,
			NumVotes: numVotes,
		})
	}
	sort.Sort(sort.Reverse(byNumberOfVotes(siblings)))
	return siblings
}

// handleTrackSideChainsTimeout handles potentially reorganizing the chain to a
// side chain block with the most votes in the case the minimum number of
// votes needed to build a block template on the current tip have not been
// received within a certain timeout.
//
// It also doubles to reset the regen timer for the current template in the case
// no validate candidates are found since it is disabled when setting up this
// timeout to prevent creating new templates that would very likely be stale
// soon after.
//
// This function is only intended for use by the regen handler goroutine.
func (g *BgBlkTmplGenerator) handleTrackSideChainsTimeout(ctx context.Context, state *regenHandlerState) {
	// Don't allow side chain variants to override the current tip when it
	// already has the minimum required votes.
	if state.awaitingMinVotesHash == nil {
		return
	}

	// Reorganize the chain to a valid sibling of the current tip that has at
	// least the minimum number of required votes while preferring the most
	// votes.
	//
	// Also, while looping, add each tip the map of side chain blocks to monitor
	// for votes in the event there are not currently any eligible candidates
	// since they may become eligible as votes arrive.
	sortedSiblings := g.tipSiblingsSortedByVotes(state)
	for _, sibling := range sortedSiblings {
		if sibling.NumVotes >= g.minVotesRequired {
			err := g.chain.ForceHeadReorganization(*state.awaitingMinVotesHash,
				sibling.Hash)
			if err != nil {
				// Try the next block in the case of failure to reorg.
				continue
			}

			// Prevent votes on other tip candidates from causing reorg again
			// since the new chain tip has enough votes.  The reorg event clears
			// the state, but, since there is a backing queue for the events,
			// and the reorg itself might haven taken a bit of time, it could
			// allow new side chain blocks or votes on existing ones in before
			// the reorg events are processed.  Thus, update the state to
			// indicate the next template is to be built on the new tip to
			// prevent any possible logic races.
			state.awaitingMinVotesHash = nil
			state.clearSideChainTracking()
			state.stopRegenTimer()
			state.failedGenRetryTimeout = nil
			state.baseBlockHash = sibling.Hash
			return
		}

		state.awaitingSideChainMinVotes[sibling.Hash] = struct{}{}
	}

	// Generate a new template building on the parent of the current tip when
	// there is not already an existing template and the initial timeout has
	// elapsed upon receiving the new tip without receiving votes for it.  There
	// will typically only not be an existing template when the generator is
	// first instantiated and after a chain reorganization.
	if state.baseBlockHash == zeroHash {
		chainTip := g.chain.BestSnapshot()
		state.failedGenRetryTimeout = nil
		state.baseBlockHash = chainTip.PrevHash
		state.baseBlockHeight = uint32(chainTip.Height - 1)
		g.genTemplateAsync(ctx, TURNewParent)
		return
	}

	// At this point, no viable candidates to change the current template were
	// found, so reset the regen timer for the current template.
	state.resetRegenTimer(templateRegenSecs * time.Second)
}

// regenHandler is the main workhorse for generating new templates in response
// to regen events and also handles generating a new template during initial
// startup.
//
// This must be run as a goroutine.
func (g *BgBlkTmplGenerator) regenHandler(ctx context.Context) {
	// Treat the tip block as if it was just connected when starting up so the
	// existing code paths are run.
	tipBlock, err := g.chain.BlockByHash(&g.chain.BestSnapshot().Hash)
	if err != nil {
		g.setCurrentTemplate(nil, turUnknown, err)
	} else {
		select {
		case <-ctx.Done():
			g.wg.Done()
			return
		case g.queueRegenEvent <- regenEvent{rtBlockConnected, tipBlock}:
		}
	}

	state := makeRegenHandlerState()
	for {
		select {
		case event := <-g.regenEventMsgs:
			g.handleRegenEvent(ctx, &state, event)

		// This timeout is selectively enabled once the minimum number of
		// required votes has been received in order to give the remaining votes
		// an opportunity to propagate.  It is disabled if the remaining votes
		// are received prior to the timeout.
		case <-state.maxVotesTimeout:
			state.maxVotesTimeout = nil
			g.genTemplateAsync(ctx, TURNewParent)

		// This timeout is selectively enabled when a new block is connected in
		// order to give the minimum number of required votes needed to build a
		// block template on it an opportunity to propagate before attempting to
		// find any other variants that extend the same parent as the current
		// tip with enough votes to force a reorganization.  This ensures the
		// first block that is seen is chosen to build templates on so long as
		// it receives the minimum required votes in order to prevent PoW miners
		// from being able to gain an advantage through vote withholding.  It is
		// disabled if the minimum number of votes is received prior to the
		// timeout.
		case <-state.trackSideChainsTimeout:
			state.trackSideChainsTimeout = nil
			g.handleTrackSideChainsTimeout(ctx, &state)

		// This timeout is selectively enabled once a template has been
		// generated in order to allow the template to be periodically
		// regenerated with new transactions.  Note that votes have special
		// handling as described above.
		case <-state.regenTimer.C:
			// Mark the timer's channel as having been drained so the timer can
			// safely be reset.
			state.regenChanDrained = true

			// Generate a new template when there are new transactions
			// available.
			if g.tg.txSource.LastUpdated().Unix() > state.lastGeneratedTime {
				state.failedGenRetryTimeout = nil
				g.genTemplateAsync(ctx, TURNewTxns)
				continue
			}

			// There are no new transactions to include and the initial timeout
			// has been triggered, so reset the timer to check again in one
			// second.
			state.resetRegenTimer(time.Second)

		// This timeout is selectively enabled in the rare case a template fails
		// to generate and disabled prior to attempts at generating a new one.
		case <-state.failedGenRetryTimeout:
			state.failedGenRetryTimeout = nil
			g.genTemplateAsync(ctx, TURNewParent)

		case <-ctx.Done():
			g.wg.Done()
			return
		}
	}
}

// ChainReorgStarted informs the background block template generator that a
// chain reorganization has started.  It is caller's responsibility to ensure
// this is only invoked as described.
func (g *BgBlkTmplGenerator) ChainReorgStarted() {
	g.sendQueueRegenEvent(regenEvent{rtReorgStarted, nil})
}

// ChainReorgDone informs the background block template generator that a chain
// reorganization has completed.  It is caller's responsibility to ensure this
// is only invoked as described.
func (g *BgBlkTmplGenerator) ChainReorgDone() {
	g.sendQueueRegenEvent(regenEvent{rtReorgDone, nil})
}

// BlockAccepted informs the background block template generator that a block
// has been accepted to the block chain.  It is caller's responsibility to
// ensure this is only invoked as described.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) BlockAccepted(block *dcrutil.Block) {
	g.sendQueueRegenEvent(regenEvent{rtBlockAccepted, block})
}

// BlockConnected informs the background block template generator that a block
// has been connected to the main chain.  It is caller's responsibility to
// ensure this is only invoked as described.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) BlockConnected(block *dcrutil.Block) {
	g.sendQueueRegenEvent(regenEvent{rtBlockConnected, block})
}

// BlockDisconnected informs the background block template generator that a
// block has been disconnected from the main chain.  It is caller's
// responsibility to ensure this is only invoked as described.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) BlockDisconnected(block *dcrutil.Block) {
	g.sendQueueRegenEvent(regenEvent{rtBlockDisconnected, block})
}

// VoteReceived informs the background block template generator that a new vote
// has been received.  It is the caller's responsibility to ensure this is only
// invoked with valid votes.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) VoteReceived(tx *dcrutil.Tx) {
	g.sendQueueRegenEvent(regenEvent{rtVote, tx})
}

// ForceRegen asks the background block template generator to generate a new
// template, independently of most of its internal timers.
//
// Note that there is no guarantee on whether a new template will actually be
// generated or when. This function does _not_ block until a new template is
// generated.
//
// This function is safe for concurrent access.
func (g *BgBlkTmplGenerator) ForceRegen() {
	g.sendQueueRegenEvent(regenEvent{rtForceRegen, nil})
}

// Run starts the background block template generator and all other goroutines
// necessary for it to function properly and blocks until the provided context
// is cancelled.
func (g *BgBlkTmplGenerator) Run(ctx context.Context) {
	g.wg.Add(4)
	go g.regenQueueHandler(ctx)
	go g.regenHandler(ctx)
	go g.notifySubscribersHandler(ctx)
	go func() {
		<-ctx.Done()
		close(g.quit)
		g.wg.Done()
	}()
	g.wg.Wait()
}
