// Copyright (c) 2018-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/decred/base58"
	"github.com/decred/dcrd/blockchain/standalone"
	"github.com/decred/dcrd/chaincfg/v3"
)

// checkPowLimitsAreConsistent ensures PowLimit and PowLimitBits are consistent
// with each other
// PowLimit:         mainPowLimit,// big int
// PowLimitBits:     0x1d00ffff,  // conceptually the same
//                                // value, but in an obscure form
func checkPowLimitsAreConsistent(t *testing.T, params *chaincfg.Params) {
	powLimitBigInt := params.PowLimit
	powLimitCompact := params.PowLimitBits

	toBig := standalone.CompactToBig(powLimitCompact)
	toCompact := standalone.BigToCompact(powLimitBigInt)

	// Check params.PowLimitBits matches params.PowLimit converted
	// into the compact form
	if toCompact != powLimitCompact {
		t.Fatalf("PowLimit values mismatch:\n"+
			"params.PowLimit    :%064x\n"+
			"                   :%x\n"+
			"params.PowLimitBits:%064x\n"+
			"                   :%x\n"+
			"params.PowLimit is not consistent with the params.PowLimitBits",
			powLimitBigInt, toCompact, toBig, powLimitCompact)
	}
}

// checkGenesisBlockRespectsNetworkPowLimit ensures genesis.Header.Bits value
// is within the network PoW limit.
//
// Genesis header bits define starting difficulty of the network.
// Header bits of each block define target difficulty of the subsequent block.
//
// The first few solved blocks of the network will inherit the genesis block
// bits value before the difficulty readjustment takes place.
//
// Solved block shouldn't be rejected due to the PoW limit check.
//
// This test ensures these blocks will respect the network PoW limit.
func checkGenesisBlockRespectsNetworkPowLimit(t *testing.T, params *chaincfg.Params) {
	genesis := params.GenesisBlock
	bits := genesis.Header.Bits

	// Header bits as big.Int
	bitsAsBigInt := standalone.CompactToBig(bits)

	// network PoW limit
	powLimitBigInt := params.PowLimit

	if bitsAsBigInt.Cmp(powLimitBigInt) > 0 {
		t.Fatalf("Genesis block fails the consensus:\n"+
			"genesis.Header.Bits:%x\n"+
			"                   :%064x\n"+
			"params.PowLimit    :%064x\n"+
			"genesis.Header.Bits "+
			"should respect network PoW limit",
			bits, bitsAsBigInt, powLimitBigInt)
	}
}

// checkPrefix checks if targetString starts with the given prefix
func checkPrefix(t *testing.T, prefix string, targetString, networkName string) {
	if strings.Index(targetString, prefix) != 0 {
		t.Logf("Address prefix mismatch for <%s>: expected <%s> received <%s>",
			networkName, prefix, targetString)
		t.FailNow()
	}
}

// checkInterval creates two corner cases defining interval
// of all key values: [ xxxx000000000...0cccc , xxxx111111111...1cccc ],
// where xxxx - is the encoding magic, and cccc is a checksum.
// The interval is mapped to corresponding interval in base 58.
// Then prefixes are checked for mismatch.
func checkInterval(t *testing.T, desiredPrefix string, keySize int, networkName string, magic [2]byte) {
	// min and max possible keys
	// all zeroes
	minKey := bytes.Repeat([]byte{0x00}, keySize)
	// all ones
	maxKey := bytes.Repeat([]byte{0xff}, keySize)

	base58interval := [2]string{
		base58.CheckEncode(minKey, magic),
		base58.CheckEncode(maxKey, magic),
	}
	checkPrefix(t, desiredPrefix, base58interval[0], networkName)
	checkPrefix(t, desiredPrefix, base58interval[1], networkName)
}

// checkAddressPrefixesAreConsistent ensures address encoding magics and
// NetworkAddressPrefix are consistent with each other.
// This test will light red when a new network is started with incorrect values.
func checkAddressPrefixesAreConsistent(t *testing.T, privateKeyPrefix string, params *chaincfg.Params) {
	P := params.NetworkAddressPrefix

	// Desired prefixes
	Pk := P + "k"
	Ps := P + "s"
	Pe := P + "e"
	PS := P + "S"
	Pc := P + "c"
	pk := privateKeyPrefix

	checkInterval(t, Pk, 33, params.Name, params.PubKeyAddrID)
	checkInterval(t, Ps, 20, params.Name, params.PubKeyHashAddrID)
	checkInterval(t, Pe, 20, params.Name, params.PKHEdwardsAddrID)
	checkInterval(t, PS, 20, params.Name, params.PKHSchnorrAddrID)
	checkInterval(t, Pc, 20, params.Name, params.ScriptHashAddrID)
	checkInterval(t, pk, 33, params.Name, params.PrivateKeyID)
}

// TestDecredNetworkSettings checks Network-specific settings
func TestDecredNetworkSettings(t *testing.T) {
	mainNetParams := chaincfg.MainNetParams()
	testNet3Params := chaincfg.TestNet3Params()
	simNetParams := chaincfg.SimNetParams()
	regNetParams := chaincfg.RegNetParams()

	checkPowLimitsAreConsistent(t, mainNetParams)
	checkPowLimitsAreConsistent(t, testNet3Params)
	checkPowLimitsAreConsistent(t, simNetParams)
	checkPowLimitsAreConsistent(t, regNetParams)

	checkGenesisBlockRespectsNetworkPowLimit(t, mainNetParams)
	checkGenesisBlockRespectsNetworkPowLimit(t, testNet3Params)
	checkGenesisBlockRespectsNetworkPowLimit(t, simNetParams)
	checkGenesisBlockRespectsNetworkPowLimit(t, regNetParams)

	checkAddressPrefixesAreConsistent(t, "Pm", mainNetParams)
	checkAddressPrefixesAreConsistent(t, "Pt", testNet3Params)
	checkAddressPrefixesAreConsistent(t, "Ps", simNetParams)
	checkAddressPrefixesAreConsistent(t, "Pr", regNetParams)
}
