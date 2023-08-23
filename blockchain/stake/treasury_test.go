// Copyright (c) 2020-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"math/rand"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

// Private and public keys for tests.
var (
	// Serialized private key.
	//privateKey = []byte{
	//	0x76, 0x87, 0x56, 0x13, 0x94, 0xcc, 0xc6, 0x11,
	//	0x01, 0x51, 0xbd, 0x9f, 0x26, 0xd4, 0x22, 0x8e,
	//	0xb2, 0xd5, 0x7b, 0xe1, 0x28, 0xc0, 0x36, 0x12,
	//	0xe3, 0x9a, 0x84, 0x4a, 0x3e, 0xcd, 0x3c, 0xcf,
	//}

	// Serialized compressed public key
	publicKey = []byte{
		0x02, 0xa4, 0xf6, 0x45, 0x86, 0xe1, 0x72, 0xc3,
		0xd9, 0xa2, 0x0c, 0xfa, 0x6c, 0x7a, 0xc8, 0xfb,
		0x12, 0xf0, 0x11, 0x5b, 0x3f, 0x69, 0xc3, 0xc3,
		0x5a, 0xec, 0x93, 0x3a, 0x4c, 0x47, 0xc7, 0xd9,
		0x2c,
	}

	// Valid signature of chainhash.HashB([]byte("test message"))
	validSignature = []byte{
		0x77, 0x69, 0x84, 0xf6, 0x83, 0x13, 0xb1, 0xac,
		0x62, 0x9e, 0x62, 0x4a, 0xf0, 0x59, 0x5b, 0xdc,
		0x09, 0xd8, 0xde, 0xd0, 0x2b, 0xc2, 0xb2, 0x9f,
		0xbd, 0xb3, 0x95, 0x95, 0xe0, 0x3a, 0xc8, 0xb0,
		0xcf, 0x81, 0x8c, 0xa5, 0x36, 0x72, 0x3e, 0x63,
		0x90, 0xd3, 0x08, 0x4e, 0x0e, 0x31, 0xc7, 0x94,
		0x22, 0x29, 0x15, 0x3c, 0xe3, 0x4d, 0x87, 0x39,
		0x29, 0xb1, 0x60, 0x88, 0xd9, 0xe1, 0xaf, 0x43,
	}

	// OP_DATA_64 <signature> <pikey> OP_TSPEND
	tspendValidKey = []byte{
		0x40, // OP_DATA_64 valid signature
		0x77, 0x69, 0x84, 0xf6, 0x83, 0x13, 0xb1, 0xac,
		0x62, 0x9e, 0x62, 0x4a, 0xf0, 0x59, 0x5b, 0xdc,
		0x09, 0xd8, 0xde, 0xd0, 0x2b, 0xc2, 0xb2, 0x9f,
		0xbd, 0xb3, 0x95, 0x95, 0xe0, 0x3a, 0xc8, 0xb0,
		0xcf, 0x81, 0x8c, 0xa5, 0x36, 0x72, 0x3e, 0x63,
		0x90, 0xd3, 0x08, 0x4e, 0x0e, 0x31, 0xc7, 0x94,
		0x22, 0x29, 0x15, 0x3c, 0xe3, 0x4d, 0x87, 0x39,
		0x29, 0xb1, 0x60, 0x88, 0xd9, 0xe1, 0xaf, 0x43,
		0x21, // OP_DATA_33 valid public key
		0x02, 0xa4, 0xf6, 0x45, 0x86, 0xe1, 0x72, 0xc3,
		0xd9, 0xa2, 0x0c, 0xfa, 0x6c, 0x7a, 0xc8, 0xfb,
		0x12, 0xf0, 0x11, 0x5b, 0x3f, 0x69, 0xc3, 0xc3,
		0x5a, 0xec, 0x93, 0x3a, 0x4c, 0x47, 0xc7, 0xd9,
		0x2c,
		0xc2, // OP_TSPEND
	}

	// OP_DATA_64 <signature> <pikey>
	tspendNoTSpend = []byte{
		0x40, // OP_DATA_64 valid signature
		0x77, 0x69, 0x84, 0xf6, 0x83, 0x13, 0xb1, 0xac,
		0x62, 0x9e, 0x62, 0x4a, 0xf0, 0x59, 0x5b, 0xdc,
		0x09, 0xd8, 0xde, 0xd0, 0x2b, 0xc2, 0xb2, 0x9f,
		0xbd, 0xb3, 0x95, 0x95, 0xe0, 0x3a, 0xc8, 0xb0,
		0xcf, 0x81, 0x8c, 0xa5, 0x36, 0x72, 0x3e, 0x63,
		0x90, 0xd3, 0x08, 0x4e, 0x0e, 0x31, 0xc7, 0x94,
		0x22, 0x29, 0x15, 0x3c, 0xe3, 0x4d, 0x87, 0x39,
		0x29, 0xb1, 0x60, 0x88, 0xd9, 0xe1, 0xaf, 0x43,
		0x21, // OP_DATA_33 valid public key
		0x02, 0xa4, 0xf6, 0x45, 0x86, 0xe1, 0x72, 0xc3,
		0xd9, 0xa2, 0x0c, 0xfa, 0x6c, 0x7a, 0xc8, 0xfb,
		0x12, 0xf0, 0x11, 0x5b, 0x3f, 0x69, 0xc3, 0xc3,
		0x5a, 0xec, 0x93, 0x3a, 0x4c, 0x47, 0xc7, 0xd9,
		0x2c, // No OP_TSPEND
	}

	// nolint: dupword
	//
	// OP_DATA_64 <signature> <pikey> OP_TSPEND OP_TSPEND
	tspendTwoTSpend = []byte{
		0x40, // OP_DATA_64 valid signature
		0x77, 0x69, 0x84, 0xf6, 0x83, 0x13, 0xb1, 0xac,
		0x62, 0x9e, 0x62, 0x4a, 0xf0, 0x59, 0x5b, 0xdc,
		0x09, 0xd8, 0xde, 0xd0, 0x2b, 0xc2, 0xb2, 0x9f,
		0xbd, 0xb3, 0x95, 0x95, 0xe0, 0x3a, 0xc8, 0xb0,
		0xcf, 0x81, 0x8c, 0xa5, 0x36, 0x72, 0x3e, 0x63,
		0x90, 0xd3, 0x08, 0x4e, 0x0e, 0x31, 0xc7, 0x94,
		0x22, 0x29, 0x15, 0x3c, 0xe3, 0x4d, 0x87, 0x39,
		0x29, 0xb1, 0x60, 0x88, 0xd9, 0xe1, 0xaf, 0x43,
		0x21, // OP_DATA_33 valid public key
		0x02, 0xa4, 0xf6, 0x45, 0x86, 0xe1, 0x72, 0xc3,
		0xd9, 0xa2, 0x0c, 0xfa, 0x6c, 0x7a, 0xc8, 0xfb,
		0x12, 0xf0, 0x11, 0x5b, 0x3f, 0x69, 0xc3, 0xc3,
		0x5a, 0xec, 0x93, 0x3a, 0x4c, 0x47, 0xc7, 0xd9,
		0x2c, // No OP_TSPEND
		0xc2, // OP_TSPEND
		0xc2, // Extra OP_TSPEND
	}

	// OP_DATA_64 <signature> <pikey> OP_TSPEND OP_DATA_1
	tspendTrailingData = []byte{
		0x40, // OP_DATA_64 valid signature
		0x77, 0x69, 0x84, 0xf6, 0x83, 0x13, 0xb1, 0xac,
		0x62, 0x9e, 0x62, 0x4a, 0xf0, 0x59, 0x5b, 0xdc,
		0x09, 0xd8, 0xde, 0xd0, 0x2b, 0xc2, 0xb2, 0x9f,
		0xbd, 0xb3, 0x95, 0x95, 0xe0, 0x3a, 0xc8, 0xb0,
		0xcf, 0x81, 0x8c, 0xa5, 0x36, 0x72, 0x3e, 0x63,
		0x90, 0xd3, 0x08, 0x4e, 0x0e, 0x31, 0xc7, 0x94,
		0x22, 0x29, 0x15, 0x3c, 0xe3, 0x4d, 0x87, 0x39,
		0x29, 0xb1, 0x60, 0x88, 0xd9, 0xe1, 0xaf, 0x43,
		0x21, // OP_DATA_33 valid public key
		0x02, 0xa4, 0xf6, 0x45, 0x86, 0xe1, 0x72, 0xc3,
		0xd9, 0xa2, 0x0c, 0xfa, 0x6c, 0x7a, 0xc8, 0xfb,
		0x12, 0xf0, 0x11, 0x5b, 0x3f, 0x69, 0xc3, 0xc3,
		0x5a, 0xec, 0x93, 0x3a, 0x4c, 0x47, 0xc7, 0xd9,
		0x2c, // No OP_TSPEND
		0xc2, // OP_TSPEND
		0x01, // OP_DATA_1, ByteIndex test in CheckTSpend
	}
)

