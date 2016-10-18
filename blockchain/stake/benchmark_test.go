// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"testing"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
)

// BenchmarkHashPRNG benchmarks how long it takes to generate a random uint32
// using the hash-based deterministic PRNG.
func BenchmarkHashPRNG(b *testing.B) {
	seed := chainhash.HashFuncB([]byte{0x01})
	prng := NewHash256PRNG(seed)

	for n := 0; n < b.N; n++ {
		prng.Hash256Rand()
	}
}

// BenchmarkFindingVerdicts benchmarks how long it takes to iterate an
// a list of tallies and determine verdicts for the default mainnet
// parameters.
func BenchmarkFindingVerdicts(b *testing.B) {
	// Set up the cache and genesis block rolling tally.
	params := &chaincfg.MainNetParams
	cache := make(RollingVotingPrefixTallyCache)
	var bestTally RollingVotingPrefixTally
	bestTally.LastIntervalBlock = BlockKey{*params.GenesisHash, 0}

	// 347 intervals in blocks.
	numBlocks := 49968
	vbSlice := []uint16{}
	for i := 1; i < numBlocks; i++ {
		if int64(i) >= chaincfg.MainNetParams.StakeValidationHeight {
			switch i % 5 {
			case 0:
				vbSlice = []uint16{0x6665, 0x2345, 0x9999, 0xa0a1, 0xc432}
			case 1:
				vbSlice = []uint16{0x6687, 0x6689, 0x66bb, 0x66bb, 0x66e1}
			case 2:
				vbSlice = []uint16{0x3465, 0x5565, 0x6165, 0x65b6, 0xaaaa}
			case 3:
				vbSlice = []uint16{0xffff, 0x0000, 0x2342, 0x6444, 0xa333}
			case 4:
				vbSlice = []uint16{0x231b, 0xa343, 0xff34, 0x90bb}
			}
		}

		// Skip using the database.
		var err error
		bestTally, err = bestTally.ConnectBlockToTally(cache, nil,
			chainhash.Hash{byte(i)}, chainhash.Hash{byte(i - 1)}, uint32(i),
			vbSlice, &chaincfg.MainNetParams)
		if err != nil {
			b.Fatalf("failed")
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bestTally.GenerateVotingResults(cache,
			nil, chaincfg.MainNetParams.VotingIntervals,
			&chaincfg.MainNetParams)
	}

}
