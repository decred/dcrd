// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"math/big"
)

// Errors returned by canonicalPadding.
var (
	errNegativeValue          = errors.New("value may be interpreted as negative")
	errExcessivelyPaddedValue = errors.New("value is excessively padded")
)

// Signature is a type representing an ecdsa signature.
type Signature struct {
	R *big.Int
	S *big.Int
}

var (
	// Curve order and halforder, used to tame ECDSA malleability (see BIP-0062)
	order     = new(big.Int).Set(S256().N)
	halforder = new(big.Int).Rsh(order, 1)

	// Used in RFC6979 implementation when testing the nonce for correctness.
	bigOne = big.NewInt(1)

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

// Serialize returns the ECDSA signature in the more strict DER format.  Note
// that the serialized bytes returned do not include the appended hash type used
// in Decred signature scripts.
//
// 0x30 <length> 0x02 <length r> r 0x02 <length s> s
func (sig *Signature) Serialize() []byte {
	// Low 'S' malleability breaker.
	sigS := sig.S
	if sigS.Cmp(halforder) == 1 {
		sigS = new(big.Int).Sub(order, sigS)
	}

	// Ensure the encoded bytes for the R and S values are canonical and thus
	// suitable for DER encoding.
	rb := canonicalizeInt(sig.R)
	sb := canonicalizeInt(sigS)

	// Total length of returned signature is 1 byte for each magic and length
	// (6 total), plus lengths of R and S.
	length := 6 + len(rb) + len(sb)
	b := make([]byte, length)

	b[0] = 0x30
	b[1] = byte(length - 2)
	b[2] = 0x02
	b[3] = byte(len(rb))
	offset := copy(b[4:], rb) + 4
	b[offset] = 0x02
	b[offset+1] = byte(len(sb))
	copy(b[offset+2:], sb)
	return b
}

// Verify returns whether or not the signature is valid for the provided hash
// and secp256k1 public key.
func (sig *Signature) Verify(hash []byte, pubKey *PublicKey) bool {
	return ecdsa.Verify(pubKey.ToECDSA(), hash, sig.R, sig.S)
}

// IsEqual compares this Signature instance to the one passed, returning true if
// both Signatures are equivalent.  A signature is equivalent to another, if
// they both have the same scalar value for R and S.
func (sig *Signature) IsEqual(otherSig *Signature) bool {
	return sig.R.Cmp(otherSig.R) == 0 &&
		sig.S.Cmp(otherSig.S) == 0
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
	signature.R = new(big.Int).SetBytes(rBytes)
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
	signature.S = new(big.Int).SetBytes(sBytes)
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
	if signature.R.Sign() != 1 {
		return nil, errors.New("signature R isn't 1 or more")
	}
	if signature.S.Sign() != 1 {
		return nil, errors.New("signature S isn't 1 or more")
	}
	if signature.R.Cmp(curve.Params().N) >= 0 {
		return nil, errors.New("signature R is >= curve.N")
	}
	if signature.S.Cmp(curve.Params().N) >= 0 {
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

// canonicalizeInt returns the bytes for the passed big integer adjusted as
// necessary to ensure that a big-endian encoded integer can't possibly be
// misinterpreted as a negative number.  This can happen when the most
// significant bit is set, so it is padded by a leading zero byte in this case.
// Also, the returned bytes will have at least a single byte when the passed
// value is 0.  This is required for DER encoding.
func canonicalizeInt(val *big.Int) []byte {
	b := val.Bytes()
	if len(b) == 0 {
		b = []byte{0x00}
	}
	if b[0]&0x80 != 0 {
		paddedBytes := make([]byte, len(b)+1)
		copy(paddedBytes[1:], b)
		b = paddedBytes
	}
	return b
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
	Rx.Add(Rx, sig.R)
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
	invr := new(big.Int).ModInverse(sig.R, curve.Params().N)

	// first term.
	invrS := new(big.Int).Mul(invr, sig.S)
	invrS.Mod(invrS, curve.Params().N)
	fRx, fRy := bigAffineToField(Rx, Ry)
	fRz := new(fieldVal).SetInt(1)
	sRx, sRy, sRz := new(fieldVal), new(fieldVal), new(fieldVal)
	scalarMultJacobian(fRx, fRy, fRz, invrS.Bytes(), sRx, sRy, sRz)

	// second term.
	e.Neg(e)
	e.Mod(e, curve.Params().N)
	e.Mul(e, invr)
	e.Mod(e, curve.Params().N)
	minuseGx, minuseGy, minuseGz := new(fieldVal), new(fieldVal), new(fieldVal)
	fQx, fQy, fQz := new(fieldVal), new(fieldVal), new(fieldVal)
	scalarBaseMultJacobian(e.Bytes(), minuseGx, minuseGy, minuseGz)
	addJacobian(sRx, sRy, sRz, minuseGx, minuseGy, minuseGz, fQx, fQy, fQz)

	Qx, Qy := fieldJacobianToBigAffine(fQx, fQy, fQz)

	return &PublicKey{
		Curve: curve,
		X:     Qx,
		Y:     Qy,
	}, nil
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

	curve := S256()
	// bitcoind checks the bit length of R and S here. The ecdsa signature
	// algorithm returns R and S mod N therefore they will be the bitsize of
	// the curve, and thus correctly sized.
	for i := 0; i < (curve.H+1)*2; i++ {
		pk, err := recoverKeyFromSignature(sig, hash, i, true)
		if err == nil && pk.X.Cmp(key.X) == 0 && pk.Y.Cmp(key.Y) == 0 {
			result := make([]byte, 1, 2*curve.byteSize+1)
			result[0] = 27 + byte(i)
			if isCompressedKey {
				result[0] += 4
			}
			// Not sure this needs rounding but safer to do so.
			curvelen := (curve.BitSize + 7) / 8

			// Pad R and S to curvelen if needed.
			bytelen := (sig.R.BitLen() + 7) / 8
			if bytelen < curvelen {
				result = append(result,
					make([]byte, curvelen-bytelen)...)
			}
			result = append(result, sig.R.Bytes()...)

			bytelen = (sig.S.BitLen() + 7) / 8
			if bytelen < curvelen {
				result = append(result,
					make([]byte, curvelen-bytelen)...)
			}
			result = append(result, sig.S.Bytes()...)

			return result, nil
		}
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
		R: new(big.Int).SetBytes(signature[1 : bitlen+1]),
		S: new(big.Int).SetBytes(signature[bitlen+1:]),
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
	curve := S256()
	N := order
	for iteration := uint32(0); ; iteration++ {
		k := NonceRFC6979(privateKey.D, hash, nil, nil, iteration)
		inv := new(big.Int).ModInverse(k, N)
		r, _ := curve.ScalarBaseMult(k.Bytes())
		r.Mod(r, N)
		if r.Sign() == 0 {
			continue
		}

		e := hashToInt(hash)
		s := new(big.Int).Mul(privateKey.D, r)
		s.Add(s, e)
		s.Mul(s, inv)
		s.Mod(s, N)

		if s.Cmp(halforder) == 1 {
			s.Sub(N, s)
		}
		if s.Sign() == 0 {
			continue
		}
		return &Signature{R: r, S: s}
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
func NonceRFC6979(privKey *big.Int, hash []byte, extra []byte, version []byte, extraIterations uint32) *big.Int {
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

	// Drop most significant bytes of private key and hash if they are too long
	// and leave left padding of zeros when they're too short.
	privKeyBytes := privKey.Bytes()
	if len(privKeyBytes) > privKeyLen {
		copy(privKeyBytes, privKeyBytes[privKeyLen-len(privKeyBytes):])
	}
	if len(hash) > hashLen {
		copy(hash, privKeyBytes[hashLen-len(hash):])
	}
	offset := privKeyLen - len(privKeyBytes) // Zero left padding if needed.
	offset += copy(keyBuf[offset:], privKeyBytes)
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
	curve := S256()
	q := curve.Params().N
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
		secret := hashToInt(v)
		if secret.Cmp(bigOne) >= 0 && secret.Cmp(q) < 0 {
			generated++
			if generated > extraIterations {
				return secret
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
