// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package blockchain implements Decred block handling and chain selection
// rules.
package blockchain

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/bits"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/gcs/v4"
	"github.com/decred/dcrd/gcs/v4/blockcf2"
	"github.com/decred/dcrd/internal/blockchain/indexers"
	"github.com/decred/dcrd/lru"
	"github.com/decred/dcrd/math/uint256"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/wire"
)

const (
	// minMemoryStakeNodes is the maximum height to keep stake nodes
	// in memory for in their respective nodes.  Beyond this height,
	// they will need to be manually recalculated.  This value should
	// be at least the stake retarget interval.
	minMemoryStakeNodes = 288

	// recentBlockCacheSize is the number of recent blocks to keep in memory.
	// This value is set based on the target block time for the main network
	// such that there is approximately one hour of blocks cached.  This could
	// be made network independent and calculated based on the parameters, but
	// that would result in larger caches than desired for other networks.
	recentBlockCacheSize = 12

	// contextCheckCacheSize is the number of recent successful contextual block
	// check results to keep in memory.
	contextCheckCacheSize = 25
)

// panicf is a convenience function that formats according to the given format
// specifier and arguments and then logs the result at the critical level and
// panics with it.
func panicf(format string, args ...interface{}) {
	str := fmt.Sprintf(format, args...)
	log.Critical(str)
	panic(str)
}

// BlockLocator is used to help locate a specific block.  The algorithm for
// building the block locator is to add the hashes in reverse order until
// the genesis block is reached.  In order to keep the list of locator hashes
// to a reasonable number of entries, first the most recent previous 12 block
// hashes are added, then the step is doubled each loop iteration to
// exponentially decrease the number of hashes as a function of the distance
// from the block being located.
//
// For example, assume a block chain with a side chain as depicted below:
//
//	genesis -> 1 -> 2 -> ... -> 15 -> 16  -> 17  -> 18
//	                              \-> 16a -> 17a
//
// The block locator for block 17a would be the hashes of blocks:
// [17a 16a 15 14 13 12 11 10 9 8 7 6 4 genesis]
type BlockLocator []*chainhash.Hash

// BestState houses information about the current best block and other info
// related to the state of the main chain as it exists from the point of view of
// the current best block.
//
// The BestSnapshot method can be used to obtain access to this information
// in a concurrent safe manner and the data will not be changed out from under
// the caller when chain state changes occur as the function name implies.
// However, the returned snapshot must be treated as immutable since it is
// shared by all callers.
type BestState struct {
	Hash                chainhash.Hash   // The hash of the block.
	PrevHash            chainhash.Hash   // The previous block hash.
	Height              int64            // The height of the block.
	Bits                uint32           // The difficulty bits of the block.
	NextPoolSize        uint32           // The ticket pool size.
	NextStakeDiff       int64            // The next stake difficulty.
	BlockSize           uint64           // The size of the block.
	NumTxns             uint64           // The number of txns in the block.
	TotalTxns           uint64           // The total number of txns in the chain.
	MedianTime          time.Time        // Median time as per CalcPastMedianTime.
	TotalSubsidy        int64            // The total subsidy for the chain.
	NextExpiringTickets []chainhash.Hash // The tickets set to expire next block.
	NextWinningTickets  []chainhash.Hash // The eligible tickets to vote on the next block.
	MissedTickets       []chainhash.Hash // The missed tickets set to be revoked.
	NextFinalState      [6]byte          // The calculated state of the lottery for the next block.
}

// newBestState returns a new best stats instance for the given parameters.
func newBestState(node *blockNode, blockSize, numTxns, totalTxns uint64,
	medianTime time.Time, totalSubsidy int64, nextPoolSize uint32,
	nextStakeDiff int64, nextExpiring, nextWinners, missed []chainhash.Hash,
	nextFinalState [6]byte) *BestState {
	prevHash := *zeroHash
	if node.parent != nil {
		prevHash = node.parent.hash
	}
	return &BestState{
		Hash:                node.hash,
		PrevHash:            prevHash,
		Height:              node.height,
		Bits:                node.bits,
		NextPoolSize:        nextPoolSize,
		NextStakeDiff:       nextStakeDiff,
		BlockSize:           blockSize,
		NumTxns:             numTxns,
		TotalTxns:           totalTxns,
		MedianTime:          medianTime,
		TotalSubsidy:        totalSubsidy,
		NextExpiringTickets: nextExpiring,
		NextWinningTickets:  nextWinners,
		MissedTickets:       missed,
		NextFinalState:      nextFinalState,
	}
}

// deploymentInfo houses information about the state of a consensus rule change
// deployment.
type deploymentInfo struct {
	// version is the associated deployment version.
	version uint32

	// deployment houses the deployment parameters for the consensus rule change
	// vote.
	deployment *chaincfg.ConsensusDeployment

	// forcedState optionally specifies a threshold state to use instead of
	// determining the state via the normal means of tallying votes.  This only
	// applies when it is not nil and is only populated when the associated
	// chain parameters specify a forced choice.
	forcedState *ThresholdStateTuple

	// cache is used to efficiently keep track of the threshold state for the
	// deployment.
	cache *thresholdStateCache
}

// BlockChain provides functions for working with the Decred block chain.  It
// includes functionality such as rejecting duplicate blocks, ensuring blocks
// follow all rules, checkpoint handling, and best chain selection with
// reorganization.
type BlockChain struct {
	// The following fields are set when the instance is created and can't
	// be changed afterwards, so there is no need to protect them with a
	// separate mutex.
	assumeValid              chainhash.Hash
	allowOldForks            bool
	expectedBlocksInTwoWeeks int64
	deploymentData           map[string]deploymentInfo
	minKnownWork             *uint256.Uint256
	minTestNetTarget         *big.Int
	minTestNetDiffBits       uint32
	db                       database.DB
	dbInfo                   *databaseInfo
	chainParams              *chaincfg.Params
	timeSource               MedianTimeSource
	notifications            NotificationCallback
	sigCache                 *txscript.SigCache
	indexSubscriber          *indexers.IndexSubscriber
	interrupt                <-chan struct{}
	utxoCache                UtxoCacher

	// subsidyCache is the cache that provides quick lookup of subsidy
	// values.
	subsidyCache *standalone.SubsidyCache

	// processLock protects concurrent access to overall chain processing
	// independent from the chain lock which is periodically released to
	// send notifications.
	processLock sync.Mutex

	// chainLock protects concurrent access to the vast majority of the
	// fields in this struct below this point.
	chainLock sync.RWMutex

	// These fields are related to the memory block index.  They both have
	// their own locks, however they are often also protected by the chain
	// lock to help prevent logic races when blocks are being processed.
	//
	// index houses the entire block index in memory.  The block index is
	// a tree-shaped structure.
	//
	// bestChain tracks the current active chain by making use of an
	// efficient chain view into the block index.
	index     *blockIndex
	bestChain *chainView

	// isCurrentLatch tracks whether or not the chain believes it is current in
	// such a way that once it becomes current it latches to that state unless
	// the chain falls too far behind again which likely indicates it is forked
	// from the network.  It is protected by the chain lock.
	isCurrentLatch bool

	// These fields house caches for blocks to facilitate faster chain reorgs,
	// block connection, and more efficient recent block serving.
	//
	// recentBlocks houses a block cache of block data that has been seen
	// recently.
	//
	// recentContextChecks tracks recent blocks that have successfully passed
	// all contextual checks and is primarily used as an optimization to avoid
	// running the checks again when possible.
	recentBlocks        lru.KVCache
	recentContextChecks lru.Cache

	// These fields house a cached view that represents a block that votes
	// against its parent and therefore contains all changes as a result
	// of disconnecting all regular transactions in its parent.  It is only
	// lazily updated to the current tip when fetching a utxo view via the
	// FetchUtxoView function with the flag indicating the block votes against
	// the parent set.
	disapprovedViewLock sync.Mutex
	disapprovedView     *UtxoViewpoint

	// rejectForksCheckpoint tracks the block to treat as a checkpoint for the
	// purposes of rejecting old forks.  It will be nil when no suitable block
	// is known or old forks are allowed for the current network.
	//
	// It is protected by the chain lock.
	rejectForksCheckpoint *blockNode

	// assumeValidNode tracks the assumed valid block.  It will be nil when a
	// block header with the assumed valid block hash has not been discovered or
	// when assume valid is disabled.  It is protected by the chain lock.
	assumeValidNode *blockNode

	// The state is used as a fairly efficient way to cache information
	// about the current best chain state that is returned to callers when
	// requested.  It operates on the principle of MVCC such that any time a
	// new block becomes the best block, the state pointer is replaced with
	// a new struct and the old state is left untouched.  In this way,
	// multiple callers can be pointing to different best chain states.
	// This is acceptable for most callers because the state is only being
	// queried at a specific point in time.
	//
	// In addition, some of the fields are stored in the database so the
	// chain state can be quickly reconstructed on load.
	stateLock     sync.RWMutex
	stateSnapshot *BestState

	// pruner is the automatic pruner for block nodes and stake nodes,
	// so that the memory may be restored by the garbage collector if
	// it is unlikely to be referenced in the future.
	pruner *chainPruner

	// The following maps are various caches for the stake version/voting
	// system.  The goal of these is to reduce disk access to load blocks
	// from disk.  Measurements indicate that it is slightly more expensive
	// so setup the cache (<10%) vs doing a straight chain walk.  Every
	// other subsequent call is >10x faster.
	isVoterMajorityVersionCache   map[[stakeMajorityCacheKeySize]byte]bool
	isStakeMajorityVersionCache   map[[stakeMajorityCacheKeySize]byte]bool
	calcPriorStakeVersionCache    map[[chainhash.HashSize]byte]uint32
	calcVoterVersionIntervalCache map[[chainhash.HashSize]byte]uint32
	calcStakeVersionCache         map[[chainhash.HashSize]byte]uint32

	// cachedBlake3WorkDiffCandidateAnchor houses a cached candidate anchor
	// point to use for the difficulty algorithm defined in DCP0011 before the
	// state of the blake3 proof of work agenda can be determined definitively.
	//
	// This is set based on heuristics that make it very likely to be the
	// correct anchor point and exists to avoid additional work that would
	// otherwise be required during the initial header sync.
	cachedBlake3WorkDiffCandidateAnchor atomic.Pointer[blockNode]

	// cachedBlake3WorkDiffAnchor houses a cached anchor point to use for the
	// difficulty algorithm defined in DCP0011.
	//
	// It is only set when the blake3 proof of work agenda has been determined
	// to be active and will be the block just prior to the activation of the
	// agenda.  It will not be set for networks where the agenda is always
	// active such as the simulation network.
	cachedBlake3WorkDiffAnchor atomic.Pointer[blockNode]

	// bulkImportMode provides a mechanism to indicate that several validation
	// checks can be avoided when bulk importing blocks already known to be valid.
	// It is protected by the chain lock.
	bulkImportMode bool
}

