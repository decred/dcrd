// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"math/bits"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
)

// ThresholdState define the various threshold states used when voting on
// consensus changes.
type ThresholdState byte

// These constants are used to identify specific threshold states.
const (
	// ThresholdInvalid is an invalid state and exists for use as the zero value
	// in error paths.
	ThresholdInvalid ThresholdState = iota

	// ThresholdDefined is the initial state for each deployment and is the
	// state for the genesis block has by definition for all deployments.
	ThresholdDefined

	// ThresholdStarted is the state for a deployment once its start time has
	// been reached.
	ThresholdStarted

	// ThresholdLockedIn is the state for a deployment during the retarget
	// period which is after the ThresholdStarted state period and the number of
	// blocks that have voted for the deployment equal or exceed the required
	// number of votes for the deployment.
	ThresholdLockedIn

	// ThresholdActive is the state for a deployment for all blocks after a
	// retarget period in which the deployment was in the ThresholdLockedIn
	// state.
	ThresholdActive

	// ThresholdFailed is the state for a deployment once its expiration time
	// has been reached and it did not reach the ThresholdLockedIn state.
	ThresholdFailed
)

// thresholdStateStrings is a map of ThresholdState values back to their
// constant names for pretty printing.
var thresholdStateStrings = map[ThresholdState]string{
	ThresholdInvalid:  "ThresholdInvalid",
	ThresholdDefined:  "ThresholdDefined",
	ThresholdStarted:  "ThresholdStarted",
	ThresholdLockedIn: "ThresholdLockedIn",
	ThresholdActive:   "ThresholdActive",
	ThresholdFailed:   "ThresholdFailed",
}

// String returns the ThresholdState as a human-readable name.
func (t ThresholdState) String() string {
	if s := thresholdStateStrings[t]; s != "" {
		return s
	}
	return fmt.Sprintf("Unknown ThresholdState (%d)", int(t))
}

// ThresholdStateTuple contains the current state and the activated choice,
// when valid.
type ThresholdStateTuple struct {
	// State contains the current ThresholdState.
	State ThresholdState

	// Choice is the specific choice that received the majority vote for the
	// ThresholdLockedIn and ThresholdActive states.
	//
	// IMPORTANT: It will only be set to the majority no choice for the
	// ThresholdFailed state if the vote failed as the result of a majority no
	// vote.  Otherwise, it will be nil for the ThresholdFailed state if the
	// vote failed due to the voting period expiring before any majority is
	// reached.  This distinction allows callers to differentiate between a vote
	// failing due to a majority no vote versus due to expiring, but it does
	// mean the caller must check for nil before using it for the state.
	//
	// It is nil for all other states.
	Choice *chaincfg.Choice
}

// thresholdStateTupleStrings is a map of ThresholdState values back to their
// constant names for pretty printing.
var thresholdStateTupleStrings = map[ThresholdState]string{
	ThresholdInvalid:  "invalid",
	ThresholdDefined:  "defined",
	ThresholdStarted:  "started",
	ThresholdLockedIn: "lockedin",
	ThresholdActive:   "active",
	ThresholdFailed:   "failed",
}

// String returns the ThresholdStateTuple as a human-readable tuple.
func (t ThresholdStateTuple) String() string {
	if s := thresholdStateTupleStrings[t.State]; s != "" {
		if t.Choice != nil {
			return fmt.Sprintf("%v (choice: %v)", s, t.Choice.Id)
		}
		return fmt.Sprintf("%v", s)
	}
	return "invalid"
}

// newThresholdState returns an initialized ThresholdStateTuple.
func newThresholdState(state ThresholdState, choice *chaincfg.Choice) ThresholdStateTuple {
	return ThresholdStateTuple{State: state, Choice: choice}
}

// thresholdStateCache provides a type to cache the threshold states of each
// threshold window for a set of IDs.  It also keeps track of which entries have
// been modified and therefore need to be written to the database.
type thresholdStateCache struct {
	entries map[chainhash.Hash]ThresholdStateTuple
}

// Lookup returns the threshold state associated with the given hash along with
// a boolean that indicates whether or not it is valid.
func (c *thresholdStateCache) Lookup(hash chainhash.Hash) (ThresholdStateTuple, bool) {
	state, ok := c.entries[hash]
	return state, ok
}

