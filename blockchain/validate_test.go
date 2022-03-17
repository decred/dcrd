// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"compress/bzip2"
	"context"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	mrand "math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v4"
	"github.com/decred/dcrd/blockchain/v5/chaingen"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
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
	chain, err := chainSetup(t, params)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}

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

		forkLen, err := chain.ProcessBlock(bl)
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

			ntx := countSpentOutputs(block, noTreasury)
			stxos, err := dbFetchSpendJournalEntry(dbTx, block,
				noTreasury)
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
	// Create a test block database.
	const testDbType = "ffldb"
	params := chaincfg.RegNetParams()
	db, err := createTestDatabase(t, testDbType, params.Net)
	if err != nil {
		t.Fatalf("Failed to create database: %v\n", err)
	}

	// Create a test UTXO database.
	utxoDb, teardownUtxoDb, err := createTestUtxoDatabase(t)
	if err != nil {
		t.Fatalf("Failed to create UTXO database: %v\n", err)
	}
	defer teardownUtxoDb()

	// Create a new BlockChain instance using the underlying database for
	// the simnet network.
	utxoBackend := NewLevelDbUtxoBackend(utxoDb)
	chain, err := New(context.Background(),
		&Config{
			DB:          db,
			UtxoBackend: utxoBackend,
			ChainParams: params,
			TimeSource:  NewMedianTime(),
			UtxoCache: NewUtxoCache(&UtxoCacheConfig{
				Backend: utxoBackend,
				FlushBlockDB: func() error {
					return nil
				},
				MaxSize: 100 * 1024 * 1024, // 100 MiB
			}),
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
	if !errors.Is(err, ErrTxTooBig) {
		t.Fatalf("CheckTransactionSanity: unexpected error for transaction "+
			"that is too large -- got %v, want %v", err, ErrTxTooBig)
	}
}

// badBlock is an intentionally bad block that should fail the context-less
// sanity checks.
var badBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:      1,
		MerkleRoot:   *mustParseHash("66aa7491b9adce110585ccab7e3fb5fe280de174530cca10eba2c6c3df01c10d"),
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
	g := newChaingenHarness(t, params)

	// Define some additional convenience helper functions to process the
	// current tip block associated with the generator.
	//
	// acceptedBlockTemplate expected the block to considered a valid block
	// template.
	//
	// rejectedBlockTemplate expects the block to be considered an invalid
	// block template due to the provided error kind.
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
	rejectedBlockTemplate := func(kind ErrorKind) {
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

		// Ensure the error reject kind matches the value specified in the test
		// instance.
		if !errors.Is(err, kind) {
			t.Fatalf("block template %q (hash %s, height %d) does "+
				"not have expected reject code -- got %v, want %v",
				g.TipName(), block.Hash(), blockHeight, err, kind)
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

// TestCheckTicketExhaustion ensures the function which checks for inevitable
// ticket exhaustion works as intended with a variety of scenarios including
// various corner cases such as before, after, and straddling stake validation
// height.
func TestCheckTicketExhaustion(t *testing.T) {
	// Hardcoded values expected by the tests so they remain valid if network
	// parameters change.
	const (
		coinbaseMaturity      = 16
		ticketMaturity        = 16
		ticketsPerBlock       = 5
		stakeEnabledHeight    = coinbaseMaturity + ticketMaturity
		stakeValidationHeight = 144
	)

	// Create chain params based on regnet params with the specific values
	// overridden.
	params := chaincfg.RegNetParams()
	params.CoinbaseMaturity = coinbaseMaturity
	params.TicketMaturity = ticketMaturity
	params.TicketsPerBlock = ticketsPerBlock
	params.StakeEnabledHeight = stakeEnabledHeight
	params.StakeValidationHeight = stakeValidationHeight

	// ticketInfo is used to control the tests by specifying the details about
	// how many fake blocks to create with the specified number of tickets.
	type ticketInfo struct {
		numNodes uint32 // number of fake blocks to create
		tickets  uint8  // number of tickets to buy in each block
	}

	tests := []struct {
		name        string       // test description
		ticketInfo  []ticketInfo // num blocks and tickets to construct
		newBlockTix uint8        // num tickets in new block for check call
		err         error        // expected error
	}{{
		// Reach inevitable ticket exhaustion by not including any ticket
		// purchases up to and including the final possible block prior to svh
		// that can prevent it.
		name: "guaranteed exhaustion prior to svh",
		ticketInfo: []ticketInfo{
			{126, 0}, // height: 126, 0 live, 0 immature
		},
		newBlockTix: 0, // extending height: 126, 0 live, 0 immature
		err:         ErrTicketExhaustion,
	}, {
		// Reach inevitable ticket exhaustion by not including any ticket
		// purchases up to just before the final possible block prior to svh
		// that can prevent it and only include enough tickets in that final
		// block such that it is one short of the required amount.
		name: "one ticket short in final possible block prior to svh",
		ticketInfo: []ticketInfo{
			{126, 0}, // height: 126, 0 live, 0 immature
		},
		newBlockTix: 4, // extending height: 126, 0 live, 4 immature
		err:         ErrTicketExhaustion,
	}, {
		// Construct chain such that there are no ticket purchases up to just
		// before the final possible block prior to svh that can prevent ticket
		// exhaustion and that final block contains the exact amount of ticket
		// purchases required to prevent it.
		name: "just enough in final possible block prior to svh",
		ticketInfo: []ticketInfo{
			{126, 0}, // height: 126, 0 live, 0 immature
		},
		newBlockTix: 5, // extending height: 126, 0 live, 5 immature
		err:         nil,
	}, {
		// Reach inevitable ticket exhaustion with one live ticket less than
		// needed to prevent it at the first block which ticket exhaustion can
		// happen.
		name: "one ticket short with live tickets at 1st possible exhaustion",
		ticketInfo: []ticketInfo{
			{16, 0},  // height: 16, 0 live, 0 immature
			{1, 4},   // height: 17, 0 live, 4 immature
			{109, 0}, // height: 126, 4 live, 0 immature
		},
		newBlockTix: 0, // extending height: 126, 4 live, 0 immature
		err:         ErrTicketExhaustion,
	}, {
		name: "just enough live tickets at 1st possible exhaustion",
		ticketInfo: []ticketInfo{
			{16, 0},  // height: 16, 0 live, 0 immature
			{1, 5},   // height: 17, 0 live, 5 immature
			{109, 0}, // height: 126, 5 live, 0 immature
		},
		newBlockTix: 0, // extending height: 126, 5 live, 0 immature
		err:         nil,
	}, {
		// Reach inevitable ticket exhaustion in the second possible block that
		// it can happen.  Notice this means it consumes the exact number of
		// live tickets in the first block that ticket exhaustion can happen.
		name: "exhaustion at 2nd possible block, five tickets short",
		ticketInfo: []ticketInfo{
			{16, 0},  // height: 16, 0 live, 0 immature
			{1, 5},   // height: 17, 0 live, 5 immature
			{110, 0}, // height: 127, 5 live, 0 immature
		},
		newBlockTix: 0, // extending height: 127, 5 live, 0 immature
		err:         ErrTicketExhaustion,
	}, {
		// Reach inevitable ticket exhaustion in the second possible block that
		// it can happen with one live ticket less than needed to prevent it.
		name: "exhaustion at 2nd possible block, one ticket short",
		ticketInfo: []ticketInfo{
			{16, 0},  // height: 16, 0 live, 0 immature
			{1, 9},   // height: 17, 0 live, 9 immature
			{110, 0}, // height: 127, 9 live, 0 immature
		},
		newBlockTix: 0, // extending height: 127, 9 live, 0 immature
		err:         ErrTicketExhaustion,
	}, {
		// Construct chain to one block before svh such that there are exactly
		// enough live tickets to prevent exhaustion.
		name: "just enough to svh-1 with live tickets",
		ticketInfo: []ticketInfo{
			{36, 0},  // height: 36, 0 live, 0 immature
			{4, 20},  // height: 40, 0 live, 80 immature
			{1, 5},   // height: 41, 0 live, 85 immature
			{101, 0}, // height: 142, 85 live, 0 immature
		},
		newBlockTix: 0, // extending height: 142, 85 live, 0 immature
		err:         nil,
	}, {
		// Construct chain to one block before svh such that there is a mix of
		// live and immature tickets that sum to exactly enough prevent
		// exhaustion.
		name: "just enough to svh-1 with mix of live and immature tickets",
		ticketInfo: []ticketInfo{
			{36, 0},  // height: 36, 0 live, 0 immature
			{3, 20},  // height: 39, 0 live, 60 immature
			{1, 15},  // height: 40, 0 live, 75 immature
			{101, 0}, // height: 141, 75 live, 0 immature
			{1, 5},   // height: 142, 75 live, 5 immature
		},
		newBlockTix: 5, // extending height: 142, 75 live, 10 immature
		err:         nil,
	}, {
		// Construct chain to svh such that there are exactly enough live
		// tickets to prevent exhaustion.
		name: "just enough to svh with live tickets",
		ticketInfo: []ticketInfo{
			{32, 0},  // height: 32, 0 live, 0 immature
			{4, 20},  // height: 36, 0 live, 80 immature
			{1, 10},  // height: 37, 0 live, 90 immature
			{106, 0}, // height: 143, 90 live, 0 immature
		},
		newBlockTix: 0, // extending height: 143, 90 live, 0 immature
		err:         nil,
	}, {
		// Construct chain to svh such that there are exactly enough live
		// tickets just becoming mature to prevent exhaustion.
		name: "just enough to svh with maturing",
		ticketInfo: []ticketInfo{
			{126, 0}, // height: 126, 0 live, 0 immature
			{16, 5},  // height: 142, 0 live, 80 immature
			{1, 5},   // height: 143, 5 live, 80 immature
		},
		newBlockTix: 5, // extending height: 143, 5 live, 80 immature
		err:         nil,
	}, {
		// Reach inevitable ticket exhaustion after creating a large pool of
		// live tickets and allowing the live ticket pool to dwindle due to
		// votes without buying more any more tickets.
		name: "exhaustion due to dwindling live tickets w/o new purchases",
		ticketInfo: []ticketInfo{
			{126, 0}, // height: 126, 0 live, 0 immature
			{25, 20}, // height: 151, 140 live, 360 immature
			{75, 0},  // height: 226, 85 live, 0 immature
		},
		newBlockTix: 0, // extending height: 226, 85 live, 0 immature
		err:         ErrTicketExhaustion,
	}}

	for _, test := range tests {
		bc := newFakeChain(params)

		// immatureTickets tracks which height the purchased tickets will mature
		// and thus be eligible for admission to the live ticket pool.
		immatureTickets := make(map[int64]uint8)
		var poolSize uint32
		node := bc.bestChain.Tip()
		blockTime := time.Unix(node.timestamp, 0)
		for _, ticketInfo := range test.ticketInfo {
			for i := uint32(0); i < ticketInfo.numNodes; i++ {
				blockTime = blockTime.Add(time.Second)
				node = newFakeNode(node, 1, 1, 0, blockTime)
				node.poolSize = poolSize
				node.freshStake = ticketInfo.tickets

				// Update the pool size for the next header.  Notice how tickets
				// that mature for this block do not show up in the pool size
				// until the next block.  This is correct behavior.
				poolSize += uint32(immatureTickets[node.height])
				delete(immatureTickets, node.height)
				if node.height >= stakeValidationHeight {
					poolSize -= ticketsPerBlock
				}

				// Track maturity height for new ticket purchases.
				maturityHeight := node.height + ticketMaturity
				immatureTickets[maturityHeight] = ticketInfo.tickets

				// Add the new fake node to the block index and update the chain
				// to use it as the new best node.
				bc.index.AddNode(node)
				bc.bestChain.SetTip(node)

				// Ensure the test data does not have any invalid intermediate
				// states leading up to the final test condition.
				parentHash := &node.parent.hash
				err := bc.CheckTicketExhaustion(parentHash, ticketInfo.tickets)
				if err != nil {
					t.Errorf("%q: unexpected err: %v", test.name, err)
				}
			}
		}

		// Ensure the expected result is returned from ticket exhaustion check.
		err := bc.CheckTicketExhaustion(&node.hash, test.newBlockTix)
		if !errors.Is(err, test.err) {
			t.Errorf("%q: mismatched err -- got %v, want %v", test.name, err,
				test.err)
			continue
		}
	}
}

// TestExplicitVerUpgradesSemantics ensures that the various semantics enforced
// by the explicit version upgrades agenda behave as intended.
func TestExplicitVerUpgradesSemantics(t *testing.T) {
	// Use a set of test chain parameters which allow for quicker vote
	// activation as compared to various existing network params.
	params := quickVoteActivationParams()

	// Clone the parameters so they can be mutated, find the correct deployment
	// for the explicit version upgrade and ensure it is always available to
	// vote by removing the time constraints to prevent test failures when the
	// real expiration time passes.
	const voteID = chaincfg.VoteIDExplicitVersionUpgrades
	params = cloneParams(params)
	deploymentVer, deployment, err := findDeployment(params, voteID)
	if err != nil {
		t.Fatal(err)
	}
	removeDeploymentTimeConstraints(deployment)

	// Shorter versions of useful params for convenience.
	ticketsPerBlock := uint32(params.TicketsPerBlock)
	stakeValidationHeight := uint32(params.StakeValidationHeight)
	coinbaseMaturity := params.CoinbaseMaturity

	// Create a test harness initialized with the genesis block as the tip.
	g := newChaingenHarness(t, params)

	// The following funcs are convenience funcs for asserting the tests are
	// actually testing what they intend to.
	//
	// assertRegularBlockTxVer is a helper to assert that the version of the
	// transaction at the provided index of the regular transaction tree in the
	// given block is the expected version.
	//
	// assertStakeBlockTxVer is a helper to assert that the version of the
	// transaction at the provided index of the stake transaction tree in the
	// given block is the expected version.
	assertRegularBlockTxVer := func(blockName string, txIdx int, expected uint16) {
		t.Helper()

		tx := g.BlockByName(blockName).Transactions[txIdx]
		if tx.Version != expected {
			t.Fatalf("tx version for block %q tx idx %d is %d instead of "+
				"expected %d", blockName, txIdx, tx.Version, expected)
		}
	}
	assertStakeBlockTxVer := func(blockName string, txIdx int, expected uint16) {
		t.Helper()

		tx := g.BlockByName(blockName).STransactions[txIdx]
		if tx.Version != expected {
			t.Fatalf("tx version for block %q stake tx idx %d is %d instead "+
				"of expected %d", blockName, txIdx, tx.Version, expected)
		}
	}

	// splitRegSpendTx modifies the regular spend transaction created by the
	// harness to create several outputs by evenly splitting the original output
	// over multiple.
	splitRegSpendTx := func(b *wire.MsgBlock) {
		// Save the current outputs of the spend tx and clear them.
		tx := b.Transactions[1]
		origOut := tx.TxOut[0]
		origOpReturnOut := tx.TxOut[1]
		tx.TxOut = tx.TxOut[:0]

		// Evenly split the original output amount over multiple outputs.
		const numOutputs = 3
		amount := origOut.Value / numOutputs
		for i := 0; i < numOutputs; i++ {
			if i == numOutputs-1 {
				amount = origOut.Value - amount*(numOutputs-1)
			}
			tx.AddTxOut(wire.NewTxOut(amount, origOut.PkScript))
		}

		// Add the original op return back to the outputs.
		tx.AddTxOut(origOpReturnOut)
	}

	// -------------------------------------------------------------------------
	// Generate and accept enough blocks to reach one block prior to stake
	// validation height.
	// -------------------------------------------------------------------------

	g.AdvanceToHeight(stakeValidationHeight-1, ticketsPerBlock)

	// -------------------------------------------------------------------------
	// Create block at stake validation height that has both a regular and stake
	// transaction with a version allowed prior to the explicit version upgrades
	// agenda but not after it activates.  Also, set one of the outputs of
	// another regular transaction to a script version that is also allowed
	// prior to the explicit version upgrades agenda but not after it activates.
	//
	// The outputs are created now so they can be spent after the agenda
	// activates to ensure they remain spendable.
	//
	// The block should be accepted because the agenda is not active yet.
	//
	//   ... -> bsvh
	// -------------------------------------------------------------------------

	lowFee := dcrutil.Amount(1)
	outs := g.OldestCoinbaseOuts()
	g.NextBlock("bsvh", &outs[0], outs[1:], splitRegSpendTx, func(b *wire.MsgBlock) {
		// Create new transactions spending from the outputs of the regular
		// spend transaction.
		tx := b.Transactions[1]
		numOutputs := uint32(len(tx.TxOut) - 2)
		blkHeight := b.Header.Height
		for txOutIdx := uint32(0); txOutIdx < numOutputs; txOutIdx++ {
			spend := chaingen.MakeSpendableOutForTx(tx, blkHeight, 1, txOutIdx)
			txNew := g.CreateSpendTx(&spend, lowFee)
			b.AddTransaction(txNew)
		}

		// Set transaction versions to values that will no longer be valid for
		// new transactions after the explicit version upgrades agenda
		// activates.
		b.Transactions[2].Version = ^uint16(0)
		b.STransactions[3].Version = ^uint16(0)

		// Set output script version of a regular transaction output to a value
		// that will no longer be valid for new outputs after the explicit
		// version upgrades agenda activates.  Also, set the script to false
		// which ordinarily would make the output unspendable if the script
		// version were 0.
		b.Transactions[3].TxOut[0].Version = 1
		b.Transactions[3].TxOut[0].PkScript = []byte{txscript.OP_FALSE}
	})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()
	g.SnapshotCoinbaseOuts("postSVH")

	// -------------------------------------------------------------------------
	// Create block that has a stake transaction with a script version that is
	// not allowed regardless of the explicit version upgrades agenda.
	//
	// The block should be rejected because stake transaction script versions
	// are separately enforced and are required to be a specific version
	// independently of the consensus change.
	//
	//   ... -> bsvh
	//              \-> bbadvote
	// -------------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("bbadvote", &outs[0], outs[1:], func(b *wire.MsgBlock) {
		b.STransactions[4].TxOut[2].Version = 12345
	})
	g.RejectTipBlock(ErrBadTxInput)

	// -------------------------------------------------------------------------
	// Create enough blocks to allow the stake output created at stake
	// validation height to mature.
	//
	//   ... -> bsvh -> btmp0 -> ... -> btmp#
	// -------------------------------------------------------------------------

	g.SetTip("bsvh")
	for i := uint16(0); i < coinbaseMaturity; i++ {
		blockName := fmt.Sprintf("btmp%d", i)
		g.NextBlock(blockName, &outs[0], outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// -------------------------------------------------------------------------
	// Create block that spends the regular and stake transactions created above
	// with versions that will no longer be allowed for new transactions after
	// the explicit version upgrades agenda to ensure the utxos remain spendable
	// prior to the activation of the agenda.
	//
	// Note that this block and the temp blocks above will be undone later so
	// the same outputs can be spent after the explicit version upgrades agenda
	// is active as well.
	//
	//   ... -> btmp# -> bspend0
	// -------------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("bspend0", &outs[0], outs[1:], func(b *wire.MsgBlock) {
		const regSpendTxIdx = 2
		assertRegularBlockTxVer("bsvh", regSpendTxIdx, ^uint16(0))
		bsvh := g.BlockByName("bsvh")
		regSpend := chaingen.MakeSpendableOut(bsvh, regSpendTxIdx, 0)
		regSpendTx := g.CreateSpendTx(&regSpend, lowFee)
		b.AddTransaction(regSpendTx)

		const stakeSpendTxIdx = 3
		assertStakeBlockTxVer("bsvh", stakeSpendTxIdx, ^uint16(0))
		stakeSpend := chaingen.MakeSpendableStakeOut(bsvh, stakeSpendTxIdx, 2)
		stakeSpendTx := g.CreateSpendTx(&stakeSpend, lowFee)
		b.AddTransaction(stakeSpendTx)

		// Notice the signature script is nil because the version is unsupported
		// which means the scripts are never executed.
		const secondRegSpendTxIdx = 3
		regSpend = chaingen.MakeSpendableOut(bsvh, secondRegSpendTxIdx, 0)
		regSpendTx = g.CreateSpendTx(&regSpend, lowFee)
		regSpendTx.TxIn[0].SignatureScript = nil
		b.AddTransaction(regSpendTx)

	})
	g.AcceptTipBlock()

	// -------------------------------------------------------------------------
	// Set the harness tip back to stake validation height, invalidate the
	// blocks above so the underlying chain is also set back to that svh, and
	// then activate the explicit version upgrades agenda.
	// -------------------------------------------------------------------------

	g.SetTip("bsvh")
	g.InvalidateBlockAndExpectTip("btmp0", nil, "bsvh")
	g.RestoreCoinbaseOutsSnapshot("postSVH")
	g.AdvanceFromSVHToActiveAgendas(voteID)

	// replaceVers is a munge function which modifies the provided block by
	// replacing the block, stake, and vote versions with the explicit version
	// deployment version.
	replaceVers := func(b *wire.MsgBlock) {
		chaingen.ReplaceBlockVersion(int32(deploymentVer))(b)
		chaingen.ReplaceStakeVersion(deploymentVer)(b)
		chaingen.ReplaceVoteVersions(deploymentVer)(b)
	}

	// -------------------------------------------------------------------------
	// Assert the stake output at stake validation height has reached maturity
	// and is therefore spendable as a result of advancing to activate the
	// agenda.
	// -------------------------------------------------------------------------

	svhTxnsMaturityHeight := stakeValidationHeight + uint32(coinbaseMaturity)
	if g.Tip().Header.Height < svhTxnsMaturityHeight {
		t.Fatalf("stake output requires height %d to mature, but current tip "+
			"is block %q (height %d)", svhTxnsMaturityHeight, g.TipName(),
			g.Tip().Header.Height)
	}

	// -------------------------------------------------------------------------
	// Create a stable base for the remaining tests.
	//
	// ... -> bbase
	// -------------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("bbase", &outs[0], outs[1:], replaceVers)
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	// -------------------------------------------------------------------------
	// Create block that has a regular transaction with a version that is no
	// longer allowed for new transactions after the explicit version upgrades
	// agenda is active.
	//
	// The block should be rejected because the agenda is active.
	//
	//   ... -> bbase
	//                \-> b0bad
	// -------------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b0bad", &outs[0], outs[1:], replaceVers, func(b *wire.MsgBlock) {
		b.Transactions[1].Version = ^uint16(0)
	})
	g.RejectTipBlock(ErrTxVersionTooHigh)

	// -------------------------------------------------------------------------
	// Create block that has a stake transaction with a version that is no
	// longer allowed for new transactions after the explicit version upgrades
	// agenda is active.
	//
	// The block should be rejected because the agenda is active.
	//
	//   ... -> bbase
	//                \-> b0bad2
	// -------------------------------------------------------------------------

	g.SetTip("bbase")
	g.NextBlock("b0bad2", &outs[0], outs[1:], replaceVers, func(b *wire.MsgBlock) {
		b.STransactions[3].Version = ^uint16(0)
	})
	g.RejectTipBlock(ErrTxVersionTooHigh)

	// -------------------------------------------------------------------------
	// Create block that has a regular transaction with an output that has a
	// script version that is no longer allowed for new transactions after the
	// explicit version upgrades agenda is active.
	//
	// The block should be rejected because the agenda is active.
	//
	//   ... -> bbase
	//                \-> b0bad3
	// -------------------------------------------------------------------------

	g.SetTip("bbase")
	g.NextBlock("b0bad3", &outs[0], outs[1:], replaceVers, func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Version = ^uint16(0)
	})
	g.RejectTipBlock(ErrScriptVersionTooHigh)

	// -------------------------------------------------------------------------
	// Create block that spends both the regular and stake transactions created
	// with a version that is no longer allowed for new transactions after the
	// explicit version upgrades agenda is active but already existed prior to
	// its activation.
	//
	// The block should be accepted because all existing utxos must remain
	// spendable after the agenda activates.
	//
	//   ... -> bbase -> b0
	// -------------------------------------------------------------------------

	g.SetTip("bbase")
	g.NextBlock("b0", &outs[0], outs[1:], replaceVers, func(b *wire.MsgBlock) {
		const regSpendTxIdx = 2
		assertRegularBlockTxVer("bsvh", regSpendTxIdx, ^uint16(0))
		bsvh := g.BlockByName("bsvh")
		regSpend := chaingen.MakeSpendableOut(bsvh, regSpendTxIdx, 0)
		regSpendTx := g.CreateSpendTx(&regSpend, lowFee)
		b.AddTransaction(regSpendTx)

		const stakeSpendTxIdx = 3
		assertStakeBlockTxVer("bsvh", stakeSpendTxIdx, ^uint16(0))
		stakeSpend := chaingen.MakeSpendableStakeOut(bsvh, stakeSpendTxIdx, 2)
		stakeSpendTx := g.CreateSpendTx(&stakeSpend, lowFee)
		b.AddTransaction(stakeSpendTx)
	})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	// -------------------------------------------------------------------------
	// Create block that spends the regular transaction output created with a
	// script version that is no longer allowed for new outputs after the
	// explicit version upgrades agenda is active but already existed prior to
	// its activation.
	//
	// The block should be accepted because all existing utxos must remain
	// spendable after the agenda activates.
	//
	//   ... -> bbase -> b0 -> b1
	// -------------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b1", &outs[0], outs[1:], replaceVers, func(b *wire.MsgBlock) {
		// Notice the output is still spendable even though the signature script
		// is nil because the version is unsupported which means the scripts are
		// never executed.
		const regSpendTxIdx = 3
		bsvh := g.BlockByName("bsvh")
		regSpend := chaingen.MakeSpendableOut(bsvh, regSpendTxIdx, 0)
		regSpendTx := g.CreateSpendTx(&regSpend, lowFee)
		regSpendTx.TxIn[0].SignatureScript = nil
		b.AddTransaction(regSpendTx)
	})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

}

