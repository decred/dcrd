// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2018-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v4"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/wire"
)

// blockStatus is a bit field representing the validation state of the block.
type blockStatus byte

// The following constants specify possible status bit flags for a block.
//
// NOTE: This section specifically does not use iota since the block status is
// serialized and must be stable for long-term storage.
const (
	// statusNone indicates that the block has no validation state flags set.
	statusNone blockStatus = 0

	// statusDataStored indicates that the block's payload is stored on disk.
	statusDataStored blockStatus = 1 << 0

	// statusValidated indicates that the block has been fully validated.  It
	// also means that all of its ancestors have also been validated.
	statusValidated blockStatus = 1 << 1

	// statusValidateFailed indicates that the block has failed validation.
	statusValidateFailed blockStatus = 1 << 2

	// statusInvalidAncestor indicates that one of the ancestors of the block
	// has failed validation, thus the block is also invalid.
	statusInvalidAncestor blockStatus = 1 << 3
)

const (
	// cachedTipsPruneInterval is the amount of time to wait in between pruning
	// the cache that tracks the most recent chain tips.
	cachedTipsPruneInterval = time.Minute * 5

	// cachedTipsPruneDepth is the number of blocks before the provided best
	// block hint to prune cached chain tips.  This value is set based on the
	// target block time for the main network such that there is approximately
	// one hour of chain tips cached.
	cachedTipsPruneDepth = 12
)

// HaveData returns whether the full block data is stored in the database.  This
// will return false for a block node where only the header is downloaded or
// stored.
func (status blockStatus) HaveData() bool {
	return status&statusDataStored != 0
}

// HasValidated returns whether the block is known to have been successfully
// validated.  A return value of false in no way implies the block is invalid.
// Thus, this will return false for a valid block that has not been fully
// validated yet.
//
// NOTE: A block that is known to have been validated might also be marked as
// known invalid as well if the block is manually invalidated.
func (status blockStatus) HasValidated() bool {
	return status&statusValidated != 0
}

// KnownInvalid returns whether either the block itself is known to be invalid
// or to have an invalid ancestor.  A return value of false in no way implies
// the block is valid or only has valid ancestors.  Thus, this will return false
// for invalid blocks that have not been proven invalid yet as well as return
// false for blocks with invalid ancestors that have not been proven invalid
// yet.
//
// NOTE: A block that is known invalid might also be marked as known to have
// been successfully validated as well if the block is manually invalidated.
func (status blockStatus) KnownInvalid() bool {
	return status&(statusValidateFailed|statusInvalidAncestor) != 0
}

// KnownInvalidAncestor returns whether the block is known to have an invalid
// ancestor.  A return value of false in no way implies the block only has valid
// ancestors.  Thus, this will return false for blocks with invalid ancestors
// that have not been proven invalid yet.
func (status blockStatus) KnownInvalidAncestor() bool {
	return status&(statusInvalidAncestor) != 0
}

// KnownValidateFailed returns whether the block is known to have failed
// validation.  A return value of false in no way implies the block is valid.
// Thus, this will return false for blocks that have not been proven to fail
// validation yet.
func (status blockStatus) KnownValidateFailed() bool {
	return status&(statusValidateFailed) != 0
}

// blockNode represents a block within the block chain and is primarily used to
// aid in selecting the best chain to be the main chain.  The main chain is
// stored into the block database.
type blockNode struct {
	// NOTE: Additions, deletions, or modifications to the order of the
	// definitions in this struct should not be changed without considering
	// how it affects alignment on 64-bit platforms.  The current order is
	// specifically crafted to result in minimal padding.  There will be
	// hundreds of thousands of these in memory, so a few extra bytes of
	// padding adds up.

	// parent is the parent block for this node.
	parent *blockNode

	// skipToAncestor is used to provide a skip list to significantly speed up
	// traversal to ancestors deep in history.
	skipToAncestor *blockNode

	// hash is the hash of the block this node represents.
	hash chainhash.Hash

	// workSum is the total amount of work in the chain up to and including
	// this node.
	workSum *big.Int

	// Some fields from block headers to aid in best chain selection and
	// reconstructing headers from memory.  These must be treated as
	// immutable and are intentionally ordered to avoid padding on 64-bit
	// platforms.
	height       int64
	voteBits     uint16
	finalState   [6]byte
	blockVersion int32
	voters       uint16
	freshStake   uint8
	revocations  uint8
	poolSize     uint32
	bits         uint32
	sbits        int64
	timestamp    int64
	merkleRoot   chainhash.Hash
	stakeRoot    chainhash.Hash
	blockSize    uint32
	nonce        uint32
	extraData    [32]byte
	stakeVersion uint32

	// status is a bitfield representing the validation state of the block.
	// This field, unlike most other fields, may be changed after the block
	// node is created, so it must only be accessed or updated using the
	// concurrent-safe NodeStatus, SetStatusFlags, and UnsetStatusFlags
	// methods on blockIndex once the node has been added to the index.
	status blockStatus

	// isFullyLinked indicates whether or not this block builds on a branch
	// that has the block data for all of its ancestors and is therefore
	// eligible for validation.
	//
	// It is protected by the block index mutex and is not stored in the
	// database.
	isFullyLinked bool

	// stakeNode contains all the consensus information required for the
	// staking system.  The node also caches information required to add or
	// remove stake nodes, so that the stake node itself may be prunable
	// to save memory while maintaining high throughput efficiency for the
	// evaluation of sidechains.
	stakeNode      *stake.Node
	newTickets     []chainhash.Hash
	ticketsVoted   []chainhash.Hash
	ticketsRevoked []chainhash.Hash

	// Keep track of all vote version and bits in this block.
	votes []stake.VoteVersionTuple

	// receivedOrderID tracks the order block data was received for the node and
	// is only stored in memory.  It is set when the block data is received, and
	// the block data for all parents is also already known, as opposed to when
	// the header was received in order to ensure that no additional priority in
	// terms of chain selection between competing branches can be gained by
	// submitting the header first.
	//
	// It is protected by the block index mutex.
	receivedOrderID uint32
}

