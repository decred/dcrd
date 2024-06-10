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
func (m *Map[K, V]) lruElem() *element[K, V] {
	defer m.mtx.Unlock()
	m.mtx.Lock()
	return m.root.prev
}

// makeKeysAndVals returns slices containing the requested number of keys and
// values to use in testing the map code.
func makeKeysAndVals(numItems uint32) ([]uint64, []uint32) {
	keys := make([]uint64, 0, numItems)
	vals := make([]uint32, 0, numItems)
	for i := uint32(0); i < numItems; i++ {
		keys = append(keys, uint64(i))
		vals = append(vals, i)
	}

	return keys, vals
}

// testContextMap houses a test-related state that is useful to pass to helper
// functions as a single argument.
type testContextMap struct {
	t    *testing.T
	name string
	m    *Map[uint64, uint32]
}

// assertMapLen asserts the map associated with the provided test context
// returns the provided length from Len.
func assertMapLen(tc *testContextMap, wantLen uint32) {
	tc.t.Helper()

	if gotLen := tc.m.Len(); gotLen != wantLen {
		tc.t.Fatalf("%q: unexpected number of items in map: got %v, want %v",
			tc.name, gotLen, wantLen)
	}
}

// assertMapKeysAndVals asserts the map associated with the provided test
// context returns the provided keys and values from Keys and Vals,
// respectively.
func assertMapKeysAndVals(tc *testContextMap, wantKeys []uint64, wantVals []uint32) {
	tc.t.Helper()

	gotKeys := tc.m.Keys()
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		tc.t.Fatalf("%q: unexpected keys: got %v, want %v", tc.name, gotKeys,
			wantKeys)
	}

	gotVals := tc.m.Values()
	if !reflect.DeepEqual(gotVals, wantVals) {
		tc.t.Fatalf("%q: unexpected values: got %v, want %v", tc.name, gotVals,
			wantVals)
	}
}

// putMapAndAssertNumEvicted asserts that calling Put with the provided key and
// value on the map associated with the provided test context returns the given
// expected number of evicted items.
func putMapAndAssertNumEvicted(tc *testContextMap, key uint64, val uint32, wantNumEvicted uint32) {
	tc.t.Helper()

	gotNumEvicted := tc.m.Put(key, val)
	if gotNumEvicted != wantNumEvicted {
		tc.t.Fatalf("%q: Put returned unexpected number of evicted items: "+
			"got %v, want %v", tc.name, gotNumEvicted, wantNumEvicted)
	}
}

// putMapTTLAndAssertNumEvicted asserts that calling PutWithTTL with the
// provided key, value, and ttl on the map associated with the provided test
// context returns the given expected number of evicted items.
func putMapTTLAndAssertNumEvicted(tc *testContextMap, key uint64, val uint32, ttl time.Duration, wantNumEvicted uint32) {
	tc.t.Helper()

	gotNumEvicted := tc.m.PutWithTTL(key, val, ttl)
	if gotNumEvicted != wantNumEvicted {
		tc.t.Fatalf("%q: PutWithTTL returned unexpected number of evicted "+
			"items: got %v, want %v", tc.name, gotNumEvicted, wantNumEvicted)
	}
}

// assertMapMembership asserts the map associated with the provided test context
// reports the given expected value and existence for Exists, Peek, and Get.
func assertMapMembership(tc *testContextMap, key uint64, wantVal uint32, shouldExist bool) {
	tc.t.Helper()

	ok := tc.m.Exists(key)
	if ok != shouldExist {
		tc.t.Fatalf("%q: Exists key %d: got %v, want %v", tc.name, key, ok,
			shouldExist)
	}

	gotVal, ok := tc.m.Peek(key)
	if ok != shouldExist {
		tc.t.Fatalf("%q: Peek key %d: got %v, want %v", tc.name, key, ok,
			shouldExist)
	}
	if ok && gotVal != wantVal {
		tc.t.Fatalf("%q: mismatched Peek value for key %d: got %v, want %v",
			tc.name, key, gotVal, wantVal)
	}

	gotVal, ok = tc.m.Get(key)
	if ok != shouldExist {
		tc.t.Fatalf("%q: Get key %d: got %v, want %v", tc.name, key, ok,
			shouldExist)
	}
	if ok && gotVal != wantVal {
		tc.t.Fatalf("%q: mismatched Get value for key %d: got %v, want %v",
			tc.name, key, gotVal, wantVal)
	}
}

