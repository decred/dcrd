// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package schnorr

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v3"
)

// hexToModNScalar converts the passed hex string into a ModNScalar and will
// panic if there is an error.  This is only provided for the hard-coded
// constants so errors in the source code can be detected. It will only (and
// must only) be called with hard-coded values.
func hexToModNScalar(s string) *secp256k1.ModNScalar {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	var scalar secp256k1.ModNScalar
	if overflow := scalar.SetByteSlice(b); overflow {
		panic("hex in source file overflows mod N scalar: " + s)
	}
	return &scalar
}

// hexToBytes converts the passed hex string into bytes and will panic if there
// is an error.  This is only provided for the hard-coded constants so errors in
// the source code can be detected. It will only (and must only) be called with
// hard-coded values.
func hexToBytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	return b
}

// hexToBigInt converts the passed hex string into a big integer and will panic
// is there is an error.  This is only provided for the hard-coded constants so
// errors in the source code can bet detected. It will only (and must only) be
// called for initialization purposes.
func hexToBigInt(s string) *big.Int {
	r, ok := new(big.Int).SetString(s, 16)
	if !ok {
		panic("invalid hex in source file: " + s)
	}
	return r
}

// BenchmarkSign benchmarks how long it takes to sign a message.
func BenchmarkSign(b *testing.B) {
	// From randomly generated keypair.
	d := hexToModNScalar("9e0699c91ca1e3b7e3c9ba71eb71c89890872be97576010fe593fbf3fd57e66d")
	privKey := secp256k1.NewPrivateKey(d)

	// blake256 of []byte{0x01, 0x02, 0x03, 0x04}.
	msgHash := hexToBytes("c301ba9de5d6053caad9f5eb46523f007702add2c62fa39de03146a36b8026b7")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Sign(privKey, msgHash)
	}
}

// BenchmarkSigVerify benchmarks how long it takes to verify Schnorr signatures.
func BenchmarkSigVerify(b *testing.B) {
	// From randomly generated keypair.
	d := hexToModNScalar("9e0699c91ca1e3b7e3c9ba71eb71c89890872be97576010fe593fbf3fd57e66d")
	privKey := secp256k1.NewPrivateKey(d)
	pubKey := secp256k1.NewPublicKey(
		hexToBigInt("d2e670a19c6d753d1a6d8b20bd045df8a08fb162cf508956c31268c6d81ffdab"),
		hexToBigInt("ab65528eefbb8057aa85d597258a3fbd481a24633bc9b47a9aa045c91371de52"),
	)

	// blake256 of []byte{0x01, 0x02, 0x03, 0x04}.
	msgHash := hexToBytes("c301ba9de5d6053caad9f5eb46523f007702add2c62fa39de03146a36b8026b7")

	// Generate the signature.
	sig, _ := Sign(privKey, msgHash)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sig.Verify(msgHash, pubKey)
	}
}

// BenchmarkSigVerify benchmarks how long it takes to serialize Schnorr
// signatures.
func BenchmarkSigSerialize(b *testing.B) {
	// From randomly generated keypair.
	d := hexToModNScalar("9e0699c91ca1e3b7e3c9ba71eb71c89890872be97576010fe593fbf3fd57e66d")
	privKey := secp256k1.NewPrivateKey(d)

	// blake256 of []byte{0x01, 0x02, 0x03, 0x04}.
	msgHash := hexToBytes("c301ba9de5d6053caad9f5eb46523f007702add2c62fa39de03146a36b8026b7")

	// Generate the signature.
	sig, _ := Sign(privKey, msgHash)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sig.Serialize()
	}
}
