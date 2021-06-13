// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// TestHasCanonicalPush ensures the isCanonicalPush function works as expected.
func TestHasCanonicalPush(t *testing.T) {
	t.Parallel()

	const scriptVersion = 0
	for i := 0; i < 65535; i++ {
		builder := NewScriptBuilder()
		builder.AddInt64(int64(i))
		script, err := builder.Script()
		if err != nil {
			t.Errorf("Script: test #%d unexpected error: %v\n", i, err)
			continue
		}
		if !IsPushOnlyScript(script) {
			t.Errorf("IsPushOnlyScript: test #%d failed: %x\n", i, script)
			continue
		}
		tokenizer := MakeScriptTokenizer(scriptVersion, script)
		for tokenizer.Next() {
			if !isCanonicalPush(tokenizer.Opcode(), tokenizer.Data()) {
				t.Errorf("isCanonicalPush: test #%d failed: %x\n", i, script)
				break
			}
		}
	}
	for i := 0; i <= MaxScriptElementSize; i++ {
		builder := NewScriptBuilder()
		builder.AddData(bytes.Repeat([]byte{0x49}, i))
		script, err := builder.Script()
		if err != nil {
			t.Errorf("Script: test #%d unexpected error: %v\n", i, err)
			continue
		}
		if !IsPushOnlyScript(script) {
			t.Errorf("IsPushOnlyScript: test #%d failed: %x\n", i, script)
			continue
		}
		tokenizer := MakeScriptTokenizer(scriptVersion, script)
		for tokenizer.Next() {
			if !isCanonicalPush(tokenizer.Opcode(), tokenizer.Data()) {
				t.Errorf("isCanonicalPush: test #%d failed: %x\n", i, script)
				break
			}
		}
	}
}

// TestGetSigOpCount tests that the GetSigOpCount function behaves as expected.
func TestGetSigOpCount(t *testing.T) {
	// This should correspond to MaxPubKeysPerMultiSig. It's intentionally
	// not referred to here so that any changes to MaxPubKeysPerMultisig
	// are flagged during tests.
	maxMultiSigOps := 20

	// Build out a script that tests every opcode not on the "special list"
	// below. These will be tested separately.
	specialOpCodes := map[byte]struct{}{
		OP_CHECKSIG:            {},
		OP_CHECKSIGVERIFY:      {},
		OP_CHECKSIGALT:         {},
		OP_CHECKSIGALTVERIFY:   {},
		OP_CHECKMULTISIG:       {},
		OP_CHECKMULTISIGVERIFY: {},
		OP_TADD:                {},
		OP_TGEN:                {},
		OP_TSPEND:              {},
	}
	otherOpCodesScript := ""
	for i := 0; i <= 255; i++ {
		if _, ok := specialOpCodes[byte(i)]; ok {
			continue
		}
		otherOpCodesScript = fmt.Sprintf("%s 0x%.2x", otherOpCodesScript, i)
	}

	testCases := []struct {
		name              string
		script            string
		wantCount         int
		wantTreasuryCount int
	}{{
		name:              "all opcodes that dont count for a sigop",
		script:            otherOpCodesScript,
		wantCount:         0,
		wantTreasuryCount: 0,
	}, {
		name:              "all opcodes that count for a sigop",
		script:            "CHECKSIG CHECKSIGVERIFY CHECKSIGALT CHECKSIGALTVERIFY",
		wantCount:         4,
		wantTreasuryCount: 4,
	}, {
		name:              "multisig with < MaxPubKeysPerMultiSig",
		script:            "2 DATA_33 0x00{33} 0x00{33} 0x00{33} 3 OP_CHECKMULTISIG",
		wantCount:         maxMultiSigOps,
		wantTreasuryCount: maxMultiSigOps,
	}, {
		name:              "multisigverify with less than MaxPubKeysPerMultiSig",
		script:            "2 DATA_33 0x00{33} 0x00{33} 0x00{33} 3 OP_CHECKMULTISIGVERIFY",
		wantCount:         maxMultiSigOps,
		wantTreasuryCount: maxMultiSigOps,
	}, {
		name:              "valid p2pkh output",
		script:            "DUP HASH160 DATA_20 0x00{20} EQUALVERIFY CHECKSIG",
		wantCount:         1,
		wantTreasuryCount: 1,
	}, {
		name:              "valid p2sh output",
		script:            "HASH160 DATA_20 0x00{20} EQUAL",
		wantCount:         0,
		wantTreasuryCount: 0,
	}, {
		name:              "valid stake change output",
		script:            "SSTXCHANGE DUP HASH160 DATA_20 0x00{20} EQUALVERIFY CHECKSIG",
		wantCount:         1,
		wantTreasuryCount: 1,
	}, {
		name:              "valid tspend output",
		script:            "TGEN DUP HASH160 DATA_20 0x00{20} EQUALVERIFY CHECKSIG",
		wantCount:         1,
		wantTreasuryCount: 1,
	}, {
		name:              "valid tspend input",
		script:            "DATA_64 0x00{64} TSPEND",
		wantCount:         0,
		wantTreasuryCount: 1,
	}, {
		name:              "valid tadd output",
		script:            "TADD",
		wantCount:         0,
		wantTreasuryCount: 0,
	}}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			script := mustParseShortForm(tc.script)
			gotCount := GetSigOpCount(script, false)
			if gotCount != tc.wantCount {
				t.Fatalf("unexpected sigOpCount with treasury=false. "+
					"want=%d got=%d", tc.wantCount, gotCount)
			}

			gotTreasuryCount := GetSigOpCount(script, true)
			if gotTreasuryCount != tc.wantTreasuryCount {
				t.Fatalf("unexpected sigOpCount with treasury=true. "+
					"want=%d got=%d", tc.wantTreasuryCount,
					gotTreasuryCount)
			}
		})
	}
}

