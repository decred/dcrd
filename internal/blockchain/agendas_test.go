// Copyright (c) 2017-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/blockchain/v5/chaingen"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/wire"
)

// testLNFeaturesDeployment ensures the deployment of the LN features agenda
// activates the expected changes for the provided network parameters.
func testLNFeaturesDeployment(t *testing.T, params *chaincfg.Params) {
	// baseConsensusScriptVerifyFlags are the expected script flags when the
	// agenda is not active.
	const baseConsensusScriptVerifyFlags = txscript.ScriptVerifyCleanStack |
		txscript.ScriptVerifyCheckLockTimeVerify

	// Clone the parameters so they can be mutated, find the correct deployment
	// for the LN features agenda as well as the yes vote choice within it, and,
	// finally, ensure it is always available to vote by removing the time
	// constraints to prevent test failures when the real expiration time
	// passes.
	const voteID = chaincfg.VoteIDLNFeatures
	params = cloneParams(params)
	deploymentVer, deployment := findDeployment(t, params, voteID)
	yesChoice := findDeploymentChoice(t, deployment, "yes")
	removeDeploymentTimeConstraints(deployment)

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
			bc.index.AddNode(node)
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
		gotActive, err = bc.IsLNFeaturesAgendaActive(&node.hash)
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
	testLNFeaturesDeployment(t, chaincfg.MainNetParams())
	testLNFeaturesDeployment(t, chaincfg.RegNetParams())
}

// TestFixedSequenceLocks ensures that sequence locks within blocks behave as
// expected.
func TestFixedSequenceLocks(t *testing.T) {
	// Use a set of test chain parameters which allow for quicker vote
	// activation as compared to various existing network params.
	params := quickVoteActivationParams()

	// Clone the parameters so they can be mutated.
	params = cloneParams(params)

	// Create a test harness initialized with the genesis block as the tip.
	g := newChaingenHarness(t, params)

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks to reach stake validation height.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()

	// ---------------------------------------------------------------------
	// Perform a series of sequence lock tests.
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
	b0 := g.NextBlock("b0", &outs[0], outs[1:], func(b *wire.MsgBlock) {
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
	g.NextBlock("b1a", nil, outs[1:], func(b *wire.MsgBlock) {
		spend := chaingen.MakeSpendableOut(b0, 1, 0)
		tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
		enableSeqLocks(tx, 0)
		b.AddTransaction(tx)
	})
	g.AcceptTipBlock()

	// ---------------------------------------------------------------------
	// Create block that involves reorganize to a sequence lock spending
	// from an output created in a block prior to the parent also spent on
	// the side chain.
	//
	//   ... -> b0 -> b1  -> b2
	//            \-> b1a
	// ---------------------------------------------------------------------
	g.SetTip("b0")
	g.NextBlock("b1", nil, outs[1:])
	g.SaveTipCoinbaseOuts()
	g.AcceptedToSideChainWithExpectedTip("b1a")

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b2", nil, outs[1:], func(b *wire.MsgBlock) {
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
	g.NextBlock("b3", nil, outs[1:], func(b *wire.MsgBlock) {
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
	g.NextBlock("b4", nil, outs[1:], func(b *wire.MsgBlock) {
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
	g.NextBlock("b5", nil, outs[1:], func(b *wire.MsgBlock) {
		spend := chaingen.MakeSpendableOut(b0, 1, 1)
		tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
		b.AddTransaction(tx)
	})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b6", nil, outs[1:], func(b *wire.MsgBlock) {
		spend := chaingen.MakeSpendableOut(b0, 1, 2)
		tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
		enableSeqLocks(tx, 0)
		b.AddTransaction(tx)
	})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	// ---------------------------------------------------------------------
	// Create block that involves a sequence lock spending from a regular
	// tree transaction earlier in the block.  This used to be rejected
	// due to a consensus bug, however the fix sequence locks agenda allows
	// it to be accepted as desired.
	//
	//   ... -> b6 -> b7
	// ---------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b7", &outs[0], outs[1:], func(b *wire.MsgBlock) {
		spend := chaingen.MakeSpendableOut(b, 1, 0)
		tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
		enableSeqLocks(tx, 0)
		b.AddTransaction(tx)
	})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	// ---------------------------------------------------------------------
	// Create block that involves a sequence lock spending from a block
	// prior to the parent.  This used to be rejected due to a consensus
	// bug, however the fix sequence locks agenda allows it to be accepted
	// as desired.
	//
	//   ... -> b6 -> b8 -> b9
	// ---------------------------------------------------------------------

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b8", nil, outs[1:])
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b9", nil, outs[1:], func(b *wire.MsgBlock) {
		spend := chaingen.MakeSpendableOut(b0, 1, 3)
		tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
		enableSeqLocks(tx, 0)
		b.AddTransaction(tx)
	})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

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
	g.NextBlock("b10", nil, outs[1:], func(b *wire.MsgBlock) {
		spend := chaingen.MakeSpendableOut(b0, 1, 4)
		tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
		b.AddTransaction(tx)
	})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b11", nil, outs[1:],
		g.ReplaceVoteBits(vbDisapprovePrev),
		func(b *wire.MsgBlock) {
			b.Header.VoteBits &^= vbApprovePrev
			spend := chaingen.MakeSpendableOut(b0, 1, 5)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			enableSeqLocks(tx, 0)
			b.AddTransaction(tx)
		})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()
}

