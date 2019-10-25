// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrec

// SignatureType defines a specific cryptographic signature and curve pair for
// use in transaction scripts and addresses.
type SignatureType int

const (
	// STEcdsaSecp256k1 specifies that the signature is an ECDSA signature
	// over the secp256k1 elliptic curve.
	STEcdsaSecp256k1 SignatureType = 1

	// STEd25519 specifies that the signature is an ECDSA signature over the
	// edwards25519 twisted Edwards curve.
	STEd25519 = 2

	// STSchnorrSecp256k1 specifies that the signature is a Schnorr
	// signature over the secp256k1 elliptic curve.
	STSchnorrSecp256k1 = 3
)
