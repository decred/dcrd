// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2015-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/internal/staging/primitives"
	"github.com/decred/dcrd/math/uint256"
	"github.com/decred/dcrd/wire"
)

// hexToFinalState converts the passed hex string into an array of 6 bytes and
// will panic if there is an error.  This is only provided for the hard-coded
// constants so errors in the source code can be detected. It will only (and
// must only) be called with hard-coded values.
func hexToFinalState(s string) [6]byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}

	var finalState [6]byte
	if len(b) != len(finalState) {
		panic("invalid hex in source file: " + s)
	}
	copy(finalState[:], b)
	return finalState
}

// hexToExtraData converts the passed hex string into an array of 32 bytes and
// will panic if there is an error.  This is only provided for the hard-coded
// constants so errors in the source code can be detected. It will only (and
// must only) be called with hard-coded values.
func hexToExtraData(s string) [32]byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}

	var extraData [32]byte
	if len(b) != len(extraData) {
		panic("invalid hex in source file: " + s)
	}
	copy(extraData[:], b)
	return extraData
}

// isNotInMainChainErr returns whether or not the passed error is an
// errNotInMainChain error.
func isNotInMainChainErr(err error) bool {
	var e errNotInMainChain
	return errors.As(err, &e)
}

// TestErrNotInMainChain ensures the functions related to errNotInMainChain work
// as expected.
func TestErrNotInMainChain(t *testing.T) {
	errStr := "no block at height 1 exists"
	err := error(errNotInMainChain(errStr))

	// Ensure the stringized output for the error is as expected.
	if err.Error() != errStr {
		t.Fatalf("errNotInMainChain returned unexpected error string - "+
			"got %q, want %q", err.Error(), errStr)
	}

	// Ensure error is detected as the correct type.
	if !isNotInMainChainErr(err) {
		t.Fatalf("isNotInMainChainErr did not detect as expected type")
	}
	err = errors.New("something else")
	if isNotInMainChainErr(err) {
		t.Fatalf("isNotInMainChainErr detected incorrect type")
	}
}

// TestBlockIndexSerialization ensures serializing and deserializing block index
// entries works as expected.
func TestBlockIndexSerialization(t *testing.T) {
	t.Parallel()

	// base data is based on block 150287 on mainnet and serves as a template
	// for the various tests below.
	baseHeader := wire.BlockHeader{
		Version:      4,
		PrevBlock:    *mustParseHash("000000000000016916671ae225343a5ee131c999d5cadb6348805db25737731f"),
		MerkleRoot:   *mustParseHash("5ef2bb79795d7503c0ccc5cb6e0d4731992fc8c8c5b332c1c0e2c687d864c666"),
		StakeRoot:    *mustParseHash("022965059b7527dc2bc18daaa533f806eda1f96fd0b04bbda2381f5552d7c2de"),
		VoteBits:     0x0001,
		FinalState:   hexToFinalState("313e16e64c0b"),
		Voters:       4,
		FreshStake:   3,
		Revocations:  2,
		PoolSize:     41332,
		Bits:         0x1a016f98,
		SBits:        7473162478,
		Height:       150287,
		Size:         11295,
		Timestamp:    time.Unix(1499907127, 0),
		Nonce:        4116576260,
		ExtraData:    hexToExtraData("8f01ed92645e0a6b11ee3b3c0000000000000000000000000000000000000000"),
		StakeVersion: 4,
	}
	baseVoteInfo := []stake.VoteVersionTuple{
		{Version: 4, Bits: 0x0001},
		{Version: 4, Bits: 0x0015},
		{Version: 4, Bits: 0x0015},
		{Version: 4, Bits: 0x0001},
	}

	tests := []struct {
		name       string
		entry      blockIndexEntry
		serialized []byte
	}{{
		name: "no votes",
		entry: blockIndexEntry{
			header:   baseHeader,
			status:   statusDataStored | statusValidated,
			voteInfo: nil,
		},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c0000000000000000000000000000000000000000040000000300"),
	}, {
		name: "1 vote",
		entry: blockIndexEntry{
			header:   baseHeader,
			status:   statusDataStored | statusValidated,
			voteInfo: baseVoteInfo[:1],
		},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a" +
			"3425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f99314" +
			"70d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06" +
			"f833a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a100009" +
			"86f011aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92" +
			"645e0a6b11ee3b3c00000000000000000000000000000000000000000400000" +
			"003010401"),
	}, {
		name: "4 votes, same vote versions, different vote bits",
		entry: blockIndexEntry{
			header:   baseHeader,
			status:   statusDataStored | statusValidated,
			voteInfo: baseVoteInfo,
		},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c00000000000000000000000000000000000000000400000003040" +
			"401041504150401"),
	}}

	for _, test := range tests {
		// Ensure the function to calculate the serialized size without
		// actually serializing it is calculated properly.
		gotSize := blockIndexEntrySerializeSize(&test.entry)
		if gotSize != len(test.serialized) {
			t.Errorf("%q: did not get expected size - got %d, want %d", test.name,
				gotSize, len(test.serialized))
		}

		// Ensure the block index entry serializes to the expected value.
		gotSerialized, err := serializeBlockIndexEntry(&test.entry)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", test.name, err)
			continue
		}
		if !bytes.Equal(gotSerialized, test.serialized) {
			t.Errorf("%q: did not get expected bytes - got %x, want %x", test.name,
				gotSerialized, test.serialized)
			continue
		}

		// Ensure the block index entry serializes to the expected value
		// and produces the expected number of bytes written via a
		// direct put.
		gotSerialized2 := make([]byte, gotSize)
		gotBytesWritten, err := putBlockIndexEntry(gotSerialized2,
			&test.entry)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", test.name, err)
			continue
		}
		if !bytes.Equal(gotSerialized2, test.serialized) {
			t.Errorf("%q: did not get expected bytes - got %x, want %x", test.name,
				gotSerialized2, test.serialized)
			continue
		}
		if gotBytesWritten != len(test.serialized) {
			t.Errorf("%q: did not get expected number of bytes written - got %d, "+
				"want %d", test.name, gotBytesWritten, len(test.serialized))
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// block index entry.
		gotEntry, err := deserializeBlockIndexEntry(test.serialized)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", test.name, err)
			continue
		}
		if !reflect.DeepEqual(*gotEntry, test.entry) {
			t.Errorf("%q: mismatched entries\ngot %+v\nwant %+v", test.name, gotEntry,
				test.entry)
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// block index entry.
		var gotEntry2 blockIndexEntry
		bytesRead, err := decodeBlockIndexEntry(test.serialized, &gotEntry2)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", test.name, err)
			continue
		}
		if !reflect.DeepEqual(gotEntry2, test.entry) {
			t.Errorf("%q: mismatched entries\ngot %+v\nwant %+v", test.name,
				gotEntry2, test.entry)
			continue
		}
		if bytesRead != len(test.serialized) {
			t.Errorf("%q: did not get expected number of bytes read - got %d, "+
				"want %d", test.name, bytesRead, len(test.serialized))
			continue
		}
	}
}