// testHeaderCommitmentsDeployment ensures the deployment of the header
// commitments agenda activates for the provided network parameters.
func testHeaderCommitmentsDeployment(t *testing.T, params *chaincfg.Params) {
	// Clone the parameters so they can be mutated, find the correct deployment
	// for the header commitments agenda as well as the yes vote choice within
	// it, and, finally, ensure it is always available to vote by removing the
	// time constraints to prevent test failures when the real expiration time
	// passes.
	const voteID = chaincfg.VoteIDHeaderCommitments
	params = cloneParams(params)
	deploymentVer, deployment := findDeployment(t, params, voteID)
	yesChoice := findDeploymentChoice(t, deployment, "yes")
	removeDeploymentTimeConstraints(deployment)

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
			bc.index.AddNode(node)
			bc.bestChain.SetTip(node)
			curTimestamp = curTimestamp.Add(time.Second)
		}

		// Ensure the agenda reports the expected activation status for the
		// current block.
		gotActive, err := bc.isHeaderCommitmentsAgendaActive(node.parent)
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
		gotActive, err = bc.IsHeaderCommitmentsAgendaActive(&node.hash)
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

// TestHeaderCommitmentsDeployment ensures the deployment of the header
// commitments agenda activates as expected.
func TestHeaderCommitmentsDeployment(t *testing.T) {
	testHeaderCommitmentsDeployment(t, chaincfg.MainNetParams())
	testHeaderCommitmentsDeployment(t, chaincfg.RegNetParams())
}