// assertInternMapMembership asserts the map associated with the provided test
// context has an internal state that matches the given expected value and
// existence.
func assertInternMapMembership(tc *testContextMap, key uint64, wantVal uint32, shouldExist bool) {
	tc.t.Helper()

	tc.m.mtx.Lock()
	elem, exists := tc.m.items[key]
	tc.m.mtx.Unlock()

	if exists != shouldExist {
		tc.t.Fatalf("%q: unexpected physical existence for key %d: got %v, "+
			"want %v", tc.name, key, exists, shouldExist)
	}
	if exists && elem.value != wantVal {
		tc.t.Fatalf("%q: mismatched value for key %d: got %v, want %v",
			tc.name, key, elem.value, wantVal)
	}
}

// evictMapExpiredAndAssertNumEvicted asserts that calling EvictExpiredNow on
// the map associated with the provided test context returns the given expected
// number of evicted items.
func evictMapExpiredAndAssertNumEvicted(tc *testContextMap, wantNumEvicted uint32, expirationDisabled bool) {
	tc.t.Helper()

	gotNumEvicted := tc.m.EvictExpiredNow()
	if expirationDisabled {
		wantNumEvicted = 0
	}
	if gotNumEvicted != wantNumEvicted {
		tc.t.Fatalf("%q: EvictExpiredNow returned unexpected number of "+
			"evicted items: got %v, want %v", tc.name, gotNumEvicted,
			wantNumEvicted)
	}
}

// assertMapHitRatio asserts the map associated with the provided test context
// returns the expected hit ratio.
func assertMapHitRatio(tc *testContextMap, wantRatio float64) {
	tc.t.Helper()

	gotRatio := tc.m.HitRatio()
	roundedRatio := math.Round(gotRatio*100) / 100
	if roundedRatio != wantRatio {
		tc.t.Fatalf("%q: unexpected hit ratio: got %v, want %v", tc.name,
			roundedRatio, wantRatio)
	}
}

