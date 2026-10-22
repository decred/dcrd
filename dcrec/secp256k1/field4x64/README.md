field4x64
=========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/dcrec/secp256k1/v4/field4x64)

Package field4x64 provides highly optimized pure-Go arithmetic over the
secp256k1 finite field using a 4x64 representation.

It is designed for correctness, performance, security, and high assurance
through specialized arithmetic, constant time engineering, formal verification
of critical arithmetic routines, differential testing, and multiple
complementary testing techniques.

This package exposes the underlying implementation directly and is primarily
intended for specialized use cases.  Most consumers should prefer the
`secp256k1` package, which provides the stable public `FieldVal` type and
automatically selects the appropriate backend implementation for the target
architecture.

## Design

The implementation represents field elements with four 64-bit limbs using a
canonical 256-bit representation in the range [0, p-1].  Arithmetic is
specialized for the secp256k1 prime and uses optimized pure-Go implementations
together with architecture-specific assembly where available.

Unlike the 10x26 backend, this representation fully reduces every arithmetic
operation.  Consequently, field elements are always maintained in canonical form
and callers are not required to manually track normalization or magnitude.

The semantics simplify both the API and implementation reasoning while remaining
highly efficient on modern 64-bit processors.

## Assurance

This package emphasizes correctness and implementation assurance through
multiple complementary validation techniques.

Key implementation characteristics include:

- Highly optimized arithmetic specialized specifically for the secp256k1 field
- Constant time implementations suitable for secret-dependent operations
- Formal verification of critical arithmetic operations using the Z3 theorem
  prover
- Differential testing against independent secp256k1 implementations
- Deterministic test vectors
- Property-based testing
- Randomized-input testing
- Coverage-guided fuzz testing
- Manual review of security-critical implementation details

## Disabling Assembler Optimizations

The `purego` build tag may be used to disable all assembly code.

## Installation and Updating

This package is part of the `github.com/decred/dcrd/dcrec/secp256k1/v4` module.
Use the standard go tooling for working with modules to incorporate it.

## License

Package field4x64 is licensed under the [copyfree](http://copyfree.org) ISC
License.
