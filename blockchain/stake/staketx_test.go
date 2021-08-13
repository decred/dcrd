// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math/rand"
	"reflect"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

const (
	// noTreasury signifies the treasury agenda should be treated as though
	// it is inactive.  It is used to increase the readability of the
	// tests.
	noTreasury = false

	// noAutoRevocations signifies the automatic ticket revocations agenda should
	// be treated as though it is inactive.  It is used to increase the
	// readability of the tests.
	noAutoRevocations = false

	// withTreasury signifies the treasury agenda should be treated as
	// though it is active.  It is used to increase the readability of
	// the tests.
	withTreasury = true
)

// hexToBytes converts the passed hex string into bytes and will panic if there
// is an error.  This is only provided for the hard-coded constants so errors in
// the source code can be detected. It will only (and must only) be called with
// hard-coded values.
func hexToBytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	return b
}

// mustParseHash converts the passed big-endian hex string into a
// chainhash.Hash and will panic if there is an error.  It only differs from the
// one available in chainhash in that it will panic so errors in the source code
// be detected.  It will only (and must only) be called with hard-coded, and
// therefore known good, hashes.
func mustParseHash(s string) *chainhash.Hash {
	hash, err := chainhash.NewHashFromStr(s)
	if err != nil {
		panic("invalid hash in source file: " + s)
	}
	return hash
}

// SSTX TESTING -------------------------------------------------------------------

// TestSStx ensures the CheckSStx and IsSStx functions correctly recognize stake
// submission transactions.
func TestSStx(t *testing.T) {
	var sstx = dcrutil.NewTx(sstxMsgTx)
	sstx.SetTree(wire.TxTreeStake)
	sstx.SetIndex(0)

	err := CheckSStx(sstx.MsgTx())
	if err != nil {
		t.Errorf("CheckSStx: unexpected err: %v", err)
	}
	if !IsSStx(sstx.MsgTx()) {
		t.Errorf("IsSStx claimed a valid sstx is invalid")
	}

	// ---------------------------------------------------------------------------
	// Test for an OP_RETURN commitment push of the maximum size
	biggestPush := []byte{
		0x6a, 0x4b, // OP_RETURN Push 75-bytes
		0x14, 0x94, 0x8c, 0x76, 0x5a, 0x69, 0x14, 0xd4, // 75 bytes
		0x3f, 0x2a, 0x7a, 0xc1, 0x77, 0xda, 0x2c, 0x2f,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde,
	}

	sstx = dcrutil.NewTxDeep(sstxMsgTx)
	sstx.MsgTx().TxOut[1].PkScript = biggestPush
	sstx.SetTree(wire.TxTreeStake)
	sstx.SetIndex(0)

	err = CheckSStx(sstx.MsgTx())
	if err != nil {
		t.Errorf("CheckSStx: unexpected err: %v", err)
	}
	if !IsSStx(sstx.MsgTx()) {
		t.Errorf("IsSStx claimed a valid sstx is invalid")
	}
}

// TestSSTxErrors ensures the CheckSStx and IsSStx functions correctly identify
// errors in stake submission transactions and does not report them as valid.
func TestSSTxErrors(t *testing.T) {
	// Initialize the buffer for later manipulation
	var buf bytes.Buffer
	buf.Grow(sstxMsgTx.SerializeSize())
	err := sstxMsgTx.Serialize(&buf)
	if err != nil {
		t.Errorf("Error serializing the reference sstx: %v", err)
	}
	bufBytes := buf.Bytes()

	// ---------------------------------------------------------------------------
	// Test too many inputs with sstxMsgTxExtraInputs

	var sstxExtraInputs = dcrutil.NewTx(sstxMsgTxExtraInput)
	sstxExtraInputs.SetTree(wire.TxTreeStake)
	sstxExtraInputs.SetIndex(0)

	err = CheckSStx(sstxExtraInputs.MsgTx())
	if !errors.Is(err, ErrSStxTooManyInputs) {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			ErrSStxTooManyInputs, err)
	}
	if IsSStx(sstxExtraInputs.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test too many outputs with sstxMsgTxExtraOutputs

	var sstxExtraOutputs = dcrutil.NewTx(sstxMsgTxExtraOutputs)
	sstxExtraOutputs.SetTree(wire.TxTreeStake)
	sstxExtraOutputs.SetIndex(0)

	err = CheckSStx(sstxExtraOutputs.MsgTx())
	if !errors.Is(err, ErrSStxTooManyOutputs) {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			ErrSStxTooManyOutputs, err)
	}
	if IsSStx(sstxExtraOutputs.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Check to make sure the first output is OP_SSTX tagged

	var tx wire.MsgTx
	testFirstOutTagged := bytes.Replace(bufBytes,
		[]byte{0x00, 0xe3, 0x23, 0x21, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x1a, 0xba},
		[]byte{0x00, 0xe3, 0x23, 0x21, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x19},
		1)

	// Deserialize the manipulated tx
	rbuf := bytes.NewReader(testFirstOutTagged)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var sstxUntaggedOut = dcrutil.NewTx(&tx)
	sstxUntaggedOut.SetTree(wire.TxTreeStake)
	sstxUntaggedOut.SetIndex(0)

	err = CheckSStx(sstxUntaggedOut.MsgTx())
	if !errors.Is(err, ErrSStxInvalidOutputs) {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			ErrSStxInvalidOutputs, err)
	}
	if IsSStx(sstxUntaggedOut.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for mismatched number of inputs versus number of outputs

	var sstxInsOutsMismatched = dcrutil.NewTx(sstxMismatchedInsOuts)
	sstxInsOutsMismatched.SetTree(wire.TxTreeStake)
	sstxInsOutsMismatched.SetIndex(0)

	err = CheckSStx(sstxInsOutsMismatched.MsgTx())
	if !errors.Is(err, ErrSStxInOutProportions) {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			ErrSStxInOutProportions, err)
	}
	if IsSStx(sstxInsOutsMismatched.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for bad version of output.
	var sstxBadVerOut = dcrutil.NewTx(sstxBadVersionOut)
	sstxBadVerOut.SetTree(wire.TxTreeStake)
	sstxBadVerOut.SetIndex(0)

	err = CheckSStx(sstxBadVerOut.MsgTx())
	if !errors.Is(err, ErrSStxInvalidOutputs) {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			ErrSStxInvalidOutputs, err)
	}
	if IsSStx(sstxBadVerOut.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for second or more output not being OP_RETURN push

	var sstxNoNullData = dcrutil.NewTx(sstxNullDataMissing)
	sstxNoNullData.SetTree(wire.TxTreeStake)
	sstxNoNullData.SetIndex(0)

	err = CheckSStx(sstxNoNullData.MsgTx())
	if !errors.Is(err, ErrSStxInvalidOutputs) {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			ErrSStxInvalidOutputs, err)
	}
	if IsSStx(sstxNoNullData.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for change output being in the wrong place

	var sstxNullDataMis = dcrutil.NewTx(sstxNullDataMisplaced)
	sstxNullDataMis.SetTree(wire.TxTreeStake)
	sstxNullDataMis.SetIndex(0)

	err = CheckSStx(sstxNullDataMis.MsgTx())
	if !errors.Is(err, ErrSStxInvalidOutputs) {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			ErrSStxInvalidOutputs, err)
	}
	if IsSStx(sstxNullDataMis.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for too short of a pubkeyhash being given in an OP_RETURN output

	testPKHLength := bytes.Replace(bufBytes,
		[]byte{
			0x20, 0x6a, 0x1e, 0x94, 0x8c, 0x76, 0x5a, 0x69,
			0x14, 0xd4, 0x3f, 0x2a, 0x7a, 0xc1, 0x77, 0xda,
			0x2c, 0x2f, 0x6b, 0x52, 0xde, 0x3d, 0x7c,
		},
		[]byte{
			0x1f, 0x6a, 0x1d, 0x94, 0x8c, 0x76, 0x5a, 0x69,
			0x14, 0xd4, 0x3f, 0x2a, 0x7a, 0xc1, 0x77, 0xda,
			0x2c, 0x2f, 0x6b, 0x52, 0xde, 0x3d,
		},
		1)

	// Deserialize the manipulated tx
	rbuf = bytes.NewReader(testPKHLength)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var sstxWrongPKHLength = dcrutil.NewTx(&tx)
	sstxWrongPKHLength.SetTree(wire.TxTreeStake)
	sstxWrongPKHLength.SetIndex(0)

	err = CheckSStx(sstxWrongPKHLength.MsgTx())
	if !errors.Is(err, ErrSStxInvalidOutputs) {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			ErrSStxInvalidOutputs, err)
	}
	if IsSStx(sstxWrongPKHLength.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for an invalid OP_RETURN prefix with too big of a push
	tooBigPush := []byte{
		0x6a, 0x4c, 0x4c, // OP_RETURN Push 76-bytes
		0x14, 0x94, 0x8c, 0x76, 0x5a, 0x69, 0x14, 0xd4, // 76 bytes
		0x3f, 0x2a, 0x7a, 0xc1, 0x77, 0xda, 0x2c, 0x2f,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d,
	}

	// Deserialize the manipulated tx
	rbuf = bytes.NewReader(bufBytes)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}
	tx.TxOut[1].PkScript = tooBigPush

	var sstxWrongPrefix = dcrutil.NewTx(&tx)
	sstxWrongPrefix.SetTree(wire.TxTreeStake)
	sstxWrongPrefix.SetIndex(0)

	err = CheckSStx(sstxWrongPrefix.MsgTx())
	if !errors.Is(err, ErrSStxInvalidOutputs) {
		t.Errorf("CheckSStx should have returned %v but instead returned %v",
			ErrSStxInvalidOutputs, err)
	}
	if IsSStx(sstxWrongPrefix.MsgTx()) {
		t.Errorf("IsSStx claimed an invalid sstx is valid")
	}
}

// SSGEN TESTING ------------------------------------------------------------------

// TestSSGen ensures the CheckSSGen and IsSSGen functions correctly recognize
// stake submission generation transactions.
func TestSSGen(t *testing.T) {
	var ssgen = dcrutil.NewTx(ssgenMsgTx)
	ssgen.SetTree(wire.TxTreeStake)
	ssgen.SetIndex(0)

	err := CheckSSGen(ssgen.MsgTx(), noTreasury)
	if err != nil {
		t.Errorf("IsSSGen: unexpected err: %v", err)
	}
	if !IsSSGen(ssgen.MsgTx(), noTreasury) {
		t.Errorf("IsSSGen claimed a valid ssgen is invalid")
	}

	// Test for an OP_RETURN VoteBits push of the maximum size
	biggestPush := []byte{
		0x6a, 0x4b, // OP_RETURN Push 75-bytes
		0x14, 0x94, 0x8c, 0x76, 0x5a, 0x69, 0x14, 0xd4, // 75 bytes
		0x3f, 0x2a, 0x7a, 0xc1, 0x77, 0xda, 0x2c, 0x2f,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde, 0x3d, 0x7c, 0x7c, 0x7c, 0x7c,
		0x6b, 0x52, 0xde,
	}

	ssgen = dcrutil.NewTxDeep(ssgenMsgTx)
	ssgen.SetTree(wire.TxTreeStake)
	ssgen.SetIndex(0)
	ssgen.MsgTx().TxOut[1].PkScript = biggestPush

	err = CheckSSGen(ssgen.MsgTx(), noTreasury)
	if err != nil {
		t.Errorf("IsSSGen: unexpected err: %v", err)
	}
	if !IsSSGen(ssgen.MsgTx(), noTreasury) {
		t.Errorf("IsSSGen claimed a valid ssgen is invalid")
	}
}

