// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package lru provides generic type and concurrent safe LRU data structures
// with near O(1) perf and optional time-based expiration support.
package lru

import (
	"sync"
	"time"
)

// expireScanInterval is the minimum interval between expiration scans.
const expireScanInterval = 30 * time.Second

// element is an element in the linked list used to house the KV pairs and
// associated expiration data.
type element[K comparable, V any] struct {
	key          K
	value        V
	expiresAfter int64
	prev, next   *element[K, V]
}

// Map provides a concurrency safe least recently used (LRU) map with nearly
// O(1) lookups, inserts, and deletions.  The map is limited to a maximum number
// of items with eviction of the least recently used entry when the limit is
// exceeded.
//
// It also supports optional item expiration after a configurable time to live
// (TTL) with periodic lazy removal.  [NewMap] provides no item expiration by
// default while [NewMapWithDefaultTTL] provides a configurable TTL that is
// applied to all items by default.  In both cases, callers may override the
// default TTL of individual items via [PutWithTTL].
//
// Expiration TTLs are relative to the time an item is added or updated via
// [Put] or [PutWithTTL].  Accessing items does not extend the TTL.
//
// An efficient lazy removal scheme is used such that expired items are
// periodically removed when items are added or updated via [Put] or
// [PutWithTTL].  In other words, expired items may physically remain in the map
// until the next expiration scan is triggered, however, they will no longer
// publicly appear as members of the map.  This approach allows for efficient
// amortized removal of expired items without the need for additional background
// tasks or timers.
//
// Callers may optionally use [EvictExpiredNow] to immediately remove all items
// that are marked expired without waiting for the next expiration scan in cases
// where more deterministic expiration is required.
//
// [NewMap] or [NewMapWithDefaultTTL] must be used to create a usable map since
// the zero value of this struct is not valid.
type Map[K comparable, V any] struct {
	// These fields do not require a mutex since they are set at initialization
	// time and never modified after.
	nowFn func() time.Time
	limit uint32
	ttl   time.Duration

	mtx   sync.RWMutex
	items map[K]*element[K, V] // nearly O(1) lookups
	root  element[K, V]        // O(1) insert, update, delete
	elems []*element[K, V]     // cache for reusable old elements

	// probablyHasTimeouts is used to optimize out expiration scans when there
	// is no default TTL and there have never been any items with a TTL.
	probablyHasTimeouts bool

	nextExpireScan time.Time

	// These fields track the total number of hits and misses and are used to
	// measure the overall hit ratio.
	hits   uint64
	misses uint64
}

// NewMap returns an initialized and empty LRU map where no items will expire
// by default.
//
// The provided limit is the maximum number of items the map will hold before it
// evicts least recently used items to make room for new items.
//
// See [NewMapWithDefaultTTL] for a variant that allows a configurable time to
// live to be applied to all items by default.
//
// See the documentation for [Map] for more details.
func NewMap[K comparable, V any](limit uint32) *Map[K, V] {
	m := Map[K, V]{
		nowFn: time.Now,
		items: make(map[K]*element[K, V], limit),
		elems: make([]*element[K, V], 0, limit),
		limit: limit,
	}
	m.root.prev = &m.root
	m.root.next = &m.root
	return &m
}

// NewMapWithDefaultTTL returns an initialized and empty LRU map where the
// provided non-zero time to live (TTL) is applied to all items by default.
//
// A TTL of zero disables item expiration by default and is equivalent to
// [NewMap].
//
// See [NewMap] for a variant that does not expire any items by default.
//
// See the documentation for [Map] for more details.
func NewMapWithDefaultTTL[K comparable, V any](limit uint32, ttl time.Duration) *Map[K, V] {
	m := Map[K, V]{
		nowFn:               time.Now,
		items:               make(map[K]*element[K, V], limit),
		elems:               make([]*element[K, V], 0, limit),
		limit:               limit,
		ttl:                 ttl,
		probablyHasTimeouts: ttl > 0,
		nextExpireScan:      time.Now().Add(expireScanInterval),
	}
	m.root.prev = &m.root
	m.root.next = &m.root
	return &m
}

