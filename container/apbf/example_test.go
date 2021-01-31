// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package apbf_test

import (
	"fmt"

	"github.com/decred/dcrd/container/apbf"
)

// This example demonstrates creating a new APBF, adding items to it up to the
// maximum capacity, querying the oldest item, then adding more items to it in
// order to cause the older items to be evicted.
func Example_basicUsage() {
	// Create a new filter that will always return true for the most-recent 1000
	// items and maintains a false positive rate on items that were never added
	// of 0.01% (1 in 10,000).
	const maxItems = 1000
	const fpRate = 0.0001
	filter := apbf.NewFilter(maxItems, fpRate)

	// Add items to the filter.
	for i := 0; i < maxItems; i++ {
		item := fmt.Sprintf("item %d", i)
		filter.Add([]byte(item))
	}

	// At this point, the maximum number of items guaranteed to return true has
	// been reached, so the first entry will still be a member.
	if !filter.Contains([]byte("item 0")) {
		fmt.Println("filter does not contain expected item 0")
		return
	}

	// Adding more items will eventually evict the oldest items.
	for i := 0; i < maxItems; i++ {
		item := fmt.Sprintf("item %d", i+maxItems)
		filter.Add([]byte(item))
	}
	if filter.Contains([]byte("item 0")) {
		fmt.Println("filter contains unexpected item 0")
		return
	}

	// Output:
	//
}
