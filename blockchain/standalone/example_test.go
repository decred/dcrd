// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package standalone_test

import (
	"fmt"
	"math/big"

	"github.com/decred/dcrd/blockchain/standalone"
	"github.com/decred/dcrd/chaincfg/chainhash"
)

// This example demonstrates how to convert the compact "bits" in a block header
// which represent the target difficulty to a big integer and display it using
// the typical hex notation.
func ExampleCompactToBig() {
	// Convert the bits from block 1 in the main chain.
	bits := uint32(453115903)
	targetDifficulty := standalone.CompactToBig(bits)

	// Display it in hex.
	fmt.Printf("%064x\n", targetDifficulty.Bytes())

	// Output:
	// 000000000001ffff000000000000000000000000000000000000000000000000
}

// This example demonstrates how to convert a target difficulty into the compact
// "bits" in a block header which represent that target difficulty.
func ExampleBigToCompact() {
	// Convert the target difficulty from block 1 in the main chain to compact
	// form.
	t := "000000000001ffff000000000000000000000000000000000000000000000000"
	targetDifficulty, success := new(big.Int).SetString(t, 16)
	if !success {
		fmt.Println("invalid target difficulty")
		return
	}
	bits := standalone.BigToCompact(targetDifficulty)

	fmt.Println(bits)

	// Output:
	// 453115903
}

// This example demonstrates checking the proof of work of a block hash against
// a target difficulty.
func ExampleCheckProofOfWork() {
	// This is the pow limit for mainnet and would ordinarily come from chaincfg
	// params, however, it is hard coded here for the purposes of the example.
	l := "00000000ffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	powLimit, success := new(big.Int).SetString(l, 16)
	if !success {
		fmt.Println("invalid pow limit")
		return
	}

	// Check the proof of work for block 1 in the main chain.
	h := "000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9"
	hash, err := chainhash.NewHashFromStr(h)
	if err != nil {
		fmt.Printf("failed to parse hash: %v\n", err)
		return
	}
	bits := uint32(453115903)

	if err := standalone.CheckProofOfWork(hash, bits, powLimit); err != nil {
		fmt.Printf("proof of work check failed: %v\n", err)
		return
	}

	// Output:
	//
}

// This example demonstrates calculating a merkle root from a slice of leaf
// hashes.
func ExampleCalcMerkleRoot() {
	// Create a slice of the leaf hashes.
	leaves := make([]chainhash.Hash, 3)
	for i := range leaves {
		// The hash would ordinarily be calculated from the TxHashFull function
		// on a transaction, however, it's left as a zero hash for the purposes
		// of this example.
		leaves[i] = chainhash.Hash{}
	}

	merkleRoot := standalone.CalcMerkleRoot(leaves)
	fmt.Printf("Result: %s", merkleRoot)

	// Output:
	// Result: 5fdfcaba377aefc1bfc4af5ef8e0c2a61656e10e8105c4db7656ae5d58f8b77f
}
