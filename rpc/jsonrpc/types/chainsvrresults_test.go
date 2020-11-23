// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import (
	"encoding/json"
	"testing"
)

// TestChainSvrCustomResults ensures any results that have custom marshalling
// work as intended.
// and unmarshal code of results are as expected.
func TestChainSvrCustomResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		result   interface{}
		expected string
	}{
		{
			name: "custom vin marshal with coinbase",
			result: &Vin{
				Coinbase: "021234",
				Sequence: 4294967295,
			},
			expected: `{"coinbase":"021234","sequence":4294967295,"amountin":0,"blockheight":0,"blockindex":0}`,
		},
		{
			name: "custom vin marshal without coinbase",
			result: &Vin{
				Txid: "123",
				Vout: 1,
				Tree: 0,
				ScriptSig: &ScriptSig{
					Asm: "0",
					Hex: "00",
				},
				Sequence: 4294967295,
			},
			expected: `{"txid":"123","vout":1,"tree":0,"sequence":4294967295,"amountin":0,"blockheight":0,"blockindex":0,"scriptSig":{"asm":"0","hex":"00"}}`,
		},
		{
			name: "custom vin marshal with treasurybase",
			result: &Vin{
				Treasurybase: true,
				Sequence:     4294967295,
			},
			expected: `{"treasurybase":true,"sequence":4294967295,"amountin":0,"blockheight":0,"blockindex":0}`,
		},
		{
			name: "custom vin marshal with treasuryspend",
			result: &Vin{
				TreasurySpend: "0000c2",
				Sequence:      4294967295,
			},
			expected: `{"treasuryspend":"0000c2","sequence":4294967295,"amountin":0,"blockheight":0,"blockindex":0}`,
		},
		{
			name: "custom vinprevout marshal with coinbase",
			result: &VinPrevOut{
				Coinbase: "021234",
				Sequence: 4294967295,
			},
			expected: `{"coinbase":"021234","sequence":4294967295}`,
		},
		{
			name: "custom vinprevout marshal without coinbase",
			result: &VinPrevOut{
				Txid: "123",
				Vout: 1,
				ScriptSig: &ScriptSig{
					Asm: "0",
					Hex: "00",
				},
				PrevOut: &PrevOut{
					Addresses: []string{"addr1"},
					Value:     0,
				},
				Sequence: 4294967295,
			},
			expected: `{"txid":"123","vout":1,"tree":0,"scriptSig":{"asm":"0","hex":"00"},"prevOut":{"addresses":["addr1"],"value":0},"sequence":4294967295}`,
		},
		{
			name: "custom vinprevout marshal with treasurybase",
			result: &VinPrevOut{
				Treasurybase: true,
				Sequence:     4294967295,
			},
			expected: `{"treasurybase":true,"sequence":4294967295}`,
		},
		{
			name: "custom vinprevout marshal with treasuryspend",
			result: &VinPrevOut{
				TreasurySpend: "0000c2",
				Sequence:      4294967295,
			},
			expected: `{"treasuryspend":"0000c2","sequence":4294967295}`,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		marshalled, err := json.Marshal(test.result)
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}
		if string(marshalled) != test.expected {
			t.Errorf("Test #%d (%s) unexpected marshalled data - "+
				"got %s, want %s", i, test.name, marshalled,
				test.expected)
			continue
		}
	}
}