// TestBlockIndexDecodeErrors performs negative tests against decoding block
// index entries to ensure error paths work as expected.
func TestBlockIndexDecodeErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		entry      blockIndexEntry
		serialized []byte
		bytesRead  int // Expected number of bytes read.
		errType    error
	}{{
		name:       "nothing serialized",
		entry:      blockIndexEntry{},
		serialized: hexToBytes(""),
		errType:    errDeserialize(""),
		bytesRead:  0,
	}, {
		name:  "no data after block header",
		entry: blockIndexEntry{},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c000000000000000000000000000000000000000004000000"),
		errType:   errDeserialize(""),
		bytesRead: 180,
	}, {
		name:  "no data after status",
		entry: blockIndexEntry{},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c00000000000000000000000000000000000000000400000003"),
		errType:   errDeserialize(""),
		bytesRead: 181,
	}, {
		name:  "no data after num votes with votes",
		entry: blockIndexEntry{},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c0000000000000000000000000000000000000000040000000301"),
		errType:   errDeserialize(""),
		bytesRead: 182,
	}, {
		name:  "no data after vote version",
		entry: blockIndexEntry{},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c000000000000000000000000000000000000000004000000030104"),
		errType:   errDeserialize(""),
		bytesRead: 183,
	}}

	for _, test := range tests {
		// Ensure the expected error type is returned.
		gotBytesRead, err := decodeBlockIndexEntry(test.serialized,
			&test.entry)
		if !errors.Is(err, test.errType) {
			t.Errorf("%q: expected error type does not match - got %T, want %T",
				test.name, err, test.errType)
			continue
		}

		// Ensure the expected number of bytes read is returned.
		if gotBytesRead != test.bytesRead {
			t.Errorf("%q: unexpected number of bytes read - got %d, want %d",
				test.name, gotBytesRead, test.bytesRead)
			continue
		}
	}
}

