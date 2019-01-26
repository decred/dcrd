// Copyright (c) 2017-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/chaingen"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

// testLNFeaturesDeployment ensures the deployment of the LN features agenda
// activates the expected changes for the provided network parameters and
// expected deployment version.
func testLNFeaturesDeployment(t *testing.T, params *chaincfg.Params, deploymentVer uint32) {
	// baseConsensusScriptVerifyFlags are the expected script flags when the
	// agenda is not active.
	const baseConsensusScriptVerifyFlags = txscript.ScriptVerifyCleanStack |
		txscript.ScriptVerifyCheckLockTimeVerify

	// Find the correct deployment for the LN features agenda and ensure it
	// never expires to prevent test failures when the real expiration time
	// passes.  Also, clone the provided parameters first to avoid mutating
	// them.
	params = cloneParams(params)
	var deployment *chaincfg.ConsensusDeployment
	deployments := params.Deployments[deploymentVer]
	for deploymentID, depl := range deployments {
		if depl.Vote.Id == chaincfg.VoteIDLNFeatures {
			deployment = &deployments[deploymentID]
			break
		}
	}
	if deployment == nil {
		t.Fatalf("Unable to find consensus deployement for %s",
			chaincfg.VoteIDLNFeatures)
	}
	deployment.ExpireTime = math.MaxUint64 // Never expires.

	// Find the correct choice for the yes vote.
	const yesVoteID = "yes"
	var yesChoice chaincfg.Choice
	for _, choice := range deployment.Vote.Choices {
		if choice.Id == yesVoteID {
			yesChoice = choice
		}
	}
	if yesChoice.Id != yesVoteID {
		t.Fatalf("Unable to find vote choice for id %q", yesVoteID)
	}

	// Shorter versions of params for convenience.
	stakeValidationHeight := uint32(params.StakeValidationHeight)
	ruleChangeActivationInterval := params.RuleChangeActivationInterval

	tests := []struct {
		name          string
		numNodes      uint32 // num fake nodes to create
		curActive     bool   // whether agenda active for current block
		nextActive    bool   // whether agenda active for NEXT block
		expectedFlags txscript.ScriptFlags
	}{
		{
			name:          "stake validation height",
			numNodes:      stakeValidationHeight,
			curActive:     false,
			nextActive:    false,
			expectedFlags: baseConsensusScriptVerifyFlags,
		},
		{
			name:          "started",
			numNodes:      ruleChangeActivationInterval,
			curActive:     false,
			nextActive:    false,
			expectedFlags: baseConsensusScriptVerifyFlags,
		},
		{
			name:          "lockedin",
			numNodes:      ruleChangeActivationInterval,
			curActive:     false,
			nextActive:    false,
			expectedFlags: baseConsensusScriptVerifyFlags,
		},
		{
			name:          "one before active",
			numNodes:      ruleChangeActivationInterval - 1,
			curActive:     false,
			nextActive:    true,
			expectedFlags: baseConsensusScriptVerifyFlags,
		},
		{
			name:       "exactly active",
			numNodes:   1,
			curActive:  true,
			nextActive: true,
			expectedFlags: baseConsensusScriptVerifyFlags |
				txscript.ScriptVerifyCheckSequenceVerify |
				txscript.ScriptVerifySHA256,
		},
		{
			name:       "one after active",
			numNodes:   1,
			curActive:  true,
			nextActive: true,
			expectedFlags: baseConsensusScriptVerifyFlags |
				txscript.ScriptVerifyCheckSequenceVerify |
				txscript.ScriptVerifySHA256,
		},
	}

	curTimestamp := time.Now()
	bc := newFakeChain(params)
	node := bc.bestChain.Tip()
	for _, test := range tests {
		for i := uint32(0); i < test.numNodes; i++ {
			node = newFakeNode(node, int32(deploymentVer),
				deploymentVer, 0, curTimestamp)

			// Create fake votes that vote yes on the agenda to
			// ensure it is activated.
			for j := uint16(0); j < params.TicketsPerBlock; j++ {
				node.votes = append(node.votes, stake.VoteVersionTuple{
					Version: deploymentVer,
					Bits:    yesChoice.Bits | 0x01,
				})
			}
			bc.bestChain.SetTip(node)
			curTimestamp = curTimestamp.Add(time.Second)
		}

		// Ensure the agenda reports the expected activation status for
		// the current block.
		gotActive, err := bc.isLNFeaturesAgendaActive(node.parent)
		if err != nil {
			t.Errorf("%s: unexpected err: %v", test.name, err)
			continue
		}
		if gotActive != test.curActive {
			t.Errorf("%s: mismatched current active status - got: "+
				"%v, want: %v", test.name, gotActive,
				test.curActive)
			continue
		}

		// Ensure the agenda reports the expected activation status for
		// the NEXT block
		gotActive, err = bc.IsLNFeaturesAgendaActive()
		if err != nil {
			t.Errorf("%s: unexpected err: %v", test.name, err)
			continue
		}
		if gotActive != test.nextActive {
			t.Errorf("%s: mismatched next active status - got: %v, "+
				"want: %v", test.name, gotActive,
				test.nextActive)
			continue
		}

		// Ensure the consensus script verify flags are as expected.
		gotFlags, err := bc.consensusScriptVerifyFlags(node)
		if err != nil {
			t.Errorf("%s: unexpected err: %v", test.name, err)
			continue
		}
		if gotFlags != test.expectedFlags {
			t.Errorf("%s: mismatched flags - got %v, want %v",
				test.name, gotFlags, test.expectedFlags)
			continue
		}
	}
}

