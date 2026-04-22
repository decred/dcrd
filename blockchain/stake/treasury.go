// Copyright (c) 2020-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"fmt"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/schnorr"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/wire"
)

const (
	// TSpendScriptLen is the exact length of a TSpend script.
	// <OP_DATA_64> <signature> <OP_DATA_33> <public key> <OP_TSPEND>
	// 1 + 64 + 1 + 33 + 1 = 100
	TSpendScriptLen = 100
)

// -----------------------------------------------------------------------------
// This file contains functions that verify that treasury transactions strictly
// adhere to the specified format.
//
// == User sends to treasury ==
// TxIn:    Normal TxIn signature scripts
// TxOut[0] OP_TADD
// TxOut[1] optional OP_SSTXCHANGE
//
// == Treasurybase add ==
// TxIn[0]: Treasurybase
// TxOut[0] OP_TADD
// TxOut[1] OP_RETURN OP_DATA_12 [4]{LE encoded height} [8]{random}
//
// == Spend from treasury ==
// TxIn[0]     <signature> <pi pubkey> OP_TSPEND
// TxOut[0]    OP_RETURN OP_DATA_32 [8]{LE encoded input value} [24]{random}
// TxOut[1..N] OP_TGEN <paytopubkeyhash || paytoscripthash>
// -----------------------------------------------------------------------------

// CheckTAdd verifies that the provided transaction satisfies the structural
// requirements to be a valid treasury add transaction.  A treasury adds
// transaction is one that sends existing funds to the decentralized treasury.
//
// A valid treasury add must have:
//   - The transaction version set to [wire.TxVersionTreasury]
//   - One or more normal inputs referencing the coins to spend
//   - An output with a treasury add script (OP_TADD)
//   - An optional second output that must be a stake change script
//     (OP_SSTXCHANGE) when present
//   - All script versions set to 0
func CheckTAdd(tx *wire.MsgTx) error {
	// The transaction version must be the required treasury version.
	if tx.Version != wire.TxVersionTreasury {
		str := fmt.Sprintf("treasury add transaction version is %d instead of %d",
			tx.Version, wire.TxVersionTreasury)
		return stakeRuleError(ErrTAddInvalidTxVersion, str)
	}

	// A treasury add must have at least one input and one or two outputs.
	if len(tx.TxIn) < 1 {
		const str = "treasury add transaction does not have any inputs"
		return stakeRuleError(ErrTAddInvalidCount, str)
	}
	if len(tx.TxOut) != 1 && len(tx.TxOut) != 2 {
		str := fmt.Sprintf("treasury add transaction has %d outputs instead "+
			"of 1 or 2", len(tx.TxOut))
		return stakeRuleError(ErrTAddInvalidCount, str)
	}

	// All output scripts must be version 0 and non-empty.
	const consensusScriptVer = 0
	for txOutIdx := range tx.TxOut {
		txOut := tx.TxOut[txOutIdx]
		if txOut.Version != consensusScriptVer {
			str := fmt.Sprintf("treasury add transaction output %d script "+
				"version is %d instead of %d", txOutIdx, txOut.Version,
				consensusScriptVer)
			return stakeRuleError(ErrTAddInvalidVersion, str)
		}
		if len(txOut.PkScript) == 0 {
			str := fmt.Sprintf("treasury add transaction output %d script is "+
				"empty", txOutIdx)
			return stakeRuleError(ErrTAddInvalidScriptLength, str)
		}
	}

	// The first output must be a script that only consists of OP_TADD.
	firstTxOut := tx.TxOut[0]
	if len(firstTxOut.PkScript) != 1 {
		str := fmt.Sprintf("treasury add transaction output 0 script length "+
			"is %d bytes instead of 1 byte", len(firstTxOut.PkScript))
		return stakeRuleError(ErrTAddInvalidLength, str)
	}
	if firstTxOut.PkScript[0] != txscript.OP_TADD {
		str := fmt.Sprintf("treasury add transaction output 0 script is 0x%x "+
			"instead of OP_TADD (0x%x)", firstTxOut.PkScript[0],
			txscript.OP_TADD)
		return stakeRuleError(ErrTAddInvalidOpcode, str)
	}

	// The second output must be a valid stake change output when present.
	if len(tx.TxOut) == 2 {
		changeTxOut := tx.TxOut[1]
		if !IsStakeChangeScript(changeTxOut.Version, changeTxOut.PkScript) {
			const str = "treasury add transaction output 1 is not a " +
				"stake change script"
			return stakeRuleError(ErrTAddInvalidChange, str)
		}
	}

	return nil
}

// IsTAdd returns whether or not the provided transaction satisfies the
// structural requirements to be a valid treasury add transaction.
//
// See the [CheckTAdd] documentation for more details.
func IsTAdd(tx *wire.MsgTx) bool {
	return CheckTAdd(tx) == nil
}

