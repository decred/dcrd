// Copyright (c) 2019-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package standalone

import (
	"testing"
)

const (
	// noTreasury signifies the treasury agenda should be treated as though
	// it is inactive.  It is used to increase the readability of the
	// tests.
	noTreasury = false

	// withTreasury signifies the treasury agenda should be treated as
	// though it is active.  It is used to increase the readability of
	// the tests.
	withTreasury = true
)

// mockSubsidyParams implements the SubsidyParams interface and is used
// throughout the tests to mock networks.
type mockSubsidyParams struct {
	blockOne              int64
	baseSubsidy           int64
	reductionMultiplier   int64
	reductionDivisor      int64
	reductionInterval     int64
	workProportion        uint16
	voteProportion        uint16
	treasuryProportion    uint16
	stakeValidationHeight int64
	votesPerBlock         uint16
}

// Ensure the mock subsidy params satisfy the SubsidyParams interface.
var _ SubsidyParams = (*mockSubsidyParams)(nil)

// BlockOneSubsidy returns the value associated with the mock params for the
// total subsidy of block height 1 for the network.
//
// This is part of the SubsidyParams interface.
func (p *mockSubsidyParams) BlockOneSubsidy() int64 {
	return p.blockOne
}

// BaseSubsidyValue returns the value associated with the mock params for the
// starting base max potential subsidy amount for mined blocks.
//
// This is part of the SubsidyParams interface.
func (p *mockSubsidyParams) BaseSubsidyValue() int64 {
	return p.baseSubsidy
}

// SubsidyReductionMultiplier returns the value associated with the mock params
// for the multiplier to use when performing the exponential subsidy reduction
// described by the CalcBlockSubsidy documentation.
//
// This is part of the SubsidyParams interface.
func (p *mockSubsidyParams) SubsidyReductionMultiplier() int64 {
	return p.reductionMultiplier
}

// SubsidyReductionDivisor returns the value associated with the mock params for
// the divisor to use when performing the exponential subsidy reduction
// described by the CalcBlockSubsidy documentation.
//
// This is part of the SubsidyParams interface.
func (p *mockSubsidyParams) SubsidyReductionDivisor() int64 {
	return p.reductionDivisor
}

// SubsidyReductionIntervalBlocks returns the value associated with the mock
// params for the reduction interval in number of blocks.
//
// This is part of the SubsidyParams interface.
func (p *mockSubsidyParams) SubsidyReductionIntervalBlocks() int64 {
	return p.reductionInterval
}

// WorkSubsidyProportion returns the value associated with the mock params for
// the comparative proportion of the subsidy generated for creating a block
// (PoW).
//
// This is part of the SubsidyParams interface.
func (p *mockSubsidyParams) WorkSubsidyProportion() uint16 {
	return p.workProportion
}

// StakeSubsidyProportion returns the value associated with the mock params for
// the comparative proportion of the subsidy generated for casting stake votes
// (collectively, per block).
//
// This is part of the SubsidyParams interface.
func (p *mockSubsidyParams) StakeSubsidyProportion() uint16 {
	return p.voteProportion
}

// TreasurySubsidyProportion returns the value associated with the mock params
// for the comparative proportion of the subsidy allocated to the project
// treasury.
//
// This is part of the SubsidyParams interface.
func (p *mockSubsidyParams) TreasurySubsidyProportion() uint16 {
	return p.treasuryProportion
}

// StakeValidationBeginHeight returns the value associated with the mock params
// for the height at which votes become required to extend a block.
//
// This is part of the SubsidyParams interface.
func (p *mockSubsidyParams) StakeValidationBeginHeight() int64 {
	return p.stakeValidationHeight
}

// VotesPerBlock returns the value associated with the mock params for the
// maximum number of votes a block must contain to receive full subsidy once
// voting begins at StakeValidationBeginHeight
//
// This is part of the SubsidyParams interface.
func (p *mockSubsidyParams) VotesPerBlock() uint16 {
	return p.votesPerBlock
}

// mockMainNetParams returns mock mainnet subsidy parameters to use throughout
// the tests.  They match the Decred mainnet params as of the time this comment
// was written.
func mockMainNetParams() *mockSubsidyParams {
	return &mockSubsidyParams{
		blockOne:              168000000000000,
		baseSubsidy:           3119582664,
		reductionMultiplier:   100,
		reductionDivisor:      101,
		reductionInterval:     6144,
		workProportion:        6,
		voteProportion:        3,
		treasuryProportion:    1,
		stakeValidationHeight: 4096,
		votesPerBlock:         5,
	}
}

