// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"errors"
	"fmt"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

var (
	errStakeVersionHeight           = errors.New("not enough height")
	errVoterVersionMajorityNotFound = errors.New("voter version majority " +
		"not found")
	errStakeVersionMajorityNotFound = errors.New("stake version majority " +
		"not found")
)

// calcWantHeight calculates the height of the final block of the previous
// interval.  The adjusted height accounts for the fact the starting validation
// height does not necessarily start on an interval and thus the intervals
// might not be zero-based.
func calcWantHeight(skip, interval, height int64) int64 {
	intervalOffset := skip % interval
	adjustedHeight := height - intervalOffset - 1
	return (adjustedHeight - ((adjustedHeight + 1) % interval)) +
		intervalOffset
}

// findStakeVersionPriorNode walks the chain backwards from prevNode until it
// finds the appropriate interval block to return to the caller.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) findStakeVersionPriorNode(prevNode *blockNode) (*blockNode, error) {
	// Check to see if the blockchain is high enough to begin accounting
	// stake versions.
	nextHeight := prevNode.height + 1
	if nextHeight < b.chainParams.StakeValidationHeight+
		b.chainParams.StakeVersionInterval {
		return nil, errStakeVersionHeight
	}

	wantHeight := calcWantHeight(b.chainParams.StakeValidationHeight,
		b.chainParams.StakeVersionInterval, nextHeight)

	// Walk backwards until we find an interval block and make sure we
	// don't blow through the minimum height.
	iterNode := prevNode
	for iterNode.height > wantHeight {
		var err error
		iterNode, err = b.getPrevNodeFromNode(iterNode)
		if err != nil {
			return nil, err
		}
	}

	return iterNode, nil
}

// isVoterMajorityVersion determines if minVer requirement is met based on
// prevNode.  The function always uses the voter versions of the prior window.
// For example, if StakeVersionInterval = 11 and StakeValidationHeight = 13 the
// windows start at 13 + (11 * 2) 25 and are as follows: 24-34, 35-45, 46-56 ...
// If height comes in at 35 we use the 24-34 window, up to height 45.
// If height comes in at 46 we use the 35-45 window, up to height 56 etc.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isVoterMajorityVersion(minVer uint32, prevNode *blockNode) bool {
	// Walk blockchain backwards to calculate version.
	node, err := b.findStakeVersionPriorNode(prevNode)
	if err != nil {
		if err == errStakeVersionHeight {
			return 0 >= minVer
		}
		return false
	}

	// Calculate version.
	totalVotesFound := int32(0)
	versionCount := int32(0)
	iterNode := node
	for i := int64(0); i < b.chainParams.StakeVersionInterval && iterNode != nil; i++ {
		totalVotesFound += int32(len(iterNode.voterVersions))
		for _, version := range iterNode.voterVersions {
			if version >= minVer {
				versionCount += 1
			}
		}

		var err error
		iterNode, err = b.getPrevNodeFromNode(iterNode)
		if err != nil {
			return false
		}
	}

	// Ensure we have enoguh votes.
	if int64(totalVotesFound) < b.chainParams.StakeVersionInterval*
		int64(b.chainParams.TicketsPerBlock-2) {
		return false
	}

	// Determine the required amount of votes to reach supermajority.
	numRequired := totalVotesFound * b.chainParams.StakeMajorityMultiplier /
		b.chainParams.StakeMajorityDivisor

	return versionCount >= numRequired
}

// isStakeMajorityVersion determines if minVer requirement is met based on
// prevNode.  The function always uses the stake versions of the prior window.
// For example, if StakeVersionInterval = 11 and StakeValidationHeight = 13 the
// windows start at 13 + (11 * 2) 25 and are as follows: 24-34, 35-45, 46-56 ...
// If height comes in at 35 we use the 24-34 window, up to height 45.
// If height comes in at 46 we use the 35-45 window, up to height 56 etc.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isStakeMajorityVersion(minVer uint32, prevNode *blockNode) bool {
	// Walk blockchain backwards to calculate version.
	node, err := b.findStakeVersionPriorNode(prevNode)
	if err != nil {
		if err == errStakeVersionHeight {
			return 0 >= minVer
		}
		return false
	}

	// Calculate version.
	versionCount := int32(0)
	iterNode := node
	for i := int64(0); i < b.chainParams.StakeVersionInterval && iterNode != nil; i++ {
		if iterNode.header.StakeVersion >= minVer {
			versionCount += 1
		}

		var err error
		iterNode, err = b.getPrevNodeFromNode(iterNode)
		if err != nil {
			return false
		}
	}

	// Determine the required amount of votes to reach supermajority.
	numRequired := int32(b.chainParams.StakeVersionInterval) *
		b.chainParams.StakeMajorityMultiplier /
		b.chainParams.StakeMajorityDivisor

	return versionCount >= numRequired
}

// calcPriorStakeVersion calculates the header stake version of the prior
// interval.  The function walks the chain backwards by one interval and then
// it performs a standard majority calculation.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) calcPriorStakeVersion(prevNode *blockNode) (uint32, error) {
	// Walk blockchain backwards to calculate version.
	node, err := b.findStakeVersionPriorNode(prevNode)
	if err != nil {
		if err == errStakeVersionHeight {
			return 0, nil
		}
		return 0, err
	}

	// Calculate version.
	versions := make(map[uint32]int32) // [version][count]
	iterNode := node
	for i := int64(0); i < b.chainParams.StakeVersionInterval && iterNode != nil; i++ {
		versions[iterNode.header.StakeVersion]++

		var err error
		iterNode, err = b.getPrevNodeFromNode(iterNode)
		if err != nil {
			return 0, err
		}
	}

	// Determine the required amount of votes to reach supermajority.
	numRequired := int32(b.chainParams.StakeVersionInterval) *
		b.chainParams.StakeMajorityMultiplier /
		b.chainParams.StakeMajorityDivisor

	for version, count := range versions {
		if count >= numRequired {
			return version, nil
		}
	}

	return 0, errStakeVersionMajorityNotFound
}

