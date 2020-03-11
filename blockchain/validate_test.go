// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"compress/bzip2"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	mrand "math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/v3/chaingen"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/wire"
)

// TestBlockchainSpendJournal tests for whether or not the spend journal is being
// written to disk correctly on a live blockchain.
func TestBlockchainSpendJournal(t *testing.T) {
	// Update parameters to reflect what is expected by the legacy data.
	params := chaincfg.RegNetParams()
	params.GenesisBlock.Header.MerkleRoot = *mustParseHash("a216ea043f0d481a072424af646787794c32bcefd3ed181a090319bbf8a37105")
	params.GenesisBlock.Header.Timestamp = time.Unix(1401292357, 0)
	params.GenesisBlock.Transactions[0].TxIn[0].ValueIn = 0
	params.PubKeyHashAddrID = [2]byte{0x0e, 0x91}
	params.StakeBaseSigScript = []byte{0xde, 0xad, 0xbe, 0xef}
	params.OrganizationPkScript = hexToBytes("a914cbb08d6ca783b533b2c7d24a51fbca92d937bf9987")
	params.BlockOneLedger = []chaincfg.TokenPayout{{
		ScriptVersion: 0,
		Script:        hexToBytes("76a91494ff37a0ee4d48abc45f70474f9b86f9da69a70988ac"),
		Amount:        100000 * 1e8,
	}, {
		ScriptVersion: 0,
		Script:        hexToBytes("76a914a6753ebbc08e2553e7dd6d64bdead4bcbff4fcf188ac"),
		Amount:        100000 * 1e8,
	}, {
		ScriptVersion: 0,
		Script:        hexToBytes("76a9147aa3211c2ead810bbf5911c275c69cc196202bd888ac"),
		Amount:        100000 * 1e8,
	}}
	params.GenesisHash = params.GenesisBlock.BlockHash()

	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("spendjournalunittest", params)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}
	defer teardownFunc()

	// Load up the rest of the blocks up to HEAD.
	filename := filepath.Join("testdata", "reorgto179.bz2")
	fi, err := os.Open(filename)
	if err != nil {
		t.Errorf("Failed to open %s: %v", filename, err)
	}
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data
	bcDecoder := gob.NewDecoder(bcBuf)
	blockChain := make(map[int64][]byte)

	// Decode the blockchain into the map
	if err := bcDecoder.Decode(&blockChain); err != nil {
		t.Errorf("error decoding test blockchain: %v", err.Error())
	}

	// Load up the short chain
	finalIdx1 := 179
	for i := 1; i < finalIdx1+1; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain[int64(i)])
		if err != nil {
			t.Fatalf("NewBlockFromBytes error: %v", err.Error())
		}

		forkLen, err := chain.ProcessBlock(bl, BFNone)
		if err != nil {
			t.Fatalf("ProcessBlock error at height %v: %v", i, err.Error())
		}
		isMainChain := forkLen == 0
		if !isMainChain {
			t.Fatalf("block %s (height %d) should have been "+
				"accepted to the main chain", bl.Hash(),
				bl.MsgBlock().Header.Height)
		}
	}

	// Loop through all of the blocks and ensure the number of spent outputs
	// matches up with the information loaded from the spend journal.
	err = chain.db.View(func(dbTx database.Tx) error {
		for i := int64(2); i <= chain.bestChain.Tip().height; i++ {
			node := chain.bestChain.NodeByHeight(i)
			if node == nil {
				str := fmt.Sprintf("no block at height %d exists", i)
				return errNotInMainChain(str)
			}
			block, err := dbFetchBlockByNode(dbTx, node)
			if err != nil {
				return err
			}

			ntx := countSpentOutputs(block)
			stxos, err := dbFetchSpendJournalEntry(dbTx, block)
			if err != nil {
				return err
			}

			if ntx != len(stxos) {
				return fmt.Errorf("bad number of stxos "+
					"calculated at "+"height %v, got %v "+
					"expected %v", i, len(stxos), ntx)
			}
		}

		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestSequenceLocksActive ensure the sequence locks are detected as active or
// not as expected in all possible scenarios.
func TestSequenceLocksActive(t *testing.T) {
	now := time.Now().Unix()
	tests := []struct {
		name          string
		seqLockHeight int64
		seqLockTime   int64
		blockHeight   int64
		medianTime    int64
		want          bool
	}{
		{
			// Block based sequence lock with height at min
			// required.
			name:          "min active block height",
			seqLockHeight: 1000,
			seqLockTime:   -1,
			blockHeight:   1001,
			medianTime:    now + 31,
			want:          true,
		},
		{
			// Time based sequence lock with relative time at min
			// required.
			name:          "min active median time",
			seqLockHeight: -1,
			seqLockTime:   now + 30,
			blockHeight:   1001,
			medianTime:    now + 31,
			want:          true,
		},
		{
			// Block based sequence lock at same height.
			name:          "same height",
			seqLockHeight: 1000,
			seqLockTime:   -1,
			blockHeight:   1000,
			medianTime:    now + 31,
			want:          false,
		},
		{
			// Time based sequence lock with relative time equal to
			// lock time.
			name:          "same median time",
			seqLockHeight: -1,
			seqLockTime:   now + 30,
			blockHeight:   1001,
			medianTime:    now + 30,
			want:          false,
		},
		{
			// Block based sequence lock with relative height below
			// required.
			name:          "height below required",
			seqLockHeight: 1000,
			seqLockTime:   -1,
			blockHeight:   999,
			medianTime:    now + 31,
			want:          false,
		},
		{
			// Time based sequence lock with relative time before
			// required.
			name:          "median time before required",
			seqLockHeight: -1,
			seqLockTime:   now + 30,
			blockHeight:   1001,
			medianTime:    now + 29,
			want:          false,
		},
	}

	for _, test := range tests {
		seqLock := SequenceLock{
			MinHeight: test.seqLockHeight,
			MinTime:   test.seqLockTime,
		}
		got := SequenceLockActive(&seqLock, test.blockHeight,
			time.Unix(test.medianTime, 0))
		if got != test.want {
			t.Errorf("%s: mismatched sequence lock status - got %v, "+
				"want %v", test.name, got, test.want)
			continue
		}
	}
}

// quickVoteActivationParams returns a set of test chain parameters which allow
// for quicker vote activation as compared to various existing network params by
// reducing the required maturities, the ticket pool size, the stake enabled and
// validation heights, the proof-of-work block version upgrade window, the stake
// version interval, and the rule change activation interval.
func quickVoteActivationParams() *chaincfg.Params {
	params := chaincfg.RegNetParams()
	params.WorkDiffWindowSize = 200000
	params.WorkDiffWindows = 1
	params.TargetTimespan = params.TargetTimePerBlock *
		time.Duration(params.WorkDiffWindowSize)
	params.CoinbaseMaturity = 2
	params.BlockEnforceNumRequired = 5
	params.BlockRejectNumRequired = 7
	params.BlockUpgradeNumToCheck = 10
	params.TicketMaturity = 2
	params.TicketPoolSize = 4
	params.TicketExpiry = 6 * uint32(params.TicketPoolSize)
	params.StakeEnabledHeight = int64(params.CoinbaseMaturity) +
		int64(params.TicketMaturity)
	params.StakeValidationHeight = int64(params.CoinbaseMaturity) +
		int64(params.TicketPoolSize)*2
	params.StakeVersionInterval = 10
	params.RuleChangeActivationInterval = uint32(params.TicketPoolSize) *
		uint32(params.TicketsPerBlock)
	params.RuleChangeActivationQuorum = params.RuleChangeActivationInterval *
		uint32(params.TicketsPerBlock*100) / 1000

	return params
}

// TestLegacySequenceLocks ensure that sequence locks within blocks behave as
// expected according to the legacy semantics in previous version of the
// software.
func TestLegacySequenceLocks(t *testing.T) {
	// Use a set of test chain parameters which allow for quicker vote
	// activation as compared to various existing network params.
	params := quickVoteActivationParams()

	// Clone the parameters so they can be mutated, find the correct deployment
	// for the LN features agenda, and, finally, ensure it is always available
	// to vote by removing the time constraints to prevent test failures when
	// the real expiration time passes.
	const lnfVoteID = chaincfg.VoteIDLNFeatures
	params = cloneParams(params)
	lnfVersion, deployment, err := findDeployment(params, lnfVoteID)
	if err != nil {
		t.Fatal(err)
	}
	removeDeploymentTimeConstraints(deployment)

	// Create a test harness initialized with the genesis block as the tip.
	g, teardownFunc := newChaingenHarness(t, params, "seqlocksoldsemanticstest")
	defer teardownFunc()

	// replaceLNFeaturesVersions is a munge function which modifies the provided
	// block by replacing the block, stake, and vote versions with the LN
	// features deployment version.
	replaceLNFeaturesVersions := func(b *wire.MsgBlock) {
		chaingen.ReplaceBlockVersion(int32(lnfVersion))(b)
		chaingen.ReplaceStakeVersion(lnfVersion)(b)
		chaingen.ReplaceVoteVersions(lnfVersion)(b)
	}

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks with the appropriate vote bits set
	// to reach one block prior to the LN features agenda becoming active.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()
	g.AdvanceFromSVHToActiveAgenda(lnfVoteID)

	// ---------------------------------------------------------------------
	// Perform a series of sequence lock tests now that ln feature
	// enforcement is active.
	// ---------------------------------------------------------------------

	// enableSeqLocks modifies the passed transaction to enable sequence locks
	// for the provided input.
	enableSeqLocks := func(tx *wire.MsgTx, txInIdx int) {
		tx.Version = 2
		tx.TxIn[txInIdx].Sequence = 0
	}

	// ---------------------------------------------------------------------
	// Create block that has a transaction with an input shared with a
	// transaction in the stake tree and has several outputs used in
	// subsequent blocks.  Also, enable sequence locks for the first of
	// those outputs.
	//
	//   ... -> b0
	// ---------------------------------------------------------------------

	outs := g.OldestCoinbaseOuts()
	b0 := g.NextBlock("b0", &outs[0], outs[1:], replaceLNFeaturesVersions,
		func(b *wire.MsgBlock) {
			// Save the current outputs of the spend tx and clear them.
			tx := b.Transactions[1]
			origOut := tx.TxOut[0]
			origOpReturnOut := tx.TxOut[1]
			tx.TxOut = tx.TxOut[:0]

			// Evenly split the original output amount over multiple outputs.
			const numOutputs = 6
			amount := origOut.Value / numOutputs
			for i := 0; i < numOutputs; i++ {
				if i == numOutputs-1 {
					amount = origOut.Value - amount*(numOutputs-1)
				}
				tx.AddTxOut(wire.NewTxOut(amount, origOut.PkScript))
			}

			// Add the original op return back to the outputs and enable
			// sequence locks for the first output.
			tx.AddTxOut(origOpReturnOut)
			enableSeqLocks(tx, 0)
		})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	// ---------------------------------------------------------------------
	// Create block that spends from an output created in the previous
	// block.
	//
	//   ... -> b0 -> b1a
	// ---------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b1a", nil, outs[1:], replaceLNFeaturesVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b0, 1, 0)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			enableSeqLocks(tx, 0)
			b.AddTransaction(tx)
		})
	g.AcceptTipBlock()

	// ---------------------------------------------------------------------
	// Create block that involves reorganize to a sequence lock spending
	// from an output created in a block prior to the parent also spent on
	// on the side chain.
	//
	//   ... -> b0 -> b1  -> b2
	//            \-> b1a
	// ---------------------------------------------------------------------
	g.SetTip("b0")
	g.NextBlock("b1", nil, outs[1:], replaceLNFeaturesVersions)
	g.SaveTipCoinbaseOuts()
	g.AcceptedToSideChainWithExpectedTip("b1a")

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b2", nil, outs[1:], replaceLNFeaturesVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b0, 1, 0)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			enableSeqLocks(tx, 0)
			b.AddTransaction(tx)
		})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()
	g.ExpectTip("b2")

	// ---------------------------------------------------------------------
	// Create block that involves a sequence lock on a vote.
	//
	//   ... -> b2 -> b3
	// ---------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b3", nil, outs[1:], replaceLNFeaturesVersions,
		func(b *wire.MsgBlock) {
			enableSeqLocks(b.STransactions[0], 0)
		})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	// ---------------------------------------------------------------------
	// Create block that involves a sequence lock on a ticket.
	//
	//   ... -> b3 -> b4
	// ---------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b4", nil, outs[1:], replaceLNFeaturesVersions,
		func(b *wire.MsgBlock) {
			enableSeqLocks(b.STransactions[5], 0)
		})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	// ---------------------------------------------------------------------
	// Create two blocks such that the tip block involves a sequence lock
	// spending from a different output of a transaction the parent block
	// also spends from.
	//
	//   ... -> b4 -> b5 -> b6
	// ---------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b5", nil, outs[1:], replaceLNFeaturesVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b0, 1, 1)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			b.AddTransaction(tx)
		})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b6", nil, outs[1:], replaceLNFeaturesVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b0, 1, 2)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			enableSeqLocks(tx, 0)
			b.AddTransaction(tx)
		})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	// ---------------------------------------------------------------------
	// Create block that involves a sequence lock spending from a regular
	// tree transaction earlier in the block.  It should be rejected due
	// to a consensus bug.
	//
	//   ... -> b6
	//            \-> b7
	// ---------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b7", &outs[0], outs[1:], replaceLNFeaturesVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b, 1, 0)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			enableSeqLocks(tx, 0)
			b.AddTransaction(tx)
		})
	g.RejectTipBlock(ErrMissingTxOut)

	// ---------------------------------------------------------------------
	// Create block that involves a sequence lock spending from a block
	// prior to the parent.  It should be rejected due to a consensus bug.
	//
	//   ... -> b6 -> b8
	//                  \-> b9
	// ---------------------------------------------------------------------

	g.SetTip("b6")
	g.NextBlock("b8", nil, outs[1:], replaceLNFeaturesVersions)
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b9", nil, outs[1:], replaceLNFeaturesVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b0, 1, 3)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			enableSeqLocks(tx, 0)
			b.AddTransaction(tx)
		})
	g.RejectTipBlock(ErrMissingTxOut)

	// ---------------------------------------------------------------------
	// Create two blocks such that the tip block involves a sequence lock
	// spending from a different output of a transaction the parent block
	// also spends from when the parent block has been disapproved.    It
	// should be rejected due to a consensus bug.
	//
	//   ... -> b8 -> b10
	//                   \-> b11
	// ---------------------------------------------------------------------

	const (
		// vbDisapprovePrev and vbApprovePrev represent no and yes votes,
		// respectively, on whether or not to approve the previous block.
		vbDisapprovePrev = 0x0000
		vbApprovePrev    = 0x0001
	)

	g.SetTip("b8")
	g.NextBlock("b10", nil, outs[1:], replaceLNFeaturesVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b0, 1, 4)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			b.AddTransaction(tx)
		})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b11", nil, outs[1:], replaceLNFeaturesVersions,
		chaingen.ReplaceVotes(vbDisapprovePrev, lnfVersion),
		func(b *wire.MsgBlock) {
			b.Header.VoteBits &^= vbApprovePrev
			spend := chaingen.MakeSpendableOut(b0, 1, 5)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			enableSeqLocks(tx, 0)
			b.AddTransaction(tx)
		})
	g.RejectTipBlock(ErrMissingTxOut)
}

