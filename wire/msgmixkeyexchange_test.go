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

func newTestMixKeyExchange() *MsgMixKeyExchange {
	// Use easily-distinguishable fields.
	sig := *(*[64]byte)(repeat(0x80, 64))
	id := *(*[33]byte)(repeat(0x81, 33))
	sid := *(*[32]byte)(repeat(0x82, 32))

	const epoch = uint64(0x8383838383838383)
	const run = uint32(0x84848484)

	ecdh := *(*[33]byte)(repeat(0x85, 33))
	pqpk := *(*[1218]byte)(repeat(0x86, 1218))
	commitment := *(*[32]byte)(repeat(0x87, 32))

	seenPRs := make([]chainhash.Hash, 4)
	for b := byte(0x88); b < 0x8C; b++ {
		copy(seenPRs[b-0x88][:], repeat(b, 32))
	}

	ke := NewMsgMixKeyExchange(id, sid, epoch, run, ecdh, pqpk, commitment, seenPRs)
	ke.Signature = sig

	return ke
}

func TestMsgMixKeyExchangeWire(t *testing.T) {
	pver := MixVersion

	ke := newTestMixKeyExchange()

	buf := new(bytes.Buffer)
	err := ke.BtcEncode(buf, pver)
	if err != nil {
		t.Fatal(err)
	}

	expected := make([]byte, 0, buf.Len())
	expected = append(expected, repeat(0x80, 64)...)   // Signature
	expected = append(expected, repeat(0x81, 33)...)   // Identity
	expected = append(expected, repeat(0x82, 32)...)   // Session ID
	expected = append(expected, repeat(0x83, 8)...)    // Epoch
	expected = append(expected, repeat(0x84, 4)...)    // Run
	expected = append(expected, repeat(0x85, 33)...)   // ECDH public key
	expected = append(expected, repeat(0x86, 1218)...) // PQ public key
	expected = append(expected, repeat(0x87, 32)...)   // Secrets commitment
	// Four seen PRs (repeating 32 bytes of 0x88, 0x89, 0x8a, 0x8b)
	expected = append(expected, 0x04)
	expected = append(expected, repeat(0x88, 32)...)
	expected = append(expected, repeat(0x89, 32)...)
	expected = append(expected, repeat(0x8a, 32)...)
	expected = append(expected, repeat(0x8b, 32)...)

	expectedSerializationEqual(t, buf.Bytes(), expected)

	decodedKE := new(MsgMixKeyExchange)
	err = decodedKE.BtcDecode(bytes.NewReader(buf.Bytes()), pver)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(ke, decodedKE) {
		t.Errorf("BtcDecode got: %s want: %s",
			spew.Sdump(decodedKE), spew.Sdump(ke))
	} else {
		t.Logf("bytes: %x", buf.Bytes())
		t.Logf("spew: %s", spew.Sdump(decodedKE))
	}
}

func TestMsgMixKeyExchangeCrossProtocol(t *testing.T) {
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

			msg := newTestMixKeyExchange()

			buf := new(bytes.Buffer)
			err := msg.BtcEncode(buf, tc.encodeVersion)
			if err != nil {
				t.Fatalf("encode failed: %v", err)
			}

			msg = new(MsgMixKeyExchange)
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

// TestMsgMixKeyExchangeMaxPayloadLength tests the results returned by
// [MsgMixKeyExchange.MaxPayloadLength] by calculating the maximum payload length.
func TestMsgMixKeyExchangeMaxPayloadLength(t *testing.T) {
	var ke *MsgMixKeyExchange

	// Test all protocol versions before MixVersion
	for pver := uint32(0); pver < MixVersion; pver++ {
		t.Run(fmt.Sprintf("pver=%d", pver), func(t *testing.T) {
			got := ke.MaxPayloadLength(pver)
			if got != 0 {
				t.Errorf("got %d, expected %d", got, 0)
			}
		})
	}

	var expectedLen uint32 = 64 + // Signature
		33 + // Identity
		32 + // Session ID
		8 + // Epoch
		4 + // Run
		33 + // ECDH public key
		1218 + // sntrup4591761 public key
		32 + // Secrets commitment
		uint32(VarIntSerializeSize(MaxMixPeers)) + // Pair request count
		32*MaxMixPeers // Pair request hashes

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
			got := ke.MaxPayloadLength(tc.pver)
			if got != tc.len {
				t.Errorf("got %d, expected %d", got, tc.len)
			}
		})
	}
}
