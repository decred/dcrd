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
	"net/http"
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

func TestCheckAuthUserPass(t *testing.T) {
	s, err := New(&Config{
		RPCUser:      "user",
		RPCPass:      "pass",
		RPCLimitUser: "limit",
		RPCLimitPass: "limit",
	})
	if err != nil {
		t.Fatalf("unable to create RPC server: %v", err)
	}
	tests := []struct {
		name       string
		user       string
		pass       string
		wantAuthed bool
		wantAdmin  bool
	}{
		{
			name:       "correct admin",
			user:       "user",
			pass:       "pass",
			wantAuthed: true,
			wantAdmin:  true,
		},
		{
			name:       "correct limited user",
			user:       "limit",
			pass:       "limit",
			wantAuthed: true,
			wantAdmin:  false,
		},
		{
			name:       "invalid admin",
			user:       "user",
			pass:       "p",
			wantAuthed: false,
			wantAdmin:  false,
		},
		{
			name:       "invalid limited user",
			user:       "limit",
			pass:       "",
			wantAuthed: false,
			wantAdmin:  false,
		},
		{
			name:       "invalid empty user",
			user:       "",
			pass:       "",
			wantAuthed: false,
			wantAdmin:  false,
		},
	}
	for _, test := range tests {
		authed, isAdmin := s.checkAuthUserPass(test.user, test.pass, "addr")
		if authed != test.wantAuthed {
			t.Errorf("%q: unexpected authed -- got %v, want %v", test.name, authed,
				test.wantAuthed)
		}
		if isAdmin != test.wantAdmin {
			t.Errorf("%q: unexpected isAdmin -- got %v, want %v", test.name, isAdmin,
				test.wantAdmin)
		}
	}
}

func TestCheckAuth(t *testing.T) {
	{
		s, err := New(&Config{})
		if err != nil {
			t.Fatalf("unable to create RPC server: %v", err)
		}
		for i := 0; i <= 1; i++ {
			authed, isAdmin, err := s.checkAuth(&http.Request{}, i == 0)
			if !authed {
				t.Errorf(" unexpected authed -- got %v, want %v", authed, true)
			}
			if !isAdmin {
				t.Errorf("unexpected isAdmin -- got %v, want %v", isAdmin, true)
			}
			if err != nil {
				t.Errorf("unexpected err -- got %v, want %v", err, nil)
			}
		}
	}
	{
		s, err := New(&Config{
			RPCUser:      "user",
			RPCPass:      "pass",
			RPCLimitUser: "limit",
			RPCLimitPass: "limit",
		})
		if err != nil {
			t.Fatalf("unable to create RPC server: %v", err)
		}
		for i := 0; i <= 1; i++ {
			authed, isAdmin, err := s.checkAuth(&http.Request{}, i == 0)
			if authed {
				t.Errorf(" unexpected authed -- got %v, want %v", authed, false)
			}
			if isAdmin {
				t.Errorf("unexpected isAdmin -- got %v, want %v", isAdmin, false)
			}
			if i == 0 && err == nil {
				t.Errorf("unexpected err -- got %v, want auth failure", err)
			} else if i != 0 && err != nil {
				t.Errorf("unexpected err -- got %v, want <nil>", err)
			}
		}
		for i := 0; i <= 1; i++ {
			r := &http.Request{Header: make(map[string][]string, 1)}
			r.Header["Authorization"] = []string{"Basic Nothing"}
			authed, isAdmin, err := s.checkAuth(r, i == 0)
			if authed {
				t.Errorf(" unexpected authed -- got %v, want %v", authed, false)
			}
			if isAdmin {
				t.Errorf("unexpected isAdmin -- got %v, want %v", isAdmin, false)
			}
			if err == nil {
				t.Errorf("unexpected err -- got %v, want auth failure", err)
			}
		}
	}
}
