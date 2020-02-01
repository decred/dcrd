// Copyright 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

// References:
//   [SECG]: Recommended Elliptic Curve Domain Parameters
//     https://www.secg.org/sec2-v2.pdf
//
//   [GECC]: Guide to Elliptic Curve Cryptography (Hankerson, Menezes, Vanstone)

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"math/big"
	"sync"
)

// CurveParams contains the parameters for the secp256k1 curve.
type CurveParams struct {
	*elliptic.CurveParams
	q *big.Int
	H int // cofactor of the curve.

	// byteSize is simply the bit size / 8 and is provided for convenience
	// since it is calculated repeatedly.
	byteSize int

	// The next 6 values are used specifically for endomorphism
	// optimizations in ScalarMult.

	// lambda must fulfill lambda^3 = 1 mod N where N is the order of G.
	lambda *big.Int

	// beta must fulfill beta^3 = 1 mod P where P is the prime field of the
	// curve.
	beta *fieldVal

	// See the EndomorphismVectors in gensecp256k1.go to see how these are
	// derived.
	a1 *big.Int
	b1 *big.Int
	a2 *big.Int
	b2 *big.Int
}

// Curve parameters taken from [SECG] section 2.4.1.
var fieldPrime = fromHex("fffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc2f")
var curveParams = CurveParams{
	CurveParams: &elliptic.CurveParams{
		P:       fieldPrime,
		N:       fromHex("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141"),
		B:       fromHex("0000000000000000000000000000000000000000000000000000000000000007"),
		Gx:      fromHex("79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"),
		Gy:      fromHex("483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8"),
		BitSize: 256,
	},
	H: 1,
	q: new(big.Int).Div(new(big.Int).Add(fieldPrime, big.NewInt(1)),
		big.NewInt(4)),

	// Provided for convenience since this gets computed repeatedly.
	byteSize: 256 / 8,

	// Next 6 constants are from Hal Finney's bitcointalk.org post:
	// https://bitcointalk.org/index.php?topic=3238.msg45565#msg45565
	// May he rest in peace.
	//
	// They have also been independently derived from the code in the
	// EndomorphismVectors function in genstatics.go.
	lambda: fromHex("5363ad4cc05c30e0a5261c028812645a122e22ea20816678df02967c1b23bd72"),
	beta:   new(fieldVal).SetHex("7ae96a2b657c07106e64479eac3434e99cf0497512f58995c1396c28719501ee"),
	a1:     fromHex("3086d221a7d46bcde86c90e49284eb15"),
	b1:     fromHex("-e4437ed6010e88286f547fa90abfe4c3"),
	a2:     fromHex("114ca50f7a8e2f3f657c1108d9d44cfd8"),
	b2:     fromHex("3086d221a7d46bcde86c90e49284eb15"),

	// Alternatively, we can use the parameters below, however, they seem
	//  to be about 8% slower.
	// secp256k1.lambda = fromHex("ac9c52b33fa3cf1f5ad9e3fd77ed9ba4a880b9fc8ec739c2e0cfc810b51283ce")
	// secp256k1.beta = new(fieldVal).SetHex("851695d49a83f8ef919bb86153cbcb16630fb68aed0a766a3ec693d68e6afa40")
	// secp256k1.a1 = fromHex("e4437ed6010e88286f547fa90abfe4c3")
	// secp256k1.b1 = fromHex("-3086d221a7d46bcde86c90e49284eb15")
	// secp256k1.a2 = fromHex("3086d221a7d46bcde86c90e49284eb15")
	// secp256k1.b2 = fromHex("114ca50f7a8e2f3f657c1108d9d44cfd8")
}

// Params returns the secp256k1 curve parameters for convenience.
func Params() *CurveParams {
	return &curveParams
}

// KoblitzCurve provides an implementation for secp256k1 that fits the ECC Curve
// interface from crypto/elliptic.
type KoblitzCurve struct {
	*CurveParams

	// bytePoints
	bytePoints *[32][256][3]fieldVal
}

// bigAffineToField takes an affine point (x, y) as big integers and converts
// it to an affine point as field values.
func bigAffineToField(x, y *big.Int) (*fieldVal, *fieldVal) {
	x3, y3 := new(fieldVal), new(fieldVal)
	x3.SetByteSlice(x.Bytes())
	y3.SetByteSlice(y.Bytes())

	return x3, y3
}

// fieldJacobianToBigAffine takes a Jacobian point (x, y, z) as field values and
// converts it to an affine point as big integers.
func fieldJacobianToBigAffine(x, y, z *fieldVal) (*big.Int, *big.Int) {
	// Inversions are expensive and both point addition and point doubling
	// are faster when working with points that have a z value of one.  So,
	// if the point needs to be converted to affine, go ahead and normalize
	// the point itself at the same time as the calculation is the same.
	var zInv, tempZ fieldVal
	zInv.Set(z).Inverse()   // zInv = Z^-1
	tempZ.SquareVal(&zInv)  // tempZ = Z^-2
	x.Mul(&tempZ)           // X = X/Z^2 (mag: 1)
	y.Mul(tempZ.Mul(&zInv)) // Y = Y/Z^3 (mag: 1)
	z.SetInt(1)             // Z = 1 (mag: 1)

	// Normalize the x and y values.
	x.Normalize()
	y.Normalize()

	// Convert the field values for the now affine point to big.Ints.
	x3, y3 := new(big.Int), new(big.Int)
	x3.SetBytes(x.Bytes()[:])
	y3.SetBytes(y.Bytes()[:])
	return x3, y3
}

