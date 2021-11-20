primitives
==========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/internal/staging/primitives)

## Package and Module Status

This package is currently a work in progress in the context of a larger refactor
and thus does not yet provide most of the things that are ultimately planned.
See https://github.com/decred/dcrd/issues/2786 for further details.

The intention is to create a containing `primitives` module that will be kept at
an experimental module version ("v0") until everything is stabilized to avoid
major module version churn in the mean time.

## Overview

This package ultimately aims to provide core data structures and functions for
working with several aspects of Decred consensus.

The provided functions fall into the following categories:

- Proof-of-work
  - Converting to and from the target difficulty bits representation
  - Calculating work values based on the target difficulty bits
  - Checking that a block hash satisfies a target difficulty and that the target
    difficulty is within a valid range
- Merkle root calculation
  - Calculation from individual leaf hashes

## Maintainer Note

Since the `primitives` module is heavily relied upon by consensus code, there
are some important aspects that must be kept in mind when working on this code:

- It must provide correctness guarantees and full test coverage
- Be extremely careful when making any changes to avoid breaking consensus
  - This often means existing code can't be changed without adding an internal
    flag to control behavior and introducing a new method with the new behavior
- Minimize the number of allocations as much as possible
- Opt for data structures that improve cache locality
- Keep a strong focus on providing efficient code
- Avoid external dependencies
  - Consensus code requires much stronger guarantees than typical code and
    consequently code that is not specifically designed with such rigid
    constraints will almost always eventually break things in subtle ways over
    time
- Do not change the API in a way that requires a new major semantic version
  - This is not entirely prohibited, but major module version bumps have severe
    ramifications on every consumer, and thus they should be an absolute last
    resort
- Take care when adding new methods to avoid method signatures that would
  require a major version bump due to a new major dependency version

## Installation and Updating

This package is internal and therefore is neither directly installed nor needs
to be manually updated.

## License

Package primitives is licensed under the [copyfree](http://copyfree.org) ISC
License.
