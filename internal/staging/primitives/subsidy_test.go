// Copyright (c) 2019-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package primitives

import (
	"testing"
)

// mockSubsidyParams implements the SubsidyParams interface and is used
// throughout the tests to mock networks.
type mockSubsidyParams struct {
	blockOne              int64
	baseSubsidy           int64
	reductionMultiplier   int64
	reductionDivisor      int64
	reductionInterval     int64
	stakeValidationHeight int64
	votesPerBlock         uint16
}

// Ensure the mock subsidy params satisfy the SubsidyParams interface.
var _ SubsidyParams = (*mockSubsidyParams)(nil)

// BlockOneSubsidy returns the value associated with the mock params for the
// total subsidy of block height 1 for the network.
func (p *mockSubsidyParams) BlockOneSubsidy() int64 {
	return p.blockOne
}

// BaseSubsidyValue returns the value associated with the mock params for the
// starting base max potential subsidy amount for mined blocks.
func (p *mockSubsidyParams) BaseSubsidyValue() int64 {
	return p.baseSubsidy
}

// SubsidyReductionMultiplier returns the value associated with the mock params
// for the multiplier to use when performing the exponential subsidy reduction
// described by the CalcBlockSubsidy documentation.
func (p *mockSubsidyParams) SubsidyReductionMultiplier() int64 {
	return p.reductionMultiplier
}

// SubsidyReductionDivisor returns the value associated with the mock params for
// the divisor to use when performing the exponential subsidy reduction
// described by the CalcBlockSubsidy documentation.
func (p *mockSubsidyParams) SubsidyReductionDivisor() int64 {
	return p.reductionDivisor
}

// SubsidyReductionIntervalBlocks returns the value associated with the mock
// params for the reduction interval in number of blocks.
func (p *mockSubsidyParams) SubsidyReductionIntervalBlocks() int64 {
	return p.reductionInterval
}

// StakeValidationBeginHeight returns the value associated with the mock params
// for the height at which votes become required to extend a block.
func (p *mockSubsidyParams) StakeValidationBeginHeight() int64 {
	return p.stakeValidationHeight
}

// VotesPerBlock returns the value associated with the mock params for the
// maximum number of votes a block must contain to receive full subsidy once
// voting begins at StakeValidationBeginHeight
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
		stakeValidationHeight: 4096,
		votesPerBlock:         5,
	}
}

