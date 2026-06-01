// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ratelimit_test

import (
	"fmt"
	"math"
	"time"

	"github.com/decred/dcrd/internal/ratelimit"
)

// This example demonstrates creating and using a rate limiter that allows 10
// events per second with periodic bursts of up to 25 events.
func ExampleLimiter_Allow() {
	const eventsPerSec = 10
	const burstTokens = 25
	limiter := ratelimit.New(eventsPerSec, burstTokens)

	// Simulate a burst of events.  Ordinarily the if statement would be in
	// response to events that are externally generated, but the events are
	// simulated here with a loop for the purposes of the example.
	var numAllowed, numDropped uint64
	for range burstTokens + 10 {
		if limiter.Allow() {
			// Ordinarily this would process the allowed event.
			numAllowed++
		} else {
			numDropped++
		}
	}
	fmt.Printf("num events allowed: %v\n", numAllowed)
	fmt.Printf("num events dropped: %v\n", numDropped)

	// There will be no more tokens available since another event will not be
	// allowed for another 100 milliseconds (minus however long has already
	// elapsed since the final allowed event and reaching this point) given the
	// rate is 10 per second and the burst size has been exhausted.
	fmt.Printf("num tokens remaining: %v\n", math.Floor(limiter.Tokens()))
	const msPerEvent = time.Duration(float64(1000)/eventsPerSec) * time.Millisecond
	time.Sleep(msPerEvent)
	isAllowed := limiter.UntilNextAllowed() == 0
	fmt.Printf("event allowed after %v: %v\n", msPerEvent, isAllowed)

	// Output:
	// num events allowed: 25
	// num events dropped: 10
	// num tokens remaining: 0
	// event allowed after 100ms: true
}
