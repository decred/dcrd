// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"math/big"
)

// References:
//   [GECC]: Guide to Elliptic Curve Cryptography (Hankerson, Menezes, Vanstone)
//
//   [ISO/IEC 8825-1]: Information technology — ASN.1 encoding rules:
//     Specification of Basic Encoding Rules (BER), Canonical Encoding Rules
//     (CER) and Distinguished Encoding Rules (DER)

// Errors returned by canonicalPadding.
var (
	errNegativeValue          = errors.New("value may be interpreted as negative")
	errExcessivelyPaddedValue = errors.New("value is excessively padded")
)

// Signature is a type representing an ECDSA signature.
type Signature struct {
	r *big.Int
	s *big.Int
}

var (
	// singleZero is used during RFC6979 nonce generation.  It is provided
	// here to avoid the need to create it multiple times.
	singleZero = []byte{0x00}

	// zeroInitializer is used during RFC6979 nonce generation.  It is provided
	// here to avoid the need to create it multiple times.
	zeroInitializer = bytes.Repeat([]byte{0x00}, sha256.BlockSize)

	// singleOne is used during RFC6979 nonce generation.  It is provided
	// here to avoid the need to create it multiple times.
	singleOne = []byte{0x01}

	// oneInitializer is used during RFC6979 nonce generation.  It is provided
	// here to avoid the need to create it multiple times.
	oneInitializer = bytes.Repeat([]byte{0x01}, sha256.Size)
)

// NewSignature instantiates a new signature given some R,S values.
func NewSignature(r, s *big.Int) *Signature {
	return &Signature{r, s}
}

// Serialize returns the ECDSA signature in the Distinguished Encoding Rules
// (DER) format per section 10 of [ISO/IEC 8825-1] and such that the S component
// of the signature is less than or equal to the half order of the group.
//
// Note that the serialized bytes returned do not include the appended hash type
// used in Decred signature scripts.
func (sig *Signature) Serialize() []byte {
	// The format of a DER encoded signature is as follows:
	//
	// 0x30 <total length> 0x02 <length of R> <R> 0x02 <length of S> <S>
	//   - 0x30 is the ASN.1 identifier for a sequence
	//   - Total length is 1 byte and specifies length of all remaining data
	//   - 0x02 is the ASN.1 identifier that specifies an integer follows
	//   - Length of R is 1 byte and specifies how many bytes R occupies
	//   - R is the arbitrary length big-endian encoded number which
	//     represents the R value of the signature.  DER encoding dictates
	//     that the value must be encoded using the minimum possible number
	//     of bytes.  This implies the first byte can only be null if the
	//     highest bit of the next byte is set in order to prevent it from
	//     being interpreted as a negative number.
	//   - 0x02 is once again the ASN.1 integer identifier
	//   - Length of S is 1 byte and specifies how many bytes S occupies
	//   - S is the arbitrary length big-endian encoded number which
	//     represents the S value of the signature.  The encoding rules are
	//     identical as those for R.
	const (
		asn1SequenceID = 0x30
		asn1IntegerID  = 0x02
	)

	// Convert big ints to mod N scalars.  Ultimately the goal is to convert
	// the signature type itself to use mod N scalars directly which will allow
	// this step to be removed.
	var r, s ModNScalar
	r.SetByteSlice(sig.r.Bytes())
	s.SetByteSlice(sig.s.Bytes())

	// Ensure the S component of the signature is less than or equal to the half
	// order of the group because both S and its negation are valid signatures
	// modulo the order, so this forces a consistent choice to reduce signature
	// malleability.
	sigS := new(ModNScalar).Set(&s)
	if sigS.IsOverHalfOrder() {
		sigS.Negate()
	}

	// Serialize the R and S components of the signature into their fixed
	// 32-byte big-endian encoding.
	var rBytes, sBytes [32]byte
	r.PutBytes(&rBytes)
	sigS.PutBytes(&sBytes)

	// Ensure the encoded bytes for the R and S components are canonical per DER
	// by trimming all leading zero bytes so long as the next byte does not have
	// the high bit set and it's not the final byte.
	var rBuf, sBuf [33]byte
	copy(rBuf[1:], rBytes[:])
	copy(sBuf[1:], sBytes[:])
	canonR, canonS := rBuf[:], sBuf[:]
	for len(canonR) > 1 && canonR[0] == 0x00 && canonR[1]&0x80 == 0 {
		canonR = canonR[1:]
	}
	for len(canonS) > 1 && canonS[0] == 0x00 && canonS[1]&0x80 == 0 {
		canonS = canonS[1:]
	}

	// Total length of returned signature is 1 byte for each magic and length
	// (6 total), plus lengths of R and S.
	totalLen := 6 + len(canonR) + len(canonS)
	b := make([]byte, 0, totalLen)
	b = append(b, asn1SequenceID)
	b = append(b, byte(totalLen-2))
	b = append(b, asn1IntegerID)
	b = append(b, byte(len(canonR)))
	b = append(b, canonR...)
	b = append(b, asn1IntegerID)
	b = append(b, byte(len(canonS)))
	b = append(b, canonS...)
	return b
}

