// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"io/ioutil"
	"math"
	mrand "math/rand"
	"os"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v2"
	"github.com/decred/dcrd/blockchain/v2/chaingen"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v2"
	"github.com/decred/dcrd/database/v2"
	_ "github.com/decred/dcrd/database/v2/ffldb"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/txscript/v2"
	"github.com/decred/dcrd/wire"
)

const (
	// testDbType is the database backend type to use for the tests.
	testDbType = "ffldb"

	// blockDataNet is the expected network in the test block data.
	blockDataNet = wire.MainNet
)

// isSupportedDbType returns whether or not the passed database type is
// currently supported.
func isSupportedDbType(dbType string) bool {
	supportedDrivers := database.SupportedDrivers()
	for _, driver := range supportedDrivers {
		if dbType == driver {
			return true
		}
	}

	return false
}

// chainSetup is used to create a new db and chain instance with the genesis
// block already inserted.  In addition to the new chain instance, it returns
// a teardown function the caller should invoke when done testing to clean up.
func chainSetup(dbName string, params *chaincfg.Params) (*BlockChain, func(), error) {
	if !isSupportedDbType(testDbType) {
		return nil, nil, fmt.Errorf("unsupported db type %v", testDbType)
	}

	// Handle memory database specially since it doesn't need the disk
	// specific handling.
	var db database.DB
	var teardown func()
	if testDbType == "memdb" {
		ndb, err := database.Create(testDbType)
		if err != nil {
			return nil, nil, fmt.Errorf("error creating db: %v", err)
		}
		db = ndb

		// Setup a teardown function for cleaning up.  This function is
		// returned to the caller to be invoked when it is done testing.
		teardown = func() {
			db.Close()
		}
	} else {
		// Create the directory for test database.
		dbPath, err := ioutil.TempDir("", dbName)
		if err != nil {
			err := fmt.Errorf("unable to create test db path: %v",
				err)
			return nil, nil, err
		}

		// Create a new database to store the accepted blocks into.
		ndb, err := database.Create(testDbType, dbPath, blockDataNet)
		if err != nil {
			os.RemoveAll(dbPath)
			return nil, nil, fmt.Errorf("error creating db: %v", err)
		}
		db = ndb

		// Setup a teardown function for cleaning up.  This function is
		// returned to the caller to be invoked when it is done testing.
		teardown = func() {
			db.Close()
			os.RemoveAll(dbPath)
		}
	}

	// Copy the chain params to ensure any modifications the tests do to
	// the chain parameters do not affect the global instance.
	paramsCopy := *params

	// Create the main chain instance.
	chain, err := New(&Config{
		DB:          db,
		ChainParams: &paramsCopy,
		TimeSource:  NewMedianTime(),
		SigCache:    txscript.NewSigCache(1000),
	})

	if err != nil {
		teardown()
		err := fmt.Errorf("failed to create chain instance: %v", err)
		return nil, nil, err
	}

	return chain, teardown, nil
}

// newFakeChain returns a chain that is usable for synthetic tests.  It is
// important to note that this chain has no database associated with it, so
// it is not usable with all functions and the tests must take care when making
// use of it.
func newFakeChain(params *chaincfg.Params) *BlockChain {
	// Create a genesis block node and block index populated with it for use
	// when creating the fake chain below.
	node := newBlockNode(&params.GenesisBlock.Header, nil)
	node.status = statusDataStored | statusValid
	index := newBlockIndex(nil)
	index.AddNode(node)

	// Generate a deployment ID to version map from the provided params.
	deploymentVers, err := extractDeploymentIDVersions(params)
	if err != nil {
		panic(err)
	}

	return &BlockChain{
		deploymentVers:                deploymentVers,
		chainParams:                   params,
		deploymentCaches:              newThresholdCaches(params),
		index:                         index,
		bestChain:                     newChainView(node),
		isVoterMajorityVersionCache:   make(map[[stakeMajorityCacheKeySize]byte]bool),
		isStakeMajorityVersionCache:   make(map[[stakeMajorityCacheKeySize]byte]bool),
		calcPriorStakeVersionCache:    make(map[[chainhash.HashSize]byte]uint32),
		calcVoterVersionIntervalCache: make(map[[chainhash.HashSize]byte]uint32),
		calcStakeVersionCache:         make(map[[chainhash.HashSize]byte]uint32),
	}
}

