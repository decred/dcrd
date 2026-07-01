// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestSemaphore ensures the semaphore acquire, release, and context cancel
// semantics are as expected.
func TestSemaphore(t *testing.T) {
	// Create a closure that acquires a semaphore with a timeout.
	ctx := context.Background()
	timedAcquire := func(sem semaphore, timeout time.Duration) bool {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return sem.Acquire(ctx)
	}

	// Create a closure that tries to acquire a semaphore via the nonblocking
	// method with a timeout.
	timedTryAcquire := func(sem semaphore, timeout time.Duration) (bool, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		if timeout == 0 {
			cancel()
		} else {
			defer cancel()
		}
		return sem.TryAcquire(ctx)
	}

	// perSemTest describes a test to run against the same semaphore.
	type perSemTest struct {
		name        string // test description
		numAcquires uint32 // num to acquire
		numTries    uint32 // num to try to acquire via nonblocking method
		cancelTry   bool   // whether or not to cancel nonblocking try
		numReleases uint32 // num to release
	}

	tests := []struct {
		name        string       // test description
		cap         uint32       // capacity of the semaphore
		perSemTests []perSemTest // tests to run against same semaphore
		want        []bool       // expected results
	}{{
		name: "normal block/release behavior",
		cap:  2,
		perSemTests: []perSemTest{{
			name:        "cap 2 (0 acquired): acquire 3, release 1",
			numAcquires: 3,
			numReleases: 1,
		}, {
			name:        "cap 2 (1 acquired): acquire 2, release 0",
			numAcquires: 2,
			numReleases: 0,
		}, {
			name:        "cap 2 (2 acquired): acquire 1, release 2",
			numAcquires: 1,
			numReleases: 2,
		}},
		want: []bool{true, true, false, true, false, false},
	}, {
		// Releasing more than acquired ignores the extra release and does not
		// influence future ops.
		name: "relase more than acquired",
		cap:  5,
		perSemTests: []perSemTest{{
			name:        "cap 5 (0 acquired): acquire 1, release 2",
			numAcquires: 1,
			numReleases: 2,
		}, {
			name:        "cap 5 (0 acquired): acquire 5, release 1",
			numAcquires: 5,
			numReleases: 1,
		}, {
			name:        "cap 5 (4 acquired): acquire 2, release 5",
			numAcquires: 2,
			numReleases: 5,
		}},
		want: []bool{true, true, true, true, true, true, true, false},
	}, {
		name: "nonblocking tryacquire and blocking acquire mixed",
		cap:  3,
		perSemTests: []perSemTest{{
			name:        "cap 3 (0 acquired): try 1, release 2",
			numTries:    1,
			numReleases: 2,
		}, {
			name:        "cap 3 (0 acquired): acquire 2, try 1, release 1",
			numAcquires: 2,
			numTries:    1,
			numReleases: 1,
		}, {
			name:        "cap 3 (2 acquired): acquire 1, try 2, release 3",
			numAcquires: 1,
			numTries:    2,
			numReleases: 3,
		}},
		want: []bool{true, true, true, true, true, false, false},
	}, {
		name: "nonblocking tryacquire with canceled context",
		cap:  1,
		perSemTests: []perSemTest{{
			name:        "cap 1 (0 acquired): try 1 (canceled), release 0",
			numTries:    1,
			cancelTry:   true,
			numReleases: 0,
		}, {
			name:        "cap 1 (0 acquired): acquire 1, try 1, release 1",
			numAcquires: 1,
			numTries:    1,
			numReleases: 1,
		}, {
			name:        "cap 1 (0 acquired): try 2, release 1",
			numAcquires: 0,
			numTries:    2,
			numReleases: 1,
		}},
		want: []bool{false, true, false, true, false},
	}}

	for _, test := range tests {
		// Create semaphore with the capacity specified in the test and the
		// a slice to hold the results.
		sem := makeSemaphore(test.cap)
		results := make([]bool, 0, len(test.want))

		// Perform each sequence of acquires, try acquires, and releases as
		// specified by the per semaphore tests.
		for _, psTest := range test.perSemTests {
			const timeout = 10 * time.Millisecond
			for range psTest.numAcquires {
				results = append(results, timedAcquire(sem, timeout))
			}
			for range psTest.numTries {
				// Override timeout with a duration 0 and expected error when
				// the flag to force the context for the try acquire to be
				// canceled is specified.
				var wantErr error
				tryTimeout := timeout
				if psTest.cancelTry {
					tryTimeout = 0
					wantErr = context.DeadlineExceeded
				}
				acquired, err := timedTryAcquire(sem, tryTimeout)
				if !errors.Is(err, wantErr) {
					t.Fatalf("%q: unexpected try acquire error: got %v, want %v",
						psTest.name, err, wantErr)
				}
				results = append(results, acquired)
			}
			for range psTest.numReleases {
				sem.Release()
			}
		}

		if len(results) != len(test.want) {
			t.Errorf("%q: unexpected number of results: got %d, want %d",
				test.name, len(results), len(test.want))
		}
		for i := range results {
			if results[i] != test.want[i] {
				t.Errorf("%q: unexpected result for [%d]: got %v, want %v",
					test.name, i, results[i], test.want[i])
			}
		}

		// Ensure all acquires were released as expected.
		if numAcquired := uint32(len(sem)); numAcquired != 0 {
			t.Errorf("%q: unexpected final semaphore count: got %v, want %v",
				test.name, numAcquired, 0)
		}
	}
}
