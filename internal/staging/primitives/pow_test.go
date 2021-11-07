// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package primitives

import (
	"encoding/hex"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/internal/staging/primitives/uint256"
)

// hexToBytes converts the passed hex string into bytes and will panic if there
// is an error.  This is only provided for the hard-coded constants so errors in
// the source code can be detected. It will only (and must only) be called with
// hard-coded values.
func hexToBytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	return b
}

// hexToUint256 converts the passed hex string into a Uint256 and will panic if
// there is an error.  This is only provided for the hard-coded constants so
// errors in the source code can be detected. It will only (and must only) be
// called with hard-coded values.
func hexToUint256(s string) *uint256.Uint256 {
	if len(s)%2 != 0 {
		s = "0" + s
	}
	b := hexToBytes(s)
	if len(b) > 32 {
		panic("hex in source file overflows mod 2^256: " + s)
	}
	return new(uint256.Uint256).SetByteSlice(b)
}

// TestDiffBitsToUint256 ensures converting from the compact representation used
// for target difficulties to unsigned 256-bit integers produces the correct
// results.
func TestDiffBitsToUint256(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string // test description
		input     uint32 // compact target difficulty bits to test
		want      string // expected uint256
		neg       bool   // expect result to be a negative number
		overflows bool   // expect result to overflow
	}{{
		name:  "mainnet block 1",
		input: 0x1b01ffff,
		want:  "000000000001ffff000000000000000000000000000000000000000000000000",
	}, {
		name:  "mainnet block 288",
		input: 0x1b01330e,
		want:  "000000000001330e000000000000000000000000000000000000000000000000",
	}, {
		name:  "higher diff (exponent 24, sign bit 0, mantissa 0x5fb28a)",
		input: 0x185fb28a,
		want:  "00000000000000005fb28a000000000000000000000000000000000000000000",
	}, {
		name:  "zero",
		input: 0,
		want:  "00",
	}, {
		name:  "-1 (exponent 1, sign bit 1, mantissa 0x10000)",
		input: 0x1810000,
		want:  "01",
		neg:   true,
	}, {
		name:  "-128 (exponent 2, sign bit 1, mantissa 0x08000)",
		input: 0x2808000,
		want:  "80",
		neg:   true,
	}, {
		name:  "-32768 (exponent 3, sign bit 1, mantissa 0x08000)",
		input: 0x3808000,
		want:  "8000",
		neg:   true,
	}, {
		name:  "-8388608 (exponent 4, sign bit 1, mantissa 0x08000)",
		input: 0x4808000,
		want:  "800000",
		neg:   true,
	}, {
		name:      "max uint256 + 1 via exponent 33 (overflows)",
		input:     0x21010000,
		want:      "00",
		neg:       false,
		overflows: true,
	}, {
		name:      "negative max uint256 + 1 (negative and overflows)",
		input:     0x21810000,
		want:      "00",
		neg:       true,
		overflows: true,
	}, {
		name:      "max uint256 + 1 via exponent 34 (overflows)",
		input:     0x22000100,
		want:      "00",
		neg:       false,
		overflows: true,
	}, {
		name:      "max uint256 + 1 via exponent 35 (overflows)",
		input:     0x23000001,
		want:      "00",
		neg:       false,
		overflows: true,
	}}

	for _, test := range tests {
		want := hexToUint256(test.want)

		result, isNegative, overflows := DiffBitsToUint256(test.input)
		if result.Cmp(want) != 0 {
			t.Errorf("%q: mismatched result -- got %x, want %x", test.name,
				result, want)
			continue
		}
		if isNegative != test.neg {
			t.Errorf("%q: mismatched negative -- got %v, want %v", test.name,
				isNegative, test.neg)
			continue
		}
		if overflows != test.overflows {
			t.Errorf("%q: mismatched overflows -- got %v, want %v", test.name,
				overflows, test.overflows)
			continue
		}
	}
}