// removeElem removes the passed element from the internal list.
//
// This function MUST be called with the embedded mutex locked (for writes).
func (m *Map[K, V]) removeElem(elem *element[K, V]) {
	elem.prev.next = elem.next
	elem.next.prev = elem.prev
	elem.next = nil
	elem.prev = nil
}

// insertElemFront inserts the passed element at the front of the internal list.
//
// This function MUST be called with the embedded mutex locked (for writes).
func (m *Map[K, V]) insertElemFront(elem *element[K, V]) {
	elem.prev = &m.root
	elem.next = m.root.next
	elem.prev.next = elem
	elem.next.prev = elem
}

// moveElemToFront moves the passed element to the front of the internal list.
//
// This function MUST be called with the embedded mutex locked (for writes).
func (m *Map[K, V]) moveElemToFront(elem *element[K, V]) {
	// Nothing to do when the element is already at the front of the list.
	if m.root.next == elem {
		return
	}

	m.removeElem(elem)
	m.insertElemFront(elem)
}

// deleteElem removes the provided element from the internal list and deletes
// it from the map.  The caller must ensure the provided key matches the
// provided element and that the element is a member of the map.
//
// This function MUST be called with the embedded mutex locked (for writes).
func (m *Map[K, V]) deleteElem(key K, elem *element[K, V]) {
	m.removeElem(elem)
	delete(m.items, key)
	elem.key = *new(K)   // Prevent potential memleak.
	elem.value = *new(V) // Prevent potential memleak.
	m.elems = append(m.elems, elem)
}

// isElemExpired returns true when expiration is enabled and the provided
// element has expired.
//
// This function MUST be called with the embedded mutex locked (for writes).
func (m *Map[K, V]) isElemExpired(elem *element[K, V], now time.Time) bool {
	return m.probablyHasTimeouts && now.UnixNano() > elem.expiresAfter
}

// removeExpired scans through all items in the map, removes any that have
// expired, and updates the minimum time the next expire scan can take place.
//
// This function MUST be called with the embedded mutex locked (for writes).
func (m *Map[K, V]) removeExpired(now time.Time) uint32 {
	var numEvicted uint32
	nowUnix := now.UnixNano()
	for key, elem := range m.items {
		if nowUnix > elem.expiresAfter {
			m.deleteElem(key, elem)
			numEvicted++
		}
	}

	// Set next expiration scan to occur after the scan interval.
	m.nextExpireScan = now.Add(expireScanInterval)
	return numEvicted
}

// put implements the core shared logic described by [Put] and [PutWithTTL].
//
// That is, it either adds the passed key/value pair when an item for that key
// does not already exist or updates the existing item for the given key to the
// passed value.  The associated item becomes the most recently used item.
//
// The least recently used item is evicted when adding a new item would exceed
// the max limit.
//
// The expiration time for existing items is also updated according to the
// passed TTL.  A TTL of zero disables expiration for the item.
//
// It returns the number of evicted items which will either be 0 or 1.
//
// Since this method is for internal use only, it MUST only be called with
// m.limit > 0 for correct behavior.
//
// This function MUST be called with the embedded mutex locked (for writes).
func (m *Map[K, V]) put(key K, value V, now time.Time, ttl time.Duration) (numEvicted uint32) {
	// Treat zero TTLs as 100 years in the future in order to allow individual
	// elements to effectively have expiration disabled without needing to store
	// an additional byte in every element.
	if ttl == 0 {
		const oneHundredYears = time.Hour * 24 * 365 * 100
		ttl = oneHundredYears
	}

	// When the entry already exists move it to the front of the list thereby
	// marking it most recently used.
	if elem, exists := m.items[key]; exists {
		elem.value = value
		elem.expiresAfter = now.Add(ttl).UnixNano()
		m.moveElemToFront(elem)
		return 0
	}

	// Evict the least recently used entry (back of the list) if the new entry
	// would exceed the size limit.  Also reuse the node so a new one doesn't
	// have to be allocated.
	if uint32(len(m.items))+1 > m.limit {
		// Evict least recently used item.
		elem := m.root.prev
		delete(m.items, elem.key)

		// Reuse the list element of the item that was just evicted for the new
		// item.
		elem.key = key
		elem.value = value
		elem.expiresAfter = now.Add(ttl).UnixNano()
		m.moveElemToFront(elem)
		m.items[key] = elem
		return 1
	}

	// The limit hasn't been reached yet, so just add the new item.  Reuse old
	// list elements when possible.
	var elem *element[K, V]
	if len(m.elems) > 0 {
		finalCacheIdx := len(m.elems) - 1
		elem = m.elems[finalCacheIdx]
		m.elems = m.elems[0:finalCacheIdx]
		elem.key = key
		elem.value = value
	} else {
		elem = &element[K, V]{key: key, value: value}
	}
	elem.expiresAfter = now.Add(ttl).UnixNano()
	m.insertElemFront(elem)
	m.items[key] = elem
	return 0
}