// TestCheckBlockSanity tests the context free block sanity checks with blocks
// not on a chain.
func TestCheckBlockSanity(t *testing.T) {
	params := chaincfg.RegNetParams()
	timeSource := NewMedianTime()
	block := dcrutil.NewBlock(&badBlock)
	err := CheckBlockSanity(block, timeSource, params)
	if err == nil {
		t.Fatalf("block should fail.\n")
	}
}

// TestCheckBlockHeaderContext tests that genesis block passes context headers
// because its parent is nil.
func TestCheckBlockHeaderContext(t *testing.T) {
	// Create a new database for the blocks.
	params := chaincfg.RegNetParams()
	dbPath := filepath.Join(os.TempDir(), "examplecheckheadercontext")
	_ = os.RemoveAll(dbPath)
	db, err := database.Create("ffldb", dbPath, params.Net)
	if err != nil {
		t.Fatalf("Failed to create database: %v\n", err)
		return
	}
	defer os.RemoveAll(dbPath)
	defer db.Close()

	// Create a new BlockChain instance using the underlying database for
	// the simnet network.
	chain, err := New(context.Background(),
		&Config{
			DB:          db,
			ChainParams: params,
			TimeSource:  NewMedianTime(),
		})
	if err != nil {
		t.Fatalf("Failed to create chain instance: %v\n", err)
		return
	}

	err = chain.checkBlockHeaderContext(&params.GenesisBlock.Header, nil, BFNone)
	if err != nil {
		t.Fatalf("genesisblock should pass just by definition: %v\n", err)
		return
	}

	// Test failing checkBlockHeaderContext when calcNextRequiredDifficulty
	// fails.
	block := dcrutil.NewBlock(&badBlock)
	newNode := newBlockNode(&block.MsgBlock().Header, nil)
	err = chain.checkBlockHeaderContext(&block.MsgBlock().Header, newNode, BFNone)
	if err == nil {
		t.Fatalf("Should fail due to bad diff in newNode\n")
		return
	}
}