// fieldToModNScalar converts a field value to scalar modulo the group order and
// returns the scalar along with either 1 if it was reduced (aka it overflowed)
// or 0 otherwise.
//
// Note that a bool is not used here because it is not possible in Go to convert
// from a bool to numeric value in constant time and many constant-time
// operations require a numeric value.
func fieldToModNScalar(v *fieldVal) (ModNScalar, uint32) {
	var buf [32]byte
	v.PutBytes(&buf)
	var s ModNScalar
	overflow := s.SetBytes(&buf)
	zeroArray32(&buf)
	return s, overflow
}

// Verify returns whether or not the signature is valid for the provided hash
// and secp256k1 public key.
func (sig *Signature) Verify(hash []byte, pubKey *PublicKey) bool {
	// The algorithm for verifying an ECDSA signature is given as algorithm 4.30
	// in [GECC].
	//
	// The following is a paraphrased version for reference:
	//
	// G = curve generator
	// N = curve order
	// Q = public key
	// m = message
	// R, S = signature
	//
	// 1. Fail if R and S are not in [1, N-1]
	// 2. e = H(m)
	// 3. w = S^-1 mod N
	// 4. u1 = e * w mod N
	//    u2 = R * w mod N
	// 5. X = u1G + u2Q
	// 6. Fail if X is the point at infinity
	// 7. x = X.x mod N (X.x is the x coordinate of X)
	// 8. Verified if x == R

	// Step 1.
	//
	// Fail if R and S are not in [1, N-1].
	var R, S ModNScalar
	if overflow := R.SetByteSlice(sig.r.Bytes()); overflow || R.IsZero() {
		return false
	}
	if overflow := S.SetByteSlice(sig.s.Bytes()); overflow || S.IsZero() {
		return false
	}

	// Step 2.
	//
	// e = H(m)
	var e ModNScalar
	e.SetByteSlice(hash)

	// Step 3.
	//
	// w = S^-1 mod N
	w := new(ModNScalar).InverseValNonConst(&S)

	// Step 4.
	//
	// u1 = e * w mod N
	// u2 = R * w mod N
	u1 := new(ModNScalar).Mul2(&e, w)
	u2 := new(ModNScalar).Mul2(&R, w)

	// Step 5.
	//
	// X = u1G + u2Q
	var X, Q, u1G, u2Q jacobianPoint
	bigAffineToJacobian(pubKey.X, pubKey.Y, &Q)
	scalarBaseMultJacobian(u1, &u1G)
	scalarMultJacobian(u2, &Q, &u2Q)
	addJacobian(&u1G, &u2Q, &X)

	// Step 6.
	//
	// Fail if X is the point at infinity
	if (X.x.IsZero() && X.y.IsZero()) || X.z.IsZero() {
		return false
	}

	// Step 7.
	//
	// x = X.x mod N (X.x is the x coordinate of X)
	//
	// Note that the point must be in affine coordinates since R is in affine
	// coordinates.
	X.ToAffine()
	x, _ := fieldToModNScalar(&X.x)

	// Step 8.
	//
	// Verified if x == R
	return x.Equals(&R)
}

