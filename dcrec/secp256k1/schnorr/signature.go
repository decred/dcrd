// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package schnorr

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/dcrec/secp256k1/v3"
)

// Signature is a type representing a Schnorr signature.
type Signature struct {
	r *big.Int
	s *big.Int
}

const (
	// SignatureSize is the size of an encoded Schnorr signature.
	SignatureSize = 64

	// scalarSize is the size of an encoded big endian scalar.
	scalarSize = 32
)

var (
	// bigZero is the big representation of zero.
	bigZero = new(big.Int).SetInt64(0)
)

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

// schnorrVerify is the internal function for verification of a secp256k1
// Schnorr signature.
func schnorrVerify(sig *Signature, pubkey *secp256k1.PublicKey, msg []byte) (bool, error) {
	curve := secp256k1.S256()
	if len(msg) != scalarSize {
		str := fmt.Sprintf("wrong size for message (got %v, want %v)",
			len(msg), scalarSize)
		return false, schnorrError(ErrBadInputSize, str)
	}

	if pubkey == nil {
		str := "nil pubkey"
		return false, schnorrError(ErrInputValue, str)
	}

	if !curve.IsOnCurve(pubkey.X(), pubkey.Y()) {
		str := "pubkey point is not on curve"
		return false, schnorrError(ErrPointNotOnCurve, str)
	}

	rBytes := bigIntToEncodedBytes(sig.r)
	toHash := make([]byte, 0, len(msg)+scalarSize)
	toHash = append(toHash, rBytes[:]...)
	toHash = append(toHash, msg...)
	h := blake256.Sum256(toHash)
	hBig := new(big.Int).SetBytes(h[:])

	// If the hash ends up larger than the order of the curve, abort.
	// Same thing for hash == 0 (as unlikely as that is...).
	if hBig.Cmp(curve.N) >= 0 {
		str := "hash of (R || m) too big"
		return false, schnorrError(ErrSchnorrHashValue, str)
	}
	if hBig.Cmp(bigZero) == 0 {
		str := "hash of (R || m) is zero value"
		return false, schnorrError(ErrSchnorrHashValue, str)
	}

	// We also can't have s greater than the order of the curve.
	if sig.s.Cmp(curve.N) >= 0 {
		str := "s value is too big"
		return false, schnorrError(ErrInputValue, str)
	}

	// r can't be larger than the curve prime.
	if sig.r.Cmp(curve.P) == 1 {
		str := "given R was greater than curve prime"
		return false, schnorrError(ErrBadSigRNotOnCurve, str)
	}

	// r' = hQ + sG
	sBytes := bigIntToEncodedBytes(sig.s)
	lx, ly := curve.ScalarMult(pubkey.X(), pubkey.Y(), h[:])
	rx, ry := curve.ScalarBaseMult(sBytes[:])
	rlx, rly := curve.Add(lx, ly, rx, ry)

	if rly.Bit(0) == 1 {
		str := "calculated R y-value is odd"
		return false, schnorrError(ErrBadSigRYValue, str)
	}
	if !curve.IsOnCurve(rlx, rly) {
		str := "calculated R point is not on curve"
		return false, schnorrError(ErrBadSigRNotOnCurve, str)
	}
	rlxB := bigIntToEncodedBytes(rlx)

	// r == r' --> valid signature
	if !bytes.Equal(rBytes[:], rlxB[:]) {
		str := "calculated R point was not given R"
		return false, schnorrError(ErrUnequalRValues, str)
	}

	return true, nil
}

// Verify is the generalized and exported function for the verification of a
// secp256k1 Schnorr signature. BLAKE256 is used as the hashing function.
func (sig *Signature) Verify(msg []byte, pubkey *secp256k1.PublicKey) bool {
	ok, _ := schnorrVerify(sig, pubkey, msg)
	return ok
}

// zeroArray zeroes the memory of a scalar array.
func zeroArray(a *[scalarSize]byte) {
	for i := 0; i < scalarSize; i++ {
		a[i] = 0x00
	}
}

// zeroSlice zeroes the memory of a scalar byte slice.
func zeroSlice(s []byte) {
	for i := 0; i < scalarSize; i++ {
		s[i] = 0x00
	}
}

// zeroBigInt zeroes the underlying memory used by the passed big integer.  The
// big integer must not be used after calling this as it changes the internal
// state out from under it which can lead to unpredictable results.
func zeroBigInt(v *big.Int) {
	words := v.Bits()
	for i := 0; i < len(words); i++ {
		words[i] = 0
	}
	v.SetInt64(0)
}

