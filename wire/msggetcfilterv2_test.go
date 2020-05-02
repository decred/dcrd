// Copyright (c) 2019-2020 The Decred developers
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

// baseMsgGetCFilterV2 returns a MsgGetCFilterV2 struct populated with mock
// values that are used throughout tests.  Note that the tests will need to be
// updated if these values are changed since they rely on the current values.
func baseMsgGetCFilterV2(t *testing.T) *MsgGetCFilterV2 {
	t.Helper()

	// Mock block hash.
	hashStr := "000000000000c41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba"
	blockHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	return NewMsgGetCFilterV2(blockHash)
}

// TestGetCFilterV2 tests the MsgGetCFilterV2 API against the latest protocol
// version.
func TestGetCFilterV2(t *testing.T) {
	pver := ProtocolVersion

	// Ensure the command is expected value.
	wantCmd := "getcfilterv2"
	msg := baseMsgGetCFilterV2(t)
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgGetCFilterV2: wrong command - got %v want %v", cmd,
			wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Block hash + max commitment name length (including varint).
	wantPayload := uint32(32)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for protocol "+
			"version %d - got %v, want %v", pver, maxPayload, wantPayload)
	}

	// Ensure max payload length is not more than MaxMessagePayload.
	if maxPayload > MaxMessagePayload {
		t.Fatalf("MaxPayloadLength: payload length (%v) for protocol version "+
			"%d exceeds MaxMessagePayload (%v).", maxPayload, pver,
			MaxMessagePayload)
	}
}

// TestGetCFilterV2PreviousProtocol tests the MsgGetCFilterV2 API against the
// protocol prior to version CFilterV2Version.
func TestGetCFilterV2PreviousProtocol(t *testing.T) {
	// Use the protocol version just prior to CFilterV2Version changes.
	pver := CFilterV2Version - 1

	msg := baseMsgGetCFilterV2(t)

	// Test encode with old protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver)
	if err == nil {
		t.Errorf("encode of NewMsgGetCFilterV2 succeeded when it should have " +
			"failed")
	}

	// Test decode with old protocol version.
	var readmsg MsgGetCFilterV2
	err = readmsg.BtcDecode(&buf, pver)
	if err == nil {
		t.Errorf("decode of NewMsgGetCFilterV2 succeeded when it should have " +
			"failed")
	}
}

// TestGetCFilterV2CrossProtocol tests the MsgGetCFilterV2 API when encoding
// with the latest protocol version and decoding with CFilterV2Version.
func TestGetCFilterV2CrossProtocol(t *testing.T) {
	msg := baseMsgGetCFilterV2(t)

	// Encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, ProtocolVersion)
	if err != nil {
		t.Errorf("encode of MsgGetCFilterV2 failed %v err <%v>", msg, err)
	}

	// Decode with old protocol version.
	var readmsg MsgGetCFilterV2
	err = readmsg.BtcDecode(&buf, CFilterV2Version)
	if err != nil {
		t.Errorf("decode of MsgGetCFilterV2 failed [%v] err <%v>", buf, err)
	}
}

// TestGetCFilterV2Wire tests the MsgGetCFilterV2 wire encode and decode for
// various commitment names and protocol versions.
func TestGetCFilterV2Wire(t *testing.T) {
	// MsgGetCFilterV2 message with mock block hash.
	msgGetCFilterV2 := baseMsgGetCFilterV2(t)
	msgGetCFilterV2Encoded := []byte{
		0xba, 0xdc, 0xb8, 0xe5, 0xc1, 0xe8, 0x95, 0xe8,
		0xe8, 0xfe, 0xf8, 0xd3, 0x42, 0x5f, 0xa0, 0xbf,
		0xe9, 0xd2, 0x8f, 0xdb, 0xf7, 0x2f, 0x87, 0x19,
		0x10, 0xc4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Mock block hash
	}

	tests := []struct {
		in   *MsgGetCFilterV2 // Message to encode
		out  *MsgGetCFilterV2 // Expected decoded message
		buf  []byte           // Wire encoding
		pver uint32           // Protocol version for wire encoding
	}{{
		// Latest protocol version.
		msgGetCFilterV2,
		msgGetCFilterV2,
		msgGetCFilterV2Encoded,
		ProtocolVersion,
	}, {
		// Protocol version CFilterV2Version+1.
		msgGetCFilterV2,
		msgGetCFilterV2,
		msgGetCFilterV2Encoded,
		CFilterV2Version + 1,
	}, {
		// Protocol version CFilterV2Version.
		msgGetCFilterV2,
		msgGetCFilterV2,
		msgGetCFilterV2Encoded,
		CFilterV2Version,
	}}

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
		var msg MsgGetCFilterV2
		rbuf := bytes.NewReader(test.buf)
		err = msg.BtcDecode(rbuf, test.pver)
		if err != nil {
			t.Errorf("BtcDecode #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&msg, test.out) {
			t.Errorf("BtcDecode #%d\n got: %s want: %s", i, spew.Sdump(&msg),
				spew.Sdump(test.out))
			continue
		}
	}
}

// TestGetCFilterV2WireErrors performs negative tests against wire encode and
// decode of MsgGetCFilterV2 to confirm error paths work correctly.
func TestGetCFilterV2WireErrors(t *testing.T) {
	pver := ProtocolVersion

	// MsgGetCFilterV2 message with mock block hash.
	baseGetCFilterV2 := baseMsgGetCFilterV2(t)
	baseGetCFilterV2Encoded := []byte{
		0xba, 0xdc, 0xb8, 0xe5, 0xc1, 0xe8, 0x95, 0xe8,
		0xe8, 0xfe, 0xf8, 0xd3, 0x42, 0x5f, 0xa0, 0xbf,
		0xe9, 0xd2, 0x8f, 0xdb, 0xf7, 0x2f, 0x87, 0x19,
		0x10, 0xc4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Mock block hash
	}

	tests := []struct {
		in       *MsgGetCFilterV2 // Value to encode
		buf      []byte           // Wire encoding
		pver     uint32           // Protocol version for wire encoding
		max      int              // Max size of fixed buffer to induce errors
		writeErr error            // Expected write error
		readErr  error            // Expected read error
	}{
		// Force error in start of block hash.
		{baseGetCFilterV2, baseGetCFilterV2Encoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in middle of block hash.
		{baseGetCFilterV2, baseGetCFilterV2Encoded, pver, 8, io.ErrShortWrite, io.ErrUnexpectedEOF},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		w := newFixedWriter(test.max)
		err := test.in.BtcEncode(w, test.pver)
		if !errors.Is(err, test.writeErr) {
			t.Errorf("BtcEncode #%d wrong error got: %v, want: %v", i, err,
				test.writeErr)
			continue
		}

		// Decode from wire format.
		var msg MsgGetCFilterV2
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if !errors.Is(err, test.readErr) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v", i, err,
				test.readErr)
			continue
		}
	}
}
