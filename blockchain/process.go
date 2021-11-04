// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"errors"
	"fmt"

	"github.com/decred/dcrd/blockchain/stake/v4"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/wire"
)

// BehaviorFlags is a bitmask defining tweaks to the normal behavior when
// performing chain processing and consensus rules checks.
type BehaviorFlags uint32

const (
	// BFFastAdd may be set to indicate that several checks can be avoided
	// for the block since it is already known to fit into the chain due to
	// already proving it correctly links into the chain up to a known
	// checkpoint.  This is primarily used for headers-first mode.
	BFFastAdd BehaviorFlags = 1 << iota

	// BFNoPoWCheck may be set to indicate the proof of work check which
	// ensures a block hashes to a value less than the required target will
	// not be performed.
	BFNoPoWCheck

	// BFNone is a convenience value to specifically indicate no flags.
	BFNone BehaviorFlags = 0
)

// checkKnownInvalidBlock returns an appropriate error when the provided block
// is known to be invalid either due to failing validation itself or due to
// having a known invalid ancestor (aka being part of an invalid branch).
//
// This function is safe for concurrent access.
func (b *BlockChain) checkKnownInvalidBlock(node *blockNode) error {
	status := b.index.NodeStatus(node)
	if status.KnownValidateFailed() {
		str := fmt.Sprintf("block %s is known to be invalid", node.hash)
		return ruleError(ErrKnownInvalidBlock, str)
	}
	if status.KnownInvalidAncestor() {
		str := fmt.Sprintf("block %s is known to be part of an invalid branch",
			node.hash)
		return ruleError(ErrInvalidAncestorBlock, str)
	}

	return nil
}

// maybeAcceptBlockHeader potentially accepts the header to the block index and,
// if accepted, returns the block node associated with the header.  It performs
// several context independent checks as well as those which depend on its
// position within the chain.  It should be noted that some of the header fields
// require the full block data to be available in order to be able to validate
// them, so those fields are not included here.  This provides support for full
// headers-first semantics.
//
// The flag for check header sanity allows the additional header sanity checks
// to be skipped which is useful for the full block processing path which checks
// the sanity of the entire block, including the header, before attempting to
// accept its header in order to quickly eliminate blocks that are obviously
// incorrect.
//
// In the case the block header is already known, the associated block node is
// examined to determine if the block is already known to be invalid, in which
// case an appropriate error will be returned.  Otherwise, the block node is
// returned.
//
// The flags do not modify the behavior of this function directly, however they
// are needed to pass along to checkBlockHeaderSanity and
// checkBlockHeaderPositional.
//
// This function MUST be called with the chain lock held (for writes).
func (b *BlockChain) maybeAcceptBlockHeader(header *wire.BlockHeader, flags BehaviorFlags, checkHeaderSanity bool) (*blockNode, error) {
	// Avoid validating the header again if its validation status is already
	// known.  Invalid headers are never added to the block index, so if there
	// is an entry for the block hash, the header itself is known to be valid.
	// However, it might have since been marked invalid either due to the
	// associated block, or an ancestor, later failing validation.
	hash := header.BlockHash()
	if node := b.index.LookupNode(&hash); node != nil {
		if err := b.checkKnownInvalidBlock(node); err != nil {
			return nil, err
		}

		return node, nil
	}

	// Perform context-free sanity checks on the block header.
	if checkHeaderSanity {
		err := checkBlockHeaderSanity(header, b.timeSource, flags, b.chainParams)
		if err != nil {
			return nil, err
		}
	}

	// Orphan headers are not allowed and this function should never be called
	// with the genesis block.
	prevHash := &header.PrevBlock
	prevNode := b.index.LookupNode(prevHash)
	if prevNode == nil {
		str := fmt.Sprintf("previous block %s is not known", prevHash)
		return nil, ruleError(ErrMissingParent, str)
	}

	// There is no need to validate the header if an ancestor is already known
	// to be invalid.
	prevNodeStatus := b.index.NodeStatus(prevNode)
	if prevNodeStatus.KnownInvalid() {
		str := fmt.Sprintf("previous block %s is known to be invalid", prevHash)
		return nil, ruleError(ErrInvalidAncestorBlock, str)
	}

	// The block header must pass all of the validation rules which depend on
	// its position within the block chain.
	err := b.checkBlockHeaderPositional(header, prevNode, flags)
	if err != nil {
		return nil, err
	}

	// Create a new block node for the block and add it to the block index.
	//
	// Note that the additional information for the actual votes, tickets, and
	// revocations in the block can't be populated until the full block data is
	// known since that information is not available in the header.
	newNode := newBlockNode(header, prevNode)
	newNode.status = statusNone
	b.index.AddNode(newNode)

	// Potentially update the most recently known checkpoint to this block
	// header.
	b.maybeUpdateMostRecentCheckpoint(newNode)

	return newNode, nil
}

