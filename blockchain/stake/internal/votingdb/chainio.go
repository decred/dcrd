// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package votingdb

import (
	"fmt"
	"time"

	"github.com/decred/dcrd/blockchain/stake/internal/dbnamespace"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
)

const (
	// upgradeStartedBit if the bit flag for whether or not a database
	// upgrade is in progress. It is used to determine if the database
	// is in an inconsistent state from the update.
	upgradeStartedBit = 0x80000000

	// currentDatabaseVersion indicates what the current database
	// version is.
	currentDatabaseVersion = 1
)

// databaseInfoSize is the serialized size of the best chain state in bytes.
const databaseInfoSize = 8

// DatabaseInfo is the structure for a database.
type DatabaseInfo struct {
	Version        uint32
	Date           time.Time
	UpgradeStarted bool
}

// serializeDatabaseInfo serializes a database information struct.
func serializeDatabaseInfo(dbi *DatabaseInfo) []byte {
	version := dbi.Version
	if dbi.UpgradeStarted {
		version |= upgradeStartedBit
	}

	val := make([]byte, databaseInfoSize)
	versionBytes := make([]byte, 4)
	dbnamespace.ByteOrder.PutUint32(versionBytes, version)
	copy(val[0:4], versionBytes)
	timestampBytes := make([]byte, 4)
	dbnamespace.ByteOrder.PutUint32(timestampBytes, uint32(dbi.Date.Unix()))
	copy(val[4:8], timestampBytes)

	return val
}

// DbPutDatabaseInfo uses an existing database transaction to store the database
// information.
func DbPutDatabaseInfo(dbTx database.Tx, dbi *DatabaseInfo) error {
	meta := dbTx.Metadata()
	subsidyBucket := meta.Bucket(dbnamespace.VotingDbInfoBucketName)
	val := serializeDatabaseInfo(dbi)

	// Store the current database info into the database.
	return subsidyBucket.Put(dbnamespace.VotingDbInfoBucketName, val[:])
}

// deserializeDatabaseInfo deserializes a database information struct.
func deserializeDatabaseInfo(dbInfoBytes []byte) (*DatabaseInfo, error) {
	if len(dbInfoBytes) < databaseInfoSize {
		return nil, votingDBError(ErrDatabaseInfoShortRead,
			"short read when deserializing voting database info")
	}

	rawVersion := dbnamespace.ByteOrder.Uint32(dbInfoBytes[0:4])
	upgradeStarted := (upgradeStartedBit & rawVersion) > 0
	version := rawVersion &^ upgradeStartedBit
	ts := dbnamespace.ByteOrder.Uint32(dbInfoBytes[4:8])

	return &DatabaseInfo{
		Version:        version,
		Date:           time.Unix(int64(ts), 0),
		UpgradeStarted: upgradeStarted,
	}, nil
}

// DbFetchDatabaseInfo uses an existing database transaction to
// fetch the database versioning and creation information.
func DbFetchDatabaseInfo(dbTx database.Tx) (*DatabaseInfo, error) {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(dbnamespace.VotingDbInfoBucketName)

	// Uninitialized state.
	if bucket == nil {
		return nil, nil
	}

	dbInfoBytes := bucket.Get(dbnamespace.VotingDbInfoBucketName)
	if dbInfoBytes == nil {
		return nil, votingDBError(ErrMissingKey, "missing key for voting "+
			"database info")
	}

	return deserializeDatabaseInfo(dbInfoBytes)
}

// -----------------------------------------------------------------------------
// The best chain state consists of the best block hash and height, the best
// block tally, and the tally of the previous interval block.
//
// The serialized format is:
//
//   <block hash><block height><current tally><last interval tally>
//
//   Field                Type              Size
//   block hash           chainhash.Hash    chainhash.HashSize
//   block height         uint32            4 bytes
//   current tally        []byte            100 bytes (preserialized)
// -----------------------------------------------------------------------------

// minimumBestChainStateSize is the minimum serialized size of the best chain
// state in bytes.
var minimumBestChainStateSize = chainhash.HashSize + 4 + 100

// BestChainState represents the data to be stored the database for the current
// best chain state.
type BestChainState struct {
	Hash         chainhash.Hash
	Height       uint32
	CurrentTally []byte
}

// serializeBestChainState returns the serialization of the passed block best
// chain state.  This is data to be stored in the chain state bucket. This
// function will panic if the number of tickets per block is less than the
// size of next winners, which should never happen unless there is memory
// corruption.
func serializeBestChainState(state BestChainState) []byte {
	// Serialize the chain state.
	serializedData := make([]byte, minimumBestChainStateSize)

	offset := 0
	copy(serializedData[offset:offset+chainhash.HashSize], state.Hash[:])
	offset += chainhash.HashSize
	dbnamespace.ByteOrder.PutUint32(serializedData[offset:], state.Height)
	offset += 4

	// Serialize the tallies.
	copy(serializedData[offset:], state.CurrentTally[:])
	offset += 100

	return serializedData[:]
}

