// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/wire"
)

const (
	// utxoDbName is the UTXO database name.
	utxoDbName = "utxodb"

	// utxoDbDefaultDriver is the default driver to use for the UTXO database.
	utxoDbDefaultDriver = "ffldb"
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
func removeRegressionDB(net wire.CurrencyNet, dbPath string) error {
	// Don't do anything if not in regression test mode.
	if net != wire.RegNet {
		return nil
	}

	// Remove the old regression test database if it already exists.
	fi, err := os.Stat(dbPath)
	if err == nil {
		log.Infof("Removing regression test UTXO database from '%s'", dbPath)
		return removeDB(dbPath, fi)
	}

	return nil
}

// LoadUtxoDB loads (or creates when needed) the UTXO database and returns a
// handle to it.  It also contains additional logic such as ensuring the
// regression test database is clean when in regression test mode.
func LoadUtxoDB(params *chaincfg.Params, dataDir string) (database.DB, error) {
	// Set the database path based on the data directory and database name.
	dbPath := filepath.Join(dataDir, utxoDbName)

	// The regression test is special in that it needs a clean database for each
	// run, so remove it now if it already exists.
	removeRegressionDB(params.Net, dbPath)

	// createDB is a convenience func that creates the database with the type and
	// network specified in the config at the path determined above while also
	// creating any intermediate directories in the configured data directory path
	// as needed.
	createDB := func() (database.DB, error) {
		// Create the data dir if it does not exist.
		err := os.MkdirAll(dataDir, 0700)
		if err != nil {
			return nil, err
		}
		return database.Create(utxoDbDefaultDriver, dbPath, params.Net)
	}

	// Open the existing database or create a new one as needed.
	log.Infof("Loading UTXO database from '%s'", dbPath)
	db, err := database.Open(utxoDbDefaultDriver, dbPath, params.Net)
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

	log.Info("UTXO database loaded")

	return db, nil
}
