// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package apbf

import (
	"encoding/binary"
	"fmt"
	"testing"
)

// BenchmarkAdd benchmarks adding items to an APBF for various capacities and
// false positive rates.
func BenchmarkAdd(b *testing.B) {
	benches := []struct {
		capacity uint32  // target capacity
		fpRate   float64 // target false positive rate
	}{{
		capacity: 1000,
		fpRate:   0.001,
	}, {
		capacity: 1000,
		fpRate:   0.0001,
	}, {
		capacity: 1000,
		fpRate:   0.00001,
	}, {
		capacity: 100000,
		fpRate:   0.0001,
	}, {
		capacity: 100000,
		fpRate:   0.000001,
	}, {
		capacity: 1000000,
		fpRate:   0.0000001,
	}}

	for benchIdx := range benches {
		bench := benches[benchIdx]
		benchName := fmt.Sprintf("capacity=%d/fprate=%0.7f", bench.capacity,
			bench.fpRate)
		b.Run(benchName, func(b *testing.B) {
			filter := NewFilter(bench.capacity, bench.fpRate)

			b.ResetTimer()
			b.ReportAllocs()
			var data [4]byte
			for i := 0; i < b.N; i++ {
				binary.LittleEndian.PutUint32(data[:], uint32(i))
				filter.Add(data[:])
			}
		})
	}
}

// BenchmarkContainsTrue benchmarks membership queries on an APBF for various
// capacities and false positive rates when the item exists in the filter and
// is near worst case performance.
func BenchmarkContainsTrue(b *testing.B) {
	benches := []struct {
		capacity uint32  // target capacity
		fpRate   float64 // target false positive rate
	}{{
		capacity: 1000,
		fpRate:   0.001,
	}, {
		capacity: 1000,
		fpRate:   0.0001,
	}, {
		capacity: 1000,
		fpRate:   0.00001,
	}, {
		capacity: 100000,
		fpRate:   0.0001,
	}, {
		capacity: 100000,
		fpRate:   0.000001,
	}}

	for benchIdx := range benches {
		bench := benches[benchIdx]
		benchName := fmt.Sprintf("capacity=%d/fprate=%0.6f", bench.capacity,
			bench.fpRate)
		b.Run(benchName, func(b *testing.B) {
			// Load the filter so the benchmark is with a max capacity filter.
			filter := NewFilter(bench.capacity, bench.fpRate)
			var data [4]byte
			for i := uint32(0); i < filter.Capacity(); i++ {
				binary.LittleEndian.PutUint32(data[:], i)
				filter.Add(data[:])
			}
			binary.LittleEndian.PutUint32(data[:], filter.Capacity()/2)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				filter.Contains(data[:])
			}
		})
	}
}

// BenchmarkContainsFalse benchmarks membership queries on an APBF for various
// capacities and false positive rates when the item does not exist in the
// filter.
func BenchmarkContainsFalse(b *testing.B) {
	benches := []struct {
		capacity uint32  // target capacity
		fpRate   float64 // target false positive rate
	}{{
		capacity: 1000,
		fpRate:   0.001,
	}, {
		capacity: 1000,
		fpRate:   0.0001,
	}, {
		capacity: 1000,
		fpRate:   0.00001,
	}, {
		capacity: 100000,
		fpRate:   0.0001,
	}, {
		capacity: 100000,
		fpRate:   0.000001,
	}}

	for benchIdx := range benches {
		bench := benches[benchIdx]
		benchName := fmt.Sprintf("capacity=%d/fprate=%0.6f", bench.capacity,
			bench.fpRate)
		b.Run(benchName, func(b *testing.B) {
			// Load the filter so the benchmark is with a max capacity filter.
			filter := NewFilter(bench.capacity, bench.fpRate)
			var data [4]byte
			for i := uint32(0); i < filter.Capacity(); i++ {
				binary.LittleEndian.PutUint32(data[:], i)
				filter.Add(data[:])
			}
			binary.LittleEndian.PutUint32(data[:], filter.Capacity()+1)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				filter.Contains(data[:])
			}
		})
	}
}

// BenchmarkReset benchmarks resetting an APBF for various capacities and false
// positive rates.
func BenchmarkReset(b *testing.B) {
	benches := []struct {
		capacity uint32  // target capacity
		fpRate   float64 // target false positive rate
	}{{
		capacity: 1000,
		fpRate:   0.001,
	}, {
		capacity: 1000,
		fpRate:   0.0001,
	}, {
		capacity: 1000,
		fpRate:   0.00001,
	}, {
		capacity: 10000,
		fpRate:   0.00001,
	}, {
		capacity: 100000,
		fpRate:   0.000001,
	}}

	for benchIdx := range benches {
		bench := benches[benchIdx]
		benchName := fmt.Sprintf("capacity=%d/fprate=%0.6f", bench.capacity,
			bench.fpRate)
		b.Run(benchName, func(b *testing.B) {
			filter := NewFilter(bench.capacity, bench.fpRate)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				filter.Reset()
			}
		})
	}
}

// BenchmarkNewFilter benchmarks creating a new APBF for various capacities and
// false positive rates.
func BenchmarkNewFilter(b *testing.B) {
	benches := []struct {
		capacity uint32  // target capacity
		fpRate   float64 // target false positive rate
	}{{
		capacity: 1000,
		fpRate:   0.001,
	}, {
		capacity: 1000,
		fpRate:   0.0001,
	}, {
		capacity: 1000,
		fpRate:   0.00001,
	}, {
		capacity: 10000,
		fpRate:   0.00001,
	}, {
		capacity: 100000,
		fpRate:   0.000001,
	}}

	var noElide *Filter
	for benchIdx := range benches {
		bench := benches[benchIdx]
		benchName := fmt.Sprintf("capacity=%d/fprate=%0.6f", bench.capacity,
			bench.fpRate)
		b.Run(benchName, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				noElide = NewFilter(bench.capacity, bench.fpRate)
			}
		})
	}
	_ = noElide
}
