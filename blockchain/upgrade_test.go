// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// TestBlockIndexDecodeErrorsV2 performs negative tests against decoding block
// index entries from the legacy version 2 format to ensure error paths work as
// expected.
func TestBlockIndexDecodeErrorsV2(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		entry      blockIndexEntryV2
		serialized []byte
		bytesRead  int // Expected number of bytes read.
		errType    error
	}{{
		name:       "nothing serialized",
		entry:      blockIndexEntryV2{},
		serialized: hexToBytes(""),
		errType:    errDeserialize(""),
		bytesRead:  0,
	}, {
		name:  "no data after block header",
		entry: blockIndexEntryV2{},
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
		entry: blockIndexEntryV2{},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c00000000000000000000000000000000000000000400000003"),
		errType:   errDeserialize(""),
		bytesRead: 181,
	}, {
		name:  "no data after num votes with no votes",
		entry: blockIndexEntryV2{},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c0000000000000000000000000000000000000000040000000300"),
		errType:   errDeserialize(""),
		bytesRead: 182,
	}, {
		name:  "no data after num votes with votes",
		entry: blockIndexEntryV2{},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c0000000000000000000000000000000000000000040000000301"),
		errType:   errDeserialize(""),
		bytesRead: 182,
	}, {
		name:  "short data in vote ticket hash",
		entry: blockIndexEntryV2{},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c00000000000000000000000000000000000000000400000003012" +
			"e9b0f3f37c74ba8e8a9211d38e9706106ec482a1422a880ea53475477a862"),
		errType:   errDeserialize(""),
		bytesRead: 182,
	}, {
		name:  "no data after vote ticket hash",
		entry: blockIndexEntryV2{},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c00000000000000000000000000000000000000000400000003012" +
			"e9b0f3f37c74ba8e8a9211d38e9706106ec482a1422a880ea53475477a8628b"),
		errType:   errDeserialize(""),
		bytesRead: 214,
	}, {
		name:  "no data after vote version",
		entry: blockIndexEntryV2{},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c00000000000000000000000000000000000000000400000003012" +
			"e9b0f3f37c74ba8e8a9211d38e9706106ec482a1422a880ea53475477a8628b04"),
		errType:   errDeserialize(""),
		bytesRead: 215,
	}, {
		name:  "no data after votes",
		entry: blockIndexEntryV2{},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c00000000000000000000000000000000000000000400000003012" +
			"e9b0f3f37c74ba8e8a9211d38e9706106ec482a1422a880ea53475477a8628b0" +
			"401"),
		errType:   errDeserialize(""),
		bytesRead: 216,
	}, {
		name:  "no data after num revokes with revokes",
		entry: blockIndexEntryV2{},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c000000000000000000000000000000000000000004000000030001"),
		errType:   errDeserialize(""),
		bytesRead: 183,
	}}

	for _, test := range tests {
		// Ensure the expected error type is returned.
		gotBytesRead, err := decodeBlockIndexEntryV2(test.serialized,
			&test.entry)
		if !errors.As(err, &test.errType) {
			t.Errorf("decodeBlockIndexEntry (%s): expected error "+
				"type does not match - got %T, want %T",
				test.name, err, test.errType)
			continue
		}

		// Ensure the expected number of bytes read is returned.
		if gotBytesRead != test.bytesRead {
			t.Errorf("decodeBlockIndexEntry (%s): unexpected "+
				"number of bytes read - got %d, want %d",
				test.name, gotBytesRead, test.bytesRead)
			continue
		}
	}
}

