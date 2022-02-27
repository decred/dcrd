// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"fmt"
	"math/big"
	"testing"
)

// TestIsOnCurveAdaptor ensures the IsOnCurve method used to satisfy the
// elliptic.Curve interface works as intended.
func TestIsOnCurveAdaptor(t *testing.T) {
	s256 := S256()
	if !s256.IsOnCurve(s256.Params().Gx, s256.Params().Gy) {
		t.Fatal("generator point does not claim to be on the curve")
	}
}

// isValidAffinePoint returns true if the point (x,y) is on the secp256k1 curve
// or is the point at infinity.
func isValidAffinePoint(x, y *big.Int) bool {
	if x.Sign() == 0 && y.Sign() == 0 {
		return true
	}
	return S256().IsOnCurve(x, y)
}

// TestAddAffineAdaptor tests addition of points in affine coordinates via the
// method used to satisfy the elliptic.Curve interface works as intended for
// some edge cases and known good values.
func TestAddAffineAdaptor(t *testing.T) {
	tests := []struct {
		name   string // test description
		x1, y1 string // hex encoded coordinates of first point to add
		x2, y2 string // hex encoded coordinates of second point to add
		x3, y3 string // hex encoded coordinates of expected point
	}{{
		// Addition with the point at infinity (left hand side).
		name: "∞ + P = P",
		x1:   "0",
		y1:   "0",
		x2:   "d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
		y2:   "131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
		x3:   "d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
		y3:   "131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
	}, {
		// Addition with the point at infinity (right hand side).
		name: "P + ∞ = P",
		x1:   "d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
		y1:   "131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
		x2:   "0",
		y2:   "0",
		x3:   "d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
		y3:   "131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
	}, {
		// Addition with different x values.
		name: "P(x1, y1) + P(x2, y2)",
		x1:   "34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
		y1:   "0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
		x2:   "d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
		y2:   "131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
		x3:   "fd5b88c21d3143518d522cd2796f3d726793c88b3e05636bc829448e053fed69",
		y3:   "21cf4f6a5be5ff6380234c50424a970b1f7e718f5eb58f68198c108d642a137f",
	}, {
		// Addition with same x opposite y.
		name: "P(x, y) + P(x, -y) = ∞",
		x1:   "34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
		y1:   "0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
		x2:   "34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
		y2:   "f48e156428cf0276dc092da5856e182288d7569f97934a56fe44be60f0d359fd",
		x3:   "0",
		y3:   "0",
	}, {
		// Addition with same point.
		name: "P(x, y) + P(x, y) = 2P",
		x1:   "34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
		y1:   "0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
		x2:   "34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
		y2:   "0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
		x3:   "59477d88ae64a104dbb8d31ec4ce2d91b2fe50fa628fb6a064e22582196b365b",
		y3:   "938dc8c0f13d1e75c987cb1a220501bd614b0d3dd9eb5c639847e1240216e3b6",
	}}

	curve := S256()
	for _, test := range tests {
		// Parse the test data.
		x1, y1 := fromHex(test.x1), fromHex(test.y1)
		x2, y2 := fromHex(test.x2), fromHex(test.y2)
		x3, y3 := fromHex(test.x3), fromHex(test.y3)

		// Ensure the test data is using points that are actually on the curve
		// (or the point at infinity).
		if !isValidAffinePoint(x1, y1) {
			t.Errorf("%s: first point is not on curve", test.name)
			continue
		}
		if !isValidAffinePoint(x2, y2) {
			t.Errorf("%s: second point is not on curve", test.name)
			continue
		}
		if !isValidAffinePoint(x3, y3) {
			t.Errorf("%s: expected point is not on curve", test.name)
			continue
		}

		// Add the two points and ensure the result matches expected.
		rx, ry := curve.Add(x1, y1, x2, y2)
		if rx.Cmp(x3) != 0 || ry.Cmp(y3) != 0 {
			t.Errorf("%s: wrong result\ngot: (%x, %x)\nwant: (%x, %x)",
				test.name, rx, ry, x3, y3)
			continue
		}
	}
}