// TestSSGenErrors ensures the CheckSSGen and IsSSGen functions correctly
// identify errors in stake submission generation transactions and does not
// report them as valid.
func TestSSGenErrors(t *testing.T) {
	// Initialize the buffer for later manipulation
	var buf bytes.Buffer
	buf.Grow(ssgenMsgTx.SerializeSize())
	err := ssgenMsgTx.Serialize(&buf)
	if err != nil {
		t.Errorf("Error serializing the reference sstx: %v", err)
	}
	bufBytes := buf.Bytes()

	// ---------------------------------------------------------------------------
	// Test too many inputs with ssgenMsgTxExtraInputs

	var ssgenExtraInputs = dcrutil.NewTx(ssgenMsgTxExtraInput)
	ssgenExtraInputs.SetTree(wire.TxTreeStake)
	ssgenExtraInputs.SetIndex(0)

	err = CheckSSGen(ssgenExtraInputs.MsgTx(), noTreasury)
	if !errors.Is(err, ErrSSGenWrongNumInputs) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenWrongNumInputs, err)
	}
	if IsSSGen(ssgenExtraInputs.MsgTx(), noTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test too many outputs with sstxMsgTxExtraOutputs

	var ssgenExtraOutputs = dcrutil.NewTx(ssgenMsgTxExtraOutputs)
	ssgenExtraOutputs.SetTree(wire.TxTreeStake)
	ssgenExtraOutputs.SetIndex(0)

	err = CheckSSGen(ssgenExtraOutputs.MsgTx(), noTreasury)
	if !errors.Is(err, ErrSSGenTooManyOutputs) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenTooManyOutputs, err)
	}
	if IsSSGen(ssgenExtraOutputs.MsgTx(), noTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test 0th input not being stakebase error

	var ssgenStakeBaseWrong = dcrutil.NewTx(ssgenMsgTxStakeBaseWrong)
	ssgenStakeBaseWrong.SetTree(wire.TxTreeStake)
	ssgenStakeBaseWrong.SetIndex(0)

	err = CheckSSGen(ssgenStakeBaseWrong.MsgTx(), noTreasury)
	if !errors.Is(err, ErrSSGenNoStakebase) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenNoStakebase, err)
	}
	if IsSSGen(ssgenStakeBaseWrong.MsgTx(), noTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Wrong tree for inputs test

	// Replace TxTreeStake with TxTreeRegular
	testWrongTreeInputs := bytes.Replace(bufBytes,
		[]byte{0x79, 0xac, 0x88, 0xfd, 0xf3, 0x57, 0xa1, 0x87, 0x00,
			0x00, 0x00, 0x00, 0x01},
		[]byte{0x79, 0xac, 0x88, 0xfd, 0xf3, 0x57, 0xa1, 0x87, 0x00,
			0x00, 0x00, 0x00, 0x00},
		1)

	// Deserialize the manipulated tx
	var tx wire.MsgTx
	rbuf := bytes.NewReader(testWrongTreeInputs)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var ssgenWrongTreeIns = dcrutil.NewTx(&tx)
	ssgenWrongTreeIns.SetTree(wire.TxTreeStake)
	ssgenWrongTreeIns.SetIndex(0)

	err = CheckSSGen(ssgenWrongTreeIns.MsgTx(), noTreasury)
	if !errors.Is(err, ErrSSGenWrongTxTree) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenWrongTxTree, err)
	}
	if IsSSGen(ssgenWrongTreeIns.MsgTx(), noTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for bad version of output.
	var ssgenTxBadVerOut = dcrutil.NewTx(ssgenMsgTxBadVerOut)
	ssgenTxBadVerOut.SetTree(wire.TxTreeStake)
	ssgenTxBadVerOut.SetIndex(0)

	err = CheckSSGen(ssgenTxBadVerOut.MsgTx(), noTreasury)
	if !errors.Is(err, ErrSSGenBadGenOuts) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenBadGenOuts, err)
	}
	if IsSSGen(ssgenTxBadVerOut.MsgTx(), noTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test 0th output not being OP_RETURN push

	var ssgenWrongZeroethOut = dcrutil.NewTx(ssgenMsgTxWrongZeroethOut)
	ssgenWrongZeroethOut.SetTree(wire.TxTreeStake)
	ssgenWrongZeroethOut.SetIndex(0)

	err = CheckSSGen(ssgenWrongZeroethOut.MsgTx(), noTreasury)
	if !errors.Is(err, ErrSSGenNoReference) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenNoReference, err)
	}
	if IsSSGen(ssgenWrongZeroethOut.MsgTx(), noTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for too short of an OP_RETURN push being given in the 0th tx out

	testDataPush0Length := bytes.Replace(bufBytes,
		[]byte{
			0x26, 0x6a, 0x24,
			0x94, 0x8c, 0x76, 0x5a, 0x69, 0x14, 0xd4, 0x3f,
			0x2a, 0x7a, 0xc1, 0x77, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0x52, 0xde, 0x3d, 0x7c,
			0x00, 0xe3, 0x23, 0x21,
		},
		[]byte{
			0x25, 0x6a, 0x23,
			0x94, 0x8c, 0x76, 0x5a, 0x69, 0x14, 0xd4, 0x3f,
			0x2a, 0x7a, 0xc1, 0x77, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0x52, 0xde, 0x3d, 0x7c,
			0x00, 0xe3, 0x23,
		},
		1)

	// Deserialize the manipulated tx
	rbuf = bytes.NewReader(testDataPush0Length)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var ssgenWrongDataPush0Length = dcrutil.NewTx(&tx)
	ssgenWrongDataPush0Length.SetTree(wire.TxTreeStake)
	ssgenWrongDataPush0Length.SetIndex(0)

	err = CheckSSGen(ssgenWrongDataPush0Length.MsgTx(), noTreasury)
	if !errors.Is(err, ErrSSGenBadReference) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenBadReference, err)
	}
	if IsSSGen(ssgenWrongDataPush0Length.MsgTx(), noTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for an invalid OP_RETURN prefix

	testNullData0Prefix := bytes.Replace(bufBytes,
		[]byte{
			0x26, 0x6a, 0x24,
			0x94, 0x8c, 0x76, 0x5a, 0x69, 0x14, 0xd4, 0x3f,
			0x2a, 0x7a, 0xc1, 0x77, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0x52, 0xde, 0x3d, 0x7c,
			0x00, 0xe3, 0x23, 0x21,
		},
		[]byte{ // This uses an OP_PUSHDATA1 35-byte push to achieve 36 bytes
			0x26, 0x6a, 0x4c, 0x23,
			0x94, 0x8c, 0x76, 0x5a, 0x69, 0x14, 0xd4, 0x3f,
			0x2a, 0x7a, 0xc1, 0x77, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c, 0x52, 0xde, 0x3d, 0x7c,
			0x00, 0xe3, 0x23,
		},
		1)

	// Deserialize the manipulated tx
	rbuf = bytes.NewReader(testNullData0Prefix)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var ssgenWrongNullData0Prefix = dcrutil.NewTx(&tx)
	ssgenWrongNullData0Prefix.SetTree(wire.TxTreeStake)
	ssgenWrongNullData0Prefix.SetIndex(0)

	err = CheckSSGen(ssgenWrongNullData0Prefix.MsgTx(), noTreasury)
	if !errors.Is(err, ErrSSGenBadReference) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenBadReference, err)
	}
	if IsSSGen(ssgenWrongNullData0Prefix.MsgTx(), noTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test 1st output not being OP_RETURN push

	var ssgenWrongFirstOut = dcrutil.NewTx(ssgenMsgTxWrongFirstOut)
	ssgenWrongFirstOut.SetTree(wire.TxTreeStake)
	ssgenWrongFirstOut.SetIndex(0)

	err = CheckSSGen(ssgenWrongFirstOut.MsgTx(), noTreasury)
	if !errors.Is(err, ErrSSGenNoVotePush) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenNoVotePush, err)
	}
	if IsSSGen(ssgenWrongFirstOut.MsgTx(), noTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for too short of an OP_RETURN push being given in the 1st tx out
	testDataPush1Length := bytes.Replace(bufBytes,
		[]byte{
			0x04, 0x6a, 0x02, 0x94, 0x8c,
		},
		[]byte{
			0x03, 0x6a, 0x01, 0x94,
		},
		1)

	// Deserialize the manipulated tx
	rbuf = bytes.NewReader(testDataPush1Length)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var ssgenWrongDataPush1Length = dcrutil.NewTx(&tx)
	ssgenWrongDataPush1Length.SetTree(wire.TxTreeStake)
	ssgenWrongDataPush1Length.SetIndex(0)

	err = CheckSSGen(ssgenWrongDataPush1Length.MsgTx(), noTreasury)
	if !errors.Is(err, ErrSSGenBadVotePush) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenBadVotePush, err)
	}
	if IsSSGen(ssgenWrongDataPush1Length.MsgTx(), noTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for an invalid OP_RETURN prefix

	testNullData1Prefix := bytes.Replace(bufBytes,
		[]byte{
			0x04, 0x6a, 0x02, 0x94, 0x8c,
		},
		[]byte{ // This uses an OP_PUSHDATA1 2-byte push to do the push in 5 bytes
			0x05, 0x6a, 0x4c, 0x02, 0x00, 0x00,
		},
		1)

	// Deserialize the manipulated tx
	rbuf = bytes.NewReader(testNullData1Prefix)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var ssgenWrongNullData1Prefix = dcrutil.NewTx(&tx)
	ssgenWrongNullData1Prefix.SetTree(wire.TxTreeStake)
	ssgenWrongNullData1Prefix.SetIndex(0)

	err = CheckSSGen(ssgenWrongNullData1Prefix.MsgTx(), noTreasury)
	if !errors.Is(err, ErrSSGenBadVotePush) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenBadVotePush, err)
	}
	if IsSSGen(ssgenWrongNullData1Prefix.MsgTx(), noTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Test for an index 2+ output being not OP_SSGEN tagged

	testGenOutputUntagged := bytes.Replace(bufBytes,
		[]byte{
			0x1a, 0xbb, 0x76, 0xa9, 0x14, 0xc3, 0x98,
		},
		[]byte{
			0x19, 0x76, 0xa9, 0x14, 0xc3, 0x98,
		},
		1)

	// Deserialize the manipulated tx
	rbuf = bytes.NewReader(testGenOutputUntagged)
	err = tx.Deserialize(rbuf)
	if err != nil {
		t.Errorf("Deserialize error %v", err)
	}

	var ssgentestGenOutputUntagged = dcrutil.NewTx(&tx)
	ssgentestGenOutputUntagged.SetTree(wire.TxTreeStake)
	ssgentestGenOutputUntagged.SetIndex(0)

	err = CheckSSGen(ssgentestGenOutputUntagged.MsgTx(), noTreasury)
	if !errors.Is(err, ErrSSGenBadGenOuts) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenBadGenOuts, err)
	}
	if IsSSGen(ssgentestGenOutputUntagged.MsgTx(), noTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// ---------------------------------------------------------------------------
	// Tresury enabled

	// Verify optional OP_RETURN with no discriminator.
	var ssgenNoDiscriminator = dcrutil.NewTx(ssgenMsgTxNoDiscriminator)
	ssgenNoDiscriminator.SetTree(wire.TxTreeStake)
	ssgenNoDiscriminator.SetIndex(0)

	err = CheckSSGen(ssgenNoDiscriminator.MsgTx(), withTreasury)
	if !errors.Is(err, ErrSSGenInvalidDiscriminatorLength) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenInvalidDiscriminatorLength, err)
	}
	if IsSSGen(ssgenNoDiscriminator.MsgTx(), withTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// Verify optional OP_RETURN with an invalid discriminator length.
	var ssgenInvalidDiscriminator = dcrutil.NewTx(ssgenMsgTxInvalidDiscriminator)
	ssgenInvalidDiscriminator.SetTree(wire.TxTreeStake)
	ssgenInvalidDiscriminator.SetIndex(0)

	err = CheckSSGen(ssgenInvalidDiscriminator.MsgTx(), withTreasury)
	if !errors.Is(err, ErrSSGenInvalidDiscriminatorLength) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenInvalidDiscriminatorLength, err)
	}
	if IsSSGen(ssgenInvalidDiscriminator.MsgTx(), withTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// Verify optional OP_RETURN with an unknown discriminator.
	var ssgenInvalidDiscriminator2 = dcrutil.NewTx(ssgenMsgTxUnknownDiscriminator)
	ssgenInvalidDiscriminator2.SetTree(wire.TxTreeStake)
	ssgenInvalidDiscriminator2.SetIndex(0)

	err = CheckSSGen(ssgenInvalidDiscriminator2.MsgTx(), withTreasury)
	if !errors.Is(err, ErrSSGenUnknownDiscriminator) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenUnknownDiscriminator, err)
	}
	if IsSSGen(ssgenInvalidDiscriminator2.MsgTx(), withTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// Verify optional OP_RETURN with an invalid OP_PUSHDATA1.
	var ssgenInvalidDiscriminator3 = dcrutil.NewTx(ssgenMsgTxUnknownDiscriminator2)
	ssgenInvalidDiscriminator3.SetTree(wire.TxTreeStake)
	ssgenInvalidDiscriminator3.SetIndex(0)

	err = CheckSSGen(ssgenInvalidDiscriminator3.MsgTx(), withTreasury)
	if !errors.Is(err, ErrSSGenBadGenOuts) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenBadGenOuts, err)
	}
	if IsSSGen(ssgenInvalidDiscriminator3.MsgTx(), withTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}
	// Verify we don't crash in this case as well.
	_, err = GetSSGenTreasuryVotes(ssgenInvalidDiscriminator3.MsgTx().TxOut[4].PkScript)
	if !errors.Is(err, ErrSSGenInvalidNullScript) {
		t.Error(err)
	}

	// Verify optional OP_RETURN with a valid discriminator but no vote.
	var ssgenInvalidTVNoVote = dcrutil.NewTx(ssgenMsgTxInvalidTV)
	ssgenInvalidTVNoVote.SetTree(wire.TxTreeStake)
	ssgenInvalidTVNoVote.SetIndex(0)

	err = CheckSSGen(ssgenInvalidTVNoVote.MsgTx(), withTreasury)
	if !errors.Is(err, ErrSSGenInvalidTVLength) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenInvalidTVLength, err)
	}
	if IsSSGen(ssgenInvalidTVNoVote.MsgTx(), withTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// Verify optional OP_RETURN with a valid discriminator but a short vote.
	var ssgenInvalidTVNoVote2 = dcrutil.NewTx(ssgenMsgTxInvalidTV2)
	ssgenInvalidTVNoVote2.SetTree(wire.TxTreeStake)
	ssgenInvalidTVNoVote2.SetIndex(0)

	err = CheckSSGen(ssgenInvalidTVNoVote2.MsgTx(), withTreasury)
	if !errors.Is(err, ErrSSGenInvalidTVLength) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenInvalidTVLength, err)
	}
	if IsSSGen(ssgenInvalidTVNoVote2.MsgTx(), withTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// Verify optional OP_RETURN with a valid discriminator one valid vote
	// and a short vote.
	var ssgenInvalidTVNoVote3 = dcrutil.NewTx(ssgenMsgTxInvalidTV3)
	ssgenInvalidTVNoVote3.SetTree(wire.TxTreeStake)
	ssgenInvalidTVNoVote3.SetIndex(0)

	err = CheckSSGen(ssgenInvalidTVNoVote3.MsgTx(), withTreasury)
	if !errors.Is(err, ErrSSGenInvalidTVLength) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenInvalidTVLength, err)
	}
	if IsSSGen(ssgenInvalidTVNoVote3.MsgTx(), withTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// Verify optional OP_RETURN with a valid discriminator 7 valid votes
	// and a short vote.
	var ssgenInvalidTVNoVote4 = dcrutil.NewTx(ssgenMsgTxInvalidTV4)
	ssgenInvalidTVNoVote4.SetTree(wire.TxTreeStake)
	ssgenInvalidTVNoVote4.SetIndex(0)

	err = CheckSSGen(ssgenInvalidTVNoVote4.MsgTx(), withTreasury)
	if !errors.Is(err, ErrSSGenInvalidTVLength) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenInvalidTVLength, err)
	}
	if IsSSGen(ssgenInvalidTVNoVote4.MsgTx(), withTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// Verify optional OP_RETURN with a valid discriminator 7 valid votes
	// but with an invalid OP_PUSHDATAX encoding..
	var ssgenInvalidTVNoVote5 = dcrutil.NewTx(ssgenMsgTxInvalidTV5)
	ssgenInvalidTVNoVote5.SetTree(wire.TxTreeStake)
	ssgenInvalidTVNoVote5.SetIndex(0)

	err = CheckSSGen(ssgenInvalidTVNoVote5.MsgTx(), withTreasury)
	if !errors.Is(err, ErrSSGenInvalidDiscriminatorLength) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenInvalidDiscriminatorLength, err)
	}
	if IsSSGen(ssgenInvalidTVNoVote5.MsgTx(), withTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// Verify invalid treasury vote bits (too many bits).
	var ssgenInvalidTVote = dcrutil.NewTx(ssgenMsgTxInvalidTVote)
	ssgenInvalidTVote.SetTree(wire.TxTreeStake)
	ssgenInvalidTVote.SetIndex(0)

	err = CheckSSGen(ssgenInvalidTVote.MsgTx(), withTreasury)
	if !errors.Is(err, ErrSSGenInvalidTreasuryVote) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenInvalidTreasuryVote, err)
	}
	if IsSSGen(ssgenInvalidTVote.MsgTx(), withTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// Verify invalid treasury vote bits (no bits).
	var ssgenInvalidTVote2 = dcrutil.NewTx(ssgenMsgTxInvalidTVote2)
	ssgenInvalidTVote2.SetTree(wire.TxTreeStake)
	ssgenInvalidTVote2.SetIndex(0)

	err = CheckSSGen(ssgenInvalidTVote2.MsgTx(), withTreasury)
	if !errors.Is(err, ErrSSGenInvalidTreasuryVote) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenInvalidTreasuryVote, err)
	}
	if IsSSGen(ssgenInvalidTVote2.MsgTx(), withTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

	// Verify duplicate tspend hash.
	var ssgenInvalidTVote3 = dcrutil.NewTx(ssgenMsgTxInvalidTVote3)
	ssgenInvalidTVote3.SetTree(wire.TxTreeStake)
	ssgenInvalidTVote3.SetIndex(0)

	err = CheckSSGen(ssgenInvalidTVote3.MsgTx(), withTreasury)
	if !errors.Is(err, ErrSSGenDuplicateTreasuryVote) {
		t.Errorf("CheckSSGen should have returned %v but instead returned %v",
			ErrSSGenDuplicateTreasuryVote, err)
	}
	if IsSSGen(ssgenInvalidTVote3.MsgTx(), withTreasury) {
		t.Errorf("IsSSGen claimed an invalid ssgen is valid")
	}

}

