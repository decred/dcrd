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

	// STPubKeyEd25519 identifies a standard script that imposes an encumbrance
	// that requires a valid Ed25519 signature for a specific Ed25519 public
	// key.
	//
	// This is commonly referred to as a pay-to-pubkey-ed25519 script.
	STPubKeyEd25519

	// STPubKeySchnorrSecp256k1 identifies a standard script that imposes an
	// encumbrance that requires a valid EC-Schnorr-DCRv0 signature for a
	// specific secp256k1 public key.
	//
	// This is commonly referred to as a pay-to-pubkey-schnorr-secp256k1 script.
	STPubKeySchnorrSecp256k1

	// STPubKeyHashEcdsaSecp256k1 identifies a standard script that imposes an
	// encumbrance that requires a secp256k1 public key that hashes to a
	// specific value along with a valid ECDSA signature for that public key.
	//
	// This is commonly referred to as either a pay-to-pubkey-hash (P2PKH)
	// script or the more specific pay-to-pubkey-hash-ecdsa-secp256k1 script.
	STPubKeyHashEcdsaSecp256k1

	// STPubKeyHashEd25519 identifies a standard script that imposes an
	// encumbrance that requires an Ed25519 public key that hashes to a specific
	// value along with a valid Ed25519 signature for that public key.
	//
	// This is commonly referred to as a pay-to-pubkey-hash-ed25519 script.
	STPubKeyHashEd25519

	// STPubKeyHashSchnorrSecp256k1 identifies a standard script that imposes an
	// encumbrance that requires a secp256k1 public key that hashes to a
	// specific value along with a valid EC-Schnorr-DCRv0 signature for that
	// public key.
	//
	// This is commonly referred to as a pay-to-pubkey-hash-schnorr-secp256k1
	// script.
	STPubKeyHashSchnorrSecp256k1

	// STScriptHash identifies a standard script that imposes an encumbrance
	// that requires a script that hashes to a specific value along with all of
	// the encumbrances that script itself imposes.  The script is commonly
	// referred to as a redeem script.
	//
	// This is commonly referred to as pay-to-script-hash (P2SH).
	STScriptHash

	// STMultiSig identifies a standard script that imposes an encumbrance that
	// requires a given number of valid ECDSA signatures which correspond to
	// given secp256k1 public keys.
	//
	// This is commonly referred to as a standard ECDSA n-of-m multi-signature
	// script.
	STMultiSig

	// STNullData identifies a standard null data script that is provably
	// prunable.
	STNullData

	// STStakeSubmissionPubKeyHash identifies a script that is only valid when
	// used as part of a ticket purchase transaction in the staking system and
	// is used for imposing voting rights.
	//
	// It imposes an encumbrance that requires a secp256k1 public key that
	// hashes to a specific value along with a valid ECDSA signature for that
	// public key.
	STStakeSubmissionPubKeyHash

	// STStakeSubmissionScriptHash identifies a script that is only valid when
	// used as part of a ticket purchase transaction in the staking system and
	// is used for imposing voting rights.
	//
	// It imposes an encumbrance that requires a script that hashes to a
	// specific value along with all of the encumbrances that script itself
	// imposes.  The script is commonly referred to as a redeem script.
	STStakeSubmissionScriptHash

	// STStakeGenPubKeyHash identifies a script that is only valid when used as
	// part of a vote transaction in the staking system.
	//
	// It imposes an encumbrance that requires a secp256k1 public key that
	// hashes to a specific value along with a valid ECDSA signature for that
	// public key.
	STStakeGenPubKeyHash

	// numScriptTypes is the maximum script type number used in tests.  This
	// entry MUST be the last entry in the enum.
	numScriptTypes
)

