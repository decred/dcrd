// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"encoding/binary"
	"encoding/hex"
	"math/bits"
)

// References:
//   [HAC]: Handbook of Applied Cryptography Menezes, van Oorschot, Vanstone.
//     https://cacr.uwaterloo.ca/hac/

// This file provides an alternate implementation of the secp256k1 finite field.
// It uses tight 256-bit packing with four little-endian uint64s and fully
// reduces after each operation.  Hardware intrinsics are used when available.

// FieldVal64 implements optimized fixed-precision arithmetic over the
// secp256k1 finite field.  This means all arithmetic is performed modulo
//
//	0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc2f.
//
// Unlike [FieldVal], this fully reduces after each operation and therefore does
// not require normalization or manual magnitude tracking.  It is also quite a
// bit faster than [FieldVal] on all modern 64-bit hardware.
type FieldVal64 struct {
	// Each 256-bit value is represented as 4 64-bit integers in base 2^64.
	// It only implements the arithmetic needed for elliptic curve operations.
	//
	// The following depicts the internal representation:
	// 	 --------------------------------------------------------------------
	// 	|       n[3]     |      n[2]      |      n[1]      |      n[0]      |
	// 	| 64 bits        | 64 bits        | 64 bits        | 64 bits        |
	// 	| Mult: 2^(64*3) | Mult: 2^(64*2) | Mult: 2^(64*1) | Mult: 2^(64*0) |
	// 	 --------------------------------------------------------------------
	//
	// For example, consider the number 2^87 + 1.  It would be represented as:
	// 	n[0] = 1
	// 	n[1] = 2^23
	// 	n[2] = n[3] = 0
	//
	// The full 256-bit value is then calculated by looping i from 3..0 and
	// performing sum(n[i] * 2^(64i)) as follows:
	// 	n[3] * 2^(64*3) = 0    * 2^192 = 0
	// 	n[2] * 2^(64*2) = 0    * 2^128 = 0
	// 	n[1] * 2^(64*1) = 2^23 * 2^64  = 2^87
	// 	n[0] * 2^(64*0) = 1    * 2^0   = 1
	// 	Sum: 0 + 0 + 2^87 + 1 = 2^87 + 1
	n [4]uint64
}

// Constants related to the field representation.
const (
	// field64PrimeComplement is the two's complement of the secp256k1 prime.
	field64PrimeComplement = 0x1000003d1 // 2^32 + 977

	// These fields provide convenient access to each of the limbs of the
	// secp256k1 prime in the internal field representation to improve code
	// readability.
	field64Prime0 = 0xfffffffefffffc2f
	field64Prime1 = 0xffffffffffffffff
	field64Prime2 = 0xffffffffffffffff
	field64Prime3 = 0xffffffffffffffff
)

// String returns the field value as a human-readable hex string.
func (f FieldVal64) String() string {
	return hex.EncodeToString(f.Bytes()[:])
}

// Zero sets the field value to zero in constant time.  A newly created field
// value is already set to zero.  This function can be useful to clear an
// existing field value for reuse.
func (f *FieldVal64) Zero() {
	f.n = [4]uint64{}
}

// Set sets the field value equal to the passed value in constant time.
//
// The field value is returned to support chaining.  This enables syntax like:
// f := new(FieldVal).Set(f2).Add(1) so that f = f2 + 1 where f2 is not
// modified.
func (f *FieldVal64) Set(val *FieldVal64) *FieldVal64 {
	f.n = val.n
	return f
}

// SetInt sets the field value to the passed integer in constant time.  This is
// a convenience function since it is fairly common to perform some arithmetic
// with small native integers.
//
// The field value is returned to support chaining.  This enables syntax such
// as f := new(FieldVal).SetInt(2).Mul(f2) so that f = 2 * f2.
func (f *FieldVal64) SetInt(v uint16) *FieldVal64 {
	f.n = [4]uint64{uint64(v), 0, 0, 0}
	return f
}

// SetBytes packs the passed 32-byte big-endian value into the internal field
// value representation in constant time.  It interprets the provided array as a
// 256-bit big-endian unsigned integer, packs it into the internal field value
// representation, and returns either 1 if it is greater than or equal to the
// field prime (aka it overflowed) or 0 otherwise in constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.
func (f *FieldVal64) SetBytes(b *[32]byte) uint32 {
	// Pack the 256 total bits across the 4 uint64 limbs.
	f.n[0] = binary.BigEndian.Uint64(b[24:32])
	f.n[1] = binary.BigEndian.Uint64(b[16:24])
	f.n[2] = binary.BigEndian.Uint64(b[8:16])
	f.n[3] = binary.BigEndian.Uint64(b[0:8])

	// Since f < 2^256 < 2p (where p is the secp256k1 prime), the max possible
	// number of reductions required is one.  Therefore, in the case a reduction
	// is needed, it can be performed with a single subtraction of p.
	//
	// Since p must only conditionally be subtracted when f ≥ p, the following
	// handles it in constant time by always calculating s = f - p and selecting
	// the correct case via a constant time select.

	// Subtract p with borrow propagation.  borrow is set iff f < p.
	//
	// In other words, the input overflowed (≥ p) when f - p does NOT borrow.
	//
	// s = f - p
	var s0, s1, s2, s3, borrow uint64
	s0, borrow = bits.Sub64(f.n[0], field64Prime0, 0)
	s1, borrow = bits.Sub64(f.n[1], field64Prime1, borrow)
	s2, borrow = bits.Sub64(f.n[2], field64Prime2, borrow)
	s3, borrow = bits.Sub64(f.n[3], field64Prime3, borrow)

	// Constant-time select.
	//
	// Set f = f when f < p (aka borrow is set).  Otherwise f = s = f - p.
	f.n[0] = constantTimeSelect64(borrow, f.n[0], s0)
	f.n[1] = constantTimeSelect64(borrow, f.n[1], s1)
	f.n[2] = constantTimeSelect64(borrow, f.n[2], s2)
	f.n[3] = constantTimeSelect64(borrow, f.n[3], s3)
	return uint32(1 - borrow)
}

// SetByteSlice interprets the provided slice as a 256-bit big-endian unsigned
// integer (meaning it is truncated to the first 32 bytes), packs it into the
// internal field value representation, and returns whether or not the resulting
// truncated 256-bit integer is greater than or equal to the field prime (aka it
// overflowed) in constant time.
//
// Note that since passing a slice with more than 32 bytes is truncated, it is
// possible that the truncated value is less than the field prime and hence it
// will not be reported as having overflowed in that case.  It is up to the
// caller to decide whether it needs to provide numbers of the appropriate size
// or it if is acceptable to use this function with the described truncation and
// overflow behavior.
func (f *FieldVal64) SetByteSlice(b []byte) bool {
	var b32 [32]byte
	b = b[:constantTimeMin(uint32(len(b)), 32)]
	copy(b32[:], b32[:32-len(b)])
	copy(b32[32-len(b):], b)
	result := f.SetBytes(&b32)
	zeroArray32(&b32)
	return result != 0
}