// TestStxoSerialization ensures serializing and deserializing spent transaction
// output entries works as expected.
func TestStxoSerialization(t *testing.T) {
	t.Parallel()

	// Define constants for indicating flags.
	const (
		noCoinbase   = false
		withCoinbase = true
		noExpiry     = false
		withExpiry   = true
	)

	tests := []struct {
		name       string
		stxo       spentTxOut
		serialized []byte
		txOutIndex uint32
	}{{
		name: "Coinbase, no expiry",
		stxo: spentTxOut{
			amount:        9999,
			scriptVersion: 0,
			pkScript: hexToBytes("76a9146edbc6c4d31bae9f1ccc38538a114bf42de65e8688a" +
				"c"),
			blockHeight: 12345,
			blockIndex:  54321,
			packedFlags: encodeFlags(
				withCoinbase,
				noExpiry,
				stake.TxTypeRegular,
			),
		},
		serialized: hexToBytes("0100006edbc6c4d31bae9f1ccc38538a114bf42de65e86"),
		txOutIndex: 3,
	}, {
		name: "Ticket submission",
		stxo: spentTxOut{
			amount:        4294959555,
			scriptVersion: 0,
			pkScript: hexToBytes("ba76a914a13afb81d54c9f8bb0c5e082d56fd563ab9b35968" +
				"8ac"),
			ticketMinOuts: &ticketMinimalOutputs{
				data: hexToBytes("03808efefade57001aba76a914a13afb81d54c9f8bb0c5e082d" +
					"56fd563ab9b359688ac0000206a1e9ac39159847e259c9162405b5f6c8135d2c7e" +
					"af1a375040001000000005800001abd76a91400000000000000000000000000000" +
					"0000000000088ac"),
			},
			blockHeight: 85314,
			blockIndex:  6,
			packedFlags: encodeFlags(
				noCoinbase,
				withExpiry,
				stake.TxTypeSStx,
			),
		},
		serialized: hexToBytes("06005aba76a914a13afb81d54c9f8bb0c5e082d56fd563ab" +
			"9b359688ac03808efefade57001aba76a914a13afb81d54c9f8bb0c5e082d56fd563a" +
			"b9b359688ac0000206a1e9ac39159847e259c9162405b5f6c8135d2c7eaf1a3750400" +
			"01000000005800001abd76a914000000000000000000000000000000000000000088ac"),
		txOutIndex: 0,
	}}

	for _, test := range tests {
		// Ensure the function to calculate the serialized size without
		// actually serializing it is calculated properly.
		gotSize := spentTxOutSerializeSize(&test.stxo)
		if gotSize != len(test.serialized) {
			t.Errorf("%q: did not get expected size - got %d, want %d", test.name,
				gotSize, len(test.serialized))
		}

		// Ensure the stxo serializes to the expected value.
		gotSerialized := make([]byte, gotSize)
		gotBytesWritten := putSpentTxOut(gotSerialized, &test.stxo)
		if !bytes.Equal(gotSerialized, test.serialized) {
			t.Errorf("%q: did not get expected bytes - got %x, want %x", test.name,
				gotSerialized, test.serialized)
			continue
		}
		if gotBytesWritten != len(test.serialized) {
			t.Errorf("%q: did not get expected number of bytes written - got %d, "+
				"want %d", test.name, gotBytesWritten, len(test.serialized))
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// stxo.
		var gotStxo spentTxOut
		offset, err := decodeSpentTxOut(test.serialized, &gotStxo,
			test.stxo.amount, test.stxo.blockHeight, test.stxo.blockIndex,
			test.txOutIndex)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", test.name, err)
			continue
		}
		if !reflect.DeepEqual(gotStxo, test.stxo) {
			t.Errorf("%q: mismatched stxo:\nwant: %+v\n got: %+v\n", test.name,
				test.stxo, gotStxo)
		}
		if offset != len(test.serialized) {
			t.Errorf("%q: did not get expected number of bytes read - got %d, "+
				"want %d", test.name, offset, len(test.serialized))
			continue
		}
	}
}

