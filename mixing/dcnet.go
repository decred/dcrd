// Copyright (c) 2019-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixing

import (
	"encoding/binary"
	"math/big"

	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/wire"
)

// SRMixPads creates a vector of exponential DC-net pads from a vector of
// shared secrets with each participating peer in the DC-net.
func SRMixPads(kp [][]byte, my uint32) []*big.Int {
	h := blake256.NewHasher256()
	scratch := make([]byte, 8)
	pads := make([]*big.Int, len(kp))
	partialPad := new(big.Int)
	for j := uint32(0); j < uint32(len(kp)); j++ {
		pads[j] = new(big.Int)
		binary.LittleEndian.PutUint64(scratch, uint64(j)+1)
		for i := uint32(0); i < uint32(len(kp)); i++ {
			if my == i {
				continue
			}
			h.Reset()
			h.Write(kp[i])
			h.Write(scratch)
			digest := h.Sum256()
			partialPad.SetBytes(digest[:])
			if my > i {
				pads[j].Add(pads[j], partialPad)
			} else {
				pads[j].Sub(pads[j], partialPad)
			}
		}
		pads[j].Mod(pads[j], F)
	}
	return pads
}

// SRMix creates the padded {m**1, m**2, ..., m**n} message exponentials
// vector.  Message must be bounded by the field prime and must be unique to
// every exponential SR run in a mix session to ensure anonymity.
func SRMix(m *big.Int, pads []*big.Int) []*big.Int {
	mix := make([]*big.Int, len(pads))
	exp := new(big.Int)
	for i := int64(0); i < int64(len(mix)); i++ {
		mexp := new(big.Int).Exp(m, exp.SetInt64(i+1), nil)
		mix[i] = mexp.Add(mexp, pads[i])
		mix[i].Mod(mix[i], F)
	}
	return mix
}

// IntVectorsFromBytes creates a 2-dimensional *big.Int slice from their absolute
// values as bytes.
func IntVectorsFromBytes(vs [][][]byte) [][]*big.Int {
	ints := make([][]*big.Int, len(vs))
	for i := range vs {
		ints[i] = make([]*big.Int, len(vs[i]))
		for j := range vs[i] {
			ints[i][j] = new(big.Int).SetBytes(vs[i][j])
		}
	}
	return ints
}

// IntVectorsToBytes creates a 2-dimensional slice of big.Int absolute values as
// bytes.
func IntVectorsToBytes(ints [][]*big.Int) [][][]byte {
	bytes := make([][][]byte, len(ints))
	for i := range ints {
		bytes[i] = make([][]byte, len(ints[i]))
		for j := range ints[i] {
			bytes[i][j] = ints[i][j].Bytes()
		}
	}
	return bytes
}

// AddVectors sums each vector element over F, returning a new vector.  When
// peers are honest (DC-mix pads sum to zero) this creates the unpadded vector
// of message power sums.
func AddVectors(vs ...[]*big.Int) []*big.Int {
	sums := make([]*big.Int, len(vs))
	for i := range sums {
		sums[i] = new(big.Int)
		for j := range vs {
			sums[i].Add(sums[i], vs[j][i])
		}
		sums[i].Mod(sums[i], F)
	}
	return sums
}

// Coefficients calculates a{0}..a{n} for the polynomial:
//
//	g(x) = a{0} + a{1}x + a{2}x**2 + ... + a{n-1}x**(n-1) + a{n}x**n  (mod F)
//
// where
//
//	a{n}   = -1
//	a{n-1} = -(1/1) *    a{n}*S{0}
//	a{n-2} = -(1/2) * (a{n-1}*S{0} +   a{n}*S{1})
//	a{n-3} = -(1/3) * (a{n-2}*S{0} + a{n-1}*S{1} + a{n}*S{2})
//	...
//
// The roots of this polynomial are the set of recovered messages.
//
// Note that the returned slice of coefficients is one element larger than the
// slice of partial sums.
func Coefficients(S []*big.Int) []*big.Int {
	n := len(S) + 1
	a := make([]*big.Int, n)
	a[len(a)-1] = big.NewInt(-1)
	a[len(a)-1].Add(a[len(a)-1], F) // a{n} = -1 (mod F) = F - 1
	scratch := new(big.Int)
	for i := 0; i < len(a)-1; i++ {
		a[n-2-i] = new(big.Int)
		for j := 0; j <= i; j++ {
			a[n-2-i].Add(a[n-2-i], scratch.Mul(a[n-1-i+j], S[j]))
		}
		xinv := scratch.ModInverse(scratch.SetInt64(int64(i)+1), F)
		xinv.Neg(xinv)
		a[n-2-i].Mul(a[n-2-i], xinv)
		a[n-2-i].Mod(a[n-2-i], F)
	}
	return a
}

// IsRoot checks that the message m is a root of the polynomial with
// coefficients a (mod F) without solving for every root.
func IsRoot(m *big.Int, a []*big.Int) bool {
	sum := new(big.Int)
	scratch := new(big.Int)
	for i := range a {
		scratch.Exp(m, scratch.SetInt64(int64(i)), F)
		scratch.Mul(scratch, a[i])
		sum.Add(sum, scratch)
	}
	sum.Mod(sum, F)
	return sum.Sign() == 0
}

// DCMixPads creates the vector of DC-net pads from shared secrets with each mix
// participant.
func DCMixPads(kp []wire.MixVect, my uint32) Vec {
	pads := make(Vec, len(kp))
	for i := range kp {
		if uint32(i) == my {
			continue
		}
		pads.Xor(pads, Vec(kp[i]))
	}
	return pads
}

// DCMix creates the DC-net vector of message m xor'd into m's reserved
// anonymous slot position of the pads DC-net pads.  Panics if len(m) is not the
// vector's message size.
func DCMix(pads Vec, m []byte, slot uint32) Vec {
	if len(m) != Msize {
		panic("m is not len Msize")
	}

	dcmix := make(Vec, len(pads))
	copy(dcmix, pads)
	slotm := dcmix[slot][:]
	for i := range m {
		slotm[i] ^= m[i]
	}
	return dcmix
}
