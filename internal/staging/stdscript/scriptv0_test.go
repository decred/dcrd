// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdscript

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"
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

// scriptV0Tests houses several version 0 test scripts used to ensure various
// script types and data extraction is working as expected.  It's defined as a
// test global versus inside a specific test function scope since it spans
// multiple tests and benchmarks.
var scriptV0Tests = func() []scriptTest {
	// Convience function that combines fmt.Sprintf with mustParseShortForm
	// to create more compact tests.
	p := func(format string, a ...interface{}) []byte {
		const scriptVersion = 0
		return mustParseShortForm(scriptVersion, fmt.Sprintf(format, a...))
	}

	// ---------------------------------------------------------------------
	// Define some data shared in the tests for convenience.
	// ---------------------------------------------------------------------

	// Uncompressed and compressed/hybrid even/odd secp256k1 public keys.
	pkUE := "0479be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f817" +
		"98483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8"
	pkUO := "04fff97bd5755eeea420453a14355235d382f6472f8568a18b2f057a14602975" +
		"56ae12777aacfbb620f3be96017f45c560de80f0f6518fe4a03c870c36b075f297"
	pkCE := "02" + pkUE[2:66]
	pkCO := "03" + pkUO[2:66]
	pkHE := "05" + pkUE[2:]
	pkHO := "06" + pkUO[2:]

	return []scriptTest{{
		// ---------------------------------------------------------------------
		// Misc negative tests.
		// ---------------------------------------------------------------------

		name:     "malformed v0 script that does not parse",
		script:   p("DATA_5 0x01020304"),
		wantType: STNonStandard,
	}, {
		name:     "empty v0 script",
		script:   nil,
		wantType: STNonStandard,
	}, {
		// ---------------------------------------------------------------------
		// Negative P2PK ECDSA secp256k1 tests.
		// ---------------------------------------------------------------------

		name:     "v0 p2pk-ecdsa-secp256k1 hybrid odd",
		script:   p("DATA_33 0x%s CHECKSIG", pkHO),
		wantType: STNonStandard,
	}, {
		name:     "v0 p2pk-ecdsa-secp256k1 hybrid even",
		script:   p("DATA_33 0x%s CHECKSIG", pkHE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 p2pk-ecdsa-secp256k1 -- trailing opcode",
		script:   p("DATA_33 0x%s CHECKSIG TRUE", pkCE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 p2pk-ecdsa-secp256k1 -- pubkey not pushed",
		script:   p("0x%s CHECKSIG", pkCE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 p2pk-ecdsa-secp256k1 -- malformed pubkey prefix",
		script:   p("DATA_33 0x08%s CHECKSIG", pkCE[2:]),
		wantType: STNonStandard,
	}, {
		// ---------------------------------------------------------------------
		// Positive P2PK ECDSA secp256k1 tests.
		// ---------------------------------------------------------------------

		name:     "v0 p2pk-ecdsa-secp256k1 uncompressed",
		script:   p("DATA_65 0x%s CHECKSIG", pkUE),
		wantType: STPubKeyEcdsaSecp256k1,
		wantData: hexToBytes(pkUE),
	}, {
		name:     "v0 p2pk-ecdsa-secp256k1 compressed even",
		script:   p("DATA_33 0x%s CHECKSIG", pkCE),
		wantType: STPubKeyEcdsaSecp256k1,
		wantData: hexToBytes(pkCE),
	}, {
		name:     "v0 p2pk-ecdsa-secp256k1 compressed odd",
		script:   p("DATA_33 0x%s CHECKSIG", pkCO),
		wantType: STPubKeyEcdsaSecp256k1,
		wantData: hexToBytes(pkCO),
	}}
}()

// asByteSlice attempts to convert the data associated with the passed script
// test to a byte slice or causes a fatal test error.
func asByteSlice(t *testing.T, test scriptTest) []byte {
	t.Helper()

	want, ok := test.wantData.([]byte)
	if !ok {
		t.Fatalf("%q: unexpected want data type -- got %T", test.name,
			test.wantData)
	}
	return want
}

// TestExtractPubKeysV0 ensures that extracting a public key from the various
// version 0 pay-to-pubkey-ecdsa-secp256k1 style scripts works as intended
// for all of the version 0 test scripts.
func TestExtractPubKeysV0(t *testing.T) {
	for _, test := range scriptV0Tests {
		// Determine the expected data based on the expected script type and
		// data specified in the test.
		var want, wantCompressed, wantUncompressed []byte
		if test.wantType == STPubKeyEcdsaSecp256k1 {
			want = asByteSlice(t, test)
			if len(want) == 33 {
				wantCompressed = want
			} else if len(want) == 65 {
				wantUncompressed = want
			}
		}

		testExtract := func(fn func(script []byte) []byte, want []byte) {
			t.Helper()

			got := fn(test.script)
			if !bytes.Equal(got, want) {
				t.Errorf("%q: unexpected pubkey -- got %x, want %x (script %x)",
					test.name, got, want, test.script)
			}
		}
		testExtract(ExtractPubKeyV0, want)
		testExtract(ExtractCompressedPubKeyV0, wantCompressed)
		testExtract(ExtractUncompressedPubKeyV0, wantUncompressed)
	}
}
