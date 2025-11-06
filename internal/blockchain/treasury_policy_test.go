// Copyright (c) 2021-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"testing"

	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/blockchain/v5/chaingen"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/wire"
)

// TestTSpendLegacyExpendituresPolicy performs tests against the treasury
// policy expenditure rule that were originally activated with the treasury
// agenda.  The following is the rough test plan:
//
//   - Mine and mature 4 treasury spends that spend the maximum allowed by the
//     bootstrap policy
//   - Mine and mature 1 treasury add
//   - Advance until the previous tspends and tadd are out of the "current"
//     expenditure window and in a past "policy check"
//   - Attempt to drain as much as possible from the treasury given the update
//     of the expenditure policy
//   - Advance the chain until all previous tspends are outside the policy check
//     range
//   - Attempt to drain the bootstrap policy again.
func TestTSpendLegacyExpendituresPolicy(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and
	// significantly increase the SRI so that the treasurybase isn't reduced
	// throughout these tests to simplify the arithmetic.
	//
	// Also, mark the agendas the tests apply to as always active.
	const voteID = chaincfg.VoteIDTreasury
	params := quickVoteActivationParams()
	params.SubsidyReductionInterval = 1000
	forceDeploymentResult(t, params, voteID, "yes")

	// CoinbaseMaturity MUST be lower than TVI otherwise assumptions about
	// tspend maturity expenditure are broken. This is a sanity check to
	// ensure we're testing reasonable scenarios.
	if uint64(params.CoinbaseMaturity) > params.TreasuryVoteInterval {
		t.Fatal("params sanity check failed. CBM > TVI ")
	}

	// Shorter versions of useful params for convenience.
	cbm := params.CoinbaseMaturity
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier
	tew := params.TreasuryExpenditureWindow
	tep := params.TreasuryExpenditurePolicy

	// Before any tspends are mined on the chain, the maximum allowed to be
	// spent is 150% of the expenditure bootstrap amount.
	expendBootstrap := int64(params.TreasuryExpenditureBootstrap +
		params.TreasuryExpenditureBootstrap/2)

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

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
	tbaseBlocks := int64(g.Tip().Header.Height - 1)

	// Ensure the treasury balance is the expected value at this point.  It is
	// the number of treasurybases added minus the coinbase maturity all times
	// the amount of each treasurybase.
	trsyBaseAmt := g.Tip().STransactions[0].TxIn[0].ValueIn
	g.ExpectTreasuryBalance((tbaseBlocks - int64(cbm)) * trsyBaseAmt)

	// ---------------------------------------------------------------------
	// Generate enough blocks to get to the next TVI.
	//
	// ... -> bsv# -> bpretvi0 -> .. -> bpretvin
	// ---------------------------------------------------------------------

	nextBlockHeight := g.Tip().Header.Height + 1
	expiry := standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	start, _, err := standalone.CalcTSpendWindow(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}

	// Generate up to TVI blocks.
	outs := g.OldestCoinbaseOuts()
	for i := uint32(0); i < start-nextBlockHeight; i++ {
		name := fmt.Sprintf("bpretvi%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// Treasury balance should be the sum of the mature treasurybases.
	wantBalance := (tbaseBlocks - int64(cbm)) * trsyBaseAmt
	g.ExpectTreasuryBalance(wantBalance)

	// Figure out the expiry for the next tspends.
	nextBlockHeight = g.Tip().Header.Height + 1 - uint32(tvi) // travel a bit back
	expiry = standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)

	// Each tspend will spend 1/4 of all the funds expendable within a
	// treasury expenditure policy window.
	const nbTSpends = 4
	tspendAmount := dcrutil.Amount(expendBootstrap / nbTSpends)
	const tspendFee = 0
	tspends := make([]*wire.MsgTx, nbTSpends)
	for i := 0; i < nbTSpends; i++ {
		tspends[i] = g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
			{Amount: tspendAmount - tspendFee}}, tspendFee, expiry)
	}

	// Create a TADD to ensure they *DO NOT* count towards the policy.
	taddAmount := int64(1701)
	taddFee := dcrutil.Amount(0)
	tadd := g.CreateTreasuryTAdd(&outs[0], dcrutil.Amount(taddAmount), taddFee)

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the previously created tspends. A
	// tvi-worth of yes votes should be sufficient.
	//
	// ... -> bpretvin -> bv0 -> ... -> bvn
	// ---------------------------------------------------------------------
	for i := uint64(0); i < tvi*2; i++ {
		name := fmt.Sprintf("bv%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			tspends...))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// ---------------------------------------------------------------------
	// Generate a block that includes the tspends and a tadd.
	//
	// ... -> bvn -> btspends
	// ---------------------------------------------------------------------
	g.NextBlock("btspends", nil, outs[1:], func(b *wire.MsgBlock) {
		for _, tspend := range tspends {
			b.AddSTransaction(tspend)
		}
		b.AddSTransaction(tadd)
	})
	g.SaveTipCoinbaseOuts()
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
	for i := uint32(0); i < uint32(cbm+1); i++ {
		name := fmt.Sprintf("btsm%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// Ensure the balance equals everything received by the treasury minus
	// immature tbases and the just mined tspends.
	wantBalance = (tbaseBlocks-int64(cbm))*trsyBaseAmt +
		taddAmount - int64(tspendAmount)*nbTSpends
	g.ExpectTreasuryBalance(wantBalance)

	// Expiry for the next set of tspends.
	nextBlockHeight = g.Tip().Header.Height + 1 - uint32(tvi)
	expiry = standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)

	// We have spent the entire possible amount for the most recent
	// expenditure window. To assert that is true, create, approve and
	// attempt to mine a very small tspend.
	const smallTSpendAmount = 1
	const smallTSpendFee = 0
	smallTSpend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{Amount: smallTSpendAmount - smallTSpendFee}}, smallTSpendFee, expiry)

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the previously created small
	// tspend.  A tvi-worth of yes votes should be sufficient.
	//
	// ... -> btsmn -> bvv0 -> ... -> bvvn
	// ---------------------------------------------------------------------
	for i := uint64(0); i < tvi*2; i++ {
		name := fmt.Sprintf("bvv%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			smallTSpend))
		g.SaveTipCoinbaseOuts()
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
	g.NextBlock("bsmalltspendfail", nil, outs[1:], func(b *wire.MsgBlock) {
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
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			smallTSpend))
		g.SaveTipCoinbaseOuts()
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
	for i := 0; i < nbTests; i++ {
		tspends[i] = g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
			{Amount: dcrutil.Amount(testCases[i].amount) - tspendFee}},
			tspendFee, expiry)
	}

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the previously created tspends. A
	// tvi-worth of yes votes should be sufficient.
	//
	// ... -> btsmn -> bvvv0 -> ... -> bvvvn
	// ---------------------------------------------------------------------
	for i := uint64(0); i < tvi*2; i++ {
		name := fmt.Sprintf("bvvv%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			tspends...))
		g.SaveTipCoinbaseOuts()
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

		g.NextBlock(name, nil, outs[1:], func(b *wire.MsgBlock) {
			b.AddSTransaction(tspends[i])
		})
		if tc.wantAccept {
			g.SaveTipCoinbaseOuts()
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
	for i := uint32(0); i < uint32(cbm+1); i++ {
		name := fmt.Sprintf("btstm%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// Ensure the balance equals everything received by the treasury minus
	// immature tbases and the just mined tspends.
	wantBalance = (tbaseBlocks-int64(cbm))*trsyBaseAmt +
		taddAmount - int64(tspendAmount)*nbTSpends - testCases[nbTests-1].amount
	g.ExpectTreasuryBalance(wantBalance)

	// ---------------------------------------------------------------------
	// Generate enough blocks to advance until all previously mined tspends
	// have falled out of the policy check window and we revert back to the
	// bootstrap rate.
	//
	// ... -> btstmn -> ... -> btpolfinn
	// -------------------------------------------------------------------
	for i := uint32(0); i < uint32(tvi*mul*tew*(tep+1)); i++ {
		name := fmt.Sprintf("btpolfin%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// Create a final tspend that should be able to fetch the maximum
	// amount allowed by the bootstrap policy again.
	nextBlockHeight = g.Tip().Header.Height + 1 - uint32(tvi)
	expiry = standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	tspend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{Amount: dcrutil.Amount(expendBootstrap) - tspendFee}}, tspendFee,
		expiry)

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the previously created tspend. A
	// tvi-worth of yes votes should be sufficient.
	//
	// ... -> btpolfinn -> bvvvv0 -> ... -> bvvvvn
	// ---------------------------------------------------------------------
	for i := uint64(0); i < tvi*2; i++ {
		name := fmt.Sprintf("bvvvv%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(tspend))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// ---------------------------------------------------------------------
	// Generate a block that includes the final tspend
	//
	// ... -> bvvvn -> btfinaltspend
	// ---------------------------------------------------------------------
	g.NextBlock("btfinaltspend", nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tspend)
	})
	g.AcceptTipBlock()
}

// TestTSpendExpendituresPolicyDCP0007 performs tests against the treasury
// policy window rules. The following is the rough test plan:
//
//   - Approve 4 tspends that together spend the maximum allowed by policy and
//     a one atom tspend
//   - Mine a tadd
//   - Mine the 4 tspends that spend the maximum allowed by the policy
//   - Advance the chain until all the tspends are outside the policy check
//     range
//   - Attempt to spend the max allowed by policy again with one tspend
//   - Advance the chain until all the tspends are outside the policy check
//     range again
//   - Mine a one atom tspend
//   - Advance one tvi
//   - Mine a tspend that spends the max allowed by policy again (tests the fix
//     done by DCP0007)
func TestTSpendExpendituresPolicyDCP0007(t *testing.T) {
	t.Parallel()

	// Use a set of test chain parameters which allow for faster and more
	// efficient testing as compared to various existing network params and
	// significantly increase the SRI so that the treasurybase isn't reduced
	// throughout these tests to simplify the arithmetic.
	//
	// Also, mark the agendas the tests apply to as always active.
	const voteID1 = chaincfg.VoteIDTreasury
	const voteID2 = chaincfg.VoteIDRevertTreasuryPolicy
	params := quickVoteActivationParams()
	params.SubsidyReductionInterval = 1000
	forceDeploymentResult(t, params, voteID1, "yes")
	forceDeploymentResult(t, params, voteID2, "yes")

	// CoinbaseMaturity MUST be lower than TVI otherwise assumptions about
	// tspend maturity expenditure are broken. This is a sanity check to
	// ensure we're testing reasonable scenarios.
	if uint64(params.CoinbaseMaturity) > params.TreasuryVoteInterval {
		t.Fatal("params sanity check failed. CBM > TVI ")
	}

	// Shorter versions of useful params for convenience.
	tvi := params.TreasuryVoteInterval
	mul := params.TreasuryVoteIntervalMultiplier
	tew := params.TreasuryExpenditureWindow

	// Create a test harness initialized with the genesis block as the tip and
	// configure it to use the decentralized treasury semantics.
	g := newChaingenHarness(t, params)
	g.UseTreasurySemantics(chaingen.TSDCP0006)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()
	trsyBaseAmt := g.Tip().STransactions[0].TxIn[0].ValueIn

	// tbaseBlocks will keep track of how many blocks with a treasury base
	// have been added to the chain.
	blockHeight := uint64(g.Tip().Header.Height)
	tbaseBlocks := int64(blockHeight - 1)

	// ---------------------------------------------------------------------
	// Mine as many blocks as needed so that the treasury has enough funds
	// for our tests. Since we'll spend 150% of the "monthly" income of
	// treasury twice, we'll mine as many blocks as that 3 times.
	//
	// We'll also ensure we end just before a TVI block.
	//
	// ... -> bsv# -> bpretf0 -> ... -> bpretf#
	// ---------------------------------------------------------------------
	nbBlocks := tvi * mul * tew * 3
	if blockHeight%tvi != tvi-1 {
		nbBlocks += tvi - (blockHeight % tvi) - 1
	}
	outs := g.OldestCoinbaseOuts()
	for i := uint32(0); i < uint32(nbBlocks); i++ {
		name := fmt.Sprintf("bpretf%v", i)
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
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
	const nbTSpends = 4
	tbaseAmount := int64(tvi*mul*tew) * trsyBaseAmt
	taddAmount := nbTSpends*10 - tbaseAmount%nbTSpends
	incomeAmount := taddAmount + tbaseAmount
	tspendAmount := (incomeAmount + incomeAmount/2) / nbTSpends
	const tspendFee = 0
	tspends := make([]*wire.MsgTx, nbTSpends)
	for i := 0; i < nbTSpends; i++ {
		tspends[i] = g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
			{Amount: dcrutil.Amount(tspendAmount) - tspendFee}}, tspendFee,
			expiry)
	}

	// Also generate an additional, one atom tspend that will be approved.
	// This will be used to make assertions that the limit of expenditures
	// has not been breached.
	const smallTSpendAmount = 1
	const smallTSpendFee = 0
	smallTSpend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{Amount: smallTSpendAmount - smallTSpendFee}}, smallTSpendFee, expiry)

	// Create a TADD to ensure they *DO* count towards the maximum
	// expenditure policy.
	taddFee := dcrutil.Amount(0)
	tadd := g.CreateTreasuryTAdd(&outs[0], dcrutil.Amount(taddAmount), taddFee)

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the previously created tspends.
	// Two tvi-worth of yes votes should be sufficient. Stop just before
	// that so we can mine the TAdd.
	//
	// ... -> bpretvi# -> bv0 -> ... -> bv#
	// ---------------------------------------------------------------------
	for i := uint64(0); i < tvi*2-1; i++ {
		name := fmt.Sprintf("bv%v", i)
		g.NextBlock(name, nil, outs[1:],
			chaingen.AddTreasurySpendYesVotes(tspends...),
			chaingen.AddTreasurySpendYesVotes(smallTSpend))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// ---------------------------------------------------------------------
	// Generate a block that includes the tadd.
	//
	// ... -> bv# -> btadd
	// ---------------------------------------------------------------------
	g.NextBlock("btadd", nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(tadd)
	})
	g.SaveTipCoinbaseOuts()
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
	g.NextBlock("btspendserr", nil, outs[1:], func(b *wire.MsgBlock) {
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
	g.NextBlock("btspends", nil, outs[1:], func(b *wire.MsgBlock) {
		for _, tspend := range tspends {
			b.AddSTransaction(tspend)
		}
	})
	g.SaveTipCoinbaseOuts()
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
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
		tbaseBlocks++
	}

	// Ensure the balance equals everything received by the treasury minus
	// immature tbases and mined tspends.
	wantBalance := (tbaseBlocks-int64(params.CoinbaseMaturity))*trsyBaseAmt +
		taddAmount - tspendAmount*nbTSpends
	g.ExpectTreasuryBalance(wantBalance)

	// Generate a new tspend that spends the entire possible amount and one
	// that spends one atom. This time we don't create a tadd.
	nextBlockHeight = g.Tip().Header.Height + 1 - uint32(tvi)
	expiry = standalone.CalcTSpendExpiry(int64(nextBlockHeight), tvi, mul)
	tspendAmount = tbaseAmount + tbaseAmount/2
	largeTSpend := g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{Amount: dcrutil.Amount(tspendAmount) - tspendFee}}, tspendFee, expiry)

	smallTSpend = g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{Amount: smallTSpendAmount - smallTSpendFee}}, smallTSpendFee, expiry)

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the previously created tspends.
	// Two tvi-worth of yes votes should be sufficient.
	//
	// ... -> btsm# -> bvv0 -> ... -> bvv#
	// ---------------------------------------------------------------------
	for i := uint64(0); i < tvi*2; i++ {
		name := fmt.Sprintf("bvv%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			largeTSpend, smallTSpend))
		g.SaveTipCoinbaseOuts()
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
	g.NextBlock("btspendserr2", nil, outs[1:], func(b *wire.MsgBlock) {
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
	g.NextBlock("btspends2", nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(largeTSpend)
	})
	g.SaveTipCoinbaseOuts()
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
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
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
		{Amount: dcrutil.Amount(tspendAmount) - tspendFee}}, tspendFee, expiry)

	smallTSpend = g.CreateTreasuryTSpend(privKey, []chaingen.AddressAmountTuple{
		{Amount: smallTSpendAmount - smallTSpendFee}}, smallTSpendFee, expiry)

	// ---------------------------------------------------------------------
	// Generate enough blocks to approve the previously created tspends.
	// Two tvi-worth of yes votes should be sufficient.
	//
	// ... -> btsmm# -> bvvv0 -> ... -> bvvv#
	// ---------------------------------------------------------------------
	for i := uint64(0); i < tvi*2; i++ {
		name := fmt.Sprintf("bvvv%v", i)
		g.NextBlock(name, nil, outs[1:], chaingen.AddTreasurySpendYesVotes(
			largeTSpend, smallTSpend))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Include the small tspend.
	//
	// ... -> bvvv# -> btsmallts
	// ---------------------------------------------------------------------
	g.NextBlock("btsmallts", nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(smallTSpend)
	})
	g.SaveTipCoinbaseOuts()
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
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// ---------------------------------------------------------------------
	// Include the large tspend.
	//
	// ... -> btsmmm# -> btlargets
	// ---------------------------------------------------------------------
	g.NextBlock("btlargets", nil, outs[1:], func(b *wire.MsgBlock) {
		b.AddSTransaction(largeTSpend)
	})
	g.SaveTipCoinbaseOuts()
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
		g.NextBlock(name, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
		outs = g.OldestCoinbaseOuts()
	}

	// After mining the small and large tspends, the maximum allowed
	// expenditure shoud be zero.
	tipHash := &g.chain.BestSnapshot().Hash
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