// ProcessBlockHeader is the main workhorse for handling insertion of new block
// headers into the block chain using headers-first semantics.  It includes
// functionality such as rejecting headers that do not connect to an existing
// known header, ensuring headers follow all rules that do not depend on having
// all ancestor block data available, and insertion into the block index.
//
// Block headers that have already been inserted are ignored, unless they have
// subsequently been marked invalid, in which case an appropriate error is
// returned.
//
// It should be noted that this function intentionally does not accept block
// headers that do not connect to an existing known header or to headers which
// are already known to be a part of an invalid branch.  This means headers must
// be processed in order.
//
// This function is safe for concurrent access.
func (b *BlockChain) ProcessBlockHeader(header *wire.BlockHeader, flags BehaviorFlags) error {
	b.processLock.Lock()
	defer b.processLock.Unlock()

	// Potentially accept the header to the block index.  When the header
	// already exists in the block index, this acts as a lookup of the existing
	// node along with a status check to avoid additional work when possible.
	//
	// On the other hand, when the header does not already exist in the block
	// index, validate it according to both context free and context dependent
	// positional checks, and create a block index entry for it.
	b.chainLock.Lock()
	const checkHeaderSanity = true
	_, err := b.maybeAcceptBlockHeader(header, flags, checkHeaderSanity)
	if err != nil {
		b.chainLock.Unlock()
		return err
	}

	// Write any modified block index entries to the database since any new
	// headers will have added a new entry.
	if err := b.flushBlockIndex(); err != nil {
		b.chainLock.Unlock()
		return err
	}
	b.chainLock.Unlock()

	return nil
}