// schnorrSign signs a Schnorr signature using a specified hash function
// and the given nonce, private key, message, and optional public nonce.
// CAVEAT: Lots of variable time algorithms using both the private key and
// k, which can expose the signer to constant time attacks. You have been
// warned! DO NOT use this algorithm where you might have the possibility
// of someone having EM field/cache/etc access.
// Memory management is also kind of sloppy and whether or not your keys
// or nonces can be found in memory later is likely a product of when the
// garbage collector runs.
// TODO Use field elements with constant time algorithms to prevent said
// attacks.
func schnorrSign(msg []byte, ps []byte, k []byte) (*Signature, error) {
	curve := secp256k1.S256()
	if len(msg) != scalarSize {
		str := fmt.Sprintf("wrong size for message (got %v, want %v)",
			len(msg), scalarSize)
		return nil, schnorrError(ErrBadInputSize, str)
	}
	if len(ps) != scalarSize {
		str := fmt.Sprintf("wrong size for privkey (got %v, want %v)",
			len(ps), scalarSize)
		return nil, schnorrError(ErrBadInputSize, str)
	}
	if len(k) != scalarSize {
		str := fmt.Sprintf("wrong size for nonce k (got %v, want %v)",
			len(k), scalarSize)
		return nil, schnorrError(ErrBadInputSize, str)
	}

	psBig := new(big.Int).SetBytes(ps)
	bigK := new(big.Int).SetBytes(k)

	if psBig.Cmp(bigZero) == 0 {
		str := "secret scalar is zero"
		return nil, schnorrError(ErrInputValue, str)
	}
	if psBig.Cmp(curve.N) >= 0 {
		str := "secret scalar is out of bounds"
		return nil, schnorrError(ErrInputValue, str)
	}
	if bigK.Cmp(bigZero) == 0 {
		str := "k scalar is zero"
		return nil, schnorrError(ErrInputValue, str)
	}
	if bigK.Cmp(curve.N) >= 0 {
		str := "k scalar is out of bounds"
		return nil, schnorrError(ErrInputValue, str)
	}

	// R = kG
	var Rpx, Rpy *big.Int
	Rpx, Rpy = curve.ScalarBaseMult(k)

	// Check if the field element that would be represented by Y is odd.
	// If it is, just keep k in the group order.
	if Rpy.Bit(0) == 1 {
		bigK.Mod(bigK, curve.N)
		bigK.Sub(curve.N, bigK)
	}

	// h = Hash(r || m)
	Rpxb := bigIntToEncodedBytes(Rpx)
	hashInput := make([]byte, 0, scalarSize*2)
	hashInput = append(hashInput, Rpxb[:]...)
	hashInput = append(hashInput, msg...)
	h := blake256.Sum256(hashInput)
	hBig := new(big.Int).SetBytes(h[:])

	// If the hash ends up larger than the order of the curve, abort.
	if hBig.Cmp(curve.N) >= 0 {
		str := "hash of (R || m) too big"
		return nil, schnorrError(ErrSchnorrHashValue, str)
	}

	// s = k - hx
	// TODO Speed this up a bunch by using field elements, not
	// big ints. That we multiply the private scalar using big
	// ints is also probably bad because we can only assume the
	// math isn't in constant time, thus opening us up to side
	// channel attacks. Using a constant time field element
	// implementation will fix this.
	sBig := new(big.Int)
	sBig.Mul(hBig, psBig)
	sBig.Sub(bigK, sBig)
	sBig.Mod(sBig, curve.N)

	if sBig.Cmp(bigZero) == 0 {
		str := fmt.Sprintf("sig s %v is zero", sBig)
		return nil, schnorrError(ErrZeroSigS, str)
	}

	// Zero out the private key and nonce when we're done with it.
	zeroBigInt(bigK)
	zeroSlice(k)
	zeroBigInt(psBig)
	zeroSlice(ps)

	return &Signature{Rpx, sBig}, nil
}

// nonceRFC6979 is a local instantiation of deterministic nonce generation
// by the standards of RFC6979.
func nonceRFC6979(privKey []byte, hash []byte, extra []byte, version []byte, extraIterations uint32) []byte {
	k := secp256k1.NonceRFC6979(privKey, hash, extra, version, extraIterations)
	kBytes := k.Bytes()
	defer zeroArray(&kBytes)
	bigK := new(big.Int).SetBytes(kBytes[:])
	defer zeroBigInt(bigK)
	nonce := bigIntToEncodedBytes(bigK)
	return nonce[:]
}

// Sign is the exported version of sign. It uses RFC6979 and Blake256 to
// produce a Schnorr signature.
func Sign(priv *secp256k1.PrivateKey, hash []byte) (r, s *big.Int, err error) {
	// Convert the private scalar to a 32 byte big endian number.
	bigPriv := new(big.Int).SetBytes(priv.Serialize())
	pA := bigIntToEncodedBytes(bigPriv)
	defer zeroArray(pA)

	for iteration := uint32(0); ; iteration++ {
		// Generate a 32-byte scalar to use as a nonce via RFC6979.
		kB := nonceRFC6979(priv.Serialize(), hash, nil, nil, iteration)
		sig, err := schnorrSign(hash, pA[:], kB)
		if err == nil {
			r = sig.r
			s = sig.s
			break
		}

		errTyped, ok := err.(Error)
		if !ok {
			return nil, nil, fmt.Errorf("unknown error type")
		}
		if errTyped.ErrorCode != ErrSchnorrHashValue {
			return nil, nil, err
		}
	}

	return r, s, nil
}
