// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/wire"
)

// Define constants for indicating flags throughout the tests.
const (
	noCoinbase   = false
	withCoinbase = true
	noExpiry     = false
	withExpiry   = true
)

// outpoint299 returns a test outpoint from block height 299 that can be used
// throughout the tests.
func outpoint299() wire.OutPoint {
	return wire.OutPoint{
		Hash: *mustParseHash("e299d2cc5deb5b39d230ad2a6046ff9cc164064f431a289" +
			"3eb628b467d018452"),
		Index: 0,
		Tree:  wire.TxTreeRegular,
	}
}

// entry299 returns a utxo entry from block height 299 that can be used
// throughout the tests.
func entry299() *UtxoEntry {
	return &UtxoEntry{
		amount: 58795424,
		pkScript: hexToBytes("76a914454017705ab80470d089c7f644e39cc9e0fd308e" +
			"88ac"),
		blockHeight:   299,
		blockIndex:    1,
		scriptVersion: 0,
		packedFlags: encodeUtxoFlags(
			noCoinbase,
			noExpiry,
			stake.TxTypeRegular,
		),
	}
}

// outpoint1100 returns a test outpoint from block height 1100 that can be used
// throughout the tests.
func outpoint1100() wire.OutPoint {
	return wire.OutPoint{
		Hash: *mustParseHash("ce1d0f74440c391d15516015224755a8661e56e796ac254" +
			"90f30ad1081c5d638"),
		Index: 1,
		Tree:  wire.TxTreeRegular,
	}
}

// entry1100 returns a utxo entry from block height 1100 that can be used
// throughout the tests.
func entry1100() *UtxoEntry {
	return &UtxoEntry{
		amount: 52454022,
		pkScript: hexToBytes("76a9146b65f16ebca9b848158701d5a2eb5124547a2144" +
			"88ac"),
		blockHeight:   1100,
		blockIndex:    1,
		scriptVersion: 0,
		packedFlags: encodeUtxoFlags(
			noCoinbase,
			noExpiry,
			stake.TxTypeRegular,
		),
	}
}

// outpoint1200 returns a test outpoint from block height 1200 that can be used
// throughout the tests.
func outpoint1200() wire.OutPoint {
	return wire.OutPoint{
		Hash: *mustParseHash("72914cae2d4bc75f7777373b7c085c4b92d59f3e059fc7f" +
			"d39def71c9fe188b5"),
		Index: 2,
		Tree:  wire.TxTreeRegular,
	}
}

// entry1200 returns a utxo entry from block height 1200 that can be used
// throughout the tests.
func entry1200() *UtxoEntry {
	return &UtxoEntry{
		amount: 1871749598,
		pkScript: hexToBytes("76a9142ec5027abadede723c47b6acdbace3be10b7e937" +
			"88ac"),
		blockHeight:   1200,
		blockIndex:    0,
		scriptVersion: 0,
		packedFlags: encodeUtxoFlags(
			withCoinbase,
			noExpiry,
			stake.TxTypeRegular,
		),
	}
}

// outpoint85314 returns a test outpoint from block height 85314 that can be
// used throughout the tests.
func outpoint85314() wire.OutPoint {
	return wire.OutPoint{
		Hash: *mustParseHash("d3bce77da2747baa85fb7ca4f6f8e123f31cd15ac691b2f" +
			"82543780158587d3a"),
		Index: 0,
		Tree:  wire.TxTreeStake,
	}
}

// entry85314 returns a utxo entry from block height 85314 that can be used
// throughout the tests.
func entry85314() *UtxoEntry {
	return &UtxoEntry{
		amount: 4294959555,
		pkScript: hexToBytes("ba76a914a13afb81d54c9f8bb0c5e082d56fd563ab9b359" +
			"688ac"),
		blockHeight:   85314,
		blockIndex:    6,
		scriptVersion: 0,
		packedFlags: encodeUtxoFlags(
			noCoinbase,
			withExpiry,
			stake.TxTypeSStx,
		),
		ticketMinOuts: &ticketMinimalOutputs{
			data: hexToBytes("03808efefade57001aba76a914a13afb81d54c9f8bb0c5e" +
				"082d56fd563ab9b359688ac0000206a1e9ac39159847e259c9162405b5f6" +
				"c8135d2c7eaf1a375040001000000005800001abd76a9140000000000000" +
				"00000000000000000000000000088ac"),
		},
	}
}

// testUtxoCache provides a mock utxo cache by implementing the UtxoCacher
// interface.  It allows for toggling flushing on and off to more easily
// simulate various scenarios.
type testUtxoCache struct {
	*UtxoCache
	disableFlush bool
}

// MaybeFlush conditionally flushes the cache to the backend.  If the disable
// flush flag is set on the test utxo cache, this function will return
// immediately without attempting to flush the cache.
func (c *testUtxoCache) MaybeFlush(bestHash *chainhash.Hash, bestHeight uint32,
	forceFlush bool, logFlush bool) error {

	// Return immediately if disable flush is set.
	if c.disableFlush {
		return nil
	}

	return c.UtxoCache.MaybeFlush(bestHash, bestHeight, forceFlush, logFlush)
}

// newTestUtxoCache returns a testUtxoCache instance using the provided
// configuration details.
func newTestUtxoCache(config *UtxoCacheConfig) *testUtxoCache {
	return &testUtxoCache{
		UtxoCache: NewUtxoCache(config),
	}
}

