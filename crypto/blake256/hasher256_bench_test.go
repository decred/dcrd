// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// BLAKE-256 benchmarks originally written by Dave Collins May 2020.

package blake256

import (
	"testing"
)

// bufIn is a buffer for use in the benchmarks.
var bufIn = make([]byte, 16384)

// hashBenchTest describes tests that are used for the various hash benchmarks.
// It is defined separately so the same tests can easily be used in comparison
// benchmarks.
type hashBenchTest struct {
	name string // benchmark description
	n    int64  // number of bytes to hash
}

// makeHashBenches returns a slice of tests that consist of a specific number of
// bytes to hash for use in the hash benchmarks.
func makeHashBenches() []hashBenchTest {
	return []hashBenchTest{
		{name: "32b", n: 32},
		{name: "64b", n: 64},
		{name: "1KiB", n: 1024},
		{name: "8KiB", n: 8192},
		{name: "16KiB", n: 16384},
	}
}

// BenchmarkHasher256 benchmarks how long it takes to hash various amount of
// data with BLAKE-256 via [Hasher256.Sum256] along with the number of
// allocations needed.
func BenchmarkHasher256(b *testing.B) {
	benches := makeHashBenches()
	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			hasher := NewHasher256()
			buf := bufIn[:bench.n]

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(bench.n)
			for i := 0; i < b.N; i++ {
				hasher.Write(buf)
				_ = hasher.Sum256()
				hasher.Reset()
			}
		})
	}
}

// BenchmarkSum256 benchmarks how long it takes to hash various amount of
// data with BLAKE-256 via [Sum256] along with the number of allocations needed.
func BenchmarkSum256(b *testing.B) {
	benches := makeHashBenches()
	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			buf := bufIn[:bench.n]

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(bench.n)
			for i := 0; i < b.N; i++ {
				_ = Sum256(buf)
			}
		})
	}
}

// BenchmarkHasher256ResumeCopy benchmarks how long it takes to produce a
// BLAKE-256 hash starting from a copied saved intermediate hashing state
// instance where a various amount of data is already written along with the
// number of allocations needed.
func BenchmarkHasher256ResumeCopy(b *testing.B) {
	benches := makeHashBenches()
	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			hasher := NewHasher256()
			hasher.Write(bufIn[:bench.n])
			savedState := *hasher

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(bench.n)
			for i := 0; i < b.N; i++ {
				hasher := savedState
				_ = hasher.Sum256()
			}
		})
	}
}

// BenchmarkHasher256SaveState benchmarks how long it takes to serialize a
// BLAKE-256 intermediate state using [Hasher256.SaveState] along with the
// number of allocations needed.
func BenchmarkHasher256SaveState(b *testing.B) {
	hasher := NewHasher256()
	state := make([]byte, SavedStateSize)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		state = hasher.SaveState(state[:0])
	}
}

// BenchmarkHasher256MarshalBinary benchmarks how long it takes to serialize a
// BLAKE-256 intermediate state using [Hasher256.MarshalBinary] along with the
// number of allocations needed.
func BenchmarkHasher256MarshalBinary(b *testing.B) {
	hasher := NewHasher256()

	var state []byte
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		state, _ = hasher.MarshalBinary()
	}
	_ = state
}
