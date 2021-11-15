// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"github.com/decred/dcrd/dcrec"
)

const (
	// MaxDataCarrierSize is the maximum number of bytes allowed in pushed
	// data to be considered a nulldata transaction.
	MaxDataCarrierSize = 256
)

// ScriptClass is an enumeration for the list of standard types of script.
type ScriptClass byte

// Classes of script payment known about in the blockchain.
const (
	NonStandardTy     ScriptClass = iota // None of the recognized forms.
	PubKeyTy                             // Pay pubkey.
	PubKeyHashTy                         // Pay pubkey hash.
	ScriptHashTy                         // Pay to script hash.
	MultiSigTy                           // Multi signature.
	NullDataTy                           // Empty data-only (provably prunable).
	StakeSubmissionTy                    // Stake submission.
	StakeGenTy                           // Stake generation
	StakeRevocationTy                    // Stake revocation.
	StakeSubChangeTy                     // Change for stake submission tx.
	PubkeyAltTy                          // Alternative signature pubkey.
	PubkeyHashAltTy                      // Alternative signature pubkey hash.
	TreasuryAddTy                        // Add value to treasury
	TreasuryGenTy                        // Generate utxos from treasury account
)

// scriptClassToName houses the human-readable strings which describe each
// script class.
var scriptClassToName = []string{
	NonStandardTy:     "nonstandard",
	PubKeyTy:          "pubkey",
	PubkeyAltTy:       "pubkeyalt",
	PubKeyHashTy:      "pubkeyhash",
	PubkeyHashAltTy:   "pubkeyhashalt",
	ScriptHashTy:      "scripthash",
	MultiSigTy:        "multisig",
	NullDataTy:        "nulldata",
	StakeSubmissionTy: "stakesubmission",
	StakeGenTy:        "stakegen",
	StakeRevocationTy: "stakerevoke",
	StakeSubChangeTy:  "sstxchange",
	TreasuryAddTy:     "treasuryadd",
	TreasuryGenTy:     "treasurygen",
}

// String implements the Stringer interface by returning the name of
// the enum script class. If the enum is invalid then "Invalid" will be
// returned.
func (t ScriptClass) String() string {
	if int(t) > len(scriptClassToName) || int(t) < 0 {
		return "Invalid"
	}
	return scriptClassToName[t]
}

// extractCompressedPubKey extracts a compressed public key from the passed
// script if it is a standard pay-to-compressed-secp256k1-pubkey script.  It
// will return nil otherwise.
func extractCompressedPubKey(script []byte) []byte {
	// A pay-to-compressed-pubkey script is of the form:
	//  OP_DATA_33 <33-byte compressed pubkey> OP_CHECKSIG

	// All compressed secp256k1 public keys must start with 0x02 or 0x03.
	if len(script) == 35 &&
		script[34] == OP_CHECKSIG &&
		script[0] == OP_DATA_33 &&
		(script[1] == 0x02 || script[1] == 0x03) {

		return script[1:34]
	}
	return nil
}

// extractUncompressedPubKey extracts an uncompressed public key from the
// passed script if it is a standard pay-to-uncompressed-secp256k1-pubkey
// script.  It will return nil otherwise.
func extractUncompressedPubKey(script []byte) []byte {
	// A pay-to-uncompressed-pubkey script is of the form:
	//  OP_DATA_65 <65-byte uncompressed pubkey> OP_CHECKSIG

	// All non-hybrid uncompressed secp256k1 public keys must start with 0x04.
	if len(script) == 67 &&
		script[66] == OP_CHECKSIG &&
		script[0] == OP_DATA_65 &&
		script[1] == 0x04 {

		return script[1:66]
	}
	return nil
}

// extractPubKey extracts either a compressed or uncompressed public key from the
// passed script if it is either a standard pay-to-compressed-secp256k1-pubkey
// or pay-to-uncompressed-secp256k1-pubkey script, respectively.  It will return
// nil otherwise.
func extractPubKey(script []byte) []byte {
	if pubKey := extractCompressedPubKey(script); pubKey != nil {
		return pubKey
	}
	return extractUncompressedPubKey(script)
}

// isPubKeyScript returns whether or not the passed script is either a standard
// pay-to-compressed-secp256k1-pubkey or pay-to-uncompressed-secp256k1-pubkey
// script.
func isPubKeyScript(script []byte) bool {
	return extractPubKey(script) != nil
}

