// Copyright (c) 2020-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/blockchain/v5/chaingen"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/container/lru"
	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/gcs/v4/blockcf2"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/sign"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

var (
	// These keys come from chaincfg/regnetparams.go
	privKey  = hexToBytes("68ab7efdac0eb99b1edf83b23374cc7a9c8d0a4183a2627afc8ea0437b20589e")
	privKey2 = hexToBytes("2527f13f61024c9b9f4b30186f16e0b0af35b08c54ed2ed67def863b447ea11b")
)

// TestTreasuryValueTypeDebits ensures the IsDebit() method of
// treasuryValueType works as expected.
func TestTreasuryValueTypeDebits(t *testing.T) {
	tests := []struct {
		name      string
		typ       treasuryValueType
		wantDebit bool
	}{{
		name:      "treasuryValueTBase",
		typ:       treasuryValueTBase,
		wantDebit: false,
	}, {
		name:      "treasuryValueTAdd",
		typ:       treasuryValueTAdd,
		wantDebit: false,
	}, {
		name:      "treasuryValueFee",
		typ:       treasuryValueFee,
		wantDebit: true,
	}, {
		name:      "treasuryValueTSpend",
		typ:       treasuryValueTSpend,
		wantDebit: true,
	}}

	for _, test := range tests {
		gotDebit := test.typ.IsDebit()
		if gotDebit != test.wantDebit {
			t.Fatalf("mismatched IsDebit result for %s -- got %v, "+
				"want %v", test.name, gotDebit, test.wantDebit)
		}
	}
}

// TestTreasuryStateSerialization ensures serializing and deserializing treasury
// states works as expected, including the possible error conditions.
func TestTreasuryStateSerialization(t *testing.T) {
	tests := []struct {
		name       string
		state      *treasuryState
		serialized []byte
		encodeErr  error
		decodeErr  error
	}{{
		name: "equal",
		state: &treasuryState{
			balance: 100,
			values: []treasuryValue{
				{treasuryValueTBase, 1},
				{treasuryValueTAdd, 2},
				{treasuryValueTAdd, 3},
				{treasuryValueTSpend, -3},
				{treasuryValueFee, -2},
			},
		},
		serialized: hexToBytes("640501010202020304030302"),
	}, {
		name: "negative balance",
		state: &treasuryState{
			balance: -100,
			values: []treasuryValue{
				{treasuryValueTBase, 1},
				{treasuryValueTAdd, 2},
				{treasuryValueTAdd, 3},
				{treasuryValueTSpend, -3},
				{treasuryValueFee, -2},
			},
		},
		encodeErr: errDbTreasury(""),
		decodeErr: errDeserialize(""),
	}, {
		name:       "empty data",
		state:      nil,
		serialized: hexToBytes(""),
		decodeErr:  errDeserialize(""),
	}, {
		name:       "no data after balance",
		state:      nil,
		serialized: hexToBytes("64"),
		decodeErr:  errDeserialize(""),
	}, {
		name:       "no data after num values",
		state:      nil,
		serialized: hexToBytes("6401"),
		decodeErr:  errDeserialize(""),
	}, {
		name:       "no data after first value",
		state:      nil,
		serialized: hexToBytes("640201"),
		decodeErr:  errDeserialize(""),
	}, {
		name: "trying to serialize incorrect negative tbase",
		state: &treasuryState{
			values: []treasuryValue{{typ: treasuryValueTBase, amount: -1}},
		},
		encodeErr:  errDbTreasury(""),
		serialized: nil,
		decodeErr:  errDeserialize(""),
	}, {
		name: "trying to serialize incorrect negative tadd",
		state: &treasuryState{
			values: []treasuryValue{{typ: treasuryValueTAdd, amount: -1}},
		},
		encodeErr:  errDbTreasury(""),
		serialized: nil,
		decodeErr:  errDeserialize(""),
	}, {
		name: "trying to serialize incorrect positive tspend",
		state: &treasuryState{
			values: []treasuryValue{{typ: treasuryValueTSpend, amount: 1}},
		},
		encodeErr:  errDbTreasury(""),
		serialized: nil,
		decodeErr:  errDeserialize(""),
	}, {
		name: "trying to serialize incorrect positive fee",
		state: &treasuryState{
			values: []treasuryValue{{typ: treasuryValueFee, amount: 1}},
		},
		encodeErr:  errDbTreasury(""),
		serialized: nil,
		decodeErr:  errDeserialize(""),
	}, {
		name: "serialize zero amounts for all value types",
		state: &treasuryState{
			balance: 100,
			values: []treasuryValue{
				{typ: treasuryValueTBase, amount: 0},
				{typ: treasuryValueTAdd, amount: 0},
				{typ: treasuryValueTSpend, amount: 0},
				{typ: treasuryValueFee, amount: 0},
			},
		},
		serialized: hexToBytes("64040100020004000300"),
	}}

	for _, test := range tests {
		// Ensure the serialized bytes are deserialized back to the expected
		// treasury state.
		gotState, err := deserializeTreasuryState(test.serialized)
		if !errors.Is(err, test.decodeErr) {
			t.Errorf("%s: mismatched err -- got %v, want %v", test.name, err,
				test.decodeErr)
			continue
		}
		if test.decodeErr == nil && !reflect.DeepEqual(gotState, test.state) {
			t.Errorf("%s: mismatched states\ngot %+v\nwant %+v", test.name,
				gotState, test.state)
			continue
		}

		// Skip serialization tests for nil states.
		if test.state == nil {
			continue
		}

		// Ensure the treasury state serializes to the expected value.
		gotSerialized, err := serializeTreasuryState(*test.state)
		if !errors.Is(err, test.encodeErr) {
			t.Errorf("%s: mismatched err -- got %v, want %v", test.name, err,
				test.encodeErr)
		}
		if !bytes.Equal(gotSerialized, test.serialized) {
			t.Errorf("%s: did not get expected bytes - got %x, want %x",
				test.name, gotSerialized, test.serialized)
			continue
		}
	}
}