// TestLNFeaturesDeployment ensures the deployment of the LN features agenda
// activate the expected changes.
func TestLNFeaturesDeployment(t *testing.T) {
	testLNFeaturesDeployment(t, &chaincfg.MainNetParams, 5)
	testLNFeaturesDeployment(t, &chaincfg.RegNetParams, 6)
}

// testFixSeqLocksDeployment ensures the deployment of the fix sequence locks
// agenda activates for the provided network parameters and expected deployment
// version.
func testFixSeqLocksDeployment(t *testing.T, params *chaincfg.Params, deploymentVer uint32) {
	// Clone the parameters so they can be mutated, find the correct deployment
	// for the fix sequence locks agenda and ensure it is always available to
	// vote by removing the time constraints to prevent test failures when the
	// real expiration time passes.
	params = cloneParams(params)
	var deployment *chaincfg.ConsensusDeployment
	deployments := params.Deployments[deploymentVer]
	for deploymentID, depl := range deployments {
		if depl.Vote.Id == chaincfg.VoteIDFixLNSeqLocks {
			deployment = &deployments[deploymentID]
			break
		}
	}
	if deployment == nil {
		t.Fatalf("Unable to find consensus deployement for %s",
			chaincfg.VoteIDFixLNSeqLocks)
	}
	deployment.StartTime = 0               // Always available for vote.
	deployment.ExpireTime = math.MaxUint64 // Never expires.

	// Find the correct choice for the yes vote.
	const yesVoteID = "yes"
	var yesChoice *chaincfg.Choice
	for i, choice := range deployment.Vote.Choices {
		if choice.Id == yesVoteID {
			yesChoice = &deployment.Vote.Choices[i]
		}
	}
	if yesChoice.Id != yesVoteID {
		t.Fatalf("Unable to find vote choice for id %q", yesVoteID)
	}

	// Shorter versions of params for convenience.
	stakeValidationHeight := uint32(params.StakeValidationHeight)
	ruleChangeActivationInterval := params.RuleChangeActivationInterval

	tests := []struct {
		name       string
		numNodes   uint32 // num fake nodes to create
		curActive  bool   // whether agenda active for current block
		nextActive bool   // whether agenda active for NEXT block
	}{
		{
			name:       "stake validation height",
			numNodes:   stakeValidationHeight,
			curActive:  false,
			nextActive: false,
		},
		{
			name:       "started",
			numNodes:   ruleChangeActivationInterval,
			curActive:  false,
			nextActive: false,
		},
		{
			name:       "lockedin",
			numNodes:   ruleChangeActivationInterval,
			curActive:  false,
			nextActive: false,
		},
		{
			name:       "one before active",
			numNodes:   ruleChangeActivationInterval - 1,
			curActive:  false,
			nextActive: true,
		},
		{
			name:       "exactly active",
			numNodes:   1,
			curActive:  true,
			nextActive: true,
		},
		{
			name:       "one after active",
			numNodes:   1,
			curActive:  true,
			nextActive: true,
		},
	}

	curTimestamp := time.Now()
	bc := newFakeChain(params)
	node := bc.bestChain.Tip()
	for _, test := range tests {
		for i := uint32(0); i < test.numNodes; i++ {
			node = newFakeNode(node, int32(deploymentVer), deploymentVer, 0,
				curTimestamp)

			// Create fake votes that vote yes on the agenda to ensure it is
			// activated.
			for j := uint16(0); j < params.TicketsPerBlock; j++ {
				node.votes = append(node.votes, stake.VoteVersionTuple{
					Version: deploymentVer,
					Bits:    yesChoice.Bits | 0x01,
				})
			}
			bc.bestChain.SetTip(node)
			curTimestamp = curTimestamp.Add(time.Second)
		}

		// Ensure the agenda reports the expected activation status for the
		// current block.
		gotActive, err := bc.isFixSeqLocksAgendaActive(node.parent)
		if err != nil {
			t.Errorf("%s: unexpected err: %v", test.name, err)
			continue
		}
		if gotActive != test.curActive {
			t.Errorf("%s: mismatched current active status - got: %v, want: %v",
				test.name, gotActive, test.curActive)
			continue
		}

		// Ensure the agenda reports the expected activation status for the NEXT
		// block
		gotActive, err = bc.IsFixSeqLocksAgendaActive()
		if err != nil {
			t.Errorf("%s: unexpected err: %v", test.name, err)
			continue
		}
		if gotActive != test.nextActive {
			t.Errorf("%s: mismatched next active status - got: %v, want: %v",
				test.name, gotActive, test.nextActive)
			continue
		}
	}
}

