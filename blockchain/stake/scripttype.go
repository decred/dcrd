// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"fmt"

	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/txscript/v3"
)

// extractScriptHash extracts the script hash from the passed script if it is a
// pay-to-script-hash script.  It will return nil otherwise.
//
// NOTE: This function is only valid for version 0 opcodes.
func extractScriptHash(script []byte) []byte {
	// A pay-to-script-hash script is of the form:
	//  OP_HASH160 <20-byte scripthash> OP_EQUAL
	if len(script) == 23 &&
		script[0] == txscript.OP_HASH160 &&
		script[1] == txscript.OP_DATA_20 &&
		script[22] == txscript.OP_EQUAL {
		return script[2:22]
	}
	return nil
}

// isScriptHashScript returns whether or not the passed script is a
// pay-to-script-hash script per consensus rules.
//
// NOTE: This function is only valid for version 0 opcodes.
func isScriptHashScript(script []byte) bool {
	return extractScriptHash(script) != nil
}

// extractPubKeyHash extracts the public key hash from the passed script if it
// is a standard pay-to-pubkey-hash script.  It will return nil otherwise.
//
// NOTE: This function is only valid for version 0 opcodes.
func extractPubKeyHash(script []byte) []byte {
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

// extractStakeScriptHash extracts a script hash from the passed public key
// script if it is a standard pay-to-script-hash script tagged with the provided
// stake opcode.  It will return nil otherwise.
//
// NOTE: This function is only valid for version 0 opcodes.
func extractStakeScriptHash(script []byte, stakeOpcode byte) []byte {
	if len(script) == 24 &&
		script[0] == stakeOpcode &&
		script[1] == txscript.OP_HASH160 &&
		script[2] == txscript.OP_DATA_20 &&
		script[23] == txscript.OP_EQUAL {
		return script[3:23]
	}
	return nil
}

// isPubKeyHashScript returns whether or not the passed script is
// a pay-to-pubkey-hash script per consensus rules.
func isPubKeyHashScript(script []byte) bool {
	return extractPubKeyHash(script) != nil
}

// isSmallInt returns whether or not the opcode is considered a small integer,
// which is an OP_0, or OP_1 through OP_16.
//
// NOTE: This function is only valid for version 0 opcodes.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func isSmallInt(op byte) bool {
	return op == txscript.OP_0 || (op >= txscript.OP_1 && op <= txscript.OP_16)
}

// asSmallInt returns the passed opcode, which must be true according to
// isSmallInt(), as an integer.
func asSmallInt(op byte) int {
	if op == txscript.OP_0 {
		return 0
	}
	return int(op - (txscript.OP_1 - 1))
}

// isStandardAltSignatureType returns whether or not the provided opcode
// represents a push of a alt signature type.
func isStandardAltSignatureType(op byte) bool {
	if !isSmallInt(op) {
		return false
	}
	sigType := asSmallInt(op)
	return sigType == dcrec.STEd25519 || sigType == dcrec.STSchnorrSecp256k1
}

// extractPubKeyHashAltDetails extracts the public key hash and signature type
// from the passed script if it is a pay-to-alt-pubkey-hash script.  It
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
		script[0] == txscript.OP_DUP &&
		script[1] == txscript.OP_HASH160 &&
		script[2] == txscript.OP_DATA_20 &&
		script[23] == txscript.OP_EQUALVERIFY &&
		isStandardAltSignatureType(script[24]) &&
		script[25] == txscript.OP_CHECKSIGALT {
		return script[3:23], dcrec.SignatureType(asSmallInt(script[24]))
	}
	return nil, 0
}

// extractPubKey extracts either a compressed or uncompressed public key from the
// passed script if it is either a standard pay-to-compressed-secp256k1-pubkey
// or pay-to-uncompressed-secp256k1-pubkey script, respectively.  It will return
// nil otherwise.
func extractPubKey(script []byte) []byte {
	var pk []byte

	// A pay-to-compressed-pubkey script is of the form:
	//  OP_DATA_33 <33-byte compressed pubkey> OP_CHECKSIG
	//
	// All compressed secp256k1 public keys must start with 0x02 or 0x03.
	if len(script) == 35 &&
		script[34] == txscript.OP_CHECKSIG &&
		script[0] == txscript.OP_DATA_33 &&
		(script[1] == 0x02 || script[1] == 0x03) {
		pk = script[1:34]
	}

	if pk == nil {
		// A pay-to-uncompressed-pubkey script is of the form:
		//  OP_DATA_65 <65-byte uncompressed pubkey> OP_CHECKSIG
		//
		// All non-hybrid uncompressed secp256k1 public keys must start with 0x04.
		if len(script) == 67 &&
			script[66] == txscript.OP_CHECKSIG &&
			script[0] == txscript.OP_DATA_65 &&
			script[1] == 0x04 {
			pk = script[1:66]
		}
	}

	return pk
}