// ChainParams returns the chain parameters.
func (b *BlockChain) ChainParams() *chaincfg.Params {
	return b.chainParams
}

const (
	// stakeMajorityCacheKeySize is comprised of the stake version and the
	// hash size.  The stake version is a little endian uint32, hence we
	// add 4 to the overall size.
	stakeMajorityCacheKeySize = 4 + chainhash.HashSize
)

// StakeVersions is a condensed form of a dcrutil.Block that is used to prevent
// using gigabytes of memory.
type StakeVersions struct {
	Hash         chainhash.Hash
	Height       int64
	BlockVersion int32
	StakeVersion uint32
	Votes        []stake.VoteVersionTuple
}

// GetStakeVersions returns a cooked array of StakeVersions.  We do this in
// order to not bloat memory by returning raw blocks.
func (b *BlockChain) GetStakeVersions(hash *chainhash.Hash, count int32) ([]StakeVersions, error) {
	startNode := b.index.LookupNode(hash)
	if startNode == nil || !b.index.CanValidate(startNode) {
		return nil, unknownBlockError(hash)
	}

	// Nothing to do if no count requested.
	if count == 0 {
		return nil, nil
	}

	if count < 0 {
		return nil, fmt.Errorf("count must not be less than zero - "+
			"got %d", count)
	}

	// Limit the requested count to the max possible for the requested block.
	if count > int32(startNode.height+1) {
		count = int32(startNode.height + 1)
	}

	result := make([]StakeVersions, 0, count)
	prevNode := startNode
	for i := int32(0); prevNode != nil && i < count; i++ {
		sv := StakeVersions{
			Hash:         prevNode.hash,
			Height:       prevNode.height,
			BlockVersion: prevNode.blockVersion,
			StakeVersion: prevNode.stakeVersion,
			Votes:        prevNode.votes,
		}

		result = append(result, sv)

		prevNode = prevNode.parent
	}

	return result, nil
}

// VoteInfo represents information on agendas and their respective states for
// a consensus deployment.
type VoteInfo struct {
	Agendas      []chaincfg.ConsensusDeployment
	AgendaStatus []ThresholdStateTuple
}

// EnableBulkImportMode provides a mechanism to indicate that several validation
// checks can be avoided when bulk importing blocks already known to be valid.
// This must NOT be enabled in any other circumstance where blocks need to be
// fully validated.
//
// This function is safe for concurrent access.
func (b *BlockChain) EnableBulkImportMode(bulkImportMode bool) {
	b.chainLock.Lock()
	b.bulkImportMode = bulkImportMode
	b.chainLock.Unlock()
}

// GetVoteInfo returns information on consensus deployment agendas and their
// respective states at the provided hash, for the provided deployment version.
func (b *BlockChain) GetVoteInfo(hash *chainhash.Hash, version uint32) (*VoteInfo, error) {
	deployments, ok := b.chainParams.Deployments[version]
	if !ok {
		str := fmt.Sprintf("stake version %d does not exist", version)
		return nil, contextError(ErrUnknownDeploymentVersion, str)
	}

	vi := VoteInfo{
		Agendas: make([]chaincfg.ConsensusDeployment,
			0, len(deployments)),
		AgendaStatus: make([]ThresholdStateTuple, 0, len(deployments)),
	}
	for _, deployment := range deployments {
		vi.Agendas = append(vi.Agendas, deployment)
		status, err := b.NextThresholdState(hash, deployment.Vote.Id)
		if err != nil {
			return nil, err
		}
		vi.AgendaStatus = append(vi.AgendaStatus, status)
	}

	return &vi, nil
}

// HaveHeader returns whether or not the chain instance has the block header
// represented by the passed hash.  Note that this will return true for both the
// main chain and any side chains.
//
// This function is safe for concurrent access.
func (b *BlockChain) HaveHeader(hash *chainhash.Hash) bool {
	return b.index.LookupNode(hash) != nil
}

// HaveBlock returns whether or not the chain instance has the block represented
// by the passed hash.  This includes checking the various places a block can
// be like part of the main chain or on a side chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) HaveBlock(hash *chainhash.Hash) bool {
	return b.index.HaveBlock(hash)
}

// ChainWork returns the total work up to and including the block of the
// provided block hash.
func (b *BlockChain) ChainWork(hash *chainhash.Hash) (uint256.Uint256, error) {
	node := b.index.LookupNode(hash)
	if node == nil {
		return uint256.Uint256{}, unknownBlockError(hash)
	}

	return node.workSum, nil
}

// TipGeneration returns the entire generation of blocks stemming from the
// parent of the current tip.
//
// The function is safe for concurrent access.
func (b *BlockChain) TipGeneration() []chainhash.Hash {
	var nodeHashes []chainhash.Hash
	b.chainLock.Lock()
	b.index.RLock()
	entry := b.index.chainTips[b.bestChain.Tip().height]
	if entry.tip != nil {
		nodeHashes = make([]chainhash.Hash, 0, len(entry.otherTips)+1)
		nodeHashes = append(nodeHashes, entry.tip.hash)
		for _, n := range entry.otherTips {
			nodeHashes = append(nodeHashes, n.hash)
		}
	}
	b.index.RUnlock()
	b.chainLock.Unlock()
	return nodeHashes
}

// addRecentBlock adds a block to the recent block LRU cache and evicts the
// least recently used item if needed.
//
// This function is safe for concurrent access.
func (b *BlockChain) addRecentBlock(block *dcrutil.Block) {
	b.recentBlocks.Add(*block.Hash(), block)
}

// lookupRecentBlock attempts to return the requested block from the recent
// block LRU cache.  When the block exists, it will be made the most recently
// used item.
//
// This function is safe for concurrent access.
func (b *BlockChain) lookupRecentBlock(hash *chainhash.Hash) (*dcrutil.Block, bool) {
	block, ok := b.recentBlocks.Lookup(*hash)
	if ok {
		return block.(*dcrutil.Block), true
	}
	return nil, false
}

// fetchMainChainBlockByNode returns the block from the main chain associated
// with the given node.  It first attempts to use cache and then falls back to
// loading it from the database.
//
// An error is returned if the block is either not found or not in the main
// chain.
//
// This function MUST be called with the chain lock held (for reads).
func (b *BlockChain) fetchMainChainBlockByNode(node *blockNode) (*dcrutil.Block, error) {
	// Ensure the block is in the main chain.
	if !b.bestChain.Contains(node) {
		str := fmt.Sprintf("block %s is not in the main chain", node.hash)
		return nil, errNotInMainChain(str)
	}

	// Attempt to load the block from the recent block cache.
	block, ok := b.lookupRecentBlock(&node.hash)
	if ok {
		return block, nil
	}

	// Load the block from the database.
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		block, err = dbFetchBlockByNode(dbTx, node)
		return err
	})
	return block, err
}

// fetchBlockByNode returns the block associated with the given node all known
// sources such as the internal caches and the database.  This function returns
// blocks regardless or whether or not they are part of the main chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) fetchBlockByNode(node *blockNode) (*dcrutil.Block, error) {
	// Attempt to load the block from the recent block cache.
	block, ok := b.lookupRecentBlock(&node.hash)
	if ok {
		return block, nil
	}

	// Load the block from the database.
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		block, err = dbFetchBlockByNode(dbTx, node)
		return err
	})
	return block, err
}

// pruneStakeNodes removes references to old stake nodes which should no
// longer be held in memory so as to keep the maximum memory usage down.
// It proceeds from the bestNode back to the determined minimum height node,
// finds all the relevant children, and then drops the stake nodes from
// them by assigning nil and allowing the memory to be recovered by GC.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) pruneStakeNodes() {
	// Find the height to prune to.
	pruneToNode := b.bestChain.Tip()
	for i := int64(0); i < minMemoryStakeNodes-1 && pruneToNode != nil; i++ {
		pruneToNode = pruneToNode.parent
	}

	// Nothing to do if there are not enough nodes.
	if pruneToNode == nil || pruneToNode.parent == nil {
		return
	}

	// Determine the nodes that need to be pruned.  This will typically end up
	// being a small number of nodes since the pruning interval currently
	// coincides with the average block time.
	pruneNodes := make([]*blockNode, 0, b.pruner.prunedPerIntervalHint)
	for n := pruneToNode.parent; n != nil && n.stakeNode != nil; n = n.parent {
		pruneNodes = append(pruneNodes, n)
	}

	// Loop through each node to prune from the oldest to the newest and prune
	// the stake-related fields.
	for i := len(pruneNodes) - 1; i >= 0; i-- {
		node := pruneNodes[i]
		node.stakeNode = nil
		node.newTickets = nil
		node.ticketsVoted = nil
		node.ticketsRevoked = nil
	}
}

// isMajorityVersion determines if a previous number of blocks in the chain
// starting with startNode are at least the minimum passed version.
//
// This function MUST be called with the chain state lock held (for reads).
func (b *BlockChain) isMajorityVersion(minVer int32, startNode *blockNode, numRequired uint64) bool {
	numFound := uint64(0)
	iterNode := startNode
	for i := uint64(0); i < b.chainParams.BlockUpgradeNumToCheck &&
		numFound < numRequired && iterNode != nil; i++ {

		// This node has a version that is at least the minimum version.
		if iterNode.blockVersion >= minVer {
			numFound++
		}

		iterNode = iterNode.parent
	}

	return numFound >= numRequired
}

