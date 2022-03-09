// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// TestGetAddrLatest tests the MsgGetAddr API against the latest protocol
// version to ensure it is no longer valid.
func TestGetAddrLatest(t *testing.T) {
	pver := ProtocolVersion
	msg := NewMsgGetAddr()

	// Ensure max payload is expected value.
	wantPayload := uint32(0)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for protocol "+
			"version %d - got %v, want %v", pver, maxPayload, wantPayload)
	}

	// Ensure encode fails with the latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver)
	if !errors.Is(err, ErrMsgInvalidForPVer) {
		t.Errorf("MsgGetAddr encode unexpected err -- got %v, want %v", err,
			ErrMsgInvalidForPVer)
	}

	// Ensure decode fails with the latest protocol version.
	var readMsg MsgGetAddr
	err = readMsg.BtcDecode(&buf, pver)
	if !errors.Is(err, ErrMsgInvalidForPVer) {
		t.Errorf("MsgGetAddr decode unexpected err -- got %v, want %v", err,
			ErrMsgInvalidForPVer)
	}
}

// TestGetAddr tests the MsgGetAddr API.
func TestGetAddr(t *testing.T) {
	pver := ProtocolVersion

	// Ensure the command is expected value.
	wantCmd := "getaddr"
	msg := NewMsgGetAddr()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgGetAddr: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Num addresses (varInt) + max allowed addresses.
	wantPayload := uint32(0)
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
}

// TestGetAddrWireLastSupported tests the MsgGetAddr wire encode and decode for
// the last supported protocol versions.
func TestGetAddrWireLastSupported(t *testing.T) {
	const lastSupportedProtocolVersion = AddrV2Version - 1
	msgGetAddr := NewMsgGetAddr()
	msgGetAddrEncoded := []byte{}

	tests := []struct {
		in   *MsgGetAddr // Message to encode
		out  *MsgGetAddr // Expected decoded message
		buf  []byte      // Wire encoding
		pver uint32      // Protocol version for wire encoding
	}{
		// Last supported protocol version.
		{
			msgGetAddr,
			msgGetAddr,
			msgGetAddrEncoded,
			lastSupportedProtocolVersion,
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
		var msg MsgGetAddr
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
