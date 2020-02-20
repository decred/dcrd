// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"errors"
	"fmt"
	"math/big"
)

// These constants define the lengths of serialized public keys.
const (
	PubKeyBytesLenCompressed   = 33
	PubKeyBytesLenUncompressed = 65
)

// PublicKey provides facilities for efficiently working with secp256k1 private
// keys within this package and includes functions to serialize in both
// uncompressed and compressed SEC (Standards for Efficient Cryptography)
// formats.
type PublicKey struct {
	x *big.Int
	y *big.Int
}

// X returns the x coordinate of the public key.
func (p *PublicKey) X() *big.Int {
	return p.x
}

// Y returns the y coordinate of the public key.
func (p *PublicKey) Y() *big.Int {
	return p.y
}

// NewPublicKey instantiates a new public key with the given X,Y coordinates.
func NewPublicKey(x *big.Int, y *big.Int) *PublicKey {
	return &PublicKey{
		x: x,
		y: y,
	}
}

// decompressY attempts to calculate the Y coordinate for the given X coordinate
// such that the result pair is a point on the secp256k1 curve.  It adjusts Y
// based on the desired oddness and returns whether or not it was successful
// since not all X coordinates are valid.
//
// The magnitude of the provided X coordinate field val must be a max of 8 for a
// correct result.  The resulting Y field val will have a max magnitude of 2.
func decompressY(x *fieldVal, odd bool, resultY *fieldVal) bool {
	// The curve equation for secp256k1 is: y^2 = x^3 + 7.  Thus
	// y = +-sqrt(x^3 + 7).
	//
	// The x coordinate must be invalid if there is no square root for the
	// calculated rhs because it means the X coordinate is not for a point on
	// the curve.
	x3PlusB := new(fieldVal).SquareVal(x).Mul(x).AddInt(7)
	if hasSqrt := resultY.SquareRootVal(x3PlusB); !hasSqrt {
		return false
	}
	if resultY.Normalize().IsOdd() != odd {
		resultY.Negate(1)
	}
	return true
}

func isOdd(a *big.Int) bool {
	return a.Bit(0) == 1
}

// decompressPoint decompresses a point on the given curve given the X point and
// the solution to use.
func decompressPoint(x *big.Int, ybit bool) (*big.Int, error) {
	var fy fieldVal
	fx := new(fieldVal).SetByteSlice(x.Bytes())
	if !decompressY(fx, ybit, &fy) {
		return nil, fmt.Errorf("invalid public key x coordinate")
	}
	fy.Normalize()

	return new(big.Int).SetBytes(fy.Bytes()[:]), nil
}

const (
	pubkeyCompressed   byte = 0x2 // y_bit + x coord
	pubkeyUncompressed byte = 0x4 // x coord + y coord
)

// ParsePubKey parses a secp256k1 public key encoded according to the format
// specified by ANSI X9.62-1998, which means it is also compatible with the
// SEC (Standards for Efficient Cryptography) specification which is a subset of
// the former.  In other words, it supports the uncompressed, compressed, and
// hybrid formats as follows:
//
// Compressed:
//   <format byte = 0x02/0x03><32-byte X coordinate>
// Uncompressed:
//   <format byte = 0x04><32-byte X coordinate><32-byte Y coordinate>
// Hybrid:
//   <format byte = 0x05/0x06><32-byte X coordinate><32-byte Y coordinate>
//
// NOTE: The hybrid format makes little sense in practice an therefore this
// package will not produce public keys serialized in this format.  However,
// this function will properly parse them since they exist in the wild.
func ParsePubKey(pubKeyStr []byte) (key *PublicKey, err error) {
	pubkey := PublicKey{}

	if len(pubKeyStr) == 0 {
		return nil, errors.New("pubkey string is empty")
	}

	format := pubKeyStr[0]
	ybit := (format & 0x1) == 0x1
	format &= ^byte(0x1)

	switch len(pubKeyStr) {
	case PubKeyBytesLenUncompressed:
		if format != pubkeyUncompressed {
			return nil, fmt.Errorf("invalid magic in pubkey str: "+
				"%d", pubKeyStr[0])
		}

		pubkey.x = new(big.Int).SetBytes(pubKeyStr[1:33])
		pubkey.y = new(big.Int).SetBytes(pubKeyStr[33:])
	case PubKeyBytesLenCompressed:
		// format is 0x2 | solution, <X coordinate>
		// solution determines which solution of the curve we use.
		/// y^2 = x^3 + Curve.B
		if format != pubkeyCompressed {
			return nil, fmt.Errorf("invalid magic in compressed "+
				"pubkey string: %d", pubKeyStr[0])
		}
		pubkey.x = new(big.Int).SetBytes(pubKeyStr[1:33])
		pubkey.y, err = decompressPoint(pubkey.x, ybit)
		if err != nil {
			return nil, err
		}
	default: // wrong!
		return nil, fmt.Errorf("invalid pub key length %d",
			len(pubKeyStr))
	}

	curve := S256()
	if pubkey.x.Cmp(curveParams.P) >= 0 {
		return nil, fmt.Errorf("pubkey X parameter is >= to P")
	}
	if pubkey.y.Cmp(curveParams.P) >= 0 {
		return nil, fmt.Errorf("pubkey Y parameter is >= to P")
	}
	if !curve.IsOnCurve(pubkey.x, pubkey.y) {
		return nil, fmt.Errorf("pubkey [%v,%v] isn't on secp256k1 curve",
			pubkey.x, pubkey.y)
	}
	return &pubkey, nil
}

// SerializeUncompressed serializes a public key in a 65-byte uncompressed
// format.
func (p PublicKey) SerializeUncompressed() []byte {
	b := make([]byte, 0, PubKeyBytesLenUncompressed)
	b = append(b, pubkeyUncompressed)
	b = paddedAppend(32, b, p.x.Bytes())
	return paddedAppend(32, b, p.y.Bytes())
}

// SerializeCompressed serializes a public key in a 33-byte compressed format.
func (p PublicKey) SerializeCompressed() []byte {
	b := make([]byte, 0, PubKeyBytesLenCompressed)
	format := pubkeyCompressed
	if isOdd(p.y) {
		format |= 0x1
	}
	b = append(b, format)
	return paddedAppend(32, b, p.x.Bytes())
}

// IsEqual compares this PublicKey instance to the one passed, returning true if
// both PublicKeys are equivalent. A PublicKey is equivalent to another, if they
// both have the same X and Y coordinate.
func (p *PublicKey) IsEqual(otherPubKey *PublicKey) bool {
	return p.x.Cmp(otherPubKey.x) == 0 && p.y.Cmp(otherPubKey.y) == 0
}

// paddedAppend appends the src byte slice to dst, returning the new slice.
// If the length of the source is smaller than the passed size, leading zero
// bytes are appended to the dst slice before appending src.
func paddedAppend(size uint, dst, src []byte) []byte {
	for i := 0; i < int(size)-len(src); i++ {
		dst = append(dst, 0)
	}
	return append(dst, src...)
}
