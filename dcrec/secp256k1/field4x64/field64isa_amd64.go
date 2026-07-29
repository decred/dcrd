// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build !purego

package field4x64

// field64MulReduce sets r = a * b (mod p) using processor-specific hardware
// extensions when available.
func field64MulReduce(r *[4]uint64, a, b *[4]uint64) {
	if field64UseBMI2AndADX {
		field64MulReduceADX(r, a, b)
		return
	}
	field64MulReduceGeneric(r, a, b)
}

// field64SquareReduce sets r = a^2 (mod p) using processor-specific hardware
// extensions when available.
func field64SquareReduce(r *[4]uint64, a *[4]uint64) {
	if field64UseBMI2AndADX {
		field64SquareReduceADX(r, a)
		return
	}
	field64SquareReduceGeneric(r, a)
}
