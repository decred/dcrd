// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rand

import (
	crand "crypto/rand"
	"testing"
	"time"
)

// readBenchTest describes tests that are used for the read benchmarks.  It is
// defined separately so the same tests can easily be used in comparison
// benchmarks between the specialized readers in this package and stdlib
// crypto/rand reader.
type readBenchTest struct {
	name string // benchmark description
	n    int    // number of bytes to read
}

// makeReadBenches returns a slice of tests that consist of a specific number of
// bytes to read for use in the read benchmarks.
func makeReadBenches() []readBenchTest {
	return []readBenchTest{
		{name: "4b", n: 4},
		{name: "8b", n: 8},
		{name: "32b", n: 32},
		{name: "512b", n: 512},
		{name: "1KiB", n: 1024},
		{name: "4KiB", n: 4096},
	}
}

// BenchmarkDcrdRead benchmarks reading random values via the global Read method
// with various size reads.
func BenchmarkDcrdRead(b *testing.B) {
	benches := makeReadBenches()
	for benchIdx := range benches {
		bench := benches[benchIdx]
		b.Run(bench.name, func(b *testing.B) {
			buf := make([]byte, bench.n)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				Read(buf)
			}
		})
	}
}

// BenchmarkStdlibRead benchmarks reading random values via the stdlib
// crypto/rand Read method with various size reads.
func BenchmarkStdlibRead(b *testing.B) {
	benches := makeReadBenches()
	for benchIdx := range benches {
		bench := benches[benchIdx]
		b.Run(bench.name, func(b *testing.B) {
			buf := make([]byte, bench.n)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				crand.Read(buf)
			}
		})
	}
}

// BenchmarkDcrdReadPRNG benchmarks reading random values via a local PRNG
// instance with various size reads.
func BenchmarkDcrdReadPRNG(b *testing.B) {
	benches := makeReadBenches()
	for benchIdx := range benches {
		bench := benches[benchIdx]
		b.Run(bench.name, func(b *testing.B) {
			prng, err := NewPRNG()
			if err != nil {
				b.Fatalf("unexpected error creating PRNG: %v", err)
			}
			buf := make([]byte, bench.n)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				prng.Read(buf)
			}
		})
	}
}

// BenchmarkInt32N benchmarks obtaining a uniformly random int32 up to a random
// upper limit via the global method.
func BenchmarkInt32N(b *testing.B) {
	// Choose a random value for the upper limit.
	n := Int32()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Int32N(n)
	}
}

// BenchmarkUint32N benchmarks obtaining a uniformly random uint32 up to a
// random limit via the global method.
func BenchmarkUint32N(b *testing.B) {
	// Choose a random value for the upper limit.
	n := Uint32()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Uint32N(n)
	}
}

// BenchmarkInt64N benchmarks obtaining a uniformly random int64 up to a random
// limit via the global method.
func BenchmarkInt64N(b *testing.B) {
	// Choose a random value for the upper limit, but don't exceed a uint32
	// since such large values for random selection are exceedingly rare in
	// practice.
	n := int64(Uint32())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Int64N(n)
	}
}

// BenchmarkUint64N benchmarks obtaining a uniformly random uint64 up to a
// random limit via the global method.
func BenchmarkUint64N(b *testing.B) {
	// Choose a random value for the upper limit, but don't exceed a uint32
	// since such large values for random selection are exceedingly rare in
	// practice.
	n := uint64(Uint32())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Uint64N(n)
	}
}

// BenchmarkDuration benchmarks obtaining a uniformly random time.Duration up to
// a random number of seconds via the global method.
func BenchmarkDuration(b *testing.B) {
	// Choose a random number of seconds for the upper limit.
	durationSecs := time.Second * time.Duration(Uint32())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Duration(durationSecs)
	}
}

// BenchmarkShuffleSlice benchmarks randomizing the order of all elements in a
// slice via the global method.  It is normalized to benchmark the shuffling
// operation itself independent of the number of items in the slice.
func BenchmarkShuffleSlice(b *testing.B) {
	const numItems = 100
	s := make([]uint64, numItems)
	for i := 0; i < numItems; i++ {
		s[i] = Uint64()
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i += numItems {
		ShuffleSlice(s)
	}
}
