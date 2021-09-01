// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// scriptTestName returns a descriptive test name for the given reference script
// test data.
func scriptTestName(test []string) (string, error) {
	// The test must consist of at least a signature script, public key script,
	// verification flags, and expected error.  Finally, it may optionally
	// contain a comment.
	if len(test) < 4 || len(test) > 5 {
		return "", fmt.Errorf("invalid test length %d", len(test))
	}

	// Use the comment for the test name if one is specified, otherwise,
	// construct the name based on the signature script, public key script,
	// and flags.
	var name string
	if len(test) == 5 {
		name = fmt.Sprintf("test (%s)", test[4])
	} else {
		name = fmt.Sprintf("test ([%s, %s, %s])", test[0], test[1],
			test[2])
	}
	return name, nil
}

// parseScriptFlags parses the provided flags string from the format used in the
// reference tests into ScriptFlags suitable for use in the script engine.
func parseScriptFlags(flagStr string) (ScriptFlags, error) {
	var flags ScriptFlags

	sFlags := strings.Split(flagStr, ",")
	for _, flag := range sFlags {
		switch flag {
		case "":
			// Nothing.
		case "CHECKLOCKTIMEVERIFY":
			flags |= ScriptVerifyCheckLockTimeVerify
		case "CHECKSEQUENCEVERIFY":
			flags |= ScriptVerifyCheckSequenceVerify
		case "CLEANSTACK":
			flags |= ScriptVerifyCleanStack
		case "DISCOURAGE_UPGRADABLE_NOPS":
			flags |= ScriptDiscourageUpgradableNops
		case "NONE":
			// Nothing.
		case "SIGPUSHONLY":
			flags |= ScriptVerifySigPushOnly
		case "SHA256":
			flags |= ScriptVerifySHA256
		case "TREASURY":
			flags |= ScriptVerifyTreasury
		default:
			return flags, fmt.Errorf("invalid flag: %s", flag)
		}
	}
	return flags, nil
}

