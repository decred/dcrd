// Copyright (c) 2015-2024 The Decred developers
// Copyright 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"testing"
)

// BenchmarkAddNonConst benchmarks the secp256k1 curve AddNonConst function with
// Z values of 1 so that the associated optimizations are used.
func BenchmarkAddNonConst(b *testing.B) {
	p1 := jacobianPointFromHex(
		"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
		"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
		"1",
	)
	p2 := jacobianPointFromHex(
		"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
		"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
		"1",
	)

	b.ReportAllocs()
	b.ResetTimer()
	var result JacobianPoint
	for i := 0; i < b.N; i++ {
		AddNonConst(&p1, &p2, &result)
	}
}

// BenchmarkAddNonConstNotZOne benchmarks the secp256k1 curve AddNonConst
// function with Z values other than one so the optimizations associated with
// Z=1 aren't used.
func BenchmarkAddNonConstNotZOne(b *testing.B) {
	x1 := new(FieldVal).SetHex("d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718")
	y1 := new(FieldVal).SetHex("5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190")
	z1 := new(FieldVal).SetHex("2")
	x2 := new(FieldVal).SetHex("91abba6a34b7481d922a4bd6a04899d5a686f6cf6da4e66a0cb427fb25c04bd4")
	y2 := new(FieldVal).SetHex("03fede65e30b4e7576a2abefc963ddbf9fdccbf791b77c29beadefe49951f7d1")
	z2 := new(FieldVal).SetHex("3")
	p1 := MakeJacobianPoint(x1, y1, z1)
	p2 := MakeJacobianPoint(x2, y2, z2)

	b.ReportAllocs()
	b.ResetTimer()
	var result JacobianPoint
	for i := 0; i < b.N; i++ {
		AddNonConst(&p1, &p2, &result)
	}
}

// BenchmarkScalarBaseMultNonConst benchmarks multiplying a scalar by the base
// point of the curve using whichever variant is active.
func BenchmarkScalarBaseMultNonConst(b *testing.B) {
	k := hexToModNScalar("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")

	b.ReportAllocs()
	b.ResetTimer()
	var result JacobianPoint
	for i := 0; i < b.N; i++ {
		ScalarBaseMultNonConst(k, &result)
	}
}

// BenchmarkScalarBaseMultNonConstFast benchmarks multiplying a scalar by the
// base point of the curve using the fast variant.
func BenchmarkScalarBaseMultNonConstFast(b *testing.B) {
	k := hexToModNScalar("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")

	b.ReportAllocs()
	b.ResetTimer()
	var result JacobianPoint
	for i := 0; i < b.N; i++ {
		scalarBaseMultNonConstFast(k, &result)
	}
}

// BenchmarkScalarBaseMultNonConstSlow benchmarks multiplying a scalar by the
// base point of the curve using the resource-constrained slow variant.
func BenchmarkScalarBaseMultNonConstSlow(b *testing.B) {
	k := hexToModNScalar("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")

	b.ReportAllocs()
	b.ResetTimer()
	var result JacobianPoint
	for i := 0; i < b.N; i++ {
		scalarBaseMultNonConstSlow(k, &result)
	}
}

// BenchmarkSplitK benchmarks decomposing scalars into a balanced length-two
// representation.
func BenchmarkSplitK(b *testing.B) {
	// Values computed from the group half order and lambda such that they
	// exercise the decomposition edge cases and maximize the bit lengths of the
	// produced scalars.
	h := "7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0"
	negOne := new(ModNScalar).NegateVal(oneModN)
	halfOrder := hexToModNScalar(h)
	halfOrderMOne := new(ModNScalar).Add2(halfOrder, negOne)
	halfOrderPOne := new(ModNScalar).Add2(halfOrder, oneModN)
	lambdaMOne := new(ModNScalar).Add2(endoLambda, negOne)
	lambdaPOne := new(ModNScalar).Add2(endoLambda, oneModN)
	negLambda := new(ModNScalar).NegateVal(endoLambda)
	halfOrderMOneMLambda := new(ModNScalar).Add2(halfOrderMOne, negLambda)
	halfOrderMLambda := new(ModNScalar).Add2(halfOrder, negLambda)
	halfOrderPOneMLambda := new(ModNScalar).Add2(halfOrderPOne, negLambda)
	lambdaPHalfOrder := new(ModNScalar).Add2(endoLambda, halfOrder)
	lambdaPOnePHalfOrder := new(ModNScalar).Add2(lambdaPOne, halfOrder)
	scalars := []*ModNScalar{
		new(ModNScalar),      // zero
		oneModN,              // one
		negOne,               // group order - 1 (aka -1 mod N)
		halfOrderMOneMLambda, // group half order - 1 - lambda
		halfOrderMLambda,     // group half order - lambda
		halfOrderPOneMLambda, // group half order + 1 - lambda
		halfOrderMOne,        // group half order - 1
		halfOrder,            // group half order
		halfOrderPOne,        // group half order + 1
		lambdaMOne,           // lambda - 1
		endoLambda,           // lambda
		lambdaPOne,           // lambda + 1
		lambdaPHalfOrder,     // lambda + group half order
		lambdaPOnePHalfOrder, // lambda + 1 + group half order
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(scalars) {
		for j := 0; j < len(scalars); j++ {
			_, _ = splitK(scalars[j])
		}
	}
}

// BenchmarkScalarMultNonConst benchmarks multiplying a scalar by an arbitrary
// point on the curve.
func BenchmarkScalarMultNonConst(b *testing.B) {
	k := hexToModNScalar("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")
	point := jacobianPointFromHex(
		"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
		"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
		"1",
	)

	b.ReportAllocs()
	b.ResetTimer()
	var result JacobianPoint
	for i := 0; i < b.N; i++ {
		ScalarMultNonConst(k, &point, &result)
	}
}

// BenchmarkNAF benchmarks conversion of a positive integer into its
// non-adjacent form representation.
func BenchmarkNAF(b *testing.B) {
	k := fromHex("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")
	kBytes := k.Bytes()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		naf(kBytes)
	}
}

// BenchmarkJacobianPointEquivalency benchmarks determining if two Jacobian
// points represent the same affine point.
func BenchmarkJacobianPointEquivalency(b *testing.B) {
	// Create two Jacobian points with different Z values that represent the
	// same affine point.
	point1 := jacobianPointFromHex(
		"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
		"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
		"2",
	)
	point2 := jacobianPointFromHex(
		"dcc3768780c74a0325e2851edad0dc8a566fa61a9e7fc4a34d13dcb509f99bc7",
		"3503be6fb22abd76cb082f8aed63745b9149dd2b037728d32ebfebac99b51f17",
		"3",
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		point1.EquivalentNonConst(&point2)
	}
}
