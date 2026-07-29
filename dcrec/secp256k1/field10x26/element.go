// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2026 The Decred developers
// Copyright (c) 2013-2026 Dave Collins
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package field10x26 implements highly optimized, constant-time arithmetic
// over the secp256k1 finite field using a 10x26 representation with bounded
// overflow between normalizations.
package field10x26

import (
	"encoding/hex"

	"github.com/decred/dcrd/dcrec/secp256k1/v4/internal/arith"
)

// References:
//   [HAC]: Handbook of Applied Cryptography Menezes, van Oorschot, Vanstone.
//     https://cacr.uwaterloo.ca/hac/

// All elliptic curve operations for secp256k1 are done in a finite field
// characterized by a 256-bit prime.  Given this precision is larger than the
// biggest available native type, obviously some form of bignum math is needed.
// This package implements specialized fixed-precision field arithmetic rather
// than relying on an arbitrary-precision arithmetic package such as math/big
// for dealing with the field math since the size is known.  As a result, rather
// large performance gains are achieved by taking advantage of many
// optimizations not available to arbitrary-precision arithmetic and generic
// modular arithmetic algorithms.
//
// There are various internal representations for finite field elements.  For
// example, the most obvious representation would be to use an array of 4
// uint64s (aka 4x64: 64 bits * 4 = 256 bits).  However, at the time this
// implementation was written, Go did not have access to hardware intrinsics, so
// that representation suffered from a couple of issues.  First, there is no
// native Go type large enough to handle the intermediate results while adding
// or multiplying two 64-bit numbers, and second there is no space left for
// overflows when performing the intermediate arithmetic between each array
// element which would lead to expensive carry propagation.
//
// While both of those things are still true without intrinsics, modern Go now
// provides access to intrinsics that permit the hardware to perform both full
// 128-bit products and addition with carry which entirely mitigates those
// limitations.  As a result, there is now an alternative [secp256k1.FieldVal64]
// implementation that uses the aforementioned 4x64 representation with
// intrinsics.
//
// Given those limitations, this implementation represents the field elements as
// 10 uint32s with each limb (array entry) treated as base 2^26.  This was
// chosen for the following reasons:
// 1) Most systems at the current time are 64-bit (or at least have 64-bit
//    registers available for specialized purposes such as MMX) so the
//    intermediate results can typically be done using a native register (and
//    using uint64s to avoid the need for additional half-word arithmetic)
// 2) In order to allow addition of the internal limbs without having to
//    propagate the carry, the max normalized value for each register must
//    be less than the number of bits available in the register
// 3) Since we're dealing with 32-bit values, 64-bits of overflow is a
//    reasonable choice for #2
// 4) Given the need for 256-bits of precision and the properties stated in #1,
//    #2, and #3, the representation which best accommodates this is 10 uint32s
//    with base 2^26 (26 bits * 10 = 260 bits, so the final limb only needs 22
//    bits) which leaves the desired 64 bits (32 * 10 = 320, 320 - 256 = 64) for
//    overflow
//
// Since it is so important that the field arithmetic is extremely fast for high
// performance crypto, this type does not perform any validation where it
// ordinarily would.  See the documentation for [Element] for more details.

// Constants used to make the code more readable.
const (
	twoBitsMask   = 0x3
	fourBitsMask  = 0xf
	sixBitsMask   = 0x3f
	eightBitsMask = 0xff
)

// Constants related to the internal representation.
const (
	// fieldLimbs is the number of limbs used to internally represent the
	// 256-bit value.
	fieldLimbs = 10

	// fieldBase is the exponent used to form the numeric base of each limb.
	// 2^(fieldBase*i) where i is the limb position.
	fieldBase = 26

	// fieldBaseMask is the mask for the bits in each limb needed to
	// represent the numeric base of each limb (except the most significant
	// limb).
	fieldBaseMask = (1 << fieldBase) - 1

	// fieldMSBBits is the number of bits in the most significant limb used
	// to represent the value.
	fieldMSBBits = 256 - (fieldBase * (fieldLimbs - 1))

	// fieldMSBMask is the mask for the bits in the most significant limb
	// needed to represent the value.
	fieldMSBMask = (1 << fieldMSBBits) - 1

	// These fields provide convenient access to each of the limbs of the
	// secp256k1 prime in the internal representation to improve code
	// readability.
	fieldPrimeLimb0 = 0x03fffc2f
	fieldPrimeLimb1 = 0x03ffffbf
	fieldPrimeLimb2 = 0x03ffffff
	fieldPrimeLimb3 = 0x03ffffff
	fieldPrimeLimb4 = 0x03ffffff
	fieldPrimeLimb5 = 0x03ffffff
	fieldPrimeLimb6 = 0x03ffffff
	fieldPrimeLimb7 = 0x03ffffff
	fieldPrimeLimb8 = 0x03ffffff
	fieldPrimeLimb9 = 0x003fffff
)

// Element implements optimized fixed-precision arithmetic over the secp256k1
// finite field.  This means all arithmetic is performed modulo
//
//	0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc2f
//
// WARNING: Since it is so important for the field arithmetic to be extremely
// fast for high performance crypto, this type does not perform any validation
// of documented preconditions where it ordinarily would.  As a result, it is
// IMPERATIVE for callers to understand some key concepts that are described
// below and ensure the methods are called with the necessary preconditions that
// each method is documented with.  For example, some methods only give the
// correct result if the element is normalized and others require the elements
// involved to have a maximum magnitude and THERE ARE NO EXPLICIT CHECKS TO
// ENSURE THOSE PRECONDITIONS ARE SATISFIED.  This does, unfortunately, make the
// type more difficult to use correctly and while it is typically preferable to
// ensure all state and input is valid for most code, this is a bit of an
// exception because those extra checks really add up in what ends up being
// critical hot paths.
//
// The first key concept when working with this type is normalization.  In order
// to avoid the need to propagate a ton of carries, the internal representation
// provides additional overflow bits for each limb of the overall 256-bit value.
// This means that there are multiple internal representations for the same
// value and, as a result, any methods that rely on comparison of the value,
// such as equality and oddness determination, require the caller to provide a
// normalized field element.
//
// The second key concept when working with this type is magnitude.  As
// previously mentioned, the internal representation provides additional
// overflow bits which means that the more math operations that are performed on
// the element between normalizations, the more those overflow bits accumulate.
// The magnitude is effectively that maximum possible number of those overflow
// bits that could possibly be required as a result of a given operation.  Since
// there are only a limited number of overflow bits available, this implies that
// the max possible magnitude MUST be tracked by the caller and the caller MUST
// normalize the element if a given operation would cause the magnitude of the
// result to exceed the max allowed value.
//
// IMPORTANT: The max allowed magnitude of an element is 32.
type Element struct {
	// Each 256-bit value is represented as 10 32-bit integers in base 2^26.
	// This provides 6 bits of overflow in each limb (10 bits in the most
	// significant limb) for a total of 64 bits of overflow (9*6 + 10 = 64).  It
	// only implements the arithmetic needed for elliptic curve operations.
	//
	// The following depicts the internal representation:
	// 	 -----------------------------------------------------------------
	// 	|        n[9]       |        n[8]       | ... |        n[0]       |
	// 	| 32 bits available | 32 bits available | ... | 32 bits available |
	// 	| 22 bits for value | 26 bits for value | ... | 26 bits for value |
	// 	| 10 bits overflow  |  6 bits overflow  | ... |  6 bits overflow  |
	// 	| Mult: 2^(26*9)    | Mult: 2^(26*8)    | ... | Mult: 2^(26*0)    |
	// 	 -----------------------------------------------------------------
	//
	// For example, consider the number 2^49 + 1.  It would be represented as:
	// 	n[0] = 1
	// 	n[1] = 2^23
	// 	n[2..9] = 0
	//
	// The full 256-bit value is then calculated by looping i from 9..0 and
	// doing sum(n[i] * 2^(26i)) like so:
	// 	n[9] * 2^(26*9) = 0    * 2^234 = 0
	// 	n[8] * 2^(26*8) = 0    * 2^208 = 0
	// 	...
	// 	n[1] * 2^(26*1) = 2^23 * 2^26  = 2^49
	// 	n[0] * 2^(26*0) = 1    * 2^0   = 1
	// 	Sum: 0 + 0 + ... + 2^49 + 1 = 2^49 + 1
	n [10]uint32
}