// clearLowestOneBit clears the lowest set bit in the passed value.
func clearLowestOneBit(n int64) int64 {
	return n & (n - 1)
}

// calcSkipListHeight calculates the height of an ancestor block to use when
// constructing the ancestor traversal skip list.
func calcSkipListHeight(height int64) int64 {
	if height < 0 {
		return 0
	}

	// Traditional skip lists create multiple levels to achieve expected average
	// search, insert, and delete costs of O(log n).  Since the blockchain is
	// append only, there is no need to handle random insertions or deletions,
	// so this takes advantage of that to effectively create a deterministic
	// skip list with a single level that is reasonably close to O(log n) in
	// order to reduce the number of pointers and implementation complexity.
	//
	// This calculation is definitely not the most optimal possible in terms of
	// the number of steps in the worst case, however, it is predominantly
	// logarithmic, easy to reason about, deterministic, blazing fast to
	// calculate and can easily be shown to have a worst case performance of
	// 420 steps for heights up to 4,294,967,296 (2^32) and 1580 steps for
	// heights up to 2^63 - 1.
	//
	// Finally, it also satisfies the only real requirement for proper operation
	// of the skip list which is for the calculated height to be less than the
	// provided height.
	return clearLowestOneBit(clearLowestOneBit(height))
}

// initBlockNode initializes a block node from the given header, initialization
// vector for the ticket lottery, and parent node.  The workSum is calculated
// based on the parent, or, in the case no parent is provided, it will just be
// the work for the passed block.
//
// This function is NOT safe for concurrent access.  It must only be called when
// initially creating a node.
func initBlockNode(node *blockNode, blockHeader *wire.BlockHeader, parent *blockNode) {
	*node = blockNode{
		hash:         blockHeader.BlockHash(),
		workSum:      standalone.CalcWork(blockHeader.Bits),
		height:       int64(blockHeader.Height),
		blockVersion: blockHeader.Version,
		voteBits:     blockHeader.VoteBits,
		finalState:   blockHeader.FinalState,
		voters:       blockHeader.Voters,
		freshStake:   blockHeader.FreshStake,
		poolSize:     blockHeader.PoolSize,
		bits:         blockHeader.Bits,
		sbits:        blockHeader.SBits,
		timestamp:    blockHeader.Timestamp.Unix(),
		merkleRoot:   blockHeader.MerkleRoot,
		stakeRoot:    blockHeader.StakeRoot,
		revocations:  blockHeader.Revocations,
		blockSize:    blockHeader.Size,
		nonce:        blockHeader.Nonce,
		extraData:    blockHeader.ExtraData,
		stakeVersion: blockHeader.StakeVersion,
		status:       statusNone,
	}
	if parent != nil {
		node.parent = parent
		node.skipToAncestor = parent.Ancestor(calcSkipListHeight(node.height))
		node.workSum = node.workSum.Add(parent.workSum, node.workSum)
	}
}

// newBlockNode returns a new block node for the given block header and parent
// node.  The workSum is calculated based on the parent, or, in the case no
// parent is provided, it will just be the work for the passed block.
func newBlockNode(blockHeader *wire.BlockHeader, parent *blockNode) *blockNode {
	var node blockNode
	initBlockNode(&node, blockHeader, parent)
	return &node
}

// Header constructs a block header from the node and returns it.
//
// This function is safe for concurrent access.
func (node *blockNode) Header() wire.BlockHeader {
	// No lock is needed because all accessed fields are immutable.
	prevHash := zeroHash
	if node.parent != nil {
		prevHash = &node.parent.hash
	}
	return wire.BlockHeader{
		Version:      node.blockVersion,
		PrevBlock:    *prevHash,
		MerkleRoot:   node.merkleRoot,
		StakeRoot:    node.stakeRoot,
		VoteBits:     node.voteBits,
		FinalState:   node.finalState,
		Voters:       node.voters,
		FreshStake:   node.freshStake,
		Revocations:  node.revocations,
		PoolSize:     node.poolSize,
		Bits:         node.bits,
		SBits:        node.sbits,
		Height:       uint32(node.height),
		Size:         node.blockSize,
		Timestamp:    time.Unix(node.timestamp, 0),
		Nonce:        node.nonce,
		ExtraData:    node.extraData,
		StakeVersion: node.stakeVersion,
	}
}

// lotteryIV returns the initialization vector for the deterministic PRNG used
// to determine winning tickets.
//
// This function is safe for concurrent access.
func (node *blockNode) lotteryIV() chainhash.Hash {
	// Serialize the block header for use in calculating the initialization
	// vector for the ticket lottery.  The only way this can fail is if the
	// process is out of memory in which case it would panic anyways, so
	// although panics are generally frowned upon in package code, it is
	// acceptable here.
	buf := bytes.NewBuffer(make([]byte, 0, wire.MaxBlockHeaderPayload))
	header := node.Header()
	if err := header.Serialize(buf); err != nil {
		panic(err)
	}

	return stake.CalcHash256PRNGIV(buf.Bytes())
}

