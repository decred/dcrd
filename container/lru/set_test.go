// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package lru

import (
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"
)

// lruElem returns the least recently used item without modifying any state.
func (c *Set[T]) lruElem() *element[T, struct{}] {
	defer c.m.mtx.Unlock()
	c.m.mtx.Lock()
	return c.m.root.prev
}

// testContextSet houses a test-related state that is useful to pass to helper
// functions as a single argument.
type testContextSet struct {
	t    *testing.T
	name string
	s    *Set[uint64]
}

// assertSetLen asserts the set associated with the provided test context
// returns the provided length from Len.
func assertSetLen(tc *testContextSet, wantLen uint32) {
	tc.t.Helper()

	if gotLen := tc.s.Len(); gotLen != wantLen {
		tc.t.Fatalf("%q: unexpected number of items in set: got %v, want %v",
			tc.name, gotLen, wantLen)
	}
}

// putSetAndAssertNumEvicted asserts that calling Put with the provided item on
// the set associated with the provided test context returns the given expected
// number of evicted items.
func putSetAndAssertNumEvicted(tc *testContextSet, item uint64, wantNumEvicted uint32) {
	tc.t.Helper()

	gotNumEvicted := tc.s.Put(item)
	if gotNumEvicted != wantNumEvicted {
		tc.t.Fatalf("%q: Put returned unexpected number of evicted items: "+
			"got %v, want %v", tc.name, gotNumEvicted, wantNumEvicted)
	}
}

// putSetTTLAndAssertNumEvicted asserts that calling PutWithTTL with the
// provided item and ttl on the set associated with the provided test context
// returns the given expected number of evicted items.
func putSetTTLAndAssertNumEvicted(tc *testContextSet, item uint64, ttl time.Duration, wantNumEvicted uint32) {
	tc.t.Helper()

	gotNumEvicted := tc.s.PutWithTTL(item, ttl)
	if gotNumEvicted != wantNumEvicted {
		tc.t.Fatalf("%q: PutWithTTL returned unexpected number of evicted "+
			"items: got %v, want %v", tc.name, gotNumEvicted, wantNumEvicted)
	}
}

// assertSetItems asserts the set associated with the provided test context
// returns the provided items from Items.
func assertSetItems(tc *testContextSet, wantItems []uint64) {
	tc.t.Helper()

	gotItems := tc.s.Items()
	if !reflect.DeepEqual(gotItems, wantItems) {
		tc.t.Fatalf("%q: unexpected items: got %v, want %v", tc.name, gotItems,
			wantItems)
	}
}

// assertSetMembership asserts the set associated with the provided test context
// reports the given expected existence for Exists and Contains.
func assertSetMembership(tc *testContextSet, item uint64, shouldExist bool) {
	tc.t.Helper()

	ok := tc.s.Exists(item)
	if ok != shouldExist {
		tc.t.Fatalf("%q: Exists item %d: got %v, want %v", tc.name, item, ok,
			shouldExist)
	}

	ok = tc.s.Contains(item)
	if ok != shouldExist {
		tc.t.Fatalf("%q: Contains item %d: got %v, want %v", tc.name, item, ok,
			shouldExist)
	}
}

// assertInternSetMembership asserts the set associated with the provided test
// context has an internal state that matches the given expected existence.
func assertInternSetMembership(tc *testContextSet, item uint64, shouldExist bool) {
	tc.t.Helper()

	tc.s.m.mtx.Lock()
	_, exists := tc.s.m.items[item]
	tc.s.m.mtx.Unlock()

	if exists != shouldExist {
		tc.t.Fatalf("%q: unexpected physical existence for item %d: got %v, "+
			"want %v", tc.name, item, exists, shouldExist)
	}
}

// evictSetExpiredAndAssertNumEvicted asserts that calling EvictExpiredNow on
// the set associated with the provided test context returns the given expected
// number of evicted items.
func evictSetExpiredAndAssertNumEvicted(tc *testContextSet, wantNumEvicted uint32, expirationDisabled bool) {
	tc.t.Helper()

	gotNumEvicted := tc.s.EvictExpiredNow()
	if expirationDisabled {
		wantNumEvicted = 0
	}
	if gotNumEvicted != wantNumEvicted {
		tc.t.Fatalf("%q: EvictExpiredNow returned unexpected number of "+
			"evicted items: got %v, want %v", tc.name, gotNumEvicted,
			wantNumEvicted)
	}
}

