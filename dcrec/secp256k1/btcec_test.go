// Copyright 2011 The Go Authors. All rights reserved.
// Copyright (c) 2015-2017 The Decred developers
// Copyright 2011 ThePiachu. All rights reserved.
// Copyright 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"
)

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

func TestOnCurve(t *testing.T) {
	s256 := S256()
	if !s256.IsOnCurve(s256.Params().Gx, s256.Params().Gy) {
		t.Errorf("FAIL S256")
	}
}

type baseMultTest struct {
	k    string
	x, y string
}

//TODO: add more test vectors
var s256BaseMultTests = []baseMultTest{
	{
		"AA5E28D6A97A2479A65527F7290311A3624D4CC0FA1578598EE3C2613BF99522",
		"34F9460F0E4F08393D192B3C5133A6BA099AA0AD9FD54EBCCFACDFA239FF49C6",
		"B71EA9BD730FD8923F6D25A7A91E7DD7728A960686CB5A901BB419E0F2CA232",
	},
	{
		"7E2B897B8CEBC6361663AD410835639826D590F393D90A9538881735256DFAE3",
		"D74BF844B0862475103D96A611CF2D898447E288D34B360BC885CB8CE7C00575",
		"131C670D414C4546B88AC3FF664611B1C38CEB1C21D76369D7A7A0969D61D97D",
	},
	{
		"6461E6DF0FE7DFD05329F41BF771B86578143D4DD1F7866FB4CA7E97C5FA945D",
		"E8AECC370AEDD953483719A116711963CE201AC3EB21D3F3257BB48668C6A72F",
		"C25CAF2F0EBA1DDB2F0F3F47866299EF907867B7D27E95B3873BF98397B24EE1",
	},
	{
		"376A3A2CDCD12581EFFF13EE4AD44C4044B8A0524C42422A7E1E181E4DEECCEC",
		"14890E61FCD4B0BD92E5B36C81372CA6FED471EF3AA60A3E415EE4FE987DABA1",
		"297B858D9F752AB42D3BCA67EE0EB6DCD1C2B7B0DBE23397E66ADC272263F982",
	},
	{
		"1B22644A7BE026548810C378D0B2994EEFA6D2B9881803CB02CEFF865287D1B9",
		"F73C65EAD01C5126F28F442D087689BFA08E12763E0CEC1D35B01751FD735ED3",
		"F449A8376906482A84ED01479BD18882B919C140D638307F0C0934BA12590BDE",
	},
}

//TODO: test different curves as well?
func TestBaseMult(t *testing.T) {
	s256 := S256()
	for i, e := range s256BaseMultTests {
		k, ok := new(big.Int).SetString(e.k, 16)
		if !ok {
			t.Errorf("%d: bad value for k: %s", i, e.k)
		}
		x, y := s256.ScalarBaseMult(k.Bytes())
		if fmt.Sprintf("%X", x) != e.x || fmt.Sprintf("%X", y) != e.y {
			t.Errorf("%d: bad output for k=%s: got (%X, %X), want (%s, %s)", i, e.k, x, y, e.x, e.y)
		}
		if testing.Short() && i > 5 {
			break
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
	// Get a random exponent from the generator point at first
	// This creates a new point which is used in the next iteration
	// Use another random exponent on the new point.
	// We use BaseMult to verify by multiplying the previous exponent
	// and the new random exponent together (mod N)
	s256 := S256()
	x, y := s256.Gx, s256.Gy
	exponent := big.NewInt(1)
	for i := 0; i < 1024; i++ {
		data := make([]byte, 32)
		_, err := rand.Read(data)
		if err != nil {
			t.Fatalf("failed to read random data at %d", i)
			break
		}
		x, y = s256.ScalarMult(x, y, data)
		exponent.Mul(exponent, new(big.Int).SetBytes(data))
		xWant, yWant := s256.ScalarBaseMult(exponent.Bytes())
		if x.Cmp(xWant) != 0 || y.Cmp(yWant) != 0 {
			t.Fatalf("%d: bad output for %X: got (%X, %X), want (%X, %X)", i, data, x, y, xWant, yWant)
			break
		}
	}
}

// Test this curve's usage with the ecdsa package.
func testKeyGeneration(t *testing.T, c *KoblitzCurve, tag string) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Errorf("%s: error: %s", tag, err)
		return
	}
	if !c.IsOnCurve(priv.PublicKey.X, priv.PublicKey.Y) {
		t.Errorf("%s: public key invalid: %s", tag, err)
	}
}