// TestSubsidyCacheCalcs ensures the subsidy cache calculates the various
// subsidy proportions and values as expected.
func TestSubsidyCacheCalcs(t *testing.T) {
	t.Parallel()

	// Mock params used in tests.
	params := mockMainNetParams()

	tests := []struct {
		name            string // test description
		height          int64  // height to calculate subsidy for
		numVotes        uint16 // number of votes
		wantFull        int64  // expected full block subsidy
		wantWork        int64  // expected pow subsidy w/o DCP0010
		wantWorkDCP0010 int64  // expected pow subsidy with DCP0010
		wantVote        int64  // expected single vote subsidy
		wantVoteDCP0010 int64  // expected single vote subsidy w/o DCP0010
		wantTrsy        int64  // expected treasury subsidy w/o DCP0006
		wantTrsyDCP0006 int64  // expected treasury subsidy with DCP0006
	}{{
		name:            "negative height",
		height:          -1,
		numVotes:        0,
		wantFull:        0,
		wantWork:        0,
		wantWorkDCP0010: 0,
		wantVote:        0,
		wantVoteDCP0010: 0,
		wantTrsy:        0,
		wantTrsyDCP0006: 0,
	}, {
		name:            "height 0",
		height:          0,
		numVotes:        0,
		wantFull:        0,
		wantWork:        0,
		wantWorkDCP0010: 0,
		wantVote:        0,
		wantVoteDCP0010: 0,
		wantTrsy:        0,
		wantTrsyDCP0006: 0,
	}, {
		name:            "height 1 (initial payouts)",
		height:          1,
		numVotes:        0,
		wantFull:        168000000000000,
		wantWork:        168000000000000,
		wantWorkDCP0010: 168000000000000,
		wantVote:        0,
		wantVoteDCP0010: 0,
		wantTrsy:        0,
		wantTrsyDCP0006: 0,
	}, {
		name:            "height 2 (first non-special block prior voting start)",
		height:          2,
		numVotes:        0,
		wantFull:        3119582664,
		wantWork:        1871749598,
		wantWorkDCP0010: 311958266,
		wantVote:        0,
		wantVoteDCP0010: 0,
		wantTrsy:        311958266,
		wantTrsyDCP0006: 311958266,
	}, {
		name:            "height 4094 (two blocks prior to voting start)",
		height:          4094,
		numVotes:        0,
		wantFull:        3119582664,
		wantWork:        1871749598,
		wantWorkDCP0010: 311958266,
		wantVote:        0,
		wantVoteDCP0010: 0,
		wantTrsy:        311958266,
		wantTrsyDCP0006: 311958266,
	}, {
		name:            "height 4095 (final block prior to voting start)",
		height:          4095,
		numVotes:        0,
		wantFull:        3119582664,
		wantWork:        1871749598,
		wantWorkDCP0010: 311958266,
		wantVote:        187174959,
		wantVoteDCP0010: 499133226,
		wantTrsy:        311958266,
		wantTrsyDCP0006: 311958266,
	}, {
		name:            "height 4096 (voting start), 5 votes",
		height:          4096,
		numVotes:        5,
		wantFull:        3119582664,
		wantWork:        1871749598,
		wantWorkDCP0010: 311958266,
		wantVote:        187174959,
		wantVoteDCP0010: 499133226,
		wantTrsy:        311958266,
		wantTrsyDCP0006: 311958266,
	}, {
		name:            "height 4096 (voting start), 4 votes",
		height:          4096,
		numVotes:        4,
		wantFull:        3119582664,
		wantWork:        1497399678,
		wantWorkDCP0010: 249566612,
		wantVote:        187174959,
		wantVoteDCP0010: 499133226,
		wantTrsy:        249566612,
		wantTrsyDCP0006: 311958266, // No reduction
	}, {
		name:            "height 4096 (voting start), 3 votes",
		height:          4096,
		numVotes:        3,
		wantFull:        3119582664,
		wantWork:        1123049758,
		wantWorkDCP0010: 187174959,
		wantVote:        187174959,
		wantVoteDCP0010: 499133226,
		wantTrsy:        187174959,
		wantTrsyDCP0006: 311958266, // No reduction
	}, {
		name:            "height 4096 (voting start), 2 votes",
		height:          4096,
		numVotes:        2,
		wantFull:        3119582664,
		wantWork:        0,
		wantWorkDCP0010: 0,
		wantVote:        187174959,
		wantVoteDCP0010: 499133226,
		wantTrsy:        0,
		wantTrsyDCP0006: 0,
	}, {
		name:            "height 6143 (final block prior to 1st reduction), 5 votes",
		height:          6143,
		numVotes:        5,
		wantFull:        3119582664,
		wantWork:        1871749598,
		wantWorkDCP0010: 311958266,
		wantVote:        187174959,
		wantVoteDCP0010: 499133226,
		wantTrsy:        311958266,
		wantTrsyDCP0006: 311958266,
	}, {
		name:            "height 6144 (1st block in 1st reduction), 5 votes",
		height:          6144,
		numVotes:        5,
		wantFull:        3088695706,
		wantWork:        1853217423,
		wantWorkDCP0010: 308869570,
		wantVote:        185321742,
		wantVoteDCP0010: 494191312,
		wantTrsy:        308869570,
		wantTrsyDCP0006: 308869570,
	}, {
		name:            "height 6144 (1st block in 1st reduction), 4 votes",
		height:          6144,
		numVotes:        4,
		wantFull:        3088695706,
		wantWork:        1482573938,
		wantWorkDCP0010: 247095656,
		wantVote:        185321742,
		wantVoteDCP0010: 494191312,
		wantTrsy:        247095656,
		wantTrsyDCP0006: 308869570, // No reduction
	}, {
		name:            "height 12287 (last block in 1st reduction), 5 votes",
		height:          12287,
		numVotes:        5,
		wantFull:        3088695706,
		wantWork:        1853217423,
		wantWorkDCP0010: 308869570,
		wantVote:        185321742,
		wantVoteDCP0010: 494191312,
		wantTrsy:        308869570,
		wantTrsyDCP0006: 308869570,
	}, {
		name:            "height 12288 (1st block in 2nd reduction), 5 votes",
		height:          12288,
		numVotes:        5,
		wantFull:        3058114560,
		wantWork:        1834868736,
		wantWorkDCP0010: 305811456,
		wantVote:        183486873,
		wantVoteDCP0010: 489298329,
		wantTrsy:        305811456,
		wantTrsyDCP0006: 305811456,
	}, {
		name:            "height 307200 (1st block in 50th reduction), 5 votes",
		height:          307200,
		numVotes:        5,
		wantFull:        1896827356,
		wantWork:        1138096413,
		wantWorkDCP0010: 189682735,
		wantVote:        113809641,
		wantVoteDCP0010: 303492376,
		wantTrsy:        189682735,
		wantTrsyDCP0006: 189682735,
	}, {
		name:            "height 307200 (1st block in 50th reduction), 3 votes",
		height:          307200,
		numVotes:        3,
		wantFull:        1896827356,
		wantWork:        682857847,
		wantWorkDCP0010: 113809641,
		wantVote:        113809641,
		wantVoteDCP0010: 303492376,
		wantTrsy:        113809641,
		wantTrsyDCP0006: 189682735, // No reduction
	}, {
		// First zero vote subsidy without DCP0010.
		name:            "height 10911744 (1776th reduction), 5 votes",
		height:          10911744,
		numVotes:        5,
		wantFull:        16,
		wantWork:        9,
		wantWorkDCP0010: 1,
		wantVote:        0,
		wantVoteDCP0010: 2,
		wantTrsy:        1,
		wantTrsyDCP0006: 1,
	}, {
		// First zero treasury subsidy.
		// First zero work subsidy with DCP0010
		name:            "height 10954752 (1783rd reduction), 5 votes",
		height:          10954752,
		numVotes:        5,
		wantFull:        9,
		wantWork:        5,
		wantWorkDCP0010: 0,
		wantVote:        0,
		wantVoteDCP0010: 1,
		wantTrsy:        0,
		wantTrsyDCP0006: 0,
	}, {
		// First zero vote subsidy with DCP0010.
		name:            "height 10973184 (1786th reduction), 5 votes",
		height:          10973184,
		numVotes:        5,
		wantFull:        6,
		wantWork:        3,
		wantWorkDCP0010: 0,
		wantVote:        0,
		wantVoteDCP0010: 0,
		wantTrsy:        0,
		wantTrsyDCP0006: 0,
	}, {
		// First zero work subsidy without DCP0010.
		name:            "height 11003904 (1791st reduction), 5 votes",
		height:          11003904,
		numVotes:        5,
		wantFull:        1,
		wantWork:        0,
		wantWorkDCP0010: 0,
		wantVote:        0,
		wantVoteDCP0010: 0,
		wantTrsy:        0,
		wantTrsyDCP0006: 0,
	}, {
		// First zero full subsidy.
		name:            "height 11010048 (1792nd reduction), 5 votes",
		height:          11010048,
		numVotes:        5,
		wantFull:        0,
		wantWork:        0,
		wantWorkDCP0010: 0,
		wantVote:        0,
		wantVoteDCP0010: 0,
		wantTrsy:        0,
		wantTrsyDCP0006: 0,
	}}

	for _, test := range tests {
		// Ensure the full subsidy is the expected value.
		cache := NewSubsidyCache(params)
		fullSubsidyResult := cache.CalcBlockSubsidy(test.height)
		if fullSubsidyResult != test.wantFull {
			t.Errorf("%s: unexpected full subsidy result -- got %d, want %d",
				test.name, fullSubsidyResult, test.wantFull)
			continue
		}

		// Ensure the PoW subsidy is the expected value both with and without
		// DCP0010.
		gotWork := cache.CalcWorkSubsidy(test.height, test.numVotes, false)
		if gotWork != test.wantWork {
			t.Errorf("%s: unexpected work subsidy result -- got %d, want %d",
				test.name, gotWork, test.wantWork)
			continue
		}
		gotWork = cache.CalcWorkSubsidy(test.height, test.numVotes, true)
		if gotWork != test.wantWorkDCP0010 {
			t.Errorf("%s: unexpected work subsidy result (DCP0010) -- got %d, "+
				"want %d", test.name, gotWork, test.wantWorkDCP0010)
			continue
		}

		// Ensure the vote subsidy is the expected value both with and without
		// DCP00010.
		gotVote := cache.CalcStakeVoteSubsidy(test.height, false)
		if gotVote != test.wantVote {
			t.Errorf("%s: unexpected vote subsidy result -- got %d, want %d",
				test.name, gotVote, test.wantVote)
			continue
		}
		gotVote = cache.CalcStakeVoteSubsidy(test.height, true)
		if gotVote != test.wantVoteDCP0010 {
			t.Errorf("%s: unexpected vote subsidy result (DCP0010) -- got %d, "+
				"want %d", test.name, gotVote, test.wantVoteDCP0010)
			continue
		}

		// Ensure the treasury subsidy is the expected value both with and
		// without DCP0006.
		gotTrsy := cache.CalcTreasurySubsidy(test.height, test.numVotes, false)
		if gotTrsy != test.wantTrsy {
			t.Errorf("%s: unexpected treasury subsidy result -- got %d, want %d",
				test.name, gotTrsy, test.wantTrsy)
			continue
		}
		gotTrsy = cache.CalcTreasurySubsidy(test.height, test.numVotes, true)
		if gotTrsy != test.wantTrsyDCP0006 {
			t.Errorf("%s: unexpected treasury subsidy result (DCP0006) -- got "+
				"%d, want %d", test.name, gotTrsy, test.wantTrsyDCP0006)
			continue
		}
	}
}

