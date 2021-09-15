// Copyright (c) 2018-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"math/big"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/wire"
)

// TestBlockNodeHeader ensures that block nodes reconstruct the correct header
// and fetching the header from the chain reconstructs it from memory.
func TestBlockNodeHeader(t *testing.T) {
	// Create a fake chain and block header with all fields set to nondefault
	// values.
	params := chaincfg.RegNetParams()
	bc := newFakeChain(params)
	tip := bc.bestChain.Tip()
	testHeader := wire.BlockHeader{
		Version:      1,
		PrevBlock:    tip.hash,
		MerkleRoot:   *mustParseHash("09876543210987654321"),
		StakeRoot:    *mustParseHash("43210987654321098765"),
		VoteBits:     0x03,
		FinalState:   [6]byte{0xaa},
		Voters:       4,
		FreshStake:   5,
		Revocations:  6,
		PoolSize:     20000,
		Bits:         0x1234,
		SBits:        123456789,
		Height:       1,
		Size:         393216,
		Timestamp:    time.Unix(1454954400, 0),
		Nonce:        7,
		ExtraData:    [32]byte{0xbb},
		StakeVersion: 5,
	}
	node := newBlockNode(&testHeader, tip)
	bc.index.AddNode(node)

	// Ensure reconstructing the header for the node produces the same header
	// used to create the node.
	gotHeader := node.Header()
	if !reflect.DeepEqual(gotHeader, testHeader) {
		t.Fatalf("node.Header: mismatched headers: got %+v, want %+v",
			gotHeader, testHeader)
	}

	// Ensure fetching the header from the chain produces the same header used
	// to create the node.
	testHeaderHash := testHeader.BlockHash()
	gotHeader, err := bc.HeaderByHash(&testHeaderHash)
	if err != nil {
		t.Fatalf("HeaderByHash: unexpected error: %v", err)
	}
	if !reflect.DeepEqual(gotHeader, testHeader) {
		t.Fatalf("HeaderByHash: mismatched headers: got %+v, want %+v",
			gotHeader, testHeader)
	}
}

// TestCalcPastMedianTime ensures the CalcPastMedianTie function works as
// intended including when there are less than the typical number of blocks
// which happens near the beginning of the chain.
func TestCalcPastMedianTime(t *testing.T) {
	tests := []struct {
		name       string
		timestamps []int64
		expected   int64
	}{
		{
			name:       "one block",
			timestamps: []int64{1517188771},
			expected:   1517188771,
		},
		{
			name:       "two blocks, in order",
			timestamps: []int64{1517188771, 1517188831},
			expected:   1517188771,
		},
		{
			name:       "three blocks, in order",
			timestamps: []int64{1517188771, 1517188831, 1517188891},
			expected:   1517188831,
		},
		{
			name:       "three blocks, out of order",
			timestamps: []int64{1517188771, 1517188891, 1517188831},
			expected:   1517188831,
		},
		{
			name:       "four blocks, in order",
			timestamps: []int64{1517188771, 1517188831, 1517188891, 1517188951},
			expected:   1517188831,
		},
		{
			name:       "four blocks, out of order",
			timestamps: []int64{1517188831, 1517188771, 1517188951, 1517188891},
			expected:   1517188831,
		},
		{
			name: "eleven blocks, in order",
			timestamps: []int64{1517188771, 1517188831, 1517188891, 1517188951,
				1517189011, 1517189071, 1517189131, 1517189191, 1517189251,
				1517189311, 1517189371},
			expected: 1517189071,
		},
		{
			name: "eleven blocks, out of order",
			timestamps: []int64{1517188831, 1517188771, 1517188891, 1517189011,
				1517188951, 1517189071, 1517189131, 1517189191, 1517189251,
				1517189371, 1517189311},
			expected: 1517189071,
		},
		{
			name: "fifteen blocks, in order",
			timestamps: []int64{1517188771, 1517188831, 1517188891, 1517188951,
				1517189011, 1517189071, 1517189131, 1517189191, 1517189251,
				1517189311, 1517189371, 1517189431, 1517189491, 1517189551,
				1517189611},
			expected: 1517189311,
		},
		{
			name: "fifteen blocks, out of order",
			timestamps: []int64{1517188771, 1517188891, 1517188831, 1517189011,
				1517188951, 1517189131, 1517189071, 1517189251, 1517189191,
				1517189371, 1517189311, 1517189491, 1517189431, 1517189611,
				1517189551},
			expected: 1517189311,
		},
	}

	// Ensure the genesis block timestamp of the test params is before the test
	// data.  Also, clone the provided parameters first to avoid mutating them.
	//
	// The timestamp corresponds to 2018-01-01 00:00:00 +0000 UTC.
	params := chaincfg.RegNetParams()
	params.GenesisBlock.Header.Timestamp = time.Unix(1514764800, 0)
	params.GenesisHash = params.GenesisBlock.BlockHash()

	for _, test := range tests {
		// Create a synthetic chain with the correct number of nodes and the
		// timestamps as specified by the test.
		bc := newFakeChain(params)
		node := bc.bestChain.Tip()
		for _, timestamp := range test.timestamps {
			node = newFakeNode(node, 0, 0, 0, time.Unix(timestamp, 0))
			bc.index.AddNode(node)
			bc.bestChain.SetTip(node)
		}

		// Ensure the median time is the expected value.
		gotTime := node.CalcPastMedianTime()
		wantTime := time.Unix(test.expected, 0)
		if !gotTime.Equal(wantTime) {
			t.Errorf("%s: mismatched timestamps -- got: %v, want: %v",
				test.name, gotTime, wantTime)
			continue
		}
	}
}

