// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stdscript

import (
	"bytes"
	"testing"

	"github.com/decred/dcrd/txscript/v4"
)

// complexScriptV0 is a version 0 script comprised of half as many opcodes as
// the maximum allowed followed by as many max size data pushes fit without
// exceeding the max allowed script size.
var complexScriptV0 = func() []byte {
	const (
		maxScriptSize        = txscript.MaxScriptSize
		maxScriptElementSize = txscript.MaxScriptElementSize
	)
	var scriptLen int
	builder := txscript.NewScriptBuilder()
	for i := 0; i < txscript.MaxOpsPerScript/2; i++ {
		builder.AddOp(txscript.OP_TRUE)
		scriptLen++
	}
	maxData := bytes.Repeat([]byte{0x02}, maxScriptElementSize)
	for i := 0; i < (maxScriptSize-scriptLen)/maxScriptElementSize; i++ {
		builder.AddData(maxData)
	}
	script, err := builder.Script()
	if err != nil {
		panic(err)
	}
	return script
}()

// makeBenchmarks constructs a slice of tests to use in the benchmarks as
// follows:
// - Start with a version 0 complex non standard script
// - Add all tests for which the provided filter function returns true
func makeBenchmarks(filterFn func(test scriptTest) bool) []scriptTest {
	benches := make([]scriptTest, 0, 5)
	benches = append(benches, scriptTest{
		name:    "v0 complex non standard",
		version: 0,
		script:  complexScriptV0,
	})
	for _, test := range scriptV0Tests {
		if filterFn(test) {
			benches = append(benches, test)
		}
	}
	return benches
}

// benchIsX is a convenience function that runs benchmarks for the entries that
// match the provided filter function using the given script type determination
// function and ensures the result matches the expected one.
func benchIsX(b *testing.B, filterFn func(test scriptTest) bool, isXFn func(scriptVersion uint16, script []byte) bool) {
	b.Helper()

	benches := makeBenchmarks(filterFn)
	for _, bench := range benches {
		want := filterFn(bench)
		b.Run(bench.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				got := isXFn(bench.version, bench.script)
				if got != want {
					b.Fatalf("%q: unexpected result -- got %v, want %v",
						bench.name, got, want)
				}
			}
		})
	}
}

// BenchmarkIsPubKeyScript benchmarks the performance of analyzing various
// public key scripts to determine if they are p2pk-ecdsa-secp256k1 scripts.
func BenchmarkIsPubKeyScript(b *testing.B) {
	filterFn := func(test scriptTest) bool {
		return test.wantType == STPubKeyEcdsaSecp256k1
	}
	benchIsX(b, filterFn, IsPubKeyScript)
}

// BenchmarkIsPubKeyEd25519Script benchmarks the performance of analyzing
// various public key scripts to determine if they are p2pkh-ed25519 scripts.
func BenchmarkIsPubKeyEd25519Script(b *testing.B) {
	filterFn := func(test scriptTest) bool {
		return test.wantType == STPubKeyEd25519
	}
	benchIsX(b, filterFn, IsPubKeyEd25519Script)
}

// BenchmarkIsPubKeySchnorrSecp256k1Script benchmarks the performance of
// analyzing various public key scripts to determine if they are
// p2pkh-schnorr-secp256k1 scripts.
func BenchmarkIsPubKeySchnorrSecp256k1Script(b *testing.B) {
	filterFn := func(test scriptTest) bool {
		return test.wantType == STPubKeySchnorrSecp256k1
	}
	benchIsX(b, filterFn, IsPubKeySchnorrSecp256k1Script)
}

// BenchmarkIsPubKeyHashScript benchmarks the performance of analyzing various
// public key scripts to determine if they are p2pkh-ecdsa-secp256k1 scripts.
func BenchmarkIsPubKeyHashScript(b *testing.B) {
	filterFn := func(test scriptTest) bool {
		return test.wantType == STPubKeyHashEcdsaSecp256k1
	}
	benchIsX(b, filterFn, IsPubKeyHashScript)
}

// BenchmarkIsPubKeyHashEd25519Script benchmarks the performance of analyzing
// various public key scripts to determine if they are p2pkh-ed25519 scripts.
func BenchmarkIsPubKeyHashEd25519Script(b *testing.B) {
	filterFn := func(test scriptTest) bool {
		return test.wantType == STPubKeyHashEd25519
	}
	benchIsX(b, filterFn, IsPubKeyHashEd25519Script)
}

// BenchmarkIsPubKeyHashSchnorrSecp256k1Script benchmarks the performance of
// analyzing various public key scripts to determine if they are
// p2pkh-schnorr-secp256k1 scripts.
func BenchmarkIsPubKeyHashSchnorrSecp256k1Script(b *testing.B) {
	filterFn := func(test scriptTest) bool {
		return test.wantType == STPubKeyHashSchnorrSecp256k1
	}
	benchIsX(b, filterFn, IsPubKeyHashSchnorrSecp256k1Script)
}

// BenchmarkIsScriptHashScript benchmarks the performance of analyzing various
// public key scripts to determine if they are p2sh scripts.
func BenchmarkIsScriptHashScript(b *testing.B) {
	filterFn := func(test scriptTest) bool {
		return test.wantType == STScriptHash
	}
	benchIsX(b, filterFn, IsScriptHashScript)
}
