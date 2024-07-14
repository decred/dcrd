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

The package detects hardware support and arranges for the exported `Blocks`
function to automatically use the fastest available supported hardware
extensions that are not disabled.

## Tests and Benchmarks

The package also provides full tests for all implementations as well as
benchmarks.

## Disabling Assembler Optimizations

The `purego` build tag may be used to disable all assembly code.

Additionally, when built normally without the `purego` build tag, the assembly
optimizations for each of the supported vector extensions can individually be
disabled at runtime by setting the following environment variables to `1`.

* `BLAKE256_DISABLE_AVX=1`: Disable Advanced Vector Extensions (AVX) optimizations
* `BLAKE256_DISABLE_SSE41=1`: Disable Streaming SIMD Extensions 4.1 (SSE4.1) optimizations
* `BLAKE256_DISABLE_SSE2=1`: Disable Streaming SIMD Extensions 2 (SSE2) optimizations

## Installation and Updating

This package is internal and therefore is neither directly installed nor needs
to be manually updated.

## License

Package compress is licensed under the [copyfree](http://copyfree.org) ISC
License.
