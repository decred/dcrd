// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package spendpruner

import (
	"bytes"
	"encoding/binary"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database/v3"
)

const (
	// depsSeparator is the character used in separating spend journal consumer
	// dependencies when serializing.
	depsSeparator = ','
)

var (
	// spendJournalHeightsBucketName is the name of the bucket used in
	// storing spend journal heights.
	spendJournalHeightsBucketName = []byte("spendheights")
)

// initConsumerDepsBucket creates the spend consumer dependencies bucket if it
// does not exist.
func initConsumerDepsBucket(db database.DB) error {
	// Create the spend consumer dependencies bucket if it does not exist yet.
	return db.Update(func(dbTx database.Tx) error {
		meta := dbTx.Metadata()
		_, err := meta.CreateBucketIfNotExists(spendConsumerDepsBucketName)
		return err
	})
}

// initSpendJournalHeightsBucket creates the spend journal heights bucket
// if it does not exist.
func initSpendJournalHeightsBucket(db database.DB) error {
	// Create the spend journal heights bucket if it does not exist yet.
	return db.Update(func(dbTx database.Tx) error {
		meta := dbTx.Metadata()
		_, err := meta.CreateBucketIfNotExists(spendJournalHeightsBucketName)
		return err
	})
}

	var buf bytes.Buffer
	for idx := 0; idx < len(deps); idx++ {
		buf.WriteString(deps[idx])
		if idx < len(deps)-1 {
			buf.WriteByte(depsSeparator)
		}
	}

	return buf.Bytes()
}

// deserializeSpendConsumerDeps returns deserialized spend consumer
// dependencies from the provided serialized bytes.
func deserializeSpendConsumerDeps(serializedBytes []byte) []string {
	depBytes := bytes.Split(serializedBytes, []byte{depsSeparator})
	deps := make([]string, len(depBytes))
	for idx := 0; idx < len(depBytes); idx++ {
		deps[idx] = string(depBytes[idx])
	}

	return deps
}

// dbUpdateSpendConsumerDeps uses an existing database transaction to update
// the spend consumer dependency entry for the provided block hash.
func dbUpdateSpendConsumerDeps(dbTx database.Tx, blockHash chainhash.Hash, consumerDeps []string) error {
	depsBucket := dbTx.Metadata().Bucket(spendConsumerDepsBucketName)

	// Remove the dependency entry if there are no spend consumer dependencies
	// left for the block hash.
	if len(consumerDeps) == 0 {
		return depsBucket.Delete(blockHash[:])
	}

	// Update the dependency entry.
	serialized := serializeSpendConsumerDeps(consumerDeps)
	return depsBucket.Put(blockHash[:], serialized)
}

// dbPersistSpendHeights uses an existing database transaction to persist the
// provided spend heights.
func dbPersistSpendHeights(dbTx database.Tx, spendHeights map[chainhash.Hash]uint32) error {
	heightsBucket := dbTx.Metadata().Bucket(spendJournalHeightsBucketName)

	// return immediately if there are no spend heights to persist.
	if len(spendHeights) == 0 {
		return nil
	}

	// Persist all spend height map entries.
	for blockHash, height := range spendHeights {
		var b [8]byte
		binary.LittleEndian.PutUint32(b[:], height)
		err := heightsBucket.Put(blockHash[:], b[:])
		if err != nil {
			return err
		}
	}

	return nil
}

// dbRemoveSpendHeight uses an existing database transaction to
// remove the spend height entry for the provided block hash.
func dbRemoveSpendHeight(dbTx database.Tx, blockHash chainhash.Hash) error {
	heightsBucket := dbTx.Metadata().Bucket(spendJournalHeightsBucketName)
	return heightsBucket.Delete(blockHash[:])
}

// dbPruneSpendDependencies uses an existing database transaction to prune the
// spend dependencies associated with the provided keys.
func dbPruneSpendDependencies(dbTx database.Tx, keys []chainhash.Hash) error {
	depsBucket := dbTx.Metadata().Bucket(spendConsumerDependenciesBucketName)

	// Return immediately if there are no spend dependencies to remove.
	if len(keys) == 0 {
		return nil
	}

	// Prune all spend dependency map entries.
	for _, blockHash := range keys {
		err := depsBucket.Delete(blockHash[:])
		if err != nil {
			return err
		}
	}

	return nil
}

// dbPruneSpendHeights uses an existing database transaction to prune the
// provided spend heights associated with the provided keys.
func dbPruneSpendHeights(dbTx database.Tx, keys []chainhash.Hash) error {
	heightsBucket := dbTx.Metadata().Bucket(spendJournalHeightsBucketName)

	// Return immediately if there are no spend heights to remove.
	if len(keys) == 0 {
		return nil
	}

	// Persist all spend height map entries.
	for _, blockHash := range keys {
		err := heightsBucket.Delete(blockHash[:])
		if err != nil {
			return err
		}
	}

	return nil
}
	consumerDeps := make(map[chainhash.Hash][]string)
	cursor := depsBucket.Cursor()
	for ok := cursor.First(); ok; ok = cursor.Next() {
		hash, err := chainhash.NewHash(cursor.Key())
		if err != nil {
			return nil, err
		}

		deps := deserializeSpendConsumerDeps(cursor.Value())
		consumerDeps[*hash] = deps
	}

	return consumerDeps, nil
}

// dbFetchSpendHeights uses an existing database transaction to fetch all
// spend journal height entries in the database.
func dbFetchSpendHeights(dbTx database.Tx) (map[chainhash.Hash]uint32, error) {
	heightsBucket := dbTx.Metadata().Bucket(spendJournalHeightsBucketName)
	spendHeights := make(map[chainhash.Hash]uint32)
	cursor := heightsBucket.Cursor()
	for ok := cursor.First(); ok; ok = cursor.Next() {
		hash, err := chainhash.NewHash(cursor.Key())
		if err != nil {
			return nil, err
		}

		height := binary.LittleEndian.Uint32(cursor.Value())
		spendHeights[*hash] = height
	}

	return spendHeights, nil
}
