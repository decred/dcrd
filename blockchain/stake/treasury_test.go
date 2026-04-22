// Copyright (c) 2020-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"encoding/binary"
	"errors"
	"math"
	"math/rand"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

// Private and public keys for tests.
var (
	// Serialized private key.
	// privateKey = hexToBytes("7687561394ccc6110151bd9f26d4228eb2d57be128c036" +
	// 	"12e39a844a3ecd3ccf")

	// Serialized compressed public key.
	publicKey = hexToBytes("02a4f64586e172c3d9a20cfa6c7ac8fb12f0115b3f69c3c3" +
		"5aec933a4c47c7d92c")

	// Valid signature of chainhash.HashB([]byte("test message"))
	validSignature = hexToBytes("776984f68313b1ac629e624af0595bdc09d8ded02bc2" +
		"b29fbdb39595e03ac8b0cf818ca536723e6390d3084e0e31c7942229153ce34d8739" +
		"29b16088d9e1af43")

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

// opReturnScript returns a provably-pruneable OP_RETURN script with the
// provided data.
func opReturnScript(data []byte) []byte {
	builder := txscript.NewScriptBuilder()
	script, err := builder.AddOp(txscript.OP_RETURN).AddData(data).Script()
	if err != nil {
		panic(err)
	}
	return script
}

// treasurybaseOpReturnScript returns a script suitable for use as the second
// output of the treasurybase transaction of a new block.  In particular, the
// serialized data used with the OP_RETURN starts with the block height and is
// followed by 8 bytes of cryptographically random data.
func treasurybaseOpReturnScript(blockHeight uint32) []byte {
	data := make([]byte, 12)
	binary.LittleEndian.PutUint32(data[0:4], blockHeight)
	binary.LittleEndian.PutUint64(data[4:12], rand.Uint64())
	return opReturnScript(data)
}

// treasurySpendOpReturnScript returns a script suitable for use as the first
// output of a treasury spend transaction.  In particular, the serialized data
// used with the OP_RETURN starts with the total spend amount and is followed by
// 24 bytes of cryptographically random data.
func treasurySpendOpReturnScript(amount int64) []byte {
	data := make([]byte, 32)
	binary.LittleEndian.PutUint64(data[0:8], uint64(amount))
	rand.Read(data[8:])
	return opReturnScript(data)
}

// treasurySpendSignature returns a treasury spend signature script with the
// provided signature and public key.
func treasurySpendSignature(sig, pubKey []byte) []byte {
	builder := txscript.NewScriptBuilder()
	builder.AddData(sig)
	builder.AddData(pubKey)
	builder.AddOp(txscript.OP_TSPEND)
	script, err := builder.Script()
	if err != nil {
		panic(err)
	}
	return script
}

// fakeTreasurySpendSignature returns a signature script that is valid enough to
// pass all checks, but would fail if actually checked.  This identification
// funcs in this package do not verify signatures, so valid signatures are not
// required for the tests.
func fakeTreasurySpendSignature() []byte {
	return treasurySpendSignature(validSignature, publicKey)
}

// newTxOut returns a new transaction output with the given parameters.
func newTxOut(amount int64, pkScriptVer uint16, pkScript []byte) *wire.TxOut {
	return &wire.TxOut{
		Value:    amount,
		Version:  pkScriptVer,
		PkScript: pkScript,
	}
}

var (
	// opTrueScript is a simple public key script that contains the OP_TRUE
	// opcode.
	opTrueScript = []byte{txscript.OP_TRUE}

	// p2shOpTrueAddr is a pay-to-script-hash address that can be redeemed with
	// [opTrueScript].
	p2shOpTrueAddr = func() *stdaddr.AddressScriptHashV0 {
		params := chaincfg.RegNetParams()
		addr, err := stdaddr.NewAddressScriptHashV0(opTrueScript, params)
		if err != nil {
			panic(err)
		}
		return addr
	}()

	// baseTreasuryAddTx is a valid treasury add transaction that includes a
	// change output.  It is used as a base to be further manipulated in the
	// tests.
	baseTreasuryAddTx = func() *wire.MsgTx {
		changeScriptVer, changeScript := p2shOpTrueAddr.StakeChangeScript()

		tx := wire.NewMsgTx()
		tx.Version = wire.TxVersionTreasury
		tx.AddTxIn(&wire.TxIn{}) // One input required
		tx.AddTxOut(newTxOut(0, 0, []byte{txscript.OP_TADD}))
		tx.AddTxOut(newTxOut(1, changeScriptVer, changeScript))
		return tx
	}()

	// baseTreasuryBaseTx is a valid treasury base transaction that commits to a
	// random height.  It is used as a base to be further manipulated in the
	// tests.
	baseTreasuryBaseTx = func() *wire.MsgTx {
		tx := wire.NewMsgTx()
		tx.Version = wire.TxVersionTreasury
		tx.AddTxIn(&wire.TxIn{
			// Treasurybase transactions have no inputs, so previous outpoint is
			// zero hash and max index.
			PreviousOutPoint: *wire.NewOutPoint(zeroHash, wire.MaxPrevOutIndex,
				wire.TxTreeRegular),
			Sequence:        wire.MaxTxInSequenceNum,
			ValueIn:         0,
			BlockHeight:     wire.NullBlockHeight,
			BlockIndex:      wire.NullBlockIndex,
			SignatureScript: nil, // Must be nil by consensus.
		})
		tx.AddTxOut(newTxOut(0, 0, []byte{txscript.OP_TADD}))
		tx.AddTxOut(newTxOut(0, 0, treasurybaseOpReturnScript(rand.Uint32())))
		return tx
	}()

	// baseTreasurySpendTx is a valid treasury spend transaction that pays to a
	// p2sh script.  It is used as a base to be further manipulated in the
	// tests.
	baseTreasurySpendTx = func() *wire.MsgTx {
		const payout = 1e8
		const fee = 5000
		payoutScriptVer, payoutScript := p2shOpTrueAddr.PayFromTreasuryScript()

		tx := wire.NewMsgTx()
		tx.Version = wire.TxVersionTreasury
		tx.AddTxIn(&wire.TxIn{
			// Treasury spend transactions have no inputs, so previous outpoint
			// is zero hash and max index.
			PreviousOutPoint: *wire.NewOutPoint(zeroHash, wire.MaxPrevOutIndex,
				wire.TxTreeRegular),
			Sequence:        wire.MaxTxInSequenceNum,
			ValueIn:         fee + payout,
			BlockHeight:     wire.NullBlockHeight,
			BlockIndex:      wire.NullBlockIndex,
			SignatureScript: fakeTreasurySpendSignature(),
		})
		tx.AddTxOut(newTxOut(0, 0, treasurySpendOpReturnScript(payout)))
		tx.AddTxOut(newTxOut(0, payoutScriptVer, payoutScript))
		return tx
	}()
)

// TestTreasuryIsFunctions confirms the various treasury transaction type
// identification functions return the expected results.  Each transaction is
// tested against all funcs to help ensure none of them are incorrectly detected
// as any other.
func TestTreasuryIsFunctions(t *testing.T) {
	tests := []struct {
		name          string      // test description
		tx            *wire.MsgTx // transaction to test
		treasuryAdd   bool        // expected check is treasury add
		treasuryBase  bool        // expected is treasury base
		treasurySpend bool        // expected is treasury spend
	}{{
		name:        "treasury add from user with change",
		tx:          baseTreasuryAddTx,
		treasuryAdd: true,
	}, {
		name: "treasury add from user with no change",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryAddTx.Copy()
			tx.TxOut = tx.TxOut[:1]
			return tx
		}(),
		treasuryAdd: true,
	}, {
		// This passes stakebase checks but is NOT a valid TADD.
		name: "treasury add from user with OP_RETURN",
		tx: func() *wire.MsgTx {
			params := chaincfg.RegNetParams()

			const voteSubsidy = 1e8
			const ticketPrice = 2e8
			tx := baseTreasuryBaseTx.Copy()
			tx.TxIn[0].ValueIn = voteSubsidy
			tx.TxIn[0].SignatureScript = params.StakeBaseSigScript
			tx.AddTxIn(&wire.TxIn{
				PreviousOutPoint: *wire.NewOutPoint(zeroHash, 0, wire.TxTreeStake),
				Sequence:         wire.MaxTxInSequenceNum,
				ValueIn:          ticketPrice,
				BlockHeight:      wire.NullBlockHeight,
				BlockIndex:       wire.NullBlockIndex,
				SignatureScript:  opTrueScript,
			})
			if !IsStakeBase(tx) {
				panic("transaction does not pass stakebase checks")
			}
			return tx
		}(),
	}, {
		name:         "treasury add from treasurybase",
		tx:           baseTreasuryBaseTx,
		treasuryBase: true,
	}, {
		name:          "treasury spend p2sh",
		tx:            baseTreasurySpendTx,
		treasurySpend: true,
	}, {
		name: "treasury spend p2pkh",
		tx: func() *wire.MsgTx {
			params := chaincfg.RegNetParams()
			pkHash := stdaddr.Hash160(publicKey)
			p2pkhAddr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
				pkHash, params)
			if err != nil {
				panic(err)
			}
			payoutScriptVer, payoutScript := p2pkhAddr.PayFromTreasuryScript()

			tx := baseTreasurySpendTx.Copy()
			tx.TxOut[1].Version = payoutScriptVer
			tx.TxOut[1].PkScript = payoutScript
			return tx
		}(),
		treasurySpend: true,
	}, {
		name: "treasury spend invalid output 1 p2pk (not p2sh/p2pkh)",
		tx: func() *wire.MsgTx {
			// Start with a normal payment script for the p2pk and manually add
			// the OP_TGEN prefix since there is no standard method to create
			// the pay from treasury script on a p2pk address given it is
			// invalid.
			params := chaincfg.RegNetParams()
			p2pkAddr, err := stdaddr.NewAddressPubKeyEcdsaSecp256k1V0Raw(
				publicKey, params)
			if err != nil {
				panic(err)
			}
			payoutScriptVer, payScript := p2pkAddr.PaymentScript()
			payoutScript := make([]byte, len(payScript)+1)
			payoutScript[0] = txscript.OP_TGEN
			copy(payoutScript[1:], payScript)

			tx := baseTreasurySpendTx.Copy()
			tx.TxOut[1].Version = payoutScriptVer
			tx.TxOut[1].PkScript = payoutScript
			return tx
		}(),
	}}

	for _, test := range tests {
		gotTreasuryAdd := IsTAdd(test.tx)
		if gotTreasuryAdd != test.treasuryAdd {
			t.Errorf("%s: unexpected treasury add result - got %v, want %v",
				test.name, gotTreasuryAdd, test.treasuryAdd)
		}

		gotTreasuryBase := IsTreasuryBase(test.tx)
		if gotTreasuryBase != test.treasuryBase {
			t.Errorf("%s: unexpected treasurybase result - got %v, want %v",
				test.name, gotTreasuryBase, test.treasuryBase)
		}

		gotTreasurySpend := IsTSpend(test.tx)
		if gotTreasurySpend != test.treasurySpend {
			t.Errorf("%s: unexpected treasury spend result - got %v, want %v",
				test.name, gotTreasurySpend, test.treasurySpend)
		}
	}
}

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
