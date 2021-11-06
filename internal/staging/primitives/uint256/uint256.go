// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package uint256 implements highly optimized fixed precision unsigned 256-bit
// integer arithmetic.
package uint256

var (
	// zero32 is an array of 32 bytes used for the purposes of zeroing and is
	// defined here to avoid extra allocations.
	zero32 = [32]byte{}
)

// Uint256 implements high-performance, zero-allocation, unsigned 256-bit
// fixed-precision arithmetic.  All operations are performed modulo 2^256, so
// callers may rely on "wrap around" semantics.
//
// It currently implements support for interpreting big endian bytes.
//
// Future commits will implement support for interpreting and producing big and
// little endian bytes, the primary arithmetic operations (addition,
// subtraction, multiplication, squaring, division, negation), bitwise
// operations (lsh, rsh, not, or, and, xor), comparison operations (equals,
// less, greater, cmp), and other convenience methods such as determining the
// minimum number of bits required to represent the current value, whether or
// not the value can be represented as a uint64 without loss of precision, and
// text formatting with base conversion.
type Uint256 struct {
	// The uint256 is represented as 4 unsigned 64-bit integers in base 2^64.
	//
	// The following depicts the internal representation:
	//
	//  --------------------------------------------------------------------
	// |      n[3]      |      n[2]      |      n[1]      |      n[0]      |
	// | 64 bits        | 64 bits        | 64 bits        | 64 bits        |
	// | Mult: 2^(64*3) | Mult: 2^(64*2) | Mult: 2^(64*1) | Mult: 2^(64*0) |
	//  -------------------------------------------------------------------
	//
	// For example, consider the number:
	//  0x0000000000000000080000000000000000000000000001000000000000000001 =
	//  2^187 + 2^72 + 1
	//
	// It would be represented as:
	//  n[0] = 1
	//  n[1] = 2^8
	//  n[2] = 2^59
	//  n[3] = 0
	//
	// The full 256-bit value is then calculated by looping i from 3..0 and
	// doing sum(n[i] * 2^(64i)) as follows:
	//  n[3] * 2^(64*3) = 0    * 2^192 = 0
	//  n[2] * 2^(64*2) = 2^59 * 2^128 = 2^187
	//  n[1] * 2^(64*1) = 2^8  * 2^64  = 2^72
	//  n[0] * 2^(64*0) = 1    * 2^0   = 1
	//  Sum: 0 + 2^187 + 2^72 + 1 = 2^187 + 2^72 + 1
	n [4]uint64
}

// Set sets the uint256 equal to the same value as the passed one.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n := new(Uint256).Set(n2).AddUint64(1) so that n = n2 + 1 where n2 is not
// modified.
func (n *Uint256) Set(n2 *Uint256) *Uint256 {
	*n = *n2
	return n
}

// SetUint64 sets the uint256 to the passed unsigned 64-bit integer.  This is a
// convenience function since it is fairly common to perform arithmetic with
// small native integers.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n := new(Uint256).SetUint64(2).Mul(n2) so that n = 2 * n2.
func (n *Uint256) SetUint64(n2 uint64) *Uint256 {
	n.n[0] = n2
	n.n[1] = 0
	n.n[2] = 0
	n.n[3] = 0
	return n
}

// SetBytes interprets the provided array as a 256-bit big-endian unsigned
// integer and sets the uint256 to the result.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n := new(Uint256).SetBytes(n2Bytes).AddUint64(1) so that n = n2 + 1.
func (n *Uint256) SetBytes(b *[32]byte) *Uint256 {
	// Pack the 256 total bits across the 4 uint64 words.  This could be done
	// with a for loop, but benchmarks show this unrolled version is about 2
	// times faster than the variant that uses a loop.
	n.n[0] = uint64(b[31]) | uint64(b[30])<<8 | uint64(b[29])<<16 |
		uint64(b[28])<<24 | uint64(b[27])<<32 | uint64(b[26])<<40 |
		uint64(b[25])<<48 | uint64(b[24])<<56
	n.n[1] = uint64(b[23]) | uint64(b[22])<<8 | uint64(b[21])<<16 |
		uint64(b[20])<<24 | uint64(b[19])<<32 | uint64(b[18])<<40 |
		uint64(b[17])<<48 | uint64(b[16])<<56
	n.n[2] = uint64(b[15]) | uint64(b[14])<<8 | uint64(b[13])<<16 |
		uint64(b[12])<<24 | uint64(b[11])<<32 | uint64(b[10])<<40 |
		uint64(b[9])<<48 | uint64(b[8])<<56
	n.n[3] = uint64(b[7]) | uint64(b[6])<<8 | uint64(b[5])<<16 |
		uint64(b[4])<<24 | uint64(b[3])<<32 | uint64(b[2])<<40 |
		uint64(b[1])<<48 | uint64(b[0])<<56
	return n
}

// zeroArray32 zeroes the provided 32-byte buffer.
func zeroArray32(b *[32]byte) {
	copy(b[:], zero32[:])
}

// minInt is a helper function to return the minimum of two ints.
// This avoids a math import and the need to cast to floats.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SetByteSlice interprets the provided slice as a 256-bit big-endian unsigned
// integer (meaning it is truncated to the final 32 bytes so that it is modulo
// 2^256), and sets the uint256 to the result.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n := new(Uint256).SetByteSlice(n2Slice).AddUint64(1) so that n = n2 + 1.
func (n *Uint256) SetByteSlice(b []byte) *Uint256 {
	var b32 [32]byte
	b = b[len(b)-minInt(len(b), 32):]
	copy(b32[32-len(b):], b)
	n.SetBytes(&b32)
	zeroArray32(&b32)
	return n
}