// generateKeys generates all the keys that are hard coded in this file.
//func generateKeys() {
//	key := secp256k1.PrivKeyFromBytes(privateKey)
//	pubKey := key.PubKey()
//	message := "test message"
//	messageHash := chainhash.HashB([]byte(message))
//	signature, err := schnorr.Sign(key, messageHash)
//	if err != nil {
//		panic(err)
//	}
//	fmt.Printf("Sig 0x%x: %x\n", len(signature.Serialize()),
//		signature.Serialize())
//	fmt.Printf("Public key 0x%x: %x\n", len(pubKey.SerializeCompressed()),
//		pubKey.SerializeCompressed())
//	for k, v := range signature.Serialize() {
//		if k%8 == 0 {
//			fmt.Printf("\n")
//		}
//		fmt.Printf("0x%02x,", v)
//	}
//	fmt.Printf("\n")
//}
//
//func init() {
//	generateKeys()
//	panic("x")
//}

// newTxOut returns a new transaction output with the given parameters.
func newTxOut(amount int64, pkScriptVer uint16, pkScript []byte) *wire.TxOut {
	return &wire.TxOut{
		Value:    amount,
		Version:  pkScriptVer,
		PkScript: pkScript,
	}
}

// TestTreasuryIsFunctions goes through all valid treasury opcode combinations.
func TestTreasuryIsFunctions(t *testing.T) {
	tests := []struct {
		name     string
		createTx func() *wire.MsgTx
		is       func(*wire.MsgTx) bool
		expected bool
		check    func(*wire.MsgTx) error
	}{{
		name: "tadd from user, no change",
		createTx: func() *wire.MsgTx {
			builder := txscript.NewScriptBuilder()
			builder.AddOp(txscript.OP_TADD)
			script, err := builder.Script()
			if err != nil {
				panic(err)
			}
			msgTx := wire.NewMsgTx()
			msgTx.Version = wire.TxVersionTreasury
			msgTx.AddTxOut(wire.NewTxOut(0, script))
			msgTx.AddTxIn(&wire.TxIn{}) // One input required
			return msgTx
		},
		is:       IsTAdd,
		expected: true,
		check:    checkTAdd,
	}, {
		name: "check tadd from user, no change with istreasurybase",
		createTx: func() *wire.MsgTx {
			builder := txscript.NewScriptBuilder()
			builder.AddOp(txscript.OP_TADD)
			script, err := builder.Script()
			if err != nil {
				panic(err)
			}
			msgTx := wire.NewMsgTx()
			msgTx.Version = wire.TxVersionTreasury
			msgTx.AddTxOut(wire.NewTxOut(0, script))
			msgTx.AddTxIn(&wire.TxIn{}) // One input required
			return msgTx
		},
		is:       IsTreasuryBase,
		expected: false,
		check:    checkTreasuryBase,
	}, {
		// This is a valid stakebase but NOT a valid TADD.
		name: "tadd from user, with OP_RETURN",
		createTx: func() *wire.MsgTx {
			builder := txscript.NewScriptBuilder()
			builder.AddOp(txscript.OP_TADD)
			script, err := builder.Script()
			if err != nil {
				panic(err)
			}
			msgTx := wire.NewMsgTx()
			msgTx.Version = wire.TxVersionTreasury
			msgTx.AddTxOut(wire.NewTxOut(0, script))

			// OP_RETURN <data>
			payload := make([]byte, chainhash.HashSize)
			_, err = rand.Read(payload)
			if err != nil {
				panic(err)
			}
			builder = txscript.NewScriptBuilder()
			builder.AddOp(txscript.OP_RETURN)
			builder.AddData(payload)
			script, err = builder.Script()
			if err != nil {
				panic(err)
			}
			msgTx.AddTxOut(wire.NewTxOut(0, script))

			msgTx.AddTxIn(&wire.TxIn{
				// Stakebase transactions have no
				// inputs, so previous outpoint is zero
				// hash and max index.
				PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
					wire.MaxPrevOutIndex, wire.TxTreeRegular),
				Sequence:        wire.MaxTxInSequenceNum,
				BlockHeight:     wire.NullBlockHeight,
				BlockIndex:      wire.NullBlockIndex,
				SignatureScript: []byte{txscript.OP_TRUE},
			})
			return msgTx
		},
		is:       IsTAdd,
		expected: false,
		check:    checkTAdd,
	}, {
		name: "tadd from user, with change",
		createTx: func() *wire.MsgTx {
			builder := txscript.NewScriptBuilder()
			builder.AddOp(txscript.OP_TADD)
			script, err := builder.Script()
			if err != nil {
				panic(err)
			}
			msgTx := wire.NewMsgTx()
			msgTx.Version = wire.TxVersionTreasury
			msgTx.AddTxOut(wire.NewTxOut(0, script))

			opTrueScript := []byte{txscript.OP_TRUE}
			p2shOpTrueAddr, err := stdaddr.NewAddressScriptHashV0(opTrueScript,
				chaincfg.MainNetParams())
			if err != nil {
				panic(err)
			}
			changeScriptVer, changeScript := p2shOpTrueAddr.StakeChangeScript()
			msgTx.AddTxOut(newTxOut(1, changeScriptVer, changeScript))
			msgTx.AddTxIn(&wire.TxIn{}) // One input required
			return msgTx
		},
		is:       IsTAdd,
		expected: true,
		check:    checkTAdd,
	}, {
		name: "tadd from treasurybase",
		createTx: func() *wire.MsgTx {
			builder := txscript.NewScriptBuilder()
			builder.AddOp(txscript.OP_TADD)
			script, err := builder.Script()
			if err != nil {
				panic(err)
			}
			msgTx := wire.NewMsgTx()
			msgTx.Version = wire.TxVersionTreasury
			msgTx.AddTxOut(wire.NewTxOut(0, script))

			// OP_RETURN <height> <random>
			payload := make([]byte, 12)
			_, err = rand.Read(payload)
			if err != nil {
				panic(err)
			}
			builder = txscript.NewScriptBuilder()
			builder.AddOp(txscript.OP_RETURN)
			builder.AddData(payload)
			script, err = builder.Script()
			if err != nil {
				panic(err)
			}
			msgTx.AddTxOut(wire.NewTxOut(0, script))

			// treasurybase
			msgTx.AddTxIn(&wire.TxIn{
				// Stakebase transactions have no
				// inputs, so previous outpoint is zero
				// hash and max index.
				PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
					wire.MaxPrevOutIndex, wire.TxTreeRegular),
				Sequence:        wire.MaxTxInSequenceNum,
				BlockHeight:     wire.NullBlockHeight,
				BlockIndex:      wire.NullBlockIndex,
				SignatureScript: nil,
			})

			return msgTx
		},
		is:       IsTreasuryBase,
		expected: true,
		check:    checkTreasuryBase,
	}, {
		name: "check treasury base with tadd",
		createTx: func() *wire.MsgTx {
			builder := txscript.NewScriptBuilder()
			builder.AddOp(txscript.OP_TADD)
			script, err := builder.Script()
			if err != nil {
				panic(err)
			}
			msgTx := wire.NewMsgTx()
			msgTx.Version = wire.TxVersionTreasury
			msgTx.AddTxOut(wire.NewTxOut(0, script))

			// OP_RETURN <height> <random>
			payload := make([]byte, 12)
			_, err = rand.Read(payload)
			if err != nil {
				panic(err)
			}
			builder = txscript.NewScriptBuilder()
			builder.AddOp(txscript.OP_RETURN)
			builder.AddData(payload)
			script, err = builder.Script()
			if err != nil {
				panic(err)
			}
			msgTx.AddTxOut(wire.NewTxOut(0, script))

			msgTx.AddTxIn(&wire.TxIn{
				// Stakebase transactions have no
				// inputs, so previous outpoint is zero
				// hash and max index.
				PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
					wire.MaxPrevOutIndex, wire.TxTreeRegular),
				Sequence:        wire.MaxTxInSequenceNum,
				BlockHeight:     wire.NullBlockHeight,
				BlockIndex:      wire.NullBlockIndex,
				SignatureScript: nil,
			})

			return msgTx
		},
		is:       IsTAdd,
		expected: false,
		check:    checkTAdd,
	}, {
		name: "tspend P2SH",
		createTx: func() *wire.MsgTx {
			// OP_RETURN <32 byte random>
			payload := make([]byte, chainhash.HashSize)
			_, err := rand.Read(payload)
			if err != nil {
				panic(err)
			}
			builder := txscript.NewScriptBuilder()
			builder.AddOp(txscript.OP_RETURN)
			builder.AddData(payload)
			opretScript, err := builder.Script()
			if err != nil {
				panic(err)
			}
			msgTx := wire.NewMsgTx()
			msgTx.Version = wire.TxVersionTreasury
			msgTx.AddTxOut(wire.NewTxOut(0, opretScript))

			// OP_TGEN
			opTrueScript := []byte{txscript.OP_TRUE}
			p2shOpTrueAddr, err := stdaddr.NewAddressScriptHashV0(opTrueScript,
				chaincfg.MainNetParams())
			if err != nil {
				panic(err)
			}
			genScriptVer, genScript := p2shOpTrueAddr.PayFromTreasuryScript()
			msgTx.AddTxOut(newTxOut(0, genScriptVer, genScript))

			// tspend
			builder = txscript.NewScriptBuilder()
			builder.AddData(validSignature)
			builder.AddData(publicKey)
			builder.AddOp(txscript.OP_TSPEND)
			tspendScript, err := builder.Script()
			if err != nil {
				panic(err)
			}

			msgTx.AddTxIn(&wire.TxIn{
				// Stakebase transactions have no
				// inputs, so previous outpoint is zero
				// hash and max index.
				PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
					wire.MaxPrevOutIndex, wire.TxTreeRegular),
				Sequence:        wire.MaxTxInSequenceNum,
				BlockHeight:     wire.NullBlockHeight,
				BlockIndex:      wire.NullBlockIndex,
				SignatureScript: tspendScript,
			})

			return msgTx
		},
		is:       IsTSpend,
		expected: true,
		check:    checkTSpend,
	}, {
		name: "tspend invalid output 1 (not P2SH/P2PKH)",
		createTx: func() *wire.MsgTx {
			// OP_RETURN <32 byte random>
			payload := make([]byte, chainhash.HashSize)
			_, err := rand.Read(payload)
			if err != nil {
				panic(err)
			}
			builder := txscript.NewScriptBuilder()
			builder.AddOp(txscript.OP_RETURN)
			builder.AddData(payload)
			opretScript, err := builder.Script()
			if err != nil {
				panic(err)
			}
			msgTx := wire.NewMsgTx()
			msgTx.Version = wire.TxVersionTreasury
			msgTx.AddTxOut(wire.NewTxOut(0, opretScript))

			// OP_TGEN
			privKey := secp256k1.NewPrivateKey(new(secp256k1.ModNScalar).SetInt(1))
			pubKey := privKey.PubKey().SerializeCompressed()
			p2pkAddr, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0Raw(pubKey,
				chaincfg.MainNetParams())
			if err != nil {
				panic(err)
			}
			p2pkScriptVer, p2pkScript := p2pkAddr.PaymentScript()
			script := make([]byte, len(p2pkScript)+1)
			script[0] = txscript.OP_TGEN
			copy(script[1:], p2pkScript)
			msgTx.AddTxOut(newTxOut(0, p2pkScriptVer, script))

			// tspend
			builder = txscript.NewScriptBuilder()
			builder.AddData(validSignature)
			builder.AddData(publicKey)
			builder.AddOp(txscript.OP_TSPEND)
			tspendScript, err := builder.Script()
			if err != nil {
				panic(err)
			}

			msgTx.AddTxIn(&wire.TxIn{
				// Stakebase transactions have no
				// inputs, so previous outpoint is zero
				// hash and max index.
				PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
					wire.MaxPrevOutIndex, wire.TxTreeRegular),
				Sequence:        wire.MaxTxInSequenceNum,
				BlockHeight:     wire.NullBlockHeight,
				BlockIndex:      wire.NullBlockIndex,
				SignatureScript: tspendScript,
			})

			return msgTx
		},
		is:       IsTSpend,
		expected: false,
		check:    checkTSpend,
	}, {
		name: "tspend P2PKH",
		createTx: func() *wire.MsgTx {
			// OP_RETURN <32 byte random>
			payload := make([]byte, chainhash.HashSize)
			_, err := rand.Read(payload)
			if err != nil {
				panic(err)
			}
			builder := txscript.NewScriptBuilder()
			builder.AddOp(txscript.OP_RETURN)
			builder.AddData(payload)
			opretScript, err := builder.Script()
			if err != nil {
				panic(err)
			}
			msgTx := wire.NewMsgTx()
			msgTx.Version = wire.TxVersionTreasury
			msgTx.AddTxOut(wire.NewTxOut(0, opretScript))

			// OP_TGEN
			privKey := secp256k1.NewPrivateKey(new(secp256k1.ModNScalar).SetInt(1))
			pubKey := privKey.PubKey()
			pkHash := stdaddr.Hash160(pubKey.SerializeCompressed())
			p2pkhAddr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
				pkHash, chaincfg.MainNetParams())
			if err != nil {
				panic(err)
			}
			genScriptVer, genScript := p2pkhAddr.PayFromTreasuryScript()
			msgTx.AddTxOut(newTxOut(0, genScriptVer, genScript))

			// tspend
			builder = txscript.NewScriptBuilder()
			builder.AddData(validSignature)
			builder.AddData(publicKey)
			builder.AddOp(txscript.OP_TSPEND)
			tspendScript, err := builder.Script()
			if err != nil {
				panic(err)
			}

			msgTx.AddTxIn(&wire.TxIn{
				// Stakebase transactions have no
				// inputs, so previous outpoint is zero
				// hash and max index.
				PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
					wire.MaxPrevOutIndex, wire.TxTreeRegular),
				Sequence:        wire.MaxTxInSequenceNum,
				BlockHeight:     wire.NullBlockHeight,
				BlockIndex:      wire.NullBlockIndex,
				SignatureScript: tspendScript,
			})

			return msgTx
		},
		is:       IsTSpend,
		expected: true,
		check:    checkTSpend,
	}}

	for i, test := range tests {
		if got := test.is(test.createTx()); got != test.expected {
			// Obtain error
			err := test.check(test.createTx())
			t.Fatalf("%v %v: failed got %v want %v error %v",
				i, test.name, got, test.expected, err)
		}
	}
}

