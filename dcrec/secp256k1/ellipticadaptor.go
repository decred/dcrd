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
	fieldJacobianToAffine(x, y, z)

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

// IsOnCurve returns whether or not the affine point (x,y) is on the curve.
//
// This is part of the elliptic.Curve interface implementation.  This function
// differs from the crypto/elliptic algorithm since a = 0 not -3.
func (curve *KoblitzCurve) IsOnCurve(x, y *big.Int) bool {
	// Convert big ints to field values for faster arithmetic.
	fx, fy := bigAffineToField(x, y)
	return isOnCurve(fx, fy)
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
	addJacobian(fx1, fy1, fOne, fx2, fy2, fOne, fx3, fy3, fz3)

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
	doubleJacobian(fx1, fy1, fOne, fx3, fy3, fz3)

	// Convert the Jacobian coordinate field values back to affine big
	// integers.
	return fieldJacobianToBigAffine(fx3, fy3, fz3)
}

// ScalarMult returns k*(Bx, By) where k is a big endian integer.
//
// This is part of the elliptic.Curve interface implementation.
func (curve *KoblitzCurve) ScalarMult(Bx, By *big.Int, k []byte) (*big.Int, *big.Int) {
	x, y := bigAffineToField(Bx, By)
	z := new(fieldVal).SetInt(1)
	rx, ry, rz := new(fieldVal), new(fieldVal), new(fieldVal)
	scalarMultJacobian(x, y, z, k, rx, ry, rz)

	// Convert the Jacobian coordinate field values back to affine big.Ints.
	return fieldJacobianToBigAffine(rx, ry, rz)
}

// ScalarBaseMult returns k*G where G is the base point of the group and k is a
// big endian integer.
//
// This is part of the elliptic.Curve interface implementation.
func (curve *KoblitzCurve) ScalarBaseMult(k []byte) (*big.Int, *big.Int) {
	rx, ry, rz := new(fieldVal), new(fieldVal), new(fieldVal)
	scalarBaseMultJacobian(k, rx, ry, rz)

	// Convert the Jacobian coordinate field values back to affine big.Ints.
	return fieldJacobianToBigAffine(rx, ry, rz)
}

// ToECDSA returns the public key as a *ecdsa.PublicKey.
func (p PublicKey) ToECDSA() *ecdsa.PublicKey {
	ecpk := ecdsa.PublicKey(p)
	return &ecpk
}

// ToECDSA returns the private key as a *ecdsa.PrivateKey.
func (p *PrivateKey) ToECDSA() *ecdsa.PrivateKey {
	fx, fy, fz := new(fieldVal), new(fieldVal), new(fieldVal)
	scalarBaseMultJacobian(p.D.Bytes(), fx, fy, fz)
	x, y := fieldJacobianToBigAffine(fx, fy, fz)
	return &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: S256(),
			X:     x,
			Y:     y,
		},
		D: p.D,
	}
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