// String returns the element as a normalized human-readable hex string.
//
//	Preconditions: None
//	Output Normalized: Element is not modified -- same as input element
//	Output Max Magnitude: Element is not modified -- same as input element
func (e Element) String() string {
	// e is a copy, so it's safe to normalize it without mutating the original.
	e.Normalize()
	return hex.EncodeToString(e.Bytes()[:])
}

// Zero sets the element to zero in constant time.  A newly created element is
// already set to zero.  This function can be useful to clear an existing
// element for reuse.
//
//	Preconditions: None
//	Output Normalized: Yes
//	Output Max Magnitude: 1
func (e *Element) Zero() {
	e.n = [10]uint32{}
}

// Set sets the element equal to the passed element in constant time.  The
// normalization and magnitude of the two elements will be identical.
//
// The element is returned to support chaining.  This enables syntax like:
// e := new(Element).Set(e2).Add(1) so that e = e2 + 1 where e2 is not modified.
//
//	Preconditions: None
//	Output Normalized: Same as input element
//	Output Max Magnitude: Same as input element
func (e *Element) Set(val *Element) *Element {
	*e = *val
	return e
}

// SetInt sets the element to the passed integer in constant time.  This is a
// convenience function since it is fairly common to perform arithmetic with
// small native integers.
//
// The element is returned to support chaining.  This enables syntax such
// as e := new(Element).SetInt(2).Mul(e2) so that e = 2 * e2.
//
//	Preconditions: None
//	Output Normalized: Yes
//	Output Max Magnitude: 1
func (e *Element) SetInt(ui uint16) *Element {
	e.Zero()
	e.n[0] = uint32(ui)
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
//
//	Preconditions: None
//	Output Normalized: Yes when no overflow, No otherwise
//	Output Max Magnitude: 1
func (e *Element) SetBytes(b *[32]byte) uint32 {
	// Pack the 256 total bits across the 10 uint32 limbs with at most
	// 26-bits per limb.  This could be done with a couple of for loops,
	// but this unrolled version is significantly faster.  Benchmarks show
	// this is about 34 times faster than the variant which uses loops.
	e.n[0] = uint32(b[31]) | uint32(b[30])<<8 | uint32(b[29])<<16 |
		(uint32(b[28])&twoBitsMask)<<24
	e.n[1] = uint32(b[28])>>2 | uint32(b[27])<<6 | uint32(b[26])<<14 |
		(uint32(b[25])&fourBitsMask)<<22
	e.n[2] = uint32(b[25])>>4 | uint32(b[24])<<4 | uint32(b[23])<<12 |
		(uint32(b[22])&sixBitsMask)<<20
	e.n[3] = uint32(b[22])>>6 | uint32(b[21])<<2 | uint32(b[20])<<10 |
		uint32(b[19])<<18
	e.n[4] = uint32(b[18]) | uint32(b[17])<<8 | uint32(b[16])<<16 |
		(uint32(b[15])&twoBitsMask)<<24
	e.n[5] = uint32(b[15])>>2 | uint32(b[14])<<6 | uint32(b[13])<<14 |
		(uint32(b[12])&fourBitsMask)<<22
	e.n[6] = uint32(b[12])>>4 | uint32(b[11])<<4 | uint32(b[10])<<12 |
		(uint32(b[9])&sixBitsMask)<<20
	e.n[7] = uint32(b[9])>>6 | uint32(b[8])<<2 | uint32(b[7])<<10 |
		uint32(b[6])<<18
	e.n[8] = uint32(b[5]) | uint32(b[4])<<8 | uint32(b[3])<<16 |
		(uint32(b[2])&twoBitsMask)<<24
	e.n[9] = uint32(b[2])>>2 | uint32(b[1])<<6 | uint32(b[0])<<14

	// The intuition here is that the element is greater than the prime if one
	// of the higher individual limbs is greater than corresponding limb of the
	// prime and all higher limbs in the element are equal to their
	// corresponding limb of the prime.  Since this type is modulo the prime,
	// being equal is also an overflow back to 0.
	//
	// Note that because the input is 32 bytes and it was just packed into the
	// internal representation, the only limbs that can possibly be greater are
	// zero and one, because ceil(log_2(2^256 - 1 - P)) = 33 bits max and the
	// internal representation encodes 26 bits with each limb.
	//
	// Thus, there is no need to test if the upper limbs of the element exceeds
	// them, hence, only equality is checked for them.
	highLimbsEq := arith.ConstantTimeEq(e.n[9], fieldPrimeLimb9)
	highLimbsEq &= arith.ConstantTimeEq(e.n[8], fieldPrimeLimb8)
	highLimbsEq &= arith.ConstantTimeEq(e.n[7], fieldPrimeLimb7)
	highLimbsEq &= arith.ConstantTimeEq(e.n[6], fieldPrimeLimb6)
	highLimbsEq &= arith.ConstantTimeEq(e.n[5], fieldPrimeLimb5)
	highLimbsEq &= arith.ConstantTimeEq(e.n[4], fieldPrimeLimb4)
	highLimbsEq &= arith.ConstantTimeEq(e.n[3], fieldPrimeLimb3)
	highLimbsEq &= arith.ConstantTimeEq(e.n[2], fieldPrimeLimb2)
	overflow := highLimbsEq & arith.ConstantTimeGreater(e.n[1], fieldPrimeLimb1)
	highLimbsEq &= arith.ConstantTimeEq(e.n[1], fieldPrimeLimb1)
	overflow |= highLimbsEq & arith.ConstantTimeGreaterOrEq(e.n[0], fieldPrimeLimb0)

	return overflow
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
//
//	Preconditions: None
//	Output Normalized: Yes when no overflow, No otherwise
//	Output Max Magnitude: 1
func (e *Element) SetByteSlice(b []byte) bool {
	var b32 [32]byte
	b = b[:arith.ConstantTimeMin(uint32(len(b)), 32)]
	copy(b32[:], b32[:32-len(b)])
	copy(b32[32-len(b):], b)
	result := e.SetBytes(&b32)
	zeroArray32(&b32)
	return result != 0
}

// Normalize normalizes the internal limbs into the desired range and performs
// fast modular reduction over the secp256k1 prime by making use of the special
// form of the prime in constant time.
//
//	Preconditions: None
//	Output Normalized: Yes
//	Output Max Magnitude: 1
func (e *Element) Normalize() *Element {
	// The internal representation leaves 6 bits of overflow in each limb so
	// intermediate calculations can be performed without needing to propagate
	// the carry to each higher limb during the calculations.  In order to
	// normalize, we need to "compact" the full 256-bit value to the right while
	// propagating any carries through to the high order limb.
	//
	// Since this is doing arithmetic modulo the secp256k1 prime, we also need
	// to perform modular reduction over the prime.
	//
	// Per [HAC] section 14.3.4: Reduction method of moduli of special form,
	// when the modulus is of the special form m = b^t - c, highly efficient
	// reduction can be achieved.
	//
	// The secp256k1 prime is equivalent to 2^256 - 4294968273, so it fits
	// this criteria.
	//
	// 4294968273 in internal representation (base 2^26) is:
	// n[0] = 977
	// n[1] = 64
	// That is to say (2^26 * 64) + 977 = 4294968273
	//
	// The algorithm presented in the referenced section typically repeats until
	// the quotient is zero.  However, due to our internal representation we
	// already know to within one reduction how many times we would need to
	// repeat as it's the uppermost bits of the high order limb.  Thus we can
	// simply multiply the magnitude by the internal representation of the prime
	// and do a single iteration.  After this step there might be an additional
	// carry to bit 256 (bit 22 of the high order limb).
	t9 := e.n[9]
	m := t9 >> fieldMSBBits
	t9 &= fieldMSBMask
	t0 := e.n[0] + m*977
	t1 := (t0 >> fieldBase) + e.n[1] + (m << 6)
	t0 &= fieldBaseMask
	t2 := (t1 >> fieldBase) + e.n[2]
	t1 &= fieldBaseMask
	t3 := (t2 >> fieldBase) + e.n[3]
	t2 &= fieldBaseMask
	t4 := (t3 >> fieldBase) + e.n[4]
	t3 &= fieldBaseMask
	t5 := (t4 >> fieldBase) + e.n[5]
	t4 &= fieldBaseMask
	t6 := (t5 >> fieldBase) + e.n[6]
	t5 &= fieldBaseMask
	t7 := (t6 >> fieldBase) + e.n[7]
	t6 &= fieldBaseMask
	t8 := (t7 >> fieldBase) + e.n[8]
	t7 &= fieldBaseMask
	t9 = (t8 >> fieldBase) + t9
	t8 &= fieldBaseMask

	// At this point, the magnitude is guaranteed to be one, however, the
	// element could still be greater than the prime if there was either a
	// carry through to bit 256 (bit 22 of the higher order limb) or the
	// element is greater than or equal to the field characteristic.  The
	// following determines if either or these conditions are true and does
	// the final reduction in constant time.
	//
	// Also note that 'm' will be zero when neither of the aforementioned
	// conditions are true and the element will not be changed when 'm' is zero.
	m = arith.ConstantTimeEq(t9, fieldMSBMask)
	m &= arith.ConstantTimeEq(t8&t7&t6&t5&t4&t3&t2, fieldBaseMask)
	m &= arith.ConstantTimeGreater(t1+64+((t0+977)>>fieldBase), fieldBaseMask)
	m |= t9 >> fieldMSBBits
	t0 += m * 977
	t1 = (t0 >> fieldBase) + t1 + (m << 6)
	t0 &= fieldBaseMask
	t2 = (t1 >> fieldBase) + t2
	t1 &= fieldBaseMask
	t3 = (t2 >> fieldBase) + t3
	t2 &= fieldBaseMask
	t4 = (t3 >> fieldBase) + t4
	t3 &= fieldBaseMask
	t5 = (t4 >> fieldBase) + t5
	t4 &= fieldBaseMask
	t6 = (t5 >> fieldBase) + t6
	t5 &= fieldBaseMask
	t7 = (t6 >> fieldBase) + t7
	t6 &= fieldBaseMask
	t8 = (t7 >> fieldBase) + t8
	t7 &= fieldBaseMask
	t9 = (t8 >> fieldBase) + t9
	t8 &= fieldBaseMask
	t9 &= fieldMSBMask // Remove potential multiple of 2^256.

	// Finally, set the normalized and reduced limbs.
	e.n[0] = t0
	e.n[1] = t1
	e.n[2] = t2
	e.n[3] = t3
	e.n[4] = t4
	e.n[5] = t5
	e.n[6] = t6
	e.n[7] = t7
	e.n[8] = t8
	e.n[9] = t9
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
//
//	Preconditions:
//	  - The element MUST be normalized
//	  - The target slice MUST have at least 32 bytes available
func (e *Element) PutBytesUnchecked(b []byte) {
	// Unpack the 256 total bits from the 10 uint32 limbs with at most
	// 26-bits per limb.  This could be done with a couple of for loops,
	// but this unrolled version is a bit faster.  Benchmarks show this is
	// about 10 times faster than the variant which uses loops.
	b[31] = byte(e.n[0] & eightBitsMask)
	b[30] = byte((e.n[0] >> 8) & eightBitsMask)
	b[29] = byte((e.n[0] >> 16) & eightBitsMask)
	b[28] = byte((e.n[0]>>24)&twoBitsMask | (e.n[1]&sixBitsMask)<<2)
	b[27] = byte((e.n[1] >> 6) & eightBitsMask)
	b[26] = byte((e.n[1] >> 14) & eightBitsMask)
	b[25] = byte((e.n[1]>>22)&fourBitsMask | (e.n[2]&fourBitsMask)<<4)
	b[24] = byte((e.n[2] >> 4) & eightBitsMask)
	b[23] = byte((e.n[2] >> 12) & eightBitsMask)
	b[22] = byte((e.n[2]>>20)&sixBitsMask | (e.n[3]&twoBitsMask)<<6)
	b[21] = byte((e.n[3] >> 2) & eightBitsMask)
	b[20] = byte((e.n[3] >> 10) & eightBitsMask)
	b[19] = byte((e.n[3] >> 18) & eightBitsMask)
	b[18] = byte(e.n[4] & eightBitsMask)
	b[17] = byte((e.n[4] >> 8) & eightBitsMask)
	b[16] = byte((e.n[4] >> 16) & eightBitsMask)
	b[15] = byte((e.n[4]>>24)&twoBitsMask | (e.n[5]&sixBitsMask)<<2)
	b[14] = byte((e.n[5] >> 6) & eightBitsMask)
	b[13] = byte((e.n[5] >> 14) & eightBitsMask)
	b[12] = byte((e.n[5]>>22)&fourBitsMask | (e.n[6]&fourBitsMask)<<4)
	b[11] = byte((e.n[6] >> 4) & eightBitsMask)
	b[10] = byte((e.n[6] >> 12) & eightBitsMask)
	b[9] = byte((e.n[6]>>20)&sixBitsMask | (e.n[7]&twoBitsMask)<<6)
	b[8] = byte((e.n[7] >> 2) & eightBitsMask)
	b[7] = byte((e.n[7] >> 10) & eightBitsMask)
	b[6] = byte((e.n[7] >> 18) & eightBitsMask)
	b[5] = byte(e.n[8] & eightBitsMask)
	b[4] = byte((e.n[8] >> 8) & eightBitsMask)
	b[3] = byte((e.n[8] >> 16) & eightBitsMask)
	b[2] = byte((e.n[8]>>24)&twoBitsMask | (e.n[9]&sixBitsMask)<<2)
	b[1] = byte((e.n[9] >> 6) & eightBitsMask)
	b[0] = byte((e.n[9] >> 14) & eightBitsMask)
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
//
//	Preconditions:
//	  - The element MUST be normalized
func (e *Element) PutBytes(b *[32]byte) {
	e.PutBytesUnchecked(b[:])
}

// Bytes unpacks the element to a 32-byte big-endian value in constant time.
//
// See [Element.PutBytes] and [Element.PutBytesUnchecked] for variants that
// allow an array or slice to be passed which can be useful to cut down on the
// number of allocations by allowing the caller to reuse a buffer or write
// directly into part of a larger buffer.
//
//	Preconditions:
//	  - The element MUST be normalized
func (e *Element) Bytes() *[32]byte {
	b := new([32]byte)
	e.PutBytesUnchecked(b[:])
	return b
}

// IsZeroBit returns 1 when the element is equal to zero or 0 otherwise in
// constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.  See [Element.IsZero] for the version
// that returns a bool.
//
//	Preconditions:
//	  - The element MUST be normalized
func (e *Element) IsZeroBit() uint32 {
	// The element can only be zero if no bits are set in any of the limbs.
	// This is a constant time implementation.
	bits := e.n[0] | e.n[1] | e.n[2] | e.n[3] | e.n[4] |
		e.n[5] | e.n[6] | e.n[7] | e.n[8] | e.n[9]

	return arith.ConstantTimeEq(bits, 0)
}

// IsZero returns whether or not the element is equal to zero in constant time.
//
//	Preconditions:
//	  - The element MUST be normalized
func (e *Element) IsZero() bool {
	// The element can only be zero if no bits are set in any of the limbs.
	// This is a constant time implementation.
	bits := e.n[0] | e.n[1] | e.n[2] | e.n[3] | e.n[4] |
		e.n[5] | e.n[6] | e.n[7] | e.n[8] | e.n[9]

	return bits == 0
}

// IsOneBit returns 1 when the element is equal to one or 0 otherwise in
// constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.  See [Element.IsOne] for the version
// that returns a bool.
//
//	Preconditions:
//	   - The element MUST be normalized
func (e *Element) IsOneBit() uint32 {
	// The element can only be one if the single lowest significant bit is set in
	// the first limb and no other bits are set in any of the other limbs.
	// This is a constant time implementation.
	bits := (e.n[0] ^ 1) | e.n[1] | e.n[2] | e.n[3] | e.n[4] | e.n[5] |
		e.n[6] | e.n[7] | e.n[8] | e.n[9]

	return arith.ConstantTimeEq(bits, 0)
}

// IsOne returns whether or not the element is equal to one in constant time.
//
//	Preconditions:
//	  - The element MUST be normalized
func (e *Element) IsOne() bool {
	// The element can only be one if the single lowest significant bit is set
	// in the first limb and no other bits are set in any of the other limbs.
	// This is a constant time implementation.
	bits := (e.n[0] ^ 1) | e.n[1] | e.n[2] | e.n[3] | e.n[4] | e.n[5] |
		e.n[6] | e.n[7] | e.n[8] | e.n[9]

	return bits == 0
}

// IsOddBit returns 1 when the element is an odd number or 0 otherwise in
// constant time.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.  See [Element.IsOdd] for the version
// that returns a bool.
//
//	Preconditions:
//	  - The element MUST be normalized
func (e *Element) IsOddBit() uint32 {
	// Only odd numbers have the bottom bit set.
	return e.n[0] & 1
}

// IsOdd returns whether or not the element is an odd number in constant time.
//
//	Preconditions:
//	  - The element MUST be normalized
func (e *Element) IsOdd() bool {
	// Only odd numbers have the bottom bit set.
	return e.n[0]&1 == 1
}

// Equals returns whether or not the two elements are the same in constant time.
//
//	Preconditions:
//	  - Both elements being compared MUST be normalized
func (e *Element) Equals(val *Element) bool {
	// Xor only sets bits when they are different, so the two elements can only
	// be the same if no bits are set after xoring each limb.  This is a
	// constant time implementation.
	bits := (e.n[0] ^ val.n[0]) | (e.n[1] ^ val.n[1]) | (e.n[2] ^ val.n[2]) |
		(e.n[3] ^ val.n[3]) | (e.n[4] ^ val.n[4]) | (e.n[5] ^ val.n[5]) |
		(e.n[6] ^ val.n[6]) | (e.n[7] ^ val.n[7]) | (e.n[8] ^ val.n[8]) |
		(e.n[9] ^ val.n[9])

	return bits == 0
}

// NegateVal negates the passed element and stores the result in e in constant
// time.  The caller must provide the maximum magnitude of the passed element
// for a correct result.
//
// The element is returned to support chaining.  This enables syntax like:
// e.NegateVal(e2).AddInt(1) so that e = -e2 + 1.
//
//	Preconditions:
//	  - The max magnitude MUST be 31
//	Output Normalized: No
//	Output Max Magnitude: Input magnitude + 1
func (e *Element) NegateVal(val *Element, magnitude uint32) *Element {
	// Negation in the field is just the prime minus the element.  However, in
	// order to allow negation against an element without having to
	// normalize/reduce it first, multiply by the magnitude (that is how "far"
	// away it is from its normalized representation) to adjust.  Also, since
	// negating an element pushes it one more order of magnitude away from the
	// normalized range, add 1 to compensate.
	//
	// For some intuition here, imagine you're performing mod 12 arithmetic
	// (picture a clock) and you are negating the number 7.  So you start at 12
	// (which is of course 0 under mod 12) and count backwards (left on the
	// clock) 7 times to arrive at 5.  Notice this is just 12-7 = 5.  Now,
	// assume you're starting with 19, which is a number that is already larger
	// than the modulus and congruent to 7 (mod 12).  When an element is already
	// in the desired range, its magnitude is 1.  Since 19 is an additional
	// "step", its magnitude (mod 12) is 2.  Since any multiple of the modulus
	// is congruent to zero (mod m), the answer can be shortcut by simply
	// multiplying the magnitude by the modulus and subtracting.  Keeping with
	// the example, this would be (2*12)-19 = 5.
	e.n[0] = (magnitude+1)*fieldPrimeLimb0 - val.n[0]
	e.n[1] = (magnitude+1)*fieldPrimeLimb1 - val.n[1]
	e.n[2] = (magnitude+1)*fieldBaseMask - val.n[2]
	e.n[3] = (magnitude+1)*fieldBaseMask - val.n[3]
	e.n[4] = (magnitude+1)*fieldBaseMask - val.n[4]
	e.n[5] = (magnitude+1)*fieldBaseMask - val.n[5]
	e.n[6] = (magnitude+1)*fieldBaseMask - val.n[6]
	e.n[7] = (magnitude+1)*fieldBaseMask - val.n[7]
	e.n[8] = (magnitude+1)*fieldBaseMask - val.n[8]
	e.n[9] = (magnitude+1)*fieldMSBMask - val.n[9]

	return e
}

// Negate negates the element in constant time.  The existing element is
// modified.  The caller must provide the maximum magnitude of the element for a
// correct result.
//
// The element is returned to support chaining.  This enables syntax like:
// e.Negate().AddInt(1) so that e = -e + 1.
//
//	Preconditions:
//	  - The max magnitude MUST be 31
//	Output Normalized: No
//	Output Max Magnitude: Input magnitude + 1
func (e *Element) Negate(magnitude uint32) *Element {
	return e.NegateVal(e, magnitude)
}

// AddInt adds the passed integer to the existing element and stores the result
// in e in constant time.  This is a convenience function since it is fairly
// common to perform some arithmetic with small native integers.
//
// The element is returned to support chaining.  This enables syntax like:
// e.AddInt(1).Add(e2) so that e = e + 1 + e2.
//
//	Preconditions:
//	  - The element MUST have a max magnitude of 31
//	  - The integer MUST be at most 32767
//	Output Normalized: No
//	Output Max Magnitude: Existing element magnitude + 1
func (e *Element) AddInt(ui uint16) *Element {
	// Since the internal representation intentionally provides overflow bits,
	// it's ok to use carryless addition as the carry bit is safely part of the
	// limb and will be normalized out.
	e.n[0] += uint32(ui)

	return e
}

// Add adds the passed element to the existing element and stores the result in
// e in constant time.
//
// The element is returned to support chaining.  This enables syntax like:
// e.Add(e2).AddInt(1) so that e = e + e2 + 1.
//
//	Preconditions:
//	  - The sum of the magnitudes of the two elements MUST be at most 32
//	Output Normalized: No
//	Output Max Magnitude: Sum of the magnitude of the two individual elements
func (e *Element) Add(val *Element) *Element {
	// Since the internal representation intentionally provides overflow bits,
	// it's ok to use carryless addition as the carry bit is safely part of each
	// limb and will be normalized out.  This could obviously be done in a loop,
	// but the unrolled version is faster.
	e.n[0] += val.n[0]
	e.n[1] += val.n[1]
	e.n[2] += val.n[2]
	e.n[3] += val.n[3]
	e.n[4] += val.n[4]
	e.n[5] += val.n[5]
	e.n[6] += val.n[6]
	e.n[7] += val.n[7]
	e.n[8] += val.n[8]
	e.n[9] += val.n[9]

	return e
}

// Add2 adds the passed two elements together and stores the result in e in
// constant time.
//
// The element is returned to support chaining.  This enables syntax like:
// e3.Add2(e, e2).AddInt(1) so that e3 = e + e2 + 1.
//
//	Preconditions:
//	  - The sum of the magnitudes of the two elements MUST be at most 32
//	Output Normalized: No
//	Output Max Magnitude: Sum of the magnitude of the two elements
func (e *Element) Add2(val *Element, val2 *Element) *Element {
	// Since the internal representation intentionally provides overflow bits,
	// it's ok to use carryless addition as the carry bit is safely part of each
	// limb and will be normalized out.  This could obviously be done in a loop,
	// but the unrolled version is faster.
	e.n[0] = val.n[0] + val2.n[0]
	e.n[1] = val.n[1] + val2.n[1]
	e.n[2] = val.n[2] + val2.n[2]
	e.n[3] = val.n[3] + val2.n[3]
	e.n[4] = val.n[4] + val2.n[4]
	e.n[5] = val.n[5] + val2.n[5]
	e.n[6] = val.n[6] + val2.n[6]
	e.n[7] = val.n[7] + val2.n[7]
	e.n[8] = val.n[8] + val2.n[8]
	e.n[9] = val.n[9] + val2.n[9]

	return e
}

// MulBy2 multiplies the element by 2 and stores the result in e in constant
// time.  Note that this function can overflow if multiplying the element causes
// any individual limb to overflow uint32.  Therefore it is important that the
// caller ensures no overflows will occur before using this function.
//
// The element is returned to support chaining.  This enables syntax like:
// e.MulBy2().Add(e2) so that e = 2 * e + e2.
//
//	Preconditions:
//	  - The element magnitude multiplied by 2 MUST be at most 32
//	Output Normalized: No
//	Output Max Magnitude: Existing element magnitude times 2
func (e *Element) MulBy2() *Element {
	return e.MulInt(2)
}

// MulBy3 multiplies the element by 3 and stores the result in e in constant
// time.  Note that this function can overflow if multiplying the element causes
// any individual limb to overflow uint32.  Therefore it is important that the
// caller ensures no overflows will occur before using this function.
//
// The element is returned to support chaining.  This enables syntax like:
// e.MulBy3().Add(e2) so that e = 3 * e + e2.
//
//	Preconditions:
//	  - The element magnitude multiplied by 3 MUST be at most 32
//	Output Normalized: No
//	Output Max Magnitude: Existing element magnitude times 3
func (e *Element) MulBy3() *Element {
	return e.MulInt(3)
}

// MulBy4 multiplies the element by 4 and stores the result in e in constant
// time.  Note that this function can overflow if multiplying the element causes
// any individual limb to overflow uint32.  Therefore it is important that the
// caller ensures no overflows will occur before using this function.
//
// The element is returned to support chaining.  This enables syntax like:
// e.MulBy4().Add(e2) so that e = 4 * e + e2.
//
//	Preconditions:
//	  - The element magnitude multiplied by 4 MUST be at most 32
//	Output Normalized: No
//	Output Max Magnitude: Existing element magnitude times 4
func (e *Element) MulBy4() *Element {
	return e.MulInt(4)
}

// MulBy8 multiplies the element by 8 and stores the result in e in constant
// time.  Note that this function can overflow if multiplying the element causes
// any individual limb to overflow uint32.  Therefore it is important that the
// caller ensures no overflows will occur before using this function.
//
// The element is returned to support chaining.  This enables syntax like:
// e.MulBy8().Add(e2) so that e = 8 * e + e2.
//
//	Preconditions:
//	  - The element magnitude multiplied by 8 MUST be at most 32
//	Output Normalized: No
//	Output Max Magnitude: Existing element times 8
func (e *Element) MulBy8() *Element {
	return e.MulInt(8)
}

// MulInt multiplies the element by the passed int and stores the result in e in
// constant time.  Note that this function can overflow if multiplying the
// element causes any individual limb to overflow uint32.  Therefore it is
// important that the caller ensures no overflows will occur before using this
// function.
//
// Callers should prefer using the specialized methods for multiplying by 2, 3,
// 4, and 8, as they are commonly used in curve equations.
//
// See [Element.MulBy2], [Element.MulBy3], [Element.MulBy4], and
// [Element.MulBy8] for the aforementioned specialized methods.
//
// The element is returned to support chaining.  This enables syntax like:
// e.MulInt(2).Add(e2) so that e = 2 * e + e2.
//
//	Preconditions:
//	  - The element magnitude multiplied by given val MUST be at most 32
//	Output Normalized: No
//	Output Max Magnitude: Existing element magnitude times the provided integer val
func (e *Element) MulInt(val uint8) *Element {
	// Since each limb of the internal representation can hold up to 32 -
	// [fieldBase] extra bits which will be normalized out, it's safe to
	// multiply each limb without using a larger type or carry propagation so
	// long as the values won't overflow a uint32.  This could obviously be done
	// in a loop, but the unrolled version is faster.
	ui := uint32(val)
	e.n[0] *= ui
	e.n[1] *= ui
	e.n[2] *= ui
	e.n[3] *= ui
	e.n[4] *= ui
	e.n[5] *= ui
	e.n[6] *= ui
	e.n[7] *= ui
	e.n[8] *= ui
	e.n[9] *= ui

	return e
}

// Mul multiplies the passed element to the existing element and stores the
// result in e in constant time.  Note that this function can overflow if
// multiplying causes any individual limb to overflow uint32.  In practice, this
// means the magnitude of either element involved in the multiplication must be
// at most 8.
//
// The element is returned to support chaining.  This enables syntax like:
// e.Mul(e2).AddInt(1) so that e = (e * e2) + 1.
//
//	Preconditions:
//	  - Both elements MUST have a max magnitude of 8
//	Output Normalized: No
//	Output Max Magnitude: 1
func (e *Element) Mul(val *Element) *Element {
	return e.Mul2(e, val)
}

// Mul2 multiplies the passed two elements together and stores the result in e
// in constant time.  Note that this function can overflow if multiplying any of
// the individual limbs exceeds a max uint32.  In practice, this means the
// magnitude of either element involved in the multiplication must be at most 8.
//
// The element is returned to support chaining.  This enables syntax like:
// e3.Mul2(e, e2).AddInt(1) so that e3 = (e * e2) + 1.
//
//	Preconditions:
//	  - Both input elements MUST have a max magnitude of 8
//	Output Normalized: No
//	Output Max Magnitude: 1
func (e *Element) Mul2(val *Element, val2 *Element) *Element {
	// This could be done with a couple of for loops and an array to store
	// the intermediate terms, but this unrolled version is significantly
	// faster.

	// Terms for 2^(fieldBase*0).
	m := uint64(val.n[0]) * uint64(val2.n[0])
	t0 := m & fieldBaseMask

	// Terms for 2^(fieldBase*1).
	m = (m >> fieldBase) +
		uint64(val.n[0])*uint64(val2.n[1]) +
		uint64(val.n[1])*uint64(val2.n[0])
	t1 := m & fieldBaseMask

	// Terms for 2^(fieldBase*2).
	m = (m >> fieldBase) +
		uint64(val.n[0])*uint64(val2.n[2]) +
		uint64(val.n[1])*uint64(val2.n[1]) +
		uint64(val.n[2])*uint64(val2.n[0])
	t2 := m & fieldBaseMask

	// Terms for 2^(fieldBase*3).
	m = (m >> fieldBase) +
		uint64(val.n[0])*uint64(val2.n[3]) +
		uint64(val.n[1])*uint64(val2.n[2]) +
		uint64(val.n[2])*uint64(val2.n[1]) +
		uint64(val.n[3])*uint64(val2.n[0])
	t3 := m & fieldBaseMask

	// Terms for 2^(fieldBase*4).
	m = (m >> fieldBase) +
		uint64(val.n[0])*uint64(val2.n[4]) +
		uint64(val.n[1])*uint64(val2.n[3]) +
		uint64(val.n[2])*uint64(val2.n[2]) +
		uint64(val.n[3])*uint64(val2.n[1]) +
		uint64(val.n[4])*uint64(val2.n[0])
	t4 := m & fieldBaseMask

	// Terms for 2^(fieldBase*5).
	m = (m >> fieldBase) +
		uint64(val.n[0])*uint64(val2.n[5]) +
		uint64(val.n[1])*uint64(val2.n[4]) +
		uint64(val.n[2])*uint64(val2.n[3]) +
		uint64(val.n[3])*uint64(val2.n[2]) +
		uint64(val.n[4])*uint64(val2.n[1]) +
		uint64(val.n[5])*uint64(val2.n[0])
	t5 := m & fieldBaseMask

	// Terms for 2^(fieldBase*6).
	m = (m >> fieldBase) +
		uint64(val.n[0])*uint64(val2.n[6]) +
		uint64(val.n[1])*uint64(val2.n[5]) +
		uint64(val.n[2])*uint64(val2.n[4]) +
		uint64(val.n[3])*uint64(val2.n[3]) +
		uint64(val.n[4])*uint64(val2.n[2]) +
		uint64(val.n[5])*uint64(val2.n[1]) +
		uint64(val.n[6])*uint64(val2.n[0])
	t6 := m & fieldBaseMask

	// Terms for 2^(fieldBase*7).
	m = (m >> fieldBase) +
		uint64(val.n[0])*uint64(val2.n[7]) +
		uint64(val.n[1])*uint64(val2.n[6]) +
		uint64(val.n[2])*uint64(val2.n[5]) +
		uint64(val.n[3])*uint64(val2.n[4]) +
		uint64(val.n[4])*uint64(val2.n[3]) +
		uint64(val.n[5])*uint64(val2.n[2]) +
		uint64(val.n[6])*uint64(val2.n[1]) +
		uint64(val.n[7])*uint64(val2.n[0])
	t7 := m & fieldBaseMask

	// Terms for 2^(fieldBase*8).
	m = (m >> fieldBase) +
		uint64(val.n[0])*uint64(val2.n[8]) +
		uint64(val.n[1])*uint64(val2.n[7]) +
		uint64(val.n[2])*uint64(val2.n[6]) +
		uint64(val.n[3])*uint64(val2.n[5]) +
		uint64(val.n[4])*uint64(val2.n[4]) +
		uint64(val.n[5])*uint64(val2.n[3]) +
		uint64(val.n[6])*uint64(val2.n[2]) +
		uint64(val.n[7])*uint64(val2.n[1]) +
		uint64(val.n[8])*uint64(val2.n[0])
	t8 := m & fieldBaseMask

	// Terms for 2^(fieldBase*9).
	m = (m >> fieldBase) +
		uint64(val.n[0])*uint64(val2.n[9]) +
		uint64(val.n[1])*uint64(val2.n[8]) +
		uint64(val.n[2])*uint64(val2.n[7]) +
		uint64(val.n[3])*uint64(val2.n[6]) +
		uint64(val.n[4])*uint64(val2.n[5]) +
		uint64(val.n[5])*uint64(val2.n[4]) +
		uint64(val.n[6])*uint64(val2.n[3]) +
		uint64(val.n[7])*uint64(val2.n[2]) +
		uint64(val.n[8])*uint64(val2.n[1]) +
		uint64(val.n[9])*uint64(val2.n[0])
	t9 := m & fieldBaseMask

	// Terms for 2^(fieldBase*10).
	m = (m >> fieldBase) +
		uint64(val.n[1])*uint64(val2.n[9]) +
		uint64(val.n[2])*uint64(val2.n[8]) +
		uint64(val.n[3])*uint64(val2.n[7]) +
		uint64(val.n[4])*uint64(val2.n[6]) +
		uint64(val.n[5])*uint64(val2.n[5]) +
		uint64(val.n[6])*uint64(val2.n[4]) +
		uint64(val.n[7])*uint64(val2.n[3]) +
		uint64(val.n[8])*uint64(val2.n[2]) +
		uint64(val.n[9])*uint64(val2.n[1])
	t10 := m & fieldBaseMask

	// Terms for 2^(fieldBase*11).
	m = (m >> fieldBase) +
		uint64(val.n[2])*uint64(val2.n[9]) +
		uint64(val.n[3])*uint64(val2.n[8]) +
		uint64(val.n[4])*uint64(val2.n[7]) +
		uint64(val.n[5])*uint64(val2.n[6]) +
		uint64(val.n[6])*uint64(val2.n[5]) +
		uint64(val.n[7])*uint64(val2.n[4]) +
		uint64(val.n[8])*uint64(val2.n[3]) +
		uint64(val.n[9])*uint64(val2.n[2])
	t11 := m & fieldBaseMask

	// Terms for 2^(fieldBase*12).
	m = (m >> fieldBase) +
		uint64(val.n[3])*uint64(val2.n[9]) +
		uint64(val.n[4])*uint64(val2.n[8]) +
		uint64(val.n[5])*uint64(val2.n[7]) +
		uint64(val.n[6])*uint64(val2.n[6]) +
		uint64(val.n[7])*uint64(val2.n[5]) +
		uint64(val.n[8])*uint64(val2.n[4]) +
		uint64(val.n[9])*uint64(val2.n[3])
	t12 := m & fieldBaseMask

	// Terms for 2^(fieldBase*13).
	m = (m >> fieldBase) +
		uint64(val.n[4])*uint64(val2.n[9]) +
		uint64(val.n[5])*uint64(val2.n[8]) +
		uint64(val.n[6])*uint64(val2.n[7]) +
		uint64(val.n[7])*uint64(val2.n[6]) +
		uint64(val.n[8])*uint64(val2.n[5]) +
		uint64(val.n[9])*uint64(val2.n[4])
	t13 := m & fieldBaseMask

	// Terms for 2^(fieldBase*14).
	m = (m >> fieldBase) +
		uint64(val.n[5])*uint64(val2.n[9]) +
		uint64(val.n[6])*uint64(val2.n[8]) +
		uint64(val.n[7])*uint64(val2.n[7]) +
		uint64(val.n[8])*uint64(val2.n[6]) +
		uint64(val.n[9])*uint64(val2.n[5])
	t14 := m & fieldBaseMask

	// Terms for 2^(fieldBase*15).
	m = (m >> fieldBase) +
		uint64(val.n[6])*uint64(val2.n[9]) +
		uint64(val.n[7])*uint64(val2.n[8]) +
		uint64(val.n[8])*uint64(val2.n[7]) +
		uint64(val.n[9])*uint64(val2.n[6])
	t15 := m & fieldBaseMask

	// Terms for 2^(fieldBase*16).
	m = (m >> fieldBase) +
		uint64(val.n[7])*uint64(val2.n[9]) +
		uint64(val.n[8])*uint64(val2.n[8]) +
		uint64(val.n[9])*uint64(val2.n[7])
	t16 := m & fieldBaseMask

	// Terms for 2^(fieldBase*17).
	m = (m >> fieldBase) +
		uint64(val.n[8])*uint64(val2.n[9]) +
		uint64(val.n[9])*uint64(val2.n[8])
	t17 := m & fieldBaseMask

	// Terms for 2^(fieldBase*18).
	m = (m >> fieldBase) + uint64(val.n[9])*uint64(val2.n[9])
	t18 := m & fieldBaseMask

	// What's left is for 2^(fieldBase*19).
	t19 := m >> fieldBase

	// At this point, all of the terms are grouped into their respective
	// base.
	//
	// Per [HAC] section 14.3.4: Reduction method of moduli of special form,
	// when the modulus is of the special form m = b^t - c, highly efficient
	// reduction can be achieved per the provided algorithm.
	//
	// The secp256k1 prime is equivalent to 2^256 - 4294968273, so it fits
	// this criteria.
	//
	// 4294968273 in the internal representation (base 2^26) is:
	// n[0] = 977
	// n[1] = 64
	// That is to say (2^26 * 64) + 977 = 4294968273
	//
	// Since each limb is in base 26, the upper terms (t10 and up) start at 260
	// bits (versus the final desired range of 256 bits), so the internal
	// representation of 'c' from above needs to be adjusted for the extra 4
	// bits by multiplying it by 2^4 = 16.  4294968273 * 16 = 68719492368.
	// Thus, the adjusted internal representation of 'c' is:
	// n[0] = 977 * 16 = 15632
	// n[1] = 64 * 16 = 1024
	// That is to say (2^26 * 1024) + 15632 = 68719492368
	//
	// To reduce the final term, t19, the entire 'c' value is needed instead of
	// only n[0] because there are no more terms left to handle n[1].  This
	// means there might be some magnitude left in the upper bits that is
	// handled below.
	m = t0 + t10*15632
	t0 = m & fieldBaseMask
	m = (m >> fieldBase) + t1 + t10*1024 + t11*15632
	t1 = m & fieldBaseMask
	m = (m >> fieldBase) + t2 + t11*1024 + t12*15632
	t2 = m & fieldBaseMask
	m = (m >> fieldBase) + t3 + t12*1024 + t13*15632
	t3 = m & fieldBaseMask
	m = (m >> fieldBase) + t4 + t13*1024 + t14*15632
	t4 = m & fieldBaseMask
	m = (m >> fieldBase) + t5 + t14*1024 + t15*15632
	t5 = m & fieldBaseMask
	m = (m >> fieldBase) + t6 + t15*1024 + t16*15632
	t6 = m & fieldBaseMask
	m = (m >> fieldBase) + t7 + t16*1024 + t17*15632
	t7 = m & fieldBaseMask
	m = (m >> fieldBase) + t8 + t17*1024 + t18*15632
	t8 = m & fieldBaseMask
	m = (m >> fieldBase) + t9 + t18*1024 + t19*68719492368
	t9 = m & fieldMSBMask
	m >>= fieldMSBBits

	// At this point, if the magnitude is greater than 0, the overall element
	// is greater than the max possible 256-bit value.  In particular, it is
	// "how many times larger" than the max value it is.
	//
	// The algorithm presented in [HAC] section 14.3.4 repeats until the
	// quotient is zero.  However, due to the above, we already know at
	// least how many times we would need to repeat as it's the value
	// currently in m.  Thus we can simply multiply the magnitude by the
	// internal representation of the prime and do a single iteration.  Notice
	// that nothing will be changed when the magnitude is zero, so we could
	// skip this in that case, however always running regardless allows it
	// to run in constant time.  The final result will be in the range
	// 0 <= result <= prime + (2^64 - c), so it is guaranteed to have a
	// magnitude of 1, but it is denormalized.
	d := t0 + m*977
	e.n[0] = uint32(d & fieldBaseMask)
	d = (d >> fieldBase) + t1 + m*64
	e.n[1] = uint32(d & fieldBaseMask)
	e.n[2] = uint32((d >> fieldBase) + t2)
	e.n[3] = uint32(t3)
	e.n[4] = uint32(t4)
	e.n[5] = uint32(t5)
	e.n[6] = uint32(t6)
	e.n[7] = uint32(t7)
	e.n[8] = uint32(t8)
	e.n[9] = uint32(t9)

	return e
}

// SquareRootVal either calculates the square root of the passed element when it
// exists or the square root of the negation of the element when it does not
// exist and stores the result in e in constant time.  The return flag is true
// when the calculated square root is for the passed element itself and false
// when it is for its negation.
//
// Note that this function can overflow if multiplying any of the individual
// limbs exceeds a max uint32.  In practice, this means the magnitude of the
// element must be at most 8 to prevent overflow.  The magnitude of the result
// will be 1.
//
//	Preconditions:
//	  - The input element MUST have a max magnitude of 8
//	Output Normalized: No
//	Output Max Magnitude: 1
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
	e.Square().Square()                                    // e = a^(2^254 - 2^30 - 2^8 - 2^3 - 2^2)
	//                                                     //   = a^(2^254 - 2^30 - 244)
	//                                                     //   = a^((p+1)/4)

	// Ensure the calculated result is actually the square root by squaring it
	// and checking against the original element.
	var sqr Element
	return sqr.SquareVal(e).Normalize().Equals(val.Normalize())
}

// Square squares the element in constant time.  The existing element is
// modified.  Note that this function can overflow if multiplying any of the
// individual limbs exceeds a max uint32.  In practice, this means the magnitude
// of the element must be at most 8 to prevent overflow.
//
// The element is returned to support chaining.  This enables syntax like:
// e.Square().Mul(e2) so that e = e^2 * e2.
//
//	Preconditions:
//	  - The element MUST have a max magnitude of 8
//	Output Normalized: No
//	Output Max Magnitude: 1
func (e *Element) Square() *Element {
	return e.SquareVal(e)
}

// SquareVal squares the passed element and stores the result in e in constant
// time.  Note that this function can overflow if multiplying any of the
// individual limbs exceeds a max uint32.  In practice, this means the magnitude
// of the element being squared must be at most 8 to prevent overflow.
//
// The element is returned to support chaining.  This enables syntax like:
// e3.SquareVal(e).Mul(e) so that e3 = e^2 * e = e^3.
//
//	Preconditions:
//	  - The input element MUST have a max magnitude of 8
//	Output Normalized: No
//	Output Max Magnitude: 1
func (e *Element) SquareVal(val *Element) *Element {
	// This could be done with a couple of for loops and an array to store
	// the intermediate terms, but this unrolled version is significantly
	// faster.

	// Terms for 2^(fieldBase*0).
	m := uint64(val.n[0]) * uint64(val.n[0])
	t0 := m & fieldBaseMask

	// Terms for 2^(fieldBase*1).
	m = (m >> fieldBase) + 2*uint64(val.n[0])*uint64(val.n[1])
	t1 := m & fieldBaseMask

	// Terms for 2^(fieldBase*2).
	m = (m >> fieldBase) +
		2*uint64(val.n[0])*uint64(val.n[2]) +
		uint64(val.n[1])*uint64(val.n[1])
	t2 := m & fieldBaseMask

	// Terms for 2^(fieldBase*3).
	m = (m >> fieldBase) +
		2*uint64(val.n[0])*uint64(val.n[3]) +
		2*uint64(val.n[1])*uint64(val.n[2])
	t3 := m & fieldBaseMask

	// Terms for 2^(fieldBase*4).
	m = (m >> fieldBase) +
		2*uint64(val.n[0])*uint64(val.n[4]) +
		2*uint64(val.n[1])*uint64(val.n[3]) +
		uint64(val.n[2])*uint64(val.n[2])
	t4 := m & fieldBaseMask

	// Terms for 2^(fieldBase*5).
	m = (m >> fieldBase) +
		2*uint64(val.n[0])*uint64(val.n[5]) +
		2*uint64(val.n[1])*uint64(val.n[4]) +
		2*uint64(val.n[2])*uint64(val.n[3])
	t5 := m & fieldBaseMask

	// Terms for 2^(fieldBase*6).
	m = (m >> fieldBase) +
		2*uint64(val.n[0])*uint64(val.n[6]) +
		2*uint64(val.n[1])*uint64(val.n[5]) +
		2*uint64(val.n[2])*uint64(val.n[4]) +
		uint64(val.n[3])*uint64(val.n[3])
	t6 := m & fieldBaseMask

	// Terms for 2^(fieldBase*7).
	m = (m >> fieldBase) +
		2*uint64(val.n[0])*uint64(val.n[7]) +
		2*uint64(val.n[1])*uint64(val.n[6]) +
		2*uint64(val.n[2])*uint64(val.n[5]) +
		2*uint64(val.n[3])*uint64(val.n[4])
	t7 := m & fieldBaseMask

	// Terms for 2^(fieldBase*8).
	m = (m >> fieldBase) +
		2*uint64(val.n[0])*uint64(val.n[8]) +
		2*uint64(val.n[1])*uint64(val.n[7]) +
		2*uint64(val.n[2])*uint64(val.n[6]) +
		2*uint64(val.n[3])*uint64(val.n[5]) +
		uint64(val.n[4])*uint64(val.n[4])
	t8 := m & fieldBaseMask

	// Terms for 2^(fieldBase*9).
	m = (m >> fieldBase) +
		2*uint64(val.n[0])*uint64(val.n[9]) +
		2*uint64(val.n[1])*uint64(val.n[8]) +
		2*uint64(val.n[2])*uint64(val.n[7]) +
		2*uint64(val.n[3])*uint64(val.n[6]) +
		2*uint64(val.n[4])*uint64(val.n[5])
	t9 := m & fieldBaseMask

	// Terms for 2^(fieldBase*10).
	m = (m >> fieldBase) +
		2*uint64(val.n[1])*uint64(val.n[9]) +
		2*uint64(val.n[2])*uint64(val.n[8]) +
		2*uint64(val.n[3])*uint64(val.n[7]) +
		2*uint64(val.n[4])*uint64(val.n[6]) +
		uint64(val.n[5])*uint64(val.n[5])
	t10 := m & fieldBaseMask

	// Terms for 2^(fieldBase*11).
	m = (m >> fieldBase) +
		2*uint64(val.n[2])*uint64(val.n[9]) +
		2*uint64(val.n[3])*uint64(val.n[8]) +
		2*uint64(val.n[4])*uint64(val.n[7]) +
		2*uint64(val.n[5])*uint64(val.n[6])
	t11 := m & fieldBaseMask

	// Terms for 2^(fieldBase*12).
	m = (m >> fieldBase) +
		2*uint64(val.n[3])*uint64(val.n[9]) +
		2*uint64(val.n[4])*uint64(val.n[8]) +
		2*uint64(val.n[5])*uint64(val.n[7]) +
		uint64(val.n[6])*uint64(val.n[6])
	t12 := m & fieldBaseMask

	// Terms for 2^(fieldBase*13).
	m = (m >> fieldBase) +
		2*uint64(val.n[4])*uint64(val.n[9]) +
		2*uint64(val.n[5])*uint64(val.n[8]) +
		2*uint64(val.n[6])*uint64(val.n[7])
	t13 := m & fieldBaseMask

	// Terms for 2^(fieldBase*14).
	m = (m >> fieldBase) +
		2*uint64(val.n[5])*uint64(val.n[9]) +
		2*uint64(val.n[6])*uint64(val.n[8]) +
		uint64(val.n[7])*uint64(val.n[7])
	t14 := m & fieldBaseMask

	// Terms for 2^(fieldBase*15).
	m = (m >> fieldBase) +
		2*uint64(val.n[6])*uint64(val.n[9]) +
		2*uint64(val.n[7])*uint64(val.n[8])
	t15 := m & fieldBaseMask

	// Terms for 2^(fieldBase*16).
	m = (m >> fieldBase) +
		2*uint64(val.n[7])*uint64(val.n[9]) +
		uint64(val.n[8])*uint64(val.n[8])
	t16 := m & fieldBaseMask

	// Terms for 2^(fieldBase*17).
	m = (m >> fieldBase) + 2*uint64(val.n[8])*uint64(val.n[9])
	t17 := m & fieldBaseMask

	// Terms for 2^(fieldBase*18).
	m = (m >> fieldBase) + uint64(val.n[9])*uint64(val.n[9])
	t18 := m & fieldBaseMask

	// What's left is for 2^(fieldBase*19).
	t19 := m >> fieldBase

	// At this point, all of the terms are grouped into their respective
	// base.
	//
	// Per [HAC] section 14.3.4: Reduction method of moduli of special form,
	// when the modulus is of the special form m = b^t - c, highly efficient
	// reduction can be achieved per the provided algorithm.
	//
	// The secp256k1 prime is equivalent to 2^256 - 4294968273, so it fits
	// this criteria.
	//
	// 4294968273 in the internal representation (base 2^26) is:
	// n[0] = 977
	// n[1] = 64
	// That is to say (2^26 * 64) + 977 = 4294968273
	//
	// Since each limb is in base 26, the upper terms (t10 and up) start at 260
	// bits (versus the final desired range of 256 bits), so the internal
	// representation of 'c' from above needs to be adjusted for the extra 4
	// bits by multiplying it by 2^4 = 16.  4294968273 * 16 = 68719492368.
	// Thus, the adjusted internal representation of 'c' is:
	// n[0] = 977 * 16 = 15632
	// n[1] = 64 * 16 = 1024
	// That is to say (2^26 * 1024) + 15632 = 68719492368
	//
	// To reduce the final term, t19, the entire 'c' value is needed instead
	// of only n[0] because there are no more terms left to handle n[1].
	// This means there might be some magnitude left in the upper bits that
	// is handled below.
	m = t0 + t10*15632
	t0 = m & fieldBaseMask
	m = (m >> fieldBase) + t1 + t10*1024 + t11*15632
	t1 = m & fieldBaseMask
	m = (m >> fieldBase) + t2 + t11*1024 + t12*15632
	t2 = m & fieldBaseMask
	m = (m >> fieldBase) + t3 + t12*1024 + t13*15632
	t3 = m & fieldBaseMask
	m = (m >> fieldBase) + t4 + t13*1024 + t14*15632
	t4 = m & fieldBaseMask
	m = (m >> fieldBase) + t5 + t14*1024 + t15*15632
	t5 = m & fieldBaseMask
	m = (m >> fieldBase) + t6 + t15*1024 + t16*15632
	t6 = m & fieldBaseMask
	m = (m >> fieldBase) + t7 + t16*1024 + t17*15632
	t7 = m & fieldBaseMask
	m = (m >> fieldBase) + t8 + t17*1024 + t18*15632
	t8 = m & fieldBaseMask
	m = (m >> fieldBase) + t9 + t18*1024 + t19*68719492368
	t9 = m & fieldMSBMask
	m >>= fieldMSBBits

	// At this point, if the magnitude is greater than 0, the overall element
	// is greater than the max possible 256-bit value.  In particular, it is
	// "how many times larger" than the max value it is.
	//
	// The algorithm presented in [HAC] section 14.3.4 repeats until the
	// quotient is zero.  However, due to the above, we already know at
	// least how many times we would need to repeat as it's the quantity
	// currently in m.  Thus we can simply multiply the magnitude by the
	// internal representation of the prime and do a single iteration.  Notice
	// that nothing will be changed when the magnitude is zero, so we could
	// skip this in that case, however always running regardless allows it
	// to run in constant time.  The final result will be in the range
	// 0 <= result <= prime + (2^64 - c), so it is guaranteed to have a
	// magnitude of 1, but it is denormalized.
	n := t0 + m*977
	e.n[0] = uint32(n & fieldBaseMask)
	n = (n >> fieldBase) + t1 + m*64
	e.n[1] = uint32(n & fieldBaseMask)
	e.n[2] = uint32((n >> fieldBase) + t2)
	e.n[3] = uint32(t3)
	e.n[4] = uint32(t4)
	e.n[5] = uint32(t5)
	e.n[6] = uint32(t6)
	e.n[7] = uint32(t7)
	e.n[8] = uint32(t8)
	e.n[9] = uint32(t9)

	return e
}

// Inverse finds the modular multiplicative inverse of the element in constant
// time.  The existing element is modified.
//
// The element is returned to support chaining.  This enables syntax like:
// e.Inverse().Mul(e2) so that e = e^-1 * e2.
//
//	Preconditions:
//	  - The element MUST have a max magnitude of 8
//	Output Normalized: No
//	Output Max Magnitude: 1
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
//
//	Preconditions:
//	  - The element MUST be normalized
func (e *Element) IsGtOrEqPrimeMinusOrder() bool {
	// The secp256k1 prime is equivalent to 2^256 - 4294968273 and the group
	// order is 2^256 - 432420386565659656852420866394968145599.  Thus,
	// the prime minus the group order is:
	// 432420386565659656852420866390673177326
	//
	// In hex that is:
	// 0x00000000 00000000 00000000 00000001 45512319 50b75fc4 402da172 2fc9baee
	//
	// Converting that to the internal representation (base 2^26) is:
	//
	// n[0] = 0x03c9baee
	// n[1] = 0x03685c8b
	// n[2] = 0x01fc4402
	// n[3] = 0x006542dd
	// n[4] = 0x01455123
	//
	// This can be verified with the following test code:
	//   pMinusN := new(big.Int).Sub(curveParams.P, curveParams.N)
	//   var v Element
	//   v.SetByteSlice(pMinusN.Bytes())
	//   t.Logf("%x", v.n)
	//
	//   Outputs: [3c9baee 3685c8b 1fc4402 6542dd 1455123 0 0 0 0 0]
	const (
		pMinusNLimb0 = 0x03c9baee
		pMinusNLimb1 = 0x03685c8b
		pMinusNLimb2 = 0x01fc4402
		pMinusNLimb3 = 0x006542dd
		pMinusNLimb4 = 0x01455123
		pMinusNLimb5 = 0x00000000
		pMinusNLimb6 = 0x00000000
		pMinusNLimb7 = 0x00000000
		pMinusNLimb8 = 0x00000000
		pMinusNLimb9 = 0x00000000
	)

	// The intuition here is that the element is greater than field prime minus
	// the group order if one of the higher individual limbs is greater than the
	// corresponding limb and all higher limbs in the element are equal.
	result := arith.ConstantTimeGreater(e.n[9], pMinusNLimb9)
	highLimbsEqual := arith.ConstantTimeEq(e.n[9], pMinusNLimb9)
	result |= highLimbsEqual & arith.ConstantTimeGreater(e.n[8], pMinusNLimb8)
	highLimbsEqual &= arith.ConstantTimeEq(e.n[8], pMinusNLimb8)
	result |= highLimbsEqual & arith.ConstantTimeGreater(e.n[7], pMinusNLimb7)
	highLimbsEqual &= arith.ConstantTimeEq(e.n[7], pMinusNLimb7)
	result |= highLimbsEqual & arith.ConstantTimeGreater(e.n[6], pMinusNLimb6)
	highLimbsEqual &= arith.ConstantTimeEq(e.n[6], pMinusNLimb6)
	result |= highLimbsEqual & arith.ConstantTimeGreater(e.n[5], pMinusNLimb5)
	highLimbsEqual &= arith.ConstantTimeEq(e.n[5], pMinusNLimb5)
	result |= highLimbsEqual & arith.ConstantTimeGreater(e.n[4], pMinusNLimb4)
	highLimbsEqual &= arith.ConstantTimeEq(e.n[4], pMinusNLimb4)
	result |= highLimbsEqual & arith.ConstantTimeGreater(e.n[3], pMinusNLimb3)
	highLimbsEqual &= arith.ConstantTimeEq(e.n[3], pMinusNLimb3)
	result |= highLimbsEqual & arith.ConstantTimeGreater(e.n[2], pMinusNLimb2)
	highLimbsEqual &= arith.ConstantTimeEq(e.n[2], pMinusNLimb2)
	result |= highLimbsEqual & arith.ConstantTimeGreater(e.n[1], pMinusNLimb1)
	highLimbsEqual &= arith.ConstantTimeEq(e.n[1], pMinusNLimb1)
	result |= highLimbsEqual & arith.ConstantTimeGreaterOrEq(e.n[0], pMinusNLimb0)

	return result != 0
}
