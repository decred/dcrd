// Copyright (c) 2018-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/decred/dcrd/wire"
)

const (
	// noTreasury signifies the treasury agenda should be treated as though
	// it is inactive.  It is used to increase the readability of the
	// tests.
	noTreasury = false

	// withTreasury signifies the treasury agenda should be treated as
	// though it is active.  It is used to increase the readability of
	// the tests.
	withTreasury = true
)

var (
	// manyInputsBenchTx is a transaction that contains a lot of inputs which is
	// useful for benchmarking signature hash calculation.
	manyInputsBenchTx wire.MsgTx

	// A mock previous output script to use in the signing benchmark.
	prevOutScript = hexToBytes("a914f5916158e3e2c4551c1796708db8367207ed13bb87")
)

func init() {
	// tx 620f57c92cf05a7f7e7f7d28255d5f7089437bc48e34dcfebf7751d08b7fb8f5
	txHex, err := os.ReadFile("data/many_inputs_tx.hex")
	if err != nil {
		panic(fmt.Sprintf("unable to read benchmark tx file: %v", err))
	}

	txBytes := hexToBytes(string(txHex))
	err = manyInputsBenchTx.Deserialize(bytes.NewReader(txBytes))
	if err != nil {
		panic(err)
	}
}

// BenchmarkCalcSigHash benchmarks how long it takes to calculate the signature
// hashes for all inputs of a transaction with many inputs.
func BenchmarkCalcSigHash(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for j := 0; j < len(manyInputsBenchTx.TxIn); j++ {
			_, err := CalcSignatureHash(prevOutScript, SigHashAll,
				&manyInputsBenchTx, j, nil)
			if err != nil {
				b.Fatalf("failed to calc signature hash: %v", err)
			}
		}
	}
}

// genComplexScript returns a script comprised of half as many opcodes as the
// maximum allowed followed by as many max size data pushes fit without
// exceeding the max allowed script size.
func genComplexScript() ([]byte, error) {
	var scriptLen int
	builder := NewScriptBuilder()
	for i := 0; i < MaxOpsPerScript/2; i++ {
		builder.AddOp(OP_TRUE)
		scriptLen++
	}
	maxData := bytes.Repeat([]byte{0x02}, MaxScriptElementSize)
	for i := 0; i < (MaxScriptSize-scriptLen)/MaxScriptElementSize; i++ {
		builder.AddData(maxData)
	}
	return builder.Script()
}

// BenchmarkScriptParsing benchmarks how long it takes to parse a very large
// script.
func BenchmarkScriptParsing(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	const scriptVersion = 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tokenizer := MakeScriptTokenizer(scriptVersion, script)
		for tokenizer.Next() {
			_ = tokenizer.Opcode()
			_ = tokenizer.Data()
			_ = tokenizer.ByteIndex()
		}
		if err := tokenizer.Err(); err != nil {
			b.Fatalf("failed to parse script: %v", err)
		}
	}
}

// BenchmarkDisasmString benchmarks how long it takes to disassemble a very
// large script.
func BenchmarkDisasmString(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := DisasmString(script)
		if err != nil {
			b.Fatalf("failed to disasm script: %v", err)
		}
	}
}

// BenchmarkIsPayToScriptHash benchmarks how long it takes IsPayToScriptHash to
// analyze a very large script.
func BenchmarkIsPayToScriptHash(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsPayToScriptHash(script)
	}
}

// BenchmarkGetSigOpCount benchmarks how long it takes to count the signature
// operations of a very large script.
func BenchmarkGetSigOpCount(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetSigOpCount(script, noTreasury)
	}
}

// BenchmarkGetSigOpCountTreasury benchmarks how long it takes to count the
// signature operations of a very large script.
func BenchmarkGetSigOpCountTreasury(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetSigOpCount(script, withTreasury)
	}
}