// connectBlock handles connecting the passed node/block to the end of the main
// (best) chain.
//
// This passed utxo view must have all referenced txos the block spends marked
// as spent and all of the new txos the block creates added to it.  In addition,
// the passed stxos slice must be populated with all of the information for the
// spent txos.  This approach is used because the connection validation that
// must happen prior to calling this function requires the same details, so
// it would be inefficient to repeat it.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) connectBlock(node *blockNode, block, parent *dcrutil.Block, view *UtxoViewpoint, stxos []spentTxOut, hdrCommitments *headerCommitmentData) error {
	// Make sure it's extending the end of the best chain.
	prevHash := block.MsgBlock().Header.PrevBlock
	tip := b.bestChain.Tip()
	if prevHash != tip.hash {
		panicf("block %v (height %v) connects to block %v instead of "+
			"extending the best chain (hash %v, height %v)", node.hash,
			node.height, prevHash, tip.hash, tip.height)
	}

	// Create agenda flags for checking transactions based on which ones are
	// active as of the block being connected.
	checkTxFlags, err := b.determineCheckTxFlags(node.parent)
	if err != nil {
		return err
	}
	isTreasuryEnabled := checkTxFlags.IsTreasuryEnabled()

	// Sanity check the correct number of stxos are provided.
	if len(stxos) != countSpentOutputs(block) {
		panicf("provided %v stxos for block %v (height %v) which spends %v "+
			"outputs", len(stxos), node.hash, node.height,
			countSpentOutputs(block))
	}

	// Write any modified block index entries to the database before
	// updating the best state.
	if err := b.flushBlockIndex(); err != nil {
		return err
	}

	// Get the stake node for this node, filling in any data that
	// may have yet to have been filled in.  In all cases this
	// should simply give a pointer to data already prepared, but
	// run this anyway to be safe.
	stakeNode, err := b.fetchStakeNode(node)
	if err != nil {
		return err
	}

	// Calculate the next stake difficulty.
	nextStakeDiff := b.calcNextRequiredStakeDifficulty(node)

	// Determine the individual commitment hashes that comprise the leaves of
	// the header commitment merkle tree depending on the active agendas.  These
	// are stored in the database below so that inclusion proofs can be
	// generated for each commitment.
	var hdrCommitmentLeaves []chainhash.Hash
	hdrCommitmentsActive, err := b.isHeaderCommitmentsAgendaActive(node.parent)
	if err != nil {
		return err
	}
	if hdrCommitmentsActive {
		hdrCommitmentLeaves = hdrCommitments.v1Leaves()
	}

	// Generate a new best state snapshot that will be used to update the
	// database and later memory if all database updates are successful.
	b.stateLock.RLock()
	curTotalTxns := b.stateSnapshot.TotalTxns
	curTotalSubsidy := b.stateSnapshot.TotalSubsidy
	b.stateLock.RUnlock()
	subsidy := calculateAddedSubsidy(block, parent)
	numTxns := uint64(len(block.Transactions()) + len(block.STransactions()))
	blockSize := uint64(block.MsgBlock().Header.Size)
	state := newBestState(node, blockSize, numTxns, curTotalTxns+numTxns,
		node.CalcPastMedianTime(), curTotalSubsidy+subsidy,
		uint32(node.stakeNode.PoolSize()), nextStakeDiff,
		node.stakeNode.ExpiringNextBlock(), node.stakeNode.Winners(),
		node.stakeNode.MissedTickets(), node.stakeNode.FinalState())

	// Atomically insert info into the database.
	err = b.db.Update(func(dbTx database.Tx) error {
		// Update best block state.
		err := dbPutBestState(dbTx, state, &node.workSum)
		if err != nil {
			return err
		}

		// Update the transaction spend journal by adding a record for
		// the block that contains all txos spent by it.
		err = dbPutSpendJournalEntry(dbTx, block.Hash(), stxos)
		if err != nil {
			return err
		}

		// Insert the block into the stake database.
		err = stake.WriteConnectedBestNode(dbTx, stakeNode, node.hash)
		if err != nil {
			return err
		}

		// Insert the treasury information into the database.
		if isTreasuryEnabled {
			err = b.dbPutTreasuryBalance(dbTx, block, node)
			if err != nil {
				return err
			}

			err = b.dbPutTSpend(dbTx, block)
			if err != nil {
				return err
			}
		}

		// Insert the GCS filter for the block into the database.
		err = dbPutGCSFilter(dbTx, block.Hash(), hdrCommitments.filter)
		if err != nil {
			return err
		}

		// Store the leaf hashes of the header commitment merkle tree in the
		// database.  Nothing is written when there aren't any.
		err = dbPutHeaderCommitments(dbTx, block.Hash(), hdrCommitmentLeaves)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Commit all entries in the view to the utxo cache.  All entries in the view
	// that are marked as modified and spent are removed from the view.
	// Additionally, all entries that are added to the cache are removed from the
	// view.
	err = b.utxoCache.Commit(view)
	if err != nil {
		return err
	}

	// Conditionally flush the utxo cache to the database.  Force a flush if the
	// chain believes it is current since blocks are connected infrequently at
	// that point.  Only log the flush when the chain is not current as it is
	// mostly useful to see the flush details when many blocks are being connected
	// (and subsequently flushed) in quick succession.
	isCurrent := b.isCurrent(node)
	err = b.utxoCache.MaybeFlush(&node.hash, uint32(node.height), isCurrent,
		!isCurrent)
	if err != nil {
		return err
	}

	// This node is now the end of the best chain.
	b.bestChain.SetTip(node)
	b.index.MaybePruneCachedTips(node)

	// Update the state for the best block.  Notice how this replaces the
	// entire struct instead of updating the existing one.  This effectively
	// allows the old version to act as a snapshot which callers can use
	// freely without needing to hold a lock for the duration.  See the
	// comments on the state variable for more details.
	b.stateLock.Lock()
	b.stateSnapshot = state
	b.stateLock.Unlock()

	// Conditionally log target difficulty changes at retarget intervals.  Only
	// log when the chain believes it is current since it is very noisy during
	// syncing otherwise.
	if isCurrent && node.parent != nil && node.bits != node.parent.bits &&
		node.height%b.chainParams.WorkDiffWindowSize == 0 {

		oldDiff := standalone.CompactToBig(node.parent.bits)
		newDiff := standalone.CompactToBig(node.bits)
		log.Debugf("Difficulty retarget at block height %d", node.height)
		log.Debugf("Old target %08x (%064x)", node.parent.bits, oldDiff)
		log.Debugf("New target %08x (%064x)", node.bits, newDiff)
	}

	// Notify the caller that the block was connected to the main chain.
	// The caller would typically want to react with actions such as
	// updating wallets.
	b.chainLock.Unlock()
	b.sendNotification(NTBlockConnected, &BlockConnectedNtfnsData{
		Block:        block,
		ParentBlock:  parent,
		CheckTxFlags: checkTxFlags,
	})
	b.chainLock.Lock()

	// Send stake notifications about the new block.
	if node.height >= b.chainParams.StakeEnabledHeight {
		// Notify of new tickets.
		nextStakeDiff := b.calcNextRequiredStakeDifficulty(node)
		b.sendNotification(NTNewTickets,
			&TicketNotificationsData{
				Hash:            node.hash,
				Height:          node.height,
				StakeDifficulty: nextStakeDiff,
				TicketsNew:      node.stakeNode.NewTickets(),
			})
	}

	// Optimization:  Immediately prune the parent's stake node when it is no
	// longer needed due to being too far behind the best known header.
	var pruneHeight int64
	bestHeader := b.index.BestHeader()
	if bestHeader.height > minMemoryStakeNodes {
		pruneHeight = bestHeader.height - minMemoryStakeNodes
	}
	if node.height < pruneHeight {
		parent := node.parent
		parent.stakeNode = nil
		parent.newTickets = nil
		parent.ticketsVoted = nil
		parent.ticketsRevoked = nil
	}

	b.addRecentBlock(block)

	return nil
}

// disconnectBlock handles disconnecting the passed node/block from the end of
// the main (best) chain.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) disconnectBlock(node *blockNode, block, parent *dcrutil.Block, view *UtxoViewpoint) error {
	// Make sure the node being disconnected is the end of the best chain.
	tip := b.bestChain.Tip()
	if node.hash != tip.hash {
		panicf("block %v (height %v) is not the end of the best chain "+
			"(hash %v, height %v)", node.hash, node.height, tip.hash,
			tip.height)
	}

	// Create agenda flags for checking transactions based on which ones were
	// active as of the block being disconnected.
	checkTxFlags, err := b.determineCheckTxFlags(node.parent)
	if err != nil {
		return err
	}

	// Write any modified block index entries to the database before
	// updating the best state.
	if err := b.flushBlockIndex(); err != nil {
		return err
	}

	// Prepare the information required to update the stake database
	// contents.
	childStakeNode, err := b.fetchStakeNode(node)
	if err != nil {
		return err
	}
	parentStakeNode, err := b.fetchStakeNode(node.parent)
	if err != nil {
		return err
	}

	// Generate a new best state snapshot that will be used to update the
	// database and later memory if all database updates are successful.
	b.stateLock.RLock()
	curTotalTxns := b.stateSnapshot.TotalTxns
	curTotalSubsidy := b.stateSnapshot.TotalSubsidy
	b.stateLock.RUnlock()
	parentBlockSize := uint64(parent.MsgBlock().Header.Size)
	numParentTxns := uint64(len(parent.Transactions()) + len(parent.STransactions()))
	numBlockTxns := uint64(len(block.Transactions()) + len(block.STransactions()))
	newTotalTxns := curTotalTxns - numBlockTxns
	subsidy := calculateAddedSubsidy(block, parent)
	newTotalSubsidy := curTotalSubsidy - subsidy
	prevNode := node.parent
	state := newBestState(prevNode, parentBlockSize, numParentTxns,
		newTotalTxns, prevNode.CalcPastMedianTime(), newTotalSubsidy,
		uint32(prevNode.stakeNode.PoolSize()), node.sbits,
		prevNode.stakeNode.ExpiringNextBlock(), prevNode.stakeNode.Winners(),
		prevNode.stakeNode.MissedTickets(), prevNode.stakeNode.FinalState())

	err = b.db.Update(func(dbTx database.Tx) error {
		// Update best block state.
		err := dbPutBestState(dbTx, state, &node.workSum)
		if err != nil {
			return err
		}

		err = stake.WriteDisconnectedBestNode(dbTx, parentStakeNode,
			node.parent.hash, childStakeNode.UndoData())
		if err != nil {
			return err
		}

		// NOTE: The GCS filter is intentionally not removed on disconnect to
		// ensure that lightweight clients still have access to them if they
		// happen to be on a side chain after coming back online after a reorg.
		//
		// Similarly, the commitment hashes needed to generate the associated
		// inclusion proof for the header commitment are not removed for the
		// same reason.

		return nil
	})
	if err != nil {
		return err
	}

	// Commit all entries in the view to the utxo cache.  All entries in the view
	// that are marked as modified and spent are removed from the view.
	// Additionally, all entries that are added to the cache are removed from the
	// view.
	err = b.utxoCache.Commit(view)
	if err != nil {
		return err
	}

	// Force a utxo cache flush when blocks are being disconnected.  A cache flush
	// is forced here since the spend journal entry for the disconnected block
	// will be removed below.
	err = b.utxoCache.MaybeFlush(&node.parent.hash, uint32(node.parent.height),
		true, false)
	if err != nil {
		return err
	}

	// Update the transaction spend journal by removing the record that contains
	// all txos spent by the block.  This is intentionally done AFTER the utxo
	// cache has been force flushed since the spend journal information will no
	// longer be available for the cache to use for recovery purposes after
	// being removed.
	err = b.db.Update(func(dbTx database.Tx) error {
		return dbRemoveSpendJournalEntry(dbTx, &node.hash)
	})
	if err != nil {
		return err
	}

	// This node's parent is now the end of the best chain.
	b.bestChain.SetTip(node.parent)

	// Update the state for the best block.  Notice how this replaces the
	// entire struct instead of updating the existing one.  This effectively
	// allows the old version to act as a snapshot which callers can use
	// freely without needing to hold a lock for the duration.  See the
	// comments on the state variable for more details.
	b.stateLock.Lock()
	b.stateSnapshot = state
	b.stateLock.Unlock()

	// Notify the caller that the block was disconnected from the main
	// chain.  The caller would typically want to react with actions such as
	// updating wallets.
	b.chainLock.Unlock()
	b.sendNotification(NTBlockDisconnected, &BlockDisconnectedNtfnsData{
		Block:        block,
		ParentBlock:  parent,
		CheckTxFlags: checkTxFlags,
	})
	b.chainLock.Lock()

	return nil
}

