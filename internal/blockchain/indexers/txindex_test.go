// Copyright (c) 2021-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/v5/chaingen"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/database/v3"
	_ "github.com/decred/dcrd/database/v3/ffldb"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/wire"
)

// testChain represents a mock implementation of a block chain as
// defined by the indexer.ChainQueryer interface.
type testChain struct {
	bestHeight       int64
	bestHash         *chainhash.Hash
	treasuryActive   bool
	keyedByHeight    map[int64]*dcrutil.Block
	keyedByHash      map[chainhash.Hash]*dcrutil.Block
	orphans          map[chainhash.Hash]*dcrutil.Block
	removedSpendDeps map[chainhash.Hash][]string
	mtx              sync.Mutex
}

// newTestChain initializes a test chain.
func newTestChain() (*testChain, error) {
	tc := &testChain{
		keyedByHeight:    make(map[int64]*dcrutil.Block),
		keyedByHash:      make(map[chainhash.Hash]*dcrutil.Block),
		orphans:          make(map[chainhash.Hash]*dcrutil.Block),
		removedSpendDeps: make(map[chainhash.Hash][]string),
	}
	genesis := dcrutil.NewBlock(chaincfg.SimNetParams().GenesisBlock)
	return tc, tc.AddBlock(genesis)
}

// AddBlock extends the chain with the provided block.
func (tc *testChain) AddBlock(blk *dcrutil.Block) error {
	tc.mtx.Lock()
	defer tc.mtx.Unlock()

	// Ensure the incoming block is the child of the current chain tip.
	if tc.bestHash != nil {
		if blk.MsgBlock().Header.PrevBlock != *tc.bestHash {
			return fmt.Errorf("block %s is an orphan", blk.Hash())
		}
	}

	height := blk.Height()
	hash := blk.Hash()
	tc.keyedByHash[*hash] = blk
	tc.keyedByHeight[height] = blk
	tc.bestHeight = height
	tc.bestHash = hash

	return nil
}

// Remove block disconnects the provided block as the tip
// of the chain. The provided block is required to be the
// current chain tip.
func (tc *testChain) RemoveBlock(blk *dcrutil.Block) error {
	tc.mtx.Lock()
	defer tc.mtx.Unlock()

	hash := blk.Hash()

	// Ensure the block being removed is the current chain tip.
	if *tc.bestHash != *hash {
		return fmt.Errorf("block %s is not the current chain tip", blk.Hash())
	}

	// Set the new chain tip.
	tc.bestHash = &blk.MsgBlock().Header.PrevBlock
	tc.bestHeight--

	height := blk.Height()
	delete(tc.keyedByHash, *hash)
	delete(tc.keyedByHeight, height)

	tc.orphans[*hash] = blk

	return nil
}

// MainChainHasBlock asserts if the provided block is part of the
// chain.
func (tc *testChain) MainChainHasBlock(hash *chainhash.Hash) bool {
	tc.mtx.Lock()
	defer tc.mtx.Unlock()

	_, ok := tc.keyedByHash[*hash]
	return ok
}

// Best returns the height and block hash of the the current
// chain tip.
func (tc *testChain) Best() (int64, *chainhash.Hash) {
	tc.mtx.Lock()
	defer tc.mtx.Unlock()

	return tc.bestHeight, tc.bestHash
}

// IsTreasuryAgendaActive returns whether or not the treasury agenda is active.
func (tc *testChain) IsTreasuryAgendaActive(hash *chainhash.Hash) (bool, error) {
	return tc.treasuryActive, nil
}

// BlockHeightByHash returns the height of the provided block hash if it is
// part of the chain.
func (tc *testChain) BlockHeightByHash(hash *chainhash.Hash) (int64, error) {
	blk, err := tc.BlockByHash(hash)
	if err != nil {
		return 0, err
	}

	return blk.Height(), nil
}