// TestTotalSubsidy ensures the total subsidy produced matches the expected
// value both with and without the flag for the decentralized treasury agenda
// defined in DCP0006 set.
func TestTotalSubsidy(t *testing.T) {
	t.Parallel()

	// Locals for convenience.
	mockMainNetParams := mockMainNetParams()
	reductionInterval := mockMainNetParams.SubsidyReductionIntervalBlocks()
	stakeValidationHeight := mockMainNetParams.StakeValidationBeginHeight()
	votesPerBlock := mockMainNetParams.VotesPerBlock()

	checkTotalSubsidy := func(useDCP0006 bool) {
		t.Helper()

		// subsidySum returns the sum of the individual subsidy types for the
		// given height.  Note that this value is not exactly the same as the
		// full subsidy originally used to calculate the individual proportions
		// due to the use of integer math.
		cache := NewSubsidyCache(mockMainNetParams)
		subsidySum := func(height int64) int64 {
			const useDCP0010 = false
			work := cache.CalcWorkSubsidy(height, votesPerBlock, useDCP0010)
			vote := cache.CalcStakeVoteSubsidy(height, useDCP0010)
			vote *= int64(votesPerBlock)
			trsy := cache.CalcTreasurySubsidy(height, votesPerBlock, useDCP0006)
			return work + vote + trsy
		}

		// Calculate the total possible subsidy.
		totalSubsidy := mockMainNetParams.BlockOneSubsidy()
		for reductionNum := int64(0); ; reductionNum++ {
			// The first interval contains a few special cases:
			// 1) Block 0 does not produce any subsidy
			// 2) Block 1 consists of a special initial coin distribution
			// 3) Votes do not produce subsidy until voting begins
			if reductionNum == 0 {
				// Account for the block up to the point voting begins ignoring
				// the first two special blocks.
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

			// Account for the all other reduction intervals until all subsidy
			// has been produced.
			subsidyCalcHeight := reductionNum * reductionInterval
			sum := subsidySum(subsidyCalcHeight)
			if sum == 0 {
				break
			}
			totalSubsidy += sum * reductionInterval
		}

		const expectedTotalSubsidy = 2099999999800912
		if totalSubsidy != expectedTotalSubsidy {
			t.Fatalf("mismatched total subsidy (treasury flag: %v) -- got %d, "+
				"want %d", useDCP0006, totalSubsidy, expectedTotalSubsidy)
		}
	}

	// Ensure the total calculated subsidy is the expected value both with and
	// without the flag for the decentralized treasury agenda
	// defined in DCP0006 set
	checkTotalSubsidy(false)
	checkTotalSubsidy(true)
}

// TestTotalSubsidyDCP0010 ensures the estimated total subsidy produced with the
// subsidy split defined in DCP0010 matches the expected value.
func TestTotalSubsidyDCP0010(t *testing.T) {
	t.Parallel()

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
		const useDCP0006 = false
		work := cache.CalcWorkSubsidy(height, votesPerBlock, useDCP0010)
		vote := cache.CalcStakeVoteSubsidy(height, useDCP0010) *
			int64(votesPerBlock)
		treasury := cache.CalcTreasurySubsidy(height, votesPerBlock, useDCP0006)
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
	t.Parallel()

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
