// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package apbf

import (
	"encoding/binary"
	"math"
	"testing"
)

// TestFilterMembership ensures that the most recent items added to the filter
// up to its capacity are always reported (zero false negatives) and that aging
// the filter expires entries as expected.
func TestFilterMembership(t *testing.T) {
	tests := []struct {
		name     string  // test description
		capacity uint32  // target capacity
		fpRate   float64 // target false positive rate
	}{{
		name:     "capacity 100, fpRate 0.1",
		capacity: 100,
		fpRate:   0.1,
	}, {
		name:     "capacity 100, fpRate 0.01",
		capacity: 100,
		fpRate:   0.01,
	}, {
		name:     "capacity 100, fpRate 0.001",
		capacity: 100,
		fpRate:   0.001,
	}, {
		name:     "capacity 1000, fpRate 0.1",
		capacity: 1000,
		fpRate:   0.1,
	}}

nextTest:
	for _, test := range tests {
		// Add twice the capacity of the filter to also test expiration and
		// absence of false negatives.
		const capacityMul = 2
		var data [4]byte
		filter := NewFilter(test.capacity, test.fpRate)
		actualCapacity := filter.Capacity()
		for i := uint32(0); i < actualCapacity*capacityMul; i++ {
			binary.BigEndian.PutUint32(data[:], i)
			filter.Add(data[:])

			// Ensure the item that was just added is in the filter.
			if !filter.Contains(data[:]) {
				t.Errorf("%q: filter missing expected data %x", test.name, data)
				continue nextTest
			}
		}

		// Ensure there are no false negatives for the most recent items up to
		// the filter capacity.
		startVal := actualCapacity * (capacityMul - 1)
		for i := startVal; i < actualCapacity*capacityMul; i++ {
			binary.BigEndian.PutUint32(data[:], i)
			if !filter.Contains(data[:]) {
				t.Errorf("%q: filter missing expected data %x", test.name, data)
				continue nextTest
			}
		}

		// Force all items to be expired and ensure they are gone while
		// accounting for the possibility of false positives even though there
		// will typically be 0 since the filter will be totally clear.
		var numFP uint32
		for i := uint8(0); i <= filter.L(); i++ {
			filter.nextGeneration()
		}
		for i := uint32(0); i < actualCapacity*capacityMul; i++ {
			binary.BigEndian.PutUint32(data[:], i)
			if filter.Contains(data[:]) {
				numFP++
			}
		}
		maxItemsPerFP := uint32(math.Ceil(1 / filter.FPRate()))
		maxFP := (actualCapacity * capacityMul) / maxItemsPerFP
		if numFP > maxFP {
			t.Errorf("%q: items not expired -- max expected fp %d, got %d",
				test.name, maxFP, numFP)
			continue
		}
	}
}

// TestFalsePositives ensures that filters of various capacities and false
// positive rates do not produce more false positives than the expected rate
// after exceeding their capacity several times over.
func TestFalsePositives(t *testing.T) {
	tests := []struct {
		name     string  // test description
		capacity uint32  // target capacity
		fpRate   float64 // target false positive rate
	}{{
		name:     "capacity 1000, fpRate 0.1",
		capacity: 1000,
		fpRate:   0.1,
	}, {
		name:     "capacity 1000, fpRate 0.01",
		capacity: 1000,
		fpRate:   0.01,
	}, {
		name:     "capacity 1000, fpRate 0.001",
		capacity: 1000,
		fpRate:   0.001,
	}, {
		name:     "capacity 10000, fpRate 0.1",
		capacity: 10000,
		fpRate:   0.1,
	}}

nextTest:
	for _, test := range tests {
		// Add several times the capacity of the filter.
		const capacityMul = 4
		var data [4]byte
		filter := NewFilter(test.capacity, test.fpRate)
		numToAdd := test.capacity * capacityMul
		for i := uint32(0); i < numToAdd; i++ {
			binary.BigEndian.PutUint32(data[:], i)
			filter.Add(data[:])

			// Ensure the item that was just added is in the filter.
			if !filter.Contains(data[:]) {
				t.Errorf("%q: filter missing expected data %x", test.name, data)
				continue nextTest
			}
		}

		// Test for items that were never added to the filter at 100 times the
		// false positive rate and expect a max of one and half that number to
		// ensure the rate is not significantly worse than expected and
		// stablizes around the expected rate.
		const fpRateMul = 100
		var numFP uint32
		numToProbe := uint32(math.Floor(1/test.fpRate) * fpRateMul)
		for i := uint32(0); i < numToProbe; i++ {
			val := i + numToAdd
			binary.BigEndian.PutUint32(data[:], val)
			if filter.Contains(data[:]) {
				numFP++
			}
		}
		maxFP := uint32(math.Ceil(fpRateMul * 1.5))
		if numFP > maxFP {
			t.Errorf("%q: expected a maximum of %d false positives, got %d",
				test.name, maxFP, numFP)
			continue
		}
	}
}

