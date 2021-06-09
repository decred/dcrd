// Copyright (c) 2020-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"testing"

	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
)

var (
	hash160 = stdaddr.Hash160([]byte("test"))
)

func TestIsRevocationScript(t *testing.T) {
	tests := []struct {
		name         string
		scriptSource *txscript.ScriptBuilder
		version      uint16
		expected     bool
	}{
		{
			name: "revocation-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: true,
		},
		{
			name: "revocation-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "vote-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: false,
		},
		{
			name: "vote-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "revocation-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: true,
		},
		{
			name: "revocation-tagged p2sh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  100,
			expected: false,
		},
		{
			name: "ticket purchase-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: false,
		},
	}

	for _, test := range tests {
		script, err := test.scriptSource.Script()
		if err != nil {
			t.Fatalf("%s: unexpected script generation error: %s",
				test.name, err)
		}

		result := IsRevocationScript(test.version, script)
		if result != test.expected {
			t.Fatalf("%s: expected %v, got %v", test.name,
				test.expected, result)
		}
	}
}

func TestIsTicketPurchaseScript(t *testing.T) {
	tests := []struct {
		name         string
		scriptSource *txscript.ScriptBuilder
		version      uint16
		expected     bool
	}{
		{
			name: "ticket purchase-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTX).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: true,
		},
		{
			name: "ticket purchase-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTX).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "vote-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: false,
		},
		{
			name: "vote-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "ticket purchase-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: true,
		},
		{
			name: "ticket purchase-tagged p2sh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  100,
			expected: false,
		},
		{
			name: "revocation-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: false,
		},
	}

	for _, test := range tests {
		script, err := test.scriptSource.Script()
		if err != nil {
			t.Fatalf("%s: unexpected script generation error: %s",
				test.name, err)
		}

		result := IsTicketPurchaseScript(test.version, script)
		if result != test.expected {
			t.Fatalf("%s, expected %v, got %v", test.name,
				test.expected, result)
		}
	}
}

func TestIsVoteScript(t *testing.T) {
	tests := []struct {
		name         string
		scriptSource *txscript.ScriptBuilder
		version      uint16
		expected     bool
	}{
		{
			name: "vote-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: true,
		},
		{
			name: "vote-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "ticket purchase-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTX).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: false,
		},
		{
			name: "ticket purchase-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTX).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "vote-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: true,
		},
		{
			name: "vote-tagged p2sh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  100,
			expected: false,
		},
		{
			name: "revocation-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: false,
		},
	}

	for _, test := range tests {
		script, err := test.scriptSource.Script()
		if err != nil {
			t.Fatalf("%s: unexpected script generation error: %s",
				test.name, err)
		}

		result := IsVoteScript(test.version, script)
		if result != test.expected {
			t.Fatalf("%s, expected %v, got %v", test.name,
				test.expected, result)
		}
	}
}

func TestIsStakeChangeScript(t *testing.T) {
	tests := []struct {
		name         string
		scriptSource *txscript.ScriptBuilder
		version      uint16
		expected     bool
	}{
		{
			name: "stake change-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: true,
		},
		{
			name: "stake change-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "vote-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: false,
		},
		{
			name: "vote-tagged p2pkh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  1,
			expected: false,
		},
		{
			name: "stake change-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: true,
		},
		{
			name: "stake change-tagged p2sh script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  100,
			expected: false,
		},
		{
			name: "revocation-tagged p2sh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_HASH160).
				AddData(hash160).AddOp(txscript.OP_EQUAL),
			version:  0,
			expected: false,
		},
	}

	for _, test := range tests {
		script, err := test.scriptSource.Script()
		if err != nil {
			t.Fatalf("%s: unexpected script generation error: %s",
				test.name, err)
		}

		result := IsStakeChangeScript(test.version, script)
		if result != test.expected {
			t.Fatalf("%s, expected %v, got %v", test.name,
				test.expected, result)
		}
	}
}

// TestIsTreasuryGenScript ensures the method to determine if a script is a
// treasury generation script works as intended.
func TestIsTreasuryGenScript(t *testing.T) {
	tests := []struct {
		name         string
		scriptSource *txscript.ScriptBuilder
		version      uint16
		expected     bool
	}{{
		name: "treasurygen-tagged p2pkh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_TGEN).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version:  0,
		expected: true,
	}, {
		name: "treasurygen-tagged p2pkh script with unsupported version",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_TGEN).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version:  9999,
		expected: false,
	}, {
		name: "stake change-tagged p2pkh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version:  0,
		expected: false,
	}, {
		name: "stake change-tagged p2pkh script with unsupported version",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version:  1,
		expected: false,
	}, {
		name: "vote-tagged p2pkh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version:  0,
		expected: false,
	}, {
		name: "vote-tagged p2pkh script with unsupported version",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version:  1,
		expected: false,
	}, {
		name: "treasurygen-tagged p2sh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_TGEN).AddOp(txscript.OP_HASH160).
			AddData(hash160).AddOp(txscript.OP_EQUAL),
		version:  0,
		expected: true,
	}, {
		name: "treasurygen-tagged p2sh script with unsupported version",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_TGEN).AddOp(txscript.OP_HASH160).
			AddData(hash160).AddOp(txscript.OP_EQUAL),
		version:  100,
		expected: false,
	}, {
		name: "stake change-tagged p2sh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_HASH160).
			AddData(hash160).AddOp(txscript.OP_EQUAL),
		version:  0,
		expected: false,
	}, {
		name: "stake change-tagged p2sh script with unsupported version",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_HASH160).
			AddData(hash160).AddOp(txscript.OP_EQUAL),
		version:  100,
		expected: false,
	}, {
		name: "revocation-tagged p2sh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_HASH160).
			AddData(hash160).AddOp(txscript.OP_EQUAL),
		version:  0,
		expected: false,
	}}

	for _, test := range tests {
		script, err := test.scriptSource.Script()
		if err != nil {
			t.Fatalf("%q: unexpected script generation error: %s", test.name,
				err)
		}

		result := IsTreasuryGenScript(test.version, script)
		if result != test.expected {
			t.Fatalf("%q: unexpected result -- got %v, want %v", test.name,
				result, test.expected)
		}
	}
}