// TestCalcTicketReturnAmounts ensures that ticket return amounts are calculated
// correctly under a variety of conditions.
func TestCalcTicketReturnAmounts(t *testing.T) {
	t.Parallel()

	// Default header bytes for tests.
	prevHeaderBytes, _ := hex.DecodeString("07000000dc02335daa073d293e1b15064" +
		"8f0444a60b9c97604abd01e00000000000000003c449b2321c4bd0d1fa76ed59f80e" +
		"baf46f16cfb2d17ba46948f09f21861095566482410a463ed49473c27278cd7a2a37" +
		"12a3b19ff1f6225717d3eb71cc2b5590100012c7312a3c30500050095a100000cf42" +
		"418f1820a870300000020a10700091600005b32a55f5bcce31078832100007469943" +
		"958002e000000000000000000000000000000000000000007000000")

	// Default ticket commitment script hex for tests.
	p2pkhCommitScriptHex := "ba76a914097e847d49c6806f6933e806a350f43b97ac70d088ac"

	// createTestTicketOuts is a helper function that creates mock minimal outputs
	// for a ticket with the given contribution amounts.  Note that this only
	// populates the ticket commitment outputs since those are the only outputs
	// used to calculate ticket return amounts.
	createTestTicketOuts := func(contribAmounts []int64) []*stake.MinimalOutput {
		ticketOuts := make([]*stake.MinimalOutput, len(contribAmounts)*2+1)
		for i := 0; i < len(contribAmounts); i++ {
			commitScript := hexToBytes(p2pkhCommitScriptHex)
			amtBytes := commitScript[commitAmountStartIdx:commitAmountEndIdx]
			binary.LittleEndian.PutUint64(amtBytes, uint64(contribAmounts[i]))
			ticketOuts[i*2+1] = &stake.MinimalOutput{PkScript: commitScript}
		}
		return ticketOuts
	}

	tests := []struct {
		name                     string
		contribAmounts           []int64
		ticketPurchaseAmount     int64
		voteSubsidy              int64
		prevHeaderBytes          []byte
		isVote                   bool
		isAutoRevocationsEnabled bool
		want                     []int64
	}{{
		name: "vote rewards - evenly divisible over all outputs (auto " +
			"revocations disabled)",
		contribAmounts: []int64{
			2500000000,
			2500000000,
			5000000000,
			10000000000,
		},
		ticketPurchaseAmount: 20000000000,
		voteSubsidy:          100000000,
		isVote:               true,
		want: []int64{
			2512500000,
			2512500000,
			5025000000,
			10050000000,
		},
	}, {
		name: "revocation rewards - evenly divisible over all outputs (auto " +
			"revocations disabled)",
		contribAmounts: []int64{
			2500000000,
			2500000000,
			5000000000,
			10000000000,
		},
		ticketPurchaseAmount: 20000000000,
		voteSubsidy:          0,
		want: []int64{
			2500000000,
			2500000000,
			5000000000,
			10000000000,
		},
	}, {
		name: "vote rewards - remainder of 2 (auto revocations disabled)",
		contribAmounts: []int64{
			100000000,
			100000000,
			100000000,
		},
		ticketPurchaseAmount: 300000000,
		voteSubsidy:          300002,
		isVote:               true,
		want: []int64{
			100100000,
			100100000,
			100100000,
		},
	}, {
		name: "revocation rewards - remainder of 4 (auto revocations disabled)",
		contribAmounts: []int64{
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
		},
		ticketPurchaseAmount: 799999996,
		voteSubsidy:          0,
		want: []int64{
			99999999,
			99999999,
			99999999,
			99999999,
			99999999,
			99999999,
			99999999,
			99999999,
		},
	}, {
		name: "vote rewards - evenly divisible over all outputs (auto " +
			"revocations enabled)",
		contribAmounts: []int64{
			2500000000,
			2500000000,
			5000000000,
			10000000000,
		},
		ticketPurchaseAmount:     20000000000,
		voteSubsidy:              100000000,
		prevHeaderBytes:          prevHeaderBytes,
		isVote:                   true,
		isAutoRevocationsEnabled: true,
		want: []int64{
			2512500000,
			2512500000,
			5025000000,
			10050000000,
		},
	}, {
		name: "revocation rewards - evenly divisible over all outputs (auto " +
			"revocations enabled)",
		contribAmounts: []int64{
			2500000000,
			2500000000,
			5000000000,
			10000000000,
		},
		ticketPurchaseAmount:     20000000000,
		voteSubsidy:              0,
		prevHeaderBytes:          prevHeaderBytes,
		isAutoRevocationsEnabled: true,
		want: []int64{
			2500000000,
			2500000000,
			5000000000,
			10000000000,
		},
	}, {
		name: "vote rewards - remainder of 2 (auto revocations enabled)",
		contribAmounts: []int64{
			100000000,
			100000000,
			100000000,
			100000000,
		},
		ticketPurchaseAmount:     400000000,
		voteSubsidy:              400002,
		prevHeaderBytes:          prevHeaderBytes,
		isVote:                   true,
		isAutoRevocationsEnabled: true,
		want: []int64{
			100100000,
			100100000,
			100100000,
			100100000,
		},
	}, {
		name: "revocation rewards - remainder of 4 (auto revocations enabled)",
		contribAmounts: []int64{
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
		},
		ticketPurchaseAmount:     799999996,
		voteSubsidy:              0,
		prevHeaderBytes:          prevHeaderBytes,
		isAutoRevocationsEnabled: true,
		want: []int64{
			99999999,
			100000000,
			99999999,
			99999999,
			99999999,
			99999999,
			100000001,
			100000000,
		},
	}}

	for _, test := range tests {
		got := calcTicketReturnAmounts(createTestTicketOuts(test.contribAmounts),
			test.ticketPurchaseAmount, test.voteSubsidy, test.prevHeaderBytes,
			test.isVote, test.isAutoRevocationsEnabled)
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%q: unexpected result -- got %v, want %v", test.name, got,
				test.want)
		}
	}
}

