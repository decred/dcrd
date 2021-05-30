// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdscript

import (
	"testing"
)

// TestScriptTypeStringer tests the stringized output for the ScriptType type.
func TestScriptTypeStringer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   ScriptType
		want string
	}{
		{STNonStandard, "nonstandard"},
		{STPubKeyEcdsaSecp256k1, "pubkey"},
		{STPubKeyEd25519, "pubkey-ed25519"},
		{STPubKeySchnorrSecp256k1, "pubkey-schnorr-secp256k1"},
		{STPubKeyHashEcdsaSecp256k1, "pubkeyhash"},
		{STPubKeyHashEd25519, "pubkeyhash-ed25519"},
		{STPubKeyHashSchnorrSecp256k1, "pubkeyhash-schnorr-secp256k1"},
		{STScriptHash, "scripthash"},
		{STMultiSig, "multisig"},
		{STNullData, "nulldata"},
		{0xff, "invalid"},
	}

	// Detect additional error codes that don't have the stringer added.
	if len(tests)-1 != int(numScriptTypes) {
		t.Error("It appears a script type was added without adding an " +
			"associated stringer test")
	}

	for _, test := range tests {
		result := test.in.String()
		if result != test.want {
			t.Errorf("%q: unexpected string -- got: %s", test.want, result)
			continue
		}
	}
}

// scriptTest describes tests for scripts that are used to ensure various script
// types and data extraction is working as expected.  It's defined separately
// since it is intended for use in multiple shared per-version tests.
type scriptTest struct {
	name     string      // test description
	version  uint16      // version of script to analyze
	script   []byte      // script to analyze
	isSig    bool        // treat as a signature script instead
	wantType ScriptType  // expected script type
	wantData interface{} // expected extracted data when applicable
}

// TestDetermineScriptType ensures a wide variety of scripts for various script
// versions are identified with the expected script type.
func TestDetermineScriptType(t *testing.T) {
	t.Parallel()

	// Specify the per-version tests to include in the overall tests here.
	// This is done to make it easy to add independent tests for new script
	// versions while still testing them all through the API that accepts a
	// specific version versus the exported variant that is specific to a given
	// version per its exported name.
	//
	// NOTE: Maintainers should add tests for new script versions following the
	// way scriptV0Tests is handled and add the resulting per-version tests
	// here.
	perVersionTests := [][]scriptTest{
		scriptV0Tests,
	}

	// Flatten all of the per-version tests into a single set of tests.
	var tests []scriptTest
	for _, bundle := range perVersionTests {
		tests = append(tests, bundle...)
	}

	for _, test := range tests {
		// Ensure that the script is considered non standard for unsupported
		// script versions regardless.
		const unsupportedScriptVer = 9999
		gotType := DetermineScriptType(unsupportedScriptVer, test.script)
		if gotType != STNonStandard {
			t.Errorf("%q -- unsupported script version: mismatched type -- "+
				"got %s, want %s (script %x)", test.name, gotType,
				STNonStandard, test.script)
			continue
		}

		// Ensure the overall type determination produces the expected result.
		gotType = DetermineScriptType(test.version, test.script)
		if !test.isSig && gotType != test.wantType {
			t.Errorf("%q: mismatched type -- got %s, want %s (script %x)",
				test.name, gotType, test.wantType, test.script)
			continue
		}

		// These are convenience helpers to ensure a given individual script
		// type determination function matches the expected result per the given
		// desired type.
		type isXFn func(scriptVersion uint16, script []byte) bool
		testIsXInternal := func(fn isXFn, isSig bool, wantType ScriptType) {
			t.Helper()

			// Ensure that the script is considered non standard for unsupported
			// script versions regardless.
			got := fn(unsupportedScriptVer, test.script)
			if got {
				t.Errorf("%q -- unsupported script version return true "+
					"(script %x)", test.name, test.script)
				return
			}

			got = fn(test.version, test.script)
			want := test.isSig == isSig && test.wantType == wantType
			if got != want {
				t.Errorf("%q: mismatched is func %v -- got %v, want %v "+
					"(script %x)", test.name, wantType, got, want, test.script)
			}
		}
		testIsX := func(fn isXFn, wantType ScriptType) {
			t.Helper()
			testIsXInternal(fn, false, wantType)
		}
		testIsSigX := func(fn isXFn, wantType ScriptType) {
			t.Helper()
			testIsXInternal(fn, true, wantType)
		}

		// Ensure the individual determination methods produce the expected
		// results.
		testIsX(IsPubKeyScript, STPubKeyEcdsaSecp256k1)
		testIsX(IsPubKeyEd25519Script, STPubKeyEd25519)
		testIsX(IsPubKeySchnorrSecp256k1Script, STPubKeySchnorrSecp256k1)
		testIsX(IsPubKeyHashScript, STPubKeyHashEcdsaSecp256k1)
		testIsX(IsPubKeyHashEd25519Script, STPubKeyHashEd25519)
		testIsX(IsPubKeyHashSchnorrSecp256k1Script, STPubKeyHashSchnorrSecp256k1)
		testIsX(IsScriptHashScript, STScriptHash)
		testIsX(IsMultiSigScript, STMultiSig)
		testIsX(IsNullDataScript, STNullData)

		// Ensure the special case of determining if a signature script appears
		// to be a signature script which consists of a pay-to-script-hash
		// multi-signature redeem script.
		testIsSigX(IsMultiSigSigScript, STMultiSig)
	}
}
