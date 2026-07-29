// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package field4x64

import (
	"math/bits"

	"github.com/decred/dcrd/dcrec/secp256k1/v4/internal/arith"
)

// field64Reduce512 reduces a 512-bit little-endian limb array modulo p in
// constant time and stores the result in r using pure Go.
func field64Reduce512(r *[4]uint64, x *[8]uint64) {
	// This algorithm has been formally verified, including its intermediate
	// bounds, carry assumptions, and functional correctness.  The verification
	// artifacts are available in internal/proofs.

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

	// Note that since hi is the upper 64 bits of the product of a uint64 with
	// c and c < 2^33:
	//   hi ≤ floor((2^64-1)(2^33 - 1) / 2^64) = 2^33 - 2
	//
	// Then, because carry ≤ 1, a loose bound for h is:
	//   h ≤ hi + 1 = 2^33 - 1 < 2^64
	//
	// Therefore, it is safe to discard the carry and the same applies to the
	// next two limbs (second h and first t4).
	hi, lo = bits.Mul64(x[5], field64PrimeComplement)
	t1, carry = bits.Add64(lo, h, 0)
	h, _ = bits.Add64(hi, 0, carry)

	hi, lo = bits.Mul64(x[6], field64PrimeComplement)
	t2, carry = bits.Add64(lo, h, 0)
	h, _ = bits.Add64(hi, 0, carry)

	hi, lo = bits.Mul64(x[7], field64PrimeComplement)
	t3, carry = bits.Add64(lo, h, 0)
	t4, _ = bits.Add64(hi, 0, carry)

	// The carryless add into t4 below is safe because, per the bound above,
	// t4 ≤ 2^33 - 1 and carry ≤ 1, so:
	//  t4 ≤ (2^33 - 1) + 1 = 2^33 < 2^64
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
	r[0] = arith.ConstantTimeSelect64(borrow, t0, s0)
	r[1] = arith.ConstantTimeSelect64(borrow, t1, s1)
	r[2] = arith.ConstantTimeSelect64(borrow, t2, s2)
	r[3] = arith.ConstantTimeSelect64(borrow, t3, s3)
}

// field64MulReduceGeneric sets r = a * b (mod p).  This is a generic
// implementation that performs the multiplication and reduction steps
// separately without any reliance on specific hardware extensions.
func field64MulReduceGeneric(r *[4]uint64, a, b *[4]uint64) {
	var product [8]uint64
	arith.Mul512(&product, a, b)
	field64Reduce512(r, &product)
}

// field64SquareReduceGeneric sets r = a^2 (mod p).  This is a generic
// implementation that performs the squaring and reduction steps separately
// without any reliance on specific hardware extensions.
func field64SquareReduceGeneric(r *[4]uint64, a *[4]uint64) {
	var product [8]uint64
	arith.Square512(&product, a)
	field64Reduce512(r, &product)
}