// TestCheckTicketRedeemers ensures that all ticket redeemer consensus rule
// checks work as expected.
func TestCheckTicketRedeemers(t *testing.T) {
	t.Parallel()

	// Generate a slice of ticket hashes to use for tests.
	const numTicketHashes = 9
	testTicketHashes := make([]chainhash.Hash, numTicketHashes)
	for i := 0; i < numTicketHashes; i++ {
		var randHash chainhash.Hash
		if _, err := mrand.Read(randHash[:]); err != nil {
			t.Fatalf("error reading random hash: %v", err)
		}
		testTicketHashes[i] = randHash
	}

	// Create a default exists missed ticket function.
	defaultExistsMissedTicket := func(ticket chainhash.Hash) bool {
		return false
	}

	tests := []struct {
		name                     string
		voteTicketHashes         []chainhash.Hash
		revocationTicketHashes   []chainhash.Hash
		winners                  []chainhash.Hash
		expiringNextBlock        []chainhash.Hash
		existsMissedTicket       func(ticket chainhash.Hash) bool
		isTreasuryEnabled        bool
		isAutoRevocationsEnabled bool
		wantErr                  error
	}{{
		name:             "ok with no revocations (auto revocations disabled)",
		voteTicketHashes: testTicketHashes[:5],
		winners:          testTicketHashes[:5],
	}, {
		name: "ok with revocations for previously missed tickets (auto " +
			"revocations disabled)",
		voteTicketHashes:       testTicketHashes[:5],
		revocationTicketHashes: testTicketHashes[5:6],
		winners:                testTicketHashes[:5],
		existsMissedTicket: func(ticket chainhash.Hash) bool {
			return ticket == testTicketHashes[5]
		},
	}, {
		name: "ok block does not contain a revocation for ticket that is " +
			"becoming missed as of this block (auto revocations disabled)",
		voteTicketHashes: testTicketHashes[:4],
		winners:          testTicketHashes[:5],
	}, {
		name: "ok block does not contain a revocation for ticket that is " +
			"becoming expired as of this block (auto revocations disabled)",
		voteTicketHashes:  testTicketHashes[:5],
		winners:           testTicketHashes[:5],
		expiringNextBlock: testTicketHashes[5:6],
	}, {
		name: "revocations not allowed for tickets missed this block (auto " +
			"revocations disabled)",
		voteTicketHashes:       testTicketHashes[:4],
		revocationTicketHashes: testTicketHashes[4:5],
		winners:                testTicketHashes[:5],
		wantErr:                ErrInvalidSSRtx,
	}, {
		name: "revocations not allowed for tickets expired this block (auto " +
			"revocations disabled)",
		voteTicketHashes:       testTicketHashes[:5],
		revocationTicketHashes: testTicketHashes[5:6],
		winners:                testTicketHashes[:5],
		expiringNextBlock:      testTicketHashes[5:6],
		wantErr:                ErrInvalidSSRtx,
	}, {
		name: "block contains vote for ineligible ticket (auto revocations " +
			"disabled)",
		voteTicketHashes: testTicketHashes[:5],
		winners:          testTicketHashes[1:6],
		wantErr:          ErrTicketUnavailable,
	}, {
		name: "block contains revocation of ineligible ticket (auto " +
			"revocations disabled)",
		voteTicketHashes:       testTicketHashes[:5],
		revocationTicketHashes: testTicketHashes[5:6],
		winners:                testTicketHashes[:5],
		wantErr:                ErrInvalidSSRtx,
	}, {
		name: "ok with no revocations (auto revocations " +
			"enabled)",
		voteTicketHashes:         testTicketHashes[:5],
		winners:                  testTicketHashes[:5],
		isAutoRevocationsEnabled: true,
	}, {
		name: "ok with revocations for previously missed tickets (auto " +
			"revocations enabled)",
		voteTicketHashes:       testTicketHashes[:5],
		revocationTicketHashes: testTicketHashes[5:6],
		winners:                testTicketHashes[:5],
		existsMissedTicket: func(ticket chainhash.Hash) bool {
			return ticket == testTicketHashes[5]
		},
		isAutoRevocationsEnabled: true,
	}, {
		name: "ok with revocations for tickets missed this block (auto " +
			"revocations enabled)",
		voteTicketHashes:         testTicketHashes[:4],
		revocationTicketHashes:   testTicketHashes[4:5],
		winners:                  testTicketHashes[:5],
		isAutoRevocationsEnabled: true,
	}, {
		name: "ok with revocations for tickets expired this block (auto " +
			"revocations enabled)",
		voteTicketHashes:         testTicketHashes[:5],
		revocationTicketHashes:   testTicketHashes[5:6],
		winners:                  testTicketHashes[:5],
		expiringNextBlock:        testTicketHashes[5:6],
		isAutoRevocationsEnabled: true,
	}, {
		name: "ok with revocations for tickets missed this block and tickets " +
			"expired this block (auto revocations enabled)",
		voteTicketHashes:         testTicketHashes[:4],
		revocationTicketHashes:   testTicketHashes[4:6],
		winners:                  testTicketHashes[:5],
		expiringNextBlock:        testTicketHashes[5:6],
		isAutoRevocationsEnabled: true,
	}, {
		name: "ok with revocations for previously missed tickets, tickets " +
			"missed this block, and tickets expired this block (auto " +
			"revocations enabled)",
		voteTicketHashes:       testTicketHashes[:3],
		revocationTicketHashes: testTicketHashes[3:9],
		winners:                testTicketHashes[:5],
		expiringNextBlock:      testTicketHashes[5:7],
		existsMissedTicket: func(ticket chainhash.Hash) bool {
			return ticket == testTicketHashes[7] || ticket == testTicketHashes[8]
		},
		isAutoRevocationsEnabled: true,
	}, {
		name: "block contains vote for ineligible ticket (auto revocations " +
			"enabled)",
		voteTicketHashes:         testTicketHashes[:5],
		winners:                  testTicketHashes[1:6],
		isAutoRevocationsEnabled: true,
		wantErr:                  ErrTicketUnavailable,
	}, {
		name: "block contains revocation of ineligible ticket (auto " +
			"revocations enabled)",
		voteTicketHashes:         testTicketHashes[:5],
		revocationTicketHashes:   testTicketHashes[5:6],
		winners:                  testTicketHashes[:5],
		isAutoRevocationsEnabled: true,
		wantErr:                  ErrInvalidSSRtx,
	}, {
		name: "block does not contain a revocation for ticket that is " +
			"becoming missed as of this block (auto revocations enabled)",
		voteTicketHashes:         testTicketHashes[:4],
		winners:                  testTicketHashes[:5],
		isAutoRevocationsEnabled: true,
		wantErr:                  ErrNoMissedTicketRevocation,
	}, {
		name: "block does not contain a revocation for ticket that is " +
			"becoming expired as of this block (auto revocations enabled)",
		voteTicketHashes:         testTicketHashes[:5],
		winners:                  testTicketHashes[:5],
		expiringNextBlock:        testTicketHashes[5:6],
		isAutoRevocationsEnabled: true,
		wantErr:                  ErrNoExpiredTicketRevocation,
	}}

	for _, test := range tests {
		// Use the default exists missed ticket function unless its overridden
		// by the test.
		existsMissedTicketFunc := defaultExistsMissedTicket
		if test.existsMissedTicket != nil {
			existsMissedTicketFunc = test.existsMissedTicket
		}

		// Run ticket redeemer validation rules.
		err := checkTicketRedeemers(test.voteTicketHashes,
			test.revocationTicketHashes, test.winners, test.expiringNextBlock,
			existsMissedTicketFunc, test.isTreasuryEnabled,
			test.isAutoRevocationsEnabled)

		// Validate that the expected error was returned for negative tests.
		if test.wantErr != nil {
			if !errors.Is(err, test.wantErr) {
				t.Errorf("%q: mismatched error -- got %T, want %T", test.name,
					err, test.wantErr)
			}
			continue
		}

		// Validate that an unexpected error was not returned.
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", test.name, err)
		}
	}
}

