// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package schnorr

import (
	"fmt"
	"math/big"

	"github.com/decred/dcrd/dcrec/secp256k1"
)

// Sha256VersionStringRFC6979 is the RFC6979 nonce version for a Schnorr signature
// over the secp256k1 curve using SHA256 as the hash function.
var Sha256VersionStringRFC6979 = []byte("Schnorr+SHA256  ")

// BlakeVersionStringRFC6979 is the RFC6979 nonce version for a Schnorr signature
// over the secp256k1 curve using BLAKE256 as the hash function.
var BlakeVersionStringRFC6979 = []byte("Schnorr+BLAKE256")

// combinePubkeys combines a slice of public keys into a single public key
// by adding them together with point addition.
func combinePubkeys(pks []*secp256k1.PublicKey) *secp256k1.PublicKey {
	numPubKeys := len(pks)
	curve := secp256k1.S256()

	// Have to have at least two pubkeys.
	if numPubKeys < 1 {
		return nil
	}
	if numPubKeys == 1 {
		return pks[0]
	}
	if pks[0] == nil || pks[1] == nil {
		return nil
	}

	var pkSumX *big.Int
	var pkSumY *big.Int

	pkSumX, pkSumY = curve.Add(pks[0].GetX(), pks[0].GetY(),
		pks[1].GetX(), pks[1].GetY())

	if numPubKeys > 2 {
		for i := 2; i < numPubKeys; i++ {
			pkSumX, pkSumY = curve.Add(pkSumX, pkSumY,
				pks[i].GetX(), pks[i].GetY())
		}
	}

	if !curve.IsOnCurve(pkSumX, pkSumY) {
		return nil
	}

	return secp256k1.NewPublicKey(pkSumX, pkSumY)
}

// nonceRFC6979 is a local instatiation of deterministic nonce generation
// by the standards of RFC6979.
func nonceRFC6979(privkey []byte, hash []byte, extra []byte,
	version []byte) []byte {
	pkD := new(big.Int).SetBytes(privkey)
	defer pkD.SetInt64(0)
	bigK := secp256k1.NonceRFC6979(pkD, hash, extra, version)
	defer bigK.SetInt64(0)
	k := bigIntToEncodedBytes(bigK)
	return k[:]
}

// generateNoncePair deterministically generate a nonce pair for use in
// partial signing of a message. Returns a public key (nonce to dissemanate)
// and a private nonce to keep as a secret for the signer.
func generateNoncePair(msg []byte, priv []byte,
	nonceFunction func([]byte, []byte, []byte, []byte) []byte, extra []byte,
	version []byte) ([]byte, *secp256k1.PublicKey, error) {
	k := nonceFunction(priv, msg, extra, version)
	bigK := new(big.Int).SetBytes(k)
	curve := secp256k1.S256()

	// k scalar sanity checks.
	if bigK.Cmp(bigZero) == 0 {
		str := fmt.Sprintf("k scalar is zero")
		return nil, nil, schnorrError(ErrBadNonce, str)
	}
	if bigK.Cmp(curve.N) >= 0 {
		str := fmt.Sprintf("k scalar is >= curve.N")
		return nil, nil, schnorrError(ErrBadNonce, str)
	}
	bigK.SetInt64(0)

	pubx, puby := curve.ScalarBaseMult(k)
	pubnonce := secp256k1.NewPublicKey(pubx, puby)

	return k, pubnonce, nil
}

