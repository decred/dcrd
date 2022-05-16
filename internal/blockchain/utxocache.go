// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/wire"
)

const (
	// outpointSize is the size of an outpoint on a 64-bit platform.  It is
	// equivalent to what unsafe.Sizeof(wire.OutPoint{}) returns on a 64-bit
	// platform.
	outpointSize = 56

	// pointerSize is the size of a pointer on a 64-bit platform.
	pointerSize = 8

	// p2pkhScriptLen is the length of a standard pay-to-pubkey-hash script.  It
	// is used in the calculation to approximate the average size of a utxo
	// entry when setting the initial capacity of the cache.
	p2pkhScriptLen = 25

	// mapOverhead is the number of bytes per entry to use when approximating
	// the memory overhead of the entries map itself (i.e. the memory usage due
	// to internals of the map, such as the underlying buckets that are
	// allocated).  This number was determined by inspecting the true size of
	// the map with various numbers of entries and comparing it with the total
	// size of all entries in the map.  The average overhead came out to 57
	// bytes per entry.
	mapOverhead = 57

	// evictionPercentage is the targeted percentage of entries to evict from
	// the cache when its maximum size has been reached.
	//
	// A lower percentage will result in a higher overall hit ratio for the
	// cache, and thus better performance, but will require eviction again
	// sooner.  This value was selected to keep the hit ratio of the cache as
	// high as possible while still evicting a significant portion of the cache
	// when it reaches its maximum allowed size.
	evictionPercentage = 0.15

	// periodicFlushInterval is the amount of time to wait before a periodic
	// flush is required.
	//
	// The cache is flushed periodically during initial block download to avoid
	// requiring a flush that would take a significant amount of time on
	// shutdown (or, in the case of an unclean shutdown, a significant amount of
	// time to initialize the cache when restarted).
	periodicFlushInterval = time.Minute * 2
)

// UtxoCacher represents a utxo cache that sits on top of a utxo set backend.
//
// The interface contract requires that all of these methods are safe for
// concurrent access.
type UtxoCacher interface {
	// Commit updates the cache based on the state of each entry in the provided
	// view.
	//
	// All entries in the provided view that are marked as modified and spent
	// are removed from the view.  Additionally, all entries that are added to
	// the cache are removed from the provided view.
	Commit(view *UtxoViewpoint) error

	// FetchBackendState returns the current state of the UTXO set in the
	// backend.
	FetchBackendState() (*UtxoSetState, error)

	// FetchEntries adds the requested transaction outputs to the provided view.
	// It first checks the cache for each output, and if an output does not
	// exist in the cache, it will fetch it from the backend.
	//
	// Upon completion of this function, the view will contain an entry for each
	// requested outpoint.  Spent outputs, or those which otherwise don't exist,
	// will result in a nil entry in the view.
	FetchEntries(filteredSet ViewFilteredSet, view *UtxoViewpoint) error

	// FetchEntry returns the specified transaction output from the utxo set.
	// If the output exists in the cache, it is returned immediately.
	// Otherwise, it fetches the output from the backend, caches it, and returns
	// it to the caller.  The entry that is returned can safely be mutated by
	// the caller without invalidating the cache.
	//
	// When there is no entry for the provided output, nil will be returned for
	// both the entry and the error.
	FetchEntry(outpoint wire.OutPoint) (*UtxoEntry, error)

	// FetchStats returns statistics on the current utxo set.
	FetchStats(bestHash *chainhash.Hash, bestHeight uint32) (*UtxoStats, error)

	// Initialize initializes the utxo cache and underlying utxo backend.  This
	// entails running any database migrations as well as ensuring that the utxo
	// set is caught up to the tip of the best chain.
	Initialize(ctx context.Context, b *BlockChain, tip *blockNode) error

	// MaybeFlush conditionally flushes the cache to the backend.  A flush can
	// be forced by setting the force flush parameter.
	MaybeFlush(bestHash *chainhash.Hash, bestHeight uint32, forceFlush bool,
		logFlush bool) error
}

