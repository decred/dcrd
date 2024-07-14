compress
========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/crypto/blake256/internal/compress)

## Overview

Package `compress` implements the BLAKE-224 and BLAKE-256 block compression
function.  It only provides a pure Go implementation currently, but it will be
updated to provided several specialized implementations that take advantage of
vector extensions in the future.

## Tests and Benchmarks

The package also provides full tests for all implementations as well as
benchmarks.

## Installation and Updating

This package is internal and therefore is neither directly installed nor needs
to be manually updated.

## License

Package compress is licensed under the [copyfree](http://copyfree.org) ISC
License.
