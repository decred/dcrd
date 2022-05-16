// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2016-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"errors"
	"testing"

	"github.com/decred/dcrd/blockchain/v5/fullblocktests"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/wire"
)

// fullBlockTestErrToLocalErr converts the provided full block test error kind
// to the associated local blockchain error kind.
func fullBlockTestErrToLocalErr(t *testing.T, kind fullblocktests.ErrorKind) ErrorKind {
	t.Helper()

	switch kind {
	case fullblocktests.ErrDuplicateBlock:
		return ErrDuplicateBlock
	case fullblocktests.ErrBlockTooBig:
		return ErrBlockTooBig
	case fullblocktests.ErrWrongBlockSize:
		return ErrWrongBlockSize
	case fullblocktests.ErrInvalidTime:
		return ErrInvalidTime
	case fullblocktests.ErrTimeTooOld:
		return ErrTimeTooOld
	case fullblocktests.ErrTimeTooNew:
		return ErrTimeTooNew
	case fullblocktests.ErrUnexpectedDifficulty:
		return ErrUnexpectedDifficulty
	case fullblocktests.ErrHighHash:
		return ErrHighHash
	case fullblocktests.ErrBadMerkleRoot:
		return ErrBadMerkleRoot
	case fullblocktests.ErrNoTransactions:
		return ErrNoTransactions
	case fullblocktests.ErrNoTxInputs:
		return ErrNoTxInputs
	case fullblocktests.ErrNoTxOutputs:
		return ErrNoTxOutputs
	case fullblocktests.ErrBadTxOutValue:
		return ErrBadTxOutValue
	case fullblocktests.ErrDuplicateTxInputs:
		return ErrDuplicateTxInputs
	case fullblocktests.ErrBadTxInput:
		return ErrBadTxInput
	case fullblocktests.ErrMissingTxOut:
		return ErrMissingTxOut
	case fullblocktests.ErrUnfinalizedTx:
		return ErrUnfinalizedTx
	case fullblocktests.ErrDuplicateTx:
		return ErrDuplicateTx
	case fullblocktests.ErrImmatureSpend:
		return ErrImmatureSpend
	case fullblocktests.ErrSpendTooHigh:
		return ErrSpendTooHigh
	case fullblocktests.ErrTooManySigOps:
		return ErrTooManySigOps
	case fullblocktests.ErrFirstTxNotCoinbase:
		return ErrFirstTxNotCoinbase
	case fullblocktests.ErrCoinbaseHeight:
		return ErrCoinbaseHeight
	case fullblocktests.ErrMultipleCoinbases:
		return ErrMultipleCoinbases
	case fullblocktests.ErrStakeTxInRegularTree:
		return ErrStakeTxInRegularTree
	case fullblocktests.ErrRegTxInStakeTree:
		return ErrRegTxInStakeTree
	case fullblocktests.ErrBadCoinbaseScriptLen:
		return ErrBadCoinbaseScriptLen
	case fullblocktests.ErrBadCoinbaseValue:
		return ErrBadCoinbaseValue
	case fullblocktests.ErrBadCoinbaseFraudProof:
		return ErrBadCoinbaseFraudProof
	case fullblocktests.ErrBadCoinbaseAmountIn:
		return ErrBadCoinbaseAmountIn
	case fullblocktests.ErrBadStakebaseAmountIn:
		return ErrBadStakebaseAmountIn
	case fullblocktests.ErrBadStakebaseScriptLen:
		return ErrBadStakebaseScriptLen
	case fullblocktests.ErrBadStakebaseScrVal:
		return ErrBadStakebaseScrVal
	case fullblocktests.ErrScriptMalformed:
		return ErrScriptMalformed
	case fullblocktests.ErrScriptValidation:
		return ErrScriptValidation
	case fullblocktests.ErrNotEnoughStake:
		return ErrNotEnoughStake
	case fullblocktests.ErrStakeBelowMinimum:
		return ErrStakeBelowMinimum
	case fullblocktests.ErrNotEnoughVotes:
		return ErrNotEnoughVotes
	case fullblocktests.ErrTooManyVotes:
		return ErrTooManyVotes
	case fullblocktests.ErrFreshStakeMismatch:
		return ErrFreshStakeMismatch
	case fullblocktests.ErrInvalidEarlyStakeTx:
		return ErrInvalidEarlyStakeTx
	case fullblocktests.ErrTicketUnavailable:
		return ErrTicketUnavailable
	case fullblocktests.ErrVotesOnWrongBlock:
		return ErrVotesOnWrongBlock
	case fullblocktests.ErrVotesMismatch:
		return ErrVotesMismatch
	case fullblocktests.ErrIncongruentVotebit:
		return ErrIncongruentVotebit
	case fullblocktests.ErrInvalidSSRtx:
		return ErrInvalidSSRtx
	case fullblocktests.ErrRevocationsMismatch:
		return ErrRevocationsMismatch
	case fullblocktests.ErrTicketCommitment:
		return ErrTicketCommitment
	case fullblocktests.ErrBadNumPayees:
		return ErrBadNumPayees
	case fullblocktests.ErrMismatchedPayeeHash:
		return ErrMismatchedPayeeHash
	case fullblocktests.ErrBadPayeeValue:
		return ErrBadPayeeValue
	case fullblocktests.ErrTxSStxOutSpend:
		return ErrTxSStxOutSpend
	case fullblocktests.ErrRegTxCreateStakeOut:
		return ErrRegTxCreateStakeOut
	case fullblocktests.ErrInvalidFinalState:
		return ErrInvalidFinalState
	case fullblocktests.ErrPoolSize:
		return ErrPoolSize
	case fullblocktests.ErrBadBlockHeight:
		return ErrBadBlockHeight
	case fullblocktests.ErrBlockOneOutputs:
		return ErrBlockOneOutputs
	case fullblocktests.ErrNoTreasury:
		return ErrNoTreasury
	case fullblocktests.ErrExpiredTx:
		return ErrExpiredTx
	case fullblocktests.ErrFraudAmountIn:
		return ErrFraudAmountIn
	case fullblocktests.ErrFraudBlockHeight:
		return ErrFraudBlockHeight
	case fullblocktests.ErrFraudBlockIndex:
		return ErrFraudBlockIndex
	case fullblocktests.ErrInvalidEarlyVoteBits:
		return ErrInvalidEarlyVoteBits
	case fullblocktests.ErrInvalidEarlyFinalState:
		return ErrInvalidEarlyFinalState
	default:
		t.Fatalf("unconverted fullblocktest error kind %v", kind)
	}

	panic("unreachable")
}