// TestGetPreciseSigOps ensures the more precise signature operation counting
// mechanism which includes signatures in P2SH scripts works as expected.
func TestGetPreciseSigOps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		scriptSig []byte
		nSigOps   int
	}{
		{
			name:      "scriptSig doesn't parse",
			scriptSig: mustParseShortForm("PUSHDATA1 0x02"),
		},
		{
			name:      "scriptSig isn't push only",
			scriptSig: mustParseShortForm("1 DUP"),
			nSigOps:   0,
		},
		{
			name:      "scriptSig length 0",
			scriptSig: nil,
			nSigOps:   0,
		},
		{
			name: "No script at the end",
			// No script at end but still push only.
			scriptSig: mustParseShortForm("1 1"),
			nSigOps:   0,
		},
		{
			name:      "pushed script doesn't parse",
			scriptSig: mustParseShortForm("DATA_2 PUSHDATA1 0x02"),
		},
	}

	// The signature in the p2sh script is nonsensical for the tests since
	// this script will never be executed.  What matters is that it matches
	// the right pattern. Without treasury enabled.
	pkScript := mustParseShortForm("HASH160 DATA_20 0x433ec2ac1ffa1b7b7d0" +
		"27f564529c57197f9ae88 EQUAL")
	for _, test := range tests {
		count := GetPreciseSigOpCount(test.scriptSig, pkScript,
			noTreasury)
		if count != test.nSigOps {
			t.Errorf("%s: expected count of %d, got %d", test.name,
				test.nSigOps, count)
		}
	}

	// The signature in the p2sh script is nonsensical for the tests since
	// this script will never be executed.  What matters is that it matches
	// the right pattern. With treasury enabled.
	pkScript = mustParseShortForm("HASH160 DATA_20 0x433ec2ac1ffa1b7b7d0" +
		"27f564529c57197f9ae88 EQUAL")
	for _, test := range tests {
		count := GetPreciseSigOpCount(test.scriptSig, pkScript,
			withTreasury)
		if count != test.nSigOps {
			t.Errorf("%s: expected count of %d, got %d", test.name,
				test.nSigOps, count)
		}
	}
}

