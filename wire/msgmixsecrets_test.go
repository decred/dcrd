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

func newTestMixSecrets() *MsgMixSecrets {
	// Use easily-distinguishable fields.
	sig := *(*[64]byte)(repeat(0x80, 64))
	id := *(*[33]byte)(repeat(0x81, 33))
	sid := *(*[32]byte)(repeat(0x82, 32))

	const run = uint32(0x83838383)

	seed := *(*[32]byte)(repeat(0x84, 32))

	sr := make([][]byte, 4)
	for b := byte(0x85); b < 0x89; b++ {
		sr[b-0x85] = repeat(b, 32)
	}

	m := make(MixVect, 4)
	for b := byte(0x89); b < 0x8D; b++ {
		copy(m[b-0x89][:], repeat(b, 20))
	}

	seenRSs := make([]chainhash.Hash, 4)
	for b := byte(0x8D); b < 0x91; b++ {
		copy(seenRSs[b-0x8D][:], repeat(b, 32))
	}

	rs := NewMsgMixSecrets(id, sid, run, seed, sr, m)
	rs.SeenSecrets = seenRSs
	rs.Signature = sig

	return rs
}

func TestMsgMixSecretsWire(t *testing.T) {
	pver := MixVersion

	rs := newTestMixSecrets()

	buf := new(bytes.Buffer)
	err := rs.BtcEncode(buf, pver)
	if err != nil {
		t.Fatal(err)
	}

	expected := make([]byte, 0, buf.Len())
	expected = append(expected, repeat(0x80, 64)...) // Signature
	expected = append(expected, repeat(0x81, 33)...) // Identity
	expected = append(expected, repeat(0x82, 32)...) // Session ID
	expected = append(expected, repeat(0x83, 4)...)  // Run
	expected = append(expected, repeat(0x84, 32)...) // Seed
	// Four slot reservation mixed messages (repeating 32 bytes of 0x85, 0x86, 0x87, 0x88)
	expected = append(expected, 0x04)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x85, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x86, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x87, 32)...)
	expected = append(expected, 0x20)
	expected = append(expected, repeat(0x88, 32)...)
	// Four xor dc-net mixed messages (repeating 20 bytes of 0x89, 0x8a, 0x8b, 0x8c)
	expected = append(expected, 0x04, 0x14)
	expected = append(expected, repeat(0x89, 20)...)
	expected = append(expected, repeat(0x8a, 20)...)
	expected = append(expected, repeat(0x8b, 20)...)
	expected = append(expected, repeat(0x8c, 20)...)
	// Four seen RSs (repeating 32 bytes of 0x8d, 0x8e, 0x8f, 0x90)
	expected = append(expected, 0x04)
	expected = append(expected, repeat(0x8d, 32)...)
	expected = append(expected, repeat(0x8e, 32)...)
	expected = append(expected, repeat(0x8f, 32)...)
	expected = append(expected, repeat(0x90, 32)...)

	expectedSerializationEqual(t, buf.Bytes(), expected)

	decodedRS := new(MsgMixSecrets)
	err = decodedRS.BtcDecode(bytes.NewReader(buf.Bytes()), pver)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(rs, decodedRS) {
		t.Errorf("BtcDecode got: %s want: %s",
			spew.Sdump(decodedRS), spew.Sdump(rs))
	}
}

func TestMsgMixSecretsCrossProtocol(t *testing.T) {
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

			msg := newTestMixSecrets()

			buf := new(bytes.Buffer)
			err := msg.BtcEncode(buf, tc.encodeVersion)
			if err != nil {
				t.Fatalf("encode failed: %v", err)
			}

			msg = new(MsgMixSecrets)
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

// TestMsgMixSecretsMaxPayloadLength tests the results returned by
// [MsgMixSecrets.MaxPayloadLength] by calculating the maximum payload length.
func TestMsgMixSecretsMaxPayloadLength(t *testing.T) {
	var rs *MsgMixSecrets

	// Test all protocol versions before MixVersion
	for pver := uint32(0); pver < MixVersion; pver++ {
		t.Run(fmt.Sprintf("pver=%d", pver), func(t *testing.T) {
			got := rs.MaxPayloadLength(pver)
			if got != 0 {
				t.Errorf("got %d, expected %d", got, 0)
			}
		})
	}

	var expectedLen uint32 = 64 + // Signature
		33 + // Identity
		32 + // Session ID
		4 + // Run
		32 + // Seed
		uint32(VarIntSerializeSize(MaxMixMcount)) + // SR message count
		MaxMixMcount*varBytesLen(MaxMixFieldValLen) + // Unpadded SR values
		uint32(VarIntSerializeSize(MaxMixMcount)) + // DC-net message count
		uint32(VarIntSerializeSize(MixMsgSize)) + // DC-net message size
		MaxMixMcount*MixMsgSize + // DC-net messages
		uint32(VarIntSerializeSize(MaxMixPeers)) + // RS count
		32*MaxMixPeers // RS hashes

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
			got := rs.MaxPayloadLength(tc.pver)
			if got != tc.len {
				t.Errorf("got %d, expected %d", got, tc.len)
			}
		})
	}
}
