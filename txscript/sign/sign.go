// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package sign

import (
	"errors"
	"fmt"

	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/edwards/v2"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/schnorr"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/txscript/v4/stdscript"
	"github.com/decred/dcrd/wire"
)

// RawTxInSignature returns the serialized ECDSA signature for the input idx of
// the given transaction, with hashType appended to it.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func RawTxInSignature(tx *wire.MsgTx, idx int, subScript []byte,
	hashType txscript.SigHashType, key []byte,
	sigType dcrec.SignatureType) ([]byte, error) {

	hash, err := txscript.CalcSignatureHash(subScript, hashType, tx, idx, nil)
	if err != nil {
		return nil, err
	}

	var sigBytes []byte
	switch sigType {
	case dcrec.STEcdsaSecp256k1:
		priv := secp256k1.PrivKeyFromBytes(key)
		sig := ecdsa.Sign(priv, hash)
		sigBytes = sig.Serialize()
	case dcrec.STEd25519:
		priv, _ := edwards.PrivKeyFromBytes(key)
		if priv == nil {
			return nil, fmt.Errorf("invalid privkey")
		}
		sig, err := priv.Sign(hash)
		if err != nil {
			return nil, fmt.Errorf("cannot sign tx input: %w", err)
		}
		sigBytes = sig.Serialize()
	case dcrec.STSchnorrSecp256k1:
		priv := secp256k1.PrivKeyFromBytes(key)
		sig, err := schnorr.Sign(priv, hash)
		if err != nil {
			return nil, fmt.Errorf("cannot sign tx input: %w", err)
		}
		sigBytes = sig.Serialize()
	default:
		return nil, fmt.Errorf("unknown signature type '%v'", sigType)
	}

	return append(sigBytes, byte(hashType)), nil
}

// SignatureScript creates an input signature script for tx to spend coins sent
// from a previous output to the owner of privKey. tx must include all
// transaction inputs and outputs, however txin scripts are allowed to be filled
// or empty. The returned script is calculated to be used as the idx'th txin
// sigscript for tx. subscript is the PkScript of the previous output being used
// as the idx'th input. privKey is serialized in the respective format for the
// ECDSA type. This format must match the same format used to generate the payment
// address, or the script validation will fail.
func SignatureScript(tx *wire.MsgTx, idx int, subscript []byte,
	hashType txscript.SigHashType, privKey []byte,
	sigType dcrec.SignatureType, compress bool) ([]byte, error) {

	sig, err := RawTxInSignature(tx, idx, subscript, hashType, privKey, sigType)
	if err != nil {
		return nil, err
	}

	var pkData []byte
	switch sigType {
	case dcrec.STEcdsaSecp256k1:
		priv := secp256k1.PrivKeyFromBytes(privKey)
		if compress {
			pkData = priv.PubKey().SerializeCompressed()
		} else {
			pkData = priv.PubKey().SerializeUncompressed()
		}
	case dcrec.STEd25519:
		_, pub := edwards.PrivKeyFromBytes(privKey)
		pkData = pub.Serialize()
	case dcrec.STSchnorrSecp256k1:
		priv := secp256k1.PrivKeyFromBytes(privKey)
		pkData = priv.PubKey().SerializeCompressed()
	default:
		return nil, fmt.Errorf("unsupported signature type '%v'", sigType)
	}

	return txscript.NewScriptBuilder().AddData(sig).AddData(pkData).Script()
}

// p2pkSignatureScript constructs a pay-to-pubkey signature script.
func p2pkSignatureScript(tx *wire.MsgTx, idx int, subScript []byte,
	hashType txscript.SigHashType, privKey []byte,
	sigType dcrec.SignatureType) ([]byte, error) {

	sig, err := RawTxInSignature(tx, idx, subScript, hashType, privKey, sigType)
	if err != nil {
		return nil, err
	}

	return txscript.NewScriptBuilder().AddData(sig).Script()
}

// signMultiSig signs as many of the outputs in the provided multisig script as
// possible. It returns the generated script and a boolean if the script
// fulfills the contract (i.e. nrequired signatures are provided).  Since it is
// arguably legal to not be able to sign any of the outputs, no error is
// returned.
func signMultiSig(tx *wire.MsgTx, idx int, subScript []byte,
	hashType txscript.SigHashType, addresses []stdaddr.Address,
	nRequired uint16, kdb KeyDB) ([]byte, bool) {

	// No need to add dummy in Decred.
	builder := txscript.NewScriptBuilder()
	var signed uint16
	for _, addr := range addresses {
		key, sigType, _, err := kdb.GetKey(addr)
		if err != nil {
			continue
		}

		sig, err := RawTxInSignature(tx, idx, subScript, hashType, key, sigType)
		if err != nil {
			continue
		}

		builder.AddData(sig)
		signed++
		if signed == nRequired {
			break
		}
	}

	script, _ := builder.Script()
	return script, signed == nRequired
}

