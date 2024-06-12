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

// Package uniform provides uniformly distributed, cryptographically secure
// random numbers with randomness obtained from a CSPRNG.
//
// Random sources are required to never error; any errors reading the random
// source will result in a panic.
package uniform

import (
	"encoding/binary"
	"math/bits"
	"time"

	"github.com/decred/dcrd/peer/v3/internal/uprng"
)

func readRand(buf []byte) {
	uprng.Read(buf)
}

// Uint32 returns a uniform random uint32.
func Uint32() uint32 {
	b := make([]byte, 4)
	readRand(b)
	return binary.LittleEndian.Uint32(b)
}

// Uint64 returns a uniform random uint64.
func Uint64() uint64 {
	b := make([]byte, 8)
	readRand(b)
	return binary.LittleEndian.Uint64(b)
}

// Uint32n returns a random uint32 in range [0,n) without modulo bias.
func Uint32n(n uint32) uint32 {
	if n&(n-1) == 0 { // n is power of two, can mask
		return uint32(Uint64()) & (n - 1)
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
	x := Uint64()
	lo1a, lo0 := bits.Mul32(uint32(x), n)
	hi, lo1b := bits.Mul32(uint32(x>>32), n)
	lo1, c := bits.Add32(lo1a, lo1b, 0)
	hi += c
	if lo1 == 0 && lo0 < n {
		n64 := uint64(n)
		thresh := uint32(-n64 % n64)
		for lo1 == 0 && lo0 < thresh {
			x := Uint64()
			lo1a, lo0 = bits.Mul32(uint32(x), n)
			hi, lo1b = bits.Mul32(uint32(x>>32), n)
			lo1, c = bits.Add32(lo1a, lo1b, 0)
			hi += c
		}
	}
	return hi
}

const is32bit = ^uint(0)>>32 == 0

// Uint64n returns a random uint32 in range [0,n) without modulo bias.
func Uint64n(n uint64) uint64 {
	if is32bit && uint64(uint32(n)) == n {
		return uint64(Uint32n(uint32(n)))
	}
	if n&(n-1) == 0 { // n is power of two, can mask
		return Uint64() & (n - 1)
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
	hi, lo := bits.Mul64(Uint64(), n)
	if lo < n {
		thresh := -n % n
		for lo < thresh {
			hi, lo = bits.Mul64(Uint64(), n)
		}
	}
	return hi
}

// Int32 returns a random 31-bit non-negative integer as an int32 without
// modulo bias.
func Int32() int32 {
	return int32(Uint32() & 0x7FFFFFFF)
}

// Int32n returns, as an int32, a random 31-bit non-negative integer in [0,n)
// without modulo bias.
// Panics if n <= 0.
func Int32n(n int32) int32 {
	if n <= 0 {
		panic("uniform: invalid argument to Int32n")
	}
	return int32(Uint32n(uint32(n)))
}

// Int64 returns a random 63-bit non-negative integer as an int64 without
// modulo bias.
func Int64() int64 {
	return int64(Uint64() & 0x7FFFFFFF_FFFFFFFF)
}

// Int64n returns, as an int64, a random 63-bit non-negative integer in [0,n)
// without modulo bias.
// Panics if n <= 0.
func Int64n(n int64) int64 {
	if n <= 0 {
		panic("uniform: invalid argument to Int64n")
	}
	return int64(Uint64n(uint64(n)))
}

// Duration returns a random duration in [0,n) without modulo bias.
// Panics if n <= 0.
func Duration(n time.Duration) time.Duration {
	if n <= 0 {
		panic("uniform: invalid argument to Duration")
	}
	return time.Duration(Uint64n(uint64(n)))
}
