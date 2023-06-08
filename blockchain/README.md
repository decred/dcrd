blockchain
==========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/blockchain/v5)

The blockchain module provides a couple of packages useful for testing:

* [chaingen](./chaingen/README.md) - Provides facilities for generating a full
  chain of blocks
* [fullblocktests](./fullblocktests/README.md) - Provides a set of full block
  tests to be used for testing the consensus validation rules

## Sub Modules

Note that the following separate sub modules that are not a part of the this
module are also available:

* [standalone](./standalone/README.md) - Provides standalone functions useful
  for working with the Decred blockchain consensus rules.
* [stake](./stake/doc.go) - Contains code for all of dcrd's stake transaction
  chain handling and other portions related to the Proof-of-Stake (PoS) system.


## Installation and Updating

This is the `github.com/decred/dcrd/blockchain/v5` module.  Use the standard go
tooling for working with modules to incorporate it.

## License

Module blockchain is licensed under the [copyfree](http://copyfree.org) ISC
License.
