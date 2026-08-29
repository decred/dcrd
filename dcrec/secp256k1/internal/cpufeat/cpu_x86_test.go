// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build (386 || amd64) && !purego

package cpufeat

import "testing"

// TestHasBit ensures that determining whether a specific bit is set in a 32-bit
// test value works as expected for edge cases, including the exact bit
// positions used to detect BMI2 and ADX.
func TestHasBit(t *testing.T) {
	tests := []struct {
		name    string // test description
		testVal uint32 // value to test
		bit     uint   // bit position to check
		want    bool   // expected result
	}{{
		name:    "bit 0 set",
		testVal: 0x00000001,
		bit:     0,
		want:    true,
	}, {
		name:    "bit 0 unset",
		testVal: 0x00000000,
		bit:     0,
		want:    false,
	}, {
		name:    "bit 8 set (BMI2 position in CPUID leaf 7 EBX)",
		testVal: 0x00000100,
		bit:     8,
		want:    true,
	}, {
		name:    "bit 8 unset with all other bits set",
		testVal: 0xfffffeff,
		bit:     8,
		want:    false,
	}, {
		name:    "bit 19 set (ADX position in CPUID leaf 7 EBX)",
		testVal: 0x00080000,
		bit:     19,
		want:    true,
	}, {
		name:    "bit 19 unset with all other bits set",
		testVal: 0xfff7ffff,
		bit:     19,
		want:    false,
	}, {
		name:    "bit 31 set (highest bit)",
		testVal: 0x80000000,
		bit:     31,
		want:    true,
	}, {
		name:    "bit 31 unset with all other bits set",
		testVal: 0x7fffffff,
		bit:     31,
		want:    false,
	}, {
		name:    "all bits set, check an arbitrary middle bit",
		testVal: 0xffffffff,
		bit:     15,
		want:    true,
	}, {
		name:    "no bits set, check an arbitrary middle bit",
		testVal: 0x00000000,
		bit:     15,
		want:    false,
	}}

	for _, test := range tests {
		got := hasBit(test.testVal, test.bit)
		if got != test.want {
			t.Errorf("%s: unexpected result -- got %v, want %v", test.name,
				got, test.want)
		}
	}
}

// TestSupportsCPUID ensures that querying whether the CPU supports the CPUID
// opcode returns an idempotent result across repeated calls.
//
// Note that this intentionally does not assert a specific value since the
// result is hardware dependent.  In practice, every CPU capable of running
// this compiled test binary supports CPUID, but the point of this test is to
// catch a broken implementation that behaves inconsistently rather than to
// assert a particular machine's capabilities.
func TestSupportsCPUID(t *testing.T) {
	got1 := supportsCPUID()
	got2 := supportsCPUID()
	if got1 != got2 {
		t.Fatalf("returned inconsistent results across calls -- got %v vs %v",
			got1, got2)
	}
}

// TestCPUID ensures that querying CPUID with the same inputs consistently
// returns the same outputs.
func TestCPUID(t *testing.T) {
	// The maximum supported input value leaf (EAX=0) is queried since it is the
	// first leaf queried by [detect] itself which makes it reasonable for
	// exercising the primitive directly.
	eax1, ebx1, ecx1, edx1 := cpuid(0, 0)
	eax2, ebx2, ecx2, edx2 := cpuid(0, 0)
	if eax1 != eax2 || ebx1 != ebx2 || ecx1 != ecx2 || edx1 != edx2 {
		t.Fatalf("cpuid returned inconsistent results across calls -- got "+
			"(%x,%x,%x,%x) vs (%x,%x,%x,%x)", eax1, ebx1, ecx1, edx1, eax2,
			ebx2, ecx2, edx2)
	}
}
