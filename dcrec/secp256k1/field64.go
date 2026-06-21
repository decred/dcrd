// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/bits"
)

// FieldVal64 is a secp256k1 field element stored as four little-endian uint64
// limbs with Crandall reduction for p = 2^256 - 0x1000003D1.
//
// Unlike FieldVal (10 x uint32 base 2^26), this uses tight 256-bit packing and
// fully reduces after each operation.
type FieldVal64 struct {
	n [4]uint64
}

const (
	field64PrimeComplement = 0x1000003D1 // 2^32 + 977

	field64Prime0 = 0xFFFFFFFEFFFFFC2F
	field64Prime1 = 0xFFFFFFFFFFFFFFFF
	field64Prime2 = 0xFFFFFFFFFFFFFFFF
	field64Prime3 = 0xFFFFFFFFFFFFFFFF
)

// SetBytes sets f to the 32-byte big-endian value and returns 1 if the input is
// >= p.
func (f *FieldVal64) SetBytes(b *[32]byte) uint32 {
	f.n[0] = binary.BigEndian.Uint64(b[24:32])
	f.n[1] = binary.BigEndian.Uint64(b[16:24])
	f.n[2] = binary.BigEndian.Uint64(b[8:16])
	f.n[3] = binary.BigEndian.Uint64(b[0:8])

	// Subtract p once. The input overflowed (>= p) when f - p does not borrow,
	// in which case the reduced result s replaces f via constant-time select.
	var s0, s1, s2, s3, borrow uint64
	s0, borrow = bits.Sub64(f.n[0], field64Prime0, 0)
	s1, borrow = bits.Sub64(f.n[1], field64Prime1, borrow)
	s2, borrow = bits.Sub64(f.n[2], field64Prime2, borrow)
	s3, borrow = bits.Sub64(f.n[3], field64Prime3, borrow)

	mask := -(1 - borrow)
	f.n[0] ^= (s0 ^ f.n[0]) & mask
	f.n[1] ^= (s1 ^ f.n[1]) & mask
	f.n[2] ^= (s2 ^ f.n[2]) & mask
	f.n[3] ^= (s3 ^ f.n[3]) & mask
	return uint32(1 - borrow)
}

// Bytes returns the 32-byte big-endian encoding.
func (f *FieldVal64) Bytes() *[32]byte {
	var b [32]byte
	f.PutBytes(&b)
	return &b
}

// PutBytes writes the value to b in big-endian order.
func (f *FieldVal64) PutBytes(b *[32]byte) {
	binary.BigEndian.PutUint64(b[0:8], f.n[3])
	binary.BigEndian.PutUint64(b[8:16], f.n[2])
	binary.BigEndian.PutUint64(b[16:24], f.n[1])
	binary.BigEndian.PutUint64(b[24:32], f.n[0])
}

// Normalize fully reduces f modulo p. FieldVal64 values are always kept fully
// reduced, so this is a no-op kept for API parity with FieldVal.
func (f *FieldVal64) Normalize() *FieldVal64 {
	return f
}

// SetInt sets f to a small integer.
func (f *FieldVal64) SetInt(v uint16) *FieldVal64 {
	f.n = [4]uint64{uint64(v), 0, 0, 0}
	return f
}

// Set sets f to val.
func (f *FieldVal64) Set(val *FieldVal64) *FieldVal64 {
	f.n = val.n
	return f
}

// Equals reports whether two values are equal.
func (f *FieldVal64) Equals(val *FieldVal64) bool {
	bits := (f.n[0] ^ val.n[0]) | (f.n[1] ^ val.n[1]) |
		(f.n[2] ^ val.n[2]) | (f.n[3] ^ val.n[3])
	return bits == 0
}

// IsOdd reports whether f is odd.
func (f *FieldVal64) IsOdd() bool {
	return f.n[0]&1 == 1
}

