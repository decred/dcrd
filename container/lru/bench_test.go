// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package lru

import (
	"testing"
	"time"
)

// makeBenchmarkVals creates a slice of test values to use when benchmarking.
func makeBenchmarkVals(numItems uint32) []uint64 {
	vals := make([]uint64, 0, numItems)
	for i := uint32(0); i < numItems; i++ {
		vals = append(vals, uint64(i))
	}
	return vals
}

// BenchmarkMapPutNoExp performs a basic benchmark of adding items to a map with
// eviction and no expiration.
func BenchmarkMapPutNoExp(b *testing.B) {
	// Create a bunch of values.
	b.StopTimer()
	const numItems = 100000
	vals := makeBenchmarkVals(numItems)
	b.StartTimer()

	// Benchmark the add plus eviction code.
	const limit = 20000
	m := NewMap[uint64, uint64](limit)
	for i := 0; i < b.N; i++ {
		m.Put(vals[i%numItems], vals[i%numItems])
	}
}

// BenchmarkMapPutWithExp performs a basic benchmark of adding items to a map
// with eviction and expiration.
func BenchmarkMapPutWithExp(b *testing.B) {
	// Create a bunch of values.
	b.StopTimer()
	const numItems = 100000
	vals := makeBenchmarkVals(numItems)
	b.StartTimer()

	// Benchmark the add plus eviction code.
	const limit = 20000
	const ttl = 60 * time.Second
	m := NewMapWithDefaultTTL[uint64, uint64](limit, ttl)
	for i := 0; i < b.N; i++ {
		m.Put(vals[i%numItems], vals[i%numItems])
	}
}

// BenchmarkMapGet performs a basic benchmark of fetching items from a map.
func BenchmarkMapGet(b *testing.B) {
	// Create a bunch of values.
	b.StopTimer()
	const numItems = 100000
	vals := makeBenchmarkVals(numItems)

	// Create and populate the map.
	m := NewMap[uint64, uint64](numItems)
	for i := 0; i < numItems; i++ {
		m.Put(vals[i], vals[i])
	}
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		m.Get(vals[i%numItems])
	}
}

// BenchmarkMapExists performs a basic benchmark of item existence testing in
// a map.
func BenchmarkMapExists(b *testing.B) {
	// Create a bunch of values.
	b.StopTimer()
	const numItems = 100000
	vals := makeBenchmarkVals(numItems)

	// Create and populate the map.
	m := NewMap[uint64, uint64](numItems)
	for i := 0; i < numItems; i++ {
		m.Put(vals[i], vals[i])
	}
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		m.Exists(vals[i%numItems])
	}
}

// BenchmarkMapPeek performs a basic benchmark of peeking at an item in a map.
func BenchmarkMapPeek(b *testing.B) {
	// Create a bunch of values.
	b.StopTimer()
	const numItems = 100000
	vals := makeBenchmarkVals(numItems)

	// Create and populate the map.
	m := NewMap[uint64, uint64](numItems)
	for i := 0; i < numItems; i++ {
		m.Put(vals[i], vals[i])
	}
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		m.Peek(vals[i%numItems])
	}
}

// BenchmarkSetPutNoExp performs a basic benchmark of adding items to a set with
// eviction and no expiration.
func BenchmarkSetPutNoExp(b *testing.B) {
	// Create a bunch of values.
	b.StopTimer()
	const numItems = 100000
	vals := makeBenchmarkVals(numItems)
	b.StartTimer()

	// Benchmark the add plus eviction code.
	const limit = 20000
	c := NewSet[uint64](limit)
	for i := 0; i < b.N; i++ {
		c.Put(vals[i%numItems])
	}
}

// BenchmarkSetPutWithExp performs a basic benchmark of adding items to a set
// with eviction and expiration.
func BenchmarkSetPutWithExp(b *testing.B) {
	// Create a bunch of values.
	b.StopTimer()
	const numItems = 100000
	vals := makeBenchmarkVals(numItems)
	b.StartTimer()

	// Benchmark the add plus eviction code.
	const limit = 20000
	const ttl = 60 * time.Second
	c := NewSetWithDefaultTTL[uint64](limit, ttl)
	for i := 0; i < b.N; i++ {
		c.Put(vals[i%numItems])
	}
}

// BenchmarkSetContains performs a basic benchmark of checking item containment
// in a set.
func BenchmarkSetContains(b *testing.B) {
	// Create a bunch of values.
	b.StopTimer()
	const numItems = 100000
	vals := makeBenchmarkVals(numItems)

	// Create and populate the set.
	c := NewSet[uint64](numItems)
	for i := 0; i < numItems; i++ {
		c.Put(vals[i])
	}
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		c.Contains(vals[i%numItems])
	}
}

// BenchmarkSetExists performs a basic benchmark of checking item existence in a
// set.
func BenchmarkSetExists(b *testing.B) {
	// Create a bunch of values.
	b.StopTimer()
	const numItems = 100000
	vals := makeBenchmarkVals(numItems)

	// Create and populate the set.
	c := NewSet[uint64](numItems)
	for i := 0; i < numItems; i++ {
		c.Put(vals[i])
	}
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		c.Exists(vals[i%numItems])
	}
}
