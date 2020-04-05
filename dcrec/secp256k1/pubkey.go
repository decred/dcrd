// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
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

func isOdd(a *big.Int) bool {
	return a.Bit(0) == 1
}

// decompressPoint decompresses a point on the given curve given the X point and
// the solution to use.
func decompressPoint(x *big.Int, ybit bool) (*big.Int, error) {
	var fx, fy FieldVal
	if overflow := fx.SetByteSlice(x.Bytes()); overflow {
		str := "invalid public key: x >= field prime"
		return nil, makeError(ErrPubKeyXTooBig, str)
	}
	if !DecompressY(&fx, ybit, &fy) {
		str := fmt.Sprintf("invalid public key: x coordinate %v is not on the "+
			"secp256k1 curve", fx)
		return nil, makeError(ErrPubKeyNotOnCurve, str)
	}
	fy.Normalize()

	return new(big.Int).SetBytes(fy.Bytes()[:]), nil
}

const (
	pubkeyCompressed   byte = 0x2 // y_bit + x coord
	pubkeyUncompressed byte = 0x4 // x coord + y coord
	pubkeyHybrid       byte = 0x6 // y_bit + x coord + y coord
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
		str := "invalid public key: empty"
		return nil, makeError(ErrPubKeyInvalidLen, str)
	}

	format := pubKeyStr[0]
	ybit := (format & 0x1) == 0x1
	format &= ^byte(0x1)

	switch len(pubKeyStr) {
	case PubKeyBytesLenUncompressed:
		if format != pubkeyUncompressed && format != pubkeyHybrid {
			str := fmt.Sprintf("invalid public key: unsupported format: %x",
				format)
			return nil, makeError(ErrPubKeyInvalidFormat, str)
		}

		pubkey.x = new(big.Int).SetBytes(pubKeyStr[1:33])
		pubkey.y = new(big.Int).SetBytes(pubKeyStr[33:])
		// hybrid keys have extra information, make use of it.
		if format == pubkeyHybrid && ybit != isOdd(pubkey.y) {
			str := fmt.Sprintf("invalid public key: y oddness does not match "+
				"specified value of %v", ybit)
			return nil, makeError(ErrPubKeyMismatchedOddness, str)
		}

	case PubKeyBytesLenCompressed:
		// format is 0x2 | solution, <X coordinate>
		// solution determines which solution of the curve we use.
		/// y^2 = x^3 + Curve.B
		if format != pubkeyCompressed {
			str := fmt.Sprintf("invalid public key: unsupported format: %x",
				format)
			return nil, makeError(ErrPubKeyInvalidFormat, str)
		}
		pubkey.x = new(big.Int).SetBytes(pubKeyStr[1:33])
		pubkey.y, err = decompressPoint(pubkey.x, ybit)
		if err != nil {
			return nil, err
		}

	default: // wrong!
		str := fmt.Sprintf("malformed public key: invalid length: %d",
			len(pubKeyStr))
		return nil, makeError(ErrPubKeyInvalidLen, str)
	}

	curve := S256()
	if pubkey.x.Cmp(curveParams.P) >= 0 {
		str := "invalid public key: x >= field prime"
		return nil, makeError(ErrPubKeyXTooBig, str)
	}
	if pubkey.y.Cmp(curveParams.P) >= 0 {
		str := "invalid public key: y >= field prime"
		return nil, makeError(ErrPubKeyYTooBig, str)
	}
	if !curve.IsOnCurve(pubkey.x, pubkey.y) {
		str := fmt.Sprintf("invalid public key: [%v,%v] not on secp256k1 curve",
			pubkey.x, pubkey.y)
		return nil, makeError(ErrPubKeyNotOnCurve, str)
	}
	return &pubkey, nil
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

// AsJacobian converts the public key into a Jacobian point with Z=1 and stores
// the result in the provided result param.  This allows the public key to be
// treated a Jacobian point in the secp256k1 group in calculations.
func (p *PublicKey) AsJacobian(result *JacobianPoint) {
	bigAffineToJacobian(p.x, p.y, result)
}

// IsOnCurve returns whether or not the public key represents a point on the
// secp256k1 curve.
func (p *PublicKey) IsOnCurve() bool {
	var point JacobianPoint
	bigAffineToJacobian(p.x, p.y, &point)
	return isOnCurve(&point.X, &point.Y)
}
