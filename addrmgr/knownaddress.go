// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package addrmgr

import (
	"math"
	"sync"
	"time"
)

// KnownAddress tracks information about a known network address that is used
// to determine how viable an address is.
type KnownAddress struct {
	// mtx is used to ensure safe concurrent access to methods on a known
	// address instance.
	mtx sync.Mutex

	// na is the primary network address that the known address represents.
	na *NetAddress

	// srcAddr is the network address of the peer that suggested the primary
	// network address.
	srcAddr *NetAddress

	// The following fields track the attempts made to connect to the primary
	// network address.  Initially connecting to a peer counts as an attempt,
	// and a successful version message exchange resets the number of attempts
	// to zero.
	attempts    int
	lastattempt time.Time
	lastsuccess time.Time

	// tried indicates whether the address currently exists in a tried bucket.
	tried bool

	// refs represents the total number of new buckets that the known address
	// exists in.  This is updated as the address moves between new and tried
	// buckets.
	refs int
}

// NetAddress returns the underlying addrmgr.NetAddress associated with the
// known address.
func (ka *KnownAddress) NetAddress() *NetAddress {
	ka.mtx.Lock()
	defer ka.mtx.Unlock()
	return ka.na
}

// LastAttempt returns the last time the known address was attempted.
func (ka *KnownAddress) LastAttempt() time.Time {
	ka.mtx.Lock()
	defer ka.mtx.Unlock()
	return ka.lastattempt
}

// chance returns the selection probability for a known address.  The priority
// depends upon how recently the address has been seen, how recently it was last
// attempted, and how often attempts to connect to it have failed.
func (ka *KnownAddress) chance() float64 {
	ka.mtx.Lock()
	defer ka.mtx.Unlock()

	// Very recent attempts are less likely to be retried.
	const minChance = 0.01
	if ka.lastattempt.IsZero() ||
		time.Since(ka.lastattempt) < 10*time.Minute {
		return minChance
	}

	// Failed attempts deprioritise.
	c := 1.0 / math.Pow(1.5, float64(ka.attempts))

	return math.Max(c, minChance)
}

// isBad returns true if the address in question has not been tried in the last
// minute and meets one of the following criteria:
// 1) It claims to be from the future
// 2) It hasn't been seen in over a month
// 3) It has failed at least three times and never succeeded
// 4) It has failed a total of maxFailures in the last week
// An address that meets any of these criteria is assumed to be worthless.
func (ka *KnownAddress) isBad() bool {
	ka.mtx.Lock()
	defer ka.mtx.Unlock()
	now := time.Now()

	switch {
	// Wait a minute after the last check.
	case ka.lastattempt.After(now.Add(-1 * time.Minute)):
		return false

	// From the future?
	case ka.na.Timestamp.After(now.Add(10 * time.Minute)):
		return true

	// Over a month old?
	case ka.na.Timestamp.Before(now.Add(-1 * numMissingDays * time.Hour * 24)):
		return true

	// Never succeeded?
	case ka.lastsuccess.IsZero() && ka.attempts >= numRetries:
		return true

	// Hasn't succeeded in too long?
	case !ka.lastsuccess.After(now.Add(-1*minBadDays*time.Hour*24)) &&
		ka.attempts >= maxFailures:
		return true

	default:
		return false
	}
}
