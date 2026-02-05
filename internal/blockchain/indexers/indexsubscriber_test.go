// Copyright (c) 2021-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/v5/chaingen"
	"github.com/decred/dcrd/chaincfg/v3"
)

// TestIndexSubscriberAsync ensures the index subscriber
// behaves as expected sending notifications to its subscribers.
func TestIndexSubscriberAsync(t *testing.T) {
	db := setupDB(t)

	chain, err := newTestChain()
	if err != nil {
		t.Fatal(err)
	}

	g, err := chaingen.MakeGenerator(chaincfg.SimNetParams())
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	// Add three blocks to the chain.
	addBlock(t, chain, &g, "bk1")
	addBlock(t, chain, &g, "bk2")
	bk3 := addBlock(t, chain, &g, "bk3")

	// Initialize all indexes.
	ctx := t.Context()

	subber := NewIndexSubscriber(ctx)

	txIdx, err := NewTxIndex(subber, db, chain)
	if err != nil {
		t.Fatal(err)
	}

	existsAddrIdx, err := NewExistsAddrIndex(subber, db, chain)
	if err != nil {
		t.Fatal(err)
	}

	err = subber.CatchUp(ctx, db, chain)
	if err != nil {
		t.Fatal(err)
	}

	go subber.Run(ctx)

	// Ensure all indexes through their prerequisite/dependency relationships
	// are synced to the current chain tip (bk3).
	txIdxTipHeight, txIdxTipHash, err := txIdx.Tip()
	if err != nil {
		t.Fatal(err)
	}

	if txIdxTipHeight != bk3.Height() {
		t.Fatalf("expected tip height to be %d, got %d",
			bk3.Height(), txIdxTipHeight)
	}

	if *txIdxTipHash != *bk3.Hash() {
		t.Fatalf("expected tip hash to be %s, got %s", bk3.Hash(), txIdxTipHash)
	}

	existsAddrIdxTipHeight, existsAddrIdxTipHash, err := existsAddrIdx.Tip()
	if err != nil {
		t.Fatal(err)
	}

	if existsAddrIdxTipHeight != bk3.Height() {
		t.Fatalf("expected tip height to be %d, got %d",
			bk3.Height(), existsAddrIdxTipHeight)
	}

	if *existsAddrIdxTipHash != *bk3.Hash() {
		t.Fatalf("expected tip hash to be %s, got %s", bk3.Hash(),
			existsAddrIdxTipHash)
	}

	// Create 3 sync subscribers for the tx index.
	for idx := 0; idx < 3; idx++ {
		txIdx.WaitForSync()
	}

	// Wait for the sync subscriber handler to run.
	time.Sleep(time.Millisecond * 750)

	// Ensure the tx index no longer has sync subscribers.
	txIdx.mtx.Lock()
	if len(txIdx.subscribers) > 0 {
		txIdx.mtx.Unlock()
		t.Fatalf("expected no sync subscribers for the tx index")
	}
	txIdx.mtx.Unlock()

	// Ensure the address index remains in sync with the main chain when new
	// blocks are connected.
	bk4 := addBlock(t, chain, &g, "bk4")
	ntfn := &IndexNtfn{
		NtfnType:          ConnectNtfn,
		Block:             bk4,
		Parent:            bk3,
		IsTreasuryEnabled: false,
	}
	notifyAndWait(t, subber, ntfn)

	// Ensure all indexes synced to the newly added block (bk4).
	txIdxTipHeight, txIdxTipHash, err = txIdx.Tip()
	if err != nil {
		t.Fatal(err)
	}

	if txIdxTipHeight != bk4.Height() {
		t.Fatalf("expected tip height to be %d, got %d",
			bk4.Height(), txIdxTipHeight)
	}

	if *txIdxTipHash != *bk4.Hash() {
		t.Fatalf("expected tip hash to be %s, got %s", bk4.Hash(), txIdxTipHash)
	}

	existsAddrIdxTipHeight, existsAddrIdxTipHash, err = existsAddrIdx.Tip()
	if err != nil {
		t.Fatal(err)
	}

	if existsAddrIdxTipHeight != bk4.Height() {
		t.Fatalf("expected tip height to be %d, got %d",
			bk4.Height(), existsAddrIdxTipHeight)
	}

	if *existsAddrIdxTipHash != *bk4.Hash() {
		t.Fatalf("expected tip hash to be %s, got %s", bk4.Hash(),
			existsAddrIdxTipHash)
	}

	// Ensure stopping a prequisite subscription stops its dependency as well.
	subber.mtx.Lock()
	err = txIdx.sub.stop()
	subber.mtx.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	bk5 := addBlock(t, chain, &g, "bk5")
	ntfn = &IndexNtfn{
		NtfnType:          ConnectNtfn,
		Block:             bk5,
		Parent:            bk4,
		IsTreasuryEnabled: false,
	}
	notifyAndWait(t, subber, ntfn)

	// Ensure only the exists address index synced to the newly added block (bk5).
	txIdxTipHeight, txIdxTipHash, err = txIdx.Tip()
	if err != nil {
		t.Fatal(err)
	}

	if txIdxTipHeight != bk4.Height() {
		t.Fatalf("expected tip height to be %d, got %d",
			bk4.Height(), txIdxTipHeight)
	}

	if *txIdxTipHash != *bk4.Hash() {
		t.Fatalf("expected tip hash to be %s, got %s", bk4.Hash(), txIdxTipHash)
	}

	existsAddrIdxTipHeight, existsAddrIdxTipHash, err = existsAddrIdx.Tip()
	if err != nil {
		t.Fatal(err)
	}

	if existsAddrIdxTipHeight != bk5.Height() {
		t.Fatalf("expected tip height to be %d, got %d",
			bk5.Height(), existsAddrIdxTipHeight)
	}

	if *existsAddrIdxTipHash != *bk5.Hash() {
		t.Fatalf("expected tip hash to be %s, got %s", bk5.Hash(),
			existsAddrIdxTipHash)
	}
}
