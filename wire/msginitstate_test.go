// Copyright (c) 2020-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/chaincfg/chainhash"
)

// TestInitState tests the MsgInitState API.
func TestInitState(t *testing.T) {
	pver := ProtocolVersion

	// Ensure Command() returns the expected value.
	wantCmd := "initstate"
	msg := NewMsgInitState()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("MsgInitMsg: wrong command - got %v, want %v",
			cmd, wantCmd)
	}

	// Ensure max payload returns the expected value for latest protocol
	// version. a var int and n * hashes for each of block, vote and tspend
	// hashes.
	wantPayload := uint32((1 + 32*8) + (1 + 32*40) + (1 + 32*7))
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Ensure max payload length is not more than MaxMessagePayload.
	if maxPayload > MaxMessagePayload {
		t.Fatalf("MaxPayloadLength: payload length (%v) for protocol "+
			"version %d exceeds MaxMessagePayload (%v).", maxPayload, pver,
			MaxMessagePayload)
	}

	var hash chainhash.Hash

	// Ensure AddBlockHash adds up to the maximum allowed.
	for i := 0; i < MaxISBlocksAtHeadPerMsg; i++ {
		if err := msg.AddBlockHash(&hash); err != nil {
			t.Fatalf("AddBlockHash: unable to add max number of entries: %v", err)
		}
	}

	// Ensure AddBlockHash does _not_ add one more than the maximum
	// allowed.
	if err := msg.AddBlockHash(&hash); !errors.Is(err, ErrTooManyHeaders) {
		t.Fatalf("AddBlockHash: unexpected error - got=%v, want=%v",
			err, ErrTooManyHeaders)
	}

	// Ensure AddVoteHash adds up to the maximum allowed.
	for i := 0; i < MaxISVotesAtHeadPerMsg; i++ {
		if err := msg.AddVoteHash(&hash); err != nil {
			t.Fatalf("AddVoteHash: unable to add max number of entries: %v", err)
		}
	}

	// Ensure AddVoteHash does _not_ add one more than the maximum allowed.
	if err := msg.AddVoteHash(&hash); !errors.Is(err, ErrTooManyVotes) {
		t.Fatalf("AddVoteHash: unexpected error - got=%v, want=%v",
			err, ErrTooManyVotes)
	}

	// Ensure AddTSpendHash adds up to the maximum allowed.
	for i := 0; i < MaxISTSpendsAtHeadPerMsg; i++ {
		if err := msg.AddTSpendHash(&hash); err != nil {
			t.Fatalf("AddTSpendHash: unable to add max number of entries: %v", err)
		}
	}

	// Ensure AddTSpendHash does _not_ add one more than the maximum
	// allowed.
	if err := msg.AddTSpendHash(&hash); !errors.Is(err, ErrTooManyTSpends) {
		t.Fatalf("AddTSpendHash: unexpected error - got=%v, want=%v",
			err, ErrTooManyTSpends)
	}

	// Ensure NewMsgInitStateFilled returns errors if more than the allowed
	// for each type of response data is provided.
	var maxHashes [41]chainhash.Hash
	_, err := NewMsgInitStateFilled(maxHashes[:MaxISBlocksAtHeadPerMsg+1], nil, nil)
	if !errors.Is(err, ErrTooManyBlocks) {
		t.Fatalf("NewMsgInitStateFilled: unexpected error - got=%v, want=%v",
			err, ErrTooManyBlocks)
	}
	_, err = NewMsgInitStateFilled(nil, maxHashes[:MaxISVotesAtHeadPerMsg+1], nil)
	if !errors.Is(err, ErrTooManyVotes) {
		t.Fatalf("NewMsgInitStateFilled: unexpected error - got=%v, want=%v",
			err, ErrTooManyVotes)
	}
	_, err = NewMsgInitStateFilled(nil, nil, maxHashes[:MaxISTSpendsAtHeadPerMsg+1])
	if !errors.Is(err, ErrTooManyTSpends) {
		t.Fatalf("NewMsgInitStateFilled: unexpected error - got=%v, want=%v",
			err, ErrTooManyTSpends)
	}
}