// TestTxValidationErrors ensures certain malformed freestanding transactions
// are rejected as as expected.
func TestTxValidationErrors(t *testing.T) {
	// Create a transaction that is too large
	tx := wire.NewMsgTx()
	prevOut := wire.NewOutPoint(&chainhash.Hash{0x01}, 0, wire.TxTreeRegular)
	tx.AddTxIn(wire.NewTxIn(prevOut, 0, nil))
	pkScript := bytes.Repeat([]byte{0x00}, wire.MaxBlockPayload)
	tx.AddTxOut(wire.NewTxOut(0, pkScript))

	// Assert the transaction is larger than the max allowed size.
	txSize := tx.SerializeSize()
	if txSize <= wire.MaxBlockPayload {
		t.Fatalf("generated transaction is not large enough -- got "+
			"%d, want > %d", txSize, wire.MaxBlockPayload)
	}

	// Ensure transaction is rejected due to being too large.
	err := CheckTransactionSanity(tx, chaincfg.MainNetParams())
	var rerr RuleError
	if !errors.As(err, &rerr) {
		t.Fatalf("CheckTransactionSanity: unexpected error type for "+
			"transaction that is too large -- got %T", err)
	}
	if rerr.ErrorCode != ErrTxTooBig {
		t.Fatalf("CheckTransactionSanity: unexpected error code for "+
			"transaction that is too large -- got %v, want %v",
			rerr.ErrorCode, ErrTxTooBig)
	}
}

