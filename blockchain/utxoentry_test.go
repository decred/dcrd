// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/decred/dcrd/blockchain/stake/v4"
)

// TestEncodeUtxoFlags validates that the correct bit representation is returned
// for various combinations of flags.
func TestEncodeUtxoFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		coinbase  bool
		spent     bool
		modified  bool
		hasExpiry bool
		txType    stake.TxType
		want      utxoFlags
	}{{
		name:      "no flags set, regular tx",
		coinbase:  false,
		spent:     false,
		modified:  false,
		hasExpiry: false,
		txType:    stake.TxTypeRegular,
		want:      0x00,
	}, {
		name:      "coinbase, has expiry, vote tx",
		coinbase:  true,
		spent:     false,
		modified:  false,
		hasExpiry: true,
		txType:    stake.TxTypeSSGen,
		want:      0x29,
	}, {
		name:      "spent, modified, has expiry, ticket tx",
		coinbase:  false,
		spent:     true,
		modified:  true,
		hasExpiry: true,
		txType:    stake.TxTypeSStx,
		want:      0x1e,
	}}

	for _, test := range tests {
		got := encodeUtxoFlags(test.coinbase, test.spent, test.modified,
			test.hasExpiry, test.txType)
		if got != test.want {
			t.Errorf("%q: unexpected result -- got %x, want %x", test.name, got,
				test.want)
		}
	}
}

// TestIsTicketSubmissionOutput validates that the determination of whether
// various inputs represent a ticket submission output or not is correct.
func TestIsTicketSubmissionOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		txType   stake.TxType
		txOutIdx uint32
		want     bool
	}{{
		name:     "ticket tx, output 0",
		txType:   stake.TxTypeSStx,
		txOutIdx: 0,
		want:     true,
	}, {
		name:     "ticket tx, output 1",
		txType:   stake.TxTypeSStx,
		txOutIdx: 1,
		want:     false,
	}, {
		name:     "vote tx, output 0",
		txType:   stake.TxTypeSSGen,
		txOutIdx: 0,
		want:     false,
	}}

	for _, test := range tests {
		got := isTicketSubmissionOutput(test.txType, test.txOutIdx)
		if got != test.want {
			t.Errorf("%q: unexpected result -- got %v, want %v", test.name, got,
				test.want)
		}
	}
}

