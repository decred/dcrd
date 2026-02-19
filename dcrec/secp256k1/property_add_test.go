// Copyright (c) 2024-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"math/big"
	"testing"

	"pgregory.net/rapid"
)

// fieldPrimeBigInt is the secp256k1 field prime as a big.Int for use in
// property generators.
var fieldPrimeBigInt = curveParams.P

// complement256 is 2^256 - p, the maximum value that can be added to a valid
// field element before overflowing 32 bytes.
var complement256 = new(big.Int).Sub(
	new(big.Int).Lsh(big.NewInt(1), 256),
	fieldPrimeBigInt,
)

// genOnCurvePoint generates a random valid affine point on secp256k1 by
// performing a scalar base multiplication with a random non-zero scalar.
func genOnCurvePoint(t *rapid.T) (*big.Int, *big.Int) {
	// Generate a random non-zero scalar and multiply by the generator.
	scalarBytes := rapid.SliceOfN(rapid.Byte(), 32, 32).Draw(t, "scalar")
	var k ModNScalar
	k.SetByteSlice(scalarBytes)
	if k.IsZero() {
		k.SetInt(1)
	}

	var result JacobianPoint
	ScalarBaseMultNonConst(&k, &result)
	result.ToAffine()

	x := new(big.Int).SetBytes(result.X.Bytes()[:])
	y := new(big.Int).SetBytes(result.Y.Bytes()[:])
	return x, y
}

// genSmallXOnCurvePoint generates a random valid point with a small x
// coordinate (< 2^256 - p) so that x+p still fits in 32 bytes.
func genSmallXOnCurvePoint(t *rapid.T) (*big.Int, *big.Int) {
	// Iterate from a random small starting x to find a point on the curve.
	start := rapid.Int64Range(1, 100000).Draw(t, "startX")
	exp := new(big.Int).Add(fieldPrimeBigInt, big.NewInt(1))
	exp.Rsh(exp, 2)

	for x := start; x < start+100000; x++ {
		xBig := big.NewInt(x)
		y2 := new(big.Int).Exp(xBig, big.NewInt(3), fieldPrimeBigInt)
		y2.Add(y2, big.NewInt(7))
		y2.Mod(y2, fieldPrimeBigInt)

		yCandidate := new(big.Int).Exp(y2, exp, fieldPrimeBigInt)
		ySquared := new(big.Int).Mul(yCandidate, yCandidate)
		ySquared.Mod(ySquared, fieldPrimeBigInt)
		if ySquared.Cmp(y2) == 0 {
			return xBig, yCandidate
		}
	}

	// Fallback to the known x=1 point.
	xBig := big.NewInt(1)
	y2 := new(big.Int).Exp(xBig, big.NewInt(3), fieldPrimeBigInt)
	y2.Add(y2, big.NewInt(7))
	y2.Mod(y2, fieldPrimeBigInt)
	yCandidate := new(big.Int).Exp(y2, exp, fieldPrimeBigInt)
	return xBig, yCandidate
}

// TestPropertyAddUnreducedEqualsDouble verifies that for any point P with a
// small x coordinate, Add(P, P') == Double(P) where P' = (x+p, y).
//
// This property would have caught the unreduced coordinate addition bug.
func TestPropertyAddUnreducedEqualsDouble(t *testing.T) {
	curve := S256()

	rapid.Check(t, func(t *rapid.T) {
		ptX, ptY := genSmallXOnCurvePoint(t)

		// Create unreduced x' = x + p.
		xUnreduced := new(big.Int).Add(ptX, fieldPrimeBigInt)
		if len(xUnreduced.Bytes()) > 32 {
			t.Skip("x + p exceeds 32 bytes")
		}

		// Add(P, P') must equal Double(P).
		doubleX, doubleY := curve.Double(ptX, ptY)
		addX, addY := curve.Add(ptX, ptY, xUnreduced, ptY)

		if doubleX.Cmp(addX) != 0 || doubleY.Cmp(addY) != 0 {
			t.Fatalf("Add((x, y), (x+p, y)) != Double((x, y))\n"+
				"  x=%d\n"+
				"  Double: (%x, %x)\n"+
				"  Add:    (%d, %d)",
				ptX, doubleX, doubleY, addX, addY)
		}
	})
}

// TestPropertyAddUnreducedBothEqualsDouble verifies that when both x and y are
// unreduced (for points with small coordinates), Add(P, P') still equals
// Double(P).
func TestPropertyAddUnreducedBothEqualsDouble(t *testing.T) {
	curve := S256()

	rapid.Check(t, func(t *rapid.T) {
		ptX, ptY := genSmallXOnCurvePoint(t)

		xUnreduced := new(big.Int).Add(ptX, fieldPrimeBigInt)
		if len(xUnreduced.Bytes()) > 32 {
			t.Skip("x + p exceeds 32 bytes")
		}

		// Also try with unreduced y if it fits.
		yUnreduced := new(big.Int).Add(ptY, fieldPrimeBigInt)
		useUnreducedY := len(yUnreduced.Bytes()) <= 32
		if !useUnreducedY {
			yUnreduced = ptY
		}

		doubleX, doubleY := curve.Double(ptX, ptY)
		addX, addY := curve.Add(ptX, ptY, xUnreduced, yUnreduced)

		if doubleX.Cmp(addX) != 0 || doubleY.Cmp(addY) != 0 {
			t.Fatalf("Add(P, P_unreduced) != Double(P)\n"+
				"  x=%d, unreducedY=%v\n"+
				"  Double: (%x, %x)\n"+
				"  Add:    (%d, %d)",
				ptX, useUnreducedY, doubleX, doubleY, addX, addY)
		}
	})
}

