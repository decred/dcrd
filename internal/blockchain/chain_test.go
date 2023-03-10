// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"compress/bzip2"
	"encoding/gob"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/v5/chaingen"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

const (
	// noTreasury signifies the treasury agenda should be treated as though
	// it is inactive.  It is used to increase the readability of the
	// tests.
	noTreasury = false
)

// cloneParams returns a deep copy of the provided parameters so the caller is
// free to modify them without worrying about interfering with other tests.
func cloneParams(params *chaincfg.Params) *chaincfg.Params {
	// Encode via gob.
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	enc.Encode(params)

	// Decode via gob to make a deep copy.
	var paramsCopy chaincfg.Params
	dec := gob.NewDecoder(buf)
	dec.Decode(&paramsCopy)
	return &paramsCopy
}

// TestDeploymentParamsValidation ensures that the code related to validating
// the deployments in the provided chain parameters works as intended.
func TestDeploymentParamsValidation(t *testing.T) {
	// Create parameters with the deployments replaced with known mocked
	// values for use as a base to mutate in the tests below.
	mockParams := cloneParams(chaincfg.RegNetParams())
	mockParams.Deployments = map[uint32][]chaincfg.ConsensusDeployment{
		0: {{
			Vote: chaincfg.Vote{
				Id:   "testvote",
				Mask: 0x6,
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "keep the existing rules",
					Bits:        0x0002, // Bit 1
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "change to the new rules",
					Bits:        0x0004, // Bit 2
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}},
	}

	tests := []struct {
		name   string                 // test description
		munger func(*chaincfg.Params) // func to mutate params
		err    error                  // expected error
	}{{
		name: "reject duplicate deployment id in the same version",
		munger: func(params *chaincfg.Params) {
			// Duplicate the vote in deployment version 0 so there is a
			// duplicate id in the same version.
			votes := params.Deployments[0]
			params.Deployments[0] = append(votes, votes[0])
		},
		err: ErrDuplicateDeployment,
	}, {
		name: "reject duplicate deployment id in different versions",
		munger: func(params *chaincfg.Params) {
			// Duplicate the votes in deployment version 0 to deployment version
			// 1 so there is a duplicate id in different versions.
			params.Deployments[1] = params.Deployments[0]
		},
		err: ErrDuplicateDeployment,
	}, {
		name: "require optional forced choice to exist when specified",
		munger: func(params *chaincfg.Params) {
			deployment := &params.Deployments[0][0]
			deployment.ForcedChoiceID = "bogus"
		},
		err: ErrUnknownDeploymentChoice,
	}, {
		name: "require optional forced choice to be non abstain when specified",
		munger: func(params *chaincfg.Params) {
			deployment := &params.Deployments[0][0]
			deployment.ForcedChoiceID = "abstain"
		},
		err: ErrDeploymentChoiceAbstain,
	}, {
		name: "require distinct masks for all votes in same deployment version",
		munger: func(params *chaincfg.Params) {
			// Create another deployment with a mask that uses a bit that is
			// already used in the existing one.
			deployment := chaincfg.ConsensusDeployment{
				Vote: chaincfg.Vote{
					Id:   "testvote2",
					Mask: 0xc,
					Choices: []chaincfg.Choice{{
						Id:          "abstain",
						Description: "abstain voting for change",
						Bits:        0x0000,
						IsAbstain:   true,
						IsNo:        false,
					}, {
						Id:          "no",
						Description: "keep the existing rules",
						Bits:        0x0004, // Bit 2
						IsAbstain:   false,
						IsNo:        true,
					}, {
						Id:          "yes",
						Description: "change to the new rules",
						Bits:        0x0008, // Bit 3
						IsAbstain:   false,
						IsNo:        false,
					}},
				},
				StartTime:  0,             // Always available for vote
				ExpireTime: math.MaxInt64, // Never expires
			}
			params.Deployments[0] = append(params.Deployments[0], deployment)
		},
		err: ErrDeploymentBadMask,
	}, {
		name: "require non-zero mask",
		munger: func(params *chaincfg.Params) {
			vote := &params.Deployments[0][0].Vote
			vote.Mask = 0
		},
		err: ErrDeploymentBadMask,
	}, {
		name: "reject mask with reserved parent regular tx tree approval bit",
		munger: func(params *chaincfg.Params) {
			vote := &params.Deployments[0][0].Vote
			vote.Mask |= dcrutil.BlockValid
		},
		err: ErrDeploymentBadMask,
	}, {
		name: "require consecutive mask",
		munger: func(params *chaincfg.Params) {
			vote := &params.Deployments[0][0].Vote
			vote.Mask = 0xa // 0b1010 -- non-consecutive mask
		},
		err: ErrDeploymentBadMask,
	}, {
		name: "reject too many choices",
		munger: func(params *chaincfg.Params) {
			vote := &params.Deployments[0][0].Vote
			vote.Choices = append(vote.Choices, chaincfg.Choice{
				Id:   "maybe",
				Bits: 0x0006,
			})
			vote.Choices = append(vote.Choices, chaincfg.Choice{
				Id:   "another",
				Bits: 0x0006, // invalid bits too
			})
		},
		err: ErrDeploymentTooManyChoices,
	}, {
		name: "reject choices without an id",
		munger: func(params *chaincfg.Params) {
			vote := &params.Deployments[0][0].Vote
			vote.Choices[1].Id = ""
		},
		err: ErrDeploymentMissingChoiceID,
	}, {
		name: "reject choice with abstain bits but no abstain flag",
		munger: func(params *chaincfg.Params) {
			// Mark the abstain choice as not being abstain.
			vote := &params.Deployments[0][0].Vote
			vote.Choices[0].IsAbstain = false
		},
		err: ErrDeploymentBadChoiceBits,
	}, {
		name: "reject choice with bits that do not conform to the mask",
		munger: func(params *chaincfg.Params) {
			// Assign invalid bits per the mask to a choice.
			vote := &params.Deployments[0][0].Vote
			vote.Choices[2].Bits = 0xc // 0b1100
		},
		err: ErrDeploymentBadChoiceBits,
	}, {
		name: "require exclusive abstain and no flags",
		munger: func(params *chaincfg.Params) {
			// Mark a choice with both flags.
			vote := &params.Deployments[0][0].Vote
			vote.Choices[0].IsNo = true
		},
		err: ErrDeploymentNonExclusiveFlags,
	}, {
		name: "require unique choice ids",
		munger: func(params *chaincfg.Params) {
			// Assign duplicate id.
			vote := &params.Deployments[0][0].Vote
			vote.Choices[2].Id = vote.Choices[1].Id
		},
		err: ErrDeploymentDuplicateChoice,
	}, {
		name: "require the abstain choice",
		munger: func(params *chaincfg.Params) {
			// Remove the abstain choice.
			vote := &params.Deployments[0][0].Vote
			vote.Choices = vote.Choices[1:]
		},
		err: ErrDeploymentMissingAbstain,
	}, {
		name: "reject more than one abstain choice",
		munger: func(params *chaincfg.Params) {
			// Mark a second choice as abstain.
			vote := &params.Deployments[0][0].Vote
			vote.Choices[1].IsAbstain = true
			vote.Choices[1].IsNo = false
		},
		err: ErrDeploymentTooManyAbstain,
	}, {
		name: "require a no choice",
		munger: func(params *chaincfg.Params) {
			// Remove the no choice.
			vote := &params.Deployments[0][0].Vote
			vote.Choices = append(vote.Choices[:1], vote.Choices[2:]...)
		},
		err: ErrDeploymentMissingNo,
	}, {
		name: "reject more than one no choice",
		munger: func(params *chaincfg.Params) {
			// Mark a second choice as no.
			vote := &params.Deployments[0][0].Vote
			vote.Choices[2].IsNo = true
		},
		err: ErrDeploymentTooManyNo,
	}}

	for _, test := range tests {
		// Clone the mocked parameters so they can be mutated by each test
		// without affecting the others and then mutate them per the test.
		params := cloneParams(mockParams)
		if test.munger != nil {
			test.munger(params)
		}

		_, err := extractDeployments(params)
		if !errors.Is(err, test.err) {
			t.Fatalf("%q: unexpected err -- got %v, want %v", test.name, err,
				test.err)
		}
	}
}

// TestBlockchainFunction tests the various blockchain API to ensure proper
// functionality.
func TestBlockchainFunctions(t *testing.T) {
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

	// Load up the rest of the blocks up to HEAD~1.
	filename := filepath.Join("testdata", "blocks0to168.bz2")
	fi, err := os.Open(filename)
	if err != nil {
		t.Errorf("Unable to open %s: %v", filename, err)
	}
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file.
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data.
	bcDecoder := gob.NewDecoder(bcBuf)
	blockChain := make(map[int64][]byte)

	// Decode the blockchain into the map.
	if err := bcDecoder.Decode(&blockChain); err != nil {
		t.Errorf("error decoding test blockchain: %v", err.Error())
	}

	// Insert blocks 1 to 168 and perform various tests.
	for i := 1; i <= 168; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain[int64(i)])
		if err != nil {
			t.Errorf("NewBlockFromBytes error: %v", err.Error())
		}

		_, err = chain.ProcessBlock(bl)
		if err != nil {
			t.Fatalf("ProcessBlock error at height %v: %v", i, err.Error())
		}
	}

	val, err := chain.TicketPoolValue()
	if err != nil {
		t.Errorf("Failed to get ticket pool value: %v", err)
	}
	expectedVal := dcrutil.Amount(3495091704)
	if val != expectedVal {
		t.Errorf("Failed to get correct result for ticket pool value; "+
			"want %v, got %v", expectedVal, val)
	}

	a, _ := stdaddr.DecodeAddress("SsbKpMkPnadDcZFFZqRPY8nvdFagrktKuzB", params)
	hs, err := chain.TicketsWithAddress(a.(stdaddr.StakeAddress))
	if err != nil {
		t.Errorf("Failed to do TicketsWithAddress: %v", err)
	}
	expectedLen := 223
	if len(hs) != expectedLen {
		t.Errorf("Failed to get correct number of tickets for "+
			"TicketsWithAddress; want %v, got %v", expectedLen, len(hs))
	}

	totalSubsidy := chain.BestSnapshot().TotalSubsidy
	expectedSubsidy := int64(35783267326630)
	if expectedSubsidy != totalSubsidy {
		t.Errorf("Failed to get correct total subsidy for "+
			"TotalSubsidy; want %v, got %v", expectedSubsidy,
			totalSubsidy)
	}
}