// maybeAcceptBlockData potentially accepts the data for the given block into
// the database, updates the block index state to account for the full data now
// being available, and returns a list of all descendant blocks that already
// have their respective data available and are now therefore eligible for
// validation.
//
// The block is only accepted if it passes several validation checks which
// depend on its position within the block chain and having the headers of all
// ancestors available.  This function does not, and must not, rely on having
// the full block data of all ancestors available.
//
// Note that this currently expects that it is only ever called from
// ProcessBlock which already checked the block sanity.  Care must be taken if
// the code is changed to violate that assumption.
//
// The flags do not modify the behavior of this function directly, however they
// are needed to pass along to checkBlockPositional.
//
// This function MUST be called with the chain lock held (for writes).
func (b *BlockChain) maybeAcceptBlockData(node *blockNode, block *dcrutil.Block, flags BehaviorFlags) ([]*blockNode, error) {
	// Nothing more to do if the block data is already available.  Note that
	// this function is never called when the data is already available at the
	// time this comment was written, but it's a fast check and will prevent
	// incorrect behavior if that changes at some point in the future.
	if b.index.NodeStatus(node).HaveData() {
		return nil, nil
	}

	// Populate the prunable information that is related to tickets and votes.
	ticketInfo := stake.FindSpentTicketsInBlock(block.MsgBlock())
	b.index.PopulateTicketInfo(node, ticketInfo)

	// The block must pass all of the validation rules which depend on the
	// position of the block within the block chain.  Not that this only checks
	// the block data, not including the header, because the header was already
	// checked when it was accepted to the block index.
	err := b.checkBlockDataPositional(block, node.parent, flags)
	if err != nil {
		b.index.MarkBlockFailedValidation(node)
		return nil, err
	}

	// Prune stake nodes that are no longer needed.
	b.pruner.pruneChainIfNeeded()

	// Insert the block into the database if it's not already there.  Even
	// though it is possible the block will ultimately fail to connect, it has
	// already passed all proof-of-work and validity tests which means it would
	// be prohibitively expensive for an attacker to fill up the disk with a
	// bunch of blocks that fail to connect.  This is necessary since it allows
	// block download to be decoupled from the much more expensive connection
	// logic.  It also has some other nice properties such as making blocks that
	// never become part of the main chain or blocks that fail to connect
	// available for further analysis.
	err = b.db.Update(func(dbTx database.Tx) error {
		return dbMaybeStoreBlock(dbTx, block)
	})
	if err != nil {
		return nil, err
	}
	b.index.SetStatusFlags(node, statusDataStored)

	// Update the block index state to account for the full data for the block
	// now being available.  This might result in the block, and any others that
	// are descendants of it, becoming fully linked (meaning a block has all of
	// its own data available and all of its ancestors also have their data
	// available) which makes them eligible for full validation.
	tip := b.bestChain.Tip()
	linkedBlocks := b.index.AcceptBlockData(node, tip)

	return linkedBlocks, nil
}

// maybeAcceptBlocks tentatively accepts the given blocks, which must have
// already been determined to be fully linked by the caller, to the chain if
// they pass several validation checks which depend on having the full block
// data for all of their ancestors available and updates the block index state
// to account for any that fail validation.
//
// It returns those that were accepted along with an error that applies to the
// first one that failed validation (if any).  This is sufficient because the
// provided blocks must all be descendants of previous ones which means all of
// the remaining ones after a validation failure are not eligible for further
// processing and acceptance because they have an invalid ancestor.
//
// The flags do not modify the behavior of this function directly, however they
// are needed to pass along to checkBlockContext.
//
// This function MUST be called with the chain lock held (for writes).
func (b *BlockChain) maybeAcceptBlocks(curTip *blockNode, nodes []*blockNode, flags BehaviorFlags) ([]*blockNode, error) {
	isCurrent := b.isCurrent(curTip)
	for i, n := range nodes {
		var err error
		linkedBlock, err := b.fetchBlockByNode(n)
		if err != nil {
			return nodes[:i], err
		}

		// The block must pass all of the validation rules which depend on
		// having the full block data for all of its ancestors available.
		if err := b.checkBlockContext(linkedBlock, n.parent, flags); err != nil {
			var rErr RuleError
			if errors.As(err, &rErr) {
				b.index.MarkBlockFailedValidation(n)
			}

			return nodes[:i], err
		}

		// Cache the block and mark it as recently checked to avoid loading and
		// checking it again when connecting it in the typical case.  Since the
		// cache is limited in size, it is technically possible that a large
		// enough chain of blocks becoming linked at once will end up evicting
		// some of the early ones, but the only effect in that case is
		// potentially having to load the block and run the context checks again
		// later.  That said, in practice, eviction of items essentially never
		// happens under normal operation, especially once the chain is fully
		// synced.
		b.addRecentBlock(linkedBlock)
		b.recentContextChecks.Add(n.hash)

		// Notify the caller when the block intends to extend the main chain,
		// the chain believes it is current, and the block has passed all of the
		// sanity and contextual checks, such as having valid proof of work,
		// valid merkle and stake roots, and only containing allowed votes and
		// revocations.
		//
		// This allows the block to be relayed before doing the more expensive
		// connection checks, because even though the block might still fail to
		// connect and become the new main chain tip, that is quite rare in
		// practice since a lot of work was expended to create a block that
		// satisfies the proof of work requirement.
		//
		// Notice that the chain lock is not released before sending the
		// notification.  This is intentional and must not be changed without
		// understanding why!
		if n.parent == curTip && isCurrent {
			b.sendNotification(NTNewTipBlockChecked, linkedBlock)
		}
	}

	return nodes, nil
}

