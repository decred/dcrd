// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// Uniform random algorithms modified from the Go math/rand/v2 package with
// the following license:
//
// Copyright (c) 2009 The Go Authors. All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//    * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//    * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//    * Neither the name of Google Inc. nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package rand

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"math/bits"
	"time"
)

// Uint32 returns a uniform random uint32.
func (p *PRNG) Uint32() uint32 {
	b := make([]byte, 4)
	p.Read(b)
	return binary.LittleEndian.Uint32(b)
}

// Uint64 returns a uniform random uint64.
func (p *PRNG) Uint64() uint64 {
	b := make([]byte, 8)
	p.Read(b)
	return binary.LittleEndian.Uint64(b)
}

// Uint32N returns a random uint32 in range [0,n) without modulo bias.
func (p *PRNG) Uint32N(n uint32) uint32 {
	if n&(n-1) == 0 { // n is power of two, can mask
		return uint32(p.Uint64()) & (n - 1)
	}
	// On 64-bit systems we still use the uint64 code below because
	// the probability of a random uint64 lo being < a uint32 n is near zero,
	// meaning the unbiasing loop almost never runs.
	// On 32-bit systems, here we need to implement that same logic in 32-bit math,
	// both to preserve the exact output sequence observed on 64-bit machines
	// and to preserve the optimization that the unbiasing loop almost never runs.
	//
	// We want to compute
	// 	hi, lo := bits.Mul64(r.Uint64(), n)
	// In terms of 32-bit halves, this is:
	// 	x1:x0 := r.Uint64()
	// 	0:hi, lo1:lo0 := bits.Mul64(x1:x0, 0:n)
	// Writing out the multiplication in terms of bits.Mul32 allows
	// using direct hardware instructions and avoiding
	// the computations involving these zeros.
	x := p.Uint64()
	lo1a, lo0 := bits.Mul32(uint32(x), n)
	hi, lo1b := bits.Mul32(uint32(x>>32), n)
	lo1, c := bits.Add32(lo1a, lo1b, 0)
	hi += c
	if lo1 == 0 && lo0 < n {
		n64 := uint64(n)
		thresh := uint32(-n64 % n64)
		for lo1 == 0 && lo0 < thresh {
			x := p.Uint64()
			lo1a, lo0 = bits.Mul32(uint32(x), n)
			hi, lo1b = bits.Mul32(uint32(x>>32), n)
			lo1, c = bits.Add32(lo1a, lo1b, 0)
			hi += c
		}
	}
	return hi
}

const is32bit = ^uint(0)>>32 == 0

// Uint64N returns a random uint32 in range [0,n) without modulo bias.
func (p *PRNG) Uint64N(n uint64) uint64 {
	if is32bit && uint64(uint32(n)) == n {
		return uint64(p.Uint32N(uint32(n)))
	}
	if n&(n-1) == 0 { // n is power of two, can mask
		return p.Uint64() & (n - 1)
	}

	// Suppose we have a uint64 x uniform in the range [0,2⁶⁴)
	// and want to reduce it to the range [0,n) preserving exact uniformity.
	// We can simulate a scaling arbitrary precision x * (n/2⁶⁴) by
	// the high bits of a double-width multiply of x*n, meaning (x*n)/2⁶⁴.
	// Since there are 2⁶⁴ possible inputs x and only n possible outputs,
	// the output is necessarily biased if n does not divide 2⁶⁴.
	// In general (x*n)/2⁶⁴ = k for x*n in [k*2⁶⁴,(k+1)*2⁶⁴).
	// There are either floor(2⁶⁴/n) or ceil(2⁶⁴/n) possible products
	// in that range, depending on k.
	// But suppose we reject the sample and try again when
	// x*n is in [k*2⁶⁴, k*2⁶⁴+(2⁶⁴%n)), meaning rejecting fewer than n possible
	// outcomes out of the 2⁶⁴.
	// Now there are exactly floor(2⁶⁴/n) possible ways to produce
	// each output value k, so we've restored uniformity.
	// To get valid uint64 math, 2⁶⁴ % n = (2⁶⁴ - n) % n = -n % n,
	// so the direct implementation of this algorithm would be:
	//
	//	hi, lo := bits.Mul64(r.Uint64(), n)
	//	thresh := -n % n
	//	for lo < thresh {
	//		hi, lo = bits.Mul64(r.Uint64(), n)
	//	}
	//
	// That still leaves an expensive 64-bit division that we would rather avoid.
	// We know that thresh < n, and n is usually much less than 2⁶⁴, so we can
	// avoid the last four lines unless lo < n.
	//
	// See also:
	// https://lemire.me/blog/2016/06/27/a-fast-alternative-to-the-modulo-reduction
	// https://lemire.me/blog/2016/06/30/fast-random-shuffling
	hi, lo := bits.Mul64(p.Uint64(), n)
	if lo < n {
		thresh := -n % n
		for lo < thresh {
			hi, lo = bits.Mul64(p.Uint64(), n)
		}
	}
	return hi
}

