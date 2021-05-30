// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdscript

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/decred/dcrd/dcrec"
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

	// Uncompressed and compressed/hybrid even/odd secp256k1 public keys along
	// with hash160s of the compressed even ones.
	pkUE := "0479be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f817" +
		"98483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8"
	pkUO := "04fff97bd5755eeea420453a14355235d382f6472f8568a18b2f057a14602975" +
		"56ae12777aacfbb620f3be96017f45c560de80f0f6518fe4a03c870c36b075f297"
	pkCE := "02" + pkUE[2:66]
	h160CE := "e280cb6e66b96679aec288b1fbdbd4db08077a1b"
	pkCO := "03" + pkUO[2:66]
	pkHE := "05" + pkUE[2:]
	pkHO := "06" + pkUO[2:]

	// Ed25519 public key and hash.
	pkEd := "cecc1507dc1ddd7295951c290888f095adb9044d1b73d696e6df065d683bd4fc"
	h160Ed := "456d8ee57a4b9121987b4ecab8c3bcb5797e8a53"

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
	}, {
		// ---------------------------------------------------------------------
		// Negative P2PK Alt tests.
		// ---------------------------------------------------------------------

		name:     "v0 p2pk-alt unsupported signature type 0",
		script:   p("DATA_33 0x%s 0 CHECKSIGALT", pkCE),
		wantType: STNonStandard,
	}, {
		name:     "v0 p2pk-alt unsupported signature type 3",
		script:   p("DATA_33 0x%s 3 CHECKSIGALT", pkCE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 p2pk-alt -- signature type not small int",
		script:   p("DATA_33 0x%s DATA_1 2 CHECKSIGALT", pkCE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 p2pk-alt -- NOP for signature type",
		script:   p("DATA_33 0x%s NOP CHECKSIGALT", pkCE),
		wantType: STNonStandard,
	}, {
		// ---------------------------------------------------------------------
		// Negative P2PK Ed25519 tests.
		// ---------------------------------------------------------------------

		name:     "almost v0 p2pk-ed25519 -- trailing opcode",
		script:   p("DATA_32 0x%s 1 CHECKSIGALT TRUE", pkEd),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 p2pk-ed25519 -- pubkey not pushed",
		script:   p("0x%s 1 CHECKSIGALT", pkEd),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 p2pk-ed25519 -- wrong signature type",
		script:   p("DATA_32 0x%s 2 CHECKSIGALT", pkEd),
		wantType: STNonStandard,
	}, {
		// ---------------------------------------------------------------------
		// Positive P2PK Ed25519 tests.
		// ---------------------------------------------------------------------

		name:     "v0 p2pk-ed25519",
		script:   p("DATA_32 0x%s 1 CHECKSIGALT", pkEd),
		wantType: STPubKeyEd25519,
		wantData: hexToBytes(pkEd),
	}, {
		// ---------------------------------------------------------------------
		// Negative P2PK Schnorr secp256k1 tests.
		// ---------------------------------------------------------------------

		name:     "v0 p2pk-schnorr-secp256k1 uncompressed",
		script:   p("DATA_65 0x%s 2 CHECKSIGALT", pkUE),
		wantType: STNonStandard,
	}, {
		name:     "v0 p2pk-schnorr-secp256k1 hybrid odd",
		script:   p("DATA_65 0x%s 2 CHECKSIGALT", pkHO),
		wantType: STNonStandard,
	}, {
		name:     "v0 p2pk-schnorr-secp256k1 hybrid even",
		script:   p("DATA_65 0x%s 2 CHECKSIGALT", pkHE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 p2pk-schnorr-secp256k1 -- trailing opcode",
		script:   p("DATA_33 0x%s 2 CHECKSIGALT TRUE", pkCE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 p2pk-schnorr-secp256k1 -- pubkey not pushed",
		script:   p("0x%s 2 CHECKSIGALT", pkCE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 p2pk-schnorr-secp256k1 -- malformed pubkey prefix",
		script:   p("DATA_33 0x08%s 2 CHECKSIGALT", pkCE[2:]),
		wantType: STNonStandard,
	}, {
		// ---------------------------------------------------------------------
		// Positive P2PK Schnorr secp256k1 tests.
		// ---------------------------------------------------------------------

		name:     "v0 p2pk-schnorr-secp256k1 compressed even",
		script:   p("DATA_33 0x%s 2 CHECKSIGALT", pkCE),
		wantType: STPubKeySchnorrSecp256k1,
		wantData: hexToBytes(pkCE),
	}, {
		name:     "v0 p2pk-schnorr-secp256k1 compressed odd",
		script:   p("DATA_33 0x%s 2 CHECKSIGALT", pkCO),
		wantType: STPubKeySchnorrSecp256k1,
		wantData: hexToBytes(pkCO),
	}, {
		// ---------------------------------------------------------------------
		// Negative P2PKH ECDSA secp256k1 tests.
		// ---------------------------------------------------------------------

		name:     "almost v0 p2pkh-ecdsa-secp256k1 -- wrong hash length",
		script:   p("DUP HASH160 DATA_21 0x00%s EQUALVERIFY CHECKSIG", h160CE),
		wantType: STNonStandard,
	}, {
		// ---------------------------------------------------------------------
		// Positive P2PKH ECDSA secp256k1 tests.
		// ---------------------------------------------------------------------

		name:     "v0 p2pkh-ecdsa-secp256k1",
		script:   p("DUP HASH160 DATA_20 0x%s EQUALVERIFY CHECKSIG", h160CE),
		wantType: STPubKeyHashEcdsaSecp256k1,
		wantData: hexToBytes(h160CE),
	}, {
		// ---------------------------------------------------------------------
		// Negative P2PKH Alt tests.
		// ---------------------------------------------------------------------

		name: "v0 p2pkh-alt unsupported signature type 0",
		script: p("DUP HASH160 DATA_20 0x%s EQUALVERIFY 0 CHECKSIGALT",
			h160CE),
		wantType: STNonStandard,
	}, {
		name: "v0 p2pkh-alt unsupported signature type 3",
		script: p("DUP HASH160 DATA_20 0x%s EQUALVERIFY 3 CHECKSIGALT",
			h160CE),
		wantType: STNonStandard,
	}, {
		name: "almost v0 p2pkh-alt -- signature type not a small int",
		script: p("DUP HASH160 DATA_20 0x%s EQUALVERIFY DATA_1 2 CHECKSIGALT",
			h160CE),
		wantType: STNonStandard,
	}, {
		name: "almost v0 p2pkh-alt -- NOP for signature type",
		script: p("DUP HASH160 DATA_20 0x%s EQUALVERIFY NOP CHECKSIGALT",
			h160CE),
		wantType: STNonStandard,
	}, {
		// ---------------------------------------------------------------------
		// Negative P2PKH Ed25519 tests.
		// ---------------------------------------------------------------------

		name: "almost v0 p2pkh-ed25519 -- wrong hash length",
		script: p("DUP HASH160 DATA_21 0x00%s EQUALVERIFY 1 CHECKSIGALT",
			h160Ed),
		wantType: STNonStandard,
	}, {
		// ---------------------------------------------------------------------
		// Positive P2PKH Ed25519 tests.
		// ---------------------------------------------------------------------

		name: "v0 p2pkh-ed25519",
		script: p("DUP HASH160 DATA_20 0x%s EQUALVERIFY 1 CHECKSIGALT",
			h160Ed),
		wantType: STPubKeyHashEd25519,
		wantData: hexToBytes(h160Ed),
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

// TestExtractPubKeyAltDetailsV0 ensures that extracting a public key and
// signature type from the various version 0 pay-to-alt-pubkey style scripts
// works as intended for all of the version 0 test scripts.
func TestExtractPubKeyAltDetailsV0(t *testing.T) {
	for _, test := range scriptV0Tests {
		// Determine the expected data based on the expected script type and
		// data specified in the test.
		var wantBytes []byte
		var wantSigType dcrec.SignatureType
		switch test.wantType {
		case STPubKeyEd25519:
			wantBytes = asByteSlice(t, test)
			wantSigType = dcrec.STEd25519

		case STPubKeySchnorrSecp256k1:
			wantBytes = asByteSlice(t, test)
			wantSigType = dcrec.STSchnorrSecp256k1
		}

		gotBytes, gotSigType := ExtractPubKeyAltDetailsV0(test.script)
		if !bytes.Equal(gotBytes, wantBytes) {
			t.Errorf("%q: unexpected pubkey -- got %x, want %x", test.name,
				gotBytes, wantBytes)
			continue
		}
		if gotBytes != nil && gotSigType != wantSigType {
			t.Errorf("%q: unexpected sig type -- got %d, want %d", test.name,
				gotSigType, wantSigType)
			continue
		}
	}
}

// TestExtractPubKeyHashV0 ensures that extracting a public key hash from the
// various version 0 pay-to-pubkey-hash-ecdsa-secp256k1 scripts works as
// intended for all of the version 0 test scripts.
func TestExtractPubKeyHashV0(t *testing.T) {
	for _, test := range scriptV0Tests {
		// Determine the expected data based on the expected script type and
		// data specified in the test.
		var want []byte
		if test.wantType == STPubKeyHashEcdsaSecp256k1 {
			want = asByteSlice(t, test)
		}

		got := ExtractPubKeyHashV0(test.script)
		if !bytes.Equal(got, want) {
			t.Errorf("%q: unexpected pubkey hash -- got %x, want %x", test.name,
				got, want)
			continue
		}
	}
}

// TestExtractPubKeyHashAltDetailsV0 ensures that extracting a public key hash
// and signature type from the version 0 pay-to-alt-pubkey-hash style scripts
// works as intended for all of the version 0 test scripts.
func TestExtractPubKeyHashAltDetailsV0(t *testing.T) {
	for _, test := range scriptV0Tests {
		// Determine the expected data based on the expected script type and
		// data specified in the test.
		var wantBytes []byte
		var wantSigType dcrec.SignatureType
		switch test.wantType {
		case STPubKeyHashEd25519:
			wantBytes = asByteSlice(t, test)
			wantSigType = dcrec.STEd25519
		}

		gotBytes, gotSigType := ExtractPubKeyHashAltDetailsV0(test.script)
		if !bytes.Equal(gotBytes, wantBytes) {
			t.Errorf("%q: unexpected pubkey hash -- got %x, want %x", test.name,
				gotBytes, wantBytes)
			continue
		}
		if gotBytes != nil && gotSigType != wantSigType {
			t.Errorf("%q: unexpected sig type -- got %d, want %d", test.name,
				gotSigType, wantSigType)
			continue
		}
	}
}
