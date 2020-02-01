// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"crypto/ecdsa"
	"crypto/rand"
	"math/big"
)

// PrivateKey provides facilities for working with secp256k1 private keys within
// this package and includes functionality such as serializing and parsing them
// as well as computing their associated public key.
type PrivateKey struct {
	D *big.Int
}

// NewPrivateKey instantiates a new private key from a scalar encoded as a
// big integer.
func NewPrivateKey(d *big.Int) *PrivateKey {
	b := make([]byte, 0, PrivKeyBytesLen)
	dB := paddedAppend(PrivKeyBytesLen, b, d.Bytes())
	return PrivKeyFromBytes(dB)
}

// PrivKeyFromBytes returns a private and public key for `curve' based on the
// private key passed as an argument as a byte slice.
func PrivKeyFromBytes(pk []byte) *PrivateKey {
	return &PrivateKey{
		D: new(big.Int).SetBytes(pk),
	}
}

// GeneratePrivateKey returns a private key that is suitable for use with
// secp256k1.
func GeneratePrivateKey() (*PrivateKey, error) {
	key, err := ecdsa.GenerateKey(S256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	return &PrivateKey{
		D: key.D,
	}, nil
}

// PubKey computes and returns the public key corresponding to this private key.
// PubKey returns the PublicKey corresponding to this private key.
func (p *PrivateKey) PubKey() *PublicKey {
	var result jacobianPoint
	scalarBaseMultJacobian(p.D.Bytes(), &result)
	return NewPublicKey(jacobianToBigAffine(&result))
}

// Sign generates an ECDSA signature for the provided hash (which should be the
// result of hashing a larger message) using the private key. Produced signature
// is deterministic (same message and same key yield the same signature) and
// canonical in accordance with RFC6979 and BIP0062.
func (p *PrivateKey) Sign(hash []byte) *Signature {
	return signRFC6979(p, hash)
}

// PrivKeyBytesLen defines the length in bytes of a serialized private key.
const PrivKeyBytesLen = 32

// Serialize returns the private key as a big-endian binary-encoded number,
// padded to a length of 32 bytes.
func (p PrivateKey) Serialize() []byte {
	b := make([]byte, 0, PrivKeyBytesLen)
	return paddedAppend(PrivKeyBytesLen, b, p.D.Bytes())
}