// TestTreasuryDatabase tests treasury database functionality.
func TestTreasuryDatabase(t *testing.T) {
	// Create a new database to store treasury state.
	dbPath := t.TempDir()
	net := chaincfg.RegNetParams().Net
	testDb, err := database.Create(testDbType, dbPath, net)
	if err != nil {
		t.Fatalf("error creating treasury db: %v", err)
	}
	defer testDb.Close()

	// Add bucket.
	err = testDb.Update(func(dbTx database.Tx) error {
		_, err := dbTx.Metadata().CreateBucketIfNotExists(treasuryBucketName)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Write maxTreasuryState records out.
	maxTreasuryState := uint64(1024)
	for i := uint64(0); i < maxTreasuryState; i++ {
		// Create synthetic treasury state
		ts := treasuryState{
			balance: int64(i),
			values: []treasuryValue{
				{typ: treasuryValueTBase, amount: int64(i)},
				{typ: treasuryValueTSpend, amount: -1 - int64(i)},
			},
		}

		// Create hash of counter.
		b := make([]byte, 16)
		binary.LittleEndian.PutUint64(b[0:], i)
		hash := chainhash.HashH(b)

		err = testDb.Update(func(dbTx database.Tx) error {
			return dbPutTreasuryBalance(dbTx, hash, ts)
		})
		if err != nil {
			t.Fatalf("%v", err)
		}
	}

	// Pull records back out.
	for i := uint64(0); i < maxTreasuryState; i++ {
		// Create synthetic treasury state
		ts := treasuryState{
			balance: int64(i),
			values: []treasuryValue{
				{typ: treasuryValueTBase, amount: int64(i)},
				{typ: treasuryValueTSpend, amount: -1 - int64(i)},
			},
		}

		// Create hash of counter.
		b := make([]byte, 16)
		binary.LittleEndian.PutUint64(b[0:], i)
		hash := chainhash.HashH(b)

		var tsr *treasuryState
		err = testDb.View(func(dbTx database.Tx) error {
			tsr, err = dbFetchTreasuryBalance(dbTx, hash)
			return err
		})
		if err != nil {
			t.Fatalf("%v", err)
		}

		if !reflect.DeepEqual(ts, *tsr) {
			t.Fatalf("not same treasury state got %v wanted %v", ts, *tsr)
		}
	}
}

// TestTspendDatabase tests tspend database functionality including
// serialization and deserialization.
func TestTSpendDatabase(t *testing.T) {
	// Create a new database to store treasury state.
	dbPath := t.TempDir()
	net := chaincfg.RegNetParams().Net
	testDb, err := database.Create(testDbType, dbPath, net)
	if err != nil {
		t.Fatalf("error creating tspend db: %v", err)
	}
	defer testDb.Close()

	// Add bucket.
	err = testDb.Update(func(dbTx database.Tx) error {
		_, err := dbTx.Metadata().CreateBucketIfNotExists(treasuryTSpendBucketName)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Write maxTSpendState records out.
	maxTSpendState := uint64(8)
	txHash := chainhash.Hash{}
	for i := uint64(0); i < maxTSpendState; i++ {
		// Create hash of counter.
		b := make([]byte, 16)
		binary.LittleEndian.PutUint64(b[0:], i)
		blockHash := chainhash.HashH(b)

		err = testDb.Update(func(dbTx database.Tx) error {
			return dbUpdateTSpend(dbTx, txHash, blockHash)
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Pull records back out.
	var hashes []chainhash.Hash
	err = testDb.View(func(dbTx database.Tx) error {
		hashes, err = dbFetchTSpend(dbTx, txHash)
		return err
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := uint64(0); i < maxTSpendState; i++ {
		// Create hash of counter.
		b := make([]byte, 16)
		binary.LittleEndian.PutUint64(b[0:], i)
		hash := chainhash.HashH(b)
		if !hash.IsEqual(&hashes[i]) {
			t.Fatalf("not same tspend hash got %v wanted %v", hashes[i], hash)
		}
	}
}

func TestTSpendVoteCount(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// Shorter versions of useful params for convenience.
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()
	startTip := g.TipName()

	// ---------------------------------------------------------------------
	// Create TSpend in "mempool"
	// ---------------------------------------------------------------------

	nextBlockHeight := g.Tip().Header.Height + 1
	const tspendAmount = 1000
	const tspendFee = 100
	expiry := standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	start, end, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}

	tspend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{Amount: tspendAmount - tspendFee}}, tspendFee, expiry)

	// ---------------------------------------------------------------------
	// Try to insert TSPEND while not on a TVI
	//
	//   ... -> bva19
	//                  \-> bnottvi0
	// ---------------------------------------------------------------------

	// Assert we are not on a TVI and generate block. This should fail.
	if standalone.IsTreasuryVoteInterval(uint64(nextBlockHeight), tvi) {
		t.Fatalf("expected !TVI %v", nextBlockHeight)
	}
	outs := g.OldestCoinbaseOuts()
	name := "bnottvi0"
	g.NextBlock(name, nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
	})
	g.RejectTipBlock(ErrNotTVI)

	// ---------------------------------------------------------------------
	// Generate enough blocks to get to TVI.
	//
	//   ... -> bva19 -> bpretvi0 -> bpretvi1
	//                  \-> bnottvi0
	// ---------------------------------------------------------------------

	// Generate votes up to TVI. This is legal however they should NOT be
	// counted in the totals since they are outside of the voting window.
	g.SetTip(startTip)
	for i := uint32(0); i < start-nextBlockHeight; i++ {
		name := fmt.Sprintf("bpretvi%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			tspend))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Add TSpend on first block of window. This should fail with not
	// enough votes.
	//
	//   ... -> bpretvi1
	//         \-> btvinotenough0
	// ---------------------------------------------------------------------

	// Assert we are on a TVI and generate block. This should fail.
	startTip = g.TipName()
	if standalone.IsTreasuryVoteInterval(uint64(g.Tip().Header.Height), tvi) {
		t.Fatalf("expected !TVI %v", g.Tip().Header.Height)
	}
	name = "btvinotenough0"
	g.NextBlock(name, nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
	})
	g.RejectTipBlock(ErrNotEnoughTSpendVotes)

	// ---------------------------------------------------------------------
	// Generate 1 TVI of No votes and add TSpend,
	//
	//   ... -> bpretvi1 -> btvi0 -> ... -> btvi3
	// ---------------------------------------------------------------------
	g.SetTip(startTip)
	for i := uint64(0); i < tvi; i++ {
		name := fmt.Sprintf("btvi%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendNoVotes(tspend))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// Assert we are on a TVI and generate block. This should fail.
	startTip = g.TipName()
	if standalone.IsTreasuryVoteInterval(uint64(g.Tip().Header.Height), tvi) {
		t.Fatalf("expected !TVI %v", g.Tip().Header.Height)
	}
	name = "btvinotenough1"
	g.NextBlock(name, nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
	})
	g.RejectTipBlock(ErrNotEnoughTSpendVotes)

	// ---------------------------------------------------------------------
	// Generate two more TVI of no votes.
	//
	//   ... -> btvinotenough0 -> btvi4 -> ... -> btvi7
	// ---------------------------------------------------------------------

	g.SetTip(startTip)
	for i := uint64(0); i < tvi*2; i++ {
		name := fmt.Sprintf("btvi%v", tvi+i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendNoVotes(tspend))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// Assert we are on a TVI and generate block. This should fail with No
	// vote (TSpend should not have been submitted).
	startTip = g.TipName()
	if standalone.IsTreasuryVoteInterval(uint64(g.Tip().Header.Height), tvi) {
		t.Fatalf("expected !TVI %v", g.Tip().Header.Height)
	}
	name = "btvienough0"
	g.NextBlock(name, nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
	})
	g.RejectTipBlock(ErrNotEnoughTSpendVotes)

	// Assert we have the correct number of votes and voting window.
	tv, err := g.chain.tSpendCountVotes(g.chain.bestChain.Tip(),
		dcrutil.NewTx(tspend))
	if err != nil {
		t.Fatal(err)
	}

	if start != tv.start {
		t.Fatalf("invalid start block got %v wanted %v", tv.start, start)
	}
	if end != tv.end {
		t.Fatalf("invalid end block got %v wanted %v", tv.end, end)
	}

	expectedYesVotes := 0 // We voted a bunch of times outside the window
	expectedNoVotes := tvi * mul * uint64(params.TicketsPerBlock)
	if expectedYesVotes != tv.yes {
		t.Fatalf("invalid yes votes got %v wanted %v", expectedYesVotes, tv.yes)
	}
	if expectedNoVotes != uint64(tv.no) {
		t.Fatalf("invalid no votes got %v wanted %v", expectedNoVotes, tv.no)
	}

	// ---------------------------------------------------------------------
	// Generate one more TVI and append expired TSpend.
	//
	//   ... -> bposttvi0 ... -> bposttvi3 ->
	//                                     \-> bexpired0
	// ---------------------------------------------------------------------

	g.SetTip(startTip)
	for i := uint64(0); i < tvi; i++ {
		name := fmt.Sprintf("bposttvi%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// Assert TSpend expired
	startTip = g.TipName()
	if standalone.IsTreasuryVoteInterval(uint64(g.Tip().Header.Height), tvi) {
		t.Fatalf("expected !TVI %v", g.Tip().Header.Height)
	}
	name = "bexpired0"
	g.NextBlock(name, nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
	})
	g.RejectTipBlock(ErrExpiredTx)

	// ---------------------------------------------------------------------
	// Create TSpend in "mempool"
	//
	// Test corner of quorum-1 vote and exact quorum yes vote.
	// ---------------------------------------------------------------------

	// Use exact hight to validate that tspend starts on next tvi.
	expiry = standalone.CalcTSpendExpiry(int64(g.Tip().Header.Height), tvi, mul)
	start, _, err = standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}

	// While here test that start is next tvi while on tvi.
	if g.Tip().Header.Height+uint32(tvi) != start {
		t.Fatalf("expected to see exactly next tvi got %v wanted %v",
			start, g.Tip().Header.Height+uint32(tvi))
	}

	tspend = g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{Amount: tspendAmount - tspendFee}}, tspendFee, expiry)

	// Fast forward to next tvi and add no votes which should not count.
	g.SetTip(startTip)
	for i := uint64(0); i < tvi; i++ {
		name := fmt.Sprintf("bnovote%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendNoVotes(tspend))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// Hit quorum-1 yes votes.
	maxVotes := uint32(params.TicketsPerBlock) * (tv.end - tv.start)
	quorum := uint64(maxVotes) * params.TreasuryVoteQuorumMultiplier /
		params.TreasuryVoteQuorumDivisor
	totalVotes := uint16(quorum - 1)
	for i := uint64(0); i < tvi; i++ {
		name := fmt.Sprintf("byesvote%v", i)
		g.NextBlock(name, nil, outs[1:], func(b *wire.MsgBlock) {
			numVotes := totalVotes
			if numVotes > params.TicketsPerBlock {
				numVotes = params.TicketsPerBlock
			}
			for i := uint16(0); i < numVotes; i++ {
				voteTx := b.STransactions[i+1]
				const voteYes = byte(stake.TreasuryVoteYes)
				chaingen.SetTreasurySpendVote(voteTx, tspend, voteYes)
			}
			totalVotes -= numVotes
		})
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// Verify we are one vote shy of quorum
	startTip = g.TipName()
	if standalone.IsTreasuryVoteInterval(uint64(g.Tip().Header.Height), tvi) {
		t.Fatalf("expected !TVI %v", g.Tip().Header.Height)
	}
	name = "bquorum0"
	g.NextBlock(name, nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
	})
	g.RejectTipBlock(ErrNotEnoughTSpendVotes)

	// Count votes.
	tv, err = g.chain.tSpendCountVotes(g.chain.bestChain.Tip(),
		dcrutil.NewTx(tspend))
	if err != nil {
		t.Fatal(err)
	}
	if int(quorum-1) != tv.yes {
		t.Fatalf("unexpected yesVote count got %v wanted %v",
			tv.yes, quorum-1)
	}

	// Hit exact yes vote quorum
	g.SetTip(startTip)
	totalVotes = uint16(1)
	for i := uint64(0); i < tvi*2; i++ {
		name := fmt.Sprintf("byesvote%v", tvi+i)
		g.NextBlock(name, nil, outs[1:], func(b *wire.MsgBlock) {
			numVotes := totalVotes
			if numVotes > params.TicketsPerBlock {
				numVotes = params.TicketsPerBlock
			}
			for i := uint16(0); i < numVotes; i++ {
				voteTx := b.STransactions[i+1]
				const voteYes = byte(stake.TreasuryVoteYes)
				chaingen.SetTreasurySpendVote(voteTx, tspend, voteYes)
			}
			totalVotes -= numVotes
		})
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// Count votes.
	tv, err = g.chain.tSpendCountVotes(g.chain.bestChain.Tip(),
		dcrutil.NewTx(tspend))
	if err != nil {
		t.Fatal(err)
	}
	if int(quorum) != tv.yes {
		t.Fatalf("unexpected yesVote count got %v wanted %v",
			tv.yes, quorum)
	}

	// Verify TSpend can be added exactly on quorum.
	if standalone.IsTreasuryVoteInterval(uint64(g.Tip().Header.Height), tvi) {
		t.Fatalf("expected !TVI %v", g.Tip().Header.Height)
	}
	name = "bquorum1"
	g.NextBlock(name, nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
	})
	g.AcceptTipBlock()
}

// TestTSpendEmptyTreasury tests that we can't generate a tspend that spends
// more funds than available in the treasury even when otherwise allowed by
// policy.
func TestTSpendEmptyTreasury(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// To ensure we can completely drain the treasury, increase the
	// bootstrap policy to allow a tspend with a _very_ high value.
	params.TreasuryExpenditureBootstrap = 21e6 * 1e8

	// Shorter versions of useful params for convenience.
	cbm := params.CoinbaseMaturity
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// Ensure the treasury balance is the expected value at this point.  It is
	// the number of treasurybases added minus the coinbase maturity all times
	// the amount of each treasurybase.
	tipHeight := g.Tip().Header.Height
	numTrsyBases := int64(tipHeight - 1)
	trsyBaseAmt := g.Tip().STransactions[0].TxIn[0].ValueIn
	expectedBal := (numTrsyBases - int64(cbm)) * trsyBaseAmt
	g.ExpectTreasuryBalance(expectedBal)

	// ---------------------------------------------------------------------
	// Create TSPEND in mempool for exact amount of treasury + 1 atom
	// ---------------------------------------------------------------------

	nextBlockHeight := tipHeight + 1
	expiry := standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	start, _, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}

	// Create a treasury spend that is exactly one atom too much.
	const spendFee = 10
	futureBlocks := tvi*mul + uint64(start-nextBlockHeight)
	futureTrsyBal := expectedBal + trsyBaseAmt*(int64(futureBlocks)+1)
	tspendAmount := futureTrsyBal - spendFee + 1
	payout := dcrutil.Amount(tspendAmount)
	payouts := []chaingen.AddressAmountTuple{{Amount: payout}}
	tspend := g.CreateTreasuryTSpend(privKey, payouts, spendFee, expiry)

	// ---------------------------------------------------------------------
	// Generate enough blocks to get to TVI.
	//
	//   ... -> bva19 -> bpretvi0 -> bpretvi1
	// ---------------------------------------------------------------------

	// Generate votes up to TVI. This is legal however they should NOT be
	// counted in the totals since they are outside of the voting window.
	outs := g.OldestCoinbaseOuts()
	for i := uint32(0); i < start-nextBlockHeight; i++ {
		name := fmt.Sprintf("bpretvi%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Generate a TVI worth of rewards and try to spend more.
	//
	//   ... -> b#
	//            \-> btoomuch0
	// ---------------------------------------------------------------------

	for i := uint64(0); i < tvi*mul; i++ {
		name := fmt.Sprintf("b%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			tspend))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// Ensure treasury balance for the next block is 1 atom less than calculated
	// amount.
	g.ExpectTreasuryBalance(tspendAmount + spendFee - trsyBaseAmt - 1)

	// Try spending 1 atom more than treasury balance.
	name := "btoomuch0"
	g.NextBlock(name, nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
	})
	g.RejectTipBlock(ErrInvalidExpenditure)
}

// TestExpendituresReorg tests that the correct treasury balance is tracked
// when reorgs remove tspends/tadds from the main chain. Test plan:
//
// - Approve and mine TSpend and TAdd
// - Reorg to chain without TSpend/Tadd
// - Mine until TSpend/TAdd would have been mature
// - Extend TSpend/TAdd chain and reorg back to it
func TestExpendituresReorg(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// Shorter versions of useful params for convenience.
	cbm := params.CoinbaseMaturity
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// Ensure the treasury balance is the expected value at this point.  It is
	// the number of treasurybases added minus the coinbase maturity all times
	// the amount of each treasurybase.
	tipHeight := g.Tip().Header.Height
	numTrsyBases := int64(tipHeight - 1)
	trsyBaseAmt := g.Tip().STransactions[0].TxIn[0].ValueIn
	expectedBal := (numTrsyBases - int64(cbm)) * trsyBaseAmt
	g.ExpectTreasuryBalance(expectedBal)

	// ---------------------------------------------------------------------
	// Generate enough blocks to have spendable outputs during the reorg
	// stage of the test. Due to having lowered the coinbase maturity,
	// we'll need to manually generate and store these outputs until we
	// have enough to create two sidechains in the future.
	//
	//   ... -> bsv# -> bout0 -> ... -> bout#
	// ---------------------------------------------------------------------

	outs := g.OldestCoinbaseOuts()
	neededBlocks := (cbm + 1) * 2
	neededOuts := neededBlocks * params.TicketsPerBlock
	oldOuts := make([][]chaingen.SpendableOut, 1)
	for i := uint32(0); i < uint32(neededOuts); i++ {
		name := fmt.Sprintf("bout%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		lastOuts := &oldOuts[len(oldOuts)-1]
		*lastOuts = append(*lastOuts, outs[0])
		if len(*lastOuts) == int(params.TicketsPerBlock) && i < uint32(neededOuts-1) {
			oldOuts = append(oldOuts, make([]chaingen.SpendableOut, 0, int(params.TicketsPerBlock)))
		}
		numTrsyBases++
	}

	// ---------------------------------------------------------------------
	// Generate enough blocks to get to TVI.
	//
	//   ... -> bout# -> bpretvi0 -> ... -> bpretvi#
	// ---------------------------------------------------------------------

	nextBlockHeight := g.Tip().Header.Height + 1
	expiry := standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	start, _, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}

	// Generate up to TVI blocks.
	for i := uint32(0); i < start-nextBlockHeight; i++ {
		name := fmt.Sprintf("bpretvi%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		numTrsyBases++
	}

	// Generate a TSpend for some amount.
	const tspendAmount = 1e7
	const tspendFee = 0
	tspend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{{
		Amount: tspendAmount - tspendFee}}, tspendFee, expiry)

	// Generate a TAdd for some amount.
	const taddAmount = 1701
	const taddFee = 0
	tadd := g.CreateTreasuryTAdd(&outs[0], taddAmount, taddFee)

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the tspend.
	//
	// ... -> bpretvi# -> bv0 -> ... -> bv#
	// ---------------------------------------------------------------------

	for i := uint64(0); i < tvi*mul; i++ {
		name := fmt.Sprintf("bv%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			tspend))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		numTrsyBases++
	}

	// Remember the block immediately before the TSpend is mined so we can
	// create sidechains.
	preTSpendBlock := g.TipName()

	// ---------------------------------------------------------------------
	// Generate a block that includes the tspend and tadd.
	//
	// ... -> bvn -> btspend
	// ---------------------------------------------------------------------
	g.NextBlock("btspend", nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
		b.AddSTransaction(tadd)
	})
	g.AcceptTipBlock()

	// Balance only reflects the tbase added so far. We add 1 to
	// tbaseBlocks to account for the `btspend` block.
	wantBalance := (numTrsyBases + 1 - int64(cbm)) * trsyBaseAmt
	g.ExpectTreasuryBalance(wantBalance)

	// ---------------------------------------------------------------------
	// Generate a sidechain that does _not_ include the TSpend and mine it
	// all the way to when the TSpend would have been mature and ensure the
	// treasury balance does _not_ reflect the reorged txs.
	//
	// ... -> bvn -> btspend
	//           \-> bnotxs0 -> ... -> bnotxs#
	// ---------------------------------------------------------------------
	g.SetTip(preTSpendBlock)
	blocksToTreasuryChange := uint64(cbm + 1)
	for i := uint64(0); i < blocksToTreasuryChange; i++ {
		name := fmt.Sprintf("bnotxs%v", i)
		g.NextBlock(name, nil, oldOuts[0], chaingen.AddTreasurySpendYesVotes(
			tspend))
		switch i {
		case 0:
			// The first block creates a sidechain.
			g.AcceptedToSideChainWithExpectedTip("btspend")
		default:
			g.AcceptTipBlock()
		}
		oldOuts = oldOuts[1:]
		numTrsyBases++
	}

	// Given we reorged to a side chain that does _not_ include the TSpend
	// and TAdd, we expect the current balance to have only tbases.
	wantBalance = (numTrsyBases - int64(cbm)) * trsyBaseAmt
	g.ExpectTreasuryBalance(wantBalance)

	// ---------------------------------------------------------------------
	// Extend the chain starting at btspend again, until it becomes the
	// main chain again.
	//
	// ... -> bvn -> btspend -> btspend0 -> ... -> btspend#
	//           \-> bnotxs0 -> ........................... -> bnotxs#
	// ---------------------------------------------------------------------
	tip := g.TipName()
	g.SetTip("btspend")
	for i := uint64(0); i < blocksToTreasuryChange; i++ {
		name := fmt.Sprintf("btspend%v", i)
		g.NextBlock(name, nil, oldOuts[0], chaingen.AddTreasurySpendYesVotes(
			tspend))

		switch {
		case i < blocksToTreasuryChange-1:
			// Only the last block changes tip.
			g.AcceptedToSideChainWithExpectedTip(tip)
		default:
			g.AcceptTipBlock()
		}
		oldOuts = oldOuts[1:]
	}

	// We mined one more block vs the previous chain.
	numTrsyBases++

	// We reorged back to the chain that includes the mature TSpend/TAdd so
	// now ensure the treasury balance reflects their presence.
	wantBalance = (numTrsyBases-int64(cbm))*trsyBaseAmt - int64(tspendAmount) +
		taddAmount
	g.ExpectTreasuryBalance(wantBalance)
}

// TestSpendableTreasuryTxs tests that the outputs of mined TSpends and TAdds
// are actually spendable by transactions that can be mined.
//
// - Approve and mine TSpend and TAdd
// - Mine until TSpend/TAdd are mature
// - Create a tx that spends from the TSpend and TAdd change.
func TestSpendableTreasuryTxs(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// Shorter versions of useful params for convenience.
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// ---------------------------------------------------------------------
	// Generate enough blocks to get to TVI.
	//
	//   ... -> bsv# -> bpretvi#
	// ---------------------------------------------------------------------

	nextBlockHeight := g.Tip().Header.Height + 1
	expiry := standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	start, _, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}

	// Generate up to TVI blocks.
	outs := g.OldestCoinbaseOuts()
	taddOut1 := outs[0] // Save this output for a tadd.
	for i := uint32(0); i < start-nextBlockHeight; i++ {
		name := fmt.Sprintf("bpretvi%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// Create P2PKH and P2SH scripts to test spendability of TSpend and
	// TAdd outputs. Each one is unique so we can do a cfilter test after
	// mining these txs.
	spendPrivKey := secp256k1.NewPrivateKey(new(secp256k1.ModNScalar).SetInt(1))
	pubKey := spendPrivKey.PubKey().SerializeCompressed()
	pubKeyHash := stdaddr.Hash160(pubKey)
	spendP2pkhAddr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
		pubKeyHash, params)
	if err != nil {
		t.Fatal(err)
	}
	spendP2shScript := []byte{txscript.OP_NOP, txscript.OP_TRUE}
	spendP2shAddr, err := stdaddr.NewAddressScriptHashV0(spendP2shScript,
		params)
	if err != nil {
		t.Fatal(err)
	}

	addPrivKey := secp256k1.NewPrivateKey(new(secp256k1.ModNScalar).SetInt(2))
	pubKey = addPrivKey.PubKey().SerializeCompressed()
	pubKeyHash = stdaddr.Hash160(pubKey)
	addP2pkhAddr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(
		pubKeyHash, params)
	if err != nil {
		t.Fatal(err)
	}
	addP2shScript := []byte{txscript.OP_NOP, txscript.OP_NOP, txscript.OP_TRUE}
	addP2shAddr, err := stdaddr.NewAddressScriptHashV0(addP2shScript, params)
	if err != nil {
		t.Fatal(err)
	}

	// Generate a TSpend for some amount.
	const tspendAmount = 1e7
	const tspendFee = 0
	tspend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{Amount: tspendAmount - tspendFee, Address: spendP2pkhAddr},
		{Amount: tspendAmount - tspendFee, Address: spendP2shAddr},
	}, tspendFee, expiry)
	tspendHash := tspend.TxHash()

	// Generate a TAdd for some amount ensuring there's change and direct
	// the change to a P2PKH addr.
	taddAmount := int64(outs[0].Amount() / 2)
	taddChange := taddAmount
	const taddFee = 0
	tadd1 := g.CreateTreasuryTAddChange(&taddOut1, dcrutil.Amount(taddAmount),
		taddFee, addP2pkhAddr)
	taddHash1 := tadd1.TxHash()

	// Generate a second TAdd that pays the change to a unique P2SH script.
	tadd2 := g.CreateTreasuryTAddChange(&outs[0], dcrutil.Amount(taddAmount),
		taddFee, addP2shAddr)
	taddHash2 := tadd2.TxHash()

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the tspend.
	//
	// ... -> bpretvi# -> bv0 -> ... -> bv#
	// ---------------------------------------------------------------------
	for i := uint64(0); i < tvi*mul; i++ {
		name := fmt.Sprintf("bv%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			tspend))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Generate a block that includes the tspend and tadd.
	//
	// ... -> bv# -> btspend
	// ---------------------------------------------------------------------
	var tspendHeight, tspendIndex uint32
	g.NextBlock("btspend", nil, outs[1:], func(b *wire.MsgBlock) {
		tspendHeight = b.Header.Height
		tspendIndex = uint32(len(b.STransactions))
		b.AddSTransaction(tspend)
		b.AddSTransaction(tadd1)
		b.AddSTransaction(tadd2)
	})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()
	outs = g.OldestCoinbaseOuts()

	// Ensure the CFilter committed to the outputs of the TSpend and TAdds.
	tipHash := &g.chain.BestSnapshot().Hash
	bcf, _, err := g.chain.FilterByBlockHash(tipHash)
	if err != nil {
		t.Fatal(err)
	}
	bcfKey := blockcf2.Key(&(g.Tip().Header.MerkleRoot))
	wantMatches := [][]byte{
		tspend.TxOut[1].PkScript[1:], // P2PKH without OP_TGEN
		tspend.TxOut[2].PkScript[1:], // P2SH without OP_TGEN
		tadd1.TxOut[1].PkScript[1:],  // P2PKH without OP_SSTxChange
		tadd2.TxOut[1].PkScript[1:],  // P2SH without OP_SSTxChange
	}
	for i, wantMatch := range wantMatches {
		if !bcf.Match(bcfKey, wantMatch) {
			t.Fatalf("bcf did not match script %d %x", i, wantMatch)
		}
	}

	// Create a tx that spends from the TSpend outputs and the TAdd change
	// output.
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{ // TSpend P2PKH output
		PreviousOutPoint: wire.OutPoint{
			Hash:  tspendHash,
			Index: 1,
			Tree:  1,
		},
		Sequence:    wire.MaxTxInSequenceNum,
		ValueIn:     int64(tspendAmount),
		BlockHeight: tspendHeight,
		BlockIndex:  tspendIndex,
	})
	tx.AddTxIn(&wire.TxIn{ // TSPend P2SH output
		PreviousOutPoint: wire.OutPoint{
			Hash:  tspendHash,
			Index: 2,
			Tree:  1,
		},
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         int64(tspendAmount),
		BlockHeight:     tspendHeight,
		BlockIndex:      tspendIndex,
		SignatureScript: append([]byte{txscript.OP_DATA_2}, spendP2shScript...),
	})
	tx.AddTxIn(&wire.TxIn{ // TAdd P2PKH output
		PreviousOutPoint: wire.OutPoint{
			Hash:  taddHash1,
			Index: 1,
			Tree:  1,
		},
		Sequence:    wire.MaxTxInSequenceNum,
		ValueIn:     taddChange,
		BlockHeight: tspendHeight,
		BlockIndex:  tspendIndex + 1,
	})
	tx.AddTxIn(&wire.TxIn{ // TAdd P2SH output
		PreviousOutPoint: wire.OutPoint{
			Hash:  taddHash2,
			Index: 1,
			Tree:  1,
		},
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         taddChange,
		BlockHeight:     tspendHeight,
		BlockIndex:      tspendIndex + 2,
		SignatureScript: append([]byte{txscript.OP_DATA_3}, addP2shScript...),
	})
	tx.AddTxOut(&wire.TxOut{
		Version: 0,
		Value:   int64(tspendAmount*2) + taddChange,
	})

	// Generate the valid signature for the first input, which is a P2PKH.
	sig, err := sign.SignatureScript(tx, 0, tspend.TxOut[1].PkScript,
		txscript.SigHashAll, spendPrivKey.Serialize(), dcrec.STEcdsaSecp256k1,
		true)
	if err != nil {
		t.Fatalf("unable to generate sig: %v", err)
	}
	tx.TxIn[0].SignatureScript = sig

	// Generate the valid signature for the third input, which is a P2PKH.
	sig, err = sign.SignatureScript(tx, 2, tadd1.TxOut[1].PkScript,
		txscript.SigHashAll, addPrivKey.Serialize(), dcrec.STEcdsaSecp256k1,
		true)
	if err != nil {
		t.Fatalf("unable to generate sig: %v", err)
	}
	tx.TxIn[2].SignatureScript = sig

	// ---------------------------------------------------------------------
	// Generate up to the block where the tspend is spendable.
	//
	// ... -> btspend -> bpremat0 -> ... -> bpremat#
	// ---------------------------------------------------------------------

	for i := uint32(0); i < uint32(params.CoinbaseMaturity-1); i++ {
		name := fmt.Sprintf("bpremat%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Generate a block that includes the transaction spending from the
	// TSpend and TAdd.
	//
	// ... -> btpremat# -> btredeem
	// ---------------------------------------------------------------------
	g.NextBlock("btredeem", nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddTransaction(tx)
	})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()
}

func TestTSpendDupVote(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// Shorter versions of useful params for convenience.
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// ---------------------------------------------------------------------
	// Create two TSPENDs with invalid bits and duplicate votes.
	// ---------------------------------------------------------------------

	nextBlockHeight := g.Tip().Header.Height + 1
	expiry := standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	start, _, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}

	const tspendAmount = 10
	const tspendFee = 0
	tspend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{{
		Amount: tspendAmount - tspendFee}}, tspendFee, expiry)
	tspend2 := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{{
		Amount: tspendAmount - tspendFee}}, tspendFee, expiry)

	// ---------------------------------------------------------------------
	// Generate enough blocks to get to TVI.
	//
	//   ... -> bva19 -> bpretvi0 -> bpretvi1
	// ---------------------------------------------------------------------

	// Generate votes up to TVI. This is legal however they should NOT be
	// counted in the totals since they are outside of the voting window.
	outs := g.OldestCoinbaseOuts()
	for i := uint32(0); i < start-nextBlockHeight; i++ {
		name := fmt.Sprintf("bpretvi%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Duplicate votes on the same treasury spend tx hash is illegal and
	// therefore the tx is NOT recognized as a vote.
	//
	//   ... -> pretvi1
	//                 \-> bdv0
	// ---------------------------------------------------------------------

	startTip := g.TipName()
	g.NextBlock("bdv0", nil, outs[1:], chaingen.AddTreasurySpendYesVotes(tspend,
		tspend))
	g.RejectTipBlock(ErrBadTxInput)

	// ---------------------------------------------------------------------
	// Invalid treasury spend tx vote bits are illegal and therefore the tx
	// is NOT recognized as a vote.
	//
	//   ... -> pretvi1
	//                 \-> bdv1
	// ---------------------------------------------------------------------

	g.SetTip(startTip)
	g.NextBlock("bdv1", nil, outs[1:], func(b *wire.MsgBlock) {
		const invalidBits = 0x04
		firstStakeVote := b.STransactions[1]
		chaingen.AddTreasurySpendVote(firstStakeVote, tspend2, invalidBits)
	})
	g.RejectTipBlock(ErrBadTxInput)
}

func TestTSpendTooManyTSpend(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// Shorter versions of useful params for convenience.
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// ---------------------------------------------------------------------
	// Create two TSPEND with invalid bits and duplicate votes.
	// ---------------------------------------------------------------------

	nextBlockHeight := g.Tip().Header.Height + 1
	expiry := standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	start, _, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}

	const tspendAmount = 10
	const tspendFee = 0
	const maxTspends = 7
	tspends := make([]*wire.MsgTx, maxTspends+1)
	for i := 0; i < maxTspends+1; i++ {
		tspends[i] = g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
			{Amount: tspendAmount - tspendFee}}, tspendFee, expiry)
	}

	// ---------------------------------------------------------------------
	// Generate enough blocks to get to TVI.
	//
	//   ... -> bsv# -> bpretvi0 -> bpretvi#
	// ---------------------------------------------------------------------

	// Generate votes up to TVI. This is legal however they should NOT be
	// counted in the totals since they are outside of the voting window.
	outs := g.OldestCoinbaseOuts()
	for i := uint32(0); i < start-nextBlockHeight; i++ {
		name := fmt.Sprintf("bpretvi%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// 8 votes is illegal and therefore the tx is NOT recognized as a vote.
	//
	//   ... -> pretvi#
	//                 \-> bdv0
	// ---------------------------------------------------------------------

	g.NextBlock("bdv0", nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
		tspends...))
	g.RejectTipBlock(ErrBadTxInput)
}

func TestTSpendWindow(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// Shorter versions of useful params for convenience.
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// ---------------------------------------------------------------------
	// Create TSPEND in mempool
	// ---------------------------------------------------------------------

	nextBlockHeight := g.Tip().Header.Height + uint32(tvi*mul*4) + 1
	expiry := standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	start, _, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}

	const tspendAmount = 10
	const tspendFee = 0
	tspend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{Amount: tspendAmount - tspendFee}}, tspendFee, expiry)

	// ---------------------------------------------------------------------
	// Generate enough blocks to get to TVI.
	//
	//   ... -> bsv# -> bpretvi0 -> bpretvi#
	// ---------------------------------------------------------------------

	// Generate votes up to TVI. This is legal however they should NOT be
	// counted in the totals since they are outside of the voting window.
	outs := g.OldestCoinbaseOuts()
	for i := uint32(0); i < start-nextBlockHeight; i++ {
		name := fmt.Sprintf("bpretvi%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Generate a TVI worth of rewards and try to spend more.
	//
	//   ... -> bpretvi# -> b0 -> ... -> b#
	//                                      \-> bvidaltoosoon0
	// ---------------------------------------------------------------------

	for i := uint64(0); i < tvi*mul; i++ {
		name := fmt.Sprintf("b%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			tspend))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// Try accepting TSpend from the future.
	g.NextBlock("bvidaltoosoon0", nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
	})
	g.RejectTipBlock(ErrInvalidTSpendWindow)
}

// TestTSpendSignature verifies that both PI keys work and in addition that an
// invalid key is indeed rejected.
func TestTSpendSignature(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// Shorter versions of useful params for convenience.
	cbm := params.CoinbaseMaturity
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// Ensure the treasury balance is the expected value at this point.  It is
	// the number of treasurybases added minus the coinbase maturity all times
	// the amount of each treasurybase.
	tipHeight := g.Tip().Header.Height
	numTrsyBases := int64(tipHeight - 1)
	trsyBaseAmt := g.Tip().STransactions[0].TxIn[0].ValueIn
	expectedBal := (numTrsyBases - int64(cbm)) * trsyBaseAmt
	g.ExpectTreasuryBalance(expectedBal)

	// ---------------------------------------------------------------------
	// Create TSPEND in mempool with key 1 and 2
	// ---------------------------------------------------------------------

	nextBlockHeight := tipHeight + 1
	expiry := standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	start, _, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}

	tspendAmount1 := dcrutil.Amount(trsyBaseAmt / 3)
	tspendFee := dcrutil.Amount(0)
	tspend1 := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{Amount: tspendAmount1 - tspendFee}}, tspendFee, expiry)

	// tspend 2.
	tspendAmount2 := dcrutil.Amount(trsyBaseAmt / 5)
	tspend2 := g.CreateTreasuryTSpend(privKey2, []chaingen.AddressAmountTuple{
		{Amount: tspendAmount2 - tspendFee}}, tspendFee, expiry)

	// ---------------------------------------------------------------------
	// Generate enough blocks to get to TVI.
	//
	//   ... -> bsv# -> bpretvi0 -> bpretvi#
	// ---------------------------------------------------------------------

	outs := g.OldestCoinbaseOuts()
	for i := uint32(0); i < start-nextBlockHeight; i++ {
		name := fmt.Sprintf("bpretvi%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		numTrsyBases++
	}

	// ---------------------------------------------------------------------
	// Generate TVI*MUL of yes votes for both TSpends,
	//
	//   ... -> bpretvi# -> btvi0 -> ... -> btvi#
	// ---------------------------------------------------------------------

	for i := uint64(0); i < tvi*mul; i++ {
		name := fmt.Sprintf("btvi%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			tspend1, tspend2))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		numTrsyBases++
	}

	// Assert that we are on a TVI.
	if standalone.IsTreasuryVoteInterval(uint64(g.Tip().Header.Height), tvi) {
		t.Fatalf("expected !TVI %v", g.Tip().Header.Height)
	}

	// Assert treasury balance
	g.ExpectTreasuryBalance((numTrsyBases - int64(cbm)) * trsyBaseAmt)

	// ---------------------------------------------------------------------
	// Add tspend1 twice
	//
	//   ... -> btvi#
	//               \-> double0
	// ---------------------------------------------------------------------

	startTip := g.TipName()
	g.NextBlock("double0", nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend1)
		b.AddSTransaction(tspend2)
		b.AddSTransaction(tspend1)
	})
	g.RejectTipBlock(ErrDuplicateTx)

	// ---------------------------------------------------------------------
	// Add tspend1 and tspend2
	//
	//   ... -> btvi# -> doubleok0
	// ---------------------------------------------------------------------

	g.SetTip(startTip)
	g.NextBlock("doubleok0", nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend1)
		b.AddSTransaction(tspend2)
	})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()
	outs = g.OldestCoinbaseOuts()
	numTrsyBases++

	// Assert treasury balance
	g.ExpectTreasuryBalance((numTrsyBases - int64(cbm)) * trsyBaseAmt)

	// ---------------------------------------------------------------------
	// Add Coinbase maturity blocks and assert treasury balance.
	//
	//   ... -> btvi# -> doubleok0 -> cbm0 -> cbm#
	// ---------------------------------------------------------------------

	for i := uint32(0); i < uint32(params.CoinbaseMaturity); i++ {
		name := fmt.Sprintf("cbm%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		numTrsyBases++
	}

	// Assert treasury balance
	expectedBalance := (numTrsyBases - int64(cbm)) * trsyBaseAmt
	expectedBalance -= int64(tspendAmount1)
	expectedBalance -= int64(tspendAmount2)
	g.ExpectTreasuryBalance(expectedBalance)
}

// TestTSpendSignatureInvalid verifies that a tspend is disallowed with an
// invalid key.
func TestTSpendSignatureInvalid(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// Shorter versions of useful params for convenience.
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// ---------------------------------------------------------------------
	// Create TSPEND in mempool.
	// ---------------------------------------------------------------------

	nextBlockHeight := g.Tip().Header.Height + 1
	expiry := standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	start, _, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt key for test.
	wrongKey := make([]byte, len(privKey))
	copy(wrongKey, privKey)
	wrongKey[len(wrongKey)-1] = ^wrongKey[len(wrongKey)-1]

	const tspendAmount = 10
	const tspendFee = 0
	tspend := g.CreateTreasuryTSpend(wrongKey, []chaingen.AddressAmountTuple{
		{Amount: tspendAmount - tspendFee}}, tspendFee, expiry)

	// ---------------------------------------------------------------------
	// Generate enough blocks to get to TVI.
	//
	//   ... -> bsv# -> bpretvi0 -> bpretvi#
	// ---------------------------------------------------------------------

	outs := g.OldestCoinbaseOuts()
	for i := uint32(0); i < start-nextBlockHeight; i++ {
		name := fmt.Sprintf("bpretvi%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Generate TVI*MUL of yes votes for TSpend,
	//
	//   ... -> bpretvi# -> btvi0 -> ... -> btvi#
	// ---------------------------------------------------------------------

	for i := uint64(0); i < tvi*mul; i++ {
		name := fmt.Sprintf("btvi%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			tspend))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// Assert that we are on a TVI.
	if standalone.IsTreasuryVoteInterval(uint64(g.Tip().Header.Height), tvi) {
		t.Fatalf("expected !TVI %v", g.Tip().Header.Height)
	}

	// ---------------------------------------------------------------------
	// Add tspend with invalid signature
	//
	//   ... -> btvi#
	//                \-> invalidsig
	// ---------------------------------------------------------------------

	g.NextBlock("invalidsig", nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
	})
	g.RejectTipBlock(ErrUnknownPiKey)
}

func TestTSpendExists(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// Shorter versions of useful params for convenience.
	cbm := params.CoinbaseMaturity
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// splitSecondRegularTxOutputs is a munge function which modifies the
	// provided block by replacing its second regular transaction with one
	// that creates several utxos.
	const splitTxNumOutputs = 6
	splitSecondRegularTxOutputs := func(b *wire.MsgBlock) {
		// Remove the current outputs of the second transaction while
		// saving the relevant public key script, input amount, and fee
		// for below.
		tx := b.Transactions[1]
		inputAmount := tx.TxIn[0].ValueIn
		pkScript := tx.TxOut[0].PkScript
		fee := inputAmount - tx.TxOut[0].Value
		tx.TxOut = tx.TxOut[:0]

		// Final outputs are the input amount minus the fee split into
		// more than one output.  These are intended to provide
		// additional utxos for testing.
		outputAmount := inputAmount - fee
		splitAmount := outputAmount / splitTxNumOutputs
		for i := 0; i < splitTxNumOutputs; i++ {
			if i == splitTxNumOutputs-1 {
				splitAmount = outputAmount -
					splitAmount*(splitTxNumOutputs-1)
			}
			tx.AddTxOut(wire.NewTxOut(splitAmount, pkScript))
		}
	}

	// Generate spendable outputs to do fork tests with.
	var txOuts [][]chaingen.SpendableOut
	genBlocks := cbm * 8
	for i := uint16(0); i < genBlocks; i++ {
		outs := g.OldestCoinbaseOuts()
		name := fmt.Sprintf("bouts%v", i)
		g.NextBlock(name, &outs[0], outs[1:], splitSecondRegularTxOutputs)
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()

		souts := make([]chaingen.SpendableOut, 0, splitTxNumOutputs)
		for j := 0; j < splitTxNumOutputs; j++ {
			spendableOut := chaingen.MakeSpendableOut(g.Tip(), 1,
				uint32(j))
			souts = append(souts, spendableOut)
		}
		txOuts = append(txOuts, souts)
	}

	// ---------------------------------------------------------------------
	// Create TSPEND in mempool
	// ---------------------------------------------------------------------

	nextBlockHeight := g.Tip().Header.Height + 1
	expiry := standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	start, _, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}

	const tspendAmount = 10
	const tspendFee = 0
	tspend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{{
		Amount: tspendAmount - tspendFee}}, tspendFee, expiry)

	// ---------------------------------------------------------------------
	// Generate enough blocks to get to TVI.
	//
	//   ... -> bouts -> ... -> bpretvi#
	// ---------------------------------------------------------------------

	outs := g.OldestCoinbaseOuts()
	for i := uint32(0); i < start-nextBlockHeight; i++ {
		name := fmt.Sprintf("bpretvi%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Generate two TVI with votes.
	//
	//   ... -> bpretvi# -> b0 -> ... -> b#
	// ---------------------------------------------------------------------

	for i := uint64(0); i < tvi*2; i++ {
		name := fmt.Sprintf("b%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			tspend))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Generate a TVI and mine same TSpend, which has to be rejected.
	//
	//   ... -> b# -> be0 -> ... -> be#
	//                                 \-> bexists0
	// ---------------------------------------------------------------------

	startTip := g.TipName()
	for i := uint64(0); i < tvi; i++ {
		name := fmt.Sprintf("be%v", i)
		if i == 0 {
			g.NextBlock(name, nil, txOuts[i][1:], func(b *wire.MsgBlock) {
				b.AddSTransaction(tspend)
			})
		} else {
			g.NextBlock(name, nil, txOuts[i][1:])
		}
		g.AcceptTipBlock()
	}
	oldTip := g.TipName()

	// Mine tspend again.
	g.NextBlock("bexists0", nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
	})
	g.RejectTipBlock(ErrTSpendExists)

	// ---------------------------------------------------------------------
	// Generate a TVI and mine same TSpend, should not exist since it is a
	// fork.
	//
	//   ... -> b# -> be0  -> ... -> be#
	//            \-> bep0 -> ... -> bep# -> bexists1
	// ---------------------------------------------------------------------

	g.SetTip(startTip)
	var nextFork uint64
	for i := uint64(0); i < tvi; i++ {
		name := fmt.Sprintf("bep%v", i)
		g.NextBlock(name, nil, txOuts[i][1:])
		g.AcceptedToSideChainWithExpectedTip(oldTip)
		nextFork = i + 1
	}

	// Mine tspend again.
	g.NextBlock("bexists1", nil, txOuts[nextFork][1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
	})
	g.AcceptTipBlock()

	// ---------------------------------------------------------------------
	// Generate a TVI and mine same TSpend, should not exist since it is a
	// fork.
	//
	//   ... -> b# -> be0   -> ... -> be#
	//
	//            \-> bep0  -> ... -> bexists1 -> bepa0  -> ... -> bepa#
	//            \-> bepp0 -> ... -> bexists2 -> beppa0 -> ........... -> beppa#
	// ---------------------------------------------------------------------

	// Generate one more block to extend current best chain.
	name := fmt.Sprintf("bepa%v", nextFork)
	g.NextBlock(name, nil, txOuts[nextFork+1][1:])
	g.AcceptTipBlock()
	oldTip = g.TipName()
	oldHeight := g.Tip().Header.Height

	// Create bepp fork
	g.SetTip(startTip)
	txIdx := uint64(0)
	for i := uint64(0); i < tvi; i++ {
		name := fmt.Sprintf("bepp%v", i)
		g.NextBlock(name, nil, txOuts[i][1:])
		g.AcceptedToSideChainWithExpectedTip(oldTip)
		txIdx = i + 1
	}

	// Mine tspend yet again.
	g.NextBlock("bexists2", nil, txOuts[txIdx][1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
	})
	g.AcceptedToSideChainWithExpectedTip(oldTip)
	txIdx++

	// Force reorg
	for i := uint64(0); i < tvi; i++ {
		name := fmt.Sprintf("beppa%v", i)
		b := g.NextBlock(name, nil, txOuts[txIdx][1:])
		if b.Header.Height <= oldHeight {
			g.AcceptedToSideChainWithExpectedTip(oldTip)
		} else {
			g.AcceptTipBlock()
		}
		txIdx++
	}
}

func TestTreasuryBalance(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// Shorter versions of useful params for convenience.
	cbm := params.CoinbaseMaturity
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// Ensure the treasury balance is the expected value at this point.  It is
	// the number of treasurybases added minus the coinbase maturity all times
	// the amount of each treasurybase.
	tipHeight := g.Tip().Header.Height
	numTrsyBases := int64(tipHeight - 1)
	trsyBaseAmt := g.Tip().STransactions[0].TxIn[0].ValueIn
	startingBal := (numTrsyBases - int64(cbm)) * trsyBaseAmt
	g.ExpectTreasuryBalance(startingBal)

	// ---------------------------------------------------------------------
	// Create 10 blocks that have a tadd without change.
	//
	//   ... -> bsv# -> b0 -> ... -> b9
	// ---------------------------------------------------------------------

	const blockCount = 10
	expectedTotal := startingBal + blockCount*trsyBaseAmt
	var skippedTotal int64
	for i := 0; i < blockCount; i++ {
		amount := int64(i + 1)
		if i < blockCount-int(params.CoinbaseMaturity) {
			expectedTotal += amount
		} else {
			skippedTotal += amount
		}
		outs := g.OldestCoinbaseOuts()
		name := fmt.Sprintf("b%v", i)
		_ = g.NextBlock(name, nil, outs[1:], func(b *wire.MsgBlock) {
			tx := g.CreateTreasuryTAdd(&outs[0], dcrutil.Amount(amount),
				dcrutil.Amount(0))
			b.AddSTransaction(tx)
		})
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		numTrsyBases++
	}
	g.ExpectTreasuryBalance(expectedTotal)
	g.ExpectTreasuryBalanceChange(1, treasuryValueTAdd, int64(blockCount))

	// ---------------------------------------------------------------------
	// Create 10 blocks that has a tadd with change. Pretend that the TSpend
	// transaction is in the mempool and vote on it.
	//
	//   ... -> b10 -> ... -> b19
	// ---------------------------------------------------------------------

	// This looks a little funky but it was coppied from the prior TSPEND
	// test that created this many tspends. Since that is no longer
	// possible use the for loop to get to the same totals.
	var tspendAmount, tspendFee dcrutil.Amount
	nextTipHeight := int64(g.Tip().Header.Height + 1)
	expiry := standalone.CalcTSpendExpiry(nextTipHeight, tvi, mul)
	for i := 0; i < blockCount*2; i++ {
		tspendAmount += dcrutil.Amount(i) + 1
		tspendFee++
	}
	tspend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{Amount: tspendAmount - tspendFee}}, tspendFee, expiry)

	expectedTotal += skippedTotal
	expectedTotal += blockCount * trsyBaseAmt
	for i := blockCount; i < blockCount*2; i++ {
		amount := int64(i + 1)
		if i < (blockCount*2)-int(params.CoinbaseMaturity) {
			expectedTotal += amount
		}
		outs := g.OldestCoinbaseOuts()
		name := fmt.Sprintf("b%v", i)
		_ = g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			tspend), func(b *wire.MsgBlock) {
			tx := g.CreateTreasuryTAdd(&outs[0], dcrutil.Amount(amount),
				dcrutil.Amount(1))
			b.AddSTransaction(tx)
		})
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		numTrsyBases++
	}

	g.ExpectTreasuryBalance(expectedTotal)
	g.ExpectTreasuryBalanceChange(1, treasuryValueTAdd, int64(blockCount*2))

	// ---------------------------------------------------------------------
	// Create 20 blocks that has a tspend and params.CoinbaseMaturity more
	// to bring treasury balance back to the starting amount.
	//
	//   ... -> b19 -> ... -> b41
	// ---------------------------------------------------------------------

	var doneTSpend bool
	for i := 0; i < blockCount*2+int(params.CoinbaseMaturity); i++ {
		outs := g.OldestCoinbaseOuts()
		name := fmt.Sprintf("b%v", i+blockCount*2)
		if (g.Tip().Header.Height+1)%4 == 0 && !doneTSpend {
			// Insert TSPEND
			g.NextBlock(name, nil, outs[1:], func(b *wire.MsgBlock) {
				b.AddSTransaction(tspend)
			})
			doneTSpend = true
		} else {
			g.NextBlock(name, nil, outs[1:])
		}
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		numTrsyBases++
	}

	expected := (numTrsyBases - int64(cbm)) * trsyBaseAmt
	g.ExpectTreasuryBalance(expected)
}

// newTxOut returns a new transaction output with the given parameters.
func newTxOut(amount int64, pkScriptVer uint16, pkScript []byte) *wire.TxOut {
	return &wire.TxOut{
		Value:    amount,
		Version:  pkScriptVer,
		PkScript: pkScript,
	}
}

func createTAdd(spend chaingen.SpendableOut, changeDivisor dcrutil.Amount, params *chaincfg.Params) func(b *wire.MsgBlock) {
	redeemScript := []byte{txscript.OP_TRUE}
	p2shOpTrueAddr, err := stdaddr.NewAddressScriptHashV0(redeemScript, params)
	if err != nil {
		panic(err)
	}

	return func(b *wire.MsgBlock) {
		amount := spend.Amount()
		tx := wire.NewMsgTx()
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: spend.PrevOut(),
			Sequence:         wire.MaxTxInSequenceNum,
			ValueIn:          int64(amount),
			BlockHeight:      spend.BlockHeight(),
			BlockIndex:       spend.BlockIndex(),
			SignatureScript:  []byte{txscript.OP_DATA_1, txscript.OP_TRUE},
		})

		// Add negative change.
		trsyAddScript := []byte{txscript.OP_TADD}
		if changeDivisor != 0 {
			changeScriptVer, changeScript := p2shOpTrueAddr.StakeChangeScript()

			change := spend.Amount() / changeDivisor
			amount := spend.Amount() - change
			tx.AddTxOut(wire.NewTxOut(int64(amount), trsyAddScript))
			tx.AddTxOut(newTxOut(int64(change), changeScriptVer, changeScript))
		} else {
			tx.AddTxOut(wire.NewTxOut(int64(amount), trsyAddScript))
		}
		tx.Version = wire.TxVersionTreasury
		b.AddSTransaction(tx)
	}
}