// TestUint256ToDiffBits ensures converting from unsigned 256-bit integers to
// the representation used for target difficulties in the header bits field
// produces the correct results.
func TestUint256ToDiffBits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string // test description
		input string // uint256 to test
		neg   bool   // treat as a negative number
		want  uint32 // expected encoded value
	}{{
		name:  "mainnet block 1",
		input: "000000000001ffff000000000000000000000000000000000000000000000000",
		want:  0x1b01ffff,
	}, {
		name:  "mainnet block 288",
		input: "000000000001330e000000000000000000000000000000000000000000000000",
		want:  0x1b01330e,
	}, {
		name:  "higher diff (exponent 24, sign bit 0, mantissa 0x5fb28a)",
		input: "00000000000000005fb28a000000000000000000000000000000000000000000",
		want:  0x185fb28a,
	}, {
		name:  "zero",
		input: "00",
		want:  0,
	}, {
		name:  "negative zero is zero",
		input: "00",
		neg:   true,
		want:  0,
	}, {
		name:  "-1 (exponent 1, sign bit 1, mantissa 0x10000)",
		input: "01",
		neg:   true,
		want:  0x1810000,
	}, {
		name:  "-128 (exponent 2, sign bit 1, mantissa 0x08000)",
		input: "80",
		neg:   true,
		want:  0x2808000,
	}, {
		name:  "-32768 (exponent 3, sign bit 1, mantissa 0x08000)",
		input: "8000",
		neg:   true,
		want:  0x3808000,
	}, {
		name:  "-8388608 (exponent 4, sign bit 1, mantissa 0x08000)",
		input: "800000",
		neg:   true,
		want:  0x4808000,
	}}

	for _, test := range tests {
		input := hexToUint256(test.input)

		// Either use the internal function or the exported function depending
		// on whether or not the test is for negative inputs.  This is done in
		// order to ensure both funcs are tested and because only the internal
		// one accepts a flag to specify the value should be treated as
		// negative.
		var result uint32
		if test.neg {
			result = uint256ToDiffBits(input, true)
		} else {
			result = Uint256ToDiffBits(input)
		}
		if result != test.want {
			t.Errorf("%q: mismatched result -- got %x, want %x", test.name,
				result, test.want)
			continue
		}
	}
}

// TestCalcWork ensures calculating a work value from a compact target
// difficulty produces the correct results.
func TestCalcWork(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string // test description
		input uint32 // target difficulty bits to test
		want  string // expected uint256
	}{{
		name:  "mainnet block 1",
		input: 0x1b01ffff,
		want:  "0000000000000000000000000000000000000000000000000000800040002000",
	}, {
		name:  "mainnet block 288",
		input: 0x1b01330e,
		want:  "0000000000000000000000000000000000000000000000000000d56f2dcbe105",
	}, {
		name:  "higher diff (exponent 24)",
		input: 0x185fb28a,
		want:  "000000000000000000000000000000000000000000000002acd33ddd458512da",
	}, {
		name:  "zero",
		input: 0,
		want:  "0000000000000000000000000000000000000000000000000000000000000000",
	}, {
		name:  "max uint256",
		input: 0x2100ffff,
		want:  "0000000000000000000000000000000000000000000000000000000000000001",
	}, {
		name:  "negative target difficulty",
		input: 0x1810000,
		want:  "0000000000000000000000000000000000000000000000000000000000000000",
	}}

	for _, test := range tests {
		want := hexToUint256(test.want)
		result := CalcWork(test.input)
		if !result.Eq(want) {
			t.Errorf("%q: mismatched result -- got %x, want %x", test.name,
				result, want)
			continue
		}
	}
}

// TestHashToUint256 ensures converting a hash treated as a little endian
// unsigned 256-bit value to a uint256 works as intended.
func TestHashToUint256(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string // test description
		hash string // hash to convert
		want string // expected uint256 bytes in hex
	}{{
		name: "mainnet block 1 hash",
		hash: "000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9",
		want: "000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9",
	}, {
		name: "mainnet block 2 hash",
		hash: "000000000000c41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba",
		want: "000000000000c41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba",
	}}

	for _, test := range tests {
		hash, err := chainhash.NewHashFromStr(test.hash)
		if err != nil {
			t.Errorf("%q: unexpected err parsing test hash: %v", test.name, err)
			continue
		}
		want := hexToUint256(test.want)

		result := HashToUint256(hash)
		if !result.Eq(want) {
			t.Errorf("%s: unexpected result -- got %x, want %x", test.name,
				result, want)
			continue
		}
	}
}