// parseExpectedResult parses the provided expected result string into allowed
// script error kinds.  An error is returned if the expected result string is
// not supported.
func parseExpectedResult(expected string) ([]ErrorKind, error) {
	switch expected {
	case "OK":
		return nil, nil
	case "ERR_EARLY_RETURN":
		return []ErrorKind{ErrEarlyReturn}, nil
	case "ERR_EMPTY_STACK":
		return []ErrorKind{ErrEmptyStack}, nil
	case "ERR_EVAL_FALSE":
		return []ErrorKind{ErrEvalFalse}, nil
	case "ERR_SCRIPT_SIZE":
		return []ErrorKind{ErrScriptTooBig}, nil
	case "ERR_PUSH_SIZE":
		return []ErrorKind{ErrElementTooBig}, nil
	case "ERR_OP_COUNT":
		return []ErrorKind{ErrTooManyOperations}, nil
	case "ERR_STACK_SIZE":
		return []ErrorKind{ErrStackOverflow}, nil
	case "ERR_PUBKEY_COUNT":
		return []ErrorKind{ErrInvalidPubKeyCount}, nil
	case "ERR_SIG_COUNT":
		return []ErrorKind{ErrInvalidSignatureCount}, nil
	case "ERR_OUT_OF_RANGE":
		return []ErrorKind{ErrNumOutOfRange}, nil
	case "ERR_VERIFY":
		return []ErrorKind{ErrVerify}, nil
	case "ERR_EQUAL_VERIFY":
		return []ErrorKind{ErrEqualVerify}, nil
	case "ERR_DISABLED_OPCODE":
		return []ErrorKind{ErrDisabledOpcode}, nil
	case "ERR_RESERVED_OPCODE":
		return []ErrorKind{ErrReservedOpcode}, nil
	case "ERR_P2SH_STAKE_OPCODES":
		return []ErrorKind{ErrP2SHStakeOpCodes}, nil
	case "ERR_MALFORMED_PUSH":
		return []ErrorKind{ErrMalformedPush}, nil
	case "ERR_INVALID_STACK_OPERATION", "ERR_INVALID_ALTSTACK_OPERATION":
		return []ErrorKind{ErrInvalidStackOperation}, nil
	case "ERR_UNBALANCED_CONDITIONAL":
		return []ErrorKind{ErrUnbalancedConditional}, nil
	case "ERR_NEGATIVE_SUBSTR_INDEX":
		return []ErrorKind{ErrNegativeSubstrIdx}, nil
	case "ERR_OVERFLOW_SUBSTR_INDEX":
		return []ErrorKind{ErrOverflowSubstrIdx}, nil
	case "ERR_NEGATIVE_ROTATION":
		return []ErrorKind{ErrNegativeRotation}, nil
	case "ERR_OVERFLOW_ROTATION":
		return []ErrorKind{ErrOverflowRotation}, nil
	case "ERR_DIVIDE_BY_ZERO":
		return []ErrorKind{ErrDivideByZero}, nil
	case "ERR_NEGATIVE_SHIFT":
		return []ErrorKind{ErrNegativeShift}, nil
	case "ERR_OVERFLOW_SHIFT":
		return []ErrorKind{ErrOverflowShift}, nil
	case "ERR_MINIMAL_DATA":
		return []ErrorKind{ErrMinimalData}, nil
	case "ERR_SIG_HASH_TYPE":
		return []ErrorKind{ErrInvalidSigHashType}, nil
	case "ERR_SIG_TOO_SHORT":
		return []ErrorKind{ErrSigTooShort}, nil
	case "ERR_SIG_TOO_LONG":
		return []ErrorKind{ErrSigTooLong}, nil
	case "ERR_SIG_INVALID_SEQ_ID":
		return []ErrorKind{ErrSigInvalidSeqID}, nil
	case "ERR_SIG_INVALID_DATA_LEN":
		return []ErrorKind{ErrSigInvalidDataLen}, nil
	case "ERR_SIG_MISSING_S_TYPE_ID":
		return []ErrorKind{ErrSigMissingSTypeID}, nil
	case "ERR_SIG_MISSING_S_LEN":
		return []ErrorKind{ErrSigMissingSLen}, nil
	case "ERR_SIG_INVALID_S_LEN":
		return []ErrorKind{ErrSigInvalidSLen}, nil
	case "ERR_SIG_INVALID_R_INT_ID":
		return []ErrorKind{ErrSigInvalidRIntID}, nil
	case "ERR_SIG_ZERO_R_LEN":
		return []ErrorKind{ErrSigZeroRLen}, nil
	case "ERR_SIG_NEGATIVE_R":
		return []ErrorKind{ErrSigNegativeR}, nil
	case "ERR_SIG_TOO_MUCH_R_PADDING":
		return []ErrorKind{ErrSigTooMuchRPadding}, nil
	case "ERR_SIG_INVALID_S_INT_ID":
		return []ErrorKind{ErrSigInvalidSIntID}, nil
	case "ERR_SIG_ZERO_S_LEN":
		return []ErrorKind{ErrSigZeroSLen}, nil
	case "ERR_SIG_NEGATIVE_S":
		return []ErrorKind{ErrSigNegativeS}, nil
	case "ERR_SIG_TOO_MUCH_S_PADDING":
		return []ErrorKind{ErrSigTooMuchSPadding}, nil
	case "ERR_SIG_HIGH_S":
		return []ErrorKind{ErrSigHighS}, nil
	case "ERR_SIG_PUSHONLY":
		return []ErrorKind{ErrNotPushOnly}, nil
	case "ERR_PUBKEY_TYPE":
		return []ErrorKind{ErrPubKeyType}, nil
	case "ERR_CLEAN_STACK":
		return []ErrorKind{ErrCleanStack}, nil
	case "ERR_DISCOURAGE_UPGRADABLE_NOPS":
		return []ErrorKind{ErrDiscourageUpgradableNOPs}, nil
	case "ERR_NEGATIVE_LOCKTIME":
		return []ErrorKind{ErrNegativeLockTime}, nil
	case "ERR_UNSATISFIED_LOCKTIME":
		return []ErrorKind{ErrUnsatisfiedLockTime}, nil
	}

	return nil, fmt.Errorf("unrecognized expected result in test data: %v",
		expected)
}

