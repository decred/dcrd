// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package cpufeat

import "testing"

// TestSupported ensures that querying the cached feature set is stable and
// idempotent across repeated calls since it is only ever computed once during
// package initialization.  It also ensures that invoking detect directly
// produces the same result.
func TestSupported(t *testing.T) {
	got1 := Supported()
	got2 := Supported()
	if got1 != got2 {
		t.Fatalf("inconsistent results across calls -- got %+v and %+v", got1,
			got2)
	}

	// Ensure the cached features exposed via [Supported] match since they are
	// derived at package initialization time.
	want := detect()
	if got := Supported(); got != want {
		t.Fatalf("unexpected supported features -- got %+v, want %+v", got,
			want)
	}
}
