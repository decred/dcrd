// Copyright (c) 2026 The Decred developers
// Copyright (c) 2013-2026 Dave Collins
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import "testing"

// TestConstantTimeSelect64 ensures that the 64-bit constant time selection
// function works as expected in terms of behavior.
func TestConstantTimeSelect64(t *testing.T) {
	tests := []struct {
		name string // test description
		cond uint64 // condition value
		a    uint64 // first value
		b    uint64 // second value
		want uint64 // expected selected value
	}{
		{"sel(0,1,2)==2", 0, 1, 2, 2},
		{"sel(1,1,2)==1", 1, 1, 2, 1},
		{"sel(0,1<<64-1,1)==1", 0, 1<<64 - 1, 1, 1},
		{"sel(1,1<<64-1,1)==1<<64-1", 1, 1<<64 - 1, 1, 1<<64 - 1},
		{"sel(0,1,1<<64-1)==1<<64-1", 0, 1, 1<<64 - 1, 1<<64 - 1},
		{"sel(1,1,1<<64-1)==1", 1, 1, 1<<64 - 1, 1},
		{"sel(0,1<<64-1,1<<64-2)==1<<64-2", 0, 1<<64 - 1, 1<<64 - 2, 1<<64 - 2},
		{"sel(1,1<<64-1,1<<64-2)==1<<64-1", 1, 1<<64 - 1, 1<<64 - 2, 1<<64 - 1},
	}

	for _, test := range tests {
		got := constantTimeSelect64(test.cond, test.a, test.b)
		if got != test.want {
			t.Errorf("%q: unexpected result -- got %d, want %d", test.name,
				got, test.want)
		}
	}
}
