// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/decred/dcrd/blockchain/internal/progresslog"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/blockchain/standalone"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/dcrutil"
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
func interruptRequested(interrupted <-chan struct{}) bool {
	select {
	case <-interrupted:
		return true
	default:
	}

	return false
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

// ticketsVotedInBlock fetches a list of tickets that were voted in the
// block.
func ticketsVotedInBlock(bl *dcrutil.Block) []chainhash.Hash {
	var tickets []chainhash.Hash
	for _, stx := range bl.MsgBlock().STransactions {
		if stake.IsSSGen(stx) {
			tickets = append(tickets, stx.TxIn[1].PreviousOutPoint.Hash)
		}
	}

	return tickets
}

// ticketsRevokedInBlock fetches a list of tickets that were revoked in the
// block.
func ticketsRevokedInBlock(bl *dcrutil.Block) []chainhash.Hash {
	var tickets []chainhash.Hash
	for _, stx := range bl.MsgBlock().STransactions {
		if stake.DetermineTxType(stx) == stake.TxTypeSSRtx {
			tickets = append(tickets, stx.TxIn[0].PreviousOutPoint.Hash)
		}
	}

	return tickets
}

// upgradeToVersion2 upgrades a version 1 blockchain to version 2, allowing
// use of the new on-disk ticket database.
func upgradeToVersion2(db database.DB, chainParams *chaincfg.Params, dbInfo *databaseInfo) error {
	// Hardcoded so updates to the global values do not affect old upgrades.
	byteOrder := binary.LittleEndian
	chainStateKeyName := []byte("chainstate")
	heightIdxBucketName := []byte("heightidx")

	// These are legacy functions that relied on information in the database
	// that is no longer available in more recent code.
	dbFetchHashByHeight := func(dbTx database.Tx, height int64) (*chainhash.Hash, error) {
		var serializedHeight [4]byte
		byteOrder.PutUint32(serializedHeight[:], uint32(height))

		meta := dbTx.Metadata()
		heightIndex := meta.Bucket(heightIdxBucketName)
		hashBytes := heightIndex.Get(serializedHeight[:])
		if hashBytes == nil {
			str := fmt.Sprintf("no block at height %d exists", height)
			return nil, errNotInMainChain(str)
		}

		var hash chainhash.Hash
		copy(hash[:], hashBytes)
		return &hash, nil
	}
	dbFetchBlockByHeight := func(dbTx database.Tx, height int64) (*dcrutil.Block, error) {
		// First find the hash associated with the provided height in the index.
		hash, err := dbFetchHashByHeight(dbTx, height)
		if err != nil {
			return nil, err
		}

		// Load the raw block bytes from the database.
		blockBytes, err := dbTx.FetchBlock(hash)
		if err != nil {
			return nil, err
		}

		// Create the encapsulated block and set the height appropriately.
		block, err := dcrutil.NewBlockFromBytes(blockBytes)
		if err != nil {
			return nil, err
		}

		return block, nil
	}

	log.Infof("Initializing upgrade to database version 2")
	progressLogger := progresslog.NewBlockProgressLogger("Upgraded", log)

	// The upgrade is atomic, so there is no need to set the flag that
	// the database is undergoing an upgrade here.  Get the stake node
	// for the genesis block, and then begin connecting stake nodes
	// incrementally.
	err := db.Update(func(dbTx database.Tx) error {
		// Fetch the stored best chain state from the database metadata.
		serializedData := dbTx.Metadata().Get(chainStateKeyName)
		best, err := deserializeBestChainState(serializedData)
		if err != nil {
			return err
		}

		bestStakeNode, errLocal := stake.InitDatabaseState(dbTx, chainParams)
		if errLocal != nil {
			return errLocal
		}

		parent, errLocal := dbFetchBlockByHeight(dbTx, 0)
		if errLocal != nil {
			return errLocal
		}

		for i := int64(1); i <= int64(best.height); i++ {
			block, errLocal := dbFetchBlockByHeight(dbTx, i)
			if errLocal != nil {
				return errLocal
			}

			// If we need the tickets, fetch them too.
			var newTickets []chainhash.Hash
			if i >= chainParams.StakeEnabledHeight {
				matureHeight := i - int64(chainParams.TicketMaturity)
				matureBlock, errLocal := dbFetchBlockByHeight(dbTx, matureHeight)
				if errLocal != nil {
					return errLocal
				}
				for _, stx := range matureBlock.MsgBlock().STransactions {
					if stake.IsSStx(stx) {
						h := stx.TxHash()
						newTickets = append(newTickets, h)
					}
				}
			}

			// Iteratively connect the stake nodes in memory.
			header := block.MsgBlock().Header
			hB, errLocal := header.Bytes()
			if errLocal != nil {
				return errLocal
			}
			bestStakeNode, errLocal = bestStakeNode.ConnectNode(
				stake.CalcHash256PRNGIV(hB), ticketsVotedInBlock(block),
				ticketsRevokedInBlock(block), newTickets)
			if errLocal != nil {
				return errLocal
			}

			// Write the top block stake node to the database.
			errLocal = stake.WriteConnectedBestNode(dbTx, bestStakeNode,
				best.hash)
			if errLocal != nil {
				return errLocal
			}

			progressLogger.LogBlockHeight(block.MsgBlock(), parent.MsgBlock())
			parent = block
		}

		// Write the new database version.
		dbInfo.version = 2
		return dbPutDatabaseInfo(dbTx, dbInfo)
	})
	if err != nil {
		return err
	}

	log.Infof("Upgrade to new stake database was successful!")

	return nil
}

// migrateBlockIndex migrates all block entries from the v1 block index bucket
// manged by ffldb to the v2 bucket managed by this package.  The v1 bucket
// stored all block entries keyed by block hash, whereas the v2 bucket stores
// them keyed by block height + hash.  Also, the old block index only stored the
// header, while the new one stores all info needed to recreate block nodes.
//
// The new block index is guaranteed to be fully updated if this returns without
// failure.
func migrateBlockIndex(db database.DB, interrupt <-chan struct{}) error {
	// blkHdrOffset defines the offsets into a v1 block index row for the block
	// header.
	//
	// The serialized block index row format is:
	//   <blocklocation><blockheader>
	const blkHdrOffset = 12

	// blkHdrHeightStart is the offset of the height in the serialized block
	// header bytes as it existed at the time of this migration.  It is hard
	// coded here so potential future changes do not affect old upgrades.
	const blkHdrHeightStart = 128

	// Hardcoded bucket names so updates to the global values do not affect old
	// upgrades.
	v1BucketName := []byte("ffldb-blockidx")
	v2BucketName := []byte("blockidx")
	hashIdxBucketName := []byte("hashidx")

	log.Info("Reindexing block information in the database.  This will take " +
		"a while...")
	start := time.Now()

	// Create the new block index bucket as needed.
	err := db.Update(func(dbTx database.Tx) error {
		_, err := dbTx.Metadata().CreateBucketIfNotExists(v2BucketName)
		return err
	})
	if err != nil {
		return err
	}

	// doBatch contains the primary logic for upgrading the block index from
	// version 1 to 2 in batches.  This is done because attempting to migrate in
	// a single database transaction could result in massive memory usage and
	// could potentially crash on many systems due to ulimits.
	//
	// It returns the number of entries processed.
	const maxEntries = 20000
	var resumeOffset uint32
	doBatch := func(dbTx database.Tx) (uint32, error) {
		meta := dbTx.Metadata()
		v1BlockIdxBucket := meta.Bucket(v1BucketName)
		if v1BlockIdxBucket == nil {
			return 0, fmt.Errorf("bucket %s does not exist", v1BucketName)
		}

		v2BlockIdxBucket := meta.Bucket(v2BucketName)
		if v2BlockIdxBucket == nil {
			return 0, fmt.Errorf("bucket %s does not exist", v2BucketName)
		}

		hashIdxBucket := meta.Bucket(hashIdxBucketName)
		if hashIdxBucket == nil {
			return 0, fmt.Errorf("bucket %s does not exist", hashIdxBucketName)
		}

		// Migrate block index entries so long as the max number of entries for
		// this batch has not been exceeded.
		var numMigrated, numIterated uint32
		err := v1BlockIdxBucket.ForEach(func(hashBytes, blockRow []byte) error {
			if numMigrated >= maxEntries {
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
			var blockHash chainhash.Hash
			copy(blockHash[:], hashBytes)
			endOffset := blkHdrOffset + blockHdrSize
			headerBytes := blockRow[blkHdrOffset:endOffset:endOffset]
			heightBytes := headerBytes[blkHdrHeightStart : blkHdrHeightStart+4]
			height := binary.LittleEndian.Uint32(heightBytes)
			key := blockIndexKey(&blockHash, height)
			if v2BlockIdxBucket.Get(key) != nil {
				return nil
			}

			// Load the raw full block from the database.
			blockBytes, err := dbTx.FetchBlock(&blockHash)
			if err != nil {
				return err
			}

			// Deserialize the block bytes.
			var block wire.MsgBlock
			err = block.Deserialize(bytes.NewReader(blockBytes))
			if err != nil {
				return err
			}

			// Mark the block as valid if it's part of the main chain.  While it
			// is possible side chain blocks were validated too, there was
			// previously no tracking of that information, so there is no way to
			// know for sure.  It's better to be safe and just assume side chain
			// blocks were never validated.
			status := statusDataStored
			if hashIdxBucket.Get(blockHash[:]) != nil {
				status |= statusValid
			}

			// Write the serialized block index entry to the new bucket keyed by
			// its hash and height.
			ticketInfo := stake.FindSpentTicketsInBlock(&block)
			serialized, err := serializeBlockIndexEntry(&blockIndexEntry{
				header:         block.Header,
				status:         status,
				voteInfo:       ticketInfo.Votes,
				ticketsVoted:   ticketInfo.VotedTickets,
				ticketsRevoked: ticketInfo.RevokedTickets,
			})
			if err != nil {
				return err
			}
			err = v2BlockIdxBucket.Put(key, serialized)
			if err != nil {
				return err
			}

			numMigrated++

			if interruptRequested(interrupt) {
				return errInterruptRequested
			}

			return nil
		})
		return numMigrated, err
	}

	// Migrate all entries in batches for the reasons mentioned above.
	var totalMigrated uint64
	for {
		var numMigrated uint32
		err := db.Update(func(dbTx database.Tx) error {
			var err error
			numMigrated, err = doBatch(dbTx)
			if err == errInterruptRequested || err == errBatchFinished {
				// No error here so the database transaction is not cancelled
				// and therefore outstanding work is written to disk.  The
				// outer function will exit with an interrupted error below due
				// to another interrupted check.
				err = nil
			}
			return err
		})
		if err != nil {
			return err
		}

		if interruptRequested(interrupt) {
			return errInterruptRequested
		}

		if numMigrated == 0 {
			break
		}

		totalMigrated += uint64(numMigrated)
		log.Infof("Migrated %d entries (%d total)", numMigrated, totalMigrated)
	}

	seconds := int64(time.Since(start) / time.Second)
	log.Infof("Done upgrading block index.  Total entries: %d in %d seconds",
		totalMigrated, seconds)
	return nil
}

// upgradeToVersion3 upgrades a version 2 blockchain to version 3 along with
// upgrading the block index to version 2.
func upgradeToVersion3(db database.DB, dbInfo *databaseInfo, interrupt <-chan struct{}) error {
	if err := migrateBlockIndex(db, interrupt); err != nil {
		return err
	}

	// Update and persist the updated database versions.
	dbInfo.version = 3
	dbInfo.bidxVer = 2
	return db.Update(func(dbTx database.Tx) error {
		return dbPutDatabaseInfo(dbTx, dbInfo)
	})
}

// removeMainChainIndex removes the main chain hash index and height index
// buckets.  These are no longer needed due to using the full block index in
// memory.
//
// The database is guaranteed to be fully updated if this returns without
// failure.
func removeMainChainIndex(db database.DB, interrupt <-chan struct{}) error {
	// Hardcoded bucket names so updates to the global values do not affect old
	// upgrades.
	hashIdxBucketName := []byte("hashidx")
	heightIdxBucketName := []byte("heightidx")

	log.Info("Removing unneeded indexes in the database...")
	start := time.Now()

	// Delete the main chain index buckets.
	err := db.Update(func(dbTx database.Tx) error {
		// Delete the main chain hash to height index.
		meta := dbTx.Metadata()
		hashIdxBucket := meta.Bucket(hashIdxBucketName)
		if hashIdxBucket != nil {
			if err := meta.DeleteBucket(hashIdxBucketName); err != nil {
				return err
			}
			log.Infof("Removed hash index.")
		}

		if interruptRequested(interrupt) {
			// No error here so the database transaction is not cancelled
			// and therefore outstanding work is written to disk.  The
			// outer function will exit with an interrupted error below due
			// to another interrupted check.
			return nil
		}

		// Delete the main chain hash to height index.
		heightIdxBucket := meta.Bucket(heightIdxBucketName)
		if heightIdxBucket != nil {
			if err := meta.DeleteBucket(heightIdxBucketName); err != nil {
				return err
			}
			log.Infof("Removed height index.")
		}

		return nil
	})
	if err != nil {
		return err
	}

	if interruptRequested(interrupt) {
		return errInterruptRequested
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done upgrading database in %v.", elapsed)
	return nil
}

// upgradeToVersion4 upgrades a version 3 blockchain database to version 4.
func upgradeToVersion4(db database.DB, dbInfo *databaseInfo, interrupt <-chan struct{}) error {
	if err := removeMainChainIndex(db, interrupt); err != nil {
		return err
	}

	// Update and persist the updated database versions.
	dbInfo.version = 4
	return db.Update(func(dbTx database.Tx) error {
		return dbPutDatabaseInfo(dbTx, dbInfo)
	})
}

// incrementalFlatDrop uses multiple database updates to remove key/value pairs
// saved to a flag bucket.
func incrementalFlatDrop(db database.DB, bucketKey []byte, humanName string, interrupt <-chan struct{}) error {
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

		if interruptRequested(interrupt) {
			return errInterruptRequested
		}
	}
	return nil
}

// upgradeToVersion5 upgrades a version 4 blockchain database to version 5.
func upgradeToVersion5(db database.DB, chainParams *chaincfg.Params, dbInfo *databaseInfo, interrupt <-chan struct{}) error {
	// Hardcoded bucket and key names so updates to the global values do not
	// affect old upgrades.
	utxoSetBucketName := []byte("utxoset")
	spendJournalBucketName := []byte("spendjournal")
	chainStateKeyName := []byte("chainstate")
	v5ReindexTipKeyName := []byte("v5reindextip")

	log.Info("Clearing database utxoset and spend journal for upgrade...")
	start := time.Now()

	// Clear the utxoset.
	err := incrementalFlatDrop(db, utxoSetBucketName, "utxoset", interrupt)
	if err != nil {
		return err
	}
	log.Infof("Cleared utxoset.")

	if interruptRequested(interrupt) {
		return errInterruptRequested
	}

	// Clear the spend journal.
	err = incrementalFlatDrop(db, spendJournalBucketName, "spend journal",
		interrupt)
	if err != nil {
		return err
	}
	log.Infof("Cleared spend journal.")

	if interruptRequested(interrupt) {
		return errInterruptRequested
	}

	err = db.Update(func(dbTx database.Tx) error {
		// Reset the ticket database to the genesis block.
		log.Infof("Resetting the ticket database.  This might take a while...")
		if err := stake.ResetDatabase(dbTx, chainParams); err != nil {
			return err
		}

		// Fetch the stored best chain state from the database metadata.
		meta := dbTx.Metadata()
		serializedData := meta.Get(chainStateKeyName)
		best, err := deserializeBestChainState(serializedData)
		if err != nil {
			return err
		}

		// Store the current best chain tip as the reindex target.
		if err := meta.Put(v5ReindexTipKeyName, best.hash[:]); err != nil {
			return err
		}

		// Reset the state related to the best block to the genesis block.
		genesisBlock := chainParams.GenesisBlock
		numTxns := uint64(len(genesisBlock.Transactions))
		serializedData = serializeBestChainState(bestChainState{
			hash:         genesisBlock.BlockHash(),
			height:       0,
			totalTxns:    numTxns,
			totalSubsidy: 0,
			workSum:      standalone.CalcWork(genesisBlock.Header.Bits),
		})
		err = meta.Put(chainStateKeyName, serializedData)
		if err != nil {
			return err
		}

		// Update and persist the updated database versions.
		dbInfo.version = 5
		return dbPutDatabaseInfo(dbTx, dbInfo)
	})
	if err != nil {
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	log.Infof("Done upgrading database in %v.", elapsed)
	return nil
}

// maybeFinishV5Upgrade potentially reindexes the chain due to a version 5
// database upgrade.  It will resume previously uncompleted attempts.
func (b *BlockChain) maybeFinishV5Upgrade() error {
	// Nothing to do if the database is not version 5.
	if b.dbInfo.version != 5 {
		return nil
	}

	// Hardcoded key name so updates to the global values do not affect old
	// upgrades.
	v5ReindexTipKeyName := []byte("v5reindextip")

	// Finish the version 5 reindex as needed.
	var v5ReindexTipHash *chainhash.Hash
	err := b.db.View(func(dbTx database.Tx) error {
		hash := dbTx.Metadata().Get(v5ReindexTipKeyName)
		if hash != nil {
			v5ReindexTipHash = new(chainhash.Hash)
			copy(v5ReindexTipHash[:], hash)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if v5ReindexTipHash != nil {
		// Look up the final target tip to reindex to in the block index.
		targetTip := b.index.LookupNode(v5ReindexTipHash)
		if targetTip == nil {
			return AssertError(fmt.Sprintf("maybeFinishV5Upgrade: cannot find "+
				"chain tip %s in block index", v5ReindexTipHash))
		}

		// Ensure all ancestors of the current best chain tip are marked as
		// valid.  This is necessary due to older software versions not marking
		// nodes before the final checkpoint as valid.
		for node := targetTip; node != nil; node = node.parent {
			b.index.SetStatusFlags(node, statusValid)
		}
		if err := b.index.flush(); err != nil {
			return err
		}

		// Disable notifications during the reindex.
		ntfnCallback := b.notifications
		b.notifications = nil
		defer func() {
			b.notifications = ntfnCallback
		}()

		tip := b.bestChain.Tip()
		for tip != targetTip {
			if interruptRequested(b.interrupt) {
				return errInterruptRequested
			}

			// Limit to a reasonable number of blocks at a time.
			const maxReindexBlocks = 250
			intermediateTip := targetTip
			if intermediateTip.height-tip.height > maxReindexBlocks {
				intermediateTip = intermediateTip.Ancestor(tip.height +
					maxReindexBlocks)
			}

			log.Infof("Reindexing to height %d of %d (progress %.2f%%)...",
				intermediateTip.height, targetTip.height,
				float64(intermediateTip.height)/float64(targetTip.height)*100)
			b.chainLock.Lock()
			if err := b.reorganizeChainInternal(intermediateTip); err != nil {
				b.chainLock.Unlock()
				return err
			}
			b.chainLock.Unlock()

			tip = b.bestChain.Tip()
		}

		// Mark the v5 reindex as complete by removing the associated key.
		err := b.db.Update(func(dbTx database.Tx) error {
			return dbTx.Metadata().Delete(v5ReindexTipKeyName)
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// upgradeDB upgrades old database versions to the newest version by applying
// all possible upgrades iteratively.
//
// NOTE: The passed database info will be updated with the latest versions.
func upgradeDB(db database.DB, chainParams *chaincfg.Params, dbInfo *databaseInfo, interrupt <-chan struct{}) error {
	if dbInfo.version == 1 {
		if err := upgradeToVersion2(db, chainParams, dbInfo); err != nil {
			return err
		}
	}

	// Migrate to the new v2 block index format if needed.  That database
	// version was bumped because prior versions of the software did not have
	// a block index version.
	if dbInfo.version == 2 && dbInfo.bidxVer < 2 {
		if err := upgradeToVersion3(db, dbInfo, interrupt); err != nil {
			return err
		}
	}

	// Remove the main chain index from the database if needed.
	if dbInfo.version == 3 {
		if err := upgradeToVersion4(db, dbInfo, interrupt); err != nil {
			return err
		}
	}

	// Clear the utxoset, clear the spend journal, reset the best chain back to
	// the genesis block, and mark that a v5 reindex is required if needed.
	if dbInfo.version == 4 {
		err := upgradeToVersion5(db, chainParams, dbInfo, interrupt)
		if err != nil {
			return err
		}
	}

	return nil
}
