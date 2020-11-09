// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package lru

import (
	"container/list"
	"sync"
)

// kv represents a key-value pair.
type kv struct {
	key   interface{}
	value interface{}
}

// KVCache provides a concurrency safe least-recently-used key/value cache with
// nearly O(1) lookups, inserts, and deletions.  The cache is limited to a
// maximum number of items with eviction for the oldest entry when the
// limit is exceeded.
//
// The NewKVCache function must be used to create a usable cache since the zero
// value of this struct is not valid.
type KVCache struct {
	mtx   sync.Mutex
	cache map[interface{}]*list.Element // nearly O(1) lookups
	list  *list.List                    // O(1) insert, update, delete
	limit uint
}

// Lookup returns the associated value of the passed key, if it is a member of
// the cache. Looking up an existing item makes it the most recently used item.
//
// This function is safe for concurrent access.
func (m *KVCache) Lookup(key interface{}) (interface{}, bool) {
	var value interface{}
	m.mtx.Lock()
	node, exists := m.cache[key]
	if exists {
		m.list.MoveToFront(node)
		pair := node.Value.(*kv)
		value = pair.value
	}
	m.mtx.Unlock()

	return value, exists
}

// Contains returns whether or not the passed key is a member of the cache.
// The associated item of the passed key if it exists becomes the most
// recently used item.
//
// This function is safe for concurrent access.
func (m *KVCache) Contains(key interface{}) bool {
	m.mtx.Lock()
	node, exists := m.cache[key]
	if exists {
		m.list.MoveToFront(node)
	}
	m.mtx.Unlock()
	return exists
}

// Add adds the passed k/v to the cache and handles eviction of the oldest pair
// if adding the new pair would exceed the max limit.  Adding an existing pair
// makes it the most recently used item.
//
// This function is safe for concurrent access.
func (m *KVCache) Add(key interface{}, value interface{}) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	// When the limit is zero, nothing can be added to the cache, so just
	// return.
	if m.limit == 0 {
		return
	}

	// When the k/v already exists update the value and move it to the
	// front of the list thereby marking it most recently used.
	if node, exists := m.cache[key]; exists {
		node.Value.(*kv).value = value
		m.list.MoveToFront(node)
		m.cache[key] = node
		return
	}

	// Evict the least recently used k/v (back of the list) if the new
	// k/v would exceed the size limit for the cache.  Also reuse the list
	// node so a new one doesn't have to be allocated.
	if uint(len(m.cache))+1 > m.limit {
		node := m.list.Back()
		lru := node.Value.(*kv)

		// Evict least recently used k/v.
		delete(m.cache, lru.key)

		// Reuse the list node of the k/v that was just evicted for the new
		// k/v.
		lru.key = key
		lru.value = value
		m.list.MoveToFront(node)
		m.cache[key] = node
		return
	}

	// The limit hasn't been reached yet, so just add the new k/v.
	node := m.list.PushFront(&kv{key: key, value: value})
	m.cache[key] = node
}

// Delete deletes the k/v associated with passed key from the cache
// (if it exists).
//
// This function is safe for concurrent access.
func (m *KVCache) Delete(key interface{}) {
	m.mtx.Lock()
	if node, exists := m.cache[key]; exists {
		m.list.Remove(node)
		delete(m.cache, key)
	}
	m.mtx.Unlock()
}

// NewKVCache returns an initialized and empty KV LRU cache.
// See the documentation for KV for more details.
func NewKVCache(limit uint) KVCache {
	return KVCache{
		cache: make(map[interface{}]*list.Element),
		list:  list.New(),
		limit: limit,
	}
}
