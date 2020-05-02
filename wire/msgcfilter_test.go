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

// TestCFilter tests the MsgCFilter API.
func TestCFilter(t *testing.T) {
	pver := ProtocolVersion

	// Block 200,000 hash.
	hashStr := "000000000000007a59f30586c1003752956a8b55e6f741fd5f24c800cd5e5e8c"
	blockHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	// Arbitrary CF data.
	data := make([]byte, 32)

	// Ensure the command is expected value.
	wantCmd := "cfilter"
	msg := NewMsgCFilter(blockHash, GCSFilterExtended, data)
	if cmd := msg.Command(); cmd != wantCmd {
		t.Fatalf("NewMsgCFilter: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Block hash 32 bytes + filter type 1 byte + CF data size (varInt) 5 bytes
	// + max CF data size.
	wantPayload := uint32(262182)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Fatalf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Ensure we get the same data out.
	if msg.BlockHash != *blockHash {
		t.Fatalf("NewMsgCFilter: wrong block hash - got %v, want %v",
			spew.Sdump(&msg.BlockHash), spew.Sdump(blockHash))
	}
	if msg.FilterType != GCSFilterExtended {
		t.Fatalf("NewMsgCFilter: wrong filter type - got %v, want %v",
			spew.Sdump(msg.FilterType), spew.Sdump(GCSFilterExtended))
	}
	if !bytes.Equal(msg.Data, data) {
		t.Fatalf("NewMsgCFilter: wrong data - got %v, want %v",
			spew.Sdump(msg.Data), spew.Sdump(data))
	}

	// Ensure encoding with max CF data per message returns no error.
	data = make([]byte, MaxCFilterDataSize)
	msg = NewMsgCFilter(blockHash, GCSFilterExtended, data)
	if err != nil {
		t.Fatalf("NewMsgCFilter: %v", err)
	}
	var buf bytes.Buffer
	err = msg.BtcEncode(&buf, pver)
	if err != nil {
		t.Fatalf("BtcEncode: %v", err)
	}
}

// TestCFilterWire tests the MsgCFilter wire encode and decode for various
// protocol versions.
func TestCFilterWire(t *testing.T) {
	// Block 200,000 hash.
	hashStr := "000000000000007a59f30586c1003752956a8b55e6f741fd5f24c800cd5e5e8c"
	blockHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	// Arbitrary CF data.
	data := make([]byte, 32)

	msgCFilter := NewMsgCFilter(blockHash, GCSFilterExtended, data)
	msgCFilterEncoded := []byte{
		0x8c, 0x5e, 0x5e, 0xcd, 0x00, 0xc8, 0x24, 0x5f,
		0xfd, 0x41, 0xf7, 0xe6, 0x55, 0x8b, 0x6a, 0x95,
		0x52, 0x37, 0x00, 0xc1, 0x86, 0x05, 0xf3, 0x59,
		0x7a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Block hash
		0x01, // Filter type
		0x20, // Varint for data size
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // CF data
	}

	tests := []struct {
		in   *MsgCFilter // Message to encode
		out  *MsgCFilter // Expected decoded message
		buf  []byte      // Wire encoding
		pver uint32      // Protocol version for wire encoding
	}{
		// Latest protocol version.
		{
			msgCFilter,
			msgCFilter,
			msgCFilterEncoded,
			ProtocolVersion,
		},

		// First CF protocol version.
		{
			msgCFilter,
			msgCFilter,
			msgCFilterEncoded,
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
		var msg MsgCFilter
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

// TestCFilterWireErrors performs negative tests against wire encode and
// decode of MsgCFilter to confirm error paths work correctly.
func TestCFilterWireErrors(t *testing.T) {
	pver := ProtocolVersion
	oldPver := NodeCFVersion - 1

	// Block 200,000 hash.
	hashStr := "000000000000007a59f30586c1003752956a8b55e6f741fd5f24c800cd5e5e8c"
	blockHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	// Arbitrary CF data.
	data := make([]byte, 32)

	baseCFilter := NewMsgCFilter(blockHash, GCSFilterExtended, data)
	baseCFilterEncoded := []byte{
		0x8c, 0x5e, 0x5e, 0xcd, 0x00, 0xc8, 0x24, 0x5f,
		0xfd, 0x41, 0xf7, 0xe6, 0x55, 0x8b, 0x6a, 0x95,
		0x52, 0x37, 0x00, 0xc1, 0x86, 0x05, 0xf3, 0x59,
		0x7a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Block hash
		0x01, // Filter type
		0x20, // Varint for data size
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // CF data
	}

	// Message that forces an error by having more than the max allowed
	// filter data.
	maxData := make([]byte, MaxCFilterDataSize+1)
	maxCFilter := NewMsgCFilter(blockHash, GCSFilterRegular, maxData)
	maxCFilterEncoded := []byte{
		0x8c, 0x5e, 0x5e, 0xcd, 0x00, 0xc8, 0x24, 0x5f,
		0xfd, 0x41, 0xf7, 0xe6, 0x55, 0x8b, 0x6a, 0x95,
		0x52, 0x37, 0x00, 0xc1, 0x86, 0x05, 0xf3, 0x59,
		0x7a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Block hash
		0x01,                         // Filter type
		0xfe, 0x01, 0x00, 0x04, 0x00, // Varint for data size (262145)
	}

	tests := []struct {
		in       *MsgCFilter // Value to encode
		buf      []byte      // Wire encoding
		pver     uint32      // Protocol version for wire encoding
		max      int         // Max size of fixed buffer to induce errors
		writeErr error       // Expected write error
		readErr  error       // Expected read error
	}{
		// Error in old protocol version with and without enough buffer.
		{baseCFilter, baseCFilterEncoded, oldPver, 0, ErrMsgInvalidForPVer, ErrMsgInvalidForPVer},
		{baseCFilter, baseCFilterEncoded, oldPver, 33, ErrMsgInvalidForPVer, ErrMsgInvalidForPVer},
		{baseCFilter, baseCFilterEncoded, oldPver, 66, ErrMsgInvalidForPVer, ErrMsgInvalidForPVer},

		// Force error in start of block hash.
		{baseCFilter, baseCFilterEncoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in middle of block hash.
		{baseCFilter, baseCFilterEncoded, pver, 16, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in filter type.
		{baseCFilter, baseCFilterEncoded, pver, 32, io.ErrShortWrite, io.EOF},
		// Force error in data size.
		{baseCFilter, baseCFilterEncoded, pver, 33, io.ErrShortWrite, io.EOF},
		// Force error in start of filter data.
		{baseCFilter, baseCFilterEncoded, pver, 34, io.ErrShortWrite, io.EOF},
		// Force error in middle of filter data.
		{baseCFilter, baseCFilterEncoded, pver, 50, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error with greater than max filter data size.
		{maxCFilter, maxCFilterEncoded, pver, 38, ErrFilterTooLarge, ErrVarBytesTooLong},
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
		var msg MsgCFilter
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if !errors.Is(err, test.readErr) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}

// TestCFilter performs negative tests against wire decode of MsgCFilter to
// confirm malformed encoded data doesn't pass through.
func TestCFilterMalformedErrors(t *testing.T) {
	pver := ProtocolVersion

	tests := []struct {
		buf []byte // Wire malformed encoded data
		err error  // Expected read error
	}{
		// Has no encoded data.
		{
			[]byte{}, io.EOF,
		},

		// The data size is longer than what is allowed.
		{
			[]byte{
				0x8c, 0x5e, 0x5e, 0xcd, 0x00, 0xc8, 0x24, 0x5f,
				0xfd, 0x41, 0xf7, 0xe6, 0x55, 0x8b, 0x6a, 0x95,
				0x52, 0x37, 0x00, 0xc1, 0x86, 0x05, 0xf3, 0x59,
				0x7a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Block hash
				0x00,                         // Filter type
				0xfe, 0x01, 0x00, 0x04, 0x00, // Varint for data size (262145)
			}, ErrVarBytesTooLong,
		},

		// Data size is greater than inserted data.
		{
			[]byte{
				0x8c, 0x5e, 0x5e, 0xcd, 0x00, 0xc8, 0x24, 0x5f,
				0xfd, 0x41, 0xf7, 0xe6, 0x55, 0x8b, 0x6a, 0x95,
				0x52, 0x37, 0x00, 0xc1, 0x86, 0x05, 0xf3, 0x59,
				0x7a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Block hash
				0x00, // Filter type
				0x20, // Varint for data size
			}, io.EOF,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Decode from wire format.
		var msg MsgCFilter
		rbuf := bytes.NewReader(test.buf)
		err := msg.BtcDecode(rbuf, pver)
		if !errors.Is(err, test.err) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v", i, err,
				test.err)
			continue
		}
	}
}
