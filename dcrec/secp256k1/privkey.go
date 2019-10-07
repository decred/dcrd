// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"io"
	"math/big"

	v2 "github.com/decred/dcrd/dcrec/secp256k1/v2"
)

// PrivateKey wraps an ecdsa.PrivateKey as a convenience mainly for signing
// things with the the private key without having to directly import the ecdsa
// package.
type PrivateKey = v2.PrivateKey

// NewPrivateKey instantiates a new private key from a scalar encoded as a
// big integer.
func NewPrivateKey(d *big.Int) *PrivateKey {
	return v2.NewPrivateKey(d)
}

// PrivKeyFromBytes returns a private and public key for `curve' based on the
// private key passed as an argument as a byte slice.
func PrivKeyFromBytes(pk []byte) (*PrivateKey, *PublicKey) {
	return v2.PrivKeyFromBytes(pk)
}

// PrivKeyFromScalar is the same as PrivKeyFromBytes in secp256k1.
func PrivKeyFromScalar(s []byte) (*PrivateKey, *PublicKey) {
	return PrivKeyFromBytes(s)
}

// GeneratePrivateKey is a wrapper for ecdsa.GenerateKey that returns a PrivateKey
// instead of the normal ecdsa.PrivateKey.
func GeneratePrivateKey() (*PrivateKey, error) {
	return v2.GeneratePrivateKey()
}

// GenerateKey generates a key using a random number generator, returning
// the private scalar and the corresponding public key points.
func GenerateKey(rand io.Reader) ([]byte, *big.Int, *big.Int, error) {
	return v2.GenerateKey(rand)
}

// PrivKeyBytesLen defines the length in bytes of a serialized private key.
const PrivKeyBytesLen = 32
