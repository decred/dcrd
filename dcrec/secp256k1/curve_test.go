// Copyright (c) 2015-2020 The Decred developers
// Copyright 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"crypto/rand"
	"fmt"
	"math/big"
	mrand "math/rand"
	"testing"
	"time"
)

// isJacobianOnS256Curve returns boolean if the point (x,y,z) is on the
// secp256k1 curve.
func isJacobianOnS256Curve(point *JacobianPoint) bool {
	// Elliptic curve equation for secp256k1 is: y^2 = x^3 + 7
	// In Jacobian coordinates, Y = y/z^3 and X = x/z^2
	// Thus:
	// (y/z^3)^2 = (x/z^2)^3 + 7
	// y^2/z^6 = x^3/z^6 + 7
	// y^2 = x^3 + 7*z^6
	var y2, z2, x3, result FieldVal
	y2.SquareVal(&point.Y).Normalize()
	z2.SquareVal(&point.Z)
	x3.SquareVal(&point.X).Mul(&point.X)
	result.SquareVal(&z2).Mul(&z2).MulInt(7).Add(&x3).Normalize()
	return y2.Equals(&result)
}

// jacobianPointFromHex decodes the passed big-endian hex strings into a
// Jacobian point with its internal fields set to the resulting values.  Only
// the first 32-bytes are used.
func jacobianPointFromHex(x, y, z string) JacobianPoint {
	var p JacobianPoint
	p.X.SetHex(x)
	p.Y.SetHex(y)
	p.Z.SetHex(z)
	return p
}

// IsStrictlyEqual returns whether or not the two Jacobian points are strictly
// equal for use in the tests.  Recall that several Jacobian points can be equal
// in affine coordinates, while not having the same coordinates in projective
// space, so the two points not being equal doesn't necessarily mean they aren't
// actually the same affine point.
func (p *JacobianPoint) IsStrictlyEqual(other *JacobianPoint) bool {
	return p.X.Equals(&other.X) && p.Y.Equals(&other.Y) && p.Z.Equals(&other.Z)
}

