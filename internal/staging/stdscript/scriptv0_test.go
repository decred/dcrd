// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdscript

import (
	"fmt"
)

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
	}}
}()
