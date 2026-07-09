// Copyright (c) 2020-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"math/bits"
	"sync"
)

// References:
//   [SECG]: Recommended Elliptic Curve Domain Parameters
//     https://www.secg.org/sec2-v2.pdf
//
//   [HAC]: Handbook of Applied Cryptography Menezes, van Oorschot, Vanstone.
//     https://cacr.uwaterloo.ca/hac/

// Many elliptic curve operations require working with scalars in a finite field
// defined by the order of the group underlying the secp256k1 curve.  Since the
// group order exceeds the width of the largest available native type, some form
// of multi-precision arithmetic is required.  This code implements specialized
// fixed-precision field arithmetic rather than relying on an
// arbitrary-precision arithmetic package such as math/big for performing
// arithmetic modulo the fixed group order.  As a result, significant
// performance gains are achieved by taking advantage of many optimizations not
// available to arbitrary-precision arithmetic and generic modular arithmetic
// algorithms.
//
// There are various ways to internally represent each element with their own
// pros and cons.  Some common representations are:
//
//   - 8 uint32s with base 2^32 limbs (aka 8x32: 32 bits * 8 = 256 bits)
//   - 10 uint32s with base 2^26 limbs (aka 10x26: 26 bits * 10 = 260 bits)
//   - 4 uint64s with base 2^64 limbs (aka 4x64: 64 bits * 4 = 256 bits)
//   - 5 uint64s with base 2^52 limbs (aka 5x52: 52 bits * 5 = 260 bits)
//
// The primary tradeoff is complexity versus performance and the optimal choice
// also largely depends on the available hardware capabilities.
//
// For example, 5x52 and 10x26 allow performing several operations in a row
// before carry propagation or modular reduction becomes necessary since there
// are additional bits available in each limb, but that comes at the cost of
// manual magnitude tracking, multiple non-canonical representations of each
// number, and periodic normalization to propagate the accumulated carries and
// perform the modular reduction all at once.
//
// Conversely, 8x32 and 4x64 involve less complexity, are simpler to use, have
// fewer limbs to manage, and only have a single canonical representation for
// each number, but that comes at the cost of requiring carry propagation and
// modular reduction for every operation and larger intermediate results.
//
// The requirement for larger intermediate results is particularly notable for
// uint64s because there is no native Go type large enough to handle the 128-bit
// intermediate results while adding and multiplying the 64-bit limbs.
//
// This implementation historically used 8x32 for that reason, coupled with the
// fact that Go did not reliably make use of hardware-supported wide arithmetic,
// to ensure all intermediate results fit cleanly into uint64s.
//
// However, most systems are now 64-bit and almost all have instructions capable
// of handling the 128-bit intermediate results without the need for additional
// half-word arithmetic.  Further, modern versions of Go automatically use those
// instructions on hardware that supports it.
//
// Given the above, this implementation represents the field elements as 4
// uint64s with each limb (array entry) treated as base 2^64 (aka 4x64) and
// keeps the value reduced at all times.
const (
	// These fields provide convenient access to each of the limbs of the
	// secp256k1 curve group order N to improve code readability.
	//
	// The group order of the curve per [SECG] is:
	// 0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141
	orderLimb0 uint64 = 0xbfd25e8cd0364141
	orderLimb1 uint64 = 0xbaaedce6af48a03b
	orderLimb2 uint64 = 0xfffffffffffffffe
	orderLimb3 uint64 = 0xffffffffffffffff

	// These fields provide convenient access to each of the limbs of the two's
	// complement of the secp256k1 curve group order N to improve code
	// readability.
	//
	// The two's complement of the group order is:
	// 0x00000000 00000000 00000000 00000001 45512319 50b75fc4 402da173 2fc9bebf
	orderComplementLimb0 uint64 = (^orderLimb0) + 1
	orderComplementLimb1 uint64 = ^orderLimb1
	orderComplementLimb2 uint64 = ^orderLimb2
	// orderComplementLimb3 uint64 = ^orderLimb3 // unused
)

var (
	// zero32 is an array of 32 bytes used for the purposes of zeroing and is
	// defined here to avoid extra allocations.
	zero32 = [32]byte{}
)

