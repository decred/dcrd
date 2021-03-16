// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file is ignored during the regular tests due to the following build tag.
// +build rpctest

package rpctest

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v3"
	"github.com/decred/dcrd/wire"
)

const (
	numMatureOutputs = 25
)

func testSendOutputs(ctx context.Context, r *Harness, t *testing.T) {
	tracef(t, "testSendOutputs start")
	defer tracef(t, "testSendOutputs end")

	genSpend := func(amt dcrutil.Amount) *chainhash.Hash {
		// Grab a fresh address from the wallet.
		addr, err := r.NewAddress()
		if err != nil {
			t.Fatalf("unable to get new address: %v", err)
		}

		// Next, send amt to this address, spending from one of our
		// mature coinbase outputs.
		addrScriptVer, addrScript := addr.PaymentScript()
		output := newTxOut(int64(amt), addrScriptVer, addrScript)
		txid, err := r.SendOutputs([]*wire.TxOut{output}, 10)
		if err != nil {
			t.Fatalf("coinbase spend failed: %v", err)
		}
		return txid
	}

	assertTxMined := func(ctx context.Context, txid *chainhash.Hash, blockHash *chainhash.Hash) {
		block, err := r.Node.GetBlock(ctx, blockHash)
		if err != nil {
			t.Fatalf("unable to get block: %v", err)
		}

		numBlockTxns := len(block.Transactions)
		if numBlockTxns < 2 {
			t.Fatalf("crafted transaction wasn't mined, block should have "+
				"at least %v transactions instead has %v", 2, numBlockTxns)
		}

		minedTx := block.Transactions[1]
		txHash := minedTx.TxHash()
		if txHash != *txid {
			t.Fatalf("txid's don't match, %v vs %v", txHash, txid)
		}
	}

	// First, generate a small spend which will require only a single
	// input.
	txid := genSpend(dcrutil.Amount(5 * dcrutil.AtomsPerCoin))

	// Generate a single block, the transaction the wallet created should
	// be found in this block.
	if err := r.Node.RegenTemplate(ctx); err != nil {
		t.Fatalf("unable to regenerate block template: %v", err)
	}
	time.Sleep(time.Millisecond * 500)
	blockHashes, err := r.Node.Generate(ctx, 1)
	if err != nil {
		t.Fatalf("unable to generate single block: %v", err)
	}
	assertTxMined(ctx, txid, blockHashes[0])

	// Next, generate a spend much greater than the block reward. This
	// transaction should also have been mined properly.
	txid = genSpend(dcrutil.Amount(5000 * dcrutil.AtomsPerCoin))
	if err := r.Node.RegenTemplate(ctx); err != nil {
		t.Fatalf("unable to regenerate block template: %v", err)
	}
	time.Sleep(time.Millisecond * 500)
	blockHashes, err = r.Node.Generate(ctx, 1)
	if err != nil {
		t.Fatalf("unable to generate single block: %v", err)
	}
	assertTxMined(ctx, txid, blockHashes[0])

	// Generate another block to ensure the transaction is removed from the
	// mempool.
	if _, err := r.Node.Generate(ctx, 1); err != nil {
		t.Fatalf("unable to generate block: %v", err)
	}
}

func assertConnectedTo(ctx context.Context, t *testing.T, nodeA *Harness, nodeB *Harness) {
	tracef(t, "assertConnectedTo start")
	defer tracef(t, "assertConnectedTo end")

	nodeAPeers, err := nodeA.Node.GetPeerInfo(ctx)
	if err != nil {
		t.Fatalf("unable to get nodeA's peer info")
	}

	nodeAddr := nodeB.node.config.listen
	addrFound := false
	for _, peerInfo := range nodeAPeers {
		if peerInfo.Addr == nodeAddr {
			addrFound = true
			tracef(t, "found %v", nodeAddr)
			break
		}
	}

	if !addrFound {
		t.Fatal("nodeA not connected to nodeB")
	}
}

