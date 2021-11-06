// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package uint256

import (
	"reflect"
	"testing"
)

// TestUint256SetUint64 ensures that setting a scalar to various native integers
// works as expected.
func TestUint256SetUint64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string    // test description
		n    uint64    // test value
		want [4]uint64 // expected words
	}{{
		name: "five",
		n:    0x5,
		want: [4]uint64{0x5, 0, 0, 0},
	}, {
		name: "2^32 - 1",
		n:    0xffffffff,
		want: [4]uint64{0xffffffff, 0, 0, 0},
	}, {
		name: "2^32",
		n:    0x100000000,
		want: [4]uint64{0x100000000, 0, 0, 0},
	}, {
		name: "2^64 - 1",
		n:    0xffffffffffffffff,
		want: [4]uint64{0xffffffffffffffff, 0, 0, 0},
	}}

	for _, test := range tests {
		n := new(Uint256).SetUint64(test.n)
		if !reflect.DeepEqual(n.n, test.want) {
			t.Errorf("%s: wrong result -- got: %x want: %x", test.name, n.n,
				test.want)
			continue
		}
	}
}
