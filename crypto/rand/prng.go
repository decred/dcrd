// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build !(openbsd && go1.24)

package rand

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"math/bits"
	"sync"
	"time"

	"golang.org/x/crypto/chacha20"
)

const (
	maxCipherRead     = 4 * 1024 * 1024 // 4 MiB
	maxCipherDuration = 20 * time.Second
)

// nonce implements a 12-byte little endian counter suitable for use as an
// incrementing ChaCha20 nonce.
type nonce [chacha20.NonceSize]byte

func (n *nonce) inc() {
	n0 := binary.LittleEndian.Uint32(n[0:4])
	n1 := binary.LittleEndian.Uint32(n[4:8])
	n2 := binary.LittleEndian.Uint32(n[8:12])

	var carry uint32
	n0, carry = bits.Add32(n0, 1, carry)
	n1, carry = bits.Add32(n1, 0, carry)
	n2, _ = bits.Add32(n2, 0, carry)

	binary.LittleEndian.PutUint32(n[0:4], n0)
	binary.LittleEndian.PutUint32(n[4:8], n1)
	binary.LittleEndian.PutUint32(n[8:12], n2)
}

// PRNG is a cryptographically secure pseudorandom number generator capable of
// generating random bytes and integers.  PRNG methods are not safe for
// concurrent access.
type PRNG struct {
	key    [chacha20.KeySize]byte
	nonce  nonce
	cipher chacha20.Cipher
	read   int
	t      time.Time
}

// NewPRNG returns a seeded PRNG.
func NewPRNG() (*PRNG, error) {
	p := new(PRNG)
	err := p.seed()
	if err != nil {
		return nil, err
	}
	return p, nil
}

// seed reseeds the prng with kernel and existing cipher entropy, if the
// cipher has been originally seeded.
// Only returns an error during initial seeding if a crypto/rand read errors.
func (p *PRNG) seed() error {
	_, err := cryptorand.Read(p.key[:])
	if err != nil && p.t.IsZero() {
		return err
	}
	p.cipher.XORKeyStream(p.key[:], p.key[:])

	// never errors with correct key and nonce sizes
	cipher, _ := chacha20.NewUnauthenticatedCipher(p.key[:], p.nonce[:])
	p.cipher = *cipher
	p.nonce.inc()
	p.read = 0
	p.t = time.Now().Add(maxCipherDuration)
	return nil
}

// Read fills s with len(s) of cryptographically-secure random bytes.
// Read never errors.
func (p *PRNG) Read(s []byte) (n int, err error) {
	if time.Now().After(p.t) {
		// Reseed the cipher.
		// The panic will never be hit except by calling the Read
		// method on the zero PRNG value and if crypto/rand read fails.
		// Creating the PRNG properly with NewPRNG will return nil and an
		// error if the first seeding fails.
		// Later calls to seed will never return an error.
		if err := p.seed(); err != nil {
			panic(err)
		}
	}

	for p.read+len(s) > maxCipherRead {
		l := maxCipherRead - p.read
		p.cipher.XORKeyStream(s[:l], s[:l])
		p.seed()
		n += l
		s = s[l:]
	}
	p.cipher.XORKeyStream(s, s)
	p.read += len(s)
	n += len(s)
	return
}

type lockingPRNG struct {
	*PRNG
	sync.Mutex
}

func init() {
	p, err := NewPRNG()
	if err != nil {
		panic(err)
	}
	globalRand = &lockingPRNG{PRNG: p}
}

func (p *lockingPRNG) Read(s []byte) (n int, err error) {
	p.Lock()
	defer p.Unlock()

	return p.PRNG.Read(s)
}
