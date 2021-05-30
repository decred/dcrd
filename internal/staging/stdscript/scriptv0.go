// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdscript

import (
	"github.com/decred/dcrd/dcrec"
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

// ExtractPubKeyAltDetailsV0 extracts the public key and signature type from the
// passed script if it is a standard version 0 pay-to-alt-pubkey script.  It
// will return nil otherwise.
func ExtractPubKeyAltDetailsV0(script []byte) ([]byte, dcrec.SignatureType) {
	// A pay-to-alt-pubkey script is of the form:
	//  PUBKEY SIGTYPE OP_CHECKSIGALT
	//
	// The only two currently supported alternative signature types are ed25519
	// and schnorr + secp256k1 (with a compressed pubkey).
	//
	//  OP_DATA_32 <32-byte pubkey> <1-byte ed25519 sigtype> OP_CHECKSIGALT
	//  OP_DATA_33 <33-byte pubkey> <1-byte schnorr+secp sigtype> OP_CHECKSIGALT

	// The script can't possibly be a pay-to-alt-pubkey script if it doesn't
	// end with OP_CHECKSIGALT or have at least two small integer pushes
	// preceding it (although any reasonable pubkey will certainly be larger).
	// Fail fast to avoid more work below.
	if len(script) < 3 || script[len(script)-1] != txscript.OP_CHECKSIGALT {
		return nil, 0
	}

	if len(script) == 35 && script[0] == txscript.OP_DATA_32 &&
		txscript.IsSmallInt(script[33]) &&
		txscript.AsSmallInt(script[33]) == dcrec.STEd25519 {

		return script[1:33], dcrec.STEd25519
	}

	if len(script) == 36 && script[0] == txscript.OP_DATA_33 &&
		txscript.IsSmallInt(script[34]) &&
		txscript.AsSmallInt(script[34]) == dcrec.STSchnorrSecp256k1 &&
		txscript.IsStrictCompressedPubKeyEncoding(script[1:34]) {

		return script[1:34], dcrec.STSchnorrSecp256k1
	}

	return nil, 0
}

// IsPubKeyEd25519ScriptV0 returns whether or not the passed script is a
// standard version 0 pay-to-ed25519-pubkey script.
func IsPubKeyEd25519ScriptV0(script []byte) bool {
	pk, sigType := ExtractPubKeyAltDetailsV0(script)
	return pk != nil && sigType == dcrec.STEd25519
}

// IsPubKeySchnorrSecp256k1ScriptV0 returns whether or not the passed script is
// a standard version 0 pay-to-schnorr-secp256k1-pubkey script.
func IsPubKeySchnorrSecp256k1ScriptV0(script []byte) bool {
	pk, sigType := ExtractPubKeyAltDetailsV0(script)
	return pk != nil && sigType == dcrec.STSchnorrSecp256k1
}

// ExtractPubKeyHashV0 extracts the public key hash from the passed script if it
// is a standard version 0 pay-to-pubkey-hash-ecdsa-secp256k1 script.  It will
// return nil otherwise.
func ExtractPubKeyHashV0(script []byte) []byte {
	// A pay-to-pubkey-hash script is of the form:
	//  OP_DUP OP_HASH160 <20-byte hash> OP_EQUALVERIFY OP_CHECKSIG
	if len(script) == 25 &&
		script[0] == txscript.OP_DUP &&
		script[1] == txscript.OP_HASH160 &&
		script[2] == txscript.OP_DATA_20 &&
		script[23] == txscript.OP_EQUALVERIFY &&
		script[24] == txscript.OP_CHECKSIG {

		return script[3:23]
	}

	return nil
}

// IsPubKeyHashScriptV0 returns whether or not the passed script is a standard
// version 0 pay-to-pubkey-hash-ecdsa-secp256k1 script.
func IsPubKeyHashScriptV0(script []byte) bool {
	return ExtractPubKeyHashV0(script) != nil
}

// IsStandardAltSignatureTypeV0 returns whether or not the provided version 0
// script opcode represents a push of a standard alt signature type.
func IsStandardAltSignatureTypeV0(op byte) bool {
	if !txscript.IsSmallInt(op) {
		return false
	}

	sigType := txscript.AsSmallInt(op)
	return sigType == dcrec.STEd25519 || sigType == dcrec.STSchnorrSecp256k1
}

// ExtractPubKeyHashAltDetailsV0 extracts the public key hash and signature type
// from the passed script if it is a standard version 0 pay-to-alt-pubkey-hash
// script.  It will return nil otherwise.
func ExtractPubKeyHashAltDetailsV0(script []byte) ([]byte, dcrec.SignatureType) {
	// A pay-to-alt-pubkey-hash script is of the form:
	//  DUP HASH160 <20-byte hash> EQUALVERIFY SIGTYPE CHECKSIG
	//
	// The only two currently supported alternative signature types are ed25519
	// and schnorr + secp256k1 (with a compressed pubkey).
	//
	//  DUP HASH160 <20-byte hash> EQUALVERIFY <1-byte ed25519 sigtype> CHECKSIGALT
	//  DUP HASH160 <20-byte hash> EQUALVERIFY <1-byte schnorr+secp sigtype> CHECKSIGALT
	//
	//  Notice that OP_0 is not specified since signature type 0 disabled.
	if len(script) == 26 &&
		script[0] == txscript.OP_DUP &&
		script[1] == txscript.OP_HASH160 &&
		script[2] == txscript.OP_DATA_20 &&
		script[23] == txscript.OP_EQUALVERIFY &&
		IsStandardAltSignatureTypeV0(script[24]) &&
		script[25] == txscript.OP_CHECKSIGALT {

		return script[3:23], dcrec.SignatureType(txscript.AsSmallInt(script[24]))
	}

	return nil, 0
}

// IsPubKeyHashEd25519ScriptV0 returns whether or not the passed script is a
// standard version 0 pay-to-pubkey-hash-ed25519 script.
func IsPubKeyHashEd25519ScriptV0(script []byte) bool {
	pk, sigType := ExtractPubKeyHashAltDetailsV0(script)
	return pk != nil && sigType == dcrec.STEd25519
}

// IsPubKeyHashSchnorrSecp256k1ScriptV0 returns whether or not the passed script
// is a standard version 0 pay-to-pubkey-hash-schnorr-secp256k1 script.
func IsPubKeyHashSchnorrSecp256k1ScriptV0(script []byte) bool {
	pk, sigType := ExtractPubKeyHashAltDetailsV0(script)
	return pk != nil && sigType == dcrec.STSchnorrSecp256k1
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
	case IsPubKeyEd25519ScriptV0(script):
		return STPubKeyEd25519
	case IsPubKeySchnorrSecp256k1ScriptV0(script):
		return STPubKeySchnorrSecp256k1
	case IsPubKeyHashScriptV0(script):
		return STPubKeyHashEcdsaSecp256k1
	case IsPubKeyHashEd25519ScriptV0(script):
		return STPubKeyHashEd25519
	case IsPubKeyHashSchnorrSecp256k1ScriptV0(script):
		return STPubKeyHashSchnorrSecp256k1
	}

	return STNonStandard
}
