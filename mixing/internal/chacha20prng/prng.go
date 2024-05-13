// Copyright (c) 2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chacha20prng

import (
	"encoding/binary"
	"strconv"

	"golang.org/x/crypto/chacha20"
)

// SeedSize is the required length of seeds for New.
const SeedSize = 32

// Reader is a ChaCha20 PRNG for a DC-net run.  It implements io.Reader.
type Reader struct {
	cipher *chacha20.Cipher
}

// New creates a ChaCha20 PRNG seeded by a 32-byte key and a run iteration.  The
// returned reader is not safe for concurrent access.  This will panic if the
// length of seed is not SeedSize bytes.
func New(seed []byte, run uint32) *Reader {
	if l := len(seed); l != SeedSize {
		panic("chacha20prng: bad seed length " + strconv.Itoa(l))
	}

	nonce := make([]byte, chacha20.NonceSize)
	binary.LittleEndian.PutUint32(nonce[:4], run)

	cipher, _ := chacha20.NewUnauthenticatedCipher(seed, nonce)
	return &Reader{cipher: cipher}
}

// Read implements io.Reader.
func (r *Reader) Read(b []byte) (int, error) {
	// Zero the source such that the destination is written with just the
	// keystream.  Destination and source are allowed to overlap (exactly).
	for i := range b {
		b[i] = 0
	}
	r.cipher.XORKeyStream(b, b)
	return len(b), nil
}

// Next returns the next n bytes from the reader.
func (r *Reader) Next(n int) []byte {
	b := make([]byte, n)
	r.cipher.XORKeyStream(b, b)
	return b
}
