// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package lru

import (
	"time"
)

// Set provides a concurrency safe least recently used (LRU) set with nearly
// O(1) lookups, inserts, and deletions.  The set is limited to a maximum number
// of items with eviction of the least recently used entry when the limit is
// exceeded.
//
// It also supports optional item expiration after a configurable time to live
// (TTL) with periodic lazy removal.  [NewSet] provides no item expiration by
// default while [NewSetWithDefaultTTL] provides a configurable TTL that is
// applied to all items by default.  In both cases, callers may override the
// default TTL of individual items via [PutWithTTL].
//
// Expiration TTLs are relative to the time an item is added or updated via
// [Put] or [PutWithTTL].  Accessing items does not extend the TTL.
//
// An efficient lazy removal scheme is used such that expired items are
// periodically removed when items are added or updated via [Put] or
// [PutWithTTL].  In other words, expired items may physically remain in the set
// until the next expiration scan is triggered, however, they will no longer
// publicly appear as members of the set.  This approach allows for efficient
// amortized removal of expired items without the need for additional background
// tasks or timers.
//
// Callers may optionally use [EvictExpiredNow] to immediately remove all items
// that are marked expired without waiting for the next expiration scan in cases
// where more deterministic expiration is required.
//
// [NewSet] or [NewSetWithDefaultTTL] must be used to create a usable set since
// the zero value of this struct is not valid.
type Set[T comparable] struct {
	m *Map[T, struct{}]
}

// NewSet returns an initialized and empty LRU set where no items will expire by
// default.
//
// The provided limit is the maximum number of items the set will hold before it
// evicts least recently used items to make room for new items.
//
// See [NewSetWithDefaultTTL] for a variant that allows a configurable time to
// live to be applied to all items by default.
//
// See the documentation for [Set] for more details.
func NewSet[T comparable](limit uint32) *Set[T] {
	return &Set[T]{
		m: NewMap[T, struct{}](limit),
	}
}

// NewSetWithDefaultTTL returns an initialized and empty LRU set where the
// provided non-zero time to live (TTL) is applied to all items by default.
//
// A TTL of zero disables item expiration by default and is equivalent to
// [NewSet].
//
// See [NewSet] for a variant that does not expire any items by default.
//
// See the documentation for [Set] for more details.
func NewSetWithDefaultTTL[T comparable](limit uint32, ttl time.Duration) *Set[T] {
	return &Set[T]{
		m: NewMapWithDefaultTTL[T, struct{}](limit, ttl),
	}
}

// Put either adds the passed item when it does not already exist or refreshes
// the existing item and arranges for the item to expire after the configured
// default TTL (if any).  The item becomes the most recently used item.
//
// The least recently used item is evicted when adding a new item would exceed
// the max limit.
//
// It returns the number of evicted items which includes any items that were
// evicted due to being marked expired.
//
// See [PutWithTTL] for a variant that allows configuring a specific TTL for the
// item.
//
// This function is safe for concurrent access.
func (c *Set[T]) Put(item T) (numEvicted uint32) {
	return c.m.Put(item, struct{}{})
}

// PutWithTTL either adds the passed item when it does not already exist or
// refreshes the existing item and arranges for the item to expire after the
// provided time to live (TTL).  The item becomes the most recently used item.
//
// The least recently used item is evicted when adding a new item would exceed
// the max limit.
//
// A TTL of zero will disable expiration for the item.  This can be useful to
// disable the TTL for specific items when the set was configured with a default
// expiration TTL.
//
// It returns the number of evicted items which includes any items that were
// evicted due to being marked expired.
//
// This function is safe for concurrent access.
func (c *Set[T]) PutWithTTL(item T, ttl time.Duration) (numEvicted uint32) {
	return c.m.PutWithTTL(item, struct{}{}, ttl)
}

// Contains returns whether or not the passed item is a member and modifies its
// priority to be the most recently used item when it is.  The expiration time
// for the item is not modified.  The hit ratio is updated accordingly.
//
// See [Exists] for a variant that does not modify the priority of the item or
// update the hit ratio.
//
// This function is safe for concurrent access.
func (c *Set[T]) Contains(item T) bool {
	_, exists := c.m.Get(item)
	return exists
}

// Exists returns whether or not the passed item is a member.  The priority and
// expiration time for the item are not modified.  It does not affect the hit
// ratio.
//
// See [Contains] for a variant that modifies the priority of the item to be the
// most recently used item when it exists and updates the hit ratio accordingly.
//
// This function is safe for concurrent access.
func (c *Set[T]) Exists(item T) bool {
	return c.m.Exists(item)
}

// Len returns the number of items in the set.
//
// This function is safe for concurrent access.
func (c *Set[T]) Len() uint32 {
	return c.m.Len()
}

// Delete deletes the passed item if it exists.
//
// This function is safe for concurrent access.
func (c *Set[T]) Delete(item T) {
	c.m.Delete(item)
}

// EvictExpiredNow immediately removes all items that are marked expired without
// waiting for the next expiration scan.  It returns the number of items that
// were removed.
//
// It is expected that most callers will typically use the automatic lazy
// expiration behavior and thus have no need to call this method.  However, it
// is provided in case more deterministic expiration is required.
func (c *Set[T]) EvictExpiredNow() (numEvicted uint32) {
	return c.m.EvictExpiredNow()
}

// Clear removes all items and resets the hit ratio.
//
// This function is safe for concurrent access.
func (c *Set[T]) Clear() {
	c.m.Clear()
}

// Items returns a slice of unexpired items ordered from least recently used to
// most recently used.  The priority and expiration times for the items are not
// modified.
//
// This function is safe for concurrent access.
func (c *Set[T]) Items() []T {
	return c.m.Keys()
}

// HitRatio returns the percentage of lookups via [Contains] that resulted in a
// successful hit.
//
// This function is safe for concurrent access.
func (c *Set[T]) HitRatio() float64 {
	return c.m.HitRatio()
}
