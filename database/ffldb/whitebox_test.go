// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file is part of the ffldb package rather than the ffldb_test package as
// it provides whitebox testing.

package ffldb

import (
	"bytes"
	"compress/bzip2"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/wire"
	"github.com/syndtr/goleveldb/leveldb"
	ldberrors "github.com/syndtr/goleveldb/leveldb/errors"
)

var (
	// blockDataNet is the expected network in the test block data.
	blockDataNet = wire.MainNet

	// blockDataFile is the path to a file containing the first 256 blocks
	// of the block chain.
	blockDataFile = filepath.Join("..", "testdata", "blocks0to168.bz2")

	// errSubTestFail is used to signal that a sub test returned false.
	errSubTestFail = fmt.Errorf("sub test failure")
)

// loadBlocks loads the blocks contained in the testdata directory and returns
// a slice of them.
func loadBlocks(t *testing.T, dataFile string, network wire.CurrencyNet) ([]*dcrutil.Block, error) {
	// Open the file that contains the blocks for reading.
	fi, err := os.Open(dataFile)
	if err != nil {
		t.Errorf("failed to open file %v, err %v", dataFile, err)
		return nil, err
	}
	defer func() {
		if err := fi.Close(); err != nil {
			t.Errorf("failed to close file %v %v", dataFile,
				err)
		}
	}()

	bcStream := bzip2.NewReader(fi)

	// Create a buffer of the read file.
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data.
	bcDecoder := gob.NewDecoder(bcBuf)
	blockChain := make(map[int64][]byte)

	// Decode the blockchain into the map.
	if err := bcDecoder.Decode(&blockChain); err != nil {
		t.Errorf("error decoding test blockchain: %v", err.Error())
	}

	// Fetch blocks 1 to 168 and perform various tests.
	blocks := make([]*dcrutil.Block, 169)
	for i := 0; i <= 168; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain[int64(i)])
		if err != nil {
			t.Errorf("NewBlockFromBytes error: %v", err.Error())
		}

		blocks[i] = bl
	}

	return blocks, nil
}

// checkDbError ensures the passed error is a database.Error that matches the
// passed error kind.
func checkDbError(t *testing.T, testName string, gotErr error, wantErr database.ErrorKind) bool {
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("%s: unexpected error - got %v, want %v",
			testName, gotErr, wantErr)
		return false
	}

	return true
}

// testContext is used to store context information about a running test which
// is passed into helper functions.
type testContext struct {
	t            *testing.T
	db           database.DB
	files        map[uint32]*lockableFile
	maxFileSizes map[uint32]int64
	blocks       []*dcrutil.Block
}

// TestConvertErr ensures the leveldb error to database error conversion works
// as expected.
func TestConvertErr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err         error
		wantErrKind database.ErrorKind
	}{
		{&ldberrors.ErrCorrupted{}, database.ErrCorruption},
		{leveldb.ErrClosed, database.ErrDbNotOpen},
		{leveldb.ErrSnapshotReleased, database.ErrTxClosed},
		{leveldb.ErrIterReleased, database.ErrTxClosed},
	}

	for i, test := range tests {
		gotErr := convertErr("test", test.err)
		if !errors.Is(gotErr, test.wantErrKind) {
			t.Errorf("convertErr #%d unexpected error - got %v, "+
				"want %v", i, gotErr, test.wantErrKind)
			continue
		}
	}
}