// BenchmarkGetPreciseSigOpCount benchmarks how long it takes to count the
// signature operations of a very large script using the more precise counting
// method.
func BenchmarkGetPreciseSigOpCount(b *testing.B) {
	redeemScript, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	// Create a fake pay-to-script-hash to pass the necessary checks and create
	// the signature script accordingly by pushing the generated "redeem" script
	// as the final data push so the benchmark will cover the p2sh path.
	scriptHash := "0x0000000000000000000000000000000000000001"
	pkScript := mustParseShortFormV0("HASH160 DATA_20 " + scriptHash + " EQUAL")
	sigScript, err := NewScriptBuilder().AddData(redeemScript).Script()
	if err != nil {
		b.Fatalf("failed to create signature script: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetPreciseSigOpCount(sigScript, pkScript, noTreasury)
	}
}

// BenchmarkGetPreciseSigOpCountTreasury benchmarks how long it takes to count
// the signature operations of a very large script using the more precise
// counting method.
func BenchmarkGetPreciseSigOpCountTreasury(b *testing.B) {
	redeemScript, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	// Create a fake pay-to-script-hash to pass the necessary checks and create
	// the signature script accordingly by pushing the generated "redeem" script
	// as the final data push so the benchmark will cover the p2sh path.
	scriptHash := "0x0000000000000000000000000000000000000001"
	pkScript := mustParseShortFormV0("HASH160 DATA_20 " + scriptHash + " EQUAL")
	sigScript, err := NewScriptBuilder().AddDataUnchecked(redeemScript).Script()
	if err != nil {
		b.Fatalf("failed to create signature script: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetPreciseSigOpCount(sigScript, pkScript, withTreasury)
	}
}

// BenchmarkIsAnyKindOfScriptHash benchmarks how long it takes
// isAnyKindOfScriptHash to analyze operations of a very large script.
func BenchmarkIsAnyKindOfScriptHash(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	vm := Engine{flags: 0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vm.isAnyKindOfScriptHash(script)
	}
}

// BenchmarkIsAnyKindOfScriptHashTreasury benchmarks how long it takes
// isAnyKindOfScriptHash to analyze operations of a very large script with the
// treasury agenda enabled.
func BenchmarkIsAnyKindOfScriptHashTreasury(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	vm := Engine{flags: ScriptVerifyTreasury}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vm.isAnyKindOfScriptHash(script)
	}
}

// BenchmarkIsPushOnlyScript benchmarks how long it takes IsPushOnlyScript to
// analyze a very large script.
func BenchmarkIsPushOnlyScript(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsPushOnlyScript(script)
	}
}

// BenchmarkIsAltPubKeyHashScript benchmarks how long it takes to analyze a very
// large script to determine if it is a standard pay-to-alt-pubkey-hash script.
func BenchmarkIsAltPubKeyHashScript(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = isPubKeyHashAltScript(script)
	}
}

// BenchmarkContainsStakeOpCodes benchmarks how long it takes
// ContainsStakeOpCodes to analyze a very large script.
func BenchmarkContainsStakeOpCodes(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = ContainsStakeOpCodes(script, noTreasury)
		if err != nil {
			b.Fatalf("unexpected err: %v", err)
		}
	}
}

// BenchmarkContainsStakeOpCodesTreasury benchmarks how long it takes
// ContainsStakeOpCodes to analyze a very large script.
func BenchmarkContainsStakeOpCodesTreasury(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = ContainsStakeOpCodes(script, withTreasury)
		if err != nil {
			b.Fatalf("unexpected err: %v", err)
		}
	}
}

// BenchmarkIsUnspendable benchmarks how long it takes IsUnspendable to analyze
// a very large script.
func BenchmarkIsUnspendable(b *testing.B) {
	script, err := genComplexScript()
	if err != nil {
		b.Fatalf("failed to create benchmark script: %v", err)
	}
	const amount = 100000000
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsUnspendable(amount, script)
	}
}

// BenchmarkCheckSignatureEncoding benchmarks how long it takes to check the
// signature encoding for correctness of a typical DER-encoded ECDSA signature.
func BenchmarkCheckSignatureEncoding(b *testing.B) {
	sig := hexToBytes("3045022100cd496f2ab4fe124f977ffe3caa09f7576d8a34156b4e" +
		"55d326b4dffc0399a094022013500a0510b5094bff220c74656879b8ca0369d3da78" +
		"004004c970790862fc03")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := CheckSignatureEncoding(sig)
		if err != nil {
			b.Fatalf("unexpected err: %v", err)
		}
	}
}
