// Copyright (c) 2017-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math"
	"math/bits"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/chaincfg/v3"
)

// TestVoting ensure the overall voting of an agenda works as expected including
// a wide variety of conditions.  Some examples include tests for multiple
// concurrent votes, tests for all edge cases between the stake version and rule
// change intervals, and tests for proof of work and proof of stake vesion
// upgrade semantics.
func TestVoting(t *testing.T) {
	t.Parallel()

	// Base versions used for the mock parameters and tests.
	const (
		posVersion = uint32(4)
		powVersion = int32(4)
	)

	// Create parameters with known mocked values for use as a base to mutate in
	// the tests below.
	mockParams := cloneParams(chaincfg.RegNetParams())
	mockParams.Deployments = map[uint32][]chaincfg.ConsensusDeployment{
		posVersion: {{
			Vote: chaincfg.Vote{
				Id:          "vote1",
				Description: "Vote 1",
				Mask:        0x6, // 0b0110
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsAbstain:   true,
				}, {
					Id:          "no",
					Description: "vote no",
					Bits:        0x0002, // Bit 1 (1 << 1)
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "vote yes",
					Bits:        0x0004, // Bit 2 (2 << 1)
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}, {
			Vote: chaincfg.Vote{
				Id:          "vote2",
				Description: "Vote 2",
				Mask:        0x18, // 0b11000
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsAbstain:   true,
				}, {
					Id:          "no",
					Description: "vote no",
					Bits:        0x0008, // Bit 3 (1 << 3)
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "vote yes",
					Bits:        0x0010, // Bit 4 (2 << 3)
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}, {
			Vote: chaincfg.Vote{
				Id:          "multiplechoice",
				Description: "Pick one",
				Mask:        0xe0, // 0b11100000
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain multiple choice",
					Bits:        0x0,
					IsAbstain:   true,
				}, {
					Id:          "vote against",
					Description: "vote against all multiple",
					Bits:        0x20, // Bit 5 (1 << 5)
					IsNo:        true,
				}, {
					Id:          "one",
					Description: "Choice 1",
					Bits:        0x40, // Bit 6 (2 << 5)
				}, {
					Id:          "two",
					Description: "Choice 2",
					Bits:        0x60, // Bits 5 and 6 (3 << 5)
				}, {
					Id:          "three",
					Description: "Choice 3",
					Bits:        0x80, // Bit 7 (4 << 5)
				}, {
					Id:          "four",
					Description: "Choice 4",
					Bits:        0xa0, // Bits 5 and 7 (5 << 5)
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}},
	}

	// Shorter versions of useful params for convenience.
	ruleChangeInterval := mockParams.RuleChangeActivationInterval
	svh := uint32(mockParams.StakeValidationHeight)
	stakeVerInterval := uint32(mockParams.StakeVersionInterval)
	rciDivSvi := ruleChangeInterval / stakeVerInterval
	rciModSvi := ruleChangeInterval % stakeVerInterval
	ruleChangeQuorum := mockParams.RuleChangeActivationQuorum
	ruleChangeMult := mockParams.RuleChangeActivationMultiplier
	ruleChangeDiv := mockParams.RuleChangeActivationDivisor

	// Assert requirements for the tests.
	if rciModSvi == 0 {
		t.Fatalf("These tests require a rule change interval that is not a "+
			"multiple of the stake version interval (rci %d, svi %d)",
			ruleChangeInterval, stakeVerInterval)
	}

	// Convenient references to the mock parameter votes and choices.
	const (
		vote1NoIdx      = 1
		vote1YesIdx     = 2
		vote2NoIdx      = 1
		vote2YesIdx     = 2
		vote3NoIdx      = 1
		vote3Choice1Idx = 2
		vote3Choice2Idx = 3
		vote3Choice3Idx = 4
		vote3Choice4Idx = 5
	)
	vote1 := &mockParams.Deployments[posVersion][0].Vote
	vote1No := &vote1.Choices[vote1NoIdx]
	vote1Yes := &vote1.Choices[vote1YesIdx]
	vote2 := &mockParams.Deployments[posVersion][1].Vote
	vote2No := &vote2.Choices[vote2NoIdx]
	vote2Yes := &vote2.Choices[vote2YesIdx]
	vote3 := &mockParams.Deployments[posVersion][2].Vote
	vote3No := &vote3.Choices[vote3NoIdx]
	vote3Choice1 := &vote3.Choices[vote3Choice1Idx]
	vote3Choice2 := &vote3.Choices[vote3Choice2Idx]
	vote3Choice3 := &vote3.Choices[vote3Choice3Idx]
	vote3Choice4 := &vote3.Choices[vote3Choice4Idx]

	// Determine what the vote bits for the next choice in the various votes
	// would be if they existed.
	vote1ChoiceIdxShift := bits.TrailingZeros16(vote1.Mask)
	vbVote1Invalid := uint16(len(vote1.Choices) << vote1ChoiceIdxShift)
	vote2ChoiceIdxShift := bits.TrailingZeros16(vote2.Mask)
	vbVote2Invalid := uint16(len(vote2.Choices) << vote2ChoiceIdxShift)
	vote3ChoiceIdxShift := bits.TrailingZeros16(vote3.Mask)
	vbVote3Invalid := uint16(len(vote3.Choices) << vote3ChoiceIdxShift)

	// Calculate the minimum required possible votes to meet quorum with a
	// majority.
	quorumMajority := ruleChangeQuorum * ruleChangeMult / ruleChangeDiv

	// perInstanceTest describes a test to run against the same chain instance.
	type wantStatesMap map[string]ThresholdStateTuple
	type perInstanceTest struct {
		name        string           // test description
		voteVersion uint32           // version for fake votes
		voteBits    uint16           // vote bits for fake votes
		munger      func(*blockNode) // func to mutate fake block node
		numBlocks   uint32           // num fake blocks to create
		wantStates  wantStatesMap    // expected threshold states
	}

	tests := []struct {
		name             string            // test description
		expireOffset     int64             // optional vote expire offset
		blockVersion     int32             // block version for fake blocks
		stakeVersion     uint32            // stake version for fake blocks
		perInstanceTests []perInstanceTest // tests to run against same instance
	}{{
		name:         "no voting prior to svh",
		blockVersion: powVersion,
		stakeVersion: posVersion,
		perInstanceTests: []perInstanceTest{{
			name:        "one block prior to svh",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   svh - 2,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}},
	}, {
		// Ensure voting does not start until a rule change interval is reached
		// after the majority stake version has upgraded.
		//
		// Then go on to test normal voting results per:
		//
		// Vote 1: 100% yes
		// Vote 2: 100% no
		// Vote 3: 100% abstain
		name:         "PoS upgrade with different parallel outcomes",
		blockVersion: powVersion,
		stakeVersion: posVersion - 1,
		perInstanceTests: []perInstanceTest{{
			name:      "svh",
			numBlocks: svh - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast votes with a newer stake version for a full stake version
			// interval to achieve a majority stake version upgrade.  Even
			// though a majority has been reached, the state should remain
			// started because it can only change on a rule change interval.
			name:        "stake version interval",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   stakeVerInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Continue casting normal votes to reach the next rule change
			// interval.  The state should move to started since the stake
			// version upgrade was achieved.
			name:        "rule change interval",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval - stakeVerInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast votes: 100% yes for vote 1, 100% no for vote 2, and 100%
			// abstain for vote 3.  The state should move to locked in for vote
			// 1 since it passed, failed for vote 2, and remain in started for
			// vote 3 since the vote is still ongoing.
			name:        "rule change interval 2",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid | vote1Yes.Bits | vote2No.Bits,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdLockedIn, vote1YesIdx),
				vote2.Id: newThresholdState(ThresholdFailed, vote2NoIdx),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast normal votes for another rule change interval.  The state
			// should move to active for vote 1, remain as failed for vote 2,
			// and remain in started for vote 3 since the vote is still ongoing.
			name:        "rule change interval 3",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdActive, vote1YesIdx),
				vote2.Id: newThresholdState(ThresholdFailed, vote2NoIdx),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}},
	}, {
		// Ensure voting does not start on a rule change interval in the middle
		// of a stake version interval even if there is already enough to
		// achieve a majority since the stake version interval hasn't completed
		// yet and thus that majority doesn't count yet.
		//
		// Then go on to test normal voting results per:
		//
		// Vote 1: 100% abstain
		// Vote 2: 100% yes
		// Vote 3: 100% no
		name:         "delayed PoS upgrade with different parallel outcomes",
		blockVersion: powVersion,
		stakeVersion: posVersion - 1,
		perInstanceTests: []perInstanceTest{{
			name:      "svh",
			numBlocks: svh - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast votes with an older stake version for as many full stake
			// version intervals that are possible prior to a rule change
			// interval so a majority stake version upgrade is not achieved.
			// The state should remain in defined since achieving a majority
			// stake upgrade is one of the prerequisites for a vote to start.
			name:        "stake version interval 2",
			voteVersion: posVersion - 1,
			voteBits:    vbPrevBlockValid,
			numBlocks:   rciDivSvi * stakeVerInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Reach the next rule change interval while casting votes with the
			// newer stake version such that there are enough to theoretically
			// qualify for a majority stake version upgrade, but is still in the
			// middle of the stake version interval.  The state should remain in
			// defined since achieving a stake upgrade is one of the
			// prerequisites for a vote to start and the stake upgrade does not
			// happen until the end of the stake version interval despite there
			// already technically being a majority at this point.
			name:        "rule change interval",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   rciModSvi,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Reach the next stake version interval while continuing to cast
			// votes with the newer stake version to achieve a majority stake
			// version upgrade.  Even though the upgrade has been achieved, the
			// state should remain in defined because it can only change on a
			// rule change interval.
			name:        "stake version interval 3",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   stakeVerInterval - rciModSvi,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Continue casting normal votes to reach the next rule change
			// interval.  The state should move to started since the stake
			// upgrade was achieved.
			name:        "rule change interval 2",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval - (stakeVerInterval - rciModSvi),
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast votes: 100% abstain for vote 1, 100% yes for vote 2, and
			// 100% no for vote 3.  The state should remain in started for vote
			// 1 since the vote is still ongoing, move to locked in for vote 2
			// since it passed, and failed for vote 3.
			name:        "rule change interval 3",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid | vote2Yes.Bits | vote3No.Bits,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdLockedIn, vote2YesIdx),
				vote3.Id: newThresholdState(ThresholdFailed, vote3NoIdx),
			},
		}, {
			// Cast normal votes for another rule change interval.  The state
			// should remain in started for vote 1, move to active for vote 2,
			// and remain as failed for vote 3.
			name:        "rule change interval 4",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdActive, vote2YesIdx),
				vote3.Id: newThresholdState(ThresholdFailed, vote3NoIdx),
			},
		}},
	}, {
		// Ensure votes on ongoing deployments that haven't expired yet continue
		// to count even if the majority stake version has upgraded to newer
		// vesions than the version that defined the deployment.
		//
		// Also test normal voting results per:
		//
		// Vote 1: 100% no
		// Vote 2: 100% abstain
		// Vote 3: 100% yes with choice 1
		name:         "votes on undecided older deployment versions count",
		blockVersion: powVersion,
		stakeVersion: posVersion + 1,
		perInstanceTests: []perInstanceTest{{
			name:      "svh",
			numBlocks: svh - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast normal votes with the stake version that matches the
			// deployment but is older than the version the majority has
			// upgraded to in order to reach the next rule change interval. The
			// state should move to started since the required stake version
			// upgrade was already previously achieved.
			name:        "rule change interval",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast votes: 100% no for vote 1, 100% abstain for vote 2, and 100%
			// choice 1 for vote 3.  The state should move to failed for vote 1,
			// remain in started for vote 2 since it is still ongoing, and move
			// to locked in for vote 3 since it passed.
			name:        "rule change interval 2",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid | vote1No.Bits | vote3Choice1.Bits,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdFailed, vote1NoIdx),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdLockedIn, vote3Choice1Idx),
			},
		}, {
			// Cast normal votes for another rule change interval.  The state
			// should remain in failed for vote 1, remain in started for vote 2,
			// and move to active for vote 3.
			name:        "rule change interval 3",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdFailed, vote1NoIdx),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdActive, vote3Choice1Idx),
			},
		}},
	}, {
		// Ensure votes do not count (voting does not start) until there is a
		// majority PoW version upgrade.
		name:         "votes do not count without majority PoW version upgrade",
		blockVersion: powVersion - 1,
		stakeVersion: posVersion,
		perInstanceTests: []perInstanceTest{{
			name:      "svh",
			numBlocks: svh - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast normal votes to reach the next rule change interval.  The
			// state should remain in defined since achieving a majority PoW
			// upgrade is one of the prerequisites for a vote to start.
			name:        "rule change interval",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast normal votes to reach the next rule change interval.  The
			// state should remain in defined since achieving a majority PoW
			// upgrade is one of the prerequisites for a vote to start.
			name:        "rule change interval 2",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid | vote1Yes.Bits,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}},
	}, {
		// Ensure that another majority PoW version upgrade after the one that
		// was required for the vote to start to begin with while a vote is
		// ongoing does not prevent that vote from continuing to completion.
		//
		// Also test normal voting results per:
		//
		// Vote 1: 100% yes
		// Vote 2: 100% abstain
		// Vote 3: 100% yes with choice 2
		name:         "new majority PoW versions don't stop ongoing votes",
		blockVersion: powVersion + 1,
		stakeVersion: posVersion,
		perInstanceTests: []perInstanceTest{{
			name:      "svh",
			numBlocks: svh - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast normal votes to reach the first rule change interval.  The
			// state for all votes in the deployment should move to started.
			name:        "rule change interval",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast votes: 100% yes for vote 1, 100% abstain for vote 2, and
			// 100% choice 2 for vote 3.  The state should move to locked in for
			// vote 1 since it passed, remain in started for vote 2 since the
			// vote is still ongoing, and move to locked in for vote 3 since it
			// passed.
			name:        "rule change interval 2",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid | vote1Yes.Bits | vote3Choice2.Bits,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdLockedIn, vote1YesIdx),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdLockedIn, vote3Choice2Idx),
			},
		}, {
			// Cast normal votes for another rule change interval.  The state
			// should move to active for vote 1, remain in started for vote 2,
			// and move to active for vote 3.
			name:        "rule change interval 3",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdActive, vote1YesIdx),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdActive, vote3Choice2Idx),
			},
		}},
	}, {
		// Ensure that vote versions that are newer than the version that
		// defined the deployment do not count towards that deployment and also
		// do not cause the vote to terminate before expiration despite
		// resulting in another majority PoS version upgrade.
		//
		// Also test normal voting results per:
		//
		// Vote 1: 100% abstain
		// Vote 2: 100% yes
		// Vote 3: 100% yes with choice 3
		name:         "new majority PoS versions don't stop ongoing votes",
		blockVersion: powVersion,
		stakeVersion: posVersion,
		perInstanceTests: []perInstanceTest{{
			name:      "svh",
			numBlocks: svh - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast normal votes with a vote version that is higher than the
			// deployment version for a rule change interval so another majority
			// stake version upgrade is achieved.
			name:        "rule change interval",
			voteVersion: posVersion + 1,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast votes with a vote version that is higher than the deployment
			// version for a rule change interval.  This would ordinarily result
			// in state changes if the votes were the right version.  However,
			// the state should remain started because they don't count and thus
			// the vote is still ongoing.
			name:        "rule change interval 2",
			voteVersion: posVersion + 1,
			voteBits:    vbPrevBlockValid | vote2Yes.Bits | vote3Choice3.Bits,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast votes with a vote version that is higher than the deployment
			// version for another rule change interval.  Similar to the
			// previous case, the state should remain started because they don't
			// count towards the deployment due to being a newer version and
			// thus the vote is still ongoing.
			name:        "rule change interval 3",
			voteVersion: posVersion + 1,
			voteBits:    vbPrevBlockValid | vote2Yes.Bits | vote3Choice3.Bits,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast votes with the correct vote version for the deployment this
			// time: 100% abstain for vote 1, 100% yes for vote 2, and 100%
			// choice 3 for vote 3.  The state should remain in started for vote
			// 1 and move to locked in for votes 2 and 3 since they passed.
			name:        "rule change interval 4",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid | vote2Yes.Bits | vote3Choice3.Bits,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdLockedIn, vote2YesIdx),
				vote3.Id: newThresholdState(ThresholdLockedIn, vote3Choice3Idx),
			},
		}, {
			// Cast normal votes with a vote version that is higher than the
			// version that just locked in.  The states for votes 2 and 3 should
			// still move on to active because they have already passed.
			name:        "rule change interval 5",
			voteVersion: posVersion + 1,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdActive, vote2YesIdx),
				vote3.Id: newThresholdState(ThresholdActive, vote3Choice3Idx),
			},
		}},
	}, {
		// Ensure accepting all votes in parallel does not result in any
		// unexpected behavior.
		//
		// Vote 1: 100% yes
		// Vote 2: 100% yes
		// Vote 3: 100% yes with choice 4
		name:         "100% parallel yes votes",
		blockVersion: powVersion,
		stakeVersion: posVersion,
		perInstanceTests: []perInstanceTest{{
			name:      "svh",
			numBlocks: svh - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast normal votes to reach the first rule change interval.  The
			// state for all votes in the deployment should move to started.
			name:        "rule change interval",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast votes: 100% yes for vote 1, 100% yes for vote 2, and 100%
			// choice 4 for vote 3.  The state should move to locked in for all
			// of them since they all passed.
			name:        "rule change interval 2",
			voteVersion: posVersion,
			voteBits: vbPrevBlockValid | vote1Yes.Bits | vote2Yes.Bits |
				vote3Choice4.Bits,
			numBlocks: ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdLockedIn, vote1YesIdx),
				vote2.Id: newThresholdState(ThresholdLockedIn, vote2YesIdx),
				vote3.Id: newThresholdState(ThresholdLockedIn, vote3Choice4Idx),
			},
		}, {
			// Cast normal votes for another rule change interval.  The state
			// should move to active for all votes.
			name:        "rule change interval 3",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdActive, vote1YesIdx),
				vote2.Id: newThresholdState(ThresholdActive, vote2YesIdx),
				vote3.Id: newThresholdState(ThresholdActive, vote3Choice4Idx),
			},
		}, {
			// Cast normal votes for another rule change interval.  The state
			// should remain active for all votes.
			name:        "rule change interval 4",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdActive, vote1YesIdx),
				vote2.Id: newThresholdState(ThresholdActive, vote2YesIdx),
				vote3.Id: newThresholdState(ThresholdActive, vote3Choice4Idx),
			},
		}},
	}, {
		// Ensure rejecting all votes in parallel does not result in any
		// unexpected behavior.
		//
		// Vote 1: 100% no
		// Vote 2: 100% no
		// Vote 3: 100% no
		name:         "100% parallel no votes",
		blockVersion: powVersion,
		stakeVersion: posVersion,
		perInstanceTests: []perInstanceTest{{
			name:      "svh",
			numBlocks: svh - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast normal votes to reach the first rule change interval.  The
			// state for all votes in the deployment should move to started.
			name:        "rule change interval",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast votes: 100% no for vote 1, 100% no for vote 2, and 100% no
			// for vote 3.  The state should move to failed for all votes.
			name:        "rule change interval 2",
			voteVersion: posVersion,
			voteBits: vbPrevBlockValid | vote1No.Bits | vote2No.Bits |
				vote3No.Bits,
			numBlocks: ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdFailed, vote1NoIdx),
				vote2.Id: newThresholdState(ThresholdFailed, vote2NoIdx),
				vote3.Id: newThresholdState(ThresholdFailed, vote3NoIdx),
			},
		}, {
			// Cast normal votes for another rule change interval.  The state
			// should remain failed for all votes.
			name:        "rule change interval 3",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdFailed, vote1NoIdx),
				vote2.Id: newThresholdState(ThresholdFailed, vote2NoIdx),
				vote3.Id: newThresholdState(ThresholdFailed, vote3NoIdx),
			},
		}},
	}, {
		// Ensure abstaining from all votes in parallel does not result in any
		// unexpected behavior.
		//
		// Vote 1: 100% abstain
		// Vote 2: 100% abstain
		// Vote 3: 100% abstain
		name:         "100% parallel abstain votes",
		blockVersion: powVersion,
		stakeVersion: posVersion,
		perInstanceTests: []perInstanceTest{{
			name:      "svh",
			numBlocks: svh - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast normal votes to reach the first rule change interval.  The
			// state for all votes in the deployment should move to started.
			name:        "rule change interval",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast normal votes while abstaining from all ongoing votes for a
			// rule change interval.  The state should remain in started for all
			// votes since they are still ongoing.
			name:        "rule change interval 2",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Continue cast normal votes while abstaining from all ongoing
			// votes for another rule change interval.  The state should remain
			// in started for all votes since they are still ongoing.
			name:        "rule change interval 3",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Continue cast normal votes while abstaining from all ongoing
			// votes foryet  another rule change interval.  The state should
			// remain in started for all votes since they are still ongoing.
			name:        "rule change interval 4",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}},
	}, {
		// Ensure votes with bits comprised of invalid choices for all votes are
		// ignored and do not prevent votes from ultimately reaching a majority.
		//
		// Also test normal voting results per:
		//
		// Vote 1: 100% yes
		// Vote 2: 100% no
		// Vote 3: 100% no
		name:         "100% invalid votes followed by majority results",
		blockVersion: powVersion,
		stakeVersion: posVersion,
		perInstanceTests: []perInstanceTest{{
			name:      "svh",
			numBlocks: svh - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast normal votes to reach the first rule change interval.  The
			// state for all votes in the deployment should move to started.
			name:        "rule change interval",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast 100% invalid votes.  The state should remain started fore
			// all of the votes because none of the invalid choices for them
			// count and thus the votes are still ongoing.
			name:        "rule change interval 2",
			voteVersion: posVersion,
			voteBits: vbPrevBlockValid | vbVote1Invalid | vbVote2Invalid |
				vbVote3Invalid,
			numBlocks: ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Vote yes for vote 1.  Its state should move to locked in.  The
			// vote 2 state should move to active because the vote passed.
			name:        "rule change interval 3",
			voteVersion: posVersion,
			voteBits: vbPrevBlockValid | vote1Yes.Bits | vote2No.Bits |
				vote3No.Bits,
			numBlocks: ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdLockedIn, vote1YesIdx),
				vote2.Id: newThresholdState(ThresholdFailed, vote2NoIdx),
				vote3.Id: newThresholdState(ThresholdFailed, vote3NoIdx),
			},
		}, {
			// Continue casting normal votes to reach the next rule change
			// interval.  The vote 1 state should move to active because the
			// vote passed.
			name:        "rule change interval 4",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdActive, vote1YesIdx),
				vote2.Id: newThresholdState(ThresholdFailed, vote2NoIdx),
				vote3.Id: newThresholdState(ThresholdFailed, vote3NoIdx),
			},
		}},
	}, {
		// Ensure votes with invalid choices for one vote cast alongside valid
		// choices for other votes do not affect the outcome of those other
		// votes in the same deployment.
		//
		// Also test normal voting results per:
		//
		// Vote 1: 100% abstain
		// Vote 2: 100% yes
		// Vote 3: 100% choice 1
		name:         "mixed parallel invalid votes with majority votes",
		blockVersion: powVersion,
		stakeVersion: posVersion,
		perInstanceTests: []perInstanceTest{{
			name:      "svh",
			numBlocks: svh - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast normal votes to reach the first rule change interval.  The
			// state for all votes in the deployment should move to started.
			name:        "rule change interval",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast votes: 100% invalid for vote 1, 100% yes for vote 2, 100%
			// choice 1 for vote 3.  The state should remain in started for vote
			// 1 because the invalid choice for it doesn't count, and move to
			// locked in for votes 2 and 3 since they both passed.
			name:        "rule change interval 2",
			voteVersion: posVersion,
			voteBits: vbPrevBlockValid | vbVote1Invalid | vote2Yes.Bits |
				vote3Choice1.Bits,
			numBlocks: ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdLockedIn, vote2YesIdx),
				vote3.Id: newThresholdState(ThresholdLockedIn, vote3Choice1Idx),
			},
		}, {
			// Continue casting normal votes to reach the next rule change
			// interval.  The state should remain started for vote 1 since it is
			// still ongoing and move to active for votes 2 and 3.
			name:        "rule change interval 3",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdActive, vote2YesIdx),
				vote3.Id: newThresholdState(ThresholdActive, vote3Choice1Idx),
			},
		}},
	}, {
		// Ensure deployments fail if they reach expiration before ever
		// achieving the prerequisites needed to start (e.g. PoW and PoS
		// majority version upgrades).
		name:         "expire before started",
		expireOffset: int64(svh + ruleChangeInterval + ruleChangeInterval/2),
		blockVersion: powVersion,
		stakeVersion: posVersion - 1,
		perInstanceTests: []perInstanceTest{{
			name:      "svh",
			numBlocks: svh - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast normal votes with an older stake version until the first
			// rule change interval so a majority stake version upgrade is not
			// achieved.  The state should remain in defined since achieving a
			// majority stake upgrade is one of the prerequisites for a vote to
			// start and thus none of the votes count.
			name:        "rule change interval",
			voteVersion: posVersion - 1,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Continue casting normal votes with an older stake version for
			// another rule change interval that also exceeds the expiration for
			// the deployment.  The state should move to failed for all votes
			// because they expired.
			name:        "rule change interval 2",
			voteVersion: posVersion - 1,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdFailed, invalidChoice),
				vote2.Id: newThresholdState(ThresholdFailed, invalidChoice),
				vote3.Id: newThresholdState(ThresholdFailed, invalidChoice),
			},
		}},
	}, {
		// Ensure deployments fail if they reach expiration before achieving a
		// majority result.
		name:         "expire after started",
		expireOffset: int64(svh + ruleChangeInterval + ruleChangeInterval/2),
		blockVersion: powVersion,
		stakeVersion: posVersion - 1,
		perInstanceTests: []perInstanceTest{{
			name:      "svh",
			numBlocks: svh - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast votes with a newer stake version for a full rule change
			// interval to achieve a majority stake version upgrade.  The state
			// should move to started since the stake version upgrade was
			// achieved.
			name:        "rule change interval",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast yes votes which would ordinarily cause all of the votes in
			// the deployment to pass, but since the expiration was
			// intentionally set such that it is exceeded, the state should move
			// to failed for all votes.
			name:        "rule change interval 2",
			voteVersion: posVersion,
			voteBits: vbPrevBlockValid | vote1Yes.Bits | vote2Yes.Bits |
				vote3Choice1.Bits,
			numBlocks: ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdFailed, invalidChoice),
				vote2.Id: newThresholdState(ThresholdFailed, invalidChoice),
				vote3.Id: newThresholdState(ThresholdFailed, invalidChoice),
			},
		}},
	}, {
		// Ensure the interplay between the stake version and rule change
		// intervals at the edge conditions produce the expected results.  For
		// example, one important case is when the majority stake version
		// upgrade happens exactly when the stake version and rule change
		// intervals align.
		//
		// Also test normal voting results per:
		//
		// Vote 1: 100% yes
		// Vote 2: 100% yes
		// Vote 3: 100% choice 2
		name:         "rci and svi alignment edges",
		blockVersion: powVersion,
		stakeVersion: posVersion - 1,
		perInstanceTests: []perInstanceTest{{
			// Cast enough normal votes with a vote version that is less than
			// the one being voted for in order to reach one stake version
			// interval prior to the point where it and the rule change interval
			// align.  The state should stay in defined and not start since the
			// vote versions indicate PoS has not upgraded.
			name:        "one interval prior to svi and rci alignment",
			voteVersion: posVersion - 1,
			voteBits:    vbPrevBlockValid,
			numBlocks: func() uint32 {
				// Calculate the first height where the rule change and stake
				// version intervals align.  It is the least common multiple of
				// the two.  It is calculated here as the product divided by the
				// gcd (which is itself calculated via the Euclidean algorithm).
				x, y := ruleChangeInterval, stakeVerInterval
				for y != 0 {
					x, y = y, x%y
				}
				firstAlignment := (ruleChangeInterval / x) * stakeVerInterval

				// Return one stake version interval prior.
				return svh + firstAlignment - stakeVerInterval - 1
			}(),
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast normal votes with the correct vote version in order to reach
			// one block prior to the point the stake version and rule change
			// intervals align.  The state should still be defined since the
			// state only changes on rule change intervals and the majority
			// stake version upgrade will not be achieved until the next block.
			name:        "one block prior to svi and rci alignment",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   stakeVerInterval - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast another normal vote with the correct vote version to reach
			// the exact point the stake version and rule change intervals
			// align.  The state should move to started since a majority stake
			// version upgrade has been achieved as of this block since it's a
			// stake version interval and it's is also a rule change interval.
			name:        "svi and rci alignment",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast votes for one block less than a rule change interval: 100%
			// yes for vote 1, 100% for vote 2, 100% choice 2 for vote 3.  The
			// state should remain in started because the state only changes on
			// rule change intervals.
			name:        "one block prior to rule change interval",
			voteVersion: posVersion,
			voteBits: vbPrevBlockValid | vote1Yes.Bits | vote2Yes.Bits |
				vote3Choice2.Bits,
			numBlocks: ruleChangeInterval - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast one more vote with the same choices as the previous to reach
			// the rule change interval.  The state should move to started for
			// all votes in the deployment since they all passed.
			name:        "rule change interval",
			voteVersion: posVersion,
			voteBits: vbPrevBlockValid | vote1Yes.Bits | vote2Yes.Bits |
				vote3Choice2.Bits,
			numBlocks: 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdLockedIn, vote1YesIdx),
				vote2.Id: newThresholdState(ThresholdLockedIn, vote2YesIdx),
				vote3.Id: newThresholdState(ThresholdLockedIn, vote3Choice2Idx),
			},
		}, {
			// Continue casting normal votes to reach the next rule change
			// interval.  The state should move to active for all votes in the
			// deployment.
			name:        "rule change interval 2",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdActive, vote1YesIdx),
				vote2.Id: newThresholdState(ThresholdActive, vote2YesIdx),
				vote3.Id: newThresholdState(ThresholdActive, vote3Choice2Idx),
			},
		}},
	}, {
		// Ensure the quorum requirements are imposed as intended by testing the
		// edge conditions as well as achieving varying majority results in
		// parallel votes with the exact minimum number of votes needed.
		//
		// The final majority results:
		//
		// Vote 1: 75% no / 25% yes
		// Vote 2: 75% yes / 25% no
		// Vote 3: 75% choice 3 / 25% choice 4
		name:         "minimum quorum majority",
		blockVersion: powVersion,
		stakeVersion: posVersion,
		perInstanceTests: []perInstanceTest{{
			name:      "svh",
			numBlocks: svh - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast normal votes to reach the first rule change interval.  The
			// state for all votes in the deployment should move to started.
			name:        "rule change interval",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast votes such that:
			// 1) There are exactly one less than the minimum number of
			//    non-abstaining votes that are required to reach quorum
			// 2) The percentage of vote choices for those non-abstaining votes
			//    are: 100% no for vote 1, 100% yes for vote 2, and 100% choice
			//    3 for vote 3
			// 3) The remaining votes abstain from all choices
			//
			// The state should remain in started for all votes in the
			// deployment despite them all otherwise achieving a majority choice
			// because qourum is not met.
			name:        "rule change interval 2",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			munger: func() func(*blockNode) {
				var totalVotes uint32
				return func(node *blockNode) {
					for i := 0; i < len(node.votes); i++ {
						if totalVotes < ruleChangeQuorum-1 {
							node.votes[i].Bits |= vote1No.Bits | vote2Yes.Bits |
								vote3Choice3.Bits
						}
						totalVotes++
					}
				}
			}(),
			numBlocks: ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast votes such that:
			// 1) There are exactly enough non-abstaining votes required to
			//    reach quorum
			// 2) The percentage of vote choices for those non-abstaining votes
			//    are exactly one vote fewer than the minimum required number
			//    to reach a majority and split as follows:
			//    a) 75% (minus 1) no / 25% (plus 1) yes
			//    b) 75% (minus 1) yes / 25% (plus 1) no
			//    a) 75% (minus 1) choice 3 / 25% (plus 1) choice 4
			// 3) The remaining votes abstain from all choices
			//
			// The state should remain in started for all votes in the
			// deployment despite the minimum quorum being achieved because none
			// of them resulted in the minimum required majority.
			name:        "rule change interval 3",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			munger: func() func(*blockNode) {
				var totalVotes uint32
				return func(node *blockNode) {
					for i := 0; i < len(node.votes); i++ {
						if totalVotes < quorumMajority-1 {
							node.votes[i].Bits |= vote1No.Bits | vote2Yes.Bits |
								vote3Choice3.Bits
						} else if totalVotes < ruleChangeQuorum {
							node.votes[i].Bits |= vote1Yes.Bits | vote2No.Bits |
								vote3Choice4.Bits
						}
						totalVotes++
					}
				}
			}(),
			numBlocks: ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast votes such that:
			// 1) There are exactly enough non-abstaining votes required to
			//    reach quorum
			// 2) The percentage of vote choices for those non-abstaining votes
			//    are exactly the minimum required number to reach a majority
			//    and split as follows:
			//    a) 75% no / 25% yes
			//    b) 75% yes / 25% no
			//    a) 75% choice 3 / 25% choice 4
			// 3) The remaining votes abstain from all choices
			//
			// The state should move to failed for vote 1 since it has majority
			// no and locked in for votes 2 and 3 since they have majority
			// affirmative choices.
			name:        "rule change interval 4",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			munger: func() func(*blockNode) {
				var totalVotes uint32
				return func(node *blockNode) {
					for i := 0; i < len(node.votes); i++ {
						if totalVotes < quorumMajority {
							node.votes[i].Bits |= vote1No.Bits | vote2Yes.Bits |
								vote3Choice3.Bits
						} else if totalVotes < ruleChangeQuorum {
							node.votes[i].Bits |= vote1Yes.Bits | vote2No.Bits |
								vote3Choice4.Bits
						}
						totalVotes++
					}
				}
			}(),
			numBlocks: ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdFailed, vote1NoIdx),
				vote2.Id: newThresholdState(ThresholdLockedIn, vote2YesIdx),
				vote3.Id: newThresholdState(ThresholdLockedIn, vote3Choice3Idx),
			},
		}, {
			// Cast normal votes for another rule change interval.  The state
			// should remain in failed for vote 1 and move to active for votes 2
			// and 3.
			name:        "rule change interval 5",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdFailed, vote1NoIdx),
				vote2.Id: newThresholdState(ThresholdActive, vote2YesIdx),
				vote3.Id: newThresholdState(ThresholdActive, vote3Choice3Idx),
			},
		}},
	}, {
		// Ensure the results of votes don't change with a different majority
		// choice result after an original majority choice has been achieved.
		name:         "majority vote after lockin does not change outcome",
		blockVersion: powVersion,
		stakeVersion: posVersion,
		perInstanceTests: []perInstanceTest{{
			name:      "svh",
			numBlocks: svh - 1,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote2.Id: newThresholdState(ThresholdDefined, invalidChoice),
				vote3.Id: newThresholdState(ThresholdDefined, invalidChoice),
			},
		}, {
			// Cast normal votes to reach the first rule change interval.  The
			// state for all votes in the deployment should move to started.
			name:        "rule change interval",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote2.Id: newThresholdState(ThresholdStarted, invalidChoice),
				vote3.Id: newThresholdState(ThresholdStarted, invalidChoice),
			},
		}, {
			// Cast votes: 100% yes for vote 1, 100% no for vote 2, and 100%
			// choice 1 for vote 3.  The state should move to locked in for
			// votes 1 and 3 since they passed and failed for vote 2.
			name:        "rule change interval 2",
			voteVersion: posVersion,
			voteBits: vbPrevBlockValid | vote1Yes.Bits | vote2No.Bits |
				vote3Choice1.Bits,
			numBlocks: ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdLockedIn, vote1YesIdx),
				vote2.Id: newThresholdState(ThresholdFailed, vote2NoIdx),
				vote3.Id: newThresholdState(ThresholdLockedIn, vote3Choice1Idx),
			},
		}, {
			// Cast votes that are different than they majority results already
			// achieved: 100% no for vote 1, 100% yes for vote 2, and 100%
			// choice 2 for vote 3.  The state should move to active for vote 1
			// active with the original choice 1 for vote 3, and remain failed
			// for vote 2 since none of these votes should count due to the
			// existing majority results.
			name:        "rule change interval 3",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdActive, vote1YesIdx),
				vote2.Id: newThresholdState(ThresholdFailed, vote2NoIdx),
				vote3.Id: newThresholdState(ThresholdActive, vote3Choice1Idx),
			},
		}, {
			// Cast normal votes for another rule change interval.  The state
			// should remain the same as the previous interval for all votes.
			name:        "rule change interval 4",
			voteVersion: posVersion,
			voteBits:    vbPrevBlockValid,
			numBlocks:   ruleChangeInterval,
			wantStates: wantStatesMap{
				vote1.Id: newThresholdState(ThresholdActive, vote1YesIdx),
				vote2.Id: newThresholdState(ThresholdFailed, vote2NoIdx),
				vote3.Id: newThresholdState(ThresholdActive, vote3Choice1Idx),
			},
		}},
	}}