// UtxoCache is an unspent transaction output cache that sits on top of the
// utxo set backend and provides significant runtime performance benefits at
// the cost of some additional memory usage.  It drastically reduces the amount
// of reading and writing to disk, especially during initial block download when
// a very large number of blocks are being processed in quick succession.
//
// The UtxoCache is a read-through cache.  All utxo reads go through the cache.
// When there is a cache miss, the cache loads the missing data from the
// backend, caches it, and returns it to the caller.
//
// The UtxoCache is a write-back cache.  Writes to the cache are acknowledged
// by the cache immediately but are only periodically flushed to the backend.
// This allows intermediate steps to effectively be skipped.  For example, a
// utxo that is created and then spent in between flushes never needs to be
// written to the utxo set in the backend.
//
// Due to the write-back nature of the cache, at any given time the backend
// may not be in sync with the cache, and therefore all utxo reads and writes
// MUST go through the cache, and never read or write to the backend directly.
type UtxoCache struct {
	// backend is the backend that contains the UTXO set.  It is set when the
	// instance is created and is not changed afterward.
	backend UtxoBackend

	// flushBlockDB defines the function to use to flush the block database to
	// disk.  The block database is always flushed to disk before the UTXO cache
	// writes to disk in order to maintain a recoverable state in the event of
	// an unclean shutdown.  It is set when the instance is created and is not
	// changed afterward.
	flushBlockDB func() error

	// maxSize is the maximum allowed size of the utxo cache, in bytes.  It is
	// set when the instance is created and is not changed afterward.
	maxSize uint64

	// cacheLock protects access to the fields in the struct below this point.
	// A standard mutex is used rather than a read-write mutex since the cache
	// will often write when reads result in a cache miss, so it is generally
	// not worth the additional overhead of using a read-write mutex.
	cacheLock sync.Mutex

	// entries holds the cached utxo entries.
	entries map[wire.OutPoint]*UtxoEntry

	// lastFlushHash is the block hash of the last flush.  It is used to compare
	// the state of the cache to the utxo set state in the backend so that the
	// utxo set can properly be initialized in the case that the latest utxo
	// data had not been flushed to the backend yet.
	lastFlushHash chainhash.Hash

	// lastFlushTime is the last time that the cache was flushed to the backend.
	// It is used to determine when to periodically flush the cache to the
	// backend during initial block download even if the cache isn't full to
	// minimize the amount of progress lost if an unclean shutdown occurs.
	lastFlushTime time.Time

	// lastEvictionHeight is the block height of the last eviction.  When the
	// cache reaches the maximum allowed size, entries are evicted based on the
	// height of the block that they are contained in, and last eviction height
	// is updated to the current height.
	lastEvictionHeight uint32

	// totalEntrySize is the total size of all utxo entries in the cache, in
	// bytes.  It is updated whenever an entry is added or removed from the
	// cache.
	totalEntrySize uint64

	// The following fields track the total number of cache hits and misses and
	// are used to measure the overall cache hit ratio.
	hits   uint64
	misses uint64

	// timeNow defines the function to use to get the current local time.  It
	// defaults to time.Now but an alternative function can be provided for
	// testing purposes.
	timeNow func() time.Time
}

// Ensure UtxoCache implements the UtxoCacher interface.
var _ UtxoCacher = (*UtxoCache)(nil)

// UtxoCacheConfig is a descriptor which specifies the utxo cache instance
// configuration.
type UtxoCacheConfig struct {
	// Backend defines the backend which houses the utxo set.
	//
	// This field is required.
	Backend UtxoBackend

	// FlushBlockDB defines the function to use to flush the block database to
	// disk.  The block database is always flushed to disk before the UTXO cache
	// writes to disk in order to maintain a recoverable state in the event of
	// an unclean shutdown.
	//
	// This field is required.
	FlushBlockDB func() error

	// MaxSize defines the maximum allowed size of the utxo cache, in bytes.
	//
	// This field is required.
	MaxSize uint64
}

// NewUtxoCache returns a UtxoCache instance using the provided configuration
// details.
func NewUtxoCache(config *UtxoCacheConfig) *UtxoCache {
	// Approximate the maximum number of entries allowed in the cache in order
	// to set the initial capacity of the entries map.
	avgEntrySize := mapOverhead + outpointSize + pointerSize + baseEntrySize +
		p2pkhScriptLen
	maxEntries := math.Ceil(float64(config.MaxSize) / float64(avgEntrySize))

	return &UtxoCache{
		backend:       config.Backend,
		flushBlockDB:  config.FlushBlockDB,
		maxSize:       config.MaxSize,
		entries:       make(map[wire.OutPoint]*UtxoEntry, uint64(maxEntries)),
		lastFlushTime: time.Now(),
		timeNow:       time.Now,
	}
}

