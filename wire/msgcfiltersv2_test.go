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

// baseMsgCFiltersV2 returns a MsgCFiltersV2 struct populated with mock values
// that are used throughout tests.  Note that the tests will need to be updated
// if these values are changed since they rely on the current values.
func baseMsgCFiltersV2(t *testing.T) *MsgCFiltersV2 {
	t.Helper()

	filters := []MsgCFilterV2{
		*baseMsgCFilterV2(t),
	}

	return NewMsgCFiltersV2(filters)
}

// TestCFiltersV2 tests the MsgCFiltersV2 API against the latest protocol
// version.
func TestCFiltersV2(t *testing.T) {
	pver := ProtocolVersion

	// Ensure the command is expected value.
	wantCmd := "cfiltersv2"
	msg := baseMsgCFiltersV2(t)
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgCFiltersV2: wrong command - got %v want %v", cmd,
			wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// varint max number of cfilters + max number of cfilters *
	// (Block hash + max commitment name length (including varint) +
	// proof index + max num proof hashes (including varint).)
	wantPayload := uint32(26321001)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for protocol "+
			"version %d - got %v, want %v", pver, maxPayload, wantPayload)
	}

	// Ensure encoding max number of cfilters with max cfilter data and
	// max proof hashes returns no error.
	maxData := make([]byte, MaxCFilterDataSize)
	maxProofHashes := make([]chainhash.Hash, MaxHeaderProofHashes)
	msg.CFilters = make([]MsgCFilterV2, 0, MaxCFiltersV2PerBatch)
	for len(msg.CFilters) < MaxCFiltersV2PerBatch {
		cf := baseMsgCFilterV2(t)
		cf.Data = maxData
		cf.ProofHashes = maxProofHashes
		msg.CFilters = append(msg.CFilters, *cf)
	}

	var buf bytes.Buffer
	if err := msg.BtcEncode(&buf, pver); err != nil {
		t.Fatal(err)
	}

	// Ensure the maximum actually encoded length is less than or equal to
	// the max payload length.
	if uint32(buf.Len()) > maxPayload {
		t.Fatalf("Largest message encoded to a buffer larger than the "+
			" MaxPayloadLength for protocol version %d - got %v, want %v",
			pver, buf.Len(), maxPayload)
	}
}

// TestCFiltersV2PreviousProtocol tests the MsgCFiltersV2 API against the protocol
// prior to version BatchedCFiltersV2Version.
func TestCFiltersV2PreviousProtocol(t *testing.T) {
	// Use the protocol version just prior to BatchedCFiltersV2Version changes.
	pver := BatchedCFiltersV2Version - 1

	msg := baseMsgCFiltersV2(t)

	// Test encode with old protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver)
	if !errors.Is(err, ErrMsgInvalidForPVer) {
		t.Errorf("unexpected error when encoding for protocol version %d, "+
			"prior to message introduction - got %v, want %v", pver,
			err, ErrMsgInvalidForPVer)
	}

	// Test decode with old protocol version.
	var readmsg MsgCFiltersV2
	err = readmsg.BtcDecode(&buf, pver)
	if !errors.Is(err, ErrMsgInvalidForPVer) {
		t.Errorf("unexpected error when decoding for protocol version %d, "+
			"prior to message introduction - got %v, want %v", pver,
			err, ErrMsgInvalidForPVer)
	}
}

// TestCFiltersV2CrossProtocol tests the MsgCFiltersV2 API when encoding with
// the latest protocol version and decoding with BatchedCFiltersV2Version.
func TestCFiltersV2CrossProtocol(t *testing.T) {
	msg := baseMsgCFiltersV2(t)

	// Encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, ProtocolVersion)
	if err != nil {
		t.Errorf("encode of MsgCFiltersV2 failed %v err <%v>", msg, err)
	}

	// Decode with old protocol version.
	var readmsg MsgCFiltersV2
	err = readmsg.BtcDecode(&buf, BatchedCFiltersV2Version)
	if err != nil {
		t.Errorf("decode of MsgCFiltersV2 failed [%v] err <%v>", buf, err)
	}
}