// stakeSubScriptType potentially transforms the provided script type by
// converting the various stake-specific script types to their associated sub
// type.  It will be returned unmodified otherwise.
func stakeSubScriptType(scriptType stdscript.ScriptType, isTreasuryEnabled bool) stdscript.ScriptType {
	if scriptType == stdscript.STStakeSubmissionPubKeyHash ||
		scriptType == stdscript.STStakeChangePubKeyHash ||
		scriptType == stdscript.STStakeGenPubKeyHash ||
		scriptType == stdscript.STStakeRevocationPubKeyHash ||
		(isTreasuryEnabled && scriptType == stdscript.STTreasuryGenPubKeyHash) {

		return stdscript.STPubKeyHashEcdsaSecp256k1

	} else if scriptType == stdscript.STStakeSubmissionScriptHash ||
		scriptType == stdscript.STStakeChangeScriptHash ||
		scriptType == stdscript.STStakeGenScriptHash ||
		scriptType == stdscript.STStakeRevocationScriptHash ||
		(isTreasuryEnabled && scriptType == stdscript.STTreasuryGenScriptHash) {

		return stdscript.STScriptHash
	}

	return scriptType
}

// handleStakeOutSign is a convenience function for reducing code clutter in
// sign. It handles the signing of stake outputs.
func handleStakeOutSign(tx *wire.MsgTx, idx int, subScript []byte,
	hashType txscript.SigHashType, kdb KeyDB, sdb ScriptDB,
	addresses []stdaddr.Address, scriptType stdscript.ScriptType,
	isTreasuryEnabled bool) ([]byte, stdscript.ScriptType, []stdaddr.Address, error) {

	// look up key for address
	subType := stakeSubScriptType(scriptType, isTreasuryEnabled)
	switch subType {
	case stdscript.STPubKeyHashEcdsaSecp256k1:
		key, sigType, compressed, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, scriptType, nil, err
		}
		txscript, err := SignatureScript(tx, idx, subScript, hashType,
			key, sigType, compressed)
		if err != nil {
			return nil, scriptType, nil, err
		}
		return txscript, scriptType, addresses, nil

	case stdscript.STScriptHash:
		script, err := sdb.GetScript(addresses[0])
		if err != nil {
			return nil, scriptType, nil, err
		}

		return script, scriptType, addresses, nil
	}

	return nil, scriptType, nil, fmt.Errorf("unknown sub script type for " +
		"stake output to sign")
}

