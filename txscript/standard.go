// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"encoding/binary"
	"fmt"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1"
	"github.com/decred/dcrd/dcrutil"
)

const (
	// MaxDataCarrierSize is the maximum number of bytes allowed in pushed
	// data to be considered a nulldata transaction.
	MaxDataCarrierSize = 256

	// nilAddrErrStr is the common error string to use for attempts to
	// generate payment scripts to nil addresses embedded within a
	// dcrutil.Address interface.
	nilAddrErrStr = "unable to generate payment script for nil address"
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

// multiSigDetails houses details extracted from a standard multisig script.
type multiSigDetails struct {
	requiredSigs int
	numPubKeys   int
	pubKeys      [][]byte
	valid        bool
}

// extractMultisigScriptDetails attempts to extract details from the passed
// script if it is a standard multisig script.  The returned details struct will
// have the valid flag set to false otherwise.
//
// The extract pubkeys flag indicates whether or not the pubkeys themselves
// should also be extracted and is provided because extracting them results in
// an allocation that the caller might wish to avoid.  The pubKeys member of
// the returned details struct will be nil when the flag is false.
//
// NOTE: This function is only valid for version 0 scripts.  The returned
// details struct will always be empty and have the valid flag set to false for
// other script versions.
func extractMultisigScriptDetails(scriptVersion uint16, script []byte, extractPubKeys bool) multiSigDetails {
	// The only currently supported script version is 0.
	if scriptVersion != 0 {
		return multiSigDetails{}
	}

	// A multi-signature script is of the form:
	//  NUM_SIGS PUBKEY PUBKEY PUBKEY ... NUM_PUBKEYS OP_CHECKMULTISIG

	// The script can't possibly be a multisig script if it doesn't end with
	// OP_CHECKMULTISIG or have at least two small integer pushes preceding it.
	// Fail fast to avoid more work below.
	if len(script) < 3 || script[len(script)-1] != OP_CHECKMULTISIG {
		return multiSigDetails{}
	}

	// The first opcode must be a small integer specifying the number of
	// signatures required.
	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	if !tokenizer.Next() || !isSmallInt(tokenizer.Opcode()) {
		return multiSigDetails{}
	}
	requiredSigs := asSmallInt(tokenizer.Opcode())

	// The next series of opcodes must either push public keys or be a small
	// integer specifying the number of public keys.
	var numPubKeys int
	var pubKeys [][]byte
	if extractPubKeys {
		pubKeys = make([][]byte, 0, MaxPubKeysPerMultiSig)
	}
	for tokenizer.Next() {
		data := tokenizer.Data()
		if !isStrictPubKeyEncoding(data) {
			break
		}
		numPubKeys++
		if extractPubKeys {
			pubKeys = append(pubKeys, data)
		}
	}
	if tokenizer.Done() {
		return multiSigDetails{}
	}

	// The next opcode must be a small integer specifying the number of public
	// keys required.
	op := tokenizer.Opcode()
	if !isSmallInt(op) || asSmallInt(op) != numPubKeys {
		return multiSigDetails{}
	}

	// There must only be a single opcode left unparsed which will be
	// OP_CHECKMULTISIG per the check above.
	if int32(len(tokenizer.Script()))-tokenizer.ByteIndex() != 1 {
		return multiSigDetails{}
	}

	return multiSigDetails{
		requiredSigs: requiredSigs,
		numPubKeys:   numPubKeys,
		pubKeys:      pubKeys,
		valid:        true,
	}
}

// isMultisigScript returns whether or not the passed script is a standard
// multisig script.
//
// NOTE: This function is only valid for version 0 scripts.  It will always
// return false for other script versions.
func isMultisigScript(scriptVersion uint16, script []byte) bool {
	// Since this is only checking the form of the script, don't extract the
	// public keys to avoid the allocation.
	details := extractMultisigScriptDetails(scriptVersion, script, false)
	return details.valid
}

// IsMultisigScript returns whether or not the passed script is a standard
// multisignature script.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func IsMultisigScript(script []byte) bool {
	const scriptVersion = 0
	return isMultisigScript(scriptVersion, script)
}

// IsMultisigSigScript returns whether or not the passed script appears to be a
// signature script which consists of a pay-to-script-hash multi-signature
// redeem script.  Determining if a signature script is actually a redemption of
// pay-to-script-hash requires the associated public key script which is often
// expensive to obtain.  Therefore, this makes a fast best effort guess that has
// a high probability of being correct by checking if the signature script ends
// with a data push and treating that data push as if it were a p2sh redeem
// script
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func IsMultisigSigScript(script []byte) bool {
	const scriptVersion = 0

	// The script can't possibly be a multisig signature script if it doesn't
	// end with OP_CHECKMULTISIG in the redeem script or have at least two small
	// integers preceding it, and the redeem script itself must be preceded by
	// at least a data push opcode.  Fail fast to avoid more work below.
	if len(script) < 4 || script[len(script)-1] != OP_CHECKMULTISIG {
		return false
	}

	// Parse through the script to find the last opcode and any data it might
	// push and treat it as a p2sh redeem script even though it might not
	// actually be one.
	possibleRedeemScript := finalOpcodeData(scriptVersion, script)
	if possibleRedeemScript == nil {
		return false
	}

	// Finally, return if that possible redeem script is a multisig script.
	return isMultisigScript(scriptVersion, possibleRedeemScript)
}