// Update updates the cache to contain the provided hash to threshold state
// mapping.
func (c *thresholdStateCache) Update(hash chainhash.Hash, state ThresholdStateTuple) {
	if existing, ok := c.entries[hash]; ok && existing == state {
		return
	}

	c.entries[hash] = state
}

// currentDeploymentVersion returns the highest deployment version that is
// defined in the given network parameters.  Zero is returned if no deployments
// are defined.
func currentDeploymentVersion(params *chaincfg.Params) uint32 {
	var currentVersion uint32
	for version := range params.Deployments {
		if version > currentVersion {
			currentVersion = version
		}
	}
	return currentVersion
}

// nextDeploymentVersion returns the lowest deployment version defined in the
// given network parameters that is higher than the provided version.  Zero is
// returned if a deployment version higher than the provided version does not
// exist.
func nextDeploymentVersion(params *chaincfg.Params, version uint32) uint32 {
	var nextVersion uint32
	for deploymentVersion := range params.Deployments {
		// Continue if the deployment version is not greater than the provided
		// version.
		if deploymentVersion <= version {
			continue
		}

		// Set next version if it has not been set yet or if the deployment
		// version is lower than the current next version.
		if nextVersion == 0 || deploymentVersion < nextVersion {
			nextVersion = deploymentVersion
		}
	}
	return nextVersion
}