// createSpendTx generates a basic spending transaction given the passed
// signature and public key scripts.
func createSpendingTx(sigScript, pkScript []byte) *wire.MsgTx {
	coinbaseTx := wire.NewMsgTx()

	outPoint := wire.NewOutPoint(&chainhash.Hash{}, ^uint32(0),
		wire.TxTreeRegular)
	txIn := wire.NewTxIn(outPoint, 0, []byte{OP_0, OP_0})
	txOut := wire.NewTxOut(0, pkScript)
	coinbaseTx.AddTxIn(txIn)
	coinbaseTx.AddTxOut(txOut)

	spendingTx := wire.NewMsgTx()
	coinbaseTxHash := coinbaseTx.TxHash()
	outPoint = wire.NewOutPoint(&coinbaseTxHash, 0, wire.TxTreeRegular)
	txIn = wire.NewTxIn(outPoint, 0, sigScript)
	txOut = wire.NewTxOut(0, nil)

	spendingTx.AddTxIn(txIn)
	spendingTx.AddTxOut(txOut)

	return spendingTx
}

// testScripts ensures all of the passed script tests execute with the expected
// results with or without using a signature cache, as specified by the
// parameter.
func testScripts(t *testing.T, tests [][]string, useSigCache bool) {
	// Create a signature cache to use only if requested.
	var sigCache *SigCache
	if useSigCache {
		var err error
		sigCache, err = NewSigCache(10)
		if err != nil {
			t.Fatalf("error creating NewSigCache: %v", err)
		}
	}

	// "Format is: [scriptSig, scriptPubKey, flags, expectedScriptError, ...
	//   comments]"
	for i, test := range tests {
		// Skip single line comments.
		if len(test) == 1 {
			continue
		}

		// Construct a name for the test based on the comment and test data.
		name, err := scriptTestName(test)
		if err != nil {
			t.Errorf("TestScripts: invalid test #%d: %v", i, err)
			continue
		}

		// Extract and parse the signature script from the test fields.
		scriptSig, err := parseShortForm(test[0])
		if err != nil {
			t.Errorf("%s: can't parse scriptSig; %v", name, err)
			continue
		}

		// Extract and parse the public key script from the test fields.
		scriptPubKey, err := parseShortForm(test[1])
		if err != nil {
			t.Errorf("%s: can't parse scriptPubkey; %v", name, err)
			continue
		}

		// Extract and parse the script flags from the test fields.
		flags, err := parseScriptFlags(test[2])
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}

		// Extract and parse the expected result from the test fields.
		//
		// Convert the expected result string into the allowed script errors.
		// This allows txscript to be more fine grained with its errors than the
		// reference test data by allowing some of the test data errors to map
		// to more than one possibility.
		resultStr := test[3]
		allowErrorKinds, err := parseExpectedResult(resultStr)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}

		// Generate a transaction pair such that one spends from the other and
		// the provided signature and public key scripts are used, then create a
		// new engine to execute the scripts.
		tx := createSpendingTx(scriptSig, scriptPubKey)
		vm, err := NewEngine(scriptPubKey, tx, 0, flags, 0, sigCache)
		if err == nil {
			err = vm.Execute()
		}

		// Ensure there were no errors when the expected result is OK.
		if resultStr == "OK" {
			if err != nil {
				t.Errorf("%s failed to execute: %v", name, err)
			}
			continue
		}

		// At this point an error was expected so ensure the result of the
		// execution matches it.
		success := false
		for _, kind := range allowErrorKinds {
			if errors.Is(err, kind) {
				success = true
				break
			}
		}
		if !success {
			var serr Error
			if errors.As(err, &serr) {
				t.Errorf("%s: want error kinds %v, got %v", name,
					allowErrorKinds, serr.Err)
				continue
			}
			t.Errorf("%s: want error kinds %v, got err: %v (%T)", name,
				allowErrorKinds, err, err)
			continue
		}
	}
}

// TestScripts ensures all of the tests in script_tests.json execute with the
// expected results as defined in the test data.
func TestScripts(t *testing.T) {
	file, err := os.ReadFile("data/script_tests.json")
	if err != nil {
		t.Fatalf("TestScripts: %v\n", err)
	}

	var tests [][]string
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Fatalf("TestScripts failed to unmarshal: %v", err)
	}

	// Run all script tests with and without the signature cache.
	testScripts(t, tests, true)
	testScripts(t, tests, false)
}

