// Copyright (c) 2017-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"errors"
	"testing"

	"github.com/decred/dcrd/wire"
)

// mockVote is a convenience func to return a mock vote that is used as a base
// throughout the tests.
func mockVote() Vote {
	return Vote{
		Id:   "testvote",
		Mask: 0x6,
		Choices: []Choice{{
			Id:          "abstain",
			Description: "abstain voting for change",
			Bits:        0x0000,
			IsAbstain:   true,
			IsNo:        false,
		}, {
			Id:          "no",
			Description: "keep the existing rules",
			Bits:        0x0002, // Bit 1
			IsAbstain:   false,
			IsNo:        true,
		}, {
			Id:          "yes",
			Description: "change to the new rules",
			Bits:        0x0004, // Bit 2
			IsAbstain:   false,
			IsNo:        false,
		}},
	}
}

// TestValidateChoices ensures the validation logic for enforcing choice
// semantics works as intended.
func TestValidateChoices(t *testing.T) {
	tests := []struct {
		name     string
		munger   func(*Vote)
		expected error
	}{{
		name: "require consecutive mask",
		munger: func(vote *Vote) {
			vote.Mask = 0xa // 0b1010 -- non-consecutive mask
		},
		expected: errInvalidMask,
	}, {
		name: "require consecutive choices",
		munger: func(vote *Vote) {
			// The other choices in the mock vote are for abstain and yes, so
			// the next one should be for bit 2 (0x0004).
			vote.Choices[2].Bits = 0x6
		},
		expected: errNotConsecutive,
	}, {
		name: "reject too many choices",
		munger: func(vote *Vote) {
			vote.Choices = append(vote.Choices, Choice{
				Id:   "maybe",
				Bits: 0x0006,
			})
			vote.Choices = append(vote.Choices, Choice{
				Id:   "another",
				Bits: 0x0006, // invalid bits too
			})
		},
		expected: errTooManyChoices,
	}, {
		name: "reject choice with abstain bits but no abstain flag",
		munger: func(vote *Vote) {
			// Mark the abstain choice as not being abstain.
			vote.Choices[0].IsAbstain = false
		},
		expected: errInvalidAbstain,
	}, {
		name: "reject choice with bits that do not conform to the mask",
		munger: func(vote *Vote) {
			// Assign invalid bits per the mask to a choice.
			vote.Choices[2].Bits = 0xc // 0b1100
		},
		expected: errInvalidBits,
	}, {
		name: "require an abstain choice",
		munger: func(vote *Vote) {
			// Remove all choices.
			vote.Choices = nil
		},
		expected: errMissingAbstain,
	}, {
		name: "reject more than one abstain choice",
		munger: func(vote *Vote) {
			// Mark a second choice as abstain.
			vote.Choices[1].IsAbstain = true
			vote.Choices[1].IsNo = false
		},
		expected: errTooManyAbstain,
	}, {
		name: "require a no choice",
		munger: func(vote *Vote) {
			// Replace the no choice with the yes choice.
			vote.Choices = vote.Choices[:2]
			vote.Choices[1].Id = "yes"
			vote.Choices[1].IsNo = false
		},
		expected: errMissingNo,
	}, {
		name: "reject more than one no choice",
		munger: func(vote *Vote) {
			// Mark a second choice as no.
			vote.Choices[2].IsNo = true
		},
		expected: errTooManyNo,
	}, {
		name: "require exclusive abstain and no flags",
		munger: func(vote *Vote) {
			// Mark a choice with both flags.
			vote.Choices[1].IsAbstain = true
			vote.Choices[1].IsNo = true
		},
		expected: errBothFlags,
	}, {
		name: "require unique choice ids",
		munger: func(vote *Vote) {
			// Assign duplicate id.
			vote.Choices[2].Id = vote.Choices[1].Id
		},
		expected: errDuplicateChoiceId,
	}}

	for _, test := range tests {
		// Create the vote and mutate it per the test.
		vote := mockVote()
		if test.munger != nil {
			test.munger(&vote)
		}

		err := validateChoices(vote.Mask, vote.Choices)
		if !errors.Is(err, test.expected) {
			t.Fatalf("%q: unexpected err -- got %v, want %v", test.name, err,
				test.expected)
		}
	}
}

