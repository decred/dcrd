// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// Benchmarks originally written by Dave Collins July 2024.

//go:build !purego

package compress

import (
	"sync"
	"testing"
)

var skipsLogged = sync.Map{}

// BenchmarkBlocksAMD64 benchmarks how long it takes to compress a block of data
// with each of the specialized amd64 implementations along with the number of
// allocations needed.
func BenchmarkBlocksAMD64(b *testing.B) {
	benches := []struct {
		name      string
		fn        func(state *State, msg []byte, counter uint64)
		supported bool
	}{
		{name: "Pure Go", fn: blocksGeneric, supported: true},
		{name: "SSE2", fn: blocksSSE2, supported: hasSSE2},
		{name: "SSE41", fn: blocksSSE41, supported: hasSSE41},
		{name: "AVX", fn: blocksAVX, supported: hasAVX},
	}

	var state State
	var msg [64]byte
	const counter = 0

	for _, bench := range benches {
		if !bench.supported {
			if _, ok := skipsLogged.Load(bench.name); !ok {
				b.Logf("Skipping %s bench (disabled or no instruction set "+
					"support)", bench.name)
				skipsLogged.Store(bench.name, struct{}{})
			}
			continue
		}
		b.Run(bench.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(64)
			for i := 0; i < b.N; i++ {
				bench.fn(&state, msg[:], counter)
			}
		})
	}
}
