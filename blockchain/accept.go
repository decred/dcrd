// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"

	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/dcrutil/v3"
)

// maybeAcceptBlock potentially accepts a block into the block chain and, if
// accepted, returns the length of the fork the block extended.  It performs
// several validation checks which depend on its position within the block chain
// before adding it.  The block is expected to have already gone through
// ProcessBlock before calling this function with it.  In the case the block
// extends the best chain or is now the tip of the best chain due to causing a
// reorganize, the fork length will be 0.
//
// The flags are also passed to checkBlockPositional, checkBlockContext and
// connectBestChain.  See their documentation for how the flags modify their
// behavior.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) maybeAcceptBlock(block *dcrutil.Block, flags BehaviorFlags) (int64, error) {
	// This function should never be called with orphan blocks or the
	// genesis block.
	prevHash := &block.MsgBlock().Header.PrevBlock
	prevNode := b.index.LookupNode(prevHash)
	if prevNode == nil {
		str := fmt.Sprintf("previous block %s is not known", prevHash)
		return 0, ruleError(ErrMissingParent, str)
	}

	// There is no need to validate the block if an ancestor is already
	// known to be invalid.
	if b.index.NodeStatus(prevNode).KnownInvalid() {
		str := fmt.Sprintf("previous block %s is known to be invalid",
			prevHash)
		return 0, ruleError(ErrInvalidAncestorBlock, str)
	}

	// The block must pass all of the validation rules which depend on having
	// the headers of all ancestors available, but do not rely on having the
	// full block data of all ancestors available.
	err := b.checkBlockPositional(block, prevNode, flags)
	if err != nil {
		return 0, err
	}

	// The block must pass all of the validation rules which depend on having
	// the full block data for all of its ancestors available.
	err = b.checkBlockContext(block, prevNode, flags)
	if err != nil {
		return 0, err
	}

	// Prune stake nodes which are no longer needed before creating a new
	// node.
	b.pruner.pruneChainIfNeeded()

	// Insert the block into the database if it's not already there.  Even
	// though it is possible the block will ultimately fail to connect, it
	// has already passed all proof-of-work and validity tests which means
	// it would be prohibitively expensive for an attacker to fill up the
	// disk with a bunch of blocks that fail to connect.  This is necessary
	// since it allows block download to be decoupled from the much more
	// expensive connection logic.  It also has some other nice properties
	// such as making blocks that never become part of the main chain or
	// blocks that fail to connect available for further analysis.
	err = b.db.Update(func(dbTx database.Tx) error {
		return dbMaybeStoreBlock(dbTx, block)
	})
	if err != nil {
		return 0, err
	}

	// Create a new block node for the block and add it to the block index.
	// The block could either be on a side chain or the main chain, but it
	// starts off as a side chain regardless.
	blockHeader := &block.MsgBlock().Header
	newNode := newBlockNode(blockHeader, prevNode)
	newNode.populateTicketInfo(stake.FindSpentTicketsInBlock(block.MsgBlock()))
	newNode.status = statusDataStored
	b.index.AddNode(newNode)

	// Ensure the new block index entry is written to the database.
	err = b.flushBlockIndex()
	if err != nil {
		return 0, err
	}

	// Notify the caller when the block intends to extend the main chain,
	// the chain believes it is current, and the block has passed all of the
	// sanity and contextual checks, such as having valid proof of work,
	// valid merkle and stake roots, and only containing allowed votes and
	// revocations.
	//
	// This allows the block to be relayed before doing the more expensive
	// connection checks, because even though the block might still fail
	// to connect and becomes the new main chain tip, that is quite rare in
	// practice since a lot of work was expended to create a block that
	// satisfies the proof of work requirement.
	//
	// Notice that the chain lock is not released before sending the
	// notification.  This is intentional and must not be changed without
	// understanding why!
	if b.isCurrent() && b.bestChain.Tip() == prevNode {
		b.sendNotification(NTNewTipBlockChecked, block)
	}

	// Fetching a stake node could enable a new DoS vector, so restrict
	// this only to blocks that are recent in history.
	if newNode.height < b.bestChain.Tip().height-minMemoryNodes {
		newNode.stakeNode, err = b.fetchStakeNode(newNode)
		if err != nil {
			return 0, err
		}
	}

	// Grab the parent block since it is required throughout the block
	// connection process.
	parent, err := b.fetchBlockByNode(newNode.parent)
	if err != nil {
		return 0, err
	}

	// Connect the passed block to the chain while respecting proper chain
	// selection according to the chain with the most proof of work.  This
	// also handles validation of the transaction scripts.
	forkLen, err := b.connectBestChain(newNode, block, parent, flags)
	if err != nil {
		return 0, err
	}

	// Potentially update the most recently known checkpoint to this block.
	b.maybeUpdateMostRecentCheckpoint(newNode)

	// Notify the caller that the new block was accepted into the block
	// chain.  The caller would typically want to react by relaying the
	// inventory to other peers unless it was already relayed above
	// via NTNewTipBlockChecked.
	bestHeight := b.bestChain.Tip().height
	b.chainLock.Unlock()
	b.sendNotification(NTBlockAccepted, &BlockAcceptedNtfnsData{
		BestHeight: bestHeight,
		ForkLen:    forkLen,
		Block:      block,
	})
	b.chainLock.Lock()

	return forkLen, nil
}
