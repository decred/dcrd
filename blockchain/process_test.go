// Copyright (c) 2019-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"sync"
	"testing"
	"time"

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
// The generated blocks form a fairly complex overall block tree as follows:
//
//	'-' denotes invalid header
//	'!' denotes invalid block prior to connection (e.g. vote from bad ticket)
//	'@' denotes invalid block when connected (e.g. double spend)
//	'bfb' is the required first block
//	'bm#' are blocks which allow coins to mature
//	'bse#' are blocks which reach stake enabled height
//	'bsv#' are blocks which reach stake validation height
//	'bfbbad' is a variant of the first block that pays too much
//	'bfbbadchild' and bfbbadchilda are valid blocks that descend from bfbbad
//	'b1badhdr' has a header with too few votes (hdr sanity failure)
//	'b1badhdra' has a header with a mismatched height (hdr positional failure)
//	'b1bad' has a ticket purchase that pays too little (block sanity failure)
//	'b1bada' is a block with an expired tx (block positional failure)
//	'b5b' is a block with a vote from bad ticket (context failure)
//	'b6g' is a block with a vote from bad ticket (context failure)
//	'b5h' is a block with a double spend (connect failure)
//	'b6i' is a block with a double spend (connect failure)
//	'b7j' is a block with a double spend (connect failure)
//
//	  genesis -> bfb -> bm0 -> ... -> bm# -> bse0 -> ... -> bse# -> bsv0 -> ...
//	         \-> bfbbad! -> bfbbadchild
//	                    \-> bfbbadchilda
//
//	  ... -> b3 -> b4  -> b5   -> b6   -> b7   -> b8  -> b9  -> b10  -> b11
//	           |                                            \-> b10a
//	           \-> b4b -> b5b! -> b6b  -> b7b  -> b8b
//	           \-> b4c                |       \-> b8c -> b9c
//	           |                      \-> b7d  -> b8d -> b9d
//	           |                      \-> b7e  -> b8e
//	           |
//	           \-> b4f -> b5f  -> b6f  -> b7f  -> b8f -> b9f -> b10f
//	           \-> b4g -> b5g  -> b6g! -> b7g  -> b8g
//	           \-> b4h -> b5h@ -> b6h  -> b7h
//	           \-> b4i -> b5i  -> b6i@ -> b7i  -> b8i
//	           \-> b4j -> b5j  -> b6j  -> b7j@
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
	// are currently attributed to the block that caused them, the error in b7j
	// should be attributed to b6j, but b6j should still end up as the tip since
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

