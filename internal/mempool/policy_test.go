// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2016-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/txscript/v4/stdscript"
	"github.com/decred/dcrd/wire"
)

const (
	// noTreasury signifies the treasury agenda should be treated as though it
	// is inactive.  It is used to increase the readability of the tests.
	noTreasury = false
)

// TestCalcMinRequiredTxRelayFee tests the calcMinRequiredTxRelayFee API.
func TestCalcMinRequiredTxRelayFee(t *testing.T) {
	tests := []struct {
		name     string         // test description.
		size     int64          // Transaction size in bytes.
		relayFee dcrutil.Amount // minimum relay transaction fee.
		want     int64          // Expected fee.
	}{
		{
			// Ensure combination of size and fee that are less than
			// 1000 produce a non-zero fee.
			"250 bytes with relay fee of 3",
			250,
			3,
			3,
		},
		{
			"1000 bytes with default minimum relay fee",
			1000,
			DefaultMinRelayTxFee,
			1e4,
		},
		{
			"max standard tx size with default minimum relay fee",
			MaxStandardTxSize,
			DefaultMinRelayTxFee,
			1e6,
		},
		{
			"max standard tx size with max relay fee",
			MaxStandardTxSize,
			dcrutil.MaxAmount,
			dcrutil.MaxAmount,
		},
		{
			"1500 bytes with 5000 relay fee",
			1500,
			5000,
			7500,
		},
		{
			"1500 bytes with 3000 relay fee",
			1500,
			3000,
			4500,
		},
		{
			"782 bytes with 5000 relay fee",
			782,
			5000,
			3910,
		},
		{
			"782 bytes with 3000 relay fee",
			782,
			3000,
			2346,
		},
		{
			"782 bytes with 2550 relay fee",
			782,
			2550,
			1994,
		},
	}

	for _, test := range tests {
		got := calcMinRequiredTxRelayFee(test.size, test.relayFee)
		if got != test.want {
			t.Errorf("TestCalcMinRequiredTxRelayFee test '%s' "+
				"failed: got %v want %v", test.name, got,
				test.want)
			continue
		}
	}
}

// TestCheckPkScriptStandard tests the checkPkScriptStandard API.
func TestCheckPkScriptStandard(t *testing.T) {
	var pubKeys [][]byte
	for i := 0; i < 4; i++ {
		pk := secp256k1.NewPrivateKey(new(secp256k1.ModNScalar).SetInt(0))
		pubKeys = append(pubKeys, pk.PubKey().SerializeCompressed())
	}

	tests := []struct {
		name       string // test description.
		script     *txscript.ScriptBuilder
		isStandard bool
	}{
		{
			"key1 and key2",
			txscript.NewScriptBuilder().AddOp(txscript.OP_2).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_2).AddOp(txscript.OP_CHECKMULTISIG),
			true,
		},
		{
			"key1 or key2",
			txscript.NewScriptBuilder().AddOp(txscript.OP_1).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_2).AddOp(txscript.OP_CHECKMULTISIG),
			true,
		},
		{
			"escrow",
			txscript.NewScriptBuilder().AddOp(txscript.OP_2).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddData(pubKeys[2]).
				AddOp(txscript.OP_3).AddOp(txscript.OP_CHECKMULTISIG),
			true,
		},
		{
			"one of four",
			txscript.NewScriptBuilder().AddOp(txscript.OP_1).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddData(pubKeys[2]).AddData(pubKeys[3]).
				AddOp(txscript.OP_4).AddOp(txscript.OP_CHECKMULTISIG),
			false,
		},
		{
			"malformed1",
			txscript.NewScriptBuilder().AddOp(txscript.OP_3).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_2).AddOp(txscript.OP_CHECKMULTISIG),
			false,
		},
		{
			"malformed2",
			txscript.NewScriptBuilder().AddOp(txscript.OP_2).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_3).AddOp(txscript.OP_CHECKMULTISIG),
			false,
		},
		{
			"malformed3",
			txscript.NewScriptBuilder().AddOp(txscript.OP_0).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_2).AddOp(txscript.OP_CHECKMULTISIG),
			false,
		},
		{
			"malformed4",
			txscript.NewScriptBuilder().AddOp(txscript.OP_1).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_0).AddOp(txscript.OP_CHECKMULTISIG),
			false,
		},
		{
			"malformed5",
			txscript.NewScriptBuilder().AddOp(txscript.OP_1).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_CHECKMULTISIG),
			false,
		},
		{
			"malformed6",
			txscript.NewScriptBuilder().AddOp(txscript.OP_1).
				AddData(pubKeys[0]).AddData(pubKeys[1]),
			false,
		},
	}

	for _, test := range tests {
		script, err := test.script.Script()
		if err != nil {
			t.Fatalf("TestCheckPkScriptStandard test '%s' "+
				"failed: %v", test.name, err)
		}
		scriptType := stdscript.DetermineScriptType(0, script)
		got := checkPkScriptStandard(0, script, scriptType)
		if (test.isStandard && got != nil) ||
			(!test.isStandard && got == nil) {

			t.Fatalf("TestCheckPkScriptStandard test '%s' failed",
				test.name)
		}
	}
}

