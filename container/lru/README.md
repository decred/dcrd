lru
===

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/container/lru)

Package lru provides generic type and concurrent safe LRU data structures with
near O(1) perf and optional time-based expiration support.

## Overview

A basic least recently used (LRU) cache is a cache that holds a limited number
of items with an eviction policy such that when the capacity of the cache is
exceeded, the least recently used item is automatically removed when inserting a
new item.  The meaning of `used` in this implementation is accessing items via
lookups, adding items, and updating existing items.

This package provides two similar full-featured implementations named `Set` and
`Map` which implement the aforementioned LRU cache semantics in addition to
providing optional configurable per-item expiration timeouts via a time to live
(TTL) mechanism.

The primary difference between the two is that `Set` is tailored towards use
cases that involve storing a distinct collection of items with existence
testing, while `Map` is aimed at use cases that require caching and retrieving
values by key.

Both implementations are based on type safe generics and are safe for use in
multi-threaded (concurrent) workloads.

## Item Expiration

Both `Set` and `Map` support optional configurable default TTLs for item
expiration as well as provide the option to override the default TTL on a
per-item basis.

Expiration TTLs are relative to the time an item is added or updated via `Put`
or `PutWithTTL`.  Accessing items does not extend the TTL.

An efficient lazy removal scheme is used such that expired items are
periodically removed when items are added or updated via `Put` or `PutWithTTL`.
In other words, items may physically remain in the cache past their expiration
until the next expiration scan is triggered, however, they will no longer
publicly appear as members of the cache.  This approach allows for efficient
amortized removal of expired items without the need for additional background
tasks or timers.

While it is expected that most callers will typically use the automatic lazy
expiration behavior, callers may optionally use `EvictExpiredNow` to immediately
remove all items that are marked expired without waiting for the next expiration
scan in cases where more deterministic expiration is required.

## Creating Instances and Configuring TTLs

`NewSet` and `NewMap` create instances of the respective type that impose no
item expiration by default.  Alternatively, `NewSetWithDefaultTTL` and
`NewMapWithDefaultTTL` create instances with the specified TTL applied to all
items by default.

In all cases, `Put` adds or updates an item per the default TTL specified when
creating the instance while `PutWithTTL` allows overriding the default TTL of
the individual item.

Invoking `PutWithTTL` with a TTL of zero will disable expiration for the item.
This feature is useful to disable the expiration for specific items when the
instance was configured with a default expiration TTL.

## Accessing and Querying Items and Values

Typically, callers will want to make use of `Contains` to determine set
membership for `Set` and `Get` to access the value associated with a given key
for `Map`.  Both of these methods impose LRU semantics meaning they modify the
priority of the accessed items so they will be evicted after less recently used
items.  They also both affect the hit ratio.

However, it is sometimes useful to access information without affecting any
state.  With that in mind, `Exists`, `Peek`, `Keys`, `Values`, and `Items` are
also provided.  Since these methods do not update any state, the priority of the
items accessed via the methods is not modified and the hit ratio is unaffected.

## Manually Removing Items

`Delete` may be used to manually remove individual items at any time and `Clear`
completely empties all items.

## Hit Ratio Reporting

A hit ratio is the percentage of overall lookups that resulted in a successful
hit and is often a useful metric to measure cache effectiveness.

The hit ratio for a given instance may be obtained via `HitRatio`.

## Benchmarks

The following results demonstrate the performance of the primary map and set
operations.  The benchmarks are from a Ryzen 7 5800X3D processor.

Operation                  | Time / Op   | Allocs / Op
---------------------------|-------------|------------
MapPut (no expiration)     | 108ns ± 2%  | 0
MapPut (with expiration)   | 110ns ± 1%  | 0
MapGet                     | 40.9ns ± 1% | 0
MapExists                  | 34.2ns ± 2% | 0
MapPeek                    | 37.6ns ± 0% | 0
SetPut (no expiration)     | 109ns ± 2%  | 0
SetPut (with expiration)   | 110ns ± 1%  | 0
SetContains                | 41.4ns ± 3% | 0
SetExists                  | 34.7ns ± 1% | 0

## Installation and Updating

This package is part of the `github.com/decred/dcrd/container/lru`
module.  Use the standard go tooling for working with modules to incorporate it.

## Examples

* [Basic Map Usage](https://pkg.go.dev/github.com/decred/dcrd/container/lru#example-package-BasicMapUsage)
  Demonstrates creating a new map instance, inserting items into the map,
  existence checking, looking up an item, causing an eviction of the
  least recently used item, and removing an item.
* [Explicit Map Expiration](https://pkg.go.dev/github.com/decred/dcrd/container/lru#example-package-ExplicitMapExpiration)
  Demonstrates creating a new map instance with time-based expiration, inserting
  items into it, and manually triggering removal of expired items.
* [Basic Set Usage](https://pkg.go.dev/github.com/decred/dcrd/container/lru#example-package-BasicSetUsage)
  Demonstrates creating a new set instance, inserting items into the set,
  checking set containment, causing an eviction of the least recently used item,
  and removing an item.
* [Per-Item Expiration](https://pkg.go.dev/github.com/decred/dcrd/container/lru#example-Set.PutWithTTL)
  Demonstrates per-item expiration by creating a new set instance with no
  time-based expiration, inserting items into it, updating one of the items to
  have a timeout, and manually triggering removal of expired item.

## License

Package lru is licensed under the [copyfree](http://copyfree.org) ISC License.