// testNoncePrng provides a deterministic prng for the nonce in generated fake
// nodes.  The ensures that the nodes have unique hashes.
var testNoncePrng = mrand.New(mrand.NewSource(0))

// newFakeNode creates a block node connected to the passed parent with the
// provided fields populated and fake values for the other fields.
func newFakeNode(parent *blockNode, blockVersion int32, stakeVersion uint32, bits uint32, timestamp time.Time) *blockNode {
	// Make up a header and create a block node from it.
	var prevHash chainhash.Hash
	var height uint32
	if parent != nil {
		prevHash = parent.hash
		height = uint32(parent.height + 1)
	}
	header := &wire.BlockHeader{
		Version:      blockVersion,
		PrevBlock:    prevHash,
		VoteBits:     0x01,
		Bits:         bits,
		Height:       height,
		Timestamp:    timestamp,
		Nonce:        testNoncePrng.Uint32(),
		StakeVersion: stakeVersion,
	}
	node := newBlockNode(header, parent)
	node.status = statusDataStored | statusValid
	return node
}

// chainedFakeNodes returns the specified number of nodes constructed such that
// each subsequent node points to the previous one to create a chain.  The first
// node will point to the passed parent which can be nil if desired.
func chainedFakeNodes(parent *blockNode, numNodes int) []*blockNode {
	nodes := make([]*blockNode, numNodes)
	tip := parent
	blockTime := time.Now()
	if tip != nil {
		blockTime = time.Unix(tip.timestamp, 0)
	}
	for i := 0; i < numNodes; i++ {
		blockTime = blockTime.Add(time.Second)
		node := newFakeNode(tip, 1, 1, 0, blockTime)
		tip = node

		nodes[i] = node
	}
	return nodes
}

// branchTip is a convenience function to grab the tip of a chain of block nodes
// created via chainedFakeNodes.
func branchTip(nodes []*blockNode) *blockNode {
	return nodes[len(nodes)-1]
}

// appendFakeVotes appends the passed number of votes to the node with the
// provided version and vote bits.
func appendFakeVotes(node *blockNode, numVotes uint16, voteVersion uint32, voteBits uint16) {
	for i := uint16(0); i < numVotes; i++ {
		node.votes = append(node.votes, stake.VoteVersionTuple{
			Version: voteVersion,
			Bits:    voteBits,
		})
	}
}

// findDeployment finds the provided vote ID within the deployments of the
// provided parameters and either returns the deployment version it was found in
// along with a pointer to the deployment or an error when not found.
func findDeployment(params *chaincfg.Params, voteID string) (uint32, *chaincfg.ConsensusDeployment, error) {
	// Find the correct deployment for the passed vote ID.
	for version, deployments := range params.Deployments {
		for i, deployment := range deployments {
			if deployment.Vote.Id == voteID {
				return version, &deployments[i], nil
			}
		}
	}

	return 0, nil, fmt.Errorf("unable to find deployement for id %q", voteID)
}

// findDeploymentChoice finds the provided choice ID within the given
// deployment params and either returns a pointer to the found choice or an
// error when not found.
func findDeploymentChoice(deployment *chaincfg.ConsensusDeployment, choiceID string) (*chaincfg.Choice, error) {
	// Find the correct choice for the passed choice ID.
	for i, choice := range deployment.Vote.Choices {
		if choice.Id == choiceID {
			return &deployment.Vote.Choices[i], nil
		}
	}

	return nil, fmt.Errorf("unable to find vote choice for id %q", choiceID)
}

// removeDeploymentTimeConstraints modifies the passed deployment to remove the
// voting time constraints by making it always available to vote and to never
// expire.
//
// NOTE: This will mutate the passed deployment, so ensure this function is
// only called with parameters that are not globally available.
func removeDeploymentTimeConstraints(deployment *chaincfg.ConsensusDeployment) {
	deployment.StartTime = 0               // Always available for vote.
	deployment.ExpireTime = math.MaxUint64 // Never expires.
}

