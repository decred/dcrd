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
)

type mixPairReqArgs struct {
	identity               [33]byte
	signature              [64]byte
	expiry                 uint32
	mixAmount              int64
	scriptClass            string
	txVersion              uint16
	lockTime, messageCount uint32
	inputValue             int64
	utxos                  []MixPairReqUTXO
	change                 *TxOut
}

func (a *mixPairReqArgs) msg() (*MsgMixPairReq, error) {
	return NewMsgMixPairReq(a.identity, a.expiry, a.mixAmount, a.scriptClass,
		a.txVersion, a.lockTime, a.messageCount, a.inputValue, a.utxos, a.change)
}

func newMixPairReqArgs() *mixPairReqArgs {
	// Use easily-distinguishable fields.

	sig := *(*[64]byte)(repeat(0x80, 64))
	id := *(*[33]byte)(repeat(0x81, 33))

	const expiry = uint32(0x82828282)
	const mixAmount = int64(0x0383838383838383)
	const sc = "P2PKH-secp256k1-v0"
	const txVersion = uint16(0x8484)
	const lockTime = uint32(0x85858585)
	const messageCount = uint32(0x86868686)
	const inputValue = int64(0x0787878787878787)

	utxos := []MixPairReqUTXO{
		{
			OutPoint: OutPoint{
				Hash:  rhash(0x88),
				Index: 0x89898989,
				Tree:  0x0A,
			},
			Script:    []byte{},
			PubKey:    repeat(0x8B, 33),
			Signature: repeat(0x8C, 64),
			Opcode:    0xBB, // OP_SSGEN
		},
		{
			OutPoint: OutPoint{
				Hash:  rhash(0x8D),
				Index: 0x8E8E8E8E,
				Tree:  0x0F,
			},
			Script:    repeat(0x90, 25),
			PubKey:    repeat(0x91, 33),
			Signature: repeat(0x92, 64),
			Opcode:    0xBC, // OP_SSRTX
		},
	}

	const changeValue = int64(0x1393939393939393)
	pkScript := repeat(0x94, 25)
	change := NewTxOut(changeValue, pkScript)

	return &mixPairReqArgs{
		identity:     id,
		signature:    sig,
		expiry:       expiry,
		mixAmount:    mixAmount,
		scriptClass:  sc,
		txVersion:    txVersion,
		lockTime:     lockTime,
		messageCount: messageCount,
		inputValue:   inputValue,
		utxos:        utxos,
		change:       change,
	}
}

func TestMsgMixPairReqWire(t *testing.T) {
	t.Parallel()

	pver := MixVersion

	a := newMixPairReqArgs()
	pr, err := a.msg()
	if err != nil {
		t.Fatal(err)
	}
	pr.Signature = a.signature

	buf := new(bytes.Buffer)
	err = pr.BtcEncode(buf, pver)
	if err != nil {
		t.Fatal(err)
	}

	expected := make([]byte, 0, buf.Len())
	expected = append(expected, repeat(0x80, 64)...)    // Signature
	expected = append(expected, repeat(0x81, 33)...)    // Identity
	expected = append(expected, repeat(0x82, 4)...)     // Expiry
	expected = append(expected, 0x83, 0x83, 0x83, 0x83, // Amount
		0x83, 0x83, 0x83, 0x03)
	expected = append(expected, byte(len("P2PKH-secp256k1-v0"))) // Script class
	expected = append(expected, []byte("P2PKH-secp256k1-v0")...)
	expected = append(expected, 0x84, 0x84)             // Tx version
	expected = append(expected, repeat(0x85, 4)...)     // Locktime
	expected = append(expected, repeat(0x86, 4)...)     // Message count
	expected = append(expected, 0x87, 0x87, 0x87, 0x87, // Input value
		0x87, 0x87, 0x87, 0x07)
	expected = append(expected, 0x02) // UTXO count
	// First UTXO 8888888888888888888888888888888888888888888888888888888888888888:0x89898989
	expected = append(expected, repeat(0x88, 32)...) // Hash
	expected = append(expected, repeat(0x89, 4)...)  // Index
	expected = append(expected, 0x0a)                // Tree
	expected = append(expected, 0x00)                // Zero-length P2SH redeem script
	expected = append(expected, 0x21)                // 33-byte pubkey
	expected = append(expected, repeat(0x8b, 33)...)
	expected = append(expected, 0x40) // 64-byte signature
	expected = append(expected, repeat(0x8c, 64)...)
	expected = append(expected, 0xBB) // Opcode
	// Second UTXO 8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d8d:0x8e8e8e8e
	expected = append(expected, repeat(0x8d, 32)...) // Hash
	expected = append(expected, repeat(0x8e, 4)...)  // Index
	expected = append(expected, 0x0f)                // Tree
	expected = append(expected, 0x19)                // 25-byte P2SH redeem script
	expected = append(expected, repeat(0x90, 25)...)
	expected = append(expected, 0x21) // 33-byte pubkey
	expected = append(expected, repeat(0x91, 33)...)
	expected = append(expected, 0x40) // 64-byte signature
	expected = append(expected, repeat(0x92, 64)...)
	expected = append(expected, 0xBC) // Opcode
	// Change output
	expected = append(expected, 0x01) // Has change = true
	expected = append(expected, []byte{
		0x93, 0x93, 0x93, 0x93, 0x93, 0x93, 0x93, 0x13, // Amount
		0x00, 0x00, // Version
		0x19, // 25-byte Pkscript
		0x94, 0x94, 0x94, 0x94, 0x94, 0x94, 0x94, 0x94,
		0x94, 0x94, 0x94, 0x94, 0x94, 0x94, 0x94, 0x94,
		0x94, 0x94, 0x94, 0x94, 0x94, 0x94, 0x94, 0x94,
		0x94,
	}...)

	expectedSerializationEqual(t, buf.Bytes(), expected)

	decodedPR := new(MsgMixPairReq)
	err = decodedPR.BtcDecode(bytes.NewReader(buf.Bytes()), pver)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(pr, decodedPR) {
		t.Errorf("BtcDecode got: %s want: %s",
			spew.Sdump(decodedPR), spew.Sdump(pr))
	}
}

