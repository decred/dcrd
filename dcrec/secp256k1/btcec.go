// Copyright 2010 The Go Authors. All rights reserved.
// Copyright (c) 2015-2020 The Decred developers
// Copyright 2011 ThePiachu. All rights reserved.
// Copyright 2013-2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

// References:
//   [GECC]: Guide to Elliptic Curve Cryptography (Hankerson, Menezes, Vanstone)

// This package operates, internally, on Jacobian coordinates. For a given
// (x, y) position on the curve, the Jacobian coordinates are (x1, y1, z1)
// where x = x1/z1² and y = y1/z1³. The greatest speedups come when the whole
// calculation can be performed within the transform (as in ScalarMult and
// ScalarBaseMult). But even for Add and Double, it's faster to apply and
// reverse the transform than to operate in affine coordinates.

import (
	"math/big"
)

// splitK returns a balanced length-two representation of k and their signs.
// This is algorithm 3.74 from [GECC].
//
// One thing of note about this algorithm is that no matter what c1 and c2 are,
// the final equation of k = k1 + k2 * lambda (mod n) will hold.  This is
// provable mathematically due to how a1/b1/a2/b2 are computed.
//
// c1 and c2 are chosen to minimize the max(k1,k2).
func (curve *KoblitzCurve) splitK(k []byte) ([]byte, []byte, int, int) {
	// All math here is done with big.Int, which is slow.
	// At some point, it might be useful to write something similar to
	// fieldVal but for N instead of P as the prime field if this ends up
	// being a bottleneck.
	bigIntK := new(big.Int)
	c1, c2 := new(big.Int), new(big.Int)
	tmp1, tmp2 := new(big.Int), new(big.Int)
	k1, k2 := new(big.Int), new(big.Int)

	bigIntK.SetBytes(k)
	// c1 = round(b2 * k / n) from step 4.
	// Rounding isn't really necessary and costs too much, hence skipped
	c1.Mul(curve.b2, bigIntK)
	c1.Div(c1, curve.N)
	// c2 = round(b1 * k / n) from step 4 (sign reversed to optimize one step)
	// Rounding isn't really necessary and costs too much, hence skipped
	c2.Mul(curve.b1, bigIntK)
	c2.Div(c2, curve.N)
	// k1 = k - c1 * a1 - c2 * a2 from step 5 (note c2's sign is reversed)
	tmp1.Mul(c1, curve.a1)
	tmp2.Mul(c2, curve.a2)
	k1.Sub(bigIntK, tmp1)
	k1.Add(k1, tmp2)
	// k2 = - c1 * b1 - c2 * b2 from step 5 (note c2's sign is reversed)
	tmp1.Mul(c1, curve.b1)
	tmp2.Mul(c2, curve.b2)
	k2.Sub(tmp2, tmp1)

	// Note Bytes() throws out the sign of k1 and k2. This matters
	// since k1 and/or k2 can be negative. Hence, we pass that
	// back separately.
	return k1.Bytes(), k2.Bytes(), k1.Sign(), k2.Sign()
}

// moduloReduce reduces k from more than 32 bytes to 32 bytes and under.  This
// is done by doing a simple modulo curve.N.  We can do this since G^N = 1 and
// thus any other valid point on the elliptic curve has the same order.
func (curve *KoblitzCurve) moduloReduce(k []byte) []byte {
	// Since the order of G is curve.N, we can use a much smaller number
	// by doing modulo curve.N
	if len(k) > curve.byteSize {
		// Reduce k by performing modulo curve.N.
		tmpK := new(big.Int).SetBytes(k)
		tmpK.Mod(tmpK, curve.N)
		return tmpK.Bytes()
	}

	return k
}

// naf takes a positive integer k and returns the Non-Adjacent Form (NAF) as two
// byte slices.  The first is where 1s will be.  The second is where -1s will
// be.  NAF is convenient in that on average, only 1/3rd of its values are
// non-zero.  This is algorithm 3.30 from [GECC].
//
// Essentially, this makes it possible to minimize the number of operations
// since the resulting ints returned will be at least 50% 0s.
func naf(k []byte) ([]byte, []byte) {
	// The essence of this algorithm is that whenever we have consecutive 1s
	// in the binary, we want to put a -1 in the lowest bit and get a bunch
	// of 0s up to the highest bit of consecutive 1s.  This is due to this
	// identity:
	// 2^n + 2^(n-1) + 2^(n-2) + ... + 2^(n-k) = 2^(n+1) - 2^(n-k)
	//
	// The algorithm thus may need to go 1 more bit than the length of the
	// bits we actually have, hence bits being 1 bit longer than was
	// necessary.  Since we need to know whether adding will cause a carry,
	// we go from right-to-left in this addition.
	var carry, curIsOne, nextIsOne bool
	// these default to zero
	retPos := make([]byte, len(k)+1)
	retNeg := make([]byte, len(k)+1)
	for i := len(k) - 1; i >= 0; i-- {
		curByte := k[i]
		for j := uint(0); j < 8; j++ {
			curIsOne = curByte&1 == 1
			if j == 7 {
				if i == 0 {
					nextIsOne = false
				} else {
					nextIsOne = k[i-1]&1 == 1
				}
			} else {
				nextIsOne = curByte&2 == 2
			}
			if carry {
				if curIsOne {
					// This bit is 1, so continue to carry
					// and don't need to do anything.
				} else {
					// We've hit a 0 after some number of
					// 1s.
					if nextIsOne {
						// Start carrying again since
						// a new sequence of 1s is
						// starting.
						retNeg[i+1] += 1 << j
					} else {
						// Stop carrying since 1s have
						// stopped.
						carry = false
						retPos[i+1] += 1 << j
					}
				}
			} else if curIsOne {
				if nextIsOne {
					// If this is the start of at least 2
					// consecutive 1s, set the current one
					// to -1 and start carrying.
					retNeg[i+1] += 1 << j
					carry = true
				} else {
					// This is a singleton, not consecutive
					// 1s.
					retPos[i+1] += 1 << j
				}
			}
			curByte >>= 1
		}
	}
	if carry {
		retPos[0] = 1
		return retPos, retNeg
	}
	return retPos[1:], retNeg[1:]
}
