// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/gcs/v3"
	"github.com/decred/dcrd/gcs/v3/blockcf2"
	"github.com/decred/dcrd/wire"
)

// errInterruptRequested indicates that an operation was cancelled due
// to a user-requested interrupt.
var errInterruptRequested = errors.New("interrupt requested")

// errBatchFinished indicates that a foreach database loop was exited due to
// reaching the maximum batch size.
var errBatchFinished = errors.New("batch finished")

// interruptRequested returns true when the provided channel has been closed.
// This simplifies early shutdown slightly since the caller can just use an if
// statement instead of a select.
func interruptRequested(ctx context.Context) bool {
	return ctx.Err() != nil
}

// deserializeDatabaseInfoV2 deserializes a database information struct from the
// passed serialized byte slice according to the legacy version 2 format.
//
// The legacy format is as follows:
//
//   Field      Type     Size      Description
//   version    uint32   4 bytes   The version of the database
//   compVer    uint32   4 bytes   The script compression version of the database
//   created    uint32   4 bytes   The date of the creation of the database
//
// The high bit (0x80000000) is used on version to indicate that an upgrade
// is in progress and used to confirm the database fidelity on start up.
func deserializeDatabaseInfoV2(dbInfoBytes []byte) (*databaseInfo, error) {
	// upgradeStartedBit if the bit flag for whether or not a database
	// upgrade is in progress. It is used to determine if the database
	// is in an inconsistent state from the update.
	const upgradeStartedBit = 0x80000000

	byteOrder := binary.LittleEndian

	rawVersion := byteOrder.Uint32(dbInfoBytes[0:4])
	upgradeStarted := (upgradeStartedBit & rawVersion) > 0
	version := rawVersion &^ upgradeStartedBit
	compVer := byteOrder.Uint32(dbInfoBytes[4:8])
	ts := byteOrder.Uint32(dbInfoBytes[8:12])

	if upgradeStarted {
		return nil, AssertError("database is in the upgrade started " +
			"state before resumable upgrades were supported - " +
			"delete the database and resync the blockchain")
	}

	return &databaseInfo{
		version: version,
		compVer: compVer,
		created: time.Unix(int64(ts), 0),
	}, nil
}

// -----------------------------------------------------------------------------
// The legacy version 2 block index consists of an entry for every known block.
// which includes information such as the block header and hashes of tickets
// voted and revoked.
//
// The serialized key format is:
//
//   <block height><block hash>
//
//   Field           Type              Size
//   block height    uint32            4 bytes
//   block hash      chainhash.Hash    chainhash.HashSize
//
// The serialized value format is:
//
//   <block header><status><num votes><votes info><num revoked><revoked tickets>
//
//   Field              Type                Size
//   block header       wire.BlockHeader    180 bytes
//   status             blockStatus         1 byte
//   num votes          VLQ                 variable
//   vote info
//     ticket hash      chainhash.Hash      chainhash.HashSize
//     vote version     VLQ                 variable
//     vote bits        VLQ                 variable
//   num revoked        VLQ                 variable
//   revoked tickets
//     ticket hash      chainhash.Hash      chainhash.HashSize
// -----------------------------------------------------------------------------
type blockIndexEntryV2 struct {
	header         wire.BlockHeader
	status         blockStatus
	voteInfo       []stake.VoteVersionTuple
	ticketsVoted   []chainhash.Hash
	ticketsRevoked []chainhash.Hash
}

// blockIndexEntrySerializeSizeV2 returns the number of bytes it would take to
// serialize the passed block index entry according to the legacy version 2
// format described above.
func blockIndexEntrySerializeSizeV2(entry *blockIndexEntryV2) int {
	voteInfoSize := 0
	for i := range entry.voteInfo {
		voteInfoSize += chainhash.HashSize +
			serializeSizeVLQ(uint64(entry.voteInfo[i].Version)) +
			serializeSizeVLQ(uint64(entry.voteInfo[i].Bits))
	}

	return blockHdrSize + 1 + serializeSizeVLQ(uint64(len(entry.voteInfo))) +
		voteInfoSize + serializeSizeVLQ(uint64(len(entry.ticketsRevoked))) +
		chainhash.HashSize*len(entry.ticketsRevoked)
}

// putBlockIndexEntryV2 serializes the passed block index entry according to the
// legacy version 2 format described above directly into the passed target byte
// slice.  The target byte slice must be at least large enough to handle the
// number of bytes returned by the blockIndexEntrySerializeSizeV2 function or it
// will panic.
func putBlockIndexEntryV2(target []byte, entry *blockIndexEntryV2) (int, error) {
	if len(entry.voteInfo) != len(entry.ticketsVoted) {
		return 0, AssertError("putBlockIndexEntry called with " +
			"mismatched number of tickets voted and vote info")
	}

	// Serialize the entire block header.
	w := bytes.NewBuffer(target[0:0])
	if err := entry.header.Serialize(w); err != nil {
		return 0, err
	}

	// Serialize the status.
	offset := blockHdrSize
	target[offset] = byte(entry.status)
	offset++

	// Serialize the number of votes and associated vote information.
	offset += putVLQ(target[offset:], uint64(len(entry.voteInfo)))
	for i := range entry.voteInfo {
		offset += copy(target[offset:], entry.ticketsVoted[i][:])
		offset += putVLQ(target[offset:], uint64(entry.voteInfo[i].Version))
		offset += putVLQ(target[offset:], uint64(entry.voteInfo[i].Bits))
	}

	// Serialize the number of revocations and associated revocation
	// information.
	offset += putVLQ(target[offset:], uint64(len(entry.ticketsRevoked)))
	for i := range entry.ticketsRevoked {
		offset += copy(target[offset:], entry.ticketsRevoked[i][:])
	}

	return offset, nil
}

// decodeBlockIndexEntryV2 decodes the passed serialized block index entry into
// the passed struct according to the legacy version 2 format described above.
// It returns the number of bytes read.
func decodeBlockIndexEntryV2(serialized []byte, entry *blockIndexEntryV2) (int, error) {
	// Hardcoded value so updates do not affect old upgrades.
	const blockHdrSize = 180

	// Ensure there are enough bytes to decode header.
	if len(serialized) < blockHdrSize {
		return 0, errDeserialize("unexpected end of data while reading block " +
			"header")
	}
	hB := serialized[0:blockHdrSize]

	// Deserialize the header.
	var header wire.BlockHeader
	if err := header.Deserialize(bytes.NewReader(hB)); err != nil {
		return 0, err
	}
	offset := blockHdrSize

	// Deserialize the status.
	if offset+1 > len(serialized) {
		return offset, errDeserialize("unexpected end of data while reading " +
			"status")
	}
	status := blockStatus(serialized[offset])
	offset++

	// Deserialize the number of tickets spent.
	var ticketsVoted []chainhash.Hash
	var votes []stake.VoteVersionTuple
	numVotes, bytesRead := deserializeVLQ(serialized[offset:])
	if bytesRead == 0 {
		return offset, errDeserialize("unexpected end of data while reading " +
			"num votes")
	}
	offset += bytesRead
	if numVotes > 0 {
		ticketsVoted = make([]chainhash.Hash, numVotes)
		votes = make([]stake.VoteVersionTuple, numVotes)
		for i := uint64(0); i < numVotes; i++ {
			// Deserialize the ticket hash associated with the vote.
			if offset+chainhash.HashSize > len(serialized) {
				return offset, errDeserialize(fmt.Sprintf("unexpected end of "+
					"data while reading vote #%d hash", i))
			}
			copy(ticketsVoted[i][:], serialized[offset:])
			offset += chainhash.HashSize

			// Deserialize the vote version.
			version, bytesRead := deserializeVLQ(serialized[offset:])
			if bytesRead == 0 {
				return offset, errDeserialize(fmt.Sprintf("unexpected end of "+
					"data while reading vote #%d version", i))
			}
			offset += bytesRead

			// Deserialize the vote bits.
			voteBits, bytesRead := deserializeVLQ(serialized[offset:])
			if bytesRead == 0 {
				return offset, errDeserialize(fmt.Sprintf("unexpected end of "+
					"data while reading vote #%d bits", i))
			}
			offset += bytesRead

			votes[i].Version = uint32(version)
			votes[i].Bits = uint16(voteBits)
		}
	}

	// Deserialize the number of tickets revoked.
	var ticketsRevoked []chainhash.Hash
	numTicketsRevoked, bytesRead := deserializeVLQ(serialized[offset:])
	if bytesRead == 0 {
		return offset, errDeserialize("unexpected end of data while reading " +
			"num tickets revoked")
	}
	offset += bytesRead
	if numTicketsRevoked > 0 {
		ticketsRevoked = make([]chainhash.Hash, numTicketsRevoked)
		for i := uint64(0); i < numTicketsRevoked; i++ {
			// Deserialize the ticket hash associated with the
			// revocation.
			if offset+chainhash.HashSize > len(serialized) {
				return offset, errDeserialize(fmt.Sprintf("unexpected end of "+
					"data while reading revocation #%d", i))
			}
			copy(ticketsRevoked[i][:], serialized[offset:])
			offset += chainhash.HashSize
		}
	}

	entry.header = header
	entry.status = status
	entry.voteInfo = votes
	entry.ticketsVoted = ticketsVoted
	entry.ticketsRevoked = ticketsRevoked
	return offset, nil
}