// totalSize returns the total size of the cache on a 64-bit platform, in bytes.
// Note that this only takes the entries map into account, which represents the
// vast majority of the memory that the cache uses, and does not include the
// memory usage of other fields in the utxo cache struct.
//
// This function MUST be called with the cache lock held.
func (c *UtxoCache) totalSize() uint64 {
	numEntries := uint64(len(c.entries))
	return mapOverhead*numEntries + outpointSize*numEntries +
		pointerSize*numEntries + c.totalEntrySize
}

// hitRatio returns the percentage of cache lookups that resulted in a cache
// hit.
//
// This function MUST be called with the cache lock held.
func (c *UtxoCache) hitRatio() float64 {
	totalLookups := c.hits + c.misses
	if totalLookups == 0 {
		return 100
	}

	return float64(c.hits) / float64(totalLookups) * 100
}

// addEntry adds the specified output to the cache.  The entry being added MUST
// NOT be mutated by the caller after being passed to this function.
//
// Note that this function does not check if the entry is unspendable and
// therefore the caller should ensure that the entry is spendable before adding
// it to the cache.
//
// This function MUST be called with the cache lock held.
func (c *UtxoCache) addEntry(outpoint wire.OutPoint, entry *UtxoEntry) error {
	// Attempt to get an existing entry from the cache.
	cachedEntry := c.entries[outpoint]

	// If an existing entry does not exist, the added entry should be marked as
	// modified and fresh.
	if cachedEntry == nil {
		entry.state |= utxoStateModified | utxoStateFresh
	}

	// Add the entry to the cache.  In the case that an entry already exists,
	// the existing entry is overwritten.  All fields are overwritten because
	// it's possible (although extremely unlikely) that the existing entry is
	// being replaced by a different transaction with the same hash.  This is
	// allowed so long as the previous transaction is fully spent.
	c.entries[outpoint] = entry

	// Update the total entry size of the cache.
	if cachedEntry != nil {
		c.totalEntrySize -= cachedEntry.size()
	}
	c.totalEntrySize += entry.size()

	return nil
}

// AddEntry adds the specified output to the cache.  The entry being added MUST
// NOT be mutated by the caller after being passed to this function.
//
// Note that this function does not check if the entry is unspendable and
// therefore the caller should ensure that the entry is spendable before adding
// it to the cache.
//
// This function is safe for concurrent access.
func (c *UtxoCache) AddEntry(outpoint wire.OutPoint, entry *UtxoEntry) error {
	c.cacheLock.Lock()
	err := c.addEntry(outpoint, entry)
	c.cacheLock.Unlock()
	return err
}

// spendEntry marks the specified output as spent.
//
// This function MUST be called with the cache lock held.
func (c *UtxoCache) spendEntry(outpoint wire.OutPoint) {
	// Attempt to get an existing entry from the cache.
	cachedEntry := c.entries[outpoint]

	// If the entry is nil or already spent, return immediately.
	if cachedEntry == nil || cachedEntry.IsSpent() {
		return
	}

	// If the entry is fresh, and is now being spent, it can safely be removed.
	// This is an optimization to skip writing to the backend for outputs that
	// are added and spent in between flushes to the backend.
	if cachedEntry.isFresh() {
		// The entry in the map is marked as nil rather than deleting it so that
		// subsequent lookups for the outpoint will still result in a cache hit
		// and avoid querying the backend.
		c.entries[outpoint] = nil
		c.totalEntrySize -= cachedEntry.size()
		return
	}

	// Mark the output as spent and modified.
	cachedEntry.Spend()
}

// SpendEntry marks the specified output as spent.
//
// This function is safe for concurrent access.
func (c *UtxoCache) SpendEntry(outpoint wire.OutPoint) {
	c.cacheLock.Lock()
	c.spendEntry(outpoint)
	c.cacheLock.Unlock()
}