// IsEqual compares this Signature instance to the one passed, returning true if
// both Signatures are equivalent.  A signature is equivalent to another, if
// they both have the same scalar value for R and S.
func (sig *Signature) IsEqual(otherSig *Signature) bool {
	return sig.r.Cmp(otherSig.r) == 0 && sig.s.Cmp(otherSig.s) == 0
}

// parseSig attempts to parse the provided raw signature bytes into a Signature
// struct.  The der flag specifies whether or not to enforce the more strict
// Distinguished Encoding Rules (DER) of the ASN.1 spec versus the more lax
// Basic Encoding Rules (BER).
func parseSig(sigStr []byte, der bool) (*Signature, error) {
	// Originally this code used encoding/asn1 in order to parse the
	// signature, but a number of problems were found with this approach.
	// Despite the fact that signatures are stored as DER, the difference
	// between go's idea of a bignum (and that they have sign) doesn't agree
	// with the openssl one (where they do not). The above is true as of
	// Go 1.1. In the end it was simpler to rewrite the code to explicitly
	// understand the format which is this:
	// 0x30 <length of whole message> <0x02> <length of R> <R> 0x2
	// <length of S> <S>.

	signature := &Signature{}
	curve := S256()

	// minimal message is when both numbers are 1 bytes. adding up to:
	// 0x30 + len + 0x02 + 0x01 + <byte> + 0x2 + 0x01 + <byte>
	if len(sigStr) < 8 {
		return nil, errors.New("malformed signature: too short")
	}
	// 0x30
	index := 0
	if sigStr[index] != 0x30 {
		return nil, errors.New("malformed signature: no header magic")
	}
	index++
	// length of remaining message
	siglen := sigStr[index]
	index++
	if int(siglen+2) > len(sigStr) {
		return nil, errors.New("malformed signature: bad length")
	}
	// trim the slice we're working on so we only look at what matters.
	sigStr = sigStr[:siglen+2]

	// 0x02
	if sigStr[index] != 0x02 {
		return nil,
			errors.New("malformed signature: no 1st int marker")
	}
	index++

	// Length of signature R.
	rLen := int(sigStr[index])
	// must be positive, must be able to fit in another 0x2, <len> <s>
	// hence the -3. We assume that the length must be at least one byte.
	index++
	if rLen <= 0 || rLen > len(sigStr)-index-3 {
		return nil, errors.New("malformed signature: bogus R length")
	}

	// Then R itself.
	rBytes := sigStr[index : index+rLen]
	if der {
		switch err := canonicalPadding(rBytes); err {
		case errNegativeValue:
			return nil, errors.New("signature R is negative")
		case errExcessivelyPaddedValue:
			return nil, errors.New("signature R is excessively padded")
		}
	}
	signature.r = new(big.Int).SetBytes(rBytes)
	index += rLen
	// 0x02. length already checked in previous if.
	if sigStr[index] != 0x02 {
		return nil, errors.New("malformed signature: no 2nd int marker")
	}
	index++

	// Length of signature S.
	sLen := int(sigStr[index])
	index++
	// S should be the rest of the string.
	if sLen <= 0 || sLen > len(sigStr)-index {
		return nil, errors.New("malformed signature: bogus S length")
	}

	// Then S itself.
	sBytes := sigStr[index : index+sLen]
	if der {
		switch err := canonicalPadding(sBytes); err {
		case errNegativeValue:
			return nil, errors.New("signature S is negative")
		case errExcessivelyPaddedValue:
			return nil, errors.New("signature S is excessively padded")
		}
	}
	signature.s = new(big.Int).SetBytes(sBytes)
	index += sLen

	// sanity check length parsing
	if index != len(sigStr) {
		return nil, fmt.Errorf("malformed signature: bad final length %v != %v",
			index, len(sigStr))
	}

	// Verify also checks this, but we can be more sure that we parsed
	// correctly if we verify here too.
	// FWIW the ecdsa spec states that R and S must be | 1, N - 1 |
	// but crypto/ecdsa only checks for Sign != 0. Mirror that.
	if signature.r.Sign() != 1 {
		return nil, errors.New("signature R isn't 1 or more")
	}
	if signature.s.Sign() != 1 {
		return nil, errors.New("signature S isn't 1 or more")
	}
	if signature.r.Cmp(curve.Params().N) >= 0 {
		return nil, errors.New("signature R is >= curve.N")
	}
	if signature.s.Cmp(curve.Params().N) >= 0 {
		return nil, errors.New("signature S is >= curve.N")
	}

	return signature, nil
}

