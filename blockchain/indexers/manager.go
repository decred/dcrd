// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2016-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"context"
	"errors"
	"fmt"

	"github.com/decred/dcrd/blockchain/v4/internal/progresslog"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/dcrutil/v4"
)

var (
	// indexTipsBucketName is the name of the db bucket used to house the
	// current tip of each index.
	indexTipsBucketName = []byte("idxtips")
)

// -----------------------------------------------------------------------------
// The index manager tracks the current tip and version of each index by using a
// parent bucket that contains an entry for index and a separate entry for its
// version.
//
// The serialized format for an index tip is:
//
//   [<block hash><block height>],...
//
//   Field           Type             Size
//   block hash      chainhash.Hash   chainhash.HashSize
//   block height    uint32           4 bytes
//
// The serialized format for an index version is:
//
//   [<version>]
//
//   Field           Type             Size
//   index version   uint32           4 bytes
// -----------------------------------------------------------------------------

// dbPutIndexerTip uses an existing database transaction to update or add the
// current tip for the given index to the provided values.
func dbPutIndexerTip(dbTx database.Tx, idxKey []byte, hash *chainhash.Hash, height int32) error {
	serialized := make([]byte, chainhash.HashSize+4)
	copy(serialized, hash[:])
	byteOrder.PutUint32(serialized[chainhash.HashSize:], uint32(height))

	indexesBucket := dbTx.Metadata().Bucket(indexTipsBucketName)
	return indexesBucket.Put(idxKey, serialized)
}

// dbFetchIndexerTip uses an existing database transaction to retrieve the
// hash and height of the current tip for the provided index.
func dbFetchIndexerTip(dbTx database.Tx, idxKey []byte) (*chainhash.Hash, int32, error) {
	indexesBucket := dbTx.Metadata().Bucket(indexTipsBucketName)
	serialized := indexesBucket.Get(idxKey)
	if len(serialized) < chainhash.HashSize+4 {
		str := fmt.Sprintf("unexpected end of data for index %q tip",
			string(idxKey))
		return nil, 0, makeDbErr(database.ErrCorruption, str)
	}

	var hash chainhash.Hash
	copy(hash[:], serialized[:chainhash.HashSize])
	height := int32(byteOrder.Uint32(serialized[chainhash.HashSize:]))
	return &hash, height, nil
}

// indexVersionKey returns the key for an index which houses the current version
// of the index.
func indexVersionKey(idxKey []byte) []byte {
	verKey := make([]byte, len(idxKey)+1)
	verKey[0] = 'v'
	copy(verKey[1:], idxKey)
	return verKey
}

// dbPutIndexerVersion uses an existing database transaction to update the
// version for the given index to the provided value.
func dbPutIndexerVersion(dbTx database.Tx, idxKey []byte, version uint32) error {
	serialized := make([]byte, 4)
	byteOrder.PutUint32(serialized[0:4], version)

	indexesBucket := dbTx.Metadata().Bucket(indexTipsBucketName)
	return indexesBucket.Put(indexVersionKey(idxKey), serialized)
}

// dbFetchIndexerVersion uses an existing database transaction to retrieve the
// version of the provided index.  It will return one if the version has not
// previously been stored.
func dbFetchIndexerVersion(dbTx database.Tx, idxKey []byte) (uint32, error) {
	indexesBucket := dbTx.Metadata().Bucket(indexTipsBucketName)
	serialized := indexesBucket.Get(indexVersionKey(idxKey))
	if len(serialized) == 0 {
		return 1, nil
	}

	if len(serialized) < 4 {
		str := fmt.Sprintf("unexpected end of data for index %q version",
			string(idxKey))
		return 0, makeDbErr(database.ErrCorruption, str)
	}

	version := byteOrder.Uint32(serialized)
	return version, nil
}

