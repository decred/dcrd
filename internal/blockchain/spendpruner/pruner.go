// Copyright (c) 2021-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package spendpruner

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database/v3"
)

const (
	// batchSignalBufferSize is the buffer size for batch signals.
	batchSignalBufferSize = 32

	// batchThreshold is the prune batch size that triggers a batch prune.
	batchThreshold = 128

	// BatchPruneInterval is the time elapsed before a batch prune is triggered.
	BatchPruneInterval = time.Second * 10

	// DependencyPruneInterval is the time elapsed before a dependency prune
	// is triggered.
	DependencyPruneInterval = time.Second * 30

	// maxDependencyDifference is the maximum difference in height
	// between a spend dependency and the chain tip which makes the
	// dependency prunable.
	maxDependencyDifference = 288
)

// SpendJournalPruner represents a spend journal pruner that ensures spend
// journal entries needed by consumers are retained until no longer needed.
type SpendJournalPruner struct {
	// This field tracks the chain tip height based on block connections and
	// disconnections.
	currentTip uint32 // Update atomically.

	// This removes the spend journal entries of the provided block hashes if
	// they are not part of the main chain.
	batchRemoveSpendEntry func(hash []chainhash.Hash) error

	// These fields track spend consumers, their spend journal dependencies
	// and block heights of the spend entries.
	dependents      map[chainhash.Hash][]string
	dependentsMtx   sync.Mutex
	spendHeights    map[chainhash.Hash]uint32
	spendHeightsMtx sync.Mutex
	consumers       map[string]SpendConsumer
	consumersMtx    sync.Mutex

	// This field relays prune batch signals for processing.
	ch chan struct{}

	// These fields batches hashes associated with spend journal entries
	// scheduled to be pruned.
	pruneBatch    []chainhash.Hash
	pruneBatchMtx sync.Mutex

	// This is the maximum time between processing batched prunes.
	batchPruneInterval time.Duration

	// This is the maximum time between processing dependency prunes.
	dependencyPruneInterval time.Duration

	// This field provides access to the database.
	db database.DB

	// This field synchronizes channel sends and receives.
	quit chan struct{}
}

// NewSpendJournalPruner initializes a spend journal pruner.
func NewSpendJournalPruner(db database.DB, batchRemoveSpendEntry func(hash []chainhash.Hash) error, currentTip uint32, batchPruneInterval time.Duration, dependencyPruneInterval time.Duration) (*SpendJournalPruner, error) {
	err := initConsumerDependenciesBucket(db)
	if err != nil {
		return nil, err
	}

	err = initSpendJournalHeightsBucket(db)
	if err != nil {
		return nil, err
	}

	spendPruner := &SpendJournalPruner{
		db:                      db,
		currentTip:              currentTip,
		batchRemoveSpendEntry:   batchRemoveSpendEntry,
		batchPruneInterval:      batchPruneInterval,
		dependencyPruneInterval: dependencyPruneInterval,
		dependents:              make(map[chainhash.Hash][]string),
		spendHeights:            make(map[chainhash.Hash]uint32),
		consumers:               make(map[string]SpendConsumer),
		pruneBatch:              make([]chainhash.Hash, 0, batchThreshold),
		ch:                      make(chan struct{}, batchSignalBufferSize),
		quit:                    make(chan struct{}),
	}

	err = spendPruner.loadSpendConsumerDependencies()
	if err != nil {
		return nil, err
	}

	err = spendPruner.loadSpendJournalHeights()
	if err != nil {
		return nil, err
	}

	return spendPruner, nil
}

// AddConsumer adds a spend journal consumer to the pruner.
func (s *SpendJournalPruner) AddConsumer(consumer SpendConsumer) {
	s.consumersMtx.Lock()
	s.consumers[consumer.ID()] = consumer
	s.consumersMtx.Unlock()
}

// FetchConsumer returns the spend journal consumer associated with the
// provided id.
func (s *SpendJournalPruner) FetchConsumer(id string) (SpendConsumer, error) {
	s.consumersMtx.Lock()
	consumer, ok := s.consumers[id]
	s.consumersMtx.Unlock()
	if !ok {
		msg := fmt.Sprintf("no spend consumer found with id %s", id)
		return nil, pruneError(ErrNoConsumer, msg)
	}

	return consumer, nil
}

