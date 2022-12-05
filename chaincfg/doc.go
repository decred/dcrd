// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package chaincfg defines chain configuration parameters.
//
// In addition to the main Decred network, which is intended for the transfer
// of monetary value, there also exists two currently active standard networks:
// regression test and testnet (version 0).  These networks are incompatible
// with each other (each sharing a different genesis block) and software should
// handle errors where input intended for one network is used on an application
// instance running on a different network.
//
// For main packages, a (typically global) var may be assigned the address of
// one of the standard Param vars for use as the application's "active" network.
// When a network parameter is needed, it may then be looked up through this
// variable (either directly, or hidden in a library call).
//
//	package main
//
//	import (
//		"flag"
//		"fmt"
//		"log"
//
//		"github.com/decred/dcrd/chaincfg/v3"
//		"github.com/decred/dcrd/txscript/v4/stdaddr"
//	)
//
//	func main() {
//		var testnet = flag.Bool("testnet", false, "operate on the test network")
//		flag.Parse()
//
//		// By default (without -testnet), use mainnet.
//		var chainParams = chaincfg.MainNetParams()
//
//		// Modify active network parameters if operating on testnet.
//		if *testnet {
//			chainParams = chaincfg.TestNet3Params()
//		}
//
//		// later...
//
//		// Create and print new payment address, specific to the active network.
//		pubKeyHash := make([]byte, 20)
//		addr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(pubKeyHash,
//			chainParams)
//		if err != nil {
//			log.Fatal(err)
//		}
//		fmt.Println(addr)
//	}
//
// If an application does not use one of the standard Decred networks, a new
// Params struct may be created which defines the parameters for the
// non-standard network.  As a general rule of thumb, all network parameters
// should be unique to the network, but parameter collisions can still occur.
package chaincfg