// incrementalFlatDrop uses multiple database updates to remove key/value pairs
// saved to a flag bucket.
func incrementalFlatDrop(ctx context.Context, db database.DB, bucketKey []byte, humanName string) error {
	const maxDeletions = 2000000
	var totalDeleted uint64
	for numDeleted := maxDeletions; numDeleted == maxDeletions; {
		numDeleted = 0
		err := db.Update(func(dbTx database.Tx) error {
			bucket := dbTx.Metadata().Bucket(bucketKey)
			cursor := bucket.Cursor()
			for ok := cursor.First(); ok; ok = cursor.Next() &&
				numDeleted < maxDeletions {

				if err := cursor.Delete(); err != nil {
					return err
				}
				numDeleted++
			}
			return nil
		})
		if err != nil {
			return err
		}

		if numDeleted > 0 {
			totalDeleted += uint64(numDeleted)
			log.Infof("Deleted %d keys (%d total) from %s", numDeleted,
				totalDeleted, humanName)
		}

		if interruptRequested(ctx) {
			return errInterruptRequested
		}
	}
	return nil
}

// runUpgradeStageOnce ensures the provided function is only run one time by
// checking if the provided key already exists in the database and writing it to
// the database upon successful completion of the provided function when it is
// not.
//
// This is useful to ensure upgrades that consist of multiple stages can be
// interrupted without redoing all of the work associated with stages that were
// previously completed successfully.
func runUpgradeStageOnce(ctx context.Context, db database.DB, doneKeyName []byte, fn func() error) error {
	// Don't run again if the provided key already exists.
	var alreadyDone bool
	err := db.View(func(dbTx database.Tx) error {
		alreadyDone = dbTx.Metadata().Get(doneKeyName) != nil
		return nil
	})
	if err != nil || alreadyDone {
		return err
	}

	if err := fn(); err != nil {
		return err
	}

	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	// Save the key to mark the update fully complete in case of interruption.
	return db.Update(func(dbTx database.Tx) error {
		return dbTx.Metadata().Put(doneKeyName, nil)
	})
}

// batchFn represents the batch function used by the batched update function.
type batchFn func(dbTx database.Tx) (bool, error)

// batchedUpdate calls the provided batch function repeatedly until it either
// returns an error other than the special ones described in this comment or
// its return indicates no more calls are necessary.
//
// In order to ensure the database is updated with the results of the batch that
// have already been successfully completed, it is allowed to return
// errBatchFinished and errInterruptRequested.  In the case of the former, the
// error will be ignored.  In the case of the latter, the database will be
// updated and the error will be returned accordingly.  The database will NOT
// be updated if any other errors are returned.
func batchedUpdate(ctx context.Context, db database.DB, doBatch batchFn) error {
	var isFullyDone bool
	for !isFullyDone {
		err := db.Update(func(dbTx database.Tx) error {
			var err error
			isFullyDone, err = doBatch(dbTx)
			if errors.Is(err, errInterruptRequested) ||
				errors.Is(err, errBatchFinished) {

				// No error here so the database transaction is not cancelled
				// and therefore outstanding work is written to disk.  The outer
				// function will exit with an interrupted error below due to
				// another interrupted check.
				err = nil
			}
			return err
		})
		if err != nil {
			return err
		}

		if interruptRequested(ctx) {
			return errInterruptRequested
		}
	}

	return nil
}

// clearFailedBlockFlagsV2 unmarks all blocks in a version 2 block index
// previously marked failed so they are eligible for validation again under new
// consensus rules.  This ensures clients that did not update prior to new rules
// activating are able to automatically recover under the new rules without
// having to download the entire chain again.
func clearFailedBlockFlagsV2(ctx context.Context, db database.DB) error {
	// Hardcoded bucket name so updates do not affect old upgrades.
	v2BucketName := []byte("blockidx")

	log.Info("Reindexing block information in the database.  This may take a " +
		"while...")
	start := time.Now()

	// doBatch contains the primary logic for updating the block index in
	// batches.  This is done because attempting to migrate in a single database
	// transaction could result in massive memory usage and could potentially
	// crash on many systems due to ulimits.
	//
	// It returns whether or not all entries have been updated.
	const maxEntries = 20000
	var resumeOffset uint32
	var totalUpdated uint64
	doBatch := func(dbTx database.Tx) (bool, error) {
		meta := dbTx.Metadata()
		v2BlockIdxBucket := meta.Bucket(v2BucketName)
		if v2BlockIdxBucket == nil {
			return false, fmt.Errorf("bucket %s does not exist", v2BucketName)
		}

		// Update block index entries so long as the max number of entries for
		// this batch has not been exceeded.
		var logProgress bool
		var numUpdated, numIterated uint32
		err := v2BlockIdxBucket.ForEach(func(key, oldSerialized []byte) error {
			if interruptRequested(ctx) {
				logProgress = true
				return errInterruptRequested
			}

			if numUpdated >= maxEntries {
				logProgress = true
				return errBatchFinished
			}

			// Skip entries that have already been migrated in previous batches.
			numIterated++
			if numIterated-1 < resumeOffset {
				return nil
			}
			resumeOffset++

			// Decode the old block index entry.
			var entry blockIndexEntryV2
			_, err := decodeBlockIndexEntryV2(oldSerialized, &entry)
			if err != nil {
				return err
			}

			// Mark the block index entry as eligible for validation again.
			const (
				v2StatusValidateFailed  = 1 << 2
				v2StatusInvalidAncestor = 1 << 3
			)
			origStatus := entry.status
			entry.status &^= v2StatusValidateFailed | v2StatusInvalidAncestor
			if entry.status != origStatus {
				targetSize := blockIndexEntrySerializeSizeV2(&entry)
				serialized := make([]byte, targetSize)
				_, err = putBlockIndexEntryV2(serialized, &entry)
				if err != nil {
					return err
				}
				err = v2BlockIdxBucket.Put(key, serialized)
				if err != nil {
					return err
				}
			}

			numUpdated++
			return nil
		})
		isFullyDone := err == nil
		if (isFullyDone || logProgress) && numUpdated > 0 {
			totalUpdated += uint64(numUpdated)
			log.Infof("Updated %d entries (%d total)", numUpdated, totalUpdated)
		}
		return isFullyDone, err
	}

	// Update all entries in batches for the reasons mentioned above.
	if err := batchedUpdate(ctx, db, doBatch); err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done updating block index.  Total entries: %d in %v",
		totalUpdated, elapsed)
	return nil
}

