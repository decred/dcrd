// Copyright (c) 2015-2016 The Decred developers
// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chainhash

import (
	"hash"

	"github.com/decred/dcrd/crypto/blake256"
)

// HashFunc calculates the hash of the supplied bytes.
func HashFunc(b []byte) [blake256.Size]byte {
	var outB [blake256.Size]byte
	copy(outB[:], HashB(b))

	return outB
}

// HashB calculates hash(b) and returns the resulting bytes.
func HashB(b []byte) []byte {
	hash := blake256.Sum256(b)
	return hash[:]
}

// HashH calculates hash(b) and returns the resulting bytes as a Hash.
func HashH(b []byte) Hash {
	return Hash(blake256.Sum256(b))
}

// HashBlockSize is the block size of the hash algorithm in bytes.
const HashBlockSize = blake256.BlockSize

// New returns a new hash.Hash computing the hash written to the object.
func New() hash.Hash {
	return blake256.New()
}
