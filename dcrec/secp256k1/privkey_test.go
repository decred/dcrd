// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"bytes"
	"testing"
)

// TestGeneratePrivateKey ensures the key generation works as expected.
func TestGeneratePrivateKey(t *testing.T) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Errorf("failed to generate private key: %s", err)
		return
	}
	pub := priv.PubKey()
	if !isOnCurve(&pub.x, &pub.y) {
		t.Error("public key is not on the curve")
	}
}

// TestPrivKeys ensures a private key created from bytes produces both the
// correct associated public key as well serializes back to the original bytes.
func TestPrivKeys(t *testing.T) {
	tests := []struct {
		name string
		priv string // hex encoded private key to test
		pub  string // expected hex encoded serialized compressed public key
	}{{
		name: "random private key 1",
		priv: "eaf02ca348c524e6392655ba4d29603cd1a7347d9d65cfe93ce1ebffdca22694",
		pub:  "025ceeba2ab4a635df2c0301a3d773da06ac5a18a7c3e0d09a795d7e57d233edf1",
	}, {
		name: "random private key 2",
		priv: "24b860d0651db83feba821e7a94ba8b87162665509cefef0cbde6a8fbbedfe7c",
		pub:  "032a6e51bf218085647d330eac2fafaeee07617a777ad9e8e7141b4cdae92cb637",
	}}

	for _, test := range tests {
		// Parse test data.
		privKeyBytes := hexToBytes(test.priv)
		wantPubKeyBytes := hexToBytes(test.pub)

		priv := PrivKeyFromBytes(privKeyBytes)
		pub := priv.PubKey()

		serializedPubKey := pub.SerializeCompressed()
		if !bytes.Equal(serializedPubKey, wantPubKeyBytes) {
			t.Errorf("%s unexpected serialized public key - got: %x, want: %x",
				test.name, serializedPubKey, wantPubKeyBytes)
		}

		serializedPrivKey := priv.Serialize()
		if !bytes.Equal(serializedPrivKey, privKeyBytes) {
			t.Errorf("%s unexpected serialized private key - got: %x, want: %x",
				test.name, serializedPrivKey, privKeyBytes)
		}
	}
}

// TestPrivateKeyZero ensures that zeroing a private key clears the memory
// associated with it.
func TestPrivateKeyZero(t *testing.T) {
	// Create a new private key and zero the initial key material that is now
	// copied into the private key.
	key := new(ModNScalar).SetHex("eaf02ca348c524e6392655ba4d29603cd1a7347d9d65cfe93ce1ebffdca22694")
	privKey := NewPrivateKey(key)
	key.Zero()

	// Ensure the private key is non zero.
	if privKey.Key.IsZero() {
		t.Fatal("private key is zero when it should be non zero")
	}

	// Zero the private key and ensure it was properly zeroed.
	privKey.Zero()
	if !privKey.Key.IsZero() {
		t.Fatal("private key is non zero when it should be zero")
	}
}
