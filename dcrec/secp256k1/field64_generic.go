// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build !amd64 || purego

package secp256k1

import "github.com/decred/dcrd/dcrec/secp256k1/v4/internal/arith"

// field64MulReduce sets r = a * b (mod p).
func field64MulReduce(r *[4]uint64, a, b *[4]uint64) {
	var product [8]uint64
	arith.Mul512(&product, a, b)
	field64Reduce512(r, &product)
}

// field64SquareReduce sets r = a^2 (mod p).
func field64SquareReduce(r *[4]uint64, a *[4]uint64) {
	var product [8]uint64
	arith.Square512(&product, a)
	field64Reduce512(r, &product)
}