// TestUtxoEntry validates that the utxo entry methods behave as expected under
// a variety of conditions.
func TestUtxoEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                      string
		coinbase                  bool
		spent                     bool
		modified                  bool
		expiry                    bool
		txType                    stake.TxType
		amount                    int64
		pkScript                  []byte
		blockHeight               uint32
		blockIndex                uint32
		scriptVersion             uint16
		ticketMinOuts             *ticketMinimalOutputs
		deserializedTicketMinOuts []*stake.MinimalOutput
	}{{
		name:     "coinbase output",
		coinbase: true,
		txType:   stake.TxTypeRegular,
		amount:   9999,
		pkScript: hexToBytes("76a9146edbc6c4d31bae9f1ccc38538a114bf42de65e8688a" +
			"c"),
		blockHeight:   54321,
		blockIndex:    0,
		scriptVersion: 0,
	}, {
		name:   "ticket submission output",
		expiry: true,
		txType: stake.TxTypeSStx,
		amount: 4294959555,
		pkScript: hexToBytes("ba76a914a13afb81d54c9f8bb0c5e082d56fd563ab9b359688a" +
			"c"),
		blockHeight:   55555,
		blockIndex:    1,
		scriptVersion: 0,
		ticketMinOuts: &ticketMinimalOutputs{
			data: hexToBytes("03808efefade57001aba76a914a13afb81d54c9f8bb0c5e082d56" +
				"fd563ab9b359688ac0000206a1e9ac39159847e259c9162405b5f6c8135d2c7eaf1a" +
				"375040001000000005800001abd76a91400000000000000000000000000000000000" +
				"0000088ac"),
		},
		deserializedTicketMinOuts: []*stake.MinimalOutput{{
			PkScript: hexToBytes("ba76a914a13afb81d54c9f8bb0c5e082d56fd563ab9b35968" +
				"8ac"),
			Value:   4294959555,
			Version: 0,
		}, {
			PkScript: hexToBytes("6a1e9ac39159847e259c9162405b5f6c8135d2c7eaf1a3750" +
				"400010000000058"),
			Value:   0,
			Version: 0,
		}, {
			PkScript: hexToBytes("bd76a91400000000000000000000000000000000000000008" +
				"8ac"),
			Value:   0,
			Version: 0,
		}},
	}}

	for _, test := range tests {
		// Create an entry given the parameters for the current test.
		entry := &UtxoEntry{
			amount:        test.amount,
			pkScript:      test.pkScript,
			ticketMinOuts: test.ticketMinOuts,
			blockHeight:   test.blockHeight,
			blockIndex:    test.blockIndex,
			scriptVersion: test.scriptVersion,
			packedFlags: encodeUtxoFlags(
				test.coinbase,
				test.spent,
				test.modified,
				test.expiry,
				test.txType,
			),
		}

		// Validate the modified flag.
		isModified := entry.isModified()
		if isModified != test.modified {
			t.Fatalf("unexpected modified flag -- got %v, want %v", isModified,
				test.modified)
		}

		// Validate the coinbase flag.
		isCoinBase := entry.IsCoinBase()
		if isCoinBase != test.coinbase {
			t.Fatalf("unexpected coinbase flag -- got %v, want %v", isCoinBase,
				test.coinbase)
		}

		// Validate the spent flag.
		isSpent := entry.IsSpent()
		if isSpent != test.spent {
			t.Fatalf("unexpected spent flag -- got %v, want %v", isSpent, test.spent)
		}

		// Validate the expiry flag.
		hasExpiry := entry.HasExpiry()
		if hasExpiry != test.expiry {
			t.Fatalf("unexpected expiry flag -- got %v, want %v", hasExpiry,
				test.expiry)
		}

		// Validate the height of the block containing the output.
		gotBlockHeight := entry.BlockHeight()
		if gotBlockHeight != int64(test.blockHeight) {
			t.Fatalf("unexpected block height -- got %v, want %v", gotBlockHeight,
				int64(test.blockHeight))
		}

		// Validate the index of the transaction that the output is contained in.
		gotBlockIndex := entry.BlockIndex()
		if gotBlockIndex != test.blockIndex {
			t.Fatalf("unexpected block index -- got %v, want %v", gotBlockIndex,
				test.blockIndex)
		}

		// Validate the type of the transaction that the output is contained in.
		gotTxType := entry.TransactionType()
		if gotTxType != test.txType {
			t.Fatalf("unexpected transaction type -- got %v, want %v", gotTxType,
				test.txType)
		}

		// Validate the amount of the output.
		gotAmount := entry.Amount()
		if gotAmount != test.amount {
			t.Fatalf("unexpected amount -- got %v, want %v", gotAmount, test.amount)
		}

		// Validate the script of the output.
		gotScript := entry.PkScript()
		if !bytes.Equal(gotScript, test.pkScript) {
			t.Fatalf("unexpected script -- got %v, want %v", gotScript, test.pkScript)
		}

		// Validate the script version of the output.
		gotScriptVersion := entry.ScriptVersion()
		if gotScriptVersion != test.scriptVersion {
			t.Fatalf("unexpected script version -- got %v, want %v", gotScriptVersion,
				test.scriptVersion)
		}

		// Spend the entry.  Validate that it is marked as spent and modified.
		entry.Spend()
		if !entry.IsSpent() {
			t.Fatal("expected entry to be spent")
		}
		if !entry.isModified() {
			t.Fatal("expected entry to be modified")
		}

		// Validate that if spend is called again the entry is still marked as spent
		// and modified.
		entry.Spend()
		if !entry.IsSpent() {
			t.Fatal("expected entry to still be marked as spent")
		}
		if !entry.isModified() {
			t.Fatal("expected entry to still be marked as modified")
		}

		// Validate the ticket minimal outputs.
		ticketMinOutsResult := entry.TicketMinimalOutputs()
		if !reflect.DeepEqual(ticketMinOutsResult, test.deserializedTicketMinOuts) {
			t.Fatalf("unexpected ticket min outs -- got %v, want %v",
				ticketMinOutsResult, test.deserializedTicketMinOuts)
		}

		// Clone the entry and validate that all values are deep equal.
		clonedEntry := entry.Clone()
		if !reflect.DeepEqual(clonedEntry, entry) {
			t.Fatalf("expected entry to be equal to cloned entry -- got %v, want %v",
				clonedEntry, entry)
		}

		// Validate that clone returns nil when called on a nil entry.
		var nilEntry *UtxoEntry
		if nilEntry.Clone() != nil {
			t.Fatal("expected nil when calling clone on a nil entry")
		}
	}
}
