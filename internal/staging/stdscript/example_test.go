// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdscript_test

import (
	"encoding/hex"
	"fmt"

	"github.com/decred/dcrd/internal/staging/stdscript"
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

// This example demonstrates determining the type of a script for a given
// scripting language version.
func ExampleDetermineScriptType() {
	// Ordinarily the script version and script would be obtained from the
	// output of a transaction, but they are hard coded here for the purposes of
	// this example.
	const scriptVersion = 0
	script := hexToBytes("76a914e280cb6e66b96679aec288b1fbdbd4db08077a1b88ac")
	scriptType := stdscript.DetermineScriptType(scriptVersion, script)
	switch scriptType {
	case stdscript.STPubKeyHashEcdsaSecp256k1:
		fmt.Printf("standard version %d %v script\n", scriptVersion, scriptType)

	default:
		fmt.Printf("other script type: %v\n", scriptType)
	}

	// Output:
	// standard version 0 pubkeyhash script
}
