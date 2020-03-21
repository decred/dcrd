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
)

// References:
//   [GECC]: Guide to Elliptic Curve Cryptography (Hankerson, Menezes, Vanstone)
//
//   [ISO/IEC 8825-1]: Information technology — ASN.1 encoding rules:
//     Specification of Basic Encoding Rules (BER), Canonical Encoding Rules
//     (CER) and Distinguished Encoding Rules (DER)
//
//   [SEC1]: Elliptic Curve Cryptography (May 31, 2009, Version 2.0)
//     https://www.secg.org/sec1-v2.pdf

// Signature is a type representing an ECDSA signature.
type Signature struct {
	r ModNScalar
	s ModNScalar
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

	// orderAsFieldVal is the order of the secp256k1 curve group stored as a
	// field value.  It is provided here to avoid the need to create it multiple
	// times.
	orderAsFieldVal = new(FieldVal).SetByteSlice(curveParams.N.Bytes())
)

const (
	// asn1SequenceID is the ASN.1 identifier for a sequence and is used when
	// parsing and serializing signatures encoded with the Distinguished
	// Encoding Rules (DER) format per section 10 of [ISO/IEC 8825-1].
	asn1SequenceID = 0x30

	// asn1IntegerID is the ASN.1 identifier for an integer and is used when
	// parsing and serializing signatures encoded with the Distinguished
	// Encoding Rules (DER) format per section 10 of [ISO/IEC 8825-1].
	asn1IntegerID = 0x02
)