// TestSSGenTreasuryVotes verifies that valid treasury votes return hashes.
func TestSSGenTreasuryVotes(t *testing.T) {
	var ssgenValidVote = dcrutil.NewTx(ssgenMsgTxValid)
	ssgenValidVote.SetVersion(wire.TxVersionTreasury)
	ssgenValidVote.SetTree(wire.TxTreeStake)
	ssgenValidVote.SetIndex(0)

	// Check null data
	lastTxOut := ssgenMsgTxValid.TxOut[len(ssgenMsgTxValid.TxOut)-1]
	if !IsNullDataScript(lastTxOut.Version, lastTxOut.PkScript) {
		t.Fatal("Expected null data script for final output")
	}

	// Make sure ssgen is valid.
	if !IsSSGen(ssgenValidVote.MsgTx(), withTreasury) {
		t.Error("IsSSGen claimed a valid ssgen is invalid")
	}
	err := CheckSSGen(ssgenValidVote.MsgTx(), withTreasury)
	if err != nil {
		t.Error(err)
	}

	// Pull out votes:
	votes, err := GetSSGenTreasuryVotes(lastTxOut.PkScript)
	if err != nil {
		t.Error(err)
	}

	// Verify hash equality.
	expectedHash, err := chainhash.NewHash([]byte{
		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
	})
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range votes {
		expectedHash[0] = byte(k) // Make each hash unique.
		if !expectedHash.IsEqual(&v.Hash) {
			t.Errorf("hash %v not equal. Got %v, wanted %v", k,
				v, expectedHash)
		}
	}
}

// SSRTX TESTING ------------------------------------------------------------------

