arith
=====

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/dcrec/secp256k1/v4/internal/arith)

## Overview

This provides low-level constant-time primitives and modulus-agnostic arithmetic
shared by multiple implementations in this module.

See [internal/proofs/README.md](../proofs/README.md) for formal verification of
the 512-bit multiplication and 512-bit squaring.

## Installation and Updating

This package is internal and therefore is neither directly installed nor needs
to be manually updated.

## License

Package arith is licensed under the [copyfree](http://copyfree.org) ISC License.
