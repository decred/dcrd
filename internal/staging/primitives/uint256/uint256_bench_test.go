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
	noElideInt   int
)

// randBenchVal houses values used throughout the benchmarks that are randomly
// generated with each run to ensure they are not overfitted.
type randBenchVal struct {
	buf1       [32]byte
	buf2       [32]byte
	n1         *Uint256
	n2         *Uint256
	n2Low64    *Uint256
	bigN1      *big.Int
	bigN2      *big.Int
	bigN2Low64 *big.Int
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
		val.n2Low64 = new(Uint256).SetUint64(val.n2.Uint64())
		val.bigN1 = new(big.Int).SetBytes(val.buf1[:])
		val.bigN2 = new(big.Int).SetBytes(val.buf2[:])
		val.bigN2Low64 = new(big.Int).SetUint64(val.n2.Uint64())
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

// BenchmarkUint256IsOdd benchmarks determining if an unsigned 256-bit integer
// is odd with the specialized type.
func BenchmarkUint256IsOdd(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			noElideBool = vals[j].n1.IsOdd()
		}
	}
}

// BenchmarkBigIntIsOdd benchmarks determining if an unsigned 256-bit integer is
// odd with stdlib big integers.
func BenchmarkBigIntIsOdd(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			noElideBool = vals[j].bigN1.Bit(0) == 1
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

// BenchmarkUint256Cmp benchmarks comparing two unsigned 256-bit integers with
// the specialized type.
func BenchmarkUint256Cmp(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			noElideInt = val.n1.Cmp(val.n2)
		}
	}
}

// BenchmarkBigIntCmp benchmarks comparing two unsigned 256-bit integers with
// stdlib big integers.
func BenchmarkBigIntCmp(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			noElideInt = val.bigN1.Cmp(val.bigN2)
		}
	}
}

// BenchmarkUint256CmpUint64 benchmarks comparing an unsigned 256-bit integer
// with an unsigned 64-bit integer with the specialized type.
func BenchmarkUint256CmpUint64(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			noElideInt = val.n1.CmpUint64(val.n2Low64.Uint64())
		}
	}
}

// BenchmarkBigIntCmp benchmarks comparing an unsigned 256-bit integer with an
// unsigned 64-bit integer with stdlib big integers.
func BenchmarkBigIntCmpUint64(b *testing.B) {
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			noElideInt = val.bigN1.Cmp(val.bigN2Low64)
		}
	}
}

// BenchmarkUint256Add benchmarks computing the sum of unsigned 256-bit integers
// with the specialized type.
func BenchmarkUint256Add(b *testing.B) {
	n := new(Uint256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Add2(val.n1, val.n2)
		}
	}
}

// BenchmarkBigIntAdd benchmarks computing the sum of unsigned 256-bit integers
// with stdlib big integers.
func BenchmarkBigIntAdd(b *testing.B) {
	n := new(big.Int)
	two256 := new(big.Int).Lsh(big.NewInt(1), 256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Add(val.bigN1, val.bigN2)
			n.Mod(n, two256)
		}
	}
}

// BenchmarkUint256AddUint64 benchmarks computing the sum of an unsigned 256-bit
// integer and an unsigned 64-bit integer with the specialized type.
func BenchmarkUint256AddUint64(b *testing.B) {
	n := new(Uint256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Set(val.n1)
			n.AddUint64(val.n2Low64.Uint64())
		}
	}
}

// BenchmarkBigIntAddUint64 benchmarks computing the sum of an unsigned 256-bit
// integer and an unsigned 64-bit integer with stdlib big integers.
func BenchmarkBigIntAddUint64(b *testing.B) {
	n := new(big.Int)
	two256 := new(big.Int).Lsh(big.NewInt(1), 256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Set(val.bigN1) // For fair comparison.
			n.Add(n, val.bigN2Low64)
			n.Mod(n, two256)
		}
	}
}

// BenchmarkUint256Sub benchmarks computing the difference of unsigned 256-bit
// integers with the specialized type.
func BenchmarkUint256Sub(b *testing.B) {
	n := new(Uint256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Sub2(val.n1, val.n2)
		}
	}
}

// BenchmarkBigIntSub benchmarks computing the difference of unsigned 256-bit
// integers with stdlib big integers.
func BenchmarkBigIntSub(b *testing.B) {
	n := new(big.Int)
	two256 := new(big.Int).Lsh(big.NewInt(1), 256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Sub(val.bigN1, val.bigN2)
			n.Mod(n, two256)
		}
	}
}