// TestAutoRevocations ensures that all of the validation rules associated with
// the automatic ticket revocations agenda work as expected.
func TestAutoRevocations(t *testing.T) {
	// Use a set of test chain parameters which allow for quicker vote
	// activation as compared to various existing network params.
	params := quickVoteActivationParams()

	// Clone the parameters so they can be mutated, find the correct
	// deployment for the automatic ticket revocations agenda, and, finally,
	// ensure it is always available to vote by removing the time constraints to
	// prevent test failures when the real expiration time passes.
	const autoRevocationsEnabled = true
	const voteID = chaincfg.VoteIDAutoRevocations
	params = cloneParams(params)
	version, deployment, err := findDeployment(params, voteID)
	if err != nil {
		t.Fatal(err)
	}
	removeDeploymentTimeConstraints(deployment)

	// Shorter versions of useful params for convenience.
	coinbaseMaturity := params.CoinbaseMaturity
	stakeValidationHeight := params.StakeValidationHeight
	ruleChangeInterval := int64(params.RuleChangeActivationInterval)

	// Create a test harness initialized with the genesis block as the tip.
	g := newChaingenHarness(t, params)

	// replaceAutoRevocationsVersions is a munge function which modifies the
	// provided block by replacing the block, stake, vote, and revocation
	// transaction versions with the versions associated with the automatic
	// ticket revocations deployment.
	replaceAutoRevocationsVersions := func(b *wire.MsgBlock) {
		chaingen.ReplaceBlockVersion(int32(version))(b)
		chaingen.ReplaceStakeVersion(version)(b)
		chaingen.ReplaceVoteVersions(version)(b)
		chaingen.ReplaceRevocationVersions(stake.TxVersionAutoRevocations)(b)
	}

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks with the appropriate vote bits set to
	// reach one block prior to the automatic ticket revocations agenda becoming
	// active.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()
	g.AdvanceFromSVHToActiveAgendas(voteID)
	activeAgendaHeight := uint32(stakeValidationHeight + ruleChangeInterval*3 - 1)
	g.AssertTipHeight(activeAgendaHeight)

	// Ensure the automatic ticket revocations agenda is active.
	tipHash := &g.chain.BestSnapshot().Hash
	gotActive, err := g.chain.IsAutoRevocationsAgendaActive(tipHash)
	if err != nil {
		t.Fatalf("error checking auto revocations agenda status: %v", err)
	}
	if !gotActive {
		t.Fatal("expected auto revocations agenda to be active")
	}

	// ---------------------------------------------------------------------
	// Generate enough blocks to have a known distance to the first mature
	// coinbase outputs for all tests that follow.  These blocks continue to
	// purchase tickets to avoid running out of votes.
	//
	//   ... -> bsv# -> bbm0 -> bbm1 -> ... -> bbm#
	// ---------------------------------------------------------------------

	for i := uint16(0); i < coinbaseMaturity; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bbm%d", i)
		g.NextBlock(blockName, nil, outs[1:], replaceAutoRevocationsVersions)
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
	}
	g.AssertTipHeight(activeAgendaHeight + uint32(coinbaseMaturity))

	// Collect spendable outputs into two different slices.  The outs slice is
	// intended to be used for regular transactions that spend from the output,
	// while the ticketOuts slice is intended to be used for stake ticket
	// purchases.
	var outs []*chaingen.SpendableOut
	var ticketOuts [][]chaingen.SpendableOut
	for i := uint16(0); i < coinbaseMaturity; i++ {
		coinbaseOuts := g.OldestCoinbaseOuts()
		outs = append(outs, &coinbaseOuts[0])
		ticketOuts = append(ticketOuts, coinbaseOuts[1:])
	}

	// Create a block that misses a vote and does not contain a revocation for
	// that missed vote.
	//
	//   ...
	//      \-> b1(0)
	startTip := g.TipName()
	g.NextBlock("b1", outs[0], ticketOuts[0], g.ReplaceWithNVotes(4),
		replaceAutoRevocationsVersions)
	g.AssertTipNumRevocations(0)
	g.RejectTipBlock(ErrNoMissedTicketRevocation)

	// Create a block that misses a vote and contains a version 1 revocation
	// transaction.
	//
	//   ...
	//      \-> b2(0)
	g.SetTip(startTip)
	g.NextBlock("b2", outs[0], ticketOuts[0], g.ReplaceWithNVotes(4),
		g.CreateRevocationsForMissedTickets(), replaceAutoRevocationsVersions,
		chaingen.ReplaceRevocationVersions(1))
	g.AssertTipNumRevocations(1)
	g.RejectTipBlock(ErrInvalidRevocationTxVersion)

	// Create a block that misses a vote and contains a revocation with a
	// non-zero fee.
	//
	//   ...
	//      \-> b3(0)
	g.SetTip(startTip)
	g.NextBlock("b3", outs[0], ticketOuts[0], g.ReplaceWithNVotes(4),
		g.CreateRevocationsForMissedTickets(), replaceAutoRevocationsVersions,
		func(b *wire.MsgBlock) {
			for _, stx := range b.STransactions {
				if !stake.IsSSRtx(stx, autoRevocationsEnabled) {
					continue
				}

				// Decrement the first output value to create a non-zero fee and
				// return so that only a single revocation transaction is
				// modified.
				stx.TxOut[0].Value--
				return
			}
		})
	g.AssertTipNumRevocations(1)
	// Note that this will fail with ErrRegTxCreateStakeOut rather than hitting
	// the later error case of ErrBadPayeeValue since a revocation with a
	// non-zero fee will not be identified as a revocation if the automatic
	// ticket revocations agenda is active.
	g.RejectTipBlock(ErrRegTxCreateStakeOut)

	// Create a valid block that misses multiple votes and contains revocation
	// transactions for those votes.
	//
	//   ... -> b4(0)
	g.SetTip(startTip)
	g.NextBlock("b4", outs[0], ticketOuts[0], g.ReplaceWithNVotes(3),
		g.CreateRevocationsForMissedTickets(), replaceAutoRevocationsVersions)
	g.AssertTipNumRevocations(2)
	g.AcceptTipBlock()

	// Create a slice of the ticket hashes that revocations spent in the tip
	// block that was just connected.
	revocationTicketHashes := make([]chainhash.Hash, 0, params.TicketsPerBlock)
	for _, stx := range g.Tip().STransactions {
		// Append revocation ticket hashes.
		if stake.IsSSRtx(stx, autoRevocationsEnabled) {
			ticketHash := stx.TxIn[0].PreviousOutPoint.Hash
			revocationTicketHashes = append(revocationTicketHashes, ticketHash)

			continue
		}
	}

	// Validate that the revocations are now in the revoked ticket treap in the
	// ticket database.
	tipHash = &g.chain.BestSnapshot().Hash
	blockNode := g.chain.index.LookupNode(tipHash)
	stakeNode, err := g.chain.fetchStakeNode(blockNode)
	if err != nil {
		t.Fatalf("error fetching stake node: %v", err)
	}
	for _, revocationTicketHash := range revocationTicketHashes {
		if !stakeNode.ExistsRevokedTicket(revocationTicketHash) {
			t.Fatalf("expected ticket %v to exist in the revoked ticket treap",
				revocationTicketHash)
		}
	}

	// Invalidate the previously connected block so that it is disconnected.
	g.InvalidateBlockAndExpectTip("b4", nil, startTip)

	// Validate that the revocations from the disconnected block are now back in
	// the live ticket treap in the ticket database.
	tipHash = &g.chain.BestSnapshot().Hash
	blockNode = g.chain.index.LookupNode(tipHash)
	stakeNode, err = g.chain.fetchStakeNode(blockNode)
	if err != nil {
		t.Fatalf("error fetching stake node: %v", err)
	}
	for _, revocationTicketHash := range revocationTicketHashes {
		if !stakeNode.ExistsLiveTicket(revocationTicketHash) {
			t.Fatalf("expected ticket %v to exist in the live ticket treap",
				revocationTicketHash)
		}
	}
}