// extractPubKeyAltDetails extracts the public key and signature type from the
// passed script if it is a standard pay-to-alt-pubkey script.  It will return
// nil otherwise.
func extractPubKeyAltDetails(script []byte) ([]byte, dcrec.SignatureType) {
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
	if len(script) < 3 || script[len(script)-1] != OP_CHECKSIGALT {
		return nil, 0
	}

	if len(script) == 35 && script[0] == OP_DATA_32 &&
		IsSmallInt(script[33]) && AsSmallInt(script[33]) == dcrec.STEd25519 {

		return script[1:33], dcrec.STEd25519
	}

	if len(script) == 36 && script[0] == OP_DATA_33 &&
		IsSmallInt(script[34]) &&
		AsSmallInt(script[34]) == dcrec.STSchnorrSecp256k1 &&
		isStrictPubKeyEncoding(script[1:34]) {

		return script[1:34], dcrec.STSchnorrSecp256k1
	}

	return nil, 0
}

// isPubKeyAltScript returns whether or not the passed script is a standard
// pay-to-alt-pubkey script.
func isPubKeyAltScript(script []byte) bool {
	pk, _ := extractPubKeyAltDetails(script)
	return pk != nil
}

// extractPubKeyHash extracts the public key hash from the passed script if it
// is a standard pay-to-pubkey-hash script.  It will return nil otherwise.
func extractPubKeyHash(script []byte) []byte {
	// A pay-to-pubkey-hash script is of the form:
	//  OP_DUP OP_HASH160 <20-byte hash> OP_EQUALVERIFY OP_CHECKSIG
	if len(script) == 25 &&
		script[0] == OP_DUP &&
		script[1] == OP_HASH160 &&
		script[2] == OP_DATA_20 &&
		script[23] == OP_EQUALVERIFY &&
		script[24] == OP_CHECKSIG {

		return script[3:23]
	}

	return nil
}

// isStandardAltSignatureType returns whether or not the provided opcode
// represents a push of a standard alt signature type.
func isStandardAltSignatureType(op byte) bool {
	if !IsSmallInt(op) {
		return false
	}

	sigType := AsSmallInt(op)
	return sigType == dcrec.STEd25519 || sigType == dcrec.STSchnorrSecp256k1
}

// extractPubKeyHashAltDetails extracts the public key hash and signature type
// from the passed script if it is a standard pay-to-alt-pubkey-hash script.  It
// will return nil otherwise.
func extractPubKeyHashAltDetails(script []byte) ([]byte, dcrec.SignatureType) {
	// A pay-to-alt-pubkey-hash script is of the form:
	//  DUP HASH160 <20-byte hash> EQUALVERIFY SIGTYPE CHECKSIG
	//
	// The only two currently supported alternative signature types are ed25519
	// and schnorr + secp256k1 (with a compressed pubkey).
	//
	//  DUP HASH160 <20-byte hash> EQUALVERIFY <1-byte ed25519 sigtype> CHECKSIG
	//  DUP HASH160 <20-byte hash> EQUALVERIFY <1-byte schnorr+secp sigtype> CHECKSIG
	//
	//  Notice that OP_0 is not specified since signature type 0 disabled.

	if len(script) == 26 &&
		script[0] == OP_DUP &&
		script[1] == OP_HASH160 &&
		script[2] == OP_DATA_20 &&
		script[23] == OP_EQUALVERIFY &&
		isStandardAltSignatureType(script[24]) &&
		script[25] == OP_CHECKSIGALT {

		return script[3:23], dcrec.SignatureType(AsSmallInt(script[24]))
	}

	return nil, 0
}

// isPubKeyHashAltScript returns whether or not the passed script is a standard
// pay-to-alt-pubkey-hash script.
func isPubKeyHashAltScript(script []byte) bool {
	pk, _ := extractPubKeyHashAltDetails(script)
	return pk != nil
}

