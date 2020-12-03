// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"testing"

	"github.com/decred/dcrd/blockchain/v4"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/wire"
)

// Tests the behavior of the mining view when returned from a tx source
// containing a transaction chain depicted as:
//
//                       /--> d --> f
//                       |
//                 |--> b --|
// <coinbase>  --> a        |-->e
//                 |--> c --|
//
//
func TestMiningView(t *testing.T) {
	harness, spendableOuts, err := newMiningHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create mining harness: %v", err)
	}

	applyTxFee := func(fee int64) func(*wire.MsgTx) {
		return func(tx *wire.MsgTx) {
			tx.TxOut[0].Value -= fee
		}
	}

	// All transactions have the same number of outputs to keep the size uniform.
	txA, _ := harness.CreateSignedTx([]spendableOutput{
		spendableOuts[0],
	}, 2, applyTxFee(1000))

	txB, _ := harness.CreateSignedTx([]spendableOutput{
		txOutToSpendableOut(txA, 0, wire.TxTreeRegular),
	}, 2, applyTxFee(5000))

	txC, _ := harness.CreateSignedTx([]spendableOutput{
		txOutToSpendableOut(txA, 1, wire.TxTreeRegular),
	}, 2, applyTxFee(5000))

	txD, _ := harness.CreateSignedTx([]spendableOutput{
		txOutToSpendableOut(txB, 0, wire.TxTreeRegular),
	}, 2, applyTxFee(5000))

	txE, _ := harness.CreateSignedTx([]spendableOutput{
		txOutToSpendableOut(txB, 1, wire.TxTreeRegular),
		txOutToSpendableOut(txC, 0, wire.TxTreeRegular),
	}, 2, applyTxFee(5000))

	txF, _ := harness.CreateSignedTx([]spendableOutput{
		txOutToSpendableOut(txD, 0, wire.TxTreeRegular),
	}, 2, applyTxFee(0))

	// Add all to the tx source.  Note that txB is out of order to test the
	// behavior of an orphan transaction with ancestors brought into the tx source.
	allTxns := []*dcrutil.Tx{txB, txA, txC, txD, txE}
	for _, tx := range allTxns {
		_, err = harness.AddTransactionToTxSource(tx)
		if err != nil {
			t.Fatalf("unable to add transaction to tx source: %v", err)
		}
	}

	txALen := int64(txA.MsgTx().SerializeSize())
	txBLen := int64(txB.MsgTx().SerializeSize())
	txCLen := int64(txC.MsgTx().SerializeSize())

	txATotalSigOps, err := harness.CountTotalSigOps(txA)
	if err != nil {
		t.Fatalf("failed to count sigops for txA %v", err)
	}

	txBTotalSigOps, err := harness.CountTotalSigOps(txB)
	if err != nil {
		t.Fatalf("failed to count sigops for txB %v", err)
	}

	txCTotalSigOps, err := harness.CountTotalSigOps(txC)
	if err != nil {
		t.Fatalf("failed to count sigops for txC %v", err)
	}

	tests := []struct {
		name                 string
		miningView           TxMiningView
		subject              *dcrutil.Tx
		expectedAncestorFees int64
		expectedSizeBytes    int64
		expectedSigOps       int
		ancestors            []*dcrutil.Tx
		descendants          map[chainhash.Hash]*dcrutil.Tx
		orderedAncestors     [][]*dcrutil.Tx
	}{{
		name:                 "Tx A",
		subject:              txA,
		expectedAncestorFees: 0,
		expectedSizeBytes:    0,
		expectedSigOps:       0,
		ancestors:            nil,
		descendants: map[chainhash.Hash]*dcrutil.Tx{
			txB.MsgTx().TxHash(): txB,
			txC.MsgTx().TxHash(): txC,
			txD.MsgTx().TxHash(): txD,
			txE.MsgTx().TxHash(): txE,
		},
		orderedAncestors: nil,
	}, {
		name:                 "Tx B",
		subject:              txB,
		expectedAncestorFees: 1000,
		expectedSizeBytes:    txALen,
		expectedSigOps:       txATotalSigOps,
		ancestors:            []*dcrutil.Tx{txA},
		descendants: map[chainhash.Hash]*dcrutil.Tx{
			txD.MsgTx().TxHash(): txD,
			txE.MsgTx().TxHash(): txE,
		},
		orderedAncestors: [][]*dcrutil.Tx{
			{txA},
		},
	}, {
		name:                 "Tx C",
		subject:              txC,
		expectedAncestorFees: 1000,
		expectedSizeBytes:    txALen,
		expectedSigOps:       txATotalSigOps,
		ancestors:            []*dcrutil.Tx{txA},
		descendants: map[chainhash.Hash]*dcrutil.Tx{
			txE.MsgTx().TxHash(): txE,
		},
		orderedAncestors: [][]*dcrutil.Tx{
			{txA},
		},
	}, {
		name:                 "Tx D",
		subject:              txD,
		expectedAncestorFees: 6000,
		expectedSizeBytes:    txALen + txBLen,
		expectedSigOps:       txATotalSigOps + txBTotalSigOps,
		ancestors:            []*dcrutil.Tx{txA, txB},
		descendants:          map[chainhash.Hash]*dcrutil.Tx{},
		orderedAncestors: [][]*dcrutil.Tx{
			{txA, txB},
		},
	}, {
		name:                 "Tx E",
		subject:              txE,
		expectedAncestorFees: 11000,
		expectedSizeBytes:    txALen + txBLen + txCLen,
		expectedSigOps: txATotalSigOps + txBTotalSigOps +
			txCTotalSigOps,
		ancestors:   []*dcrutil.Tx{txA, txB, txC},
		descendants: map[chainhash.Hash]*dcrutil.Tx{},
		orderedAncestors: [][]*dcrutil.Tx{
			{txA, txB, txC},
			{txA, txC, txB},
		},
	}}

	for _, test := range tests {
		txHash := test.subject.Hash()
		txDesc := harness.txSource.pool[*txHash]
		miningView := harness.txSource.MiningView()

		// Make sure transaction is not rejected between view instances.
		if miningView.isRejected(txHash) {
			t.Fatalf("%v: expected test subject txn %v to not be rejected",
				test.name, *txHash)
		}

		// Ensure stats are calculated before retrieving ancestors.
		initialStat, hasStats := miningView.AncestorStats(txHash)
		if !hasStats {
			t.Fatalf("%v: expected mining view to have ancestor stats",
				test.name)
		}

		if test.expectedAncestorFees != initialStat.Fees {
			t.Fatalf("%v: expected subject txn to have bundle fees %v, got %v",
				test.name, test.expectedAncestorFees, initialStat.Fees)
		}

		if test.expectedSizeBytes != initialStat.SizeBytes {
			t.Fatalf("%v: expected subject txn to have SizeBytes=%v, got %v",
				test.name, test.expectedSizeBytes, initialStat.SizeBytes)
		}

		if test.expectedSigOps != initialStat.TotalSigOps {
			t.Fatalf("%v: expected subject txn to have SigOps=%v, got %v",
				test.name, test.expectedSigOps, initialStat.TotalSigOps)
		}

		if len(test.ancestors) != initialStat.NumAncestors {
			t.Fatalf("%v: expected subject txn to have NumAncestors=%v, got %v",
				test.name, len(test.ancestors), initialStat.NumAncestors)
		}

		// Retrieving ancestors will cause the cached stats to be updated.
		ancestors := miningView.ancestors(txHash)
		stat, _ := miningView.AncestorStats(txHash)

		if initialStat == stat {
			t.Fatalf("%v: expected ancestor stats reference to have changed "+
				"after retrieving list of ancestors", test.name)
		}

		// Get snapshot of transaction relationships as they exist in the tx source.
		descendants := harness.txSource.miningView.descendants(txHash)

		if len(test.ancestors) != len(ancestors) {
			t.Fatalf("%v: expected subject txn to have %v ancestors, got %v",
				test.name, len(test.ancestors), len(ancestors))
		}

		if len(ancestors) != stat.NumAncestors {
			t.Fatalf("%v: expected subject txn to have NumAncestors=%v, got %v",
				test.name, len(ancestors), stat.NumAncestors)
		}

		if len(descendants) != stat.NumDescendants {
			t.Fatalf("%v: expected subject txn to have NumDescendants=%v, "+
				"got %v", test.name, len(descendants), stat.NumDescendants)
		}

		for _, descendantHash := range descendants {
			if _, exist := test.descendants[*descendantHash]; !exist {
				t.Fatalf("%v: expected subject txn to have descendant=%v",
					test.name, descendantHash)
			}
		}

		// Ensure that transactions have a valid order, and that one of the test's
		// orderedAncestors matches ancestors returned from the view exactly.
		exactMatch := true
		for _, ancestorGroups := range test.orderedAncestors {
			exactMatch = true
			for index, ancestor := range ancestorGroups {
				if *ancestors[index].Tx.Hash() != *ancestor.Hash() {
					exactMatch = false
					break
				}
			}

			if exactMatch {
				break
			}
		}

		if !exactMatch {
			t.Fatalf("%v: subject txn ancestors returned out of order",
				test.name)
		}

		if miningView.hasParents(txHash) && len(test.ancestors) == 0 {
			t.Fatalf("%v: expected subject txn to not have 0 parents, got %v",
				test.name, len(test.ancestors))
		}

		// Make sure all parents are valid outpoints from this tx.
		txInputHashes := make(map[chainhash.Hash]struct{})
		for _, outpoint := range test.subject.MsgTx().TxIn {
			txInputHashes[outpoint.PreviousOutPoint.Hash] = struct{}{}
		}

		for _, parentDesc := range miningView.parents(txHash) {
			if _, exist := txInputHashes[*parentDesc.Tx.Hash()]; !exist {
				t.Fatalf("%v: test subject %v has invalid parent %v",
					test.name, *txHash, *parentDesc.Tx.Hash())
			}
		}

		// if the transaction has children, make sure it's no longer the child's
		// ancestor after removal.
		removalMiningView := harness.txSource.MiningView()
		if children := removalMiningView.children(txHash); len(children) > 0 {
			removalMiningView.RemoveTransaction(txHash, false)
			child := children[0]
			siblings := removalMiningView.parents(child.Tx.Hash())

			// Make sure ancestor stats have not changed.
			oldStat, hasStats := miningView.AncestorStats(child.Tx.Hash())
			if !hasStats {
				t.Fatalf("%v: Expected ancestor stats to be available",
					test.name)
			}

			newStat, hasStats := removalMiningView.AncestorStats(
				child.Tx.Hash())
			if !hasStats {
				t.Fatalf("%v: Expected ancestor stats to be available",
					test.name)
			}

			if *oldStat != *newStat {
				t.Fatalf("%v: expected test subject's child bundle stats to "+
					"remain unchanged.", test.name)
			}

			for _, sibling := range siblings {
				if *sibling.Tx.Hash() == *txHash {
					t.Fatalf("%v: expected test subject to no longer be an "+
						"ancestor of %v after being removed from the view.",
						test.name, *sibling.Tx.Hash())
				}
			}
		}

		// reset the mining view after removing the tx and ensure that all descendants
		// have the fee removed for this transaction when calling Remove with the
		// option to do so.
		removalMiningView = harness.txSource.MiningView()
		removalMiningView.RemoveTransaction(txHash, true)

		for _, descendant := range descendants {
			oldStat, hasStats := miningView.AncestorStats(descendant)
			if !hasStats {
				t.Fatalf("%v: Expected ancestor stats to be available",
					test.name)
			}

			newStat, hasStats := removalMiningView.AncestorStats(descendant)
			if !hasStats {
				t.Fatalf("%v: Expected ancestor stats to be available",
					test.name)
			}

			expectedFee := oldStat.Fees - txDesc.Fee
			expectedSigOps := oldStat.TotalSigOps - txDesc.TotalSigOps
			expectedSizeBytes := oldStat.SizeBytes -
				int64(txDesc.Tx.MsgTx().SerializeSize())
			expectedNumAncestors := oldStat.NumAncestors - 1

			if newStat.Fees != expectedFee {
				t.Fatalf("%v: expected descendant to have adjusted "+
					"Fees=%v, but got %v", test.name, expectedFee, newStat.Fees)
			}

			if newStat.TotalSigOps != expectedSigOps {
				t.Fatalf("%v: expected descendant to have adjusted "+
					"NumSigOps=%v, but got %v", test.name,
					expectedSigOps, newStat.TotalSigOps)
			}

			if newStat.SizeBytes != expectedSizeBytes {
				t.Fatalf("%v: expected descendant to have adjusted "+
					"SizeBytes=%v, but got %v", test.name,
					expectedSizeBytes, newStat.SizeBytes)
			}

			if newStat.NumAncestors != expectedNumAncestors {
				t.Fatalf("%v: expected descendant to have "+
					"NumAncestors=%v, but got %v", test.name,
					expectedNumAncestors, newStat.NumAncestors)
			}
		}

		miningView.reject(txHash)

		if !miningView.isRejected(txHash) {
			t.Fatalf("%v: expected test subject txn %v to be rejected",
				test.name, *txHash)
		}

		// Rejecting a transaction rejects all descendant transactions and removes the
		// tx from the graph.
		if ancestors := miningView.ancestors(txHash); len(ancestors) > 0 {
			t.Fatalf("%v: expected subject txn to have 0 ancestors, got %v",
				test.name, len(ancestors))
		}

		for _, descendant := range descendants {
			if !miningView.isRejected(descendant) {
				t.Fatalf("%v: expected descendant txn %v to be rejected.",
					test.name, *descendant)
			}
		}
	}

	// Get mining view snapshot prior to removing the tx from the tx source.
	miningViewSnapshot := harness.txSource.MiningView()

	// Remove txC and its descendants from the tx source.
	harness.RemoveTransactionFromTxSource(txC, true)

	// Add a new transaction to the tx source.
	_, err = harness.AddTransactionToTxSource(txF)
	if err != nil {
		t.Fatalf("unable to add transaction to the tx source: %v", err)
	}

	miningView := harness.txSource.MiningView()
	addedTxns := miningView.children(txD.Hash())
	if len(addedTxns) != 1 {
		t.Fatalf("AddTxnToTxSource: expected txD to have exactly 1 child, got %v",
			len(addedTxns))
	}

	if addedTxns[0].Tx != txF {
		t.Fatalf("AddTxnToTxSource: expected txF to exist in the mining view.")
	}

	// Ensure newly added transaction not added to the mining view snapshot.
	if len(miningViewSnapshot.children(txD.Hash())) != 0 {
		t.Fatalf("AddTxnToTxSource: expected txD to not have descendants in " +
			"snapshot mining view.")
	}

	if len(miningView.children(txC.Hash())) != 0 {
		t.Fatalf("RemoveTxnFromTxSource: expected txC to not have descendents " +
			"after being removed from the tx source.")
	}

	// Ensure tx and children are still accessible in cloned view.
	if len(miningViewSnapshot.children(txC.Hash())) != 1 {
		t.Fatalf("RemoveTxnFromTxSource: expected txC to exist in snapshot view " +
			"even though it was removed from the tx source.")
	}
}

