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

// TestGetCFilter tests the MsgGetCFilter API.
func TestGetCFilter(t *testing.T) {
	pver := ProtocolVersion

	// Block 200,000 hash.
	hashStr := "000000000000007a59f30586c1003752956a8b55e6f741fd5f24c800cd5e5e8c"
	blockHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	// Ensure we get the same data back out.
	filterType := GCSFilterRegular
	msg := NewMsgGetCFilter(blockHash, filterType)
	if !msg.BlockHash.IsEqual(blockHash) {
		t.Fatalf("NewMsgGetCFilter: wrong block hash - got %v, want %v",
			msg.BlockHash, blockHash)
	}

	if msg.FilterType != filterType {
		t.Fatalf("NewMsgGetCFilter: wrong filter type - got %v, want %v",
			msg.FilterType, filterType)
	}

	// Ensure the command is expected value.
	wantCmd := "getcfilter"
	if cmd := msg.Command(); cmd != wantCmd {
		t.Fatalf("NewMsgGetCFilter: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Block hash 32 bytes + filter type 1 byte.
	wantPayload := uint32(33)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Fatalf("NewMsgGetCFilter: wrong max payload length for "+
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

// TestGetCFilterWire tests the MsgGetCFilter wire encode and decode for various
// protocol versions.
func TestGetCFilterWire(t *testing.T) {
	// Block 200,000 hash.
	hashStr := "000000000000007a59f30586c1003752956a8b55e6f741fd5f24c800cd5e5e8c"
	blockHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	// A MsgGetCFilter sample for block 200,000.
	msgGetCFilter1 := NewMsgGetCFilter(blockHash, GCSFilterExtended)
	msgGetCFilter1Encoded1 := []byte{
		0x8c, 0x5e, 0x5e, 0xcd, 0x00, 0xc8, 0x24, 0x5f,
		0xfd, 0x41, 0xf7, 0xe6, 0x55, 0x8b, 0x6a, 0x95,
		0x52, 0x37, 0x00, 0xc1, 0x86, 0x05, 0xf3, 0x59,
		0x7a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Block 200,000 Hash
		0x01, // Filter type
	}

	// A MsgGetCFilter sample with no hash (zero filled hash).
	msgGetCFilter2 := NewMsgGetCFilter(&chainhash.Hash{}, GCSFilterRegular)
	msgGetCFilter2Encoded := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // No hash
		0x00, // Filter type
	}
	tests := []struct {
		in   *MsgGetCFilter // Message to encode
		out  *MsgGetCFilter // Expected decoded message
		buf  []byte         // Wire encoding
		pver uint32         // Protocol version for wire encoding
	}{
		// Block 200,000 for latest protocol version.
		{
			msgGetCFilter1,
			msgGetCFilter1,
			msgGetCFilter1Encoded1,
			ProtocolVersion,
		},

		// Block 200,000 for the first supported version of committed
		// filters (CF).
		{
			msgGetCFilter1,
			msgGetCFilter1,
			msgGetCFilter1Encoded1,
			NodeCFVersion,
		},

		// No hash (zero filled) for the first supported version of committed
		// filters (CF).
		{
			msgGetCFilter2,
			msgGetCFilter2,
			msgGetCFilter2Encoded,
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
		var msg MsgGetCFilter
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

// TestGetCFilterWireErrors performs negative tests against wire encode and
// decode of MsgGetCFilter to confirm error paths work correctly.
func TestGetCFilterWireErrors(t *testing.T) {
	pver := ProtocolVersion
	oldPver := NodeCFVersion - 1

	// Block 200,000 hash.
	hashStr := "000000000000007a59f30586c1003752956a8b55e6f741fd5f24c800cd5e5e8c"
	blockHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}
	msgGetCFilter := NewMsgGetCFilter(blockHash, GCSFilterExtended)
	msgGetCFilterEncoded := []byte{
		0x8c, 0x5e, 0x5e, 0xcd, 0x00, 0xc8, 0x24, 0x5f,
		0xfd, 0x41, 0xf7, 0xe6, 0x55, 0x8b, 0x6a, 0x95,
		0x52, 0x37, 0x00, 0xc1, 0x86, 0x05, 0xf3, 0x59,
		0x7a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Block 200,000 Hash
		0x01, // Filter type
	}

	tests := []struct {
		in       *MsgGetCFilter // Value to encode
		buf      []byte         // Wire encoding
		pver     uint32         // Protocol version for wire encoding
		max      int            // Max size of fixed buffer to induce errors
		writeErr error          // Expected write error
		readErr  error          // Expected read error
	}{
		// Error in old protocol version with and without enough buffer.
		{msgGetCFilter, msgGetCFilterEncoded, oldPver, 0, ErrMsgInvalidForPVer, ErrMsgInvalidForPVer},
		{msgGetCFilter, msgGetCFilterEncoded, oldPver, 31, ErrMsgInvalidForPVer, ErrMsgInvalidForPVer},
		{msgGetCFilter, msgGetCFilterEncoded, oldPver, 100, ErrMsgInvalidForPVer, ErrMsgInvalidForPVer},

		// Latest protocol version with intentional read/write errors.
		// Force error in start of block hash.
		{msgGetCFilter, msgGetCFilterEncoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in middle of block hash.
		{msgGetCFilter, msgGetCFilterEncoded, pver, 16, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in start of filter type.
		{msgGetCFilter, msgGetCFilterEncoded, pver, 32, io.ErrShortWrite, io.EOF},
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
		var msg MsgGetCFilter
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if !errors.Is(err, test.readErr) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v", i, err,
				test.readErr)
			continue
		}
	}
}

// TestGetCFilterMalformedErrors performs negative tests against wire decode
// of MsgGetCFilter to confirm malformed encoded data doesn't pass through.
func TestGetCFilterMalformedErrors(t *testing.T) {
	pver := ProtocolVersion

	tests := []struct {
		buf []byte // Wire malformed encoded data
		err error  // Expected read error
	}{
		// Has no encoded data.
		{
			[]byte{}, io.EOF,
		},

		// Has encoded data to middle of block hash.
		{
			[]byte{
				0x8c, 0x5e, 0x5e, 0xcd, 0x00, 0xc8, 0x24, 0x5f,
				0xfd, 0x41, 0xf7, 0xe6, 0x55, 0x8b, 0x6a, 0x95,
			}, io.ErrUnexpectedEOF,
		},

		// Has no encoded data for filter type.
		{
			[]byte{
				0x8c, 0x5e, 0x5e, 0xcd, 0x00, 0xc8, 0x24, 0x5f,
				0xfd, 0x41, 0xf7, 0xe6, 0x55, 0x8b, 0x6a, 0x95,
				0x52, 0x37, 0x00, 0xc1, 0x86, 0x05, 0xf3, 0x59,
				0x7a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Block Hash
			}, io.EOF,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Decode from wire format.
		var msg MsgGetCFilter
		rbuf := bytes.NewReader(test.buf)
		err := msg.BtcDecode(rbuf, pver)
		if !errors.Is(err, test.err) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v", i, err,
				test.err)
			continue
		}
	}
}
