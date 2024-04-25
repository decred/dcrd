// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/chaincfg/chainhash"
)

// makeHeader is a convenience function to make a message header in the form of
// a byte slice.  It is used to force errors when reading messages.
func makeHeader(dcrnet CurrencyNet, command string,
	payloadLen uint32, checksum uint32) []byte {

	// The length of a Decred message header is 24 bytes.
	// 4 byte magic number of the Decred network + 12 byte command + 4 byte
	// payload length + 4 byte checksum.
	buf := make([]byte, 24)
	binary.LittleEndian.PutUint32(buf, uint32(dcrnet))
	copy(buf[4:], []byte(command))
	binary.LittleEndian.PutUint32(buf[16:], payloadLen)
	binary.LittleEndian.PutUint32(buf[20:], checksum)
	return buf
}

// TestMessage tests the Read/WriteMessage and Read/WriteMessageN API.
func TestMessage(t *testing.T) {
	pver := ProtocolVersion

	// Create the various types of messages to test.

	// MsgVersion.
	addrYou := &net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 8333}
	you, err := NewNetAddress(addrYou, SFNodeNetwork)
	if err != nil {
		t.Errorf("NewNetAddress: %v", err)
	}
	you.Timestamp = time.Time{} // Version message has zero value timestamp.
	addrMe := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8333}
	me, err := NewNetAddress(addrMe, SFNodeNetwork)
	if err != nil {
		t.Errorf("NewNetAddress: %v", err)
	}
	me.Timestamp = time.Time{} // Version message has zero value timestamp.
	msgVersion := NewMsgVersion(me, you, 123123, 0)

	msgVerack := NewMsgVerAck()
	msgGetAddr := NewMsgGetAddr()
	msgAddr := NewMsgAddr()
	msgGetBlocks := NewMsgGetBlocks(&chainhash.Hash{})
	msgBlock := &testBlock
	msgInv := NewMsgInv()
	msgGetData := NewMsgGetData()
	msgNotFound := NewMsgNotFound()
	msgTx := NewMsgTx()
	msgPing := NewMsgPing(123123)
	msgPong := NewMsgPong(123123)
	msgGetHeaders := NewMsgGetHeaders()
	msgHeaders := NewMsgHeaders()
	msgMemPool := NewMsgMemPool()
	msgGetCFilter := NewMsgGetCFilter(&chainhash.Hash{}, GCSFilterExtended)
	msgGetCFHeaders := NewMsgGetCFHeaders()
	msgGetCFTypes := NewMsgGetCFTypes()
	msgCFilter := NewMsgCFilter(&chainhash.Hash{}, GCSFilterExtended,
		[]byte("payload"))
	msgCFHeaders := NewMsgCFHeaders()
	msgCFTypes := NewMsgCFTypes([]FilterType{GCSFilterExtended})
	msgReject := NewMsgReject("block", RejectDuplicate, "duplicate block")
	msgGetInitState := NewMsgGetInitState()
	msgInitState := NewMsgInitState()
	msgMixPR, err := NewMsgMixPairReq([33]byte{}, 1, 1, "", 1, 1, 1, 1, []MixPairReqUTXO{}, NewTxOut(0, []byte{}))
	if err != nil {
		t.Errorf("NewMsgMixPairReq: %v", err)
	}
	msgMixKE := NewMsgMixKeyExchange([33]byte{}, [32]byte{}, 1, 1, [33]byte{}, [1218]byte{}, [32]byte{}, []chainhash.Hash{})
	msgMixCT := NewMsgMixCiphertexts([33]byte{}, [32]byte{}, 1, [][1047]byte{}, []chainhash.Hash{})
	msgMixSR := NewMsgMixSlotReserve([33]byte{}, [32]byte{}, 1, [][][]byte{{{}}}, []chainhash.Hash{})
	msgMixDC := NewMsgMixDCNet([33]byte{}, [32]byte{}, 1, []MixVect{make(MixVect, 1)}, []chainhash.Hash{})
	msgMixCM := NewMsgMixConfirm([33]byte{}, [32]byte{}, 1, NewMsgTx(), []chainhash.Hash{})
	msgMixRS := NewMsgMixSecrets([33]byte{}, [32]byte{}, 1, [32]byte{}, [][]byte{}, MixVect{})

	tests := []struct {
		in     Message     // Value to encode
		out    Message     // Expected decoded value
		pver   uint32      // Protocol version for wire encoding
		dcrnet CurrencyNet // Network to use for wire encoding
		bytes  int         // Expected num bytes read/written
	}{
		{msgVersion, msgVersion, pver, MainNet, 125},
		{msgVerack, msgVerack, pver, MainNet, 24},
		{msgGetAddr, msgGetAddr, pver, MainNet, 24},
		{msgAddr, msgAddr, pver, MainNet, 25},
		{msgGetBlocks, msgGetBlocks, pver, MainNet, 61},
		{msgBlock, msgBlock, pver, MainNet, 522},
		{msgInv, msgInv, pver, MainNet, 25},
		{msgGetData, msgGetData, pver, MainNet, 25},
		{msgNotFound, msgNotFound, pver, MainNet, 25},
		{msgTx, msgTx, pver, MainNet, 39},
		{msgPing, msgPing, pver, MainNet, 32},
		{msgPong, msgPong, pver, MainNet, 32},
		{msgGetHeaders, msgGetHeaders, pver, MainNet, 61},
		{msgHeaders, msgHeaders, pver, MainNet, 25},
		{msgMemPool, msgMemPool, pver, MainNet, 24},
		{msgReject, msgReject, RemoveRejectVersion - 1, MainNet, 79},
		{msgGetCFilter, msgGetCFilter, pver, MainNet, 57},
		{msgGetCFHeaders, msgGetCFHeaders, pver, MainNet, 58},
		{msgGetCFTypes, msgGetCFTypes, pver, MainNet, 24},
		{msgCFilter, msgCFilter, pver, MainNet, 65},
		{msgCFHeaders, msgCFHeaders, pver, MainNet, 58},
		{msgCFTypes, msgCFTypes, pver, MainNet, 26},
		{msgGetInitState, msgGetInitState, pver, MainNet, 25},
		{msgInitState, msgInitState, pver, MainNet, 28},
		{msgMixPR, msgMixPR, pver, MainNet, 165},
		{msgMixKE, msgMixKE, pver, MainNet, 1449},
		{msgMixCT, msgMixCT, pver, MainNet, 158},
		{msgMixSR, msgMixSR, pver, MainNet, 161},
		{msgMixDC, msgMixDC, pver, MainNet, 181},
		{msgMixCM, msgMixCM, pver, MainNet, 173},
		{msgMixRS, msgMixRS, pver, MainNet, 192},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		var buf bytes.Buffer
		nw, err := WriteMessageN(&buf, test.in, test.pver, test.dcrnet)
		if err != nil {
			t.Errorf("WriteMessage #%d error %v", i, err)
			continue
		}

		// Ensure the number of bytes written match the expected value.
		if nw != test.bytes {
			t.Errorf("WriteMessage #%d unexpected num bytes "+
				"written - got %d, want %d", i, nw, test.bytes)
		}

		// Decode from wire format.
		rbuf := bytes.NewReader(buf.Bytes())
		nr, msg, _, err := ReadMessageN(rbuf, test.pver, test.dcrnet)
		if err != nil {
			t.Errorf("ReadMessage #%d error %v, msg %v", i, err,
				spew.Sdump(msg))
			continue
		}
		if !reflect.DeepEqual(msg, test.out) {
			t.Errorf("ReadMessage #%d\n got: %v want: %v", i,
				spew.Sdump(msg), spew.Sdump(test.out))
			continue
		}

		// Ensure the number of bytes read match the expected value.
		if nr != test.bytes {
			t.Errorf("ReadMessage #%d unexpected num bytes read - "+
				"got %d, want %d", i, nr, test.bytes)
		}
	}

	// Do the same thing for Read/WriteMessage, but ignore the bytes since
	// they don't return them.
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		var buf bytes.Buffer
		err := WriteMessage(&buf, test.in, test.pver, test.dcrnet)
		if err != nil {
			t.Errorf("WriteMessage #%d error %v", i, err)
			continue
		}

		// Decode from wire format.
		rbuf := bytes.NewReader(buf.Bytes())
		msg, _, err := ReadMessage(rbuf, test.pver, test.dcrnet)
		if err != nil {
			t.Errorf("ReadMessage #%d error %v, msg %v", i, err,
				spew.Sdump(msg))
			continue
		}
		if !reflect.DeepEqual(msg, test.out) {
			t.Errorf("ReadMessage #%d\n got: %v want: %v", i,
				spew.Sdump(msg), spew.Sdump(test.out))
			continue
		}
	}
}