// populateTicketInfo sets prunable ticket information in the provided block
// node.
//
// This function is NOT safe for concurrent access.  It must only be called when
// initially creating a node or when protected by the block index lock.
func (node *blockNode) populateTicketInfo(spentTickets *stake.SpentTicketsInBlock) {
	node.ticketsVoted = spentTickets.VotedTickets
	node.ticketsRevoked = spentTickets.RevokedTickets
	node.votes = spentTickets.Votes
}

// Ancestor returns the ancestor block node at the provided height by following
// the chain backwards from this node.  The returned block will be nil when a
// height is requested that is after the height of the passed node or is less
// than zero.
//
// This function is safe for concurrent access.
func (node *blockNode) Ancestor(height int64) *blockNode {
	if height < 0 || height > node.height {
		return nil
	}

	n := node
	for n != nil && n.height != height {
		// Skip to the linked ancestor when it won't overshoot the target
		// height.
		if n.skipToAncestor != nil && calcSkipListHeight(n.height) >= height {
			n = n.skipToAncestor
			continue
		}

		n = n.parent
	}

	return n
}

// RelativeAncestor returns the ancestor block node a relative 'distance' blocks
// before this node.  This is equivalent to calling Ancestor with the node's
// height minus provided distance.
//
// This function is safe for concurrent access.
func (node *blockNode) RelativeAncestor(distance int64) *blockNode {
	return node.Ancestor(node.height - distance)
}

// CalcPastMedianTime calculates the median time of the previous few blocks
// prior to, and including, the block node.
//
// This function is safe for concurrent access.
func (node *blockNode) CalcPastMedianTime() time.Time {
	// Create a slice of the previous few block timestamps used to calculate
	// the median per the number defined by the constant medianTimeBlocks.
	timestamps := make([]int64, medianTimeBlocks)
	numNodes := 0
	iterNode := node
	for i := 0; i < medianTimeBlocks && iterNode != nil; i++ {
		timestamps[i] = iterNode.timestamp
		numNodes++

		iterNode = iterNode.parent
	}

	// Prune the slice to the actual number of available timestamps which
	// will be fewer than desired near the beginning of the block chain
	// and sort them.
	timestamps = timestamps[:numNodes]
	sort.Sort(timeSorter(timestamps))

	// NOTE: The consensus rules incorrectly calculate the median for even
	// numbers of blocks.  A true median averages the middle two elements
	// for a set with an even number of elements in it.   Since the constant
	// for the previous number of blocks to be used is odd, this is only an
	// issue for a few blocks near the beginning of the chain.  I suspect
	// this is an optimization even though the result is slightly wrong for
	// a few of the first blocks since after the first few blocks, there
	// will always be an odd number of blocks in the set per the constant.
	//
	// This code follows suit to ensure the same rules are used, however, be
	// aware that should the medianTimeBlocks constant ever be changed to an
	// even number, this code will be wrong.
	medianTimestamp := timestamps[numNodes/2]
	return time.Unix(medianTimestamp, 0)
}

// compareHashesAsUint256LE compares two raw hashes treated as if they were
// little-endian uint256s in a way that is more efficient than converting them
// to big integers first.  It returns 1 when a > b, -1 when a < b, and 0 when a
// == b.
func compareHashesAsUint256LE(a, b *chainhash.Hash) int {
	// Find the index of the first byte that differs.
	index := len(a) - 1
	for ; index >= 0 && a[index] == b[index]; index-- {
		// Nothing to do.
	}
	if index < 0 {
		return 0
	}
	if a[index] > b[index] {
		return 1
	}
	return -1
}

// workSorterLess returns whether node 'a' is a worse candidate than 'b' for the
// purposes of best chain selection.
//
// The criteria for determining what constitutes a worse candidate, in order of
// priority, is as follows:
//
// 1. Less total cumulative work
// 2. Not having block data available
// 3. Receiving data later
// 4. Hash that represents less work (larger value as a little-endian uint256)
//
// This function MUST be called with the block index lock held (for reads).
func workSorterLess(a, b *blockNode) bool {
	// First, sort by the total cumulative work.
	//
	// Blocks with less cumulative work are worse candidates for best chain
	// selection.
	if workCmp := a.workSum.Cmp(b.workSum); workCmp != 0 {
		return workCmp < 0
	}

	// Then sort according to block data availability.
	//
	// Blocks that do not have all of their data available yet are worse
	// candidates than those that do.  They have the same priority if either
	// both have their data available or neither do.
	if aHasData := a.status.HaveData(); aHasData != b.status.HaveData() {
		return !aHasData
	}

	// Then sort according to blocks that received their data first.  Note that
	// the received order will be 0 for both in the case neither block has its
	// data available.
	//
	// Blocks that receive their data later are worse candidates.
	if a.receivedOrderID != b.receivedOrderID {
		// Using greater than here because data that was received later will
		// have a higher id.
		return a.receivedOrderID > b.receivedOrderID
	}

	// Finally, fall back to sorting based on the hash in the case the work,
	// block data availability, and received order are all the same.  In
	// practice, the order will typically only be the same for blocks loaded
	// from disk since the received order is only stored in memory, however it
	// can be the same when the block data for a given header is not yet known
	// as well.
	//
	// Note that it is more difficult to find hashes with more leading zeros
	// when treated as a little-endian uint256, so larger values represent less
	// work and are therefore worse candidates.
	return compareHashesAsUint256LE(&a.hash, &b.hash) > 0
}

// chainTipEntry defines an entry used to track the chain tips and is structured
// such that there is a single statically-allocated field to house a tip, and a
// dynamically-allocated slice for the rare case when there are multiple
// tips at the same height.
//
// This is done to reduce the number of allocations for the common case since
// there is typically only a single tip at a given height.
type chainTipEntry struct {
	tip       *blockNode
	otherTips []*blockNode
}

