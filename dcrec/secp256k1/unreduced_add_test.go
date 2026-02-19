// Copyright (c) 2024-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"math/big"
	"testing"
)

// findSmallXPoint finds a point on secp256k1 with x < 2^256 - p (so that x+p
// fits in 32 bytes) by iterating small x values and checking for a valid
// y-coordinate.
func findSmallXPoint(t *testing.T) (*big.Int, *big.Int) {
	t.Helper()

	p := curveParams.P

	// For p ≡ 3 mod 4, sqrt(a) = a^((p+1)/4) mod p.
	exp := new(big.Int).Add(p, big.NewInt(1))
	exp.Rsh(exp, 2)

	for x := int64(1); x < 1000000; x++ {
		xBig := big.NewInt(x)

		// Compute y^2 = x^3 + 7 mod p.
		y2 := new(big.Int).Exp(xBig, big.NewInt(3), p)
		y2.Add(y2, big.NewInt(7))
		y2.Mod(y2, p)

		// Attempt square root.
		yCandidate := new(big.Int).Exp(y2, exp, p)

		// Verify the candidate is a valid square root.
		ySquared := new(big.Int).Mul(yCandidate, yCandidate)
		ySquared.Mod(ySquared, p)
		if ySquared.Cmp(y2) == 0 {
			return xBig, yCandidate
		}
	}

	t.Fatal("failed to find a valid point with small x coordinate")
	return nil, nil
}

// TestAddUnreducedCoordinates verifies that point addition correctly dispatches
// to doubling when two equal points have different unreduced FieldVal
// representations.
//
// The bug: bigAffineToJacobian stores big.Int values in FieldVal without
// reducing mod p. When a value v >= p is stored, its FieldVal representation
// differs from the reduced equivalent v-p. The Equals check in the addition
// functions then fails to detect that two points are identical, causing the
// addition formula to degenerate and return the point at infinity instead of 2P.
func TestAddUnreducedCoordinates(t *testing.T) {
	curve := S256()
	p := curveParams.P

	ptX, ptY := findSmallXPoint(t)

	// Construct unreduced x' = x + p. Since x is small and p < 2^256 -
	// 2^32, x' still fits in 32 bytes and bypasses the truncation in
	// SetByteSlice.
	xUnreduced := new(big.Int).Add(ptX, p)
	if len(xUnreduced.Bytes()) > 32 {
		t.Fatal("x + p exceeds 32 bytes, cannot trigger the bug")
	}

	// The correct result: 2P via explicit doubling.
	doubleX, doubleY := curve.Double(ptX, ptY)

	// Bug case 1: Add(P, P') where P' has unreduced x = x + p.
	addX, addY := curve.Add(ptX, ptY, xUnreduced, ptY)
	if doubleX.Cmp(addX) != 0 || doubleY.Cmp(addY) != 0 {
		t.Errorf("Add(P, P') != Double(P)\n"+
			"  Double(P):  (0x%x, 0x%x)\n"+
			"  Add(P, P'): (%d, %d)",
			doubleX, doubleY, addX, addY)
	}

	// Bug case 2: Symmetric -- Add(P', P).
	addX2, addY2 := curve.Add(xUnreduced, ptY, ptX, ptY)
	if doubleX.Cmp(addX2) != 0 || doubleY.Cmp(addY2) != 0 {
		t.Errorf("Add(P', P) != Double(P)\n"+
			"  Double(P):  (0x%x, 0x%x)\n"+
			"  Add(P', P): (%d, %d)",
			doubleX, doubleY, addX2, addY2)
	}

	// Bug case 3: Both unreduced -- Add(P', P') should also equal 2P.
	// Note: this case works before the fix because both FieldVals have the
	// same unreduced representation, so Equals returns true.
	addX3, addY3 := curve.Add(xUnreduced, ptY, xUnreduced, ptY)
	if doubleX.Cmp(addX3) != 0 || doubleY.Cmp(addY3) != 0 {
		t.Errorf("Add(P', P') != Double(P)\n"+
			"  Double(P):   (0x%x, 0x%x)\n"+
			"  Add(P', P'): (%d, %d)",
			doubleX, doubleY, addX3, addY3)
	}

	// Bug case 4: Unreduced y coordinate. Construct y' = y + p.
	yUnreduced := new(big.Int).Add(ptY, p)
	if len(yUnreduced.Bytes()) <= 32 {
		addX4, addY4 := curve.Add(ptX, ptY, ptX, yUnreduced)
		if doubleX.Cmp(addX4) != 0 || doubleY.Cmp(addY4) != 0 {
			t.Errorf("Add(P, P_yunreduced) != Double(P)\n"+
				"  Double(P): (0x%x, 0x%x)\n"+
				"  Add:       (%d, %d)",
				doubleX, doubleY, addX4, addY4)
		}
	}
}

// TestBigAffineToJacobianNormalization verifies that bigAffineToJacobian
// normalizes the field values, ensuring that unreduced big.Int coordinates
// are properly reduced mod p before being stored.
func TestBigAffineToJacobianNormalization(t *testing.T) {
	ptX, ptY := findSmallXPoint(t)
	p := curveParams.P
	xUnreduced := new(big.Int).Add(ptX, p)

	// Convert both the reduced and unreduced x values through
	// bigAffineToJacobian.
	var p1, p2 JacobianPoint
	bigAffineToJacobian(ptX, ptY, &p1)
	bigAffineToJacobian(xUnreduced, ptY, &p2)

	// After the fix, both points should have identical normalized FieldVal
	// representations because bigAffineToJacobian normalizes.
	if !p1.X.Equals(&p2.X) {
		t.Errorf("bigAffineToJacobian did not normalize x coordinate\n"+
			"  from x:   %x\n"+
			"  from x+p: %x",
			p1.X.Bytes(), p2.X.Bytes())
	}
	if !p1.Y.Equals(&p2.Y) {
		t.Errorf("bigAffineToJacobian did not normalize y coordinate")
	}

	// Since the FieldVals are now identical, AddNonConst should correctly
	// dispatch to doubling.
	var result JacobianPoint
	AddNonConst(&p1, &p2, &result)

	var expected JacobianPoint
	DoubleNonConst(&p1, &expected)

	result.ToAffine()
	expected.ToAffine()

	if !result.X.Equals(&expected.X) || !result.Y.Equals(&expected.Y) {
		t.Errorf("AddNonConst(P, P_from_unreduced) != DoubleNonConst(P)\n"+
			"  Expected X: %x\n"+
			"  Got X:      %x",
			expected.X.Bytes(), result.X.Bytes())
	}
}