// TestCFiltersV2Wire tests the MsgCFiltersV2 wire encode and decode for various
// protocol versions.
func TestCFiltersV2Wire(t *testing.T) {
	msgCFiltersV2 := baseMsgCFiltersV2(t)
	msgCFiltersV2Encoded := []byte{
		0x01, // Varint for number of filters
		0xba, 0xdc, 0xb8, 0xe5, 0xc1, 0xe8, 0x95, 0xe8,
		0xe8, 0xfe, 0xf8, 0xd3, 0x42, 0x5f, 0xa0, 0xbf,
		0xe9, 0xd2, 0x8f, 0xdb, 0xf7, 0x2f, 0x87, 0x19,
		0x10, 0xc4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Mock block hash
		0x1d, // Varint for filter data length
		0x00, 0x00, 0x00, 0x11, 0x1c, 0xa3, 0xaa, 0xfb,
		0x02, 0x30, 0x74, 0xdc, 0x5b, 0xf2, 0x49, 0x8d,
		0xf7, 0x91, 0xb7, 0xd6, 0xe8, 0x46, 0xe9, 0xf5,
		0x01, 0x60, 0x06, 0xd6, 0x00, // Filter data
		0x00, 0x00, 0x00, 0x00, // Proof index
		0x01, // Varint for num proof hashes
		0x47, 0x63, 0x69, 0x67, 0x50, 0xe6, 0x72, 0x86,
		0x7f, 0x91, 0x00, 0x68, 0x79, 0x94, 0x18, 0xdb,
		0x8d, 0xa6, 0x07, 0xba, 0xf2, 0x28, 0x08, 0x55,
		0x22, 0x48, 0xb5, 0xd0, 0xb9, 0x5f, 0x89, 0xb4, // first proof hash
	}

	tests := []struct {
		in   *MsgCFiltersV2 // Message to encode
		out  *MsgCFiltersV2 // Expected decoded message
		buf  []byte         // Wire encoding
		pver uint32         // Protocol version for wire encoding
	}{{
		// Latest protocol version.
		msgCFiltersV2,
		msgCFiltersV2,
		msgCFiltersV2Encoded,
		ProtocolVersion,
	}, {
		// Protocol version BatchedCFiltersV2Version+1.
		msgCFiltersV2,
		msgCFiltersV2,
		msgCFiltersV2Encoded,
		BatchedCFiltersV2Version + 1,
	}, {
		// Protocol version BatchedCFiltersV2Version.
		msgCFiltersV2,
		msgCFiltersV2,
		msgCFiltersV2Encoded,
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
		var msg MsgCFiltersV2
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

// TestCFiltersV2WireErrors performs negative tests against wire encode and
// decode of MsgCFiltersV2 to confirm error paths work correctly.
func TestCFiltersV2WireErrors(t *testing.T) {
	pver := ProtocolVersion

	// Message with valid mock values.
	baseCFiltersV2 := baseMsgCFiltersV2(t)
	baseCFiltersV2Encoded := []byte{
		0x01, // Varint for number of cfilters
		0xba, 0xdc, 0xb8, 0xe5, 0xc1, 0xe8, 0x95, 0xe8,
		0xe8, 0xfe, 0xf8, 0xd3, 0x42, 0x5f, 0xa0, 0xbf,
		0xe9, 0xd2, 0x8f, 0xdb, 0xf7, 0x2f, 0x87, 0x19,
		0x10, 0xc4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Mock block hash
		0x1d, // Varint for filter data length
		0x00, 0x00, 0x00, 0x11, 0x1c, 0xa3, 0xaa, 0xfb,
		0x02, 0x30, 0x74, 0xdc, 0x5b, 0xf2, 0x49, 0x8d,
		0xf7, 0x91, 0xb7, 0xd6, 0xe8, 0x46, 0xe9, 0xf5,
		0x01, 0x60, 0x06, 0xd6, 0x00, // Filter data
		0x00, 0x00, 0x00, 0x00, // Proof index
		0x01, // Varint for num proof hashes
		0x47, 0x63, 0x69, 0x67, 0x50, 0xe6, 0x72, 0x86,
		0x7f, 0x91, 0x00, 0x68, 0x79, 0x94, 0x18, 0xdb,
		0x8d, 0xa6, 0x07, 0xba, 0xf2, 0x28, 0x08, 0x55,
		0x22, 0x48, 0xb5, 0xd0, 0xb9, 0x5f, 0x89, 0xb4, // first proof hash
	}

	// Message that forces an error by having a data that exceeds the max
	// allowed length.
	badFilterData := bytes.Repeat([]byte{0x00}, MaxCFilterDataSize+1)
	maxDataCFiltersV2 := baseMsgCFiltersV2(t)
	maxDataCFiltersV2.CFilters[0].Data = badFilterData
	maxDataCFiltersV2Encoded := []byte{
		0x01, // Varint for number of cfilters
		0xba, 0xdc, 0xb8, 0xe5, 0xc1, 0xe8, 0x95, 0xe8,
		0xe8, 0xfe, 0xf8, 0xd3, 0x42, 0x5f, 0xa0, 0xbf,
		0xe9, 0xd2, 0x8f, 0xdb, 0xf7, 0x2f, 0x87, 0x19,
		0x10, 0xc4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Mock block hash
		0xfe, 0x01, 0x00, 0x04, 0x00, // Varint for filter data length
	}

	// Message that forces an error by having more than the max allowed proof
	// hashes.
	maxHashesCFiltersV2 := baseMsgCFiltersV2(t)
	maxHashesCFiltersV2.CFilters[0].ProofHashes = make([]chainhash.Hash, MaxHeaderProofHashes+1)
	maxHashesCFiltersV2Encoded := []byte{
		0x01, // Varint for number of cfilters
		0xba, 0xdc, 0xb8, 0xe5, 0xc1, 0xe8, 0x95, 0xe8,
		0xe8, 0xfe, 0xf8, 0xd3, 0x42, 0x5f, 0xa0, 0xbf,
		0xe9, 0xd2, 0x8f, 0xdb, 0xf7, 0x2f, 0x87, 0x19,
		0x10, 0xc4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Mock block hash
		0x1d, // Varint for filter data length
		0x00, 0x00, 0x00, 0x11, 0x1c, 0xa3, 0xaa, 0xfb,
		0x02, 0x30, 0x74, 0xdc, 0x5b, 0xf2, 0x49, 0x8d,
		0xf7, 0x91, 0xb7, 0xd6, 0xe8, 0x46, 0xe9, 0xf5,
		0x01, 0x60, 0x06, 0xd6, 0x00, // Filter data
		0x00, 0x00, 0x00, 0x00, // Proof index
		0x21, // Varint for num proof hashes
	}

	// Message that forces an error by having more than the max allowed number
	// of cfilters.
	maxCFiltersV2 := baseMsgCFiltersV2(t)
	for len(maxCFiltersV2.CFilters) < MaxCFiltersV2PerBatch+1 {
		maxCFiltersV2.CFilters = append(maxCFiltersV2.CFilters, *baseMsgCFilterV2(t))
	}
	maxCFiltersV2Encoded := []byte{
		0x65, // Varint for number of cfilters
	}

	tests := []struct {
		in       *MsgCFiltersV2 // Value to encode
		buf      []byte         // Wire encoding
		pver     uint32         // Protocol version for wire encoding
		max      int            // Max size of fixed buffer to induce errors
		writeErr error          // Expected write error
		readErr  error          // Expected read error
	}{
		// Force error in cfilter number varint.
		{baseCFiltersV2, baseCFiltersV2Encoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in start of block hash.
		{baseCFiltersV2, baseCFiltersV2Encoded, pver, 1, io.ErrShortWrite, io.EOF},
		// Force error in middle of block hash.
		{baseCFiltersV2, baseCFiltersV2Encoded, pver, 9, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in filter data len.
		{baseCFiltersV2, baseCFiltersV2Encoded, pver, 33, io.ErrShortWrite, io.EOF},
		// Force error in start of filter data.
		{baseCFiltersV2, baseCFiltersV2Encoded, pver, 34, io.ErrShortWrite, io.EOF},
		// Force error in middle of filter data.
		{baseCFiltersV2, baseCFiltersV2Encoded, pver, 46, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in start of proof index.
		{baseCFiltersV2, baseCFiltersV2Encoded, pver, 63, io.ErrShortWrite, io.EOF},
		// Force error in middle of proof index.
		{baseCFiltersV2, baseCFiltersV2Encoded, pver, 65, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in num proof hashes.
		{baseCFiltersV2, baseCFiltersV2Encoded, pver, 67, io.ErrShortWrite, io.EOF},
		// Force error in start of first proof hash.
		{baseCFiltersV2, baseCFiltersV2Encoded, pver, 68, io.ErrShortWrite, io.EOF},
		// Force error in middle of first proof hash.
		{baseCFiltersV2, baseCFiltersV2Encoded, pver, 78, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error with greater than max filter data.
		{maxDataCFiltersV2, maxDataCFiltersV2Encoded, pver, 38, ErrFilterTooLarge, ErrVarBytesTooLong},
		// Force error with greater than max proof hashes.
		{maxHashesCFiltersV2, maxHashesCFiltersV2Encoded, pver, 68, ErrTooManyProofs, ErrTooManyProofs},
		// Force error with greater than max cfilters.
		{maxCFiltersV2, maxCFiltersV2Encoded, pver, 1, ErrTooManyCFilters, ErrTooManyCFilters},
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
		var msg MsgCFiltersV2
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if !errors.Is(err, test.readErr) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v", i, err,
				test.readErr)
			continue
		}
	}
}