// blockIndex provides facilities for keeping track of an in-memory index of the
// block chain.  Although the name block chain suggests a single chain of
// blocks, it is actually a tree-shaped structure where any node can have
// multiple children.  However, there can only be one active branch which does
// indeed form a chain from the tip all the way back to the genesis block.
type blockIndex struct {
	// The following fields are set when the instance is created and can't
	// be changed afterwards, so there is no need to protect them with a
	// separate mutex.
	db database.DB

	// These following fields are protected by the embedded mutex.
	//
	// index contains an entry for every known block tracked by the block
	// index.
	//
	// modified contains an entry for all nodes that have been modified
	// since the last time the index was flushed to disk.
	//
	// chainTips contains an entry with the tip of all known side chains.
	//
	// totalTips tracks the total number of all known chain tips.
	sync.RWMutex
	index     map[chainhash.Hash]*blockNode
	modified  map[*blockNode]struct{}
	chainTips map[int64]chainTipEntry
	totalTips uint64

	// These fields are related to selecting the best chain.  They are protected
	// by the embedded mutex.
	//
	// bestHeader tracks the highest work block node in the index that is not
	// known to be invalid.  This is not necessarily the same as the active best
	// chain, especially when block data is not yet known.  However, since block
	// nodes are only added to the index for block headers that pass all sanity
	// and positional checks, which include checking proof of work, it does
	// represent the tip of the header chain with the highest known work that
	// has a reasonably high chance of becoming the best chain tip and is useful
	// for things such as reporting progress and discovering the most suitable
	// blocks to download.
	//
	// bestInvalid tracks the highest work block node that was found to be
	// invalid.
	//
	// bestChainCandidates tracks a set of block nodes in the block index that
	// are potential candidates to become the best chain.
	//
	// unlinkedChildrenOf maps blocks that do not yet have the full block data
	// available to any immediate children that do have the full block data
	// available.  It is used to efficiently discover all child blocks which
	// might be eligible for connection when the full block data for a block
	// becomes available.
	//
	// nextReceivedOrderID is assigned to block nodes and incremented each time
	// block data is received in order to aid in chain selection.  In
	// particular, it helps ensure that no additional priority in terms of chain
	// selection between competing branches can be gained by submitting the
	// header first.
	bestHeader          *blockNode
	bestInvalid         *blockNode
	bestChainCandidates map[*blockNode]struct{}
	unlinkedChildrenOf  map[*blockNode][]*blockNode
	nextReceivedOrderID uint32

	// These fields are related to caching the most recent chain tips.  They are
	// protected by the embedded mutex.
	//
	// cachedTips is similar to chainTips except that it only tracks chain tips
	// starting at the height specified by cachedTipsStart.  It is primarily
	// used to optimize the block invalidation logic.
	//
	// cachedTipsStart is the starting height (inclusive) for which the cached
	// chain tips are tracked.
	//
	// cachedTipsLastPruned is the last time the cached chain tips were pruned.
	cachedTips           map[chainhash.Hash]*blockNode
	cachedTipsStart      int64
	cachedTipsLastPruned time.Time
}

// newBlockIndex returns a new empty instance of a block index.  The index will
// be dynamically populated as block nodes are loaded from the database and
// manually added.
func newBlockIndex(db database.DB) *blockIndex {
	// Notice the next received ID starts at one since all entries loaded from
	// disk will be zero.
	return &blockIndex{
		db:                  db,
		index:               make(map[chainhash.Hash]*blockNode),
		modified:            make(map[*blockNode]struct{}),
		chainTips:           make(map[int64]chainTipEntry),
		cachedTips:          make(map[chainhash.Hash]*blockNode),
		bestChainCandidates: make(map[*blockNode]struct{}),
		unlinkedChildrenOf:  make(map[*blockNode][]*blockNode),
		nextReceivedOrderID: 1,
	}
}

// HaveBlock returns whether or not the block index contains the provided hash
// and the block data is available.
//
// This function is safe for concurrent access.
func (bi *blockIndex) HaveBlock(hash *chainhash.Hash) bool {
	bi.RLock()
	node := bi.lookupNode(hash)
	hasBlock := node != nil && node.status.HaveData()
	bi.RUnlock()
	return hasBlock
}

// addNode adds the provided node to the block index.  Duplicate entries are not
// checked so it is up to caller to avoid adding them.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) addNode(node *blockNode) {
	bi.index[node.hash] = node

	// Since the block index does not support nodes that do not connect to
	// an existing node (except the genesis block), all new nodes are either
	// extending an existing chain or are on a side chain, but in either
	// case, are a new chain tip.  In the case the node is extending a
	// chain, the parent is no longer a tip.
	bi.addChainTip(node)
	if node.parent != nil {
		bi.removeChainTip(node.parent)
	}

	// Update the header with most known work that is also not known to be
	// invalid to this node if needed.
	if !node.status.KnownInvalid() && workSorterLess(bi.bestHeader, node) {
		bi.bestHeader = node
	}
}

// addNodeFromDB adds the provided node, which is expected to have come from
// storage, to the block index and also updates the unlinked block dependencies
// and best known invalid block as needed.
//
// This differs from addNode in that it performs the additional updates to the
// block index which only apply when nodes are first loaded from storage.
//
// This function is NOT safe for concurrent access and therefore must only be
// called during block index initialization.
func (bi *blockIndex) addNodeFromDB(node *blockNode) {
	bi.addNode(node)

	// Add this node to the map of unlinked blocks that are potentially eligible
	// for connection when it is not already fully linked, but the data for it
	// is already known and its parent is not already known to be invalid.
	if !node.isFullyLinked && node.status.HaveData() && node.parent != nil &&
		!node.parent.status.KnownInvalid() {

		unlinkedChildren := bi.unlinkedChildrenOf[node.parent]
		bi.unlinkedChildrenOf[node.parent] = append(unlinkedChildren, node)
	}

	// Set this node as the best known invalid block when it is invalid and has
	// more work than the current one.
	if node.status.KnownInvalid() {
		bi.maybeUpdateBestInvalid(node)
	}
}

