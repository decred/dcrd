// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package arith

import (
	"encoding/binary"
	"fmt"
	"math/big"
	mrand "math/rand"
	"testing"
	"time"
)

// mustBig converts the passed hex string into a big integer and will panic if
// there is an error.  This is only provided for the hard-coded constants so
// errors in the source code can be detected. It will only (and must only) be
// called with hard-coded values.
func mustBig(s string) *big.Int {
	val, ok := new(big.Int).SetString(s, 16)
	if !ok {
		panic("failed to parse big integer from hex: " + s)
	}
	return val
}

// mustBigToUint256 converts a [big.Int] to an array of 4 uint64s representing a
// 256-bit little-endian value.  It will panic if the big integer is larger than
// the maximum 256-bit value.  This is only provided for the hard-coded
// constants and randomly-generated values that are known to be in range, so
// errors in the source code can be detected.
func mustBigToUint256(v *big.Int) [4]uint64 {
	if v.BitLen() > 256 {
		panic(fmt.Sprintf("big integer %x is larger than max uint256", v))
	}

	var buf [32]byte
	v.FillBytes(buf[:])

	var result [4]uint64
	for i := 0; i < 4; i++ {
		result[i] = binary.BigEndian.Uint64(buf[32-((i+1)*8):])
	}
	return result
}

// uint512ToBig converts an array of 8 uint64s representing a 512-bit
// little-endian value to a [big.Int].
func uint512ToBig(v [8]uint64) *big.Int {
	var buf [64]byte
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint64(buf[i*8:], v[7-i])
	}

	return new(big.Int).SetBytes(buf[:])
}

// randBig returns a random 256-bit [big.Int] created from the passed rng.
func randBig(t *testing.T, rng *mrand.Rand) *big.Int {
	t.Helper()

	var buf [32]byte
	if _, err := rng.Read(buf[:]); err != nil {
		t.Fatalf("failed to read random: %v", err)
	}

	return new(big.Int).SetBytes(buf[:])
}

// TestMul512 ensures [Mul512] returns the expected result by comparing the
// product against the [big.Int] result.  It also tests commutativity and full
// buffer replacement.
func TestMul512(t *testing.T) {
	tests := []struct {
		name string // test description
		x, y string // hex encoded multiplicands
	}{{
		name: "all zero",
		x:    "0",
		y:    "0",
	}, {
		name: "zero * identity",
		x:    "0",
		y:    "1",
	}, {
		name: "zero * max uint256",
		x:    "0",
		y:    "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	}, {
		name: "identity",
		x:    "1",
		y:    "1",
	}, {
		name: "identity * max uint256",
		x:    "1",
		y:    "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	}, {
		name: "small values",
		x:    "2",
		y:    "3",
	}, {
		name: "unbalanced small values",
		x:    "1",
		y:    "ffffffffffffffff",
	}, {
		name: "small * max uint256",
		x:    "2",
		y:    "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	}, {
		name: "max uint64 * max uint64",
		x:    "ffffffffffffffff",
		y:    "ffffffffffffffff",
	}, {
		name: "max uint64 * 2*max uint64",
		x:    "ffffffffffffffff",
		y:    "1fffffffffffffffe",
	}, {
		name: "2^1 * 2^1",
		x:    "2",
		y:    "2",
	}, {
		name: "2^8 * 2^8",
		x:    "100",
		y:    "100",
	}, {
		name: "2^16 * 2^16",
		x:    "10000",
		y:    "10000",
	}, {
		name: "2^32 * 2^32",
		x:    "100000000",
		y:    "100000000",
	}, {
		name: "2^64 * 2^64",
		x:    "10000000000000000",
		y:    "10000000000000000",
	}, {
		name: "2^128 * 2^128",
		x:    "100000000000000000000000000000000",
		y:    "100000000000000000000000000000000",
	}, {
		name: "2^192 * 2^64",
		x:    "1000000000000000000000000000000000000000000000000",
		y:    "10000000000000000",
	}, {
		name: "2^255 * 2^255",
		x:    "8000000000000000000000000000000000000000000000000000000000000000",
		y:    "8000000000000000000000000000000000000000000000000000000000000000",
	}, {
		name: "max uint256 minus one",
		x:    "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe",
		y:    "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe",
	}, {
		name: "max uint256 * max uint256",
		x:    "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		y:    "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	}, {
		name: "alternating bits",
		x:    "a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5",
		y:    "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a",
	}, {
		name: "alternating bits 2",
		x:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		y:    "5555555555555555555555555555555555555555555555555555555555555555",
	}, {
		name: "carry propagation through zero limbs",
		x:    "ffffffffffffffffffffffffffffffff00000000000000000000000000000000",
		y:    "100000000000000000000000000000001",
	}}

	for _, test := range tests {
		xBig, yBig := mustBig(test.x), mustBig(test.y)
		x, y := mustBigToUint256(xBig), mustBigToUint256(yBig)
		want := new(big.Int).Mul(xBig, yBig)

		// Fill the array that will be used to store the product with non-zero
		// values to ensure the entire array is overwritten.
		var product [8]uint64
		for i := 0; i < 8; i++ {
			product[i] = 0xffffffffffffffff
		}

		// Compute the product with [Mul512] and ensure it matches the same
		// result produced by [big.Int].
		Mul512(&product, &x, &y)
		if got := uint512ToBig(product); got.Cmp(want) != 0 {
			t.Errorf("%s: incorrect product\n  x: %064x\n  y: %064x\n"+
				"got:  %0128x\nwant: %0128x", test.name, xBig, yBig, got, want)
		}

		// Ensure commutativity works properly with [Mul512] by computing the
		// product again with the operands swapped.  Poison the array used to
		// store the product again to ensure the opposite order overwrites the
		// entire array too.
		for i := 0; i < 8; i++ {
			product[i] = 0xffffffffffffffff
		}
		Mul512(&product, &y, &x)
		if got := uint512ToBig(product); got.Cmp(want) != 0 {
			t.Errorf("%s (swapped): incorrect product\n  y: %064x\n  x: %064x\n"+
				"got:  %0128x\nwant: %0128x", test.name, yBig, xBig, got, want)
		}
	}
}