// countSpentRegularOutputs returns the number of utxos the regular transactions
// in the passed block spend.
func countSpentRegularOutputs(block *dcrutil.Block) int {
	// Skip the coinbase since it has no inputs.
	var numSpent int
	for _, tx := range block.MsgBlock().Transactions[1:] {
		numSpent += len(tx.TxIn)
	}
	return numSpent
}

// countSpentStakeOutputs returns the number of utxos the stake transactions in
// the passed block spend.
func countSpentStakeOutputs(block *dcrutil.Block) int {
	var numSpent int
	for _, stx := range block.MsgBlock().STransactions {
		// Exclude the vote stakebase since it has no input.
		if stake.IsSSGen(stx) {
			numSpent++
			continue
		}

		// Exclude treasurybase and treasury spends since neither have any
		// inputs.
		if stake.IsTreasuryBase(stx) || stake.IsTSpend(stx) {
			continue
		}

		numSpent += len(stx.TxIn)
	}
	return numSpent
}

// countSpentOutputs returns the number of utxos the passed block spends.
func countSpentOutputs(block *dcrutil.Block) int {
	return countSpentRegularOutputs(block) + countSpentStakeOutputs(block)
}

// loadOrCreateFilter attempts to load and return the version 2 GCS filter for
// the given block from the database and falls back to creating a new one in
// the case one has not previously been stored.
func (b *BlockChain) loadOrCreateFilter(block *dcrutil.Block, view *UtxoViewpoint) (*gcs.FilterV2, error) {
	// Attempt to load and return the version 2 block filter for the given block
	// from the database.
	var filter *gcs.FilterV2
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		filter, err = dbFetchGCSFilter(dbTx, block.Hash())
		return err
	})
	if err != nil {
		return nil, err
	}
	if filter != nil {
		return filter, nil
	}

	// At this point the version 2 block filter has not been stored in the
	// database for the block, so create and return one.
	filter, err = blockcf2.Regular(block.MsgBlock(), view)
	if err != nil {
		return nil, ruleError(ErrMissingTxOut, err.Error())
	}

	return filter, nil
}

// reorganizeChainInternal attempts to reorganize the block chain to the given
// target without attempting to undo failed reorgs.
//
// The actions needed to reorganize the chain to the given target fall into
// three main cases:
//
//  1. The target is a descendant of the current best chain tip (most common)
//  2. The target is an ancestor of the current best chain tip (least common)
//  3. The target is neither of the above which means it is on another branch
//     and that branch forks from the main chain at some ancestor of the current
//     best chain tip
//
// For the first case, the blocks between the current best chain tip and the
// given target need to be connected (think pushed onto the end of the chain).
//
// For the second case, the blocks between the current best chain tip and the
// given target need to be disconnected in reverse order (think popped off the
// end of chain).
//
// The third case is essentially a combination of the first two.  Namely, the
// blocks between the current best chain tip and the fork point between it and
// the given target need to be disconnected in reverse order and then the blocks
// between that fork point and the given target (aka the blocks that form the
// new branch) need to be connected in forwards order.
//
// This function may modify the validation state of nodes in the block index
// without flushing in the case the chain is not able to reorganize due to a
// block failing to connect.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) reorganizeChainInternal(target *blockNode) error {
	// Find the fork point between the current tip and target block.
	tip := b.bestChain.Tip()
	fork := b.bestChain.FindFork(target)

	// Disconnect all of the blocks back to the point of the fork.  This entails
	// loading the blocks and their associated spent txos from the database and
	// using that information to unspend all of the spent txos and remove the
	// utxos created by the blocks.  In addition, if a block votes against its
	// parent, the regular transactions are reconnected.
	view := NewUtxoViewpoint(b.utxoCache)
	view.SetBestHash(&tip.hash)
	var nextBlockToDetach *dcrutil.Block
	for tip != nil && tip != fork {
		select {
		case <-b.interrupt:
			return errInterruptRequested
		default:
		}

		// Grab the block to detach based on the node.  Use the fact that the
		// blocks are being detached in reverse order, so the parent of the
		// current block being detached is the next one being detached.
		n := tip
		block := nextBlockToDetach
		if block == nil {
			var err error
			block, err = b.fetchMainChainBlockByNode(n)
			if err != nil {
				return err
			}
		}
		if n.hash != *block.Hash() {
			panicf("detach block node hash %v (height %v) does not match "+
				"previous parent block hash %v", &n.hash, n.height,
				block.Hash())
		}

		// Grab the parent of the current block and also save a reference to it
		// as the next block to detach so it doesn't need to be loaded again on
		// the next iteration.
		parent, err := b.fetchMainChainBlockByNode(n.parent)
		if err != nil {
			return err
		}
		nextBlockToDetach = parent

		// Determine if treasury agenda is active.
		isTreasuryEnabled, err := b.isTreasuryAgendaActive(n.parent)
		if err != nil {
			return err
		}

		// Load all of the spent txos for the block from the spend journal.
		var stxos []spentTxOut
		err = b.db.View(func(dbTx database.Tx) error {
			stxos, err = dbFetchSpendJournalEntry(dbTx, block, isTreasuryEnabled)
			return err
		})
		if err != nil {
			return err
		}

		// Update the view to unspend all of the spent txos and remove the utxos
		// created by the block.  Also, if the block votes against its parent,
		// reconnect all of the regular transactions.
		err = view.disconnectBlock(block, parent, stxos, isTreasuryEnabled)
		if err != nil {
			return err
		}

		// Update the database and chain state.
		err = b.disconnectBlock(n, block, parent, view)
		if err != nil {
			return err
		}

		log.Tracef("Disconnected block %s (height %d) from main chain", n.hash,
			n.height)

		tip = n.parent
	}

	// Determine the blocks to attach after the fork point.  Each block is added
	// to the slice from back to front so they are attached in the appropriate
	// order when iterating the slice below.
	attachNodes := make([]*blockNode, target.height-fork.height)
	for n := target; n != nil && n != fork; n = n.parent {
		attachNodes[n.height-fork.height-1] = n
	}

	// Load the fork block if there are blocks to attach and its not already
	// loaded which will be the case if no nodes were detached.  The fork block
	// is used as the parent to the first node to be attached below.
	forkBlock := nextBlockToDetach
	if len(attachNodes) > 0 && forkBlock == nil {
		var err error
		forkBlock, err = b.fetchMainChainBlockByNode(tip)
		if err != nil {
			return err
		}
	}

	// Attempt to connect each block that needs to be attached to the main
	// chain.  This entails performing several checks to verify each block can
	// be connected without violating any consensus rules and updating the
	// relevant information related to the current chain state.
	var prevBlockAttached *dcrutil.Block
	for i, n := range attachNodes {
		select {
		case <-b.interrupt:
			return errInterruptRequested
		default:
		}

		// Grab the block to attach based on the node.  Use the fact that the
		// parent of the block is either the fork point for the first node being
		// attached or the previous one that was attached for subsequent blocks
		// to optimize.
		block, err := b.fetchBlockByNode(n)
		if err != nil {
			return err
		}
		parent := forkBlock
		if i > 0 {
			parent = prevBlockAttached
		}
		if n.parent.hash != *parent.Hash() {
			panicf("attach block node hash %v (height %v) parent hash %v does "+
				"not match previous parent block hash %v", &n.hash, n.height,
				&n.parent.hash, parent.Hash())
		}

		// Store the loaded block as parent of next iteration.
		prevBlockAttached = block

		// Determine if treasury agenda is active.
		isTreasuryEnabled, err := b.isTreasuryAgendaActive(n.parent)
		if err != nil {
			return err
		}

		// Skip validation if the block has already been validated.  However,
		// the utxo view still needs to be updated and the stxos and header
		// commitment data are still needed.
		numSpentOutputs := countSpentOutputs(block)
		stxos := make([]spentTxOut, 0, numSpentOutputs)
		var hdrCommitments headerCommitmentData
		if b.index.NodeStatus(n).HasValidated() {
			// Update the view to mark all utxos referenced by the block as
			// spent and add all transactions being created by this block to it.
			// In the case the block votes against the parent, also disconnect
			// all of the regular transactions in the parent block.  Finally,
			// provide an stxo slice so the spent txout details are generated.
			err := view.connectBlock(b.db, block, parent, &stxos,
				isTreasuryEnabled)
			if err != nil {
				return err
			}

			filter, err := b.loadOrCreateFilter(block, view)
			if err != nil {
				return err
			}
			hdrCommitments.filter = filter
			hdrCommitments.filterHash = filter.Hash()
		} else {
			// The block must pass all of the validation rules which depend on
			// having the full block data for all of its ancestors available.
			if err := b.checkBlockContext(block, n.parent, BFNone); err != nil {
				var rerr RuleError
				if errors.As(err, &rerr) {
					b.index.MarkBlockFailedValidation(n)
				}
				return err
			}

			// Mark the block as recently checked to avoid checking it again
			// when processing.
			b.recentContextChecks.Add(n.hash)

			// In the case the block is determined to be invalid due to a rule
			// violation, mark it as invalid and mark all of its descendants as
			// having an invalid ancestor.
			err = b.checkConnectBlock(n, block, parent, view, &stxos,
				&hdrCommitments)
			if err != nil {
				var rerr RuleError
				if errors.As(err, &rerr) {
					b.index.MarkBlockFailedValidation(n)
				}
				return err
			}
			b.index.SetStatusFlags(n, statusValidated)
		}

		// Update the database and chain state.
		err = b.connectBlock(n, block, parent, view, stxos, &hdrCommitments)
		if err != nil {
			return err
		}

		log.Tracef("Connected block %s (height %d) to main chain", n.hash,
			n.height)

		// Remove any best chain candidates that have less work than the new
		// tip.
		b.index.RemoveLessWorkCandidates(n)
	}

	return nil
}