// nextThresholdState returns the current rule change threshold state for the
// block AFTER the given node and deployment.  The cache is used to ensure the
// threshold states for previous windows are only calculated once.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) nextThresholdState(prevNode *blockNode, deployment *deploymentInfo) ThresholdStateTuple {
	// The threshold state for the window that contains the genesis block is
	// defined by definition.
	ruleChangeInterval := b.chainParams.RuleChangeActivationInterval
	confirmationWindow := int64(ruleChangeInterval)
	svh := b.chainParams.StakeValidationHeight
	if prevNode == nil || prevNode.height+1 < svh+confirmationWindow {
		return newThresholdState(ThresholdDefined, nil)
	}

	// Get the ancestor that is the last block of the previous confirmation
	// window in order to get its threshold state.  This can be done because
	// the state is the same for all blocks within a given window.
	wantHeight := calcWantHeight(svh, int64(ruleChangeInterval),
		prevNode.height+1)
	prevNode = prevNode.Ancestor(wantHeight)

	// Iterate backwards through each of the previous confirmation windows
	// to find the most recently cached threshold state.
	beginTime := deployment.deployment.StartTime
	cache := deployment.cache
	var neededStates []*blockNode
	for prevNode != nil {
		// Nothing more to do if the state of the block is already
		// cached.
		if _, ok := cache.Lookup(prevNode.hash); ok {
			break
		}

		// The start and expiration times are based on the median block
		// time, so calculate it now.
		medianTime := prevNode.CalcPastMedianTime()

		// The state is simply defined if the start time hasn't been reached
		// yet.
		if uint64(medianTime.Unix()) < beginTime {
			cache.Update(prevNode.hash, newThresholdState(ThresholdDefined, nil))
			break
		}

		// Add this node to the list of nodes that need the state
		// calculated and cached.
		neededStates = append(neededStates, prevNode)

		// Get the ancestor that is the last block of the previous
		// confirmation window.
		prevNode = prevNode.RelativeAncestor(confirmationWindow)
	}

	// Start with the threshold state for the most recent confirmation
	// window that has a cached state.
	stateTuple := newThresholdState(ThresholdDefined, nil)
	if prevNode != nil {
		var ok bool
		stateTuple, ok = cache.Lookup(prevNode.hash)
		if !ok {
			// A cache entry is guaranteed to exist due to the above code and
			// the code below relies on it, so assert the assumption.
			panicf("threshold state cache lookup failed for %v", prevNode.hash)
		}
	}

	// Since each threshold state depends on the state of the previous
	// window, iterate starting from the oldest unknown window.
	endTime := deployment.deployment.ExpireTime
	for neededNum := len(neededStates) - 1; neededNum >= 0; neededNum-- {
		prevNode := neededStates[neededNum]

		switch stateTuple.State {
		case ThresholdDefined:
			// Ensure we are at the minimal require height.
			if prevNode.height < svh {
				stateTuple.State = ThresholdDefined
				break
			}

			// The deployment of the rule change fails if it expires
			// before it is accepted and locked in.
			medianTime := prevNode.CalcPastMedianTime()
			medianTimeUnix := uint64(medianTime.Unix())
			if medianTimeUnix >= endTime {
				stateTuple.State = ThresholdFailed
				break
			}

			// Make sure we are on the correct stake version.
			if b.calcStakeVersion(prevNode) < deployment.version {
				stateTuple.State = ThresholdDefined
				break
			}

			// The state must remain in the defined state so long as
			// a majority of the PoW miners have not upgraded.
			if !b.isMajorityVersion(int32(deployment.version), prevNode,
				b.chainParams.BlockRejectNumRequired) {

				stateTuple.State = ThresholdDefined
				break
			}

			// The state for the rule moves to the started state
			// once its start time has been reached (and it hasn't
			// already expired per the above).
			if medianTimeUnix >= beginTime {
				stateTuple.State = ThresholdStarted
			}

		case ThresholdStarted:
			// The deployment of the rule change fails if it expires
			// before it is accepted and locked in.
			medianTime := prevNode.CalcPastMedianTime()
			if uint64(medianTime.Unix()) >= endTime {
				stateTuple.State = ThresholdFailed
				break
			}

			// The choice index is zero based while the deployment voting choice
			// bit mask is not.  Calculate the number of bits to shift the
			// relevant vote bits (after applying the relevant mask to them) so
			// they are zero based as well.
			voteParams := &deployment.deployment.Vote
			choiceIdxShift := bits.TrailingZeros16(voteParams.Mask)

			// At this point, the rule change is still being voted on, so
			// iterate backwards through the confirmation window to count all of
			// the votes in it.
			var totalNonAbstainVotes uint32
			choiceCounts := make([]uint32, len(voteParams.Choices))
			countNode := prevNode
			for i := int64(0); i < confirmationWindow; i++ {
				for _, vote := range countNode.votes {
					// Ignore votes that are not for the deployment version.
					if vote.Version != deployment.version {
						continue
					}

					// Ignore votes for invalid choices.
					choiceIdx := (vote.Bits & voteParams.Mask) >> choiceIdxShift
					if choiceIdx > uint16(len(voteParams.Choices)-1) {
						continue
					}

					choiceCounts[choiceIdx]++
					if !voteParams.Choices[choiceIdx].IsAbstain {
						totalNonAbstainVotes++
					}
				}

				countNode = countNode.parent
			}

			// The state remains in the started state when the quorum has not
			// been met since the vote requires a quorum for a concrete result.
			if totalNonAbstainVotes < b.chainParams.RuleChangeActivationQuorum {
				break
			}

			// The state moves to either locked in or failed with a specific
			// non-abstaining choice when any of them meets the majority
			// activation threshold.
			ruleChangeActivationThreshold := totalNonAbstainVotes *
				b.chainParams.RuleChangeActivationMultiplier /
				b.chainParams.RuleChangeActivationDivisor
		nextChoice:
			for choiceIdx := range voteParams.Choices {
				// Abstain never changes the state.
				choice := &voteParams.Choices[choiceIdx]
				if choice.IsAbstain {
					continue nextChoice
				}

				// The state does not change when the choice does not reach
				// majority.
				choiceCount := choiceCounts[choiceIdx]
				if choiceCount < ruleChangeActivationThreshold {
					continue nextChoice
				}

				// Move to the locked in or failed state accordingly when the
				// choice has reached majority.
				switch {
				case !choice.IsNo:
					stateTuple.State = ThresholdLockedIn
					stateTuple.Choice = choice
					break nextChoice

				case choice.IsNo:
					stateTuple.State = ThresholdFailed
					stateTuple.Choice = choice
					break nextChoice
				}
			}

		case ThresholdLockedIn:
			// The new rule becomes active when its previous state
			// was locked in.
			stateTuple.State = ThresholdActive

		// Nothing to do if the previous state is active or failed since
		// they are both terminal states.
		case ThresholdActive:
		case ThresholdFailed:
		}

		// Update the cache to avoid recalculating the state in the
		// future.
		cache.Update(prevNode.hash, stateTuple)
	}

	return stateTuple
}

// deploymentState returns the current rule change threshold for a given
// deployment.  The threshold is evaluated from the point of view of the block
// node passed in as the first argument to this method.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) deploymentState(prevNode *blockNode, deployment *deploymentInfo) (ThresholdStateTuple, error) {
	if deployment.forcedState != nil {
		return *deployment.forcedState, nil
	}

	return b.nextThresholdState(prevNode, deployment), nil
}