// extractCompressedPubKey extracts a compressed public key from the passed
// script if it is a standard pay-to-compressed-secp256k1-pubkey script.  It
// will return nil otherwise.
func extractCompressedPubKey(script []byte) []byte {
	// A pay-to-compressed-pubkey script is of the form:
	//  OP_DATA_33 <33-byte compresed pubkey> OP_CHECKSIG

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
	// A pay-to-compressed-pubkey script is of the form:
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

// extractPubKey extracts either compressed or uncompressed public key from the
// passed script if it is a either a standard pay-to-compressed-secp256k1-pubkey
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

	// The script can't possibly be a a pay-to-alt-pubkey script if it doesn't
	// end with OP_CHECKSIGALT or have at least two small integer pushes
	// preceding it (although any reasonable pubkey will certainly be larger).
	// Fail fast to avoid more work below.
	if len(script) < 3 || script[len(script)-1] != OP_CHECKSIGALT {
		return nil, 0
	}

	if len(script) == 35 && script[0] == OP_DATA_32 &&
		isSmallInt(script[33]) && asSmallInt(script[33]) == dcrec.STEd25519 {

		return script[1:33], dcrec.STEd25519
	}

	if len(script) == 36 && script[0] == OP_DATA_33 &&
		isSmallInt(script[34]) &&
		asSmallInt(script[34]) == dcrec.STSchnorrSecp256k1 &&
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

// isPubKeyHashScript returns whether or not the passed script is a standard
// pay-to-pubkey-hash script.
func isPubKeyHashScript(script []byte) bool {
	return extractPubKeyHash(script) != nil
}

// isStandardAltSignatureType returns whether or not the provided opcode
// represents a push of a standard alt signature type.
func isStandardAltSignatureType(op byte) bool {
	if !isSmallInt(op) {
		return false
	}

	sigType := asSmallInt(op)
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

		return script[3:23], dcrec.SignatureType(asSmallInt(script[24]))
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

	// The script can't possibly be a a null data script if it doesn't start
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
		(isSmallInt(tokenizer.Opcode()) || tokenizer.Opcode() <= OP_PUSHDATA4) &&
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

	return extractScriptHash(script[1:])
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

// isStakeChangeScript returns whether or not the passed script is a supported
// stake change script.
//
// NOTE: This function is only valid for version 0 scripts.  It will always
// return false for other script versions.
func isStakeChangeScript(scriptVersion uint16, script []byte) bool {
	// The only currently supported script version is 0.
	if scriptVersion != 0 {
		return false
	}

	// The only supported stake change scripts are pay-to-pubkey-hash and
	// pay-to-script-hash tagged with the stake submission opcode.
	const stakeOpcode = OP_SSTXCHANGE
	return extractStakePubKeyHash(script, stakeOpcode) != nil ||
		extractStakeScriptHash(script, stakeOpcode) != nil
}

// scriptType returns the type of the script being inspected from the known
// standard types.
//
// NOTE:  All scripts that are not version 0 are currently considered non
// standard.
func typeOfScript(scriptVersion uint16, script []byte) ScriptClass {
	if scriptVersion != 0 {
		return NonStandardTy
	}

	switch {
	case isPubKeyScript(script):
		return PubKeyTy
	case isPubKeyAltScript(script):
		return PubkeyAltTy
	case isPubKeyHashScript(script):
		return PubKeyHashTy
	case isPubKeyHashAltScript(script):
		return PubkeyHashAltTy
	case isScriptHashScript(script):
		return ScriptHashTy
	case isMultisigScript(scriptVersion, script):
		return MultiSigTy
	case isNullDataScript(scriptVersion, script):
		return NullDataTy
	case isStakeSubmissionScript(scriptVersion, script):
		return StakeSubmissionTy
	case isStakeGenScript(scriptVersion, script):
		return StakeGenTy
	case isStakeRevocationScript(scriptVersion, script):
		return StakeRevocationTy
	case isStakeChangeScript(scriptVersion, script):
		return StakeSubChangeTy
	}

	return NonStandardTy
}

// GetScriptClass returns the class of the script passed.
//
// NonStandardTy will be returned when the script does not parse.
func GetScriptClass(version uint16, script []byte) ScriptClass {
	if version != DefaultScriptVersion {
		return NonStandardTy
	}

	return typeOfScript(version, script)
}

// expectedInputs returns the number of arguments required by a script.  A value
// of -1 is returned if the script is of unknown type such that the number can
// not be determined.  This is an internal function and thus assumes that both
// class and subclass are correct for the script (and can thus assume things
// that were determined while finding out the type).
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func expectedInputs(script []byte, class ScriptClass, subclass ScriptClass) int {
	switch class {
	case PubKeyTy:
		return 1

	case PubKeyHashTy:
		return 2

	case StakeSubmissionTy:
		if subclass == PubKeyHashTy {
			return 2
		}
		return 1 // P2SH

	case StakeGenTy:
		if subclass == PubKeyHashTy {
			return 2
		}
		return 1 // P2SH

	case StakeRevocationTy:
		if subclass == PubKeyHashTy {
			return 2
		}
		return 1 // P2SH

	case StakeSubChangeTy:
		if subclass == PubKeyHashTy {
			return 2
		}
		return 1 // P2SH

	case ScriptHashTy:
		// Not including script, handled below.
		return 1

	case MultiSigTy:
		// Standard multisig has a small number push for the number of sigs and
		// number of keys.  Check the first push instruction to to see how many
		// arguments are expected. typeOfScript already checked this so we know
		// it'll be a small int.
		return asSmallInt(script[0])

	case NullDataTy:
		fallthrough
	default:
		return -1
	}
}

// isStakeOutput returns true is a script output is a stake type.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func isStakeOutput(pkScript []byte) bool {
	const scriptVersion = 0
	class := typeOfScript(scriptVersion, pkScript)
	return class == StakeSubmissionTy ||
		class == StakeGenTy ||
		class == StakeRevocationTy ||
		class == StakeSubChangeTy
}

// GetStakeOutSubclass extracts the subclass (P2PKH or P2SH)
// from a stake output.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func GetStakeOutSubclass(pkScript []byte) (ScriptClass, error) {
	const scriptVersion = 0
	if err := checkScriptParses(scriptVersion, pkScript); err != nil {
		return 0, err
	}

	class := typeOfScript(scriptVersion, pkScript)
	isStake := class == StakeSubmissionTy ||
		class == StakeGenTy ||
		class == StakeRevocationTy ||
		class == StakeSubChangeTy

	subClass := ScriptClass(0)
	if isStake {
		subClass = typeOfScript(scriptVersion, pkScript[1:])
	} else {
		return 0, fmt.Errorf("not a stake output")
	}

	return subClass, nil
}