// reorganizeChain attempts to reorganize the block chain to the given target
// with additional handling for failed reorgs.
//
// When the given target is already known to be invalid, or is determined to be
// invalid during the process, the chain will be reorganized to the best valid
// block as determined by having the most cumulative proof of work instead.
//
// This is most commonly called with a target that is a descendant of the
// current best chain.  However, it supports arbitrary targets.
//
// See reorganizeChainInternal for more details on the various actions needed to
// reorganize the chain.
//
// This function may modify the validation state of nodes in the block index
// without flushing.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) reorganizeChain(target *blockNode) error {
	// Nothing to do if there is no target specified or it is already the
	// current best chain tip.
	tip := b.bestChain.Tip()
	if target == nil || tip == target {
		return nil
	}
	origTip := tip

	var sentReorgingNtfn bool
	var reorgErrs []error
	for ; target != nil && tip != target; tip = b.bestChain.Tip() {
		select {
		case <-b.interrupt:
			return errInterruptRequested
		default:
		}

		// Determine if the chain is being reorganized to a competing branch.
		// This is the case when the current tip is not an ancestor of the
		// target tip.
		if !sentReorgingNtfn && !tip.IsAncestorOf(target) {
			// Send a notification announcing the start of the chain
			// reorganization.
			//
			// Notice that the chain lock is not released before sending the
			// notification.  This is intentional and must not be changed
			// without understanding why!
			b.sendNotification(NTChainReorgStarted, nil)
			sentReorgingNtfn = true

			defer func() {
				// Send a notification announcing the end of the chain
				// reorganization.
				//
				// Notice that the chain lock is not released before sending the
				// notification.  This is intentional and must not be changed
				// without understanding why!
				b.sendNotification(NTChainReorgDone, nil)
			}()
		}

		// Attempt to reorganize the chain to the new tip.  In the case it
		// fails, attempt to reorganize to the best valid block with the most
		// cumulative proof of work instead.
		err := b.reorganizeChainInternal(target)
		if err != nil {
			// Shutting down.
			if errors.Is(err, errInterruptRequested) {
				return err
			}

			// Typically, if a reorganize fails, there will only be a single
			// error due to the block that caused the failure.  However, it is
			// possible that several candidate branches might fail.  Thus, track
			// them all so they can potentially be converted to a multi error
			// later if needed.
			reorgErrs = append(reorgErrs, err)

			// Determine a new best candidate since the reorg failed.  This
			// should realistically always result in a different target than the
			// current one unless there is some type of unrecoverable error,
			// such as a disk failure.  In that case, bail out to avoid
			// attempting to do the same reorg over and over.
			newTarget := b.index.FindBestChainCandidate()
			if newTarget == target {
				break
			}
			target = newTarget
		}
	}

	// Potentially update whether or not the chain believes it is current based
	// on the new tip.  Notice that the tip is reset to whatever the best chain
	// actually is here versus using the one from above since it might not match
	// reality if there were errors while reorganizing.
	newTip := b.bestChain.Tip()
	wasLatched := b.isCurrentLatch
	b.maybeUpdateIsCurrent(newTip)

	// If the chain just latched to current, force the UTXO cache to flush to the
	// database.  This ensures that the full UTXO set is always flushed to the
	// database when the chain becomes current, which allows for fetching
	// up-to-date UTXO set stats.
	if !wasLatched && b.isCurrentLatch {
		err := b.utxoCache.MaybeFlush(&newTip.hash, uint32(newTip.height), true,
			true)
		if err != nil {
			return err
		}
	}

	// Log chain reorganizations and send a notification as needed.
	if sentReorgingNtfn && newTip != origTip {
		// Send a notification that a chain reorganization took place.
		//
		// Notice that the chain lock is not released before sending the
		// notification.  This is intentional and must not be changed without
		// understanding why!
		b.sendNotification(NTReorganization, &ReorganizationNtfnsData{
			OldHash:   origTip.hash,
			OldHeight: origTip.height,
			NewHash:   newTip.hash,
			NewHeight: newTip.height,
		})

		// Log the point where the chain forked and old and new best chain tips.
		if fork := b.bestChain.FindFork(origTip); fork != nil {
			log.Infof("REORGANIZE: Chain forks at %v (height %v)", fork.hash,
				fork.height)
		}
		log.Infof("REORGANIZE: Old best chain tip was %v (height %v)",
			&origTip.hash, origTip.height)
		log.Infof("REORGANIZE: New best chain tip is %v (height %v)",
			&newTip.hash, newTip.height)
	}

	// Determine if there were any reorg errors and either extract and return
	// the error directly when there was only a single error or return them all
	// as a multi error when there are more.
	var finalErr error
	switch {
	case len(reorgErrs) == 1:
		finalErr = reorgErrs[0]
	case len(reorgErrs) > 1:
		finalErr = MultiError(reorgErrs)
	}
	return finalErr
}

// forceHeadReorganization forces a reorganization of the block chain to the
// block hash requested, so long as it matches up with the current organization
// of the best chain.
//
// This function may modify the validation state of nodes in the block index
// without flushing.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) forceHeadReorganization(formerBest chainhash.Hash, newBest chainhash.Hash) error {
	// Don't try to reorganize to the same block.
	if formerBest == newBest {
		str := "tried to force reorg to the same block"
		return ruleError(ErrForceReorgSameBlock, str)
	}

	// Don't allow a reorganize when the former best is not the current best
	// chain tip.
	formerBestNode := b.bestChain.Tip()
	if formerBestNode.hash != formerBest {
		str := "tried to force reorg on wrong chain"
		return ruleError(ErrForceReorgWrongChain, str)
	}

	// Child to reorganize to is missing.
	newBestNode := b.index.LookupNode(&newBest)
	if newBestNode == nil || newBestNode.parent != formerBestNode.parent {
		str := "missing child of common parent for forced reorg"
		return ruleError(ErrForceReorgMissingChild, str)
	}

	// Don't allow a reorganize to a known invalid chain.
	newBestNodeStatus := b.index.NodeStatus(newBestNode)
	if newBestNodeStatus.KnownInvalid() {
		str := "block is known to be invalid"
		return ruleError(ErrKnownInvalidBlock, str)
	}

	// Don't try to reorganize to a block when its data is not available.
	if !newBestNodeStatus.HaveData() {
		return ruleError(ErrNoBlockData, "block data is not available")
	}

	// Reorganize the chain and flush any potential unsaved changes to the
	// block index to the database.  It is safe to ignore any flushing
	// errors here as the only time the index will be modified is if the
	// block failed to connect.
	err := b.reorganizeChain(newBestNode)
	b.flushBlockIndexWarnOnly()
	return err
}

// ForceHeadReorganization forces a reorganization of the block chain to the
// block hash requested, so long as it matches up with the current organization
// of the best chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) ForceHeadReorganization(formerBest chainhash.Hash, newBest chainhash.Hash) error {
	b.processLock.Lock()
	b.chainLock.Lock()
	err := b.forceHeadReorganization(formerBest, newBest)
	b.chainLock.Unlock()
	b.processLock.Unlock()
	return err
}

// flushBlockIndex populates any ticket data that has been pruned from modified
// block nodes, writes those nodes to the database and clears the set of
// modified nodes if it succeeds.
//
// This function MUST be called with the chain lock held (for writes).
func (b *BlockChain) flushBlockIndex() error {
	// Ensure that any ticket information that has been pruned is reloaded
	// before flushing modified nodes.
	//
	// Note that a separate slice is created for the modified nodes that
	// potentially need the ticket information reloaded as opposed to doing it
	// directly in the loop over the modified nodes because reloading the ticket
	// information is shared code that locks the index to mark the entry
	// modified.  Therefore, it has to be called without the index lock.
	b.index.RLock()
	maybePruned := make([]*blockNode, 0, len(b.index.modified))
	for node := range b.index.modified {
		if !b.index.canValidate(node) {
			continue
		}
		maybePruned = append(maybePruned, node)
	}
	b.index.RUnlock()
	for _, node := range maybePruned {
		if err := b.maybeFetchTicketInfo(node); err != nil {
			return err
		}
	}
	return b.index.Flush()
}

// flushBlockIndexWarnOnly attempts to flush any modified block index nodes to
// the database and will log a warning if it fails.
//
// NOTE: This MUST only be used in the specific circumstances where failure to
// flush only results in a worst case scenario of requiring one or more blocks
// to be validated again.  All other cases must directly call the function on
// the block index and check the error return accordingly.
//
// This function MUST be called with the chain lock held (for writes).
func (b *BlockChain) flushBlockIndexWarnOnly() {
	if err := b.flushBlockIndex(); err != nil {
		log.Warnf("Unable to flush block index changes to db: %v", err)
	}
}

// isOldTimestamp returns whether the given node has a timestamp too far in
// history for the purposes of determining if the chain should be considered
// current.
func (b *BlockChain) isOldTimestamp(node *blockNode) bool {
	minus24Hours := b.timeSource.AdjustedTime().Add(-24 * time.Hour).Unix()
	return node.timestamp < minus24Hours
}

// maybeUpdateIsCurrent potentially updates whether or not the chain believes it
// is current using the provided best chain tip.
//
// It makes use of a latching approach such that once the chain becomes current
// it will only switch back to false in the case no new blocks have been seen
// for an extended period of time.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) maybeUpdateIsCurrent(curBest *blockNode) {
	// Do some additional checks when the chain is not already latched to being
	// current.
	if !b.isCurrentLatch {
		// Not current if the latest best block has a cumulative work less than
		// the minimum known work specified by the network parameters.
		if b.minKnownWork != nil && curBest.workSum.Lt(b.minKnownWork) {
			return
		}

		// Not current if the best block is not synced to the header with the
		// most cumulative work that is not known to be invalid.
		bestHeader := b.index.BestHeader()
		syncedToBestHeader := curBest.height == bestHeader.height ||
			bestHeader.IsAncestorOf(curBest)
		if !syncedToBestHeader {
			return
		}
	}

	// Not current if the latest best block has too old of a timestamp.
	//
	// The chain appears to be current if none of the checks reported otherwise.
	wasLatched := b.isCurrentLatch
	b.isCurrentLatch = !b.isOldTimestamp(curBest)
	if !wasLatched && b.isCurrentLatch {
		log.Debugf("Chain latched to current at block %s (height %d)",
			curBest.hash, curBest.height)
	}
}

// MaybeUpdateIsCurrent potentially updates whether or not the chain believes it
// is current.
//
// It makes use of a latching approach such that once the chain becomes current
// it will only switch back to false in the case no new blocks have been seen
// for an extended period of time.
//
// This function is safe for concurrent access.
func (b *BlockChain) MaybeUpdateIsCurrent() {
	b.chainLock.Lock()
	b.maybeUpdateIsCurrent(b.bestChain.Tip())
	b.chainLock.Unlock()
}

// isCurrent returns whether or not the chain believes it is current based on
// the current latched state and an additional check which returns false in the
// case no new blocks have been seen for an extended period of time.
//
// This function MUST be called with the chain state lock held (for reads).
func (b *BlockChain) isCurrent(curBest *blockNode) bool {
	return b.isCurrentLatch && !b.isOldTimestamp(curBest)
}

// IsCurrent returns whether or not the chain believes it is current based on
// the current latched state and an additional check which returns false in the
// case no new blocks have been seen for an extended period of time.
//
// The initial factors that are used to latch the state to current are:
//   - Total amount of cumulative work is more than the minimum known work
//     specified by the parameters for the network
//   - The best chain is synced to the header with the most cumulative work that
//     is not known to be invalid
//   - Latest block has a timestamp newer than 24 hours ago
//
// This function is safe for concurrent access.
func (b *BlockChain) IsCurrent() bool {
	b.chainLock.RLock()
	isCurrent := b.isCurrent(b.bestChain.Tip())
	b.chainLock.RUnlock()
	return isCurrent
}

// BestSnapshot returns information about the current best chain block and
// related state as of the current point in time.  The returned instance must be
// treated as immutable since it is shared by all callers.
//
// This function is safe for concurrent access.
func (b *BlockChain) BestSnapshot() *BestState {
	b.stateLock.RLock()
	snapshot := b.stateSnapshot
	b.stateLock.RUnlock()
	return snapshot
}