// Ensure testUtxoCache implements the UtxoCacher interface.
var _ UtxoCacher = (*testUtxoCache)(nil)

// createTestUtxoCache creates a test utxo cache with the specified entries.
func createTestUtxoCache(t *testing.T, entries map[wire.OutPoint]*UtxoEntry) *UtxoCache {
	t.Helper()

	utxoCache := NewUtxoCache(&UtxoCacheConfig{
		FlushBlockDB: func() error {
			return nil
		},
	})
	for outpoint, entry := range entries {
		// Add the entry to the cache.  The entry is cloned before being added
		// so that any modifications that the cache makes to the entry are not
		// reflected in the provided test entry.
		err := utxoCache.AddEntry(outpoint, entry.Clone())
		if err != nil {
			t.Fatalf("unexpected error when adding entry: %v", err)
		}

		// Set the state of the cached entries based on the provided entries.
		// This is allowed for tests to easily simulate entries in the cache
		// that are not fresh without having to fetch them from the backend.
		cachedEntry := utxoCache.entries[outpoint]
		if cachedEntry != nil {
			cachedEntry.state = entry.state
		}
	}
	return utxoCache
}

// TestTotalSize validates that the correct number of bytes is returned for the
// size of the utxo cache.
func TestTotalSize(t *testing.T) {
	t.Parallel()

	// Create test entries to be used throughout the tests.
	outpointRegular := outpoint1200()
	entryRegular := entry1200()
	outpointTicket := outpoint85314()
	entryTicket := entry85314()

	tests := []struct {
		name    string
		entries map[wire.OutPoint]*UtxoEntry
		want    uint64
	}{{
		name:    "without any entries",
		entries: map[wire.OutPoint]*UtxoEntry{},
		want:    0,
	}, {
		name: "with entries",
		entries: map[wire.OutPoint]*UtxoEntry{
			outpointRegular: entryRegular,
			outpointTicket:  entryTicket,
		},
		// mapOverhead*numEntries + outpointSize*numEntries +
		// pointerSize*numEntries + (first entry: base entry size +
		// len(pkScript)) + (second entry: base entry size + len(pkScript) +
		// len(ticketMinOuts.data))
		want: mapOverhead*2 + outpointSize*2 + pointerSize*2 +
			(baseEntrySize + 25) + (baseEntrySize + 26 + 99),
	}}

	for _, test := range tests {
		// Create a utxo cache with the entries specified by the test.
		utxoCache := createTestUtxoCache(t, test.entries)

		// Validate that total size returns the expected value.
		got := utxoCache.totalSize()
		if got != test.want {
			t.Errorf("%q: unexpected result -- got %d, want %d", test.name, got,
				test.want)
		}
	}
}

// TestHitRatio validates that the correct hit ratio is returned based on the
// number of cache hits and misses.
func TestHitRatio(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		hits   uint64
		misses uint64
		want   float64
	}{{
		name: "no hits or misses",
		want: 100,
	}, {
		name: "all hits, no misses",
		hits: 50,
		want: 100,
	}, {
		name:   "98.5% hit ratio",
		hits:   197,
		misses: 3,
		want:   98.5,
	}}

	for _, test := range tests {
		// Create a utxo cache with hits and misses as specified by the test.
		utxoCache := NewUtxoCache(&UtxoCacheConfig{})
		utxoCache.hits = test.hits
		utxoCache.misses = test.misses

		// Validate that hit ratio returns the expected value.
		got := utxoCache.hitRatio()
		if got != test.want {
			t.Errorf("%q: unexpected result -- got %f, want %f", test.name, got,
				test.want)
		}
	}
}

// TestAddEntry validates that entries are added to the cache properly under a
// variety of conditions.
func TestAddEntry(t *testing.T) {
	t.Parallel()

	// Create test entries to be used throughout the tests.
	outpoint := outpoint299()
	entry := entry299()
	entryModified := entry.Clone()
	entryModified.amount++
	entryModified.state |= utxoStateModified
	entryFresh := entry.Clone()
	entryFresh.state |= utxoStateModified | utxoStateFresh

	tests := []struct {
		name            string
		existingEntries map[wire.OutPoint]*UtxoEntry
		outpoint        wire.OutPoint
		entry           *UtxoEntry
		wantEntry       *UtxoEntry
	}{{
		name:      "add an entry that does not already exist in the cache",
		outpoint:  outpoint,
		entry:     entry,
		wantEntry: entryFresh,
	}, {
		name: "add an entry that overwrites an existing entry",
		existingEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint: entry,
		},
		outpoint:  outpoint,
		entry:     entryModified,
		wantEntry: entryModified,
	}}

	for _, test := range tests {
		// Create a utxo cache with the existing entries specified by the test.
		utxoCache := createTestUtxoCache(t, test.existingEntries)
		wantTotalEntrySize := utxoCache.totalEntrySize

		// Attempt to get an existing entry from the cache.  If it exists,
		// subtract its size from the expected total entry size since it will be
		// overwritten.
		existingEntry := utxoCache.entries[test.outpoint]
		if existingEntry != nil {
			wantTotalEntrySize -= test.entry.size()
		}

		// Add the entry specified by the test.
		err := utxoCache.AddEntry(test.outpoint, test.entry)
		if err != nil {
			t.Fatalf("%q: unexpected error when adding entry: %v", test.name,
				err)
		}
		wantTotalEntrySize += test.entry.size()

		// Attempt to get the added entry from the cache.
		cachedEntry := utxoCache.entries[test.outpoint]

		// Validate that the added entry exists in the cache.
		if cachedEntry == nil {
			t.Fatalf("%q: expected entry for outpoint %v to exist in the cache",
				test.name, test.outpoint)
		}

		// Validate that the entry is marked as modified.
		if !cachedEntry.isModified() {
			t.Fatalf("%q: unexpected modified flag -- got false, want true",
				test.name)
		}

		// Validate that the cached entry matches the expected entry.
		if !reflect.DeepEqual(cachedEntry, test.wantEntry) {
			t.Fatalf("%q: mismatched cached entry:\nwant: %+v\n got: %+v\n",
				test.name, test.wantEntry, cachedEntry)
		}

		// Validate that the total entry size was updated as expected.
		if utxoCache.totalEntrySize != wantTotalEntrySize {
			t.Fatalf("%q: unexpected total entry size -- got %v, want %v",
				test.name, utxoCache.totalEntrySize, wantTotalEntrySize)
		}
	}
}