// TestSubsidyCacheCalcs ensures the subsidy cache calculates the various
// subsidy proportions and values as expected.
func TestSubsidyCacheCalcs(t *testing.T) {
	// Mock params used in tests.
	mockMainNetParams := mockMainNetParams()

	tests := []struct {
		name         string        // test description
		params       SubsidyParams // params to use in subsidy calculations
		height       int64         // height to calculate subsidy for
		numVotes     uint16        // number of votes
		wantFull     int64         // expected full block subsidy
		wantWork     int64         // expected pow subsidy
		wantVote     int64         // expected single vote subsidy
		wantTreasury int64         // expected treasury subsidy
		useDCP0010   bool          // use subsidy split defined in DCP0010
	}{{
		name:         "negative height",
		params:       mockMainNetParams,
		height:       -1,
		numVotes:     0,
		wantFull:     0,
		wantWork:     0,
		wantVote:     0,
		wantTreasury: 0,
	}, {
		name:         "negative height, use DCP0010",
		params:       mockMainNetParams,
		height:       -1,
		numVotes:     0,
		wantFull:     0,
		wantWork:     0,
		wantVote:     0,
		wantTreasury: 0,
		useDCP0010:   true,
	}, {
		name:         "height 0",
		params:       mockMainNetParams,
		height:       0,
		numVotes:     0,
		wantFull:     0,
		wantWork:     0,
		wantVote:     0,
		wantTreasury: 0,
	}, {
		name:         "height 0, use DCP0010",
		params:       mockMainNetParams,
		height:       0,
		numVotes:     0,
		wantFull:     0,
		wantWork:     0,
		wantVote:     0,
		wantTreasury: 0,
		useDCP0010:   true,
	}, {
		name:         "height 1 (initial payouts)",
		params:       mockMainNetParams,
		height:       1,
		numVotes:     0,
		wantFull:     168000000000000,
		wantWork:     168000000000000,
		wantVote:     0,
		wantTreasury: 0,
	}, {
		name:         "height 1 (initial payouts), use DCP0010",
		params:       mockMainNetParams,
		height:       1,
		numVotes:     0,
		wantFull:     168000000000000,
		wantWork:     168000000000000,
		wantVote:     0,
		wantTreasury: 0,
		useDCP0010:   true,
	}, {
		name:         "height 2 (first non-special block prior voting start)",
		params:       mockMainNetParams,
		height:       2,
		numVotes:     0,
		wantFull:     3119582664,
		wantWork:     1871749598,
		wantVote:     0,
		wantTreasury: 311958266,
	}, {
		name:         "height 2 (first non-special block prior voting start), use DCP0010",
		params:       mockMainNetParams,
		height:       2,
		numVotes:     0,
		wantFull:     3119582664,
		wantWork:     311958266,
		wantVote:     0,
		wantTreasury: 311958266,
		useDCP0010:   true,
	}, {
		name:         "height 4094 (two blocks prior to voting start)",
		params:       mockMainNetParams,
		height:       4094,
		numVotes:     0,
		wantFull:     3119582664,
		wantWork:     1871749598,
		wantVote:     0,
		wantTreasury: 311958266,
	}, {
		name:         "height 4094 (two blocks prior to voting start), use DCP0010",
		params:       mockMainNetParams,
		height:       4094,
		numVotes:     0,
		wantFull:     3119582664,
		wantWork:     311958266,
		wantVote:     0,
		wantTreasury: 311958266,
		useDCP0010:   true,
	}, {
		name:         "height 4095 (final block prior to voting start)",
		params:       mockMainNetParams,
		height:       4095,
		numVotes:     0,
		wantFull:     3119582664,
		wantWork:     1871749598,
		wantVote:     187174959,
		wantTreasury: 311958266,
	}, {
		name:         "height 4095 (final block prior to voting start), use DCP0010",
		params:       mockMainNetParams,
		height:       4095,
		numVotes:     0,
		wantFull:     3119582664,
		wantWork:     311958266,
		wantVote:     499133226,
		wantTreasury: 311958266,
		useDCP0010:   true,
	}, {
		name:         "height 4096 (voting start), 5 votes",
		params:       mockMainNetParams,
		height:       4096,
		numVotes:     5,
		wantFull:     3119582664,
		wantWork:     1871749598,
		wantVote:     187174959,
		wantTreasury: 311958266,
	}, {
		name:         "height 4096 (voting start), 5 votes, use DCP0010",
		params:       mockMainNetParams,
		height:       4096,
		numVotes:     5,
		wantFull:     3119582664,
		wantWork:     311958266,
		wantVote:     499133226,
		wantTreasury: 311958266,
		useDCP0010:   true,
	}, {
		name:         "height 4096 (voting start), 4 votes",
		params:       mockMainNetParams,
		height:       4096,
		numVotes:     4,
		wantFull:     3119582664,
		wantWork:     1497399678,
		wantVote:     187174959,
		wantTreasury: 249566612,
	}, {
		name:         "height 4096 (voting start), 4 votes, use DCP0010",
		params:       mockMainNetParams,
		height:       4096,
		numVotes:     4,
		wantFull:     3119582664,
		wantWork:     249566612,
		wantVote:     499133226,
		wantTreasury: 249566612,
		useDCP0010:   true,
	}, {
		name:         "height 4096 (voting start), 3 votes",
		params:       mockMainNetParams,
		height:       4096,
		numVotes:     3,
		wantFull:     3119582664,
		wantWork:     1123049758,
		wantVote:     187174959,
		wantTreasury: 187174959,
	}, {
		name:         "height 4096 (voting start), 3 votes, use DCP0010",
		params:       mockMainNetParams,
		height:       4096,
		numVotes:     3,
		wantFull:     3119582664,
		wantWork:     187174959,
		wantVote:     499133226,
		wantTreasury: 187174959,
		useDCP0010:   true,
	}, {
		name:         "height 4096 (voting start), 2 votes",
		params:       mockMainNetParams,
		height:       4096,
		numVotes:     2,
		wantFull:     3119582664,
		wantWork:     0,
		wantVote:     187174959,
		wantTreasury: 0,
	}, {
		name:         "height 4096 (voting start), 2 votes, use DCP0010",
		params:       mockMainNetParams,
		height:       4096,
		numVotes:     2,
		wantFull:     3119582664,
		wantWork:     0,
		wantVote:     499133226,
		wantTreasury: 0,
		useDCP0010:   true,
	}, {
		name:         "height 6143 (final block prior to 1st reduction), 5 votes",
		params:       mockMainNetParams,
		height:       6143,
		numVotes:     5,
		wantFull:     3119582664,
		wantWork:     1871749598,
		wantVote:     187174959,
		wantTreasury: 311958266,
	}, {
		name:         "height 6143 (final block prior to 1st reduction), 5 votes, use DCP0010",
		params:       mockMainNetParams,
		height:       6143,
		numVotes:     5,
		wantFull:     3119582664,
		wantWork:     311958266,
		wantVote:     499133226,
		wantTreasury: 311958266,
		useDCP0010:   true,
	}, {
		name:         "height 6144 (1st block in 1st reduction), 5 votes",
		params:       mockMainNetParams,
		height:       6144,
		numVotes:     5,
		wantFull:     3088695706,
		wantWork:     1853217423,
		wantVote:     185321742,
		wantTreasury: 308869570,
	}, {
		name:         "height 6144 (1st block in 1st reduction), 5 votes, use DCP0010",
		params:       mockMainNetParams,
		height:       6144,
		numVotes:     5,
		wantFull:     3088695706,
		wantWork:     308869570,
		wantVote:     494191312,
		wantTreasury: 308869570,
		useDCP0010:   true,
	}, {
		name:         "height 6144 (1st block in 1st reduction), 4 votes",
		params:       mockMainNetParams,
		height:       6144,
		numVotes:     4,
		wantFull:     3088695706,
		wantWork:     1482573938,
		wantVote:     185321742,
		wantTreasury: 247095656,
	}, {
		name:         "height 6144 (1st block in 1st reduction), 4 votes, use DCP0010",
		params:       mockMainNetParams,
		height:       6144,
		numVotes:     4,
		wantFull:     3088695706,
		wantWork:     247095656,
		wantVote:     494191312,
		wantTreasury: 247095656,
		useDCP0010:   true,
	}, {
		name:         "height 12287 (last block in 1st reduction), 5 votes",
		params:       mockMainNetParams,
		height:       12287,
		numVotes:     5,
		wantFull:     3088695706,
		wantWork:     1853217423,
		wantVote:     185321742,
		wantTreasury: 308869570,
	}, {
		name:         "height 12287 (last block in 1st reduction), 5 votes, use DCP0010",
		params:       mockMainNetParams,
		height:       12287,
		numVotes:     5,
		wantFull:     3088695706,
		wantWork:     308869570,
		wantVote:     494191312,
		wantTreasury: 308869570,
		useDCP0010:   true,
	}, {
		name:         "height 12288 (1st block in 2nd reduction), 5 votes",
		params:       mockMainNetParams,
		height:       12288,
		numVotes:     5,
		wantFull:     3058114560,
		wantWork:     1834868736,
		wantVote:     183486873,
		wantTreasury: 305811456,
	}, {
		name:         "height 12288 (1st block in 2nd reduction), 5 votes, use DCP0010",
		params:       mockMainNetParams,
		height:       12288,
		numVotes:     5,
		wantFull:     3058114560,
		wantWork:     305811456,
		wantVote:     489298329,
		wantTreasury: 305811456,
		useDCP0010:   true,
	}, {
		name:         "height 307200 (1st block in 50th reduction), 5 votes",
		params:       mockMainNetParams,
		height:       307200,
		numVotes:     5,
		wantFull:     1896827356,
		wantWork:     1138096413,
		wantVote:     113809641,
		wantTreasury: 189682735,
	}, {
		name:         "height 307200 (1st block in 50th reduction), 5 votes, use DCP0010",
		params:       mockMainNetParams,
		height:       307200,
		numVotes:     5,
		wantFull:     1896827356,
		wantWork:     189682735,
		wantVote:     303492376,
		wantTreasury: 189682735,
		useDCP0010:   true,
	}, {
		name:         "height 307200 (1st block in 50th reduction), 3 votes",
		params:       mockMainNetParams,
		height:       307200,
		numVotes:     3,
		wantFull:     1896827356,
		wantWork:     682857847,
		wantVote:     113809641,
		wantTreasury: 113809641,
	}, {
		name:         "height 307200 (1st block in 50th reduction), 3 votes, use DCP0010",
		params:       mockMainNetParams,
		height:       307200,
		numVotes:     3,
		wantFull:     1896827356,
		wantWork:     113809641,
		wantVote:     303492376,
		wantTreasury: 113809641,
		useDCP0010:   true,
	}, {
		name:         "height 10911744 (first zero vote subsidy 1776th reduction), 5 votes",
		params:       mockMainNetParams,
		height:       10911744,
		numVotes:     5,
		wantFull:     16,
		wantWork:     9,
		wantVote:     0,
		wantTreasury: 1,
	}, {
		name:         "height 10954752 (first zero treasury subsidy 1783rd reduction), 5 votes",
		params:       mockMainNetParams,
		height:       10954752,
		numVotes:     5,
		wantFull:     9,
		wantWork:     5,
		wantVote:     0,
		wantTreasury: 0,
	}, {
		name:         "height 10954752 (first zero work subsidy with DCP0010 1783rd reduction), 5 votes",
		params:       mockMainNetParams,
		height:       10954752,
		numVotes:     5,
		wantFull:     9,
		wantWork:     0,
		wantVote:     1,
		wantTreasury: 0,
		useDCP0010:   true,
	}, {
		name:         "height 10973184 (first zero vote subsidy with DCP0010 1786th reduction), 5 votes",
		params:       mockMainNetParams,
		height:       10973184,
		numVotes:     5,
		wantFull:     6,
		wantWork:     0,
		wantVote:     0,
		wantTreasury: 0,
		useDCP0010:   true,
	}, {
		name:         "height 11003904 (first zero work subsidy 1791st reduction), 5 votes",
		params:       mockMainNetParams,
		height:       11003904,
		numVotes:     5,
		wantFull:     1,
		wantWork:     0,
		wantVote:     0,
		wantTreasury: 0,
	}, {
		name:         "height 11010048 (first zero full subsidy 1792nd reduction), 5 votes",
		params:       mockMainNetParams,
		height:       11010048,
		numVotes:     5,
		wantFull:     0,
		wantWork:     0,
		wantVote:     0,
		wantTreasury: 0,
	}, {
		name:         "height 11010048 (first zero full subsidy 1792nd reduction), 5 votes, use DCP0010",
		params:       mockMainNetParams,
		height:       11010048,
		numVotes:     5,
		wantFull:     0,
		wantWork:     0,
		wantVote:     0,
		wantTreasury: 0,
		useDCP0010:   true,
	}}

	for _, test := range tests {
		// Ensure the full subsidy is the expected value.
		cache := NewSubsidyCache(test.params)
		fullSubsidyResult := cache.CalcBlockSubsidy(test.height)
		if fullSubsidyResult != test.wantFull {
			t.Errorf("%s: unexpected full subsidy result -- got %d, want %d",
				test.name, fullSubsidyResult, test.wantFull)
			continue
		}

		// Ensure the PoW subsidy is the expected value.
		workResult := cache.CalcWorkSubsidyV2(test.height, test.numVotes,
			test.useDCP0010)
		if workResult != test.wantWork {
			t.Errorf("%s: unexpected work subsidy result -- got %d, want %d",
				test.name, workResult, test.wantWork)
			continue
		}

		// Ensure the vote subsidy is the expected value.
		voteResult := cache.CalcStakeVoteSubsidyV2(test.height, test.useDCP0010)
		if voteResult != test.wantVote {
			t.Errorf("%s: unexpected vote subsidy result -- got %d, want %d",
				test.name, voteResult, test.wantVote)
			continue
		}

		// Ensure the treasury subsidy is the expected value when the treasury
		// agenda is not active.
		treasuryResult := cache.CalcTreasurySubsidy(test.height, test.numVotes,
			noTreasury)
		if treasuryResult != test.wantTreasury {
			t.Errorf("%s: unexpected treasury subsidy result -- got %d, want %d",
				test.name, treasuryResult, test.wantTreasury)
			continue
		}
	}
}