// scriptSourceEntry houses a script and its associated version.
type scriptSourceEntry struct {
	version uint16
	script  []byte
}

// scriptSource provides a source of transaction output scripts and their
// associated script version for given outpoints and implements the PrevScripter
// interface so it may be used in cases that require access to said scripts.
type scriptSource map[wire.OutPoint]scriptSourceEntry

// PrevScript returns the script and script version associated with the provided
// previous outpoint along with a bool that indicates whether or not the
// requested entry exists.  This ensures the caller is able to distinguish
// between missing entry and empty v0 scripts.
func (s scriptSource) PrevScript(prevOut *wire.OutPoint) (uint16, []byte, bool) {
	entry, ok := s[*prevOut]
	if !ok {
		return 0, nil, false
	}
	return entry.version, entry.script, true
}

// determineMinimalOutputsSizeV1 determines and returns the size of the stored
// set of minimal outputs in a version 1 spend journal entry.
func determineMinimalOutputsSizeV1(serialized []byte) (int, error) {
	numOutputs, offset := deserializeVLQ(serialized)
	if offset == 0 {
		return offset, errDeserialize("unexpected end of data during " +
			"decoding (num outputs)")
	}
	for i := 0; i < int(numOutputs); i++ {
		// Amount.
		_, bytesRead := deserializeVLQ(serialized[offset:])
		if bytesRead == 0 {
			return offset, errDeserialize("unexpected end of data during " +
				"decoding (output amount)")
		}
		offset += bytesRead

		// Script version.
		_, bytesRead = deserializeVLQ(serialized[offset:])
		if bytesRead == 0 {
			return offset, errDeserialize("unexpected end of data during " +
				"decoding (output script version)")
		}
		offset += bytesRead

		// Script.
		var scriptSize uint64
		scriptSize, bytesRead = deserializeVLQ(serialized[offset:])
		if bytesRead == 0 {
			return offset, errDeserialize("unexpected end of data during " +
				"decoding (output script size)")
		}
		offset += bytesRead

		if uint64(len(serialized[offset:])) < scriptSize {
			return offset, errDeserialize("unexpected end of data during " +
				"decoding (output script)")
		}

		offset += int(scriptSize)
	}

	return offset, nil
}

// scriptSourceFromSpendJournalV1 uses the legacy v1 spend journal along with
// the provided block to create a source of previous transaction scripts and
// versions spent by the block.
func scriptSourceFromSpendJournalV1(dbTx database.Tx, block *wire.MsgBlock) (scriptSource, error) {
	// Load the serialized spend journal entry from the database, construct the
	// full list of transactions that spend outputs (notice the coinbase
	// transaction is excluded since it can't spend anything), and perform an
	// initial sanity check to ensure there is serialized data for the block
	// when there are transactions that spend outputs.
	blockHash := block.BlockHash()
	v1SpendJournalBucketName := []byte("spendjournal")
	spendBucket := dbTx.Metadata().Bucket(v1SpendJournalBucketName)
	serialized := spendBucket.Get(blockHash[:])

	txns := make([]*wire.MsgTx, 0, len(block.STransactions)+
		len(block.Transactions[1:]))
	txns = append(txns, block.STransactions...)
	txns = append(txns, block.Transactions[1:]...)
	if len(txns) > 0 && len(serialized) == 0 {
		str := fmt.Sprintf("missing spend journal data for %s", blockHash)
		return nil, errDeserialize(str)
	}

	// The legacy version 1 transaction spend journal consists of an entry for
	// each block connected to the main chain which contains the transaction
	// outputs the block spends serialized such that the order is the reverse of
	// the order they were spent.
	//
	// The legacy format for this entry is roughly:
	//
	//   [<flags><script version><compressed pkscript><optional data>],...
	//
	// The legacy optional data is only present if the flags indicate the
	// transaction is fully spent (bit 4 in legacy format) and its format is
	// roughly:
	//
	//   <tx version><optional stake data>
	//
	// The legacy optional stake data is only present if the flags indicate the
	// transaction type is a ticket and its format is roughly:
	//
	//   <num outputs>[<amount><script version><script len><script>],...
	//
	//   Field                   Type           Size
	//   flags                   VLQ            variable (always 1 byte)
	//   script version          VLQ            variable
	//   compressed pkscript     []byte         variable
	//   optional data (only present if flags indicates fully spent)
	//     transaction version   VLQ            variable
	//     stake data (only present if flags indicates tx type ticket)
	//       num outputs         VLQ            variable
	//       output info
	//         amount            VLQ            variable
	//         script version    VLQ            variable
	//         script len        VLQ            variable
	//         script            []byte         variable
	//
	// The legacy serialized flags code format is:
	//
	//   bit  0   - containing transaction is a coinbase
	//   bit  1   - containing transaction has an expiry
	//   bits 2-3 - transaction type
	//   bit  4   - is fully spent
	//   bits 5-7 - unused
	//
	// Given the only information needed is the script version and associated
	// pkscript, the following specifically finds the relevant information while
	// skipping everything else.
	const (
		v1FullySpentFlag = 1 << 4
		v1TxTypeMask     = 0x0c
		v1TxTypeShift    = 2
		v1TxTypeTicket   = 1
		v1CompressionVer = 1
	)

	// Loop backwards through all transactions so everything is read in reverse
	// order to match the serialization order.
	source := make(scriptSource)
	var offset int
	for txIdx := len(txns) - 1; txIdx > -1; txIdx-- {
		tx := txns[txIdx]
		isVote := stake.IsSSGen(tx, false)

		// Loop backwards through all of the transaction inputs and read the
		// associated stxo.
		for txInIdx := len(tx.TxIn) - 1; txInIdx > -1; txInIdx-- {
			// Skip stakebase since it has no input.
			if txInIdx == 0 && isVote {
				continue
			}

			txIn := tx.TxIn[txInIdx]

			// Deserialize the flags.
			if offset >= len(serialized) {
				str := "unexpected end of spend journal entry"
				return nil, errDeserialize(str)
			}
			flags64, bytesRead := deserializeVLQ(serialized[offset:])
			offset += bytesRead
			if bytesRead != 1 {
				str := fmt.Sprintf("unexpected flags size -- got %d, want 1",
					bytesRead)
				return nil, errDeserialize(str)
			}
			flags := byte(flags64)
			fullySpent := flags&v1FullySpentFlag != 0
			txType := (flags & v1TxTypeMask) >> v1TxTypeShift

			// Deserialize the script version.
			if offset >= len(serialized) {
				str := "unexpected end of data after flags"
				return nil, errDeserialize(str)
			}
			scriptVersion, bytesRead := deserializeVLQ(serialized[offset:])
			offset += bytesRead

			// Decode the compressed script size and ensure there are enough
			// bytes left in the slice for it.
			if offset >= len(serialized) {
				str := "unexpected end of data after script version"
				return nil, errDeserialize(str)
			}
			scriptSize := decodeCompressedScriptSize(serialized[offset:],
				v1CompressionVer)
			if scriptSize < 0 {
				str := "negative script size"
				return nil, errDeserialize(str)
			}
			if offset+scriptSize > len(serialized) {
				str := "unexpected end of data after script size"
				return nil, errDeserialize(str)
			}
			pkScript := serialized[offset : offset+scriptSize]
			offset += scriptSize

			// Create an output in the script source for the referenced script
			// and version using the data from the spend journal.
			prevOut := &txIn.PreviousOutPoint
			source[*prevOut] = scriptSourceEntry{
				version: uint16(scriptVersion),
				script:  decompressScript(pkScript, v1CompressionVer),
			}

			// Deserialize the tx version and minimal outputs for tickets as
			// needed to locate the offset of the next entry.
			if fullySpent {
				if offset >= len(serialized) {
					str := "unexpected end of data after script size"
					return nil, errDeserialize(str)
				}
				_, bytesRead := deserializeVLQ(serialized[offset:])
				offset += bytesRead
				if txType == v1TxTypeTicket {
					if offset >= len(serialized) {
						str := "unexpected end of data after tx version"
						return nil, errDeserialize(str)
					}

					sz, err := determineMinimalOutputsSizeV1(serialized[offset:])
					if err != nil {
						return nil, err
					}
					offset += sz
				}
			}
		}
	}

	return source, nil
}

