// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"fmt"
	"math/big"
	"testing"
)

// TestIsOnCurveAdaptor ensures the IsOnCurve method used to satisfy the
// elliptic.Curve interfaces works as intended.
func TestIsOnCurveAdaptor(t *testing.T) {
	s256 := S256()
	if !s256.IsOnCurve(s256.Params().Gx, s256.Params().Gy) {
		t.Fatal("generator point does not claim to be on the curve")
	}
}

// TestScalarBaseMultAdaptor ensures the ScalarBaseMult method used to satisfy
// the elliptic.Curve interfaces works as intended.
func TestScalarBaseMultAdaptor(t *testing.T) {
	tests := []struct {
		k    string
		x, y string
	}{{
		"aa5e28d6a97a2479a65527f7290311a3624d4cc0fa1578598ee3c2613bf99522",
		"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
		"b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
	}, {
		"7e2b897b8cebc6361663ad410835639826d590f393d90a9538881735256dfae3",
		"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
		"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
	}, {
		"6461e6df0fe7dfd05329f41bf771b86578143d4dd1f7866fb4ca7e97c5fa945d",
		"e8aecc370aedd953483719a116711963ce201ac3eb21d3f3257bb48668c6a72f",
		"c25caf2f0eba1ddb2f0f3f47866299ef907867b7d27e95b3873bf98397b24ee1",
	}, {
		"376a3a2cdcd12581efff13ee4ad44c4044b8a0524c42422a7e1e181e4deeccec",
		"14890e61fcd4b0bd92e5b36c81372ca6fed471ef3aa60a3e415ee4fe987daba1",
		"297b858d9f752ab42d3bca67ee0eb6dcd1c2b7b0dbe23397e66adc272263f982",
	}, {
		"1b22644a7be026548810c378d0b2994eefa6d2b9881803cb02ceff865287d1b9",
		"f73c65ead01c5126f28f442d087689bfa08e12763e0cec1d35b01751fd735ed3",
		"f449a8376906482a84ed01479bd18882b919c140d638307f0c0934ba12590bde",
	}}

	s256 := S256()
	for i, test := range tests {
		k, ok := new(big.Int).SetString(test.k, 16)
		if !ok {
			t.Errorf("%d: bad value for k: %s", i, test.k)
		}
		x, y := s256.ScalarBaseMult(k.Bytes())
		if fmt.Sprintf("%x", x) != test.x || fmt.Sprintf("%x", y) != test.y {
			t.Errorf("%d: bad output for k=%s: got (%x, %x), want (%s, %s)", i,
				test.k, x, y, test.x, test.y)
		}
		if testing.Short() && i > 5 {
			break
		}
	}
}
