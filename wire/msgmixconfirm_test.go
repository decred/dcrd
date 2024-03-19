// Copyright (c) 2023-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/chaincfg/chainhash"
)

func newTestMixConfirm() *MsgMixConfirm {
	// Use easily-distinguishable fields.
	sig := *(*[64]byte)(repeat(0x80, 64))
	id := *(*[33]byte)(repeat(0x81, 33))
	sid := *(*[32]byte)(repeat(0x82, 32))

	const run = uint32(0x83838383)

	mix := NewMsgTx()

	seenDCs := make([]chainhash.Hash, 4)
	for b := byte(0x84); b < 0x88; b++ {
		copy(seenDCs[b-0x84][:], repeat(b, 32))
	}

	cm := NewMsgMixConfirm(id, sid, run, mix, seenDCs)
	cm.Signature = sig

	return cm
}

func TestMsgMixConfirmWire(t *testing.T) {
	pver := MixVersion

	cm := newTestMixConfirm()

	buf := new(bytes.Buffer)
	err := cm.BtcEncode(buf, pver)
	if err != nil {
		t.Fatal(err)
	}

	expected := make([]byte, 0, buf.Len())
	expected = append(expected, repeat(0x80, 64)...) // Signature
	expected = append(expected, repeat(0x81, 33)...) // Identity
	expected = append(expected, repeat(0x82, 32)...) // Session ID
	expected = append(expected, repeat(0x83, 4)...)  // Run
	expected = append(expected, []byte{              // Mix transaction
		0x01, 0x00, 0x00, 0x00, // Version
		0x00,                   // Varint for number of input transactions
		0x00,                   // Varint for number of output transactions
		0x00, 0x00, 0x00, 0x00, // Lock time
		0x00, 0x00, 0x00, 0x00, // Expiry
		0x00, // Varint for number of input signatures
	}...)
	// Four seen DCs (repeating 32 bytes of 0x84, 0x85, 0x86, 0x87)
	expected = append(expected, 0x04)
	expected = append(expected, repeat(0x84, 32)...)
	expected = append(expected, repeat(0x85, 32)...)
	expected = append(expected, repeat(0x86, 32)...)
	expected = append(expected, repeat(0x87, 32)...)

	expectedSerializationEqual(t, buf.Bytes(), expected)

	decodedCM := new(MsgMixConfirm)
	err = decodedCM.BtcDecode(bytes.NewReader(buf.Bytes()), pver)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(cm, decodedCM) {
		t.Errorf("BtcDecode got: %s want: %s",
			spew.Sdump(decodedCM), spew.Sdump(cm))
	}
}

func TestMsgMixConfirmCrossProtocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		encodeVersion  uint32
		decodeVersion  uint32
		err            error
		remainingBytes int
	}{{
		name:          "Latest->MixVersion",
		encodeVersion: ProtocolVersion,
		decodeVersion: MixVersion,
	}, {
		name:          "Latest->MixVersion-1",
		encodeVersion: ProtocolVersion,
		decodeVersion: MixVersion - 1,
		err:           ErrMsgInvalidForPVer,
	}, {
		name:          "MixVersion->Latest",
		encodeVersion: MixVersion,
		decodeVersion: ProtocolVersion,
	}}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.err != nil && tc.remainingBytes != 0 {
				t.Errorf("invalid testcase: non-zero remaining bytes " +
					"expects no decoding error")
			}

			msg := newTestMixConfirm()

			buf := new(bytes.Buffer)
			err := msg.BtcEncode(buf, tc.encodeVersion)
			if err != nil {
				t.Fatalf("encode failed: %v", err)
			}

			msg = new(MsgMixConfirm)
			err = msg.BtcDecode(buf, tc.decodeVersion)
			if !errors.Is(err, tc.err) {
				t.Errorf("decode failed; want %v, got %v", tc.err, err)
			}
			if err == nil && buf.Len() != tc.remainingBytes {
				t.Errorf("buffer contains unexpected remaining bytes "+
					"from encoded message: want %v bytes, got %v (hex: %[2]x)",
					buf.Len(), buf.Bytes())
			}
		})
	}
}

// TestMsgMixConfirmMaxPayloadLength tests the results returned by
// [MsgMixConfirm.MaxPayloadLength] by calculating the maximum payload length.
func TestMsgMixConfirmMaxPayloadLength(t *testing.T) {
	var cm *MsgMixConfirm

	// Test all protocol versions before MixVersion
	for pver := uint32(0); pver < MixVersion; pver++ {
		t.Run(fmt.Sprintf("pver=%d", pver), func(t *testing.T) {
			got := cm.MaxPayloadLength(pver)
			if got != 0 {
				t.Errorf("got %d, expected %d", got, 0)
			}
		})
	}

	var expectedLen uint32 = 64 + // Signature
		33 + // Identity
		32 + // Session ID
		4 + // Run
		MaxBlockPayloadV3 + // Maximum transaction size
		uint32(VarIntSerializeSize(MaxMixPeers)) + // DC-net count
		32*MaxMixPeers // DC-net hashes

	tests := []struct {
		name string
		pver uint32
		len  uint32
	}{{
		name: "MixVersion",
		pver: MixVersion,
		len:  expectedLen,
	}, {
		name: "ProtocolVersion",
		pver: ProtocolVersion,
		len:  expectedLen,
	}}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("pver=%s", tc.name), func(t *testing.T) {
			got := cm.MaxPayloadLength(tc.pver)
			if got != tc.len {
				t.Errorf("got %d, expected %d", got, tc.len)
			}
		})
	}
}