// TestDust tests the isDust API.
func TestDust(t *testing.T) {
	pkScript := []byte{0x76, 0xa9, 0x14, 0xb1, 0x2d, 0x0f, 0xca,
		0xeb, 0x46, 0x14, 0xa3, 0x4b, 0x1e, 0x88, 0x61, 0xe7,
		0x55, 0x4f, 0xd4, 0x13, 0xf7, 0xa6, 0x47, 0x88, 0xac}

	tests := []struct {
		name     string // test description
		txOut    wire.TxOut
		relayFee dcrutil.Amount // minimum relay transaction fee.
		isDust   bool
	}{
		{
			// Any value is allowed with a zero relay fee.
			"zero value with zero relay fee",
			wire.TxOut{Value: 0, Version: 0, PkScript: pkScript},
			0,
			true,
		},
		{
			// Zero value is dust with any relay fee"
			"zero value with very small tx fee",
			wire.TxOut{Value: 0, Version: 0, PkScript: pkScript},
			1,
			true,
		},
		{
			"25 byte public key script with value 602, relay fee 1e3",
			wire.TxOut{Value: 602, Version: 0, PkScript: pkScript},
			1000,
			true,
		},
		{
			"25 byte public key script with value 603, relay fee 1e3",
			wire.TxOut{Value: 603, Version: 0, PkScript: pkScript},
			1000,
			false,
		},
		{
			"25 byte public key script with value 60299, relay fee 1e5",
			wire.TxOut{Value: 60299, Version: 0, PkScript: pkScript},
			1e5,
			true,
		},
		{
			"25 byte public key script with value 60300, relay fee 1e5",
			wire.TxOut{Value: 60300, Version: 0, PkScript: pkScript},
			1e5,
			false,
		},
		{
			"25 byte public key script with value 6029, relay fee 1e4",
			wire.TxOut{Value: 6029, Version: 0, PkScript: pkScript},
			1e4,
			true,
		},
		{
			"25 byte public key script with value 6030, relay fee 1e4",
			wire.TxOut{Value: 6030, Version: 0, PkScript: pkScript},
			1e4,
			false,
		},
		{
			// Maximum allowed value is never dust.
			"max amount is never dust",
			wire.TxOut{Value: dcrutil.MaxAmount, Version: 0, PkScript: pkScript},
			dcrutil.MaxAmount,
			false,
		},
		{
			// Maximum int64 value causes overflow.
			"maximum int64 value",
			wire.TxOut{Value: 1<<63 - 1, Version: 0, PkScript: pkScript},
			1<<63 - 1,
			true,
		},
		{
			// Unspendable pkScript due to an invalid public key
			// script.
			"unspendable pkScript",
			wire.TxOut{Value: 5000, Version: 0, PkScript: []byte{0x01}},
			0, // no relay fee
			true,
		},
	}
	for _, test := range tests {
		res := isDust(&test.txOut, test.relayFee)
		if res != test.isDust {
			t.Fatalf("Dust test '%s' failed: want %v got %v",
				test.name, test.isDust, res)
		}
	}
}