// TestSpendEntry validates that entries in the cache are properly updated when
// being spent under a variety of conditions.
func TestSpendEntry(t *testing.T) {
	t.Parallel()

	// Create test entries to be used throughout the tests.
	outpoint := outpoint299()
	entry := entry299()
	entryFresh := entry.Clone()
	entryFresh.state |= utxoStateModified | utxoStateFresh
	entrySpent := entry.Clone()
	entrySpent.Spend()

	tests := []struct {
		name            string
		existingEntries map[wire.OutPoint]*UtxoEntry
		outpoint        wire.OutPoint
		entry           *UtxoEntry
	}{{
		name:     "spend an entry that does not exist in the cache",
		outpoint: outpoint,
		entry:    entry,
	}, {
		name: "spend an entry that exists in the cache but is already spent",
		existingEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint: entrySpent,
		},
		outpoint: outpoint,
		entry:    entrySpent,
	}, {
		name: "spend an entry that exists in the cache and is fresh",
		existingEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint: entryFresh,
		},
		outpoint: outpoint,
		entry:    entryFresh,
	}, {
		name: "spend an entry that exists in the cache and is not fresh",
		existingEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint: entry,
		},
		outpoint: outpoint,
		entry:    entry,
	}}

	for _, test := range tests {
		// Create a utxo cache with the existing entries specified by the test.
		utxoCache := createTestUtxoCache(t, test.existingEntries)
		wantTotalEntrySize := utxoCache.totalEntrySize

		// Attempt to get an existing entry from the cache.
		entry := utxoCache.entries[test.outpoint]
		var entryAlreadySpent bool
		if entry != nil {
			entryAlreadySpent = entry.IsSpent()
		}

		// Spend the entry specified by the test.
		utxoCache.SpendEntry(test.outpoint)

		// If the existing entry was nil or spent, continue as there is nothing
		// else to validate.
		if entry == nil || entryAlreadySpent {
			continue
		}

		// If the entry is fresh, validate that it was removed from the cache
		// when spent.
		if entry.isFresh() {
			wantTotalEntrySize -= test.entry.size()
			if utxoCache.entries[test.outpoint] != nil {
				t.Fatalf("%q: entry for outpoint %v was not removed from the "+
					"cache", test.name, test.outpoint)
			}
		}

		// Validate that the total entry size was updated as expected.
		if utxoCache.totalEntrySize != wantTotalEntrySize {
			t.Fatalf("%q: unexpected total entry size -- got %v, want %v",
				test.name, utxoCache.totalEntrySize, wantTotalEntrySize)
		}

		// If entry is not fresh, validate that it still exists in the cache and
		// is now marked as spent.
		if !entry.isFresh() {
			cachedEntry := utxoCache.entries[test.outpoint]
			if cachedEntry == nil || !cachedEntry.IsSpent() {
				t.Fatalf("%q: expected entry for outpoint %v to exist in the "+
					"cache and be marked spent", test.name, test.outpoint)
			}
		}
	}
}

