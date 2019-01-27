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

	"github.com/decred/dcrd/blockchain/chaingen"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ffldb"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
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

// newFakeChain returns a chain that is usable for syntetic tests.  It is
// important to note that this chain has no database associated with it, so
// it is not usable with all functions and the tests must take care when making
// use of it.
func newFakeChain(params *chaincfg.Params) *BlockChain {
	// Create a genesis block node and block index populated with it for use
	// when creating the fake chain below.
	node := newBlockNode(&params.GenesisBlock.Header, nil)
	node.status = statusDataStored | statusValid
	index := newBlockIndex(nil, params)
	index.AddNode(node)

	return &BlockChain{
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

// findDeploymentChoice finds the provided choice ID withing the given
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

// chaingenHelpers houses several helper closures which encapsulate a test
// instance, a chaingen generator instance, and a block chain instance.  These
// are used when performing various full block tests.
//
// Use generateChaingenHelpers to return an instance of this structure with the
// described closures.
type chaingenHelpers struct {
	// accepted processes the current tip block associated with the generator
	// and expects it to be accepted to the main chain.
	accepted func()

	// rejected expects the current tip block associated with the generator to
	// be rejected with the provided error code.
	rejected func(ErrorCode)

	// expectTip expects the provided block to be the current tip of the main
	// chain associated with the generator.
	expectTip func(string)

	// acceptedToSideChainWithExpectedTip expects the tip block associated with
	// the generator to be accepted to a side chain, but the current best chain
	// tip to be the provided value.
	acceptedToSideChainWithExpectedTip func(string)

	// testThresholdState queries the threshold state from the current tip block
	// associated with the generator and expects the returned state to match the
	// provided value.
	testThresholdState func(id string, state ThresholdState)

	// forceTipReorg forces the chain instance associated with the generator to
	// reorganize the current tip of the main chain from the given block to the
	// given block.  An error will result if the provided from block is not
	// actually the current tip.
	forceTipReorg func(fromTipName, toTipName string)
}

// generateChaingenHelpers return a struct which houses several helper closures
// that encapsulate the provided state and are used when performing various full
// block tests.
//
// See chaingenHelpers for a description of the returned closures.
func generateChaingenHelpers(t *testing.T, g *chaingen.Generator, chain *BlockChain) chaingenHelpers {
	// accepted processes the current tip block associated with the generator
	// and expects it to be accepted to the main chain.
	accepted := func() {
		t.Helper()

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

	// rejected expects the current tip block associated with the generator to
	// be rejected with the provided error code.
	rejected := func(code ErrorCode) {
		t.Helper()

		msgBlock := g.Tip()
		blockHeight := msgBlock.Header.Height
		block := dcrutil.NewBlock(msgBlock)
		t.Logf("Testing block %s (hash %s, height %d)", g.TipName(),
			block.Hash(), blockHeight)

		_, _, err := chain.ProcessBlock(block, BFNone)
		if err == nil {
			t.Fatalf("block %q (hash %s, height %d) should not have been "+
				"accepted", g.TipName(), block.Hash(), blockHeight)
		}

		// Ensure the error code is of the expected type and the reject code
		// matches the value specified in the test instance.
		rerr, ok := err.(RuleError)
		if !ok {
			t.Fatalf("block %q (hash %s, height %d) returned unexpected error "+
				"type -- got %T, want blockchain.RuleError", g.TipName(),
				block.Hash(), blockHeight, err)
		}
		if rerr.ErrorCode != code {
			t.Fatalf("block %q (hash %s, height %d) does not have expected "+
				"reject code -- got %v, want %v", g.TipName(), block.Hash(),
				blockHeight, rerr.ErrorCode, code)
		}
	}

	// expectTip expects the provided block to be the current tip of the main
	// chain associated with the generator.
	expectTip := func(tipName string) {
		t.Helper()

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

	// acceptedToSideChainWithExpectedTip expects the tip block associated with
	// the generator to be accepted to a side chain, but the current best chain
	// tip to be the provided value.
	acceptedToSideChainWithExpectedTip := func(tipName string) {
		t.Helper()

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

	// lookupDeploymentVersion returns the version of the deployment with the
	// provided ID and caches the result for future invocations.  An error is
	// returned if the ID is not found.
	deploymentVersions := make(map[string]uint32)
	lookupDeploymentVersion := func(voteID string) (uint32, error) {
		if version, ok := deploymentVersions[voteID]; ok {
			return version, nil
		}

		version, _, err := findDeployment(g.Params(), voteID)
		if err != nil {
			return 0, err
		}

		deploymentVersions[voteID] = version
		return version, nil
	}

	// testThresholdState queries the threshold state from the current tip block
	// associated with the generator and expects the returned state to match the
	// provided value.
	testThresholdState := func(id string, state ThresholdState) {
		t.Helper()

		tipHash := g.Tip().BlockHash()
		deploymentVer, err := lookupDeploymentVersion(id)
		if err != nil {
			t.Fatalf("block %q (hash %s, height %d) unexpected error when "+
				"retrieving threshold state: %v", g.TipName(), tipHash,
				g.Tip().Header.Height, err)
		}

		s, err := chain.NextThresholdState(&tipHash, deploymentVer, id)
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

	// forceTipReorg forces the chain instance associated with the generator to
	// reorganize the current tip of the main chain from the given block to the
	// given block.  An error will result if the provided from block is not
	// actually the current tip.
	forceTipReorg := func(fromTipName, toTipName string) {
		t.Helper()

		from := g.BlockByName(fromTipName)
		to := g.BlockByName(toTipName)
		t.Logf("Testing forced reorg from %s (hash %s, height %d) "+
			"to %s (hash %s, height %d)", fromTipName,
			from.BlockHash(), from.Header.Height, toTipName,
			to.BlockHash(), to.Header.Height)

		err := chain.ForceHeadReorganization(from.BlockHash(), to.BlockHash())
		if err != nil {
			t.Fatalf("failed to force header reorg from block %q "+
				"(hash %s, height %d) to block %q (hash %s, "+
				"height %d): %v", fromTipName, from.BlockHash(),
				from.Header.Height, toTipName, to.BlockHash(),
				to.Header.Height, err)
		}
	}

	return chaingenHelpers{
		accepted:                           accepted,
		rejected:                           rejected,
		expectTip:                          expectTip,
		acceptedToSideChainWithExpectedTip: acceptedToSideChainWithExpectedTip,
		testThresholdState:                 testThresholdState,
		forceTipReorg:                      forceTipReorg,
	}
}
