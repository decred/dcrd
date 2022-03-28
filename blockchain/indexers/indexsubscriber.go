// Copyright (c) 2021-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/blockchain/v5/internal/progresslog"
	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/dcrutil/v4"
)

// IndexNtfnType represents an index notification type.
type IndexNtfnType int

const (
	// ConnectNtfn indicates the index notification signals a block
	// connected to the main chain.
	ConnectNtfn IndexNtfnType = iota

	// DisconnectNtfn indicates the index notification signals a block
	// disconnected from the main chain.
	DisconnectNtfn
)

var (
	// bufferSize represents the index notification buffer size.
	bufferSize = 128

	// noPrereqs indicates no index prerequisites.
	noPrereqs = "none"

	// syncUpdateInterval is the time between periodically checking indexes
	// and notifying their synchronization subscribers if synced.
	syncUpdateInterval = time.Millisecond * 500
)

// IndexNtfn represents an index notification detailing a block connection
// or disconnection.
type IndexNtfn struct {
	NtfnType          IndexNtfnType
	Block             *dcrutil.Block
	Parent            *dcrutil.Block
	PrevScripts       PrevScripter
	IsTreasuryEnabled bool
	Done              chan bool
}

// IndexSubscription represents a subscription for index updates.
type IndexSubscription struct {
	id         string
	idx        Indexer
	subscriber *IndexSubscriber
	mtx        sync.Mutex

	// prerequisite defines the notification processing hierarchy for this
	// subscription. It is expected that the subscriber associated with the
	// prerequisite provided processes notifications before they are
	// delivered by this subscription to its subscriber. An empty string
	// indicates the subscription has no prerequisite.
	prerequisite string

	// dependent defines the index subscription that requires the subscriber
	// associated with this subscription to have processed incoming
	// notifications before it does. A nil dependency indicates the subscription
	// has no dependencies.
	dependent *IndexSubscription
}

// newIndexSubscription initializes a new index subscription.
func newIndexSubscription(subber *IndexSubscriber, indexer Indexer, prereq string) *IndexSubscription {
	return &IndexSubscription{
		id:           indexer.Name(),
		idx:          indexer,
		prerequisite: prereq,
		subscriber:   subber,
	}
}

// stop prevents any future index updates from being delivered and
// unsubscribes the associated subscription.
//
// This must be called with the index subscriber mutex held for writes.
func (s *IndexSubscription) stop() error {

	// If the subscription has a prerequisite, find it and remove the
	// subscription as a dependency.
	if s.prerequisite != noPrereqs {
		s.mtx.Lock()
		prereq, ok := s.subscriber.subscriptions[s.prerequisite]
		s.mtx.Unlock()
		if !ok {
			return fmt.Errorf("no subscription found with id %s", s.prerequisite)
		}

		prereq.mtx.Lock()
		prereq.dependent = nil
		prereq.mtx.Unlock()

		return nil
	}

	// If the subscription has a dependent, stop it as well.
	if s.dependent != nil {
		err := s.dependent.stop()
		if err != nil {
			return err
		}
	}

	// If the subscription is independent, remove it from the
	// index subscriber's subscriptions.
	delete(s.subscriber.subscriptions, s.id)

	return nil
}

// IndexSubscriber subscribes clients for index updates.
type IndexSubscriber struct {
	subscribers uint32 // update atomically.

	c             chan IndexNtfn
	subscriptions map[string]*IndexSubscription
	mtx           sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	quit          chan struct{}
}

// NewIndexSubscriber creates a new index subscriber. It also starts the
// handler for incoming index update subscriptions.
func NewIndexSubscriber(sCtx context.Context) *IndexSubscriber {
	ctx, cancel := context.WithCancel(sCtx)
	s := &IndexSubscriber{
		c:             make(chan IndexNtfn, bufferSize),
		subscriptions: make(map[string]*IndexSubscription),
		ctx:           ctx,
		cancel:        cancel,
		quit:          make(chan struct{}),
	}
	return s
}

