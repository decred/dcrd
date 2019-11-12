package lru

import (
	"fmt"
	"testing"
)

type intkey int

// TestKVCache ensures the KV LRU Cache behaves as expected including limiting,
// eviction of least-recently used entries, specific entry removal, and
// existence tests.
func TestKVCache(t *testing.T) {
	// Create a bunch of fake nonces and keys to use in testing the lru nonce code.
	numNonces := 10
	nonces := make([]uint64, 0, numNonces)
	keys := make([]intkey, 0, numNonces)
	for i := 0; i < numNonces; i++ {
		nonces = append(nonces, uint64(i))
		keys = append(keys, intkey(i))
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
		cache := NewKVCache(uint(test.limit))
		for j := 0; j < numNonces; j++ {
			cache.Add(keys[j], nonces[j])
		}

		// Ensure the limited number of most recent entries in the list exist.
		for j := numNonces - test.limit; j < numNonces; j++ {
			if !cache.Contains(keys[j]) {
				t.Errorf("Contains #%d (%s) entry with key %d "+
					"does not exist", i, test.name, keys[j])
				continue testLoop
			}
		}

		// Ensure each key corresponds to its expected value.
		for j := numNonces - test.limit; j < numNonces; j++ {
			value, exists := cache.Lookup(keys[j])
			if !exists {
				t.Errorf("Contains #%d (%s) entry with key %d "+
					"does not exist", i, test.name, keys[j])
				continue testLoop
			}

			v := value.(uint64)
			if uint64(keys[j]) != v {
				t.Errorf("Contains #%d (%s) entry with key %d "+
					"does not have expected value %d, got %d", i, test.name,
					keys[j], nonces[j], v)
				continue testLoop
			}
		}

		// Ensure the entries before the limited number of most recent entries
		// in the list do not exist.
		for j := 0; j < numNonces-test.limit; j++ {
			if cache.Contains(keys[j]) {
				t.Errorf("Contains #%d (%s) entry with key %d "+
					"exists", i, test.name, keys[j])
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
			_ = cache.Contains(keys[origLruIndex])

			newNonce := uint64(numNonces) + 1
			cache.Add(intkey(int(newNonce)), newNonce)

			// Ensure the original lru entry still exists since it was updated
			// and should have become the lru entry.
			if !cache.Contains(keys[origLruIndex]) {
				t.Errorf("Contains #%d (%s) entry with key %d "+
					"does not exist", i, test.name, keys[origLruIndex])
				continue testLoop
			}

			// Ensure the entry that should've become the new lru entry was
			// evicted.
			newLruIndex := origLruIndex + 1
			if cache.Contains(keys[newLruIndex]) {
				t.Errorf("Contains #%d (%s) entry with key %d exists",
					i, test.name, keys[newLruIndex])
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
			cache.Add(keys[origLruIndex], nonces[origLruIndex])

			newNonce := uint64(numNonces) + 2
			cache.Add(intkey(int(newNonce)), newNonce)

			// Ensure the original lru entry still exists since it was updated
			// and should've have become the lru entry.
			if !cache.Contains(keys[origLruIndex]) {
				t.Errorf("Contains #%d (%s) entry with key %d "+
					"does not exist", i, test.name, keys[origLruIndex])
				continue testLoop
			}

			// Ensure the entry that should've become the new lru entry was
			// evicted.
			newLruIndex := origLruIndex + 1
			if cache.Contains(keys[newLruIndex]) {
				t.Errorf("Contains #%d (%s) entry with key %d exists",
					i, test.name, keys[newLruIndex])
				continue testLoop
			}
		}

		// Ensure an addition using an existing key updates the entry value.
		//
		// This check needs at least 1 entry.
		if test.limit > 1 {
			oldValue, _ := cache.Lookup(keys[0])
			cache.Add(keys[0], 100)
			newValue, _ := cache.Lookup(keys[0])
			if oldValue == newValue {
				t.Errorf("Contains #%d (%s) addition on key %d did not update "+
					"value", i, test.name, keys[0])
				continue testLoop
			}
		}

		// Delete all of the entries in the list, including those that don't
		// exist in the cache, and ensure they no longer exist.
		for j := 0; j < numNonces; j++ {
			cache.Delete(keys[j])
			if cache.Contains(keys[j]) {
				t.Errorf("Delete #%d (%s) entry with key %d exists",
					i, test.name, keys[j])
				continue testLoop
			}
		}
	}
}

// BenchmarkKV performs basic benchmarks on the least recently used cache
// handling.
func BenchmarkKV(b *testing.B) {
	// Create a bunch of fake nonces to use in benchmarking the lru nonce code.
	b.StopTimer()
	numNonces := 100000
	nonces := make([]uint64, 0, numNonces)
	keys := make([]string, 0, numNonces)
	for i := 0; i < numNonces; i++ {
		nonces = append(nonces, uint64(i))
		keys = append(keys, fmt.Sprintf("k%d", i))
	}
	b.StartTimer()

	// Benchmark the add plus eviction code.
	limit := uint(20000)
	cache := NewKVCache(limit)
	for i := 0; i < b.N; i++ {
		cache.Add(keys[i%numNonces], nonces[i%numNonces])
	}
}
