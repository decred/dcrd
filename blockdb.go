// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"errors"
	_ "net/http/pprof"
	"os"
	"path/filepath"

	"github.com/decred/dcrd/blockchain/v4"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/internal/progresslog"
)

const (
	// blockDbNamePrefix is the prefix for the block database name.  The
	// database type is appended to this value to form the full block
	// database name.
	blockDbNamePrefix = "blocks"
)

// removeDB removes the database at the provided path.  The fi parameter MUST
// agree with the provided path.
func removeDB(dbPath string, fi os.FileInfo) error {
	if fi.IsDir() {
		return os.RemoveAll(dbPath)
	}

	return os.Remove(dbPath)
}

// removeRegressionDB removes the existing regression test database if running
// in regression test mode and it already exists.
func removeRegressionDB(dbPath string) error {
	// Don't do anything if not in regression test mode.
	if !cfg.RegNet {
		return nil
	}

	// Remove the old regression test database if it already exists.
	fi, err := os.Stat(dbPath)
	if err == nil {
		dcrdLog.Infof("Removing regression test database from '%s'", dbPath)
		return removeDB(dbPath, fi)
	}

	return nil
}

// blockDbPath returns the path to the block database given a database type.
func blockDbPath(dbType string) string {
	// The database name is based on the database type.
	dbName := blockDbNamePrefix + "_" + dbType
	if dbType == "sqlite" {
		dbName = dbName + ".db"
	}
	dbPath := filepath.Join(cfg.DataDir, dbName)
	return dbPath
}

// warnMultipleDBs shows a warning if multiple block database types are detected.
// This is not a situation most users want.  It is handy for development however
// to support multiple side-by-side databases.
func warnMultipleDBs() {
	// This is intentionally not using the known db types which depend on the
	// database types compiled into the binary since we want to detect legacy db
	// types as well.
	dbTypes := []string{"ffldb", "leveldb", "sqlite"}
	duplicateDbPaths := make([]string, 0, len(dbTypes)-1)
	for _, dbType := range dbTypes {
		if dbType == cfg.DbType {
			continue
		}

		// Store db path as a duplicate db if it exists.
		dbPath := blockDbPath(dbType)
		if fileExists(dbPath) {
			duplicateDbPaths = append(duplicateDbPaths, dbPath)
		}
	}

	// Warn if there are extra databases.
	if len(duplicateDbPaths) > 0 {
		selectedDbPath := blockDbPath(cfg.DbType)
		dcrdLog.Warnf("WARNING: There are multiple block chain databases "+
			"using different database types.\nYou probably don't want to "+
			"waste disk space by having more than one.\nYour current database "+
			"is located at [%v].\nThe additional database is located at %v",
			selectedDbPath, duplicateDbPaths)
	}
}

// loadBlockDB loads (or creates when needed) the block database taking into
// account the selected database backend and returns a handle to it.  It also
// contains additional logic such warning the user if there are multiple
// databases which consume space on the file system and ensuring the regression
// test database is clean when in regression test mode.
func loadBlockDB(params *chaincfg.Params) (database.DB, error) {
	// The memdb backend does not have a file path associated with it, so handle
	// it uniquely.  We also don't want to worry about the multiple database
	// type warnings when running with the memory database.
	if cfg.DbType == "memdb" {
		dcrdLog.Infof("Creating block database in memory.")
		db, err := database.Create(cfg.DbType)
		if err != nil {
			return nil, err
		}
		return db, nil
	}

	warnMultipleDBs()

	// The database name is based on the database type.
	dbPath := blockDbPath(cfg.DbType)

	// The regression test is special in that it needs a clean database for
	// each run, so remove it now if it already exists.
	removeRegressionDB(dbPath)

	// createDB is a convenience func that creates the database with the type
	// and network specified in the config at the path determined above while
	// also creating any any intermediate directories in the configured data
	// directory path as needed.
	createDB := func() (database.DB, error) {
		// Create the data dir if it does not exist.
		err := os.MkdirAll(cfg.DataDir, 0700)
		if err != nil {
			return nil, err
		}
		return database.Create(cfg.DbType, dbPath, params.Net)
	}

	// Open the existing database or create a new one as needed.
	dcrdLog.Infof("Loading block database from '%s'", dbPath)
	db, err := database.Open(cfg.DbType, dbPath, params.Net)
	if err != nil {
		// Return the error if it's not because the database doesn't exist.
		if !errors.Is(err, database.ErrDbDoesNotExist) {
			return nil, err
		}

		db, err = createDB()
		if err != nil {
			return nil, err
		}
	}

	// Remove and recreate the blockchain database when it can no longer be
	// upgraded due to being too old.
	if err := blockchain.CheckDBTooOldToUpgrade(db); err != nil {
		// Any errors other than the database being too old upgrade are
		// unexpected.
		if !errors.Is(err, blockchain.ErrDBTooOldToUpgrade) {
			return nil, err
		}
		dcrdLog.Infof("Removing database from '%s': %v", dbPath, err)

		// Close the database so it can be removed cleanly.
		if err := db.Close(); err != nil {
			return nil, err
		}

		// Remove the old database and create/open a new one.
		fi, err := os.Stat(dbPath)
		if err != nil {
			return nil, err
		}

		err = removeDB(dbPath, fi)
		if err != nil {
			return nil, err
		}

		db, err = createDB()
		if err != nil {
			return nil, err
		}
	}

	dcrdLog.Info("Block database loaded")
	return db, nil
}

// dumpBlockChain dumps a map of the blockchain blocks as serialized bytes.
func dumpBlockChain(params *chaincfg.Params, b *blockchain.BlockChain) error {
	dcrdLog.Infof("Writing the blockchain to flat file %q.  This might take a "+
		"while...", cfg.DumpBlockchain)

	progressLogger := progresslog.New("Wrote", dcrdLog)

	file, err := os.Create(cfg.DumpBlockchain)
	if err != nil {
		return err
	}
	defer file.Close()

	// Store the network ID in an array for later writing.
	var net [4]byte
	binary.LittleEndian.PutUint32(net[:], uint32(params.Net))

	// Write the blocks sequentially, excluding the genesis block.
	tipHeight := b.BestSnapshot().Height
	var sz [4]byte
	for i := int64(1); i <= tipHeight; i++ {
		bl, err := b.BlockByHeight(i)
		if err != nil {
			return err
		}

		// Serialize the block for writing.
		blB, err := bl.Bytes()
		if err != nil {
			return err
		}

		// Write the network ID first.
		_, err = file.Write(net[:])
		if err != nil {
			return err
		}

		// Write the size of the block as a little endian uint32,
		// then write the block itself serialized.
		binary.LittleEndian.PutUint32(sz[:], uint32(len(blB)))
		_, err = file.Write(sz[:])
		if err != nil {
			return err
		}

		_, err = file.Write(blB)
		if err != nil {
			return err
		}

		msgBlock := bl.MsgBlock()
		forceLog := int64(msgBlock.Header.Height) >= tipHeight
		progressLogger.LogProgress(msgBlock, forceLog, func() float64 {
			return float64(msgBlock.Header.Height) / float64(tipHeight) * 100
		})
	}

	srvrLog.Infof("Successfully dumped the blockchain (%v blocks) to %v.",
		tipHeight, cfg.DumpBlockchain)

	return nil
}
