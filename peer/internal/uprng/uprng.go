// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package uprng

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"io"
	"math/bits"
	"sync"
	"time"

	"golang.org/x/crypto/chacha20"
)

// Reader implements a cryptographically secure userspace PRNG that is
// periodically reseeded with entropy obtained from crypto/rand.
// Reader is safe for concurrent access.
var Reader io.Reader

// Read fills b with random bytes obtained from the userspace PRNG.
//
// When possible, prefer Read over using Reader to avoid escape analysis
// unnecessarily allocating b's backing array on the heap.
func Read(b []byte) {
	defaultReader.Read(b)
}

var defaultReader = newPRNG()

func init() {
	Reader = defaultReader
}

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

type prng struct {
	cipher *chacha20.Cipher
	read   int
	t      time.Time
	key    []byte
	nonce  nonce
	mu     sync.Mutex
}

func newPRNG() *prng {
	p := &prng{
		key: make([]byte, chacha20.KeySize),
	}
	p.seed()
	return p
}

// seed reseeds the prng with kernel and existing cipher entropy, if the
// cipher has been originally seeded.
// Panics only during initial seeding if a crypto/rand read errors.
func (p *prng) seed() {
	_, err := cryptorand.Read(p.key)
	if err != nil && p.cipher == nil {
		panic(err)
	}
	if p.cipher != nil {
		p.cipher.XORKeyStream(p.key, p.key)
	}

	// never errors with correct key and nonce sizes
	cipher, _ := chacha20.NewUnauthenticatedCipher(p.key, p.nonce[:])
	p.nonce.inc()

	p.cipher = cipher
	p.read = 0
	p.t = time.Now().Add(maxCipherDuration)
}

func (p *prng) Read(s []byte) (n int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if time.Now().After(p.t) {
		p.seed()
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
