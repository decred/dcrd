// Copyright (c) 2021-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package spendpruner

import (
	"context"
	"fmt"
	"sync"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database/v3"
)

// spendPrunerEventType represents a spend pruner event message.
type spendPrunerEventType int

const (
	// spBlockConnected indicates a new block was connected to the chain.
	spBlockConnected spendPrunerEventType = iota

	// spBlockDisconnected indicates there are no spend dependencies for the
	// block disconnected from the chain.
	spBlockDisconnected
)

// SpendJournalNotification represents the notification received by the pruner
// on block connections and disconnections.
type SpendJournalNotification struct {
	BlockHash *chainhash.Hash
	Event     spendPrunerEventType
	Done      chan bool
}

// SpendJournalPruner represents a spend journal pruner that ensures spend
// journal entries needed by consumers are retained until no longer needed.
type SpendJournalPruner struct {
	// This removes the spend journal entry of the provided block hash if it
	// is not part of the main chain.
	removeSpendEntry func(hash *chainhash.Hash) error

	// These fields track spend consumers and their spend journal dependencies.
	dependents    map[chainhash.Hash][]string
	dependentsMtx sync.RWMutex
	consumers     map[string]SpendConsumer
	consumersMtx  sync.RWMutex

	// This field relays block connection and disconnection signals for
	// processing.
	ch chan SpendJournalNotification

	// This field provides access to the database.
	db database.DB
}

// NewSpendJournalPruner initializes a spend journal pruner.
	err := initConsumerDependenciesBucket(db)
	if err != nil {
		return nil, err
	}

	spendPruner := &SpendJournalPruner{
		db:               db,
		removeSpendEntry: removeSpendEntry,
		dependents:       make(map[chainhash.Hash][]string),
		consumers:        make(map[string]SpendConsumer),
		ch:               make(chan SpendJournalNotification),
	}

	err = spendPruner.loadSpendConsumerDependencies()
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
	s.consumersMtx.RLock()
	defer s.consumersMtx.RUnlock()
	consumer, ok := s.consumers[id]
	if !ok {
		msg := fmt.Sprintf("no spend consumer found with id %s", id)
		return nil, pruneError(ErrNoConsumer, msg)
	}

	return consumer, nil
}

// DependencyExists determines whether there are spend consumer dependencies
// for the provided block hash.
func (s *SpendJournalPruner) DependencyExists(blockHash *chainhash.Hash) bool {
	s.dependentsMtx.RLock()
	defer s.dependentsMtx.RUnlock()

	_, ok := s.dependents[*blockHash]
	return ok
}

// NotifyConnectedBlock signals the spend pruner of the provided
// connected block hash.
func (s *SpendJournalPruner) NotifyConnectedBlock(blockHash *chainhash.Hash) {
	s.ch <- SpendJournalNotification{
		BlockHash: blockHash,
		Event:     spBlockConnected,
	}
}

// dependencyExistsInternal determines whether a spend consumer depends on
// the spend data of the provided block hash.
func (s *SpendJournalPruner) dependencyExistsInternal(blockHash *chainhash.Hash, consumerID string) bool {
	s.dependentsMtx.RLock()
	dependents, ok := s.dependents[*blockHash]
	s.dependentsMtx.RUnlock()
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

// addSpendConsumerDependencies adds an entry for each spend consumer dependent
// on journal data for the provided block hash.
func (s *SpendJournalPruner) addSpendConsumerDependencies(blockHash *chainhash.Hash, blockHeight uint32) error {

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

	// Update the persisted spend consumer deps entry for
	// the provided block hash.
	err := s.db.Update(func(tx database.Tx) error {
		err := dbUpdateSpendConsumerDeps(tx, *blockHash, dependents)
		return err
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
		go func() {
			s.ch <- SpendJournalNotification{
				BlockHash: blockHash,
				Event:     spBlockDisconnected,
			}
		}()
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
func (s *SpendJournalPruner) removeSpendConsumerDependencies(blockHash *chainhash.Hash) error {
	s.dependentsMtx.Lock()
	delete(s.dependents, *blockHash)
	s.dependentsMtx.Unlock()

	// Remove the tracked spend journal entry for the provided
	// block hash.
		err := dbUpdateSpendConsumerDependencies(tx, *blockHash, nil)
		msg := fmt.Sprintf("unable to remove persisted consumer dependencies "+
			"entry for block hash %v: %v", blockHash, err)
		return pruneError(ErrUpdateConsumerDeps, msg)
	}

	return nil
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
		for k, v := range consumerDeps {
			s.dependents[k] = v
		}
		s.dependentsMtx.Unlock()

		return nil
	})
}

// HandleSignals processes incoming signals to the spend pruner.
func (s *SpendJournalPruner) HandleSignals(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case ntfn := <-s.ch:
			signalDone := func() {
				if ntfn.Done != nil {
					close(ntfn.Done)
				}
			}

			switch ntfn.Event {
			case spBlockDisconnected:
				err := s.removeSpendEntry(ntfn.BlockHash)
				if err != nil {
					log.Errorf("unable to prune spend data for "+
						"block hash (%s): %v", ntfn.BlockHash, err)

				}

				signalDone()

			case spBlockConnected:
				if !s.DependencyExists(ntfn.BlockHash) {
					// Do nothing if there are no spend journal dependencies
					// for the the connected block.
					signalDone()
					continue
				}

				// Remove the key/value pair of persisted spend consumer
				// dependencies and the provided connected block hash from
				// the prune set.
				err := s.removeSpendConsumerDeps(ntfn.BlockHash)
				if err != nil {
					log.Error(err)
				}

				signalDone()

			default:
				log.Errorf("unknown spend journal notification type: %d",
					ntfn.Event)

				signalDone()
			}
		}
	}
}

// MaybePruneSpendData firsts adds consumer spend dependencies for the provided
// blockhash if any. If there are no dependencies the spend journal entry
// associated with the provided block hash is pruned.
func (s *SpendJournalPruner) MaybePruneSpendData(blockHash *chainhash.Hash, done chan bool) error {
	err := s.addSpendConsumerDeps(blockHash)
	if err != nil {
		return err
	}

	if s.DependencyExists(blockHash) {
		// Do nothing if there are spend dependencies for the provided block
		// hash.
		return nil
	}

	go func() {
		s.ch <- SpendJournalNotification{
			BlockHash: blockHash,
			Event:     spBlockDisconnected,
			Done:      done,
		}
	}()

	return nil
}