// isStrictPubKeyEncoding returns whether or not the passed public key adheres
// to the strict encoding requirements.
func isStrictPubKeyEncoding(pubKey []byte) bool {
	// Check for a compressed public key.
	if len(pubKey) == 33 && (pubKey[0] == 0x02 || pubKey[0] == 0x03) {
		return true
	}

	// Check for an uncompressed public key.
	if len(pubKey) == 65 && pubKey[0] == 0x04 {
		return true
	}

	return false
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
	if len(script) < 3 || script[len(script)-1] != txscript.OP_CHECKSIGALT {
		return nil, 0
	}

	if len(script) == 35 && script[0] == txscript.OP_DATA_32 &&
		isSmallInt(script[33]) && asSmallInt(script[33]) == dcrec.STEd25519 {

		return script[1:33], dcrec.STEd25519
	}

	if len(script) == 36 && script[0] == txscript.OP_DATA_33 &&
		isSmallInt(script[34]) &&
		asSmallInt(script[34]) == dcrec.STSchnorrSecp256k1 &&
		isStrictPubKeyEncoding(script[1:34]) {

		return script[1:34], dcrec.STSchnorrSecp256k1
	}

	return nil, 0
}

// extractStakePubKeyHash extracts a pubkey hash from the passed public key
// script if it is a standard pay-to-pubkey-hash script tagged with the provided
// stake opcode.  It will return nil otherwise.
func extractStakePubKeyHash(script []byte, stakeOpcode byte) []byte {
	if len(script) == 26 &&
		script[0] == stakeOpcode &&
		script[1] == txscript.OP_DUP &&
		script[2] == txscript.OP_HASH160 &&
		script[3] == txscript.OP_DATA_20 &&
		script[24] == txscript.OP_EQUALVERIFY &&
		script[25] == txscript.OP_CHECKSIG {
		return script[4:24]
	}
	return nil
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

// IsNullDataScript returns whether or not the passed script is a null
// data script.
//
// NOTE: This function is only valid for version 0 scripts.  It will always
// return false for other script versions.
func IsNullDataScript(scriptVersion uint16, script []byte) bool {
	// The only supported script version is 0.
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
	if len(script) < 1 || script[0] != txscript.OP_RETURN {
		return false
	}

	// Single OP_RETURN.
	if len(script) == 1 {
		return true
	}

	// OP_RETURN followed by data push up to MaxDataCarrierSize bytes.
	tokenizer := txscript.MakeScriptTokenizer(scriptVersion, script[1:])
	return tokenizer.Next() && tokenizer.Done() &&
		(isSmallInt(tokenizer.Opcode()) || tokenizer.Opcode() <= txscript.OP_PUSHDATA4) &&
		len(tokenizer.Data()) <= txscript.MaxDataCarrierSize
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
	// The only supported version is 0.
	if scriptVersion != 0 {
		return multiSigDetails{}
	}

	// A multi-signature script is of the form:
	//  NUM_SIGS PUBKEY PUBKEY PUBKEY ... NUM_PUBKEYS OP_CHECKMULTISIG
	//
	// It therefore must end with OP_CHECKMULTISIG and have at least two
	// small integer pushes preceding it.
	if len(script) < 3 || script[len(script)-1] != txscript.OP_CHECKMULTISIG {
		return multiSigDetails{}
	}

	// The first opcode must be a small integer specifying the number of
	// signatures required.
	tokenizer := txscript.MakeScriptTokenizer(scriptVersion, script)
	if !tokenizer.Next() || !isSmallInt(tokenizer.Opcode()) {
		return multiSigDetails{}
	}
	requiredSigs := asSmallInt(tokenizer.Opcode())

	// The next series of opcodes must either push public keys or be a small
	// integer specifying the number of public keys.
	var numPubKeys int
	var pubKeys [][]byte
	if extractPubKeys {
		pubKeys = make([][]byte, 0, txscript.MaxPubKeysPerMultiSig)
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
	details := extractMultisigScriptDetails(scriptVersion, script, false)
	return details.valid
}

// finalOpcodeData returns the data associated with the final opcode in the
// script.  It will return nil if the script fails to parse.
func finalOpcodeData(scriptVersion uint16, script []byte) []byte {
	// The only supported script version is 0.
	if len(script) == 0 {
		return nil
	}

	var data []byte
	tokenizer := txscript.MakeScriptTokenizer(scriptVersion, script)
	for tokenizer.Next() {
		data = tokenizer.Data()
	}
	if tokenizer.Err() != nil {
		return nil
	}
	return data
}

// IsMultisigSigScript returns whether or not the passed script appears to be a
// signature script which consists of a pay-to-script-hash multi-signature
// redeem script.
func IsMultisigSigScript(scriptVersion uint16, script []byte) bool {
	// The only supported script version is 0.
	if len(script) == 0 {
		return false
	}

	// A multisig signature script must end with OP_CHECKMULTISIG in the
	// redeem script and  have at least two small integers preceding it.
	// The redeem script itself must be preceded by at least a data
	// push opcode.
	if len(script) < 4 || script[len(script)-1] != txscript.OP_CHECKMULTISIG {
		return false
	}

	possibleRedeemScript := finalOpcodeData(scriptVersion, script)
	if possibleRedeemScript == nil {
		return false
	}

	return isMultisigScript(scriptVersion, possibleRedeemScript)
}

// MultisigRedeemScriptFromScriptSig attempts to extract a multi-signature
// redeem script from a P2SH-redeeming input.  The script is expected to already
// have been checked to be a multisignature script prior to calling this
// function.  The results are undefined for other script types.
//
// NOTE: This function is only valid for version 0 scripts.
func MultisigRedeemScriptFromScriptSig(version uint16, script []byte) []byte {
	// The redeemScript is always the last item on the stack of the script sig.
	return finalOpcodeData(version, script)
}

// ExtractPkScriptAddrs returns the type of script, addresses and required
// signatures associated with the passed PkScript.  Note that it only works for
// ransaction script types accepted as consensus.  Any data such as public
// keys which are invalid are omitted from the results.
//
// NOTE: This function only attempts to identify version 0 scripts.  The return
// value will indicate a nonstandard script type for other script versions along
// with an invalid script version error.
func ExtractPkScriptAddrs(version uint16, pkScript []byte,
	chainParams dcrutil.AddressParams) (txscript.ScriptClass, []dcrutil.Address, int, error) {
	if version != 0 {
		return txscript.NonStandardTy, nil, 0, fmt.Errorf("invalid script version")
	}

	// Check for pay-to-pubkey-hash script.
	if hash := extractPubKeyHash(pkScript); hash != nil {
		addr, err := dcrutil.NewAddressPubKeyHash(hash, chainParams,
			dcrec.STEcdsaSecp256k1)
		if err != nil {
			return txscript.NonStandardTy, nil, 0, err
		}
		return txscript.PubKeyHashTy, []dcrutil.Address{addr}, 1, nil
	}

	// Check for pay-to-script-hash.
	if hash := extractScriptHash(pkScript); hash != nil {
		addr, err := dcrutil.NewAddressScriptHashFromHash(hash, chainParams)
		if err != nil {
			return txscript.NonStandardTy, nil, 0, err
		}
		return txscript.ScriptHashTy, []dcrutil.Address{addr}, 1, nil
	}

	// Check for pay-to-alt-pubkey-hash script.
	if data, sigType := extractPubKeyHashAltDetails(pkScript); data != nil {
		addr, err := dcrutil.NewAddressPubKeyHash(data, chainParams, sigType)
		if err != nil {
			return txscript.NonStandardTy, nil, 0, err
		}
		return txscript.PubkeyHashAltTy, []dcrutil.Address{addr}, 1, nil
	}

	// Check for pay-to-pubkey script.
	if data := extractPubKey(pkScript); data != nil {
		pk, err := secp256k1.ParsePubKey(data)
		if err != nil {
			return txscript.NonStandardTy, nil, 0, err
		}
		addr, err := dcrutil.NewAddressSecpPubKeyCompressed(pk, chainParams)
		if err != nil {
			return txscript.NonStandardTy, nil, 0, err
		}
		return txscript.PubKeyTy, []dcrutil.Address{addr}, 1, nil
	}

	// Check for pay-to-alt-pubkey script.
	if pk, sigType := extractPubKeyAltDetails(pkScript); pk != nil {
		switch sigType {
		case dcrec.STEd25519:
			addr, err := dcrutil.NewAddressEdwardsPubKey(pk, chainParams)
			if err != nil {
				return txscript.NonStandardTy, nil, 0, err
			}
			return txscript.PubkeyAltTy, []dcrutil.Address{addr}, 1, nil

		case dcrec.STSchnorrSecp256k1:
			addr, err := dcrutil.NewAddressSecSchnorrPubKey(pk, chainParams)
			if err == nil {
				return txscript.NonStandardTy, nil, 0, err
			}
			return txscript.PubkeyAltTy, []dcrutil.Address{addr}, 1, nil

		default:
			return txscript.NonStandardTy, nil, 0,
				fmt.Errorf("unknown pay-to-alt-pubkey script type")
		}
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
		return txscript.MultiSigTy, addrs, details.requiredSigs, nil
	}

	// Check for stake submission script.  Only stake-submission-tagged
	// pay-to-pubkey-hash and pay-to-script-hash are allowed.
	if hash := extractStakePubKeyHash(pkScript, txscript.OP_SSTX); hash != nil {
		addr, err := dcrutil.NewAddressPubKeyHash(hash, chainParams,
			dcrec.STEcdsaSecp256k1)
		if err != nil {
			return txscript.NonStandardTy, nil, 0, err
		}
		return txscript.StakeSubmissionTy, []dcrutil.Address{addr}, 1, nil
	}

	if hash := extractStakeScriptHash(pkScript, txscript.OP_SSTX); hash != nil {
		addr, err := dcrutil.NewAddressScriptHashFromHash(hash, chainParams)
		if err != nil {
			return txscript.NonStandardTy, nil, 0, err
		}
		return txscript.StakeSubmissionTy, []dcrutil.Address{addr}, 1, nil
	}

	// Check for stake generation script.  Only stake-generation-tagged
	// pay-to-pubkey-hash and pay-to-script-hash are allowed.
	if hash := extractStakePubKeyHash(pkScript, txscript.OP_SSGEN); hash != nil {
		addr, err := dcrutil.NewAddressPubKeyHash(hash, chainParams,
			dcrec.STEcdsaSecp256k1)
		if err != nil {
			return txscript.NonStandardTy, nil, 0, err
		}
		return txscript.StakeGenTy, []dcrutil.Address{addr}, 1, nil
	}
	if hash := extractStakeScriptHash(pkScript, txscript.OP_SSGEN); hash != nil {
		addr, err := dcrutil.NewAddressScriptHashFromHash(hash, chainParams)
		if err != nil {
			return txscript.NonStandardTy, nil, 0, err
		}
		return txscript.StakeGenTy, []dcrutil.Address{addr}, 1, nil
	}

	// Check for stake revocation script.  Only stake-revocation-tagged
	// pay-to-pubkey-hash and pay-to-script-hash are allowed.
	if hash := extractStakePubKeyHash(pkScript, txscript.OP_SSRTX); hash != nil {
		addr, err := dcrutil.NewAddressPubKeyHash(hash, chainParams,
			dcrec.STEcdsaSecp256k1)
		if err != nil {
			return txscript.NonStandardTy, nil, 0, err
		}
		return txscript.StakeRevocationTy, []dcrutil.Address{addr}, 1, nil
	}
	if hash := extractStakeScriptHash(pkScript, txscript.OP_SSRTX); hash != nil {
		addr, err := dcrutil.NewAddressScriptHashFromHash(hash, chainParams)
		if err != nil {
			return txscript.NonStandardTy, nil, 0, err
		}
		return txscript.StakeRevocationTy, []dcrutil.Address{addr}, 1, nil
	}

	// Check for stake change script.  Only stake-change-tagged
	// pay-to-pubkey-hash and pay-to-script-hash are allowed.
	if hash := extractStakePubKeyHash(pkScript, txscript.OP_SSTXCHANGE); hash != nil {
		addr, err := dcrutil.NewAddressPubKeyHash(hash, chainParams,
			dcrec.STEcdsaSecp256k1)
		if err != nil {
			return txscript.NonStandardTy, nil, 0, err
		}
		return txscript.StakeSubChangeTy, []dcrutil.Address{addr}, 1, nil
	}
	if hash := extractStakeScriptHash(pkScript, txscript.OP_SSTXCHANGE); hash != nil {
		addr, err := dcrutil.NewAddressScriptHashFromHash(hash, chainParams)
		if err != nil {
			return txscript.NonStandardTy, nil, 0, err
		}
		return txscript.StakeSubChangeTy, []dcrutil.Address{addr}, 1, nil
	}

	// Check for null data script.
	if IsNullDataScript(version, pkScript) {
		// Null data transactions have no addresses or required signatures.
		return txscript.NullDataTy, nil, 0, nil
	}

	// Don't attempt to extract addresses or required signatures for nonstandard
	// transactions.
	return txscript.NonStandardTy, nil, 0, nil
}