// getStakeOutSubscript extracts the subscript (P2PKH or P2SH)
// from a stake output.
func getStakeOutSubscript(pkScript []byte) []byte {
	return pkScript[1:]
}

// ContainsStakeOpCodes returns whether or not a pkScript contains stake tagging
// OP codes.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func ContainsStakeOpCodes(pkScript []byte) (bool, error) {
	const scriptVersion = 0
	tokenizer := MakeScriptTokenizer(scriptVersion, pkScript)
	for tokenizer.Next() {
		if isStakeOpcode(tokenizer.Opcode()) {
			return true, nil
		}
	}

	return false, tokenizer.Err()
}

// CalcMultiSigStats returns the number of public keys and signatures from
// a multi-signature transaction script.  The passed script MUST already be
// known to be a multi-signature script.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func CalcMultiSigStats(script []byte) (int, int, error) {
	// The public keys are not needed here, so pass false to avoid the extra
	// allocation.
	const scriptVersion = 0
	details := extractMultisigScriptDetails(scriptVersion, script, false)
	if !details.valid {
		str := fmt.Sprintf("script %x is not a multisig script", script)
		return 0, 0, scriptError(ErrNotMultisigScript, str)
	}

	return details.numPubKeys, details.requiredSigs, nil
}

// MultisigRedeemScriptFromScriptSig attempts to extract a multi-signature
// redeem script from a P2SH-redeeming input.  The script is expected to already
// have been checked to be a multisignature script prior to calling this
// function.  The results are undefined for other script types.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func MultisigRedeemScriptFromScriptSig(script []byte) []byte {
	// The redeemScript is always the last item on the stack of the script sig.
	const scriptVersion = 0
	return finalOpcodeData(scriptVersion, script)
}

// payToPubKeyHashScript creates a new script to pay a transaction
// output to a 20-byte pubkey hash. It is expected that the input is a valid
// hash.
func payToPubKeyHashScript(pubKeyHash []byte) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_DUP).AddOp(OP_HASH160).
		AddData(pubKeyHash).AddOp(OP_EQUALVERIFY).AddOp(OP_CHECKSIG).
		Script()
}

// payToPubKeyHashEdwardsScript creates a new script to pay a transaction
// output to a 20-byte pubkey hash of an Edwards public key. It is expected
// that the input is a valid hash.
func payToPubKeyHashEdwardsScript(pubKeyHash []byte) ([]byte, error) {
	edwardsData := []byte{byte(dcrec.STEd25519)}
	return NewScriptBuilder().AddOp(OP_DUP).AddOp(OP_HASH160).
		AddData(pubKeyHash).AddOp(OP_EQUALVERIFY).AddData(edwardsData).
		AddOp(OP_CHECKSIGALT).Script()
}

// payToPubKeyHashSchnorrScript creates a new script to pay a transaction
// output to a 20-byte pubkey hash of a secp256k1 public key, but expecting
// a schnorr signature instead of a classic secp256k1 signature. It is
// expected that the input is a valid hash.
func payToPubKeyHashSchnorrScript(pubKeyHash []byte) ([]byte, error) {
	schnorrData := []byte{byte(dcrec.STSchnorrSecp256k1)}
	return NewScriptBuilder().AddOp(OP_DUP).AddOp(OP_HASH160).
		AddData(pubKeyHash).AddOp(OP_EQUALVERIFY).AddData(schnorrData).
		AddOp(OP_CHECKSIGALT).Script()
}

// payToScriptHashScript creates a new script to pay a transaction output to a
// script hash. It is expected that the input is a valid hash.
func payToScriptHashScript(scriptHash []byte) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_HASH160).AddData(scriptHash).
		AddOp(OP_EQUAL).Script()
}

// PayToScriptHashScript is the exported version of payToScriptHashScript.
func PayToScriptHashScript(scriptHash []byte) ([]byte, error) {
	return payToScriptHashScript(scriptHash)
}