// TestFixSeqLocksDeployment ensures the deployment of the fix sequence locks
// agenda activates as expected.
func TestFixSeqLocksDeployment(t *testing.T) {
	testFixSeqLocksDeployment(t, &chaincfg.MainNetParams, 6)
	testFixSeqLocksDeployment(t, &chaincfg.RegNetParams, 7)
}

// TestFixedSequenceLocks ensures that sequence locks within blocks behave as
// expected once the fix sequence locks agenda is active.
func TestFixedSequenceLocks(t *testing.T) {
	// Use a set of test chain parameters which allow for quicker vote
	// activation as compared to various existing network params.
	params := quickVoteActivationParams()

	// fslVersion is the deployment version of the fix sequence locks vote for
	// the chain params.
	const fslVersion = 7

	// Find the correct deployment for the LN features agenda.
	fslVoteID := chaincfg.VoteIDFixLNSeqLocks
	var deployment *chaincfg.ConsensusDeployment
	deployments := params.Deployments[fslVersion]
	for deploymentID, depl := range deployments {
		if depl.Vote.Id == fslVoteID {
			deployment = &deployments[deploymentID]
			break
		}
	}
	if deployment == nil {
		t.Fatalf("Unable to find consensus deployement for %s", fslVoteID)
	}

	// Find the correct choice for the yes vote.
	const yesVoteID = "yes"
	var fslYes *chaincfg.Choice
	for i, choice := range deployment.Vote.Choices {
		if choice.Id == yesVoteID {
			fslYes = &deployment.Vote.Choices[i]
		}
	}
	if fslYes == nil {
		t.Fatalf("Unable to find vote choice for id %q", yesVoteID)
	}

	// Create a test generator instance initialized with the genesis block as
	// the tip.
	g, err := chaingen.MakeGenerator(params)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("seqlocksoldsemanticstest", params)
	if err != nil {
		t.Fatalf("Failed to setup chain instance: %v", err)
	}
	defer teardownFunc()

	// accepted processes the current tip block associated with the generator
	// and expects it to be accepted to the main chain.
	//
	// expectTip expects the provided block to be the current tip of the
	// main chain.
	//
	// acceptedToSideChainWithExpectedTip expects the block to be accepted to a
	// side chain, but the current best chain tip to be the provided value.
	//
	// testThresholdState queries the threshold state from the current tip block
	// associated with the generator and expects the returned state to match the
	// provided value.
	accepted := func() {
		msgBlock := g.Tip()
		blockHeight := msgBlock.Header.Height
		block := dcrutil.NewBlock(msgBlock)
		t.Logf("Testing block %s (hash %s, height %d)", g.TipName(),
			block.Hash(), blockHeight)

		forkLen, isOrphan, err := chain.ProcessBlock(block, BFNone)
		if err != nil {
			t.Fatalf("block %q (hash %s, height %d) should have been "+
				"accepted: %v", g.TipName(), block.Hash(), blockHeight, err)
		}

		// Ensure the main chain and orphan flags match the values specified in
		// the test.
		isMainChain := !isOrphan && forkLen == 0
		if !isMainChain {
			t.Fatalf("block %q (hash %s, height %d) unexpected main chain "+
				"flag -- got %v, want true", g.TipName(), block.Hash(),
				blockHeight, isMainChain)
		}
		if isOrphan {
			t.Fatalf("block %q (hash %s, height %d) unexpected orphan flag -- "+
				"got %v, want false", g.TipName(), block.Hash(), blockHeight,
				isOrphan)
		}
	}
	expectTip := func(tipName string) {
		// Ensure hash and height match.
		wantTip := g.BlockByName(tipName)
		best := chain.BestSnapshot()
		if best.Hash != wantTip.BlockHash() ||
			best.Height != int64(wantTip.Header.Height) {
			t.Fatalf("block %q (hash %s, height %d) should be the current tip "+
				"-- got (hash %s, height %d)", tipName, wantTip.BlockHash(),
				wantTip.Header.Height, best.Hash, best.Height)
		}
	}
	acceptedToSideChainWithExpectedTip := func(tipName string) {
		msgBlock := g.Tip()
		blockHeight := msgBlock.Header.Height
		block := dcrutil.NewBlock(msgBlock)
		t.Logf("Testing block %s (hash %s, height %d)", g.TipName(),
			block.Hash(), blockHeight)

		forkLen, isOrphan, err := chain.ProcessBlock(block, BFNone)
		if err != nil {
			t.Fatalf("block %q (hash %s, height %d) should have been "+
				"accepted: %v", g.TipName(), block.Hash(), blockHeight, err)
		}

		// Ensure the main chain and orphan flags match the values specified in
		// the test.
		isMainChain := !isOrphan && forkLen == 0
		if isMainChain {
			t.Fatalf("block %q (hash %s, height %d) unexpected main chain "+
				"flag -- got %v, want false", g.TipName(), block.Hash(),
				blockHeight, isMainChain)
		}
		if isOrphan {
			t.Fatalf("block %q (hash %s, height %d) unexpected orphan flag -- "+
				"got %v, want false", g.TipName(), block.Hash(), blockHeight,
				isOrphan)
		}

		expectTip(tipName)
	}
	testThresholdState := func(id string, state ThresholdState) {
		tipHash := g.Tip().BlockHash()
		s, err := chain.NextThresholdState(&tipHash, fslVersion, id)
		if err != nil {
			t.Fatalf("block %q (hash %s, height %d) unexpected error when "+
				"retrieving threshold state: %v", g.TipName(), tipHash,
				g.Tip().Header.Height, err)
		}

		if s.State != state {
			t.Fatalf("block %q (hash %s, height %d) unexpected threshold "+
				"state for %s -- got %v, want %v", g.TipName(), tipHash,
				g.Tip().Header.Height, id, s.State, state)
		}
	}

	// replaceFixSeqLocksVersions is a munge function which modifies the
	// provided block by replacing the block, stake, and vote versions with the
	// fix sequence locks deployment version.
	replaceFixSeqLocksVersions := func(b *wire.MsgBlock) {
		chaingen.ReplaceBlockVersion(fslVersion)(b)
		chaingen.ReplaceStakeVersion(fslVersion)(b)
		chaingen.ReplaceVoteVersions(fslVersion)(b)
	}

	// Shorter versions of useful params for convenience.
	ticketsPerBlock := int64(params.TicketsPerBlock)
	coinbaseMaturity := params.CoinbaseMaturity
	stakeEnabledHeight := params.StakeEnabledHeight
	stakeValidationHeight := params.StakeValidationHeight
	stakeVerInterval := params.StakeVersionInterval
	ruleChangeInterval := int64(params.RuleChangeActivationInterval)

	// ---------------------------------------------------------------------
	// First block.
	// ---------------------------------------------------------------------

	// Add the required first block.
	//
	//   genesis -> bp
	g.CreatePremineBlock("bp", 0)
	g.AssertTipHeight(1)
	accepted()

	// ---------------------------------------------------------------------
	// Generate enough blocks to have mature coinbase outputs to work with.
	//
	//   genesis -> bp -> bm0 -> bm1 -> ... -> bm#
	// ---------------------------------------------------------------------

	for i := uint16(0); i < coinbaseMaturity; i++ {
		blockName := fmt.Sprintf("bm%d", i)
		g.NextBlock(blockName, nil, nil)
		g.SaveTipCoinbaseOuts()
		accepted()
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
		accepted()
	}
	g.AssertTipHeight(uint32(stakeEnabledHeight))

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake validation height while
	// continuing to purchase tickets using the coinbases matured above and
	// allowing the immature tickets to mature and thus become live.
	//
	// The blocks are also generated with the deployment version to ensure
	// stake version and fix sequence locks enforcement is reached.
	//
	//   ... -> bse# -> bsv0 -> bsv1 -> ... -> bsv#
	// ---------------------------------------------------------------------

	targetPoolSize := int64(g.Params().TicketPoolSize) * ticketsPerBlock
	for i := int64(0); int64(g.Tip().Header.Height) < stakeValidationHeight; i++ {
		// Only purchase tickets until the target ticket pool size is reached.
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
		g.NextBlock(blockName, nil, ticketOuts,
			chaingen.ReplaceBlockVersion(fslVersion))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight))

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next two stake
	// version intervals with block and vote versions for the fix sequence
	// locks agenda and stake version 0.
	//
	// This will result in triggering enforcement of the stake version and
	// that the stake version is the fix seqence locks version.  The
	// threshold state for deployment will move to started since the next
	// block also coincides with the start of a new rule change activation
	// interval for the chosen parameters.
	//
	//   ... -> bsv# -> bvu0 -> bvu1 -> ... -> bvu#
	// ---------------------------------------------------------------------

	// Two stake versions intervals are needed since the first one is required
	// to activate initial stake version enforcement while the second upgrades
	// to the desired version which, in conjunction with the PoW upgrade via the
	// block version allows the vote to start at the next rule change interval.
	blocksNeeded := stakeValidationHeight + stakeVerInterval*2 - 1 -
		int64(g.Tip().Header.Height)
	for i := int64(0); i < blocksNeeded; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bvu%d", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(fslVersion),
			chaingen.ReplaceVoteVersions(fslVersion))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	testThresholdState(fslVoteID, ThresholdStarted)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the next rule change interval with
	// block, stake, and vote versions for the fix sequence locks agenda.
	// Also, set the vote bits to include yes votes for the agenda.
	//
	// This will result in moving the threshold state for the fix sequence
	// locks agenda to locked in.
	//
	//   ... -> bvu# -> bvtli0 -> bvtli1 -> ... -> bvtli#
	// ---------------------------------------------------------------------

	for i := int64(0); i < ruleChangeInterval; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bvtli%d", i)
		g.NextBlock(blockName, nil, outs[1:], replaceFixSeqLocksVersions,
			chaingen.ReplaceVotes(vbPrevBlockValid|fslYes.Bits, fslVersion))
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertBlockVersion(fslVersion)
	g.AssertStakeVersion(fslVersion)
	testThresholdState(fslVoteID, ThresholdLockedIn)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the next rule change interval with
	// block, stake, and vote versions for the fix sequence locks agenda.
	//
	// This will result in moving the threshold state for the fix sequence
	// lock agenda to active thereby activating it.
	//
	//   ... -> bvtli# -> bvta0 -> bvta1 -> ... -> bvta#
	// ---------------------------------------------------------------------

	for i := int64(0); i < ruleChangeInterval; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bvta%d", i)
		g.NextBlock(blockName, nil, outs[1:], replaceFixSeqLocksVersions)
		g.SaveTipCoinbaseOuts()
		accepted()
	}
	g.AssertBlockVersion(fslVersion)
	g.AssertStakeVersion(fslVersion)
	testThresholdState(fslVoteID, ThresholdActive)

	// ---------------------------------------------------------------------
	// Perform a series of sequence lock tests now that fix sequence locks
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
	b0 := g.NextBlock("b0", &outs[0], outs[1:], replaceFixSeqLocksVersions,
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
				tx.AddTxOut(wire.NewTxOut(int64(amount), origOut.PkScript))
			}

			// Add the original op return back to the outputs and enable
			// sequence locks for the first output.
			tx.AddTxOut(origOpReturnOut)
			enableSeqLocks(tx, 0)
		})
	g.SaveTipCoinbaseOuts()
	accepted()

	// ---------------------------------------------------------------------
	// Create block that spends from an output created in the previous
	// block.
	//
	//   ... -> b0 -> b1a
	// ---------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b1a", nil, outs[1:], replaceFixSeqLocksVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b0, 1, 0)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			enableSeqLocks(tx, 0)
			b.AddTransaction(tx)
		})
	accepted()

	// ---------------------------------------------------------------------
	// Create block that involves reorganize to a sequence lock spending
	// from an output created in a block prior to the parent also spent on
	// on the side chain.
	//
	//   ... -> b0 -> b1  -> b2
	//            \-> b1a
	// ---------------------------------------------------------------------
	g.SetTip("b0")
	g.NextBlock("b1", nil, outs[1:], replaceFixSeqLocksVersions)
	g.SaveTipCoinbaseOuts()
	acceptedToSideChainWithExpectedTip("b1a")

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b2", nil, outs[1:], replaceFixSeqLocksVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b0, 1, 0)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			enableSeqLocks(tx, 0)
			b.AddTransaction(tx)
		})
	g.SaveTipCoinbaseOuts()
	accepted()
	expectTip("b2")

	// ---------------------------------------------------------------------
	// Create block that involves a sequence lock on a vote.
	//
	//   ... -> b2 -> b3
	// ---------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b3", nil, outs[1:], replaceFixSeqLocksVersions,
		func(b *wire.MsgBlock) {
			enableSeqLocks(b.STransactions[0], 0)
		})
	g.SaveTipCoinbaseOuts()
	accepted()

	// ---------------------------------------------------------------------
	// Create block that involves a sequence lock on a ticket.
	//
	//   ... -> b3 -> b4
	// ---------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b4", nil, outs[1:], replaceFixSeqLocksVersions,
		func(b *wire.MsgBlock) {
			enableSeqLocks(b.STransactions[5], 0)
		})
	g.SaveTipCoinbaseOuts()
	accepted()

	// ---------------------------------------------------------------------
	// Create two blocks such that the tip block involves a sequence lock
	// spending from a different output of a transaction the parent block
	// also spends from.
	//
	//   ... -> b4 -> b5 -> b6
	// ---------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b5", nil, outs[1:], replaceFixSeqLocksVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b0, 1, 1)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			b.AddTransaction(tx)
		})
	g.SaveTipCoinbaseOuts()
	accepted()

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b6", nil, outs[1:], replaceFixSeqLocksVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b0, 1, 2)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			enableSeqLocks(tx, 0)
			b.AddTransaction(tx)
		})
	g.SaveTipCoinbaseOuts()
	accepted()

	// ---------------------------------------------------------------------
	// Create block that involves a sequence lock spending from a regular
	// tree transaction earlier in the block.  This used to be rejected
	// due to a consensus bug, however the fix sequence locks agenda allows
	// it to be accepted as desired.
	//
	//   ... -> b6 -> b7
	// ---------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b7", &outs[0], outs[1:], replaceFixSeqLocksVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b, 1, 0)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			enableSeqLocks(tx, 0)
			b.AddTransaction(tx)
		})
	g.SaveTipCoinbaseOuts()
	accepted()

	// ---------------------------------------------------------------------
	// Create block that involves a sequence lock spending from a block
	// prior to the parent.  This used to be rejected due to a consensus
	// bug, however the fix sequence locks agenda allows it to be accepted
	// as desired.
	//
	//   ... -> b6 -> b8 -> b9
	// ---------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b8", nil, outs[1:], replaceFixSeqLocksVersions)
	g.SaveTipCoinbaseOuts()
	accepted()

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b9", nil, outs[1:], replaceFixSeqLocksVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b0, 1, 3)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			enableSeqLocks(tx, 0)
			b.AddTransaction(tx)
		})
	g.SaveTipCoinbaseOuts()
	accepted()

	// ---------------------------------------------------------------------
	// Create two blocks such that the tip block involves a sequence lock
	// spending from a different output of a transaction the parent block
	// also spends from when the parent block has been disapproved.  This
	// used to be rejected due to a consensus bug, however the fix sequence
	// locks agenda allows it to be accepted as desired.
	//
	//   ... -> b8 -> b10 -> b11
	// ---------------------------------------------------------------------

	const (
		// vbDisapprovePrev and vbApprovePrev represent no and yes votes,
		// respectively, on whether or not to approve the previous block.
		vbDisapprovePrev = 0x0000
		vbApprovePrev    = 0x0001
	)

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b10", nil, outs[1:], replaceFixSeqLocksVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b0, 1, 4)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			b.AddTransaction(tx)
		})
	g.SaveTipCoinbaseOuts()
	accepted()

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b11", nil, outs[1:], replaceFixSeqLocksVersions,
		chaingen.ReplaceVotes(vbDisapprovePrev, fslVersion),
		func(b *wire.MsgBlock) {
			b.Header.VoteBits &^= vbApprovePrev
			spend := chaingen.MakeSpendableOut(b0, 1, 5)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			enableSeqLocks(tx, 0)
			b.AddTransaction(tx)
		})
	g.SaveTipCoinbaseOuts()
	accepted()
}
