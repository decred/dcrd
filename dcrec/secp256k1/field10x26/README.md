field10x26
==========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/dcrec/secp256k1/v4/field10x26)

Package field10x26 provides highly optimized pure-Go arithmetic over the
secp256k1 finite field using a 10x26 representation.

It is designed for correctness, performance, security, and high assurance
through specialized arithmetic, constant time engineering, differential testing,
and multiple complementary testing techniques.

This package exposes the underlying implementation directly and is primarily
intended for specialized use cases.  Most consumers should prefer the
`secp256k1` package, which provides the stable public `FieldVal` type and
automatically selects the appropriate backend implementation for the target
architecture.

## Design

The implementation represents field elements using ten base-2^26 limbs with
carefully bounded intermediate overflow between normalizations.  This approach
minimizes carry propagation during common arithmetic operations and enables
efficient constant-time implementations.

Unlike implementations that always maintain canonical field elements, this
representation intentionally permits bounded overflow between normalizations.
Consequently, callers are responsible for observing the documented
normalization, magnitude, and operation preconditions described by the `Element`
type.

The semantics are a deliberate design choice that eliminates unnecessary work
from performance-critical arithmetic by allowing callers to normalize only when
required.

## Assurance

This package emphasizes correctness and implementation assurance through
multiple complementary validation techniques.

Key implementation characteristics include:

- Highly optimized arithmetic specialized specifically for the secp256k1 field
- Constant time implementation suitable for secret-dependent operations
- Differential testing against independent secp256k1 implementations
- Deterministic test vectors
- Property-based testing
- Randomized-input testing
- Coverage-guided fuzz testing
- Manual review of security-critical implementation details

## Installation and Updating

This package is part of the `github.com/decred/dcrd/dcrec/secp256k1/v4` module.
Use the standard go tooling for working with modules to incorporate it.

## License

Package field10x26 is licensed under the [copyfree](http://copyfree.org) ISC
License.