// TestCheckSSRtx validates that checking a revocation transaction works as
// expected under a variety of conditions.
func TestCheckSSRtx(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		tx                       *wire.MsgTx
		isAutoRevocationsEnabled bool
		wantErr                  error
	}{{
		name: "ok",
		tx:   ssrtxMsgTx,
	}, {
		name: "ok (auto revocations enabled)",
		tx: func() *wire.MsgTx {
			tx := ssrtxMsgTx.Copy()
			tx.TxIn[0].SignatureScript = nil
			return tx
		}(),
		isAutoRevocationsEnabled: true,
	}, {
		name:    "too many inputs",
		tx:      ssrtxMsgTxTooManyInputs,
		wantErr: ErrSSRtxWrongNumInputs,
	}, {
		name:    "too many outputs",
		tx:      ssrtxMsgTxTooManyOutputs,
		wantErr: ErrSSRtxTooManyOutputs,
	}, {
		name: "no outputs",
		tx: func() *wire.MsgTx {
			tx := ssrtxMsgTx.Copy()
			tx.TxOut = nil
			return tx
		}(),
		wantErr: ErrSSRtxNoOutputs,
	}, {
		name:    "invalid script version",
		tx:      ssrtxMsgTxBadVerOut,
		wantErr: ErrSSRtxBadOuts,
	}, {
		name: "input from incorrect tree",
		tx: func() *wire.MsgTx {
			tx := ssrtxMsgTx.Copy()
			tx.TxIn[0].PreviousOutPoint.Tree = wire.TxTreeRegular
			return tx
		}(),
		wantErr: ErrSSRtxWrongTxTree,
	}, {
		name: "output not OP_SSRTX tagged",
		tx: func() *wire.MsgTx {
			tx := ssrtxMsgTx.Copy()
			tx.TxOut[0].PkScript[0] = txscript.OP_SSGEN
			return tx
		}(),
		wantErr: ErrSSRtxBadOuts,
	}, {
		name: "input contains a non-empty signature script (auto revocations " +
			"enabled)",
		tx:                       ssrtxMsgTx,
		isAutoRevocationsEnabled: true,
		wantErr:                  ErrSSRtxInputHasSigScript,
	}, {
		name: "non-zero fee (auto revocations enabled)",
		tx: func() *wire.MsgTx {
			tx := ssrtxMsgTx.Copy()
			tx.TxIn[0].SignatureScript = nil
			outputAmt := int64(0)
			for _, txOut := range tx.TxOut {
				outputAmt += txOut.Value
			}
			tx.TxIn[0].ValueIn = outputAmt + 1
			return tx
		}(),
		isAutoRevocationsEnabled: true,
		wantErr:                  ErrSSRtxInvalidFee,
	}}

	for _, test := range tests {
		// Check if the test transaction is a revocation.
		err := CheckSSRtx(test.tx, test.isAutoRevocationsEnabled)

		// Validate that the expected error was returned for negative tests.
		if test.wantErr != nil {
			if !errors.Is(err, test.wantErr) {
				t.Errorf("%q: mismatched error -- got %T, want %T", test.name, err,
					test.wantErr)
			}
			continue
		}

		// Validate that an unexpected error was not returned.
		if err != nil {
			t.Fatalf("%q: unexpected error checking transaction: %v", test.name, err)
		}
	}
}

// --------------------------------------------------------------------------------
// Minor function testing
func TestGetSSGenBlockVotedOn(t *testing.T) {
	var ssgen = dcrutil.NewTx(ssgenMsgTx)
	ssgen.SetTree(wire.TxTreeStake)
	ssgen.SetIndex(0)

	blockHash, height := SSGenBlockVotedOn(ssgen.MsgTx())

	correctBlockHash, _ := chainhash.NewHash(
		[]byte{
			0x94, 0x8c, 0x76, 0x5a, // 32 byte hash
			0x69, 0x14, 0xd4, 0x3f,
			0x2a, 0x7a, 0xc1, 0x77,
			0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c,
			0xda, 0x2c, 0x2f, 0x6b,
			0x52, 0xde, 0x3d, 0x7c,
			0x52, 0xde, 0x3d, 0x7c,
		})

	correctheight := uint32(0x2123e300)

	if !reflect.DeepEqual(blockHash, *correctBlockHash) {
		t.Errorf("Error thrown on TestGetSSGenBlockVotedOn: Looking for "+
			"hash %v, got hash %v", *correctBlockHash, blockHash)
	}

	if height != correctheight {
		t.Errorf("Error thrown on TestGetSSGenBlockVotedOn: Looking for "+
			"height %v, got height %v", correctheight, height)
	}
}

func TestGetSStxStakeOutputInfo(t *testing.T) {
	var sstx = dcrutil.NewTx(sstxMsgTx)
	sstx.SetTree(wire.TxTreeStake)
	sstx.SetIndex(0)

	correctTyp := true

	correctPkh := []byte{0x94, 0x8c, 0x76, 0x5a, // 20 byte address
		0x69, 0x14, 0xd4, 0x3f,
		0x2a, 0x7a, 0xc1, 0x77,
		0xda, 0x2c, 0x2f, 0x6b,
		0x52, 0xde, 0x3d, 0x7c,
	}

	correctAmt := int64(0x2123e300)

	correctChange := int64(0x2223e300)

	correctRule := true

	correctLimit := uint16(4)

	typs, pkhs, amts, changeAmts, rules, limits :=
		TxSStxStakeOutputInfo(sstx.MsgTx())

	if typs[2] != correctTyp {
		t.Errorf("Error thrown on TestGetSStxStakeOutputInfo: Looking for "+
			"type %v, got type %v", correctTyp, typs[1])
	}

	if !reflect.DeepEqual(pkhs[1], correctPkh) {
		t.Errorf("Error thrown on TestGetSStxStakeOutputInfo: Looking for "+
			"pkh %v, got pkh %v", correctPkh, pkhs[1])
	}

	if amts[1] != correctAmt {
		t.Errorf("Error thrown on TestGetSStxStakeOutputInfo: Looking for "+
			"amount %v, got amount %v", correctAmt, amts[1])
	}

	if changeAmts[1] != correctChange {
		t.Errorf("Error thrown on TestGetSStxStakeOutputInfo: Looking for "+
			"amount %v, got amount %v", correctChange, changeAmts[1])
	}

	if rules[1][0] != correctRule {
		t.Errorf("Error thrown on TestGetSStxStakeOutputInfo: Looking for "+
			"rule %v, got rule %v", correctRule, rules[1][0])
	}

	if limits[1][0] != correctLimit {
		t.Errorf("Error thrown on TestGetSStxStakeOutputInfo: Looking for "+
			"limit %v, got limit %v", correctLimit, rules[1][0])
	}
}

func TestGetSSGenVoteBits(t *testing.T) {
	var ssgen = dcrutil.NewTx(ssgenMsgTx)
	ssgen.SetTree(wire.TxTreeStake)
	ssgen.SetIndex(0)

	correctvbs := uint16(0x8c94)

	votebits := SSGenVoteBits(ssgen.MsgTx())

	if correctvbs != votebits {
		t.Errorf("Error thrown on TestGetSSGenVoteBits: Looking for "+
			"vbs % x, got vbs % x", correctvbs, votebits)
	}
}

func TestGetSSGenVersion(t *testing.T) {
	var ssgen = ssgenMsgTx.Copy()

	missingVersion := uint32(VoteConsensusVersionAbsent)
	version := SSGenVersion(ssgen)
	if version != missingVersion {
		t.Fatalf("Error thrown on TestGetSSGenVersion: Looking for "+
			"version % x, got version % x", missingVersion, version)
	}

	vbBytes := []byte{0x01, 0x00, 0x01, 0xef, 0xcd, 0xab}
	expectedVersion := uint32(0xabcdef01)
	builder := txscript.NewScriptBuilder()
	pkScript, err := builder.AddOp(txscript.OP_RETURN).AddData(vbBytes).Script()
	if err != nil {
		t.Fatalf("error generating vote bits: %v", err)
	}
	ssgen.TxOut[1].PkScript = pkScript
	version = SSGenVersion(ssgen)

	if version != expectedVersion {
		t.Fatalf("Error thrown on TestGetSSGenVersion: Looking for "+
			"version % x, got version % x", expectedVersion, version)
	}
}

func TestGetSStxNullOutputAmounts(t *testing.T) {
	commitAmts := []int64{
		0x2122e300,
		0x12000000,
		0x12300000,
	}
	changeAmts := []int64{
		0x0122e300,
		0x02000000,
		0x02300000,
	}
	amtTicket := int64(0x9122e300)

	_, _, err := SStxNullOutputAmounts(
		[]int64{
			0x12000000,
			0x12300000,
		},
		changeAmts,
		amtTicket)

	// len commit to amts != len change amts
	lenErrStr := "amounts was not equal in length " +
		"to change amounts!"
	if err == nil || err.Error() != lenErrStr {
		t.Errorf("TestGetSStxNullOutputAmounts unexpected error: %v", err)
	}

	// too small amount to commit
	_, _, err = SStxNullOutputAmounts(
		commitAmts,
		changeAmts,
		int64(0x00000000))
	tooSmallErrStr := "committed amount was too small!"
	if err == nil || err.Error() != tooSmallErrStr {
		t.Errorf("TestGetSStxNullOutputAmounts unexpected error: %v", err)
	}

	// overspending error
	tooMuchChangeAmts := []int64{
		0x0122e300,
		0x02000000,
		0x12300001,
	}

	_, _, err = SStxNullOutputAmounts(
		commitAmts,
		tooMuchChangeAmts,
		int64(0x00000020))
	if !errors.Is(err, ErrSStxBadChangeAmts) {
		t.Errorf("TestGetSStxNullOutputAmounts unexpected error: %v", err)
	}

	fees, amts, err := SStxNullOutputAmounts(commitAmts, changeAmts,
		amtTicket)

	if err != nil {
		t.Errorf("TestGetSStxNullOutputAmounts unexpected error: %v", err)
	}

	expectedFees := int64(-1361240832)

	if expectedFees != fees {
		t.Errorf("TestGetSStxNullOutputAmounts error, wanted %v, "+
			"but got %v", expectedFees, fees)
	}

	expectedAmts := []int64{
		0x20000000,
		0x10000000,
		0x10000000,
	}

	if !reflect.DeepEqual(expectedAmts, amts) {
		t.Errorf("TestGetSStxNullOutputAmounts error, wanted %v, "+
			"but got %v", expectedAmts, amts)
	}
}

// TestCalculateRewards ensures that ticket output amounts are calculated
// correctly for votes under a variety of conditions.
func TestCalculateRewards(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		contribAmounts       []int64
		ticketPurchaseAmount int64
		voteSubsidy          int64
		want                 []int64
	}{{
		name: "vote rewards - evenly divisible over all outputs",
		contribAmounts: []int64{
			2500000000,
			2500000000,
			5000000000,
			10000000000,
		},
		ticketPurchaseAmount: 20000000000,
		voteSubsidy:          100000000,
		want: []int64{
			2512500000,
			2512500000,
			5025000000,
			10050000000,
		},
	}, {
		name: "vote rewards - remainder of 2",
		contribAmounts: []int64{
			100000000,
			100000000,
			100000000,
		},
		ticketPurchaseAmount: 300000000,
		voteSubsidy:          300002,
		want: []int64{
			100100000,
			100100000,
			100100000,
		},
	}}

	for _, test := range tests {
		got := CalculateRewards(test.contribAmounts, test.ticketPurchaseAmount,
			test.voteSubsidy)
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%q: unexpected result -- got %v, want %v", test.name, got,
				test.want)
		}
	}
}

