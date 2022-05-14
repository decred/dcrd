// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package fullblocktests

// ErrorKind identifies a kind of error.  It has full support for errors.Is and
// errors.As, so the caller can directly check against an error kind when
// determining the reason for an error.
type ErrorKind string

// These constants are used to identify a specific ErrorKind.
const (
	// ErrDuplicateBlock indicates a block with the same hash already exists.
	ErrDuplicateBlock = ErrorKind("ErrDuplicateBlock")

	// ErrBlockTooBig indicates the serialized block size exceeds the maximum
	// allowed size.
	ErrBlockTooBig = ErrorKind("ErrBlockTooBig")

	// ErrWrongBlockSize indicates that the block size in the header is not the
	// actual serialized size of the block.
	ErrWrongBlockSize = ErrorKind("ErrWrongBlockSize")

	// ErrInvalidTime indicates the time in the passed block has a precision
	// that is more than one second.  The chain consensus rules require
	// timestamps to have a maximum precision of one second.
	ErrInvalidTime = ErrorKind("ErrInvalidTime")

	// ErrTimeTooOld indicates the time is either before the median time of the
	// last several blocks per the chain consensus rules or prior to the most
	// recent checkpoint.
	ErrTimeTooOld = ErrorKind("ErrTimeTooOld")

	// ErrTimeTooNew indicates the time is too far in the future as compared the
	// current time.
	ErrTimeTooNew = ErrorKind("ErrTimeTooNew")

	// ErrUnexpectedDifficulty indicates specified bits do not align with the
	// expected value either because it doesn't match the calculated value based
	// on difficulty regarding the rules or it is out of the valid range.
	ErrUnexpectedDifficulty = ErrorKind("ErrUnexpectedDifficulty")

	// ErrHighHash indicates the block does not hash to a value which is lower
	// than the required target difficultly.
	ErrHighHash = ErrorKind("ErrHighHash")

	// ErrBadMerkleRoot indicates the calculated merkle root does not match the
	// expected value.
	ErrBadMerkleRoot = ErrorKind("ErrBadMerkleRoot")

	// ErrNoTransactions indicates the block does not have at least one
	// transaction.  A valid block must have at least the coinbase transaction.
	ErrNoTransactions = ErrorKind("ErrNoTransactions")

	// ErrNoTxInputs indicates a transaction does not have any inputs.  A valid
	// transaction must have at least one input.
	ErrNoTxInputs = ErrorKind("ErrNoTxInputs")

	// ErrNoTxOutputs indicates a transaction does not have any outputs.  A
	// valid transaction must have at least one output.
	ErrNoTxOutputs = ErrorKind("ErrNoTxOutputs")

	// ErrBadTxOutValue indicates an output value for a transaction is invalid
	// in some way such as being out of range.
	ErrBadTxOutValue = ErrorKind("ErrBadTxOutValue")

	// ErrDuplicateTxInputs indicates a transaction references the same input
	// more than once.
	ErrDuplicateTxInputs = ErrorKind("ErrDuplicateTxInputs")

	// ErrBadTxInput indicates a transaction input is invalid in some way such
	// as referencing a previous transaction outpoint which is out of range or
	// not referencing one at all.
	ErrBadTxInput = ErrorKind("ErrBadTxInput")

	// ErrMissingTxOut indicates a transaction output referenced by an input
	// either does not exist or has already been spent.
	ErrMissingTxOut = ErrorKind("ErrMissingTxOut")

	// ErrUnfinalizedTx indicates a transaction has not been finalized.  A valid
	// block may only contain finalized transactions.
	ErrUnfinalizedTx = ErrorKind("ErrUnfinalizedTx")

	// ErrDuplicateTx indicates a block contains an identical transaction (or at
	// least two transactions which hash to the same value).  A valid block may
	// only contain unique transactions.
	ErrDuplicateTx = ErrorKind("ErrDuplicateTx")

	// ErrImmatureSpend indicates a transaction is attempting to spend a
	// coinbase that has not yet reached the required maturity.
	ErrImmatureSpend = ErrorKind("ErrImmatureSpend")

	// ErrSpendTooHigh indicates a transaction is attempting to spend more value
	// than the sum of all of its inputs.
	ErrSpendTooHigh = ErrorKind("ErrSpendTooHigh")

	// ErrTooManySigOps indicates the total number of signature operations for a
	// transaction or block exceed the maximum allowed limits.
	ErrTooManySigOps = ErrorKind("ErrTooManySigOps")

	// ErrFirstTxNotCoinbase indicates the first transaction in a block is not a
	// coinbase transaction.
	ErrFirstTxNotCoinbase = ErrorKind("ErrFirstTxNotCoinbase")

	// ErrCoinbaseHeight indicates that the encoded height in the coinbase is
	// incorrect.
	ErrCoinbaseHeight = ErrorKind("ErrCoinbaseHeight")

	// ErrMultipleCoinbases indicates a block contains more than one coinbase
	// transaction.
	ErrMultipleCoinbases = ErrorKind("ErrMultipleCoinbases")

	// ErrStakeTxInRegularTree indicates a stake transaction was found in the
	// regular transaction tree.
	ErrStakeTxInRegularTree = ErrorKind("ErrStakeTxInRegularTree")

	// ErrRegTxInStakeTree indicates that a regular transaction was found in the
	// stake transaction tree.
	ErrRegTxInStakeTree = ErrorKind("ErrRegTxInStakeTree")

	// ErrBadCoinbaseScriptLen indicates the length of the signature script for
	// a coinbase transaction is not within the valid range.
	ErrBadCoinbaseScriptLen = ErrorKind("ErrBadCoinbaseScriptLen")

	// ErrBadCoinbaseValue indicates the amount of a coinbase value does not
	// match the expected value of the subsidy plus the sum of all fees.
	ErrBadCoinbaseValue = ErrorKind("ErrBadCoinbaseValue")

	// ErrBadCoinbaseFraudProof indicates that the fraud proof for a coinbase
	// input was non-null.
	ErrBadCoinbaseFraudProof = ErrorKind("ErrBadCoinbaseFraudProof")

	// ErrBadCoinbaseAmountIn indicates that the AmountIn (=subsidy) for a
	// coinbase input was incorrect.
	ErrBadCoinbaseAmountIn = ErrorKind("ErrBadCoinbaseAmountIn")

	// ErrBadStakebaseAmountIn indicates that the AmountIn (=subsidy) for a
	// stakebase input was incorrect.
	ErrBadStakebaseAmountIn = ErrorKind("ErrBadStakebaseAmountIn")

	// ErrBadStakebaseScriptLen indicates the length of the signature script for
	// a stakebase transaction is not within the valid range.
	ErrBadStakebaseScriptLen = ErrorKind("ErrBadStakebaseScriptLen")

	// ErrBadStakebaseScrVal indicates the signature script for a stakebase
	// transaction was not set to the network consensus value.
	ErrBadStakebaseScrVal = ErrorKind("ErrBadStakebaseScrVal")

	// ErrScriptMalformed indicates a transaction script is malformed in some
	// way.  For example, it might be longer than the maximum allowed length or
	// fail to parse.
	ErrScriptMalformed = ErrorKind("ErrScriptMalformed")

	// ErrScriptValidation indicates the result of executing a transaction
	// script failed.  The error covers any failure when executing scripts such
	// as signature verification failures and execution past the end of the
	// stack.
	ErrScriptValidation = ErrorKind("ErrScriptValidation")

	// ErrNotEnoughStake indicates that there was for some SStx in a given
	// block, the given SStx did not have enough stake to meet the network
	// target.
	ErrNotEnoughStake = ErrorKind("ErrNotEnoughStake")

	// ErrStakeBelowMinimum indicates that for some SStx in a given block, the
	// given SStx had an amount of stake below the minimum network target.
	ErrStakeBelowMinimum = ErrorKind("ErrStakeBelowMinimum")

	// ErrNotEnoughVotes indicates that a block contained less than a majority
	// of voters.
	ErrNotEnoughVotes = ErrorKind("ErrNotEnoughVotes")

	// ErrTooManyVotes indicates that a block contained more than the maximum
	// allowable number of votes.
	ErrTooManyVotes = ErrorKind("ErrTooManyVotes")

	// ErrFreshStakeMismatch indicates that a block's header contained a
	// different number of SStx as compared to what was found in the block.
	ErrFreshStakeMismatch = ErrorKind("ErrFreshStakeMismatch")

	// ErrInvalidEarlyStakeTx indicates that a tx type other than SStx was found
	// in the stake tx tree before the period when stake validation begins, or
	// before the stake tx type could possibly be included in the block.
	ErrInvalidEarlyStakeTx = ErrorKind("ErrInvalidEarlyStakeTx")

	// ErrTicketUnavailable indicates that a vote in the block spent a ticket
	// that could not be found.
	ErrTicketUnavailable = ErrorKind("ErrTicketUnavailable")

	// ErrVotesOnWrongBlock indicates that an SSGen voted on a block that is not
	// the block's parent, and so was ineligible for inclusion into that block.
	ErrVotesOnWrongBlock = ErrorKind("ErrVotesOnWrongBlock")

	// ErrVotesMismatch indicates that the number of SSGen in the block was not
	// equivalent to the number of votes provided in the block header.
	ErrVotesMismatch = ErrorKind("ErrVotesMismatch")

	// ErrIncongruentVotebit indicates that the first votebit in votebits was
	// not the same as that determined by the majority of voters in the SSGen tx
	// included in the block.
	ErrIncongruentVotebit = ErrorKind("ErrIncongruentVotebit")

	// ErrInvalidSSRtx indicates than an SSRtx in a block could not be found to
	// have a valid missed sstx input as per the stake ticket database.
	ErrInvalidSSRtx = ErrorKind("ErrInvalidSSRtx")

	// ErrInvalidRevNum indicates that the number of revocations from the header
	// was not the same as the number of SSRtx included in the block.
	ErrRevocationsMismatch = ErrorKind("ErrRevocationsMismatch")

	// ErrTicketCommitment indicates that a ticket commitment contains an amount
	// that does not coincide with the associated ticket input amount.
	ErrTicketCommitment = ErrorKind("ErrTicketCommitment")

	// ErrBadNumPayees indicates that either a vote or revocation transaction
	// does not make the correct number of payments per the associated ticket
	// commitments.
	ErrBadNumPayees = ErrorKind("ErrBadNumPayees")

	// ErrBadPayeeScriptType indicates that either a vote or revocation
	// transaction output that corresponds to a ticket commitment does not pay
	// to the hash required by the commitment.
	ErrMismatchedPayeeHash = ErrorKind("ErrMismatchedPayeeHash")

	// ErrBadPayeeValue indicates that either a vote or revocation transaction
	// output that corresponds to a ticket commitment does not pay the expected
	// amount required by the commitment.
	ErrBadPayeeValue = ErrorKind("ErrBadPayeeValue")

	// ErrTxSStxOutSpend indicates that a non SSGen or SSRtx tx attempted to
	// spend an OP_SSTX tagged output from an SStx.
	ErrTxSStxOutSpend = ErrorKind("ErrTxSStxOutSpend")

	// ErrRegTxCreateStakeOut indicates that a regular tx attempted to create a
	// stake tagged output.
	ErrRegTxCreateStakeOut = ErrorKind("ErrRegTxCreateStakeOut")

	// ErrInvalidFinalState indicates that the final state of the PRNG included
	// in the block differed from the calculated final state.
	ErrInvalidFinalState = ErrorKind("ErrInvalidFinalState")

	// ErrPoolSize indicates an error in the ticket pool size for this block.
	ErrPoolSize = ErrorKind("ErrPoolSize")

	// ErrBadBlockHeight indicates that a block header's embedded block height
	// was different from where it was actually embedded in the block chain.
	ErrBadBlockHeight = ErrorKind("ErrBadBlockHeight")

	// ErrBlockOneOutputs indicates that block height 1 failed to incorporate
	// the ledger addresses correctly into the transaction's outputs.
	ErrBlockOneOutputs = ErrorKind("ErrBlockOneOutputs")

	// ErrNoTreasury indicates that there was no treasury payout present in the
	// coinbase of a block after height 1 and prior to the activation of the
	// decentralized treasury.
	ErrNoTreasury = ErrorKind("ErrNoTreasury")

	// ErrExpiredTx indicates that the transaction is currently expired.
	ErrExpiredTx = ErrorKind("ErrExpiredTx")

	// ErrFraudAmountIn indicates the witness amount given was fraudulent.
	ErrFraudAmountIn = ErrorKind("ErrFraudAmountIn")

	// ErrFraudBlockHeight indicates the witness block height given was
	// fraudulent.
	ErrFraudBlockHeight = ErrorKind("ErrFraudBlockHeight")

	// ErrFraudBlockIndex indicates the witness block index given was
	// fraudulent.
	ErrFraudBlockIndex = ErrorKind("ErrFraudBlockIndex")

	// ErrInvalidEarlyVoteBits indicates that a block before stake validation
	// height had an unallowed vote bits value.
	ErrInvalidEarlyVoteBits = ErrorKind("ErrInvalidEarlyVoteBits")

	// ErrInvalidEarlyFinalState indicates that a block before stake validation
	// height had a non-zero final state.
	ErrInvalidEarlyFinalState = ErrorKind("ErrInvalidEarlyFinalState")
)

// Error satisfies the error interface and prints human-readable errors.
func (e ErrorKind) Error() string {
	return string(e)
}

// RuleError identifies a rule violation.  It is used to indicate that
// processing of a block or transaction failed due to one of the many validation
// rules.  It has full support for errors.Is and errors.As, so the caller can
// ascertain the specific reason for the rule violation.
type RuleError struct {
	Err         error
	Description string
}

// Error satisfies the error interface and prints human-readable errors.
func (e RuleError) Error() string {
	return e.Description
}

// Unwrap returns the underlying wrapped error.
func (e RuleError) Unwrap() error {
	return e.Err
}

// ruleError creates a RuleError given a set of arguments.
func ruleError(kind ErrorKind, desc string) RuleError {
	return RuleError{Err: kind, Description: desc}
}
