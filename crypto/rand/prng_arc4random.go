// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build openbsd && go1.24

package rand

import (
	cryptorand "crypto/rand"
)

// PRNG is a cryptographically secure pseudorandom number generator capable of
// generating random bytes and integers.  PRNG methods are not safe for
// concurrent access.
type PRNG struct{}

// NewPRNG returns a seeded PRNG.
func NewPRNG() (*PRNG, error) {
	return new(PRNG), nil
}

// Read fills s with len(s) of cryptographically-secure random bytes.
// Read never errors.
func (*PRNG) Read(s []byte) (n int, err error) {
	return cryptorand.Read(s)
}

// stdlib crypto/rand can be read without extra locking.
type lockingPRNG struct {
	PRNG
}

func init() {
	globalRand = new(lockingPRNG)
}

func (*lockingPRNG) Read(s []byte) (n int, err error) {
	return cryptorand.Read(s)
}

func (*lockingPRNG) Lock()   {}
func (*lockingPRNG) Unlock() {}
