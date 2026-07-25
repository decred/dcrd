// Copyright (c) 2026 The Decred developers
// Copyright (c) 2013-2026 Dave Collins
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package arith

import "testing"

// TestConstantTimeEq ensures [ConstantTimeEq] returns the expected results.
func TestConstantTimeEq(t *testing.T) {
	tests := []struct {
		a    uint32 // first value
		b    uint32 // second value
		want uint32 // expected result (0 or 1)
	}{
		{0, 0, 1},
		{1, 1, 1},
		{1<<32 - 1, 1<<32 - 1, 1},
		{0x12345678, 0x12345678, 1},
		{0, 1, 0},
		{1, 0, 0},
		{1<<32 - 1, 0, 0},
		{0x12345678, 0x87654321, 0},
		{0, 1 << 31, 0},
		{1 << 31, 1 << 31, 1},
		{1, 2, 0},
		{1<<32 - 2, 1<<32 - 1, 0},
	}

	for _, test := range tests {
		got := ConstantTimeEq(test.a, test.b)
		if got != test.want {
			t.Errorf("%08x == %08x: got %d, want %d", test.a, test.b, got,
				test.want)
		}
	}
}

// TestConstantTimeEq64 ensures [ConstantTimeEq64] returns the expected results.
func TestConstantTimeEq64(t *testing.T) {
	tests := []struct {
		a    uint64 // first value
		b    uint64 // second value
		want uint32 // expected result (0 or 1)
	}{
		{0, 0, 1},
		{1, 1, 1},
		{1<<64 - 1, 1<<64 - 1, 1},
		{0x123456789abcdef0, 0x123456789abcdef0, 1},
		{0, 1, 0},
		{1, 0, 0},
		{1<<64 - 1, 0, 0},
		{0x123456789abcdef0, 0xfedcba9876543210, 0},
		{0, 1 << 63, 0},
		{1 << 63, 1 << 63, 1},
		{1, 2, 0},
		{1<<64 - 2, 1<<64 - 1, 0},
	}

	for _, test := range tests {
		got := ConstantTimeEq64(test.a, test.b)
		if got != test.want {
			t.Errorf("%016x == %016x: got %d, want %d", test.a, test.b, got,
				test.want)
		}
	}
}

// TestConstantTimeNotEq64 ensures [ConstantTimeNotEq64] returns the expected
// results.
func TestConstantTimeNotEq64(t *testing.T) {
	tests := []struct {
		a    uint64 // first value
		b    uint64 // second value
		want uint32 // expected result
	}{
		{0, 0, 0},
		{1, 1, 0},
		{1<<64 - 1, 1<<64 - 1, 0},
		{0x123456789abcdef0, 0x123456789abcdef0, 0},
		{0, 1, 1},
		{1, 0, 1},
		{1<<64 - 1, 0, 1},
		{0x123456789abcdef0, 0xfedcba9876543210, 1},
		{0, 1 << 63, 1},
		{1 << 63, 1 << 63, 0},
		{1, 2, 1},
		{1<<64 - 2, 1<<64 - 1, 1},
	}

	for _, test := range tests {
		got := ConstantTimeNotEq64(test.a, test.b)
		if got != test.want {
			t.Errorf("%016x != %016x: got %d, want %d", test.a, test.b, got,
				test.want)
		}
	}
}

// TestConstantTimeLess ensures [ConstantTimeLess] returns the expected results.
func TestConstantTimeLess(t *testing.T) {
	tests := []struct {
		a    uint32 // first value
		b    uint32 // second value
		want uint32 // expected result (0 or 1)
	}{
		{0, 1, 1},
		{1, 2, 1},
		{0, 1<<32 - 1, 1},
		{1<<31 - 1, 1 << 31, 1},
		{1<<32 - 2, 1<<32 - 1, 1},
		{0x12345678, 0x87654321, 1},
		{1, 0, 0},
		{1, 1, 0},
		{1<<32 - 1, 0, 0},
		{1<<32 - 1, 1<<32 - 1, 0},
		{0x87654321, 0x12345678, 0},
		{0, 0, 0},
		{1 << 31, 1<<32 - 1, 1},
	}

	for _, test := range tests {
		got := ConstantTimeLess(test.a, test.b)
		if got != test.want {
			t.Errorf("%08x < %08x: got %d, want %d", test.a, test.b, got,
				test.want)
		}
	}
}

// TestConstantTimeLessOrEq ensures [ConstantTimeLessOrEq] returns the expected
// results.
func TestConstantTimeLessOrEq(t *testing.T) {
	tests := []struct {
		a    uint32 // first value
		b    uint32 // second value
		want uint32 // expected result (0 or 1)
	}{
		{0, 0, 1},
		{0, 1, 1},
		{1, 2, 1},
		{1, 1, 1},
		{0, 1<<32 - 1, 1},
		{0x12345678, 0x87654321, 1},
		{1<<32 - 1, 1<<32 - 1, 1},
		{1<<32 - 2, 1<<32 - 1, 1},
		{1, 0, 0},
		{2, 1, 0},
		{1<<32 - 1, 0, 0},
		{0x87654321, 0x12345678, 0},
		{1 << 31, 1<<32 - 1, 1},
		{1 << 31, 1 << 31, 1},
	}

	for _, test := range tests {
		got := ConstantTimeLessOrEq(test.a, test.b)
		if got != test.want {
			t.Errorf("%08x <= %08x: got %d, want %d", test.a, test.b, got,
				test.want)
		}
	}
}

