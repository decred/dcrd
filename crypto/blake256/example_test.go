// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// Examples originally written by Dave Collins July 2024.

package blake256_test

import (
	"bytes"
	"fmt"
	"sort"

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

// This example demonstrates creating a rolling BLAKE-256 hasher, writing some
// data to it, making a copy of the intermediate state, restoring the
// intermediate state in multiple goroutines, writing more data to each of those
// restored copies, and computing the final hashes.
func Example_sameProcessSaveRestore() {
	// Create 1024 bytes of mock shared prefix data.  This is meant to simulate
	// what would ordinarily be actual data at the beginning that does not
	// change between multiple hash invocations.
	sharedPrefixData := bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 256)

	// Create a new zero-allocation hasher for BLAKE-256 and write the mock
	// shared prefix data to it.
	hasher := blake256.NewHasher256()
	hasher.WriteBytes(sharedPrefixData)

	// Make a copy of the current intermediate hasher state.
	savedHasher := *hasher

	// Launch some goroutines that each restore the saved intermediate state,
	// write different suffix data, compute the final hash, and deliver the
	// resulting hashes on a channel.
	//
	// Note that the key point this is attempting to demonstrate is that all of
	// the shared prefix data does not need to be rewritten.
	const numResults = 4
	results := make(chan [blake256.Size]byte, numResults)
	for i := 0; i < numResults; i++ {
		go func(moreData uint16) {
			restoredHasher := savedHasher
			restoredHasher.WriteUint16BE(moreData)
			results <- restoredHasher.Sum256()
		}(uint16(i))
	}

	// Collect the resulting hashes from the goroutines.
	hashes := make([][blake256.Size]byte, numResults)
	for i := 0; i < numResults; i++ {
		hashes[i] = <-results
	}

	// Sort the results for stable test output.  This typically wouldn't be
	// needed in a real application.
	sort.Slice(hashes, func(i, j int) bool {
		return bytes.Compare(hashes[i][:], hashes[j][:]) < 0
	})
	for i, hash := range hashes {
		fmt.Printf("hash%d: %x\n", i+1, hash)
	}

	// Output:
	// hash1: 290278508c9ba1b7717338bbde28fa91e36aa2c39c7eb63bd96ba6abd5ebff21
	// hash2: 2c44931aacea282297d9c153bc0cd9f48d13796d16f634cad672504518e36af2
	// hash3: 77c9e453e13c923b540bda1a61dddab55de4c7b624cd499f2990e9de0345bacc
	// hash4: 8c61d57f5be4ce7b12c577c4d8e51127766b331a1ff54220e5ca1c7ea9c0360a
}
