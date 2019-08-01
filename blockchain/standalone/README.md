standalone
==========

[![Build Status](https://img.shields.io/travis/decred/dcrd.svg)](https://travis-ci.org/decred/dcrd)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/github.com/decred/dcrd/blockchain/standalone)

Package standalone provides standalone functions useful for working with the
Decred blockchain consensus rules.

The primary goal of offering these functions via a separate module is to reduce
the required dependencies to a minimum as compared to the blockchain module.

It is ideal for applications such as lightweight clients that need to ensure
basic security properties hold and calculate appropriate vote subsidies and
block explorers.

For example, some things an SPV wallet needs to prove are that the block headers
all connect together, that they satisfy the proof of work requirements, and that
a given transaction tree is valid for a given header.

The provided functions fall into the following categories:

- Proof-of-work
  - Converting to and from the compact target difficulty representation
  - Calculating work values based on the compact target difficulty
  - Checking a block hash satisfies a target difficulty and that target
    difficulty is within a valid range
- Merkle root calculation
  - Calculation from individual leaf hashes
  - Calculation from a slice of transactions
- Subsidy calculation  (WIP - not yet available)
- Coinbase transaction identification (WIP - not yet available)

## Installation and Updating

```bash
$ go get -u github.com/decred/dcrd/blockchain/standalone
```

## Examples

* [CompactToBig Example](https://godoc.org/github.com/decred/dcrd/blockchain/standalone#example-CompactToBig)  
  Demonstrates how to convert the compact "bits" in a block header which
  represent the target difficulty to a big integer and display it using the
  typical hex notation.

* [BigToCompact Example](https://godoc.org/github.com/decred/dcrd/blockchain/standalone#example-BigToCompact)  
  Demonstrates how to convert a target difficulty into the compact "bits" in a
  block header which represent that target difficulty.

* [CheckProofOfWork Example](https://godoc.org/github.com/decred/dcrd/blockchain/standalone#example-CheckProofOfWork)  
  Demonstrates checking the proof of work of a block hash against a target
  difficulty.

* [CalcMerkleRoot Example](https://godoc.org/github.com/decred/dcrd/blockchain/standalone#example-CalcMerkleRoot)  
  Demonstrates calculating a merkle root from a slice of leaf hashes.

## License

Package standalone is licensed under the [copyfree](http://copyfree.org) ISC
License.
