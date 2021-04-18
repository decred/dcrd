// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/wire"
)

const (
	// currentUtxoDatabaseVersion indicates the current UTXO database version.
	currentUtxoDatabaseVersion = 1

	// currentUtxoSetVersion indicates the current UTXO set database version.
	currentUtxoSetVersion = 3
)

var (
	// utxoDbInfoBucketName is the name of the database bucket used to house
	// global versioning and date information for the UTXO database.
	utxoDbInfoBucketName = []byte("dbinfo")

	// utxoDbInfoVersionKeyName is the name of the database key used to house the
	// database version.  It is itself under the utxoDbInfoBucketName bucket.
	utxoDbInfoVersionKeyName = []byte("version")

	// utxoDbInfoCompressionVerKeyName is the name of the database key used to
	// house the database compression version.  It is itself under the
	// utxoDbInfoBucketName bucket.
	utxoDbInfoCompressionVerKeyName = []byte("compver")

	// utxoDbInfoUtxoVerKeyName is the name of the database key used to house the
	// database UTXO set version.  It is itself under the utxoDbInfoBucketName
	// bucket.
	utxoDbInfoUtxoVerKeyName = []byte("utxover")

	// utxoDbInfoCreatedKeyName is the name of the database key used to house
	// the date the database was created.  It is itself under the
	// utxoDbInfoBucketName bucket.
	utxoDbInfoCreatedKeyName = []byte("created")

	// utxoSetBucketName is the name of the db bucket used to house the unspent
	// transaction output set.
	utxoSetBucketName = []byte("utxosetv3")

	// utxoSetStateKeyName is the name of the database key used to house the
	// state of the unspent transaction output set.
	utxoSetStateKeyName = []byte("utxosetstate")
)

// -----------------------------------------------------------------------------
// The UTXO database information contains information about the version and date
// of the UTXO database.
//
// It consists of a separate key for each individual piece of information:
//
//  Key        Value    Size      Description
//  version    uint32   4 bytes   The version of the database
//  compver    uint32   4 bytes   The script compression version of the database
//  utxover    uint32   4 bytes   The UTXO set version of the database
//  created    uint64   8 bytes   The date of the creation of the database
// -----------------------------------------------------------------------------

// utxoDatabaseInfo is the structure that holds the versioning and date
// information for the UTXO database.
type utxoDatabaseInfo struct {
	version uint32
	compVer uint32
	utxoVer uint32
	created time.Time
}

// dbPutUtxoDatabaseInfo uses an existing database transaction to store the
// database information.
func dbPutUtxoDatabaseInfo(dbTx database.Tx, dbi *utxoDatabaseInfo) error {
	// uint32Bytes is a helper function to convert a uint32 to a byte slice
	// using the byte order specified by the database namespace.
	uint32Bytes := func(ui32 uint32) []byte {
		var b [4]byte
		byteOrder.PutUint32(b[:], ui32)
		return b[:]
	}

	// uint64Bytes is a helper function to convert a uint64 to a byte slice
	// using the byte order specified by the database namespace.
	uint64Bytes := func(ui64 uint64) []byte {
		var b [8]byte
		byteOrder.PutUint64(b[:], ui64)
		return b[:]
	}

	// Store the database version.
	meta := dbTx.Metadata()
	bucket := meta.Bucket(utxoDbInfoBucketName)
	err := bucket.Put(utxoDbInfoVersionKeyName, uint32Bytes(dbi.version))
	if err != nil {
		return err
	}

	// Store the compression version.
	err = bucket.Put(utxoDbInfoCompressionVerKeyName, uint32Bytes(dbi.compVer))
	if err != nil {
		return err
	}

	// Store the UTXO set version.
	err = bucket.Put(utxoDbInfoUtxoVerKeyName, uint32Bytes(dbi.utxoVer))
	if err != nil {
		return err
	}

	// Store the database creation date.
	return bucket.Put(utxoDbInfoCreatedKeyName,
		uint64Bytes(uint64(dbi.created.Unix())))
}