// dbIndexConnectBlock adds all of the index entries associated with the
// given block using the provided indexer and updates the tip of the indexer
// accordingly.  An error will be returned if the current tip for the indexer is
// not the previous block for the passed block.
func dbIndexConnectBlock(dbTx database.Tx, indexer Indexer, block, parent *dcrutil.Block, prevScripts PrevScripter, isTreasuryEnabled bool) error {
	// Assert that the block being connected properly connects to the
	// current tip of the index.
	idxKey := indexer.Key()
	curTipHash, _, err := dbFetchIndexerTip(dbTx, idxKey)
	if err != nil {
		return err
	}
	if !curTipHash.IsEqual(&block.MsgBlock().Header.PrevBlock) {
		return AssertError(fmt.Sprintf("dbIndexConnectBlock must be "+
			"called with a block that extends the current index "+
			"tip (%s, tip %s, block %s)", indexer.Name(),
			curTipHash, block.Hash()))
	}

	// Notify the indexer with the connected block so it can index it.
	err = indexer.ConnectBlock(dbTx, block, parent, prevScripts,
		isTreasuryEnabled)
	if err != nil {
		return err
	}

	// Update the current index tip.
	return dbPutIndexerTip(dbTx, idxKey, block.Hash(), int32(block.Height()))
}

// dbIndexDisconnectBlock removes all of the index entries associated with the
// given block using the provided indexer and updates the tip of the indexer
// accordingly.  An error will be returned if the current tip for the indexer is
// not the passed block.
func dbIndexDisconnectBlock(dbTx database.Tx, indexer Indexer, block, parent *dcrutil.Block, prevScripts PrevScripter, isTreasuryEnabled bool) error {
	// Assert that the block being disconnected is the current tip of the
	// index.
	idxKey := indexer.Key()
	curTipHash, _, err := dbFetchIndexerTip(dbTx, idxKey)
	if err != nil {
		return err
	}
	if !curTipHash.IsEqual(block.Hash()) {
		return AssertError(fmt.Sprintf("dbIndexDisconnectBlock must "+
			"be called with the block at the current index tip "+
			"(%s, tip %s, block %s)", indexer.Name(),
			curTipHash, block.Hash()))
	}

	// Notify the indexer with the disconnected block so it can remove all
	// of the appropriate entries.
	err = indexer.DisconnectBlock(dbTx, block, parent, prevScripts,
		isTreasuryEnabled)
	if err != nil {
		return err
	}

	// Update the current index tip.
	prevHash := &block.MsgBlock().Header.PrevBlock
	return dbPutIndexerTip(dbTx, idxKey, prevHash, int32(block.Height()-1))
}

// Manager defines an index manager that manages multiple optional indexes and
// implements the IndexManager interface so it can be seamlessly plugged into
// normal chain processing.
type Manager struct {
	params         *chaincfg.Params
	db             database.DB
	enabledIndexes []Indexer
}

// Ensure the Manager type implements the IndexManager interface.
var _ IndexManager = (*Manager)(nil)

// indexDropKey returns the key for an index which indicates it is in the
// process of being dropped.
func indexDropKey(idxKey []byte) []byte {
	dropKey := make([]byte, len(idxKey)+1)
	dropKey[0] = 'd'
	copy(dropKey[1:], idxKey)
	return dropKey
}