// TestCornerCases ensures several corner cases which can happen when opening
// a database and/or block files work as expected.
func TestCornerCases(t *testing.T) {
	t.Parallel()

	// Create a file at the database path to force the open below to fail.
	dbPath := filepath.Join(os.TempDir(), "ffldb-errors-v2")
	_ = os.RemoveAll(dbPath)
	fi, err := os.Create(dbPath)
	if err != nil {
		t.Errorf("os.Create: unexpected error: %v", err)
		return
	}
	fi.Close()

	// Ensure creating a new database fails when a file exists where a
	// directory is needed.
	testName := "openDB: fail due to file at target location"
	wantErrKind := database.ErrDriverSpecific
	idb, err := openDB(dbPath, blockDataNet, true)
	if !checkDbError(t, testName, err, wantErrKind) {
		if err == nil {
			idb.Close()
		}
		_ = os.RemoveAll(dbPath)
		return
	}

	// Remove the file and create the database to run tests against.  It
	// should be successful this time.
	_ = os.RemoveAll(dbPath)
	idb, err = openDB(dbPath, blockDataNet, true)
	if err != nil {
		t.Errorf("openDB: unexpected error: %v", err)
		return
	}
	defer os.RemoveAll(dbPath)
	defer idb.Close()

	// Ensure attempting to write to a file that can't be created returns
	// the expected error.
	testName = "writeBlock: open file failure"
	filePath := blockFilePath(dbPath, 0)
	if err := os.Mkdir(filePath, 0755); err != nil {
		t.Errorf("os.Mkdir: unexpected error: %v", err)
		return
	}
	store := idb.(*db).store
	_, err = store.writeBlock([]byte{0x00})
	if !checkDbError(t, testName, err, database.ErrDriverSpecific) {
		return
	}
	_ = os.RemoveAll(filePath)

	// Close the underlying leveldb database out from under the database.
	ldb := idb.(*db).cache.ldb
	ldb.Close()

	// Ensure initialization errors in the underlying database work as
	// expected.
	testName = "initDB: reinitialization"
	wantErrKind = database.ErrDbNotOpen
	err = initDB(ldb)
	if !checkDbError(t, testName, err, wantErrKind) {
		return
	}

	// Ensure the View handles errors in the underlying leveldb database
	// properly.
	testName = "View: underlying leveldb error"
	wantErrKind = database.ErrDbNotOpen
	err = idb.View(func(tx database.Tx) error {
		return nil
	})
	if !checkDbError(t, testName, err, wantErrKind) {
		return
	}

	// Ensure the Update handles errors in the underlying leveldb database
	// properly.
	testName = "Update: underlying leveldb error"
	err = idb.Update(func(tx database.Tx) error {
		return nil
	})
	if !checkDbError(t, testName, err, wantErrKind) {
		return
	}
}

// resetDatabase removes everything from the opened database associated with the
// test context including all metadata and the mock files.
func resetDatabase(tc *testContext) bool {
	// Reset the metadata.
	err := tc.db.Update(func(tx database.Tx) error {
		// Remove all the keys using a cursor while also generating a
		// list of buckets.  It's not safe to remove keys during ForEach
		// iteration nor is it safe to remove buckets during cursor
		// iteration, so this dual approach is needed.
		var bucketNames [][]byte
		cursor := tx.Metadata().Cursor()
		for ok := cursor.First(); ok; ok = cursor.Next() {
			if cursor.Value() != nil {
				if err := cursor.Delete(); err != nil {
					return err
				}
			} else {
				bucketNames = append(bucketNames, cursor.Key())
			}
		}

		// Remove the buckets.
		for _, k := range bucketNames {
			if err := tx.Metadata().DeleteBucket(k); err != nil {
				return err
			}
		}

		_, err := tx.Metadata().CreateBucket(blockIdxBucketName)
		return err
	})
	if err != nil {
		tc.t.Errorf("Update: unexpected error: %v", err)
		return false
	}

	// Reset the mock files.
	store := tc.db.(*db).store
	wc := store.writeCursor
	wc.curFile.Lock()
	if wc.curFile.file != nil {
		wc.curFile.file.Close()
		wc.curFile.file = nil
	}
	wc.curFile.Unlock()
	wc.Lock()
	wc.curFileNum = 0
	wc.curOffset = 0
	wc.Unlock()
	tc.files = make(map[uint32]*lockableFile)
	tc.maxFileSizes = make(map[uint32]int64)
	return true
}

