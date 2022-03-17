// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"testing"

	"github.com/decred/dcrd/blockchain/stake/v4"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/blockchain/v4/chaingen"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/wire"
)

// TestTSpendLegacyExpendituresPolicy performs tests against the treasury
// policy expenditure rule that were originally activated with the treasury
// agenda.  The following is the rough test plan:
//
// - Start chain and approve treasury vote
// - Mine and mature the following treasury txs:
//   - 4 tspends that spend the maximum allowed by the bootstrap policy
//   - 1 tadd
// - Advance until the previous tspends and tadd are out of the "current"
// expenditure window and in a past "policy check"
// - Attempt to drain as much as possible from the treasury given the update
// of the expenditure policy
// - Advance the chain until all previous tspends are outside the policy check
// range
// - Attempt to drain the bootstrap policy again.
func TestTSpendLegacyExpendituresPolicy(t *testing.T) {
	// Use a set of test chain parameters which allow for quicker vote
	// activation as compared to various existing network params.
	params := quickVoteActivationParams()

	// We'll significantly increase the SRI so that the treasurybase isn't
	// reduced throughout these tests and our arithmetic can be simpler.
	params.SubsidyReductionInterval = 1000

	// CoinbaseMaturity MUST be lower than TVI otherwise assumptions about
	// tspend maturity expenditure are broken. This is a sanity check to
	// ensure we're testing reasonable scenarios.
	if uint64(params.CoinbaseMaturity) > params.TreasuryVoteInterval {
		t.Fatal("params sanity check failed. CBM > TVI ")
	}

	// Clone the parameters so they can be mutated, find the correct
	// deployment for the treasury agenda, and, finally, ensure it is
	// always available to vote by removing the time constraints to prevent
	// test failures when the real expiration time passes.
	const tVoteID = chaincfg.VoteIDTreasury
	params = cloneParams(params)
	tVersion, deployment, err := findDeployment(params, tVoteID)
	if err != nil {
		t.Fatal(err)
	}
	removeDeploymentTimeConstraints(deployment)

	// Shorter versions of useful params for convenience.
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier
	tew := params.TreasuryExpenditureWindow
	tep := params.TreasuryExpenditurePolicy

	// Before any tspends are mined on the chain, the maximum allowed to be
	// spent is 150% of the expenditure bootstrap amount.
	expendBootstrap := int64(params.TreasuryExpenditureBootstrap +
		params.TreasuryExpenditureBootstrap/2)

	// Create a test harness initialized with the genesis block as the tip.
	g := newChaingenHarness(t, params)

	// Helper to verify the tip balance.
	assertTipTreasuryBalance := func(wantBalance int64) {
		t.Helper()
		ts, err := getTreasuryState(g, g.Tip().BlockHash())
		if err != nil {
			t.Fatal(err)
		}
		if ts.balance != wantBalance {
			t.Fatalf("unexpected treasury balance. want=%d got %d",
				wantBalance, ts.balance)
		}
	}

	// replaceTreasuryVersions is a munge function which modifies the
	// provided block by replacing the block, stake, and vote versions with
	// the treasury deployment version.
	replaceTreasuryVersions := func(b *wire.MsgBlock) {
		chaingen.ReplaceBlockVersion(int32(tVersion))(b)
		chaingen.ReplaceStakeVersion(tVersion)(b)
		chaingen.ReplaceVoteVersions(tVersion)(b)
	}

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks with the appropriate vote bits set
	// to reach one block prior to the treasury agenda becoming active.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()
	g.AdvanceFromSVHToActiveAgendas(tVoteID)

	// Ensure treasury agenda is active.
	tipHash := &g.chain.BestSnapshot().Hash
	gotActive, err := g.chain.IsTreasuryAgendaActive(tipHash)
	if err != nil {
		t.Fatalf("IsTreasuryAgendaActive: %v", err)
	}
	if !gotActive {
		t.Fatal("IsTreasuryAgendaActive: expected enabled treasury")
	}

	// Ensure the new maximum expenditure policy is *NOT* active (i.e. the
	// legacy policy is being enforced).
	gotActive, err = g.chain.IsRevertTreasuryPolicyActive(tipHash)
	if err != nil {
		t.Fatalf("IsRevertTreasuryPolicyActive: %v", err)
	}
	if gotActive {
		t.Fatal("IsRevertTreasuryPolicyActive: expected inactive reverted expenditure policy")
	}

	// tbaseBlocks will keep track of how many blocks with a treasury base
	// have been added to the chain.
	var tbaseBlocks int

	// ---------------------------------------------------------------------
	// Generate enough blocks that the treasury has funds. After
	// activation, the first block with funds will be at CoinbaseMaturity +
	// 1 blocks.
	//
	//   ... -> bva19 -> bpretf0 -> bpretf1
	//   ---------------------------------------------------------------------

	// Mine CoinbaseMaturity blocks
	outs := g.OldestCoinbaseOuts()
	for i := uint32(0); i < uint32(params.CoinbaseMaturity); i++ {
		name := fmt.Sprintf("bpretf%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase)
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// Assert the treasury is still empty.
	assertTipTreasuryBalance(0)

	// ---------------------------------------------------------------------
	// Generate a block that matures funds into the treasury.
	//
	// ... -> bpretf1 -> btfund
	// -------------------------------------------------------------------
	g.NextBlock("btfund", nil, outs[1:], replaceTreasuryVersions,
		replaceCoinbase)
	g.SaveTipCoinbaseOutsWithTreasury()
	g.AcceptTipBlock()
	outs = g.OldestCoinbaseOuts()
	tbaseBlocks++

	// The current treasury balance should equal the first treasurybase
	// subsidy.
	assertTipTreasuryBalance(devsub)

	// ---------------------------------------------------------------------
	// Generate enough blocks to get to the next TVI.
	//
	// ... -> btfund -> bpretvi0 -> .. -> bpretvin
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
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase)
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// Treasury balance should be the sum of the mature treasurybases.
	wantBalance := (int64(tbaseBlocks) - int64(params.CoinbaseMaturity)) * devsub
	assertTipTreasuryBalance(wantBalance)

	// Figure out the expiry for the next tspends.
	nextBlockHeight = g.Tip().Header.Height + 1 - uint32(tvi) // travel a bit back
	expiry = standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)

	// Each tspend will spend 1/4 of all the funds expendable within a
	// treasury expenditure policy window.
	nbTSpends := 4
	tspendAmount := expendBootstrap / int64(nbTSpends)
	tspendFee := uint64(0)
	tspends := make([]*wire.MsgTx, nbTSpends)
	tspendHashes := make([]*chainhash.Hash, nbTSpends)
	tspendVotes := make([]stake.TreasuryVoteT, nbTSpends)
	for i := 0; i < nbTSpends; i++ {
		tspends[i] = g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
			{
				Amount: dcrutil.Amount(uint64(tspendAmount) - tspendFee),
			},
		},
			dcrutil.Amount(tspendFee), expiry)
		h := tspends[i].TxHash()
		tspendHashes[i] = &h
		tspendVotes[i] = stake.TreasuryVoteYes
	}

	// Create a TADD to ensure they *DO NOT* count towards the policy.
	taddAmount := int64(1701)
	taddFee := dcrutil.Amount(0)
	tadd := g.CreateTreasuryTAdd(&outs[0], dcrutil.Amount(taddAmount), taddFee)
	tadd.Version = wire.TxVersionTreasury

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the previously created tspends. A
	// tvi-worth of yes votes should be sufficient.
	//
	// ... -> bpretvin -> bv0 -> ... -> bvn
	// ---------------------------------------------------------------------
	voteCount := params.TicketsPerBlock
	for i := uint64(0); i < tvi*2; i++ {
		name := fmt.Sprintf("bv%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase,
			addTSpendVotes(t, tspendHashes, tspendVotes, voteCount,
				false))
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// ---------------------------------------------------------------------
	// Generate a block that includes the tspends and a tadd.
	//
	// ... -> bvn -> btspends
	// ---------------------------------------------------------------------
	g.NextBlock("btspends", nil, outs[1:], replaceTreasuryVersions,
		replaceCoinbase,
		func(b *wire.MsgBlock) {
			for _, tspend := range tspends {
				b.AddSTransaction(tspend)
			}
			b.AddSTransaction(tadd)
		})
	g.SaveTipCoinbaseOutsWithTreasury()
	g.AcceptTipBlock()
	outs = g.OldestCoinbaseOuts()
	tbaseBlocks++
	tspendsMinedHeight := g.Tip().Header.Height

	// ---------------------------------------------------------------------
	// Generate enough blocks that the tspends become mature and their
	// outputs are reflected in the treasury balance.
	//
	// ... -> btspends -> btsm0 -> ... -> btsmn
	// ---------------------------------------------------------------------

	// Mine CoinbaseMaturity+1 blocks
	for i := uint32(0); i < uint32(params.CoinbaseMaturity+1); i++ {
		name := fmt.Sprintf("btsm%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase)
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// Assert the balance equals everything received by the treasury minus
	// immature tbases and the just mined tspends.
	wantBalance = (int64(tbaseBlocks)-int64(params.CoinbaseMaturity))*devsub +
		taddAmount - tspendAmount*int64(nbTSpends)
	assertTipTreasuryBalance(wantBalance)

	// Expiry for the next set of tspends.
	nextBlockHeight = g.Tip().Header.Height + 1 - uint32(tvi)
	expiry = standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)

	// We have spent the entire possible amount for the most recent
	// expenditure window. To assert that is true, create, approve and
	// attempt to mine a very small tspend.
	smallTSpendAmount := int64(1)
	smallTSpendFee := int64(0)
	smallTSpend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{
			Amount: dcrutil.Amount(uint64(smallTSpendAmount - smallTSpendFee)),
		},
	},
		dcrutil.Amount(smallTSpendFee), expiry)
	h := smallTSpend.TxHash()
	tspendHashes = []*chainhash.Hash{&h}
	tspendVotes = []stake.TreasuryVoteT{stake.TreasuryVoteYes}

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the previously created small
	// tspend.  A tvi-worth of yes votes should be sufficient.
	//
	// ... -> btsmn -> bvv0 -> ... -> bvvn
	// ---------------------------------------------------------------------
	for i := uint64(0); i < tvi*2; i++ {
		name := fmt.Sprintf("bvv%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase,
			addTSpendVotes(t, tspendHashes, tspendVotes, voteCount,
				false))
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// ---------------------------------------------------------------------
	// Attempt to mine a block to include the small tspend.
	//
	// ... -> bvvn -> bsmalltspendfail
	// ---------------------------------------------------------------------
	tipName := g.TipName()
	g.NextBlock("bsmalltspendfail", nil, outs[1:], replaceTreasuryVersions,
		replaceCoinbase,
		func(b *wire.MsgBlock) {
			b.AddSTransaction(smallTSpend)
		})
	g.RejectTipBlock(ErrInvalidExpenditure)
	g.SetTip(tipName)

	// The next part of the test will attempt to mine some tspends on the
	// first block after the expenditure window that includes the previous
	// tspends is no longer the current window but is rather used to
	// determine the average past expenditure.
	//
	// In order to do that we need to work backwards from the target test
	// height so that we can approve the test tspends at _exactly_ the
	// correct block.
	nextEWHeight := uint64(tspendsMinedHeight) + tvi*mul*tew
	currentHeight := uint64(g.Tip().Header.Height)
	nbBlocks := nextEWHeight - currentHeight - 1

	// ---------------------------------------------------------------------
	// Advance until the block we'll need to start voting on.
	//
	// ... -> bvvn -> bsmalltspendfail
	//            \-> bnextew0 -> ... -> bnextewn
	// ---------------------------------------------------------------------
	for i := uint64(0); i < nbBlocks; i++ {
		name := fmt.Sprintf("bnextew%d", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase,
			addTSpendVotes(t, tspendHashes, tspendVotes, voteCount,
				false))
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// The maximum we can now spend is 150% of the previous tspend sum.
	maxPolicySpend := expendBootstrap + expendBootstrap/2

	// Expiry for the next set of tspends.
	nextBlockHeight = g.Tip().Header.Height + 1 - uint32(tvi)
	expiry = standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)

	// Create a new set of tspends to test various policy scenarios. We
	// create and approve all in parallel to ease testing.
	testCases := []struct {
		name       string
		amount     int64
		wantAccept bool
	}{{
		// One more than the maximum allowed policy.
		name:       "max policy spend +1",
		amount:     maxPolicySpend + 1,
		wantAccept: false,
	}, {
		// Exactly the maximum allowed policy.
		name:       "max policy spend",
		amount:     maxPolicySpend,
		wantAccept: true,
	}}

	// Create the tspend txs, hashes and votes.
	nbTests := len(testCases)
	tspends = make([]*wire.MsgTx, nbTests)
	tspendHashes = make([]*chainhash.Hash, nbTests)
	tspendVotes = make([]stake.TreasuryVoteT, nbTests)
	for i := 0; i < nbTests; i++ {
		tspends[i] = g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
			{
				Amount: dcrutil.Amount(uint64(testCases[i].amount) - tspendFee),
			},
		},
			dcrutil.Amount(tspendFee), expiry)
		h := tspends[i].TxHash()
		tspendHashes[i] = &h
		tspendVotes[i] = stake.TreasuryVoteYes
	}

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the previously created tspends. A
	// tvi-worth of yes votes should be sufficient.
	//
	// ... -> btsmn -> bvvv0 -> ... -> bvvvn
	// ---------------------------------------------------------------------
	for i := uint64(0); i < tvi*2; i++ {
		name := fmt.Sprintf("bvvv%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase,
			addTSpendVotes(t, tspendHashes, tspendVotes, voteCount,
				false))
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// ---------------------------------------------------------------------
	// Perform the previously setup test cases. We mine one block for each
	// tspend. Only the last one is a valid one.
	//
	// ... -> bvvn
	//           \-> btest0 (ErrInvalidExpenditure)
	//           \-> btest1 (ok)
	// ---------------------------------------------------------------------

	// We'll come back to this block to test the policy scenarios.
	preTspendsBlock := g.TipName()

	for i, tc := range testCases {
		t.Logf("Case %s", tc.name)
		name := fmt.Sprintf("btest%d", i)

		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase,
			func(b *wire.MsgBlock) {
				b.AddSTransaction(tspends[i])
			})
		if tc.wantAccept {
			g.SaveTipCoinbaseOutsWithTreasury()
			g.AcceptTipBlock()
			outs = g.OldestCoinbaseOuts()
			tbaseBlocks++
		} else {
			g.RejectTipBlock(ErrInvalidExpenditure)

			// Switch back to the pre-tspends block to execute the
			// next test case.
			g.SetTip(preTspendsBlock)
		}
	}

	// ---------------------------------------------------------------------
	// Generate enough blocks that the tspend become mature and its outputs
	// are reflected in the treasury balance.
	//
	// ... -> btest4 -> btstm0 -> ... -> btstmn
	// ---------------------------------------------------------------------

	// Mine CoinbaseMaturity+1 blocks
	for i := uint32(0); i < uint32(params.CoinbaseMaturity+1); i++ {
		name := fmt.Sprintf("btstm%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase)
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// Assert the balance equals everything received by the treasury minus
	// immature tbases and the just mined tspends.
	wantBalance = (int64(tbaseBlocks)-int64(params.CoinbaseMaturity))*devsub +
		taddAmount - tspendAmount*int64(nbTSpends) - testCases[nbTests-1].amount
	assertTipTreasuryBalance(wantBalance)

	// ---------------------------------------------------------------------
	// Generate enough blocks to advance until all previously mined tspends
	// have falled out of the policy check window and we revert back to the
	// bootstrap rate.
	//
	// ... -> btstmn -> ... -> btpolfinn
	// -------------------------------------------------------------------
	for i := uint32(0); i < uint32(tvi*mul*tew*(tep+1)); i++ {
		name := fmt.Sprintf("btpolfin%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase)
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// Create a final tspend that should be able to fetch the maximum
	// amount allowed by the bootstrap policy again.
	nextBlockHeight = g.Tip().Header.Height + 1 - uint32(tvi)
	expiry = standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	tspendHashes = make([]*chainhash.Hash, 1)
	tspendVotes = make([]stake.TreasuryVoteT, 1)
	tspend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{
			Amount: dcrutil.Amount(uint64(expendBootstrap) - tspendFee),
		},
	},
		dcrutil.Amount(tspendFee), expiry)
	h = tspend.TxHash()
	tspendHashes[0] = &h
	tspendVotes[0] = stake.TreasuryVoteYes

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the previously created tspend. A
	// tvi-worth of yes votes should be sufficient.
	//
	// ... -> btpolfinn -> bvvvv0 -> ... -> bvvvvn
	// ---------------------------------------------------------------------
	for i := uint64(0); i < tvi*2; i++ {
		name := fmt.Sprintf("bvvvv%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase,
			addTSpendVotes(t, tspendHashes, tspendVotes, voteCount,
				false))
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// ---------------------------------------------------------------------
	// Generate a block that includes the final tspend
	//
	// ... -> bvvvn -> btfinaltspend
	// ---------------------------------------------------------------------
	g.NextBlock("btfinaltspend", nil, outs[1:], replaceTreasuryVersions,
		replaceCoinbase,
		func(b *wire.MsgBlock) {
			b.AddSTransaction(tspend)
		})
	g.AcceptTipBlock()
}

// TestTSpendExpendituresPolicyDCP0007 performs tests against the treasury
// policy window rules. The following is the rough test plan:
//
// - Start chain and approve treasury and new expenditure policy vote.
// - Approve 4 tspends that together spend the maximum allowed by policy and
//   a one atom tspend.
// - Mine a tadd.
// - Mine the 4 tspends that spend the maximum allowed by the policy
// - Advance the chain until all the tspends are outside the policy check
//   range.
// - Attempt to spend the max allowed by policy again with one tspend.
// - Advance the chain until all the tspends are outside the policy check
//   range again.
// - Mine a one atom tspend.
// - Advance one tvi.
// - Mine a tspend that spends the max allowed by policy again (tests the fix
//   done by DCP0007).
func TestTSpendExpendituresPolicyDCP0007(t *testing.T) {
	// Use a set of test chain parameters which allow for quicker vote
	// activation as compared to various existing network params.
	params := quickVoteActivationParams()

	// We'll significantly increase the SRI so that the treasurybase isn't
	// reduced throughout these tests and our arithmetic can be simpler.
	params.SubsidyReductionInterval = 1000

	// CoinbaseMaturity MUST be lower than TVI otherwise assumptions about
	// tspend maturity expenditure are broken. This is a sanity check to
	// ensure we're testing reasonable scenarios.
	if uint64(params.CoinbaseMaturity) > params.TreasuryVoteInterval {
		t.Fatal("params sanity check failed. CBM > TVI ")
	}

	// Clone the parameters so they can be mutated, find the correct
	// deployment for the treasury agenda, and, finally, ensure it is
	// always available to vote by removing the time constraints to prevent
	// test failures when the real expiration time passes.
	const tVoteID = chaincfg.VoteIDTreasury
	const tPolVoteID = chaincfg.VoteIDRevertTreasuryPolicy
	params = cloneParams(params)
	tVersion, err := mergeAgendas(params, []string{tVoteID, tPolVoteID})
	if err != nil {
		t.Fatal(err)
	}
	for i := range params.Deployments[tVersion] {
		removeDeploymentTimeConstraints(&params.Deployments[tVersion][i])
	}

	// Shorter versions of useful params for convenience.
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier
	tew := params.TreasuryExpenditureWindow

	// replaceTreasuryVersions is a munge function which modifies
	// the provided block by replacing the block, stake, and vote
	// versions with the treasury deployment version.
	replaceTreasuryVersions := func(b *wire.MsgBlock) {
		chaingen.ReplaceBlockVersion(int32(tVersion))(b)
		chaingen.ReplaceStakeVersion(tVersion)(b)
		chaingen.ReplaceVoteVersions(tVersion)(b)
	}

	// Create a test harness initialized with the genesis block as the tip.
	g := newChaingenHarness(t, params)

	// Helper to verify the tip balance.
	assertTipTreasuryBalance := func(wantBalance int64) {
		t.Helper()
		ts, err := getTreasuryState(g, g.Tip().BlockHash())
		if err != nil {
			t.Fatal(err)
		}
		if ts.balance != wantBalance {
			t.Fatalf("unexpected treasury balance. want=%d got %d",
				wantBalance, ts.balance)
		}
	}

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks with the appropriate vote bits set
	// to activate the treasury and revert treasury expenditure policy
	// agendas.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()
	g.AdvanceFromSVHToActiveAgendas(tVoteID, tPolVoteID)

	// Ensure treasury and revert expenditure policy agendas are active.
	tipHash := &g.chain.BestSnapshot().Hash
	gotActive, err := g.chain.IsTreasuryAgendaActive(tipHash)
	if err != nil {
		t.Fatalf("IsTreasuryAgendaActive: %v", err)
	}
	if !gotActive {
		t.Fatal("IsTreasuryAgendaActive: expected enabled treasury")
	}
	gotActive, err = g.chain.IsRevertTreasuryPolicyActive(tipHash)
	if err != nil {
		t.Fatalf("IsRevertTreasuryPolicyActive: %v", err)
	}
	if !gotActive {
		t.Fatal("IsRevertTreasuryPolicyActive: expected active reverted expenditure policy")
	}

	// tbaseBlocks will keep track of how many blocks with a treasury base
	// have been added to the chain.
	var tbaseBlocks int

	// ---------------------------------------------------------------------
	// Mine as many blocks as needed so that the treasury has enough funds
	// for our tests. Since we'll spend 150% of the "monthly" income of
	// treasury twice, we'll mine as many blocks as that 3 times.
	//
	// We'll also ensure we end just before a TVI block.
	//
	// ... -> bva19 -> bpretf0 -> ... -> bpretf#
	// ---------------------------------------------------------------------
	nbBlocks := tvi * mul * tew * 3
	blockHeight := uint64(g.Tip().Header.Height)
	if blockHeight%tvi != tvi-1 {
		nbBlocks += tvi - (blockHeight % tvi) - 1
	}
	outs := g.OldestCoinbaseOuts()
	for i := uint32(0); i < uint32(nbBlocks); i++ {
		name := fmt.Sprintf("bpretf%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase)
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// Figure out the expiry for the next tspends.
	nextBlockHeight := g.Tip().Header.Height + 1 - uint32(tvi) // travel a bit back
	expiry := standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)

	// Each tspend will spend 1/4 of all the funds expendable within a
	// treasury expenditure policy window. The maximum spendable amount is
	// the sum of tbases and tadds in the period, along with a 50% increase
	// allowance.
	//
	// We make sure the tadd amount that will be added makes the total sum
	// exactly divisble by the number of tspends (4).
	nbTSpends := 4
	tbaseAmount := int64(tvi * mul * tew * devsub)
	taddAmount := int64(nbTSpends*10) - tbaseAmount%int64(nbTSpends)
	incomeAmount := taddAmount + tbaseAmount
	tspendAmount := (incomeAmount + incomeAmount/2) / int64(nbTSpends)
	tspendFee := uint64(0)
	tspends := make([]*wire.MsgTx, nbTSpends)
	tspendHashes := make([]*chainhash.Hash, nbTSpends)
	tspendVotes := make([]stake.TreasuryVoteT, nbTSpends)
	for i := 0; i < nbTSpends; i++ {
		tspends[i] = g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
			{
				Amount: dcrutil.Amount(uint64(tspendAmount) - tspendFee),
			},
		},
			dcrutil.Amount(tspendFee), expiry)
		h := tspends[i].TxHash()
		tspendHashes[i] = &h
		tspendVotes[i] = stake.TreasuryVoteYes
	}

	// Also generate an additional, one atom tspend that will be approved.
	// This will be used to make assertions that the limit of expenditures
	// has not been breached.
	smallTSpendAmount := int64(1)
	smallTSpendFee := int64(0)
	smallTSpend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{
			Amount: dcrutil.Amount(uint64(smallTSpendAmount - smallTSpendFee)),
		},
	},
		dcrutil.Amount(smallTSpendFee), expiry)
	h := smallTSpend.TxHash()
	tspendHashes = append(tspendHashes, &h)
	tspendVotes = append(tspendVotes, stake.TreasuryVoteYes)

	// Create a TADD to ensure they *DO* count towards the maximum
	// expenditure policy.
	taddFee := dcrutil.Amount(0)
	tadd := g.CreateTreasuryTAdd(&outs[0], dcrutil.Amount(taddAmount), taddFee)
	tadd.Version = wire.TxVersionTreasury

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the previously created tspends.
	// Two tvi-worth of yes votes should be sufficient. Stop just before
	// that so we can mine the TAdd.
	//
	// ... -> bpretvi# -> bv0 -> ... -> bv#
	// ---------------------------------------------------------------------
	voteCount := params.TicketsPerBlock
	for i := uint64(0); i < tvi*2-1; i++ {
		name := fmt.Sprintf("bv%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase,
			addTSpendVotes(t, tspendHashes, tspendVotes, voteCount,
				false))
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// ---------------------------------------------------------------------
	// Generate a block that includes the tadd.
	//
	// ... -> bv# -> btadd
	// ---------------------------------------------------------------------
	g.NextBlock("btadd", nil, outs[1:], replaceTreasuryVersions,
		replaceCoinbase,
		func(b *wire.MsgBlock) {
			b.AddSTransaction(tadd)
		})
	g.SaveTipCoinbaseOutsWithTreasury()
	g.AcceptTipBlock()
	outs = g.OldestCoinbaseOuts()
	tbaseBlocks++

	// ---------------------------------------------------------------------
	// Negative test: a block that attempts to spend one more than the
	// maximum allowed by the policy is rejected.
	//
	// ... -> btadd
	//             \-> btspendserr
	// ---------------------------------------------------------------------
	preTspendsBlock := g.TipName()
	g.NextBlock("btspendserr", nil, outs[1:], replaceTreasuryVersions,
		replaceCoinbase,
		func(b *wire.MsgBlock) {
			for _, tspend := range tspends {
				b.AddSTransaction(tspend)
			}
			b.AddSTransaction(smallTSpend)
		})
	g.RejectTipBlock(ErrInvalidExpenditure)
	g.SetTip(preTspendsBlock)

	// ---------------------------------------------------------------------
	// Positive test: a block that attempts to spend exactly the maximum
	// allowed by the policy is accepted.
	//
	// ... -> btadd -> btspends
	//             \-> btspendserr
	// ---------------------------------------------------------------------
	g.NextBlock("btspends", nil, outs[1:], replaceTreasuryVersions,
		replaceCoinbase,
		func(b *wire.MsgBlock) {
			for _, tspend := range tspends {
				b.AddSTransaction(tspend)
			}
		})
	g.SaveTipCoinbaseOutsWithTreasury()
	g.AcceptTipBlock()
	outs = g.OldestCoinbaseOuts()
	tbaseBlocks++

	// ---------------------------------------------------------------------
	// Mine as many blocks as needed so that the mined tspends leave the
	// recent expenditure window and new tspends can be mined and stop just
	// before the next tvi block.
	//
	// ... -> btspends -> btsm0 -> ... -> btsm#
	// ---------------------------------------------------------------------
	nbBlocks = tvi*mul*tew - 1
	for i := uint32(0); i < uint32(nbBlocks); i++ {
		name := fmt.Sprintf("btsm%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase)
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// Assert the balance equals everything received by the treasury minus
	// immature tbases and mined tspends.
	wantBalance := (int64(tbaseBlocks)-int64(params.CoinbaseMaturity))*devsub +
		taddAmount - tspendAmount*int64(nbTSpends)
	assertTipTreasuryBalance(wantBalance)

	// Generate a new tspend that spends the entire possible amount and one
	// that spends one atom. This time we don't create a tadd.
	nextBlockHeight = g.Tip().Header.Height + 1 - uint32(tvi)
	expiry = standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	tspendAmount = tbaseAmount + tbaseAmount/2
	largeTSpend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{
			Amount: dcrutil.Amount(uint64(tspendAmount) - tspendFee),
		},
	},
		dcrutil.Amount(tspendFee), expiry)

	smallTSpend = g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{
			Amount: dcrutil.Amount(uint64(smallTSpendAmount - smallTSpendFee)),
		},
	},
		dcrutil.Amount(smallTSpendFee), expiry)
	txhLargeTSpend := largeTSpend.TxHash()
	txhSmallTSpend := smallTSpend.TxHash()
	tspendHashes = []*chainhash.Hash{&txhLargeTSpend, &txhSmallTSpend}
	tspendVotes = []stake.TreasuryVoteT{stake.TreasuryVoteYes, stake.TreasuryVoteYes}

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the previously created tspends.
	// Two tvi-worth of yes votes should be sufficient.
	//
	// ... -> btsm# -> bvv0 -> ... -> bvv#
	// ---------------------------------------------------------------------
	for i := uint64(0); i < tvi*2; i++ {
		name := fmt.Sprintf("bvv%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase,
			addTSpendVotes(t, tspendHashes, tspendVotes, voteCount,
				false))
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Negative test: a block that attempts to spend one more than the
	// maximum allowed by the policy is rejected.
	//
	// ... -> bvv#
	//            \-> btspendserr2
	// ---------------------------------------------------------------------
	preTspendsBlock = g.TipName()
	g.NextBlock("btspendserr2", nil, outs[1:], replaceTreasuryVersions,
		replaceCoinbase,
		func(b *wire.MsgBlock) {
			b.AddSTransaction(largeTSpend)
			b.AddSTransaction(smallTSpend)
		})
	g.RejectTipBlock(ErrInvalidExpenditure)
	g.SetTip(preTspendsBlock)

	// ---------------------------------------------------------------------
	// Positive test: a block that attempts to spend exactly the maximum
	// allowed by the policy is accepted.
	//
	// ... -> bvv# -> btspends2
	//            \-> btspendserr2
	// ---------------------------------------------------------------------
	g.NextBlock("btspends2", nil, outs[1:], replaceTreasuryVersions,
		replaceCoinbase,
		func(b *wire.MsgBlock) {
			b.AddSTransaction(largeTSpend)
		})
	g.SaveTipCoinbaseOutsWithTreasury()
	g.AcceptTipBlock()
	outs = g.OldestCoinbaseOuts()

	// ---------------------------------------------------------------------
	// Again, mine as many blocks as needed so that the mined tspends leave
	// the recent expenditure window and new tspends can be mined and stop
	// just before the next tvi block.
	//
	// ... -> btspends2 -> btsmm0 -> ... -> btsmm#
	// ---------------------------------------------------------------------
	nbBlocks = tvi*mul*tew - 1
	for i := uint32(0); i < uint32(nbBlocks); i++ {
		name := fmt.Sprintf("btsmm%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase)
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// Now we'll test that DCP0007 fixes the issue with locked maximum
	// expenditure.
	//
	// Again create two tspends (one small, one large) but this time both
	// could be included at the same time in a block.
	nextBlockHeight = g.Tip().Header.Height + 1 - uint32(tvi)
	expiry = standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	tspendAmount = tbaseAmount + tbaseAmount/2 - smallTSpendAmount - smallTSpendFee
	largeTSpend = g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{
			Amount: dcrutil.Amount(uint64(tspendAmount) - tspendFee),
		},
	},
		dcrutil.Amount(tspendFee), expiry)

	smallTSpend = g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{
			Amount: dcrutil.Amount(uint64(smallTSpendAmount - smallTSpendFee)),
		},
	},
		dcrutil.Amount(smallTSpendFee), expiry)
	txhLargeTSpend = largeTSpend.TxHash()
	txhSmallTSpend = smallTSpend.TxHash()
	tspendHashes = []*chainhash.Hash{&txhLargeTSpend, &txhSmallTSpend}
	tspendVotes = []stake.TreasuryVoteT{stake.TreasuryVoteYes, stake.TreasuryVoteYes}

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the previously created tspends.
	// Two tvi-worth of yes votes should be sufficient.
	//
	// ... -> btsmm# -> bvvv0 -> ... -> bvvv#
	// ---------------------------------------------------------------------
	for i := uint64(0); i < tvi*2; i++ {
		name := fmt.Sprintf("bvvv%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase,
			addTSpendVotes(t, tspendHashes, tspendVotes, voteCount,
				false))
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Include the small tspend.
	//
	// ... -> bvvv# -> btsmallts
	// ---------------------------------------------------------------------
	g.NextBlock("btsmallts", nil, outs[1:], replaceTreasuryVersions,
		replaceCoinbase,
		func(b *wire.MsgBlock) {
			b.AddSTransaction(smallTSpend)
		})
	g.SaveTipCoinbaseOutsWithTreasury()
	g.AcceptTipBlock()
	outs = g.OldestCoinbaseOuts()

	// ---------------------------------------------------------------------
	// Get to one block before the next TVI. The large tspend should still
	// be valid.
	//
	// ... -> btsmallts -> btsmmm0 -> ... -> btsmmm#
	// ---------------------------------------------------------------------
	for i := uint64(0); i < tvi-1; i++ {
		name := fmt.Sprintf("btsmmm%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase)
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Include the large tspend.
	//
	// ... -> btsmmm# -> btlargets
	// ---------------------------------------------------------------------
	g.NextBlock("btlargets", nil, outs[1:], replaceTreasuryVersions,
		replaceCoinbase,
		func(b *wire.MsgBlock) {
			b.AddSTransaction(largeTSpend)
		})
	g.SaveTipCoinbaseOutsWithTreasury()
	g.AcceptTipBlock()
	outs = g.OldestCoinbaseOuts()

	// ---------------------------------------------------------------------
	// Get to one block before the next TVI to verify the max allowed
	// expenditure.
	//
	// ... -> btlargets -> btsmmmm0 -> ... -> btsmmmm#
	// ---------------------------------------------------------------------
	for i := uint64(0); i < tvi-1; i++ {
		name := fmt.Sprintf("btsmmmm%v", i)
		g.NextBlock(name, nil, outs[1:], replaceTreasuryVersions,
			replaceCoinbase)
		g.SaveTipCoinbaseOutsWithTreasury()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// After mining the small and large tspends, the maximum allowed
	// expenditure shoud be zero.
	tipHash = &g.chain.BestSnapshot().Hash
	maxExpenditure, err := g.chain.MaxTreasuryExpenditure(tipHash)
	if err != nil {
		t.Fatal(err)
	}
	wantExpenditure := int64(0)
	if maxExpenditure != wantExpenditure {
		t.Fatalf("unexpected max expenditure - got %d, want %d",
			maxExpenditure, wantExpenditure)
	}
}