// isNullDataScript returns whether or not the passed script is a standard
// null data script.
//
// NOTE: This function is only valid for version 0 scripts.  It will always
// return false for other script versions.
func isNullDataScript(scriptVersion uint16, script []byte) bool {
	// The only currently supported script version is 0.
	if scriptVersion != 0 {
		return false
	}

	// A null script is of the form:
	//  OP_RETURN <optional data>
	//
	// Thus, it can either be a single OP_RETURN or an OP_RETURN followed by a
	// data push up to MaxDataCarrierSize bytes.

	// The script can't possibly be a null data script if it doesn't start
	// with OP_RETURN.  Fail fast to avoid more work below.
	if len(script) < 1 || script[0] != OP_RETURN {
		return false
	}

	// Single OP_RETURN.
	if len(script) == 1 {
		return true
	}

	// OP_RETURN followed by data push up to MaxDataCarrierSize bytes.
	tokenizer := MakeScriptTokenizer(scriptVersion, script[1:])
	return tokenizer.Next() && tokenizer.Done() &&
		(IsSmallInt(tokenizer.Opcode()) || tokenizer.Opcode() <= OP_PUSHDATA4) &&
		len(tokenizer.Data()) <= MaxDataCarrierSize
}

// extractStakePubKeyHash extracts the public key hash from the passed script if
// it is a standard stake-tagged pay-to-pubkey-hash script with the provided
// stake opcode.  It will return nil otherwise.
func extractStakePubKeyHash(script []byte, stakeOpcode byte) []byte {
	// A stake-tagged pay-to-pubkey-hash is of the form:
	//   <stake opcode> <standard-pay-to-pubkey-hash script>

	// The script can't possibly be a stake-tagged pay-to-pubkey-hash if it
	// doesn't start with the given stake opcode.  Fail fast to avoid more work
	// below.
	if len(script) < 1 || script[0] != stakeOpcode {
		return nil
	}

	return extractPubKeyHash(script[1:])
}

// extractStakeScriptHash extracts the script hash from the passed script if it
// is a standard stake-tagged pay-to-script-hash script with the provided stake
// opcode.  It will return nil otherwise.
func extractStakeScriptHash(script []byte, stakeOpcode byte) []byte {
	// A stake-tagged pay-to-script-hash is of the form:
	//   <stake opcode> <standard-pay-to-script-hash script>

	// The script can't possibly be a stake-tagged pay-to-script-hash if it
	// doesn't start with the given stake opcode.  Fail fast to avoid more work
	// below.
	if len(script) < 1 || script[0] != stakeOpcode {
		return nil
	}

	return ExtractScriptHash(script[1:])
}

// isStakeSubmissionScript returns whether or not the passed script is a
// supported stake submission script.
//
// NOTE: This function is only valid for version 0 scripts.  It will always
// return false for other script versions.
func isStakeSubmissionScript(scriptVersion uint16, script []byte) bool {
	// The only currently supported script version is 0.
	if scriptVersion != 0 {
		return false
	}

	// The only supported stake submission scripts are pay-to-pubkey-hash and
	// pay-to-script-hash tagged with the stake submission opcode.
	const stakeOpcode = OP_SSTX
	return extractStakePubKeyHash(script, stakeOpcode) != nil ||
		extractStakeScriptHash(script, stakeOpcode) != nil
}

// isStakeGenScript returns whether or not the passed script is a supported
// stake generation script.
//
// NOTE: This function is only valid for version 0 scripts.  It will always
// return false for other script versions.
func isStakeGenScript(scriptVersion uint16, script []byte) bool {
	// The only currently supported script version is 0.
	if scriptVersion != 0 {
		return false
	}

	// The only supported stake generation scripts are pay-to-pubkey-hash and
	// pay-to-script-hash tagged with the stake submission opcode.
	const stakeOpcode = OP_SSGEN
	return extractStakePubKeyHash(script, stakeOpcode) != nil ||
		extractStakeScriptHash(script, stakeOpcode) != nil
}

// isStakeRevocationScript returns whether or not the passed script is a
// supported stake revocation script.
//
// NOTE: This function is only valid for version 0 scripts.  It will always
// return false for other script versions.
func isStakeRevocationScript(scriptVersion uint16, script []byte) bool {
	// The only currently supported script version is 0.
	if scriptVersion != 0 {
		return false
	}

	// The only supported stake revocation scripts are pay-to-pubkey-hash and
	// pay-to-script-hash tagged with the stake submission opcode.
	const stakeOpcode = OP_SSRTX
	return extractStakePubKeyHash(script, stakeOpcode) != nil ||
		extractStakeScriptHash(script, stakeOpcode) != nil
}