// schnorrPartialSign creates a partial Schnorr signature which may be combined
// with other Schnorr signatures to create a valid signature for a group pubkey.
func schnorrPartialSign(msg []byte, priv []byte, privNonce []byte,
	pubSum *secp256k1.PublicKey, hashFunc func([]byte) []byte) (*Signature, error) {
	// Sanity checks.
	if len(msg) != scalarSize {
		str := fmt.Sprintf("wrong size for message (got %v, want %v)",
			len(msg), scalarSize)
		return nil, schnorrError(ErrBadInputSize, str)
	}
	if len(priv) != scalarSize {
		str := fmt.Sprintf("wrong size for privkey (got %v, want %v)",
			len(priv), scalarSize)
		return nil, schnorrError(ErrBadInputSize, str)
	}
	if len(privNonce) != scalarSize {
		str := fmt.Sprintf("wrong size for privnonce (got %v, want %v)",
			len(privNonce), scalarSize)
		return nil, schnorrError(ErrBadInputSize, str)
	}
	if pubSum == nil {
		str := fmt.Sprintf("nil pubkey")
		return nil, schnorrError(ErrInputValue, str)
	}

	curve := secp256k1.S256()
	privBig := new(big.Int).SetBytes(priv)
	if privBig.Cmp(bigZero) == 0 {
		str := fmt.Sprintf("priv scalar is zero")
		return nil, schnorrError(ErrInputValue, str)
	}
	if privBig.Cmp(curve.N) >= 0 {
		str := fmt.Sprintf("priv scalar is out of bounds")
		return nil, schnorrError(ErrInputValue, str)
	}
	privBig.SetInt64(0)

	privNonceBig := new(big.Int).SetBytes(privNonce)
	if privNonceBig.Cmp(bigZero) == 0 {
		str := fmt.Sprintf("privNonce scalar is zero")
		return nil, schnorrError(ErrInputValue, str)
	}
	if privNonceBig.Cmp(curve.N) >= 0 {
		str := fmt.Sprintf("privNonce scalar is out of bounds")
		return nil, schnorrError(ErrInputValue, str)
	}
	privNonceBig.SetInt64(0)

	if !curve.IsOnCurve(pubSum.GetX(), pubSum.GetY()) {
		str := fmt.Sprintf("public key sum is off curve")
		return nil, schnorrError(ErrInputValue, str)
	}

	return schnorrSign(msg, priv, privNonce, pubSum.GetX(),
		pubSum.GetY(), hashFunc)
}

// schnorrCombineSigs combines a list of partial Schnorr signatures s values
// into a complete signature s for some group public key. This is achieved
// by simply adding the s values of the partial signatures as scalars.
func schnorrCombineSigs(sigss [][]byte) (*big.Int,
	error) {
	curve := secp256k1.S256()
	combinedSigS := new(big.Int).SetInt64(0)
	for i, sigs := range sigss {
		sigsBI := encodedBytesToBigInt(copyBytes(sigs))
		if sigsBI.Cmp(bigZero) == 0 {
			str := fmt.Sprintf("sig s %v is zero", i)
			return nil, schnorrError(ErrInputValue, str)
		}
		if sigsBI.Cmp(curve.N) >= 0 {
			str := fmt.Sprintf("sig s %v is out of bounds", i)
			return nil, schnorrError(ErrInputValue, str)
		}

		combinedSigS.Add(combinedSigS, sigsBI)
		combinedSigS.Mod(combinedSigS, curve.N)
	}

	if combinedSigS.Cmp(bigZero) == 0 {
		str := fmt.Sprintf("combined sig s %v is zero", combinedSigS)
		return nil, schnorrError(ErrZeroSigS, str)
	}

	return combinedSigS, nil
}

// combineSigs is the generalized and exported version of
// generateNoncePair.
func combineSigs(sigs []*Signature) (*Signature, error) {
	sigss := make([][]byte, len(sigs))
	for i, sig := range sigs {
		if sig == nil {
			return nil, fmt.Errorf("nil signature")
		}

		if i > 0 {
			if sigs[i-1].GetR().Cmp(sig.GetR()) != 0 {
				str := fmt.Sprintf("nonmatching r values for idx %v, %v",
					i, i-1)
				return nil, schnorrError(ErrNonmatchingR, str)
			}
		}

		sigss[i] = bigIntToEncodedBytes(sig.GetS())[:]
	}

	combinedSigS, err := schnorrCombineSigs(sigss)
	if err != nil {
		return nil, err
	}

	return NewSignature(sigs[0].R, combinedSigS), nil
}