// Int32 returns a random 31-bit non-negative integer as an int32 without
// modulo bias.
func (p *PRNG) Int32() int32 {
	return int32(p.Uint32() & 0x7FFFFFFF)
}

// Int32N returns, as an int32, a random 31-bit non-negative integer in [0,n)
// without modulo bias.
// Panics if n <= 0.
func (p *PRNG) Int32N(n int32) int32 {
	if n <= 0 {
		panic("rand: invalid argument to Int32N")
	}
	return int32(p.Uint32N(uint32(n)))
}

// Int64 returns a random 63-bit non-negative integer as an int64 without
// modulo bias.
func (p *PRNG) Int64() int64 {
	return int64(p.Uint64() & 0x7FFFFFFF_FFFFFFFF)
}

// Int64N returns, as an int64, a random 63-bit non-negative integer in [0,n)
// without modulo bias.
// Panics if n <= 0.
func (p *PRNG) Int64N(n int64) int64 {
	if n <= 0 {
		panic("rand: invalid argument to Int64N")
	}
	return int64(p.Uint64N(uint64(n)))
}

// Int returns a non-negative integer without bias.
func (p *PRNG) Int() int {
	return int(uint(p.Uint64()) << 1 >> 1)
}

// IntN returns, as an int, a random non-negative integer in [0,n) without
// modulo bias.
// Panics if n <= 0.
func (p *PRNG) IntN(n int) int {
	if n <= 0 {
		panic("rand: invalid argument to IntN")
	}
	return int(p.Uint64N(uint64(n)))
}

// UintN returns, as an uint, a random integer in [0,n) without modulo bias.
func (p *PRNG) UintN(n uint) uint {
	return uint(p.Uint64N(uint64(n)))
}

// Duration returns a random duration in [0,n) without modulo bias.
// Panics if n <= 0.
func (p *PRNG) Duration(n time.Duration) time.Duration {
	if n <= 0 {
		panic("rand: invalid argument to Duration")
	}
	return time.Duration(p.Uint64N(uint64(n)))
}

// Shuffle randomizes the order of n elements by swapping the elements at
// indexes i and j.
// Panics if n < 0.
func (p *PRNG) Shuffle(n int, swap func(i, j int)) {
	if n < 0 {
		panic("rand: invalid argument to Shuffle")
	}

	// Fisher-Yates shuffle: https://en.wikipedia.org/wiki/Fisher%E2%80%93Yates_shuffle
	// Shuffle really ought not be called with n that doesn't fit in 32 bits.
	// Not only will it take a very long time, but with 2³¹! possible permutations,
	// there's no way that any PRNG can have a big enough internal state to
	// generate even a minuscule percentage of the possible permutations.
	// Nevertheless, the right API signature accepts an int n, so handle it as best we can.
	for i := n - 1; i > 0; i-- {
		j := int(p.Uint64N(uint64(i + 1)))
		swap(i, j)
	}
}

// Int returns a uniform random value in [0,max).
// Panics if max <= 0.
func (p *PRNG) BigInt(max *big.Int) *big.Int {
	// Will never error with our reader.
	n, _ := rand.Int(p, max)
	return n
}