// payToPubkeyScript creates a new script to pay a transaction output to a
// public key. It is expected that the input is a valid pubkey.
func payToPubKeyScript(serializedPubKey []byte) ([]byte, error) {
	return NewScriptBuilder().AddData(serializedPubKey).
		AddOp(OP_CHECKSIG).Script()
}

// payToEdwardsPubKeyScript creates a new script to pay a transaction output
// to an Ed25519 public key. It is expected that the input is a valid pubkey.
func payToEdwardsPubKeyScript(serializedPubKey []byte) ([]byte, error) {
	edwardsData := []byte{byte(dcrec.STEd25519)}
	return NewScriptBuilder().AddData(serializedPubKey).AddData(edwardsData).
		AddOp(OP_CHECKSIGALT).Script()
}

// payToSchnorrPubKeyScript creates a new script to pay a transaction output
// to a secp256k1 public key, but to be signed by Schnorr type signature. It
// is expected that the input is a valid pubkey.
func payToSchnorrPubKeyScript(serializedPubKey []byte) ([]byte, error) {
	schnorrData := []byte{byte(dcrec.STSchnorrSecp256k1)}
	return NewScriptBuilder().AddData(serializedPubKey).AddData(schnorrData).
		AddOp(OP_CHECKSIGALT).Script()
}

// PayToSStx creates a new script to pay a transaction output to a script hash or
// public key hash, but tags the output with OP_SSTX. For use in constructing
// valid SStxs.
func PayToSStx(addr dcrutil.Address) ([]byte, error) {
	// Only pay to pubkey hash and pay to script hash are
	// supported.
	scriptType := PubKeyHashTy
	switch addr := addr.(type) {
	case *dcrutil.AddressPubKeyHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		if addr.DSA(addr.Net()) != dcrec.STEcdsaSecp256k1 {
			str := "unable to generate payment script for " +
				"unsupported digital signature algorithm"
			return nil, scriptError(ErrUnsupportedAddress, str)
		}

	case *dcrutil.AddressScriptHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		scriptType = ScriptHashTy

	default:
		str := fmt.Sprintf("unable to generate payment script for "+
			"unsupported address type %T", addr)
		return nil, scriptError(ErrUnsupportedAddress, str)
	}

	hash := addr.ScriptAddress()

	if scriptType == PubKeyHashTy {
		return NewScriptBuilder().AddOp(OP_SSTX).AddOp(OP_DUP).
			AddOp(OP_HASH160).AddData(hash).AddOp(OP_EQUALVERIFY).
			AddOp(OP_CHECKSIG).Script()
	}
	return NewScriptBuilder().AddOp(OP_SSTX).AddOp(OP_HASH160).
		AddData(hash).AddOp(OP_EQUAL).Script()
}

// PayToSStxChange creates a new script to pay a transaction output to a
// public key hash, but tags the output with OP_SSTXCHANGE. For use in constructing
// valid SStxs.
func PayToSStxChange(addr dcrutil.Address) ([]byte, error) {
	// Only pay to pubkey hash and pay to script hash are
	// supported.
	scriptType := PubKeyHashTy
	switch addr := addr.(type) {
	case *dcrutil.AddressPubKeyHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		if addr.DSA(addr.Net()) != dcrec.STEcdsaSecp256k1 {
			str := "unable to generate payment script for " +
				"unsupported digital signature algorithm"
			return nil, scriptError(ErrUnsupportedAddress, str)
		}

	case *dcrutil.AddressScriptHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		scriptType = ScriptHashTy

	default:
		str := fmt.Sprintf("unable to generate payment script for "+
			"unsupported address type %T", addr)
		return nil, scriptError(ErrUnsupportedAddress, str)
	}

	hash := addr.ScriptAddress()

	if scriptType == PubKeyHashTy {
		return NewScriptBuilder().AddOp(OP_SSTXCHANGE).AddOp(OP_DUP).
			AddOp(OP_HASH160).AddData(hash).AddOp(OP_EQUALVERIFY).
			AddOp(OP_CHECKSIG).Script()
	}
	return NewScriptBuilder().AddOp(OP_SSTXCHANGE).AddOp(OP_HASH160).
		AddData(hash).AddOp(OP_EQUAL).Script()
}

// PayToSSGen creates a new script to pay a transaction output to a public key
// hash or script hash, but tags the output with OP_SSGEN. For use in constructing
// valid SSGen txs.
func PayToSSGen(addr dcrutil.Address) ([]byte, error) {
	// Only pay to pubkey hash and pay to script hash are
	// supported.
	scriptType := PubKeyHashTy
	switch addr := addr.(type) {
	case *dcrutil.AddressPubKeyHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		if addr.DSA(addr.Net()) != dcrec.STEcdsaSecp256k1 {
			str := "unable to generate payment script for " +
				"unsupported digital signature algorithm"
			return nil, scriptError(ErrUnsupportedAddress, str)
		}

	case *dcrutil.AddressScriptHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		scriptType = ScriptHashTy

	default:
		str := fmt.Sprintf("unable to generate payment script for "+
			"unsupported address type %T", addr)
		return nil, scriptError(ErrUnsupportedAddress, str)
	}

	hash := addr.ScriptAddress()

	if scriptType == PubKeyHashTy {
		return NewScriptBuilder().AddOp(OP_SSGEN).AddOp(OP_DUP).
			AddOp(OP_HASH160).AddData(hash).AddOp(OP_EQUALVERIFY).
			AddOp(OP_CHECKSIG).Script()
	}
	return NewScriptBuilder().AddOp(OP_SSGEN).AddOp(OP_HASH160).
		AddData(hash).AddOp(OP_EQUAL).Script()
}

