// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package lru_test

import (
	"fmt"
	"time"

	"github.com/decred/dcrd/container/lru"
)

// This example demonstrates creating a new map instance, inserting items into
// the map, existence checking, looking up an item, causing an eviction of the
// least recently used item, and removing an item.
func Example_basicMapUsage() {
	// Create a new map instance with the desired limit and no time-based
	// expiration of items.
	const maxItems = 10
	m := lru.NewMap[int, int](maxItems)

	// Insert items into the map.
	for i := 0; i < maxItems; i++ {
		m.Put(i, (i+1)*100)
	}

	// At this point, the map has reached the limit, so the first entry will
	// still be a member.
	if !m.Exists(0) {
		fmt.Println("map does not contain expected item 0")
		return
	}

	// Lookup an item.
	if v, ok := m.Get(0); !ok || v != 100 {
		fmt.Println("map does not contain expected value for item 0")
		return
	}

	// Inserting another item will evict the least recently used item, which
	// will be the item with key 1 since 0 was just accessed above.
	const oneOverMax = maxItems + 1
	numEvicted := m.Put(oneOverMax, oneOverMax)
	if numEvicted != 1 {
		fmt.Printf("expected one evicted item, but got %d", numEvicted)
		return
	}
	if m.Exists(1) {
		fmt.Println("map contains unexpected item 1")
		return
	}

	// Remove an item.
	m.Delete(3)
	if m.Exists(3) {
		fmt.Println("map contains unexpected item 3")
		return
	}

	// Output:
	//
}

// This example demonstrates creating a new map instance with time-based
// expiration, inserting items into it, and manually triggering removal of
// expired items.
func ExampleMap_EvictExpiredNow() {
	// Create a new map instance with the desired limit and a time-based
	// expiration of items for one millisecond.  Callers would typically want to
	// use much longer durations.
	const maxItems = 10
	const ttl = time.Millisecond
	m := lru.NewMapWithDefaultTTL[string, string](maxItems, ttl)

	// Insert an item, wait for the item to expire, and remove expired items.
	//
	// Ordinarily, callers do not need to explicitly call the removal method
	// since expired items are periodically removed as items are added and
	// updated.  However, it is done here for the purposes of the example.
	const key = "foo"
	m.Put(key, "bar")
	time.Sleep(ttl * 2)
	numEvicted := m.EvictExpiredNow()
	if numEvicted != 1 {
		fmt.Printf("expected one evicted item, but got %d", numEvicted)
		return
	}
	if m.Exists(key) {
		fmt.Printf("map contains unexpected expired item %q\n", key)
		return
	}

	// Output:
	//
}

// This example demonstrates creating a new set instance, inserting items into
// the set, checking set containment, causing an eviction of the least recently
// used item, and removing an item.
func Example_basicSetUsage() {
	// Create a new set instance with the desired limit and no time-based
	// expiration of items.
	const maxItems = 10
	set := lru.NewSet[int](maxItems)

	// Insert items into the set.
	for i := 0; i < maxItems; i++ {
		set.Put(i)
	}

	// At this point, the set has reached the limit, so the first entry will
	// still be a member.  Note that Exists does not update any state so it
	// will not modify the eviction priority or the hit ratio.
	if !set.Exists(0) {
		fmt.Println("set does not contain expected item 0")
		return
	}

	// Check set containment.  This modifies the priority to be the most
	// recently used item so it will be evicted last.
	if !set.Contains(0) {
		fmt.Println("set does not contain expected item 0")
		return
	}

	// Inserting another item will evict the least recently used item, which
	// will be the item 1 since item 0 was just accessed via Contains.
	numEvicted := set.Put(maxItems + 1)
	if numEvicted != 1 {
		fmt.Printf("expected one evicted item, but got %d", numEvicted)
		return
	}
	if set.Exists(1) {
		fmt.Println("set contains unexpected item 1")
		return
	}

	// Remove an item.
	set.Delete(3)
	if set.Exists(3) {
		fmt.Println("set contains unexpected item 3")
		return
	}

	// Output:
	//
}

// This example demonstrates per-item expiration by creating a new set instance
// with no time-based expiration, inserting items into it, updating one of the
// items to have a timeout, and manually triggering removal of expired item.
func ExampleSet_PutWithTTL() {
	// Create a new set instance with the desired limit and no time-based
	// expiration of items.
	const maxItems = 10
	set := lru.NewSet[int](maxItems)

	// Insert items into the set.
	for i := 0; i < maxItems; i++ {
		set.Put(i)
	}

	// Update item 1 to expire after one millisecond, wait for the item to
	// expire, and remove expired items.  Callers would typically want to use
	// much longer durations.
	//
	// Since all of the other items are not set to expire by default, only the
	// single item updated to have a TTL will be expired.
	//
	// Ordinarily, callers do not need to explicitly call the removal method
	// since expired items are periodically removed as items are added and
	// updated.  However, it is done here for the purposes of the example.
	const ttl = time.Millisecond
	numEvicted := set.PutWithTTL(1, ttl)
	if numEvicted != 0 {
		fmt.Printf("expected no evicted items, but got %d", numEvicted)
		return
	}
	time.Sleep(ttl * 2)
	numEvicted = set.EvictExpiredNow()
	if numEvicted != 1 {
		fmt.Printf("expected one evicted item, but got %d", numEvicted)
		return
	}

	// Output:
	//
}