// TestChainTips ensures the chain tip tracking in the block index works
// as expected.
func TestChainTips(t *testing.T) {
	params := chaincfg.RegNetParams()
	bc := newFakeChain(params)
	genesis := bc.bestChain.NodeByHeight(0)

	// Construct a synthetic simnet chain consisting of the following structure.
	// 0 -> 1 -> 2  -> 3  -> 4
	//  |    \-> 2a -> 3a -> 4a -> 5a -> 6a -> 7a -> ... -> 26a
	//  |    |     \-> 3b -> 4b -> 5b
	//  |    \-> 2c -> 3c -> 4c -> 5c -> 6c -> 7c -> ... -> 26c
	//  \-> 1d
	//  |     \
	//  \-> 1e |
	//         \-> 2f (added after 1e)

	branches := make([][]*blockNode, 7)
	branches[0] = chainedFakeNodes(genesis, 4)
	branches[1] = chainedFakeNodes(branches[0][0], 25)
	branches[2] = chainedFakeNodes(branches[1][0], 3)
	branches[3] = chainedFakeNodes(branches[0][0], 25)
	branches[4] = chainedFakeNodes(genesis, 1)
	branches[5] = chainedFakeNodes(genesis, 1)
	branches[6] = chainedFakeNodes(branches[4][0], 1)

	// Add all of the nodes to the index.
	for _, branch := range branches {
		for _, node := range branch {
			bc.index.AddNode(node)
		}
	}

	// Create a map of all of the chain tips the block index believes exist.
	chainTips := make(map[*blockNode]struct{})
	bc.index.RLock()
	for _, entry := range bc.index.chainTips {
		chainTips[entry.tip] = struct{}{}
		for _, node := range entry.otherTips {
			chainTips[node] = struct{}{}
		}
	}
	bc.index.RUnlock()

	// Exclude tips that are part of an earlier set of branch nodes that was
	// built on via a new set of branch nodes.
	excludeExpected := make(map[*blockNode]struct{})
	excludeExpected[branchTip(branches[4])] = struct{}{}

	// The expected chain tips are the tips of all of the branches minus any
	// that were excluded.
	expectedTips := make(map[*blockNode]struct{})
	for _, branch := range branches {
		if _, ok := excludeExpected[branchTip(branch)]; ok {
			continue
		}
		expectedTips[branchTip(branch)] = struct{}{}
	}

	// Ensure the chain tips are the expected values.
	if len(chainTips) != len(expectedTips) {
		t.Fatalf("block index reports %d chain tips, but %d were expected",
			len(chainTips), len(expectedTips))
	}
	for node := range expectedTips {
		if _, ok := chainTips[node]; !ok {
			t.Fatalf("block index does not contain expected tip %s (height %d)",
				node.hash, node.height)
		}
	}
}

