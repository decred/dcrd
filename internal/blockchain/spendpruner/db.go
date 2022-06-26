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
	// spendConsumerDependenciesBucketName is the name of the bucket used in
	// storing spend journal consumer dependencies.
	spendConsumerDependenciesBucketName = []byte("spendconsumerdeps")
	// spendJournalHeightsBucketName is the name of the bucket used in
	// storing spend journal heights.
	spendJournalHeightsBucketName = []byte("spendheights")
)

// initConsumerDependenciesBucket creates the spend consumer dependencies
// bucket if it does not exist.
func initConsumerDependenciesBucket(db database.DB) error {
	// Create the spend consumer dependencies bucket if it does not exist yet.
	return db.Update(func(dbTx database.Tx) error {
		meta := dbTx.Metadata()
		_, err := meta.CreateBucketIfNotExists(spendConsumerDependenciesBucketName)
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

// serializeSpendConsumerDependencies returns serialized bytes of the provided
// spend journal consumer dependencies.
func serializeSpendConsumerDependencies(deps []string) []byte {
	var buf bytes.Buffer
	for idx := 0; idx < len(deps); idx++ {
		buf.WriteString(deps[idx])
		if idx < len(deps)-1 {
			buf.WriteByte(depsSeparator)
		}
	}

	return buf.Bytes()
}

// deserializeSpendConsumerDependencies returns deserialized spend consumer
// dependencies from the provided serialized bytes.
func deserializeSpendConsumerDependencies(serializedBytes []byte) []string {
	depBytes := bytes.Split(serializedBytes, []byte{depsSeparator})
	deps := make([]string, len(depBytes))
	for idx := 0; idx < len(depBytes); idx++ {
		deps[idx] = string(depBytes[idx])
	}

	return deps
}

// dbUpdateSpendConsumerDependencies uses an existing database transaction to
// update the spend consumer dependencies entry for the provided block hash.
func dbUpdateSpendConsumerDependencies(dbTx database.Tx, blockHash chainhash.Hash, consumerDeps []string) error {
	depsBucket := dbTx.Metadata().Bucket(spendConsumerDependenciesBucketName)

	// Remove the dependency entry if there are no spend consumer dependencies
	// left for the block hash.
	if len(consumerDeps) == 0 {
		return depsBucket.Delete(blockHash[:])
	}

	// Update the dependency entry.
	serialized := serializeSpendConsumerDependencies(consumerDeps)
	return depsBucket.Put(blockHash[:], serialized)
}

// dbPersistSpendHeights uses an existing database transaction to persist the
// provided spend heights.
func dbPersistSpendHeights(dbTx database.Tx, spendHeights map[chainhash.Hash]uint32) error {
	heightsBucket := dbTx.Metadata().Bucket(spendJournalHeightsBucketName)

	// Return immediately if there are no spend heights to persist.
	if len(spendHeights) == 0 {
		return nil
	}

	// Persist all spend height map entries.
	for blockHash, height := range spendHeights {
		hashCopy := blockHash

		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], height)
		err := heightsBucket.Put(hashCopy[:], b[:])
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
	for idx := range keys {
		err := depsBucket.Delete(keys[idx][:])
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
	for idx := range keys {
		err := heightsBucket.Delete(keys[idx][:])
		if err != nil {
			return err
		}
	}

	return nil
}

// dbFetchSpendConsumerDependencies uses an existing database transaction to
// fetch all spend consumer dependency entries in the database.
func dbFetchSpendConsumerDependencies(dbTx database.Tx) (map[chainhash.Hash][]string, error) {
	depsBucket := dbTx.Metadata().Bucket(spendConsumerDependenciesBucketName)
	consumerDeps := make(map[chainhash.Hash][]string)
	cursor := depsBucket.Cursor()
	for ok := cursor.First(); ok; ok = cursor.Next() {
		hash, err := chainhash.NewHash(cursor.Key())
		if err != nil {
			return nil, err
		}

		deps := deserializeSpendConsumerDependencies(cursor.Value())
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
		var hash chainhash.Hash
		copy(hash[:], cursor.Key())

		height := binary.LittleEndian.Uint32(cursor.Value())
		spendHeights[hash] = height
	}

	return spendHeights, nil
}