// testTreasuryFeaturesDeployment ensures the deployment of the treasury
// features agenda activates the expected changes for the provided network
// parameters.
func testTreasuryFeaturesDeployment(t *testing.T, params *chaincfg.Params) {
	// baseConsensusScriptVerifyFlags are the expected script flags when the
	// agenda is not active.
	baseConsensusScriptVerifyFlags := txscript.ScriptVerifyCleanStack |
		txscript.ScriptVerifyCheckLockTimeVerify

	// Since some agendas are active by default on testnet, modify the expected
	// base script flags accordingly.
	if params.Net == wire.TestNet3 {
		baseConsensusScriptVerifyFlags |=
			txscript.ScriptVerifyCheckSequenceVerify |
				txscript.ScriptVerifySHA256
	}

	// Clone the parameters so they can be mutated, find the correct deployment
	// for the Treasury features agenda as well as the yes vote choice within
	// it, and, finally, ensure it is always available to vote by removing the
	// time constraints to prevent test failures when the real expiration time
	// passes.
	const voteID = chaincfg.VoteIDTreasury
	params = cloneParams(params)
	deploymentVer, deployment := findDeployment(t, params, voteID)
	yesChoice := findDeploymentChoice(t, deployment, "yes")
	removeDeploymentTimeConstraints(deployment)

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
				txscript.ScriptVerifyTreasury,
		},
		{
			name:       "one after active",
			numNodes:   1,
			curActive:  true,
			nextActive: true,
			expectedFlags: baseConsensusScriptVerifyFlags |
				txscript.ScriptVerifyTreasury,
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
			bc.index.AddNode(node)
			bc.bestChain.SetTip(node)
			curTimestamp = curTimestamp.Add(time.Second)
		}

		// Ensure the agenda reports the expected activation status for the
		// current block.
		gotActive, err := bc.isTreasuryAgendaActive(node.parent)
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
		gotActive, err = bc.IsTreasuryAgendaActive(&node.hash)
		if err != nil {
			t.Errorf("%s: unexpected err: %v", test.name, err)
			continue
		}
		if gotActive != test.nextActive {
			t.Errorf("%s: mismatched next active status - got: %v, want: %v",
				test.name, gotActive, test.nextActive)
			continue
		}

		// Ensure the consensus script verify flags are as expected.
		gotFlags, err := bc.consensusScriptVerifyFlags(node)
		if err != nil {
			t.Errorf("%s: unexpected err: %v", test.name, err)
			continue
		}
		if gotFlags != test.expectedFlags {
			t.Errorf("%s: mismatched flags - got %v, want %v", test.name,
				gotFlags, test.expectedFlags)
			continue
		}
	}
}

// TestTreasuryFeaturesDeployment ensures the deployment of the Treasury
// features agenda activate the expected changes.
func TestTreasuryFeaturesDeployment(t *testing.T) {
	testTreasuryFeaturesDeployment(t, chaincfg.MainNetParams())
	testTreasuryFeaturesDeployment(t, chaincfg.TestNet3Params())
	testTreasuryFeaturesDeployment(t, chaincfg.RegNetParams())
}

// testExplicitVerUpgradesDeployment ensures the deployment of the explicit
// version upgrades agenda activates for the provided network parameters.
func testExplicitVerUpgradesDeployment(t *testing.T, params *chaincfg.Params) {
	// Clone the parameters so they can be mutated, find the correct deployment
	// for the header commitments agenda as well as the yes vote choice within
	// it, and, finally, ensure it is always available to vote by removing the
	// time constraints to prevent test failures when the real expiration time
	// passes.
	const voteID = chaincfg.VoteIDExplicitVersionUpgrades
	params = cloneParams(params)
	deploymentVer, deployment := findDeployment(t, params, voteID)
	yesChoice := findDeploymentChoice(t, deployment, "yes")
	removeDeploymentTimeConstraints(deployment)

	// Shorter versions of params for convenience.
	stakeValidationHeight := uint32(params.StakeValidationHeight)
	ruleChangeActivationInterval := params.RuleChangeActivationInterval

	tests := []struct {
		name       string
		numNodes   uint32 // num fake nodes to create
		curActive  bool   // whether agenda active for current block
		nextActive bool   // whether agenda active for NEXT block
	}{{
		name:       "stake validation height",
		numNodes:   stakeValidationHeight,
		curActive:  false,
		nextActive: false,
	}, {
		name:       "started",
		numNodes:   ruleChangeActivationInterval,
		curActive:  false,
		nextActive: false,
	}, {
		name:       "lockedin",
		numNodes:   ruleChangeActivationInterval,
		curActive:  false,
		nextActive: false,
	}, {
		name:       "one before active",
		numNodes:   ruleChangeActivationInterval - 1,
		curActive:  false,
		nextActive: true,
	}, {
		name:       "exactly active",
		numNodes:   1,
		curActive:  true,
		nextActive: true,
	}, {
		name:       "one after active",
		numNodes:   1,
		curActive:  true,
		nextActive: true,
	}}

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
			bc.index.AddNode(node)
			bc.bestChain.SetTip(node)
			curTimestamp = curTimestamp.Add(time.Second)
		}

		// Ensure the agenda reports the expected activation status for the
		// current block.
		gotActive, err := bc.isExplicitVerUpgradesAgendaActive(node.parent)
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
		gotActive, err = bc.IsExplicitVerUpgradesAgendaActive(&node.hash)
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