// TestCheckTransactionStandard tests the checkTransactionStandard API.
func TestCheckTransactionStandard(t *testing.T) {
	// Create some dummy, but otherwise standard, data for transactions.
	prevOutHash, err := chainhash.NewHashFromStr("01")
	if err != nil {
		t.Fatalf("NewHashFromStr: unexpected error: %v", err)
	}
	dummyPrevOut := wire.OutPoint{Hash: *prevOutHash, Index: 1, Tree: 0}
	dummySigScript := bytes.Repeat([]byte{0x00}, 65)
	dummyTxIn := wire.TxIn{
		PreviousOutPoint: dummyPrevOut,
		Sequence:         wire.MaxTxInSequenceNum,
		ValueIn:          0,
		BlockHeight:      0,
		BlockIndex:       0,
		SignatureScript:  dummySigScript,
	}
	addrHash := [20]byte{0x01}
	addr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(addrHash[:],
		chaincfg.RegNetParams())
	if err != nil {
		t.Fatalf("NewAddressPubKeyHash: unexpected error: %v", err)
	}
	dummyPkScriptVer, dummyPkScript := addr.PaymentScript()
	dummyTxOut := wire.TxOut{
		Value:    100000000, // 1 BTC
		Version:  dummyPkScriptVer,
		PkScript: dummyPkScript,
	}

	tests := []struct {
		name       string
		tx         wire.MsgTx
		height     int64
		isStandard bool
		err        error
	}{
		{
			name: "Typical pay-to-pubkey-hash transaction",
			tx: wire.MsgTx{
				SerType:  wire.TxSerializeFull,
				Version:  1,
				TxIn:     []*wire.TxIn{&dummyTxIn},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
		{
			name: "Transaction serialize type not full",
			tx: wire.MsgTx{
				SerType:  wire.TxSerializeNoWitness,
				Version:  1,
				TxIn:     []*wire.TxIn{&dummyTxIn},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			err:        ErrNonStandard,
		},
		{
			name: "Transaction is not finalized",
			tx: wire.MsgTx{
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: dummyPrevOut,
					SignatureScript:  dummySigScript,
					Sequence:         0,
				}},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 300001,
			},
			height:     300000,
			isStandard: false,
			err:        ErrNonStandard,
		},
		{
			name: "Transaction size is too large",
			tx: wire.MsgTx{
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value: 0,
					PkScript: bytes.Repeat([]byte{0x00},
						MaxStandardTxSize+1),
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			err:        ErrNonStandard,
		},
		{
			name: "Signature script size is too large",
			tx: wire.MsgTx{
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: dummyPrevOut,
					SignatureScript: bytes.Repeat([]byte{0x00},
						maxStandardSigScriptSize+1),
					Sequence: wire.MaxTxInSequenceNum,
				}},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			err:        ErrNonStandard,
		},
		{
			name: "Signature script that does more than push data",
			tx: wire.MsgTx{
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: dummyPrevOut,
					SignatureScript: []byte{
						txscript.OP_CHECKSIGVERIFY},
					Sequence: wire.MaxTxInSequenceNum,
				}},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			err:        ErrNonStandard,
		},
		{
			name: "Valid but non standard public key script",
			tx: wire.MsgTx{
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    100000000,
					PkScript: []byte{txscript.OP_TRUE},
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			err:        ErrNonStandard,
		},
		{
			name: "More than four nulldata outputs",
			tx: wire.MsgTx{
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}, {
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}, {
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}, {
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}, {
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			err:        ErrNonStandard,
		},
		{
			name: "Dust output",
			tx: wire.MsgTx{
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    0,
					PkScript: dummyPkScript,
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			err:        ErrDustOutput,
		},
		{
			name: "One nulldata output with 0 amount (standard)",
			tx: wire.MsgTx{
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
	}

	medianTime := time.Now()
	for _, test := range tests {
		// Ensure standardness is as expected.
		txType := stake.DetermineTxType(&test.tx)
		tx := dcrutil.NewTx(&test.tx)
		err := checkTransactionStandard(tx, txType, test.height, medianTime,
			DefaultMinRelayTxFee)
		if err == nil && test.isStandard {
			// Test passes since function returned standard for a
			// transaction which is intended to be standard.
			continue
		}
		if err == nil && !test.isStandard {
			t.Errorf("checkTransactionStandard (%s): standard when "+
				"it should not be", test.name)
			continue
		}
		if err != nil && test.isStandard {
			t.Errorf("checkTransactionStandard (%s): nonstandard "+
				"when it should not be: %v", test.name, err)
			continue
		}

		// Ensure the error is the expected one.
		if !errors.Is(err, test.err) {
			t.Errorf("checkTransactionStandard (%s): unexpected error -- got "+
				"%v, want %v", test.name, err, test.err)
			continue
		}
	}
}
