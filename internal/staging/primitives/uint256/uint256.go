// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package uint256 implements highly optimized fixed precision unsigned 256-bit
// integer arithmetic.
package uint256

// Uint256 implements high-performance, zero-allocation, unsigned 256-bit
// fixed-precision arithmetic.  All operations are performed modulo 2^256, so
// callers may rely on "wrap around" semantics.
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
