// Copyright (c) 2015-2026 The Decred developers
// Copyright 2013-2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"encoding/hex"
	"math/bits"
)

// References:
//   [SECG]: Recommended Elliptic Curve Domain Parameters
//     https://www.secg.org/sec2-v2.pdf
//
//   [GECC]: Guide to Elliptic Curve Cryptography (Hankerson, Menezes, Vanstone)
//
//   [BRID]: On Binary Representations of Integers with Digits -1, 0, 1
//           (Prodinger, Helmut)
//
//   [STWS]: Secure-TWS: Authenticating Node to Multi-user Communication in
//           Shared Sensor Networks (Oliveira, Leonardo B. et al)

// All group operations are performed using Jacobian coordinates.  For a given
// (x, y) position on the curve, the Jacobian coordinates are (x1, y1, z1)
// where x = x1/z1^2 and y = y1/z1^3.

// mustFieldValInternal converts the passed hex string into a [FieldVal] and
// will panic if there is an error.  Values that overflow are treated as an
// error unless the allow overflow flag is set.
//
// This is only provided for the hard-coded constants so errors in the source
// code can be detected. It will only (and must only) be called with hard-coded
// values.
func mustFieldValInternal(s string, allowOverflow bool) *FieldVal {
	if len(s)%2 != 0 {
		s = "0" + s
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	if len(b) > 32 {
		panic("hex in source file overflows uint256: " + s)
	}
	var f FieldVal
	if overflow := f.SetByteSlice(b); overflow && !allowOverflow {
		panic("hex in source file overflows mod N scalar: " + s)
	}
	return &f
}

// mustFieldVal converts the passed hex string into a [FieldVal] and will panic
// if there is an error.  Values that overflow are treated as an error.
//
// This is only provided for the hard-coded constants so errors in the source
// code can be detected. It will only (and must only) be called with hard-coded
// values.
func mustFieldVal(s string) *FieldVal {
	return mustFieldValInternal(s, false)
}

// mustModNScalarInternal converts the passed hex string into a [ModNScalar] and
// will panic if there is an error.  Values that overflow are treated as an
// error unless the allow overflow flag is set.
//
// This is only provided for the hard-coded constants so errors in the source
// code can be detected. It will only (and must only) be called with hard-coded
// values.
func mustModNScalarInternal(s string, allowOverflow bool) *ModNScalar {
	var isNegative bool
	if len(s) > 0 && s[0] == '-' {
		isNegative = true
		s = s[1:]
	}
	if len(s)%2 != 0 {
		s = "0" + s
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	if len(b) > 32 {
		panic("hex in source file overflows uint256: " + s)
	}
	var scalar ModNScalar
	if overflow := scalar.SetByteSlice(b); overflow && !allowOverflow {
		panic("hex in source file overflows mod N scalar: " + s)
	}
	if isNegative {
		scalar.Negate()
	}
	return &scalar
}

// mustModNScalar converts the passed hex string into a [ModNScalar] and will
// panic if there is an error.  Values that overflow are treated as an error.
//
// This is only provided for the hard-coded constants so errors in the source
// code can be detected.  It will only (and must only) be called with hard-coded
// values.
func mustModNScalar(s string) *ModNScalar {
	return mustModNScalarInternal(s, false)
}

var (
	// The following constants are used to accelerate scalar point
	// multiplication through the use of the endomorphism:
	//
	// φ(Q) ⟼ λ*Q = (β*Q.x mod p, Q.y)
	//
	// See the code in the deriveEndomorphismParams function in genprecomps.go
	// for details on their derivation.
	//
	// Additionally, see the scalar multiplication function in this file for
	// details on how they are used.
	endoNegLambda = mustModNScalar("-5363ad4cc05c30e0a5261c028812645a122e22ea20816678df02967c1b23bd72")
	endoBeta      = mustFieldVal("7ae96a2b657c07106e64479eac3434e99cf0497512f58995c1396c28719501ee")
	endoNegB1     = mustModNScalar("e4437ed6010e88286f547fa90abfe4c3")
	endoNegB2     = mustModNScalar("-3086d221a7d46bcde86c90e49284eb15")
	endoZ1        = mustModNScalar("3086d221a7d46bcde86c90e49284eb153daa8a1471e8ca7f")
	endoZ2        = mustModNScalar("e4437ed6010e88286f547fa90abfe4c4221208ac9df506c6")

	// Alternatively, the following parameters are valid as well, however,
	// benchmarks show them to be about 2% slower in practice.
	// endoNegLambda = mustModNScalar("-ac9c52b33fa3cf1f5ad9e3fd77ed9ba4a880b9fc8ec739c2e0cfc810b51283ce")
	// endoBeta      = mustFieldVal("851695d49a83f8ef919bb86153cbcb16630fb68aed0a766a3ec693d68e6afa40")
	// endoNegB1     = mustModNScalar("3086d221a7d46bcde86c90e49284eb15")
	// endoNegB2     = mustModNScalar("-114ca50f7a8e2f3f657c1108d9d44cfd8")
	// endoZ1        = mustModNScalar("114ca50f7a8e2f3f657c1108d9d44cfd95fbc92c10fddd145")
	// endoZ2        = mustModNScalar("3086d221a7d46bcde86c90e49284eb153daa8a1471e8ca7f")
)

// JacobianPoint is an element of the group formed by the secp256k1 curve in
// Jacobian projective coordinates and thus represents a point on the curve.
type JacobianPoint struct {
	// The X coordinate in Jacobian projective coordinates.  The affine point is
	// X/z^2.
	X FieldVal

	// The Y coordinate in Jacobian projective coordinates.  The affine point is
	// Y/z^3.
	Y FieldVal

	// The Z coordinate in Jacobian projective coordinates.
	Z FieldVal
}

// MakeJacobianPoint returns a Jacobian point with the provided X, Y, and Z
// coordinates.
func MakeJacobianPoint(x, y, z *FieldVal) JacobianPoint {
	var p JacobianPoint
	p.X.Set(x)
	p.Y.Set(y)
	p.Z.Set(z)
	return p
}

// Set sets the Jacobian point to the provided point.
func (p *JacobianPoint) Set(other *JacobianPoint) {
	p.X.Set(&other.X)
	p.Y.Set(&other.Y)
	p.Z.Set(&other.Z)
}

// ToAffine reduces the Z value of the existing point to 1 effectively
// making it an affine coordinate in constant time.  The point will be
// normalized.
func (p *JacobianPoint) ToAffine() {
	// Inversions are expensive and both point addition and point doubling
	// are faster when working with points that have a z value of one.  So,
	// if the point needs to be converted to affine, go ahead and normalize
	// the point itself at the same time as the calculation is the same.
	var zInv, tempZ FieldVal
	zInv.Set(&p.Z).Inverse()  // zInv = Z^-1
	tempZ.SquareVal(&zInv)    // tempZ = Z^-2
	p.X.Mul(&tempZ)           // X = X/Z^2 (mag: 1)
	p.Y.Mul(tempZ.Mul(&zInv)) // Y = Y/Z^3 (mag: 1)
	p.Z.SetInt(1)             // Z = 1 (mag: 1)

	// Normalize the x and y values.
	p.X.Normalize()
	p.Y.Normalize()
}

// EquivalentNonConst returns whether or not two Jacobian points represent the
// same affine point in *non-constant* time.
func (p *JacobianPoint) EquivalentNonConst(other *JacobianPoint) bool {
	// Since the point at infinity is the identity element for the group, note
	// that P = P + ∞ trivially implies that P - P = ∞.
	//
	// Use that fact to determine if the points represent the same affine point.
	var result JacobianPoint
	result.Set(p)
	result.Y.Normalize().Negate(1).Normalize()
	AddNonConst(&result, other, &result)
	return (result.X.IsZero() && result.Y.IsZero()) || result.Z.IsZero()
}

// addZ1AndZ2EqualsOne adds two Jacobian points that are already known to have
// z values of 1 and stores the result in the provided result param.  That is to
// say result = p1 + p2.  It performs faster addition than the generic add
// routine since less arithmetic is needed due to the ability to avoid the z
// value multiplications.
//
// NOTE: The points must be normalized for this function to return the correct
// result.  The resulting point will be normalized.
func addZ1AndZ2EqualsOne(p1, p2, result *JacobianPoint) {
	// To compute the point addition efficiently, this implementation splits
	// the equation into intermediate elements which are used to minimize
	// the number of field multiplications using the method shown at:
	// https://hyperelliptic.org/EFD/g1p/auto-shortw-jacobian-0.html#addition-mmadd-2007-bl
	//
	// In particular it performs the calculations using the following:
	// H = X2-X1, HH = H^2, I = 4*HH, J = H*I, r = 2*(Y2-Y1), V = X1*I
	// X3 = r^2-J-2*V, Y3 = r*(V-X3)-2*Y1*J, Z3 = 2*H
	//
	// This results in a cost of 4 field multiplications, 2 field squarings,
	// 6 field additions, and 5 integer multiplications.
	x1, y1 := &p1.X, &p1.Y
	x2, y2 := &p2.X, &p2.Y
	x3, y3, z3 := &result.X, &result.Y, &result.Z

	// When the x coordinates are the same for two points on the curve, the
	// y coordinates either must be the same, in which case it is point
	// doubling, or they are opposite and the result is the point at
	// infinity per the group law for elliptic curve cryptography.
	if x1.Equals(x2) {
		if y1.Equals(y2) {
			// Since x1 == x2 and y1 == y2, point doubling must be
			// done, otherwise the addition would end up dividing
			// by zero.
			DoubleNonConst(p1, result)
			return
		}

		// Since x1 == x2 and y1 == -y2, the sum is the point at
		// infinity per the group law.
		x3.SetInt(0)
		y3.SetInt(0)
		z3.SetInt(0)
		return
	}

	// Calculate X3, Y3, and Z3 according to the intermediate elements
	// breakdown above.
	var h, i, j, r, v FieldVal
	var negJ, neg2V, negX3 FieldVal
	h.Set(x1).Negate(1).Add(x2)                // H = X2-X1 (mag: 3)
	i.SquareVal(&h).MulBy4()                   // I = 4*H^2 (mag: 4)
	j.Mul2(&h, &i)                             // J = H*I (mag: 1)
	r.Set(y1).Negate(1).Add(y2).MulBy2()       // r = 2*(Y2-Y1) (mag: 6)
	v.Mul2(x1, &i)                             // V = X1*I (mag: 1)
	negJ.Set(&j).Negate(1)                     // negJ = -J (mag: 2)
	neg2V.Set(&v).MulBy2().Negate(2)           // neg2V = -(2*V) (mag: 3)
	x3.Set(&r).Square().Add(&negJ).Add(&neg2V) // X3 = r^2-J-2*V (mag: 6)
	negX3.Set(x3).Negate(6)                    // negX3 = -X3 (mag: 7)
	j.Mul(y1).MulBy2().Negate(2)               // J = -(2*Y1*J) (mag: 3)
	y3.Set(&v).Add(&negX3).Mul(&r).Add(&j)     // Y3 = r*(V-X3)-2*Y1*J (mag: 4)
	z3.Set(&h).MulBy2()                        // Z3 = 2*H (mag: 6)

	// Normalize the resulting field values as needed.
	x3.Normalize()
	y3.Normalize()
	z3.Normalize()
}

// addZ1EqualsZ2 adds two Jacobian points that are already known to have the
// same z value and stores the result in the provided result param.  That is to
// say result = p1 + p2.  It performs faster addition than the generic add
// routine since less arithmetic is needed due to the known equivalence.
//
// NOTE: The points must be normalized for this function to return the correct
// result.  The resulting point will be normalized.
func addZ1EqualsZ2(p1, p2, result *JacobianPoint) {
	// To compute the point addition efficiently, this implementation splits
	// the equation into intermediate elements which are used to minimize
	// the number of field multiplications using a slightly modified version
	// of the method shown at:
	// https://hyperelliptic.org/EFD/g1p/auto-shortw-jacobian-0.html#addition-zadd-2007-m
	//
	// In particular it performs the calculations using the following:
	// A = X2-X1, B = A^2, C=Y2-Y1, D = C^2, E = X1*B, F = X2*B
	// X3 = D-E-F, Y3 = C*(E-X3)-Y1*(F-E), Z3 = Z1*A
	//
	// This results in a cost of 5 field multiplications, 2 field squarings,
	// 9 field additions, and 0 integer multiplications.
	x1, y1, z1 := &p1.X, &p1.Y, &p1.Z
	x2, y2 := &p2.X, &p2.Y
	x3, y3, z3 := &result.X, &result.Y, &result.Z

	// When the x coordinates are the same for two points on the curve, the
	// y coordinates either must be the same, in which case it is point
	// doubling, or they are opposite and the result is the point at
	// infinity per the group law for elliptic curve cryptography.
	if x1.Equals(x2) {
		if y1.Equals(y2) {
			// Since x1 == x2 and y1 == y2, point doubling must be
			// done, otherwise the addition would end up dividing
			// by zero.
			DoubleNonConst(p1, result)
			return
		}

		// Since x1 == x2 and y1 == -y2, the sum is the point at
		// infinity per the group law.
		x3.SetInt(0)
		y3.SetInt(0)
		z3.SetInt(0)
		return
	}

	// Calculate X3, Y3, and Z3 according to the intermediate elements
	// breakdown above.
	var a, b, c, d, e, f FieldVal
	var negX1, negY1, negE, negX3 FieldVal
	negX1.Set(x1).Negate(1)                // negX1 = -X1 (mag: 2)
	negY1.Set(y1).Negate(1)                // negY1 = -Y1 (mag: 2)
	a.Set(&negX1).Add(x2)                  // A = X2-X1 (mag: 3)
	b.SquareVal(&a)                        // B = A^2 (mag: 1)
	c.Set(&negY1).Add(y2)                  // C = Y2-Y1 (mag: 3)
	d.SquareVal(&c)                        // D = C^2 (mag: 1)
	e.Mul2(x1, &b)                         // E = X1*B (mag: 1)
	negE.Set(&e).Negate(1)                 // negE = -E (mag: 2)
	f.Mul2(x2, &b)                         // F = X2*B (mag: 1)
	x3.Add2(&e, &f).Negate(2).Add(&d)      // X3 = D-E-F (mag: 4)
	negX3.Set(x3).Negate(4)                // negX3 = -X3 (mag: 5)
	y3.Set(y1).Mul(f.Add(&negE)).Negate(1) // Y3 = -(Y1*(F-E)) (mag: 2)
	y3.Add(e.Add(&negX3).Mul(&c))          // Y3 = C*(E-X3)+Y3 (mag: 3)
	z3.Mul2(z1, &a)                        // Z3 = Z1*A (mag: 1)

	// Normalize the resulting field values as needed.
	x3.Normalize()
	y3.Normalize()
	z3.Normalize()
}

// addZ2EqualsOne adds two Jacobian points when the second point is already
// known to have a z value of 1 (and the z value for the first point is not 1)
// and stores the result in the provided result param.  That is to say result =
// p1 + p2.  It performs faster addition than the generic add routine since
// less arithmetic is needed due to the ability to avoid multiplications by the
// second point's z value.
//
// NOTE: The points must be normalized for this function to return the correct
// result.  The resulting point will be normalized.
func addZ2EqualsOne(p1, p2, result *JacobianPoint) {
	// To compute the point addition efficiently, this implementation splits
	// the equation into intermediate elements which are used to minimize
	// the number of field multiplications using the method shown at:
	// https://hyperelliptic.org/EFD/g1p/auto-shortw-jacobian-0.html#addition-madd-2007-bl
	//
	// In particular it performs the calculations using the following:
	// Z1Z1 = Z1^2, U2 = X2*Z1Z1, S2 = Y2*Z1*Z1Z1, H = U2-X1, HH = H^2,
	// I = 4*HH, J = H*I, r = 2*(S2-Y1), V = X1*I
	// X3 = r^2-J-2*V, Y3 = r*(V-X3)-2*Y1*J, Z3 = (Z1+H)^2-Z1Z1-HH
	//
	// This results in a cost of 7 field multiplications, 4 field squarings,
	// 9 field additions, and 4 integer multiplications.
	x1, y1, z1 := &p1.X, &p1.Y, &p1.Z
	x2, y2 := &p2.X, &p2.Y
	x3, y3, z3 := &result.X, &result.Y, &result.Z

	// When the x coordinates are the same for two points on the curve, the
	// y coordinates either must be the same, in which case it is point
	// doubling, or they are opposite and the result is the point at
	// infinity per the group law for elliptic curve cryptography.  Since
	// any number of Jacobian coordinates can represent the same affine
	// point, the x and y values need to be converted to like terms.  Due to
	// the assumption made for this function that the second point has a z
	// value of 1 (z2=1), the first point is already "converted".
	var z1z1, u2, s2 FieldVal
	z1z1.SquareVal(z1)                        // Z1Z1 = Z1^2 (mag: 1)
	u2.Set(x2).Mul(&z1z1).Normalize()         // U2 = X2*Z1Z1 (mag: 1)
	s2.Set(y2).Mul(&z1z1).Mul(z1).Normalize() // S2 = Y2*Z1*Z1Z1 (mag: 1)
	if x1.Equals(&u2) {
		if y1.Equals(&s2) {
			// Since x1 == x2 and y1 == y2, point doubling must be
			// done, otherwise the addition would end up dividing
			// by zero.
			DoubleNonConst(p1, result)
			return
		}

		// Since x1 == x2 and y1 == -y2, the sum is the point at
		// infinity per the group law.
		x3.SetInt(0)
		y3.SetInt(0)
		z3.SetInt(0)
		return
	}

	// Calculate X3, Y3, and Z3 according to the intermediate elements
	// breakdown above.
	var h, hh, i, j, r, rr, v FieldVal
	var negX1, negY1, negX3 FieldVal
	negX1.Set(x1).Negate(1)               // negX1 = -X1 (mag: 2)
	h.Add2(&u2, &negX1)                   // H = U2-X1 (mag: 3)
	hh.SquareVal(&h)                      // HH = H^2 (mag: 1)
	i.Set(&hh).MulBy4()                   // I = 4 * HH (mag: 4)
	j.Mul2(&h, &i)                        // J = H*I (mag: 1)
	negY1.Set(y1).Negate(1)               // negY1 = -Y1 (mag: 2)
	r.Set(&s2).Add(&negY1).MulBy2()       // r = 2*(S2-Y1) (mag: 6)
	rr.SquareVal(&r)                      // rr = r^2 (mag: 1)
	v.Mul2(x1, &i)                        // V = X1*I (mag: 1)
	x3.Set(&v).MulBy2().Add(&j).Negate(3) // X3 = -(J+2*V) (mag: 4)
	x3.Add(&rr)                           // X3 = r^2+X3 (mag: 5)
	negX3.Set(x3).Negate(5)               // negX3 = -X3 (mag: 6)
	y3.Set(y1).Mul(&j).MulBy2().Negate(2) // Y3 = -(2*Y1*J) (mag: 3)
	y3.Add(v.Add(&negX3).Mul(&r))         // Y3 = r*(V-X3)+Y3 (mag: 4)
	z3.Add2(z1, &h).Square()              // Z3 = (Z1+H)^2 (mag: 1)
	z3.Add(z1z1.Add(&hh).Negate(2))       // Z3 = Z3-(Z1Z1+HH) (mag: 4)

	// Normalize the resulting field values as needed.
	x3.Normalize()
	y3.Normalize()
	z3.Normalize()
}

// addGeneric adds two Jacobian points without any assumptions about the z
// values of the two points and stores the result in the provided result param.
// That is to say result = p1 + p2.  It is the slowest of the add routines due
// to requiring the most arithmetic.
//
// NOTE: The points must be normalized for this function to return the correct
// result.  The resulting point will be normalized.
func addGeneric(p1, p2, result *JacobianPoint) {
	// To compute the point addition efficiently, this implementation splits
	// the equation into intermediate elements which are used to minimize
	// the number of field multiplications using the method shown at:
	// https://hyperelliptic.org/EFD/g1p/auto-shortw-jacobian-0.html#addition-add-2007-bl
	//
	// In particular it performs the calculations using the following:
	// Z1Z1 = Z1^2, Z2Z2 = Z2^2, U1 = X1*Z2Z2, U2 = X2*Z1Z1, S1 = Y1*Z2*Z2Z2
	// S2 = Y2*Z1*Z1Z1, H = U2-U1, I = (2*H)^2, J = H*I, r = 2*(S2-S1)
	// V = U1*I
	// X3 = r^2-J-2*V, Y3 = r*(V-X3)-2*S1*J, Z3 = ((Z1+Z2)^2-Z1Z1-Z2Z2)*H
	//
	// This results in a cost of 11 field multiplications, 5 field squarings,
	// 9 field additions, and 4 integer multiplications.
	x1, y1, z1 := &p1.X, &p1.Y, &p1.Z
	x2, y2, z2 := &p2.X, &p2.Y, &p2.Z
	x3, y3, z3 := &result.X, &result.Y, &result.Z

	// When the x coordinates are the same for two points on the curve, the
	// y coordinates either must be the same, in which case it is point
	// doubling, or they are opposite and the result is the point at
	// infinity.  Since any number of Jacobian coordinates can represent the
	// same affine point, the x and y values need to be converted to like
	// terms.
	var z1z1, z2z2, u1, u2, s1, s2 FieldVal
	z1z1.SquareVal(z1)                        // Z1Z1 = Z1^2 (mag: 1)
	z2z2.SquareVal(z2)                        // Z2Z2 = Z2^2 (mag: 1)
	u1.Set(x1).Mul(&z2z2).Normalize()         // U1 = X1*Z2Z2 (mag: 1)
	u2.Set(x2).Mul(&z1z1).Normalize()         // U2 = X2*Z1Z1 (mag: 1)
	s1.Set(y1).Mul(&z2z2).Mul(z2).Normalize() // S1 = Y1*Z2*Z2Z2 (mag: 1)
	s2.Set(y2).Mul(&z1z1).Mul(z1).Normalize() // S2 = Y2*Z1*Z1Z1 (mag: 1)
	if u1.Equals(&u2) {
		if s1.Equals(&s2) {
			// Since x1 == x2 and y1 == y2, point doubling must be
			// done, otherwise the addition would end up dividing
			// by zero.
			DoubleNonConst(p1, result)
			return
		}

		// Since x1 == x2 and y1 == -y2, the sum is the point at
		// infinity per the group law.
		x3.SetInt(0)
		y3.SetInt(0)
		z3.SetInt(0)
		return
	}

	// Calculate X3, Y3, and Z3 according to the intermediate elements
	// breakdown above.
	var h, i, j, r, rr, v FieldVal
	var negU1, negS1, negX3 FieldVal
	negU1.Set(&u1).Negate(1)              // negU1 = -U1 (mag: 2)
	h.Add2(&u2, &negU1)                   // H = U2-U1 (mag: 3)
	i.Set(&h).MulBy2().Square()           // I = (2*H)^2 (mag: 1)
	j.Mul2(&h, &i)                        // J = H*I (mag: 1)
	negS1.Set(&s1).Negate(1)              // negS1 = -S1 (mag: 2)
	r.Set(&s2).Add(&negS1).MulBy2()       // r = 2*(S2-S1) (mag: 6)
	rr.SquareVal(&r)                      // rr = r^2 (mag: 1)
	v.Mul2(&u1, &i)                       // V = U1*I (mag: 1)
	x3.Set(&v).MulBy2().Add(&j).Negate(3) // X3 = -(J+2*V) (mag: 4)
	x3.Add(&rr)                           // X3 = r^2+X3 (mag: 5)
	negX3.Set(x3).Negate(5)               // negX3 = -X3 (mag: 6)
	y3.Mul2(&s1, &j).MulBy2().Negate(2)   // Y3 = -(2*S1*J) (mag: 3)
	y3.Add(v.Add(&negX3).Mul(&r))         // Y3 = r*(V-X3)+Y3 (mag: 4)
	z3.Add2(z1, z2).Square()              // Z3 = (Z1+Z2)^2 (mag: 1)
	z3.Add(z1z1.Add(&z2z2).Negate(2))     // Z3 = Z3-(Z1Z1+Z2Z2) (mag: 4)
	z3.Mul(&h)                            // Z3 = Z3*H (mag: 1)

	// Normalize the resulting field values as needed.
	x3.Normalize()
	y3.Normalize()
	z3.Normalize()
}

// AddNonConst adds the passed Jacobian points together and stores the result in
// the provided result param in *non-constant* time.
//
// NOTE: The points must be normalized for this function to return the correct
// result.  The resulting point will be normalized.
func AddNonConst(p1, p2, result *JacobianPoint) {
	// The point at infinity is the identity according to the group law for
	// elliptic curve cryptography.  Thus, ∞ + P = P and P + ∞ = P.
	if (p1.X.IsZero() && p1.Y.IsZero()) || p1.Z.IsZero() {
		result.Set(p2)
		return
	}
	if (p2.X.IsZero() && p2.Y.IsZero()) || p2.Z.IsZero() {
		result.Set(p1)
		return
	}

	// Faster point addition can be achieved when certain assumptions are
	// met.  For example, when both points have the same z value, arithmetic
	// on the z values can be avoided.  This section thus checks for these
	// conditions and calls an appropriate add function which is accelerated
	// by using those assumptions.
	isZ1One := p1.Z.IsOne()
	isZ2One := p2.Z.IsOne()
	switch {
	case isZ1One && isZ2One:
		addZ1AndZ2EqualsOne(p1, p2, result)
		return
	case p1.Z.Equals(&p2.Z):
		addZ1EqualsZ2(p1, p2, result)
		return
	case isZ2One:
		addZ2EqualsOne(p1, p2, result)
		return
	}

	// None of the above assumptions are true, so fall back to generic
	// point addition.
	addGeneric(p1, p2, result)
}

// doubleZ1EqualsOne performs point doubling on the passed Jacobian point when
// the point is already known to have a z value of 1 and stores the result in
// the provided result param.  That is to say result = 2*p.  It performs faster
// point doubling than the generic routine since less arithmetic is needed due
// to the ability to avoid multiplication by the z value.
//
// NOTE: The resulting point will be normalized.
func doubleZ1EqualsOne(p, result *JacobianPoint) {
	// This function uses the assumptions that z1 is 1, thus the point
	// doubling formulas reduce to:
	//
	// X3 = (3*X1^2)^2 - 8*X1*Y1^2
	// Y3 = (3*X1^2)*(4*X1*Y1^2 - X3) - 8*Y1^4
	// Z3 = 2*Y1
	//
	// To compute the above efficiently, this implementation splits the
	// equation into intermediate elements which are used to minimize the
	// number of field multiplications in favor of field squarings which
	// are roughly 35% faster than field multiplications with the current
	// implementation at the time this was written.
	//
	// This uses a slightly modified version of the method shown at:
	// https://hyperelliptic.org/EFD/g1p/auto-shortw-jacobian-0.html#doubling-mdbl-2007-bl
	//
	// In particular it performs the calculations using the following:
	// A = X1^2, B = Y1^2, C = B^2, D = 2*((X1+B)^2-A-C)
	// E = 3*A, F = E^2, X3 = F-2*D, Y3 = E*(D-X3)-8*C
	// Z3 = 2*Y1
	//
	// This results in a cost of 1 field multiplication, 5 field squarings,
	// 6 field additions, and 5 integer multiplications.
	x1, y1 := &p.X, &p.Y
	x3, y3, z3 := &result.X, &result.Y, &result.Z
	var a, b, c, d, e, f FieldVal
	z3.Set(y1).MulBy2()                      // Z3 = 2*Y1 (mag: 2)
	a.SquareVal(x1)                          // A = X1^2 (mag: 1)
	b.SquareVal(y1)                          // B = Y1^2 (mag: 1)
	c.SquareVal(&b)                          // C = B^2 (mag: 1)
	b.Add(x1).Square()                       // B = (X1+B)^2 (mag: 1)
	d.Set(&a).Add(&c).Negate(2)              // D = -(A+C) (mag: 3)
	d.Add(&b).MulBy2()                       // D = 2*(B+D)(mag: 8)
	e.Set(&a).MulBy3()                       // E = 3*A (mag: 3)
	f.SquareVal(&e)                          // F = E^2 (mag: 1)
	x3.Set(&d).MulBy2().Negate(16)           // X3 = -(2*D) (mag: 17)
	x3.Add(&f)                               // X3 = F+X3 (mag: 18)
	f.Set(x3).Negate(18).Add(&d).Normalize() // F = D-X3 (mag: 1)
	y3.Set(&c).MulBy8().Negate(8)            // Y3 = -(8*C) (mag: 9)
	y3.Add(f.Mul(&e))                        // Y3 = E*F+Y3 (mag: 10)

	// Normalize the resulting field values as needed.
	x3.Normalize()
	y3.Normalize()
	z3.Normalize()
}

// doubleGeneric performs point doubling on the passed Jacobian point without
// any assumptions about the z value and stores the result in the provided
// result param.  That is to say result = 2*p.  It is the slowest of the point
// doubling routines due to requiring the most arithmetic.
//
// NOTE: The resulting point will be normalized.
func doubleGeneric(p, result *JacobianPoint) {
	// Point doubling formula for Jacobian coordinates for the secp256k1
	// curve:
	//
	// X3 = (3*X1^2)^2 - 8*X1*Y1^2
	// Y3 = (3*X1^2)*(4*X1*Y1^2 - X3) - 8*Y1^4
	// Z3 = 2*Y1*Z1
	//
	// To compute the above efficiently, this implementation splits the
	// equation into intermediate elements which are used to minimize the
	// number of field multiplications in favor of field squarings which
	// are roughly 35% faster than field multiplications with the current
	// implementation at the time this was written.
	//
	// This uses a slightly modified version of the method shown at:
	// https://hyperelliptic.org/EFD/g1p/auto-shortw-jacobian-0.html#doubling-dbl-2009-l
	//
	// In particular it performs the calculations using the following:
	// A = X1^2, B = Y1^2, C = B^2, D = 2*((X1+B)^2-A-C)
	// E = 3*A, F = E^2, X3 = F-2*D, Y3 = E*(D-X3)-8*C
	// Z3 = 2*Y1*Z1
	//
	// This results in a cost of 1 field multiplication, 5 field squarings,
	// 6 field additions, and 5 integer multiplications.
	x1, y1, z1 := &p.X, &p.Y, &p.Z
	x3, y3, z3 := &result.X, &result.Y, &result.Z
	var a, b, c, d, e, f FieldVal
	z3.Mul2(y1, z1).MulBy2()                 // Z3 = 2*Y1*Z1 (mag: 2)
	a.SquareVal(x1)                          // A = X1^2 (mag: 1)
	b.SquareVal(y1)                          // B = Y1^2 (mag: 1)
	c.SquareVal(&b)                          // C = B^2 (mag: 1)
	b.Add(x1).Square()                       // B = (X1+B)^2 (mag: 1)
	d.Set(&a).Add(&c).Negate(2)              // D = -(A+C) (mag: 3)
	d.Add(&b).MulBy2()                       // D = 2*(B+D)(mag: 8)
	e.Set(&a).MulBy3()                       // E = 3*A (mag: 3)
	f.SquareVal(&e)                          // F = E^2 (mag: 1)
	x3.Set(&d).MulBy2().Negate(16)           // X3 = -(2*D) (mag: 17)
	x3.Add(&f)                               // X3 = F+X3 (mag: 18)
	f.Set(x3).Negate(18).Add(&d).Normalize() // F = D-X3 (mag: 1)
	y3.Set(&c).MulBy8().Negate(8)            // Y3 = -(8*C) (mag: 9)
	y3.Add(f.Mul(&e))                        // Y3 = E*F+Y3 (mag: 10)

	// Normalize the resulting field values as needed.
	x3.Normalize()
	y3.Normalize()
	z3.Normalize()
}

// DoubleNonConst doubles the passed Jacobian point and stores the result in the
// provided result parameter in *non-constant* time.
//
// NOTE: The point must be normalized for this function to return the correct
// result.  The resulting point will be normalized.
func DoubleNonConst(p, result *JacobianPoint) {
	// Doubling the point at infinity is still infinity.
	if p.Y.IsZero() || p.Z.IsZero() {
		result.X.SetInt(0)
		result.Y.SetInt(0)
		result.Z.SetInt(0)
		return
	}

	// Slightly faster point doubling can be achieved when the z value is 1
	// by avoiding the multiplication on the z value.  This section calls
	// a point doubling function which is accelerated by using that
	// assumption when possible.
	if p.Z.IsOne() {
		doubleZ1EqualsOne(p, result)
		return
	}

	// Fall back to generic point doubling which works with arbitrary z
	// values.
	doubleGeneric(p, result)
}

// mulAdd64 multiplies the two passed base 2^64 digits together, adds the given
// value to the result, and returns the 128-bit result via a (hi, lo) tuple
// where the upper half of the bits are returned in hi and the lower half in lo.
func mulAdd64(digit1, digit2, m uint64) (hi, lo uint64) {
	// Note the carry on the final add is safe to discard because the maximum
	// possible value is:
	//   (2^64 - 1)(2^64 - 1) + (2^64 - 1) = 2^128 - 2^64
	// and:
	//   2^128 - 2^64 < 2^128.
	var c uint64
	hi, lo = bits.Mul64(digit1, digit2)
	lo, c = bits.Add64(lo, m, 0)
	hi, _ = bits.Add64(hi, 0, c)
	return hi, lo
}

// mulAdd64Carry multiplies the two passed base 2^64 digits together, adds both
// the given value and carry to the result, and returns the 128-bit result via a
// (hi, lo) tuple where the upper half of the bits are returned in hi and the
// lower half in lo.
func mulAdd64Carry(digit1, digit2, m, c uint64) (hi, lo uint64) {
	// Note the carry on the high order add is safe to discard because the
	// maximum possible value is:
	//   (2^64 - 1)(2^64 - 1) + 2*(2^64 - 1) = 2^128 - 1
	// and:
	//   2^128 - 1 < 2^128.
	var c2 uint64
	hi, lo = mulAdd64(digit1, digit2, m)
	lo, c2 = bits.Add64(lo, c, 0)
	hi, _ = bits.Add64(hi, 0, c2)
	return hi, lo
}

// mul512Rsh320Round computes the full 512-bit product of the two given scalars,
// right shifts the result by 320 bits, rounds to the nearest integer, and
// returns the result in constant time.
//
// Note that despite the inputs and output being mod n scalars, the 512-bit
// product is NOT reduced mod N prior to the right shift.  This is intentional
// because it is used for replacing division with multiplication and thus the
// intermediate results must be done via a field extension to a larger field.
func mul512Rsh320Round(n1, n2 *ModNScalar) ModNScalar {
	// Compute the full 512-bit product n1*n2.
	var r0, r1, r2, r3, r4, r5, r6, r7, c uint64

	// Terms resulting from the product of the first digit of the second number
	// by all digits of the first number.
	//
	// Note that r0 is ignored because it is not needed to compute the higher
	// terms and it is shifted out below anyway.
	c, _ = bits.Mul64(n2.n[0], n1.n[0])
	c, r1 = mulAdd64(n2.n[0], n1.n[1], c)
	c, r2 = mulAdd64(n2.n[0], n1.n[2], c)
	r4, r3 = mulAdd64(n2.n[0], n1.n[3], c)

	// Terms resulting from the product of the second digit of the second number
	// by all digits of the first number.
	//
	// Note that r1 is ignored because it is no longer needed to compute the
	// higher terms and it is shifted out below anyway.
	c, _ = mulAdd64(n2.n[1], n1.n[0], r1)
	c, r2 = mulAdd64Carry(n2.n[1], n1.n[1], r2, c)
	c, r3 = mulAdd64Carry(n2.n[1], n1.n[2], r3, c)
	r5, r4 = mulAdd64Carry(n2.n[1], n1.n[3], r4, c)

	// Terms resulting from the product of the third digit of the second number
	// by all digits of the first number.
	//
	// Note that r2 is ignored because it is no longer needed to compute the
	// higher terms and it is shifted out below anyway.
	c, _ = mulAdd64(n2.n[2], n1.n[0], r2)
	c, r3 = mulAdd64Carry(n2.n[2], n1.n[1], r3, c)
	c, r4 = mulAdd64Carry(n2.n[2], n1.n[2], r4, c)
	r6, r5 = mulAdd64Carry(n2.n[2], n1.n[3], r5, c)

	// Terms resulting from the product of the fourth digit of the second number
	// by all digits of the first number.
	//
	// Note that r3 is ignored because it is no longer needed to compute the
	// higher terms and it is shifted out below anyway.
	c, _ = mulAdd64(n2.n[3], n1.n[0], r3)
	c, r4 = mulAdd64Carry(n2.n[3], n1.n[1], r4, c)
	c, r5 = mulAdd64Carry(n2.n[3], n1.n[2], r5, c)
	r7, r6 = mulAdd64Carry(n2.n[3], n1.n[3], r6, c)

	// At this point the upper 256 bits of the full 512-bit product n1*n2 are in
	// r4..r7 (recall the low order results were discarded as noted above).
	//
	// Right shift the result 320 bits.  Note that the MSB of r4 determines
	// whether or not to round because it is the final bit that is shifted out.
	//
	// Also, notice that r3..r7 would also ordinarily be set to 0 as well for
	// the full shift, but that is skipped since they are no longer used as
	// their values are known to be zero.
	roundBit := r4 >> 63
	r2, r1, r0 = r7, r6, r5

	// Conditionally add 1 depending on the round bit in constant time.
	r0, c = bits.Add64(r0, roundBit, 0)
	r1, c = bits.Add64(r1, 0, c)
	r2, r3 = bits.Add64(r2, 0, c)

	// Finally, convert the result to a mod n scalar.
	//
	// No modular reduction is needed because the result is guaranteed to be
	// less than the group order given the group order is > 2^255 and the
	// maximum possible value of the result is 2^192.
	var result ModNScalar
	result.n[0] = r0
	result.n[1] = r1
	result.n[2] = r2
	result.n[3] = r3
	return result
}

// splitK returns two scalars (k1 and k2) that are a balanced length-two
// representation of the provided scalar such that k ≡ k1 + k2*λ (mod N), where
// N is the secp256k1 group order.
func splitK(k *ModNScalar) (ModNScalar, ModNScalar) {
	// The ultimate goal is to decompose k into two scalars that are around
	// half the bit length of k such that the following equation is satisfied:
	//
	// k1 + k2*λ ≡ k (mod n)
	//
	// The strategy used here is based on algorithm 3.74 from [GECC] with a few
	// modifications to make use of the more efficient mod n scalar type, avoid
	// some costly long divisions, and minimize the number of calculations.
	//
	// Start by defining a function that takes a vector v = <a,b> ∈ ℤ⨯ℤ:
	//
	// f(v) = a + bλ (mod n)
	//
	// Then, find two vectors, v1 = <a1,b1>, and v2 = <a2,b2> in ℤ⨯ℤ such that:
	// 1) v1 and v2 are linearly independent
	// 2) f(v1) = f(v2) = 0
	// 3) v1 and v2 have small Euclidean norm
	//
	// The vectors that satisfy these properties are found via the Euclidean
	// algorithm and are precomputed since both n and λ are fixed values for the
	// secp256k1 curve.  See genprecomps.go for derivation details.
	//
	// Next, consider k as a vector <k, 0> in ℚ⨯ℚ and by linear algebra write:
	//
	// <k, 0> = g1*v1 + g2*v2, where g1, g2 ∈ ℚ
	//
	// Note that, per above, the components of vector v1 are a1 and b1 while the
	// components of vector v2 are a2 and b2.  Given the vectors v1 and v2 were
	// generated such that a1*b2 - a2*b1 = n, solving the equation for g1 and g2
	// yields:
	//
	// g1 = b2*k / n
	// g2 = -b1*k / n
	//
	// Observe:
	// <k, 0> = g1*v1 + g2*v2
	//        = (b2*k/n)*<a1,b1> + (-b1*k/n)*<a2,b2>              | substitute
	//        = <a1*b2*k/n, b1*b2*k/n> + <-a2*b1*k/n, -b2*b1*k/n> | scalar mul
	//        = <a1*b2*k/n - a2*b1*k/n, b1*b2*k/n - b2*b1*k/n>    | vector add
	//        = <[a1*b2*k - a2*b1*k]/n, 0>                        | simplify
	//        = <k*[a1*b2 - a2*b1]/n, 0>                          | factor out k
	//        = <k*n/n, 0>                                        | substitute
	//        = <k, 0>                                            | simplify
	//
	// Now, consider an integer-valued vector v:
	//
	// v = c1*v1 + c2*v2, where c1, c2 ∈ ℤ (mod n)
	//
	// Since vectors v1 and v2 are linearly independent and were generated such
	// that f(v1) = f(v2) = 0, all possible scalars c1 and c2 also produce a
	// vector v such that f(v) = 0.
	//
	// In other words, c1 and c2 can be any integers and the resulting
	// decomposition will still satisfy the required equation.  However, since
	// the goal is to produce a balanced decomposition that provides a
	// performance advantage by minimizing max(k1, k2), c1 and c2 need to be
	// integers close to g1 and g2, respectively, so the resulting vector v is
	// an integer-valued vector that is close to <k, 0>.
	//
	// Finally, consider the vector u:
	//
	// u = <k, 0> - v
	//
	// It follows that f(u) = k and thus the two components of vector u satisfy
	// the required equation:
	//
	// k1 + k2*λ ≡ k (mod n)
	//
	// Choosing c1 and c2:
	// -------------------
	//
	// As mentioned above, c1 and c2 need to be integers close to g1 and g2,
	// respectively.  The algorithm in [GECC] chooses the following values:
	//
	// c1 = round(g1) = round(b2*k / n)
	// c2 = round(g2) = round(-b1*k / n)
	//
	// However, as section 3.4.2 of [STWS] notes, the aforementioned approach
	// requires costly long divisions that can be avoided by precomputing
	// rounded estimates as follows:
	//
	// t = bitlen(n) + 1
	// z1 = round(2^t * b2 / n)
	// z2 = round(2^t * -b1 / n)
	//
	// Then, use those precomputed estimates to perform a multiplication by k
	// along with a floored division by 2^t, which is a simple right shift by t:
	//
	// c1 = floor(k * z1 / 2^t) = (k * z1) >> t
	// c2 = floor(k * z2 / 2^t) = (k * z2) >> t
	//
	// Finally, round up if last bit discarded in the right shift by t is set by
	// adding 1.
	//
	// As a further optimization, rather than setting t = bitlen(n) + 1 = 257 as
	// stated by [STWS], this implementation uses a higher precision estimate of
	// t = bitlen(n) + 64 = 320 because it allows simplification of the shifts
	// in the internal calculations that are done via uint64s and also allows
	// the use of floor in the precomputations.
	//
	// Thus, the calculations this implementation uses are:
	//
	// z1 = floor(b2<<320 / n)                                     | precomputed
	// z2 = floor((-b1)<<320) / n)                                 | precomputed
	// c1 = ((k * z1) >> 320) + (((k * z1) >> 319) & 1)
	// c2 = ((k * z2) >> 320) + (((k * z2) >> 319) & 1)
	//
	// Putting it all together:
	// ------------------------
	//
	// Calculate the following vectors using the values discussed above:
	//
	// v = c1*v1 + c2*v2
	// u = <k, 0> - v
	//
	// The two components of the resulting vector v are:
	// va = c1*a1 + c2*a2
	// vb = c1*b1 + c2*b2
	//
	// Thus, the two components of the resulting vector u are:
	// k1 = k - va
	// k2 = 0 - vb = -vb
	//
	// As some final optimizations:
	//
	// 1) Note that k1 + k2*λ ≡ k (mod n) means that k1 ≡ k - k2*λ (mod n).
	//    Therefore, the computation of va can be avoided to save two
	//    field multiplications and a field addition.
	//
	// 2) Since k1 ≡ k - k2*λ ≡ k + k2*(-λ), an additional field negation is
	//    saved by storing and using the negative version of λ.
	//
	// 3) Since k2 ≡ -vb ≡ -(c1*b1 + c2*b2) ≡ c1*(-b1) + c2*(-b2), one more
	//    field negation is saved by storing and using the negative versions of
	//    b1 and b2.
	//
	// k2 = c1*(-b1) + c2*(-b2)
	// k1 = k + k2*(-λ)
	var k1, k2 ModNScalar
	c1 := mul512Rsh320Round(k, endoZ1)
	c2 := mul512Rsh320Round(k, endoZ2)
	k2.Add2(c1.Mul(endoNegB1), c2.Mul(endoNegB2))
	k1.Mul2(&k2, endoNegLambda).Add(k)
	return k1, k2
}

// wNAFWidth is the window width of the NAF representation used in scalar
// multiplication.
//
// It is important to note that this constant primarily exists to improve code
// readability, to parameterize parts of the scalar multiplication
// implementation, and to allow tests to assert invariants.
//
// However, the overall implementation is intentionally not fully parameterized
// based on this constant because it is carefully optimized for this specific
// window width and those optimizations involve taking advantage of known bounds
// that an arbitrary window size would violate.
//
// Choosing a new window size involves a variety of tradeoffs that must be
// carefully analyzed.  For example, recoding costs, precomputation costs, and
// average density.
const wNAFWidth = 5

// wnaf5RecodeCodes maps the low five bits of an integer to the encoded
// representative of the width-5 wNAF digit that should be emitted.
//
// These values are not the signed digits themselves.  Rather, each is an
// encoded representative that serves directly as an index into precomputed
// tables.  See [wnafScalar] for the encoding details.
//
// For each odd residue, the represented signed digit is the unique value in
// {±1, ±3, ±5, ±7, ±9, ±11, ±13, ±15} that is congruent to the residue modulo
// 32:
//
//	residue ≡ digit (mod 32)
//
// Thus, subtracting the represented signed digit always produces an integer
// that is divisible by 32.  This allows the optimized recoding algorithm to
// skip five one-bit iterations of the canonical algorithm at once.
//
// Even residues always map to zero because they are already divisible by two
// and therefore correspond only to implicit zero digits.
//
// The corresponding arithmetic adjustment for each encoded representative is
// provided by [wnaf5RecodeAdjustments].
var wnaf5RecodeCodes = [32]uint8{
	0, 1, 0, 2, 0, 3, 0, 4, 0, 5, 0, 6, 0, 7, 0, 8,
	0, 16, 0, 15, 0, 14, 0, 13, 0, 12, 0, 11, 0, 10, 0, 9,
}

// wnaf5RecodeAdjustments maps the low five bits of an integer to the signed
// adjustment that must be added to the working integer before shifting during
// the width-5 wNAF recoding algorithm.
//
// While [wnaf5RecodeCodes] produces the encoded representative that is stored
// in the resulting wNAF encoding, this table provides the corresponding
// arithmetic adjustment.  The represented signed digit d satisfies:
//
//	residue ≡ d (mod 32)
//
// and the recoding algorithm updates the working integer via:
//
//	n = (n - d) / 2
//
// Rather than subtracting d directly, this table stores -d encoded as an
// unsigned 64-bit two's complement value.  Negative adjustments therefore
// appear as values such as:
//
//	-1 -> ^uint64(0)
//	-3 -> ^uint64(2)
//	-5 -> ^uint64(4)
//	-7 -> ^uint64(6)
//	-9 -> ^uint64(8)
//	-11 -> ^uint64(10)
//	-13 -> ^uint64(12)
//	-15 -> ^uint64(14)
//
// The adjustment is sign-extended across the upper limbs during the multiword
// addition which allows the hot loop to remain branch free.
var wnaf5RecodeAdjustments = [32]uint64{
	0, ^uint64(0), 0, ^uint64(2), 0, ^uint64(4), 0, ^uint64(6),
	0, ^uint64(8), 0, ^uint64(10), 0, ^uint64(12), 0, ^uint64(14),
	0, 15, 0, 13, 0, 11, 0, 9, 0, 7, 0, 5, 0, 3, 0, 1,
}

// wnafScalar represents a non-negative integer less than 2^129 encoded in
// width-5 windowed non-adjacent form (wNAF).
//
// wNAF is a signed-digit representation where the valid digits are 0, ±1, ±3,
// ±5, ±7, ±9, ±11, ±13, and ±15.
//
// Each entry corresponds to one bit position and encodes the signed digit.
//
// The encoding from code to digit is:
// 0 -> 0
// 1 -> +1
// 2 -> +3
// 3 -> +5
// 4 -> +7
// 5 -> +9
// 6 -> +11
// 7 -> +13
// 8 -> +15
// 9 -> -1
// 10 -> -3
// 11 -> -5
// 12 -> -7
// 13 -> -9
// 14 -> -11
// 15 -> -13
// 16 -> -15
//
// The encoding is intentionally not using a more compact form so that it serves
// directly as an index into precomputed tables which allows simplification of
// hot paths.
//
// Formally, letting c denote the stored code value, the decoded signed digit
// d(c) is given by the piecewise function:
//
//	       {0,      where c = 0
//	d(c) = {2c - 1, where 0 < c ≤ 8
//	       {17 - 2c, where 8 < c ≤ 16
//
// The array contains one extra entry because a recoding may produce a carry
// into an additional most-significant digit.
type wnafScalar struct {
	codes [130]uint8
	bits  uint8
}

// wnaf takes a non-negative integer less than 2^129 and returns its width-5
// windowed non-adjacent form (wNAF) which is a unique signed-digit
// representation such that non-zero digits are separated by at least 4 zeroes.
// See [wnafScalar] for details on how the representation is encoded and how to
// interpret it.
//
// Width-5 wNAF is useful because, on average, only about one in every six
// digits is non-zero.
//
// This property is particularly beneficial for optimizing elliptic curve point
// multiplication because it greatly reduces the number of point additions at
// the cost of precomputing a few odd multiples of the point and, in the worst
// case, one additional point doubling due to a carry introduced by the
// recoding.  This is an excellent tradeoff because subtraction of points has
// the same computational complexity as addition of points and point doubling is
// faster than both.
func wnaf(k *ModNScalar) wnafScalar {
	const (
		// This is intentionally not using the package constant [wNAFWidth] for
		// the window size for the reasons stated by its documentation.
		//
		// In particular, this implementation is carefully optimized for this
		// specific width and involves assumptions that an arbitrary window size
		// might violate.
		//
		// Using a separate constant helps make it clear that updating the
		// window size here requires extra care to assert correctness.
		windowSize = 5
		windowMask = (1 << windowSize) - 1
	)

	// The arithmetic for wNAF recoding is performed over the ordinary integers,
	// not modulo the curve order.  Moreover, this method is required to be
	// called with the scalar's balanced integer representative, which is known
	// to fit within 129 bits.
	//
	// Consequently, the scalar is first converted to a uint192 with three
	// 64-bit limbs where the top limb is at most 1.
	t0, t1, t2 := k.n[0], k.n[1], k.n[2]

	// This implementation is based on the standard width-w NAF recoding
	// algorithm presented as Algorithm 3.35 in [GECC].  However, it has been
	// modified to skip runs of zero digits instead of processing one bit at a
	// time.
	//
	// The optimizations exploit the fact that runs of zero digits are implicit
	// in the output representation.
	//
	// Also, note that the last non-zero bit is initialized to the maximum value
	// so that adding 1 at the end to account for the exclusive endpoint wraps
	// around to 0 when no digits are emitted.
	var result wnafScalar
	var c uint64
	var bit uint8
	var lastNonZeroBit = ^uint8(0)
	for t0|t1|t2 != 0 {
		// The next five iterations of the canonical bit-by-bit algorithm would
		// emit only zero when the low window is zero, so skip directly to the
		// next window boundary in that case.
		if residue := uint8(t0 & windowMask); residue != 0 {
			// Skip the zero digits that the canonical bit-by-bit algorithm
			// would emit before reaching the next odd value.
			//
			// Since 0 < residue < 32, shift is guaranteed to satisfy the
			// following bounds:
			//
			//   0 ≤ shift < 5
			shift := uint8(bits.TrailingZeros8(residue))
			t0 = t0>>shift | t1<<(64-shift)
			t1 = t1>>shift | t2<<(64-shift)
			t2 >>= shift
			bit += shift

			residue = uint8(t0 & windowMask)
			result.codes[bit] = wnaf5RecodeCodes[residue]
			lastNonZeroBit = bit

			// The selected digit is congruent to the low five bits modulo 32.
			// Therefore, adding the stored adjustment (which represents the
			// negation of that digit) makes the value divisible by 32.
			//
			// Negative adjustments are stored in two's complement.  Sign
			// extending the value across the upper limbs allows the multiword
			// addition to operate without branches.
			adjustment := wnaf5RecodeAdjustments[residue]
			signExtended := uint64(int64(adjustment) >> 63)
			t0, c = bits.Add64(t0, adjustment, 0)
			t1, c = bits.Add64(t1, signExtended, c)
			t2, _ = bits.Add64(t2, signExtended, c)
		}

		// Divide by 32 to prepare for the next window.
		//
		// The adjustment above guarantees the current value is divisible by 32.
		t0 = t0>>windowSize | t1<<(64-windowSize)
		t1 = t1>>windowSize | t2<<(64-windowSize)
		t2 >>= windowSize
		bit += windowSize
	}

	result.bits = lastNonZeroBit + 1
	return result
}

// nafScalar represents a positive integer up to a maximum value of 2^256 - 1
// encoded in non-adjacent form.
//
// NAF is a signed-digit representation where each digit can be +1, 0, or -1.
//
// In order to efficiently encode that information, this type uses two arrays, a
// "positive" array where set bits represent the +1 signed digits and a
// "negative" array where set bits represent the -1 signed digits.  0 is
// represented by neither array having a bit set in that position.
//
// The Pos and Neg methods return the aforementioned positive and negative
// arrays, respectively.
type nafScalar struct {
	// pos houses the positive portion of the representation.  An additional
	// byte is required for the positive portion because the NAF encoding can be
	// up to 1 bit longer than the normal binary encoding of the value.
	//
	// neg houses the negative portion of the representation.  Even though the
	// additional byte is not required for the negative portion, since it can
	// never exceed the length of the normal binary encoding of the value,
	// keeping the same length for positive and negative portions simplifies
	// working with the representation and allows extra conditional branches to
	// be avoided.
	//
	// start and end specify the starting and ending index to use within the pos
	// and neg arrays, respectively.  This allows fixed size arrays to be used
	// versus needing to dynamically allocate space on the heap.
	//
	// NOTE: The fields are defined in the order that they are to minimize the
	// padding on 32-bit and 64-bit platforms.
	pos        [33]byte
	start, end uint8
	neg        [33]byte
}

// Pos returns the bytes of the encoded value with bits set in the positions
// that represent a signed digit of +1.
func (s *nafScalar) Pos() []byte {
	return s.pos[s.start:s.end]
}

// Neg returns the bytes of the encoded value with bits set in the positions
// that represent a signed digit of -1.
func (s *nafScalar) Neg() []byte {
	return s.neg[s.start:s.end]
}

// naf takes a positive integer up to a maximum value of 2^256 - 1 and returns
// its non-adjacent form (NAF), which is a unique signed-digit representation
// such that no two consecutive digits are nonzero.  See the documentation for
// the returned type for details on how the representation is encoded
// efficiently and how to interpret it
//
// NAF is useful in that it has the fewest nonzero digits of any signed digit
// representation, only 1/3rd of its digits are nonzero on average, and at least
// half of the digits will be 0.
//
// The aforementioned properties are particularly beneficial for optimizing
// elliptic curve point multiplication because they effectively minimize the
// number of required point additions in exchange for needing to perform a mix
// of fewer point additions and subtractions and possibly one additional point
// doubling.  This is an excellent tradeoff because subtraction of points has
// the same computational complexity as addition of points and point doubling is
// faster than both.
func naf(k []byte) nafScalar {
	// Strip leading zero bytes.
	for len(k) > 0 && k[0] == 0x00 {
		k = k[1:]
	}

	// The non-adjacent form (NAF) of a positive integer k is an expression
	// k = ∑_(i=0, l-1) k_i * 2^i where k_i ∈ {0,±1}, k_(l-1) != 0, and no two
	// consecutive digits k_i are nonzero.
	//
	// The traditional method of computing the NAF of a positive integer is
	// given by algorithm 3.30 in [GECC].  It consists of repeatedly dividing k
	// by 2 and choosing the remainder so that the quotient (k−r)/2 is even
	// which ensures the next NAF digit is 0.  This requires log_2(k) steps.
	//
	// However, in [BRID], Prodinger notes that a closed form expression for the
	// NAF representation is the bitwise difference 3k/2 - k/2.  This is more
	// efficient as it can be computed in O(1) versus the O(log(n)) of the
	// traditional approach.
	//
	// The following code makes use of that formula to compute the NAF more
	// efficiently.
	//
	// To understand the logic here, observe that the only way the NAF has a
	// nonzero digit at a given bit is when either 3k/2 or k/2 has a bit set in
	// that position, but not both.  In other words, the result of a bitwise
	// xor.  This can be seen simply by considering that when the bits are the
	// same, the subtraction is either 0-0 or 1-1, both of which are 0.
	//
	// Further, observe that the "+1" digits in the result are contributed by
	// 3k/2 while the "-1" digits are from k/2.  So, they can be determined by
	// taking the bitwise and of each respective value with the result of the
	// xor which identifies which bits are nonzero.
	//
	// Using that information, this loops backwards from the least significant
	// byte to the most significant byte while performing the aforementioned
	// calculations by propagating the potential carry and high order bit from
	// the next word during the right shift.
	kLen := len(k)
	var result nafScalar
	var carry uint8
	for byteNum := kLen - 1; byteNum >= 0; byteNum-- {
		// Calculate k/2.  Notice the carry from the previous word is added and
		// the low order bit from the next word is shifted in accordingly.
		kc := uint16(k[byteNum]) + uint16(carry)
		var nextWord uint8
		if byteNum > 0 {
			nextWord = k[byteNum-1]
		}
		halfK := kc>>1 | uint16(nextWord<<7)

		// Calculate 3k/2 and determine the non-zero digits in the result.
		threeHalfK := kc + halfK
		nonZeroResultDigits := threeHalfK ^ halfK

		// Determine the signed digits {0, ±1}.
		result.pos[byteNum+1] = uint8(threeHalfK & nonZeroResultDigits)
		result.neg[byteNum+1] = uint8(halfK & nonZeroResultDigits)

		// Propagate the potential carry from the 3k/2 calculation.
		carry = uint8(threeHalfK >> 8)
	}
	result.pos[0] = carry

	// Set the starting and ending positions within the fixed size arrays to
	// identify the bytes that are actually used.  This is important since the
	// encoding is big endian and thus trailing zero bytes changes its value.
	result.start = 1 - carry
	result.end = uint8(kLen + 1)
	return result
}

// wNAFPrecompTableSize is the size of the wNAF precomputed point table.
const wNAFPrecompTableSize = 1 << (wNAFWidth - 1)

// wNAFPrecompTable houses the precomputed odd point multiples used during wNAF
// scalar multiplication.
//
// The first half of the table (starting at index 1) contains the positive odd
// multiples:
//
//	P, 3P, 5P, ...
//
// While the second half of the table contains the corresponding negatives:
//
//	-P, -3P, -5P, ...
//
// Entry 0 is intentionally left unused so the encoded wNAF digit can be used
// directly as an array index.
type wNAFPrecompTable [wNAFPrecompTableSize + 1]JacobianPoint

// wNAFNegateStart returns the index of the half of a precomputed table whose
// points must be negated in order to match the sign adjustment made to the
// corresponding scalar.
//
// When the scalar has been negated, the first half is negated so it represents
// the negative odd multiples while the second half remains positive.
// Otherwise, the opposite arrangement is used.
func wNAFNegateStart(isScalarNegated bool) int {
	if isScalarNegated {
		return 1
	}
	return wNAFPrecompTableSize/2 + 1
}

// buildOddPrecomputeTables constructs the odd multiple tables for both P and
// φ(P) and arranges the positive and negative representatives so they can be
// indexed directly by encoded wNAF digits.
func buildOddPrecomputeTables(p *JacobianPoint, pTable, phiTable *wNAFPrecompTable, k1Neg, k2Neg bool) {
	// oddMultiplesPerHalf is the total number of odd multiples in each half of
	// the table.
	const oddMultiplesPerHalf = wNAFPrecompTableSize / 2

	// Build the positive odd multiples of P.
	//
	// Width-w wNAF only ever emits odd signed digits, so only odd multiples of
	// the point need to be precomputed.
	//
	// Both positive and negative entries are ultimately needed.  The second
	// half is initially a copy of the positive half and the appropriate half is
	// negated below depending on the sign of k1.
	//
	// The negation is deferred until after the φ(P) table is built because that
	// table is constructed by applying φ to the corresponding positive odd
	// multiples of P.
	var twoP JacobianPoint
	pTable[1].Set(p)
	DoubleNonConst(&pTable[1], &twoP)
	for i := 1; i < oddMultiplesPerHalf; i++ {
		AddNonConst(&twoP, &pTable[i], &pTable[i+1])
	}
	copy(pTable[oddMultiplesPerHalf+1:], pTable[1:oddMultiplesPerHalf+1])

	// Build the corresponding odd multiples of φ(P).
	//
	// As above, both positive and negative entries are ultimately needed.  The
	// sign of this table is chosen independently from the P table because it
	// depends only on the sign of k2.
	//
	// Note that φ is a group homomorphism, so:
	//
	//   φ(n*P) = n*φ(P)
	//
	// This allows the second table to be obtained by applying φ to each entry
	// of the first table instead of computing each odd multiple independently.
	//
	// NOTE: φ(x,y) = (βx,y).  The Jacobian z coordinates are the same, so this
	// math goes through.
	for i := 1; i <= oddMultiplesPerHalf; i++ {
		phiTable[i].Set(&pTable[i])
		phiTable[i].X.Mul(endoBeta)
	}
	copy(phiTable[oddMultiplesPerHalf+1:], phiTable[1:oddMultiplesPerHalf+1])

	// Negate whichever half of the table corresponds to the sign adjustment
	// that was applied to k1.
	//
	// This maintains the invariant:
	//
	//   k1*P = -k1*-P
	negateStart := wNAFNegateStart(k1Neg)
	for i := negateStart; i < negateStart+oddMultiplesPerHalf; i++ {
		pTable[i].Y.Negate(1).Normalize()
	}

	// Apply the same transformation independently to the φ(P) table based on
	// the sign adjustment that was applied to k2.
	//
	// Similarly, this maintains the invariant:
	//
	//   k2*φ(P) = -k2*-φ(P)
	negateStart = wNAFNegateStart(k2Neg)
	for i := negateStart; i < negateStart+oddMultiplesPerHalf; i++ {
		phiTable[i].Y.Negate(1).Normalize()
	}
}

// ScalarMultNonConst multiplies k*P where k is a scalar modulo the curve order
// and P is a point in Jacobian projective coordinates and stores the result in
// the provided Jacobian point.
//
// NOTE: The point must be normalized for this function to return the correct
// result.  The resulting point will be normalized.
func ScalarMultNonConst(k *ModNScalar, point, result *JacobianPoint) {
	// -------------------------------------------------------------------------
	// This makes use of the following efficiently-computable endomorphism to
	// accelerate the computation:
	//
	// φ(P) ⟼ λ*P = (β*P.x mod p, P.y)
	//
	// In other words, there is a special scalar λ that every point on the
	// elliptic curve can be multiplied by that will result in the same point as
	// performing a single field multiplication of the point's X coordinate by
	// the special value β.
	//
	// This is useful because scalar point multiplication is significantly more
	// expensive than a single field multiplication given the former involves a
	// series of point doublings and additions which themselves consist of a
	// combination of several field multiplications, squarings, and additions.
	//
	// So, the idea behind making use of the endomorphism is thus to decompose
	// the scalar into two scalars that are each about half the bit length of
	// the original scalar such that:
	//
	// k ≡ k1 + k2*λ (mod n)
	//
	// This in turn allows the scalar point multiplication to be performed as a
	// sum of two smaller half-length multiplications as follows:
	//
	// k*P = (k1 + k2*λ)*P
	//     = k1*P + k2*λ*P
	//     = k1*P + k2*φ(P)
	//
	// Thus, a speedup is achieved so long as it's faster to decompose the
	// scalar, compute φ(P), and perform a simultaneous multiply of the
	// half-length point multiplications than it is to compute a full width
	// point multiplication.
	//
	// In practice, benchmarks show the current implementation provides a
	// speedup of around 30-35% versus not using the endomorphism.
	//
	// See section 3.5 in [GECC] for a more rigorous treatment.
	// -------------------------------------------------------------------------

	// Decompose k into k1 and k2 such that k = k1 + k2*λ (mod n) where k1 and
	// k2 are around half the bit length of k in order to halve the number of EC
	// operations.
	//
	// Notice that this also flips the sign of the scalars and points as needed
	// to minimize the bit lengths of the scalars k1 and k2.
	//
	// This is done because the scalars are operating modulo the group order
	// which means that when they would otherwise be a small negative magnitude
	// they will instead be a large positive magnitude.  Since the goal is for
	// the scalars to have a small magnitude to achieve a performance boost, use
	// their negation when they are greater than the half order of the group.
	//
	// In order to compensate, the positive and negative values of the
	// corresponding points that will be multiplied by are flipped later.
	//
	// In other words, transform the calc when k1 is over the half order to:
	//   k1*P = -k1*-P
	//
	// Similarly, transform the calc when k2 is over the half order to:
	//   k2*φ(P) = -k2*-φ(P)
	var k1Neg, k2Neg bool
	k1, k2 := splitK(k)
	if k1.IsOverHalfOrder() {
		k1.Negate()
		k1Neg = true
	}
	if k2.IsOverHalfOrder() {
		k2.Negate()
		k2Neg = true
	}

	// Per above, the main equation here to remember is:
	//   k*P = k1*P + k2*φ(P)
	//
	// The simultaneous multiplication therefore needs two sets of precomputed
	// odd multiples:
	//
	//   P, 3P, 5P, ...
	//
	// and:
	//
	//   φ(P), 3φ(P), 5φ(P), ...
	var pPrecomps, phiPrecomps wNAFPrecompTable
	buildOddPrecomputeTables(point, &pPrecomps, &phiPrecomps, k1Neg, k2Neg)

	// Convert k1 and k2 into their windowed NAF representations since they have
	// a lot more zeroes overall on average which greatly reduces the number of
	// point additions at the cost of precomputing a few odd multiples of the
	// point and, in the worst case, one additional point doubling due to a
	// carry introduced by the recoding.
	//
	// This is an excellent tradeoff because subtraction of points has the same
	// computational complexity as addition of points and point doubling is
	// faster than both.
	//
	// Concretely, on average, 1/2 of all bits will be non-zero with the normal
	// binary representation whereas only 1/6 of the bits will be non-zero with
	// width-5 wNAF.
	k1NAF, k2NAF := wnaf(&k1), wnaf(&k2)

	// Add left-to-right using the endomorphism and wNAF optimizations.  See
	// algorithms 3.36 and 3.77 from [GECC].
	//
	// Point Q = ∞ (point at infinity).
	//
	// Like ordinary binary scalar multiplication, the accumulator is doubled
	// once per processed bit position.  However, instead of each bit
	// contributing either 0 or 1 copies of the point, each non-zero wNAF digit
	// contributes a precomputed odd multiple while the preceding doublings
	// supply the required power-of-two scaling.
	//
	//    ±P, ±3P, ±5P, ...
	//
	// Since non-zero digits are sparse and separated by several zero digits,
	// relatively few point additions are required compared to the ordinary
	// binary representation.
	var q JacobianPoint
	maxBits := k1NAF.bits
	if maxBits < k2NAF.bits {
		maxBits = k2NAF.bits
	}
	for bit := maxBits; bit > 0; bit-- {
		// Q = 2 * Q
		DoubleNonConst(&q, &q)

		// The encoded wNAF digit of k1 serves directly as an index into the
		// precomputed odd multiples of P.
		if code := k1NAF.codes[bit-1]; code != 0 {
			AddNonConst(&q, &pPrecomps[code], &q)
		}

		// Likewise, the encoded wNAF digit of k2 serves directly as an index
		// into the precomputed odd multiples of φ(P).
		if code := k2NAF.codes[bit-1]; code != 0 {
			AddNonConst(&q, &phiPrecomps[code], &q)
		}
	}

	result.Set(&q)
}

// ScalarBaseMultNonConst multiplies k*G where k is a scalar modulo the curve
// order and G is the base point of the group and stores the result in the
// provided Jacobian point.
//
// NOTE: The resulting point will be normalized.
func ScalarBaseMultNonConst(k *ModNScalar, result *JacobianPoint) {
	scalarBaseMultNonConst(k, result)
}

// jacobianG is the secp256k1 base point converted to Jacobian coordinates and
// is defined here to avoid repeatedly converting it.
var jacobianG = func() JacobianPoint {
	var G JacobianPoint
	bigAffineToJacobian(curveParams.Gx, curveParams.Gy, &G)
	return G
}()

// scalarBaseMultNonConstSlow computes k*G through [ScalarMultNonConst].
func scalarBaseMultNonConstSlow(k *ModNScalar, result *JacobianPoint) {
	ScalarMultNonConst(k, &jacobianG, result)
}

// scalarBaseMultNonConstFast computes k*G through the precomputed lookup
// tables.
func scalarBaseMultNonConstFast(k *ModNScalar, result *JacobianPoint) {
	bytePoints := s256BytePoints()

	// Start with the point at infinity.
	result.X.Zero()
	result.Y.Zero()
	result.Z.Zero()

	// bytePoints has all 256 byte points for each 8-bit window.  The strategy
	// is to add up the byte points.  This is best understood by expressing k in
	// base-256 which it already sort of is.  Each "digit" in the 8-bit window
	// can be looked up using bytePoints and added together.
	kb := k.Bytes()
	for i := 0; i < len(kb); i++ {
		pt := &bytePoints[i][kb[i]]
		AddNonConst(result, pt, result)
	}
}

// isOnCurve returns whether or not the affine point (x,y) is on the curve.
func isOnCurve(fx, fy *FieldVal) bool {
	// Elliptic curve equation for secp256k1 is: y^2 = x^3 + 7
	y2 := new(FieldVal).SquareVal(fy).Normalize()
	result := new(FieldVal).SquareVal(fx).Mul(fx).AddInt(7).Normalize()
	return y2.Equals(result)
}

// DecompressY attempts to calculate the Y coordinate for the given X coordinate
// such that the result pair is a point on the secp256k1 curve.  It adjusts Y
// based on the desired oddness and returns whether or not it was successful
// since not all X coordinates are valid.
//
// The magnitude of the provided X coordinate field value must be a max of 8 for
// a correct result.  The resulting Y field value will have a magnitude of 1.
//
//	Preconditions:
//	  - The input field value MUST have a max magnitude of 8
//	Output Normalized: Yes if the func returns true, no otherwise
//	Output Max Magnitude: 1
func DecompressY(x *FieldVal, odd bool, resultY *FieldVal) bool {
	// The curve equation for secp256k1 is: y^2 = x^3 + 7.  Thus
	// y = +-sqrt(x^3 + 7).
	//
	// The x coordinate must be invalid if there is no square root for the
	// calculated rhs because it means the X coordinate is not for a point on
	// the curve.
	x3PlusB := new(FieldVal).SquareVal(x).Mul(x).AddInt(7)
	if hasSqrt := resultY.SquareRootVal(x3PlusB); !hasSqrt {
		return false
	}
	if resultY.Normalize().IsOdd() != odd {
		resultY.Negate(1).Normalize()
	}
	return true
}
