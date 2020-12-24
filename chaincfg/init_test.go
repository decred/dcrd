// Copyright (c) 2017-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"errors"
	"testing"

	"github.com/decred/dcrd/wire"
)

var (
	consecMask = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0xa, // 0b1010 XXX not consecutive mask
		Choices: []Choice{
			{
				Id:          "Abstain",
				Description: "Abstain voting for Pedro",
				Bits:        0x0, // 0b0000
				IsAbstain:   true,
				IsNo:        false,
			},
			{
				Id:          "Yes",
				Description: "Vote for Pedro",
				Bits:        0x2, // 0b0010
				IsAbstain:   false,
				IsNo:        false,
			},
			{
				Id:          "No",
				Description: "Dont vote for Pedro",
				Bits:        0x4, // 0b0100
				IsAbstain:   false,
				IsNo:        true,
			},
		},
	}

	tooManyChoices = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
			{
				Id:          "Abstain",
				Description: "Abstain voting for Pedro",
				Bits:        0x0, // 0b0000
				IsAbstain:   true,
				IsNo:        false,
			},
			{
				Id:          "Yes",
				Description: "Vote for Pedro",
				Bits:        0x2, // 0b0010
				IsAbstain:   false,
				IsNo:        false,
			},
			{
				Id:          "No",
				Description: "Dont vote for Pedro",
				Bits:        0x4, // 0b0100
				IsAbstain:   false,
				IsNo:        true,
			},
			{
				Id:          "Maybe",
				Description: "Vote for Pedro",
				Bits:        0x6, // 0b0110
				IsAbstain:   false,
				IsNo:        false,
			},
			// XXX here we go out of mask bounds
			{
				Id:          "Hmmmm",
				Description: "Vote for Pedro",
				Bits:        0x6, // 0b0110 XXX invalid bits too
				IsAbstain:   false,
				IsNo:        false,
			},
		},
	}

	notConsec = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
			{
				Id:          "Abstain",
				Description: "Abstain voting for Pedro",
				Bits:        0x0, // 0b0000
				IsAbstain:   true,
				IsNo:        false,
			},
			{
				Id:          "Yes",
				Description: "Vote for Pedro",
				Bits:        0x2, // 0b0010
				IsAbstain:   false,
				IsNo:        false,
			},
			// XXX not consecutive
			{
				Id:          "Maybe",
				Description: "Vote for Pedro",
				Bits:        0x6, // 0b0110
				IsAbstain:   false,
				IsNo:        false,
			},
		},
	}

	invalidAbstain = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
			{
				Id:          "Abstain",
				Description: "Abstain voting for Pedro",
				Bits:        0x0,   // 0b0000
				IsAbstain:   false, // XXX this is the invalid bit
				IsNo:        false,
			},
			{
				Id:          "Yes",
				Description: "Vote for Pedro",
				Bits:        0x2, // 0b0010
				IsAbstain:   false,
				IsNo:        false,
			},
			{
				Id:          "No",
				Description: "Dont vote for Pedro",
				Bits:        0x4, // 0b0100
				IsAbstain:   false,
				IsNo:        true,
			},
		},
	}

	invalidVoteBits = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
			{
				Id:          "Abstain",
				Description: "Abstain voting for Pedro",
				Bits:        0x0, // 0b0000
				IsAbstain:   true,
				IsNo:        false,
			},
			{
				Id:          "Yes",
				Description: "Vote for Pedro",
				Bits:        0x2, // 0b0010
				IsAbstain:   false,
				IsNo:        false,
			},
			{
				Id:          "No",
				Description: "Dont vote for Pedro",
				Bits:        0xc, // 0b1100 XXX invalid bits
				IsAbstain:   false,
				IsNo:        true,
			},
		},
	}

	twoIsAbstain = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
			{
				Id:          "Abstain",
				Description: "Abstain voting for Pedro",
				Bits:        0x0, // 0b0000
				IsAbstain:   true,
				IsNo:        false,
			},
			{
				Id:          "Yes",
				Description: "Vote for Pedro",
				Bits:        0x2,  // 0b0010
				IsAbstain:   true, // XXX this is the invalid choice
				IsNo:        false,
			},
			{
				Id:          "No",
				Description: "Dont vote for Pedro",
				Bits:        0x4, // 0b0100
				IsAbstain:   false,
				IsNo:        true,
			},
		},
	}

	twoIsNo = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
			{
				Id:          "Abstain",
				Description: "Abstain voting for Pedro",
				Bits:        0x0, // 0b0000
				IsAbstain:   true,
				IsNo:        false,
			},
			{
				Id:          "Yes",
				Description: "Vote for Pedro",
				Bits:        0x2, // 0b0010
				IsAbstain:   false,
				IsNo:        true, // XXX this is the invalid choice
			},
			{
				Id:          "No",
				Description: "Dont vote for Pedro",
				Bits:        0x4, // 0b0100
				IsAbstain:   false,
				IsNo:        true,
			},
		},
	}

	bothFlags = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
			{
				Id:          "Abstain",
				Description: "Abstain voting for Pedro",
				Bits:        0x0, // 0b0000
				IsAbstain:   true,
				IsNo:        false,
			},
			{
				Id:          "Yes",
				Description: "Vote for Pedro",
				Bits:        0x2,  // 0b0010
				IsAbstain:   true, // XXX this is the invalid choice
				IsNo:        true, // XXX this is the invalid choice
			},
			{
				Id:          "No",
				Description: "Dont vote for Pedro",
				Bits:        0x4, // 0b0100
				IsAbstain:   false,
				IsNo:        true,
			},
		},
	}

	dupChoice = Vote{
		Id:          "voteforpedro",
		Description: "You should always vote for Pedro",
		Mask:        0x6, // 0b0110
		Choices: []Choice{
			{
				Id:          "Abstain",
				Description: "Abstain voting for Pedro",
				Bits:        0x0, // 0b0000
				IsAbstain:   true,
				IsNo:        false,
			},
			{
				Id:          "Yes",
				Description: "Vote for Pedro",
				Bits:        0x2, // 0b0010
				IsAbstain:   false,
				IsNo:        false,
			},
			{
				Id:          "Yes", // XXX this is the invalid ID
				Description: "Dont vote for Pedro",
				Bits:        0x4, // 0b0100
				IsAbstain:   false,
				IsNo:        true,
			},
		},
	}
)

