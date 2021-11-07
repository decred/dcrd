// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package primitives

import (
	"testing"
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
