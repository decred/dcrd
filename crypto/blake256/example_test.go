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