// chaingenHarness provides a test harness which encapsulates a test instance, a
// chaingen generator instance, and a block chain instance to provide all of the
// functionality of the aforementioned types as well as several convenience
// functions such as block acceptance and rejection, expected tip checking, and
// threshold state checking.
//
// The chaingen generator is embedded in the struct so callers can directly
// access its method the same as if they were directly working with the
// underlying generator.
//
// Since chaingen involves creating fully valid and solved blocks, which is
// relatively expensive, only tests which actually require that functionality
// should make use of this harness.  In many cases, a much faster synthetic
// chain instance created by newFakeChain will suffice.
type chaingenHarness struct {
	*chaingen.Generator

	t                  *testing.T
	chain              *BlockChain
	deploymentVersions map[string]uint32
}

// newChaingenHarness creates and returns a new instance of a chaingen harness
// that encapsulates the provided test instance along with a teardown function
// the caller should invoke when done testing to clean up.
//
// See the documentation for the chaingenHarness type for more details.
func newChaingenHarness(t *testing.T, params *chaincfg.Params, dbName string) (*chaingenHarness, func()) {
	t.Helper()

	// Create a test generator instance initialized with the genesis block as
	// the tip.
	g, err := chaingen.MakeGenerator(params)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup(dbName, params)
	if err != nil {
		t.Fatalf("Failed to setup chain instance: %v", err)
	}

	harness := chaingenHarness{
		Generator:          &g,
		t:                  t,
		chain:              chain,
		deploymentVersions: make(map[string]uint32),
	}
	return &harness, teardownFunc
}

// AcceptBlock processes the block associated with the given name in the
// harness generator and expects it to be accepted to the main chain.
func (g *chaingenHarness) AcceptBlock(blockName string) {
	g.t.Helper()

	msgBlock := g.BlockByName(blockName)
	blockHeight := msgBlock.Header.Height
	block := dcrutil.NewBlock(msgBlock)
	g.t.Logf("Testing block %s (hash %s, height %d)", blockName, block.Hash(),
		blockHeight)

	forkLen, isOrphan, err := g.chain.ProcessBlock(block, BFNone)
	if err != nil {
		g.t.Fatalf("block %q (hash %s, height %d) should have been accepted: %v",
			blockName, block.Hash(), blockHeight, err)
	}

	// Ensure the main chain and orphan flags match the values specified in the
	// test.
	isMainChain := !isOrphan && forkLen == 0
	if !isMainChain {
		g.t.Fatalf("block %q (hash %s, height %d) unexpected main chain flag "+
			"-- got %v, want true", blockName, block.Hash(), blockHeight,
			isMainChain)
	}
	if isOrphan {
		g.t.Fatalf("block %q (hash %s, height %d) unexpected orphan flag -- "+
			"got %v, want false", blockName, block.Hash(), blockHeight,
			isOrphan)
	}
}

// AcceptTipBlock processes the current tip block associated with the harness
// generator and expects it to be accepted to the main chain.
func (g *chaingenHarness) AcceptTipBlock() {
	g.t.Helper()

	g.AcceptBlock(g.TipName())
}

// RejectBlock expects the block associated with the given name in the harness
// generator to be rejected with the provided error code.
func (g *chaingenHarness) RejectBlock(blockName string, code ErrorCode) {
	g.t.Helper()

	msgBlock := g.BlockByName(blockName)
	blockHeight := msgBlock.Header.Height
	block := dcrutil.NewBlock(msgBlock)
	g.t.Logf("Testing block %s (hash %s, height %d)", blockName, block.Hash(),
		blockHeight)

	_, _, err := g.chain.ProcessBlock(block, BFNone)
	if err == nil {
		g.t.Fatalf("block %q (hash %s, height %d) should not have been accepted",
			blockName, block.Hash(), blockHeight)
	}

	// Ensure the error code is of the expected type and the reject code matches
	// the value specified in the test instance.
	rerr, ok := err.(RuleError)
	if !ok {
		g.t.Fatalf("block %q (hash %s, height %d) returned unexpected error "+
			"type -- got %T, want blockchain.RuleError", blockName,
			block.Hash(), blockHeight, err)
	}
	if rerr.ErrorCode != code {
		g.t.Fatalf("block %q (hash %s, height %d) does not have expected reject "+
			"code -- got %v, want %v", blockName, block.Hash(), blockHeight,
			rerr.ErrorCode, code)
	}
}

