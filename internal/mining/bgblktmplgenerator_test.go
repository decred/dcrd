// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"testing"
	"time"
)

// TestWaitGroup ensures the API for the local waitGroup implementation behaves
// correctly.
func TestWaitGroup(t *testing.T) {
	// goWait is a helper that calls wg.Wait() on a gorutine and closes the
	// returned chan once Wait() returns.
	goWait := func(wg *waitGroup) chan struct{} {
		c := make(chan struct{})
		started := make(chan struct{})
		go func() {
			close(started)
			wg.Wait()
			close(c)
		}()

		// Allow Wait() to be called.
		<-started
		return c
	}

	// waitReturned returns true if the the passed channel created by
	// goWait is closed before a timeout.
	waitReturned := func(c chan struct{}) bool {
		select {
		case <-c:
			return true
		case <-time.After(time.Second):
			return false
		}
	}

	tests := []struct {
		name string
		test func(t *testing.T)
	}{{
		name: "Wait() before Add() returns immediately",
		test: func(t *testing.T) {
			var wg waitGroup
			c := goWait(&wg)
			if !waitReturned(c) {
				t.Fatalf("wait did not return immediately")
			}
		},
	}, {
		name: "Wait() only returns after Done()",
		test: func(t *testing.T) {
			var wg waitGroup
			wg.Add(1)
			c := goWait(&wg)
			if waitReturned(c) {
				t.Fatalf("Wait() before Done() should not return")
			}
			wg.Done()
			if !waitReturned(c) {
				t.Fatalf("Wait() after Done() should return")
			}
		},
	}, {
		name: "Wait() only returns after correct nb of Done() calls",
		test: func(t *testing.T) {
			var wg waitGroup
			wg.Add(2)
			c := goWait(&wg)
			wg.Done()
			if waitReturned(c) {
				t.Fatalf("Wait() before Done() should not return")
			}
			wg.Done()
			if !waitReturned(c) {
				t.Fatalf("Wait() after Done() should return")
			}
		},
	}, {
		name: "multiple Waits()",
		test: func(t *testing.T) {
			var wg waitGroup
			wg.Add(1)
			nb := 5
			chans := make([]chan struct{}, nb)
			for i := 0; i < nb; i++ {
				chans[i] = goWait(&wg)
			}

			// No chan should be signalled yet.
			for i := 0; i < nb; i++ {
				select {
				case <-chans[i]:
					t.Fatalf("unexpected Wait() return")
				default:
				}
			}

			wg.Done()

			// Every chan should be signalled now.
			for i := 0; i < nb; i++ {
				if !waitReturned(chans[i]) {
					t.Fatalf("Wait() should've returned")
				}
			}

		},
	}, {
		name: "Wait() after Add(0) returns immediately",
		test: func(t *testing.T) {
			var wg waitGroup
			wg.Add(0)
			c := goWait(&wg)
			if !waitReturned(c) {
				t.Fatalf("Wait() after Add(0) should return")
			}
		},
	}, {
		name: "reusing the waitGroup works",
		test: func(t *testing.T) {
			var wg waitGroup
			wg.Add(1)
			c := goWait(&wg)
			wg.Done()
			if !waitReturned(c) {
				t.Fatalf("Wait() after Done() should return")
			}

			wg.Add(1)
			c = goWait(&wg)
			wg.Done()
			if !waitReturned(c) {
				t.Fatalf("Second usage of Wait() after Done() should return")
			}

		},
	}, {
		name: "negative Add() panics",
		test: func(t *testing.T) {
			var wg waitGroup
			defer func() {
				if err := recover(); err == nil {
					t.Fatalf("Add(-1) did not panic")
				}
			}()
			wg.Add(-1)
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, tc.test)
	}
}
