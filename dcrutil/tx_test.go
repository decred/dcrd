// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// TestTx tests the API for Tx.
func TestTx(t *testing.T) {
	testTx := Block100000.Transactions[0]
	tx := NewTx(testTx)

	// Ensure we get the same data back out.
	if msgTx := tx.MsgTx(); !reflect.DeepEqual(msgTx, testTx) {
		t.Errorf("MsgTx: mismatched MsgTx - got %v, want %v",
			spew.Sdump(msgTx), spew.Sdump(testTx))
	}

	// Ensure transaction index set and get work properly.
	wantIndex := 0
	tx.SetIndex(0)
	if gotIndex := tx.Index(); gotIndex != wantIndex {
		t.Errorf("Index: mismatched index - got %v, want %v",
			gotIndex, wantIndex)
	}

	// Ensure tree type set and get work properly.
	wantTree := int8(0)
	tx.SetTree(0)
	if gotTree := tx.Tree(); gotTree != wantTree {
		t.Errorf("Index: mismatched index - got %v, want %v",
			gotTree, wantTree)
	}

	// Ensure stake transaction index set and get work properly.
	wantIndex = 0
	tx.SetIndex(0)
	if gotIndex := tx.Index(); gotIndex != wantIndex {
		t.Errorf("Index: mismatched index - got %v, want %v",
			gotIndex, wantIndex)
	}

	// Ensure tree type set and get work properly.
	wantTree = int8(1)
	tx.SetTree(1)
	if gotTree := tx.Tree(); gotTree != wantTree {
		t.Errorf("Index: mismatched index - got %v, want %v",
			gotTree, wantTree)
	}

	// Hash for block 100,000 transaction 0.
	wantHashStr := "1cbd9fe1a143a265cc819ff9d8132a7cbc4ca48eb68c0de39cfdf7ecf42cbbd1"
	wantHash, err := chainhash.NewHashFromStr(wantHashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
	}

	// Request the hash multiple times to test generation and caching.
	for i := 0; i < 2; i++ {
		hash := tx.Hash()
		if !hash.IsEqual(wantHash) {
			t.Errorf("Hash #%d mismatched hash - got %v, want %v", i,
				hash, wantHash)
		}
	}
}

// TestNewTxFromBytes tests creation of a Tx from serialized bytes.
func TestNewTxFromBytes(t *testing.T) {
	// Serialize the test transaction.
	testTx := Block100000.Transactions[0]
	var testTxBuf bytes.Buffer
	testTxBuf.Grow(testTx.SerializeSize())
	err := testTx.Serialize(&testTxBuf)
	if err != nil {
		t.Errorf("Serialize: %v", err)
	}
	testTxBytes := testTxBuf.Bytes()

	// Create a new transaction from the serialized bytes.
	tx, err := NewTxFromBytes(testTxBytes)
	if err != nil {
		t.Errorf("NewTxFromBytes: %v", err)
		return
	}

	// Ensure the generated MsgTx is correct.
	if msgTx := tx.MsgTx(); !reflect.DeepEqual(msgTx, testTx) {
		t.Errorf("MsgTx: mismatched MsgTx - got %v, want %v",
			spew.Sdump(msgTx), spew.Sdump(testTx))
	}
}

// TestTxErrors tests the error paths for the Tx API.
func TestTxErrors(t *testing.T) {
	// Serialize the test transaction.
	testTx := Block100000.Transactions[0]
	var testTxBuf bytes.Buffer
	testTxBuf.Grow(testTx.SerializeSize())
	err := testTx.Serialize(&testTxBuf)
	if err != nil {
		t.Errorf("Serialize: %v", err)
	}
	testTxBytes := testTxBuf.Bytes()

	// Truncate the transaction byte buffer to force errors.
	shortBytes := testTxBytes[:4]
	_, err = NewTxFromBytes(shortBytes)
	if !errors.Is(err, io.EOF) {
		t.Errorf("NewTxFromBytes: did not get expected error - "+
			"got %v, want %v", err, io.EOF)
	}
}

