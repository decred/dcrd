hdkeychain
==========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/github.com/decred/dcrd/hdkeychain)

Package hdkeychain provides an API for Decred hierarchical deterministic
extended keys (based on BIP0032).

A comprehensive suite of tests is provided to ensure proper functionality.  See
`test_coverage.txt` for the gocov coverage report.  Alternatively, if you are
running a POSIX OS, you can run the `cov_report.sh` script for a real-time
report.

## Feature Overview

- Full BIP0032 implementation
- Single type for private and public extended keys
- Convenient cryptograpically secure seed generation
- Simple creation of master nodes
- Support for multi-layer derivation
- Easy serialization and deserialization for both private and public extended
  keys
- Support for custom networks by accepting a network parameters interface
- Obtaining the underlying EC pubkeys and EC privkeys ties in seamlessly with
  existing secp256k1 types which provide powerful tools for working with them to
  do things like sign transactions and generate payment scripts
- Uses the highly-optimized secp256k1 package
- Code examples including:
  - Generating a cryptographically secure random seed and deriving a master node
    from it
  - Default HD wallet layout as described by BIP0032
  - Audits use case as described by BIP0032
- Comprehensive test coverage including the BIP0032 test vectors
- Benchmarks

## Installation and Updating

```bash
$ go get -u github.com/decred/dcrd/hdkeychain
```

## Examples

* [NewMaster Example](https://godoc.org/github.com/decred/dcrd/hdkeychain#example-package--NewMaster)
  Demonstrates how to generate a cryptographically random seed then use it to
  create a new master node (extended key).
* [Default Wallet Layout Example](https://godoc.org/github.com/decred/dcrd/hdkeychain#example-package--DefaultWalletLayout)
  Demonstrates the default hierarchical deterministic wallet layout as described
  in BIP0032.
* [Audits Use Case Example](https://godoc.org/github.com/decred/dcrd/hdkeychain#example-package--Audits)
  Demonstrates the audits use case in BIP0032.

## License

Package hdkeychain is licensed under the [copyfree](http://copyfree.org) ISC
License.