// Ancestor returns the ancestor block hash of the provided block at the
// provided height.
func (tc *testChain) Ancestor(block *chainhash.Hash, height int64) *chainhash.Hash {
	blk, err := tc.BlockByHash(block)
	if err != nil {
		log.Error(err)
		return nil
	}

	tipHeight := blk.Height()

	// if the provided height is greater than the chain's tip height,
	// the associated block cannot be an ancestor.
	if height > tipHeight {
		return nil
	}

	for {
		if blk.Height() == height {
			return blk.Hash()
		}

		if blk.Height() < height {
			return nil
		}

		prev := &blk.MsgBlock().Header.PrevBlock
		blk, err = tc.BlockByHash(prev)
		if err != nil {
			log.Error(err)
			return nil
		}
	}
}

// RemoveSpendConsumerDependency removes the provided spend consumer dependency
// associated with the provided block hash.
func (tc *testChain) RemoveSpendConsumerDependency(_ database.Tx, blockHash *chainhash.Hash, consumerID string) error {
	tc.mtx.Lock()
	defer tc.mtx.Unlock()

	removedDeps, ok := tc.removedSpendDeps[*blockHash]
	if !ok {
		tc.removedSpendDeps[*blockHash] = []string{consumerID}

		return nil
	}
	_ = append(removedDeps, consumerID)

	return nil
}

// IsRemovedSpendConsumerDependency returns whether the provided consumer has
// a spend journal dependency for the provided block hash.
func (tc *testChain) IsRemovedSpendConsumerDependency(blockHash *chainhash.Hash, consumerID string) bool {
	tc.mtx.Lock()
	defer tc.mtx.Unlock()

	ids, ok := tc.removedSpendDeps[*blockHash]
	if !ok {
		return false
	}

	for _, id := range ids {
		if id == consumerID {
			return true
		}
	}

	return false
}

// ChainParams returns the parameters of the chain.
func (tc *testChain) ChainParams() *chaincfg.Params {
	return chaincfg.SimNetParams()
}

// BlockHashByHeight returns the block hash of the block at the provided
// height.
func (tc *testChain) BlockHashByHeight(height int64) (*chainhash.Hash, error) {
	tc.mtx.Lock()
	defer tc.mtx.Unlock()

	blk := tc.keyedByHeight[height]
	if blk == nil {
		return nil, fmt.Errorf("no block found with height %d", height)
	}

	return blk.Hash(), nil
}

// BlockByHash returns the block associated with the provided block hash.
func (tc *testChain) BlockByHash(hash *chainhash.Hash) (*dcrutil.Block, error) {
	tc.mtx.Lock()
	defer tc.mtx.Unlock()

	blk := tc.keyedByHash[*hash]
	if blk == nil {
		blk = tc.orphans[*hash]
		if blk == nil {
			return nil, fmt.Errorf("no block found with hash %s", hash)
		}
	}

	return blk, nil
}

// BlockHeaderByHash returns the block header identified by the given hash.
func (tc *testChain) BlockHeaderByHash(hash *chainhash.Hash) (wire.BlockHeader, error) {
	tc.mtx.Lock()
	defer tc.mtx.Unlock()

	blk := tc.keyedByHash[*hash]
	if blk == nil {
		blk = tc.orphans[*hash]
		if blk == nil {
			return wire.BlockHeader{}, fmt.Errorf("no block found with hash %s",
				hash)
		}
	}

	return blk.MsgBlock().Header, nil
}

// PrevScripts returns a source of previous transaction scripts and their
// associated versions spent by the provided block.
func (tc *testChain) PrevScripts(database.Tx, *dcrutil.Block) (PrevScripter, error) {
	return nil, nil
}

// notifyAndWait sends the provided notification and waits for done signal
// with a one second timeout.
func notifyAndWait(t *testing.T, subber *IndexSubscriber, ntfn *IndexNtfn) {
	t.Helper()
	done := make(chan bool)
	ntfn.Done = done
	subber.Notify(ntfn)
	select {
	case <-done:
		// Nothing to do.
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for done signal for notification")
	}
}