// dbFetchUtxoDatabaseInfo uses an existing database transaction to fetch the
// database versioning and creation information.
func dbFetchUtxoDatabaseInfo(dbTx database.Tx) *utxoDatabaseInfo {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(utxoDbInfoBucketName)

	// Uninitialized state.
	if bucket == nil {
		return nil
	}

	// Load the database version.
	var version uint32
	versionBytes := bucket.Get(utxoDbInfoVersionKeyName)
	if versionBytes != nil {
		version = byteOrder.Uint32(versionBytes)
	}

	// Load the database compression version.
	var compVer uint32
	compVerBytes := bucket.Get(utxoDbInfoCompressionVerKeyName)
	if compVerBytes != nil {
		compVer = byteOrder.Uint32(compVerBytes)
	}

	// Load the database UTXO set version.
	var utxoVer uint32
	utxoVerBytes := bucket.Get(utxoDbInfoUtxoVerKeyName)
	if utxoVerBytes != nil {
		utxoVer = byteOrder.Uint32(utxoVerBytes)
	}

	// Load the database creation date.
	var created time.Time
	createdBytes := bucket.Get(utxoDbInfoCreatedKeyName)
	if createdBytes != nil {
		ts := byteOrder.Uint64(createdBytes)
		created = time.Unix(int64(ts), 0)
	}

	return &utxoDatabaseInfo{
		version: version,
		compVer: compVer,
		utxoVer: utxoVer,
		created: created,
	}
}

// -----------------------------------------------------------------------------
// The unspent transaction output (utxo) set consists of an entry for each
// unspent output using a format that is optimized to reduce space using domain
// specific compression algorithms.
//
// Each entry is keyed by an outpoint as specified below.  It is important to
// note that the key encoding uses a VLQ, which employs an MSB encoding so
// iteration of utxos when doing byte-wise comparisons will produce them in
// order.
//
// The serialized key format is:
//
//   <hash><tree><output index>
//
//   Field                Type             Size
//   hash                 chainhash.Hash   chainhash.HashSize
//   tree                 VLQ              variable
//   output index         VLQ              variable
//
// The serialized value format is:
//
//   <block height><block index><flags><compressed txout>
//   OPTIONAL: [<ticket min outs>]
//
//   Field                Type     Size
//   block height         VLQ      variable
//   block index          VLQ      variable
//   flags                VLQ      variable
//   compressed txout
//     compressed amount   VLQ      variable
//     script version      VLQ      variable
//     compressed script   []byte   variable
//
//   OPTIONAL
//     ticketMinOuts      []byte         variable
//
// The serialized flags format is:
//   bit  0     - containing transaction is a coinbase
//   bit  1     - containing transaction has an expiry
//   bits 2-5   - transaction type
//   bits 6-7   - unused
//
// The ticket min outs field contains minimally encoded outputs for all outputs
// of a ticket transaction. It is only encoded for ticket submission outputs.
//
// -----------------------------------------------------------------------------

// maxUint32VLQSerializeSize is the maximum number of bytes a max uint32 takes
// to serialize as a VLQ.
var maxUint32VLQSerializeSize = serializeSizeVLQ(1<<32 - 1)

// maxUint8VLQSerializeSize is the maximum number of bytes a max uint8 takes to
// serialize as a VLQ.
var maxUint8VLQSerializeSize = serializeSizeVLQ(1<<8 - 1)

// outpointKeyPool defines a concurrent safe free list of byte slices used to
// provide temporary buffers for outpoint database keys.
var outpointKeyPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, chainhash.HashSize+maxUint8VLQSerializeSize+
			maxUint32VLQSerializeSize)
		return &b // Pointer to slice to avoid boxing alloc.
	},
}

// outpointKey returns a key suitable for use as a database key in the utxo set
// while making use of a free list.  A new buffer is allocated if there are not
// already any available on the free list.  The returned byte slice should be
// returned to the free list by using the recycleOutpointKey function when the
// caller is done with it _unless_ the slice will need to live for longer than
// the caller can calculate such as when used to write to the database.
func outpointKey(outpoint wire.OutPoint) *[]byte {
	// A VLQ employs an MSB encoding, so they are useful not only to reduce the
	// amount of storage space, but also so iteration of utxos when doing
	// byte-wise comparisons will produce them in order.
	key := outpointKeyPool.Get().(*[]byte)
	tree := uint64(outpoint.Tree)
	idx := uint64(outpoint.Index)
	*key = (*key)[:chainhash.HashSize+serializeSizeVLQ(tree) +
		+serializeSizeVLQ(idx)]
	copy(*key, outpoint.Hash[:])
	offset := chainhash.HashSize
	offset += putVLQ((*key)[offset:], tree)
	putVLQ((*key)[offset:], idx)
	return key
}

