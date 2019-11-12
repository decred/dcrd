// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package lru_test

import (
	"fmt"

	"github.com/decred/dcrd/lru"
)

// This example demonstrates creating a new cache instance, inserting items into
// the cache, causing an eviction of the least-recently-used item, and removing
// an item.
func Example_basicUsage() {
	// Create a new cache instance with the desired limit.
	const maxItems = 100
	cache := lru.NewCache(maxItems)

	// Insert items into the cache.
	for i := 0; i < maxItems; i++ {
		cache.Add(i)
	}

	// At this point, the cache has reached the limit, so the first entry will
	// still be a member of the cache.
	if !cache.Contains(0) {
		fmt.Println("cache does not contain expected item 0")
		return
	}

	// Adding another item will evict the least-recently-used item, which will
	// be the value 1 since 0 was just accessed above.
	cache.Add(int(maxItems) + 1)
	if cache.Contains(1) {
		fmt.Println("cache contains unexpected item 1")
		return
	}

	// Remove an item from the cache.
	cache.Delete(3)
	if cache.Contains(3) {
		fmt.Println("cache contains unexpected item 3")
		return
	}

	// Output:
	//
}

// This example demonstrates creating a new kv cache instance, inserting items
// into the cache, causing an eviction of the least-recently-used item, and
// removing an item.
func Example_basicKVUsage() {
	// Create a new cache instance with the desired limit.
	const maxItems = 100
	cache := lru.NewKVCache(maxItems)

	// Insert items into the cache.
	for i := 0; i < maxItems; i++ {
		cache.Add(i, i)
	}

	// At this point, the cache has reached the limit, so the first entry will
	// still be a member of the cache.
	if !cache.Contains(0) {
		fmt.Println("cache does not contain expected item 0")
		return
	}

	// Adding another item will evict the least-recently-used item, which will
	// be the value 1 since 0 was just accessed above.
	oneOverMax := int(maxItems) + 1
	cache.Add(oneOverMax, oneOverMax)
	if cache.Contains(1) {
		fmt.Println("cache contains unexpected item 1")
		return
	}

	// Remove an item from the cache.
	cache.Delete(3)
	if cache.Contains(3) {
		fmt.Println("cache contains unexpected item 3")
		return
	}

	// Output:
	//
}