// assertSetHitRatio asserts the set associated with the provided test context
// returns the expected hit ratio.
func assertSetHitRatio(tc *testContextSet, wantRatio float64) {
	tc.t.Helper()

	gotRatio := tc.s.HitRatio()
	roundedRatio := math.Round(gotRatio*100) / 100
	if roundedRatio != wantRatio {
		tc.t.Fatalf("%q: unexpected hit ratio: got %v, want %v", tc.name,
			roundedRatio, wantRatio)
	}
}

// TestSet ensures the set behaves as expected including limiting, eviction of
// least recently used entries, specific entry removal, existence, containment,
// and length tests.
func TestSet(t *testing.T) {
	t.Parallel()

	// Create some items to use in testing the set code.
	const numItems = 10
	items, _ := makeKeysAndVals(numItems)

	tests := []struct {
		name  string // test description
		limit uint32 // max number of items in the set
	}{
		{name: "limit 0", limit: 0},
		{name: "limit 1", limit: 1},
		{name: "limit 5", limit: 5},
		{name: "limit 7", limit: 7},
		{name: "limit one less than available", limit: numItems - 1},
		{name: "limit all available", limit: numItems},
	}

	for _, test := range tests {
		// Create a new set limited by the specified test limit and add all of
		// the test vectors.  This will cause eviction since there are more test
		// items than the limits.
		set := NewSet[uint64](test.limit)
		tc := &testContextSet{t: t, name: test.name, s: set}
		for i := uint32(0); i < numItems; i++ {
			var wantNumEvicted uint32
			if i >= test.limit && test.limit > 0 {
				wantNumEvicted = 1
			}
			putSetAndAssertNumEvicted(tc, items[i], wantNumEvicted)
		}

		// Ensure the expected number of items and items themselves are returned
		// in the expected order.
		assertSetLen(tc, test.limit)
		var wantItems []uint64
		if test.limit > 0 {
			wantItems = items[numItems-test.limit : numItems]
		}
		assertSetItems(tc, wantItems)

		// Ensure the limited number of most recent entries exist.
		for i := numItems - test.limit; i < numItems; i++ {
			assertSetMembership(tc, items[i], true)
		}

		// Ensure the entries before the limited number of most recent entries
		// do not exist.
		for i := uint32(0); i < numItems-test.limit; i++ {
			assertSetMembership(tc, items[i], false)
		}

		// Ensure accessing the least recently used entry with Contains makes it
		// the most recently used entry and therefore is not evicted when the
		// limit is exceeded.
		//
		// This check needs at least 2 entries.
		if test.limit > 1 {
			// Ensure the least recently used entry is the expected one.
			origLruIdx := numItems - test.limit
			gotLruKey := set.lruElem().key
			if gotLruKey != items[origLruIdx] {
				t.Fatalf("%q: unexpected lru item: got %v, want %v", test.name,
					gotLruKey, items[origLruIdx])
			}

			// Access the entry so it becomes the most recently used entry.
			_ = set.Contains(items[origLruIdx])

			// Force an eviction by putting an entry that doesn't exist.
			newItem := uint64(numItems) + 1
			putSetAndAssertNumEvicted(tc, newItem, 1)

			// Ensure the original lru entry still exists since it was updated
			// and should have become the mru entry.
			if !set.Exists(items[origLruIdx]) {
				t.Fatalf("%q: entry %d does not exist", test.name,
					items[origLruIdx])
			}

			// Ensure the entry that should've become the new lru entry was
			// evicted.
			shouldEvictItem := items[origLruIdx+1]
			if set.Exists(shouldEvictItem) {
				t.Fatalf("%q: entry %d exists", test.name, shouldEvictItem)
			}
		}

		// Ensure updating the least recently used entry makes it the most
		// recently used entry and therefore is not evicted when the limit is
		// exceeded.
		//
		// This check needs at least 2 entries.
		if test.limit > 1 {
			// Ensure the least recently used entry is the expected one.
			origLruIdx := numItems - test.limit + 2
			gotLruKey := set.lruElem().key
			if gotLruKey != items[origLruIdx] {
				t.Fatalf("%q: unexpected lru item: got %v, want %v", test.name,
					gotLruKey, items[origLruIdx])
			}

			// Put the entry again so it becomes the most recently used entry.
			putSetAndAssertNumEvicted(tc, items[origLruIdx], 0)

			// Force an eviction by putting an entry that doesn't exist.
			newItem := uint32(numItems) + 2
			putSetAndAssertNumEvicted(tc, uint64(newItem), 1)

			// Ensure the original lru entry still exists since it was updated
			// and should've have become the mru entry.
			if !set.Exists(items[origLruIdx]) {
				t.Fatalf("%q: entry %d does not exist", test.name,
					items[origLruIdx])
			}

			// Ensure the entry that should've become the new lru entry was
			// evicted.
			shouldEvictItem := items[origLruIdx+1]
			if set.Exists(shouldEvictItem) {
				t.Fatalf("%q: entry %d exists", test.name, shouldEvictItem)
			}
		}

		// Delete all of the entries in the list, including those that don't
		// exist in the set, and ensure they no longer exist.
		for i := 0; i < numItems; i++ {
			set.Delete(items[i])
			if set.Exists(items[i]) {
				t.Fatalf("%q: deleted entry %d exists", test.name, items[i])
			}
		}

		// Ensure a limit of zero is respected by the put variant that accepts
		// a TTL as well.
		if test.limit == 0 {
			const ttl0 = 0
			for i := uint32(0); i < numItems; i++ {
				putSetTTLAndAssertNumEvicted(tc, items[i], ttl0, 0)
			}
			assertSetLen(tc, 0)
		}
	}
}

