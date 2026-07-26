// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build !amd64 || purego

package secp256k1

// field64Mul sets r = a * b (mod p).
func field64Mul(r *[4]uint64, a, b *[4]uint64) {
	var product [8]uint64
	field64Mul512(&product, a, b)
	field64Reduce512(r, &product)
}

// field64Square sets r = a^2 (mod p).
func field64Square(r *[4]uint64, a *[4]uint64) {
	var product [8]uint64
	field64Square512(&product, a)
	field64Reduce512(r, &product)
}