// TestStxoDecodeErrors performs negative tests against decoding spent
// transaction outputs to ensure error paths work as expected.
func TestStxoDecodeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stxo       spentTxOut
		txOutIndex uint32
		serialized []byte
		errType    error
		bytesRead  int // Expected number of bytes read.
	}{{
		// [EOF]
		name:       "nothing serialized (no flags)",
		stxo:       spentTxOut{},
		txOutIndex: 0,
		serialized: hexToBytes(""),
		errType:    errDeserialize(""),
		bytesRead:  0,
	}, {
		// [<flags 01> EOF]
		name:       "no data after flags",
		stxo:       spentTxOut{},
		txOutIndex: 0,
		serialized: hexToBytes("01"),
		errType:    errDeserialize(""),
		bytesRead:  1,
	}, {
		// [<flags 01> <script version 00> <compressed pk script 12> EOF]
		name:       "incomplete compressed txout",
		stxo:       spentTxOut{},
		txOutIndex: 0,
		serialized: hexToBytes("010012"),
		errType:    errDeserialize(""),
		bytesRead:  2,
	}, {
		// [<flags 06> <script version 00> <compressed pk script 01 6e ...> EOF]
		name: "no minimal output data after script for a ticket submission " +
			"output",
		stxo:       spentTxOut{},
		txOutIndex: 0,
		serialized: hexToBytes("0600016edbc6c4d31bae9f1ccc38538a114bf42de65e86"),
		errType:    errDeserialize(""),
		bytesRead:  23,
	}, {
		// [<flags 06> <script version 00> <compressed pk script 01 6e ...>
		//  <ticket min outs {num outputs 01}> EOF]
		name: "truncated minimal output data after script for a ticket " +
			"submission output (num outputs only)",
		stxo:       spentTxOut{},
		serialized: hexToBytes("0600016edbc6c4d31bae9f1ccc38538a114bf42de65e8601"),
		errType:    errDeserialize(""),
		bytesRead:  24,
	}, {
		// [<flags 06> <script version 00> <compressed pk script 01 6e ...>
		//  <ticket min outs {num outputs 01} {amount 0f}> EOF]
		name: "truncated minimal output data after script for a ticket " +
			"submission output (num outputs and amount only)",
		stxo: spentTxOut{},
		serialized: hexToBytes("0600016edbc6c4d31bae9f1ccc38538a114bf42de65e86010" +
			"f"),
		errType:   errDeserialize(""),
		bytesRead: 25,
	}, {
		// [<flags 06> <script version 00> <compressed pk script 01 6e ...>
		//  <ticket min outs {num outputs 01} {amount 0f} {script version 00}> EOF]
		name: "truncated minimal output data after script for a ticket " +
			"submission output (num outputs, amount, and script version only)",
		stxo: spentTxOut{},
		serialized: hexToBytes("0600016edbc6c4d31bae9f1ccc38538a114bf42de65e86010" +
			"f00"),
		errType:   errDeserialize(""),
		bytesRead: 26,
	}, {
		// [<flags 06> <script version 00> <compressed pk script 01 6e ...>
		//  <ticket min outs {num outputs 01} {amount 0f} {script version 00}
		//  {script size 1a} {25 bytes of script instead of 26}> EOF]
		name: "truncated minimal output data after script for a ticket " +
			"submission output (script size specified as 0x1a, but only 0x19 bytes " +
			"provided)",
		stxo: spentTxOut{},
		serialized: hexToBytes("0600016edbc6c4d31bae9f1ccc38538a114bf42de65e86010" +
			"f001aba76a9140cdf9941c0c221243cb8672cd1ad2c4c0933850588"),
		errType:   errDeserialize(""),
		bytesRead: 27,
	}}

	for _, test := range tests {
		// Ensure the expected error type is returned.
		gotBytesRead, err := decodeSpentTxOut(test.serialized,
			&test.stxo, test.stxo.amount, test.stxo.blockHeight, test.stxo.blockIndex,
			test.txOutIndex)
		if !errors.Is(err, test.errType) {
			t.Errorf("%q: expected error type does not match - got %T, want %T",
				test.name, err, test.errType)
			continue
		}

		// Ensure the expected number of bytes read is returned.
		if gotBytesRead != test.bytesRead {
			t.Errorf("%q: unexpected number of bytes read - got %d, want %d",
				test.name, gotBytesRead, test.bytesRead)
			continue
		}
	}
}

