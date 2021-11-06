// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package uint256 implements highly optimized fixed precision unsigned 256-bit
// integer arithmetic.
package uint256

import "math/bits"

var (
	// zero32 is an array of 32 bytes used for the purposes of zeroing and is
	// defined here to avoid extra allocations.
	zero32 = [32]byte{}
)

// Uint256 implements high-performance, zero-allocation, unsigned 256-bit
// fixed-precision arithmetic.  All operations are performed modulo 2^256, so
// callers may rely on "wrap around" semantics.
//
// It currently implements the primary arithmetic operations (addition,
// subtraction, multiplication, squaring), comparison operations (equals, less,
// greater, cmp), interpreting and producing big and little endian bytes, and
// other convenience methods such as whether or not the value can be represented
// as a uint64 without loss of precision.
//
// Future commits will implement the primary arithmetic operations (division,
// negation), bitwise operations (lsh, rsh, not, or, and, xor), and other
// convenience methods such as determining the minimum number of bits required
// to represent the current value and text formatting with base conversion.
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
