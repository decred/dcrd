// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// Examples originally written by Dave Collins July 2024.

package blake256_test

import (
	"fmt"

	"github.com/decred/dcrd/crypto/blake256"
)

// This example demonstrates the simplest method of hashing an existing
// serialized data buffer with BLAKE-256.
func Example_basicUsageExistingBuffer() {
	// The data to hash in this scenario would ordinarily come from somewhere
	// else, but it is hard coded here for the purposes of the example.
	data := []byte{0x01, 0x02, 0x03, 0x04}
	hash := blake256.Sum256(data)
	fmt.Printf("hash: %x\n", hash)

	// Output:
	// hash: c301ba9de5d6053caad9f5eb46523f007702add2c62fa39de03146a36b8026b7
}

// This example demonstrates creating a rolling BLAKE-256 hasher, writing
// various data types to it, computing the hash, writing more data, and
// finally computing the cumulative hash.
func Example_rollingHasherUsage() {
	// Create a new zero-allocation hasher for BLAKE-256 and write data with
	// various types to it.
	hasher := blake256.NewHasher256()
	hasher.WriteString("Some string to include in the hash")
	hasher.WriteBytes([]byte{0x01, 0x02, 0x03, 0x04})
	hasher.WriteByte(0x05)
	hasher.WriteUint16BE(6789)       // Big endian
	hasher.WriteUint32LE(268435455)  // Little endian
	hasher.WriteUint64BE(4294967297) // Big endian

	// Compute the hash of the data written to this point.  Note that the hasher
	// state is not modified, so it is possible to continue writing more data
	// afterwards as demonstrated next.
	hash := hasher.Sum256()
	fmt.Printf("hash1: %x\n", hash)

	// Write some more data and compute the cumulative hash.
	hasher.WriteString("more data to include in the hash")
	fmt.Printf("hash2: %x\n", hasher.Sum256())

	// Output:
	// hash1: 3bd1db9a41e8a7942a14d30eb196f1420a3991a1094bdbf678d3b4ded30dda6f
	// hash2: 36771288c89b3cda5659e270338127b76361e1a07bfe26bb2baae14d46e56c64
}