// TestRemoveOpcodeByData ensures that removing data carrying opcodes based on
// the data they contain works as expected.
func TestRemoveOpcodeByData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		before []byte
		remove []byte
		err    error
		after  []byte
	}{
		{
			name:   "nothing to do",
			before: mustParseShortForm("NOP"),
			remove: []byte{1, 2, 3, 4},
			after:  mustParseShortForm("NOP"),
		},
		{
			name:   "simple case",
			before: mustParseShortForm("DATA_4 0x01020304"),
			remove: []byte{1, 2, 3, 4},
			after:  nil,
		},
		{
			name:   "simple case (miss)",
			before: mustParseShortForm("DATA_4 0x01020304"),
			remove: []byte{1, 2, 3, 5},
			after:  mustParseShortForm("DATA_4 0x01020304"),
		},
		{
			name: "stakesubmission simple case p2pkh",
			before: mustParseShortForm("SSTX DUP HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUALVERIFY CHECKSIG"),
			remove: []byte{1, 2, 3, 4},
			after:  mustParseShortForm("SSTX DUP HASH160 EQUALVERIFY CHECKSIG"),
		},
		{
			name: "stakesubmission simple case p2pkh (miss)",
			before: mustParseShortForm("SSTX DUP HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUALVERIFY CHECKSIG"),
			remove: []byte{1, 2, 3, 4, 5},
			after: mustParseShortForm("SSTX DUP HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUALVERIFY CHECKSIG"),
		},
		{
			name: "stakesubmission simple case p2sh",
			before: mustParseShortForm("SSTX HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUAL"),
			remove: []byte{1, 2, 3, 4},
			after:  mustParseShortForm("SSTX HASH160 EQUAL"),
		},
		{
			name: "stakesubmission simple case p2sh (miss)",
			before: mustParseShortForm("SSTX HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUAL"),
			remove: []byte{1, 2, 3, 4, 5},
			after: mustParseShortForm("SSTX HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUAL"),
		},
		{
			name: "stakegen simple case p2pkh",
			before: mustParseShortForm("SSGEN DUP HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUALVERIFY CHECKSIG"),
			remove: []byte{1, 2, 3, 4},
			after:  mustParseShortForm("SSGEN DUP HASH160 EQUALVERIFY CHECKSIG"),
		},
		{
			name: "stakegen simple case p2pkh (miss)",
			before: mustParseShortForm("SSGEN DUP HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUALVERIFY CHECKSIG"),
			remove: []byte{1, 2, 3, 4, 5},
			after: mustParseShortForm("SSGEN DUP HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUALVERIFY CHECKSIG"),
		},
		{
			name: "stakegen simple case p2sh",
			before: mustParseShortForm("SSGEN HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUAL"),
			remove: []byte{1, 2, 3, 4},
			after:  mustParseShortForm("SSGEN HASH160 EQUAL"),
		},
		{
			name: "stakegen simple case p2sh (miss)",
			before: mustParseShortForm("SSGEN HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUAL"),
			remove: []byte{1, 2, 3, 4, 5},
			after: mustParseShortForm("SSGEN HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUAL"),
		},
		{
			name: "stakerevoke simple case p2pkh",
			before: mustParseShortForm("SSRTX DUP HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUALVERIFY CHECKSIG"),
			remove: []byte{1, 2, 3, 4},
			after:  []byte{OP_SSRTX, OP_DUP, OP_HASH160, OP_EQUALVERIFY, OP_CHECKSIG},
		},
		{
			name: "stakerevoke simple case p2pkh (miss)",
			before: mustParseShortForm("SSRTX DUP HASH160 DATA_20 0x00{20} " +
				"EQUALVERIFY CHECKSIG"),
			remove: bytes.Repeat([]byte{0}, 21),
			after: mustParseShortForm("SSRTX DUP HASH160 DATA_20 0x00{20} " +
				"EQUALVERIFY CHECKSIG"),
		},
		{
			name: "stakerevoke simple case p2sh",
			before: mustParseShortForm("SSRTX HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUAL"),
			remove: []byte{1, 2, 3, 4},
			after:  mustParseShortForm("SSRTX HASH160 EQUAL"),
		},
		{
			name: "stakerevoke simple case p2sh (miss)",
			before: mustParseShortForm("SSRTX HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUAL"),
			remove: []byte{1, 2, 3, 4, 5},
			after: mustParseShortForm("SSRTX HASH160 DATA_20 0x00{16} " +
				"0x01020304 EQUAL"),
		},
		{
			// padded to keep it canonical.
			name:   "simple case (pushdata1)",
			before: mustParseShortForm("PUSHDATA1 0x4c 0x00{72} 0x01020304"),
			remove: []byte{1, 2, 3, 4},
			after:  nil,
		},
		{
			name:   "simple case (pushdata1 miss)",
			before: mustParseShortForm("PUSHDATA1 0x4c 0x00{72} 0x01020304"),
			remove: []byte{1, 2, 3, 5},
			after:  mustParseShortForm("PUSHDATA1 0x4c 0x00{72} 0x01020304"),
		},
		{
			name:   "simple case (pushdata1 miss noncanonical)",
			before: mustParseShortForm("PUSHDATA1 0x04 0x01020304"),
			remove: []byte{1, 2, 3, 4},
			after:  mustParseShortForm("PUSHDATA1 0x04 0x01020304"),
		},
		{
			name:   "simple case (pushdata2)",
			before: mustParseShortForm("PUSHDATA2 0x0001 0x00{252} 0x01020304"),
			remove: []byte{1, 2, 3, 4},
			after:  nil,
		},
		{
			name:   "simple case (pushdata2 miss)",
			before: mustParseShortForm("PUSHDATA2 0x0001 0x00{252} 0x01020304"),
			remove: []byte{1, 2, 3, 4, 5},
			after:  mustParseShortForm("PUSHDATA2 0x0001 0x00{252} 0x01020304"),
		},
		{
			name:   "simple case (pushdata2 miss noncanonical)",
			before: mustParseShortForm("PUSHDATA2 0x0400 0x01020304"),
			remove: []byte{1, 2, 3, 4},
			after:  mustParseShortForm("PUSHDATA2 0x0400 0x01020304"),
		},
		{
			// This is padded to make the push canonical.
			name: "simple case (pushdata4)",
			before: mustParseShortForm("PUSHDATA4 0x00000100 0x00{65532} " +
				"0x01020304"),
			remove: []byte{1, 2, 3, 4},
			after:  nil,
		},
		{
			name:   "simple case (pushdata4 miss noncanonical)",
			before: mustParseShortForm("PUSHDATA4 0x04000000 0x01020304"),
			remove: []byte{1, 2, 3, 4},
			after:  mustParseShortForm("PUSHDATA4 0x04000000 0x01020304"),
		},
		{
			// This is padded to make the push canonical.
			name: "simple case (pushdata4 miss)",
			before: mustParseShortForm("PUSHDATA4 0x00000100 0x00{65532} " +
				"0x01020304"),
			remove: []byte{1, 2, 3, 4, 5},
			after: mustParseShortForm("PUSHDATA4 0x00000100 0x00{65532} " +
				"0x01020304"),
		},
		{
			name:   "invalid opcode",
			before: []byte{OP_UNKNOWN240},
			remove: []byte{1, 2, 3, 4},
			after:  []byte{OP_UNKNOWN240},
		},
		{
			name:   "invalid length (instruction)",
			before: []byte{OP_PUSHDATA1},
			remove: []byte{1, 2, 3, 4},
			err:    ErrMalformedPush,
		},
		{
			name:   "invalid length (data)",
			before: []byte{OP_PUSHDATA1, 255, 254},
			remove: []byte{1, 2, 3, 4},
			err:    ErrMalformedPush,
		},
	}

	// tstRemoveOpcodeByData is a convenience function to ensure the provided
	// script parses before attempting to remove the passed data.
	const scriptVersion = 0
	tstRemoveOpcodeByData := func(script []byte, data []byte) ([]byte, error) {
		if err := checkScriptParses(scriptVersion, script); err != nil {
			return nil, err
		}

		return removeOpcodeByData(script, data), nil
	}

	for _, test := range tests {
		result, err := tstRemoveOpcodeByData(test.before, test.remove)
		if !errors.Is(err, test.err) {
			t.Errorf("%s: unexpected error - got %v, want %v", test.name, err,
				test.err)
			continue
		}

		if !bytes.Equal(test.after, result) {
			t.Errorf("%s: value does not equal expected: exp: %q"+
				" got: %q", test.name, test.after, result)
		}
	}
}

