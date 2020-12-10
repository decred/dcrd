// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// PriorityInputser defines an interface that provides access to information
// about an transaction output needed to calculate a priority based on the input
// age of a transaction.  It is used within this package as a generic means to
// provide the block heights and amounts referenced by all of the inputs to a
// transaction that are needed to calculate an input age.  The boolean return
// indicates whether or not the information for the provided outpoint was found.
type PriorityInputser interface {
	PriorityInput(prevOut *wire.OutPoint) (blockHeight int64, amount int64, ok bool)
}

// TxSource represents a source of transactions to consider for inclusion in
// new blocks.
//
// The interface contract requires that all of these methods are safe for
// concurrent access with respect to the source.
type TxSource interface {
	// LastUpdated returns the last time a transaction was added to or
	// removed from the source pool.
	LastUpdated() time.Time

	// HaveTransaction returns whether or not the passed transaction hash
	// exists in the source pool.
	HaveTransaction(hash *chainhash.Hash) bool

	// HaveAllTransactions returns whether or not all of the passed
	// transaction hashes exist in the source pool.
	HaveAllTransactions(hashes []chainhash.Hash) bool

	// VoteHashesForBlock returns the hashes for all votes on the provided
	// block hash that are currently available in the source pool.
	VoteHashesForBlock(hash *chainhash.Hash) []chainhash.Hash

	// VotesForBlocks returns a slice of vote descriptors for all votes on
	// the provided block hashes that are currently available in the source
	// pool.
	VotesForBlocks(hashes []chainhash.Hash) [][]VoteDesc

	// IsRegTxTreeKnownDisapproved returns whether or not the regular
	// transaction tree of the block represented by the provided hash is
	// known to be disapproved according to the votes currently in the
	// source pool.
	IsRegTxTreeKnownDisapproved(hash *chainhash.Hash) bool

	// MiningView returns a snapshot of the underlying TxSource.
	MiningView() *TxMiningView
}

// blockManagerFacade provides the mining package with a subset of
// the methods originally defined in the blockManager.
type blockManagerFacade interface {
	// ForceReorganization forces a reorganization of the block chain to the block
	// hash requested, so long as it matches up with the current organization of the
	// best chain.
	ForceReorganization(formerBest, newBest chainhash.Hash) error

	// IsCurrent returns whether or not the block manager believes it is synced
	// with the connected peers.
	IsCurrent() bool
}