// TestSubsidyCacheCalcsTreasury ensures the subsidy cache calculates the
// various subsidy proportions and values as expected.
func TestSubsidyCacheCalcsTreasury(t *testing.T) {
	// Mock params used in tests.
	mockMainNetParams := mockMainNetParams()

	tests := []struct {
		name         string        // test description
		params       SubsidyParams // params to use in subsidy calculations
		height       int64         // height to calculate subsidy for
		numVotes     uint16        // number of votes
		wantFull     int64         // expected full block subsidy
		wantWork     int64         // expected pow subsidy
		wantVote     int64         // expected single vote subsidy
		wantTreasury int64         // expected treasury subsidy
	}{{
		name:         "negative height",
		params:       mockMainNetParams,
		height:       -1,
		numVotes:     0,
		wantFull:     0,
		wantWork:     0,
		wantVote:     0,
		wantTreasury: 0,
	}, {
		name:         "height 0",
		params:       mockMainNetParams,
		height:       0,
		numVotes:     0,
		wantFull:     0,
		wantWork:     0,
		wantVote:     0,
		wantTreasury: 0,
	}, {
		name:         "height 1 (initial payouts)",
		params:       mockMainNetParams,
		height:       1,
		numVotes:     0,
		wantFull:     168000000000000,
		wantWork:     168000000000000,
		wantVote:     0,
		wantTreasury: 0,
	}, {
		name:         "height 2 (first non-special block prior voting start)",
		params:       mockMainNetParams,
		height:       2,
		numVotes:     0,
		wantFull:     3119582664,
		wantWork:     1871749598,
		wantVote:     0,
		wantTreasury: 311958266,
	}, {
		name:         "height 4094 (two blocks prior to voting start)",
		params:       mockMainNetParams,
		height:       4094,
		numVotes:     0,
		wantFull:     3119582664,
		wantWork:     1871749598,
		wantVote:     0,
		wantTreasury: 311958266,
	}, {
		name:         "height 4095 (final block prior to voting start)",
		params:       mockMainNetParams,
		height:       4095,
		numVotes:     0,
		wantFull:     3119582664,
		wantWork:     1871749598,
		wantVote:     187174959,
		wantTreasury: 311958266,
	}, {
		name:         "height 4096 (voting start), 5 votes",
		params:       mockMainNetParams,
		height:       4096,
		numVotes:     5,
		wantFull:     3119582664,
		wantWork:     1871749598,
		wantVote:     187174959,
		wantTreasury: 311958266,
	}, {
		name:         "height 4096 (voting start), 4 votes",
		params:       mockMainNetParams,
		height:       4096,
		numVotes:     4,
		wantFull:     3119582664,
		wantWork:     1497399678,
		wantVote:     187174959,
		wantTreasury: 311958266, // No reduction
	}, {
		name:         "height 4096 (voting start), 3 votes",
		params:       mockMainNetParams,
		height:       4096,
		numVotes:     3,
		wantFull:     3119582664,
		wantWork:     1123049758,
		wantVote:     187174959,
		wantTreasury: 311958266, // No reduction
	}, {
		name:         "height 4096 (voting start), 2 votes",
		params:       mockMainNetParams,
		height:       4096,
		numVotes:     2,
		wantFull:     3119582664,
		wantWork:     0,
		wantVote:     187174959,
		wantTreasury: 0,
	}, {
		name:         "height 6143 (final block prior to 1st reduction), 5 votes",
		params:       mockMainNetParams,
		height:       6143,
		numVotes:     5,
		wantFull:     3119582664,
		wantWork:     1871749598,
		wantVote:     187174959,
		wantTreasury: 311958266,
	}, {
		name:         "height 6144 (1st block in 1st reduction), 5 votes",
		params:       mockMainNetParams,
		height:       6144,
		numVotes:     5,
		wantFull:     3088695706,
		wantWork:     1853217423,
		wantVote:     185321742,
		wantTreasury: 308869570,
	}, {
		name:         "height 6144 (1st block in 1st reduction), 4 votes",
		params:       mockMainNetParams,
		height:       6144,
		numVotes:     4,
		wantFull:     3088695706,
		wantWork:     1482573938,
		wantVote:     185321742,
		wantTreasury: 308869570, // No reduction
	}, {
		name:         "height 12287 (last block in 1st reduction), 5 votes",
		params:       mockMainNetParams,
		height:       12287,
		numVotes:     5,
		wantFull:     3088695706,
		wantWork:     1853217423,
		wantVote:     185321742,
		wantTreasury: 308869570,
	}, {
		name:         "height 12288 (1st block in 2nd reduction), 5 votes",
		params:       mockMainNetParams,
		height:       12288,
		numVotes:     5,
		wantFull:     3058114560,
		wantWork:     1834868736,
		wantVote:     183486873,
		wantTreasury: 305811456,
	}, {
		name:         "height 307200 (1st block in 50th reduction), 5 votes",
		params:       mockMainNetParams,
		height:       307200,
		numVotes:     5,
		wantFull:     1896827356,
		wantWork:     1138096413,
		wantVote:     113809641,
		wantTreasury: 189682735,
	}, {
		name:         "height 307200 (1st block in 50th reduction), 3 votes",
		params:       mockMainNetParams,
		height:       307200,
		numVotes:     3,
		wantFull:     1896827356,
		wantWork:     682857847,
		wantVote:     113809641,
		wantTreasury: 189682735, // No reduction
	}, {
		name:         "height 10911744 (first zero vote subsidy 1776th reduction), 5 votes",
		params:       mockMainNetParams,
		height:       10911744,
		numVotes:     5,
		wantFull:     16,
		wantWork:     9,
		wantVote:     0,
		wantTreasury: 1,
	}, {
		name:         "height 10954752 (first zero treasury subsidy 1783rd reduction), 5 votes",
		params:       mockMainNetParams,
		height:       10954752,
		numVotes:     5,
		wantFull:     9,
		wantWork:     5,
		wantVote:     0,
		wantTreasury: 0,
	}, {
		name:         "height 11003904 (first zero work subsidy 1791st reduction), 5 votes",
		params:       mockMainNetParams,
		height:       11003904,
		numVotes:     5,
		wantFull:     1,
		wantWork:     0,
		wantVote:     0,
		wantTreasury: 0,
	}, {
		name:         "height 11010048 (first zero full subsidy 1792nd reduction), 5 votes",
		params:       mockMainNetParams,
		height:       11010048,
		numVotes:     5,
		wantFull:     0,
		wantWork:     0,
		wantVote:     0,
		wantTreasury: 0,
	}}

	for _, test := range tests {
		// Ensure the full subsidy is the expected value.
		cache := NewSubsidyCache(test.params)
		fullSubsidyResult := cache.CalcBlockSubsidy(test.height)
		if fullSubsidyResult != test.wantFull {
			t.Errorf("%s: unexpected full subsidy result -- got %d, want %d",
				test.name, fullSubsidyResult, test.wantFull)
			continue
		}

		// Ensure the PoW subsidy is the expected value.
		workResult := cache.CalcWorkSubsidy(test.height, test.numVotes)
		if workResult != test.wantWork {
			t.Errorf("%s: unexpected work subsidy result -- got %d, want %d",
				test.name, workResult, test.wantWork)
			continue
		}

		// Ensure the vote subsidy is the expected value.
		voteResult := cache.CalcStakeVoteSubsidy(test.height)
		if voteResult != test.wantVote {
			t.Errorf("%s: unexpected vote subsidy result -- got %d, want %d",
				test.name, voteResult, test.wantVote)
			continue
		}

		// Ensure the treasury subsidy is the expected value. With treasury.
		treasuryResult := cache.CalcTreasurySubsidy(test.height, test.numVotes,
			withTreasury)
		if treasuryResult != test.wantTreasury {
			t.Errorf("%s: unexpected treasury subsidy result -- got %d, want %d",
				test.name, treasuryResult, test.wantTreasury)
			continue
		}
	}
}