func TestTAddCorners(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// Shorter versions of useful params for convenience.
	cbm := params.CoinbaseMaturity

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// splitSecondRegularTxOutputs is a munge function which modifies the
	// provided block by replacing its second regular transaction with one
	// that creates several utxos.
	const splitTxNumOutputs = 6
	splitSecondRegularTxOutputs := func(b *wire.MsgBlock) {
		// Remove the current outputs of the second transaction while
		// saving the relevant public key script, input amount, and fee
		// for below.
		tx := b.Transactions[1]
		inputAmount := tx.TxIn[0].ValueIn
		pkScript := tx.TxOut[0].PkScript
		fee := inputAmount - tx.TxOut[0].Value
		tx.TxOut = tx.TxOut[:0]

		// Final outputs are the input amount minus the fee split into
		// more than one output.  These are intended to provide
		// additional utxos for testing.
		outputAmount := inputAmount - fee
		splitAmount := outputAmount / splitTxNumOutputs
		for i := 0; i < splitTxNumOutputs; i++ {
			if i == splitTxNumOutputs-1 {
				splitAmount = outputAmount -
					splitAmount*(splitTxNumOutputs-1)
			}
			tx.AddTxOut(wire.NewTxOut(splitAmount, pkScript))
		}
	}

	// Generate spendable outputs to do fork tests with.
	var txOuts [][]chaingen.SpendableOut
	genBlocks := cbm * 8
	for i := uint16(0); i < genBlocks; i++ {
		outs := g.OldestCoinbaseOuts()
		name := fmt.Sprintf("bouts%v", i)
		g.NextBlock(name, &outs[0], outs[1:], splitSecondRegularTxOutputs)
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()

		souts := make([]chaingen.SpendableOut, 0, splitTxNumOutputs)
		for j := 0; j < splitTxNumOutputs; j++ {
			spendableOut := chaingen.MakeSpendableOut(g.Tip(), 1,
				uint32(j))
			souts = append(souts, spendableOut)
		}
		txOuts = append(txOuts, souts)
	}

	// ---------------------------------------------------------------------
	// Create TAdd with negative change.
	//
	//   ... -> bn0
	// ---------------------------------------------------------------------
	mungeValueChange := func(b *wire.MsgBlock) {
		for k := range b.STransactions {
			if !stake.IsTAdd(b.STransactions[k]) {
				continue
			}
			b.STransactions[k].TxOut[1].Value *= -1
			break
		}
	}

	startTip := g.TipName()
	g.NextBlock("bn0", nil, txOuts[0][1:], createTAdd(txOuts[0][0], 10, params),
		mungeValueChange)
	g.RejectTipBlock(ErrBadTxOutValue)

	// ---------------------------------------------------------------------
	// Create TAdd with negative in amount.
	//
	//   ... -> bn1
	// ---------------------------------------------------------------------

	mungeValueIn := func(b *wire.MsgBlock) {
		for k := range b.STransactions {
			if !stake.IsTAdd(b.STransactions[k]) {
				continue
			}
			b.STransactions[k].TxIn[0].ValueIn =
				-b.STransactions[k].TxIn[0].ValueIn
			break
		}
	}

	g.SetTip(startTip)
	g.NextBlock("bn1", nil, txOuts[0][1:], createTAdd(txOuts[0][0], 10, params),
		mungeValueIn)
	g.RejectTipBlock(ErrFraudAmountIn)

	// ---------------------------------------------------------------------
	// Create TAdd with negative out amount.
	//
	//   ... -> bn2
	// ---------------------------------------------------------------------

	mungeValueOut := func(b *wire.MsgBlock) {
		for k := range b.STransactions {
			if !stake.IsTAdd(b.STransactions[k]) {
				continue
			}
			b.STransactions[k].TxOut[0].Value =
				-b.STransactions[k].TxOut[0].Value
			break
		}
	}

	g.SetTip(startTip)
	g.NextBlock("bn2", nil, txOuts[0][1:], createTAdd(txOuts[0][0], 10, params),
		mungeValueOut)
	g.RejectTipBlock(ErrBadTxOutValue)

	// ---------------------------------------------------------------------
	// Create TAdd with 0 change.
	//
	//   ... -> bn3
	// ---------------------------------------------------------------------

	mungeChangeValue := func(b *wire.MsgBlock) {
		for k := range b.STransactions {
			if !stake.IsTAdd(b.STransactions[k]) {
				continue
			}
			b.STransactions[k].TxOut[1].Value = 0
			break
		}
	}

	g.SetTip(startTip)
	g.NextBlock("bn3", nil, txOuts[0][1:], createTAdd(txOuts[0][0], 10, params),
		mungeChangeValue)
	g.RejectTipBlock(ErrInvalidTAddChange)
}