// PayToSSGenPKHDirect creates a new script to pay a transaction output to a
// public key hash, but tags the output with OP_SSGEN. For use in constructing
// valid SSGen txs. Unlike PayToSSGen, this function directly uses the HASH160
// pubkeyhash (instead of an address).
func PayToSSGenPKHDirect(pkh []byte) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_SSGEN).AddOp(OP_DUP).
		AddOp(OP_HASH160).AddData(pkh).AddOp(OP_EQUALVERIFY).
		AddOp(OP_CHECKSIG).Script()
}

// PayToSSGenSHDirect creates a new script to pay a transaction output to a
// script hash, but tags the output with OP_SSGEN. For use in constructing
// valid SSGen txs. Unlike PayToSSGen, this function directly uses the HASH160
// script hash (instead of an address).
func PayToSSGenSHDirect(sh []byte) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_SSGEN).AddOp(OP_HASH160).
		AddData(sh).AddOp(OP_EQUAL).Script()
}

// PayToSSRtx creates a new script to pay a transaction output to a
// public key hash, but tags the output with OP_SSRTX. For use in constructing
// valid SSRtx.
func PayToSSRtx(addr dcrutil.Address) ([]byte, error) {
	// Only pay to pubkey hash and pay to script hash are
	// supported.
	scriptType := PubKeyHashTy
	switch addr := addr.(type) {
	case *dcrutil.AddressPubKeyHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		if addr.DSA(addr.Net()) != dcrec.STEcdsaSecp256k1 {
			str := "unable to generate payment script for " +
				"unsupported digital signature algorithm"
			return nil, scriptError(ErrUnsupportedAddress, str)
		}

	case *dcrutil.AddressScriptHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		scriptType = ScriptHashTy

	default:
		str := fmt.Sprintf("unable to generate payment script for "+
			"unsupported address type %T", addr)
		return nil, scriptError(ErrUnsupportedAddress, str)
	}

	hash := addr.ScriptAddress()

	if scriptType == PubKeyHashTy {
		return NewScriptBuilder().AddOp(OP_SSRTX).AddOp(OP_DUP).
			AddOp(OP_HASH160).AddData(hash).AddOp(OP_EQUALVERIFY).
			AddOp(OP_CHECKSIG).Script()
	}
	return NewScriptBuilder().AddOp(OP_SSRTX).AddOp(OP_HASH160).
		AddData(hash).AddOp(OP_EQUAL).Script()
}

// PayToSSRtxPKHDirect creates a new script to pay a transaction output to a
// public key hash, but tags the output with OP_SSRTX. For use in constructing
// valid SSRtx. Unlike PayToSSRtx, this function directly uses the HASH160
// pubkeyhash (instead of an address).
func PayToSSRtxPKHDirect(pkh []byte) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_SSRTX).AddOp(OP_DUP).
		AddOp(OP_HASH160).AddData(pkh).AddOp(OP_EQUALVERIFY).
		AddOp(OP_CHECKSIG).Script()
}

// PayToSSRtxSHDirect creates a new script to pay a transaction output to a
// script hash, but tags the output with OP_SSRTX. For use in constructing
// valid SSRtx. Unlike PayToSSRtx, this function directly uses the HASH160
// script hash (instead of an address).
func PayToSSRtxSHDirect(sh []byte) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_SSRTX).AddOp(OP_HASH160).
		AddData(sh).AddOp(OP_EQUAL).Script()
}

// GenerateSStxAddrPush generates an OP_RETURN push for SSGen payment addresses in
// an SStx.
func GenerateSStxAddrPush(addr dcrutil.Address, amount dcrutil.Amount, limits uint16) ([]byte, error) {
	// Only pay to pubkey hash and pay to script hash are
	// supported.
	scriptType := PubKeyHashTy
	switch addr := addr.(type) {
	case *dcrutil.AddressPubKeyHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		if addr.DSA(addr.Net()) != dcrec.STEcdsaSecp256k1 {
			str := "unable to generate payment script for " +
				"unsupported digital signature algorithm"
			return nil, scriptError(ErrUnsupportedAddress, str)
		}

	case *dcrutil.AddressScriptHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		scriptType = ScriptHashTy

	default:
		str := fmt.Sprintf("unable to generate payment script for "+
			"unsupported address type %T", addr)
		return nil, scriptError(ErrUnsupportedAddress, str)
	}

	// Concatenate the prefix, pubkeyhash, and amount.
	adBytes := make([]byte, 20+8+2)
	copy(adBytes[0:20], addr.ScriptAddress())
	binary.LittleEndian.PutUint64(adBytes[20:28], uint64(amount))
	binary.LittleEndian.PutUint16(adBytes[28:30], limits)

	// Set the bit flag indicating pay to script hash.
	if scriptType == ScriptHashTy {
		adBytes[27] |= 1 << 7
	}

	return NewScriptBuilder().AddOp(OP_RETURN).AddData(adBytes).Script()
}

