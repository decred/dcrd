// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package fullblocktests

import (
	"errors"
	"io"
	"testing"
)

// TestErrorKindStringer tests the stringized output for the ErrorKind type.
func TestErrorKindStringer(t *testing.T) {
	tests := []struct {
		in   ErrorKind
		want string
	}{
		{ErrDuplicateBlock, "ErrDuplicateBlock"},
		{ErrBlockTooBig, "ErrBlockTooBig"},
		{ErrWrongBlockSize, "ErrWrongBlockSize"},
		{ErrInvalidTime, "ErrInvalidTime"},
		{ErrTimeTooOld, "ErrTimeTooOld"},
		{ErrTimeTooNew, "ErrTimeTooNew"},
		{ErrUnexpectedDifficulty, "ErrUnexpectedDifficulty"},
		{ErrHighHash, "ErrHighHash"},
		{ErrBadMerkleRoot, "ErrBadMerkleRoot"},
		{ErrNoTransactions, "ErrNoTransactions"},
		{ErrNoTxInputs, "ErrNoTxInputs"},
		{ErrNoTxOutputs, "ErrNoTxOutputs"},
		{ErrBadTxOutValue, "ErrBadTxOutValue"},
		{ErrDuplicateTxInputs, "ErrDuplicateTxInputs"},
		{ErrBadTxInput, "ErrBadTxInput"},
		{ErrMissingTxOut, "ErrMissingTxOut"},
		{ErrUnfinalizedTx, "ErrUnfinalizedTx"},
		{ErrDuplicateTx, "ErrDuplicateTx"},
		{ErrImmatureSpend, "ErrImmatureSpend"},
		{ErrSpendTooHigh, "ErrSpendTooHigh"},
		{ErrTooManySigOps, "ErrTooManySigOps"},
		{ErrFirstTxNotCoinbase, "ErrFirstTxNotCoinbase"},
		{ErrCoinbaseHeight, "ErrCoinbaseHeight"},
		{ErrMultipleCoinbases, "ErrMultipleCoinbases"},
		{ErrStakeTxInRegularTree, "ErrStakeTxInRegularTree"},
		{ErrRegTxInStakeTree, "ErrRegTxInStakeTree"},
		{ErrBadCoinbaseScriptLen, "ErrBadCoinbaseScriptLen"},
		{ErrBadCoinbaseValue, "ErrBadCoinbaseValue"},
		{ErrBadCoinbaseFraudProof, "ErrBadCoinbaseFraudProof"},
		{ErrBadCoinbaseAmountIn, "ErrBadCoinbaseAmountIn"},
		{ErrBadStakebaseAmountIn, "ErrBadStakebaseAmountIn"},
		{ErrBadStakebaseScriptLen, "ErrBadStakebaseScriptLen"},
		{ErrBadStakebaseScrVal, "ErrBadStakebaseScrVal"},
		{ErrScriptMalformed, "ErrScriptMalformed"},
		{ErrScriptValidation, "ErrScriptValidation"},
		{ErrNotEnoughStake, "ErrNotEnoughStake"},
		{ErrStakeBelowMinimum, "ErrStakeBelowMinimum"},
		{ErrNotEnoughVotes, "ErrNotEnoughVotes"},
		{ErrTooManyVotes, "ErrTooManyVotes"},
		{ErrFreshStakeMismatch, "ErrFreshStakeMismatch"},
		{ErrInvalidEarlyStakeTx, "ErrInvalidEarlyStakeTx"},
		{ErrTicketUnavailable, "ErrTicketUnavailable"},
		{ErrVotesOnWrongBlock, "ErrVotesOnWrongBlock"},
		{ErrVotesMismatch, "ErrVotesMismatch"},
		{ErrIncongruentVotebit, "ErrIncongruentVotebit"},
		{ErrInvalidSSRtx, "ErrInvalidSSRtx"},
		{ErrRevocationsMismatch, "ErrRevocationsMismatch"},
		{ErrTicketCommitment, "ErrTicketCommitment"},
		{ErrBadNumPayees, "ErrBadNumPayees"},
		{ErrMismatchedPayeeHash, "ErrMismatchedPayeeHash"},
		{ErrBadPayeeValue, "ErrBadPayeeValue"},
		{ErrTxSStxOutSpend, "ErrTxSStxOutSpend"},
		{ErrRegTxCreateStakeOut, "ErrRegTxCreateStakeOut"},
		{ErrInvalidFinalState, "ErrInvalidFinalState"},
		{ErrPoolSize, "ErrPoolSize"},
		{ErrBadBlockHeight, "ErrBadBlockHeight"},
		{ErrBlockOneOutputs, "ErrBlockOneOutputs"},
		{ErrNoTreasury, "ErrNoTreasury"},
		{ErrExpiredTx, "ErrExpiredTx"},
		{ErrFraudAmountIn, "ErrFraudAmountIn"},
		{ErrFraudBlockHeight, "ErrFraudBlockHeight"},
		{ErrFraudBlockIndex, "ErrFraudBlockIndex"},
		{ErrInvalidEarlyVoteBits, "ErrInvalidEarlyVoteBits"},
		{ErrInvalidEarlyFinalState, "ErrInvalidEarlyFinalState"},
	}

	for i, test := range tests {
		result := test.in.Error()
		if result != test.want {
			t.Errorf("#%d: got: %s want: %s", i, result, test.want)
			continue
		}
	}
}

