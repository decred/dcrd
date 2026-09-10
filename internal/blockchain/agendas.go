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
	"github.com/decred/dcrd/wire"
)

// requiredAgendaIDs identifies IDs for all agendas that influence consensus
// behavior.
//
// Any agenda IDs in this list that do not also have an associated deployment in
// the network chain parameters will have an associated default consensus agenda
// created such that the agenda will always be inactive for the main network and
// active (with an empty choice) for all other networks.
//
// The primary motivation is to allow the simulation network and new versions of
// test networks that explicitly want to avoid requiring an agenda vote to use
// newer consensus rules by default.
//
// The behavior for the non-main network case is similar to, but slightly
// different than, a deployment with a forced choice of active.  Both scenarios
// ultimately result in an agenda always being considered active without a vote,
// but an explicitly forced choice requires entries in the network chain params
// of every network even though they never had a deployment vote for the change.
var requiredAgendaIDs = map[string]struct{}{
	chaincfg.VoteIDMaxBlockSize:            {},
	chaincfg.VoteIDSDiffAlgorithm:          {},
	chaincfg.VoteIDLNFeatures:              {},
	chaincfg.VoteIDFixLNSeqLocks:           {},
	chaincfg.VoteIDHeaderCommitments:       {},
	chaincfg.VoteIDTreasury:                {},
	chaincfg.VoteIDRevertTreasuryPolicy:    {},
	chaincfg.VoteIDExplicitVersionUpgrades: {},
	chaincfg.VoteIDAutoRevocations:         {},
	chaincfg.VoteIDChangeSubsidySplit:      {},
	chaincfg.VoteIDBlake3Pow:               {},
	chaincfg.VoteIDChangeSubsidySplitR2:    {},
	chaincfg.VoteIDMaxTreasurySpend:        {},
}

// historicalAgenda defines the result of a historical consensus rule change
// vote that is now an established fact of the chain.
type historicalAgenda map[string]historicalActivationState

