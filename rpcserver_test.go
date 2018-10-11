// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file is ignored during the regular tests due to the following build tag.
// +build rpctest

package main

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"runtime/debug"
	"testing"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/rpctest"
)

func testGetBestBlock(r *rpctest.Harness, t *testing.T) {
	_, prevbestHeight, err := r.Node.GetBestBlock()
	if err != nil {
		t.Fatalf("Call to `getbestblock` failed: %v", err)
	}

	// Create a new block connecting to the current tip.
	generatedBlockHashes, err := r.Node.Generate(1)
	if err != nil {
		t.Fatalf("Unable to generate block: %v", err)
	}

	bestHash, bestHeight, err := r.Node.GetBestBlock()
	if err != nil {
		t.Fatalf("Call to `getbestblock` failed: %v", err)
	}

	// Hash should be the same as the newly submitted block.
	if !bytes.Equal(bestHash[:], generatedBlockHashes[0][:]) {
		t.Fatalf("Block hashes do not match. Returned hash %v, wanted "+
			"hash %v", bestHash, generatedBlockHashes[0][:])
	}

	// Block height should now reflect newest height.
	if bestHeight != prevbestHeight+1 {
		t.Fatalf("Block heights do not match. Got %v, wanted %v",
			bestHeight, prevbestHeight+1)
	}
}

func testGetBlockCount(r *rpctest.Harness, t *testing.T) {
	// Save the current count.
	currentCount, err := r.Node.GetBlockCount()
	if err != nil {
		t.Fatalf("Unable to get block count: %v", err)
	}

	if _, err := r.Node.Generate(1); err != nil {
		t.Fatalf("Unable to generate block: %v", err)
	}

	// Count should have increased by one.
	newCount, err := r.Node.GetBlockCount()
	if err != nil {
		t.Fatalf("Unable to get block count: %v", err)
	}
	if newCount != currentCount+1 {
		t.Fatalf("Block count incorrect. Got %v should be %v",
			newCount, currentCount+1)
	}
}

func testGetBlockHash(r *rpctest.Harness, t *testing.T) {
	// Create a new block connecting to the current tip.
	generatedBlockHashes, err := r.Node.Generate(1)
	if err != nil {
		t.Fatalf("Unable to generate block: %v", err)
	}

	info, err := r.Node.GetInfo()
	if err != nil {
		t.Fatalf("call to getinfo cailed: %v", err)
	}

	blockHash, err := r.Node.GetBlockHash(int64(info.Blocks))
	if err != nil {
		t.Fatalf("Call to `getblockhash` failed: %v", err)
	}

	// Block hashes should match newly created block.
	if !bytes.Equal(generatedBlockHashes[0][:], blockHash[:]) {
		t.Fatalf("Block hashes do not match. Returned hash %v, wanted "+
			"hash %v", blockHash, generatedBlockHashes[0][:])
	}
}

var rpcTestCases = []rpctest.HarnessTestCase{
	testGetBestBlock,
	testGetBlockCount,
	testGetBlockHash,
}

var primaryHarness *rpctest.Harness

func TestMain(m *testing.M) {
	var err error

	// Parse the -test.* flags before removing them from the command line
	// arguments list, which we do to allow go-flags to succeed.
	// See config_test.go for more info
	flag.Parse()
	os.Args = os.Args[:1]

	// In order to properly test scenarios on as if we were on mainnet,
	// ensure that non-standard transactions aren't accepted into the
	// mempool or relayed.
	args := []string{"--rejectnonstd"}
	primaryHarness, err = rpctest.New(&chaincfg.RegNetParams, nil, args)
	if err != nil {
		fmt.Println("unable to create primary harness: ", err)
		os.Exit(1)
	}

	// Initialize the primary mining node with a chain of length 125,
	// providing 25 mature coinbases to allow spending from for testing
	// purposes.
	if err := primaryHarness.SetUp(true, 25); err != nil {
		fmt.Println("unable to setup test chain: ", err)

		// Even though the harness was not fully setup, it still needs
		// to be torn down to ensure all resources such as temp
		// directories are cleaned up.  The error is intentionally
		// ignored since this is already an error path and nothing else
		// could be done about it anyways.
		_ = primaryHarness.TearDown()
		os.Exit(1)
	}

	exitCode := m.Run()

	// Clean up any active harnesses that are still currently running.This
	// includes removing all temporary directories, and shutting down any
	// created processes.
	if err := rpctest.TearDownAll(); err != nil {
		fmt.Println("unable to tear down all harnesses: ", err)
		os.Exit(1)
	}

	os.Exit(exitCode)
}

func TestRpcServer(t *testing.T) {
	var currentTestNum int
	defer func() {
		// If one of the integration tests caused a panic within the main
		// goroutine, then tear down all the harnesses in order to avoid
		// any leaked dcrd processes.
		if r := recover(); r != nil {
			fmt.Println("recovering from test panic: ", r)
			if err := rpctest.TearDownAll(); err != nil {
				fmt.Println("unable to tear down all harnesses: ", err)
			}
			t.Fatalf("test #%v panicked: %s", currentTestNum, debug.Stack())
		}
	}()

	for _, testCase := range rpcTestCases {
		testCase(primaryHarness, t)

		currentTestNum++
	}
}

func TestCertCreationWithHosts(t *testing.T) {
	certfile, err := ioutil.TempFile("", "certfile")
	if err != nil {
		t.Fatalf("Unable to create temp certfile: %s", err)
	}
	keyfile, err := ioutil.TempFile("", "keyfile")
	if err != nil {
		t.Fatalf("Unable to create temp keyfile: %s", err)
	}
	hostnames := []string{"hostname1", "hostname2"}
	defer os.Remove(keyfile.Name())
	defer os.Remove(certfile.Name())
	err = genCertPair(certfile.Name(), keyfile.Name(), hostnames)
	if err != nil {
		t.Fatalf("certifcate was not created correctly: %s", err)
	}
	certBytes, err := ioutil.ReadFile(certfile.Name())
	if err != nil {
		t.Fatalf("Unable to read the certfile: %s", err)
	}
	pemCert, _ := pem.Decode(certBytes)
	x509Cert, err := x509.ParseCertificate(pemCert.Bytes)
	if err != nil {
		t.Fatalf("Unable to parse the certificate: %s", err)
	}
	// Ensure the specified extra hosts are present.
	for _, host := range hostnames {
		err := x509Cert.VerifyHostname(host)
		if err != nil {
			t.Fatalf("failed to verify extra host '%s'", host)
		}
	}
}

func TestCertCreationWithOutHosts(t *testing.T) {
	certfile, err := ioutil.TempFile("", "certfile")
	if err != nil {
		t.Fatalf("Unable to create temp certfile: %s", err)
	}
	keyfile, err := ioutil.TempFile("", "keyfile")
	if err != nil {
		t.Fatalf("Unable to create temp keyfile: %s", err)
	}
	defer os.Remove(keyfile.Name())
	defer os.Remove(certfile.Name())
	err = genCertPair(certfile.Name(), keyfile.Name(), []string{})
	if err != nil {
		t.Fatalf("certifcate was not created correctly: %s", err)
	}
}