// Put either adds the passed key/value pair when an item for that key does not
// already exist or updates the existing item for the given key to the passed
// value and arranges for the item to expire after the configured default TTL
// (if any).  The associated item becomes the most recently used item.
//
// The least recently used item is evicted when adding a new item would exceed
// the max limit.
//
// It returns the number of evicted items which includes any items that were
// evicted due to being marked expired.
//
// See [PutWithTTL] for a variant that allows setting a specific TTL for the
// item.
//
// This function is safe for concurrent access.
func (m *Map[K, V]) Put(key K, value V) (numEvicted uint32) {
	defer m.mtx.Unlock()
	m.mtx.Lock()

	// Nothing can be added when the limit is zero.
	if m.limit == 0 {
		return 0
	}

	// Scan through the items and remove any that are expired when expiration is
	// enabled and the scan interval has elapsed.  This is done for efficiency
	// so the scan only happens periodically instead of on every put.
	now := m.nowFn()
	if m.probablyHasTimeouts && now.After(m.nextExpireScan) {
		numEvicted = m.removeExpired(now)
	}

	numEvicted += m.put(key, value, now, m.ttl)
	return numEvicted
}

// PutWithTTL either adds the passed key/value pair when an item for that key
// does not already exist or updates the existing item for the given key to the
// passed value and arranges for the item to expire after the provided time to
// live (TTL).  The associated item becomes the most recently used item.
//
// The least recently used item is evicted when adding a new item would exceed
// the max limit.
//
// A TTL of zero will disable expiration for the item.  This can be useful to
// disable the expiration for specific items when the map was configured with a
// default expiration TTL.
//
// It returns the number of evicted items which includes any items that were
// evicted due to being marked expired.
//
// This function is safe for concurrent access.
func (m *Map[K, V]) PutWithTTL(key K, value V, ttl time.Duration) (numEvicted uint32) {
	defer m.mtx.Unlock()
	m.mtx.Lock()

	// Nothing can be added when the limit is zero.
	if m.limit == 0 {
		return 0
	}

	// Enable item expiration when not already done and the passed TTL is
	// non-zero.  Note that this being true also implies there is no default TTL
	// configured since item expiration would already be marked active in that
	// case.
	//
	// Otherwise, scan through the items and remove any that are expired when
	// item expiration was already enabled and the scan interval has elapsed.
	// This is done for efficiency so the scan only happens periodically instead
	// of on every put.
	now := m.nowFn()
	if !m.probablyHasTimeouts && ttl > 0 {
		m.probablyHasTimeouts = true
		m.nextExpireScan = now.Add(expireScanInterval)
	} else if m.probablyHasTimeouts && now.After(m.nextExpireScan) {
		numEvicted = m.removeExpired(now)
	}

	numEvicted += m.put(key, value, now, ttl)
	return numEvicted
}

// Get returns the value associated with the passed key if it is a member and
// modifies its priority to be the most recently used item.  The expiration time
// for the item is not modified.
//
// The boolean return value indicates whether or not the item lookup was
// successful.
//
// See [Peek] for a variant that does not modify the priority or update the hit
// ratio.
//
// This function is safe for concurrent access.
func (m *Map[K, V]) Get(key K) (V, bool) {
	defer m.mtx.Unlock()
	m.mtx.Lock()

	elem, exists := m.items[key]
	if !exists || m.isElemExpired(elem, m.nowFn()) {
		m.misses++
		return *new(V), false
	}

	m.hits++
	m.moveElemToFront(elem)
	return elem.value, true
}