// AddNode adds the provided node to the block index and marks it as modified.
// Duplicate entries are not checked so it is up to caller to avoid adding them.
//
// This function is safe for concurrent access.
func (bi *blockIndex) AddNode(node *blockNode) {
	bi.Lock()
	bi.addNode(node)
	bi.modified[node] = struct{}{}
	bi.Unlock()
}

// addChainTip adds the passed block node as a new chain tip.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) addChainTip(tip *blockNode) {
	bi.totalTips++
	bi.cachedTips[tip.hash] = tip

	// When an entry does not already exist for the given tip height, add an
	// entry to the map with the tip stored in the statically-allocated field.
	entry, ok := bi.chainTips[tip.height]
	if !ok {
		bi.chainTips[tip.height] = chainTipEntry{tip: tip}
		return
	}

	// Otherwise, an entry already exists for the given tip height, so store the
	// tip in the dynamically-allocated slice.
	entry.otherTips = append(entry.otherTips, tip)
	bi.chainTips[tip.height] = entry
}

// removeChainTip removes the passed block node from the available chain tips.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) removeChainTip(tip *blockNode) {
	// Remove it from the cached tips as needed.
	delete(bi.cachedTips, tip.hash)

	// Nothing to do if no tips exist at the given height.
	entry, ok := bi.chainTips[tip.height]
	if !ok {
		return
	}

	// The most common case is a single tip at the given height, so handle the
	// case where the tip that is being removed is the tip that is stored in the
	// statically-allocated field first.
	if entry.tip == tip {
		bi.totalTips--
		entry.tip = nil

		// Remove the map entry altogether if there are no more tips left.
		if len(entry.otherTips) == 0 {
			delete(bi.chainTips, tip.height)
			return
		}

		// There are still tips stored in the dynamically-allocated slice, so
		// move the first tip from it to the statically-allocated field, nil the
		// slice so it can be garbage collected when there are no more items in
		// it, and update the map with the modified entry accordingly.
		entry.tip = entry.otherTips[0]
		entry.otherTips = entry.otherTips[1:]
		if len(entry.otherTips) == 0 {
			entry.otherTips = nil
		}
		bi.chainTips[tip.height] = entry
		return
	}

	// The tip being removed is not the tip stored in the statically-allocated
	// field, so attempt to remove it from the dyanimcally-allocated slice.
	for i, n := range entry.otherTips {
		if n == tip {
			bi.totalTips--

			copy(entry.otherTips[i:], entry.otherTips[i+1:])
			entry.otherTips[len(entry.otherTips)-1] = nil
			entry.otherTips = entry.otherTips[:len(entry.otherTips)-1]
			if len(entry.otherTips) == 0 {
				entry.otherTips = nil
			}
			bi.chainTips[tip.height] = entry
			return
		}
	}
}

// forEachChainTip calls the provided function with each chain tip known to the
// block index.  Returning an error from the provided function will stop the
// iteration early and return said error from this function.
//
// This function MUST be called with the block index lock held (for reads).
func (bi *blockIndex) forEachChainTip(f func(tip *blockNode) error) error {
	for _, tipEntry := range bi.chainTips {
		if err := f(tipEntry.tip); err != nil {
			return err
		}
		for _, tip := range tipEntry.otherTips {
			if err := f(tip); err != nil {
				return err
			}
		}
	}
	return nil
}

// forEachChainTipAfterHeight calls the provided function with each chain tip
// known to the block index that has a height which is greater than the provided
// filter node.
//
// Providing a filter node also makes use of the recent chain tip cache when
// possible which typically further reduces the number of chain tips that need
// to be iterated since all old chain tips are pruned from the cache.
//
// Returning an error from the provided function will stop the iteration early
// and return said error from this function.
//
// This function MUST be called with the block index lock held (for reads).
func (bi *blockIndex) forEachChainTipAfterHeight(filter *blockNode, f func(tip *blockNode) error) error {
	// Use the cached recent chain tips when the filter height permits it.
	if filter.height >= bi.cachedTipsStart-1 {
		for _, tip := range bi.cachedTips {
			// Ignore any chain tips at the same or lower heights than the
			// provided filter.
			if tip.height <= filter.height {
				continue
			}

			if err := f(tip); err != nil {
				return err
			}
		}
		return nil
	}

	// Fall back to iterating through all chain tips when the filter height is
	// prior to the point the cached recent chain tips are tracking.
	for tipHeight, tipEntry := range bi.chainTips {
		// Ignore any chain tips at the same or lower heights than the provided
		// filter.
		if tipHeight <= filter.height {
			continue
		}

		if err := f(tipEntry.tip); err != nil {
			return err
		}
		for _, tip := range tipEntry.otherTips {
			if err := f(tip); err != nil {
				return err
			}
		}
	}

	return nil
}

// lookupNode returns the block node identified by the provided hash.  It will
// return nil if there is no entry for the hash.
//
// This function MUST be called with the block index lock held (for reads).
func (bi *blockIndex) lookupNode(hash *chainhash.Hash) *blockNode {
	return bi.index[*hash]
}