// TestCalculateRevocationRewards ensures that ticket output amounts are
// calculated correctly for revocations under a variety of conditions.
func TestCalculateRevocationRewards(t *testing.T) {
	t.Parallel()

	// Default header bytes for tests.
	prevHeaderBytes := hexToBytes("07000000dc02335daa073d293e1b150648f0444a60b9" +
		"c97604abd01e00000000000000003c449b2321c4bd0d1fa76ed59f80ebaf46f16cfb2d17" +
		"ba46948f09f21861095566482410a463ed49473c27278cd7a2a3712a3b19ff1f6225717d" +
		"3eb71cc2b5590100012c7312a3c30500050095a100000cf42418f1820a870300000020a1" +
		"0700091600005b32a55f5bcce31078832100007469943958002e00000000000000000000" +
		"0000000000000000000007000000")

	tests := []struct {
		name                     string
		contribAmounts           []int64
		ticketPurchaseAmount     int64
		prevHeaderBytes          []byte
		isAutoRevocationsEnabled bool
		want                     []int64
	}{{
		name: "revocation rewards - evenly divisible over all outputs (auto " +
			"revocations disabled)",
		contribAmounts: []int64{
			2500000000,
			2500000000,
			5000000000,
			10000000000,
		},
		ticketPurchaseAmount: 20000000000,
		want: []int64{
			2500000000,
			2500000000,
			5000000000,
			10000000000,
		},
	}, {
		name: "revocation rewards - remainder of 4 (auto revocations disabled)",
		contribAmounts: []int64{
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
		},
		ticketPurchaseAmount: 799999996,
		want: []int64{
			99999999,
			99999999,
			99999999,
			99999999,
			99999999,
			99999999,
			99999999,
			99999999,
		},
	}, {
		name: "revocation rewards - evenly divisible over all outputs (auto " +
			"revocations enabled)",
		contribAmounts: []int64{
			2500000000,
			2500000000,
			5000000000,
			10000000000,
		},
		ticketPurchaseAmount:     20000000000,
		prevHeaderBytes:          prevHeaderBytes,
		isAutoRevocationsEnabled: true,
		want: []int64{
			2500000000,
			2500000000,
			5000000000,
			10000000000,
		},
	}, {
		name: "revocation rewards - remainder of 4 (auto revocations enabled)",
		contribAmounts: []int64{
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
			100000000,
		},
		ticketPurchaseAmount:     799999996,
		prevHeaderBytes:          prevHeaderBytes,
		isAutoRevocationsEnabled: true,
		want: []int64{
			99999999,
			100000000,
			99999999,
			99999999,
			99999999,
			99999999,
			100000001,
			100000000,
		},
	}}

	for _, test := range tests {
		got := CalculateRevocationRewards(test.contribAmounts,
			test.ticketPurchaseAmount, test.prevHeaderBytes,
			test.isAutoRevocationsEnabled)
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%q: unexpected result -- got %v, want %v", test.name, got,
				test.want)
		}
	}
}

func TestIsNullDataScript(t *testing.T) {
	var hash160 = stdaddr.Hash160([]byte("test"))
	var overMaxDataCarrierSize = make([]byte, MaxDataCarrierSize+1)
	var underMaxDataCarrierSize = make([]byte, MaxDataCarrierSize/2)
	rand.Read(overMaxDataCarrierSize)
	rand.Read(underMaxDataCarrierSize)

	tests := []struct {
		name         string
		scriptSource *txscript.ScriptBuilder
		version      uint16
		expected     bool
	}{
		{
			name: "OP_RETURN script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_RETURN),
			version:  0,
			expected: true,
		},
		{
			name: "OP_RETURN script with unsupported version",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_RETURN),
			version:  100,
			expected: false,
		},
		{
			name: "OP_RETURN script with data under MaxDataCarrierSize",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_RETURN).AddData(underMaxDataCarrierSize),
			version:  0,
			expected: true,
		},
		{
			name: "OP_RETURN script with data over MaxDataCarrierSize",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_RETURN).AddData(overMaxDataCarrierSize),
			version:  0,
			expected: false,
		},
		{
			name: "revocation-tagged p2pkh script",
			scriptSource: txscript.NewScriptBuilder().
				AddOp(txscript.OP_SSRTX).AddOp(txscript.OP_DUP).
				AddOp(txscript.OP_HASH160).AddData(hash160).
				AddOp(txscript.OP_EQUALVERIFY).AddOp(txscript.OP_CHECKSIG),
			version:  0,
			expected: false,
		},
	}

	for _, test := range tests {
		script, err := test.scriptSource.Script()
		if err != nil {
			t.Fatalf("%s: unexpected script generation error: %s",
				test.name, err)
		}

		result := IsNullDataScript(test.version, script)
		if result != test.expected {
			t.Fatalf("%s: expected %v, got %v", test.name,
				test.expected, result)
		}
	}
}

// TestCreateRevocationFromTicket validates that revocation transactions are
// created correctly under a variety of conditions.
func TestCreateRevocationFromTicket(t *testing.T) {
	t.Parallel()

	// Default network parameters to use for tests.
	params := chaincfg.RegNetParams()

	// Default header bytes for tests.
	prevHeaderBytes := hexToBytes("07000000dc02335daa073d293e1b150648f0444a60b9" +
		"c97604abd01e00000000000000003c449b2321c4bd0d1fa76ed59f80ebaf46f16cfb2d17" +
		"ba46948f09f21861095566482410a463ed49473c27278cd7a2a3712a3b19ff1f6225717d" +
		"3eb71cc2b5590100012c7312a3c30500050095a100000cf42418f1820a870300000020a1" +
		"0700091600005b32a55f5bcce31078832100007469943958002e00000000000000000000" +
		"0000000000000000000007000000")

	// The following variables are derived from a mainnet ticket that was
	// purchased in block 135375 and revoked in block 140709.
	ticketHash := mustParseHash("dad48ac8c59ee97d1a6fd04ad4f1c8392357a6ee78d39f" +
		"fc00fb467a0cdba695")
	ticketOut1 := &MinimalOutput{
		PkScript: hexToBytes("ba76a914097e847d49c6806f6933e806a350f43b97ac70d088a" +
			"c"),
		Value:   4126629682,
		Version: 0,
	}
	ticketOut2 := &MinimalOutput{
		PkScript: hexToBytes("6a1e86c6da62556f5e21fbce3564b7374724d65f0cbb51c66d0" +
			"0000000800058"),
		Value:   0,
		Version: 0,
	}
	ticketOut3 := &MinimalOutput{
		PkScript: hexToBytes("bd76a914000000000000000000000000000000000000000088a" +
			"c"),
		Value:   0,
		Version: 0,
	}
	ticketOut4 := &MinimalOutput{
		PkScript: hexToBytes("6a1e7e8efe653b374ba7ad950c873e483fc53c5bafa9c1c439f" +
			"8000000000058"),
		Value:   0,
		Version: 0,
	}
	ticketOut5 := &MinimalOutput{
		PkScript: hexToBytes("bd76a914000000000000000000000000000000000000000088a" +
			"c"),
		Value:   0,
		Version: 0,
	}
	ticketMinOuts := []*MinimalOutput{
		ticketOut1,
		ticketOut2,
		ticketOut3,
		ticketOut4,
		ticketOut5,
	}
	revocationHash := mustParseHash("46ae5f78174c6c6e3675d0bbfec27e25c40f3a119e" +
		"df9183b96261db5cda7a4f")
	revocationTxFee := dcrutil.Amount(285000)
	revocationTxVersion := uint16(1)

	// With auto revocations enabled.
	autoRevocationsTxHash := mustParseHash("c8999b6e2544f339419cc5416f15e9b942c" +
		"fd6481246012d2da82f4594310e65")
	autoRevocationsTxFee := dcrutil.Amount(0)
	autoRevocationsTxVersion := TxVersionAutoRevocations

	// Invalid script version.
	ticketOutInvalidScriptVersion := &MinimalOutput{
		PkScript: hexToBytes("6a1e86c6da62556f5e21fbce3564b7374724d65f0cbb51c66d0" +
			"0000000800058"),
		Value:   0,
		Version: 1,
	}
	ticketMinOutsInvalidScriptVersion := []*MinimalOutput{
		ticketOut1,
		ticketOutInvalidScriptVersion,
		ticketOut3,
		ticketOut4,
		ticketOut5,
	}

	// Invalid ticket commitment amount.
	ticketOutInvalidCommitmentAmt := &MinimalOutput{
		PkScript: hexToBytes("6a1e86c6da62556f5e21fbce3564b7374724d65f0cbbfffffff" +
			"fffffffff0058"),
		Value:   0,
		Version: 0,
	}
	ticketMinOutsInvalidCommitmentAmt := []*MinimalOutput{
		ticketOut1,
		ticketOutInvalidCommitmentAmt,
		ticketOut3,
		ticketOut4,
		ticketOut5,
	}

	// No fee limit.
	ticketOut2NoFeeLimit := &MinimalOutput{
		PkScript: hexToBytes("6a1e86c6da62556f5e21fbce3564b7374724d65f0cbb51c66d0" +
			"0000000800000"),
		Value:   0,
		Version: 0,
	}
	ticketMinOutsNoFeeLimit := []*MinimalOutput{
		ticketOut1,
		ticketOut2NoFeeLimit,
		ticketOut3,
		ticketOut4,
		ticketOut5,
	}

	tests := []struct {
		name                     string
		ticketHash               *chainhash.Hash
		ticketMinOuts            []*MinimalOutput
		revocationTxFee          dcrutil.Amount
		revocationTxVersion      uint16
		prevHeaderBytes          []byte
		isAutoRevocationsEnabled bool
		wantTxHash               chainhash.Hash
		wantErr                  error
	}{{
		name:                "valid with P2SH and P2PKH outputs",
		ticketHash:          ticketHash,
		ticketMinOuts:       ticketMinOuts,
		revocationTxFee:     revocationTxFee,
		revocationTxVersion: revocationTxVersion,
		prevHeaderBytes:     prevHeaderBytes,
		wantTxHash:          *revocationHash,
	}, {
		name: "valid with P2SH and P2PKH outputs (auto revocations " +
			"enabled",
		ticketHash:               ticketHash,
		ticketMinOuts:            ticketMinOuts,
		revocationTxFee:          autoRevocationsTxFee,
		revocationTxVersion:      autoRevocationsTxVersion,
		prevHeaderBytes:          prevHeaderBytes,
		isAutoRevocationsEnabled: true,
		wantTxHash:               *autoRevocationsTxHash,
	}, {
		name:                "invalid ticket minimal outputs",
		ticketHash:          ticketHash,
		ticketMinOuts:       nil,
		revocationTxFee:     revocationTxFee,
		revocationTxVersion: revocationTxVersion,
		wantErr:             ErrSStxNoOutputs,
	}, {
		name:                "invalid script version",
		ticketHash:          ticketHash,
		ticketMinOuts:       ticketMinOutsInvalidScriptVersion,
		revocationTxFee:     revocationTxFee,
		revocationTxVersion: revocationTxVersion,
		prevHeaderBytes:     prevHeaderBytes,
		wantErr:             ErrSStxInvalidOutputs,
	}, {
		name:                "invalid ticket commitment amount",
		ticketHash:          ticketHash,
		ticketMinOuts:       ticketMinOutsInvalidCommitmentAmt,
		revocationTxFee:     revocationTxFee,
		revocationTxVersion: revocationTxVersion,
		prevHeaderBytes:     prevHeaderBytes,
		wantErr:             ErrSStxBadCommitAmount,
	}, {
		name:                "invalid fee",
		ticketHash:          ticketHash,
		ticketMinOuts:       ticketMinOuts,
		revocationTxFee:     16777217,
		revocationTxVersion: revocationTxVersion,
		wantErr:             ErrSSRtxInvalidFee,
	}, {
		name:                "invalid fee (no fee limit)",
		ticketHash:          ticketHash,
		ticketMinOuts:       ticketMinOutsNoFeeLimit,
		revocationTxFee:     revocationTxFee,
		revocationTxVersion: revocationTxVersion,
		wantErr:             ErrSSRtxInvalidFee,
	}, {
		name:                     "invalid fee (auto revocations enabled)",
		ticketHash:               ticketHash,
		ticketMinOuts:            ticketMinOuts,
		revocationTxFee:          revocationTxFee,
		revocationTxVersion:      revocationTxVersion,
		isAutoRevocationsEnabled: true,
		wantErr:                  ErrSSRtxInvalidFee,
	}, {
		name:                     "invalid tx version (auto revocations enabled)",
		ticketHash:               ticketHash,
		ticketMinOuts:            ticketMinOuts,
		revocationTxFee:          autoRevocationsTxFee,
		revocationTxVersion:      revocationTxVersion,
		isAutoRevocationsEnabled: true,
		wantErr:                  ErrSSRtxInvalidTxVersion,
	}}

	for _, test := range tests {
		// Create a revocation transaction with the given test parameters.
		revocationTx, err := CreateRevocationFromTicket(test.ticketHash,
			test.ticketMinOuts, test.revocationTxFee, test.revocationTxVersion,
			params, test.prevHeaderBytes, test.isAutoRevocationsEnabled)

		// Validate that the expected error was returned for negative tests.
		if test.wantErr != nil {
			if !errors.Is(err, test.wantErr) {
				t.Errorf("%q: mismatched error -- got %T, want %T", test.name, err,
					test.wantErr)
			}
			continue
		}

		// Validate that an unexpected error was not returned.
		if err != nil {
			t.Fatalf("%q: unexpected error creating revocation: %v", test.name, err)
		}

		// Validate that the revocation transaction was created correctly.
		err = CheckSSRtx(revocationTx, test.isAutoRevocationsEnabled)
		if err != nil {
			t.Errorf("%q: unexpected error checking revocation: %v", test.name, err)
			continue
		}

		// Validate the revocation transaction version.
		if revocationTx.Version != test.revocationTxVersion {
			t.Errorf("%q: mismatched tx version -- got %d, want %d", test.name,
				revocationTx.Version, test.revocationTxVersion)
			continue
		}

		// Validate that the resulting revocation transaction hash matches what is
		// expected.
		revocationTxHash := revocationTx.TxHash()
		if revocationTxHash != test.wantTxHash {
			t.Errorf("%q: mismatched tx hash -- got %v, want %v", test.name,
				revocationTxHash, test.wantTxHash)
			continue
		}
	}
}