// TestExplicitVerUpgradesDeployment ensures the deployment of the explicit
// version upgrades agenda activates as expected.
func TestExplicitVerUpgradesDeployment(t *testing.T) {
	testExplicitVerUpgradesDeployment(t, chaincfg.MainNetParams())
	testExplicitVerUpgradesDeployment(t, chaincfg.RegNetParams())
}

// testAutoRevocationsDeployment ensures the deployment of the automatic ticket
// revocations agenda activates for the provided network parameters.
func testAutoRevocationsDeployment(t *testing.T, params *chaincfg.Params) {
	// Clone the parameters so they can be mutated, find the correct deployment
	// for the automatic ticket revocations agenda as well as the yes vote choice
	// within it, and, finally, ensure it is always available to vote by removing
	// the time constraints to prevent test failures when the real expiration time
	// passes.
	const voteID = chaincfg.VoteIDAutoRevocations
	params = cloneParams(params)
	deploymentVer, deployment := findDeployment(t, params, voteID)
	yesChoice := findDeploymentChoice(t, deployment, "yes")
	removeDeploymentTimeConstraints(deployment)

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
			bc.index.AddNode(node)
			bc.bestChain.SetTip(node)
			curTimestamp = curTimestamp.Add(time.Second)
		}

		// Ensure the agenda reports the expected activation status for the
		// current block.
		gotActive, err := bc.isAutoRevocationsAgendaActive(node.parent)
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
		gotActive, err = bc.IsAutoRevocationsAgendaActive(&node.hash)
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

// TestAutoRevocationsDeployment ensures the deployment of the automatic ticket
// revocations agenda activates as expected.
func TestAutoRevocationsDeployment(t *testing.T) {
	testAutoRevocationsDeployment(t, chaincfg.MainNetParams())
	testAutoRevocationsDeployment(t, chaincfg.RegNetParams())
}

// testSubsidySplitDeployment ensures the deployment of the 10/80/10 subsidy
// split agenda activates for the provided network parameters.
func testSubsidySplitDeployment(t *testing.T, params *chaincfg.Params) {
	// Clone the parameters so they can be mutated, find the correct deployment
	// for the modified subsidy split agenda as well as the yes vote choice
	// within it, and, finally, ensure it is always available to vote by
	// removing the time constraints to prevent test failures when the real
	// expiration time passes.
	const voteID = chaincfg.VoteIDChangeSubsidySplit
	params = cloneParams(params)
	deploymentVer, deployment := findDeployment(t, params, voteID)
	yesChoice := findDeploymentChoice(t, deployment, "yes")
	removeDeploymentTimeConstraints(deployment)

	// Shorter versions of params for convenience.
	stakeValidationHeight := uint32(params.StakeValidationHeight)
	ruleChangeActivationInterval := params.RuleChangeActivationInterval

	tests := []struct {
		name       string
		numNodes   uint32 // num fake nodes to create
		curActive  bool   // whether agenda active for current block
		nextActive bool   // whether agenda active for NEXT block
	}{{
		name:       "stake validation height",
		numNodes:   stakeValidationHeight,
		curActive:  false,
		nextActive: false,
	}, {
		name:       "started",
		numNodes:   ruleChangeActivationInterval,
		curActive:  false,
		nextActive: false,
	}, {
		name:       "lockedin",
		numNodes:   ruleChangeActivationInterval,
		curActive:  false,
		nextActive: false,
	}, {
		name:       "one before active",
		numNodes:   ruleChangeActivationInterval - 1,
		curActive:  false,
		nextActive: true,
	}, {
		name:       "exactly active",
		numNodes:   1,
		curActive:  true,
		nextActive: true,
	}, {
		name:       "one after active",
		numNodes:   1,
		curActive:  true,
		nextActive: true,
	}}

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
			bc.index.AddNode(node)
			bc.bestChain.SetTip(node)
			curTimestamp = curTimestamp.Add(time.Second)
		}

		// Ensure the agenda reports the expected activation status for the
		// current block.
		gotActive, err := bc.isSubsidySplitAgendaActive(node.parent)
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
		gotActive, err = bc.IsSubsidySplitAgendaActive(&node.hash)
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

