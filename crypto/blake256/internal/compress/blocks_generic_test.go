// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// Tests originally written by Dave Collins May 2020.  Additional cleanup and
// comments added July 2024.

package compress

import (
	"encoding/hex"
	"testing"
)

// hexTo64Bytes converts the passed hex string into bytes and will panic if
// there is an error.  This is only provided for the hard-coded constants so
// errors in the source code can be detected.  It will only (and must only) be
// called with hard-coded values.
func hexTo64Bytes(s string) [64]byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 64 {
		panic("invalid hex in source file: " + s)
	}
	return *(*[64]byte)(b)
}

// blockVecTest describes data to feed to the block compression function along
// with the expected chain value for both BLAKE-224 and BLAKE-256.  It's defined
// separately since it is used in multiple tests.
type blockVecTest struct {
	name string    // description
	h    [8]uint32 // chain value
	msg  [64]byte  // padded message
	s    [4]uint32 // 16-byte salt
	cnt  uint64    // total message bits hashed
	want [8]uint32 // chain value after 14 rounds
}

// blockVecs houses expected results from the block compression function for
// BLAKE-224 and BLAKE-256.  It includes the golden values from the
// specification.
var blockVecs = []blockVecTest{{
	name: "one-block message from specification for BLAKE-256",
	h: [8]uint32{
		0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a,
		0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
	},
	msg: hexTo64Bytes("00800000000000000000000000000000" +
		"00000000000000000000000000000000" +
		"00000000000000000000000000000000" +
		"00000000000000010000000000000008"),
	cnt: 8,
	want: [8]uint32{
		0x0ce8d4ef, 0x4dd7cd8d, 0x62dfded9, 0xd4edb0a7,
		0x74ae6a41, 0x929a74da, 0x23109e8f, 0x11139c87,
	},
}, {
	name: "two-block message from specification for BLAKE-256, first",
	h: [8]uint32{
		0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a,
		0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
	},
	msg: hexTo64Bytes("00000000000000000000000000000000" +
		"00000000000000000000000000000000" +
		"00000000000000000000000000000000" +
		"00000000000000000000000000000000"),
	cnt: 512,
	want: [8]uint32{
		0xb5bfb2f9, 0x14cfcc63, 0xb85c549c, 0xc9b4184e,
		0x67dfc6ce, 0x29e9904b, 0xd59ee74e, 0xfaa9c653,
	},
}, {
	name: "two-block message from specification for BLAKE-256, second",
	h: [8]uint32{
		0xb5bfb2f9, 0x14cfcc63, 0xb85c549c, 0xc9b4184e,
		0x67dfc6ce, 0x29e9904b, 0xd59ee74e, 0xfaa9c653,
	},
	msg: hexTo64Bytes("00000000000000008000000000000000" +
		"00000000000000000000000000000000" +
		"00000000000000000000000000000000" +
		"00000000000000010000000000000240"),
	cnt: 576,
	want: [8]uint32{
		0xd419bad3, 0x2d504fb7, 0xd44d460c, 0x42c5593f,
		0xe544fa4c, 0x135dec31, 0xe21bd9ab, 0xdcc22d41,
	},
}, {
	name: "one-block message from specification for BLAKE-224",
	h: [8]uint32{
		0xc1059ed8, 0x367cd507, 0x3070dd17, 0xf70e5939,
		0xffc00b31, 0x68581511, 0x64f98fa7, 0xbefa4fa4,
	},
	msg: hexTo64Bytes("00800000000000000000000000000000" +
		"00000000000000000000000000000000" +
		"00000000000000000000000000000000" +
		"00000000000000000000000000000008"),
	cnt: 8,
	want: [8]uint32{
		0x4504cb03, 0x14fb2a4f, 0x7a692e69, 0x6e487912,
		0xfe3f2468, 0xfe312c73, 0xa5278ec5, 0xc626599d,
	},
}, {
	name: "two-block message from specification for BLAKE-224, first",
	h: [8]uint32{
		0xc1059ed8, 0x367cd507, 0x3070dd17, 0xf70e5939,
		0xffc00b31, 0x68581511, 0x64f98fa7, 0xbefa4fa4,
	},
	msg: hexTo64Bytes("00000000000000000000000000000000" +
		"00000000000000000000000000000000" +
		"00000000000000000000000000000000" +
		"00000000000000000000000000000000"),
	cnt: 512,
	want: [8]uint32{
		0x176605a7, 0x569c689d, 0xa3ede776, 0x67093f69,
		0x7d51757d, 0x5f8fd329, 0x607c6b0c, 0x978312c4,
	},
}, {
	name: "two-block message from specification for BLAKE-224, second",
	h: [8]uint32{
		0x176605a7, 0x569c689d, 0xa3ede776, 0x67093f69,
		0x7d51757d, 0x5f8fd329, 0x607c6b0c, 0x978312c4,
	},
	msg: hexTo64Bytes("00000000000000008000000000000000" +
		"00000000000000000000000000000000" +
		"00000000000000000000000000000000" +
		"00000000000000000000000000000240"),
	cnt: 576,
	want: [8]uint32{
		0xf5aa00dd, 0x1cb847e3, 0x140372af, 0x7b5c46b4,
		0x888d82c8, 0xc0a91791, 0x3cfb5d04, 0x236a7cea,
	},
}}

// TestBlocks ensures the pure Go block compression function and whichever one
// is exported based on the current arch and supported asm extensions return the
// expected results.
func TestBlocks(t *testing.T) {
	t.Parallel()

	for i := range blockVecs {
		test := &blockVecs[i]

		// Ensure the internal pure Go block compression function returns the
		// expected results.
		state := State{CV: test.h, S: test.s}
		blocksGeneric(&state, test.msg[:], test.cnt)
		if state.CV != test.want {
			t.Fatalf("%q: unexpected result -- got %08x, want %08x", test.name,
				state.CV, test.want)
		}

		// Ensure whichever exported block compression function that is selected
		// based on the current arch and supported asm extensions returns the
		// expected results.
		state = State{CV: test.h, S: test.s}
		Blocks(&state, test.msg[:], test.cnt)
		if state.CV != test.want {
			t.Fatalf("%q: unexpected result -- got %08x, want %08x", test.name,
				state.CV, test.want)
		}
	}
}
