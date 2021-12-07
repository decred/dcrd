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
		hasExpiry bool
		txType    stake.TxType
		want      utxoFlags
	}{{
		name:      "no flags set, regular tx",
		coinbase:  false,
		hasExpiry: false,
		txType:    stake.TxTypeRegular,
		want:      0x00,
	}, {
		name:      "coinbase, has expiry, vote tx",
		coinbase:  true,
		hasExpiry: true,
		txType:    stake.TxTypeSSGen,
		want:      0x0b,
	}, {
		name:      "has expiry, ticket tx",
		coinbase:  false,
		hasExpiry: true,
		txType:    stake.TxTypeSStx,
		want:      0x06,
	}}

	for _, test := range tests {
		got := encodeUtxoFlags(test.coinbase, test.hasExpiry, test.txType)
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
		spent                     bool
		modified                  bool
		fresh                     bool
		coinbase                  bool
		expiry                    bool
		txType                    stake.TxType
		amount                    int64
		pkScript                  []byte
		blockHeight               uint32
		blockIndex                uint32
		scriptVersion             uint16
		ticketMinOuts             *ticketMinimalOutputs
		deserializedTicketMinOuts []*stake.MinimalOutput
		size                      uint64
	}{{
		name:     "coinbase output",
		coinbase: true,
		txType:   stake.TxTypeRegular,
		amount:   9999,
		pkScript: hexToBytes("76a9146edbc6c4d31bae9f1ccc38538a114bf42de65e868" +
			"8ac"),
		blockHeight:   54321,
		blockIndex:    0,
		scriptVersion: 0,
		// baseEntrySize + len(pkScript).
		size: baseEntrySize + 25,
	}, {
		name:   "ticket submission output",
		fresh:  true,
		expiry: true,
		txType: stake.TxTypeSStx,
		amount: 4294959555,
		pkScript: hexToBytes("ba76a914a13afb81d54c9f8bb0c5e082d56fd563ab9b359" +
			"688ac"),
		blockHeight:   55555,
		blockIndex:    1,
		scriptVersion: 0,
		ticketMinOuts: &ticketMinimalOutputs{
			data: hexToBytes("03808efefade57001aba76a914a13afb81d54c9f8bb0c5e" +
				"082d56fd563ab9b359688ac0000206a1e9ac39159847e259c9162405b5f6" +
				"c8135d2c7eaf1a375040001000000005800001abd76a9140000000000000" +
				"00000000000000000000000000088ac"),
		},
		deserializedTicketMinOuts: []*stake.MinimalOutput{{
			PkScript: hexToBytes("ba76a914a13afb81d54c9f8bb0c5e082d56fd563ab9" +
				"b359688ac"),
			Value:   4294959555,
			Version: 0,
		}, {
			PkScript: hexToBytes("6a1e9ac39159847e259c9162405b5f6c8135d2c7eaf" +
				"1a3750400010000000058"),
			Value:   0,
			Version: 0,
		}, {
			PkScript: hexToBytes("bd76a91400000000000000000000000000000000000" +
				"0000088ac"),
			Value:   0,
			Version: 0,
		}},
		// baseEntrySize + len(pkScript) + len(ticketMinOuts.data).
		size: baseEntrySize + 26 + 99,
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
				test.expiry,
				test.txType,
			),
		}

		// Set state flags given the parameters for the current test.
		if test.spent {
			entry.state |= utxoStateSpent
		}
		if test.modified {
			entry.state |= utxoStateModified
		}
		if test.fresh {
			entry.state |= utxoStateFresh
		}

		// Validate the size of the entry.
		size := entry.size()
		if size != test.size {
			t.Fatalf("%q: unexpected size -- got %v, want %v", test.name, size,
				test.size)
		}

		// Validate the spent flag.
		isSpent := entry.IsSpent()
		if isSpent != test.spent {
			t.Fatalf("%q: unexpected spent flag -- got %v, want %v", test.name,
				isSpent, test.spent)
		}

		// Validate the modified flag.
		isModified := entry.isModified()
		if isModified != test.modified {
			t.Fatalf("%q: unexpected modified flag -- got %v, want %v",
				test.name, isModified, test.modified)
		}

		// Validate the fresh flag.
		isFresh := entry.isFresh()
		if isFresh != test.fresh {
			t.Fatalf("%q: unexpected fresh flag -- got %v, want %v", test.name,
				isFresh, test.fresh)
		}

		// Validate the coinbase flag.
		isCoinBase := entry.IsCoinBase()
		if isCoinBase != test.coinbase {
			t.Fatalf("%q: unexpected coinbase flag -- got %v, want %v",
				test.name, isCoinBase, test.coinbase)
		}

		// Validate the expiry flag.
		hasExpiry := entry.HasExpiry()
		if hasExpiry != test.expiry {
			t.Fatalf("%q: unexpected expiry flag -- got %v, want %v", test.name,
				hasExpiry, test.expiry)
		}

		// Validate the type of the transaction that the output is contained in.
		gotTxType := entry.TransactionType()
		if gotTxType != test.txType {
			t.Fatalf("%q: unexpected transaction type -- got %v, want %v",
				test.name, gotTxType, test.txType)
		}

		// Validate the height of the block containing the output.
		gotBlockHeight := entry.BlockHeight()
		if gotBlockHeight != int64(test.blockHeight) {
			t.Fatalf("%q: unexpected block height -- got %v, want %v",
				test.name, gotBlockHeight, int64(test.blockHeight))
		}

		// Validate the index of the transaction that the output is contained
		// in.
		gotBlockIndex := entry.BlockIndex()
		if gotBlockIndex != test.blockIndex {
			t.Fatalf("%q: unexpected block index -- got %v, want %v", test.name,
				gotBlockIndex, test.blockIndex)
		}

		// Validate the amount of the output.
		gotAmount := entry.Amount()
		if gotAmount != test.amount {
			t.Fatalf("%q: unexpected amount -- got %v, want %v", test.name,
				gotAmount, test.amount)
		}

		// Validate the script of the output.
		gotScript := entry.PkScript()
		if !bytes.Equal(gotScript, test.pkScript) {
			t.Fatalf("%q: unexpected script -- got %v, want %v", test.name,
				gotScript, test.pkScript)
		}

		// Validate the script version of the output.
		gotScriptVersion := entry.ScriptVersion()
		if gotScriptVersion != test.scriptVersion {
			t.Fatalf("%q: unexpected script version -- got %v, want %v",
				test.name, gotScriptVersion, test.scriptVersion)
		}

		// Spend the entry.  Validate that it is marked as spent and modified.
		entry.Spend()
		if !entry.IsSpent() {
			t.Fatalf("%q: expected entry to be spent", test.name)
		}
		if !entry.isModified() {
			t.Fatalf("%q: expected entry to be modified", test.name)
		}

		// Validate that if spend is called again the entry is still marked as
		// spent and modified.
		entry.Spend()
		if !entry.IsSpent() {
			t.Fatalf("%q: expected entry to still be marked as spent",
				test.name)
		}
		if !entry.isModified() {
			t.Fatalf("%q: expected entry to still be marked as modified",
				test.name)
		}

		// Validate the ticket minimal outputs.
		ticketMinOutsResult := entry.TicketMinimalOutputs()
		if !reflect.DeepEqual(ticketMinOutsResult,
			test.deserializedTicketMinOuts) {

			t.Fatalf("%q: unexpected ticket min outs -- got %v, want %v",
				test.name, ticketMinOutsResult, test.deserializedTicketMinOuts)
		}

		// Clone the entry and validate that all values are deep equal.
		clonedEntry := entry.Clone()
		if !reflect.DeepEqual(clonedEntry, entry) {
			t.Fatalf("%q: expected entry to be equal to cloned entry -- got "+
				"%v, want %v", test.name, clonedEntry, entry)
		}

		// Validate that clone returns nil when called on a nil entry.
		var nilEntry *UtxoEntry
		if nilEntry.Clone() != nil {
			t.Fatalf("%q: expected nil when calling clone on a nil entry",
				test.name)
		}
	}
}
