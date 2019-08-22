// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package edwards

import (
	"fmt"
	"math/big"
)

// Sha512VersionStringRFC6979 is the RFC6979 nonce version for a Schnorr signature
// over the Curve25519 curve using BLAKE256 as the hash function.
var Sha512VersionStringRFC6979 = []byte("Edwards+SHA512  ")

// combinePubkeys combines a slice of public keys into a single public key
// by adding them together with point addition.
func combinePubkeys(pks []*PublicKey) *PublicKey {
	numPubKeys := len(pks)

	// Have to have at least two pubkeys.
	if numPubKeys < 1 {
		return nil
	}
	if numPubKeys == 1 {
		return pks[0]
	}
	if pks == nil {
		return nil
	}
	if pks[0] == nil || pks[1] == nil {
		return nil
	}

	curve := Edwards()
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

	return NewPublicKey(pkSumX, pkSumY)
}

// generateNoncePair deterministically generate a nonce pair for use in
// partial signing of a message. Returns a public key (nonce to disseminate)
// and a private nonce to keep as a secret for the signer.
func generateNoncePair(msg []byte, priv []byte,
	nonceFunction func([]byte, []byte, []byte,
		[]byte) []byte, extra []byte, version []byte) ([]byte, *PublicKey, error) {

	curve := Edwards()
	k := nonceFunction(priv, msg, extra, version)
	bigK := new(big.Int).SetBytes(k)
	bigK.Mod(bigK, curve.N)

	// k scalar sanity checks.
	if bigK.Cmp(zero) == 0 {
		return nil, nil, fmt.Errorf("k scalar is zero")
	}
	if bigK.Cmp(curve.N) >= 0 {
		return nil, nil, fmt.Errorf("k scalar is >= curve.N")
	}
	bigK.SetInt64(0)

	pubx, puby := curve.ScalarBaseMult(k)
	pubnonce := NewPublicKey(pubx, puby)

	return k, pubnonce, nil
}

// schnorrPartialSign creates a partial Schnorr signature which may be combined
// with other Schnorr signatures to create a valid signature for a group pubkey.
func schnorrPartialSign(msg []byte, priv []byte,
	groupPublicKey []byte, privNonce []byte, pubNonceSum []byte) (*big.Int,
	*big.Int, error) {

	// Sanity checks.
	if len(msg) != PrivScalarSize {
		str := fmt.Sprintf("wrong size for message (got %v, want %v)",
			len(msg), PrivScalarSize)
		return nil, nil, fmt.Errorf("%v", str)
	}
	if len(priv) != PrivScalarSize {
		str := fmt.Sprintf("wrong size for privkey (got %v, want %v)",
			len(priv), PrivScalarSize)
		return nil, nil, fmt.Errorf("%v", str)
	}
	if len(privNonce) != PrivScalarSize {
		str := fmt.Sprintf("wrong size for privnonce (got %v, want %v)",
			len(privNonce), PrivScalarSize)
		return nil, nil, fmt.Errorf("%v", str)
	}
	if len(groupPublicKey) != PubKeyBytesLen {
		str := fmt.Sprintf("wrong size for group public key (got %v, want %v)",
			len(privNonce), PubKeyBytesLen)
		return nil, nil, fmt.Errorf("%v", str)
	}
	if len(pubNonceSum) != PubKeyBytesLen {
		str := fmt.Sprintf("wrong size for group nonce public key (got %v, "+
			"want %v)",
			len(privNonce), PubKeyBytesLen)
		return nil, nil, fmt.Errorf("%v", str)
	}

	curve := Edwards()
	privBig := new(big.Int).SetBytes(priv)
	if privBig.Cmp(zero) == 0 {
		str := fmt.Sprintf("priv scalar is zero")
		return nil, nil, fmt.Errorf("%v", str)
	}
	if privBig.Cmp(curve.N) >= 0 {
		str := fmt.Sprintf("priv scalar is out of bounds")
		return nil, nil, fmt.Errorf("%v", str)
	}
	privBig.SetInt64(0)

	privNonceBig := new(big.Int).SetBytes(privNonce)
	if privNonceBig.Cmp(zero) == 0 {
		str := fmt.Sprintf("privNonce scalar is zero")
		return nil, nil, fmt.Errorf("%v", str)
	}
	if privNonceBig.Cmp(curve.N) >= 0 {
		str := fmt.Sprintf("privNonce scalar is out of bounds")
		return nil, nil, fmt.Errorf("%v", str)
	}
	privNonceBig.SetInt64(0)

	gpkX, gpkY, err := curve.encodedBytesToBigIntPoint(copyBytes(groupPublicKey))
	if err != nil {
		str := fmt.Sprintf("public key point could not be decoded")
		return nil, nil, fmt.Errorf("%v", str)
	}
	if !curve.IsOnCurve(gpkX, gpkY) {
		str := fmt.Sprintf("public key sum is off curve")
		return nil, nil, fmt.Errorf("%v", str)
	}

	gpnX, gpnY, err := curve.encodedBytesToBigIntPoint(copyBytes(pubNonceSum))
	if err != nil {
		str := fmt.Sprintf("public key point could not be decoded")
		return nil, nil, fmt.Errorf("%v", str)
	}
	if !curve.IsOnCurve(gpnX, gpnY) {
		str := fmt.Sprintf("public key sum is off curve")
		return nil, nil, fmt.Errorf("%v", str)
	}

	privDecoded, _, _ := PrivKeyFromScalar(priv)
	groupPubKeyDecoded, _ := ParsePubKey(groupPublicKey)
	privNonceDecoded, _, _ := PrivKeyFromScalar(privNonce)
	pubNonceSumDecoded, _ := ParsePubKey(pubNonceSum)

	return SignThreshold(privDecoded, groupPubKeyDecoded, msg,
		privNonceDecoded, pubNonceSumDecoded)
}

// schnorrCombineSigs combines a list of partial Schnorr signatures s values
// into a complete signature s for some group public key. This is achieved
// by simply adding the s values of the partial signatures as scalars.
func schnorrCombineSigs(sigss [][]byte) (*big.Int, error) {
	curve := Edwards()
	combinedSigS := new(big.Int).SetInt64(0)
	for i, sigs := range sigss {
		sigsBI := encodedBytesToBigInt(copyBytes(sigs))
		if sigsBI.Cmp(zero) == 0 {
			str := fmt.Sprintf("sig s %v is zero", i)
			return nil, fmt.Errorf("%v", str)
		}
		if sigsBI.Cmp(curve.N) >= 0 {
			str := fmt.Sprintf("sig s %v is out of bounds", i)
			return nil, fmt.Errorf("%v", str)
		}

		combinedSigS = scalarAdd(combinedSigS, sigsBI)
		combinedSigS.Mod(combinedSigS, curve.N)
	}

	if combinedSigS.Cmp(zero) == 0 {
		str := fmt.Sprintf("combined sig s %v is zero", combinedSigS)
		return nil, fmt.Errorf("%v", str)
	}

	return combinedSigS, nil
}

// schnorrCombinePartialSigs combines partial signatures.
func schnorrCombinePartialSigs(sigs []*Signature) (*Signature, error) {
	sigss := make([][]byte, len(sigs))
	for i, sig := range sigs {
		if sig == nil {
			return nil, fmt.Errorf("nil signature")
		}

		if i > 0 {
			if sigs[i-1].GetR().Cmp(sig.GetR()) != 0 {
				str := fmt.Sprintf("nonmatching r values for idx %v, %v",
					i, i-1)
				return nil, fmt.Errorf("%v", str)
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
