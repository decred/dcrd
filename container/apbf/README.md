apbf
====

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/container/apbf)

## Age-Partitioned Bloom Filter (APBF)

An [Age-Partitioned Bloom Filter](https://arxiv.org/pdf/2001.03147.pdf) is a
probabilistic data structure suitable for processing unbounded data streams
where more recent items are more significant than older ones and some false
positives are acceptable.  It has similar computational costs as traditional
Bloom filters and provides space efficiency that is competitive with the current
best-known, and more complex, Dictionary approaches.

Similar to classic Bloom filters, APBFs have a non-zero probability of false
positives that can be tuned via parameters and are free from false negatives up
to the capacity of the filter.  However, unlike classic Bloom filters, where the
false positive rate climbs as items are added until all queries are a false
positive, APBFs provide a configurable upper bound on the false positive rate
for an unbounded number of additions.

The unbounded property is achieved by adding and retiring disjoint segments over
time where each segment is a slice of a partitioned Bloom filter.  The slices
are conceptually aged over time as new items are added by shifting them and
discarding the oldest one.

An ideal use case for APBFs is efficiently deduplicating a continuous and
unbounded event stream with a tunable upper bound on the false positive rate.

### Additional Implementation Details

This implementation deviates from the original paper in at least the following
important ways:

- It uses Dillinger-Manolis enhanced double hashing instead of the more
  traditional Kirsch-Mitzenmacher double hashing since the latter imposes an
  observable accuracy limit on the filter while the former comes much closer to
  the theoretical limit of two-index fingerprinting
- Every filter is given a unique key for the internal hashing logic so each one
  will have a unique set of false positives and that key is automatically
  changed when the filter is manually reset by the caller
- Lemire fast reduction is used instead of standard modular reduction

## Choosing Parameters

The API provides two different mechanisms for creating an APBF with the desired properties:

- The `NewFilter` method

  This is the recommended approach for most applications. It is parameterized by
  the number of most-recently added items that must always return true and the
  target false positive rate to maintain.

  For most applications, the values chosen by this option will be perfectly
  acceptable, however, applications that desire greater control over the tuning
  can make use of the second method instead.

- The `NewFilterKL` method

  This provides applications with greater control over the tradeoff between
  overall filter size and computational speed.  It is parameterized by the
  number of most-recently added items that must always return true, the number
  slices which need consecutive matches, `K`, and the number of additional
  slices, `L`, to reach a desired target false positive rate to maintain.

  For convenience, the `go generate` command may be used in the code directory
  to generate a table of `K` and `L` combinations along with the false positive
  rate they maintain and average expected number of accesses for false queries
  to help select appropriate values.

## Accuracy Under Workloads

The following details the results of experimental validation of this
implementation with respect to false positive rate of filters with various
capacities and target false positive rates.

The methodology used for this analysis is to subject each filter to
`10*capacity` distinct additions and to probe each filter with
`capacity*(1/false positive rate)` distinct items that were never added.  For
example, in the case of a capacity of `1000` and target false positive rate of
`0.001`, ten thousand items are added and one million items that were never
added are probed.

It is also worth noting that this analysis uses the `NewFilter` method, as
described above, for the parameter selection.

Capacity | Target FP | Actual Observed FP
---------|-----------|-------------------
1000     |   0.1%    | 0.097%
10000    |   0.1%    | 0.099%
10000000 |   0.1%    | 0.1%
1000     |   1.0%    | 0.867%
10000    |   1.0%    | 0.862%
10000000 |   1.0%    | 0.857%
1000     |   2.0%    | 1.464%
10000    |   2.0%    | 1.451%
10000000 |   2.0%    | 1.461%
1000     |  10.0%    | 6.74%
10000    |  10.0%    | 6.96%
10000000 |  10.0%    | 7.024%

## Memory Usage

The total space (in bytes) used by a filter that can hold `n` items with a false
positive rate of `fpRate` is approximately `1.3n * -log(fpRate)`, where log is
base 10.

Here is a table of measured usage for some common values for convenience:

Capacity | Target FP | Size
---------|-----------|-----------
1000     |   1.0%    | 2.78 KiB
10000    |   1.0%    | 27.19 KiB
100000   |   1.0%    | 271.28 KiB
1000     |   0.1%    | 4.10 KiB
10000    |   0.1%    | 40.34 KiB
100000   |   0.1%    | 402.62 KiB

## Benchmarks

The following results demonstrate the performance of the primary filter
operations.  The benchmarks are from a Ryzen 7 1700 processor.

### `Add`

Capacity | Target FP |  Time / Op  | Allocs / Op
---------|-----------|-------------|------------
1000     | 0.1%      |  59ns ± 1%  | 0
1000     | 0.01%     |  69ns ± 2%  | 0
1000     | 0.001%    |  78ns ± 2%  | 0
100000   | 0.01%     |  80ns ± 1%  | 0
100000   | 0.0001%   | 110ns ± 1%  | 0
1000000  | 0.00001%  | 205ns ± 2%  | 0

### `Contains` (item matches filter, worst case)

Capacity | Target FP |  Time / Op  | Allocs / Op
---------|-----------|-------------|------------
1000     | 0.1%      |  69ns ± 2%  | 0
1000     | 0.01%     |  80ns ± 1%  | 0
1000     | 0.001%    |  89ns ± 1%  | 0
100000   | 0.01%     |  80ns ± 1%  | 0
100000   | 0.0001%   |  98ns ± 1%  | 0

### `Contains` (item does NOT match filter)

Capacity | Target FP |  Time / Op  | Allocs / Op
---------|-----------|-------------|------------
1000     | 0.1%      | 42.0ns ±26% | 0
1000     | 0.01%     | 37.7ns ±5%  | 0
1000     | 0.001%    | 37.0ns ±4%  | 0
100000   | 0.01%     | 37.6ns ±10% | 0
100000   | 0.0001%   | 36.3ns ±6%  | 0

## Installation and Updating

This package is part of the `github.com/decred/dcrd/container/apbf` module.  Use
the standard go tooling for working with modules to incorporate it.

## Examples

* [Basic Usage](https://pkg.go.dev/github.com/decred/dcrd/container/apbf#example-package-BasicUsage)  
  Demonstrates creating a new APBF, adding items to it up to the maximum
  capacity, querying the oldest item, then adding more items to it in order to
  cause the older items to be evicted.

## License

Package apbf is licensed under the [copyfree](http://copyfree.org) ISC License.