// testVecF64ToUint32 properly handles conversion of float64s read from the JSON
// test data to unsigned 32-bit integers.  This is necessary because some of the
// test data uses -1 as a shortcut to mean max uint32 and direct conversion of a
// negative float to an unsigned int is implementation dependent and therefore
// doesn't result in the expected value on all platforms.  This function works
// around that limitation by converting to a 32-bit signed integer first and
// then to a 32-bit unsigned integer which results in the expected behavior on
// all platforms.
func testVecF64ToUint32(f float64) uint32 {
	return uint32(int32(f))
}

// TestTxInvalidTests ensures all of the tests in tx_invalid.json fail as
// expected.
func TestTxInvalidTests(t *testing.T) {
	file, err := os.ReadFile("data/tx_invalid.json")
	if err != nil {
		t.Errorf("TestTxInvalidTests: %v\n", err)
		return
	}

	var tests [][]interface{}
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Errorf("TestTxInvalidTests couldn't Unmarshal: %v\n", err)
		return
	}

	// form is either:
	//   ["this is a comment "]
	// or:
	//   [[[previous hash, previous index, previous scriptPubKey]...,]
	//	serializedTransaction, verifyFlags]
testloop:
	for i, test := range tests {
		inputs, ok := test[0].([]interface{})
		if !ok {
			continue
		}

		if len(test) != 3 {
			t.Errorf("bad test (bad length) %d: %v", i, test)
			continue
		}
		serializedhex, ok := test[1].(string)
		if !ok {
			t.Errorf("bad test (arg 2 not string) %d: %v", i, test)
			continue
		}
		serializedTx, err := hex.DecodeString(serializedhex)
		if err != nil {
			t.Errorf("bad test (arg 2 not hex %v) %d: %v", err, i,
				test)
			continue
		}

		var tx wire.MsgTx
		if err := tx.Deserialize(bytes.NewReader(serializedTx)); err != nil {
			t.Errorf("bad test (arg 2 not msgtx %v) %d: %v", err, i, test)
			continue
		}

		verifyFlags, ok := test[2].(string)
		if !ok {
			t.Errorf("bad test (arg 3 not string) %d: %v", i, test)
			continue
		}

		flags, err := parseScriptFlags(verifyFlags)
		if err != nil {
			t.Errorf("bad test %d: %v", i, err)
			continue
		}

		prevOuts := make(map[wire.OutPoint][]byte)
		for j, iinput := range inputs {
			input, ok := iinput.([]interface{})
			if !ok {
				t.Errorf("bad test (%dth input not array)"+
					"%d: %v", j, i, test)
				continue testloop
			}

			if len(input) != 3 {
				t.Errorf("bad test (%dth input wrong length)"+
					"%d: %v", j, i, test)
				continue testloop
			}

			previoustx, ok := input[0].(string)
			if !ok {
				t.Errorf("bad test (%dth input hash not string)"+
					"%d: %v", j, i, test)
				continue testloop
			}

			prevhash, err := chainhash.NewHashFromStr(previoustx)
			if err != nil {
				t.Errorf("bad test (%dth input hash not hash %v)"+
					"%d: %v", j, err, i, test)
				continue testloop
			}

			idxf, ok := input[1].(float64)
			if !ok {
				t.Errorf("bad test (%dth input idx not number)"+
					"%d: %v", j, i, test)
				continue testloop
			}
			idx := testVecF64ToUint32(idxf)

			oscript, ok := input[2].(string)
			if !ok {
				t.Errorf("bad test (%dth input script not "+
					"string) %d: %v", j, i, test)
				continue testloop
			}

			script, err := parseShortForm(oscript)
			if err != nil {
				t.Errorf("bad test (%dth input script doesn't "+
					"parse %v) %d: %v", j, err, i, test)
				continue testloop
			}

			prevOuts[*wire.NewOutPoint(prevhash, idx, wire.TxTreeRegular)] = script
		}

		for k, txIn := range tx.TxIn {
			pkScript, ok := prevOuts[txIn.PreviousOutPoint]
			if !ok {
				t.Errorf("bad test (missing %dth input) %d:%v",
					k, i, test)
				continue testloop
			}
			// These are meant to fail, so as soon as the first
			// input fails the transaction has failed. (some of the
			// test txns have good inputs, too..
			vm, err := NewEngine(pkScript, &tx, k, flags, 0, nil)
			if err != nil {
				continue testloop
			}

			err = vm.Execute()
			if err != nil {
				continue testloop
			}
		}
		t.Errorf("test (%d:%v) succeeded when should fail",
			i, test)
	}
}

