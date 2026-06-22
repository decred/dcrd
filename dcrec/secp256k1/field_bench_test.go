// Copyright (c) 2020-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"math/big"
	"testing"
)

// BenchmarkFieldNormalize benchmarks how long it takes the internal field
// to perform normalization (which includes modular reduction).
func BenchmarkFieldNormalize(b *testing.B) {
	// The function is constant time so any value is fine.
	f := &FieldVal{n: [10]uint32{
		0x000148f6, 0x03ffffc0, 0x03ffffff, 0x03ffffff, 0x03ffffff,
		0x03ffffff, 0x03ffffff, 0x03ffffff, 0x03ffffff, 0x00000007,
	}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Normalize()
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

// BenchmarkFieldNegate benchmarks calculating the additive inverse of an
// unsigned 256-bit big-endian integer modulo the field prime with [FieldVal].
func BenchmarkFieldNegate(b *testing.B) {
	// The function is constant time so any value is fine.
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	f := new(FieldVal).SetHex(valHex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result FieldVal
		_ = result.NegateVal(f, 1)
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
		result.Mod(result, curveParams.N)
	}
}

// BenchmarkFieldAdd benchmarks adding two unsigned 256-bit big-endian integers
// modulo the field prime with [FieldVal].
func BenchmarkFieldAdd(b *testing.B) {
	// The function is constant time so any values are fine.
	f1Hex := "d2e670a19c6d753d1a6d8b20bd045df8a08fb162cf508956c31268c6d81ffdab"
	f2Hex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	f1 := new(FieldVal).SetHex(f1Hex)
	f2 := new(FieldVal).SetHex(f2Hex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sum FieldVal
		sum.Add2(f1, f2)
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

// BenchmarkFieldSqrt benchmarks calculating the square root of an unsigned
// 256-bit big-endian integer modulo the field prime with the specialized type.
func BenchmarkFieldSqrt(b *testing.B) {
	// The function is constant time so any value is fine.
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	f := new(FieldVal).SetHex(valHex).Normalize()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result FieldVal
		_ = result.SquareRootVal(f)
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

// BenchmarkFieldInverse calculating the multiplicative inverse of an unsigned
// 256-bit big-endian integer modulo the field prime with the specialized type.
func BenchmarkFieldInverse(b *testing.B) {
	// The function is constant time so any value is fine.
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	f := new(FieldVal).SetHex(valHex).Normalize()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Inverse()
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

// BenchmarkFieldIsGtOrEqPrimeMinusOrder benchmarks determining whether a value
// is greater than or equal to the field prime minus the group order with the
// specialized type.
func BenchmarkFieldIsGtOrEqPrimeMinusOrder(b *testing.B) {
	// The function is constant time so any value is fine.
	valHex := "16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca"
	f := new(FieldVal).SetHex(valHex).Normalize()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = f.IsGtOrEqPrimeMinusOrder()
	}
}
