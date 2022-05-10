// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// AssertError identifies an error that indicates an internal code consistency
// issue and should be treated as a critical and unrecoverable error.
type AssertError string

// Error returns the assertion error as a human-readable string and satisfies
// the error interface.
func (e AssertError) Error() string {
	return "assertion failed: " + string(e)
}

// ErrorKind identifies a kind of error.  It has full support for errors.Is and
// errors.As, so the caller can directly check against an error kind when
// determining the reason for an error.
type ErrorKind string

// These constants are used to identify a specific ErrorKind.
const (
	// ErrDuplicateBlock indicates a block with the same hash already
	// exists.
	ErrDuplicateBlock = ErrorKind("ErrDuplicateBlock")

	// ErrMissingParent indicates that the block was an orphan.
	ErrMissingParent = ErrorKind("ErrMissingParent")

	// ErrNoBlockData indicates an attempt to perform an operation on a block
	// that requires all data to be available does not have the data.  This is
	// typically because the header is known, but the full data has not been
	// received yet.
	ErrNoBlockData = ErrorKind("ErrNoBlockData")

	// ErrBlockTooBig indicates the serialized block size exceeds the
	// maximum allowed size.
	ErrBlockTooBig = ErrorKind("ErrBlockTooBig")

	// ErrWrongBlockSize indicates that the block size in the header is not
	// the actual serialized size of the block.
	ErrWrongBlockSize = ErrorKind("ErrWrongBlockSize")

	// ErrBlockVersionTooOld indicates the block version is too old and is
	// no longer accepted since the majority of the network has upgraded
	// to a newer version.
	ErrBlockVersionTooOld = ErrorKind("ErrBlockVersionTooOld")

	// ErrBadStakeVersionindicates the block version is too old and is no
	// longer accepted since the majority of the network has upgraded to a
	// newer version.
	ErrBadStakeVersion = ErrorKind("ErrBadStakeVersion")

	// ErrInvalidTime indicates the time in the passed block has a precision
	// that is more than one second.  The chain consensus rules require
	// timestamps to have a maximum precision of one second.
	ErrInvalidTime = ErrorKind("ErrInvalidTime")

	// ErrTimeTooOld indicates the time is either before the median time of
	// the last several blocks per the chain consensus rules or prior to the
	// most recent checkpoint.
	ErrTimeTooOld = ErrorKind("ErrTimeTooOld")

	// ErrTimeTooNew indicates the time is too far in the future as compared
	// the current time.
	ErrTimeTooNew = ErrorKind("ErrTimeTooNew")

	// ErrDifficultyTooLow indicates the difficulty for the block is lower
	// than the difficulty required by the most recent checkpoint.
	ErrDifficultyTooLow = ErrorKind("ErrDifficultyTooLow")

	// ErrUnexpectedDifficulty indicates specified bits do not align with
	// the expected value either because it doesn't match the calculated
	// value based on difficulty regarding the rules or it is out of the
	// valid range.
	ErrUnexpectedDifficulty = ErrorKind("ErrUnexpectedDifficulty")

	// ErrHighHash indicates the block does not hash to a value which is
	// lower than the required target difficultly.
	ErrHighHash = ErrorKind("ErrHighHash")

	// ErrBadMerkleRoot indicates the calculated merkle root does not match
	// the expected value.
	ErrBadMerkleRoot = ErrorKind("ErrBadMerkleRoot")

	// ErrBadCommitmentRoot indicates the calculated commitment root does
	// not match the expected value.
	ErrBadCommitmentRoot = ErrorKind("ErrBadCommitmentRoot")

	// ErrForkTooOld indicates a block is attempting to fork the block chain
	// before the fork rejection checkpoint.
	ErrForkTooOld = ErrorKind("ErrForkTooOld")

	// ErrNoTransactions indicates the block does not have a least one
	// transaction.  A valid block must have at least the coinbase
	// transaction.
	ErrNoTransactions = ErrorKind("ErrNoTransactions")

	// ErrNoTxInputs indicates a transaction does not have any inputs.  A
	// valid transaction must have at least one input.
	ErrNoTxInputs = ErrorKind("ErrNoTxInputs")

	// ErrNoTxOutputs indicates a transaction does not have any outputs.  A
	// valid transaction must have at least one output.
	ErrNoTxOutputs = ErrorKind("ErrNoTxOutputs")

	// ErrTxTooBig indicates a transaction exceeds the maximum allowed size
	// when serialized.
	ErrTxTooBig = ErrorKind("ErrTxTooBig")

	// ErrBadTxOutValue indicates an output value for a transaction is
	// invalid in some way such as being out of range.
	ErrBadTxOutValue = ErrorKind("ErrBadTxOutValue")

	// ErrDuplicateTxInputs indicates a transaction references the same
	// input more than once.
	ErrDuplicateTxInputs = ErrorKind("ErrDuplicateTxInputs")

	// ErrTxVersionTooHigh indicates a transaction version is higher than the
	// maximum version allowed by the active consensus rules.
	ErrTxVersionTooHigh = ErrorKind("ErrTxVersionTooHigh")

	// ErrBadTxInput indicates a transaction input is invalid in some way
	// such as referencing a previous transaction outpoint which is out of
	// range or not referencing one at all.
	ErrBadTxInput = ErrorKind("ErrBadTxInput")

	// ErrScriptVersionTooHigh indicates a transaction script version is higher
	// than the maximum version allowed by the active consensus rules.
	ErrScriptVersionTooHigh = ErrorKind("ErrScriptVersionTooHigh")

	// ErrMissingTxOut indicates a transaction output referenced by an input
	// either does not exist or has already been spent.
	ErrMissingTxOut = ErrorKind("ErrMissingTxOut")

	// ErrUnfinalizedTx indicates a transaction has not been finalized.
	// A valid block may only contain finalized transactions.
	ErrUnfinalizedTx = ErrorKind("ErrUnfinalizedTx")

	// ErrDuplicateTx indicates a block contains an identical transaction
	// (or at least two transactions which hash to the same value).  A
	// valid block may only contain unique transactions.
	ErrDuplicateTx = ErrorKind("ErrDuplicateTx")

	// ErrOverwriteTx indicates a block contains a transaction that has
	// the same hash as a previous transaction which has not been fully
	// spent.
	ErrOverwriteTx = ErrorKind("ErrOverwriteTx")

	// ErrImmatureSpend indicates a transaction is attempting to spend a
	// coinbase that has not yet reached the required maturity.
	ErrImmatureSpend = ErrorKind("ErrImmatureSpend")

	// ErrSpendTooHigh indicates a transaction is attempting to spend more
	// value than the sum of all of its inputs.
	ErrSpendTooHigh = ErrorKind("ErrSpendTooHigh")

	// ErrBadFees indicates the total fees for a block are invalid due to
	// exceeding the maximum possible value.
	ErrBadFees = ErrorKind("ErrBadFees")

	// ErrTooManySigOps indicates the total number of signature operations
	// for a transaction or block exceed the maximum allowed limits.
	ErrTooManySigOps = ErrorKind("ErrTooManySigOps")

	// ErrFirstTxNotCoinbase indicates the first transaction in a block
	// is not a coinbase transaction.
	ErrFirstTxNotCoinbase = ErrorKind("ErrFirstTxNotCoinbase")

	// ErrCoinbaseHeight indicates that the encoded height in the coinbase
	// is incorrect.
	ErrCoinbaseHeight = ErrorKind("ErrCoinbaseHeight")

	// ErrMultipleCoinbases indicates a block contains more than one
	// coinbase transaction.
	ErrMultipleCoinbases = ErrorKind("ErrMultipleCoinbases")

	// ErrStakeTxInRegularTree indicates a stake transaction was found in
	// the regular transaction tree.
	ErrStakeTxInRegularTree = ErrorKind("ErrStakeTxInRegularTree")

	// ErrRegTxInStakeTree indicates that a regular transaction was found in
	// the stake transaction tree.
	ErrRegTxInStakeTree = ErrorKind("ErrRegTxInStakeTree")

	// ErrBadCoinbaseScriptLen indicates the length of the signature script
	// for a coinbase transaction is not within the valid range.
	ErrBadCoinbaseScriptLen = ErrorKind("ErrBadCoinbaseScriptLen")

	// ErrBadCoinbaseValue indicates the amount of a coinbase value does
	// not match the expected value of the subsidy plus the sum of all fees.
	ErrBadCoinbaseValue = ErrorKind("ErrBadCoinbaseValue")

	// ErrBadCoinbaseOutpoint indicates that the outpoint used by a coinbase
	// as input was non-null.
	ErrBadCoinbaseOutpoint = ErrorKind("ErrBadCoinbaseOutpoint")

	// ErrBadCoinbaseFraudProof indicates that the fraud proof for a coinbase
	// input was non-null.
	ErrBadCoinbaseFraudProof = ErrorKind("ErrBadCoinbaseFraudProof")

	// ErrBadCoinbaseAmountIn indicates that the AmountIn (=subsidy) for a
	// coinbase input was incorrect.
	ErrBadCoinbaseAmountIn = ErrorKind("ErrBadCoinbaseAmountIn")

	// ErrBadStakebaseAmountIn indicates that the AmountIn (=subsidy) for a
	// stakebase input was incorrect.
	ErrBadStakebaseAmountIn = ErrorKind("ErrBadStakebaseAmountIn")

	// ErrBadStakebaseScriptLen indicates the length of the signature script
	// for a stakebase transaction is not within the valid range.
	ErrBadStakebaseScriptLen = ErrorKind("ErrBadStakebaseScriptLen")

	// ErrBadStakebaseScrVal indicates the signature script for a stakebase
	// transaction was not set to the network consensus value.
	ErrBadStakebaseScrVal = ErrorKind("ErrBadStakebaseScrVal")

	// ErrScriptMalformed indicates a transaction script is malformed in
	// some way.  For example, it might be longer than the maximum allowed
	// length or fail to parse.
	ErrScriptMalformed = ErrorKind("ErrScriptMalformed")

	// ErrScriptValidation indicates the result of executing transaction
	// script failed.  The error covers any failure when executing scripts
	// such signature verification failures and execution past the end of
	// the stack.
	ErrScriptValidation = ErrorKind("ErrScriptValidation")

	// ErrNotEnoughStake indicates that there was for some SStx in a given block,
	// the given SStx did not have enough stake to meet the network target.
	ErrNotEnoughStake = ErrorKind("ErrNotEnoughStake")

	// ErrStakeBelowMinimum indicates that for some SStx in a given block,
	// the given SStx had an amount of stake below the minimum network target.
	ErrStakeBelowMinimum = ErrorKind("ErrStakeBelowMinimum")

	// ErrNonstandardStakeTx indicates that a block contained a stake tx that
	// was not one of the allowed types of a stake transactions.
	ErrNonstandardStakeTx = ErrorKind("ErrNonstandardStakeTx")

	// ErrNotEnoughVotes indicates that a block contained less than a majority
	// of voters.
	ErrNotEnoughVotes = ErrorKind("ErrNotEnoughVotes")

	// ErrTooManyVotes indicates that a block contained more than the maximum
	// allowable number of votes.
	ErrTooManyVotes = ErrorKind("ErrTooManyVotes")

	// ErrFreshStakeMismatch indicates that a block's header contained a different
	// number of SStx as compared to what was found in the block.
	ErrFreshStakeMismatch = ErrorKind("ErrFreshStakeMismatch")

	// ErrTooManySStxs indicates that more than the allowed number of SStx was
	// found in a block.
	ErrTooManySStxs = ErrorKind("ErrTooManySStxs")

	// ErrInvalidEarlyStakeTx indicates that a tx type other than SStx was found
	// in the stake tx tree before the period when stake validation begins, or
	// before the stake tx type could possibly be included in the block.
	ErrInvalidEarlyStakeTx = ErrorKind("ErrInvalidEarlyStakeTx")

	// ErrTicketUnavailable indicates that a vote in the block spent a ticket
	// that could not be found.
	ErrTicketUnavailable = ErrorKind("ErrTicketUnavailable")

	// ErrVotesOnWrongBlock indicates that an SSGen voted on a block not the
	// block's parent, and so was ineligible for inclusion into that block.
	ErrVotesOnWrongBlock = ErrorKind("ErrVotesOnWrongBlock")

	// ErrVotesMismatch indicates that the number of SSGen in the block was not
	// equivalent to the number of votes provided in the block header.
	ErrVotesMismatch = ErrorKind("ErrVotesMismatch")

	// ErrIncongruentVotebit indicates that the first votebit in votebits was not
	// the same as that determined by the majority of voters in the SSGen tx
	// included in the block.
	ErrIncongruentVotebit = ErrorKind("ErrIncongruentVotebit")

	// ErrInvalidSSRtx indicates than an SSRtx in a block could not be found to
	// have a valid missed sstx input as per the stake ticket database.
	ErrInvalidSSRtx = ErrorKind("ErrInvalidSSRtx")

	// ErrInvalidRevNum indicates that the number of revocations from the
	// header was not the same as the number of SSRtx included in the block.
	ErrRevocationsMismatch = ErrorKind("ErrRevocationsMismatch")

	// ErrTooManyRevocations indicates more revocations were found in a block
	// than were allowed.
	ErrTooManyRevocations = ErrorKind("ErrTooManyRevocations")

	// ErrTicketCommitment indicates that a ticket commitment contains an amount
	// that does not coincide with the associated ticket input amount.
	ErrTicketCommitment = ErrorKind("ErrTicketCommitment")

	// ErrInvalidVoteInput indicates that an input to a vote transaction is
	// either not a stake ticket submission or is not a supported version.
	ErrInvalidVoteInput = ErrorKind("ErrInvalidVoteInput")

	// ErrBadNumPayees indicates that either a vote or revocation transaction
	// does not make the correct number of payments per the associated ticket
	// commitments.
	ErrBadNumPayees = ErrorKind("ErrBadNumPayees")

	// ErrBadPayeeScriptVersion indicates that either a vote or revocation
	// transaction output that corresponds to a ticket commitment does not use
	// a supported script version.
	ErrBadPayeeScriptVersion = ErrorKind("ErrBadPayeeScriptVersion")

	// ErrBadPayeeScriptType indicates that either a vote or revocation
	// transaction output that corresponds to a ticket commitment does not pay
	// to the same script type required by the commitment.
	ErrBadPayeeScriptType = ErrorKind("ErrBadPayeeScriptType")

	// ErrBadPayeeScriptType indicates that either a vote or revocation
	// transaction output that corresponds to a ticket commitment does not pay
	// to the hash required by the commitment.
	ErrMismatchedPayeeHash = ErrorKind("ErrMismatchedPayeeHash")

	// ErrBadPayeeValue indicates that either a vote or revocation transaction
	// output that corresponds to a ticket commitment does not pay the expected
	// amount required by the commitment.
	ErrBadPayeeValue = ErrorKind("ErrBadPayeeValue")

	// ErrSSGenSubsidy indicates that there was an error in the amount of subsidy
	// generated in the vote.
	ErrSSGenSubsidy = ErrorKind("ErrSSGenSubsidy")

	// ErrImmatureTicketSpend indicates that a vote or revocation is attempting
	// to spend a ticket submission output that has not yet reached the required
	// maturity.
	ErrImmatureTicketSpend = ErrorKind("ErrImmatureTicketSpend")

	// ErrTicketInputScript indicates that a ticket input is not one of the
	// supported script forms or versions.
	ErrTicketInputScript = ErrorKind("ErrTicketInputScript")

	// ErrInvalidRevokeInput indicates that an input to a revocation transaction
	// is either not a stake ticket submission or is not a supported version.
	ErrInvalidRevokeInput = ErrorKind("ErrInvalidRevokeInput")

	// ErrTxSStxOutSpend indicates that a non SSGen or SSRtx tx attempted to spend
	// an OP_SSTX tagged output from an SStx.
	ErrTxSStxOutSpend = ErrorKind("ErrTxSStxOutSpend")

	// ErrRegTxCreateStakeOut indicates that a regular tx attempted to create
	// a stake tagged output.
	ErrRegTxCreateStakeOut = ErrorKind("ErrRegTxCreateStakeOut")

	// ErrInvalidFinalState indicates that the final state of the PRNG included
	// in the block differed from the calculated final state.
	ErrInvalidFinalState = ErrorKind("ErrInvalidFinalState")

	// ErrPoolSize indicates an error in the ticket pool size for this block.
	ErrPoolSize = ErrorKind("ErrPoolSize")

	// ErrForceReorgSameBlock indicates that a reorganization  was attempted to
	// be forced to the same block.
	ErrForceReorgSameBlock = ErrorKind("ErrForceReorgSameBlock")

	// ErrForceReorgWrongChain indicates that a reorganization  was attempted
	// to be forced, but the chain indicated was not mirrored by b.bestChain.
	ErrForceReorgWrongChain = ErrorKind("ErrForceReorgWrongChain")

	// ErrForceReorgMissingChild indicates that a reorganization  was attempted
	// to be forced, but the child node to reorganize to could not be found.
	ErrForceReorgMissingChild = ErrorKind("ErrForceReorgMissingChild")

	// ErrBadStakebaseValue indicates that a block's stake tx tree has spent
	// more than it is allowed.
	ErrBadStakebaseValue = ErrorKind("ErrBadStakebaseValue")

	// ErrStakeFees indicates an error with the fees found in the stake
	// transaction tree.
	ErrStakeFees = ErrorKind("ErrStakeFees")

	// ErrNoStakeTx indicates there were no stake transactions found in a
	// block after stake validation height.
	ErrNoStakeTx = ErrorKind("ErrNoStakeTx")

	// ErrBadBlockHeight indicates that a block header's embedded block height
	// was different from where it was actually embedded in the block chain.
	ErrBadBlockHeight = ErrorKind("ErrBadBlockHeight")

	// ErrBlockOneTx indicates that block height 1 failed to correct generate
	// the block one initial payout transaction.
	ErrBlockOneTx = ErrorKind("ErrBlockOneTx")

	// ErrBlockOneTx indicates that block height 1 coinbase transaction in
	// zero was incorrect in some way.
	ErrBlockOneInputs = ErrorKind("ErrBlockOneInputs")

	// ErrBlockOneOutputs indicates that block height 1 failed to incorporate
	// the ledger addresses correctly into the transaction's outputs.
	ErrBlockOneOutputs = ErrorKind("ErrBlockOneOutputs")

	// ErrNoTax indicates that there was no tax present in the coinbase of a
	// block after height 1.
	ErrNoTax = ErrorKind("ErrNoTax")

	// ErrExpiredTx indicates that the transaction is currently expired.
	ErrExpiredTx = ErrorKind("ErrExpiredTx")

	// ErrExpiryTxSpentEarly indicates that an output from a transaction
	// that included an expiry field was spent before coinbase maturity
	// many blocks had passed in the blockchain.
	ErrExpiryTxSpentEarly = ErrorKind("ErrExpiryTxSpentEarly")

	// ErrFraudAmountIn indicates the witness amount given was fraudulent.
	ErrFraudAmountIn = ErrorKind("ErrFraudAmountIn")

	// ErrFraudBlockHeight indicates the witness block height given was
	// fraudulent.
	ErrFraudBlockHeight = ErrorKind("ErrFraudBlockHeight")

	// ErrFraudBlockIndex indicates the witness block index given was
	// fraudulent.
	ErrFraudBlockIndex = ErrorKind("ErrFraudBlockIndex")

	// ErrZeroValueOutputSpend indicates that a transaction attempted to spend a
	// zero value output.
	ErrZeroValueOutputSpend = ErrorKind("ErrZeroValueOutputSpend")

	// ErrInvalidEarlyVoteBits indicates that a block before stake validation
	// height had an unallowed vote bits value.
	ErrInvalidEarlyVoteBits = ErrorKind("ErrInvalidEarlyVoteBits")

	// ErrInvalidEarlyFinalState indicates that a block before stake validation
	// height had a non-zero final state.
	ErrInvalidEarlyFinalState = ErrorKind("ErrInvalidEarlyFinalState")

	// ErrKnownInvalidBlock indicates that this block has previously failed
	// validation.
	ErrKnownInvalidBlock = ErrorKind("ErrKnownInvalidBlock")

	// ErrInvalidAncestorBlock indicates that an ancestor of this block has
	// failed validation.
	ErrInvalidAncestorBlock = ErrorKind("ErrInvalidAncestorBlock")

	// ErrInvalidTemplateParent indicates that a block template builds on a
	// block that is either not the current best chain tip or its parent.
	ErrInvalidTemplateParent = ErrorKind("ErrInvalidTemplateParent")

	// ErrUnknownPiKey indicates that the provided public Pi Key is not
	// a well known key.
	ErrUnknownPiKey = ErrorKind("ErrUnknownPiKey")

	// ErrInvalidPiSignature indicates that a treasury spend transaction
	// was not properly signed.
	ErrInvalidPiSignature = ErrorKind("ErrInvalidPiSignature")

	// ErrInvalidTVoteWindow indicates that a treasury spend transaction
	// appeared in a block that is prior to a valid treasury vote window.
	ErrInvalidTVoteWindow = ErrorKind("ErrInvalidTVoteWindow")

	// ErrNotTVI indicates that a treasury spend transaction appeared in a
	// block that is not at a TVI interval.
	ErrNotTVI = ErrorKind("ErrNotTVI")

	// ErrInvalidTSpendWindow indicates that this treasury spend
	// transaction is outside of the allowed window.
	ErrInvalidTSpendWindow = ErrorKind("ErrInvalidTSpendWindow")

	// ErrNotEnoughTSpendVotes indicates that a treasury spend transaction
	// does not have enough votes to be included in block.
	ErrNotEnoughTSpendVotes = ErrorKind("ErrNotEnoughTSpendVotes")

	// ErrInvalidTSpendValueIn indicates that a treasury spend transaction
	// ValueIn does not match the encoded copy in the first TxOut.
	ErrInvalidTSpendValueIn = ErrorKind("ErrInvalidTSpendValueIn")

	// ErrTSpendExists indicates that a duplicate treasury spend
	// transaction has been mined on a TVI in the current best chain.
	ErrTSpendExists = ErrorKind("ErrTSpendExists")

	// ErrInvalidExpenditure indicates that a treasury spend transaction
	// expenditure is out of range.
	ErrInvalidExpenditure = ErrorKind("ErrInvalidExpenditure")

	// ErrFirstTxNotTreasurybase indicates the first transaction in a block
	// is not a treasurybase transaction.
	ErrFirstTxNotTreasurybase = ErrorKind("ErrFirstTxNotTreasurybase")

	// ErrBadTreasurybaseOutpoint indicates that the outpoint used by a
	// treasurybase as input was non-null.
	ErrBadTreasurybaseOutpoint = ErrorKind("ErrBadTreasurybaseOutpoint")

	// ErrBadTreasurybaseFraudProof indicates that the fraud proof for a
	// treasurybase input was non-null.
	ErrBadTreasurybaseFraudProof = ErrorKind("ErrBadTreasurybaseFraudProof")

	// ErrBadTreasurybaseScriptLen indicates the length of the signature script
	// for a treasurybase transaction is not within the valid range.
	ErrBadTreasurybaseScriptLen = ErrorKind("ErrBadTreasurybaseScriptLen")

	// ErrTreasurybaseTxNotOpReturn indicates the second output of a
	// treasury base transaction is not an OP_RETURN.
	ErrTreasurybaseTxNotOpReturn = ErrorKind("ErrTreasurybaseTxNotOpReturn")

	// ErrTreasurybaseHeight indicates that the encoded height in the
	// treasurybase is incorrect.
	ErrTreasurybaseHeight = ErrorKind("ErrTreasurybaseHeight")

	// ErrInvalidTreasurybaseTxOutputs indicates that the transaction does
	// not have the correct number of outputs.
	ErrInvalidTreasurybaseTxOutputs = ErrorKind("ErrInvalidTreasurybaseTxOutputs")

	// ErrInvalidTreasurybaseVersion indicates that the transaction output
	// has the wrong version.
	ErrInvalidTreasurybaseVersion = ErrorKind("ErrInvalidTreasurybaseVersion")

	// ErrInvalidTreasurybaseScript indicates that the transaction output
	// script is invalid.
	ErrInvalidTreasurybaseScript = ErrorKind("ErrInvalidTreasurybaseScript")

	// ErrTreasurybaseOutValue ensures that the OP_TADD value of a
	// treasurybase is not the expected amount.
	ErrTreasurybaseOutValue = ErrorKind("ErrTreasurybaseOutValue")

	// ErrMultipleTreasurybases indicates a block contains more than one
	// treasurybase transaction.
	ErrMultipleTreasurybases = ErrorKind("ErrMultipleTreasurybases")

	// ErrBadTreasurybaseAmountIn indicates that a block contains an
	// invalid treasury contribution.
	ErrBadTreasurybaseAmountIn = ErrorKind("ErrBadTreasurybaseAmountIn")

	// ErrBadTSpendOutpoint indicates that the outpoint used by a
	// treasury spend as input was non-null.
	ErrBadTSpendOutpoint = ErrorKind("ErrBadTSpendOutpoint")

	// ErrBadTSpendFraudProof indicates that the fraud proof for a treasury
	// spend transaction input was non-null.
	ErrBadTSpendFraudProof = ErrorKind("ErrBadTSpendFraudProof")

	// ErrBadTSpendScriptLen indicates the length of the signature script
	// for a treasury spend transaction is not within the valid range.
	ErrBadTSpendScriptLen = ErrorKind("ErrBadTSpendScriptLen")

	// ErrInvalidTAddChange indicates the change output of a TAdd is zero.
	ErrInvalidTAddChange = ErrorKind("ErrInvalidTAddChange")

	// ErrTooManyTAdds indicates the number of treasury adds in a given
	// block is larger than the maximum allowed.
	ErrTooManyTAdds = ErrorKind("ErrTooManyTAdds")

	// ErrTicketExhaustion indicates extending a given block with another one
	// would result in an unrecoverable chain due to ticket exhaustion.
	ErrTicketExhaustion = ErrorKind("ErrTicketExhaustion")

	// ErrDBTooOldToUpgrade indicates the database version is prior to the
	// minimum supported version for which upgrades are supported.
	ErrDBTooOldToUpgrade = ErrorKind("ErrDBTooOldToUpgrade")

	// ErrUnknownDeploymentID indicates a deployment id does not exist.
	ErrUnknownDeploymentID = ErrorKind("ErrUnknownDeploymentID")

	// ErrUnknownDeploymentVersion indicates a version for a given deployment id
	// was specified that does not exist.
	ErrUnknownDeploymentVersion = ErrorKind("ErrUnknownDeploymentVersion")

	// ErrDuplicateDeployment indicates a duplicate deployment id exists in the
	// network parameter deployment definitions.
	ErrDuplicateDeployment = ErrorKind("ErrDuplicateDeployment")

	// ErrUnknownBlock indicates a requested block does not exist.
	ErrUnknownBlock = ErrorKind("ErrUnknownBlock")

	// ErrNoFilter indicates a filter for a given block hash does not exist.
	ErrNoFilter = ErrorKind("ErrNoFilter")

	// ErrNoTreasuryBalance indicates the treasury balance for a given block
	// hash does not exist.
	ErrNoTreasuryBalance = ErrorKind("ErrNoTreasuryBalance")

	// ErrInvalidateGenesisBlock indicates an attempt to invalidate the genesis
	// block which is not allowed.
	ErrInvalidateGenesisBlock = ErrorKind("ErrInvalidateGenesisBlock")

	// ErrSerializeHeader indicates an attempt to serialize a block header failed.
	ErrSerializeHeader = ErrorKind("ErrSerializeHeader")

	// ------------------------------------------
	// Errors related to the UTXO backend.
	// ------------------------------------------

	// ErrUtxoBackend indicates that a general error was encountered when
	// accessing the UTXO backend.
	ErrUtxoBackend = ErrorKind("ErrUtxoBackend")

	// ErrUtxoBackendCorruption indicates that underlying data being accessed in
	// the UTXO backend is corrupted.
	ErrUtxoBackendCorruption = ErrorKind("ErrUtxoBackendCorruption")

	// ErrUtxoBackendNotOpen indicates that the UTXO backend was accessed before
	// it was opened or after it was closed.
	ErrUtxoBackendNotOpen = ErrorKind("ErrUtxoBackendNotOpen")

	// ErrUtxoBackendTxClosed indicates an attempt was made to commit or rollback
	// a UTXO backend transaction that has already had one of those operations
	// performed.
	ErrUtxoBackendTxClosed = ErrorKind("ErrUtxoBackendTxClosed")

	// -----------------------------------------------------------------
	// Errors related to the automatic ticket revocations agenda.
	// -----------------------------------------------------------------

	// ErrInvalidRevocationTxVersion indicates that the revocation is the wrong
	// transaction version.
	ErrInvalidRevocationTxVersion = ErrorKind("ErrInvalidRevocationTxVersion")

	// ErrNoExpiredTicketRevocation indicates that the block does not contain a
	// revocation for a ticket that is becoming expired as of that block.
	ErrNoExpiredTicketRevocation = ErrorKind("ErrNoExpiredTicketRevocation")

	// ErrNoMissedTicketRevocation indicates that the block does not contain a
	// revocation for a ticket that is becoming missed as of that block.
	ErrNoMissedTicketRevocation = ErrorKind("ErrNoMissedTicketRevocation")
)

