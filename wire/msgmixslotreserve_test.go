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

func newTestMixSlotReserve() *MsgMixSlotReserve {
	// Use easily-distinguishable fields.
	sig := *(*[64]byte)(repeat(0x80, 64))
	id := *(*[33]byte)(repeat(0x81, 33))
	sid := *(*[32]byte)(repeat(0x82, 32))

	const run = uint32(0x83838383)

	const mcount = 4
	const kpcount = 4
	dcmix := make([][][]byte, mcount)
	// will add 4x4 field numbers of incrementing repeating byte values to
	// dcmix, ranging from 0x84 through 0x93
	b := byte(0x84)
	for i := 0; i < mcount; i++ {
		dcmix[i] = make([][]byte, kpcount)
		for j := 0; j < kpcount; j++ {
			dcmix[i][j] = repeat(b, 32)
			b++
		}
	}

	seenCTs := make([]chainhash.Hash, 4)
	for b := byte(0x94); b < 0x98; b++ {
		copy(seenCTs[b-0x94][:], repeat(b, 32))
	}

	sr := NewMsgMixSlotReserve(id, sid, run, dcmix, seenCTs)
	sr.Signature = sig

	return sr
}
func TestMsgMixSlotReserveWire(t *testing.T) {
	pver := MixVersion

	sr := newTestMixSlotReserve()

	buf := new(bytes.Buffer)
	err := sr.BtcEncode(buf, pver)
	if err != nil {
		t.Fatal(err)
	}

	expected := make([]byte, 0, buf.Len())
	expected = append(expected, repeat(0x80, 64)...) // Signature
	expected = append(expected, repeat(0x81, 33)...) // Identity
	expected = append(expected, repeat(0x82, 32)...) // Session ID
	expected = append(expected, repeat(0x83, 4)...)  // Run
	// 4x4 slot reservation mixed messages (repeating 32 bytes from 0x84 through 0x93)
	expected = append(expected, 0x04, 0x04)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x84, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x85, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x86, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x87, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x88, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x89, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x8a, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x8b, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x8c, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x8d, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x8e, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x8f, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x90, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x91, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x92, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x93, 32)...)
	// Four seen CTs (repeating 32 bytes of 0x94, 0x95, 0x96, 0x97)
	expected = append(expected, 0x04)
	expected = append(expected, repeat(0x94, 32)...)
	expected = append(expected, repeat(0x95, 32)...)
	expected = append(expected, repeat(0x96, 32)...)
	expected = append(expected, repeat(0x97, 32)...)

	expectedSerializationEqual(t, buf.Bytes(), expected)

	decodedSR := new(MsgMixSlotReserve)
	err = decodedSR.BtcDecode(bytes.NewReader(buf.Bytes()), pver)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(sr, decodedSR) {
		t.Errorf("BtcDecode got: %s want: %s",
			spew.Sdump(decodedSR), spew.Sdump(sr))
	}
}

func TestMsgMixSlotReserveCrossProtocol(t *testing.T) {
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

			msg := newTestMixSlotReserve()

			buf := new(bytes.Buffer)
			err := msg.BtcEncode(buf, tc.encodeVersion)
			if err != nil {
				t.Fatalf("encode failed: %v", err)
			}

			msg = new(MsgMixSlotReserve)
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

// TestMsgMixSlotReserveMaxPayloadLength tests the results returned by
// [MsgMixSlotReserve.MaxPayloadLength] by calculating the maximum payload length.
func TestMsgMixSlotReserveMaxPayloadLength(t *testing.T) {
	var sr *MsgMixSlotReserve

	// Test all protocol versions before MixVersion
	for pver := uint32(0); pver < MixVersion; pver++ {
		t.Run(fmt.Sprintf("pver=%d", pver), func(t *testing.T) {
			got := sr.MaxPayloadLength(pver)
			if got != 0 {
				t.Errorf("got %d, expected %d", got, 0)
			}
		})
	}

	var expectedLen uint32 = 64 + // Signature
		33 + // Identity
		32 + // Session ID
		4 + // Run
		uint32(VarIntSerializeSize(MaxMixMcount)) + // Message count
		uint32(VarIntSerializeSize(MaxMixPeers)) + // Peer count
		MaxMixMcount*MaxMixPeers*varBytesLen(MaxMixFieldValLen) + // Padded SR values
		uint32(VarIntSerializeSize(MaxMixPeers)) + // Ciphertext count
		32*MaxMixPeers // Ciphertext hashes

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
			got := sr.MaxPayloadLength(tc.pver)
			if got != tc.len {
				t.Errorf("got %d, expected %d", got, tc.len)
			}
		})
	}
}
