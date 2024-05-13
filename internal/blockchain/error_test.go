// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

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
		{ErrMissingParent, "ErrMissingParent"},
		{ErrNoBlockData, "ErrNoBlockData"},
		{ErrBlockTooBig, "ErrBlockTooBig"},
		{ErrWrongBlockSize, "ErrWrongBlockSize"},
		{ErrBlockVersionTooOld, "ErrBlockVersionTooOld"},
		{ErrBadStakeVersion, "ErrBadStakeVersion"},
		{ErrInvalidTime, "ErrInvalidTime"},
		{ErrTimeTooOld, "ErrTimeTooOld"},
		{ErrTimeTooNew, "ErrTimeTooNew"},
		{ErrUnexpectedDifficulty, "ErrUnexpectedDifficulty"},
		{ErrHighHash, "ErrHighHash"},
		{ErrBadMerkleRoot, "ErrBadMerkleRoot"},
		{ErrBadCommitmentRoot, "ErrBadCommitmentRoot"},
		{ErrForkTooOld, "ErrForkTooOld"},
		{ErrBadMaxDiffCheckpoint, "ErrBadMaxDiffCheckpoint"},
		{ErrNoTransactions, "ErrNoTransactions"},
		{ErrNoTxInputs, "ErrNoTxInputs"},
		{ErrNoTxOutputs, "ErrNoTxOutputs"},
		{ErrTxTooBig, "ErrTxTooBig"},
		{ErrBadTxOutValue, "ErrBadTxOutValue"},
		{ErrDuplicateTxInputs, "ErrDuplicateTxInputs"},
		{ErrTxVersionTooHigh, "ErrTxVersionTooHigh"},
		{ErrBadTxInput, "ErrBadTxInput"},
		{ErrScriptVersionTooHigh, "ErrScriptVersionTooHigh"},
		{ErrMissingTxOut, "ErrMissingTxOut"},
		{ErrUnfinalizedTx, "ErrUnfinalizedTx"},
		{ErrDuplicateTx, "ErrDuplicateTx"},
		{ErrOverwriteTx, "ErrOverwriteTx"},
		{ErrImmatureSpend, "ErrImmatureSpend"},
		{ErrSpendTooHigh, "ErrSpendTooHigh"},
		{ErrBadFees, "ErrBadFees"},
		{ErrTooManySigOps, "ErrTooManySigOps"},
		{ErrFirstTxNotCoinbase, "ErrFirstTxNotCoinbase"},
		{ErrCoinbaseHeight, "ErrCoinbaseHeight"},
		{ErrMultipleCoinbases, "ErrMultipleCoinbases"},
		{ErrStakeTxInRegularTree, "ErrStakeTxInRegularTree"},
		{ErrRegTxInStakeTree, "ErrRegTxInStakeTree"},
		{ErrBadCoinbaseScriptLen, "ErrBadCoinbaseScriptLen"},
		{ErrBadCoinbaseValue, "ErrBadCoinbaseValue"},
		{ErrBadCoinbaseOutpoint, "ErrBadCoinbaseOutpoint"},
		{ErrBadCoinbaseFraudProof, "ErrBadCoinbaseFraudProof"},
		{ErrBadCoinbaseAmountIn, "ErrBadCoinbaseAmountIn"},
		{ErrBadStakebaseAmountIn, "ErrBadStakebaseAmountIn"},
		{ErrBadStakebaseScriptLen, "ErrBadStakebaseScriptLen"},
		{ErrBadStakebaseScrVal, "ErrBadStakebaseScrVal"},
		{ErrScriptMalformed, "ErrScriptMalformed"},
		{ErrScriptValidation, "ErrScriptValidation"},
		{ErrNotEnoughStake, "ErrNotEnoughStake"},
		{ErrStakeBelowMinimum, "ErrStakeBelowMinimum"},
		{ErrNonstandardStakeTx, "ErrNonstandardStakeTx"},
		{ErrNotEnoughVotes, "ErrNotEnoughVotes"},
		{ErrTooManyVotes, "ErrTooManyVotes"},
		{ErrFreshStakeMismatch, "ErrFreshStakeMismatch"},
		{ErrTooManySStxs, "ErrTooManySStxs"},
		{ErrInvalidEarlyStakeTx, "ErrInvalidEarlyStakeTx"},
		{ErrTicketUnavailable, "ErrTicketUnavailable"},
		{ErrVotesOnWrongBlock, "ErrVotesOnWrongBlock"},
		{ErrVotesMismatch, "ErrVotesMismatch"},
		{ErrIncongruentVotebit, "ErrIncongruentVotebit"},
		{ErrInvalidSSRtx, "ErrInvalidSSRtx"},
		{ErrRevocationsMismatch, "ErrRevocationsMismatch"},
		{ErrTooManyRevocations, "ErrTooManyRevocations"},
		{ErrTicketCommitment, "ErrTicketCommitment"},
		{ErrInvalidVoteInput, "ErrInvalidVoteInput"},
		{ErrBadNumPayees, "ErrBadNumPayees"},
		{ErrBadPayeeScriptVersion, "ErrBadPayeeScriptVersion"},
		{ErrBadPayeeScriptType, "ErrBadPayeeScriptType"},
		{ErrMismatchedPayeeHash, "ErrMismatchedPayeeHash"},
		{ErrBadPayeeValue, "ErrBadPayeeValue"},
		{ErrSSGenSubsidy, "ErrSSGenSubsidy"},
		{ErrImmatureTicketSpend, "ErrImmatureTicketSpend"},
		{ErrTicketInputScript, "ErrTicketInputScript"},
		{ErrInvalidRevokeInput, "ErrInvalidRevokeInput"},
		{ErrTxSStxOutSpend, "ErrTxSStxOutSpend"},
		{ErrRegTxCreateStakeOut, "ErrRegTxCreateStakeOut"},
		{ErrInvalidFinalState, "ErrInvalidFinalState"},
		{ErrPoolSize, "ErrPoolSize"},
		{ErrForceReorgSameBlock, "ErrForceReorgSameBlock"},
		{ErrForceReorgWrongChain, "ErrForceReorgWrongChain"},
		{ErrForceReorgMissingChild, "ErrForceReorgMissingChild"},
		{ErrBadStakebaseValue, "ErrBadStakebaseValue"},
		{ErrStakeFees, "ErrStakeFees"},
		{ErrNoStakeTx, "ErrNoStakeTx"},
		{ErrBadBlockHeight, "ErrBadBlockHeight"},
		{ErrBlockOneTx, "ErrBlockOneTx"},
		{ErrBlockOneInputs, "ErrBlockOneInputs"},
		{ErrBlockOneOutputs, "ErrBlockOneOutputs"},
		{ErrNoTreasury, "ErrNoTreasury"},
		{ErrExpiredTx, "ErrExpiredTx"},
		{ErrExpiryTxSpentEarly, "ErrExpiryTxSpentEarly"},
		{ErrFraudAmountIn, "ErrFraudAmountIn"},
		{ErrFraudBlockHeight, "ErrFraudBlockHeight"},
		{ErrFraudBlockIndex, "ErrFraudBlockIndex"},
		{ErrZeroValueOutputSpend, "ErrZeroValueOutputSpend"},
		{ErrInvalidEarlyVoteBits, "ErrInvalidEarlyVoteBits"},
		{ErrInvalidEarlyFinalState, "ErrInvalidEarlyFinalState"},
		{ErrKnownInvalidBlock, "ErrKnownInvalidBlock"},
		{ErrInvalidAncestorBlock, "ErrInvalidAncestorBlock"},
		{ErrInvalidTemplateParent, "ErrInvalidTemplateParent"},
		{ErrUnknownPiKey, "ErrUnknownPiKey"},
		{ErrInvalidPiSignature, "ErrInvalidPiSignature"},
		{ErrInvalidTVoteWindow, "ErrInvalidTVoteWindow"},
		{ErrNotTVI, "ErrNotTVI"},
		{ErrInvalidTSpendWindow, "ErrInvalidTSpendWindow"},
		{ErrNotEnoughTSpendVotes, "ErrNotEnoughTSpendVotes"},
		{ErrInvalidTSpendValueIn, "ErrInvalidTSpendValueIn"},
		{ErrTSpendExists, "ErrTSpendExists"},
		{ErrInvalidExpenditure, "ErrInvalidExpenditure"},
		{ErrFirstTxNotTreasurybase, "ErrFirstTxNotTreasurybase"},
		{ErrBadTreasurybaseOutpoint, "ErrBadTreasurybaseOutpoint"},
		{ErrBadTreasurybaseFraudProof, "ErrBadTreasurybaseFraudProof"},
		{ErrBadTreasurybaseScriptLen, "ErrBadTreasurybaseScriptLen"},
		{ErrTreasurybaseTxNotOpReturn, "ErrTreasurybaseTxNotOpReturn"},
		{ErrTreasurybaseHeight, "ErrTreasurybaseHeight"},
		{ErrMultipleTreasurybases, "ErrMultipleTreasurybases"},
		{ErrInvalidTreasurybaseTxOutputs, "ErrInvalidTreasurybaseTxOutputs"},
		{ErrInvalidTreasurybaseVersion, "ErrInvalidTreasurybaseVersion"},
		{ErrInvalidTreasurybaseScript, "ErrInvalidTreasurybaseScript"},
		{ErrMultipleTreasurybases, "ErrMultipleTreasurybases"},
		{ErrBadTreasurybaseAmountIn, "ErrBadTreasurybaseAmountIn"},
		{ErrBadTSpendOutpoint, "ErrBadTSpendOutpoint"},
		{ErrBadTSpendFraudProof, "ErrBadTSpendFraudProof"},
		{ErrBadTSpendScriptLen, "ErrBadTSpendScriptLen"},
		{ErrInvalidTAddChange, "ErrInvalidTAddChange"},
		{ErrTooManyTAdds, "ErrTooManyTAdds"},
		{ErrTicketExhaustion, "ErrTicketExhaustion"},
		{ErrDBTooOldToUpgrade, "ErrDBTooOldToUpgrade"},
		{ErrUnknownBlock, "ErrUnknownBlock"},
		{ErrNoFilter, "ErrNoFilter"},
		{ErrNoTreasuryBalance, "ErrNoTreasuryBalance"},
		{ErrInvalidateGenesisBlock, "ErrInvalidateGenesisBlock"},
		{ErrSerializeHeader, "ErrSerializeHeader"},
		{ErrNotAnAncestor, "ErrNotAnAncestor"},
		{ErrRequestTooLarge, "ErrRequestTooLarge"},
		{ErrUtxoBackend, "ErrUtxoBackend"},
		{ErrUtxoBackendCorruption, "ErrUtxoBackendCorruption"},
		{ErrUtxoBackendNotOpen, "ErrUtxoBackendNotOpen"},
		{ErrUtxoBackendTxClosed, "ErrUtxoBackendTxClosed"},
		{ErrInvalidRevocationTxVersion, "ErrInvalidRevocationTxVersion"},
		{ErrNoExpiredTicketRevocation, "ErrNoExpiredTicketRevocation"},
		{ErrNoMissedTicketRevocation, "ErrNoMissedTicketRevocation"},
		{ErrUnknownDeploymentID, "ErrUnknownDeploymentID"},
		{ErrUnknownDeploymentVersion, "ErrUnknownDeploymentVersion"},
		{ErrDuplicateDeployment, "ErrDuplicateDeployment"},
		{ErrUnknownDeploymentChoice, "ErrUnknownDeploymentChoice"},
		{ErrDeploymentBadMask, "ErrDeploymentBadMask"},
		{ErrDeploymentTooManyChoices, "ErrDeploymentTooManyChoices"},
		{ErrDeploymentMissingChoiceID, "ErrDeploymentMissingChoiceID"},
		{ErrDeploymentBadChoiceBits, "ErrDeploymentBadChoiceBits"},
		{ErrDeploymentNonExclusiveFlags, "ErrDeploymentNonExclusiveFlags"},
		{ErrDeploymentDuplicateChoice, "ErrDeploymentDuplicateChoice"},
		{ErrDeploymentMissingAbstain, "ErrDeploymentMissingAbstain"},
		{ErrDeploymentTooManyAbstain, "ErrDeploymentTooManyAbstain"},
		{ErrDeploymentMissingNo, "ErrDeploymentMissingNo"},
		{ErrDeploymentTooManyNo, "ErrDeploymentTooManyNo"},
		{ErrDeploymentChoiceAbstain, "ErrDeploymentChoiceAbstain"},
		{ErrForcedMainNetChoice, "ErrForcedMainNetChoice"},
	}

	t.Logf("Running %d tests", len(tests))
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

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.Error()
		if result != test.want {
			t.Errorf("#%d: got: %s want: %s", i, result, test.want)
			continue
		}
	}
}