// stateLastChanged returns the node at which the provided consensus deployment
// agenda last changed state.  The function will return nil if the state has
// never changed.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) stateLastChanged(node *blockNode, deployment *deploymentInfo) *blockNode {
	// No state changes are possible if the chain is not yet past stake
	// validation height and had a full interval to change.
	confirmationInterval := int64(b.chainParams.RuleChangeActivationInterval)
	svh := b.chainParams.StakeValidationHeight
	if node == nil || node.height < svh+confirmationInterval {
		return nil
	}

	// Determine the current state.  Notice that nextThresholdState always
	// calculates the state for the block after the provided one, so use the
	// parent to get the state for the requested block.
	curState := b.nextThresholdState(node.parent, deployment)

	// Determine the first block of the current confirmation interval in order
	// to determine block at which the state possibly changed.  Since the state
	// can only change at an interval boundary, loop backwards one interval at
	// a time to determine when (and if) the state changed.
	finalNodeHeight := calcWantHeight(svh, confirmationInterval, node.height)
	node = node.Ancestor(finalNodeHeight + 1)
	priorStateChangeNode := node
	for node != nil && node.parent != nil {
		// As previously mentioned, nextThresholdState always calculates the
		// state for the block after the provided one, so use the parent to get
		// the state of the block itself.
		state := b.nextThresholdState(node.parent, deployment)

		if state.State != curState.State {
			return priorStateChangeNode
		}

		// Get the ancestor that is the first block of the previous confirmation
		// interval.
		priorStateChangeNode = node
		node = node.RelativeAncestor(confirmationInterval)
	}

	return nil
}

// StateLastChangedHeight returns the height at which the provided consensus
// deployment agenda last changed state.  Note that, unlike the
// NextThresholdState function, this function returns the information as of the
// passed block hash.
//
// This function is safe for concurrent access.
func (b *BlockChain) StateLastChangedHeight(hash *chainhash.Hash, deploymentID string) (int64, error) {
	node := b.index.LookupNode(hash)
	if node == nil || !b.index.CanValidate(node) {
		return 0, unknownBlockError(hash)
	}

	// Determine the deployment details for the provided deployment id.
	deployment, ok := b.deploymentData[deploymentID]
	if !ok {
		str := fmt.Sprintf("deployment ID %s does not exist", deploymentID)
		return 0, contextError(ErrUnknownDeploymentID, str)
	}
	if deployment.forcedState != nil {
		// The state change height is 1 since the genesis block never
		// experiences changes regardless of consensus rule changes.
		return 1, nil
	}

	// Find the node at which the current state changed.
	b.chainLock.Lock()
	stateNode := b.stateLastChanged(node, &deployment)
	b.chainLock.Unlock()

	var height int64
	if stateNode != nil {
		height = stateNode.height
	}
	return height, nil
}

// NextThresholdState returns the current rule change threshold state of the
// given deployment ID for the block AFTER the provided block hash.
//
// This function is safe for concurrent access.
func (b *BlockChain) NextThresholdState(hash *chainhash.Hash, deploymentID string) (ThresholdStateTuple, error) {
	node := b.index.LookupNode(hash)
	if node == nil || !b.index.CanValidate(node) {
		return ThresholdStateTuple{}, unknownBlockError(hash)
	}

	deployment, ok := b.deploymentData[deploymentID]
	if !ok {
		str := fmt.Sprintf("deployment ID %s does not exist", deploymentID)
		return ThresholdStateTuple{}, contextError(ErrUnknownDeploymentID, str)
	}

	b.chainLock.Lock()
	state, err := b.deploymentState(node, &deployment)
	b.chainLock.Unlock()
	return state, err
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
	// Determine the correct deployment details for the LN features consensus
	// vote as defined in DCP0002 and DCP0003.
	const deploymentID = chaincfg.VoteIDLNFeatures
	deployment, ok := b.deploymentData[deploymentID]
	if !ok {
		str := fmt.Sprintf("deployment ID %s does not exist", deploymentID)
		return false, contextError(ErrUnknownDeploymentID, str)
	}

	state, err := b.deploymentState(prevNode, &deployment)
	if err != nil {
		return false, err
	}

	// NOTE: The choice field of the return threshold state is not examined
	// here because there is only one possible choice that can be active for
	// the agenda, which is yes, so there is no need to check it.
	return state.State == ThresholdActive, nil
}

