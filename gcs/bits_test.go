// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gcs

import (
	"bytes"
	"encoding/hex"
	"io"
	"testing"
)

// TestBitWriter ensures the bit writer and all associated methods work as
// expected including all corner cases at byte boundaries.
func TestBitWriter(t *testing.T) {
	// perWriterTest describes a test to run against the same bitstream writer.
	type perWriterTest struct {
		name      string // test description
		bits      uint64 // bits to write one at a time
		nBits     uint   // number of individual bits to write
		val       uint64 // uint64 value from which to write nValBits
		nValBits  uint   // number of bits of the val to write
		wantBytes string // expected bytes in writer as of this test
	}

	tests := []struct {
		name           string          // test description
		perWriterTests []perWriterTest // tests to run against same writer
	}{{
		name: "1 bit - 0",
		perWriterTests: []perWriterTest{{
			name:      "1st bit",
			bits:      0x0,
			nBits:     1,
			wantBytes: "00",
		}},
	}, {
		name: "1 bit - 1",
		perWriterTests: []perWriterTest{{
			name:      "1st bit",
			bits:      0x1,
			nBits:     1,
			wantBytes: "80",
		}},
	}, {
		name: "2 bits - 00",
		perWriterTests: []perWriterTest{{
			name:      "1st and 2nd bits",
			bits:      0x0,
			nBits:     2,
			wantBytes: "00",
		}},
	}, {
		name: "2 bits - 01",
		perWriterTests: []perWriterTest{{
			name:      "1st and 2nd bits",
			bits:      0x1,
			nBits:     2,
			wantBytes: "40",
		}},
	}, {
		name: "9 bits - 010101010",
		perWriterTests: []perWriterTest{{
			name:      "first 9 bits",
			bits:      0x0aa,
			nBits:     9,
			wantBytes: "5500",
		}},
	}, {
		name: "9 bits - 010101011",
		perWriterTests: []perWriterTest{{
			name:      "first 9 bits",
			bits:      0x0ab,
			nBits:     9,
			wantBytes: "5580",
		}},
	}, {
		name: "2 bits, 3 bits of uint64 - 01 110",
		perWriterTests: []perWriterTest{{
			name:      "first 2 bits",
			bits:      0x1,
			nBits:     2,
			wantBytes: "40",
		}, {
			name:      "3 bits of uint64",
			val:       6,
			nValBits:  3,
			wantBytes: "70",
		}},
	}, {
		name: "7 bits, 2 bits of uint64 - 1111111 00",
		perWriterTests: []perWriterTest{{
			name:      "first 7 bits",
			bits:      0x7f,
			nBits:     7,
			wantBytes: "fe",
		}, {
			name:      "2 bits of uint64",
			val:       0,
			nValBits:  2,
			wantBytes: "fe00",
		}},
	}, {
		name: "7 bits, 11 bits of uint64 - 1111111 00100000101",
		perWriterTests: []perWriterTest{{
			name:      "first 7 bits",
			bits:      0x7f,
			nBits:     7,
			wantBytes: "fe",
		}, {
			name:      "11 bits of uint64",
			val:       261,
			nValBits:  11,
			wantBytes: "fe4140",
		}},
	}, {
		name: "7 bits of uint64, 2 bits  - 1010101 11",
		perWriterTests: []perWriterTest{{
			name:      "7 bits of uint64",
			val:       85,
			nValBits:  7,
			wantBytes: "aa",
		}, {
			name:      "last 2 bits",
			bits:      0x3,
			nBits:     2,
			wantBytes: "ab80",
		}},
	}}

nextTest:
	for _, test := range tests {
		var w bitWriter
		for _, pwTest := range test.perWriterTests {
			// Start by writing the specified individual bits one a time.
			bitSource := pwTest.bits
			for i := uint(0); i < pwTest.nBits; i++ {
				switch (bitSource >> (pwTest.nBits - 1 - i)) & 0x1 {
				case 0:
					w.writeZero()
				case 1:
					w.writeOne()
				}
			}

			// Write the specified number of bits from the uint64.
			w.writeNBits(pwTest.val, pwTest.nValBits)

			// Parse the expected serialized bytes and ensure they match.
			wantBytes, err := hex.DecodeString(pwTest.wantBytes)
			if err != nil {
				t.Errorf("%q-%q: unexpected err parsing want bytes hex: %v",
					test.name, pwTest.name, err)
				continue nextTest
			}
			resultBytes := w.bytes
			if !bytes.Equal(resultBytes, wantBytes) {
				t.Errorf("%q-%q: mismatched bytes -- got %x, want %x",
					test.name, pwTest.name, resultBytes, wantBytes)
				continue nextTest
			}
		}
	}
}

