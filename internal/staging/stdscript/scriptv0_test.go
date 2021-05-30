// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdscript

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"reflect"
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
	pkCE2 := "02f9308a019258c31049344f85f89d5229b531c845836f99b08601f113bce036f9"
	h160CE2 := "01557763e0252dc0ff9e0996ad1d04b167bb993c"
	pkCO := "03" + pkUO[2:66]
	pkHE := "05" + pkUE[2:]
	pkHO := "06" + pkUO[2:]

	// Ed25519 public key and hash.
	pkEd := "cecc1507dc1ddd7295951c290888f095adb9044d1b73d696e6df065d683bd4fc"
	h160Ed := "456d8ee57a4b9121987b4ecab8c3bcb5797e8a53"

	// Script hash for a 2-of-3 multisig composed of pkCE, pkCE2, and pkCO.
	p2sh := "f86b5a7c6d32566aa4dccc04d1533530b4d64cf3"

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
	}, {
		// ---------------------------------------------------------------------
		// Negative P2PKH Schnorr secp256k1 tests.
		// ---------------------------------------------------------------------

		name: "almost v0 p2pkh-schnorr-secp256k1 -- wrong hash length",
		script: p("DUP HASH160 DATA_21 0x00%s EQUALVERIFY 2 CHECKSIGALT",
			h160CE),
		wantType: STNonStandard,
	}, {
		// ---------------------------------------------------------------------
		// Positive P2PKH Schnorr secp256k1 tests.
		// ---------------------------------------------------------------------

		name: "v0 p2pkh-schnorr-secp256k1",
		script: p("DUP HASH160 DATA_20 0x%s EQUALVERIFY 2 CHECKSIGALT",
			h160CE),
		wantType: STPubKeyHashSchnorrSecp256k1,
		wantData: hexToBytes(h160CE),
	}, {
		name: "v0 p2pkh-schnorr-secp256k1 2",
		script: p("DUP HASH160 DATA_20 0x%s EQUALVERIFY 2 CHECKSIGALT",
			h160CE2),
		wantType: STPubKeyHashSchnorrSecp256k1,
		wantData: hexToBytes(h160CE2),
	}, {
		// ---------------------------------------------------------------------
		// Negative P2SH tests.
		// ---------------------------------------------------------------------

		name:     "almost v0 p2sh -- wrong hash length",
		script:   p("HASH160 DATA_21 0x00%s EQUAL", p2sh),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 p2sh -- trailing opcode",
		script:   p("HASH160 DATA_20 0x%s EQUAL TRUE", p2sh),
		wantType: STNonStandard,
	}, {
		// ---------------------------------------------------------------------
		// Positive P2SH tests.
		// ---------------------------------------------------------------------

		name:     "v0 p2sh",
		script:   p("HASH160 DATA_20 0x%s EQUAL", p2sh),
		wantType: STScriptHash,
		wantData: hexToBytes(p2sh),
	}, {
		// ---------------------------------------------------------------------
		// Negative ECDSA multisig secp256k1 tests.
		// ---------------------------------------------------------------------

		name:     "almost v0 multisig 1-of-2 -- mixed (un)compressed pubkeys",
		script:   p("1 DATA_65 0x%s DATA_33 0x%s 2 CHECKMULTISIG", pkUE, pkCO),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig -- no req sigs",
		script:   p("0 0 CHECKMULTISIG"),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig -- invalid pubkey",
		script:   p("1 DATA_32 0x%s 1 CHECKMULTISIG", pkCE[2:]),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig -- hybrid pubkey",
		script:   p("1 DATA_65 0x%s 1 CHECKMULTISIG", pkHO),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig -- invalid number of signatures",
		script:   p("DUP DATA_33 0x%s 1 CHECKMULTISIG", pkCE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig -- ends with CHECKSIG instead",
		script:   p("1 DATA_33 0x%s 1 CHECKSIG", pkCE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig -- num required sigs not small int",
		script:   p("DATA_1 1 DATA_33 0x%s 1 CHECKMULTISIG", pkCE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig -- num public keys not small int",
		script:   p("1 DATA_33 0x%s DATA_1 1 CHECKMULTISIG", pkCE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig -- missing num public keys",
		script:   p("1 DATA_33 0x%s CHECKMULTISIG", pkCE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig -- num pubkeys does not match given keys",
		script:   p("2 DATA_33 0x%s DATA_33 0x%s 3 CHECKMULTISIG", pkCE, pkCO),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig -- fewer pubkeys than num required sigs",
		script:   p("1 0 CHECKMULTISIG"),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig -- CHECKMULTISIGVERIFY",
		script:   p("1 DATA_33 0x%s 1 CHECKMULTISIGVERIFY", pkCE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig -- extra NOP prior to final opcode",
		script:   p("1 DATA_33 0x%s 1 NOP CHECKMULTISIG", pkCE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig -- trailing opcode",
		script:   p("1 DATA_33 0x%s 1 CHECKMULTISIG TRUE", pkCE),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig -- no pubkeys specified",
		script:   p("1 CHECKMULTISIG"),
		wantType: STNonStandard,
	}, {
		// ---------------------------------------------------------------------
		// Positive ECDSA multisig secp256k1 tests.
		// ---------------------------------------------------------------------

		name:     "v0 multisig 1-of-1 compressed pubkey",
		script:   p("1 DATA_33 0x%s 1 CHECKMULTISIG", pkCE),
		wantType: STMultiSig,
		wantData: MultiSigDetailsV0{
			RequiredSigs: 1,
			NumPubKeys:   1,
			PubKeys:      [][]byte{hexToBytes(pkCE)},
			Valid:        true,
		},
	}, {
		name:     "v0 multisig 1-of-2 compressed pubkeys",
		script:   p("1 DATA_33 0x%s DATA_33 0x%s 2 CHECKMULTISIG", pkCE, pkCE2),
		wantType: STMultiSig,
		wantData: MultiSigDetailsV0{
			RequiredSigs: 1,
			NumPubKeys:   2,
			PubKeys:      [][]byte{hexToBytes(pkCE), hexToBytes(pkCE2)},
			Valid:        true,
		},
	}, {
		name: "v0 multisig 2-of-3 compressed pubkeys",
		script: p("2 DATA_33 0x%s DATA_33 0x%s DATA_33 0x%s 3 CHECKMULTISIG",
			pkCE, pkCE2, pkCO),
		wantType: STMultiSig,
		wantData: MultiSigDetailsV0{
			RequiredSigs: 2,
			NumPubKeys:   3,
			PubKeys: [][]byte{
				hexToBytes(pkCE), hexToBytes(pkCE2), hexToBytes(pkCO),
			},
			Valid: true,
		},
	}, {
		// ---------------------------------------------------------------------
		// Negative ECDSA multisig secp256k1 redeem script tests.
		// ---------------------------------------------------------------------

		name:     "almost v0 multisig redeem script -- no req sigs",
		script:   p("DATA_3 0 0 CHECKMULTISIG"),
		isSig:    true,
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig 1-of-1 redeem script -- trailing opcode",
		script:   p("DATA_38 1 DATA_33 0x%s 1 CHECKMULTISIG TRUE", pkCE),
		isSig:    true,
		wantType: STNonStandard,
	}, {
		name:     "almost v0 multisig 1-of-1 redeem script -- parse error",
		script:   p("DATA_38 1 DATA_33 0x%s 1 CHECKMULTISIG", pkCE),
		isSig:    true,
		wantType: STNonStandard,
	}, {
		// ---------------------------------------------------------------------
		// Positive ECDSA multisig secp256k1 redeem script tests.
		// ---------------------------------------------------------------------

		name:     "v0 multisig 1-of-1 compressed pubkey redeem script",
		script:   p("DATA_37 1 DATA_33 0x%s 1 CHECKMULTISIG", pkCE),
		isSig:    true,
		wantType: STMultiSig,
		wantData: p("1 DATA_33 0x%s 1 CHECKMULTISIG", pkCE),
	}, {
		name: "v0 multisig 1-of-2 compressed pubkeys redeem script",
		script: p("DATA_71 1 DATA_33 0x%s DATA_33 0x%s 2 CHECKMULTISIG", pkCE,
			pkCE2),
		isSig:    true,
		wantType: STMultiSig,
		wantData: p("1 DATA_33 0x%s DATA_33 0x%s 2 CHECKMULTISIG", pkCE, pkCE2),
	}, {
		// ---------------------------------------------------------------------
		// Negative nulldata tests.
		// ---------------------------------------------------------------------

		name:     "almost v0 nulldata -- NOP instead of data push",
		script:   p("RETURN NOP"),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 nulldata -- non-canonical small int push (DATA_1 vs 12)",
		script:   p("RETURN DATA_1 0x0c"),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 nulldata -- non-canonical small int push (PUSHDATA1 vs 12)",
		script:   p("RETURN PUSHDATA1 0x01 0x0c"),
		wantType: STNonStandard,
	}, {
		name: "almost v0 nulldata -- non-canonical 60-byte push (PUSHDATA1 vs DATA_60)",
		script: p("RETURN PUSHDATA1 0x3c 0x046708afdb0fe5548271967f1a67130b7105" +
			"cd6a828e03909a67962e0ea1f61deb649f6bc3f4cef3046708afdb0fe5548271" +
			"967f1a67130b7105cd6a"),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 nulldata -- non-canonical 12-byte push (PUSHDATA2)",
		script:   p("RETURN PUSHDATA2 0x0c00 0x046708afdb0fe5548271967f"),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 nulldata -- non-canonical 12-byte push (PUSHDATA4)",
		script:   p("RETURN PUSHDATA4 0x0c000000 0x046708afdb0fe5548271967f"),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 nulldata -- exceeds max standard push",
		script:   p("RETURN PUSHDATA2 0x0101 0x01{257}"),
		wantType: STNonStandard,
	}, {
		name:     "almost v0 nulldata -- trailing opcode",
		script:   p("RETURN 4 TRUE"),
		wantType: STNonStandard,
	}, {
		// ---------------------------------------------------------------------
		// Positive nulldata tests.
		// ---------------------------------------------------------------------

		name:     "v0 nulldata no data push",
		script:   p("RETURN"),
		wantType: STNullData,
	}, {
		name:     "v0 nulldata single zero push",
		script:   p("RETURN 0"),
		wantType: STNullData,
	}, {
		name:     "v0 nulldata small int push",
		script:   p("RETURN 1"),
		wantType: STNullData,
	}, {
		name:     "v0 nulldata max small int push",
		script:   p("RETURN 16"),
		wantType: STNullData,
	}, {
		name:     "v0 nulldata small data push",
		script:   p("RETURN DATA_8 0x046708afdb0fe554"),
		wantType: STNullData,
	}, {
		name: "v0 nulldata 60-byte push",
		script: p("RETURN 0x3c 0x046708afdb0fe5548271967f1a67130b7105cd6a828e03" +
			"909a67962e0ea1f61deb649f6bc3f4cef3046708afdb0fe5548271967f1a6713" +
			"0b7105cd6a"),
		wantType: STNullData,
	}, {
		name:     "v0 nulldata max standard push",
		script:   p("RETURN PUSHDATA2 0x0001 0x01{256}"),
		wantType: STNullData,
	}, {
		// ---------------------------------------------------------------------
		// Negative stake submission P2PKH tests.
		// ---------------------------------------------------------------------

		name: "almost v0 stake sub p2pkh-ecdsa-secp256k1 -- wrong hash length",
		script: p("SSTX DUP HASH160 DATA_21 0x00%s EQUALVERIFY CHECKSIG",
			h160CE),
		wantType: STNonStandard,
	}, {
		// ---------------------------------------------------------------------
		// Positive stake submission P2PKH tests.
		// ---------------------------------------------------------------------

		name:     "v0 stake submission p2pkh-ecdsa-secp256k1",
		script:   p("SSTX DUP HASH160 DATA_20 0x%s EQUALVERIFY CHECKSIG", h160CE),
		wantType: STStakeSubmissionPubKeyHash,
		wantData: hexToBytes(h160CE),
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

		case STPubKeyHashSchnorrSecp256k1:
			wantBytes = asByteSlice(t, test)
			wantSigType = dcrec.STSchnorrSecp256k1
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

// TestExtractScriptHashV0 ensures that extracting a script hash from the
// various version 0 pay-to-script-hash scripts works as intended for all of the
// version 0 test scripts.
func TestExtractScriptHashV0(t *testing.T) {
	for _, test := range scriptV0Tests {
		// Determine the expected data based on the expected script type and
		// data specified in the test.
		var want []byte
		if test.wantType == STScriptHash {
			want = asByteSlice(t, test)
		}

		got := ExtractScriptHashV0(test.script)
		if !bytes.Equal(got, want) {
			t.Errorf("%q: unexpected script hash -- got %x, want %x", test.name,
				got, want)
			continue
		}
	}
}

// TestExtractMultiSigScriptDetailsV0 ensures that extracting details about a
// version 0 ECDSA multisignature script works as intended for all of the
// version 0 test scripts.
func TestExtractMultiSigScriptDetailsV0(t *testing.T) {
	for _, test := range scriptV0Tests {
		// Determine the expected data based on the expected script type and
		// data specified in the test.
		var want MultiSigDetailsV0
		if test.wantType == STMultiSig && !test.isSig {
			var ok bool
			want, ok = test.wantData.(MultiSigDetailsV0)
			if !ok {
				t.Fatalf("%q: unexpected want data type -- got %T", test.name,
					test.wantData)
			}
		}

		// Attempt to extract the multisig data from the script and ensure
		// the individual fields of the extracted data is accurate.
		got := ExtractMultiSigScriptDetailsV0(test.script, true)
		if got.Valid != want.Valid {
			t.Errorf("%q: unexpected validity -- got %v, want %v", test.name,
				got.Valid, want.Valid)
			continue
		}
		if got.RequiredSigs != want.RequiredSigs {
			t.Errorf("%q: unexpected required sigs -- got %d, want %d",
				test.name, got.RequiredSigs, want.RequiredSigs)
			continue
		}
		if got.NumPubKeys != want.NumPubKeys {
			t.Errorf("%q: unexpected num public keys -- got %d, want %d",
				test.name, got.NumPubKeys, want.NumPubKeys)
			continue
		}
		if !reflect.DeepEqual(got.PubKeys, want.PubKeys) {
			t.Errorf("%q: unexpected extracted pubkeys -- got %x, want %x",
				test.name, got.PubKeys, want.PubKeys)
			continue
		}
	}
}

// TestMultiSigRedeemScriptFromScriptSigV0 ensures extracting a version 0 ECDSA
// multisignature redeem script returns the expected scripts for the version 0
// test scripts that are actually multisignature redeem scripts.
func TestMultiSigRedeemScriptFromScriptSigV0(t *testing.T) {
	// Add an additional test to ensure empty redeem scripts are handled
	// correctly.
	tests := []scriptTest{{
		name:     "v0 empty script",
		script:   nil,
		isSig:    true,
		wantData: []byte(nil),
	}}
	for _, test := range scriptV0Tests {
		// Per the documentation, unlike most of the extraction funcs, the
		// multisig redeem script extraction function is only valid for scripts
		// that have already been determined to be of the correct form.
		if test.wantType != STMultiSig || !test.isSig {
			continue
		}

		tests = append(tests, test)
	}

	for _, test := range tests {
		want := asByteSlice(t, test)
		got := MultiSigRedeemScriptFromScriptSigV0(test.script)
		if !bytes.Equal(got, want) {
			t.Errorf("%q: unexpected redeem script -- got %x, want %x",
				test.name, got, want)
			continue
		}
	}
}

// TestExtractStakeSubmissionPubKeyHashV0 ensures that extracting a public key
// hash from a version 0 stake submission pay-to-pubkey-hash script works as
// intended for all of the version 0 test scripts.
func TestExtractStakeSubmissionPubKeyHashV0(t *testing.T) {
	for _, test := range scriptV0Tests {
		// Determine the expected data based on the expected script type and
		// data specified in the test.
		var want []byte
		if test.wantType == STStakeSubmissionPubKeyHash {
			want = asByteSlice(t, test)
		}

		got := ExtractStakeSubmissionPubKeyHashV0(test.script)
		if !bytes.Equal(got, want) {
			t.Errorf("%q: unexpected pubkey hash -- got %x, want %x", test.name,
				got, want)
			continue
		}
	}
}