// ModNScalar implements optimized 256-bit constant-time fixed-precision
// arithmetic over the secp256k1 group order. This means all arithmetic is
// performed modulo:
//
//	0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141
//
// It only implements the arithmetic needed for elliptic curve operations,
// however, the operations that are not implemented can typically be worked
// around if absolutely needed.  For example, subtraction can be performed by
// adding the negation.
//
// Should it be absolutely necessary, conversion to a standard library [big.Int]
// can be accomplished by using [ModNScalar.Bytes], slicing the resulting
// fixed-size array, and feeding it to [big.Int.SetBytes].  However, that should
// typically be avoided when possible as conversion to a [big.Int] requires
// allocations, is not constant time, and is slower when working modulo the
// group order.
type ModNScalar struct {
	// The scalar is represented as 4 64-bit integers in base 2^64.
	//
	// The following depicts the internal representation:
	// 	 --------------------------------------------------------------------
	// 	|       n[3]     |      n[2]      |      n[1]      |      n[0]      |
	// 	| 64 bits        | 64 bits        | 64 bits        | 64 bits        |
	// 	| Mult: 2^(64*3) | Mult: 2^(64*2) | Mult: 2^(64*1) | Mult: 2^(64*0) |
	// 	 --------------------------------------------------------------------
	//
	// For example, consider the number 2^135 + 2^87 + 1.  It would be
	// represented as:
	// 	n[0] = 1
	// 	n[1] = 2^23
	// 	n[2] = 2^7
	// 	n[3] = 0
	//
	// The full 256-bit value is then calculated by looping i from 3..0 and
	// doing sum(n[i] * 2^(64i)) like so:
	// 	n[3] * 2^(64*3) = 0    * 2^192 = 0
	// 	n[2] * 2^(64*2) = 2^7  * 2^128  = 2^135
	// 	n[1] * 2^(64*1) = 2^23 * 2^64  = 2^87
	// 	n[0] * 2^(64*0) = 1    * 2^0   = 1
	// 	Sum: 0 + 2^135 + 2^87 + 1 = 2^135 + 2^87 + 1
	n [4]uint64
}

// String returns the scalar as a human-readable hex string.
//
// This is NOT constant time.
func (s ModNScalar) String() string {
	b := s.Bytes()
	return hex.EncodeToString(b[:])
}

// Set sets the scalar equal to a copy of the passed one in constant time.
//
// The scalar is returned to support chaining.  This enables syntax like:
// s := new(ModNScalar).Set(s2).Add(1) so that s = s2 + 1 where s2 is not
// modified.
func (s *ModNScalar) Set(val *ModNScalar) *ModNScalar {
	*s = *val
	return s
}

// Zero sets the scalar to zero in constant time.  A newly created scalar is
// already set to zero.  This function can be useful to clear an existing scalar
// for reuse.
func (s *ModNScalar) Zero() {
	s.n = [4]uint64{}
}

// IsZeroBit returns 1 when the scalar is equal to zero or 0 otherwise in
// constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.  See [ModNScalar.IsZero] for the version
// that returns a bool.
func (s *ModNScalar) IsZeroBit() uint32 {
	return constantTimeEq64(s.n[0]|s.n[1]|s.n[2]|s.n[3], 0)
}

// IsZero returns whether or not the scalar is equal to zero in constant time.
func (s *ModNScalar) IsZero() bool {
	// The scalar can only be zero if no bits are set in any of the words.
	return (s.n[0] | s.n[1] | s.n[2] | s.n[3]) == 0
}

// SetInt sets the scalar to the passed integer in constant time.  This is a
// convenience function since it is fairly common to perform some arithmetic
// with small native integers.
//
// The scalar is returned to support chaining.  This enables syntax like:
// s := new(ModNScalar).SetInt(2).Mul(s2) so that s = 2 * s2.
func (s *ModNScalar) SetInt(ui uint32) *ModNScalar {
	s.n = [4]uint64{uint64(ui), 0, 0, 0}
	return s
}

// SetBytes interprets the provided array as a 256-bit big-endian unsigned
// integer, reduces it modulo the group order, sets the scalar to the result,
// and returns either 1 if it was reduced (aka it overflowed) or 0 otherwise in
// constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.
func (s *ModNScalar) SetBytes(b *[32]byte) uint32 {
	// Pack the 256 total bits across the 4 uint64 words.  This could be done
	// with a for loop, but benchmarks show this unrolled version is about 2
	// times faster than the variant that uses a loop.
	s.n[0] = binary.BigEndian.Uint64(b[24:32])
	s.n[1] = binary.BigEndian.Uint64(b[16:24])
	s.n[2] = binary.BigEndian.Uint64(b[8:16])
	s.n[3] = binary.BigEndian.Uint64(b[0:8])

	// Since s < 2^256 < 2N (where N is the secp256k1 group order), the max
	// possible number of reductions required is one.  Therefore, in the case a
	// reduction is needed, it can be performed with a single subtraction of N.
	//
	// Since N must only conditionally be subtracted when s ≥ N, the following
	// handles it in constant time by always calculating t = s - N and selecting
	// the correct case via a constant time select.

	// Subtract N with borrow propagation.  borrow is set iff s < N.
	//
	// In other words, the input overflowed (≥ N) when s - N does NOT borrow.
	//
	// t = s - N
	var t0, t1, t2, t3, borrow uint64
	t0, borrow = bits.Sub64(s.n[0], orderLimb0, 0)
	t1, borrow = bits.Sub64(s.n[1], orderLimb1, borrow)
	t2, borrow = bits.Sub64(s.n[2], orderLimb2, borrow)
	t3, borrow = bits.Sub64(s.n[3], orderLimb3, borrow)

	// Constant-time select.
	//
	// Set s = s when s < N (aka borrow is set).  Otherwise s = t = s - N.
	s.n[0] = constantTimeSelect64(borrow, s.n[0], t0)
	s.n[1] = constantTimeSelect64(borrow, s.n[1], t1)
	s.n[2] = constantTimeSelect64(borrow, s.n[2], t2)
	s.n[3] = constantTimeSelect64(borrow, s.n[3], t3)
	return uint32(1 - borrow)
}

