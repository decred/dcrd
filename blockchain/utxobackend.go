// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"github.com/decred/dcrd/database/v2"
)

// UtxoBackend represents a persistent storage layer for the UTXO set.
//
// The interface contract requires that all of these methods are safe for
// concurrent access.
type UtxoBackend interface{}

// LevelDbUtxoBackend implements the UtxoBackend interface using an underlying
// leveldb database instance.
type LevelDbUtxoBackend struct {
	// db is the database that contains the UTXO set.  It is set when the instance
	// is created and is not changed afterward.
	db database.DB
}

// Ensure LevelDbUtxoBackend implements the UtxoBackend interface.
var _ UtxoBackend = (*LevelDbUtxoBackend)(nil)

// NewLevelDbUtxoBackend returns a new LevelDbUtxoBackend instance using the
// provided database.
func NewLevelDbUtxoBackend(db database.DB) *LevelDbUtxoBackend {
	return &LevelDbUtxoBackend{
		db: db,
	}
}