// TestRuleError tests the error output for the RuleError type.
func TestRuleError(t *testing.T) {
	tests := []struct {
		in   RuleError
		want string
	}{{
		RuleError{Description: "duplicate block"},
		"duplicate block",
	}, {
		RuleError{Description: "human-readable error"},
		"human-readable error",
	}}

	for i, test := range tests {
		result := test.in.Error()
		if result != test.want {
			t.Errorf("#%d: got: %s want: %s", i, result, test.want)
			continue
		}
	}
}

// TestErrorKindIsAs ensures both ErrorKind and Error can be identified as being
// a specific error kind via errors.Is and unwrapped via errors.As.
func TestErrorKindIsAs(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		target    error
		wantMatch bool
		wantAs    ErrorKind
	}{{
		name:      "ErrBlockTooBig == ErrBlockTooBig",
		err:       ErrBlockTooBig,
		target:    ErrBlockTooBig,
		wantMatch: true,
		wantAs:    ErrBlockTooBig,
	}, {
		name:      "RuleError.ErrBlockTooBig == ErrBlockTooBig",
		err:       ruleError(ErrBlockTooBig, ""),
		target:    ErrBlockTooBig,
		wantMatch: true,
		wantAs:    ErrBlockTooBig,
	}, {
		name:      "RuleError.ErrBlockTooBig == RuleError.ErrBlockTooBig",
		err:       ruleError(ErrBlockTooBig, ""),
		target:    ruleError(ErrBlockTooBig, ""),
		wantMatch: true,
		wantAs:    ErrBlockTooBig,
	}, {
		name:      "ErrBlockTooBig != ErrWrongBlockSize",
		err:       ErrBlockTooBig,
		target:    ErrWrongBlockSize,
		wantMatch: false,
		wantAs:    ErrBlockTooBig,
	}, {
		name:      "RuleError.ErrBlockTooBig != ErrWrongBlockSize",
		err:       ruleError(ErrBlockTooBig, ""),
		target:    ErrWrongBlockSize,
		wantMatch: false,
		wantAs:    ErrBlockTooBig,
	}, {
		name:      "ErrBlockTooBig != RuleError.ErrWrongBlockSize",
		err:       ErrBlockTooBig,
		target:    ruleError(ErrWrongBlockSize, ""),
		wantMatch: false,
		wantAs:    ErrBlockTooBig,
	}, {
		name:      "RuleError.ErrBlockTooBig != RuleError.ErrWrongBlockSize",
		err:       ruleError(ErrBlockTooBig, ""),
		target:    ruleError(ErrWrongBlockSize, ""),
		wantMatch: false,
		wantAs:    ErrBlockTooBig,
	}, {
		name:      "RuleError.ErrBlockTooBig != io.EOF",
		err:       ruleError(ErrBlockTooBig, ""),
		target:    io.EOF,
		wantMatch: false,
		wantAs:    ErrBlockTooBig,
	}}

	for _, test := range tests {
		// Ensure the error matches or not depending on the expected result.
		result := errors.Is(test.err, test.target)
		if result != test.wantMatch {
			t.Errorf("%s: incorrect error identification -- got %v, want %v",
				test.name, result, test.wantMatch)
			continue
		}

		// Ensure the underlying error kind can be unwrapped and is the expected
		// kind.
		var kind ErrorKind
		if !errors.As(test.err, &kind) {
			t.Errorf("%s: unable to unwrap to error kind", test.name)
			continue
		}
		if kind != test.wantAs {
			t.Errorf("%s: unexpected unwrapped error kind -- got %v, want %v",
				test.name, kind, test.wantAs)
			continue
		}
	}
}