// TestSubsidySplitDeployment ensures the deployment of the 10/80/10 subsidy
// split agenda activates as expected.
func TestSubsidySplitDeployment(t *testing.T) {
	testSubsidySplitDeployment(t, chaincfg.MainNetParams())
	testSubsidySplitDeployment(t, chaincfg.RegNetParams())
}

// testBlake3PowDeployment ensures the deployment of the blake3 proof of work
// agenda activates for the provided network parameters.
func testBlake3PowDeployment(t *testing.T, params *chaincfg.Params) {
	// Clone the parameters so they can be mutated, find the correct deployment
	// for the modified subsidy split agenda as well as the yes vote choice
	// within it, and, finally, ensure it is always available to vote by
	// removing the time constraints to prevent test failures when the real
	// expiration time passes.
	const voteID = chaincfg.VoteIDBlake3Pow
	params = cloneParams(params)
	deploymentVer, deployment := findDeployment(t, params, voteID)
	yesChoice := findDeploymentChoice(t, deployment, "yes")
	removeDeploymentTimeConstraints(deployment)

	// Shorter versions of params for convenience.
	stakeValidationHeight := uint32(params.StakeValidationHeight)
	ruleChangeActivationInterval := params.RuleChangeActivationInterval

	// blake3AnchorHeight is the height of the expected blake3 anchor block
	// given the test conditions below.
	blake3AnchorHeight := int64(stakeValidationHeight +
		ruleChangeActivationInterval*3 - 1)

	tests := []struct {
		name        string
		numNodes    uint32 // num fake nodes to create
		curActive   bool   // whether agenda active for current block
		nextActive  bool   // whether agenda active for NEXT block
		resetAnchor bool   // whether or not to reset cached blake3 anchors
	}{{
		name:       "stake validation height",
		numNodes:   stakeValidationHeight,
		curActive:  false,
		nextActive: false,
	}, {
		name:       "started",
		numNodes:   ruleChangeActivationInterval,
		curActive:  false,
		nextActive: false,
	}, {
		name:       "lockedin",
		numNodes:   ruleChangeActivationInterval,
		curActive:  false,
		nextActive: false,
	}, {
		name:       "one before active",
		numNodes:   ruleChangeActivationInterval - 1,
		curActive:  false,
		nextActive: true,
	}, {
		name:       "exactly active",
		numNodes:   1,
		curActive:  true,
		nextActive: true,
	}, {
		name:       "one after active",
		numNodes:   1,
		curActive:  true,
		nextActive: true,
	}, {
		name:       "one before next rcai after active",
		numNodes:   ruleChangeActivationInterval - 2,
		curActive:  true,
		nextActive: true,
	}, {
		name:       "exactly next rcai after active",
		numNodes:   1,
		curActive:  true,
		nextActive: true,
	}, {
		name:       "one after next rcai after active",
		numNodes:   1,
		curActive:  true,
		nextActive: true,
	}, {
		name:        "one before 2nd rcai after active with anchor reset",
		numNodes:    ruleChangeActivationInterval - 2,
		curActive:   true,
		nextActive:  true,
		resetAnchor: true,
	}, {
		name:        "exactly 2nd rcai after active with anchor reset",
		numNodes:    1,
		curActive:   true,
		nextActive:  true,
		resetAnchor: true,
	}, {
		name:        "one after 2nd rcai after active with anchor reset",
		numNodes:    1,
		curActive:   true,
		nextActive:  true,
		resetAnchor: true,
	}}

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
			bc.index.AddNode(node)
			bc.bestChain.SetTip(node)
			curTimestamp = curTimestamp.Add(time.Second)
		}

		// Ensure the agenda reports the expected activation status for the
		// current block.
		gotActive, err := bc.isBlake3PowAgendaActive(node.parent)
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
		gotActive, err = bc.IsBlake3PowAgendaActive(&node.hash)
		if err != nil {
			t.Errorf("%s: unexpected err: %v", test.name, err)
			continue
		}
		if gotActive != test.nextActive {
			t.Errorf("%s: mismatched next active status - got: %v, want: %v",
				test.name, gotActive, test.nextActive)
			continue
		}

		// Reset the cached blake3 anchor block when requested by the test flag.
		// This helps ensure the logic that walks backwards to find the anchor
		// works as intended.
		if test.resetAnchor {
			bc.cachedBlake3WorkDiffCandidateAnchor.Store(nil)
			bc.cachedBlake3WorkDiffAnchor.Store(nil)
		}

		// Ensure the blake3 anchor block is the expected value once the agenda
		// is active.
		if test.nextActive {
			wantAnchor := bc.bestChain.nodeByHeight(blake3AnchorHeight)
			gotAnchor := bc.blake3WorkDiffAnchor(node)
			if gotAnchor != wantAnchor {
				t.Errorf("%s: mistmatched blake3 anchor - got: %s, want %s",
					test.name, gotAnchor, wantAnchor)
				continue
			}
		}
	}
}