// decodeOutpointKey decodes the passed serialized key into the passed outpoint.
func decodeOutpointKey(serialized []byte, outpoint *wire.OutPoint) error {
	if chainhash.HashSize >= len(serialized) {
		return errDeserialize("unexpected length for serialized outpoint key")
	}

	// Deserialize the hash.
	var hash chainhash.Hash
	copy(hash[:], serialized[:chainhash.HashSize])
	offset := chainhash.HashSize

	// Deserialize the tree.
	tree, bytesRead := deserializeVLQ(serialized[offset:])
	offset += bytesRead
	if offset >= len(serialized) {
		return errDeserialize("unexpected end of data after tree")
	}

	// Deserialize the index.
	idx, _ := deserializeVLQ(serialized[offset:])

	// Populate the outpoint.
	outpoint.Hash = hash
	outpoint.Tree = int8(tree)
	outpoint.Index = uint32(idx)

	return nil
}

// recycleOutpointKey puts the provided byte slice, which should have been
// obtained via the outpointKey function, back on the free list.
func recycleOutpointKey(key *[]byte) {
	outpointKeyPool.Put(key)
}

// serializeUtxoEntry returns the entry serialized to a format that is suitable
// for long-term storage.  The format is described in detail above.
func serializeUtxoEntry(entry *UtxoEntry) ([]byte, error) {
	// Spent entries have no serialization.
	if entry.IsSpent() {
		return nil, nil
	}

	// Calculate the size needed to serialize the entry.
	const hasAmount = true
	flags := encodeFlags(entry.IsCoinBase(), entry.HasExpiry(),
		entry.TransactionType())
	size := serializeSizeVLQ(uint64(entry.blockHeight)) +
		serializeSizeVLQ(uint64(entry.blockIndex)) +
		serializeSizeVLQ(uint64(flags)) +
		compressedTxOutSize(uint64(entry.amount), entry.scriptVersion,
			entry.pkScript, hasAmount)

	if entry.ticketMinOuts != nil {
		size += len(entry.ticketMinOuts.data)
	}

	// Serialize the entry.
	serialized := make([]byte, size)
	offset := putVLQ(serialized, uint64(entry.blockHeight))
	offset += putVLQ(serialized[offset:], uint64(entry.blockIndex))
	offset += putVLQ(serialized[offset:], uint64(flags))
	offset += putCompressedTxOut(serialized[offset:], uint64(entry.amount),
		entry.scriptVersion, entry.pkScript, hasAmount)

	if entry.ticketMinOuts != nil {
		copy(serialized[offset:], entry.ticketMinOuts.data)
	}

	return serialized, nil
}

// deserializeUtxoEntry decodes a utxo entry from the passed serialized byte
// slice into a new UtxoEntry using a format that is suitable for long-term
// storage.  The format is described in detail above.
func deserializeUtxoEntry(serialized []byte, txOutIndex uint32) (*UtxoEntry, error) {
	// Deserialize the block height.
	blockHeight, bytesRead := deserializeVLQ(serialized)
	offset := bytesRead
	if offset >= len(serialized) {
		return nil, errDeserialize("unexpected end of data after height")
	}

	// Deserialize the block index.
	blockIndex, bytesRead := deserializeVLQ(serialized[offset:])
	offset += bytesRead
	if offset >= len(serialized) {
		return nil, errDeserialize("unexpected end of data after index")
	}

	// Deserialize the flags.
	flags, bytesRead := deserializeVLQ(serialized[offset:])
	offset += bytesRead
	if offset >= len(serialized) {
		return nil, errDeserialize("unexpected end of data after flags")
	}
	isCoinBase, hasExpiry, txType := decodeFlags(txOutFlags(flags))

	// Decode the compressed unspent transaction output.
	amount, scriptVersion, script, bytesRead, err :=
		decodeCompressedTxOut(serialized[offset:], true)
	if err != nil {
		return nil, errDeserialize(fmt.Sprintf("unable to decode utxo: %v", err))
	}
	offset += bytesRead

	// Create a new utxo entry with the details deserialized above.
	entry := &UtxoEntry{
		amount:        amount,
		pkScript:      script,
		blockHeight:   uint32(blockHeight),
		blockIndex:    uint32(blockIndex),
		scriptVersion: scriptVersion,
		packedFlags:   encodeUtxoFlags(isCoinBase, hasExpiry, txType),
	}

	// Copy the minimal outputs if this was a ticket submission output.
	if isTicketSubmissionOutput(txType, txOutIndex) {
		sz, err := readDeserializeSizeOfMinimalOutputs(serialized[offset:])
		if err != nil {
			return nil, errDeserialize(fmt.Sprintf("unable to decode "+
				"ticket outputs: %v", err))
		}
		entry.ticketMinOuts = &ticketMinimalOutputs{
			data: make([]byte, sz),
		}
		copy(entry.ticketMinOuts.data, serialized[offset:offset+sz])
	}

	return entry, nil
}

