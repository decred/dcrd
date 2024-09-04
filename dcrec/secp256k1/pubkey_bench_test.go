// Copyright (c) 2015-2024 The Decred developers
// Copyright 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"testing"
)

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