// initializeGCSFilters creates and stores version 2 GCS filters for all blocks
// in the main chain.  This ensures they are immediately available to clients
// and simplifies the rest of the related code since it can rely on the filters
// being available once the upgrade completes.
//
// The database is guaranteed to have a filter entry for every block in the
// main chain if this returns without failure.
func initializeGCSFilters(ctx context.Context, db database.DB, genesisHash *chainhash.Hash) error {
	log.Info("Creating and storing GCS filters.  This will take a while...")
	start := time.Now()

	// Determine the blocks in the main chain using the version 2 block index
	// and version 1 chain state.
	var mainChainBlocks []chainhash.Hash
	err := db.View(func(dbTx database.Tx) error {
		// Hardcoded bucket names and keys so updates do not affect old
		// upgrades.
		v2BucketName := []byte("blockidx")
		v1ChainStateKeyName := []byte("chainstate")

		// Load the current best chain tip hash and height from the v1 chain
		// state.
		//
		// The serialized format of the v1 chain state is roughly:
		//
		//   <block hash><rest of data>
		//
		//   Field             Type             Size
		//   block hash        chainhash.Hash   chainhash.HashSize
		//   rest of data...
		meta := dbTx.Metadata()
		serializedChainState := meta.Get(v1ChainStateKeyName)
		if serializedChainState == nil {
			str := fmt.Sprintf("chain state with key %s does not exist",
				v1ChainStateKeyName)
			return errDeserialize(str)
		}
		if len(serializedChainState) < chainhash.HashSize {
			str := "version 1 chain state is malformed"
			return errDeserialize(str)
		}
		var tipHash chainhash.Hash
		copy(tipHash[:], serializedChainState[0:chainhash.HashSize])

		// blockTreeEntry represents a version 2 block index entry with just
		// enough information to be able to determine which blocks comprise the
		// main chain.
		type blockTreeEntry struct {
			parent *blockTreeEntry
			hash   chainhash.Hash
			height uint32
		}

		// Construct a full block tree from the version 2 block index by mapping
		// each block to its parent block.
		var lastEntry, parent *blockTreeEntry
		blockTree := make(map[chainhash.Hash]*blockTreeEntry)
		v2BlockIdxBucket := meta.Bucket(v2BucketName)
		if v2BlockIdxBucket == nil {
			return fmt.Errorf("bucket %s does not exist", v2BucketName)
		}
		err := v2BlockIdxBucket.ForEach(func(_, serialized []byte) error {
			// Decode the block index entry.
			var entry blockIndexEntryV2
			_, err := decodeBlockIndexEntryV2(serialized, &entry)
			if err != nil {
				return err
			}
			header := &entry.header

			// Determine the parent block node.  Since the entries are iterated
			// in order of height, there is a very good chance the previous
			// one processed is the parent.
			blockHash := header.BlockHash()
			if lastEntry == nil {
				if blockHash != *genesisHash {
					str := fmt.Sprintf("initializeGCSFilters: expected first "+
						"entry in block index to be genesis block, found %s",
						blockHash)
					return errDeserialize(str)
				}
			} else if header.PrevBlock == lastEntry.hash {
				parent = lastEntry
			} else {
				parent = blockTree[header.PrevBlock]
				if parent == nil {
					str := fmt.Sprintf("initializeGCSFilters: could not find "+
						"parent for block %s", blockHash)
					return errDeserialize(str)
				}
			}

			// Add the block to the block tree.
			treeEntry := &blockTreeEntry{
				parent: parent,
				hash:   blockHash,
				height: header.Height,
			}
			blockTree[blockHash] = treeEntry

			lastEntry = treeEntry
			return nil
		})
		if err != nil {
			return err
		}

		// Construct a view of the blocks that comprise the main chain by
		// starting at the best tip and walking backwards to the genesis block
		// while assigning each one to its respective height.
		tipEntry := blockTree[tipHash]
		if tipEntry == nil {
			str := fmt.Sprintf("chain tip %s is not in block index", tipHash)
			return errDeserialize(str)
		}
		mainChainBlocks = make([]chainhash.Hash, tipEntry.height+1)
		for entry := tipEntry; entry != nil; entry = entry.parent {
			mainChainBlocks[entry.height] = entry.hash
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Create the new filter bucket as needed.
	gcsBucketName := []byte("gcsfilters")
	err = db.Update(func(dbTx database.Tx) error {
		_, err := dbTx.Metadata().CreateBucketIfNotExists(gcsBucketName)
		return err
	})
	if err != nil {
		return err
	}

	// newFilter loads the full block for the provided node from the db along
	// with its spend journal information and uses it to create a v2 GCS filter.
	newFilter := func(dbTx database.Tx, blockHash *chainhash.Hash) (*gcs.FilterV2, error) {
		// Load the full block from the database.
		blockBytes, err := dbTx.FetchBlock(blockHash)
		if err != nil {
			return nil, err
		}
		var block wire.MsgBlock
		if err := block.FromBytes(blockBytes); err != nil {
			return nil, err
		}

		// Use the combination of the block and the spent transaction output
		// data from the database to create a source of previous scripts spent
		// by the block needed to create the filter.
		prevScripts, err := scriptSourceFromSpendJournalV1(dbTx, &block)
		if err != nil {
			return nil, err
		}

		// Create the filter from the block and referenced previous output
		// scripts.
		filter, err := blockcf2.Regular(&block, prevScripts)
		if err != nil {
			return nil, err
		}

		return filter, nil
	}

	// doBatch contains the primary logic for creating the GCS filters when
	// moving from database version 5 to 6 in batches.  This is done because
	// attempting to create them all in a single database transaction could
	// result in massive memory usage and could potentially crash on many
	// systems due to ulimits.
	//
	// It returns whether or not all entries have been updated.
	const maxEntries = 20000
	blockHeight := int64(0)
	var totalCreated, totalFilterBytes uint64
	doBatch := func(dbTx database.Tx) (bool, error) {
		filterBucket := dbTx.Metadata().Bucket(gcsBucketName)
		if filterBucket == nil {
			return false, fmt.Errorf("bucket %s does not exist", gcsBucketName)
		}

		var logProgress bool
		var numCreated, totalBytes uint64
		err := func() error {
			for ; blockHeight < int64(len(mainChainBlocks)); blockHeight++ {
				if interruptRequested(ctx) {
					logProgress = true
					return errInterruptRequested
				}

				if numCreated >= maxEntries {
					logProgress = true
					return errBatchFinished
				}

				// Create the filter from the block and referenced previous output
				// scripts.
				blockHash := &mainChainBlocks[blockHeight]
				filter, err := newFilter(dbTx, blockHash)
				if err != nil {
					return err
				}

				// Store the filter to the database.
				serialized := filter.Bytes()
				err = filterBucket.Put(blockHash[:], serialized)
				if err != nil {
					return err
				}
				totalBytes += uint64(len(serialized))

				numCreated++
			}
			return nil
		}()
		isFullyDone := err == nil
		if (isFullyDone || logProgress) && numCreated > 0 {
			totalCreated += numCreated
			totalFilterBytes += totalBytes
			log.Infof("Created %d entries (%d total)", numCreated, totalCreated)
		}
		return isFullyDone, err
	}

	// Create the filters in batches for the reasons mentioned above.
	if err := batchedUpdate(ctx, db, doBatch); err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done creating GCS filters in %v.  Total entries: %d (%d bytes)",
		elapsed, totalCreated, totalFilterBytes)
	return nil
}

// upgradeToVersion6 upgrades a version 5 blockchain database to version 6.
func upgradeToVersion6(ctx context.Context, db database.DB, chainParams *chaincfg.Params, dbInfo *databaseInfo) error {
	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	log.Info("Upgrading database to version 6...")
	start := time.Now()

	// Unmark all blocks previously marked failed so they are eligible for
	// validation again under the new consensus rules.
	v2ClearFailedDoneKeyName := []byte("blockidxv2clearfaileddone")
	err := runUpgradeStageOnce(ctx, db, v2ClearFailedDoneKeyName, func() error {
		return clearFailedBlockFlagsV2(ctx, db)
	})
	if err != nil {
		return err
	}

	// Create and store version 2 GCS filters for all blocks in the main chain.
	err = initializeGCSFilters(ctx, db, &chainParams.GenesisHash)
	if err != nil {
		return err
	}

	// Update and persist the database versions and remove upgrade progress
	// tracking keys.
	err = db.Update(func(dbTx database.Tx) error {
		err := dbTx.Metadata().Delete(v2ClearFailedDoneKeyName)
		if err != nil {
			return err
		}

		dbInfo.version = 6
		return dbPutDatabaseInfo(dbTx, dbInfo)
	})
	if err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done upgrading database in %v.", elapsed)
	return nil
}

// migrateBlockIndexVersion2To3 migrates all block entries from the v2 block
// index bucket to a v3 bucket and removes the old v2 bucket.  As compared to
// the v2 block index, the v3 index removes the ticket hashes associated with
// vote info and revocations.
//
// The new block index is guaranteed to be fully updated if this returns without
// failure.
func migrateBlockIndexVersion2To3(ctx context.Context, db database.DB, dbInfo *databaseInfo) error {
	// Hardcoded bucket names so updates do not affect old upgrades.
	v2BucketName := []byte("blockidx")
	v3BucketName := []byte("blockidxv3")

	log.Info("Reindexing block information in the database.  This may take a " +
		"while...")
	start := time.Now()

	// Create the new block index bucket as needed.
	err := db.Update(func(dbTx database.Tx) error {
		_, err := dbTx.Metadata().CreateBucketIfNotExists(v3BucketName)
		return err
	})
	if err != nil {
		return err
	}

	// doBatch contains the primary logic for upgrading the block index from
	// version 2 to 3 in batches.  This is done because attempting to migrate in
	// a single database transaction could result in massive memory usage and
	// could potentially crash on many systems due to ulimits.
	//
	// It returns whether or not all entries have been updated.
	const maxEntries = 20000
	var resumeOffset uint32
	var totalMigrated uint64
	doBatch := func(dbTx database.Tx) (bool, error) {
		meta := dbTx.Metadata()
		v2BlockIdxBucket := meta.Bucket(v2BucketName)
		if v2BlockIdxBucket == nil {
			return false, fmt.Errorf("bucket %s does not exist", v2BucketName)
		}

		v3BlockIdxBucket := meta.Bucket(v3BucketName)
		if v3BlockIdxBucket == nil {
			return false, fmt.Errorf("bucket %s does not exist", v3BucketName)
		}

		// Migrate block index entries so long as the max number of entries for
		// this batch has not been exceeded.
		var logProgress bool
		var numMigrated, numIterated uint32
		err := v2BlockIdxBucket.ForEach(func(key, oldSerialized []byte) error {
			if interruptRequested(ctx) {
				logProgress = true
				return errInterruptRequested
			}

			if numMigrated >= maxEntries {
				logProgress = true
				return errBatchFinished
			}

			// Skip entries that have already been migrated in previous batches.
			numIterated++
			if numIterated-1 < resumeOffset {
				return nil
			}
			resumeOffset++

			// Skip entries that have already been migrated in previous
			// interrupted upgrades.
			if v3BlockIdxBucket.Get(key) != nil {
				return nil
			}

			// Decode the old block index entry.
			var entry blockIndexEntryV2
			_, err := decodeBlockIndexEntryV2(oldSerialized, &entry)
			if err != nil {
				return err
			}

			// Write the block index entry seriliazed with the new format to the
			// new bucket.
			serialized, err := serializeBlockIndexEntry(&blockIndexEntry{
				header:   entry.header,
				status:   entry.status,
				voteInfo: entry.voteInfo,
			})
			if err != nil {
				return err
			}
			err = v3BlockIdxBucket.Put(key, serialized)
			if err != nil {
				return err
			}

			numMigrated++
			return nil
		})
		isFullyDone := err == nil
		if (isFullyDone || logProgress) && numMigrated > 0 {
			totalMigrated += uint64(numMigrated)
			log.Infof("Migrated %d entries (%d total)", numMigrated,
				totalMigrated)
		}
		return isFullyDone, err
	}

	// Migrate all entries in batches for the reasons mentioned above.
	if err := batchedUpdate(ctx, db, doBatch); err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done migrating block index.  Total entries: %d in %v",
		totalMigrated, elapsed)

	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	// Drop version 2 block index.
	log.Info("Removing old block index entries...")
	start = time.Now()
	err = incrementalFlatDrop(ctx, db, v2BucketName, "old block index")
	if err != nil {
		return err
	}
	elapsed = time.Since(start).Round(time.Millisecond)
	log.Infof("Done removing old block index entries in %v", elapsed)

	// Update and persist the database versions.
	err = db.Update(func(dbTx database.Tx) error {
		dbInfo.bidxVer = 3
		return dbPutDatabaseInfo(dbTx, dbInfo)
	})
	return err
}

// migrateUtxoSetVersion1To2 migrates all utxoset entries from the v1 bucket to
// a v2 bucket and removes the old v1 bucket.  As compared to the v1 utxoset,
// the v2 utxoset moves the bit which defines whether or not a tx is fully spent
// from bit 4 to bit 6.
//
// The utxoset is guaranteed to be fully updated if this returns without
// failure.
func migrateUtxoSetVersion1To2(ctx context.Context, db database.DB) error {
	// Hardcoded bucket and key names so updates do not affect old upgrades.
	v1BucketName := []byte("utxoset")
	v2BucketName := []byte("utxosetv2")

	log.Info("Migrating database utxoset.  This may take a while...")
	start := time.Now()

	// Create the new utxoset bucket as needed.
	err := db.Update(func(dbTx database.Tx) error {
		_, err := dbTx.Metadata().CreateBucketIfNotExists(v2BucketName)
		return err
	})
	if err != nil {
		return err
	}

	// doBatch contains the primary logic for upgrading the utxoset from version
	// 1 to 2 in batches.  This is done because attempting to migrate in a
	// single database transaction could result in massive memory usage and
	// could potentially crash on many systems due to ulimits.
	//
	// It returns whether or not all entries have been updated.
	const maxEntries = 20000
	var resumeOffset uint32
	var totalMigrated uint64
	doBatch := func(dbTx database.Tx) (bool, error) {
		meta := dbTx.Metadata()
		v1Bucket := meta.Bucket(v1BucketName)
		if v1Bucket == nil {
			return false, fmt.Errorf("bucket %s does not exist", v1BucketName)
		}

		v2Bucket := meta.Bucket(v2BucketName)
		if v2Bucket == nil {
			return false, fmt.Errorf("bucket %s does not exist", v2BucketName)
		}

		// Migrate utxoset entries so long as the max number of entries for this
		// batch has not been exceeded.
		var logProgress bool
		var numMigrated, numIterated uint32
		err := v1Bucket.ForEach(func(key, oldSerialized []byte) error {
			if interruptRequested(ctx) {
				logProgress = true
				return errInterruptRequested
			}

			if numMigrated >= maxEntries {
				logProgress = true
				return errBatchFinished
			}

			// Skip entries that have already been migrated in previous batches.
			numIterated++
			if numIterated-1 < resumeOffset {
				return nil
			}
			resumeOffset++

			// Skip entries that have already been migrated in previous
			// interrupted upgrades.
			if v2Bucket.Get(key) != nil {
				return nil
			}

			// Copy the existing serialized bytes so they can be mutated and
			// rewritten to the new bucket as needed.
			serialized := make([]byte, len(oldSerialized))
			copy(serialized, oldSerialized)

			// The legacy version 1 unspent transaction output (utxo) set
			// consists of an entry for each transaction which contains a utxo
			// serialized using a format that is highly optimized to reduce
			// space using domain specific compression algorithms.
			//
			// The legacy format for this entry is roughly:
			//
			//   <version><height><index><flags><rest of data>
			//
			//   Field                 Type     Size
			//   transaction version   VLQ      variable
			//   block height          VLQ      variable
			//   block index           VLQ      variable
			//   flags                 VLQ      variable (only ever 1 byte)
			//   rest of data...
			//
			// The legacy serialized flags code format is:
			//   bit  0   - containing transaction is a coinbase
			//   bit  1   - containing transaction has an expiry
			//   bits 2-3 - transaction type
			//   bit  4   - is fully spent
			//   bits 5-7 - unused
			//
			// Given the migration only needs to move the fully spent bit from
			// bit 4 to bit 6, the following specifically finds and modifies the
			// relevant byte while leaving everything else untouched.

			// Deserialize the tx version, block height, and block index to
			// locate the offset of the flags byte that needs to be modified.
			_, bytesRead := deserializeVLQ(serialized)
			offset := bytesRead
			if offset >= len(serialized) {
				return errDeserialize("unexpected end of data after version")
			}
			_, bytesRead = deserializeVLQ(serialized[offset:])
			offset += bytesRead
			if offset >= len(serialized) {
				return errDeserialize("unexpected end of data after height")
			}
			_, bytesRead = deserializeVLQ(serialized[offset:])
			offset += bytesRead
			if offset >= len(serialized) {
				return errDeserialize("unexpected end of data after index")
			}

			// Migrate flags to the new format.
			const v1FullySpentFlag = 1 << 4
			const v2FullySpentFlag = 1 << 6
			flags64, bytesRead := deserializeVLQ(serialized[offset:])
			if bytesRead != 1 {
				str := fmt.Sprintf("unexpected flags size -- got %d, want 1",
					bytesRead)
				return errDeserialize(str)
			}
			flags := byte(flags64)
			fullySpent := flags&v1FullySpentFlag != 0
			flags &^= v1FullySpentFlag
			if fullySpent {
				flags |= v2FullySpentFlag
			}
			serialized[offset] = flags

			// Write the entry serialized with the new format to the new bucket.
			err = v2Bucket.Put(key, serialized)
			if err != nil {
				return err
			}

			numMigrated++
			return nil
		})
		isFullyDone := err == nil
		if (isFullyDone || logProgress) && numMigrated > 0 {
			totalMigrated += uint64(numMigrated)
			log.Infof("Migrated %d entries (%d total)", numMigrated,
				totalMigrated)
		}
		return isFullyDone, err
	}

	// Migrate all entries in batches for the reasons mentioned above.
	if err := batchedUpdate(ctx, db, doBatch); err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done migrating utxoset.  Total entries: %d in %v", totalMigrated,
		elapsed)

	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	// Drop version 1 utxoset.
	log.Info("Removing old utxoset entries...")
	start = time.Now()
	err = incrementalFlatDrop(ctx, db, v1BucketName, "old utxoset")
	if err != nil {
		return err
	}
	elapsed = time.Since(start).Round(time.Millisecond)
	log.Infof("Done removing old utxoset entries in %v", elapsed)
	return nil
}