// Error satisfies the error interface and prints human-readable errors.
func (e ErrorKind) Error() string {
	return string(e)
}

// ContextError wraps an error with additional context.  It has full support for
// errors.Is and errors.As, so the caller can ascertain the specific wrapped
// error.
//
// RawErr contains the original error in the case where an error has been
// converted.
type ContextError struct {
	Err         error
	Description string
	RawErr      error
}

// Error satisfies the error interface and prints human-readable errors.
func (e ContextError) Error() string {
	return e.Description
}

// Unwrap returns the underlying wrapped error.
func (e ContextError) Unwrap() error {
	return e.Err
}

// contextError creates a ContextError given a set of arguments.
func contextError(kind ErrorKind, desc string) ContextError {
	return ContextError{Err: kind, Description: desc}
}

// unknownBlockError create a ContextError with the kind of error set to
// ErrUnknownBlock and a description that includes the provided hash.
func unknownBlockError(hash *chainhash.Hash) ContextError {
	str := fmt.Sprintf("block %s is not known", hash)
	return contextError(ErrUnknownBlock, str)
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

// MultiError houses several errors as a single error that provides full support
// for errors.Is and errors.As so the caller can easily determine if any of the
// errors match any specific error or error type.  Note that this differs from
// typical wrapped error chains which only represent a single error.
type MultiError []error

// Error satisfies the error interface and prints human-readable errors.
func (e MultiError) Error() string {
	if len(e) == 1 {
		return e[0].Error()
	}

	var builder strings.Builder
	builder.WriteString("multiple errors (")
	builder.WriteString(strconv.Itoa(len(e)))
	builder.WriteString("):\n")
	const maxErrs = 5
	i := 0
	for ; i < len(e) && i < maxErrs; i++ {
		builder.WriteString(" - ")
		builder.WriteString(e[i].Error())
		builder.WriteRune('\n')
	}
	if len(e) > maxErrs {
		builder.WriteString(" - ... ")
		builder.WriteString(strconv.Itoa(len(e) - maxErrs))
		builder.WriteString(" more error(s)")
		builder.WriteRune('\n')
	}

	return builder.String()
}

// Is implements the interface to work with the standard library's errors.Is.
//
// It iterates each of the errors in the multi error and calls errors.Is on it
// until the first one that matches target is found, in which case it returns
// true.  Otherwise, it returns false.
//
// This means it keeps all of the same semantics typically provided by Is in
// terms of unwrapping error chains.
func (e MultiError) Is(target error) bool {
	for _, err := range e {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// As implements the interface to work with the standard library's errors.As.
//
// It iterates each of the errors in the multi error and calls errors.As on it
// until the first one that matches target is found, in which case it returns
// true.  Otherwise, it returns false.
//
// This means it keeps all of the same semantics typically provided by As in
// terms of unwrapping error chains and setting the target to the matched error.
func (e MultiError) As(target interface{}) bool {
	for _, err := range e {
		if errors.As(err, target) {
			return true
		}
	}
	return false
}