// Normalize is a no-op.  It is provided to keep API parity with [FieldVal].
func (f *FieldVal64) Normalize() {}

// PutBytesUnchecked unpacks the field value to a 32-byte big-endian value
// directly into the passed byte slice in constant time.  The target slice must
// have at least 32 bytes available or it will panic.
//
// There is a similar function, [FieldVal64.PutBytes], which unpacks the field
// value into a 32-byte array directly.  This version is provided since it can
// be useful to write directly into part of a larger buffer without needing a
// separate allocation.
func (f *FieldVal64) PutBytesUnchecked(b []byte) {
	// Unpack the 256 total bits from the 4 uint64 limbs.
	binary.BigEndian.PutUint64(b[0:8], f.n[3])
	binary.BigEndian.PutUint64(b[8:16], f.n[2])
	binary.BigEndian.PutUint64(b[16:24], f.n[1])
	binary.BigEndian.PutUint64(b[24:32], f.n[0])
}

// PutBytes unpacks the field value to a 32-byte big-endian value using the
// passed byte array in constant time.
//
// There is a similar function, [FieldVal64.PutBytesUnchecked], which unpacks
// the field value into a slice that must have at least 32 bytes available.
// This version is provided since it can be useful to write directly into an
// array that is type checked.
//
// Alternatively, there is also [FieldVal64.Bytes], which unpacks the field
// value into a new array and returns that which can sometimes be more ergonomic
// in applications that aren't concerned about an additional copy.
func (f *FieldVal64) PutBytes(b *[32]byte) {
	f.PutBytesUnchecked(b[:])
}

// Bytes unpacks the field value to a 32-byte big-endian value in constant time.
//
// See [FieldVal64.PutBytes] and [FieldVal64.PutBytesUnchecked] for variants
// that allow an array or slice to be passed which can be useful to cut down on
// the number of allocations by allowing the caller to reuse a buffer or write
// directly into part of a larger buffer.
func (f *FieldVal64) Bytes() *[32]byte {
	var b [32]byte
	f.PutBytesUnchecked(b[:])
	return &b
}

// IsZeroBit returns 1 when the field value is equal to zero or 0 otherwise in
// constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.  See IsZero for the version that returns
// a bool.
func (f *FieldVal64) IsZeroBit() uint32 {
	return constantTimeEq64(f.n[0]|f.n[1]|f.n[2]|f.n[3], 0)
}

// IsZero returns whether or not the field value is equal to zero in constant
// time.
func (f *FieldVal64) IsZero() bool {
	return (f.n[0] | f.n[1] | f.n[2] | f.n[3]) == 0
}

// IsOneBit returns 1 when the field value is equal to one or 0 otherwise in
// constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.  See [FieldVal64.IsOne] for the version
// that returns a bool.
func (f *FieldVal64) IsOneBit() uint32 {
	// The value can only be one if the single lowest significant bit is set in
	// the first word and no other bits are set in any of the other words.
	// This is a constant time implementation.
	return constantTimeEq64((f.n[0]^1)|f.n[1]|f.n[2]|f.n[3], 0)
}

// IsOne returns whether or not the field value is equal to one in constant
// time.
func (f *FieldVal64) IsOne() bool {
	// The value can only be one if the single lowest significant bit is set in
	// the first word and no other bits are set in any of the other words.
	// This is a constant time implementation.
	return ((f.n[0] ^ 1) | f.n[1] | f.n[2] | f.n[3]) == 0
}

// IsOddBit returns 1 when the field value is an odd number or 0 otherwise in
// constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.  See [FieldVal64.IsOdd] for the version
// that returns a bool.
func (f *FieldVal64) IsOddBit() uint32 {
	// Only odd numbers have the bottom bit set.
	return uint32(f.n[0] & 1)
}

// IsOdd returns whether or not the field value is an odd number in constant
// time.
func (f *FieldVal64) IsOdd() bool {
	// Only odd numbers have the bottom bit set.
	return f.n[0]&1 == 1
}

// Equals returns whether or not the two field values are the same in constant
// time.
func (f *FieldVal64) Equals(val *FieldVal64) bool {
	// Xor only sets bits when they are different, so the two field values
	// can only be the same if no bits are set after xoring each word.
	// This is a constant time implementation.
	return ((f.n[0] ^ val.n[0]) | (f.n[1] ^ val.n[1]) | (f.n[2] ^ val.n[2]) |
		(f.n[3] ^ val.n[3])) == 0
}

// NegateVal negates the passed value and stores the result in f in constant
// time.  The ignored parameter exists to keep API parity with [FieldVal].
//
// The field value is returned to support chaining.  This enables syntax like:
// f.NegateVal(f2).AddInt(1) so that f = -f2 + 1.
func (f *FieldVal64) NegateVal(val *FieldVal64, _ uint32) *FieldVal64 {
	// Since the value is already in the range 0 ≤ val < p, where p is the
	// secp256k1 prime, negation modulo p is just p - val.  This implies that
	// the result will always be in the desired range with the sole exception of
	// 0 because p - 0 = p itself.
	//
	// The following handles that case in constant time by creating a mask that
	// is all 0s in the case the value being negated is 0 and all 1s otherwise
	// and then bitwise ands that mask with each word of the prime.

	// Subtract val from 0. borrow is set iff val != 0.
	//
	// t = 0 - val = -val
	var t0, t1, t2, t3, borrow uint64
	t0, borrow = bits.Sub64(0, val.n[0], 0)
	t1, borrow = bits.Sub64(0, val.n[1], borrow)
	t2, borrow = bits.Sub64(0, val.n[2], borrow)
	t3, borrow = bits.Sub64(0, val.n[3], borrow)

	// Mask the prime with the borrow (p when val != 0, else 0).
	//
	// The upper limbs of the prime are all 1s, so there is no need to mask them
	// given they are equal to the mask for both cases.
	mask := -borrow
	maskedPrime0 := field64Prime0 & mask

	// Add 0 when val == 0 or p when val != 0.  The result is either:
	//
	// val == 0: f = 0 + 0 = 0
	// val != 0: f = -val + p = p - val
	var carry uint64
	f.n[0], carry = bits.Add64(t0, maskedPrime0, 0)
	f.n[1], carry = bits.Add64(t1, mask, carry)
	f.n[2], carry = bits.Add64(t2, mask, carry)
	f.n[3], _ = bits.Add64(t3, mask, carry)
	return f
}

// Negate negates the field value in constant time.  The existing field value is
// modified.  The ignored parameter exists to keep API parity with [FieldVal].
//
// The field value is returned to support chaining.  This enables syntax like:
// f.Negate().AddInt(1) so that f = -f + 1.
func (f *FieldVal64) Negate(_ uint32) *FieldVal64 {
	return f.NegateVal(f, 0)
}