// Add2 sets f = a + b (mod p).
func (f *FieldVal64) Add2(a, b *FieldVal64) *FieldVal64 {
	var t0, t1, t2, t3, overflow, carry uint64

	// Pass 1: add.
	t0, carry = bits.Add64(a.n[0], b.n[0], 0)
	t1, carry = bits.Add64(a.n[1], b.n[1], carry)
	t2, carry = bits.Add64(a.n[2], b.n[2], carry)
	t3, overflow = bits.Add64(a.n[3], b.n[3], carry)

	// Pass 2: subtract p. Since p = 2^256 - C, the low 256 bits of t - p are
	// identical to t + C, i.e. the folded result for a 2^256 overflow.
	var s0, s1, s2, s3, borrow uint64
	s0, borrow = bits.Sub64(t0, field64Prime0, 0)
	s1, borrow = bits.Sub64(t1, field64Prime1, borrow)
	s2, borrow = bits.Sub64(t2, field64Prime2, borrow)
	s3, borrow = bits.Sub64(t3, field64Prime3, borrow)

	// Pass 3: constant-time select. Keep t only when there was no overflow and
	// t < p (borrow set); otherwise use s (= t - p when t >= p, = t + C folded
	// when overflow occurred).
	mask := -((1 - overflow) & borrow)
	f.n[0] = s0 ^ ((t0 ^ s0) & mask)
	f.n[1] = s1 ^ ((t1 ^ s1) & mask)
	f.n[2] = s2 ^ ((t2 ^ s2) & mask)
	f.n[3] = s3 ^ ((t3 ^ s3) & mask)
	return f
}

// Add sets f = f + val (mod p).
func (f *FieldVal64) Add(val *FieldVal64) *FieldVal64 {
	return f.Add2(f, val)
}

// MulInt sets f = f * val (mod p). val is limited to the small constants used by
// the curve formulas (up to 8); larger values panic. FieldVal magnitude tracking
// is irrelevant since FieldVal64 stays fully reduced. Doublings (f.Add(f)) keep
// the addition count low.
func (f *FieldVal64) MulInt(val uint8) *FieldVal64 {
	if val > 8 {
		panic(fmt.Sprintf("FieldVal64.MulInt: val %d exceeds supported maximum of 8", val))
	}

	switch val {
	case 0:
		f.n = [4]uint64{}
		return f
	case 1:
		return f
	}

	var orig FieldVal64
	orig.Set(f)
	switch val {
	case 2:
		f.Add(&orig) // 2
	case 3:
		f.Add(&orig).Add(&orig) // 3
	case 4:
		f.Add(&orig) // 2
		f.Add(f)     // 4
	case 5:
		f.Add(&orig) // 2
		f.Add(f)     // 4
		f.Add(&orig) // 5
	case 6:
		f.Add(&orig).Add(&orig) // 3
		f.Add(f)                // 6
	case 7:
		f.Add(&orig).Add(&orig) // 3
		f.Add(f)                // 6
		f.Add(&orig)            // 7
	case 8:
		f.Add(&orig) // 2
		f.Add(f)     // 4
		f.Add(f)     // 8
	}
	return f
}

// NegateVal sets f = -val (mod p). The magnitude parameter exists for API
// parity with FieldVal and is ignored since FieldVal64 stays fully reduced.
func (f *FieldVal64) NegateVal(val *FieldVal64, magnitude uint32) *FieldVal64 {
	// Pass 1: subtract val from 0. borrow is set iff val != 0.
	var t0, t1, t2, t3, borrow uint64
	t0, borrow = bits.Sub64(0, val.n[0], 0)
	t1, borrow = bits.Sub64(0, val.n[1], borrow)
	t2, borrow = bits.Sub64(0, val.n[2], borrow)
	t3, borrow = bits.Sub64(0, val.n[3], borrow)

	// Pass 2: mask the modulus with the borrow (p when val != 0, else 0).
	mask := -borrow
	maskedPrime0 := field64Prime0 & mask

	// Pass 3: add the masked modulus so (0 - val) + p = p - val (mod 2^256),
	// while val == 0 stays 0.
	var carry uint64
	f.n[0], carry = bits.Add64(t0, maskedPrime0, 0)
	f.n[1], carry = bits.Add64(t1, mask, carry)
	f.n[2], carry = bits.Add64(t2, mask, carry)
	f.n[3], _ = bits.Add64(t3, mask, carry)
	return f
}