// zeroArray32 zeroes the provided 32-byte buffer.
func zeroArray32(b *[32]byte) {
	copy(b[:], zero32[:])
}

// SetByteSlice interprets the provided slice as a 256-bit big-endian unsigned
// integer (meaning it is truncated to the first 32 bytes), reduces it modulo
// the group order, sets the scalar to the result, and returns whether or not
// the resulting truncated 256-bit integer overflowed in constant time.
//
// Note that since passing a slice with more than 32 bytes is truncated, it is
// possible that the truncated value is less than the order of the curve and
// hence it will not be reported as having overflowed in that case.  It is up to
// the caller to decide whether it needs to provide numbers of the appropriate
// size or it is acceptable to use this function with the described truncation
// and overflow behavior.
func (s *ModNScalar) SetByteSlice(b []byte) bool {
	// Always copy a total of 32 bytes regardless of the input length to avoid
	// introducing data-dependent timing.
	var b32 [32]byte
	b = b[:constantTimeMin(uint32(len(b)), 32)]
	copy(b32[:], b32[:32-len(b)])
	copy(b32[32-len(b):], b)
	result := s.SetBytes(&b32)
	zeroArray32(&b32)
	return result != 0
}

// PutBytesUnchecked unpacks the scalar to a 32-byte big-endian value directly
// into the passed byte slice in constant time.  The target slice must have at
// least 32 bytes available or it will panic.
//
// There is a similar function, [ModNScalar.PutBytes], which unpacks the scalar
// into a 32-byte array directly.  This version is provided since it can be
// useful to write directly into part of a larger buffer without needing a
// separate allocation.
//
// Preconditions:
//   - The target slice MUST have at least 32 bytes available
func (s *ModNScalar) PutBytesUnchecked(b []byte) {
	// Unpack the 256 total bits from the 4 uint64 words.  This could be done
	// with a for loop, but benchmarks show this unrolled version is about 3
	// times faster than the variant which uses a loop.
	binary.BigEndian.PutUint64(b[0:8], s.n[3])
	binary.BigEndian.PutUint64(b[8:16], s.n[2])
	binary.BigEndian.PutUint64(b[16:24], s.n[1])
	binary.BigEndian.PutUint64(b[24:32], s.n[0])
}

// PutBytes unpacks the scalar to a 32-byte big-endian value using the passed
// byte array in constant time.
//
// There is a similar function, [ModNScalar.PutBytesUnchecked], which unpacks
// the scalar into a slice that must have at least 32 bytes available.  This
// version is provided since it can be useful to write directly into an array
// that is type checked.
//
// Alternatively, there is also [ModNScalar.Bytes], which unpacks the scalar
// into a new array and returns that which can sometimes be more ergonomic in
// applications that aren't concerned about an additional copy.
func (s *ModNScalar) PutBytes(b *[32]byte) {
	s.PutBytesUnchecked(b[:])
}

// Bytes unpacks the scalar to a 32-byte big-endian value in constant time.
//
// See [ModNScalar.PutBytes] and [ModNScalar.PutBytesUnchecked] for variants
// that allow an array or slice to be passed which can be useful to cut down on
// the number of allocations by allowing the caller to reuse a buffer or write
// directly into part of a larger buffer.
func (s *ModNScalar) Bytes() [32]byte {
	var b [32]byte
	s.PutBytesUnchecked(b[:])
	return b
}

// IsOdd returns whether or not the scalar is an odd number in constant time.
func (s *ModNScalar) IsOdd() bool {
	// Only odd numbers have the bottom bit set.
	return s.n[0]&1 == 1
}

// Equals returns whether or not the two scalars are the same in constant time.
func (s *ModNScalar) Equals(val *ModNScalar) bool {
	// Xor only sets bits when they are different, so the two scalars can only
	// be the same if no bits are set after xoring each word.
	return ((s.n[0] ^ val.n[0]) | (s.n[1] ^ val.n[1]) | (s.n[2] ^ val.n[2]) |
		(s.n[3] ^ val.n[3])) == 0
}

// Add2 adds the passed two scalars together modulo the group order in constant
// time and stores the result in s.
//
// The scalar is returned to support chaining.  This enables syntax like:
// s3.Add2(s, s2).AddInt(1) so that s3 = s + s2 + 1.
func (s *ModNScalar) Add2(a, b *ModNScalar) *ModNScalar {
	// Since both values are already in the range 0 ≤ val < N (where N is the
	// secp256k1 group order), the maximum possible result is < 2N - 1.  So a
	// maximum of one subtraction of N is required in the worst case.
	//
	// Since N must only conditionally be subtracted when a+b ≥ N, the following
	// handles it in constant time by calculating both t = a+b and u = a+b - N
	// and selecting the correct case via a constant time select.

	// Add with carry propagation.  overflow is set iff t = a+b ≥ 2^256.
	//
	// t = a + b
	var t0, t1, t2, t3, overflow, carry uint64
	t0, carry = bits.Add64(a.n[0], b.n[0], 0)
	t1, carry = bits.Add64(a.n[1], b.n[1], carry)
	t2, carry = bits.Add64(a.n[2], b.n[2], carry)
	t3, overflow = bits.Add64(a.n[3], b.n[3], carry)

	// Subtract N with borrow propagation.  borrow is set iff t = a+b < N.
	//
	// u = t - N = a+b - N
	var u0, u1, u2, u3, borrow uint64
	u0, borrow = bits.Sub64(t0, orderLimb0, 0)
	u1, borrow = bits.Sub64(t1, orderLimb1, borrow)
	u2, borrow = bits.Sub64(t2, orderLimb2, borrow)
	u3, borrow = bits.Sub64(t3, orderLimb3, borrow)

	// Constant-time select.
	//
	// Set s = t = a+b only when there was no overflow and t < N (borrow set).
	// Otherwise s = u = a+b - N.
	cond := (1 - overflow) & borrow
	s.n[0] = constantTimeSelect64(cond, t0, u0)
	s.n[1] = constantTimeSelect64(cond, t1, u1)
	s.n[2] = constantTimeSelect64(cond, t2, u2)
	s.n[3] = constantTimeSelect64(cond, t3, u3)
	return s
}

