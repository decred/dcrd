blockchain
==========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/blockchain/v3)

Package blockchain implements Decred block handling and chain selection rules.

The Decred block handling and chain selection rules are an integral, and quite
likely the most important, part of Decred.  At its core, Decred is a distributed
consensus of which blocks are valid and which ones will comprise the main block
chain (public ledger) that ultimately determines accepted transactions, so it is
extremely important that fully validating nodes agree on all rules.

At a high level, this package provides support for inserting new blocks into the
block chain according to the aforementioned rules.  It includes functionality
such as rejecting duplicate blocks, ensuring blocks and transactions follow all
rules, and best chain selection along with reorganization.

Since this package does not deal with other Decred specifics such as network
communication or wallets, it provides a notification system which gives the
caller a high level of flexibility in how they want to react to certain events
such as newly connected main chain blocks which might result in wallet updates.

A comprehensive suite of tests is provided to ensure proper functionality.

## Decred Chain Processing Overview

Before a block is allowed into the block chain, it must go through an intensive
series of validation rules.  The following list serves as a general outline of
those rules to provide some intuition into what is going on under the hood, but
is by no means exhaustive:

 - Reject duplicate blocks
 - Perform a series of sanity checks on the block and its transactions such as
   verifying proof of work, timestamps, number and character of transactions,
   transaction amounts, script complexity, and merkle root calculations
 - Compare the block against predetermined checkpoints for expected timestamps
   and difficulty based on elapsed time since the checkpoint
 - Perform a series of more thorough checks that depend on the block's position
   within the block chain such as verifying block difficulties adhere to
   difficulty retarget rules, timestamps are after the median of the last
   several blocks, all transactions are finalized, checkpoint blocks match, and
   block versions are in line with the previous blocks
 - Determine how the block fits into the chain and perform different actions
   accordingly in order to ensure any side chains which have higher difficulty
   than the main chain become the new main chain
 - When a block is being connected to the main chain (either through
   reorganization of a side chain to the main chain or just extending the
   main chain), perform further checks on the block's transactions such as
   verifying transaction duplicates, script complexity for the combination of
   connected scripts, coinbase maturity, double spends, and connected
   transaction values
 - Run the transaction scripts to verify the spender is allowed to spend the
   coins
 - Insert the block into the block database

 ## Processing Order

 This package supports headers-first semantics such that block data can be
 processed out of order so long as the associated header is already known.

 The headers themselves, however, must be processed in the correct order since
 headers that do not properly connect are rejected.  In other words, orphan
 headers are not allowed.

The processing code always maintains the best chain as the branch tip that has
the most cumulative proof of work, so it is important to keep that in mind when
considering errors returned from processing blocks.

Notably, due to the ability to process blocks out of order, and the fact blocks
can only be fully validated once all of their ancestors have the block data
available, it is to be expected that no error is returned immediately for blocks
that are valid enough to make it to the point they require the remaining
ancestor block data to be fully validated even though they might ultimately end
up failing validation.  Similarly, because the data for a block becoming
available makes any of its direct descendants that already have their data
available eligible for validation, an error being returned does not necessarily
mean the block being processed is the one that failed validation.

## Installation and Updating

```bash
$ go get -u github.com/decred/dcrd/blockchain
```

## Examples

* [ProcessBlock Example](https://pkg.go.dev/github.com/decred/dcrd/blockchain/v3#example-BlockChain.ProcessBlock)
  Demonstrates how to create a new chain instance and use ProcessBlock to
  attempt to add a block to the chain.  This example intentionally
  attempts to insert a duplicate genesis block to illustrate how an invalid
  block is handled.

## License

Package blockchain is licensed under the [copyfree](http://copyfree.org) ISC
License.
