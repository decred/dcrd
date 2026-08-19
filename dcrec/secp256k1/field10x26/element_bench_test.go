// Copyright (c) 2020-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package field10x26

import (
	"math/big"
	"testing"
)

// BenchmarkElementNormalize benchmarks how long it takes the internal field
// to perform normalization (which includes modular reduction) with [Element].
func BenchmarkElementNormalize(b *testing.B) {
	// The function is constant time so any value is fine.
	e := &Element{n: [10]uint32{
		0x000148f6, 0x03ffffc0, 0x03ffffff, 0x03ffffff, 0x03ffffff,
		0x03ffffff, 0x03ffffff, 0x03ffffff, 0x03ffffff, 0x00000007,
	}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Normalize()
	}
}

// BenchmarkBigIntNegateModP benchmarks calculating the additive inverse of an
// unsigned 256-bit big-endian integer modulo the field prime with stdlib big
// integers.
func BenchmarkBigIntNegateModP(b *testing.B) {
	v1Hex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	v1 := fromHex(v1Hex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := new(big.Int).Neg(v1)
		result.Mod(result, curveParams.P)
	}
}

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
		_ = result.NegateVal(e, 1)
	}
}

// BenchmarkBigIntAddModP benchmarks adding two unsigned 256-bit big-endian
// integers modulo the field prime with stdlib big integers.
func BenchmarkBigIntAddModP(b *testing.B) {
	v1Hex := "d2e670a19c6d753d1a6d8b20bd045df8a08fb162cf508956c31268c6d81ffdab"
	v2Hex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	v1 := fromHex(v1Hex)
	v2 := fromHex(v2Hex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := new(big.Int).Add(v1, v2)
		result.Mod(result, curveParams.P)
	}
}

// BenchmarkElementAdd benchmarks adding two unsigned 256-bit big-endian
// integers modulo the field prime with [Element].
func BenchmarkElementAdd(b *testing.B) {
	// The function is constant time so any values are fine.
	f1Hex := "d2e670a19c6d753d1a6d8b20bd045df8a08fb162cf508956c31268c6d81ffdab"
	f2Hex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	f1 := mustElement(f1Hex)
	f2 := mustElement(f2Hex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sum Element
		sum.Add2(f1, f2)
	}
}

// BenchmarkElementMulBy2 benchmarks multiplying an unsigned 256-bit big-endian
// integer by 2 with [Element.MulBy2].
func BenchmarkElementMulBy2(b *testing.B) {
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	e := mustElement(valHex)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.MulBy2()
	}
}

// BenchmarkElementMulBy3 benchmarks multiplying an unsigned 256-bit big-endian
// integer by 3 with [Element.MulBy3].
func BenchmarkElementMulBy3(b *testing.B) {
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	e := mustElement(valHex)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.MulBy3()
	}
}

// BenchmarkElementMulBy4 benchmarks multiplying an unsigned 256-bit big-endian
// integer by 4 with [Element.MulBy4].
func BenchmarkElementMulBy4(b *testing.B) {
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	e := mustElement(valHex)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.MulBy4()
	}
}

// BenchmarkElementMulBy8 benchmarks multiplying an unsigned 256-bit big-endian
// integer by 8 with [Element.MulBy8].
func BenchmarkElementMulBy8(b *testing.B) {
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	e := mustElement(valHex)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.MulBy8()
	}
}

// BenchmarkElementMulInt benchmarks multiplying an unsigned 256-bit big-endian
// integer by small integers with [Element.MulInt].
func BenchmarkElementMulInt(b *testing.B) {
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	e := mustElement(valHex)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.MulInt(2)
	}
}

// BenchmarkBigIntMulModP benchmarks multiplying two unsigned 256-bit big-endian
// integers modulo the field prime with stdlib big integers.
func BenchmarkBigIntMulModP(b *testing.B) {
	v1Hex := "d2e670a19c6d753d1a6d8b20bd045df8a08fb162cf508956c31268c6d81ffdab"
	v2Hex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	v1 := fromHex(v1Hex)
	v2 := fromHex(v2Hex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := new(big.Int).Mul(v1, v2)
		result.Mod(result, curveParams.P)
	}
}

// BenchmarkElementMul benchmarks multiplying two unsigned 256-bit big-endian
// integers modulo the field prime with [Element].
func BenchmarkElementMul(b *testing.B) {
	// The function is constant time so any values are fine.
	f1Hex := "d2e670a19c6d753d1a6d8b20bd045df8a08fb162cf508956c31268c6d81ffdab"
	f2Hex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	f1 := mustElement(f1Hex)
	f2 := mustElement(f2Hex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var prod Element
		prod.Mul2(f1, f2)
	}
}

// BenchmarkBigIntSqrtModP benchmarks calculating the square root of an unsigned
// 256-bit big-endian integer modulo the field prime with stdlib big integers.
func BenchmarkBigIntSqrtModP(b *testing.B) {
	v1Hex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	v1 := fromHex(v1Hex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = new(big.Int).ModSqrt(v1, curveParams.P)
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

// BenchmarkBigIntSquareModP benchmarks squaring an unsigned 256-bit big-endian
// integer modulo the field prime with stdlib big integers.
func BenchmarkBigIntSquareModP(b *testing.B) {
	v1Hex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	v1 := fromHex(v1Hex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := new(big.Int).Mul(v1, v1)
		result.Mod(result, curveParams.P)
	}
}

// BenchmarkElementSquare benchmarks squaring a 256-bit big-endian integer
// modulo the field prime with [Element].
func BenchmarkElementSquare(b *testing.B) {
	// The function is constant time so any values are fine.
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	e := mustElement(valHex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sq Element
		sq.SquareVal(e)
	}
}

// BenchmarkBigIntInverseModP benchmarks calculating the multiplicative inverse
// of an unsigned 256-bit big-endian integer modulo the field prime with stdlib
// big integers.
func BenchmarkBigIntInverseModP(b *testing.B) {
	v1Hex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	v1 := fromHex(v1Hex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = new(big.Int).ModInverse(v1, curveParams.P)
	}
}

// BenchmarkElementInverse calculating the multiplicative inverse of an unsigned
// 256-bit big-endian integer modulo the field prime with [Element].
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

// BenchmarkBigIntIsGtOrEqPrimeMinusOrder benchmarks determining whether a value
// is greater than or equal to the field prime minus the group order with stdlib
// big integers.
func BenchmarkBigIntIsGtOrEqPrimeMinusOrder(b *testing.B) {
	// Same value used in field val version.
	v1Hex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	v1 := fromHex(v1Hex)
	bigPMinusN := new(big.Int).Sub(curveParams.P, curveParams.N)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// In practice, the internal value to compare would have to be converted
		// to a big integer from bytes, so it's a fair comparison to allocate a
		// new big int here and set all bytes.
		_ = new(big.Int).SetBytes(v1.Bytes()).Cmp(bigPMinusN) >= 0
	}
}

// BenchmarkElementIsGtOrEqPrimeMinusOrder benchmarks determining whether a value
// is greater than or equal to the field prime minus the group order with the
// specialized type.
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