// tspendTxInNoPubkey
var tspendTxInNoPubkey = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash:  chainhash.Hash{},
		Index: 0xffffffff,
		Tree:  wire.TxTreeRegular,
	},
	SignatureScript: []byte{
		0xc2, // OP_TSPEND
	},
	BlockHeight: wire.NullBlockHeight,
	BlockIndex:  wire.NullBlockIndex,
	Sequence:    0xffffffff,
}

// tspendTxInInvalidPubkey is a TxIn with an invalid key on the OP_TSPEND.
var tspendTxInInvalidPubkey = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash:  chainhash.Hash{},
		Index: 0xffffffff,
		Tree:  wire.TxTreeRegular,
	},
	SignatureScript: []byte{
		0xc2, // OP_TSPEND
		0x23, // OP_DATA_35
		0x03, // Valid pubkey version
		0x00, // invalid compressed key
	},
	BlockHeight: wire.NullBlockHeight,
	BlockIndex:  wire.NullBlockIndex,
	Sequence:    0xffffffff,
}

// tspendTxInInvalidOpcode is a TxIn with an invalid opcode where OP_TSPEND was
// supposed to be.
var tspendTxInInvalidOpcode = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash:  chainhash.Hash{},
		Index: 0xffffffff,
		Tree:  wire.TxTreeRegular,
	},
	SignatureScript: []byte{
		0x40, // OP_DATA_64 valid signature
		0x77, 0x69, 0x84, 0xf6, 0x83, 0x13, 0xb1, 0xac,
		0x62, 0x9e, 0x62, 0x4a, 0xf0, 0x59, 0x5b, 0xdc,
		0x09, 0xd8, 0xde, 0xd0, 0x2b, 0xc2, 0xb2, 0x9f,
		0xbd, 0xb3, 0x95, 0x95, 0xe0, 0x3a, 0xc8, 0xb0,
		0xcf, 0x81, 0x8c, 0xa5, 0x36, 0x72, 0x3e, 0x63,
		0x90, 0xd3, 0x08, 0x4e, 0x0e, 0x31, 0xc7, 0x94,
		0x22, 0x29, 0x15, 0x3c, 0xe3, 0x4d, 0x87, 0x39,
		0x29, 0xb1, 0x60, 0x88, 0xd9, 0xe1, 0xaf, 0x43,
		0x21, // OP_DATA_33 valid public key
		0x02, 0xa4, 0xf6, 0x45, 0x86, 0xe1, 0x72, 0xc3,
		0xd9, 0xa2, 0x0c, 0xfa, 0x6c, 0x7a, 0xc8, 0xfb,
		0x12, 0xf0, 0x11, 0x5b, 0x3f, 0x69, 0xc3, 0xc3,
		0x5a, 0xec, 0x93, 0x3a, 0x4c, 0x47, 0xc7, 0xd9,
		0x2c,
		0x6a, // OP_RETURN instead of OP_TSPEND
	},
	BlockHeight: wire.NullBlockHeight,
	BlockIndex:  wire.NullBlockIndex,
	Sequence:    0xffffffff,
}

