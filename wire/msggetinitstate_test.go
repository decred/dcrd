// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// TestGetInitState tests the MsgGetInitState API.
func TestGetInitState(t *testing.T) {
	pver := ProtocolVersion

	// Ensure Command() returns the expected value.
	wantCmd := "getinitstate"
	msg := NewMsgGetInitState()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgInitMsg: wrong command - got %v, want %v",
			cmd, wantCmd)
	}

	// Ensure max payload returns the expected value for latest protocol
	// version. Num types (varInt) 1 byte + n var strings with max len.
	wantPayload := uint32(1 + 32*(32+1))
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

	// Ensure AddType returns an error when trying to add an invalid type.
	longType := strings.Repeat("x", 33)
	if err := msg.AddType(longType); !errors.Is(err, ErrInitStateTypeTooLong) {
		t.Fatalf("AddType: unexpeted error - got=%v, want %v",
			err, ErrInitStateTypeTooLong)
	}
	nonAscii := "á"
	if err := msg.AddType(nonAscii); !errors.Is(err, ErrMalformedStrictString) {
		t.Fatalf("AddType: unexpeted error - got=%v, want=%v",
			err, ErrMalformedStrictString)
	}

	// Ensure AddType adds up to the maximum allowed.
	for i := 0; i < MaxInitStateTypes; i++ {
		if err := msg.AddType(""); err != nil {
			t.Fatalf("AddType: unable to add max number of entries: %v", err)
		}
	}

	// Ensure AddType does _not_ add one more than the maximum allowed.
	if err := msg.AddType(""); !errors.Is(err, ErrTooManyInitStateTypes) {
		t.Fatalf("AddType: unexpected error - got=%v, want=%v",
			err, ErrTooManyInitStateTypes)
	}
}

// TestGetInitStateWire tests the MsgGetInitState wire encode and decode for
// various numbers of types.
func TestGetInitStateWire(t *testing.T) {
	pver := ProtocolVersion

	// MsgGetInitState message with no types.
	noTypes := NewMsgGetInitState()
	noTypesEncoded := []byte{
		0x00, // Varint for number of types
	}

	// MsgGetInitState message with multiple types.
	multiTypes := NewMsgGetInitState()
	multiTypes.AddType("first")
	multiTypes.AddType("second")
	multiTypes.AddType("third")
	multiTypesEncoded := []byte{
		0x03, // Varint for number of types
		0x05, // Varint for first type
		'f', 'i', 'r', 's', 't',
		0x06, // Varint for second type
		's', 'e', 'c', 'o', 'n', 'd',
		0x05, // Varint for third type
		't', 'h', 'i', 'r', 'd',
	}

	tests := []struct {
		in   *MsgGetInitState // Message to encode
		out  *MsgGetInitState // Expected decoded message
		buf  []byte           // Wire encoding
		pver uint32           // Protocol version for wire encoding
	}{{
		in:   noTypes,
		out:  noTypes,
		buf:  noTypesEncoded,
		pver: pver,
	}, {
		in:   multiTypes,
		out:  multiTypes,
		buf:  multiTypesEncoded,
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
		var msg MsgGetInitState
		rbuf := bytes.NewReader(test.buf)
		err = msg.BtcDecode(rbuf, test.pver)
		if err != nil {
			t.Errorf("BtcDecode #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&msg, test.out) {
			t.Errorf("BtcDecode #%d - got %s, want: %s", i,
				spew.Sdump(&msg), spew.Sdump(test.out))
			continue
		}
	}
}

// TestGetInitStateWireErrors performs negative tests against wire encode and decode of
// MsgGetInitState to confirm error paths work correctly.
func TestGetInitStateWireErrors(t *testing.T) {
	pver := ProtocolVersion

	baseMsg := NewMsgGetInitState()
	baseMsg.AddType("first")
	baseMsg.AddType("second")
	baseMsg.AddType("third")
	baseMsgEncoded := []byte{
		0x03, // Varint for number of types
		0x05, // Varint for first type
		'f', 'i', 'r', 's', 't',
		0x06, // Varint for second type
		's', 'e', 'c', 'o', 'n', 'd',
		0x05, // Varint for third type
		't', 'h', 'i', 'r', 'd',
	}

	// Message that forces an error by having more than the max allowed
	// number of types.
	maxTypes := NewMsgGetInitState()
	maxTypes.Types = make([]string, MaxInitStateTypes+1)
	maxTypesEncoded := []byte{
		0xfc, // Varint for number of types
	}

	// Message that forces an error by trying to encode a longer than
	// allowed type.
	longType := NewMsgGetInitState()
	longType.Types = []string{strings.Repeat("a", MaxInitStateTypeLen+1)}
	longTypeEncoded := []byte{
		0x01, // Varint for number of types
		0xfc, // Varint for first type
	}

	// Message that forces an error by trying to encode a type with invalid
	// characters.
	nonAsciiType := NewMsgGetInitState()
	nonAsciiType.Types = []string{"á"}
	nonAsciiTypeEncoded := []byte{
		0x01, // Varint for number of types
		0x01, // Varint for first type
		'á',
	}

	tests := []struct {
		in       *MsgGetInitState // Value to encode
		buf      []byte           // Wire encoding
		pver     uint32           // Protocol version for wire encoding
		max      int              // Max size of fixed buffer to induce errors
		writeErr error            // Expected write error
		readErr  error            // Expected read error
	}{
		// Force error in number of types varint.
		{baseMsg, baseMsgEncoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in first var string len.
		{baseMsg, baseMsgEncoded, pver, 1, io.ErrShortWrite, io.EOF},
		// Force error in first string varint.
		{baseMsg, baseMsgEncoded, pver, 3, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in first string.
		{baseMsg, baseMsgEncoded, pver, 4, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error with greater than allowed number of types.
		{maxTypes, maxTypesEncoded, pver, 2, ErrTooManyInitStateTypes, ErrTooManyInitStateTypes},
		// Force error with longer than allowed type.
		{longType, longTypeEncoded, pver, 3, ErrInitStateTypeTooLong, ErrVarStringTooLong},
		// Force error with non-ascii type.
		{nonAsciiType, nonAsciiTypeEncoded, pver, 3, ErrMalformedStrictString, ErrMalformedStrictString},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		w := newFixedWriter(test.max)
		err := test.in.BtcEncode(w, test.pver)
		if !errors.Is(err, test.writeErr) {
			t.Errorf("BtcEncode #%d wrong error - got %v, want: %v", i, err,
				test.writeErr)
			continue
		}

		// Decode from wire format.
		var msg MsgGetInitState
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if !errors.Is(err, test.readErr) {
			t.Errorf("BtcDecode #%d wrong error - got %v, want: %v", i, err,
				test.readErr)
			continue
		}
	}
}
