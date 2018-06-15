// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"testing"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/chaincfg"
)

//TestDecredNetworkSettings checks Network-specific settings
func TestDecredNetworkSettings(t *testing.T) {

	checkPowLimitsAreConsistent(t, chaincfg.MainNetParams)
	checkPowLimitsAreConsistent(t, chaincfg.TestNet2Params)
	checkPowLimitsAreConsistent(t, chaincfg.SimNetParams)

	checkGenesisBlockRespectsNetworkPowLimit(t, chaincfg.MainNetParams)
	checkGenesisBlockRespectsNetworkPowLimit(t, chaincfg.TestNet2Params)
	checkGenesisBlockRespectsNetworkPowLimit(t, chaincfg.SimNetParams)

}

// checkPowLimitsAreConsistent ensures PowLimit and PowLimitBits are consistent with each other
// PowLimit:         mainPowLimit,// big int
// PowLimitBits:     0x1d00ffff,  //conceptually the same
//                                //value, but in an obscure form
func checkPowLimitsAreConsistent(t *testing.T, params chaincfg.Params) {

	powLimitBigInt := params.PowLimit
	powLimitCompact := params.PowLimitBits

	toBig := blockchain.CompactToBig(powLimitCompact)
	toCompact := blockchain.BigToCompact(powLimitBigInt)

	//Check params.PowLimitBits matches params.PowLimit converted into the compact form
	if toCompact != powLimitCompact {
		//Print debug info
		fmt.Println("params.PowLimit    :", fmt.Sprintf("%064x", powLimitBigInt))
		fmt.Println("                   :", fmt.Sprintf("%x", toCompact)) // PoW limit converted to compact
		fmt.Println("params.PowLimitBits:", fmt.Sprintf("%064x", toBig))  //Bits converted to big.Int
		fmt.Println("                   :", fmt.Sprintf("%x", powLimitCompact))
		fmt.Println()

		t.Fatalf("PowLimit values mismatch: params.PowLimit=%v is not consistent with the params.PowLimitBits=%v", fmt.Sprintf("%064x", powLimitBigInt), fmt.Sprintf("%x", powLimitCompact))
	}

}

// checkGenesisBlockRespectsNetworkPowLimit ensures genesis.Header.Bits value respects the consensus rules
func checkGenesisBlockRespectsNetworkPowLimit(t *testing.T, params chaincfg.Params) {
	genesis := params.GenesisBlock
	bits := genesis.Header.Bits
	bitsAsBigInt := blockchain.CompactToBig(bits) //Header bits as big.Int

	powLimitBigInt := params.PowLimit //network PoW limit

	// The block is valid when the Header.Bits value respects network PoW limit
	if bitsAsBigInt.Cmp(powLimitBigInt) > 0 {
		//Print debug info
		fmt.Println("genesis.Header.Bits:", fmt.Sprintf("%x", bits))
		fmt.Println("                   :", fmt.Sprintf("%064x", bitsAsBigInt)) //converted to big.In
		fmt.Println("params.PowLimit    :", fmt.Sprintf("%064x", powLimitBigInt))
		fmt.Println()

		t.Fatalf("Genesis block fails the consensus: genesis.Header.Bits=%v should respect network PoW limit: %v", fmt.Sprintf("%x", bits), fmt.Sprintf("%064x", powLimitBigInt))
	}

}
