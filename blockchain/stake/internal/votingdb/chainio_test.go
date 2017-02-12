// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package votingdb

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ffldb"
)

const (
	// testDbType is the database backend type to use for the tests.
	testDbType = "ffldb"

	// testDbRoot is the root directory used to create all test databases.
	testDbRoot = "testdbs"
)

// hexToBytes converts a hex string to bytes, without returning any errors.
func hexToBytes(s string) []byte {
	b, _ := hex.DecodeString(s)

	return b
}

// newShaHashFromStr converts a 64 character hex string to a chainhash.Hash.
func newShaHashFromStr(s string) *chainhash.Hash {
	h, _ := chainhash.NewHashFromStr(s)

	return h
}

// TestDatabaseInfoSerialization ensures serializing and deserializing the
// database version information works as expected.
func TestDatabaseInfoSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		info       DatabaseInfo
		serialized []byte
	}{
		{
			name: "not upgrade",
			info: DatabaseInfo{
				Version:        currentDatabaseVersion,
				Date:           time.Unix(int64(0x57acca95), 0),
				UpgradeStarted: false,
			},
			serialized: hexToBytes("0100000095caac57"),
		},
		{
			name: "upgrade",
			info: DatabaseInfo{
				Version:        currentDatabaseVersion,
				Date:           time.Unix(int64(0x57acca95), 0),
				UpgradeStarted: true,
			},
			serialized: hexToBytes("0100008095caac57"),
		},
	}

	for i, test := range tests {
		// Ensure the state serializes to the expected value.
		gotBytes := serializeDatabaseInfo(&test.info)
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeDatabaseInfo #%d (%s): mismatched "+
				"bytes - got %x, want %x", i, test.name,
				gotBytes, test.serialized)
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// state.
		info, err := deserializeDatabaseInfo(test.serialized)
		if err != nil {
			t.Errorf("deserializeDatabaseInfo #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}
		if !reflect.DeepEqual(info, &test.info) {
			t.Errorf("deserializeDatabaseInfo #%d (%s) "+
				"mismatched state - got %v, want %v", i,
				test.name, info, test.info)
			continue
		}
	}
}

// TestDbInfoDeserializeErrors performs negative tests against
// deserializing the database information to ensure error paths
// work as expected.
func TestDbInfoDeserializeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serialized []byte
		errCode    ErrorCode
	}{
		{
			name:       "short read",
			serialized: hexToBytes("0000"),
			errCode:    ErrDatabaseInfoShortRead,
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned.
		_, err := deserializeDatabaseInfo(test.serialized)
		ticketDBErr, ok := err.(DBError)
		if !ok {
			t.Errorf("couldn't convert deserializeDatabaseInfo error "+
				"to ticket db error (err: %v)", err)
			continue
		}
		if ticketDBErr.GetCode() != test.errCode {
			t.Errorf("deserializeDatabaseInfo (%s): expected error type "+
				"does not match - got %v, want %v", test.name,
				ticketDBErr.ErrorCode, test.errCode)
			continue
		}
	}
}