// TestFullBlocks ensures all tests generated by the fullblocktests package
// have the expected result when processed via ProcessBlock.
func TestFullBlocks(t *testing.T) {
	tests, err := fullblocktests.Generate(false)
	if err != nil {
		t.Fatalf("failed to generate tests: %v", err)
	}

	// Create a new database and chain instance to run tests against.
	chain, err := chainSetup(t, chaincfg.RegNetParams())
	if err != nil {
		t.Fatalf("Failed to setup chain instance: %v", err)
	}

	// testAcceptedBlock attempts to process the block in the provided test
	// instance and ensures that it was accepted according to the flags
	// specified in the test.
	testAcceptedBlock := func(item fullblocktests.AcceptedBlock) {
		blockHeight := item.Block.Header.Height
		block := dcrutil.NewBlock(item.Block)
		t.Logf("Testing block %s (hash %s, height %d)",
			item.Name, block.Hash(), blockHeight)

		var isOrphan bool
		forkLen, err := chain.ProcessBlock(block)
		if errors.Is(err, ErrMissingParent) {
			isOrphan = true
			err = nil
		}
		if err != nil {
			t.Fatalf("block %q (hash %s, height %d) should have "+
				"been accepted: %v", item.Name, block.Hash(),
				blockHeight, err)
		}

		// Ensure the main chain and orphan flags match the values
		// specified in the test.
		isMainChain := !isOrphan && forkLen == 0
		if isMainChain != item.IsMainChain {
			t.Fatalf("block %q (hash %s, height %d) unexpected main "+
				"chain flag -- got %v, want %v", item.Name,
				block.Hash(), blockHeight, isMainChain,
				item.IsMainChain)
		}
		if isOrphan != item.IsOrphan {
			t.Fatalf("block %q (hash %s, height %d) unexpected "+
				"orphan flag -- got %v, want %v", item.Name,
				block.Hash(), blockHeight, isOrphan,
				item.IsOrphan)
		}
	}

	// testRejectedBlock attempts to process the block in the provided test
	// instance and ensures that it was rejected with the reject code
	// specified in the test.
	testRejectedBlock := func(item fullblocktests.RejectedBlock) {
		blockHeight := item.Block.Header.Height
		block := dcrutil.NewBlock(item.Block)
		t.Logf("Testing block %s (hash %s, height %d)",
			item.Name, block.Hash(), blockHeight)

		_, err := chain.ProcessBlock(block)
		if err == nil {
			t.Fatalf("block %q (hash %s, height %d) should not "+
				"have been accepted", item.Name, block.Hash(),
				blockHeight)
		}

		// Convert the full block test error kind to the associated local
		// blockchain error kind.
		wantRejectKind := fullBlockTestErrToLocalErr(t, item.RejectKind)

		// Ensure the error reject kind matches the value specified in the test
		// instance.
		if !errors.Is(err, wantRejectKind) {
			t.Fatalf("block %q (hash %s, height %d) does not have "+
				"expected reject code -- got %v, want %v",
				item.Name, block.Hash(), blockHeight, err, wantRejectKind)
		}
	}

	// testRejectedNonCanonicalBlock attempts to decode the block in the
	// provided test instance and ensures that it failed to decode with a
	// message error.
	testRejectedNonCanonicalBlock := func(item fullblocktests.RejectedNonCanonicalBlock) {
		headerLen := wire.MaxBlockHeaderPayload
		if headerLen > len(item.RawBlock) {
			headerLen = len(item.RawBlock)
		}
		blockHeader := item.RawBlock[0:headerLen]
		blockHash := chainhash.HashH(chainhash.HashB(blockHeader))
		blockHeight := item.Height
		t.Logf("Testing block %s (hash %s, height %d)", item.Name,
			blockHash, blockHeight)

		// Ensure there is an error due to deserializing the block.
		var msgBlock wire.MsgBlock
		err := msgBlock.BtcDecode(bytes.NewReader(item.RawBlock), 0)
		var werr *wire.MessageError
		if !errors.As(err, &werr) {
			t.Fatalf("block %q (hash %s, height %d) should have "+
				"failed to decode", item.Name, blockHash,
				blockHeight)
		}
	}

	// testOrphanOrRejectedBlock attempts to process the block in the
	// provided test instance and ensures that it was either accepted as an
	// orphan or rejected with a rule violation.
	testOrphanOrRejectedBlock := func(item fullblocktests.OrphanOrRejectedBlock) {
		blockHeight := item.Block.Header.Height
		block := dcrutil.NewBlock(item.Block)
		t.Logf("Testing block %s (hash %s, height %d)",
			item.Name, block.Hash(), blockHeight)

		_, err := chain.ProcessBlock(block)
		if err != nil {
			// Ensure the error is of the expected type.  Note that orphans are
			// rejected with ErrMissingParent, so this check covers both
			// conditions.
			var rerr RuleError
			if !errors.As(err, &rerr) {
				t.Fatalf("block %q (hash %s, height %d) "+
					"returned unexpected error type -- "+
					"got %T, want blockchain.RuleError",
					item.Name, block.Hash(), blockHeight,
					err)
			}
		}
	}

	// testExpectedTip ensures the current tip of the blockchain is the
	// block specified in the provided test instance.
	testExpectedTip := func(item fullblocktests.ExpectedTip) {
		blockHeight := item.Block.Header.Height
		block := dcrutil.NewBlock(item.Block)
		t.Logf("Testing tip for block %s (hash %s, height %d)",
			item.Name, block.Hash(), blockHeight)

		// Ensure hash and height match.
		best := chain.BestSnapshot()
		if best.Hash != item.Block.BlockHash() ||
			best.Height != int64(blockHeight) {

			t.Fatalf("block %q (hash %s, height %d) should be "+
				"the current tip -- got (hash %s, height %d)",
				item.Name, block.Hash(), blockHeight, best.Hash,
				best.Height)
		}
	}

	for testNum, test := range tests {
		for itemNum, item := range test {
			switch item := item.(type) {
			case fullblocktests.AcceptedBlock:
				testAcceptedBlock(item)
			case fullblocktests.RejectedBlock:
				testRejectedBlock(item)
			case fullblocktests.RejectedNonCanonicalBlock:
				testRejectedNonCanonicalBlock(item)
			case fullblocktests.OrphanOrRejectedBlock:
				testOrphanOrRejectedBlock(item)
			case fullblocktests.ExpectedTip:
				testExpectedTip(item)
			default:
				t.Fatalf("test #%d, item #%d is not one of "+
					"the supported test instance types -- "+
					"got type: %T", testNum, itemNum, item)
			}
		}
	}
}
