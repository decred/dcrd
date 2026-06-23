// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import "testing"

// BenchmarkField64Add benchmarks adding two unsigned 256-bit big-endian
// integers modulo the field prime with [FieldVal64].
func BenchmarkField64Add(b *testing.B) {
	a := mustFieldVal64("d2e670a19c6d753d1a6d8b20bd045df8a08fb162cf508956c31268c6d81ffdab")
	c := mustFieldVal64("16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sum FieldVal64
		sum.Add2(a, c)
	}
}

// BenchmarkField64Mul benchmarks multiplying two unsigned 256-bit big-endian
// integers modulo the field prime with [FieldVal64].
func BenchmarkField64Mul(b *testing.B) {
	a := mustFieldVal64("d2e670a19c6d753d1a6d8b20bd045df8a08fb162cf508956c31268c6d81ffdab")
	c := mustFieldVal64("16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var prod FieldVal64
		prod.Mul2(a, c)
	}
}

// BenchmarkField64Sqrt benchmarks calculating the square root of an unsigned
// 256-bit big-endian integer modulo the field prime with the FieldVal64 type.
func BenchmarkField64Sqrt(b *testing.B) {
	// The function is constant time so any value is fine.
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	f := mustFieldVal64(valHex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result FieldVal64
		_ = result.SquareRootVal(f)
	}
}

// BenchmarkField64Square benchmarks squaring a 256-bit big-endian integer
// modulo the field prime with [FieldVal64].
func BenchmarkField64Square(b *testing.B) {
	a := mustFieldVal64("16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sq FieldVal64
		sq.SquareVal(a)
	}
}

// BenchmarkField64Inverse benchmarks calculating the multiplicative inverse of
// an unsigned 256-bit big-endian integer modulo the field prime with
// [FieldVal64].
func BenchmarkField64Inverse(b *testing.B) {
	// The function is constant time so any value is fine.
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	f := mustFieldVal64(valHex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Inverse()
	}
}

// BenchmarkField64IsGtOrEqPrimeMinusOrder benchmarks determining whether a
// value is greater than or equal to the field prime minus the group order with
// [FieldVal64].
func BenchmarkField64IsGtOrEqPrimeMinusOrder(b *testing.B) {
	// The function is constant time so any value is fine.
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	f := mustFieldVal64(valHex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.IsGtOrEqPrimeMinusOrder()
	}
}
