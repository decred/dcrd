// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package arith provides low-level constant-time primitives and
// modulus-agnostic arithmetic.
package arith

import "math/bits"

// Mul512 sets t = x * y as an unreduced 512-bit product.
func Mul512(t *[8]uint64, x, y *[4]uint64) {
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

// Square512 sets t = a^2 as an unreduced 512-bit product.
func Square512(t *[8]uint64, a *[4]uint64) {
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
	// A full proof involves case analysis that is omitted here since the
	// impossibility of the carry is formally proven in internal/proofs, but the
	// key point is that the only way the final add could have a carry is if all
	// 3 of the following conditions were simultaneously true:
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