func TestKeyGeneration(t *testing.T) {
	testKeyGeneration(t, S256(), "S256")
}

func testSignAndVerify(t *testing.T, c *KoblitzCurve, tag string) {
	priv, _ := GeneratePrivateKey()
	pubx, puby := priv.Public()
	pub := NewPublicKey(pubx, puby)

	hashed := []byte("testing")
	sig, err := priv.Sign(hashed)
	if err != nil {
		t.Errorf("%s: error signing: %s", tag, err)
		return
	}

	if !sig.Verify(hashed, pub) {
		t.Errorf("%s: Verify failed", tag)
	}

	hashed[0] ^= 0xff
	if sig.Verify(hashed, pub) {
		t.Errorf("%s: Verify always works!", tag)
	}
}

func TestSignAndVerify(t *testing.T) {
	testSignAndVerify(t, S256(), "S256")
}

func TestNAF(t *testing.T) {
	tests := []string{
		"6df2b5d30854069ccdec40ae022f5c948936324a4e9ebed8eb82cfd5a6b6d766",
		"b776e53fb55f6b006a270d42d64ec2b1",
		"d6cc32c857f1174b604eefc544f0c7f7",
		"45c53aa1bb56fcd68c011e2dad6758e4",
		"a2e79d200f27f2360fba57619936159b",
	}
	negOne := big.NewInt(-1)
	one := big.NewInt(1)
	two := big.NewInt(2)
	for i, test := range tests {
		want, _ := new(big.Int).SetString(test, 16)
		nafPos, nafNeg := NAF(want.Bytes())
		got := big.NewInt(0)
		// Check that the NAF representation comes up with the right number
		for i := 0; i < len(nafPos); i++ {
			bytePos := nafPos[i]
			byteNeg := nafNeg[i]
			for j := 7; j >= 0; j-- {
				got.Mul(got, two)
				if bytePos&0x80 == 0x80 {
					got.Add(got, one)
				} else if byteNeg&0x80 == 0x80 {
					got.Add(got, negOne)
				}
				bytePos <<= 1
				byteNeg <<= 1
			}
		}
		if got.Cmp(want) != 0 {
			t.Errorf("%d: Failed NAF got %X want %X", i, got, want)
		}
	}
}

func TestNAFRand(t *testing.T) {
	negOne := big.NewInt(-1)
	one := big.NewInt(1)
	two := big.NewInt(2)
	for i := 0; i < 1024; i++ {
		data := make([]byte, 32)
		_, err := rand.Read(data)
		if err != nil {
			t.Fatalf("failed to read random data at %d", i)
			break
		}
		nafPos, nafNeg := NAF(data)
		want := new(big.Int).SetBytes(data)
		got := big.NewInt(0)
		// Check that the NAF representation comes up with the right number
		for i := 0; i < len(nafPos); i++ {
			bytePos := nafPos[i]
			byteNeg := nafNeg[i]
			for j := 7; j >= 0; j-- {
				got.Mul(got, two)
				if bytePos&0x80 == 0x80 {
					got.Add(got, one)
				} else if byteNeg&0x80 == 0x80 {
					got.Add(got, negOne)
				}
				bytePos <<= 1
				byteNeg <<= 1
			}
		}
		if got.Cmp(want) != 0 {
			t.Errorf("%d: Failed NAF got %X want %X", i, got, want)
		}
	}
}
