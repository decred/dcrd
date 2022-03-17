// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2016-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/decred/dcrd/blockchain/v4"
	"github.com/decred/dcrd/blockchain/v4/fullblocktests"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/wire"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

const (
	// testDbType is the database backend type to use for the tests.
	testDbType = "ffldb"

	// blockDataNet is the expected network in the test block data.
	blockDataNet = wire.MainNet
)

// isSupportedDbType returns whether or not the passed database type is
// currently supported.
func isSupportedDbType(dbType string) bool {
	supportedDrivers := database.SupportedDrivers()
	for _, driver := range supportedDrivers {
		if dbType == driver {
			return true
		}
	}

	return false
}

// createTestDatabase creates a test database with the provided database name
// and database type for the given network.
func createTestDatabase(t testing.TB, dbType string, net wire.CurrencyNet) (database.DB, error) {
	// Handle memory database specially since it doesn't need the disk specific
	// handling.
	var db database.DB
	if dbType == "memdb" {
		ndb, err := database.Create(dbType)
		if err != nil {
			return nil, fmt.Errorf("error creating db: %w", err)
		}
		db = ndb
	} else {
		// Create the directory for the test database.
		dbPath := t.TempDir()

		// Create the test database.
		ndb, err := database.Create(dbType, dbPath, net)
		if err != nil {
			return nil, fmt.Errorf("error creating db: %w", err)
		}
		db = ndb
	}
	t.Cleanup(func() {
		db.Close()
	})

	return db, nil
}

// createTestUtxoDatabase creates a test UTXO database with the provided
// database name.
func createTestUtxoDatabase(t testing.TB) (*leveldb.DB, func(), error) {
	// Construct the database filepath
	dbPath := t.TempDir()

	// Open the database (will create it if needed).
	opts := opt.Options{
		Strict:      opt.DefaultStrict,
		Compression: opt.NoCompression,
		Filter:      filter.NewBloomFilter(10),
	}
	db, err := leveldb.OpenFile(dbPath, &opts)
	if err != nil {
		return nil, nil, err
	}

	// Setup a teardown function for cleaning up.  This function is returned to
	// the caller to be invoked when it is done testing.
	teardown := func() {
		_ = db.Close()
		_ = os.RemoveAll(dbPath)
	}

	return db, teardown, nil
}

// chainSetup is used to create a new db and chain instance with the genesis
// block already inserted.
func chainSetup(t testing.TB, params *chaincfg.Params) (*blockchain.BlockChain, error) {
	if !isSupportedDbType(testDbType) {
		return nil, fmt.Errorf("unsupported db type %v", testDbType)
	}

	// Create a test block database.
	db, err := createTestDatabase(t, testDbType, blockDataNet)
	if err != nil {
		return nil, err
	}

	// Create a test UTXO database.
	utxoDb, teardownUtxoDb, err := createTestUtxoDatabase(t)
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() {
		teardownUtxoDb()
	})

	// Copy the chain params to ensure any modifications the tests do to
	// the chain parameters do not affect the global instance.
	paramsCopy := *params

	// Create a SigCache instance.
	sigCache, err := txscript.NewSigCache(1000)
	if err != nil {
		return nil, err
	}

	// Create the main chain instance.
	utxoBackend := blockchain.NewLevelDbUtxoBackend(utxoDb)
	chain, err := blockchain.New(context.Background(),
		&blockchain.Config{
			DB:          db,
			UtxoBackend: utxoBackend,
			ChainParams: &paramsCopy,
			TimeSource:  blockchain.NewMedianTime(),
			SigCache:    sigCache,
			UtxoCache: blockchain.NewUtxoCache(&blockchain.UtxoCacheConfig{
				Backend: utxoBackend,
				FlushBlockDB: func() error {
					// Don't flush to disk since it is slow and this is used in a lot of
					// tests.
					return nil
				},
				MaxSize: 100 * 1024 * 1024, // 100 MiB
			}),
		})

	if err != nil {
		err := fmt.Errorf("failed to create chain instance: %w", err)
		return nil, err
	}

	return chain, nil
}

