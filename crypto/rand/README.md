rand
====

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/crypto/rand)

## Overview

Package `rand` implements a fast userspace cryptographically secure pseudorandom
number generator (CSPRNG) that is periodically reseeded with entropy obtained
from `crypto/rand`.  The PRNG can be used to obtain random bytes as well as
generating uniformly-distributed integers in a full or limited range.  It also
provides additional convenience functions for common related tasks such as
obtaining uniformly-distributed `time.Duration`s and randomizing the order of
all elements in a slice without bias.

The default global PRNG will never panic after package init and is safe for
concurrent access.  Additional PRNGs which avoid the locking overhead can be
created by calling `NewPRNG`.

## Statistical Test Quality Assessment Results

The quality of the random number generation provided by this implementation has
been verified against statistical tests from the following test suites:

- [Dieharder: A Random Number Test Suite](https://webhome.phy.duke.edu/~rgb/General/dieharder.php)
- [NIST Statistical Test Suite (STS)](https://csrc.nist.rip/Projects/Random-Bit-Generation/Documentation-and-Software)
- [AIS 31 / BSI suite procedure A (tests T0 and T1-T5)](https://www.bsi.bund.de/SharedDocs/Downloads/DE/BSI/Zertifizierung/Interpretationen/AIS_31_testsuit_zip.zip)
- [NIST Entropy Assessment (EA) Test Suite](https://github.com/usnistgov/SP800-90B_EntropyAssessment)

## Benchmarks

The following results demonstrate the performance of most provided operations.
The benchmarks are from a Ryzen 7 5800X3D processor on Linux and are the result
of feeding benchstat 10 iterations of each.

Operation       | Time / Op   | Allocs / Op
----------------|-------------|------------
Read (4b)       | 22.0ns ± 1% | 0
Read (8b)       | 28.4ns ± 1% | 0
Read (32b)      | 68.5ns ± 1% | 0
Read (512b)     |  709ns ± 1% | 0
Read (1KiB)     | 1.38µs ± 1% | 0
Read (4KiB)     | 5.41µs ± 1% | 0
ReadPRNG (4b)   | 18.0ns ± 1% | 0
ReadPRNG (8b)   | 24.2ns ± 1% | 0
ReadPRNG (32b)  | 61.3ns ± 2% | 0
ReadPRNG (512b) |  684ns ± 0% | 0
ReadPRNG (1KiB) | 1.35µs ± 0% | 0
ReadPRNG (4KiB) | 5.39µs ± 1% | 0
Int32N          | 32.4ns ± 3% | 0
Uint32N         | 32.7ns ± 2% | 0
Int64N          | 31.2ns ± 2% | 0
Uint64N         | 31.2ns ± 2% | 0
Duration        | 33.8ns ±12% | 0
ShuffleSlice    | 28.0ns ± 1% | 0

## Read Performance Comparison Vs Standard Libary

The following benchmark results demonstrate the performance of reading random
bytes as compared to standard library `crypto/rand`.  The benchmarks are from a
Ryzen 7 5800X3D processor on Linux and are the result of feeding benchstat 10
iterations of each.

Operation     | `stdlib` Time / Op | `dcrd` Time / Op | Delta vs `stdlib`
--------------|--------------------|------------------|------------------
Read (4b)     |     470ns ± 7%     |     22ns ± 1%    | -95.32%
Read (8b)     |     447ns ± 1%     |     28ns ± 1%    | -93.65%
Read (32b)    |     447ns ± 1%     |     68ns ± 1%    | -84.67%
Read (512b)   |    1.72µs ± 6%     |   0.71µs ± 1%    | -58.78%
Read (1KiB)   |    2.89µs ± 1%     |   1.38µs ± 1%    | -52.09%
Read (4KiB)   |    10.5µs ± 2%     |    5.4µs ± 1%    | -48.37%

## Installation and Updating

This package is part of the `github.com/decred/dcrd/crypto/rand` module.  Use
the standard go tooling for working with modules to incorporate it.

## License

Package rand is licensed under the [copyfree](http://copyfree.org) ISC License.