// migrateSpendJournalVersion1To2 migrates all spend journal entries from the v1
// bucket to a v2 bucket and removes the old v1 bucket.  As compared to the v1
// spend journal, the v2 spend journal moves the bit which defines whether or
// not a tx is fully spent from bit 4 to bit 6.
//
// The spend journal is guaranteed to be fully updated if this returns without
// failure.
func migrateSpendJournalVersion1To2(ctx context.Context, db database.DB) error {
	// Hardcoded bucket and key names so updates do not affect old upgrades.
	v1BucketName := []byte("spendjournal")
	v2BucketName := []byte("spendjournalv2")

	log.Info("Migrating database spend journal.  This may take a while...")
	start := time.Now()

	// Create the new spend journal bucket as needed.
	err := db.Update(func(dbTx database.Tx) error {
		_, err := dbTx.Metadata().CreateBucketIfNotExists(v2BucketName)
		return err
	})
	if err != nil {
		return err
	}

	// doBatch contains the primary logic for upgrading the spend journal from
	// version 1 to 2 in batches.  This is done because attempting to migrate in
	// a single database transaction could result in massive memory usage and
	// could potentially crash on many systems due to ulimits.
	//
	// It returns whether or not all entries have been updated.
	const maxEntries = 20000
	var resumeOffset uint32
	var totalMigrated uint64
	doBatch := func(dbTx database.Tx) (bool, error) {
		meta := dbTx.Metadata()
		v1Bucket := meta.Bucket(v1BucketName)
		if v1Bucket == nil {
			return false, fmt.Errorf("bucket %s does not exist", v1BucketName)
		}

		v2Bucket := meta.Bucket(v2BucketName)
		if v2Bucket == nil {
			return false, fmt.Errorf("bucket %s does not exist", v2BucketName)
		}

		// Migrate spend journal entries so long as the max number of entries
		// for this batch has not been exceeded.
		var logProgress bool
		var numMigrated, numIterated uint32
		err := v1Bucket.ForEach(func(key, oldSerialized []byte) error {
			if interruptRequested(ctx) {
				logProgress = true
				return errInterruptRequested
			}

			if numMigrated >= maxEntries {
				logProgress = true
				return errBatchFinished
			}

			// Skip entries that have already been migrated in previous batches.
			numIterated++
			if numIterated-1 < resumeOffset {
				return nil
			}
			resumeOffset++

			// Skip entries that have already been migrated in previous
			// interrupted upgrades.
			if v2Bucket.Get(key) != nil {
				return nil
			}

			// Copy the existing serialized bytes so they can be mutated and
			// rewritten to the new bucket as needed.
			serialized := make([]byte, len(oldSerialized))
			copy(serialized, oldSerialized)

			// The legacy version 1 transaction spend journal consists of an
			// entry for each block connected to the main chain which contains
			// the transaction outputs the block spends serialized such that the
			// order is the reverse of the order they were spent.
			//
			// The legacy format for this entry is roughly:
			//
			//   [<flags><script version><compressed pkscript><optional data>],...
			//
			// The legacy optional data is only present if the flags indicate
			// the transaction is fully spent (bit 4 in legacy format) and its
			// format is roughly:
			//
			//   <tx version><optional stake data>
			//
			// The legacy optional stake data is only present if the flags
			// indicate the transaction type is a ticket and its format is
			// roughly:
			//
			//   <num outputs>[<amount><script version><script len><script>],...
			//
			//   Field                   Type           Size
			//   flags                   VLQ            variable (always 1 byte)
			//   script version          VLQ            variable
			//   compressed pkscript     []byte         variable
			//   optional data (only present if flags indicates fully spent)
			//     transaction version   VLQ            variable
			//     stake data (only present if flags indicates tx type ticket)
			//       num outputs         VLQ            variable
			//       output info
			//         amount            VLQ            variable
			//         script version    VLQ            variable
			//         script len        VLQ            variable
			//         script            []byte         variable
			//
			// The legacy serialized flags code format is:
			//
			//   bit  0   - containing transaction is a coinbase
			//   bit  1   - containing transaction has an expiry
			//   bits 2-3 - transaction type
			//   bit  4   - is fully spent
			//   bits 5-7 - unused
			//
			// Given the migration only needs to move the fully spent bit from
			// bit 4 to bit 6, the following specifically finds and modifies the
			// relevant flags bytes while leaving everything else untouched.
			const (
				v1FullySpentFlag = 1 << 4
				v1TxTypeMask     = 0x0c
				v1TxTypeShift    = 2
				v1TxTypeTicket   = 1
				v1CompressionVer = 1
				v2FullySpentFlag = 1 << 6
			)
			var offset int
			for offset != len(serialized) {
				// Migrate the flags for the entry to the new format.
				if offset >= len(serialized) {
					str := "unexpected end of spend journal entry"
					return errDeserialize(str)
				}
				flags64, bytesRead := deserializeVLQ(serialized[offset:])
				if bytesRead != 1 {
					str := fmt.Sprintf("unexpected flags size -- got %d, want 1",
						bytesRead)
					return errDeserialize(str)
				}
				flags := byte(flags64)
				fullySpent := flags&v1FullySpentFlag != 0
				txType := (flags & v1TxTypeMask) >> v1TxTypeShift
				flags &^= v1FullySpentFlag
				if fullySpent {
					flags |= v2FullySpentFlag
				}
				serialized[offset] = flags
				offset += bytesRead

				// Deserialize the compressed txout, tx version, and minimal
				// outputs for tickets as needed to locate the offset of the
				// next flags byte that needs to be modified.
				if offset >= len(serialized) {
					str := "unexpected end of data after flags"
					return errDeserialize(str)
				}
				_, bytesRead = deserializeVLQ(serialized[offset:])
				offset += bytesRead
				if offset >= len(serialized) {
					str := "unexpected end of data after script version"
					return errDeserialize(str)
				}
				scriptSize := decodeCompressedScriptSize(serialized[offset:],
					v1CompressionVer)
				offset += scriptSize
				if fullySpent {
					if offset >= len(serialized) {
						str := "unexpected end of data after script size"
						return errDeserialize(str)
					}
					_, bytesRead := deserializeVLQ(serialized[offset:])
					offset += bytesRead
					if txType == v1TxTypeTicket {
						if offset >= len(serialized) {
							str := "unexpected end of data after tx version"
							return errDeserialize(str)
						}
						sz, err := determineMinimalOutputsSizeV1(
							serialized[offset:])
						if err != nil {
							return err
						}
						offset += sz
					}
				}
			}

			// Write the entry serialized with the new format to the new bucket.
			err = v2Bucket.Put(key, serialized)
			if err != nil {
				return err
			}

			numMigrated++
			return nil
		})
		isFullyDone := err == nil
		if (isFullyDone || logProgress) && numMigrated > 0 {
			totalMigrated += uint64(numMigrated)
			log.Infof("Migrated %d entries (%d total)", numMigrated,
				totalMigrated)
		}
		return isFullyDone, err
	}

	// Migrate all entries in batches for the reasons mentioned above.
	if err := batchedUpdate(ctx, db, doBatch); err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done migrating spend journal.  Total entries: %d in %v",
		totalMigrated, elapsed)

	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	// Drop version 1 spend journal.
	log.Info("Removing old spend journal entries...")
	start = time.Now()
	err = incrementalFlatDrop(ctx, db, v1BucketName, "old spend journal")
	if err != nil {
		return err
	}
	elapsed = time.Since(start).Round(time.Millisecond)
	log.Infof("Done removing old spend journal entries in %v", elapsed)

	return nil
}

