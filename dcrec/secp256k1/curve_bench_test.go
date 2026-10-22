// Copyright (c) 2015-2024 The Decred developers
// Copyright 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"testing"
)

// BenchmarkAddNonConst benchmarks [AddNonConst] with different Z values so the
// associated optimizations are used.
func BenchmarkAddNonConst(b *testing.B) {
	benches := []struct {
		name string        // benchmark name
		p1   JacobianPoint // first point to add
		p2   JacobianPoint // second point to add
	}{{
		name: "Z1AndZ2EqualsOne",
		p1: jacobianPointFromHex(
			"e58701ec91f70116d687eae0a9ff983c18bbf8b9dc7af4c0d156ef6b346976d9",
			"8548ba41c5151ac1c5508407f3e88fb5564bc0ca9ed9b45c09d37f7b2bf0e233",
			"1",
		),
		p2: jacobianPointFromHex(
			"ab9dc865d911428f736323edc54754a66779f132d34b15776594ae6ae3ef7ed8",
			"be70a94a7174ca3fa4f4cf336237f8f4f5eb1871f19652b660de1f6e035516db",
			"1",
		),
	}, {
		name: "Z1EqualsZ2",
		p1: jacobianPointFromHex(
			"fe721918180fc1c438af34ba5c9dcc2474dc44bb51231a25e3a823a63bd77acd",
			"c53f8a498b8ee07ac3446225502fd58e95b5aada3455daf8c0429b647a0c87cc",
			"a8694009b10277101fc19d4e2305294025bdefbd6b9977d5f8b744fcae2e8abe",
		),
		p2: jacobianPointFromHex(
			"b2482d2a189f4bbd1525a26e7a31e7ec1d3b019936fbc15fbeccd07f02ec0756",
			"23d89d4b247cde0fd0cefd045d44715ae7cb2f82283a253434ce847747c308ba",
			"a8694009b10277101fc19d4e2305294025bdefbd6b9977d5f8b744fcae2e8abe",
		),
	}, {
		name: "Z2EqualsOne",
		p1: jacobianPointFromHex(
			"7a230dc234eb781bbe7cd768ced84fc345ecc4e34e476f9337ffb1e870941262",
			"e1afbbc701bdf452371bb279dd1fef8c5ae82e72584ce1a78ae88676af1cfd00",
			"707fd03ce03fd71c12805fd8424f6a62c72dc99b0bcdb9d272dcffca5f0a0acb",
		),
		p2: jacobianPointFromHex(
			"57d381ef9578a5a0cc4ba29bdcf50bd200785c5eeca3e47662f2c6afc9dcb85a",
			"49693374f10fa368f9a356e517cda646105aeb241974202d900639ac82c4775c",
			"1",
		),
	}, {
		name: "Generic",
		p1: jacobianPointFromHex(
			"54af26a77dc7bcaf3432f50078563e86e93b3b8e0a082113686063268cf80238",
			"87d299a77db5236ce03f44324f0554400e18e137eb2c27744ceb59202e3d020a",
			"d916a80aeffc75213aa03540ef318aa4d8ba24a31bff57697cb07027a1d8327f",
		),
		p2: jacobianPointFromHex(
			"e189299e58e2fef759c180b0b3c24fc83632e933b3c53f84e3c5aa90599a2c25",
			"dc9a10999eeb9eaa933ba65771a86d017f779b2798f7453d58339d2b2bcad75b",
			"21dc46a4efcb7db7785de3ca1fc904d587bbe4322b97134a32f500e5e83bc6ef",
		),
	}}

	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			var result JacobianPoint
			for i := 0; i < b.N; i++ {
				AddNonConst(&bench.p1, &bench.p2, &result)
			}
		})
	}
}

// BenchmarkDoubleNonConst benchmarks [DoubleNonConst] with different Z values
// so the associated optimizations are used.
func BenchmarkDoubleNonConst(b *testing.B) {
	benches := []struct {
		name  string
		point JacobianPoint
	}{{
		name: "ZEqualsOne",
		point: jacobianPointFromHex(
			"12f38f1a8f5937a4532da5eca79c2c2f7cdaefc49d499d39ab80a89cd039dc0e",
			"14cad95d93ea03e084c86d8dd39bcd3b91fbd2e521e9cb35d87ee5b7d70938a4",
			"1",
		),
	}, {
		name: "Generic",
		point: jacobianPointFromHex(
			"d05d2dbbf41e6597ecf2ff7fa1181c328dc12c80a19c655496e94ab8a0896e70",
			"87cf4bd45163dd66ae3f80c17c6ff89eee00ea408c6d9c06ab1383637715653c",
			"ce69442581a092b658fc4c697e07ba9bce9c63267a548d324df6f007868cf342",
		),
	}}

	for _, bench := range benches {
		b.Run(bench.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			var result JacobianPoint
			for i := 0; i < b.N; i++ {
				DoubleNonConst(&bench.point, &result)
			}
		})
	}
}

// BenchmarkScalarBaseMultNonConst benchmarks multiplying a scalar by the base
// point of the curve using whichever variant is active.
func BenchmarkScalarBaseMultNonConst(b *testing.B) {
	k := mustModNScalar("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")

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
	k := mustModNScalar("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")

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
	k := mustModNScalar("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")

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
	halfOrder := mustModNScalar(h)
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
	k := mustModNScalar("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")
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

// BenchmarkWNAF benchmarks conversion of a non-negative integer into its
// width-w windowed non-adjacent form representation, where w is [wNAFWidth].
func BenchmarkWNAF(b *testing.B) {
	k := mustModNScalar("8447e288d34b360bc885cb8ce7c00575")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wnaf(k)
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
