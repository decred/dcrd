// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package spendpruner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/decred/dcrd/database/v2"
	_ "github.com/decred/dcrd/database/v2/ffldb"
	"github.com/decred/dcrd/wire"
)

// TestSerializeDeserializeConsumerDeps ensures consumer dependencies
// serialize and deserialize as intended.
func TestSerializeDeserializeConsumerDeps(t *testing.T) {
	deps := []string{"alpha", "bravo", "charlie", "echo"}

	serializedBytes := serializeSpendConsumerDeps(deps)
	deserializedDeps := deserializeSpendConsumerDeps(serializedBytes)

	for _, deserializedDep := range deserializedDeps {
		match := false
		for _, dep := range deps {
			if deserializedDep == dep {
				match = true
				break
			}
		}

		if !match {
			t.Fatalf("deserialized dependency %s, not found in "+
				"original set", deserializedDep)
		}
	}
}

// createdDB creates the test database. This is intended to be used
// for testing purposes only.
func createDB() (database.DB, func(), error) {
	dbPath := filepath.Join(os.TempDir(), "spdb")

	err := os.MkdirAll(dbPath, 0700)
	if err != nil {
		return nil, nil, err
	}

	db, err := database.Create("ffldb", dbPath, wire.SimNet)
	if err != nil {
		return nil, nil, err
	}

	err = initConsumerDepsBucket(db)
	if err != nil {
		return nil, nil, err
	}

	teardown := func() {
		db.Close()
		os.RemoveAll(dbPath)
	}

	return db, teardown, nil
}
