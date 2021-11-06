// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package uint256_test

import (
	"fmt"

	"github.com/decred/dcrd/internal/staging/primitives/uint256"
)

// This example demonstrates calculating the result of dividing a max unsigned
// 256-bit integer by a max unsigned 128-bit integer and outputting that result
// in hex with leading zeros.
func Example_basicUsage() {
	// Calculate maxUint256 / maxUint128 and output it in hex with leading zeros.
	maxUint128 := new(uint256.Uint256).SetUint64(1).Lsh(128).SubUint64(1)
	result := new(uint256.Uint256).Not().Div(maxUint128)
	fmt.Printf("result: %064x\n", result)

	// Output:
	// result: 0000000000000000000000000000000100000000000000000000000000000001
}
