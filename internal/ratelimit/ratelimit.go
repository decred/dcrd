// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package ratelimit implements a simple concurrent safe token bucket rate
// limiter.
package ratelimit

import (
	"math"
	"sync"
	"time"
)

// Forever represents an infinite duration.
const Forever time.Duration = math.MaxInt64

// Limiter provides a simple token bucket rate limiter for controlling the
// frequency of permitted events.  The token bucket algorithm used by this
// implementation works by starting with a fixed number of tokens specified by
// the burst size and refills the tokens at the specified rate.  Events are
// allowed so long as there is at least one token in the bucket at the time of
// the event.  It is ideal for use in traffic shaping and traffic policing.
//
// The limiter is primarily aimed at serving use cases where the intention is to
// drop events that exceed the rate limit by only processing events when
// [Limiter.Allow] reports true and dropping them otherwise.
//
// The current number of tokens in the bucket can be obtained with
// [Limiter.Tokens] and the fixed burst size is available via [Limiter.Burst].
//
// Callers that wish to block until the next event is allowed instead of merely
// dropping events that exceed the rate can make use of
// [Limiter.UntilNextAllowed] to determine how long they need to wait.
//
// [New] must be used to create a usable limiter since the zero value of this
// struct is not valid.
type Limiter struct {
	// These fields do not require a mutex since they are set at initialization
	// time and never modified after.
	nowFn func() time.Time

	// These fields are protected by the embedded mutex.
	//
	// rate is the number of events to allow per second.
	//
	// burst is the maximum amount of tokens to permit for handling rapid
	// bursts of events.
	//
	// tokens is the number of remaining tokens in the bucket.  More events are
	// permitted while it is greater than 0.
	//
	// updated is the last time tokens was updated.
	mtx     sync.Mutex
	rate    float64
	burst   float64
	tokens  float64
	updated time.Time
}

// New returns a token bucket rate limiter that allows events up to the provided
// rate, in number of events per second, while also allowing bursts up to the
// provided burst size.
//
// For example a rate of 10.5 with a burst size of 30 would allow an average of
// 10.5 events per second with periodic bursts of up to 30 events.
//
// In order to rate limit events to every X seconds (versus X events per
// second), specify the rate scaled by 1/X.
//
// Scale the rate accordingly for other time units.
//
// For example, to specify a rate of 15 events per minute, the rate would be
// 0.25 (15/60) and 450 events every 2 hours would be 450/(2*3600) = 0.0625.
func New(rate float64, burst uint32) *Limiter {
	return &Limiter{
		nowFn:  time.Now,
		rate:   rate,
		burst:  float64(burst),
		tokens: float64(burst),
	}
}

// Burst returns the burst size specified when the limiter was created.
//
// This function is safe for concurrent access.
func (l *Limiter) Burst() uint32 {
	l.mtx.Lock()
	burst := uint32(l.burst)
	l.mtx.Unlock()
	return burst
}

// durationToTokens returns the number of tokens that would refill during the
// provided duration at the given rate.
func durationToTokens(d time.Duration, rate float64) float64 {
	if rate <= 0 {
		return 0
	}
	return d.Seconds() * rate
}

// tokensAt returns the number of available tokens at the provided time.  Times
// prior to the last time the limiter was updated will return the current number
// of tokens in the bucket.
//
// This function MUST be called with the embedded mutex held (for reads).
func (l *Limiter) tokensAt(t time.Time) float64 {
	updated := l.updated
	if t.Before(updated) {
		updated = t
	}
	elapsed := t.Sub(updated)
	delta := durationToTokens(elapsed, l.rate)
	numTokens := l.tokens + delta
	if numTokens > l.burst {
		numTokens = l.burst
	}
	return numTokens
}

// Tokens returns the number of available tokens at the current time.
//
// This function is safe for concurrent access.
func (l *Limiter) Tokens() float64 {
	l.mtx.Lock()
	tokens := l.tokensAt(l.nowFn())
	l.mtx.Unlock()
	return tokens
}

// Allow returns true when an event is allowed to happen at the current time
// and, when it returns true, the state is updated to consume a token.
//
// Callers will typically want to process the event normally when true is
// returned and drop it otherwise.
//
// This function is safe for concurrent access.
func (l *Limiter) Allow() bool {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	now := l.nowFn()
	tokens := l.tokensAt(now)
	tokens--

	if tokens >= 0 && tokens <= l.burst {
		l.updated = now
		l.tokens = tokens
		return true
	}
	return false
}

// tokensToDuration returns the duration the provided number of tokens would
// take to refill at the given rate.
//
// This function is safe for concurrent access.
func tokensToDuration(tokens float64, rate float64) time.Duration {
	if rate <= 0 {
		return Forever
	}
	duration := (tokens / rate) * float64(time.Second)
	if uint64(duration) >= math.MaxInt64 {
		return Forever
	}
	return time.Duration(duration)
}

// UntilNextAllowed returns the duration that must elapse until the next event
// is allowed.  [Forever] is returned when no more events will ever be allowed,
// such as when there is a negative rate or a burst size of 0.
//
// This function is safe for concurrent access.
func (l *Limiter) UntilNextAllowed() time.Duration {
	// Events are never allowed with a burst size of 0.
	l.mtx.Lock()
	if l.burst == 0 {
		l.mtx.Unlock()
		return Forever
	}
	tokens := l.tokensAt(l.nowFn())
	rate := l.rate
	l.mtx.Unlock()

	// The next event is not allowed until there is at least one token, so
	// determine how much is needed to reach one.
	needed := 1 - tokens
	if needed <= 0 {
		// There is already one or more tokens available.
		return 0
	}

	// Convert the needed tokens into a duration based on the rate.
	return tokensToDuration(needed, rate)
}
