// Copyright (c) 2020-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/blockchain/v5/chaingen"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/wire"
)

// TestFetchUtxoView ensures fetching a utxo viewpoint for the current tip block
// works as intended for various combinations of disapproval states for the tip
// block itself as well as when the tip block disapproves its parent.
func TestFetchUtxoView(t *testing.T) {
	const (
		// voteBitNo and voteBitYes represent no and yes votes, respectively, on
		// whether or not to approve the previous block.
		voteBitNo  = 0x0000
		voteBitYes = 0x0001
	)

	// Create a test harness initialized with the genesis block as the tip.
	params := chaincfg.RegNetParams()
	g := newChaingenHarness(t, params)

	// ---------------------------------------------------------------------
	// Create some convenience functions to improve test readability.
	// ---------------------------------------------------------------------

	// fetchUtxoViewTipApproved calls FetchUtxoView method with the flag set to
	// indicate that the transactions in the regular transaction tree of the tip
	// should be included.
	fetchUtxoViewTipApproved := func(tx *dcrutil.Tx) (*UtxoViewpoint, error) {
		return g.chain.FetchUtxoView(tx, true)
	}

	// fetchUtxoViewTipDisapproved calls FetchUtxoView method with the flag set
	// to indicate that the transactions in the regular transaction tree of the
	// tip should NOT be included.
	fetchUtxoViewTipDisapproved := func(tx *dcrutil.Tx) (*UtxoViewpoint, error) {
		return g.chain.FetchUtxoView(tx, false)
	}

	// testInputsSpent checks the provided view considers all of the transaction
	// outputs referenced by the inputs of the provided transaction spent
	// according to the given flag.
	testInputsSpent := func(view *UtxoViewpoint, tx *dcrutil.Tx, spent bool) {
		t.Helper()

		for txInIdx, txIn := range tx.MsgTx().TxIn {
			prevOut := &txIn.PreviousOutPoint
			entry := view.LookupEntry(*prevOut)
			gotSpent := entry == nil || entry.IsSpent()
			if gotSpent != spent {
				t.Fatalf("unexpected spent state for txo %s referenced by "+
					"input %d -- got %v, want %v", prevOut, txInIdx,
					gotSpent, spent)
			}
		}
	}

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// ---------------------------------------------------------------------
	// Create block that has a transaction available in its regular tx tree
	// to use as a base for the tests below.
	//
	//   ... -> b0
	// ---------------------------------------------------------------------

	outs := g.OldestCoinbaseOuts()
	b0 := g.NextBlock("b0", &outs[0], outs[1:])
	g.AcceptTipBlock()

	// Create a transaction that spends from an output in the regular tx tree of
	// the tip block.
	fee := dcrutil.Amount(1)
	b0Tx1Out0 := chaingen.MakeSpendableOut(b0, 1, 0)
	spendB0Tx1Out0 := dcrutil.NewTx(g.CreateSpendTx(&b0Tx1Out0, fee))

	// Ensure that fetching a view for a transaction that spends an output in
	// the regular tx tree of the tip block does NOT consider the referenced
	// output spent when the regular tx tree of the tip block is treated as
	// approved.
	view, err := fetchUtxoViewTipApproved(spendB0Tx1Out0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	testInputsSpent(view, spendB0Tx1Out0, false)

	// Ensure that fetching a view for a transaction that spends an output in
	// the regular tx tree of the tip block considers the referenced output
	// spent when the regular tx tree of the tip block is treated as disapproved
	// since they should be treated as if they do NOT exist.
	view, err = fetchUtxoViewTipDisapproved(spendB0Tx1Out0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	testInputsSpent(view, spendB0Tx1Out0, true)

	// ---------------------------------------------------------------------
	// Create block that disapproves the regular tx tree of the prev block
	// and does NOT include any disapproved regular transactions from it.
	//
	//   ... -> b0 -> b1 (disapproves b0)
	// ---------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b1", &outs[0], outs[1:], func(b *wire.MsgBlock) {
		b.Header.VoteBits &^= voteBitYes
		g.ReplaceVoteBits(voteBitNo)(b)
	})
	g.AssertTipDisapprovesPrevious()
	g.AcceptTipBlock()

	// Ensure that fetching a view for a transaction that is in the regular tx
	// tree of a disapproved block prior to the tip block when the tip block
	// does NOT otherwise spend those outputs does NOT consider the referenced
	// outputs spent (because the spend was undone by the disapproval)
	// regardless of whether or not the regular tx tree of the tip block is
	// treated as approved or disapproved.
	b0Tx1 := dcrutil.NewTx(b0.Transactions[1])
	view, err = fetchUtxoViewTipApproved(b0Tx1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	testInputsSpent(view, b0Tx1, false)

	view, err = fetchUtxoViewTipDisapproved(b0Tx1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	testInputsSpent(view, b0Tx1, false)

	// Ensure that fetching a view for a transaction that spends an output in
	// the regular tx tree of a disapproved block prior to the tip block when
	// the tip block does NOT otherwise spend the output considers the output
	// spent (since it was undone by the disapproval and therefore does NOT
	// exist) regardless of whether or not the regular tx tree of the tip block
	// is treated as approved or disapproved.
	view, err = fetchUtxoViewTipApproved(spendB0Tx1Out0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	testInputsSpent(view, spendB0Tx1Out0, true)

	view, err = fetchUtxoViewTipDisapproved(spendB0Tx1Out0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	testInputsSpent(view, spendB0Tx1Out0, true)

	// ---------------------------------------------------------------------
	// Create block that disapproves the regular tx tree of the prev block
	// and also includes one of those disapproved regular transactions and
	// then reorganize the chain to it.
	//
	//   ... -> b0 -> b1a (disapproves b0)
	//            \-> b1 (disapproves b0)
	// ---------------------------------------------------------------------

	g.SetTip("b0")
	b1a := g.NextBlock("b1a", &outs[0], outs[1:], func(b *wire.MsgBlock) {
		b.Header.VoteBits &^= voteBitYes
		g.ReplaceVoteBits(voteBitNo)(b)

		b.Transactions[1] = b0Tx1.MsgTx()
	})
	g.AssertTipDisapprovesPrevious()
	g.AcceptedToSideChainWithExpectedTip("b1")
	g.ForceTipReorg("b1", "b1a")

	// Ensure that fetching a view for a transaction that is in the regular tx
	// tree of a disapproved block prior to the tip block and is also included
	// in the tip block itself considers the referenced outputs spent when the
	// regular tx tree of the tip block is treated as approved.
	view, err = fetchUtxoViewTipApproved(b0Tx1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	testInputsSpent(view, b0Tx1, true)

	// Ensure that fetching a view for a transaction that is in the regular tx
	// tree of a disapproved block prior to the tip block and is also included
	// in the tip block itself does NOT consider the referenced outputs spent
	// when the regular tx tree of the tip block is treated as disapproved since
	// both instances of the transaction are disapproved.
	view, err = fetchUtxoViewTipDisapproved(b0Tx1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	testInputsSpent(view, b0Tx1, false)

	// Create a transaction that spends from an output in the regular tx tree of
	// the tip block.  Note that this is spending from the same transaction that
	// was disapproved in the block prior to the current tip and included in the
	// tip again.
	b1aTx1Out0 := chaingen.MakeSpendableOut(b1a, 1, 0)
	spendB1aTx1Out0 := dcrutil.NewTx(g.CreateSpendTx(&b1aTx1Out0, fee))

	// Ensure that fetching a view for a transaction that spends an output in
	// the regular tx tree of the tip block that was also included in the block
	// the tip disapproves does NOT consider the referenced output spent when
	// the regular tx tree of the tip block is treated as approved.
	view, err = fetchUtxoViewTipApproved(spendB1aTx1Out0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	testInputsSpent(view, spendB1aTx1Out0, false)

	// Ensure that fetching a view for a transaction that spends an output in
	// the regular tx tree of the tip block that was also included in the block
	// the tip disapproves consider the referenced output spent when the regular
	// tx tree of the tip block is treated as disapproved.
	view, err = fetchUtxoViewTipDisapproved(spendB1aTx1Out0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	testInputsSpent(view, spendB1aTx1Out0, true)
}

// testDisapprovalScenarios exercises the utxo state transitions that occur when
// a block disapproves the regular transaction tree of its parent along with
// reinclusion and spending of transactions and outputs resurrected by the
// disapproval.
//
// Concretely, it exercises the following scenarios:
//
//   - Disapproval followed by spending a resurrected output in a different
//     regular transaction
//   - Disapproval followed by spending a resurrected output in a stake
//     transaction (ticket purchase)
//   - Disapproval of a block containing a zero conf spend followed by
//     reinclusion of only the transaction that created the spent output
//   - Disapproval followed by reinclusion of a transaction and a zero conf
//     spend of its previously unspent output in the disapproving block
//   - Disapproval of a block containing a zero conf spend, followed by
//     reinclusion of the creating transaction and a new zero conf spend that
//     replaces the disapproved spend
func testDisapprovalScenarios(t *testing.T, flushUtxoCache bool) {
	// Create a test harness initialized with the genesis block as the tip and
	// advance it to stake validation height.
	params := chaincfg.RegNetParams()
	g := newChaingenHarness(t, params)
	g.AdvanceToStakeValidationHeight()
	g.ForceUtxoCacheFlush()

	if !flushUtxoCache {
		g.DisableUtxoCacheFlushes()
	}

	// outpoint is a convenience method for creating a regular tree outpoint for
	// the provided transaction and output index.
	outpoint := func(tx *wire.MsgTx, index uint32) wire.OutPoint {
		hash := tx.TxHash()
		return *wire.NewOutPoint(&hash, index, wire.TxTreeRegular)
	}

	// stakeOutpoint is a convenience method for creating a stake tree outpoint
	// for the provided transaction and output index.
	stakeOutpoint := func(tx *wire.MsgTx, index uint32) wire.OutPoint {
		hash := tx.TxHash()
		return *wire.NewOutPoint(&hash, index, wire.TxTreeStake)
	}

	// findSpendingTicket is a convenience method for finding which ticket in
	// the provided block spends the given output.
	findSpendingTicket := func(b *wire.MsgBlock, out wire.OutPoint) *wire.MsgTx {
		t.Helper()

		for _, tx := range b.STransactions {
			if !stake.IsSStx(tx) {
				continue
			}
			for _, txIn := range tx.TxIn {
				if txIn.PreviousOutPoint == out {
					return tx
				}
			}
		}
		t.Fatalf("no ticket spends outpoint %v (tree %d)", out, out.Tree)
		return nil
	}

	// disapproveParent is a munger that modifies the block to disapprove the
	// regular transaction tree of its parent.
	disapproveParent := func(b *wire.MsgBlock) {
		const (
			voteBitNo  = 0x0000
			voteBitYes = 0x0001
		)
		b.Header.VoteBits &^= voteBitYes
		g.ReplaceVoteBits(voteBitNo)(b)
	}

	// addTx returns a munger that modifies the block by adding the provided
	// transaction.
	addTx := func(tx *wire.MsgTx) func(b *wire.MsgBlock) {
		return func(b *wire.MsgBlock) {
			b.AddTransaction(tx)
		}
	}

	// addZeroConfSpend returns a munger that modifies the block by adding a
	// transaction that spends the provided output index of the transaction at
	// the provided index in the regular tree.
	addZeroConfSpend := func(txIndex, txOutIndex uint32) func(b *wire.MsgBlock) {
		return func(b *wire.MsgBlock) {
			const fee = dcrutil.Amount(1)
			spend := chaingen.MakeSpendableOut(b, txIndex, txOutIndex)
			b.AddTransaction(g.CreateSpendTx(&spend, fee))
		}
	}

	// -------------------------------------------------------------------------
	// Create a block with a regular transaction that spends a coinbase output,
	// followed by a block that disapproves the regular tx tree of the previous
	// block and spends the resurrected coinbase output with a different regular
	// transaction.
	//
	// This exercises spending an output that becomes available again when the
	// regular transaction tree of the previous block is disapproved.
	//
	//   ... -> s1b0 -> s1b1 (disapproves s1b0)
	//
	// s1b0: txA (spends a coinbase output)
	// s1b1: txB (spends the coinbase output previously spent by txA)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		sharedSpend := outs[0]
		b0 := g.NextBlock("s1b0", &sharedSpend, outs[1:])
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))

		ticketOuts := g.OldestCoinbaseOuts()
		b1 := g.NextBlock("s1b1", &sharedSpend, ticketOuts[1:], disapproveParent)
		g.AssertTipDisapprovesPrevious()
		g.AcceptTipBlock()
		g.ExpectTip("s1b1")
		txB := b1.Transactions[1]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))
	}

	// -------------------------------------------------------------------------
	// Create a block with a regular transaction that spends a coinbase output,
	// followed by a block that disapproves the regular tx tree of the previous
	// block and spends the resurrected coinbase output in the stake tree.
	//
	// This exercises the same utxo transition as the previous scenario, but
	// spends the restored output by a stake transaction.  This ensures the
	// correct handling applies to stake transactions as well as regular
	// transactions.
	//
	//   ... -> s2b0 -> s2b1 (disapproves s2b0)
	//
	// s2b0: txA (spends a coinbase output)
	// s2b1: ticket (spends the coinbase output previously spent by txA)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		sharedSpend := outs[0]
		b0 := g.NextBlock("s2b0", &sharedSpend, outs[1:])
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))

		// Replace the output the first ticket spends with the coinbase output
		// previously spent by txA.
		outs = g.OldestCoinbaseOuts()
		outs[1] = sharedSpend
		b1 := g.NextBlock("s2b1", nil, outs[1:], disapproveParent)
		g.AssertTipDisapprovesPrevious()
		g.AcceptTipBlock()
		g.ExpectTip("s2b1")
		ticket := findSpendingTicket(b1, sharedSpend.PrevOut())
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(stakeOutpoint(ticket, 0))
	}

	// -------------------------------------------------------------------------
	// Create a block with two regular transactions where the second transaction
	// spends an output from the first transaction as a zero conf spend,
	// followed by a block that disapproves the regular tx tree of the previous
	// block and reincludes only the first transaction.
	//
	// This exercises disapproval of a regular transaction tree where a tx
	// spends an output created earlier in that tree.  The disapproving block
	// reincludes the creating tx (txA) without reincluding its original zero
	// conf spend (txB), so the output of txA must be spendable and the output
	// of txB must not be spendable.
	//
	//   ... -> s3b0 -> s3b1 (disapproves s3b0)
	//
	// s3b0: txA (spends a coinbase output) -> txB (zero conf spend of txA)
	// s3b1: txA (reincluded from s3b0)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s3b0", &outs[0], outs[1:], addZeroConfSpend(1, 0))
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		txB := b0.Transactions[2]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))

		outs = g.OldestCoinbaseOuts()
		g.NextBlock("s3b1", nil, outs[1:], disapproveParent, addTx(txA))
		g.AssertTipDisapprovesPrevious()
		g.AcceptTipBlock()
		g.ExpectTip("s3b1")
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
	}

	// -------------------------------------------------------------------------
	// Create a block with a regular transaction that spends a coinbase output,
	// followed by a block that disapproves the regular tx tree of the previous
	// block, reincludes that transaction, and spends its output with a new zero
	// conf transaction.
	//
	// Unlike the previous scenario, the original block does not contain a zero
	// conf spend of the reincluded transaction.  This exercises spending an
	// output created by a transaction that is reincluded in the disapproving
	// block when that output was not spent in the original block.
	//
	//   ... -> s4b0 -> s4b1 (disapproves s4b0)
	//
	// s4b0: txA (spends a coinbase output)
	// s4b1: txA (reincluded from s4b0) -> txB (zero conf spend of txA)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s4b0", &outs[0], outs[1:])
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))

		outs = g.OldestCoinbaseOuts()
		b1 := g.NextBlock("s4b1", nil, outs[1:], disapproveParent, addTx(txA),
			addZeroConfSpend(1, 0))
		g.AssertTipDisapprovesPrevious()
		g.AcceptTipBlock()
		g.ExpectTip("s4b1")
		txB := b1.Transactions[2]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))
	}

	// -------------------------------------------------------------------------
	// Create a block with a regular transaction that spends a coinbase output,
	// followed by a block that disapproves the regular tx tree of the previous
	// block, reincludes that transaction, and spends its output with a new zero
	// conf transaction.
	//
	// This exercises replacing a zero conf spend from the disapproved regular
	// transaction tree with a different zero conf spend of the same output in
	// the disapproving block.  The resulting utxo state must reflect the new
	// spend rather than the displaced transaction.
	//
	//   ... -> s5b0 -> s5b1 (disapproves s5b0)
	//
	// s5b0: txA (spends a coinbase output) -> txB (zero conf spend of txA)
	// s5b1: txA (reincluded from s5b0) -> txC (new zero conf spend of txA)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s5b0", &outs[0], outs[1:], addZeroConfSpend(1, 0))
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		txB := b0.Transactions[2]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))

		outs = g.OldestCoinbaseOuts()
		b1 := g.NextBlock("s5b1", nil, outs[1:], disapproveParent, addTx(txA),
			addZeroConfSpend(1, 0))
		g.AssertTipDisapprovesPrevious()
		g.AcceptTipBlock()
		g.ExpectTip("s5b1")
		txC := b1.Transactions[2]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
		g.ExpectUtxoSpendable(outpoint(txC, 0))
	}
}

// TestDisapprovalScenarios exercises the UTXO state transitions caused by
// disapproval of the regular transaction tree of a parent block.
//
// See [testDisapprovalScenarios] for details of the tested scenarios.
//
// Each scenario is exercised with the UTXO cache flushing enabled and disabled
// to provide coverage of both cache states.
func TestDisapprovalScenarios(t *testing.T) {
	// Run the disapproval scenarios with flushes enabled to mimic the UTXO
	// cache behavior when the chain is current.  That is, the UTXO cache is
	// flushed after every block.
	t.Run("utxo cache flush enabled", func(t *testing.T) {
		testDisapprovalScenarios(t, true)
	})

	// Run the scenarios again with flushes disabled to mimic the UTXO cache
	// behavior during initial chain synchronization.
	t.Run("utxo cache flush disabled", func(t *testing.T) {
		testDisapprovalScenarios(t, false)
	})
}
