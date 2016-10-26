// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import "fmt"

func (b *BlockChain) calcNextStakeVersion(node *blockNode) (uint32, error) {

	if node == nil {
		return 0, AssertError(fmt.Sprintf(
			"calcNextStakeVersion: invalid node"))
	}

	// Check to see if the blockchain is deep enough.
	if node.height < b.chainParams.StakeDiffWindowSize+
		b.chainParams.StakeValidationHeight {

		return node.header.StakeVersion, nil
	}

	// Check if we have to calculate next version or return prior version.
	if (node.height-b.chainParams.StakeValidationHeight)%
		b.chainParams.StakeDiffWindowSize != 0 {

		return node.header.StakeVersion, nil
	}

	// Convert last StakeDiffWindowSize versions into version:count map.
	versions := make(map[uint32]uint32) // [version] count
	oldNode := node
	totalVotes := int64(0)
	for i := int64(0); i < b.chainParams.StakeDiffWindowSize; i++ {
		for _, v := range oldNode.voterVersions {
			versions[v] += 1
			totalVotes++
		}

		// Get the previous block node.
		var err error
		oldNode, err = b.getPrevNodeFromNode(oldNode)
		if err != nil {
			// Can't be hit but we keep test.
			return 0, err
		}

		// If we're at the genesis block, reset the oldNode
		// so that it stays at the genesis block.
		if oldNode == nil {
			// Can't be hit but we keep test.
			return node.header.StakeVersion, nil
		}
	}

	// Determine if we have a 95% majority.
	threshold := (totalVotes *
		b.chainParams.SuperMajorityMultiplier) /
		b.chainParams.SuperMajorityDivisor
	for version, count := range versions {
		if int64(count) >= threshold && version > node.header.StakeVersion {
			return version, nil
		}
	}

	// No new version found.
	return node.header.StakeVersion, nil
}

func (b *BlockChain) CalcNextStakeVersion() (uint32, error) {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	return b.calcNextStakeVersion(b.bestNode)
}
