// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"
)

// testAddUnsigned ensures [addUnsigned] produces the expected results for the
// given supported unsigned type.  It uses generics to avoid repeating the tests
// for each type.
func testAddUnsigned[T uint16 | uint32 | uint64](t *testing.T, typ string) {
	t.Helper()

	var maxVal = ^T(0)
	tests := []struct {
		name string // test description
		a, b T      // unsigned vals to test
		sum  T      // expected sum
		ok   bool   // expected result
	}{
		// No overflow cases.
		{"zero", 0, 0, 0, true},
		{"max + zero", maxVal, 0, maxVal, true},
		{"zero + max", 0, maxVal, maxVal, true},
		{"small positive", 10, 20, 30, true},
		{"max edge", maxVal - 1, 1, maxVal, true},
		{"max edge rev", 1, maxVal - 1, maxVal, true},
		{"halfmax + halfmax", maxVal / 2, maxVal / 2, maxVal - 1, true},
		{"mid + halfmax", maxVal/2 + 1, maxVal / 2, maxVal, true},
		{"halfmax + mid", maxVal / 2, maxVal/2 + 1, maxVal, true},

		// Overflow cases.
		{"max + 1 overflow exact", maxVal, 1, 0, false},
		{"1 + max overflow exact", 1, maxVal, 0, false},
		{"small overflow", maxVal - 5, 6, 0, false},
		{"small overflow rev", 6, maxVal - 5, 0, false},
		{"mid overflow", maxVal/2 + 1, maxVal/2 + 1, 0, false},
		{"mid + max overflow", maxVal/2 + 1, maxVal, maxVal / 2, false},
		{"max + mid overflow", maxVal, maxVal/2 + 1, maxVal / 2, false},
	}

	for _, test := range tests {
		sum, ok := addUnsigned(test.a, test.b)
		if sum != test.sum || ok != test.ok {
			t.Errorf("%q (%s): unexpected result - got (%v, %v), want (%v, %v)",
				test.name, typ, sum, ok, test.sum, test.ok)
		}
	}
}

// TestAddUnsigned ensures [addUnsigned] produces the expected results for all
// three supported unsigned int types (uint16, uint32, uint64).
func TestAddUnsigned(t *testing.T) {
	testAddUnsigned[uint16](t, "uint16")
	testAddUnsigned[uint32](t, "uint32")
	testAddUnsigned[uint64](t, "uint64")
}

// testAddSigned ensures [addSigned] produces the expected results for the given
// supported signed type.  It uses generics to avoid repeating the tests for
// each type.
func testAddSigned[T int16 | int32 | int64](t *testing.T, typ string, maxVal T) {
	t.Helper()

	minVal := ^maxVal
	tests := []struct {
		name string // test description
		a, b T      // signed vals to test
		sum  T      // expected sum
		ok   bool   // expected result
	}{
		// No overflow or underflow cases.
		{"zero", 0, 0, 0, true},
		{"min + zero", minVal, 0, minVal, true},
		{"zero + min", 0, minVal, minVal, true},
		{"max + zero", maxVal, 0, maxVal, true},
		{"zero + max", 0, maxVal, maxVal, true},
		{"small positive", 10, 20, 30, true},
		{"small negative", -10, -20, -30, true},
		{"mixed small", 100, -50, 50, true},
		{"mixed small rev", -100, 50, -50, true},
		{"min edge", minVal + 1, -1, minVal, true},
		{"max edge", maxVal - 1, 1, maxVal, true},
		{"min + max", minVal, maxVal, -1, true},
		{"max + min", maxVal, minVal, -1, true},
		{"halfmin + halfmin", minVal / 2, minVal / 2, minVal, true},
		{"mid + min", maxVal/2 + 1, minVal, minVal / 2, true},
		{"min + mid", minVal, maxVal/2 + 1, minVal / 2, true},

		// Overflow cases.
		{"max + 1 overflow exact", maxVal, 1, minVal, false},
		{"1 + max overflow exact", 1, maxVal, minVal, false},
		{"small overflow", maxVal - 5, 6, minVal, false},
		{"small overflow rev", 6, maxVal - 5, minVal, false},
		{"pos to neg mid overflow", maxVal/2 + 1, maxVal/2 + 1, minVal, false},
		{"mid + max overflow", maxVal/2 + 1, maxVal, minVal/2 - 1, false},
		{"max + mid overflow", maxVal, maxVal/2 + 1, minVal/2 - 1, false},
		{"max + max overflow", maxVal, maxVal, -2, false},

		// Underflow cases.
		{"min + (-1) underflow exact", minVal, -1, maxVal, false},
		{"-1 + min underflow exact", -1, minVal, maxVal, false},
		{"small underflow", minVal + 5, -6, maxVal, false},
		{"small underflow rev", -6, minVal + 5, maxVal, false},
		{"neg to pos mid underflow", minVal/2 - 1, minVal / 2, maxVal, false},
		{"neg to pos mid underflow rev", minVal / 2, minVal/2 - 1, maxVal, false},
		{"halfmin + min underflow", minVal / 2, minVal, maxVal/2 + 1, false},
		{"min + halfmin underflow", minVal, minVal / 2, maxVal/2 + 1, false},
		{"min + min underflow", minVal, minVal, 0, false},
	}

	for _, test := range tests {
		sum, ok := addSigned(test.a, test.b)
		if sum != test.sum || ok != test.ok {
			t.Errorf("%q (%s): unexpected result - got (%v, %v), want (%v, %v)",
				test.name, typ, sum, ok, test.sum, test.ok)
		}
	}
}

// TestAddSigned ensures [addSigned] produces the expected results for all three
// supported signed int types (int16, int32, int64).
func TestAddSigned(t *testing.T) {
	testAddSigned[int16](t, "int16", 1<<15-1)
	testAddSigned[int32](t, "int32", 1<<31-1)
	testAddSigned[int64](t, "int64", 1<<63-1)
}
