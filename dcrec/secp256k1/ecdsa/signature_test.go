// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ecdsa

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

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

// TestSignatureParsing ensures that signatures are properly parsed according
// to DER rules.  The error paths are tested as well.
func TestSignatureParsing(t *testing.T) {
	tests := []struct {
		name string
		sig  []byte
		err  error
	}{{
		// signature from Decred blockchain tx
		// 76634e947f49dfc6228c3e8a09cd3e9e15893439fc06df7df0fc6f08d659856c:0
		name: "valid signature 1",
		sig: hexToBytes("3045022100cd496f2ab4fe124f977ffe3caa09f7576d8a34156" +
			"b4e55d326b4dffc0399a094022013500a0510b5094bff220c74656879b8ca03" +
			"69d3da78004004c970790862fc03"),
		err: nil,
	}, {
		// signature from Decred blockchain tx
		// 76634e947f49dfc6228c3e8a09cd3e9e15893439fc06df7df0fc6f08d659856c:1
		name: "valid signature 2",
		sig: hexToBytes("3044022036334e598e51879d10bf9ce3171666bc2d1bbba6164" +
			"cf46dd1d882896ba35d5d022056c39af9ea265c1b6d7eab5bc977f06f81e35c" +
			"dcac16f3ec0fd218e30f2bad2a"),
		err: nil,
	}, {
		name: "empty",
		sig:  nil,
		err:  ErrSigTooShort,
	}, {
		name: "too short",
		sig:  hexToBytes("30050201000200"),
		err:  ErrSigTooShort,
	}, {
		name: "too long",
		sig: hexToBytes("3045022100f5353150d31a63f4a0d06d1f5a01ac65f7267a719e" +
			"49f2a1ac584fd546bef074022030e09575e7a1541aa018876a4003cefe1b061a" +
			"90556b5140c63e0ef8481352480101"),
		err: ErrSigTooLong,
	}, {
		name: "bad ASN.1 sequence id",
		sig: hexToBytes("3145022100f5353150d31a63f4a0d06d1f5a01ac65f7267a719e" +
			"49f2a1ac584fd546bef074022030e09575e7a1541aa018876a4003cefe1b061a" +
			"90556b5140c63e0ef848135248"),
		err: ErrSigInvalidSeqID,
	}, {
		name: "mismatched data length (short one byte)",
		sig: hexToBytes("3044022100f5353150d31a63f4a0d06d1f5a01ac65f7267a719e" +
			"49f2a1ac584fd546bef074022030e09575e7a1541aa018876a4003cefe1b061a" +
			"90556b5140c63e0ef848135248"),
		err: ErrSigInvalidDataLen,
	}, {
		name: "mismatched data length (long one byte)",
		sig: hexToBytes("3046022100f5353150d31a63f4a0d06d1f5a01ac65f7267a719e" +
			"49f2a1ac584fd546bef074022030e09575e7a1541aa018876a4003cefe1b061a" +
			"90556b5140c63e0ef848135248"),
		err: ErrSigInvalidDataLen,
	}, {
		name: "bad R ASN.1 int marker",
		sig: hexToBytes("304403204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56c" +
			"bbac4622082221a8768d1d09"),
		err: ErrSigInvalidRIntID,
	}, {
		name: "zero R length",
		sig: hexToBytes("30240200022030e09575e7a1541aa018876a4003cefe1b061a90" +
			"556b5140c63e0ef848135248"),
		err: ErrSigZeroRLen,
	}, {
		name: "negative R (too little padding)",
		sig: hexToBytes("30440220b2ec8d34d473c3aa2ab5eb7cc4a0783977e5db8c8daf" +
			"777e0b6d7bfa6b6623f302207df6f09af2c40460da2c2c5778f636d3b2e27e20" +
			"d10d90f5a5afb45231454700"),
		err: ErrSigNegativeR,
	}, {
		name: "too much R padding",
		sig: hexToBytes("304402200077f6e93de5ed43cf1dfddaa79fca4b766e1a8fc879" +
			"b0333d377f62538d7eb5022054fed940d227ed06d6ef08f320976503848ed1f5" +
			"2d0dd6d17f80c9c160b01d86"),
		err: ErrSigTooMuchRPadding,
	}, {
		name: "bad S ASN.1 int marker",
		sig: hexToBytes("3045022100f5353150d31a63f4a0d06d1f5a01ac65f7267a719e" +
			"49f2a1ac584fd546bef074032030e09575e7a1541aa018876a4003cefe1b061a" +
			"90556b5140c63e0ef848135248"),
		err: ErrSigInvalidSIntID,
	}, {
		name: "missing S ASN.1 int marker",
		sig: hexToBytes("3023022100f5353150d31a63f4a0d06d1f5a01ac65f7267a719e" +
			"49f2a1ac584fd546bef074"),
		err: ErrSigMissingSTypeID,
	}, {
		name: "S length missing",
		sig: hexToBytes("3024022100f5353150d31a63f4a0d06d1f5a01ac65f7267a719e" +
			"49f2a1ac584fd546bef07402"),
		err: ErrSigMissingSLen,
	}, {
		name: "invalid S length (short one byte)",
		sig: hexToBytes("3045022100f5353150d31a63f4a0d06d1f5a01ac65f7267a719e" +
			"49f2a1ac584fd546bef074021f30e09575e7a1541aa018876a4003cefe1b061a" +
			"90556b5140c63e0ef848135248"),
		err: ErrSigInvalidSLen,
	}, {
		name: "invalid S length (long one byte)",
		sig: hexToBytes("3045022100f5353150d31a63f4a0d06d1f5a01ac65f7267a719e" +
			"49f2a1ac584fd546bef074022130e09575e7a1541aa018876a4003cefe1b061a" +
			"90556b5140c63e0ef848135248"),
		err: ErrSigInvalidSLen,
	}, {
		name: "zero S length",
		sig: hexToBytes("3025022100f5353150d31a63f4a0d06d1f5a01ac65f7267a719e" +
			"49f2a1ac584fd546bef0740200"),
		err: ErrSigZeroSLen,
	}, {
		name: "negative S (too little padding)",
		sig: hexToBytes("304402204fc10344934662ca0a93a84d14d650d8a21cf2ab91f6" +
			"08e8783d2999c955443202208441aacd6b17038ff3f6700b042934f9a6fea0ce" +
			"c2051b51dc709e52a5bb7d61"),
		err: ErrSigNegativeS,
	}, {
		name: "too much S padding",
		sig: hexToBytes("304402206ad2fdaf8caba0f2cb2484e61b81ced77474b4c2aa06" +
			"9c852df1351b3314fe20022000695ad175b09a4a41cd9433f6b2e8e83253d6a7" +
			"402096ba313a7be1f086dde5"),
		err: ErrSigTooMuchSPadding,
	}, {
		name: "R == 0",
		sig: hexToBytes("30250201000220181522ec8eca07de4860a4acdd12909d831cc5" +
			"6cbbac4622082221a8768d1d09"),
		err: ErrSigRIsZero,
	}, {
		name: "R == N",
		sig: hexToBytes("3045022100fffffffffffffffffffffffffffffffebaaedce6af" +
			"48a03bbfd25e8cd03641410220181522ec8eca07de4860a4acdd12909d831cc5" +
			"6cbbac4622082221a8768d1d09"),
		err: ErrSigRTooBig,
	}, {
		name: "R > N (>32 bytes)",
		sig: hexToBytes("3045022101cd496f2ab4fe124f977ffe3caa09f756283910fc1a" +
			"96f60ee6873e88d3cfe1d50220181522ec8eca07de4860a4acdd12909d831cc5" +
			"6cbbac4622082221a8768d1d09"),
		err: ErrSigRTooBig,
	}, {
		name: "R > N",
		sig: hexToBytes("3045022100fffffffffffffffffffffffffffffffebaaedce6af" +
			"48a03bbfd25e8cd03641420220181522ec8eca07de4860a4acdd12909d831cc5" +
			"6cbbac4622082221a8768d1d09"),
		err: ErrSigRTooBig,
	}, {
		name: "S == 0",
		sig: hexToBytes("302502204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd41020100"),
		err: ErrSigSIsZero,
	}, {
		name: "S == N",
		sig: hexToBytes("304502204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd41022100fffffffffffffffffffffffffffffffebaaedc" +
			"e6af48a03bbfd25e8cd0364141"),
		err: ErrSigSTooBig,
	}, {
		name: "S > N (>32 bytes)",
		sig: hexToBytes("304502204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd4102210113500a0510b5094bff220c74656879b784b246" +
			"ba89c0a07bc49bcf05d8993d44"),
		err: ErrSigSTooBig,
	}, {
		name: "S > N",
		sig: hexToBytes("304502204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd41022100fffffffffffffffffffffffffffffffebaaedc" +
			"e6af48a03bbfd25e8cd0364142"),
		err: ErrSigSTooBig,
	}}

	for _, test := range tests {
		_, err := ParseDERSignature(test.sig)
		if !errors.Is(err, test.err) {
			t.Errorf("%s mismatched err -- got %v, want %v", test.name, err,
				test.err)
			continue
		}
	}
}