// TestIsPayToScriptHash ensures the IsPayToScriptHash function returns the
// expected results for all the scripts in scriptClassTests.
func TestIsPayToScriptHash(t *testing.T) {
	t.Parallel()

	for _, test := range scriptClassTests {
		script := mustParseShortForm(test.script)
		shouldBe := (test.class == ScriptHashTy)
		p2sh := IsPayToScriptHash(script)
		if p2sh != shouldBe {
			t.Errorf("%s: expected p2sh %v, got %v", test.name,
				shouldBe, p2sh)
		}
	}
}

// TestIsAnyKindOfScriptHash ensures the isAnyKindOfScriptHash function returns
// the expected results for all the scripts in scriptClassTests.
func TestIsAnyKindOfScriptHash(t *testing.T) {
	t.Parallel()

	for _, test := range scriptClassTests {
		script := mustParseShortForm(test.script)
		want := (test.class == ScriptHashTy || test.subClass == ScriptHashTy)

		// Without treasury enabled.
		vm := Engine{flags: 0}
		p2sh := vm.isAnyKindOfScriptHash(script)
		if p2sh != want {
			t.Errorf("%s: expected p2sh %v, got %v", test.name,
				want, p2sh)
		}

		// With treasury enabled.
		vm = Engine{flags: ScriptVerifyTreasury}
		p2sh = vm.isAnyKindOfScriptHash(script)
		if p2sh != want {
			t.Errorf("%s: expected p2sh %v, got %v", test.name,
				want, p2sh)
		}
	}
}