// TestAncestorTrackingLimits ensures that ancestor tracking is disabled for
// transactions that have too many ancestors.
func TestAncestorTrackingLimits(t *testing.T) {
	harness, spendableOuts, err := newMiningHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("unable to create mining harness: %v", err)
	}

	// Create a chain of transactions.
	var allTxns []*dcrutil.Tx
	prevSpendableOut := spendableOuts[0]
	for i := 0; i < ancestorTrackingLimit+2; i++ {
		tx, _ := harness.CreateSignedTx([]spendableOutput{
			prevSpendableOut,
		}, 1)

		allTxns = append(allTxns, tx)
		prevSpendableOut = txOutToSpendableOut(tx, 0, wire.TxTreeRegular)
	}

	// Add all transactions to the tx source.
	for _, tx := range allTxns {
		_, err = harness.AddTransactionToTxSource(tx)
		if err != nil {
			t.Fatalf("unable to add transaction to the tx source: %v", err)
		}
	}

	// Remove each transaction from the tx source to demonstrate that once a
	// transaction has less ancestors than the limit that it begins having ancestor
	// stats tracked and it continues to do so until it is removed from the tx
	// source.
	allTxnsQueue := allTxns
	for len(allTxnsQueue) > 0 {
		for index, tx := range allTxnsQueue {
			_, hasStats := harness.txSource.miningView.AncestorStats(tx.Hash())
			if index <= ancestorTrackingLimit && !hasStats {
				t.Fatalf("expected transaction %v at index %v to have ancestor"+
					" tracking enabled", tx.Hash(), index)
			}

			if index > ancestorTrackingLimit && hasStats {
				t.Fatalf("expected transaction %v at index %v to not have "+
					"ancestor tracking enabled", tx.Hash(), index)
			}
		}

		tx := allTxnsQueue[0]
		allTxnsQueue = allTxnsQueue[1:]

		// Simulate the transaction being included in a block.
		newBlockHeight := harness.chain.bestState.Height + 1
		harness.AddFakeUTXO(tx, newBlockHeight, wire.NullBlockIndex,
			harness.chain.isTreasuryAgendaActive)
		harness.chain.bestState = blockchain.BestState{
			Height: newBlockHeight,
		}

		harness.RemoveTransactionFromTxSource(tx, false)
		harness.txSource.MaybeAcceptDependents(tx,
			harness.chain.isTreasuryAgendaActive)
	}

	// Add all transactions back to the tx source in reverse order to demonstrate
	// that a transaction that initially has ancestor stats tracked is later
	// excluded from the ancestor tracked set when it has too many ancestors.
	for index := len(allTxns) - 1; index >= 0; index-- {
		tx := allTxns[index]
		txHash := tx.Hash()
		harness.chain.utxos.LookupEntry(txHash).SpendOutput(0)
		_, err = harness.AddTransactionToTxSource(tx)
		if err != nil {
			t.Fatalf("unable to add transaction to the tx source: %v", err)
		}

		_, hasStats := harness.txSource.miningView.AncestorStats(txHash)
		if !hasStats {
			t.Fatalf("expected transaction %v at index %v to have ancestor"+
				" tracking enabled when added back to the tx source", txHash, index)
		}
	}

	for index, tx := range allTxns {
		txHash := tx.Hash()
		_, hasStats := harness.txSource.miningView.AncestorStats(txHash)
		if index <= ancestorTrackingLimit && !hasStats {
			t.Fatalf("expected transaction %v at index %v to have ancestor"+
				" tracking enabled", txHash, index)
		}

		if index > ancestorTrackingLimit && hasStats {
			t.Fatalf("expected transaction %v at index %v to not have "+
				"ancestor tracking enabled", txHash, index)
		}
	}
}
