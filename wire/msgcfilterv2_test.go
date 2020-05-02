// Copyright (c) 2019-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/chaincfg/chainhash"
)

// baseMsgCFilterV2 returns a MsgCFilterV2 struct populated with mock values
// that are used throughout tests.  Note that the tests will need to be updated
// if these values are changed since they rely on the current values.
func baseMsgCFilterV2(t *testing.T) *MsgCFilterV2 {
	t.Helper()

	// Mock block hash, filter, and proof.
	hashStr := "000000000000c41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba"
	blockHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("Invalid mock block hash %v", err)
	}
	mockFilterStr := "000000111ca3aafb023074dc5bf2498df791b7d6e846e9f5016006d600"
	filterData, err := hex.DecodeString(mockFilterStr)
	if err != nil {
		t.Fatalf("Invalid mock filter data: %v", err)
	}
	hashStr = "b4895fb9d0b54822550828f2ba07a68ddb1894796800917f8672e65067696347"
	proofHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("Invalid mock proof hash: %v", err)
	}
	mockProof := []chainhash.Hash{*proofHash}

	return NewMsgCFilterV2(blockHash, filterData, 0, mockProof)
}

// TestCFilterV2 tests the MsgCFilterV2 API against the latest protocol
// version.
func TestCFilterV2(t *testing.T) {
	pver := ProtocolVersion

	// Ensure the command is expected value.
	wantCmd := "cfilterv2"
	msg := baseMsgCFilterV2(t)
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgCFilterV2: wrong command - got %v want %v", cmd,
			wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Block hash + max commitment name length (including varint) +
	// proof index + max num proof hashes (including varint).
	wantPayload := uint32(263210)
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

	// Ensure encoding with max filter data per message returns no error.
	msg.Data = make([]byte, MaxCFilterDataSize)
	var buf bytes.Buffer
	if err := msg.BtcEncode(&buf, pver); err != nil {
		t.Fatal(err)
	}

	// Ensure encoding with max proof hashes per message returns no error.
	msg = baseMsgCFilterV2(t)
	msg.ProofHashes = make([]chainhash.Hash, MaxHeaderProofHashes)
	if err := msg.BtcEncode(&buf, pver); err != nil {
		t.Fatal(err)
	}
}

// TestCFilterV2PreviousProtocol tests the MsgCFilterV2 API against the protocol
// prior to version CFilterV2Version.
func TestCFilterV2PreviousProtocol(t *testing.T) {
	// Use the protocol version just prior to CFilterV2Version changes.
	pver := CFilterV2Version - 1

	msg := baseMsgCFilterV2(t)

	// Test encode with old protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver)
	if err == nil {
		t.Errorf("encode of NewMsgCFilterV2 succeeded when it should have " +
			"failed")
	}

	// Test decode with old protocol version.
	var readmsg MsgCFilterV2
	err = readmsg.BtcDecode(&buf, pver)
	if err == nil {
		t.Errorf("decode of NewMsgCFilterV2 succeeded when it should have " +
			"failed")
	}
}

// TestCFilterV2CrossProtocol tests the MsgCFilterV2 API when encoding with
// the latest protocol version and decoding with CFilterV2Version.
func TestCFilterV2CrossProtocol(t *testing.T) {
	msg := baseMsgCFilterV2(t)

	// Encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, ProtocolVersion)
	if err != nil {
		t.Errorf("encode of MsgCFilterV2 failed %v err <%v>", msg, err)
	}

	// Decode with old protocol version.
	var readmsg MsgCFilterV2
	err = readmsg.BtcDecode(&buf, CFilterV2Version)
	if err != nil {
		t.Errorf("decode of MsgCFilterV2 failed [%v] err <%v>", buf, err)
	}
}

