// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package schnorr

import (
	"fmt"
	"math/big"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec/secp256k1/v3"
)

// Signature is a type representing a Schnorr signature.
type Signature struct {
	r *big.Int
	s *big.Int
}

// SignatureSize is the size of an encoded Schnorr signature.
const SignatureSize = 64

// NewSignature instantiates a new signature given some R,S values.
func NewSignature(r, s *big.Int) *Signature {
	return &Signature{r, s}
}

// Serialize returns the Schnorr signature in the more strict format.
//
// The signatures are encoded as
//   sig[0:32]  R, a point encoded as big endian
//   sig[32:64] S, scalar multiplication/addition results = (ab+c) mod l
//     encoded also as big endian
func (sig Signature) Serialize() []byte {
	rBytes := bigIntToEncodedBytes(sig.r)
	sBytes := bigIntToEncodedBytes(sig.s)

	all := append(rBytes[:], sBytes[:]...)

	return all
}

func parseSig(sigStr []byte) (*Signature, error) {
	if len(sigStr) != SignatureSize {
		return nil, fmt.Errorf("bad signature size; have %v, want %v",
			len(sigStr), SignatureSize)
	}

	rBytes := copyBytes(sigStr[0:32])
	r := encodedBytesToBigInt(rBytes)
	sBytes := copyBytes(sigStr[32:64])
	s := encodedBytesToBigInt(sBytes)

	return &Signature{r, s}, nil
}

// ParseSignature parses a signature in BER format for the curve type `curve'
// into a Signature type, performing some basic sanity checks.
func ParseSignature(sigStr []byte) (*Signature, error) {
	return parseSig(sigStr)
}

// IsEqual compares this Signature instance to the one passed, returning true
// if both Signatures are equivalent. A signature is equivalent to another, if
// they both have the same scalar value for R and S.
func (sig Signature) IsEqual(otherSig *Signature) bool {
	return sig.r.Cmp(otherSig.r) == 0 &&
		sig.s.Cmp(otherSig.s) == 0
}

// Verify is the generalized and exported function for the verification of a
// secp256k1 Schnorr signature. BLAKE256 is used as the hashing function.
func (sig Signature) Verify(msg []byte, pubkey *secp256k1.PublicKey) bool {
	ok, _ := schnorrVerify(sig.Serialize(), pubkey, msg, chainhash.HashB)
	return ok
}
