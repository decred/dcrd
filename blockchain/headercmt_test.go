// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// TestCalcCommitmentRootV1 ensures the expected version 1 commitment root is
// produced for known values.
func TestCalcCommitmentRootV1(t *testing.T) {
	tests := []struct {
		name       string // test description
		filterHash string // filter hash to commit to
		want       string // expected result
	}{{
		name:       "mainnet block 1",
		filterHash: "e4f2ca9db1a0aef9295ed4154f9fb852e169afe56c11c602e34852fe49e8f408",
		want:       "e4f2ca9db1a0aef9295ed4154f9fb852e169afe56c11c602e34852fe49e8f408",
	}, {
		name:       "mainnet block 2",
		filterHash: "af7d2082aa44d453590d018a4a15592ea9c6bd1980fefe8bd4c848d78c5fec64",
		want:       "af7d2082aa44d453590d018a4a15592ea9c6bd1980fefe8bd4c848d78c5fec64",
	}, {
		name:       "mainnet block 3",
		filterHash: "df9376f3ffaf0d222b65cad30c9366d8951bbe95041f504e235f1165ba453ba8",
		want:       "df9376f3ffaf0d222b65cad30c9366d8951bbe95041f504e235f1165ba453ba8",
	}}

	for _, test := range tests {
		filterHash, err := chainhash.NewHashFromStr(test.filterHash)
		if err != nil {
			t.Errorf("%q: unexpected err parsing filter hash %q: %v", test.name,
				test.filterHash, err)
			continue
		}

		// Parse the expected merkle root.
		want, err := chainhash.NewHashFromStr(test.want)
		if err != nil {
			t.Errorf("%q: unexpected err parsing want hex: %v", test.name, err)
			continue
		}

		result := CalcCommitmentRootV1(*filterHash)
		if result != *want {
			t.Errorf("%q: mismatched result -- got %v, want %v", test.name,
				result, *want)
			continue
		}
	}
}
