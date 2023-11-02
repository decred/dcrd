// Copyright (c) 2024 The Decred developers
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

// baseMsgGetCFsV2 returns a MsgGetCFsV2 struct populated with mock
// values that are used throughout tests.  Note that the tests will need to be
// updated if these values are changed since they rely on the current values.
func baseMsgGetCFsV2(t *testing.T) *MsgGetCFsV2 {
	t.Helper()

	// Mock block hash.
	startHashStr := "000000000000c41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba"
	startHash, err := chainhash.NewHashFromStr(startHashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	endHashStr := "00000000000108ac3e3f51a0f4424dd757a3b0485da0ec96592f637f27bd1cf5"
	endHash, err := chainhash.NewHashFromStr(endHashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	return NewMsgGetCFsV2(startHash, endHash)
}

// TestGetCFiltersV2 tests the MsgGetCFsV2 API against the latest protocol
// version.
func TestGetCFiltersV2(t *testing.T) {
	pver := ProtocolVersion

	// Ensure the command is expected value.
	wantCmd := "getcfsv2"
	msg := baseMsgGetCFsV2(t)
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgGetCFsV2: wrong command - got %v want %v", cmd,
			wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Start hash + end hash.
	wantPayload := uint32(64)
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

// TestGetCFiltersV2PreviousProtocol tests the MsgGetCFsV2 API against the
// protocol prior to version BatchedCFiltersV2Version.
func TestGetCFiltersV2PreviousProtocol(t *testing.T) {
	// Use the protocol version just prior to CFilterV2Version changes.
	pver := BatchedCFiltersV2Version - 1

	msg := baseMsgGetCFsV2(t)

	// Test encode with old protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver)
	if !errors.Is(err, ErrMsgInvalidForPVer) {
		t.Errorf("unexpected error when encoding for protocol version %d, "+
			"prior to message introduction - got %v, want %v", pver,
			err, ErrMsgInvalidForPVer)
	}

	// Test decode with old protocol version.
	var readmsg MsgGetCFsV2
	err = readmsg.BtcDecode(&buf, pver)
	if !errors.Is(err, ErrMsgInvalidForPVer) {
		t.Errorf("unexpected error when decoding for protocol version %d, "+
			"prior to message introduction - got %v, want %v", pver,
			err, ErrMsgInvalidForPVer)
	}
}

// TestGetCFiltersV2CrossProtocol tests the MsgGetCFsV2 API when encoding
// with the latest protocol version and decoding with BatchedCFiltersV2Version.
func TestGetCFiltersV2CrossProtocol(t *testing.T) {
	msg := baseMsgGetCFsV2(t)

	// Encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, ProtocolVersion)
	if err != nil {
		t.Errorf("encode of MsgGetCFsV2 failed %v err <%v>", msg, err)
	}

	// Decode with old protocol version.
	var readmsg MsgGetCFilterV2
	err = readmsg.BtcDecode(&buf, BatchedCFiltersV2Version)
	if err != nil {
		t.Errorf("decode of MsgGetCFsV2 failed [%v] err <%v>", buf, err)
	}
}

// TestGetCFiltersV2Wire tests the MsgGetCFsV2 wire encode and decode for
// various commitment names and protocol versions.
func TestGetCFiltersV2Wire(t *testing.T) {
	// MsgGetCFsV2 message with mock block hashes.
	msgGetCFsV2 := baseMsgGetCFsV2(t)
	msgGetCFsV2Encoded := []byte{
		0xba, 0xdc, 0xb8, 0xe5, 0xc1, 0xe8, 0x95, 0xe8,
		0xe8, 0xfe, 0xf8, 0xd3, 0x42, 0x5f, 0xa0, 0xbf,
		0xe9, 0xd2, 0x8f, 0xdb, 0xf7, 0x2f, 0x87, 0x19,
		0x10, 0xc4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Mock start hash
		0xf5, 0x1c, 0xbd, 0x27, 0x7f, 0x63, 0x2f, 0x59,
		0x96, 0xec, 0xa0, 0x5d, 0x48, 0xb0, 0xa3, 0x57,
		0xd7, 0x4d, 0x42, 0xf4, 0xa0, 0x51, 0x3f, 0x3e,
		0xac, 0x08, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, // Mock end hash
	}

	tests := []struct {
		in   *MsgGetCFsV2 // Message to encode
		out  *MsgGetCFsV2 // Expected decoded message
		buf  []byte       // Wire encoding
		pver uint32       // Protocol version for wire encoding
	}{{
		// Latest protocol version.
		msgGetCFsV2,
		msgGetCFsV2,
		msgGetCFsV2Encoded,
		ProtocolVersion,
	}, {
		// Protocol version CFilterV2Version+1.
		msgGetCFsV2,
		msgGetCFsV2,
		msgGetCFsV2Encoded,
		BatchedCFiltersV2Version + 1,
	}, {
		// Protocol version CFilterV2Version.
		msgGetCFsV2,
		msgGetCFsV2,
		msgGetCFsV2Encoded,
		BatchedCFiltersV2Version,
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
		var msg MsgGetCFsV2
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

// TestGetCFiltersV2WireErrors performs negative tests against wire encode and
// decode of MsgGetCFsV2 to confirm error paths work correctly.
func TestGetCFiltersV2WireErrors(t *testing.T) {
	pver := ProtocolVersion

	// MsgGetCFilterV2 message with mock block hash.
	baseGetCFiltersV2 := baseMsgGetCFsV2(t)
	baseGetCFiltersV2Encoded := []byte{
		0xba, 0xdc, 0xb8, 0xe5, 0xc1, 0xe8, 0x95, 0xe8,
		0xe8, 0xfe, 0xf8, 0xd3, 0x42, 0x5f, 0xa0, 0xbf,
		0xe9, 0xd2, 0x8f, 0xdb, 0xf7, 0x2f, 0x87, 0x19,
		0x10, 0xc4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Mock block hash
		0xf5, 0x1c, 0xbd, 0x27, 0x7f, 0x63, 0x2f, 0x59,
		0x96, 0xec, 0xa0, 0x5d, 0x48, 0xb0, 0xa3, 0x57,
		0xd7, 0x4d, 0x42, 0xf4, 0xa0, 0x51, 0x3f, 0x3e,
		0xac, 0x08, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, // Mock end hash
	}

	tests := []struct {
		in       *MsgGetCFsV2 // Value to encode
		buf      []byte       // Wire encoding
		pver     uint32       // Protocol version for wire encoding
		max      int          // Max size of fixed buffer to induce errors
		writeErr error        // Expected write error
		readErr  error        // Expected read error
	}{
		// Force error in start of start hash.
		{baseGetCFiltersV2, baseGetCFiltersV2Encoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in middle of start hash.
		{baseGetCFiltersV2, baseGetCFiltersV2Encoded, pver, 8, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in start of end hash.
		{baseGetCFiltersV2, baseGetCFiltersV2Encoded, pver, 32, io.ErrShortWrite, io.EOF},
		// Force error in middle of end hash.
		{baseGetCFiltersV2, baseGetCFiltersV2Encoded, pver, 40, io.ErrShortWrite, io.ErrUnexpectedEOF},
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
		var msg MsgGetCFsV2
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if !errors.Is(err, test.readErr) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v", i, err,
				test.readErr)
			continue
		}
	}
}