// Mul2 sets f = a * b (mod p). The 256x256 -> 256 modular multiply is provided
// by field64Mul, which uses native/hardware-optimized assembly on the supported
// platforms and falls back to a portable Go implementation elsewhere.
func (f *FieldVal64) Mul2(a, b *FieldVal64) *FieldVal64 {
	field64Mul(&f.n, &a.n, &b.n)
	return f
}

// Mul sets f = f * val (mod p).
func (f *FieldVal64) Mul(val *FieldVal64) *FieldVal64 {
	return f.Mul2(f, val)
}

// SquareVal sets f = val^2 (mod p). The modular square is provided by
// field64Square, which uses native/hardware-optimized assembly on the supported
// platforms and falls back to a portable Go implementation elsewhere.
func (f *FieldVal64) SquareVal(val *FieldVal64) *FieldVal64 {
	field64Square(&f.n, &val.n)
	return f
}

// Square sets f = f^2 (mod p).
func (f *FieldVal64) Square() *FieldVal64 {
	return f.SquareVal(f)
}

// p - n (field prime minus the group order) as little-endian 64-bit limbs.
const (
	field64PMinusN0 = 0x402da1722fc9baee
	field64PMinusN1 = 0x4551231950b75fc4
	field64PMinusN2 = 0x0000000000000001
	field64PMinusN3 = 0x0000000000000000
)

// Negate sets f = -f (mod p). The magnitude parameter exists for API parity with
// FieldVal and is ignored since FieldVal64 stays fully reduced.
func (f *FieldVal64) Negate(magnitude uint32) *FieldVal64 {
	return f.NegateVal(f, magnitude)
}

// AddInt sets f = f + ui (mod p).
func (f *FieldVal64) AddInt(ui uint16) *FieldVal64 {
	var t FieldVal64
	t.SetInt(ui)
	return f.Add(&t)
}

// Zero sets f to zero.
func (f *FieldVal64) Zero() {
	f.n = [4]uint64{}
}

// IsZero reports whether f is zero.
func (f *FieldVal64) IsZero() bool {
	return (f.n[0] | f.n[1] | f.n[2] | f.n[3]) == 0
}

// IsZeroBit returns 1 if f is zero and 0 otherwise in constant time. See IsZero
// for the bool version.
func (f *FieldVal64) IsZeroBit() uint32 {
	// (bits | -bits) >> 63 is 1 iff bits != 0; invert for is-zero.
	bits := f.n[0] | f.n[1] | f.n[2] | f.n[3]
	return uint32(((bits | -bits) >> 63) ^ 1)
}

// IsOne reports whether f is one.
func (f *FieldVal64) IsOne() bool {
	return f.n[0] == 1 && (f.n[1]|f.n[2]|f.n[3]) == 0
}

// IsOneBit returns 1 if f is one and 0 otherwise in constant time. See IsOne for
// the bool version.
func (f *FieldVal64) IsOneBit() uint32 {
	// (bits | -bits) >> 63 is 1 iff bits != 0; invert for is-one.
	bits := (f.n[0] ^ 1) | f.n[1] | f.n[2] | f.n[3]
	return uint32(((bits | -bits) >> 63) ^ 1)
}

// String returns the field value as a human-readable hex string.
func (f FieldVal64) String() string {
	return hex.EncodeToString(f.Bytes()[:])
}

// IsOddBit returns 1 if f is odd and 0 otherwise.
func (f *FieldVal64) IsOddBit() uint32 {
	return uint32(f.n[0] & 1)
}

// SetByteSlice interprets b as a big-endian unsigned integer (truncated to the
// first 32 bytes), stores it modulo p, and returns whether the input overflowed
// p before reduction.
func (f *FieldVal64) SetByteSlice(b []byte) bool {
	var b32 [32]byte
	if len(b) > 32 {
		b = b[:32]
	}
	copy(b32[32-len(b):], b)
	return f.SetBytes(&b32) != 0
}

