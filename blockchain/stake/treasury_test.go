// Copyright (c) 2020-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"encoding/binary"
	"errors"
	"math/rand"
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
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

// TestTreasurySpendErrors verifies that all check treasury spend errors can be
// hit and return the proper error.
func TestTreasurySpendErrors(t *testing.T) {
	tests := []struct {
		name     string      // test description
		tx       *wire.MsgTx // transaction to test
		expected error       // expected error
	}{{
		name: "treasury spend invalid tx version",
		tx: func() *wire.MsgTx {
			tx := baseTreasurySpendTx.Copy()
			tx.Version = 1
			return tx
		}(),
		expected: ErrTSpendInvalidTxVersion,
	}, {
		name: "treasury spend with invalid num inputs",
		tx: func() *wire.MsgTx {
			tx := baseTreasurySpendTx.Copy()
			tx.TxIn = nil
			return tx
		}(),
		expected: ErrTSpendInvalidLength,
	}, {
		name: "treasury spend with invalid num outputs",
		tx: func() *wire.MsgTx {
			tx := baseTreasurySpendTx.Copy()
			tx.TxOut = nil
			return tx
		}(),
		expected: ErrTSpendInvalidLength,
	}, {
		name: "treasury spend with an invalid script version",
		tx: func() *wire.MsgTx {
			tx := baseTreasurySpendTx.Copy()
			tx.TxOut[1].Version = 1
			return tx
		}(),
		expected: ErrTSpendInvalidVersion,
	}, {
		name: "treasury spend with invalid output - no pubkey script",
		tx: func() *wire.MsgTx {
			tx := baseTreasurySpendTx.Copy()
			tx.TxOut[1].PkScript = nil
			return tx
		}(),
		expected: ErrTSpendInvalidScriptLength,
	}, {
		name: "treasury spend invalid input sig script - wrong script length",
		tx: func() *wire.MsgTx {
			sig := treasurySpendSignature(validSignature, nil)
			tx := baseTreasurySpendTx.Copy()
			tx.TxIn[0].SignatureScript = sig
			return tx
		}(),
		expected: ErrTSpendInvalidScript,
	}, {
		name: "treasury spend input sig script invalid - wrong sig len",
		tx: func() *wire.MsgTx {
			tx := baseTreasurySpendTx.Copy()
			sig := tx.TxIn[0].SignatureScript
			if sig[0] != txscript.OP_DATA_64 {
				panic("signature script format changed")
			}
			sig[0] = txscript.OP_DATA_65 // Wrong length.
			return tx
		}(),
		expected: ErrTSpendInvalidScript,
	}, {
		name: "treasury spend input sig script invalid - wrong pubkey len",
		tx: func() *wire.MsgTx {
			tx := baseTreasurySpendTx.Copy()
			sig := tx.TxIn[0].SignatureScript
			if sig[65] != txscript.OP_DATA_33 {
				panic("signature script format changed")
			}
			sig[65] = txscript.OP_DATA_34 // Wrong length.
			return tx
		}(),
		expected: ErrTSpendInvalidScript,
	}, {
		name: "treasury spend input sig invalid - wrong opcode for OP_TSPEND",
		tx: func() *wire.MsgTx {
			tx := baseTreasurySpendTx.Copy()
			sig := tx.TxIn[0].SignatureScript
			if sig[len(sig)-1] != txscript.OP_TSPEND {
				panic("signature script format changed")
			}
			sig[len(sig)-1] = txscript.OP_RETURN // Wrong opcode.
			return tx
		}(),
		expected: ErrTSpendInvalidScript,
	}, {
		name: "treasury spend input sig invalid - no tspend opcode",
		tx: func() *wire.MsgTx {
			tx := baseTreasurySpendTx.Copy()
			sig := tx.TxIn[0].SignatureScript
			tx.TxIn[0].SignatureScript = sig[:len(sig)-1]
			return tx
		}(),
		expected: ErrTSpendInvalidScript,
	}, {
		name: "treasury spend input sig invalid - two tspend opcodes",
		tx: func() *wire.MsgTx {
			tx := baseTreasurySpendTx.Copy()
			sig := tx.TxIn[0].SignatureScript
			sig = append(sig, txscript.OP_TSPEND)
			tx.TxIn[0].SignatureScript = sig
			return tx
		}(),
		expected: ErrTSpendInvalidScript,
	}, {
		name: "treasury spend input sig invalid - trailing data",
		tx: func() *wire.MsgTx {
			tx := baseTreasurySpendTx.Copy()
			sig := tx.TxIn[0].SignatureScript
			sig = append(sig, 0x01)
			tx.TxIn[0].SignatureScript = sig
			return tx
		}(),
		expected: ErrTSpendInvalidScript,
	}, {
		name: "treasury spend input sig script invalid - bad pubkey type",
		tx: func() *wire.MsgTx {
			pubKey := make([]byte, len(publicKey))
			copy(pubKey, publicKey)
			pubKey[0] |= 0x04
			sig := treasurySpendSignature(validSignature, pubKey)

			tx := baseTreasurySpendTx.Copy()
			tx.TxIn[0].SignatureScript = sig
			return tx
		}(),
		expected: ErrTSpendInvalidPubkey,
	}, {
		name: "treasury spend invalid - extra empty output",
		tx: func() *wire.MsgTx {
			tx := baseTreasurySpendTx.Copy()
			tx.AddTxOut(&wire.TxOut{})
			return tx
		}(),
		expected: ErrTSpendInvalidScriptLength,
	}, {
		name: "treasury spend invalid OP_RETURN output - short one byte",
		tx: func() *wire.MsgTx {
			tx := baseTreasurySpendTx.Copy()
			script := tx.TxOut[0].PkScript
			script = script[:len(script)-1]
			tx.TxOut[0].PkScript = script
			return tx
		}(),
		expected: ErrTSpendInvalidTransaction,
	}, {
		name: "treasury spend payment output - wrong opcode for OP_TGEN",
		tx: func() *wire.MsgTx {
			tx := baseTreasurySpendTx.Copy()
			if tx.TxOut[1].PkScript[0] != txscript.OP_TGEN {
				panic("payment output format changed")
			}
			tx.TxOut[1].PkScript[0] = txscript.OP_RETURN
			return tx
		}(),
		expected: ErrTSpendInvalidTGen,
	}, {
		name: "treasury spend payment output - unsupported p2pk",
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
		expected: ErrTSpendInvalidSpendScript,
	}}

	for _, test := range tests {
		err := checkTSpend(test.tx)
		if !errors.Is(err, test.expected) {
			t.Errorf("%q: unexpected error -- got %v, want %v", test.name, err,
				test.expected)
		}
		if IsTSpend(test.tx) {
			t.Errorf("%q: IsTSpend claimed an invalid treasury spend is valid",
				test.name)
		}
	}
}