// TestForceHeadReorg ensures forcing header reorganization works as expected.
func TestForceHeadReorg(t *testing.T) {
	// Create a test harness initialized with the genesis block as the tip.
	params := chaincfg.RegNetParams()
	g := newChaingenHarness(t, params)

	// Define some additional convenience helper functions to process the
	// current tip block associated with the generator.
	//
	// rejectForceTipReorg forces the chain instance to reorganize the
	// current tip of the main chain from the given block to the given
	// block and expected it to be rejected with the provided error kind.
	rejectForceTipReorg := func(fromTipName, toTipName string, kind ErrorKind) {
		from := g.BlockByName(fromTipName)
		to := g.BlockByName(toTipName)
		t.Logf("Testing forced reorg from %s (hash %s, height %d) "+
			"to %s (hash %s, height %d)", fromTipName,
			from.BlockHash(), from.Header.Height, toTipName,
			to.BlockHash(), to.Header.Height)

		err := g.chain.ForceHeadReorganization(from.BlockHash(), to.BlockHash())
		if err == nil {
			t.Fatalf("forced header reorg from block %q (hash %s, "+
				"height %d) to block %q (hash %s, height %d) "+
				"should have failed", fromTipName, from.BlockHash(),
				from.Header.Height, toTipName, to.BlockHash(),
				to.Header.Height)
		}

		// Ensure the error kind is of the expected type and matches
		// the value specified in the test instance.
		if !errors.Is(err, kind) {
			t.Fatalf("forced header reorg from block %q (hash %s, "+
				"height %d) to block %q (hash %s, height %d) "+
				"does not have expected reject code -- got %v, "+
				"want %v", fromTipName,
				from.BlockHash(), from.Header.Height, toTipName,
				to.BlockHash(), to.Header.Height, err, kind)
		}
	}

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

	// ---------------------------------------------------------------------
	// Forced header reorganization test.
	// ---------------------------------------------------------------------

	// Start by building a block at current tip (value in parens is which
	// output is spent):
	//
	//   ... -> b1(0)
	g.NextBlock("b1", outs[0], ticketOuts[0])
	g.AcceptTipBlock()

	// Create a fork from b1 with an invalid block due to committing to an
	// invalid number of votes.  Since verifying the header commitment is a
	// basic sanity check, no entry will be added to the block index.
	//
	//   ... -> b1(0) -> b2(1)
	//               \-> b2bad0(1)
	g.SetTip("b1")
	g.NextBlock("b2bad0", outs[1], ticketOuts[1], func(b *wire.MsgBlock) {
		b.Header.Voters++
	})
	g.RejectTipBlock(ErrTooManyVotes)

	// Create a fork from b1 with an invalid block due to committing to an
	// invalid input amount.  Since verifying the fraud proof necessarily
	// requires access to the inputs, this will trigger the failure late
	// enough to ensure an entry is added to the block index.  Further,
	// since the block is attempting to extend the main chain it will also
	// be fully checked and thus will be known invalid.
	//
	//   ... -> b1(0)
	//               \-> b2bad1(1)
	g.SetTip("b1")
	g.NextBlock("b2bad1", outs[1], ticketOuts[1], func(b *wire.MsgBlock) {
		b.Transactions[1].TxIn[0].ValueIn--
	})
	g.RejectTipBlock(ErrFraudAmountIn)

	// Create some forks from b1.  There should not be a reorg since b1 is
	// the current tip and b2 is seen first.
	//
	//   ... -> b1(0) -> b2(1)
	//               \-> b3(1)
	//               \-> b4(1)
	//               \-> b5(1)
	//               \-> b2bad0(1)
	//               \-> b2bad1(1)
	g.SetTip("b1")
	g.NextBlock("b2", outs[1], ticketOuts[1])
	g.AcceptTipBlock()

	g.SetTip("b1")
	g.NextBlock("b3", outs[1], ticketOuts[1])
	g.AcceptedToSideChainWithExpectedTip("b2")

	g.SetTip("b1")
	g.NextBlock("b4", outs[1], ticketOuts[1])
	g.AcceptedToSideChainWithExpectedTip("b2")

	g.SetTip("b1")
	g.NextBlock("b5", outs[1], ticketOuts[1])
	g.AcceptedToSideChainWithExpectedTip("b2")

	// Create a fork from b1 with an invalid block due to committing to an
	// invalid input amount.  Since verifying the fraud proof necessarily
	// requires access to the inputs, this will trigger the failure late
	// enough to ensure an entry is added to the block index.  Further,
	// since the block is not attempting to extend the main chain it will
	// not be fully checked and thus will not yet have a known validation
	// status.
	//
	//   ... -> b1(0) -> b2(1)
	//               \-> b3(1)
	//               \-> b4(1)
	//               \-> b5(1)
	//               \-> b2bad0(1)
	//               \-> b2bad1(1)
	//               \-> b2bad2(1)
	g.SetTip("b1")
	g.NextBlock("b2bad2", outs[1], ticketOuts[1], func(b *wire.MsgBlock) {
		b.Transactions[1].TxIn[0].ValueIn--
	})
	g.AcceptedToSideChainWithExpectedTip("b2")

	// Force tip reorganization to b3.
	//
	//   ... -> b1(0) -> b3(1)
	//               \-> b2(1)
	//               \-> b4(1)
	//               \-> b5(1)
	//               \-> b2bad0(1)
	//               \-> b2bad1(1)
	//               \-> b2bad2(1)
	g.SetTip("b1")
	g.ForceTipReorg("b2", "b3")
	g.ExpectTip("b3")

	// Force tip reorganization to b4.
	//
	//   ... -> b1(0) -> b4(1)
	//               \-> b2(1)
	//               \-> b3(1)
	//               \-> b5(1)
	//               \-> b2bad0(1)
	//               \-> b2bad1(1)
	//               \-> b2bad2(1)
	g.ForceTipReorg("b3", "b4")
	g.ExpectTip("b4")

	// Force tip reorganization to b5.
	//
	//   ... -> b1(0) -> b5(1)
	//               \-> b2(1)
	//               \-> b3(1)
	//               \-> b4(1)
	//               \-> b2bad0(1)
	//               \-> b2bad1(1)
	//               \-> b2bad2(1)
	g.ForceTipReorg("b4", "b5")
	g.ExpectTip("b5")

	// Force tip reorganization back to b3 to ensure cached validation
	// results are exercised.
	//
	//   ... -> b1(0) -> b3(1)
	//               \-> b2(1)
	//               \-> b4(1)
	//               \-> b5(1)
	//               \-> b2bad0(1)
	//               \-> b2bad1(1)
	//               \-> b2bad2(1)
	g.ForceTipReorg("b5", "b3")
	g.ExpectTip("b3")

	// Attempt to force tip reorganization to the same tip.  This should
	// fail since that is not allowed.
	//
	//   ... -> b1(0) -> b3(1)
	//               \-> b2(1)
	//               \-> b4(1)
	//               \-> b5(1)
	//               \-> b2bad0(1)
	//               \-> b2bad1(1)
	//               \-> b2bad2(1)
	rejectForceTipReorg("b2", "b2", ErrForceReorgSameBlock)

	// Attempt to force tip reorganization from a block that is not the
	// current tip.  This should fail since that is not allowed.
	//
	//   ... -> b1(0) -> b3(1)
	//               \-> b2(1)
	//               \-> b4(1)
	//               \-> b5(1)
	//               \-> b2bad0(1)
	//               \-> b2bad1(1)
	//               \-> b2bad2(1)
	rejectForceTipReorg("b2", "b4", ErrForceReorgWrongChain)
	g.ExpectTip("b3")

	// Attempt to force tip reorganization to an invalid block that does
	// not have an entry in the block index.
	//
	//   ... -> b1(0) -> b3(1)
	//               \-> b2(1)
	//               \-> b4(1)
	//               \-> b5(1)
	//               \-> b2bad0(1)
	//               \-> b2bad1(1)
	//               \-> b2bad2(1)
	rejectForceTipReorg("b3", "b2bad0", ErrForceReorgMissingChild)
	g.ExpectTip("b3")

	// Attempt to force tip reorganization to an invalid block that has an
	// entry in the block index and is already known to be invalid.
	//
	//   ... -> b1(0) -> b3(1)
	//               \-> b2(1)
	//               \-> b4(1)
	//               \-> b5(1)
	//               \-> b2bad0(1)
	//               \-> b2bad1(1)
	//               \-> b2bad2(1)
	rejectForceTipReorg("b3", "b2bad1", ErrKnownInvalidBlock)
	g.ExpectTip("b3")

	// Attempt to force tip reorganization to an invalid block that has an
	// entry in the block index, but is not already known to be invalid.
	// Notice that this requires a full reorganization attempt, so the
	// expected behavior is to reorganize back to the best known good tip,
	// which is b2 because it was seen before b3.
	//
	//   ... -> b1(0) -> b3(1)
	//               \-> b2(1)
	//               \-> b4(1)
	//               \-> b5(1)
	//               \-> b2bad0(1)
	//               \-> b2bad1(1)
	//               \-> b2bad2(1)
	rejectForceTipReorg("b3", "b2bad2", ErrFraudAmountIn)
	g.ExpectTip("b2")
}

