// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017-2020 The Decred developers

// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file is ignored during the regular tests due to the following build tag.
//go:build rpctest
// +build rpctest

package rpcserver

import (
	"bytes"
	"context"
	"fmt"
	"runtime/debug"
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/rpctest"
)

func testGetBestBlock(ctx context.Context, r *rpctest.Harness, t *testing.T) {
	_, prevbestHeight, err := r.Node.GetBestBlock(ctx)
	if err != nil {
		t.Fatalf("Call to `getbestblock` failed: %v", err)
	}

	// Create a new block connecting to the current tip.
	generatedBlockHashes, err := r.Node.Generate(ctx, 1)
	if err != nil {
		t.Fatalf("Unable to generate block: %v", err)
	}

	bestHash, bestHeight, err := r.Node.GetBestBlock(ctx)
	if err != nil {
		t.Fatalf("Call to `getbestblock` failed: %v", err)
	}

	// Hash should be the same as the newly submitted block.
	if !bytes.Equal(bestHash[:], generatedBlockHashes[0][:]) {
		t.Fatalf("Block hashes do not match. Returned hash %v, wanted "+
			"hash %v", bestHash, generatedBlockHashes[0])
	}

	// Block height should now reflect newest height.
	if bestHeight != prevbestHeight+1 {
		t.Fatalf("Block heights do not match. Got %v, wanted %v",
			bestHeight, prevbestHeight+1)
	}
}

func testGetBlockCount(ctx context.Context, r *rpctest.Harness, t *testing.T) {
	// Save the current count.
	currentCount, err := r.Node.GetBlockCount(ctx)
	if err != nil {
		t.Fatalf("Unable to get block count: %v", err)
	}

	if _, err := r.Node.Generate(ctx, 1); err != nil {
		t.Fatalf("Unable to generate block: %v", err)
	}

	// Count should have increased by one.
	newCount, err := r.Node.GetBlockCount(ctx)
	if err != nil {
		t.Fatalf("Unable to get block count: %v", err)
	}
	if newCount != currentCount+1 {
		t.Fatalf("Block count incorrect. Got %v should be %v",
			newCount, currentCount+1)
	}
}

func testGetBlockHash(ctx context.Context, r *rpctest.Harness, t *testing.T) {
	// Create a new block connecting to the current tip.
	generatedBlockHashes, err := r.Node.Generate(ctx, 1)
	if err != nil {
		t.Fatalf("Unable to generate block: %v", err)
	}

	info, err := r.Node.GetInfo(ctx)
	if err != nil {
		t.Fatalf("call to getinfo failed: %v", err)
	}

	blockHash, err := r.Node.GetBlockHash(ctx, info.Blocks)
	if err != nil {
		t.Fatalf("Call to `getblockhash` failed: %v", err)
	}

	// Block hashes should match newly created block.
	if !bytes.Equal(generatedBlockHashes[0][:], blockHash[:]) {
		t.Fatalf("Block hashes do not match. Returned hash %v, wanted "+
			"hash %v", blockHash, generatedBlockHashes[0])
	}
}

func TestRpcServer(t *testing.T) {
	// In order to properly test scenarios on as if we were on mainnet,
	// ensure that non-standard transactions aren't accepted into the
	// mempool or relayed.
	args := []string{"--rejectnonstd"}
	harness, err := rpctest.New(t, chaincfg.RegNetParams(), nil, args)
	if err != nil {
		t.Fatalf("unable to create primary harness: %v", err)
	}

	// Initialize the primary mining node with a chain of length 125,
	// providing 25 mature coinbases to allow spending from for testing
	// purposes.
	if err := harness.SetUp(true, 25); err != nil {
		// Even though the harness was not fully setup, it still needs
		// to be torn down to ensure all resources such as temp
		// directories are cleaned up.  The error is intentionally
		// ignored since this is already an error path and nothing else
		// could be done about it anyways.
		_ = harness.TearDown()
		t.Fatalf("unable to setup test chain: %v", err)
	}

	defer func() {
		// Clean up any active harnesses that are still currently
		// running.This includes removing all temporary directories,
		// and shutting down any created processes.
		if err := rpctest.TearDownAll(); err != nil {
			t.Fatalf("unable to tear down all harnesses: %v", err)
		}
	}()

	var currentTestNum int
	defer func() {
		// If one of the integration tests caused a panic within the
		// main goroutine, then tear down all the harnesses in order to
		// avoid any leaked dcrd processes.
		if r := recover(); r != nil {
			fmt.Println("recovering from test panic: ", r)
			if err := rpctest.TearDownAll(); err != nil {
				fmt.Println("unable to tear down all harnesses: ", err)
			}
			t.Fatalf("test #%v panicked: %s", currentTestNum, debug.Stack())
		}
	}()

	// Test cases.
	tests := []struct {
		name string
		f    func(context.Context, *rpctest.Harness, *testing.T)
	}{
		{
			f:    testGetBestBlock,
			name: "testGetBestBlock",
		},
		{
			f:    testGetBlockCount,
			name: "testGetBlockCount",
		},
		{
			f:    testGetBlockHash,
			name: "testGetBlockHash",
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		test.f(ctx, harness, t)
		t.Logf("=== Running test: %v ===", test.name)

		currentTestNum++
	}
}
