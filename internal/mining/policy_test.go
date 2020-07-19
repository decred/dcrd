// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// fromHex converts the passed hex string into a byte slice and will panic if
// there is an error.  This is only provided for the hard-coded constants so
// errors in the source code can be detected. It will only (and must only) be
// called for initialization purposes.
func fromHex(s string) []byte {
	r, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	return r
}

// mockPrioInputSourceEntry houses a block height and amount for use when
// associating them to a given transaction output.
type mockPrioInputSourceEntry struct {
	height int64
	amount int64
}

// mockPrioInputSource provides a source of transaction output block heights and
// amounts for given outpoints and implements the PriorityInputser interface so
// it may be used in cases that require access to said information.
type mockPrioInputSource map[wire.OutPoint]mockPrioInputSourceEntry

// PriorityInput returns the block height and amount associated with the
// provided previous outpoint along with a bool that indicates whether or not
// the requested entry exists.  This ensures the caller is able to distinguish
// missing entries from zero values.
func (m mockPrioInputSource) PriorityInput(prevOut *wire.OutPoint) (int64, int64, bool) {
	entry, ok := m[*prevOut]
	if !ok {
		return 0, 0, false
	}

	return entry.height, entry.amount, true
}

// TestCheckTransactionStandard tests the checkTransactionStandard API.
func TestCalcPriority(t *testing.T) {
	// Create some dummy, but otherwise standard, data for transactions.
	prevOutHash, err := chainhash.NewHashFromStr("01")
	if err != nil {
		t.Fatalf("NewHashFromStr: unexpected error: %v", err)
	}
	dummyPrevOut := wire.OutPoint{Hash: *prevOutHash, Index: 1, Tree: 0}
	dummySigScript := bytes.Repeat([]byte{0x00}, 107)
	dummyTxIn := wire.TxIn{
		PreviousOutPoint: dummyPrevOut,
		Sequence:         wire.MaxTxInSequenceNum,
		ValueIn:          10000,
		BlockHeight:      150000,
		BlockIndex:       2,
		SignatureScript:  dummySigScript,
	}
	dummySigScriptP2SH := bytes.Repeat([]byte{0x00}, 145)
	dummyPrevOutP2SH := wire.OutPoint{Hash: *prevOutHash, Index: 1, Tree: 0}
	dummyPrevOutP2SH.Hash[0] = 0x02
	dummyTxInP2SH := wire.TxIn{
		PreviousOutPoint: dummyPrevOutP2SH,
		Sequence:         wire.MaxTxInSequenceNum,
		ValueIn:          20000,
		BlockHeight:      149950,
		BlockIndex:       2,
		SignatureScript:  dummySigScriptP2SH,
	}
	dummyP2PKHScript := fromHex("76a914000000000000000000000000000000000000000088ac")
	dummyTxOut := wire.TxOut{
		Value:    dummyTxIn.ValueIn - 3000, // Use 3000 atoms for dummy fee.
		Version:  0,
		PkScript: dummyP2PKHScript,
	}
	dummyTxOutP2SH := wire.TxOut{
		Value:    dummyTxInP2SH.ValueIn - 3000, // Use 3000 atoms for dummy fee.
		Version:  0,
		PkScript: dummyP2PKHScript,
	}

	tests := []struct {
		name       string
		tx         wire.MsgTx
		prioInputs mockPrioInputSource
		nextHeight int64
		wantSize   int
		want       float64
	}{{
		name: "p2pkh spend (input age 100) with one output",
		tx: wire.MsgTx{
			SerType:  wire.TxSerializeFull,
			Version:  1,
			TxIn:     []*wire.TxIn{&dummyTxIn},
			TxOut:    []*wire.TxOut{&dummyTxOut},
			LockTime: 0,
		},
		prioInputs: mockPrioInputSource{
			dummyPrevOut: mockPrioInputSourceEntry{
				height: int64(dummyTxIn.BlockHeight),
				amount: dummyTxIn.ValueIn,
			}},
		nextHeight: int64(dummyTxIn.BlockHeight) + 100,
		wantSize:   216,
		want:       19607.843137254902,
	}, {
		name: "p2pkh spend (input age 100) with two outputs",
		tx: wire.MsgTx{
			SerType: wire.TxSerializeFull,
			Version: 1,
			TxIn:    []*wire.TxIn{&dummyTxIn},
			TxOut: func() []*wire.TxOut {
				dummyTxOut1 := dummyTxOut
				dummyTxOut1.Value = dummyTxIn.ValueIn/2 - 1500
				dummyTxOut2 := dummyTxOut
				dummyTxOut2.Value = dummyTxIn.ValueIn/2 - 1500
				return []*wire.TxOut{&dummyTxOut1, &dummyTxOut2}
			}(),
			LockTime: 0,
		},
		prioInputs: mockPrioInputSource{
			dummyPrevOut: mockPrioInputSourceEntry{
				height: int64(dummyTxIn.BlockHeight),
				amount: dummyTxIn.ValueIn,
			}},
		nextHeight: int64(dummyTxIn.BlockHeight) + 100,
		wantSize:   252,
		want:       11494.252873563218,
	}, {
		name: "p2pkh spend (input age 350) with one output",
		tx: wire.MsgTx{
			SerType:  wire.TxSerializeFull,
			Version:  1,
			TxIn:     []*wire.TxIn{&dummyTxIn},
			TxOut:    []*wire.TxOut{&dummyTxOut},
			LockTime: 0,
		},
		prioInputs: mockPrioInputSource{
			dummyPrevOut: mockPrioInputSourceEntry{
				height: int64(dummyTxIn.BlockHeight),
				amount: dummyTxIn.ValueIn,
			}},
		nextHeight: int64(dummyTxIn.BlockHeight) + 350,
		wantSize:   216,
		want:       68627.45098039215,
	}, {
		name: "p2pkh spend (input age 350) with two outputs",
		tx: wire.MsgTx{
			SerType: wire.TxSerializeFull,
			Version: 1,
			TxIn:    []*wire.TxIn{&dummyTxIn},
			TxOut: func() []*wire.TxOut {
				dummyTxOut1 := dummyTxOut
				dummyTxOut1.Value = dummyTxIn.ValueIn/2 - 1500
				dummyTxOut2 := dummyTxOut
				dummyTxOut2.Value = dummyTxIn.ValueIn/2 - 1500
				return []*wire.TxOut{&dummyTxOut1, &dummyTxOut2}
			}(),
			LockTime: 0,
		},
		prioInputs: mockPrioInputSource{
			dummyPrevOut: mockPrioInputSourceEntry{
				height: int64(dummyTxIn.BlockHeight),
				amount: dummyTxIn.ValueIn,
			}},
		nextHeight: int64(dummyTxIn.BlockHeight) + 350,
		wantSize:   252,
		want:       40229.88505747126,
	}, {
		name: "p2sh spend (input age 50) with one output",
		tx: wire.MsgTx{
			SerType:  wire.TxSerializeFull,
			Version:  1,
			TxIn:     []*wire.TxIn{&dummyTxInP2SH},
			TxOut:    []*wire.TxOut{&dummyTxOutP2SH},
			LockTime: 0,
		},
		prioInputs: mockPrioInputSource{
			dummyPrevOutP2SH: mockPrioInputSourceEntry{
				height: int64(dummyTxInP2SH.BlockHeight),
				amount: dummyTxInP2SH.ValueIn,
			}},
		nextHeight: int64(dummyTxInP2SH.BlockHeight) + 50,
		wantSize:   254,
		want:       11627.906976744186,
	}, {
		name: "p2pkh and p2sh spends (input age 50 and 100) with one output",
		tx: wire.MsgTx{
			SerType: wire.TxSerializeFull,
			Version: 1,
			TxIn: func() []*wire.TxIn {
				return []*wire.TxIn{&dummyTxIn, &dummyTxInP2SH}
			}(),
			TxOut:    []*wire.TxOut{&dummyTxOutP2SH},
			LockTime: 0,
		},
		prioInputs: mockPrioInputSource{
			dummyPrevOut: mockPrioInputSourceEntry{
				height: int64(dummyTxIn.BlockHeight),
				amount: dummyTxIn.ValueIn,
			},
			dummyPrevOutP2SH: mockPrioInputSourceEntry{
				height: int64(dummyTxInP2SH.BlockHeight),
				amount: dummyTxInP2SH.ValueIn,
			}},
		nextHeight: int64(dummyTxIn.BlockHeight) + 50,
		wantSize:   419,
		want:       29069.767441860465,
	}, {
		name: "p2pkh and p2sh spends (input age 50 and 100) with one output",
		tx: wire.MsgTx{
			SerType: wire.TxSerializeFull,
			Version: 1,
			TxIn: func() []*wire.TxIn {
				return []*wire.TxIn{&dummyTxIn, &dummyTxInP2SH}
			}(),
			TxOut:    []*wire.TxOut{&dummyTxOutP2SH},
			LockTime: 0,
		},
		prioInputs: mockPrioInputSource{
			dummyPrevOut: mockPrioInputSourceEntry{
				height: int64(dummyTxIn.BlockHeight),
				amount: dummyTxIn.ValueIn,
			},
			dummyPrevOutP2SH: mockPrioInputSourceEntry{
				height: int64(dummyTxInP2SH.BlockHeight),
				amount: dummyTxInP2SH.ValueIn,
			}},
		nextHeight: int64(dummyTxIn.BlockHeight) + 50,
		wantSize:   419,
		want:       29069.767441860465,
	}}

	for _, test := range tests {
		// Ensure the serialized transaction size matches hardcoded values to
		// help ensure the related overhead values are updated properly if the
		// transaction serialization format changes.
		gotSize := test.tx.SerializeSize()
		if gotSize != test.wantSize {
			t.Fatalf("%q: unexpected tx size -- got %d, want %d", test.name,
				gotSize, test.wantSize)
		}

		// Ensure the calculate priority is the expected value.
		got := CalcPriority(&test.tx, test.prioInputs, test.nextHeight)
		if got != test.want {
			t.Fatalf("%q: unexpected priority -- got %v, want %v", test.name,
				got, test.want)
		}
	}
}
