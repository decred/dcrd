// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"sync"

	"github.com/decred/dcrd/crypto/rand"
)

// csprng provides an interface for the CSPRNG methods the connection manager
// uses.  This primarily exists so tests can replace the real implementation
// with a deterministic PRNG for reproducibility.
type csprng interface {
	Uint64N(n uint64) uint64
}

// lockingPRNG wraps an instance of [rand.PRNG] with a mutex so it can be used
// concurrently.
type lockingPRNG struct {
	prng *rand.PRNG
	sync.Mutex
}

// Uint64 returns a uniform random uint64.
func (p *lockingPRNG) Uint64() uint64 {
	p.Lock()
	defer p.Unlock()

	return p.prng.Uint64()
}

// Uint64N returns a random uint64 in range [0,n) without modulo bias.
func (p *lockingPRNG) Uint64N(n uint64) uint64 {
	p.Lock()
	defer p.Unlock()

	return p.prng.Uint64N(n)
}

// Read fills s with len(s) of cryptographically-secure random bytes.  It never
// errors.
func (p *lockingPRNG) Read(s []byte) {
	p.Lock()
	defer p.Unlock()

	_, _ = p.prng.Read(s)
}

// globalRand is set at init time so any failure to seed, which should never
// happen in practice, will cause a panic at startup instead of runtime.
var globalRand *lockingPRNG

func init() {
	p, err := rand.NewPRNG()
	if err != nil {
		panic(err)
	}
	globalRand = &lockingPRNG{prng: p}
}
