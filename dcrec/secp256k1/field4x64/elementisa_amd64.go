// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build !purego

package field4x64

// mulReduce sets r = a * b (mod p) using processor-specific hardware extensions
// when available.
func mulReduce(r *[4]uint64, a, b *[4]uint64) {
	if useBMI2AndADX {
		mulReduceADX(r, a, b)
		return
	}
	mulReduceGeneric(r, a, b)
}

// squareReduce sets r = a^2 (mod p) using processor-specific hardware
// extensions when available.
func squareReduce(r *[4]uint64, a *[4]uint64) {
	if useBMI2AndADX {
		squareReduceADX(r, a)
		return
	}
	squareReduceGeneric(r, a)
}