// TestFetchEntry validates that fetch entry returns the correct entry under a
// variety of conditions.
func TestFetchEntry(t *testing.T) {
	t.Parallel()

	// Create a test backend.
	backend := createTestUtxoBackend(t)

	// Create test entries to be used throughout the tests.
	outpoint := outpoint299()
	entry := entry299()
	entryModified := entry.Clone()
	entryModified.state |= utxoStateModified

	tests := []struct {
		name           string
		cachedEntries  map[wire.OutPoint]*UtxoEntry
		backendEntries map[wire.OutPoint]*UtxoEntry
		outpoint       wire.OutPoint
		cacheHit       bool
		wantEntry      *UtxoEntry
	}{{
		name:     "entry is not in the cache or the backend",
		outpoint: outpoint,
	}, {
		name: "entry is in the cache",
		cachedEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint: entry,
		},
		outpoint:  outpoint,
		cacheHit:  true,
		wantEntry: entry,
	}, {
		name: "entry is not in the cache but is in the backend",
		backendEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint: entryModified,
		},
		outpoint:  outpoint,
		wantEntry: entry,
	}}

	for _, test := range tests {
		// Create a utxo cache with the cached entries specified by the test.
		utxoCache := createTestUtxoCache(t, test.cachedEntries)
		utxoCache.backend = backend
		wantTotalEntrySize := utxoCache.totalEntrySize

		// Add entries specified by the test to the test backend.
		err := backend.PutUtxos(test.backendEntries, &UtxoSetState{})
		if err != nil {
			t.Fatalf("%q: unexpected error adding entries to test backend: %v",
				test.name, err)
		}

		// Attempt to fetch the entry for the outpoint specified by the test.
		entry, err = utxoCache.FetchEntry(test.outpoint)
		if err != nil {
			t.Fatalf("%q: unexpected error fetching entry: %v", test.name, err)
		}

		// Ensure that the fetched entry matches the expected entry.
		if !reflect.DeepEqual(entry, test.wantEntry) {
			t.Fatalf("%q: mismatched entry:\nwant: %+v\n got: %+v\n", test.name,
				test.wantEntry, entry)
		}

		// Ensure that the entry is now cached.
		cachedEntry := utxoCache.entries[test.outpoint]
		if !reflect.DeepEqual(cachedEntry, test.wantEntry) {
			t.Fatalf("%q: mismatched cached entry:\nwant: %+v\n got: %+v\n",
				test.name, test.wantEntry, cachedEntry)
		}

		// Validate the cache hits and misses counts.
		if test.cacheHit && utxoCache.hits != 1 {
			t.Fatalf("%q: unexpected cache hits -- got %v, want 1", test.name,
				utxoCache.hits)
		}
		if !test.cacheHit && utxoCache.misses != 1 {
			t.Fatalf("%q: unexpected cache misses -- got %v, want 1", test.name,
				utxoCache.misses)
		}

		// Validate that the total entry size was updated as expected.
		if !test.cacheHit && cachedEntry != nil {
			wantTotalEntrySize += cachedEntry.size()
		}
		if utxoCache.totalEntrySize != wantTotalEntrySize {
			t.Fatalf("%q: unexpected total entry size -- got %v, want %v",
				test.name, utxoCache.totalEntrySize, wantTotalEntrySize)
		}
	}
}

// TestFetchEntries validates that the provided view is populated with the
// requested entries as expected.
func TestFetchEntries(t *testing.T) {
	t.Parallel()

	// Create a test backend.
	backend := createTestUtxoBackend(t)

	// Create test entries to be used throughout the tests.
	outpoint299 := outpoint299()
	outpoint1100 := outpoint1100()
	entry1100 := entry1100()
	outpoint1200 := outpoint1200()
	entry1200 := entry1200()
	entry1200Modified := entry1200.Clone()
	entry1200Modified.state |= utxoStateModified

	tests := []struct {
		name           string
		cachedEntries  map[wire.OutPoint]*UtxoEntry
		backendEntries map[wire.OutPoint]*UtxoEntry
		filteredSet    ViewFilteredSet
		wantEntries    map[wire.OutPoint]*UtxoEntry
	}{{
		name: "entries are fetched from the cache and the backend",
		cachedEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint1100: entry1100,
		},
		backendEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint1200: entry1200Modified,
		},
		filteredSet: ViewFilteredSet{
			outpoint299:  struct{}{},
			outpoint1100: struct{}{},
			outpoint1200: struct{}{},
		},
		wantEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint299:  nil,
			outpoint1100: entry1100,
			outpoint1200: entry1200,
		},
	}}

	for _, test := range tests {
		// Create a utxo cache with the cached entries specified by the test.
		utxoCache := createTestUtxoCache(t, test.cachedEntries)
		utxoCache.backend = backend

		// Add entries specified by the test to the test backend.
		err := backend.PutUtxos(test.backendEntries, &UtxoSetState{})
		if err != nil {
			t.Fatalf("%q: unexpected error adding entries to test backend: %v",
				test.name, err)
		}

		// Fetch the entries requested by the test and add them to a view.
		view := NewUtxoViewpoint(utxoCache)
		err = utxoCache.FetchEntries(test.filteredSet, view)
		if err != nil {
			t.Fatalf("%q: unexpected error fetching entries for view: %v",
				test.name, err)
		}

		// Ensure that the fetched entries match the expected entries.
		if !reflect.DeepEqual(view.entries, test.wantEntries) {
			t.Fatalf("%q: mismatched entries:\nwant: %+v\n got: %+v\n",
				test.name, test.wantEntries, view.entries)
		}
	}
}