// fetchEntry returns the specified transaction output from the utxo set.  If
// the output exists in the cache, it is returned immediately.  Otherwise, it
// fetches the output from the backend, caches it, and returns it to the
// caller.  A cloned copy of the entry is returned so it can safely be mutated
// by the caller without invalidating the cache.
//
// When there is no entry for the provided output, nil will be returned for both
// the entry and the error.
//
// This function MUST be called with the cache lock held.
func (c *UtxoCache) fetchEntry(outpoint wire.OutPoint) (*UtxoEntry, error) {
	// If the cache already has the entry, return it immediately.  A cloned copy
	// of the entry is returned so it can safely be mutated by the caller
	// without invalidating the cache.
	var entry *UtxoEntry
	if entry, found := c.entries[outpoint]; found {
		c.hits++
		return entry.Clone(), nil
	}

	// Increment cache misses.
	c.misses++

	// Fetch the entry from the backend.
	//
	// NOTE: Missing entries are not considered an error here and instead
	// will result in nil entries in the view.  This is intentionally done
	// so other code can use the presence of an entry in the view as a way
	// to unnecessarily avoid attempting to reload it from the backend.
	entry, err := c.backend.FetchEntry(outpoint)
	if err != nil {
		return nil, err
	}

	// Update the total entry size of the cache.
	if entry != nil {
		c.totalEntrySize += entry.size()
	}

	// Add the entry to the cache and return it.  A cloned copy of the entry is
	// returned so it can safely be mutated by the caller without invalidating
	// the cache.
	c.entries[outpoint] = entry
	return entry.Clone(), nil
}

// FetchEntry returns the specified transaction output from the utxo set.  If
// the output exists in the cache, it is returned immediately.  Otherwise, it
// fetches the output from the backend, caches it, and returns it to the
// caller.  A cloned copy of the entry is returned so it can safely be mutated
// by the caller without invalidating the cache.
//
// When there is no entry for the provided output, nil will be returned for both
// the entry and the error.
//
// This function is safe for concurrent access.
func (c *UtxoCache) FetchEntry(outpoint wire.OutPoint) (*UtxoEntry, error) {
	c.cacheLock.Lock()
	entry, err := c.fetchEntry(outpoint)
	c.cacheLock.Unlock()
	return entry, err
}

// FetchEntries adds the requested transaction outputs to the provided view.  It
// first checks the cache for each output, and if an output does not exist in
// the cache, it will fetch it from the backend.
//
// Upon completion of this function, the view will contain an entry for each
// requested outpoint.  Spent outputs, or those which otherwise don't exist,
// will result in a nil entry in the view.
//
// This function is safe for concurrent access.
func (c *UtxoCache) FetchEntries(filteredSet ViewFilteredSet, view *UtxoViewpoint) error {
	c.cacheLock.Lock()
	for outpoint := range filteredSet {
		entry, err := c.fetchEntry(outpoint)
		if err != nil {
			c.cacheLock.Unlock()
			return err
		}

		// NOTE: Missing entries are not considered an error here and instead
		// will result in nil entries in the view.  This is intentionally done
		// so other code can use the presence of an entry in the view as a way
		// to unnecessarily avoid attempting to reload it from the backend.
		view.entries[outpoint] = entry
	}
	c.cacheLock.Unlock()

	return nil
}

// FetchBackendState returns the current state of the UTXO set in the backend.
func (c *UtxoCache) FetchBackendState() (*UtxoSetState, error) {
	return c.backend.FetchState()
}

// FetchStats returns statistics on the current utxo set.
func (c *UtxoCache) FetchStats(bestHash *chainhash.Hash, bestHeight uint32) (*UtxoStats, error) {
	// Force a UTXO cache flush.  This is required in order for the backend to
	// fetch statistics on the full UTXO set.
	err := c.MaybeFlush(bestHash, bestHeight, true, false)
	if err != nil {
		return nil, err
	}

	return c.backend.FetchStats()
}