// TestSpendJournalSerialization ensures serializing and deserializing spend
// journal entries works as expected.
func TestSpendJournalSerialization(t *testing.T) {
	t.Parallel()

	// Define constants for indicating flags.
	const (
		noCoinbase   = false
		withCoinbase = true
		noExpiry     = false
		withExpiry   = true
	)

	tests := []struct {
		name       string
		entry      []spentTxOut
		blockTxns  []*wire.MsgTx
		serialized []byte
	}{{
		name:       "No spends",
		entry:      nil,
		blockTxns:  nil,
		serialized: nil,
	}, {
		// Adapted from mainnet block 100267:
		//   Regular tx that spends from a coinbase:
		//     575a1f489c1d2135efcfc03f1556151fe4601d5bed235767e0c61cdda2b56757
		//   Vote tx:
		//     b3e36d0ee300d16c5a7fdc6de403c1dcf7434fba84ddcaf2e26b276c80051b98
		//   (other transactions within that block omitted)
		name: "One regular tx that spends from a coinbase and one vote tx",
		entry: []spentTxOut{{
			amount:        1597192852,
			scriptVersion: 0,
			pkScript: hexToBytes("76a914b3c2069c496bc13228c154b03993809f278233" +
				"d188ac"),
			blockHeight: 100000,
			blockIndex:  0,
			packedFlags: encodeFlags(
				withCoinbase,
				noExpiry,
				stake.TxTypeRegular,
			),
		}, {
			amount:        4294959555,
			scriptVersion: 0,
			pkScript: hexToBytes("ba76a914a13afb81d54c9f8bb0c5e082d56fd563ab9b" +
				"359688ac"),
			ticketMinOuts: &ticketMinimalOutputs{
				data: hexToBytes("03808efefade57001aba76a914a13afb81d54c9f8bb0c5e082d" +
					"56fd563ab9b359688ac0000206a1e9ac39159847e259c9162405b5f6c8135d2c7e" +
					"af1a375040001000000005800001abd76a91400000000000000000000000000000" +
					"0000000000088ac"),
			},
			blockHeight: 85314,
			blockIndex:  6,
			packedFlags: encodeFlags(
				noCoinbase,
				withExpiry,
				stake.TxTypeSStx,
			),
		}},
		blockTxns: []*wire.MsgTx{{
			SerType: wire.TxSerializeFull,
			Version: 1,
			TxIn: []*wire.TxIn{{
				PreviousOutPoint: wire.OutPoint{
					Hash: *mustParseHash("0e0c8ac0b57b7bff8461e3c9e251ee05b1d04ee5ef971" +
						"4b6414ea4bff1939fb9"),
					Index: 2,
					Tree:  0,
				},
				SignatureScript: hexToBytes("483045022100b7c226f487d4f3086c6f3b27a3f8" +
					"6c15078ee3fc40effee28e88ad5bda81c36a02204ad34bb78ff170c3eaa383a12a" +
					"d159c6104744a0bfd13130476a85240297a6fe012103da2b1c13507c9b96b39952" +
					"0c3233f6c81eb4beed2d30be8077ad3a04854e4fec"),
				Sequence:    0xffffffff,
				BlockHeight: 100000,
				BlockIndex:  0,
				ValueIn:     1597192852,
			}},
			TxOut: []*wire.TxOut{{
				Value:   1388604152,
				Version: 0,
				PkScript: hexToBytes("76a9142bf10bc24646c590d16b35b925cf7ad3c5aef0b28" +
					"8ac"),
			}, {
				Value:   208335700,
				Version: 0,
				PkScript: hexToBytes("76a914cf71aca9293855190e270650c405395ed00dfca58" +
					"8ac"),
			}},
			LockTime: 0,
			Expiry:   0,
		}, {
			SerType: wire.TxSerializeFull,
			Version: 1,
			TxIn: []*wire.TxIn{{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 0xffffffff,
					Tree:  0,
				},
				SignatureScript: hexToBytes("0000"),
				Sequence:        0xffffffff,
				BlockHeight:     wire.NullBlockHeight,
				BlockIndex:      wire.NullBlockIndex,
				ValueIn:         159626785,
			}, {
				PreviousOutPoint: wire.OutPoint{
					Hash: *mustParseHash("d3bce77da2747baa85fb7ca4f6f8e123f31cd15ac691b" +
						"2f82543780158587d3a"),
					Index: 0,
					Tree:  1,
				},
				SignatureScript: hexToBytes("483045022100cc166d42c07e7a59e4b5a4ee13c0" +
					"c9f96a4cd1a7f566356b8d599dec7126e52a0220126e28c59113efc66d28e04f8b" +
					"4dc82538304f934ba4039831b5e814f69a2344012102a26ab1e011211185fe3583" +
					"bc6393e713f65751cf000c00d9b80e12610cbb5a92ab9b359688ac"),
				Sequence:    0xffffffff,
				BlockHeight: 85314,
				BlockIndex:  6,
				ValueIn:     4294959555,
			}},
			TxOut: []*wire.TxOut{{
				Value:   0,
				Version: 0,
				PkScript: hexToBytes("6a24014c0832ff0ce236cd774274772f2e31aab07dda514" +
					"36cbd6d03000000000000aa870100"),
			}, {
				Value:    0,
				Version:  0,
				PkScript: hexToBytes("6a06010002000000"),
			}, {
				Value:   4454586340,
				Version: 0,
				PkScript: hexToBytes("bb76a9149ac39159847e259c9162405b5f6c8135d2c7eaf" +
					"188ac"),
			}},
			LockTime: 0,
			Expiry:   0,
		}},
		serialized: hexToBytes("06005aba76a914a13afb81d54c9f8bb0c5e082d56fd563ab9" +
			"b359688ac03808efefade57001aba76a914a13afb81d54c9f8bb0c5e082d56fd563ab9" +
			"b359688ac0000206a1e9ac39159847e259c9162405b5f6c8135d2c7eaf1a3750400010" +
			"00000005800001abd76a914000000000000000000000000000000000000000088ac010" +
			"000b3c2069c496bc13228c154b03993809f278233d1"),
	}}

	for _, test := range tests {
		// Ensure the journal entry serializes to the expected value.
		gotBytes, err := serializeSpendJournalEntry(test.entry)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", test.name, err)
			continue
		}
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("%q: mismatched bytes - got %x, want %x", test.name, gotBytes,
				test.serialized)
			continue
		}

		// Deserialize to a spend journal entry.
		gotEntry, err := deserializeSpendJournalEntry(test.serialized,
			test.blockTxns)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", test.name, err)
			continue
		}

		// Ensure that the deserialized spend journal entry has the
		// correct properties.
		for j := range gotEntry {
			if !reflect.DeepEqual(gotEntry[j], test.entry[j]) {
				t.Errorf("%q: mismatched entries in idx %v - got %v, want %v",
					test.name, j, gotEntry[j], test.entry[j])
				continue
			}
		}
	}
}