// TestMul512Random ensures that multiplying randomly-generated values via
// [Mul512] returns the expected result by comparing the product against the
// [big.Int] result.  It also tests commutativity.
func TestMul512Random(t *testing.T) {
	// Use a unique random seed each test instance and log it if the tests fail.
	seed := time.Now().UnixNano()
	rng := mrand.New(mrand.NewSource(seed))
	defer func(t *testing.T, seed int64) {
		if t.Failed() {
			t.Logf("random seed: %d", seed)
		}
	}(t, seed)

	for i := 0; i < 1000; i++ {
		// Generate random [big.Int] operands and the expected product.
		xBig, yBig := randBig(t, rng), randBig(t, rng)
		x, y := mustBigToUint256(xBig), mustBigToUint256(yBig)
		want := new(big.Int).Mul(xBig, yBig)

		// Compute the product with [Mul512] and ensure it matches the same
		// result produced by [big.Int].
		var product [8]uint64
		Mul512(&product, &x, &y)
		if got := uint512ToBig(product); got.Cmp(want) != 0 {
			t.Errorf("incorrect product\n  x: %064x\n  y: %064x\ngot:  %0128x\n"+
				"want: %0128x", xBig, yBig, got, want)
		}

		// Ensure commutativity works properly with [Mul512] by computing the
		// product again with the operands swapped.
		var product2 [8]uint64
		Mul512(&product2, &y, &x)
		if got := uint512ToBig(product2); got.Cmp(want) != 0 {
			t.Errorf("incorrect product\n  y: %064x\n  x: %064x\ngot:  %0128x\n"+
				"want: %0128x", yBig, xBig, got, want)
		}
	}
}

