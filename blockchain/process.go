// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"time"

	"github.com/decred/dcrd/dcrutil/v4"
)

// BehaviorFlags is a bitmask defining tweaks to the normal behavior when
// performing chain processing and consensus rules checks.
type BehaviorFlags uint32

const (
	// BFFastAdd may be set to indicate that several checks can be avoided
	// for the block since it is already known to fit into the chain due to
	// already proving it correct links into the chain up to a known
	// checkpoint.  This is primarily used for headers-first mode.
	BFFastAdd BehaviorFlags = 1 << iota

	// BFNoPoWCheck may be set to indicate the proof of work check which
	// ensures a block hashes to a value less than the required target will
	// not be performed.
	BFNoPoWCheck

	// BFNone is a convenience value to specifically indicate no flags.
	BFNone BehaviorFlags = 0
)

// ProcessBlock is the main workhorse for handling insertion of new blocks into
// the block chain.  It includes functionality such as rejecting duplicate
// blocks, ensuring blocks follow all rules, and insertion into the block chain
// along with best chain selection and reorganization.
//
// It is up to the caller to ensure the blocks are processed in order since
// orphans are rejected.
//
// When no errors occurred during processing, the first return value indicates
// the length of the fork the block extended.  In the case it either extended
// the best chain or is now the tip of the best chain due to causing a
// reorganize, the fork length will be 0.
//
// This function is safe for concurrent access.
func (b *BlockChain) ProcessBlock(block *dcrutil.Block, flags BehaviorFlags) (int64, error) {
	b.chainLock.Lock()
	defer b.chainLock.Unlock()

	blockHash := block.Hash()
	log.Tracef("Processing block %v", blockHash)
	currentTime := time.Now()
	defer func() {
		elapsedTime := time.Since(currentTime)
		log.Debugf("Block %v (height %v) finished processing in %s",
			blockHash, block.Height(), elapsedTime)
	}()

	// The block must not already exist in the main chain or side chains.
	if b.index.HaveBlock(blockHash) {
		str := fmt.Sprintf("already have block %v", blockHash)
		return 0, ruleError(ErrDuplicateBlock, str)
	}

	// Perform preliminary sanity checks on the block and its transactions.
	err := checkBlockSanity(block, b.timeSource, flags, b.chainParams)
	if err != nil {
		return 0, err
	}

	// This function should never be called with orphans or the genesis block.
	blockHeader := &block.MsgBlock().Header
	prevHash := &blockHeader.PrevBlock
	if !b.index.HaveBlock(prevHash) {
		// The fork length of orphans is unknown since they, by definition, do
		// not connect to the best chain.
		str := fmt.Sprintf("previous block %s is not known", prevHash)
		return 0, ruleError(ErrMissingParent, str)
	}

	// The block has passed all context independent checks and appears sane
	// enough to potentially accept it into the block chain.
	forkLen, err := b.maybeAcceptBlock(block, flags)
	if err != nil {
		return 0, err
	}

	log.Debugf("Accepted block %v", blockHash)

	return forkLen, nil
}