// Commit updates the cache based on the state of each entry in the provided
// view.
//
// All entries in the provided view that are marked as modified and spent are
// removed from the view.  Additionally, all entries that are added to the cache
// are removed from the provided view.
//
// This function is safe for concurrent access.
func (c *UtxoCache) Commit(view *UtxoViewpoint) error {
	c.cacheLock.Lock()
	for outpoint, entry := range view.entries {
		// If the entry is nil, delete it from the view and continue.
		if entry == nil {
			delete(view.entries, outpoint)
			continue
		}

		// If the entry is not modified and not fresh, continue as there is
		// nothing to do.
		if !entry.isModified() && !entry.isFresh() {
			continue
		}

		// If the entry is modified and spent, mark it as spent in the cache and
		// then delete it from the view.
		if entry.isModified() && entry.IsSpent() {
			// Mark the entry as spent in the cache.
			c.spendEntry(outpoint)

			// Delete the entry from the view.
			delete(view.entries, outpoint)
			continue
		}

		// If we passed all of the conditions above, the entry is modified or
		// fresh, but not spent, and should be added to the cache.
		err := c.addEntry(outpoint, entry)
		if err != nil {
			c.cacheLock.Unlock()
			return err
		}

		// All entries that are added to the cache should be removed from the
		// provided view.  This is an optimization to allow the cache to take
		// ownership of the entry from the view and avoid an additional
		// allocation.  It is removed from the view to ensure that it is not
		// mutated by the caller after being added to the cache.
		//
		// This does cause the view to refetch the entry if it is requested
		// again after being removed.  However, this only really occurs during
		// reorgs, whereas committing the view to the cache happens with every
		// connected block, so this optimizes for the much more common case.
		delete(view.entries, outpoint)
	}
	c.cacheLock.Unlock()

	return nil
}

// calcEvictionHeight returns the eviction height based on the best height of
// the main chain and the last eviction height.  All entries that are contained
// in a block at a height less than the eviction height will be evicted from the
// cache when the cache reaches its maximum allowed size.
//
// Eviction is based on height since the height of the block that an entry is
// contained in is a proxy for how old the utxo is.  On average, recent utxos
// are much more likely to be spent in upcoming blocks than older utxos, so the
// strategy used is to evict the oldest utxos in order to maximize the hit ratio
// of the cache.
//
// This function MUST be called with the cache lock held.
func (c *UtxoCache) calcEvictionHeight(bestHeight uint32) uint32 {
	if bestHeight < c.lastEvictionHeight {
		return bestHeight
	}

	lastEvictionDepth := bestHeight - c.lastEvictionHeight
	numBlocksToEvict := math.Ceil(float64(lastEvictionDepth) * evictionPercentage)
	return c.lastEvictionHeight + uint32(numBlocksToEvict)
}

// shouldFlush returns whether or not a flush should be performed.
//
// If the maximum size of the cache has been reached, or if the periodic flush
// interval has been reached, then a flush is required.
//
// This function MUST be called with the cache lock held.
func (c *UtxoCache) shouldFlush(bestHash *chainhash.Hash) bool {
	// No need to flush if the cache has already been flushed through the best
	// hash.
	if c.lastFlushHash == *bestHash {
		return false
	}

	// Flush if the max size of the cache has been reached.
	if c.totalSize() >= c.maxSize {
		return true
	}

	// Flush if the periodic flush interval has been reached.
	return time.Since(c.lastFlushTime) >= periodicFlushInterval
}

