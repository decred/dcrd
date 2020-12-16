// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"fmt"

	"github.com/decred/dcrd/dcrec/secp256k1/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v3/schnorr"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/wire"
)

const (
	// TSpendScriptLen is the exact length of a TSpend script.
	// <OP_DATA_64> <signature> <OP_DATA_33> <public key> <OP_TSPEND>
	// 1 + 64 + 1 + 33 + 1 = 100
	TSpendScriptLen = 100
)

// This file contains the functions that verify that treasury transactions
// strictly adhere to the specified format.
//
// == User sends to treasury ==
// TxIn:  Normal TxIn signature scripts
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

// checkTAdd verifies that the provided MsgTx is a valid TADD.
// Note: this function does not recognize treasurybase TADDs.
func checkTAdd(mtx *wire.MsgTx) error {
	// Require version TxVersionTreasury.
	if mtx.Version != wire.TxVersionTreasury {
		return stakeRuleError(ErrTAddInvalidTxVersion,
			fmt.Sprintf("invalid TADD script version: %v",
				mtx.Version))
	}

	// A TADD consists of one OP_TADD in PkScript[0] followed by 0 or 1
	// stake change outputs. It also requires at least one input.
	if !(len(mtx.TxOut) == 1 || len(mtx.TxOut) == 2) || len(mtx.TxIn) < 1 {
		return stakeRuleError(ErrTAddInvalidCount,
			fmt.Sprintf("invalid TADD script: TxIn %v TxOut %v",
				len(mtx.TxIn), len(mtx.TxOut)))
	}

	// Verify all TxOut script versions and lengths.
	for k := range mtx.TxOut {
		if mtx.TxOut[k].Version != consensusVersion {
			return stakeRuleError(ErrTAddInvalidVersion,
				fmt.Sprintf("invalid script version found "+
					"in TADD TxOut: %v", k))
		}

		if len(mtx.TxOut[k].PkScript) == 0 {
			return stakeRuleError(ErrTAddInvalidScriptLength,
				fmt.Sprintf("zero script length found in "+
					"TADD: %v", k))
		}
	}

	// First output must be a TADD
	if len(mtx.TxOut[0].PkScript) != 1 {
		return stakeRuleError(ErrTAddInvalidLength,
			fmt.Sprintf("TADD script length is not 1 byte, got %v",
				len(mtx.TxOut[0].PkScript)))
	}
	if mtx.TxOut[0].PkScript[0] != txscript.OP_TADD {
		return stakeRuleError(ErrTAddInvalidOpcode,
			fmt.Sprintf("first output must be a TADD, got 0x%x",
				mtx.TxOut[0].PkScript[0]))
	}

	// Only 1 stake change output allowed.
	if len(mtx.TxOut) == 2 {
		// Script length has been already verified.
		if !txscript.IsStakeChangeScript(mtx.TxOut[1].Version,
			mtx.TxOut[1].PkScript) {
			return stakeRuleError(ErrTAddInvalidChange,
				"second output must be an OP_SSTXCHANGE script")
		}
	}

	return nil
}

// CheckTAdd exports checkTAdd for testing purposes.
func CheckTAdd(mtx *wire.MsgTx) error {
	return checkTAdd(mtx)
}

// IsTAdd returns true if the provided transaction is a proper TADD.
func IsTAdd(tx *wire.MsgTx) bool {
	return checkTAdd(tx) == nil
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

	// Check to make sure that all output scripts are the consensus version.
	for k, txOut := range mtx.TxOut {
		if txOut.Version != consensusVersion {
			return nil, nil, stakeRuleError(ErrTSpendInvalidVersion,
				fmt.Sprintf("invalid script version found in "+
					"TxOut: %v", k))
		}

		// Make sure there is a script.
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
		if !(txscript.IsPubKeyHashScript(txOut.PkScript[1:]) ||
			txscript.IsPayToScriptHash(txOut.PkScript[1:])) {
			return nil, nil, stakeRuleError(ErrTSpendInvalidSpendScript,
				fmt.Sprintf("Output %v is not P2SH or P2PKH",
					k+1))
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

	// Verify all TxOut script versions.
	for k := range mtx.TxOut {
		if mtx.TxOut[k].Version != consensusVersion {
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