// Add adds the passed scalar to the existing one modulo the group order in
// constant time and stores the result in s.
//
// The scalar is returned to support chaining.  This enables syntax like:
// s.Add(s2).AddInt(1) so that s = s + s2 + 1.
func (s *ModNScalar) Add(val *ModNScalar) *ModNScalar {
	return s.Add2(s, val)
}

// scalar64Reduce512 reduces a 512-bit little-endian limb array modulo the group
// order in constant time and stores the result in r.
func scalar64Reduce512(r *[4]uint64, x *[8]uint64) {
	// The overall strategy employed here is:
	// 1) Start with the full unreduced 512-bit product of the two scalars.
	// 2) Reduce the result modulo the group order via Crandall reduction
	//    (described below).
	// 3) Repeat step 2 noting that each iteration reduces the required number
	//    of bits by 127 because the two's complement of N has 127 leading zero
	//    bits.
	// 4) Once reduced to less than a max of 2N, perform the final reduction
	//    with a constant-time conditional subtraction.
	//
	// Technically the max possible input value here will be (N-1)^2, since the
	// only time it is called is with the product of two scalars that are always
	// mod N.  Nevertheless, it is safer to treat it as the product of two max
	// size 256-bit values, (2^256-1)^2 = 2^512 - 2^257 + 1, to ensure
	// correctness if that were to ever change.
	//
	// Per [HAC] section 14.3.4: Reduction method of moduli of special form,
	// when the modulus is of the special form m = b^t - c, highly efficient
	// reduction can be achieved.  While [HAC] only presents the algorithm and
	// does not call it out by name or provide the mathematical justification,
	// the underlying technique is known as Crandall reduction and is often
	// presented as 2^k - c.  It is easy to see they are equivalent by setting
	// b = 2 and t = k.
	//
	// The secp256k1 group order is 2^256 - 0x14551231950b75fc4402da1732fc9bebf,
	// so it fits this criteria where:
	//   k = 256
	//   c = 432420386565659656852420866394968145599
	//
	// Crandall reduction works by taking advantage of the fact that if a prime
	// is of the form 2^k - c, then 2^k - c ≡ 0 (mod p), so 2^k ≡ c (mod p).  In
	// other words, every multiple of 2^k is equivalent to adding c when working
	// modulo p.
	//
	// Since the 512-bit value to reduce is tightly packed into uint64s, the
	// upper 4 limbs are all multiples of 2^256.  Therefore, reducing modulo the
	// group order is equivalent to multiplying those upper limbs by c and
	// adding the result to the corresponding lower 4 limbs while propagating
	// the carries.
	//
	// For the specific case of the secp256k1 group order, a max of 4 reductions
	// are required because c is 129 bits and so the first round will reduce
	// from 512 bits to a max of 385 bits, the second round will reduce to a max
	// of 258 bits, and the third round will reduce to within 2N.  Then, a
	// conditional subtraction of N handles the final reduction.
	//
	// The reduction is slightly complicated by the fact c needs 3 uint64 limbs
	// to represent it, so multiplying the upper limbs by c requires multiplying
	// each one by all 3 limbs of c and carefully managing the columns and
	// carries.  The uppermost limb of c = 1, so multiplication by that limb is
	// avoided in the code below.

	var t0, t1, t2, t3, t4, t5, t6, l0, h0, l1, h1, l2, carry uint64

	// -------------------------------------------------------------------------
	// First reduction.
	//
	// The code in this section is equivalent to the following if uint512 and
	// larger numbers and shifts were supported directly:
	//
	// t0 = uint64(x%(1<<256) + (x>>256)*c)
	// t1 = uint64((x%(1<<256) + (x>>256)*c) >> 64)
	// t2 = uint64((x%(1<<256) + (x>>256)*c) >> 128)
	// t3 = uint64((x%(1<<256) + (x>>256)*c) >> 192)
	// t4 = uint64((x%(1<<256) + (x>>256)*c) >> 256)
	// t5 = uint64((x%(1<<256) + (x>>256)*c) >> 320)
	// t6 = uint64((x%(1<<256) + (x>>256)*c) >> 384)
	//
	// To visualize the logic, multiply the 3 limbs of c by the upper limbs via
	// the standard pencil-and-paper method:
	//
	//   2^576 2^512 2^448 2^384 2^320 2^256 (place values)
	//   -----------------------------------
	//                  x7    x6    x5    x4
	//                        c2    c1    c0 *
	//   -----------------------------------
	//                      x4c2  x4c1  x4c0
	//                x5c2  x5c1  x5c0       +
	//          x6c2  x6c1  x6c0             +
	//    x7c2  x7c1  x7c0                   +
	//   -----------------------------------
	//    x7c2  ....  ....  ....  ....  x4c0
	//
	// Then add them to the associated lower limbs per the reduction identity:
	//
	//   2^384 2^320 2^256 2^192 2^128  2^64   2^0 (place values)
	//   -----------------------------------------
	//                        x3    x2    x1    x0
	//                            x4c2  x4c1  x4c0 +
	//                      x5c2  x5c1  x5c0       +
	//                x6c2  x6c1  x6c0             +
	//          x7c2  x7c1  x7c0                   +
	//   -----------------------------------------
	//      t6    t5    t4    t3    t2    t1    t0
	//
	// Where t6 ≤ 1 for the potential carry.
	//
	// Thus 0 ≤ t < 2^385 after the 1st reduction.
	// -------------------------------------------------------------------------

	// Compute x4*c with result in t0..t2 with carries propagated and the final
	// carry in t3.
	h0, t0 = bits.Mul64(x[4], orderComplementLimb0)
	h1, t1 = bits.Mul64(x[4], orderComplementLimb1)
	t1, carry = bits.Add64(t1, h0, 0)
	t2, carry = bits.Add64(x[4], h1, carry)
	t3 = carry

	// Compute x5*c with result added to t1..t3 with carries propagated and the
	// final carry in t4.
	//
	// Note that since h1 is the upper 64 bits of the product of a uint64 with
	// c1 (limb 1 of c) and c1 < 2^63 - 1:
	//   h1 ≤ floor((2^64-1)(2^63-1) / 2^64) = 2^63 - 2
	//
	// Then, because current t3 ≤ 1 and carry ≤ 1, a loose bound for the first
	// t3 below is:
	//   t3 ≤ h1 + 2 = 2^63 < 2^64
	//
	// Therefore, it is safe to discard the carry.
	h0, l0 = bits.Mul64(x[5], orderComplementLimb0)
	h1, l1 = bits.Mul64(x[5], orderComplementLimb1)
	t1, carry = bits.Add64(t1, l0, 0)
	t2, carry = bits.Add64(t2, h0, carry)
	t3, _ = bits.Add64(t3, h1, carry)
	t2, carry = bits.Add64(t2, l1, 0)
	t3, carry = bits.Add64(t3, x[5], carry)
	t4 = carry

	// Compute x6*c with result added to t2..t4 with carries propagated and the
	// final carry in t5.
	//
	// It is safe to discard the carry on the first t4 for the same reason as
	// above.
	h0, l0 = bits.Mul64(x[6], orderComplementLimb0)
	h1, l1 = bits.Mul64(x[6], orderComplementLimb1)
	t2, carry = bits.Add64(t2, l0, 0)
	t3, carry = bits.Add64(t3, h0, carry)
	t4, _ = bits.Add64(t4, h1, carry)
	t3, carry = bits.Add64(t3, l1, 0)
	t4, carry = bits.Add64(t4, x[6], carry)
	t5 = carry

	// Compute x7*c with result added to t3..t5 with carries propagated and the
	// final carry in t6.
	//
	// It is safe to discard the carry on the first t5 for the same reason as
	// above.
	h0, l0 = bits.Mul64(x[7], orderComplementLimb0)
	h1, l1 = bits.Mul64(x[7], orderComplementLimb1)
	t3, carry = bits.Add64(t3, l0, 0)
	t4, carry = bits.Add64(t4, h0, carry)
	t5, _ = bits.Add64(t5, h1, carry)
	t4, carry = bits.Add64(t4, l1, 0)
	t5, carry = bits.Add64(t5, x[7], carry)
	t6 = carry

	// Add result to lower limbs and propagate carries.
	//
	// It is safe to discard the carry on t6 since the resulting t6 ≤ 1 as
	// previously described.
	t0, carry = bits.Add64(t0, x[0], 0)
	t1, carry = bits.Add64(t1, x[1], carry)
	t2, carry = bits.Add64(t2, x[2], carry)
	t3, carry = bits.Add64(t3, x[3], carry)
	t4, carry = bits.Add64(t4, 0, carry)
	t5, carry = bits.Add64(t5, 0, carry)
	t6, _ = bits.Add64(t6, 0, carry)

	// -------------------------------------------------------------------------
	// Second reduction.
	//
	// The value now fits in 385 bits, so reduce it again.  Only t4, t5, and t6
	// need to be considered since the higher limb, t7, is ≥ 448 bits and thus
	// guaranteed to be 0.  Further, as previously noted, t6 ≤ 1.
	//
	// The code in this section is equivalent to following if uint512 and larger
	// numbers and shifts were supported:
	//
	// t0 = uint64(t%2^256 + (t>>256)*c)
	// t1 = uint64((t%2^256 + (t>>256)*c) >> 64)
	// t2 = uint64((t%2^256 + (t>>256)*c) >> 128)
	// t3 = uint64((t%2^256 + (t>>256)*c) >> 192)
	// t4 = uint64((t%2^256 + (t>>256)*c) >> 256)
	//
	// To visualize the logic, multiply the 3 limbs of c by the new upper limbs
	// via the standard pencil-and-paper method:
	//
	//   2^512 2^448 2^384 2^320 2^256 (place values)
	//   -----------------------------
	//                  t6    t5    t4
	//                  c2    c1    c0 *
	//   -----------------------------
	//                t4c2  t4c1  t4c0
	//          t5c2  t5c1  t5c0       +
	//    t6c2  t6c1  t6c0             +
	//    ----------------------------
	//    t6c2  ....  ....  ....  t4c0
	//
	// Then add them to the associated lower limbs per the reduction identity
	// noting that the code reuses t for the sums:
	//
	//   2^256 2^192 2^128  2^64   2^0 (place values)
	//   -----------------------------
	//            t3    t2    t1    t0
	//                t4c2  t4c1  t4c0 +
	//          t5c2  t5c1  t5c0       +
	//    t6c2  t6c1  t6c0             +
	//   -----------------------------
	//      t4    t3    t2    t1    t0
	//
	// With a loose bound for t4 ≤ 3.
	//
	// Proof:
	//
	// Known: c2=1, t6 ≤ 1, t3 ≤ 2^64-1, t5 ≤ 2^64-1, and c1 ≤ 2^63-1.
	//
	// Let s = t3 + t5c2 + t6c1.  Then:
	//   s ≤ 2^64-1 + (2^64-1)*1 + 1*(2^63-1) = 2^65 + 2^63 - 3
	//
	// So, the loose bound on the carry in to t4 is carryIn ≤ floor(s/2^64) = 2.
	//
	// Finally, t4 ≤ t6*c2 + carryIn ≤ 1*1 + 2 ≤ 3.
	//
	// Thus 0 ≤ t < 2^258 after the 2nd reduction.
	// -------------------------------------------------------------------------

	// Compute t4*c with result added to t0..t2 with carries propagated and the
	// final carry in t4.
	//
	// It is safe to discard the carry on the final t4 given the carries ≤ 1.
	h0, l0 = bits.Mul64(t4, orderComplementLimb0)
	h1, l1 = bits.Mul64(t4, orderComplementLimb1)
	l2 = t4 // h2, l2 = bits.Mul64(t4, orderComplementLimb2) => h2=0, l2=t4
	t0, carry = bits.Add64(t0, l0, 0)
	t1, carry = bits.Add64(t1, h0, carry)
	t2, carry = bits.Add64(t2, h1, carry)
	t3, carry = bits.Add64(t3, 0, carry)
	t4 = carry
	t1, carry = bits.Add64(t1, l1, 0)
	t2, carry = bits.Add64(t2, l2, carry)
	t3, carry = bits.Add64(t3, 0, carry)
	t4, _ = bits.Add64(t4, 0, carry)

	// Compute t5*c with result added to t1..t3 with carries propagated.
	//
	// It is safe to discard the carry on the final t4 given the carries ≤ 1.
	h0, l0 = bits.Mul64(t5, orderComplementLimb0)
	h1, l1 = bits.Mul64(t5, orderComplementLimb1)
	// h2, l2 = bits.Mul64(t5, orderComplementLimb2) => h2=0, l2=t5
	t1, carry = bits.Add64(t1, l0, 0)
	t2, carry = bits.Add64(t2, h0, carry)
	t3, carry = bits.Add64(t3, h1, carry)
	t4, _ = bits.Add64(t4, 0, carry)
	t2, carry = bits.Add64(t2, l1, 0)
	t3, carry = bits.Add64(t3, t5, carry)
	t4, _ = bits.Add64(t4, 0, carry)

	// Compute t6*c with result added to t2..t4 with carries propagated.
	//
	// Note that since t6 ≤ 1, the product can't overflow a uint64, so it is
	// safe to use normal 64-bit multiplication.
	//
	// It is safe to discard the carry on t4 since the resulting t4 ≤ 3 as
	// previously described.
	t2, carry = bits.Add64(t2, t6*orderComplementLimb0, 0)
	t3, carry = bits.Add64(t3, t6*orderComplementLimb1, carry)
	t4, _ = bits.Add64(t4, t6, carry)

	// -------------------------------------------------------------------------
	// Third reduction.
	//
	// The value now fits in 258 bits, so reduce it again.  Only t4 needs to be
	// considered since the higher limbs are ≥ 320 bits and thus guaranteed to
	// be 0.
	//
	// The code in this section is equivalent to following if uint512 and larger
	// numbers and shifts were supported:
	//
	// t0 = uint64(t%2^256 + (t>>256)*c)
	// t1 = uint64((t%2^256 + (t>>256)*c) >> 64)
	// t2 = uint64((t%2^256 + (t>>256)*c) >> 128)
	// t3 = uint64((t%2^256 + (t>>256)*c) >> 192)
	// t4 = uint64((t%2^256 + (t>>256)*c) >> 256)
	//
	// To visualize the logic, multiply the 3 limbs of c by the new upper limb
	// via the standard pencil-and-paper method:
	//
	//   2^512 2^448 2^384 2^320 2^256 (place values)
	//   -----------------------------
	//                              t4
	//                  c2    c1    c0 *
	//   -----------------------------
	//                t4c2  t4c1  t4c0
	//
	// Then add them to the associated lower limbs noting that the code reuses t
	// for the sums:
	//
	//   2^256 2^192 2^128  2^64   2^0 (place values)
	//   -----------------------------
	//            t3    t2    t1    t0
	//                t4c2  t4c1  t4c0 +
	//   -----------------------------
	//      t4    t3    t2    t1    t0
	//
	// Where t4 ≤ 1 for the carry.
	// -------------------------------------------------------------------------

	// Compute t4*c with result added to t0..t2 with carries propagated and the
	// final carry in t4.
	//
	// Note that since current t4 ≤ 3, these bounds for the product hold:
	//   t4*c0 < 2^64
	//   t4*c1 < 2^64
	//
	// So, uint64 overflow is impossible and therefore it is safe to use normal
	// 64-bit multiplication.
	t0, carry = bits.Add64(t0, t4*orderComplementLimb0, 0)
	t1, carry = bits.Add64(t1, t4*orderComplementLimb1, carry)
	t2, carry = bits.Add64(t2, t4, carry)
	t3, carry = bits.Add64(t3, 0, carry)
	t4 = carry

	// -------------------------------------------------------------------------
	// Final reduction.
	//
	// The value is now in the range 0 ≤ t < 2N, so one 5-limb conditional
	// subtract of N guarantees it is fully reduced.
	// -------------------------------------------------------------------------

	// Subtract N with borrow propagation.  borrow is set iff t < N.
	//
	// In other words, the value overflowed (≥ N) when t - N does NOT borrow.
	//
	// s = t - N
	var s0, s1, s2, s3, borrow uint64
	s0, borrow = bits.Sub64(t0, orderLimb0, 0)
	s1, borrow = bits.Sub64(t1, orderLimb1, borrow)
	s2, borrow = bits.Sub64(t2, orderLimb2, borrow)
	s3, borrow = bits.Sub64(t3, orderLimb3, borrow)
	_, borrow = bits.Sub64(t4, 0, borrow)

	// Constant-time select.
	//
	// Set r = t only when t < N (borrow set).
	// Otherwise r = s = t - N.
	r[0] = constantTimeSelect64(borrow, t0, s0)
	r[1] = constantTimeSelect64(borrow, t1, s1)
	r[2] = constantTimeSelect64(borrow, t2, s2)
	r[3] = constantTimeSelect64(borrow, t3, s3)
}