func TestTreasuryBaseCorners(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// removeAllButTreasurybase is a munge function which modifies the provided
	// block by removing all stake transactions other than the treasurybase.
	removeAllButTreasurybase := func(b *wire.MsgBlock) {
		b.Header.FreshStake = 0
		b.STransactions = b.STransactions[:1]
	}

	// removeStakeTxns is a munge function which modifies the provided block by
	// removing all stake transactions other than votes.
	removeStakeTxns := func(b *wire.MsgBlock) {
		b.Header.FreshStake = 0
		b.Header.Revocations = 0
		b.STransactions = b.STransactions[1:6]
	}

	// dupTreasurybase is a munge function which modifies the provided block by
	// adding a duplicate treasurybase at the end of the stake tree.
	dupTreasurybase := func(b *wire.MsgBlock) {
		txCopy := b.STransactions[0].Copy()
		txCopy.TxOut[1].PkScript[len(txCopy.TxOut[1].PkScript)-1] ^= 0x55
		b.AddSTransaction(txCopy)
	}

	// flipTreasurybase is a munge function which modifies the provided block by
	// swapping the treasurybase with the second transaction in the stake tree.
	flipTreasurybase := func(b *wire.MsgBlock) {
		stakeTxns := b.STransactions
		stakeTxns[0], stakeTxns[1] = stakeTxns[1], stakeTxns[0]
	}

	// removeTreasurybase is a munge function which modifies the provided block
	// by removing the treasurybase from the stake tree.
	removeTreasurybase := func(b *wire.MsgBlock) {
		b.STransactions = b.STransactions[1:]
	}

	// changeHeightTreasurybase is a munge function which modifies the provided
	// block by adding one to the height encoded in the treasurybase.
	changeHeightTreasurybase := func(b *wire.MsgBlock) {
		b.STransactions[0].TxOut[1].PkScript[2]++
	}

	// corruptLengthTreasurybase is a munge function which modifies the provided
	// block by mutating the treasurybase script to corrupt it.
	corruptLengthTreasurybase := func(b *wire.MsgBlock) {
		s := b.STransactions[0].TxOut[1].PkScript
		b.STransactions[0].TxOut[1].PkScript = s[:len(s)-1]
	}

	// corruptTreasurybaseValueIn is a munge function which modifies the
	// provided block by mutating the treasurybase in value.
	corruptTreasurybaseValueIn := func(b *wire.MsgBlock) {
		b.STransactions[0].TxIn[0].ValueIn--
	}

	// corruptTreasurybaseValueOut is a munge function which modifies the
	// provided block by mutating the treasurybase out value.
	corruptTreasurybaseValueOut := func(b *wire.MsgBlock) {
		b.STransactions[0].TxOut[0].Value--
	}

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// ---------------------------------------------------------------------
	// Append a treasurybase to STransactions.
	// ---------------------------------------------------------------------

	outs := g.OldestCoinbaseOuts()
	startTip := g.TipName()
	g.NextBlock("twotb0", nil, outs[1:], dupTreasurybase)
	g.RejectTipBlock(ErrMultipleTreasurybases)

	// ---------------------------------------------------------------------
	// Flip STransactions 0 and 1.
	// ---------------------------------------------------------------------

	g.SetTip(startTip)
	g.NextBlock("notbon0", nil, outs[1:], flipTreasurybase)
	g.RejectTipBlock(ErrFirstTxNotTreasurybase)

	// ---------------------------------------------------------------------
	// No treasurybase.
	// ---------------------------------------------------------------------

	g.SetTip(startTip)
	g.NextBlock("notb0", nil, outs[1:], removeTreasurybase)
	g.RejectTipBlock(ErrFirstTxNotTreasurybase)

	// ---------------------------------------------------------------------
	// No treasurybase and no tickets.
	// ---------------------------------------------------------------------

	g.SetTip(startTip)
	g.NextBlock("nothing0", nil, outs[1:], removeStakeTxns)
	g.RejectTipBlock(ErrFirstTxNotTreasurybase)

	// ---------------------------------------------------------------------
	// Treasury base invalid height
	// ---------------------------------------------------------------------
	g.SetTip(startTip)
	g.NextBlock("height0", nil, outs[1:], changeHeightTreasurybase)
	g.RejectTipBlock(ErrTreasurybaseHeight)

	// ---------------------------------------------------------------------
	// Treasury base invalid OP_RETURN script length.
	// ---------------------------------------------------------------------
	g.SetTip(startTip)
	g.NextBlock("length0", nil, outs[1:], corruptLengthTreasurybase)
	g.RejectTipBlock(ErrFirstTxNotTreasurybase)

	// ---------------------------------------------------------------------
	// Only treasury base.
	// ---------------------------------------------------------------------
	g.SetTip(startTip)
	g.NextBlock("novotes0", nil, outs[1:], removeAllButTreasurybase)
	g.RejectTipBlock(ErrVotesMismatch)

	// ---------------------------------------------------------------------
	// Treasury base invalid TxIn[0].Value
	// ---------------------------------------------------------------------
	g.SetTip(startTip)
	g.NextBlock("invalidin0", nil, outs[1:], corruptTreasurybaseValueIn)
	g.RejectTipBlock(ErrBadTreasurybaseAmountIn)

	// ---------------------------------------------------------------------
	// Treasury base invalid TxOut[0].Value
	// ---------------------------------------------------------------------
	g.SetTip(startTip)
	g.NextBlock("invalidout0", nil, outs[1:], corruptTreasurybaseValueOut)
	g.RejectTipBlock(ErrTreasurybaseOutValue)

	// Note we can't hit the following errors in consensus:
	// * ErrFirstTxNotTreasurybase (missing OP_RETURN)
	// * ErrFirstTxNotTreasurybase (version)
	// * ErrTreasurybaseTxNotOpReturn
	// * ErrInvalidTreasurybaseTxOutputs
	// * ErrInvalidTreasurybaseVersion
	// * ErrInvalidTreasurybaseScript
}

