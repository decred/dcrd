// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdscript

import (
	"testing"
)

// BenchmarkExtractStakePubKeyHashV0 benchmarks the performance of attempting to
// extract public key hashes from various version 0 stake-tagged public key
// scripts.
func BenchmarkExtractStakePubKeyHashV0(b *testing.B) {
	counts := make(map[ScriptType]int)
	benches := makeBenchmarks(func(test scriptTest) bool {
		// Limit to one of each script type.
		counts[test.wantType]++
		return counts[test.wantType] == 1 &&
			(test.wantType == STStakeSubmissionPubKeyHash ||
				test.wantType == STStakeGenPubKeyHash ||
				test.wantType == STStakeRevocationPubKeyHash ||
				test.wantType == STStakeChangePubKeyHash ||
				test.wantType == STTreasuryGenPubKeyHash)
	})

	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ExtractStakePubKeyHashV0(bench.script)
			}
		})
	}
}

// BenchmarkExtractStakeScriptHashV0 benchmarks the performance of attempting to
// extract script hashes from various version 0 stake-tagged public key scripts.
func BenchmarkExtractStakeScriptHashV0(b *testing.B) {
	counts := make(map[ScriptType]int)
	benches := makeBenchmarks(func(test scriptTest) bool {
		// Limit to one of each script type.
		counts[test.wantType]++
		return counts[test.wantType] == 1 &&
			(test.wantType == STStakeSubmissionScriptHash ||
				test.wantType == STStakeGenScriptHash ||
				test.wantType == STStakeRevocationScriptHash ||
				test.wantType == STStakeChangeScriptHash ||
				test.wantType == STTreasuryGenScriptHash)
	})

	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ExtractStakeScriptHashV0(bench.script)
			}
		})
	}
}