// LookupNode returns the block node identified by the provided hash.  It will
// return nil if there is no entry for the hash.
//
// This function is safe for concurrent access.
func (bi *blockIndex) LookupNode(hash *chainhash.Hash) *blockNode {
	bi.RLock()
	node := bi.lookupNode(hash)
	bi.RUnlock()
	return node
}

// PopulateTicketInfo sets prunable ticket information in the provided block
// node.
//
// This function is safe for concurrent access.
func (bi *blockIndex) PopulateTicketInfo(node *blockNode, spentTickets *stake.SpentTicketsInBlock) {
	bi.Lock()
	node.populateTicketInfo(spentTickets)
	bi.modified[node] = struct{}{}
	bi.Unlock()
}

// NodeStatus returns the status associated with the provided node.
//
// This function is safe for concurrent access.
func (bi *blockIndex) NodeStatus(node *blockNode) blockStatus {
	bi.RLock()
	status := node.status
	bi.RUnlock()
	return status
}

// setStatusFlags sets the provided status flags for the given block node
// regardless of their previous state.  It does not unset any flags.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) setStatusFlags(node *blockNode, flags blockStatus) {
	origStatus := node.status
	node.status |= flags
	if node.status != origStatus {
		bi.modified[node] = struct{}{}
	}
}

// SetStatusFlags sets the provided status flags for the given block node
// regardless of their previous state.  It does not unset any flags.
//
// This function is safe for concurrent access.
func (bi *blockIndex) SetStatusFlags(node *blockNode, flags blockStatus) {
	bi.Lock()
	bi.setStatusFlags(node, flags)
	bi.Unlock()
}

// unsetStatusFlags unsets the provided status flags for the given block node
// regardless of their previous state.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) unsetStatusFlags(node *blockNode, flags blockStatus) {
	origStatus := node.status
	node.status &^= flags
	if node.status != origStatus {
		bi.modified[node] = struct{}{}
	}
}

// UnsetStatusFlags unsets the provided status flags for the given block node
// regardless of their previous state.
//
// This function is safe for concurrent access.
func (bi *blockIndex) UnsetStatusFlags(node *blockNode, flags blockStatus) {
	bi.Lock()
	bi.unsetStatusFlags(node, flags)
	bi.Unlock()
}

// addBestChainCandidate adds the passed block node as a potential candidate
// for becoming the tip of the best chain.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) addBestChainCandidate(node *blockNode) {
	bi.bestChainCandidates[node] = struct{}{}
}

// pruneCachedTips removes old cached chain tips used to optimize block
// invalidation by treating the passed best known block as a reference point.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) pruneCachedTips(bestNode *blockNode) {
	// No blocks exist before height 0.
	height := bestNode.height - cachedTipsPruneDepth
	if height <= 0 {
		bi.cachedTipsLastPruned = time.Now()
		return
	}

	for hash, n := range bi.cachedTips {
		if n.height < height {
			delete(bi.cachedTips, hash)
		}
	}
	bi.cachedTipsStart = height
	bi.cachedTipsLastPruned = time.Now()
}

// MaybePruneCachedTips periodically removes old cached chain tips used to
// optimize block invalidation by treating the passed best known block as a
// reference point.
//
// This function is safe for concurrent access.
func (bi *blockIndex) MaybePruneCachedTips(bestNode *blockNode) {
	bi.Lock()
	if time.Since(bi.cachedTipsLastPruned) >= cachedTipsPruneInterval {
		bi.pruneCachedTips(bestNode)
	}
	bi.Unlock()
}

// removeBestChainCandidate removes the passed block node from the potential
// candidates for becoming the tip of the best chain.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) removeBestChainCandidate(node *blockNode) {
	delete(bi.bestChainCandidates, node)
}

// maybeUpdateBestInvalid potentially updates the best known invalid block, as
// determined by having the most cumulative work, by comparing the passed block
// node, which must have already been determined to be invalid, against the
// current one.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) maybeUpdateBestInvalid(invalidNode *blockNode) {
	if bi.bestInvalid == nil || workSorterLess(bi.bestInvalid, invalidNode) {
		bi.bestInvalid = invalidNode
	}
}

// maybeUpdateBestHeaderForTip potentially updates the best known header that is
// not known to be invalid, as determined by having the most cumulative work.
// It works by walking backwards from the provided tip so long as those headers
// have more work than the current best header and selecting the first one that
// is not known to be invalid.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) maybeUpdateBestHeaderForTip(tip *blockNode) {
	for n := tip; n != nil && workSorterLess(bi.bestHeader, n); n = n.parent {
		if !n.status.KnownInvalid() {
			bi.bestHeader = n
			return
		}
	}
}