// sign is the main signing workhorse. It takes a script, its input transaction,
// its input index, a database of keys, a database of scripts, and information
// about the type of signature and returns a signature, script type, the
// addresses involved, and the number of signatures required.
func sign(chainParams stdaddr.AddressParams, tx *wire.MsgTx, idx int,
	subScript []byte, hashType txscript.SigHashType, kdb KeyDB, sdb ScriptDB,
	isTreasuryEnabled bool) ([]byte, stdscript.ScriptType, []stdaddr.Address, error) {

	scriptType, addresses := stdscript.ExtractAddrsV0(subScript, chainParams)
	switch scriptType {
	case stdscript.STPubKeyEcdsaSecp256k1:
		// look up key for address
		key, sigType, _, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, scriptType, nil, err
		}

		script, err := p2pkSignatureScript(tx, idx, subScript, hashType,
			key, sigType)
		if err != nil {
			return nil, scriptType, nil, err
		}

		return script, scriptType, addresses, nil

	case stdscript.STPubKeyEd25519, stdscript.STPubKeySchnorrSecp256k1:
		// look up key for address
		key, sigType, _, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, scriptType, nil, err
		}

		script, err := p2pkSignatureScript(tx, idx, subScript, hashType,
			key, sigType)
		if err != nil {
			return nil, scriptType, nil, err
		}

		return script, scriptType, addresses, nil

	case stdscript.STPubKeyHashEcdsaSecp256k1:
		// look up key for address
		key, sigType, compressed, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, scriptType, nil, err
		}

		script, err := SignatureScript(tx, idx, subScript, hashType,
			key, sigType, compressed)
		if err != nil {
			return nil, scriptType, nil, err
		}

		return script, scriptType, addresses, nil

	case stdscript.STPubKeyHashEd25519, stdscript.STPubKeyHashSchnorrSecp256k1:
		// look up key for address
		key, sigType, compressed, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, scriptType, nil, err
		}

		script, err := SignatureScript(tx, idx, subScript, hashType,
			key, sigType, compressed)
		if err != nil {
			return nil, scriptType, nil, err
		}

		return script, scriptType, addresses, nil

	case stdscript.STScriptHash:
		script, err := sdb.GetScript(addresses[0])
		if err != nil {
			return nil, scriptType, nil, err
		}

		return script, scriptType, addresses, nil

	case stdscript.STMultiSig:
		details := stdscript.ExtractMultiSigScriptDetailsV0(subScript, false)
		threshold := details.RequiredSigs
		script, _ := signMultiSig(tx, idx, subScript, hashType, addresses,
			threshold, kdb)
		return script, scriptType, addresses, nil

	case stdscript.STStakeSubmissionPubKeyHash,
		stdscript.STStakeSubmissionScriptHash:

		return handleStakeOutSign(tx, idx, subScript, hashType, kdb, sdb,
			addresses, scriptType, isTreasuryEnabled)

	case stdscript.STStakeGenPubKeyHash, stdscript.STStakeGenScriptHash:
		return handleStakeOutSign(tx, idx, subScript, hashType, kdb, sdb,
			addresses, scriptType, isTreasuryEnabled)

	case stdscript.STStakeRevocationPubKeyHash,
		stdscript.STStakeRevocationScriptHash:

		return handleStakeOutSign(tx, idx, subScript, hashType, kdb, sdb,
			addresses, scriptType, isTreasuryEnabled)

	case stdscript.STStakeChangePubKeyHash, stdscript.STStakeChangeScriptHash:
		return handleStakeOutSign(tx, idx, subScript, hashType, kdb, sdb,
			addresses, scriptType, isTreasuryEnabled)

	case stdscript.STTreasuryGenPubKeyHash, stdscript.STTreasuryGenScriptHash:
		return handleStakeOutSign(tx, idx, subScript, hashType, kdb, sdb,
			addresses, scriptType, isTreasuryEnabled)

	case stdscript.STNullData:
		return nil, scriptType, nil,
			errors.New("can't sign NULLDATA transactions")

	default:
		return nil, scriptType, nil,
			errors.New("can't sign unknown transactions")
	}
}

// mergeMultiSig combines the two signature scripts sigScript and prevScript
// that both provide signatures for pkScript in output idx of tx. addresses
// and nRequired should be the results from extracting the addresses from
// pkScript. Since this function is internal only we assume that the arguments
// have come from other functions internally and thus are all consistent with
// each other, behaviour is undefined if this contract is broken.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func mergeMultiSig(tx *wire.MsgTx, idx int, addresses []stdaddr.Address,
	nRequired uint16, pkScript, sigScript, prevScript []byte) []byte {

	// Nothing to merge if either the new or previous signature scripts are
	// empty.
	if len(sigScript) == 0 {
		return prevScript
	}
	if len(prevScript) == 0 {
		return sigScript
	}

	// Convenience function to avoid duplication.
	var possibleSigs [][]byte
	extractSigs := func(script []byte) error {
		const scriptVersion = 0
		tokenizer := txscript.MakeScriptTokenizer(scriptVersion, script)
		for tokenizer.Next() {
			if data := tokenizer.Data(); len(data) != 0 {
				possibleSigs = append(possibleSigs, data)
			}
		}
		return tokenizer.Err()
	}

	// Attempt to extract signatures from the two scripts.  Return the other
	// script that is intended to be merged in the case signature extraction
	// fails for some reason.
	if err := extractSigs(sigScript); err != nil {
		return prevScript
	}
	if err := extractSigs(prevScript); err != nil {
		return sigScript
	}

	// Now we need to match the signatures to pubkeys, the only real way to
	// do that is to try to verify them all and match it to the pubkey
	// that verifies it. we then can go through the addresses in order
	// to build our script. Anything that doesn't parse or doesn't verify we
	// throw away.
	addrToSig := make(map[string][]byte)
sigLoop:
	for _, sig := range possibleSigs {

		// can't have a valid signature that doesn't at least have a
		// hashtype, in practise it is even longer than this. but
		// that'll be checked next.
		if len(sig) < 1 {
			continue
		}
		tSig := sig[:len(sig)-1]
		hashType := txscript.SigHashType(sig[len(sig)-1])

		pSig, err := ecdsa.ParseDERSignature(tSig)
		if err != nil {
			continue
		}

		// We have to do this each round since hash types may vary
		// between signatures and so the hash will vary. We can,
		// however, assume no sigs etc are in the script since that
		// would make the transaction nonstandard and thus not
		// MultiSigTy, so we just need to hash the full thing.
		hash, err := txscript.CalcSignatureHash(pkScript, hashType, tx, idx, nil)
		if err != nil {
			// Decred -- is this the right handling for SIGHASH_SINGLE error ?
			// TODO make sure this doesn't break anything.
			continue
		}

		for _, addr := range addresses {
			// All multisig addresses should be pubkey addresses
			// it is an error to call this internal function with
			// bad input.
			pkAddr := addr.(stdaddr.SerializedPubKeyer)
			pubKey, err := secp256k1.ParsePubKey(pkAddr.SerializedPubKey())
			if err != nil {
				continue
			}

			// If it matches we put it in the map. We only
			// can take one signature per public key so if we
			// already have one, we can throw this away.
			if pSig.Verify(hash, pubKey) {
				aStr := addr.String()
				if _, ok := addrToSig[aStr]; !ok {
					addrToSig[aStr] = sig
				}
				continue sigLoop
			}
		}
	}

	builder := txscript.NewScriptBuilder()
	var doneSigs uint16
	// This assumes that addresses are in the same order as in the script.
	for _, addr := range addresses {
		sig, ok := addrToSig[addr.String()]
		if !ok {
			continue
		}
		builder.AddData(sig)
		doneSigs++
		if doneSigs == nRequired {
			break
		}
	}

	// padding for missing ones.
	for i := doneSigs; i < nRequired; i++ {
		builder.AddOp(txscript.OP_0)
	}

	script, _ := builder.Script()
	return script
}