// Exists returns whether or not the passed item is a member.  The priority and
// expiration time for the item are not modified.  It does not affect the hit
// ratio.
//
// This function is safe for concurrent access.
func (m *Map[K, V]) Exists(key K) bool {
	m.mtx.RLock()
	elem, exists := m.items[key]
	exists = exists && !m.isElemExpired(elem, m.nowFn())
	m.mtx.RUnlock()
	return exists
}

// Peek returns the associated value of the passed key if it is a member without
// modifying any priority or the hit ratio.  The boolean return value indicates
// whether or not the item lookup is successful.
//
// This function is safe for concurrent access.
func (m *Map[K, V]) Peek(key K) (V, bool) {
	defer m.mtx.RUnlock()
	m.mtx.RLock()

	elem, exists := m.items[key]
	if !exists || m.isElemExpired(elem, m.nowFn()) {
		return *new(V), false
	}

	return elem.value, true
}

// Len returns the number of items in the map.
//
// This function is safe for concurrent access.
func (m *Map[K, V]) Len() uint32 {
	m.mtx.RLock()
	numItems := uint32(len(m.items))
	m.mtx.RUnlock()
	return numItems
}

// Delete deletes the item associated with the passed key if it exists.
//
// This function is safe for concurrent access.
func (m *Map[K, V]) Delete(key K) {
	m.mtx.Lock()
	if elem, exists := m.items[key]; exists {
		m.deleteElem(key, elem)
	}
	m.mtx.Unlock()
}

// EvictExpiredNow immediately removes all items that are marked expired without
// waiting for the next expiration scan.  It returns the number of items that
// were removed.
//
// It is expected that most callers will typically use the automatic lazy
// expiration behavior and thus have no need to call this method.  However, it
// is provided in case more deterministic expiration is required.
func (m *Map[K, V]) EvictExpiredNow() (numEvicted uint32) {
	defer m.mtx.Unlock()
	m.mtx.Lock()

	if !m.probablyHasTimeouts {
		return 0
	}

	numEvicted = m.removeExpired(m.nowFn())
	return numEvicted
}

// Clear removes all items and resets the hit ratio.
//
// This function is safe for concurrent access.
func (m *Map[K, V]) Clear() {
	m.mtx.Lock()
	for k, elem := range m.items {
		m.deleteElem(k, elem)
	}
	m.root.prev = &m.root
	m.root.next = &m.root
	m.probablyHasTimeouts = m.ttl > 0
	m.nextExpireScan = m.nowFn().Add(expireScanInterval)
	m.hits = 0
	m.misses = 0
	m.mtx.Unlock()
}

// Keys returns a slice of unexpired keys ordered from least recently used to
// most recently used.  The priority and expiration times for the items are not
// modified.
//
// This function is safe for concurrent access.
func (m *Map[K, V]) Keys() []K {
	defer m.mtx.RUnlock()
	m.mtx.RLock()

	numItems := len(m.items)
	if numItems == 0 {
		return nil
	}

	now := m.nowFn()
	keys := make([]K, 0, numItems)
	for elem := m.root.prev; elem != &m.root; elem = elem.prev {
		if !m.isElemExpired(elem, now) {
			keys = append(keys, elem.key)
		}
	}
	return keys
}

// Values returns a slice of unexpired values ordered from least recently used
// to most recently used.  The priority and expiration times for the items are
// not modified.
//
// This function is safe for concurrent access.
func (m *Map[K, V]) Values() []V {
	defer m.mtx.RUnlock()
	m.mtx.RLock()

	numItems := len(m.items)
	if numItems == 0 {
		return nil
	}

	now := m.nowFn()
	values := make([]V, 0, numItems)
	for elem := m.root.prev; elem != &m.root; elem = elem.prev {
		if !m.isElemExpired(elem, now) {
			values = append(values, elem.value)
		}
	}
	return values
}

// HitRatio returns the percentage of lookups via [Get] that resulted in a
// successful hit.
//
// This function is safe for concurrent access.
func (m *Map[K, V]) HitRatio() float64 {
	m.mtx.RLock()
	hits, misses := m.hits, m.misses
	m.mtx.RUnlock()

	totalLookups := hits + misses
	if totalLookups == 0 {
		return 100
	}

	return float64(m.hits) / float64(totalLookups) * 100
}