// BenchmarkUint256SubUint64 benchmarks computing the difference of an unsigned
// 256-bit integer and unsigned 64-bit integer with the specialized type.
func BenchmarkUint256SubUint64(b *testing.B) {
	n := new(Uint256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Set(val.n1)
			n.SubUint64(val.n2Low64.Uint64())
		}
	}
}

// BenchmarkBigIntSubUint64 benchmarks computing the difference of an unsigned
// 256-bit integer and unsigned 64-bit integer with stdlib big integers.
func BenchmarkBigIntSubUint64(b *testing.B) {
	n := new(big.Int)
	two256 := new(big.Int).Lsh(big.NewInt(1), 256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Set(val.bigN1) // For fair comparison.
			n.Sub(n, val.bigN2Low64)
			n.Mod(n, two256)
		}
	}
}

// BenchmarkUint256Mul benchmarks computing the product of unsigned 256-bit
// integers with the specialized type.
func BenchmarkUint256Mul(b *testing.B) {
	n := new(Uint256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Mul2(val.n1, val.n2)
		}
	}
}

// BenchmarkBigIntMul benchmarks computing the product of unsigned 256-bit
// integers with stdlib big integers.
func BenchmarkBigIntMul(b *testing.B) {
	n := new(big.Int)
	two256 := new(big.Int).Lsh(big.NewInt(1), 256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Mul(val.bigN1, val.bigN2)
			n.Mod(n, two256)
		}
	}
}

// BenchmarkUint256MulUint64 benchmarks computing the product of an unsigned
// 256-bit integer and unsigned 64-bit integer with the specialized type.
func BenchmarkUint256MulUint64(b *testing.B) {
	n := new(Uint256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Set(val.n1)
			n.MulUint64(val.n2Low64.Uint64())
		}
	}
}

// BenchmarkBigIntMulUint64 benchmarks computing the product of an unsigned
// 256-bit integer and unsigned 64-bit integer with stdlib big integers.
func BenchmarkBigIntMulUint64(b *testing.B) {
	n := new(big.Int)
	two256 := new(big.Int).Lsh(big.NewInt(1), 256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Set(val.bigN1) // For fair comparison.
			n.Mul(n, val.bigN2Low64)
			n.Mod(n, two256)
		}
	}
}

// BenchmarkUint256Square benchmarks computing the quotient of unsigned 256-bit
// integers with the specialized type.
func BenchmarkUint256Square(b *testing.B) {
	n := new(Uint256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.SquareVal(val.n1)
		}
	}
}

// BenchmarkBigIntSquare benchmarks computing the quotient of unsigned 256-bit
// integers with stdlib big integers.
func BenchmarkBigIntSquare(b *testing.B) {
	n := new(big.Int)
	two256 := new(big.Int).Lsh(big.NewInt(1), 256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Mul(val.bigN1, val.bigN1)
			n.Mod(n, two256)
		}
	}
}

// divBenchTest describes tests that are used for the deterministic division
// benchmarks.  It is defined separately so the same tests can easily be used in
// comparison benchmarks between the specialized type in this package and stdlib
// big integers.
type divBenchTest struct {
	name string   // benchmark description
	n1   *Uint256 // dividend
	n2   *Uint256 // divisor
}

