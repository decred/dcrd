// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package uint256

import (
	"fmt"
	"math/big"
	"math/rand"
	"testing"
	"time"
)

// randBenchVal houses values used throughout the benchmarks that are randomly
// generated with each run to ensure they are not overfitted.
type randBenchVal struct {
	buf1  [32]byte
	buf2  [32]byte
	n1    *Uint256
	n2    *Uint256
	bigN1 *big.Int
	bigN2 *big.Int
}

// randBenchVals houses a slice of the aforementioned randomly-generated values
// to be used throughout the benchmarks to ensure they are not overfitted.
var randBenchVals = func() []randBenchVal {
	// Use a unique random seed each benchmark instance.
	seed := time.Now().Unix()
	rng := rand.New(rand.NewSource(seed))
	var zeroArr [32]byte

	vals := make([]randBenchVal, 512)
	for i := 0; i < len(vals); i++ {
		val := &vals[i]
		if _, err := rng.Read(val.buf1[:]); err != nil {
			panic(fmt.Sprintf("failed to read random: %v", err))
		}
		for val.buf2 == zeroArr {
			if _, err := rng.Read(val.buf2[:]); err != nil {
				panic(fmt.Sprintf("failed to read random: %v", err))
			}
		}

		val.n1 = new(Uint256).SetBytes(&val.buf1)
		val.n2 = new(Uint256).SetBytes(&val.buf2)
		val.bigN1 = new(big.Int).SetBytes(val.buf1[:])
		val.bigN2 = new(big.Int).SetBytes(val.buf2[:])
	}
	return vals
}()

// BenchmarkUint256SetBytes benchmarks initializing an unsigned 256-bit integer
// from bytes in big endian with the specialized type.
func BenchmarkUint256SetBytes(b *testing.B) {
	n := new(Uint256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			n.SetBytes(&vals[j].buf1)
		}
	}
}

// BenchmarkBigIntSetBytes benchmarks initializing an unsigned 256-bit integer
// from bytes in big endian with stdlib big integers.
func BenchmarkBigIntSetBytes(b *testing.B) {
	n := new(big.Int)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			n.SetBytes(vals[j].buf1[:])
		}
	}
}

// BenchmarkUint256SetBytesLE benchmarks initializing an unsigned 256-bit
// integer from bytes in little endian with the specialized type.
func BenchmarkUint256SetBytesLE(b *testing.B) {
	n := new(Uint256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			n.SetBytesLE(&vals[j].buf1)
		}
	}
}

// BenchmarkBigIntSetBytesLE benchmarks initializing an unsigned 256-bit integer
// from bytes in little endian with stdlib big integers.
func BenchmarkBigIntSetBytesLE(b *testing.B) {
	n := new(big.Int)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			// The big int API only takes the bytes in big endian, so they need
			// to be reversed by the caller.  Note that this implementation
			// assumes the buffer is already 32 bytes and is not robust against
			// other cases, but it's good enough for a reasonable benchmark.
			buf := vals[j].buf1[:]
			reversed := make([]byte, len(buf))
			blen := len(buf)
			for i := 0; i < blen/2; i++ {
				reversed[i], reversed[blen-1-i] = buf[blen-1-i], buf[i]
			}
			n.SetBytes(reversed)
		}
	}
}