// TestCommit validates that all entries in both the cache and the provided view
// are updated appropriately when committing the provided view to the cache.
func TestCommit(t *testing.T) {
	t.Parallel()

	// Create test entries to be used throughout the tests.
	outpoint299 := outpoint299()
	outpoint1100 := outpoint1100()
	entry1100Unmodified := entry1100()
	outpoint1200 := outpoint1200()
	entry1200 := entry1200()
	entry1200Spent := entry1200.Clone()
	entry1200Spent.Spend()
	outpoint85314 := outpoint85314()
	entry85314Modified := entry85314()
	entry85314Modified.state |= utxoStateModified

	tests := []struct {
		name              string
		viewEntries       map[wire.OutPoint]*UtxoEntry
		cachedEntries     map[wire.OutPoint]*UtxoEntry
		wantViewEntries   map[wire.OutPoint]*UtxoEntry
		wantCachedEntries map[wire.OutPoint]*UtxoEntry
	}{{
		name: "view contains nil, unmodified, spent, and modified entries",
		viewEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint299:   nil,
			outpoint1100:  entry1100Unmodified,
			outpoint1200:  entry1200Spent,
			outpoint85314: entry85314Modified,
		},
		cachedEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint1200: entry1200,
		},
		// outpoint299 is removed from the view since the entry is nil.
		// entry1100Unmodified remains in the view since it is unmodified.
		// entry1200Spent is removed from the view since the entry is spent.
		// entry85314Modified is removed from the view since it is modified and
		// added to the cache.
		wantViewEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint1100: entry1100Unmodified,
		},
		// entry1200Spent remains in the cache but is now spent.
		// entry85314Modified is added to the cache.
		wantCachedEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint1200:  entry1200Spent,
			outpoint85314: entry85314Modified,
		},
	}}

	for _, test := range tests {
		// Create a utxo cache with the cached entries specified by the test.
		utxoCache := createTestUtxoCache(t, test.cachedEntries)

		// Create a utxo cache with the view entries specified by the test.
		view := &UtxoViewpoint{
			cache:   utxoCache,
			entries: test.viewEntries,
		}

		// Commit the view to the cache.
		err := utxoCache.Commit(view)
		if err != nil {
			t.Fatalf("%q: unexpected error committing view to the cache: %v",
				test.name, err)
		}

		// Validate the cached entries after committing.
		if !reflect.DeepEqual(utxoCache.entries, test.wantCachedEntries) {
			t.Fatalf("%q: mismatched cached entries:\nwant: %+v\n got: %+v\n",
				test.name, test.wantCachedEntries, utxoCache.entries)
		}

		// Validate the view entries after committing.
		if !reflect.DeepEqual(view.entries, test.wantViewEntries) {
			t.Fatalf("%q: mismatched view entries:\nwant: %+v\n got: %+v\n",
				test.name, test.wantViewEntries, view.entries)
		}
	}
}

// TestCalcEvictionHeight validates that the correct eviction height is returned
// based on the provided best height and the last eviction height.
func TestCalcEvictionHeight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		lastEvictionHeight uint32
		bestHeight         uint32
		want               uint32
	}{{
		name:       "no last eviction",
		bestHeight: 100,
		want:       15,
	}, {
		name:               "best height less than last eviction height",
		lastEvictionHeight: 101,
		bestHeight:         100,
		want:               100,
	}, {
		name:               "best height greater than last eviction height",
		lastEvictionHeight: 99,
		bestHeight:         200,
		want:               115,
	}}

	for _, test := range tests {
		// Create a utxo cache with the last eviction height as specified by the
		// test.
		utxoCache := NewUtxoCache(&UtxoCacheConfig{})
		utxoCache.lastEvictionHeight = test.lastEvictionHeight

		// Validate that calc eviction height returns the expected value.
		got := utxoCache.calcEvictionHeight(test.bestHeight)
		if got != test.want {
			t.Errorf("%q: unexpected result -- got %d, want %d", test.name, got,
				test.want)
		}
	}
}

// TestShouldFlush validates that it is correctly determined whether or not a
// flush should be performed given various conditions.
func TestShouldFlush(t *testing.T) {
	t.Parallel()

	// Create test hashes to be used throughout the tests.
	block1000Hash := mustParseHash("0000000000004740ad140c86753f9295e09f9cc81" +
		"b1bb75d7f5552aeeedb7012")
	block2000Hash := mustParseHash("0000000000000c8a886e3f7c32b1bb08422066dcf" +
		"d008de596471f11a5aff475")

	tests := []struct {
		name           string
		totalEntrySize uint64
		maxSize        uint64
		lastFlushTime  time.Time
		lastFlushHash  *chainhash.Hash
		bestHash       *chainhash.Hash
		want           bool
	}{{
		name:           "already flushed through the best hash",
		totalEntrySize: 100,
		maxSize:        1000,
		lastFlushTime:  time.Now(),
		lastFlushHash:  block1000Hash,
		bestHash:       block1000Hash,
		want:           false,
	}, {
		name:           "less than max size and periodic duration not reached",
		totalEntrySize: 100,
		maxSize:        1000,
		lastFlushTime:  time.Now(),
		lastFlushHash:  block1000Hash,
		bestHash:       block2000Hash,
		want:           false,
	}, {
		name:           "equal to max size",
		totalEntrySize: 1000,
		maxSize:        1000,
		lastFlushTime:  time.Now(),
		lastFlushHash:  block1000Hash,
		bestHash:       block2000Hash,
		want:           true,
	}, {
		name:           "greater than max size",
		totalEntrySize: 1001,
		maxSize:        1000,
		lastFlushTime:  time.Now(),
		lastFlushHash:  block1000Hash,
		bestHash:       block2000Hash,
		want:           true,
	}, {
		name:           "less than max size but periodic duration reached",
		totalEntrySize: 100,
		maxSize:        1000,
		lastFlushTime:  time.Now().Add(periodicFlushInterval * -1),
		lastFlushHash:  block1000Hash,
		bestHash:       block2000Hash,
		want:           true,
	}}

	for _, test := range tests {
		// Create a utxo cache and set the field values as specified by the
		// test.
		utxoCache := NewUtxoCache(&UtxoCacheConfig{
			MaxSize: test.maxSize,
		})
		utxoCache.totalEntrySize = test.totalEntrySize
		utxoCache.lastFlushTime = test.lastFlushTime
		utxoCache.lastFlushHash = *test.lastFlushHash

		// Validate that should flush returns the expected value.
		got := utxoCache.shouldFlush(test.bestHash)
		if got != test.want {
			t.Errorf("%q: unexpected result -- got %v, want %v", test.name, got,
				test.want)
		}
	}
}

