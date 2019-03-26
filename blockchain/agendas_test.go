// Copyright (c) 2017-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
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
	params = cloneParams(params)
	deploymentVer, deployment, err := findDeployment(params,
		chaincfg.VoteIDLNFeatures)
	if err != nil {
		t.Fatal(err)
	}
	yesChoice, err := findDeploymentChoice(deployment, "yes")
	if err != nil {
		t.Fatal(err)
	}
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
	testLNFeaturesDeployment(t, &chaincfg.MainNetParams)
	testLNFeaturesDeployment(t, &chaincfg.RegNetParams)
}

// testFixSeqLocksDeployment ensures the deployment of the fix sequence locks
// agenda activates for the provided network parameters.
func testFixSeqLocksDeployment(t *testing.T, params *chaincfg.Params) {
	// Clone the parameters so they can be mutated, find the correct deployment
	// for the fix sequence locks agenda as well as the yes vote choice within
	// it, and, finally, ensure it is always available to vote by removing the
	// time constraints to prevent test failures when the real expiration time
	// passes.
	params = cloneParams(params)
	deploymentVer, deployment, err := findDeployment(params,
		chaincfg.VoteIDFixLNSeqLocks)
	if err != nil {
		t.Fatal(err)
	}
	yesChoice, err := findDeploymentChoice(deployment, "yes")
	if err != nil {
		t.Fatal(err)
	}
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
	testFixSeqLocksDeployment(t, &chaincfg.MainNetParams)
	testFixSeqLocksDeployment(t, &chaincfg.RegNetParams)
}

// TestFixedSequenceLocks ensures that sequence locks within blocks behave as
// expected once the fix sequence locks agenda is active.
func TestFixedSequenceLocks(t *testing.T) {
	// Use a set of test chain parameters which allow for quicker vote
	// activation as compared to various existing network params.
	params := quickVoteActivationParams()

	// Clone the parameters so they can be mutated, find the correct deployment
	// for the fix sequence locks agenda, and, finally, ensure it is always
	// available to vote by removing the time constraints to prevent test
	// failures when the real expiration time passes.
	const fslVoteID = chaincfg.VoteIDFixLNSeqLocks
	params = cloneParams(params)
	fslVersion, deployment, err := findDeployment(params, fslVoteID)
	if err != nil {
		t.Fatal(err)
	}
	removeDeploymentTimeConstraints(deployment)

	// Create a test harness initialized with the genesis block as the tip.
	g, teardownFunc := newChaingenHarness(t, params, "fixseqlockstest")
	defer teardownFunc()

	// replaceFixSeqLocksVersions is a munge function which modifies the
	// provided block by replacing the block, stake, and vote versions with the
	// fix sequence locks deployment version.
	replaceFixSeqLocksVersions := func(b *wire.MsgBlock) {
		chaingen.ReplaceBlockVersion(int32(fslVersion))(b)
		chaingen.ReplaceStakeVersion(fslVersion)(b)
		chaingen.ReplaceVoteVersions(fslVersion)(b)
	}

	// ---------------------------------------------------------------------
	// Generate and accept enough blocks with the appropriate vote bits set
	// to reach one block prior to the fix sequence locks agenda becoming
	// active.
	// ---------------------------------------------------------------------

	g.AdvanceToStakeValidationHeight()
	g.AdvanceFromSVHToActiveAgenda(fslVoteID)

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
	g.NextBlock("b1a", nil, outs[1:], replaceFixSeqLocksVersions,
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
	g.NextBlock("b1", nil, outs[1:], replaceFixSeqLocksVersions)
	g.SaveTipCoinbaseOuts()
	g.AcceptedToSideChainWithExpectedTip("b1a")

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b2", nil, outs[1:], replaceFixSeqLocksVersions,
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
	g.NextBlock("b3", nil, outs[1:], replaceFixSeqLocksVersions,
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
	g.NextBlock("b4", nil, outs[1:], replaceFixSeqLocksVersions,
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
	g.NextBlock("b5", nil, outs[1:], replaceFixSeqLocksVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b0, 1, 1)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			b.AddTransaction(tx)
		})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b6", nil, outs[1:], replaceFixSeqLocksVersions,
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
	g.NextBlock("b8", nil, outs[1:], replaceFixSeqLocksVersions)
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

	outs = g.OldestCoinbaseOuts()
	g.NextBlock("b9", nil, outs[1:], replaceFixSeqLocksVersions,
		func(b *wire.MsgBlock) {
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
	g.NextBlock("b10", nil, outs[1:], replaceFixSeqLocksVersions,
		func(b *wire.MsgBlock) {
			spend := chaingen.MakeSpendableOut(b0, 1, 4)
			tx := g.CreateSpendTx(&spend, dcrutil.Amount(1))
			b.AddTransaction(tx)
		})
	g.SaveTipCoinbaseOuts()
	g.AcceptTipBlock()

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
	g.AcceptTipBlock()
}