// TestCFilterV2Wire tests the MsgCFilterV2 wire encode and decode for various
// protocol versions.
func TestCFilterV2Wire(t *testing.T) {
	msgCFilterV2 := baseMsgCFilterV2(t)
	msgCFilterV2Encoded := []byte{
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
		in   *MsgCFilterV2 // Message to encode
		out  *MsgCFilterV2 // Expected decoded message
		buf  []byte        // Wire encoding
		pver uint32        // Protocol version for wire encoding
	}{{
		// Latest protocol version.
		msgCFilterV2,
		msgCFilterV2,
		msgCFilterV2Encoded,
		ProtocolVersion,
	}, {
		// Protocol version CFilterV2Version+1.
		msgCFilterV2,
		msgCFilterV2,
		msgCFilterV2Encoded,
		CFilterV2Version + 1,
	}, {
		// Protocol version CFilterV2Version.
		msgCFilterV2,
		msgCFilterV2,
		msgCFilterV2Encoded,
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
		var msg MsgCFilterV2
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

// TestCFilterV2WireErrors performs negative tests against wire encode and
// decode of MsgCFilterV2 to confirm error paths work correctly.
func TestCFilterV2WireErrors(t *testing.T) {
	pver := ProtocolVersion

	// Message with valid mock values.
	baseCFilterV2 := baseMsgCFilterV2(t)
	baseCFilterV2Encoded := []byte{
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

	// Message that forces an error by having a commitment name that exceeds the
	// max allowed length.
	badFilterData := bytes.Repeat([]byte{0x00}, MaxCFilterDataSize+1)
	maxDataCFilterV2 := baseMsgCFilterV2(t)
	maxDataCFilterV2.Data = badFilterData
	maxDataCFilterV2Encoded := []byte{
		0xba, 0xdc, 0xb8, 0xe5, 0xc1, 0xe8, 0x95, 0xe8,
		0xe8, 0xfe, 0xf8, 0xd3, 0x42, 0x5f, 0xa0, 0xbf,
		0xe9, 0xd2, 0x8f, 0xdb, 0xf7, 0x2f, 0x87, 0x19,
		0x10, 0xc4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Mock block hash
		0xfe, 0x01, 0x00, 0x04, 0x00, // Varint for filter data length
	}

	// Message that forces an error by having more than the max allowed proof
	// hashes.
	maxHashesCFilterV2 := baseMsgCFilterV2(t)
	maxHashesCFilterV2.ProofHashes = make([]chainhash.Hash, MaxHeaderProofHashes+1)
	maxHashesCFilterV2Encoded := []byte{
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

	tests := []struct {
		in       *MsgCFilterV2 // Value to encode
		buf      []byte        // Wire encoding
		pver     uint32        // Protocol version for wire encoding
		max      int           // Max size of fixed buffer to induce errors
		writeErr error         // Expected write error
		readErr  error         // Expected read error
	}{
		// Force error in start of block hash.
		{baseCFilterV2, baseCFilterV2Encoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in middle of block hash.
		{baseCFilterV2, baseCFilterV2Encoded, pver, 8, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in filter data len.
		{baseCFilterV2, baseCFilterV2Encoded, pver, 32, io.ErrShortWrite, io.EOF},
		// Force error in start of filter data.
		{baseCFilterV2, baseCFilterV2Encoded, pver, 33, io.ErrShortWrite, io.EOF},
		// Force error in middle of filter data.
		{baseCFilterV2, baseCFilterV2Encoded, pver, 45, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in start of proof index.
		{baseCFilterV2, baseCFilterV2Encoded, pver, 62, io.ErrShortWrite, io.EOF},
		// Force error in middle of proof index.
		{baseCFilterV2, baseCFilterV2Encoded, pver, 64, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in num proof hashes.
		{baseCFilterV2, baseCFilterV2Encoded, pver, 66, io.ErrShortWrite, io.EOF},
		// Force error in start of first proof hash.
		{baseCFilterV2, baseCFilterV2Encoded, pver, 67, io.ErrShortWrite, io.EOF},
		// Force error in middle of first proof hash.
		{baseCFilterV2, baseCFilterV2Encoded, pver, 77, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error with greater than max filter data.
		{maxDataCFilterV2, maxDataCFilterV2Encoded, pver, 37, ErrFilterTooLarge, ErrVarBytesTooLong},
		// Force error with greater than max proof hashes.
		{maxHashesCFilterV2, maxHashesCFilterV2Encoded, pver, 67, ErrTooManyProofs, ErrTooManyProofs},
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
		var msg MsgCFilterV2
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if !errors.Is(err, test.readErr) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v", i, err,
				test.readErr)
			continue
		}
	}
}
