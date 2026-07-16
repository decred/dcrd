secp256k1
=========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/dcrec/secp256k1/v4)

Package secp256k1 provides an optimized pure-Go implementation of secp256k1
elliptic curve cryptography.

It is designed for correctness, performance, security, and high assurance
through specialized secp256k1 arithmetic, constant-time engineering, formal
verification of critical arithmetic routines, differential testing, and multiple
complementary testing techniques.  It requires no cgo and provides data
structures and functions for working with public and private secp256k1 keys.

In addition, subpackages are provided to produce, verify, parse, and serialize
ECDSA signatures and EC-Schnorr-DCRv0 (a custom Schnorr-based signature scheme
specific to Decred) signatures.  See the README.md files in the relevant sub
packages for more details about those aspects.

## Features

- Pure Go implementation with no cgo dependency
- Private key generation, parsing, and serialization
- Public key generation, parsing, and serialization per ANSI X9.62-1998
  - Parses uncompressed, compressed, and hybrid public keys
  - Serializes uncompressed and compressed public keys
- Point decompression from a given x coordinate
- RFC6979 deterministic nonce generation with support for extra data and
  version information to prevent nonce reuse across signing algorithms
- Optimized elliptic curve operations in Jacobian projective coordinates
  - Point addition
  - Point doubling
  - Scalar multiplication with an arbitrary point
  - Scalar multiplication with the base point (group generator)
- Specialized arithmetic optimized specifically for secp256k1
  - `FieldVal` for arithmetic modulo the secp256k1 field prime
  - `ModNScalar` for arithmetic modulo the secp256k1 group order

> NOTE: The `S256()` function and the associated `crypto/elliptic.Curve` methods
> (`Add`, `Double`, etc.) on `KoblitzCurve` are **deprecated**.
>
> Use the `ecdsa` subpackage.  The generic `crypto/elliptic` interface is
> significantly slower for secp256k1 and has been **deprecated** by the Go
> authors.

## Assurance

This package emphasizes correctness and implementation assurance through
multiple complementary validation techniques.

Key implementation characteristics include:

- Continuous production use since 2013 across the Decred and Bitcoin ecosystems
- Specialized secp256k1 arithmetic rather than generic elliptic curve
  implementations
- Constant-time field and scalar arithmetic for secret-dependent operations
- Formal verification of critical arithmetic operations using the Z3 theorem
  prover
- Differential testing against other secp256k1 implementations, including
  Bitcoin Core's libsecp256k1
- Deterministic test vectors
- Property-based testing
- Coverage-guided fuzz testing
- Manual review of security-critical implementation details

The formal verification artifacts establish correctness properties of critical
optimized arithmetic implementations, including multi-precision multiplication,
intermediate value bounds, carry handling, and modular reduction.  They are
available in the [internal/proofs](internal/proofs) directory.

Secret-dependent arithmetic is implemented using constant-time algorithms and
generated machine code is inspected during validation to help confirm that
compiler optimizations preserve the intended constant-time properties.

No single technique is sufficient to establish confidence in cryptographic
software.  Formal verification, differential validation, constant-time
engineering, testing, and manual review provide complementary sources of
implementation assurance.

## Testing and Verification

In addition to extensive unit testing, this package employs multiple validation
techniques appropriate for cryptographic software, including deterministic test
vectors, property-based testing, differential testing against independent
implementations, coverage-guided fuzz testing, and formal verification of
security-critical arithmetic routines.

Although this package was originally developed for dcrd, it has intentionally
been designed as a standalone module for any projects needing to use optimized
secp256k1 elliptic curve cryptography.

## References

The secp256k1 domain parameters are specified in [SEC 2: Recommended Elliptic Curve Domain Parameters](https://www.secg.org/sec2-v2.pdf).

## secp256k1 use in Decred

At the time of this writing, the primary public key cryptography in widespread
use on the Decred network used to secure coins is based on elliptic curves
defined by the secp256k1 domain parameters.

## Installation and Updating

This package is part of the `github.com/decred/dcrd/dcrec/secp256k1/v4` module.
Use the standard go tooling for working with modules to incorporate it.

## Examples

* [Encryption](https://pkg.go.dev/github.com/decred/dcrd/dcrec/secp256k1/v4#example-package-EncryptDecryptMessage)
  Demonstrates encrypting and decrypting a message using a shared key derived
  through ECDHE.

## License

Package secp256k1 is licensed under the [copyfree](http://copyfree.org) ISC
License.
