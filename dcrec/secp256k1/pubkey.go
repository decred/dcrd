// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"math/big"

	v2 "github.com/decred/dcrd/dcrec/secp256k1/v2"
)

// These constants define the lengths of serialized public keys.
const (
	PubKeyBytesLenCompressed   = 33
	PubKeyBytesLenUncompressed = 65
)

// PublicKey is an ecdsa.PublicKey with additional functions to
// serialize in uncompressed and compressed formats.
type PublicKey = v2.PublicKey

const (
	pubkeyCompressed   byte = 0x2 // y_bit + x coord
	pubkeyUncompressed byte = 0x4 // x coord + y coord
)

// NewPublicKey instantiates a new public key with the given X,Y coordinates.
func NewPublicKey(x *big.Int, y *big.Int) *PublicKey {
	return v2.NewPublicKey(x, y)
}

// ParsePubKey parses a public key for a koblitz curve from a bytestring into a
// ecdsa.Publickey, verifying that it is valid. It supports compressed and
// uncompressed signature formats, but not the hybrid format.
func ParsePubKey(pubKeyStr []byte) (key *PublicKey, err error) {
	return v2.ParsePubKey(pubKeyStr)
}