// RejectTipBlock expects the current tip block associated with the harness
// generator to be rejected with the provided error code.
func (g *chaingenHarness) RejectTipBlock(code ErrorCode) {
	g.t.Helper()

	g.RejectBlock(g.TipName(), code)
}

// ExpectTip expects the provided block to be the current tip of the main chain
// associated with the harness generator.
func (g *chaingenHarness) ExpectTip(tipName string) {
	g.t.Helper()

	// Ensure hash and height match.
	wantTip := g.BlockByName(tipName)
	best := g.chain.BestSnapshot()
	if best.Hash != wantTip.BlockHash() ||
		best.Height != int64(wantTip.Header.Height) {
		g.t.Fatalf("block %q (hash %s, height %d) should be the current tip "+
			"-- got (hash %s, height %d)", tipName, wantTip.BlockHash(),
			wantTip.Header.Height, best.Hash, best.Height)
	}
}

// AcceptedToSideChainWithExpectedTip expects the tip block associated with the
// generator to be accepted to a side chain, but the current best chain tip to
// be the provided value.
func (g *chaingenHarness) AcceptedToSideChainWithExpectedTip(tipName string) {
	g.t.Helper()

	msgBlock := g.Tip()
	blockHeight := msgBlock.Header.Height
	block := dcrutil.NewBlock(msgBlock)
	g.t.Logf("Testing block %s (hash %s, height %d)", g.TipName(), block.Hash(),
		blockHeight)

	forkLen, isOrphan, err := g.chain.ProcessBlock(block, BFNone)
	if err != nil {
		g.t.Fatalf("block %q (hash %s, height %d) should have been accepted: %v",
			g.TipName(), block.Hash(), blockHeight, err)
	}

	// Ensure the main chain and orphan flags match the values specified in
	// the test.
	isMainChain := !isOrphan && forkLen == 0
	if isMainChain {
		g.t.Fatalf("block %q (hash %s, height %d) unexpected main chain flag "+
			"-- got %v, want false", g.TipName(), block.Hash(), blockHeight,
			isMainChain)
	}
	if isOrphan {
		g.t.Fatalf("block %q (hash %s, height %d) unexpected orphan flag -- "+
			"got %v, want false", g.TipName(), block.Hash(), blockHeight,
			isOrphan)
	}

	g.ExpectTip(tipName)
}

// lookupDeploymentVersion returns the version of the deployment with the
// provided ID and caches the result for future invocations.  An error is
// returned if the ID is not found.
func (g *chaingenHarness) lookupDeploymentVersion(voteID string) (uint32, error) {
	if version, ok := g.deploymentVersions[voteID]; ok {
		return version, nil
	}

	version, _, err := findDeployment(g.Params(), voteID)
	if err != nil {
		return 0, err
	}

	g.deploymentVersions[voteID] = version
	return version, nil
}

// TestThresholdState queries the threshold state from the current tip block
// associated with the harness generator and expects the returned state to match
// the provided value.
func (g *chaingenHarness) TestThresholdState(id string, state ThresholdState) {
	g.t.Helper()

	tipHash := g.Tip().BlockHash()
	tipHeight := g.Tip().Header.Height
	deploymentVer, err := g.lookupDeploymentVersion(id)
	if err != nil {
		g.t.Fatalf("block %q (hash %s, height %d) unexpected error when "+
			"retrieving threshold state: %v", g.TipName(), tipHash, tipHeight,
			err)
	}

	s, err := g.chain.NextThresholdState(&tipHash, deploymentVer, id)
	if err != nil {
		g.t.Fatalf("block %q (hash %s, height %d) unexpected error when "+
			"retrieving threshold state: %v", g.TipName(), tipHash, tipHeight,
			err)
	}

	if s.State != state {
		g.t.Fatalf("block %q (hash %s, height %d) unexpected threshold "+
			"state for %s -- got %v, want %v", g.TipName(), tipHash, tipHeight,
			id, s.State, state)
	}
}