// IsLNFeaturesAgendaActive returns whether or not the LN features agenda vote,
// as defined in DCP0002 and DCP0003 has passed and is now active for the block
// AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsLNFeaturesAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	prevNode := b.index.LookupNode(prevHash)
	if prevNode == nil || !b.index.CanValidate(prevNode) {
		return false, unknownBlockError(prevHash)
	}

	b.chainLock.Lock()
	isActive, err := b.isLNFeaturesAgendaActive(prevNode)
	b.chainLock.Unlock()
	return isActive, err
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
	// Determine the correct deployment details for the header commitments
	// consensus vote as defined in DCP0005.
	const deploymentID = chaincfg.VoteIDHeaderCommitments
	deployment, ok := b.deploymentData[deploymentID]
	if !ok {
		str := fmt.Sprintf("deployment ID %s does not exist", deploymentID)
		return false, contextError(ErrUnknownDeploymentID, str)
	}

	state, err := b.deploymentState(prevNode, &deployment)
	if err != nil {
		return false, err
	}

	// NOTE: The choice field of the return threshold state is not examined
	// here because there is only one possible choice that can be active for
	// the agenda, which is yes, so there is no need to check it.
	return state.State == ThresholdActive, nil
}

// IsHeaderCommitmentsAgendaActive returns whether or not the header commitments
// agenda vote, as defined in DCP0005 has passed and is now active for the block
// AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsHeaderCommitmentsAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	node := b.index.LookupNode(prevHash)
	if node == nil || !b.index.CanValidate(node) {
		return false, unknownBlockError(prevHash)
	}

	b.chainLock.Lock()
	isActive, err := b.isHeaderCommitmentsAgendaActive(node)
	b.chainLock.Unlock()
	return isActive, err
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
	if prevNode.height == 0 {
		return false, nil
	}

	// Determine the correct deployment details for the decentralized treasury
	// consensus vote as defined in DCP0006.
	const deploymentID = chaincfg.VoteIDTreasury
	deployment, ok := b.deploymentData[deploymentID]
	if !ok {
		str := fmt.Sprintf("deployment ID %s does not exist", deploymentID)
		return false, contextError(ErrUnknownDeploymentID, str)
	}

	state, err := b.deploymentState(prevNode, &deployment)
	if err != nil {
		return false, err
	}

	// NOTE: The choice field of the return threshold state is not examined
	// here because there is only one possible choice that can be active for
	// the agenda, which is yes, so there is no need to check it.
	return state.State == ThresholdActive, nil
}

// IsTreasuryAgendaActive returns whether or not the treasury agenda vote, as
// defined in DCP0006, has passed and is now active for the block AFTER the
// given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsTreasuryAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	// The treasury agenda is never active for the genesis block.
	if *prevHash == *zeroHash {
		return false, nil
	}

	prevNode := b.index.LookupNode(prevHash)
	if prevNode == nil || !b.index.CanValidate(prevNode) {
		return false, unknownBlockError(prevHash)
	}

	b.chainLock.Lock()
	isActive, err := b.isTreasuryAgendaActive(prevNode)
	b.chainLock.Unlock()
	return isActive, err
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
	// Determine the correct deployment details for the revert treasury
	// expenditure policy consensus vote as defined in DCP0007.
	const deploymentID = chaincfg.VoteIDRevertTreasuryPolicy
	deployment, ok := b.deploymentData[deploymentID]
	if !ok {
		str := fmt.Sprintf("deployment ID %s does not exist", deploymentID)
		return false, contextError(ErrUnknownDeploymentID, str)
	}

	state, err := b.deploymentState(prevNode, &deployment)
	if err != nil {
		return false, err
	}

	// NOTE: The choice field of the return threshold state is not examined
	// here because there is only one possible choice that can be active for
	// the agenda, which is yes, so there is no need to check it.
	return state.State == ThresholdActive, nil
}