// testWriteFailures tests various failures paths when writing to the block
// files.
func testWriteFailures(tc *testContext) bool {
	if !resetDatabase(tc) {
		return false
	}

	// Ensure file sync errors during flush return the expected error.
	store := tc.db.(*db).store
	testName := "flush: file sync failure"
	store.writeCursor.Lock()
	oldFile := store.writeCursor.curFile
	store.writeCursor.curFile = &lockableFile{
		file: &mockFile{forceSyncErr: true, maxSize: -1},
	}
	store.writeCursor.Unlock()
	err := tc.db.(*db).cache.flush()
	if !checkDbError(tc.t, testName, err, database.ErrDriverSpecific) {
		return false
	}
	store.writeCursor.Lock()
	store.writeCursor.curFile = oldFile
	store.writeCursor.Unlock()

	// Force errors in the various error paths when writing data by using
	// mock files with a limited max size.
	block0Bytes, _ := tc.blocks[0].Bytes()
	tests := []struct {
		fileNum uint32
		maxSize int64
	}{
		// Force an error when writing the network bytes.
		{fileNum: 0, maxSize: 2},

		// Force an error when writing the block size.
		{fileNum: 0, maxSize: 6},

		// Force an error when writing the block.
		{fileNum: 0, maxSize: 17},

		// Force an error when writing the checksum.
		{fileNum: 0, maxSize: int64(len(block0Bytes)) + 10},

		// Force an error after writing enough blocks for force multiple
		// files.
		{fileNum: 15, maxSize: 1},
	}

	for i, test := range tests {
		if !resetDatabase(tc) {
			return false
		}

		// Ensure storing the specified number of blocks using a mock
		// file that fails the write fails when the transaction is
		// committed, not when the block is stored.
		tc.maxFileSizes = map[uint32]int64{test.fileNum: test.maxSize}
		err := tc.db.Update(func(tx database.Tx) error {
			for i, block := range tc.blocks {
				err := tx.StoreBlock(block)
				if err != nil {
					tc.t.Errorf("StoreBlock (%d): unexpected "+
						"error: %v", i, err)
					return errSubTestFail
				}
			}

			return nil
		})
		testName := fmt.Sprintf("Force update commit failure - test "+
			"%d, fileNum %d, maxsize %d", i, test.fileNum,
			test.maxSize)
		if !checkDbError(tc.t, testName, err, database.ErrDriverSpecific) {
			tc.t.Errorf("%v", err)
			return false
		}

		// Ensure the commit rollback removed all extra files and data.
		if len(tc.files) != 1 {
			tc.t.Errorf("Update rollback: new not removed - want "+
				"1 file, got %d", len(tc.files))
			return false
		}
		if _, ok := tc.files[0]; !ok {
			tc.t.Error("Update rollback: file 0 does not exist")
			return false
		}
		file := tc.files[0].file.(*mockFile)
		if len(file.data) != 0 {
			tc.t.Errorf("Update rollback: file did not truncate - "+
				"want len 0, got len %d", len(file.data))
			return false
		}
	}

	return true
}