// Params returns the parameters for the curve.
//
// This is part of the elliptic.Curve interface implementation.
func (curve *KoblitzCurve) Params() *elliptic.CurveParams {
	return curve.CurveParams.CurveParams
}

// IsOnCurve returns boolean if the point (x,y) is on the curve.
//
// This is part of the elliptic.Curve interface implementation.  This function
// differs from the crypto/elliptic algorithm since a = 0 not -3.
func (curve *KoblitzCurve) IsOnCurve(x, y *big.Int) bool {
	// Convert big ints to field values for faster arithmetic.
	fx, fy := bigAffineToField(x, y)

	// TODO(davec): Split to curve.go?

	// Elliptic curve equation for secp256k1 is: y^2 = x^3 + 7
	y2 := new(fieldVal).SquareVal(fy).Normalize()
	result := new(fieldVal).SquareVal(fx).Mul(fx).AddInt(7).Normalize()
	return y2.Equals(result)
}

// Add returns the sum of (x1,y1) and (x2,y2).
//
// This is part of the elliptic.Curve interface implementation.
func (curve *KoblitzCurve) Add(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
	// A point at infinity is the identity according to the group law for
	// elliptic curve cryptography.  Thus, ∞ + P = P and P + ∞ = P.
	if x1.Sign() == 0 && y1.Sign() == 0 {
		return x2, y2
	}
	if x2.Sign() == 0 && y2.Sign() == 0 {
		return x1, y1
	}

	// Convert the affine coordinates from big integers to field values
	// and do the point addition in Jacobian projective space.
	fx1, fy1 := bigAffineToField(x1, y1)
	fx2, fy2 := bigAffineToField(x2, y2)
	fx3, fy3, fz3 := new(fieldVal), new(fieldVal), new(fieldVal)
	fOne := new(fieldVal).SetInt(1)
	curve.addJacobian(fx1, fy1, fOne, fx2, fy2, fOne, fx3, fy3, fz3)

	// Convert the Jacobian coordinate field values back to affine big
	// integers.
	return fieldJacobianToBigAffine(fx3, fy3, fz3)
}

// Double returns 2*(x1,y1).
//
// This is part of the elliptic.Curve interface implementation.
func (curve *KoblitzCurve) Double(x1, y1 *big.Int) (*big.Int, *big.Int) {
	if y1.Sign() == 0 {
		return new(big.Int), new(big.Int)
	}

	// Convert the affine coordinates from big integers to field values
	// and do the point doubling in Jacobian projective space.
	fx1, fy1 := bigAffineToField(x1, y1)
	fx3, fy3, fz3 := new(fieldVal), new(fieldVal), new(fieldVal)
	fOne := new(fieldVal).SetInt(1)
	curve.doubleJacobian(fx1, fy1, fOne, fx3, fy3, fz3)

	// Convert the Jacobian coordinate field values back to affine big
	// integers.
	return fieldJacobianToBigAffine(fx3, fy3, fz3)
}