// maxBlockSize returns the maximum permitted block size for the block
// AFTER the given node.
//
// This function MUST be called with the chain state lock held (for reads).
func (b *BlockChain) maxBlockSize(prevNode *blockNode) int64 {
	// Determine the correct deployment details for the block size consensus
	// vote or treat it as inactive when voting is not enabled for the current
	// network.
	const deploymentID = chaincfg.VoteIDMaxBlockSize
	deployment, ok := b.deploymentData[deploymentID]
	if !ok {
		return int64(b.chainParams.MaximumBlockSizes[0])
	}

	// Return the larger block size if the stake vote for the max block size
	// increase agenda is active.
	//
	// NOTE: The choice field of the return threshold state is not examined
	// here because there is only one possible choice that can be active
	// for the agenda, which is yes, so there is no need to check it.
	state := b.deploymentState(prevNode, &deployment)
	if state.State == ThresholdActive {
		return int64(b.chainParams.MaximumBlockSizes[1])
	}

	// The max block size is not changed in any other cases.
	maxSize := int64(b.chainParams.MaximumBlockSizes[0])
	return maxSize
}

// MaxBlockSize returns the maximum permitted block size for the block AFTER
// the provided block hash.
//
// This function is safe for concurrent access.
func (b *BlockChain) MaxBlockSize(hash *chainhash.Hash) (int64, error) {
	node := b.index.LookupNode(hash)
	if node == nil || !b.index.CanValidate(node) {
		return 0, unknownBlockError(hash)
	}

	b.chainLock.Lock()
	maxSize := b.maxBlockSize(node)
	b.chainLock.Unlock()
	return maxSize, nil
}

// HeaderByHash returns the block header identified by the given hash or an
// error if it doesn't exist.  Note that this will return headers from both the
// main chain and any side chains.
//
// This function is safe for concurrent access.
func (b *BlockChain) HeaderByHash(hash *chainhash.Hash) (wire.BlockHeader, error) {
	node := b.index.LookupNode(hash)
	if node == nil {
		return wire.BlockHeader{}, unknownBlockError(hash)
	}

	return node.Header(), nil
}

// HeaderByHeight returns the block header at the given height in the main
// chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) HeaderByHeight(height int64) (wire.BlockHeader, error) {
	node := b.bestChain.NodeByHeight(height)
	if node == nil {
		str := fmt.Sprintf("no block at height %d exists", height)
		return wire.BlockHeader{}, errNotInMainChain(str)
	}

	return node.Header(), nil
}

// BlockByHash searches the internal chain block stores and the database in an
// attempt to find the requested block and returns it.  This function returns
// blocks regardless of whether or not they are part of the main chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) BlockByHash(hash *chainhash.Hash) (*dcrutil.Block, error) {
	node := b.index.LookupNode(hash)
	if node == nil || !b.index.NodeStatus(node).HaveData() {
		return nil, unknownBlockError(hash)
	}

	// Return the block from either cache or the database.
	return b.fetchBlockByNode(node)
}

// BlockByHeight returns the block at the given height in the main chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) BlockByHeight(height int64) (*dcrutil.Block, error) {
	// Lookup the block height in the best chain.
	node := b.bestChain.NodeByHeight(height)
	if node == nil {
		str := fmt.Sprintf("no block at height %d exists", height)
		return nil, errNotInMainChain(str)
	}

	// Return the block from either cache or the database.  Note that this is
	// not using fetchMainChainBlockByNode since the main chain check has
	// already been done.
	return b.fetchBlockByNode(node)
}

// MainChainHasBlock returns whether or not the block with the given hash is in
// the main chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) MainChainHasBlock(hash *chainhash.Hash) bool {
	node := b.index.LookupNode(hash)
	return node != nil && b.bestChain.Contains(node)
}

// MedianTimeByHash returns the median time of a block by the given hash or an
// error if it doesn't exist.  Note that this will return times from both the
// main chain and any side chains.
//
// This function is safe for concurrent access.
func (b *BlockChain) MedianTimeByHash(hash *chainhash.Hash) (time.Time, error) {
	node := b.index.LookupNode(hash)
	if node == nil {
		return time.Time{}, unknownBlockError(hash)
	}
	return node.CalcPastMedianTime(), nil
}

// BlockHeightByHash returns the height of the block with the given hash in the
// main chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) BlockHeightByHash(hash *chainhash.Hash) (int64, error) {
	node := b.index.LookupNode(hash)
	if node == nil || !b.bestChain.Contains(node) {
		str := fmt.Sprintf("block %s is not in the main chain", hash)
		return 0, errNotInMainChain(str)
	}

	return node.height, nil
}

// BlockHashByHeight returns the hash of the block at the given height in the
// main chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) BlockHashByHeight(height int64) (*chainhash.Hash, error) {
	node := b.bestChain.NodeByHeight(height)
	if node == nil {
		str := fmt.Sprintf("no block at height %d exists", height)
		return nil, errNotInMainChain(str)
	}

	return &node.hash, nil
}

// HeightRange returns a range of block hashes for the given start and end
// heights.  It is inclusive of the start height and exclusive of the end
// height.  In other words, it is the half open range [startHeight, endHeight).
//
// The end height will be limited to the current main chain height.
//
// This function is safe for concurrent access.
func (b *BlockChain) HeightRange(startHeight, endHeight int64) ([]chainhash.Hash, error) {
	// Ensure requested heights are sane.
	if startHeight < 0 {
		return nil, fmt.Errorf("start height of fetch range must not "+
			"be less than zero - got %d", startHeight)
	}
	if endHeight < startHeight {
		return nil, fmt.Errorf("end height of fetch range must not "+
			"be less than the start height - got start %d, end %d",
			startHeight, endHeight)
	}

	// There is nothing to do when the start and end heights are the same,
	// so return now to avoid extra work.
	if startHeight == endHeight {
		return nil, nil
	}

	// When the requested start height is after the most recent best chain
	// height, there is nothing to do.
	latestHeight := b.bestChain.Tip().height
	if startHeight > latestHeight {
		return nil, nil
	}

	// Limit the ending height to the latest height of the chain.
	if endHeight > latestHeight+1 {
		endHeight = latestHeight + 1
	}

	// Fetch as many as are available within the specified range.
	hashes := make([]chainhash.Hash, endHeight-startHeight)
	iterNode := b.bestChain.NodeByHeight(endHeight - 1)
	for i := startHeight; i < endHeight; i++ {
		// Since the desired result is from the starting node to the
		// ending node in forward order, but they are iterated in
		// reverse, add them in reverse order.
		hashes[endHeight-i-1] = iterNode.hash
		iterNode = iterNode.parent
	}
	return hashes, nil
}

// locateInventory returns the node of the block after the first known block in
// the locator along with the number of subsequent nodes needed to either reach
// the provided stop hash or the provided max number of entries.
//
// In addition, there are two special cases:
//
//   - When no locators are provided, the stop hash is treated as a request for
//     that block, so it will either return the node associated with the stop
//     hash if it is known, or nil if it is unknown
//   - When locators are provided, but none of them are known, nodes starting
//     after the genesis block will be returned
//
// This is primarily a helper function for the locateBlocks and locateHeaders
// functions.
//
// This function MUST be called with the chain state lock held (for reads).
func (b *BlockChain) locateInventory(locator BlockLocator, hashStop *chainhash.Hash, maxEntries uint32) (*blockNode, uint32) {
	// There are no block locators so a specific block is being requested
	// as identified by the stop hash.
	stopNode := b.index.LookupNode(hashStop)
	if len(locator) == 0 {
		if stopNode == nil {
			// No blocks with the stop hash were found so there is
			// nothing to do.
			return nil, 0
		}
		return stopNode, 1
	}

	// Find the most recent locator block hash in the main chain.  In the
	// case none of the hashes in the locator are in the main chain, fall
	// back to the genesis block.
	startNode := b.bestChain.Genesis()
	for _, hash := range locator {
		node := b.index.LookupNode(hash)
		if node != nil && b.bestChain.Contains(node) {
			startNode = node
			break
		}
	}

	// Start at the block after the most recently known block.  When there
	// is no next block it means the most recently known block is the tip of
	// the best chain, so there is nothing more to do.
	startNode = b.bestChain.Next(startNode)
	if startNode == nil {
		return nil, 0
	}

	// Calculate how many entries are needed.
	total := uint32((b.bestChain.Tip().height - startNode.height) + 1)
	if stopNode != nil && b.bestChain.Contains(stopNode) &&
		stopNode.height >= startNode.height {

		total = uint32((stopNode.height - startNode.height) + 1)
	}
	if total > maxEntries {
		total = maxEntries
	}

	return startNode, total
}

// locateBlocks returns the hashes of the blocks after the first known block in
// the locator until the provided stop hash is reached, or up to the provided
// max number of block hashes.
//
// See the comment on the exported function for more details on special cases.
//
// This function MUST be called with the chain state lock held (for reads).
func (b *BlockChain) locateBlocks(locator BlockLocator, hashStop *chainhash.Hash, maxHashes uint32) []chainhash.Hash {
	// Find the node after the first known block in the locator and the
	// total number of nodes after it needed while respecting the stop hash
	// and max entries.
	node, total := b.locateInventory(locator, hashStop, maxHashes)
	if total == 0 {
		return nil
	}

	// Populate and return the found hashes.
	hashes := make([]chainhash.Hash, 0, total)
	for i := uint32(0); i < total; i++ {
		hashes = append(hashes, node.hash)
		node = b.bestChain.Next(node)
	}
	return hashes
}

// LocateBlocks returns the hashes of the blocks after the first known block in
// the locator until the provided stop hash is reached, or up to the provided
// max number of block hashes.
//
// In addition, there are two special cases:
//
//   - When no locators are provided, the stop hash is treated as a request for
//     that block, so it will either return the stop hash itself if it is known,
//     or nil if it is unknown
//   - When locators are provided, but none of them are known, hashes starting
//     after the genesis block will be returned
//
// This function is safe for concurrent access.
func (b *BlockChain) LocateBlocks(locator BlockLocator, hashStop *chainhash.Hash, maxHashes uint32) []chainhash.Hash {
	b.chainLock.RLock()
	hashes := b.locateBlocks(locator, hashStop, maxHashes)
	b.chainLock.RUnlock()
	return hashes
}

// locateHeaders returns the headers of the blocks after the first known block
// in the locator until the provided stop hash is reached, or up to the provided
// max number of block headers.
//
// See the comment on the exported function for more details on special cases.
//
// This function MUST be called with the chain state lock held (for reads).
func (b *BlockChain) locateHeaders(locator BlockLocator, hashStop *chainhash.Hash, maxHeaders uint32) []wire.BlockHeader {
	// Find the node after the first known block in the locator and the
	// total number of nodes after it needed while respecting the stop hash
	// and max entries.
	node, total := b.locateInventory(locator, hashStop, maxHeaders)
	if total == 0 {
		return nil
	}

	// Populate and return the found headers.
	headers := make([]wire.BlockHeader, 0, total)
	for i := uint32(0); i < total; i++ {
		headers = append(headers, node.Header())
		node = b.bestChain.Next(node)
	}
	return headers
}

