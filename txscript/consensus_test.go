// Copyright (c) 2018-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"errors"
	"testing"
)

// TestCheckSignatureEncoding ensures that checking strict signature encoding
// works as expected.
func TestCheckSignatureEncoding(t *testing.T) {
	t.Parallel()

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
		// Although signatures with R == 0 will ultimately be invalid, it is
		// considered a valid encoding from the standpoint of preventing
		// malleability.
		name: "R == 0",
		sig: hexToBytes("30250201000220181522ec8eca07de4860a4acdd12909d831cc5" +
			"6cbbac4622082221a8768d1d09"),
		err: nil,
	}, {
		// Although signatures with R == N will ultimately be invalid, it is
		// considered a valid encoding from the standpoint of preventing
		// malleability.
		name: "R == N",
		sig: hexToBytes("3045022100fffffffffffffffffffffffffffffffebaaedce6af" +
			"48a03bbfd25e8cd03641410220181522ec8eca07de4860a4acdd12909d831cc5" +
			"6cbbac4622082221a8768d1d09"),
		err: nil,
	}, {
		// Although signatures with R > N will ultimately be invalid, it is
		// considered a valid encoding from the standpoint of preventing
		// malleability.
		name: "R > N (>32 bytes)",
		sig: hexToBytes("3045022101cd496f2ab4fe124f977ffe3caa09f756283910fc1a" +
			"96f60ee6873e88d3cfe1d50220181522ec8eca07de4860a4acdd12909d831cc5" +
			"6cbbac4622082221a8768d1d09"),
		err: nil,
	}, {
		// Although signatures with R > N will ultimately be invalid, it is
		// considered a valid encoding from the standpoint of preventing
		// malleability.
		name: "R > N",
		sig: hexToBytes("3045022100fffffffffffffffffffffffffffffffebaaedce6af" +
			"48a03bbfd25e8cd03641420220181522ec8eca07de4860a4acdd12909d831cc5" +
			"6cbbac4622082221a8768d1d09"),
		err: nil,
	}, {
		// Although signatures with S == 0 will ultimately be invalid, it is
		// considered a valid encoding from the standpoint of preventing
		// malleability.
		name: "S == 0",
		sig: hexToBytes("302502204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd41020100"),
		err: nil,
	}, {
		name: "S > N/2 (half order)",
		sig: hexToBytes("304602210080e256f8a9df823ff0322c5515fc4d4538d65a3785" +
			"fb6dd1b448af216864318d022100cfbf242e941d77555bd79fadd3d23b49d3ca" +
			"929459fa114247e55ff8b4fcf832"),
		err: ErrSigHighS,
	}, {
		name: "S == N",
		sig: hexToBytes("304502204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd41022100fffffffffffffffffffffffffffffffebaaedc" +
			"e6af48a03bbfd25e8cd0364141"),
		err: ErrSigHighS,
	}, {
		name: "S > N (>32 bytes)",
		sig: hexToBytes("304502204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd4102210113500a0510b5094bff220c74656879b784b246" +
			"ba89c0a07bc49bcf05d8993d44"),
		err: ErrSigHighS,
	}, {
		name: "S > N",
		sig: hexToBytes("304502204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd41022100fffffffffffffffffffffffffffffffebaaedc" +
			"e6af48a03bbfd25e8cd0364142"),
		err: ErrSigHighS,
	}}

	for _, test := range tests {
		err := CheckSignatureEncoding(test.sig)
		if !errors.Is(err, test.err) {
			t.Errorf("%s mismatched err -- got %v, want %v", test.name, err,
				test.err)
		}
	}
}