// --------------------------------------------------------------------------------
// TESTING VARIABLES BEGIN HERE

// sstxTxIn is the first input in the reference valid sstx
var sstxTxIn = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash: chainhash.Hash([32]byte{ // Make go vet happy.
			0x03, 0x2e, 0x38, 0xe9, 0xc0, 0xa8, 0x4c, 0x60,
			0x46, 0xd6, 0x87, 0xd1, 0x05, 0x56, 0xdc, 0xac,
			0xc4, 0x1d, 0x27, 0x5e, 0xc5, 0x5f, 0xc0, 0x07,
			0x79, 0xac, 0x88, 0xfd, 0xf3, 0x57, 0xa1, 0x87,
		}), // 87a157f3fd88ac7907c05fc55e271dc4acdc5605d187d646604ca8c0e9382e03
		Index: 0,
		Tree:  wire.TxTreeRegular,
	},
	SignatureScript: []byte{
		0x49, // OP_DATA_73
		0x30, 0x46, 0x02, 0x21, 0x00, 0xc3, 0x52, 0xd3,
		0xdd, 0x99, 0x3a, 0x98, 0x1b, 0xeb, 0xa4, 0xa6,
		0x3a, 0xd1, 0x5c, 0x20, 0x92, 0x75, 0xca, 0x94,
		0x70, 0xab, 0xfc, 0xd5, 0x7d, 0xa9, 0x3b, 0x58,
		0xe4, 0xeb, 0x5d, 0xce, 0x82, 0x02, 0x21, 0x00,
		0x84, 0x07, 0x92, 0xbc, 0x1f, 0x45, 0x60, 0x62,
		0x81, 0x9f, 0x15, 0xd3, 0x3e, 0xe7, 0x05, 0x5c,
		0xf7, 0xb5, 0xee, 0x1a, 0xf1, 0xeb, 0xcc, 0x60,
		0x28, 0xd9, 0xcd, 0xb1, 0xc3, 0xaf, 0x77, 0x48,
		0x01, // 73-byte signature
		0x41, // OP_DATA_65
		0x04, 0xf4, 0x6d, 0xb5, 0xe9, 0xd6, 0x1a, 0x9d,
		0xc2, 0x7b, 0x8d, 0x64, 0xad, 0x23, 0xe7, 0x38,
		0x3a, 0x4e, 0x6c, 0xa1, 0x64, 0x59, 0x3c, 0x25,
		0x27, 0xc0, 0x38, 0xc0, 0x85, 0x7e, 0xb6, 0x7e,
		0xe8, 0xe8, 0x25, 0xdc, 0xa6, 0x50, 0x46, 0xb8,
		0x2c, 0x93, 0x31, 0x58, 0x6c, 0x82, 0xe0, 0xfd,
		0x1f, 0x63, 0x3f, 0x25, 0xf8, 0x7c, 0x16, 0x1b,
		0xc6, 0xf8, 0xa6, 0x30, 0x12, 0x1d, 0xf2, 0xb3,
		0xd3, // 65-byte pubkey
	},
	Sequence: 0xffffffff,
}

// sstxTxOut0 is the first output in the reference valid sstx
var sstxTxOut0 = wire.TxOut{
	Value:   0x2123e300, // 556000000
	Version: 0x0000,
	PkScript: []byte{
		0xba, // OP_SSTX
		0x76, // OP_DUP
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x88, // OP_EQUALVERIFY
		0xac, // OP_CHECKSIG
	},
}

// sstxTxOut1 is the second output in the reference valid sstx
var sstxTxOut1 = wire.TxOut{
	Value:   0x00000000, // 0
	Version: 0x0000,
	PkScript: []byte{
		0x6a,                   // OP_RETURN
		0x1e,                   // 30 bytes to be pushed
		0x94, 0x8c, 0x76, 0x5a, // 20 byte address
		0x69, 0x14, 0xd4, 0x3f,
		0x2a, 0x7a, 0xc1, 0x77,
		0xda, 0x2c, 0x2f, 0x6b,
		0x52, 0xde, 0x3d, 0x7c,
		0x00, 0xe3, 0x23, 0x21, // Transaction amount
		0x00, 0x00, 0x00, 0x00,
		0x44, 0x3f, // Fee limits
	},
}

// sstxTxOut2 is the third output in the reference valid sstx
var sstxTxOut2 = wire.TxOut{
	Value:   0x2223e300,
	Version: 0x0000,
	PkScript: []byte{
		0xbd, // OP_SSTXCHANGE
		0x76, // OP_DUP
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x88, // OP_EQUALVERIFY
		0xac, // OP_CHECKSIG
	},
}

// sstxTxOut3 is another output in an SStx, this time instruction to pay to
// a P2SH output
var sstxTxOut3 = wire.TxOut{
	Value:   0x00000000, // 0
	Version: 0x0000,
	PkScript: []byte{
		0x6a,                   // OP_RETURN
		0x1e,                   // 30 bytes to be pushed
		0x94, 0x8c, 0x76, 0x5a, // 20 byte address
		0x69, 0x14, 0xd4, 0x3f,
		0x2a, 0x7a, 0xc1, 0x77,
		0xda, 0x2c, 0x2f, 0x6b,
		0x52, 0xde, 0x3d, 0x7c,
		0x00, 0xe3, 0x23, 0x21, // Transaction amount
		0x00, 0x00, 0x00, 0x80, // Last byte flagged
		0x44, 0x3f, // Fee limits
	},
}

// sstxTxOut4 is the another output in the reference valid sstx, and pays change
// to a P2SH address
var sstxTxOut4 = wire.TxOut{
	Value:   0x2223e300,
	Version: 0x0000,
	PkScript: []byte{
		0xbd, // OP_SSTXCHANGE
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x87, // OP_EQUAL
	},
}

// sstxTxOut4VerBad is the third output in the reference valid sstx, with a
// bad version.
var sstxTxOut4VerBad = wire.TxOut{
	Value:   0x2223e300,
	Version: 0x1234,
	PkScript: []byte{
		0xbd, // OP_SSTXCHANGE
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x87, // OP_EQUAL
	},
}

// sstxMsgTx is a valid SStx MsgTx with an input and outputs and is used in various
// tests
var sstxMsgTx = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&sstxTxIn,
		&sstxTxIn,
		&sstxTxIn,
	},
	TxOut: []*wire.TxOut{
		&sstxTxOut0,
		&sstxTxOut1,
		&sstxTxOut2, // emulate change address
		&sstxTxOut1,
		&sstxTxOut2, // emulate change address
		&sstxTxOut3, // P2SH
		&sstxTxOut4, // P2SH change
	},
	LockTime: 0,
	Expiry:   0,
}

// sstxMsgTxExtraInputs is an invalid SStx MsgTx with too many inputs
var sstxMsgTxExtraInput = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
		&sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn, &sstxTxIn,
	},
	TxOut: []*wire.TxOut{
		&sstxTxOut0,
		&sstxTxOut1,
	},
	LockTime: 0,
	Expiry:   0,
}

// sstxMsgTxExtraOutputs is an invalid SStx MsgTx with too many outputs
var sstxMsgTxExtraOutputs = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&sstxTxIn,
	},
	TxOut: []*wire.TxOut{
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1, &sstxTxOut1,
	},
	LockTime: 0,
	Expiry:   0,
}

// sstxMismatchedInsOuts is an invalid SStx MsgTx with too many outputs for the
// number of inputs it has
var sstxMismatchedInsOuts = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&sstxTxIn,
	},
	TxOut: []*wire.TxOut{
		&sstxTxOut0, &sstxTxOut1, &sstxTxOut2, &sstxTxOut1, &sstxTxOut2,
	},
	LockTime: 0,
	Expiry:   0,
}

// sstxBadVersionOut is an invalid SStx MsgTx with an output containing a bad
// version.
var sstxBadVersionOut = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&sstxTxIn,
		&sstxTxIn,
		&sstxTxIn,
	},
	TxOut: []*wire.TxOut{
		&sstxTxOut0,
		&sstxTxOut1,
		&sstxTxOut2,       // emulate change address
		&sstxTxOut1,       // 3
		&sstxTxOut2,       // 4
		&sstxTxOut3,       // 5 P2SH
		&sstxTxOut4VerBad, // 6 P2SH change
	},
	LockTime: 0,
	Expiry:   0,
}

// sstxNullDataMissing is an invalid SStx MsgTx with no address push in the second
// output
var sstxNullDataMissing = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&sstxTxIn,
	},
	TxOut: []*wire.TxOut{
		&sstxTxOut0, &sstxTxOut0, &sstxTxOut2,
	},
	LockTime: 0,
	Expiry:   0,
}

// sstxNullDataMisplaced is an invalid SStx MsgTx that has the commitment and
// change outputs swapped
var sstxNullDataMisplaced = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&sstxTxIn,
	},
	TxOut: []*wire.TxOut{
		&sstxTxOut0, &sstxTxOut2, &sstxTxOut1,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenTxIn0 is the 0th position input in a valid SSGen tx used to test out the
// IsSSGen function
var ssgenTxIn0 = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash:  chainhash.Hash{},
		Index: 0xffffffff,
		Tree:  wire.TxTreeRegular,
	},
	SignatureScript: []byte{
		0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04,
	},
	BlockHeight: wire.NullBlockHeight,
	BlockIndex:  wire.NullBlockIndex,
	Sequence:    0xffffffff,
}

