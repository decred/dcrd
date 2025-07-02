// Copyright (c) 2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"sync"
	"testing"
)

// TestMaybeUpdateSyncHeight ensures [SyncManager.maybeUpdateSyncHeight] and
// [SyncManager.SyncHeight] behave as expected including when updated
// concurrently.
func TestMaybeUpdateSyncHeight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string // test description
		n    int64  // new sync height
		want int64  // expected sync height
	}{{
		name: "5 > 0 => 5",
		n:    5,
		want: 5,
	}, {
		name: "5 == 5 => 5",
		n:    5,
		want: 5,
	}, {
		name: "10 > 5 => 10",
		n:    10,
		want: 10,
	}, {
		name: "5 < 10 => 10",
		n:    5,
		want: 10,
	}, {
		name: "9 < 10 => 10",
		n:    9,
		want: 10,
	}, {
		name: "7 < 10 => 10",
		n:    7,
		want: 10,
	}, {
		name: "20 > 10 => 20",
		n:    20,
		want: 20,
	}}

	// Ensure the result of setting each individual test produces the expected
	// result.  Also, determine the overall maximum value for the upcoming
	// asynchronous test.
	var mgr SyncManager
	var maxHeight int64
	for _, test := range tests {
		mgr.maybeUpdateSyncHeight(test.n)
		gotHeight := mgr.SyncHeight()
		if gotHeight != test.want {
			t.Errorf("%s: wrong result -- got: %d want: %d", test.name,
				gotHeight, test.want)
		}
		if gotHeight > maxHeight {
			maxHeight = gotHeight
		}
	}

	// Set each value in the tests via separate goroutines and ensure the final
	// result is the max of them all once the gouroutines have finished.
	mgr.syncHeight.Store(0)
	var wg sync.WaitGroup
	wg.Add(len(tests))
	for _, test := range tests {
		go func(n int64) {
			mgr.maybeUpdateSyncHeight(n)
			wg.Done()
		}(test.n)
	}
	wg.Wait()
	if gotHeight := mgr.SyncHeight(); gotHeight != maxHeight {
		t.Errorf("wrong async result -- got: %d want: %d", gotHeight, maxHeight)
	}
}