// TestAddJacobian tests addition of points projected in Jacobian coordinates.
func TestAddJacobian(t *testing.T) {
	tests := []struct {
		x1, y1, z1 string // Coordinates (in hex) of first point to add
		x2, y2, z2 string // Coordinates (in hex) of second point to add
		x3, y3, z3 string // Coordinates (in hex) of expected point
	}{
		// Addition with a point at infinity (left hand side).
		// ∞ + P = P
		{
			"0",
			"0",
			"0",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"1",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"1",
		},
		// Addition with a point at infinity (right hand side).
		// P + ∞ = P
		{
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"1",
			"0",
			"0",
			"0",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"1",
		},
		// Addition with z1=z2=1 different x values.
		{
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"1",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"1",
			"0cfbc7da1e569b334460788faae0286e68b3af7379d5504efc25e4dba16e46a6",
			"e205f79361bbe0346b037b4010985dbf4f9e1e955e7d0d14aca876bfa79aad87",
			"44a5646b446e3877a648d6d381370d9ef55a83b666ebce9df1b1d7d65b817b2f",
		},
		// Addition with z1=z2=1 same x opposite y.
		// P(x, y, z) + P(x, -y, z) = infinity
		{
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"1",
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"f48e156428cf0276dc092da5856e182288d7569f97934a56fe44be60f0d359fd",
			"1",
			"0",
			"0",
			"0",
		},
		// Addition with z1=z2=1 same point.
		// P(x, y, z) + P(x, y, z) = 2P
		{
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"1",
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"1",
			"ec9f153b13ee7bd915882859635ea9730bf0dc7611b2c7b0e37ee64f87c50c27",
			"b082b53702c466dcf6e984a35671756c506c67c2fcb8adb408c44dd0755c8f2a",
			"16e3d537ae61fb1247eda4b4f523cfbaee5152c0d0d96b520376833c1e594464",
		},

		// Addition with z1=z2 (!=1) different x values.
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"5d2fe112c21891d440f65a98473cb626111f8a234d2cd82f22172e369f002147",
			"98e3386a0a622a35c4561ffb32308d8e1c6758e10ebb1b4ebd3d04b4eb0ecbe8",
			"2",
			"cfbc7da1e569b334460788faae0286e68b3af7379d5504efc25e4dba16e46a60",
			"817de4d86ef80d1ac0ded00426176fd3e787a5579f43452b2a1db021e6ac3778",
			"129591ad11b8e1de99235b4e04dc367bd56a0ed99baf3a77c6c75f5a6e05f08d",
		},
		// Addition with z1=z2 (!=1) same x opposite y.
		// P(x, y, z) + P(x, -y, z) = infinity
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"a470ab21467813b6e0496d2c2b70c11446bab4fcbc9a52b7f225f30e869aea9f",
			"2",
			"0",
			"0",
			"0",
		},
		// Addition with z1=z2 (!=1) same point.
		// P(x, y, z) + P(x, y, z) = 2P
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"9f153b13ee7bd915882859635ea9730bf0dc7611b2c7b0e37ee65073c50fabac",
			"2b53702c466dcf6e984a35671756c506c67c2fcb8adb408c44dd125dc91cb988",
			"6e3d537ae61fb1247eda4b4f523cfbaee5152c0d0d96b520376833c2e5944a11",
		},

		// Addition with z1!=z2 and z2=1 different x values.
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"1",
			"3ef1f68795a6ccd1181e23eab80a1b9a2cebdcde755413bf097936eb5b91b4f3",
			"0bef26c377c068d606f6802130bb7e9f3c3d2abcfa1a295950ed81133561cb04",
			"252b235a2371c3bd3246b69c09b86cf7aad41db3375e74ef8d8ebeb4dc0be11a",
		},
		// Addition with z1!=z2 and z2=1 same x opposite y.
		// P(x, y, z) + P(x, -y, z) = infinity
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"f48e156428cf0276dc092da5856e182288d7569f97934a56fe44be60f0d359fd",
			"1",
			"0",
			"0",
			"0",
		},
		// Addition with z1!=z2 and z2=1 same point.
		// P(x, y, z) + P(x, y, z) = 2P
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"1",
			"9f153b13ee7bd915882859635ea9730bf0dc7611b2c7b0e37ee65073c50fabac",
			"2b53702c466dcf6e984a35671756c506c67c2fcb8adb408c44dd125dc91cb988",
			"6e3d537ae61fb1247eda4b4f523cfbaee5152c0d0d96b520376833c2e5944a11",
		},

		// Addition with z1!=z2 and z2!=1 different x values.
		// P(x, y, z) + P(x, y, z) = 2P
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"91abba6a34b7481d922a4bd6a04899d5a686f6cf6da4e66a0cb427fb25c04bd4",
			"03fede65e30b4e7576a2abefc963ddbf9fdccbf791b77c29beadefe49951f7d1",
			"3",
			"3f07081927fd3f6dadd4476614c89a09eba7f57c1c6c3b01fa2d64eac1eef31e",
			"949166e04ebc7fd95a9d77e5dfd88d1492ecffd189792e3944eb2b765e09e031",
			"eb8cba81bcffa4f44d75427506737e1f045f21e6d6f65543ee0e1d163540c931",
		}, // Addition with z1!=z2 and z2!=1 same x opposite y.
		// P(x, y, z) + P(x, -y, z) = infinity
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"dcc3768780c74a0325e2851edad0dc8a566fa61a9e7fc4a34d13dcb509f99bc7",
			"cafc41904dd5428934f7d075129c8ba46eb622d4fc88d72cd1401452664add18",
			"3",
			"0",
			"0",
			"0",
		},
		// Addition with z1!=z2 and z2!=1 same point.
		// P(x, y, z) + P(x, y, z) = 2P
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"dcc3768780c74a0325e2851edad0dc8a566fa61a9e7fc4a34d13dcb509f99bc7",
			"3503be6fb22abd76cb082f8aed63745b9149dd2b037728d32ebfebac99b51f17",
			"3",
			"9f153b13ee7bd915882859635ea9730bf0dc7611b2c7b0e37ee65073c50fabac",
			"2b53702c466dcf6e984a35671756c506c67c2fcb8adb408c44dd125dc91cb988",
			"6e3d537ae61fb1247eda4b4f523cfbaee5152c0d0d96b520376833c2e5944a11",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Convert hex to Jacobian points.
		p1 := jacobianPointFromHex(test.x1, test.y1, test.z1)
		p2 := jacobianPointFromHex(test.x2, test.y2, test.z2)
		want := jacobianPointFromHex(test.x3, test.y3, test.z3)

		// Ensure the test data is using points that are actually on
		// the curve (or the point at infinity).
		if !p1.Z.IsZero() && !isJacobianOnS256Curve(&p1) {
			t.Errorf("#%d first point is not on the curve -- "+
				"invalid test data", i)
			continue
		}
		if !p2.Z.IsZero() && !isJacobianOnS256Curve(&p2) {
			t.Errorf("#%d second point is not on the curve -- "+
				"invalid test data", i)
			continue
		}
		if !want.Z.IsZero() && !isJacobianOnS256Curve(&want) {
			t.Errorf("#%d expected point is not on the curve -- "+
				"invalid test data", i)
			continue
		}

		// Add the two points.
		var r JacobianPoint
		AddNonConst(&p1, &p2, &r)

		// Ensure result matches expected.
		if !r.IsStrictlyEqual(&want) {
			t.Errorf("#%d wrong result\ngot: (%v, %v, %v)\n"+
				"want: (%v, %v, %v)", i, r.X, r.Y, r.Z, want.X, want.Y, want.Z)
			continue
		}
	}
}