// tspendTxInInvalidPubkey2 is a TxIn with an invalid public key on the
// OP_TSPEND.
var tspendTxInInvalidPubkey2 = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash:  chainhash.Hash{},
		Index: 0xffffffff,
		Tree:  wire.TxTreeRegular,
	},
	SignatureScript: []byte{
		0x40, // OP_DATA_64 valid signature
		0x77, 0x69, 0x84, 0xf6, 0x83, 0x13, 0xb1, 0xac,
		0x62, 0x9e, 0x62, 0x4a, 0xf0, 0x59, 0x5b, 0xdc,
		0x09, 0xd8, 0xde, 0xd0, 0x2b, 0xc2, 0xb2, 0x9f,
		0xbd, 0xb3, 0x95, 0x95, 0xe0, 0x3a, 0xc8, 0xb0,
		0xcf, 0x81, 0x8c, 0xa5, 0x36, 0x72, 0x3e, 0x63,
		0x90, 0xd3, 0x08, 0x4e, 0x0e, 0x31, 0xc7, 0x94,
		0x22, 0x29, 0x15, 0x3c, 0xe3, 0x4d, 0x87, 0x39,
		0x29, 0xb1, 0x60, 0x88, 0xd9, 0xe1, 0xaf, 0x43,
		0x21, // OP_DATA_33 INVALID public key
		0x00, 0xa4, 0xf6, 0x45, 0x86, 0xe1, 0x72, 0xc3,
		0xd9, 0xa2, 0x0c, 0xfa, 0x6c, 0x7a, 0xc8, 0xfb,
		0x12, 0xf0, 0x11, 0x5b, 0x3f, 0x69, 0xc3, 0xc3,
		0x5a, 0xec, 0x93, 0x3a, 0x4c, 0x47, 0xc7, 0xd9,
		0x2c,
		0xc2, // OP_TSPEND
	},
	BlockHeight: wire.NullBlockHeight,
	BlockIndex:  wire.NullBlockIndex,
	Sequence:    0xffffffff,
}