func TestTSpendCorners(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// Shorter versions of useful params for convenience.
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// ---------------------------------------------------------------------
	// Create TSpend in mempool
	// ---------------------------------------------------------------------
	nextBlockHeight := g.Tip().Header.Height + 1
	const tspendAmount = 1000
	const tspendFee = 100
	expiry := standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	start, _, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}
	tspend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{Amount: tspendAmount - tspendFee}}, tspendFee, expiry)

	// ---------------------------------------------------------------------
	// Get to TVI
	//   ... -> pretvi1
	// ---------------------------------------------------------------------

	outs := g.OldestCoinbaseOuts()
	for i := uint32(0); i < start-nextBlockHeight; i++ {
		name := fmt.Sprintf("bpretvi%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Vote on TSpend
	//   pretvi1 -> btvi0 -> ... -> btvi7
	// ---------------------------------------------------------------------

	for i := uint64(0); i < tvi*mul; i++ {
		name := fmt.Sprintf("btvi%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			tspend))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Mine TSpend with improperly encoded ValueIn.
	// -> btvi7
	// \-> btspend0
	// ---------------------------------------------------------------------

	tspend.TxIn[0].ValueIn++
	g.NextBlock("btspend0", nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
	})
	g.RejectTipBlock(ErrInvalidTSpendValueIn)
}

func TestTSpendFirstTVICorner(t *testing.T) {
	t.Parallel()

	// Mark the treasury agenda as always active.
	const voteID = chaincfg.VoteIDTreasury
	params := chaincfg.RegNetParams()
	forceDeploymentResult(t, params, voteID, "yes")

	// Change params to hit corner case.
	params.StakeValidationHeight = 144
	params.TicketPoolSize = 144
	params.TreasuryVoteInterval = 11
	params.TreasuryVoteIntervalMultiplier = 1

	// Shorter versions of useful params for convenience.
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier
	svh := params.StakeValidationHeight
	t.Logf("svh: %v tvi %v mul %v", svh, tvi, mul)

	// Create a test harness initialized with the genesis block as the tip.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// With an SVH of 144 and a TVI of 11 and a MUL of 1 create a tspend
	// that expires on block 156 (voting interval [143,154]). Start voting
	// on block 144 (SVH) up to 153 (inclusive) which is 10*5 votes of the
	// total of 55 possible.  That easily is voted in since 60% is 33 yes
	// votes.
	expiry := standalone.CalcTSpendExpiry(140, tvi, mul)
	start, end, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("tspend expiry %v start %v end %v", expiry, start, end)
	tspend := newFakeCreateTSpend(privKey, []dcrutil.Amount{100000000}, 1,
		expiry)
	_, _, err = stake.CheckTSpend(tspend)
	if err != nil {
		t.Fatal(err)
	}

	// ---------------------------------------------------------------------
	// Block One.
	// ---------------------------------------------------------------------

	// Add the required first block.
	//
	//   genesis -> bfb
	g.CreateBlockOne("bfb", 0)
	g.AssertTipHeight(1)
	g.AcceptTipBlock()

	// Assert treasury agenda is indeed active.
	tipHash := &g.chain.BestSnapshot().Hash
	gotActive, err := g.chain.IsTreasuryAgendaActive(tipHash)
	if err != nil {
		t.Fatalf("IsTreasuryAgendaActive: %v", err)
	}
	if !gotActive {
		t.Fatalf("IsTreasuryAgendaActive: expected enabled treasury")
	}

	// ---------------------------------------------------------------------
	// Generate enough blocks to have mature coinbase outputs to work with.
	// Note that going forward all coinbase is munged to also output
	// treasurybase.
	//
	//   genesis -> bfb -> bm0 -> bm1 -> ... -> bm#
	// ---------------------------------------------------------------------

	ticketsPerBlock := params.TicketsPerBlock
	coinbaseMaturity := params.CoinbaseMaturity
	stakeEnabledHeight := params.StakeEnabledHeight
	stakeValidationHeight := params.StakeValidationHeight
	for i := uint16(0); i < coinbaseMaturity; i++ {
		blockName := fmt.Sprintf("bm%d", i)
		g.NextBlock(blockName, nil, nil)
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
	}
	g.AssertTipHeight(uint32(coinbaseMaturity) + 1)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake enabled height while
	// creating ticket purchases that spend from the coinbases matured
	// above.  This will also populate the pool of immature tickets.
	//
	//   ... -> bm# ... -> bse0 -> bse1 -> ... -> bse#
	// ---------------------------------------------------------------------

	var ticketsPurchased int
	for i := int64(0); int64(g.Tip().Header.Height) < stakeEnabledHeight; i++ {
		outs := g.OldestCoinbaseOuts()
		ticketOuts := outs[1:]
		ticketsPurchased += len(ticketOuts)
		blockName := fmt.Sprintf("bse%d", i)
		g.NextBlock(blockName, nil, ticketOuts)
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
	}
	g.AssertTipHeight(uint32(stakeEnabledHeight))

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake validation height while
	// continuing to purchase tickets using the coinbases matured above and
	// allowing the immature tickets to mature and thus become live.
	//
	//   ... -> bse# -> bsv0 -> bsv1 -> ... -> bsv#
	// ---------------------------------------------------------------------

	targetPoolSize := g.Params().TicketPoolSize * ticketsPerBlock
	for i := int64(0); int64(g.Tip().Header.Height) < stakeValidationHeight-1; i++ {
		// Only purchase tickets until the target ticket pool size is
		// reached.
		outs := g.OldestCoinbaseOuts()
		ticketOuts := outs[1:]
		if ticketsPurchased+len(ticketOuts) > int(targetPoolSize) {
			ticketsNeeded := int(targetPoolSize) - ticketsPurchased
			if ticketsNeeded > 0 {
				ticketOuts = ticketOuts[1 : ticketsNeeded+1]
			} else {
				ticketOuts = nil
			}
		}
		ticketsPurchased += len(ticketOuts)

		blockName := fmt.Sprintf("bsv%d", i)
		g.NextBlock(blockName, nil, ticketOuts)
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
	}

	g.AssertTipHeight(uint32(stakeValidationHeight - 1))

	// ---------------------------------------------------------------------
	// Test adding a tspend on the first possible height despite not being
	// on a TVI.
	//
	//   ... -> bse# -> bsv0 -> bsv1 -> ... -> bsv#
	//                                      \-> boink0
	// ---------------------------------------------------------------------

	startTip := g.TipName()
	g.NextBlock("boink0", nil, nil, func(b *wire.MsgBlock) {
		// Add TSpend
		b.AddSTransaction(tspend)
	})
	g.RejectTipBlock(ErrNotTVI)

	// ---------------------------------------------------------------------
	// Start TSpend 'yes' voting here.
	//
	//   ... -> bse# -> bsv0 -> bsv1 -> ... -> bsv# -> bsvv#
	//                                      \-> boink0
	// ---------------------------------------------------------------------

	// Full voting window -1 for block that contained no votes and another
	// -1 because the TSpend can only be mined on a TVI.
	numVotingBlocks := int64(tvi*mul - 2)

	g.SetTip(startTip)
	targetPoolSize = g.Params().TicketPoolSize * ticketsPerBlock
	for i := int64(0); int64(g.Tip().Header.Height) <
		stakeValidationHeight+numVotingBlocks; i++ {

		// Only purchase tickets until the target ticket pool size is
		// reached.
		outs := g.OldestCoinbaseOuts()
		ticketOuts := outs[1:]
		if ticketsPurchased+len(ticketOuts) > int(targetPoolSize) {
			ticketsNeeded := int(targetPoolSize) - ticketsPurchased
			if ticketsNeeded > 0 {
				ticketOuts = ticketOuts[1 : ticketsNeeded+1]
			} else {
				ticketOuts = nil
			}
		}
		ticketsPurchased += len(ticketOuts)

		blockName := fmt.Sprintf("bsvv%d", i)
		g.NextBlock(blockName, nil, ticketOuts,
			chaingen.AddTreasurySpendYesVotes(tspend))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + numVotingBlocks))

	// ---------------------------------------------------------------------
	// Mine TSpend on TVI.
	//
	//   ... -> bse# -> bsv0 -> bsv1 -> ... -> bsv# -> bsvv# ->
	//                                      \-> boink0       \-> boink1
	// ---------------------------------------------------------------------

	g.NextBlock("boink1", nil, nil, func(b *wire.MsgBlock) {
		// Add TSpend
		b.AddSTransaction(tspend)
	})
	g.RejectTipBlock(ErrInvalidTVoteWindow)
}

func TestTreasuryInRegularTree(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// addTreasuryBaseRegular is a munge function which modifies the provided
	// block to add a treasurybase to the regular transaction tree.
	addTreasuryBaseRegular := func(b *wire.MsgBlock) {
		txCopy := b.STransactions[0].Copy()
		txCopy.TxOut[1].PkScript[len(txCopy.TxOut[1].PkScript)-1] ^= 0x55
		b.AddTransaction(txCopy)
	}

	// moveTAddRegular is a munge function which modifies the provided block by
	// moving a treasury add transaction from the stake transaction tree to the
	// regular transaction tree.
	moveTAddRegular := func(b *wire.MsgBlock) {
		// Move TAdd to regular tree
		tAddIdx := len(b.STransactions) - 1
		b.Transactions = append(b.Transactions, b.STransactions[tAddIdx])
		b.STransactions = b.STransactions[:tAddIdx]
	}

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// ---------------------------------------------------------------------
	// Append a treasurybase to the regular transaction tree.
	// ---------------------------------------------------------------------

	outs := g.OldestCoinbaseOuts()
	startTip := g.TipName()
	g.NextBlock("tb0", nil, outs[1:], addTreasuryBaseRegular)
	g.RejectTipBlock(ErrMultipleCoinbases)

	// ---------------------------------------------------------------------
	// Append tadd to the regular transaction tree.
	// ---------------------------------------------------------------------

	g.SetTip(startTip)
	g.NextBlock("tadd0", nil, outs[1:], createTAdd(outs[0], 15, params),
		moveTAddRegular)
	g.RejectTipBlock(ErrStakeTxInRegularTree)

	// ---------------------------------------------------------------------
	// Append TSpend to the regular transaction tree.
	// ---------------------------------------------------------------------

	const tspendAmount = 10
	const tspendFee = 0
	tspend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{Amount: tspendAmount - tspendFee}}, tspendFee, 78) // Expiry check not hit.

	g.SetTip(startTip)
	g.NextBlock("tspend0", nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddTransaction(tspend)
	})
	g.RejectTipBlock(ErrStakeTxInRegularTree)
}

