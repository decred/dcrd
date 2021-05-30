// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdscript

import (
	"github.com/decred/dcrd/txscript/v4"
)

// ExtractCompressedPubKeyV0 extracts a compressed public key from the passed
// script if it is a standard version 0 pay-to-compressed-secp256k1-pubkey
// script.  It will return nil otherwise.
func ExtractCompressedPubKeyV0(script []byte) []byte {
	// A pay-to-compressed-pubkey script is of the form:
	//  OP_DATA_33 <33-byte compressed pubkey> OP_CHECKSIG

	// All compressed secp256k1 public keys must start with 0x02 or 0x03.
	if len(script) == 35 &&
		script[34] == txscript.OP_CHECKSIG &&
		script[0] == txscript.OP_DATA_33 &&
		(script[1] == 0x02 || script[1] == 0x03) {

		return script[1:34]
	}
	return nil
}

// ExtractUncompressedPubKeyV0 extracts an uncompressed public key from the
// passed script if it is a standard version 0
// pay-to-uncompressed-secp256k1-pubkey script.  It will return nil otherwise.
func ExtractUncompressedPubKeyV0(script []byte) []byte {
	// A pay-to-uncompressed-pubkey script is of the form:
	//  OP_DATA_65 <65-byte uncompressed pubkey> OP_CHECKSIG

	// All non-hybrid uncompressed secp256k1 public keys must start with 0x04.
	if len(script) == 67 &&
		script[66] == txscript.OP_CHECKSIG &&
		script[0] == txscript.OP_DATA_65 &&
		script[1] == 0x04 {

		return script[1:66]
	}
	return nil
}

// ExtractPubKeyV0 extracts either a compressed or uncompressed public key from
// the passed script if it is either a standard version 0
// pay-to-compressed-secp256k1-pubkey or pay-to-uncompressed-secp256k1-pubkey
// script, respectively.  It will return nil otherwise.
func ExtractPubKeyV0(script []byte) []byte {
	if pubKey := ExtractCompressedPubKeyV0(script); pubKey != nil {
		return pubKey
	}
	return ExtractUncompressedPubKeyV0(script)
}

// IsPubKeyScriptV0 returns whether or not the passed script is either a
// standard version 0 pay-to-compressed-secp256k1-pubkey or
// pay-to-uncompressed-secp256k1-pubkey script.
func IsPubKeyScriptV0(script []byte) bool {
	return ExtractPubKeyV0(script) != nil
}

// DetermineScriptTypeV0 returns the type of the passed version 0 script from
// the known standard types.  This includes both types that are required by
// consensus as well as those which are not.
//
// STNonStandard will be returned when the script does not parse.
func DetermineScriptTypeV0(script []byte) ScriptType {
	switch {
	case IsPubKeyScriptV0(script):
		return STPubKeyEcdsaSecp256k1
	}

	return STNonStandard
}
