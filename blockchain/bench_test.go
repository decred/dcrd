// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"
)

// BenchmarkAncestor benchmarks ancestor traversal for various numbers of nodes.
func BenchmarkAncestor(b *testing.B) {
	// Construct a synthetic block chain with consisting of the following
	// structure.
	// 	0 -> 1 -> 2 -> ... -> 499997 -> 499998 -> 499999  -> 500000
	// 	                                      \-> 499999a
	// 	                            \-> 499998a
	branch0Nodes := chainedFakeSkipListNodes(nil, 500001)
	branch1Nodes := chainedFakeSkipListNodes(branch0Nodes[499998], 1)
	branch2Nodes := chainedFakeSkipListNodes(branch0Nodes[499997], 1)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		branchTip(branch0Nodes).Ancestor(0)
		branchTip(branch0Nodes).Ancestor(131072) // Power of two.
		branchTip(branch0Nodes).Ancestor(131071) // One less than power of two.
		branchTip(branch0Nodes).Ancestor(131070) // Two less than power of two.
		branchTip(branch1Nodes).Ancestor(0)
		branchTip(branch2Nodes).Ancestor(0)
	}
}