// ParseSignature parses a signature in the Basic Encoding Rules (BER) format
// into a Signature type, performing some basic sanity checks.  If parsing
// according to the more strict DER format is needed, use ParseDERSignature.
func ParseSignature(sigStr []byte) (*Signature, error) {
	return parseSig(sigStr, false)
}

// ParseDERSignature parses a signature in the Distinguished Encoding Rules
// (DER) format of the ASN.1 spec into a Signature type.  If parsing according
// to the less strict BER format is needed, use ParseSignature.
func ParseDERSignature(sigStr []byte) (*Signature, error) {
	return parseSig(sigStr, true)
}

// canonicalPadding checks whether a big-endian encoded integer could
// possibly be misinterpreted as a negative number (even though OpenSSL
// treats all numbers as unsigned), or if there is any unnecessary
// leading zero padding.
func canonicalPadding(b []byte) error {
	switch {
	case b[0]&0x80 == 0x80:
		return errNegativeValue
	case len(b) > 1 && b[0] == 0x00 && b[1]&0x80 != 0x80:
		return errExcessivelyPaddedValue
	default:
		return nil
	}
}

// hashToInt converts a hash value to an integer. There is some disagreement
// about how this is done. [NSA] suggests that this is done in the obvious
// manner, but [SECG] truncates the hash to the bit-length of the curve order
// first. We follow [SECG] because that's what OpenSSL does. Additionally,
// OpenSSL right shifts excess bits from the number if the hash is too large
// and we mirror that too.
// This is borrowed from crypto/ecdsa.
func hashToInt(hash []byte) *big.Int {
	orderBits := S256().Params().N.BitLen()
	orderBytes := (orderBits + 7) / 8
	if len(hash) > orderBytes {
		hash = hash[:orderBytes]
	}

	ret := new(big.Int).SetBytes(hash)
	excess := len(hash)*8 - orderBits
	if excess > 0 {
		ret.Rsh(ret, uint(excess))
	}
	return ret
}

