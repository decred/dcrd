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
// It currently implements support for interpreting and producing big and little
// endian bytes.
//
// Future commits will implement the primary arithmetic operations (addition,
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