// TestNewTxDeep tests the API for Tx deep copy.
func TestNewTxDeep(t *testing.T) {
	orgTx := Block100000.Transactions[0]
	copyTxDeep := NewTxDeep(orgTx)
	copyTx := copyTxDeep.MsgTx()

	// Ensure original and copied has equal values to original transaction.
	if !reflect.DeepEqual(orgTx, copyTx) {
		t.Fatalf("MsgTx is not equal - got %v, want %v",
			spew.Sdump(copyTx), spew.Sdump(&orgTx))
	}

	// Ensure original and copied transaction referring to different allocations.
	if orgTx == copyTx {
		t.Fatal("MsgTx is referring to the same allocation")
	}

	// Compare each original and copied input transaction allocations.
	for i := 0; i < len(orgTx.TxIn); i++ {
		// Ensure input transactions referring to different allocations.
		if orgTx.TxIn[i] == copyTx.TxIn[i] {
			t.Errorf("TxIn #%d is referring to the same allocation", i)
		}

		// Ensure previous transaction output points referring to different
		// allocations.
		if &orgTx.TxIn[i].PreviousOutPoint == &copyTx.TxIn[i].PreviousOutPoint {
			t.Errorf("PreviousOutPoint #%d is referring to the same allocation", i)
		}

		// Ensure signature scripts referring to different allocations.
		if &orgTx.TxIn[i].SignatureScript[0] == &copyTx.TxIn[i].SignatureScript[0] {
			t.Errorf("SignatureScript #%d is referring to the same allocation", i)
		}
	}

	// Compare each original and copied output transaction allocations.
	for i := 0; i < len(orgTx.TxOut); i++ {
		// Ensure output transactions referring to different allocations.
		if orgTx.TxOut[i] == copyTx.TxOut[i] {
			t.Errorf("TxOut #%d is referring to the same allocation", i)
		}

		// Ensure PkScripts referring to different allocations.
		if &orgTx.TxOut[i].PkScript[0] == &copyTx.TxOut[i].PkScript[0] {
			t.Errorf("PkScript #%d is referring to the same allocation", i)
		}
	}
}

// TestNewTxDeepTxIns tests the API for creation of a Tx with deep TxIn copy.
func TestNewTxDeepTxIns(t *testing.T) {
	msgTx := Block100000.Transactions[0]

	// Create a new Tx with an underlying wire.MsgTx and set the tree and index
	// to ensure that those are copied over as well.
	tx := NewTx(msgTx)
	tx.SetTree(wire.TxTreeRegular)
	tx.SetIndex(0)

	// Ensure original and copied transactions has equal values.
	copyTxDeep := NewTxDeepTxIns(tx)
	if !reflect.DeepEqual(tx, copyTxDeep) {
		t.Fatalf("Tx is not equal - got %v, want %v",
			spew.Sdump(copyTxDeep), spew.Sdump(&tx))
	}

	// Ensure original and copied transactions refer to different allocations.
	cMsgTx := copyTxDeep.MsgTx()
	if msgTx == cMsgTx {
		t.Fatal("MsgTx is referring to the same allocation")
	}

	// Compare each original and copied input transaction allocations.
	for i := 0; i < len(msgTx.TxIn); i++ {
		// Ensure input transactions refer to different allocations.
		if msgTx.TxIn[i] == cMsgTx.TxIn[i] {
			t.Errorf("TxIn #%d is referring to the same allocation", i)
		}

		// Ensure previous transaction output points refer to different
		// allocations.
		if &msgTx.TxIn[i].PreviousOutPoint == &cMsgTx.TxIn[i].PreviousOutPoint {
			t.Errorf("PreviousOutPoint #%d is referring to the same"+
				" allocation", i)
		}

		// Ensure signature scripts refer to different allocations.
		if &msgTx.TxIn[i].SignatureScript[0] == &cMsgTx.TxIn[i].SignatureScript[0] {
			t.Errorf("SignatureScript #%d is referring to the same"+
				" allocation", i)
		}
	}

	// Compare each original and copied output transaction allocations.
	for i := 0; i < len(msgTx.TxOut); i++ {
		// Ensure output transactions refer to same allocation.
		if msgTx.TxOut[i] != cMsgTx.TxOut[i] {
			t.Errorf("TxOut #%d is not referring to same allocation", i)
		}
	}
}