// TestThresholdStateChoice queries the threshold state from the current tip
// block associated with the harness generator and expects the returned state
// and choice to match the provided value.
func (g *chaingenHarness) TestThresholdStateChoice(id string, state ThresholdState, choice uint32) {
	g.t.Helper()

	tipHash := g.Tip().BlockHash()
	tipHeight := g.Tip().Header.Height
	deploymentVer, err := g.lookupDeploymentVersion(id)
	if err != nil {
		g.t.Fatalf("block %q (hash %s, height %d) unexpected error when "+
			"retrieving threshold state: %v", g.TipName(), tipHash, tipHeight,
			err)
	}

	s, err := g.chain.NextThresholdState(&tipHash, deploymentVer, id)
	if err != nil {
		g.t.Fatalf("block %q (hash %s, height %d) unexpected error when "+
			"retrieving threshold state: %v", g.TipName(), tipHash, tipHeight,
			err)
	}

	if s.State != state {
		g.t.Fatalf("block %q (hash %s, height %d) unexpected threshold "+
			"state for %s -- got %v, want %v", g.TipName(), tipHash, tipHeight,
			id, s.State, state)
	}
	if s.Choice != choice {
		g.t.Fatalf("block %q (hash %s, height %d) unexpected choice for %s -- "+
			"got %v, want %v", g.TipName(), tipHash, tipHeight, id, s.Choice,
			choice)
	}
}

// ForceTipReorg forces the chain instance associated with the generator to
// reorganize the current tip of the main chain from the given block to the
// given block.  An error will result if the provided from block is not actually
// the current tip.
func (g *chaingenHarness) ForceTipReorg(fromTipName, toTipName string) {
	g.t.Helper()

	from := g.BlockByName(fromTipName)
	to := g.BlockByName(toTipName)
	g.t.Logf("Testing forced reorg from %s (hash %s, height %d) to %s (hash "+
		"%s, height %d)", fromTipName, from.BlockHash(), from.Header.Height,
		toTipName, to.BlockHash(), to.Header.Height)

	err := g.chain.ForceHeadReorganization(from.BlockHash(), to.BlockHash())
	if err != nil {
		g.t.Fatalf("failed to force header reorg from block %q (hash %s, "+
			"height %d) to block %q (hash %s, height %d): %v", fromTipName,
			from.BlockHash(), from.Header.Height, toTipName, to.BlockHash(),
			to.Header.Height, err)
	}
}

// AdvanceToStakeValidationHeight generates and accepts enough blocks to the
// chain instance associated with the harness to reach stake validation height.
//
// The function will fail with a fatal test error if it is not called with the
// harness at the genesis block which is the case when it is first created.
func (g *chaingenHarness) AdvanceToStakeValidationHeight() {
	g.t.Helper()

	// Only allow this to be called on a newly created harness.
	if g.Tip().Header.Height != 0 {
		g.t.Fatalf("chaingen harness instance must be at the genesis block " +
			"to advance to stake validation height")
	}

	// Shorter versions of useful params for convenience.
	params := g.Params()
	ticketsPerBlock := params.TicketsPerBlock
	coinbaseMaturity := params.CoinbaseMaturity
	stakeEnabledHeight := params.StakeEnabledHeight
	stakeValidationHeight := params.StakeValidationHeight

	// ---------------------------------------------------------------------
	// Block One.
	// ---------------------------------------------------------------------

	// Add the required first block.
	//
	//   genesis -> bfb
	g.CreateBlockOne("bfb", 0)
	g.AssertTipHeight(1)
	g.AcceptTipBlock()

	// ---------------------------------------------------------------------
	// Generate enough blocks to have mature coinbase outputs to work with.
	//
	//   genesis -> bfb -> bm0 -> bm1 -> ... -> bm#
	// ---------------------------------------------------------------------

	for i := uint16(0); i < coinbaseMaturity; i++ {
		blockName := fmt.Sprintf("bm%d", i)
		g.NextBlock(blockName, nil, nil)
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
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
		g.AcceptTipBlock()
	}
	g.AssertTipHeight(uint32(stakeEnabledHeight))

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake validation height while
	// continuing to purchase tickets using the coinbases matured above and
	// allowing the immature tickets to mature and thus become live.
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
		g.AcceptTipBlock()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight))
}