// makeHistoricalAgendas returns a map that defines the result of historical
// consensus rule change votes that are now established facts of the chain.
//
// Note that the anchor height and hash identify the parent of the historical
// block at which a definitive result was determined.  This differs from the
// actual block height and hash published in DCPs, which identify the historical
// block itself rather than its parent.  It is important to use the parent as
// the anchor because successful rule changes require all descendants of the
// parent to be validated under the new rules, including the exact historical
// block in the main chain at which the rules activated as well as any side
// chain blocks at that same height.
func makeHistoricalAgendas() map[wire.CurrencyNet]historicalAgenda {
	return map[wire.CurrencyNet]historicalAgenda{
		wire.MainNet: {
			chaincfg.VoteIDSDiffAlgorithm: {
				anchorHeight: 149247,
				anchorHash:   *mustParseHash("0000000000000139582d056bc20bb352f4e9b248acbb202724f46000e59c9f75"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDLNSupport: {
				anchorHeight: 149247,
				anchorHash:   *mustParseHash("0000000000000139582d056bc20bb352f4e9b248acbb202724f46000e59c9f75"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDLNFeatures: {
				anchorHeight: 189567,
				anchorHash:   *mustParseHash("000000000000005ca2ebe4ef0649128e1cd16d063f80c884cbd865d3af2300ae"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDFixLNSeqLocks: {
				anchorHeight: 342783,
				anchorHash:   *mustParseHash("000000000000000017c053b63c7ae9bb8e73ee1c13ef3da493042a7a05d7401c"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDHeaderCommitments: {
				anchorHeight: 431487,
				anchorHash:   *mustParseHash("0000000000000000225b8f795704d4ebc484fad4ab09eec615fb44b4a95c3c26"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDTreasury: {
				anchorHeight: 552447,
				anchorHash:   *mustParseHash("000000000000000012d3e57c450f632a9877a28534dbffbf85d41a78aadb9acb"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDRevertTreasuryPolicy: {
				anchorHeight: 657279,
				anchorHash:   *mustParseHash("00000000000000000669da8bbb90dad39a2fd889cbdda00bf259776af822fcaa"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDExplicitVersionUpgrades: {
				anchorHeight: 657279,
				anchorHash:   *mustParseHash("00000000000000000669da8bbb90dad39a2fd889cbdda00bf259776af822fcaa"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDAutoRevocations: {
				anchorHeight: 657279,
				anchorHash:   *mustParseHash("00000000000000000669da8bbb90dad39a2fd889cbdda00bf259776af822fcaa"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDChangeSubsidySplit: {
				anchorHeight: 657279,
				anchorHash:   *mustParseHash("00000000000000000669da8bbb90dad39a2fd889cbdda00bf259776af822fcaa"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDBlake3Pow: {
				anchorHeight: 794367,
				anchorHash:   *mustParseHash("0000000000000000c293d8c67409d05e960447ea25cdaf770e864d995c764ef0"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDChangeSubsidySplitR2: {
				anchorHeight: 794367,
				anchorHash:   *mustParseHash("0000000000000000c293d8c67409d05e960447ea25cdaf770e864d995c764ef0"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDMaxTreasurySpend: {
				anchorHeight: 1052415,
				anchorHash:   *mustParseHash("bc1b84dcfd532a01fbd42470d0668fcdbc7e6e4e9108218f55025d9e565dfaea"),
				choiceID:     "yes",
			},
		},
		wire.TestNet3: {
			chaincfg.VoteIDFixLNSeqLocks: {
				anchorHeight: 136847,
				anchorHash:   *mustParseHash("0000000004232de037b11ee39641910ae0fd0becb968f76f20a40cb7fa64f475"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDHeaderCommitments: {
				anchorHeight: 323327,
				anchorHash:   *mustParseHash("0000002438f7146f7dbfea248b4c94c4c35a8c714442a589110f4c7513a6b26a"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDTreasury: {
				anchorHeight: 560207,
				anchorHash:   *mustParseHash("0000004f5b6cefb4b5c0ae9dd7a92a431c2de697d22aed9215fe595ac8c1ec23"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDRevertTreasuryPolicy: {
				anchorHeight: 867647,
				anchorHash:   *mustParseHash("0000000036af134886ea61aecea318c1f18f349d3acbf2a8c40ced62854b057b"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDExplicitVersionUpgrades: {
				anchorHeight: 867647,
				anchorHash:   *mustParseHash("0000000036af134886ea61aecea318c1f18f349d3acbf2a8c40ced62854b057b"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDAutoRevocations: {
				anchorHeight: 867647,
				anchorHash:   *mustParseHash("0000000036af134886ea61aecea318c1f18f349d3acbf2a8c40ced62854b057b"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDChangeSubsidySplit: {
				anchorHeight: 877727,
				anchorHash:   *mustParseHash("000000000000e9528cd8a67898b2c02c854c825a3d09c0d83296f051d8c2adac"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDBlake3Pow: {
				anchorHeight: 1170047,
				anchorHash:   *mustParseHash("000000b396bfeaa6ae6fa9e3cee441d7215191630bdaa9b979a872985caed727"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDChangeSubsidySplitR2: {
				anchorHeight: 1170047,
				anchorHash:   *mustParseHash("000000b396bfeaa6ae6fa9e3cee441d7215191630bdaa9b979a872985caed727"),
				choiceID:     "yes",
			},
			chaincfg.VoteIDMaxTreasurySpend: {
				anchorHeight: 1805087,
				anchorHash:   *mustParseHash("f3e25c6c40baaed62b27ad4f8c0ed6be89bc6890b415089e33acf7783ded248d"),
				choiceID:     "yes",
			},
		},
	}
}

// deploymentInfo houses information about the state of a consensus rule change
// deployment.
type deploymentInfo struct {
	// version is the associated deployment version.
	version uint32

	// deployment houses the deployment parameters for the associated consensus
	// rule change vote.
	deployment *chaincfg.ConsensusDeployment

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
	tuple := newThresholdState(state, forcedChoiceID)
	return &tuple, nil
}

// historicalActivationState houses information about hard-coded historical
// agenda activations.
type historicalActivationState struct {
	// anchorHeight is the height of the parent of the historical block at which
	// the agenda activated.
	anchorHeight int64

	// anchorHash is the hash of the parent of the historical block at which
	// the agenda activated.
	anchorHash chainhash.Hash

	// choiceID is the ID of the specific winning choice for the associated
	// agenda.
	choiceID string
}

// activeAnchorState houses the parent of the block at which an agenda activated
// along with the winning choice id.
type activeAnchorState struct {
	anchor   *blockNode
	choiceID string
}

// consensusAgenda houses information about the state of a consensus rule change
// agenda.
type consensusAgenda struct {
	// forcedState optionally specifies a threshold state to use instead of
	// determining the state via other means, such as tallying votes for a
	// deployment.  This only applies when it is not nil.
	//
	// It is only populated when the associated chain parameters specify a
	// forced choice or a required agenda is created by default because it has
	// no associated deployment in the chain parameters.
	forcedState *ThresholdStateTuple

	// historicalState optionally specifies hard-coded information about the
	// historical activation of the agenda.  It only applies when it is not nil.
	//
	// It is only set when the hard-coded historical vote results include the
	// necessary data for the agenda on the network and no forced state is
	// specified (aka is nil).
	historicalState *historicalActivationState

	// activeAnchor is a cached anchor point that corresponds to the parent of
	// the block at which the agenda activated.
	//
	// It is set when the associated agenda has been determined to be active
	// either when it is resolved via the historical state or discovered when
	// tallying votes in a full-context path.
	//
	// It will not be set when a forced state is specified (aka not nil).
	//
	// It is protected by the chain state mutex.
	activeAnchor *activeAnchorState

	// deployment optionally houses information about the associated consensus
	// rule change deployment.  It will only be set when the associated details
	// are specified by the chain parameters and there is not a forced state.
	deployment *deploymentInfo
}

// makeAgendas returns a map of consensus rule change agendas populated with
// details used to determine the status of each agenda.
//
// The returned map will contain an agenda for every deployment specified in the
// provided chain params for the network.  Each agenda added as a result of a
// deployment that does not also have a forced state specified will have the
// [consensusAgenda.deployment] field populated with the relevant details
// extracted from the associated deployment.
//
// It also returns an appropriate error when any additional sanity checks fail.
// For example, duplicate deployment IDs are rejected and forced choices in the
// chain params are disallowed on the main network.
func makeAgendas(params *chaincfg.Params, historicalAgendas map[wire.CurrencyNet]historicalAgenda) (map[string]*consensusAgenda, error) {
	// Create an agenda for each deployment specified in the chain params.
	agendas := make(map[string]*consensusAgenda)
	for version, deployments := range params.Deployments {
		var usedMaskBits uint16
		for i := range deployments {
			deployment := &deployments[i]
			id := deployment.Vote.Id
			if _, ok := agendas[id]; ok {
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

			agenda := &consensusAgenda{
				forcedState: forcedState,
			}
			if forcedState == nil {
				agenda.deployment = &deploymentInfo{
					version:    version,
					deployment: deployment,
					cache: &thresholdStateCache{
						entries: make(map[chainhash.Hash]ThresholdStateTuple),
					},
				}
			}
			agendas[id] = agenda
		}
	}

	// Add historical consensus change details to the agendas map.
	//
	// This currently requires an associated deployment.
	//
	// Ideally, a historical agenda should be able to stand in for a deployment
	// entirely in addition to working alongside one when present.  However, the
	// ability to query state changes and vote information via
	// [BlockChain.StateLastChangedHeight], [BlockChain.NextThresholdState], and
	// [BlockChain.GetVoteCounts] currently depend on agendas having an
	// associated deployment.  They would need to be modified to support missing
	// deployment information first in order to allow an agenda to provide only
	// historical state with no associated deployment.
	for id, histAgenda := range historicalAgendas[params.Net] {
		info, ok := agendas[id]
		if !ok {
			str := fmt.Sprintf("agenda ID %s for historical consensus "+
				"change does not exist", id)
			return nil, contextError(ErrUnknownAgendaID, str)
		}

		if info.forcedState != nil {
			str := fmt.Sprintf("agenda ID %s has both a forced choice and a "+
				"historical state configured", id)
			return nil, contextError(ErrHistoricalForcedChoice, str)
		}

		// Find the specified winning choice ID within the associated deployment
		// vote choices.  The choice must not be the abstain choice because
		// agendas for historical consensus changes must have resolved to either
		// an active or failed state.
		var winningChoice *chaincfg.Choice
		for choiceIdx := range info.deployment.deployment.Vote.Choices {
			choice := &info.deployment.deployment.Vote.Choices[choiceIdx]
			if choice.Id == histAgenda.choiceID {
				winningChoice = choice
				break
			}
		}
		if winningChoice == nil {
			str := fmt.Sprintf("deployment ID %s has a historical state with "+
				"unknown winning choice %q", id, histAgenda.choiceID)
			return nil, contextError(ErrUnknownDeploymentChoice, str)
		}
		if winningChoice.IsAbstain {
			str := fmt.Sprintf("deployment ID %s historical state choice %q "+
				"is of invalid type abstain", id, histAgenda.choiceID)
			return nil, contextError(ErrDeploymentChoiceAbstain, str)
		}

		// Only add changes that passed to historical activations.
		if winningChoice.IsNo {
			continue
		}

		info.historicalState = &historicalActivationState{
			anchorHeight: histAgenda.anchorHeight,
			anchorHash:   histAgenda.anchorHash,
			choiceID:     winningChoice.Id,
		}
	}

	// Create an agenda with a default forced state and nil deployment for each
	// required agenda that does not have an associated deployment specified in
	// the chain params.
	for id := range requiredAgendaIDs {
		if _, ok := agendas[id]; ok {
			continue
		}

		var forcedState ThresholdStateTuple
		if isMainNet(params) {
			forcedState = newThresholdState(ThresholdDefined, "")
		} else {
			forcedState = newThresholdState(ThresholdActive, "")
		}
		agendas[id] = &consensusAgenda{
			forcedState: &forcedState,
		}
	}

	return agendas, nil
}

// agendaActiveInfo houses whether or not a positional agenda state query result
// is valid and, when it is, whether or not the agenda is active.
type agendaActiveInfo struct {
	isValid  bool
	isActive bool
}

// isAgendaActivePositional attempts to determine whether or not an agenda is
// active for the block AFTER the given block node using only information
// available that depends on its position within the block chain and the headers
// of all ancestors.  It does not, and must not, rely on having the full block
// data of any ancestors available or deployment details associated with the
// agenda.
//
// It is not always possible to determine whether an agenda is active since it
// might depend on votes in ancestor blocks.  The valid flag in the returned
// struct indicates whether or not the active flag is safe to use.
//
// The result can be determined in the following circumstances:
//
//   - The agenda is forced active
//   - The parent of the activation block is a historical fact
//   - The activation has been opportunistically discovered while tallying votes
//     in a full-context path
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isAgendaActivePositional(prevNode *blockNode, agenda *consensusAgenda) agendaActiveInfo {
	// Agendas are never active for the genesis block.
	if prevNode == nil {
		return agendaActiveInfo{isValid: true, isActive: false}
	}

	// Forced states are independent of chain position and take precedence.
	if state := agenda.forcedState; state != nil {
		isActive := state.State == ThresholdActive
		return agendaActiveInfo{isValid: true, isActive: isActive}
	}

	// The deployment is definitively known to be inactive when the activation
	// height is a known historical fact and the queried block is before the
	// activation anchor.
	//
	// The historical state identifies the parent of the block where the agenda
	// activated and the queried block height is one more than the provided
	// parent node, so the comparison is intentionally exclusive.
	hs := agenda.historicalState
	if hs != nil && prevNode.height < hs.anchorHeight {
		return agendaActiveInfo{isValid: true, isActive: false}
	}

	// Use the previously cached anchor when it exists and is actually an
	// ancestor of the queried block (which includes the anchor block itself).
	//
	// The anchor may have been determined based on a known historical fact
	// below or discovered opportunistically when tallying votes in a
	// full-context path.
	if anchorState := agenda.activeAnchor; anchorState != nil {
		anchor := anchorState.anchor
		if anchor != nil && anchor.IsAncestorOf(prevNode) {
			return agendaActiveInfo{isValid: true, isActive: true}
		}
	}

	// Attempt to resolve a known historical activation anchor using the
	// ancestry of the queried block itself.
	//
	// It is highly likely that future calls will involve descendants of this
	// anchor once it is resolved as opposed to blocks on entirely unrelated
	// side chains.
	//
	// Unrelated side chains are handled correctly because the cached anchor is
	// only used when it is an ancestor of the queried block and the anchor is
	// only set when its hash matches the historical anchor hash.
	if hs != nil && prevNode.height >= hs.anchorHeight {
		anchor := prevNode.Ancestor(hs.anchorHeight)
		if anchor != nil && anchor.hash == hs.anchorHash {
			agenda.activeAnchor = &activeAnchorState{
				anchor:   anchor,
				choiceID: hs.choiceID,
			}
			return agendaActiveInfo{isValid: true, isActive: true}
		}
	}

	return agendaActiveInfo{isValid: false}
}

// isAgendaActivePositionalByID attempts to determine whether or not an agenda
// is active for the block AFTER the given block node using only information
// available that depends on its position within the block chain and having the
// headers of all ancestors available.  It does not, and must not, rely on
// having the full block data of all ancestors available or deployment details
// associated with the agenda.
//
// See [BlockChain.isAgendaActivePositional] for more details.  This only
// differs in that it is a convenience wrapper that looks up the agenda for the
// given ID and returns an error if the provided ID is unknown.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isAgendaActivePositionalByID(prevNode *blockNode, agendaID string) (agendaActiveInfo, error) {
	agenda, ok := b.agendas[agendaID]
	if !ok {
		str := fmt.Sprintf("agenda ID %s does not exist", agendaID)
		return agendaActiveInfo{}, contextError(ErrUnknownAgendaID, str)
	}

	info := b.isAgendaActivePositional(prevNode, agenda)
	return info, nil
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
func (b *BlockChain) isAgendaActive(prevNode *blockNode, agendaID string) (bool, error) {
	agenda, ok := b.agendas[agendaID]
	if !ok {
		str := fmt.Sprintf("agenda ID %s does not exist", agendaID)
		return false, contextError(ErrUnknownAgendaID, str)
	}

	// Agendas are never active for the genesis block.
	if prevNode == nil {
		return false, nil
	}

	// Attempt to determine the status of the agenda via the faster positional
	// path which includes making use of things such as forced active agendas,
	// cached anchor points and hard-coded historical activation points.
	info := b.isAgendaActivePositional(prevNode, agenda)
	if info.isValid {
		return info.isActive, nil
	}

	// Determine the status by tallying votes.
	//
	// NOTE: The choice field of the return threshold state is intentionally not
	// examined here.  This assumes there is only one possible passing choice
	// that makes the agenda active.  Consequently, this function is not
	// suitable for agendas with more than one possible passing choice.
	state := b.agendaState(prevNode, agenda)
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

// isMaxBlockSizeAgendaActive returns whether or not the max block size agenda
// vote that only took place on an earlier version of the test network has
// passed and is now active from the point of view of the passed block node.
//
// The original test network where the vote took place has since been replaced
// with a newer version that already has the larger size specified as the
// default and no associated vote.  Therefore, in practice, only the simulation
// and regression networks can currently have this active.
//
// It is important to note that, as the variable name indicates, this function
// expects the block node prior to the block for which the deployment state is
// desired.  In other words, the returned deployment state is for the block
// AFTER the passed node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) isMaxBlockSizeAgendaActive(prevNode *blockNode) (bool, error) {
	const agendaID = chaincfg.VoteIDMaxBlockSize
	return b.isAgendaActive(prevNode, agendaID)
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
	const agendaID = chaincfg.VoteIDSDiffAlgorithm
	return b.isAgendaActive(prevNode, agendaID)
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
	const agendaID = chaincfg.VoteIDLNFeatures
	return b.isAgendaActive(prevNode, agendaID)
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
	const agendaID = chaincfg.VoteIDHeaderCommitments
	return b.isAgendaActive(prevNode, agendaID)
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

	const agendaID = chaincfg.VoteIDTreasury
	return b.isAgendaActive(prevNode, agendaID)
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
	const agendaID = chaincfg.VoteIDRevertTreasuryPolicy
	return b.isAgendaActive(prevNode, agendaID)
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
	const agendaID = chaincfg.VoteIDExplicitVersionUpgrades
	return b.isAgendaActive(prevNode, agendaID)
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
	const agendaID = chaincfg.VoteIDAutoRevocations
	return b.isAgendaActive(prevNode, agendaID)
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
	const agendaID = chaincfg.VoteIDChangeSubsidySplit
	return b.isAgendaActive(prevNode, agendaID)
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
// active.
//
// This function is safe for concurrent access.
func (b *BlockChain) isBlake3PowAgendaForcedActive() bool {
	const agendaID = chaincfg.VoteIDBlake3Pow
	agenda, ok := b.agendas[agendaID]
	if !ok {
		return false
	}

	state := agenda.forcedState
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
	const agendaID = chaincfg.VoteIDBlake3Pow
	return b.isAgendaActive(prevNode, agendaID)
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
	const agendaID = chaincfg.VoteIDChangeSubsidySplitR2
	return b.isAgendaActive(prevNode, agendaID)
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
	const agendaID = chaincfg.VoteIDMaxTreasurySpend
	return b.isAgendaActive(prevNode, agendaID)
}

// IsMaxTreasurySpendAgendaActive returns whether or not the agenda to change
// the maximum treasury spend to 4% per expenditure policy window, as defined in
// DCP0013, has passed and is now active for the block AFTER the given block.
//
// This function is safe for concurrent access.
func (b *BlockChain) IsMaxTreasurySpendAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	return b.isAgendaActiveByHash(prevHash, b.isMaxTreasurySpendAgendaActive)
}
