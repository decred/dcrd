// Copyright (c) 2019-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
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

# Function categories

The provided functions fall into the following categories:

  - Proof-of-work
  - Merkle root calculation
  - Subsidy calculation
  - Coinbase transaction identification
  - Merkle tree inclusion proofs
  - Transaction sanity checking

# Proof-of-work

  - Converting to and from the compact target difficulty representation
  - Calculating work values based on the compact target difficulty
  - Checking a block hash satisfies a target difficulty and that target
    difficulty is within a valid range

# Merkle root calculation

  - Calculation from individual leaf hashes
  - Calculation from a slice of transactions

# Subsidy calculation

  - Proof-of-work subsidy for a given height and number of votes
  - Stake vote subsidy for a given height
  - Treasury subsidy for a given height and number of votes

# Merkle tree inclusion proofs

  - Generate an inclusion proof for a given tree and leaf index
  - Verify a leaf is a member of the tree at a given index via the proof

# Errors

Errors returned by this package are of type standalone.RuleError.  This allows
the caller to differentiate between errors further up the call stack through
type assertions.  In addition, callers can programmatically determine the
specific rule violation by examining the ErrorCode field of the type asserted
standalone.RuleError.
*/
package standalone