// TestFilterTargetCapacity ensures filters are created with a max capacity that
// is at least the target capacity.
func TestFilterTargetCapacity(t *testing.T) {
	tests := []struct {
		name    string  // test description
		filter  *Filter // filter to test with
		wantCap uint32  // expected actual capacity
	}{{
		name:    "capacity 100, fpRate 0.1",
		filter:  NewFilter(100, 0.1),
		wantCap: 102,
	}, {
		name:    "capacity 100, fpRate 0.01",
		filter:  NewFilter(100, 0.01),
		wantCap: 100,
	}, {
		name:    "capacity 100, fpRate 0.001",
		filter:  NewFilter(100, 0.001),
		wantCap: 105,
	}, {
		name:    "capacity 1000, fpRate 0.1",
		filter:  NewFilter(1000, 0.1),
		wantCap: 1002,
	}, {
		name:    "capacity 250, K 20, L 14",
		filter:  NewFilterKL(250, 20, 14),
		wantCap: 255,
	}, {
		name:    "capacity 500, K 17, L 12",
		filter:  NewFilterKL(500, 17, 12),
		wantCap: 507,
	}}

	for _, test := range tests {
		if gotCap := test.filter.Capacity(); gotCap != test.wantCap {
			t.Errorf("%q: unexpected capacity -- got %d, want %d", test.name,
				gotCap, test.wantCap)
			continue
		}
	}
}

// TestFilterKL ensures filters are created with the expected K and L params and
// the associated methods return them as expected.
func TestFilterKL(t *testing.T) {
	tests := []struct {
		name   string  // test description
		filter *Filter // filter to test with
		wantK  uint8   // expected K param
		wantL  uint8   // expected L param
	}{{
		name:   "capacity 100, fpRate 0.1",
		filter: NewFilter(100, 0.1),
		wantK:  4,
		wantL:  2,
	}, {
		name:   "capacity 100, fpRate 0.01",
		filter: NewFilter(100, 0.01),
		wantK:  7,
		wantL:  4,
	}, {
		name:   "capacity 100, fpRate 0.001",
		filter: NewFilter(100, 0.001),
		wantK:  10,
		wantL:  6,
	}, {
		name:   "capacity 1000, fpRate 0.1",
		filter: NewFilter(1000, 0.1),
		wantK:  4,
		wantL:  2,
	}, {
		name:   "capacity 250, K 20, L 14",
		filter: NewFilterKL(250, 20, 14),
		wantK:  20,
		wantL:  14,
	}, {
		name:   "capacity 500, K 17, L 12",
		filter: NewFilterKL(500, 17, 12),
		wantK:  17,
		wantL:  12,
	}}

	for _, test := range tests {
		if gotK := test.filter.K(); gotK != test.wantK {
			t.Errorf("%q: unexpected K param -- got %d, want %d", test.name,
				gotK, test.wantK)
			continue
		}

		if gotL := test.filter.L(); gotL != test.wantL {
			t.Errorf("%q: unexpected L param -- got %d, want %d", test.name,
				gotL, test.wantL)
			continue
		}
	}
}