// AddInt adds the passed integer to the existing field value and stores the
// result in f in constant time.  This is a convenience function since it is
// fairly common to perform some arithmetic with small native integers.
//
// The field value is returned to support chaining.  This enables syntax like:
// f.AddInt(1).Add(f2) so that f = f + 1 + f2.
func (f *FieldVal64) AddInt(ui uint16) *FieldVal64 {
	return f.Add(new(FieldVal64).SetInt(ui))
}

// Add adds the passed value to the existing field value and stores the result
// in f in constant time.
//
// The field value is returned to support chaining.  This enables syntax like:
// f.Add(f2).AddInt(1) so that f = f + f2 + 1.
func (f *FieldVal64) Add(val *FieldVal64) *FieldVal64 {
	return f.Add2(f, val)
}

// Add2 adds the passed two field values together and stores the result in f in
// constant time.
//
// The field value is returned to support chaining.  This enables syntax like:
// f3.Add2(f, f2).AddInt(1) so that f3 = f + f2 + 1.
func (f *FieldVal64) Add2(a, b *FieldVal64) *FieldVal64 {
	// Since both values are already in the range 0 ≤ val < p (where p is the
	// secp256k1 prime), the maximum possible result is < 2p - 1.  So a maximum
	// of one subtraction of p is required in the worst case.
	//
	// Since p must only conditionally be subtracted when a+b ≥ p, the following
	// handles it in constant time by calculating both t = a+b and s = a+b - p
	// and selecting the correct case via a constant time select.

	// Add with carry propagation.  overflow is set iff t = a+b ≥ 2^256.
	//
	// t = a + b
	var t0, t1, t2, t3, overflow, carry uint64
	t0, carry = bits.Add64(a.n[0], b.n[0], 0)
	t1, carry = bits.Add64(a.n[1], b.n[1], carry)
	t2, carry = bits.Add64(a.n[2], b.n[2], carry)
	t3, overflow = bits.Add64(a.n[3], b.n[3], carry)

	// Subtract p with borrow propagation.  borrow is set iff t = a+b < p.
	//
	// s = t - p = a+b - p
	var s0, s1, s2, s3, borrow uint64
	s0, borrow = bits.Sub64(t0, field64Prime0, 0)
	s1, borrow = bits.Sub64(t1, field64Prime1, borrow)
	s2, borrow = bits.Sub64(t2, field64Prime2, borrow)
	s3, borrow = bits.Sub64(t3, field64Prime3, borrow)

	// Constant-time select.
	//
	// Set f = t = a+b only when there was no overflow and t < p (borrow set).
	// Otherwise f = s = a+b - p.
	cond := (1 - overflow) & borrow
	f.n[0] = constantTimeSelect64(cond, t0, s0)
	f.n[1] = constantTimeSelect64(cond, t1, s1)
	f.n[2] = constantTimeSelect64(cond, t2, s2)
	f.n[3] = constantTimeSelect64(cond, t3, s3)
	return f
}

// MulBy2 multiplies the field value by 2 and stores the result in f in constant
// time.
//
// This method is optimized to provide a significant speed advantage over the
// more general [FieldVal64.MulInt].
//
// The field value is returned to support chaining.  This enables syntax like:
// f.MulBy2().Add(f2) so that f = 2 * f + f2.
func (f *FieldVal64) MulBy2() *FieldVal64 {
	return f.Add(f)
}

// MulBy3 multiplies the field value by 3 and stores the result in f in constant
// time.
//
// This method is optimized to provide a significant speed advantage over the
// more general [FieldVal64.MulInt].
//
// The field value is returned to support chaining.  This enables syntax like:
// f.MulBy3().Add(f2) so that f = 3 * f + f2.
func (f *FieldVal64) MulBy3() *FieldVal64 {
	var orig FieldVal64
	orig.Set(f)
	return f.MulBy2().Add(&orig)
}

// MulBy4 multiplies the field value by 4 and stores the result in f in constant
// time.
//
// This method is optimized to provide a significant speed advantage over the
// more general [FieldVal64.MulInt].
//
// The field value is returned to support chaining.  This enables syntax like:
// f.MulBy4().Add(f2) so that f = 4 * f + f2.
func (f *FieldVal64) MulBy4() *FieldVal64 {
	return f.MulBy2().MulBy2()
}

// MulBy8 multiplies the field value by 8 and stores the result in f in constant
// time.
//
// This method is optimized to provide a significant speed advantage over the
// more general [FieldVal64.MulInt].
//
// The field value is returned to support chaining.  This enables syntax like:
// f.MulBy8().Add(f2) so that f = 8 * f + f2.
func (f *FieldVal64) MulBy8() *FieldVal64 {
	return f.MulBy4().MulBy2()
}

// MulInt multiplies the field value by the passed int and stores the result in
// f in constant time.
//
// Callers should prefer using the faster specialized methods for multiplying by
// 2, 3, 4, and 8, as they are commonly used in curve equations.
//
// See [FieldVal64.MulBy2], [FieldVal64.MulBy3], [FieldVal64.MulBy4], and
// [FieldVal64.MulBy8] for the aforementioned optimized methods.
//
// The field value is returned to support chaining.  This enables syntax like:
// f.MulInt(15).Add(f2) so that f = 15 * f + f2.
func (f *FieldVal64) MulInt(val uint8) *FieldVal64 {
	return f.Mul(new(FieldVal64).SetInt(uint16(val)))
}

// Mul multiplies the passed value to the existing field value and stores the
// result in f in constant time.
//
// The field value is returned to support chaining.  This enables syntax like:
// f.Mul(f2).AddInt(1) so that f = (f * f2) + 1.
func (f *FieldVal64) Mul(val *FieldVal64) *FieldVal64 {
	return f.Mul2(f, val)
}

// Mul2 multiplies the passed two field values together and stores the result in
// f in constant time.
//
// The field value is returned to support chaining.  This enables syntax like:
// f3.Mul2(f, f2).AddInt(1) so that f3 = (f * f2) + 1.
func (f *FieldVal64) Mul2(a, b *FieldVal64) *FieldVal64 {
	field64Mul(&f.n, &a.n, &b.n)
	return f
}