// TestAncestorSkipList ensures the skip list functionality and ancestor
// traversal that makes use of it works as expected.
func TestAncestorSkipList(t *testing.T) {
	// Create fake nodes to use for skip list traversal.
	nodes := chainedFakeSkipListNodes(nil, 250000)

	// Ensure the skip list is constructed correctly by checking that each node
	// points to an ancestor with a lower height and that said ancestor is
	// actually the node at that height.
	for i, node := range nodes[1:] {
		ancestorHeight := node.skipToAncestor.height
		if ancestorHeight >= int64(i+1) {
			t.Fatalf("height for skip list pointer %d is not lower than "+
				"current node height %d", ancestorHeight, int64(i+1))
		}

		if node.skipToAncestor != nodes[ancestorHeight] {
			t.Fatalf("unexpected node for skip list pointer for height %d",
				ancestorHeight)
		}
	}

	// Use a unique random seed each test instance and log it if the tests fail.
	seed := time.Now().Unix()
	rng := rand.New(rand.NewSource(seed))
	defer func(t *testing.T, seed int64) {
		if t.Failed() {
			t.Logf("random seed: %d", seed)
		}
	}(t, seed)

	for i := 0; i < 2500; i++ {
		// Ensure obtaining the ancestor at a random starting height from the
		// tip is the expected node.
		startHeight := rng.Int63n(int64(len(nodes) - 1))
		startNode := nodes[startHeight]
		if branchTip(nodes).Ancestor(startHeight) != startNode {
			t.Fatalf("unexpected ancestor for height %d from tip",
				startHeight)
		}

		// Ensure obtaining the ancestor at height 0 starting from the node at
		// the random starting height is the expected node.
		if startNode.Ancestor(0) != nodes[0] {
			t.Fatalf("unexpected ancestor for height 0 from start height %d",
				startHeight)
		}

		// Ensure obtaining the ancestor from a random ending height starting
		// from the node at the random starting height is the expected node.
		endHeight := rng.Int63n(startHeight + 1)
		if startNode.Ancestor(endHeight) != nodes[endHeight] {
			t.Fatalf("unexpected ancestor for height %d from start height %d",
				endHeight, startHeight)
		}
	}
}

// TestWorkSorterCompare ensures the work sorter less and hash comparison
// functions work as intended including multiple keys.
func TestWorkSorterCompare(t *testing.T) {
	lowerHash := mustParseHash("000000000000c41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba")
	higherHash := mustParseHash("000000000000d41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba")
	tests := []struct {
		name     string     // test description
		nodeA    *blockNode // first node to compare
		nodeB    *blockNode // second node to compare
		wantCmp  int        // expected result of the hash comparison
		wantLess bool       // expected result of the less comparison

	}{{
		name: "exactly equal, both data",
		nodeA: &blockNode{
			hash:            *mustParseHash("0000000000000000000000000000000000000000000000000000000000000000"),
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *mustParseHash("0000000000000000000000000000000000000000000000000000000000000000"),
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		wantCmp:  0,
		wantLess: false,
	}, {
		name: "exactly equal, no data",
		nodeA: &blockNode{
			hash:            *mustParseHash("0000000000000000000000000000000000000000000000000000000000000000"),
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *mustParseHash("0000000000000000000000000000000000000000000000000000000000000000"),
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		wantCmp:  0,
		wantLess: false,
	}, {
		name: "a has more cumulative work, same order, higher hash, a has data",
		nodeA: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(4),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		wantCmp:  1,
		wantLess: false,
	}, {
		name: "a has less cumulative work, same order, lower hash, b has data",
		nodeA: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(4),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		wantCmp:  -1,
		wantLess: true,
	}, {
		name: "a has same cumulative work, same order, lower hash, a has data",
		nodeA: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		wantCmp:  -1,
		wantLess: false,
	}, {
		name: "a has same cumulative work, same order, higher hash, b has data",
		nodeA: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		wantCmp:  1,
		wantLess: true,
	}, {
		name: "a has same cumulative work, higher order, lower hash, both data",
		nodeA: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 1,
		},
		nodeB: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		wantCmp:  -1,
		wantLess: true,
	}, {
		name: "a has same cumulative work, lower order, lower hash, both data",
		nodeA: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 1,
		},
		nodeB: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 2,
		},
		wantCmp:  -1,
		wantLess: false,
	}, {
		name: "a has same cumulative work, same order, lower hash, no data",
		nodeA: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		wantCmp:  -1,
		wantLess: false,
	}, {
		name: "a has same cumulative work, same order, lower hash, both data",
		nodeA: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		wantCmp:  -1,
		wantLess: false,
	}, {
		name: "a has same cumulative work, same order, higher hash, both data",
		nodeA: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		wantCmp:  1,
		wantLess: true,
	}}

	for _, test := range tests {
		gotLess := workSorterLess(test.nodeA, test.nodeB)
		if gotLess != test.wantLess {
			t.Fatalf("%q: unexpected result -- got %v, want %v", test.name,
				gotLess, test.wantLess)
		}

		gotCmp := compareHashesAsUint256LE(&test.nodeA.hash, &test.nodeB.hash)
		if gotCmp != test.wantCmp {
			t.Fatalf("%q: unexpected result -- got %v, want %v", test.name,
				gotCmp, test.wantCmp)
		}
	}
}