// CheckTSpend verifies that the provided transaction satisfies the structural
// requirements to be a valid treasury spend transaction.  It returns the
// signature and public key encoded in the first input when the error is nil.
//
// This function DOES NOT check the signature or if the public key is a well
// known PI key.  It also DOES NOT check the input value encoded in the data
// push of the first output matches the input value.
//
// A valid treasury spend must have:
//   - The transaction version set to [wire.TxVersionTreasury]
//   - A single input with a treasury spend script (<sig> <pi pubkey> OP_TSPEND)
//   - The first output with a 32 byte nulldata script
//     (<8-byte LE encoded input value + 24-byte random>)
//   - One or more remaining outputs that must be treasury gen scripts (OP_TGEN
//     followed by pay-to-pubkey-hash or pay-to-script-hash)
//   - All script versions set to 0
func CheckTSpend(tx *wire.MsgTx) ([]byte, []byte, error) {
	// The transaction version must be the required treasury version.
	if tx.Version != wire.TxVersionTreasury {
		str := fmt.Sprintf("treasury spend transaction version is %d instead "+
			"of %d", tx.Version, wire.TxVersionTreasury)
		return nil, nil, stakeRuleError(ErrTSpendInvalidTxVersion, str)
	}

	// A treasury spend must have exactly one input and at least two outputs.
	if len(tx.TxIn) != 1 {
		str := fmt.Sprintf("treasury spend transaction has %d inputs instead "+
			"of 1", len(tx.TxIn))
		return nil, nil, stakeRuleError(ErrTSpendInvalidLength, str)
	}
	if len(tx.TxOut) < 2 {
		str := fmt.Sprintf("treasury spend transaction does not have enough "+
			"outputs (min: %d, have: %d)", 2, len(tx.TxOut))
		return nil, nil, stakeRuleError(ErrTSpendInvalidLength, str)
	}

	// All output scripts must be version 0 and non-empty.
	const consensusScriptVer = 0
	for txOutIdx, txOut := range tx.TxOut {
		if txOut.Version != consensusScriptVer {
			str := fmt.Sprintf("treasury spend transaction output %d script "+
				"version is %d instead of %d", txOutIdx, txOut.Version,
				consensusScriptVer)
			return nil, nil, stakeRuleError(ErrTSpendInvalidVersion, str)
		}
		if len(txOut.PkScript) == 0 {
			str := fmt.Sprintf("treasury spend transaction output %d script "+
				"is empty", txOutIdx)
			return nil, nil, stakeRuleError(ErrTSpendInvalidScriptLength, str)
		}
	}

	// The single input must have the exact treasury spend script format:
	//
	// DATA_64 <64-byte schnorr signature> DATA_33 <33-byte pubkey> OP_TSPEND
	txIn := tx.TxIn[0].SignatureScript
	if len(txIn) != TSpendScriptLen || txIn[0] != txscript.OP_DATA_64 ||
		txIn[65] != txscript.OP_DATA_33 || txIn[99] != txscript.OP_TSPEND {

		const str = "treasury spend transaction input 0 script is malformed"
		return nil, nil, stakeRuleError(ErrTSpendInvalidScript, str)
	}
	signature := txIn[1 : 1+schnorr.SignatureSize]
	pubKey := txIn[66 : 66+secp256k1.PubKeyBytesLenCompressed]

	// The public key must adhere to the strict compressed public key encoding.
	if !txscript.IsStrictCompressedPubKeyEncoding(pubKey) {
		str := fmt.Sprintf("treasury spend transaction input 0 public key %x "+
			"does not use strict compressed encoding", pubKey)
		return nil, nil, stakeRuleError(ErrTSpendInvalidPubkey, str)
	}

	// The first output must be an OP_RETURN followed by a 32 byte data push.
	firstTxOut := tx.TxOut[0]
	if !txscript.IsStrictNullData(firstTxOut.Version, firstTxOut.PkScript, 32) {
		const str = "treasury spend transaction output 0 script is not an " +
			"OP_RETURN followed by a 32 byte data push"
		return nil, nil, stakeRuleError(ErrTSpendInvalidTransaction, str)
	}

	// All outputs after the first one must have OP_TGEN tagged p2pkh or p2sh
	// scripts.
	for txOutIdx, txOut := range tx.TxOut[1:] {
		script := txOut.PkScript
		if script[0] != txscript.OP_TGEN {
			str := fmt.Sprintf("treasury spend transaction output %d script "+
				"is not tagged with OP_TGEN", txOutIdx+1)
			return nil, nil, stakeRuleError(ErrTSpendInvalidTGen, str)
		}
		if !isPubKeyHashScript(script[1:]) && !isScriptHashScript(script[1:]) {
			str := fmt.Sprintf("treasury spend transaction output %d script "+
				"is not pay-to-script-hash or pay-to-pubkey-hash", txOutIdx+1)
			return nil, nil, stakeRuleError(ErrTSpendInvalidSpendScript, str)
		}
	}

	return signature, pubKey, nil
}