// MarkBlockFailedValidation marks the passed node as having failed validation
// and then marks all of its descendants (if any) as having a failed ancestor.
//
// This function is safe for concurrent access.
func (bi *blockIndex) MarkBlockFailedValidation(node *blockNode) {
	bi.Lock()
	bi.setStatusFlags(node, statusValidateFailed)
	bi.unsetStatusFlags(node, statusValidated)
	bi.removeBestChainCandidate(node)
	bi.maybeUpdateBestInvalid(node)
	delete(bi.unlinkedChildrenOf, node)

	// Mark all descendants of the failed block as having a failed ancestor.
	//
	// In order to fairly efficiently determine all of the descendants of the
	// block without having to iterate the entire block index, walk through all
	// of the known chain tips and check if the block being invalidated is an
	// ancestor of the tip.  In the case it is, then all blocks between that tip
	// and the failed block are descendants.  As an additional optimization, a
	// cache of recent tips (those after a recent height) is maintained and used
	// when possible to reduce the number of potential affected chain tips that
	// need to be iterated.
	//
	// In order to help visualize the logic, consider the following block tree
	// with several branches:
	//
	// 100 -> 101  -> 102  -> 103  -> 104  -> 105  -> 106  -> 107  -> 108
	//    \-> 101a -> 102a -> 103a -> 104a -> 105a        \-> 107a
	//    \-> 101b    ----        |       \-> 105b -> 106b
	//                 ^^         \-> 104c -> 105c -> 106c
	//               Failed       \-> 104d -> 105d
	//
	// Further, assume block 102a failed validation.  As can be seen, its
	// descendants are 103a, 104a, 105a, 105b, 106b, 104c, 105c, 106c, 104d, and
	// 105d, and the chain tips of this hypothetical block tree would be 101b,
	// 105a, 105d, 106b, 106c, 107a, and 108.
	//
	// Since the failed block, 102a, is not an ancestor of tips 101b, 107a, or
	// 108, those tips are ignored.  Also notice that, of the remaining tips,
	// 103a is a common ancestor to all of them, and 104a is a common ancestor
	// to tips 105a and 106b.
	//
	// Given all of the above, the blocks would semantically be marked as having
	// an invalid ancestor as follows:
	//
	// Tip 105a: 105a, 104a, 103a       (102a is failed block, next)
	// Tip 106b: 106b, 105b, 104a, 103a (102a is failed block, next)
	// Tip 106c: 106c, 105c, 104c, 103a (102a is failed block, next)
	// Tip 105d: 105d, 104d, 103a       (102a is failed block, next)
	//
	// Note that it might be tempting to consider trying to optimize this to
	// skip directly to the next tip once a node with a common invalid ancestor
	// is found.  However, that would result in incorrect behavior if a block is
	// marked invalid deeper in a branch first and then an earlier block is
	// later marked invalid as it would result in skipping the intermediate
	// blocks thereby NOT marking them as invalid as they should be.
	//
	// For example, consider what happens if 105c is marked invalid prior to the
	// block data for 102a becoming available and found to be invalid.  Blocks
	// 106c and 105c would already be marked invalid, and blocks 104c and 103a
	// need to be marked invalid.
	markDescendantsInvalid := func(node, tip *blockNode) {
		// Nothing to do if the node is not an ancestor of the given chain tip.
		if tip.Ancestor(node.height) != node {
			return
		}

		// Set this chain tip as the best known invalid block when it has more
		// work than the current one.
		bi.maybeUpdateBestInvalid(tip)

		// Mark everything that descends from the failed block as having an
		// invalid ancestor.
		for n := tip; n != node; n = n.parent {
			// Skip blocks that are already known to have an invalid ancestor.
			if n.status.KnownInvalidAncestor() {
				continue
			}

			bi.setStatusFlags(n, statusInvalidAncestor)
			bi.unsetStatusFlags(n, statusValidated)
			bi.removeBestChainCandidate(n)

			// Remove any children that depend on the failed block from the set
			// of unlinked blocks accordingly since they are no longer eligible
			// for connection even if the full block data for a block becomes
			// available.
			delete(bi.unlinkedChildrenOf, n)
		}
	}

	// Chain tips at the same or lower heights than the failed block can't
	// possibly be descendants of it, so use it as the lower height bound filter
	// when iterating chain tips.  Note that this will make use of the cache of
	// recent tips when possible.
	bi.forEachChainTipAfterHeight(node, func(tip *blockNode) error {
		markDescendantsInvalid(node, tip)
		return nil
	})

	// Update the best header if the current one is now invalid which will be
	// the case when the best header is a descendant of the failed block.
	if bi.bestHeader.status.KnownInvalid() {
		// Use the first ancestor of the failed block that is not known to be
		// invalid as the lower bound for the best header.  This will typically
		// be the parent of the failed block, but it might be some more distant
		// ancestor when performing manual invalidation.
		n := node.parent
		for n != nil && n.status.KnownInvalid() {
			n = n.parent
		}
		bi.bestHeader = n

		// Scour the block tree to find a new best header.
		//
		// Note that all chain tips must be iterated versus filtering based on
		// the current best header height because, while uncommon, it is
		// possible for lower heights to have more work.
		bi.forEachChainTip(func(tip *blockNode) error {
			// Skip chain tips that are descendants of the failed block since
			// none of the intermediate headers are eligible to become the best
			// header given they all have an invalid ancestor.
			if tip.Ancestor(node.height) == node {
				return nil
			}

			bi.maybeUpdateBestHeaderForTip(tip)
			return nil
		})
	}
	bi.Unlock()
}

// canValidate returns whether or not the block associated with the provided
// node can be validated.  In order for a block to be validated, both it, and
// all of its ancestors, must have the block data available.
//
// This function MUST be called with the block index lock held (for reads).
func (bi *blockIndex) canValidate(node *blockNode) bool {
	return node.isFullyLinked && node.status.HaveData()
}

// CanValidate returns whether or not the block associated with the provided
// node can be validated.  In order for a block to be validated, both it, and
// all of its ancestors, must have the block data available.
//
// This function is safe for concurrent access.
func (bi *blockIndex) CanValidate(node *blockNode) bool {
	bi.RLock()
	canValidate := bi.canValidate(node)
	bi.RUnlock()
	return canValidate
}