// TestSignatureSerialize ensures that serializing signatures works as expected.
func TestSignatureSerialize(t *testing.T) {
	tests := []struct {
		name     string
		ecsig    *Signature
		expected []byte
	}{{
		// signature from bitcoin blockchain tx
		// 0437cd7f8525ceed2324359c2d0ba26006d92d85
		"valid 1 - r and s most significant bits are zero",
		&Signature{
			r: *hexToModNScalar("4e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd41"),
			s: *hexToModNScalar("181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d09"),
		},
		hexToBytes("304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d62" +
			"4c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc" +
			"56cbbac4622082221a8768d1d09"),
	}, {
		// signature from bitcoin blockchain tx
		// cb00f8a0573b18faa8c4f467b049f5d202bf1101d9ef2633bc611be70376a4b4
		"valid 2 - r most significant bit is one",
		&Signature{
			r: *hexToModNScalar("82235e21a2300022738dabb8e1bbd9d19cfb1e7ab8c30a23b0afbb8d178abcf3"),
			s: *hexToModNScalar("24bf68e256c534ddfaf966bf908deb944305596f7bdcc38d69acad7f9c868724"),
		},
		hexToBytes("304502210082235e21a2300022738dabb8e1bbd9d19cfb1e7ab8c" +
			"30a23b0afbb8d178abcf3022024bf68e256c534ddfaf966bf908deb94430" +
			"5596f7bdcc38d69acad7f9c868724"),
	}, {
		// signature from bitcoin blockchain tx
		// fda204502a3345e08afd6af27377c052e77f1fefeaeb31bdd45f1e1237ca5470
		//
		// Note that signatures with an S component that is > half the group
		// order are neither allowed nor produced in Decred, so this has been
		// modified to expect the equally valid low S signature variant.
		"valid 3 - s most significant bit is one",
		&Signature{
			r: *hexToModNScalar("1cadddc2838598fee7dc35a12b340c6bde8b389f7bfd19a1252a17c4b5ed2d71"),
			s: *hexToModNScalar("c1a251bbecb14b058a8bd77f65de87e51c47e95904f4c0e9d52eddc21c1415ac"),
		},
		hexToBytes("304402201cadddc2838598fee7dc35a12b340c6bde8b389f7bfd1" +
			"9a1252a17c4b5ed2d7102203e5dae44134eb4fa757428809a2178199e66f" +
			"38daa53df51eaa380cab4222b95"),
	}, {
		"zero signature",
		&Signature{
			r: *new(secp256k1.ModNScalar).SetInt(0),
			s: *new(secp256k1.ModNScalar).SetInt(0),
		},
		hexToBytes("3006020100020100"),
	}}

	for i, test := range tests {
		result := test.ecsig.Serialize()
		if !bytes.Equal(result, test.expected) {
			t.Errorf("Serialize #%d (%s) unexpected result:\n"+
				"got:  %x\nwant: %x", i, test.name, result,
				test.expected)
		}
	}
}

