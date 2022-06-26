// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package spendpruner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database/v3"
)

// testSpendConsumer represents a mock spend consumer for testing purposes only.
type testSpendConsumer struct {
	id               string
	queryer          *testChain
	tip              *chainhash.Hash
	needSpendData    bool
	needSpendDataErr error
}

// ID returns the identifier of the consumer.
func (t *testSpendConsumer) ID() string {
	return t.id
}

// NeedSpendData checks whether the associated spend journal entry
// for the provided block hash will be needed by the consumer.
func (t *testSpendConsumer) NeedSpendData(hash *chainhash.Hash) (bool, error) {
	return t.needSpendData, t.needSpendDataErr
}

// testChain represents a mock chain for testing purposes only.
type testChain struct {
	removedSpendEntries map[chainhash.Hash]struct{}
	removeSpendEntryErr error
	mtx                 sync.Mutex
}

// BatchRemoveSpendEntry purges the spend journal entries of the
// provided batched block hashes if they are not part of the main chain.
func (t *testChain) BatchRemoveSpendEntry(batch []chainhash.Hash) error {
	if t.removeSpendEntryErr != nil {
		return t.removeSpendEntryErr
	}

	t.mtx.Lock()
	for _, entry := range batch {
		t.removedSpendEntries[entry] = struct{}{}
	}
	t.mtx.Unlock()

	return nil
}

