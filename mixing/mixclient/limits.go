package mixclient

import (
	"errors"

	"github.com/decred/dcrd/wire"
)

var errExceedsStandardSize = errors.New("tx size would exceed standardness rules")

const (
	redeemP2PKHv0SigScriptSize = 1 + 73 + 1 + 33
	p2pkhv0PkScriptSize        = 1 + 1 + 1 + 20 + 1 + 1
)

var estimatedRedeemP2PKHv0InputSize = estimateInputSize(redeemP2PKHv0SigScriptSize)

// estimateInputSize returns the worst case serialize size estimate for a tx input
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
func estimateOutputSize(scriptSize int) int {
	return 8 + // previous tx
		2 + // version
		wire.VarIntSerializeSize(uint64(scriptSize)) + // size of script
		scriptSize // script itself
}

func estimateP2PKHv0SerializeSize(inputs int, outputScriptSizes []int) int {
	// Sum the estimated sizes of the inputs and outputs.
	txInsSize := inputs * estimatedRedeemP2PKHv0InputSize

	var txOutsSize int
	for _, sz := range outputScriptSizes {
		txOutsSize += estimateOutputSize(sz)
	}

	// 12 additional bytes are for version, locktime and expiry.
	return 12 + (2 * wire.VarIntSerializeSize(uint64(inputs))) +
		wire.VarIntSerializeSize(uint64(len(outputScriptSizes))) +
		txInsSize + txOutsSize
}

type coinjoinSize struct {
	currentInputs            int
	currentOutputScriptSizes []int
}

// join determines if adding inputs, mixed outputs, and potentially a change
// output from a peer would cause the coinjoin transaction size to exceed the
// maximum allowed size.  The coinjoin size estimator is updated if the peer
// can be included, and remains unchanged (returning an error) when it can't.
// Peers must be excluded from mixes if their contributions would cause the
// total transaction size to be too large, even if they have not acted
// maliciously in the mixing protocol.
func (c *coinjoinSize) join(contributedInputs, mcount int, change *wire.TxOut) error {
	const maxStandardSize = 100000

	totalInputs := c.currentInputs + contributedInputs

	l := len(c.currentOutputScriptSizes)
	for i := 0; i < mcount; i++ {
		c.currentOutputScriptSizes = append(c.currentOutputScriptSizes, p2pkhv0PkScriptSize)
	}
	if change != nil {
		c.currentOutputScriptSizes = append(c.currentOutputScriptSizes, len(change.PkScript))
	}
	totalOutputScriptSizes := c.currentOutputScriptSizes
	c.currentOutputScriptSizes = c.currentOutputScriptSizes[:l]

	estimated := estimateP2PKHv0SerializeSize(totalInputs, totalOutputScriptSizes)
	if estimated >= maxStandardSize {
		return errExceedsStandardSize
	}

	c.currentInputs = totalInputs
	c.currentOutputScriptSizes = totalOutputScriptSizes

	return nil
}