// dbFetchUtxoEntry uses an existing database transaction to fetch the specified
// transaction output from the utxo set.
//
// When there is no entry for the provided output, nil will be returned for both
// the entry and the error.
func dbFetchUtxoEntry(dbTx database.Tx, outpoint wire.OutPoint) (*UtxoEntry, error) {
	// Fetch the unspent transaction output information for the passed transaction
	// output.  Return now when there is no entry.
	key := outpointKey(outpoint)
	utxoBucket := dbTx.Metadata().Bucket(utxoSetBucketName)
	serializedUtxo := utxoBucket.Get(*key)
	recycleOutpointKey(key)
	if serializedUtxo == nil {
		return nil, nil
	}

	// A non-nil zero-length entry means there is an entry in the database for a
	// spent transaction output which should never be the case.
	if len(serializedUtxo) == 0 {
		return nil, AssertError(fmt.Sprintf("database contains entry for spent tx "+
			"output %v", outpoint))
	}

	// Deserialize the utxo entry and return it.
	entry, err := deserializeUtxoEntry(serializedUtxo, outpoint.Index)
	if err != nil {
		// Ensure any deserialization errors are returned as database corruption
		// errors.
		if isDeserializeErr(err) {
			str := fmt.Sprintf("corrupt utxo entry for %v: %v", outpoint, err)
			return nil, makeDbErr(database.ErrCorruption, str)
		}

		return nil, err
	}

	return entry, nil
}

// dbFetchUxtoStats fetches statistics on the current unspent transaction output
// set.
func dbFetchUtxoStats(dbTx database.Tx) (*UtxoStats, error) {
	utxoBucket := dbTx.Metadata().Bucket(utxoSetBucketName)

	var stats UtxoStats
	transactions := make(map[chainhash.Hash]struct{})
	leaves := make([]chainhash.Hash, 0)
	cursor := utxoBucket.Cursor()

	for ok := cursor.First(); ok; ok = cursor.Next() {
		key := cursor.Key()
		var outpoint wire.OutPoint
		err := decodeOutpointKey(key, &outpoint)
		if err != nil {
			return nil, err
		}

		serializedUtxo := cursor.Value()
		entrySize := len(serializedUtxo)

		// A non-nil zero-length entry means there is an entry in the database for a
		// spent transaction output which should never be the case.
		if entrySize == 0 {
			return nil, AssertError(fmt.Sprintf("database contains entry for spent "+
				"tx output %v", outpoint))
		}

		stats.Utxos++
		stats.Size += int64(entrySize)
		transactions[outpoint.Hash] = struct{}{}

		leaves = append(leaves, chainhash.HashH(serializedUtxo))

		// Deserialize the utxo entry.
		entry, err := deserializeUtxoEntry(serializedUtxo, outpoint.Index)
		if err != nil {
			// Ensure any deserialization errors are returned as database corruption
			// errors.
			if isDeserializeErr(err) {
				str := fmt.Sprintf("corrupt utxo entry for %v: %v", outpoint, err)
				return nil, makeDbErr(database.ErrCorruption, str)
			}

			return nil, err
		}

		stats.Total += entry.amount
	}

	stats.SerializedHash = standalone.CalcMerkleRootInPlace(leaves)
	stats.Transactions = int64(len(transactions))

	return &stats, nil
}

