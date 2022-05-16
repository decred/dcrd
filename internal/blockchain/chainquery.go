// Copyright (c) 2018-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"math"
	"sort"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// nodeHeightSorter implements sort.Interface to allow a slice of nodes to
// be sorted by height in ascending order.
type nodeHeightSorter []*blockNode

// Len returns the number of nodes in the slice.  It is part of the
// sort.Interface implementation.
func (s nodeHeightSorter) Len() int {
	return len(s)
}

// Swap swaps the nodes at the passed indices.  It is part of the
// sort.Interface implementation.
func (s nodeHeightSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less returns whether the node with index i should sort before the node with
// index j.  It is part of the sort.Interface implementation.
func (s nodeHeightSorter) Less(i, j int) bool {
	// To ensure stable order when the heights are the same, fall back to
	// sorting based on hash.
	if s[i].height == s[j].height {
		return bytes.Compare(s[i].hash[:], s[j].hash[:]) < 0
	}
	return s[i].height < s[j].height
}

// ChainTipInfo models information about a chain tip.
type ChainTipInfo struct {
	// Height specifies the block height of the chain tip.
	Height int64

	// Hash specifies the block hash of the chain tip.
	Hash chainhash.Hash

	// BranchLen specifies the length of the branch that connects the chain tip
	// to the main chain.  It will be zero for the main chain tip.
	BranchLen int64

	// Status specifies the validation status of chain formed by the chain tip.
	//
	// active:
	//   The current best chain tip.
	//
	// invalid:
	//   The block or one of its ancestors is invalid.
	//
	// headers-only:
	//   The block or one of its ancestors does not have the full block data
	//   available which also means the block can't be validated or connected.
	//
	// valid-fork:
	//   The block is fully validated which implies it was probably part of the
	//   main chain at one point and was reorganized.
	//
	// valid-headers:
	//   The full block data is available and the header is valid, but the block
	//   was never validated which implies it was probably never part of the
	//   main chain.
	Status string
}

// ChainTips returns information, in JSON-RPC format, about all of the currently
// known chain tips in the block index.
//
// This function is safe for concurrent access.
func (b *BlockChain) ChainTips() []ChainTipInfo {
	b.index.RLock()
	chainTips := make([]*blockNode, 0, b.index.totalTips)
	for _, entry := range b.index.chainTips {
		chainTips = append(chainTips, entry.tip)
		if len(entry.otherTips) > 0 {
			chainTips = append(chainTips, entry.otherTips...)
		}
	}
	b.index.RUnlock()

	// Generate the results sorted by descending height.
	sort.Sort(sort.Reverse(nodeHeightSorter(chainTips)))
	results := make([]ChainTipInfo, len(chainTips))
	bestTip := b.bestChain.Tip()
	for i, tip := range chainTips {
		result := &results[i]
		result.Height = tip.height
		result.Hash = tip.hash
		result.BranchLen = tip.height - b.bestChain.FindFork(tip).height

		// Determine the status of the chain tip.
		//
		// active:
		//   The current best chain tip.
		//
		// invalid:
		//   The block or one of its ancestors is invalid.
		//
		// headers-only:
		//   The block or one of its ancestors does not have the full block data
		//   available which also means the block can't be validated or
		//   connected.
		//
		// valid-fork:
		//   The block is fully validated which implies it was probably part of
		//   main chain at one point and was reorganized.
		//
		// valid-headers:
		//   The full block data is available and the header is valid, but the
		//   block was never validated which implies it was probably never part
		//   of the main chain.
		tipStatus := b.index.NodeStatus(tip)
		if tip == bestTip {
			result.Status = "active"
		} else if tipStatus.KnownInvalid() {
			result.Status = "invalid"
		} else if !tipStatus.HaveData() {
			result.Status = "headers-only"
		} else if tipStatus.HasValidated() {
			result.Status = "valid-fork"
		} else {
			result.Status = "valid-headers"
		}
	}
	return results
}

// BestHeader returns the header with the most cumulative work that is NOT
// known to be invalid.
//
// This function is safe for concurrent access.
func (b *BlockChain) BestHeader() (chainhash.Hash, int64) {
	header := b.index.BestHeader()
	return header.hash, header.height
}

// BestInvalidHeader returns the header with the most cumulative work that is
// known to be invalid.  It will be a hash of all zeroes if there is no such
// header.
//
// This function is safe for concurrent access.
func (b *BlockChain) BestInvalidHeader() chainhash.Hash {
	var hash chainhash.Hash
	b.index.RLock()
	if b.index.bestInvalid != nil {
		hash = b.index.bestInvalid.hash
	}
	b.index.RUnlock()
	return hash
}

// PutNextNeededBlocks populates the provided slice with hashes for the next
// blocks after the current best chain tip that are needed to make progress
// towards the current best known header skipping any blocks that already have
// their data available.
//
// The provided slice will be populated with either as many hashes as it will
// fit per its length or as many hashes it takes to reach best header, whichever
// is smaller.
//
// It returns a sub slice of the provided one with its bounds adjusted to the
// number of entries populated.
//
// This function is safe for concurrent access.
func (b *BlockChain) PutNextNeededBlocks(out []chainhash.Hash) []chainhash.Hash {
	// Nothing to do when no results are requested.
	maxResults := len(out)
	if maxResults == 0 {
		return out[:0]
	}

	b.index.RLock()
	defer b.index.RUnlock()

	// Populate the provided slice by making use of a sliding window.  Note that
	// the needed block hashes are populated in forwards order while it is
	// necessary to walk the block index backwards to determine them.  Further,
	// an unknown number of blocks may already have their data and need to be
	// skipped, so it's not possible to determine the precise height after the
	// fork point to start iterating from.  Using a sliding window efficiently
	// handles these conditions without needing additional allocations.
	//
	// The strategy is to initially determine the common ancestor between the
	// current best chain tip and the current best known header as the starting
	// fork point and move the fork point forward by the window size after
	// populating the output slice with all relevant nodes in the window until
	// either there are no more results or the desired number of results have
	// been populated.
	const windowSize = 32
	var outputIdx int
	var window [windowSize]chainhash.Hash
	bestHeader := b.index.bestHeader
	fork := b.bestChain.FindFork(bestHeader)
	for outputIdx < maxResults && fork != nil && fork != bestHeader {
		// Determine the final descendant block on the branch that leads to the
		// best known header in this window by clamping the number of
		// descendants to consider to the window size.
		endNode := bestHeader
		numBlocksToConsider := endNode.height - fork.height
		if numBlocksToConsider > windowSize {
			endNode = endNode.Ancestor(fork.height + windowSize)
		}

		// Populate the blocks in this window from back to front by walking
		// backwards from the final block to consider in the window to the first
		// one excluding any blocks that already have their data available.
		windowIdx := windowSize
		for node := endNode; node != nil && node != fork; node = node.parent {
			if node.status.HaveData() {
				continue
			}

			windowIdx--
			window[windowIdx] = node.hash
		}

		// Populate the outputs with as many from the back of the window as
		// possible (since the window might not have been fully populated due to
		// skipped blocks) and move the output index forward to match.
		outputIdx += copy(out[outputIdx:], window[windowIdx:])

		// Move the fork point forward to the final block of the window.
		fork = endNode
	}

	return out[:outputIdx]
}

// VerifyProgress returns a percentage that is a guess of the progress of the
// chain verification process.
//
// This function is safe for concurrent access.
func (b *BlockChain) VerifyProgress() float64 {
	bestHdr := b.index.BestHeader()
	if bestHdr.height == 0 {
		return 0.0
	}

	tip := b.bestChain.Tip()
	return math.Min(float64(tip.height)/float64(bestHdr.height), 1.0) * 100
}

// IsKnownInvalidBlock returns whether either the provided block is itself known
// to be invalid or to have an invalid ancestor.  A return value of false in no
// way implies the block is valid or only has valid ancestors.  Thus, this will
// return false for invalid blocks that have not been proven invalid yet as well
// as return false for blocks with invalid ancestors that have not been proven
// invalid yet.
//
// It will also return false when the provided block is unknown.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsKnownInvalidBlock(hash *chainhash.Hash) bool {
	node := b.index.LookupNode(hash)
	if node == nil {
		return false
	}

	return b.index.NodeStatus(node).KnownInvalid()
}