// testBlockFileErrors ensures the database returns expected errors with various
// file-related issues such as closed and missing files.
func testBlockFileErrors(tc *testContext) bool {
	if !resetDatabase(tc) {
		return false
	}

	// Ensure errors in blockFile and openFile when requesting invalid file
	// numbers.
	store := tc.db.(*db).store
	testName := "blockFile invalid file open"
	_, err := store.blockFile(^uint32(0))
	if !checkDbError(tc.t, testName, err, database.ErrDriverSpecific) {
		return false
	}
	testName = "openFile invalid file open"
	_, err = store.openFile(^uint32(0))
	if !checkDbError(tc.t, testName, err, database.ErrDriverSpecific) {
		return false
	}

	// Insert the first block into the mock file.
	err = tc.db.Update(func(tx database.Tx) error {
		err := tx.StoreBlock(tc.blocks[0])
		if err != nil {
			tc.t.Errorf("StoreBlock: unexpected error: %v", err)
			return errSubTestFail
		}

		return nil
	})
	if err != nil {
		if !errors.Is(err, errSubTestFail) {
			tc.t.Errorf("Update: unexpected error: %v", err)
		}
		return false
	}

	// Ensure errors in readBlock and readBlockRegion when requesting a file
	// number that doesn't exist.
	block0Hash := tc.blocks[0].Hash()
	testName = "readBlock invalid file number"
	invalidLoc := blockLocation{
		blockFileNum: ^uint32(0),
		blockLen:     80,
	}
	_, err = store.readBlock(block0Hash, invalidLoc)
	if !checkDbError(tc.t, testName, err, database.ErrDriverSpecific) {
		return false
	}
	testName = "readBlockRegion invalid file number"
	_, err = store.readBlockRegion(invalidLoc, 0, 80)
	if !checkDbError(tc.t, testName, err, database.ErrDriverSpecific) {
		return false
	}

	// Close the block file out from under the database.
	store.writeCursor.curFile.Lock()
	store.writeCursor.curFile.file.Close()
	store.writeCursor.curFile.Unlock()

	// Ensure failures in FetchBlock and FetchBlockRegion(s) since the
	// underlying file they need to read from has been closed.
	err = tc.db.View(func(tx database.Tx) error {
		testName = "FetchBlock closed file"
		wantErrKind := database.ErrDriverSpecific
		_, err := tx.FetchBlock(block0Hash)
		if !checkDbError(tc.t, testName, err, wantErrKind) {
			return errSubTestFail
		}

		testName = "FetchBlockRegion closed file"
		regions := []database.BlockRegion{
			{
				Hash:   block0Hash,
				Len:    80,
				Offset: 0,
			},
		}
		_, err = tx.FetchBlockRegion(&regions[0])
		if !checkDbError(tc.t, testName, err, wantErrKind) {
			return errSubTestFail
		}

		testName = "FetchBlockRegions closed file"
		_, err = tx.FetchBlockRegions(regions)
		if !checkDbError(tc.t, testName, err, wantErrKind) {
			return errSubTestFail
		}

		return nil
	})
	if err != nil {
		if !errors.Is(err, errSubTestFail) {
			tc.t.Errorf("View: unexpected error: %v", err)
		}
		return false
	}

	return true
}

// testCorruption ensures the database returns expected errors under various
// corruption scenarios.
func testCorruption(tc *testContext) bool {
	if !resetDatabase(tc) {
		return false
	}

	// Insert the first block into the mock file.
	err := tc.db.Update(func(tx database.Tx) error {
		err := tx.StoreBlock(tc.blocks[0])
		if err != nil {
			tc.t.Errorf("StoreBlock: unexpected error: %v", err)
			return errSubTestFail
		}

		return nil
	})
	if err != nil {
		if !errors.Is(err, errSubTestFail) {
			tc.t.Errorf("Update: unexpected error: %v", err)
		}
		return false
	}

	// Ensure corruption is detected by intentionally modifying the bytes
	// stored to the mock file and reading the block.
	block0Bytes, _ := tc.blocks[0].Bytes()
	block0Hash := tc.blocks[0].Hash()
	tests := []struct {
		offset      uint32
		fixChecksum bool
		wantErrKind database.ErrorKind
	}{
		// One of the network bytes.  The checksum needs to be fixed so
		// the invalid network is detected.
		{2, true, database.ErrDriverSpecific},

		// The same network byte, but this time don't fix the checksum
		// to ensure the corruption is detected.
		{2, false, database.ErrCorruption},

		// One of the block length bytes.
		{6, false, database.ErrCorruption},

		// Random header byte.
		{17, false, database.ErrCorruption},

		// Random transaction byte.
		{90, false, database.ErrCorruption},

		// Random checksum byte.
		{uint32(len(block0Bytes)) + 10, false, database.ErrCorruption},
	}
	err = tc.db.View(func(tx database.Tx) error {
		data := tc.files[0].file.(*mockFile).data
		for i, test := range tests {
			// Corrupt the byte at the offset by a single bit.
			data[test.offset] ^= 0x10

			// Fix the checksum if requested to force other errors.
			fileLen := len(data)
			var oldChecksumBytes [4]byte
			copy(oldChecksumBytes[:], data[fileLen-4:])
			if test.fixChecksum {
				toSum := data[:fileLen-4]
				cksum := crc32.Checksum(toSum, castagnoli)
				binary.BigEndian.PutUint32(data[fileLen-4:], cksum)
			}

			testName := fmt.Sprintf("FetchBlock (test #%d): "+
				"corruption", i)
			_, err := tx.FetchBlock(block0Hash)
			if !checkDbError(tc.t, testName, err, test.wantErrKind) {
				return errSubTestFail
			}

			// Reset the corrupted data back to the original.
			data[test.offset] ^= 0x10
			if test.fixChecksum {
				copy(data[fileLen-4:], oldChecksumBytes[:])
			}
		}

		return nil
	})
	if err != nil {
		if !errors.Is(err, errSubTestFail) {
			tc.t.Errorf("View: unexpected error: %v", err)
		}
		return false
	}

	return true
}