// initializeTreasuryBuckets creates the buckets that house the treasury account
// and spend information as needed.
func initializeTreasuryBuckets(db database.DB) error {
	// Hardcoded key names so updates do not affect old upgrades.
	treasuryBucketName := []byte("treasury")
	treasuryTSpendBucketName := []byte("tspend")

	// Create the new treasury buckets as needed.
	return db.Update(func(dbTx database.Tx) error {
		meta := dbTx.Metadata()
		_, err := meta.CreateBucketIfNotExists(treasuryBucketName)
		if err != nil {
			return err
		}
		_, err = meta.CreateBucketIfNotExists(treasuryTSpendBucketName)
		return err
	})
}

// upgradeToVersion7 upgrades a version 6 blockchain database to version 7.
func upgradeToVersion7(ctx context.Context, db database.DB, dbInfo *databaseInfo) error {
	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	log.Info("Upgrading database to version 7...")
	start := time.Now()

	// Create the new treasury buckets as needed.
	if err := initializeTreasuryBuckets(db); err != nil {
		return err
	}

	// Migrate the utxoset to version 2.
	v2DoneKeyName := []byte("utxosetv2done")
	err := runUpgradeStageOnce(ctx, db, v2DoneKeyName, func() error {
		return migrateUtxoSetVersion1To2(ctx, db)
	})
	if err != nil {
		return err
	}

	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	// Migrate the spend journal to version 2.
	if err := migrateSpendJournalVersion1To2(ctx, db); err != nil {
		return err
	}

	// Update and persist the database versions and remove upgrade progress
	// tracking keys.
	err = db.Update(func(dbTx database.Tx) error {
		err := dbTx.Metadata().Delete(v2DoneKeyName)
		if err != nil {
			return err
		}

		dbInfo.version = 7
		return dbPutDatabaseInfo(dbTx, dbInfo)
	})
	if err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done upgrading database in %v.", elapsed)
	return nil
}