// LocateHeaders returns the headers of the blocks after the first known block
// in the locator until the provided stop hash is reached, or up to a max of
// wire.MaxBlockHeadersPerMsg headers.
//
// In addition, there are two special cases:
//
//   - When no locators are provided, the stop hash is treated as a request for
//     that header, so it will either return the header for the stop hash itself
//     if it is known, or nil if it is unknown
//   - When locators are provided, but none of them are known, headers starting
//     after the genesis block will be returned
//
// This function is safe for concurrent access.
func (b *BlockChain) LocateHeaders(locator BlockLocator, hashStop *chainhash.Hash) []wire.BlockHeader {
	b.chainLock.RLock()
	headers := b.locateHeaders(locator, hashStop, wire.MaxBlockHeadersPerMsg)
	b.chainLock.RUnlock()
	return headers
}

// BlockLocatorFromHash returns a block locator for the passed block hash.
// See BlockLocator for details on the algorithm used to create a block locator.
//
// In addition to the general algorithm referenced above, this function will
// return the block locator for the latest known tip of the main (best) chain if
// the passed hash is not currently known.
//
// This function is safe for concurrent access.
func (b *BlockChain) BlockLocatorFromHash(hash *chainhash.Hash) BlockLocator {
	b.chainLock.RLock()
	node := b.index.LookupNode(hash)
	locator := b.bestChain.BlockLocator(node)
	b.chainLock.RUnlock()
	return locator
}

// isMainNet returns whether or not the provided chain params are for the main
// network.
func isMainNet(params *chaincfg.Params) bool {
	return params.Net == wire.MainNet
}

// validateDeploymentChoices ensures the provided choices conform to the
// semantics required by the consensus rule change vote tallying and state
// determination logic.
func validateDeploymentChoices(voteParams *chaincfg.Vote) error {
	// Ensure the mask is not zero.
	if voteParams.Mask == 0 {
		str := fmt.Sprintf("deployment ID %s mask is zero", voteParams.Id)
		return contextError(ErrDeploymentBadMask, str)
	}

	// Ensure the mask does not use the bit reserved to specify whether or not
	// the voters approve the regular transaction tree of the parent block.
	if dcrutil.IsFlagSet16(voteParams.Mask, dcrutil.BlockValid) {
		str := fmt.Sprintf("deployment ID %s mask %#04x uses reserved bit 0",
			voteParams.Id, voteParams.Mask)
		return contextError(ErrDeploymentBadMask, str)
	}

	// Count the number of consecutive 1 bits set in the mask.
	var consecOnes uint8
	for v := voteParams.Mask; v != 0; consecOnes++ {
		v &= (v << 1)
	}

	// Ensure the mask only consists of consecutive bits.
	maskPopulationCount := uint8(bits.OnesCount16(voteParams.Mask))
	if consecOnes != maskPopulationCount {
		str := fmt.Sprintf("deployment ID %s mask %#04x does not have "+
			"consecutive bits", voteParams.Id, voteParams.Mask)
		return contextError(ErrDeploymentBadMask, str)
	}

	// Ensure there are not more choices than the mask bits can represent.
	numChoices := len(voteParams.Choices)
	if numChoices > 1<<maskPopulationCount {
		str := fmt.Sprintf("deployment ID %s has %d choices for mask %#04x "+
			"which can only represent %d choices", voteParams.Id, numChoices,
			voteParams.Mask, 1<<maskPopulationCount)
		return contextError(ErrDeploymentTooManyChoices, str)
	}

	var numAbstain, numNo int
	dups := make(map[string]int)
	for choiceIdx, choice := range voteParams.Choices {
		// Ensure the id is not empty.
		if choice.Id == "" {
			str := fmt.Sprintf("deployment ID %s choice index %d does not "+
				"have an ID", voteParams.Id, choiceIdx)
			return contextError(ErrDeploymentMissingChoiceID, str)
		}

		// Ensure that the choice bits are not zero for all choices except the
		// abstain choice.
		if choice.Bits == 0 && !choice.IsAbstain {
			str := fmt.Sprintf("deployment ID %s choice ID %s (index %d) vote "+
				"bits are zero for choice that is not marked abstain",
				voteParams.Id, choice.Id, choiceIdx)
			return contextError(ErrDeploymentBadChoiceBits, str)
		}

		// Ensure the bits for the choice are covered by the mask.
		if voteParams.Mask&choice.Bits != choice.Bits {
			str := fmt.Sprintf("deployment ID %s choice ID %s (index %d) vote "+
				"bits %#04x are not covered by the mask %04x", voteParams.Id,
				choice.Id, choiceIdx, choice.Bits, voteParams.Mask)
			return contextError(ErrDeploymentBadChoiceBits, str)
		}

		// Ensure only one of the choice type identification flags are set.
		if choice.IsAbstain && choice.IsNo {
			str := fmt.Sprintf("deployment ID %s choice ID %s (index %d) has "+
				"both the abstain and no choice flags set", voteParams.Id,
				choice.Id, choiceIdx)
			return contextError(ErrDeploymentNonExclusiveFlags, str)
		}

		// Count flags.
		if choice.IsAbstain {
			numAbstain++
		}
		if choice.IsNo {
			numNo++
		}

		// Ensure there are not any duplicates.
		id := strings.ToLower(choice.Id)
		if origChoiceIdx, found := dups[id]; found {
			str := fmt.Sprintf("deployment ID %s choice ID %s at index %d "+
				"already exists for the choice at index %d", voteParams.Id,
				choice.Id, choiceIdx, origChoiceIdx)
			return contextError(ErrDeploymentDuplicateChoice, str)
		}
		dups[id] = choiceIdx
	}

	// Ensure there is one and only one of each choice type identification flag
	// set.
	switch {
	case numAbstain == 0:
		str := fmt.Sprintf("deployment ID %s does not have an abstain choice",
			voteParams.Id)
		return contextError(ErrDeploymentMissingAbstain, str)
	case numAbstain > 1:
		str := fmt.Sprintf("deployment ID %s has more than one abstain choice",
			voteParams.Id)
		return contextError(ErrDeploymentTooManyAbstain, str)
	case numNo == 0:
		str := fmt.Sprintf("deployment ID %s does not have a no choice",
			voteParams.Id)
		return contextError(ErrDeploymentMissingNo, str)
	case numNo > 1:
		str := fmt.Sprintf("deployment ID %s has more than one no choice",
			voteParams.Id)
		return contextError(ErrDeploymentTooManyNo, str)
	}

	return nil
}

// determineForcedThresholdState returns the appropriate threshold state and
// choice for the given deployment when it has a forced choice specified.  It
// returns nil when a forced choice is not specified or an error when the choice
// either does not exist or is unsuitable for use as a forced choice.
func determineForcedThresholdState(deployment *chaincfg.ConsensusDeployment) (*ThresholdStateTuple, error) {
	// Nothing to extract when there is no forced choice.
	forcedChoiceID := deployment.ForcedChoiceID
	if forcedChoiceID == "" {
		return nil, nil
	}
	deploymentID := deployment.Vote.Id

	// Attempt to find the choice with the ID that matches the forced choice
	// specified in the deployment chain params and ensure it exists.
	var forcedChoice *chaincfg.Choice
	for choiceIdx := range deployment.Vote.Choices {
		choice := &deployment.Vote.Choices[choiceIdx]
		if choice.Id == forcedChoiceID {
			forcedChoice = choice
			break
		}
	}
	if forcedChoice == nil {
		str := fmt.Sprintf("deployment ID %s forced choice %q does not exist "+
			"in the chain parameters", deploymentID, forcedChoiceID)
		return nil, contextError(ErrUnknownDeploymentChoice, str)
	}

	// The forced choice must not be the abstain choice because it must resolve
	// to either an active or failed state.
	if forcedChoice.IsAbstain {
		str := fmt.Sprintf("deployment ID %s forced choice %q is of type "+
			"abstain which is not a valid forced choice", deploymentID,
			forcedChoiceID)
		return nil, contextError(ErrDeploymentChoiceAbstain, str)
	}

	// Return the appropriate forced threshold state with the associated found
	// choice.
	state := ThresholdActive
	if forcedChoice.IsNo {
		state = ThresholdFailed
	}
	tuple := newThresholdState(state, forcedChoice)
	return &tuple, nil
}

// extractDeployments returns a map of all deployment IDs within the provided
// params to a deployment info structure populated with the associated details.
//
// It also returns an appropriate error when any additional sanity checks fail.
// For example, duplicate deployment IDs are rejected and forced choices are
// disallowed on the main network.
func extractDeployments(params *chaincfg.Params) (map[string]deploymentInfo, error) {
	// Generate a deployment ID map from the provided params.
	deploymentData := make(map[string]deploymentInfo)
	for version, deployments := range params.Deployments {
		var usedMaskBits uint16
		for i := range deployments {
			deployment := &deployments[i]
			id := deployment.Vote.Id
			if _, ok := deploymentData[id]; ok {
				str := fmt.Sprintf("deployment ID %s exists in more than one "+
					"deployment", id)
				return nil, contextError(ErrDuplicateDeployment, str)
			}

			// Ensure the masks in all deployments for the same version do not
			// have any shared bits.
			voteParams := &deployment.Vote
			if voteParams.Mask&usedMaskBits != 0 {
				str := fmt.Sprintf("deployment ID %s mask %#04x uses bits "+
					"that are already used by other votes in the deployment "+
					"(used bits %#04x)", voteParams.Id, voteParams.Mask,
					usedMaskBits)
				return nil, contextError(ErrDeploymentBadMask, str)
			}
			usedMaskBits |= voteParams.Mask

			// Ensure the deployment choices conform to the semantics expected
			// by the vote tallying threshold state logic.
			if err := validateDeploymentChoices(voteParams); err != nil {
				return nil, err
			}

			// Determine the forced threshold state when the deployment has a
			// forced choice specified.
			forcedState, err := determineForcedThresholdState(deployment)
			if err != nil {
				return nil, err
			}

			// Prevent forced choices on the main network.
			if isMainNet(params) && forcedState != nil {
				str := fmt.Sprintf("deployment ID %s has a forced choice for "+
					"the main network", id)
				return nil, contextError(ErrForcedMainNetChoice, str)
			}

			deploymentData[id] = deploymentInfo{
				version:     version,
				deployment:  deployment,
				forcedState: forcedState,
				cache: &thresholdStateCache{
					entries: make(map[chainhash.Hash]ThresholdStateTuple),
				},
			}
		}
	}

	return deploymentData, nil
}

// ChainQueryerAdapter provides an adapter from a BlockChain instance to the
// indexers.ChainQueryer interface.
type ChainQueryerAdapter struct {
	*BlockChain
}