// TestTotalSubsidy ensures the total subsidy produced matches the expected
// value.
func TestTotalSubsidy(t *testing.T) {
	// Locals for convenience.
	mockMainNetParams := mockMainNetParams()
	reductionInterval := mockMainNetParams.SubsidyReductionIntervalBlocks()
	stakeValidationHeight := mockMainNetParams.StakeValidationBeginHeight()
	votesPerBlock := mockMainNetParams.VotesPerBlock()

	// subsidySum returns the sum of the individual subsidy types for the given
	// height.  Note that this value is not exactly the same as the full subsidy
	// originally used to calculate the individual proportions due to the use
	// of integer math.
	cache := NewSubsidyCache(mockMainNetParams)
	subsidySum := func(height int64) int64 {
		work := cache.CalcWorkSubsidy(height, votesPerBlock)
		vote := cache.CalcStakeVoteSubsidy(height) * int64(votesPerBlock)
		treasury := cache.CalcTreasurySubsidy(height, votesPerBlock,
			noTreasury)
		return work + vote + treasury
	}

	// Calculate the total possible subsidy.
	totalSubsidy := mockMainNetParams.BlockOneSubsidy()
	for reductionNum := int64(0); ; reductionNum++ {
		// The first interval contains a few special cases:
		// 1) Block 0 does not produce any subsidy
		// 2) Block 1 consists of a special initial coin distribution
		// 3) Votes do not produce subsidy until voting begins
		if reductionNum == 0 {
			// Account for the block up to the point voting begins ignoring the
			// first two special blocks.
			subsidyCalcHeight := int64(2)
			nonVotingBlocks := stakeValidationHeight - subsidyCalcHeight
			totalSubsidy += subsidySum(subsidyCalcHeight) * nonVotingBlocks

			// Account for the blocks remaining in the interval once voting
			// begins.
			subsidyCalcHeight = stakeValidationHeight
			votingBlocks := reductionInterval - subsidyCalcHeight
			totalSubsidy += subsidySum(subsidyCalcHeight) * votingBlocks
			continue
		}

		// Account for the all other reduction intervals until all subsidy has
		// been produced.
		subsidyCalcHeight := reductionNum * reductionInterval
		sum := subsidySum(subsidyCalcHeight)
		if sum == 0 {
			break
		}
		totalSubsidy += sum * reductionInterval
	}

	// Ensure the total calculated subsidy is the expected value.
	const expectedTotalSubsidy = 2099999999800912
	if totalSubsidy != expectedTotalSubsidy {
		t.Fatalf("mismatched total subsidy -- got %d, want %d", totalSubsidy,
			expectedTotalSubsidy)
	}
}

