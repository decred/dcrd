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

// TestGetCFHeaders tests the MsgGetCfHeaders API.
func TestGetCFHeaders(t *testing.T) {
	pver := ProtocolVersion

	// Block 200,000 hash.
	hashStr := "000000000000007a59f30586c1003752956a8b55e6f741fd5f24c800cd5e5e8c"
	locatorHash, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	// Ensure the command is expected value.
	wantCmd := "getcfheaders"
	msg := NewMsgGetCFHeaders()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Fatalf("NewMsgGetCFHeaders: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Count of hashes (varInt) 3 bytes + max block locator hashes + hash stop
	// 32 bytes + filter type 1 byte.
	wantPayload := uint32(16036)
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

	// Ensure block locator hashes are added properly.
	err = msg.AddBlockLocatorHash(locatorHash)
	if err != nil {
		t.Fatalf("AddBlockLocatorHash: %v", err)
	}
	if msg.BlockLocatorHashes[0] != locatorHash {
		t.Fatalf("AddBlockLocatorHash: wrong block locator added - "+
			"got %v, want %v",
			spew.Sprint(msg.BlockLocatorHashes[0]),
			spew.Sprint(locatorHash))
	}

	// Ensure adding up to the max allowed block locator hashes per message
	// returns no error.
	msg = NewMsgGetCFHeaders()
	for i := 0; i < MaxBlockLocatorsPerMsg; i++ {
		err = msg.AddBlockLocatorHash(locatorHash)
	}
	if err != nil {
		t.Fatalf("AddBlockLocatorHash: %v", err)
	}

	// Ensure adding more than the max allowed block locator hashes per
	// message returns an error.
	err = msg.AddBlockLocatorHash(locatorHash)
	if err == nil {
		t.Fatal("AddBlockLocatorHash: expected error on too many " +
			"block locator hashes not received")
	}
}

// TestGetCFHeadersWire tests the MsgGetCFHeaders wire encode and decode for
// various numbers of block locator hashes and protocol versions.
func TestGetCFHeadersWire(t *testing.T) {
	// Block 194,999 hash.
	hashStr := "00000000000000258d61596292b8d2f0630923cdbf82678a43fb2b17a26e99f3"
	hashLocator, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	// Block 195,000 hash.
	hashStr = "000000000000004aaae462677ed9c6841bfb0290117b26456fe2c63434668adb"
	hashLocator2, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	// Block 200,000 hash.
	hashStr = "000000000000007a59f30586c1003752956a8b55e6f741fd5f24c800cd5e5e8c"
	hashStop, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	// MsgGetCFHeaders message with no block locators or stop hash.
	noLocators := NewMsgGetCFHeaders()
	noLocators.FilterType = GCSFilterRegular
	noLocatorsEncoded := []byte{
		0x00, // Varint for number of block locator hashes
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Hash stop
		0x00, // Filter type
	}

	// MsgGetCFHeaders message with multiple block locators and a stop hash.
	multiLocators := NewMsgGetCFHeaders()
	multiLocators.FilterType = GCSFilterExtended
	multiLocators.HashStop = *hashStop
	multiLocators.AddBlockLocatorHash(hashLocator)
	multiLocators.AddBlockLocatorHash(hashLocator2)
	multiLocatorsEncoded := []byte{
		0x02, // Varint for number of block locator hashes
		0xf3, 0x99, 0x6e, 0xa2, 0x17, 0x2b, 0xfb, 0x43,
		0x8a, 0x67, 0x82, 0xbf, 0xcd, 0x23, 0x09, 0x63,
		0xf0, 0xd2, 0xb8, 0x92, 0x62, 0x59, 0x61, 0x8d,
		0x25, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Block 199,499 hash
		0xdb, 0x8a, 0x66, 0x34, 0x34, 0xc6, 0xe2, 0x6f,
		0x45, 0x26, 0x7b, 0x11, 0x90, 0x02, 0xfb, 0x1b,
		0x84, 0xc6, 0xd9, 0x7e, 0x67, 0x62, 0xe4, 0xaa,
		0x4a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Block 199,500 hash
		0x8c, 0x5e, 0x5e, 0xcd, 0x00, 0xc8, 0x24, 0x5f,
		0xfd, 0x41, 0xf7, 0xe6, 0x55, 0x8b, 0x6a, 0x95,
		0x52, 0x37, 0x00, 0xc1, 0x86, 0x05, 0xf3, 0x59,
		0x7a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Hash stop
		0x01, // Filter type
	}

	tests := []struct {
		in   *MsgGetCFHeaders // Message to encode
		out  *MsgGetCFHeaders // Expected decoded message
		buf  []byte           // Wire encoding
		pver uint32           // Protocol version for wire encoding
	}{
		// Latest protocol version with no block locators.
		{
			noLocators,
			noLocators,
			noLocatorsEncoded,
			ProtocolVersion,
		},

		// First protocol version supported committed filters with no block
		// locators.
		{
			noLocators,
			noLocators,
			noLocatorsEncoded,
			NodeCFVersion,
		},

		// Latest protocol version with multiple block locators.
		{
			multiLocators,
			multiLocators,
			multiLocatorsEncoded,
			ProtocolVersion,
		},

		// First protocol version supported committed filters with multiple
		// block locators.
		{
			multiLocators,
			multiLocators,
			multiLocatorsEncoded,
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
		var msg MsgGetCFHeaders
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

// TestGetCFHeadersWireErrors performs negative tests against wire encode and
// decode of MsgGetCFHeaders to confirm error paths work correctly.
func TestGetCFHeadersWireErrors(t *testing.T) {
	pver := ProtocolVersion
	oldPver := NodeCFVersion - 1

	// Block 194,999 hash.
	hashStr := "00000000000000258d61596292b8d2f0630923cdbf82678a43fb2b17a26e99f3"
	hashLocator, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	// Block 195,000 hash.
	hashStr = "000000000000004aaae462677ed9c6841bfb0290117b26456fe2c63434668adb"
	hashLocator2, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	// Block 200,000 hash.
	hashStr = "000000000000007a59f30586c1003752956a8b55e6f741fd5f24c800cd5e5e8c"
	hashStop, err := chainhash.NewHashFromStr(hashStr)
	if err != nil {
		t.Fatalf("NewHashFromStr: %v", err)
	}

	// MsgGetCFHeaders message with multiple block locators and a stop hash.
	getCFHeaders := NewMsgGetCFHeaders()
	getCFHeaders.FilterType = GCSFilterExtended
	getCFHeaders.HashStop = *hashStop
	getCFHeaders.AddBlockLocatorHash(hashLocator)
	getCFHeaders.AddBlockLocatorHash(hashLocator2)
	getCFHeadersEncoded := []byte{
		0x02, // Varint for number of block locator hashes
		0xf3, 0x99, 0x6e, 0xa2, 0x17, 0x2b, 0xfb, 0x43,
		0x8a, 0x67, 0x82, 0xbf, 0xcd, 0x23, 0x09, 0x63,
		0xf0, 0xd2, 0xb8, 0x92, 0x62, 0x59, 0x61, 0x8d,
		0x25, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Block 199,499 hash
		0xdb, 0x8a, 0x66, 0x34, 0x34, 0xc6, 0xe2, 0x6f,
		0x45, 0x26, 0x7b, 0x11, 0x90, 0x02, 0xfb, 0x1b,
		0x84, 0xc6, 0xd9, 0x7e, 0x67, 0x62, 0xe4, 0xaa,
		0x4a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Block 199,500 hash
		0x8c, 0x5e, 0x5e, 0xcd, 0x00, 0xc8, 0x24, 0x5f,
		0xfd, 0x41, 0xf7, 0xe6, 0x55, 0x8b, 0x6a, 0x95,
		0x52, 0x37, 0x00, 0xc1, 0x86, 0x05, 0xf3, 0x59,
		0x7a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Hash stop
		0x01, // Filter type
	}

	tests := []struct {
		in       *MsgGetCFHeaders // Value to encode
		buf      []byte           // Wire encoding
		pver     uint32           // Protocol version for wire encoding
		max      int              // Max size of fixed buffer to induce errors
		writeErr error            // Expected write error
		readErr  error            // Expected read error
	}{
		// Error in old protocol version with and without enough buffer.
		{getCFHeaders, getCFHeadersEncoded, oldPver, 0, ErrMsgInvalidForPVer, ErrMsgInvalidForPVer},
		{getCFHeaders, getCFHeadersEncoded, oldPver, 33, ErrMsgInvalidForPVer, ErrMsgInvalidForPVer},
		{getCFHeaders, getCFHeadersEncoded, oldPver, 98, ErrMsgInvalidForPVer, ErrMsgInvalidForPVer},

		// Force error in block locator hash count.
		{getCFHeaders, getCFHeadersEncoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in start of block locator hashes.
		{getCFHeaders, getCFHeadersEncoded, pver, 1, io.ErrShortWrite, io.EOF},
		// Force error in middle of block locator hashes.
		{getCFHeaders, getCFHeadersEncoded, pver, 17, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in second block locator hash.
		{getCFHeaders, getCFHeadersEncoded, pver, 33, io.ErrShortWrite, io.EOF},
		// Force error in start of stop hash.
		{getCFHeaders, getCFHeadersEncoded, pver, 65, io.ErrShortWrite, io.EOF},
		// Force error in middle of stop hash.
		{getCFHeaders, getCFHeadersEncoded, pver, 81, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in filter type.
		{getCFHeaders, getCFHeadersEncoded, pver, 97, io.ErrShortWrite, io.EOF},
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
		var msg MsgGetCFHeaders
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if !errors.Is(err, test.readErr) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v", i, err,
				test.readErr)
			continue
		}
	}
}

// TestGetCFHeadersMalformedErrors performs negative tests against wire decode
// of MsgGetCFHeaders to confirm malformed encoded data doesn't pass through.
func TestGetCFHeadersMalformedErrors(t *testing.T) {
	pver := ProtocolVersion

	tests := []struct {
		buf []byte // Wire malformed encoded data
		err error  // Expected read error
	}{
		// Has no encoded data.
		{
			[]byte{}, io.EOF,
		},

		// The count of block locator hashes is longer than what is allowed.
		{
			[]byte{
				0xfd, 0xf5, 0x01, // Varint for number of block locators (501)
			}, ErrTooManyLocators,
		},

		// Malformed varint.
		{
			[]byte{
				0xfd, 0x10, 0x00, // Invalid varint
			}, ErrNonCanonicalVarInt,
		},

		// Block locator hashes counter is greater than inserted hashes.
		{
			[]byte{
				0x01, // Varint for number of block locator hashes
				0x8c, 0x5e, 0x5e, 0xcd, 0x00, 0xc8, 0x24, 0x5f,
				0xfd, 0x41, 0xf7, 0xe6, 0x55, 0x8b, 0x6a, 0x95,
				0x52, 0x37, 0x00, 0xc1, 0x86, 0x05, 0xf3, 0x59,
				0x7a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Hash stop
				0x01, // Filter type
			}, io.ErrUnexpectedEOF,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Decode from wire format.
		var msg MsgGetCFHeaders
		rbuf := bytes.NewReader(test.buf)
		err := msg.BtcDecode(rbuf, pver)
		if !errors.Is(err, test.err) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v", i, err,
				test.err)
			continue
		}
	}
}