// TestContextError tests the error output for the ContextError type.
func TestContextError(t *testing.T) {
	tests := []struct {
		in   ContextError
		want string
	}{{
		ContextError{Description: "duplicate block"},
		"duplicate block",
	}, {
		ContextError{Description: "human-readable error"},
		"human-readable error",
	}}

	t.Logf("Running %d tests", len(tests))
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
		name:      "ErrDuplicateBlock == ErrDuplicateBlock",
		err:       ErrDuplicateBlock,
		target:    ErrDuplicateBlock,
		wantMatch: true,
		wantAs:    ErrDuplicateBlock,
	}, {
		name:      "RuleError.ErrDuplicateBlock == ErrDuplicateBlock",
		err:       ruleError(ErrDuplicateBlock, ""),
		target:    ErrDuplicateBlock,
		wantMatch: true,
		wantAs:    ErrDuplicateBlock,
	}, {
		name:      "RuleError.ErrDuplicateBlock == RuleError.ErrDuplicateBlock",
		err:       ruleError(ErrDuplicateBlock, ""),
		target:    ruleError(ErrDuplicateBlock, ""),
		wantMatch: true,
		wantAs:    ErrDuplicateBlock,
	}, {
		name:      "ErrDuplicateBlock != ErrMissingParent",
		err:       ErrDuplicateBlock,
		target:    ErrMissingParent,
		wantMatch: false,
		wantAs:    ErrDuplicateBlock,
	}, {
		name:      "RuleError.ErrDuplicateBlock != ErrMissingParent",
		err:       ruleError(ErrDuplicateBlock, ""),
		target:    ErrMissingParent,
		wantMatch: false,
		wantAs:    ErrDuplicateBlock,
	}, {
		name:      "ErrDuplicateBlock != RuleError.ErrMissingParent",
		err:       ErrDuplicateBlock,
		target:    ruleError(ErrMissingParent, ""),
		wantMatch: false,
		wantAs:    ErrDuplicateBlock,
	}, {
		name:      "RuleError.ErrDuplicateBlock != RuleError.ErrMissingParent",
		err:       ruleError(ErrDuplicateBlock, ""),
		target:    ruleError(ErrMissingParent, ""),
		wantMatch: false,
		wantAs:    ErrDuplicateBlock,
	}, {
		name:      "RuleError.ErrDuplicateBlock != io.EOF",
		err:       ruleError(ErrDuplicateBlock, ""),
		target:    io.EOF,
		wantMatch: false,
		wantAs:    ErrDuplicateBlock,
	}, {
		name:      "ContextError.ErrDBTooOldToUpgrade == ErrDBTooOldToUpgrade",
		err:       contextError(ErrDBTooOldToUpgrade, ""),
		target:    ErrDBTooOldToUpgrade,
		wantMatch: true,
		wantAs:    ErrDBTooOldToUpgrade,
	}, {
		name:      "ContextError.ErrDBTooOldToUpgrade == ContextError.ErrDBTooOldToUpgrade",
		err:       contextError(ErrDBTooOldToUpgrade, ""),
		target:    contextError(ErrDBTooOldToUpgrade, ""),
		wantMatch: true,
		wantAs:    ErrDBTooOldToUpgrade,
	}, {
		name:      "ContextError.ErrDBTooOldToUpgrade != ErrMissingParent",
		err:       contextError(ErrDBTooOldToUpgrade, ""),
		target:    ErrMissingParent,
		wantMatch: false,
		wantAs:    ErrDBTooOldToUpgrade,
	}, {
		name:      "ErrDBTooOldToUpgrade != ContextError.ErrMissingParent",
		err:       ErrDBTooOldToUpgrade,
		target:    contextError(ErrMissingParent, ""),
		wantMatch: false,
		wantAs:    ErrDBTooOldToUpgrade,
	}, {
		name:      "ContextError.ErrDBTooOldToUpgrade != ContextError.ErrMissingParent",
		err:       contextError(ErrDBTooOldToUpgrade, ""),
		target:    contextError(ErrMissingParent, ""),
		wantMatch: false,
		wantAs:    ErrDBTooOldToUpgrade,
	}, {
		name:      "ContextError.ErrDBTooOldToUpgrade != io.EOF",
		err:       contextError(ErrDBTooOldToUpgrade, ""),
		target:    io.EOF,
		wantMatch: false,
		wantAs:    ErrDBTooOldToUpgrade,
	}}

	for _, test := range tests {
		// Ensure the error matches or not depending on the expected result.
		result := errors.Is(test.err, test.target)
		if result != test.wantMatch {
			t.Errorf("%s: incorrect error identification -- got %v, want %v",
				test.name, result, test.wantMatch)
			continue
		}

		// Ensure the underlying error kind can be unwrapped and is the
		// expected kind.
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