// TestInvalidateReconsider ensures that manually invalidating blocks and
// reconsidering them works as expected under a wide variety of scenarios.
func TestInvalidateReconsider(t *testing.T) {
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
	g, teardownFunc := newChaingenHarnessWithGen(t, "invalidatetest", sharedGen)
	defer teardownFunc()

	// Shorter versions of useful params for convenience.
	params := g.Params()
	coinbaseMaturity := params.CoinbaseMaturity
	stakeEnabledHeight := params.StakeEnabledHeight
	stakeValidationHeight := params.StakeValidationHeight

	// -------------------------------------------------------------------------
	// Accept all headers in the initial test chain through stake validation
	// height and the base maturity blocks.
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
	// Initial setup of headers and block data needed for the invalidate and
	// reconsider tests below.
	// -------------------------------------------------------------------------

	// Accept block headers for the following tree structure:
	//
	// Note that the ! below indicates a block that is invalid due to violating
	// a consensus rule during the contextual checks prior to connection and the
	// @ indicates a block that is invalid due to violating a consensus rule
	// during block connection.
	//
	// ... -> b1 -> b2 -> ...
	//
	// ... -> b3 -> b4  -> b5   -> b6   -> b7  -> b8  -> b9  -> b10  -> b11
	//          \-> b4b -> b5b! -> b6b  -> b7b -> b8b       \-> b10a
	//          \-> b4c                |      \-> b8c -> b9c
	//          |                      \-> b7d -> b8d
	//          |                      \-> b7e -> b8e
	//          \-> b4f -> b5f  -> b6f  -> b7f -> b8f -> b9f -> b10f
	//          \-> b4i -> b5i  -> b6i@
	//
	g.AcceptHeader("b1")
	g.AcceptHeader("b2")
	g.AcceptHeader("b3")
	g.AcceptHeader("b4")
	g.AcceptHeader("b4b")
	g.AcceptHeader("b4c")
	g.AcceptHeader("b4f")
	g.AcceptHeader("b4i")
	g.AcceptHeader("b5")
	g.AcceptHeader("b5b") // Invalid block, but header valid.
	g.AcceptHeader("b5f")
	g.AcceptHeader("b5i")
	g.AcceptHeader("b6")
	g.AcceptHeader("b6b")
	g.AcceptHeader("b6f")
	g.AcceptHeader("b6i")
	g.AcceptHeader("b7")
	g.AcceptHeader("b7b")
	g.AcceptHeader("b7d")
	g.AcceptHeader("b7e")
	g.AcceptHeader("b7f")
	g.AcceptHeader("b8")
	g.AcceptHeader("b8b")
	g.AcceptHeader("b8c")
	g.AcceptHeader("b8d")
	g.AcceptHeader("b8e")
	g.AcceptHeader("b8f")
	g.AcceptHeader("b9")
	g.AcceptHeader("b9c")
	g.AcceptHeader("b9f")
	g.AcceptHeader("b10")
	g.AcceptHeader("b10a")
	g.AcceptHeader("b10f")
	g.AcceptHeader("b11")
	g.ExpectBestHeader("b11")

	// Accept all of the block data in the test chain through stake validation
	// height and the base maturity blocks to reach the point where the more
	// complicated branching structure starts.
	//
	//   ... -> bfb -> bm0 -> ... -> bm# -> bse0 -> ... -> bse# -> ...
	//
	//   ... bsv0 -> ... -> bsv# -> bbm0 -> ... -> bbm#
	g.AcceptBlock("bfb")
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

	// Accept the block data for several blocks in the main branch of the test
	// data.
	//
	// ... -> b1 -> b2 -> b3 -> b4  -> b5  -> b6  -> b7
	g.AcceptBlock("b1")
	g.AcceptBlock("b2")
	g.AcceptBlock("b3")
	g.AcceptBlock("b4")
	g.AcceptBlock("b5")
	g.AcceptBlock("b6")
	g.AcceptBlock("b7")

	// Accept the block data for several blocks in a branch of the test data
	// that contains a bad block such that the invalid branch has more work.
	// Notice that the bad block is only processed AFTER all of the data for its
	// descendants is added since they would otherwise be rejected due to being
	// part of a known invalid branch.
	//
	// ... -> b1 -> b2 -> b3 -> b4  -> b5  -> b6  -> b7
	//                      \-> b4b -> b5b -> b6b -> b7b -> b8b
	//                                 ---
	//                                 ^ (invalid block)
	g.AcceptBlockData("b4b")
	g.AcceptBlockData("b6b")
	g.AcceptBlockData("b7b")
	g.AcceptBlockData("b8b")
	g.RejectBlock("b5b", ErrTicketUnavailable)
	g.ExpectTip("b7")
	g.ExpectBestInvalidHeader("b9c")
	g.ExpectIsCurrent(false)

	// -------------------------------------------------------------------------
	// Invalidating and reconsidering an unknown block must error.
	// -------------------------------------------------------------------------

	g.InvalidateBlockAndExpectTip("b1bad", ErrUnknownBlock, "b7")
	g.ReconsiderBlockAndExpectTip("b1bad", ErrUnknownBlock, "b7")

	// -------------------------------------------------------------------------
	// The genesis block is not allowed to be invalidated, but it can be
	// reconsidered.
	// -------------------------------------------------------------------------

	g.InvalidateBlockAndExpectTip("genesis", ErrInvalidateGenesisBlock, "b7")
	g.ReconsiderBlockAndExpectTip("genesis", nil, "b7")
	g.ExpectBestHeader("b11")
	g.ExpectBestInvalidHeader("b9c")
	g.ExpectIsCurrent(false)

	// -------------------------------------------------------------------------
	// Invalidating a block that is already known to have failed validation has
	// no effect.
	// -------------------------------------------------------------------------

	g.InvalidateBlockAndExpectTip("b5b", nil, "b7")
	g.ExpectBestHeader("b11")
	g.ExpectBestInvalidHeader("b9c")
	g.ExpectIsCurrent(false)

	// -------------------------------------------------------------------------
	// Reconsider a block that is actually invalid when that block is located in
	// a side chain that has more work than the current best chain.  Since the
	// block is actually invalid, that side chain should remain invalid and
	// therefore NOT become the best chain even though it has more work.
	//
	// Note that the * indicates blocks that are marked as failed validation and
	// the # indicates blocks that are marked as having an invalid ancestor.
	//
	// Before reconsider:
	//
	// ... -> b1 -> b2 -> b3 -> b4  -> b5   -> b6   -> b7
	//                      |                          --
	//                      |                          ^ (best chain tip)
	//                      \-> b4b -> b5b* -> b6b# -> b7b# -> b8b#
	//                                 ---
	//                                 ^ (invalid block, reconsider)
	//
	// After reconsider:
	//
	// ... -> b1 -> b2 -> b3 -> b4  -> b5   -> b6   -> b7
	//                      |                          --
	//                      |                          ^ (best chain tip)
	//                      \-> b4b -> b5b* -> b6b# -> b7b# -> b8b#
	//                                 ---
	//                                 ^ (still invalid block)
	// -------------------------------------------------------------------------

	g.ReconsiderBlockAndExpectTip("b5b", ErrTicketUnavailable, "b7")
	g.ExpectBestHeader("b11")
	g.ExpectBestInvalidHeader("b9c")
	g.ExpectIsCurrent(false)

	// -------------------------------------------------------------------------
	// Reconsider a block that is a descendant of a block that is actually
	// invalid when that block is located in a side chain that has more work
	// than the current best chain.  Since an ancestor block is actually
	// invalid, that side chain should remain invalid and therefore NOT become
	// the best chain even though it has more work.
	//
	// Note that the * indicates blocks that are marked as failed validation and
	// the # indicates blocks that are marked as having an invalid ancestor.
	//
	// Before reconsider:
	//
	// ... -> b1 -> b2 -> b3 -> b4  -> b5   -> b6   -> b7
	//                      |                          --
	//                      |                          ^ (best chain tip)
	//                      \-> b4b -> b5b* -> b6b# -> b7b# -> b8b#
	//                                 ---     ----
	//                (invalid block)  ^       ^ (reconsider)
	//
	// After reconsider:
	//
	// ... -> b1 -> b2 -> b3 -> b4  -> b5   -> b6   -> b7
	//                      |                          --
	//                      |                          ^ (best chain tip)
	//                      \-> b4b -> b5b* -> b6b# -> b7b# -> b8b#
	//                                 ---
	//                                 ^ (still invalid block)
	// -------------------------------------------------------------------------

	g.ReconsiderBlockAndExpectTip("b6b", ErrTicketUnavailable, "b7")
	g.ExpectBestHeader("b11")
	g.ExpectBestInvalidHeader("b9c")
	g.ExpectIsCurrent(false)

	// -------------------------------------------------------------------------
	// Invalidate a multi-hop descendant of a block that is actually invalid
	// when that block is located in a side chain that has more work than the
	// current best chain.  Then, reconsider a block that is a descendant of
	// the block that is actually invalid, but an ancestor of the invalidated
	// multi-hop descendant.
	//
	// Since the part of that side chain that is then potentially valid has less
	// work than the current best chain, no attempt to reorg to that side chain
	// should be made.  Therefore, no error is expected and the actually invalid
	// block along with its descendants that are also ancestors of the multi-hop
	// descendant should no longer be known to be invalid.
	//
	// Note that the * indicates blocks that are marked as failed validation and
	// the # indicates blocks that are marked as having an invalid ancestor.
	//
	// Before reconsider:
	//
	// ... -> b1 -> b2 -> b3 -> b4  -> b5   -> b6   -> b7
	//                      |                          --
	//                      |                          ^ (best chain tip)
	//                      \-> b4b -> b5b* -> b6b# -> b7b* -> b8b#
	//                                 ---     ----    ----
	//                                 |       |       ^ (marked invalid)
	//                (invalid block)  ^       ^ (reconsider)
	//
	// After reconsider:
	//
	// ... -> b1 -> b2 -> b3 -> b4  -> b5   -> b6   -> b7
	//                      |                          --
	//                      |                          ^ (best chain tip)
	//                      \-> b4b -> b5b  -> b6b  -> b7b* -> b8b#
	//                                 -----------     ----
	//      (no longer known to be invalid) ^          ^ (still marked invalid)
	// -------------------------------------------------------------------------

	g.InvalidateBlockAndExpectTip("b7b", nil, "b7")
	g.ExpectBestHeader("b11")
	g.ExpectBestInvalidHeader("b9c")
	g.ExpectIsCurrent(false)

	g.ReconsiderBlockAndExpectTip("b6b", nil, "b7")
	g.ExpectBestHeader("b11")
	g.ExpectBestInvalidHeader("b9c")
	g.ExpectIsCurrent(false)

	// -------------------------------------------------------------------------
	// Reconsider a descendant of the manually invalidated multi-hop descendant
	// from the previous test.  Since the part of the side chain that is
	// then potentially valid has more work than the current chain, an attempt
	// to reorg to that branch should be made and fail due to the actually
	// invalid ancestor on that branch.
	//
	// Note that the * indicates blocks that are marked as failed validation and
	// the # indicates blocks that are marked as having an invalid ancestor.
	//
	// Before reconsider:
	//
	// ... -> b1 -> b2 -> b3 -> b4  -> b5   -> b6   -> b7
	//                      |                          --
	//                      |                          ^ (best chain tip)
	//                      \-> b4b -> b5b  -> b6b  -> b7b* -> b8b#
	//                                 ---             ----    ----
	//  (invalid, but not known as such) ^             ^       ^ (reconsider)
	//                                                 (marked invalid)
	// After reconsider:
	//
	// ... -> b1 -> b2 -> b3 -> b4  -> b5   -> b6   -> b7
	//                      |                          --
	//                      |                          ^ (best chain tip)
	//                      \-> b4b -> b5b* -> b6b# -> b7b# -> b8b#
	//                                 ---
	//                                 ^ (invalid block)
	// -------------------------------------------------------------------------

	g.ReconsiderBlockAndExpectTip("b8b", ErrTicketUnavailable, "b7")
	g.ExpectBestHeader("b11")
	g.ExpectBestInvalidHeader("b9c")
	g.ExpectIsCurrent(false)

	// -------------------------------------------------------------------------
	// Invalidate and reconsider block one.  After the invalidate, the genesis
	// block should become the best chain tip and the best header not known to
	// be invalid should become the best header known to be invalid while the
	// former becomes the genesis block header.
	//
	// Reconsidering the block should return the chain back to the original
	// state and notably should NOT have any validation errors because the
	// branches with the block that is actually invalid should still be known to
	// be invalid despite it being a descendant of the reconsidered block and
	// therefore no attempt to reorg should happen.
	//
	// Note that the * indicates blocks that are marked as failed validation and
	// the # indicates blocks that are marked as having an invalid ancestor.
	//
	// Before invalidate:
	//
	// genesis -> bfb -> ...-> b3 -> b4  -> b5   -> b6   -> b7
	//                           |                          --
	//                           |                          ^ (best chain tip)
	//                           \-> b4b -> b5b* -> b6b# -> b7b# -> b8b#
	//                                      ---
	//                                      ^ (invalid block)
	//
	// After invalidate:
	//
	// genesis -> bfb* -> ...-> b3# -> b4# -> b5#  -> b6#  -> b7#
	// -------                     |
	// ^ (best chain tip)          |
	//                             \-> b4b#-> b5b* -> b6b# -> b7b# -> b8b#
	//                                        ---
	//                                        ^ (invalid block)
	// After reconsider:
	//
	// genesis -> bfb -> ...-> b3 -> b4  -> b5   -> b6   -> b7
	//                           |                          --
	//                           |                          ^ (best chain tip)
	//                           \-> b4b -> b5b* -> b6b# -> b7b# -> b8b#
	//                                      ---
	//                                      ^ (invalid block)
	// -------------------------------------------------------------------------

	// Note that the genesis block is too far in the past to be considered
	// current.
	g.InvalidateBlockAndExpectTip("bfb", nil, "genesis")
	g.ExpectBestHeader("genesis")
	g.ExpectBestInvalidHeader("b11")
	g.ExpectIsCurrent(false)

	g.ReconsiderBlockAndExpectTip("bfb", nil, "b7")
	g.ExpectBestHeader("b11")
	g.ExpectBestInvalidHeader("b9c")
	g.ExpectIsCurrent(false)

	// -------------------------------------------------------------------------
	// Reconsidering a block that is actually invalid upon connection when it is
	// in a side chain that has had an ancestor manually invalidated and where
	// the parent of that invalid block has more work than the current best
	// chain should cause a reorganize to that parent block.
	//
	// In other words, ancestors of reconsidered blocks, even those that are
	// actually invalid, should have their invalidity status cleared and become
	// eligible for best chain selection.
	//
	// Note that the * indicates blocks that are marked as failed validation and
	// the # indicates blocks that are marked as having an invalid ancestor.
	//
	// Before reconsider:
	//
	// genesis -> bfb -> ...-> b3 -> b4  -> b5*  -> b6#  -> b7#
	//                           |   --
	//                           |   ^ (best chain tip)
	//                           \-> b4i -> b5i* -> b6i*
	//                                              ---
	//                   (invalid block, reconsider) ^
	//
	// After reconsider:
	//
	// genesis -> bfb -> ...-> b3 -> b4  -> b5*  -> b6#  -> b7#
	//                           \-> b4i -> b5i  -> b6i*
	//                                      ---     ---
	//                     (best chain tip) ^       ^ (invalid block)
	// -------------------------------------------------------------------------

	// The data for b6i should be accepted because the best chain tip has more
	// work at this point and thus there is no attempt to connect it.
	g.AcceptBlockData("b4i")
	g.AcceptBlockData("b5i")
	g.AcceptBlockData("b6i")

	// Invalidate the ancestor in the side chain first to ensure there is no
	// reorg when the main chain is invalidated for the purposes of making it
	// have less work when the side chain is reconsidered.
	g.InvalidateBlockAndExpectTip("b5i", nil, "b7")
	g.InvalidateBlockAndExpectTip("b5", nil, "b4")
	g.ExpectBestHeader("b10f")
	g.ExpectBestInvalidHeader("b11")
	g.ExpectIsCurrent(false)

	g.ReconsiderBlockAndExpectTip("b6i", ErrMissingTxOut, "b5i")
	g.ExpectBestHeader("b10f")
	g.ExpectBestInvalidHeader("b11")
	g.ExpectIsCurrent(false)

	// -------------------------------------------------------------------------
	// Undo the main chain invalidation for tests below.
	// -------------------------------------------------------------------------

	g.ReconsiderBlockAndExpectTip("b5", nil, "b7")
	g.ExpectBestHeader("b11")
	g.ExpectBestInvalidHeader("b9c")
	g.ExpectIsCurrent(false)

	// -------------------------------------------------------------------------
	// Make the primary branch have the most cumulative work and then invalidate
	// and reconsider a block that is an ancestor of the current best chain tip,
	// the best header that is NOT known to be invalid, the best header that is
	// known to be invalid, as well as several other branches.
	//
	// After the invalidation, the best invalid header should become the one
	// that was previously the best NOT known to be invalid and the best chain
	// tip and header should coincide which also means the chain should latch to
	// current.
	//
	// After reconsideration, everything should revert to the previous state,
	// including the chain believing it is not current again.
	//
	// Note that the * indicates blocks that are marked as failed validation and
	// the # indicates blocks that are marked as having an invalid ancestor.
	//
	// After invalidate:
	//
	//        v (best chain tip, best header)
	//        --
	// ... -> b2 -> ...
	//                                                        (best invalid)v
	//                                                                      ----
	// ... -> b3* -> b4#  -> b5#  -> b6#  -> b7#  -> b8#  -> b9# -> b10# -> b11#
	//           \-> b4b# -> b5b* -> b6b# -> b7b# -> b8b#       \-> b10a#
	//           \-> b4c#                |       \-> b8c# -> b9c#
	//           |                       \-> b7d# -> b8d#
	//           |                       \-> b7e# -> b8e#
	//           \-> b4f# -> b5f# -> b6f# -> b7f# -> b8f# -> b9f# -> b10f#
	//           \-> b4i# -> b5i# -> b6i*
	//
	// After reconsider:
	//
	//                                      (best chain tip) v  (best header) v
	//                                                       --             ---
	// ... -> b3  -> b4   -> b5   -> b6   -> b7   -> b8   -> b9  -> b10  -> b11
	//           \-> b4b  -> b5b* -> b6b# -> b7b# -> b8b#       \-> b10a
	//           \-> b4c                 |       \-> b8c# -> b9c#
	//           |                       |                   ----
	//           |                       |                   ^ (best invalid)
	//           |                       \-> b7d# -> b8d#
	//           |                       \-> b7e# -> b8e#
	//           \-> b4f  -> b5f  -> b6f  -> b7f  -> b8f  -> b9f  -> b10f
	//           \-> b4i  -> b5i  -> b6i*
	// -------------------------------------------------------------------------

	g.AcceptBlock("b8")
	g.AcceptBlock("b9")
	g.InvalidateBlockAndExpectTip("b3", nil, "b2")
	g.ExpectBestHeader("b2")
	g.ExpectBestInvalidHeader("b11")
	g.ExpectIsCurrent(true)

	g.ReconsiderBlockAndExpectTip("b3", nil, "b9")
	g.ExpectBestHeader("b11")
	g.ExpectBestInvalidHeader("b9c")
	g.ExpectIsCurrent(false)

	// -------------------------------------------------------------------------
	// Invalidate a block that does not have its data available and then
	// reconsider a descendant of it that itself has both ancestors and
	// descendants without block data available.  Then make the missing data
	// available such that there is more cumulative work than the current best
	// chain tip and ensure that everything is linked and the branch becomes the
	// new best tip.
	//
	// This ensures that reconsidering blocks properly resurrects tracking of
	// blocks that are not fully linked yet for both descendants and ancestors.
	//
	// Note that the @ below indicates data that is available before the
	// invalidate and reconsider and ! indicates data that is made available
	// afterwards.
	//
	//                                            (orig tip) v
	//                                                       --
	// ... -> b3  -> b4   -> b5   -> b6   -> b7   -> b8   -> b9
	//           \-> b4f! -> b5f@ -> b6f! -> b7f@ -> b8f! -> b9f@ -> b10f!
	//               ----            ---     ---                     -----
	//  (added last) ^   (invalidate) ^       ^ (reconsider)         ^ (new tip)
	// -------------------------------------------------------------------------

	// Notice that the data for b4f, b6f and, b8f, and b10f are intentionally
	// not made available yet.
	g.AcceptBlockData("b5f")
	g.AcceptBlockData("b7f")
	g.AcceptBlockData("b9f")
	g.InvalidateBlockAndExpectTip("b6f", nil, "b9")
	g.ReconsiderBlockAndExpectTip("b7f", nil, "b9")
	g.ExpectIsCurrent(false)

	// Add the missing data to complete the side chain while only adding b4f
	// last to ensure it triggers a reorg that consists of all of the blocks
	// that were previously missing data, but should now be linked.
	g.AcceptBlockData("b6f")
	g.AcceptBlockData("b8f")
	g.AcceptBlockData("b10f")
	g.AcceptBlockData("b4f")
	g.ExpectTip("b10f")
	g.ExpectBestHeader("b11")
	g.ExpectBestInvalidHeader("b9c")
	g.ExpectIsCurrent(false)

	// -------------------------------------------------------------------------
	// Reconsider a block that is a multi-hop descendant of an invalidated chain
	// and also itself has descendants such that the entire branch has more
	// cumulative work than the current best chain.  The entire branch,
	// including the ancestors of the block, is expected to be reconsidered
	// resulting in it becoming the new best chain.
	//
	// Note that the * indicates blocks that are marked as failed validation and
	// the # indicates blocks that are marked as having an invalid ancestor.
	//
	// Before reconsider:
	//
	// b3 -> b4   -> b5   -> b6   -> b7   -> b8   -> b9*  -> b10#  -> b11#
	//   |                                   --
	//   |                                   ^ (best chain tip)
	//   \-> b4f* -> b5f# -> b6f# -> b7f# -> b8f# -> b9f# -> b10f#
	//                               ----
	//                               ^ (reconsider)
	//
	// After reconsider:
	//
	// b3 -> b4   -> b5   -> b6   -> b7   -> b8   -> b9*  -> b10#  -> b11#
	//   \-> b4f  -> b5f  -> b6f  -> b7f  -> b8f  -> b9f  -> b10f
	//                                                       ----
	//                                                       ^ (best chain tip)
	// -------------------------------------------------------------------------

	g.InvalidateBlockAndExpectTip("b4f", nil, "b9")
	g.InvalidateBlockAndExpectTip("b9", nil, "b8")
	g.ExpectBestHeader("b8")
	g.ExpectBestInvalidHeader("b11")
	g.ExpectIsCurrent(true)

	g.ReconsiderBlockAndExpectTip("b7f", nil, "b10f")
	g.ExpectBestHeader("b10f")
	g.ExpectBestInvalidHeader("b11")
	g.ExpectIsCurrent(true)

	// -------------------------------------------------------------------------
	// Invalidate and reconsider the current best chain tip.  The chain should
	// believe it is current both before and after since the best chain tip
	// matches the best header in both cases.
	//
	// Note that the * indicates blocks that are marked as failed validation and
	// the # indicates blocks that are marked as having an invalid ancestor.
	//
	// b3 -> b4   -> b5   -> b6   -> b7   -> b8   -> b9*  -> b10#  -> b11#
	//   \-> b4f  -> b5f  -> b6f  -> b7f  -> b8f  -> b9f  -> b10f
	//                                                       ----
	//              (invalidate, reconsider, best chain tip) ^
	// -------------------------------------------------------------------------

	g.InvalidateBlockAndExpectTip("b10f", nil, "b9f")
	g.ExpectBestHeader("b9f")
	g.ExpectBestInvalidHeader("b11")
	g.ExpectIsCurrent(true)

	g.ReconsiderBlockAndExpectTip("b10f", nil, "b10f")
	g.ExpectBestHeader("b10f")
	g.ExpectBestInvalidHeader("b11")
	g.ExpectIsCurrent(true)

	// -------------------------------------------------------------------------
	// Reconsider a chain tip for a branch that has a previously invalidated
	// ancestor and that has more work than the current best tip, but does not
	// have all of the block data available.  All of the ancestors must no
	// longer be marked invalid and any descendants on other branches from any
	// of those ancestors that were reconsidered must also have their invalid
	// ancestor status removed.  However, any such descendants that are marked
	// as having failed validation instead must NOT have that status cleared.
	//
	// Note that the * indicates blocks that are marked as failed validation and
	// the # indicates blocks that are marked as having an invalid ancestor.
	//
	// Before reconsider (b10 and b11 data not available):
	//
	//                                                   (reconsider) v
	//                                                                ---
	// b3 -> b4   -> b5   -> b6   -> b7   -> b8   -> b9*  -> b10#  -> b11#
	//   \-> b4b  -> b5b* -> b6b# -> b7b# -> b8b#        \-> b10a#
	//   |                               \-> b8c# -> b9c#
	//   \-> b4f  -> b5f  -> b6f  -> b7f  -> b8f  -> b9f  -> b10f
	//                                                       ----
	//                                      (best chain tip) ^
	//
	// After reconsider (b10 and b11 data not available):
	//
	// b3 -> b4   -> b5   -> b6   -> b7   -> b8   -> b9   -> b10   -> b11
	//   \-> b4b  -> b5b* -> b6b# -> b7b# -> b8b#        \-> b10a
	//   |                               \-> b8c# -> b9c#
	//   |                                           ----
	//   |                                            ^ (best invalid header)
	//   \-> b4f  -> b5f  -> b6f  -> b7f  -> b8f  -> b9f  -> b10f
	//                                                       ----
	//                                      (best chain tip) ^
	// -------------------------------------------------------------------------

	g.ReconsiderBlockAndExpectTip("b11", nil, "b10f")
	g.ExpectBestHeader("b11")
	g.ExpectBestInvalidHeader("b9c")
	g.ExpectIsCurrent(false)

	// -------------------------------------------------------------------------
	// Accept the rest of the block data to reach the current best header so
	// it becomes the best chain tip and ensure the chain latches to current.
	//
	// ... -> b3 -> b4 -> b5 -> b6 -> b7 -> b8 -> b9 -> b10 -> b11
	// -------------------------------------------------------------------------

	g.AcceptBlockData("b10")
	g.AcceptBlock("b11")
	g.ExpectBestHeader("b11")
	g.ExpectIsCurrent(true)

	// -------------------------------------------------------------------------
	// Invalidate and reconsider a block at the tip of the current best chain so
	// that it has the same cumulative work as another branch that received the
	// block data first.  The second chain must become the best one despite them
	// having the exact same work and the header for the initial branch being
	// seen first because branches that receive their block data first take
	// precedence.  The initial branch must become the best chain again after
	// it is reconsidered because it has more work.
	//
	// The chain should believe it is current both before and after since the
	// best chain tip matches the best header in both cases.
	//
	//                    (invalidate, reconsider, header seen first) v
	//                                                                ---
	// ... -> b3 -> b4  -> b5  -> b6  -> b7  -> b8  -> b9  -> b10  -> b11
	//          \-> b4f -> b5f -> b6f -> b7f -> b8f -> b9f -> b10f
	//                                                        ----
	//                                      (data seen first) ^
	// -------------------------------------------------------------------------

	g.InvalidateBlockAndExpectTip("b11", nil, "b10f")
	g.ExpectBestHeader("b10f")
	g.ExpectIsCurrent(true)

	g.ReconsiderBlockAndExpectTip("b11", nil, "b11")
	g.ExpectBestHeader("b11")
	g.ExpectBestInvalidHeader("b9c")
	g.ExpectIsCurrent(true)
}

