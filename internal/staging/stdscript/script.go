// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package stdscript provides facilities for working with standard scripts.
package stdscript

// ScriptType identifies the type of known scripts in the blockchain that are
// typically considered standard by the default policy of most nodes.  All other
// scripts are considered non-standard.
type ScriptType byte

const (
	// STNonStandard indicates a script is none of the recognized standard
	// forms.
	STNonStandard ScriptType = iota

	// STPubKeyEcdsaSecp256k1 identifies a standard script that imposes an
	// encumbrance that requires a valid ECDSA signature for a specific
	// secp256k1 public key.
	//
	// This is commonly referred to as either a pay-to-pubkey (P2PK) script or
	// the more specific pay-to-pubkey-ecdsa-secp256k1 script.
	STPubKeyEcdsaSecp256k1

	// numScriptTypes is the maximum script type number used in tests.  This
	// entry MUST be the last entry in the enum.
	numScriptTypes
)

// scriptTypeToName houses the human-readable strings which describe each script
// type.
var scriptTypeToName = []string{
	STNonStandard:          "nonstandard",
	STPubKeyEcdsaSecp256k1: "pubkey",
}

// String returns the ScriptType as a human-readable name.
func (t ScriptType) String() string {
	if t >= numScriptTypes {
		return "invalid"
	}
	return scriptTypeToName[t]
}

// IsPubKeyScript returns whether or not the passed script is either a standard
// pay-to-compressed-secp256k1-pubkey or pay-to-uncompressed-secp256k1-pubkey
// script.
//
// NOTE: Version 0 scripts are the only currently supported version.  It will
// always return false for other script versions.
func IsPubKeyScript(scriptVersion uint16, script []byte) bool {
	switch scriptVersion {
	case 0:
		return IsPubKeyScriptV0(script)
	}

	return false
}

// DetermineScriptType returns the type of the script passed.
//
// NOTE: Version 0 scripts are the only currently supported version.  It will
// always return STNonStandard for other script versions.
//
// Similarly, STNonStandard is returned when the script does not parse.
func DetermineScriptType(scriptVersion uint16, script []byte) ScriptType {
	switch scriptVersion {
	case 0:
		return DetermineScriptTypeV0(script)
	}

	// All scripts with newer versions are considered non standard.
	return STNonStandard
}