// SquareRootVal either calculates the square root of the passed value when it
// exists or the square root of the negation of the value when it does not exist
// and stores the result in f in constant time.  The return flag is true when
// the calculated square root is for the passed value itself and false when it
// is for its negation.
func (f *FieldVal64) SquareRootVal(val *FieldVal64) bool {
	// This uses the Tonelli-Shanks method for calculating the square root of
	// the value when it exists.  The key principles of the method follow.
	//
	// Fermat's little theorem states that for a nonzero number 'a' and prime
	// 'p', a^(p-1) ≡ 1 (mod p).
	//
	// Further, Euler's criterion states that an integer 'a' has a square root
	// (aka is a quadratic residue) modulo a prime if a^((p-1)/2) ≡ 1 (mod p)
	// and, conversely, when it does NOT have a square root (aka 'a' is a
	// non-residue) a^((p-1)/2) ≡ -1 (mod p).
	//
	// This can be seen by considering that Fermat's little theorem can be
	// written as (a^((p-1)/2) - 1)(a^((p-1)/2) + 1) ≡ 0 (mod p).  Therefore,
	// one of the two factors must be 0.  Then, when a ≡ x^2 (aka 'a' is a
	// quadratic residue), (x^2)^((p-1)/2) ≡ x^(p-1) ≡ 1 (mod p) which implies
	// the first factor must be zero.  Finally, per Lagrange's theorem, the
	// non-residues are the only remaining possible solutions and thus must make
	// the second factor zero to satisfy Fermat's little theorem implying that
	// a^((p-1)/2) ≡ -1 (mod p) for that case.
	//
	// The Tonelli-Shanks method uses these facts along with factoring out
	// powers of two to solve a congruence that results in either the solution
	// when the square root exists or the square root of the negation of the
	// value when it does not.  In the case of primes that are ≡ 3 (mod 4), the
	// possible solutions are r = ±a^((p+1)/4) (mod p).  Therefore, either r^2 ≡
	// a (mod p) is true in which case ±r are the two solutions, or r^2 ≡ -a
	// (mod p) in which case 'a' is a non-residue and there are no solutions.
	//
	// The secp256k1 prime is ≡ 3 (mod 4), so this result applies.
	//
	// In other words, calculate a^((p+1)/4) and then square it and check it
	// against the original value to determine if it is actually the square
	// root.
	//
	// In order to efficiently compute a^((p+1)/4), (p+1)/4 needs to be split
	// into a sequence of squares and multiplications that minimizes the number
	// of multiplications needed (since they are more costly than squarings).
	//
	// The secp256k1 prime + 1 / 4 is 2^254 - 2^30 - 244.  In binary, that is:
	//
	// 00111111 11111111 11111111 11111111
	// 11111111 11111111 11111111 11111111
	// 11111111 11111111 11111111 11111111
	// 11111111 11111111 11111111 11111111
	// 11111111 11111111 11111111 11111111
	// 11111111 11111111 11111111 11111111
	// 11111111 11111111 11111111 11111111
	// 10111111 11111111 11111111 00001100
	//
	// Notice that can be broken up into three windows of consecutive 1s (in
	// order of least to most significant) as:
	//
	//   6-bit window with two bits set (bits 4, 5, 6, 7 unset)
	//   23-bit window with 22 bits set (bit 30 unset)
	//   223-bit window with all 223 bits set
	//
	// Thus, the groups of 1 bits in each window forms the set:
	// S = {2, 22, 223}.
	//
	// The strategy is to calculate a^(2^n - 1) for each grouping via an
	// addition chain with a sliding window.
	//
	// The addition chain used is (credits to Peter Dettman):
	// (0,0),(1,0),(2,2),(3,2),(4,1),(5,5),(6,6),(7,7),(8,8),(9,7),(10,2)
	// => 2^1 2^[2] 2^3 2^6 2^9 2^11 2^[22] 2^44 2^88 2^176 2^220 2^[223]
	//
	// This has a cost of 254 field squarings and 13 field multiplications.
	var a, a2, a3, a6, a9, a11, a22, a44, a88, a176, a220, a223 FieldVal64
	a.Set(val)
	a2.SquareVal(&a).Mul(&a)                                  // a2 = a^(2^2 - 1)
	a3.SquareVal(&a2).Mul(&a)                                 // a3 = a^(2^3 - 1)
	a6.SquareVal(&a3).Square().Square()                       // a6 = a^(2^6 - 2^3)
	a6.Mul(&a3)                                               // a6 = a^(2^6 - 1)
	a9.SquareVal(&a6).Square().Square()                       // a9 = a^(2^9 - 2^3)
	a9.Mul(&a3)                                               // a9 = a^(2^9 - 1)
	a11.SquareVal(&a9).Square()                               // a11 = a^(2^11 - 2^2)
	a11.Mul(&a2)                                              // a11 = a^(2^11 - 1)
	a22.SquareVal(&a11).Square().Square().Square().Square()   // a22 = a^(2^16 - 2^5)
	a22.Square().Square().Square().Square().Square()          // a22 = a^(2^21 - 2^10)
	a22.Square()                                              // a22 = a^(2^22 - 2^11)
	a22.Mul(&a11)                                             // a22 = a^(2^22 - 1)
	a44.SquareVal(&a22).Square().Square().Square().Square()   // a44 = a^(2^27 - 2^5)
	a44.Square().Square().Square().Square().Square()          // a44 = a^(2^32 - 2^10)
	a44.Square().Square().Square().Square().Square()          // a44 = a^(2^37 - 2^15)
	a44.Square().Square().Square().Square().Square()          // a44 = a^(2^42 - 2^20)
	a44.Square().Square()                                     // a44 = a^(2^44 - 2^22)
	a44.Mul(&a22)                                             // a44 = a^(2^44 - 1)
	a88.SquareVal(&a44).Square().Square().Square().Square()   // a88 = a^(2^49 - 2^5)
	a88.Square().Square().Square().Square().Square()          // a88 = a^(2^54 - 2^10)
	a88.Square().Square().Square().Square().Square()          // a88 = a^(2^59 - 2^15)
	a88.Square().Square().Square().Square().Square()          // a88 = a^(2^64 - 2^20)
	a88.Square().Square().Square().Square().Square()          // a88 = a^(2^69 - 2^25)
	a88.Square().Square().Square().Square().Square()          // a88 = a^(2^74 - 2^30)
	a88.Square().Square().Square().Square().Square()          // a88 = a^(2^79 - 2^35)
	a88.Square().Square().Square().Square().Square()          // a88 = a^(2^84 - 2^40)
	a88.Square().Square().Square().Square()                   // a88 = a^(2^88 - 2^44)
	a88.Mul(&a44)                                             // a88 = a^(2^88 - 1)
	a176.SquareVal(&a88).Square().Square().Square().Square()  // a176 = a^(2^93 - 2^5)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^98 - 2^10)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^103 - 2^15)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^108 - 2^20)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^113 - 2^25)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^118 - 2^30)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^123 - 2^35)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^128 - 2^40)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^133 - 2^45)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^138 - 2^50)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^143 - 2^55)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^148 - 2^60)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^153 - 2^65)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^158 - 2^70)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^163 - 2^75)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^168 - 2^80)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^173 - 2^85)
	a176.Square().Square().Square()                           // a176 = a^(2^176 - 2^88)
	a176.Mul(&a88)                                            // a176 = a^(2^176 - 1)
	a220.SquareVal(&a176).Square().Square().Square().Square() // a220 = a^(2^181 - 2^5)
	a220.Square().Square().Square().Square().Square()         // a220 = a^(2^186 - 2^10)
	a220.Square().Square().Square().Square().Square()         // a220 = a^(2^191 - 2^15)
	a220.Square().Square().Square().Square().Square()         // a220 = a^(2^196 - 2^20)
	a220.Square().Square().Square().Square().Square()         // a220 = a^(2^201 - 2^25)
	a220.Square().Square().Square().Square().Square()         // a220 = a^(2^206 - 2^30)
	a220.Square().Square().Square().Square().Square()         // a220 = a^(2^211 - 2^35)
	a220.Square().Square().Square().Square().Square()         // a220 = a^(2^216 - 2^40)
	a220.Square().Square().Square().Square()                  // a220 = a^(2^220 - 2^44)
	a220.Mul(&a44)                                            // a220 = a^(2^220 - 1)
	a223.SquareVal(&a220).Square().Square()                   // a223 = a^(2^223 - 2^3)
	a223.Mul(&a3)                                             // a223 = a^(2^223 - 1)

	f.SquareVal(&a223).Square().Square().Square().Square() // f = a^(2^228 - 2^5)
	f.Square().Square().Square().Square().Square()         // f = a^(2^233 - 2^10)
	f.Square().Square().Square().Square().Square()         // f = a^(2^238 - 2^15)
	f.Square().Square().Square().Square().Square()         // f = a^(2^243 - 2^20)
	f.Square().Square().Square()                           // f = a^(2^246 - 2^23)
	f.Mul(&a22)                                            // f = a^(2^246 - 2^22 - 1)
	f.Square().Square().Square().Square().Square()         // f = a^(2^251 - 2^27 - 2^5)
	f.Square()                                             // f = a^(2^252 - 2^28 - 2^6)
	f.Mul(&a2)                                             // f = a^(2^252 - 2^28 - 2^6 - 2^1 - 1)
	f.Square().Square()                                    // f = a^(2^254 - 2^30 - 244) = a^((p+1)/4)

	// Verify the result is actually the square root by squaring it and checking
	// against the original value.
	var sqr FieldVal64
	return sqr.SquareVal(f).Equals(val)
}

