// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdscript

import (
	"reflect"
	"testing"

	"github.com/decred/dcrd/txscript/v4/stdaddr"
)

// addressTest describes tests for scripts that are used to ensure various
// script types and address extraction is working as expected.  It's defined
// separately since it is intended for use in multiple shared per-version tests.
type addressTest struct {
	name      string                // test description
	version   uint16                // version of script to analyze
	script    []byte                // script to analyze
	params    stdaddr.AddressParams // params for network
	wantType  ScriptType            // expected script type
	wantAddrs []string              // expected extracted addresses
}

// TestExtractAddrs ensures a wide variety of scripts for various script
// versions return the expected extracted addresses.
func TestExtractAddrs(t *testing.T) {
	t.Parallel()

	// Specify the per-version tests to include in the overall tests here.
	// This is done to make it easy to add independent tests for new script
	// versions while still testing them all through the API that accepts a
	// specific version versus the exported variant that is specific to a given
	// version per its exported name.
	//
	// NOTE: Maintainers should add tests for new script versions following the
	// way addressV0Tests is handled and add the resulting per-version tests
	// here.
	perVersionTests := [][]addressTest{
		addressV0Tests,
	}

	// Flatten all of the per-version tests into a single set of tests.
	var tests []addressTest
	for _, bundle := range perVersionTests {
		tests = append(tests, bundle...)
	}

	for _, test := range tests {
		// Ensure that the script is considered non standard and no addresses
		// are returned for unsupported script versions regardless.
		const unsupportedScriptVer = 9999
		gotType, gotAddrs := ExtractAddrs(unsupportedScriptVer, test.script,
			test.params)
		if gotType != STNonStandard {
			t.Errorf("%q -- unsupported script version: mismatched type -- "+
				"got %s, want %s (script %x)", test.name, gotType,
				STNonStandard, test.script)
			continue
		}
		if len(gotAddrs) != 0 {
			t.Errorf("%q -- unsupported script version: returned addresses -- "+
				"got %s, want 0 addrs (script %x)", test.name, gotAddrs,
				test.script)
			continue
		}

		// Extract the script type and addresses for the given test data.
		gotType, gotAddrs = ExtractAddrs(test.version, test.script, test.params)

		// Ensure the script type matches the expected type.
		if gotType != test.wantType {
			t.Errorf("%q: mismatched script type -- got %v, want %v", test.name,
				gotType, test.wantType)
			continue
		}

		// Ensure the addresses match the expected ones.
		var gotAddrsStr []string
		if len(gotAddrs) > 0 {
			gotAddrsStr = make([]string, 0, len(gotAddrs))
			for _, addr := range gotAddrs {
				gotAddrsStr = append(gotAddrsStr, addr.String())
			}
		}
		if len(gotAddrsStr) != len(test.wantAddrs) {
			t.Errorf("%q: mismatched number of addrs -- got %d, want %d",
				test.name, len(gotAddrsStr), len(test.wantAddrs))
			continue
		}
		if !reflect.DeepEqual(gotAddrsStr, test.wantAddrs) {
			t.Errorf("%q: mismatched address result -- got %v, want %v",
				test.name, gotAddrsStr, test.wantAddrs)
			continue
		}
	}
}