// checkScriptParses returns an error if the provided script fails to parse.
func checkScriptParses(scriptVersion uint16, script []byte) error {
	tokenizer := txscript.MakeScriptTokenizer(scriptVersion, script)
	for tokenizer.Next() {
		// Nothing to do.
	}
	return tokenizer.Err()
}

// finalOpcodeData returns the data associated with the final opcode in the
// script.  It will return nil if the script fails to parse.
func finalOpcodeData(scriptVersion uint16, script []byte) []byte {
	// Avoid unnecessary work.
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

// mergeScripts merges sigScript and prevScript assuming they are both
// partial solutions for pkScript spending output idx of tx. scriptType,
// addresses and nrequired are the result of extracting the addresses from
// pkscript.  The return value is the best effort merging of the two scripts.
// Calling this function with addresses, scriptType and nRequired that do not
// match pkScript is an error and results in undefined behaviour.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func mergeScripts(chainParams stdaddr.AddressParams, tx *wire.MsgTx, idx int,
	pkScript []byte, scriptType stdscript.ScriptType, addresses []stdaddr.Address,
	sigScript, prevScript []byte) []byte {

	// TODO(oga) the scripthash and multisig paths here are overly
	// inefficient in that they will recompute already known data.
	// some internal refactoring could probably make this avoid needless
	// extra calculations.
	const scriptVersion = 0
	switch scriptType {
	case stdscript.STScriptHash:
		// Nothing to merge if either the new or previous signature
		// scripts are empty or fail to parse.
		if len(sigScript) == 0 ||
			checkScriptParses(scriptVersion, sigScript) != nil {

			return prevScript
		}
		if len(prevScript) == 0 ||
			checkScriptParses(scriptVersion, prevScript) != nil {

			return sigScript
		}

		// Remove the last push in the script and then recurse.
		// this could be a lot less inefficient.
		//
		// Assume that final script is the correct one since it was just
		// made and it is a pay-to-script-hash.
		script := finalOpcodeData(scriptVersion, sigScript)

		// Determine the type of the redeem script, extract the standard
		// addresses from it, and merge.
		scriptType, addresses := stdscript.ExtractAddrs(scriptVersion, script,
			chainParams)
		mergedScript := mergeScripts(chainParams, tx, idx, script, scriptType,
			addresses, sigScript, prevScript)

		// Reappend the script and return the result.
		builder := txscript.NewScriptBuilder()
		builder.AddOps(mergedScript)
		builder.AddData(script)
		finalScript, _ := builder.Script()
		return finalScript

	case stdscript.STMultiSig:
		details := stdscript.ExtractMultiSigScriptDetailsV0(pkScript, false)
		return mergeMultiSig(tx, idx, addresses, details.RequiredSigs, pkScript,
			sigScript, prevScript)

	// It doesn't actually make sense to merge anything other than multisig
	// and scripthash (because it could contain multisig). Everything else
	// has either zero signature, can't be spent, or has a single signature
	// which is either present or not. The other two cases are handled
	// above. In the conflict case here we just assume the longest is
	// correct (this matches behaviour of the reference implementation).
	default:
		if len(sigScript) > len(prevScript) {
			return sigScript
		}
		return prevScript
	}
}