// Square squares the field value in constant time.  The existing field value is
// modified.
//
// The field value is returned to support chaining.  This enables syntax like:
// f.Square().Mul(f2) so that f = f^2 * f2.
func (f *FieldVal64) Square() *FieldVal64 {
	return f.SquareVal(f)
}

// SquareVal squares the passed value and stores the result in f in constant
// time.
//
// The field value is returned to support chaining.  This enables syntax like:
// f3.SquareVal(f).Mul(f) so that f3 = f^2 * f = f^3.
func (f *FieldVal64) SquareVal(val *FieldVal64) *FieldVal64 {
	field64Square(&f.n, &val.n)
	return f
}

// field64Mul512 sets t = x * y as an unreduced 512-bit product via a row-by-row
// schoolbook multiply.
func field64Mul512(t *[8]uint64, x, y *[4]uint64) {
	// The intermediate bounds and carry assumptions used by this algorithm have
	// been formally verified.  The verification artifacts are available in
	// internal/proofs.

	a0, a1, a2, a3 := x[0], x[1], x[2], x[3]
	b0, b1, b2, b3 := y[0], y[1], y[2], y[3]

	var c uint64

	// Row 0: p0..p4 = a * b0.
	//
	// Note that since h3 is the upper 64 bits of the product of two uint64s:
	//   h3 ≤ floor((2^64-1)^2 / 2^64) = 2^64 - 2
	//
	// Without any other considerations, c ≤ 1, so a loose bound is:
	//   p4 ≤ h3 + 1 = 2^64 - 1 < 2^64
	//
	// This already shows that the carryless add in p4 is safe, however, a tight
	// upper bound is more useful to prove no overflow is possible in the upper
	// words of the subsequent rows.
	//
	// Claim: p4 ≤ 2^64 - 2
	//
	// Consider the row product A*b, where A ≤ 2^256 - 1, b ≤ 2^64 - 1, then:
	//   A*b ≤ (2^256 - 1)(2^64 - 1) = 2^320 - 2^256 - 2^64 + 1
	//
	// Next, expressing the product in base 2^256 gives:
	//   A*b = p4*2^256 + qlow
	//
	// Where qlow is the low 256 bits of the product and p4 is the integer
	// quotient:
	//   p4 = floor(A*b / 2^256)
	//   qlow = A*b (mod 2^256)
	//
	// Finally, bound the quotient:
	//   p4 = floor(A*b / 2^256)
	//      ≤ floor((2^320 - 2^256 - 2^64 + 1) / 2^256)
	//      = floor(2^64 - 1 - 2^(-192) + 2^(-256))
	//      ≤ 2^64 - 2
	//
	// So, p4 ≤ 2^64 - 2.
	h0, p0 := bits.Mul64(a0, b0)
	h1, p1 := bits.Mul64(a1, b0)
	h2, p2 := bits.Mul64(a2, b0)
	h3, p3 := bits.Mul64(a3, b0)
	p1, c = bits.Add64(p1, h0, 0)
	p2, c = bits.Add64(p2, h1, c)
	p3, c = bits.Add64(p3, h2, c)
	p4 := h3 + c

	// Row 1: p1..p5 += a * b1.
	//
	// Per row 0 above, the tight bound on q4 for this row is:
	//   q4 ≤ 2^64 - 2
	//
	// Since c ≤ 1:
	//   p5 ≤ q4 + 1 = 2^64 - 1 < 2^64
	//
	// So, the carryless add in p5 is safe.
	h0, q0 := bits.Mul64(a0, b1)
	h1, q1 := bits.Mul64(a1, b1)
	h2, q2 := bits.Mul64(a2, b1)
	h3, q3 := bits.Mul64(a3, b1)
	q1, c = bits.Add64(q1, h0, 0)
	q2, c = bits.Add64(q2, h1, c)
	q3, c = bits.Add64(q3, h2, c)
	q4 := h3 + c
	p1, c = bits.Add64(p1, q0, 0)
	p2, c = bits.Add64(p2, q1, c)
	p3, c = bits.Add64(p3, q2, c)
	p4, c = bits.Add64(p4, q3, c)
	p5 := q4 + c

	// Row 2: p2..p6 += a * b2.
	//
	// The same bounds calculation as row 1 applies.
	h0, q0 = bits.Mul64(a0, b2)
	h1, q1 = bits.Mul64(a1, b2)
	h2, q2 = bits.Mul64(a2, b2)
	h3, q3 = bits.Mul64(a3, b2)
	q1, c = bits.Add64(q1, h0, 0)
	q2, c = bits.Add64(q2, h1, c)
	q3, c = bits.Add64(q3, h2, c)
	q4 = h3 + c
	p2, c = bits.Add64(p2, q0, 0)
	p3, c = bits.Add64(p3, q1, c)
	p4, c = bits.Add64(p4, q2, c)
	p5, c = bits.Add64(p5, q3, c)
	p6 := q4 + c

	// Row 3: p3..p7 += a * b3.
	//
	// The same bounds calculation as row 1 applies.
	h0, q0 = bits.Mul64(a0, b3)
	h1, q1 = bits.Mul64(a1, b3)
	h2, q2 = bits.Mul64(a2, b3)
	h3, q3 = bits.Mul64(a3, b3)
	q1, c = bits.Add64(q1, h0, 0)
	q2, c = bits.Add64(q2, h1, c)
	q3, c = bits.Add64(q3, h2, c)
	q4 = h3 + c
	p3, c = bits.Add64(p3, q0, 0)
	p4, c = bits.Add64(p4, q1, c)
	p5, c = bits.Add64(p5, q2, c)
	p6, c = bits.Add64(p6, q3, c)
	p7 := q4 + c

	t[0], t[1], t[2], t[3] = p0, p1, p2, p3
	t[4], t[5], t[6], t[7] = p4, p5, p6, p7
}

