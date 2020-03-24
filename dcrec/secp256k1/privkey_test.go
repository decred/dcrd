// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"bytes"
	"testing"
)

func TestPrivKeys(t *testing.T) {
	tests := []struct {
		name string
		key  []byte
	}{
		{
			name: "check curve",
			key: []byte{
				0xea, 0xf0, 0x2c, 0xa3, 0x48, 0xc5, 0x24, 0xe6,
				0x39, 0x26, 0x55, 0xba, 0x4d, 0x29, 0x60, 0x3c,
				0xd1, 0xa7, 0x34, 0x7d, 0x9d, 0x65, 0xcf, 0xe9,
				0x3c, 0xe1, 0xeb, 0xff, 0xdc, 0xa2, 0x26, 0x94,
			},
		},
	}

	for _, test := range tests {
		priv := PrivKeyFromBytes(test.key)
		pub := priv.PubKey()

		_, err := ParsePubKey(pub.SerializeUncompressed())
		if err != nil {
			t.Errorf("%s privkey: %v", test.name, err)
			continue
		}

		hash := []byte{0x0, 0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9}
		sig := priv.Sign(hash)
		if !sig.Verify(hash, pub) {
			t.Errorf("%s could not verify: %v", test.name, err)
			continue
		}

		serializedKey := priv.Serialize()
		if !bytes.Equal(serializedKey, test.key) {
			t.Errorf("%s unexpected serialized bytes - got: %x, "+
				"want: %x", test.name, serializedKey, test.key)
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
