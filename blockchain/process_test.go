// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"testing"

	"github.com/decred/dcrd/blockchain/v3/chaingen"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v2"
	"github.com/decred/dcrd/wire"
)

// TestProcessOrder ensures processing-specific logic such as orphan handling,
// duplicate block handling, and out-of-order reorgs to invalid blocks works as
// expected.
func TestProcessOrder(t *testing.T) {
	// Create a test harness initialized with the genesis block as the tip.
	params := chaincfg.RegNetParams()
	g, teardownFunc := newChaingenHarness(t, params, "processordertest")
	defer teardownFunc()

	// Shorter versions of useful params for convenience.
	coinbaseMaturity := params.CoinbaseMaturity
	stakeValidationHeight := params.StakeValidationHeight

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// ---------------------------------------------------------------------
	// Generate enough blocks to have a known distance to the first mature
	// coinbase outputs for all tests that follow.  These blocks continue
	// to purchase tickets to avoid running out of votes.
	//
	//   ... -> bsv# -> bbm0 -> bbm1 -> ... -> bbm#
	// ---------------------------------------------------------------------

	for i := uint16(0); i < coinbaseMaturity; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bbm%d", i)
		g.NextBlock(blockName, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight) + uint32(coinbaseMaturity))

	// Collect spendable outputs into two different slices.  The outs slice
	// is intended to be used for regular transactions that spend from the
	// output, while the ticketOuts slice is intended to be used for stake
	// ticket purchases.
	var outs []*chaingen.SpendableOut
	var ticketOuts [][]chaingen.SpendableOut
	for i := uint16(0); i < coinbaseMaturity; i++ {
		coinbaseOuts := g.OldestCoinbaseOuts()
		outs = append(outs, &coinbaseOuts[0])
		ticketOuts = append(ticketOuts, coinbaseOuts[1:])
	}

	// Ensure duplicate blocks are rejected.
	//
	//   ... -> b1(0)
	//      \-> b1(0)
	g.NextBlock("b1", outs[0], ticketOuts[0])
	g.AcceptTipBlock()
	g.RejectTipBlock(ErrDuplicateBlock)

	// ---------------------------------------------------------------------
	// Orphan tests.
	// ---------------------------------------------------------------------

	// Create valid orphan block with zero prev hash.
	//
	//   No previous block
	//                    \-> borphan0(1)
	g.SetTip("b1")
	g.NextBlock("borphan0", outs[1], ticketOuts[1], func(b *wire.MsgBlock) {
		b.Header.PrevBlock = chainhash.Hash{}
	})
	g.RejectTipBlock(ErrMissingParent)

	// Create valid orphan block.
	//
	//   ... -> b1(0)
	//               \-> borphanbase(1) -> borphan1(2)
	g.SetTip("b1")
	g.NextBlock("borphanbase", outs[1], ticketOuts[1])
	g.NextBlock("borphan1", outs[2], ticketOuts[2])
	g.RejectTipBlock(ErrMissingParent)

	// Ensure duplicate orphan blocks are rejected.
	g.RejectTipBlock(ErrMissingParent)

	// ---------------------------------------------------------------------
	// Out-of-order forked reorg to invalid block tests.
	//
	// NOTE: These tests have been modified to be processed in order despite
	// the comments for now due to the removal of orphan processing.  They
	// are otherwise left intact because future commits will allow headers
	// to be processed independently from the block data which will allow
	// out of order processing of data again so long as the headers are
	// processed in order.
	// ---------------------------------------------------------------------

	// Create a fork that ends with block that generates too much proof-of-work
	// coinbase, but with a valid fork first.
	//
	//   ... -> b1(0) -> b2(1)
	//               \-> bpw1(1) -> bpw2(2) -> bpw3(3)
	//                  (bpw1 added last)
	g.SetTip("b1")
	g.NextBlock("b2", outs[1], ticketOuts[1])
	g.AcceptTipBlock()
	g.ExpectTip("b2")

	g.SetTip("b1")
	g.NextBlock("bpw1", outs[1], ticketOuts[1])
	g.AcceptedToSideChainWithExpectedTip("b2")
	g.NextBlock("bpw2", outs[2], ticketOuts[2])
	g.NextBlock("bpw3", outs[3], ticketOuts[3], func(b *wire.MsgBlock) {
		// Increase the first proof-of-work coinbase subsidy.
		b.Transactions[0].TxOut[2].Value++
	})
	g.AcceptBlock("bpw2")
	g.RejectBlock("bpw3", ErrBadCoinbaseValue)
	g.ExpectTip("bpw2")

	// Create a fork that ends with block that generates too much dev-org
	// coinbase, but with a valid fork first.
	//
	//   ... -> b1(0) -> bpw1(1) -> bpw2(2)
	//                          \-> bdc1(2) -> bdc2(3) -> bdc3(4)
	//                             (bdc1 added last)
	g.SetTip("bpw1")
	g.NextBlock("bdc1", outs[2], ticketOuts[2])
	g.AcceptedToSideChainWithExpectedTip("bpw2")
	g.NextBlock("bdc2", outs[3], ticketOuts[3])
	g.NextBlock("bdc3", outs[4], ticketOuts[4], func(b *wire.MsgBlock) {
		// Increase the proof-of-work dev subsidy by the provided amount.
		b.Transactions[0].TxOut[0].Value++
	})
	g.AcceptBlock("bdc2")
	g.RejectBlock("bdc3", ErrNoTax)
	g.ExpectTip("bdc2")
}
