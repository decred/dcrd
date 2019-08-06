// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/decred/dcrd/chaincfg"
)

func TestBlockSubsidy(t *testing.T) {
	mainnet := &chaincfg.MainNetParams
	reductionInterval := mainnet.SubsidyReductionInterval
	stakeValidationHeight := mainnet.StakeValidationHeight
	votesPerBlock := mainnet.TicketsPerBlock

	// subsidySum returns the sum of the individual subsidy types for the given
	// height.  Note that this value is not exactly the same as the full subsidy
	// originally used to calculate the individual proportions due to the use
	// of integer math.
	cache := NewSubsidyCache(0, mainnet)
	subsidySum := func(height int64) int64 {
		work := CalcBlockWorkSubsidy(cache, height, votesPerBlock, mainnet)
		vote := CalcStakeVoteSubsidy(cache, height, mainnet) * int64(votesPerBlock)
		treasury := CalcBlockTaxSubsidy(cache, height, votesPerBlock, mainnet)
		return work + vote + treasury
	}

	// Calculate the total possible subsidy.
	totalSubsidy := mainnet.BlockOneSubsidy()
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

	if totalSubsidy != 2099999999800912 {
		t.Errorf("Bad total subsidy; want 2099999999800912, got %v", totalSubsidy)
	}
}

func TestCachedCalcBlockSubsidy(t *testing.T) {
	mainnet := &chaincfg.MainNetParams

	cacheA := NewSubsidyCache(0, mainnet)
	_ = cacheA.CalcBlockSubsidy(mainnet.SubsidyReductionInterval + 1)

	// subsidyA is internally being calculated using the last cached subsidy.
	subsidyA := cacheA.CalcBlockSubsidy((mainnet.SubsidyReductionInterval * 2) + 1)

	cacheB := NewSubsidyCache(0, mainnet)

	// subsidyB is internally being calculated from scratch.
	subsidyB := cacheB.CalcBlockSubsidy((mainnet.SubsidyReductionInterval * 2) + 1)

	if subsidyA != subsidyB {
		t.Fatalf("Expected equal subsidies, got sudsidyA -> %v, "+
			"subsidyB -> %v", subsidyA, subsidyB)
	}
}
