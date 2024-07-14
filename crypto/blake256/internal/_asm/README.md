_asm
====

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/crypto/blake256/internal/_asm)

## Overview

This provides a separate internal module for generating the assembly code that
provides vector acceleration for the BLAKE-224 and BLAKE-256 block compression
function.

It uses Go to generate the assembly functions and related stubs to support SSE2,
SSE4.1, and AVX for `amd64` via [avo](https://github.com/mmcloughlin/avo).

The internal module ensures the specific version of avo used to generate the
code is pinned and therefore entirely reproducible without adding an otherwise
unnecessary dependency to the main public module.

## Generating Code

The assembly code can be generating by running the following command in this
directory:

```bash
$ GOWORK=off go generate
```

## Installation and Updating

This package is internal and therefore is neither directly installed nor needs
to be manually updated.

## License

Package _asm is licensed under the [copyfree](http://copyfree.org) ISC License.