// GenerateSSGenBlockRef generates an OP_RETURN push for the block header hash and
// height which the block votes on.
func GenerateSSGenBlockRef(blockHash chainhash.Hash, height uint32) ([]byte, error) {
	// Serialize the block hash and height
	brBytes := make([]byte, 32+4)
	copy(brBytes[0:32], blockHash[:])
	binary.LittleEndian.PutUint32(brBytes[32:36], height)

	return NewScriptBuilder().AddOp(OP_RETURN).AddData(brBytes).Script()
}

// GenerateSSGenVotes generates an OP_RETURN push for the vote bits in an SSGen tx.
func GenerateSSGenVotes(votebits uint16) ([]byte, error) {
	// Serialize the votebits
	vbBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(vbBytes, votebits)

	return NewScriptBuilder().AddOp(OP_RETURN).AddData(vbBytes).Script()
}

// GenerateProvablyPruneableOut creates a provably-prunable script containing
// OP_RETURN followed by the passed data.  An Error with the error code
// ErrTooMuchNullData will be returned if the length of the passed data exceeds
// MaxDataCarrierSize.
func GenerateProvablyPruneableOut(data []byte) ([]byte, error) {
	if len(data) > MaxDataCarrierSize {
		str := fmt.Sprintf("data size %d is larger than max "+
			"allowed size %d", len(data), MaxDataCarrierSize)
		return nil, scriptError(ErrTooMuchNullData, str)
	}

	return NewScriptBuilder().AddOp(OP_RETURN).AddData(data).Script()
}

// PayToAddrScript creates a new script to pay a transaction output to a the
// specified address.
func PayToAddrScript(addr dcrutil.Address) ([]byte, error) {
	switch addr := addr.(type) {
	case *dcrutil.AddressPubKeyHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		switch addr.DSA(addr.Net()) {
		case dcrec.STEcdsaSecp256k1:
			return payToPubKeyHashScript(addr.ScriptAddress())
		case dcrec.STEd25519:
			return payToPubKeyHashEdwardsScript(addr.ScriptAddress())
		case dcrec.STSchnorrSecp256k1:
			return payToPubKeyHashSchnorrScript(addr.ScriptAddress())
		}

	case *dcrutil.AddressScriptHash:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		return payToScriptHashScript(addr.ScriptAddress())

	case *dcrutil.AddressSecpPubKey:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		return payToPubKeyScript(addr.ScriptAddress())

	case *dcrutil.AddressEdwardsPubKey:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		return payToEdwardsPubKeyScript(addr.ScriptAddress())

	case *dcrutil.AddressSecSchnorrPubKey:
		if addr == nil {
			return nil, scriptError(ErrUnsupportedAddress,
				nilAddrErrStr)
		}
		return payToSchnorrPubKeyScript(addr.ScriptAddress())
	}

	str := fmt.Sprintf("unable to generate payment script for unsupported "+
		"address type %T", addr)
	return nil, scriptError(ErrUnsupportedAddress, str)
}

// MultiSigScript returns a valid script for a multisignature redemption where
// nrequired of the keys in pubkeys are required to have signed the transaction
// for success.  An Error with the error code ErrTooManyRequiredSigs will be
// returned if nrequired is larger than the number of keys provided.
func MultiSigScript(pubkeys []*dcrutil.AddressSecpPubKey, nrequired int) ([]byte, error) {
	if len(pubkeys) < nrequired {
		str := fmt.Sprintf("unable to generate multisig script with "+
			"%d required signatures when there are only %d public "+
			"keys available", nrequired, len(pubkeys))
		return nil, scriptError(ErrTooManyRequiredSigs, str)
	}

	builder := NewScriptBuilder().AddInt64(int64(nrequired))
	for _, key := range pubkeys {
		builder.AddData(key.ScriptAddress())
	}
	builder.AddInt64(int64(len(pubkeys)))
	builder.AddOp(OP_CHECKMULTISIG)

	return builder.Script()
}

// PushedData returns an array of byte slices containing any pushed data found
// in the passed script.  This includes OP_0, but not OP_1 - OP_16.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func PushedData(script []byte) ([][]byte, error) {
	const scriptVersion = 0

	var data [][]byte
	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	for tokenizer.Next() {
		if tokenizer.Data() != nil {
			data = append(data, tokenizer.Data())
		} else if tokenizer.Opcode() == OP_0 {
			data = append(data, nil)
		}
	}
	if err := tokenizer.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

// pubKeyHashToAddrs is a convenience function to attempt to convert the
// passed hash to a pay-to-pubkey-hash address housed within an address
// slice.  It is used to consolidate common code.
func pubKeyHashToAddrs(hash []byte, params *chaincfg.Params) []dcrutil.Address {
	// Skip the pubkey hash if it's invalid for some reason.
	var addrs []dcrutil.Address
	addr, err := dcrutil.NewAddressPubKeyHash(hash, params,
		dcrec.STEcdsaSecp256k1)
	if err == nil {
		addrs = append(addrs, addr)
	}
	return addrs
}

// scriptHashToAddrs is a convenience function to attempt to convert the passed
// hash to a pay-to-script-hash address housed within an address slice.  It is
// used to consolidate common code.
func scriptHashToAddrs(hash []byte, params *chaincfg.Params) []dcrutil.Address {
	// Skip the hash if it's invalid for some reason.
	var addrs []dcrutil.Address
	addr, err := dcrutil.NewAddressScriptHashFromHash(hash, params)
	if err == nil {
		addrs = append(addrs, addr)
	}
	return addrs
}