func testConnectNode(ctx context.Context, r *Harness, t *testing.T) {
	tracef(t, "testConnectNode start")
	defer tracef(t, "testConnectNode end")

	// Create a fresh test harness.
	harness, err := New(t, chaincfg.RegNetParams(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.SetUp(false, 0); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}
	defer func() {
		tracef(t, "testConnectNode: calling harness.TearDown")
		harness.TearDown()
	}()

	// Establish a p2p connection from our new local harness to the main
	// harness.
	if err := ConnectNode(harness, r); err != nil {
		t.Fatalf("unable to connect local to main harness: %v", err)
	}

	// The main harness should show up in our local harness' peer's list,
	// and vice verse.
	assertConnectedTo(ctx, t, harness, r)
}

func assertNotConnectedTo(ctx context.Context, t *testing.T, nodeA *Harness, nodeB *Harness) {
	tracef(t, "assertNotConnectedTo start")
	defer tracef(t, "assertNotConnectedTo end")

	nodeAPeers, err := nodeA.Node.GetPeerInfo(ctx)
	if err != nil {
		t.Fatalf("unable to get nodeA's peer info")
	}

	nodeAddr := nodeB.node.config.listen
	addrFound := false
	for _, peerInfo := range nodeAPeers {
		if peerInfo.Addr == nodeAddr {
			addrFound = true
			break
		}
	}

	if addrFound {
		t.Fatal("nodeA is connected to nodeB")
	}
}

func testDisconnectNode(ctx context.Context, r *Harness, t *testing.T) {
	tracef(t, "testDisconnectNode start")
	defer tracef(t, "testDisconnectNode end")

	// Create a fresh test harness.
	harness, err := New(t, chaincfg.RegNetParams(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.SetUp(false, 0); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}
	defer harness.TearDown()

	// Establish a p2p connection from our new local harness to the main
	// harness.
	if err := ConnectNode(harness, r); err != nil {
		t.Fatalf("unable to connect local to main harness: %v", err)
	}

	// Sanity check.
	assertConnectedTo(ctx, t, harness, r)

	// Disconnect the nodes.
	if err := RemoveNode(ctx, harness, r); err != nil {
		t.Fatalf("unable to disconnect local to main harness: %v", err)
	}

	assertNotConnectedTo(ctx, t, harness, r)

	// Re-connect the nodes. We'll perform the test in the reverse direction now
	// and assert that the nodes remain connected and that RemoveNode() fails.
	if err := ConnectNode(harness, r); err != nil {
		t.Fatalf("unable to connect local to main harness: %v", err)
	}

	// Sanity check.
	assertConnectedTo(ctx, t, harness, r)

	// Try to disconnect the nodes in the reverse direction. This should fail,
	// as the nodes are connected in the harness->r direction.
	if err := RemoveNode(ctx, r, harness); err == nil {
		t.Fatalf("removeNode on unconnected peers should return an error")
	}

	// Ensure the nodes remain connected after trying to disconnect them in the
	// reverse order.
	assertConnectedTo(ctx, t, harness, r)
}

func testNodesConnected(ctx context.Context, r *Harness, t *testing.T) {
	tracef(t, "testNodesConnected start")
	defer tracef(t, "testNodesConnected end")

	// Create a fresh test harness.
	harness, err := New(t, chaincfg.RegNetParams(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.SetUp(false, 0); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}
	defer harness.TearDown()

	// Establish a p2p connection from our new local harness to the main
	// harness.
	if err := ConnectNode(harness, r); err != nil {
		t.Fatalf("unable to connect local to main harness: %v", err)
	}

	// Sanity check.
	assertConnectedTo(ctx, t, harness, r)

	// Ensure nodes are still connected.
	assertConnectedTo(ctx, t, harness, r)

	testCases := []struct {
		name         string
		allowReverse bool
		expected     bool
		from         *Harness
		to           *Harness
	}{
		// The existing connection is h->r.
		{"!allowReverse, h->r", false, true, harness, r},
		{"allowReverse, h->r", true, true, harness, r},
		{"!allowReverse, r->h", false, false, r, harness},
		{"allowReverse, r->h", true, true, r, harness},
	}

	for _, tc := range testCases {
		actual, err := NodesConnected(ctx, tc.from, tc.to, tc.allowReverse)
		if err != nil {
			t.Fatalf("unable to determine node connection: %v", err)
		}
		if actual != tc.expected {
			t.Fatalf("test case %s: actual result (%v) differs from expected "+
				"(%v)", tc.name, actual, tc.expected)
		}
	}

	// Disconnect the nodes.
	if err := RemoveNode(ctx, harness, r); err != nil {
		t.Fatalf("unable to disconnect local to main harness: %v", err)
	}

	// Sanity check.
	assertNotConnectedTo(ctx, t, harness, r)

	// All test cases must return false now.
	for _, tc := range testCases {
		actual, err := NodesConnected(ctx, tc.from, tc.to, tc.allowReverse)
		if err != nil {
			t.Fatalf("unable to determine node connection: %v", err)
		}
		if actual {
			t.Fatalf("test case %s: nodes connected after commanded to "+
				"disconnect", tc.name)
		}
	}
}

func testTearDownAll(t *testing.T) {
	tracef(t, "testTearDownAll start")
	defer tracef(t, "testTearDownAll end")

	// Grab a local copy of the currently active harnesses before
	// attempting to tear them all down.
	initialActiveHarnesses := ActiveHarnesses()

	// Tear down all currently active harnesses.
	if err := TearDownAll(); err != nil {
		t.Fatalf("unable to teardown all harnesses: %v", err)
	}

	// The global testInstances map should now be fully purged with no
	// active test harnesses remaining.
	if len(ActiveHarnesses()) != 0 {
		t.Fatalf("test harnesses still active after TearDownAll")
	}

	for _, harness := range initialActiveHarnesses {
		// Ensure all test directories have been deleted.
		if _, err := os.Stat(harness.testNodeDir); err == nil {
			if !(debug || trace) {
				t.Errorf("created test datadir was not deleted.")
			}
		}
	}
}

func testActiveHarnesses(_ context.Context, r *Harness, t *testing.T) {
	tracef(t, "testActiveHarnesses start")
	defer tracef(t, "testActiveHarnesses end")

	numInitialHarnesses := len(ActiveHarnesses())

	// Create a single test harness.
	harness1, err := New(t, chaincfg.RegNetParams(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer harness1.TearDown()

	// With the harness created above, a single harness should be detected
	// as active.
	numActiveHarnesses := len(ActiveHarnesses())
	if !(numActiveHarnesses > numInitialHarnesses) {
		t.Fatalf("ActiveHarnesses not updated, should have an " +
			"additional test harness listed.")
	}
}

func testJoinMempools(ctx context.Context, r *Harness, t *testing.T) {
	tracef(t, "testJoinMempools start")
	defer tracef(t, "testJoinMempools end")

	// Assert main test harness has no transactions in its mempool.
	pooledHashes, err := r.Node.GetRawMempool(ctx, dcrdtypes.GRMAll)
	if err != nil {
		t.Fatalf("unable to get mempool for main test harness: %v", err)
	}
	if len(pooledHashes) != 0 {
		t.Fatal("main test harness mempool not empty")
	}

	// Create a local test harness with only the genesis block.  The nodes
	// will be synced below so the same transaction can be sent to both
	// nodes without it being an orphan.
	harness, err := New(t, chaincfg.RegNetParams(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.SetUp(false, 0); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}
	defer harness.TearDown()

	nodeSlice := []*Harness{r, harness}

	// Both mempools should be considered synced as they are empty.
	// Therefore, this should return instantly.
	if err := JoinNodes(nodeSlice, Mempools); err != nil {
		t.Fatalf("unable to join node on mempools: %v", err)
	}

	// Generate a coinbase spend to a new address within the main harness'
	// mempool.
	addr, err := r.NewAddress()
	if err != nil {
		t.Fatalf("unable to get new address: %v", err)
	}
	addrScriptVer, addrScript := addr.PaymentScript()
	if err != nil {
		t.Fatalf("unable to generate pkscript to addr: %v", err)
	}
	output := newTxOut(5e8, addrScriptVer, addrScript)
	testTx, err := r.CreateTransaction([]*wire.TxOut{output}, 10)
	if err != nil {
		t.Fatalf("coinbase spend failed: %v", err)
	}
	if _, err := r.Node.SendRawTransaction(ctx, testTx, true); err != nil {
		t.Fatalf("send transaction failed: %v", err)
	}

	// Wait until the transaction shows up to ensure the two mempools are
	// not the same.
	harnessSynced := make(chan error)
	go func() {
		for {
			poolHashes, err := r.Node.GetRawMempool(ctx, dcrdtypes.GRMAll)
			if err != nil {
				err = fmt.Errorf("failed to retrieve harness mempool: %w", err)
				harnessSynced <- err
				return
			}
			if len(poolHashes) > 0 {
				break
			}
			time.Sleep(time.Millisecond * 100)
		}
		harnessSynced <- nil
	}()

	select {
	case err := <-harnessSynced:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Minute):
		t.Fatal("harness node never received transaction")
	}

	// This select case should fall through to the default as the goroutine
	// should be blocked on the JoinNodes call.
	poolsSynced := make(chan error)
	go func() {
		if err := JoinNodes(nodeSlice, Mempools); err != nil {
			err = fmt.Errorf("unable to join node on mempools: %w", err)
			poolsSynced <- err
			return
		}
		poolsSynced <- nil
	}()
	select {
	case err := <-poolsSynced:
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal("mempools detected as synced yet harness has a new tx")
	default:
	}

	// Establish an outbound connection from the local harness to the main
	// harness and wait for the chains to be synced.
	if err := ConnectNode(harness, r); err != nil {
		t.Fatalf("unable to connect harnesses: %v", err)
	}
	if err := JoinNodes(nodeSlice, Blocks); err != nil {
		t.Fatalf("unable to join node on blocks: %v", err)
	}

	// Send the transaction to the local harness which will result in synced
	// mempools.
	if _, err := harness.Node.SendRawTransaction(ctx, testTx, true); err != nil {
		t.Fatalf("send transaction failed: %v", err)
	}

	// Select once again with a special timeout case after 1 minute. The
	// goroutine above should now be blocked on sending into the unbuffered
	// channel. The send should immediately succeed. In order to avoid the
	// test hanging indefinitely, a 1 minute timeout is in place.
	select {
	case err := <-poolsSynced:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Minute):
		t.Fatal("mempools never detected as synced")
	}
}

func testJoinBlocks(_ context.Context, r *Harness, t *testing.T) {
	tracef(t, "testJoinBlocks start")
	defer tracef(t, "testJoinBlocks end")

	// Create a second harness with only the genesis block so it is behind
	// the main harness.
	harness, err := New(t, chaincfg.RegNetParams(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.SetUp(false, 0); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}
	defer harness.TearDown()

	nodeSlice := []*Harness{r, harness}
	blocksSynced := make(chan error)
	go func() {
		if err := JoinNodes(nodeSlice, Blocks); err != nil {
			blocksSynced <- fmt.Errorf("unable to join node on blocks: %w", err)
			return
		}
		blocksSynced <- nil
	}()

	// This select case should fall through to the default as the goroutine
	// should be blocked on the JoinNodes calls.
	select {
	case err := <-blocksSynced:
		if err != nil {
			t.Fatal(err)
		}
		t.Fatalf("blocks detected as synced yet local harness is behind")
	default:
	}

	// Connect the local harness to the main harness which will sync the
	// chains.
	if err := ConnectNode(harness, r); err != nil {
		t.Fatalf("unable to connect harnesses: %v", err)
	}

	// Select once again with a special timeout case after 1 minute. The
	// goroutine above should now be blocked on sending into the unbuffered
	// channel. The send should immediately succeed. In order to avoid the
	// test hanging indefinitely, a 1 minute timeout is in place.
	select {
	case err := <-blocksSynced:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Minute):
		t.Fatalf("blocks never detected as synced")
	}
}

func testMemWalletReorg(_ context.Context, r *Harness, t *testing.T) {
	tracef(t, "testMemWalletReorg start")
	defer tracef(t, "testMemWalletReorg end")

	// Create a fresh harness, we'll be using the main harness to force a
	// re-org on this local harness.
	harness, err := New(t, chaincfg.RegNetParams(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.SetUp(true, 5); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}
	defer harness.TearDown()

	// Ensure the internal wallet has the expected balance.
	expectedBalance := dcrutil.Amount(5 * 300 * dcrutil.AtomsPerCoin)
	walletBalance := harness.ConfirmedBalance()
	if expectedBalance != walletBalance {
		t.Fatalf("wallet balance incorrect: expected %v, got %v",
			expectedBalance, walletBalance)
	}

	// Now connect this local harness to the main harness then wait for
	// their chains to synchronize.
	if err := ConnectNode(harness, r); err != nil {
		t.Fatalf("unable to connect harnesses: %v", err)
	}
	nodeSlice := []*Harness{r, harness}
	if err := JoinNodes(nodeSlice, Blocks); err != nil {
		t.Fatalf("unable to join node on blocks: %v", err)
	}

	// The original wallet should now have a balance of 0 Coin as its entire
	// chain should have been decimated in favor of the main harness'
	// chain.
	expectedBalance = dcrutil.Amount(0)
	walletBalance = harness.ConfirmedBalance()
	if expectedBalance != walletBalance {
		t.Fatalf("wallet balance incorrect: expected %v, got %v",
			expectedBalance, walletBalance)
	}
}

func testMemWalletLockedOutputs(_ context.Context, r *Harness, t *testing.T) {
	tracef(t, "testMemWalletLockedOutputs start")
	defer tracef(t, "testMemWalletLockedOutputs end")

	// Obtain the initial balance of the wallet at this point.
	startingBalance := r.ConfirmedBalance()

	// First, create a signed transaction spending some outputs.
	addr, err := r.NewAddress()
	if err != nil {
		t.Fatalf("unable to generate new address: %v", err)
	}
	pkScriptVer, pkScript := addr.PaymentScript()
	outputAmt := dcrutil.Amount(50 * dcrutil.AtomsPerCoin)
	output := newTxOut(int64(outputAmt), pkScriptVer, pkScript)
	tx, err := r.CreateTransaction([]*wire.TxOut{output}, 10)
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// The current wallet balance should now be at least 50 Coin less
	// (accounting for fees) than the period balance
	currentBalance := r.ConfirmedBalance()
	if !(currentBalance <= startingBalance-outputAmt) {
		t.Fatalf("spent outputs not locked: previous balance %v, "+
			"current balance %v", startingBalance, currentBalance)
	}

	// Now unlocked all the spent inputs within the unbroadcast signed
	// transaction. The current balance should now be exactly that of the
	// starting balance.
	r.UnlockOutputs(tx.TxIn)
	currentBalance = r.ConfirmedBalance()
	if currentBalance != startingBalance {
		t.Fatalf("current and starting balance should now match: "+
			"expected %v, got %v", startingBalance, currentBalance)
	}
}

func TestHarness(t *testing.T) {
	var err error
	mainHarness, err := New(t, chaincfg.RegNetParams(), nil, nil)
	if err != nil {
		t.Fatalf("unable to create main harness: %v", err)
	}

	// Initialize the main mining node with a chain of length 42, providing
	// 25 mature coinbases to allow spending from for testing purposes.
	if err = mainHarness.SetUp(true, numMatureOutputs); err != nil {
		// Even though the harness was not fully setup, it still needs
		// to be torn down to ensure all resources such as temp
		// directories are cleaned up.  The error is intentionally
		// ignored since this is already an error path and nothing else
		// could be done about it anyways.
		_ = mainHarness.TearDown()
		t.Fatalf("unable to setup test chain: %v", err)
	}

	// Cleanup when we exit.
	defer func() {
		// Clean up any active harnesses that are still currently
		// running.
		if len(ActiveHarnesses()) > 0 {
			if err := TearDownAll(); err != nil {
				t.Fatalf("unable to tear down chain: %v", err)
			}
		}
	}()

	// We should have the expected amount of mature unspent outputs.
	expectedBalance := dcrutil.Amount(numMatureOutputs * 300 * dcrutil.AtomsPerCoin)
	harnessBalance := mainHarness.ConfirmedBalance()
	if harnessBalance != expectedBalance {
		t.Fatalf("expected wallet balance of %v instead have %v",
			expectedBalance, harnessBalance)
	}

	// Current tip should be at a height of numMatureOutputs plus the
	// required number of blocks for coinbase maturity plus an additional
	// block for the premine block.
	ctx := context.Background()
	nodeInfo, err := mainHarness.Node.GetInfo(ctx)
	if err != nil {
		t.Fatalf("unable to execute getinfo on node: %v", err)
	}
	coinbaseMaturity := uint32(mainHarness.ActiveNet.CoinbaseMaturity)
	expectedChainHeight := numMatureOutputs + coinbaseMaturity + 1
	if uint32(nodeInfo.Blocks) != expectedChainHeight {
		t.Errorf("Chain height is %v, should be %v",
			nodeInfo.Blocks, expectedChainHeight)
	}

	// Skip tests when running with -short
	if !testing.Short() {
		tests := []struct {
			name string
			f    func(context.Context, *Harness, *testing.T)
		}{
			{
				f:    testSendOutputs,
				name: "testSendOutputs",
			},
			{
				f:    testConnectNode,
				name: "testConnectNode",
			},
			{
				f:    testDisconnectNode,
				name: "testDisconnectNode",
			},
			{
				f:    testNodesConnected,
				name: "testNodesConnected",
			},
			{
				f:    testActiveHarnesses,
				name: "testActiveHarnesses",
			},
			{
				f:    testJoinBlocks,
				name: "testJoinBlocks",
			},
			{
				f:    testJoinMempools, // Depends on results of testJoinBlocks
				name: "testJoinMempools",
			},
			{
				f:    testMemWalletReorg,
				name: "testMemWalletReorg",
			},
			{
				f:    testMemWalletLockedOutputs,
				name: "testMemWalletLockedOutputs",
			},
		}

		for _, testCase := range tests {
			t.Logf("=== Running test: %v ===", testCase.name)

			c := make(chan struct{})
			go func() {
				testCase.f(ctx, mainHarness, t)
				c <- struct{}{}
			}()

			// Go wait for 10 seconds
			select {
			case <-c:
			case <-time.After(10 * time.Second):
				t.Logf("Test timeout, aborting running nodes")
				PanicAll(t)
				os.Exit(1)
			}
		}
	}

	testTearDownAll(t)
}