// TestRejectDuplicateVoteIDs ensures the validation logic for enforcing unique
// deployment/vote IDs across all deployments works as intended.
func TestRejectDuplicateVoteIDs(t *testing.T) {
	tests := []struct {
		name        string
		deployments map[uint32][]ConsensusDeployment
		expected    error
	}{{
		name: "reject duplicate vote id in the same version",
		deployments: map[uint32][]ConsensusDeployment{
			1: {
				{Vote: mockVote()},
				{Vote: mockVote()},
			},
		},
		expected: errDuplicateVoteId,
	}, {
		name: "reject duplicate vote id in different versions",
		deployments: map[uint32][]ConsensusDeployment{
			1: {{Vote: mockVote()}},
			2: {{Vote: mockVote()}},
		},
		expected: errDuplicateVoteId,
	}}

	for _, test := range tests {
		err := validateDeployments(test.deployments)
		if !errors.Is(err, test.expected) {
			t.Fatalf("%q: unexpected err -- got %v, want %v", test.name, err,
				test.expected)
		}
	}
}

// allDefaultNetParams returns the parameters for all of the default networks
// for use in the tests.
func allDefaultNetParams() []*Params {
	return []*Params{MainNetParams(), TestNet3Params(), SimNetParams(),
		RegNetParams()}
}

// TestRequiredUnique ensures that the network parameter fields that are
// required to be unique are in fact unique for all of the provided default
// parameters.
func TestRequiredUnique(t *testing.T) {
	var (
		netMagics         = make(map[wire.CurrencyNet]struct{})
		netPrefixes       = make(map[string]struct{})
		pubKeyAddrIDs     = make(map[[2]byte]struct{})
		pubKeyHashAddrIDs = make(map[[2]byte]struct{})
		pkhEdwardsAddrIDs = make(map[[2]byte]struct{})
		pkhSchnorrAddrIDs = make(map[[2]byte]struct{})
		scriptHashAddrIDs = make(map[[2]byte]struct{})
	)

	for _, params := range allDefaultNetParams() {
		if _, ok := netMagics[params.Net]; ok {
			t.Fatalf("%q: duplicate network magic %x", params.Name,
				params.Net)
		}
		netMagics[params.Net] = struct{}{}

		if _, ok := netPrefixes[params.NetworkAddressPrefix]; ok {
			t.Fatalf("%q: duplicate network address prefix %s", params.Name,
				params.NetworkAddressPrefix)
		}
		netPrefixes[params.Name] = struct{}{}

		if _, ok := pubKeyAddrIDs[params.PubKeyAddrID]; ok {
			t.Fatalf("%q: duplicate pubkey addr ID %x", params.Name,
				params.PubKeyAddrID)
		}
		pubKeyAddrIDs[params.PubKeyAddrID] = struct{}{}

		if _, ok := pubKeyHashAddrIDs[params.PubKeyHashAddrID]; ok {
			t.Fatalf("%q: duplicate pubkey hash addr ID %x", params.Name,
				params.PubKeyHashAddrID)
		}
		pubKeyHashAddrIDs[params.PubKeyHashAddrID] = struct{}{}

		if _, ok := pkhEdwardsAddrIDs[params.PKHEdwardsAddrID]; ok {
			t.Fatalf("%q: duplicate edwards pubkey hash addr ID %x",
				params.Name, params.PKHEdwardsAddrID)
		}
		pkhEdwardsAddrIDs[params.PKHEdwardsAddrID] = struct{}{}

		if _, ok := pkhSchnorrAddrIDs[params.PKHSchnorrAddrID]; ok {
			t.Fatalf("%q: duplicate schnorr pubkey hash addr ID %x",
				params.Name, params.PKHSchnorrAddrID)
		}
		pkhSchnorrAddrIDs[params.PKHSchnorrAddrID] = struct{}{}

		if _, ok := scriptHashAddrIDs[params.ScriptHashAddrID]; ok {
			t.Fatalf("%q: duplicate script hash addr ID %x", params.Name,
				params.ScriptHashAddrID)
		}
		scriptHashAddrIDs[params.ScriptHashAddrID] = struct{}{}
	}
}
