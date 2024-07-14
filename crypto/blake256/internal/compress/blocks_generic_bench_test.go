// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// Benchmarks originally written by Dave Collins July 2024.

package compress

import (
	"testing"
)

// BenchmarkBlocks benchmarks how long it takes to compress a block of data with
// the pure Go block compression function implementation.
func BenchmarkBlocks(b *testing.B) {
	var state State
	var m [64]byte
	const counter = 0
	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(64)
	for i := 0; i < b.N; i++ {
		blocksGeneric(&state, m[:], counter)
	}
}
