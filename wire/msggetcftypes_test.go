// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// TestGetCFTypes tests the MsgGetCFTypes API.
func TestGetCFTypes(t *testing.T) {
	pver := ProtocolVersion
	oldPver := NodeCFVersion - 1

	// Ensure the command is expected value.
	wantCmd := "getcftypes"
	msg := NewMsgGetCFTypes()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Fatalf("NewMsgGetCFTypes: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	wantPayload := uint32(0)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Fatalf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Ensure max payload length is not more than MaxMessagePayload.
	if maxPayload > MaxMessagePayload {
		t.Fatalf("MaxPayloadLength: payload length (%v) for protocol "+
			"version %d exceeds MaxMessagePayload (%v).", maxPayload, pver,
			MaxMessagePayload)
	}

	// Older protocol versions should fail encode since message didn't
	// exist yet.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, oldPver)
	if err == nil {
		t.Fatalf("encode of MsgGetCFTypes passed for old protocol "+
			"version %v err <%v>", msg, err)
	}

	// Older protocol versions should fail decode since message didn't
	// exist yet.
	err = msg.BtcDecode(&buf, oldPver)
	if err == nil {
		t.Fatalf("decode of MsgGetCFTypes passed for old protocol "+
			"version %v err <%v>", msg, err)
	}
}

// TestGetCFTypesWire tests the MsgGetCFTypes wire encode and decode for various
// protocol versions.
func TestGetCFTypesWire(t *testing.T) {
	msgGetCFTypes := NewMsgGetCFTypes()
	msgGetCFTypesEncoded := []byte{}

	tests := []struct {
		in   *MsgGetCFTypes // Message to encode
		out  *MsgGetCFTypes // Expected decoded message
		buf  []byte         // Wire encoding
		pver uint32         // Protocol version for wire encoding
	}{
		// Latest protocol version.
		{
			msgGetCFTypes,
			msgGetCFTypes,
			msgGetCFTypesEncoded,
			ProtocolVersion,
		},

		// First protocol version supported committed filters (CF).
		{
			msgGetCFTypes,
			msgGetCFTypes,
			msgGetCFTypesEncoded,
			NodeCFVersion,
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
		var msg MsgGetCFTypes
		rbuf := bytes.NewReader(test.buf)
		err = msg.BtcDecode(rbuf, test.pver)
		if err != nil {
			t.Errorf("BtcDecode #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&msg, test.out) {
			t.Errorf("BtcDecode #%d\n got: %s want: %s", i,
				spew.Sdump(msg), spew.Sdump(test.out))
			continue
		}
	}
}
