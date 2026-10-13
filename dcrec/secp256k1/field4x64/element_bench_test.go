// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package field4x64

import (
	"testing"
)

// BenchmarkElementNegate benchmarks calculating the additive inverse of an
// unsigned 256-bit big-endian integer modulo the field prime with [Element].
func BenchmarkElementNegate(b *testing.B) {
	// The function is constant time so any value is fine.
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	e := mustElement(valHex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result Element
		_ = result.NegateVal(e, 0)
	}
}

// BenchmarkElementAdd benchmarks adding two unsigned 256-bit big-endian
// integers modulo the field prime with [Element].
func BenchmarkElementAdd(b *testing.B) {
	a := mustElement("d2e670a19c6d753d1a6d8b20bd045df8a08fb162cf508956c31268c6d81ffdab")
	c := mustElement("16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sum Element
		sum.Add2(a, c)
	}
}

// BenchmarkElementMulBy2 benchmarks multiplying an unsigned 256-bit big-endian
// integer by 2 with [Element.MulBy2].
func BenchmarkElementMulBy2(b *testing.B) {
	eHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	e := mustElement(eHex)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.MulBy2()
	}
}

// BenchmarkElementMulBy3 benchmarks multiplying an unsigned 256-bit big-endian
// integer by 3 with [Element.MulBy3].
func BenchmarkElementMulBy3(b *testing.B) {
	eHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	e := mustElement(eHex)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.MulBy3()
	}
}

// BenchmarkElementMulBy4 benchmarks multiplying an unsigned 256-bit big-endian
// integer by 4 with [Element.MulBy4].
func BenchmarkElementMulBy4(b *testing.B) {
	eHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	e := mustElement(eHex)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.MulBy4()
	}
}

// BenchmarkElementMulBy8 benchmarks multiplying an unsigned 256-bit big-endian
// integer by 8 with [Element.MulBy8].
func BenchmarkElementMulBy8(b *testing.B) {
	eHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	e := mustElement(eHex)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.MulBy8()
	}
}

// BenchmarkFieldMulInt benchmarks multiplying an unsigned 256-bit big-endian
// integer by small integers with [Element.MulInt].
func BenchmarkElementMulInt(b *testing.B) {
	eHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	e := mustElement(eHex)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.MulInt(2)
	}
}

// BenchmarkElementMul benchmarks multiplying two unsigned 256-bit big-endian
// integers modulo the field prime with [Element].
func BenchmarkElementMul(b *testing.B) {
	a := mustElement("d2e670a19c6d753d1a6d8b20bd045df8a08fb162cf508956c31268c6d81ffdab")
	c := mustElement("16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var prod Element
		prod.Mul2(a, c)
	}
}

// BenchmarkElementSqrt benchmarks calculating the square root of an unsigned
// 256-bit big-endian integer modulo the field prime with [Element].
func BenchmarkElementSqrt(b *testing.B) {
	// The function is constant time so any value is fine.
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	e := mustElement(valHex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result Element
		_ = result.SquareRootVal(e)
	}
}

// BenchmarkElementSquare benchmarks squaring a 256-bit big-endian integer
// modulo the field prime with [Element].
func BenchmarkElementSquare(b *testing.B) {
	a := mustElement("16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sq Element
		sq.SquareVal(a)
	}
}

// BenchmarkElementInverse benchmarks calculating the multiplicative inverse of
// an unsigned 256-bit big-endian integer modulo the field prime with
// [Element].
func BenchmarkElementInverse(b *testing.B) {
	// The function is constant time so any value is fine.
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	e := mustElement(valHex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Inverse()
	}
}

// BenchmarkElementIsGtOrEqPrimeMinusOrder benchmarks determining whether a
// value is greater than or equal to the field prime minus the group order with
// [Element].
func BenchmarkElementIsGtOrEqPrimeMinusOrder(b *testing.B) {
	// The function is constant time so any value is fine.
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	e := mustElement(valHex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.IsGtOrEqPrimeMinusOrder()
	}
}
