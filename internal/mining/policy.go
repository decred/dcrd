// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2016-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/wire"
)

const (
	// UnminedHeight is the height used for the "block" height field of the
	// contextual transaction information provided in a transaction store
	// when it has not yet been mined into a block.
	UnminedHeight = 0x7fffffff
)

// Policy houses the policy (configuration parameters) which is used to control
// the generation of block templates.  See the documentation for
// NewBlockTemplate for more details on each of these parameters are used.
type Policy struct {
	// BlockMinSize is the minimum block size in bytes to be used when
	// generating a block template.
	BlockMinSize uint32

	// BlockMaxSize is the maximum block size in bytes to be used when
	// generating a block template.
	BlockMaxSize uint32

	// BlockPrioritySize is the size in bytes for high-priority / low-fee
	// transactions to be used when generating a block template.
	BlockPrioritySize uint32

	// TxMinFreeFee is the minimum fee in Atoms/1000 bytes that is
	// required for a transaction to be treated as free for mining purposes
	// (block template generation).
	TxMinFreeFee dcrutil.Amount

	AggressiveMining bool

	// StandardVerifyFlags defines the function to retrieve the flags to
	// use for verifying scripts for the block after the current best block.
	// It must set the verification flags properly depending on the result
	// of any agendas that affect them.
	//
	// This function must be safe for concurrent access.
	StandardVerifyFlags func() (txscript.ScriptFlags, error)
}

// minInt is a helper function to return the minimum of two ints.  This avoids
// a math import and the need to cast to floats.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// calcInputValueAge is a helper function used to calculate the input age of
// a transaction.  The input age for a txin is the number of confirmations
// since the referenced txout multiplied by its output value.  The total input
// age is the sum of this value for each txin.  Any inputs to the transaction
// which are currently in the mempool and hence not mined into a block yet,
// contribute no additional input age to the transaction.
func calcInputValueAge(tx *wire.MsgTx, prioInputs PriorityInputser, nextBlockHeight int64) float64 {
	var totalInputAge float64
	for _, txIn := range tx.TxIn {
		// Don't attempt to accumulate the total input age if the
		// referenced transaction output doesn't exist.
		prevOut := &txIn.PreviousOutPoint
		originHeight, inputValue, ok := prioInputs.PriorityInput(prevOut)
		if ok {
			// Inputs with dependencies currently in the mempool
			// have their block height set to a special constant.
			// Their input age should be computed as zero since
			// their parent hasn't made it into a block yet.
			var inputAge int64
			if originHeight == UnminedHeight {
				inputAge = 0
			} else {
				inputAge = nextBlockHeight - originHeight
			}

			// Sum the input value times age.
			totalInputAge += float64(inputValue * inputAge)
		}
	}

	return totalInputAge
}

// CalcPriority returns a transaction priority given a transaction and the sum
// of each of its input values multiplied by their age (# of confirmations).
// Thus, the final formula for the priority is:
// sum(inputValue * inputAge) / adjustedTxSize
func CalcPriority(tx *wire.MsgTx, prioInputs PriorityInputser, nextBlockHeight int64) float64 {
	// In order to encourage spending multiple old unspent transaction
	// outputs thereby reducing the total set, don't count the constant
	// overhead for each input as well as enough bytes of the signature
	// script to cover a pay-to-script-hash redemption with a compressed
	// pubkey.  This makes additional inputs free by boosting the priority
	// of the transaction accordingly.  No more incentive is given to avoid
	// encouraging gaming future transactions through the use of junk
	// outputs.
	//
	// The constant overhead for a txin is 58 bytes since the previous
	// outpoint is 37 bytes + 4 bytes for the sequence + 8 bytes for the
	// input value + 4 bytes for the block height of the referenced output +
	// 4 bytes for the block index of the referenced output + 1 byte the
	// signature script length.
	//
	// A compressed pubkey pay-to-script-hash redemption with a maximum len
	// signature is of the form:
	// [OP_DATA_73 <73-byte sig> + OP_DATA_35 + {OP_DATA_33
	// <33 byte compressed pubkey> + OP_CHECKSIG}]
	//
	// Thus 1 + 73 + 1 + 1 + 33 + 1 = 110
	overhead := 0
	for _, txIn := range tx.TxIn {
		// Max inputs + size can't possibly overflow here.
		overhead += 58 + minInt(110, len(txIn.SignatureScript))
	}

	serializedTxSize := tx.SerializeSize()
	if overhead >= serializedTxSize {
		return 0.0
	}

	inputValueAge := calcInputValueAge(tx, prioInputs, nextBlockHeight)
	return inputValueAge / float64(serializedTxSize-overhead)
}
