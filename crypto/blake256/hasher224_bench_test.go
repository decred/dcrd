// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// BLAKE-224 benchmarks originally written by Dave Collins July 2024.

package blake256

import (
	"testing"
)

// BenchmarkHasher224 benchmarks how long it takes to hash various amount of
// data with BLAKE-224 via [Hasher224.Sum224] along with the number of
// allocations needed.
func BenchmarkHasher224(b *testing.B) {
	benches := makeHashBenches()
	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			hasher := NewHasher224()
			buf := bufIn[:bench.n]

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(bench.n)
			for i := 0; i < b.N; i++ {
				hasher.Write(buf)
				_ = hasher.Sum224()
				hasher.Reset()
			}
		})
	}
}

// BenchmarkSum224 benchmarks how long it takes to hash various amount of
// data with BLAKE-224 via [Sum224] along with the number of allocations needed.
func BenchmarkSum224(b *testing.B) {
	benches := makeHashBenches()
	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			buf := bufIn[:bench.n]

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(bench.n)
			for i := 0; i < b.N; i++ {
				_ = Sum224(buf)
			}
		})
	}
}

// BenchmarkHasher224ResumeCopy benchmarks how long it takes to produce a
// BLAKE-224 hash starting from a copied saved intermediate hashing state
// instance where a various amount of data is already written along with the
// number of allocations needed.
func BenchmarkHasher224ResumeCopy(b *testing.B) {
	benches := makeHashBenches()
	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			hasher := NewHasher224()
			hasher.Write(bufIn[:bench.n])
			savedState := *hasher

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(bench.n)
			for i := 0; i < b.N; i++ {
				hasher := savedState
				_ = hasher.Sum224()
			}
		})
	}
}

// BenchmarkHasher224SaveState benchmarks how long it takes to serialize a
// BLAKE-224 intermediate state using [Hasher224.SaveState] along with the
// number of allocations needed.
func BenchmarkHasher224SaveState(b *testing.B) {
	hasher := NewHasher224()
	state := make([]byte, SavedStateSize)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		state = hasher.SaveState(state[:0])
	}
}

// BenchmarkHasher224MarshalBinary benchmarks how long it takes to serialize a
// BLAKE-224 intermediate state using [Hasher224.MarshalBinary] along with the
// number of allocations needed.
func BenchmarkHasher224MarshalBinary(b *testing.B) {
	hasher := NewHasher224()

	var state []byte
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		state, _ = hasher.MarshalBinary()
	}
	_ = state
}