// ssgenTxIn1 is the 1st position input in a valid SSGen tx used to test out the
// IsSSGen function
var ssgenTxIn1 = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash: chainhash.Hash([32]byte{ // Make go vet happy.
			0x03, 0x2e, 0x38, 0xe9, 0xc0, 0xa8, 0x4c, 0x60,
			0x46, 0xd6, 0x87, 0xd1, 0x05, 0x56, 0xdc, 0xac,
			0xc4, 0x1d, 0x27, 0x5e, 0xc5, 0x5f, 0xc0, 0x07,
			0x79, 0xac, 0x88, 0xfd, 0xf3, 0x57, 0xa1, 0x87,
		}), // 87a157f3fd88ac7907c05fc55e271dc4acdc5605d187d646604ca8c0e9382e03
		Index: 0,
		Tree:  wire.TxTreeStake,
	},
	SignatureScript: []byte{
		0x49, // OP_DATA_73
		0x30, 0x46, 0x02, 0x21, 0x00, 0xc3, 0x52, 0xd3,
		0xdd, 0x99, 0x3a, 0x98, 0x1b, 0xeb, 0xa4, 0xa6,
		0x3a, 0xd1, 0x5c, 0x20, 0x92, 0x75, 0xca, 0x94,
		0x70, 0xab, 0xfc, 0xd5, 0x7d, 0xa9, 0x3b, 0x58,
		0xe4, 0xeb, 0x5d, 0xce, 0x82, 0x02, 0x21, 0x00,
		0x84, 0x07, 0x92, 0xbc, 0x1f, 0x45, 0x60, 0x62,
		0x81, 0x9f, 0x15, 0xd3, 0x3e, 0xe7, 0x05, 0x5c,
		0xf7, 0xb5, 0xee, 0x1a, 0xf1, 0xeb, 0xcc, 0x60,
		0x28, 0xd9, 0xcd, 0xb1, 0xc3, 0xaf, 0x77, 0x48,
		0x01, // 73-byte signature
		0x41, // OP_DATA_65
		0x04, 0xf4, 0x6d, 0xb5, 0xe9, 0xd6, 0x1a, 0x9d,
		0xc2, 0x7b, 0x8d, 0x64, 0xad, 0x23, 0xe7, 0x38,
		0x3a, 0x4e, 0x6c, 0xa1, 0x64, 0x59, 0x3c, 0x25,
		0x27, 0xc0, 0x38, 0xc0, 0x85, 0x7e, 0xb6, 0x7e,
		0xe8, 0xe8, 0x25, 0xdc, 0xa6, 0x50, 0x46, 0xb8,
		0x2c, 0x93, 0x31, 0x58, 0x6c, 0x82, 0xe0, 0xfd,
		0x1f, 0x63, 0x3f, 0x25, 0xf8, 0x7c, 0x16, 0x1b,
		0xc6, 0xf8, 0xa6, 0x30, 0x12, 0x1d, 0xf2, 0xb3,
		0xd3, // 65-byte pubkey
	},
	Sequence: 0xffffffff,
}

// ssgenTxOut0 is the 0th position output in a valid SSGen tx used to test out the
// IsSSGen function
var ssgenTxOut0 = wire.TxOut{
	Value:   0x00000000, // 0
	Version: 0x0000,
	PkScript: []byte{
		0x6a,                   // OP_RETURN
		0x24,                   // 36 bytes to be pushed
		0x94, 0x8c, 0x76, 0x5a, // 32 byte hash
		0x69, 0x14, 0xd4, 0x3f,
		0x2a, 0x7a, 0xc1, 0x77,
		0xda, 0x2c, 0x2f, 0x6b,
		0x52, 0xde, 0x3d, 0x7c,
		0xda, 0x2c, 0x2f, 0x6b,
		0x52, 0xde, 0x3d, 0x7c,
		0x52, 0xde, 0x3d, 0x7c,
		0x00, 0xe3, 0x23, 0x21, // 4 byte height
	},
}

// ssgenTxOut1 is the 1st position output in a valid SSGen tx used to test out the
// IsSSGen function
var ssgenTxOut1 = wire.TxOut{
	Value:   0x00000000, // 0
	Version: 0x0000,
	PkScript: []byte{
		0x6a,       // OP_RETURN
		0x02,       // 2 bytes to be pushed
		0x94, 0x8c, // Vote bits
	},
}

// ssgenTxOut2 is the 2nd position output in a valid SSGen tx used to test out the
// IsSSGen function
var ssgenTxOut2 = wire.TxOut{
	Value:   0x2123e300, // 556000000
	Version: 0x0000,
	PkScript: []byte{
		0xbb, // OP_SSGEN
		0x76, // OP_DUP
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x88, // OP_EQUALVERIFY
		0xac, // OP_CHECKSIG
	},
}

// ssgenTxOut3 is a P2SH output
var ssgenTxOut3 = wire.TxOut{
	Value:   0x2123e300, // 556000000
	Version: 0x0000,
	PkScript: []byte{
		0xbb, // OP_SSGEN
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x87, // OP_EQUAL
	},
}

// ssgenTxOut3BadVer is a P2SH output with a bad version.
var ssgenTxOut3BadVer = wire.TxOut{
	Value:   0x2123e300, // 556000000
	Version: 0x0100,
	PkScript: []byte{
		0xbb, // OP_SSGEN
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x87, // OP_EQUAL
	},
}

// ssgenMsgTx is a valid SSGen MsgTx with an input and outputs and is used in
// various testing scenarios
var ssgenMsgTx = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxExtraInput is an invalid SSGen MsgTx with too many inputs
var ssgenMsgTxExtraInput = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxExtraOutputs is an invalid SSGen MsgTx with too many outputs
var ssgenMsgTxExtraOutputs = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
		&ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2, &ssgenTxOut2,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxStakeBaseWrong is an invalid SSGen tx with the stakebase in the wrong
// position
var ssgenMsgTxStakeBaseWrong = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssgenTxIn1,
		&ssgenTxIn0,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxBadVerOut is an invalid SSGen tx that contains an output with a bad
// version
var ssgenMsgTxBadVerOut = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3BadVer,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxWrongZeroethOut is an invalid SSGen tx with the first output being not
// an OP_RETURN push
var ssgenMsgTxWrongZeroethOut = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 0,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut2,
		&ssgenTxOut1,
		&ssgenTxOut0,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxWrongFirstOut is an invalid SSGen tx with the second output being not
// an OP_RETURN push
var ssgenMsgTxWrongFirstOut = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 0,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut2,
		&ssgenTxOut1,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxNoDiscriminator is a valid SSGen MsgTx with inputs/outputs and an
// invalid OP_RETURN that has no discriminator.
var ssgenMsgTxNoDiscriminator = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 0,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3,
		&ssgenTxOutNoDiscriminator,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxInvalidDiscriminator is a valid SSGen MsgTx with inputs/outputs
// and an invalid OP_RETURN that has an invalid discriminator.
var ssgenMsgTxInvalidDiscriminator = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 0,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3,
		&ssgenTxOutInvalidDiscriminator,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxUnknownDiscriminator is a valid SSGen MsgTx with inputs/outputs
// and an invalid OP_RETURN that has an unknown discriminator.
var ssgenMsgTxUnknownDiscriminator = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 0,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3,
		&ssgenTxOutUnknownDiscriminator,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxUnknownDiscriminator2 is a valid SSGen MsgTx with inputs/outputs
// and an invalid OP_RETURN that is missing a byte at the end.
var ssgenMsgTxUnknownDiscriminator2 = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 0,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3,
		&ssgenTxOutUnknownDiscriminator2,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxInvalidTV is a valid SSGen MsgTx with inputs/outputs and a
// valid OP_RETURN followed by 'T','V' but has no votes.
var ssgenMsgTxInvalidTV = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 0,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3,
		&ssgenTxOutInvalidTV,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxInvalidTV2 is a valid SSGen MsgTx with inputs/outputs and a
// valid OP_RETURN followed by 'T','V' but has a short vote.
var ssgenMsgTxInvalidTV2 = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 0,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3,
		&ssgenTxOutInvalidTV2,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxInvalidTV3 is a valid SSGen MsgTx with inputs/outputs and a valid
// OP_RETURN followed by 'T','V' but has one valid and one short vote.
var ssgenMsgTxInvalidTV3 = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 0,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3,
		&ssgenTxOutInvalidTV3,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxInvalidTV4 is a valid SSGen MsgTx with inputs/outputs and a valid
// OP_RETURN followed by 'T','V' but has seven valid and one short vote.
var ssgenMsgTxInvalidTV4 = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 0,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3,
		&ssgenTxOutInvalidTV4,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxInvalidTV5 is a valid SSGen MsgTx with inputs/outputs and a valid
// OP_RETURN followed by 'T','V' but has invalid OP_PUSHDATAX encoding.
var ssgenMsgTxInvalidTV5 = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 0,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3,
		&ssgenTxOutInvalidTV5,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxInvalidTVote is a valid SSGen MsgTx with inputs/outputs and a
// valid OP_RETURN followed by 'T','V' but has an invalid vote.
var ssgenMsgTxInvalidTVote = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 0,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3,
		&ssgenTxOutInvalidTVote,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxInvalidTVote2 is a valid SSGen MsgTx with inputs/outputs and a
// valid OP_RETURN followed by 'T','V' but has an invalid vote.
var ssgenMsgTxInvalidTVote2 = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 0,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3,
		&ssgenTxOutInvalidTVote2,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxInvalidTVote3 is a valid SSGen MsgTx with inputs/outputs and a
// valid OP_RETURN followed by 'T','V' but has duplicate votes.
var ssgenMsgTxInvalidTVote3 = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 0,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3,
		&ssgenTxOutInvalidTVote3,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssgenMsgTxValid is a valid SSGen MsgTx with inputs/outputs and a valid