// removeLessWorkCandidates removes all potential best chain candidates that
// have less work than the provided node, which is typically a newly connected
// best chain tip.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) removeLessWorkCandidates(node *blockNode) {
	// Remove all best chain candidates that have less work than the passed
	// node.
	for n := range bi.bestChainCandidates {
		if n.workSum.Cmp(node.workSum) < 0 {
			bi.removeBestChainCandidate(n)
		}
	}

	// The best chain candidates must always contain at least the current best
	// chain tip.  Assert this assumption is true.
	if len(bi.bestChainCandidates) == 0 {
		panicf("best chain candidates list is empty after removing less work " +
			"candidates")
	}
}

// RemoveLessWorkCandidates removes all potential best chain candidates that
// have less work than the provided node, which is typically a newly connected
// best chain tip.
//
// This function is safe for concurrent access.
func (bi *blockIndex) RemoveLessWorkCandidates(node *blockNode) {
	bi.Lock()
	bi.removeLessWorkCandidates(node)
	bi.Unlock()
}

// linkBlockData marks the provided block as fully linked to indicate that both
// it and all of its ancestors have their data available and then determines if
// there are any unlinked blocks which depend on the passed block and links
// those as well until there are no more.  It returns a list of blocks that were
// linked.
//
// It also accounts for the order that the blocks are linked and potentially
// adds the newly-linked blocks as best chain candidates if they have more
// cumulative work than the current best chain tip.
//
// This function MUST be called with the block index lock held (for writes).
func (bi *blockIndex) linkBlockData(node, tip *blockNode) []*blockNode {
	// Start with processing at least the passed node.
	//
	// Note that no additional space is preallocated here because it is fairly
	// rare (after the initial sync) for there to be more than the single block
	// being linked and thus it will typically remain on the stack and avoid an
	// allocation.
	linkedNodes := []*blockNode{node}
	for nodeIndex := 0; nodeIndex < len(linkedNodes); nodeIndex++ {
		linkedNode := linkedNodes[nodeIndex]

		// Mark the block as fully linked to indicate that both it and all of
		// its ancestors have their data available.
		linkedNode.isFullyLinked = true

		// Keep track of the order in which the block data was received to
		// ensure miners gain no advantage by advertising the header first.
		linkedNode.receivedOrderID = bi.nextReceivedOrderID
		bi.nextReceivedOrderID++

		// The block is now a candidate to potentially become the best chain if
		// it has the same or more work than the current best chain tip.
		if linkedNode.workSum.Cmp(tip.workSum) >= 0 {
			bi.addBestChainCandidate(linkedNode)
		}

		// Add any children of the block that was just linked to the list to be
		// linked and remove them from the set of unlinked blocks accordingly.
		// There will typically only be zero or one, but it could be more if
		// multiple solutions are mined and broadcast around the same time.
		unlinkedChildren := bi.unlinkedChildrenOf[linkedNode]
		if len(unlinkedChildren) > 0 {
			linkedNodes = append(linkedNodes, unlinkedChildren...)
			delete(bi.unlinkedChildrenOf, linkedNode)
		}
	}

	return linkedNodes
}

// AcceptBlockData updates the block index state to account for the full data
// for a block becoming available.  For example, blocks that are currently not
// eligible for validation due to either not having the block data itself or not
// having all ancestor data available might become eligible for validation.  It
// returns a list of all blocks that were linked, if any.
//
// NOTE: It is up to the caller to only call this function when the data was not
// previously available.
//
// This function is safe for concurrent access.
func (bi *blockIndex) AcceptBlockData(node, tip *blockNode) []*blockNode {
	// The passed block, and any blocks that also have their data available, are
	// now eligible for validation when the parent of the passed block is also
	// eligible (or has already been validated).
	var linkedBlocks []*blockNode
	bi.Lock()
	if bi.canValidate(node.parent) {
		linkedBlocks = bi.linkBlockData(node, tip)
	} else if !node.parent.status.KnownInvalid() {
		unlinkedChildren := bi.unlinkedChildrenOf[node.parent]
		bi.unlinkedChildrenOf[node.parent] = append(unlinkedChildren, node)
	}
	bi.Unlock()
	return linkedBlocks
}

// FindBestChainCandidate searches the block index for the best potentially
// valid chain that contains the most cumulative work and returns its tip.  In
// order to be potentially valid, all of the block data leading up to a block
// must have already been received and must not be part of a chain that is
// already known to be invalid.  A chain that has not yet been fully validated,
// such as a side chain that has never been the main chain, is neither known to
// be valid nor invalid, so it is possible that the returned candidate will form
// a chain that is invalid.
//
// This function is safe for concurrent access.
func (bi *blockIndex) FindBestChainCandidate() *blockNode {
	bi.RLock()
	defer bi.RUnlock()

	// Find the best candidate among the potential candidates as determined by
	// having the highest cumulative work with fallback to the criteria
	// described by the invoked function in the case of equal work.
	//
	// Note that the best candidate should never actually be nil in practice
	// since the current best tip is always a candidate.
	var bestCandidate *blockNode
	for node := range bi.bestChainCandidates {
		if bestCandidate == nil || workSorterLess(bestCandidate, node) {
			bestCandidate = node
		}
	}
	return bestCandidate
}

// flush writes all of the modified block nodes to the database and clears the
// set of modified nodes if it succeeds.
func (bi *blockIndex) flush() error {
	// Nothing to flush if there are no modified nodes.
	bi.Lock()
	if len(bi.modified) == 0 {
		bi.Unlock()
		return nil
	}

	// Write all of the nodes in the set of modified nodes to the database.
	err := bi.db.Update(func(dbTx database.Tx) error {
		for node := range bi.modified {
			err := dbPutBlockNode(dbTx, node)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		bi.Unlock()
		return err
	}

	// Clear the set of modified nodes.
	bi.modified = make(map[*blockNode]struct{})
	bi.Unlock()
	return nil
}