// TestMaybeFlush validates that the cache is properly flushed to the backend
// under a variety of conditions.
func TestMaybeFlush(t *testing.T) {
	t.Parallel()

	// Create a test backend.
	backend := createTestUtxoBackend(t)

	// Create test hashes to be used throughout the tests.
	block1000Hash := mustParseHash("0000000000004740ad140c86753f9295e09f9cc81" +
		"b1bb75d7f5552aeeedb7012")
	block2000Hash := mustParseHash("0000000000000c8a886e3f7c32b1bb08422066dcf" +
		"d008de596471f11a5aff475")

	// entry299Fresh is from block height 299 and is modified and fresh.
	outpoint299 := outpoint299()
	entry299Fresh := entry299()
	entry299Fresh.state |= utxoStateModified | utxoStateFresh

	// entry299Unmodified is from block height 299 and is unmodified.
	entry299Unmodified := entry299()

	// entry1100Spent is from block height 1100 and is modified and spent.
	outpoint1100 := outpoint1100()
	entry1100Spent := entry1100()
	entry1100Spent.Spend()

	// entry1100Modified is from block height 1100 and is modified and unspent.
	entry1100Modified := entry1100()
	entry1100Modified.state |= utxoStateModified

	// entry1100Unmodified is from block height 1100 and is unspent and
	// unmodified.
	entry1100Unmodified := entry1100()

	// entry1200Fresh is from block height 1200 and is modified and fresh.
	outpoint1200 := outpoint1200()
	entry1200Fresh := entry1200()
	entry1200Fresh.state |= utxoStateModified | utxoStateFresh

	// entry1200Unmodified is from block height 1200 and is unmodified.
	entry1200Unmodified := entry1200()

	tests := []struct {
		name                     string
		maxSize                  uint64
		lastEvictionHeight       uint32
		lastFlushHash            *chainhash.Hash
		bestHash                 *chainhash.Hash
		bestHeight               uint32
		forceFlush               bool
		cachedEntries            map[wire.OutPoint]*UtxoEntry
		backendEntries           map[wire.OutPoint]*UtxoEntry
		wantCachedEntries        map[wire.OutPoint]*UtxoEntry
		wantBackendEntries       map[wire.OutPoint]*UtxoEntry
		wantLastEvictionHeight   uint32
		wantLastFlushHash        *chainhash.Hash
		wantUpdatedLastFlushTime bool
	}{{
		name:               "flush not required",
		maxSize:            1000,
		lastEvictionHeight: 0,
		lastFlushHash:      block1000Hash,
		bestHash:           block2000Hash,
		bestHeight:         2000,
		cachedEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint299:  entry299Fresh,
			outpoint1100: entry1100Spent,
			outpoint1200: entry1200Fresh,
		},
		backendEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint1100: entry1100Modified,
		},
		// The cache should remain unchanged since a flush is not required.
		wantCachedEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint299:  entry299Fresh,
			outpoint1100: entry1100Spent,
			outpoint1200: entry1200Fresh,
		},
		// The backend should remain unchanged since a flush is not required.
		wantBackendEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint1100: entry1100Unmodified,
		},
		wantLastEvictionHeight:   0,
		wantLastFlushHash:        block1000Hash,
		wantUpdatedLastFlushTime: false,
	}, {
		name:               "all entries flushed, some entries evicted",
		maxSize:            0,
		lastEvictionHeight: 0,
		lastFlushHash:      block1000Hash,
		bestHash:           block2000Hash,
		bestHeight:         2000,
		cachedEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint299:  entry299Fresh,
			outpoint1100: entry1100Spent,
			outpoint1200: entry1200Fresh,
		},
		backendEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint1100: entry1100Modified,
		},
		// entry299Fresh should be evicted from the cache due to its height.
		// entry1100Spent should be evicted since it is spent.
		// entry1200Fresh should remain in the cache but should now be
		// unmodified.
		wantCachedEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint1200: entry1200Unmodified,
		},
		// entry299Unmodified should be added to the backend during the flush.
		// entry1100Unmodified should be removed from the backend since it now
		// spent.
		// entry1200Unmodified should be added to the backend during the flush.
		wantBackendEntries: map[wire.OutPoint]*UtxoEntry{
			outpoint299:  entry299Unmodified,
			outpoint1200: entry1200Unmodified,
		},
		wantLastEvictionHeight:   300,
		wantLastFlushHash:        block2000Hash,
		wantUpdatedLastFlushTime: true,
	}}

	for _, test := range tests {
		// Create a utxo cache with the cached entries specified by the test.
		utxoCache := createTestUtxoCache(t, test.cachedEntries)
		utxoCache.backend = backend
		utxoCache.maxSize = test.maxSize
		utxoCache.lastEvictionHeight = test.lastEvictionHeight
		utxoCache.lastFlushHash = *test.lastFlushHash

		// Mock the current time as 1 minute after the last flush time.  This
		// allows for validating that the last flush time is correctly updated
		// to the current time when a flush occurs.
		mockedCurrentTime := utxoCache.lastFlushTime.Add(time.Minute)
		utxoCache.timeNow = func() time.Time {
			return mockedCurrentTime
		}

		// Add entries specified by the test to the test backend.
		err := backend.PutUtxos(test.backendEntries, &UtxoSetState{
			lastFlushHeight: utxoCache.lastEvictionHeight,
			lastFlushHash:   utxoCache.lastFlushHash,
		})
		if err != nil {
			t.Fatalf("%q: unexpected error adding entries to test backend: %v",
				test.name, err)
		}

		// Conditionally flush the cache based on the test parameters.
		err = utxoCache.MaybeFlush(test.bestHash, test.bestHeight,
			test.forceFlush, false)
		if err != nil {
			t.Fatalf("%q: unexpected error flushing cache: %v", test.name, err)
		}

		// Validate that the cached entries match the expected entries after
		// eviction.
		if !reflect.DeepEqual(utxoCache.entries, test.wantCachedEntries) {
			t.Fatalf("%q: mismatched cached entries:\nwant: %+v\n got: %+v\n",
				test.name, test.wantCachedEntries, utxoCache.entries)
		}

		// Validate that the backend entries match the expected entries after
		// flushing the cache.
		backendEntries := make(map[wire.OutPoint]*UtxoEntry)
		for outpoint := range test.cachedEntries {
			entry, err := backend.FetchEntry(outpoint)
			if err != nil {
				t.Fatalf("%q: unexpected error fetching entries from test "+
					"backend: %v", test.name, err)
			}

			if entry != nil {
				backendEntries[outpoint] = entry
			}
		}
		if err != nil {
			t.Fatalf("%q: unexpected error fetching entries from test "+
				"backend: %v", test.name, err)
		}
		if !reflect.DeepEqual(backendEntries, test.wantBackendEntries) {
			t.Fatalf("%q: mismatched backend entries:\nwant: %+v\n got: %+v\n",
				test.name, test.wantBackendEntries, backendEntries)
		}

		// Validate that the last flush hash and time have been updated as
		// expected.
		if utxoCache.lastFlushHash != *test.wantLastFlushHash {
			t.Fatalf("%q: unexpected last flush hash -- got %x, want %x",
				test.name, utxoCache.lastFlushHash, *test.wantLastFlushHash)
		}
		updatedLastFlushTime := utxoCache.lastFlushTime == mockedCurrentTime
		if updatedLastFlushTime != test.wantUpdatedLastFlushTime {
			t.Fatalf("%q: unexpected updated last flush time -- got %v, want "+
				" %v", test.name, updatedLastFlushTime,
				test.wantUpdatedLastFlushTime)
		}

		// Validate the updated last eviction height.
		if utxoCache.lastEvictionHeight != test.wantLastEvictionHeight {
			t.Fatalf("%q: unexpected last eviction height -- got %d, want %d",
				test.name, utxoCache.lastEvictionHeight,
				test.wantLastEvictionHeight)
		}

		// Validate the updated total entry size of the cache.
		wantTotalEntrySize := uint64(0)
		for _, entry := range test.wantCachedEntries {
			wantTotalEntrySize += entry.size()
		}
		if utxoCache.totalEntrySize != wantTotalEntrySize {
			t.Fatalf("%q: unexpected total entry size -- got %v, want %v",
				test.name, utxoCache.totalEntrySize, wantTotalEntrySize)
		}
	}
}

