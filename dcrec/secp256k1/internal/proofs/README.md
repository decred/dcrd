proofs
======

## Overview

This consists of formal verification artifacts used to help establish the
correctness of the security-critical arithmetic implementations.

The current proofs formally verify properties of the optimized field arithmetic
implementations using [z3](https://github.com/Z3Prover/z3):

- Correctness of multi-precision multiplication
- Bounds on intermediate values
- Safety of intentionally discarded carries
- Correctness of modular reduction steps

The proofs are not required when building or using the package.  They are
provided to document important correctness properties and make it possible to
independently verify correctness claims documented throughout the optimized
implementations.

The proofs use SMT-based verification to model the underlying fixed-width
integer operations and mechanically verify those properties.

These artifacts are only intended to complement, rather than replace,
traditional testing and code review.  They provide additional assurance that the
optimized implementations preserve the required arithmetic invariants.

## Contents

### secp256k1 Arithmetic Proofs

- [Helpers for z3 proofs](z3_proof_helpers.py)
  Provides common helpers used in several of the proofs.  It is not intended to
  be run directly.

- [512-bit Multiplication with 64-bit Limbs](mul512_4x64_prover.py)
  Provides formal verification of the multiplication of two 256-bit values to
  produce a full 512-bit unreduced product using saturated 64-bit limbs.

- [512-bit Squaring with 64-bit Limbs](square512_4x64_prover.py)
  Provides formal verification of the squaring of a 256-bit value to produce a
  full 512-bit unreduced result using saturated 64-bit limbs.

- [Field Modular Reduction](field_4x64_reduce_prover.py)
  Provides formal verification of the reduction of a 512-bit value represented
  using saturated 64-bit limbs modulo the secp256k1 prime.