// TestShortBlockKeyCollisions ensures the block index handles addition and
// lookup of short key collisions as expected.
func TestShortBlockKeyCollisions(t *testing.T) {
	params := chaincfg.RegNetParams()
	bc := newFakeChain(params)
	genesis := bc.bestChain.Genesis()

	// Assert the regression test genesis block hasn't changed because this test
	// relies on specific hashes that are specially crafted to force collisions.
	// Should this assertion fail in the future, it can be resolved by
	// overriding the parameters above to match the expected values.
	requiredHash := "2ced94b4ae95bba344cfa043268732d230649c640f92dce2d9518823d3057cb0"
	if genesis.hash != *mustParseHash(requiredHash) {
		t.Fatalf("this test relies on specific hash values which stem from a "+
			"regression test genesis block with hash %s, but it has been "+
			"changed to %s", requiredHash, genesis.hash)
	}

	// assertShortKeyCollision causes a fatal test error if the short key for
	// the two provided nodes is not the same.
	assertShortKeyCollision := func(node1, node2 *blockNode) {
		t.Helper()

		node1Key := shortBlockKey(&node1.hash)
		node2Key := shortBlockKey(&node2.hash)
		if node1Key != node2Key {
			t.Fatalf("test data did not produce a key collision: %d != %d",
				node1Key, node2Key)
		}
	}

	// assertFullKeyCollision causes a fatal test error if the full key for
	// the two provided nodes is not the same.
	assertFullKeyCollision := func(node1, node2 *blockNode) {
		t.Helper()

		if node1.hash != node2.hash {
			t.Fatalf("test data did not produce a key collision: %s != %s",
				node1.hash, node2.hash)
		}
	}

	// assertLookupResult causes a fatal test error if looking up the provided
	// node from the block index does not produce given desired node.
	assertLookupResult := func(lookup, want *blockNode) {
		t.Helper()

		got := bc.index.lookupNode(&lookup.hash)
		if got != want {
			gotStr, wantStr := "nil", "nil"
			if got != nil {
				gotStr = fmt.Sprintf("%s (short %x, pointer %p)", got.hash,
					shortBlockKey(&got.hash), got)
			}
			if want != nil {
				wantStr = fmt.Sprintf("%s (short %x, pointer %p)", want.hash,
					shortBlockKey(&want.hash), want)
			}
			t.Fatalf("failed to lookup %s (short %x) -- got %s, want %s",
				lookup.hash, shortBlockKey(&lookup.hash), gotStr, wantStr)
		}
	}

	// mutatedHeader returns a copy of the header from the passed node modified
	// to increase its height and timestamp by one second and to have the
	// provided nonce and extra data byte.
	mutatedHeader := func(node *blockNode, nonce uint32, extraData byte) *wire.BlockHeader {
		headerCopy := node.Header()
		headerCopy.Height++
		headerCopy.Timestamp = headerCopy.Timestamp.Add(time.Second)
		headerCopy.Nonce = nonce
		headerCopy.ExtraData[0] = extraData
		return &headerCopy
	}

	// Create a few nodes that do not have a collision with the short key.
	nodes := make([]*blockNode, 10)
	nodes[0] = newBlockNode(mutatedHeader(genesis, 0x1234, 0x00), genesis)
	nodes[1] = newBlockNode(mutatedHeader(nodes[0], 0x1235, 0x00), nodes[0])
	nodes[2] = newBlockNode(mutatedHeader(nodes[1], 0x1236, 0x00), nodes[1])
	nodes[3] = newBlockNode(mutatedHeader(nodes[2], 0x1237, 0x00), nodes[2])
	collisionFreeNodes := nodes[0:4]

	// Create some nodes that are specifically crafted to have different full
	// hashes that collide with the short key.
	nodes[4] = newBlockNode(mutatedHeader(nodes[3], 0x1238, 0x00), nodes[3])
	nodes[5] = newBlockNode(mutatedHeader(nodes[4], 0xd322d912, 0x00), nodes[4])
	assertShortKeyCollision(nodes[4], nodes[5])
	nodes[6] = newBlockNode(mutatedHeader(nodes[4], 0xe10a4cee, 0x01), nodes[4])
	assertShortKeyCollision(nodes[4], nodes[6])
	collisions := nodes[4:7]

	// Create a node that has a full hash collision with one of the previous
	// nodes that also has a short key collision with another distinct node.
	//
	// Since the security properties of the hash function dictate that it is
	// computationally infeasible to find such a collision with a different
	// preimage, just create a different node with the same data to fake it.
	nodes[7] = newBlockNode(mutatedHeader(nodes[4], 0xe10a4cee, 0x01), nodes[4])
	assertShortKeyCollision(nodes[6], nodes[7])
	assertFullKeyCollision(nodes[6], nodes[7])
	doubleCollisions := nodes[6:8]

	// Similar to the previous, create nodes that have a full hash collision,
	// however, this time for nodes do not have short key collisions with any
	// other nodes aside from themselves.
	nodes[8] = newBlockNode(mutatedHeader(genesis, 0xabcd, 0x00), genesis)
	nodes[9] = newBlockNode(mutatedHeader(genesis, 0xabcd, 0x00), genesis)
	assertShortKeyCollision(nodes[8], nodes[9])
	assertFullKeyCollision(nodes[8], nodes[9])
	fullCollisions := nodes[8:10]

	// Ensure none of the nodes are returned when looked up before being added.
	for _, node := range nodes {
		assertLookupResult(node, nil)
	}

	// Add the nodes that do not have any collisions in the short or full key to
	// the block index and ensure they are looked up as expected.  Each one is
	// looked up as it's added and then all of them are looked up again once all
	// nodes are added for extra assurance.
	for _, node := range collisionFreeNodes {
		bc.index.addNode(node)
		assertLookupResult(node, node)
	}
	for _, node := range collisionFreeNodes {
		assertLookupResult(node, node)
	}

	// Ensure nodes that have a short key collision with a node that is in the
	// index but are not themselves in the index do not return the colliding
	// node.
	bc.index.addNode(collisions[0])
	assertLookupResult(collisions[0], collisions[0])
	assertLookupResult(collisions[1], nil)
	assertLookupResult(collisions[2], nil)

	// Add nodes with short key collisions and ensure looking up the nodes
	// returns the expected ones instead of the colliding ones.
	bc.index.addNode(collisions[1])
	assertLookupResult(collisions[0], collisions[0])
	assertLookupResult(collisions[1], collisions[1])
	assertLookupResult(collisions[2], nil)
	bc.index.addNode(collisions[2])
	assertLookupResult(collisions[0], collisions[0])
	assertLookupResult(collisions[1], collisions[1])
	assertLookupResult(collisions[2], collisions[2])

	// Ensure a node that has a full hash that collides with a node that also
	// has a short key collision with another node with a distinct hash (in
	// other words, the existing nodes have already been moved to the collisions
	// map) is replaced with the new one.
	assertLookupResult(doubleCollisions[0], doubleCollisions[0])
	assertLookupResult(doubleCollisions[1], doubleCollisions[0])
	bc.index.addNode(doubleCollisions[1])
	assertLookupResult(doubleCollisions[0], doubleCollisions[1])
	assertLookupResult(doubleCollisions[1], doubleCollisions[1])

	// Ensure a node that has a full hash that collides with a node that does
	// not have any other short key collisions (in other words, the existing
	// node has NOT already been moved to the collisions map) is replaced with
	// the new one.
	assertLookupResult(fullCollisions[0], nil)
	assertLookupResult(fullCollisions[1], nil)
	bc.index.addNode(fullCollisions[0])
	assertLookupResult(fullCollisions[0], fullCollisions[0])
	assertLookupResult(fullCollisions[1], fullCollisions[0])
	bc.index.addNode(fullCollisions[1])
	assertLookupResult(fullCollisions[0], fullCollisions[1])
	assertLookupResult(fullCollisions[1], fullCollisions[1])
}