// TestHasCanonicalPushes ensures the isCanonicalPush function properly
// determines what is considered a canonical push for the purposes of
// removeOpcodeByData.
func TestHasCanonicalPushes(t *testing.T) {
	t.Parallel()

	const scriptVersion = 0
	tests := []struct {
		name     string
		script   string
		expected bool
	}{
		{
			name: "does not parse",
			script: "0x046708afdb0fe5548271967f1a67130b7105cd6a82" +
				"8e03909a67962e0ea1f61d",
			expected: false,
		},
		{
			name:     "non-canonical push",
			script:   "PUSHDATA1 0x04 0x01020304",
			expected: false,
		},
	}

	for _, test := range tests {
		script := mustParseShortForm(test.script)
		if err := checkScriptParses(scriptVersion, script); err != nil {
			if test.expected {
				t.Errorf("%q: script parse failed: %v", test.name, err)
			}
			continue
		}
		tokenizer := MakeScriptTokenizer(scriptVersion, script)
		for tokenizer.Next() {
			result := isCanonicalPush(tokenizer.Opcode(), tokenizer.Data())
			if result != test.expected {
				t.Errorf("%q: isCanonicalPush wrong result\ngot: %v\nwant: %v",
					test.name, result, test.expected)
				break
			}
		}
	}
}

// TestIsPushOnlyScript ensures the IsPushOnlyScript function returns the
// expected results.
func TestIsPushOnlyScript(t *testing.T) {
	t.Parallel()

	test := struct {
		name     string
		script   []byte
		expected bool
	}{
		name: "does not parse",
		script: mustParseShortForm("0x046708afdb0fe5548271967f1a67130" +
			"b7105cd6a828e03909a67962e0ea1f61d"),
		expected: false,
	}

	if IsPushOnlyScript(test.script) != test.expected {
		t.Errorf("IsPushOnlyScript (%s) wrong result\ngot: %v\nwant: "+
			"%v", test.name, true, test.expected)
	}
}