// TestInitialize validates that the cache recovers properly during
// initialization under a variety of conditions.
func TestInitialize(t *testing.T) {
	// Create a test harness initialized with the genesis block as the tip.
	params := chaincfg.RegNetParams()
	g := newChaingenHarness(t, params)

	// -------------------------------------------------------------------------
	// Create some convenience functions to improve test readability.
	// -------------------------------------------------------------------------

	// resetTestUtxoCache replaces the current utxo cache with a new test utxo
	// cache and calls initialize on it.  This simulates an empty utxo cache
	// that gets created and initialized at startup.
	backend := createTestUtxoBackend(t)
	err := backend.InitInfo(g.chain.dbInfo.version)
	if err != nil {
		t.Fatalf("error initializing backend info: %v", err)
	}
	resetTestUtxoCache := func() *testUtxoCache {
		testUtxoCache := newTestUtxoCache(&UtxoCacheConfig{
			Backend:      backend,
			FlushBlockDB: g.chain.db.Flush,
			MaxSize:      100 * 1024 * 1024, // 100 MiB
		})
		g.chain.utxoCache = testUtxoCache
		err := testUtxoCache.Initialize(context.Background(), g.chain,
			g.chain.bestChain.Tip())
		if err != nil {
			t.Fatalf("error initializing test cache: %v", err)
		}
		return testUtxoCache
	}

	// forceFlush forces a cache flush to the best chain tip.
	forceFlush := func(utxoCache *testUtxoCache) {
		tip := g.chain.bestChain.Tip()
		utxoCache.MaybeFlush(&tip.hash, uint32(tip.height), true, false)
	}

	// -------------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	//
	// Disable flushing of the cache while advancing the chain.  After reaching
	// stake validation height, reset the cache and validate that it recovers
	// and properly catches up to the tip.
	// -------------------------------------------------------------------------

	// Replace the utxo cache in the test chain with a test utxo cache so that
	// flushing can be toggled on and off for testing.
	testUtxoCache := resetTestUtxoCache()

	// Validate that the tip and utxo set state are currently at the genesis
	// block.
	g.ExpectTip("genesis")
	g.ExpectUtxoSetState("genesis")

	// Disable flushing and advance the chain.
	testUtxoCache.disableFlush = true
	g.AdvanceToStakeValidationHeight()

	// Validate that the tip is at stake validation height but the utxo set
	// state is still at the genesis block.
	g.AssertTipHeight(uint32(params.StakeValidationHeight))
	g.ExpectUtxoSetState("genesis")

	// Reset the cache and force a flush.
	testUtxoCache = resetTestUtxoCache()
	forceFlush(testUtxoCache)

	// Validate that the utxo cache is now caught up to the tip.
	g.ExpectUtxoSetState(g.TipName())

	// -------------------------------------------------------------------------
	// Create a few blocks to use as a base for the tests below.
	//
	//   ... -> b0 -> b1
	// -------------------------------------------------------------------------

	outs := g.OldestCoinbaseOuts()
	g.NextBlock("b0", &outs[0], outs[1:])
	g.AcceptTipBlock()

	outs = g.OldestCoinbaseOuts()
	b1 := g.NextBlock("b1", &outs[0], outs[1:])
	g.AcceptTipBlock()

	// Force a cache flush and validate that the cache is caught up to block b1.
	forceFlush(testUtxoCache)
	g.ExpectUtxoSetState("b1")

	// -------------------------------------------------------------------------
	// Simulate the following scenario:
	//   - The utxo cache was last flushed at the tip block
	//   - A reorg to a side chain is triggered
	//   - During the reorg, a failure resulting in an unclean shutdown
	//     occurs after disconnecting a block but before flushing the cache
	//     and removing the spend journal
	//   - The resulting state should be:
	//     - The cache was flushed at b1
	//     - The spend journal for b1 was not removed
	//     - The chain tip is at b1a
	//
	//        last cache flush here
	//                vvv
	//   ... -> b0 -> b1
	//            \-> b1a
	//                ^^^
	//              new tip
	// -------------------------------------------------------------------------

	// Disable flushing to simulate a failure resulting in the cache not being
	// flushed after disconnecting a block.
	testUtxoCache.disableFlush = true

	// Save the spend journal entry for b1.  The spend journal entry for block
	// b1 needs to be restored after the reorg to properly simulate the failure
	// scenario described above.
	var serialized []byte
	b1Hash := b1.BlockHash()
	g.chain.db.View(func(dbTx database.Tx) error {
		spendBucket := dbTx.Metadata().Bucket(spendJournalBucketName)
		serialized = spendBucket.Get(b1Hash[:])
		return nil
	})
	if serialized == nil {
		t.Fatalf("unable to fetch spend journal entry for block: %v", b1Hash)
	}

	// Force a reorg as described above.
	g.SetTip("b0")
	g.NextBlock("b1a", &outs[0], outs[1:])
	g.AcceptedToSideChainWithExpectedTip("b1")
	g.ForceTipReorg("b1", "b1a")

	// Restore the spend journal entry for block b1.
	err = g.chain.db.Update(func(dbTx database.Tx) error {
		spendBucket := dbTx.Metadata().Bucket(spendJournalBucketName)
		return spendBucket.Put(b1Hash[:], serialized)
	})
	if err != nil {
		t.Fatalf("unexpected error putting spend journal entry: %v", err)
	}

	// Validate that the tip is at b1a but the utxo cache flushed state is at
	// b1.
	g.ExpectTip("b1a")
	g.ExpectUtxoSetState("b1")

	// Reset the cache and force a flush.
	testUtxoCache = resetTestUtxoCache()
	forceFlush(testUtxoCache)

	// Validate that the cache recovered and is now caught up to b1a.
	g.ExpectUtxoSetState("b1a")
}