// flush commits all modified entries to the backend and conditionally evicts
// entries.
//
// Entries that are nil or spent are always evicted since they are
// unlikely to be accessed again.  Additionally, if the cache has reached its
// maximum size, entries are evicted based on the height of the block that they
// are contained in.
//
// This function MUST be called with the cache lock held.
func (c *UtxoCache) flush(bestHash *chainhash.Hash, bestHeight uint32, logFlush bool) error {
	// If the maximum allowed size of the cache has been reached, determine the
	// eviction height.
	var evictionHeight uint32
	memUsage := c.totalSize()
	if memUsage >= c.maxSize {
		evictionHeight = c.calcEvictionHeight(bestHeight)
	}

	// Log that a flush is starting and indicate the current memory usage, hit
	// ratio, and eviction height.
	var hitRatio float64
	var evictionLog string
	var preFlushNumEntries int
	if logFlush {
		preFlushNumEntries = len(c.entries)
		memUsageMiB := float64(memUsage) / 1024 / 1024
		memUsagePercent := float64(memUsage) / float64(c.maxSize) * 100
		hitRatio = c.hitRatio()
		if evictionHeight != 0 {
			evictionLog = fmt.Sprintf(", eviction height: %d", evictionHeight)
		}
		log.Debugf("UTXO cache flush starting (%d entries, %.2f MiB (%.2f%%), "+
			"%.2f%% hit ratio, height: %d%s)", preFlushNumEntries, memUsageMiB,
			memUsagePercent, hitRatio, bestHeight, evictionLog)
	}

	// Flush the block database to disk.  The block database MUST always be
	// flushed to disk prior to flushing the UTXO cache to the UTXO database.
	// This ensures that the block database is always at least as far along as
	// the UTXO database which keeps the UTXO database in a recoverable state in
	// the event of an unclean shutdown.
	err := c.flushBlockDB()
	if err != nil {
		return err
	}

	// Atomically flush all of the entries in the cache along with the best hash
	// and best height to the backend.
	err = c.backend.PutUtxos(c.entries, &UtxoSetState{
		lastFlushHeight: bestHeight,
		lastFlushHash:   *bestHash,
	})
	if err != nil {
		return err
	}

	// Update the entries in the cache after flushing to the backend.  This is
	// done after the updates to the backend have been successfully committed to
	// ensure that an unexpected backend error would not leave the cache in an
	// inconsistent state.
	for outpoint, entry := range c.entries {
		// Conditionally evict entries from the cache.  Entries that are nil or
		// spent are always evicted since they are unlikely to be accessed
		// again.  Additionally, entries that are contained in a block with a
		// height less than the eviction height are evicted.
		if entry == nil || entry.IsSpent() ||
			entry.BlockHeight() < int64(evictionHeight) {

			// Remove the entry from the cache.
			delete(c.entries, outpoint)

			// Update the total entry size of the cache.
			if entry != nil {
				c.totalEntrySize -= entry.size()
			}

			continue
		}

		// If the entry wasn't removed from the cache, clear the modified and
		// fresh flags since it has been updated in the backend.
		entry.state &^= utxoStateModified
		entry.state &^= utxoStateFresh
	}

	// Update the last flush on the cache instance now that the flush has been
	// completed.
	c.lastFlushHash = *bestHash
	c.lastFlushTime = c.timeNow()

	// Update the last eviction height on the cache instance if we evicted just
	// now.
	if evictionHeight != 0 {
		c.lastEvictionHeight = evictionHeight
	}

	// Log that the flush has been completed and indicate the updated memory
	// usage as it will be reduced due to evicting entries above.
	if logFlush {
		remainingEntries := len(c.entries)
		flushedEntries := preFlushNumEntries - remainingEntries
		memUsage = c.totalSize()
		memUsageMiB := float64(memUsage) / 1024 / 1024
		memUsagePercent := float64(memUsage) / float64(c.maxSize) * 100
		log.Debugf("UTXO cache flush completed (%d entries flushed, %d "+
			"entries remaining, %.2f MiB (%.2f%%))", flushedEntries,
			remainingEntries, memUsageMiB, memUsagePercent)
	}

	return nil
}

// MaybeFlush conditionally flushes the cache to the backend.
//
// If the maximum size of the cache has been reached, or if the periodic flush
// interval has been reached, then a flush is required.  Additionally, a flush
// can be forced by setting the force flush parameter.
//
// This function is safe for concurrent access.
func (c *UtxoCache) MaybeFlush(bestHash *chainhash.Hash, bestHeight uint32,
	forceFlush bool, logFlush bool) error {

	c.cacheLock.Lock()
	if forceFlush || c.shouldFlush(bestHash) {
		err := c.flush(bestHash, bestHeight, logFlush)
		c.cacheLock.Unlock()
		return err
	}

	c.cacheLock.Unlock()
	return nil
}