// field64Square512 sets t = a^2 as an unreduced 512-bit product.
func field64Square512(t *[8]uint64, a *[4]uint64) {
	// The intermediate bounds and carry assumptions used by this algorithm have
	// been formally verified.  The verification artifacts are available in
	// internal/proofs.

	a0, a1, a2, a3 := a[0], a[1], a[2], a[3]

	var c uint64

	// Off-diagonal upper-triangle products (not yet doubled).
	//
	// Note that since h03 is the upper 64 bits of the product of two uint64s:
	//   h03 ≤ floor((2^64-1)^2 / 2^64) = 2^64 - 2
	//
	// Then, because c ≤ 1, a loose bound is:
	//   p4 ≤ h03 + 1 = 2^64 - 1 < 2^64
	//
	// Therefore, it is safe to discard the carry.
	p2, p1 := bits.Mul64(a0, a1)
	h02, l02 := bits.Mul64(a0, a2)
	h03, l03 := bits.Mul64(a0, a3)
	p2, c = bits.Add64(p2, l02, 0)
	p3, c := bits.Add64(h02, l03, c)
	p4, _ := bits.Add64(h03, 0, c)

	h12, l12 := bits.Mul64(a1, a2)
	p3, c = bits.Add64(p3, l12, 0)
	p4, c = bits.Add64(p4, h12, c)
	p5 := c

	// The p5 carry is safe to discard because p5 + h13 + c ≤ 2^64 - 1 (where c
	// is the carry from p4 + l13).
	//
	// A full proof involves case work that is omitted here, but the key point
	// is that the only way the final add could have a carry is if all 3 of the
	// following conditions were simultaneously true:
	//
	// 1) p5_old = 1 (the carry from the earlier chain, so ≤ 1)
	// 2) h13 = 2^64 - 2 (h13 ≤ 2^64 - 2 as proven previously)
	// 3) c = 1 (implies p4 + l13 ≥ 2^64)
	//
	// However, that combination of conditions is impossible because in order
	// for condition 2 to be true, a1 = a3 = 2^64  - 1, in which case l13 = 1
	// and so in order for condition 3 to also be true, p4 = 2^64 - 1.  But then
	// the combination of those conditions forces p5_old = 0.
	h13, l13 := bits.Mul64(a1, a3)
	p4, c = bits.Add64(p4, l13, 0)
	p5, _ = bits.Add64(p5, h13, c)

	// Similarly, the p6 carry is safe to discard because, per above:
	//   h23 ≤ 2^64 - 2
	//
	// Then, again c ≤ 1, so the same loose bound applies:
	//   p6 ≤ h23 + 1 = 2^64 - 1 < 2^64
	h23, l23 := bits.Mul64(a2, a3)
	p5, c = bits.Add64(p5, l23, 0)
	p6, _ := bits.Add64(h23, 0, c)

	// Double p1..p6, capturing the top carry into p7.
	p1, c = bits.Add64(p1, p1, 0)
	p2, c = bits.Add64(p2, p2, c)
	p3, c = bits.Add64(p3, p3, c)
	p4, c = bits.Add64(p4, p4, c)
	p5, c = bits.Add64(p5, p5, c)
	p6, c = bits.Add64(p6, p6, c)
	p7 := c

	// Add the diagonal squares a[i]^2 at columns 0,2,4,6 in one carry chain.
	//
	// The carry on the final add is safe to discard because a < p < 2^256, so:
	//   (2^256 - 1)^2 = 2^512 - 2^257 + 1 < 2^512
	h0, p0 := bits.Mul64(a0, a0)
	h1, l1 := bits.Mul64(a1, a1)
	h2, l2 := bits.Mul64(a2, a2)
	h3, l3 := bits.Mul64(a3, a3)
	p1, c = bits.Add64(p1, h0, 0)
	p2, c = bits.Add64(p2, l1, c)
	p3, c = bits.Add64(p3, h1, c)
	p4, c = bits.Add64(p4, l2, c)
	p5, c = bits.Add64(p5, h2, c)
	p6, c = bits.Add64(p6, l3, c)
	p7, _ = bits.Add64(p7, h3, c)

	t[0], t[1], t[2], t[3] = p0, p1, p2, p3
	t[4], t[5], t[6], t[7] = p4, p5, p6, p7
}

