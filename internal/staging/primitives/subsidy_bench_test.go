// Copyright (c) 2019-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package primitives

import (
	"testing"
)

// BenchmarkCalcSubsidyCacheSparse benchmarks calculating the subsidy for
// various heights with a sparse access pattern.
func BenchmarkCalcSubsidyCacheSparse(b *testing.B) {
	mockParams := mockMainNetParams()
	reductionInterval := mockParams.SubsidyReductionIntervalBlocks()

	b.ResetTimer()
	b.ReportAllocs()
	const numIterations = 100
	for i := 0; i < b.N; i += (numIterations * 5) {
		cache := NewSubsidyCache(mockParams)
		for j := int64(0); j < numIterations; j++ {
			cache.CalcBlockSubsidy(reductionInterval * (10000 + j))
			cache.CalcBlockSubsidy(reductionInterval * 1)
			cache.CalcBlockSubsidy(reductionInterval * 5)
			cache.CalcBlockSubsidy(reductionInterval * 25)
			cache.CalcBlockSubsidy(reductionInterval * 13)
		}
	}
}

// BenchmarkCalcWorkSubsidy benchmarks calculating the work subsidy proportion
// for various heights with a sparse access pattern, varying numbers of votes,
// and with and without DCP0010 active.
func BenchmarkCalcWorkSubsidy(b *testing.B) {
	mockParams := mockMainNetParams()
	reductionInterval := mockParams.SubsidyReductionIntervalBlocks()

	b.ResetTimer()
	b.ReportAllocs()
	const numIterations = 100
	for i := 0; i < b.N; i += (numIterations * 5) {
		cache := NewSubsidyCache(mockParams)
		for j := int64(0); j < numIterations; j++ {
			cache.CalcWorkSubsidy(reductionInterval*(10000+j), 5, true)
			cache.CalcWorkSubsidy(reductionInterval*1, 4, false)
			cache.CalcWorkSubsidy(reductionInterval*5, 3, false)
			cache.CalcWorkSubsidy(reductionInterval*25, 4, true)
			cache.CalcWorkSubsidy(reductionInterval*13, 5, false)
		}
	}
}

// BenchmarkCalcStakeVoteSubsidy benchmarks calculating the stake vote subsidy
// proportion for various heights with a sparse access pattern with and without
// DCP0010 active.
func BenchmarkCalcStakeVoteSubsidy(b *testing.B) {
	mockParams := mockMainNetParams()
	reductionInterval := mockParams.SubsidyReductionIntervalBlocks()

	b.ResetTimer()
	b.ReportAllocs()
	const numIterations = 100
	for i := 0; i < b.N; i += (numIterations * 5) {
		cache := NewSubsidyCache(mockParams)
		for j := int64(0); j < numIterations; j++ {
			cache.CalcStakeVoteSubsidy(reductionInterval*(10000+j), true)
			cache.CalcStakeVoteSubsidy(reductionInterval*1, false)
			cache.CalcStakeVoteSubsidy(reductionInterval*5, false)
			cache.CalcStakeVoteSubsidy(reductionInterval*25, true)
			cache.CalcStakeVoteSubsidy(reductionInterval*13, false)
		}
	}
}

// BenchmarkCalcTreasurySubsidy benchmarks calculating the treasury subsidy
// proportion for various heights with a sparse access pattern, varying numbers
// of votes, and with and without DCP0006 active.
func BenchmarkCalcTreasurySubsidy(b *testing.B) {
	mockParams := mockMainNetParams()
	reductionInterval := mockParams.SubsidyReductionIntervalBlocks()

	b.ResetTimer()
	b.ReportAllocs()
	const numIterations = 100
	for i := 0; i < b.N; i += (numIterations * 5) {
		cache := NewSubsidyCache(mockParams)
		for j := int64(0); j < numIterations; j++ {
			cache.CalcTreasurySubsidy(reductionInterval*(10000+j), 5, true)
			cache.CalcTreasurySubsidy(reductionInterval*1, 4, false)
			cache.CalcTreasurySubsidy(reductionInterval*5, 3, false)
			cache.CalcTreasurySubsidy(reductionInterval*25, 4, true)
			cache.CalcTreasurySubsidy(reductionInterval*13, 5, true)
		}
	}
}