// TestDefaultSetExpiration ensures the set expiration with a default expiration
// behaves as expected including lazy expiration, explicit expiration, and the
// interaction between expiration and limits.
//
// It also doubles to ensure clearing the set behaves as expected.
func TestDefaultSetExpiration(t *testing.T) {
	t.Parallel()

	// Create some items to use in testing the set code.  A couple of extra
	// values are created to ensure there are unique values to exceed the limit
	// with.
	const numItems = 10
	items, _ := makeKeysAndVals(numItems + 2)

	// Create a mock time to control expiration testing.
	baseNow := time.Now()
	mockNow := baseNow
	mockNowFn := func() time.Time { return mockNow }

	// repopulateSet resets the set to the following state:
	//
	// - Half the entries with expiration at baseNow + ttl
	// - The remaining entries with expiration at baseNow + ttl + 1
	// - The next expire scan interval at baseNow + expireScanInterval
	//
	// This ensures each half has different expiration times.
	repopulateSet := func(tc *testContextSet) {
		mockNow = baseNow
		tc.s.Clear()
		for i := 0; i < numItems/2; i++ {
			putSetAndAssertNumEvicted(tc, items[i], 0)
		}
		mockNow = baseNow.Add(time.Second)
		for i := numItems / 2; i < numItems; i++ {
			putSetAndAssertNumEvicted(tc, items[i], 0)
		}
	}

	tests := []struct {
		name string        // test description
		ttl  time.Duration // default TTL for all items
	}{
		{name: "expiration disabled", ttl: 0},
		{name: "half expire scan interval", ttl: expireScanInterval / 2},
		{name: "expire scan interval - 1", ttl: expireScanInterval - time.Second},
	}

	for _, test := range tests {
		// Create a new set with a TTL specified by the test that is big enough
		// to hold all test values.  Also override the now function so the
		// current time can be manipulated for testing expiration semantics.
		set := NewSetWithDefaultTTL[uint64](numItems+1, test.ttl)
		set.m.nowFn = mockNowFn
		set.m.nextExpireScan = baseNow.Add(expireScanInterval)
		tc := &testContextSet{t: t, name: test.name, s: set}

		// Ensure expired items are not physically removed until an expiration
		// scan is triggered, but the public methods treat them as if they were
		// already removed.  Also, ensure that an expiration scan is not
		// triggered prior to the expiration scan interval passing.  Finally,
		// ensure no expiration takes place under the same scenario when
		// expiration is disabled.
		//
		// Methodology:
		// - Start with the set such that second half of the values expire one
		//   second after the first half
		// - Set the mock time such that only the first half of the items have
		//   expired but is still prior to the next expiration scan
		// - Put a new value into the set and remove it to ensure expiration
		//   is not triggered
		// - Confirm the public methods treat the items as expired (unless
		//   expiration is disabled)
		// - Confirm the items are physically still in the set
		expirationDisabled := test.ttl == 0
		repopulateSet(tc)
		mockNow = baseNow.Add(test.ttl + time.Second)
		putSetAndAssertNumEvicted(tc, items[numItems], 0)
		set.Delete(items[numItems])
		for i := 0; i < numItems/2; i++ {
			assertSetMembership(tc, items[i], expirationDisabled)
			assertInternSetMembership(tc, items[i], true)
		}
		for i := numItems / 2; i < numItems; i++ {
			assertSetMembership(tc, items[i], true)
		}

		// Ensure the expected items are returned in the expected order.
		wantItems := items[numItems/2 : numItems]
		if expirationDisabled {
			wantItems = items[0:numItems]
		}
		assertSetItems(tc, wantItems)

		// Ensure all expired items removed after the expiration scan interval
		// when explicitly requested when expiration is enabled.
		//
		// Conversely, ensure items that would be expired are not removed when
		// expiration is disabled even when explicitly requested.
		repopulateSet(tc)
		mockNow = baseNow.Add(expireScanInterval + time.Second)
		evictSetExpiredAndAssertNumEvicted(tc, numItems, expirationDisabled)
		for i := 0; i < numItems; i++ {
			assertSetMembership(tc, items[i], expirationDisabled)
			assertInternSetMembership(tc, items[i], expirationDisabled)
		}

		// Ensure expired items are removed prior to the expiration scan
		// interval when explicitly requested and expiration is enabled.
		//
		// Also, ensure no expiration takes place under the same scenario when
		// expiration is disabled.
		//
		// Methodology:
		// - Start with the set such that second half of the values expire one
		//   second after the first half
		// - Set the mock time such that only the first half of the items have
		//   expired but is still prior to the next expiration scan
		// - Run explicit expiration
		// - Confirm the first half is expired (unless disabled) and the second
		//   half is NOT
		// - Set the mock time such that the second half of the items have
		//   expired but is still prior to the next expiration scan
		// - Run explicit expiration
		// - Confirm all items are expired (unless disabled)
		repopulateSet(tc)
		mockNow = baseNow.Add(test.ttl + time.Second)
		set.EvictExpiredNow()
		for i := 0; i < numItems/2; i++ {
			assertSetMembership(tc, items[i], expirationDisabled)
			assertInternSetMembership(tc, items[i], expirationDisabled)
		}
		for i := numItems / 2; i < numItems; i++ {
			assertSetMembership(tc, items[i], true)
		}
		mockNow = baseNow.Add(test.ttl + 2*time.Second)
		set.EvictExpiredNow()
		for i := 0; i < numItems; i++ {
			assertSetMembership(tc, items[i], expirationDisabled)
			assertInternSetMembership(tc, items[i], expirationDisabled)
		}

		// Ensure putting an item with the same key as an expired item updates
		// that item so it is no longer expired.
		//
		// Also, ensure no expiration takes place under the same scenario when
		// expiration is disabled.
		//
		// Methodology:
		// - Start with the set such that second half of the values expire one
		//   second after the first half
		// - Set the mock time such that only the first half of the items have
		//   expired but is still prior to the next expiration scan
		// - Put the first and last items in the set again so their expiration
		//   times are updated
		// - Run explicit expiration
		// - Confirm the first item and the second half are NOT expired and
		//   the remaining items in the first half ARE expired (unless disabled)
		// - Set the mock time such that the second half of the items have
		//   expired but is still prior to the next expiration scan
		// - Run explicit expiration
		// - Confirm the first and last items are NOT expired and the remaining
		//   items ARE expired (unless disabled)
		repopulateSet(tc)
		mockNow = baseNow.Add(test.ttl + time.Second)
		putSetAndAssertNumEvicted(tc, items[0], 0)
		putSetAndAssertNumEvicted(tc, items[numItems-1], 0)
		evictSetExpiredAndAssertNumEvicted(tc, numItems/2-1, expirationDisabled)
		assertSetMembership(tc, items[0], true)
		for i := 1; i < numItems/2; i++ {
			assertSetMembership(tc, items[i], expirationDisabled)
			assertInternSetMembership(tc, items[i], expirationDisabled)
		}
		for i := numItems / 2; i < numItems; i++ {
			assertSetMembership(tc, items[i], true)
		}
		mockNow = baseNow.Add(test.ttl + 2*time.Second)
		evictSetExpiredAndAssertNumEvicted(tc, numItems/2-1, expirationDisabled)
		assertSetMembership(tc, items[0], true)
		for i := 1; i < numItems/2; i++ {
			assertSetMembership(tc, items[i], expirationDisabled)
			assertInternSetMembership(tc, items[i], expirationDisabled)
		}
		for i := numItems / 2; i < numItems-1; i++ {
			assertSetMembership(tc, items[i], expirationDisabled)
			assertInternSetMembership(tc, items[i], expirationDisabled)
		}
		assertSetMembership(tc, items[numItems-1], true)

		// Ensure putting an item with a 0 TTL overrides the default TTL and
		// disables expiration for that item.
		//
		// Also, ensure no expiration takes place under the same scenario when
		// expiration is disabled.
		//
		// Methodology:
		// - Start with the set such that second half of the values expire one
		//   second after the first half
		// - Put the last item again with a TTL of 0 to disable its expiration
		// - Set the mock time such that all items would be expired and the
		//   expire scan interval has elapsed
		// - Run explicit expiration
		// - Confirm all of the items except that last one ARE expired (unless
		//   disabled) and the last is NOT expired.
		const ttl0 = 0
		repopulateSet(tc)
		putSetTTLAndAssertNumEvicted(tc, items[numItems-1], ttl0, 0)
		mockNow = baseNow.Add(expireScanInterval + 3*time.Second)
		evictSetExpiredAndAssertNumEvicted(tc, numItems-1, expirationDisabled)
		for i := 0; i < numItems-2; i++ {
			assertSetMembership(tc, items[i], expirationDisabled)
			assertInternSetMembership(tc, items[i], expirationDisabled)
		}
		assertSetMembership(tc, items[numItems-1], true)

		// Ensure putting a new item that would ordinarily cause an eviction of
		// the least recently used item due to exceeding the limit takes the
		// place of expired items instead.
		//
		// Methodology:
		// - Start with the set such that the last entry expires prior to the
		//   next expire scan interval and the remaining items all expire after
		//   it
		//   - Since the final item is added last, it will be the **mru** item
		//     and thus ordinarily would not be evicted when the limit is
		//     exceeded
		// - Set the mock time such that both the last item is expired and the
		//   expire scan interval has elapsed, but the remaining items are NOT
		//   expired
		// - Put a new item in the set that would exceed the max limit
		// - Confirm only the expired item was removed and the new item took its
		//   place
		if !expirationDisabled {
			mockNow = baseNow
			set.Clear()
			mockNow = baseNow.Add(expireScanInterval - time.Second)
			for i := 0; i < numItems; i++ {
				putSetAndAssertNumEvicted(tc, items[i], 0)
			}
			mockNow = baseNow
			putSetAndAssertNumEvicted(tc, items[numItems], 0)
			mockNow = baseNow.Add(expireScanInterval + time.Second)
			putSetAndAssertNumEvicted(tc, items[numItems+1], 1)
			assertSetMembership(tc, items[numItems], false)
			for i := 0; i < numItems; i++ {
				assertSetMembership(tc, items[i], true)
			}
			assertSetMembership(tc, items[numItems+1], true)
		}
	}
}

