// Copyright (c) 2019-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixing

import (
	"math/big"
)

// F is the field prime 2**127 - 1.
var F *big.Int

func init() {
	F, _ = new(big.Int).SetString("7fffffffffffffffffffffffffffffff", 16)
}

// InField returns whether x is bounded by the field F.
func InField(x *big.Int) bool {
	return x.Sign() != -1 && x.Cmp(F) == -1
}
