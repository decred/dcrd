// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"math/big"
	"testing"

	"github.com/decred/dcrd/blockchainutil"
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

func TestCheckDifficulty(t *testing.T) {
	bigOne := big.NewInt(1)

	{ //Check proof-of-work limit parameter for the main network
		testBigIntValue := new(big.Int).Sub(new(big.Int).Lsh(bigOne, 256-4*8), bigOne)
		testCompactBitsValue := uint32(0x1d00ffff)
		checkPowerLimit(t, mainPowLimit, testCompactBitsValue, testBigIntValue)
	}
	{ //Check proof-of-work limit parameter for the test network
		testBigIntValue := new(big.Int).Sub(new(big.Int).Lsh(bigOne, 256-3*8), bigOne)
		testCompactBitsValue := uint32(0x1e00ffff)
		checkPowerLimit(t, testNetPowLimit, testCompactBitsValue, testBigIntValue)
	}
	{ //Check proof-of-work limit parameter for the sim network
		testBigIntValue := new(big.Int).Sub(new(big.Int).Lsh(bigOne, 256-1), bigOne)
		testCompactBitsValue := uint32(0x207fffff)
		checkPowerLimit(t, simNetPowLimit, testCompactBitsValue, testBigIntValue)
	}

}

func checkPowerLimit(t *testing.T, dif *blockchainutil.Difficulty, testCompactBitsValue uint32, testBigIntValue *big.Int) {
	dif.Print() //for code coverage and tests debug

	if dif.ToBigInt().Cmp(testBigIntValue) != 0 {
		t.Fatalf("mainPowLimit values mismatch: testBigIntValue=%v is not equal to dif.ToBigInt()=%v", testBigIntValue, mainPowLimit.ToBigInt())
	}

	if dif.ToCompact() != testCompactBitsValue {
		t.Fatalf("mainPowLimit values mismatch: testCompactBitsValue=%v is not equal to dif.ToCompact()=%v", testBigIntValue, mainPowLimit.ToBigInt())
	}

}