// ProcessBlock is the main workhorse for handling insertion of new blocks into
// the block chain.  It includes functionality such as rejecting duplicate
// blocks, ensuring blocks follow all rules, and insertion into the block chain
// along with best chain selection and reorganization.
//
// This function permits blocks to be processed out of order so long as their
// header has already been successfully processed via ProcessBlockHeader which
// itself requires the headers to properly connect.  In other words, orphan
// blocks are rejected and thus is up to the caller to either ensure that the
// blocks are processed in order or that the headers for the blocks have already
// been successfully processed.
//
// Upon return, the best chain tip will be whatever branch tip has the most
// proof of work and also passed all validation checks.  Due to this, it is also
// worth noting that the best chain tip might be updated even in the case of
// processing a block that ultimately fails validation.
//
// Additionally, due to the ability to process blocks out of order, and the fact
// blocks can only be fully validated once all of their ancestors have the block
// data available, it is to be expected that no error is returned immediately
// for blocks that are valid enough to make it to the point they require the
// remaining ancestor block data to be fully validated even though they might
// ultimately end up failing validation.  Similarly, because the data for a
// block becoming available makes any of its direct descendants that already
// have their data available eligible for validation, an error being returned
// does not necessarily mean the block being processed is the one that failed
// validation.
//
// When no errors occurred during processing, the first return value indicates
// the length of the fork the block extended.  In the case it either extended
// the best chain or is now the tip of the best chain due to causing a
// reorganize, the fork length will be 0.
//
// This function is safe for concurrent access.
func (b *BlockChain) ProcessBlock(block *dcrutil.Block) (int64, error) {
	// Since the chain lock is periodically released to send notifications,
	// protect the overall processing of blocks with a separate mutex.
	b.processLock.Lock()
	defer b.processLock.Unlock()

	// The block must not already exist in the main chain or side chains.
	blockHash := block.Hash()
	if b.index.HaveBlock(blockHash) {
		str := fmt.Sprintf("already have block %v", blockHash)
		return 0, ruleError(ErrDuplicateBlock, str)
	}

	b.chainLock.Lock()
	defer b.chainLock.Unlock()

	// Reject blocks that are already known to be invalid immediately to avoid
	// additional work when possible.
	node := b.index.LookupNode(block.Hash())
	if node != nil {
		if err := b.checkKnownInvalidBlock(node); err != nil {
			return 0, err
		}
	}

	// Perform preliminary sanity checks on the block and its transactions.
	// This is done prior to any attempts to accept the block data and connect
	// the block to quickly eliminate blocks that are obviously incorrect and
	// significantly increase the cost to attackers.  Of particular note is that
	// the checks include proof-of-work validation which means a significant
	// amount of work must have been done in order to pass this check.
	err := checkBlockSanity(block, b.timeSource, BFNone, b.chainParams)
	if err != nil {
		// When there is a block index entry for the block, which will be the
		// case if the header was previously seen and passed all validation,
		// mark it as having failed validation and all of its descendants as
		// having an invalid ancestor.
		if node != nil {
			b.index.MarkBlockFailedValidation(node)
		}
		return 0, err
	}

	// Potentially accept the header to the block index when it does not already
	// exist.
	//
	// This entails fully validating it according to both context independent
	// and context dependent checks and creating a block index entry for it.
	//
	// Note that the header sanity checks are skipped because they were just
	// performed above as part of the full block sanity checks.
	if node == nil {
		const checkHeaderSanity = false
		header := &block.MsgBlock().Header
		node, err = b.maybeAcceptBlockHeader(header, BFNone, checkHeaderSanity)
		if err != nil {
			return 0, err
		}
	}

	// Enable skipping some of the more expensive validation checks when the
	// block is an ancestor of a known good checkpoint.
	flags := BFNone
	if b.bulkImportMode || b.isKnownCheckpointAncestor(node) {
		b.index.SetStatusFlags(node, statusValidated)
		flags |= BFFastAdd
	}

	// Potentially accept the block data into the database and update the block
	// index state to account for the full data now being available.
	//
	// This consists of performing several validation checks which depend on the
	// block's position within the block chain and determining if the block, and
	// any descendants of it are now eligible for full validation due to being
	// fully linked (meaning a block has all of its own data available and all
	// of its ancestors also have their data available).
	//
	// The returned linked block nodes are for those aforementioned blocks that
	// are now eligible for validation.
	linkedNodes, err := b.maybeAcceptBlockData(node, block, flags)
	if err != nil {
		return 0, err
	}

	// Write any modified block index entries to the database since any new
	// headers will have added a new entry and the block will be marked as now
	// having its data stored.
	if err := b.flushBlockIndex(); err != nil {
		return 0, err
	}

	// Tentatively accept the linked blocks to the chain if they pass several
	// validation checks which depend on having the full block data for all of
	// their ancestors available and update the block index state to account for
	// any that fail validation.
	//
	// Note that this is done here because it allows any blocks that fail this
	// level of validation to be detected and discounted early before doing more
	// work.
	//
	// Also, any blocks that do not ultimately end up becoming part of the best
	// chain would otherwise not have contextual checks run on them, which is
	// required before accepting them, without somewhat more complicated logic
	// later to detect them.
	var finalErr error
	currentTip := b.bestChain.Tip()
	b.addRecentBlock(block)
	acceptedNodes, err := b.maybeAcceptBlocks(currentTip, linkedNodes, flags)
	if err != nil {
		finalErr = err

		// This intentionally falls through since the code below must run
		// whether or not any blocks were accepted.
	}

	// Determine what the expected effects of the block, in terms of forks and
	// reorganizations, will be on the chain and log it.
	//
	// 1) There is no effect if the block is not able to be validated yet
	// 2) The block is causing a reorg when the new current best tip is not an
	//    ancestor of the new target tip
	// 3) The block is either forking the best chain or extending an existing
	//    fork of it when it does not cause a reorg and it is not an ancestor
	//    of the new target tip
	target := b.index.FindBestChainCandidate()
	if b.index.CanValidate(node) {
		triggersReorg := !currentTip.IsAncestorOf(target)
		if triggersReorg {
			log.Infof("REORGANIZE: Block %v is causing a reorganize", node.hash)
		} else if !node.IsAncestorOf(target) {
			fork := b.bestChain.FindFork(node)
			if fork == node.parent {
				log.Infof("FORK: Block %v (height %v) forks the chain at "+
					"height %d/block %v, but does not cause a reorganize",
					node.hash, node.height, fork.height, fork.hash)
			} else {
				log.Infof("EXTEND FORK: Block %v (height %v) extends a side "+
					"chain which forks the chain at height %d/block %v",
					node.hash, node.height, fork.height, fork.hash)
			}
		}
	}

	// Find the best chain candidate and attempt to reorganize the chain to it.
	// This will have no effect when the target is the same as the current best
	// chain tip.
	//
	// Note that any errors that take place in the reorg will be attributed to
	// the block being processed.  The calling code currently depends on this
	// behavior, so care must be taken if this behavior is changed.
	reorgErr := b.reorganizeChain(target)
	switch {
	// The final error is just the reorg error in the case there was no error
	// carried forward from above.
	case reorgErr != nil && finalErr == nil:
		finalErr = reorgErr

	// The final error is a multi error when there is a reorg error and an error
	// was carried forward from above.  Additionally, in the case the reorg
	// error is itself a multi error, combine it into a single multi error
	// rather than wrapping it inside another one.
	case reorgErr != nil && finalErr != nil:
		var mErr MultiError
		if errors.As(reorgErr, &mErr) {
			combined := make([]error, 0, len(mErr)+1)
			combined = append(combined, finalErr)
			combined = append(combined, mErr...)
			finalErr = MultiError(combined)
		} else {
			finalErr = MultiError{finalErr, reorgErr}
		}
	}

	// Notify the caller about any blocks that are now linked and were accepted
	// to the block chain.  The caller would typically want to react by relaying
	// the inventory to other peers unless it was already relayed above via
	// NTNewTipBlockChecked.
	//
	// Note that this intentionally waits until after the chain reorganization
	// above so that the information is relative to the final best chain after
	// validation.
	newTip := b.bestChain.Tip()
	b.chainLock.Unlock()
	for _, n := range acceptedNodes {
		// Skip any blocks which either themselves failed validation or are
		// descenants of one that failed.
		if b.index.NodeStatus(n).KnownInvalid() {
			continue
		}

		var forkLen int64
		if fork := b.bestChain.FindFork(n); fork != nil {
			forkLen = n.height - fork.height
		}
		b.sendNotification(NTBlockAccepted, &BlockAcceptedNtfnsData{
			BestHeight: newTip.height,
			ForkLen:    forkLen,
			Block:      block,
		})
	}
	b.chainLock.Lock()

	var forkLen int64
	if finalErr == nil {
		if fork := b.bestChain.FindFork(node); fork != nil {
			forkLen = node.height - fork.height
		}
	}
	return forkLen, finalErr
}