// locatorHashes is a convenience function that returns the hashes for all of
// the passed indexes of the provided nodes.  It is used to construct expected
// block locators in the tests.
func locatorHashes(nodes []*blockNode, indexes ...int) BlockLocator {
	hashes := make(BlockLocator, 0, len(indexes))
	for _, idx := range indexes {
		hashes = append(hashes, &nodes[idx].hash)
	}
	return hashes
}

// nodeHashes is a convenience function that returns the hashes for all of the
// passed indexes of the provided nodes.  It is used to construct expected hash
// slices in the tests.
func nodeHashes(nodes []*blockNode, indexes ...int) []chainhash.Hash {
	hashes := make([]chainhash.Hash, 0, len(indexes))
	for _, idx := range indexes {
		hashes = append(hashes, nodes[idx].hash)
	}
	return hashes
}

// nodeHeaders is a convenience function that returns the headers for all of
// the passed indexes of the provided nodes.  It is used to construct expected
// located headers in the tests.
func nodeHeaders(nodes []*blockNode, indexes ...int) []wire.BlockHeader {
	headers := make([]wire.BlockHeader, 0, len(indexes))
	for _, idx := range indexes {
		headers = append(headers, nodes[idx].Header())
	}
	return headers
}

// TestLocateInventory ensures that locating inventory via the LocateHeaders and
// LocateBlocks functions behaves as expected.
func TestLocateInventory(t *testing.T) {
	// Construct a synthetic block chain with a block index consisting of
	// the following structure.
	// 	genesis -> 1 -> 2 -> ... -> 15 -> 16  -> 17  -> 18
	// 	                              \-> 16a -> 17a
	tip := branchTip
	chain := newFakeChain(chaincfg.MainNetParams())
	branch0Nodes := chainedFakeNodes(chain.bestChain.Genesis(), 18)
	branch1Nodes := chainedFakeNodes(branch0Nodes[14], 2)
	for _, node := range branch0Nodes {
		chain.index.AddNode(node)
	}
	for _, node := range branch1Nodes {
		chain.index.AddNode(node)
	}
	chain.bestChain.SetTip(tip(branch0Nodes))

	// Create chain views for different branches of the overall chain to
	// simulate a local and remote node on different parts of the chain.
	localView := newChainView(tip(branch0Nodes))
	remoteView := newChainView(tip(branch1Nodes))

	// Create a chain view for a completely unrelated block chain to
	// simulate a remote node on a totally different chain.
	unrelatedBranchNodes := chainedFakeNodes(nil, 5)
	unrelatedView := newChainView(tip(unrelatedBranchNodes))

	tests := []struct {
		name       string
		locator    BlockLocator       // locator for requested inventory
		hashStop   chainhash.Hash     // stop hash for locator
		maxAllowed uint32             // max to locate, 0 = wire const
		headers    []wire.BlockHeader // expected located headers
		hashes     []chainhash.Hash   // expected located hashes
	}{
		{
			// Empty block locators and unknown stop hash.  No
			// inventory should be located.
			name:     "no locators, no stop",
			locator:  nil,
			hashStop: chainhash.Hash{},
			headers:  nil,
			hashes:   nil,
		},
		{
			// Empty block locators and stop hash in side chain.
			// The expected result is the requested block.
			name:     "no locators, stop in side",
			locator:  nil,
			hashStop: tip(branch1Nodes).hash,
			headers:  nodeHeaders(branch1Nodes, 1),
			hashes:   nodeHashes(branch1Nodes, 1),
		},
		{
			// Empty block locators and stop hash in main chain.
			// The expected result is the requested block.
			name:     "no locators, stop in main",
			locator:  nil,
			hashStop: branch0Nodes[12].hash,
			headers:  nodeHeaders(branch0Nodes, 12),
			hashes:   nodeHashes(branch0Nodes, 12),
		},
		{
			// Locators based on remote being on side chain and a
			// stop hash local node doesn't know about.  The
			// expected result is the blocks after the fork point in
			// the main chain and the stop hash has no effect.
			name:     "remote side chain, unknown stop",
			locator:  remoteView.BlockLocator(nil),
			hashStop: chainhash.Hash{0x01},
			headers:  nodeHeaders(branch0Nodes, 15, 16, 17),
			hashes:   nodeHashes(branch0Nodes, 15, 16, 17),
		},
		{
			// Locators based on remote being on side chain and a
			// stop hash in side chain.  The expected result is the
			// blocks after the fork point in the main chain and the
			// stop hash has no effect.
			name:     "remote side chain, stop in side",
			locator:  remoteView.BlockLocator(nil),
			hashStop: tip(branch1Nodes).hash,
			headers:  nodeHeaders(branch0Nodes, 15, 16, 17),
			hashes:   nodeHashes(branch0Nodes, 15, 16, 17),
		},
		{
			// Locators based on remote being on side chain and a
			// stop hash in main chain, but before fork point.  The
			// expected result is the blocks after the fork point in
			// the main chain and the stop hash has no effect.
			name:     "remote side chain, stop in main before",
			locator:  remoteView.BlockLocator(nil),
			hashStop: branch0Nodes[13].hash,
			headers:  nodeHeaders(branch0Nodes, 15, 16, 17),
			hashes:   nodeHashes(branch0Nodes, 15, 16, 17),
		},
		{
			// Locators based on remote being on side chain and a
			// stop hash in main chain, but exactly at the fork
			// point.  The expected result is the blocks after the
			// fork point in the main chain and the stop hash has no
			// effect.
			name:     "remote side chain, stop in main exact",
			locator:  remoteView.BlockLocator(nil),
			hashStop: branch0Nodes[14].hash,
			headers:  nodeHeaders(branch0Nodes, 15, 16, 17),
			hashes:   nodeHashes(branch0Nodes, 15, 16, 17),
		},
		{
			// Locators based on remote being on side chain and a
			// stop hash in main chain just after the fork point.
			// The expected result is the blocks after the fork
			// point in the main chain up to and including the stop
			// hash.
			name:     "remote side chain, stop in main after",
			locator:  remoteView.BlockLocator(nil),
			hashStop: branch0Nodes[15].hash,
			headers:  nodeHeaders(branch0Nodes, 15),
			hashes:   nodeHashes(branch0Nodes, 15),
		},
		{
			// Locators based on remote being on side chain and a
			// stop hash in main chain some time after the fork
			// point.  The expected result is the blocks after the
			// fork point in the main chain up to and including the
			// stop hash.
			name:     "remote side chain, stop in main after more",
			locator:  remoteView.BlockLocator(nil),
			hashStop: branch0Nodes[16].hash,
			headers:  nodeHeaders(branch0Nodes, 15, 16),
			hashes:   nodeHashes(branch0Nodes, 15, 16),
		},
		{
			// Locators based on remote being on main chain in the
			// past and a stop hash local node doesn't know about.
			// The expected result is the blocks after the known
			// point in the main chain and the stop hash has no
			// effect.
			name:     "remote main chain past, unknown stop",
			locator:  localView.BlockLocator(branch0Nodes[12]),
			hashStop: chainhash.Hash{0x01},
			headers:  nodeHeaders(branch0Nodes, 13, 14, 15, 16, 17),
			hashes:   nodeHashes(branch0Nodes, 13, 14, 15, 16, 17),
		},
		{
			// Locators based on remote being on main chain in the
			// past and a stop hash in a side chain.  The expected
			// result is the blocks after the known point in the
			// main chain and the stop hash has no effect.
			name:     "remote main chain past, stop in side",
			locator:  localView.BlockLocator(branch0Nodes[12]),
			hashStop: tip(branch1Nodes).hash,
			headers:  nodeHeaders(branch0Nodes, 13, 14, 15, 16, 17),
			hashes:   nodeHashes(branch0Nodes, 13, 14, 15, 16, 17),
		},
		{
			// Locators based on remote being on main chain in the
			// past and a stop hash in the main chain before that
			// point.  The expected result is the blocks after the
			// known point in the main chain and the stop hash has
			// no effect.
			name:     "remote main chain past, stop in main before",
			locator:  localView.BlockLocator(branch0Nodes[12]),
			hashStop: branch0Nodes[11].hash,
			headers:  nodeHeaders(branch0Nodes, 13, 14, 15, 16, 17),
			hashes:   nodeHashes(branch0Nodes, 13, 14, 15, 16, 17),
		},
		{
			// Locators based on remote being on main chain in the
			// past and a stop hash in the main chain exactly at that
			// point.  The expected result is the blocks after the
			// known point in the main chain and the stop hash has
			// no effect.
			name:     "remote main chain past, stop in main exact",
			locator:  localView.BlockLocator(branch0Nodes[12]),
			hashStop: branch0Nodes[12].hash,
			headers:  nodeHeaders(branch0Nodes, 13, 14, 15, 16, 17),
			hashes:   nodeHashes(branch0Nodes, 13, 14, 15, 16, 17),
		},
		{
			// Locators based on remote being on main chain in the
			// past and a stop hash in the main chain just after
			// that point.  The expected result is the blocks after
			// the known point in the main chain and the stop hash
			// has no effect.
			name:     "remote main chain past, stop in main after",
			locator:  localView.BlockLocator(branch0Nodes[12]),
			hashStop: branch0Nodes[13].hash,
			headers:  nodeHeaders(branch0Nodes, 13),
			hashes:   nodeHashes(branch0Nodes, 13),
		},
		{
			// Locators based on remote being on main chain in the
			// past and a stop hash in the main chain some time
			// after that point.  The expected result is the blocks
			// after the known point in the main chain and the stop
			// hash has no effect.
			name:     "remote main chain past, stop in main after more",
			locator:  localView.BlockLocator(branch0Nodes[12]),
			hashStop: branch0Nodes[15].hash,
			headers:  nodeHeaders(branch0Nodes, 13, 14, 15),
			hashes:   nodeHashes(branch0Nodes, 13, 14, 15),
		},
		{
			// Locators based on remote being at exactly the same
			// point in the main chain and a stop hash local node
			// doesn't know about.  The expected result is no
			// located inventory.
			name:     "remote main chain same, unknown stop",
			locator:  localView.BlockLocator(nil),
			hashStop: chainhash.Hash{0x01},
			headers:  nil,
			hashes:   nil,
		},
		{
			// Locators based on remote being at exactly the same
			// point in the main chain and a stop hash at exactly
			// the same point.  The expected result is no located
			// inventory.
			name:     "remote main chain same, stop same point",
			locator:  localView.BlockLocator(nil),
			hashStop: tip(branch0Nodes).hash,
			headers:  nil,
			hashes:   nil,
		},
		{
			// Locators from remote that don't include any blocks
			// the local node knows.  This would happen if the
			// remote node is on a completely separate chain that
			// isn't rooted with the same genesis block.  The
			// expected result is the blocks after the genesis
			// block.
			name:     "remote unrelated chain",
			locator:  unrelatedView.BlockLocator(nil),
			hashStop: chainhash.Hash{},
			headers: nodeHeaders(branch0Nodes, 0, 1, 2, 3, 4, 5, 6,
				7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17),
			hashes: nodeHashes(branch0Nodes, 0, 1, 2, 3, 4, 5, 6,
				7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17),
		},
		{
			// Locators from remote for second block in main chain
			// and no stop hash, but with an overridden max limit.
			// The expected result is the blocks after the second
			// block limited by the max.
			name:       "remote genesis",
			locator:    locatorHashes(branch0Nodes, 0),
			hashStop:   chainhash.Hash{},
			maxAllowed: 3,
			headers:    nodeHeaders(branch0Nodes, 1, 2, 3),
			hashes:     nodeHashes(branch0Nodes, 1, 2, 3),
		},
		{
			// Poorly formed locator.
			//
			// Locator from remote that only includes a single
			// block on a side chain the local node knows.  The
			// expected result is the blocks after the genesis
			// block since even though the block is known, it is on
			// a side chain and there are no more locators to find
			// the fork point.
			name:     "weak locator, single known side block",
			locator:  locatorHashes(branch1Nodes, 1),
			hashStop: chainhash.Hash{},
			headers: nodeHeaders(branch0Nodes, 0, 1, 2, 3, 4, 5, 6,
				7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17),
			hashes: nodeHashes(branch0Nodes, 0, 1, 2, 3, 4, 5, 6,
				7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17),
		},
		{
			// Poorly formed locator.
			//
			// Locator from remote that only includes multiple
			// blocks on a side chain the local node knows however
			// none in the main chain.  The expected result is the
			// blocks after the genesis block since even though the
			// blocks are known, they are all on a side chain and
			// there are no more locators to find the fork point.
			name:     "weak locator, multiple known side blocks",
			locator:  locatorHashes(branch1Nodes, 1),
			hashStop: chainhash.Hash{},
			headers: nodeHeaders(branch0Nodes, 0, 1, 2, 3, 4, 5, 6,
				7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17),
			hashes: nodeHashes(branch0Nodes, 0, 1, 2, 3, 4, 5, 6,
				7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17),
		},
		{
			// Poorly formed locator.
			//
			// Locator from remote that only includes multiple
			// blocks on a side chain the local node knows however
			// none in the main chain but includes a stop hash in
			// the main chain.  The expected result is the blocks
			// after the genesis block up to the stop hash since
			// even though the blocks are known, they are all on a
			// side chain and there are no more locators to find the
			// fork point.
			name:     "weak locator, multiple known side blocks, stop in main",
			locator:  locatorHashes(branch1Nodes, 1),
			hashStop: branch0Nodes[5].hash,
			headers:  nodeHeaders(branch0Nodes, 0, 1, 2, 3, 4, 5),
			hashes:   nodeHashes(branch0Nodes, 0, 1, 2, 3, 4, 5),
		},
	}
	for _, test := range tests {
		// Ensure the expected headers are located.
		var headers []wire.BlockHeader
		if test.maxAllowed != 0 {
			// Need to use the unexported function to override the
			// max allowed for headers.
			chain.chainLock.RLock()
			headers = chain.locateHeaders(test.locator,
				&test.hashStop, test.maxAllowed)
			chain.chainLock.RUnlock()
		} else {
			headers = chain.LocateHeaders(test.locator,
				&test.hashStop)
		}
		if !reflect.DeepEqual(headers, test.headers) {
			t.Errorf("%s: unexpected headers -- got %v, want %v",
				test.name, headers, test.headers)
			continue
		}

		// Ensure the expected block hashes are located.
		maxAllowed := uint32(wire.MaxBlocksPerMsg)
		if test.maxAllowed != 0 {
			maxAllowed = test.maxAllowed
		}
		hashes := chain.LocateBlocks(test.locator, &test.hashStop,
			maxAllowed)
		if !reflect.DeepEqual(hashes, test.hashes) {
			t.Errorf("%s: unexpected hashes -- got %v, want %v",
				test.name, hashes, test.hashes)
			continue
		}
	}
}