// TestBitReader ensures the bit reader and all associated methods work as
// expected including expected errors and corner cases at byte boundaries.
func TestBitReader(t *testing.T) {
	// perReaderTest describes a test to run against the same bitstream reader.
	type perReaderTest struct {
		name      string // test description
		doUnary   bool   // whether or not to perform a unary read
		wantUnary uint64 // expected number of consecutive ones
		unaryErr  error  // expected error on unary read
		nValBits  uint   // number of bits to read from bitstream as uint64
		wantVal   uint64 // expected value from nValBits read
		bitsErr   error  // expected error on bits read
	}

	tests := []struct {
		name           string          // test description
		bytes          string          // bytes to use as the bitstream
		perReaderTests []perReaderTest // tests to run against same reader
	}{{
		name:  "unary read on empty bytes error",
		bytes: "",
		perReaderTests: []perReaderTest{{
			name:      "unary read",
			doUnary:   true,
			wantUnary: 0,
			unaryErr:  io.EOF,
		}},
	}, {
		name:  "0 bits read on empty bytes (no error)",
		bytes: "",
		perReaderTests: []perReaderTest{{
			name:     "0 bit read",
			nValBits: 0,
			wantVal:  0,
		}},
	}, {
		name:  "1 bit read on empty bytes error",
		bytes: "",
		perReaderTests: []perReaderTest{{
			name:     "1 bit read",
			nValBits: 1,
			bitsErr:  io.EOF,
		}},
	}, {
		name:  "9 bit read on single byte error (straddle byte boundary)",
		bytes: "0f",
		perReaderTests: []perReaderTest{{
			name:     "9 bit read",
			nValBits: 9,
			bitsErr:  io.EOF,
		}},
	}, {
		name:  "16 bit read on single byte error (byte boundary)",
		bytes: "0f",
		perReaderTests: []perReaderTest{{
			name:     "16 bit read",
			nValBits: 16,
			bitsErr:  io.EOF,
		}},
	}, {
		name:  "0 bits followed by 8 bits ",
		bytes: "ff",
		perReaderTests: []perReaderTest{{
			name:     "0 bit read",
			nValBits: 0,
			wantVal:  0,
		}, {
			name:     "8 bit read",
			nValBits: 8,
			wantVal:  0xff,
		}},
	}, {
		name:  "unary 1",
		bytes: "80",
		perReaderTests: []perReaderTest{{
			name:      "first unary read",
			doUnary:   true,
			wantUnary: 1,
		}},
	}, {
		name:  "unary 2",
		bytes: "c0",
		perReaderTests: []perReaderTest{{
			name:      "first unary read",
			doUnary:   true,
			wantUnary: 2,
		}},
	}, {
		name:  "unary 9 (more than one byte)",
		bytes: "ff80",
		perReaderTests: []perReaderTest{{
			name:      "first unary read",
			doUnary:   true,
			wantUnary: 9,
		}},
	}, {
		name:  "unary 0, 1 bit read",
		bytes: "40",
		perReaderTests: []perReaderTest{{
			name:      "unary read",
			doUnary:   true,
			wantUnary: 0,
		}, {
			name:     "1 bit read",
			nValBits: 1,
			wantVal:  1,
		}},
	}, {
		name:  "unary 0, 8 bits read (straddle byte)",
		bytes: "5a80",
		perReaderTests: []perReaderTest{{
			name:      "unary read",
			doUnary:   true,
			wantUnary: 0,
		}, {
			name:     "8 bit read",
			nValBits: 8,
			wantVal:  0xb5,
		}},
	}, {
		name:  "unary 0, 15 bits read (byte boundary)",
		bytes: "5ac5",
		perReaderTests: []perReaderTest{{
			name:      "unary read",
			doUnary:   true,
			wantUnary: 0,
		}, {
			name:     "15 bit read",
			nValBits: 15,
			wantVal:  0x5ac5,
		}},
	}, {
		name:  "unary 0, 16 bits read (straddle 2nd byte boundary)",
		bytes: "5ac580",
		perReaderTests: []perReaderTest{{
			name:      "unary read",
			doUnary:   true,
			wantUnary: 0,
		}, {
			name:     "16 bit read",
			nValBits: 16,
			wantVal:  0xb58b,
		}},
	}, {
		name:  "unary 3, 15 bits read, unary 2",
		bytes: "eac518",
		perReaderTests: []perReaderTest{{
			name:      "first unary read",
			doUnary:   true,
			wantUnary: 3,
		}, {
			name:     "15 bit read",
			nValBits: 15,
			wantVal:  0x5628,
		}, {
			name:      "second unary read",
			doUnary:   true,
			wantUnary: 2,
		}},
	}}

nextTest:
	for _, test := range tests {
		// Parse the specified bytes to read and create a bitstream reader from
		// them.
		testBytes, err := hex.DecodeString(test.bytes)
		if err != nil {
			t.Errorf("%q: unexpected err parsing bytes hex: %v", test.name, err)
			continue
		}
		r := newBitReader(testBytes)

		for _, prTest := range test.perReaderTests {
			// Read unary and ensure expected result if requested.
			if prTest.doUnary {
				gotUnary, err := r.readUnary()
				if err != prTest.unaryErr {
					t.Errorf("%q-%q: unexpected unary err -- got %v, want %v",
						test.name, prTest.name, err, prTest.unaryErr)
					continue nextTest
				}
				if err != nil {
					continue nextTest
				}

				if gotUnary != prTest.wantUnary {
					t.Errorf("%q-%q: unexpected unary value -- got %x, want %x",
						test.name, prTest.name, gotUnary, prTest.wantUnary)
					continue nextTest
				}
			}

			// Read specified number of bits as uint64 and ensure expected
			// result.
			gotVal, err := r.readNBits(prTest.nValBits)
			if err != prTest.bitsErr {
				t.Errorf("%q-%q: unexpected nbits err -- got %v, want %v",
					test.name, prTest.name, err, prTest.bitsErr)
				continue nextTest
			}
			if err != nil {
				continue nextTest
			}
			if gotVal != prTest.wantVal {
				t.Errorf("%q-%q: mismatched value -- got %x, want %x",
					test.name, prTest.name, gotVal, prTest.wantVal)
				continue nextTest
			}
		}
	}
}
