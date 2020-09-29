// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package standalone

import (
	"math"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

const (
	// These constants are opcodes are defined here to avoid a dependency on
	// txscript.  They are used in consensus code which can't be changed without
	// a vote anyway, so not referring to them directly via txscript is safe.
	opData12 = 0x0c
	opReturn = 0x6a
	opTAdd   = 0xc1
	opTSpend = 0xc2
	opTGen   = 0xc3
)

var (
	// zeroHash is the zero value for a chainhash.Hash and is defined as a
	// package level variable to avoid the need to create a new instance every
	// time a check is needed.
	zeroHash = chainhash.Hash{}
)

// isNullOutpoint determines whether or not a previous transaction output point
// is set.
func isNullOutpoint(tx *wire.MsgTx) bool {
	nullInOP := tx.TxIn[0].PreviousOutPoint
	if nullInOP.Index == math.MaxUint32 && nullInOP.Hash.IsEqual(&zeroHash) &&
		nullInOP.Tree == wire.TxTreeRegular {
		return true
	}
	return false
}

// IsCoinBaseTx determines whether or not a transaction is a coinbase.  A
// coinbase is a special transaction created by miners that has no inputs.
// This is represented in the block chain by a transaction with a single input
// that has a previous output transaction index set to the maximum value along
// with a zero hash.
func IsCoinBaseTx(tx *wire.MsgTx, isTreasuryEnabled bool) bool {
	// A coinbase must be version 3 once the treasury agenda is active.
	if isTreasuryEnabled && tx.Version != wire.TxVersionTreasury {
		return false
	}

	// A coinbase must only have one transaction input.
	if len(tx.TxIn) != 1 {
		return false
	}

	// The previous output of a coinbase must have a max value index and a
	// zero hash.
	prevOut := &tx.TxIn[0].PreviousOutPoint
	if prevOut.Index != math.MaxUint32 || prevOut.Hash != zeroHash {
		return false
	}

	// isTreasurySpendLike returns whether or not the provided transaction is
	// likely to be a treasury spend transaction for the purposes of
	// differentiating it from a coinbase.
	//
	// Note that this relies on the checks above to avoid panics.
	isTreasurySpendLike := func(tx *wire.MsgTx) bool {
		// Treasury spends have at least two outputs.
		if len(tx.TxOut) < 2 {
			return false
		}

		// Treasury spends have scripts in the first input and all outputs.
		l := len(tx.TxIn[0].SignatureScript)
		if l == 0 ||
			len(tx.TxOut[0].PkScript) == 0 ||
			len(tx.TxOut[1].PkScript) == 0 {

			return false
		}

		// Treasury spends have an OP_TSPEND as the last byte of the signature
		// script of the first input, an OP_RETURN as the first byte of the
		// public key script of the first output, and at least one output with
		// an OP_TGEN as the first byte of its public key script.
		return tx.TxIn[0].SignatureScript[l-1] == opTSpend &&
			tx.TxOut[0].PkScript[0] == opReturn &&
			tx.TxOut[1].PkScript[0] == opTGen
	}

	// Avoid detecting treasury spends as a coinbase transaction when the
	// treasury agenda is active.
	if isTreasuryEnabled && isTreasurySpendLike(tx) {
		return false
	}

	return true
}

// IsTreasuryBase does a minimal check to see if a transaction is a treasury
// base.
func IsTreasuryBase(tx *wire.MsgTx) bool {
	if tx.Version != wire.TxVersionTreasury {
		return false
	}

	if len(tx.TxIn) != 1 || len(tx.TxOut) != 2 {
		return false
	}

	if len(tx.TxIn[0].SignatureScript) != 0 {
		return false
	}

	if len(tx.TxOut[0].PkScript) != 1 ||
		tx.TxOut[0].PkScript[0] != opTAdd {
		return false
	}

	if len(tx.TxOut[1].PkScript) != 14 ||
		tx.TxOut[1].PkScript[0] != opReturn ||
		tx.TxOut[1].PkScript[1] != opData12 {
		return false
	}

	return isNullOutpoint(tx)
}