// recoverKeyFromSignature recovers a public key from the signature "sig" on the
// given message hash "msg". Based on the algorithm found in section 5.1.5 of
// SEC 1 Ver 2.0, page 47-48 (53 and 54 in the pdf). This performs the details
// in the inner loop in Step 1. The counter provided is actually the j parameter
// of the loop * 2 - on the first iteration of j we do the R case, else the -R
// case in step 1.6. This counter is used in the Decred compressed signature
// format and thus we match bitcoind's behaviour here.
func recoverKeyFromSignature(sig *Signature, msg []byte, iter int, doChecks bool) (*PublicKey, error) {
	// 1.1 x = (n * i) + r
	curve := S256()
	Rx := new(big.Int).Mul(curve.Params().N,
		new(big.Int).SetInt64(int64(iter/2)))
	Rx.Add(Rx, sig.r)
	if Rx.Cmp(curve.Params().P) != -1 {
		return nil, errors.New("calculated Rx is larger than curve P")
	}

	// convert 02<Rx> to point R. (step 1.2 and 1.3). If we are on an odd
	// iteration then 1.6 will be done with -R, so we calculate the other
	// term when uncompressing the point.
	Ry, err := decompressPoint(Rx, iter%2 == 1)
	if err != nil {
		return nil, err
	}

	// 1.4 Check n*R is point at infinity
	if doChecks {
		nRx, nRy := curve.ScalarMult(Rx, Ry, curve.Params().N.Bytes())
		if nRx.Sign() != 0 || nRy.Sign() != 0 {
			return nil, errors.New("n*R does not equal the point at infinity")
		}
	}

	// 1.5 calculate e from message using the same algorithm as ecdsa
	// signature calculation.
	e := hashToInt(msg)

	// Step 1.6.1:
	// We calculate the two terms sR and eG separately multiplied by the
	// inverse of r (from the signature). We then add them to calculate
	// Q = r^-1(sR-eG)
	invr := new(big.Int).ModInverse(sig.r, curve.Params().N)

	// first term.
	invrS := new(big.Int).Mul(invr, sig.s)
	invrS.Mod(invrS, curve.Params().N)
	var invrSModN ModNScalar
	invrSModN.SetByteSlice(invrS.Bytes())
	var fR, sR jacobianPoint
	bigAffineToJacobian(Rx, Ry, &fR)
	scalarMultJacobian(&invrSModN, &fR, &sR)

	// second term.
	e.Neg(e)
	e.Mod(e, curve.Params().N)
	e.Mul(e, invr)
	e.Mod(e, curve.Params().N)
	var eModN ModNScalar
	eModN.SetByteSlice(e.Bytes())

	var minusEG, q jacobianPoint
	scalarBaseMultJacobian(&eModN, &minusEG)
	addJacobian(&sR, &minusEG, &q)

	Qx, Qy := jacobianToBigAffine(&q)
	return NewPublicKey(Qx, Qy), nil
}

// SignCompact produces a compact signature of the data in hash with the given
// private key on the secp256k1 curve. The isCompressed parameter should be used
// to detail if the given signature should reference a compressed public key or
// not. If successful the bytes of the compact signature will be returned in the
// format:
// <(byte of 27+public key solution)+4 if compressed >< padded bytes for signature R><padded bytes for signature S>
// where the R and S parameters are padded up to the bitlength of the curve.
func SignCompact(key *PrivateKey, hash []byte, isCompressedKey bool) ([]byte, error) {
	sig := key.Sign(hash)
	signingPubKey := key.PubKey()

	for i := 0; i < (curveParams.H+1)*2; i++ {
		recoveredPubKey, err := recoverKeyFromSignature(sig, hash, i, true)
		if err != nil || !recoveredPubKey.IsEqual(signingPubKey) {
			continue
		}

		result := make([]byte, 1, 2*curveParams.byteSize+1)
		result[0] = 27 + byte(i)
		if isCompressedKey {
			result[0] += 4
		}
		// Not sure this needs rounding but safer to do so.
		curvelen := (curveParams.BitSize + 7) / 8

		// Pad R and S to curvelen if needed.
		bytelen := (sig.r.BitLen() + 7) / 8
		if bytelen < curvelen {
			result = append(result,
				make([]byte, curvelen-bytelen)...)
		}
		result = append(result, sig.r.Bytes()...)

		bytelen = (sig.s.BitLen() + 7) / 8
		if bytelen < curvelen {
			result = append(result,
				make([]byte, curvelen-bytelen)...)
		}
		result = append(result, sig.s.Bytes()...)

		return result, nil
	}

	return nil, errors.New("no valid solution for pubkey found")
}