// dbPutUtxoEntry uses an existing database transaction to update the utxo
// entry for the given outpoint based on the provided utxo entry state.  In
// particular, the entry is only written to the database if it is marked as
// modified, and if the entry is marked as spent it is removed from the
// database.
func dbPutUtxoEntry(dbTx database.Tx, outpoint wire.OutPoint, entry *UtxoEntry) error {
	// No need to update the database if the entry was not modified.
	if entry == nil || !entry.isModified() {
		return nil
	}

	// Remove the utxo entry if it is spent.
	utxoBucket := dbTx.Metadata().Bucket(utxoSetBucketName)
	if entry.IsSpent() {
		key := outpointKey(outpoint)
		err := utxoBucket.Delete(*key)
		recycleOutpointKey(key)
		if err != nil {
			return err
		}

		return nil
	}

	// Serialize and store the utxo entry.
	serialized, err := serializeUtxoEntry(entry)
	if err != nil {
		return err
	}
	key := outpointKey(outpoint)
	err = utxoBucket.Put(*key, serialized)
	// NOTE: The key is intentionally not recycled here since the database
	// interface contract prohibits modifications.  It will be garbage collected
	// normally when the database is done with it.
	if err != nil {
		return err
	}

	return nil
}

// -----------------------------------------------------------------------------
// The utxo set state contains information regarding the current state of the
// utxo set.  In particular, it tracks the block height and block hash of the
// last completed flush.
//
// The utxo set state is tracked in the database since at any given time, the
// utxo cache may not be consistent with the utxo set in the database.  This is
// due to the fact that the utxo cache only flushes changes to the database
// periodically.  Therefore, during initialization, the utxo set state is used
// to identify the last flushed state of the utxo set and it can be caught up
// to the current best state of the main chain.
//
// Note: The utxo set state MUST always be updated in the same database
// transaction that the utxo set is updated in to guarantee that they stay in
// sync in the database.
//
// The serialized format is:
//
//   <block height><block hash>
//
//   Field          Type             Size
//   block height   VLQ              variable
//   block hash     chainhash.Hash   chainhash.HashSize
//
// -----------------------------------------------------------------------------

// utxoSetState represents the current state of the utxo set.  In particular,
// it tracks the block height and block hash of the last completed flush.
type utxoSetState struct {
	lastFlushHeight uint32
	lastFlushHash   chainhash.Hash
}

// serializeUtxoSetState serializes the provided utxo set state.  The format is
// described in detail above.
func serializeUtxoSetState(state *utxoSetState) []byte {
	// Calculate the size needed to serialize the utxo set state.
	size := serializeSizeVLQ(uint64(state.lastFlushHeight)) + chainhash.HashSize

	// Serialize the utxo set state and return it.
	serialized := make([]byte, size)
	offset := putVLQ(serialized, uint64(state.lastFlushHeight))
	copy(serialized[offset:], state.lastFlushHash[:])
	return serialized
}

// deserializeUtxoSetState deserializes the passed serialized byte slice into
// the utxo set state.  The format is described in detail above.
func deserializeUtxoSetState(serialized []byte) (*utxoSetState, error) {
	// Deserialize the block height.
	blockHeight, bytesRead := deserializeVLQ(serialized)
	offset := bytesRead
	if offset >= len(serialized) {
		return nil, errDeserialize("unexpected end of data after height")
	}

	// Deserialize the hash.
	if len(serialized[offset:]) != chainhash.HashSize {
		return nil, errDeserialize("unexpected length for serialized hash")
	}
	var hash chainhash.Hash
	copy(hash[:], serialized[offset:offset+chainhash.HashSize])

	// Create the utxo set state and return it.
	return &utxoSetState{
		lastFlushHeight: uint32(blockHeight),
		lastFlushHash:   hash,
	}, nil
}

// dbPutUtxoSetState uses an existing database transaction to update the utxo
// set state in the database.
func dbPutUtxoSetState(dbTx database.Tx, state *utxoSetState) error {
	// Serialize and store the utxo set state.
	return dbTx.Metadata().Put(utxoSetStateKeyName, serializeUtxoSetState(state))
}