// Mul2 multiplies the passed two scalars together modulo the group order in
// constant time and stores the result in s.
//
// The scalar is returned to support chaining.  This enables syntax like:
// s3.Mul2(s, s2).AddInt(1) so that s3 = (s * s2) + 1.
func (s *ModNScalar) Mul2(a, b *ModNScalar) *ModNScalar {
	var product [8]uint64
	field64Mul512(&product, &a.n, &b.n)
	scalar64Reduce512(&s.n, &product)
	return s
}

// Mul multiplies the passed scalar with the existing one modulo the group order
// in constant time and stores the result in s.
//
// The scalar is returned to support chaining.  This enables syntax like:
// s.Mul(s2).AddInt(1) so that s = (s * s2) + 1.
func (s *ModNScalar) Mul(val *ModNScalar) *ModNScalar {
	return s.Mul2(s, val)
}

// SquareVal squares the passed scalar modulo the group order in constant time
// and stores the result in s.
//
// The scalar is returned to support chaining.  This enables syntax like:
// s3.SquareVal(s).Mul(s) so that s3 = s^2 * s = s^3.
func (s *ModNScalar) SquareVal(val *ModNScalar) *ModNScalar {
	var product [8]uint64
	field64Square512(&product, &val.n)
	scalar64Reduce512(&s.n, &product)
	return s
}