// DependencyExists determines whether there are spend consumer dependencies
// for the provided block hash.
func (s *SpendJournalPruner) DependencyExists(blockHash *chainhash.Hash) bool {
	s.dependentsMtx.Lock()
	_, ok := s.dependents[*blockHash]
	s.dependentsMtx.Unlock()
	return ok
}

// filterDependentPrunes filters spend dependencies eligible for pruning.
func (s *SpendJournalPruner) filterPrunableDependents() []chainhash.Hash {
	var toPrune []chainhash.Hash
	currentTip := atomic.LoadUint32(&s.currentTip)

	s.spendHeightsMtx.Lock()
	for blockHash, height := range s.spendHeights {
		if currentTip-height >= maxDependencyDifference {
			toPrune = append(toPrune, blockHash)
		}
	}
	s.spendHeightsMtx.Unlock()

	return toPrune
}

// pruneSpendDependencies purges spend journal dependency information of the
// provided set of block hashes.
func (s *SpendJournalPruner) pruneSpendDependencies(dependencies []chainhash.Hash) error {
	for _, hash := range dependencies {
		s.dependentsMtx.Lock()
		delete(s.dependents, hash)
		s.dependentsMtx.Unlock()

		s.spendHeightsMtx.Lock()
		delete(s.spendHeights, hash)
		s.spendHeightsMtx.Unlock()
	}

	err := s.db.Update(func(tx database.Tx) error {
		err := dbPruneSpendDependencies(tx, dependencies)
		if err != nil {
			return err
		}

		err = dbPruneSpendHeights(tx, dependencies)
		if err != nil {
			return err
		}

		return nil
	})
	return err
}

// ConnectBlock updates the spend pruner of the provided connected block hash.
func (s *SpendJournalPruner) ConnectBlock(blockHash *chainhash.Hash) error {
	if !s.DependencyExists(blockHash) {
		// Do nothing if there are no spend journal dependencies
		// for the the connected block.
		return nil
	}

	// Remove the key/value pair of persisted spend consumer dependencies and
	// the provided connected block hash from the prune set as well as the
	// spend height of the connected block.
	err := s.removeSpendConsumerDependencies(blockHash)
	if err != nil {
		return err
	}

	atomic.AddUint32(&s.currentTip, uint32(1))

	return nil
}

// DisconnectBlock signals the spend pruner of the provided
// disconnected block hash.
func (s *SpendJournalPruner) DisconnectBlock(blockHash *chainhash.Hash) {
	s.pruneBatchMtx.Lock()
	s.pruneBatch = append(s.pruneBatch, *blockHash)
	length := len(s.pruneBatch)
	s.pruneBatchMtx.Unlock()

	atomic.AddUint32(&s.currentTip, ^uint32(0))

	if length >= batchThreshold {
		select {
		case <-s.quit:
		case s.ch <- struct{}{}:
		default:
			// Non-blocking send fallthrough.
		}
	}
}

// dependencyExistsInternal determines whether a spend consumer depends on
// the spend data of the provided block hash.
func (s *SpendJournalPruner) dependencyExistsInternal(blockHash *chainhash.Hash, consumerID string) bool {
	s.dependentsMtx.Lock()
	dependents, ok := s.dependents[*blockHash]
	s.dependentsMtx.Unlock()
	if !ok {
		// The dependency does not exist if the block hash is not
		// a key for dependents.
		return false
	}

	for _, id := range dependents {
		if consumerID == id {
			return true
		}
	}

	return false
}

// spendHeightExists determines if there is a spend height entry for the
// provided block hash.
func (s *SpendJournalPruner) spendHeightExists(hash *chainhash.Hash) bool {
	s.spendHeightsMtx.Lock()
	_, ok := s.spendHeights[*hash]
	s.spendHeightsMtx.Unlock()

	return ok
}

