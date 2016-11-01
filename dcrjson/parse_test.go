// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson_test

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrjson"
	"strings"
)

func decodeHash(reversedHash string) chainhash.Hash {
	h, err := chainhash.NewHashFromStr(reversedHash)
	if err != nil {
		panic(err)
	}
	return *h
}

func TestEncodeConcatenatedHashes(t *testing.T) {
	// Input Hash slice. These are the hexadecimal values of the underlying byte
	// array of each hash.
	hashSlice := []chainhash.Hash{
		{
			0x80, 0xd9, 0x21, 0x2b, 0xf4, 0xce, 0xb0, 0x66,
			0xde, 0xd2, 0x86, 0x6b, 0x39, 0xd4, 0xed, 0x89,
			0xe0, 0xab, 0x60, 0xf3, 0x35, 0xc1, 0x1d, 0xf8,
			0xe7, 0xbf, 0x85, 0xd9, 0xc3, 0x5c, 0x8e, 0x29,
		}, {
			0xb9, 0x26, 0xd1, 0x87, 0x0d, 0x6f, 0x88, 0x76,
			0x0a, 0x8b, 0x10, 0xdb, 0x0d, 0x44, 0x39, 0xe5,
			0xcd, 0x74, 0xf3, 0x82, 0x7f, 0xd4, 0xb6, 0x82,
			0x74, 0x43, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}, {
			0xba, 0xdc, 0xb8, 0xe5, 0xc1, 0xe8, 0x95, 0xe8,
			0xe8, 0xfe, 0xf8, 0xd3, 0x42, 0x5f, 0xa0, 0xbf,
			0xe9, 0xd2, 0x8f, 0xdb, 0xf7, 0x2f, 0x87, 0x19,
			0x10, 0xc4, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}, {
			0xf5, 0x1c, 0xbd, 0x27, 0x7f, 0x63, 0x2f, 0x59,
			0x96, 0xec, 0xa0, 0x5d, 0x48, 0xb0, 0xa3, 0x57,
			0xd7, 0x4d, 0x42, 0xf4, 0xa0, 0x51, 0x3f, 0x3e,
			0xac, 0x08, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
		},
	}
	hashLen := hex.EncodedLen(len(hashSlice[0]))

	// Expected output. The string representation the same hexadecimal values in
	// the input []chainhash.Hash
	blockHashes := []string{
		"80d9212bf4ceb066ded2866b39d4ed89e0ab60f335c11df8e7bf85d9c35c8e29",
		"b926d1870d6f88760a8b10db0d4439e5cd74f3827fd4b6827443000000000000",
		"badcb8e5c1e895e8e8fef8d3425fa0bfe9d28fdbf72f871910c4000000000000",
		"f51cbd277f632f5996eca05d48b0a357d74d42f4a0513f3eac08010000000000",
	}
	concatenatedHashes := strings.Join(blockHashes, "")

	// Test from 0 to N of the hashes
	for j := 0; j < len(hashSlice)+1; j++ {
		// Expected output string
		concatRef := concatenatedHashes[:j*hashLen]

		// Encode to string
		concatenated, err := dcrjson.EncodeConcatenatedHashes(hashSlice[:j])
		if err != nil {
			t.Fatal("Encode failed:", err)
		}
		// Verify output
		if concatenated != concatRef {
			t.Fatalf("EncodeConcatenatedHashes failed (%v!=%v)",
				concatenated, concatRef)
		}
	}
}

func TestDecodeConcatenatedHashes(t *testing.T) {
	// Test data taken from Decred's first three mainnet blocks
	testHashes := []chainhash.Hash{
		decodeHash("298e5cc3d985bfe7f81dc135f360abe089edd4396b86d2de66b0cef42b21d980"),
		decodeHash("000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9"),
		decodeHash("000000000000c41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba"),
	}
	var concatenatedHashBytes []byte
	for _, h := range testHashes {
		concatenatedHashBytes = append(concatenatedHashBytes, h[:]...)
	}
	concatenatedHashes := hex.EncodeToString(concatenatedHashBytes)
	decodedHashes, err := dcrjson.DecodeConcatenatedHashes(concatenatedHashes)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if len(testHashes) != len(decodedHashes) {
		t.Fatalf("Got wrong number of decoded hashes (%v)", len(decodedHashes))
	}
	for i, expected := range testHashes {
		if expected != decodedHashes[i] {
			t.Fatalf("Decoded hash %d `%v` does not match expected `%v`",
				i, decodedHashes[i], expected)
		}
	}
}