// TestTxValidTests ensures all of the tests in tx_valid.json pass as expected.
func TestTxValidTests(t *testing.T) {
	file, err := os.ReadFile("data/tx_valid.json")
	if err != nil {
		t.Errorf("TestTxValidTests: %v\n", err)
		return
	}

	var tests [][]interface{}
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Errorf("TestTxValidTests couldn't Unmarshal: %v\n", err)
		return
	}

	// form is either:
	//   ["this is a comment "]
	// or:
	//   [[[previous hash, previous index, previous scriptPubKey]...,]
	//	serializedTransaction, verifyFlags]
testloop:
	for i, test := range tests {
		inputs, ok := test[0].([]interface{})
		if !ok {
			continue
		}

		if len(test) != 3 {
			t.Errorf("bad test (bad length) %d: %v", i, test)
			continue
		}
		serializedhex, ok := test[1].(string)
		if !ok {
			t.Errorf("bad test (arg 2 not string) %d: %v", i, test)
			continue
		}
		serializedTx, err := hex.DecodeString(serializedhex)
		if err != nil {
			t.Errorf("bad test (arg 2 not hex %v) %d: %v", err, i,
				test)
			continue
		}

		var tx wire.MsgTx
		if err := tx.Deserialize(bytes.NewReader(serializedTx)); err != nil {
			t.Errorf("bad test (arg 2 not msgtx %v) %d: %v", err, i, test)
			continue
		}

		verifyFlags, ok := test[2].(string)
		if !ok {
			t.Errorf("bad test (arg 3 not string) %d: %v", i, test)
			continue
		}

		flags, err := parseScriptFlags(verifyFlags)
		if err != nil {
			t.Errorf("bad test %d: %v", i, err)
			continue
		}

		prevOuts := make(map[wire.OutPoint][]byte)
		for j, iinput := range inputs {
			input, ok := iinput.([]interface{})
			if !ok {
				t.Errorf("bad test (%dth input not array)"+
					"%d: %v", j, i, test)
				continue
			}

			if len(input) != 3 {
				t.Errorf("bad test (%dth input wrong length)"+
					"%d: %v", j, i, test)
				continue
			}

			previoustx, ok := input[0].(string)
			if !ok {
				t.Errorf("bad test (%dth input hash not string)"+
					"%d: %v", j, i, test)
				continue
			}

			prevhash, err := chainhash.NewHashFromStr(previoustx)
			if err != nil {
				t.Errorf("bad test (%dth input hash not hash %v)"+
					"%d: %v", j, err, i, test)
				continue
			}

			idxf, ok := input[1].(float64)
			if !ok {
				t.Errorf("bad test (%dth input idx not number)"+
					"%d: %v", j, i, test)
				continue
			}
			idx := testVecF64ToUint32(idxf)

			oscript, ok := input[2].(string)
			if !ok {
				t.Errorf("bad test (%dth input script not "+
					"string) %d: %v", j, i, test)
				continue
			}

			script, err := parseShortForm(oscript)
			if err != nil {
				t.Errorf("bad test (%dth input script doesn't "+
					"parse %v) %d: %v", j, err, i, test)
				continue
			}

			prevOuts[*wire.NewOutPoint(prevhash, idx, wire.TxTreeRegular)] = script
		}

		for k, txIn := range tx.TxIn {
			pkScript, ok := prevOuts[txIn.PreviousOutPoint]
			if !ok {
				t.Errorf("bad test (missing %dth input) %d:%v",
					k, i, test)
				continue testloop
			}
			vm, err := NewEngine(pkScript, &tx, k, flags, 0, nil)
			if err != nil {
				t.Errorf("test (%d:%v:%d) failed to create "+
					"script: %v", i, test, k, err)
				continue
			}

			err = vm.Execute()
			if err != nil {
				t.Errorf("test (%d:%v:%d) failed to execute: "+
					"%v", i, test, k, err)
				continue
			}
		}
	}
}