// Initialize initializes the utxo cache and underlying utxo backend.  This
// entails running any database migrations as well as ensuring that the utxo set
// is caught up to the tip of the best chain.
//
// Since the cache is only flushed to the backend periodically, the utxo set
// may not be caught up to the tip of the best chain.  This function catches the
// utxo set up by replaying all blocks from the block after the block that was
// last flushed to the tip block through the cache.
//
// This function should only be called during initialization.
func (c *UtxoCache) Initialize(ctx context.Context, b *BlockChain, tip *blockNode) error {
	log.Infof("UTXO cache initializing (max size: %d MiB)...",
		c.maxSize/1024/1024)

	// Upgrade the UTXO backend as needed.
	err := c.backend.Upgrade(ctx, b)
	if err != nil {
		return err
	}

	// Fetch the utxo set state from the backend.
	state, err := c.backend.FetchState()
	if err != nil {
		return err
	}

	// If the state is nil, update the state to the tip.  This should only be
	// the case when starting from a fresh backend or a backend that has not
	// been run with the utxo cache yet.
	if state == nil {
		state = &UtxoSetState{
			lastFlushHeight: uint32(tip.height),
			lastFlushHash:   tip.hash,
		}
		err := c.backend.PutUtxos(c.entries, state)
		if err != nil {
			return err
		}
	}

	// Set the last flush hash and the last eviction height from the saved state
	// since that is where we are starting from.
	c.lastFlushHash = state.lastFlushHash
	c.lastEvictionHeight = state.lastFlushHeight

	// If state is already caught up to the tip, return as there is nothing to
	// do.
	if state.lastFlushHash == tip.hash {
		log.Info("UTXO cache initialization completed")
		return nil
	}

	// Find the fork point between the current tip and the last flushed block.
	lastFlushedNode := b.index.LookupNode(&state.lastFlushHash)
	if lastFlushedNode == nil {
		// panic if the last flushed block node does not exist.  This should
		// never happen unless the backend is corrupted.
		panicf("last flushed block node hash %v (height %v) does not exist",
			state.lastFlushHash, state.lastFlushHeight)
	}
	fork := b.bestChain.FindFork(lastFlushedNode)

	// Disconnect all of the blocks back to the point of the fork.  This entails
	// loading the blocks and their associated spent txos from the backend and
	// using that information to unspend all of the spent txos and remove the
	// utxos created by the blocks.  In addition, if a block votes against its
	// parent, the regular transactions are reconnected.
	//
	// Note that blocks will only need to be disconnected during initialization
	// if an unclean shutdown occurred between a block being disconnected and
	// the cache being flushed.  Since the cache is always flushed immediately
	// after disconnecting a block, this will occur very infrequently.  In the
	// typical catchup case, the fork node will be the last flushed node itself
	// and this loop will be skipped.
	view := NewUtxoViewpoint(c)
	view.SetBestHash(&tip.hash)
	var nextBlockToDetach *dcrutil.Block
	n := lastFlushedNode
	for n != nil && n != fork {
		select {
		case <-b.interrupt:
			return errInterruptRequested
		default:
		}

		// Grab the block to detach based on the node.  Use the fact that the
		// blocks are being detached in reverse order, so the parent of the
		// current block being detached is the next one being detached.
		block := nextBlockToDetach
		if block == nil {
			var err error
			block, err = b.fetchBlockByNode(n)
			if err != nil {
				return err
			}
		}
		if n.hash != *block.Hash() {
			panicf("detach block node hash %v (height %v) does not match "+
				"previous parent block hash %v", &n.hash, n.height,
				block.Hash())
		}

		// Grab the parent of the current block and also save a reference to it
		// as the next block to detach so it doesn't need to be loaded again on
		// the next iteration.
		parent, err := b.fetchBlockByNode(n.parent)
		if err != nil {
			return err
		}
		nextBlockToDetach = parent

		// Determine if treasury agenda is active.
		isTreasuryEnabled, err := b.isTreasuryAgendaActive(n.parent)
		if err != nil {
			return err
		}

		// Load all of the spent txos for the block from the spend journal.
		var stxos []spentTxOut
		err = b.db.View(func(dbTx database.Tx) error {
			stxos, err = dbFetchSpendJournalEntry(dbTx, block, isTreasuryEnabled)
			return err
		})
		if err != nil {
			return err
		}

		// Update the view to unspend all of the spent txos and remove the utxos
		// created by the block.  Also, if the block votes against its parent,
		// reconnect all of the regular transactions.
		err = view.disconnectBlock(block, parent, stxos, isTreasuryEnabled)
		if err != nil {
			return err
		}

		// Commit all entries in the view to the utxo cache.  All entries in the
		// view that are marked as modified and spent are removed from the view.
		// Additionally, all entries that are added to the cache are removed
		// from the view.
		err = c.Commit(view)
		if err != nil {
			return err
		}

		// Conditionally flush the utxo cache to the backend.  Don't force flush
		// since many blocks may be disconnected and connected in quick
		// succession when initializing.
		err = c.MaybeFlush(&n.hash, uint32(n.height), false, true)
		if err != nil {
			return err
		}

		n = n.parent
	}

	// Determine the blocks to attach after the fork point.  Each block is added
	// to the slice from back to front so they are attached in the appropriate
	// order when iterating the slice below.
	replayNodes := make([]*blockNode, tip.height-fork.height)
	for n := tip; n != nil && n != fork; n = n.parent {
		replayNodes[n.height-fork.height-1] = n
	}

	// Replay all of the blocks through the cache.
	var prevBlockAttached *dcrutil.Block
	for i, n := range replayNodes {
		select {
		case <-b.interrupt:
			return errInterruptRequested
		default:
		}

		// Grab the block to attach based on the node.  The parent of the block
		// is the previous one that was attached except for the first node being
		// attached, which needs to be fetched.
		block, err := b.fetchBlockByNode(n)
		if err != nil {
			return err
		}
		parent := prevBlockAttached
		if i == 0 {
			parent, err = b.fetchBlockByNode(n.parent)
			if err != nil {
				return err
			}
		}
		if n.parent.hash != *parent.Hash() {
			panicf("attach block node hash %v (height %v) parent hash %v does "+
				"not match previous parent block hash %v", &n.hash, n.height,
				&n.parent.hash, parent.Hash())
		}

		// Store the loaded block as parent of the block in the next iteration.
		prevBlockAttached = block

		// Determine if treasury agenda is active.
		isTreasuryEnabled, err := b.isTreasuryAgendaActive(n.parent)
		if err != nil {
			return err
		}

		// Update the view to mark all utxos referenced by the block as
		// spent and add all transactions being created by this block to it.
		// In the case the block votes against the parent, also disconnect
		// all of the regular transactions in the parent block.
		err = view.connectBlock(b.db, block, parent, nil, isTreasuryEnabled)
		if err != nil {
			return err
		}

		// Commit all entries in the view to the utxo cache.  All entries in the
		// view that are marked as modified and spent are removed from the view.
		// Additionally, all entries that are added to the cache are removed
		// from the view.
		err = c.Commit(view)
		if err != nil {
			return err
		}

		// Conditionally flush the utxo cache to the backend.  Don't force flush
		// since many blocks may be connected in quick succession when
		// initializing.
		err = c.MaybeFlush(&n.hash, uint32(n.height), false, true)
		if err != nil {
			return err
		}
	}

	log.Info("UTXO cache initialization completed")
	return nil
}

