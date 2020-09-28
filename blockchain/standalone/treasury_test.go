// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package standalone

import (
	"errors"
	"testing"
)

func TestTSpendExpiryNegative(t *testing.T) {
	// 5 is not a valid start for a tvi of 11 with a mul of 3.
	_, err := CalculateTSpendWindowStart(5, 11, 3)
	if !errors.Is(err, ErrTSpendStartInvalidExpiry) {
		t.Fatalf("expected %v got %v",
			ErrTSpendStartInvalidExpiry, err)
	}

	// 5 is not a valid end for a tvi of 11.
	_, err = CalculateTSpendWindowEnd(5, 11)
	if !errors.Is(err, ErrTSpendEndInvalidExpiry) {
		t.Fatalf("expected %v got %v",
			ErrTSpendEndInvalidExpiry, err)
	}
}

func TestTSpendExpiry(t *testing.T) {
	tvi := uint64(288)
	mul := uint64(7)
	expiry := CalculateTSpendExpiry(2880, tvi, mul)
	if expiry != 5186 {
		t.Fatalf("got %v, expected 5186", expiry)
	}
	start, err := CalculateTSpendWindowStart(expiry, tvi, mul)
	if err != nil {
		t.Fatal(err)
	}
	end, err := CalculateTSpendWindowEnd(expiry, tvi)
	if err != nil {
		t.Fatal(err)
	}

	// Expect 3168 = expiry - (tvi*mul) - 2
	expectedStart := expiry - uint32(tvi*mul) - 2
	if start != expectedStart {
		t.Fatalf("got %v, expected %v", start, expectedStart)
	}

	// Expect 5184 = expiry - 2
	expectedEnd := expiry - 2
	if end != expectedEnd {
		t.Fatalf("got %v, expected %v", end, expectedEnd)
	}

	// Hard check numbers as well.
	if expectedStart != 3168 {
		t.Fatalf("got %v, expected %v", expectedStart, 3168)
	}
	if expectedEnd != 5184 {
		t.Fatalf("got %v, expected %v", expectedEnd, 5184)
	}
}
