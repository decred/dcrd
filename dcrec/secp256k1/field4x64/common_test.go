// Copyright (c) 2026 The Decred developers
// Copyright (c) 2013-2026 Dave Collins
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package field4x64

// References:
//   [SECG]: Recommended Elliptic Curve Domain Parameters
//     https://www.secg.org/sec2-v2.pdf
//

import (
	"encoding/hex"
	"math/big"
)

// Curve parameters taken from [SECG] section 2.4.1.
var curveParams = struct {
	P *big.Int
	N *big.Int
}{
	P: fromHex("fffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc2f"),
	N: fromHex("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141"),
}

// hexToBytes converts the passed hex string into bytes and will panic if there
// is an error.  This is only provided for the hard-coded constants so errors in
// the source code can be detected. It will only (and must only) be called with
// hard-coded values.
func hexToBytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	return b
}

// fromHex converts the passed hex string into a big integer pointer and will
// panic is there is an error.  This is only provided for the hard-coded
// constants so errors in the source code can bet detected. It will only (and
// must only) be called for initialization purposes.
func fromHex(s string) *big.Int {
	if s == "" {
		return big.NewInt(0)
	}
	r, ok := new(big.Int).SetString(s, 16)
	if !ok {
		panic("invalid hex in source file: " + s)
	}
	return r
}
