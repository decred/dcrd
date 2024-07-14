blake256
========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/crypto/blake256)

## Overview

Package `blake256` implements the [BLAKE-256 and BLAKE-224 cryptographic hash
functions](https://www.aumasson.jp/blake/blake.pdf) (SHA-3 candidate) in pure
Go.

It provides an API that enables zero allocations and the ability to save and
restore the intermediate state (also often called the midstate).  The design
philosophy has a strong on emphasis correctness, readability, and efficiency
while also aiming to provide an ergonomic API.

In addition to the zero allocation API, it also implements the standard library
interfaces `hash.Hash`, `encoding.BinaryMarshaler`, and
`encoding.BinaryUnmarshaler` for callers that are not as concerned about
avoiding allocations.  No dependencies beyond the standard library are required.

A full suite of tests with 100% branch coverage and benchmarks are provided to
help ensure proper functionality and analyze performance characteristics.

The core assembly code to take advantage of the `amd64` SIMD vector extensions
is generated with Go via [avo](https://github.com/mmcloughlin/avo).

[Show me the benchmarks already](#benchmarks)!

## Hashing Data

The simplest way to hash data that is already serialized into bytes is via the
global `Sum224` (BLAKE-224) or `Sum256` (BLAKE-256) functions.

However, since hashing typically involves writing various pieces of information
that aren't already serialized, this package provides `NewHasher224` (BLAKE-224)
and `NewHasher256` (BLAKE-256) (and their respective variants `NewHasher224Salt`
and `NewHasher256Salt` that accept salt).

These methods return rolling hasher instances that support writing an arbitrary
amount of data along with several convenience methods for writing various data
types in either big endian or little endian.  For example, `WriteString` adds a
string encoded as its UTF-8 byte sequence to the rolling hash and
`WriteUint64BE` adds an unsigned 64-bit integer encoded as an 8-byte big-endian
byte sequence to the rolling hash.

The hash is then obtained via the `Sum224` (BLAKE-224) or `Sum256` (BLAKE-256)
method on the respective hasher instance.

## Saving and Resuming Intermediate States

Many applications involve hashing data that always starts with the same sequence
of bytes (aka a shared prefix).  Whenever that prefix is larger than the block
size (`BlockSize`), or it is otherwise costly to generate and serialize, it is
typically more efficient to save the intermediate state (midstate) after writing
the shared prefix so that all future hashes can resume from that midstate and
thereby avoid redoing work.

To that end, the aforementioned rolling hasher instances support being copied to
save and restore the current midstate within the same process.

Alternatively, when a simple copy of the instance is not possible, such as when
the midstate is needed among multiple processes, perhaps on entirely different
hardware, it can be serialized via `SaveState` and restored via
`UnmarshalBinary`.  Note that there is necessarily additional overhead involved
with serializing and deserializing the intermediate state, so callers should be
sure to compare that overhead with rehashing the shared data to see which
approach yields better results for their particular application.

## Hashing With Salt

This implementation also provides `NewHasher224Salt` (BLAKE-224) and
`NewHasher256Salt` (BLAKE-256) which accept a 16-byte salt input as described by
the specification.  Hashing with distinct salts effectively provides an
efficient method to hash with different functions while using the same
underlying algorithm.  The salted variants behave exactly the same as the normal
unsalted variants described throughout the documentation.

## Benchmarks

The following benchmarks are from a Ryzen 7 5800X3D processor on Linux and are
the result of feeding `benchstat` 10 iterations of each.  Benchmarks for both
BLAKE-224 and BLAKE-256 are provided.  They are essentialy identical (within the
margin of error) as expected since the only notable difference as it pertains to
performance is that the final output is 4 bytes shorter.

### BLAKE-256 Hashing Benchmarks

The following results demonstrate the performance of hashing various amounts of
data for both small and larger inputs with the `Sum256` method.

Operation        |   Pure Go
-----------------|-------------
`Sum256` (32b)   | 168MB/s ± 1%
`Sum256` (64b)   | 187MB/s ± 0%
`Sum256` (1KiB)  | 378MB/s ± 1%
`Sum256` (8KiB)  | 405MB/s ± 1%
`Sum256` (16KiB) | 402MB/s ± 1%

Operation        |   Pure Go   | Allocs / Op
-----------------|-------------|------------
`Sum256` (32b)   | 190ns ± 1%  | 0
`Sum256` (64b)   | 342ns ± 0%  | 0
`Sum256` (1KiB)  | 2.71µs ± 1% | 0
`Sum256` (8KiB)  | 20.2µs ± 1% | 0
`Sum256` (16KiB) | 40.8µs ± 1% | 0

### BLAKE-224 Hashing Benchmarks

The following results demonstrate the performance of hashing various amounts of
data for both small and larger inputs with the `Sum224` method.

Operation        |   Pure Go
-----------------|-------------
`Sum224` (32b)   | 171MB/s ± 1%
`Sum224` (64b)   | 187MB/s ± 2%
`Sum224` (1KiB)  | 378MB/s ± 1%
`Sum224` (8KiB)  | 404MB/s ± 1%
`Sum224` (16KiB) | 401MB/s ± 1%

Operation        |   Pure Go   | Allocs / Op
-----------------|-------------|------------
`Sum224` (32b)   |  187ns ± 1% | 0
`Sum224` (64b)   |  342ns ± 2% | 0
`Sum224` (1KiB)  | 2.71µs ± 1% | 0
`Sum224` (8KiB)  | 20.3µs ± 1% | 0
`Sum224` (16KiB) | 40.9µs ± 1% | 0

### State Serialization Benchmarks

The following results demonstrate the performance of serializing the
intermediate state for both BLAKE-224 and BLAKE-256 using the zero-alloc
`SaveState` method versus the standard library `encoding.MarshalBinary`
interface.

 Metric     | `MarshalBinary` | `SaveState` | Delta
------------|-----------------|-------------|---------------------------
Time / Op   | 40.6ns ± 1%     | 16.0ns ± 0% | -60.60% (p=0.000 n=10+10)
Allocs / Op | 1               | 0           | -100.00% (p=0.000 n=10+10)

## Installation and Updating

This package is part of the `github.com/decred/dcrd/crypto/blake256` module.
Use the standard go tooling for working with modules to incorporate it.

## License

Package blake256 is licensed under the [copyfree](http://copyfree.org) ISC
License.