// TestAddAffine tests addition of points in affine coordinates.
func TestAddAffine(t *testing.T) {
	tests := []struct {
		x1, y1 string // Coordinates (in hex) of first point to add
		x2, y2 string // Coordinates (in hex) of second point to add
		x3, y3 string // Coordinates (in hex) of expected point
	}{
		// Addition with a point at infinity (left hand side).
		// ∞ + P = P
		{
			"0",
			"0",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
		},
		// Addition with a point at infinity (right hand side).
		// P + ∞ = P
		{
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"0",
			"0",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
		},

		// Addition with different x values.
		{
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"d74bf844b0862475103d96a611cf2d898447e288d34b360bc885cb8ce7c00575",
			"131c670d414c4546b88ac3ff664611b1c38ceb1c21d76369d7a7a0969d61d97d",
			"fd5b88c21d3143518d522cd2796f3d726793c88b3e05636bc829448e053fed69",
			"21cf4f6a5be5ff6380234c50424a970b1f7e718f5eb58f68198c108d642a137f",
		},
		// Addition with same x opposite y.
		// P(x, y) + P(x, -y) = infinity
		{
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"f48e156428cf0276dc092da5856e182288d7569f97934a56fe44be60f0d359fd",
			"0",
			"0",
		},
		// Addition with same point.
		// P(x, y) + P(x, y) = 2P
		{
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"59477d88ae64a104dbb8d31ec4ce2d91b2fe50fa628fb6a064e22582196b365b",
			"938dc8c0f13d1e75c987cb1a220501bd614b0d3dd9eb5c639847e1240216e3b6",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Convert hex to field values.
		x1, y1 := fromHex(test.x1), fromHex(test.y1)
		x2, y2 := fromHex(test.x2), fromHex(test.y2)
		x3, y3 := fromHex(test.x3), fromHex(test.y3)

		// Ensure the test data is using points that are actually on
		// the curve (or the point at infinity).
		if !(x1.Sign() == 0 && y1.Sign() == 0) && !S256().IsOnCurve(x1, y1) {
			t.Errorf("#%d first point is not on the curve -- "+
				"invalid test data", i)
			continue
		}
		if !(x2.Sign() == 0 && y2.Sign() == 0) && !S256().IsOnCurve(x2, y2) {
			t.Errorf("#%d second point is not on the curve -- "+
				"invalid test data", i)
			continue
		}
		if !(x3.Sign() == 0 && y3.Sign() == 0) && !S256().IsOnCurve(x3, y3) {
			t.Errorf("#%d expected point is not on the curve -- "+
				"invalid test data", i)
			continue
		}

		// Add the two points.
		rx, ry := S256().Add(x1, y1, x2, y2)

		// Ensure result matches expected.
		if rx.Cmp(x3) != 00 || ry.Cmp(y3) != 0 {
			t.Errorf("#%d wrong result\ngot: (%x, %x)\n"+
				"want: (%x, %x)", i, rx, ry, x3, y3)
			continue
		}
	}
}

// TestDoubleJacobian tests doubling of points projected in Jacobian
// coordinates.
func TestDoubleJacobian(t *testing.T) {
	tests := []struct {
		x1, y1, z1 string // Coordinates (in hex) of point to double
		x3, y3, z3 string // Coordinates (in hex) of expected point
	}{
		// Doubling a point at infinity is still infinity.
		{
			"0",
			"0",
			"0",
			"0",
			"0",
			"0",
		},
		// Doubling with z1=1.
		{
			"34f9460f0e4f08393d192b3c5133a6ba099aa0ad9fd54ebccfacdfa239ff49c6",
			"0b71ea9bd730fd8923f6d25a7a91e7dd7728a960686cb5a901bb419e0f2ca232",
			"1",
			"ec9f153b13ee7bd915882859635ea9730bf0dc7611b2c7b0e37ee64f87c50c27",
			"b082b53702c466dcf6e984a35671756c506c67c2fcb8adb408c44dd0755c8f2a",
			"16e3d537ae61fb1247eda4b4f523cfbaee5152c0d0d96b520376833c1e594464",
		},
		// Doubling with z1!=1.
		{
			"d3e5183c393c20e4f464acf144ce9ae8266a82b67f553af33eb37e88e7fd2718",
			"5b8f54deb987ec491fb692d3d48f3eebb9454b034365ad480dda0cf079651190",
			"2",
			"9f153b13ee7bd915882859635ea9730bf0dc7611b2c7b0e37ee65073c50fabac",
			"2b53702c466dcf6e984a35671756c506c67c2fcb8adb408c44dd125dc91cb988",
			"6e3d537ae61fb1247eda4b4f523cfbaee5152c0d0d96b520376833c2e5944a11",
		},
		// From btcd issue #709.
		{
			"201e3f75715136d2f93c4f4598f91826f94ca01f4233a5bd35de9708859ca50d",
			"bdf18566445e7562c6ada68aef02d498d7301503de5b18c6aef6e2b1722412e1",
			"0000000000000000000000000000000000000000000000000000000000000001",
			"4a5e0559863ebb4e9ed85f5c4fa76003d05d9a7626616e614a1f738621e3c220",
			"00000000000000000000000000000000000000000000000000000001b1388778",
			"7be30acc88bceac58d5b4d15de05a931ae602a07bcb6318d5dedc563e4482993",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Convert hex to field values.
		p1 := jacobianPointFromHex(test.x1, test.y1, test.z1)
		want := jacobianPointFromHex(test.x3, test.y3, test.z3)

		// Ensure the test data is using points that are actually on
		// the curve (or the point at infinity).
		if !p1.Z.IsZero() && !isJacobianOnS256Curve(&p1) {
			t.Errorf("#%d first point is not on the curve -- "+
				"invalid test data", i)
			continue
		}
		if !want.Z.IsZero() && !isJacobianOnS256Curve(&want) {
			t.Errorf("#%d expected point is not on the curve -- "+
				"invalid test data", i)
			continue
		}

		// Double the point.
		var result JacobianPoint
		DoubleNonConst(&p1, &result)

		// Ensure result matches expected.
		if !result.IsStrictlyEqual(&want) {
			t.Errorf("#%d wrong result\ngot: (%v, %v, %v)\n"+
				"want: (%v, %v, %v)", i, result.X, result.Y, result.Z,
				want.X, want.Y, want.Z)
			continue
		}
	}
}

// TestDoubleAffine tests doubling of points in affine coordinates.
func TestDoubleAffine(t *testing.T) {
	tests := []struct {
		x1, y1 string // Coordinates (in hex) of point to double
		x3, y3 string // Coordinates (in hex) of expected point
	}{
		// Doubling a point at infinity is still infinity.
		// 2*∞ = ∞ (point at infinity)

		{
			"0",
			"0",
			"0",
			"0",
		},

		// Random points.
		{
			"e41387ffd8baaeeb43c2faa44e141b19790e8ac1f7ff43d480dc132230536f86",
			"1b88191d430f559896149c86cbcb703193105e3cf3213c0c3556399836a2b899",
			"88da47a089d333371bd798c548ef7caae76e737c1980b452d367b3cfe3082c19",
			"3b6f659b09a362821dfcfefdbfbc2e59b935ba081b6c249eb147b3c2100b1bc1",
		},
		{
			"b3589b5d984f03ef7c80aeae444f919374799edf18d375cab10489a3009cff0c",
			"c26cf343875b3630e15bccc61202815b5d8f1fd11308934a584a5babe69db36a",
			"e193860172998751e527bb12563855602a227fc1f612523394da53b746bb2fb1",
			"2bfcf13d2f5ab8bb5c611fab5ebbed3dc2f057062b39a335224c22f090c04789",
		},
		{
			"2b31a40fbebe3440d43ac28dba23eee71c62762c3fe3dbd88b4ab82dc6a82340",
			"9ba7deb02f5c010e217607fd49d58db78ec273371ea828b49891ce2fd74959a1",
			"2c8d5ef0d343b1a1a48aa336078eadda8481cb048d9305dc4fdf7ee5f65973a2",
			"bb4914ac729e26d3cd8f8dc8f702f3f4bb7e0e9c5ae43335f6e94c2de6c3dc95",
		},
		{
			"61c64b760b51981fab54716d5078ab7dffc93730b1d1823477e27c51f6904c7a",
			"ef6eb16ea1a36af69d7f66524c75a3a5e84c13be8fbc2e811e0563c5405e49bd",
			"5f0dcdd2595f5ad83318a0f9da481039e36f135005420393e72dfca985b482f4",
			"a01c849b0837065c1cb481b0932c441f49d1cab1b4b9f355c35173d93f110ae0",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Convert hex to field values.
		x1, y1 := fromHex(test.x1), fromHex(test.y1)
		x3, y3 := fromHex(test.x3), fromHex(test.y3)

		// Ensure the test data is using points that are actually on
		// the curve (or the point at infinity).
		if !(x1.Sign() == 0 && y1.Sign() == 0) && !S256().IsOnCurve(x1, y1) {
			t.Errorf("#%d first point is not on the curve -- "+
				"invalid test data", i)
			continue
		}
		if !(x3.Sign() == 0 && y3.Sign() == 0) && !S256().IsOnCurve(x3, y3) {
			t.Errorf("#%d expected point is not on the curve -- "+
				"invalid test data", i)
			continue
		}

		// Double the point.
		rx, ry := S256().Double(x1, y1)

		// Ensure result matches expected.
		if rx.Cmp(x3) != 00 || ry.Cmp(y3) != 0 {
			t.Errorf("#%d wrong result\ngot: (%x, %x)\n"+
				"want: (%x, %x)", i, rx, ry, x3, y3)
			continue
		}
	}
}

// checkNAFEncoding returns an error if the provided positive and negative
// portions of an overall NAF encoding do not adhere to the requirements or they
// do not sum back to the provided original value.
func checkNAFEncoding(pos, neg []byte, origValue *big.Int) error {
	// NAF must not have a leading zero byte and the number of negative
	// bytes must not exceed the positive portion.
	if len(pos) > 0 && pos[0] == 0 {
		return fmt.Errorf("positive has leading zero -- got %x", pos)
	}
	if len(neg) > len(pos) {
		return fmt.Errorf("negative has len %d > pos len %d", len(neg),
			len(pos))
	}

	// Ensure the result doesn't have any adjacent non-zero digits.
	gotPos := new(big.Int).SetBytes(pos)
	gotNeg := new(big.Int).SetBytes(neg)
	posOrNeg := new(big.Int).Or(gotPos, gotNeg)
	prevBit := posOrNeg.Bit(0)
	for bit := 1; bit < posOrNeg.BitLen(); bit++ {
		thisBit := posOrNeg.Bit(bit)
		if prevBit == 1 && thisBit == 1 {
			return fmt.Errorf("adjacent non-zero digits found at bit pos %d",
				bit-1)
		}
		prevBit = thisBit
	}

	// Ensure the resulting positive and negative portions of the overall
	// NAF representation sum back to the original value.
	gotValue := new(big.Int).Sub(gotPos, gotNeg)
	if origValue.Cmp(gotValue) != 0 {
		return fmt.Errorf("pos-neg is not original value: got %x, want %x",
			gotValue, origValue)
	}

	return nil
}

// TestNAF ensures encoding various edge cases and values to non-adjacent form
// produces valid results.
func TestNAF(t *testing.T) {
	tests := []struct {
		name string // test description
		in   string // hex encoded test value
	}{{
		name: "empty is zero",
		in:   "",
	}, {
		name: "zero",
		in:   "00",
	}, {
		name: "just before first carry",
		in:   "aa",
	}, {
		name: "first carry",
		in:   "ab",
	}, {
		name: "leading zeroes",
		in:   "002f20569b90697ad471c1be6107814f53f47446be298a3a2a6b686b97d35cf9",
	}, {
		name: "257 bits when NAF encoded",
		in:   "c000000000000000000000000000000000000000000000000000000000000001",
	}, {
		name: "32-byte scalar",
		in:   "6df2b5d30854069ccdec40ae022f5c948936324a4e9ebed8eb82cfd5a6b6d766",
	}, {
		name: "first term of balanced length-two representation #1",
		in:   "b776e53fb55f6b006a270d42d64ec2b1",
	}, {
		name: "second term balanced length-two representation #1",
		in:   "d6cc32c857f1174b604eefc544f0c7f7",
	}, {
		name: "first term of balanced length-two representation #2",
		in:   "45c53aa1bb56fcd68c011e2dad6758e4",
	}, {
		name: "second term of balanced length-two representation #2",
		in:   "a2e79d200f27f2360fba57619936159b",
	}}

	for _, test := range tests {
		// Ensure the resulting positive and negative portions of the overall
		// NAF representation adhere to the requirements of NAF encoding and
		// they sum back to the original value.
		pos, neg := naf(hexToBytes(test.in))
		if err := checkNAFEncoding(pos, neg, fromHex(test.in)); err != nil {
			t.Errorf("%q: %v", test.name, err)
		}
	}
}

// TestNAFRandom ensures that encoding randomly-generated values to non-adjacent
// form produces valid results.
func TestNAFRandom(t *testing.T) {
	// Use a unique random seed each test instance and log it if the tests fail.
	seed := time.Now().Unix()
	rng := mrand.New(mrand.NewSource(seed))
	defer func(t *testing.T, seed int64) {
		if t.Failed() {
			t.Logf("random seed: %d", seed)
		}
	}(t, seed)

	for i := 0; i < 100; i++ {
		// Ensure the resulting positive and negative portions of the overall
		// NAF representation adhere to the requirements of NAF encoding and
		// they sum back to the original value.
		bigIntVal, modNVal := randIntAndModNScalar(t, rng)
		valBytes := modNVal.Bytes()
		pos, neg := naf(valBytes[:])
		if err := checkNAFEncoding(pos, neg, bigIntVal); err != nil {
			t.Fatalf("encoding err: %v\nin: %x\npos: %x\nneg: %x", err,
				bigIntVal, pos, neg)
		}
	}
}

func TestBaseMultVerify(t *testing.T) {
	s256 := S256()
	for bytes := 1; bytes < 40; bytes++ {
		for i := 0; i < 30; i++ {
			data := make([]byte, bytes)
			_, err := rand.Read(data)
			if err != nil {
				t.Errorf("failed to read random data for %d", i)
				continue
			}
			x, y := s256.ScalarBaseMult(data)
			xWant, yWant := s256.ScalarMult(s256.Gx, s256.Gy, data)
			if x.Cmp(xWant) != 0 || y.Cmp(yWant) != 0 {
				t.Errorf("%d: bad output for %X: got (%X, %X), want (%X, %X)", i, data, x, y, xWant, yWant)
			}
			if testing.Short() && i > 2 {
				break
			}
		}
	}
}

func TestScalarMult(t *testing.T) {
	tests := []struct {
		x  string
		y  string
		k  string
		rx string
		ry string
	}{
		// base mult, essentially.
		{
			"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
			"483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8",
			"18e14a7b6a307f426a94f8114701e7c8e774e7f9a47e2c2035db29a206321725",
			"50863ad64a87ae8a2fe83c1af1a8403cb53f53e486d8511dad8a04887e5b2352",
			"2cd470243453a299fa9e77237716103abc11a1df38855ed6f2ee187e9c582ba6",
		},
		// From btcd issue #709.
		{
			"000000000000000000000000000000000000000000000000000000000000002c",
			"420e7a99bba18a9d3952597510fd2b6728cfeafc21a4e73951091d4d8ddbe94e",
			"a2e8ba2e8ba2e8ba2e8ba2e8ba2e8ba219b51835b55cc30ebfe2f6599bc56f58",
			"a2112dcdfbcd10ae1133a358de7b82db68e0a3eb4b492cc8268d1e7118c98788",
			"27fc7463b7bb3c5f98ecf2c84a6272bb1681ed553d92c69f2dfe25a9f9fd3836",
		},
	}

	s256 := S256()
	for i, test := range tests {
		x, _ := new(big.Int).SetString(test.x, 16)
		y, _ := new(big.Int).SetString(test.y, 16)
		k, _ := new(big.Int).SetString(test.k, 16)
		xWant, _ := new(big.Int).SetString(test.rx, 16)
		yWant, _ := new(big.Int).SetString(test.ry, 16)
		xGot, yGot := s256.ScalarMult(x, y, k.Bytes())
		if xGot.Cmp(xWant) != 0 || yGot.Cmp(yWant) != 0 {
			t.Fatalf("%d: bad output: got (%X, %X), want (%X, %X)", i, xGot, yGot, xWant, yWant)
		}
	}
}

func TestScalarMultRand(t *testing.T) {
	// Strategy for this test:
	//
	// Get a random exponent from the generator point at first
	// This creates a new point which is used in the next iteration
	// Use another random exponent on the new point.
	// We use BaseMult to verify by multiplying the previous exponent
	// and the new random exponent together (mod N)
	var want JacobianPoint
	var point JacobianPoint
	bigAffineToJacobian(curveParams.Gx, curveParams.Gy, &point)
	exponent := new(ModNScalar).SetInt(1)
	for i := 0; i < 1024; i++ {
		data := make([]byte, 32)
		_, err := rand.Read(data)
		if err != nil {
			t.Fatalf("failed to read random data at %d", i)
			break
		}
		var k ModNScalar
		k.SetByteSlice(data)
		ScalarMultNonConst(&k, &point, &point)

		exponent.Mul(&k)
		ScalarBaseMultNonConst(exponent, &want)
		point.ToAffine()
		want.ToAffine()
		if !point.IsStrictlyEqual(&want) {
			t.Fatalf("%d: bad output for %x:\ngot (%x, %x, %x)\n"+
				"want (%x, %x, %x)", i, data, point.X, point.Y, point.Z, want.X,
				want.Y, want.Z)
			break
		}
	}
}

func TestSplitK(t *testing.T) {
	tests := []struct {
		k      string
		k1, k2 string
		s1, s2 int
	}{
		{
			"6df2b5d30854069ccdec40ae022f5c948936324a4e9ebed8eb82cfd5a6b6d766",
			"00000000000000000000000000000000b776e53fb55f6b006a270d42d64ec2b1",
			"00000000000000000000000000000000d6cc32c857f1174b604eefc544f0c7f7",
			-1, -1,
		},
		{
			"6ca00a8f10632170accc1b3baf2a118fa5725f41473f8959f34b8f860c47d88d",
			"0000000000000000000000000000000007b21976c1795723c1bfbfa511e95b84",
			"00000000000000000000000000000000d8d2d5f9d20fc64fd2cf9bda09a5bf90",
			1, -1,
		},
		{
			"b2eda8ab31b259032d39cbc2a234af17fcee89c863a8917b2740b67568166289",
			"00000000000000000000000000000000507d930fecda7414fc4a523b95ef3c8c",
			"00000000000000000000000000000000f65ffb179df189675338c6185cb839be",
			-1, -1,
		},
		{
			"f6f00e44f179936f2befc7442721b0633f6bafdf7161c167ffc6f7751980e3a0",
			"0000000000000000000000000000000008d0264f10bcdcd97da3faa38f85308d",
			"0000000000000000000000000000000065fed1506eb6605a899a54e155665f79",
			-1, -1,
		},
		{
			"8679085ab081dc92cdd23091ce3ee998f6b320e419c3475fae6b5b7d3081996e",
			"0000000000000000000000000000000089fbf24fbaa5c3c137b4f1cedc51d975",
			"00000000000000000000000000000000d38aa615bd6754d6f4d51ccdaf529fea",
			-1, -1,
		},
		{
			"6b1247bb7931dfcae5b5603c8b5ae22ce94d670138c51872225beae6bba8cdb3",
			"000000000000000000000000000000008acc2a521b21b17cfb002c83be62f55d",
			"0000000000000000000000000000000035f0eff4d7430950ecb2d94193dedc79",
			-1, -1,
		},
		{
			"a2e8ba2e8ba2e8ba2e8ba2e8ba2e8ba219b51835b55cc30ebfe2f6599bc56f58",
			"0000000000000000000000000000000045c53aa1bb56fcd68c011e2dad6758e4",
			"00000000000000000000000000000000a2e79d200f27f2360fba57619936159b",
			-1, -1,
		},
	}

	for i, test := range tests {
		k, ok := new(big.Int).SetString(test.k, 16)
		if !ok {
			t.Errorf("%d: bad value for k: %s", i, test.k)
		}
		k1, k2, k1Sign, k2Sign := splitK(k.Bytes())
		k1str := fmt.Sprintf("%064x", k1)
		if test.k1 != k1str {
			t.Errorf("%d: bad k1: got %v, want %v", i, k1str, test.k1)
		}
		k2str := fmt.Sprintf("%064x", k2)
		if test.k2 != k2str {
			t.Errorf("%d: bad k2: got %v, want %v", i, k2str, test.k2)
		}
		if test.s1 != k1Sign {
			t.Errorf("%d: bad k1 sign: got %d, want %d", i, k1Sign, test.s1)
		}
		if test.s2 != k2Sign {
			t.Errorf("%d: bad k2 sign: got %d, want %d", i, k2Sign, test.s2)
		}
		k1Int := new(big.Int).SetBytes(k1)
		k1SignInt := new(big.Int).SetInt64(int64(k1Sign))
		k1Int.Mul(k1Int, k1SignInt)
		k2Int := new(big.Int).SetBytes(k2)
		k2SignInt := new(big.Int).SetInt64(int64(k2Sign))
		k2Int.Mul(k2Int, k2SignInt)
		gotK := new(big.Int).Mul(k2Int, endomorphismLambda)
		gotK.Add(k1Int, gotK)
		gotK.Mod(gotK, curveParams.N)
		if k.Cmp(gotK) != 0 {
			t.Errorf("%d: bad k: got %X, want %X", i, gotK.Bytes(), k.Bytes())
		}
	}
}

func TestSplitKRand(t *testing.T) {
	for i := 0; i < 1024; i++ {
		bytesK := make([]byte, 32)
		_, err := rand.Read(bytesK)
		if err != nil {
			t.Fatalf("failed to read random data at %d", i)
			break
		}
		k := new(big.Int).SetBytes(bytesK)
		k1, k2, k1Sign, k2Sign := splitK(bytesK)
		k1Int := new(big.Int).SetBytes(k1)
		k1SignInt := new(big.Int).SetInt64(int64(k1Sign))
		k1Int.Mul(k1Int, k1SignInt)
		k2Int := new(big.Int).SetBytes(k2)
		k2SignInt := new(big.Int).SetInt64(int64(k2Sign))
		k2Int.Mul(k2Int, k2SignInt)
		gotK := new(big.Int).Mul(k2Int, endomorphismLambda)
		gotK.Add(k1Int, gotK)
		gotK.Mod(gotK, curveParams.N)
		if k.Cmp(gotK) != 0 {
			t.Errorf("%d: bad k: got %X, want %X", i, gotK.Bytes(), k.Bytes())
		}
	}
}

// Test this curve's usage with the ecdsa package.
func testKeyGeneration(t *testing.T, tag string) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Errorf("%s: error: %s", tag, err)
		return
	}
	pub := priv.PubKey()
	if !isOnCurve(&pub.x, &pub.y) {
		t.Errorf("%s: public key invalid: %s", tag, err)
	}
}

func TestKeyGeneration(t *testing.T) {
	testKeyGeneration(t, "S256")
}

// TestDecompressY ensures that decompressY works as expected for some edge
// cases.
func TestDecompressY(t *testing.T) {
	tests := []struct {
		name      string // test description
		x         string // hex encoded x coordinate
		valid     bool   // expected decompress result
		wantOddY  string // hex encoded expected odd y coordinate
		wantEvenY string // hex encoded expected even y coordinate
	}{{
		name:      "x = 0 -- not a point on the curve",
		x:         "0",
		valid:     false,
		wantOddY:  "",
		wantEvenY: "",
	}, {
		name:      "x = 1",
		x:         "1",
		valid:     true,
		wantOddY:  "bde70df51939b94c9c24979fa7dd04ebd9b3572da7802290438af2a681895441",
		wantEvenY: "4218f20ae6c646b363db68605822fb14264ca8d2587fdd6fbc750d587e76a7ee",
	}, {
		name:      "x = secp256k1 prime (aka 0) -- not a point on the curve",
		x:         "fffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc2f",
		valid:     false,
		wantOddY:  "",
		wantEvenY: "",
	}, {
		name:      "x = secp256k1 prime - 1 -- not a point on the curve",
		x:         "fffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc2e",
		valid:     false,
		wantOddY:  "",
		wantEvenY: "",
	}, {
		name:      "x = secp256k1 group order",
		x:         "fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141",
		valid:     true,
		wantOddY:  "670999be34f51e8894b9c14211c28801d9a70fde24b71d3753854b35d07c9a11",
		wantEvenY: "98f66641cb0ae1776b463ebdee3d77fe2658f021db48e2c8ac7ab4c92f83621e",
	}, {
		name:      "x = secp256k1 group order - 1 -- not a point on the curve",
		x:         "fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140",
		valid:     false,
		wantOddY:  "",
		wantEvenY: "",
	}}

	for _, test := range tests {
		// Decompress the test odd y coordinate for the given test x coordinate
		// and ensure the returned validity flag matches the expected result.
		var oddY FieldVal
		fx := new(FieldVal).SetHex(test.x)
		valid := DecompressY(fx, true, &oddY)
		if valid != test.valid {
			t.Errorf("%s: unexpected valid flag -- got: %v, want: %v",
				test.name, valid, test.valid)
			continue
		}

		// Decompress the test even y coordinate for the given test x coordinate
		// and ensure the returned validity flag matches the expected result.
		var evenY FieldVal
		valid = DecompressY(fx, false, &evenY)
		if valid != test.valid {
			t.Errorf("%s: unexpected valid flag -- got: %v, want: %v",
				test.name, valid, test.valid)
			continue
		}

		// Skip checks related to the y coordinate when there isn't one.
		if !valid {
			continue
		}

		// Ensure the decompressed odd Y coordinate is the expected value.
		oddY.Normalize()
		wantOddY := new(FieldVal).SetHex(test.wantOddY)
		if !wantOddY.Equals(&oddY) {
			t.Errorf("%s: mismatched odd y\ngot: %v, want: %v", test.name,
				oddY, wantOddY)
			continue
		}

		// Ensure the decompressed even Y coordinate is the expected value.
		evenY.Normalize()
		wantEvenY := new(FieldVal).SetHex(test.wantEvenY)
		if !wantEvenY.Equals(&evenY) {
			t.Errorf("%s: mismatched even y\ngot: %v, want: %v", test.name,
				evenY, wantEvenY)
			continue
		}

		// Ensure the decompressed odd y coordinate is actually odd.
		if !oddY.IsOdd() {
			t.Errorf("%s: odd y coordinate is even", test.name)
			continue
		}

		// Ensure the decompressed even y coordinate is actually even.
		if evenY.IsOdd() {
			t.Errorf("%s: even y coordinate is odd", test.name)
			continue
		}
	}
}

// TestDecompressYRandom ensures that decompressY works as expected with
// randomly-generated x coordinates.
func TestDecompressYRandom(t *testing.T) {
	// Use a unique random seed each test instance and log it if the tests fail.
	seed := time.Now().Unix()
	rng := mrand.New(mrand.NewSource(seed))
	defer func(t *testing.T, seed int64) {
		if t.Failed() {
			t.Logf("random seed: %d", seed)
		}
	}(t, seed)

	for i := 0; i < 100; i++ {
		origX := randFieldVal(t, rng)

		// Calculate both corresponding y coordinates for the random x when it
		// is a valid coordinate.
		var oddY, evenY FieldVal
		x := new(FieldVal).Set(origX)
		oddSuccess := DecompressY(x, true, &oddY)
		evenSuccess := DecompressY(x, false, &evenY)

		// Ensure that the decompression success matches for both the even and
		// odd cases depending on whether or not x is a valid coordinate.
		if oddSuccess != evenSuccess {
			t.Fatalf("mismatched decompress success for x = %v -- odd: %v, "+
				"even: %v", x, oddSuccess, evenSuccess)
		}
		if !oddSuccess {
			continue
		}

		// Ensure the x coordinate was not changed.
		if !x.Equals(origX) {
			t.Fatalf("x coordinate changed -- orig: %v, changed: %v", origX, x)
		}

		// Ensure that the resulting y coordinates match their respective
		// expected oddness.
		oddY.Normalize()
		evenY.Normalize()
		if !oddY.IsOdd() {
			t.Fatalf("requested odd y is even for x = %v", x)
		}
		if evenY.IsOdd() {
			t.Fatalf("requested even y is odd for x = %v", x)
		}

		// Ensure that the resulting x and y coordinates are actually on the
		// curve for both cases.
		if !isOnCurve(x, &oddY) {
			t.Fatalf("(%v, %v) is not a valid point", x, oddY)
		}
		if !isOnCurve(x, &evenY) {
			t.Fatalf("(%v, %v) is not a valid point", x, evenY)
		}
	}
}
