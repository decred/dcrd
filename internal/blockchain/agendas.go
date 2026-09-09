// Copyright (c) 2017-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"math/bits"
	"strings"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
)

// deploymentInfo houses information about the state of a consensus rule change
// deployment.
type deploymentInfo struct {
	// version is the associated deployment version.
	version uint32

	// deployment houses the deployment parameters for the associated consensus
	// rule change vote.
	deployment *chaincfg.ConsensusDeployment

	// forcedState optionally specifies a threshold state to use instead of
	// determining the state via the normal means of tallying votes.  This only
	// applies when it is not nil and is only populated when the associated
	// chain parameters specify a forced choice.
	forcedState *ThresholdStateTuple

	// cache is used to efficiently keep track of the threshold state for the
	// deployment.
	//
	// It is protected by the chain state mutex.
	cache *thresholdStateCache
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

		// Ensure the bits for the choice are a subset of the mask.
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

// isAgendaActive attempts to determine whether or not an agenda is active
// for the block AFTER the given block node.
//
// Its goal is to consolidate the logic that is the same for all agendas which
// only have a single valid passing choice that makes the agenda active.
// Consequently, it is not suitable for agendas that have more than one possible
// passing winning choice.
//
// Note that valid agendas will always return false for the genesis block since
// agendas are never active for it.  The genesis block is determined by a nil
// previous node since it is the only block that has no predecessor.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isAgendaActive(prevNode *blockNode, deploymentID string) (bool, error) {
	deployment, ok := b.deploymentData[deploymentID]
	if !ok {
		str := fmt.Sprintf("deployment ID %s does not exist", deploymentID)
		return false, contextError(ErrUnknownDeploymentID, str)
	}

	// Agendas are never active for the genesis block.
	if prevNode == nil {
		return false, nil
	}

	// Determine the status by tallying votes.
	//
	// NOTE: The choice field of the return threshold state is intentionally not
	// examined here.  This assumes there is only one possible passing choice
	// that makes the agenda active.  Consequently, this function is not
	// suitable for agendas with more than one possible passing choice.
	state := b.deploymentState(prevNode, &deployment)
	return state.State == ThresholdActive, nil
}

// isActiveFn represents a function used to determine whether or not an agenda
// is active from the point of view of the passed block node.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// The function will only be invoked with block nodes which can be validated as
// determined by [blockIndex.CanValidate].
//
// The function will be invoked with the chain state lock held (for writes).
type isActiveFn func(prevNode *blockNode) (bool, error)

// isAgendaActiveByHash attempts to determine whether or not an agenda is active
// for the block AFTER the given block hash.
//
// The primary goal is to consolidate the logic that is the same for all agendas
// which only have a single valid choice when they are active.
//
// Note that it will always return false for the genesis block since agendas
// are never active for it.
//
// This function is safe for concurrent access and takes the chain state lock
// (for writes).
func (b *BlockChain) isAgendaActiveByHash(prevHash *chainhash.Hash, isActiveFn isActiveFn) (bool, error) {
	// Agendas are never active for the genesis block.
	if *prevHash == *zeroHash {
		return false, nil
	}

	prevNode := b.index.LookupNode(prevHash)
	if prevNode == nil || !b.index.CanValidate(prevNode) {
		return false, unknownBlockError(prevHash)
	}

	b.chainLock.Lock()
	isActive, err := isActiveFn(prevNode)
	b.chainLock.Unlock()
	return isActive, err
}

// isSDiffAlgoAgendaActive returns whether or not the stake difficulty algorithm
// agenda vote defined in DCP0001 has passed and is now active from the point of
// view of the passed block node.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isSDiffAlgoAgendaActive(prevNode *blockNode) (bool, error) {
	// Treat the agenda as active when voting is not enabled for the current
	// network.
	//
	// This ideally should be handled in a more general way.  It is retained in
	// this form for now to avoid changing the current semantics.
	const deploymentID = chaincfg.VoteIDSDiffAlgorithm
	if _, ok := b.deploymentData[deploymentID]; !ok {
		return true, nil
	}

	return b.isAgendaActive(prevNode, deploymentID)
}