// TestAssumeValid validates that it is correctly determined whether or not a
// node is an ancestor of an assumed valid node under a variety of conditions.
func TestAssumeValid(t *testing.T) {
	// Clone the parameters so that they can be mutated.
	params := cloneParams(quickVoteActivationParams())
	stakeValidationHeight := params.StakeValidationHeight

	// Set the target time per block to 1 day so that less blocks are needed to
	// test against a 2 week threshold.
	params.TargetTimePerBlock = time.Hour * 24

	// Create a test harness initialized with the genesis block as the tip.
	g, teardownFunc := newChaingenHarness(t, params, "testassumevalid")
	defer teardownFunc()

	// Calculate the expected number of blocks in 2 weeks.
	const timeInTwoWeeks = time.Hour * 24 * 14
	targetTimePerBlock := params.TargetTimePerBlock
	expectedBlocksInTwoWeeks := int64(timeInTwoWeeks / targetTimePerBlock)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()
	g.AssertTipHeight(uint32(stakeValidationHeight))

	// ---------------------------------------------------------------------
	// Generate additional blocks until there is 1 less than the number of
	// blocks expected in 2 weeks.
	// ---------------------------------------------------------------------

	numBlocksToGen := expectedBlocksInTwoWeeks - stakeValidationHeight - 1
	i := int64(0)
	for ; i < numBlocksToGen; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bbav%d", i)
		g.NextBlock(blockName, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
	}
	g.AssertTipHeight(uint32(expectedBlocksInTwoWeeks - 1))

	// Set the assumed valid node to the tip.
	tipHash := &g.chain.BestSnapshot().Hash
	assumeValidNode := g.chain.index.LookupNode(tipHash)
	g.chain.assumeValidNode = assumeValidNode

	// Validate that isAssumeValidAncestor returns false for this scenario:
	//   - The tip height is less than the number of blocks expected in 2 weeks
	node := assumeValidNode.parent
	if g.chain.isAssumeValidAncestor(node) {
		t.Fatal("expected isAssumeValidAncestor to return false since the " +
			"tip height is less than the number of blocks expected in 2 weeks")
	}

	// ---------------------------------------------------------------------
	// Generate an additional block so that there is exactly the number of
	// blocks expected in 2 weeks.
	// ---------------------------------------------------------------------

	outs := g.OldestCoinbaseOuts()
	blockName := fmt.Sprintf("bbav%d", i)
	g.NextBlock(blockName, nil, outs[1:])
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()
	i++
	g.AssertTipHeight(uint32(expectedBlocksInTwoWeeks))

	// Set the assumed valid node to the tip.
	tipHash = &g.chain.BestSnapshot().Hash
	assumeValidNode = g.chain.index.LookupNode(tipHash)
	g.chain.assumeValidNode = assumeValidNode

	// Validate that isAssumeValidAncestor returns false for this scenario:
	//   - The assumed valid node is clamped back to be 2 weeks behind the tip
	//   - The node that is only 1 block behind the assumed valid node is not an
	//     ancestor of the clamped back assumed valid node
	node = assumeValidNode.parent
	if g.chain.isAssumeValidAncestor(node) {
		t.Fatal("expected isAssumeValidAncestor to return false since the " +
			"node is not an ancestor of the clamped back assumed valid node")
	}

	// ---------------------------------------------------------------------
	// Generate an additional 2 weeks worth of blocks + 2.  In addition to
	// the main chain, generate a side chain of the same length.
	// ---------------------------------------------------------------------
	bestSideBlockName := g.TipName()
	numBlocksToGen = i + expectedBlocksInTwoWeeks + 2
	for ; i < numBlocksToGen; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bbav%d", i)
		g.NextBlock(blockName, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		tipName := g.TipName()

		g.SetTip(bestSideBlockName)
		bestSideBlockName = fmt.Sprintf("bbava%d", i)
		g.NextBlock(bestSideBlockName, nil, outs[1:])
		g.AcceptedToSideChainWithExpectedTip(tipName)
		g.SetTip(tipName)
	}
	g.AssertTipHeight(2*uint32(expectedBlocksInTwoWeeks) + 2)

	// Set the assumed valid node to 2 weeks - 1 block behind the best side
	// chain block.
	sideBlockHash := g.BlockByName(bestSideBlockName).BlockHash()
	sideNode := g.chain.index.LookupNode(&sideBlockHash)
	assumeValidNode = sideNode.RelativeAncestor(expectedBlocksInTwoWeeks - 1)
	g.chain.assumeValidNode = assumeValidNode

	// Validate that isAssumeValidAncestor returns false for this scenario:
	//   - The node is not an ancestor of the best header
	if g.chain.isAssumeValidAncestor(assumeValidNode) {
		t.Fatal("expected isAssumeValidAncestor to return false when the " +
			"node is not an ancestor of the best header")
	}

	// Validate that isAssumeValidAncestor returns false for this scenario:
	//   - The node is an ancestor of the best header, and is at a height below
	//     the clamped back height, but is not an ancestor of the assumed valid
	//     block
	tipHash = &g.chain.BestSnapshot().Hash
	tip := g.chain.index.LookupNode(tipHash)
	node = tip.RelativeAncestor(expectedBlocksInTwoWeeks + 1)
	if g.chain.isAssumeValidAncestor(node) {
		t.Fatal("expected isAssumeValidAncestor to return false when the " +
			"node is not an ancestor of the assumed valid block")
	}

	// Set the assumed valid node to 2 weeks - 1 block behind the tip
	assumeValidNode = tip.RelativeAncestor(expectedBlocksInTwoWeeks - 1)
	g.chain.assumeValidNode = assumeValidNode

	// Validate that isAssumeValidAncestor returns false for this scenario:
	//   - The assumed valid node is 2 weeks - 1 block behind the tip
	//   - The assumed valid node is not an ancestor of the clamped back assumed
	//     valid node
	if g.chain.isAssumeValidAncestor(assumeValidNode) {
		t.Fatal("expected isAssumeValidAncestor to return false when the " +
			"assumed valid node is 2 weeks - 1 block behind the tip")
	}

	// Validate that isAssumeValidAncestor returns true for this scenario:
	//   - The assumed valid node is 2 weeks - 1 block behind the tip
	//   - The node that is 1 block behind the assumed valid node is an ancestor
	//     of the clamped back assumed valid node
	node = assumeValidNode.parent
	if !g.chain.isAssumeValidAncestor(node) {
		t.Fatal("expected isAssumeValidAncestor to return true when the " +
			"assumed valid node is 2 weeks - 1 block behind the tip and the " +
			"node is an ancestor of the clamped back assumed valid node")
	}

	// Set the assumed valid node to its parent so that it is exactly 2 weeks
	// behind the tip.
	assumeValidNode = assumeValidNode.parent
	g.chain.assumeValidNode = assumeValidNode

	// Validate that isAssumeValidAncestor returns true for this scenario:
	//   - The assumed valid node is exactly 2 weeks behind the tip
	node = assumeValidNode.parent
	if !g.chain.isAssumeValidAncestor(node) {
		t.Fatal("expected isAssumeValidAncestor to return true when the " +
			"assumed valid node is exactly 2 weeks behind the tip")
	}

	// Set the assumed valid node to its parent so that it is 2 weeks + 1 block
	// behind the tip.
	assumeValidNode = assumeValidNode.parent
	g.chain.assumeValidNode = assumeValidNode

	// Validate that isAssumeValidAncestor returns true for this scenario:
	//   - The assumed valid node is 2 weeks + 1 block behind the tip
	node = assumeValidNode.parent
	if !g.chain.isAssumeValidAncestor(node) {
		t.Fatal("expected isAssumeValidAncestor to return true when the " +
			"assumed valid node is 2 weeks + 1 block behind the tip")
	}

	// Set the assumed valid node to nil.
	assumeValidNode = nil
	g.chain.assumeValidNode = assumeValidNode

	// Validate that isAssumeValidAncestor returns false for this scenario:
	//   - The assumed valid node is nil
	if g.chain.isAssumeValidAncestor(node) {
		t.Fatal("expected isAssumeValidAncestor to return false when the " +
			"assumed valid node is nil")
	}
}