nextTest:
	for _, test := range tests {
		// Determine start and expire times for the deployment per the test and
		// then clone the mocked parameters and mutate them accordingly so the
		// changes only affect the current test.
		curTimestamp := time.Now()
		expireTimestamp := curTimestamp.Add(24 * time.Hour)
		if test.expireOffset != 0 {
			expireOffset := time.Second * time.Duration(test.expireOffset)
			expireTimestamp = curTimestamp.Add(expireOffset)
		}
		params := cloneParams(mockParams)
		for i := 0; i < len(params.Deployments[posVersion]); i++ {
			deployment := &params.Deployments[posVersion][i]
			deployment.StartTime = uint64(curTimestamp.Unix())
			deployment.ExpireTime = uint64(expireTimestamp.Unix())
		}

		// Reset the chain for every test.  This includes resetting the internal
		// deployment info and associated state caches.
		bc := newFakeChain(params)
		node := bc.bestChain.Tip()
		node.stakeVersion = test.stakeVersion

		for _, subTest := range test.perInstanceTests {
			for i := uint32(0); i < subTest.numBlocks; i++ {
				node = newFakeNode(node, test.blockVersion, test.stakeVersion,
					0, curTimestamp)

				// Create fake votes with the vote version and bits specified
				// in the test.  There are no votes prior to stake validation
				// height.
				numVotes := params.TicketsPerBlock
				if node.height < int64(svh) {
					numVotes = 0
				}
				for j := uint16(0); j < numVotes; j++ {
					node.votes = append(node.votes, stake.VoteVersionTuple{
						Version: subTest.voteVersion,
						Bits:    subTest.voteBits,
					})
				}
				if subTest.munger != nil {
					subTest.munger(node)
				}
				bc.index.AddNode(node)
				bc.bestChain.SetTip(node)
				curTimestamp = curTimestamp.Add(time.Second)
			}

			for voteID, wantState := range subTest.wantStates {
				gotState, err := bc.NextThresholdState(&node.hash, voteID)
				if err != nil {
					t.Errorf("%s-%s: unexpected err: %v", test.name,
						subTest.name, err)
					continue nextTest
				}
				if !reflect.DeepEqual(gotState, wantState) {
					t.Errorf("%s-%s: mismatched state for vote %s - got %+v, "+
						"want %+v", test.name, subTest.name, voteID, gotState,
						wantState)
					continue nextTest
				}
			}
		}
	}
}