// TestSpendPruner ensures the spend pruner works as intended.
func TestSpendPruner(t *testing.T) {
	chain := &testChain{
		removedSpendEntries: make(map[chainhash.Hash]struct{}),
	}

	consumerA := &testSpendConsumer{
		id:            "consumer_a",
		queryer:       chain,
		tip:           &chainhash.Hash{0},
		needSpendData: true,
	}

	consumerB := &testSpendConsumer{
		id:               "consumer_b",
		queryer:          chain,
		tip:              &chainhash.Hash{0},
		needSpendData:    false,
		needSpendDataErr: fmt.Errorf("unable to confirm spend data need"),
	}

	db, teardown, err := createDB(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	defer teardown()

	height := int64(0)
	blockHeightByHash := func(hash *chainhash.Hash) (int64, error) {
		height++
		return height, nil
	}

	batchPruneInterval := time.Millisecond * 100
	dependentPruneInterval := time.Millisecond * 100
	tipHeight := uint32(1)
	ctx, cancel := context.WithCancel(context.Background())
	cfg := &SpendJournalPrunerConfig{
		DB:                      db,
		BatchRemoveSpendEntry:   chain.BatchRemoveSpendEntry,
		BatchPruneInterval:      batchPruneInterval,
		DependencyPruneInterval: dependentPruneInterval,
		BlockHeightByHash:       blockHeightByHash,
	}
	pruner, err := NewSpendJournalPruner(cfg, tipHeight)
	if err != nil {
		t.Fatal(err)
	}

	go pruner.Run(ctx)

	pruner.AddConsumer(consumerA)
	pruner.AddConsumer(consumerB)

	// Ensure adding the same consumer does not create a duplicate.
	pruner.AddConsumer(consumerB)

	pruner.consumersMtx.Lock()
	consumers := len(pruner.consumers)
	pruner.consumersMtx.Unlock()

	if consumers != 2 {
		t.Fatalf("expected 2 consumers, got %d", consumers)
	}

	// Ensure adding dependents fails if a consumer errors checking whether the
	// data is needed.
	hashA := &chainhash.Hash{'a'}
	err = pruner.addSpendConsumerDependencies(hashA, 0)
	if !errors.Is(err, ErrNeedSpendData) {
		t.Fatalf("expected a spend data error, got %v", err)
	}

	fetchHashSpendDependents := func(pruner *SpendJournalPruner, hash *chainhash.Hash) ([]string, bool) {
		pruner.dependentsMtx.Lock()
		dependents, ok := pruner.dependents[*hash]
		pruner.dependentsMtx.Unlock()

		return dependents, ok
	}

	isRemovedSpendEntry := func(chain *testChain, hash *chainhash.Hash) bool {
		chain.mtx.Lock()
		_, ok := chain.removedSpendEntries[*hash]
		chain.mtx.Unlock()

		return ok
	}

	inPruneBatch := func(pruner *SpendJournalPruner, hash *chainhash.Hash) bool {
		pruner.pruneBatchMtx.Lock()
		for _, batchedHash := range pruner.pruneBatch {
			if batchedHash.IsEqual(hash) {
				pruner.pruneBatchMtx.Unlock()
				return true
			}
		}
		pruner.pruneBatchMtx.Unlock()
		return false
	}

	consumerB.needSpendDataErr = nil

	// Ensure adding dependents creates entries for consumers that need
	// the spend data of the provided block hash.
	err = pruner.addSpendConsumerDependencies(hashA, 0)
	if err != nil {
		t.Fatalf("unexpected error adding consumer dependencies: %v", err)
	}

	if !pruner.spendHeightExists(hashA) {
		t.Fatalf("expected a spend journal height entry for %s", hashA)
	}

	// Ensure there is only one dependent entry for hashA, belonging to
	// consumerA.
	dependents, ok := fetchHashSpendDependents(pruner, hashA)
	if !ok {
		t.Fatalf("expected dependent entry for hashA: %s", hashA)
	}

	if len(dependents) != 1 {
		t.Fatalf("expected one dependent entry for hashA, got %d",
			len(dependents))
	}

	if dependents[0] != consumerA.ID() {
		t.Fatalf("expected hashA's dependent to be %s, got %s",
			consumerA.ID(), dependents[0])
	}

	// Ensure there is no existing dependency for consumerB on hashA.
	exists := pruner.dependencyExistsInternal(hashA, consumerB.ID())
	if exists {
		t.Fatal("unexpected dependency found for consumerB on hashA")
	}

	// Ensure there is an existing dependency for consumerA on hashA.
	exists = pruner.dependencyExistsInternal(hashA, consumerA.ID())
	if !exists {
		t.Fatal("expected dependency found for consumerA on hashA")
	}

	// Ensure consumerC and consumerA now have dependencies on hashA.
	consumerC := &testSpendConsumer{
		id:            "consumer_c",
		tip:           &chainhash.Hash{0},
		needSpendData: true,
	}

	pruner.AddConsumer(consumerC)

	setConsumerNeedSpendData := func(state bool) {
		consumerA.needSpendData = state
		consumerB.needSpendData = state
		consumerC.needSpendData = state
	}

	removeSpendConsumerDep := func(pruner *SpendJournalPruner, blockHash *chainhash.Hash, consumerID string) error {
		return pruner.cfg.DB.Update(func(tx database.Tx) error {
			return pruner.RemoveSpendConsumerDependency(tx, blockHash, consumerID)
		})
	}

	err = pruner.addSpendConsumerDependencies(hashA, 0)
	if err != nil {
		t.Fatalf("unexpected error adding spend data dependents "+
			"for hashA: %v", err)
	}

	exists = pruner.DependencyExists(hashA)
	if !exists {
		t.Fatal("expected existing dependencies for hashA")
	}

	exists = pruner.dependencyExistsInternal(hashA, consumerC.ID())
	if !exists {
		t.Fatal("expected dependency found for consumerC on hashA")
	}

	exists = pruner.dependencyExistsInternal(hashA, consumerA.ID())
	if !exists {
		t.Fatal("expected dependency found for consumerA on hashA")
	}

	// Ensure there are now two dependent entries for hashA.
	dependents, _ = fetchHashSpendDependents(pruner, hashA)
	if len(dependents) != 2 {
		t.Fatalf("expected two dependent entry for hashA, got %d",
			len(dependents))
	}

	// Ensure removing a non-existent dependency does nothing.
	err = removeSpendConsumerDep(pruner, hashA, "consumer_x")
	if err != nil {
		t.Fatalf("unexpected error removing non-existent spend "+
			"consumer dependency %v", err)
	}

	// Ensure removing dependencies for a non-existent block hash does nothing.
	hashT := &chainhash.Hash{'T'}
	err = removeSpendConsumerDep(pruner, hashT, "consumer_x")
	if err != nil {
		t.Fatalf("unexpected error removing spend "+
			"consumer dependency for non-existent block hash %v", err)
	}

	dependents, _ = fetchHashSpendDependents(pruner, hashA)
	if len(dependents) != 2 {
		t.Fatalf("expected two dependent entry for hashA, got %d",
			len(dependents))
	}

	// Update spend consumers to not need spend data for upcoming tests.
	setConsumerNeedSpendData(false)

	// Ensure the spend pruner does not remove the spend entries if
	// there are existing dependencies for it.
	err = pruner.MaybePruneSpendData(hashA)
	if err != nil {
		t.Fatalf("[MaybePruneSpendData] unexpected error: %v", err)
	}

	ok = isRemovedSpendEntry(chain, hashA)
	if ok {
		t.Fatal("unexpected hashA spend data pruned")
	}

	if !pruner.spendHeightExists(hashA) {
		t.Fatalf("expected a spend journal height entry for %s", hashA)
	}

	// Ensure the spend pruner does remove spend entries for
	// a hash if there are no existing spend dependencies for it.
	hashX := &chainhash.Hash{'x'}
	err = pruner.MaybePruneSpendData(hashX)
	if err != nil {
		t.Fatalf("[MaybePruneSpendData] unexpected error: %v", err)
	}

	ok = inPruneBatch(pruner, hashX)
	if !ok {
		t.Fatalf("expected hashX spend notification to be batched")
	}

	time.Sleep(batchPruneInterval + (batchPruneInterval / 4))

	ok = isRemovedSpendEntry(chain, hashX)
	if !ok {
		t.Fatalf("expected hashX spend entry to be pruned")
	}

	// Remove the HashA dependency for consumerC.
	removeSpendConsumerDep(pruner, hashA, consumerC.ID())

	dependents, _ = fetchHashSpendDependents(pruner, hashA)
	if len(dependents) != 1 {
		t.Fatalf("expected no dependent entry for hashA, got %d",
			len(dependents))
	}

	// Trigger a spend journal prune for HashA's entry by removing the
	// last dependency for it.
	removeSpendConsumerDep(pruner, hashA, consumerA.ID())

	// Ensure the pruner no longer has a dependent entry for HashA.
	_, ok = pruner.dependents[*hashA]
	if ok {
		t.Fatal("expected no dependent entry for hashA")
	}

	ok = inPruneBatch(pruner, hashA)
	if !ok {
		t.Fatalf("expected hashA spend notification to be batched")
	}

	time.Sleep(batchPruneInterval + (batchPruneInterval / 4))

	ok = isRemovedSpendEntry(chain, hashA)
	if !ok {
		t.Fatalf("expected hashA spend entry to be pruned")
	}

	// Ensure the pruner no longer has a spend height entry for HashA.
	if pruner.spendHeightExists(hashA) {
		t.Fatalf("expected no spend journal height entry for %s", hashA)
	}

	// Update spend consumers to need spend data for upcoming tests.
	setConsumerNeedSpendData(true)

	// Create consumer dependencies for hashB and hashC.
	hashB := &chainhash.Hash{'b'}
	err = pruner.addSpendConsumerDependencies(hashB, 1)
	if err != nil {
		t.Fatal("unexpected error adding consumer dependencies "+
			"for hashB: %w", err)
	}

	dependents, _ = fetchHashSpendDependents(pruner, hashB)
	if len(dependents) != 3 {
		t.Fatalf("expected 3 dependent entries for hashB, got %d",
			len(dependents))
	}

	if !pruner.DependencyExists(hashB) {
		t.Fatal("expected consumer dependencies for hashB")
	}

	hashC := &chainhash.Hash{'c'}
	err = pruner.addSpendConsumerDependencies(hashC, 2)
	if err != nil {
		t.Fatalf("unexpected error adding consumer dependencies "+
			"for hashC: %v", err)
	}

	if !pruner.DependencyExists(hashC) {
		t.Fatal("expected consumer dependencies for hashC")
	}

	// Trigger a consumer dependencies purge by signalling hashB as a
	// connected block.
	pruner.ConnectBlock(hashB)

	if pruner.DependencyExists(hashB) {
		t.Fatal("expected no consumer dependencies entry for hashB")
	}

	// Ensure the pruner no longer has a spend height entry for HashB.
	if pruner.spendHeightExists(hashB) {
		t.Fatalf("expected no spend journal height entry for %s", hashB)
	}

	// Ensure the spend pruner does nothing if the provided connected block
	// does not have any spend consumer dependencies.
	pruner.ConnectBlock(hashT)

	// Ensure there are consumer dependencies and a spend height for hashC
	// before shutting down.
	if !pruner.DependencyExists(hashC) {
		t.Fatal("expected consumer dependencies for hashC")
	}

	if !pruner.spendHeightExists(hashC) {
		t.Fatalf("expected a spend journal height entry for hashC")
	}

	pruner.dependentsMtx.Lock()
	expected := len(pruner.dependents[*hashC])
	pruner.dependentsMtx.Unlock()

	cancel()

	// Load the spend pruner from the database.
	pruner, err = NewSpendJournalPruner(cfg, tipHeight)
	if err != nil {
		t.Fatal(err)
	}

	// Ensure the consumer dependencies for hashC were loaded from
	// the database.
	if !pruner.DependencyExists(hashC) {
		t.Fatal("expected consumer dependencies for hashC")
	}

	if !pruner.spendHeightExists(hashC) {
		t.Fatalf("expected a spend journal height entry for hashC")
	}

	// Ensure the expected number of dependents for hashC are identical
	// after loading from the database.
	deps, _ := fetchHashSpendDependents(pruner, hashC)

	if len(deps) != expected {
		t.Fatalf("dependencies mismatch, expected count of %d, got %d",
			expected, len(deps))
	}
}

func TestGenerateDependencySpendHeights(t *testing.T) {
	db, teardown, err := createDB(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	defer teardown()

	height := int64(0)
	blockHeightByHash := func(hash *chainhash.Hash) (int64, error) {
		height++
		return height, nil
	}

	batchPruneInterval := time.Millisecond * 100
	dependentPruneInterval := time.Millisecond * 100
	tipHeight := uint32(1)
	cfg := &SpendJournalPrunerConfig{
		DB: db,
		BatchRemoveSpendEntry: func(hash []chainhash.Hash) error {
			return nil
		},
		BatchPruneInterval:      batchPruneInterval,
		DependencyPruneInterval: dependentPruneInterval,
		BlockHeightByHash:       blockHeightByHash,
	}
	pruner := &SpendJournalPruner{
		cfg:          cfg,
		currentTip:   tipHeight,
		dependents:   make(map[chainhash.Hash][]string),
		spendHeights: make(map[chainhash.Hash]uint32),
		consumers:    make(map[string]SpendConsumer),
		pruneBatch:   make([]chainhash.Hash, 0, batchThreshold),
		ch:           make(chan struct{}, batchSignalBufferSize),
		quit:         make(chan struct{}),
	}

	hashA := chainhash.Hash{'a'}
	hashB := chainhash.Hash{'b'}
	hashC := chainhash.Hash{'c'}

	pruner.dependentsMtx.Lock()
	pruner.dependents[hashA] = []string{}
	pruner.dependents[hashB] = []string{}
	pruner.dependents[hashC] = []string{}
	pruner.dependentsMtx.Unlock()

	pruner.spendHeightsMtx.Lock()
	pruner.spendHeights[hashA] = 10
	pruner.spendHeights[hashC] = 20
	pruner.spendHeightsMtx.Unlock()

	pruner.generateDependencySpendHeights()

	pruner.spendHeightsMtx.Lock()
	spendHeightsLen := len(pruner.spendHeights)
	spendHeight, ok := pruner.spendHeights[hashB]
	pruner.spendHeightsMtx.Unlock()

	// Ensure the spend heights set is now 3.
	if spendHeightsLen != 3 {
		t.Fatalf("expected 3 spend height entries. got %d", spendHeightsLen)
	}

	// Ensure the height associated with hashA is 1.
	if !ok {
		t.Fatalf("expected hashA to have a spend height entry")
	}

	if spendHeight != 1 {
		t.Fatalf("expected associated spend height for hashA to "+
			"be 1, got %d", height)
	}
}
