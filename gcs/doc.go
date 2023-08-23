// Copyright (c) 2018-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package gcs provides an API for building and using a Golomb-coded set filter.

A Golomb-Coded Set (GCS) is a space-efficient probabilistic data structure that
is used to test set membership with a tunable false positive rate while
simultaneously preventing false negatives.  In other words, items that are in
the set will always match, but items that are not in the set will also sometimes
match with the chosen false positive rate.

This package currently implements two different versions for backwards
compatibility.  Version 1 is deprecated and therefore should no longer be used.

Version 2 is the GCS variation that follows the specification details in
DCP0005: https://github.com/decred/dcps/blob/master/dcp-0005/dcp-0005.mediawiki#golomb-coded-sets.

Version 2 sets do not permit empty items (data of zero length) to be added and
are parameterized by the following:

* A parameter `B` that defines the remainder code bit size
* A parameter `M` that defines the false positive rate as `1/M`
* A key for the SipHash-2-4 function
* The items to include in the set

# Errors

The errors returned by this package are of type gcs.Error.  This allows the
caller to programmatically determine the specific error by examining the
ErrorKind field of the type asserted gcs.Error while still providing rich error
messages with contextual information.  See ErrorKind in the package
documentation for a full list.

# GCS use in Decred

GCS is used as a mechanism for storing, transmitting, and committing to
per-block filters.  Consensus-validating full nodes commit to a single filter
for every block and serve the filter to SPV clients that match against the
filter locally to determine if the block is potentially relevant.  The required
parameters for Decred are defined by the blockcf2 package.

For more details, see the Block Filters section of DCP0005:
https://github.com/decred/dcps/blob/master/dcp-0005/dcp-0005.mediawiki#block-filters
*/
package gcs