// field64Reduce512 reduces a 512-bit little-endian limb array modulo p in
// constant time and stores the result in r.
func field64Reduce512(r *[4]uint64, x *[8]uint64) {
	// The intermediate bounds and carry assumptions used by this algorithm have
	// been formally verified.  The verification artifacts are available in
	// internal/proofs.

	// Per [HAC] section 14.3.4: Reduction method of moduli of special form,
	// when the modulus is of the special form m = b^t - c, highly efficient
	// reduction can be achieved.  While [HAC] only presents the algorithm and
	// does not call it out by name or provide the mathematical justification,
	// the underlying technique is known as Crandall reduction and is often
	// presented as 2^k - c.  It is easy to see they are equivalent by setting
	// b = 2 and t = k.
	//
	// The secp256k1 prime is 2^256 - 4294968273, so it fits this criteria where
	// k=256, and c = 4294968273 = 2^32 + 977.
	//
	// Crandall reduction works by taking advantage of the fact that if a prime
	// is of the form 2^k - c, then 2^k - c ≡ 0 (mod p), so 2^k ≡ c (mod p).  In
	// other words, every multiple of 2^k is equivalent to adding c when working
	// modulo p.
	//
	// Since the 512-bit value to reduce is tightly packed into uint64s, the
	// upper 4 limbs are all multiples of 2^256.  Therefore, reducing modulo the
	// prime is equivalent to multiplying those upper limbs by c and adding the
	// result to the corresponding lower 4 limbs while propagating the carries.
	//
	// For the specific case of the secp256k1 prime, a max of 3 reductions are
	// required because c is 33 bits and so the first round will reduce from 512
	// bits to a max of 256 + 33 = 289 bits and the second round will reduce to
	// within 2p.  Then, a conditional subtraction of p handles the final
	// reduction.

	var t0, t1, t2, t3, t4, h, lo, hi, carry uint64

	h, t0 = bits.Mul64(x[4], field64PrimeComplement)

	// Note that since hi is the upper 64 bits of the product of two uint64s:
	//   h3 ≤ floor((2^64-1)^2 / 2^64) = 2^64 - 2
	//
	// Then, because c ≤ 1, a loose bound is:
	//   p4 ≤ h3 + 1 = 2^64 - 1 < 2^64
	//
	// Therefore, it is safe to discard the carry and the same applies to the
	// next two limbs.
	hi, lo = bits.Mul64(x[5], field64PrimeComplement)
	t1, carry = bits.Add64(lo, h, 0)
	h, _ = bits.Add64(hi, 0, carry)

	hi, lo = bits.Mul64(x[6], field64PrimeComplement)
	t2, carry = bits.Add64(lo, h, 0)
	h, _ = bits.Add64(hi, 0, carry)

	hi, lo = bits.Mul64(x[7], field64PrimeComplement)
	t3, carry = bits.Add64(lo, h, 0)
	t4, _ = bits.Add64(hi, 0, carry)

	t0, carry = bits.Add64(t0, x[0], 0)
	t1, carry = bits.Add64(t1, x[1], carry)
	t2, carry = bits.Add64(t2, x[2], carry)
	t3, carry = bits.Add64(t3, x[3], carry)
	t4 += carry

	// The value now fits in 289 bits, so reduce it again.  Only the fifth limb
	// (t4) needs to be considered since all of the higher limbs are ≥ 320 bits
	// and thus guaranteed to be 0.
	h, t4 = bits.Mul64(t4, field64PrimeComplement)

	t0, carry = bits.Add64(t0, t4, 0)
	t1, carry = bits.Add64(t1, h, carry)
	t2, carry = bits.Add64(t2, 0, carry)
	t3, carry = bits.Add64(t3, 0, carry)

	// The second fold can carry out of t3.  Keep it as a fifth limb (t4) and
	// let the conditional subtract resolve it: the value is < 2p, so one 5-limb
	// subtract of p fully reduces it.
	t4 = carry

	var s0, s1, s2, s3, borrow uint64
	s0, borrow = bits.Sub64(t0, field64Prime0, 0)
	s1, borrow = bits.Sub64(t1, field64Prime1, borrow)
	s2, borrow = bits.Sub64(t2, field64Prime2, borrow)
	s3, borrow = bits.Sub64(t3, field64Prime3, borrow)
	_, borrow = bits.Sub64(t4, 0, borrow)
	r[0] = constantTimeSelect64(borrow, t0, s0)
	r[1] = constantTimeSelect64(borrow, t1, s1)
	r[2] = constantTimeSelect64(borrow, t2, s2)
	r[3] = constantTimeSelect64(borrow, t3, s3)
}

// field64Mul sets r = a * b (mod p).
func field64Mul(r *[4]uint64, a, b *[4]uint64) {
	var product [8]uint64
	field64Mul512(&product, a, b)
	field64Reduce512(r, &product)
}

// field64Square sets r = a^2 (mod p).
func field64Square(r *[4]uint64, a *[4]uint64) {
	var product [8]uint64
	field64Square512(&product, a)
	field64Reduce512(r, &product)
}

