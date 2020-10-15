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

// TxMiningView is a snapshot of the tx source.
type TxMiningView interface {
	// TxDescs returns a slice of mining descriptors for all minable
	// transactions in the source pool.
	TxDescs() []*TxDesc

	// AncestorStats returns the last known ancestor stats for the provided
	// transaction hash, and a boolean indicating whether ancestors are being
	// tracked for it.
	//
	// Calling Ancestors will update the value returned by this function to
	// reflect the newly calculated statistics for those ancestors.
	AncestorStats(txHash *chainhash.Hash) (*TxAncestorStats, bool)

	// Ancestors returns all transactions in the mining view that the provided
	// transaction hash depends on.
	Ancestors(txHash *chainhash.Hash) []*TxDesc

	// HasParents returns true if the provided transaction hash has any
	// ancestors known to the view.
	HasParents(txHash *chainhash.Hash) bool

	// Parents returns a set of transactions in the graph that the provided
	// transaction hash spends from in the view. The order of elements
	// returned is not guaranteed.
	Parents(txHash *chainhash.Hash) []*TxDesc

	// Children returns a set of transactions in the graph that spend
	// from the provided transaction hash. The order of elements
	// returned is not guaranteed.
	Children(txHash *chainhash.Hash) []*TxDesc

	// Remove causes the provided transaction to be removed from the view, if
	// it exists. The updateDescendantStats parameter indicates whether the
	// descendent transactions of the provided txHash should have their ancestor
	// stats updated within the view to account for the removal of this
	// transaction.
	Remove(txHash *chainhash.Hash, updateDescendantStats bool)

	// Reject removes and flags the provided transaction hash and all of its
	// descendants in the view as rejected.
	Reject(txHash *chainhash.Hash)

	// IsRejected checks to see if a transaction that once existed in the view
	// has been rejected.
	IsRejected(txHash *chainhash.Hash) bool
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
	MiningView() TxMiningView
}

// blockManagerFacade provides the mining package with a subset of
// the methods originally defined in the blockManager.
type blockManagerFacade interface {
	ForceReorganization(formerBest, newBest chainhash.Hash) error
	IsCurrent() bool
	NotifyWork(templateNtfn *TemplateNtfn)
}