// ScalarMult returns k*(Bx, By) where k is a big endian integer.
//
// This is part of the elliptic.Curve interface implementation.
func (curve *KoblitzCurve) ScalarMult(Bx, By *big.Int, k []byte) (*big.Int, *big.Int) {
	// Point Q = ∞ (point at infinity).
	qx, qy, qz := new(fieldVal), new(fieldVal), new(fieldVal)

	// Decompose K into k1 and k2 in order to halve the number of EC ops.
	// See Algorithm 3.74 in [GECC].
	k1, k2, signK1, signK2 := curve.splitK(curve.moduloReduce(k))

	// The main equation here to remember is:
	//   k * P = k1 * P + k2 * ϕ(P)
	//
	// P1 below is P in the equation, P2 below is ϕ(P) in the equation
	p1x, p1y := bigAffineToField(Bx, By)
	p1yNeg := new(fieldVal).NegateVal(p1y, 1)
	p1z := new(fieldVal).SetInt(1)

	// NOTE: ϕ(x,y) = (βx,y).  The Jacobian z coordinate is 1, so this math
	// goes through.
	p2x := new(fieldVal).Mul2(p1x, curve.beta)
	p2y := new(fieldVal).Set(p1y)
	p2yNeg := new(fieldVal).NegateVal(p2y, 1)
	p2z := new(fieldVal).SetInt(1)

	// Flip the positive and negative values of the points as needed
	// depending on the signs of k1 and k2.  As mentioned in the equation
	// above, each of k1 and k2 are multiplied by the respective point.
	// Since -k * P is the same thing as k * -P, and the group law for
	// elliptic curves states that P(x, y) = -P(x, -y), it's faster and
	// simplifies the code to just make the point negative.
	if signK1 == -1 {
		p1y, p1yNeg = p1yNeg, p1y
	}
	if signK2 == -1 {
		p2y, p2yNeg = p2yNeg, p2y
	}

	// NAF versions of k1 and k2 should have a lot more zeros.
	//
	// The Pos version of the bytes contain the +1s and the Neg versions
	// contain the -1s.
	k1PosNAF, k1NegNAF := naf(k1)
	k2PosNAF, k2NegNAF := naf(k2)
	k1Len := len(k1PosNAF)
	k2Len := len(k2PosNAF)

	m := k1Len
	if m < k2Len {
		m = k2Len
	}

	// Add left-to-right using the NAF optimization.  See algorithm 3.77
	// from [GECC].  This should be faster overall since there will be a lot
	// more instances of 0, hence reducing the number of Jacobian additions
	// at the cost of 1 possible extra doubling.
	var k1BytePos, k1ByteNeg, k2BytePos, k2ByteNeg byte
	for i := 0; i < m; i++ {
		// Since we're going left-to-right, pad the front with 0s.
		if i < m-k1Len {
			k1BytePos = 0
			k1ByteNeg = 0
		} else {
			k1BytePos = k1PosNAF[i-(m-k1Len)]
			k1ByteNeg = k1NegNAF[i-(m-k1Len)]
		}
		if i < m-k2Len {
			k2BytePos = 0
			k2ByteNeg = 0
		} else {
			k2BytePos = k2PosNAF[i-(m-k2Len)]
			k2ByteNeg = k2NegNAF[i-(m-k2Len)]
		}

		for j := 7; j >= 0; j-- {
			// Q = 2 * Q
			curve.doubleJacobian(qx, qy, qz, qx, qy, qz)

			if k1BytePos&0x80 == 0x80 {
				curve.addJacobian(qx, qy, qz, p1x, p1y, p1z,
					qx, qy, qz)
			} else if k1ByteNeg&0x80 == 0x80 {
				curve.addJacobian(qx, qy, qz, p1x, p1yNeg, p1z,
					qx, qy, qz)
			}

			if k2BytePos&0x80 == 0x80 {
				curve.addJacobian(qx, qy, qz, p2x, p2y, p2z,
					qx, qy, qz)
			} else if k2ByteNeg&0x80 == 0x80 {
				curve.addJacobian(qx, qy, qz, p2x, p2yNeg, p2z,
					qx, qy, qz)
			}
			k1BytePos <<= 1
			k1ByteNeg <<= 1
			k2BytePos <<= 1
			k2ByteNeg <<= 1
		}
	}

	// Convert the Jacobian coordinate field values back to affine big.Ints.
	return fieldJacobianToBigAffine(qx, qy, qz)
}

// ScalarBaseMult returns k*G where G is the base point of the group and k is a
// big endian integer.
//
// This is part of the elliptic.Curve interface implementation.
func (curve *KoblitzCurve) ScalarBaseMult(k []byte) (*big.Int, *big.Int) {
	newK := curve.moduloReduce(k)
	diff := len(curve.bytePoints) - len(newK)

	// Point Q = ∞ (point at infinity).
	qx, qy, qz := new(fieldVal), new(fieldVal), new(fieldVal)

	// curve.bytePoints has all 256 byte points for each 8-bit window. The
	// strategy is to add up the byte points. This is best understood by
	// expressing k in base-256 which it already sort of is.
	// Each "digit" in the 8-bit window can be looked up using bytePoints
	// and added together.
	for i, byteVal := range newK {
		p := curve.bytePoints[diff+i][byteVal]
		curve.addJacobian(qx, qy, qz, &p[0], &p[1], &p[2], qx, qy, qz)
	}
	return fieldJacobianToBigAffine(qx, qy, qz)
}

// ToECDSA returns the public key as a *ecdsa.PublicKey.
func (p PublicKey) ToECDSA() *ecdsa.PublicKey {
	ecpk := ecdsa.PublicKey(p)
	return &ecpk
}

// ToECDSA returns the private key as a *ecdsa.PrivateKey.
func (p *PrivateKey) ToECDSA() *ecdsa.PrivateKey {
	return (*ecdsa.PrivateKey)(p)
}

// fromHex converts the passed hex string into a big integer pointer and will
// panic is there is an error.  This is only provided for the hard-coded
// constants so errors in the source code can bet detected. It will only (and
// must only) be called for initialization purposes.
func fromHex(s string) *big.Int {
	r, ok := new(big.Int).SetString(s, 16)
	if !ok {
		panic("invalid hex in source file: " + s)
	}
	return r
}

var initonce sync.Once
var secp256k1 KoblitzCurve

func initAll() {
	initS256()
}

func initS256() {
	secp256k1.CurveParams = &curveParams

	// Deserialize and set the pre-computed table used to accelerate scalar
	// base multiplication.  This is hard-coded data, so any errors are
	// panics because it means something is wrong in the source code.
	if err := loadBytePoints(); err != nil {
		panic(err)
	}
}

// S256 returns a Curve which implements secp256k1.
func S256() *KoblitzCurve {
	initonce.Do(initAll)
	return &secp256k1
}
