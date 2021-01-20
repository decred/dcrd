hdkeychain
==========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/hdkeychain/v3)

Package hdkeychain provides an API for Decred hierarchical deterministic
extended keys (based on BIP0032).

A comprehensive suite of tests is provided to ensure proper functionality.

## Feature Overview

- Full BIP0032 implementation
- Single type for private and public extended keys
- Convenient cryptographically secure seed generation
- Simple creation of master nodes
- Support for multi-layer derivation
- Easy serialization and deserialization for both private and public extended
  keys
- Support for custom networks by accepting a network parameters interface
- Allows obtaining the underlying serialized secp256k1 pubkeys and privkeys
  directly so they can either be used directly or optionally converted to the
  secp256k1 types which provide powerful tools for working with them to do
  things like sign transactions and generate payment scripts
- Uses the highly-optimized secp256k1 package
- Code examples including:
  - Generating a cryptographically secure random seed and deriving a master node
    from it
  - Default HD wallet layout as described by BIP0032
  - Audits use case as described by BIP0032
- Comprehensive test coverage including the BIP0032 test vectors
- Benchmarks

## Installation and Updating

This package is part of the `github.com/decred/dcrd/hdkeychain/v3` module.  Use
the standard go tooling for working with modules to incorporate it.

## Examples

* [NewMaster Example](https://pkg.go.dev/github.com/decred/dcrd/hdkeychain/v3#example-package-NewMaster)
  Demonstrates how to generate a cryptographically random seed then use it to
  create a new master node (extended key).
* [Default Wallet Layout Example](https://pkg.go.dev/github.com/decred/dcrd/hdkeychain/v3#example-package-DefaultWalletLayout)
  Demonstrates the default hierarchical deterministic wallet layout as described
  in BIP0032.
* [Audits Use Case Example](https://pkg.go.dev/github.com/decred/dcrd/hdkeychain/v3#example-package-Audits)
  Demonstrates the audits use case in BIP0032.

## License

Package hdkeychain is licensed under the [copyfree](http://copyfree.org) ISC
License.