// PutBytesUnchecked writes the 32-byte big-endian encoding of f into b without
// bounds checking.
func (f *FieldVal64) PutBytesUnchecked(b []byte) {
	binary.BigEndian.PutUint64(b[0:8], f.n[3])
	binary.BigEndian.PutUint64(b[8:16], f.n[2])
	binary.BigEndian.PutUint64(b[16:24], f.n[1])
	binary.BigEndian.PutUint64(b[24:32], f.n[0])
}

// IsGtOrEqPrimeMinusOrder reports whether f >= p - n in constant time. f must be
// fully reduced, which FieldVal64 always is.
func (f *FieldVal64) IsGtOrEqPrimeMinusOrder() bool {
	var borrow uint64
	_, borrow = bits.Sub64(f.n[0], field64PMinusN0, 0)
	_, borrow = bits.Sub64(f.n[1], field64PMinusN1, borrow)
	_, borrow = bits.Sub64(f.n[2], field64PMinusN2, borrow)
	_, borrow = bits.Sub64(f.n[3], field64PMinusN3, borrow)
	return borrow == 0
}

// Inverse sets f = f^(-1) (mod p) via Fermat's little theorem (a^(p-2)).
func (f *FieldVal64) Inverse() *FieldVal64 {
	var a, a2, a3, a6, a9, a11, a22, a44, a88, a176, a220, a223 FieldVal64
	a.Set(f)
	a2.SquareVal(&a).Mul(&a)
	a3.SquareVal(&a2).Mul(&a)
	a6.SquareVal(&a3).Square().Square()
	a6.Mul(&a3)
	a9.SquareVal(&a6).Square().Square()
	a9.Mul(&a3)
	a11.SquareVal(&a9).Square()
	a11.Mul(&a2)
	a22.SquareVal(&a11).Square().Square().Square().Square()
	a22.Square().Square().Square().Square().Square()
	a22.Square()
	a22.Mul(&a11)
	a44.SquareVal(&a22).Square().Square().Square().Square()
	a44.Square().Square().Square().Square().Square()
	a44.Square().Square().Square().Square().Square()
	a44.Square().Square().Square().Square().Square()
	a44.Square().Square()
	a44.Mul(&a22)
	a88.SquareVal(&a44).Square().Square().Square().Square()
	a88.Square().Square().Square().Square().Square()
	a88.Square().Square().Square().Square().Square()
	a88.Square().Square().Square().Square().Square()
	a88.Square().Square().Square().Square().Square()
	a88.Square().Square().Square().Square().Square()
	a88.Square().Square().Square().Square().Square()
	a88.Square().Square().Square().Square().Square()
	a88.Square().Square().Square().Square()
	a88.Mul(&a44)
	a176.SquareVal(&a88).Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square().Square().Square()
	a176.Square().Square().Square()
	a176.Mul(&a88)
	a220.SquareVal(&a176).Square().Square().Square().Square()
	a220.Square().Square().Square().Square().Square()
	a220.Square().Square().Square().Square().Square()
	a220.Square().Square().Square().Square().Square()
	a220.Square().Square().Square().Square().Square()
	a220.Square().Square().Square().Square().Square()
	a220.Square().Square().Square().Square().Square()
	a220.Square().Square().Square().Square().Square()
	a220.Square().Square().Square().Square()
	a220.Mul(&a44)
	a223.SquareVal(&a220).Square().Square()
	a223.Mul(&a3)

	f.SquareVal(&a223).Square().Square().Square().Square()
	f.Square().Square().Square().Square().Square()
	f.Square().Square().Square().Square().Square()
	f.Square().Square().Square().Square().Square()
	f.Square().Square().Square()
	f.Mul(&a22)
	f.Square().Square().Square().Square().Square()
	f.Mul(&a)
	f.Square().Square().Square()
	f.Mul(&a2)
	f.Square().Square()
	return f.Mul(&a)
}

