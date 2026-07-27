// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build !amd64 || purego

package secp256k1

// field64MulReduce sets r = a * b (mod p).  This defers to the generic
// implementation that does not rely on the optimized assembly implementations.
func field64MulReduce(r *[4]uint64, a, b *[4]uint64) {
	field64MulReduceGeneric(r, a, b)
}

// field64SquareReduce sets r = a^2 (mod p).  This defers to the generic
// implementation that does not rely on the optimized assembly implementations.
func field64SquareReduce(r *[4]uint64, a *[4]uint64) {
	field64SquareReduceGeneric(r, a)
}