// isLNFeaturesAgendaActive returns whether or not the LN features agenda vote,
// as defined in DCP0002 and DCP0003 has passed and is now active from the point
// of view of the passed block node.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isLNFeaturesAgendaActive(prevNode *blockNode) (bool, error) {
	const deploymentID = chaincfg.VoteIDLNFeatures
	return b.isAgendaActive(prevNode, deploymentID)
}

// IsLNFeaturesAgendaActive returns whether or not the LN features agenda vote,
// as defined in DCP0002 and DCP0003 has passed and is now active for the block
// AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsLNFeaturesAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	return b.isAgendaActiveByHash(prevHash, b.isLNFeaturesAgendaActive)
}

// isHeaderCommitmentsAgendaActive returns whether or not the header commitments
// agenda vote, as defined in DCP0005 has passed and is now active from the
// point of view of the passed block node.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isHeaderCommitmentsAgendaActive(prevNode *blockNode) (bool, error) {
	const deploymentID = chaincfg.VoteIDHeaderCommitments
	return b.isAgendaActive(prevNode, deploymentID)
}

// IsHeaderCommitmentsAgendaActive returns whether or not the header commitments
// agenda vote, as defined in DCP0005 has passed and is now active for the block
// AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsHeaderCommitmentsAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	return b.isAgendaActiveByHash(prevHash, b.isHeaderCommitmentsAgendaActive)
}

// isTreasuryAgendaActive returns whether or not the treasury agenda, as defined
// in DCP0006, has passed and is now active from the point of view of the passed
// block node.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isTreasuryAgendaActive(prevNode *blockNode) (bool, error) {
	// Ignore block 0 and 1 because they are special.
	if prevNode == nil || prevNode.height == 0 {
		return false, nil
	}

	const deploymentID = chaincfg.VoteIDTreasury
	return b.isAgendaActive(prevNode, deploymentID)
}

// IsTreasuryAgendaActive returns whether or not the treasury agenda vote, as
// defined in DCP0006, has passed and is now active for the block AFTER the
// given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsTreasuryAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	return b.isAgendaActiveByHash(prevHash, b.isTreasuryAgendaActive)
}

// isRevertTreasuryPolicyActive returns whether or not the revert treasury
// expenditure policy agenda, as defined in DCP0007, has passed and is now
// active from the point of view of the passed block node.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isRevertTreasuryPolicyActive(prevNode *blockNode) (bool, error) {
	const deploymentID = chaincfg.VoteIDRevertTreasuryPolicy
	return b.isAgendaActive(prevNode, deploymentID)
}

// IsRevertTreasuryPolicyActive returns whether or not the revert treasury
// expenditure policy agenda vote, as defined in DCP0007, has passed and is now
// active for the block AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsRevertTreasuryPolicyActive(prevHash *chainhash.Hash) (bool, error) {
	return b.isAgendaActiveByHash(prevHash, b.isRevertTreasuryPolicyActive)
}

// isExplicitVerUpgradesAgendaActive returns whether or not the explicit version
// upgrades agenda, as defined in DCP0008, has passed and is now active from the
// point of view of the passed block node.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isExplicitVerUpgradesAgendaActive(prevNode *blockNode) (bool, error) {
	const deploymentID = chaincfg.VoteIDExplicitVersionUpgrades
	return b.isAgendaActive(prevNode, deploymentID)
}

// IsExplicitVerUpgradesAgendaActive returns whether or not the explicit version
// upgrades agenda, as defined in DCP0008, has passed and is now active for the
// block AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsExplicitVerUpgradesAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	return b.isAgendaActiveByHash(prevHash, b.isExplicitVerUpgradesAgendaActive)
}

// isAutoRevocationsAgendaActive returns whether or not the automatic ticket
// revocations agenda, as defined in DCP0009, has passed and is now active from
// the point of view of the passed block node.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isAutoRevocationsAgendaActive(prevNode *blockNode) (bool, error) {
	const deploymentID = chaincfg.VoteIDAutoRevocations
	return b.isAgendaActive(prevNode, deploymentID)
}