// KeyDB is an interface type provided to SignTxOutput, it encapsulates
// any user state required to get the private keys for an address.
type KeyDB interface {
	GetKey(stdaddr.Address) ([]byte, dcrec.SignatureType, bool, error)
}

// KeyClosure implements KeyDB with a closure.
type KeyClosure func(stdaddr.Address) ([]byte, dcrec.SignatureType, bool, error)

// GetKey implements KeyDB by returning the result of calling the closure.
func (kc KeyClosure) GetKey(address stdaddr.Address) ([]byte, dcrec.SignatureType, bool, error) {
	return kc(address)
}

// ScriptDB is an interface type provided to SignTxOutput, it encapsulates any
// user state required to get the scripts for a pay-to-script-hash address.
type ScriptDB interface {
	GetScript(stdaddr.Address) ([]byte, error)
}

// ScriptClosure implements ScriptDB with a closure.
type ScriptClosure func(stdaddr.Address) ([]byte, error)

// GetScript implements ScriptDB by returning the result of calling the closure.
func (sc ScriptClosure) GetScript(address stdaddr.Address) ([]byte, error) {
	return sc(address)
}

// SignTxOutput signs output idx of the given tx to resolve the script given in
// pkScript with a signature type of hashType. Any keys required will be
// looked up by calling getKey() with the string of the given address.
// Any pay-to-script-hash signatures will be similarly looked up by calling
// getScript. If previousScript is provided then the results in previousScript
// will be merged in a type-dependent manner with the newly generated.
// signature script.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func SignTxOutput(chainParams stdaddr.AddressParams, tx *wire.MsgTx, idx int,
	pkScript []byte, hashType txscript.SigHashType, kdb KeyDB, sdb ScriptDB,
	previousScript []byte, isTreasuryEnabled bool) ([]byte, error) {

	sigScript, scriptType, addresses, err := sign(chainParams, tx, idx,
		pkScript, hashType, kdb, sdb, isTreasuryEnabled)
	if err != nil {
		return nil, err
	}

	scriptType = stakeSubScriptType(scriptType, isTreasuryEnabled)
	if scriptType == stdscript.STScriptHash {
		// TODO keep the sub addressed and pass down to merge.
		realSigScript, _, _, err := sign(chainParams, tx, idx, sigScript,
			hashType, kdb, sdb, isTreasuryEnabled)
		if err != nil {
			return nil, err
		}

		// Append the p2sh script as the last push in the script.
		builder := txscript.NewScriptBuilder()
		builder.AddOps(realSigScript)
		builder.AddData(sigScript)

		sigScript, _ = builder.Script()
		// TODO keep a copy of the script for merging.
	}

	// Merge scripts. with any previous data, if any.
	mergedScript := mergeScripts(chainParams, tx, idx, pkScript, scriptType,
		addresses, sigScript, previousScript)
	return mergedScript, nil
}

// TSpendSignatureScript creates an input signature for the provided tx, which
// is expected to be a treasury spend transaction, to authorize coins to be
// spent from the treasury.  The private key must correspond to one of the
// valid public keys for a Pi instance recognized by consensus.
func TSpendSignatureScript(msgTx *wire.MsgTx, privKey []byte) ([]byte, error) {
	hash, err := txscript.CalcSignatureHash(nil, txscript.SigHashAll, msgTx, 0,
		nil)
	if err != nil {
		return nil, err
	}

	priv := secp256k1.PrivKeyFromBytes(privKey)
	sig, err := schnorr.Sign(priv, hash)
	if err != nil {
		return nil, fmt.Errorf("cannot sign tx input: %w", err)
	}
	sigBytes := sig.Serialize()
	pkBytes := priv.PubKey().SerializeCompressed()

	return txscript.NewScriptBuilder().AddData(sigBytes).AddData(pkBytes).
		AddOp(txscript.OP_TSPEND).Script()
}
