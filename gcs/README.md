gcs
===

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/gcs/v2)

Package gcs provides an API for building and using Golomb-coded sets.

A Golomb-Coded Set (GCS) is a space-efficient probabilistic data structure that
is used to test set membership with a tunable false positive rate while
simultaneously preventing false negatives.  In other words, items that are in
the set will always match, but items that are not in the set will also sometimes
match with the chosen false positive rate.

This package currently implements two different versions for backwards
compatibility.  Version 1 is deprecated and therefore should no longer be used.

Version 2 is the GCS variation that follows the specification details in
[DCP0005](https://github.com/decred/dcps/blob/master/dcp-0005/dcp-0005.mediawiki#golomb-coded-sets).

Version 2 sets do not permit empty items (data of zero length) to be added and
are parameterized by the following:

* A parameter `B` that defines the remainder code bit size
* A parameter `M` that defines the false positive rate as `1/M`
* A key for the SipHash-2-4 function
* The items to include in the set

A comprehensive suite of tests is provided to ensure proper functionality.

## GCS use in Decred

GCS is used as a mechanism for storing, transmitting, and committing to
per-block filters.  Consensus-validating full nodes commit to a single filter
for every block and serve the filter to SPV clients that match against the
filter locally to determine if the block is potentially relevant.  The required
parameters for Decred are defined by the blockcf2 package.

For more details, see the [Block Filters section of
DCP0005](https://github.com/decred/dcps/blob/master/dcp-0005/dcp-0005.mediawiki#block-filters).

## Installation and Updating

This package is part of the `github.com/decred/dcrd/gcs/v2` module.  Use the
standard go tooling for working with modules to incorporate it.

## License

Package blockchain is licensed under the [copyfree](http://copyfree.org) ISC
License.