// TestSquare512 ensures [Square512] returns the expected result by comparing it
// against the [big.Int] result.  It also tests full buffer replacement and that
// [Mul512] and [Square512] agree on the result.
func TestSquare512(t *testing.T) {
	tests := []struct {
		name string // test description
		a    string // hex encoded value to square
	}{{
		name: "zero",
		a:    "0",
	}, {
		name: "identity",
		a:    "1",
	}, {
		name: "small value",
		a:    "2",
	}, {
		name: "2^4",
		a:    "10",
	}, {
		name: "2^8",
		a:    "100",
	}, {
		name: "2^16",
		a:    "10000",
	}, {
		name: "2^32",
		a:    "100000000",
	}, {
		name: "max uint64",
		a:    "ffffffffffffffff",
	}, {
		name: "2^64",
		a:    "10000000000000000",
	}, {
		name: "max uint128 minus one",
		a:    "fffffffffffffffffffffffffffffffe",
	}, {
		name: "max uint128",
		a:    "ffffffffffffffffffffffffffffffff",
	}, {
		name: "2^128",
		a:    "100000000000000000000000000000000",
	}, {
		name: "max uint192",
		a:    "ffffffffffffffffffffffffffffffffffffffffffffffff",
	}, {
		name: "2^192",
		a:    "1000000000000000000000000000000000000000000000000",
	}, {
		name: "2^255",
		a:    "8000000000000000000000000000000000000000000000000000000000000000",
	}, {
		name: "near max uint256",
		a:    "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff0001",
	}, {
		name: "max uint256 minus one",
		a:    "fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe",
	}, {
		name: "max uint256",
		a:    "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	}, {
		name: "sparse bits",
		a:    "8000000000000000000000000000000100000000000000000000000000000001",
	}, {
		name: "high and low bits",
		a:    "8000000000000000000000000000000000000000000000000000000000000001",
	}, {
		name: "alternating bits",
		a:    "a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5",
	}, {
		name: "alternating bits 2",
		a:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}, {
		name: "alternating bits 3",
		a:    "5555555555555555555555555555555555555555555555555555555555555555",
	}, {
		name: "carry propagation through zero limbs",
		a:    "ffffffffffffffffffffffffffffffff00000000000000000000000000000001",
	}}

	for _, test := range tests {
		aBig := mustBig(test.a)
		a := mustBigToUint256(aBig)
		want := new(big.Int).Mul(aBig, aBig)

		// Fill the array that will be used to store the result with non-zero
		// values to ensure the entire array is overwritten.
		var result [8]uint64
		for i := 0; i < 8; i++ {
			result[i] = 0xffffffffffffffff
		}

		// Compute the result with [Square512] and ensure it matches the same
		// result produced by [big.Int].
		Square512(&result, &a)
		if got := uint512ToBig(result); got.Cmp(want) != 0 {
			t.Errorf("%s: incorrect square\n  a:    %064x\n  got:  %0128x\n"+
				"want: %0128x", test.name, aBig, got, want)
		}

		// Ensure [Mul512] and [Square512] agree.
		var mulResult [8]uint64
		Mul512(&mulResult, &a, &a)
		if result != mulResult {
			t.Errorf("%s: square and mul disagree\n  a:       %064x\n"+
				"  square:  %0128x\n  mul:     %0128x", test.name, aBig,
				uint512ToBig(result), uint512ToBig(mulResult))
		}
	}
}

// TestSquare512Random ensures that squaring randomly-generated values via
// [Square512] returns the expected result by comparing it against the [big.Int]
// result.  It also tests that [Mul512] and [Square512] agree on the result.
func TestSquare512Random(t *testing.T) {
	// Use a unique random seed each test instance and log it if the tests fail.
	seed := time.Now().UnixNano()
	rng := mrand.New(mrand.NewSource(seed))
	defer func(t *testing.T, seed int64) {
		if t.Failed() {
			t.Logf("random seed: %d", seed)
		}
	}(t, seed)

	for i := 0; i < 1000; i++ {
		// Generate a random [big.Int] operand and the expected result.
		aBig := randBig(t, rng)
		a := mustBigToUint256(aBig)
		want := new(big.Int).Mul(aBig, aBig)

		// Compute the result with [Square512] and ensure it matches the same
		// result produced by [big.Int].
		var result [8]uint64
		Square512(&result, &a)
		if got := uint512ToBig(result); got.Cmp(want) != 0 {
			t.Errorf("incorrect square\n  a:    %064x\n  got:  %0128x\n"+
				"want: %0128x", aBig, got, want)
		}

		// Ensure [Mul512] and [Square512] agree.
		var mulResult [8]uint64
		Mul512(&mulResult, &a, &a)
		if result != mulResult {
			t.Errorf("square and mul disagree\n  a:       %064x\n"+
				"  square:  %0128x\n  mul:     %0128x", aBig,
				uint512ToBig(result), uint512ToBig(mulResult))
		}
	}
}
