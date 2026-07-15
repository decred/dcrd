// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ratelimit

import (
	"math"
	"testing"
	"time"
)

// TestLimiter ensures functionality related to [Limiter.Allow] works as
// intended such as the token regeneration rate, burst handling, allowed
// reporting and time until next allowed.  It also includes tests for some
// corner cases such as negative rates and backwards time jumps.
func TestLimiter(t *testing.T) {
	t.Parallel()

	// Create a mock time to control testing.
	startTime := time.Now()
	mockNow := startTime
	mockNowFn := func() time.Time { return mockNow }

	// asSec returns the provided number of seconds as a duration.
	asSec := func(secs int) time.Duration {
		return time.Second * time.Duration(secs)
	}

	// asMs returns the provided number of milliseconds as a duration.
	asMs := func(millis int) time.Duration {
		return time.Millisecond * time.Duration(millis)
	}

	type perLimiterTest struct {
		off     time.Duration // offset from start time to test
		tokens  float64       // expected number of tokens at time
		next    time.Duration // expected duration until next event is allowed
		allowed bool          // expected allow value at time to test
	}

	tests := []struct {
		name            string           // test description
		rate            float64          // rate to use
		burst           uint32           // burst size to use
		perLimiterTests []perLimiterTest // tests to run against limiter
	}{{
		// Test burst capacity and regen rate with 1 event per sec and burst
		// size 5.
		name:  "1/s, burst 5",
		rate:  1.0,
		burst: 5,
		perLimiterTests: []perLimiterTest{
			// Bucket starts full and allows up to burst rate.
			{off: 0, tokens: 5, next: 0, allowed: true},
			{off: 0, tokens: 4, next: 0, allowed: true},
			{off: 0, tokens: 3, next: 0, allowed: true},
			{off: 0, tokens: 2, next: 0, allowed: true},
			{off: 0, tokens: 1, next: 0, allowed: true},
			// Tokens exhausted.
			{off: 0, tokens: 0, next: asSec(1), allowed: false},
			// Bucket refills at 1 per sec.
			{off: asSec(1), tokens: 1, next: 0, allowed: true},
			{off: asSec(2), tokens: 1, next: 0, allowed: true},
			{off: asSec(2), tokens: 0, next: asSec(1), allowed: false},
			// Back to full.
			{off: asSec(7), tokens: 5, next: 0, allowed: true},
			// Doesn't fill back up more than burst.
			{off: asSec(9), tokens: 5, next: 0, allowed: true},
		},
	}, {
		// Test burst capacity and regen rate with 1 event per sec and burst
		// size 1.
		name:  "1/s, burst 1",
		rate:  1.0,
		burst: 1,
		perLimiterTests: []perLimiterTest{
			// Bucket starts full and allows up to burst rate.
			{off: 0, tokens: 1, next: 0, allowed: true},
			// Tokens exhausted.
			{off: 0, tokens: 0, next: asSec(1), allowed: false},
			// Bucket refills at 1 per sec.
			{off: asSec(1), tokens: 1, next: 0, allowed: true},
			{off: asSec(2), tokens: 1, next: 0, allowed: true},
			{off: asSec(2), tokens: 0, next: asSec(1), allowed: false},
			// Back to full.
			{off: asSec(3), tokens: 1, next: 0, allowed: true},
			// Doesn't fill back up more than burst.
			{off: asSec(5), tokens: 1, next: 0, allowed: true},
		},
	}, {
		// Test backwards time jumps and regen rate with 100 events per sec and
		// burst size 6.  Thus the refill rate is 1 event per 10ms.
		name:  "100/s, burst 6",
		rate:  100.0,
		burst: 6,
		perLimiterTests: []perLimiterTest{
			// Start one second in the future, back one second to the starting
			// time and consume all tokens.
			{off: asSec(1), tokens: 6, next: 0, allowed: true},
			{off: 0, tokens: 5, next: 0, allowed: true},
			{off: 0, tokens: 4, next: 0, allowed: true},
			{off: 0, tokens: 3, next: 0, allowed: true},
			{off: 0, tokens: 2, next: 0, allowed: true},
			{off: 0, tokens: 1, next: 0, allowed: true},
			// Tokens exhausted.
			{off: 0, tokens: 0, next: asMs(10), allowed: false},
			{off: 0, tokens: 0, next: asMs(10), allowed: false},
			// Bucket refills at 1 per 10ms.
			{off: asMs(10), tokens: 1, next: 0, allowed: true},
			{off: asMs(10), tokens: 0, next: asMs(10), allowed: false},
			{off: asMs(20), tokens: 1, next: 0, allowed: true},
			// Back to full.
			{off: asMs(80), tokens: 6, next: 0, allowed: true},
			// Doesn't fill back up more than burst.
			{off: asMs(100), tokens: 6, next: 0, allowed: true},
		},
	}, {
		// Test burst capacity with 1 event per 10 secs and burst size 10.
		name:  "1 per 10s, burst 10",
		rate:  0.1,
		burst: 10,
		perLimiterTests: []perLimiterTest{
			{off: 0, tokens: 10, next: 0, allowed: true},
			{off: 0, tokens: 9, next: 0, allowed: true},
			{off: 0, tokens: 8, next: 0, allowed: true},
			{off: 0, tokens: 7, next: 0, allowed: true},
			{off: 0, tokens: 6, next: 0, allowed: true},
			{off: 0, tokens: 5, next: 0, allowed: true},
			{off: 0, tokens: 4, next: 0, allowed: true},
			{off: 0, tokens: 3, next: 0, allowed: true},
			{off: 0, tokens: 2, next: 0, allowed: true},
			{off: 0, tokens: 1, next: 0, allowed: true},
			// Tokens exhausted.
			{off: 0, tokens: 0, next: asSec(10), allowed: false},
			{off: 0, tokens: 0, next: asSec(10), allowed: false},
			// Bucket refills at 1 per 10sec.
			{off: asSec(1), tokens: 0.1, next: asSec(9), allowed: false},
			{off: asSec(5), tokens: 0.5, next: asSec(5), allowed: false},
			{off: asSec(10), tokens: 1, next: 0, allowed: true},
			{off: asSec(10), tokens: 0, next: asSec(10), allowed: false},
			// Back to full.
			{off: asSec(110), tokens: 10, next: 0, allowed: true},
			// Doesn't fill back up more than burst.
			{off: asSec(130), tokens: 10, next: 0, allowed: true},
		},
	}, {
		// Test negative rates do not regenerate tokens.
		name:  "negative rate, burst 3",
		rate:  -1.0,
		burst: 3,
		perLimiterTests: []perLimiterTest{
			{off: 0, tokens: 3, next: 0, allowed: true},
			{off: 0, tokens: 2, next: 0, allowed: true},
			{off: 0, tokens: 1, next: 0, allowed: true},
			// Tokens exhausted and do not regenerate.
			{off: 0, tokens: 0, next: Forever, allowed: false},
			{off: 0, tokens: 0, next: Forever, allowed: false},
		},
	}, {
		// Test burst size of 0 never allows any events.
		name:  "1/s, burst 0",
		rate:  1.0,
		burst: 0,
		perLimiterTests: []perLimiterTest{
			{off: 0, tokens: 0, next: Forever, allowed: false},
			{off: 0, tokens: 0, next: Forever, allowed: false},
		},
	}}

	for _, test := range tests {
		// Create limiter with the rate and burst values specified by the test.
		// Also override the now function so the cur time can be manipulated.
		limiter := New(test.rate, test.burst)
		limiter.nowFn = mockNowFn

		// Ensure burst rate is returned properly.
		if gotBurst := limiter.Burst(); gotBurst != test.burst {
			t.Errorf("%q: mismatched burst -- got %v, want %v", test.name,
				gotBurst, test.burst)
		}

		for i, plTest := range test.perLimiterTests {
			// Ensure the expected number of tokens are reported.
			mockNow = startTime.Add(plTest.off)
			gotTokens := limiter.Tokens()
			if gotTokens != plTest.tokens {
				t.Errorf("%q-%d: mismatched tokens -- got %v, want %v",
					test.name, i, gotTokens, plTest.tokens)
				continue
			}

			// Ensure the expected duration until the next allowed event is
			// reported.
			gotNextAllowed := limiter.UntilNextAllowed()
			if gotNextAllowed != plTest.next {
				t.Errorf("%q-%d: mismatched next allowed -- got %v, want %v",
					test.name, i, gotNextAllowed, plTest.next)
				continue
			}

			// Ensure the expected allowed status is reported.
			gotAllowed := limiter.Allow()
			if gotAllowed != plTest.allowed {
				t.Errorf("%q-%d: mismatched allowed -- got %v, want %v",
					test.name, i, gotAllowed, plTest.allowed)
				continue
			}
		}
	}
}