// TestReadMessageWireErrors performs negative tests against wire decoding into
// concrete messages to confirm error paths work correctly.
func TestReadMessageWireErrors(t *testing.T) {
	pver := ProtocolVersion
	dcrnet := MainNet

	// Wire encoded bytes for main and testnet networks magic identifiers.
	testNetBytes := makeHeader(TestNet3, "", 0, 0)

	// Wire encoded bytes for a message that exceeds max overall message
	// length.
	mpl := uint32(MaxMessagePayload)
	exceedMaxPayloadBytes := makeHeader(dcrnet, "getaddr", mpl+1, 0)

	// Wire encoded bytes for a command which is invalid utf-8.
	badCommandBytes := makeHeader(dcrnet, "bogus", 0, 0)
	badCommandBytes[4] = 0x81

	// Wire encoded bytes for a command which is valid, but not supported.
	unsupportedCommandBytes := makeHeader(dcrnet, "bogus", 0, 0)

	// Wire encoded bytes for a message which exceeds the max payload for
	// a specific message type.
	exceedTypePayloadBytes := makeHeader(dcrnet, "getaddr", 1, 0)

	// Wire encoded bytes for a message which does not deliver the full
	// payload according to the header length.
	shortPayloadBytes := makeHeader(dcrnet, "version", 115, 0)

	// Wire encoded bytes for a message with a bad checksum.
	badChecksumBytes := makeHeader(dcrnet, "version", 2, 0xbeef)
	badChecksumBytes = append(badChecksumBytes, []byte{0x0, 0x0}...)

	// Wire encoded bytes for a message which has a valid header, but is
	// the wrong format.  An addr starts with a varint of the number of
	// contained in the message.  Claim there is two, but don't provide
	// them.  At the same time, forge the header fields so the message is
	// otherwise accurate.
	badMessageBytes := makeHeader(dcrnet, "addr", 1, 0xeaadc31c)
	badMessageBytes = append(badMessageBytes, 0x2)

	// Wire encoded bytes for a message which the header claims has 15k
	// bytes of data to discard.
	discardBytes := makeHeader(dcrnet, "bogus", 15*1024, 0)

	tests := []struct {
		buf     []byte      // Wire encoding
		pver    uint32      // Protocol version for wire encoding
		dcrnet  CurrencyNet // Decred network for wire encoding
		max     int         // Max size of fixed buffer to induce errors
		readErr error       // Expected read error
		bytes   int         // Expected num bytes read
	}{
		// Latest protocol version with intentional read errors.

		// Short header. [0]
		{
			[]byte{},
			pver,
			dcrnet,
			0,
			io.EOF,
			0,
		},

		// Wrong network.  Want MainNet, but giving TestNet. [1]
		{
			testNetBytes,
			pver,
			dcrnet,
			len(testNetBytes),
			&MessageError{},
			24,
		},

		// Exceed max overall message payload length. [2]
		{
			exceedMaxPayloadBytes,
			pver,
			dcrnet,
			len(exceedMaxPayloadBytes),
			&MessageError{},
			24,
		},

		// Invalid UTF-8 command. [3]
		{
			badCommandBytes,
			pver,
			dcrnet,
			len(badCommandBytes),
			&MessageError{},
			24,
		},

		// Valid, but unsupported command. [4]
		{
			unsupportedCommandBytes,
			pver,
			dcrnet,
			len(unsupportedCommandBytes),
			&MessageError{},
			24,
		},

		// Exceed max allowed payload for a message of a specific type.  [5]
		{
			exceedTypePayloadBytes,
			pver,
			dcrnet,
			len(exceedTypePayloadBytes),
			&MessageError{},
			24,
		},

		// Message with a payload shorter than the header indicates. [6]
		{
			shortPayloadBytes,
			pver,
			dcrnet,
			len(shortPayloadBytes),
			io.EOF,
			24,
		},

		// Message with a bad checksum. [7]
		{
			badChecksumBytes,
			pver,
			dcrnet,
			len(badChecksumBytes),
			&MessageError{},
			26,
		},

		// Message with a valid header, but wrong format. [8]
		{
			badMessageBytes,
			pver,
			dcrnet,
			len(badMessageBytes),
			&MessageError{},
			25,
		},

		// 15k bytes of data to discard. [9]
		{
			discardBytes,
			pver,
			dcrnet,
			len(discardBytes),
			&MessageError{},
			24,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Decode from wire format.
		r := newFixedReader(test.max, test.buf)
		nr, _, _, err := ReadMessageN(r, test.pver, test.dcrnet)
		if reflect.TypeOf(err) != reflect.TypeOf(test.readErr) {
			t.Errorf("ReadMessage #%d wrong error got: %v <%T>, "+
				"want: %T", i, err, err, test.readErr)
			continue
		}

		// Ensure the number of bytes written match the expected value.
		if nr != test.bytes {
			t.Errorf("ReadMessage #%d unexpected num bytes read - "+
				"got %d, want %d", i, nr, test.bytes)
		}

		// For errors which are not of type MessageError, check them for
		// equality.
		var merr *MessageError
		if !errors.As(err, &merr) {
			if !errors.Is(err, test.readErr) {
				t.Errorf("ReadMessage #%d wrong error got: %v <%T>, "+
					"want: %v <%T>", i, err, err,
					test.readErr, test.readErr)
				continue
			}
		}
	}
}

// TestWriteMessageWireErrors performs negative tests against wire encoding from
// concrete messages to confirm error paths work correctly.
func TestWriteMessageWireErrors(t *testing.T) {
	pver := ProtocolVersion
	dcrnet := MainNet
	wireErr := &MessageError{}

	// Fake message with a command that is too long.
	badCommandMsg := &fakeMessage{command: "somethingtoolong"}

	// Fake message with a problem during encoding
	encodeErrMsg := &fakeMessage{forceEncodeErr: true}

	// Fake message that has payload which exceeds max overall message size.
	exceedOverallPayload := make([]byte, MaxMessagePayload+1)
	exceedOverallPayloadErrMsg := &fakeMessage{payload: exceedOverallPayload}

	// Fake message that has payload which exceeds max allowed per message.
	exceedPayload := make([]byte, 1)
	exceedPayloadErrMsg := &fakeMessage{payload: exceedPayload, forceLenErr: true}

	// Fake message that is used to force errors in the header and payload
	// writes.
	bogusPayload := []byte{0x01, 0x02, 0x03, 0x04}
	bogusMsg := &fakeMessage{command: "bogus", payload: bogusPayload}

	tests := []struct {
		msg    Message     // Message to encode
		pver   uint32      // Protocol version for wire encoding
		dcrnet CurrencyNet // Decred network for wire encoding
		max    int         // Max size of fixed buffer to induce errors
		err    error       // Expected error
		bytes  int         // Expected num bytes written
	}{
		// Command too long.
		{badCommandMsg, pver, dcrnet, 0, wireErr, 0},
		// Force error in payload encode.
		{encodeErrMsg, pver, dcrnet, 0, wireErr, 0},
		// Force error due to exceeding max overall message payload size.
		{exceedOverallPayloadErrMsg, pver, dcrnet, 0, wireErr, 0},
		// Force error due to exceeding max payload for message type.
		{exceedPayloadErrMsg, pver, dcrnet, 0, wireErr, 0},
		// Force error in header write.
		{bogusMsg, pver, dcrnet, 0, io.ErrShortWrite, 0},
		// Force error in payload write.
		{bogusMsg, pver, dcrnet, 24, io.ErrShortWrite, 24},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode wire format.
		w := newFixedWriter(test.max)
		nw, err := WriteMessageN(w, test.msg, test.pver, test.dcrnet)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("WriteMessage #%d wrong error got: %v <%T>, "+
				"want: %T", i, err, err, test.err)
			continue
		}

		// Ensure the number of bytes written match the expected value.
		if nw != test.bytes {
			t.Errorf("WriteMessage #%d unexpected num bytes "+
				"written - got %d, want %d", i, nw, test.bytes)
		}

		// For errors which are not of type MessageError, check them for
		// equality.
		var merr *MessageError
		if !errors.As(err, &merr) {
			if !errors.Is(err, test.err) {
				t.Errorf("ReadMessage #%d wrong error got: %v <%T>, "+
					"want: %v <%T>", i, err, err, test.err, test.err)
				continue
			}
		}
	}
}