// makeDivBenches returns a slice of tests that consist of deterministic
// unsigned 256-bit integers of various lengths for use in the division
// benchmarks.
func makeDivBenches() []divBenchTest {
	return []divBenchTest{{
		// (1<<256 - 2) / (1<<256 - 1)
		name: "dividend lt divisor",
		n1:   hexToUint256("fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe"),
		n2:   hexToUint256("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
	}, {
		// (1<<256 - 1) / (1<<256 - 1)
		name: "dividend eq divisor",
		n1:   hexToUint256("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
	}, {
		// (1<<64 - 1) / (1<<64 - 255)
		name: "1 by 1 near",
		n1:   hexToUint256("000000000000000000000000000000000000000000000000ffffffffffffffff"),
		n2:   hexToUint256("000000000000000000000000000000000000000000000000ffffffffffffff01"),
	}, {
		// (1<<64 - 1) / (1<<2 - 1)
		name: "1 by 1 far",
		n1:   hexToUint256("000000000000000000000000000000000000000000000000ffffffffffffffff"),
		n2:   hexToUint256("0000000000000000000000000000000000000000000000000000000000000003"),
	}, {
		// (1<<128 - 1) / (1<<64 - 1)
		name: "2 by 1 near",
		n1:   hexToUint256("00000000000000000000000000000000ffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("000000000000000000000000000000000000000000000000ffffffffffffffff"),
	}, {
		// (1<<128 - 1) / (1<<2 - 1)
		name: "2 by 1 far",
		n1:   hexToUint256("00000000000000000000000000000000ffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("0000000000000000000000000000000000000000000000000000000000000003"),
	}, {
		// (1<<192 - 1) / (1<<64 - 1)
		name: "3 by 1 near",
		n1:   hexToUint256("0000000000000000ffffffffffffffffffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("000000000000000000000000000000000000000000000000ffffffffffffffff"),
	}, {
		// (1<<192 - 1) / (1<<2 - 1)
		name: "3 by 1 far",
		n1:   hexToUint256("0000000000000000ffffffffffffffffffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("0000000000000000000000000000000000000000000000000000000000000003"),
	}, {
		// (1<<256 - 1) / (1<<64 - 1)
		name: "4 by 1 near",
		n1:   hexToUint256("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("000000000000000000000000000000000000000000000000ffffffffffffffff"),
	}, {
		// (1<<256 - 1) / (1<<2 - 1)
		name: "4 by 1 far",
		n1:   hexToUint256("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("0000000000000000000000000000000000000000000000000000000000000003"),
	}, {
		// (1<<128 - 1) / (1<<128 - 255)
		name: "2 by 2 near",
		n1:   hexToUint256("00000000000000000000000000000000ffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("00000000000000000000000000000000ffffffffffffffffffffffffffffff01"),
	}, {
		// (1<<128 - 1) / (1<<65 - 1)
		name: "2 by 2 far",
		n1:   hexToUint256("00000000000000000000000000000000ffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("000000000000000000000000000000000000000000000001ffffffffffffffff"),
	}, {
		// (1<<192 - 1) / (1<<128 - 1)
		name: "3 by 2 near",
		n1:   hexToUint256("0000000000000000ffffffffffffffffffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("00000000000000000000000000000000ffffffffffffffffffffffffffffffff"),
	}, {
		// (1<<192 - 1) / (1<<65 - 1)
		name: "3 by 2 far",
		n1:   hexToUint256("0000000000000000ffffffffffffffffffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("000000000000000000000000000000000000000000000001ffffffffffffffff"),
	}, {
		// (1<<256 - 1) / (1<<128 - 1)
		name: "4 by 2 near",
		n1:   hexToUint256("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("00000000000000000000000000000000ffffffffffffffffffffffffffffffff"),
	}, {
		// (1<<256 - 1) / (1<<65 - 1)
		name: "4 by 2 far",
		n1:   hexToUint256("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("000000000000000000000000000000000000000000000001ffffffffffffffff"),
	}, {
		// (1<<192 - 1) / (1<<192 - 255)
		name: "3 by 3 near",
		n1:   hexToUint256("0000000000000000ffffffffffffffffffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("0000000000000000ffffffffffffffffffffffffffffffffffffffffffffff01"),
	}, {
		// (1<<192 - 1) / (1<<129 - 1)
		name: "3 by 3 far",
		n1:   hexToUint256("0000000000000000ffffffffffffffffffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("00000000000000000000000000000001ffffffffffffffffffffffffffffffff"),
	}, {
		// (1<<256 - 1) / (1<<192 - 1)
		name: "4 by 3 near",
		n1:   hexToUint256("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("0000000000000000ffffffffffffffffffffffffffffffffffffffffffffffff"),
	}, {
		// (1<<256 - 1) / (1<<129 - 1)
		name: "4 by 3 far",
		n1:   hexToUint256("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("00000000000000000000000000000001ffffffffffffffffffffffffffffffff"),
	}, {
		// (1<<256 - 1) / (1<<256 - 255)
		name: "4 by 4 near",
		n1:   hexToUint256("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff01"),
	}, {
		// (1<<256 - 1) / (1<<193 - 1)
		name: "4 by 4 far",
		n1:   hexToUint256("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
		n2:   hexToUint256("0000000000000001ffffffffffffffffffffffffffffffffffffffffffffffff"),
	}}
}

// BenchmarkUint256Div benchmarks computing the quotient of deterministic
// unsigned 256-bit integers of various length with the specialized type.
func BenchmarkUint256Div(b *testing.B) {
	benches := makeDivBenches()
	for benchIdx := range benches {
		bench := benches[benchIdx]
		b.Run(bench.name, func(b *testing.B) {
			n := new(Uint256)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				n.Div2(bench.n1, bench.n2)
			}
		})
	}
}

// BenchmarkUint256DivRandom benchmarks computing the quotient of random large
// unsigned 256-bit integers with the specialized type.
func BenchmarkUint256DivRandom(b *testing.B) {
	n := new(Uint256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Div2(val.n1, val.n2)
		}
	}
}

// BenchmarkBigIntDiv benchmarks computing the quotient of deterministic
// unsigned 256-bit integers of various length with stdlib big integers.
func BenchmarkBigIntDiv(b *testing.B) {
	benches := makeDivBenches()
	for benchIdx := range benches {
		bench := benches[benchIdx]
		b.Run(bench.name, func(b *testing.B) {
			n := new(big.Int)
			n1Bytes, n2Bytes := bench.n1.Bytes(), bench.n2.Bytes()
			n1 := new(big.Int).SetBytes(n1Bytes[:])
			n2 := new(big.Int).SetBytes(n2Bytes[:])

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				n.Div(n1, n2)
			}
		})
	}
}

// BenchmarkBigIntDivRandom benchmarks computing the quotient of random large
// unsigned 256-bit integers with stdlib big integers.
func BenchmarkBigIntDivRandom(b *testing.B) {
	n := new(big.Int)
	two256 := new(big.Int).Lsh(big.NewInt(1), 256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Div(val.bigN1, val.bigN2)
			n.Mod(n, two256)
		}
	}
}

// BenchmarkUint256DivUint64 benchmarks computing the quotient of an unsigned
// 256-bit integer and unsigned 64-bit integer with the specialized type.
func BenchmarkUint256DivUint64(b *testing.B) {
	n := new(Uint256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Set(val.n1)
			n.DivUint64(val.n2Low64.Uint64())
		}
	}
}

// BenchmarkBigIntDivUint64 benchmarks computing the quotient of an unsigned
// 256-bit integer and unsigned 64-bit integer with stdlib big integers.
func BenchmarkBigIntDivUint64(b *testing.B) {
	n := new(big.Int)
	two256 := new(big.Int).Lsh(big.NewInt(1), 256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			val := &vals[j]
			n.Div(val.bigN1, val.bigN2Low64)
			n.Mod(n, two256)
		}
	}
}

// BenchmarkUint256Negate benchmarks computing the negation modulo 2^256 of an
// unsigned 256-bit integer with the specialized type.
func BenchmarkUint256Negate(b *testing.B) {
	n := new(Uint256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			n.NegateVal(vals[j].n1)
		}
	}
}

// BenchmarkBigIntNegate benchmarks computing the negation module 2^256 of an
// unsigned 256-bit integer with stdlib big integers.
func BenchmarkBigIntNegate(b *testing.B) {
	n := new(big.Int)
	two256 := new(big.Int).Lsh(big.NewInt(1), 256)
	vals := randBenchVals

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i += len(vals) {
		for j := 0; j < len(vals); j++ {
			n.Mod(n.Neg(vals[j].bigN1), two256)
		}
	}
}

// BenchmarkUint256Lsh benchmarks left shifting an unsigned 256-bit integer with
// the specialized type.
func BenchmarkUint256Lsh(b *testing.B) {
	for _, bits := range []uint32{0, 1, 64, 128, 192, 255, 256} {
		benchName := fmt.Sprintf("bits %d", bits)
		b.Run(benchName, func(b *testing.B) {
			result := new(Uint256)
			max256 := new(Uint256).SetByteSlice(maxUint256Bytes())

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result.LshVal(max256, bits)
			}
		})
	}
}

// BenchmarkBigIntLsh benchmarks left shifting an unsigned 256-bit integer with
// stdlib big integers.
func BenchmarkBigIntLsh(b *testing.B) {
	for _, bits := range []uint{0, 1, 64, 128, 192, 255, 256} {
		benchName := fmt.Sprintf("bits %d", bits)
		b.Run(benchName, func(b *testing.B) {
			result := new(big.Int)
			max256 := new(big.Int).SetBytes(maxUint256Bytes())

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result.Lsh(max256, bits)
			}
		})
	}
}

// BenchmarkUint256Rsh benchmarks right shifting an unsigned 256-bit integer
// with the specialized type.
func BenchmarkUint256Rsh(b *testing.B) {
	for _, bits := range []uint32{0, 1, 64, 128, 192, 255, 256} {
		benchName := fmt.Sprintf("bits %d", bits)
		b.Run(benchName, func(b *testing.B) {
			result := new(Uint256)
			max256 := new(Uint256).SetByteSlice(maxUint256Bytes())

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result.RshVal(max256, bits)
			}
		})
	}
}

// BenchmarkBigIntRsh benchmarks right shifting an unsigned 256-bit integer with
// stdlib big integers.
func BenchmarkBigIntRsh(b *testing.B) {
	for _, bits := range []uint{0, 1, 64, 128, 192, 255, 256} {
		benchName := fmt.Sprintf("bits %d", bits)
		b.Run(benchName, func(b *testing.B) {
			result := new(big.Int)
			max256 := new(big.Int).SetBytes(maxUint256Bytes())

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result.Rsh(max256, bits)
			}
		})
	}
}