// TestBestChainStateSerialization ensures serializing and deserializing the
// best chain state works as expected.
func TestBestChainStateSerialization(t *testing.T) {
	t.Parallel()

	currentTally := make([]byte, 100)
	currentTally[0] = 0xFF

	tests := []struct {
		name       string
		state      BestChainState
		serialized []byte
	}{
		{
			name: "generic block",
			state: BestChainState{
				Hash:         *newShaHashFromStr("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"),
				Height:       12323,
				CurrentTally: currentTally,
			},
			serialized: hexToBytes("6fe28c0ab6f1b372c1a6a246ae63f74f931e8365e15a089c68d619000000000023300000ff000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		},
	}

	for i, test := range tests {
		// Ensure the state serializes to the expected value.
		gotBytes := serializeBestChainState(test.state)
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeBestChainState #%d (%s): mismatched "+
				"bytes - got %x, want %x", i, test.name,
				gotBytes, test.serialized)
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// state.
		state, err := deserializeBestChainState(test.serialized)
		if err != nil {
			t.Errorf("deserializeBestChainState #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}
		if !reflect.DeepEqual(state, test.state) {
			t.Errorf("deserializeBestChainState #%d (%s) "+
				"mismatched state - got %v, want %v", i,
				test.name, state, test.state)
			continue

		}
	}
}

// TestBestChainStateDeserializeErrors performs negative tests against
// deserializing the chain state to ensure error paths work as expected.
func TestBestChainStateDeserializeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serialized []byte
		errCode    ErrorCode
	}{
		{
			name:       "short read",
			serialized: hexToBytes("0000"),
			errCode:    ErrChainStateShortRead,
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned.
		_, err := deserializeBestChainState(test.serialized)
		ticketDBErr, ok := err.(DBError)
		if !ok {
			t.Errorf("couldn't convert deserializeBestChainState error "+
				"to ticket db error (err: %v)", err)
			continue
		}
		if ticketDBErr.GetCode() != test.errCode {
			t.Errorf("deserializeBestChainState (%s): expected error type "+
				"does not match - got %v, want %v", test.name,
				ticketDBErr.ErrorCode, test.errCode)
			continue
		}
	}
}

// TestLiveDatabase tests various functions that require a live database.
func TestLiveDatabase(t *testing.T) {
	// Create a new database to store the accepted stake node data into.
	dbName := "ffldb_ticketdb_test"
	dbPath := filepath.Join(testDbRoot, dbName)
	_ = os.RemoveAll(dbPath)
	testDb, err := database.Create(testDbType, dbPath, chaincfg.SimNetParams.Net)
	if err != nil {
		t.Fatalf("error creating db: %v", err)
	}

	// Setup a teardown.
	defer os.RemoveAll(dbPath)
	defer os.RemoveAll(testDbRoot)
	defer testDb.Close()

	// Initialize the database, then try to read the version.
	err = testDb.Update(func(dbTx database.Tx) error {
		return DbCreate(dbTx)
	})
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	var dbi *DatabaseInfo
	err = testDb.View(func(dbTx database.Tx) error {
		dbi, err = DbFetchDatabaseInfo(dbTx)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	if dbi.Version != currentDatabaseVersion {
		t.Fatalf("bad version after reading from DB; want %v, got %v",
			currentDatabaseVersion, dbi.Version)
	}

	// Test storing some tally data.
	keys := make([][36]byte, 10)
	tallies := make([][100]byte, 10)
	for i := 0; i < 10; i++ {
		keys[i][0] = byte(i + 10)
		tallies[i][36] = byte(i + 20)
	}

	// Test put tallies.
	err = testDb.Update(func(dbTx database.Tx) error {
		for i := 0; i < 10; i++ {
			err = DbPutBlockTally(dbTx, keys[i][:], tallies[i][:])
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	// Test fetch tallies.
	talliesRead := make([][100]byte, 10)
	err = testDb.View(func(dbTx database.Tx) error {
		for i := 0; i < 10; i++ {
			tally, err := DbFetchBlockTally(dbTx, keys[i][0:36])
			if err != nil {
				return err
			}

			copy(talliesRead[i][:], tally[:])
		}

		return nil
	})
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	if !reflect.DeepEqual(tallies, talliesRead) {
		t.Errorf("failed to read stored tallies from database: stored %v, "+
			"read %v", tallies, talliesRead)
	}

	// Test put best state.
	best := BestChainState{
		Hash:         chainhash.Hash{0xff},
		Height:       55555,
		CurrentTally: tallies[0][:],
	}
	err = testDb.Update(func(dbTx database.Tx) error {
		err = DbPutBestState(dbTx, best)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	// Test read best state.
	var bestRead BestChainState
	err = testDb.View(func(dbTx database.Tx) error {
		bestRead, err = DbFetchBestState(dbTx)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	if !reflect.DeepEqual(best, bestRead) {
		t.Errorf("failed to read stored best state from database: stored %v, "+
			"read %v", best, bestRead)
	}
}
