cpufeat
=======

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/dcrec/secp256k1/v4/internal/cpufeat)

Package cpufeat detects support for CPU features that are relevant to
architecture-specific optimizations throughout the `secp256k1` package.

## Design

The package intentionally provides a minimal abstraction consisting only of the
feature detection required by the `secp256k1` implementation.  Architecture-
specific detection logic is gated behind build constraints and unsupported
architectures simply report that no optional features are supported.

This organization allows optimized implementations to select the most efficient
available code path without duplicating processor detection logic throughout the
codebase and ensures portable fallback implementations remain available on all
supported platforms.

## Pure Go Builds

The `purego` build tag may be used to disable all assembly code in this package.
Since feature detection relies on assembly, all optional CPU features will
report as unsupported.

## Testing

It is possible to test implementations that require otherwise unavailable CPU
features by using software such as the [Intel Software Development
Emulator](https://www.intel.com/content/www/us/en/developer/articles/tool/software-development-emulator.html).

Some relevant flags for testing purposes with the Intel SDE are:

* BMI2:  `-hsw  Set chip-check and CPUID for Intel(R) Haswell CPU`
* ADX:   `-bdw  Set chip-check and CPUID for Intel(R) Broadwell CPU`

The package determines supported features during package initialization, so
tests should be run under the emulator rather than attempting to enable features
after startup.

## Installation and Updating

This package is internal and therefore is neither directly installed nor needs
to be manually updated.

## License

Package cpufeat is licensed under the [copyfree](http://copyfree.org) ISC
License.