func TestChoices(t *testing.T) {
	tests := []struct {
		name     string
		vote     Vote
		expected error
	}{
		{
			name:     "consecutive mask",
			vote:     consecMask,
			expected: errInvalidMask,
		},
		{
			name:     "not consecutive choices",
			vote:     notConsec,
			expected: errNotConsecutive,
		},
		{
			name:     "too many choices",
			vote:     tooManyChoices,
			expected: errTooManyChoices,
		},
		{
			name:     "invalid ignore",
			vote:     invalidAbstain,
			expected: errInvalidAbstain,
		},
		{
			name:     "invalid vote bits",
			vote:     invalidVoteBits,
			expected: errInvalidBits,
		},
		{
			name:     "2 IsAbstain",
			vote:     twoIsAbstain,
			expected: errInvalidIsAbstain,
		},
		{
			name:     "2 IsNo",
			vote:     twoIsNo,
			expected: errInvalidIsNo,
		},
		{
			name:     "both IsAbstain IsNo",
			vote:     bothFlags,
			expected: errInvalidBothFlags,
		},
		{
			name:     "duplicate choice id",
			vote:     dupChoice,
			expected: errDuplicateChoiceId,
		},
	}

	for _, test := range tests {
		t.Logf("running: %v", test.name)
		err := validateAgenda(test.vote)
		if !errors.Is(err, test.expected) {
			t.Fatalf("%v: got '%v' expected '%v'", test.name, err,
				test.expected)
		}
	}
}

var (
	dupVote = []ConsensusDeployment{
		{Vote: Vote{Id: "moo"}},
		{Vote: Vote{Id: "moo"}},
	}
)

func TestDeployments(t *testing.T) {
	tests := []struct {
		name        string
		deployments []ConsensusDeployment
		expected    error
	}{
		{
			name:        "duplicate vote id",
			deployments: dupVote,
			expected:    errDuplicateVoteId,
		},
	}

	for _, test := range tests {
		t.Logf("running: %v", test.name)
		_, err := validateDeployments(test.deployments)
		if !errors.Is(err, test.expected) {
			t.Fatalf("%v: got '%v' expected '%v'", test.name, err,
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