// badBlock is an intentionally bad block that should fail the context-less
// sanity checks.
var badBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:      1,
		MerkleRoot:   *newHashFromStr("66aa7491b9adce110585ccab7e3fb5fe280de174530cca10eba2c6c3df01c10d"),
		VoteBits:     uint16(0x0000),
		FinalState:   [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Voters:       uint16(0x0000),
		FreshStake:   uint8(0x00),
		Revocations:  uint8(0x00),
		Timestamp:    time.Unix(1401292357, 0), // 2009-01-08 20:54:25 -0600 CST
		PoolSize:     uint32(0),
		Bits:         0x207fffff, // 545259519
		SBits:        int64(0x0000000000000000),
		Nonce:        0x37580963,
		StakeVersion: uint32(0),
		Height:       uint32(0),
	},
	Transactions:  []*wire.MsgTx{},
	STransactions: []*wire.MsgTx{},
}

// TestCheckConnectBlockTemplate ensures that the code which deals with
// checking block templates works as expected.
func TestCheckConnectBlockTemplate(t *testing.T) {
	// Create a test harness initialized with the genesis block as the tip.
	params := chaincfg.RegNetParams()
	g, teardownFunc := newChaingenHarness(t, params, "connectblktemplatetest")
	defer teardownFunc()

	// Define some additional convenience helper functions to process the
	// current tip block associated with the generator.
	//
	// acceptedBlockTemplate expected the block to considered a valid block
	// template.
	//
	// rejectedBlockTemplate expects the block to be considered an invalid
	// block template due to the provided error code.
	acceptedBlockTemplate := func() {
		msgBlock := g.Tip()
		blockHeight := msgBlock.Header.Height
		block := dcrutil.NewBlock(msgBlock)
		t.Logf("Testing block template %s (hash %s, height %d)",
			g.TipName(), block.Hash(), blockHeight)

		err := g.chain.CheckConnectBlockTemplate(block)
		if err != nil {
			t.Fatalf("block template %q (hash %s, height %d) should "+
				"have been accepted: %v", g.TipName(),
				block.Hash(), blockHeight, err)
		}
	}
	rejectedBlockTemplate := func(code ErrorCode) {
		msgBlock := g.Tip()
		blockHeight := msgBlock.Header.Height
		block := dcrutil.NewBlock(msgBlock)
		t.Logf("Testing block template %s (hash %s, height %d)",
			g.TipName(), block.Hash(), blockHeight)

		err := g.chain.CheckConnectBlockTemplate(block)
		if err == nil {
			t.Fatalf("block template %q (hash %s, height %d) should "+
				"not have been accepted", g.TipName(), block.Hash(),
				blockHeight)
		}

		// Ensure the error code is of the expected type and the reject
		// code matches the value specified in the test instance.
		var rerr RuleError
		if !errors.As(err, &rerr) {
			t.Fatalf("block template %q (hash %s, height %d) "+
				"returned unexpected error type -- got %T, want "+
				"blockchain.RuleError", g.TipName(),
				block.Hash(), blockHeight, err)
		}
		if rerr.ErrorCode != code {
			t.Fatalf("block template %q (hash %s, height %d) does "+
				"not have expected reject code -- got %v, want %v",
				g.TipName(), block.Hash(), blockHeight,
				rerr.ErrorCode, code)
		}
	}

	// changeNonce is a munger that modifies the block by changing the header
	// nonce to a pseudo-random value.
	prng := mrand.New(mrand.NewSource(0))
	changeNonce := func(b *wire.MsgBlock) {
		// Change the nonce so the block isn't actively solved.
		b.Header.Nonce = prng.Uint32()
	}

	// Shorter versions of useful params for convenience.
	ticketsPerBlock := params.TicketsPerBlock
	coinbaseMaturity := params.CoinbaseMaturity
	stakeEnabledHeight := params.StakeEnabledHeight
	stakeValidationHeight := params.StakeValidationHeight

	// ---------------------------------------------------------------------
	// First block templates.
	//
	// NOTE: The advance funcs on the harness are intentionally not used in
	// these tests since they need to manually test block templates at all
	// heights.
	// ---------------------------------------------------------------------

	// Produce an initial block with too much coinbase and ensure the block
	// template is rejected.
	//
	//   genesis
	//          \-> bfbbad
	g.CreateBlockOne("bfbbad", 1)
	g.AssertTipHeight(1)
	rejectedBlockTemplate(ErrBadCoinbaseValue)

	// Produce a valid, but unsolved initial block and ensure the block template
	// is accepted while the unsolved block is rejected.
	//
	//   genesis
	//          \-> bfbunsolved
	g.SetTip("genesis")
	bfbunsolved := g.CreateBlockOne("bfbunsolved", 0, changeNonce)
	// Since the difficulty is so low in the tests, the block might still
	// end up being inadvertently solved.  It can't be checked inside the
	// munger because the block is finalized after the function returns and
	// those changes could also inadvertently solve the block.  Thus, just
	// increment the nonce until it's not solved and then replace it in the
	// generator's state.
	{
		origHash := bfbunsolved.BlockHash()
		for chaingen.IsSolved(&bfbunsolved.Header) {
			bfbunsolved.Header.Nonce++
		}
		g.UpdateBlockState("bfbunsolved", origHash, "bfbunsolved", bfbunsolved)
	}
	g.AssertTipHeight(1)
	acceptedBlockTemplate()
	g.RejectTipBlock(ErrHighHash)
	g.ExpectTip("genesis")

	// Produce a valid and solved initial block.
	//
	//   genesis -> bfb
	g.SetTip("genesis")
	g.CreateBlockOne("bfb", 0)
	g.AssertTipHeight(1)
	g.AcceptTipBlock()

	// ---------------------------------------------------------------------
	// Generate enough blocks to have mature coinbase outputs to work with.
	//
	// Also, ensure that each block is considered a valid template along the
	// way.
	//
	//   genesis -> bfb -> bm0 -> bm1 -> ... -> bm#
	// ---------------------------------------------------------------------

	var tipName string
	for i := uint16(0); i < coinbaseMaturity; i++ {
		blockName := fmt.Sprintf("bm%d", i)
		g.NextBlock(blockName, nil, nil)
		g.SaveTipCoinbaseOuts()
		acceptedBlockTemplate()
		g.AcceptTipBlock()
		tipName = blockName
	}
	g.AssertTipHeight(uint32(coinbaseMaturity) + 1)

	// ---------------------------------------------------------------------
	// Generate block templates that include invalid ticket purchases.
	// ---------------------------------------------------------------------

	// Create a block template with a ticket that claims too much input
	// amount.
	//
	//   ... -> bm#
	//             \-> btixt1
	tempOuts := g.OldestCoinbaseOuts()
	tempTicketOuts := tempOuts[1:]
	g.NextBlock("btixt1", nil, tempTicketOuts, func(b *wire.MsgBlock) {
		changeNonce(b)
		b.STransactions[3].TxIn[0].ValueIn--
	})
	rejectedBlockTemplate(ErrFraudAmountIn)

	// Create a block template with a ticket that does not pay enough.
	//
	//   ... -> bm#
	//             \-> btixt2
	g.SetTip(tipName)
	g.NextBlock("btixt2", nil, tempTicketOuts, func(b *wire.MsgBlock) {
		changeNonce(b)
		b.STransactions[2].TxOut[0].Value--
	})
	rejectedBlockTemplate(ErrNotEnoughStake)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake enabled height while
	// creating ticket purchases that spend from the coinbases matured
	// above.  This will also populate the pool of immature tickets.
	//
	// Also, ensure that each block is considered a valid template along the
	// way.
	//
	//   ... -> bm# ... -> bse0 -> bse1 -> ... -> bse#
	// ---------------------------------------------------------------------

	// Use the already popped outputs.
	g.SetTip(tipName)
	g.NextBlock("bse0", nil, tempTicketOuts)
	g.SaveTipCoinbaseOuts()
	acceptedBlockTemplate()
	g.AcceptTipBlock()

	var ticketsPurchased int
	for i := int64(1); int64(g.Tip().Header.Height) < stakeEnabledHeight; i++ {
		outs := g.OldestCoinbaseOuts()
		ticketOuts := outs[1:]
		ticketsPurchased += len(ticketOuts)
		blockName := fmt.Sprintf("bse%d", i)
		g.NextBlock(blockName, nil, ticketOuts)
		g.SaveTipCoinbaseOuts()
		acceptedBlockTemplate()
		g.AcceptTipBlock()
	}
	g.AssertTipHeight(uint32(stakeEnabledHeight))

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake validation height while
	// continuing to purchase tickets using the coinbases matured above and
	// allowing the immature tickets to mature and thus become live.
	//
	// Also, ensure that each block is considered a valid template along the
	// way.
	//
	//   ... -> bse# -> bsv0 -> bsv1 -> ... -> bsv#
	// ---------------------------------------------------------------------

	targetPoolSize := g.Params().TicketPoolSize * ticketsPerBlock
	for i := int64(0); int64(g.Tip().Header.Height) < stakeValidationHeight; i++ {
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
		acceptedBlockTemplate()
		g.AcceptTipBlock()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight))

	// ---------------------------------------------------------------------
	// Generate enough blocks to have a known distance to the first mature
	// coinbase outputs for all tests that follow.  These blocks continue
	// to purchase tickets to avoid running out of votes.
	//
	// Also, ensure that each block is considered a valid template along the
	// way.
	//
	//   ... -> bsv# -> bbm0 -> bbm1 -> ... -> bbm#
	// ---------------------------------------------------------------------

	for i := uint16(0); i < coinbaseMaturity; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bbm%d", i)
		g.NextBlock(blockName, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		acceptedBlockTemplate()
		g.AcceptTipBlock()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight) + uint32(coinbaseMaturity))

	// Collect spendable outputs into two different slices.  The outs slice
	// is intended to be used for regular transactions that spend from the
	// output, while the ticketOuts slice is intended to be used for stake
	// ticket purchases.
	var outs []*chaingen.SpendableOut
	var ticketOuts [][]chaingen.SpendableOut
	for i := uint16(0); i < coinbaseMaturity; i++ {
		coinbaseOuts := g.OldestCoinbaseOuts()
		outs = append(outs, &coinbaseOuts[0])
		ticketOuts = append(ticketOuts, coinbaseOuts[1:])
	}

	// ---------------------------------------------------------------------
	// Generate block templates that build on ancestors of the tip.
	// ---------------------------------------------------------------------

	// Start by building a few of blocks at current tip (value in parens
	// is which output is spent):
	//
	//   ... -> b1(0) -> b2(1) -> b3(2)
	g.NextBlock("b1", outs[0], ticketOuts[0])
	g.AcceptTipBlock()

	g.NextBlock("b2", outs[1], ticketOuts[1])
	g.AcceptTipBlock()

	g.NextBlock("b3", outs[2], ticketOuts[2])
	g.AcceptTipBlock()

	// Create a block template that forks from b1.  It should not be allowed
	// since it is not the current tip or its parent.
	//
	//   ... -> b1(0) -> b2(1)  -> b3(2)
	//               \-> b2at(1)
	g.SetTip("b1")
	g.NextBlock("b2at", outs[1], ticketOuts[1], changeNonce)
	rejectedBlockTemplate(ErrInvalidTemplateParent)

	// Create a block template that forks from b2.  It should be accepted
	// because it is the current tip's parent.
	//
	//   ... -> b2(1) -> b3(2)
	//               \-> b3at(2)
	g.SetTip("b2")
	g.NextBlock("b3at", outs[2], ticketOuts[2], changeNonce)
	acceptedBlockTemplate()

	// ---------------------------------------------------------------------
	// Generate block templates that build on the tip's parent, but include
	// invalid votes.
	// ---------------------------------------------------------------------

	// Create a block template that forks from b2 (the tip's parent) with
	// votes that spend invalid tickets.
	//
	//   ... -> b2(1) -> b3(2)
	//               \-> b3bt(2)
	g.SetTip("b2")
	g.NextBlock("b3bt", outs[2], ticketOuts[1], changeNonce)
	rejectedBlockTemplate(ErrMissingTxOut)

	// Same as before but based on the current tip.
	//
	//   ... -> b2(1) -> b3(2)
	//                        \-> b4at(3)
	g.SetTip("b3")
	g.NextBlock("b4at", outs[3], ticketOuts[2], changeNonce)
	rejectedBlockTemplate(ErrMissingTxOut)

	// Create a block template that forks from b2 (the tip's parent) with
	// a vote that pays too much.
	//
	//   ... -> b2(1) -> b3(2)
	//               \-> b3ct(2)
	g.SetTip("b2")
	g.NextBlock("b3ct", outs[2], ticketOuts[2], func(b *wire.MsgBlock) {
		changeNonce(b)
		b.STransactions[0].TxOut[0].Value++
	})
	rejectedBlockTemplate(ErrSpendTooHigh)

	// Same as before but based on the current tip.
	//
	//   ... -> b2(1) -> b3(2)
	//                        \-> b4bt(3)
	g.SetTip("b3")
	g.NextBlock("b4bt", outs[3], ticketOuts[3], func(b *wire.MsgBlock) {
		changeNonce(b)
		b.STransactions[0].TxOut[0].Value++
	})
	rejectedBlockTemplate(ErrSpendTooHigh)

	// ---------------------------------------------------------------------
	// Generate block templates that build on the tip and its parent after a
	// forced reorg.
	// ---------------------------------------------------------------------

	// Create a fork from b2.  There should not be a reorg since b3 was seen
	// first.
	//
	//   ... -> b2(1) -> b3(2)
	//               \-> b3a(2)
	g.SetTip("b2")
	g.NextBlock("b3a", outs[2], ticketOuts[2])
	g.AcceptedToSideChainWithExpectedTip("b3")

	// Force tip reorganization to b3a.
	//
	//   ... -> b2(1) -> b3a(2)
	//               \-> b3(2)
	g.ForceTipReorg("b3", "b3a")
	g.ExpectTip("b3a")

	// Create a block template that forks from b2 (the tip's parent) and
	// ensure it is still accepted after the forced reorg.
	//
	//   ... -> b2(1) -> b3a(2)
	//               \-> b3dt(2)
	g.SetTip("b2")
	g.NextBlock("b3dt", outs[2], ticketOuts[2], changeNonce)
	acceptedBlockTemplate()
	g.ExpectTip("b3a") // Ensure chain tip didn't change.

	// Create a block template that builds on the current tip and ensure it
	// it is still accepted after the forced reorg.
	//
	//   ... -> b2(1) -> b3a(2)
	//                         \-> b4ct(3)
	g.SetTip("b3a")
	g.NextBlock("b4ct", outs[3], ticketOuts[3], changeNonce)
	acceptedBlockTemplate()
}
