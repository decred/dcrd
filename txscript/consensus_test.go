// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"testing"
)

// TestExtractCoinbaseNullData ensures the ExtractCoinbaseNullData function
// produces the expected extracted data under both valid and invalid scenarios.
func TestExtractCoinbaseNullData(t *testing.T) {
	tests := []struct {
		name   string
		script []byte
		valid  bool
		result []byte
	}{{
		name:   "block 2, height only",
		script: mustParseShortForm("RETURN DATA_4 0x02000000"),
		valid:  true,
		result: hexToBytes("02000000"),
	}, {
		name:   "block 2, height and extra nonce data",
		script: mustParseShortForm("RETURN DATA_36 0x02000000000000000000000000000000000000000000000000000000ffa310d9a6a9588e"),
		valid:  true,
		result: hexToBytes("02000000000000000000000000000000000000000000000000000000ffa310d9a6a9588e"),
	}, {
		name:   "block 2, height and reduced extra nonce data",
		script: mustParseShortForm("RETURN DATA_12 0x02000000ffa310d9a6a9588e"),
		valid:  true,
		result: hexToBytes("02000000ffa310d9a6a9588e"),
	}, {
		name:   "no push",
		script: mustParseShortForm("RETURN"),
		valid:  true,
		result: nil,
	}, {
		// Normal nulldata scripts support special handling of small data,
		// however the coinbase nulldata in question does not.
		name:   "small data",
		script: mustParseShortForm("RETURN OP_2"),
		valid:  false,
		result: nil,
	}, {
		name:   "almost correct",
		script: mustParseShortForm("OP_TRUE RETURN DATA_12 0x02000000ffa310d9a6a9588e"),
		valid:  false,
		result: nil,
	}, {
		name:   "almost correct 2",
		script: mustParseShortForm("DATA_12 0x02000000 0xffa310d9a6a9588e"),
		valid:  false,
		result: nil,
	}}

	for _, test := range tests {
		nullData, err := ExtractCoinbaseNullData(test.script)
		if test.valid && err != nil {
			t.Errorf("test '%s' unexpected error: %v", test.name, err)
			continue
		} else if !test.valid && err == nil {
			t.Errorf("test '%s' passed when it should have failed", test.name)
			continue
		}

		if !bytes.Equal(nullData, test.result) {
			t.Errorf("test '%s' mismatched result - got %x, want %x", test.name,
				nullData, test.result)
			continue
		}
	}
}

// TestCheckSignatureEncoding ensures the internal checkSignatureEncoding
// function works as expected.
func TestCheckSignatureEncoding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sig     []byte
		isValid bool
	}{
		{
			name: "valid signature",
			sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: true,
		},
		{
			name:    "empty.",
			sig:     nil,
			isValid: false,
		},
		{
			name: "bad magic",
			sig: hexToBytes("314402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "bad 1st int marker magic",
			sig: hexToBytes("304403204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "bad 2nd int marker",
			sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41032018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "short len",
			sig: hexToBytes("304302204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "long len",
			sig: hexToBytes("304502204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "long X",
			sig: hexToBytes("304402424e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "long Y",
			sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022118152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "short Y",
			sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41021918152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "trailing crap",
			sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d0901"),
			isValid: false,
		},
		{
			name: "X == N",
			sig: hexToBytes("30440220fffffffffffffffffffffffffffff" +
				"ffebaaedce6af48a03bbfd25e8cd0364141022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "X > N",
			sig: hexToBytes("30440220fffffffffffffffffffffffffffff" +
				"ffebaaedce6af48a03bbfd25e8cd0364142022018152" +
				"2ec8eca07de4860a4acdd12909d831cc56cbbac46220" +
				"82221a8768d1d09"),
			isValid: false,
		},
		{
			name: "Y == N",
			sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd410220fffff" +
				"ffffffffffffffffffffffffffebaaedce6af48a03bb" +
				"fd25e8cd0364141"),
			isValid: false,
		},
		{
			name: "Y > N",
			sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd410220fffff" +
				"ffffffffffffffffffffffffffebaaedce6af48a03bb" +
				"fd25e8cd0364142"),
			isValid: false,
		},
		{
			name: "0 len X",
			sig: hexToBytes("302402000220181522ec8eca07de4860a4acd" +
				"d12909d831cc56cbbac4622082221a8768d1d09"),
			isValid: false,
		},
		{
			name: "0 len Y",
			sig: hexToBytes("302402204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd410200"),
			isValid: false,
		},
		{
			name: "extra R padding",
			sig: hexToBytes("30450221004e45e16932b8af514961a1d3a1a" +
				"25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181" +
				"522ec8eca07de4860a4acdd12909d831cc56cbbac462" +
				"2082221a8768d1d09"),
			isValid: false,
		},
		{
			name: "extra S padding",
			sig: hexToBytes("304502204e45e16932b8af514961a1d3a1a25" +
				"fdf3f4f7732e9d624c6c61548ab5fb8cd41022100181" +
				"522ec8eca07de4860a4acdd12909d831cc56cbbac462" +
				"2082221a8768d1d09"),
			isValid: false,
		},
	}

	for _, test := range tests {
		err := CheckSignatureEncoding(test.sig)
		if err != nil && test.isValid {
			t.Errorf("checkSignatureEncoding test '%s' failed "+
				"when it should have succeeded: %v", test.name,
				err)
		} else if err == nil && !test.isValid {
			t.Errorf("checkSignatureEncooding test '%s' succeeded "+
				"when it should have failed", test.name)
		}
	}
}

// TestCheckPubKeyEncoding ensures the internal checkPubKeyEncoding function
// works as expected.
func TestCheckPubKeyEncoding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     []byte
		isValid bool
	}{
		{
			name: "uncompressed ok",
			key: hexToBytes("0411db93e1dcdb8a016b49840f8c53bc1eb68" +
				"a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf" +
				"9744464f82e160bfa9b8b64f9d4c03f999b8643f656b" +
				"412a3"),
			isValid: true,
		},
		{
			name: "compressed ok",
			key: hexToBytes("02ce0b14fb842b1ba549fdd675c98075f12e9" +
				"c510f8ef52bd021a9a1f4809d3b4d"),
			isValid: true,
		},
		{
			name: "compressed ok",
			key: hexToBytes("032689c7c2dab13309fb143e0e8fe39634252" +
				"1887e976690b6b47f5b2a4b7d448e"),
			isValid: true,
		},
		{
			name: "hybrid",
			key: hexToBytes("0679be667ef9dcbbac55a06295ce870b07029" +
				"bfcdb2dce28d959f2815b16f81798483ada7726a3c46" +
				"55da4fbfc0e1108a8fd17b448a68554199c47d08ffb1" +
				"0d4b8"),
			isValid: false,
		},
		{
			name:    "empty",
			key:     nil,
			isValid: false,
		},
	}

	for _, test := range tests {
		err := CheckPubKeyEncoding(test.key)
		if err != nil && test.isValid {
			t.Errorf("checkPubKeyEncoding test '%s' failed when "+
				"it should have succeeded: %v", test.name, err)
		} else if err == nil && !test.isValid {
			t.Errorf("checkPubKeyEncoding test '%s' succeeded "+
				"when it should have failed", test.name)
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
