// Copyright (c) 2019-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file is ignored during the regular tests due to the following build tag.
// +build rpctest

package rpctest

import (
	"context"
	"os"
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/rpcclient/v6"
)

// testCanPassSVH tests whether the wallet can maintain the chain going past SVH
// (stake validation height).
func testCanPassSVH(ctx context.Context, t *testing.T, vw *VotingWallet) {

	// Store the current (starting) height.
	_, startHeight, err := vw.hn.Node.GetBestBlock(ctx)
	if err != nil {
		t.Fatalf("unable to obtain best block: %v", err)
	}

	// Generate enough blocks to get us past SVH.
	targetHeight := vw.hn.ActiveNet.StakeValidationHeight * 2
	if targetHeight < startHeight {
		targetHeight = startHeight + 10
	}

	for h := startHeight + 1; h <= targetHeight; h++ {
		// Try and generate a block at this height.
		_, err := vw.GenerateBlocks(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}

		// Verify whether a block was actually generated (after SVH, this will
		// imply the wallet was successfully voting on blocks).
		_, actualHeight, err := vw.hn.Node.GetBestBlock(ctx)
		if err != nil {
			t.Fatalf("unable to obtain best block: %v", err)
		}
		if actualHeight != h {
			t.Fatalf("block was not mined at height %d (got %d as best height)",
				h, actualHeight)
		}
	}

	t.Logf("Generated up to block %d\n", targetHeight)
}

func TestMinimalVotingWallet(t *testing.T) {
	// Skip tests when running with -short
	if testing.Short() {
		t.Skip("Skipping minimal voting wallet in short mode")
	}

	var handlers *rpcclient.NotificationHandlers
	net := chaincfg.SimNetParams()

	logDir := "./dcrdlogs"
	extraArgs := []string{
		"--debuglevel=debug",
	}

	info, err := os.Stat(logDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("error stating log dir: %v", err)
	}
	if info != nil {
		if !info.IsDir() {
			t.Fatalf("logdir (%s) is not a dir", logDir)
		}
		err = os.RemoveAll(logDir)
		if err != nil {
			t.Fatalf("error removing logdir: %v", err)
		}
	}

	hn, err := New(net, handlers, extraArgs)
	if err != nil {
		t.Fatal(err)
	}

	err = hn.SetUp(true, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer hn.TearDown()

	type testCase struct {
		name string
		f    func(ctx context.Context, t *testing.T, vw *VotingWallet)
	}

	testCases := []testCase{
		{
			name: "can get past SVH",
			f:    testCanPassSVH,
		},
	}

	ctx := context.Background()
	for _, tc := range testCases {
		var vw *VotingWallet
		success := t.Run(tc.name, func(t1 *testing.T) {
			vw, err = NewVotingWallet(ctx, hn)
			if err != nil {
				t1.Fatalf("unable to create voting wallet for test: %v", err)
			}

			err = vw.Start()
			if err != nil {
				t1.Fatalf("unable to setup voting wallet: %v", err)
			}

			vw.SetErrorReporting(func(vwerr error) {
				t.Fatalf("voting wallet errored: %v", vwerr)
			})

			tc.f(ctx, t1, vw)
		})

		if vw != nil {
			vw.SetErrorReporting(nil)
			vw.Stop()
		}

		if !success {
			break
		}
	}

	err = hn.TearDown()
	if err != nil {
		t.Fatalf("errored while tearing down test harness: %v", err)
	}
}
