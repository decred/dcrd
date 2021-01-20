indexers
========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/blockchain/v3/indexers)

Package indexers implements optional block chain indexes.

These indexes are typically used to enhance the amount of information available
via an RPC interface.

## Supported Indexers

- Transaction-by-hash (txbyhashidx) Index
  - Creates a mapping from the hash of each transaction to the block that
    contains it along with its offset and length within the serialized block
- Transaction-by-address (txbyaddridx) Index
  - Creates a mapping from every address to all transactions which either credit
    or debit the address
  - Requires the transaction-by-hash index
- Address-ever-seen (existsaddridx) Index
  - Stores a key with an empty value for every address that has ever existed
    and was seen by the client
- Committed Filter (cfindexparentbucket) Index
  - Stores all committed filters and committed filter headers for all blocks in
    the main chain

## Installation and Updating

This package is part of the `github.com/decred/dcrd/blockchain/v3` module.  Use
the standard go tooling for working with modules to incorporate it.

## License

Package indexers is licensed under the [copyfree](http://copyfree.org) ISC
License.
