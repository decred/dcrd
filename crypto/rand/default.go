// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rand

import (
	"io"
	"sync"
	"time"
)

// Reader returns the default cryptographically secure userspace PRNG that is
// periodically reseeded with entropy obtained from crypto/rand.
// The returned Reader is safe for concurrent access.
func Reader() io.Reader {
	return globalRand
}

type lockingPRNG struct {
	*PRNG
	mu sync.Mutex
}

var globalRand *lockingPRNG

func init() {
	p, err := NewPRNG()
	if err != nil {
		panic(err)
	}
	globalRand = &lockingPRNG{PRNG: p}
}

func (p *lockingPRNG) Read(s []byte) (n int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.PRNG.Read(s)
}

// Read fills b with random bytes obtained from the default userspace PRNG.
func Read(b []byte) {
	// Mutex is acquired by (*lockingPRNG).Read.
	globalRand.Read(b)
}

// Uint32 returns a uniform random uint32.
func Uint32() uint32 {
	globalRand.mu.Lock()
	defer globalRand.mu.Unlock()

	return globalRand.Uint32()
}

// Uint64 returns a uniform random uint64.
func Uint64() uint64 {
	globalRand.mu.Lock()
	defer globalRand.mu.Unlock()

	return globalRand.Uint64()
}

// Uint32n returns a random uint32 in range [0,n) without modulo bias.
func Uint32n(n uint32) uint32 {
	globalRand.mu.Lock()
	defer globalRand.mu.Unlock()

	return globalRand.Uint32n(n)
}

// Uint64n returns a random uint32 in range [0,n) without modulo bias.
func Uint64n(n uint64) uint64 {
	globalRand.mu.Lock()
	defer globalRand.mu.Unlock()

	return globalRand.Uint64n(n)
}

// Int32 returns a random 31-bit non-negative integer as an int32 without
// modulo bias.
func Int32() int32 {
	globalRand.mu.Lock()
	defer globalRand.mu.Unlock()

	return globalRand.Int32()
}

// Int32n returns, as an int32, a random 31-bit non-negative integer in [0,n)
// without modulo bias.
// Panics if n <= 0.
func Int32n(n int32) int32 {
	globalRand.mu.Lock()
	defer globalRand.mu.Unlock()

	return globalRand.Int32n(n)
}

// Int64 returns a random 63-bit non-negative integer as an int64 without
// modulo bias.
func Int64() int64 {
	globalRand.mu.Lock()
	defer globalRand.mu.Unlock()

	return globalRand.Int64()
}

// Int64n returns, as an int64, a random 63-bit non-negative integer in [0,n)
// without modulo bias.
// Panics if n <= 0.
func Int64n(n int64) int64 {
	globalRand.mu.Lock()
	defer globalRand.mu.Unlock()

	return globalRand.Int64n(n)
}

// Int returns a non-negative integer without bias.
func Int() int {
	globalRand.mu.Lock()
	defer globalRand.mu.Unlock()

	return globalRand.Int()
}

// IntN returns, as an int, a random non-negative integer in [0,n) without
// modulo bias.
// Panics if n <= 0.
func IntN(n int) int {
	globalRand.mu.Lock()
	defer globalRand.mu.Unlock()

	return globalRand.IntN(n)
}

// UintN returns, as an uint, a random integer in [0,n) without modulo bias.
func UintN(n uint) uint {
	globalRand.mu.Lock()
	defer globalRand.mu.Unlock()

	return globalRand.UintN(n)
}

// Duration returns a random duration in [0,n) without modulo bias.
// Panics if n <= 0.
func Duration(n time.Duration) time.Duration {
	globalRand.mu.Lock()
	defer globalRand.mu.Unlock()

	return globalRand.Duration(n)
}

// Shuffle randomizes the order of n elements by swapping the elements at
// indexes i and j.
// Panics if n < 0.
func Shuffle(n int, swap func(i, j int)) {
	globalRand.mu.Lock()
	defer globalRand.mu.Unlock()

	globalRand.Shuffle(n, swap)
}