// scriptTypeToName houses the human-readable strings which describe each script
// type.
var scriptTypeToName = []string{
	STNonStandard:                "nonstandard",
	STPubKeyEcdsaSecp256k1:       "pubkey",
	STPubKeyEd25519:              "pubkey-ed25519",
	STPubKeySchnorrSecp256k1:     "pubkey-schnorr-secp256k1",
	STPubKeyHashEcdsaSecp256k1:   "pubkeyhash",
	STPubKeyHashEd25519:          "pubkeyhash-ed25519",
	STPubKeyHashSchnorrSecp256k1: "pubkeyhash-schnorr-secp256k1",
	STScriptHash:                 "scripthash",
	STMultiSig:                   "multisig",
	STNullData:                   "nulldata",
	STStakeSubmissionPubKeyHash:  "stakesubmission-pubkeyhash",
	STStakeSubmissionScriptHash:  "stakesubmission-scripthash",
	STStakeGenPubKeyHash:         "stakegen-pubkeyhash",
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

// IsPubKeyEd25519Script returns whether or not the passed script is a standard
// pay-to-ed25519-pubkey script.
//
// NOTE: Version 0 scripts are the only currently supported version.  It will
// always return false for other script versions.
func IsPubKeyEd25519Script(scriptVersion uint16, script []byte) bool {
	switch scriptVersion {
	case 0:
		return IsPubKeyEd25519ScriptV0(script)
	}

	return false
}

// IsPubKeySchnorrSecp256k1Script returns whether or not the passed script is a
// standard pay-to-schnorr-secp256k1-pubkey script.
//
// NOTE: Version 0 scripts are the only currently supported version.  It will
// always return false for other script versions.
func IsPubKeySchnorrSecp256k1Script(scriptVersion uint16, script []byte) bool {
	switch scriptVersion {
	case 0:
		return IsPubKeySchnorrSecp256k1ScriptV0(script)
	}

	return false
}

// IsPubKeyHashScript returns whether or not the passed script is a standard
// pay-to-pubkey-hash-ecdsa-secp256k1 script.
//
// NOTE: Version 0 scripts are the only currently supported version.  It will
// always return false for other script versions.
func IsPubKeyHashScript(scriptVersion uint16, script []byte) bool {
	switch scriptVersion {
	case 0:
		return IsPubKeyHashScriptV0(script)
	}

	return false
}

// IsPubKeyHashEd25519Script returns whether or not the passed script is a
// standard pay-to-pubkey-hash-ed25519 script.
//
// NOTE: Version 0 scripts are the only currently supported version.  It will
// always return false for other script versions.
func IsPubKeyHashEd25519Script(scriptVersion uint16, script []byte) bool {
	switch scriptVersion {
	case 0:
		return IsPubKeyHashEd25519ScriptV0(script)
	}

	return false
}

// IsPubKeyHashSchnorrSecp256k1Script returns whether or not the passed script
// is a standard pay-to-pubkey-hash-schnorr-secp256k1 script.
//
// NOTE: Version 0 scripts are the only currently supported version.  It will
// always return false for other script versions.
func IsPubKeyHashSchnorrSecp256k1Script(scriptVersion uint16, script []byte) bool {
	switch scriptVersion {
	case 0:
		return IsPubKeyHashSchnorrSecp256k1ScriptV0(script)
	}

	return false
}

// IsScriptHashScript returns whether or not the passed script is a standard
// pay-to-script-hash script.
//
// NOTE: Version 0 scripts are the only currently supported version.  It will
// always return false for other script versions.
func IsScriptHashScript(scriptVersion uint16, script []byte) bool {
	switch scriptVersion {
	case 0:
		return IsScriptHashScriptV0(script)
	}

	return false
}

// IsMultiSigScript returns whether or not the passed script is a standard
// ECDSA multisig script.
//
// NOTE: Version 0 scripts are the only currently supported version.  It will
// always return false for other script versions.
func IsMultiSigScript(scriptVersion uint16, script []byte) bool {
	switch scriptVersion {
	case 0:
		return IsMultiSigScriptV0(script)
	}

	return false
}

// IsMultiSigSigScript returns whether or not the passed script appears to be a
// signature script which consists of a pay-to-script-hash multi-signature
// redeem script.  Determining if a signature script is actually a redemption of
// pay-to-script-hash requires the associated public key script which is often
// expensive to obtain.  Therefore, this makes a fast best effort guess that has
// a high probability of being correct by checking if the signature script ends
// with a data push and treating that data push as if it were a p2sh redeem
// script.
//
// NOTE: Version 0 scripts are the only currently supported version.  It will
// always return false for other script versions.
func IsMultiSigSigScript(scriptVersion uint16, script []byte) bool {
	switch scriptVersion {
	case 0:
		return IsMultiSigSigScriptV0(script)
	}

	return false
}

// IsNullDataScript returns whether or not the passed script is a standard
// null data script.
//
// NOTE: Version 0 scripts are the only currently supported version.  It will
// always return false for other script versions.
func IsNullDataScript(scriptVersion uint16, script []byte) bool {
	switch scriptVersion {
	case 0:
		return IsNullDataScriptV0(script)
	}

	return false
}

// IsStakeSubmissionPubKeyHashScript returns whether or not the passed script is
// a standard stake submission pay-to-pubkey-hash script.
//
// NOTE: Version 0 scripts are the only currently supported version.  It will
// always return false for other script versions.
func IsStakeSubmissionPubKeyHashScript(scriptVersion uint16, script []byte) bool {
	switch scriptVersion {
	case 0:
		return IsStakeSubmissionPubKeyHashScriptV0(script)
	}

	return false
}

// IsStakeSubmissionScriptHashScript returns whether or not the passed script is
// a standard stake submission pay-to-script-hash script.
//
// NOTE: Version 0 scripts are the only currently supported version.  It will
// always return false for other script versions.
func IsStakeSubmissionScriptHashScript(scriptVersion uint16, script []byte) bool {
	switch scriptVersion {
	case 0:
		return IsStakeSubmissionScriptHashScriptV0(script)
	}

	return false
}

// IsStakeGenPubKeyHashScript returns whether or not the passed script is a
// standard stake generation pay-to-pubkey-hash script.
//
// NOTE: Version 0 scripts are the only currently supported version.  It will
// always return false for other script versions.
func IsStakeGenPubKeyHashScript(scriptVersion uint16, script []byte) bool {
	switch scriptVersion {
	case 0:
		return IsStakeGenPubKeyHashScriptV0(script)
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