// BestHeight returns the height of the current best block.  It is equivalent to
// the Height field of the BestSnapshot method, however, it is needed to satisfy
// the indexers.ChainQueryer interface.
//
// It is defined via a separate internal struct to avoid polluting the public
// API of the BlockChain type itself.
func (q *ChainQueryerAdapter) BestHeight() int64 {
	return q.BestSnapshot().Height
}

// IsTreasuryEnabled returns true if the treasury agenda is enabled as of the
// provided block.
func (q *ChainQueryerAdapter) IsTreasuryEnabled(hash *chainhash.Hash) (bool, error) {
	return q.IsTreasuryAgendaActive(hash)
}

// isTestNet3 returns whether or not the chain instance is for version 3 of the
// test network.
func (b *BlockChain) isTestNet3() bool {
	return b.chainParams.Net == wire.TestNet3
}

// ChainParams returns the network parameters of the chain.
//
// This is part of the indexers.ChainQueryer interface.
func (q *ChainQueryerAdapter) ChainParams() *chaincfg.Params {
	return q.chainParams
}

// Best returns the height and hash of the current best chain tip.
//
// This is part of the indexers.ChainQueryer interface.
func (q *ChainQueryerAdapter) Best() (int64, *chainhash.Hash) {
	snapshot := q.BestSnapshot()
	return snapshot.Height, &snapshot.Hash
}

// BlockHeaderByHash returns the block header identified by the given hash.
func (q *ChainQueryerAdapter) BlockHeaderByHash(hash *chainhash.Hash) (wire.BlockHeader, error) {
	return q.HeaderByHash(hash)
}

// Config is a descriptor which specifies the blockchain instance configuration.
type Config struct {
	// DB defines the database which houses the blocks and will be used to
	// store all metadata created by this package outside of the UTXO set, which
	// is stored in a separate database.
	//
	// This field is required.
	DB database.DB

	// UtxoBackend defines the backend which houses the UTXO set.
	//
	// This field is required.
	UtxoBackend UtxoBackend

	// ChainParams identifies which chain parameters the chain is associated
	// with.
	//
	// This field is required.
	ChainParams *chaincfg.Params

	// AllowOldForks enables processing of blocks that are forks deep in the
	// chain history.  This should realistically never need to be enabled in
	// practice, however, it is provided for testing purposes as well as a
	// recovery mechanism in the extremely unlikely case that a node were to
	// somehow get stuck on a bad fork and be unable to reorg to the good one
	// due to the old fork rejection semantics.
	AllowOldForks bool

	// AssumeValid is the hash of a block that has been externally verified to
	// be valid.  It allows several validation checks to be skipped for blocks
	// that are both an ancestor of the assumed valid block and an ancestor of
	// the best header.
	//
	// This field may not be set for networks that do not require it.
	AssumeValid chainhash.Hash

	// TimeSource defines the median time source to use for things such as
	// block processing and determining whether or not the chain is current.
	//
	// The caller is expected to keep a reference to the time source as well
	// and add time samples from other peers on the network so the local
	// time is adjusted to be in agreement with other peers.
	TimeSource MedianTimeSource

	// Notifications defines a callback to which notifications will be sent
	// when various events take place.  See the documentation for
	// Notification and NotificationType for details on the types and
	// contents of notifications.
	//
	// This field can be nil if the caller is not interested in receiving
	// notifications.
	Notifications NotificationCallback

	// SigCache defines a signature cache to use when validating signatures.
	// This is typically most useful when individual transactions are
	// already being validated prior to their inclusion in a block such as
	// what is usually done via a transaction memory pool.
	//
	// This field can be nil if the caller is not interested in using a
	// signature cache.
	SigCache *txscript.SigCache

	// SubsidyCache defines a subsidy cache to use when calculating and
	// validating block and vote subsidies.
	//
	// This field can be nil if the caller is not interested in using a
	// subsidy cache.
	SubsidyCache *standalone.SubsidyCache

	// IndexSubscriber defines a subscriber for relaying updates
	// concerning connected and disconnected blocks to subscribed index clients.
	IndexSubscriber *indexers.IndexSubscriber

	// UtxoCache defines a utxo cache that sits on top of the utxo set database.
	// All utxo reads and writes go through the cache, and never read or write to
	// the database directly.
	//
	// This field is required.
	UtxoCache UtxoCacher
}

// New returns a BlockChain instance using the provided configuration details.
func New(ctx context.Context, config *Config) (*BlockChain, error) {
	// Enforce required config fields.
	if config.DB == nil {
		return nil, AssertError("blockchain.New database is nil")
	}
	if config.UtxoBackend == nil {
		return nil, AssertError("blockchain.New UTXO backend is nil")
	}
	if config.ChainParams == nil {
		return nil, AssertError("blockchain.New chain parameters nil")
	}

	// Generate a deployment ID map from the provided params while validating
	// they conform to the required semantics.
	params := config.ChainParams
	deploymentData, err := extractDeployments(params)
	if err != nil {
		return nil, err
	}

	// Convert the minimum known work to a uint256 when it exists.  Ideally, the
	// chain params should be updated to use the new type, but that will be a
	// major version bump, so a one-time conversion is a good tradeoff in the
	// mean time.
	var minKnownWork *uint256.Uint256
	if params.MinKnownChainWork != nil {
		minKnownWork = new(uint256.Uint256).SetBig(params.MinKnownChainWork)
	}

	// Impose a maximum difficulty target on the test network to prevent runaway
	// difficulty on testnet by ASICs and GPUs since it's not reasonable to
	// require high-powered hardware to keep the test network running smoothly.
	var minTestNetTarget *big.Int
	var minTestNetDiffBits uint32
	if params.Net == wire.TestNet3 {
		// This equates to a maximum difficulty of 2^6 = 64.
		const maxTestDiffShift = 6
		minTestNetTarget = new(big.Int).Rsh(params.PowLimit, maxTestDiffShift)
		minTestNetDiffBits = standalone.BigToCompact(minTestNetTarget)
	}

	// Either use the subsidy cache provided by the caller or create a new
	// one when one was not provided.
	subsidyCache := config.SubsidyCache
	if subsidyCache == nil {
		subsidyCache = standalone.NewSubsidyCache(params)
	}

	// Calculate the expected number of blocks in 2 weeks and cache it in order
	// to avoid repeated calculation.
	const timeInTwoWeeks = time.Hour * 24 * 14
	expectedBlksInTwoWeeks := int64(timeInTwoWeeks / params.TargetTimePerBlock)

	// Fork rejection semantics are disabled when explicitly requested or the
	// hard-coded assume valid hash is not set for the current network.
	//
	// Note that this is very intentionally using the hard-coded value specified
	// in the chain params as opposed to the one that a user might have
	// overridden in the passed config since fork rejection affects consensus.
	allowOldForks := config.AllowOldForks || params.AssumeValid == *zeroHash

	b := BlockChain{
		assumeValid:                   config.AssumeValid,
		allowOldForks:                 allowOldForks,
		expectedBlocksInTwoWeeks:      expectedBlksInTwoWeeks,
		deploymentData:                deploymentData,
		minKnownWork:                  minKnownWork,
		minTestNetTarget:              minTestNetTarget,
		minTestNetDiffBits:            minTestNetDiffBits,
		db:                            config.DB,
		chainParams:                   params,
		timeSource:                    config.TimeSource,
		notifications:                 config.Notifications,
		sigCache:                      config.SigCache,
		interrupt:                     ctx.Done(),
		indexSubscriber:               config.IndexSubscriber,
		subsidyCache:                  subsidyCache,
		index:                         newBlockIndex(config.DB),
		bestChain:                     newChainView(nil),
		recentBlocks:                  lru.NewKVCache(recentBlockCacheSize),
		recentContextChecks:           lru.NewCache(contextCheckCacheSize),
		isVoterMajorityVersionCache:   make(map[[stakeMajorityCacheKeySize]byte]bool),
		isStakeMajorityVersionCache:   make(map[[stakeMajorityCacheKeySize]byte]bool),
		calcPriorStakeVersionCache:    make(map[[chainhash.HashSize]byte]uint32),
		calcVoterVersionIntervalCache: make(map[[chainhash.HashSize]byte]uint32),
		calcStakeVersionCache:         make(map[[chainhash.HashSize]byte]uint32),
		utxoCache:                     config.UtxoCache,
	}
	b.pruner = newChainPruner(&b)

	// Initialize the chain state from the passed database.  When the db
	// does not yet contain any chain state, both it and the chain state
	// will be initialized to contain only the genesis block.
	if err := b.initChainState(ctx, config.UtxoBackend); err != nil {
		return nil, err
	}

	// Initialize the UTXO state.  This entails running any database migrations
	// as necessary as well as initializing the UTXO cache.
	if err := b.utxoCache.Initialize(ctx, &b); err != nil {
		return nil, err
	}

	log.Infof("Blockchain database version info: chain: %d, compression: "+
		"%d, block index: %d, spend journal: %d", b.dbInfo.version,
		b.dbInfo.compVer, b.dbInfo.bidxVer, b.dbInfo.stxoVer)

	// Fetch and log the UTXO backend versioning info.
	utxoDbInfo, err := config.UtxoBackend.FetchInfo()
	if err != nil {
		return nil, err
	}
	log.Infof("UTXO database version info: version: %d, compression: %d, utxo "+
		"set: %d", utxoDbInfo.version, utxoDbInfo.compVer, utxoDbInfo.utxoVer)

	// Manually invalidate any chains on version 3 of the test network that were
	// created prior to enforcement of the maximum difficulty rules.
	if b.isTestNet3() {
		// Discover any existing nodes at the max diff activation height that do
		// not have the expected hash and have not already been invalidated.
		invalidateNodes := make([]*blockNode, 0, 1)
		b.index.Lock()
		b.index.forEachChainTip(func(tip *blockNode) error {
			node := tip.Ancestor(testNet3MaxDiffActivationHeight)
			if node != nil && node.hash != *block962928Hash &&
				!node.status.KnownInvalid() {

				invalidateNodes = append(invalidateNodes, node)
			}
			return nil
		})
		b.index.Unlock()

		// Invalidate the blocks without notification callbacks as the calling
		// code will not be fully initialized yet at this point and this is
		// being done as a part of chain initialization.
		curNtfnCallback := b.notifications
		b.notifications = nil
		for _, node := range invalidateNodes {
			if err := b.InvalidateBlock(&node.hash); err != nil {
				b.notifications = curNtfnCallback
				return nil, err
			}
		}
		b.notifications = curNtfnCallback
	}

	bestHdr := b.index.BestHeader()
	log.Infof("Best known header: height %d, hash %v", bestHdr.height,
		bestHdr.hash)

	tip := b.bestChain.Tip()
	log.Infof("Chain state: height %d, hash %v, total transactions %d, work "+
		"%v, progress %0.2f%%", tip.height, tip.hash,
		b.stateSnapshot.TotalTxns, tip.workSum, b.VerifyProgress())

	return &b, nil
}
