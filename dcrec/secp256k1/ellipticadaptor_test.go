// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"testing"
)

// TestIsOnCurveAdaptor ensures the IsOnCurve method used to satisfy the
// elliptic.Curve interfaces works as intended.
func TestIsOnCurveAdaptor(t *testing.T) {
	s256 := S256()
	if !s256.IsOnCurve(s256.Params().Gx, s256.Params().Gy) {
		t.Fatalf("generator point does not claim to be on the curve")
	}
}
