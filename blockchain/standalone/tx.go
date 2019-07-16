// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package standalone

import (
	"math"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/txscript/v3"
	"github.com/decred/dcrd/wire"
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
	if isTreasuryEnabled && tx.Version != wire.TxVersionTreasury {
		return false
	}

	// A coin base must only have one transaction input.
	if len(tx.TxIn) != 1 {
		return false
	}

	// The previous output of a coin base must have a max value index and a
	// zero hash.
	prevOut := &tx.TxIn[0].PreviousOutPoint
	if prevOut.Index != math.MaxUint32 || prevOut.Hash != zeroHash {
		return false
	}

	// We need to do additional testing when treasury is enabled or a
	// TSPEND will be recognized as a coinbase transaction.
	if isTreasuryEnabled {
		// TSpends have at least 2 outputs.
		if len(tx.TxOut) < 2 {
			return false
		}
		// TSpends have scripts in TxIn[0] and all TxOut.
		l := len(tx.TxIn[0].SignatureScript)
		if l == 0 ||
			len(tx.TxOut[0].PkScript) == 0 ||
			len(tx.TxOut[1].PkScript) == 0 {
			return false
		}
		// TSpends have a TSpend opcode as the last byte of TxIn 0 and
		// an OP_RETURN followed by at least one OP_TGEN in the zeroth
		// TxOut script.
		if tx.TxIn[0].SignatureScript[l-1] == txscript.OP_TSPEND &&
			tx.TxOut[0].PkScript[0] == txscript.OP_RETURN &&
			tx.TxOut[1].PkScript[0] == txscript.OP_TGEN {
			return false
		}
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
		tx.TxOut[0].PkScript[0] != txscript.OP_TADD {
		return false
	}

	if len(tx.TxOut[1].PkScript) != 14 ||
		tx.TxOut[1].PkScript[0] != txscript.OP_RETURN ||
		tx.TxOut[1].PkScript[1] != txscript.OP_DATA_12 {
		return false
	}

	return isNullOutpoint(tx)
}