// TestBlockIndexSerializationV2 ensures serializing and deserializing block
// index entries from the legacy version 2 format works as expected.
func TestBlockIndexSerializationV2(t *testing.T) {
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
	baseTicketsVoted := []chainhash.Hash{
		*mustParseHash("8b62a877544753ea80a822142a48ec066170e9381d21a9e8a84bc7373f0f9b2e"),
		*mustParseHash("4427a003a7aceb1404ffd9072e9aff1e128a24333a543332030e91668a389db7"),
		*mustParseHash("4415b88ac74881d7b6b15d41df465257cd1cc92d55e95f1b648434aef3a2110b"),
		*mustParseHash("9d2621b57352088809d3a069b04b76c832f30a76da14e56aece72208b3e5b87a"),
	}
	baseTicketsRevoked := []chainhash.Hash{
		*mustParseHash("8146f01b8ffca8008ebc80293d2978d63b1dffa5c456a73e7b39a9b1e695e8eb"),
		*mustParseHash("2292ff2461e725c58cc6e2051eac2a10e6ee6d1f62327ed676b7a196fb94be0c"),
	}
	baseVoteInfo := []blockIndexVoteVersionTuple{
		{version: 4, bits: 0x0001},
		{version: 4, bits: 0x0015},
		{version: 4, bits: 0x0015},
		{version: 4, bits: 0x0001},
	}

	// Version 2 block status flags used in the tests.
	const (
		v2StatusDataStored = 1 << 0
		v2StatusValidated  = 1 << 1
	)

	tests := []struct {
		name       string
		entry      blockIndexEntryV2
		serialized []byte
	}{{
		name: "no votes, no revokes",
		entry: blockIndexEntryV2{
			header:         baseHeader,
			status:         v2StatusDataStored | v2StatusValidated,
			voteInfo:       nil,
			ticketsVoted:   nil,
			ticketsRevoked: nil,
		},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c000000000000000000000000000000000000000004000000030000"),
	}, {
		name: "1 vote, no revokes",
		entry: blockIndexEntryV2{
			header:         baseHeader,
			status:         v2StatusDataStored | v2StatusValidated,
			voteInfo:       baseVoteInfo[:1],
			ticketsVoted:   baseTicketsVoted[:1],
			ticketsRevoked: nil,
		},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c00000000000000000000000000000000000000000400000003012" +
			"e9b0f3f37c74ba8e8a9211d38e9706106ec482a1422a880ea53475477a8628b0" +
			"40100"),
	}, {
		name: "no votes, 1 revoke",
		entry: blockIndexEntryV2{
			header:         baseHeader,
			status:         v2StatusDataStored | v2StatusValidated,
			voteInfo:       nil,
			ticketsVoted:   nil,
			ticketsRevoked: baseTicketsRevoked[:1],
		},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c00000000000000000000000000000000000000000400000003000" +
			"1ebe895e6b1a9397b3ea756c4a5ff1d3bd678293d2980bc8e00a8fc8f1bf04681"),
	}, {
		name: "4 votes, same vote versions, different vote bits, 2 revokes",
		entry: blockIndexEntryV2{
			header:         baseHeader,
			status:         v2StatusDataStored | v2StatusValidated,
			voteInfo:       baseVoteInfo,
			ticketsVoted:   baseTicketsVoted,
			ticketsRevoked: baseTicketsRevoked,
		},
		serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3" +
			"425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470" +
			"d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f83" +
			"3a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f0" +
			"11aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0" +
			"a6b11ee3b3c00000000000000000000000000000000000000000400000003042" +
			"e9b0f3f37c74ba8e8a9211d38e9706106ec482a1422a880ea53475477a8628b0" +
			"401b79d388a66910e033233543a33248a121eff9a2e07d9ff0414ebaca703a02" +
			"74404150b11a2f3ae3484641b5fe9552dc91ccd575246df415db1b6d78148c78" +
			"ab8154404157ab8e5b30822e7ec6ae514da760af332c8764bb069a0d30988085" +
			"273b521269d040102ebe895e6b1a9397b3ea756c4a5ff1d3bd678293d2980bc8" +
			"e00a8fc8f1bf046810cbe94fb96a1b776d67e32621f6deee6102aac1e05e2c68" +
			"cc525e76124ff9222"),
	}}

	for _, test := range tests {
		// Ensure the function to calculate the serialized size without
		// actually serializing it is calculated properly.
		gotSize := blockIndexEntrySerializeSizeV2(&test.entry)
		if gotSize != len(test.serialized) {
			t.Errorf("%s: did not get expected size - got %d, want %d",
				test.name, gotSize, len(test.serialized))
		}

		// Ensure the block index entry serializes to the expected value.
		gotSerialized := make([]byte, blockIndexEntrySerializeSizeV2(&test.entry))
		_, err := putBlockIndexEntryV2(gotSerialized, &test.entry)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", test.name, err)
			continue
		}
		if !bytes.Equal(gotSerialized, test.serialized) {
			t.Errorf("%s: did not get expected bytes - got %x, want %x",
				test.name, gotSerialized, test.serialized)
			continue
		}

		// Ensure the block index entry serializes to the expected value
		// and produces the expected number of bytes written via a
		// direct put.
		gotSerialized2 := make([]byte, gotSize)
		gotBytesWritten, err := putBlockIndexEntryV2(gotSerialized2,
			&test.entry)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", test.name, err)
			continue
		}
		if !bytes.Equal(gotSerialized2, test.serialized) {
			t.Errorf("%s: did not get expected bytes - got %x, want %x",
				test.name, gotSerialized2, test.serialized)
			continue
		}
		if gotBytesWritten != len(test.serialized) {
			t.Errorf("%s: did not get expected number of bytes written - got "+
				"%d, want %d", test.name, gotBytesWritten, len(test.serialized))
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected block
		// index entry.
		var gotEntry blockIndexEntryV2
		bytesRead, err := decodeBlockIndexEntryV2(test.serialized, &gotEntry)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", test.name, err)
			continue
		}
		if !reflect.DeepEqual(gotEntry, test.entry) {
			t.Errorf("%s: mismatched entries\ngot %+v\nwant %+v", test.name,
				gotEntry, test.entry)
			continue
		}
		if bytesRead != len(test.serialized) {
			t.Errorf("%s: did not get expected number of bytes read - got %d, "+
				"want %d", test.name, bytesRead, len(test.serialized))
			continue
		}
	}
}

