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

		// TODO: Remove once used.
		_ = test.wantData
	}
}