// TestFailureScenarios ensures several failure scenarios such as database
// corruption, block file write failures, and rollback failures are handled
// correctly.
func TestFailureScenarios(t *testing.T) {
	// Create a new database to run tests against.
	dbPath := filepath.Join(os.TempDir(), "ffldb-failurescenarios-v2")
	_ = os.RemoveAll(dbPath)
	idb, err := database.Create(dbType, dbPath, blockDataNet)
	if err != nil {
		t.Errorf("Failed to create test database (%s) %v", dbType, err)
		return
	}
	defer os.RemoveAll(dbPath)
	defer idb.Close()

	// Create a test context to pass around.
	tc := &testContext{
		t:            t,
		db:           idb,
		files:        make(map[uint32]*lockableFile),
		maxFileSizes: make(map[uint32]int64),
	}

	// Change the maximum file size to a small value to force multiple flat
	// files with the test data set and replace the file-related functions
	// to make use of mock files in memory.  This allows injection of
	// various file-related errors.
	store := idb.(*db).store
	store.maxBlockFileSize = 1024 // 1KiB
	store.openWriteFileFunc = func(fileNum uint32) (filer, error) {
		if file, ok := tc.files[fileNum]; ok {
			// "Reopen" the file.
			file.Lock()
			mock := file.file.(*mockFile)
			mock.Lock()
			mock.closed = false
			mock.Unlock()
			file.Unlock()
			return mock, nil
		}

		// Limit the max size of the mock file as specified in the test
		// context.
		maxSize := int64(-1)
		if maxFileSize, ok := tc.maxFileSizes[fileNum]; ok {
			maxSize = maxFileSize
		}
		file := &mockFile{maxSize: maxSize}
		tc.files[fileNum] = &lockableFile{file: file}
		return file, nil
	}
	store.openFileFunc = func(fileNum uint32) (*lockableFile, error) {
		// Force error when trying to open max file num.
		if fileNum == ^uint32(0) {
			return nil, makeDbErr(database.ErrDriverSpecific, "test")
		}
		if file, ok := tc.files[fileNum]; ok {
			// "Reopen" the file.
			file.Lock()
			mock := file.file.(*mockFile)
			mock.Lock()
			mock.closed = false
			mock.Unlock()
			file.Unlock()
			return file, nil
		}
		file := &lockableFile{file: &mockFile{}}
		tc.files[fileNum] = file
		return file, nil
	}
	store.deleteFileFunc = func(fileNum uint32) error {
		if file, ok := tc.files[fileNum]; ok {
			file.Lock()
			file.file.Close()
			file.Unlock()
			delete(tc.files, fileNum)
			return nil
		}

		str := fmt.Sprintf("file %d does not exist", fileNum)
		return makeDbErr(database.ErrDriverSpecific, str)
	}

	// Load the test blocks and save in the test context for use throughout
	// the tests.
	blocks, err := loadBlocks(t, blockDataFile, blockDataNet)
	if err != nil {
		t.Errorf("loadBlocks: Unexpected error: %v", err)
		return
	}
	tc.blocks = blocks

	// Test various failures paths when writing to the block files.
	if !testWriteFailures(tc) {
		return
	}

	// Test various file-related issues such as closed and missing files.
	if !testBlockFileErrors(tc) {
		return
	}

	// Test various corruption scenarios.
	testCorruption(tc)
}