var tspendTxOutValidReturn = wire.TxOut{
	Value:   500000000,
	Version: 0,
	PkScript: []byte{
		0x6a, // OP_RETURN
		0x20, // OP_DATA_32
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	},
}

var tspendTxOutInvalidReturn = wire.TxOut{
	Value:   500000000,
	Version: 0,
	PkScript: []byte{
		0x6a, // OP_RETURN
		0x20, // OP_DATA_32
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // 1 byte short
	},
}

// tspendTxInValidPubkey is a TxIn with a public key on the OP_TSPEND.
var tspendTxInValidPubkey = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash:  chainhash.Hash{},
		Index: 0xffffffff,
		Tree:  wire.TxTreeRegular,
	},
	SignatureScript: tspendValidKey,
	BlockHeight:     wire.NullBlockHeight,
	BlockIndex:      wire.NullBlockIndex,
	Sequence:        0xffffffff,
}

// tspendTxInNoTSpend is a TxIn with a public key but not TSpend opcode.
var tspendTxInNoTSpend = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash:  chainhash.Hash{},
		Index: 0xffffffff,
		Tree:  wire.TxTreeRegular,
	},
	SignatureScript: tspendNoTSpend,
	BlockHeight:     wire.NullBlockHeight,
	BlockIndex:      wire.NullBlockIndex,
	Sequence:        0xffffffff,
}

// tspendTxInTwoTSpend is a TxIn with a public key but two TSpend opcodes.
var tspendTxInTwoTSpend = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash:  chainhash.Hash{},
		Index: 0xffffffff,
		Tree:  wire.TxTreeRegular,
	},
	SignatureScript: tspendTwoTSpend,
	BlockHeight:     wire.NullBlockHeight,
	BlockIndex:      wire.NullBlockIndex,
	Sequence:        0xffffffff,
}

// tspendTxTrailingData is a TxIn with a public key, one TSpend and an
// OP_DATA_1.
var tspendTxTrailingData = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash:  chainhash.Hash{},
		Index: 0xffffffff,
		Tree:  wire.TxTreeRegular,
	},
	SignatureScript: tspendTrailingData,
	BlockHeight:     wire.NullBlockHeight,
	BlockIndex:      wire.NullBlockIndex,
	Sequence:        0xffffffff,
}

// tspendInvalidInCount has an invalid TxIn count but a valid TxOut count.
var tspendInvalidInCount = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn:    []*wire.TxIn{},
	TxOut: []*wire.TxOut{
		{}, // 2 TxOuts is valid
		{},
	},
	LockTime: 0,
	Expiry:   0,
}

// tspendInvalidOutCount has a valid TxIn count but an invalid TxOut count.
var tspendInvalidOutCount = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		&tspendTxInNoPubkey,
	},
	TxOut:    []*wire.TxOut{},
	LockTime: 0,
	Expiry:   0,
}