// TestShutdownUtxoCache validates that a cache flush is forced when shutting
// down.
func TestShutdownUtxoCache(t *testing.T) {
	// Create a test harness initialized with the genesis block as the tip.
	params := chaincfg.RegNetParams()
	g := newChaingenHarness(t, params)

	// Replace the chain utxo cache with a test cache so that flushing can be
	// disabled.
	backend := createTestUtxoBackend(t)
	testUtxoCache := newTestUtxoCache(&UtxoCacheConfig{
		Backend:      backend,
		FlushBlockDB: g.chain.db.Flush,
		MaxSize:      100 * 1024 * 1024, // 100 MiB
	})
	err := backend.InitInfo(g.chain.dbInfo.version)
	if err != nil {
		t.Fatalf("error initializing backend info: %v", err)
	}
	err = testUtxoCache.Initialize(context.Background(), g.chain,
		g.chain.bestChain.Tip())
	if err != nil {
		t.Fatalf("error initializing test cache: %v", err)
	}
	g.chain.utxoCache = testUtxoCache

	// -------------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	//
	// Disable flushing of the cache while advancing the chain.  After reaching
	// stake validation height, call shutdown and validate that it forces a
	// flush and properly catches up to the tip.
	// -------------------------------------------------------------------------

	// Validate that the tip and utxo set state are currently at the genesis
	// block.
	g.ExpectTip("genesis")
	g.ExpectUtxoSetState("genesis")

	// Disable flushing and advance the chain.
	testUtxoCache.disableFlush = true
	g.AdvanceToStakeValidationHeight()

	// Validate that the tip is at stake validation height but the utxo set
	// state is still at the genesis block.
	g.AssertTipHeight(uint32(params.StakeValidationHeight))
	g.ExpectUtxoSetState("genesis")

	// Enable flushing and shutdown the cache.
	testUtxoCache.disableFlush = false
	g.chain.ShutdownUtxoCache()

	// Validate that the utxo cache is now caught up to the tip.
	g.ExpectUtxoSetState(g.TipName())
}
