// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestGenerateSharedSecret(t *testing.T) {
	privKey1, err := GeneratePrivateKey()
	if err != nil {
		t.Errorf("private key generation error: %s", err)
		return
	}
	privKey2, err := GeneratePrivateKey()
	if err != nil {
		t.Errorf("private key generation error: %s", err)
		return
	}

	pk1x, pk1y := privKey1.Public()
	pk1 := NewPublicKey(pk1x, pk1y)
	pk2x, pk2y := privKey2.Public()
	pk2 := NewPublicKey(pk2x, pk2y)
	secret1 := GenerateSharedSecret(privKey1, pk2)
	secret2 := GenerateSharedSecret(privKey2, pk1)
	if !bytes.Equal(secret1, secret2) {
		t.Errorf("ECDH failed, secrets mismatch - first: %x, second: %x",
			secret1, secret2)
	}
}

// Test 1: Encryption and decryption
func TestCipheringBasic(t *testing.T) {
	privkey, err := GeneratePrivateKey()
	if err != nil {
		t.Fatal("failed to generate private key")
	}

	in := []byte("Hey there dude. How are you doing? This is a test.")

	pk1x, pk1y := privkey.Public()
	pk1 := NewPublicKey(pk1x, pk1y)
	out, err := Encrypt(pk1, in)
	if err != nil {
		t.Fatal("failed to encrypt:", err)
	}

	dec, err := Decrypt(privkey, out)
	if err != nil {
		t.Fatal("failed to decrypt:", err)
	}

	if !bytes.Equal(in, dec) {
		t.Error("decrypted data doesn't match original")
	}
}

// Test 2: Byte compatibility with Pyelliptic
func TestCiphering(t *testing.T) {
	pb, _ := hex.DecodeString("fe38240982f313ae5afb3e904fb8215fb11af1200592b" +
		"fca26c96c4738e4bf8f")
	privkey, _ := PrivKeyFromBytes(pb)

	in := []byte("This is just a test.")
	out, _ := hex.DecodeString("b0d66e5adaa5ed4e2f0ca68e17b8f2fc02ca002009e3" +
		"3487e7fa4ab505cf34d98f131be7bd258391588ca7804acb30251e71a04e0020ecf" +
		"df0f84608f8add82d7353af780fbb28868c713b7813eb4d4e61f7b75d7534dd9856" +
		"9b0ba77cf14348fcff80fee10e11981f1b4be372d93923e9178972f69937ec850ed" +
		"6c3f11ff572ddd5b2bedf9f9c0b327c54da02a28fcdce1f8369ffec")

	dec, err := Decrypt(privkey, out)
	if err != nil {
		t.Fatal("failed to decrypt:", err)
	}

	if !bytes.Equal(in, dec) {
		t.Error("decrypted data doesn't match original")
	}
}