// IsAutoRevocationsAgendaActive returns whether or not the automatic ticket
// revocations agenda vote, as defined in DCP0009, has passed and is now active
// for the block AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsAutoRevocationsAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	return b.isAgendaActiveByHash(prevHash, b.isAutoRevocationsAgendaActive)
}

// isSubsidySplitAgendaActive returns whether or not the agenda to change the
// block reward subsidy split to 10/80/10, as defined in DCP0010, has passed and
// is now active from the point of view of the passed block node.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isSubsidySplitAgendaActive(prevNode *blockNode) (bool, error) {
	const deploymentID = chaincfg.VoteIDChangeSubsidySplit
	return b.isAgendaActive(prevNode, deploymentID)
}

// IsSubsidySplitAgendaActive returns whether or not the agenda to change the
// block reward subsidy split to 10/80/10, as defined in DCP0010, has passed and
// is now active for the block AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsSubsidySplitAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	return b.isAgendaActiveByHash(prevHash, b.isSubsidySplitAgendaActive)
}

// isBlake3PowAgendaForcedActive returns whether or not the agenda to change the
// proof of work hash function to blake3, as defined in DCP0011, is forced
// active by the chain parameters.
//
// This function is safe for concurrent access.
func (b *BlockChain) isBlake3PowAgendaForcedActive() bool {
	const deploymentID = chaincfg.VoteIDBlake3Pow
	deployment, ok := b.deploymentData[deploymentID]
	if !ok {
		return false
	}

	state := deployment.forcedState
	return state != nil && state.State == ThresholdActive
}

// isBlake3PowAgendaActive returns whether or not the agenda to change the proof
// of work hash function to blake3, as defined in DCP0011, has passed and is now
// active from the point of view of the passed block node.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isBlake3PowAgendaActive(prevNode *blockNode) (bool, error) {
	const deploymentID = chaincfg.VoteIDBlake3Pow
	return b.isAgendaActive(prevNode, deploymentID)
}

// IsBlake3PowAgendaActive returns whether or not the agenda to change the proof
// of work hash function to blake3, as defined in DCP0011, has passed and is now
// active for the block AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsBlake3PowAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	return b.isAgendaActiveByHash(prevHash, b.isBlake3PowAgendaActive)
}

// isSubsidySplitR2AgendaActive returns whether or not the agenda to change the
// block reward subsidy split to 1/89/10, as defined in DCP0012, has passed and
// is now active from the point of view of the passed block node.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isSubsidySplitR2AgendaActive(prevNode *blockNode) (bool, error) {
	const deploymentID = chaincfg.VoteIDChangeSubsidySplitR2
	return b.isAgendaActive(prevNode, deploymentID)
}

// IsSubsidySplitR2AgendaActive returns whether or not the agenda to change the
// block reward subsidy split to 1/89/10, as defined in DCP0012, has passed and
// is now active for the block AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsSubsidySplitR2AgendaActive(prevHash *chainhash.Hash) (bool, error) {
	return b.isAgendaActiveByHash(prevHash, b.isSubsidySplitR2AgendaActive)
}

// isMaxTreasurySpendAgendaActive returns whether or not the agenda to change
// the maximum treasury spend to 4% per expenditure policy window, as defined in
// DCP0013, has passed and is now active from the point of view of the passed
// block node.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isMaxTreasurySpendAgendaActive(prevNode *blockNode) (bool, error) {
	const deploymentID = chaincfg.VoteIDMaxTreasurySpend
	return b.isAgendaActive(prevNode, deploymentID)
}

// IsMaxTreasurySpendAgendaActive returns whether or not the agenda to change
// the maximum treasury spend to 4% per expenditure policy window, as defined in
// DCP0013, has passed and is now active for the block AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsMaxTreasurySpendAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	return b.isAgendaActiveByHash(prevHash, b.isMaxTreasurySpendAgendaActive)
}
