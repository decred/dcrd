// Copyright 2013-2016 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
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

// BenchmarkScalarBaseMult benchmarks the secp256k1 curve ScalarBaseMult
// function.
func BenchmarkScalarBaseMult(b *testing.B) {
	k := fromHex("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")
	curve := S256()
	for i := 0; i < b.N; i++ {
		curve.ScalarBaseMult(k.Bytes())
	}
}

// BenchmarkScalarBaseMultNonConst benchmarks the ScalarBaseMultNonConst
// function.
func BenchmarkScalarBaseMultNonConst(b *testing.B) {
	k := new(ModNScalar).SetHex("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")
	var result JacobianPoint
	for i := 0; i < b.N; i++ {
		ScalarBaseMultNonConst(k, &result)
	}
}

// BenchmarkScalarBaseMultLarge benchmarks the secp256k1 curve ScalarBaseMult
// function with abnormally large k values.
func BenchmarkScalarBaseMultLarge(b *testing.B) {
	k := fromHex("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c005751111111011111110")
	curve := S256()
	for i := 0; i < b.N; i++ {
		curve.ScalarBaseMult(k.Bytes())
	}
}

// BenchmarkScalarMult benchmarks the secp256k1 curve ScalarMult function.
func BenchmarkScalarMult(b *testing.B) {
	x := fromHex("34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6")
	y := fromHex("0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232")
	k := fromHex("d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575")
	curve := S256()
	for i := 0; i < b.N; i++ {
		curve.ScalarMult(x, y, k.Bytes())
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

// BenchmarkPubKeyDecompress benchmarks how long it takes to decompress the  y
// coordinate from a given public key x coordinate.
func BenchmarkPubKeyDecompress(b *testing.B) {
	// Randomly generated keypair.
	// Private key: 9e0699c91ca1e3b7e3c9ba71eb71c89890872be97576010fe593fbf3fd57e66d
	pubKeyX := new(FieldVal).SetHex("d2e670a19c6d753d1a6d8b20bd045df8a08fb162cf508956c31268c6d81ffdab")

	b.ReportAllocs()
	b.ResetTimer()
	var y FieldVal
	for i := 0; i < b.N; i++ {
		_ = DecompressY(pubKeyX, false, &y)
	}
}

// BenchmarkParsePubKeyCompressed benchmarks how long it takes to parse a
// compressed public key with an even y coordinate.
func BenchmarkParsePubKeyCompressed(b *testing.B) {
	format := "02"
	x := "ce0b14fb842b1ba549fdd675c98075f12e9c510f8ef52bd021a9a1f4809d3b4d"
	pubKeyBytes := hexToBytes(format + x)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParsePubKey(pubKeyBytes)
	}
}

// BenchmarkParsePubKeyUncompressed benchmarks how long it takes to parse an
// uncompressed public key.
func BenchmarkParsePubKeyUncompressed(b *testing.B) {
	format := "04"
	x := "11db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c"
	y := "b2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3"
	pubKeyBytes := hexToBytes(format + x + y)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParsePubKey(pubKeyBytes)
	}
}