// calcVoterVersionInterval tallies all voter versions in an interval and
// returns a version that has reached 75% majority.  This function assumes that
// prevNode is at a valid StakeVersionInterval.  It does not test for this and
// if prevNode is not sitting on a vlaid StakeVersionInterval it'll walk the
// cghain backwards and find the next valid interval.
// This function is really meant to be called internally only from this file.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) calcVoterVersionInterval(prevNode *blockNode) (uint32, error) {
	// Note that we are NOT checking if we are on interval!
	var err error

	versions := make(map[uint32]int32) // [version][count]
	totalVotesFound := int32(0)
	iterNode := prevNode
	for i := int64(0); i < b.chainParams.StakeVersionInterval && iterNode != nil; i++ {
		totalVotesFound += int32(len(iterNode.voterVersions))
		for _, version := range iterNode.voterVersions {
			versions[version]++
		}

		iterNode, err = b.getPrevNodeFromNode(iterNode)
		if err != nil {
			return 0, err
		}
	}

	// Testing purposes only.
	if int64(totalVotesFound) < b.chainParams.StakeVersionInterval*
		int64(b.chainParams.TicketsPerBlock-2) {
		return 0, AssertError(fmt.Sprintf("Not enough "+
			"votes: %v expected: %v ", totalVotesFound,
			b.chainParams.StakeVersionInterval*
				int64(b.chainParams.TicketsPerBlock-2)))
	}

	// Determine the required amount of votes to reach supermajority.
	numRequired := totalVotesFound * b.chainParams.StakeMajorityMultiplier /
		b.chainParams.StakeMajorityDivisor

	for version, count := range versions {
		if count >= numRequired {
			return version, nil
		}
	}

	return 0, errVoterVersionMajorityNotFound
}

// calcVoterVersion calculates the last prior valid majority stake version.  If
// the current interval does not have a majority stake version it'll go back to
// the prior interval.  It'll keep going back up to the minimum height at which
// point we know the version was 0.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) calcVoterVersion(prevNode *blockNode) (uint32, *blockNode) {
	// Walk blockchain backwards to find interval.
	node, err := b.findStakeVersionPriorNode(prevNode)
	if err != nil {
		return 0, nil
	}

	// Iterate over versions until we find a majority.
	iterNode := node
	for iterNode != nil {
		version, err := b.calcVoterVersionInterval(iterNode)
		if err == nil {
			return version, iterNode
		}
		if err != errVoterVersionMajorityNotFound {
			break
		}

		// findStakeVersionPriorNode increases the height so we need to
		// compensate by loading the prior node.
		iterNode, err = b.getPrevNodeFromNode(iterNode)
		if err != nil {
			break
		}

		// Walk blockchain back to prior interval.
		iterNode, err = b.findStakeVersionPriorNode(iterNode)
		if err != nil {
			break
		}
	}

	// We didn't find a marority version.
	return 0, nil
}

// calcStakeVersion calculates the header stake version based on voter
// versions.  If there is a majority of voter versions it uses the header stake
// version to prevent reverting to a prior version.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) calcStakeVersion(prevNode *blockNode) uint32 {
	version, node := b.calcVoterVersion(prevNode)
	if version == 0 || node == nil {
		// short circuit
		return 0
	}

	// Walk chain backwards to start of node interval (start of current
	// period) Note that calcWantHeight returns the LAST height of the
	// prior interval; hence the + 1.
	startIntervalHeight := calcWantHeight(b.chainParams.StakeValidationHeight,
		b.chainParams.StakeVersionInterval, node.height) + 1
	iterNode := node
	for iterNode.height > startIntervalHeight {
		var err error
		iterNode, err = b.getPrevNodeFromNode(iterNode)
		if err != nil || iterNode == nil {
			return 0
		}
	}

	// See if we are enforcing V3 blocks yet.  Just return V0 since it it
	// wasn't enforced and therefore irrelevant.
	if !b.isMajorityVersion(3, iterNode,
		b.chainParams.BlockRejectNumRequired) {
		return 0
	}

	if b.isStakeMajorityVersion(version, node) {
		priorVersion, _ := b.calcPriorStakeVersion(node)
		if version > priorVersion {
			return version
		} else {
			return priorVersion
		}
	}

	return version
}

// calcStakeVersionByHash calculates the last prior valid majority stake
// version.  If the current interval does not have a majority stake version
// it'll go back to the prior interval.  It'll keep going back up to the
// minimum height at which point we know the version was 0.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) calcStakeVersionByHash(hash *chainhash.Hash) (uint32, error) {
	prevNode, err := b.findNode(hash, 0)
	if err != nil {
		return 0, err
	}

	return b.calcStakeVersionByNode(prevNode)
}

// calcStakeVersionByNode is identical to calcStakeVersionByHash but takes a
// *blockNode instead.
func (b *BlockChain) calcStakeVersionByNode(prevNode *blockNode) (uint32, error) {
	return b.calcStakeVersion(prevNode), nil
}

// CalcStakeVersionByHash calls calcStakeVersionByHash with proper locking set.
func (b *BlockChain) CalcStakeVersionByHash(hash *chainhash.Hash) (uint32, error) {
	b.chainLock.Lock()
	defer b.chainLock.Unlock()

	return b.calcStakeVersionByHash(hash)
}
