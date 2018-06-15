// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"fmt"
	"math/big"
	"testing"
)

// TestMustRegisterPanic ensures the mustRegister function panics when used to
// register an invalid network.
func TestMustRegisterPanic(t *testing.T) {
	t.Parallel()

	// Setup a defer to catch the expected panic to ensure it actually
	// paniced.
	defer func() {
		if err := recover(); err == nil {
			t.Error("mustRegister did not panic as expected")
		}
	}()

	// Intentionally try to register duplicate params to force a panic.
	mustRegister(&MainNetParams)
}

//TestDecredNetworkSettings checks Network-specific settings
func TestDecredNetworkSettings(t *testing.T) {

	checkPowLimitsAreConsistent(t, MainNetParams)
	checkPowLimitsAreConsistent(t, TestNet2Params)
	checkPowLimitsAreConsistent(t, SimNetParams)

	checkGenesisBlockRespectsNetworkPowLimit(t, MainNetParams)
	checkGenesisBlockRespectsNetworkPowLimit(t, TestNet2Params)
	checkGenesisBlockRespectsNetworkPowLimit(t, SimNetParams)

}

// checkPowLimitsAreConsistent ensures PowLimit and PowLimitBits are consistent with each other
// PowLimit:         mainPowLimit,// big int
// PowLimitBits:     0x1d00ffff,  //conceptually the same
//                                //value, but in an obscure form
func checkPowLimitsAreConsistent(t *testing.T, params Params) {

	powLimitBigInt := params.PowLimit
	powLimitCompact := params.PowLimitBits

	toBig := compactToBig(powLimitCompact)
	toCompact := bigToCompact(powLimitBigInt)

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
func checkGenesisBlockRespectsNetworkPowLimit(t *testing.T, params Params) {
	genesis := params.GenesisBlock
	bits := genesis.Header.Bits
	bitsAsBigInt := compactToBig(bits) //Header bits as big.Int

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
func compactToBig(compact uint32) *big.Int {
	mantissa := compact & 0x007fffff
	isNegative := compact&0x00800000 != 0
	exponent := uint(compact >> 24)

	var bn *big.Int
	if exponent <= 3 {
		mantissa >>= 8 * (3 - exponent)
		bn = big.NewInt(int64(mantissa))
	} else {
		bn = big.NewInt(int64(mantissa))
		bn.Lsh(bn, 8*(exponent-3))
	}

	if isNegative {
		bn = bn.Neg(bn)
	}

	return bn
}

func bigToCompact(n *big.Int) uint32 {
	if n.Sign() == 0 {
		return 0
	}

	var mantissa uint32
	exponent := uint(len(n.Bytes()))
	if exponent <= 3 {
		mantissa = uint32(n.Bits()[0])
		mantissa <<= 8 * (3 - exponent)
	} else {
		tn := new(big.Int).Set(n)
		mantissa = uint32(tn.Rsh(tn, 8*(exponent-3)).Bits()[0])
	}

	if mantissa&0x00800000 != 0 {
		mantissa >>= 8
		exponent++
	}

	compact := uint32(exponent<<24) | mantissa
	if n.Sign() < 0 {
		compact |= 0x00800000
	}
	return compact
}