// TestCheckPubKeyEncoding ensures that checking strict public key encoding
// works as expected.
func TestCheckPubKeyEncoding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  []byte
		err  error
	}{{
		name: "uncompressed ok",
		key: hexToBytes("04" +
			"11db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c" +
			"b2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3"),
		err: nil,
	}, {
		name: "compressed ok (ybit = 0)",
		key: hexToBytes("02" +
			"ce0b14fb842b1ba549fdd675c98075f12e9c510f8ef52bd021a9a1f4809d3b4d"),
		err: nil,
	}, {
		name: "compressed ok (ybit = 1)",
		key: hexToBytes("03" +
			"2689c7c2dab13309fb143e0e8fe396342521887e976690b6b47f5b2a4b7d448e"),
		err: nil,
	}, {
		// Although public keys not on the curve will ultimately be invalid, it
		// is considered a valid encoding from the standpoint of preventing
		// malleability.
		name: "uncompressed x changed (not on curve)",
		key: hexToBytes("04" +
			"15db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c" +
			"b2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3"),
		err: nil,
	}, {
		// Although public keys not on the curve will ultimately be invalid, it
		// is considered a valid encoding from the standpoint of preventing
		// malleability.
		name: "uncompressed y changed (not on curve)",
		key: hexToBytes("04" +
			"11db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c" +
			"b2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a4"),
		err: nil,
	}, {
		name: "empty rejected",
		key:  nil,
		err:  ErrPubKeyType,
	}, {
		name: "hybrid rejected (ybit = 0)",
		key: hexToBytes("06" +
			"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798" +
			"483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8"),
		err: ErrPubKeyType,
	}, {
		name: "hybrid rejected (ybit = 1)",
		key: hexToBytes("07" +
			"11db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c" +
			"b2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3"),
		err: ErrPubKeyType,
	}, {
		name: "uncompressed claims compressed rejected",
		key: hexToBytes("03" +
			"11db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c" +
			"b2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3"),
		err: ErrPubKeyType,
	}, {
		name: "compressed claims uncompressed rejected (ybit = 0)",
		key: hexToBytes("04" +
			"ce0b14fb842b1ba549fdd675c98075f12e9c510f8ef52bd021a9a1f4809d3b4d"),
		err: ErrPubKeyType,
	}, {
		name: "compressed claims uncompressed rejected (ybit = 1)",
		key: hexToBytes("04" +
			"2689c7c2dab13309fb143e0e8fe396342521887e976690b6b47f5b2a4b7d448e"),
		err: ErrPubKeyType,
	}, {
		name: "compressed claims hybrid rejected (ybit = 0)",
		key: hexToBytes("06" +
			"ce0b14fb842b1ba549fdd675c98075f12e9c510f8ef52bd021a9a1f4809d3b4d"),
		err: ErrPubKeyType,
	}, {
		name: "compressed claims hybrid rejected (ybit = 1)",
		key: hexToBytes("07" +
			"2689c7c2dab13309fb143e0e8fe396342521887e976690b6b47f5b2a4b7d448e"),
		err: ErrPubKeyType,
	}}

	for _, test := range tests {
		err := CheckPubKeyEncoding(test.key)
		if !errors.Is(err, test.err) {
			t.Errorf("%s mismatched err -- got %v, want %v", test.name, err,
				test.err)
		}
	}
}