// clearFailedBlockFlagsV3 unmarks all blocks in a version 3 block index
// previously marked failed so they are eligible for validation again under new
// consensus rules.  This ensures clients that did not update prior to new rules
// activating are able to automatically recover under the new rules without
// having to download the entire chain again.
func clearFailedBlockFlagsV3(ctx context.Context, db database.DB) error {
	// Hardcoded bucket name so updates do not affect old upgrades.
	v3BucketName := []byte("blockidxv3")

	log.Info("Reindexing block information in the database.  This may take a " +
		"while...")
	start := time.Now()

	// doBatch contains the primary logic for updating the block index in
	// batches.  This is done because attempting to migrate in a single database
	// transaction could result in massive memory usage and could potentially
	// crash on many systems due to ulimits.
	//
	// It returns whether or not all entries have been updated.
	const maxEntries = 20000
	var resumeOffset uint32
	var totalUpdated uint64
	doBatch := func(dbTx database.Tx) (bool, error) {
		meta := dbTx.Metadata()
		v3BlockIdxBucket := meta.Bucket(v3BucketName)
		if v3BlockIdxBucket == nil {
			return false, fmt.Errorf("bucket %s does not exist", v3BucketName)
		}

		// Update block index entries so long as the max number of entries for
		// this batch has not been exceeded.
		var logProgress bool
		var numUpdated, numIterated uint32
		err := v3BlockIdxBucket.ForEach(func(key, oldSerialized []byte) error {
			if interruptRequested(ctx) {
				logProgress = true
				return errInterruptRequested
			}

			if numUpdated >= maxEntries {
				logProgress = true
				return errBatchFinished
			}

			// Skip entries that have already been migrated in previous batches.
			numIterated++
			if numIterated-1 < resumeOffset {
				return nil
			}
			resumeOffset++

			// Copy the existing serialized bytes so they can be mutated and
			// rewritten to the new bucket as needed.
			serialized := make([]byte, len(oldSerialized))
			copy(serialized, oldSerialized)

			// The version 3 block index consists of an entry for every known
			// block.
			//
			// The serialized value format is roughly:
			//
			//   <block header><status><rest of data>
			//
			//   Field              Type                Size
			//   block header       wire.BlockHeader    180 bytes
			//   status             blockStatus         1 byte
			//   rest of data...
			//
			// Given the status field is the only thing that needs to be
			// modified, the following specifically finds and modifies the
			// relevant byte while leaving everything else untouched.

			// Mark the block index entry as eligible for validation again.
			const (
				blockHdrSize            = 180
				v3StatusValidateFailed  = 1 << 2
				v3StatusInvalidAncestor = 1 << 3
			)
			offset := blockHdrSize
			if offset+1 > len(serialized) {
				return errDeserialize("unexpected end of data while reading " +
					"status")
			}
			origStatus := serialized[offset]
			newStatus := origStatus
			newStatus &^= v3StatusValidateFailed | v3StatusInvalidAncestor
			serialized[offset] = newStatus
			if newStatus != origStatus {
				err := v3BlockIdxBucket.Put(key, serialized)
				if err != nil {
					return err
				}
			}

			numUpdated++
			return nil
		})
		isFullyDone := err == nil
		if (isFullyDone || logProgress) && numUpdated > 0 {
			totalUpdated += uint64(numUpdated)
			log.Infof("Updated %d entries (%d total)", numUpdated, totalUpdated)
		}
		return isFullyDone, err
	}

	// Update all entries in batches for the reasons mentioned above.
	if err := batchedUpdate(ctx, db, doBatch); err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done updating block index.  Total entries: %d in %v",
		totalUpdated, elapsed)
	return nil
}