// TestInitStateWire tests the MsgInitState wire encode and decode for various
// numbers of responses.
func TestInitStateWire(t *testing.T) {
	pver := ProtocolVersion

	// MsgInitState message with no response data.
	emptyMsg := NewMsgInitState()
	emptyMsgEncoded := []byte{
		0x00, // Varint for number of blocks
		0x00, // Varint for number of votes
		0x00, // Varint for number of tspends
	}

	fakeBlock1, _ := chainhash.NewHashFromStr("4433221144332211443322114" +
		"433221144332211443322114433221144332211")
	fakeBlock2, _ := chainhash.NewHashFromStr("9300109283044211443383938" +
		"912083481724918427109238198279148176817")
	fakeVote1, _ := chainhash.NewHashFromStr("2222111122221111222211112" +
		"222111122221111222211112222111122221111")
	fakeVote2, _ := chainhash.NewHashFromStr("4444333344443333444433334" +
		"444333344443333444433334444333344443333")
	fakeVote3, _ := chainhash.NewHashFromStr("6666555566665555666655556" +
		"666555566665555666655556666555566665555")
	fakeTSpend1, _ := chainhash.NewHashFromStr("88888888888888828282888" +
		"888888888888888888888888222888831888888")
	fakeTSpend2, _ := chainhash.NewHashFromStr("9999999999929929999999" +
		"999999999999999999991199999999999999919")
	fakeTSpend3, _ := chainhash.NewHashFromStr("aaaaaaaaaaaa9200aaaaaa" +
		"aaaaaaaaaaaaaaaaaaa9a9a9aaaaaaaaaaaaaaa")

	// MsgInitState message with multiple values for each hash.
	multiData := NewMsgInitState()
	multiData.AddBlockHash(fakeBlock1)
	multiData.AddBlockHash(fakeBlock2)
	multiData.AddVoteHash(fakeVote1)
	multiData.AddVoteHash(fakeVote2)
	multiData.AddVoteHash(fakeVote3)
	multiData.AddTSpendHash(fakeTSpend1)
	multiData.AddTSpendHash(fakeTSpend2)
	multiData.AddTSpendHash(fakeTSpend3)

	multiDataEncoded := []byte{
		0x02,                                           // Varint for number of block hashes
		0x11, 0x22, 0x33, 0x44, 0x11, 0x22, 0x33, 0x44, // Fake block 1
		0x11, 0x22, 0x33, 0x44, 0x11, 0x22, 0x33, 0x44,
		0x11, 0x22, 0x33, 0x44, 0x11, 0x22, 0x33, 0x44,
		0x11, 0x22, 0x33, 0x44, 0x11, 0x22, 0x33, 0x44,
		0x17, 0x68, 0x17, 0x48, 0x91, 0x27, 0x98, 0x81, // Fake block 2
		0x23, 0x09, 0x71, 0x42, 0x18, 0x49, 0x72, 0x81,
		0x34, 0x08, 0x12, 0x89, 0x93, 0x83, 0x33, 0x44,
		0x11, 0x42, 0x04, 0x83, 0x92, 0x10, 0x00, 0x93,
		0x03,                                           // Varint for number of vote hashes
		0x11, 0x11, 0x22, 0x22, 0x11, 0x11, 0x22, 0x22, // Fake vote 1
		0x11, 0x11, 0x22, 0x22, 0x11, 0x11, 0x22, 0x22,
		0x11, 0x11, 0x22, 0x22, 0x11, 0x11, 0x22, 0x22,
		0x11, 0x11, 0x22, 0x22, 0x11, 0x11, 0x22, 0x22,
		0x33, 0x33, 0x44, 0x44, 0x33, 0x33, 0x44, 0x44, // Fake vote 2
		0x33, 0x33, 0x44, 0x44, 0x33, 0x33, 0x44, 0x44,
		0x33, 0x33, 0x44, 0x44, 0x33, 0x33, 0x44, 0x44,
		0x33, 0x33, 0x44, 0x44, 0x33, 0x33, 0x44, 0x44,
		0x55, 0x55, 0x66, 0x66, 0x55, 0x55, 0x66, 0x66, // Fake vote 3
		0x55, 0x55, 0x66, 0x66, 0x55, 0x55, 0x66, 0x66,
		0x55, 0x55, 0x66, 0x66, 0x55, 0x55, 0x66, 0x66,
		0x55, 0x55, 0x66, 0x66, 0x55, 0x55, 0x66, 0x66,
		0x03,                                           // Varint for number of tspend hashes
		0x88, 0x88, 0x88, 0x31, 0x88, 0x88, 0x22, 0x82, // Fake tspend 1
		0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88,
		0x88, 0x88, 0x88, 0x88, 0x88, 0x82, 0x82, 0x82,
		0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x00,
		0x19, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, // Fake tspend 2
		0x19, 0x91, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99,
		0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x92,
		0x29, 0x99, 0x99, 0x99, 0x99, 0x99, 0x09, 0x00,
		0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0x9a, // Fake tspend 3
		0x9a, 0x9a, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa,
		0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0x0a, 0x20,
		0xa9, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0x0a, 0x00,
	}

	tests := []struct {
		in   *MsgInitState // Message to encode
		out  *MsgInitState // Expected decoded message
		buf  []byte        // Wire encoding
		pver uint32        // Protocol version for wire encoding
	}{{
		in:   emptyMsg,
		out:  emptyMsg,
		buf:  emptyMsgEncoded,
		pver: pver,
	}, {
		in:   multiData,
		out:  multiData,
		buf:  multiDataEncoded,
		pver: pver,
	}}

	for i, test := range tests {
		// Encode the message to wire format.
		var buf bytes.Buffer
		err := test.in.BtcEncode(&buf, test.pver)
		if err != nil {
			t.Errorf("BtcEncode #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("BtcEncode #%d - got %s, want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Decode the message from wire format.
		var msg MsgInitState
		rbuf := bytes.NewReader(test.buf)
		err = msg.BtcDecode(rbuf, test.pver)
		if err != nil {
			t.Errorf("BtcDecode #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&msg, test.out) {
			t.Errorf("BtcDecode #%d - got: %s want: %s", i,
				spew.Sdump(&msg), spew.Sdump(test.out))
			continue
		}
	}
}

// TestInitStateWireErrors performs negative tests against wire encode and decode of
// MsgInitState to confirm error paths work correctly.
func TestInitStateWireErrors(t *testing.T) {
	pver := ProtocolVersion

	fakeBlock1, _ := chainhash.NewHashFromStr("4433221144332211443322114" +
		"433221144332211443322114433221144332211")
	fakeBlock2, _ := chainhash.NewHashFromStr("9300109283044211443383938" +
		"912083481724918427109238198279148176817")
	fakeVote1, _ := chainhash.NewHashFromStr("2222111122221111222211112" +
		"222111122221111222211112222111122221111")
	fakeVote2, _ := chainhash.NewHashFromStr("4444333344443333444433334" +
		"444333344443333444433334444333344443333")
	fakeVote3, _ := chainhash.NewHashFromStr("6666555566665555666655556" +
		"666555566665555666655556666555566665555")
	fakeTSpend1, _ := chainhash.NewHashFromStr("88888888888888828282888" +
		"888888888888888888888888222888831888888")
	fakeTSpend2, _ := chainhash.NewHashFromStr("9999999999929929999999" +
		"999999999999999999991199999999999999919")
	fakeTSpend3, _ := chainhash.NewHashFromStr("aaaaaaaaaaaa9200aaaaaa" +
		"aaaaaaaaaaaaaaaaaaa9a9a9aaaaaaaaaaaaaaa")

	// MsgInitState message with multiple values for each hash.
	baseMsg := NewMsgInitState()
	baseMsg.AddBlockHash(fakeBlock1)
	baseMsg.AddBlockHash(fakeBlock2)
	baseMsg.AddVoteHash(fakeVote1)
	baseMsg.AddVoteHash(fakeVote2)
	baseMsg.AddVoteHash(fakeVote3)
	baseMsg.AddTSpendHash(fakeTSpend1)
	baseMsg.AddTSpendHash(fakeTSpend2)
	baseMsg.AddTSpendHash(fakeTSpend3)

	baseMsgEncoded := []byte{
		0x02,                                           // Varint for number of block hashes
		0x11, 0x22, 0x33, 0x44, 0x11, 0x22, 0x33, 0x44, // Fake block 1
		0x11, 0x22, 0x33, 0x44, 0x11, 0x22, 0x33, 0x44,
		0x11, 0x22, 0x33, 0x44, 0x11, 0x22, 0x33, 0x44,
		0x11, 0x22, 0x33, 0x44, 0x11, 0x22, 0x33, 0x44,
		0x17, 0x68, 0x17, 0x48, 0x91, 0x27, 0x98, 0x81, // Fake block 2
		0x23, 0x09, 0x71, 0x42, 0x18, 0x49, 0x72, 0x81,
		0x34, 0x08, 0x12, 0x89, 0x93, 0x83, 0x33, 0x44,
		0x11, 0x42, 0x04, 0x83, 0x92, 0x10, 0x00, 0x93,
		0x03,                                           // Varint for number of vote hashes
		0x11, 0x11, 0x22, 0x22, 0x11, 0x11, 0x22, 0x22, // Fake vote 1
		0x11, 0x11, 0x22, 0x22, 0x11, 0x11, 0x22, 0x22,
		0x11, 0x11, 0x22, 0x22, 0x11, 0x11, 0x22, 0x22,
		0x11, 0x11, 0x22, 0x22, 0x11, 0x11, 0x22, 0x22,
		0x33, 0x33, 0x44, 0x44, 0x33, 0x33, 0x44, 0x44, // Fake vote 2
		0x33, 0x33, 0x44, 0x44, 0x33, 0x33, 0x44, 0x44,
		0x33, 0x33, 0x44, 0x44, 0x33, 0x33, 0x44, 0x44,
		0x33, 0x33, 0x44, 0x44, 0x33, 0x33, 0x44, 0x44,
		0x55, 0x55, 0x66, 0x66, 0x55, 0x55, 0x66, 0x66, // Fake vote 3
		0x55, 0x55, 0x66, 0x66, 0x55, 0x55, 0x66, 0x66,
		0x55, 0x55, 0x66, 0x66, 0x55, 0x55, 0x66, 0x66,
		0x55, 0x55, 0x66, 0x66, 0x55, 0x55, 0x66, 0x66,
		0x03,                                           // Varint for number of tspend hashes
		0x88, 0x88, 0x88, 0x31, 0x88, 0x88, 0x22, 0x82, // Fake tspend 1
		0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88,
		0x88, 0x88, 0x88, 0x88, 0x88, 0x82, 0x82, 0x82,
		0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x00,
		0x19, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, // Fake tspend 2
		0x19, 0x91, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99,
		0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x92,
		0x29, 0x99, 0x99, 0x99, 0x99, 0x99, 0x09, 0x00,
		0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0x9a, // Fake tspend 3
		0x9a, 0x9a, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa,
		0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0x0a, 0x20,
		0xa9, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0x0a, 0x00,
	}

	// Message that forces an error by having more than the max allowed
	// number of block hashes.
	maxBlockHashes := NewMsgInitState()
	maxBlockHashes.BlockHashes = make([]chainhash.Hash, MaxISBlocksAtHeadPerMsg+1)
	maxBlockHashesEncoded := []byte{
		0xfc, // Varint for number of block hashes
	}

	// Message that forces an error by having more than the max allowed
	// number of vote hashes.
	maxVoteHashes := NewMsgInitState()
	maxVoteHashes.VoteHashes = make([]chainhash.Hash, MaxISVotesAtHeadPerMsg+1)
	maxVoteHashesEncoded := []byte{
		0x00, // Varint for number of block hashes
		0xfc, // Varint for number of vote hashes
	}

	// Message that forces an error by having more than the max allowed
	// number of tspend hashes.
	maxTSpendHashes := NewMsgInitState()
	maxTSpendHashes.TSpendHashes = make([]chainhash.Hash, MaxISTSpendsAtHeadPerMsg+1)
	maxTSpendHashesEncoded := []byte{
		0x00, // Varint for number of block hashes
		0x00, // Varint for number of vote hashes
		0xfc, // Varint for number of tspend hashes
	}

	tests := []struct {
		in       *MsgInitState // Value to encode
		buf      []byte        // Wire encoding
		pver     uint32        // Protocol version for wire encoding
		max      int           // Max size of fixed buffer to induce errors
		writeErr error         // Expected write error
		readErr  error         // Expected read error
	}{
		// Force error in number of blockHashes varint.
		{baseMsg, baseMsgEncoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in first block hash.
		{baseMsg, baseMsgEncoded, pver, 1, io.ErrShortWrite, io.EOF},
		// Force error in number of vote hashes varint.
		{baseMsg, baseMsgEncoded, pver, 1 + 32*2, io.ErrShortWrite, io.EOF},
		// Force error in first vote.
		{baseMsg, baseMsgEncoded, pver, 1 + 32*2 + 1, io.ErrShortWrite, io.EOF},
		// Force error in number of tspend hashes varint.
		{baseMsg, baseMsgEncoded, pver, 1 + 32*2 + 1 + 32*3, io.ErrShortWrite, io.EOF},
		// Force error in first tspend.
		{baseMsg, baseMsgEncoded, pver, 1 + 32*2 + 1 + 32*3 + 1, io.ErrShortWrite, io.EOF},
		// Force error with greater than allowed number of block
		// hashes.
		{maxBlockHashes, maxBlockHashesEncoded, pver, 2, ErrTooManyBlocks, ErrTooManyBlocks},
		// Force error with greater than allowed number of vote hashes.
		{maxVoteHashes, maxVoteHashesEncoded, pver, 2, ErrTooManyVotes, ErrTooManyVotes},
		// Force error with greater than allowed number of tspend
		// hashes.
		{maxTSpendHashes, maxTSpendHashesEncoded, pver, 3, ErrTooManyTSpends, ErrTooManyTSpends},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		w := newFixedWriter(test.max)
		err := test.in.BtcEncode(w, test.pver)
		if !errors.Is(err, test.writeErr) {
			t.Errorf("BtcEncode #%d wrong error - got: %v, want: %v", i, err,
				test.writeErr)
			continue
		}

		// Decode from wire format.
		var msg MsgInitState
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if !errors.Is(err, test.readErr) {
			t.Errorf("BtcDecode #%d wrong error - got: %v, want: %v", i, err,
				test.readErr)
			continue
		}
	}
}