// OP_RETURN followed by 'T','V' that has 7 valid votes.
var ssgenMsgTxValid = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 0,
	TxIn: []*wire.TxIn{
		&ssgenTxIn0,
		&ssgenTxIn1,
	},
	TxOut: []*wire.TxOut{
		&ssgenTxOut0,
		&ssgenTxOut1,
		&ssgenTxOut2,
		&ssgenTxOut3,
		&ssgenTxOutValidTV,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssrtxTxIn is the 0th position input in a valid SSRtx tx used to test out the
// IsSSRtx function
var ssrtxTxIn = wire.TxIn{
	PreviousOutPoint: wire.OutPoint{
		Hash: chainhash.Hash([32]byte{ // Make go vet happy.
			0x03, 0x2e, 0x38, 0xe9, 0xc0, 0xa8, 0x4c, 0x60,
			0x46, 0xd6, 0x87, 0xd1, 0x05, 0x56, 0xdc, 0xac,
			0xc4, 0x1d, 0x27, 0x5e, 0xc5, 0x5f, 0xc0, 0x07,
			0x79, 0xac, 0x88, 0xfd, 0xf3, 0x57, 0xa1, 0x87,
		}), // 87a157f3fd88ac7907c05fc55e271dc4acdc5605d187d646604ca8c0e9382e03
		Index: 0,
		Tree:  wire.TxTreeStake,
	},
	SignatureScript: []byte{
		0x49, // OP_DATA_73
		0x30, 0x46, 0x02, 0x21, 0x00, 0xc3, 0x52, 0xd3,
		0xdd, 0x99, 0x3a, 0x98, 0x1b, 0xeb, 0xa4, 0xa6,
		0x3a, 0xd1, 0x5c, 0x20, 0x92, 0x75, 0xca, 0x94,
		0x70, 0xab, 0xfc, 0xd5, 0x7d, 0xa9, 0x3b, 0x58,
		0xe4, 0xeb, 0x5d, 0xce, 0x82, 0x02, 0x21, 0x00,
		0x84, 0x07, 0x92, 0xbc, 0x1f, 0x45, 0x60, 0x62,
		0x81, 0x9f, 0x15, 0xd3, 0x3e, 0xe7, 0x05, 0x5c,
		0xf7, 0xb5, 0xee, 0x1a, 0xf1, 0xeb, 0xcc, 0x60,
		0x28, 0xd9, 0xcd, 0xb1, 0xc3, 0xaf, 0x77, 0x48,
		0x01, // 73-byte signature
		0x41, // OP_DATA_65
		0x04, 0xf4, 0x6d, 0xb5, 0xe9, 0xd6, 0x1a, 0x9d,
		0xc2, 0x7b, 0x8d, 0x64, 0xad, 0x23, 0xe7, 0x38,
		0x3a, 0x4e, 0x6c, 0xa1, 0x64, 0x59, 0x3c, 0x25,
		0x27, 0xc0, 0x38, 0xc0, 0x85, 0x7e, 0xb6, 0x7e,
		0xe8, 0xe8, 0x25, 0xdc, 0xa6, 0x50, 0x46, 0xb8,
		0x2c, 0x93, 0x31, 0x58, 0x6c, 0x82, 0xe0, 0xfd,
		0x1f, 0x63, 0x3f, 0x25, 0xf8, 0x7c, 0x16, 0x1b,
		0xc6, 0xf8, 0xa6, 0x30, 0x12, 0x1d, 0xf2, 0xb3,
		0xd3, // 65-byte pubkey
	},
	Sequence: 0xffffffff,
}

// ssrtxTxOut is the 0th position output in a valid SSRtx tx used to test out the
// IsSSRtx function
var ssrtxTxOut = wire.TxOut{
	Value:   0x2122e300,
	Version: 0x0000,
	PkScript: []byte{
		0xbc, // OP_SSGEN
		0x76, // OP_DUP
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x33,
		0x88, // OP_EQUALVERIFY
		0xac, // OP_CHECKSIG
	},
}

// ssrtxTxOut2 is a P2SH output
var ssrtxTxOut2 = wire.TxOut{
	Value:   0x2123e300, // 556000000
	Version: 0x0000,
	PkScript: []byte{
		0xbc, // OP_SSRTX
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x87, // OP_EQUAL
	},
}

// ssrtxTxOut2BadVer is a P2SH output with a non-default script version
var ssrtxTxOut2BadVer = wire.TxOut{
	Value:   0x2123e300, // 556000000
	Version: 0x0100,
	PkScript: []byte{
		0xbc, // OP_SSRTX
		0xa9, // OP_HASH160
		0x14, // OP_DATA_20
		0xc3, 0x98, 0xef, 0xa9,
		0xc3, 0x92, 0xba, 0x60,
		0x13, 0xc5, 0xe0, 0x4e,
		0xe7, 0x29, 0x75, 0x5e,
		0xf7, 0xf5, 0x8b, 0x32,
		0x87, // OP_EQUAL
	},
}

// ssgenTxOutNoDiscriminator is an OP_RETURN with no treasury vote
// discriminator.
var ssgenTxOutNoDiscriminator = wire.TxOut{
	Value:   0x2122e300,
	Version: 0x0000,
	PkScript: []byte{
		0x6a, // OP_RETURN
	},
}

// ssgenTxOutInvalidDiscriminator is an OP_RETURN with an invalid treasury vote
// discriminator length.
var ssgenTxOutInvalidDiscriminator = wire.TxOut{
	Value:   0x2122e300,
	Version: 0x0000,
	PkScript: []byte{
		0x6a, // OP_RETURN
		0x01, // OP_DATA_1, invalid length
		'T',
	},
}

// ssgenTxOutUnknownDiscriminator is an OP_RETURN with an unknown treasury vote
// discriminator.
var ssgenTxOutUnknownDiscriminator = wire.TxOut{
	Value:   0x2122e300,
	Version: 0x0000,
	PkScript: []byte{
		0x6a, // OP_RETURN
		0x02, // OP_DATA_2
		'T',  // Treasury
		0x0,  // Should've been 'V'
	},
}

var ssgenTxOutUnknownDiscriminator2 = wire.TxOut{
	Value:   0x2122e300,
	Version: 0x0000,
	PkScript: []byte{
		0x6a, // OP_RETURN
		0x4c, // OP_PUSHDATA1
		0x02,
		'T', // Treasury
		// Missing 'V'
	},
}

// ssgenTxOutInvalidTV is an OP_RETURN with a valid treasury vote
// discriminator but without an actual vote.
var ssgenTxOutInvalidTV = wire.TxOut{
	Value:   0x2122e300,
	Version: 0x0000,
	PkScript: []byte{
		0x6a, // OP_RETURN
		0x02, // OP_DATA_2
		'T',  // Treasury
		'V',  // Vote
	},
}

// ssgenTxOutInvalidTV2 is an OP_RETURN with a valid treasury vote
// discriminator but with a short vote.
var ssgenTxOutInvalidTV2 = wire.TxOut{
	Value:   0x2122e300,
	Version: 0x0000,
	PkScript: []byte{
		0x6a, // OP_RETURN
		0x03, // OP_DATA_3
		'T',  // Treasury
		'V',  // Vote
		0x00, // Start of vote
	},
}

// ssgenTxOutInvalidTV3 is an OP_RETURN with a valid treasury vote
// discriminator but with one valid vote and one short vote.
var ssgenTxOutInvalidTV3 = wire.TxOut{
	Value:   0x2122e300,
	Version: 0x0000,
	PkScript: []byte{
		0x6a, // OP_RETURN
		0x24, // OP_DATA_36
		'T',  // Treasury
		'V',  // Vote

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No
		0x00, // short vote
	},
}

// ssgenTxOutInvalidTV4 is an OP_RETURN with a valid treasury vote
// discriminator but with 7 valid votes and one short vote.
var ssgenTxOutInvalidTV4 = wire.TxOut{
	Value:   0x2122e300,
	Version: 0x0000,
	PkScript: []byte{
		0x6a, // OP_RETURN
		0x4c, // OP_PUSHDATA1
		234,  // 234 bytes
		'T',  // Treasury
		'V',  // Vote

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, // short vote
	},
}

// ssgenTxOutInvalidTV5 is an OP_RETURN with a valid treasury vote
// discriminator but with 7 valid votes but encoded with OP_PUSHDATA2.
var ssgenTxOutInvalidTV5 = wire.TxOut{
	Value:   0x2122e300,
	Version: 0x0000,
	PkScript: []byte{
		0x6a, // OP_RETURN
		0x4d, // OP_PUSHDATA2
		233,  // Little endian 233 bytes
		0,    // 0 bytes
		'T',  // Treasury
		'V',  // Vote

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No
	},
}

// ssgenTxOutInvalidTVote is an OP_RETURN with an invalid treasury vote.
var ssgenTxOutInvalidTVote = wire.TxOut{
	Value:   0x2122e300,
	Version: 0x0000,
	PkScript: []byte{
		0x6a, // OP_RETURN
		0x4c, // OP_PUSHDATA1
		233,  // Little endian 233 bytes
		'T',  // Treasury
		'V',  // Vote

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x01, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x02, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x03, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x04, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x05, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x06, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x03, // Vote bits, Invalid
	},
}

// ssgenTxOutInvalidTVote2 is an OP_RETURN with an invalid treasury vote.
var ssgenTxOutInvalidTVote2 = wire.TxOut{
	Value:   0x2122e300,
	Version: 0x0000,
	PkScript: []byte{
		0x6a, // OP_RETURN
		0x4c, // OP_PUSHDATA1
		233,  // Little endian 233 bytes
		'T',  // Treasury
		'V',  // Vote

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x01, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x02, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x03, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x04, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x05, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x06, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x04, // Vote bits, Invalid
	},
}

// ssgenTxOutInvalidTVote3 is an OP_RETURN with duplicate treasury votes.
var ssgenTxOutInvalidTVote3 = wire.TxOut{
	Value:   0x2122e300,
	Version: 0x0000,
	PkScript: []byte{
		0x6a, // OP_RETURN
		0x4c, // OP_PUSHDATA1
		233,  // Little endian 233 bytes
		'T',  // Treasury
		'V',  // Vote

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x02, // Vote bits, No

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x01, // Vote bits, Yes
	},
}

// ssgenTxOutValidTV is an OP_RETURN with a valid treasury vote discriminator
// with 7 valid votes.
var ssgenTxOutValidTV = wire.TxOut{
	Value:   0x2122e300,
	Version: 0x0000,
	PkScript: []byte{
		0x6a, // OP_RETURN
		0x4c, // OP_PUSHDATA1
		233,  // 233 bytes
		'T',  // Treasury
		'V',  // Vote

		0x00, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x01, // Vote bits, Yes

		0x01, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x01, // Vote bits, Yes

		0x02, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x01, // Vote bits, Yes

		0x03, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x01, // Vote bits, Yes

		0x04, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x01, // Vote bits, Yes

		0x05, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x01, // Vote bits, Yes

		0x06, 0x01, 0x02, 0x04, 0x05, 0x06, 0x07, 0x08, // 32 bytes hash
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x00, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
		0x01, // Vote bits, Yes
	},
}

// ssrtxMsgTx is a valid SSRtx MsgTx with an input and outputs and is used in
// various testing scenarios
var ssrtxMsgTx = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssrtxTxIn,
	},
	TxOut: []*wire.TxOut{
		&ssrtxTxOut,
		&ssrtxTxOut2,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssrtxMsgTx is a valid SSRtx MsgTx with an input and outputs and is used in
// various testing scenarios
var ssrtxMsgTxTooManyInputs = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssrtxTxIn,
		&ssrtxTxIn,
	},
	TxOut: []*wire.TxOut{
		&ssrtxTxOut,
	},
	LockTime: 0,
	Expiry:   0,
}

// ssrtxMsgTx is a valid SSRtx MsgTx with an input and outputs and is used in
// various testing scenarios
var ssrtxMsgTxTooManyOutputs = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssrtxTxIn,
	},
	TxOut: []*wire.TxOut{
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
		&ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut, &ssrtxTxOut,
	},
	LockTime: 0,
	Expiry:   0,
}

var ssrtxMsgTxBadVerOut = &wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		&ssrtxTxIn,
	},
	TxOut: []*wire.TxOut{
		&ssrtxTxOut,
		&ssrtxTxOut2BadVer,
	},
	LockTime: 0,
	Expiry:   0,
}
