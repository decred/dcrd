// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build !amd64 || purego

package field4x64

// useBMI2AndADX depends on hardware support and access to assembly
// instructions.
var useBMI2AndADX = false

// mulReduce sets r = a * b (mod p).  This defers to the generic
// implementation that does not rely on the optimized assembly implementations.
func mulReduce(r *[4]uint64, a, b *[4]uint64) {
	mulReduceGeneric(r, a, b)
}

// squareReduce sets r = a^2 (mod p).  This defers to the generic implementation
// that does not rely on the optimized assembly implementations.
func squareReduce(r *[4]uint64, a *[4]uint64) {
	squareReduceGeneric(r, a)
}
