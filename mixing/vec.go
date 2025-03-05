// Copyright (c) 2019-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixing

import (
	"fmt"
	"slices"
	"strings"

	"github.com/decred/dcrd/mixing/internal/chacha20prng"
)

// Msize is the size of the message being mixed.  This is the size of a
// HASH160, which allows mixes to be create either all P2PKH or P2SH outputs.
const Msize = 20

// Vec is a N-element vector of Msize []byte messages.
type Vec [][Msize]byte

func randVec(n uint32, prng *chacha20prng.Reader) Vec {
	v := make(Vec, n)
	for i := range v {
		prng.Read(v[i][:])
	}
	return v
}

// Equals returns whether the two vectors have equal dimensions and data.
func (v Vec) Equals(other Vec) bool {
	return slices.Equal(other, v)
}

func (v Vec) String() string {
	b := new(strings.Builder)
	b.Grow(2 + len(v)*(2*Msize+1))
	b.WriteString("[")
	for i := range v {
		if i != 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(b, "%x", v[i][:])
	}
	b.WriteString("]")
	return b.String()
}

// Xor writes the xor of each vector element of src1 and src2 into v.
// Source and destination vectors are allowed to be equal.
// Panics if vectors do not share identical dimensions.
func (v Vec) Xor(src1, src2 Vec) {
	if len(v) != len(src1) || len(v) != len(src2) {
		panic("dcnet: vectors do not share identical dimensions")
	}
	for i := range v {
		for j := range v[i] {
			v[i][j] = src1[i][j] ^ src2[i][j]
		}
	}
}

// XorVectors calculates the xor of all vectors.
// Panics if vectors do not share identical dimensions.
func XorVectors(vs []Vec) Vec {
	res := make(Vec, len(vs[0]))
	for i := range vs {
		res.Xor(res, vs[i])
	}
	return res
}
