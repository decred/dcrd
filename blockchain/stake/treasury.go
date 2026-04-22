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
// TxOut[0]    OP_RETURN <random>
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

// CheckTSpend verifies if a MsgTx is a valid TSPEND.
// This function DOES NOT check the signature or if the public key is a well
// known PI key. This is a convenience function to obtain the signature and
// public key without iterating over the same MsgTx over and over again. The
// return values are signature, public key and an error.
func CheckTSpend(mtx *wire.MsgTx) ([]byte, []byte, error) {
	// Require version TxVersionTreasury.
	if mtx.Version != wire.TxVersionTreasury {
		return nil, nil, stakeRuleError(ErrTSpendInvalidTxVersion,
			fmt.Sprintf("invalid TSpend script version: %v",
				mtx.Version))
	}

	// A valid TSPEND consists of a single TxIn that contains a signature,
	// a public key and an OP_TSPEND opcode.
	//
	// There must be at least two outputs. The first must contain an
	// OP_RETURN followed by a 32 byte data push of a random number. This
	// is used to randomize the transaction hash.
	// The second output must be a TGEN tagged P2SH or P2PKH script.
	if len(mtx.TxIn) != 1 || len(mtx.TxOut) < 2 {
		return nil, nil, stakeRuleError(ErrTSpendInvalidLength,
			fmt.Sprintf("invalid TSPEND script lengths in: %v "+
				"out: %v", len(mtx.TxIn), len(mtx.TxOut)))
	}

	// All output scripts must be version 0 and non-empty.
	const consensusScriptVer = 0
	for k, txOut := range mtx.TxOut {
		if txOut.Version != consensusScriptVer {
			return nil, nil, stakeRuleError(ErrTSpendInvalidVersion,
				fmt.Sprintf("invalid script version found in "+
					"TxOut: %v", k))
		}
		if len(txOut.PkScript) == 0 {
			return nil, nil, stakeRuleError(ErrTSpendInvalidScriptLength,
				fmt.Sprintf("invalid TxOut script length %v: "+
					"%v", k, len(txOut.PkScript)))
		}
	}

	txIn := mtx.TxIn[0].SignatureScript
	if !(len(txIn) == TSpendScriptLen &&
		txIn[0] == txscript.OP_DATA_64 &&
		txIn[65] == txscript.OP_DATA_33 &&
		txIn[99] == txscript.OP_TSPEND) {
		return nil, nil, stakeRuleError(ErrTSpendInvalidScript,
			"TSPEND invalid tspend script")
	}

	// Pull out signature, pubkey.
	signature := txIn[1 : 1+schnorr.SignatureSize]
	pubKey := txIn[66 : 66+secp256k1.PubKeyBytesLenCompressed]
	if !txscript.IsStrictCompressedPubKeyEncoding(pubKey) {
		return nil, nil, stakeRuleError(ErrTSpendInvalidPubkey,
			"TSPEND invalid public key")
	}

	// Make sure TxOut[0] contains an OP_RETURN followed by a 32 byte data
	// push.
	if !txscript.IsStrictNullData(mtx.TxOut[0].Version,
		mtx.TxOut[0].PkScript, 32) {
		return nil, nil, stakeRuleError(ErrTSpendInvalidTransaction,
			"First TSPEND output should have been an OP_RETURN "+
				"followed by a 32 byte data push")
	}

	// Verify that the TxOut's contains a P2PKH or P2PKH scripts.
	for k, txOut := range mtx.TxOut[1:] {
		// All tx outs are tagged with OP_TGEN
		if txOut.PkScript[0] != txscript.OP_TGEN {
			return nil, nil, stakeRuleError(ErrTSpendInvalidTGen,
				fmt.Sprintf("Output %v is not tagged with "+
					"OP_TGEN", k+1))
		}
		if !(isPubKeyHashScript(txOut.PkScript[1:]) ||
			isScriptHashScript(txOut.PkScript[1:])) {

			return nil, nil, stakeRuleError(ErrTSpendInvalidSpendScript,
				fmt.Sprintf("Output %v is not P2SH or P2PKH", k+1))
		}
	}

	return signature, pubKey, nil
}

// checkTSpend verifies if a MsgTx is a valid TSPEND.
func checkTSpend(mtx *wire.MsgTx) error {
	_, _, err := CheckTSpend(mtx)
	return err
}

// IsTSpend returns true if the provided transaction is a proper TSPEND.
func IsTSpend(tx *wire.MsgTx) bool {
	return checkTSpend(tx) == nil
}

// checkTreasuryBase verifies that the provided MsgTx is a treasury base.
func checkTreasuryBase(mtx *wire.MsgTx) error {
	// Require version TxVersionTreasury.
	if mtx.Version != wire.TxVersionTreasury {
		return stakeRuleError(ErrTreasuryBaseInvalidTxVersion,
			fmt.Sprintf("invalid treasurybase script version: %v",
				mtx.Version))
	}

	// A TADD consists of one OP_TADD in PkScript[0] followed by an
	// OP_RETURN <random> in  PkScript[1].
	if len(mtx.TxIn) != 1 || len(mtx.TxOut) != 2 {
		return stakeRuleError(ErrTreasuryBaseInvalidCount,
			fmt.Sprintf("invalid treasurybase in/out script "+
				"count: %v/%v", len(mtx.TxIn),
				len(mtx.TxOut)))
	}

	// Ensure that there is no SignatureScript on the zeroth input.
	if len(mtx.TxIn[0].SignatureScript) != 0 {
		return stakeRuleError(ErrTreasuryBaseInvalidLength,
			"treasurybase input 0 contains a script")
	}

	// All output scripts must be version 0.
	const consensusScriptVer = 0
	for k := range mtx.TxOut {
		if mtx.TxOut[k].Version != consensusScriptVer {
			return stakeRuleError(ErrTreasuryBaseInvalidVersion,
				fmt.Sprintf("invalid script version found in "+
					"treasurybase: output %v", k))
		}
	}

	// First output must be a TADD
	if len(mtx.TxOut[0].PkScript) != 1 ||
		mtx.TxOut[0].PkScript[0] != txscript.OP_TADD {
		return stakeRuleError(ErrTreasuryBaseInvalidOpcode0,
			"first treasurybase output must be a TADD")
	}

	// Required OP_RETURN, OP_DATA_12 <4 bytes le encoded height>
	// <8 bytes random> = 14 bytes total.
	if len(mtx.TxOut[1].PkScript) != 14 ||
		mtx.TxOut[1].PkScript[0] != txscript.OP_RETURN ||
		mtx.TxOut[1].PkScript[1] != txscript.OP_DATA_12 {
		return stakeRuleError(ErrTreasuryBaseInvalidOpcode1,
			"second treasurybase output must be an OP_RETURN "+
				"OP_DATA_12 data script")
	}

	if !isNullOutpoint(mtx) {
		return stakeRuleError(ErrTreasuryBaseInvalid,
			"invalid treasurybase constants")
	}

	return nil
}

// CheckTreasuryBase verifies that the provided MsgTx is a treasury base. This
// is exported for testing purposes.
func CheckTreasuryBase(mtx *wire.MsgTx) error {
	return checkTreasuryBase(mtx)
}

// IsTreasuryBase returns true if the provided transaction is a treasury base
// transaction.
func IsTreasuryBase(tx *wire.MsgTx) bool {
	return checkTreasuryBase(tx) == nil
}
