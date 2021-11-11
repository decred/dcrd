// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v4"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/gcs/v3"
	"github.com/decred/dcrd/gcs/v3/blockcf2"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/wire"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
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
//   status             byte                1 byte
//   num votes          VLQ                 variable
//   vote info
//     ticket hash      chainhash.Hash      chainhash.HashSize
//     vote version     VLQ                 variable
//     vote bits        VLQ                 variable
//   num revoked        VLQ                 variable
//   revoked tickets
//     ticket hash      chainhash.Hash      chainhash.HashSize
//
// The version 2 block status flags format is:
//
//   bit  0   - block payload is stored on disk
//   bit  1   - block and all of its ancestors have been fully validated
//   bit  2   - block failed validation
//   bit  3   - an ancestor of the block failed validation
//   bits 4-7 - unused
// -----------------------------------------------------------------------------

// blockIndexVoteVersionTuple houses the extracted vote bits and version from
// votes for use in block index database entries.
type blockIndexVoteVersionTuple struct {
	version uint32
	bits    uint16
}

// blockIndexEntryV2 represents a legacy version 2 block index database entry.
type blockIndexEntryV2 struct {
	header         wire.BlockHeader
	status         byte
	voteInfo       []blockIndexVoteVersionTuple
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
			serializeSizeVLQ(uint64(entry.voteInfo[i].version)) +
			serializeSizeVLQ(uint64(entry.voteInfo[i].bits))
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
	target[offset] = entry.status
	offset++

	// Serialize the number of votes and associated vote information.
	offset += putVLQ(target[offset:], uint64(len(entry.voteInfo)))
	for i := range entry.voteInfo {
		offset += copy(target[offset:], entry.ticketsVoted[i][:])
		offset += putVLQ(target[offset:], uint64(entry.voteInfo[i].version))
		offset += putVLQ(target[offset:], uint64(entry.voteInfo[i].bits))
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
	status := serialized[offset]
	offset++

	// Deserialize the number of tickets spent.
	var ticketsVoted []chainhash.Hash
	var votes []blockIndexVoteVersionTuple
	numVotes, bytesRead := deserializeVLQ(serialized[offset:])
	if bytesRead == 0 {
		return offset, errDeserialize("unexpected end of data while reading " +
			"num votes")
	}
	offset += bytesRead
	if numVotes > 0 {
		ticketsVoted = make([]chainhash.Hash, numVotes)
		votes = make([]blockIndexVoteVersionTuple, numVotes)
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

			votes[i].version = uint32(version)
			votes[i].bits = uint16(voteBits)
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

// utxoBackendBatchFn represents the batch function used by the UTXO backend
// batched update function.
type utxoBackendBatchFn func(tx UtxoBackendTx) (bool, error)

// utxoBackendBatchedUpdate calls the provided batch function repeatedly until
// it either returns an error other than the special ones described in this
// comment or its return indicates no more calls are necessary.
//
// In order to ensure the backend is updated with the results of the batch that
// have already been successfully completed, it is allowed to return
// errBatchFinished and errInterruptRequested.  In the case of the former, the
// error will be ignored.  In the case of the latter, the backend will be
// updated and the error will be returned accordingly.  The backend will NOT
// be updated if any other errors are returned.
func utxoBackendBatchedUpdate(ctx context.Context,
	utxoBackend UtxoBackend, doBatch utxoBackendBatchFn) error {

	var isFullyDone bool
	for !isFullyDone {
		err := utxoBackend.Update(func(tx UtxoBackendTx) error {
			var err error
			isFullyDone, err = doBatch(tx)
			if errors.Is(err, errInterruptRequested) ||
				errors.Is(err, errBatchFinished) {

				// No error here so the database transaction is not cancelled
				// and therefore outstanding work is written to disk.  The outer
				// function will exit with an interrupted error below due to
				// another interrupted check.
				return nil
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
// between missing entries and empty v0 scripts.
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

// decodeCompressedScriptSizeV1 treats the passed serialized bytes as a v1
// compressed script, possibly followed by other data, and returns the number of
// bytes it occupies taking into account the special encoding of the script size
// by the domain specific compression algorithm described above.
func decodeCompressedScriptSizeV1(serialized []byte) int {
	const (
		// Hardcoded constants so updates do not affect old upgrades.
		cstPayToPubKeyHash       = 0
		cstPayToScriptHash       = 1
		cstPayToPubKeyCompEven   = 2
		cstPayToPubKeyCompOdd    = 3
		cstPayToPubKeyUncompEven = 4
		cstPayToPubKeyUncompOdd  = 5
		numSpecialScripts        = 64
	)

	scriptSize, bytesRead := deserializeVLQ(serialized)
	if bytesRead == 0 {
		return 0
	}

	switch scriptSize {
	case cstPayToPubKeyHash:
		return 21

	case cstPayToScriptHash:
		return 21

	case cstPayToPubKeyCompEven, cstPayToPubKeyCompOdd,
		cstPayToPubKeyUncompEven, cstPayToPubKeyUncompOdd:
		return 33
	}

	scriptSize -= numSpecialScripts
	scriptSize += uint64(bytesRead)
	return int(scriptSize)
}

// decompressScriptV1 returns the original script obtained by decompressing the
// passed v1 compressed script according to the domain specific compression
// algorithm described above.
//
// NOTE: The script parameter must already have been proven to be long enough
// to contain the number of bytes returned by decodeCompressedScriptSize or it
// will panic.  This is acceptable since it is only an internal function.
func decompressScriptV1(compressedPkScript []byte) []byte {
	const (
		// Hardcoded constants so updates do not affect old upgrades.
		cstPayToPubKeyHash       = 0
		cstPayToScriptHash       = 1
		cstPayToPubKeyCompEven   = 2
		cstPayToPubKeyCompOdd    = 3
		cstPayToPubKeyUncompEven = 4
		cstPayToPubKeyUncompOdd  = 5
		numSpecialScripts        = 64
	)

	// Empty scripts, specified by 0x00, are considered nil.
	if len(compressedPkScript) == 0 {
		return nil
	}

	// Decode the script size and examine it for the special cases.
	encodedScriptSize, bytesRead := deserializeVLQ(compressedPkScript)
	switch encodedScriptSize {
	// Pay-to-pubkey-hash script.  The resulting script is:
	// <OP_DUP><OP_HASH160><20 byte hash><OP_EQUALVERIFY><OP_CHECKSIG>
	case cstPayToPubKeyHash:
		pkScript := make([]byte, 25)
		pkScript[0] = txscript.OP_DUP
		pkScript[1] = txscript.OP_HASH160
		pkScript[2] = txscript.OP_DATA_20
		copy(pkScript[3:], compressedPkScript[bytesRead:bytesRead+20])
		pkScript[23] = txscript.OP_EQUALVERIFY
		pkScript[24] = txscript.OP_CHECKSIG
		return pkScript

	// Pay-to-script-hash script.  The resulting script is:
	// <OP_HASH160><20 byte script hash><OP_EQUAL>
	case cstPayToScriptHash:
		pkScript := make([]byte, 23)
		pkScript[0] = txscript.OP_HASH160
		pkScript[1] = txscript.OP_DATA_20
		copy(pkScript[2:], compressedPkScript[bytesRead:bytesRead+20])
		pkScript[22] = txscript.OP_EQUAL
		return pkScript

	// Pay-to-compressed-pubkey script.  The resulting script is:
	// <OP_DATA_33><33 byte compressed pubkey><OP_CHECKSIG>
	case cstPayToPubKeyCompEven, cstPayToPubKeyCompOdd:
		pkScript := make([]byte, 35)
		pkScript[0] = txscript.OP_DATA_33
		oddness := byte(0x02)
		if encodedScriptSize == cstPayToPubKeyCompOdd {
			oddness = 0x03
		}
		pkScript[1] = oddness
		copy(pkScript[2:], compressedPkScript[bytesRead:bytesRead+32])
		pkScript[34] = txscript.OP_CHECKSIG
		return pkScript

	// Pay-to-uncompressed-pubkey script.  The resulting script is:
	// <OP_DATA_65><65 byte uncompressed pubkey><OP_CHECKSIG>
	case cstPayToPubKeyUncompEven, cstPayToPubKeyUncompOdd:
		// Change the leading byte to the appropriate compressed pubkey
		// identifier (0x02 or 0x03) so it can be decoded as a
		// compressed pubkey.  This really should never fail since the
		// encoding ensures it is valid before compressing to this type.
		compressedKey := make([]byte, 33)
		oddness := byte(0x02)
		if encodedScriptSize == cstPayToPubKeyUncompOdd {
			oddness = 0x03
		}
		compressedKey[0] = oddness
		copy(compressedKey[1:], compressedPkScript[1:])
		key, err := secp256k1.ParsePubKey(compressedKey)
		if err != nil {
			return nil
		}

		pkScript := make([]byte, 67)
		pkScript[0] = txscript.OP_DATA_65
		copy(pkScript[1:], key.SerializeUncompressed())
		pkScript[66] = txscript.OP_CHECKSIG
		return pkScript
	}

	// When none of the special cases apply, the script was encoded using
	// the general format, so reduce the script size by the number of
	// special cases and return the unmodified script.
	scriptSize := int(encodedScriptSize - numSpecialScripts)
	pkScript := make([]byte, scriptSize)
	copy(pkScript, compressedPkScript[bytesRead:bytesRead+scriptSize])
	return pkScript
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
			scriptSize := decodeCompressedScriptSizeV1(serialized[offset:])
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
				script:  decompressScriptV1(pkScript),
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

// -----------------------------------------------------------------------------
// The version 3 block index consists of an entry for every known block.  It
// consists of information such as the block header and information about votes.
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
//   <block header><status><num votes><votes info>
//
//   Field              Type                Size
//   block header       wire.BlockHeader    180 bytes
//   status             byte                1 byte
//   num votes          VLQ                 variable
//   vote info
//     vote version     VLQ                 variable
//     vote bits        VLQ                 variable
//
// The version 3 block status flags format is:
//
//   bit  0   - block payload is stored on disk
//   bit  1   - block and all of its ancestors have been fully validated
//   bit  2   - block failed validation
//   bit  3   - an ancestor of the block failed validation
//   bits 4-7 - unused
// -----------------------------------------------------------------------------

// blockIndexEntryV3 represents a version 3 block index database entry.
type blockIndexEntryV3 struct {
	header   wire.BlockHeader
	status   byte
	voteInfo []blockIndexVoteVersionTuple
}

// blockIndexKeyV3 generates the binary key for an entry in the version 3 block
// index bucket.  The key is composed of the block height encoded as a
// big-endian 32-bit unsigned int followed by the 32 byte block hash.  Big
// endian is used here so the entries can easily be iterated by height.
func blockIndexKeyV3(blockHash *chainhash.Hash, blockHeight uint32) []byte {
	indexKey := make([]byte, chainhash.HashSize+4)
	binary.BigEndian.PutUint32(indexKey[0:4], blockHeight)
	copy(indexKey[4:chainhash.HashSize+4], blockHash[:])
	return indexKey
}

// blockIndexEntrySerializeSizeV3 returns the number of bytes it would take to
// serialize the passed block index entry according to the version 3 format
// described above.
func blockIndexEntrySerializeSizeV3(entry *blockIndexEntryV3) int {
	voteInfoSize := 0
	for i := range entry.voteInfo {
		voteInfoSize += serializeSizeVLQ(uint64(entry.voteInfo[i].version)) +
			serializeSizeVLQ(uint64(entry.voteInfo[i].bits))
	}

	const blockHdrSize = 180
	return blockHdrSize + 1 + serializeSizeVLQ(uint64(len(entry.voteInfo))) +
		voteInfoSize
}

// putBlockIndexEntryV3 serializes the passed block index entry according to the
// version 3 format described above directly into the passed target byte slice.
// The target byte slice must be at least large enough to handle the number of
// bytes returned by the blockIndexEntrySerializeSize function or it will panic.
func putBlockIndexEntryV3(target []byte, entry *blockIndexEntryV3) (int, error) {
	// Serialize the entire block header.
	w := bytes.NewBuffer(target[0:0])
	if err := entry.header.Serialize(w); err != nil {
		return 0, err
	}

	// Serialize the status.
	const blockHdrSize = 180
	offset := blockHdrSize
	target[offset] = entry.status
	offset++

	// Serialize the number of votes and associated vote information.
	offset += putVLQ(target[offset:], uint64(len(entry.voteInfo)))
	for i := range entry.voteInfo {
		offset += putVLQ(target[offset:], uint64(entry.voteInfo[i].version))
		offset += putVLQ(target[offset:], uint64(entry.voteInfo[i].bits))
	}

	return offset, nil
}

// serializeBlockIndexEntryV3 serializes the passed block index entry into a
// single byte slice according to the version 3 format described in detail
// above.
func serializeBlockIndexEntryV3(entry *blockIndexEntryV3) ([]byte, error) {
	serialized := make([]byte, blockIndexEntrySerializeSizeV3(entry))
	_, err := putBlockIndexEntryV3(serialized, entry)
	return serialized, err
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

			// Write the block index entry serialized with the new format to the
			// new bucket.
			serialized, err := serializeBlockIndexEntryV3(&blockIndexEntryV3{
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
				scriptSize := decodeCompressedScriptSizeV1(serialized[offset:])
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

// migrateUtxoSetVersion2To3 migrates all utxoset entries from the v2 bucket to
// a v3 bucket and removes the old v2 bucket.
//
// The utxoset is guaranteed to be fully updated if this returns without
// failure.
func migrateUtxoSetVersion2To3(ctx context.Context, db database.DB) error {
	// Hardcoded bucket and key names so updates do not affect old upgrades.
	v2BucketName := []byte("utxosetv2")
	v3BucketName := []byte("utxosetv3")

	log.Info("Migrating database utxoset.  This may take a while...")
	start := time.Now()

	// Create the new utxoset bucket as needed.
	err := db.Update(func(dbTx database.Tx) error {
		_, err := dbTx.Metadata().CreateBucketIfNotExists(v3BucketName)
		return err
	})
	if err != nil {
		return err
	}

	// doBatch contains the primary logic for upgrading the utxoset from version
	// 2 to 3 in batches.  This is done because attempting to migrate in a
	// single database transaction could result in massive memory usage and
	// could potentially crash on many systems due to ulimits.
	//
	// It returns whether or not all entries have been updated.
	const maxEntries = 20000
	var resumeOffset uint32
	var totalMigrated uint64
	doBatch := func(dbTx database.Tx) (bool, error) {
		meta := dbTx.Metadata()
		v2Bucket := meta.Bucket(v2BucketName)
		if v2Bucket == nil {
			return false, fmt.Errorf("bucket %s does not exist", v2BucketName)
		}

		v3Bucket := meta.Bucket(v3BucketName)
		if v3Bucket == nil {
			return false, fmt.Errorf("bucket %s does not exist", v3BucketName)
		}

		// Migrate utxoset entries so long as the max number of entries for this
		// batch has not been exceeded.
		var logProgress bool
		var numMigrated, numIterated uint32
		err := v2Bucket.ForEach(func(oldKey, oldSerialized []byte) error {
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

			// Old key was the transaction hash.
			var txHash chainhash.Hash
			copy(txHash[:], oldKey)

			// Deserialize the legacy V2 entry which included all utxos for the given
			// transaction.
			//
			// The legacy V2 format is as follows:
			//
			//   <version><height><header code><unspentness bitmap>
			//     [<compressed txouts>,...]
			//
			//   Field                 Type     Size
			//   transaction version   VLQ      variable
			//   block height          VLQ      variable
			//   block index           VLQ      variable
			//   flags                 VLQ      variable (currently 1 byte)
			//   header code           VLQ      variable
			//   unspentness bitmap    []byte   variable
			//   compressed txouts
			//     compressed amount   VLQ      variable
			//     script version      VLQ      variable
			//     compressed script   []byte   variable
			//   stakeExtra            []byte   variable
			//
			// The serialized flags code format is:
			//   bit  0   - containing transaction is a coinbase
			//   bit  1   - containing transaction has an expiry
			//   bits 2-4 - transaction type
			//   bit  5   - unused
			//   bit  6   - is fully spent
			//   bit  7   - unused
			//
			// The serialized header code format is:
			//   bit 0 - output zero is unspent
			//   bit 1 - output one is unspent
			//   bits 2-x - number of bytes in unspentness bitmap.  When both bits 1
			//     and 2 are unset, it encodes N-1 since there must be at least one
			//     unspent output.
			//
			// The stake extra field contains minimally encoded outputs for all
			// consensus-related outputs in the stake transaction. It is only
			// encoded for tickets.

			// Deserialize the version.
			//
			// NOTE: Ignore version since it is no longer used in the new format.
			_, bytesRead := deserializeVLQ(oldSerialized)
			offset := bytesRead
			if offset >= len(oldSerialized) {
				return errDeserialize("unexpected end of data after version")
			}

			// Deserialize the block height.
			blockHeight, bytesRead := deserializeVLQ(oldSerialized[offset:])
			offset += bytesRead
			if offset >= len(oldSerialized) {
				return errDeserialize("unexpected end of data after height")
			}

			// Deserialize the block index.
			blockIndex, bytesRead := deserializeVLQ(oldSerialized[offset:])
			offset += bytesRead
			if offset >= len(oldSerialized) {
				return errDeserialize("unexpected end of data after index")
			}

			// Deserialize the flags.
			flags, bytesRead := deserializeVLQ(oldSerialized[offset:])
			offset += bytesRead
			if offset >= len(oldSerialized) {
				return errDeserialize("unexpected end of data after flags")
			}
			// Decode the flags.  The format is:
			//     0: Is coinbase
			//     1: Has an expiry
			//   2-4: Transaction type
			//     5: Unused
			//     6: Fully spent
			//     7: Unused
			isCoinBase := flags&0x01 != 0
			hasExpiry := flags&(1<<1) != 0
			txType := stake.TxType((flags & 0x1c) >> 2)

			// Deserialize the header code.
			code, bytesRead := deserializeVLQ(oldSerialized[offset:])
			offset += bytesRead
			if offset >= len(oldSerialized) {
				return errDeserialize("unexpected end of data after header")
			}

			// Decode the header code.
			//
			// Bit 0 indicates output 0 is unspent.
			// Bit 1 indicates output 1 is unspent.
			// Bits 2-x encodes the number of non-zero unspentness bitmap bytes that
			// follow.  When both output 0 and 1 are spent, it encodes N-1.
			output0Unspent := code&0x01 != 0
			output1Unspent := code&0x02 != 0
			numBitmapBytes := code >> 2
			if !output0Unspent && !output1Unspent {
				numBitmapBytes++
			}

			// Ensure there are enough bytes left to deserialize the unspentness
			// bitmap.
			if uint64(len(oldSerialized[offset:])) < numBitmapBytes {
				return errDeserialize("unexpected end of data for " +
					"unspentness bitmap")
			}

			// Add sparse outputs for unspent outputs 0 and 1 as needed based on the
			// details provided by the header code.
			var outputIndexes []uint32
			if output0Unspent {
				outputIndexes = append(outputIndexes, 0)
			}
			if output1Unspent {
				outputIndexes = append(outputIndexes, 1)
			}

			// Decode the unspentness bitmap adding a sparse output for each unspent
			// output.
			for i := uint32(0); i < uint32(numBitmapBytes); i++ {
				unspentBits := oldSerialized[offset]
				for j := uint32(0); j < 8; j++ {
					if unspentBits&0x01 != 0 {
						// The first 2 outputs are encoded via the
						// header code, so adjust the output number
						// accordingly.
						outputNum := 2 + i*8 + j
						outputIndexes = append(outputIndexes, outputNum)
					}
					unspentBits >>= 1
				}
				offset++
			}

			// Create a map to hold all of the converted outputs for the entry.
			type convertedOut struct {
				compressedAmount uint64
				compressedScript []byte
				scriptVersion    uint64
			}
			outputs := make(map[uint32]*convertedOut)

			// Decode and add all of the outputs.
			for _, outputIndex := range outputIndexes {
				// Deserialize the compressed amount and ensure there are bytes
				// remaining for the compressed script.
				compressedAmount, bytesRead := deserializeVLQ(oldSerialized[offset:])
				if bytesRead == 0 {
					return errDeserialize("unexpected end of data during decoding " +
						"(compressed amount)")
				}
				offset += bytesRead

				// Decode the script version.
				scriptVersion, bytesRead := deserializeVLQ(oldSerialized[offset:])
				if bytesRead == 0 {
					return errDeserialize("unexpected end of data during decoding " +
						"(script version)")
				}
				offset += bytesRead

				// Decode the compressed script size and ensure there are enough bytes
				// left in the slice for it.
				scriptSize := decodeCompressedScriptSizeV1(oldSerialized[offset:])
				// Note: scriptSize == 0 is OK (an empty compressed script is valid)
				if scriptSize < 0 {
					return errDeserialize("negative script size")
				}
				if len(oldSerialized[offset:]) < scriptSize {
					return errDeserialize(fmt.Sprintf("unexpected end of "+
						"data after script size (got %v, need %v)",
						len(oldSerialized[offset:]), scriptSize))
				}
				compressedScript := oldSerialized[offset : offset+scriptSize]

				offset += scriptSize

				// Create a converted utxo entry with the details deserialized above.
				outputs[outputIndex] = &convertedOut{
					compressedAmount: compressedAmount,
					compressedScript: compressedScript,
					scriptVersion:    scriptVersion,
				}
			}
			// Read the minimal outputs if this was a ticket.
			var ticketMinOuts []byte
			if txType == stake.TxTypeSStx {
				sz, err := determineMinimalOutputsSizeV1(oldSerialized[offset:])
				if err != nil {
					return errDeserialize(fmt.Sprintf("unable to decode "+
						"ticket outputs: %v", err))
				}

				// Read the ticket minimal outputs.
				ticketMinOuts = oldSerialized[offset : offset+sz]
			}

			// Create V3 utxo entries with the details deserialized above.
			//
			// The V3 serialized key format is:
			//
			//   <hash><tree><output index>
			//
			//   Field                Type             Size
			//   hash                 chainhash.Hash   chainhash.HashSize
			//   tree                 VLQ              variable
			//   output index         VLQ              variable
			//
			// The V3 serialized value format is:
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
			// The ticket min outs field contains minimally encoded outputs for all
			// outputs of a ticket transaction. It is only encoded for ticket outputs.
			for outputIndex, output := range outputs {
				// Encode the V3 utxo flags.
				encodedFlags := uint8(txType) << 2
				if isCoinBase {
					encodedFlags |= 1
				}
				if hasExpiry {
					encodedFlags |= 1 << 1
				}

				// Calculate the size needed to serialize the entry.
				size := serializeSizeVLQ(blockHeight) +
					serializeSizeVLQ(blockIndex) +
					serializeSizeVLQ(uint64(encodedFlags)) +
					serializeSizeVLQ(output.compressedAmount) +
					serializeSizeVLQ(output.scriptVersion) +
					len(output.compressedScript)

				// Only store the ticket minimal outputs in the ticket submission
				// output (output index 0).
				if outputIndex == 0 {
					size += len(ticketMinOuts)
				}

				// Serialize the entry.
				reserialized := make([]byte, size)
				reserializedOffset := putVLQ(reserialized, blockHeight)
				reserializedOffset += putVLQ(reserialized[reserializedOffset:],
					blockIndex)
				reserializedOffset += putVLQ(reserialized[reserializedOffset:],
					uint64(encodedFlags))
				reserializedOffset += putVLQ(reserialized[reserializedOffset:],
					output.compressedAmount)
				reserializedOffset += putVLQ(reserialized[reserializedOffset:],
					output.scriptVersion)
				copy(reserialized[reserializedOffset:], output.compressedScript)
				reserializedOffset += len(output.compressedScript)

				// Only store the ticket minimal outputs in the ticket submission
				// output (output index 0).
				if ticketMinOuts != nil && outputIndex == 0 {
					copy(reserialized[reserializedOffset:], ticketMinOuts)
				}

				// Create the key for the new entry.
				tree := wire.TxTreeRegular
				if txType != stake.TxTypeRegular {
					tree = wire.TxTreeStake
				}
				keySize := chainhash.HashSize + serializeSizeVLQ(uint64(tree)) +
					serializeSizeVLQ(uint64(outputIndex))
				key := make([]byte, keySize)
				copy(key, txHash[:])
				keyOffset := chainhash.HashSize
				keyOffset += putVLQ(key[keyOffset:], uint64(tree))
				putVLQ(key[keyOffset:], uint64(outputIndex))

				// Create the new entry in the V3 bucket.
				err = v3Bucket.Put(key, reserialized)
				if err != nil {
					return err
				}
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

	// Drop version 2 utxoset.
	log.Info("Removing old utxoset entries...")
	start = time.Now()
	err = incrementalFlatDrop(ctx, db, v2BucketName, "old utxoset")
	if err != nil {
		return err
	}
	elapsed = time.Since(start).Round(time.Millisecond)
	log.Infof("Done removing old utxoset entries in %v", elapsed)
	return nil
}

// migrateSpendJournalVersion2To3 migrates all spend journal entries from the v2
// bucket to a v3 bucket and removes the old v2 bucket.
//
// The spend journal entries are guaranteed to be fully updated if this returns
// without failure.
func migrateSpendJournalVersion2To3(ctx context.Context, b *BlockChain) error {
	// Hardcoded bucket and key names so updates do not affect old upgrades.
	v2SpendJournalBucketName := []byte("spendjournalv2")
	v3SpendJournalBucketName := []byte("spendjournalv3")
	v3BlockIndexBucketName := []byte("blockidxv3")
	v2UtxoSetBucketName := []byte("utxosetv2")
	tmpTxInfoBucketName := []byte("tmpTxInfo")

	log.Info("Migrating database spend journal.  This may take a while...")
	start := time.Now()

	// Create the new spend journal bucket as needed.
	err := b.db.Update(func(dbTx database.Tx) error {
		_, err := dbTx.Metadata().CreateBucketIfNotExists(v3SpendJournalBucketName)
		return err
	})
	if err != nil {
		return err
	}

	// Create the temp bucket as needed.  This bucket is used to temporarily store
	// tx info for fully spent transactions.
	err = b.db.Update(func(dbTx database.Tx) error {
		_, err := dbTx.Metadata().CreateBucketIfNotExists(tmpTxInfoBucketName)
		return err
	})
	if err != nil {
		return err
	}

	// doBatch contains the primary logic for upgrading the spend journal from
	// version 2 to 3 in batches.  This is done because attempting to migrate in a
	// single database transaction could result in massive memory usage and
	// could potentially crash on many systems due to ulimits.
	//
	// It returns whether or not all entries have been updated.
	const maxEntries = 20000
	var resumeOffset uint32
	var totalMigrated uint64
	doBatch := func(dbTx database.Tx) (bool, error) {
		meta := dbTx.Metadata()

		v2SpendJournalBucket := meta.Bucket(v2SpendJournalBucketName)
		if v2SpendJournalBucket == nil {
			return false, fmt.Errorf("bucket %s does not exist",
				v2SpendJournalBucketName)
		}

		v3SpendJournalBucket := meta.Bucket(v3SpendJournalBucketName)
		if v3SpendJournalBucket == nil {
			return false, fmt.Errorf("bucket %s does not exist",
				v3SpendJournalBucketName)
		}

		v3BlockIndexBucket := meta.Bucket(v3BlockIndexBucketName)
		if v3BlockIndexBucket == nil {
			return false, fmt.Errorf("bucket %s does not exist",
				v3BlockIndexBucketName)
		}

		v2UtxoSetBucket := meta.Bucket(v2UtxoSetBucketName)
		if v2UtxoSetBucket == nil {
			return false, fmt.Errorf("bucket %s does not exist",
				v2UtxoSetBucketName)
		}

		tmpTxInfoBucket := meta.Bucket(tmpTxInfoBucketName)
		if tmpTxInfoBucket == nil {
			return false, fmt.Errorf("bucket %s does not exist",
				tmpTxInfoBucketName)
		}

		// Migrate spend journal entries so long as the max number of entries for
		// this batch has not been exceeded.
		//
		// Use the block index to iterate through every block in reverse order.  It
		// is necessary to read the blocks in reverse order since the V2 spend
		// journal conditionally stored tx info when the output was the last spent
		// output of the containing transaction.  Therefore, we need to find the
		// last spent outputs first and temporarily store the associated tx info so
		// that it is available when creating the corresponding V3 entries.
		var logProgress bool
		var numMigrated, numIterated uint32
		cursor := v3BlockIndexBucket.Cursor()
		for ok := cursor.Last(); ok; ok = cursor.Prev() {
			// Reset err on each iteration.
			err = nil

			if interruptRequested(ctx) {
				logProgress = true
				err = errInterruptRequested
				break
			}

			if numMigrated >= maxEntries {
				logProgress = true
				err = errBatchFinished
				break
			}

			// Skip entries that have already been migrated in previous batches.
			numIterated++
			if numIterated-1 < resumeOffset {
				continue
			}
			resumeOffset++

			// Deserialize the header from the V3 block index entry.  The header is
			// the first item in the serialized entry.
			serializedBlockIndexEntry := cursor.Value()
			if len(serializedBlockIndexEntry) < blockHdrSize {
				return false, errDeserialize("unexpected end of data while " +
					"reading block header")
			}
			hB := serializedBlockIndexEntry[0:blockHdrSize]
			var header wire.BlockHeader
			if err := header.Deserialize(bytes.NewReader(hB)); err != nil {
				return false, err
			}

			// Skip entries that have already been migrated in previous interrupted
			// upgrades.
			blockHash := header.BlockHash()
			if v3SpendJournalBucket.Get(blockHash[:]) != nil {
				continue
			}

			// Load the full block from the database.
			blockBytes, err := dbTx.FetchBlock(&blockHash)
			if err != nil {
				break
			}
			var msgBlock wire.MsgBlock
			if err = msgBlock.FromBytes(blockBytes); err != nil {
				break
			}

			// Determine if treasury agenda is active.
			isTreasuryEnabled := false
			if msgBlock.Header.Height > 0 {
				parentHash := msgBlock.Header.PrevBlock
				isTreasuryEnabled, err = b.IsTreasuryAgendaActive(&parentHash)
				if err != nil {
					break
				}
			}

			// Deserialize the legacy V2 spend journal entry.
			//
			// The legacy V2 format is as follows:
			//
			//   [<flags><script version><compressed pk script>],...
			//   OPTIONAL: [<txVersion><stakeExtra>]
			//
			//   Field                Type           Size
			//   flags                VLQ            byte
			//   scriptVersion        uint16         2 bytes
			//   pkScript             VLQ+[]byte     variable
			//
			//   OPTIONAL
			//     txVersion          VLQ            variable
			//     stakeExtra         []byte         variable
			//
			// The serialized flags code format is:
			//   bit  0   - containing transaction is a coinbase
			//   bit  1   - containing transaction has an expiry
			//   bits 2-4 - transaction type
			//   bit  5   - unused
			//   bit  6   - is fully spent
			//   bit  7   - unused
			//
			// The stake extra field contains minimally encoded outputs for all
			// consensus-related outputs in the stake transaction. It is only
			// encoded for tickets.
			//
			//   NOTE: The transaction version and flags are only encoded when the
			//   spent txout was the final unspent output of the containing
			//   transaction.  Otherwise, the header code will be 0 and the version is
			//   not serialized at all. This is done because that information is only
			//   needed when the utxo set no longer has it.

			// Get the serialized V2 data.
			v2Serialized := v2SpendJournalBucket.Get(blockHash[:])

			// Continue if there is no spend journal entry for the block.
			if v2Serialized == nil {
				continue
			}

			// Exclude the coinbase transaction since it can't spend anything.
			blockTxns := make([]*wire.MsgTx, 0, len(msgBlock.STransactions)+
				len(msgBlock.Transactions[1:]))
			if len(msgBlock.STransactions) > 0 && isTreasuryEnabled {
				// Skip treasury base and remove tspends.
				for _, v := range msgBlock.STransactions[1:] {
					if stake.IsTSpend(v) {
						continue
					}
					blockTxns = append(blockTxns, v)
				}
			} else {
				blockTxns = append(blockTxns, msgBlock.STransactions...)
			}
			blockTxns = append(blockTxns, msgBlock.Transactions[1:]...)

			// Calculate the total number of stxos.
			var numStxos int
			for _, tx := range blockTxns {
				if stake.IsSSGen(tx, isTreasuryEnabled) {
					numStxos++
					continue
				}
				numStxos += len(tx.TxIn)
			}

			// If there is an empty spend journal entry for the block, create an empty
			// entry in the V3 bucket and continue.
			if len(v2Serialized) == 0 {
				// Ensure the block actually has no stxos.  This should never
				// happen unless there is database corruption or an empty entry
				// erroneously made its way into the database.
				if numStxos != 0 {
					return false, AssertError(fmt.Sprintf("mismatched spend "+
						"journal serialization - no serialization for "+
						"expected %d stxos", numStxos))
				}

				err = v3SpendJournalBucket.Put(blockHash[:], nil)
				if err != nil {
					return false, err
				}

				continue
			}

			// Create a slice to hold all of the converted stxos.
			type convertedStxo struct {
				compScript    []byte
				ticketMinOuts []byte
				scriptVersion uint16
				flags         uint8
				txOutIndex    uint32
			}
			stxos := make([]convertedStxo, numStxos)

			// Loop backwards through all transactions so everything is read in
			// reverse order to match the serialization order.
			stxoIdx := numStxos - 1
			offset := 0
			for txIdx := len(blockTxns) - 1; txIdx > -1; txIdx-- {
				tx := blockTxns[txIdx]
				isVote := stake.IsSSGen(tx, isTreasuryEnabled)

				// Loop backwards through all of the transaction inputs and read
				// the associated stxo.
				for txInIdx := len(tx.TxIn) - 1; txInIdx > -1; txInIdx-- {
					// Skip stakebase since it has no input.
					if txInIdx == 0 && isVote {
						continue
					}

					txIn := tx.TxIn[txInIdx]
					stxo := &stxos[stxoIdx]
					stxoIdx--

					// Set the tx out index that the stxo is associated with.
					stxo.txOutIndex = txIn.PreviousOutPoint.Index

					// Deserialize the flags.
					flags, bytesRead := deserializeVLQ(v2Serialized[offset:])
					if bytesRead == 0 {
						return false, errDeserialize("unexpected end of data during " +
							"decoding (flags)")
					}
					offset += bytesRead

					// Decode the script version.
					scriptVersion, bytesRead := deserializeVLQ(v2Serialized[offset:])
					if bytesRead == 0 {
						return false, errDeserialize("unexpected end of " +
							"data during decoding (script version)")
					}
					offset += bytesRead
					stxo.scriptVersion = uint16(scriptVersion)

					// Decode the compressed script size and ensure there are enough bytes
					// left in the slice for it.
					scriptSize := decodeCompressedScriptSizeV1(v2Serialized[offset:])
					// Note: scriptSize == 0 is OK (an empty compressed script is valid)
					if scriptSize < 0 {
						return false, errDeserialize("negative script size")
					}
					if len(v2Serialized[offset:]) < scriptSize {
						return false, errDeserialize(fmt.Sprintf("unexpected end of "+
							"data after script size (got %v, need %v)",
							len(v2Serialized[offset:]), scriptSize))
					}
					compressedScript := v2Serialized[offset : offset+scriptSize]
					offset += scriptSize
					stxo.compScript = compressedScript

					// Deserialize the containing transaction if the flags indicate that
					// the transaction has been fully spent.
					// The flags format is:
					//   bit  0   - containing transaction is a coinbase
					//   bit  1   - containing transaction has an expiry
					//   bits 2-4 - transaction type
					//   bit  5   - unused
					//   bit  6   - is fully spent
					//   bit  7   - unused
					fullySpent := flags&(1<<6) != 0
					if fullySpent {
						txType := stake.TxType((flags & 0x1c) >> 2)
						stxo.flags = uint8(flags)

						// Unset bit 5 in case it was unexpectedly set, since bit 5 will be
						// used for the transaction type in the new format.
						stxo.flags &^= 1 << 5

						// Unset the spent flag since it is no longer needed in the new
						// version.
						stxo.flags &^= 1 << 6

						// Deserialize the version and ignore it since it is no longer
						// used in the new format.
						_, bytesRead := deserializeVLQ(v2Serialized[offset:])
						if bytesRead == 0 {
							return false, errDeserialize("unexpected end of " +
								"data during decoding (tx version)")
						}
						offset += bytesRead

						if txType == stake.TxTypeSStx {
							sz, err := determineMinimalOutputsSizeV1(v2Serialized[offset:])
							if err != nil {
								return false, errDeserialize(fmt.Sprintf("unable to decode "+
									"ticket outputs: %v", err))
							}

							// Read the ticket minimal outputs.
							stxo.ticketMinOuts = v2Serialized[offset : offset+sz]
							offset += sz
						}

						// Save the fully spent tx info in the temp bucket.  Store the
						// fields that were conditionally stored in V2, which includes flags
						// and ticket minimal outputs.
						//
						// The serialized format is:
						//   flags             VLQ            byte
						//   ticketMinOuts     []byte         variable
						//
						// The flags format is:
						//   bit  0   - containing transaction is a coinbase
						//   bit  1   - containing transaction has an expiry
						//   bits 2-5 - transaction type
						//   bits 6-7 - unused
						//
						// The ticket min outs field is only stored for tickets.
						size := serializeSizeVLQ(uint64(stxo.flags))
						if txType == stake.TxTypeSStx {
							size += len(stxo.ticketMinOuts)
						}
						target := make([]byte, size)
						tmpOffset := putVLQ(target, uint64(stxo.flags))
						if txType == stake.TxTypeSStx {
							copy(target[tmpOffset:], stxo.ticketMinOuts)
						}
						err = tmpTxInfoBucket.Put(txIn.PreviousOutPoint.Hash[:], target)
						if err != nil {
							return false, err
						}

						// Continue since we have everything we need for stxos that were
						// marked as fully spent.
						continue
					}

					// The stxo was not marked as fully spent (otherwise we already
					// continued above).  To get the missing tx info (flags and ticket
					// min outs), first check the tmp bucket, and if not found, then
					// check the utxo set.

					// First, check the temp bucket for the tx info.
					tmpSerialized := tmpTxInfoBucket.Get(txIn.PreviousOutPoint.Hash[:])
					if len(tmpSerialized) != 0 {
						flags, bytesRead := deserializeVLQ(tmpSerialized)
						tOffset := bytesRead
						stxo.flags = uint8(flags)

						// Decode the flags.  The format is:
						//   bit  0   - containing transaction is a coinbase
						//   bit  1   - containing transaction has an expiry
						//   bits 2-5 - transaction type
						//   bits 6-7 - unused
						txType := stake.TxType((flags & 0x3c) >> 2)
						// Read the minimal outputs if this was a ticket submission output.
						if txType == stake.TxTypeSStx && stxo.txOutIndex == 0 {
							sz, err := determineMinimalOutputsSizeV1(tmpSerialized[tOffset:])
							if err != nil {
								return false, errDeserialize(fmt.Sprintf("unable to decode "+
									"ticket outputs: %v", err))
							}

							// Read the ticket minimal outputs.
							stxo.ticketMinOuts = tmpSerialized[tOffset : tOffset+sz]
						}

						// Continue to the next stxo since the flags and ticket min outs
						// have now been set.
						continue
					}

					// If the temp bucket didn't have the tx info, check the utxo set for
					// the tx info.  The key for the V2 utxo set is the transaction hash.
					utxoSerialized := v2UtxoSetBucket.Get(txIn.PreviousOutPoint.Hash[:])

					// Deserialize the legacy V2 entry which included all utxos for the
					// given transaction.
					//
					// The legacy V2 format is as follows:
					//
					//   <version><height><header code><unspentness bitmap>
					//     [<compressed txouts>,...]
					//
					//   Field                 Type     Size
					//   transaction version   VLQ      variable
					//   block height          VLQ      variable
					//   block index           VLQ      variable
					//   flags                 VLQ      variable (currently 1 byte)
					//   header code           VLQ      variable
					//   unspentness bitmap    []byte   variable
					//   compressed txouts
					//     compressed amount   VLQ      variable
					//     script version      VLQ      variable
					//     compressed script   []byte   variable
					//   stakeExtra            []byte   variable
					//
					// The serialized flags code format is:
					//   bit  0   - containing transaction is a coinbase
					//   bit  1   - containing transaction has an expiry
					//   bits 2-4 - transaction type
					//   bit  5   - unused
					//   bit  6   - is fully spent
					//   bit  7   - unused
					//
					// The serialized header code format is:
					//   bit 0 - output zero is unspent
					//   bit 1 - output one is unspent
					//   bits 2-x - number of bytes in unspentness bitmap.  When both bits
					//     1 and 2 are unset, it encodes N-1 since there must be at least
					//     one unspent output.
					//
					// The stake extra field contains minimally encoded outputs for all
					// consensus-related outputs in the stake transaction. It is only
					// encoded for tickets.

					// Deserialize the version.  Ignore it since we don't need it.
					_, bytesRead = deserializeVLQ(utxoSerialized)
					utxoOffset := bytesRead
					if utxoOffset >= len(utxoSerialized) {
						return false, errDeserialize("unexpected end of data after version")
					}

					// Deserialize the block height.  Ignore it since we don't need it.
					_, bytesRead = deserializeVLQ(utxoSerialized[utxoOffset:])
					utxoOffset += bytesRead
					if utxoOffset >= len(utxoSerialized) {
						return false, errDeserialize("unexpected end of data after height")
					}

					// Deserialize the block index. Ignore it since we don't need it.
					_, bytesRead = deserializeVLQ(utxoSerialized[utxoOffset:])
					utxoOffset += bytesRead
					if utxoOffset >= len(utxoSerialized) {
						return false, errDeserialize("unexpected end of data after index")
					}

					// Deserialize the flags.  The flags format is:
					//     0: Is coinbase
					//     1: Has an expiry
					//   2-4: Transaction type
					//     5: Unused
					//     6: Fully spent
					//     7: Unused
					v2UtxoFlags, bytesRead := deserializeVLQ(utxoSerialized[utxoOffset:])
					utxoOffset += bytesRead
					if utxoOffset >= len(utxoSerialized) {
						return false, errDeserialize("unexpected end of data after flags")
					}

					// Unset bit 5 in case it was unexpectedly set, since bit 5 will be
					// used for the transaction type in the new format.
					v2UtxoFlags &^= 1 << 5

					// Unset the fully spent flag since it is no longer needed in the new
					// version.  It shouldn't have ever been set, since spent utxos are
					// not serialized, but unset it just in case.
					v2UtxoFlags &^= 1 << 6

					// Set the flags on the stxo.
					stxo.flags = uint8(v2UtxoFlags)

					// Deserialize the header code.
					code, bytesRead := deserializeVLQ(utxoSerialized[utxoOffset:])
					utxoOffset += bytesRead
					if utxoOffset >= len(utxoSerialized) {
						return false, errDeserialize("unexpected end of data after header")
					}

					// Decode the header code.
					//
					// Bit 0 indicates output 0 is unspent.
					// Bit 1 indicates output 1 is unspent.
					// Bits 2-x encodes the number of non-zero unspentness bitmap bytes
					// that follow.  When both output 0 and 1 are spent, it encodes N-1.
					output0Unspent := code&0x01 != 0
					output1Unspent := code&0x02 != 0
					numBitmapBytes := code >> 2
					if !output0Unspent && !output1Unspent {
						numBitmapBytes++
					}

					// Ensure there are enough bytes left to deserialize the unspentness
					// bitmap.
					if uint64(len(utxoSerialized[utxoOffset:])) < numBitmapBytes {
						return false, errDeserialize("unexpected end of data for " +
							"unspentness bitmap")
					}

					// Add sparse outputs for unspent outputs 0 and 1 as needed based on
					// the details provided by the header code.
					var outputIndexes []uint32
					if output0Unspent {
						outputIndexes = append(outputIndexes, 0)
					}
					if output1Unspent {
						outputIndexes = append(outputIndexes, 1)
					}

					// Decode the unspentness bitmap adding a sparse output for each
					// unspent output.
					for i := uint32(0); i < uint32(numBitmapBytes); i++ {
						unspentBits := utxoSerialized[utxoOffset]
						for j := uint32(0); j < 8; j++ {
							if unspentBits&0x01 != 0 {
								// The first 2 outputs are encoded via the
								// header code, so adjust the output number
								// accordingly.
								outputNum := 2 + i*8 + j
								outputIndexes = append(outputIndexes, outputNum)
							}
							unspentBits >>= 1
						}
						utxoOffset++
					}

					// Decode and add all of the outputs.
					for range outputIndexes {
						// Deserialize the compressed amount.  Ignore it since we don't need
						// it.
						_, bytesRead = deserializeVLQ(utxoSerialized[utxoOffset:])
						if bytesRead == 0 {
							return false, errDeserialize("unexpected end of data during " +
								"decoding (compressed amount)")
						}
						utxoOffset += bytesRead

						// Decode the script version.  Ignore it since we don't need it.
						_, bytesRead = deserializeVLQ(utxoSerialized[utxoOffset:])
						if bytesRead == 0 {
							return false, errDeserialize("unexpected end of data during " +
								"decoding (script version)")
						}
						utxoOffset += bytesRead

						// Decode the compressed script size and ensure there are enough
						// bytes left in the slice for it.
						size := decodeCompressedScriptSizeV1(utxoSerialized[utxoOffset:])
						// Note: size == 0 is OK (an empty compressed script is valid)
						if size < 0 {
							return false, errDeserialize("negative script size")
						}
						if len(utxoSerialized[utxoOffset:]) < size {
							return false, errDeserialize(fmt.Sprintf("unexpected end of "+
								"data after script size (got %v, need %v)",
								len(utxoSerialized[utxoOffset:]), size))
						}

						utxoOffset += size
					}

					// Determine the tx type from the flags.  The flags format is:
					//     0: Is coinbase
					//     1: Has an expiry
					//   2-4: Transaction type
					//     5: Unused
					//     6: Fully spent
					//     7: Unused
					txType := stake.TxType((v2UtxoFlags & 0x1c) >> 2)

					// Read the minimal outputs if this was a ticket.
					if txType == stake.TxTypeSStx {
						sz, err := determineMinimalOutputsSizeV1(utxoSerialized[utxoOffset:])
						if err != nil {
							return false, errDeserialize(fmt.Sprintf("unable to decode "+
								"ticket outputs: %v", err))
						}

						// Read the ticket minimal outputs.  We only need the ticket minimal
						// outputs for the ticket submission output (output 0) in the new
						// format.
						if stxo.txOutIndex == 0 {
							stxo.ticketMinOuts = utxoSerialized[utxoOffset : utxoOffset+sz]
						}
					}
				}
			}

			// Create a V3 spend journal entry with the details deserialized above.
			//
			// The V3 serialized format is:
			//
			//   [<flags><script version><compressed pk script>],...
			//   OPTIONAL: <ticket min outs>
			//
			//   Field                Type           Size
			//   flags                VLQ            byte
			//   scriptVersion        uint16         2 bytes
			//   pkScript             VLQ+[]byte     variable
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
			// The ticket min outs field contains minimally encoded outputs for all
			// outputs of a ticket transaction. It is only encoded for ticket
			// submission outputs.

			// Calculate the size needed to serialize the entire journal entry.
			var size int
			sizes := make([]int, 0, len(stxos))
			for i := range stxos {
				stxo := &stxos[i]
				sz := serializeSizeVLQ(uint64(stxo.flags)) +
					serializeSizeVLQ(uint64(stxo.scriptVersion)) +
					len(stxo.compScript)

				// Determine the tx type from the flags.  The flags format is:
				//   bit  0     - containing transaction is a coinbase
				//   bit  1     - containing transaction has an expiry
				//   bits 2-5   - transaction type
				//   bits 6-7   - unused
				txType := stake.TxType((stxo.flags & 0x3c) >> 2)

				// Only store the minimal outputs if this was a ticket submission
				// output.
				if txType == stake.TxTypeSStx && stxo.txOutIndex == 0 {
					sz += len(stxo.ticketMinOuts)
				}

				sizes = append(sizes, sz)
				size += sz
			}
			reserialized := make([]byte, size)

			// Serialize each individual stxo directly into the slice in reverse
			// order one after the other.
			offset = 0
			for i := len(stxos) - 1; i > -1; i-- {
				oldOffset := offset
				stxo := &stxos[i]
				offset += putVLQ(reserialized[offset:], uint64(stxo.flags))
				offset += putVLQ(reserialized[offset:], uint64(stxo.scriptVersion))
				copy(reserialized[offset:], stxo.compScript)
				offset += len(stxo.compScript)

				// Determine the tx type from the flags.  The flags format is:
				//   bit  0     - containing transaction is a coinbase
				//   bit  1     - containing transaction has an expiry
				//   bits 2-5   - transaction type
				//   bits 6-7   - unused
				txType := stake.TxType((stxo.flags & 0x3c) >> 2)

				// Only store the minimal outputs if this was a ticket submission
				// output.
				if txType == stake.TxTypeSStx && stxo.txOutIndex == 0 {
					if len(stxo.ticketMinOuts) == 0 {
						return false, errDeserialize("missing ticket minimal output data " +
							"when serializing V3 stxo entry")
					}

					copy(reserialized[offset:], stxo.ticketMinOuts)
					offset += len(stxo.ticketMinOuts)
				}

				if offset-oldOffset != sizes[i] {
					return false, AssertError(fmt.Sprintf("bad write; expect sz %v, "+
						"got sz %v (wrote %x)", sizes[i], offset-oldOffset,
						reserialized[oldOffset:offset]))
				}
			}

			// Create the new entry in the V3 bucket.
			err = v3SpendJournalBucket.Put(blockHash[:], reserialized)
			if err != nil {
				return false, err
			}

			numMigrated++
		}
		isFullyDone := err == nil
		if (isFullyDone || logProgress) && numMigrated > 0 {
			totalMigrated += uint64(numMigrated)
			log.Infof("Migrated %d entries (%d total)", numMigrated,
				totalMigrated)
		}
		return isFullyDone, err
	}

	// Migrate all entries in batches for the reasons mentioned above.
	if err := batchedUpdate(ctx, b.db, doBatch); err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done migrating spend journal.  Total entries: %d in %v",
		totalMigrated, elapsed)

	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	// Drop temp bucket.
	log.Info("Removing temp data...")
	start = time.Now()
	err = incrementalFlatDrop(ctx, b.db, tmpTxInfoBucketName, "temp data")
	if err != nil {
		return err
	}
	elapsed = time.Since(start).Round(time.Millisecond)
	log.Infof("Done removing temp data in %v", elapsed)

	// Drop version 2 spend journal.
	log.Info("Removing old spend journal...")
	start = time.Now()
	err = incrementalFlatDrop(ctx, b.db, v2SpendJournalBucketName, "old journal")
	if err != nil {
		return err
	}
	elapsed = time.Since(start).Round(time.Millisecond)
	log.Infof("Done removing old spend journal in %v", elapsed)
	return nil
}

// upgradeSpendJournalToVersion3 upgrades a version 2 spend journal to version
// 3.
func upgradeSpendJournalToVersion3(ctx context.Context, b *BlockChain) error {
	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	log.Info("Upgrading spend journal to version 3...")
	start := time.Now()

	// Migrate the spend journal to version 3.
	err := migrateSpendJournalVersion2To3(ctx, b)
	if err != nil {
		return err
	}

	// Update and persist the spend journal database version.
	err = b.db.Update(func(dbTx database.Tx) error {
		b.dbInfo.stxoVer = 3
		return dbPutDatabaseInfo(dbTx, b.dbInfo)
	})
	if err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done upgrading spend journal in %v.", elapsed)
	return nil
}

// upgradeUtxoSetToVersion3 upgrades a version 2 utxo set to version 3.
func upgradeUtxoSetToVersion3(ctx context.Context, db database.DB,
	utxoBackend UtxoBackend) error {

	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	log.Info("Upgrading utxo set to version 3...")
	start := time.Now()

	// Migrate the utxoset to version 3.
	err := migrateUtxoSetVersion2To3(ctx, db)
	if err != nil {
		return err
	}

	// Fetch the backend versioning info.
	utxoDbInfo, err := utxoBackend.FetchInfo()
	if err != nil {
		return err
	}

	// Update and persist the UTXO set database version.
	utxoDbInfo.utxoVer = 3
	err = utxoBackend.PutInfo(utxoDbInfo)
	if err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done upgrading utxo set in %v.", elapsed)
	return nil
}

// upgradeToVersion10 upgrades a version 8 or version 9 blockchain database to
// version 10.  This entails writing the database spend journal version to the
// database so that it can be decoupled from the overall version of the block
// database.
func upgradeToVersion10(ctx context.Context, db database.DB, dbInfo *databaseInfo) error {
	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	log.Info("Upgrading database to version 10...")
	start := time.Now()

	// Update and persist the database spend journal version and overall block
	// database version.
	err := db.Update(func(dbTx database.Tx) error {
		dbInfo.stxoVer = 2
		if dbInfo.version == 9 {
			dbInfo.stxoVer = 3
		}

		dbInfo.version = 10
		return dbPutDatabaseInfo(dbTx, dbInfo)
	})
	if err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done upgrading database in %v.", elapsed)
	return nil
}

// upgradeToVersion11 upgrades a version 10 blockchain database to version 11.
// This entails bumping the overall block database version to 11 to prevent
// downgrades as the UTXO database is being moved in this same set of changes.
func upgradeToVersion11(ctx context.Context, db database.DB, dbInfo *databaseInfo) error {
	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	log.Info("Upgrading database to version 11...")
	start := time.Now()

	// Update and persist the overall block database version.
	err := db.Update(func(dbTx database.Tx) error {
		dbInfo.version = 11
		return dbPutDatabaseInfo(dbTx, dbInfo)
	})
	if err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done upgrading database in %v.", elapsed)
	return nil
}

// -----------------------------------------------------------------------------
// The version 1 stake database stores information about the state of tickets,
// potentially along with other information, using the shared format described
// here in several different places such as the stake undo data and the various
// category-based sets of tickets.
//
// The version 1 serialized format of the ticket state information is:
//
//   <block height><flags>
//
//   Field             Type             Size
//   block height      uint32           4 bytes
//   flags             byte             1 byte
//
// The version 1 serialized flags format is:
//   bit  0   - ticket is missed
//   bit  1   - ticket is revoked
//   bit  2   - ticket is spent
//   bit  3   - ticket is expired
//   bits 4-7 - unused
// -----------------------------------------------------------------------------

// Hardcoded constants so updates do not affect old upgrades.
const (
	// Version 1 ticket status flags.
	ticketMissedFlagV1  = 1 << 0
	ticketRevokedFlagV1 = 1 << 1
	ticketSpentFlagV1   = 1 << 2
	ticketExpiredFlagV1 = 1 << 3

	// ticketInfoSerializeSizeV1 is the number of bytes required to serialize
	// ticket state information in the version 1 format described above.
	ticketInfoSerializeSizeV1 = 5
)

// ticketInfoV1 represents the state of a ticket as used in version 1 stake
// database entries.
type ticketInfoV1 struct {
	hash    chainhash.Hash
	height  uint32
	missed  bool
	revoked bool
	spent   bool
	expired bool
}

// encodeTicketInfoFlagsV1 encodes the state flags of the provided ticket
// according to the version 1 format described above.
func encodeTicketInfoFlagsV1(ticket *ticketInfoV1) byte {
	var encodedFlags byte
	if ticket.missed {
		encodedFlags |= ticketMissedFlagV1
	}
	if ticket.revoked {
		encodedFlags |= ticketRevokedFlagV1
	}
	if ticket.spent {
		encodedFlags |= ticketSpentFlagV1
	}
	if ticket.expired {
		encodedFlags |= ticketExpiredFlagV1
	}
	return encodedFlags
}

// putTicketInfoV1 serializes the passed ticket state info according to the
// version 1 format described above directly into the passed target byte slice.
// The target byte slice must be at least large enough to handle the number of
// bytes specified by ticketInfoSerializeSizeV1 or it will panic.
func putTicketInfoV1(target []byte, ticket *ticketInfoV1) {
	binary.LittleEndian.PutUint32(target[0:4], ticket.height)
	target[4] = encodeTicketInfoFlagsV1(ticket)
}

// decodeTicketInfoV1 decodes the passed serialized ticket state info into the
// passed struct according to the version 1 format described above.  It returns
// the number of bytes read.
func decodeTicketInfoV1(serialized []byte, ticket *ticketInfoV1) (int, error) {
	// Ensure there are enough bytes to decode the ticket info.
	if len(serialized) < 4 {
		return 0, errDeserialize("unexpected end of data while reading ticket " +
			"info")
	}

	// Deserialize the block height associated with the ticket.
	height := binary.LittleEndian.Uint32(serialized[0:4])
	offset := 4

	// Deserialize the encoded flags.
	if offset+1 > len(serialized) {
		return offset, errDeserialize("unexpected end of data while reading " +
			"flags")
	}
	encodedFlags := serialized[offset]
	offset++

	ticket.height = height
	ticket.missed = encodedFlags&ticketMissedFlagV1 != 0
	ticket.revoked = encodedFlags&ticketRevokedFlagV1 != 0
	ticket.spent = encodedFlags&ticketSpentFlagV1 != 0
	ticket.expired = encodedFlags&ticketExpiredFlagV1 != 0
	return offset, nil
}

// -----------------------------------------------------------------------------
// The version 1 stake database undo data consists of an entry for each main
// chain block that contains data for all tickets modified by the block along
// with their updated state info in the order that they were spent, meaning that
// each element needs to be applied in reverse order to undo the effects of the
// block.
//
// The version 1 serialized format is:
//
//   [<ticket hash><ticket info>,...]
//
//   Field            Type              Size
//   ticket hash      chainhash.Hash    chainhash.HashSize
//   ticket info      ticketInfoV1      5 bytes
// -----------------------------------------------------------------------------

// v1UndoElementSize is the size of each individual element in a version 1 stake
// database undo record.
const v1UndoElementSize = chainhash.HashSize + ticketInfoSerializeSizeV1

// deserializeTicketDBUndoEntryV1 deserializes stake undo data from the passed
// serialized byte slice according to the version 1 format described in detail
// above.
func deserializeTicketDBUndoEntryV1(serialized []byte, blockHeight uint32) ([]ticketInfoV1, error) {
	// Ensure the undo data is a multiple of the entry size to detect corruption
	// and avoid the need to check additional size constraints while decoding.
	if len(serialized)%v1UndoElementSize != 0 {
		str := fmt.Sprintf("corrupt undo data for height %d", blockHeight)
		return nil, errDeserialize(str)
	}

	numEntries := len(serialized) / v1UndoElementSize
	undoEntries := make([]ticketInfoV1, 0, numEntries)
	offset := 0
	for i := 0; i < numEntries; i++ {
		var ticket ticketInfoV1
		copy(ticket.hash[:], serialized[offset:offset+chainhash.HashSize])
		offset += chainhash.HashSize
		bytesRead, err := decodeTicketInfoV1(serialized[offset:], &ticket)
		if err != nil {
			return nil, err
		}
		offset += bytesRead

		undoEntries = append(undoEntries, ticket)
	}

	return undoEntries, nil
}

// serializeTicketDBUndoEntryV1 serializes the passed undo data into a single
// byte slice according to the version 1 format described in detail above.
func serializeTicketDBUndoEntryV1(undoEntries []ticketInfoV1) []byte {
	serialized := make([]byte, len(undoEntries)*v1UndoElementSize)
	offset := 0
	for _, entry := range undoEntries {
		copy(serialized[offset:], entry.hash[:])
		offset += chainhash.HashSize
		putTicketInfoV1(serialized[offset:], &entry)
		offset += ticketInfoSerializeSizeV1
	}
	return serialized
}

// correctTreasurySpendVoteData corrects block index and ticket database entries
// to account for an issue in legacy code that caused votes which also include
// treasury spend vote data to be incorrectly marked as missed.
func correctTreasurySpendVoteData(ctx context.Context, db database.DB, params *chaincfg.Params) error {
	log.Info("Reindexing ticket database.  This will take a while...")
	start := time.Now()

	// Hardcoded data so updates do not affect old upgrades.
	const hashSize = chainhash.HashSize
	v1ChainStateKeyName := []byte("chainstate")
	v3BlockIdxBucketName := []byte("blockidxv3")
	v1StakeChainStateKeyName := []byte("stakechainstate")
	v1LiveTicketsBucketName := []byte("livetickets")
	v1MissedTicketsBucketName := []byte("missedtickets")
	v1RevokedTicketsBucketName := []byte("revokedtickets")
	v1StakeBlockUndoBucketName := []byte("stakeblockundo")
	v1TicketsInBlockBucketName := []byte("ticketsinblock")

	// uint32Bytes is a helper function to convert a uint32 to a little endian
	// byte slice.
	byteOrder := binary.LittleEndian
	uint32Bytes := func(ui32 uint32) []byte {
		var b [4]byte
		byteOrder.PutUint32(b[:], ui32)
		return b[:]
	}

	// Determine the current best chain tip using the version 1 chain state.
	var bestTipHash chainhash.Hash
	var bestTipHeight uint32
	err := db.View(func(dbTx database.Tx) error {
		// Load the current best chain tip hash and height from the v1 chain
		// state.
		//
		// The serialized format of the v1 chain state is roughly:
		//
		//   <block hash><block height><rest of data>
		//
		//   Field             Type             Size
		//   block hash        chainhash.Hash   hashSize
		//   block height      uint32           4 bytes
		//   rest of data...
		meta := dbTx.Metadata()
		serializedChainState := meta.Get(v1ChainStateKeyName)
		if serializedChainState == nil {
			str := fmt.Sprintf("chain state with key %s does not exist",
				v1ChainStateKeyName)
			return errDeserialize(str)
		}
		if len(serializedChainState) < hashSize+4 {
			str := "version 1 chain state is malformed"
			return errDeserialize(str)
		}
		copy(bestTipHash[:], serializedChainState[0:hashSize])
		offset := hashSize
		bestTipHeight = byteOrder.Uint32(serializedChainState[offset : offset+4])
		return nil
	})
	if err != nil {
		return err
	}

	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	// Determine the minimum bound to disconnect back to in order to undo the
	// incorrect information so it can then be corrected.
	//
	// The only entries that might be incorrect are those after the treasury
	// agenda activated, so a lot of work can be avoided by only considering
	// those entries.  However, determining if the treasury agenda is active and
	// the point at which it activated requires a bunch of data that is not
	// readily available here in the upgrade code, so hard code the data for the
	// main and test networks to significantly optimize those cases and just
	// redo everything back to the genesis block for other networks.  While this
	// does potentially do more work than is strictly necessary for those other
	// networks, it is quite rare for them to have old databases in practice, so
	// it is a reasonable tradeoff.
	//
	// Finally, note that the heights here are for the block _before_ the
	// treasury activated on their respective networks since the block in which
	// it activated also needs to be undone.
	var disconnectUntilHeight uint32
	switch params.Net {
	case wire.MainNet:
		disconnectUntilHeight = 552447
	case wire.TestNet3:
		disconnectUntilHeight = 560207
	}

	// There is nothing to do if the best chain tip is prior to the point after
	// where potentially incorrect entries exist.
	if bestTipHeight < disconnectUntilHeight {
		return nil
	}

	// blockTreeEntry represents a version 3 block index entry with the details
	// needed to be able to determine which blocks need to have the treasury
	// spend vote data corrected as well as which ones comprise the main chain.
	type blockTreeEntry struct {
		parent    *blockTreeEntry
		children  []*blockTreeEntry
		hash      chainhash.Hash
		height    uint32
		mainChain bool
		header    wire.BlockHeader
		status    byte
	}

	// Load the block tree to determine all blocks and ticket database entries
	// that need to be corrected as well as the required order using the version
	// 3 block index along with the information determined above.
	var bestTip *blockTreeEntry
	blockTree := make(map[chainhash.Hash]*blockTreeEntry)
	err = db.View(func(dbTx database.Tx) error {
		// Hardcoded data so updates do not affect old upgrades.
		const blockHdrSize = 180

		// Construct a full block tree from the version 2 block index by mapping
		// each block to its parent block.
		var lastEntry, parent *blockTreeEntry
		meta := dbTx.Metadata()
		v3BlockIdxBucket := meta.Bucket(v3BlockIdxBucketName)
		if v3BlockIdxBucket == nil {
			return fmt.Errorf("bucket %s does not exist", v3BlockIdxBucketName)
		}
		cursor := v3BlockIdxBucket.Cursor()
		for ok := cursor.First(); ok; ok = cursor.Next() {
			if interruptRequested(ctx) {
				return errInterruptRequested
			}

			// Deserialize the header from the version 3 block index entry.
			//
			// The serialized value format of a v3 block index entry is roughly:
			//
			//   <block header><status><rest of data>
			//
			//   Field              Type                Size
			//   block header       wire.BlockHeader    180 bytes
			//   status             byte                1 byte
			//   rest of data...
			serializedBlkIdxEntry := cursor.Value()
			if len(serializedBlkIdxEntry) < blockHdrSize {
				return errDeserialize("unexpected end of data while reading " +
					"block header")
			}
			hB := serializedBlkIdxEntry[0:blockHdrSize]
			var header wire.BlockHeader
			if err := header.Deserialize(bytes.NewReader(hB)); err != nil {
				return err
			}
			if blockHdrSize+1 > len(serializedBlkIdxEntry) {
				return errDeserialize("unexpected end of data while reading " +
					"status")
			}
			status := serializedBlkIdxEntry[blockHdrSize]

			// Stop loading if the best chain tip height is exceeded.  This
			// can happen when there are block index entries for known headers
			// that haven't had their associated block data fully validated and
			// connected yet and therefore are not relevant for this upgrade
			// code.
			blockHeight := header.Height
			if blockHeight > bestTipHeight {
				break
			}

			// Determine the parent block node.  Since the entries are iterated
			// in order of height, there is a very good chance the previous
			// one processed is the parent.
			blockHash := header.BlockHash()
			if lastEntry == nil {
				if blockHash != params.GenesisHash {
					str := fmt.Sprintf("correctTreasurySpendVoteData: expected "+
						"first entry to be genesis block, found %s", blockHash)
					return errDeserialize(str)
				}
			} else if header.PrevBlock == lastEntry.hash {
				parent = lastEntry
			} else {
				parent = blockTree[header.PrevBlock]
				if parent == nil {
					str := fmt.Sprintf("correctTreasurySpendVoteData: could "+
						"not find parent for block %s", blockHash)
					return errDeserialize(str)
				}
			}

			// Add the block to the block tree.
			treeEntry := &blockTreeEntry{
				parent: parent,
				hash:   blockHash,
				height: blockHeight,
				header: header,
				status: status,
			}
			blockTree[blockHash] = treeEntry
			if parent != nil {
				parent.children = append(parent.children, treeEntry)
			}

			lastEntry = treeEntry
		}

		// Determine the blocks that comprise the main chain by starting at the
		// best tip and walking backwards to the oldest tracked block.
		bestTip = blockTree[bestTipHash]
		if bestTip == nil {
			str := fmt.Sprintf("chain tip %s is not in block index", bestTipHash)
			return errDeserialize(str)
		}
		for entry := bestTip; entry != nil; entry = entry.parent {
			entry.mainChain = true
		}
		return nil
	})
	if err != nil {
		return err
	}

	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	// Ensure all of the buckets used repeatedly in the remaining code below
	// exist now to avoid the need to check multiple times.
	err = db.View(func(dbTx database.Tx) error {
		meta := dbTx.Metadata()
		bucketNames := [][]byte{v1StakeBlockUndoBucketName,
			v1LiveTicketsBucketName, v1MissedTicketsBucketName,
			v1RevokedTicketsBucketName, v1TicketsInBlockBucketName,
		}
		for _, bucketName := range bucketNames {
			if bucket := meta.Bucket(bucketName); bucket == nil {
				return fmt.Errorf("bucket %s does not exist", bucketName)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// ticketDBInfo represents a version 1 ticket database with only the details
	// needed to be able to correct the treasury spend vote data.
	type ticketDBInfo struct {
		tip            *blockTreeEntry
		liveTickets    map[chainhash.Hash]ticketInfoV1
		missedTickets  map[chainhash.Hash]ticketInfoV1
		revokedTickets map[chainhash.Hash]ticketInfoV1
	}

	// calcNextWinners returns a slice of tickets that are eligible to vote in
	// the next block given the current state of the provided ticket database.
	// It will return nil when there are not any winners for the next block due
	// to it being prior to the point where voting starts.
	calcNextWinners := func(ticketDB *ticketDBInfo) ([]chainhash.Hash, error) {
		// Voting does not start until stake validation height, so there are not
		// any winners prior to that point.
		if ticketDB.tip.height < uint32(params.StakeValidationHeight-1) {
			return nil, nil
		}

		// Assert there are enough live tickets to produce the required number
		// of winning tickets.
		numLiveTickets := uint32(len(ticketDB.liveTickets))
		if numLiveTickets < uint32(params.TicketsPerBlock) {
			return nil, AssertError(fmt.Sprintf("the live ticket pool as of "+
				"block %s (height %d) only has %d tickets which is less than "+
				"the required minimum to choose %d winners", ticketDB.tip.hash,
				ticketDB.tip.height, numLiveTickets, params.TicketsPerBlock))
		}

		// Sort the live tickets.
		liveTickets := make([]chainhash.Hash, 0, numLiveTickets)
		for ticketHash := range ticketDB.liveTickets {
			liveTickets = append(liveTickets, ticketHash)
		}
		sort.Slice(liveTickets, func(i, j int) bool {
			return bytes.Compare(liveTickets[i][:], liveTickets[j][:]) < 0
		})

		// Determine the winning tickets.
		nextWinners := make([]chainhash.Hash, 0, params.TicketsPerBlock)
		hB, _ := ticketDB.tip.header.Bytes()
		prng := stake.NewHash256PRNG(hB)
		usedOffsets := make(map[uint32]struct{})
		for uint16(len(nextWinners)) < params.TicketsPerBlock {
			ticketIndex := prng.UniformRandom(numLiveTickets)
			if _, exists := usedOffsets[ticketIndex]; !exists {
				usedOffsets[ticketIndex] = struct{}{}
				nextWinners = append(nextWinners, liveTickets[ticketIndex])
			}
		}

		return nextWinners, nil
	}

	// serializeStakeChainStateV1 serializes the stake chain state for the
	// passed ticket database into a single byte slice according to the version
	// 1 format described below.
	//
	// The version 1 stake chain state consists of the best block hash and
	// height, the total number of live, missed, and revoked tickets, the number
	// of winning tickets per block, and the tickets that are eligible to vote
	// in the next block (aka winning tickets).
	//
	// The serialized format is:
	//
	//   <block hash><block height><live><missed><revoked><per block><winners>
	//
	//   Field              Type              Size
	//   block hash         chainhash.Hash    hashSize
	//   block height       uint32            4 bytes
	//   live tickets       uint32            4 bytes
	//   missed tickets     uint64            8 bytes
	//   revoked tickets    uint64            8 bytes
	//   winners per block  uint16            2 bytes
	//   next winners       []chainhash.Hash  hashSize * winners per block
	//
	// NOTE: The next winners are populated with zero hashes prior to stake
	// validation height.
	serializeStakeChainStateV1 := func(ticketDB *ticketDBInfo) ([]byte, error) {
		numLiveTickets := uint32(len(ticketDB.liveTickets))
		numMissedTickets := uint64(len(ticketDB.missedTickets))
		numRevokedTickets := uint64(len(ticketDB.revokedTickets))

		const baseSize = hashSize + 4 + 4 + 8 + 8 + 2
		serialized := make([]byte, baseSize+params.TicketsPerBlock*hashSize)
		copy(serialized[0:hashSize], ticketDB.tip.hash[:])
		offset := hashSize
		byteOrder.PutUint32(serialized[offset:offset+4], ticketDB.tip.height)
		offset += 4
		byteOrder.PutUint32(serialized[offset:offset+4], numLiveTickets)
		offset += 4
		byteOrder.PutUint64(serialized[offset:offset+8], numMissedTickets)
		offset += 8
		byteOrder.PutUint64(serialized[offset:offset+8], numRevokedTickets)
		offset += 8
		byteOrder.PutUint16(serialized[offset:offset+2], params.TicketsPerBlock)
		offset += 2

		// Determine and serialize the next winning tickets, if any.  There will
		// not be any winners prior to the point where voting starts (stake
		// validation height).  In that case, since the slice is already sized
		// to include them, the hashes will be all zero as expected.
		nextWinners, err := calcNextWinners(ticketDB)
		if err != nil {
			return nil, err
		}
		for _, winner := range nextWinners {
			copy(serialized[offset:offset+hashSize], winner[:])
			offset += hashSize
		}

		return serialized, nil
	}

	// Load information about the current state of the version 1 ticket database
	// including the current tip and all of the associated live, missed, and
	// revoked tickets.
	ticketDB := ticketDBInfo{
		liveTickets:    make(map[chainhash.Hash]ticketInfoV1),
		missedTickets:  make(map[chainhash.Hash]ticketInfoV1),
		revokedTickets: make(map[chainhash.Hash]ticketInfoV1),
	}
	err = db.View(func(dbTx database.Tx) error {
		// Load the current ticket db chain tip hash and height from the v1
		// stake chain state.
		//
		// The serialized format of the v1 stake chain state is roughly:
		//
		//   <block hash><block height><rest of data>
		//
		//   Field             Type             Size
		//   block hash        chainhash.Hash   hashSize
		//   block height      uint32           4 bytes
		//   rest of data...
		var ticketDBTipHash chainhash.Hash
		var ticketDBTipHeight uint32
		meta := dbTx.Metadata()
		serializedState := meta.Get(v1StakeChainStateKeyName)
		if serializedState == nil {
			str := fmt.Sprintf("chain state with key %s does not exist",
				v1StakeChainStateKeyName)
			return errDeserialize(str)
		}
		if len(serializedState) < hashSize+4 {
			str := "version 1 stake chain state is malformed"
			return errDeserialize(str)
		}
		copy(ticketDBTipHash[:], serializedState[0:hashSize])
		offset := hashSize
		ticketDBTipHeight = byteOrder.Uint32(serializedState[offset : offset+4])

		// Look up ticket database tip in the block index and assert it exists.
		tip, ok := blockTree[ticketDBTipHash]
		if !ok {
			str := fmt.Sprintf("ticket db tip %s is not in block index",
				ticketDBTipHash)
			return errDeserialize(str)
		}
		if tip.height != ticketDBTipHeight {
			str := fmt.Sprintf("corrupt ticket db tip height for %s",
				ticketDBTipHash)
			return errDeserialize(str)
		}
		ticketDB.tip = tip

		// Load all of the live, missed, and revoked tickets.
		//
		// The version 1 stake database stores each of these sets in their own
		// bucket with an entry for every ticket keyed by its hash and the value
		// serialized according to the shared version 1 stake database ticket
		// state.
		loadTickets := func(key []byte, m map[chainhash.Hash]ticketInfoV1) error {
			bucket := meta.Bucket(key)
			return bucket.ForEach(func(k []byte, v []byte) error {
				var ticket ticketInfoV1
				copy(ticket.hash[:], k)
				_, err := decodeTicketInfoV1(v, &ticket)
				if err != nil {
					return err
				}
				m[ticket.hash] = ticket
				return nil
			})
		}
		err := loadTickets(v1LiveTicketsBucketName, ticketDB.liveTickets)
		if err != nil {
			return err
		}
		err = loadTickets(v1MissedTicketsBucketName, ticketDB.missedTickets)
		if err != nil {
			return err
		}
		return loadTickets(v1RevokedTicketsBucketName, ticketDB.revokedTickets)
	})
	if err != nil {
		return err
	}

	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	// Determine the known good block to disconnect back to by following the
	// ticket database tip back to the known good height.  This approach ensures
	// that any corner cases, such as the local best chain being on a side chain
	// for which the treasury agenda may or may not be active, are handled
	// properly at the expense of potentially doing more work than is strictly
	// necessary in those highly unlikely cases.
	disconnectTo := ticketDB.tip
	for disconnectTo.height != disconnectUntilHeight {
		disconnectTo = disconnectTo.parent
	}

	// dbPutTicketV1 writes the provided ticket info into the provided bucket
	// keyed by the ticket hash.
	dbPutTicketV1 := func(bucket database.Bucket, ticket ticketInfoV1) error {
		var serialized [5]byte
		putTicketInfoV1(serialized[:], &ticket)
		return bucket.Put(ticket.hash[:], serialized[:])
	}

	// doDisconnectBatch contains the logic for reverting the ticket database
	// back to a known good point by undoing all blocks in between in batches.
	// This is done because attempting to undo them all in a single database
	// transaction could result in massive memory usage and could potentially
	// crash on many systems due to ulimits.
	//
	// Note that the current ticket database tip is updated as the batches
	// either are interrupted or successfully complete so interrupted upgrades
	// start from the most recent tip.
	//
	// It returns whether or not all entries have been updated.
	const maxDisconnectEntries = 10000
	var totalUpdated uint64
	doDisconnectBatch := func(dbTx database.Tx) (bool, error) {
		meta := dbTx.Metadata()
		v1UndoDataBucket := meta.Bucket(v1StakeBlockUndoBucketName)
		v1LiveTicketsBucket := meta.Bucket(v1LiveTicketsBucketName)
		v1MissedTicketsBucket := meta.Bucket(v1MissedTicketsBucketName)
		v1RevokedTicketsBucket := meta.Bucket(v1RevokedTicketsBucketName)

		// loadUndoV1 loads the undo data for the given height from the version
		// 1 stake database, deserializes the data, and returns it.
		loadUndoV1 := func(height uint32) ([]ticketInfoV1, error) {
			heightAsKey := uint32Bytes(height)
			undoData := v1UndoDataBucket.Get(heightAsKey)
			if undoData == nil {
				str := fmt.Sprintf("missing undo data for height %d", height)
				return nil, errDeserialize(str)
			}

			return deserializeTicketDBUndoEntryV1(undoData, height)
		}

		var logProgress bool
		var numUpdated uint64
		err := func() error {
			// Loop backwards from the current ticket database tip to the known
			// good point.
			var batchErr error
			for ticketDB.tip != disconnectTo {
				if interruptRequested(ctx) {
					logProgress = true
					batchErr = errInterruptRequested
					break
				}

				if numUpdated >= maxDisconnectEntries {
					logProgress = true
					batchErr = errBatchFinished
					break
				}

				// Load all the undo entries for the current ticket database tip
				// and apply all of them in reverse order.
				undoEntries, err := loadUndoV1(ticketDB.tip.height)
				if err != nil {
					return err
				}
				for undoIdx := len(undoEntries) - 1; undoIdx >= 0; undoIdx-- {
					undo := &undoEntries[undoIdx]

					switch {
					// Remove tickets that became live in the block from the set
					// of live tickets.
					case !undo.missed && !undo.revoked && !undo.spent:
						delete(ticketDB.liveTickets, undo.hash)
						err := v1LiveTicketsBucket.Delete(undo.hash[:])
						if err != nil {
							return err
						}

					// Move tickets revoked in the block back to the set of
					// missed tickets.
					case undo.missed && undo.revoked:
						delete(ticketDB.revokedTickets, undo.hash)
						err := v1RevokedTicketsBucket.Delete(undo.hash[:])
						if err != nil {
							return err
						}

						ticket := ticketInfoV1{
							hash:    undo.hash,
							height:  undo.height,
							missed:  true,
							revoked: false,
							spent:   false,
							expired: undo.expired,
						}
						ticketDB.missedTickets[undo.hash] = ticket
						err = dbPutTicketV1(v1MissedTicketsBucket, ticket)
						if err != nil {
							return err
						}

					// Move tickets that became missed as of the block back to
					// the set of live tickets.
					case undo.missed && !undo.revoked:
						delete(ticketDB.missedTickets, undo.hash)
						err := v1MissedTicketsBucket.Delete(undo.hash[:])
						if err != nil {
							return err
						}

						ticket := ticketInfoV1{
							hash:    undo.hash,
							height:  undo.height,
							missed:  false,
							revoked: false,
							spent:   false,
							expired: false,
						}
						ticketDB.liveTickets[undo.hash] = ticket
						err = dbPutTicketV1(v1LiveTicketsBucket, ticket)
						if err != nil {
							return err
						}

					// Move tickets spent in the block back to the set of live
					// tickets.
					case undo.spent:
						ticket := ticketInfoV1{
							hash:    undo.hash,
							height:  undo.height,
							missed:  false,
							revoked: false,
							spent:   false,
							expired: false,
						}
						ticketDB.liveTickets[undo.hash] = ticket
						err := dbPutTicketV1(v1LiveTicketsBucket, ticket)
						if err != nil {
							return err
						}

					default:
						str := fmt.Sprintf("unknown ticket state in undo data "+
							"(missed=%v, revoked=%v, spent=%v, expired=%v)",
							undo.missed, undo.revoked, undo.spent, undo.expired)
						return errDeserialize(str)
					}
				}

				// NOTE: The undo data for the block is not dropped here to save
				// additional database writes because it will be overwritten
				// with the correct data later when the block is reconnected.
				//
				// Also, the list of tickets maturing in the block is correct,
				// so it is kept and used when the block is reconnected later to
				// avoid the otherwise much more expensive need to load the
				// historical block to determine them.

				// The ticket database tip is now the previous block.
				ticketDB.tip = ticketDB.tip.parent

				numUpdated++
			}

			// Write the new stake chain state to the database so it will resume
			// at the correct place if interrupted.
			serialized, err := serializeStakeChainStateV1(&ticketDB)
			if err != nil {
				return err
			}
			err = meta.Put(v1StakeChainStateKeyName, serialized)
			if err != nil {
				return err
			}

			return batchErr
		}()
		isFullyDone := err == nil
		if (isFullyDone || logProgress) && numUpdated > 0 {
			totalUpdated += numUpdated
			log.Infof("Updated %d entries (%d total)", numUpdated, totalUpdated)
		}
		return isFullyDone, err
	}

	// Revert the ticket database back to a known good point by undoing all
	// blocks from the current ticket database back to that point in batches.
	//
	// Note that the disconnect is done in an upgrade stage so that interrupting
	// the upgrade after the disconnect process successfully completes will not
	// attempt to undo again.
	disconnectDoneKey := []byte("tsvfdisconnectdone")
	err = runUpgradeStageOnce(ctx, db, disconnectDoneKey, func() error {
		// Update all entries in batches for the reasons mentioned above.
		return batchedUpdate(ctx, db, doDisconnectBatch)
	})
	if err != nil {
		return err
	}

	// dbFetchBlock uses an existing database transaction to retrieve the block
	// for the provided block tree entry.
	dbFetchBlock := func(dbTx database.Tx, entry *blockTreeEntry) (*wire.MsgBlock, error) {
		// Load the raw block bytes from the database.
		blockBytes, err := dbTx.FetchBlock(&entry.hash)
		if err != nil {
			return nil, err
		}

		var block wire.MsgBlock
		if err := block.FromBytes(blockBytes); err != nil {
			return nil, err
		}

		return &block, nil
	}

	// findSpentTicketsInBlock returns information about tickets spent in the
	// provided block including voted and revoked tickets as well as the vote
	// bits of each vote.
	type spentTicketsInBlock struct {
		votedTickets   []chainhash.Hash
		votes          []blockIndexVoteVersionTuple
		revokedTickets []chainhash.Hash
	}
	findSpentTicketsInBlock := func(block *wire.MsgBlock) spentTicketsInBlock {
		votes := make([]blockIndexVoteVersionTuple, 0, block.Header.Voters)
		voters := make([]chainhash.Hash, 0, block.Header.Voters)
		revocations := make([]chainhash.Hash, 0, block.Header.Revocations)

		for _, stx := range block.STransactions {
			// It is safe to use the version as a proxy for treasury activation
			// here since the format of version 3 votes was invalid prior to its
			// activation and so even if there were votes with version >= 3
			// prior to that point, those votes still would've had to conform to
			// the rules that existed at that time and therefore could not have
			// simultaneously had the new format and made it into a block.
			// Further, the old format is still treated as a valid vote when the
			// flag is set, so the historical consensus rules would not be
			// violated by incorrectly setting this flag even if there were
			// votes with version >= 3 prior to treasury activation.  Finally,
			// there were no votes with versions >= 3 prior to the treasury
			// activation point on mainnet anyway.
			isTreasuryEnabled := stx.Version >= 3
			if stake.IsSSGen(stx, isTreasuryEnabled) {
				voters = append(voters, stx.TxIn[1].PreviousOutPoint.Hash)
				votes = append(votes, blockIndexVoteVersionTuple{
					version: stake.SSGenVersion(stx),
					bits:    stake.SSGenVoteBits(stx),
				})
				continue
			}

			// The automatic ticket revocations agenda is not eligible to vote
			// prior to this upgrade code running, so it couldn't possibly be
			// active.
			const isAutoRevocationsEnabled = false
			if stake.IsSSRtx(stx, isAutoRevocationsEnabled) {
				spentTicketHash := stx.TxIn[0].PreviousOutPoint.Hash
				revocations = append(revocations, spentTicketHash)
				continue
			}
		}
		return spentTicketsInBlock{
			votedTickets:   voters,
			votes:          votes,
			revokedTickets: revocations,
		}
	}

	// nextMainChainBlock returns the block in the main chain after the provided
	// one.
	nextMainChainBlock := func(n *blockTreeEntry) *blockTreeEntry {
		for _, child := range n.children {
			if child.mainChain {
				return child
			}
		}
		return nil
	}

	// doConnectBatch contains the logic for catching the ticket database back
	// up to the current best chain tip by replaying all blocks in between in
	// batches.  This is done because attempting to undo them all in a single
	// database transaction could result in massive memory usage and could
	// potentially crash on many systems due to ulimits.
	//
	// Note that the current ticket database tip is updated as the batches
	// either are interrupted or successfully complete so interrupted upgrades
	// start from the most recent tip.
	//
	// It returns whether or not all entries have been updated.
	const maxConnectEntries = 1000
	doConnectBatch := func(dbTx database.Tx) (bool, error) {
		meta := dbTx.Metadata()
		v1LiveTicketsBucket := meta.Bucket(v1LiveTicketsBucketName)
		v1MissedTicketsBucket := meta.Bucket(v1MissedTicketsBucketName)
		v1RevokedTicketsBucket := meta.Bucket(v1RevokedTicketsBucketName)
		v1TicketsInBlockBucket := meta.Bucket(v1TicketsInBlockBucketName)
		v1UndoDataBucket := meta.Bucket(v1StakeBlockUndoBucketName)
		v3BlockIdxBucket := meta.Bucket(v3BlockIdxBucketName)

		var logProgress bool
		var numUpdated uint64
		err := func() error {
			// Loop forwards from the current ticket database tip to the current
			// best chain tip.
			var batchErr error
			for ticketDB.tip != bestTip {
				if interruptRequested(ctx) {
					logProgress = true
					batchErr = errInterruptRequested
					break
				}

				if numUpdated >= maxConnectEntries {
					logProgress = true
					batchErr = errBatchFinished
					break
				}

				// Determine the next main chain block, load it from the block
				// database and extract information about the spent tickets in
				// order to construct all of the stake info needed to update the
				// ticket database and block index.
				nextTip := nextMainChainBlock(ticketDB.tip)
				block, err := dbFetchBlock(dbTx, nextTip)
				if err != nil {
					return err
				}
				spentTickets := findSpentTicketsInBlock(block)

				// Load the list of tickets that are maturing in the block that
				// is being connected.
				//
				// The version 1 serialized format of the tickets in the block
				// is:
				//
				//   [<block hash>,...]
				//
				//   Field         Type              Size
				//   block hash    chainhash.Hash    hashSize
				heightAsKey := uint32Bytes(nextTip.height)
				serializedHashes := v1TicketsInBlockBucket.Get(heightAsKey)
				if serializedHashes == nil {
					str := fmt.Sprintf("missing maturing ticket data for "+
						"block height %d", nextTip.height)
					return errDeserialize(str)
				}
				if len(serializedHashes)%hashSize != 0 {
					str := fmt.Sprintf("malformed maturing ticket data for "+
						"block height %d", nextTip.height)
					return errDeserialize(str)
				}
				var newTickets []chainhash.Hash
				numEntries := len(serializedHashes) / hashSize
				if numEntries > 0 {
					newTickets = make([]chainhash.Hash, numEntries)
				}
				offset := 0
				for i := 0; i < numEntries; i++ {
					copy(newTickets[i][:], serializedHashes[offset:])
					offset += hashSize
				}

				// Determine the winning tickets eligible to vote in the block
				// that is being connected and remove them from the set of live
				// tickets, add any that missed to the set of missed tickets,
				// and track the state changes needed for the undo entry.
				//
				// Note that there will not be any winners prior to the point
				// where voting starts (stake validation height).
				nextWinners, err := calcNextWinners(&ticketDB)
				if err != nil {
					return err
				}
				undoEntriesSizeHint := len(nextWinners) + len(newTickets) +
					len(spentTickets.revokedTickets)
				undoEntries := make([]ticketInfoV1, 0, undoEntriesSizeHint)
				for _, winner := range nextWinners {
					ticket, ok := ticketDB.liveTickets[winner]
					if !ok {
						return AssertError(fmt.Sprintf("winning ticket %s "+
							"voting on block %s (height %d) is not in the set "+
							"of live tickets", winner, ticketDB.tip.hash,
							ticketDB.tip.height))
					}
					delete(ticketDB.liveTickets, ticket.hash)
					err := v1LiveTicketsBucket.Delete(ticket.hash[:])
					if err != nil {
						return err
					}

					// The ticket is either spent and not missed when it voted
					// or unspent and missed when it did not.
					var voted bool
					for _, votedTicket := range spentTickets.votedTickets {
						if votedTicket == ticket.hash {
							voted = true
							break
						}
					}
					ticket.missed = !voted
					ticket.revoked = false
					ticket.spent = voted
					ticket.expired = false
					if ticket.missed {
						ticketDB.missedTickets[ticket.hash] = ticket
						err := dbPutTicketV1(v1MissedTicketsBucket, ticket)
						if err != nil {
							return err
						}
					}
					undoEntries = append(undoEntries, ticket)
				}

				// Determine tickets that are expiring as of the block that is
				// being connected, move them from the set of live tickets to
				// the set of missed tickets, and track the state changes needed
				// for the undo entry.
				var expiredTickets []chainhash.Hash
				if nextTip.height >= params.TicketExpiry {
					expiringHeight := nextTip.height - params.TicketExpiry
					for _, ticket := range ticketDB.liveTickets {
						if ticket.height > expiringHeight {
							continue
						}
						expiredTickets = append(expiredTickets, ticket.hash)
					}
				}
				for _, expiredTicket := range expiredTickets {
					ticket := ticketDB.liveTickets[expiredTicket]
					delete(ticketDB.liveTickets, ticket.hash)
					err := v1LiveTicketsBucket.Delete(ticket.hash[:])
					if err != nil {
						return err
					}

					// The ticket is now missed and expired.
					ticket.missed = true
					ticket.revoked = false
					ticket.spent = false
					ticket.expired = true
					ticketDB.missedTickets[ticket.hash] = ticket
					err = dbPutTicketV1(v1MissedTicketsBucket, ticket)
					if err != nil {
						return err
					}
					undoEntries = append(undoEntries, ticket)
				}

				// Move revocations in the block that is being connected from
				// the set of missed tickets to the set of revoked tickets and
				// track the state changes needed for the undo entry.
				for _, revokedTicket := range spentTickets.revokedTickets {
					ticket, ok := ticketDB.missedTickets[revokedTicket]
					if !ok {
						return AssertError(fmt.Sprintf("ticket %s revoked by "+
							"block %s (height %d) is not in the set of missed "+
							"tickets", revokedTicket, nextTip.hash,
							nextTip.height))
					}
					delete(ticketDB.missedTickets, ticket.hash)
					err := v1MissedTicketsBucket.Delete(ticket.hash[:])
					if err != nil {
						return err
					}

					// The ticket is now revoked.  Note that it will already be
					// marked missed, but set it explicitly for clarity.
					ticket.missed = true
					ticket.revoked = true
					ticket.spent = false
					ticket.expired = false
					ticketDB.revokedTickets[ticket.hash] = ticket
					err = dbPutTicketV1(v1RevokedTicketsBucket, ticket)
					if err != nil {
						return err
					}
					undoEntries = append(undoEntries, ticket)
				}

				// Add tickets that are maturing in the block that is being
				// connected to the set of live tickets and track the state
				// changes needed for the undo entry.
				for _, newTicket := range newTickets {
					ticket := ticketInfoV1{
						hash:    newTicket,
						height:  nextTip.height,
						missed:  false,
						revoked: false,
						spent:   false,
						expired: false,
					}
					ticketDB.liveTickets[ticket.hash] = ticket
					err := dbPutTicketV1(v1LiveTicketsBucket, ticket)
					if err != nil {
						return err
					}
					undoEntries = append(undoEntries, ticket)
				}

				// Store the undo data for the block that is being connected.
				serialized := serializeTicketDBUndoEntryV1(undoEntries)
				err = v1UndoDataBucket.Put(heightAsKey, serialized)
				if err != nil {
					return err
				}

				// Correct the vote information for the block being connected as
				// well as any side chain blocks at the same height (siblings).
				for _, tip := range ticketDB.tip.children {
					// The vote data will already be loaded for the main chain
					// block, so make use of it.  Otherwise, load the associated
					// side chain block and extract if the data for the block
					// has been stored.
					votes := spentTickets.votes
					if tip != nextTip {
						// Nothing to do if the block data is not available.
						const v3StatusDataStored = 1 << 0
						if tip.status&v3StatusDataStored == 0 {
							continue
						}

						// Load the block from the database and extract the
						// vote information from it.
						block, err := dbFetchBlock(dbTx, tip)
						if err != nil {
							return err
						}
						spentTickets := findSpentTicketsInBlock(block)
						votes = spentTickets.votes
					}

					// Write the corrected block index entry to the database.
					v3Entry := &blockIndexEntryV3{
						header:   tip.header,
						status:   tip.status,
						voteInfo: votes,
					}
					serialized, err := serializeBlockIndexEntryV3(v3Entry)
					if err != nil {
						return err
					}
					indexKey := blockIndexKeyV3(&tip.hash, tip.height)
					err = v3BlockIdxBucket.Put(indexKey, serialized)
					if err != nil {
						return err
					}
				}

				// The ticket database tip is now the next main chain block.
				ticketDB.tip = nextTip

				numUpdated++
			}

			// Write the new stake chain state to the database.  This ensures it
			// will resume at the correct place if interrupted as well as match
			// the best chain state as required once the entire process is
			// finished.
			serialized, err := serializeStakeChainStateV1(&ticketDB)
			if err != nil {
				return err
			}
			err = meta.Put(v1StakeChainStateKeyName, serialized)
			if err != nil {
				return err
			}

			return batchErr
		}()
		isFullyDone := err == nil
		if (isFullyDone || logProgress) && numUpdated > 0 {
			totalUpdated += numUpdated
			log.Infof("Updated %d entries (%d total)", numUpdated, totalUpdated)
		}
		return isFullyDone, err
	}

	// Catch the ticket database back up to the current best chain tip in
	// batches for the reasons mentioned above.
	if err := batchedUpdate(ctx, db, doConnectBatch); err != nil {
		return err
	}

	// Remove upgrade progress tracking keys.
	err = db.Update(func(dbTx database.Tx) error {
		return dbTx.Metadata().Delete(disconnectDoneKey)
	})
	if err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done updating ticket database in %v.  Total entries: %d",
		elapsed, totalUpdated)
	return nil
}

// upgradeToVersion12 upgrades a version 11 blockchain database to version 12.
// This entails correcting block index and ticket database entries to account
// for an issue in legacy code that caused votes which also include treasury
// spend vote data to be incorrectly marked as missed as well as unmarking all
// blocks previously marked failed so they are eligible for validation again
// under the new consensus rules.
func upgradeToVersion12(ctx context.Context, db database.DB, chainParams *chaincfg.Params, dbInfo *databaseInfo) error {
	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	log.Info("Upgrading database to version 12...")
	start := time.Now()

	// Correct the block index and ticket database entries to account for an
	// issue in legacy code as needed.
	tsvfDoneKeyName := []byte("v12tsvfdone")
	err := runUpgradeStageOnce(ctx, db, tsvfDoneKeyName, func() error {
		return correctTreasurySpendVoteData(ctx, db, chainParams)
	})
	if err != nil {
		return err
	}

	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	// Unmark all blocks previously marked failed so they are eligible for
	// validation again under the new consensus rules.
	if err := clearFailedBlockFlagsV3(ctx, db); err != nil {
		return err
	}

	// Update and persist the overall block database version and remove upgrade
	// progress tracking keys.
	err = db.Update(func(dbTx database.Tx) error {
		err := dbTx.Metadata().Delete(tsvfDoneKeyName)
		if err != nil {
			return err
		}

		dbInfo.version = 12
		return dbPutDatabaseInfo(dbTx, dbInfo)
	})
	if err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done upgrading database in %v.", elapsed)
	return nil
}

// separateUtxoDatabase moves the UTXO set and state from the block database to
// the UTXO database.
func separateUtxoDatabase(ctx context.Context, db database.DB,
	utxoBackend UtxoBackend) error {

	// Key names, versions, and prefixes are hardcoded below so that updates do
	// not affect old upgrades.

	// Legacy buckets and key names.
	v3UtxoSetBucketName := []byte("utxosetv3")
	utxoSetStateKeyNameV1 := []byte("utxosetstate")

	// ---------------------------------------------------------------------------
	// The new keys in the UTXO backend start with a serialized prefix consisting
	// of the key set and version of that key set as follows:
	//
	//   <key set><version>
	//
	//   Key        Value    Size      Description
	//   key set    uint8    1 byte    The key set identifier, as defined below
	//   version    uint8    1 byte    The version of the key set
	//
	// The key sets as of this migration are:
	//   utxoKeySetDbInfo:    1
	//   utxoKeySetUtxoState: 2
	//   utxoKeySetUtxoSet:   3
	//
	// The versions as of this migration are:
	//   utxoKeySetDbInfo:    0
	//   utxoKeySetUtxoState: 1
	//   utxoKeySetUtxoSet:   3
	// ---------------------------------------------------------------------------
	utxoSetStateKeyNew := []byte("\x02\x01utxosetstate")
	utxoPrefixUtxoSetV3 := []byte("\x03\x03")

	log.Info("Migrating UTXO database.  This may take a while...")
	start := time.Now()

	// doBatch contains the primary logic for migrating the UTXO database.  This
	// is done because attempting to migrate in a single database transaction
	// could result in massive memory usage and could potentially crash on many
	// systems due to ulimits.
	//
	// It returns whether or not all entries have been updated.
	const maxEntries = 20000
	var resumeOffset uint32
	var totalMigrated uint64
	var err error
	doBatch := func(dbTx database.Tx, tx UtxoBackendTx) (bool, error) {
		// Get the UTXO set bucket for both the old and the new database.
		v3UtxoSetBucketOldDb := dbTx.Metadata().Bucket(v3UtxoSetBucketName)
		if v3UtxoSetBucketOldDb == nil {
			// If the UTXO set doesn't exist in the old database, return immediately
			// as there is nothing to do.
			return true, nil
		}

		// Migrate UTXO set entries so long as the max number of entries for this
		// batch has not been exceeded.
		var logProgress bool
		var numMigrated, numIterated uint32
		cursor := v3UtxoSetBucketOldDb.Cursor()
		for ok := cursor.Last(); ok; ok = cursor.Prev() {
			// Reset err on each iteration.
			err = nil

			if interruptRequested(ctx) {
				logProgress = true
				err = errInterruptRequested
				break
			}

			if numMigrated >= maxEntries {
				logProgress = true
				err = errBatchFinished
				break
			}

			// Skip entries that have already been migrated in previous batches.
			numIterated++
			if numIterated-1 < resumeOffset {
				continue
			}
			resumeOffset++

			// Create the new entry in the V3 bucket.
			newKey := prefixedKey(utxoPrefixUtxoSetV3, cursor.Key())
			err = tx.Put(newKey, cursor.Value())
			if err != nil {
				return false, err
			}

			numMigrated++
		}
		isFullyDone := err == nil
		if (isFullyDone || logProgress) && numMigrated > 0 {
			totalMigrated += uint64(numMigrated)
			log.Infof("Migrated %d entries (%d total)", numMigrated,
				totalMigrated)
		}
		return isFullyDone, err
	}

	// Migrate all entries in batches for the reasons mentioned above.
	var isFullyDone bool
	for !isFullyDone {
		err := db.View(func(dbTx database.Tx) error {
			return utxoBackend.Update(func(tx UtxoBackendTx) error {
				var err error
				isFullyDone, err = doBatch(dbTx, tx)
				if errors.Is(err, errInterruptRequested) ||
					errors.Is(err, errBatchFinished) {

					// No error here so the database transaction is not cancelled
					// and therefore outstanding work is written to disk.  The outer
					// function will exit with an interrupted error below due to
					// another interrupted check.
					return nil
				}
				return err
			})
		})
		if err != nil {
			return err
		}

		if interruptRequested(ctx) {
			return errInterruptRequested
		}
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done migrating UTXO database.  Total entries: %d in %v",
		totalMigrated, elapsed)

	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	// Get the UTXO set state from the old database.
	var serialized []byte
	err = db.View(func(dbTx database.Tx) error {
		serialized = dbTx.Metadata().Get(utxoSetStateKeyNameV1)
		return nil
	})
	if err != nil {
		return err
	}

	if serialized != nil {
		// Set the UTXO set state in the new database.
		err = utxoBackend.Update(func(tx UtxoBackendTx) error {
			return tx.Put(utxoSetStateKeyNew, serialized)
		})
		if err != nil {
			return err
		}
	}

	if serialized != nil {
		// Delete the UTXO set state from the old database.
		err = db.Update(func(dbTx database.Tx) error {
			return dbTx.Metadata().Delete(utxoSetStateKeyNameV1)
		})
		if err != nil {
			return err
		}
	}

	// Drop UTXO set from the old database.
	log.Info("Removing old UTXO set...")
	start = time.Now()
	err = incrementalFlatDrop(ctx, db, v3UtxoSetBucketName, "old UTXO set")
	if err != nil {
		return err
	}
	err = db.Update(func(dbTx database.Tx) error {
		return dbTx.Metadata().DeleteBucket(v3UtxoSetBucketName)
	})
	if err != nil {
		return err
	}
	elapsed = time.Since(start).Round(time.Millisecond)
	log.Infof("Done removing old UTXO set in %v", elapsed)

	// Force the block database to flush to disk to avoid rerunning the migration
	// in the event of an unclean shutown.
	return db.Flush()
}

// fetchLegacyBucketId returns the legacy bucket id for the provided bucket
// name.  A Get function must be provided to retrieve key/value pairs from the
// underlying data store.
func fetchLegacyBucketId(getFn func(key []byte) ([]byte, error),
	bucketName []byte) ([]byte, error) {

	// bucketIndexPrefix is the prefix used for all entries in the bucket index.
	bucketIndexPrefix := []byte("bidx")

	// parentBucketId is the parent bucket id.  This is hardcoded to zero here
	// since none of the legacy UTXO database buckets had parent buckets.
	parentBucketId := []byte{0x00, 0x00, 0x00, 0x00}

	// Construct the key and fetch the corresponding bucket id from the database.
	// The serialized bucket index key format is:
	//   <bucketindexprefix><parentbucketid><bucketname>
	bucketIdKey := prefixedKey(bucketIndexPrefix, parentBucketId)
	bucketIdKey = append(bucketIdKey, bucketName...)
	bucketId, err := getFn(bucketIdKey)
	if err != nil {
		str := fmt.Sprintf("error fetching legacy bucket id for %v bucket",
			string(bucketName))
		return nil, convertLdbErr(err, str)
	}

	return bucketId, nil
}

// migrateUtxoDbBuckets migrates the UTXO data to use simple key prefixes rather
// than buckets.
func migrateUtxoDbBuckets(ctx context.Context, utxoBackend UtxoBackend) error {
	// Key names, versions, and prefixes are hardcoded below so that updates do
	// not affect old upgrades.

	// Legacy buckets and key names.
	metadataLegacyBucketID := []byte{0x00, 0x00, 0x00, 0x00}
	utxoSetLegacyBucketName := []byte("utxosetv3")
	utxoDbInfoLegacyBucketName := []byte("dbinfo")
	utxoDbInfoVersionKeyNameV1 := []byte("version")
	utxoDbInfoCompVerKeyNameV1 := []byte("compver")
	utxoDbInfoUtxoVerKeyNameV1 := []byte("utxover")
	utxoDbInfoCreatedKeyNameV1 := []byte("created")
	utxoSetStateKeyNameV1 := []byte("utxosetstate")

	// ---------------------------------------------------------------------------
	// The new keys in the UTXO backend start with a serialized prefix consisting
	// of the key set and version of that key set as follows:
	//
	//   <key set><version>
	//
	//   Key        Value    Size      Description
	//   key set    uint8    1 byte    The key set identifier, as defined below
	//   version    uint8    1 byte    The version of the key set
	//
	// The key sets as of this migration are:
	//   utxoKeySetDbInfo:    1
	//   utxoKeySetUtxoState: 2
	//   utxoKeySetUtxoSet:   3
	//
	// The versions as of this migration are:
	//   utxoKeySetDbInfo:    0
	//   utxoKeySetUtxoState: 1
	//   utxoKeySetUtxoSet:   3
	// ---------------------------------------------------------------------------
	utxoDbInfoVersionKeyNew := []byte("\x01\x00version")
	utxoDbInfoCompVerKeyNew := []byte("\x01\x00compver")
	utxoDbInfoUtxoVerKeyNew := []byte("\x01\x00utxover")
	utxoDbInfoCreatedKeyNew := []byte("\x01\x00created")
	utxoSetStateKeyNew := []byte("\x02\x01utxosetstate")
	utxoPrefixUtxoSetV3 := []byte("\x03\x03")

	// moveKey is a helper function that uses an existing UTXO backend transaction
	// to move a key.
	moveKey := func(tx UtxoBackendTx, oldKey, newKey []byte) error {
		serialized, err := tx.Get(oldKey)
		if err != nil {
			return err
		}

		// Return immediately if the entry does not exist at the old location.
		if serialized == nil {
			return nil
		}

		err = tx.Put(newKey, serialized)
		if err != nil {
			return err
		}
		return tx.Delete(oldKey)
	}

	// Move the database info from the legacy bucket to the new keys.
	bucketId, err := fetchLegacyBucketId(utxoBackend.Get,
		utxoDbInfoLegacyBucketName)
	if err != nil {
		return err
	}
	if bucketId != nil {
		err := utxoBackend.Update(func(tx UtxoBackendTx) error {
			// Move the database version from the legacy bucket to the new key.
			oldKey := prefixedKey(bucketId, utxoDbInfoVersionKeyNameV1)
			err = moveKey(tx, oldKey, utxoDbInfoVersionKeyNew)
			if err != nil {
				return fmt.Errorf("error migrating database version: %w", err)
			}

			// Move the database compression version from the legacy bucket to the new
			// key.
			oldKey = prefixedKey(bucketId, utxoDbInfoCompVerKeyNameV1)
			err = moveKey(tx, oldKey, utxoDbInfoCompVerKeyNew)
			if err != nil {
				return fmt.Errorf("error migrating database compression version: %w",
					err)
			}

			// Move the database UTXO set version from the legacy bucket to the new
			// key.
			oldKey = prefixedKey(bucketId, utxoDbInfoUtxoVerKeyNameV1)
			err = moveKey(tx, oldKey, utxoDbInfoUtxoVerKeyNew)
			if err != nil {
				return fmt.Errorf("error migrating UTXO set version: %w", err)
			}

			// Move the database creation date from the legacy bucket to the new key.
			oldKey = prefixedKey(bucketId, utxoDbInfoCreatedKeyNameV1)
			err = moveKey(tx, oldKey, utxoDbInfoCreatedKeyNew)
			if err != nil {
				return fmt.Errorf("error migrating database creation date: %w", err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	// Move the UTXO set state from the legacy bucket to the new key.
	err = utxoBackend.Update(func(tx UtxoBackendTx) error {
		oldKey := prefixedKey(metadataLegacyBucketID, utxoSetStateKeyNameV1)
		err = moveKey(tx, oldKey, utxoSetStateKeyNew)
		if err != nil {
			return fmt.Errorf("error migrating UTXO set state: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.Info("Migrating UTXO database.  This may take a while...")
	start := time.Now()

	// Move the UTXO set from the legacy bucket to the new keys.
	bucketId, err = fetchLegacyBucketId(utxoBackend.Get, utxoSetLegacyBucketName)
	if err != nil {
		return err
	}

	// If the legacy bucket doesn't exist, return as there is nothing to do.
	if bucketId == nil {
		return nil
	}

	// doBatch contains the primary logic for migrating the UTXO set.  This is
	// done because attempting to migrate in a single database transaction could
	// result in massive memory usage and could potentially crash on many systems
	// due to ulimits.
	//
	// It returns whether or not all entries have been updated.
	const maxEntries = 20000
	var totalMigrated uint64
	doBatch := func(tx UtxoBackendTx) (bool, error) {
		// Migrate UTXO set entries so long as the max number of entries for this
		// batch has not been exceeded.
		var logProgress bool
		var numMigrated uint32

		iter := tx.NewIterator(bucketId)
		defer iter.Release()
		for iter.Next() {
			// Reset err on each iteration.
			err = nil

			if interruptRequested(ctx) {
				logProgress = true
				err = errInterruptRequested
				break
			}

			if numMigrated >= maxEntries {
				logProgress = true
				err = errBatchFinished
				break
			}

			// Move the UTXO set entry to the new key.
			oldKey := iter.Key()
			newKey := prefixedKey(utxoPrefixUtxoSetV3, oldKey[len(bucketId):])
			err = moveKey(tx, oldKey, newKey)
			if err != nil {
				return false, fmt.Errorf("error migrating UTXO set entry: %w", err)
			}

			numMigrated++
		}
		if iterErr := iter.Error(); iterErr != nil {
			return false, convertLdbErr(iterErr, iterErr.Error())
		}
		isFullyDone := err == nil
		if (isFullyDone || logProgress) && numMigrated > 0 {
			totalMigrated += uint64(numMigrated)
			log.Infof("Migrated %d entries (%d total)", numMigrated,
				totalMigrated)
		}
		return isFullyDone, err
	}

	// Migrate all entries in batches for the reasons mentioned above.
	if err := utxoBackendBatchedUpdate(ctx, utxoBackend, doBatch); err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done migrating UTXO database.  Total entries: %d in %v",
		totalMigrated, elapsed)

	return nil
}

// dbPutUtxoBackendInfoV1 uses an existing UTXO backend transaction to store the
// V1 UTXO backend information.
func dbPutUtxoBackendInfoV1(tx UtxoBackendTx, info *UtxoBackendInfo) error {
	// V1 database info keys.
	utxoDbInfoVersionKeyNameV1 := []byte("version")
	utxoDbInfoCompVerKeyNameV1 := []byte("compver")
	utxoDbInfoUtxoVerKeyNameV1 := []byte("utxover")
	utxoDbInfoCreatedKeyNameV1 := []byte("created")

	// V1 bucket info.
	bucketNameV1 := []byte("dbinfo")
	bucketIdV1, err := fetchLegacyBucketId(tx.Get, bucketNameV1)
	if err != nil {
		return err
	}

	// uint32Bytes is a helper function to convert a uint32 to a byte slice
	// using the byte order specified by the database namespace.
	byteOrder := binary.LittleEndian
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
	verKey := prefixedKey(bucketIdV1, utxoDbInfoVersionKeyNameV1)
	err = tx.Put(verKey, uint32Bytes(info.version))
	if err != nil {
		return err
	}

	// Store the compression version.
	compVerKey := prefixedKey(bucketIdV1, utxoDbInfoCompVerKeyNameV1)
	err = tx.Put(compVerKey, uint32Bytes(info.compVer))
	if err != nil {
		return err
	}

	// Store the UTXO set version.
	utxoVerKey := prefixedKey(bucketIdV1, utxoDbInfoUtxoVerKeyNameV1)
	err = tx.Put(utxoVerKey, uint32Bytes(info.utxoVer))
	if err != nil {
		return err
	}

	// Store the database creation date.
	createdKey := prefixedKey(bucketIdV1, utxoDbInfoCreatedKeyNameV1)
	return tx.Put(createdKey, uint64Bytes(uint64(info.created.Unix())))
}

// upgradeUtxoDbToVersion2 upgrades a UTXO database from version 1 to version 2.
func upgradeUtxoDbToVersion2(ctx context.Context, utxoBackend UtxoBackend) error {
	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	log.Info("Upgrading UTXO database to version 2...")
	start := time.Now()

	// Migrate the UTXO data to use simple key prefixes rather than buckets.
	err := migrateUtxoDbBuckets(ctx, utxoBackend)
	if err != nil {
		return err
	}

	// Fetch the backend versioning info.
	utxoDbInfo, err := utxoBackend.FetchInfo()
	if err != nil {
		return err
	}

	// Update and persist the UTXO database version.
	utxoDbInfo.version = 2
	err = utxoBackend.PutInfo(utxoDbInfo)
	if err != nil {
		return err
	}

	// Update and persist the UTXO database version in the legacy V1 bucket.
	// This allows older versions to identify that a newer database version exists
	// in the case of a downgrade.
	err = utxoBackend.Update(func(tx UtxoBackendTx) error {
		return dbPutUtxoBackendInfoV1(tx, utxoDbInfo)
	})
	if err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done upgrading database in %v.", elapsed)
	return nil
}

// moveUtxoDatabase moves the UTXO database from the provided old path to the
// provided new path.  The database must exist at the provided old path.
func moveUtxoDatabase(ctx context.Context, oldPath string, newPath string) error {
	log.Info("Moving UTXO database.  This may take a while...")
	start := time.Now()

	// Options for open the leveldb database.
	opts := opt.Options{
		Strict:      opt.DefaultStrict,
		Compression: opt.NoCompression,
		Filter:      filter.NewBloomFilter(10),
	}

	// Open the database at the old path.
	oldDb, err := leveldb.OpenFile(oldPath, &opts)
	if err != nil {
		str := fmt.Sprintf("failed to open UTXO database at old path: %v", err)
		return convertLdbErr(err, str)
	}

	// Create and open the database at the new path.
	newDb, err := leveldb.OpenFile(newPath, &opts)
	if err != nil {
		if err := oldDb.Close(); err != nil {
			return convertLdbErr(err, "failed to close UTXO database at old path")
		}
		str := fmt.Sprintf("failed to open UTXO database at new path: %v", err)
		return convertLdbErr(err, str)
	}

	// doBatch contains the primary logic for moving the UTXO database entries.
	// This is done because attempting to migrate in a single database transaction
	// could result in massive memory usage and could potentially crash on many
	// systems due to ulimits.
	//
	// It returns whether or not all entries have been updated.
	const maxEntries = 20000
	var resumeOffset uint32
	var totalMigrated uint64
	doBatch := func(tx UtxoBackendTx) (bool, error) {
		// Migrate UTXO database entries so long as the max number of entries for
		// this batch has not been exceeded.
		var logProgress bool
		var numMigrated, numIterated uint32

		iter := oldDb.NewIterator(nil, nil)
		defer iter.Release()
		for iter.Next() {
			// Reset err on each iteration.
			err = nil

			if interruptRequested(ctx) {
				logProgress = true
				err = errInterruptRequested
				break
			}

			if numMigrated >= maxEntries {
				logProgress = true
				err = errBatchFinished
				break
			}

			// Skip entries that have already been migrated in previous batches.
			numIterated++
			if numIterated-1 < resumeOffset {
				continue
			}
			resumeOffset++

			// Move the UTXO database entry to the new database.
			err := tx.Put(iter.Key(), iter.Value())
			if err != nil {
				return false, fmt.Errorf("error migrating UTXO database entry: %w", err)
			}

			numMigrated++
		}
		if iterErr := iter.Error(); iterErr != nil {
			return false, convertLdbErr(iterErr, iterErr.Error())
		}
		isFullyDone := err == nil
		if (isFullyDone || logProgress) && numMigrated > 0 {
			totalMigrated += uint64(numMigrated)
			log.Infof("Migrated %d entries (%d total)", numMigrated,
				totalMigrated)
		}
		return isFullyDone, err
	}

	// Migrate all entries in batches for the reasons mentioned above.
	utxoBackend := NewLevelDbUtxoBackend(newDb)
	if err := utxoBackendBatchedUpdate(ctx, utxoBackend, doBatch); err != nil {
		if err := oldDb.Close(); err != nil {
			return convertLdbErr(err, "failed to close UTXO database at old path")
		}
		if err := newDb.Close(); err != nil {
			return convertLdbErr(err, "failed to close UTXO database at new path")
		}
		return err
	}

	// Close the old and new databases.
	if err := oldDb.Close(); err != nil {
		return convertLdbErr(err, "failed to close UTXO database at old path")
	}
	if err := newDb.Close(); err != nil {
		return convertLdbErr(err, "failed to close UTXO database at new path")
	}

	// Remove the old database.
	fi, err := os.Stat(oldPath)
	if err != nil {
		return err
	}
	log.Infof("Removing old UTXO database from '%s'", oldPath)
	err = removeDB(oldPath, fi)
	if err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done moving UTXO database.  Total entries: %d in %v",
		totalMigrated, elapsed)

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
// all possible upgrades iteratively.  Note that spend journal and utxo set
// upgrades are handled separately starting with version 3 of the spend journal
// and utxo set.
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

	// Update to a version 10 database if needed.  This entails writing the
	// database spend journal version to the database so that it can be decoupled
	// from the overall version of the block database.
	//
	// This applies to both versions 8 and 9 because previously version 9 handled
	// the migration to version 3 of the spend journal and utxo set.  That
	// migration has now been decoupled to run based on the spend journal and utxo
	// set versions, but databases that are already upgraded to version 9 still
	// need to be handled here as well.
	if dbInfo.version == 8 || dbInfo.version == 9 {
		if err := upgradeToVersion10(ctx, db, dbInfo); err != nil {
			return err
		}
	}

	// Update to a version 11 database if needed.  This entails bumping the
	// overall block database version to 11 to prevent downgrades as the UTXO
	// database is being moved in this same set of changes.
	if dbInfo.version == 10 {
		if err := upgradeToVersion11(ctx, db, dbInfo); err != nil {
			return err
		}
	}

	// Update to a version 12 database if needed.  This entails correcting block
	// index and ticket database entries to account for an issue in legacy code
	// that caused votes which also include treasury spend vote data to be
	// incorrectly marked as missed as well as unmarking all blocks previously
	// marked failed so they are eligible for validation again under the new
	// consensus rules.
	if dbInfo.version == 11 {
		if err := upgradeToVersion12(ctx, db, chainParams, dbInfo); err != nil {
			return err
		}
	}

	return nil
}

// upgradeSpendJournal upgrades old spend journal versions to the newest version
// by applying all possible upgrades iteratively.  The spend journal version was
// decoupled from the overall block database version as of spend journal version
// 3, so spend journal upgrades prior to version 3 are handled in the overall
// block database upgrade path.
//
// NOTE: The database info housed by the passed blockchain instance will be
// updated with the latest versions.
func upgradeSpendJournal(ctx context.Context, b *BlockChain) error {
	// Update to a version 3 spend journal as needed.
	if b.dbInfo.stxoVer == 2 {
		if err := upgradeSpendJournalToVersion3(ctx, b); err != nil {
			return err
		}
	}

	return nil
}

// upgradeUtxoDb upgrades old utxo database versions to the newest version by
// applying all possible upgrades iteratively.  The utxo set version was
// decoupled from the overall block database version as of utxo set version 3,
// so utxo set upgrades prior to version 3 are handled in the block database
// upgrade path.
//
// NOTE: The database info housed in the passed block database instance and
// backend info housed in the passed utxo backend instance will be updated with
// the latest versions.
func upgradeUtxoDb(ctx context.Context, db database.DB,
	utxoBackend UtxoBackend) error {

	// Fetch the backend versioning info.
	utxoDbInfo, err := utxoBackend.FetchInfo()
	if err != nil {
		return err
	}

	// Update to a version 2 UTXO database as needed.
	if utxoDbInfo.version == 1 {
		if err := upgradeUtxoDbToVersion2(ctx, utxoBackend); err != nil {
			return err
		}
	}

	// Update to a version 3 utxo set as needed.
	if utxoDbInfo.utxoVer == 2 {
		if err := upgradeUtxoSetToVersion3(ctx, db, utxoBackend); err != nil {
			return err
		}
	}

	// Check if the block database contains the UTXO set or state.  If it does,
	// move the UTXO set and state from the block database to the UTXO database.
	blockDbUtxoSetExists := false
	db.View(func(dbTx database.Tx) error {
		if dbTx.Metadata().Bucket([]byte("utxosetv3")) != nil ||
			dbTx.Metadata().Get([]byte("utxosetstate")) != nil {

			blockDbUtxoSetExists = true
		}
		return nil
	})
	if blockDbUtxoSetExists {
		err := separateUtxoDatabase(ctx, db, utxoBackend)
		if err != nil {
			return err
		}
	}

	return nil
}
