// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"os"
	"path/filepath"
	"reflect"
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

// TestFetchEntryFromBackend validates that fetch entry returns the correct
// entry from the backend under a variety of conditions.
func TestFetchEntryFromBackend(t *testing.T) {
	t.Parallel()

	// Create a test backend.
	backend := createTestUtxoBackend(t)

	// Create test entries to be used throughout the tests.
	outpoint := outpoint299()
	entry := entry299()

	tests := []struct {
		name           string
		backendEntries map[wire.OutPoint][]byte
		outpoint       wire.OutPoint
		wantEntry      *UtxoEntry
		wantErr        bool
	}{{
		name:     "entry is not in the backend",
		outpoint: outpoint,
	}, {
		name: "entry is in the backend",
		backendEntries: map[wire.OutPoint][]byte{
			outpoint: hexToBytes("812b010080fba8a41b0000454017705ab80470d089c7f644e" +
				"39cc9e0fd308e"),
		},
		outpoint:  outpoint,
		wantEntry: entry,
	}, {
		name: "entry is non-nil but has zero length",
		backendEntries: map[wire.OutPoint][]byte{
			outpoint: hexToBytes(""),
		},
		outpoint: outpoint,
		wantErr:  true,
	}, {
		name: "deserialization error",
		backendEntries: map[wire.OutPoint][]byte{
			outpoint: hexToBytes("812b"),
		},
		outpoint: outpoint,
		wantErr:  true,
	}}

	for _, test := range tests {
		// Add entries specified by the test to the test backend.
		err := backend.db.Update(func(dbTx database.Tx) error {
			for outpoint, serialized := range test.backendEntries {
				key := outpointKey(outpoint)
				err := dbTx.Metadata().Bucket(utxoSetBucketName).Put(*key, serialized)
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("%q: unexpected error adding entries to test backend: %v",
				test.name, err)
		}

		// Attempt to fetch the entry for the outpoint specified by the test.
		entry, err = backend.FetchEntry(test.outpoint)
		if test.wantErr && err == nil {
			t.Fatalf("%q: did not receive expected error", test.name)
		}
		if !test.wantErr && err != nil {
			t.Fatalf("%q: unexpected error fetching entry: %v", test.name, err)
		}

		// Ensure that the fetched entry matches the expected entry.
		if !reflect.DeepEqual(entry, test.wantEntry) {
			t.Fatalf("%q: mismatched entry:\nwant: %+v\n got: %+v\n", test.name,
				test.wantEntry, entry)
		}
	}
}