// TestBlockIndexSerializationV3 ensures serializing block index entries in the
// version 3 format works as expected.
func TestBlockIndexSerializationV3(t *testing.T) {
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
	baseVoteInfo := []blockIndexVoteVersionTuple{
		{version: 4, bits: 0x0001},
		{version: 4, bits: 0x0015},
		{version: 4, bits: 0x0015},
		{version: 4, bits: 0x0001},
	}

	// Version 3 block status flags used in the tests.
	const (
		v3StatusDataStored = 1 << 0
		v3StatusValidated  = 1 << 1
	)
	tests := []struct {
		name       string
		entry      blockIndexEntryV3
		serialized []byte
	}{{
		name: "no votes",
		entry: blockIndexEntryV3{
			header:   baseHeader,
			status:   v3StatusDataStored | v3StatusValidated,
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
		entry: blockIndexEntryV3{
			header:   baseHeader,
			status:   v3StatusDataStored | v3StatusValidated,
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
		entry: blockIndexEntryV3{
			header:   baseHeader,
			status:   v3StatusDataStored | v3StatusValidated,
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
		// Ensure the function to calculate the serialized size without actually
		// serializing it is calculated properly.
		gotSize := blockIndexEntrySerializeSizeV3(&test.entry)
		if gotSize != len(test.serialized) {
			t.Errorf("%q: did not get expected size - got %d, want %d",
				test.name, gotSize, len(test.serialized))
		}

		// Ensure the block index entry serializes to the expected value.
		gotSerialized, err := serializeBlockIndexEntryV3(&test.entry)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", test.name, err)
			continue
		}
		if !bytes.Equal(gotSerialized, test.serialized) {
			t.Errorf("%q: did not get expected bytes - got %x, want %x",
				test.name, gotSerialized, test.serialized)
			continue
		}

		// Ensure the block index entry serializes to the expected value and
		// produces the expected number of bytes written via a direct put.
		gotSerialized2 := make([]byte, gotSize)
		gotBytesWritten, err := putBlockIndexEntryV3(gotSerialized2,
			&test.entry)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", test.name, err)
			continue
		}
		if !bytes.Equal(gotSerialized2, test.serialized) {
			t.Errorf("%q: did not get expected bytes - got %x, want %x",
				test.name, gotSerialized2, test.serialized)
			continue
		}
		if gotBytesWritten != len(test.serialized) {
			t.Errorf("%q: did not get expected number of bytes written - got "+
				"%d, want %d", test.name, gotBytesWritten, len(test.serialized))
			continue
		}
	}
}
