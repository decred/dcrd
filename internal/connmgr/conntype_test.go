// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"testing"
)

// TestConnectionTypeStringer tests the stringized output for connection types.
func TestConnectionTypeStringer(t *testing.T) {
	tests := []struct {
		in   ConnectionType
		want string
	}{
		{ConnTypeInbound, "inbound"},
		{ConnTypeOutbound, "outbound"},
		{ConnTypeManual, "manual"},
		{0xff, "Unknown ConnectionType (255)"},
	}

	// Detect additional defines that don't have the stringer added.
	if len(tests)-1 != int(numConnTypes) {
		t.Fatal("It appears a connection type was added without adding an " +
			"associated stringer test")
	}

	for i, test := range tests {
		if got := test.in.String(); got != test.want {
			t.Errorf("String #%d: got: %s, want: %s", i, got, test.want)
			continue
		}
	}
}