// TestTreasuryAddErrors verifies that all check treasury add errors can be hit
// and return the proper error.
func TestTreasuryAddErrors(t *testing.T) {
	tests := []struct {
		name     string
		tx       *wire.MsgTx
		expected error
	}{{
		name: "treasury add invalid tx version",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryAddTx.Copy()
			tx.Version = 1
			return tx
		}(),
		expected: ErrTAddInvalidTxVersion,
	}, {
		name: "treasury add invalid num outputs - none",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryAddTx.Copy()
			tx.TxOut = nil
			return tx
		}(),
		expected: ErrTAddInvalidCount,
	}, {
		name: "treasury add invalid num outputs - two change outputs",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryAddTx.Copy()
			tx.AddTxOut(tx.TxOut[1])
			return tx
		}(),
		expected: ErrTAddInvalidCount,
	}, {
		name: "treasury add invalid num inputs - none",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryAddTx.Copy()
			tx.TxIn = nil
			return tx
		}(),
		expected: ErrTAddInvalidCount,
	}, {
		name: "treasury add with invalid output - bad script version",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryAddTx.Copy()
			tx.TxOut[0].Version = 1
			return tx
		}(),
		expected: ErrTAddInvalidVersion,
	}, {
		name: "treasury add with invalid output - missing script",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryAddTx.Copy()
			tx.TxOut[0].PkScript = nil
			return tx
		}(),
		expected: ErrTAddInvalidScriptLength,
	}, {
		name: "treasury add with invalid output - extra trailing byte",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryAddTx.Copy()
			tx.TxOut[0].PkScript = append(tx.TxOut[0].PkScript, txscript.OP_TRUE)
			return tx
		}(),
		expected: ErrTAddInvalidLength,
	}, {
		name: "treasury add with invalid output - wrong opcode for OP_TADD",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryAddTx.Copy()
			if tx.TxOut[0].PkScript[0] != txscript.OP_TADD {
				panic("public key script format changed")
			}
			tx.TxOut[0].PkScript[0] = txscript.OP_TSPEND
			return tx
		}(),
		expected: ErrTAddInvalidOpcode,
	}, {
		name: "treasury add with invalid output - wrong opcode for change",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryAddTx.Copy()
			if tx.TxOut[1].PkScript[0] != txscript.OP_SSTXCHANGE {
				panic("public key script format changed")
			}
			tx.TxOut[1].PkScript = tx.TxOut[1].PkScript[1:]
			return tx
		}(),
		expected: ErrTAddInvalidChange,
	}}

	for _, test := range tests {
		err := CheckTAdd(test.tx)
		if !errors.Is(err, test.expected) {
			t.Errorf("%q: unexpected error -- got %v, want %v", test.name, err,
				test.expected)
		}
		if IsTAdd(test.tx) {
			t.Errorf("%q: IsTAdd claimed an invalid tadd is valid", test.name)
		}
	}
}

