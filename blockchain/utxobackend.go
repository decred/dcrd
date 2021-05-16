// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"

	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/wire"
)

var (
	// utxoSetStateKeyName is the name of the database key used to house the
	// state of the unspent transaction output set.
	utxoSetStateKeyName = []byte("utxosetstate")
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

	// FetchState returns the current state of the UTXO set.
	FetchState() (*UtxoSetState, error)

	// PutUtxos atomically updates the UTXO set with the entries from the provided
	// map along with the current state.
	PutUtxos(utxos map[wire.OutPoint]*UtxoEntry, state *UtxoSetState) error
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

// FetchState returns the current state of the UTXO set.
func (l *LevelDbUtxoBackend) FetchState() (*UtxoSetState, error) {
	// Fetch the utxo set state from the database.
	var serialized []byte
	err := l.db.View(func(dbTx database.Tx) error {
		// Fetch the serialized utxo set state from the database.
		serialized = dbTx.Metadata().Get(utxoSetStateKeyName)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Return nil if the utxo set state does not exist in the database.  This
	// should only be the case when starting from a fresh database or a database
	// that has not been run with the utxo cache yet.
	if serialized == nil {
		return nil, nil
	}

	// Deserialize the utxo set state and return it.
	return deserializeUtxoSetState(serialized)
}

// dbPutUtxoEntry uses an existing database transaction to update the utxo
// entry for the given outpoint based on the provided utxo entry state.  In
// particular, the entry is only written to the database if it is marked as
// modified, and if the entry is marked as spent it is removed from the
// database.
func (l *LevelDbUtxoBackend) dbPutUtxoEntry(dbTx database.Tx,
	outpoint wire.OutPoint, entry *UtxoEntry) error {

	// No need to update the database if the entry was not modified.
	if entry == nil || !entry.isModified() {
		return nil
	}

	// Remove the utxo entry if it is spent.
	utxoBucket := dbTx.Metadata().Bucket(utxoSetBucketName)
	if entry.IsSpent() {
		key := outpointKey(outpoint)
		err := utxoBucket.Delete(*key)
		recycleOutpointKey(key)
		if err != nil {
			return err
		}

		return nil
	}

	// Serialize and store the utxo entry.
	serialized, err := serializeUtxoEntry(entry)
	if err != nil {
		return err
	}
	key := outpointKey(outpoint)
	err = utxoBucket.Put(*key, serialized)
	// NOTE: The key is intentionally not recycled here since the database
	// interface contract prohibits modifications.  It will be garbage collected
	// normally when the database is done with it.
	if err != nil {
		return err
	}

	return nil
}

// PutUtxos atomically updates the UTXO set with the entries from the provided
// map along with the current state.
func (l *LevelDbUtxoBackend) PutUtxos(utxos map[wire.OutPoint]*UtxoEntry,
	state *UtxoSetState) error {

	// Update the database with the provided entries and UTXO set state.
	//
	// It is important that the UTXO set state is always updated in the same
	// database transaction as the utxo set itself so that it is always in sync.
	err := l.db.Update(func(dbTx database.Tx) error {
		for outpoint, entry := range utxos {
			// Write the entry to the database.
			err := l.dbPutUtxoEntry(dbTx, outpoint, entry)
			if err != nil {
				return err
			}
		}

		// Update the UTXO set state in the database.
		return dbTx.Metadata().Put(utxoSetStateKeyName,
			serializeUtxoSetState(state))
	})
	if err != nil {
		return err
	}

	// Flush the UTXO database to disk.  This is necessary in the case that the
	// UTXO set state was just initialized so that if the block database is
	// flushed, and then an unclean shutdown occurs, the UTXO cache will know
	// where to start from when recovering on startup.
	return l.db.Flush()
}