// TestSpendJournalErrors performs negative tests against deserializing spend
// journal entries to ensure error paths work as expected.
func TestSpendJournalErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		blockTxns  []*wire.MsgTx
		serialized []byte
		errType    error
	}{
		// Adapted from block 170 in main blockchain.
		{
			name: "Force assertion due to missing stxos",
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *mustParseHash("0437cd7f8525ceed2324359c2d0ba26006d92d856a9c20fa0241106ee5a597c9"),
						Index: 0,
					},
					SignatureScript: hexToBytes("47304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901"),
					Sequence:        0xffffffff,
				}},
				LockTime: 0,
			}},
			serialized: hexToBytes(""),
			errType:    AssertError(""),
		},
		{
			name: "Force deserialization error in stxos",
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *mustParseHash("0437cd7f8525ceed2324359c2d0ba26006d92d856a9c20fa0241106ee5a597c9"),
						Index: 0,
					},
					SignatureScript: hexToBytes("47304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901"),
					Sequence:        0xffffffff,
				}},
				LockTime: 0,
			}},
			serialized: hexToBytes("1301320511db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a"),
			errType:    errDeserialize(""),
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned and the returned
		// slice is nil.
		stxos, err := deserializeSpendJournalEntry(test.serialized,
			test.blockTxns)
		if !errors.Is(err, test.errType) {
			t.Errorf("%q: expected error type does not match - got %T, want %T",
				test.name, err, test.errType)
			continue
		}
		if stxos != nil {
			t.Errorf("%q: returned slice of spent transaction outputs is not nil",
				test.name)
			continue
		}
	}
}

// TestHeaderCommitmentSerialization ensures serializing and deserializing
// header commitment journal entries works as expected.
func TestHeaderCommitmentSerialization(t *testing.T) {
	t.Parallel()

	cmtOneHash := *mustParseHash("0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20")
	cmtTwoHash := *mustParseHash("02030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f2021")
	tests := []struct {
		name        string
		commitments []chainhash.Hash
		serialized  []byte
	}{{
		name:        "no commitments",
		commitments: nil,
		serialized:  nil,
	}, {
		name:        "one commitment",
		commitments: []chainhash.Hash{cmtOneHash},
		serialized: hexToBytes("01" +
			"201f1e1d1c1b1a191817161514131211100f0e0d0c0b0a090807060504030201"),
	}, {
		name:        "two commitmentments",
		commitments: []chainhash.Hash{cmtOneHash, cmtTwoHash},
		serialized: hexToBytes("02" +
			"201f1e1d1c1b1a191817161514131211100f0e0d0c0b0a090807060504030201" +
			"21201f1e1d1c1b1a191817161514131211100f0e0d0c0b0a0908070605040302"),
	}}

	for _, test := range tests {
		// Ensure the commitments serialize to the expected value.
		gotBytes := serializeHeaderCommitments(test.commitments)
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("%q: mismatched bytes - got %x, want %x", test.name,
				gotBytes, test.serialized)
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// commitments.
		commitments, err := deserializeHeaderCommitments(test.serialized)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", test.name, err)
			continue
		}
		if !reflect.DeepEqual(commitments, test.commitments) {
			t.Errorf("%q: mismatched commitments - got %v, want %v", test.name,
				commitments, test.commitments)
			continue
		}
	}
}

// TestHeaderCommitmentDeserializeErrors peforms negative tests against
// deserializing header commitment journal entries to ensure error paths work as
// expected.
func TestHeaderCommitmentDeserializeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serialized []byte
		err        error
	}{{
		name:       "short data in number of commitments",
		serialized: hexToBytes("80"),
		err:        database.ErrCorruption,
	}, {
		name: "short data in commitment hashes",
		serialized: hexToBytes("01" +
			"201f1e1d1c1b1a191817161514131211100f0e0d0c0b0a0908070605040302"),
		err: database.ErrCorruption,
	}, {
		name: "short data in commitment hashes 2 begin",
		serialized: hexToBytes("02" +
			"201f1e1d1c1b1a191817161514131211100f0e0d0c0b0a090807060504030201"),
		err: database.ErrCorruption,
	}, {
		name: "short data in commitment hashes 2 end",
		serialized: hexToBytes("02" +
			"201f1e1d1c1b1a191817161514131211100f0e0d0c0b0a090807060504030201" +
			"21201f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403"),
		err: database.ErrCorruption,
	}}

	for _, test := range tests {
		// Ensure the expected error type and code is returned.
		_, err := deserializeHeaderCommitments(test.serialized)
		if !errors.Is(err, test.err) {
			t.Errorf("%q: wrong error -- got: %v, want: %v", test.name, err,
				test.err)
			continue
		}
	}
}