// TestFullBlocks ensures all tests generated by the fullblocktests package
// have the expected result when processed via ProcessBlock.
func TestFullBlocks(t *testing.T) {
	tests, err := fullblocktests.Generate(false)
	if err != nil {
		t.Fatalf("failed to generate tests: %v", err)
	}

	// Create a new database and chain instance to run tests against.
	chain, err := chainSetup(t, chaincfg.RegNetParams())
	if err != nil {
		t.Fatalf("Failed to setup chain instance: %v", err)
	}

	// testAcceptedBlock attempts to process the block in the provided test
	// instance and ensures that it was accepted according to the flags
	// specified in the test.
	testAcceptedBlock := func(item fullblocktests.AcceptedBlock) {
		blockHeight := item.Block.Header.Height
		block := dcrutil.NewBlock(item.Block)
		t.Logf("Testing block %s (hash %s, height %d)",
			item.Name, block.Hash(), blockHeight)

		var isOrphan bool
		forkLen, err := chain.ProcessBlock(block)
		if errors.Is(err, blockchain.ErrMissingParent) {
			isOrphan = true
			err = nil
		}
		if err != nil {
			t.Fatalf("block %q (hash %s, height %d) should have "+
				"been accepted: %v", item.Name, block.Hash(),
				blockHeight, err)
		}

		// Ensure the main chain and orphan flags match the values
		// specified in the test.
		isMainChain := !isOrphan && forkLen == 0
		if isMainChain != item.IsMainChain {
			t.Fatalf("block %q (hash %s, height %d) unexpected main "+
				"chain flag -- got %v, want %v", item.Name,
				block.Hash(), blockHeight, isMainChain,
				item.IsMainChain)
		}
		if isOrphan != item.IsOrphan {
			t.Fatalf("block %q (hash %s, height %d) unexpected "+
				"orphan flag -- got %v, want %v", item.Name,
				block.Hash(), blockHeight, isOrphan,
				item.IsOrphan)
		}
	}

	// testRejectedBlock attempts to process the block in the provided test
	// instance and ensures that it was rejected with the reject code
	// specified in the test.
	testRejectedBlock := func(item fullblocktests.RejectedBlock) {
		blockHeight := item.Block.Header.Height
		block := dcrutil.NewBlock(item.Block)
		t.Logf("Testing block %s (hash %s, height %d)",
			item.Name, block.Hash(), blockHeight)

		_, err := chain.ProcessBlock(block)
		if err == nil {
			t.Fatalf("block %q (hash %s, height %d) should not "+
				"have been accepted", item.Name, block.Hash(),
				blockHeight)
		}

		// Ensure the error reject kind matches the value specified in the test
		// instance.
		if !errors.Is(err, item.RejectKind) {
			t.Fatalf("block %q (hash %s, height %d) does not have "+
				"expected reject code -- got %v, want %v",
				item.Name, block.Hash(), blockHeight, err, item.RejectKind)
		}
	}

	// testRejectedNonCanonicalBlock attempts to decode the block in the
	// provided test instance and ensures that it failed to decode with a
	// message error.
	testRejectedNonCanonicalBlock := func(item fullblocktests.RejectedNonCanonicalBlock) {
		headerLen := wire.MaxBlockHeaderPayload
		if headerLen > len(item.RawBlock) {
			headerLen = len(item.RawBlock)
		}
		blockHeader := item.RawBlock[0:headerLen]
		blockHash := chainhash.HashH(chainhash.HashB(blockHeader))
		blockHeight := item.Height
		t.Logf("Testing block %s (hash %s, height %d)", item.Name,
			blockHash, blockHeight)

		// Ensure there is an error due to deserializing the block.
		var msgBlock wire.MsgBlock
		err := msgBlock.BtcDecode(bytes.NewReader(item.RawBlock), 0)
		var werr *wire.MessageError
		if !errors.As(err, &werr) {
			t.Fatalf("block %q (hash %s, height %d) should have "+
				"failed to decode", item.Name, blockHash,
				blockHeight)
		}
	}

	// testOrphanOrRejectedBlock attempts to process the block in the
	// provided test instance and ensures that it was either accepted as an
	// orphan or rejected with a rule violation.
	testOrphanOrRejectedBlock := func(item fullblocktests.OrphanOrRejectedBlock) {
		blockHeight := item.Block.Header.Height
		block := dcrutil.NewBlock(item.Block)
		t.Logf("Testing block %s (hash %s, height %d)",
			item.Name, block.Hash(), blockHeight)

		_, err := chain.ProcessBlock(block)
		if err != nil {
			// Ensure the error is of the expected type.  Note that orphans are
			// rejected with ErrMissingParent, so this check covers both
			// conditions.
			var rerr blockchain.RuleError
			if !errors.As(err, &rerr) {
				t.Fatalf("block %q (hash %s, height %d) "+
					"returned unexpected error type -- "+
					"got %T, want blockchain.RuleError",
					item.Name, block.Hash(), blockHeight,
					err)
			}
		}
	}

	// testExpectedTip ensures the current tip of the blockchain is the
	// block specified in the provided test instance.
	testExpectedTip := func(item fullblocktests.ExpectedTip) {
		blockHeight := item.Block.Header.Height
		block := dcrutil.NewBlock(item.Block)
		t.Logf("Testing tip for block %s (hash %s, height %d)",
			item.Name, block.Hash(), blockHeight)

		// Ensure hash and height match.
		best := chain.BestSnapshot()
		if best.Hash != item.Block.BlockHash() ||
			best.Height != int64(blockHeight) {

			t.Fatalf("block %q (hash %s, height %d) should be "+
				"the current tip -- got (hash %s, height %d)",
				item.Name, block.Hash(), blockHeight, best.Hash,
				best.Height)
		}
	}

	for testNum, test := range tests {
		for itemNum, item := range test {
			switch item := item.(type) {
			case fullblocktests.AcceptedBlock:
				testAcceptedBlock(item)
			case fullblocktests.RejectedBlock:
				testRejectedBlock(item)
			case fullblocktests.RejectedNonCanonicalBlock:
				testRejectedNonCanonicalBlock(item)
			case fullblocktests.OrphanOrRejectedBlock:
				testOrphanOrRejectedBlock(item)
			case fullblocktests.ExpectedTip:
				testExpectedTip(item)
			default:
				t.Fatalf("test #%d, item #%d is not one of "+
					"the supported test instance types -- "+
					"got type: %T", testNum, itemNum, item)
			}
		}
	}
}
