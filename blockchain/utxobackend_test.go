// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/wire"
)

// createTestUtxoBackend creates a test backend with the utxo set bucket.
func createTestUtxoBackend(t *testing.T) *LevelDbUtxoBackend {
	t.Helper()

	// Create a test database.
	dbPath := filepath.Join(os.TempDir(), t.Name())
	_ = os.RemoveAll(dbPath)
	db, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("error creating test database: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(dbPath)
	})
	t.Cleanup(func() {
		db.Close()
	})

	// Create the utxo set bucket.
	err = db.Update(func(dbTx database.Tx) error {
		_, err := dbTx.Metadata().CreateBucketIfNotExists(utxoSetBucketName)
		return err
	})
	if err != nil {
		t.Fatalf("error creating utxo bucket: %v", err)
	}

	return NewLevelDbUtxoBackend(db)
}
