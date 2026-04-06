// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package utxoproof

import (
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// genSecp256k1KeyPair generates and returns a new secp256k1 key pair.
func genSecp256k1KeyPair(b *testing.B) *Secp256k1KeyPair {
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		b.Fatalf("failed to generate key pair: %v", err)
	}
	return &Secp256k1KeyPair{
		Pub:  privKey.PubKey().SerializeCompressed(),
		Priv: privKey,
	}
}

// BenchmarkValidateSecp256k1P2PKH benchmarks how long it takes to sign a utxo
// proof via [SignUtxoProof] along with the number of allocations needed.
func BenchmarkSignUtxoProof(b *testing.B) {
	const expires = 10
	keyPair := genSecp256k1KeyPair(b)

	b.ReportAllocs()
	for b.Loop() {
		_, err := keyPair.SignUtxoProof(expires)
		if err != nil {
			b.Fatalf("failed to sign utxo proof: %v", err)
		}
	}
}

// BenchmarkValidateSecp256k1P2PKH benchmarks how long it takes to validate a
// secp256k1 p2pkh utxo proof via [ValidateSecp256k1P2PKH] along with the number
// of allocations needed.
func BenchmarkValidateSecp256k1P2PKH(b *testing.B) {
	const expires = 10
	keyPair := genSecp256k1KeyPair(b)
	proof, err := keyPair.SignUtxoProof(expires)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if !ValidateSecp256k1P2PKH(keyPair.Pub, proof, expires) {
			b.Fatal("failed to validate utxo proof")
		}
	}
}