func TestTSpendVoteCountSynthetic(t *testing.T) {
	params := chaincfg.RegNetParams()
	tmh := params.TicketMaturity + 2 + 2 // Genesis + block 1 + 2 to reach tvi
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier
	tpb := params.TicketsPerBlock

	expiry := standalone.CalcTSpendExpiry(int64(tmh), tvi, mul)
	start, end, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}
	maxVotes := uint32(tpb) * (end - start)
	quorum := uint64(maxVotes) * params.TreasuryVoteQuorumMultiplier /
		params.TreasuryVoteQuorumDivisor
	requiredVotes := uint64(maxVotes) *
		params.TreasuryVoteRequiredMultiplier /
		params.TreasuryVoteRequiredDivisor

	// Create tspend.
	tspend := newFakeCreateTSpend(privKey, []dcrutil.Amount{100000000}, 1,
		expiry)
	tspendHash := tspend.TxHash()
	_, _, err = stake.CheckTSpend(tspend)
	if err != nil {
		t.Fatal(err)
	}

	// Create yes vote
	yesVote := newFakeCreateVoteTx([]treasuryVoteTuple{{
		tspendHash: tspendHash,
		vote:       0x01, // Yes
	}})
	_, err = stake.CheckSSGenVotes(yesVote)
	if err != nil {
		t.Fatal(err)
	}

	// Create no vote
	noVote := newFakeCreateVoteTx([]treasuryVoteTuple{{
		tspendHash: tspendHash,
		vote:       0x02, // No
	}})
	_, err = stake.CheckSSGenVotes(noVote)
	if err != nil {
		t.Fatal(err)
	}

	fakeTreasuryBase := &wire.MsgTx{}

	var ticketCount int // Overall ticket counter if needed in tests

	tests := []struct {
		name             string
		numNodes         int64
		set              func(*BlockChain, *blockNode)
		blockVersion     int32
		expectedYesVotes int
		expectedNoVotes  int
		failure          bool
	}{{
		name:             "All yes",
		numNodes:         int64(end),
		expectedYesVotes: int(tpb) * int(tvi*mul),
		expectedNoVotes:  0,
		failure:          false,
		set: func(bc *BlockChain, node *blockNode) {
			// Create block.
			bh := node.Header()
			mblk := wire.NewMsgBlock(&bh)

			// Append SSGen
			if node.height >= int64(start) &&
				node.height < int64(end) {
				mblk.STransactions = []*wire.MsgTx{
					fakeTreasuryBase,
					yesVote,
					yesVote,
					yesVote,
					yesVote,
					yesVote,
				}
			}

			// Insert block into the cache.
			bc.addRecentBlock(dcrutil.NewBlock(mblk))
		},
	}, {
		name:             "All no",
		numNodes:         int64(end),
		expectedYesVotes: 0,
		expectedNoVotes:  int(tpb) * int(tvi*mul),
		failure:          true,
		set: func(bc *BlockChain, node *blockNode) {
			// Create block.
			bh := node.Header()
			mblk := wire.NewMsgBlock(&bh)

			// Append SSGen
			if node.height >= int64(start) &&
				node.height < int64(end) {
				mblk.STransactions = []*wire.MsgTx{
					fakeTreasuryBase,
					noVote,
					noVote,
					noVote,
					noVote,
					noVote,
				}
			}

			// Insert block into the cache.
			bc.addRecentBlock(dcrutil.NewBlock(mblk))
		},
	}, {
		name:             "All abstain",
		numNodes:         int64(end),
		expectedYesVotes: 0,
		expectedNoVotes:  0,
		failure:          true,
		set: func(bc *BlockChain, node *blockNode) {
			// Create block.
			bh := node.Header()
			mblk := wire.NewMsgBlock(&bh)

			// Append SSGen
			if node.height >= int64(start) && node.height < int64(end) {
				mblk.STransactions = []*wire.MsgTx{
					fakeTreasuryBase,
				}
			}

			// Insert block into the cache.
			bc.addRecentBlock(dcrutil.NewBlock(mblk))
		},
	}, {
		name:             "Undervote one tvi",
		numNodes:         int64(end),
		expectedYesVotes: 0,
		expectedNoVotes:  0,
		failure:          true,
		set: func(bc *BlockChain, node *blockNode) {
			// Create block.
			bh := node.Header()
			mblk := wire.NewMsgBlock(&bh)

			// Append SSGen
			x := uint32(tvi) // subtract one tvi
			if node.height >= int64(start-x) &&
				node.height < int64(end-x) {
				mblk.STransactions = []*wire.MsgTx{
					fakeTreasuryBase,
				}
			}

			// Insert block into the cache.
			bc.addRecentBlock(dcrutil.NewBlock(mblk))
		},
	}, {
		name:             "Overvote",
		numNodes:         int64(end),
		expectedYesVotes: 0,
		expectedNoVotes:  0,
		failure:          true,
		set: func(bc *BlockChain, node *blockNode) {
			// Create block.
			bh := node.Header()
			mblk := wire.NewMsgBlock(&bh)

			// Append SSGen
			x := uint32(tvi) // add one tvi
			if node.height >= int64(start+x) {
				mblk.STransactions = []*wire.MsgTx{
					fakeTreasuryBase,
				}
			}

			// Insert block into the cache.
			bc.addRecentBlock(dcrutil.NewBlock(mblk))
		},
	}, {
		name:             "All yes quorum",
		numNodes:         int64(end),
		expectedYesVotes: int(quorum),
		expectedNoVotes:  0,
		failure:          false,
		set: func(bc *BlockChain, node *blockNode) {
			// Create block.
			bh := node.Header()
			mblk := wire.NewMsgBlock(&bh)

			// Append SSGen
			if node.height >= int64(start) &&
				node.height < int64(end) {
				mblk.STransactions = []*wire.MsgTx{
					fakeTreasuryBase,
				}

				for i := 0; i < int(tpb); i++ {
					if ticketCount >= int(quorum) {
						break
					}
					mblk.STransactions = append(mblk.STransactions,
						yesVote)
					ticketCount++
				}
			}

			// Insert block into the cache.
			bc.addRecentBlock(dcrutil.NewBlock(mblk))
		},
	}, {
		name:             "All yes quorum - 1",
		numNodes:         int64(end),
		expectedYesVotes: int(quorum - 1),
		expectedNoVotes:  0,
		failure:          true,
		set: func(bc *BlockChain, node *blockNode) {
			// Create block.
			bh := node.Header()
			mblk := wire.NewMsgBlock(&bh)

			// Append SSGen
			if node.height >= int64(start) &&
				node.height < int64(end) {
				mblk.STransactions = []*wire.MsgTx{
					fakeTreasuryBase,
				}

				for i := 0; i < int(tpb); i++ {
					if ticketCount >= int(quorum-1) {
						break
					}
					mblk.STransactions = append(mblk.STransactions,
						yesVote)
					ticketCount++
				}
			}

			// Insert block into the cache.
			bc.addRecentBlock(dcrutil.NewBlock(mblk))
		},
	}, {
		name:             "All yes quorum + 1",
		numNodes:         int64(end),
		expectedYesVotes: int(quorum + 1),
		expectedNoVotes:  0,
		failure:          false,
		set: func(bc *BlockChain, node *blockNode) {
			// Create block.
			bh := node.Header()
			mblk := wire.NewMsgBlock(&bh)

			// Append SSGen
			if node.height >= int64(start) &&
				node.height < int64(end) {
				mblk.STransactions = []*wire.MsgTx{
					fakeTreasuryBase,
				}

				for i := 0; i < int(tpb); i++ {
					if ticketCount >= int(quorum+1) {
						break
					}
					mblk.STransactions = append(mblk.STransactions,
						yesVote)
					ticketCount++
				}
			}

			// Insert block into the cache.
			bc.addRecentBlock(dcrutil.NewBlock(mblk))
		},
	}, {
		name:             "Exactly yes required",
		numNodes:         int64(end),
		expectedYesVotes: int(requiredVotes),
		expectedNoVotes:  int(maxVotes) - int(requiredVotes),
		failure:          false,
		set: func(bc *BlockChain, node *blockNode) {
			// Create block.
			bh := node.Header()
			mblk := wire.NewMsgBlock(&bh)

			// Append SSGen
			if node.height >= int64(start) &&
				node.height < int64(end) {
				mblk.STransactions = []*wire.MsgTx{
					fakeTreasuryBase,
				}

				for i := 0; i < int(tpb); i++ {
					if ticketCount >= int(requiredVotes) {
						mblk.STransactions = append(mblk.STransactions,
							noVote)
					} else {
						mblk.STransactions = append(mblk.STransactions,
							yesVote)
					}
					ticketCount++
				}
			}

			// Insert block into the cache.
			bc.addRecentBlock(dcrutil.NewBlock(mblk))
		},
	}, {
		name:             "Yes required - 1",
		numNodes:         int64(end),
		expectedYesVotes: int(requiredVotes - 1),
		expectedNoVotes:  int(maxVotes) - int(requiredVotes-1),
		failure:          true,
		set: func(bc *BlockChain, node *blockNode) {
			// Create block.
			bh := node.Header()
			mblk := wire.NewMsgBlock(&bh)

			// Append SSGen
			if node.height >= int64(start) &&
				node.height < int64(end) {
				mblk.STransactions = []*wire.MsgTx{
					fakeTreasuryBase,
				}

				for i := 0; i < int(tpb); i++ {
					if ticketCount < int(requiredVotes-1) {
						mblk.STransactions = append(mblk.STransactions,
							yesVote)
					} else {
						mblk.STransactions = append(mblk.STransactions,
							noVote)
					}
					ticketCount++
				}
			}

			// Insert block into the cache.
			bc.addRecentBlock(dcrutil.NewBlock(mblk))
		},
	}, {
		name:             "Yes required + 1",
		numNodes:         int64(end),
		expectedYesVotes: int(requiredVotes + 1),
		expectedNoVotes:  int(maxVotes) - int(requiredVotes+1),
		failure:          false,
		set: func(bc *BlockChain, node *blockNode) {
			// Create block.
			bh := node.Header()
			mblk := wire.NewMsgBlock(&bh)

			// Append SSGen
			if node.height >= int64(start) &&
				node.height < int64(end) {
				mblk.STransactions = []*wire.MsgTx{
					fakeTreasuryBase,
				}

				for i := 0; i < int(tpb); i++ {
					if ticketCount < int(requiredVotes+1) {
						mblk.STransactions = append(mblk.STransactions,
							yesVote)
					} else {
						mblk.STransactions = append(mblk.STransactions,
							noVote)
					}
					ticketCount++
				}
			}

			// Insert block into the cache.
			bc.addRecentBlock(dcrutil.NewBlock(mblk))
		},
	}, {
		name:             "Exactly yes required with abstain",
		numNodes:         int64(end),
		expectedYesVotes: int(requiredVotes),
		expectedNoVotes:  0,
		failure:          false,
		set: func(bc *BlockChain, node *blockNode) {
			// Create block.
			bh := node.Header()
			mblk := wire.NewMsgBlock(&bh)

			// Append SSGen
			if node.height >= int64(start) &&
				node.height < int64(end) {
				mblk.STransactions = []*wire.MsgTx{
					fakeTreasuryBase,
				}

				for i := 0; i < int(tpb); i++ {
					if ticketCount >= int(requiredVotes) {
						// Do nothing.
					} else {
						mblk.STransactions = append(mblk.STransactions,
							yesVote)
					}
					ticketCount++
				}
			}

			// Insert block into the cache.
			bc.addRecentBlock(dcrutil.NewBlock(mblk))
		},
	}}

	for _, test := range tests {
		// Create new BlockChain in order to blow away cache.  Also notice the
		// cache needs to be large enough to hold all of the blocks for these
		// tests without eviction.
		bc := newFakeChain(params)
		node := bc.bestChain.Tip()
		bc.recentBlocks = lru.NewMap[chainhash.Hash, *dcrutil.Block](
			uint32(test.numNodes))

		ticketCount = 0
		for i := int64(0); i < test.numNodes; i++ {
			// Version 9 is regnet decentralized treasury agenda.
			node = newFakeNode(node, test.blockVersion,
				9, 0, time.Now())

			// Override version.
			if test.set != nil {
				test.set(bc, node)
			}

			bc.index.AddNode(node)
			bc.bestChain.SetTip(node)
		}

		if !standalone.IsTreasuryVoteInterval(uint64(node.height), tvi) {
			t.Fatalf("Not a TVI: %v", node.height)
		}

		tv, err := bc.tSpendCountVotes(node.parent, dcrutil.NewTx(tspend))
		if err != nil {
			t.Fatal(err)
		}
		if tv.start != start {
			t.Fatalf("invalid start want %v got %v", start, start)
		}
		if tv.end != end {
			t.Fatalf("invalid end want %v got %v", end, end)
		}
		if tv.yes != test.expectedYesVotes {
			t.Fatalf("invalid yes want %v got %v",
				test.expectedYesVotes, tv.yes)
		}
		if tv.no != test.expectedNoVotes {
			t.Fatalf("invalid no want %v got %v",
				test.expectedNoVotes, tv.no)
		}

		// Check error
		err = bc.checkTSpendHasVotes(node.parent, dcrutil.NewTx(tspend))
		if test.failure {
			if err == nil {
				t.Fatalf("expected failure")
			}
		} else {
			if err != nil {
				t.Fatalf("%v: %v", test.name, err)
			}
		}
	}
}