// TestFilterSize ensures the filter size reports the expected values for a
// variety of target capacities and false positive rates.
func TestFilterSize(t *testing.T) {
	tests := []struct {
		name     string  // test description
		capacity uint32  // target capacity
		fpRate   float64 // target false positive rate
		want     int     // expected size
	}{{
		name:     "capacity 1000, fpRate 0.1",
		capacity: 1000,
		fpRate:   0.1,
		want:     1516,
	}, {
		name:     "capacity 1000, fpRate 0.01",
		capacity: 1000,
		fpRate:   0.01,
		want:     2848,
	}, {
		name:     "capacity 1000, fpRate 0.001",
		capacity: 1000,
		fpRate:   0.001,
		want:     4198,
	}, {
		name:     "capacity 1000, fpRate 0.0001",
		capacity: 1000,
		fpRate:   0.0001,
		want:     5374,
	}, {
		name:     "capacity 10000, fpRate 0.1",
		capacity: 10000,
		fpRate:   0.1,
		want:     14500,
	}, {
		name:     "capacity 10000, fpRate 0.01",
		capacity: 10000,
		fpRate:   0.01,
		want:     27843,
	}, {
		name:     "capacity 10000, fpRate 0.001",
		capacity: 10000,
		fpRate:   0.001,
		want:     41304,
	}, {
		name:     "capacity 10000, fpRate 0.0001",
		capacity: 10000,
		fpRate:   0.0001,
		want:     52711,
	}}

	for _, test := range tests {
		filter := NewFilter(test.capacity, test.fpRate)
		if gotSize := filter.Size(); gotSize != test.want {
			t.Errorf("%q: unexpected size -- got %d, want %d", test.name,
				gotSize, test.want)
		}
	}
}

// TestReset ensures resetting a filter resets all state and changes the hash
// key without disturbing the capacity or false positive rate.
func TestReset(t *testing.T) {
	tests := []struct {
		name     string  // test description
		capacity uint32  // target capacity
		fpRate   float64 // target false positive rate
	}{{
		name:     "capacity 10, fpRate 0.1",
		capacity: 10,
		fpRate:   0.1,
	}, {
		name:     "capacity 20, fpRate 0.01",
		capacity: 20,
		fpRate:   0.01,
	}}

	for _, test := range tests {
		// Create the filter per the test params and populate a bunch of data.
		f := NewFilter(test.capacity, test.fpRate)
		oldKey0, oldKey1 := f.key0, f.key1
		oldK, oldL := f.K(), f.L()
		oldCapacity, oldFPRate := f.Capacity(), f.FPRate()
		oldSize := f.Size()

		var data [8]byte
		for i := 0; i < 1000; i++ {
			binary.LittleEndian.PutUint64(data[:], uint64(i))
			f.Add(data[:])
		}

		// Reset the filter and ensure all of the expected fields are cleared.
		f.Reset()
		if f.baseIndex != 0 {
			t.Errorf("%q: base index is not zero -- got %d", test.name,
				f.baseIndex)
			continue
		}
		if f.itemsInCurGeneration != 0 {
			t.Errorf("%q: current gen items is not zero -- got %d", test.name,
				f.itemsInCurGeneration)
			continue
		}
		for i, b := range f.data {
			if b != 0 {
				t.Errorf("%q: data at index %d is not zero -- got %d",
					test.name, i, b)
			}
		}

		// Ensure the key is reset.
		if f.key0 == oldKey0 || f.key1 == oldKey1 {
			t.Errorf("%q: unchanged key -- old (%d, %d), new (%d, %d)",
				test.name, oldKey0, oldKey1, f.key0, f.key1)
		}

		// Ensure the K and L parameters are the same.
		newK, newL := f.K(), f.L()
		if newK != oldK {
			t.Errorf("%q: K param mismatch -- got %d, want %d", test.name, newK,
				oldK)
		}
		if newL != oldL {
			t.Errorf("%q: L param mismatch -- got %d, want %d", test.name, newL,
				oldL)
		}

		// Ensure the capacity and false positive rate are the same.
		newCapacity, newFPRate := f.Capacity(), f.FPRate()
		if newCapacity != oldCapacity {
			t.Errorf("%q: capacity mismatch -- got %d, want %d", test.name,
				newCapacity, oldCapacity)
		}
		if newFPRate != oldFPRate {
			t.Errorf("%q: false positive rate mismatch -- got %f, want %f",
				test.name, newFPRate, oldFPRate)
		}

		// Ensure the filter size is the same.
		newSize := f.Size()
		if newSize != oldSize {
			t.Errorf("%q: size mismatch -- got %d, want %d", test.name, newSize,
				oldSize)
		}
	}
}

// TestMaxNearOptimalL ensures that the function which attempts to determine a
// near optimal L value caps out at the expected point.
func TestMaxNearOptimalL(t *testing.T) {
	const wantL = 100
	if gotL := nearOptimalL(7, 0.35); gotL != wantL {
		t.Fatalf("unexepected calaculated L value -- got %d, want %d", gotL,
			wantL)
	}
}