// Subscribe subscribes an index for updates.  The returned index subscription
// has functions to retrieve a channel that produces a stream of index updates
// and to stop the stream when the caller no longer wishes to receive updates.
func (s *IndexSubscriber) Subscribe(index Indexer, prerequisite string) (*IndexSubscription, error) {
	sub := newIndexSubscription(s, index, prerequisite)

	// If the subscription has a prequisite, find it and set the subscription
	// as a dependency.
	if prerequisite != noPrereqs {
		s.mtx.Lock()
		prereq, ok := s.subscriptions[prerequisite]
		s.mtx.Unlock()
		if !ok {
			return nil, fmt.Errorf("no subscription found with id %s", prerequisite)
		}

		prereq.mtx.Lock()
		defer prereq.mtx.Unlock()

		if prereq.dependent != nil {
			return nil, fmt.Errorf("%s already has a dependent set: %s",
				prereq.id, prereq.dependent.id)
		}

		prereq.dependent = sub
		atomic.AddUint32(&s.subscribers, 1)

		return sub, nil
	}

	// If the subscription does not have a prerequisite, add it to the index
	// subscriber's subscriptions.
	s.mtx.Lock()
	s.subscriptions[sub.id] = sub
	s.mtx.Unlock()

	atomic.AddUint32(&s.subscribers, 1)

	return sub, nil
}

// Notify relays an index notification to subscribed indexes for processing.
func (s *IndexSubscriber) Notify(ntfn *IndexNtfn) {
	subscribers := atomic.LoadUint32(&s.subscribers)

	// Only relay notifications when there are subscribed indexes
	// to be notified.
	if subscribers > 0 {
		select {
		case <-s.quit:
		case s.c <- *ntfn:
		}
	}
}

// findLowestIndexTipHeight determines the lowest index tip height among
// subscribed indexes and their dependencies.
func (s *IndexSubscriber) findLowestIndexTipHeight(queryer ChainQueryer) (int64, int64, error) {
	// Find the lowest tip height to catch up among subscribed indexes.
	bestHeight, _ := queryer.Best()
	lowestHeight := bestHeight
	for _, sub := range s.subscriptions {
		tipHeight, tipHash, err := sub.idx.Tip()
		if err != nil {
			return 0, bestHeight, err
		}

		// Ensure the index tip is on the main chain.
		if !queryer.MainChainHasBlock(tipHash) {
			return 0, bestHeight, fmt.Errorf("%s: index tip (%s) is not on the "+
				"main chain", sub.idx.Name(), tipHash)
		}

		if tipHeight < lowestHeight {
			lowestHeight = tipHeight
		}

		// Update the lowest tip height if a dependent has a lower tip height.
		dependent := sub.dependent
		for dependent != nil {
			tipHeight, _, err := sub.dependent.idx.Tip()
			if err != nil {
				return 0, bestHeight, err
			}

			if tipHeight < lowestHeight {
				lowestHeight = tipHeight
			}

			dependent = dependent.dependent
		}
	}

	return lowestHeight, bestHeight, nil
}

