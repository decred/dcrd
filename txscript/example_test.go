// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript_test

import (
	"fmt"

	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
)

// This example demonstrates creating a script tokenizer instance and using it
// to count the number of opcodes a script contains.
func ExampleScriptTokenizer() {
	// Create a script to use in the example.  Ordinarily this would come from
	// some other source.
	hash160 := stdaddr.Hash160([]byte("example"))
	script, err := txscript.NewScriptBuilder().AddOp(txscript.OP_DUP).
		AddOp(txscript.OP_HASH160).AddData(hash160).
		AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG).Script()
	if err != nil {
		fmt.Printf("failed to build script: %v\n", err)
		return
	}

	// Create a tokenizer to iterate the script and count the number of opcodes.
	const scriptVersion = 0
	var numOpcodes int
	tokenizer := txscript.MakeScriptTokenizer(scriptVersion, script)
	for tokenizer.Next() {
		numOpcodes++
	}
	if tokenizer.Err() != nil {
		fmt.Printf("script failed to parse: %v\n", err)
	} else {
		fmt.Printf("script contains %d opcode(s)\n", numOpcodes)
	}

	// Output:
	// script contains 5 opcode(s)
}