// TestBestChainStateSerialization ensures serializing and deserializing the
// best chain state works as expected.
func TestBestChainStateSerialization(t *testing.T) {
	t.Parallel()

	var workSum uint256.Uint256
	tests := []struct {
		name       string
		state      bestChainState
		serialized []byte
	}{
		{
			name: "genesis",
			state: bestChainState{
				hash:         *mustParseHash("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"),
				height:       0,
				totalTxns:    1,
				totalSubsidy: 0,
				workSum: func() uint256.Uint256 {
					work := primitives.CalcWork(486604799)
					workSum.Add(&work)
					return workSum
				}(), // 0x0100010001
			},
			serialized: hexToBytes("6fe28c0ab6f1b372c1a6a246ae63f74f931e8365e15a089c68d61900000000000000000001000000000000000000000000000000050000000100010001"),
		},
		{
			name: "block 1",
			state: bestChainState{
				hash:         *mustParseHash("00000000839a8e6886ab5951d76f411475428afc90947ee320161bbf18eb6048"),
				height:       1,
				totalTxns:    2,
				totalSubsidy: 123456789,
				workSum: func() uint256.Uint256 {
					work := primitives.CalcWork(486604799)
					workSum.Add(&work)
					return workSum
				}(), // 0x0200020002,
			},
			serialized: hexToBytes("4860eb18bf1b1620e37e9490fc8a427514416fd75159ab86688e9a830000000001000000020000000000000015cd5b0700000000050000000200020002"),
		},
	}

	for _, test := range tests {
		// Ensure the state serializes to the expected value.
		gotBytes := serializeBestChainState(test.state)
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("%q: mismatched bytes - got %x, want %x", test.name, gotBytes,
				test.serialized)
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// state.
		state, err := deserializeBestChainState(test.serialized)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", test.name, err)
			continue
		}
		if !reflect.DeepEqual(state, test.state) {
			t.Errorf("%q: mismatched state - got %v, want %v", test.name, state,
				test.state)
			continue
		}
	}
}

// TestBestChainStateDeserializeErrors performs negative tests against
// deserializing the chain state to ensure error paths work as expected.
func TestBestChainStateDeserializeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serialized []byte
		err        error
	}{
		{
			name:       "nothing serialized",
			serialized: hexToBytes(""),
			err:        database.ErrCorruption,
		},
		{
			name:       "short data in hash",
			serialized: hexToBytes("0000"),
			err:        database.ErrCorruption,
		},
		{
			name:       "short data in work sum",
			serialized: hexToBytes("6fe28c0ab6f1b372c1a6a246ae63f74f931e8365e15a089c68d61900000000000000000001000000000000000500000001000100"),
			err:        database.ErrCorruption,
		},
	}

	for _, test := range tests {
		// Ensure the expected error type and code is returned.
		_, err := deserializeBestChainState(test.serialized)
		if !errors.Is(err, test.err) {
			t.Errorf("%q: wrong error code got: %v, want: %v", test.name, err,
				test.err)
			continue
		}
	}
}

// TestDbPutDeploymentVer ensures that putting and fetching the deployment
// version works as expected.
func TestDbPutDeploymentVer(t *testing.T) {
	t.Parallel()

	// Create a test database.
	db, err := createTestDatabase(t, "ffldb", wire.RegNet)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}

	tests := []struct {
		name    string
		version uint32
	}{{
		name:    "fetch the version when not yet stored in the database",
		version: 0,
	}, {
		name:    "put and fetch an initial version in the database",
		version: 1,
	}, {
		name:    "put and fetch an updated version in the database",
		version: 6,
	}, {
		name:    "put and fetch the max allowed version in the database",
		version: math.MaxUint32,
	}}

	for _, test := range tests {
		if test.version != 0 {
			// Update the deployment version.
			err = db.Update(func(dbTx database.Tx) error {
				return dbPutDeploymentVer(dbTx, test.version)
			})
			if err != nil {
				t.Fatalf("%q: error putting deployment version: %v", test.name,
					err)
			}
		}

		// Fetch the deployment version.
		var gotVersion uint32
		err = db.View(func(dbTx database.Tx) error {
			gotVersion = dbFetchDeploymentVer(dbTx)
			return nil
		})
		if err != nil {
			t.Fatalf("%q: error fetching deployment version: %v", test.name,
				err)
		}

		// Ensure that the fetched deployment version matches the expected
		// deployment version.
		if gotVersion != test.version {
			t.Errorf("%q: mismatched deployment version:\nwant: %+v\n got: "+
				"%v\n", test.name, test.version, gotVersion)
		}
	}
}

