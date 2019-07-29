// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockcf

import (
	"encoding/hex"
	"testing"
)

// BenchmarkAddSigScript benchmarks adding the data pushes in signature scripts
// to a slice of entries that are ultimately included in a filter.
func BenchmarkAddSigScript(b *testing.B) {
	// Use a standard v0 redeem P2PKH signature script.
	sigScriptHex := "483045022100c5df7450e24ba20b0a4f9fa68abe22bd7e67cdeab3ef" +
		"878583a418a10196d909022022ed41a867f6de40601d58679898d0d855a003f42a31" +
		"16bb70938a5a2b1345110121020e0a918baf432ae23d815e4572cc71f9739a5516f9" +
		"70cf1236fa529ee8bd3343"
	sigScript, err := hex.DecodeString(sigScriptHex)
	if err != nil {
		b.Fatalf("failed to initialize input data: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var data Entries
		data.AddSigScript(sigScript)
	}
}