// addBlock extends the provided chain with a generated block.
func addBlock(t *testing.T, chain *testChain, gen *chaingen.Generator, name string) *dcrutil.Block {
	firstBlock := gen.TipName() == "genesis"

	var msgBlk *wire.MsgBlock
	if firstBlock {
		msgBlk = gen.CreateBlockOne(name, 0)
	}

	if msgBlk == nil {
		msgBlk = gen.NextBlock(name, nil, nil)
		gen.SaveTipCoinbaseOuts()
	}

	blk := dcrutil.NewBlock(msgBlk)

	err := chain.AddBlock(blk)
	if err != nil {
		t.Fatal(err)
	}

	return blk
}

// setupDB initializes the test database.
func setupDB(t *testing.T) database.DB {
	dbPath := t.TempDir()

	db, err := database.Create("ffldb", dbPath, wire.SimNet)
	if err != nil {
		t.Fatalf("error creating db: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})

	return db
}

// TestTxIndexAsync ensures the tx index behaves as expected receiving
// async notifications.
func TestTxIndexAsync(t *testing.T) {
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

	// Initialize the tx index.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subber := NewIndexSubscriber(ctx)
	go subber.Run(ctx)

	idx, err := NewTxIndex(subber, db, chain)
	if err != nil {
		t.Fatal(err)
	}

	err = subber.CatchUp(ctx, db, chain)
	if err != nil {
		t.Fatal(err)
	}

	// Ensure the index got synced to bk3 on initialization.
	tipHeight, tipHash, err := idx.Tip()
	if err != nil {
		t.Fatal(err)
	}

	if tipHeight != bk3.Height() {
		t.Fatalf("expected tip height to be %d, got %d",
			bk3.Height(), tipHeight)
	}

	if *tipHash != *bk3.Hash() {
		t.Fatalf("expected tip hash to be %s, got %s", bk3.Hash(), tipHash)
	}

	// Ensure the index remains in sync with the main chain when new
	// blocks are connected.
	bk4 := addBlock(t, chain, &g, "bk4")
	ntfn := &IndexNtfn{
		NtfnType: ConnectNtfn,
		Block:    bk4,
		Parent:   bk3,
	}
	notifyAndWait(t, subber, ntfn)

	bk5 := addBlock(t, chain, &g, "bk5")
	ntfn = &IndexNtfn{
		NtfnType: ConnectNtfn,
		Block:    bk5,
		Parent:   bk4,
	}
	notifyAndWait(t, subber, ntfn)

	// Ensure the index got synced to bk5.
	tipHeight, tipHash, err = idx.Tip()
	if err != nil {
		t.Fatal(err)
	}

	if tipHeight != bk5.Height() {
		t.Fatalf("expected tip height to be %d, got %d",
			bk5.Height(), tipHeight)
	}

	if *tipHash != *bk5.Hash() {
		t.Fatalf("expected tip hash to be %s, got %s", bk5.Hash(), tipHash)
	}

	// Simulate a reorg by setting bk4 as the main chain tip. bk5 is now
	// an orphan block.
	g.SetTip("bk4")
	err = chain.RemoveBlock(bk5)
	if err != nil {
		t.Fatal(err)
	}

	// Add bk5a to the main chain.
	bk5a := addBlock(t, chain, &g, "bk5a")

	// Resubscribe the index.
	subber.mtx.Lock()
	err = idx.sub.stop()
	subber.mtx.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	idx, err = NewTxIndex(subber, db, chain)
	if err != nil {
		t.Fatal(err)
	}

	err = subber.CatchUp(ctx, db, chain)
	if err != nil {
		t.Fatal(err)
	}

	tipHeight, tipHash, err = idx.Tip()
	if err != nil {
		t.Fatal(err)
	}

	// Ensure the index recovered to bk4 and synced back to the main chain tip
	// bk5a.
	if tipHeight != bk5a.Height() {
		t.Fatalf("expected tip height to be %d, got %d",
			bk5a.Height(), tipHeight)
	}

	if *tipHash != *bk5a.Hash() {
		t.Fatalf("expected tip hash to be %s, got %s", bk5a.Hash(), tipHash)
	}

	// Ensure bk5 is no longer indexed.
	entry, err := idx.Entry(bk5.Hash())
	if err != nil {
		t.Fatal(err)
	}

	if entry != nil {
		t.Fatal("expected no index entry for bk5")
	}

	// Ensure the index remains in sync when blocks are disconnected.
	err = chain.RemoveBlock(bk5a)
	if err != nil {
		t.Fatal(err)
	}

	g.SetTip("bk4")

	ntfn = &IndexNtfn{
		NtfnType: DisconnectNtfn,
		Block:    bk5a,
		Parent:   bk4,
	}
	notifyAndWait(t, subber, ntfn)

	err = chain.RemoveBlock(bk4)
	if err != nil {
		t.Fatal(err)
	}

	g.SetTip("bk3")

	ntfn = &IndexNtfn{
		NtfnType: DisconnectNtfn,
		Block:    bk4,
		Parent:   bk3,
	}
	notifyAndWait(t, subber, ntfn)

	// Ensure the index tip is now bk3 after the disconnections.
	tipHeight, tipHash, err = idx.Tip()
	if err != nil {
		t.Fatal(err)
	}

	if tipHeight != bk3.Height() {
		t.Fatalf("expected tip height to be %d, got %d",
			bk3.Height(), tipHeight)
	}

	if *tipHash != *bk3.Hash() {
		t.Fatalf("expected tip hash to be %s, got %s", bk3.Hash(), tipHash)
	}

	// Drop the index.
	err = idx.DropIndex(ctx, idx.db)
	if err != nil {
		t.Fatal(err)
	}

	// Resubscribe the index.
	subber.mtx.Lock()
	err = idx.sub.stop()
	subber.mtx.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	idx, err = NewTxIndex(subber, db, chain)
	if err != nil {
		t.Fatal(err)
	}

	err = subber.CatchUp(ctx, db, chain)
	if err != nil {
		t.Fatal(err)
	}

	// Ensure the index got synced to bk3 on initialization.
	tipHeight, tipHash, err = idx.Tip()
	if err != nil {
		t.Fatal(err)
	}

	if tipHeight != bk3.Height() {
		t.Fatalf("expected tip height to be %d, got %d",
			bk3.Height(), tipHeight)
	}

	if *tipHash != *bk3.Hash() {
		t.Fatalf("expected tip hash to be %s, got %s", bk3.Hash(), tipHash)
	}

	// Add bk4a to the main chain.
	bk4a := addBlock(t, chain, &g, "bk4a")

	go func() {
		// Stall the index notification for bk4a.
		time.Sleep(time.Millisecond * 150)
		notif := &IndexNtfn{
			NtfnType: ConnectNtfn,
			Block:    bk4a,
			Parent:   bk3,
			Done:     make(chan bool),
		}
		subber.Notify(notif)
		select {
		case <-notif.Done:
			// Nothing to do.
		case <-time.After(time.Second):
			panic("timeout waiting for done signal for notification")
		}
	}()

	// Wait for the index to sync with the main chain before terminating.
	select {
	case <-idx.WaitForSync():
		// Nothing to do.
	case <-time.After(time.Second):
		panic("timeout waiting for index to synchronize")
	}

	// Add bk6 and bk7 to the main chain.
	bk6 := addBlock(t, chain, &g, "bk6")
	bk7 := addBlock(t, chain, &g, "bk7")

	// Ensure sending an unexpected index notification (bk7) does not
	// update the index.
	ntfn = &IndexNtfn{
		NtfnType: ConnectNtfn,
		Block:    bk7,
		Parent:   bk6,
	}
	notifyAndWait(t, subber, ntfn)

	// Ensure the address index remains at tip bk4a after receiving unexpected
	// index notification for bk7.
	tipHeight, tipHash, err = idx.Tip()
	if err != nil {
		t.Fatal(err)
	}

	if tipHeight != bk4a.Height() {
		t.Fatalf("expected tip height to be %d, got %d",
			bk4a.Height(), tipHeight)
	}

	if *tipHash != *bk4a.Hash() {
		t.Fatalf("expected tip hash to be %s, got %s", bk4a.Hash(), tipHash)
	}
}
