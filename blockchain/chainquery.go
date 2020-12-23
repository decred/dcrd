// Copyright (c) 2018-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
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
func (b *BlockChain) BestHeader() (chainhash.Hash, int64) {
	b.index.RLock()
	header := b.index.bestHeader
	b.index.RUnlock()
	return header.hash, header.height
}

// BestInvalidHeader returns the header with the most cumulative work that is
// known to be invalid.  It will be a hash of all zeroes if there is no such
// header.
func (b *BlockChain) BestInvalidHeader() chainhash.Hash {
	var hash chainhash.Hash
	b.index.RLock()
	if b.index.bestInvalid != nil {
		hash = b.index.bestInvalid.hash
	}
	b.index.RUnlock()
	return hash
}