// TestTreasuryBaseErrors verifies that all check treasurybase errors can be hit
// and return the proper error.
func TestTreasuryBaseErrors(t *testing.T) {
	tests := []struct {
		name     string
		tx       *wire.MsgTx
		expected error
	}{{
		name: "treasurybase invalid tx version",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryBaseTx.Copy()
			tx.Version = 1
			return tx
		}(),
		expected: ErrTreasuryBaseInvalidTxVersion,
	}, {
		name: "treasurybase invalid num inputs - none",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryBaseTx.Copy()
			tx.TxIn = nil
			return tx
		}(),
		expected: ErrTreasuryBaseInvalidCount,
	}, {
		name: "treasurybase invalid num outputs - none",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryBaseTx.Copy()
			tx.TxOut = nil
			return tx
		}(),
		expected: ErrTreasuryBaseInvalidCount,
	}, {
		name: "treasurybase invalid num outputs - extra outupt",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryBaseTx.Copy()
			tx.TxOut = append(tx.TxOut, newTxOut(1, 0, opTrueScript))
			return tx
		}(),
		expected: ErrTreasuryBaseInvalidCount,
	}, {
		name: "treasurybase invalid input 0 - non-empty signature script",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryBaseTx.Copy()
			tx.TxIn[0].SignatureScript = []byte{txscript.OP_TRUE}
			return tx
		}(),
		expected: ErrTreasuryBaseInvalidLength,
	}, {
		name: "treasurybase invalid output - bad script version",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryBaseTx.Copy()
			tx.TxOut[1].Version = 2
			return tx
		}(),
		expected: ErrTreasuryBaseInvalidVersion,
	}, {
		name: "treasurybase invalid output 0 - wrong opcode for OP_TADD",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryBaseTx.Copy()
			if tx.TxOut[0].PkScript[0] != txscript.OP_TADD {
				panic("public key script format changed")
			}
			tx.TxOut[0].PkScript[0] = txscript.OP_TSPEND
			return tx
		}(),
		expected: ErrTreasuryBaseInvalidOpcode0,
	}, {
		name: "treasurybase invalid output 0 - extra trailing byte",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryBaseTx.Copy()
			tx.TxOut[0].PkScript = append(tx.TxOut[0].PkScript, txscript.OP_TRUE)
			return tx
		}(),
		expected: ErrTreasuryBaseInvalidOpcode0,
	}, {
		name: "treasurybase invalid output 1 - wrong opcode for OP_RETURN",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryBaseTx.Copy()
			if tx.TxOut[1].PkScript[0] != txscript.OP_RETURN {
				panic("public key script format changed")
			}
			tx.TxOut[1].PkScript[0] = txscript.OP_TADD
			return tx
		}(),
		expected: ErrTreasuryBaseInvalidOpcode1,
	}, {
		name: "treasurybase invalid output 1 - extra trailing byte",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryBaseTx.Copy()
			tx.TxOut[1].PkScript = append(tx.TxOut[1].PkScript, txscript.OP_TRUE)
			return tx
		}(),
		expected: ErrTreasuryBaseInvalidOpcode1,
	}, {
		name: "treasurybase invalid output 1 - wrong data push size",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryBaseTx.Copy()
			if tx.TxOut[1].PkScript[1] != txscript.OP_DATA_12 {
				panic("public key script format changed")
			}
			tx.TxOut[1].PkScript[1] = txscript.OP_DATA_11
			return tx
		}(),
		expected: ErrTreasuryBaseInvalidOpcode1,
	}, {
		name: "treasurybase invalid input 0 - non-null hash",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryBaseTx.Copy()
			tx.TxIn[0].PreviousOutPoint.Hash[0] = 0x01
			return tx
		}(),
		expected: ErrTreasuryBaseInvalid,
	}, {
		name: "treasurybase invalid input 0 - wrong prev index",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryBaseTx.Copy()
			tx.TxIn[0].PreviousOutPoint.Index = 1
			return tx
		}(),
		expected: ErrTreasuryBaseInvalid,
	}, {
		name: "treasurybase invalid input 0 - wrong prev tree",
		tx: func() *wire.MsgTx {
			tx := baseTreasuryBaseTx.Copy()
			tx.TxIn[0].PreviousOutPoint.Tree = wire.TxTreeStake
			return tx
		}(),
		expected: ErrTreasuryBaseInvalid,
	}}
	for _, test := range tests {
		err := checkTreasuryBase(test.tx)
		if !errors.Is(err, test.expected) {
			t.Errorf("%q: unexpected error -- got %v, want %v", test.name, err,
				test.expected)
		}
		if IsTreasuryBase(test.tx) {
			t.Errorf("%q: IsTreasuryBase claimed an invalid treasury base is "+
				"valid", test.name)
		}
	}
}