// TestMap ensures the map behaves as expected including limiting, eviction of
// least recently used entries, specific entry removal, peeking, existence and
// length tests.
func TestMap(t *testing.T) {
	t.Parallel()

	// Create some keys and values to use in testing the map code.
	const numItems = 10
	keys, vals := makeKeysAndVals(numItems)

	tests := []struct {
		name  string // test description
		limit uint32 // max number of items in the map
	}{
		{name: "limit 0", limit: 0},
		{name: "limit 1", limit: 1},
		{name: "limit 5", limit: 5},
		{name: "limit 7", limit: 7},
		{name: "limit one less than available", limit: numItems - 1},
		{name: "limit all available", limit: numItems},
	}

	for _, test := range tests {
		// Create a new map limited by the specified test limit and add all of
		// the test vectors.  This will cause eviction since there are more test
		// items than the limits.
		m := NewMap[uint64, uint32](test.limit)
		tc := &testContextMap{t: t, name: test.name, m: m}
		for i := uint32(0); i < numItems; i++ {
			var wantNumEvicted uint32
			if i >= test.limit && test.limit > 0 {
				wantNumEvicted = 1
			}
			putMapAndAssertNumEvicted(tc, keys[i], vals[i], wantNumEvicted)
		}

		// Ensure the expected number of items, keys, and vals are returned in
		// the expected order.
		assertMapLen(tc, test.limit)
		var wantKeys []uint64
		var wantVals []uint32
		if test.limit > 0 {
			wantKeys = keys[numItems-test.limit : numItems]
			wantVals = vals[numItems-test.limit : numItems]
		}
		assertMapKeysAndVals(tc, wantKeys, wantVals)

		// Ensure the limited number of most recent entries exist.
		for i := numItems - test.limit; i < numItems; i++ {
			assertMapMembership(tc, keys[i], vals[i], true)
		}

		// Ensure the entries before the limited number of most recent entries
		// do not exist.
		for i := uint32(0); i < numItems-test.limit; i++ {
			assertMapMembership(tc, keys[i], vals[i], false)
		}

		// Ensure that peeking at an entry does not modify its priority.
		//
		// This check needs at least 1 entry.
		if test.limit > 0 {
			origLruElemKey := m.lruElem().key
			_, _ = m.Peek(origLruElemKey)
			newLruElemKey := m.lruElem().key
			if origLruElemKey != newLruElemKey {
				t.Fatalf("%q: Peek modified priority of key %d", test.name,
					origLruElemKey)
			}
		}

		// Ensure accessing the least recently used entry makes it the most
		// recently used entry and therefore is not evicted when the limit is
		// exceeded.
		//
		// This check needs at least 2 entries.
		if test.limit > 1 {
			// Ensure the least recently used entry is the expected one.
			origLruIdx := numItems - test.limit
			gotLruKey := m.lruElem().key
			if gotLruKey != keys[origLruIdx] {
				t.Fatalf("%q: unexpected lru item: got %v, want %v", test.name,
					gotLruKey, keys[origLruIdx])
			}

			// Access the entry so it becomes the most recently used entry.
			_, _ = m.Get(keys[origLruIdx])

			// Force an eviction by putting an entry that doesn't exist.
			newKey := uint64(numItems) + 1
			putMapAndAssertNumEvicted(tc, newKey, uint32(newKey), 1)

			// Ensure the original lru entry still exists since it was updated
			// and should have become the mru entry.
			if !m.Exists(keys[origLruIdx]) {
				t.Fatalf("%q: entry %d does not exist", test.name,
					keys[origLruIdx])
			}

			// Ensure the entry that should've become the new lru entry was
			// evicted.
			shouldEvictKey := keys[origLruIdx+1]
			if m.Exists(shouldEvictKey) {
				t.Fatalf("%q: entry %d exists", test.name, shouldEvictKey)
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
			gotLruKey := m.lruElem().key
			if gotLruKey != keys[origLruIdx] {
				t.Fatalf("%q: unexpected lru item: got %v, want %v", test.name,
					gotLruKey, keys[origLruIdx])
			}

			// Put the entry again so it becomes the most recently used entry.
			putMapAndAssertNumEvicted(tc, keys[origLruIdx], vals[origLruIdx], 0)

			// Force an eviction by putting an entry that doesn't exist.
			newVal := uint32(numItems) + 2
			putMapAndAssertNumEvicted(tc, uint64(newVal), newVal, 1)

			// Ensure the original lru entry still exists since it was updated
			// and should've have become the mru entry.
			if !m.Exists(keys[origLruIdx]) {
				t.Fatalf("%q: entry %d does not exist", test.name,
					keys[origLruIdx])
			}

			// Ensure the entry that should've become the new lru entry was
			// evicted.
			shouldEvictKey := keys[origLruIdx+1]
			if m.Exists(shouldEvictKey) {
				t.Fatalf("%q: entry %d exists", test.name, shouldEvictKey)
			}
		}

		// Delete all of the entries in the list, including those that don't
		// exist in the map, and ensure they no longer exist.
		for i := 0; i < numItems; i++ {
			m.Delete(keys[i])
			if m.Exists(keys[i]) {
				t.Fatalf("%q: deleted entry %d exists", test.name, keys[i])
			}
		}

		// Ensure a limit of zero is respected by the put variant that accepts
		// a TTL as well.
		if test.limit == 0 {
			const ttl0 = 0
			for i := uint32(0); i < numItems; i++ {
				putMapTTLAndAssertNumEvicted(tc, keys[i], vals[i], ttl0, 0)
			}
			assertMapLen(tc, 0)
		}
	}
}

// TestMapDefaultExpiration ensures the map expiration with a default expiration
// behaves as expected including lazy expiration, explicit expiration, and the
// interaction between expiration and map limits.
//
// It also doubles to ensure clearing the map behaves as expected.
func TestMapDefaultExpiration(t *testing.T) {
	t.Parallel()

	// Create some keys and values to use in testing the map code.  A couple of
	// extra values are created to ensure there are unique values to exceed the
	// map limit with.
	const numItems = 10
	keys, vals := makeKeysAndVals(numItems + 2)

	// Create a mock time to control expiration testing.
	baseNow := time.Now()
	mockNow := baseNow
	mockNowFn := func() time.Time { return mockNow }

	// repopulateMap resets the map to the following state:
	//
	// - Half the entries with expiration at baseNow + ttl
	// - The remaining entries with expiration at baseNow + ttl + 1
	// - The next expire scan interval at baseNow + expireScanInterval
	//
	// This ensures each half has different expiration times.
	repopulateMap := func(tc *testContextMap) {
		mockNow = baseNow
		tc.m.Clear()
		for i := 0; i < numItems/2; i++ {
			putMapAndAssertNumEvicted(tc, keys[i], vals[i], 0)
		}
		mockNow = baseNow.Add(time.Second)
		for i := numItems / 2; i < numItems; i++ {
			putMapAndAssertNumEvicted(tc, keys[i], vals[i], 0)
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
		// Create a new map with a TTL specified by the test that is big enough
		// to hold all test values.  Also override the now function so the
		// current time can be manipulated for testing expiration semantics.
		m := NewMapWithDefaultTTL[uint64, uint32](numItems+1, test.ttl)
		m.nowFn = mockNowFn
		m.nextExpireScan = baseNow.Add(expireScanInterval)
		tc := &testContextMap{t: t, name: test.name, m: m}

		// Ensure expired items are not physically removed until an expiration
		// scan is triggered, but the public methods treat them as if they were
		// already removed.  Also, ensure that an expiration scan is not
		// triggered prior to the expiration scan interval passing.  Finally,
		// ensure no expiration takes place under the same scenario when
		// expiration is disabled.
		//
		// Methodology:
		// - Start with the map such that second half of the values expire one
		//   second after the first half
		// - Set the mock time such that only the first half of the items have
		//   expired but is still prior to the next expiration scan
		// - Put a new value into the map and remove it to ensure expiration
		//   is not triggered
		// - Confirm the public methods treat the items as expired (unless
		//   expiration is disabled)
		// - Confirm the items are physically still in the map
		expirationDisabled := test.ttl == 0
		repopulateMap(tc)
		mockNow = baseNow.Add(test.ttl + time.Second)
		putMapAndAssertNumEvicted(tc, keys[numItems], vals[numItems], 0)
		m.Delete(keys[numItems])
		for i := 0; i < numItems/2; i++ {
			assertMapMembership(tc, keys[i], vals[i], expirationDisabled)
			assertInternMapMembership(tc, keys[i], vals[i], true)
		}
		for i := numItems / 2; i < numItems; i++ {
			assertMapMembership(tc, keys[i], vals[i], true)
		}

		// Ensure the expected keys and vals are returned in the expected order.
		wantKeys := keys[numItems/2 : numItems]
		wantVals := vals[numItems/2 : numItems]
		if expirationDisabled {
			wantKeys = keys[0:numItems]
			wantVals = vals[0:numItems]
		}
		assertMapKeysAndVals(tc, wantKeys, wantVals)

		// Ensure all expired items removed after the expiration scan interval
		// when explicitly requested when expiration is enabled.
		//
		// Conversely, ensure items that would be expired are not removed when
		// expiration is disabled even when explicitly requested.
		repopulateMap(tc)
		mockNow = baseNow.Add(expireScanInterval + time.Second)
		evictMapExpiredAndAssertNumEvicted(tc, numItems, expirationDisabled)
		for i := 0; i < numItems; i++ {
			assertMapMembership(tc, keys[i], vals[i], expirationDisabled)
			assertInternMapMembership(tc, keys[i], vals[i], expirationDisabled)
		}

		// Ensure expired items are removed prior to the expiration scan
		// interval when explicitly requested and expiration is enabled.
		//
		// Also, ensure no expiration takes place under the same scenario when
		// expiration is disabled.
		//
		// Methodology:
		// - Start with the map such that second half of the values expire one
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
		repopulateMap(tc)
		mockNow = baseNow.Add(test.ttl + time.Second)
		m.EvictExpiredNow()
		for i := 0; i < numItems/2; i++ {
			assertMapMembership(tc, keys[i], vals[i], expirationDisabled)
			assertInternMapMembership(tc, keys[i], vals[i], expirationDisabled)
		}
		for i := numItems / 2; i < numItems; i++ {
			assertMapMembership(tc, keys[i], vals[i], true)
		}
		mockNow = baseNow.Add(test.ttl + 2*time.Second)
		m.EvictExpiredNow()
		for i := 0; i < numItems; i++ {
			assertMapMembership(tc, keys[i], vals[i], expirationDisabled)
			assertInternMapMembership(tc, keys[i], vals[i], expirationDisabled)
		}

		// Ensure putting an item with the same key as an expired item updates
		// that item so it is no longer expired.
		//
		// Also, ensure no expiration takes place under the same scenario when
		// expiration is disabled.
		//
		// Methodology:
		// - Start with the map such that second half of the values expire one
		//   second after the first half
		// - Set the mock time such that only the first half of the items have
		//   expired but is still prior to the next expiration scan
		// - Put the first and last items in the map again so their expiration
		//   times are updated
		// - Run explicit expiration
		// - Confirm the first item and the second half are NOT expired and
		//   the remaining items in the first half ARE expired (unless disabled)
		// - Set the mock time such that the second half of the items have
		//   expired but is still prior to the next expiration scan
		// - Run explicit expiration
		// - Confirm the first and last items are NOT expired and the remaining
		//   items ARE expired (unless disabled)
		finalKey, finalVal := keys[numItems-1], vals[numItems-1]
		repopulateMap(tc)
		mockNow = baseNow.Add(test.ttl + time.Second)
		putMapAndAssertNumEvicted(tc, keys[0], vals[0], 0)
		putMapAndAssertNumEvicted(tc, finalKey, finalVal, 0)
		evictMapExpiredAndAssertNumEvicted(tc, numItems/2-1, expirationDisabled)
		assertMapMembership(tc, keys[0], vals[0], true)
		for i := 1; i < numItems/2; i++ {
			assertMapMembership(tc, keys[i], vals[i], expirationDisabled)
			assertInternMapMembership(tc, keys[i], vals[i], expirationDisabled)
		}
		for i := numItems / 2; i < numItems; i++ {
			assertMapMembership(tc, keys[i], vals[i], true)
		}
		mockNow = baseNow.Add(test.ttl + 2*time.Second)
		evictMapExpiredAndAssertNumEvicted(tc, numItems/2-1, expirationDisabled)
		assertMapMembership(tc, keys[0], vals[0], true)
		for i := 1; i < numItems/2; i++ {
			assertMapMembership(tc, keys[i], vals[i], expirationDisabled)
			assertInternMapMembership(tc, keys[i], vals[i], expirationDisabled)
		}
		for i := numItems / 2; i < numItems-1; i++ {
			assertMapMembership(tc, keys[i], vals[i], expirationDisabled)
			assertInternMapMembership(tc, keys[i], vals[i], expirationDisabled)
		}
		assertMapMembership(tc, finalKey, finalVal, true)

		// Ensure putting an item with a 0 TLL overrides the default TTL and
		// disables expiration for that item.
		//
		// Also, ensure no expiration takes place under the same scenario when
		// expiration is disabled.
		//
		// Methodology:
		// - Start with the map such that second half of the values expire one
		//   second after the first half
		// - Put the last item again with a TTL of 0 to disable its expiration
		// - Set the mock time such that all items would be expired and the
		//   expire scan interval has elapsed
		// - Run explicit expiration
		// - Confirm all of the items except that last one ARE expired (unless
		//   disabled) and the last is NOT expired.
		const ttl0 = 0
		repopulateMap(tc)
		putMapTTLAndAssertNumEvicted(tc, finalKey, finalVal, ttl0, 0)
		mockNow = baseNow.Add(expireScanInterval + 3*time.Second)
		evictMapExpiredAndAssertNumEvicted(tc, numItems-1, expirationDisabled)
		for i := 0; i < numItems-2; i++ {
			assertMapMembership(tc, keys[i], vals[i], expirationDisabled)
			assertInternMapMembership(tc, keys[i], vals[i], expirationDisabled)
		}
		assertMapMembership(tc, finalKey, finalVal, true)

		// Ensure putting a new item that would ordinarily cause an eviction of
		// the least recently used item due to exceeding the limit takes the
		// place of expired items instead.
		//
		// Methodology:
		// - Start with the map such that the last entry expires prior to the
		//   next expire scan interval and the remaining items all expire after
		//   it
		//   - Since the final item is added last, it will be the **mru** item
		//     and thus ordinarily would not be evicted when the limit is
		//     exceeded
		// - Set the mock time such that both the last item is expired and the
		//   expire scan interval has elapsed, but the remaining items are NOT
		//   expired
		// - Put a new item in the map that would exceed the max limit
		// - Confirm only the expired item was removed and the new item took its
		//   place
		if !expirationDisabled {
			mockNow = baseNow
			m.Clear()
			mockNow = baseNow.Add(expireScanInterval - time.Second)
			for i := 0; i < numItems; i++ {
				putMapAndAssertNumEvicted(tc, keys[i], vals[i], 0)
			}
			mockNow = baseNow
			putMapAndAssertNumEvicted(tc, keys[numItems], vals[numItems], 0)
			mockNow = baseNow.Add(expireScanInterval + time.Second)
			putMapAndAssertNumEvicted(tc, keys[numItems+1], vals[numItems+1], 1)
			assertMapMembership(tc, keys[numItems], vals[numItems], false)
			for i := 0; i < numItems; i++ {
				assertMapMembership(tc, keys[i], vals[i], true)
			}
			assertMapMembership(tc, keys[numItems+1], vals[numItems+1], true)
		}
	}
}

// TestMapPerItemExpiration ensures the map per-item expiration when no default
// expiration is specified behaves as expected including lazy expiration and the
// interaction between expiration and map limits.
func TestMapPerItemExpiration(t *testing.T) {
	t.Parallel()

	// Create some keys and values to use in testing the map code.  A couple of
	// extra values are created to ensure there are unique values to exceed the
	// map limit with.
	const numItems = 10
	keys, vals := makeKeysAndVals(numItems + 2)

	// Create a mock time to control expiration testing.
	baseNow := time.Now()
	mockNow := baseNow
	mockNowFn := func() time.Time { return mockNow }

	// repopulateMap resets the map to the following state:
	//
	// - Half the entries with expiration at baseNow + ttl
	// - The remaining entries with expiration at baseNow + ttl + 1
	// - The next expire scan interval at baseNow + expireScanInterval
	//
	// This ensures each half has different expiration times.
	repopulateMap := func(tc *testContextMap, piTTL time.Duration) {
		mockNow = baseNow
		tc.m.Clear()
		for i := 0; i < numItems/2; i++ {
			putMapTTLAndAssertNumEvicted(tc, keys[i], vals[i], piTTL, 0)
		}
		mockNow = baseNow.Add(time.Second)
		for i := numItems / 2; i < numItems; i++ {
			putMapTTLAndAssertNumEvicted(tc, keys[i], vals[i], piTTL, 0)
		}
	}

	tests := []struct {
		name string        // test description
		ttl  time.Duration // per-item TTL
	}{
		{name: "expiration disabled", ttl: 0},
		{name: "half expire scan interval", ttl: expireScanInterval / 2},
		{name: "expire scan interval - 1", ttl: expireScanInterval - time.Second},
	}

	for _, test := range tests {
		// Create a new map with no timeouts by default that is big enough to
		// hold all test values.  Also override the now function so the current
		// time can be manipulated for testing expiration semantics.
		m := NewMap[uint64, uint32](numItems + 1)
		m.nowFn = mockNowFn
		m.nextExpireScan = baseNow.Add(expireScanInterval)
		tc := &testContextMap{t: t, name: test.name, m: m}

		// Ensure expired items are not physically removed until an expiration
		// scan is triggered, but the public methods treat them as if they were
		// already removed.  Also, ensure that an expiration scan is not
		// triggered prior to the expiration scan interval passing.  Finally,
		// ensure no expiration takes place under the same scenario when
		// expiration is disabled.
		//
		// Methodology:
		// - Start with the map such that second half of the values expire one
		//   second after the first half
		// - Set the mock time such that only the first half of the items have
		//   expired but is still prior to the next expiration scan
		// - Put a new value into the map via the method that allows specifying
		//   the per-item TTL and remove it to ensure expiration is not
		//   triggered
		// - Confirm the public methods treat the items as expired (unless
		//   expiration is disabled)
		// - Confirm the items are physically still in the map
		expirationDisabled := test.ttl == 0
		repopulateMap(tc, test.ttl)
		mockNow = baseNow.Add(test.ttl + time.Second)
		putMapTTLAndAssertNumEvicted(tc, keys[numItems], vals[numItems],
			test.ttl, 0)
		m.Delete(keys[numItems])
		for i := 0; i < numItems/2; i++ {
			assertMapMembership(tc, keys[i], vals[i], expirationDisabled)
			assertInternMapMembership(tc, keys[i], vals[i], true)
		}
		for i := numItems / 2; i < numItems; i++ {
			assertMapMembership(tc, keys[i], vals[i], true)
		}

		// Ensure the expected keys and vals are returned in the expected order.
		wantKeys := keys[numItems/2 : numItems]
		wantVals := vals[numItems/2 : numItems]
		if expirationDisabled {
			wantKeys = keys[0:numItems]
			wantVals = vals[0:numItems]
		}
		assertMapKeysAndVals(tc, wantKeys, wantVals)

		// Ensure expired items are removed when putting items with a per-item
		// TTL specified after the expire scan interval has elapsed.
		//
		// Methodology:
		// - Start with the map populated with items that all have individually
		//   set the TTL specified by the test
		// - Set the mock time such that all items have expired and it is after
		//   the next expiration scan
		// - Put a new value into the map via the method that allows specifying
		//   the per-item TTL
		// - Confirm all of the original items are evicted (unless expiration is
		//   disabled)
		// - Confirm the public methods treat the items as expired (unless
		//   expiration is disabled)
		// - Confirm the items are physically still in the map
		repopulateMap(tc, test.ttl)
		mockNow = baseNow.Add(expireScanInterval + time.Second)
		wantNumEvicted := numItems
		if expirationDisabled {
			wantNumEvicted = 0
		}
		putMapTTLAndAssertNumEvicted(tc, keys[numItems], vals[numItems],
			test.ttl, uint32(wantNumEvicted))
		for i := 0; i < numItems; i++ {
			assertMapMembership(tc, keys[i], vals[i], expirationDisabled)
			assertInternMapMembership(tc, keys[i], vals[i], expirationDisabled)
		}
	}
}

// TestMapHitRatio ensures the map hit ratio reporting behaves as expected with
// and without expiration disabled.
func TestMapHitRatio(t *testing.T) {
	t.Parallel()

	// Create some keys and values to use in testing the map code.
	const totalItems = 50
	keys, vals := makeKeysAndVals(totalItems)

	// Create a mock time to control expiration testing.
	baseNow := time.Now()
	mockNow := baseNow
	mockNowFn := func() time.Time { return mockNow }

	tests := []struct {
		name       string        // test description
		ttl        time.Duration // default TTL for all items
		numItems   uint32        // number of items to populate
		numExpired uint32        // number of items to expire
		keyIdxs    []uint64      // indices of keys to get
		expected   []float64     // expected hit ratio after each get
	}{{
		name:       "hit ratio 100%, expiration disabled, 5 would be expired",
		ttl:        0,
		numItems:   10,
		numExpired: 5,
		keyIdxs:    []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		expected:   []float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
	}, {
		name:       "hit ratio would be 100%, but 1 item expired",
		ttl:        expireScanInterval / 2,
		numItems:   5,
		numExpired: 1,
		keyIdxs:    []uint64{0, 1, 2, 3, 4},
		expected:   []float64{0, 50, 66.67, 75, 80},
	}, {
		name:     "hit ratio 50% alternating, expiration disabled",
		numItems: 10,
		ttl:      0,
		keyIdxs:  []uint64{0, 10, 1, 11, 2, 12, 3, 13, 4, 14},
		expected: []float64{100, 50, 66.67, 50, 60, 50, 57.14, 50, 55.56, 50},
	}, {
		name:       "hit ratio 50% alternating due to expired items",
		numItems:   10,
		numExpired: 5,
		ttl:        expireScanInterval / 2,
		keyIdxs:    []uint64{5, 0, 6, 1, 7, 2, 8, 3, 9, 4},
		expected:   []float64{100, 50, 66.67, 50, 60, 50, 57.14, 50, 55.56, 50},
	}, {
		name:       "hit ratio 10% descending, expiration disabled, 3 would be",
		numItems:   10,
		numExpired: 3,
		ttl:        0,
		keyIdxs:    []uint64{0, 10, 11, 12, 13, 14, 15, 16, 17, 18},
		expected:   []float64{100, 50, 33.33, 25, 20, 16.67, 14.29, 12.5, 11.11, 10},
	}, {
		name:     "hit ratio 0% single item, expiration disabled",
		numItems: 10,
		ttl:      0,
		keyIdxs:  []uint64{10},
		expected: []float64{0},
	}, {
		name:       "hit ratio 0% due to all expired",
		numItems:   5,
		numExpired: 5,
		ttl:        expireScanInterval / 2,
		keyIdxs:    []uint64{0, 1, 2, 3, 4},
		expected:   []float64{0, 0, 0, 0, 0},
	}}

	for _, test := range tests {
		if len(test.keyIdxs) != len(test.expected) {
			t.Fatalf("%q: test data key idx len mismatch: %d != %d", test.name,
				len(test.keyIdxs), len(test.expected))
		}
		if test.numExpired > test.numItems {
			t.Fatalf("%q: test data num expired exceeds num items: %d > %d",
				test.name, test.numExpired, test.numItems)
		}

		// Create a new map with a TTL specified by the test that is big enough
		// to hold all test values.  Also override the now function so the
		// current time can be manipulated for forcing expirations.
		mockNow = baseNow
		m := NewMapWithDefaultTTL[uint64, uint32](totalItems+1, test.ttl)
		m.nowFn = mockNowFn
		m.nextExpireScan = baseNow.Add(expireScanInterval)
		tc := &testContextMap{t: t, name: test.name, m: m}

		// Ensure the hit ratio starts at 100% and populate the map such that
		// number of items specified by the test are expired.
		assertMapHitRatio(tc, 100)
		for i := uint32(0); i < test.numExpired; i++ {
			m.Put(keys[i], vals[i])
		}
		mockNow = baseNow.Add(2 * time.Second)
		for i := test.numExpired; i < test.numItems; i++ {
			m.Put(keys[i], vals[i])
		}
		mockNow = baseNow.Add(test.ttl + time.Second)

		// Ensure peeking at items does not modify the hit ratio.
		for _, keyIdx := range test.keyIdxs {
			tc.name = fmt.Sprintf("%s-keyidx %d", test.name, keyIdx)
			m.Peek(keys[keyIdx])
			assertMapHitRatio(tc, 100)
		}

		// Ensure existence checking does not modify the hit ratio.
		for _, keyIdx := range test.keyIdxs {
			tc.name = fmt.Sprintf("%s-keyidx %d", test.name, keyIdx)
			m.Exists(keys[keyIdx])
			assertMapHitRatio(tc, 100)
		}

		// Attempt to get each key specified by the test data and ensure the
		// hit ratio is as expected.
		for sliceIdx, keyIdx := range test.keyIdxs {
			tc.name = fmt.Sprintf("%s-keyidx %d", test.name, keyIdx)
			m.Get(keys[keyIdx])
			assertMapHitRatio(tc, test.expected[sliceIdx])
		}

		// Ensure clearing the map returns the hit ratio to 100%.
		tc.name = test.name
		m.Clear()
		assertMapHitRatio(tc, 100)
	}
}