// TestBlake3PowDeployment ensures the deployment of the blake3 proof of work
// agenda activates as expected.
func TestBlake3PowDeployment(t *testing.T) {
	testBlake3PowDeployment(t, chaincfg.MainNetParams())
	testBlake3PowDeployment(t, chaincfg.RegNetParams())
}

// testSubsidySplitR2Deployment ensures the deployment of the 1/89/10 subsidy
// split agenda activates for the provided network parameters.
func testSubsidySplitR2Deployment(t *testing.T, params *chaincfg.Params) {
	// Clone the parameters so they can be mutated, find the correct deployment
	// for the agenda as well as the yes vote choice within it, and, finally,
	// ensure it is always available to vote by removing the time constraints to
	// prevent test failures when the real expiration time passes.
	const voteID = chaincfg.VoteIDChangeSubsidySplitR2
	params = cloneParams(params)
	deploymentVer, deployment := findDeployment(t, params, voteID)
	yesChoice := findDeploymentChoice(t, deployment, "yes")
	removeDeploymentTimeConstraints(deployment)

	// Shorter versions of params for convenience.
	stakeValidationHeight := uint32(params.StakeValidationHeight)
	ruleChangeActivationInterval := params.RuleChangeActivationInterval

	tests := []struct {
		name       string
		numNodes   uint32 // num fake nodes to create
		curActive  bool   // whether agenda active for current block
		nextActive bool   // whether agenda active for NEXT block
	}{{
		name:       "stake validation height",
		numNodes:   stakeValidationHeight,
		curActive:  false,
		nextActive: false,
	}, {
		name:       "started",
		numNodes:   ruleChangeActivationInterval,
		curActive:  false,
		nextActive: false,
	}, {
		name:       "lockedin",
		numNodes:   ruleChangeActivationInterval,
		curActive:  false,
		nextActive: false,
	}, {
		name:       "one before active",
		numNodes:   ruleChangeActivationInterval - 1,
		curActive:  false,
		nextActive: true,
	}, {
		name:       "exactly active",
		numNodes:   1,
		curActive:  true,
		nextActive: true,
	}, {
		name:       "one after active",
		numNodes:   1,
		curActive:  true,
		nextActive: true,
	}}

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
			bc.index.AddNode(node)
			bc.bestChain.SetTip(node)
			curTimestamp = curTimestamp.Add(time.Second)
		}

		// Ensure the agenda reports the expected activation status for the
		// current block.
		gotActive, err := bc.isSubsidySplitR2AgendaActive(node.parent)
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
		gotActive, err = bc.IsSubsidySplitR2AgendaActive(&node.hash)
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

// TestSubsidySplitR2Deployment ensures the deployment of the 1/89/10 subsidy
// split agenda activates as expected.
func TestSubsidySplitR2Deployment(t *testing.T) {
	testSubsidySplitR2Deployment(t, chaincfg.MainNetParams())
	testSubsidySplitR2Deployment(t, chaincfg.RegNetParams())
}