// TestTotalSubsidyTreasury ensures the total subsidy produced matches the
// expected value with treasury enabled.
func TestTotalSubsidyTreasury(t *testing.T) {
	// Locals for convenience.
	mockMainNetParams := mockMainNetParams()
	reductionInterval := mockMainNetParams.SubsidyReductionIntervalBlocks()
	stakeValidationHeight := mockMainNetParams.StakeValidationBeginHeight()
	votesPerBlock := mockMainNetParams.VotesPerBlock()

	// subsidySum returns the sum of the individual subsidy types for the given
	// height.  Note that this value is not exactly the same as the full subsidy
	// originally used to calculate the individual proportions due to the use
	// of integer math.
	cache := NewSubsidyCache(mockMainNetParams)
	subsidySum := func(height int64) int64 {
		work := cache.CalcWorkSubsidy(height, votesPerBlock)
		vote := cache.CalcStakeVoteSubsidy(height) * int64(votesPerBlock)
		treasury := cache.CalcTreasurySubsidy(height, votesPerBlock,
			withTreasury)
		return work + vote + treasury
	}

	// Calculate the total possible subsidy.
	totalSubsidy := mockMainNetParams.BlockOneSubsidy()
	for reductionNum := int64(0); ; reductionNum++ {
		// The first interval contains a few special cases:
		// 1) Block 0 does not produce any subsidy
		// 2) Block 1 consists of a special initial coin distribution
		// 3) Votes do not produce subsidy until voting begins
		if reductionNum == 0 {
			// Account for the block up to the point voting begins ignoring the
			// first two special blocks.
			subsidyCalcHeight := int64(2)
			nonVotingBlocks := stakeValidationHeight - subsidyCalcHeight
			totalSubsidy += subsidySum(subsidyCalcHeight) * nonVotingBlocks

			// Account for the blocks remaining in the interval once voting
			// begins.
			subsidyCalcHeight = stakeValidationHeight
			votingBlocks := reductionInterval - subsidyCalcHeight
			totalSubsidy += subsidySum(subsidyCalcHeight) * votingBlocks
			continue
		}

		// Account for the all other reduction intervals until all subsidy has
		// been produced.
		subsidyCalcHeight := reductionNum * reductionInterval
		sum := subsidySum(subsidyCalcHeight)
		if sum == 0 {
			break
		}
		totalSubsidy += sum * reductionInterval
	}

	// Ensure the total calculated subsidy is the expected value.
	const expectedTotalSubsidy = 2099999999800912
	if totalSubsidy != expectedTotalSubsidy {
		t.Fatalf("mismatched total subsidy -- got %d, want %d", totalSubsidy,
			expectedTotalSubsidy)
	}
}

