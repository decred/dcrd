// Copyright (c) 2020-2026 The Decred developers
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

// TestScriptTypes ensures the various methods that determine script types work
// as intended.
func TestScriptTypes(t *testing.T) {
	tests := []struct {
		name           string                  // test description
		scriptSource   *txscript.ScriptBuilder // script to test
		version        uint16                  // script version
		revocation     bool                    // is revocation script
		ticketPurchase bool                    // is ticket purchase script
		vote           bool                    // is vote script
		stakeChange    bool                    // is stake change script
		treasuryGen    bool                    // is treasury gen script
	}{{
		name: "revocation-tagged p2pkh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version:    0,
		revocation: true,
	}, {
		name: "revocation-tagged p2pkh script with unsupported version",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version: 1,
	}, {
		name: "revocation-tagged p2sh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_HASH160).
			AddData(hash160).AddOp(txscript.OP_EQUAL),
		version:    0,
		revocation: true,
	}, {
		name: "revocation-tagged p2sh script with unsupported version",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_HASH160).
			AddData(hash160).AddOp(txscript.OP_EQUAL),
		version: 100,
	}, {
		name: "ticket purchase-tagged p2pkh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSTX).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version:        0,
		ticketPurchase: true,
	}, {
		name: "ticket purchase-tagged p2pkh script with unsupported version",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSTX).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version: 100,
	}, {
		name: "ticket purchase-tagged p2sh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSTX).AddOp(txscript.OP_HASH160).
			AddData(hash160).AddOp(txscript.OP_EQUAL),
		version:        0,
		ticketPurchase: true,
	}, {
		name: "ticket purchase-tagged p2sh script with unsupported version",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSTX).AddOp(txscript.OP_HASH160).
			AddData(hash160).AddOp(txscript.OP_EQUAL),
		version: 100,
	}, {
		name: "vote-tagged p2pkh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version: 0,
		vote:    true,
	}, {
		name: "vote-tagged p2pkh script with unsupported version",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version: 1,
	}, {
		name: "vote-tagged p2sh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_HASH160).
			AddData(hash160).AddOp(txscript.OP_EQUAL),
		version: 0,
		vote:    true,
	}, {
		name: "vote-tagged p2sh script with unsupported version",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSGEN).AddOp(txscript.OP_HASH160).
			AddData(hash160).AddOp(txscript.OP_EQUAL),
		version: 100,
	}, {
		name: "stake change-tagged p2pkh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version:     0,
		stakeChange: true,
	}, {
		name: "stake change-tagged p2pkh script with unsupported version",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version: 1,
	}, {
		name: "stake change-tagged p2sh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_HASH160).
			AddData(hash160).AddOp(txscript.OP_EQUAL),
		version:     0,
		stakeChange: true,
	}, {
		name: "stake change-tagged p2sh script with unsupported version",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_SSTXCHANGE).AddOp(txscript.OP_HASH160).
			AddData(hash160).AddOp(txscript.OP_EQUAL),
		version: 100,
	}, {
		name: "treasurygen-tagged p2pkh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_TGEN).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version:     0,
		treasuryGen: true,
	}, {
		name: "treasurygen-tagged p2pkh script with unsupported version",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_TGEN).AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).AddData(hash160).
			AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
		version: 9999,
	}, {
		name: "treasurygen-tagged p2sh script",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_TGEN).AddOp(txscript.OP_HASH160).
			AddData(hash160).AddOp(txscript.OP_EQUAL),
		version:     0,
		treasuryGen: true,
	}, {
		name: "treasurygen-tagged p2sh script with unsupported version",
		scriptSource: txscript.NewScriptBuilder().
			AddOp(txscript.OP_TGEN).AddOp(txscript.OP_HASH160).
			AddData(hash160).AddOp(txscript.OP_EQUAL),
		version: 100,
	}}

	for _, test := range tests {
		script, err := test.scriptSource.Script()
		if err != nil {
			t.Fatalf("%s: unexpected script generation error: %s", test.name,
				err)
		}

		gotRevocation := IsRevocationScript(test.version, script)
		if gotRevocation != test.revocation {
			t.Fatalf("%s: unexpected revocation script result - got %v, want %v",
				test.name, gotRevocation, test.revocation)
		}

		gotPurchase := IsTicketPurchaseScript(test.version, script)
		if gotPurchase != test.ticketPurchase {
			t.Fatalf("%s: unexpected ticket purchase script result - got %v, "+
				"want %v", test.name, gotPurchase, test.ticketPurchase)
		}

		gotVote := IsVoteScript(test.version, script)
		if gotVote != test.vote {
			t.Fatalf("%s: unexpected vote script result - got %v, want %v",
				test.name, gotVote, test.vote)
		}

		gotStakeChange := IsStakeChangeScript(test.version, script)
		if gotStakeChange != test.stakeChange {
			t.Fatalf("%s: unexpected stake change script result - got %v, "+
				"want %v", test.name, gotStakeChange, test.stakeChange)
		}

		gotTreasuryGen := IsTreasuryGenScript(test.version, script)
		if gotTreasuryGen != test.treasuryGen {
			t.Fatalf("%s: unexpected treasury gen script result - got %v, "+
				"want %v", test.name, gotTreasuryGen, test.treasuryGen)
		}
	}
}
