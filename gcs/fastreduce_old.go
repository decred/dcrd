// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build !go1.12
// +build !go1.12

package gcs

// mul64 returns the 128-bit product of x and y: (hi, lo) = x * y
// with the product bits' upper half returned in hi and the lower
// half returned in lo.
//
// Copied from go1.12 stdlib.
func mul64(x, y uint64) (hi, lo uint64) {
	const mask32 = 1<<32 - 1
	x0 := x & mask32
	x1 := x >> 32
	y0 := y & mask32
	y1 := y >> 32
	w0 := x0 * y0
	t := x1*y0 + w0>>32
	w1 := t & mask32
	w2 := t >> 32
	w1 += x0 * y1
	hi = x1*y1 + w2 + w1>>32
	lo = x * y
	return
}

// fastReduce calculates a mapping that is more or less equivalent to x mod N.
// However, instead of using a mod operation that can lead to slowness on many
// processors when not using a power of two due to unnecessary division, this
// uses a "multiply-and-shift" trick that eliminates all divisions as described
// in a blog post by Daniel Lemire, located at the following site at the time
// of this writing:
// https://lemire.me/blog/2016/06/27/a-fast-alternative-to-the-modulo-reduction/
//
// Since that link might disappear, the general idea is to multiply by N and
// shift right by log2(N).  Since N is a 64-bit integer in this case, it
// becomes:
//
// (x * N) / 2^64 == (x * N) >> 64
//
// This is a fair map since it maps integers in the range [0,2^64) to multiples
// of N in [0, N*2^64) and then divides by 2^64 to map all multiples of N in
// [0,2^64) to 0, all multiples of N in [2^64, 2*2^64) to 1, etc.  This results
// in either ceil(2^64/N) or floor(2^64/N) multiples of N.
func fastReduce(x, N uint64) uint64 {
	// The high 64 bits in a 128-bit product is the same as shifting the entire
	// product right by 64 bits.
	hi, _ := mul64(x, N)
	return hi
}