func TestNewMixPairReqErrs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		modArgs func(*mixPairReqArgs)
		err     error
	}{{
		name: "LongScriptClass",
		modArgs: func(a *mixPairReqArgs) {
			a.scriptClass = "scriptclassthatexceedsmaximumlength"
		},
		err: ErrMixPairReqScriptClassTooLong,
	}, {
		name: "NonAsciiScriptClass",
		modArgs: func(a *mixPairReqArgs) {
			a.scriptClass = string([]byte{128})
		},
		err: ErrMalformedStrictString,
	}, {
		name: "TooManyUTXOs",
		modArgs: func(a *mixPairReqArgs) {
			a.utxos = make([]MixPairReqUTXO, MaxMixPairReqUTXOs+1)
		},
		err: ErrTooManyMixPairReqUTXOs,
	}}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newMixPairReqArgs()
			tc.modArgs(a)
			_, err := a.msg()
			if !errors.Is(err, tc.err) {
				t.Errorf("expected error %v; got %v", tc.err, err)
			}
		})
	}
}

func TestMsgMixPairReqCrossProtocol(t *testing.T) {
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

			a := newMixPairReqArgs()
			msg, err := a.msg()
			if err != nil {
				t.Fatalf("%v", err)
			}

			buf := new(bytes.Buffer)
			err = msg.BtcEncode(buf, tc.encodeVersion)
			if err != nil {
				t.Fatalf("encode failed: %v", err)
			}

			msg = new(MsgMixPairReq)
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

// TestMsgMixPairReqMaxPayloadLength tests the results returned by
// [MsgMixPairReq.MaxPayloadLength] by calculating the maximum payload length.
func TestMsgMixPairReqMaxPayloadLength(t *testing.T) {
	var pr *MsgMixPairReq

	// Test all protocol versions before MixVersion
	for pver := uint32(0); pver < MixVersion; pver++ {
		t.Run(fmt.Sprintf("pver=%d", pver), func(t *testing.T) {
			got := pr.MaxPayloadLength(pver)
			if got != 0 {
				t.Errorf("got %d, expected %d", got, 0)
			}
		})
	}

	var maxUTXOLen uint32 = 32 + // Hash
		4 + // Index
		1 + // Tree
		varBytesLen(MaxMixPairReqUTXOScriptLen) + // P2SH redeem script
		varBytesLen(33) + // Pubkey
		varBytesLen(64) + // Signature
		1 // Opcode
	var maxTxOutLen uint32 = 8 + // Value
		2 + // Version
		varBytesLen(16384) // PkScript (txscript.MaxScriptLen)
	var expectedLen uint32 = 64 + // Signature
		33 + // Identity
		4 + // Expiry
		8 + // Amount
		varBytesLen(MaxMixPairReqScriptClassLen) + // Script class
		2 + // Tx version
		4 + // Locktime
		4 + // Message count
		8 + // Input value
		uint32(VarIntSerializeSize(MaxMixPairReqUTXOs)) + // UTXO count
		MaxMixPairReqUTXOs*maxUTXOLen + // UTXOs
		maxTxOutLen // Change output

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
			got := pr.MaxPayloadLength(tc.pver)
			if got != tc.len {
				t.Errorf("got %d, expected %d", got, tc.len)
			}
		})
	}
}