// testSignCompact creates a recoverable public key signature over the provided
// data by creating a random private key, signing the data, and ensure the
// public key can be recovered.
func testSignCompact(t *testing.T, tag string, data []byte, isCompressed bool) {
	priv, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}
	signingPubKey := priv.PubKey()

	hashed := []byte("testing")
	sig := SignCompact(priv, hashed, isCompressed)

	pk, wasCompressed, err := RecoverCompact(sig, hashed)
	if err != nil {
		t.Errorf("%s: error recovering: %s", tag, err)
		return
	}
	if !pk.IsEqual(signingPubKey) {
		t.Errorf("%s: recovered pubkey doesn't match original "+
			"%x vs %x", tag, pk.SerializeCompressed(),
			signingPubKey.SerializeCompressed())
		return
	}
	if wasCompressed != isCompressed {
		t.Errorf("%s: recovered pubkey doesn't match compressed state "+
			"(%v vs %v)", tag, isCompressed, wasCompressed)
		return
	}

	// If we change the compressed bit we should get the same key back,
	// but the compressed flag should be reversed.
	if isCompressed {
		sig[0] -= 4
	} else {
		sig[0] += 4
	}

	pk, wasCompressed, err = RecoverCompact(sig, hashed)
	if err != nil {
		t.Errorf("%s: error recovering (2): %s", tag, err)
		return
	}
	if !pk.IsEqual(signingPubKey) {
		t.Errorf("%s: recovered pubkey (2) doesn't match original "+
			"%x vs %x", tag, pk.SerializeCompressed(),
			signingPubKey.SerializeCompressed())
		return
	}
	if wasCompressed == isCompressed {
		t.Errorf("%s: recovered pubkey doesn't match reversed "+
			"compressed state (%v vs %v)", tag, isCompressed,
			wasCompressed)
		return
	}
}