// maybeFinishDrops determines if each of the enabled indexes are in the middle
// of being dropped and finishes dropping them when the are.  This is necessary
// because dropping and index has to be done in several atomic steps rather than
// one big atomic step due to the massive number of entries.
func (m *Manager) maybeFinishDrops(ctx context.Context) error {
	indexNeedsDrop := make([]bool, len(m.enabledIndexes))
	err := m.db.View(func(dbTx database.Tx) error {
		// None of the indexes needs to be dropped if the index tips
		// bucket hasn't been created yet.
		indexesBucket := dbTx.Metadata().Bucket(indexTipsBucketName)
		if indexesBucket == nil {
			return nil
		}

		// Mark the indexer as requiring a drop if one is already in
		// progress.
		for i, indexer := range m.enabledIndexes {
			dropKey := indexDropKey(indexer.Key())
			if indexesBucket.Get(dropKey) != nil {
				indexNeedsDrop[i] = true
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	// Finish dropping any of the enabled indexes that are already in the
	// middle of being dropped.
	for i, indexer := range m.enabledIndexes {
		if !indexNeedsDrop[i] {
			continue
		}

		log.Infof("Resuming %s drop", indexer.Name())

		switch d := indexer.(type) {
		case IndexDropper:
			err := d.DropIndex(ctx, m.db)
			if err != nil {
				return err
			}
		default:
			err := dropIndex(m.db, indexer.Key(), indexer.Name())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// maybeCreateIndexes determines if each of the enabled indexes have already
// been created and creates them if not.
func (m *Manager) maybeCreateIndexes() error {
	return m.db.Update(func(dbTx database.Tx) error {
		// Create the bucket for the current tips as needed.
		meta := dbTx.Metadata()
		indexesBucket, err := meta.CreateBucketIfNotExists(indexTipsBucketName)
		if err != nil {
			return err
		}

		for _, indexer := range m.enabledIndexes {
			// Nothing to do if the index tip already exists.
			idxKey := indexer.Key()
			if indexesBucket.Get(idxKey) != nil {
				continue
			}

			// Store the index version.
			err := dbPutIndexerVersion(dbTx, idxKey, indexer.Version())
			if err != nil {
				return err
			}

			// The tip for the index does not exist, so create it and
			// invoke the create callback for the index so it can perform
			// any one-time initialization it requires.
			if err := indexer.Create(dbTx); err != nil {
				return err
			}

			// Set the tip for the index to values which represent an
			// uninitialized index (the genesis block hash and height).
			genesisBlockHash := m.params.GenesisBlock.BlockHash()
			err = dbPutIndexerTip(dbTx, idxKey, &genesisBlockHash, 0)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// upgradeIndexes determines if each of the enabled indexes need to be upgraded
// and drops them when they do.
func (m *Manager) upgradeIndexes(ctx context.Context) error {
	indexNeedsDrop := make([]bool, len(m.enabledIndexes))
	err := m.db.View(func(dbTx database.Tx) error {
		// None of the indexes needs to be updated if the index tips bucket
		// hasn't been created yet.
		indexesBucket := dbTx.Metadata().Bucket(indexTipsBucketName)
		if indexesBucket == nil {
			return nil
		}

		for i, indexer := range m.enabledIndexes {
			idxKey := indexer.Key()
			version, err := dbFetchIndexerVersion(dbTx, idxKey)
			if err != nil {
				return err
			}

			// Upgrade is not needed if the index hasn't been created yet.
			if version < indexer.Version() {
				indexNeedsDrop[i] = true
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	// Drop any of the enabled indexes that have bumped their version.
	for i, indexer := range m.enabledIndexes {
		if !indexNeedsDrop[i] {
			continue
		}

		log.Infof("Dropping %s due to new version", indexer.Name())

		switch d := indexer.(type) {
		case IndexDropper:
			err := d.DropIndex(ctx, m.db)
			if err != nil {
				return err
			}
		default:
			err := dropIndex(m.db, indexer.Key(), indexer.Name())
			if err != nil {
				return err
			}
		}
	}

	// Create the initial state for the indexes that were dropped as needed.
	return m.maybeCreateIndexes()
}

// dbFetchBlockByHash uses an existing database transaction to retrieve the raw
// block for the provided hash, deserialize it, and return a dcrutil.Block.
func dbFetchBlockByHash(dbTx database.Tx, hash *chainhash.Hash) (*dcrutil.Block, error) {
	blockBytes, err := dbTx.FetchBlock(hash)
	if err != nil {
		return nil, err
	}
	return dcrutil.NewBlockFromBytes(blockBytes)
}

// Init initializes the enabled indexes.  This is called during chain
// initialization and primarily consists of catching up all indexes to the
// current best chain tip.  This is necessary since each index can be disabled
// and re-enabled at any time and attempting to catch-up indexes at the same
// time new blocks are being downloaded would lead to an overall longer time to
// catch up due to the I/O contention.
//
// This is part of the IndexManager interface.
func (m *Manager) Init(ctx context.Context, chain ChainQueryer) error {
	// Nothing to do when no indexes are enabled.
	if len(m.enabledIndexes) == 0 {
		return nil
	}

	if interruptRequested(ctx) {
		return errInterruptRequested
	}

	// Finish any drops that were previously interrupted.
	if err := m.maybeFinishDrops(ctx); err != nil {
		return err
	}

	// Create the initial state for the indexes as needed.
	if err := m.maybeCreateIndexes(); err != nil {
		return err
	}

	// Upgrade the indexes as needed.
	if err := m.upgradeIndexes(ctx); err != nil {
		return err
	}

	// Initialize each of the enabled indexes.
	for _, indexer := range m.enabledIndexes {
		if err := indexer.Init(); err != nil {
			return err
		}
	}

	// Rollback indexes to the main chain if their tip is an orphaned fork.
	// This is fairly unlikely, but it can happen if the chain is
	// reorganized while the index is disabled.  This has to be done in
	// reverse order because later indexes can depend on earlier ones.
	var err error
	var cachedBlock *dcrutil.Block
	for i := len(m.enabledIndexes); i > 0; i-- {
		indexer := m.enabledIndexes[i-1]

		// Fetch the current tip for the index.
		var height int32
		var hash *chainhash.Hash
		err := m.db.View(func(dbTx database.Tx) error {
			idxKey := indexer.Key()
			hash, height, err = dbFetchIndexerTip(dbTx, idxKey)
			return err
		})
		if err != nil {
			return err
		}

		// Nothing to do if the index does not have any entries yet.
		if height == 0 {
			continue
		}

		// Loop until the tip is a block that exists in the main chain.
		var interrupted bool
		initialHeight := height
		err = m.db.Update(func(dbTx database.Tx) error {
			for !chain.MainChainHasBlock(hash) {
				// Get the block, unless it's already cached.
				var block *dcrutil.Block
				if cachedBlock == nil && height > 0 {
					block, err = dbFetchBlockByHash(dbTx, hash)
					if err != nil {
						return err
					}
				} else {
					block = cachedBlock
				}

				// Load the parent block for the height since it is
				// required to remove it.
				parentHash := &block.MsgBlock().Header.PrevBlock
				parent, err := dbFetchBlockByHash(dbTx, parentHash)
				if err != nil {
					return err
				}
				cachedBlock = parent

				// When the index requires all of the referenced
				// txouts they need to be retrieved from the
				// database.
				var prevScripts PrevScripter
				if indexNeedsInputs(indexer) {
					var err error
					prevScripts, err = chain.PrevScripts(dbTx, block)
					if err != nil {
						return err
					}
				}

				// Find out if treasury was enabled at the parent.
				isTreasuryEnabled, err := chain.IsTreasuryEnabled(parent.Hash())
				if err != nil {
					return err
				}

				// Remove all of the index entries associated
				// with the block and update the indexer tip.
				err = dbIndexDisconnectBlock(dbTx, indexer,
					block, parent, prevScripts,
					isTreasuryEnabled)
				if err != nil {
					return err
				}

				// Update the tip to the previous block.
				hash = &block.MsgBlock().Header.PrevBlock
				height--

				// NOTE: This does not return as it does
				// elsewhere since it would cause the
				// database transaction to rollback and
				// undo all work that has been done.
				if interruptRequested(ctx) {
					interrupted = true
					break
				}
			}

			return nil
		})
		if err != nil {
			return err
		}
		if interrupted {
			return errInterruptRequested
		}

		if initialHeight != height {
			log.Infof("Removed %d orphaned blocks from %s "+
				"(heights %d to %d)", initialHeight-height,
				indexer.Name(), height+1, initialHeight)
		}
	}

	// Fetch the current tip heights for each index along with tracking the
	// lowest one so the catchup code only needs to start at the earliest
	// block and is able to skip connecting the block for the indexes that
	// don't need it.
	bestHeight := int32(chain.BestHeight())
	lowestHeight := bestHeight
	indexerHeights := make([]int32, len(m.enabledIndexes))
	err = m.db.View(func(dbTx database.Tx) error {
		for i, indexer := range m.enabledIndexes {
			idxKey := indexer.Key()
			hash, height, err := dbFetchIndexerTip(dbTx, idxKey)
			if err != nil {
				return err
			}

			log.Debugf("Current %s tip (height %d, hash %v)",
				indexer.Name(), height, hash)
			indexerHeights[i] = height
			if height < lowestHeight {
				lowestHeight = height
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Nothing to index if all of the indexes are caught up.
	if lowestHeight == bestHeight {
		return nil
	}

	// Create a progress logger for the indexing process below.
	progressLogger := progresslog.NewBlockProgressLogger("Indexed", log)

	// At this point, one or more indexes are behind the current best chain
	// tip and need to be caught up, so log the details and loop through
	// each block that needs to be indexed.
	log.Infof("Catching up indexes from height %d to %d", lowestHeight,
		bestHeight)

	var cachedParent *dcrutil.Block
	for height := lowestHeight + 1; height <= bestHeight; height++ {
		if interruptRequested(ctx) {
			return errInterruptRequested
		}

		var block, parent *dcrutil.Block
		err = m.db.Update(func(dbTx database.Tx) error {
			// Get the parent of the block, unless it's already cached.
			if cachedParent == nil && height > 0 {
				parentHash, err := chain.BlockHashByHeight(int64(height - 1))
				if err != nil {
					return err
				}
				parent, err = dbFetchBlockByHash(dbTx, parentHash)
				if err != nil {
					return err
				}
			} else {
				parent = cachedParent
			}

			// Load the block for the height since it is required to index
			// it.
			hash, err := chain.BlockHashByHeight(int64(height))
			if err != nil {
				return err
			}
			block, err = dbFetchBlockByHash(dbTx, hash)
			if err != nil {
				return err
			}
			cachedParent = block

			if interruptRequested(ctx) {
				return errInterruptRequested
			}

			// Connect the block for all indexes that need it.
			var prevScripts PrevScripter
			for i, indexer := range m.enabledIndexes {
				// Skip indexes that don't need to be updated with this
				// block.
				if indexerHeights[i] >= height {
					continue
				}

				// When the index requires all of the referenced
				// txouts and they haven't been loaded yet, they
				// need to be retrieved from the transaction
				// index.
				if prevScripts == nil && indexNeedsInputs(indexer) {
					var errMakeView error
					prevScripts, errMakeView = chain.PrevScripts(dbTx, block)
					if errMakeView != nil {
						return errMakeView
					}
				}

				// Find out if treasury was enabled at the parent.
				isTreasuryEnabled, err := chain.IsTreasuryEnabled(parent.Hash())
				if err != nil {
					return err
				}
				err = dbIndexConnectBlock(dbTx, indexer, block,
					parent, prevScripts, isTreasuryEnabled)
				if err != nil {
					return err
				}

				indexerHeights[i] = height
			}

			return nil
		})
		if err != nil {
			return err
		}
		progressLogger.LogBlockHeight(block.MsgBlock(), parent.MsgBlock())
	}

	log.Infof("Indexes caught up to height %d", bestHeight)
	return nil
}

// indexNeedsInputs returns whether or not the index needs access to the txouts
// referenced by the transaction inputs being indexed.
func indexNeedsInputs(index Indexer) bool {
	if idx, ok := index.(NeedsInputser); ok {
		return idx.NeedsInputs()
	}

	return false
}

// ConnectBlock must be invoked when a block is extending the main chain.  It
// keeps track of the state of each index it is managing, performs some sanity
// checks, and invokes each indexer.
//
// This is part of the IndexManager interface.
func (m *Manager) ConnectBlock(dbTx database.Tx, block, parent *dcrutil.Block, prevScripts PrevScripter, isTreasuryEnabled bool) error {
	// Call each of the currently active optional indexes with the block
	// being connected so they can update accordingly.
	for _, index := range m.enabledIndexes {
		err := dbIndexConnectBlock(dbTx, index, block, parent,
			prevScripts, isTreasuryEnabled)
		if err != nil {
			return err
		}
	}
	return nil
}

// DisconnectBlock must be invoked when a block is being disconnected from the
// end of the main chain.  It keeps track of the state of each index it is
// managing, performs some sanity checks, and invokes each indexer to remove
// the index entries associated with the block.
//
// This is part of the IndexManager interface.
func (m *Manager) DisconnectBlock(dbTx database.Tx, block, parent *dcrutil.Block, prevScripts PrevScripter, isTreasuryEnabled bool) error {
	// Call each of the currently active optional indexes with the block
	// being disconnected so they can update accordingly.
	for _, index := range m.enabledIndexes {
		err := dbIndexDisconnectBlock(dbTx, index, block, parent,
			prevScripts, isTreasuryEnabled)
		if err != nil {
			return err
		}
	}
	return nil
}

// NewManager returns a new index manager with the provided indexes enabled.
//
// The manager returned satisfies the IndexManager interface and thus cleanly
// plugs into the normal blockchain processing path.
func NewManager(db database.DB, enabledIndexes []Indexer, params *chaincfg.Params) *Manager {
	return &Manager{
		db:             db,
		enabledIndexes: enabledIndexes,
		params:         params,
	}
}

// existsIndex returns whether the index keyed by idxKey exists in the database.
func existsIndex(db database.DB, idxKey []byte, idxName string) (bool, error) {
	var exists bool
	err := db.View(func(dbTx database.Tx) error {
		indexesBucket := dbTx.Metadata().Bucket(indexTipsBucketName)
		if indexesBucket != nil && indexesBucket.Get(idxKey) != nil {
			exists = true
		}
		return nil
	})
	return exists, err
}

// markIndexDeletion marks the index identified by idxKey for deletion.  Marking
// an index for deletion allows deletion to resume next startup if an
// incremental deletion was interrupted.
func markIndexDeletion(db database.DB, idxKey []byte) error {
	return db.Update(func(dbTx database.Tx) error {
		indexesBucket := dbTx.Metadata().Bucket(indexTipsBucketName)
		return indexesBucket.Put(indexDropKey(idxKey), idxKey)
	})
}

// incrementalFlatDrop uses multiple database updates to remove key/value pairs
// saved to a flat index.
func incrementalFlatDrop(ctx context.Context, db database.DB, idxKey []byte, idxName string) error {
	const maxDeletions = 2000000
	var totalDeleted uint64
	for numDeleted := maxDeletions; numDeleted == maxDeletions; {
		numDeleted = 0
		err := db.Update(func(dbTx database.Tx) error {
			bucket := dbTx.Metadata().Bucket(idxKey)
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
			log.Infof("Deleted %d keys (%d total) from %s",
				numDeleted, totalDeleted, idxName)
		}

		if interruptRequested(ctx) {
			return errInterruptRequested
		}
	}
	return nil
}

// dropIndexMetadata drops the passed index from the database by removing the
// top level bucket for the index, the index tip, and any in-progress drop flag.
func dropIndexMetadata(db database.DB, idxKey []byte, idxName string) error {
	return db.Update(func(dbTx database.Tx) error {
		meta := dbTx.Metadata()
		indexesBucket := meta.Bucket(indexTipsBucketName)
		err := indexesBucket.Delete(idxKey)
		if err != nil {
			return err
		}

		err = meta.DeleteBucket(idxKey)
		if !errors.Is(err, database.ErrBucketNotFound) {
			return err
		}

		err = indexesBucket.Delete(indexVersionKey(idxKey))
		if err != nil {
			return err
		}

		return indexesBucket.Delete(indexDropKey(idxKey))
	})
}

// dropFlatIndex incrementally drops the passed index from the database.  Since
// indexes can be massive, it deletes the index in multiple database
// transactions in order to keep memory usage to reasonable levels.  For this
// algorithm to work, the index must be "flat" (have no nested buckets).  It
// also marks the drop in progress so the drop can be resumed if it is stopped
// before it is done before the index can be used again.
func dropFlatIndex(ctx context.Context, db database.DB, idxKey []byte, idxName string) error {
	// Nothing to do if the index doesn't already exist.
	exists, err := existsIndex(db, idxKey, idxName)
	if err != nil {
		return err
	}
	if !exists {
		log.Infof("Not dropping %s because it does not exist", idxName)
		return nil
	}

	log.Infof("Dropping all %s entries.  This might take a while...",
		idxName)

	// Mark that the index is in the process of being dropped so that it
	// can be resumed on the next start if interrupted before the process is
	// complete.
	err = markIndexDeletion(db, idxKey)
	if err != nil {
		return err
	}

	// Since the indexes can be so large, attempting to simply delete
	// the bucket in a single database transaction would result in massive
	// memory usage and likely crash many systems due to ulimits.  In order
	// to avoid this, use a cursor to delete a maximum number of entries out
	// of the bucket at a time.
	err = incrementalFlatDrop(ctx, db, idxKey, idxName)
	if err != nil {
		return err
	}

	// Remove the index tip, version, bucket, and in-progress drop flag now that
	// all index entries have been removed.
	err = dropIndexMetadata(db, idxKey, idxName)
	if err != nil {
		return err
	}

	log.Infof("Dropped %s", idxName)
	return nil
}

// dropIndex drops the passed index from the database without using incremental
// deletion.  This should be used to drop indexes containing nested buckets,
// which can not be deleted with dropFlatIndex.
func dropIndex(db database.DB, idxKey []byte, idxName string) error {
	// Nothing to do if the index doesn't already exist.
	exists, err := existsIndex(db, idxKey, idxName)
	if err != nil {
		return err
	}
	if !exists {
		log.Infof("Not dropping %s because it does not exist", idxName)
		return nil
	}

	log.Infof("Dropping all %s entries.  This might take a while...",
		idxName)

	// Mark that the index is in the process of being dropped so that it
	// can be resumed on the next start if interrupted before the process is
	// complete.
	err = markIndexDeletion(db, idxKey)
	if err != nil {
		return err
	}

	// Remove the index tip, version, bucket, and in-progress drop flag.
	// Removing the index bucket also recursively removes all values saved to
	// the index.
	err = dropIndexMetadata(db, idxKey, idxName)
	if err != nil {
		return err
	}

	log.Infof("Dropped %s", idxName)
	return nil
}