// ShutdownUtxoCache flushes the utxo cache to the backend on shutdown.  Since
// the cache is flushed periodically during initial block download and flushed
// after every block is connected after initial block download is complete,
// this flush that occurs during shutdown should finish relatively quickly.
//
// Note that if an unclean shutdown occurs, the cache will still be initialized
// properly when restarted as during initialization it will replay blocks to
// catch up to the tip block if it was not fully flushed before shutting down.
// However, it is still preferred to flush when shutting down versus always
// recovering on startup since it is faster.
//
// This function should only be called during shutdown.
func (b *BlockChain) ShutdownUtxoCache() {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	tip := b.bestChain.Tip()

	// Force a cache flush and log the flush details.
	b.utxoCache.MaybeFlush(&tip.hash, uint32(tip.height), true, true)
}

// FetchUtxoEntry loads and returns the requested unspent transaction output
// from the point of view of the main chain tip.
//
// NOTE: Requesting an output for which there is no data will NOT return an
// error.  Instead both the entry and the error will be nil.  This is done to
// allow pruning of spent transaction outputs.  In practice this means the
// caller must check if the returned entry is nil before invoking methods on it.
//
// This function is safe for concurrent access however the returned entry (if
// any) is NOT.
func (b *BlockChain) FetchUtxoEntry(outpoint wire.OutPoint) (*UtxoEntry, error) {
	b.chainLock.RLock()
	entry, err := b.utxoCache.FetchEntry(outpoint)
	b.chainLock.RUnlock()
	return entry, err
}

// FetchUtxoStats returns statistics on the current utxo set.
func (b *BlockChain) FetchUtxoStats() (*UtxoStats, error) {
	tip := b.bestChain.Tip()
	return b.utxoCache.FetchStats(&tip.hash, uint32(tip.height))
}