// TestTotalSubsidyDCP0010 ensures the estimated total subsidy produced with the
// subsidy split defined in DCP0010 matches the expected value.
func TestTotalSubsidyDCP0010(t *testing.T) {
	// Locals for convenience.
	mockMainNetParams := mockMainNetParams()
	reductionInterval := mockMainNetParams.SubsidyReductionIntervalBlocks()
	stakeValidationHeight := mockMainNetParams.StakeValidationBeginHeight()
	votesPerBlock := mockMainNetParams.VotesPerBlock()

	// subsidySum returns the sum of the individual subsidies for the given
	// height using either the original subsidy split or the modified split
	// defined in DCP0010.  Note that this value is not exactly the same as the
	// full subsidy originally used to calculate the individual proportions due
	// to the use of integer math.
	cache := NewSubsidyCache(mockMainNetParams)
	subsidySum := func(height int64, useDCP0010 bool) int64 {
		work := cache.CalcWorkSubsidyV2(height, votesPerBlock, useDCP0010)
		vote := cache.CalcStakeVoteSubsidyV2(height, useDCP0010) *
			int64(votesPerBlock)
		treasury := cache.CalcTreasurySubsidy(height, votesPerBlock, noTreasury)
		return work + vote + treasury
	}

	// Calculate the total possible subsidy.
	totalSubsidy := mockMainNetParams.BlockOneSubsidy()
	for reductionNum := int64(0); ; reductionNum++ {
		// The first interval contains a few special cases:
		// 1) Block 0 does not produce any subsidy
		// 2) Block 1 consists of a special initial coin distribution
		// 3) Votes do not produce subsidy until voting begins
		if reductionNum == 0 {
			// Account for the block up to the point voting begins ignoring the
			// first two special blocks.
			subsidyCalcHeight := int64(2)
			nonVotingBlocks := stakeValidationHeight - subsidyCalcHeight
			totalSubsidy += subsidySum(subsidyCalcHeight, false) * nonVotingBlocks

			// Account for the blocks remaining in the interval once voting
			// begins.
			subsidyCalcHeight = stakeValidationHeight
			votingBlocks := reductionInterval - subsidyCalcHeight
			totalSubsidy += subsidySum(subsidyCalcHeight, false) * votingBlocks
			continue
		}

		// Account for the all other reduction intervals until all subsidy has
		// been produced.
		//
		// Note that this is necessarily an estimate since the exact height at
		// which DCP0010 should be activated is impossible to know at the time
		// of this writing.  For testing purposes, the activation height is
		// estimated to be 638976, or in other words, the 104th reduction
		// interval on mainnet.
		subsidyCalcHeight := reductionNum * reductionInterval
		useDCP0010 := subsidyCalcHeight >= reductionInterval*104
		sum := subsidySum(subsidyCalcHeight, useDCP0010)
		if sum == 0 {
			break
		}
		totalSubsidy += sum * reductionInterval
	}

	// Ensure the total calculated subsidy is the expected value.
	const expectedTotalSubsidy = 2100000000015952
	if totalSubsidy != expectedTotalSubsidy {
		t.Fatalf("mismatched total subsidy -- got %d, want %d", totalSubsidy,
			expectedTotalSubsidy)
	}
}