// Square squares the scalar modulo the group order in constant time.  The
// existing scalar is modified.
//
// The scalar is returned to support chaining.  This enables syntax like:
// s.Square().Mul(s2) so that s = s^2 * s2.
func (s *ModNScalar) Square() *ModNScalar {
	return s.SquareVal(s)
}

// NegateVal negates the passed scalar modulo the group order and stores the
// result in s in constant time.
//
// The scalar is returned to support chaining.  This enables syntax like:
// s.NegateVal(s2).AddInt(1) so that s = -s2 + 1.
func (s *ModNScalar) NegateVal(val *ModNScalar) *ModNScalar {
	// Since the scalar is already in the range 0 <= val < N, where N is the
	// group order, negation modulo the group order is just the group order
	// minus the value.  This implies that the result will always be in the
	// desired range with the sole exception of 0 because N - 0 = N itself.
	//
	// The following handles that case in constant time by creating a mask that
	// is all 0s in the case the scalar being negated is 0 and all 1s otherwise
	// and then bitwise ands that mask with each limb.
	//
	// This approach was chosen over subtracting from 0 and then conditionally
	// adding N because it's over twice as fast on 32-bit hardware while only
	// being about 3-4% slower on 64-bit hardware.
	//
	// Determine mask first to allow aliasing.
	mask := -uint64(constantTimeNotEq64(val.n[0]|val.n[1]|val.n[2]|val.n[3], 0))

	// Unconditionally subtract the scalar from the group order.
	//
	// To simplify the carry propagation, this adds the two's complement of the
	// scalar to N in order to achieve the same result.
	//
	// s = N - val
	var c uint64
	s.n[0], c = bits.Add64(orderLimb0, ^val.n[0], 1)
	s.n[1], c = bits.Add64(orderLimb1, ^val.n[1], c)
	s.n[2], c = bits.Add64(orderLimb2, ^val.n[2], c)
	s.n[3], _ = bits.Add64(orderLimb3, ^val.n[3], c)

	// Either keep the result when val != 0 or clear it when val == 0.  The
	// result is either:
	//
	// val == 0: s = 0
	// val != 0: s = N - val
	s.n[0] &= mask
	s.n[1] &= mask
	s.n[2] &= mask
	s.n[3] &= mask
	return s
}

