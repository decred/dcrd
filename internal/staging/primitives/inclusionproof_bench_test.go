// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package primitives

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// BenchmarkGenerateInclusionProof benchmarks generating inclusion proofs for
// various numbers of leaves.
func BenchmarkGenerateInclusionProof(b *testing.B) {
	for _, numLeaves := range []uint32{64, 128, 256, 2048} {
		// Generate the specified number of leaves and choose a leaf index
		// accordingly.
		benchName := fmt.Sprintf("%d leaves", numLeaves)
		leaves := make([]chainhash.Hash, 0, numLeaves)
		for i := uint32(0); i < numLeaves; i++ {
			var buf [4]byte
			binary.LittleEndian.PutUint32(buf[:], i)
			leaves = append(leaves, chainhash.HashH(buf[:]))
		}
		leafIndex := numLeaves / 2

		b.Run(benchName, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = GenerateInclusionProof(leaves, leafIndex)
			}
		})
	}
}

// BenchmarkVerifyInclusionProof benchmarks verifying inclusion proofs for
// various numbers of leaves.
func BenchmarkVerifyInclusionProof(b *testing.B) {
	for _, numLeaves := range []uint32{64, 128, 256, 2048} {
		// Generate the specified number of leaves, calculate the merkle root,
		// choose a leaf index, and  generate a proof accordingly.
		benchName := fmt.Sprintf("%d leaves", numLeaves)
		leaves := make([]chainhash.Hash, 0, numLeaves)
		for i := uint32(0); i < numLeaves; i++ {
			var buf [4]byte
			binary.LittleEndian.PutUint32(buf[:], i)
			leaves = append(leaves, chainhash.HashH(buf[:]))
		}
		root := CalcMerkleRoot(leaves)
		leafIndex := numLeaves / 2
		leaf := leaves[leafIndex]
		proof := GenerateInclusionProof(leaves, leafIndex)

		b.Run(benchName, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				verified := VerifyInclusionProof(&root, &leaf, leafIndex, proof)
				if !verified {
					b.Fatalf("%q: failed to verify proof", benchName)
				}
			}
		})
	}
}