func TestEncodeConcatenatedVoteBits(t *testing.T) {
	testVbs := []stake.VoteBits{
		stake.VoteBits{Bits: 0, ExtendedBits: []byte{}},
		stake.VoteBits{Bits: 0, ExtendedBits: []byte{0x00}},
		stake.VoteBits{Bits: 0x1223, ExtendedBits: []byte{0x01, 0x02, 0x03, 0x04}},
		stake.VoteBits{Bits: 0xaaaa, ExtendedBits: []byte{0x01, 0x02, 0x03, 0x04, 0x05}},
	}
	encodedResults, err := dcrjson.EncodeConcatenatedVoteBits(testVbs)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	expectedEncoded := []byte{
		0x02, 0x00, 0x00, 0x03,
		0x00, 0x00, 0x00, 0x06,
		0x23, 0x12, 0x01, 0x02,
		0x03, 0x04, 0x07, 0xaa,
		0xaa, 0x01, 0x02, 0x03,
		0x04, 0x05,
	}

	encodedResultsStr, _ := hex.DecodeString(encodedResults)
	if !bytes.Equal(expectedEncoded, encodedResultsStr) {
		t.Fatalf("Encoded votebits `%x` does not match expected `%x`",
			encodedResults, expectedEncoded)
	}

	// Test too long voteBits extended.
	testVbs = []stake.VoteBits{
		stake.VoteBits{Bits: 0, ExtendedBits: bytes.Repeat([]byte{0x00}, 74)},
	}
	_, err = dcrjson.EncodeConcatenatedVoteBits(testVbs)
	if err == nil {
		t.Fatalf("expected too long error")
	}
}

func TestDecodeConcatenatedVoteBits(t *testing.T) {
	encodedBytes := []byte{
		0x03, 0x00, 0x00, 0x00,
		0x06, 0x23, 0x12, 0x01,
		0x02, 0x03, 0x04, 0x07,
		0xaa, 0xaa, 0x01, 0x02,
		0x03, 0x04, 0x05,
	}
	encodedBytesStr := hex.EncodeToString(encodedBytes)

	expectedVbs := []stake.VoteBits{
		stake.VoteBits{Bits: 0, ExtendedBits: []byte{0x00}},
		stake.VoteBits{Bits: 0x1223, ExtendedBits: []byte{0x01, 0x02, 0x03, 0x04}},
		stake.VoteBits{Bits: 0xaaaa, ExtendedBits: []byte{0x01, 0x02, 0x03, 0x04, 0x05}},
	}

	decodedSlice, err :=
		dcrjson.DecodeConcatenatedVoteBits(encodedBytesStr)
	if err != nil {
		t.Fatalf("unexpected error decoding votebits: %v", err.Error())
	}

	if !reflect.DeepEqual(expectedVbs, decodedSlice) {
		t.Fatalf("Decoded votebits `%v` does not match expected `%v`",
			decodedSlice, expectedVbs)
	}

	// Test short read.
	encodedBytes = []byte{
		0x03, 0x00, 0x00, 0x00,
		0x06, 0x23, 0x12, 0x01,
		0x02, 0x03, 0x04, 0x07,
		0xaa, 0xaa, 0x01, 0x02,
		0x03, 0x04,
	}
	encodedBytesStr = hex.EncodeToString(encodedBytes)

	decodedSlice, err = dcrjson.DecodeConcatenatedVoteBits(encodedBytesStr)
	if err == nil {
		t.Fatalf("expected short read error")
	}

	// Test too long read.
	encodedBytes = []byte{
		0x03, 0x00, 0x00, 0x00,
		0x06, 0x23, 0x12, 0x01,
		0x02, 0x03, 0x04, 0x07,
		0xaa, 0xaa, 0x01, 0x02,
		0x03, 0x04, 0x05, 0x06,
	}
	encodedBytesStr = hex.EncodeToString(encodedBytes)

	decodedSlice, err = dcrjson.DecodeConcatenatedVoteBits(encodedBytesStr)
	if err == nil {
		t.Fatalf("expected corruption error")
	}

	// Test invalid length.
	encodedBytes = []byte{
		0x01, 0x00, 0x00, 0x00,
		0x06, 0x23, 0x12, 0x01,
		0x02, 0x03, 0x04, 0x07,
		0xaa, 0xaa, 0x01, 0x02,
		0x03, 0x04, 0x05, 0x06,
	}
	encodedBytesStr = hex.EncodeToString(encodedBytes)

	decodedSlice, err = dcrjson.DecodeConcatenatedVoteBits(encodedBytesStr)
	if err == nil {
		t.Fatalf("expected corruption error")
	}
}