// TestConstantTimeGreater ensures [ConstantTimeGreater] returns the expected
// results.
func TestConstantTimeGreater(t *testing.T) {
	tests := []struct {
		a    uint32 // first value
		b    uint32 // second value
		want uint32 // expected result (0 or 1)
	}{
		{1, 0, 1},
		{2, 1, 1},
		{1<<32 - 1, 0, 1},
		{1<<32 - 1, 1<<32 - 2, 1},
		{0x87654321, 0x12345678, 1},
		{0, 1, 0},
		{1, 1, 0},
		{0, 1<<32 - 1, 0},
		{1<<32 - 1, 1<<32 - 1, 0},
		{0x12345678, 0x87654321, 0},
		{0, 0, 0},
		{1<<32 - 1, 1 << 31, 1},
	}

	for _, test := range tests {
		got := ConstantTimeGreater(test.a, test.b)
		if got != test.want {
			t.Errorf("%08x > %08x: got %d, want %d", test.a, test.b, got,
				test.want)
		}
	}
}

// TestConstantTimeGreaterOrEq ensures [ConstantTimeGreaterOrEq] returns the
// expected results.
func TestConstantTimeGreaterOrEq(t *testing.T) {
	tests := []struct {
		a    uint32 // first value
		b    uint32 // second value
		want uint32 // expected result (0 or 1)
	}{
		{0, 0, 1},
		{1, 0, 1},
		{1, 1, 1},
		{2, 1, 1},
		{1<<32 - 1, 0, 1},
		{1<<32 - 1, 1<<32 - 1, 1},
		{1<<32 - 1, 1<<32 - 2, 1},
		{0x87654321, 0x12345678, 1},
		{0, 1, 0},
		{1, 2, 0},
		{0, 1<<32 - 1, 0},
		{0x12345678, 0x87654321, 0},
		{1 << 31, 1 << 31, 1},
		{1 << 31, 1<<32 - 1, 0},
	}

	for _, test := range tests {
		got := ConstantTimeGreaterOrEq(test.a, test.b)
		if got != test.want {
			t.Errorf("%08x >= %08x: got %d, want %d", test.a, test.b, got,
				test.want)
		}
	}
}

// TestConstantTimeMin ensures [ConstantTimeMin] returns the expected results.
func TestConstantTimeMin(t *testing.T) {
	tests := []struct {
		a    uint32 // first value
		b    uint32 // second value
		want uint32 // expected minimum value
	}{
		{0, 1, 0},
		{1, 2, 1},
		{0, 1<<32 - 1, 0},
		{1<<32 - 2, 1<<32 - 1, 1<<32 - 2},
		{0x12345678, 0x87654321, 0x12345678},
		{1, 0, 0},
		{2, 1, 1},
		{1<<32 - 1, 0, 0},
		{1<<32 - 1, 1<<32 - 2, 1<<32 - 2},
		{0x87654321, 0x12345678, 0x12345678},
		{0, 0, 0},
		{1, 1, 1},
		{1<<32 - 1, 1<<32 - 1, 1<<32 - 1},
		{0x12345678, 0x12345678, 0x12345678},
		{1 << 31, 1<<32 - 1, 1 << 31},
		{1<<32 - 1, 1 << 31, 1 << 31},
	}

	for _, test := range tests {
		got := ConstantTimeMin(test.a, test.b)
		if got != test.want {
			t.Errorf("min(%08x, %08x): got %08x, want %08x", test.a, test.b,
				got, test.want)
		}
	}
}

// TestConstantTimeSelect64 ensures [ConstantTimeSelect64] returns the expected
// results.
func TestConstantTimeSelect64(t *testing.T) {
	tests := []struct {
		cond uint64 // condition value
		a    uint64 // first value
		b    uint64 // second value
		want uint64 // expected selected value
	}{
		{0, 1, 2, 2},
		{1, 1, 2, 1},
		{0, 1<<64 - 1, 1, 1},
		{1, 1<<64 - 1, 1, 1<<64 - 1},
		{0, 1, 1<<64 - 1, 1<<64 - 1},
		{1, 1, 1<<64 - 1, 1},
		{0, 1<<64 - 1, 1<<64 - 2, 1<<64 - 2},
		{1, 1<<64 - 1, 1<<64 - 2, 1<<64 - 1},
		{0, 0x123456789abcdef0, 0x123456789abcdef0, 0x123456789abcdef0},
		{1, 0x123456789abcdef0, 0x123456789abcdef0, 0x123456789abcdef0},
	}

	for _, test := range tests {
		got := ConstantTimeSelect64(test.cond, test.a, test.b)
		if got != test.want {
			t.Errorf("sel(%d, %016x, %016x): got %016x, want %016x", test.cond,
				test.a, test.b, got, test.want)
		}
	}
}
