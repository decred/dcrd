// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// TestCFTypes tests the MsgCFTypes API.
func TestCFTypes(t *testing.T) {
	pver := ProtocolVersion

	// MsgCFTypes can use more than one filter, in here we are testing
	// a combination of more than one filter at wire level, whether
	// these filters be compatible with each other must be checked at
	// higher level.
	filters := []FilterType{GCSFilterRegular, GCSFilterExtended}

	// Ensure the command is expected value.
	wantCmd := "cftypes"
	msg := NewMsgCFTypes(filters)
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgCFTypes: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Filters count (varInt) 3 bytes + 1 byte up to 256 bytes for each
	// filter type.
	wantPayload := uint32(259)
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
		t.Errorf("encode of MsgCFTypes failed %v err <%v>", msg,
			err)
	}

	// Older protocol versions should fail encode since message didn't
	// exist yet.
	oldPver := NodeCFVersion - 1
	err = msg.BtcEncode(&buf, oldPver)
	if err == nil {
		s := "encode of MsgCFTypes passed for old protocol " +
			"version %v err <%v>"
		t.Errorf(s, msg, err)
	}

	// Test decode with latest protocol version.
	readmsg := NewMsgCFTypes(filters)
	err = readmsg.BtcDecode(&buf, pver)
	if err != nil {
		t.Errorf("decode of MsgCFTypes failed [%v] err <%v>", buf,
			err)
	}

	// Older protocol versions should fail decode since message didn't
	// exist yet.
	err = readmsg.BtcDecode(&buf, oldPver)
	if err == nil {
		s := "decode of MsgCFTypes passed for old protocol " +
			"version %v err <%v>"
		t.Errorf(s, msg, err)
	}
}

// TestCFTypesWire tests the CFTypesWire wire encode and decode for
// various protocol versions and number of filter types.
func TestCFTypesWire(t *testing.T) {
	// Test cases for correctness of various combination of protocol
	// versions and different number of filter types.
	tests := []struct {
		in   *MsgCFTypes // Message to encode
		out  *MsgCFTypes // Expected decoded message
		buf  []byte      // Wire encoding
		pver uint32      // Protocol version for wire encoding
	}{
		// Empty filter type for latest protocol version.
		{
			NewMsgCFTypes([]FilterType{}), // Empty filter type
			&MsgCFTypes{SupportedFilters: []FilterType{}},
			[]byte{0x00},
			ProtocolVersion,
		},

		// One filter type for first protocol version supported
		// committed filters (CF).
		{
			NewMsgCFTypes([]FilterType{GCSFilterRegular}),
			&MsgCFTypes{
				SupportedFilters: []FilterType{FilterType(0)}},
			[]byte{
				0x01, // Number of filter types
				0x00, // Filter types
			},
			NodeCFVersion,
		},

		// More than one filter type for first protocol version
		// supported committed filters (CF).
		{
			NewMsgCFTypes([]FilterType{
				GCSFilterRegular, GCSFilterExtended}),
			&MsgCFTypes{SupportedFilters: []FilterType{FilterType(0), FilterType(1)}},
			[]byte{
				0x02,       // Number of filter types
				0x00, 0x01, // Filter types
			},
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
		var msg MsgCFTypes
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

// TestCFTypesWireErrors performs negative tests against wire encode and decode
// of CFTypes to confirm error paths work correctly.
func TestCFTypesWireErrors(t *testing.T) {
	pver := ProtocolVersion
	oldPver := NodeCFVersion - 1
	wireErr := &MessageError{}

	// Valid MsgCFTypes with it's encoded format.
	baseCf := NewMsgCFTypes([]FilterType{GCSFilterExtended})
	baseCfEncoded := []byte{
		0x01, // Varint for number of filter types
		0x01, // A sample of filter types
	}

	// Invalid encoded MsgCFTypes, the count of filter types is longer than
	// what is supported (256).
	cfInvalidEncoded := []byte{
		0xfd, 0x01, 0x01, // Varint for number of filter types (257)
		0x01, // A sample of filter types
	}

	// Message that forces an error by having more than the maximum
	// allowed (256) filter types.
	filters := make([]FilterType, MaxFilterTypesPerMsg+1)
	for i := 0; i < MaxFilterTypesPerMsg+1; i++ {
		filters[i] = FilterType(i)
	}
	maxCf := NewMsgCFTypes(filters)

	tests := []struct {
		in       *MsgCFTypes // Value to encode
		buf      []byte      // Wire encoding
		pver     uint32      // Protocol version for wire encoding
		max      int         // Max size of fixed buffer to induce errors
		writeErr error       // Expected write error
		readErr  error       // Expected read error
	}{
		// Error in old protocol version with and without enough buffer.
		{baseCf, baseCfEncoded, oldPver, 0, wireErr, wireErr},
		{baseCf, baseCfEncoded, oldPver, 100, wireErr, wireErr},

		// Force error in count of filter types.
		{baseCf, baseCfEncoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in start of filter types.
		{baseCf, baseCfEncoded, pver, 1, io.ErrShortWrite, io.EOF},

		// Error for maximum allowed filter types with enough buffer.
		{maxCf, cfInvalidEncoded, pver, 1000, wireErr, wireErr},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		w := newFixedWriter(test.max)
		err := test.in.BtcEncode(w, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.writeErr) {
			t.Errorf("BtcEncode #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// For errors which are not of type MessageError, check them for
		// equality.
		if _, ok := err.(*MessageError); !ok {
			if err != test.writeErr {
				t.Errorf("BtcEncode #%d wrong error got: %v, "+
					"want: %v", i, err, test.writeErr)
				continue
			}
		}

		// Decode from wire format.
		var msg MsgCFTypes
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.readErr) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}

		// For errors which are not of type MessageError, check them for
		// equality.
		if _, ok := err.(*MessageError); !ok {
			if err != test.readErr {
				t.Errorf("BtcDecode #%d wrong error got: %v, "+
					"want: %v %s", i, err, test.readErr, msg)
				continue
			}
		}
	}
}

// TestCFTypesMalformedErrors performs negative tests against decode
// of CFTypes to confirm malformed encoded data doesn't pass through.
func TestCFTypesMalformedErrors(t *testing.T) {
	pver := ProtocolVersion
	wireErr := &MessageError{}

	tests := []struct {
		buf []byte // Wire malformed encoded data
		err error  // Expected read error
	}{
		// Has the count of filter types without actually holding them.
		{
			[]byte{
				0x0f, // Varint for number of filter types (15)
			}, io.EOF,
		},

		// The count of filter types is longer than what is supported (256).
		{
			[]byte{
				0xfd, 0x01, 0x01, // Varint for number of filter types (257)
				0x01, // A sample of filter type
			}, wireErr,
		},

		// Malformed varint.
		{
			[]byte{
				0xfd, 0x01, 0x00, // Invalid varint
				0x01, // A sample of filter type
			}, wireErr,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Decode from wire format.
		var msg MsgCFTypes
		rbuf := bytes.NewReader(test.buf)
		err := msg.BtcDecode(rbuf, pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.err)
			continue
		}

		// For errors which are not of type MessageError, check them for
		// equality.
		if _, ok := err.(*MessageError); !ok {
			if err != test.err {
				t.Errorf("BtcDecode #%d wrong error got: %v, "+
					"want: %v %s", i, err, test.err, msg)
				continue
			}
		}
	}
}
