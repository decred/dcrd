// Copyright (c) 2020-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/decred/dcrd/blockchain/v4/chaingen"
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
