// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package field4x64 implements highly optimized, constant-time arithmetic over
// the secp256k1 finite field using a dense 4x64 representation.
package field4x64

import (
	"encoding/binary"
	"encoding/hex"
	"math/bits"

	"github.com/decred/dcrd/dcrec/secp256k1/v4/internal/arith"
)

// References:
//   [HAC]: Handbook of Applied Cryptography Menezes, van Oorschot, Vanstone.
//     https://cacr.uwaterloo.ca/hac/

// This file provides an alternate implementation of the secp256k1 finite field.
// It uses tight 256-bit packing with four little-endian uint64s and fully
// reduces after each operation.  Hardware intrinsics are used when available.

// Element implements optimized fixed-precision arithmetic over the secp256k1
// finite field.  This means all arithmetic is performed modulo
//
//	0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc2f.
//
// This fully reduces after each operation and therefore does not require
// normalization or manual magnitude tracking.  It is also quite a bit faster
// than [field10x26.Element] on all modern 64-bit hardware.
type Element struct {
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

// Constants related to the internal representation.
const (
	// fieldPrimeComplement is the two's complement of the secp256k1 prime.
	fieldPrimeComplement = 0x1000003d1 // 2^32 + 977

	// These fields provide convenient access to each of the limbs of the
	// secp256k1 prime in the internal representation to improve code
	// readability.
	fieldPrimeLimb0 = 0xfffffffefffffc2f
	fieldPrimeLimb1 = 0xffffffffffffffff
	fieldPrimeLimb2 = 0xffffffffffffffff
	fieldPrimeLimb3 = 0xffffffffffffffff
)

// String returns the element as a human-readable hex string.
func (e Element) String() string {
	return hex.EncodeToString(e.Bytes()[:])
}

// Zero sets the element to zero in constant time.  A newly created element is
// already set to zero.  This function can be useful to clear an existing
// element for reuse.
func (e *Element) Zero() {
	e.n = [4]uint64{}
}

// Set sets the element equal to the passed element in constant time.
//
// The element is returned to support chaining.  This enables syntax like:
// e := new(Element).Set(e2).Add(1) so that e = e2 + 1 where e2 is not
// modified.
func (e *Element) Set(val *Element) *Element {
	e.n = val.n
	return e
}

// SetInt sets the element to the passed integer in constant time.  This is a
// convenience function since it is fairly common to perform arithmetic with
// small native integers.
//
// The element is returned to support chaining.  This enables syntax such
// as e := new(Element).SetInt(2).Mul(e2) so that e = 2 * e2.
func (e *Element) SetInt(v uint16) *Element {
	e.n = [4]uint64{uint64(v), 0, 0, 0}
	return e
}

// SetBytes packs the passed 32-byte big-endian value into the internal
// representation in constant time.  It interprets the provided array as a
// 256-bit big-endian unsigned integer, packs it, and returns either 1 if it is
// greater than or equal to the field prime (aka it overflowed) or 0 otherwise
// in constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.
func (e *Element) SetBytes(b *[32]byte) uint32 {
	// Pack the 256 total bits across the 4 uint64 limbs.
	e.n[0] = binary.BigEndian.Uint64(b[24:32])
	e.n[1] = binary.BigEndian.Uint64(b[16:24])
	e.n[2] = binary.BigEndian.Uint64(b[8:16])
	e.n[3] = binary.BigEndian.Uint64(b[0:8])

	// Since e < 2^256 < 2p (where p is the secp256k1 prime), the max possible
	// number of reductions required is one.  Therefore, in the case a reduction
	// is needed, it can be performed with a single subtraction of p.
	//
	// Since p must only conditionally be subtracted when e ≥ p, the following
	// handles it in constant time by always calculating s = e - p and selecting
	// the correct case via a constant time select.

	// Subtract p with borrow propagation.  borrow is set iff e < p.
	//
	// In other words, the input overflowed (≥ p) when e - p does NOT borrow.
	//
	// s = e - p
	var s0, s1, s2, s3, borrow uint64
	s0, borrow = bits.Sub64(e.n[0], fieldPrimeLimb0, 0)
	s1, borrow = bits.Sub64(e.n[1], fieldPrimeLimb1, borrow)
	s2, borrow = bits.Sub64(e.n[2], fieldPrimeLimb2, borrow)
	s3, borrow = bits.Sub64(e.n[3], fieldPrimeLimb3, borrow)

	// Constant-time select.
	//
	// Set e = e when e < p (aka borrow is set).  Otherwise e = s = e - p.
	e.n[0] = arith.ConstantTimeSelect64(borrow, e.n[0], s0)
	e.n[1] = arith.ConstantTimeSelect64(borrow, e.n[1], s1)
	e.n[2] = arith.ConstantTimeSelect64(borrow, e.n[2], s2)
	e.n[3] = arith.ConstantTimeSelect64(borrow, e.n[3], s3)
	return uint32(1 - borrow)
}

// zeroArray32 zeroes the provided 32-byte buffer.
func zeroArray32(b *[32]byte) {
	*b = [32]byte{}
}

// SetByteSlice interprets the provided slice as a 256-bit big-endian unsigned
// integer (meaning it is truncated to the first 32 bytes), packs it into the
// internal representation, and returns whether or not the resulting truncated
// 256-bit integer is greater than or equal to the field prime (aka it
// overflowed) in constant time.
//
// Note that since passing a slice with more than 32 bytes is truncated, it is
// possible that the truncated value is less than the field prime and hence it
// will not be reported as having overflowed in that case.  It is up to the
// caller to decide whether it needs to provide numbers of the appropriate size
// or it if is acceptable to use this function with the described truncation and
// overflow behavior.
func (e *Element) SetByteSlice(b []byte) bool {
	var b32 [32]byte
	b = b[:arith.ConstantTimeMin(uint32(len(b)), 32)]
	copy(b32[:], b32[:32-len(b)])
	copy(b32[32-len(b):], b)
	result := e.SetBytes(&b32)
	zeroArray32(&b32)
	return result != 0
}

// Normalize is a no-op.  It is provided to keep API parity with the other field
// element implementations.
func (e *Element) Normalize() *Element {
	return e
}

// PutBytesUnchecked unpacks the element to a 32-byte big-endian value directly
// into the passed byte slice in constant time.  The target slice must have at
// least 32 bytes available or it will panic.
//
// There is a similar function, [Element.PutBytes], which unpacks the element
// into a 32-byte array directly.  This version is provided since it can be
// useful to write directly into part of a larger buffer without needing a
// separate allocation.
func (e *Element) PutBytesUnchecked(b []byte) {
	// Unpack the 256 total bits from the 4 uint64 limbs.
	binary.BigEndian.PutUint64(b[0:8], e.n[3])
	binary.BigEndian.PutUint64(b[8:16], e.n[2])
	binary.BigEndian.PutUint64(b[16:24], e.n[1])
	binary.BigEndian.PutUint64(b[24:32], e.n[0])
}

// PutBytes unpacks the element to a 32-byte big-endian value using the passed
// byte array in constant time.
//
// There is a similar function, [Element.PutBytesUnchecked], which unpacks the
// element into a slice that must have at least 32 bytes available.  This
// version is provided since it can be useful to write directly into an array
// that is type checked.
//
// Alternatively, there is also [Element.Bytes], which unpacks the element into
// a new array and returns that which can sometimes be more ergonomic in
// applications that aren't concerned about an additional copy.
func (e *Element) PutBytes(b *[32]byte) {
	e.PutBytesUnchecked(b[:])
}

// Bytes unpacks the element to a 32-byte big-endian value in constant time.
//
// See [Element.PutBytes] and [Element.PutBytesUnchecked] for variants that
// allow an array or slice to be passed which can be useful to cut down on the
// number of allocations by allowing the caller to reuse a buffer or write
// directly into part of a larger buffer.
func (e *Element) Bytes() *[32]byte {
	var b [32]byte
	e.PutBytesUnchecked(b[:])
	return &b
}

// IsZeroBit returns 1 when the element is equal to zero or 0 otherwise in
// constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.  See [Element.IsZero] for the version
// that returns a bool.
func (e *Element) IsZeroBit() uint32 {
	return arith.ConstantTimeEq64(e.n[0]|e.n[1]|e.n[2]|e.n[3], 0)
}

// IsZero returns whether or not the element is equal to zero in constant time.
func (e *Element) IsZero() bool {
	return (e.n[0] | e.n[1] | e.n[2] | e.n[3]) == 0
}

// IsOneBit returns 1 when the element is equal to one or 0 otherwise in
// constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.  See [Element.IsOne] for the version
// that returns a bool.
func (e *Element) IsOneBit() uint32 {
	// The element can only be one if the single lowest significant bit is set
	// in the first limb and no other bits are set in any of the other limbs.
	// This is a constant time implementation.
	return arith.ConstantTimeEq64((e.n[0]^1)|e.n[1]|e.n[2]|e.n[3], 0)
}

// IsOne returns whether or not the element is equal to one in constant time.
func (e *Element) IsOne() bool {
	// The element can only be one if the single lowest significant bit is set
	// in the first limb and no other bits are set in any of the other limbs.
	// This is a constant time implementation.
	return ((e.n[0] ^ 1) | e.n[1] | e.n[2] | e.n[3]) == 0
}

// IsOddBit returns 1 when the element is an odd number or 0 otherwise in
// constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.  See [Element.IsOdd] for the version that
// returns a bool.
func (e *Element) IsOddBit() uint32 {
	// Only odd numbers have the bottom bit set.
	return uint32(e.n[0] & 1)
}

// IsOdd returns whether or not the element is an odd number in constant time.
func (e *Element) IsOdd() bool {
	// Only odd numbers have the bottom bit set.
	return e.n[0]&1 == 1
}

// Equals returns whether or not the two elements are the same in constant time.
func (e *Element) Equals(val *Element) bool {
	// Xor only sets bits when they are different, so the two elements can only
	// be the same if no bits are set after xoring each limb.  This is a
	// constant time implementation.
	return ((e.n[0] ^ val.n[0]) | (e.n[1] ^ val.n[1]) | (e.n[2] ^ val.n[2]) |
		(e.n[3] ^ val.n[3])) == 0
}

// NegateVal negates the passed element and stores the result in e in constant
// time.  The ignored parameter exists to keep API parity with the field element
// implementations.
//
// The element is returned to support chaining.  This enables syntax like:
// e.NegateVal(e2).AddInt(1) so that e = -e2 + 1.
func (e *Element) NegateVal(val *Element, _ uint32) *Element {
	// Since the element is already in the range 0 ≤ val < p, where p is the
	// secp256k1 prime, negation modulo p is just p - val.  This implies that
	// the result will always be in the desired range with the sole exception of
	// 0 because p - 0 = p itself.
	//
	// The following handles that case in constant time by creating a mask that
	// is all 0s in the case the element being negated is 0 and all 1s otherwise
	// and then bitwise ands that mask with each limb of the prime.

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
	maskedPrime0 := fieldPrimeLimb0 & mask

	// Add 0 when val == 0 or p when val != 0.  The result is either:
	//
	// val == 0: e = 0 + 0 = 0
	// val != 0: e = -val + p = p - val
	var carry uint64
	e.n[0], carry = bits.Add64(t0, maskedPrime0, 0)
	e.n[1], carry = bits.Add64(t1, mask, carry)
	e.n[2], carry = bits.Add64(t2, mask, carry)
	e.n[3], _ = bits.Add64(t3, mask, carry)
	return e
}

// Negate negates the element in constant time.  The existing element is
// modified.  The ignored parameter exists to keep API parity with the field
// element implementations.
//
// The element is returned to support chaining.  This enables syntax like:
// e.Negate().AddInt(1) so that e = -e + 1.
func (e *Element) Negate(_ uint32) *Element {
	return e.NegateVal(e, 0)
}

// AddInt adds the passed integer to the existing element and stores the result
// in e in constant time.  This is a convenience function since it is fairly
// common to perform some arithmetic with small native integers.
//
// The element is returned to support chaining.  This enables syntax like:
// e.AddInt(1).Add(e2) so that e = e + 1 + e2.
func (e *Element) AddInt(ui uint16) *Element {
	return e.Add(new(Element).SetInt(ui))
}

// Add adds the passed element to the existing element and stores the result in
// e in constant time.
//
// The element is returned to support chaining.  This enables syntax like:
// e.Add(e2).AddInt(1) so that e = e + e2 + 1.
func (e *Element) Add(val *Element) *Element {
	return e.Add2(e, val)
}

// Add2 adds the passed two elements together and stores the result in e in
// constant time.
//
// The element is returned to support chaining.  This enables syntax like:
// e3.Add2(e, e2).AddInt(1) so that e3 = e + e2 + 1.
func (e *Element) Add2(a, b *Element) *Element {
	// Since both elements are already in the range 0 ≤ val < p (where p is the
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
	s0, borrow = bits.Sub64(t0, fieldPrimeLimb0, 0)
	s1, borrow = bits.Sub64(t1, fieldPrimeLimb1, borrow)
	s2, borrow = bits.Sub64(t2, fieldPrimeLimb2, borrow)
	s3, borrow = bits.Sub64(t3, fieldPrimeLimb3, borrow)

	// Constant-time select.
	//
	// Set e = t = a+b only when there was no overflow and t < p (borrow set).
	// Otherwise e = s = a+b - p.
	cond := (1 - overflow) & borrow
	e.n[0] = arith.ConstantTimeSelect64(cond, t0, s0)
	e.n[1] = arith.ConstantTimeSelect64(cond, t1, s1)
	e.n[2] = arith.ConstantTimeSelect64(cond, t2, s2)
	e.n[3] = arith.ConstantTimeSelect64(cond, t3, s3)
	return e
}

// MulBy2 multiplies the element by 2 and stores the result in e in constant
// time.
//
// This method is optimized to provide a significant speed advantage over the
// more general [Element.MulInt].
//
// The element is returned to support chaining.  This enables syntax like:
// e.MulBy2().Add(e2) so that e = 2 * e + e2.
func (e *Element) MulBy2() *Element {
	return e.Add(e)
}

// MulBy3 multiplies the element by 3 and stores the result in e in constant
// time.
//
// This method is optimized to provide a significant speed advantage over the
// more general [Element.MulInt].
//
// The element is returned to support chaining.  This enables syntax like:
// e.MulBy3().Add(e2) so that e = 3 * e + e2.
func (e *Element) MulBy3() *Element {
	var orig Element
	orig.Set(e)
	return e.MulBy2().Add(&orig)
}

// MulBy4 multiplies the element by 4 and stores the result in e in constant
// time.
//
// This method is optimized to provide a significant speed advantage over the
// more general [Element.MulInt].
//
// The element is returned to support chaining.  This enables syntax like:
// e.MulBy4().Add(e2) so that e = 4 * e + e2.
func (e *Element) MulBy4() *Element {
	return e.MulBy2().MulBy2()
}

// MulBy8 multiplies the element by 8 and stores the result in e in constant
// time.
//
// This method is optimized to provide a significant speed advantage over the
// more general [Element.MulInt].
//
// The element is returned to support chaining.  This enables syntax like:
// e.MulBy8().Add(e2) so that e = 8 * e + e2.
func (e *Element) MulBy8() *Element {
	return e.MulBy4().MulBy2()
}

// MulInt multiplies the element by the passed int and stores the result in e in
// constant time.
//
// Callers should prefer using the faster specialized methods for multiplying by
// 2, 3, 4, and 8, as they are commonly used in curve equations.
//
// See [Element.MulBy2], [Element.MulBy3], [Element.MulBy4], and
// [Element.MulBy8] for the aforementioned optimized methods.
//
// The element is returned to support chaining.  This enables syntax like:
// e.MulInt(15).Add(e2) so that e = 15 * e + e2.
func (e *Element) MulInt(val uint8) *Element {
	return e.Mul(new(Element).SetInt(uint16(val)))
}

// Mul multiplies the passed element to the existing element and stores the
// result in e in constant time.
//
// The element is returned to support chaining.  This enables syntax like:
// e.Mul(e2).AddInt(1) so that e = (e * e2) + 1.
func (e *Element) Mul(val *Element) *Element {
	return e.Mul2(e, val)
}

// Mul2 multiplies the passed two elements together and stores the result in e
// in constant time.
//
// The element is returned to support chaining.  This enables syntax like:
// e3.Mul2(e, e2).AddInt(1) so that e3 = (e * e2) + 1.
func (e *Element) Mul2(a, b *Element) *Element {
	mulReduce(&e.n, &a.n, &b.n)
	return e
}

// SquareRootVal either calculates the square root of the passed element when it
// exists or the square root of the negation of the element when it does not
// exist and stores the result in e in constant time.  The return flag is true
// when the calculated square root is for the passed element itself and false
// when it is for its negation.
func (e *Element) SquareRootVal(val *Element) bool {
	// This uses the Tonelli-Shanks method for calculating the square root of
	// the element when it exists.  The key principles of the method follow.
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
	// element when it does not.  In the case of primes that are ≡ 3 (mod 4),
	// the possible solutions are r = ±a^((p+1)/4) (mod p).  Therefore, either
	// r^2 ≡ a (mod p) is true in which case ±r are the two solutions, or r^2 ≡
	// -a (mod p) in which case 'a' is a non-residue and there are no solutions.
	//
	// The secp256k1 prime is ≡ 3 (mod 4), so this result applies.
	//
	// In other words, calculate a^((p+1)/4) and then square it and check it
	// against the original element to determine if it is actually the square
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
	var a, a2, a3, a6, a9, a11, a22, a44, a88, a176, a220, a223 Element
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

	e.SquareVal(&a223).Square().Square().Square().Square() // e = a^(2^228 - 2^5)
	e.Square().Square().Square().Square().Square()         // e = a^(2^233 - 2^10)
	e.Square().Square().Square().Square().Square()         // e = a^(2^238 - 2^15)
	e.Square().Square().Square().Square().Square()         // e = a^(2^243 - 2^20)
	e.Square().Square().Square()                           // e = a^(2^246 - 2^23)
	e.Mul(&a22)                                            // e = a^(2^246 - 2^22 - 1)
	e.Square().Square().Square().Square().Square()         // e = a^(2^251 - 2^27 - 2^5)
	e.Square()                                             // e = a^(2^252 - 2^28 - 2^6)
	e.Mul(&a2)                                             // e = a^(2^252 - 2^28 - 2^6 - 2^1 - 1)
	e.Square().Square()                                    // e = a^(2^254 - 2^30 - 244) = a^((p+1)/4)

	// Verify the result is actually the square root by squaring it and checking
	// against the original element.
	var sqr Element
	return sqr.SquareVal(e).Equals(val)
}

// Square squares the element in constant time.  The existing element is
// modified.
//
// The element is returned to support chaining.  This enables syntax like:
// e.Square().Mul(e2) so that e = e^2 * e2.
func (e *Element) Square() *Element {
	return e.SquareVal(e)
}

// SquareVal squares the passed element and stores the result in e in constant
// time.
//
// The element is returned to support chaining.  This enables syntax like:
// e3.SquareVal(e).Mul(e) so that e3 = e^2 * e = e^3.
func (e *Element) SquareVal(val *Element) *Element {
	squareReduce(&e.n, &val.n)
	return e
}

// Inverse finds the modular multiplicative inverse of the element in constant
// time.  The existing element is modified.
//
// The element is returned to support chaining.  This enables syntax like:
// e.Inverse().Mul(e2) so that e = e^-1 * e2.
func (e *Element) Inverse() *Element {
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
	var a, a2, a3, a6, a9, a11, a22, a44, a88, a176, a220, a223 Element
	a.Set(e)
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

	e.SquareVal(&a223).Square().Square().Square().Square() // e = a^(2^228 - 2^5)
	e.Square().Square().Square().Square().Square()         // e = a^(2^233 - 2^10)
	e.Square().Square().Square().Square().Square()         // e = a^(2^238 - 2^15)
	e.Square().Square().Square().Square().Square()         // e = a^(2^243 - 2^20)
	e.Square().Square().Square()                           // e = a^(2^246 - 2^23)
	e.Mul(&a22)                                            // e = a^(2^246 - 4194305)
	e.Square().Square().Square().Square().Square()         // e = a^(2^251 - 134217760)
	e.Mul(&a)                                              // e = a^(2^251 - 134217759)
	e.Square().Square().Square()                           // e = a^(2^254 - 1073742072)
	e.Mul(&a2)                                             // e = a^(2^254 - 1073742069)
	e.Square().Square()                                    // e = a^(2^256 - 4294968276)
	return e.Mul(&a)                                       // e = a^(2^256 - 4294968275) = a^(p-2)
}

// IsGtOrEqPrimeMinusOrder returns whether or not the element is greater than or
// equal to the field prime minus the secp256k1 group order in constant time.
func (e *Element) IsGtOrEqPrimeMinusOrder() bool {
	// The secp256k1 prime is equivalent to 2^256 - 4294968273 and the group
	// order is 2^256 - 432420386565659656852420866394968145599.  Thus, the
	// prime minus the group order is:
	// 432420386565659656852420866390673177326
	//
	// In hex that is:
	// 0x00000000 00000000 00000000 00000001 45512319 50b75fc4 402da172 2fc9baee
	//
	// Converting that to the internal representation (base 2^64) is:
	//
	// n[0] = 0x402da1722fc9baee
	// n[1] = 0x4551231950b75fc4
	// n[2] = 0x0000000000000001
	// n[3] = 0x0000000000000000
	//
	// This can be verified with the following test code:
	//   pMinusN := new(big.Int).Sub(curveParams.P, curveParams.N)
	//   var v Element
	//   v.SetByteSlice(pMinusN.Bytes())
	//   t.Logf("%x", v.n)
	//
	//   Outputs: [402da1722fc9baee 4551231950b75fc4 1 0]
	const (
		pMinusNLimb0 = 0x402da1722fc9baee
		pMinusNLimb1 = 0x4551231950b75fc4
		pMinusNLimb2 = 0x0000000000000001
		pMinusNLimb3 = 0x0000000000000000
	)

	// The goal is to return true when the element is greater than or equal to
	// the field prime minus the group order.  That is, return true when e ≥ p -
	// n, which is trivially rearranged to e - (p - n) ≥ 0.
	//
	// In other words, the condition is met iff subtracting (p - n) from e is
	// non-negative (aka there was no borrow).
	var borrow uint64
	_, borrow = bits.Sub64(e.n[0], pMinusNLimb0, 0)
	_, borrow = bits.Sub64(e.n[1], pMinusNLimb1, borrow)
	_, borrow = bits.Sub64(e.n[2], pMinusNLimb2, borrow)
	_, borrow = bits.Sub64(e.n[3], pMinusNLimb3, borrow)
	return borrow == 0
}