// InvalidateBlock manually invalidates the provided block as if the block had
// violated a consensus rule and marks all of its descendants as having a known
// invalid ancestor.  It then reorganizes the chain as necessary so the branch
// with the most cumulative proof of work that is still valid becomes the main
// chain.
func (b *BlockChain) InvalidateBlock(hash *chainhash.Hash) error {
	b.processLock.Lock()
	defer b.processLock.Unlock()

	// Unable to invalidate a block that does not exist.
	node := b.index.LookupNode(hash)
	if node == nil {
		return unknownBlockError(hash)
	}

	// Disallow invalidation of the genesis block.
	if node.height == 0 {
		str := "invalidating the genesis block is not allowed"
		return contextError(ErrInvalidateGenesisBlock, str)
	}

	// Nothing to do if the block is already known to have failed validation.
	// Notice that this is intentionally not considering the case when the block
	// is marked invalid due to having a known invalid ancestor so the block is
	// still manually marked as having failed validation in that case.
	if b.index.NodeStatus(node).KnownValidateFailed() {
		return nil
	}

	// Simply mark the block being invalidated as having failed validation and
	// all of its descendants as having an invalid ancestor when it is not part
	// of the current best chain.
	b.recentContextChecks.Delete(node.hash)
	if !b.bestChain.Contains(node) {
		b.index.MarkBlockFailedValidation(node)
		b.chainLock.Lock()
		b.flushBlockIndexWarnOnly()
		b.chainLock.Unlock()
		return nil
	}

	log.Infof("Rolling the chain back to block %s (height %d) due to manual "+
		"invalidation", node.parent.hash, node.parent.height)

	// At this point, the invalidated block is part of the current best chain,
	// so start by reorganizing the chain back to its parent and marking it as
	// having failed validation along with all of its descendants as having an
	// invalid ancestor.
	b.chainLock.Lock()
	if err := b.reorganizeChain(node.parent); err != nil {
		b.flushBlockIndexWarnOnly()
		b.chainLock.Unlock()
		return err
	}
	b.index.MarkBlockFailedValidation(node)

	// Reset whether or not the chain believes it is current since the best
	// chain was just invalidated.
	newTip := b.bestChain.Tip()
	b.isCurrentLatch = false
	b.maybeUpdateIsCurrent(newTip)
	b.chainLock.Unlock()

	// The new best chain tip is probably no longer in the best chain candidates
	// since it was likely removed due to previously having less work, so scour
	// the block tree in order repopulate the best chain candidates.
	b.index.Lock()
	b.index.addBestChainCandidate(newTip)
	b.index.forEachChainTip(func(tip *blockNode) error {
		// Chain tips that have less work than the new tip are not best chain
		// candidates nor are any of their ancestors since they have even less
		// work.
		if tip.workSum.Cmp(newTip.workSum) < 0 {
			return nil
		}

		// Find the first ancestor of the tip that is not known to be invalid
		// and can be validated.  Then add it as a candidate to potentially
		// become the best chain tip if it has the same or more work than the
		// current one.
		n := tip
		for n != nil && (n.status.KnownInvalid() || !b.index.canValidate(n)) {
			n = n.parent
		}
		if n != nil && n != newTip && n.workSum.Cmp(newTip.workSum) >= 0 {
			b.index.addBestChainCandidate(n)
		}

		return nil
	})
	b.index.Unlock()

	// Find the current best chain candidate and attempt to reorganize the chain
	// to it.  The most common case is for the candidate to extend the current
	// best chain, however, it might also be a candidate that would cause a
	// reorg or be the current main chain tip, which will be the case when the
	// passed block is on a side chain.
	b.chainLock.Lock()
	targetTip := b.index.FindBestChainCandidate()
	err := b.reorganizeChain(targetTip)
	b.flushBlockIndexWarnOnly()
	b.chainLock.Unlock()
	return err
}