// ExtractPkScriptAddrs returns the type of script, addresses and required
// signatures associated with the passed PkScript.  Note that it only works for
// 'standard' transaction script types.  Any data such as public keys which are
// invalid are omitted from the results.
//
// NOTE: This function only attempts to identify version 0 scripts.  The return
// value will indicate a nonstandard script type for other script versions along
// with an invalid script version error.
func ExtractPkScriptAddrs(version uint16, pkScript []byte,
	chainParams *chaincfg.Params) (ScriptClass, []dcrutil.Address, int, error) {
	if version != 0 {
		return NonStandardTy, nil, 0, fmt.Errorf("invalid script version")
	}

	// Check for pay-to-pubkey-hash script.
	if hash := extractPubKeyHash(pkScript); hash != nil {
		return PubKeyHashTy, pubKeyHashToAddrs(hash, chainParams), 1, nil
	}

	// Check for pay-to-script-hash.
	if hash := extractScriptHash(pkScript); hash != nil {
		return ScriptHashTy, scriptHashToAddrs(hash, chainParams), 1, nil
	}

	// Check for pay-to-alt-pubkey-hash script.
	if data, sigType := extractPubKeyHashAltDetails(pkScript); data != nil {
		var addrs []dcrutil.Address
		addr, err := dcrutil.NewAddressPubKeyHash(data, chainParams, sigType)
		if err == nil {
			addrs = append(addrs, addr)
		}
		return PubkeyHashAltTy, addrs, 1, nil
	}

	// Check for pay-to-pubkey script.
	if data := extractPubKey(pkScript); data != nil {
		var addrs []dcrutil.Address
		pk, err := secp256k1.ParsePubKey(data)
		if err == nil {
			addr, err := dcrutil.NewAddressSecpPubKeyCompressed(pk, chainParams)
			if err == nil {
				addrs = append(addrs, addr)
			}
		}
		return PubKeyTy, addrs, 1, nil
	}

	// Check for pay-to-alt-pubkey script.
	if pk, sigType := extractPubKeyAltDetails(pkScript); pk != nil {
		var addrs []dcrutil.Address
		switch sigType {
		case dcrec.STEd25519:
			addr, err := dcrutil.NewAddressEdwardsPubKey(pk, chainParams)
			if err == nil {
				addrs = append(addrs, addr)
			}

		case dcrec.STSchnorrSecp256k1:
			addr, err := dcrutil.NewAddressSecSchnorrPubKey(pk, chainParams)
			if err == nil {
				addrs = append(addrs, addr)
			}
		}

		return PubkeyAltTy, addrs, 1, nil
	}

	// Check for multi-signature script.
	details := extractMultisigScriptDetails(version, pkScript, true)
	if details.valid {
		// Convert the public keys while skipping any that are invalid.
		addrs := make([]dcrutil.Address, 0, details.numPubKeys)
		for i := 0; i < details.numPubKeys; i++ {
			pubkey, err := secp256k1.ParsePubKey(details.pubKeys[i])
			if err == nil {
				addr, err := dcrutil.NewAddressSecpPubKeyCompressed(pubkey,
					chainParams)
				if err == nil {
					addrs = append(addrs, addr)
				}
			}
		}
		return MultiSigTy, addrs, details.requiredSigs, nil
	}

	// Check for stake submission script.  Only stake-submission-tagged
	// pay-to-pubkey-hash and pay-to-script-hash are allowed.
	if hash := extractStakePubKeyHash(pkScript, OP_SSTX); hash != nil {
		return StakeSubmissionTy, pubKeyHashToAddrs(hash, chainParams), 1, nil
	}
	if hash := extractStakeScriptHash(pkScript, OP_SSTX); hash != nil {
		return StakeSubmissionTy, scriptHashToAddrs(hash, chainParams), 1, nil
	}

	// Check for stake generation script.  Only stake-generation-tagged
	// pay-to-pubkey-hash and pay-to-script-hash are allowed.
	if hash := extractStakePubKeyHash(pkScript, OP_SSGEN); hash != nil {
		return StakeGenTy, pubKeyHashToAddrs(hash, chainParams), 1, nil
	}
	if hash := extractStakeScriptHash(pkScript, OP_SSGEN); hash != nil {
		return StakeGenTy, scriptHashToAddrs(hash, chainParams), 1, nil
	}

	// Check for stake revocation script.  Only stake-revocation-tagged
	// pay-to-pubkey-hash and pay-to-script-hash are allowed.
	if hash := extractStakePubKeyHash(pkScript, OP_SSRTX); hash != nil {
		return StakeRevocationTy, pubKeyHashToAddrs(hash, chainParams), 1, nil
	}
	if hash := extractStakeScriptHash(pkScript, OP_SSRTX); hash != nil {
		return StakeRevocationTy, scriptHashToAddrs(hash, chainParams), 1, nil
	}

	// Check for stake change script.  Only stake-change-tagged
	// pay-to-pubkey-hash and pay-to-script-hash are allowed.
	if hash := extractStakePubKeyHash(pkScript, OP_SSTXCHANGE); hash != nil {
		return StakeSubChangeTy, pubKeyHashToAddrs(hash, chainParams), 1, nil
	}
	if hash := extractStakeScriptHash(pkScript, OP_SSTXCHANGE); hash != nil {
		return StakeSubChangeTy, scriptHashToAddrs(hash, chainParams), 1, nil
	}

	// Check for null data script.
	if isNullDataScript(version, pkScript) {
		// Null data transactions have no addresses or required signatures.
		return NullDataTy, nil, 0, nil
	}

	// Don't attempt to extract addresses or required signatures for nonstandard
	// transactions.
	return NonStandardTy, nil, 0, nil
}