// SquareRootVal either calculates the square root of the passed value when it
// exists or the square root of the negation of the value when it does not exist
// and stores the result in f in constant time. The return flag is true when the
// calculated square root is for the passed value itself and false when it is for
// its negation.
//
// Since the secp256k1 prime is ≡ 3 (mod 4), the square root is a^((p+1)/4),
// computed via the same addition chain used by FieldVal26 (254 squarings, 13
// multiplications).
func (f *FieldVal64) SquareRootVal(val *FieldVal64) bool {
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

// field64Mul512 sets t = x * y as an unreduced 512-bit product via a row-by-row
// schoolbook multiply.
func field64Mul512(t *[8]uint64, x, y *[4]uint64) {
	a0, a1, a2, a3 := x[0], x[1], x[2], x[3]
	b0, b1, b2, b3 := y[0], y[1], y[2], y[3]

	// Each row forms the 5-limb partial product a * b[j] in (q0..q4) with a
	// single carry chain, then accumulates it into the running product with a
	// second carry chain. Threading every carry through bits.Add64's carry-in
	// (rather than summing hi + carries by value) lets the compiler emit
	// efficient add-with-carry chains. The fresh top limb cannot overflow: the
	// maximum hi + partialCarry + accumulateCarry is < 2^64.
	var c uint64

	// Row 0: p0..p4 = a * b0.
	h0, p0 := bits.Mul64(a0, b0)
	h1, p1 := bits.Mul64(a1, b0)
	h2, p2 := bits.Mul64(a2, b0)
	h3, p3 := bits.Mul64(a3, b0)
	p1, c = bits.Add64(p1, h0, 0)
	p2, c = bits.Add64(p2, h1, c)
	p3, c = bits.Add64(p3, h2, c)
	p4 := h3 + c

	// Row 1: p1..p5 += a * b1.
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
	a0, a1, a2, a3 := a[0], a[1], a[2], a[3]

	// Off-diagonal upper-triangle products (not yet doubled).
	p2, p1 := bits.Mul64(a0, a1)
	h02, l02 := bits.Mul64(a0, a2)
	h03, l03 := bits.Mul64(a0, a3)
	var c uint64
	p2, c = bits.Add64(p2, l02, 0)
	p3, c := bits.Add64(h02, l03, c)
	p4, _ := bits.Add64(h03, 0, c)

	h12, l12 := bits.Mul64(a1, a2)
	p3, c = bits.Add64(p3, l12, 0)
	p4, c = bits.Add64(p4, h12, c)
	p5 := c

	h13, l13 := bits.Mul64(a1, a3)
	p4, c = bits.Add64(p4, l13, 0)
	p5, _ = bits.Add64(p5, h13, c)

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
// constant time using Crandall folding (p = 2^256 - 0x1000003D1).
func field64Reduce512(r *[4]uint64, x *[8]uint64) {
	var t0, t1, t2, t3, t4, h, lo, hi, carry uint64

	h, t0 = bits.Mul64(x[4], field64PrimeComplement)

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

	h, t4 = bits.Mul64(t4, field64PrimeComplement)

	t0, carry = bits.Add64(t0, t4, 0)
	t1, carry = bits.Add64(t1, h, carry)
	t2, carry = bits.Add64(t2, 0, carry)
	t3, carry = bits.Add64(t3, 0, carry)

	// The second fold can carry out of t3. Keep it as a fifth limb (t4) and let
	// the conditional subtract resolve it: the value is < 2p, so one 5-limb
	// subtract of p fully reduces it.
	t4 = carry

	var s0, s1, s2, s3, mask, borrow uint64
	s0, borrow = bits.Sub64(t0, field64Prime0, 0)
	s1, borrow = bits.Sub64(t1, field64Prime1, borrow)
	s2, borrow = bits.Sub64(t2, field64Prime2, borrow)
	s3, borrow = bits.Sub64(t3, field64Prime3, borrow)
	_, borrow = bits.Sub64(t4, 0, borrow)
	mask = -borrow
	r[0] = s0 ^ ((t0 ^ s0) & mask)
	r[1] = s1 ^ ((t1 ^ s1) & mask)
	r[2] = s2 ^ ((t2 ^ s2) & mask)
	r[3] = s3 ^ ((t3 ^ s3) & mask)
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
