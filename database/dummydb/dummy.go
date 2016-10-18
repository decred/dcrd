// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// dummydb implements a dummy database satisfying all interfaces but not
// actually performing any functions.  It should be used ONLY in testing
// environments, such as blockchain tests relying on blockNodes that have
// operations that call the database but are not dependent on their
// responses.  It should NEVER be used in a production environment!
package dummydb

import (
	"fmt"

	"github.com/btcsuite/btclog"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrutil"
)

var log = btclog.Disabled

const (
	dbType = "dummydb"
)

// init initializes the dummy database.
func init() {
	create := func(...interface{}) (database.DB, error) { return &db{}, nil }
	open := func(...interface{}) (database.DB, error) { return &db{}, nil }

	// Register the driver.
	driver := database.Driver{
		DbType:    dbType,
		Create:    create,
		Open:      open,
		UseLogger: nil,
	}
	if err := database.RegisterDriver(driver); err != nil {
		panic(fmt.Sprintf("Failed to regiser database driver '%s': %v",
			dbType, err))
	}
}

// cursor is a dummy structrequired to satisfy the interface.
type cursor struct {
}

// Enforce cursor implements the database.Cursor interface.
var _ database.Cursor = (*cursor)(nil)

// Bucket is a dummy function required to satisfy the interface.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Bucket() database.Bucket {
	return c.Bucket()
}

// Delete is a dummy function required to satisfy the interface.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Delete() error {
	return nil
}

// First is a dummy function required to satisfy the interface.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) First() bool {
	return false
}

// Last is a dummy function required to satisfy the interface.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Last() bool {
	return false
}

// Next is a dummy function required to satisfy the interface.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Next() bool {
	return false
}

// Prev is a dummy function required to satisfy the interface.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Prev() bool {
	return false
}

// See is a dummy function required to satisfy the interface.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Seek(seek []byte) bool {
	return false
}

// Key is a dummy function required to satisfy the interface.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Key() []byte {
	return []byte{}
}

// Value is a dummy function required to satisfy the interface.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Value() []byte {
	return []byte{}
}

// bucket is a dummy struct required to satisfy the interface.
type bucket struct {
	tx *transaction
}

// Enforce bucket implements the database.Bucket interface.
var _ database.Bucket = (*bucket)(nil)

// Bucket is a dummy function required to satisfy the interface.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) Bucket(key []byte) database.Bucket {
	return &bucket{}
}

// CreateBucket is a dummy function required to satisfy the interface.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) CreateBucket(key []byte) (database.Bucket, error) {
	return &bucket{}, nil
}

// CreateBucketIfNotExists is a dummy function required to satisfy the interface.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) CreateBucketIfNotExists(key []byte) (database.Bucket, error) {
	return b.CreateBucket(key)
}

// DeleteBucket is a dummy function required to satisfy the interface.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) DeleteBucket(key []byte) error {
	return nil
}

// Cursor is a dummy function required to satisfy the interface.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) Cursor() database.Cursor {
	return &cursor{}
}

// ForEach is a dummy function required to satisfy the interface.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) ForEach(fn func(k, v []byte) error) error {
	return nil
}

// ForEachBucket is a dummy function required to satisfy the interface.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) ForEachBucket(fn func(k []byte) error) error {
	return nil
}

// Writable is a dummy function required to satisfy the interface.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) Writable() bool {
	return false
}

// Put is a dummy function required to satisfy the interface.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) Put(key, value []byte) error {
	return nil
}

// Get is a dummy function required to satisfy the interface.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) Get(key []byte) []byte {
	return nil
}

// Delete is a dummy function required to satisfy the interface.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) Delete(key []byte) error {
	return nil
}

// transaction is a dummy struct required to satisfy the interface.
type transaction struct {
}

// Enforce transaction implements the database.Tx interface.
var _ database.Tx = (*transaction)(nil)

// Metadata  is a dummy function required to satisfy the interface.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) Metadata() database.Bucket {
	return &bucket{}
}

// StoreBlock is a dummy function required to satisfy the interface.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) StoreBlock(block *dcrutil.Block) error {
	return nil
}

// HasBlock is a dummy function required to satisfy the interface.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) HasBlock(hash *chainhash.Hash) (bool, error) {
	return false, nil
}

// HasBlocks is a dummy function required to satisfy the interface.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) HasBlocks(hashes []chainhash.Hash) ([]bool, error) {
	return []bool{}, nil
}

// FetchBlockHeader is a dummy function required to satisfy the interface.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) FetchBlockHeader(hash *chainhash.Hash) ([]byte, error) {
	return []byte{}, nil
}

// FetchBlockHeaders is a dummy function required to satisfy the interface.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) FetchBlockHeaders(hashes []chainhash.Hash) ([][]byte, error) {
	return [][]byte{}, nil
}

// FetchBlock is a dummy function required to satisfy the interface.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) FetchBlock(hash *chainhash.Hash) ([]byte, error) {
	return nil, nil
}

// FetchBlocks is a dummy function required to satisfy the interface.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) FetchBlocks(hashes []chainhash.Hash) ([][]byte, error) {
	return nil, nil
}

// FetchBlockRegion is a dummy function required to satisfy the interface.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) FetchBlockRegion(region *database.BlockRegion) ([]byte, error) {
	return nil, nil
}

// FetchBlockRegions is a dummy function required to satisfy the interface.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) FetchBlockRegions(regions []database.BlockRegion) ([][]byte, error) {
	return nil, nil
}

// Commit is a dummy function required to satisfy the interface.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) Commit() error {
	return nil
}

// Rollback is a dummy function required to satisfy the interface.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) Rollback() error {
	return nil
}

// db is a dummy struct required to satisfy the interface.
type db struct {
}

// Enforce db implements the database.DB interface.
var _ database.DB = (*db)(nil)

// Type returns the database driver type the current database instance was
// created with.
//
// This function is part of the database.DB interface implementation.
func (db *db) Type() string {
	return dbType
}

// begin is a dummy function required to satisfy the interface.
//
// This function is only separate because it returns the internal transaction
// which is used by the managed transaction code while the database method
// returns the interface.
func (db *db) begin(writable bool) (*transaction, error) {
	return &transaction{}, nil
}

// Begin is a dummy function required to satisfy the interface.
//
// This function is part of the database.DB interface implementation.
func (db *db) Begin(writable bool) (database.Tx, error) {
	return db.begin(writable)
}

// View is a dummy function required to satisfy the interface.
//
// This function is part of the database.DB interface implementation.
func (db *db) View(fn func(database.Tx) error) error {
	fn(nil)
	return nil
}

// Update is a dummy function required to satisfy the interface.
//
// This function is part of the database.DB interface implementation.
func (db *db) Update(fn func(database.Tx) error) error {
	fn(nil)
	return nil
}

// Close is a dummy function required to satisfy the interface.
//
// This function is part of the database.DB interface implementation.
func (db *db) Close() error {
	return nil
}
