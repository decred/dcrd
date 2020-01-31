// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"github.com/decred/dcrd/txscript/v3"
)

// isScriptHashScript returns whether or not the passed script is a
// pay-to-script-hash script per consensus rules.
func isScriptHashScript(script []byte) bool {
	// A pay-to-script-hash script is of the form:
	//  OP_HASH160 <20-byte scripthash> OP_EQUAL
	return len(script) == 23 &&
		script[0] == txscript.OP_HASH160 &&
		script[1] == txscript.OP_DATA_20 &&
		script[22] == txscript.OP_EQUAL
}

// isPubKeyHashScript returns whether or not the passed script is
// a pay-to-pubkey-hash script per consensus rules.
func isPubKeyHashScript(script []byte) bool {
	// A pay-to-pubkey-hash script is of the form:
	//  OP_DUP OP_HASH160 <20-byte hash> OP_EQUALVERIFY OP_CHECKSIG
	return len(script) == 25 &&
		script[0] == txscript.OP_DUP &&
		script[1] == txscript.OP_HASH160 &&
		script[2] == txscript.OP_DATA_20 &&
		script[23] == txscript.OP_EQUALVERIFY &&
		script[24] == txscript.OP_CHECKSIG
}

// isTaggedScript checks if the provided script is tagged by the
// provided op code.
func isTaggedScript(version uint16, script []byte, op int) bool {
	// The only supported version is 0.
	if version != 0 {
		return false
	}

	if len(script) < 1 {
		return false
	}

	// A stake script pay-to-script-hash is of the form:
	//   <opcode> <P2PKH or P2SH script>
	if int(script[0]) != op {
		return false
	}

	return isPubKeyHashScript(script[1:]) || isScriptHashScript(script[1:])
}

// IsTicketPurchaseScript checks if the provided script is a ticket purchase
// script.
func IsTicketPurchaseScript(version uint16, script []byte) bool {
	return isTaggedScript(version, script, txscript.OP_SSTX)
}

// IsRevocationScript checks if the provided script is a ticket revocation
// script.
func IsRevocationScript(version uint16, script []byte) bool {
	return isTaggedScript(version, script, txscript.OP_SSRTX)
}

// IsStakeChangeScript checks if the provided script is a stake change script.
func IsStakeChangeScript(version uint16, script []byte) bool {
	return isTaggedScript(version, script, txscript.OP_SSTXCHANGE)
}

// IsVoteScript checks if the provided script is a vote script.
func IsVoteScript(version uint16, script []byte) bool {
	return isTaggedScript(version, script, txscript.OP_SSGEN)
}