// TestSetHitRatio ensures the hit ratio reporting for sets behaves as expected
// with and without expiration disabled.
func TestSetHitRatio(t *testing.T) {
	t.Parallel()

	// Create some items to use in testing the set code.
	const totalItems = 50
	items, _ := makeKeysAndVals(totalItems)

	// Create a mock time to control expiration testing.
	baseNow := time.Now()
	mockNow := baseNow
	mockNowFn := func() time.Time { return mockNow }

	tests := []struct {
		name       string        // test description
		ttl        time.Duration // default TTL for all items
		numItems   uint32        // number of items to populate
		numExpired uint32        // number of items to expire
		itemIdxs   []uint64      // indices of items to get
		expected   []float64     // expected hit ratio after each get
	}{{
		name:       "hit ratio 0% due to all expired",
		numItems:   5,
		numExpired: 5,
		ttl:        expireScanInterval / 2,
		itemIdxs:   []uint64{0, 1, 2, 3, 4},
		expected:   []float64{0, 0, 0, 0, 0},
	}, {
		name:     "hit ratio 0% single item, expiration disabled",
		numItems: 10,
		ttl:      0,
		itemIdxs: []uint64{10},
		expected: []float64{0},
	}, {
		name:       "hit ratio 10% descending, expiration disabled, 3 would be",
		numItems:   10,
		numExpired: 3,
		ttl:        0,
		itemIdxs:   []uint64{0, 10, 11, 12, 13, 14, 15, 16, 17, 18},
		expected:   []float64{100, 50, 33.33, 25, 20, 16.67, 14.29, 12.5, 11.11, 10},
	}, {
		name:       "hit ratio 50% alternating due to expired items",
		numItems:   10,
		numExpired: 5,
		ttl:        expireScanInterval / 2,
		itemIdxs:   []uint64{5, 0, 6, 1, 7, 2, 8, 3, 9, 4},
		expected:   []float64{100, 50, 66.67, 50, 60, 50, 57.14, 50, 55.56, 50},
	}, {
		name:     "hit ratio 50% alternating, expiration disabled",
		numItems: 10,
		ttl:      0,
		itemIdxs: []uint64{0, 10, 1, 11, 2, 12, 3, 13, 4, 14},
		expected: []float64{100, 50, 66.67, 50, 60, 50, 57.14, 50, 55.56, 50},
	}, {
		name:       "hit ratio would be 100%, but 1 item expired",
		ttl:        expireScanInterval / 2,
		numItems:   5,
		numExpired: 1,
		itemIdxs:   []uint64{0, 1, 2, 3, 4},
		expected:   []float64{0, 50, 66.67, 75, 80},
	}, {
		name:       "hit ratio 100%, expiration disabled, 5 would be expired",
		ttl:        0,
		numItems:   10,
		numExpired: 5,
		itemIdxs:   []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		expected:   []float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
	}}

	for _, test := range tests {
		if len(test.itemIdxs) != len(test.expected) {
			t.Fatalf("%q: test data item idx len mismatch: %d != %d", test.name,
				len(test.itemIdxs), len(test.expected))
		}
		if test.numExpired > test.numItems {
			t.Fatalf("%q: test data num expired exceeds num items: %d > %d",
				test.name, test.numExpired, test.numItems)
		}

		// Create a new set with a TTL specified by the test that is big enough
		// to hold all test values.  Also override the now function so the
		// current time can be manipulated for forcing expirations.
		mockNow = baseNow
		set := NewSetWithDefaultTTL[uint64](totalItems+1, test.ttl)
		set.m.nowFn = mockNowFn
		set.m.nextExpireScan = baseNow.Add(expireScanInterval)
		tc := &testContextSet{t: t, name: test.name, s: set}

		// Ensure the hit ratio starts at 100% and populate the set such that
		// number of items specified by the test are expired.
		assertSetHitRatio(tc, 100)
		for i := uint32(0); i < test.numExpired; i++ {
			set.Put(items[i])
		}
		mockNow = baseNow.Add(2 * time.Second)
		for i := test.numExpired; i < test.numItems; i++ {
			set.Put(items[i])
		}
		mockNow = baseNow.Add(test.ttl + time.Second)

		// Ensure existence checking does not modify the hit ratio.
		for _, itemIdx := range test.itemIdxs {
			tc.name = fmt.Sprintf("%s-itemidx %d", test.name, itemIdx)
			set.Exists(items[itemIdx])
			assertSetHitRatio(tc, 100)
		}

		// Attempt to access each item specified by the test data and ensure the
		// hit ratio is as expected.
		for sliceIdx, itemIdx := range test.itemIdxs {
			tc.name = fmt.Sprintf("%s-itemidx %d", test.name, itemIdx)
			set.Contains(items[itemIdx])
			assertSetHitRatio(tc, test.expected[sliceIdx])
		}

		// Ensure clearing the set returns the hit ratio to 100%.
		tc.name = test.name
		set.Clear()
		assertSetHitRatio(tc, 100)
	}
}