// upgradeToVersion8 upgrades a version 7 blockchain database to version 8.
func upgradeToVersion8(ctx context.Context, db database.DB, dbInfo *databaseInfo) error {
	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	log.Info("Upgrading database to version 8...")
	start := time.Now()

	// Ensure the treasury buckets are created for version 7 databases.  They
	// ordinarily will have already been created during the upgrade to version 7
	// above, however, due to a bug in a release candidate, they might not have
	// been, so this is a relatively simple hack to ensure anyone in that
	// intermediate state is upgraded properly without needing to redownload the
	// chain.
	if err := initializeTreasuryBuckets(db); err != nil {
		return err
	}

	// Unmark all blocks previously marked failed so they are eligible for
	// validation again under the new consensus rules.
	if err := clearFailedBlockFlagsV3(ctx, db); err != nil {
		return err
	}

	// Update and persist the database versions.
	err := db.Update(func(dbTx database.Tx) error {
		dbInfo.version = 8
		return dbPutDatabaseInfo(dbTx, dbInfo)
	})
	if err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done upgrading database in %v.", elapsed)
	return nil
}

// checkDBTooOldToUpgrade returns an ErrDBTooOldToUpgrade error if the provided
// database version can no longer be upgraded due to being too old.
func checkDBTooOldToUpgrade(dbVersion uint32) error {
	const lowestSupportedUpgradeVer = 5
	if dbVersion < lowestSupportedUpgradeVer {
		str := fmt.Sprintf("database versions prior to version %d are no "+
			"longer supported (current version: %d)", lowestSupportedUpgradeVer,
			dbVersion)
		return contextError(ErrDBTooOldToUpgrade, str)
	}

	return nil
}

// CheckDBTooOldToUpgrade returns an ErrDBTooOldToUpgrade error if the provided
// database can no longer be upgraded due to being too old.
func CheckDBTooOldToUpgrade(db database.DB) error {
	// Fetch the database versioning information.
	var dbInfo *databaseInfo
	err := db.View(func(dbTx database.Tx) error {
		var err error
		dbInfo, err = dbFetchDatabaseInfo(dbTx)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	// The database has not been initialized and thus will be created at the
	// latest version.
	if dbInfo == nil {
		return nil
	}

	return checkDBTooOldToUpgrade(dbInfo.version)
}

// upgradeDB upgrades old database versions to the newest version by applying
// all possible upgrades iteratively.
//
// NOTE: The passed database info will be updated with the latest versions.
func upgradeDB(ctx context.Context, db database.DB, chainParams *chaincfg.Params, dbInfo *databaseInfo) error {
	// Upgrading databases prior to version 5 is no longer supported due to a
	// major overhaul that took place at that version.
	if err := checkDBTooOldToUpgrade(dbInfo.version); err != nil {
		// Override the error with some additional instructions in this path
		// since it means the caller did not check up front before attempting
		// to create a chain instance.
		if errors.Is(err, ErrDBTooOldToUpgrade) {
			str := fmt.Sprintf("%s -- please delete the existing database "+
				"and restart the application to continue", err)
			err = contextError(ErrDBTooOldToUpgrade, str)
		}
		return err
	}

	// Update to a version 6 database if needed.  This entails unmarking all
	// blocks previously marked failed so they are eligible for validation again
	// under the new consensus rules and creating and storing version 2 GCS
	// filters for all blocks in the main chain.
	if dbInfo.version == 5 {
		err := upgradeToVersion6(ctx, db, chainParams, dbInfo)
		if err != nil {
			return err
		}
	}

	// Update to the version 3 block index format if needed.
	if dbInfo.version == 6 && dbInfo.bidxVer == 2 {
		err := migrateBlockIndexVersion2To3(ctx, db, dbInfo)
		if err != nil {
			return err
		}
	}

	// Update to a version 7 database if needed.  This entails migrating the
	// utxoset and spend journal to the v2 format.
	if dbInfo.version == 6 {
		err := upgradeToVersion7(ctx, db, dbInfo)
		if err != nil {
			return err
		}
	}

	// Update to a version 8 database if needed.  This entails ensuring the
	// treasury buckets from v7 are created and unmarking all blocks previously
	// marked failed so they are eligible for validation again under the new
	// consensus rules.
	if dbInfo.version == 7 {
		if err := upgradeToVersion8(ctx, db, dbInfo); err != nil {
			return err
		}
	}

	return nil
}
