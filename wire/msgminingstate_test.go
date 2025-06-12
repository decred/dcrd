// Copyright (c) 2019-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

func TestMiningState(t *testing.T) {
	pver := ProtocolVersion

	// Ensure the command is expected value.
	wantCmd := "miningstate"
	msg := NewMsgMiningState()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgMemPool: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value.
	wantPayload := uint32(1546)
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

	// Test encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver)
	if err != nil {
		t.Errorf("encode of MsgMiningState failed %v err <%v>", msg, err)
	}

	// Test decode with latest protocol version.
	readmsg := NewMsgMiningState()
	err = readmsg.BtcDecode(&buf, pver)
	if err != nil {
		t.Errorf("decode of MsgMiningState failed [%v] err <%v>", buf, err)
	}
}

// TestMiningStateWire tests the MsgMiningState wire encode and decode for a sample
// message containing a fake block header and some fake vote hashes.
func TestMiningStateWire(t *testing.T) {
	// Empty tx message.
	sampleMSMsg := NewMsgMiningState()
	sampleMSMsg.Version = 1
	sampleMSMsg.Height = 123456

	fakeBlock, _ := chainhash.NewHashFromStr("4433221144332211443322114" +
		"433221144332211443322114433221144332211")
	err := sampleMSMsg.AddBlockHash(fakeBlock)
	if err != nil {
		t.Errorf("unexpected error for AddBlockHash: %v", err)
	}

	fakeVote1, _ := chainhash.NewHashFromStr("2222111122221111222211112" +
		"222111122221111222211112222111122221111")
	fakeVote2, _ := chainhash.NewHashFromStr("4444333344443333444433334" +
		"444333344443333444433334444333344443333")
	fakeVote3, _ := chainhash.NewHashFromStr("6666555566665555666655556" +
		"666555566665555666655556666555566665555")
	err = sampleMSMsg.AddVoteHash(fakeVote1)
	if err != nil {
		t.Errorf("unexpected error for AddVoteHash 1: %v", err)
	}
	err = sampleMSMsg.AddVoteHash(fakeVote2)
	if err != nil {
		t.Errorf("unexpected error for AddVoteHash 2: %v", err)
	}
	err = sampleMSMsg.AddVoteHash(fakeVote3)
	if err != nil {
		t.Errorf("unexpected error for AddVoteHash 3: %v", err)
	}

	sampleMSMsgEncoded := []byte{
		0x01, 0x00, 0x00, 0x00, // Version
		0x40, 0xe2, 0x01, 0x00, // Height 0001e240 in BE
		0x01,
		0x11, 0x22, 0x33, 0x44, 0x11, 0x22, 0x33, 0x44, // Dummy Block
		0x11, 0x22, 0x33, 0x44, 0x11, 0x22, 0x33, 0x44,
		0x11, 0x22, 0x33, 0x44, 0x11, 0x22, 0x33, 0x44,
		0x11, 0x22, 0x33, 0x44, 0x11, 0x22, 0x33, 0x44,
		0x03,                                           // Varint for number of votes
		0x11, 0x11, 0x22, 0x22, 0x11, 0x11, 0x22, 0x22, // Dummy votes [1]
		0x11, 0x11, 0x22, 0x22, 0x11, 0x11, 0x22, 0x22,
		0x11, 0x11, 0x22, 0x22, 0x11, 0x11, 0x22, 0x22,
		0x11, 0x11, 0x22, 0x22, 0x11, 0x11, 0x22, 0x22,
		0x33, 0x33, 0x44, 0x44, 0x33, 0x33, 0x44, 0x44, // [2]
		0x33, 0x33, 0x44, 0x44, 0x33, 0x33, 0x44, 0x44,
		0x33, 0x33, 0x44, 0x44, 0x33, 0x33, 0x44, 0x44,
		0x33, 0x33, 0x44, 0x44, 0x33, 0x33, 0x44, 0x44,
		0x55, 0x55, 0x66, 0x66, 0x55, 0x55, 0x66, 0x66, // [3]
		0x55, 0x55, 0x66, 0x66, 0x55, 0x55, 0x66, 0x66,
		0x55, 0x55, 0x66, 0x66, 0x55, 0x55, 0x66, 0x66,
		0x55, 0x55, 0x66, 0x66, 0x55, 0x55, 0x66, 0x66,
	}

	tests := []struct {
		in   *MsgMiningState // Message to encode
		out  *MsgMiningState // Expected decoded message
		buf  []byte          // Wire encoding
		pver uint32          // Protocol version for wire encoding
	}{
		// Version 1 sample message with the latest protocol version.
		{
			sampleMSMsg,
			sampleMSMsg,
			sampleMSMsgEncoded,
			ProtocolVersion,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode the message to wire format.
		var buf bytes.Buffer
		err := test.in.BtcEncode(&buf, test.pver)
		if err != nil {
			t.Errorf("BtcEncode #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("BtcEncode #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Decode the message from wire format.
		var msg MsgMiningState
		rbuf := bytes.NewReader(test.buf)
		err = msg.BtcDecode(rbuf, test.pver)
		if err != nil {
			t.Errorf("BtcDecode #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&msg, test.out) {
			t.Errorf("BtcDecode #%d\n got: %s want: %s", i,
				spew.Sdump(&msg), spew.Sdump(test.out))
			continue
		}
	}
}