// deserializeBestChainState deserializes the passed serialized best chain
// state.  This is data stored in the chain state bucket and is updated after
// every block is connected or disconnected form the main chain.
// block.
func deserializeBestChainState(serializedData []byte) (BestChainState, error) {
	// Ensure the serialized data has enough bytes to properly deserialize
	// the state.
	if len(serializedData) < minimumBestChainStateSize {
		return BestChainState{}, votingDBError(ErrChainStateShortRead,
			"short read when deserializing best chain voting state data")
	}

	state := BestChainState{}
	offset := 0
	copy(state.Hash[:], serializedData[offset:offset+chainhash.HashSize])
	offset += chainhash.HashSize
	state.Height = dbnamespace.ByteOrder.Uint32(serializedData[offset : offset+4])
	offset += 4
	state.CurrentTally = make([]byte, 100)
	copy(state.CurrentTally[:], serializedData[offset:])
	offset += 100

	return state, nil
}

// DbFetchBestState uses an existing database transaction to fetch the best chain
// state.
func DbFetchBestState(dbTx database.Tx) (BestChainState, error) {
	meta := dbTx.Metadata()
	v := meta.Get(dbnamespace.VotingChainStateKeyName)
	if v == nil {
		return BestChainState{}, votingDBError(ErrMissingKey,
			"missing key for chain state data")
	}

	return deserializeBestChainState(v)
}

// DbPutBestState uses an existing database transaction to update the best chain
// state with the given parameters.
func DbPutBestState(dbTx database.Tx, bcs BestChainState) error {
	// Serialize the current best chain state.
	serializedData := serializeBestChainState(bcs)

	// Store the current best chain state into the database.
	return dbTx.Metadata().Put(dbnamespace.VotingChainStateKeyName,
		serializedData)
}

// DbFetchBlockTally fetches an interval block's tally from the voting database.
func DbFetchBlockTally(dbTx database.Tx, blockKey []byte) ([]byte, error) {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(dbnamespace.IntervalBlockTallyBucketName)

	v := bucket.Get(blockKey[:])
	if v == nil {
		return nil, votingDBError(ErrMissingKey,
			fmt.Sprintf("missing key %x for db tally", blockKey))
	}

	if len(v) < 100 {
		return nil, votingDBError(ErrTallyShortRead,
			fmt.Sprintf("short read of db tally data (got %v, min %v)",
				len(v), 100))
	}
	serialized := make([]byte, 100)
	copy(serialized[:], v[:])

	return serialized, nil
}

// DbPutBlockTally inserts an interval block's tally into the voting database.
func DbPutBlockTally(dbTx database.Tx, key, serializedTally []byte) error {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(dbnamespace.IntervalBlockTallyBucketName)
	if len(key) < 36 {
		return votingDBError(ErrBlockKeyShortRead, fmt.Sprintf("block key "+
			"short read (got %v, min %v)", len(key), 36))
	}
	if len(serializedTally) < 100 {
		return votingDBError(ErrTallyShortRead, fmt.Sprintf("tally "+
			"short read (got %v, min %v)", len(serializedTally), 100))
	}

	return bucket.Put(key[:], serializedTally[:])
}

// DbDeleteBlockTally deletes an interval block's tally from the voting database.
func DbDeleteBlockTally(dbTx database.Tx, key []byte) error {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(dbnamespace.IntervalBlockTallyBucketName)
	if len(key) < 36 {
		return votingDBError(ErrBlockKeyShortRead, fmt.Sprintf("block key "+
			"short read (got %v, min %v)", len(key), 36))
	}
	v := bucket.Get(key[:])
	if v == nil {
		return votingDBError(ErrMissingKey,
			fmt.Sprintf("missing key %x for db tally", key))
	}

	return bucket.Delete(key[:])
}

// DbCreate initializes all the buckets required for the database and stores
// the current database version information.
func DbCreate(dbTx database.Tx) error {
	meta := dbTx.Metadata()

	// Create the bucket that houses information about the database's
	// creation and version.
	_, err := meta.CreateBucket(dbnamespace.VotingDbInfoBucketName)
	if err != nil {
		return err
	}

	dbInfo := &DatabaseInfo{
		Version:        currentDatabaseVersion,
		Date:           time.Now(),
		UpgradeStarted: false,
	}
	err = DbPutDatabaseInfo(dbTx, dbInfo)
	if err != nil {
		return err
	}

	// Create the bucket that houses the live tickets of the best node.
	_, err = meta.CreateBucket(dbnamespace.IntervalBlockTallyBucketName)
	if err != nil {
		return err
	}

	return nil
}