// TestCalcBlockSubsidySparseCaching ensures the cache calculations work
// properly when accessed sparsely and out of order.
func TestCalcBlockSubsidySparseCaching(t *testing.T) {
	// Mock params used in tests.
	mockMainNetParams := mockMainNetParams()

	// perCacheTest describes a test to run against the same cache.
	type perCacheTest struct {
		name   string // test description
		height int64  // height to calculate subsidy for
		want   int64  // expected subsidy
	}

	tests := []struct {
		name          string         // test description
		params        SubsidyParams  // params to use in subsidy calculations
		perCacheTests []perCacheTest // tests to run against same cache instance
	}{{
		name:   "negative/zero/one (special cases, no cache)",
		params: mockMainNetParams,
		perCacheTests: []perCacheTest{{
			name:   "would be negative interval",
			height: -6144,
			want:   0,
		}, {
			name:   "negative one",
			height: -1,
			want:   0,
		}, {
			name:   "height 0",
			height: 0,
			want:   0,
		}, {
			name:   "height 1",
			height: 1,
			want:   168000000000000,
		}},
	}, {
		name:   "clean cache, negative height",
		params: mockMainNetParams,
		perCacheTests: []perCacheTest{{
			name:   "would be negative interval",
			height: -6144,
			want:   0,
		}, {
			name:   "height 0",
			height: 0,
			want:   0,
		}},
	}, {
		name:   "clean cache, max int64 height twice",
		params: mockMainNetParams,
		perCacheTests: []perCacheTest{{
			name:   "max int64",
			height: 9223372036854775807,
			want:   0,
		}, {
			name:   "second max int64",
			height: 9223372036854775807,
			want:   0,
		}},
	}, {
		name:   "sparse out order interval requests with cache hits",
		params: mockMainNetParams,
		perCacheTests: []perCacheTest{{
			name:   "height 0",
			height: 0,
			want:   0,
		}, {
			name:   "height 1",
			height: 1,
			want:   168000000000000,
		}, {
			name:   "height 2 (cause interval 0 cache addition)",
			height: 2,
			want:   3119582664,
		}, {
			name:   "height 2 (interval 0 cache hit)",
			height: 2,
			want:   3119582664,
		}, {
			name:   "height 3 (interval 0 cache hit)",
			height: 2,
			want:   3119582664,
		}, {
			name:   "height 6145 (interval 1 cache addition)",
			height: 6145,
			want:   3088695706,
		}, {
			name:   "height 6145 (interval 1 cache hit)",
			height: 6145,
			want:   3088695706,
		}, {
			name:   "interval 20 cache addition most recent cache interval 1",
			height: 6144 * 20,
			want:   2556636713,
		}, {
			name:   "interval 20 cache hit",
			height: 6144 * 20,
			want:   2556636713,
		}, {
			name:   "interval 10 cache addition most recent cache interval 20",
			height: 6144 * 10,
			want:   2824117486,
		}, {
			name:   "interval 10 cache hit",
			height: 6144 * 10,
			want:   2824117486,
		}, {
			name:   "interval 15 cache addition between cached 10 and 20",
			height: 6144 * 15,
			want:   2687050883,
		}, {
			name:   "interval 15 cache hit",
			height: 6144 * 15,
			want:   2687050883,
		}, {
			name:   "interval 1792 (first with 0 subsidy) cache addition",
			height: 6144 * 1792,
			want:   0,
		}, {
			name:   "interval 1792 cache hit",
			height: 6144 * 1792,
			want:   0,
		}, {
			name:   "interval 1795 (skipping final 0 subsidy)",
			height: 6144 * 1795,
			want:   0,
		}},
	}, {
		name:   "clean cache, reverse interval requests",
		params: mockMainNetParams,
		perCacheTests: []perCacheTest{{
			name:   "interval 5 cache addition",
			height: 6144 * 5,
			want:   2968175862,
		}, {
			name:   "interval 3 cache addition",
			height: 6144 * 3,
			want:   3027836198,
		}, {
			name:   "interval 3 cache hit",
			height: 6144 * 3,
			want:   3027836198,
		}},
	}, {
		name:   "clean cache, forward non-zero start interval requests",
		params: mockMainNetParams,
		perCacheTests: []perCacheTest{{
			name:   "interval 2 cache addition",
			height: 6144 * 2,
			want:   3058114560,
		}, {
			name:   "interval 12 cache addition",
			height: 6144 * 12,
			want:   2768471213,
		}, {
			name:   "interval 12 cache hit",
			height: 6144 * 12,
			want:   2768471213,
		}},
	}}

	for _, test := range tests {
		cache := NewSubsidyCache(test.params)
		for _, pcTest := range test.perCacheTests {
			result := cache.CalcBlockSubsidy(pcTest.height)
			if result != pcTest.want {
				t.Errorf("%q-%q: mismatched subsidy -- got %d, want %d",
					test.name, pcTest.name, result, pcTest.want)
				continue
			}
		}
	}
}