// IsTSpend returns whether or not the provided transaction satisfies the
// structural requirements to be a valid treasury spend transaction.
//
// See the [CheckTSpend] documentation for more details.
func IsTSpend(tx *wire.MsgTx) bool {
	_, _, err := CheckTSpend(tx)
	return err == nil
}

// CheckTreasuryBase verifies that the provided transaction satisfies the
// structural requirements to be a valid treasurybase transaction.
//
// A valid treasurybase must have:
//   - The transaction version set to [wire.TxVersionTreasury]
//   - A single treasurybase input (no signature script, null prevout)
//   - An output with a treasury add script (OP_TADD)
//   - An output with a 12 byte nulldata script
//     (<4-byte LE encoded height + 8-byte random>)
//   - All script versions set to 0
func CheckTreasuryBase(tx *wire.MsgTx) error {
	// The transaction version must be the required treasury version.
	if tx.Version != wire.TxVersionTreasury {
		str := fmt.Sprintf("treasurybase transaction version is %d instead of %d",
			tx.Version, wire.TxVersionTreasury)
		return stakeRuleError(ErrTreasuryBaseInvalidTxVersion, str)
	}

	// A treasurybase must have exactly one input and two outputs.
	if len(tx.TxIn) != 1 {
		str := fmt.Sprintf("treasurybase transaction has %d inputs instead of 1",
			len(tx.TxIn))
		return stakeRuleError(ErrTreasuryBaseInvalidCount, str)
	}
	if len(tx.TxOut) != 2 {
		str := fmt.Sprintf("treasurybase transaction has %d output(s) instead "+
			"of 2", len(tx.TxOut))
		return stakeRuleError(ErrTreasuryBaseInvalidCount, str)
	}

	// The first input signature script must be empty and its previous output
	// must be a null outpoint (max value index, a zero hash, regular tx tree).
	if len(tx.TxIn[0].SignatureScript) != 0 {
		str := fmt.Sprintf("treasurybase input 0 signature script is %d "+
			"byte(s) instead of 0", len(tx.TxIn[0].SignatureScript))
		return stakeRuleError(ErrTreasuryBaseInvalidLength, str)
	}
	if !isNullOutpoint(tx) {
		prevOut := &tx.TxIn[0].PreviousOutPoint
		str := fmt.Sprintf("treasurybase input 0 previous output %s:%d:%d is "+
			"not a null outpoint", prevOut.Hash, prevOut.Index, prevOut.Tree)
		return stakeRuleError(ErrTreasuryBaseInvalid, str)
	}

	// All output scripts must be version 0.
	const consensusScriptVer = 0
	for txOutIdx, txOut := range tx.TxOut {
		if txOut.Version != consensusScriptVer {
			str := fmt.Sprintf("treasurybase transaction output %d script "+
				"version is %d instead of %d", txOutIdx, txOut.Version,
				consensusScriptVer)
			return stakeRuleError(ErrTreasuryBaseInvalidVersion, str)
		}
	}

	// The first output must be a script that only consists of OP_TADD.
	firstTxOut := tx.TxOut[0]
	if len(firstTxOut.PkScript) != 1 {
		str := fmt.Sprintf("treasurybase transaction output 0 script length "+
			"is %d bytes instead of 1 byte", len(firstTxOut.PkScript))
		return stakeRuleError(ErrTreasuryBaseInvalidOpcode0, str)
	}
	if firstTxOut.PkScript[0] != txscript.OP_TADD {
		str := fmt.Sprintf("treasurybase transaction output 0 script is 0x%x "+
			"instead of OP_TADD (0x%x)", firstTxOut.PkScript[0],
			txscript.OP_TADD)
		return stakeRuleError(ErrTreasuryBaseInvalidOpcode0, str)
	}

	// The second output must be an OP_RETURN followed by a 12 byte data push.
	opRetTxOut := tx.TxOut[1]
	if !txscript.IsStrictNullData(opRetTxOut.Version, opRetTxOut.PkScript, 12) {
		const str = "treasurybase transaction output 1 is not an OP_RETURN " +
			"followed by a 12 byte data push"
		return stakeRuleError(ErrTreasuryBaseInvalidOpcode1, str)
	}

	return nil
}

// IsTreasuryBase returns whether or not the provided transaction satisfies the
// structural requirements to be a valid treasurybase transaction.
//
// See the [CheckTreasuryBase] documentation for more details.
func IsTreasuryBase(tx *wire.MsgTx) bool {
	return CheckTreasuryBase(tx) == nil
}
