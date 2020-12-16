// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
)

// CheckpointConfirmations is the number of blocks before the end of the current
// best block chain that a good checkpoint candidate must be.
const CheckpointConfirmations = 4096

// Checkpoints returns a slice of checkpoints (regardless of whether they are
// already known).  When checkpoints are disabled or there are no checkpoints
// for the active network, it will return nil.
//
// This function is safe for concurrent access.
func (b *BlockChain) Checkpoints() []chaincfg.Checkpoint {
	if len(b.checkpoints) == 0 {
		return nil
	}

	return b.checkpoints
}

// LatestCheckpoint returns the most recent checkpoint (regardless of whether it
// is already known).  When checkpoints are disabled or there are no checkpoints
// for the active network, it will return nil.
//
// This function is safe for concurrent access.
func (b *BlockChain) LatestCheckpoint() *chaincfg.Checkpoint {
	if len(b.checkpoints) == 0 {
		return nil
	}

	return &b.checkpoints[len(b.checkpoints)-1]
}

// verifyCheckpoint returns whether the passed block height and hash combination
// match the hard-coded checkpoint data.  It also returns true if there is no
// checkpoint data for the passed block height.
//
// This function MUST be called with the chain lock held (for reads).
func (b *BlockChain) verifyCheckpoint(height int64, hash *chainhash.Hash) bool {
	if len(b.checkpoints) == 0 {
		return true
	}

	// Nothing to check if there is no checkpoint data for the block height.
	checkpoint, exists := b.checkpointsByHeight[height]
	if !exists {
		return true
	}

	if *checkpoint.Hash != *hash {
		return false
	}

	log.Infof("Verified checkpoint at height %d/block %s", checkpoint.Height,
		checkpoint.Hash)
	return true
}

// maybeUpdateMostRecentCheckpoint potentially updates the most recently known
// checkpoint to the provided block node.
func (b *BlockChain) maybeUpdateMostRecentCheckpoint(node *blockNode) {
	if len(b.checkpoints) == 0 {
		return
	}

	// Nothing to update if there is no checkpoint data for the block height or
	// the checkpoint hash does not match.
	checkpoint, exists := b.checkpointsByHeight[node.height]
	if !exists || node.hash != *checkpoint.Hash {
		return
	}

	// Update the previous checkpoint to current block so long as it is more
	// recent than the existing previous checkpoint.
	if b.checkpointNode == nil || b.checkpointNode.height < checkpoint.Height {
		log.Debugf("Most recent checkpoint updated to %s (height %d)",
			node.hash, node.height)
		b.checkpointNode = node
	}
}

// isNonstandardTransaction determines whether a transaction contains any
// scripts which are not one of the standard types.
func isNonstandardTransaction(tx *dcrutil.Tx) bool {
	// Check all of the output public key scripts for non-standard scripts.
	for _, txOut := range tx.MsgTx().TxOut {
		// It is ok to hard code isTreasuryEnabled to true here since
		// there is no way the chain is going to have treasury
		// transactions in it prior to activation since they are stake
		// transactions and would be rejected as unknown types.
		scriptClass := txscript.GetScriptClass(txOut.Version,
			txOut.PkScript, true)
		if scriptClass == txscript.NonStandardTy {
			return true
		}
	}
	return false
}

// IsCheckpointCandidate returns whether or not the passed block is a good
// checkpoint candidate.
//
// The factors used to determine a good checkpoint are:
//  - The block must be in the main chain
//  - The block must be at least 'CheckpointConfirmations' blocks prior to the
//    current end of the main chain
//  - The timestamps for the blocks before and after the checkpoint must have
//    timestamps which are also before and after the checkpoint, respectively
//    (due to the median time allowance this is not always the case)
//  - The block must not contain any strange transaction such as those with
//    nonstandard scripts
//
// The intent is that candidates are reviewed by a developer to make the final
// decision and then manually added to the list of checkpoints for a network.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsCheckpointCandidate(block *dcrutil.Block) (bool, error) {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	// A checkpoint must be in the main chain.
	node := b.index.LookupNode(block.Hash())
	if node == nil || !b.bestChain.Contains(node) {
		return false, nil
	}

	// Ensure the height of the passed block and the entry for the block in
	// the main chain match.  This should always be the case unless the
	// caller provided an invalid block.
	if node.height != block.Height() {
		return false, fmt.Errorf("passed block height of %d does not "+
			"match the main chain height of %d", block.Height(),
			node.height)
	}

	// A checkpoint must be at least CheckpointConfirmations blocks before
	// the end of the main chain.
	if node.height > (b.bestChain.Tip().height - CheckpointConfirmations) {
		return false, nil
	}

	// A checkpoint must be have at least one block after it.
	//
	// This should always succeed since the check above already made sure it
	// is CheckpointConfirmations back, but be safe in case the constant
	// changes.
	nextNode := b.bestChain.Next(node)
	if nextNode == nil {
		return false, nil
	}

	// A checkpoint must be have at least one block before it.
	if node.parent == nil {
		return false, nil
	}

	// A checkpoint must have timestamps for the block and the blocks on
	// either side of it in order (due to the median time allowance this is
	// not always the case).
	prevTime := time.Unix(node.parent.timestamp, 0)
	curTime := block.MsgBlock().Header.Timestamp
	nextTime := time.Unix(nextNode.timestamp, 0)
	if prevTime.After(curTime) || nextTime.Before(curTime) {
		return false, nil
	}

	// A checkpoint must have transactions that only contain standard
	// scripts.
	for _, tx := range block.Transactions() {
		if isNonstandardTransaction(tx) {
			return false, nil
		}
	}

	// All of the checks passed, so the block is a candidate.
	return true, nil
}