// CatchUp syncs all subscribed indexes to the the main chain by connecting
// blocks from after the lowest index tip to the current main chain tip.
//
// This should be called after all indexes have subscribed for updates.
func (s *IndexSubscriber) CatchUp(ctx context.Context, db database.DB, queryer ChainQueryer) error {
	lowestHeight, bestHeight, err := s.findLowestIndexTipHeight(queryer)
	if err != nil {
		return err
	}

	// Nothing to do if all indexes are synced.
	if bestHeight == lowestHeight {
		return nil
	}

	// Create a progress logger for the indexing process below.
	progressLogger := progresslog.NewBlockProgressLogger("Indexed", log)

	// tip and need to be caught up, so log the details and loop through
	// each block that needs to be indexed.
	log.Infof("Catching up from height %d to %d", lowestHeight,
		bestHeight)

	var cachedParent *dcrutil.Block
	for height := lowestHeight + 1; height <= bestHeight; height++ {
		if interruptRequested(ctx) {
			return indexerError(ErrInterruptRequested, interruptMsg)
		}

		hash, err := queryer.BlockHashByHeight(height)
		if err != nil {
			return err
		}

		// Ensure the next tip hash is on the main chain.
		if !queryer.MainChainHasBlock(hash) {
			msg := fmt.Sprintf("the next block being synced to (%s) "+
				"at height %d is not on the main chain", hash, height)
			return indexerError(ErrBlockNotOnMainChain, msg)
		}

		var parent *dcrutil.Block
		if cachedParent == nil && height > 0 {
			parentHash, err := queryer.BlockHashByHeight(height - 1)
			if err != nil {
				return err
			}
			parent, err = queryer.BlockByHash(parentHash)
			if err != nil {
				return err
			}
		} else {
			parent = cachedParent
		}

		child, err := queryer.BlockByHash(hash)
		if err != nil {
			return err
		}

		// Construct and send the index notification.
		var prevScripts PrevScripter
		err = db.View(func(dbTx database.Tx) error {
			if interruptRequested(ctx) {
				return indexerError(ErrInterruptRequested, interruptMsg)
			}

			prevScripts, err = queryer.PrevScripts(dbTx, child)
			if err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			return err
		}

		isTreasuryEnabled, err := queryer.IsTreasuryAgendaActive(parent.Hash())
		if err != nil {
			return err
		}

		ntfn := &IndexNtfn{
			NtfnType:          ConnectNtfn,
			Block:             child,
			Parent:            parent,
			PrevScripts:       prevScripts,
			IsTreasuryEnabled: isTreasuryEnabled,
		}

		// Relay the index update to subscribed indexes.
		for _, sub := range s.subscriptions {
			err := updateIndex(ctx, sub.idx, ntfn)
			if err != nil {
				s.cancel()
				return err
			}
		}

		cachedParent = child

		progressLogger.LogBlockHeight(child.MsgBlock(), parent.MsgBlock())
	}

	log.Infof("Caught up to height %d", bestHeight)

	return nil
}

// handleSyncSubscribers updates index sync subscribers when a subscribed
// indexer is fully synced.
//
// This should be run as a goroutine.
func (s *IndexSubscriber) handleSyncSubscribers(ctx context.Context) {
	ticker := time.NewTicker(syncUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.wg.Done()
			return

		case <-ticker.C:
			s.mtx.Lock()
			for _, sub := range s.subscriptions {
				err := maybeNotifySubscribers(ctx, sub.idx)
				if err != nil {
					log.Errorf("unable to notify sync subscribers: %v", err)
					s.cancel()
				}
			}
			s.mtx.Unlock()
		}
	}
}

// handleIndexUpdates relays updates to subscribed indexers.
func (s *IndexSubscriber) handleIndexUpdates(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			close(s.quit)

			// Stop all updates to subscribed indexes and terminate their
			// processes.
			s.mtx.Lock()
			for _, sub := range s.subscriptions {
				err := sub.stop()
				if err != nil {
					log.Error("unable to stop index subscription: %v", err)
				}
			}
			s.mtx.Unlock()

			s.cancel()
			s.wg.Done()

			return

		case ntfn := <-s.c:
			// Relay the index update to subscribed indexes.
			s.mtx.Lock()
			for _, sub := range s.subscriptions {
				err := updateIndex(ctx, sub.idx, &ntfn)
				if err != nil {
					log.Error(err)
					s.cancel()
					break
				}
			}
			s.mtx.Unlock()

			if ntfn.Done != nil {
				close(ntfn.Done)
			}
		}
	}
}

// Run relays index notifications to subscribed indexes.
//
// This should be run as a goroutine.
func (s *IndexSubscriber) Run(ctx context.Context) {
	s.wg.Add(2)
	go s.handleIndexUpdates(ctx)
	go s.handleSyncSubscribers(ctx)
	s.wg.Wait()

	log.Infof("Index subscriber shutting down")
}