// parseSigHashExpectedResult parses the provided expected result string into
// allowed error kinds.  An error is returned if the expected result string is
// not supported.
func parseSigHashExpectedResult(expected string) (error, error) {
	switch expected {
	case "OK":
		return nil, nil
	case "SIGHASH_SINGLE_IDX":
		return ErrInvalidSigHashSingleIndex, nil
	}

	return nil, fmt.Errorf("unrecognized expected result in test data: %v",
		expected)
}

// TestCalcSignatureHashReference runs the reference signature hash calculation
// tests in sighash.json.
func TestCalcSignatureHashReference(t *testing.T) {
	file, err := os.ReadFile("data/sighash.json")
	if err != nil {
		t.Fatalf("TestCalcSignatureHash: %v\n", err)
	}

	var tests [][]interface{}
	err = json.Unmarshal(file, &tests)
	if err != nil {
		t.Fatalf("TestCalcSignatureHash couldn't Unmarshal: %v\n", err)
	}

	const scriptVersion = 0
	for i, test := range tests {
		// Skip comment lines.
		if len(test) == 1 {
			continue
		}

		// Ensure test is well formed.
		if len(test) < 6 || len(test) > 7 {
			t.Fatalf("Test #%d: wrong length %d", i, len(test))
		}

		// Extract and parse the transaction from the test fields.
		txHex, ok := test[0].(string)
		if !ok {
			t.Errorf("Test #%d: transaction is not a string", i)
			continue
		}
		rawTx, err := hex.DecodeString(txHex)
		if err != nil {
			t.Errorf("Test #%d: unable to parse transaction: %v", i, err)
			continue
		}
		var tx wire.MsgTx
		err = tx.Deserialize(bytes.NewReader(rawTx))
		if err != nil {
			t.Errorf("Test #%d: unable to deserialize transaction: %v", i, err)
			continue
		}

		// Extract and parse the script from the test fields.
		subScriptStr, ok := test[1].(string)
		if !ok {
			t.Errorf("Test #%d: script is not a string", i)
			continue
		}
		subScript, err := hex.DecodeString(subScriptStr)
		if err != nil {
			t.Errorf("Test #%d: unable to decode script: %v", i, err)
			continue
		}
		err = checkScriptParses(scriptVersion, subScript)
		if err != nil {
			t.Errorf("Test #%d: unable to parse script: %v", i, err)
			continue
		}

		// Extract the input index from the test fields.
		inputIdxF64, ok := test[2].(float64)
		if !ok {
			t.Errorf("Test #%d: input idx is not numeric", i)
			continue
		}

		// Extract and parse the hash type from the test fields.
		hashTypeF64, ok := test[3].(float64)
		if !ok {
			t.Errorf("Test #%d: hash type is not numeric", i)
			continue
		}
		hashType := SigHashType(testVecF64ToUint32(hashTypeF64))

		// Extract and parse the signature hash from the test fields.
		expectedHashStr, ok := test[4].(string)
		if !ok {
			t.Errorf("Test #%d: signature hash is not a string", i)
			continue
		}
		expectedHash, err := hex.DecodeString(expectedHashStr)
		if err != nil {
			t.Errorf("Test #%d: unable to sig hash: %v", i, err)
			continue
		}

		// Extract and parse the expected result from the test fields.
		expectedErrStr, ok := test[5].(string)
		if !ok {
			t.Errorf("Test #%d: result field is not a string", i)
			continue
		}
		expectedErr, err := parseSigHashExpectedResult(expectedErrStr)
		if err != nil {
			t.Errorf("Test #%d: %v", i, err)
			continue
		}

		// Calculate the signature hash and verify expected result.
		hash, err := CalcSignatureHash(subScript, hashType, &tx,
			int(inputIdxF64), nil)
		if !errors.Is(err, expectedErr) {
			t.Errorf("Test #%d: want error kind %v, got err: %v (%T)", i,
				expectedErr, err, err)
			continue
		}
		if !bytes.Equal(hash, expectedHash) {
			t.Errorf("Test #%d: signature hash mismatch - got %x, want %x", i,
				hash, expectedHash)
			continue
		}
	}
}