// TestLimiterMaxDuration ensures any calculated durations that would exceed the
// max allowed value by [time.Duration] are clamped to [Forever].
func TestLimiterMaxDuration(t *testing.T) {
	// Create a mock time to control testing.
	mockNow := time.Now()
	mockNowFn := func() time.Time { return mockNow }

	// Since [time.Duration] is an int64 and is in nanoseconds, the maximum
	// duration that can be represented is 9223372036854775807 nanoseconds.
	//
	// Calculate the equivalent rate such that the time until the next allowed
	// event would exceed that and ensure it is properly clamped to [Forever].
	limiter := New(float64(time.Second)/float64(math.MaxInt64), 1)
	limiter.nowFn = mockNowFn
	if !limiter.Allow() {
		t.Fatalf("burst size of 1 should allow the event")
	}
	if gotDuration := limiter.UntilNextAllowed(); gotDuration != Forever {
		t.Fatalf("unexpected duration -- got %v, want %v", gotDuration, Forever)
	}

	// Ensure [Forever] is no longer returned once the duration no longer
	// exceeds the max after it previously did.
	mockNow = mockNow.Add(time.Millisecond)
	if gotDuration := limiter.UntilNextAllowed(); gotDuration == Forever {
		t.Fatalf("unexpected duration -- got %v, want < %v", gotDuration, Forever)
	}
}