// RecoverCompact attempts to recover the secp256k1 public key from the provided
// signature and message hash.  It first verifies the signature, and, if the
// signature matches then the recovered public key will be returned as well as a
// boolean indicating whether or not the original key was compressed.
func RecoverCompact(signature, hash []byte) (*PublicKey, bool, error) {
	bitlen := (S256().BitSize + 7) / 8
	if len(signature) != 1+bitlen*2 {
		return nil, false, errors.New("invalid compact signature size")
	}

	iteration := int((signature[0] - 27) & ^byte(4))

	// format is <header byte><bitlen R><bitlen S>
	sig := &Signature{
		r: new(big.Int).SetBytes(signature[1 : bitlen+1]),
		s: new(big.Int).SetBytes(signature[bitlen+1:]),
	}
	// The iteration used here was encoded
	key, err := recoverKeyFromSignature(sig, hash, iteration, false)
	if err != nil {
		return nil, false, err
	}

	return key, ((signature[0] - 27) & 4) == 4, nil
}

// signRFC6979 generates a deterministic ECDSA signature according to RFC 6979
// and BIP 62.
func signRFC6979(privateKey *PrivateKey, hash []byte) *Signature {
	// The algorithm for producing an ECDSA signature is given as algorithm 4.29
	// in [GECC].
	//
	// The following is a paraphrased version for reference:
	//
	// G = curve generator
	// N = curve order
	// d = private key
	// m = message
	// r, s = signature
	//
	// 1. Select random nonce k in [1, N-1]
	// 2. Compute kG
	// 3. r = kG.x mod N (kG.x is the x coordinate of the point kG)
	//    Repeat from step 1 if r = 0
	// 4. e = H(m)
	// 5. s = k^-1(e + dr) mod N
	//    Repeat from step 1 if s = 0
	// 6. Return (r,s)
	//
	// This is slightly modified here to conform to RFC6979 and BIP 62 as
	// follows:
	//
	// A. Instead of selecting a random nonce in step 1, use RFC6979 to generate
	//    a deterministic nonce in [1, N-1] parameterized by the private key,
	//    message being signed, and an iteration count for the repeat cases
	// B. Negate s calculated in step 5 if it is > N/2
	//    This is done because both s and its negation are valid signatures
	//    modulo the curve order N, so it forces a consistent choice to reduce
	//    signature malleability

	privKeyBytes := privateKey.key.Bytes()
	defer zeroArray32(&privKeyBytes)
	for iteration := uint32(0); ; iteration++ {
		// Step 1 with modification A.
		//
		// Generate a deterministic nonce in [1, N-1] parameterized by the
		// private key, message being signed, and iteration count.
		k := NonceRFC6979(privKeyBytes[:], hash, nil, nil, iteration)

		// Step 2.
		//
		// Compute kG
		//
		// Note that the algorithm expects the point in affine coordinates.
		var kG jacobianPoint
		scalarBaseMultJacobian(k, &kG)
		kG.ToAffine()

		// Step 3.
		//
		// r = kG.x mod N
		// Repeat from step 1 if r = 0
		r, _ := fieldToModNScalar(&kG.x)
		if r.IsZero() {
			continue
		}

		// Step 4.
		//
		// e = H(m)
		//
		// Note that this actually sets e = H(m) mod N which is correct since
		// it is only used in step 5 which itself is mod N.
		var e ModNScalar
		e.SetByteSlice(hash)

		// Step 5 with modification B.
		//
		// s = k^-1(e + dr) mod N
		// Repeat from step 1 if s = 0
		// s = -s if s > N/2
		kInv := new(ModNScalar).InverseValNonConst(k)
		s := new(ModNScalar).Mul2(&privateKey.key, &r).Add(&e).Mul(kInv)
		if s.IsZero() {
			continue
		}
		if s.IsOverHalfOrder() {
			s.Negate()
		}

		// Step 6.
		//
		// Return (r,s)
		rBytes, sBytes := r.Bytes(), s.Bytes()
		bigR := new(big.Int).SetBytes(rBytes[:])
		bigS := new(big.Int).SetBytes(sBytes[:])
		return &Signature{r: bigR, s: bigS}
	}
}