// TestIsStrictNullData ensures the function that deals with strict null data
// requirements works as expected.
func TestIsStrictNullData(t *testing.T) {
	tests := []struct {
		name        string
		scriptVer   uint16
		script      []byte
		requiredLen uint32
		want        bool
	}{{
		name:        "empty (bare OP_RETURN), req len 0",
		scriptVer:   0,
		script:      mustParseShortForm("RETURN"),
		requiredLen: 0,
		want:        true,
	}, {
		name:        "empty (bare OP_RETURN), req len 0, unsupported script ver",
		scriptVer:   65535,
		script:      mustParseShortForm("RETURN"),
		requiredLen: 0,
		want:        false,
	}, {
		name:        "small int push 0, req len 1",
		scriptVer:   0,
		script:      mustParseShortForm("RETURN 0"),
		requiredLen: 1,
		want:        true,
	}, {
		name:        "small int push 0, req len 1, unsupported script ver",
		scriptVer:   65535,
		script:      mustParseShortForm("RETURN 0"),
		requiredLen: 1,
		want:        false,
	}, {
		name:        "non-canonical small int push 0, req len 1",
		scriptVer:   0,
		script:      mustParseShortForm("RETURN DATA_1 0x00"),
		requiredLen: 1,
		want:        false,
	}, {
		name:        "small int push 1, req len 1",
		scriptVer:   0,
		script:      mustParseShortForm("RETURN 1"),
		requiredLen: 1,
		want:        true,
	}, {
		name:        "small int push 1, req len 1, unsupported script ver",
		scriptVer:   65535,
		script:      mustParseShortForm("RETURN 1"),
		requiredLen: 1,
		want:        false,
	}, {
		name:        "small int push 16, req len 1",
		scriptVer:   0,
		script:      mustParseShortForm("RETURN 16"),
		requiredLen: 1,
		want:        true,
	}, {
		name:        "small int push 16, req len 1, unsupported script ver",
		scriptVer:   65535,
		script:      mustParseShortForm("RETURN 16"),
		requiredLen: 1,
		want:        false,
	}, {
		name:        "small int push 0, req len 2",
		scriptVer:   0,
		script:      mustParseShortForm("RETURN 0"),
		requiredLen: 2,
		want:        false,
	}, {
		name:        "small int push 1, req len 2",
		scriptVer:   0,
		script:      mustParseShortForm("RETURN 1"),
		requiredLen: 2,
		want:        false,
	}, {
		name:        "small int push 16, req len 2",
		scriptVer:   0,
		script:      mustParseShortForm("RETURN 16"),
		requiredLen: 2,
		want:        false,
	}, {
		name:      "32-byte push, req len 32",
		scriptVer: 0,
		script: mustParseShortForm("RETURN DATA_32 0x0102030405060708090a0b0c" +
			"0d0e0f101112131415161718191a1b1c1d1e1f20"),
		requiredLen: 32,
		want:        true,
	}, {
		name:      "32-byte push, req len 32, unsupported script ver",
		scriptVer: 65535,
		script: mustParseShortForm("RETURN DATA_32 0x0102030405060708090a0b0c" +
			"0d0e0f101112131415161718191a1b1c1d1e1f20"),
		requiredLen: 32,
		want:        false,
	}, {
		name:      "32-byte push, req len 31",
		scriptVer: 0,
		script: mustParseShortForm("RETURN DATA_32 0x0102030405060708090a0b0c" +
			"0d0e0f101112131415161718191a1b1c1d1e1f20"),
		requiredLen: 31,
		want:        false,
	}, {
		name:      "32-byte push, req len 33",
		scriptVer: 0,
		script: mustParseShortForm("RETURN DATA_32 0x0102030405060708090a0b0c" +
			"0d0e0f101112131415161718191a1b1c1d1e1f20"),
		requiredLen: 33,
		want:        false,
	}, {
		name:      "32-byte push, req len 32, no leading OP_RETURN",
		scriptVer: 0,
		script: mustParseShortForm("DATA_32 0x0102030405060708090a0b0c0d0e0f" +
			"101112131415161718191a1b1c1d1e1f20"),
		requiredLen: 32,
		want:        false,
	}, {
		name:      "non-canonical 32-byte push via PUSHDATA1, req len 32",
		scriptVer: 0,
		script: mustParseShortForm("RETURN PUSHDATA1 0x20 0x0102030405060708" +
			"090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"),
		requiredLen: 32,
		want:        false,
	}, {
		name:      "non-canonical 32-byte push via PUSHDATA2, req len 32",
		scriptVer: 0,
		script: mustParseShortForm("RETURN PUSHDATA2 0x2000 0x01020304050607" +
			"08090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"),
		requiredLen: 32,
		want:        false,
	}, {
		name:      "non-canonical 32-byte push via PUSHDATA4, req len 32",
		scriptVer: 0,
		script: mustParseShortForm("RETURN PUSHDATA4 0x20000000 0x0102030405" +
			"060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"),
		requiredLen: 32,
		want:        false,
	}, {
		name:      "76-byte push, req len 76",
		scriptVer: 0,
		script: mustParseShortForm("RETURN PUSHDATA1 0x4c 0x0102030405060708" +
			"090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728" +
			"292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f404142434445464748" +
			"494a4b4c"),
		requiredLen: 76,
		want:        false,
	}}

	for _, test := range tests {
		// Ensure the test data scripts are well formed.
		if err := checkScriptParses(0, test.script); err != nil {
			t.Errorf("%s: unexpected script parse failure: %v", test.name, err)
			continue
		}

		// Ensure the result is as expected.
		got := IsStrictNullData(test.scriptVer, test.script, test.requiredLen)
		if got != test.want {
			t.Errorf("%s: mismatched result -- got %v, want %v", test.name, got,
				test.want)
		}
	}
}
