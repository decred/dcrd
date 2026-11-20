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
//   - Reorganization from a disapproving block that reincludes a transaction
//     to a branch that approves the original parent and restores its zero conf
//     spend
//   - Reorganization away from and back to a branch that disapproves a parent
//     and reincludes a transaction from its regular tree
//   - Disapproval of a block containing a longer chain of zero conf spends,
//     followed by reinclusion of the first transaction, reconstruction of
//     the zero conf spend chain, and a reorganization away from and back
//     to that branch
//   - Consecutive disapprovals that reinclude the same transaction along with
//     a different zero conf spend of its output each time
//   - Rejection of a disapproving block that spends an output created by the
//     disapproved block without reincluding the creating transaction
//   - Rejection of a disapproving block that reincludes a zero conf spend
//     with the witness data from the disapproved block
//   - Reinclusion of a zero conf spend at a different position with updated
//     witness data, followed by rejection of a spend of its output that uses
//     the stale witness data and acceptance of one that uses the updated data
//   - Rejection of a disapproving block that spends an output of a reincluded
//     transaction prior to the point in the regular tree where it is reincluded
//   - Rejection of a disapproving block that spends an output of a reincluded
//     transaction prior to the point it is reincluded when a later zero conf
//     spend also spends that same output
//   - Rejection of a disapproving block that spends an output of a reincluded
//     transaction in the stake tree
//   - Rejection of a disapproving block that spends an output of a reincluded
//     transaction in the stake tree when a zero conf spend also spends it
//   - Rejection of a disapproving block that both spends a resurrected output
//     and reincludes the disapproved transaction that also spends it
//   - Rejection of a disapproving block that spends an output created by a
//     disapproved zero conf spend
//   - Rejection of a disapproving block that reincludes only a zero conf spend
//     without the transaction that created the spent output
//   - Rejection of a disapproving block that spends the same in-flight output
//     twice
//   - Rejection of a later block that spends an output of a disapproved block
//     that was already flushed to the utxo backend
//   - Rejection of a block that spends an output only reincluded on a branch
//     that is no longer the best chain
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

	// addSpend returns a munger that modifies the block by adding a transaction
	// that spends the provided output.
	addSpend := func(spend chaingen.SpendableOut) func(b *wire.MsgBlock) {
		return func(b *wire.MsgBlock) {
			const fee = dcrutil.Amount(1)
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
		g.SaveTipCoinbaseOuts()
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
		g.SaveTipCoinbaseOuts()
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
		g.SaveTipCoinbaseOuts()
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
		g.SaveTipCoinbaseOuts()
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
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		g.ExpectTip("s5b1")
		txC := b1.Transactions[2]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
		g.ExpectUtxoSpendable(outpoint(txC, 0))
	}

	// -------------------------------------------------------------------------
	// Create a block containing a zero conf spend, followed by a block that
	// disapproves the parent and reincludes only the transaction that created
	// the spent output.
	//
	// Then reorganize to a competing branch whose first block approves the
	// original parent.  This restores the original zero conf spend and removes
	// the effect of the reinclusion on the active utxo view.
	//
	//   ... -> s6b0 -> s6b1 (disapproves s6b0)
	//              \-> s6b1a (approves s6b0) -> s6b2a
	//
	// s6b0: txA (spends a coinbase output) -> txB (zero conf spend of txA)
	// s6b1: txA (reincluded from s6b0)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s6b0", &outs[0], outs[1:], addZeroConfSpend(1, 0))
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		txB := b0.Transactions[2]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))

		outs = g.OldestCoinbaseOuts()
		g.NextBlock("s6b1", nil, outs[1:], disapproveParent, addTx(txA))
		g.AssertTipDisapprovesPrevious()
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		g.ExpectTip("s6b1")
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))

		g.SetTip("s6b0")
		g.NextBlock("s6b1a", nil, outs[1:])
		g.AcceptedToSideChainWithExpectedTip("s6b1")
		g.NextBlock("s6b2a", nil, nil)
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		g.ExpectTip("s6b2a")
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))
	}

	// -------------------------------------------------------------------------
	// Create a block with two regular transactions where the second transaction
	// spends an output from the first transaction as a zero conf spend followed
	// by a block that disapproves the parent and reincludes its first
	// transaction.
	//
	// Then create a competing branch that disapproves the same parent without
	// reincluding either transaction.
	//
	// Finally, reorganize back to the branch that reincluded the transaction.
	//
	// This exercises disconnecting and reconnecting a branch that disapproves
	// the regular transaction tree of the parent block and reincludes its
	// creating transaction.  The utxo state must remain correct when the chain
	// switches first to a competing disapproving branch and then back again.
	//
	//   ... -> s7b0 -> s7b1 (disapproves s7b0)  -> s7b2  -> s7b3
	//              \-> s7b1a (disapproves s7b0) -> s7b2a
	//
	// s7b0:  txA (spends a coinbase output) -> txB (zero conf spend of txA)
	// s7b1:  txA (reincluded from s7b0)
	// s7b1a: disapproves s7b0 without reincluding txA or txB
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s7b0", &outs[0], outs[1:], addZeroConfSpend(1, 0))
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		txB := b0.Transactions[2]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))

		outs = g.OldestCoinbaseOuts()
		g.NextBlock("s7b1", nil, outs[1:], disapproveParent, addTx(txA))
		g.AssertTipDisapprovesPrevious()
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		g.ExpectTip("s7b1")
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))

		// Force a cache flush when automatic flushing is disabled to ensure the
		// output of txA is in the backend and therefore not fresh in the cache.
		// The reorganization below will then cause a spent entry in the cache
		// for the later zero conf spend.
		if !flushUtxoCache {
			g.ForceUtxoCacheFlush()
		}

		// Create a competing branch that disapproves the parent without
		// reincluding either of its regular transactions and eventually becomes
		// the best chain.
		g.SetTip("s7b0")
		g.NextBlock("s7b1a", &outs[0], outs[1:], disapproveParent)
		g.AssertTipDisapprovesPrevious()
		g.AcceptedToSideChainWithExpectedTip("s7b1")
		g.NextBlock("s7b2a", nil, nil)
		g.AcceptTipBlock()
		g.ExpectTip("s7b2a")
		g.ExpectUtxoSpendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))

		// Extend the reinclusion branch until it becomes the best chain again.
		g.SetTip("s7b1")
		g.NextBlock("s7b2", nil, nil)
		g.AcceptedToSideChainWithExpectedTip("s7b2a")
		g.NextBlock("s7b3", nil, nil)
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		g.ExpectTip("s7b3")
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
	}

	// -------------------------------------------------------------------------
	// Create a block with a chain of regular transactions where each
	// transaction spends an output from the preceding transaction as a zero
	// conf spend, followed by a block that disapproves the regular tx tree of
	// the previous block, reincludes the first transaction, and creates a new
	// chain of zero conf spends from its output.
	//
	// Then create a competing branch that disapproves the same parent without
	// reincluding any of the disapproved transactions.
	//
	// Finally, reorganize back to the branch that reincluded the transaction.
	//
	// This exercises disconnecting and reconnecting a longer zero conf
	// transaction chain when switching between competing branches that both
	// disapprove the same parent where one of the branches does not include any
	// of the original transactions and the other builds a new chain of spends
	// from the reincluded first transaction.
	//
	//   ... -> s8b0 -> s8b1 (disapproves s8b0)  -> s8b2  -> s8b3
	//              \-> s8b1a (disapproves s8b0) -> s8b2a
	//
	// s8b0:  txA -> txB -> txC -> txD
	// s8b1:  txA (reincluded from s8b0) -> txE -> txF -> txG
	// s8b1a: disapproves s8b0 without reincluding txA, txB, txC, or txD
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s8b0", &outs[0], outs[1:], addZeroConfSpend(1, 0),
			addZeroConfSpend(2, 0), addZeroConfSpend(3, 0))
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		txB := b0.Transactions[2]
		txC := b0.Transactions[3]
		txD := b0.Transactions[4]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
		g.ExpectUtxoUnspendable(outpoint(txC, 0))
		g.ExpectUtxoSpendable(outpoint(txD, 0))

		outs = g.OldestCoinbaseOuts()
		b1 := g.NextBlock("s8b1", nil, outs[1:], disapproveParent, addTx(txA),
			addZeroConfSpend(1, 0), addZeroConfSpend(2, 0),
			addZeroConfSpend(3, 0))
		g.AssertTipDisapprovesPrevious()
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		g.ExpectTip("s8b1")
		txE := b1.Transactions[2]
		txF := b1.Transactions[3]
		txG := b1.Transactions[4]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
		g.ExpectUtxoUnspendable(outpoint(txC, 0))
		g.ExpectUtxoUnspendable(outpoint(txD, 0))
		g.ExpectUtxoUnspendable(outpoint(txE, 0))
		g.ExpectUtxoUnspendable(outpoint(txF, 0))
		g.ExpectUtxoSpendable(outpoint(txG, 0))

		// Create a competing branch that disapproves the parent without
		// reincluding any of its regular transactions and eventually becomes
		// the best chain.
		g.SetTip("s8b0")
		g.NextBlock("s8b1a", &outs[0], outs[1:], disapproveParent)
		g.AssertTipDisapprovesPrevious()
		g.AcceptedToSideChainWithExpectedTip("s8b1")
		g.NextBlock("s8b2a", nil, nil)
		g.AcceptTipBlock()
		g.ExpectTip("s8b2a")
		g.ExpectUtxoSpendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
		g.ExpectUtxoUnspendable(outpoint(txC, 0))
		g.ExpectUtxoUnspendable(outpoint(txD, 0))
		g.ExpectUtxoUnspendable(outpoint(txE, 0))
		g.ExpectUtxoUnspendable(outpoint(txF, 0))
		g.ExpectUtxoUnspendable(outpoint(txG, 0))

		// Extend the reinclusion branch until it becomes the best chain again.
		g.SetTip("s8b1")
		g.NextBlock("s8b2", nil, nil)
		g.AcceptedToSideChainWithExpectedTip("s8b2a")
		g.NextBlock("s8b3", nil, nil)
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		g.ExpectTip("s8b3")
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
		g.ExpectUtxoUnspendable(outpoint(txC, 0))
		g.ExpectUtxoUnspendable(outpoint(txD, 0))
		g.ExpectUtxoUnspendable(outpoint(txE, 0))
		g.ExpectUtxoUnspendable(outpoint(txF, 0))
		g.ExpectUtxoSpendable(outpoint(txG, 0))
	}

	// -------------------------------------------------------------------------
	// Create a block with two regular transactions where the second transaction
	// spends an output from the first transaction as a zero conf spend,
	// followed by a block that disapproves the regular tx tree of the previous
	// block and reincludes the first transaction along with a new zero conf
	// spend, followed by another block that disapproves that block and
	// reincludes the first transaction along with yet another zero conf spend.
	//
	// This exercises consecutive disapprovals where the same transaction is
	// reincluded each time and the output it creates is spent by a different
	// zero conf spend in every block.
	//
	//   ... -> s9b0 -> s9b1 (disapproves s9b0) -> s9b2 (disapproves s9b1)
	//
	// s9b0: txA (spends a coinbase output) -> txB (zero conf spend of txA)
	// s9b1: txA (reincluded from s9b0) -> txC (new zero conf spend of txA)
	// s9b2: txA (reincluded from s9b1) -> txD (new zero conf spend of txA)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s9b0", &outs[0], outs[1:], addZeroConfSpend(1, 0))
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		txB := b0.Transactions[2]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))

		outs = g.OldestCoinbaseOuts()
		b1 := g.NextBlock("s9b1", nil, outs[1:], disapproveParent, addTx(txA),
			addZeroConfSpend(1, 0))
		g.AssertTipDisapprovesPrevious()
		g.AcceptTipBlock()
		txC := b1.Transactions[2]
		g.ExpectTip("s9b1")
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
		g.ExpectUtxoSpendable(outpoint(txC, 0))

		outs = g.OldestCoinbaseOuts()
		b2 := g.NextBlock("s9b2", nil, outs[1:], disapproveParent, addTx(txA),
			addZeroConfSpend(1, 0))
		g.AssertTipDisapprovesPrevious()
		g.AcceptTipBlock()
		txD := b2.Transactions[2]
		g.SaveTipCoinbaseOuts()
		g.ExpectTip("s9b2")
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
		g.ExpectUtxoUnspendable(outpoint(txC, 0))
		g.ExpectUtxoSpendable(outpoint(txD, 0))
	}

	// Get spendable outs to reuse for the upcoming rejected blocks.
	outsForReject := g.OldestCoinbaseOuts()

	// -------------------------------------------------------------------------
	// Create a block with a regular transaction that spends a coinbase output,
	// followed by a block that disapproves the regular tx tree of the previous
	// block and attempts to spend an output created by the disapproved
	// transaction without reincluding it.
	//
	// This exercises that outputs created by a disapproved regular transaction
	// are removed from the utxo set by the disapproval and therefore can't be
	// spent by the disapproving block itself.
	//
	//   ... -> s10b0
	//               \-> s10b1 (disapproves s10b0, rejected)
	//
	// s10b0: txA (spends a coinbase output)
	// s10b1: txB (spends an output of txA without reincluding txA)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s10b0", &outs[0], outs[1:])
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))

		outs = outsForReject
		txAOut0 := chaingen.MakeSpendableOut(b0, 1, 0)
		g.NextBlock("s10b1", nil, outs[1:], disapproveParent, addSpend(txAOut0))
		g.AssertTipDisapprovesPrevious()
		g.RejectTipBlock(ErrMissingTxOut)
		g.ExpectTip("s10b0")
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))
		g.SetTip("s10b0")
	}

	// -------------------------------------------------------------------------
	// Create a block with two regular transactions where the second transaction
	// spends an output from the first transaction as a zero conf spend,
	// followed by a block that disapproves the regular tx tree of the previous
	// block and reincludes both transactions without modification.
	//
	// This exercises that the fraud proof of a reincluded zero conf spend must
	// reference the block that reincludes the creating transaction rather than
	// the disapproved block that originally contained it.
	//
	//   ... -> s11b0
	//               \-> s11b1 (disapproves s11b0, rejected)
	//
	// s11b0: txA (spends a coinbase output) -> txB (zero conf spend of txA)
	// s11b1: txA (reincluded from s11b0) -> txB (reincluded from s11b0)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s11b0", &outs[0], outs[1:], addZeroConfSpend(1, 0))
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		txB := b0.Transactions[2]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))

		outs = outsForReject
		g.NextBlock("s11b1", nil, outs[1:], disapproveParent, addTx(txA),
			addTx(txB))
		g.AssertTipDisapprovesPrevious()
		g.RejectTipBlock(ErrFraudBlockHeight)
		g.ExpectTip("s11b0")
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))
		g.SetTip("s11b0")
	}

	// -------------------------------------------------------------------------
	// Create a block with two regular transactions where the second transaction
	// spends an output from the first transaction as a zero conf spend,
	// followed by a block that disapproves the regular tx tree of the previous
	// block and reincludes both transactions at different positions in the
	// regular tree with the witness data of the zero conf spend updated to
	// reference the new position of the creating transaction.  Note that the
	// witness data is not part of the transaction hash, so the reincluded zero
	// conf spend has the same hash as the disapproved one.
	//
	// Then attempt to spend the output of the reincluded zero conf spend using
	// the witness data from the disapproved block, followed by doing so using
	// the witness data from the block that reincludes it.
	//
	// This exercises that outputs recreated by a physically distinct
	// transaction with the same hash take on the details of the transaction
	// that recreates them.
	//
	//   ... -> s12b0 -> s12b1 (disapproves s12b0) -> s12b2
	//                                            \-> s12b2a (rejected)
	//
	// s12b0:  txA (spends a coinbase output) -> txB (zero conf spend of txA)
	// s12b1:  txW (unrelated) -> txA (reincluded from s12b0) ->
	//         txB' (txB with updated witness data, same hash as txB)
	// s12b2a: txE (spends an output of txB using the witness data of s12b0)
	// s12b2:  txF (spends an output of txB using the witness data of s12b1)
	// -------------------------------------------------------------------------
	{
		const txAOrigIdx = 1
		const txBOrigIdx = 2
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s12b0", &outs[0], outs[1:], addZeroConfSpend(1, 0))
		g.AcceptTipBlock()
		txA := b0.Transactions[txAOrigIdx]
		txB := b0.Transactions[txBOrigIdx]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))

		// Reinclude txA at index 2 and txB at index 3 with the witness data of
		// txB updated to reference the new location of txA.
		const txANewIdx = 2
		const txBNewIdx = 3
		outs = g.OldestCoinbaseOuts()
		txBNew := txB.Copy()
		reincludeTxB := func(b *wire.MsgBlock) {
			txBNew.TxIn[0].BlockHeight = b.Header.Height
			txBNew.TxIn[0].BlockIndex = txANewIdx
			b.AddTransaction(txBNew)
		}
		b1 := g.NextBlock("s12b1", &outs[0], outs[1:], disapproveParent,
			addTx(txA), reincludeTxB)
		g.AssertTipDisapprovesPrevious()
		g.AcceptTipBlock()
		if txBNew.TxHash() != txB.TxHash() {
			t.Fatalf("reincluded txB hash %v differs from the original %v",
				txBNew.TxHash(), txB.TxHash())
		}
		g.ExpectTip("s12b1")
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))

		// Attempt to spend the output of txB using the witness data from the
		// disapproved block.
		txBOutOld := chaingen.MakeSpendableOut(b0, txBOrigIdx, 0)
		b2a := g.NextBlock("s12b2a", nil, nil, addSpend(txBOutOld))
		g.RejectTipBlock(ErrFraudBlockHeight)
		g.ExpectTip("s12b1")
		txE := b2a.Transactions[1]
		g.ExpectUtxoSpendable(outpoint(txB, 0))
		g.ExpectUtxoUnspendable(outpoint(txE, 0))

		// Spend the output of txB using the witness data from the block that
		// reincludes it.
		g.SetTip("s12b1")
		txBOutNew := chaingen.MakeSpendableOut(b1, txBNewIdx, 0)
		b2 := g.NextBlock("s12b2", nil, nil, addSpend(txBOutNew))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		txF := b2.Transactions[1]
		g.ExpectTip("s12b2")
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
		g.ExpectUtxoSpendable(outpoint(txF, 0))
	}

	// -------------------------------------------------------------------------
	// Create a block with a regular transaction that spends a coinbase output,
	// followed by a block that disapproves the regular tx tree of the previous
	// block, spends an output of the disapproved transaction, and only then
	// reincludes the disapproved transaction.
	//
	// This exercises that a transaction in the disapproving block can only
	// spend the output of a reincluded transaction when the reincluded
	// transaction comes earlier in the regular tree.  In particular,
	// reinclusion later in the regular transaction tree must not retroactively
	// make outputs available to earlier transactions.
	//
	//   ... -> s13b0
	//               \-> s13b1 (disapproves s13b0, rejected)
	//
	// s13b0: txA (spends a coinbase output)
	// s13b1: txB (spends an output of txA) -> txA (reincluded from s13b0)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s13b0", &outs[0], outs[1:])
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))

		outs = outsForReject
		txAOut0 := chaingen.MakeSpendableOut(b0, 1, 0)
		b1 := g.NextBlock("s13b1", nil, outs[1:], disapproveParent,
			addSpend(txAOut0), addTx(txA))
		g.AssertTipDisapprovesPrevious()
		g.RejectTipBlock(ErrMissingTxOut)
		g.ExpectTip("s13b0")
		txB := b1.Transactions[1]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
		g.SetTip("s13b0")
	}

	// -------------------------------------------------------------------------
	// Create a block with a regular transaction that spends a coinbase output,
	// followed by a block that disapproves the regular tx tree of the previous
	// block, spends an output of the disapproved transaction before reincluding
	// it, and then spends the same output again after reincluding it with a
	// zero conf spend.
	//
	// This exercises that a transaction is never able to spend an output prior
	// to the point in the regular tree where the transaction that creates it is
	// located, even when a later transaction in the tree spends the same output
	// as an in-flight zero conf spend.
	//
	//   ... -> s14b0
	//               \-> s14b1 (disapproves s14b0, rejected)
	//
	// s14b0: txA (spends a coinbase output)
	// s14b1: txB (spends an output of txA), txA (reincluded from s14b0) ->
	//        txC (zero conf spend of the same output of txA)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s14b0", &outs[0], outs[1:])
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))

		outs = outsForReject
		var txB, txC *wire.MsgTx
		g.NextBlock("s14b1", nil, outs[1:], disapproveParent,
			func(b *wire.MsgBlock) {
				const fee = dcrutil.Amount(1)
				const txANewIdx = 2
				txB = g.CreateSpendTxForTx(txA, b.Header.Height, txANewIdx, fee)
				txC = g.CreateSpendTxForTx(txA, b.Header.Height, txANewIdx, fee)
				b.AddTransaction(txB)
				b.AddTransaction(txA)
				b.AddTransaction(txC)
			})
		g.AssertTipDisapprovesPrevious()
		g.RejectTipBlock(ErrMissingTxOut)
		g.ExpectTip("s14b0")
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
		g.ExpectUtxoUnspendable(outpoint(txC, 0))
		g.SetTip("s14b0")
	}

	// -------------------------------------------------------------------------
	// Create a block with a regular transaction that spends a coinbase output,
	// followed by a block that disapproves the regular tx tree of the previous
	// block, reincludes that transaction, and spends its output in the stake
	// tree with a ticket purchase.
	//
	// This exercises that the stake tree is not able to spend an output of a
	// reincluded transaction in the regular tree of the same block, even though
	// the output was available prior to the disapproval.
	//
	//   ... -> s15b0
	//               \-> s15b1 (disapproves s15b0, rejected)
	//
	// s15b0: txA (spends a coinbase output)
	// s15b1: txA (reincluded from s15b0), ticket (spends an output of txA)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s15b0", &outs[0], outs[1:])
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))

		// Replace the output the first ticket spends with the output of txA.
		outs = g.OldestCoinbaseOuts()
		outs[1] = chaingen.MakeSpendableOut(b0, 1, 0)
		b1 := g.NextBlock("s15b1", nil, outs[1:], disapproveParent, addTx(txA))
		g.AssertTipDisapprovesPrevious()
		g.RejectTipBlock(ErrMissingTxOut)
		g.ExpectTip("s15b0")
		ticket := findSpendingTicket(b1, outs[1].PrevOut())
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(stakeOutpoint(ticket, 0))
		g.SetTip("s15b0")
	}

	// -------------------------------------------------------------------------
	// Create a block with a regular transaction that spends a coinbase output,
	// followed by a block that disapproves the regular tx tree of the previous
	// block, reincludes that transaction, spends one of its outputs with a zero
	// conf spend, and also spends the same output in the stake tree with a
	// ticket purchase.
	//
	// This exercises that the stake tree is not able to spend an output of a
	// transaction reincluded in the regular tree of the same block even when
	// the output is already available to the view as an in-flight zero conf
	// spend.
	//
	//   ... -> s16b0
	//               \-> s16b1 (disapproves s16b0, rejected)
	//
	// s16b0: txA (spends a coinbase output)
	// s16b1: txA (reincluded from s16b0) -> txB (zero conf spend of txA),
	//        ticket (spends the same output of txA)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s16b0", &outs[0], outs[1:])
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))

		// Replace the output the first ticket spends with the output of txA.
		outs = g.OldestCoinbaseOuts()
		outs[1] = chaingen.MakeSpendableOut(b0, 1, 0)
		g.NextBlock("s16b1", nil, outs[1:], disapproveParent, addTx(txA),
			addZeroConfSpend(1, 0))
		g.AssertTipDisapprovesPrevious()
		g.RejectTipBlock(ErrMissingTxOut)
		g.ExpectTip("s16b0")
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))
		g.SetTip("s16b0")
	}

	// -------------------------------------------------------------------------
	// Create a block with a regular transaction that spends a coinbase output,
	// followed by a block that disapproves the regular tx tree of the previous
	// block, spends the resurrected coinbase output with a different regular
	// transaction, and also reincludes the disapproved transaction.
	//
	// This exercises that disapproval only makes the resurrected output
	// available once.  Reincluding the disapproved transaction is a double
	// spend when the output is also spent by another transaction in the
	// disapproving block.
	//
	//   ... -> s17b0
	//               \-> s17b1 (disapproves s17b0, rejected)
	//
	// s17b0: txA (spends a coinbase output)
	// s17b1: txB (spends the coinbase output previously spent by txA) ->
	//        txA (reincluded from s17b0)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		sharedSpend := outs[0]
		b0 := g.NextBlock("s17b0", &sharedSpend, outs[1:])
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))

		outs = outsForReject
		b1 := g.NextBlock("s17b1", &sharedSpend, outs[1:], disapproveParent,
			addTx(txA))
		g.AssertTipDisapprovesPrevious()
		g.RejectTipBlock(ErrMissingTxOut)
		g.ExpectTip("s17b0")
		txB := b1.Transactions[1]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
		g.SetTip("s17b0")
	}

	// -------------------------------------------------------------------------
	// Create a block with two regular transactions where the second transaction
	// spends an output from the first transaction as a zero conf spend,
	// followed by a block that disapproves the regular tx tree of the previous
	// block, reincludes only the first transaction, and attempts to spend the
	// output of the disapproved zero conf spend.
	//
	// This exercises that outputs created by a zero conf spend in a disapproved
	// regular transaction tree are removed from the utxo set by the disapproval
	// even though the creating transaction is not reincluded.
	//
	//   ... -> s18b0
	//               \-> s18b1 (disapproves s18b0, rejected)
	//
	// s18b0: txA (spends a coinbase output) -> txB (zero conf spend of txA)
	// s18b1: txA (reincluded from s18b0) -> txC (spends an output of txB)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s18b0", &outs[0], outs[1:], addZeroConfSpend(1, 0))
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		txB := b0.Transactions[2]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))

		outs = outsForReject
		txBOut0 := chaingen.MakeSpendableOut(b0, 2, 0)
		b1 := g.NextBlock("s18b1", nil, outs[1:], disapproveParent, addTx(txA),
			addSpend(txBOut0))
		g.AssertTipDisapprovesPrevious()
		g.RejectTipBlock(ErrMissingTxOut)
		g.ExpectTip("s18b0")
		txC := b1.Transactions[2]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))
		g.ExpectUtxoUnspendable(outpoint(txC, 0))
		g.SetTip("s18b0")
	}

	// -------------------------------------------------------------------------
	// Create a block with two regular transactions where the second transaction
	// spends an output from the first transaction as a zero conf spend,
	// followed by a block that disapproves the regular tx tree of the previous
	// block and reincludes only the zero conf spend.
	//
	// This exercises that a reincluded transaction is not able to spend an
	// output of a transaction that was disapproved and not reincluded, even
	// though the same spend was valid prior to the disapproval.
	//
	//   ... -> s19b0
	//               \-> s19b1 (disapproves s19b0, rejected)
	//
	// s19b0: txA (spends a coinbase output) -> txB (zero conf spend of txA)
	// s19b1: txB (reincluded from s19b0)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s19b0", &outs[0], outs[1:], addZeroConfSpend(1, 0))
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		txB := b0.Transactions[2]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))

		outs = outsForReject
		g.NextBlock("s19b1", nil, outs[1:], disapproveParent, addTx(txB))
		g.AssertTipDisapprovesPrevious()
		g.RejectTipBlock(ErrMissingTxOut)
		g.ExpectTip("s19b0")
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))
		g.SetTip("s19b0")
	}

	// -------------------------------------------------------------------------
	// Create a block with two regular transactions where the second transaction
	// spends an output from the first transaction as a zero conf spend,
	// followed by a block that disapproves the regular tx tree of the previous
	// block, reincludes the first transaction, and adds two different zero conf
	// spends of its output.
	//
	// This exercises that an output made available as an in-flight zero conf
	// spend in the disapproving block can only be spent once.
	//
	//   ... -> s20b0
	//               \-> s20b1 (disapproves s20b0, rejected)
	//
	// s20b0: txA (spends a coinbase output) -> txB (zero conf spend of txA)
	// s20b1: txA (reincluded from s20b0) -> txC (zero conf spend of txA),
	//        txD (another zero conf spend of the same output of txA)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s20b0", &outs[0], outs[1:], addZeroConfSpend(1, 0))
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		txB := b0.Transactions[2]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))

		outs = outsForReject
		b1 := g.NextBlock("s20b1", nil, outs[1:], disapproveParent, addTx(txA),
			addZeroConfSpend(1, 0), addZeroConfSpend(1, 0))
		g.AssertTipDisapprovesPrevious()
		g.RejectTipBlock(ErrMissingTxOut)
		g.ExpectTip("s20b0")
		txC := b1.Transactions[2]
		txD := b1.Transactions[3]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))
		g.ExpectUtxoUnspendable(outpoint(txC, 0))
		g.ExpectUtxoUnspendable(outpoint(txD, 0))
		g.SetTip("s20b0")
	}

	// -------------------------------------------------------------------------
	// Create a block with two regular transactions where the second transaction
	// spends an output from the first transaction as a zero conf spend,
	// followed by a block that disapproves the regular tx tree of the previous
	// block without reincluding any of its transactions, and then a block that
	// attempts to spend the output of the disapproved zero conf spend.
	//
	// This exercises that outputs of a disapproved regular transaction tree
	// remain unavailable to later blocks, including outputs that were already
	// flushed to the utxo backend prior to the disapproval.
	//
	//   ... -> s21b0 -> s21b1 (disapproves s21b0)
	//                                            \-> s21b2 (rejected)
	//
	// s21b0: txA (spends a coinbase output) -> txB (zero conf spend of txA)
	// s21b1: disapproves s21b0 without reincluding txA or txB
	// s21b2: txC (spends an output of txB)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s21b0", &outs[0], outs[1:], addZeroConfSpend(1, 0))
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		txB := b0.Transactions[2]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoSpendable(outpoint(txB, 0))

		// Force a cache flush when automatic flushing is disabled to ensure the
		// output of txB is in the backend prior to the disapproval.
		if !flushUtxoCache {
			g.ForceUtxoCacheFlush()
		}

		outs = g.OldestCoinbaseOuts()
		g.NextBlock("s21b1", nil, outs[1:], disapproveParent)
		g.AssertTipDisapprovesPrevious()
		g.AcceptTipBlock()
		g.ExpectTip("s21b1")
		g.ExpectUtxoSpendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))

		txBOut0 := chaingen.MakeSpendableOut(b0, 2, 0)
		b2 := g.NextBlock("s21b2", nil, nil, addSpend(txBOut0))
		g.RejectTipBlock(ErrMissingTxOut)
		g.ExpectTip("s21b1")
		txC := b2.Transactions[1]
		g.ExpectUtxoSpendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
		g.ExpectUtxoUnspendable(outpoint(txC, 0))
		g.SetTip("s21b1")
	}

	// -------------------------------------------------------------------------
	// Create a block with a regular transaction that spends a coinbase output
	// followed by a block that disapproves the parent and reincludes that
	// transaction.
	//
	// Then create a competing branch that disapproves the same parent without
	// reincluding the transaction and reorganize to it.
	//
	// Finally, attempt to extend the competing branch with a block that spends
	// the output of the transaction that was only reincluded on the branch that
	// is no longer the best chain.
	//
	// This exercises that disconnecting a disapproving block that reincludes a
	// transaction does not leave the reincluded outputs available once the
	// chain switches to a branch that disapproves the same parent without them.
	//
	//   ... -> s22b0 -> s22b1  (disapproves s22b0)
	//               \-> s22b1a (disapproves s22b0)  -> s22b2a
	//                                                        \-> s22b3a (rejected)
	//
	// s22b0:  txA (spends a coinbase output)
	// s22b1:  txA (reincluded from s22b0)
	// s22b1a: disapproves s22b0 without reincluding txA
	// s22b3a: txB (spends an output of txA)
	// -------------------------------------------------------------------------
	{
		outs := g.OldestCoinbaseOuts()
		b0 := g.NextBlock("s22b0", &outs[0], outs[1:])
		g.AcceptTipBlock()
		txA := b0.Transactions[1]
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))

		outs = g.OldestCoinbaseOuts()
		g.NextBlock("s22b1", nil, outs[1:], disapproveParent, addTx(txA))
		g.AssertTipDisapprovesPrevious()
		g.AcceptTipBlock()
		g.ExpectTip("s22b1")
		g.ExpectUtxoUnspendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoSpendable(outpoint(txA, 0))

		// Force a cache flush when automatic flushing is disabled to ensure the
		// output of txA is in the backend prior to the reorganization.
		if !flushUtxoCache {
			g.ForceUtxoCacheFlush()
		}

		// Create a competing branch that disapproves the parent without
		// reincluding its regular transaction and eventually becomes the best
		// chain.
		g.SetTip("s22b0")
		g.NextBlock("s22b1a", &outs[0], outs[1:], disapproveParent)
		g.AssertTipDisapprovesPrevious()
		g.AcceptedToSideChainWithExpectedTip("s22b1")
		g.NextBlock("s22b2a", nil, nil)
		g.AcceptTipBlock()
		g.ExpectTip("s22b2a")
		g.ExpectUtxoSpendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))

		// Attempt to spend the output of txA that only exists on the branch
		// that is no longer the best chain.
		txAOut0 := chaingen.MakeSpendableOut(b0, 1, 0)
		b3a := g.NextBlock("s22b3a", nil, nil, addSpend(txAOut0))
		g.RejectTipBlock(ErrMissingTxOut)
		g.ExpectTip("s22b2a")
		txB := b3a.Transactions[1]
		g.ExpectUtxoSpendable(txA.TxIn[0].PreviousOutPoint)
		g.ExpectUtxoUnspendable(outpoint(txA, 0))
		g.ExpectUtxoUnspendable(outpoint(txB, 0))
		g.SetTip("s22b2a")
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