// hmacsha256 implements a resettable version of HMAC-SHA256.
type hmacsha256 struct {
	inner, outer hash.Hash
	ipad, opad   [sha256.BlockSize]byte
}

// Write adds data to the running hash.
func (h *hmacsha256) Write(p []byte) {
	h.inner.Write(p)
}

// initKey initializes the HMAC-SHA256 instance to the provided key.
func (h *hmacsha256) initKey(key []byte) {
	// Hash the key if it is too large.
	if len(key) > sha256.BlockSize {
		h.outer.Write(key)
		key = h.outer.Sum(nil)
	}
	copy(h.ipad[:], key)
	copy(h.opad[:], key)
	for i := range h.ipad {
		h.ipad[i] ^= 0x36
	}
	for i := range h.opad {
		h.opad[i] ^= 0x5c
	}
	h.inner.Write(h.ipad[:])
}

// ResetKey resets the HMAC-SHA256 to its initial state and then initializes it
// with the provided key.  It is equivalent to creating a new instance with the
// provided key without allocating more memory.
func (h *hmacsha256) ResetKey(key []byte) {
	h.inner.Reset()
	h.outer.Reset()
	copy(h.ipad[:], zeroInitializer)
	copy(h.opad[:], zeroInitializer)
	h.initKey(key)
}

// Resets the HMAC-SHA256 to its initial state using the current key.
func (h *hmacsha256) Reset() {
	h.inner.Reset()
	h.inner.Write(h.ipad[:])
}

// Sum returns the hash of the written data.
func (h *hmacsha256) Sum() []byte {
	h.outer.Reset()
	h.outer.Write(h.opad[:])
	h.outer.Write(h.inner.Sum(nil))
	return h.outer.Sum(nil)
}

// newHMACSHA256 returns a new HMAC-SHA256 hasher using the provided key.
func newHMACSHA256(key []byte) *hmacsha256 {
	h := new(hmacsha256)
	h.inner = sha256.New()
	h.outer = sha256.New()
	h.initKey(key)
	return h
}

