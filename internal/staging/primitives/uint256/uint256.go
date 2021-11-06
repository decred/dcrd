// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package uint256 implements highly optimized fixed precision unsigned 256-bit
// integer arithmetic.
package uint256

import (
	"bytes"
	"fmt"
	"math/bits"
)

// References:
//   [TAOCP2]: The Art of Computer Programming, Volume 2.
//             Seminumerical Algorithms (Knuth, Donald E.)

var (
	// zero32 is an array of 32 bytes used for the purposes of zeroing and is
	// defined here to avoid extra allocations.
	zero32 = [32]byte{}
)

// Uint256 implements high-performance, zero-allocation, unsigned 256-bit
// fixed-precision arithmetic.  All operations are performed modulo 2^256, so
// callers may rely on "wrap around" semantics.
//
// It implements the primary arithmetic operations (addition, subtraction,
// multiplication, squaring, division, negation), bitwise operations (lsh, rsh,
// not, or, and, xor), comparison operations (equals, less, greater, cmp),
// interpreting and producing big and little endian bytes, and other convenience
// methods such as determining the minimum number of bits required to represent
// the current value, whether or not the value can be represented as a uint64
// without loss of precision, and text formatting with base conversion.
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

// SetBytesLE interprets the provided array as a 256-bit little-endian unsigned
// integer and sets the uint256 to the result.
func (n *Uint256) SetBytesLE(b *[32]byte) *Uint256 {
	// Pack the 256 total bits across the 4 uint64 words.  This could be done
	// with a for loop, but benchmarks show this unrolled version is about 2
	// times faster than the variant that uses a loop.
	n.n[0] = uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 |
		uint64(b[3])<<24 | uint64(b[4])<<32 | uint64(b[5])<<40 |
		uint64(b[6])<<48 | uint64(b[7])<<56
	n.n[1] = uint64(b[8]) | uint64(b[9])<<8 | uint64(b[10])<<16 |
		uint64(b[11])<<24 | uint64(b[12])<<32 | uint64(b[13])<<40 |
		uint64(b[14])<<48 | uint64(b[15])<<56
	n.n[2] = uint64(b[16]) | uint64(b[17])<<8 | uint64(b[18])<<16 |
		uint64(b[19])<<24 | uint64(b[20])<<32 | uint64(b[21])<<40 |
		uint64(b[22])<<48 | uint64(b[23])<<56
	n.n[3] = uint64(b[24]) | uint64(b[25])<<8 | uint64(b[26])<<16 |
		uint64(b[27])<<24 | uint64(b[28])<<32 | uint64(b[29])<<40 |
		uint64(b[30])<<48 | uint64(b[31])<<56
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

// SetByteSliceLE interprets the provided slice as a 256-bit little-endian
// unsigned integer (meaning it is truncated to the first 32 bytes so that it is
// modulo 2^256), and sets the uint256 to the result.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n := new(Uint256).SetByteSliceLE(n2Slice).AddUint64(1) so that n = n2 + 1.
func (n *Uint256) SetByteSliceLE(b []byte) *Uint256 {
	var b32 [32]byte
	b = b[:minInt(len(b), 32)]
	copy(b32[:], b)
	n.SetBytesLE(&b32)
	zeroArray32(&b32)
	return n
}

// PutBytesUnchecked unpacks the uint256 to a 32-byte big-endian value directly
// into the passed byte slice.  The target slice must must have at least 32
// bytes available or it will panic.
//
// There is a similar function, PutBytes, which unpacks the uint256 into a
// 32-byte array directly.  This version is provided since it can be useful to
// write directly into part of a larger buffer without needing a separate
// allocation.
func (n *Uint256) PutBytesUnchecked(b []byte) {
	// Unpack the 256 total bits from the 4 uint64 words.  This could be done
	// with a for loop, but benchmarks show this unrolled version is about 2
	// times faster than the variant which uses a loop.
	b[31] = byte(n.n[0])
	b[30] = byte(n.n[0] >> 8)
	b[29] = byte(n.n[0] >> 16)
	b[28] = byte(n.n[0] >> 24)
	b[27] = byte(n.n[0] >> 32)
	b[26] = byte(n.n[0] >> 40)
	b[25] = byte(n.n[0] >> 48)
	b[24] = byte(n.n[0] >> 56)
	b[23] = byte(n.n[1])
	b[22] = byte(n.n[1] >> 8)
	b[21] = byte(n.n[1] >> 16)
	b[20] = byte(n.n[1] >> 24)
	b[19] = byte(n.n[1] >> 32)
	b[18] = byte(n.n[1] >> 40)
	b[17] = byte(n.n[1] >> 48)
	b[16] = byte(n.n[1] >> 56)
	b[15] = byte(n.n[2])
	b[14] = byte(n.n[2] >> 8)
	b[13] = byte(n.n[2] >> 16)
	b[12] = byte(n.n[2] >> 24)
	b[11] = byte(n.n[2] >> 32)
	b[10] = byte(n.n[2] >> 40)
	b[9] = byte(n.n[2] >> 48)
	b[8] = byte(n.n[2] >> 56)
	b[7] = byte(n.n[3])
	b[6] = byte(n.n[3] >> 8)
	b[5] = byte(n.n[3] >> 16)
	b[4] = byte(n.n[3] >> 24)
	b[3] = byte(n.n[3] >> 32)
	b[2] = byte(n.n[3] >> 40)
	b[1] = byte(n.n[3] >> 48)
	b[0] = byte(n.n[3] >> 56)
}

// PutBytesUncheckedLE unpacks the uint256 to a 32-byte little-endian value
// directly into the passed byte slice.  The target slice must must have at
// least 32 bytes available or it will panic.
//
// There is a similar function, PutBytesLE, which unpacks the uint256 into a
// 32-byte array directly.  This version is provided since it can be useful to
// write directly into part of a larger buffer without needing a separate
// allocation.
func (n *Uint256) PutBytesUncheckedLE(b []byte) {
	// Unpack the 256 total bits from the 4 uint64 words.  This could be done
	// with a for loop, but benchmarks show this unrolled version is about 2
	// times faster than the variant which uses a loop.
	b[31] = byte(n.n[3] >> 56)
	b[30] = byte(n.n[3] >> 48)
	b[29] = byte(n.n[3] >> 40)
	b[28] = byte(n.n[3] >> 32)
	b[27] = byte(n.n[3] >> 24)
	b[26] = byte(n.n[3] >> 16)
	b[25] = byte(n.n[3] >> 8)
	b[24] = byte(n.n[3])
	b[23] = byte(n.n[2] >> 56)
	b[22] = byte(n.n[2] >> 48)
	b[21] = byte(n.n[2] >> 40)
	b[20] = byte(n.n[2] >> 32)
	b[19] = byte(n.n[2] >> 24)
	b[18] = byte(n.n[2] >> 16)
	b[17] = byte(n.n[2] >> 8)
	b[16] = byte(n.n[2])
	b[15] = byte(n.n[1] >> 56)
	b[14] = byte(n.n[1] >> 48)
	b[13] = byte(n.n[1] >> 40)
	b[12] = byte(n.n[1] >> 32)
	b[11] = byte(n.n[1] >> 24)
	b[10] = byte(n.n[1] >> 16)
	b[9] = byte(n.n[1] >> 8)
	b[8] = byte(n.n[1])
	b[7] = byte(n.n[0] >> 56)
	b[6] = byte(n.n[0] >> 48)
	b[5] = byte(n.n[0] >> 40)
	b[4] = byte(n.n[0] >> 32)
	b[3] = byte(n.n[0] >> 24)
	b[2] = byte(n.n[0] >> 16)
	b[1] = byte(n.n[0] >> 8)
	b[0] = byte(n.n[0])
}

// PutBytes unpacks the uint256 to a 32-byte big-endian value using the passed
// byte array.
//
// There is a similar function, PutBytesUnchecked, which unpacks the uint256
// into a slice that must have at least 32 bytes available.  This version is
// provided since it can be useful to write directly into an array that is type
// checked.
//
// Alternatively, there is also Bytes, which unpacks the uint256 into a new
// array and returns that which can sometimes be more ergonomic in applications
// that aren't concerned about an additional copy.
func (n *Uint256) PutBytes(b *[32]byte) {
	n.PutBytesUnchecked(b[:])
}

// PutBytesLE unpacks the uint256 to a 32-byte little-endian value using the
// passed byte array.
//
// There is a similar function, PutBytesUncheckedLE, which unpacks the uint256
// into a slice that must have at least 32 bytes available.  This version is
// provided since it can be useful to write directly into an array that is type
// checked.
//
// Alternatively, there is also BytesLE, which unpacks the uint256 into a new
// array and returns that which can sometimes be more ergonomic in applications
// that aren't concerned about an additional copy.
func (n *Uint256) PutBytesLE(b *[32]byte) {
	n.PutBytesUncheckedLE(b[:])
}

// Bytes unpacks the uint256 to a 32-byte big-endian array.
//
// See PutBytes and PutBytesUnchecked for variants that allow an array or slice
// to be passed which can be useful to cut down on the number of allocations
// by allowing the caller to reuse a buffer or write directly into part of a
// larger buffer.
func (n *Uint256) Bytes() [32]byte {
	var b [32]byte
	n.PutBytesUnchecked(b[:])
	return b
}

// BytesLE unpacks the uint256 to a 32-byte little-endian array.
//
// See PutBytesLE and PutBytesUncheckedLE for variants that allow an array or
// slice to be passed which can be useful to cut down on the number of
// allocations by allowing the caller to reuse a buffer or write directly into
// part of a larger buffer.
func (n *Uint256) BytesLE() [32]byte {
	var b [32]byte
	n.PutBytesUncheckedLE(b[:])
	return b
}

// Zero sets the uint256 to zero.  A newly created uint256 is already set to
// zero.  This function can be useful to clear an existing uint256 for reuse.
func (n *Uint256) Zero() {
	n.n[0], n.n[1], n.n[2], n.n[3] = 0, 0, 0, 0
}

// IsZero returns whether or not the uint256 is equal to zero.
func (n *Uint256) IsZero() bool {
	return n.n[0] == 0 && n.n[1] == 0 && n.n[2] == 0 && n.n[3] == 0
}

// IsOdd returns whether or not the uint256 is odd.
func (n *Uint256) IsOdd() bool {
	// Only odd numbers have the bottom bit set.
	return n.n[0]&1 == 1
}

// IsUint32 returns whether or not the uint256 can be converted to a uint32
// without any loss of precision.  In other words, 0 <= n < 2^32.
func (n *Uint256) IsUint32() bool {
	return (n.n[0]>>32 | n.n[1] | n.n[2] | n.n[3]) == 0
}

// Uint32 returns the uint32 representation of the value.  In other words, it
// returns the low-order 32 bits of the value as a uint32 which is equivalent to
// the value modulo 2^32.  The caller can determine if this method can be used
// without truncation with the IsUint32 method.
func (n *Uint256) Uint32() uint32 {
	return uint32(n.n[0])
}

// IsUint64 returns whether or not the uint256 can be converted to a uint64
// without any loss of precision.  In other words, 0 <= n < 2^64.
func (n *Uint256) IsUint64() bool {
	return (n.n[1] | n.n[2] | n.n[3]) == 0
}

// Uint64 returns the uint64 representation of the value.  In other words, it
// returns the low-order 64 bits of the value as a uint64 which is equivalent to
// the value modulo 2^64.  The caller can determine if this method can be used
// without truncation with the IsUint64 method.
func (n *Uint256) Uint64() uint64 {
	return n.n[0]
}

// Eq returns whether or not the two uint256s represent the same value.
func (n *Uint256) Eq(n2 *Uint256) bool {
	return n.n[0] == n2.n[0] && n.n[1] == n2.n[1] && n.n[2] == n2.n[2] &&
		n.n[3] == n2.n[3]
}

// EqUint64 returns whether or not the uint256 represents the same value as the
// given uint64.
func (n *Uint256) EqUint64(n2 uint64) bool {
	return n.n[0] == n2 && (n.n[1]|n.n[2]|n.n[3]) == 0
}

// Lt returns whether or not the uint256 is less than the given one.  That is,
// it returns true when n < n2.
func (n *Uint256) Lt(n2 *Uint256) bool {
	var borrow uint64
	_, borrow = bits.Sub64(n.n[0], n2.n[0], borrow)
	_, borrow = bits.Sub64(n.n[1], n2.n[1], borrow)
	_, borrow = bits.Sub64(n.n[2], n2.n[2], borrow)
	_, borrow = bits.Sub64(n.n[3], n2.n[3], borrow)
	return borrow != 0
}

// LtUint64 returns whether or not the uint256 is less than the given uint64.
// That is, it returns true when n < n2.
func (n *Uint256) LtUint64(n2 uint64) bool {
	return n.n[0] < n2 && (n.n[1]|n.n[2]|n.n[3]) == 0
}

// LtEq returns whether or not the uint256 is less than or equal to the given
// one.  That is, it returns true when n <= n2.
func (n *Uint256) LtEq(n2 *Uint256) bool {
	return !n2.Lt(n)
}

// LtEqUint64 returns whether or not the uint256 is less than or equal to the
// given uint64.  That is, it returns true when n <= n2.
func (n *Uint256) LtEqUint64(n2 uint64) bool {
	return n.n[0] <= n2 && (n.n[1]|n.n[2]|n.n[3]) == 0
}

// Gt returns whether or not the uint256 is greater than the given one.  That
// is, it returns true when n > n2.
func (n *Uint256) Gt(n2 *Uint256) bool {
	return n2.Lt(n)
}

// GtUint64 returns whether or not the uint256 is greater than the given uint64.
// That is, it returns true when n > n2.
func (n *Uint256) GtUint64(n2 uint64) bool {
	return n.n[0] > n2 || (n.n[1]|n.n[2]|n.n[3]) != 0
}

// GtEq returns whether or not the uint256 is greater than or equal to the given
// one.  That is, it returns true when n >= n2.
func (n *Uint256) GtEq(n2 *Uint256) bool {
	return !n.Lt(n2)
}

// GtEqUint64 returns whether or not the uint256 is greater than or equal to the
// given uint64.  That is, it returns true when n >= n2.
func (n *Uint256) GtEqUint64(n2 uint64) bool {
	return !n.LtUint64(n2)
}

// Cmp compares the two uint256s and returns -1, 0, or 1 depending on whether
// the uint256 is less than, equal to, or greater than the given one.
//
// That is, it returns:
//
//   -1 when n <  n2
//    0 when n == n2
//   +1 when n >  n2
func (n *Uint256) Cmp(n2 *Uint256) int {
	var borrow uint64
	r0, borrow := bits.Sub64(n.n[0], n2.n[0], borrow)
	r1, borrow := bits.Sub64(n.n[1], n2.n[1], borrow)
	r2, borrow := bits.Sub64(n.n[2], n2.n[2], borrow)
	r3, borrow := bits.Sub64(n.n[3], n2.n[3], borrow)
	if borrow != 0 {
		return -1
	}
	if r0|r1|r2|r3 == 0 {
		return 0
	}
	return 1
}

// CmpUint64 compares the uint256 with the given uint64 and returns -1, 0, or 1
// depending on whether the uint256 is less than, equal to, or greater than the
// uint64.
//
// That is, it returns:
//
//   -1 when n <  n2
//    0 when n == n2
//   +1 when n >  n2
func (n *Uint256) CmpUint64(n2 uint64) int {
	if n.LtUint64(n2) {
		return -1
	}
	if n.GtUint64(n2) {
		return 1
	}
	return 0
}

// Add2 adds the passed two uint256s together modulo 2^256 and stores the result
// in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.Add2(n1, n2).AddUint64(1) so that n = n1 + n2 + 1.
func (n *Uint256) Add2(n1, n2 *Uint256) *Uint256 {
	var c uint64
	n.n[0], c = bits.Add64(n1.n[0], n2.n[0], c)
	n.n[1], c = bits.Add64(n1.n[1], n2.n[1], c)
	n.n[2], c = bits.Add64(n1.n[2], n2.n[2], c)
	n.n[3], _ = bits.Add64(n1.n[3], n2.n[3], c)
	return n
}

// Add adds the passed uint256 to the existing one modulo 2^256 and stores the
// result in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.Add(n2).AddUin64(1) so that n = n + n2 + 1.
func (n *Uint256) Add(n2 *Uint256) *Uint256 {
	return n.Add2(n, n2)
}

// AddUint64 adds the passed uint64 to the existing uint256 modulo 2^256 and
// stores the result in n.
//
// The scalar is returned to support chaining.  This enables syntax like:
// n.AddUint64(2).MulUint64(2) so that n = (n + 2) * 2.
func (n *Uint256) AddUint64(n2 uint64) *Uint256 {
	var c uint64
	n.n[0], c = bits.Add64(n.n[0], n2, c)
	n.n[1], c = bits.Add64(n.n[1], 0, c)
	n.n[2], c = bits.Add64(n.n[2], 0, c)
	n.n[3], _ = bits.Add64(n.n[3], 0, c)
	return n
}

// Sub2 subtracts the second given uint256 from the first modulo 2^256 and
// stores the result in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.Sub2(n1, n2).AddUint64(1) so that n = n1 - n2 + 1.
func (n *Uint256) Sub2(n1, n2 *Uint256) *Uint256 {
	var borrow uint64
	n.n[0], borrow = bits.Sub64(n1.n[0], n2.n[0], borrow)
	n.n[1], borrow = bits.Sub64(n1.n[1], n2.n[1], borrow)
	n.n[2], borrow = bits.Sub64(n1.n[2], n2.n[2], borrow)
	n.n[3], _ = bits.Sub64(n1.n[3], n2.n[3], borrow)
	return n
}

// Sub subtracts the given uint256 from the existing one modulo 2^256 and stores
// the result in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.Sub(n2).AddUint64(1) so that n = n - n2 + 1.
func (n *Uint256) Sub(n2 *Uint256) *Uint256 {
	var borrow uint64
	n.n[0], borrow = bits.Sub64(n.n[0], n2.n[0], borrow)
	n.n[1], borrow = bits.Sub64(n.n[1], n2.n[1], borrow)
	n.n[2], borrow = bits.Sub64(n.n[2], n2.n[2], borrow)
	n.n[3], _ = bits.Sub64(n.n[3], n2.n[3], borrow)
	return n
}

// SubUint64 subtracts the given uint64 from the uint256 modulo 2^256 and stores
// the result in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.SubUint64(1).MulUint64(3) so that n = (n - 1) * 3.
func (n *Uint256) SubUint64(n2 uint64) *Uint256 {
	var borrow uint64
	n.n[0], borrow = bits.Sub64(n.n[0], n2, borrow)
	n.n[1], borrow = bits.Sub64(n.n[1], 0, borrow)
	n.n[2], borrow = bits.Sub64(n.n[2], 0, borrow)
	n.n[3], _ = bits.Sub64(n.n[3], 0, borrow)
	return n
}

// mulAdd64 multiplies the two passed base 2^64 digits together, adds the given
// value to the result, and returns the 128-bit result via a (hi, lo) tuple
// where the upper half of the bits are returned in hi and the lower half in lo.
func mulAdd64(digit1, digit2, m uint64) (hi, lo uint64) {
	// Note the carry on the final add is safe to discard because the maximum
	// possible value is:
	//   (2^64 - 1)(2^64 - 1) + (2^64 - 1) = 2^128 - 2^64
	// and:
	//   2^128 - 2^64 < 2^128.
	var c uint64
	hi, lo = bits.Mul64(digit1, digit2)
	lo, c = bits.Add64(lo, m, 0)
	hi, _ = bits.Add64(hi, 0, c)
	return hi, lo
}

// mulAdd64Carry multiplies the two passed base 2^64 digits together, adds both
// the given value and carry to the result, and returns the 128-bit result via a
// (hi, lo) tuple where the upper half of the bits are returned in hi and the
// lower half in lo.
func mulAdd64Carry(digit1, digit2, m, c uint64) (hi, lo uint64) {
	// Note the carries on the high order adds are safe to discard because the
	// maximum possible value is:
	//   (2^64 - 1)(2^64 - 1) + 2*(2^64 - 1) = 2^128 - 1
	// and:
	//   2^128 - 1 < 2^128.
	var c2 uint64
	hi, lo = bits.Mul64(digit1, digit2)
	lo, c2 = bits.Add64(lo, m, 0)
	hi, _ = bits.Add64(hi, 0, c2)
	lo, c2 = bits.Add64(lo, c, 0)
	hi, _ = bits.Add64(hi, 0, c2)
	return hi, lo
}

// Mul2 multiplies the passed uint256s together modulo 2^256 and stores the
// result in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.Mul2(n1, n2).AddUint64(1) so that n = (n1 * n2) + 1.
func (n *Uint256) Mul2(n1, n2 *Uint256) *Uint256 {
	// The general strategy employed here is:
	// 1) Calculate the 512-bit product of the two uint256s using standard
	//    schoolbook multiplication.
	// 2) Reduce the result modulo 2^256.
	//
	// However, some optimizations are used versus naively calculating all
	// intermediate terms:
	// 1) Reuse the high 64 bits from the intermediate 128-bit products directly
	//    since that is equivalent to shifting the result right by 64 bits.
	// 2) Ignore all of the products between individual digits that would
	//    ordinarily be needed for the full 512-bit product when they are
	//    guaranteed to result in values that fall in the half open interval
	//    [2^256, 2^512) since they are all ≡ 0 (mod 2^256) given they are
	//    necessarily multiples of it.
	// 3) Use native uint64s for the calculations involving the final digit of
	//    the result (r3) because all overflow carries to bits >= 256 which, as
	//    above, are all ≡ 0 (mod 2^256) and thus safe to discard.
	var r0, r1, r2, r3, c uint64

	// Terms resulting from the product of the first digit of the second number
	// by all digits of the first number.
	c, r0 = bits.Mul64(n2.n[0], n1.n[0])
	c, r1 = mulAdd64(n2.n[0], n1.n[1], c)
	c, r2 = mulAdd64(n2.n[0], n1.n[2], c)
	r3 = n2.n[0]*n1.n[3] + c

	// Terms resulting from the product of the second digit of the second number
	// by all digits of the first number except those that are guaranteed to be
	// ≡ 0 (mod 2^256).
	c, r1 = mulAdd64(n2.n[1], n1.n[0], r1)
	c, r2 = mulAdd64Carry(n2.n[1], n1.n[1], r2, c)
	r3 += n2.n[1]*n1.n[2] + c

	// Terms resulting from the product of the third digit of the second number
	// by all digits of the first number except those that are guaranteed to be
	// ≡ 0 (mod 2^256).
	c, r2 = mulAdd64(n2.n[2], n1.n[0], r2)
	r3 += n2.n[2]*n1.n[1] + c

	// Terms resulting from the product of the fourth digit of the second number
	// by all digits of the first number except those that are guaranteed to be
	// ≡ 0 (mod 2^256).
	r3 += n2.n[3] * n1.n[0]

	n.n[0], n.n[1], n.n[2], n.n[3] = r0, r1, r2, r3
	return n
}

// Mul multiplies the passed uint256 by the existing value in n modulo 2^256
// and stores the result in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.Mul(n2).AddUint64(1) so that n = (n * n2) + 1.
func (n *Uint256) Mul(n2 *Uint256) *Uint256 {
	return n.Mul2(n, n2)
}

// MulUint64 multiplies the uint256 by the passed uint64 and stores the result
// in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.MulUint64(2).Add(n2) so that n = 2 * n + n2.
func (n *Uint256) MulUint64(n2 uint64) *Uint256 {
	// The general strategy employed here is:
	// 1) Calculate the 320-bit product of the uint256 and uint64 using standard
	//    schoolbook multiplication.
	// 2) Reduce the result modulo 2^256.
	//
	// However, some optimizations are used versus naively calculating all
	// intermediate terms:
	// 1) Reuse the high 64 bits from the intermediate 128-bit products directly
	//    since that is equivalent to shifting the result right by 64 bits.
	// 2) Use native uint64s for the calculations involving the final digit of
	//    the result because all overflow carries to bits >= 256 which is ≡ 0
	//    (mod 2^256) given they are necessarily multiples of it.
	var c uint64
	c, n.n[0] = bits.Mul64(n.n[0], n2)
	c, n.n[1] = mulAdd64(n.n[1], n2, c)
	c, n.n[2] = mulAdd64(n.n[2], n2, c)
	n.n[3] = n.n[3]*n2 + c
	return n
}

// SquareVal squares the passed uint256 modulo 2^256 and stores the result in
// n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.SquareVal(n2).Mul(n2) so that n = n2^2 * n2 = n2^3.
func (n *Uint256) SquareVal(n2 *Uint256) *Uint256 {
	// Similar to multiplication, the general strategy employed here is:
	// 1) Calculate the 512-bit product of the two uint256s using standard
	//    schoolbook multiplication.
	// 2) Reduce the result modulo 2^256.
	//
	// However, some optimizations are used versus naively calculating all
	// intermediate terms:
	// 1) Reuse the high 64 bits from the intermediate 128-bit products directly
	//    since that is equivalent to shifting the result right by 64 bits.
	// 2) Ignore all of the products between individual digits that would
	//    ordinarily be needed for the full 512-bit product when they are
	//    guaranteed to result in values that fall in the half open interval
	//    [2^256, 2^512) since they are all ≡ 0 (mod 2^256) given they are
	//    necessarily multiples of it.
	// 3) Use native uint64s for the calculations involving the final digit of
	//    the result (r3) because all overflow carries to bits >= 256 which, as
	//    above, are all ≡ 0 (mod 2^256) and thus safe to discard.
	// 4) Use the fact the number is being multiplied by itself to take
	//    advantage of symmetry to double the result of some individual products
	//    versus calculating them again.
	var r0, r1, r2, r3, c uint64

	// Terms resulting from the product of the first digit of the second number
	// by all digits of the first number except those that will be accounted for
	// later via symmetry.
	c, r0 = bits.Mul64(n2.n[0], n2.n[0])
	c, r1 = mulAdd64(n2.n[0], n2.n[1], c)
	c, r2 = mulAdd64(n2.n[0], n2.n[2], c)
	r3 = c

	// Terms resulting from the product of the second digit of the second number
	// by all digits of the first number except those that will be accounted for
	// later via symmetry and those that are guaranteed to be ≡ 0 (mod 2^256).
	c, r1 = mulAdd64(n2.n[1], n2.n[0], r1)
	c, r2 = mulAdd64Carry(n2.n[1], n2.n[1], r2, c)
	r3 += c

	// Terms resulting from the product of the third digit of the second number
	// by all digits of the first number except those that will be accounted for
	// later via symmetry and those that are guaranteed to be ≡ 0 (mod 2^256).
	c, r2 = mulAdd64(n2.n[2], n2.n[0], r2)
	r3 += c

	// Terms resulting from the product of the fourth digit of the second number
	// by all digits of the first number except those that are guaranteed to be
	// ≡ 0 (mod 2^256) and doubling those that were skipped earlier.
	r3 += 2 * (n2.n[0]*n2.n[3] + n2.n[1]*n2.n[2])

	n.n[0], n.n[1], n.n[2], n.n[3] = r0, r1, r2, r3
	return n
}

// Square squares the uint256 modulo 2^256 and stores the result in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.Square().Mul(n2) so that n = n^2 * n2.
func (n *Uint256) Square() *Uint256 {
	return n.SquareVal(n)
}

// numDigits returns the number of base 2^64 digits required to represent the
// uint256.  The result is 0 when the value is 0.
func (n *Uint256) numDigits() int {
	for i := 3; i >= 0; i-- {
		if n.n[i] != 0 {
			return i + 1
		}
	}
	return 0
}

// prefixLt returns whether or not the first argument is less than the prefix of
// the same length from the second when treating both as little-endian encoded
// integers.  That is, it returns true when a < b[0:len(a)].
func prefixLt(a, b []uint64) bool {
	var borrow uint64
	for i := 0; i < len(a); i++ {
		_, borrow = bits.Sub64(a[i], b[i], borrow)
	}
	return borrow != 0
}

// Div2 divides the passed uint256 dividend by the passed uint256 divisor modulo
// 2^256 and stores the result in n.  It will panic if the divisor is 0.
//
// This implements truncated division like native Go integers and it is safe to
// alias the arguments.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.Div2(n1, n2).AddUint64(1) so that n = (n1 / n2) + 1.
func (n *Uint256) Div2(dividend, divisor *Uint256) *Uint256 {
	if divisor.IsZero() {
		panic("division by zero")
	}

	// Fast path for a couple of obvious cases.  The result is zero when the
	// divisor is larger than the dividend and one when they are equal.  The
	// remaining code also has preconditions on these cases already being
	// handled.
	if divisor.Gt(dividend) {
		n.Zero()
		return n
	}
	if dividend.Eq(divisor) {
		return n.SetUint64(1)
	}

	// When the dividend can be fully represented by a uint64, perform the
	// division using native uint64s since the divisor is also guaranteed to be
	// fully representable as a uint64 given it was already confirmed to be
	// smaller than the dividend above.
	if dividend.IsUint64() {
		return n.SetUint64(dividend.n[0] / divisor.n[0])
	}

	// When the divisor can be fully represented by a uint64, the divisor only
	// consists of a single base 2^64 digit, so use that fact to avoid extra
	// work.  It is important to note that the algorithm below also requires the
	// divisor to be at least two digits, so this is not solely a performance
	// optimization.
	if divisor.IsUint64() {
		// The range here satisfies the following inequality:
		//  1 ≤ divisor < 2^64 ≤ dividend < 2^256
		var quotient Uint256
		var r uint64
		for d := dividend.numDigits() - 1; d >= 0; d-- {
			quotient.n[d], r = bits.Div64(r, dividend.n[d], divisor.n[0])
		}
		return n.Set(&quotient)
	}

	// The range at this point satisfies the following inequality:
	//  2^64 ≤ divisor < dividend < 2^256

	// -------------------------------------------------------------------------
	// The following is loosely based on Algorithm 4.3.1D of [TAOCP2] and is
	// therefore based on long (aka schoolbook) division.  It has been modified
	// as compared to said algorithm in the following ways for performance
	// reasons:
	//
	// 1. It uses full words instead of splitting each one into half words
	// 2. It does not perform the test which effectively expands the estimate to
	//    be based on 3 digits of the dividend/remainder divided by 2 digits
	//    of the divisor instead of 2 digits by 1 (Step D3)
	// 3. It conditionally subtracts the partial product from the partial
	//    remainder in the case the latter is less than the former instead of
	//    always subtracting it and then conditionally adding back (Steps D4,
	//    D5, and D6)
	// 4. It only computes the active part of the remainder and throws the rest
	//    away since it is not needed here (parts of Steps D3, D4, D5, and D6)
	// 5. It skips undoing the normalization for the remainder since it is not
	//    needed here (Step D8)
	//
	// The primary rationale for these changes follows:
	//
	// In regards to (1), almost all modern hardware provides native 128-bit by
	// 64-bit division operations as well as 64-bit by 64-bit multiplication
	// operations which the Go compiler uses in place of the respective
	// `bits.Div64/Mul64` and, as a further benefit, using full words cuts the
	// number of iterations in half.
	//
	// The test referred to by (2) serves to pin the potential error in the
	// estimated quotient to be a max of 1 too high instead of a max of 2 as
	// well as reduce the probability that single correction is needed at all.
	// However, in order to achieve that, given full 64-bit words are used here,
	// the Knuth method would involve an unconditional test which relies on
	// comparing the result of a software-implemented 128-bit product against
	// another 128-bit product as well as a 128-bit addition with carry.  That
	// is typically a sensible tradeoff when the number of digits involved is
	// large and when working with half words since an additional potential
	// correction would typically cost more than the test under those
	// circumstances.
	//
	// However, in this implementation, it ends being faster to perform the
	// extra correction than it is to perform the aforementioned test.  The
	// primary reasons for this are:
	//
	// - The number of digits is ≤ 4 which means very few operations are needed
	//   for the correction
	// - Both the comparison of the product against the remainder and the
	//   conditional subtraction of the product from the remainder are able to
	//   make use of hardware acceleration that almost all modern hardware
	//   provides
	//
	// Moreover, the probability of the initial estimate being correct
	// significantly increases as the size of the base grows.  Given this
	// implementation uses a base of 2^64, the probability of needing a
	// correction is extremely low.
	//
	// Concretely, for quotient digits that are uniformly distributed, the
	// probability of guessing the initial estimate correctly with normalization
	// for a base of 2^64 is: 1 - 2/2^64 ~= 99.99999999999999999%.  In other
	// words, on average, roughly only 1 out of every 10 quintillion (10^19)
	// will require an adjustment.
	// -------------------------------------------------------------------------

	// Start by normalizing the arguments.
	//
	// For reference, normalization is the process of scaling the operands such
	// that the leading digit of the divisor is greater than or equal to half of
	// the base (also often called the radix).  Since this implementation uses a
	// base of 2^64, half of that equates to 2^63.  Mathematically:
	//  2^63 ≤ leading digit of divisor < 2^64
	//
	// Note that the number of digits in each operand at this point satisfies
	// the following inequality:
	//  2 ≤ num divisor digits ≤ num dividend digits ≤ 4
	//
	// Further, notice that being ≥ 2^63 is equivalent to a digit having its
	// most significant bit set.  This means the scaling factor can very quickly
	// be determined by counting the number of leading zeros and applied to the
	// operands via bit shifts.
	//
	// This process significantly reduces the probability of requiring an
	// adjustment to the initial estimate for quotient digits later as per the
	// previous comments as well as bounds the possible number of error
	// adjustments.
	numDivisorDigits := divisor.numDigits()
	numDividendDigits := dividend.numDigits()
	sf := uint8(bits.LeadingZeros64(divisor.n[numDivisorDigits-1]))
	var divisorN [4]uint64
	var dividendN [5]uint64
	if sf > 0 {
		// Normalize the divisor by scaling it per the scaling factor.
		for i := numDivisorDigits - 1; i > 0; i-- {
			divisorN[i] = divisor.n[i]<<sf | divisor.n[i-1]>>(64-sf)
		}
		divisorN[0] = divisor.n[0] << sf

		// Scale the dividend by the same scaling factor too.
		dividendN[numDividendDigits] = dividend.n[numDividendDigits-1] >> (64 - sf)
		for i := numDividendDigits - 1; i > 0; i-- {
			dividendN[i] = dividend.n[i]<<sf | dividend.n[i-1]>>(64-sf)
		}
		dividendN[0] = dividend.n[0] << sf
	} else {
		copy(divisorN[:], divisor.n[:])
		copy(dividendN[:], dividend.n[:])
	}

	// NOTE: The number of digits in the non-normalized dividend and divisor
	// satisfies the following inequality at this point:
	//  2 ≤ numDivisorDigits ≤ numDividendDigits ≤ 4
	//
	// Therefore the maximum value of d in the loop is 2.
	var p [5]uint64
	var qhat, c, borrow uint64
	n.Zero()
	for d := numDividendDigits - numDivisorDigits; d >= 0; d-- {
		// Estimate the quotient digit by dividing the 2 leading digits of the
		// active part of the remainder by the leading digit of the normalized
		// divisor.
		//
		// Per Theorems A and B in section 4.3.1 of [TAOCP2], when the leading
		// digit of the divisor is normalized as described above, the estimated
		// quotient digit (qhat) produced by this is guaranteed to be ≥ the
		// correct one (q) and at most 2 more.  Mathematically:
		//  qhat - 2 ≤ q ≤ qhat
		//
		// Further, the algorithm implemented here guarantees that the leading
		// digit of the active part of the remainder will be ≤ the leading digit
		// of the normalized divisor.  When it is less, the estimated quotient
		// digit will fit into a single word and everything is normal.  However,
		// in the equals case, a second word would be required meaning the
		// calculation would overflow.
		//
		// Combining that with the theorem above, the estimate in the case of
		// overflow must either be one or two more than the maximum possible
		// value for a single digit.  In either case, assuming a type capable of
		// representing values larger than a single digit without overflow were
		// used instead, the quotient digit would have to be adjusted downwards
		// until it was correct, and given the correct quotient digit must fit
		// into a single word, it will necessarily either be the maximum
		// possible value allowed for a single digit or one less.  Therefore,
		// the equality check here bypasses the overflow by setting the estimate
		// to the max possible value and allowing the loop below to handle the
		// potential remaining (extremely rare) overestimate.
		//
		// Also note that this behavior is not merely an optimization to skip
		// the 128-bit by 64-bit division as both the hardware instruction and
		// the bits.Div64 implementation would otherwise lead to a panic due to
		// overflow as well.
		//
		// For some intuition, consider the base 10 case with a divisor of 5
		// (recall the divisor is normalized, so it must be at least half the
		// base).  Then the maximum 2-digit dividend with the leading digit less
		// than 5 is 49 and the minimum with the leading digit equal to 5 is 50.
		// Observe that floor(49/5) = 9 is a single digit result, but
		// floor(50/5) = 10 is not.  The worst overestimate case for these
		// conditions would occur at floor(59/5) = floor(11.8) = 11 which is
		// indeed 2 more than the maximum possible single digit 9.
		if dividendN[d+numDivisorDigits] == divisorN[numDivisorDigits-1] {
			qhat = ^uint64(0)
		} else {
			qhat, _ = bits.Div64(dividendN[d+numDivisorDigits],
				dividendN[d+numDivisorDigits-1], divisorN[numDivisorDigits-1])
		}

		// Calculate the product of the estimated quotient digit and divisor.
		//
		// This is semantically equivalent to the following if uint320 were
		// supported:
		//
		// p = uint320(qhat) * uint320(divisorN)
		//
		// Note that this will compute extra upper terms when the divisor is
		// fewer digits than the max possible even though it is not really
		// needed since they will all be zero.  However, because it is only
		// potentially a max of two extra digits, it ends up being faster on
		// average to avoid the additional conditional branches due to
		// pipelining.
		c, p[0] = bits.Mul64(qhat, divisorN[0])
		c, p[1] = mulAdd64(qhat, divisorN[1], c)
		c, p[2] = mulAdd64(qhat, divisorN[2], c)
		p[4], p[3] = mulAdd64(qhat, divisorN[3], c)

		// Adjust the estimate (and associated product) downwards when they are
		// too high for the active part of the partial remainder.  As described
		// above, qhat is guaranteed to be at most two more than the correct
		// quotient digit, so this loop will run at most twice.
		//
		// Moreover, the probability of a single correction is extremely rare
		// and that of two corrections is roughly an additional two orders of
		// magnitude less than that.  In other words, in practice, the two
		// corrections case almost never happens.
		for prefixLt(dividendN[d:d+numDivisorDigits+1], p[:]) {
			qhat--

			// Subtract the divisor from the product to adjust it for the
			// reduced quotient.
			//
			// This is semantically equivalent to the following if uint320 were
			// supported (and with p as a uint320 per above):
			//
			// p -= uint320(divisorN)
			//
			// Note that as with the original calculation of the product above,
			// this will compute extra upper terms when the divisor is fewer
			// digits than the max possible even though it is not really needed
			// for performance reasons as previously described.
			p[0], borrow = bits.Sub64(p[0], divisorN[0], 0)
			p[1], borrow = bits.Sub64(p[1], divisorN[1], borrow)
			p[2], borrow = bits.Sub64(p[2], divisorN[2], borrow)
			p[3], borrow = bits.Sub64(p[3], divisorN[3], borrow)
			p[4] -= borrow
		}

		// Set the quotient digit in the result.
		n.n[d] = qhat

		// Update the dividend by subtracting the resulting product from it so
		// that it becomes the new remainder to use for calculating the next
		// quotient digit.
		//
		// Note that this is only calculating the _active_ part of the remainder
		// since the final remainder is not needed and the next iteration slides
		// over one digit to the right meaning none of the upper digits are used
		// and are therefore safe to ignore despite them no longer being
		// accurate.
		//
		// Also, since 'd' is a maximum of 2 due to previous constraints, there
		// is no chance of overflowing the array.
		//
		// This is semantically equivalent to the following if uint192 and the
		// syntax to set base 2^64 digits in little endian directly were
		// supported:
		//
		// dividendN[d:d+3] = uint192(dividendN[d:d+3]) - uint192(p[0:3])
		dividendN[d], borrow = bits.Sub64(dividendN[d], p[0], 0)
		dividendN[d+1], borrow = bits.Sub64(dividendN[d+1], p[1], borrow)
		dividendN[d+2], _ = bits.Sub64(dividendN[d+2], p[2], borrow)
	}

	return n
}

// Div divides the existing value in n by the passed uint256 divisor modulo
// 2^256 and stores the result in n.  It will panic if the divisor is 0.
//
// This implements truncated division like native Go integers.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.Div(n2).AddUint64(1) so that n = (n / n2) + 1.
func (n *Uint256) Div(divisor *Uint256) *Uint256 {
	return n.Div2(n, divisor)
}

// DivUint64 divides the existing value in n by the passed uint64 divisor modulo
// 2^256 and stores the result in n.  It will panic if the divisor is 0.
//
// This implements truncated division like native Go integers.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.DivUint64(2).AddUint64(1) so that n = (n / 2) + 1.
func (n *Uint256) DivUint64(divisor uint64) *Uint256 {
	if divisor == 0 {
		panic("division by zero")
	}

	// Fast path for a couple of obvious cases.  The result is zero when the
	// dividend is smaller than the divisor and one when they are equal.  The
	// remaining code also has preconditions on these cases already being
	// handled.
	if n.LtUint64(divisor) {
		n.Zero()
		return n
	}
	if n.EqUint64(divisor) {
		return n.SetUint64(1)
	}

	// The range here satisfies the following inequalities:
	//  1 ≤ divisor < 2^64
	//  1 ≤ divisor < dividend < 2^256
	var quotient Uint256
	var r uint64
	for d := n.numDigits() - 1; d >= 0; d-- {
		quotient.n[d], r = bits.Div64(r, n.n[d], divisor)
	}
	return n.Set(&quotient)
}

// NegateVal negates the passed uint256 modulo 2^256 and stores the result in
// n.  In other words, n will be set to the two's complement of the passed
// uint256.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.NegateVal(n2).AddUint64(1) so that n = -n2 + 1.
func (n *Uint256) NegateVal(n2 *Uint256) *Uint256 {
	// This uses math/bits to perform the negation via subtraction from 0 as the
	// compiler will replace it with the relevant intrinsic on most
	// architectures.
	var borrow uint64
	n.n[0], borrow = bits.Sub64(0, n2.n[0], borrow)
	n.n[1], borrow = bits.Sub64(0, n2.n[1], borrow)
	n.n[2], borrow = bits.Sub64(0, n2.n[2], borrow)
	n.n[3], _ = bits.Sub64(0, n2.n[3], borrow)
	return n
}

// Negate negates the uint256 modulo 2^256.  In other words, n will be set to
// its two's complement.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.Negate().AddUint64(1) so that n = -n + 1.
func (n *Uint256) Negate() *Uint256 {
	return n.NegateVal(n)
}

// LshVal shifts the passed uint256 to the left the given number of bits and
// stores the result in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.LshVal(n2, 2).AddUint64(1) so that n = (n2 << 2) + 1.
func (n *Uint256) LshVal(n2 *Uint256, bits uint32) *Uint256 {
	// Fast path for large and zero shifts.
	if bits > 255 {
		n.Zero()
		return n
	}
	if bits == 0 {
		return n.Set(n2)
	}

	// Shift entire words when possible.
	switch {
	case bits >= 192:
		// Left shift 192 bits.
		n.n[3], n.n[2], n.n[1], n.n[0] = n2.n[0], 0, 0, 0
		bits -= 192
		if bits == 0 {
			return n
		}
		n.n[3] <<= bits
		return n

	case bits >= 128:
		// Left shift 128 bits.
		n.n[3], n.n[2], n.n[1], n.n[0] = n2.n[1], n2.n[0], 0, 0
		bits -= 128
		if bits == 0 {
			return n
		}
		n.n[3] = (n.n[3] << bits) | (n.n[2] >> (64 - bits))
		n.n[2] <<= bits
		return n

	case bits >= 64:
		// Left shift 64 bits.
		n.n[3], n.n[2], n.n[1], n.n[0] = n2.n[2], n2.n[1], n2.n[0], 0
		bits -= 64
		if bits == 0 {
			return n
		}
		n.n[3] = (n.n[3] << bits) | (n.n[2] >> (64 - bits))
		n.n[2] = (n.n[2] << bits) | (n.n[1] >> (64 - bits))
		n.n[1] <<= bits
		return n
	}

	// At this point the shift must be less than 64 bits, so shift each word
	// accordingly.
	n.n[3] = (n2.n[3] << bits) | (n2.n[2] >> (64 - bits))
	n.n[2] = (n2.n[2] << bits) | (n2.n[1] >> (64 - bits))
	n.n[1] = (n2.n[1] << bits) | (n2.n[0] >> (64 - bits))
	n.n[0] = n2.n[0] << bits
	return n
}

// Lsh shifts the uint256 to the left the given number of bits and stores the
// result in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.Lsh(2).AddUint64(1) so that n = (n << 2) + 1.
func (n *Uint256) Lsh(bits uint32) *Uint256 {
	if bits == 0 {
		return n
	}
	return n.LshVal(n, bits)
}

// RshVal shifts the passed uint256 to the right the given number of bits and
// stores the result in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.RshVal(n2, 2).AddUint64(1) so that n = (n2 >> 2) + 1.
func (n *Uint256) RshVal(n2 *Uint256, bits uint32) *Uint256 {
	// Fast path for large and zero shifts.
	if bits > 255 {
		n.Zero()
		return n
	}
	if bits == 0 {
		return n.Set(n2)
	}

	// Shift entire words when possible.
	switch {
	case bits >= 192:
		// Right shift 192 bits.
		n.n[3], n.n[2], n.n[1], n.n[0] = 0, 0, 0, n2.n[3]
		bits -= 192
		if bits == 0 {
			return n
		}
		n.n[0] >>= bits
		return n

	case bits >= 128:
		// Right shift 128 bits.
		n.n[3], n.n[2], n.n[1], n.n[0] = 0, 0, n2.n[3], n2.n[2]
		bits -= 128
		if bits == 0 {
			return n
		}
		n.n[0] = (n.n[0] >> bits) | (n.n[1] << (64 - bits))
		n.n[1] >>= bits
		return n

	case bits >= 64:
		// Right shift 64 bits.
		n.n[3], n.n[2], n.n[1], n.n[0] = 0, n2.n[3], n2.n[2], n2.n[1]
		bits -= 64
		if bits == 0 {
			return n
		}
		n.n[0] = (n.n[0] >> bits) | (n.n[1] << (64 - bits))
		n.n[1] = (n.n[1] >> bits) | (n.n[2] << (64 - bits))
		n.n[2] >>= bits
		return n
	}

	// At this point the shift must be less than 64 bits, so shift each word
	// accordingly.
	n.n[0] = (n2.n[0] >> bits) | (n2.n[1] << (64 - bits))
	n.n[1] = (n2.n[1] >> bits) | (n2.n[2] << (64 - bits))
	n.n[2] = (n2.n[2] >> bits) | (n2.n[3] << (64 - bits))
	n.n[3] = n2.n[3] >> bits
	return n
}

// Rsh shifts the uint256 to the right the given number of bits and stores the
// result in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.Rsh(2).AddUint64(1) so that n = (n >> 2) + 1.
func (n *Uint256) Rsh(bits uint32) *Uint256 {
	if bits == 0 {
		return n
	}
	return n.RshVal(n, bits)
}

// Not computes the bitwise not of the uint256 and stores the result in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.Not().AddUint64(1) so that n = ^n + 1.
func (n *Uint256) Not() *Uint256 {
	n.n[0] = ^n.n[0]
	n.n[1] = ^n.n[1]
	n.n[2] = ^n.n[2]
	n.n[3] = ^n.n[3]
	return n
}

// Or computes the bitwise or of the uint256 and the passed uint256 and stores
// the result in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.Or(n2).AddUint64(1) so that n = (n | n2) + 1.
func (n *Uint256) Or(n2 *Uint256) *Uint256 {
	n.n[0] |= n2.n[0]
	n.n[1] |= n2.n[1]
	n.n[2] |= n2.n[2]
	n.n[3] |= n2.n[3]
	return n
}

// And computes the bitwise and of the uint256 and the passed uint256 and stores
// the result in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.And(n2).AddUint64(1) so that n = (n & n2) + 1.
func (n *Uint256) And(n2 *Uint256) *Uint256 {
	n.n[0] &= n2.n[0]
	n.n[1] &= n2.n[1]
	n.n[2] &= n2.n[2]
	n.n[3] &= n2.n[3]
	return n
}

// Xor computes the bitwise exclusive or of the uint256 and the passed uint256
// and stores the result in n.
//
// The uint256 is returned to support chaining.  This enables syntax like:
// n.Xor(n2).AddUint64(1) so that n = (n ^ n2) + 1.
func (n *Uint256) Xor(n2 *Uint256) *Uint256 {
	n.n[0] ^= n2.n[0]
	n.n[1] ^= n2.n[1]
	n.n[2] ^= n2.n[2]
	n.n[3] ^= n2.n[3]
	return n
}

// BitLen returns the minimum number of bits required to represent the uint256.
// The result is 0 when the value is 0.
func (n *Uint256) BitLen() uint16 {
	if w := n.n[3]; w > 0 {
		return uint16(bits.Len64(w)) + 192
	}
	if w := n.n[2]; w > 0 {
		return uint16(bits.Len64(w)) + 128
	}
	if w := n.n[1]; w > 0 {
		return uint16(bits.Len64(w)) + 64
	}
	return uint16(bits.Len64(n.n[0]))
}

// bitsPerInternalWord is the number of bits used for each internal word of the
// uint256.
const bitsPerInternalWord = 64

// toBin converts the uint256 to its string representation in base 2.
func (n *Uint256) toBin() []byte {
	if n.IsZero() {
		return []byte("0")
	}

	// Create space for the max possible number of output digits.
	maxOutDigits := n.BitLen()
	result := make([]byte, maxOutDigits)

	// Convert each internal base 2^64 word to base 2 from least to most
	// significant.  Since the value is guaranteed to be non-zero per a previous
	// check, there will always be a nonzero most-significant word.  Also, note
	// that no partial digit handling is needed in this case because the shift
	// amount evenly divides the bits per internal word.
	const shift = 1
	const mask = 1<<shift - 1
	const digitsPerInternalWord = bitsPerInternalWord
	outputIdx := maxOutDigits - 1
	numInputWords := n.numDigits()
	inputWord := n.n[0]
	for inputIdx := 1; inputIdx < numInputWords; inputIdx++ {
		for i := 0; i < digitsPerInternalWord; i++ {
			result[outputIdx] = '0' + byte(inputWord&mask)
			inputWord >>= shift
			outputIdx--
		}
		inputWord = n.n[inputIdx]
	}
	for inputWord != 0 {
		result[outputIdx] = '0' + byte(inputWord&mask)
		inputWord >>= shift
		outputIdx--
	}

	return result[outputIdx+1:]
}

// toOctal converts the uint256 to its string representation in base 8.
func (n *Uint256) toOctal() []byte {
	if n.IsZero() {
		return []byte("0")
	}

	// Create space for the max possible number of output digits using the fact
	// that 3 bits converts directly to a single octal digit.
	maxOutDigits := (n.BitLen() + 2) / 3
	result := make([]byte, maxOutDigits)

	// Convert each internal base 2^64 word to base 8 from least to most
	// significant.  Since the value is guaranteed to be non-zero per a previous
	// check, there will always be a nonzero most-significant word.  Also, note
	// that partial digit handling is needed in this case because the shift
	// amount does not evenly divide the bits per internal word.
	const shift = 3
	const mask = 1<<shift - 1
	unconvertedBits := bitsPerInternalWord
	outputIdx := maxOutDigits - 1
	numInputWords := n.numDigits()
	inputWord := n.n[0]
	for inputIdx := 1; inputIdx < numInputWords; inputIdx++ {
		// Convert full digits.
		for ; unconvertedBits >= shift; unconvertedBits -= shift {
			result[outputIdx] = '0' + byte(inputWord&mask)
			inputWord >>= shift
			outputIdx--
		}

		// Move to the next input word when there are not any remaining
		// unconverted bits that need to be handled.
		if unconvertedBits == 0 {
			inputWord = n.n[inputIdx]
			unconvertedBits = bitsPerInternalWord
			continue
		}

		// Account for the remaining unconverted bits from the current word and
		// the bits needed from the next word to form a full digit for the next
		// digit.
		inputWord |= n.n[inputIdx] << unconvertedBits
		result[outputIdx] = '0' + byte(inputWord&mask)
		outputIdx--

		// Move to the next input word while accounting for the bits already
		// consumed above by shifting it and updating the unconverted bits
		// accordingly.
		inputWord = n.n[inputIdx] >> (shift - unconvertedBits)
		unconvertedBits = bitsPerInternalWord - (shift - unconvertedBits)
	}
	for inputWord != 0 {
		result[outputIdx] = '0' + byte(inputWord&mask)
		inputWord >>= shift
		outputIdx--
	}

	return result[outputIdx+1:]
}

// maxPow10ForInternalWord is the maximum power of 10 that will fit into an
// internal word.  It is the value 10^floor(64 / log2(10)) and is used when
// converting to base 10 in order to significantly reduce the number of
// divisions needed.
var maxPow10ForInternalWord = new(Uint256).SetUint64(1e19)

// toDecimal converts the uint256 to its string representation in base 10.
func (n *Uint256) toDecimal() []byte {
	if n.IsZero() {
		return []byte("0")
	}

	// Create space for the max possible number of output digits.
	//
	// Note that the actual total number of output digits is usually calculated
	// as:
	//  floor(log2(n) / log2(base)) + 1
	//
	// However, in order to avoid more expensive calculation of the full log2 of
	// the value, the code below instead calculates a value that might overcount
	// by a max of one digit and trims the result as needed via the following
	// slightly modified version of the formula:
	//  floor(bitlen(n) / log2(base)) + 1
	//
	// The modified formula is guaranteed to be large enough because:
	//  (a) floor(log2(x)) ≤ log2(x) ≤ floor(log2(x)) + 1
	//  (b) bitlen(x) = floor(log2(x)) + 1
	//
	// Which implies:
	//  (c) floor(log2(n) / log2(base)) ≤ floor(floor(log2(n))+1) / log2(base))
	//  (d) floor(log2(n) / log2(base)) ≤ floor(bitlen(n)) / log2(base))
	//
	// Note that (c) holds since the left hand side of the inequality has a
	// dividend that is ≤ the right hand side dividend due to (a) while the
	// divisor is = the right hand side divisor, and then (d) is equal to (c)
	// per (b).  Adding 1 to both sides of (d) yields an inequality where the
	// left hand side is the typical formula and the right hand side is the
	// modified formula thereby proving it will never under count.
	const log2Of10 = 3.321928094887362
	maxOutDigits := uint8(float64(n.BitLen())/log2Of10) + 1
	result := make([]byte, maxOutDigits)

	// Convert each internal base 2^64 word to base 10 from least to most
	// significant.  Since the value is guaranteed to be non-zero per a previous
	// check, there will always be a nonzero most-significant word.  Also, note
	// that partial digit handling is needed in this case because the shift
	// amount does not evenly divide the bits per internal word.
	var quo, rem, t Uint256
	var r uint64
	outputIdx := maxOutDigits - 1
	quo = *n
	for !quo.IsZero() {
		rem.Set(&quo)
		quo.Div(maxPow10ForInternalWord)
		t.Mul2(&quo, maxPow10ForInternalWord)
		inputWord := rem.Sub(&t).Uint64()
		for inputWord != 0 {
			inputWord, r = inputWord/10, inputWord%10
			result[outputIdx] = '0' + byte(r)
			outputIdx--
		}
	}

	return result[outputIdx+1:]
}

// toHex converts the uint256 to its string representation in lowercase base 16.
func (n *Uint256) toHex() []byte {
	if n.IsZero() {
		return []byte("0")
	}

	// Create space for the max possible number of output digits using the fact
	// that a nibble converts directly to a single hex digit.
	maxOutDigits := (n.BitLen() + 3) / 4
	result := make([]byte, maxOutDigits)

	// Convert each internal base 2^64 word to base 16 from least to most
	// significant.  Since the value is guaranteed to be non-zero per a
	// previous check, there will always be a nonzero most-significant word.
	// Also, note that no partial digit handling is needed in this case
	// because the shift amount evenly divides the bits per internal word.
	const alphabet = "0123456789abcdef"
	const shift = 4
	const mask = 1<<shift - 1
	const digitsPerInternalWord = bitsPerInternalWord / shift
	outputIdx := maxOutDigits - 1
	numInputWords := n.numDigits()
	inputWord := n.n[0]
	for inputIdx := 1; inputIdx < numInputWords; inputIdx++ {
		for i := 0; i < digitsPerInternalWord; i++ {
			result[outputIdx] = alphabet[inputWord&mask]
			inputWord >>= shift
			outputIdx--
		}
		inputWord = n.n[inputIdx]
	}
	for inputWord != 0 {
		result[outputIdx] = alphabet[inputWord&mask]
		inputWord >>= shift
		outputIdx--
	}

	return result[outputIdx+1:]
}

// OutputBase represents a specific base to use for the string representation of
// a number.
type OutputBase int

// These constants define the supported output bases.
const (
	// OutputBaseBinary indicates a string representation of a uint256 in
	// base 2.
	OutputBaseBinary OutputBase = 2

	// OutputBaseOctal indicates a string representation of a uint256 in base 8.
	OutputBaseOctal OutputBase = 8

	// OutputBaseDecimal indicates a string representation of a uint256 in base
	// 10.
	OutputBaseDecimal OutputBase = 10

	// OutputBaseHex indicates a string representation of a uint256 in base 16.
	OutputBaseHex OutputBase = 16
)

// Text returns the string representation of the uint256 in the given base which
// must be on of the supported bases as defined by the OutputBase type.
//
// It will return "<nil>" when the uint256 pointer is nil and a message that
// indicates the base is not supported along with the value in base 10 in the
// case the caller goes out of its way to call it with an invalid base.
func (n *Uint256) Text(base OutputBase) string {
	if n == nil {
		return "<nil>"
	}

	switch base {
	case OutputBaseHex:
		return string(n.toHex())
	case OutputBaseDecimal:
		return string(n.toDecimal())
	case OutputBaseBinary:
		return string(n.toBin())
	case OutputBaseOctal:
		return string(n.toOctal())
	}

	return fmt.Sprintf("base %d not supported (Uint256=%s)", int(base), n)
}

// String returns the scalar as a human-readable decimal string.
func (n Uint256) String() string {
	return string(n.toDecimal())
}

// Format implements fmt.Formatter.  It accepts the following format verbs:
//
//  'v' default format which is decimal
//  's' default string format which is decimal
//  'b' binary
//  'o' octal with 0 prefix when accompanied by #
//  'O' octal with 0o prefix
//  'd' decimal
//  'x' lowercase hexadecimal
//  'X' uppercase hexadecimal
//
// It also supports the full suite of the fmt package format flags for integral
// types:
//
//  '#' output base prefix:
//      binary: 0b (%#b)
//      octal: 0 (%#o)
//      hex: 0x (%#x) or 0X (%#X)
//  '-' pad with spaces on the right (left-justify field)
//  '0' pad with leading zeros rather than spaces
//
// Finally, it supports specification of the minimum number of digits
// (precision) and output field width.  Examples:
//
//  %#.64x  default width, precision 64, lowercase hex with 0x prefix
//  %256b   width 256, default precision, binary with leading zeros
//  %12.3O  width 12, precision 3, octal with 0o prefix
func (n Uint256) Format(s fmt.State, ch rune) {
	// Determine output digits for the output base.
	var digits []byte
	switch ch {
	case 'b':
		digits = n.toBin()
	case 'o', 'O':
		digits = n.toOctal()
	case 'd', 's', 'v':
		digits = n.toDecimal()
	case 'x':
		digits = n.toHex()
	case 'X':
		digits = n.toHex()
		for i, d := range digits {
			if d >= 'a' && d <= 'f' {
				digits[i] = 'A' + (d - 'a')
			}
		}

	default:
		fmt.Fprintf(s, "%%!%c(Uint256=%s)", ch, n.String())
		return
	}

	// Determine prefix characters for the output base as needed.
	var prefix string
	if s.Flag('#') {
		switch ch {
		case 'b':
			prefix = "0b"
		case 'o':
			prefix = "0"
		case 'x':
			prefix = "0x"
		case 'X':
			prefix = "0X"
		}
	}
	if ch == 'O' {
		prefix = "0o"
	}

	// Determine how many zeros to pad with based on whether or not a minimum
	// number of digits to output is specified.
	//
	// Also, do not output anything when the minimum number of digits to output
	// is zero and the uint256 is zero.
	//
	// Note that the zero padding might also be set below when the zero pad
	// ('0') flag is specified and neither a precision nor the right justify
	// ('-') flag is specified.
	var zeroPad int
	minDigits, isPrecisionSet := s.Precision()
	if isPrecisionSet {
		switch {
		case len(digits) < minDigits:
			zeroPad = minDigits - len(digits)
		case minDigits == 0 && n.IsZero():
			return
		}
	}

	// Determine the left or right padding depending on whether or not a minimum
	// number of characters to output is specified as well as the flags.
	//
	// A '-' flag indicates the output should be right justified and takes
	// precedence over the zero pad ('0') flag.  Per the above, the zero pad
	// flag is ignored when a minimum number of digits is specified.
	var leftPad, rightPad int
	digitsPlusPrefixLen := len(prefix) + zeroPad + len(digits)
	width, isWidthSet := s.Width()
	if isWidthSet && digitsPlusPrefixLen < width {
		switch pad := width - digitsPlusPrefixLen; {
		case s.Flag('-'):
			rightPad = pad
		case s.Flag('0') && !isPrecisionSet:
			zeroPad = pad
		default:
			leftPad = pad
		}
	}

	// Produce the following output:
	//
	//  [left pad][prefix][zero pad][digits][right pad]
	var buf bytes.Buffer
	buf.Grow(leftPad + len(prefix) + zeroPad + len(digits) + rightPad)
	for i := 0; i < leftPad; i++ {
		buf.WriteRune(' ')
	}
	buf.WriteString(prefix)
	for i := 0; i < zeroPad; i++ {
		buf.WriteRune('0')
	}
	buf.Write(digits)
	for i := 0; i < rightPad; i++ {
		buf.WriteRune(' ')
	}
	s.Write(buf.Bytes())
}