// Inverse finds the modular multiplicative inverse of the field value in
// constant time.  The existing field value is modified.
//
// The field value is returned to support chaining.  This enables syntax like:
// f.Inverse().Mul(f2) so that f = f^-1 * f2.
func (f *FieldVal64) Inverse() *FieldVal64 {
	// Fermat's little theorem states that for a nonzero number 'a' and prime
	// 'p', a^(p-1) ≡ 1 (mod p).  Multiplying both sides of the equation by the
	// multiplicative inverse a^-1 yields a^(p-2) ≡ a^-1 (mod p).  Thus, a^(p-2)
	// is the multiplicative inverse.
	//
	// In order to efficiently compute a^(p-2), p-2 needs to be split into a
	// sequence of squares and multiplications that minimizes the number of
	// multiplications needed (since they are more costly than squarings).
	// Intermediate results are saved and reused as well.
	//
	// The secp256k1 prime - 2 is 2^256 - 4294968275.  In binary, that is:
	//
	// 11111111 11111111 11111111 11111111
	// 11111111 11111111 11111111 11111111
	// 11111111 11111111 11111111 11111111
	// 11111111 11111111 11111111 11111111
	// 11111111 11111111 11111111 11111111
	// 11111111 11111111 11111111 11111111
	// 11111111 11111111 11111111 11111110
	// 11111111 11111111 11111100 00101101
	//
	// Notice that can be broken up into five windows of consecutive 1s (in
	// order of least to most significant) as:
	//
	//   2-bit window with 1 bit set (bit 1 unset)
	//   3-bit window with 2 bits set (bit 4 unset)
	//   5-bit window with 1 bit set (bits 6, 7, 8, 9 unset)
	//   23-bit window with 22 bits set (bit 32 unset)
	//   223-bit window with all 223 bits set
	//
	// Thus, the groups of 1 bits in each window forms the set:
	// S = {1, 2, 22, 223}.
	//
	// The strategy is to calculate a^(2^n - 1) for each grouping via an
	// addition chain with a sliding window.
	//
	// The addition chain used is (credits to Peter Dettman):
	// (0,0),(1,0),(2,2),(3,2),(4,1),(5,5),(6,6),(7,7),(8,8),(9,7),(10,2)
	// => 2^[1] 2^[2] 2^3 2^6 2^9 2^11 2^[22] 2^44 2^88 2^176 2^220 2^[223]
	//
	// This has a cost of 255 field squarings and 15 field multiplications.
	var a, a2, a3, a6, a9, a11, a22, a44, a88, a176, a220, a223 FieldVal64
	a.Set(f)
	a2.SquareVal(&a).Mul(&a)                                  // a2  = a^(2^2 - 1)
	a3.SquareVal(&a2).Mul(&a)                                 // a3  = a^(2^3 - 1)
	a6.SquareVal(&a3).Square().Square()                       // a6 = a^(2^6 - 2^3)
	a6.Mul(&a3)                                               // a6 = a^(2^6 - 1)
	a9.SquareVal(&a6).Square().Square()                       // a9 = a^(2^9 - 2^3)
	a9.Mul(&a3)                                               // a9 = a^(2^9 - 1)
	a11.SquareVal(&a9).Square()                               // a11 = a^(2^11 - 2^2)
	a11.Mul(&a2)                                              // a11 = a^(2^11 - 1)
	a22.SquareVal(&a11).Square().Square().Square().Square()   // a22 = a^(2^16 - 2^5)
	a22.Square().Square().Square().Square().Square()          // a22 = a^(2^21 - 2^10)
	a22.Square()                                              // a22 = a^(2^22 - 2^11)
	a22.Mul(&a11)                                             // a22 = a^(2^22 - 1)
	a44.SquareVal(&a22).Square().Square().Square().Square()   // a44 = a^(2^27 - 2^5)
	a44.Square().Square().Square().Square().Square()          // a44 = a^(2^32 - 2^10)
	a44.Square().Square().Square().Square().Square()          // a44 = a^(2^37 - 2^15)
	a44.Square().Square().Square().Square().Square()          // a44 = a^(2^42 - 2^20)
	a44.Square().Square()                                     // a44 = a^(2^44 - 2^22)
	a44.Mul(&a22)                                             // a44 = a^(2^44 - 1)
	a88.SquareVal(&a44).Square().Square().Square().Square()   // a88 = a^(2^49 - 2^5)
	a88.Square().Square().Square().Square().Square()          // a88 = a^(2^54 - 2^10)
	a88.Square().Square().Square().Square().Square()          // a88 = a^(2^59 - 2^15)
	a88.Square().Square().Square().Square().Square()          // a88 = a^(2^64 - 2^20)
	a88.Square().Square().Square().Square().Square()          // a88 = a^(2^69 - 2^25)
	a88.Square().Square().Square().Square().Square()          // a88 = a^(2^74 - 2^30)
	a88.Square().Square().Square().Square().Square()          // a88 = a^(2^79 - 2^35)
	a88.Square().Square().Square().Square().Square()          // a88 = a^(2^84 - 2^40)
	a88.Square().Square().Square().Square()                   // a88 = a^(2^88 - 2^44)
	a88.Mul(&a44)                                             // a88 = a^(2^88 - 1)
	a176.SquareVal(&a88).Square().Square().Square().Square()  // a176 = a^(2^93 - 2^5)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^98 - 2^10)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^103 - 2^15)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^108 - 2^20)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^113 - 2^25)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^118 - 2^30)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^123 - 2^35)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^128 - 2^40)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^133 - 2^45)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^138 - 2^50)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^143 - 2^55)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^148 - 2^60)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^153 - 2^65)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^158 - 2^70)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^163 - 2^75)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^168 - 2^80)
	a176.Square().Square().Square().Square().Square()         // a176 = a^(2^173 - 2^85)
	a176.Square().Square().Square()                           // a176 = a^(2^176 - 2^88)
	a176.Mul(&a88)                                            // a176 = a^(2^176 - 1)
	a220.SquareVal(&a176).Square().Square().Square().Square() // a220 = a^(2^181 - 2^5)
	a220.Square().Square().Square().Square().Square()         // a220 = a^(2^186 - 2^10)
	a220.Square().Square().Square().Square().Square()         // a220 = a^(2^191 - 2^15)
	a220.Square().Square().Square().Square().Square()         // a220 = a^(2^196 - 2^20)
	a220.Square().Square().Square().Square().Square()         // a220 = a^(2^201 - 2^25)
	a220.Square().Square().Square().Square().Square()         // a220 = a^(2^206 - 2^30)
	a220.Square().Square().Square().Square().Square()         // a220 = a^(2^211 - 2^35)
	a220.Square().Square().Square().Square().Square()         // a220 = a^(2^216 - 2^40)
	a220.Square().Square().Square().Square()                  // a220 = a^(2^220 - 2^44)
	a220.Mul(&a44)                                            // a220 = a^(2^220 - 1)
	a223.SquareVal(&a220).Square().Square()                   // a223 = a^(2^223 - 2^3)
	a223.Mul(&a3)                                             // a223 = a^(2^223 - 1)

	f.SquareVal(&a223).Square().Square().Square().Square() // f = a^(2^228 - 2^5)
	f.Square().Square().Square().Square().Square()         // f = a^(2^233 - 2^10)
	f.Square().Square().Square().Square().Square()         // f = a^(2^238 - 2^15)
	f.Square().Square().Square().Square().Square()         // f = a^(2^243 - 2^20)
	f.Square().Square().Square()                           // f = a^(2^246 - 2^23)
	f.Mul(&a22)                                            // f = a^(2^246 - 4194305)
	f.Square().Square().Square().Square().Square()         // f = a^(2^251 - 134217760)
	f.Mul(&a)                                              // f = a^(2^251 - 134217759)
	f.Square().Square().Square()                           // f = a^(2^254 - 1073742072)
	f.Mul(&a2)                                             // f = a^(2^254 - 1073742069)
	f.Square().Square()                                    // f = a^(2^256 - 4294968276)
	return f.Mul(&a)                                       // f = a^(2^256 - 4294968275) = a^(p-2)
}

// IsGtOrEqPrimeMinusOrder returns whether or not the field value is greater
// than or equal to the field prime minus the secp256k1 group order in constant
// time.
func (f *FieldVal64) IsGtOrEqPrimeMinusOrder() bool {
	// The secp256k1 prime is equivalent to 2^256 - 4294968273 and the group
	// order is 2^256 - 432420386565659656852420866394968145599.  Thus, the
	// prime minus the group order is:
	// 432420386565659656852420866390673177326
	//
	// In hex that is:
	// 0x00000000 00000000 00000000 00000001 45512319 50b75fc4 402da172 2fc9baee
	//
	// Converting that to field representation (base 2^64) is:
	//
	// n[0] = 0x402da1722fc9baee
	// n[1] = 0x4551231950b75fc4
	// n[2] = 0x0000000000000001
	// n[3] = 0x0000000000000000
	//
	// This can be verified with the following test code:
	//   pMinusN := new(big.Int).Sub(curveParams.P, curveParams.N)
	//   var fv FieldVal64
	//   fv.SetByteSlice(pMinusN.Bytes())
	//   t.Logf("%x", fv.n)
	//
	//   Outputs: [402da1722fc9baee 4551231950b75fc4 1 0]
	const (
		field64PMinusN0 = 0x402da1722fc9baee
		field64PMinusN1 = 0x4551231950b75fc4
		field64PMinusN2 = 0x0000000000000001
		field64PMinusN3 = 0x0000000000000000
	)

	// The goal is to return true when the value is greater than or equal to the
	// field prime minus the group order.  That is, return true when f ≥ p - n,
	// which is trivially rearranged to f - (p - n) ≥ 0.
	//
	// In other words, the condition is met iff subtracting (p - n) from f is
	// non-negative (aka there was no borrow).
	var borrow uint64
	_, borrow = bits.Sub64(f.n[0], field64PMinusN0, 0)
	_, borrow = bits.Sub64(f.n[1], field64PMinusN1, borrow)
	_, borrow = bits.Sub64(f.n[2], field64PMinusN2, borrow)
	_, borrow = bits.Sub64(f.n[3], field64PMinusN3, borrow)
	return borrow == 0
}