// Negate negates the scalar modulo the group order in constant time.  The
// existing scalar is modified.
//
// The scalar is returned to support chaining.  This enables syntax like:
// s.Negate().AddInt(1) so that s = -s + 1.
func (s *ModNScalar) Negate() *ModNScalar {
	return s.NegateVal(s)
}

// intPool is used to reduce allocations of [big.Int] limbs while calculating
// inverse modulo on [ModNScalar].
type intPool struct {
	pool sync.Pool
}

func (i *intPool) put(v *big.Int) {
	v.SetInt64(0)
	i.pool.Put(v)
}

func (i *intPool) get() *big.Int {
	return i.pool.Get().(*big.Int)
}

var intP = intPool{
	pool: sync.Pool{
		New: func() interface{} {
			return new(big.Int)
		},
	},
}

// InverseValNonConst finds the modular multiplicative inverse of the passed
// scalar and stores result in s in *non-constant* time.
//
// The scalar is returned to support chaining.  This enables syntax like:
// s3.InverseVal(s1).Mul(s2) so that s3 = s1^-1 * s2.
func (s *ModNScalar) InverseValNonConst(val *ModNScalar) *ModNScalar {
	// This is making use of big integers for now.  Ideally it will be replaced
	// with an implementation that does not depend on big integers.
	valBytes := val.Bytes()

	// Reuse [big.Int] to avoid limb allocation.
	bigVal := intP.get().SetBytes(valBytes[:])
	bigVal.ModInverse(bigVal, curveParams.N)
	var bigBytes [32]byte
	bigVal.FillBytes(bigBytes[:])
	s.SetBytes(&bigBytes)

	// Cleanup.
	zeroArray32(&bigBytes)
	intP.put(bigVal)

	return s
}