// TestIsUnspendable ensures the IsUnspendable function returns the expected
// results.
func TestIsUnspendable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		amount   int64
		pkScript []byte
		expected bool
	}{
		{
			// Unspendable
			amount:   100,
			pkScript: []byte{0x6a, 0x04, 0x74, 0x65, 0x73, 0x74},
			expected: true,
		},
		{
			// Unspendable
			amount: 0,
			pkScript: []byte{0x76, 0xa9, 0x14, 0x29, 0x95, 0xa0,
				0xfe, 0x68, 0x43, 0xfa, 0x9b, 0x95, 0x45,
				0x97, 0xf0, 0xdc, 0xa7, 0xa4, 0x4d, 0xf6,
				0xfa, 0x0b, 0x5c, 0x88, 0xac},
			expected: true,
		},
		{
			// Spendable
			amount: 100,
			pkScript: []byte{0x76, 0xa9, 0x14, 0x29, 0x95, 0xa0,
				0xfe, 0x68, 0x43, 0xfa, 0x9b, 0x95, 0x45,
				0x97, 0xf0, 0xdc, 0xa7, 0xa4, 0x4d, 0xf6,
				0xfa, 0x0b, 0x5c, 0x88, 0xac},
			expected: false,
		},
	}

	for i, test := range tests {
		res := IsUnspendable(test.amount, test.pkScript)
		if res != test.expected {
			t.Errorf("IsUnspendable #%d failed: got %v want %v", i,
				res, test.expected)
			continue
		}
	}
}

// TestGenerateSSGenBlockRef ensures the block reference script for use in stake
// vote transactions is generated correctly for various block hashes and
// heights.
func TestGenerateSSGenBlockRef(t *testing.T) {
	var tests = []struct {
		blockHash string
		height    uint32
		expected  []byte
	}{{
		"0000000000004740ad140c86753f9295e09f9cc81b1bb75d7f5552aeeedb7012",
		1000,
		hexToBytes("6a241270dbeeae52557f5db71b1bc89c9fe095923f75860c14ad40470" +
			"00000000000e8030000"),
	}, {
		"000000000000000033eafc268a67c8d1f02343d7a96cf3fe2a4915ef779b52f9",
		290000,
		hexToBytes("6a24f9529b77ef15492afef36ca9d74323f0d1c8678a26fcea3300000" +
			"00000000000d06c0400"),
	}}

	for _, test := range tests {
		h, err := chainhash.NewHashFromStr(test.blockHash)
		if err != nil {
			t.Errorf("unexpected err: %v", err)
			continue
		}
		s, err := GenerateSSGenBlockRef(*h, test.height)
		if err != nil {
			t.Errorf("unexpected err: %v", err)
			continue
		}
		if !bytes.Equal(s, test.expected) {
			t.Errorf("unexpected script -- got %x, want %x", s, test.expected)
			continue
		}
	}
}

// TestGenerateSSGenVotes ensures the expected vote script for use in stake
// vote transactions is generated correctly for various vote bits.
func TestGenerateSSGenVotes(t *testing.T) {
	var tests = []struct {
		votebits uint16
		expected []byte
	}{
		{65535, hexToBytes("6a02ffff")},
		{256, hexToBytes("6a020001")},
		{127, hexToBytes("6a027f00")},
		{0, hexToBytes("6a020000")},
	}
	for _, test := range tests {
		s, err := GenerateSSGenVotes(test.votebits)
		if err != nil {
			t.Errorf("unexpected err: %v", err)
			continue
		}
		if !bytes.Equal(s, test.expected) {
			t.Errorf("unexpected script -- got %x, want %x", s, test.expected)
			continue
		}
	}
}