// NonceRFC6979 generates a nonce deterministically according to RFC 6979 using
// HMAC-SHA256 for the hashing function.  It takes a 32-byte hash as an input
// and returns a 32-byte nonce to be used for deterministic signing.  The extra
// and version arguments are optional, but allow additional data to be added to
// the input of the HMAC.  When provided, the extra data must be 32-bytes and
// version must be 16 bytes or they will be ignored.
//
// Finally, the extraIterations parameter provides a method to produce a stream
// of deterministic nonces to ensure the signing code is able to produce a nonce
// that results in a valid signature in the extremely unlikely event the
// original nonce produced results in an invalid signature (e.g. R == 0).
// Signing code should start with 0 and increment it if necessary.
func NonceRFC6979(privKey []byte, hash []byte, extra []byte, version []byte, extraIterations uint32) *ModNScalar {
	// Input to HMAC is the 32-byte private key and the 32-byte hash.  In
	// addition, it may include the optional 32-byte extra data and 16-byte
	// version.  Create a fixed-size array to avoid extra allocs and slice it
	// properly.
	const (
		privKeyLen = 32
		hashLen    = 32
		extraLen   = 32
		versionLen = 16
	)
	var keyBuf [privKeyLen + hashLen + extraLen + versionLen]byte

	// Truncate rightmost bytes of private key and hash if they are too long and
	// leave left padding of zeros when they're too short.
	if len(privKey) > privKeyLen {
		privKey = privKey[:privKeyLen]
	}
	if len(hash) > hashLen {
		hash = hash[:hashLen]
	}
	offset := privKeyLen - len(privKey) // Zero left padding if needed.
	offset += copy(keyBuf[offset:], privKey)
	offset += hashLen - len(hash) // Zero left padding if needed.
	offset += copy(keyBuf[offset:], hash)
	if len(extra) == extraLen {
		offset += copy(keyBuf[offset:], extra)
		if len(version) == versionLen {
			offset += copy(keyBuf[offset:], version)
		}
	} else if len(version) == versionLen {
		// When the version was specified, but not the extra data, leave the
		// extra data portion all zero.
		offset += privKeyLen
		offset += copy(keyBuf[offset:], version)
	}
	key := keyBuf[:offset]

	// Step B.
	//
	// V = 0x01 0x01 0x01 ... 0x01 such that the length of V, in bits, is
	// equal to 8*ceil(hashLen/8).
	//
	// Note that since the hash length is a multiple of 8 for the chosen hash
	// function in this optimized implementation, the result is just the hash
	// length, so avoid the extra calculations.  Also, since it isn't modified,
	// start with a global value.
	v := oneInitializer

	// Step C (Go zeroes all allocated memory).
	//
	// K = 0x00 0x00 0x00 ... 0x00 such that the length of K, in bits, is
	// equal to 8*ceil(hashLen/8).
	//
	// As above, since the hash length is a multiple of 8 for the chosen hash
	// function in this optimized implementation, the result is just the hash
	// length, so avoid the extra calculations.
	k := zeroInitializer[:hashLen]

	// Step D.
	//
	// K = HMAC_K(V || 0x00 || int2octets(x) || bits2octets(h1))
	//
	// Note that key is the "int2octets(x) || bits2octets(h1)" portion along
	// with potential additional data as described by section 3.6 of the RFC.
	hasher := newHMACSHA256(k)
	hasher.Write(oneInitializer)
	hasher.Write(singleZero[:])
	hasher.Write(key)
	k = hasher.Sum()

	// Step E.
	//
	// V = HMAC_K(V)
	hasher.ResetKey(k)
	hasher.Write(v)
	v = hasher.Sum()

	// Step F.
	//
	// K = HMAC_K(V || 0x01 || int2octets(x) || bits2octets(h1))
	//
	// Note that key is the "int2octets(x) || bits2octets(h1)" portion along
	// with potential additional data as described by section 3.6 of the RFC.
	hasher.Reset()
	hasher.Write(v)
	hasher.Write(singleOne[:])
	hasher.Write(key[:])
	k = hasher.Sum()

	// Step G.
	//
	// V = HMAC_K(V)
	hasher.ResetKey(k)
	hasher.Write(v)
	v = hasher.Sum()

	// Step H.
	//
	// Repeat until the value is nonzero and less than the curve order.
	var generated uint32
	for {
		// Step H1 and H2.
		//
		// Set T to the empty sequence.  The length of T (in bits) is denoted
		// tlen; thus, at that point, tlen = 0.
		//
		// While tlen < qlen, do the following:
		//   V = HMAC_K(V)
		//   T = T || V
		//
		// Note that because the hash function output is the same length as the
		// private key in this optimized implementation, there is no need to
		// loop or create an intermediate T.
		hasher.Reset()
		hasher.Write(v)
		v = hasher.Sum()

		// Step H3.
		//
		// k = bits2int(T)
		// If k is within the range [1,q-1], return it.
		//
		// Otherwise, compute:
		// K = HMAC_K(V || 0x00)
		// V = HMAC_K(V)
		var secret ModNScalar
		overflow := secret.SetByteSlice(v)
		if !overflow && !secret.IsZero() {
			generated++
			if generated > extraIterations {
				return &secret
			}
		}

		// K = HMAC_K(V || 0x00)
		hasher.Reset()
		hasher.Write(v)
		hasher.Write(singleZero[:])
		k = hasher.Sum()

		// V = HMAC_K(V)
		hasher.ResetKey(k)
		hasher.Write(v)
		v = hasher.Sum()
	}
}
