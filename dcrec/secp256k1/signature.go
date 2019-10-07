// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"math/big"

	v2 "github.com/decred/dcrd/dcrec/secp256k1/v2"
)

// Signature is a type representing an ecdsa signature.
type Signature = v2.Signature

// NewSignature instantiates a new signature given some R,S values.
func NewSignature(r, s *big.Int) *Signature {
	return v2.NewSignature(r,s)
}

// ParseSignature parses a signature in BER format for the curve type `curve'
// into a Signature type, perfoming some basic sanity checks.  If parsing
// according to the more strict DER format is needed, use ParseDERSignature.
func ParseSignature(sigStr []byte) (*Signature, error) {
	return v2.ParseSignature(sigStr)
}

// ParseDERSignature parses a signature in DER format for the curve type
// `curve` into a Signature type.  If parsing according to the less strict
// BER format is needed, use ParseSignature.
func ParseDERSignature(sigStr []byte) (*Signature, error) {
	return v2.ParseDERSignature(sigStr)
}

// SignCompact produces a compact signature of the data in hash with the given
// private key on the given koblitz curve. The isCompressed  parameter should
// be used to detail if the given signature should reference a compressed
// public key or not. If successful the bytes of the compact signature will be
// returned in the format:
// <(byte of 27+public key solution)+4 if compressed >< padded bytes for signature R><padded bytes for signature S>
// where the R and S parameters are padde up to the bitlengh of the curve.
func SignCompact(key *PrivateKey, hash []byte, isCompressedKey bool) ([]byte, error) {
	return v2.SignCompact(key, hash, isCompressedKey)
}

// RecoverCompact verifies the compact signature "signature" of "hash" for the
// Koblitz curve in "curve". If the signature matches then the recovered public
// key will be returned as well as a boolen if the original key was compressed
// or not, else an error will be returned.
func RecoverCompact(signature, hash []byte) (*PublicKey, bool, error) {
	return v2.RecoverCompact(signature, hash)
}

// NonceRFC6979 generates an ECDSA nonce (`k`) deterministically according to
// RFC 6979. It takes a 32-byte hash as an input and returns 32-byte nonce to
// be used in ECDSA algorithm.
func NonceRFC6979(privkey *big.Int, hash []byte, extra []byte, version []byte) *big.Int {
	return v2.NonceRFC6979(privkey, hash, extra, version)
}
