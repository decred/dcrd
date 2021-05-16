// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"

	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/wire"
)

// UtxoBackend represents a persistent storage layer for the UTXO set.
//
// The interface contract requires that all of these methods are safe for
// concurrent access.
type UtxoBackend interface {
	// FetchEntry returns the specified transaction output from the UTXO set.
	//
	// When there is no entry for the provided output, nil will be returned for
	// both the entry and the error.
	FetchEntry(outpoint wire.OutPoint) (*UtxoEntry, error)
}

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

// dbFetchUtxoEntry uses an existing database transaction to fetch the specified
// transaction output from the utxo set.
//
// When there is no entry for the provided output, nil will be returned for both
// the entry and the error.
func (l *LevelDbUtxoBackend) dbFetchUtxoEntry(dbTx database.Tx,
	outpoint wire.OutPoint) (*UtxoEntry, error) {

	// Fetch the unspent transaction output information for the passed transaction
	// output.  Return now when there is no entry.
	key := outpointKey(outpoint)
	utxoBucket := dbTx.Metadata().Bucket(utxoSetBucketName)
	serializedUtxo := utxoBucket.Get(*key)
	recycleOutpointKey(key)
	if serializedUtxo == nil {
		return nil, nil
	}

	// A non-nil zero-length entry means there is an entry in the database for a
	// spent transaction output which should never be the case.
	if len(serializedUtxo) == 0 {
		return nil, AssertError(fmt.Sprintf("database contains entry for spent tx "+
			"output %v", outpoint))
	}

	// Deserialize the utxo entry and return it.
	entry, err := deserializeUtxoEntry(serializedUtxo, outpoint.Index)
	if err != nil {
		// Ensure any deserialization errors are returned as database corruption
		// errors.
		if isDeserializeErr(err) {
			str := fmt.Sprintf("corrupt utxo entry for %v: %v", outpoint, err)
			return nil, makeDbErr(database.ErrCorruption, str)
		}

		return nil, err
	}

	return entry, nil
}

// FetchEntry returns the specified transaction output from the UTXO set.
//
// When there is no entry for the provided output, nil will be returned for both
// the entry and the error.
func (l *LevelDbUtxoBackend) FetchEntry(outpoint wire.OutPoint) (*UtxoEntry, error) {
	// Fetch the entry from the database.
	//
	// NOTE: Missing entries are not considered an error here and instead
	// will result in nil entries in the view.  This is intentionally done
	// so other code can use the presence of an entry in the view as a way
	// to unnecessarily avoid attempting to reload it from the database.
	var entry *UtxoEntry
	err := l.db.View(func(dbTx database.Tx) error {
		var err error
		entry, err = l.dbFetchUtxoEntry(dbTx, outpoint)
		return err
	})
	if err != nil {
		return nil, err
	}

	return entry, nil
}