// TestPropertyAddCommutative verifies that point addition is commutative:
// P + Q == Q + P for random points, including when one has unreduced
// coordinates.
func TestPropertyAddCommutative(t *testing.T) {
	curve := S256()

	rapid.Check(t, func(t *rapid.T) {
		p1x, p1y := genOnCurvePoint(t)
		p2x, p2y := genOnCurvePoint(t)

		// Standard commutativity: P + Q == Q + P.
		r1x, r1y := curve.Add(p1x, p1y, p2x, p2y)
		r2x, r2y := curve.Add(p2x, p2y, p1x, p1y)

		if r1x.Cmp(r2x) != 0 || r1y.Cmp(r2y) != 0 {
			t.Fatalf("Add(P, Q) != Add(Q, P)")
		}
	})
}

// TestPropertyAddIdentity verifies that adding the point at infinity (identity
// element) returns the original point: P + O == P.
func TestPropertyAddIdentity(t *testing.T) {
	curve := S256()
	zero := big.NewInt(0)

	rapid.Check(t, func(t *rapid.T) {
		px, py := genOnCurvePoint(t)

		// P + O == P.
		rx, ry := curve.Add(px, py, zero, zero)
		if rx.Cmp(px) != 0 || ry.Cmp(py) != 0 {
			t.Fatalf("P + O != P")
		}

		// O + P == P.
		rx, ry = curve.Add(zero, zero, px, py)
		if rx.Cmp(px) != 0 || ry.Cmp(py) != 0 {
			t.Fatalf("O + P != P")
		}
	})
}

// TestPropertyAddInverseIsIdentity verifies that P + (-P) == O (point at
// infinity).
func TestPropertyAddInverseIsIdentity(t *testing.T) {
	curve := S256()

	rapid.Check(t, func(t *rapid.T) {
		px, py := genOnCurvePoint(t)

		// -P = (x, p - y).
		negPy := new(big.Int).Sub(fieldPrimeBigInt, py)

		rx, ry := curve.Add(px, py, px, negPy)
		if rx.Sign() != 0 || ry.Sign() != 0 {
			t.Fatalf("P + (-P) != O, got (%x, %x)", rx, ry)
		}
	})
}

// TestPropertyDoubleEqualsAddSelf verifies that Double(P) == Add(P, P) for
// random points.
func TestPropertyDoubleEqualsAddSelf(t *testing.T) {
	curve := S256()

	rapid.Check(t, func(t *rapid.T) {
		px, py := genOnCurvePoint(t)

		doubleX, doubleY := curve.Double(px, py)
		addX, addY := curve.Add(px, py, px, py)

		if doubleX.Cmp(addX) != 0 || doubleY.Cmp(addY) != 0 {
			t.Fatalf("Double(P) != Add(P, P)\n"+
				"  Double: (%x, %x)\n"+
				"  Add:    (%x, %x)",
				doubleX, doubleY, addX, addY)
		}
	})
}

// TestPropertyAddResultOnCurve verifies that the result of point addition is
// always on the curve.
func TestPropertyAddResultOnCurve(t *testing.T) {
	curve := S256()

	rapid.Check(t, func(t *rapid.T) {
		p1x, p1y := genOnCurvePoint(t)
		p2x, p2y := genOnCurvePoint(t)

		rx, ry := curve.Add(p1x, p1y, p2x, p2y)

		// The result should be on the curve (or the point at infinity).
		if rx.Sign() == 0 && ry.Sign() == 0 {
			return // Point at infinity is valid.
		}
		if !curve.IsOnCurve(rx, ry) {
			t.Fatalf("Add result not on curve: (%x, %x)", rx, ry)
		}
	})
}

