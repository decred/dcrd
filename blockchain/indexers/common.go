// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2016-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package indexers implements optional block chain indexes.
*/
package indexers

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/wire"
)

var (
	// byteOrder is the preferred byte order used for serializing numeric
	// fields for storage in the database.
	byteOrder = binary.LittleEndian

	// errInterruptRequested indicates that an operation was cancelled due
	// to a user-requested interrupt.
	errInterruptRequested = errors.New("interrupt requested")
)

// NeedsInputser provides a generic interface for an indexer to specify the it
// requires the ability to look up inputs for a transaction.
type NeedsInputser interface {
	NeedsInputs() bool
}

// PrevScripter defines an interface that provides access to scripts and their
// associated version keyed by an outpoint.  It is used within this package as a
// generic means to provide the scripts referenced by the inputs to transactions
// within a block that are needed to index it.  The boolean return indicates
// whether or not the script and version for the provided outpoint was found.
type PrevScripter interface {
	PrevScript(*wire.OutPoint) (uint16, []byte, bool)
}

// Indexer provides a generic interface for an indexer that is managed by an
// index manager such as the Manager type provided by this package.
type Indexer interface {
	// Key returns the key of the index as a byte slice.
	Key() []byte

	// Name returns the human-readable name of the index.
	Name() string

	// Return the current version of the index.
	Version() uint32

	// Create is invoked when the indexer manager determines the index needs
	// to be created for the first time.
	Create(dbTx database.Tx) error

	// Init is invoked when the index manager is first initializing the
	// index.  This differs from the Create method in that it is called on
	// every load, including the case the index was just created.
	Init() error

	// ConnectBlock is invoked when the index manager is notified that a new
	// block has been connected to the main chain.
	ConnectBlock(dbTx database.Tx, block, parent *dcrutil.Block, prevScripts PrevScripter, isTreasuryEnabled bool) error

	// DisconnectBlock is invoked when the index manager is notified that a
	// block has been disconnected from the main chain.
	DisconnectBlock(dbTx database.Tx, block, parent *dcrutil.Block, prevScripts PrevScripter, isTreasuryEnabled bool) error
}

// ChainQueryer provides a generic interface that is used to provide access to
// the chain details required to by the index manager to initialize and sync
// the various indexes.
//
// All function MUST be safe for concurrent access.
type ChainQueryer interface {
	// MainChainHasBlock returns whether or not the block with the given hash is
	// in the main chain.
	MainChainHasBlock(*chainhash.Hash) bool

	// BestHeight returns the height of the current best block.
	BestHeight() int64

	// BlockHashByHeight returns the hash of the block at the given height in
	// the main chain.
	BlockHashByHeight(int64) (*chainhash.Hash, error)

	// PrevScripts returns a source of previous transaction scripts and their
	// associated versions spent by the given block.
	PrevScripts(database.Tx, *dcrutil.Block) (PrevScripter, error)

	// IsTreasuryEnabled returns true if the treasury agenda is active at
	// the provided block.
	IsTreasuryEnabled(*chainhash.Hash) (bool, error)
}

// IndexManager provides a generic interface that is called when blocks are
// connected and disconnected to and from the tip of the main chain for the
// purpose of supporting optional indexes.
type IndexManager interface {
	// Init is invoked during chain initialize in order to allow the index
	// manager to initialize itself and any indexes it is managing.  The channel
	// parameter specifies a channel the caller can close to signal that the
	// process should be interrupted.  It can be nil if that behavior is not
	// desired.
	Init(context.Context, ChainQueryer) error

	// ConnectBlock is invoked when a new block has been connected to the main
	// chain.
	ConnectBlock(database.Tx, *dcrutil.Block, *dcrutil.Block, PrevScripter, bool) error

	// DisconnectBlock is invoked when a block has been disconnected from the
	// main chain.
	DisconnectBlock(database.Tx, *dcrutil.Block, *dcrutil.Block, PrevScripter, bool) error
}

// IndexDropper provides a method to remove an index from the database. Indexers
// may implement this for a more efficient way of deleting themselves from the
// database rather than simply dropping a bucket.
type IndexDropper interface {
	DropIndex(context.Context, database.DB) error
}

// AssertError identifies an error that indicates an internal code consistency
// issue and should be treated as a critical and unrecoverable error.
type AssertError string

// Error returns the assertion error as a huma-readable string and satisfies
// the error interface.
func (e AssertError) Error() string {
	return "assertion failed: " + string(e)
}

// errDeserialize signifies that a problem was encountered when deserializing
// data.
type errDeserialize string

// Error implements the error interface.
func (e errDeserialize) Error() string {
	return string(e)
}

// isDeserializeErr returns whether or not the passed error is an errDeserialize
// error.
func isDeserializeErr(err error) bool {
	_, ok := err.(errDeserialize)
	return ok
}

// internalBucket is an abstraction over a database bucket.  It is used to make
// the code easier to test since it allows mock objects in the tests to only
// implement these functions instead of everything a database.Bucket supports.
type internalBucket interface {
	Get(key []byte) []byte
	Put(key []byte, value []byte) error
	Delete(key []byte) error
}

// interruptRequested returns true when the provided channel has been closed.
// This simplifies early shutdown slightly since the caller can just use an if
// statement instead of a select.
func interruptRequested(ctx context.Context) bool {
	return ctx.Err() != nil
}