// IsRevertTreasuryPolicyActive returns whether or not the revert treasury
// expenditure policy agenda vote, as defined in DCP0007, has passed and is now
// active for the block AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsRevertTreasuryPolicyActive(prevHash *chainhash.Hash) (bool, error) {
	prevNode := b.index.LookupNode(prevHash)
	if prevNode == nil || !b.index.CanValidate(prevNode) {
		return false, unknownBlockError(prevHash)
	}

	b.chainLock.Lock()
	isActive, err := b.isRevertTreasuryPolicyActive(prevNode)
	b.chainLock.Unlock()
	return isActive, err
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
	// Determine the correct deployment details for the explicit version
	// upgrades consensus vote as defined in DCP0008.
	const deploymentID = chaincfg.VoteIDExplicitVersionUpgrades
	deployment, ok := b.deploymentData[deploymentID]
	if !ok {
		str := fmt.Sprintf("deployment ID %s does not exist", deploymentID)
		return false, contextError(ErrUnknownDeploymentID, str)
	}

	state, err := b.deploymentState(prevNode, &deployment)
	if err != nil {
		return false, err
	}

	// NOTE: The choice field of the return threshold state is not examined
	// here because there is only one possible choice that can be active for
	// the agenda, which is yes, so there is no need to check it.
	return state.State == ThresholdActive, nil
}

// IsExplicitVerUpgradesAgendaActive returns whether or not the explicit version
// upgrades agenda, as defined in DCP0008, has passed and is now active for the
// block AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsExplicitVerUpgradesAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	// The agenda is never active for the genesis block.
	if *prevHash == *zeroHash {
		return false, nil
	}

	prevNode := b.index.LookupNode(prevHash)
	if prevNode == nil || !b.index.CanValidate(prevNode) {
		return false, unknownBlockError(prevHash)
	}

	b.chainLock.Lock()
	isActive, err := b.isExplicitVerUpgradesAgendaActive(prevNode)
	b.chainLock.Unlock()
	return isActive, err
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
	// Determine the correct deployment details for the automatic ticket
	// revocations consensus vote as defined in DCP0009.
	const deploymentID = chaincfg.VoteIDAutoRevocations
	deployment, ok := b.deploymentData[deploymentID]
	if !ok {
		str := fmt.Sprintf("deployment ID %s does not exist", deploymentID)
		return false, contextError(ErrUnknownDeploymentID, str)
	}

	state, err := b.deploymentState(prevNode, &deployment)
	if err != nil {
		return false, err
	}

	// NOTE: The choice field of the return threshold state is not examined
	// here because there is only one possible choice that can be active for
	// the agenda, which is yes, so there is no need to check it.
	return state.State == ThresholdActive, nil
}

// IsAutoRevocationsAgendaActive returns whether or not the automatic ticket
// revocations agenda vote, as defined in DCP0009, has passed and is now active
// for the block AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsAutoRevocationsAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	// The auto revocations agenda is never active for the genesis block.
	if *prevHash == *zeroHash {
		return false, nil
	}

	prevNode := b.index.LookupNode(prevHash)
	if prevNode == nil || !b.index.CanValidate(prevNode) {
		return false, unknownBlockError(prevHash)
	}

	b.chainLock.Lock()
	isActive, err := b.isAutoRevocationsAgendaActive(prevNode)
	b.chainLock.Unlock()
	return isActive, err
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
	// Determine the correct deployment details for the block reward subsidy
	// split change consensus vote as defined in DCP0010.
	const deploymentID = chaincfg.VoteIDChangeSubsidySplit
	deployment, ok := b.deploymentData[deploymentID]
	if !ok {
		str := fmt.Sprintf("deployment ID %s does not exist", deploymentID)
		return false, contextError(ErrUnknownDeploymentID, str)
	}

	state, err := b.deploymentState(prevNode, &deployment)
	if err != nil {
		return false, err
	}

	// NOTE: The choice field of the return threshold state is not examined
	// here because there is only one possible choice that can be active for
	// the agenda, which is yes, so there is no need to check it.
	return state.State == ThresholdActive, nil
}

// IsSubsidySplitAgendaActive returns whether or not the agenda to change the
// block reward subsidy split to 10/80/10, as defined in DCP0010, has passed and
// is now active for the block AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsSubsidySplitAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	// The agenda is never active for the genesis block.
	if *prevHash == *zeroHash {
		return false, nil
	}

	prevNode := b.index.LookupNode(prevHash)
	if prevNode == nil || !b.index.CanValidate(prevNode) {
		return false, unknownBlockError(prevHash)
	}

	b.chainLock.Lock()
	isActive, err := b.isSubsidySplitAgendaActive(prevNode)
	b.chainLock.Unlock()
	return isActive, err
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
	// Determine the correct deployment details for the block reward subsidy
	// split change consensus vote as defined in DCP0012.
	const deploymentID = chaincfg.VoteIDChangeSubsidySplitR2
	deployment, ok := b.deploymentData[deploymentID]
	if !ok {
		str := fmt.Sprintf("deployment ID %s does not exist", deploymentID)
		return false, contextError(ErrUnknownDeploymentID, str)
	}

	state, err := b.deploymentState(prevNode, &deployment)
	if err != nil {
		return false, err
	}

	// NOTE: The choice field of the return threshold state is not examined
	// here because there is only one possible choice that can be active for
	// the agenda, which is yes, so there is no need to check it.
	return state.State == ThresholdActive, nil
}

