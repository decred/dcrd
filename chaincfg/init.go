// Copyright (c) 2017-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"errors"
	"fmt"
	"math/bits"
	"strings"
)

var (
	errDuplicateVoteId   = errors.New("duplicate vote id")
	errInvalidMask       = errors.New("invalid mask")
	errNotConsecutive    = errors.New("choices not consecutive")
	errTooManyChoices    = errors.New("too many choices")
	errInvalidAbstain    = errors.New("invalid abstain bits")
	errInvalidBits       = errors.New("invalid vote bits")
	errMissingAbstain    = errors.New("missing abstain choice")
	errTooManyAbstain    = errors.New("only one choice may have abstain flag")
	errMissingNo         = errors.New("missing no choice")
	errTooManyNo         = errors.New("only one choice may have no flag")
	errBothFlags         = errors.New("abstain and no flags are mutually exclusive")
	errDuplicateChoiceId = errors.New("duplicate choice ID")
)

// consecOnes counts the number of consecutive 1 bits set.
func consecOnes(bits uint16) uint {
	c := uint(0)
	for v := bits; v != 0; c++ {
		v &= (v << 1)
	}
	return c
}

// validateChoices ensures the provided choices conform to the required voting
// choice semantics.
func validateChoices(mask uint16, choices []Choice) error {
	// Ensure the mask only consists of consecutive bits.
	maskPopulationCount := uint(bits.OnesCount16(mask))
	if consecOnes(mask) != maskPopulationCount {
		return errInvalidMask
	}

	// Ensure there are not more choices than the mask bits can represent.
	if len(choices) > 1<<maskPopulationCount {
		return errTooManyChoices
	}

	var numAbstain, numNo int
	dups := make(map[string]struct{})
	s := uint(bits.TrailingZeros16(mask))
	for index, choice := range choices {
		// Ensure that choice 0 is the abstain vote.
		if mask&choice.Bits == 0 && !choice.IsAbstain {
			return errInvalidAbstain
		}

		// Ensure the bits for the choice are covered by the mask.
		if mask&choice.Bits != choice.Bits {
			return errInvalidBits
		}

		// Ensure the index is consecutive.  This test is below the mask check
		// for testing reasons.  Leave it here.
		if uint16(index) != choice.Bits>>s {
			return errNotConsecutive
		}

		// Ensure only one of the choice type identification flags are set.
		if choice.IsAbstain && choice.IsNo {
			return errBothFlags
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
		if _, found := dups[id]; found {
			return errDuplicateChoiceId
		}
		dups[id] = struct{}{}
	}

	// Ensure there is one and only one of each choice type identification flag
	// set.
	switch {
	case numAbstain == 0:
		return errMissingAbstain
	case numAbstain > 1:
		return errTooManyAbstain
	case numNo == 0:
		return errMissingNo
	case numNo > 1:
		return errTooManyNo
	}

	return nil
}

// validateDeployments ensures all of deployments in the provided map adhere to
// the required semantics for deployment definitions.  For example, it ensures
// there are no duplicate vote IDs and that all choices conform to the required
// voting choice semantics.
func validateDeployments(allDeployments map[uint32][]ConsensusDeployment) error {
	for version, deployments := range allDeployments {
		// Ensure there are no duplicate vote IDs across the deployment version.
		dups := make(map[string]struct{})
		for index, deployment := range deployments {
			voteID := strings.ToLower(deployment.Vote.Id)
			if _, found := dups[voteID]; found {
				return fmt.Errorf("version %d deployment index %d id %q: %w",
					version, index, deployment.Vote.Id, errDuplicateVoteId)
			}
			dups[voteID] = struct{}{}
		}
	}

	for version, deployments := range allDeployments {
		for index, deployment := range deployments {
			// Ensure the vote choices conform to all required semantics.
			vote := &deployment.Vote
			err := validateChoices(vote.Mask, vote.Choices)
			if err != nil {
				return fmt.Errorf("version %d deployment index %d id %q: %w",
					version, index, vote.Id, err)
			}
		}
	}

	return nil
}

func init() {
	allParams := []*Params{MainNetParams(), TestNet3Params(), SimNetParams(),
		RegNetParams()}
	for _, params := range allParams {
		if err := validateDeployments(params.Deployments); err != nil {
			panic(fmt.Sprintf("invalid agenda on %s: %v", params.Name, err))
		}
	}
}