// tspendInvalidVersion has an invalid version in an out script.
var tspendInvalidVersion = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		&tspendTxInNoPubkey,
	},
	TxOut: []*wire.TxOut{
		{
			Version: 0,
			PkScript: []byte{
				0x6a, // OP_RETURN
			},
		},
		{
			Version: 1, // Fail
			PkScript: []byte{
				0xc3, // OP_TGEN
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// tspendInvalidSignature has no publick key in the input script.
var tspendInvalidSignature = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		&tspendTxInNoPubkey,
	},
	TxOut: []*wire.TxOut{
		{
			PkScript: []byte{
				0x6a, // OP_RETURN
			},
		},
		{
			PkScript: []byte{
				0xc3, // OP_TGEN
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// tspendInvalidSignature2 has an invalid public key in the input script.
var tspendInvalidSignature2 = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		&tspendTxInInvalidPubkey,
	},
	TxOut: []*wire.TxOut{
		{
			PkScript: []byte{
				0x6a, // OP_RETURN
			},
		},
		{
			PkScript: []byte{
				0xc3, // OP_TGEN
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// tspendInvalidOpcode has an invalid opcode in the first TxIn.
var tspendInvalidOpcode = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		&tspendTxInInvalidOpcode,
	},
	TxOut: []*wire.TxOut{
		{
			PkScript: []byte{
				0x6a, // OP_RETURN
			},
		},
		{
			PkScript: []byte{
				0xc3, // OP_TGEN
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// tspendInvalidPubkey has an invalid public key on the TSPEND.
var tspendInvalidPubkey = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		&tspendTxInInvalidPubkey2,
	},
	TxOut: []*wire.TxOut{
		{
			PkScript: []byte{
				0x6a, // OP_RETURN
			},
		},
		{
			PkScript: []byte{
				0xc3, // OP_TGEN
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// tspendInvalidScriptLength has an invalid TxOut that has a zero length.
var tspendInvalidScriptLength = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		&tspendTxInValidPubkey,
	},
	TxOut: []*wire.TxOut{
		&tspendTxOutValidReturn,
		{},
	},
	LockTime: 0,
	Expiry:   0,
}

// tspendInvalidTokenCount does not have enough tokens in input script.
var tspendInvalidTokenCount = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		&tspendTxInNoTSpend,
	},
	TxOut: []*wire.TxOut{
		&tspendTxOutValidReturn,
		{
			PkScript: []byte{
				0xc3, // OP_TGEN
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// tspendInvalidTokenCount2 has too many tokens on input script.
var tspendInvalidTokenCount2 = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		&tspendTxInTwoTSpend,
	},
	TxOut: []*wire.TxOut{
		&tspendTxOutValidReturn,
		{
			PkScript: []byte{
				0xc3, // OP_TGEN
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// tspendInvalidTokenCount3 has trailing data after TSpend.
var tspendInvalidTokenCount3 = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		&tspendTxTrailingData,
	},
	TxOut: []*wire.TxOut{
		&tspendTxOutValidReturn,
		{
			PkScript: []byte{
				0xc3, // OP_TGEN
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// tspendInvalidTransaction has an invalid hash on the OP_RETURN.
var tspendInvalidTransaction = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		&tspendTxInValidPubkey,
	},
	TxOut: []*wire.TxOut{
		&tspendTxOutInvalidReturn,
		{
			PkScript: []byte{
				0xc3, // OP_TGEN
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// tspendInvalidTGen has an invalid TxOut that isn't tagged with an OP_TGEN.
var tspendInvalidTGen = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		&tspendTxInValidPubkey,
	},
	TxOut: []*wire.TxOut{
		&tspendTxOutValidReturn,
		{
			PkScript: []byte{
				0x6a, // OP_RETURN instead of OP_TGEN
			}},
	},
	LockTime: 0,
	Expiry:   0,
}

// tspendInvalidP2SH has an invalid TxOut that doesn't have a valid P2SH
// script.
var tspendInvalidP2SH = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		&tspendTxInValidPubkey,
	},
	TxOut: []*wire.TxOut{
		&tspendTxOutValidReturn,
		{
			PkScript: []byte{
				0xc3, // OP_TGEN
				0x00, // Invalid P2SH
			}},
	},
	LockTime: 0,
	Expiry:   0,
}

var tspendInvalidTxVersion = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1, // Invalid version
	TxIn: []*wire.TxIn{
		&tspendTxInValidPubkey,
	},
	TxOut: []*wire.TxOut{
		&tspendTxOutValidReturn,
	},
	LockTime: 0,
	Expiry:   0,
}

func TestTSpendGenerated(t *testing.T) {
	rawScript := "03000000010000000000000000000000000000000000000000000000000000000000000000ffffffff00ffffffff0200000000000000000000226a20562ce42e7531d1710ea1ee02628191190ef5152bbbcd23acca864433c4e4e7849cf1052a01000000000018c3a914f5a8302ee8695bf836258b8f2b57b38a0be14e478700000000520000000100f2052a0100000000000000ffffffff64408ea1c04f5e5dd59350847fad8b800887200ae7268da3b70488a605dd5f4ad28e6e240dbd483a8ba46324a047cf0d6c506e6ebb61d93cae6e868b86f31d9bda892103b459ccf3ce4935a676414fd9ec93ecf7c9dad081a52ed6993bf073c627499388c2"
	s, err := hex.DecodeString(rawScript)
	if err != nil {
		t.Fatal(err)
	}
	var tx wire.MsgTx
	err = tx.Deserialize(bytes.NewReader(s))
	if err != nil {
		t.Fatalf("Deserialize: %v", err)
	}
	tx.Version = wire.TxVersionTreasury

	err = checkTSpend(&tx)
	if err != nil {
		t.Fatalf("checkTSpend: %v", err)
	}
}

func TestTSpendErrors(t *testing.T) {
	tests := []struct {
		name     string
		tx       *wire.MsgTx
		expected error
	}{
		{
			name:     "tspendInvalidOutCount",
			tx:       tspendInvalidOutCount,
			expected: ErrTSpendInvalidLength,
		},
		{
			name:     "tspendInvalidInCount",
			tx:       tspendInvalidInCount,
			expected: ErrTSpendInvalidLength,
		},
		{
			name:     "tspendInvalidVersion",
			tx:       tspendInvalidVersion,
			expected: ErrTSpendInvalidVersion,
		},
		{
			name:     "tspendInvalidSignature",
			tx:       tspendInvalidSignature,
			expected: ErrTSpendInvalidScript,
		},
		{
			name:     "tspendInvalidSignature2",
			tx:       tspendInvalidSignature2,
			expected: ErrTSpendInvalidScript,
		},
		{
			name:     "tspendInvalidOpcode",
			tx:       tspendInvalidOpcode,
			expected: ErrTSpendInvalidScript,
		},
		{
			name:     "tspendInvalidPubkey",
			tx:       tspendInvalidPubkey,
			expected: ErrTSpendInvalidPubkey,
		},
		{
			name:     "tspendInvalidTokenCount",
			tx:       tspendInvalidTokenCount,
			expected: ErrTSpendInvalidScript,
		},
		{
			name:     "tspendInvalidTokenCount2",
			tx:       tspendInvalidTokenCount2,
			expected: ErrTSpendInvalidScript,
		},
		{
			name:     "tspendInvalidTokenCount3",
			tx:       tspendInvalidTokenCount3,
			expected: ErrTSpendInvalidScript,
		},
		{
			name:     "tspendInvalidScriptLength",
			tx:       tspendInvalidScriptLength,
			expected: ErrTSpendInvalidScriptLength,
		},
		{
			name:     "tspendInvalidTransaction",
			tx:       tspendInvalidTransaction,
			expected: ErrTSpendInvalidTransaction,
		},
		{
			name:     "tspendInvalidTGen",
			tx:       tspendInvalidTGen,
			expected: ErrTSpendInvalidTGen,
		},
		{
			name:     "tspendInvalidP2SH",
			tx:       tspendInvalidP2SH,
			expected: ErrTSpendInvalidSpendScript,
		},
		{
			name:     "tspendInvalidTxVersion",
			tx:       tspendInvalidTxVersion,
			expected: ErrTSpendInvalidTxVersion,
		},
	}
	for i, tt := range tests {
		test := dcrutil.NewTx(tt.tx)
		test.SetTree(wire.TxTreeStake)
		test.SetIndex(0)
		err := checkTSpend(test.MsgTx())
		if !errors.Is(err, tt.expected) {
			t.Errorf("%v: checkTSpend should have returned %v but "+
				"instead returned %v", tt.name, tt.expected, err)
		}
		if IsTSpend(test.MsgTx()) {
			t.Errorf("IsTSpend claimed an invalid tspend is valid"+
				" %v %v", i, tt.name)
		}
	}
}

// taddInvalidOutCount has a valid TxIn count but an invalid TxOut count.
var taddInvalidOutCount = &wire.MsgTx{
	SerType:  wire.TxSerializeFull,
	Version:  3,
	TxIn:     []*wire.TxIn{},
	TxOut:    []*wire.TxOut{},
	LockTime: 0,
	Expiry:   0,
}

// taddInvalidOutCount2 has a valid TxIn count but an invalid TxOut count.
var taddInvalidOutCount2 = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{}, // Valid TxIn count
	},
	TxOut: []*wire.TxOut{
		{},
		{},
		{},
	},
	LockTime: 0,
	Expiry:   0,
}

// taddInvalidOutCount3 has a valid TxIn count but an invalid TxIn count.
var taddInvalidOutCount3 = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn:    []*wire.TxIn{},
	TxOut: []*wire.TxOut{
		{},
		{},
	},
	LockTime: 0,
	Expiry:   0,
}

// taddInvalidVersion has an invalid out script version.
var taddInvalidVersion = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{}, // Empty TxIn
	},
	TxOut: []*wire.TxOut{
		{Version: 1},
		{Version: 0},
	},
	LockTime: 0,
	Expiry:   0,
}

// taddInvalidScriptLength has a zero script length.
var taddInvalidScriptLength = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{}, // Empty TxIn
	},
	TxOut: []*wire.TxOut{
		{Version: 0},
		{Version: 0},
	},
	LockTime: 0,
	Expiry:   0,
}

// taddInvalidLength has an invalid out script.
var taddInvalidLength = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{}, // Empty TxIn
	},
	TxOut: []*wire.TxOut{
		{PkScript: []byte{
			0xc2, // OP_TSPEND instead of OP_TADD
			0x00, // Fail length test
		}},
	},
	LockTime: 0,
	Expiry:   0,
}

// taddInvalidLength has an invalid out script opcode.
var taddInvalidOpcode = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{}, // Empty TxIn
	},
	TxOut: []*wire.TxOut{
		{
			PkScript: []byte{
				0xc2, // OP_TSPEND instead of OP_TADD
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// taddInvalidChange has an invalid out chnage script.
var taddInvalidChange = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{}, // Empty TxIn
	},
	TxOut: []*wire.TxOut{
		{
			PkScript: []byte{
				0xc1, // OP_TADD
			},
		},
		{
			PkScript: []byte{
				0x00, // Not OP_SSTXCHANGE
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// taddInvalidTxVersion has an invalid transaction version.
var taddInvalidTxVersion = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1, // Invalid
	TxIn:    []*wire.TxIn{},
	TxOut: []*wire.TxOut{
		{
			PkScript: []byte{
				0xc1, // OP_TADD
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// TestTAddErrors verifies that all TADD errors can be hit and return the
// proper error.
func TestTAddErrors(t *testing.T) {
	tests := []struct {
		name     string
		tx       *wire.MsgTx
		expected error
	}{
		{
			name:     "taddInvalidOutCount",
			tx:       taddInvalidOutCount,
			expected: ErrTAddInvalidCount,
		},
		{
			name:     "taddInvalidOutCount2",
			tx:       taddInvalidOutCount2,
			expected: ErrTAddInvalidCount,
		},
		{
			name:     "taddInvalidOutCount3",
			tx:       taddInvalidOutCount3,
			expected: ErrTAddInvalidCount,
		},
		{
			name:     "taddInvalidVersion",
			tx:       taddInvalidVersion,
			expected: ErrTAddInvalidVersion,
		},
		{
			name:     "taddInvalidScriptLength",
			tx:       taddInvalidScriptLength,
			expected: ErrTAddInvalidScriptLength,
		},
		{
			name:     "taddInvalidLength",
			tx:       taddInvalidLength,
			expected: ErrTAddInvalidLength,
		},
		{
			name:     "taddInvalidOpcode",
			tx:       taddInvalidOpcode,
			expected: ErrTAddInvalidOpcode,
		},
		{
			name:     "taddInvalidChange",
			tx:       taddInvalidChange,
			expected: ErrTAddInvalidChange,
		},
		{
			name:     "taddInvalidTxVersion",
			tx:       taddInvalidTxVersion,
			expected: ErrTAddInvalidTxVersion,
		},
	}
	for i, tt := range tests {
		test := dcrutil.NewTx(tt.tx)
		test.SetTree(wire.TxTreeStake)
		test.SetIndex(0)
		err := checkTAdd(test.MsgTx())
		if !errors.Is(err, tt.expected) {
			t.Errorf("%v: checkTAdd should have returned %v but "+
				"instead returned %v", tt.name, tt.expected, err)
		}
		if IsTAdd(test.MsgTx()) {
			t.Errorf("IsTAdd claimed an invalid tadd is valid"+
				" %v %v", i, tt.name)
		}
	}
}

// treasurybaseInvalidInCount has an invalid TxIn count.
var treasurybaseInvalidInCount = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn:    []*wire.TxIn{},
	TxOut: []*wire.TxOut{
		{},
		{},
	},
	LockTime: 0,
	Expiry:   0,
}

// treasurybaseInvalidOutCount has an invalid TxOut count.
var treasurybaseInvalidOutCount = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{},
	},
	TxOut:    []*wire.TxOut{},
	LockTime: 0,
	Expiry:   0,
}

// treasurybaseInvalidVersion has an invalid out script version.
var treasurybaseInvalidVersion = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{},
	},
	TxOut: []*wire.TxOut{
		{Version: 0},
		{Version: 2},
	},
	LockTime: 0,
	Expiry:   0,
}

// treasurybaseInvalidOpcode0 has an invalid out script opcode.
var treasurybaseInvalidOpcode0 = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{},
	},
	TxOut: []*wire.TxOut{
		{
			PkScript: []byte{
				0xc2, // OP_TSPEND instead of OP_TADD
			},
		},
		{
			PkScript: []byte{
				0x6a, // OP_RETURN
				0x0c, // OP_DATA_12
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// treasurybaseInvalidOpcode0Len has an invalid out script opcode length.
var treasurybaseInvalidOpcode0Len = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{},
	},
	TxOut: []*wire.TxOut{
		{
			PkScript: nil, // Invalid
		},
		{
			PkScript: []byte{
				0x6a, // OP_RETURN
				0x0c, // OP_DATA_12
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// treasurybaseInvalidOpcode1 has an invalid out script opcode.
var treasurybaseInvalidOpcode1 = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{},
	},
	TxOut: []*wire.TxOut{
		{
			PkScript: []byte{
				0xc1, // OP_TADD
			},
		},
		{
			PkScript: []byte{
				0xc1, // OP_TADD instead of OP_RETURN
				0x0c, // OP_DATA_32
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// treasurybaseInvalidOpcode1Len has an invalid out script opcode length.
var treasurybaseInvalidOpcode1Len = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{},
	},
	TxOut: []*wire.TxOut{
		{
			PkScript: []byte{
				0xc1, // OP_TADD
			},
		},
		{
			PkScript: nil,
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// treasurybaseInvalidOpcodeDataPush has an invalid out script data push in
// script 1 opcode 1.
var treasurybaseInvalidOpcodeDataPush = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{},
	},
	TxOut: []*wire.TxOut{
		{
			PkScript: []byte{
				0xc1, // OP_TADD
			},
		},
		{
			PkScript: []byte{
				0x6a, // OP_RETURN
				0x05, // OP_DATA_5 instead of OP_DATA_4
				0x00, 0x00, 0x00, 0x00, 0x00,
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// treasurybaseInvalid has invalid in script constants.
var treasurybaseInvalid = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Index: math.MaxUint32 - 1,
			},
		},
	},
	TxOut: []*wire.TxOut{
		{
			PkScript: []byte{
				0xc1, // OP_TADD
			},
		},
		{
			PkScript: []byte{
				0x6a, // OP_RETURN
				0x0c, // OP_DATA_12
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// treasurybaseInvalid2 has invalid in script constants.
var treasurybaseInvalid2 = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Index: math.MaxUint32,
				Hash:  chainhash.Hash{'m', 'o', 'o'},
			},
		},
	},
	TxOut: []*wire.TxOut{
		{
			PkScript: []byte{
				0xc1, // OP_TADD
			},
		},
		{
			PkScript: []byte{
				0x6a, // OP_RETURN
				0x0c, // OP_DATA_12
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// treasurybaseInvalidTxVersion has an invalid transaction version.
var treasurybaseInvalidTxVersion = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1, // Invalid
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Index: math.MaxUint32,
				Hash:  chainhash.Hash{'m', 'o', 'o'},
			},
		},
	},
	TxOut: []*wire.TxOut{
		{
			PkScript: []byte{
				0xc1, // OP_TADD
			},
		},
		{
			PkScript: []byte{
				0x6a, // OP_RETURN
				0x0c, // OP_DATA_12
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// treasurybaseInvalidLength has an invalid transaction length.
var treasurybaseInvalidLength = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 3,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Index: math.MaxUint32,
				Hash:  chainhash.Hash{'m', 'o', 'o'},
			},
			SignatureScript: []byte{0x00},
		},
	},
	TxOut: []*wire.TxOut{
		{
			PkScript: []byte{
				0xc1, // OP_TADD
			},
		},
		{
			PkScript: []byte{
				0x6a, // OP_RETURN
				0x04, // OP_DATA_4
				0x00, 0x00, 0x00, 0x00,
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// TestTreasuryBaseErrors verifies that all treasurybase errors can be hit and
// return the proper error.
func TestTreasuryBaseErrors(t *testing.T) {
	tests := []struct {
		name     string
		tx       *wire.MsgTx
		expected error
	}{
		{
			name:     "treasurybaseInvalidInCount",
			tx:       treasurybaseInvalidInCount,
			expected: ErrTreasuryBaseInvalidCount,
		},
		{
			name:     "treasurybaseInvalidOutCount",
			tx:       treasurybaseInvalidOutCount,
			expected: ErrTreasuryBaseInvalidCount,
		},
		{
			name:     "treasurybaseInvalidVersion",
			tx:       treasurybaseInvalidVersion,
			expected: ErrTreasuryBaseInvalidVersion,
		},
		{
			name:     "treasurybaseInvalidOpcode0",
			tx:       treasurybaseInvalidOpcode0,
			expected: ErrTreasuryBaseInvalidOpcode0,
		},
		{
			name:     "treasurybaseInvalidOpcode0Len",
			tx:       treasurybaseInvalidOpcode0Len,
			expected: ErrTreasuryBaseInvalidOpcode0,
		},
		{
			name:     "treasurybaseInvalidOpcode1",
			tx:       treasurybaseInvalidOpcode1,
			expected: ErrTreasuryBaseInvalidOpcode1,
		},
		{
			name:     "treasurybaseInvalidOpcode1Len",
			tx:       treasurybaseInvalidOpcode1Len,
			expected: ErrTreasuryBaseInvalidOpcode1,
		},
		{
			name:     "treasurybaseInvalidDataPush",
			tx:       treasurybaseInvalidOpcodeDataPush,
			expected: ErrTreasuryBaseInvalidOpcode1,
		},
		{
			name:     "treasurybaseInvalid",
			tx:       treasurybaseInvalid,
			expected: ErrTreasuryBaseInvalid,
		},
		{
			name:     "treasurybaseInvalid2",
			tx:       treasurybaseInvalid2,
			expected: ErrTreasuryBaseInvalid,
		},
		{
			name:     "treasurybaseInvalidTxVersion",
			tx:       treasurybaseInvalidTxVersion,
			expected: ErrTreasuryBaseInvalidTxVersion,
		},
		{
			name:     "treasurybaseInvalidLength",
			tx:       treasurybaseInvalidLength,
			expected: ErrTreasuryBaseInvalidLength,
		},
	}
	for i, tt := range tests {
		test := dcrutil.NewTx(tt.tx)
		test.SetTree(wire.TxTreeStake)
		test.SetIndex(0)
		err := checkTreasuryBase(test.MsgTx())
		if !errors.Is(err, tt.expected) {
			t.Errorf("%v: checkTreasuryBase should have returned "+
				"%v but instead returned %v", tt.name, tt.expected, err)
		}
		if IsTreasuryBase(test.MsgTx()) {
			t.Errorf("IsTreasuryBase claimed an invalid treasury "+
				"base is valid %v %v", i, tt.name)
		}
	}
}