// InverseNonConst finds the modular multiplicative inverse of the scalar in
// *non-constant* time.  The existing scalar is modified.
//
// The scalar is returned to support chaining.  This enables syntax like:
// s.Inverse().Mul(s2) so that s = s^-1 * s2.
func (s *ModNScalar) InverseNonConst() *ModNScalar {
	return s.InverseValNonConst(s)
}

// IsOverHalfOrder returns whether or not the scalar exceeds the group order
// divided by 2 in constant time.
func (s *ModNScalar) IsOverHalfOrder() bool {
	// These fields provide convenient access to each of the limbs of the
	// secp256k1 curve group order N / 2 to improve code readability and avoid
	// the need to recalculate them.
	//
	// The half order of the secp256k1 curve group is:
	// 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0
	//
	// Converting that to field representation (base 2^64) is:
	//
	// n[0] = 0xdfe92f46681b20a0
	// n[1] = 0x5d576e7357a4501d
	// n[2] = 0xffffffffffffffff
	// n[3] = 0x7fffffffffffffff
	//
	// This can be verified with the following test code:
	//   halfOrder := new(big.Int).Div(curveParams.N, big.NewInt(2))
	//   var s ModNScalar
	//   s.SetByteSlice(halfOrder.Bytes())
	//   t.Logf("%x", s.n)
	const (
		halfOrderLimb0 uint64 = 0xdfe92f46681b20a0
		halfOrderLimb1 uint64 = 0x5d576e7357a4501d
		halfOrderLimb2 uint64 = 0xffffffffffffffff
		halfOrderLimb3 uint64 = 0x7fffffffffffffff
	)

	// The goal is to return true when the scalar is greater than half of the
	// group order.  That is, return true when s > N/2, which is trivially
	// rearranged to s - (N/2 + 1) ≥ 0.
	//
	// In other words, the condition is met iff subtracting (N/2 + 1) from s is
	// non-negative (aka there was no borrow).
	var borrow uint64
	_, borrow = bits.Sub64(s.n[0], halfOrderLimb0+1, borrow)
	_, borrow = bits.Sub64(s.n[1], halfOrderLimb1, borrow)
	_, borrow = bits.Sub64(s.n[2], halfOrderLimb2, borrow)
	_, borrow = bits.Sub64(s.n[3], halfOrderLimb3, borrow)
	return borrow == 0
}