// NewSignature instantiates a new signature given some R,S values.
func NewSignature(r, s *ModNScalar) *Signature {
	return &Signature{*r, *s}
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
	//   - 0x30 is the ASN.1 identifier for a sequence.
	//   - Total length is 1 byte and specifies length of all remaining data.
	//   - 0x02 is the ASN.1 identifier that specifies an integer follows.
	//   - Length of R is 1 byte and specifies how many bytes R occupies.
	//   - R is the arbitrary length big-endian encoded number which
	//     represents the R value of the signature.  DER encoding dictates
	//     that the value must be encoded using the minimum possible number
	//     of bytes.  This implies the first byte can only be null if the
	//     highest bit of the next byte is set in order to prevent it from
	//     being interpreted as a negative number.
	//   - 0x02 is once again the ASN.1 integer identifier.
	//   - Length of S is 1 byte and specifies how many bytes S occupies.
	//   - S is the arbitrary length big-endian encoded number which
	//     represents the S value of the signature.  The encoding rules are
	//     identical as those for R.

	// Ensure the S component of the signature is less than or equal to the half
	// order of the group because both S and its negation are valid signatures
	// modulo the order, so this forces a consistent choice to reduce signature
	// malleability.
	sigS := new(ModNScalar).Set(&sig.s)
	if sigS.IsOverHalfOrder() {
		sigS.Negate()
	}

	// Serialize the R and S components of the signature into their fixed
	// 32-byte big-endian encoding.
	var rBytes, sBytes [32]byte
	sig.r.PutBytes(&rBytes)
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
func fieldToModNScalar(v *FieldVal) (ModNScalar, uint32) {
	var buf [32]byte
	v.PutBytes(&buf)
	var s ModNScalar
	overflow := s.SetBytes(&buf)
	zeroArray32(&buf)
	return s, overflow
}

// modNScalarToField converts a scalar modulo the group order to a field value.
func modNScalarToField(v *ModNScalar) FieldVal {
	var buf [32]byte
	v.PutBytes(&buf)
	var fv FieldVal
	fv.SetBytes(&buf)
	return fv
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
	//
	// However, since all group operations are done internally in Jacobian
	// projective space, the algorithm is modified slightly here in order to
	// avoid an expensive inversion back into affine coordinates at step 7.
	// Credits to Greg Maxwell for originally suggesting this optimization.
	//
	// Ordinarily, step 7 involves converting the x coordinate to affine by
	// calculating x = x / z^2 (mod P) and then calculating the remainder as
	// x = x (mod N).  Then step 8 compares it to R.
	//
	// Note that since R is the x coordinate mod N from a random point that was
	// originally mod P, and the cofactor of the secp256k1 curve is 1, there are
	// only two possible x coordinates that the original random point could have
	// been to produce R: x, where x < N, and x+N, where x+N < P.
	//
	// This implies that the signature is valid if either:
	// a) R == X.x / X.z^2 (mod P)
	//    => R * X.z^2 == X.x (mod P)
	// --or--
	// b) R + N < P && R + N == X.x / X.z^2 (mod P)
	//    => R + N < P && (R + N) * X.z^2 == X.x (mod P)
	//
	// Therefore the following modified algorithm is used:
	//
	// 1. Fail if R and S are not in [1, N-1]
	// 2. e = H(m)
	// 3. w = S^-1 mod N
	// 4. u1 = e * w mod N
	//    u2 = R * w mod N
	// 5. X = u1G + u2Q
	// 6. Fail if X is the point at infinity
	// 7. z = (X.z)^2 mod P (X.z is the z coordinate of X)
	// 8. Verified if R * z == X.x (mod P)
	// 9. Fail if R + N >= P
	// 10. Verified if (R + N) * z == X.x (mod P)

	// Step 1.
	//
	// Fail if R and S are not in [1, N-1].
	if sig.r.IsZero() || sig.s.IsZero() {
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
	w := new(ModNScalar).InverseValNonConst(&sig.s)

	// Step 4.
	//
	// u1 = e * w mod N
	// u2 = R * w mod N
	u1 := new(ModNScalar).Mul2(&e, w)
	u2 := new(ModNScalar).Mul2(&sig.r, w)

	// Step 5.
	//
	// X = u1G + u2Q
	var X, Q, u1G, u2Q jacobianPoint
	bigAffineToJacobian(pubKey.x, pubKey.y, &Q)
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
	// z = (X.z)^2 mod P (X.z is the z coordinate of X)
	z := new(FieldVal).SquareVal(&X.z)

	// Step 8.
	//
	// Verified if R * z == X.x (mod P)
	sigRModP := modNScalarToField(&sig.r)
	result := new(FieldVal).Mul2(&sigRModP, z).Normalize()
	if result.Equals(&X.x) {
		return true
	}

	// Step 9.
	//
	// Fail if R + N >= P
	if sigRModP.IsGtOrEqPrimeMinusOrder() {
		return false
	}

	// Step 10.
	//
	// Verified if (R + N) * z == X.x (mod P)
	sigRModP.Add(orderAsFieldVal)
	result.Mul2(&sigRModP, z).Normalize()
	return result.Equals(&X.x)
}

// IsEqual compares this Signature instance to the one passed, returning true if
// both Signatures are equivalent.  A signature is equivalent to another, if
// they both have the same scalar value for R and S.
func (sig *Signature) IsEqual(otherSig *Signature) bool {
	return sig.r.Equals(&otherSig.r) && sig.s.Equals(&otherSig.s)
}

// ParseDERSignature parses a signature in the Distinguished Encoding Rules
// (DER) format per section 10 of [ISO/IEC 8825-1] and enforces the following
// additional restrictions specific to secp256k1:
//
// - The R and S values must be in the valid range for secp256k1 scalars:
//   - Negative values are rejected
//   - Zero is rejected
//   - Values greater than or equal to the secp256k1 group order are rejected
func ParseDERSignature(sig []byte) (*Signature, error) {
	// The format of a DER encoded signature for secp256k1 is as follows:
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
	//
	// NOTE: The DER specification supports specifying lengths that can occupy
	// more than 1 byte, however, since this is specific to secp256k1
	// signatures, all lengths will be a single byte.
	const (
		// minSigLen is the minimum length of a DER encoded signature and is
		// when both R and S are 1 byte each.
		//
		// 0x30 + <1-byte> + 0x02 + 0x01 + <byte> + 0x2 + 0x01 + <byte>
		minSigLen = 8

		// maxSigLen is the maximum length of a DER encoded signature and is
		// when both R and S are 33 bytes each.  It is 33 bytes because a
		// 256-bit integer requires 32 bytes and an additional leading null byte
		// might be required if the high bit is set in the value.
		//
		// 0x30 + <1-byte> + 0x02 + 0x21 + <33 bytes> + 0x2 + 0x21 + <33 bytes>
		maxSigLen = 72

		// sequenceOffset is the byte offset within the signature of the
		// expected ASN.1 sequence identifier.
		sequenceOffset = 0

		// dataLenOffset is the byte offset within the signature of the expected
		// total length of all remaining data in the signature.
		dataLenOffset = 1

		// rTypeOffset is the byte offset within the signature of the ASN.1
		// identifier for R and is expected to indicate an ASN.1 integer.
		rTypeOffset = 2

		// rLenOffset is the byte offset within the signature of the length of
		// R.
		rLenOffset = 3

		// rOffset is the byte offset within the signature of R.
		rOffset = 4
	)

	// The signature must adhere to the minimum and maximum allowed length.
	sigLen := len(sig)
	if sigLen < minSigLen {
		str := fmt.Sprintf("malformed signature: too short: %d < %d", sigLen,
			minSigLen)
		return nil, signatureError(ErrSigTooShort, str)
	}
	if sigLen > maxSigLen {
		str := fmt.Sprintf("malformed signature: too long: %d > %d", sigLen,
			maxSigLen)
		return nil, signatureError(ErrSigTooLong, str)
	}

	// The signature must start with the ASN.1 sequence identifier.
	if sig[sequenceOffset] != asn1SequenceID {
		str := fmt.Sprintf("malformed signature: format has wrong type: %#x",
			sig[sequenceOffset])
		return nil, signatureError(ErrSigInvalidSeqID, str)
	}

	// The signature must indicate the correct amount of data for all elements
	// related to R and S.
	if int(sig[dataLenOffset]) != sigLen-2 {
		str := fmt.Sprintf("malformed signature: bad length: %d != %d",
			sig[dataLenOffset], sigLen-2)
		return nil, signatureError(ErrSigInvalidDataLen, str)
	}

	// Calculate the offsets of the elements related to S and ensure S is inside
	// the signature.
	//
	// rLen specifies the length of the big-endian encoded number which
	// represents the R value of the signature.
	//
	// sTypeOffset is the offset of the ASN.1 identifier for S and, like its R
	// counterpart, is expected to indicate an ASN.1 integer.
	//
	// sLenOffset and sOffset are the byte offsets within the signature of the
	// length of S and S itself, respectively.
	rLen := int(sig[rLenOffset])
	sTypeOffset := rOffset + rLen
	sLenOffset := sTypeOffset + 1
	if sTypeOffset >= sigLen {
		str := "malformed signature: S type indicator missing"
		return nil, signatureError(ErrSigMissingSTypeID, str)
	}
	if sLenOffset >= sigLen {
		str := "malformed signature: S length missing"
		return nil, signatureError(ErrSigMissingSLen, str)
	}

	// The lengths of R and S must match the overall length of the signature.
	//
	// sLen specifies the length of the big-endian encoded number which
	// represents the S value of the signature.
	sOffset := sLenOffset + 1
	sLen := int(sig[sLenOffset])
	if sOffset+sLen != sigLen {
		str := "malformed signature: invalid S length"
		return nil, signatureError(ErrSigInvalidSLen, str)
	}

	// R elements must be ASN.1 integers.
	if sig[rTypeOffset] != asn1IntegerID {
		str := fmt.Sprintf("malformed signature: R integer marker: %#x != %#x",
			sig[rTypeOffset], asn1IntegerID)
		return nil, signatureError(ErrSigInvalidRIntID, str)
	}

	// Zero-length integers are not allowed for R.
	if rLen == 0 {
		str := "malformed signature: R length is zero"
		return nil, signatureError(ErrSigZeroRLen, str)
	}

	// R must not be negative.
	if sig[rOffset]&0x80 != 0 {
		str := "malformed signature: R is negative"
		return nil, signatureError(ErrSigNegativeR, str)
	}

	// Null bytes at the start of R are not allowed, unless R would otherwise be
	// interpreted as a negative number.
	if rLen > 1 && sig[rOffset] == 0x00 && sig[rOffset+1]&0x80 == 0 {
		str := "malformed signature: R value has too much padding"
		return nil, signatureError(ErrSigTooMuchRPadding, str)
	}

	// S elements must be ASN.1 integers.
	if sig[sTypeOffset] != asn1IntegerID {
		str := fmt.Sprintf("malformed signature: S integer marker: %#x != %#x",
			sig[sTypeOffset], asn1IntegerID)
		return nil, signatureError(ErrSigInvalidSIntID, str)
	}

	// Zero-length integers are not allowed for S.
	if sLen == 0 {
		str := "malformed signature: S length is zero"
		return nil, signatureError(ErrSigZeroSLen, str)
	}

	// S must not be negative.
	if sig[sOffset]&0x80 != 0 {
		str := "malformed signature: S is negative"
		return nil, signatureError(ErrSigNegativeS, str)
	}

	// Null bytes at the start of S are not allowed, unless S would otherwise be
	// interpreted as a negative number.
	if sLen > 1 && sig[sOffset] == 0x00 && sig[sOffset+1]&0x80 == 0 {
		str := "malformed signature: S value has too much padding"
		return nil, signatureError(ErrSigTooMuchSPadding, str)
	}

	// The signature is validly encoded per DER at this point, however, enforce
	// additional restrictions to ensure R and S are in the range [1, N-1] since
	// valid ECDSA signatures are required to be in that range per spec.
	//
	// Also note that while the overflow checks are required to make use of the
	// specialized mod N scalar type, rejecting zero here is not strictly
	// required because it is also checked when verifying the signature, but
	// there really isn't a good reason not to fail early here on signatures
	// that do not conform to the ECDSA spec.

	// Strip leading zeroes from R.
	rBytes := sig[rOffset : rOffset+rLen]
	for len(rBytes) > 0 && rBytes[0] == 0x00 {
		rBytes = rBytes[1:]
	}

	// R must be in the range [1, N-1].  Notice the check for the maximum number
	// of bytes is required because SetByteSlice truncates as noted in its
	// comment so it could otherwise fail to detect the overflow.
	var r ModNScalar
	if len(rBytes) > 32 {
		str := "invalid signature: R is larger than 256 bits"
		return nil, signatureError(ErrSigRTooBig, str)
	}
	if overflow := r.SetByteSlice(rBytes); overflow {
		str := "invalid signature: R >= group order"
		return nil, signatureError(ErrSigRTooBig, str)
	}
	if r.IsZero() {
		str := "invalid signature: R is 0"
		return nil, signatureError(ErrSigRIsZero, str)
	}

	// Strip leading zeroes from S.
	sBytes := sig[sOffset : sOffset+sLen]
	for len(sBytes) > 0 && sBytes[0] == 0x00 {
		sBytes = sBytes[1:]
	}

	// S must be in the range [1, N-1].  Notice the check for the maximum number
	// of bytes is required because SetByteSlice truncates as noted in its
	// comment so it could otherwise fail to detect the overflow.
	var s ModNScalar
	if len(sBytes) > 32 {
		str := "invalid signature: S is larger than 256 bits"
		return nil, signatureError(ErrSigSTooBig, str)
	}
	if overflow := s.SetByteSlice(sBytes); overflow {
		str := "invalid signature: S >= group order"
		return nil, signatureError(ErrSigSTooBig, str)
	}
	if s.IsZero() {
		str := "invalid signature: S is 0"
		return nil, signatureError(ErrSigSIsZero, str)
	}

	// Create and return the signature.
	return NewSignature(&r, &s), nil
}

const (
	// compactSigSize is the size of a compact signature.  It consists of a
	// compact signature recovery code byte followed by the R and S components
	// serialized as 32-byte big-endian values. 1+32*2 = 65.
	// for the R and S components. 1+32+32=65.
	compactSigSize = 65

	// compactSigMagicOffset is a value used when creating the compact signature
	// recovery code inherited from Bitcoin and has no meaning, but has been
	// retained for compatibility.  For historical purposes, it was originally
	// picked to avoid a binary representation that would allow compact
	// signatures to be mistaken for other components.
	compactSigMagicOffset = 27

	// compactSigCompPubKey is a value used when creating the compact signature
	// recovery code to indicate the original public key was compressed.
	compactSigCompPubKey = 4

	// pubKeyRecoveryCodeOddnessBit specifies the bit that indicates the oddess
	// of the Y coordinate of the random point calculated when creating a
	// signature.
	pubKeyRecoveryCodeOddnessBit = 1 << 0

	// pubKeyRecoveryCodeOverflowBit specifies the bit that indicates the X
	// coordinate of the random point calculated when creating a signature was
	// >= N, where N is the order of the group.
	pubKeyRecoveryCodeOverflowBit = 1 << 1
)

// SignCompact produces a compact signature of the data in hash with the given
// private key on the secp256k1 curve. The isCompressedKey parameter specifies
// if the given signature should reference a compressed public key or not.
//
// Compact signature format:
// <1-byte compact sig recovery code><32-byte R><32-byte S>
//
// The compact sig recovery code is the value 27 + public key recovery code + 4
// if the compact signature was created with a compressed public key.
func SignCompact(key *PrivateKey, hash []byte, isCompressedKey bool) []byte {
	// Create the signature and associated pubkey recovery code and calculate
	// the compact signature recovery code.
	sig, pubKeyRecoveryCode := signRFC6979(key, hash)
	compactSigRecoveryCode := compactSigMagicOffset + pubKeyRecoveryCode
	if isCompressedKey {
		compactSigRecoveryCode += compactSigCompPubKey
	}

	// Serialize the R and S components of the signature into their fixed
	// 32-byte big-endian encoding.
	var rBytes, sBytes [32]byte
	sig.r.PutBytes(&rBytes)
	sig.s.PutBytes(&sBytes)

	// Output <compactSigRecoveryCode><32-byte R><32-byte S>.
	var b [compactSigSize]byte
	b[0] = compactSigRecoveryCode
	copy(b[1:], rBytes[:])
	copy(b[33:], sBytes[:])
	return b[:]
}

// RecoverCompact attempts to recover the secp256k1 public key from the provided
// compact signature and message hash.  It first verifies the signature, and, if
// the signature matches then the recovered public key will be returned as well
// as a boolean indicating whether or not the original key was compressed.
func RecoverCompact(signature, hash []byte) (*PublicKey, bool, error) {
	// The following is very loosely based on the information and algorithm that
	// describes recovering a public key from and ECDSA signature in section
	// 4.1.6 of [SEC1].
	//
	// Given the following parameters:
	//
	// G = curve generator
	// N = group order
	// P = field prime
	// Q = public key
	// m = message
	// e = hash of the message
	// r, s = signature
	// X = random point used when creating signature whose x coordinate is r
	//
	// The equation to recover a public key candidate from an ECDSA signature
	// is:
	// Q = r^-1(sX - eG).
	//
	// This can be verified by plugging it in for Q in the sig verification
	// equation:
	// X = s^-1(eG + rQ) (mod N)
	//  => s^-1(eG + r(r^-1(sX - eG))) (mod N)
	//  => s^-1(eG + sX - eG) (mod N)
	//  => s^-1(sX) (mod N)
	//  => X (mod N)
	//
	// However, note that since r is the x coordinate mod N from a random point
	// that was originally mod P, and the cofactor of the secp256k1 curve is 1,
	// there are four possible points that the original random point could have
	// been to produce r: (r,y), (r,-y), (r+N,y), and (r+N,-y).  At least 2 of
	// those points will successfully verify, and all 4 will successfully verify
	// when the original x coordinate was in the range [N+1, P-1], but in any
	// case, only one of them corresponds to the original private key used.
	//
	// The method described by section 4.1.6 of [SEC1] to determine which one is
	// the correct one involves calculating each possibility as a candidate
	// public key and comparing the candidate to the authentic public key.  It
	// also hints that is is possible to generate the signature in a such a
	// way that only one of the candidate public keys is viable.
	//
	// A more efficient approach that is specific to the secp256k1 curve is used
	// here instead which is to produce a "pubkey recovery code" when signing
	// that uniquely identifies which of the 4 possibilities is correct for the
	// original random point and using that to recover the pubkey directly as
	// follows:
	//
	// 1. Fail if r and s are not in [1, N-1]
	// 2. Convert r to integer mod P
	// 3. If pubkey recovery code overflow bit is set:
	//    3.1 Fail if r + N >= P
	//    3.2 r = r + N (mod P)
	// 4. y = +sqrt(r^3 + 7) (mod P)
	//    4.1 Fail if y does not exist
	//    4.2 y = -y if needed to match pubkey recovery code oddness bit
	// 5. X = (r, y)
	// 6. e = H(m) mod N
	// 7. w = r^-1 mod N
	// 8. u1 = -(e * w) mod N
	//    u2 = s * w mod N
	// 9. Q = u1G + u2X
	// 10. Fail if Q is the point at infinity

	// A compact signature consists of a recovery byte followed by the R and
	// S components serialized as 32-byte big-endian values.
	if len(signature) != compactSigSize {
		return nil, false, errors.New("invalid compact signature size")
	}

	// Parse and validate the compact signature recovery code.
	const (
		minValidCode = compactSigMagicOffset
		maxValidCode = compactSigMagicOffset + compactSigCompPubKey + 3
	)
	sigRecoveryCode := signature[0]
	if sigRecoveryCode < minValidCode || sigRecoveryCode > maxValidCode {
		return nil, false, errors.New("invalid compact signature recovery code")
	}
	sigRecoveryCode -= compactSigMagicOffset
	wasCompressed := sigRecoveryCode&compactSigCompPubKey != 0
	pubKeyRecoveryCode := sigRecoveryCode & 3

	// Step 1.
	//
	// Parse and validate the R and S signature components.
	//
	// Fail if r and s are not in [1, N-1].
	var r, s ModNScalar
	if overflow := r.SetByteSlice(signature[1:33]); overflow {
		return nil, false, errors.New("signature R is >= curve order")
	}
	if r.IsZero() {
		return nil, false, errors.New("signature R is 0")
	}
	if overflow := s.SetByteSlice(signature[33:]); overflow {
		return nil, false, errors.New("signature S is >= curve order")
	}
	if s.IsZero() {
		return nil, false, errors.New("signature S is 0")
	}

	// Step 2.
	//
	// Convert r to integer mod P.
	fieldR := modNScalarToField(&r)

	// Step 3.
	//
	// If pubkey recovery code overflow bit is set:
	if pubKeyRecoveryCode&pubKeyRecoveryCodeOverflowBit != 0 {
		// Step 3.1.
		//
		// Fail if r + N >= P
		//
		// Either the signature or the recovery code must be invalid if the
		// recovery code overflow bit is set and adding N to the R component
		// would exceed the field prime since R originally came from the X
		// coordinate of a random point on the curve.
		if fieldR.IsGtOrEqPrimeMinusOrder() {
			return nil, false, errors.New("signature R + N >= P")
		}

		// Step 3.2.
		//
		// r = r + N (mod P)
		fieldR.Add(orderAsFieldVal)
	}

	// Step 4.
	//
	// y = +sqrt(r^3 + 7) (mod P)
	// Fail if y does not exist.
	// y = -y if needed to match pubkey recovery code oddness bit
	//
	// The signature must be invalid if the calculation fails because the X
	// coord originally came from a random point on the curve which means there
	// must be a Y coord that satisfies the equation for a valid signature.
	oddY := pubKeyRecoveryCode&pubKeyRecoveryCodeOddnessBit != 0
	var y FieldVal
	if valid := decompressY(&fieldR, oddY, &y); !valid {
		return nil, false, errors.New("signature is not for a valid curve point")
	}

	// Step 5.
	//
	// X = (r, y)
	var X jacobianPoint
	X.x.Set(&fieldR).Normalize()
	X.y.Set(&y).Normalize()
	X.z.SetInt(1)

	// Step 6.
	//
	// e = H(m) mod N
	var e ModNScalar
	e.SetByteSlice(hash)

	// Step 7.
	//
	// w = r^-1 mod N
	w := new(ModNScalar).InverseValNonConst(&r)

	// Step 8.
	//
	// u1 = -(e * w) mod N
	// u2 = s * w mod N
	u1 := new(ModNScalar).Mul2(&e, w).Negate()
	u2 := new(ModNScalar).Mul2(&s, w)

	// Step 9.
	//
	// Q = u1G + u2X
	var Q, u1G, u2X jacobianPoint
	scalarBaseMultJacobian(u1, &u1G)
	scalarMultJacobian(u2, &X, &u2X)
	addJacobian(&u1G, &u2X, &Q)

	// Step 10.
	//
	// Fail if Q is the point at infinity.
	//
	// Either the signature or the pubkey recovery code must be invalid if the
	// recovered pubkey is the point at infinity.
	if (Q.x.IsZero() && Q.y.IsZero()) || Q.z.IsZero() {
		return nil, false, errors.New("recovered pubkey is the point at infinity")
	}

	// Notice that the public key is in affine coordinates.
	pubKey := NewPublicKey(jacobianToBigAffine(&Q))
	return pubKey, wasCompressed, nil
}

// signRFC6979 generates a deterministic ECDSA signature according to RFC 6979
// and BIP 62 and returns it along with an additional public key recovery code
// for efficiently recovering the public key from the signature.
func signRFC6979(privateKey *PrivateKey, hash []byte) (*Signature, byte) {
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
		// Note that the point must be in affine coordinates.
		var kG jacobianPoint
		scalarBaseMultJacobian(k, &kG)
		kG.ToAffine()

		// Step 3.
		//
		// r = kG.x mod N
		// Repeat from step 1 if r = 0
		r, overflow := fieldToModNScalar(&kG.x)
		if r.IsZero() {
			continue
		}

		// Since the secp256k1 curve has a cofactor of 1, when recovering a
		// public key from an ECDSA signature over it, there are four possible
		// candidates corresponding to the following cases:
		//
		// 1) The X coord of the random point is < N and its Y coord even
		// 2) The X coord of the random point is < N and its Y coord is odd
		// 3) The X coord of the random point is >= N and its Y coord is even
		// 4) The X coord of the random point is >= N and its Y coord is odd
		//
		// Rather than forcing the recovery procedure to check all possible
		// cases, this creates a recovery code that uniquely identifies which of
		// the cases apply by making use of 2 bits.  Bit 0 identifies the
		// oddness case and Bit 1 identifies the overflow case (aka when the X
		// coord >= N).
		//
		// It is also worth noting that making use of Hasse's theorem shows
		// there are around log_2((p-n)/p) ~= -127.65 ~= 1 in 2^127 points where
		// the X coordinate is >= N.  It is not possible to calculate these
		// points since that would require breaking the ECDLP, but, in practice
		// this strongly implies with extremely high probability that there are
		// only a few actual points for which this case is true.
		pubKeyRecoveryCode := byte(overflow<<1) | byte(kG.y.n[0]&0x01)

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

			// Negating s corresponds to the random point that would have been
			// generated by -k (mod N), which necessarily has the opposite
			// oddness since N is prime, thus flip the pubkey recovery code
			// oddness bit accordingly.
			pubKeyRecoveryCode ^= 0x01
		}

		// Step 6.
		//
		// Return (r,s)
		return NewSignature(&r, s), pubKeyRecoveryCode
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