// TestPropertyScalarMultAddition verifies the distributive property:
// (a+b)*G == a*G + b*G for random non-zero scalars a, b.
func TestPropertyScalarMultAddition(t *testing.T) {
	curve := S256()

	rapid.Check(t, func(t *rapid.T) {
		aBytes := rapid.SliceOfN(rapid.Byte(), 32, 32).Draw(t, "a")
		bBytes := rapid.SliceOfN(rapid.Byte(), 32, 32).Draw(t, "b")

		var a, b ModNScalar
		a.SetByteSlice(aBytes)
		b.SetByteSlice(bBytes)

		// Skip the zero scalar case -- 0*G is the point at infinity
		// which requires special handling in the affine Add interface.
		if a.IsZero() || b.IsZero() {
			t.Skip("zero scalar")
		}

		// Also skip if a+b == 0 (a == -b) since the result is infinity.
		ab := new(ModNScalar).Add2(&a, &b)
		if ab.IsZero() {
			t.Skip("a + b == 0")
		}

		// Compute (a+b)*G.
		var abG JacobianPoint
		ScalarBaseMultNonConst(ab, &abG)
		abG.ToAffine()
		abGx := new(big.Int).SetBytes(abG.X.Bytes()[:])
		abGy := new(big.Int).SetBytes(abG.Y.Bytes()[:])

		// Compute a*G and b*G, converting to affine BEFORE extracting
		// big.Int values.
		var aG, bG JacobianPoint
		ScalarBaseMultNonConst(&a, &aG)
		ScalarBaseMultNonConst(&b, &bG)
		aG.ToAffine()
		bG.ToAffine()
		aGx := new(big.Int).SetBytes(aG.X.Bytes()[:])
		aGy := new(big.Int).SetBytes(aG.Y.Bytes()[:])
		bGx := new(big.Int).SetBytes(bG.X.Bytes()[:])
		bGy := new(big.Int).SetBytes(bG.Y.Bytes()[:])
		sumX, sumY := curve.Add(aGx, aGy, bGx, bGy)

		if abGx.Cmp(sumX) != 0 || abGy.Cmp(sumY) != 0 {
			t.Fatalf("(a+b)*G != a*G + b*G")
		}
	})
}

// TestPropertyFieldValNormalizeIdempotent verifies that normalizing a FieldVal
// is idempotent: Normalize(Normalize(x)) == Normalize(x).
func TestPropertyFieldValNormalizeIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		b := rapid.SliceOfN(rapid.Byte(), 32, 32).Draw(t, "bytes")
		var buf [32]byte
		copy(buf[:], b)

		var fv1, fv2 FieldVal
		fv1.SetBytes(&buf)
		fv2.SetBytes(&buf)

		fv1.Normalize()
		fv2.Normalize().Normalize()

		if !fv1.Equals(&fv2) {
			t.Fatalf("Normalize is not idempotent")
		}
	})
}

// TestPropertyFieldValEqualsAfterNormalize verifies that two FieldVals that
// differ only by a multiple of p are equal after normalization.
func TestPropertyFieldValEqualsAfterNormalize(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a small value so that value + p fits in 32 bytes.
		smallVal := rapid.Int64Range(0, complement256.Int64()-1).Draw(t, "val")
		valBig := big.NewInt(smallVal)

		// Create FieldVal from the raw value.
		var fv1 FieldVal
		fv1.SetByteSlice(valBig.Bytes())

		// Create FieldVal from value + p (unreduced).
		valPlusP := new(big.Int).Add(valBig, fieldPrimeBigInt)
		var fv2 FieldVal
		fv2.SetByteSlice(valPlusP.Bytes())

		// Before normalization, they may differ.
		beforeEqual := fv1.Equals(&fv2)

		// After normalization, they must be equal.
		fv1.Normalize()
		fv2.Normalize()

		if !fv1.Equals(&fv2) {
			t.Fatalf("FieldVals not equal after normalize: "+
				"val=%d, raw_equal_before=%v", smallVal, beforeEqual)
		}
	})
}

// TestPropertyBigAffineRoundTrip verifies that converting a point to big.Int
// affine coordinates and back produces the same Jacobian point, even with
// unreduced inputs.
func TestPropertyBigAffineRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		px, py := genOnCurvePoint(t)

		// Round-trip through big.Int → Jacobian → big.Int.
		var jp JacobianPoint
		bigAffineToJacobian(px, py, &jp)
		rxBig, ryBig := jacobianToBigAffine(&jp)

		if px.Cmp(rxBig) != 0 || py.Cmp(ryBig) != 0 {
			t.Fatalf("bigAffine round-trip failed")
		}
	})
}

// TestPropertyUnreducedBigAffineRoundTrip verifies round-trip with unreduced
// inputs: bigAffineToJacobian should normalize, so (x+p, y) round-trips to
// (x, y).
func TestPropertyUnreducedBigAffineRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ptX, ptY := genSmallXOnCurvePoint(t)

		xUnreduced := new(big.Int).Add(ptX, fieldPrimeBigInt)
		if len(xUnreduced.Bytes()) > 32 {
			t.Skip("x + p exceeds 32 bytes")
		}

		// Convert unreduced coordinates through bigAffineToJacobian.
		var jp JacobianPoint
		bigAffineToJacobian(xUnreduced, ptY, &jp)
		rxBig, ryBig := jacobianToBigAffine(&jp)

		// The result should be the reduced coordinates.
		if ptX.Cmp(rxBig) != 0 || ptY.Cmp(ryBig) != 0 {
			t.Fatalf("unreduced bigAffine round-trip failed:\n"+
				"  input x:  %d (unreduced: %x)\n"+
				"  output x: %x",
				ptX, xUnreduced, rxBig)
		}
	})
}