// blockNodeInSlice return whether a given block node is an element in a slice
// of them.
func blockNodeInSlice(node *blockNode, slice []*blockNode) bool {
	for _, child := range slice {
		if child == node {
			return true
		}
	}
	return false
}

// ReconsiderBlock removes the known invalid status of the provided block and
// all of its ancestors along with the known invalid ancestor status from all of
// its descendants that are neither themselves marked as having failed
// validation nor descendants of another such block.  Therefore, it allows the
// affected blocks to be reconsidered under the current consensus rules.  It
// then potentially reorganizes the chain as necessary so the block with the
// most cumulative proof of work that is valid becomes the tip of the main
// chain.
func (b *BlockChain) ReconsiderBlock(hash *chainhash.Hash) error {
	b.processLock.Lock()
	defer b.processLock.Unlock()

	// Unable to reconsider a block that does not exist.
	node := b.index.LookupNode(hash)
	if node == nil {
		return unknownBlockError(hash)
	}

	log.Infof("Reconsidering block %s (height %d)", node.hash, node.height)

	// Remove invalidity flags from the block to be reconsidered and all of its
	// ancestors while tracking the earliest such block that is marked as having
	// failed validation since all descendants of that block need to have their
	// invalid ancestor flag removed.
	//
	// Also, add any that are eligible for validation as candidates to
	// potentially become the best chain when they have the same or more work
	// than the current best chain tip and remove any cached validation-related
	// state for them to ensure they undergo full revalidation should it be
	// necessary.
	//
	// Finally, add any that are not already fully linked and have their data
	// available to the map of unlinked blocks that are eligible for connection
	// when they are not already present.
	curBestTip := b.bestChain.Tip()
	vfNode := node
	b.index.Lock()
	for n := node; n != nil && n.height > 0; n = n.parent {
		if n.status.KnownInvalid() {
			if n.status.KnownValidateFailed() {
				vfNode = n
			}
			b.index.unsetStatusFlags(n, statusValidateFailed|statusInvalidAncestor)
			b.recentContextChecks.Delete(n.hash)
		}

		if b.index.canValidate(n) && n.workSum.Cmp(curBestTip.workSum) >= 0 {
			b.index.addBestChainCandidate(n)
		}

		if !n.isFullyLinked && n.status.HaveData() && n.parent != nil {
			unlinked := b.index.unlinkedChildrenOf[n.parent]
			if !blockNodeInSlice(n, unlinked) {
				b.index.unlinkedChildrenOf[n.parent] = append(unlinked, n)
			}
		}
	}

	// Remove the known invalid ancestor flag from all blocks that descend from
	// the earliest failed block to be reconsidered that are neither themselves
	// marked as having failed validation nor descendants of another such block.
	//
	// Also, add any that are eligible for validation as candidates to
	// potentially become the best chain when they have the same or more work
	// than the current best chain tip and remove any cached validation-related
	// state for them to ensure they undergo full revalidation should it be
	// necessary.
	//
	// Finally, add any that are not already fully linked and have their data
	// available to the map of unlinked blocks that are eligible for connection
	// when they are not already present.
	//
	// Chain tips at the same or lower heights than the earliest failed block to
	// be reconsidered can't possibly be descendants of it, so use it as the
	// lower height bound filter when iterating chain tips.
	b.index.forEachChainTipAfterHeight(vfNode, func(tip *blockNode) error {
		// Nothing to do if the earliest failed block to be reconsidered is not
		// an ancestor of this chain tip.
		if !vfNode.IsAncestorOf(tip) {
			return nil
		}

		// Find the final descendant that is not known to descend from another
		// one that failed validation since all descendants after that point
		// need to retain their known invalid ancestor status.
		finalNotKnownInvalidDescendant := tip
		for n := tip; n != vfNode; n = n.parent {
			if n.status.KnownValidateFailed() {
				finalNotKnownInvalidDescendant = n.parent
			}
		}

		for n := finalNotKnownInvalidDescendant; n != vfNode; n = n.parent {
			b.index.unsetStatusFlags(n, statusInvalidAncestor)
			b.recentContextChecks.Delete(n.hash)
			if b.index.canValidate(n) && n.workSum.Cmp(curBestTip.workSum) >= 0 {
				b.index.addBestChainCandidate(n)
			}

			if !n.isFullyLinked && n.status.HaveData() && n.parent != nil {
				unlinked := b.index.unlinkedChildrenOf[n.parent]
				if !blockNodeInSlice(n, unlinked) {
					b.index.unlinkedChildrenOf[n.parent] = append(unlinked, n)
				}
			}
		}

		return nil
	})

	// Update the best known invalid block (as determined by having the most
	// cumulative work) and best header that is not known to be invalid as
	// needed.
	//
	// Note this is separate from the above iteration because all tips must be
	// considered as opposed to just those that are possible descendants of the
	// node being reconsidered.
	b.index.bestInvalid = nil
	b.index.forEachChainTip(func(tip *blockNode) error {
		if tip.status.KnownInvalid() {
			b.index.maybeUpdateBestInvalid(tip)
		}
		b.index.maybeUpdateBestHeaderForTip(tip)
		return nil
	})
	b.index.Unlock()

	// Reset whether or not the chain believes it is current, find the best
	// chain candidate, and attempt to reorganize the chain to it.
	b.chainLock.Lock()
	b.isCurrentLatch = false
	targetTip := b.index.FindBestChainCandidate()
	err := b.reorganizeChain(targetTip)
	b.flushBlockIndexWarnOnly()
	b.chainLock.Unlock()

	// Force pruning of the cached chain tips since it's fairly likely the best
	// tip has experienced a sudden change and is higher given how this function
	// is typically used and the logic which only periodically prunes tips is
	// optimized for steady state operation.
	b.index.Lock()
	b.index.pruneCachedTips(b.bestChain.Tip())
	b.index.Unlock()
	return err
}
