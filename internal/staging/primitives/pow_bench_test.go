// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package primitives

import (
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// BenchmarkDiffBitsToUint256 benchmarks converting the compact representation
// used to encode difficulty targets to an unsigned 256-bit integer.
func BenchmarkDiffBitsToUint256(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		const input = 0x1b01330e
		DiffBitsToUint256(input)
	}
}

// BenchmarkUint256ToDiffBits benchmarks converting an unsigned 256-bit integer
// to the compact representation used to encode difficulty targets.
func BenchmarkUint256ToDiffBits(b *testing.B) {
	n := hexToUint256("1330e000000000000000000000000000000000000000000000000")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Uint256ToDiffBits(n)
	}
}

// BenchmarkCalcWork benchmarks calculating a work value from difficulty bits.
func BenchmarkCalcWork(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		const input = 0x1b01330e
		CalcWork(input)
	}
}

// BenchmarkHashToUint256 benchmarks converting a hash to an unsigned 256-bit
// integer that can be used to perform math comparisons.
func BenchmarkHashToUint256(b *testing.B) {
	h := "000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9"
	hash, err := chainhash.NewHashFromStr(h)
	if err != nil {
		b.Fatalf("unexpected error: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HashToUint256(hash)
	}
}