// ExtractPkScriptAltSigType returns the signature scheme to use for an
// alternative check signature script.
//
// NOTE: This function only attempts to identify version 0 scripts.  Since the
// function does not accept a script version, the results are undefined for
// other script versions.
func ExtractPkScriptAltSigType(pkScript []byte) (dcrec.SignatureType, error) {
	if pk, sigType := extractPubKeyAltDetails(pkScript); pk != nil {
		return sigType, nil
	}

	if pk, sigType := extractPubKeyHashAltDetails(pkScript); pk != nil {
		return sigType, nil
	}

	return -1, fmt.Errorf("not a standard pay-to-alt-pubkey or " +
		"pay-to-alt-pubkey-hash script")
}

// AtomicSwapDataPushes houses the data pushes found in atomic swap contracts.
type AtomicSwapDataPushes struct {
	RecipientHash160 [20]byte
	RefundHash160    [20]byte
	SecretHash       [32]byte
	SecretSize       int64
	LockTime         int64
}

// ExtractAtomicSwapDataPushes returns the data pushes from an atomic swap
// contract.  If the script is not an atomic swap contract,
// ExtractAtomicSwapDataPushes returns (nil, nil).  Non-nil errors are returned
// for unparsable scripts.
//
// NOTE: Atomic swaps are not considered standard script types by the dcrd
// mempool policy and should be used with P2SH.  The atomic swap format is also
// expected to change to use a more secure hash function in the future.
//
// This function is only defined in the txscript package due to API limitations
// which prevent callers using txscript to parse nonstandard scripts.
//
// DEPRECATED.  This will be removed in the next major version bump.  The error
// should also likely be removed if the code is reimplemented by any callers
// since any errors result in a nil result anyway.
func ExtractAtomicSwapDataPushes(version uint16, pkScript []byte) (*AtomicSwapDataPushes, error) {
	// An atomic swap is of the form:
	//  IF
	//   SIZE <secret size> EQUALVERIFY SHA256 <32-byte secret> EQUALVERIFY DUP
	//   HASH160 <20-byte recipient hash>
	//  ELSE
	//   <locktime> CHECKLOCKTIMEVERIFY DROP DUP HASH160 <20-byte refund hash>
	//  ENDIF
	//  EQUALVERIFY CHECKSIG
	type templateMatch struct {
		expectCanonicalInt bool
		maxIntBytes        int
		opcode             byte
		extractedInt       int64
		extractedData      []byte
	}
	var template = [20]templateMatch{
		{opcode: OP_IF},
		{opcode: OP_SIZE},
		{expectCanonicalInt: true, maxIntBytes: mathOpCodeMaxScriptNumLen},
		{opcode: OP_EQUALVERIFY},
		{opcode: OP_SHA256},
		{opcode: OP_DATA_32},
		{opcode: OP_EQUALVERIFY},
		{opcode: OP_DUP},
		{opcode: OP_HASH160},
		{opcode: OP_DATA_20},
		{opcode: OP_ELSE},
		{expectCanonicalInt: true, maxIntBytes: cltvMaxScriptNumLen},
		{opcode: OP_CHECKLOCKTIMEVERIFY},
		{opcode: OP_DROP},
		{opcode: OP_DUP},
		{opcode: OP_HASH160},
		{opcode: OP_DATA_20},
		{opcode: OP_ENDIF},
		{opcode: OP_EQUALVERIFY},
		{opcode: OP_CHECKSIG},
	}

	var templateOffset int
	tokenizer := MakeScriptTokenizer(version, pkScript)
	for tokenizer.Next() {
		// Not an atomic swap script if it has more opcodes than expected in the
		// template.
		if templateOffset >= len(template) {
			return nil, nil
		}

		op := tokenizer.Opcode()
		data := tokenizer.Data()
		tplEntry := &template[templateOffset]
		if tplEntry.expectCanonicalInt {
			switch {
			case data != nil:
				val, err := makeScriptNum(data, tplEntry.maxIntBytes)
				if err != nil {
					return nil, err
				}
				tplEntry.extractedInt = int64(val)

			case isSmallInt(op):
				tplEntry.extractedInt = int64(asSmallInt(op))

			// Not an atomic swap script if the opcode does not push an int.
			default:
				return nil, nil
			}
		} else {
			if op != tplEntry.opcode {
				return nil, nil
			}

			tplEntry.extractedData = data
		}

		templateOffset++
	}
	if err := tokenizer.Err(); err != nil {
		return nil, err
	}
	if !tokenizer.Done() || templateOffset != len(template) {
		return nil, nil
	}

	// At this point, the script appears to be an atomic swap, so populate and
	// return the extacted data.
	pushes := AtomicSwapDataPushes{
		SecretSize: template[2].extractedInt,
		LockTime:   template[11].extractedInt,
	}
	copy(pushes.SecretHash[:], template[5].extractedData)
	copy(pushes.RecipientHash160[:], template[9].extractedData)
	copy(pushes.RefundHash160[:], template[16].extractedData)
	return &pushes, nil
}
