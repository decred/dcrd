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

func newTestMixCiphertexts() *MsgMixCiphertexts {
	// Use easily-distinguishable fields.
	sig := *(*[64]byte)(repeat(0x80, 64))
	id := *(*[33]byte)(repeat(0x81, 33))
	sid := *(*[32]byte)(repeat(0x82, 32))

	const run = uint32(0x83838383)

	cts := make([][1047]byte, 4)
	for b := byte(0x84); b < 0x88; b++ {
		copy(cts[b-0x84][:], repeat(b, 1047))
	}

	seenKEs := make([]chainhash.Hash, 4)
	for b := byte(0x88); b < 0x8C; b++ {
		copy(seenKEs[b-0x88][:], repeat(b, 32))
	}

	ct := NewMsgMixCiphertexts(id, sid, run, cts, seenKEs)
	ct.Signature = sig

	return ct
}

func TestMsgMixCiphertextsWire(t *testing.T) {
	pver := MixVersion

	ct := newTestMixCiphertexts()

	buf := new(bytes.Buffer)
	err := ct.BtcEncode(buf, pver)
	if err != nil {
		t.Fatal(err)
	}

	expected := make([]byte, 0, buf.Len())
	expected = append(expected, repeat(0x80, 64)...) // Signature
	expected = append(expected, repeat(0x81, 33)...) // Identity
	expected = append(expected, repeat(0x82, 32)...) // Session ID
	expected = append(expected, repeat(0x83, 4)...)  // Run
	// Varint count of ciphertexts and seen KEs
	expected = append(expected, 0x04)
	// Four ciphextexts (repeating 1047 bytes of 0x84, 0x85, 0x86, 0x87)
	expected = append(expected, repeat(0x84, 1047)...)
	expected = append(expected, repeat(0x85, 1047)...)
	expected = append(expected, repeat(0x86, 1047)...)
	expected = append(expected, repeat(0x87, 1047)...)
	// Four seen KEs (repeating 32 bytes of 0x88, 0x89, 0x8a, 0x8b)
	expected = append(expected, repeat(0x88, 32)...)
	expected = append(expected, repeat(0x89, 32)...)
	expected = append(expected, repeat(0x8a, 32)...)
	expected = append(expected, repeat(0x8b, 32)...)

	expectedSerializationEqual(t, buf.Bytes(), expected)

	decodedCT := new(MsgMixCiphertexts)
	err = decodedCT.BtcDecode(bytes.NewReader(buf.Bytes()), pver)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(ct, decodedCT) {
		t.Errorf("BtcDecode got: %s want: %s",
			spew.Sdump(decodedCT), spew.Sdump(ct))
	}
}

func TestMsgMixCiphertextsCrossProtocol(t *testing.T) {
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

			msg := newTestMixCiphertexts()

			buf := new(bytes.Buffer)
			err := msg.BtcEncode(buf, tc.encodeVersion)
			if err != nil {
				t.Fatalf("encode failed: %v", err)
			}

			msg = new(MsgMixCiphertexts)
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

// TestMsgMixCiphertextsMaxPayloadLength tests the results returned by
// [MsgMixCiphertexts.MaxPayloadLength] by calculating the maximum payload length.
func TestMsgMixCiphertextsMaxPayloadLength(t *testing.T) {
	var ct *MsgMixCiphertexts

	// Test all protocol versions before MixVersion
	for pver := uint32(0); pver < MixVersion; pver++ {
		t.Run(fmt.Sprintf("pver=%d", pver), func(t *testing.T) {
			got := ct.MaxPayloadLength(pver)
			if got != 0 {
				t.Errorf("got %d, expected %d", got, 0)
			}
		})
	}

	var expectedLen uint32 = 64 + // Signature
		33 + // Identity
		32 + // Session ID
		4 + // Run
		uint32(VarIntSerializeSize(MaxMixPeers)) + // Ciphextext and KE hash count
		MaxMixPeers*1047 + // Ciphextexts
		32*MaxMixPeers // Key exchange hashes

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
			got := ct.MaxPayloadLength(tc.pver)
			if got != tc.len {
				t.Errorf("got %d, expected %d", got, tc.len)
			}
		})
	}
}
