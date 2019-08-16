// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package lru

import (
	"testing"
)

// TestCache ensures the LRU Cache behaves as expected including limiting,
// eviction of least-recently used entries, specific entry removal, and
// existence tests.
func TestCache(t *testing.T) {
	// Create a bunch of fake nonces to use in testing the lru nonce code.
	numNonces := 10
	nonces := make([]uint64, 0, numNonces)
	for i := 0; i < numNonces; i++ {
		nonces = append(nonces, uint64(i))
	}

	tests := []struct {
		name  string
		limit int
	}{
		{name: "limit 0", limit: 0},
		{name: "limit 1", limit: 1},
		{name: "limit 5", limit: 5},
		{name: "limit 7", limit: 7},
		{name: "limit one less than available", limit: numNonces - 1},
		{name: "limit all available", limit: numNonces},
	}

testLoop:
	for i, test := range tests {
		// Create a new lru cache limited by the specified test limit and add
		// all of the test vectors.  This will cause eviction since there are
		// more test items than the limits.
		cache := NewCache(uint(test.limit))
		for j := 0; j < numNonces; j++ {
			cache.Add(nonces[j])
		}

		// Ensure the limited number of most recent entries in the list exist.
		for j := numNonces - test.limit; j < numNonces; j++ {
			if !cache.Contains(nonces[j]) {
				t.Errorf("Contains #%d (%s) entry %d does not exist", i,
					test.name, nonces[j])
				continue testLoop
			}
		}

		// Ensure the entries before the limited number of most recent entries
		// in the list do not exist.
		for j := 0; j < numNonces-test.limit; j++ {
			if cache.Contains(nonces[j]) {
				t.Errorf("Contains #%d (%s) entry %d exists", i, test.name,
					nonces[j])
				continue testLoop
			}
		}

		// Access the entry that should currently be the least-recently used
		// entry so it becomes the most-recently used entry, then force an
		// eviction by adding an entry that doesn't exist and ensure the evicted
		// entry is the new least-recently used entry.
		//
		// This check needs at least 2 entries.
		if test.limit > 1 {
			origLruIndex := numNonces - test.limit
			_ = cache.Contains(nonces[origLruIndex])

			cache.Add(uint64(numNonces) + 1)

			// Ensure the original lru entry still exists since it was updated
			// and should've have become the lru entry.
			if !cache.Contains(nonces[origLruIndex]) {
				t.Errorf("Contains #%d (%s) entry %d does not exist", i, test.name,
					nonces[origLruIndex])
				continue testLoop
			}

			// Ensure the entry that should've become the new lru entry was
			// evicted.
			newLruIndex := origLruIndex + 1
			if cache.Contains(nonces[newLruIndex]) {
				t.Errorf("Contains #%d (%s) entry %d exists", i, test.name,
					nonces[newLruIndex])
				continue testLoop
			}
		}

		// Add the entry that should currently be the least-recently used entry
		// again so it becomes the most-recently used entry, then force an
		// eviction by adding an entry that doesn't exist and ensure the evicted
		// entry is the new least-recently used entry.
		//
		// This check needs at least 2 entries.
		if test.limit > 1 {
			origLruIndex := numNonces - test.limit
			cache.Add(nonces[origLruIndex])

			cache.Add(uint64(numNonces) + 2)

			// Ensure the original lru entry still exists since it was updated
			// and should've have become the lru entry.
			if !cache.Contains(nonces[origLruIndex]) {
				t.Errorf("Contains #%d (%s) entry %d does not exist", i, test.name,
					nonces[origLruIndex])
				continue testLoop
			}

			// Ensure the entry that should've become the new lru entry was
			// evicted.
			newLruIndex := origLruIndex + 1
			if cache.Contains(nonces[newLruIndex]) {
				t.Errorf("Contains #%d (%s) entry %d exists", i, test.name,
					nonces[newLruIndex])
				continue testLoop
			}
		}

		// Delete all of the entries in the list, including those that don't
		// exist in the cache, and ensure they no longer exist.
		for j := 0; j < numNonces; j++ {
			cache.Delete(nonces[j])
			if cache.Contains(nonces[j]) {
				t.Errorf("Delete #%d (%s) entry %d exists", i, test.name,
					nonces[j])
				continue testLoop
			}
		}
	}
}

// BenchmarkCache performs basic benchmarks on the least recently used cache
// handling.
func BenchmarkCache(b *testing.B) {
	// Create a bunch of fake nonces to use in benchmarking the lru nonce code.
	b.StopTimer()
	numNonces := 100000
	nonces := make([]uint64, 0, numNonces)
	for i := 0; i < numNonces; i++ {
		nonces = append(nonces, uint64(i))
	}
	b.StartTimer()

	// Benchmark the add plus eviction code.
	limit := uint(20000)
	cache := NewCache(limit)
	for i := 0; i < b.N; i++ {
		cache.Add(nonces[i%numNonces])
	}
}