// addSpendConsumerDependencies adds an entry for each spend consumer dependent
// on journal data for the provided block hash.
func (s *SpendJournalPruner) addSpendConsumerDependencies(blockHash *chainhash.Hash, blockHeight uint32) error {
	s.consumersMtx.Lock()
	defer s.consumersMtx.Unlock()

	for _, consumer := range s.consumers {
		if s.dependencyExistsInternal(blockHash, consumer.ID()) {
			// Dependency already created, skip.
			continue
		}

		needSpendData, err := consumer.NeedSpendData(blockHash)
		if err != nil {
			msg := fmt.Sprintf("unable to assert dependency: %s", err)
			return pruneError(ErrNeedSpendData, msg)
		}

		if !needSpendData {
			continue
		}

		// Add a spend dependency entry of the block hash for the consumer.
		s.dependentsMtx.Lock()
		dependents, ok := s.dependents[*blockHash]
		if !ok {
			dependents = []string{consumer.ID()}
			s.dependents[*blockHash] = dependents
			s.dependentsMtx.Unlock()

			continue
		}

		dependents = append(dependents, consumer.ID())
		s.dependents[*blockHash] = dependents
		s.dependentsMtx.Unlock()
	}

	s.dependentsMtx.Lock()
	dependents := s.dependents[*blockHash]
	s.dependentsMtx.Unlock()

	s.spendHeightsMtx.Lock()
	var spendHeightUpdate bool
	_, ok := s.spendHeights[*blockHash]
	if !ok {
		s.spendHeights[*blockHash] = blockHeight
		spendHeightUpdate = true
	}
	s.spendHeightsMtx.Unlock()

	// Update the persisted spend consumer deps entry for the provided block
	// hash as well as the spend heights map if it was updated.
	err := s.db.Update(func(tx database.Tx) error {
		err := dbUpdateSpendConsumerDependencies(tx, *blockHash, dependents)
		if err != nil {
			return err
		}

		if spendHeightUpdate {
			heightUpdate := map[chainhash.Hash]uint32{*blockHash: blockHeight}
			err := dbPersistSpendHeights(tx, heightUpdate)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		msg := fmt.Sprintf("unable to update persisted consumer "+
			"dependencies for block hash %v: %v", blockHash, err)
		return pruneError(ErrUpdateConsumerDependencies, msg)
	}

	return nil
}

// RemoveSpendConsumerDependency removes the provided spend consumer dependency
// associated with the provided block hash from the spend pruner. The block
// hash is removed as a key of the dependents map once all its dependency
// entries are removed.
func (s *SpendJournalPruner) RemoveSpendConsumerDependency(dbTx database.Tx, blockHash *chainhash.Hash, consumerID string) error {
	s.dependentsMtx.Lock()
	dependents, ok := s.dependents[*blockHash]
	if !ok {
		s.dependentsMtx.Unlock()
		// No entry for block hash found, do nothing.
		return nil
	}

	for idx := 0; idx < len(dependents); idx++ {
		if dependents[idx] == consumerID {
			dependents = append(dependents[:idx], dependents[idx+1:]...)
			s.dependents[*blockHash] = dependents
			break
		}
	}
	s.dependentsMtx.Unlock()

	if len(dependents) == 0 {
		s.dependentsMtx.Lock()
		delete(s.dependents, *blockHash)
		s.dependentsMtx.Unlock()

		s.DisconnectBlock(blockHash)

		// Remove the associated spend height of the block hash since
		// it has no dependencies now.
		if s.spendHeightExists(blockHash) {
			s.spendHeightsMtx.Lock()
			delete(s.spendHeights, *blockHash)
			s.spendHeightsMtx.Unlock()

			dbRemoveSpendHeight(dbTx, *blockHash)
		}
	}

	// Update the tracked spend journal entry for the provided
	// block hash.
	err := dbUpdateSpendConsumerDependencies(dbTx, *blockHash, dependents)
	if err != nil {
		msg := fmt.Sprintf("unable to update consumer dependencies "+
			"entry for block hash %v: %v", blockHash, err)
		return pruneError(ErrUpdateConsumerDependencies, msg)
	}

	return nil
}

// removeSpendConsumerDependencies removes the key/value pair of spend consumer
// dependencies and the provided block hash from the the prune set as well
// as the database. The associated spend height of the block is also removed.
func (s *SpendJournalPruner) removeSpendConsumerDependencies(blockHash *chainhash.Hash) error {
	s.dependentsMtx.Lock()
	delete(s.dependents, *blockHash)
	s.dependentsMtx.Unlock()

	s.spendHeightsMtx.Lock()
	delete(s.spendHeights, *blockHash)
	s.spendHeightsMtx.Unlock()

	// Remove the tracked spend journal entry for the provided
	// block hash.
	return s.db.Update(func(tx database.Tx) error {
		err := dbUpdateSpendConsumerDependencies(tx, *blockHash, nil)
		if err != nil {
			msg := fmt.Sprintf("unable to remove persisted consumer "+
				"dependencies entry for block hash %v: %v", blockHash, err)
			return pruneError(ErrUpdateConsumerDependencies, msg)
		}

		err = dbRemoveSpendHeight(tx, *blockHash)
		if err != nil {
			msg := fmt.Sprintf("unable to remove persisted spend journal "+
				"height entry for block hash %v: %v", blockHash, err)
			return pruneError(ErrRemoveSpendHeight, msg)
		}

		return nil
	})
}

// loadSpendConsumerDependencies loads persisted consumer spend dependencies
// from the database.
func (s *SpendJournalPruner) loadSpendConsumerDependencies() error {
	return s.db.View(func(tx database.Tx) error {
		consumerDeps, err := dbFetchSpendConsumerDependencies(tx)
		if err != nil {
			msg := fmt.Sprintf("unable to load spend consumer "+
				"dependencies: %v", err)
			return pruneError(ErrLoadSpendDependencies, msg)
		}

		s.dependentsMtx.Lock()
		s.dependents = consumerDeps
		s.dependentsMtx.Unlock()

		return nil
	})
}

// loadSpendJournalHeights loads persisted spend journal heights
// from the database.
func (s *SpendJournalPruner) loadSpendJournalHeights() error {
	return s.db.View(func(tx database.Tx) error {
		spendHeights, err := dbFetchSpendHeights(tx)
		if err != nil {
			msg := fmt.Sprintf("unable to load spend journal "+
				"heights: %v", err)
			return pruneError(ErrLoadSpendHeights, msg)
		}

		s.spendHeightsMtx.Lock()
		s.spendHeights = spendHeights
		s.spendHeightsMtx.Unlock()

		return nil
	})
}

// MaybePruneSpendData first adds consumer spend dependencies for the provided
// blockhash if any. If there are no dependencies the spend journal entry
// associated with the provided block hash is pruned.
func (s *SpendJournalPruner) MaybePruneSpendData(blockHash *chainhash.Hash) error {
	blockHeight := atomic.LoadUint32(&s.currentTip)
	err := s.addSpendConsumerDependencies(blockHash, blockHeight)
	if err != nil {
		return err
	}

	if s.DependencyExists(blockHash) {
		// Do nothing if there are spend dependencies for the provided block
		// hash.
		return nil
	}

	s.DisconnectBlock(blockHash)

	return nil
}

// handleBatchPrunes purges batched spend journal data.
//
// This should be run as a goroutine.
func (s *SpendJournalPruner) handleBatchPrunes(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case <-s.ch:
			s.pruneBatchMtx.Lock()
			batch := make([]chainhash.Hash, len(s.pruneBatch))
			copy(batch, s.pruneBatch)
			s.pruneBatch = s.pruneBatch[:0]
			s.pruneBatchMtx.Unlock()

			err := s.batchRemoveSpendEntry(batch)
			if err != nil {
				log.Errorf("unable to batch remove spend entries: %v", err)
			}
		}
	}
}

// handleTicks processes ticker signals of the spend pruner.
//
// This should be run as a goroutine.
func (s *SpendJournalPruner) handleTicks(ctx context.Context) {
	batchTicker := time.NewTicker(s.batchPruneInterval)
	dependencyTicker := time.NewTicker(s.dependencyPruneInterval)
	for {
		select {
		case <-ctx.Done():
			close(s.quit)
			batchTicker.Stop()
			dependencyTicker.Stop()
			return

		case <-batchTicker.C:
			s.pruneBatchMtx.Lock()
			length := len(s.pruneBatch)
			s.pruneBatchMtx.Unlock()

			if length >= 0 {
				select {
				case <-s.quit:
				case s.ch <- struct{}{}:
				default:
					// Non-blocking send fallthrough.
				}
			}

		case <-dependencyTicker.C:
			toPrune := s.filterPrunableDependents()
			err := s.pruneSpendDependencies(toPrune)
			if err != nil {
				log.Error("unable to prune spend dependencies: %w", err)
			}
		}
	}
}

// Run handles the lifecycle process of the spend journal pruner.
func (s *SpendJournalPruner) Run(ctx context.Context) {
	go s.handleTicks(ctx)
	go s.handleBatchPrunes(ctx)
}
