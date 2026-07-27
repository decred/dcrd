// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build !(386 || amd64) || purego

package cpufeat

import "testing"

// TestDetectGeneric ensures that the generic feature detection fallback used
// for architectures without dedicated support (and the purego build tag)
// correctly reports that no optional features are supported.
func TestDetectGeneric(t *testing.T) {
	want := Features{}
	if got := detect(); got != want {
		t.Fatalf("unexpected detected features -- got %+v, want %+v", got,
			want)
	}
}