// TestNewDeploymentsStartTime ensures that the correct start time is returned
// for new deployments under a variety of conditions.
func TestNewDeploymentsStartTime(t *testing.T) {
	t.Parallel()

	// Default parameters to use for tests.  Clone the parameters so they can be
	// mutated.
	params := cloneParams(chaincfg.RegNetParams())
	defaultDeployments := map[uint32][]chaincfg.ConsensusDeployment{
		7: {{
			Vote:       mockVote1(),
			StartTime:  1596240000,
			ExpireTime: 1627776000,
		}},
		8: {{
			Vote:       mockVote2(),
			StartTime:  1631750400,
			ExpireTime: 1694822400,
		}},
	}

	// Create a test database.
	db, err := createTestDatabase(t, "ffldb", params.Net)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}

	tests := []struct {
		name              string
		deployments       map[uint32][]chaincfg.ConsensusDeployment
		blockIndexEntries []blockIndexEntry
		prevVersion       uint32
		wantTime          uint64
	}{{
		name:     "no deployments defined",
		wantTime: 0,
	}, {
		name:        "current version == previous version",
		deployments: defaultDeployments,
		prevVersion: 0,
		wantTime:    1596240000,
	}, {
		name:        "current version > previous version",
		deployments: defaultDeployments,
		prevVersion: 7,
		wantTime:    1631750400,
	}, {
		name:        "current version < previous version",
		deployments: defaultDeployments,
		prevVersion: 9,
		wantTime:    0,
	}}

	for _, test := range tests {
		// Set deployments based on the test parameter.
		params.Deployments = test.deployments

		// Set the previous version based on the test parameter.
		if test.prevVersion != 0 {
			err = db.Update(func(dbTx database.Tx) error {
				return dbPutDeploymentVer(dbTx, test.prevVersion)
			})
			if err != nil {
				t.Fatalf("%q: error putting deployment version: %v", test.name,
					err)
			}
		}

		// Get the new deployments start time.
		var gotTime uint64
		err = db.View(func(dbTx database.Tx) error {
			gotTime = newDeploymentsStartTime(dbTx, params)
			return nil
		})
		if err != nil {
			t.Fatalf("%q: error getting new deployments start time: %v",
				test.name, err)
		}

		// Ensure that the fetched new deployments start time matches the
		// expected time.
		if gotTime != test.wantTime {
			t.Errorf("%q: mismatched new deployments start time:\nwant: %v\n "+
				"got: %v\n", test.name, test.wantTime, gotTime)
		}
	}
}

// TestUpdateDeploymentVersion ensures that updating the deployment version
// works as expected under a variety of conditions.
func TestUpdateDeploymentVersion(t *testing.T) {
	t.Parallel()

	// Default parameters to use for tests.  Clone the parameters so they can be
	// mutated.
	params := cloneParams(chaincfg.RegNetParams())
	defaultDeployments := map[uint32][]chaincfg.ConsensusDeployment{
		7: {{
			Vote: mockVote1(),
		}},
		8: {{
			Vote: mockVote2(),
		}},
	}

	// Create a test database.
	db, err := createTestDatabase(t, "ffldb", params.Net)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}

	tests := []struct {
		name              string
		deployments       map[uint32][]chaincfg.ConsensusDeployment
		blockIndexEntries []blockIndexEntry
		prevVersion       uint32
		wantVersion       uint32
	}{{
		name:        "no deployments defined",
		wantVersion: 0,
	}, {
		name:        "current version == previous version",
		deployments: defaultDeployments,
		prevVersion: 8,
		wantVersion: 8,
	}, {
		name:        "current version > previous version",
		deployments: defaultDeployments,
		prevVersion: 7,
		wantVersion: 8,
	}, {
		name:        "current version < previous version",
		deployments: defaultDeployments,
		prevVersion: 9,
		wantVersion: 8,
	}}

	for _, test := range tests {
		// Set deployments based on the test parameter.
		params.Deployments = test.deployments

		// Set the previous version based on the test parameter.
		if test.prevVersion != 0 {
			err = db.Update(func(dbTx database.Tx) error {
				return dbPutDeploymentVer(dbTx, test.prevVersion)
			})
			if err != nil {
				t.Fatalf("%q: error putting deployment version: %v", test.name,
					err)
			}
		}

		// Update the deployment version.
		err = db.Update(func(dbTx database.Tx) error {
			return updateDeploymentVersion(dbTx, params)
		})
		if err != nil {
			t.Fatalf("%q: error updating deployment version: %v", test.name,
				err)
		}

		// Fetch the updated deployment version.
		var gotVersion uint32
		err = db.View(func(dbTx database.Tx) error {
			gotVersion = dbFetchDeploymentVer(dbTx)
			return nil
		})
		if err != nil {
			t.Fatalf("%q: error fetching deployment version: %v", test.name,
				err)
		}

		// Ensure that the fetched deployment version matches the expected
		// deployment version.
		if gotVersion != test.wantVersion {
			t.Errorf("%q: mismatched deployment version:\nwant: %v\n got: %v\n",
				test.name, test.wantVersion, gotVersion)
		}
	}
}