// TestModifiedSubsidySplitSemantics ensures that the various semantics enforced
// by the modified subsidy split agenda behave as intended.
func TestModifiedSubsidySplitSemantics(t *testing.T) {
	// Use a set of test chain parameters which allow for quicker vote
	// activation as compared to various existing network params.
	params := quickVoteActivationParams()

	// Clone the parameters so they can be mutated, find the correct deployment
	// for the explicit version upgrade and ensure it is always available to
	// vote by removing the time constraints to prevent test failures when the
	// real expiration time passes.
	const voteID = chaincfg.VoteIDChangeSubsidySplit
	params = cloneParams(params)
	deploymentVer, deployment, err := findDeployment(params, voteID)
	if err != nil {
		t.Fatal(err)
	}
	removeDeploymentTimeConstraints(deployment)

	// Create a test harness initialized with the genesis block as the tip.
	g := newChaingenHarness(t, params)

	// replaceCoinbaseSubsidy is a munge function which modifies the provided
	// block by replacing the coinbase subsidy with the proportion required for
	// the modified subsidy split agenda.
	replaceCoinbaseSubsidy := func(b *wire.MsgBlock) {
		cache := g.chain.subsidyCache

		// Calculate the modified pow subsidy along with the treasury subsidy.
		const numVotes = 5
		const withDCP0010 = true
		height := int64(b.Header.Height)
		trsySubsidy := cache.CalcTreasurySubsidy(height, numVotes, noTreasury)
		powSubsidy := cache.CalcWorkSubsidyV2(height, numVotes, withDCP0010)

		// Update the input value to the the new expected subsidy sum.
		coinbaseTx := b.Transactions[0]
		coinbaseTx.TxIn[0].ValueIn = trsySubsidy + powSubsidy

		// Evenly split the modified pow subsidy over the relevant outputs.
		powOutputs := coinbaseTx.TxOut[2:]
		numPoWOutputs := int64(len(powOutputs))
		amount := powSubsidy / numPoWOutputs
		for i := int64(0); i < numPoWOutputs; i++ {
			if i == numPoWOutputs-1 {
				amount = powSubsidy - amount*(numPoWOutputs-1)
			}
			powOutputs[i].Value = amount
		}
	}

	// replaceVoteSubsidies is a munge function which modifies the provided
	// block by replacing all vote subsidies with the proportion required for
	// the modified subsidy split agenda.
	replaceVoteSubsidies := func(b *wire.MsgBlock) {
		cache := g.chain.subsidyCache

		// Calculate the modified vote subsidy and update all of the votes
		// accordingly.
		const withDCP0010 = true
		height := int64(b.Header.Height)
		voteSubsidy := cache.CalcStakeVoteSubsidyV2(height, withDCP0010)
		chaingen.ReplaceVoteSubsidies(dcrutil.Amount(voteSubsidy))(b)
	}

	// -------------------------------------------------------------------------
	// Generate and accept enough blocks with the appropriate vote bits set to
	// reach one block prior to the modified subsidy split agenda becoming
	// active.
	//
	// Note that this also ensures the subsidy split prior to the activation of
	// the agenda remains unaffected.
	// -------------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// -------------------------------------------------------------------------
	// Create a block that pays the modified work subsidy prior to activation
	// of the modified subsidy split agenda.
	//
	// The block should be rejected because the agenda is NOT active.
	//
	// ...
	//    \-> bsvhbad
	// -------------------------------------------------------------------------

	tipName := g.TipName()
	outs := g.OldestCoinbaseOuts()
	g.NextBlock("bsvhbad", &outs[0], outs[1:], replaceCoinbaseSubsidy)
	g.RejectTipBlock(ErrBadCoinbaseAmountIn)

	// -------------------------------------------------------------------------
	// Create a block that pays the modified vote subsidy prior to activation
	// of the modified subsidy split agenda.
	//
	// The block should be rejected because the agenda is NOT active.
	//
	// ...
	//    \-> bsvhbad2
	// -------------------------------------------------------------------------

	g.SetTip(tipName)
	g.NextBlock("bsvhbad2", &outs[0], outs[1:], replaceVoteSubsidies)
	g.RejectTipBlock(ErrBadStakebaseAmountIn)

	// -------------------------------------------------------------------------
	// Generate and accept enough blocks with the appropriate vote bits set to
	// reach one block prior to the modified subsidy split agenda becoming
	// active.
	// -------------------------------------------------------------------------

	g.SetTip(tipName)
	g.AdvanceFromSVHToActiveAgendas(voteID)

	// replaceVers is a munge function which modifies the provided block by
	// replacing the block, stake, and vote versions with the modified subsidy
	// split deployment version.
	replaceVers := func(b *wire.MsgBlock) {
		chaingen.ReplaceBlockVersion(int32(deploymentVer))(b)
		chaingen.ReplaceStakeVersion(deploymentVer)(b)
		chaingen.ReplaceVoteVersions(deploymentVer)(b)
	}

	// -------------------------------------------------------------------------
	// Create a block that pays the original vote subsidy that was in effect
	// prior to activation of the modified subsidy split agenda.
	//
	// The block should be rejected because the agenda is active.
	//
	// ...
	//    \-> b1bad
	// -------------------------------------------------------------------------

	tipName = g.TipName()
	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b1bad", &outs[0], outs[1:], replaceVers, replaceCoinbaseSubsidy)
	g.RejectTipBlock(ErrBadStakebaseAmountIn)

	// -------------------------------------------------------------------------
	// Create a block that pays the original work subsidy that was in effect
	// prior to activation of the modified subsidy split agenda.
	//
	// The block should be rejected because the agenda is active.
	//
	// ...
	//    \-> b1bad2
	// -------------------------------------------------------------------------

	g.SetTip(tipName)
	g.NextBlock("b1bad2", &outs[0], outs[1:], replaceVers, replaceVoteSubsidies)
	g.RejectTipBlock(ErrBadCoinbaseAmountIn)

	// -------------------------------------------------------------------------
	// Create a block that pays the modified work and vote subsidies.
	//
	// The block should be accepted because the agenda is active.
	//
	// ... -> b1
	// -------------------------------------------------------------------------

	g.SetTip(tipName)
	g.NextBlock("b1", &outs[0], outs[1:], replaceVers, replaceCoinbaseSubsidy,
		replaceVoteSubsidies)
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()
}
