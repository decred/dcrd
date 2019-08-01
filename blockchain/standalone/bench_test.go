// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package standalone

import (
	"strconv"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// BenchmarkCalcMerkleRootInPlace benchmarks merkle root calculation for various
// numbers of leaves using the mutable in-place algorithm.
func BenchmarkCalcMerkleRootInPlace(b *testing.B) {
	// Create several slices of leaves of various sizes to benchmark.
	numLeavesToBench := []int{20, 1000, 2000, 4000, 8000, 16000, 32000}
	origLeaves := make([][]chainhash.Hash, len(numLeavesToBench))
	for i, numLeaves := range numLeavesToBench {
		origLeaves[i] = make([]chainhash.Hash, numLeaves)
	}

	for benchIdx := range origLeaves {
		testLeaves := origLeaves[benchIdx]
		benchName := strconv.Itoa(len(testLeaves))
		b.Run(benchName, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = CalcMerkleRootInPlace(testLeaves)
			}
		})
	}
}

// BenchmarkCalcMerkleRoot benchmarks merkle root calculation for various
// numbers of leaves using the non-mutable version.
func BenchmarkCalcMerkleRoot(b *testing.B) {
	// Create several slices of leaves of various sizes to benchmark.
	numLeavesToBench := []int{20, 1000, 2000, 4000, 8000, 16000, 32000}
	origLeaves := make([][]chainhash.Hash, len(numLeavesToBench))
	for i, numLeaves := range numLeavesToBench {
		origLeaves[i] = make([]chainhash.Hash, numLeaves)
	}

	for benchIdx := range origLeaves {
		testLeaves := origLeaves[benchIdx]
		benchName := strconv.Itoa(len(testLeaves))
		b.Run(benchName, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = CalcMerkleRoot(testLeaves)
			}
		})
	}
}