// TestDoubleAffineAdaptor tests doubling of points in affine coordinates via
// the method used to satisfy the elliptic.Curve interface works as intended for
// some edge cases and known good values.
func TestDoubleAffineAdaptor(t *testing.T) {
	tests := []struct {
		name   string // test description
		x1, y1 string // hex encoded coordinates of point to double
		x3, y3 string // hex encoded coordinates of expected point
	}{{
		// Doubling the point at infinity is still the point at infinity.
		name: "2*∞ = ∞ (point at infinity)",
		x1:   "0",
		y1:   "0",
		x3:   "0",
		y3:   "0",
	}, {
		name: "random point 1",
		x1:   "e41387ffd8baaeeb43c2faa44e141b19790e8ac1f7ff43d480dc132230536f86",
		y1:   "1b88191d430f559896149c86cbcb703193105e3cf3213c0c3556399836a2b899",
		x3:   "88da47a089d333371bd798c548ef7caae76e737c1980b452d367b3cfe3082c19",
		y3:   "3b6f659b09a362821dfcfefdbfbc2e59b935ba081b6c249eb147b3c2100b1bc1",
	}, {
		name: "random point 2",
		x1:   "b3589b5d984f03ef7c80aeae444f919374799edf18d375cab10489a3009cff0c",
		y1:   "c26cf343875b3630e15bccc61202815b5d8f1fd11308934a584a5babe69db36a",
		x3:   "e193860172998751e527bb12563855602a227fc1f612523394da53b746bb2fb1",
		y3:   "2bfcf13d2f5ab8bb5c611fab5ebbed3dc2f057062b39a335224c22f090c04789",
	}, {
		name: "random point 3",
		x1:   "2b31a40fbebe3440d43ac28dba23eee71c62762c3fe3dbd88b4ab82dc6a82340",
		y1:   "9ba7deb02f5c010e217607fd49d58db78ec273371ea828b49891ce2fd74959a1",
		x3:   "2c8d5ef0d343b1a1a48aa336078eadda8481cb048d9305dc4fdf7ee5f65973a2",
		y3:   "bb4914ac729e26d3cd8f8dc8f702f3f4bb7e0e9c5ae43335f6e94c2de6c3dc95",
	}, {
		name: "random point 4",
		x1:   "61c64b760b51981fab54716d5078ab7dffc93730b1d1823477e27c51f6904c7a",
		y1:   "ef6eb16ea1a36af69d7f66524c75a3a5e84c13be8fbc2e811e0563c5405e49bd",
		x3:   "5f0dcdd2595f5ad83318a0f9da481039e36f135005420393e72dfca985b482f4",
		y3:   "a01c849b0837065c1cb481b0932c441f49d1cab1b4b9f355c35173d93f110ae0",
	}}

	curve := S256()
	for _, test := range tests {
		// Parse test data.
		x1, y1 := fromHex(test.x1), fromHex(test.y1)
		x3, y3 := fromHex(test.x3), fromHex(test.y3)

		// Ensure the test data is using points that are actually on
		// the curve (or the point at infinity).
		if !isValidAffinePoint(x1, y1) {
			t.Errorf("%s: first point is not on the curve", test.name)
			continue
		}
		if !isValidAffinePoint(x3, y3) {
			t.Errorf("%s: expected point is not on the curve", test.name)
			continue
		}

		// Double the point and ensure the result matches expected.
		rx, ry := curve.Double(x1, y1)
		if rx.Cmp(x3) != 0 || ry.Cmp(y3) != 0 {
			t.Errorf("%s: wrong result\ngot: (%x, %x)\nwant: (%x, %x)",
				test.name, rx, ry, x3, y3)
			continue
		}
	}
}

// TestScalarBaseMultAdaptor ensures the ScalarBaseMult method used to satisfy
// the elliptic.Curve interface works as intended.
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
