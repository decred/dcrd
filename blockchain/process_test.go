// Copyright (c) 2019-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"sync"
	"testing"

	"github.com/decred/dcrd/blockchain/v4/chaingen"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/wire"
)

// These variables are used to provide a shared set of generated blocks that
// are only generated one time on demand.
var (
	processTestGeneratorLock sync.Mutex
	processTestGenerator     *chaingen.Generator
)

// TestProcessOrder ensures processing-specific logic such as orphan handling,
// duplicate block handling, and out-of-order reorgs to invalid blocks works as
// expected.
func TestProcessOrder(t *testing.T) {
	// Create a test harness initialized with the genesis block as the tip.
	params := chaincfg.RegNetParams()
	g, teardownFunc := newChaingenHarness(t, params, "processordertest")
	defer teardownFunc()

	// Shorter versions of useful params for convenience.
	coinbaseMaturity := params.CoinbaseMaturity
	stakeValidationHeight := params.StakeValidationHeight

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// ---------------------------------------------------------------------
	// Generate enough blocks to have a known distance to the first mature
	// coinbase outputs for all tests that follow.  These blocks continue
	// to purchase tickets to avoid running out of votes.
	//
	//   ... -> bsv# -> bbm0 -> bbm1 -> ... -> bbm#
	// ---------------------------------------------------------------------

	for i := uint16(0); i < coinbaseMaturity; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bbm%d", i)
		g.NextBlock(blockName, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
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

	// Ensure duplicate blocks are rejected.
	//
	//   ... -> b1(0)
	//      \-> b1(0)
	g.NextBlock("b1", outs[0], ticketOuts[0])
	g.AcceptTipBlock()
	g.RejectTipBlock(ErrDuplicateBlock)

	// ---------------------------------------------------------------------
	// Orphan tests.
	// ---------------------------------------------------------------------

	// Create valid orphan block with zero prev hash.
	//
	//   No previous block
	//                    \-> borphan0(1)
	g.SetTip("b1")
	g.NextBlock("borphan0", outs[1], ticketOuts[1], func(b *wire.MsgBlock) {
		b.Header.PrevBlock = chainhash.Hash{}
	})
	g.RejectTipBlock(ErrMissingParent)

	// Create valid orphan block.
	//
	//   ... -> b1(0)
	//               \-> borphanbase(1) -> borphan1(2)
	g.SetTip("b1")
	g.NextBlock("borphanbase", outs[1], ticketOuts[1])
	g.NextBlock("borphan1", outs[2], ticketOuts[2])
	g.RejectTipBlock(ErrMissingParent)

	// Ensure duplicate orphan blocks are rejected.
	g.RejectTipBlock(ErrMissingParent)

	// ---------------------------------------------------------------------
	// Out-of-order forked reorg to invalid block tests.
	//
	// NOTE: These tests have been modified to be processed in order despite
	// the comments for now due to the removal of orphan processing.  They
	// are otherwise left intact because future commits will allow headers
	// to be processed independently from the block data which will allow
	// out of order processing of data again so long as the headers are
	// processed in order.
	// ---------------------------------------------------------------------

	// Create a fork that ends with block that generates too much proof-of-work
	// coinbase, but with a valid fork first.
	//
	//   ... -> b1(0) -> b2(1)
	//               \-> bpw1(1) -> bpw2(2) -> bpw3(3)
	//                  (bpw1 added last)
	g.SetTip("b1")
	g.NextBlock("b2", outs[1], ticketOuts[1])
	g.AcceptTipBlock()
	g.ExpectTip("b2")

	g.SetTip("b1")
	g.NextBlock("bpw1", outs[1], ticketOuts[1])
	g.NextBlock("bpw2", outs[2], ticketOuts[2])
	g.NextBlock("bpw3", outs[3], ticketOuts[3], func(b *wire.MsgBlock) {
		// Increase the first proof-of-work coinbase subsidy.
		b.Transactions[0].TxOut[2].Value++
	})
	g.AcceptHeader("bpw1")
	g.AcceptBlockData("bpw2")
	g.AcceptBlockData("bpw3")
	g.RejectBlock("bpw1", ErrBadCoinbaseValue)
	g.ExpectTip("bpw2")

	// Create a fork that ends with block that generates too much dev-org
	// coinbase, but with a valid fork first.
	//
	//   ... -> b1(0) -> bpw1(1) -> bpw2(2)
	//                          \-> bdc1(2) -> bdc2(3) -> bdc3(4)
	//                             (bdc1 added last)
	g.SetTip("bpw1")
	g.NextBlock("bdc1", outs[2], ticketOuts[2])
	g.NextBlock("bdc2", outs[3], ticketOuts[3])
	g.NextBlock("bdc3", outs[4], ticketOuts[4], func(b *wire.MsgBlock) {
		// Increase the proof-of-work dev subsidy by the provided amount.
		b.Transactions[0].TxOut[0].Value++
	})
	g.AcceptHeader("bdc1")
	g.AcceptBlockData("bdc2")
	g.AcceptBlockData("bdc3")
	g.RejectBlock("bdc1", ErrNoTax)
	g.ExpectTip("bdc2")
}

// genSharedProcessTestBlocks either generates a new set of blocks used in the
// process logic tests or returns an already generated set if called more than
// once.
//
//
// The generated blocks form a fairly complex overall block tree as follows:
//   - * denotes invalid header
//   - ! denotes invalid block prior to connection (e.g. vote from bad ticket)
//   - @ denotes invalid block when connected (e.g. double spend)
//   - bfb is the required first block
//   - bm# are blocks which allow coins to mature
//   - bse# are blocks which reach stake enabled height
//   - bsv# are blocks which reach stake validation height
//   - bfbbad is a variant of the first block that pays too much
//   - bfbbadchild and bfbbadchilda are valid blocks that descend from bfbbad
//   - b1badhdr has a header with too few votes (hdr sanity failure)
//   - b1badhdra has a header with a mismatched height (hdr positional failure)
//   - b1bad has a ticket purchase that pays too little (block sanity failure)
//   - b1bada is a block with an expired tx (block positional failure)
//   - b5b is a block with a vote from bad ticket (context failure)
//   - b6g is a block with a vote from bad ticket (context failure)
//   - b5h is a block with a double spend (connect failure)
//   - b6i is a block with a double spend (connect failure)
//   - b7j is a block with a double spend (connect failure)
//
//   genesis -> bfb -> bm0 -> ... -> bm# -> bse0 -> ... -> bse# -> bsv0 -> ...
//          \-> bfbbad! -> bfbbadchild
//                     \-> bfbbadchilda
//
//   ... -> bsv# -> bbm0 -> bbm1 -> ... -> bbm# -> b1 -> b2 -> ...
//                                                   \-> b1badhdr*
//                                                   \-> b1badhdra*
//                                                   \-> b1bad!
//                                                   \-> b1bada!
//
//   ... -> b3 -> b4  -> b5   -> b6   -> b7   -> b8  -> b9  -> b10  -> b11
//            |                                            \-> b10a
//            \-> b4b -> b5b! -> b6b  -> b7b  -> b8b
//            \-> b4c                |       \-> b8c -> b9c
//            |                      \-> b7d  -> b8d -> b9d
//            |                      \-> b7e  -> b8e
//            |
//            \-> b4f -> b5f  -> b6f  -> b7f  -> b8f -> b9f -> b10f
//            \-> b4g -> b5g  -> b6g! -> b7g  -> b8g
//            \-> b4h -> b5h@ -> b6h  -> b7h
//            \-> b4i -> b5i  -> b6i@ -> b7i  -> b8i
//            \-> b4j -> b5j  -> b6j  -> b7j@
func genSharedProcessTestBlocks(t *testing.T) (*chaingen.Generator, error) {
	processTestGeneratorLock.Lock()
	defer processTestGeneratorLock.Unlock()

	// Only generate the process test chain once.
	if processTestGenerator != nil {
		return processTestGenerator, nil
	}

	// Create a new database and chain instance needed to create the generator
	// populated with the desired blocks.
	params := chaincfg.RegNetParams()
	g, teardownFunc := newChaingenHarness(t, params, "sharedprocesstestblocks")
	defer teardownFunc()

	// Shorter versions of useful params for convenience.
	coinbaseMaturity := params.CoinbaseMaturity
	stakeValidationHeight := params.StakeValidationHeight

	// -------------------------------------------------------------------------
	// Block one variants.
	// -------------------------------------------------------------------------

	// Produce a first block with too much coinbase.
	//
	//   genesis
	//          \-> bfbbad
	g.CreateBlockOne("bfbbad", 1)
	g.AssertTipHeight(1)

	// Create a block that descends from the invalid first block.
	//
	//   genesis
	//          \-> bfbbad -> bfbbadchild
	g.NextBlock("bfbbadchild", nil, nil)
	g.AssertTipHeight(2)

	// Create a second block that descends from the invalid first block.
	//
	//   genesis
	//          \-> bfbbad -> bfbbadchild
	//                    \-> bfbbadchilda
	g.SetTip("bfbbad")
	g.NextBlock("bfbbadchilda", nil, nil)
	g.AssertTipHeight(2)

	// -------------------------------------------------------------------------
	// Generate enough blocks to reach stake validation height.
	// -------------------------------------------------------------------------

	g.SetTip("genesis")
	g.GenerateToStakeValidationHeight()

	// -------------------------------------------------------------------------
	// Generate enough blocks to have a known distance to the first mature
	// coinbase outputs for all tests that follow.  These blocks continue to
	// purchase tickets to avoid running out of votes.
	//
	//   ... -> bsv# -> bbm0 -> bbm1 -> ... -> bbm#
	// -------------------------------------------------------------------------

	var finalMaturityBlockName string
	for i := uint16(0); i < coinbaseMaturity; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bbm%d", i)
		g.NextBlock(blockName, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		finalMaturityBlockName = blockName
	}
	g.AssertTipHeight(uint32(stakeValidationHeight) + uint32(coinbaseMaturity))

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

	// -------------------------------------------------------------------------
	// Generate various invalid variants of b1.
	//
	// Note that * below indicates a block that has an invalid header and !
	// indicates a block that is invalid prior to connection due to sanity
	// and/or positional checks.
	//
	//   ... -> bbm#
	//              \-> b1badhdr*
	//              \-> b1badhdra*
	//              \-> b1bad!
	//              \-> b1bada!
	// -------------------------------------------------------------------------

	// Generate block with too few votes to possibly have majority which results
	// in a header that is rejected due to context-free (aka sanity) checks.
	g.NextBlock("b1badhdr", outs[0], ticketOuts[0], g.ReplaceWithNVotes(1))

	// Generate block with a mismatched height which results in a header that is
	// rejected due to positional checks.
	g.SetTip(finalMaturityBlockName)
	g.NextBlock("b1badhdra", outs[0], ticketOuts[0], func(b *wire.MsgBlock) {
		b.Header.Height--
	})

	// Generate block with a ticket that pays too little which results in being
	// rejected due to block sanity checks.
	g.SetTip(finalMaturityBlockName)
	g.NextBlock("b1bad", outs[0], ticketOuts[0], func(b *wire.MsgBlock) {
		b.STransactions[5].TxOut[0].Value--
	})

	// Generate block with an expired transaction which results in being
	// rejected due to block positional checks.
	g.SetTip(finalMaturityBlockName)
	g.NextBlock("b1bada", outs[0], ticketOuts[0], func(b *wire.MsgBlock) {
		b.Transactions[1].Expiry = b.Header.Height
	})

	// -------------------------------------------------------------------------
	// Generate a few blocks to serve as a base for more complex branches below.
	//
	//   ... -> b1 -> b2 -> b3
	// -------------------------------------------------------------------------

	g.SetTip(finalMaturityBlockName)
	g.NextBlock("b1", outs[0], ticketOuts[0])
	g.NextBlock("b2", outs[1], ticketOuts[1])
	g.NextBlock("b3", outs[2], ticketOuts[2])

	// -------------------------------------------------------------------------
	// Generate a block tree with several branches from different fork points
	// where some of them have blocks that are invalid in various ways:
	//
	// Note that ! below indicates a block that is invalid prior to connection
	// due to sanity, positional, and/or contextual checks and @ indicates a
	// block that is invalid when actually connected as part of becoming the
	// main chain.
	//
	//  ... -> b3 -> b4  -> b5   -> b6   -> b7   -> b8  -> b9  -> b10  -> b11
	//           |                                            \-> b10a
	//           \-> b4b -> b5b! -> b6b  -> b7b  -> b8b
	//           \-> b4c                |       \-> b8c -> b9c
	//           |                      \-> b7d  -> b8d -> b9d
	//           |                      \-> b7e  -> b8e
	//           |
	//           \-> b4f -> b5f  -> b6f  -> b7f  -> b8f -> b9f -> b10f
	//           \-> b4g -> b5g  -> b6g! -> b7g  -> b8g
	//           \-> b4h -> b5h@ -> b6h  -> b7h
	//           \-> b4i -> b5i  -> b6i@ -> b7i  -> b8i
	//           \-> b4j -> b5j  -> b6j  -> b7j@
	// -------------------------------------------------------------------------

	// Generate a run of blocks that has the most cumulative work and thus will
	// comprise the main chain.
	//
	//  ... -> b3 -> b4  -> b5   -> b6  -> b7  -> b8  -> b9  -> b10  -> b11
	g.NextBlock("b4", outs[3], ticketOuts[3])
	g.NextBlock("b5", outs[4], ticketOuts[4])
	g.NextBlock("b6", outs[5], ticketOuts[5])
	g.NextBlock("b7", outs[6], ticketOuts[6])
	g.NextBlock("b8", outs[7], ticketOuts[7])
	g.NextBlock("b9", outs[8], ticketOuts[8])
	g.NextBlock("b10", outs[9], ticketOuts[9])
	g.NextBlock("b11", outs[10], ticketOuts[10])

	// Generate a side chain with a single block just prior to the best tip.
	//
	//  ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//                                                       \-> b10a
	g.SetTip("b9")
	g.NextBlock("b10a", outs[9], ticketOuts[9])

	// Generate a side chain that contains a block with a contextual validation
	// error.
	//
	//  ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//           |                                           \-> b10a
	//           \-> b4b -> b5b! -> b6b  -> b7b -> b8b
	g.SetTip("b3")
	g.NextBlock("b4b", outs[3], ticketOuts[3])
	g.NextBlock("b5b", outs[4], ticketOuts[4], func(b *wire.MsgBlock) {
		// Corrupt the referenced ticket so the block is invalid due to an
		// unavailable ticket.
		b.STransactions[1].TxIn[1].PreviousOutPoint.Hash[0] ^= 0x55
	})
	g.NextBlock("b6b", outs[5], ticketOuts[5])
	g.NextBlock("b7b", outs[6], ticketOuts[6])
	g.NextBlock("b8b", outs[7], ticketOuts[7])

	// Generate a side chain with a single block further back in history.
	//
	//  ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//           |                                           \-> b10a
	//           \-> b4b -> b5b! -> b6b  -> b7b -> b8b
	//           \-> b4c
	g.SetTip("b3")
	g.NextBlock("b4c", outs[3], ticketOuts[3])

	// Generate a side chain that itself forks from a side chain.
	//
	//  ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//           |                                           \-> b10a
	//           \-> b4b -> b5b! -> b6b  -> b7b -> b8b
	//           \-> b4c                       \-> b8c -> b9c
	g.SetTip("b7b")
	g.NextBlock("b8c", outs[7], ticketOuts[7])
	g.NextBlock("b9c", outs[8], ticketOuts[8])

	// Generate a side chain that itself forks from the same side chain as
	// above, but from the prior block.
	//
	//  ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//           |                                           \-> b10a
	//           \-> b4b -> b5b! -> b6b  -> b7b -> b8b
	//           \-> b4c                |      \-> b8c -> b9c
	//                                  \-> b7d -> b8d -> b9d
	g.SetTip("b6b")
	g.NextBlock("b7d", outs[6], ticketOuts[6])
	g.NextBlock("b8d", outs[7], ticketOuts[7])
	g.NextBlock("b9d", outs[8], ticketOuts[8])

	// Generate another side chain that itself forks from the same side chain
	// and block as the above.
	//
	//  ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//           |                                           \-> b10a
	//           \-> b4b -> b5b! -> b6b  -> b7b -> b8b
	//           \-> b4c                |      \-> b8c -> b9c
	//                                  \-> b7d -> b8d -> b9d
	//                                  \-> b7e -> b8e
	g.SetTip("b6b")
	g.NextBlock("b7e", outs[6], ticketOuts[6])
	g.NextBlock("b8e", outs[7], ticketOuts[7])

	// Generate a valid competing side chain that is a single block behind the
	// best tip in terms of cumulative work.
	//
	//  ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//           \-> b4f -> b5f  -> b6f  -> b7f -> b8f -> b9f -> b10f
	g.SetTip("b3")
	g.NextBlock("b4f", outs[3], ticketOuts[3])
	g.NextBlock("b5f", outs[4], ticketOuts[4])
	g.NextBlock("b6f", outs[5], ticketOuts[5])
	g.NextBlock("b7f", outs[6], ticketOuts[6])
	g.NextBlock("b8f", outs[7], ticketOuts[7])
	g.NextBlock("b9f", outs[8], ticketOuts[8])
	g.NextBlock("b10f", outs[9], ticketOuts[9])

	// Generate a side chain that contains an invalid block nested a few blocks
	// deep where that block is invalid due to a contextual validation error.
	//
	//  ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//           \-> b4g -> b5g  -> b6g! -> b7g -> b8g
	g.SetTip("b3")
	g.NextBlock("b4g", outs[3], ticketOuts[3])
	g.NextBlock("b5g", outs[4], ticketOuts[4])
	g.NextBlock("b6g", outs[5], ticketOuts[5], func(b *wire.MsgBlock) {
		// Corrupt the referenced ticket so the block is invalid due to an
		// unavailable ticket.
		b.STransactions[1].TxIn[1].PreviousOutPoint.Hash[0] ^= 0x55
	})
	g.NextBlock("b7g", outs[6], ticketOuts[6])
	g.NextBlock("b8g", outs[7], ticketOuts[7])

	// Generate a side chain that contains an invalid block nested a couple of
	// blocks deep where that block is invalid due to a double spend of an
	// output spent by one of its ancestors.
	//
	//  ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//           \-> b4h -> b5h@ -> b6h  -> b7h
	g.SetTip("b3")
	g.NextBlock("b4h", outs[3], ticketOuts[3])
	g.NextBlock("b5h", outs[2], ticketOuts[4]) // Double spend
	g.NextBlock("b6h", outs[5], ticketOuts[5])
	g.NextBlock("b7h", outs[6], ticketOuts[6])

	// Generate a side chain that contains an invalid block nested a few blocks
	// deep where that block is invalid due to a double spend of an output spent
	// by one of its ancestors.
	//
	//  ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//           \-> b4i -> b5i  -> b6i@ -> b7i -> b8i
	g.SetTip("b3")
	g.NextBlock("b4i", outs[3], ticketOuts[3])
	g.NextBlock("b5i", outs[4], ticketOuts[4])
	g.NextBlock("b6i", outs[2], ticketOuts[5]) // Double spend
	g.NextBlock("b7i", outs[6], ticketOuts[6])
	g.NextBlock("b8i", outs[7], ticketOuts[7])

	// Generate a side chain that contains an invalid block nested several
	// blocks deep and also serves as its tip where that block is invalid due to
	// a double spend of an output spent by one of its ancestors.
	//
	//  ... -> b3 -> b4  -> b5  -> b6  -> b7   -> b8  -> b9  -> b10  -> b11
	//           \-> b4j -> b5j -> b6j -> b7j@
	g.SetTip("b3")
	g.NextBlock("b4j", outs[3], ticketOuts[3])
	g.NextBlock("b5j", outs[4], ticketOuts[4])
	g.NextBlock("b6j", outs[5], ticketOuts[5])
	g.NextBlock("b7j", outs[2], ticketOuts[6]) // Double spend

	processTestGenerator = g.Generator
	return processTestGenerator, nil
}

// TestProcessLogic ensures processing a mix of headers and blocks under a wide
// variety of fairly complex scenarios selects the expected best chain and
// properly tracks the header with the most cumulative work that is not known to
// be invalid as well as the one that is known to be invalid (when it exists).
func TestProcessLogic(t *testing.T) {
	// Generate or reuse a shared chain generator with a set of blocks that form
	// a fairly complex overall block tree including multiple forks such that
	// some branches are valid and others contain invalid headers and/or blocks
	// with multiple valid descendants as well as further forks at various
	// heights from those invalid branches.
	sharedGen, err := genSharedProcessTestBlocks(t)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	// Create a new database and chain instance to run tests against.
	g, teardownFunc := newChaingenHarnessWithGen(t, "processtest", sharedGen)
	defer teardownFunc()

	// Shorter versions of useful params for convenience.
	params := g.Params()
	coinbaseMaturity := params.CoinbaseMaturity
	stakeEnabledHeight := params.StakeEnabledHeight
	stakeValidationHeight := params.StakeValidationHeight

	// -------------------------------------------------------------------------
	// Ensure the genesis block is rejected due to already being known, but its
	// header is accepted since duplicate headers are ignored and it is not
	// marked invalid due to the duplicate attempt.
	// -------------------------------------------------------------------------

	g.RejectBlock("genesis", ErrDuplicateBlock)
	g.AcceptHeader("genesis")
	g.ExpectBestHeader("genesis")
	g.ExpectBestInvalidHeader("")

	// -------------------------------------------------------------------------
	// Test basic acceptance and rejection logic of valid headers of invalid
	// blocks and their descendants.
	// -------------------------------------------------------------------------

	// Ensure that the valid header of a block that is invalid, but not yet
	// known to be invalid, is accepted.
	//
	//   genesis -> bfbbad
	g.AcceptHeader("bfbbad")

	// Ensure that a valid header that is the child of a known header for a
	// block that is invalid, but not yet known to be invalid, is accepted.
	//
	//   genesis -> bfbbad -> bfbbadchild
	g.AcceptHeader("bfbbadchild")

	// Process the invalid block and ensure it fails with the expected consensus
	// violation error.  Since the header was already previously accepted, it
	// will be marked invalid.
	//
	//   genesis
	//          \-> bfbbad
	g.RejectBlock("bfbbad", ErrBadCoinbaseValue)
	g.ExpectBestInvalidHeader("bfbbadchild")

	// Ensure that the header for a known invalid block is rejected.
	//
	//   genesis
	//          \-> bfbbad
	g.RejectHeader("bfbbad", ErrKnownInvalidBlock)

	// Ensure that a valid header that is already known and is the child of a
	// another known header for a block that is known invalid is rejected due to
	// a known invalid ancestor.
	//
	//   genesis
	//          \-> bfbbad
	//                    \-> bfbbadchild (already known invalid)
	g.RejectHeader("bfbbadchild", ErrInvalidAncestorBlock)

	// Ensure that a valid header that is NOT already known and is the child of
	// a known header for a block that is known invalid is rejected due to a
	// known invalid ancestor.
	//
	//   genesis
	//          \-> bfbbad
	//                    \-> bfbbadchilda (NOT already known)
	g.RejectHeader("bfbbadchilda", ErrInvalidAncestorBlock)
	g.ExpectBestInvalidHeader("bfbbadchild")

	// -------------------------------------------------------------------------
	// Ensure that all headers in the test chain through stake validation height
	// and the base maturity blocks are accepted in order.
	//
	//   genesis -> bfb -> bm0 -> ... -> bm# -> bse0 -> ... -> bse# -> ...
	//
	//   ... bsv0 -> ... -> bsv# -> bbm0 -> ... -> bbm#
	// -------------------------------------------------------------------------

	g.AcceptHeader("bfb")
	for i := uint16(0); i < coinbaseMaturity; i++ {
		blockName := fmt.Sprintf("bm%d", i)
		g.AcceptHeader(blockName)
	}
	tipHeight := int64(coinbaseMaturity) + 1
	for i := int64(0); tipHeight < stakeEnabledHeight; i++ {
		blockName := fmt.Sprintf("bse%d", i)
		g.AcceptHeader(blockName)
		tipHeight++
	}
	for i := int64(0); tipHeight < stakeValidationHeight; i++ {
		blockName := fmt.Sprintf("bsv%d", i)
		g.AcceptHeader(blockName)
		tipHeight++
	}
	for i := uint16(0); i < coinbaseMaturity; i++ {
		blockName := fmt.Sprintf("bbm%d", i)
		g.AcceptHeader(blockName)
	}

	// -------------------------------------------------------------------------
	// Ensure that a header that has a context-free (aka sanity) failure is
	// rejected as expected.  Since invalid headers are not added to the block
	// index, attempting to process it again is expected to return the specific
	// failure reason as opposed to a known invalid block error.
	//
	//   ... -> bbm#
	//              \-> b1badhdr
	// -------------------------------------------------------------------------

	g.RejectHeader("b1badhdr", ErrNotEnoughVotes)
	g.RejectHeader("b1badhdr", ErrNotEnoughVotes)
	g.ExpectBestInvalidHeader("bfbbadchild")

	// -------------------------------------------------------------------------
	// Ensure that a header that has a positional failure is rejected as
	// expected.  Since invalid headers are not added to the block index,
	// attempting to process it again is expected to return the specific failure
	// reason as opposed to a known invalid block error.
	//
	//   ... -> bbm#
	//              \-> b1badhdra
	// -------------------------------------------------------------------------

	g.RejectHeader("b1badhdra", ErrBadBlockHeight)
	g.RejectHeader("b1badhdra", ErrBadBlockHeight)
	g.ExpectBestInvalidHeader("bfbbadchild")

	// -------------------------------------------------------------------------
	// Ensure both headers and blocks whose parent header is not known are
	// rejected as expected.
	//
	//   ... -> bbm# -> b1 (neither header nor block data known to chain)
	//                    \-> b2
	// -------------------------------------------------------------------------

	g.RejectHeader("b2", ErrMissingParent)
	g.RejectBlock("b2", ErrMissingParent)
	g.ExpectBestInvalidHeader("bfbbadchild")

	// -------------------------------------------------------------------------
	// Ensure that the first few headers in the test chain that build on the
	// final base maturity block are accepted in order.
	//
	//   ... -> bbm# -> b1 -> b2 -> b3
	// -------------------------------------------------------------------------

	g.AcceptHeader("b1")
	g.AcceptHeader("b2")
	g.AcceptHeader("b3")
	g.ExpectBestHeader("b3")

	// -------------------------------------------------------------------------
	// Ensure that headers that form a block index with a bunch of branches from
	// different fork points are accepted and result in the one with the most
	// cumulative work being identified as the best header that is not already
	// known to be invalid.
	//
	// Note that the ! below indicates a block that is invalid due to violating
	// a consensus rule, but has a valid header.
	//
	//  ... -> b3 -> b4  -> b5   -> b6  -> b7  -> b8  -> b9  -> b10  -> b11
	//           |                                         \-> b10a
	//           \-> b4b -> b5b! -> b6b -> b7b -> b8b
	//           \-> b4c               |      \-> b8c -> b9c
	//                                 \-> b7d -> b8d -> b9d
	//                                 \-> b7e
	// -------------------------------------------------------------------------

	g.AcceptHeader("b4")
	g.AcceptHeader("b4b")
	g.AcceptHeader("b4c")
	g.AcceptHeader("b5")
	g.AcceptHeader("b5b") // Invalid block, but header valid.
	g.AcceptHeader("b6")
	g.AcceptHeader("b6b")
	g.AcceptHeader("b7")
	g.AcceptHeader("b7b")
	g.AcceptHeader("b7d")
	g.AcceptHeader("b7e")
	g.AcceptHeader("b8")
	g.AcceptHeader("b8b")
	g.AcceptHeader("b8c")
	g.AcceptHeader("b8d")
	g.AcceptHeader("b9")
	g.AcceptHeader("b9c")
	g.AcceptHeader("b9d")
	g.AcceptHeader("b10")
	g.AcceptHeader("b10a")
	g.AcceptHeader("b11")
	g.ExpectBestHeader("b11")

	// Even though ultimately one of the tips of the branches that have the same
	// cumulative work and are descendants of b5b will become the best known
	// invalid header, the block data for b5b is not yet known and thus it is
	// not yet known to be invalid.
	g.ExpectBestInvalidHeader("bfbbadchild")

	// -------------------------------------------------------------------------
	// Process all of the block data in the test chain through stake validation
	// height and the base maturity blocks to reach the point where the more
	// complicated branching structure starts.
	//
	// All of the headers for these blocks are already known, so this also
	// exercises in order processing as compared to the later tests which deal
	// with out of order processing.
	//
	// Also ensure duplicate blocks are rejected.
	//
	//   ... -> bfb -> bm0 -> ... -> bm# -> bse0 -> ... -> bse# -> ...
	//
	//   ... bsv0 -> ... -> bsv# -> bbm0 -> ... -> bbm#
	// -------------------------------------------------------------------------

	g.AcceptBlock("bfb")
	g.RejectBlock("bfb", ErrDuplicateBlock)
	for i := uint16(0); i < coinbaseMaturity; i++ {
		blockName := fmt.Sprintf("bm%d", i)
		g.AcceptBlock(blockName)
	}
	tipHeight = int64(coinbaseMaturity) + 1
	for i := int64(0); tipHeight < stakeEnabledHeight; i++ {
		blockName := fmt.Sprintf("bse%d", i)
		g.AcceptBlock(blockName)
		tipHeight++
	}
	for i := int64(0); tipHeight < stakeValidationHeight; i++ {
		blockName := fmt.Sprintf("bsv%d", i)
		g.AcceptBlock(blockName)
		tipHeight++
	}
	for i := uint16(0); i < coinbaseMaturity; i++ {
		blockName := fmt.Sprintf("bbm%d", i)
		g.AcceptBlock(blockName)
	}

	// -------------------------------------------------------------------------
	// Ensure that a block that has a context-free (aka sanity) failure is
	// rejected as expected.
	//
	// Since invalid blocks that have not had their headers added separately are
	// not added to the block index, attempting to process it again must return
	// the specific failure reason as opposed to a known invalid block error.
	//
	// This also means that adding the header of a block that has been rejected
	// due to a sanity error will succeed because the original failure is not
	// tracked.  Thus, ensure that is the case and that processing the bad block
	// again fails as expected and is marked as failed such that future attempts
	// to either add the header or the block will fail due to being a known
	// invalid block.
	//
	//   ... -> bbm#
	//              \-> b1bad
	// -------------------------------------------------------------------------

	g.RejectBlock("b1bad", ErrNotEnoughStake)
	g.RejectBlock("b1bad", ErrNotEnoughStake)
	g.ExpectBestInvalidHeader("bfbbadchild")

	g.AcceptHeader("b1bad")
	g.RejectBlock("b1bad", ErrNotEnoughStake)
	g.ExpectBestInvalidHeader("b1bad")
	g.RejectHeader("b1bad", ErrKnownInvalidBlock)
	g.RejectBlock("b1bad", ErrKnownInvalidBlock)

	// -------------------------------------------------------------------------
	// Ensure that a block that has a positional failure is rejected as
	// expected.  Since the header is valid and the block sanity checks pass,
	// the header will be added to the index and then end up marked as invalid,
	// so attempting to process the block again is expected to return a known
	// invalid block error.
	//
	//   ... -> bbm#
	//              \-> b1bada
	// -------------------------------------------------------------------------

	g.RejectBlock("b1bada", ErrExpiredTx)
	g.RejectBlock("b1bada", ErrKnownInvalidBlock)

	// Since both b1bad and b1bada have the same work and the block data is
	// rejected for both, whichever has the lowest hash (when treated as a
	// little-endian uint256) should be considered the best invalid.
	chooseLowestHash := func(blockNames ...string) string {
		lowestBlockName := blockNames[0]
		lowestHash := g.BlockByName(lowestBlockName).Header.BlockHash()
		for _, blockName := range blockNames[1:] {
			hash := g.BlockByName(blockName).Header.BlockHash()
			if compareHashesAsUint256LE(&hash, &lowestHash) < 0 {
				lowestHash = hash
				lowestBlockName = blockName
			}
		}
		return lowestBlockName
	}
	g.ExpectBestInvalidHeader(chooseLowestHash("b1bad", "b1bada"))

	// -------------------------------------------------------------------------
	// Ensure that processing block data for known headers out of order,
	// including one on a side chain, end up with the expected block becoming
	// the best chain tip.
	//
	//   ... -> b1 -> b2 -> b3 -> b4  -> b5
	//                        \-> b4b
	// -------------------------------------------------------------------------

	g.AcceptBlockData("b2")
	g.AcceptBlockData("b4b")
	g.AcceptBlockData("b3")
	g.AcceptBlockData("b5")
	g.AcceptBlockData("b4")
	g.AcceptBlock("b1")
	g.ExpectTip("b5")

	// -------------------------------------------------------------------------
	// Ensure that duplicate blocks are rejected without marking either it or
	// any of its descendants invalid.
	//
	//   ... -> b1 -> b2 -> b3 -> b4  -> b5 -> b6 -> ... -> b10 -> b11
	//                        \-> b4b    --                        ---
	//                                   ^^                        ^^^
	//                               current tip                 best header
	// -------------------------------------------------------------------------

	g.RejectBlock("b1", ErrDuplicateBlock)
	g.ExpectTip("b5")
	g.ExpectBestHeader("b11")

	// -------------------------------------------------------------------------
	// Ensure that a block on a side chain that has a contextual failure, and
	// for which the header is already known, is rejected as expected.  Since
	// the block is then known to be invalid, also ensure that all of its
	// descendant headers and blocks are subsequently rejected due to there
	// being a known invalid ancestor.
	//
	// Note that the ! below indicates a block that is invalid due to violating
	// a consensus rule during the contextual checks prior to connection.
	//
	//                   current tip                                 best header
	//                       vv                                           vvv
	//   ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//            |                                           \-> b10a
	//            \-> b4b -> b5b! -> b6b  -> b7b -> b8b
	//            \-> b4c    ----        |      \-> b8c -> b9c
	//                       ^^^^        \-> b7d -> b8d -> b9d
	//                     bad block     \-> b7e -> b8e
	// -------------------------------------------------------------------------

	// Process the bad block and ensure its header is rejected after since it is
	// then a known bad block.
	g.RejectBlock("b5b", ErrTicketUnavailable)
	g.RejectHeader("b5b", ErrKnownInvalidBlock)

	// Ensure that all of its descendant headers that were already known are
	// rejected.
	g.RejectHeader("b6b", ErrInvalidAncestorBlock)
	g.RejectHeader("b7b", ErrInvalidAncestorBlock)
	g.RejectHeader("b8b", ErrInvalidAncestorBlock)
	g.RejectHeader("b8c", ErrInvalidAncestorBlock)
	g.RejectHeader("b9c", ErrInvalidAncestorBlock)
	g.RejectHeader("b7d", ErrInvalidAncestorBlock)
	g.RejectHeader("b8d", ErrInvalidAncestorBlock)
	g.RejectHeader("b9d", ErrInvalidAncestorBlock)
	g.RejectHeader("b7e", ErrInvalidAncestorBlock)

	// Ensure that all of its descendant blocks associated with the headers that
	// were already known are rejected.
	g.RejectBlock("b6b", ErrInvalidAncestorBlock)
	g.RejectBlock("b7b", ErrInvalidAncestorBlock)
	g.RejectBlock("b8b", ErrInvalidAncestorBlock)
	g.RejectBlock("b8c", ErrInvalidAncestorBlock)
	g.RejectBlock("b9c", ErrInvalidAncestorBlock)
	g.RejectBlock("b7d", ErrInvalidAncestorBlock)
	g.RejectBlock("b8d", ErrInvalidAncestorBlock)
	g.RejectBlock("b9d", ErrInvalidAncestorBlock)
	g.RejectBlock("b7e", ErrInvalidAncestorBlock)

	// Ensure both the best known invalid and not known invalid headers are
	// as expected.
	//
	// Since both b9c and b9d have the same work and the block data is rejected
	// for both, whichever has the lowest hash (when treated as a little-endian
	// uint256) should be considered the best invalid.
	g.ExpectBestInvalidHeader(chooseLowestHash("b9c", "b9d"))
	g.ExpectBestHeader("b11")

	// Ensure that both a descendant header and block of the failed block that
	// themselves were not already known are rejected.
	g.RejectHeader("b8e", ErrInvalidAncestorBlock)
	g.RejectBlock("b8e", ErrInvalidAncestorBlock)

	// -------------------------------------------------------------------------
	// Similar to above, but this time when the invalid block on a side chain
	// leading up to a block that causes a reorg.
	//
	// Note that the ! below indicates a block that is invalid due to violating
	// a consensus rule during the contextual checks prior to connection.
	//
	//                   current tip                               best header
	//                       vv                                        vvv
	//   ... -> b3 -> b4  -> b5  -> b6   -> b7  -> b8  -> b9 -> b10 -> b11
	//            \-> b4g -> b5g -> b6g! -> b7g -> b8g
	//                              ----
	//                              ^^^^
	//                            bad block
	// -------------------------------------------------------------------------

	// Accept the block data for b4g, b5g, and b7g but only the block header for
	// b6g.  The data for block b7g should be accepted because b6g is not yet
	// known to be invalid.  Notice that b8g is intentionally not processed yet.
	g.AcceptBlockData("b4g")
	g.AcceptBlockData("b5g")
	g.AcceptHeader("b6g")
	g.AcceptBlockData("b7g")

	// Process the bad block and ensure its header is rejected after since it is
	// then a known bad block.
	g.RejectBlock("b6g", ErrTicketUnavailable)
	g.RejectHeader("b6g", ErrKnownInvalidBlock)

	// Ensure that all of its descendant headers and blocks that were already
	// known are rejected.
	g.RejectBlock("b6g", ErrDuplicateBlock)
	g.RejectBlock("b7g", ErrDuplicateBlock)

	// Ensure that both a descendant header and block of the failed block that
	// themselves were not already known are rejected.
	g.RejectHeader("b8g", ErrInvalidAncestorBlock)
	g.RejectBlock("b8g", ErrInvalidAncestorBlock)

	// Ensure both the best known invalid and not known invalid headers are
	// unchanged.
	g.ExpectBestInvalidHeader(chooseLowestHash("b9c", "b9d"))
	g.ExpectBestHeader("b11")

	// -------------------------------------------------------------------------
	// Similar to above, but this time with a connect failure instead of a
	// contextual one on a side chain leading up to a block that causes a reorg.
	//
	// Note that the @ below indicates a block that is invalid due to violating
	// a consensus rule during block connection.
	//
	//                   current tip                               best header
	//                       vv                                        vvv
	//   ... -> b3 -> b4  -> b5   -> b6  -> b7  -> b8  -> b9 -> b10 -> b11
	//            \-> b4h -> b5h@ -> b6h -> b7h
	//                       ----
	//                       ^^^^
	//                     bad block
	// -------------------------------------------------------------------------

	// Accept the block data for b4h and b6h, but only the block header for b5h.
	// The data for block b6h should be accepted because b5h is not yet known to
	// be invalid.
	g.AcceptBlockData("b4h")
	g.AcceptHeader("b5h")
	g.AcceptBlockData("b6h")

	// Process the bad block and ensure its header is rejected after since it is
	// then a known bad block.
	g.RejectBlock("b5h", ErrMissingTxOut)
	g.RejectHeader("b5h", ErrKnownInvalidBlock)

	// Ensure that all of its descendant headers and blocks that were already
	// known are rejected.
	g.RejectHeader("b6h", ErrInvalidAncestorBlock)
	g.RejectHeader("b7h", ErrInvalidAncestorBlock)
	g.RejectBlock("b6h", ErrDuplicateBlock)
	g.RejectBlock("b7h", ErrInvalidAncestorBlock)

	// Ensure both the best known invalid and not known invalid headers are
	// unchanged.
	g.ExpectBestInvalidHeader(chooseLowestHash("b9c", "b9d"))
	g.ExpectBestHeader("b11")

	// -------------------------------------------------------------------------
	// Similar to above, but this time with a connect failure in a side chain
	// block that is the direct target of a reorg.  This also doubles to ensure
	// that blocks that are seen first take precedence in best chain selection.
	//
	// Note that the @ below indicates a block that is invalid due to violating
	// a consensus rule during block connection.
	//
	//                   current tip                                best header
	//                       vv                                         vvv
	//   ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9 -> b10 -> b11
	//            \-> b4i -> b5i  -> b6i@ -> b7i -> b8i
	//                               ----
	//                               ^^^^
	//                             bad block
	// -------------------------------------------------------------------------

	// Accept the block data for b4i and b5i, but only the block header for b6i.
	// The header for block b6i should be accepted since the block is not yet
	// known to be invalid.
	g.AcceptBlockData("b4i")
	g.AcceptBlockData("b5i")
	g.AcceptHeader("b6i")

	// Process the bad block and ensure its header is rejected after since it is
	// then a known bad block.  The chain should reorg back to b5 since it was
	// seen first.
	g.RejectBlock("b6i", ErrMissingTxOut)
	g.RejectHeader("b6i", ErrKnownInvalidBlock)
	g.ExpectTip("b5")

	// Ensure that all of its descendant headers and blocks that were already
	// known are rejected.
	g.RejectHeader("b7i", ErrInvalidAncestorBlock)
	g.RejectBlock("b7i", ErrInvalidAncestorBlock)

	// Ensure that both a descendant header and block of the failed block that
	// themselves were not already known are rejected.  Notice that the header
	// for b7i was never accepted, so these should fail due to a missing parent.
	g.RejectHeader("b8i", ErrMissingParent)
	g.RejectBlock("b8i", ErrMissingParent)

	// Ensure both the best known invalid and not known invalid headers are
	// unchanged.
	g.ExpectBestInvalidHeader(chooseLowestHash("b9c", "b9d"))
	g.ExpectBestHeader("b11")

	// -------------------------------------------------------------------------
	// Ensure attempting to reorg to an invalid block on a side chain where the
	// blocks leading up to it are valid and have more cumulative work than the
	// current tip.  The chain should reorg to the final valid block since it
	// has more cumulative proof of work.
	//
	// Note that the @ below indicates a block that is invalid due to violating
	// a consensus rule during block connection.
	//
	//                   current tip                                best header
	//                       vv                                         vvv
	//   ... -> b3 -> b4  -> b5   -> b6  -> b7   -> b8  -> b9 -> b10 -> b11
	//            \-> b4j -> b5j  -> b6j -> b7j@
	//                                      ----
	//                                      ^^^^
	//                                    bad block
	// -------------------------------------------------------------------------

	// Accept the block data for b4j, b5j, and b7j, but only the block header
	// for b6j.  The block data for b7j should be accepted at this point even
	// though it is invalid because it can't be checked yet since the data for
	// b6j is not available.
	g.ExpectTip("b5")
	g.AcceptBlockData("b4j")
	g.AcceptBlockData("b5j")
	g.AcceptHeader("b6j")
	g.AcceptBlockData("b7j")

	// Make the block data for b6j available in order to complete the link
	// needed to trigger the reorg and check the blocks.  Since errors in reorgs
	// are currently attibuted to the block that caused them, the error in b7j
	// shoud be attributed to b6j, but b6j should still end up as the tip since
	// it is valid.
	g.RejectBlock("b6j", ErrMissingTxOut)
	g.ExpectTip("b6j")

	// Ensure both the best known invalid and not known invalid headers are
	// unchanged.
	g.ExpectBestInvalidHeader(chooseLowestHash("b9c", "b9d"))
	g.ExpectBestHeader("b11")

	// -------------------------------------------------------------------------
	// Ensure reorganizing to another valid branch when the block data is
	// processed out of order works as intended.
	//
	//                                                               best header
	//                                                                    vvv
	//   ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//            \-> b4f -> b5f  -> b6f  -> b7f -> b8f -> b9f -> b10f
	//            \-> b4j -> b5j  -> b6j -> b7j@
	//                               ---
	//                               ^^^
	//                           current tip
	// -------------------------------------------------------------------------

	g.AcceptHeader("b4f")
	g.AcceptHeader("b5f")
	g.AcceptHeader("b6f")
	g.AcceptHeader("b7f")
	g.AcceptHeader("b8f")
	g.AcceptHeader("b9f")
	g.AcceptHeader("b10f")
	g.ExpectBestHeader("b11")

	// Reorganize to a branch that actually has less work than the full initial
	// branch, but since its block data is not yet known, the secondary branch
	// data becoming available forces a reorg since it will be the best known
	// branch.
	//
	//                                                              best header
	//                                                                    vvv
	//   ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//            \-> b4f -> b5f  -> b6f  -> b7f -> b8f -> b9f -> b10f
	//            \-> b4j -> b5j  -> b6j                          ^^^^
	//                               ---                          ----
	//                               ^^^                         new tip
	//                             orig tip
	g.AcceptBlockDataWithExpectedTip("b9f", "b6j")
	g.AcceptBlockDataWithExpectedTip("b7f", "b6j")
	g.AcceptBlockDataWithExpectedTip("b8f", "b6j")
	g.AcceptBlockDataWithExpectedTip("b10f", "b6j")
	g.AcceptBlockDataWithExpectedTip("b6f", "b6j")
	g.AcceptBlockDataWithExpectedTip("b4f", "b6j")
	g.AcceptBlockDataWithExpectedTip("b5f", "b10f")

	// Make the data for the initial branch available, again in an out of order
	// fashion, but with an additional test condition built in such that all of
	// the data up to the point the initial branch has the exact same work as
	// the active branch is available to ensure the first seen block data takes
	// precedence despite the fact the header for the initial branch was seen
	// first.
	//
	//                                                               best header
	//                                                                    vvv
	//   ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//            \-> b4f -> b5f  -> b6f  -> b7f -> b8f -> b9f -> b10f
	//                                                            ^^^^
	//                                                            ----
	//                                                         current tip
	g.AcceptBlockDataWithExpectedTip("b8", "b10f")
	g.AcceptBlockDataWithExpectedTip("b10", "b10f")
	g.AcceptBlockDataWithExpectedTip("b7", "b10f")
	g.AcceptBlockDataWithExpectedTip("b9", "b10f")
	g.AcceptBlockDataWithExpectedTip("b6", "b10f")
	g.ExpectIsCurrent(false)

	// Finally, accept the data for final block on the initial branch that
	// causes the reorg and ensure the chain latches to believe it is current.
	//
	//                                                                  new tip
	//                                                                    vvv
	//   ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//            \-> b4f -> b5f  -> b6f  -> b7f -> b8f -> b9f -> b10f
	//                                                            ^^^^
	//                                                            ----
	//                                                          orig tip
	g.AcceptBlockDataWithExpectedTip("b11", "b11")
	g.ExpectIsCurrent(true)
}
