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

// TestCFHeaders tests the MsgCFHeaders API.
func TestCFHeaders(t *testing.T) {
	pver := ProtocolVersion

	// Block 200,000 hash.
	hashStr := "000000000000007a59f30586c1003752956a8b55e6f741fd5f24c800cd5e5e8c"
	headerHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	// Ensure the command is expected value.
	wantCmd := "cfheaders"
	msg := NewMsgCFHeaders()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Fatalf("NewMsgCFHeaders: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Stop hash 32 bytes + filter type 1 byte + num of hashes (varInt) 3 bytes
	// + max CF header hashes.
	wantPayload := uint32(64036)
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

	// Ensure CF headers are added properly.
	err = msg.AddCFHeader(headerHash)
	if err != nil {
		t.Fatalf("AddCFHeader: %v", err)
	}
	if msg.HeaderHashes[0] != headerHash {
		t.Fatalf("AddCFHeader: wrong CF header added - got %v, want %v",
			spew.Sprint(msg.HeaderHashes[0]),
			spew.Sprint(headerHash))
	}

	// Ensure adding to max allowed CF headers per message returns no error.
	msg = NewMsgCFHeaders()
	for i := 0; i < MaxCFHeadersPerMsg; i++ {
		err = msg.AddCFHeader(headerHash)
	}
	if err != nil {
		t.Errorf("AddCFHeader: %v", err)
	}

	// Ensure adding more than the max allowed CF headers per message returns
	// error.
	err = msg.AddCFHeader(headerHash)
	if err == nil {
		t.Fatal("AddCFHeader: expected error on too many CF headers" +
			" not received")
	}
}

// TestCFHeadersWire tests the MsgCFHeaders wire encode and decode for various
// numbers of CF headers and protocol versions.
func TestCFHeadersWire(t *testing.T) {
	// Empty MsgCFHeaders message.
	emptyCFHeader := NewMsgCFHeaders()
	emptyCFHeaderEncoded := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Stop hash
		0x00, // Filter type
		0x00, // Varint for number of headers
	}

	// Block 200,000 hash.
	hashStr := "000000000000007a59f30586c1003752956a8b55e6f741fd5f24c800cd5e5e8c"
	stopHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	// Block 200,000 CF header hash.
	hashStr = "317c6894f386e2c08f2af987539afa0569d080fe27e594bc28bcbcefa669e79f"
	hashBytes, err := hex.DecodeString(hashStr)
	if err != nil {
		t.Fatalf("DecodeString: %v", err)
	}
	cfHeaderHash, err := chainhash.NewHash(hashBytes)
	if err != nil {
		t.Fatalf("NewHash: %v", err)
	}

	// MsgCFHeaders message with one CF header hash and a stop hash.
	oneCFHeader := NewMsgCFHeaders()
	oneCFHeader.StopHash = *stopHash
	oneCFHeader.FilterType = GCSFilterExtended
	oneCFHeader.AddCFHeader(cfHeaderHash)
	oneCFHeaderEncoded := []byte{
		0x8c, 0x5e, 0x5e, 0xcd, 0x00, 0xc8, 0x24, 0x5f,
		0xfd, 0x41, 0xf7, 0xe6, 0x55, 0x8b, 0x6a, 0x95,
		0x52, 0x37, 0x00, 0xc1, 0x86, 0x05, 0xf3, 0x59,
		0x7a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Stop hash
		0x01, // Filter type
		0x01, // Varint for number of CF headers
		0x31, 0x7c, 0x68, 0x94, 0xf3, 0x86, 0xe2, 0xc0,
		0x8f, 0x2a, 0xf9, 0x87, 0x53, 0x9a, 0xfa, 0x05,
		0x69, 0xd0, 0x80, 0xfe, 0x27, 0xe5, 0x94, 0xbc,
		0x28, 0xbc, 0xbc, 0xef, 0xa6, 0x69, 0xe7, 0x9f, // CF header hash
	}

	tests := []struct {
		in   *MsgCFHeaders // Message to encode
		out  *MsgCFHeaders // Expected decoded message
		buf  []byte        // Wire encoding
		pver uint32        // Protocol version for wire encoding
	}{
		// Latest protocol version with empty MsgCFHeaders.
		{
			emptyCFHeader,
			emptyCFHeader,
			emptyCFHeaderEncoded,
			ProtocolVersion,
		},

		// Latest protocol version with one CF header.
		{
			oneCFHeader,
			oneCFHeader,
			oneCFHeaderEncoded,
			ProtocolVersion,
		},

		// First CF protocol version with one CF header.
		{
			oneCFHeader,
			oneCFHeader,
			oneCFHeaderEncoded,
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
		var msg MsgCFHeaders
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

// TestCFHeadersWireErrors performs negative tests against wire encode and decode
// of MsgCFHeaders to confirm error paths work correctly.
func TestCFHeadersWireErrors(t *testing.T) {
	pver := ProtocolVersion
	oldPver := NodeCFVersion - 1
	wireErr := &MessageError{}

	// Block 200,000 hash.
	hashStr := "000000000000007a59f30586c1003752956a8b55e6f741fd5f24c800cd5e5e8c"
	stopHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	// Block 200,000 CF header hash.
	hashStr = "317c6894f386e2c08f2af987539afa0569d080fe27e594bc28bcbcefa669e79f"
	hashBytes, err := hex.DecodeString(hashStr)
	if err != nil {
		t.Fatalf("DecodeString: %v", err)
	}
	cfHeaderHash, err := chainhash.NewHash(hashBytes)
	if err != nil {
		t.Fatalf("NewHash: %v", err)
	}

	// MsgCFHeaders message with one CF header hash and a stop hash.
	oneCFHeader := NewMsgCFHeaders()
	oneCFHeader.StopHash = *stopHash
	oneCFHeader.FilterType = GCSFilterExtended
	oneCFHeader.AddCFHeader(cfHeaderHash)
	oneCFHeaderEncoded := []byte{
		0x8c, 0x5e, 0x5e, 0xcd, 0x00, 0xc8, 0x24, 0x5f,
		0xfd, 0x41, 0xf7, 0xe6, 0x55, 0x8b, 0x6a, 0x95,
		0x52, 0x37, 0x00, 0xc1, 0x86, 0x05, 0xf3, 0x59,
		0x7a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Stop hash
		0x01, // Filter type
		0x01, // Varint for number of CF headers
		0x31, 0x7c, 0x68, 0x94, 0xf3, 0x86, 0xe2, 0xc0,
		0x8f, 0x2a, 0xf9, 0x87, 0x53, 0x9a, 0xfa, 0x05,
		0x69, 0xd0, 0x80, 0xfe, 0x27, 0xe5, 0x94, 0xbc,
		0x28, 0xbc, 0xbc, 0xef, 0xa6, 0x69, 0xe7, 0x9f, // CF header hash
	}

	tests := []struct {
		in       *MsgCFHeaders // Value to encode
		buf      []byte        // Wire encoding
		pver     uint32        // Protocol version for wire encoding
		max      int           // Max size of fixed buffer to induce errors
		writeErr error         // Expected write error
		readErr  error         // Expected read error
	}{
		// Error in old protocol version with and without enough buffer.
		{oneCFHeader, oneCFHeaderEncoded, oldPver, 0, wireErr, wireErr},
		{oneCFHeader, oneCFHeaderEncoded, oldPver, 34, wireErr, wireErr},
		{oneCFHeader, oneCFHeaderEncoded, oldPver, 66, wireErr, wireErr},

		// Latest protocol version with intentional read/write errors.
		// Force error in start of stop hash.
		{oneCFHeader, oneCFHeaderEncoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in middle of stop hash.
		{oneCFHeader, oneCFHeaderEncoded, pver, 16, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in filter type.
		{oneCFHeader, oneCFHeaderEncoded, pver, 32, io.ErrShortWrite, io.EOF},
		// Force error in num of header hashes.
		{oneCFHeader, oneCFHeaderEncoded, pver, 33, io.ErrShortWrite, io.EOF},
		// Force error in start of header hash.
		{oneCFHeader, oneCFHeaderEncoded, pver, 34, io.ErrShortWrite, io.EOF},
		// Force error in middle of header hash.
		{oneCFHeader, oneCFHeaderEncoded, pver, 50, io.ErrShortWrite, io.ErrUnexpectedEOF},
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
		var merr *MessageError
		if !errors.As(err, &merr) {
			if !errors.Is(err, test.writeErr) {
				t.Errorf("BtcEncode #%d wrong error got: %v, "+
					"want: %v", i, err, test.writeErr)
				continue
			}
		}

		// Decode from wire format.
		var msg MsgCFHeaders
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.readErr) {
			spew.Dump(test)
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}

		// For errors which are not of type MessageError, check them for
		// equality.
		if !errors.As(err, &merr) {
			if !errors.Is(err, test.readErr) {
				t.Errorf("BtcDecode #%d wrong error got: %v, "+
					"want: %v", i, err, test.readErr)
				continue
			}
		}
	}
}

// TestCFHeadersMalformedErrors performs negative tests against decode
// of MsgCFHeaders to confirm malformed encoded data doesn't pass through.
func TestCFHeadersMalformedErrors(t *testing.T) {
	pver := ProtocolVersion
	wireErr := &MessageError{}

	tests := []struct {
		buf []byte // Wire malformed encoded data
		err error  // Expected read error
	}{
		// Number of header hashes is larger than what the message holds.
		{
			[]byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Stop hash
				0x00, // Filter type
				0x01, // Varint for number of header
			}, io.EOF,
		},

		// Number of header hashes is larger than max of CF header per message.
		{
			[]byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Stop hash
				0x00,             // Filter type
				0xfd, 0xd1, 0x07, // Varint for number of header (2001)
			}, wireErr,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Decode from wire format.
		var msg MsgCFHeaders
		rbuf := bytes.NewReader(test.buf)
		err := msg.BtcDecode(rbuf, pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.err)
			continue
		}

		// For errors which are not of type MessageError, check them for
		// equality.
		var merr *MessageError
		if !errors.As(err, &merr) {
			if !errors.Is(err, test.err) {
				t.Errorf("BtcDecode #%d wrong error got: %v, "+
					"want: %v %v", i, err, test.err, msg)
				continue
			}
		}
	}
}
