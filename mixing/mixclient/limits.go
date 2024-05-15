package mixclient

import (
	"errors"

	"github.com/decred/dcrd/wire"
)

// nolint: unused
const (
	redeemP2PKHv0SigScriptSize = 1 + 73 + 1 + 33
	p2pkhv0PkScriptSize        = 1 + 1 + 1 + 20 + 1 + 1
)

// nolint: unused
func estimateP2PKHv0SerializeSize(inputs, outputs int, hasChange bool) int {
	// Sum the estimated sizes of the inputs and outputs.
	txInsSize := inputs * estimateInputSize(redeemP2PKHv0SigScriptSize)
	txOutsSize := outputs * estimateOutputSize(p2pkhv0PkScriptSize)

	changeSize := 0
	if hasChange {
		changeSize = estimateOutputSize(p2pkhv0PkScriptSize)
		outputs++
	}

	// 12 additional bytes are for version, locktime and expiry.
	return 12 + (2 * wire.VarIntSerializeSize(uint64(inputs))) +
		wire.VarIntSerializeSize(uint64(outputs)) +
		txInsSize + txOutsSize + changeSize
}

// estimateInputSize returns the worst case serialize size estimate for a tx input
//
// nolint: unused
func estimateInputSize(scriptSize int) int {
	return 32 + // previous tx
		4 + // output index
		1 + // tree
		8 + // amount
		4 + // block height
		4 + // block index
		wire.VarIntSerializeSize(uint64(scriptSize)) + // size of script
		scriptSize + // script itself
		4 // sequence
}

// estimateOutputSize returns the worst case serialize size estimate for a tx output
//
// nolint: unused
func estimateOutputSize(scriptSize int) int {
	return 8 + // previous tx
		2 + // version
		wire.VarIntSerializeSize(uint64(scriptSize)) + // size of script
		scriptSize // script itself
}

// nolint: unused
func estimateIsStandardSize(inputs, outputs int) bool {
	const maxSize = 100000

	estimated := estimateP2PKHv0SerializeSize(inputs, outputs, false)
	return estimated <= maxSize
}

// checkLimited determines if adding an peer with the provided unmixed values
// and a total number of mixed outputs would cause the transaction size to
// exceed the maximum allowed size.  Peers must be excluded from mixes if
// their contributions would cause the total transaction size to be too large,
// even if they have not acted maliciously in the mixing protocol.
//
// nolint: unused
func checkLimited(currentTx, unmixed *wire.MsgTx, totalMessages int) error {
	totalInputs := len(currentTx.TxIn) + len(unmixed.TxIn)
	totalOutputs := len(currentTx.TxOut) + len(unmixed.TxOut) + totalMessages
	if !estimateIsStandardSize(totalInputs, totalOutputs) {
		return errors.New("tx size would exceed standardness rules")
	}
	return nil
}