// IsSubsidySplitR2AgendaActive returns whether or not the agenda to change the
// block reward subsidy split to 1/89/10, as defined in DCP0012, has passed and
// is now active for the block AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsSubsidySplitR2AgendaActive(prevHash *chainhash.Hash) (bool, error) {
	// The agenda is never active for the genesis block.
	if *prevHash == *zeroHash {
		return false, nil
	}

	prevNode := b.index.LookupNode(prevHash)
	if prevNode == nil || !b.index.CanValidate(prevNode) {
		return false, unknownBlockError(prevHash)
	}

	b.chainLock.Lock()
	isActive, err := b.isSubsidySplitR2AgendaActive(prevNode)
	b.chainLock.Unlock()
	return isActive, err
}

// VoteCounts is a compacted struct that is used to message vote counts.
type VoteCounts struct {
	Total        uint32
	TotalAbstain uint32
	VoteChoices  []uint32
}

// getVoteCounts returns the vote counts for the specified version for the
// current rule change activation interval.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) getVoteCounts(node *blockNode, version uint32, d *chaincfg.ConsensusDeployment) VoteCounts {
	// Don't try to count votes before the stake validation height since there
	// could not possibly have been any.
	svh := b.chainParams.StakeValidationHeight
	if node.height < svh {
		return VoteCounts{
			VoteChoices: make([]uint32, len(d.Vote.Choices)),
		}
	}

	// Calculate the final height of the prior interval.
	rcai := int64(b.chainParams.RuleChangeActivationInterval)
	height := calcWantHeight(svh, rcai, node.height)

	result := VoteCounts{
		VoteChoices: make([]uint32, len(d.Vote.Choices)),
	}
	countNode := node
	for countNode.height > height {
		for _, vote := range countNode.votes {
			// Wrong versions do not count.
			if vote.Version != version {
				continue
			}

			// Increase total votes.
			result.Total++

			index := d.Vote.VoteIndex(vote.Bits)
			if index == -1 {
				// Invalid votes are treated as abstain.
				result.TotalAbstain++
				continue
			} else if d.Vote.Choices[index].IsAbstain {
				result.TotalAbstain++
			}
			result.VoteChoices[index]++
		}

		countNode = countNode.parent
	}

	return result
}

// GetVoteCounts returns the vote counts for the specified version and
// deployment identifier for the current rule change activation interval.
//
// This function is safe for concurrent access.
func (b *BlockChain) GetVoteCounts(version uint32, deploymentID string) (VoteCounts, error) {
	for k := range b.chainParams.Deployments[version] {
		deployment := &b.chainParams.Deployments[version][k]
		if deployment.Vote.Id == deploymentID {
			b.chainLock.Lock()
			counts := b.getVoteCounts(b.bestChain.Tip(), version, deployment)
			b.chainLock.Unlock()
			return counts, nil
		}
	}
	str := fmt.Sprintf("deployment ID %s does not exist", deploymentID)
	return VoteCounts{}, contextError(ErrUnknownDeploymentID, str)
}

// CountVoteVersion returns the total number of version votes for the current
// rule change activation interval.
//
// This function is safe for concurrent access.
func (b *BlockChain) CountVoteVersion(version uint32) (uint32, error) {
	b.chainLock.Lock()
	defer b.chainLock.Unlock()
	countNode := b.bestChain.Tip()

	// Don't try to count votes before the stake validation height since there
	// could not possibly have been any.
	svh := b.chainParams.StakeValidationHeight
	if countNode.height < svh {
		return 0, nil
	}

	// Calculate the final height of the prior interval.
	rcai := int64(b.chainParams.RuleChangeActivationInterval)
	height := calcWantHeight(svh, rcai, countNode.height)

	total := uint32(0)
	for countNode.height > height {
		for _, vote := range countNode.votes {
			// Wrong versions do not count.
			if vote.Version != version {
				continue
			}

			// Increase total votes.
			total++
		}

		countNode = countNode.parent
	}

	return total, nil
}
