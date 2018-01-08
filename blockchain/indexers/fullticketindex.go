// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2016-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"errors"
	"fmt"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/dcrutil"
)

const (
	// ticketIndex is the human-readable name for the index.
	ticketIndex = "full ticket index"
)

var (
	// ticketIndexKey is the key of the ticket index and the db bucket used
	// to house it.
	ticketIndexKey = []byte("ticketbyhashidx")

	// errNotFound is an error that indicates a requested entry does
	// not exist.
	errNotFound = errors.New("no entry in the block ID index")
)

// FullTicketIndex implements a ticket index.
type FullTicketIndex struct {
	db database.DB
}

// Ensure the FullTicketIndex type implements the Indexer interface.
var _ Indexer = (*FullTicketIndex)(nil)

// Init initializes the ticket index.
//
// This is part of the Indexer interface.
func (idx *FullTicketIndex) Init() error {
	return fmt.Errorf("Init not yet")
}

// Key returns the database key to use for the index as a byte slice.
//
// This is part of the Indexer interface.
func (idx *FullTicketIndex) Key() []byte {
	return ticketIndexKey
}

// Name returns the human-readable name of the index.
//
// This is part of the Indexer interface.
func (idx *FullTicketIndex) Name() string {
	return ticketIndex
}

// Create is invoked when the indexer manager determines the index needs to be
// created for the first time.  It creates the bucket for the ticket index.
//
// This is part of the Indexer interface.
func (idx *FullTicketIndex) Create(dbTx database.Tx) error {
	meta := dbTx.Metadata()
	_, err := meta.CreateBucket(ticketIndexKey)
	return err
}

// ConnectBlock is invoked by the index manager when a new block has been
// connected to the main chain.
//
// This is part of the Indexer interface.
func (idx *FullTicketIndex) ConnectBlock(dbTx database.Tx, block, parent *dcrutil.Block, view *blockchain.UtxoViewpoint) error {
	return fmt.Errorf("ConnectBlock not yet")
}

// DisconnectBlock is invoked by the index manager when a block has been
// disconnected from the main chain.
//
// This is part of the Indexer interface.
func (idx *FullTicketIndex) DisconnectBlock(dbTx database.Tx, block, parent *dcrutil.Block, view *blockchain.UtxoViewpoint) error {
	return fmt.Errorf("DisconnectBlock not yet")
}