// TestTSpendTooManyTAdds tests that a block that contains more TAdds than
// allowed by consensus rules is rejected while a block containing exactly the
// maximum amount is accepted.
func TestTSpendTooManyTAdds(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and mark
	// the treasury agenda as always active.
	params := quickVoteActivationParams()
	const voteID = chaincfg.VoteIDTreasury
	forceDeploymentResult(t, params, voteID, "yes")

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// addTAdds is a helper function that returns a munge function that
	// includes the specified tadds in the stake tree of a block.
	addTAdds := func(tadds []*wire.MsgTx) func(b *wire.MsgBlock) {
		return func(b *wire.MsgBlock) {
			for _, tx := range tadds {
				b.AddSTransaction(tx)
			}
		}
	}

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// The maximum number of tadds is hardcoded here instead of using a
	// global const so that any code changes that unintentionally cause
	// this maximum to change in blockchain code are flagged by this test.
	const maxNbTadds = 20

	// ---------------------------------------------------------------------
	// Generate enough blocks to have spendable outputs to create the
	// required number of tadds. Due to having lowered the coinbase
	// maturity, we'll need to manually generate and store these outputs
	// until we have enough.
	//
	//   ... -> bout0 -> ... -> bout#
	// ---------------------------------------------------------------------

	neededOuts := maxNbTadds + 1
	var oldOuts []chaingen.SpendableOut
	outs := g.OldestCoinbaseOuts()
	for i := uint32(0); i < uint32(neededOuts); i++ {
		name := fmt.Sprintf("bout%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		oldOuts = append(oldOuts, outs[0])
	}

	// Create more than the maximum allowed number of TAdds.
	taddAmount := dcrutil.Amount(1701)
	taddFee := dcrutil.Amount(0)
	tadds := make([]*wire.MsgTx, 0, maxNbTadds+1)
	for i := 0; i < maxNbTadds+1; i++ {
		tadd := g.CreateTreasuryTAdd(&oldOuts[i], taddAmount, taddFee)
		tadds = append(tadds, tadd)
	}

	// ---------------------------------------------------------------------
	// Attempt to mine a block with an invalid number of tadds.
	//
	// ... -> bout#
	//             \ -> btoomanytadds
	// ---------------------------------------------------------------------

	tipName := g.TipName()
	g.NextBlock("btoomanytadds", nil, outs[1:], addTAdds(tadds))
	g.RejectTipBlock(ErrTooManyTAdds)

	// ---------------------------------------------------------------------
	// Mine a block with the maximum number of tadds.
	//
	// ... -> bout# -> bmaxtadds
	// ---------------------------------------------------------------------

	g.SetTip(tipName)
	g.NextBlock("bmaxtadds", nil, outs[1:], addTAdds(tadds[1:]))
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()
}
