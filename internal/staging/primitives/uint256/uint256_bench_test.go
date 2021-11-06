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

// These variables are used to help ensure the benchmarks do not elide code.
var (
	noElideBool  bool
	noElideBytes []byte
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

// maxUint256Bytes returns the raw bytes for a max value unsigned 256-bit
// big-endian integer used throughout the benchmarks.
func maxUint256Bytes() []byte {
	return hexToBytes("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
}

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

// BenchmarkUint256Bytes benchmarks unpacking an unsigned 256-bit integer to
// bytes in big endian with the specialized type.
func BenchmarkUint256Bytes(b *testing.B) {
	vals := randBenchVals
	var buf [32]byte

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			vals[j].n1.PutBytes(&buf)
		}
	}
}

// BenchmarkBigIntBytes benchmarks unpacking an unsigned 256-bit integer to
// bytes in big endian with the stdlib big integers.
func BenchmarkBigIntBytes(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			noElideBytes = vals[j].bigN1.Bytes()
		}
	}
}

// BenchmarkUint256BytesLE benchmarks unpacking an unsigned 256-bit integer to
// bytes in little endian with the specialized type.
func BenchmarkUint256BytesLE(b *testing.B) {
	vals := randBenchVals
	var buf [32]byte

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			vals[j].n1.PutBytesLE(&buf)
		}
	}
}

// BenchmarkBigIntBytesLE benchmarks unpacking an unsigned 256-bit integer to
// bytes in little endian with the stdlib big integers.
func BenchmarkBigIntBytesLE(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			// The big int API only provides the bytes in big endian, so they
			// need to be reversed by the caller.  Note that this implementation
			// assumes the buffer is already 32 bytes and is not robust against
			// other cases, but it's good enough for a reasonable benchmark.
			buf := vals[j].bigN1.Bytes()
			blen := len(buf)
			for i := 0; i < blen/2; i++ {
				buf[i], buf[blen-1-i] = buf[blen-1-i], buf[i]
			}
			noElideBytes = buf
		}
	}
}

// BenchmarkUint256Zero benchmarks zeroing an unsigned 256-bit integer with the
// specialized type.
func BenchmarkUint256Zero(b *testing.B) {
	n := new(Uint256).SetByteSlice(maxUint256Bytes())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n.Zero()
	}
}

// BenchmarkBigIntZero benchmarks zeroing an unsigned 256-bit integer with
// stdlib big integers.
func BenchmarkBigIntZero(b *testing.B) {
	n := new(big.Int).SetBytes(maxUint256Bytes())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n.SetUint64(0)
	}
}

// BenchmarkUint256IsZero benchmarks determining if an unsigned 256-bit integer
// is zero with the specialized type.
func BenchmarkUint256IsZero(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			noElideBool = vals[j].n1.IsZero()
		}
	}
}

// BenchmarkBigIntIsZero benchmarks determining if an unsigned 256-bit integer
// is zero with stdlib big integers.
func BenchmarkBigIntIsZero(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			noElideBool = vals[j].bigN1.Sign() == 0
		}
	}
}

// BenchmarkUint256Eq benchmarks determining equality between two unsigned
// 256-bit integers with the specialized type.
func BenchmarkUint256Eq(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			noElideBool = val.n1.Eq(val.n2)
		}
	}
}

// BenchmarkBigIntEq benchmarks determining equality between two unsigned
// 256-bit integers with stdlib big integers.
func BenchmarkBigIntEq(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			noElideBool = val.bigN1.Cmp(val.bigN2) == 0
		}
	}
}

// BenchmarkUint256Lt benchmarks determining if one unsigned 256-bit integer is
// less than another with the specialized type.
func BenchmarkUint256Lt(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			noElideBool = val.n1.Lt(val.n2)
		}
	}
}

// BenchmarkBigIntLt benchmarks determining if one unsigned 256-bit integer is
// less than another with stdlib big integers.
func BenchmarkBigIntLt(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			noElideBool = val.bigN1.Cmp(val.bigN2) < 0
		}
	}
}

// BenchmarkUint256Gt benchmarks determining if one unsigned 256-bit integer is
// greater than another with the specialized type.
func BenchmarkUint256Gt(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			noElideBool = val.n1.Gt(val.n2)
		}
	}
}

// BenchmarkBigIntGt benchmarks determining if one unsigned 256-bit integer is
// greater than another with stdlib big integers.
func BenchmarkBigIntGt(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			noElideBool = val.bigN1.Cmp(val.bigN2) > 0
		}
	}
}