// TestSignCompact ensures the public key can be recovered from recoverable
// public key signatures over random data with random private keys.
func TestSignCompact(t *testing.T) {
	for i := 0; i < 256; i++ {
		name := fmt.Sprintf("test %d", i)
		data := make([]byte, 32)
		_, err := rand.Read(data)
		if err != nil {
			t.Errorf("failed to read random data for %s", name)
			continue
		}
		compressed := i%2 != 0
		testSignCompact(t, name, data, compressed)
	}
}

// TestSignatureIsEqual ensures that equality testing between two signatures
// works as expected.
func TestSignatureIsEqual(t *testing.T) {
	sig1 := &Signature{
		r: *hexToModNScalar("82235e21a2300022738dabb8e1bbd9d19cfb1e7ab8c30a23b0afbb8d178abcf3"),
		s: *hexToModNScalar("24bf68e256c534ddfaf966bf908deb944305596f7bdcc38d69acad7f9c868724"),
	}
	sig1Copy := &Signature{
		r: *hexToModNScalar("82235e21a2300022738dabb8e1bbd9d19cfb1e7ab8c30a23b0afbb8d178abcf3"),
		s: *hexToModNScalar("24bf68e256c534ddfaf966bf908deb944305596f7bdcc38d69acad7f9c868724"),
	}
	sig2 := &Signature{
		r: *hexToModNScalar("4e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd41"),
		s: *hexToModNScalar("181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d09"),
	}

	if !sig1.IsEqual(sig1) {
		t.Fatalf("bad self signature equality check: %v == %v", sig1, sig1Copy)
	}
	if !sig1.IsEqual(sig1Copy) {
		t.Fatalf("bad signature equality check: %v == %v", sig1, sig1Copy)
	}

	if sig1.IsEqual(sig2) {
		t.Fatalf("bad signature equality check: %v != %v", sig1, sig2)
	}
}
