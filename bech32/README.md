bech32
==========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/bech32)

Package bech32 provides a Go implementation of the bech32 format specified in
[BIP 173](https://github.com/bitcoin/bips/blob/master/bip-0173.mediawiki).

Test vectors from BIP 173 are added to ensure compatibility with the BIP.

## Installation and Updating

This package is part of the `github.com/decred/dcrd/bech32` module.  Use the
standard go tooling for working with modules to incorporate it.

## Examples

* [Bech32 decode Example](https://pkg.go.dev/github.com/decred/dcrd/bech32#example-Decode)
  Demonstrates how to decode a bech32 encoded string.

* [Bech32 encode Example](https://pkg.go.dev/github.com/decred/dcrd/bech32#example-Encode)
  Demonstrates how to encode data into a bech32 string.

## License

Package bech32 is licensed under the [copyfree](http://copyfree.org) ISC
License.