// AdvanceFromSVHToActiveAgenda generates and accepts enough blocks with the
// appropriate vote bits set to reach one block prior to the specified agenda
// becoming active.
//
// The function will fail with a fatal test error if it is called when the
// harness is not at stake validation height.
//
// WARNING: This function currently assumes the chain parameters were created
// via the quickVoteActivationParams.  It should be updated in the future to
// work with arbitrary params.
func (g *chaingenHarness) AdvanceFromSVHToActiveAgenda(voteID string) {
	g.t.Helper()

	// Find the correct deployment for the provided ID along with the yes
	// vote choice within it.
	params := g.Params()
	deploymentVer, deployment, err := findDeployment(params, voteID)
	if err != nil {
		g.t.Fatal(err)
	}
	yesChoice, err := findDeploymentChoice(deployment, "yes")
	if err != nil {
		g.t.Fatal(err)
	}

	// Shorter versions of useful params for convenience.
	stakeValidationHeight := params.StakeValidationHeight
	stakeVerInterval := params.StakeVersionInterval
	ruleChangeInterval := int64(params.RuleChangeActivationInterval)

	// Only allow this to be called on a harness at SVH.
	if g.Tip().Header.Height != uint32(stakeValidationHeight) {
		g.t.Fatalf("chaingen harness instance must be at the genesis block " +
			"to advance to stake validation height")
	}

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next two stake
	// version intervals with block and vote versions for the agenda and
	// stake version 0.
	//
	// This will result in triggering enforcement of the stake version and
	// that the stake version is the deployment version.  The threshold
	// state for deployment will move to started since the next block also
	// coincides with the start of a new rule change activation interval for
	// the chosen parameters.
	//
	//   ... -> bsv# -> bvu0 -> bvu1 -> ... -> bvu#
	// ---------------------------------------------------------------------

	blocksNeeded := stakeValidationHeight + stakeVerInterval*2 - 1 -
		int64(g.Tip().Header.Height)
	for i := int64(0); i < blocksNeeded; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bvu%d", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(int32(deploymentVer)),
			chaingen.ReplaceVoteVersions(deploymentVer))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
	}
	g.TestThresholdState(voteID, ThresholdStarted)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the next rule change interval with
	// block, stake, and vote versions for the agenda.  Also, set the vote
	// bits to include yes votes for the agenda.
	//
	// This will result in moving the threshold state for the agenda to
	// locked in.
	//
	//   ... -> bvu# -> bvli0 -> bvli1 -> ... -> bvli#
	// ---------------------------------------------------------------------

	blocksNeeded = stakeValidationHeight + ruleChangeInterval*2 - 1 -
		int64(g.Tip().Header.Height)
	for i := int64(0); i < blocksNeeded; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bvli%d", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(int32(deploymentVer)),
			chaingen.ReplaceStakeVersion(deploymentVer),
			chaingen.ReplaceVotes(vbPrevBlockValid|yesChoice.Bits, deploymentVer))
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + ruleChangeInterval*2 - 1))
	g.AssertBlockVersion(int32(deploymentVer))
	g.AssertStakeVersion(deploymentVer)
	g.TestThresholdState(voteID, ThresholdLockedIn)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the next rule change interval with
	// block, stake, and vote versions for the agenda.
	//
	// This will result in moving the threshold state for the agenda to
	// active thereby activating it.
	//
	//   ... -> bvli# -> bva0 -> bva1 -> ... -> bva#
	// ---------------------------------------------------------------------

	blocksNeeded = stakeValidationHeight + ruleChangeInterval*3 - 1 -
		int64(g.Tip().Header.Height)
	for i := int64(0); i < blocksNeeded; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bva%d", i)
		g.NextBlock(blockName, nil, outs[1:],
			chaingen.ReplaceBlockVersion(int32(deploymentVer)),
			chaingen.ReplaceStakeVersion(deploymentVer),
			chaingen.ReplaceVoteVersions(deploymentVer),
		)
		g.SaveTipCoinbaseOuts()
		g.AcceptTipBlock()
	}
	g.AssertTipHeight(uint32(stakeValidationHeight + ruleChangeInterval*3 - 1))
	g.AssertBlockVersion(int32(deploymentVer))
	g.AssertStakeVersion(deploymentVer)
	g.TestThresholdState(voteID, ThresholdActive)
}
