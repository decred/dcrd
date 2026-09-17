// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixclient

import "testing"

// TestSRMixDimensions checks the dimension requirements shared by normal runs
// and blame assignment, including malformed matrices that would otherwise panic
// when comparing reconstructed mixes or silently ignore extra field elements.
func TestSRMixDimensions(t *testing.T) {
	tests := []struct {
		name   string
		widths []int
		mcount uint32
		mtot   uint32
		want   bool
	}{
		{"single message", []int{1}, 1, 1, true},
		{"multiple messages", []int{3, 3}, 2, 3, true},
		{"different session size", []int{4, 4}, 2, 4, true},
		{"missing matrix", nil, 2, 3, false},
		{"missing row", []int{3}, 2, 3, false},
		{"extra row", []int{3, 3, 3}, 2, 3, false},
		{"empty first row", []int{0, 3}, 2, 3, false},
		{"empty last row", []int{3, 0}, 2, 3, false},
		{"short first row", []int{2, 3}, 2, 3, false},
		{"short last row", []int{3, 2}, 2, 3, false},
		{"long first row", []int{4, 3}, 2, 3, false},
		{"long last row", []int{3, 4}, 2, 3, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var mix [][][]byte
			for _, width := range test.widths {
				mix = append(mix, make([][]byte, width))
			}
			if got := hasSRMixDimensions(mix, test.mcount, test.mtot); got != test.want {
				t.Errorf("hasSRMixDimensions(%v, %d, %d) = %v, want %v",
					test.widths, test.mcount, test.mtot, got, test.want)
			}
		})
	}
}