// dbFetchUtxoSetState uses an existing database transaction to fetch the utxo
// set state from the database.  If the utxo set state does not exist in the
// database, nil is returned.
func dbFetchUtxoSetState(dbTx database.Tx) (*utxoSetState, error) {
	// Fetch the serialized utxo set state from the database.
	serialized := dbTx.Metadata().Get(utxoSetStateKeyName)

	// Return nil if the utxo set state does not exist in the database.  This
	// should only be the case when starting from a fresh database or a database
	// that has not been run with the utxo cache yet.
	if serialized == nil {
		return nil, nil
	}

	// Deserialize the utxo set state and return it.
	return deserializeUtxoSetState(serialized)
}

// createUtxoDbInfo initializes the UTXO database info.  It must only be called
// on an uninitialized database.
func (b *BlockChain) createUtxoDbInfo() error {
	// Create the initial UTXO database state.
	err := b.utxoDb.Update(func(dbTx database.Tx) error {
		meta := dbTx.Metadata()

		// Create the bucket that houses information about the database's creation
		// and version.
		_, err := meta.CreateBucket(utxoDbInfoBucketName)
		if err != nil {
			return err
		}

		// Initialize the UTXO set version.  If the block database version is before
		// version 9, then initialize the UTXO set version based on the block
		// database version since that is what tracked the UTXO set version at that
		// point in time.
		utxoVer := uint32(currentUtxoSetVersion)
		if b.dbInfo.version >= 7 && b.dbInfo.version < 9 {
			utxoVer = 2
		} else if b.dbInfo.version < 7 {
			utxoVer = 1
		}

		// Write the creation and version information to the database.
		b.utxoDbInfo = &utxoDatabaseInfo{
			version: currentUtxoDatabaseVersion,
			compVer: currentCompressionVersion,
			utxoVer: utxoVer,
			created: time.Now(),
		}
		err = dbPutUtxoDatabaseInfo(dbTx, b.utxoDbInfo)
		if err != nil {
			return err
		}

		// Create the bucket that houses the UTXO set.
		_, err = meta.CreateBucket(utxoSetBucketName)
		if err != nil {
			return err
		}

		return err
	})
	return err
}

// initUtxoDbInfo loads (or creates if necessary) the UTXO database info.
func (b *BlockChain) initUtxoDbInfo(ctx context.Context) error {
	// Determine the state of the database.
	var isStateInitialized bool
	err := b.utxoDb.View(func(dbTx database.Tx) error {
		// Fetch the database versioning information.
		dbInfo := dbFetchUtxoDatabaseInfo(dbTx)

		// The database bucket for the versioning information is missing.
		if dbInfo == nil {
			return nil
		}

		// Don't allow downgrades of the UTXO database.
		if dbInfo.version > currentUtxoDatabaseVersion {
			return fmt.Errorf("the current UTXO database is no longer compatible "+
				"with this version of the software (%d > %d)", dbInfo.version,
				currentUtxoDatabaseVersion)
		}

		// Don't allow downgrades of the database compression version.
		if dbInfo.compVer > currentCompressionVersion {
			return fmt.Errorf("the current database compression version is no "+
				"longer compatible with this version of the software (%d > %d)",
				dbInfo.compVer, currentCompressionVersion)
		}

		b.utxoDbInfo = dbInfo
		isStateInitialized = true

		return nil
	})
	if err != nil {
		return err
	}

	// Initialize the database if it has not already been done.
	if !isStateInitialized {
		if err := b.createUtxoDbInfo(); err != nil {
			return err
		}
	}

	return nil
}

// initUtxoState attempts to load and initialize the UTXO state from the
// database.  This entails running any database migrations as necessary as well
// as initializing the UTXO cache.
func (b *BlockChain) initUtxoState(ctx context.Context) error {
	// Upgrade the UTXO database as needed.
	err := upgradeUtxoDb(ctx, b)
	if err != nil {
		return err
	}

	// Initialize the UTXO cache to ensure that the state of the UTXO set is
	// caught up to the tip of the best chain.
	return b.utxoCache.Initialize(b, b.bestChain.tip())
}
