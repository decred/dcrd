// Copyright (c) 2014 Conformal Systems LLC.
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

// ErrorKind identifies a kind of error.  It has full support for errors.Is and
// errors.As, so the caller can directly check against an error kind when
// determining the reason for an error.
type ErrorKind string

// These constants are used to identify a specific RuleError.
const (
	// ErrSStxTooManyInputs indicates that a given SStx contains too many
	// inputs.
	ErrSStxTooManyInputs = ErrorKind("ErrSStxTooManyInputs")

	// ErrSStxTooManyOutputs indicates that a given SStx contains too many
	// outputs.
	ErrSStxTooManyOutputs = ErrorKind("ErrSStxTooManyOutputs")

	// ErrSStxNoOutputs indicates that a given SStx has no outputs.
	ErrSStxNoOutputs = ErrorKind("ErrSStxNoOutputs")

	// ErrSStxInvalidInput indicates that an invalid output has been used as
	// an input for a SStx; only non-SStx tagged outputs may be used to
	// purchase stake tickets.
	// TODO: Add this into validate
	// Ensure that all inputs are not tagged SStx outputs of some sort,
	// along with checks to make sure they exist and are available.
	ErrSStxInvalidInputs = ErrorKind("ErrSStxInvalidInputs")

	// ErrSStxInvalidOutput indicates that the output for an SStx tx is
	// invalid; in particular, either the output was not tagged SStx or the
	// OP_RETURNs were missing or contained invalid addresses.
	ErrSStxInvalidOutputs = ErrorKind("ErrSStxInvalidOutputs")

	// ErrSStxInOutProportions indicates the number of inputs in an SStx
	// was not equal to the number of output minus one.
	ErrSStxInOutProportions = ErrorKind("ErrSStxInOutProportions")

	// ErrSStxBadCommitAmount indicates that a ticket tried to commit 0 or
	// a negative value as the commitment amount.
	ErrSStxBadCommitAmount = ErrorKind("ErrSStxBadCommitAmount")

	// ErrSStxBadChangeAmts indicates that the change amount for some SStx
	// was invalid, for instance spending more than its inputs.
	ErrSStxBadChangeAmts = ErrorKind("ErrSStxBadChangeAmts")

	// ErrSStxVerifyCalcAmts indicates that passed calculated amounts failed
	// to conform to the amounts found in the ticket.
	ErrSStxVerifyCalcAmts = ErrorKind("ErrSStxVerifyCalcAmts")

	// ErrSSGenWrongNumInputs indicates that a given SSGen tx contains an
	// invalid number of inputs.
	ErrSSGenWrongNumInputs = ErrorKind("ErrSSGenWrongNumInputs")

	// ErrSSGenTooManyOutputs indicates that a given SSGen tx contains too
	// many outputs.
	ErrSSGenTooManyOutputs = ErrorKind("ErrSSGenTooManyOutputs")

	// ErrSSGenNoOutputs indicates that a given SSGen has no outputs.
	ErrSSGenNoOutputs = ErrorKind("ErrSSGenNoOutputs")

	// ErrSSGenWrongIndex indicates that a given SSGen sstx input was not
	// using the correct index.
	ErrSSGenWrongIndex = ErrorKind("ErrSSGenWrongIndex")

	// ErrSSGenWrongTxTree indicates that a given SSGen tx input was not found in
	// the stake tx tree.
	ErrSSGenWrongTxTree = ErrorKind("ErrSSGenWrongTxTree")

	// ErrSSGenNoStakebase indicates that the SSGen tx did not contain a
	// valid StakeBase in the zeroeth position of inputs.
	ErrSSGenNoStakebase = ErrorKind("ErrSSGenNoStakebase")

	// ErrSSGenNoReference indicates that there is no reference OP_RETURN
	// included as the first output.
	ErrSSGenNoReference = ErrorKind("ErrSSGenNoReference")

	// ErrSSGenNoReference indicates that the OP_RETURN included as the
	// first output was corrupted in some way.
	ErrSSGenBadReference = ErrorKind("ErrSSGenBadReference")

	// ErrSSGenNoVotePush indicates that there is no vote bits OP_RETURN
	// included as the second output.
	ErrSSGenNoVotePush = ErrorKind("ErrSSGenNoVotePush")

	// ErrSSGenBadVotePush indicates that the OP_RETURN included as the
	// second output was corrupted in some way.
	ErrSSGenBadVotePush = ErrorKind("ErrSSGenBadVotePush")

	// ErrSSGenInvalidDiscriminatorLength indicates that the discriminator
	// length is too short.
	ErrSSGenInvalidDiscriminatorLength = ErrorKind("ErrSSGenInvalidDiscriminatorLength")

	// ErrSSGenInvalidNullScript indicates that the passed script is not a
	// valid nullscript.
	ErrSSGenInvalidNullScript = ErrorKind("ErrSSGenInvalidNullScript")

	// ErrSSGenInvalidTVLength indicates that this is an invalid Treasury
	// Vote length.
	ErrSSGenInvalidTVLength = ErrorKind("ErrSSGenInvalidTVLength")

	// ErrSSGenInvalidTreasuryVote indicates that this is an invalid
	// treasury vote.
	ErrSSGenInvalidTreasuryVote = ErrorKind("ErrSSGenInvalidTreasuryVote")

	// ErrSSGenDuplicateTreasuryVote indicates that there is a duplicate
	// treasury vote.
	ErrSSGenDuplicateTreasuryVote = ErrorKind("ErrSSGenDuplicateTreasuryVote")

	// ErrSSGenInvalidTxVersion indicates that this transaction has the
	// wrong version.
	ErrSSGenInvalidTxVersion = ErrorKind("ErrSSGenInvalidTxVersion")

	// ErrSSGenUnknownDiscriminator indicates that the supplied
	// discriminator is unsupported.
	ErrSSGenUnknownDiscriminator = ErrorKind("ErrSSGenUnknownDiscriminator")

	// ErrSSGenBadGenOuts indicates that something was wrong with the stake
	// generation outputs that were present after the first two OP_RETURN
	// pushes in an SSGen tx.
	ErrSSGenBadGenOuts = ErrorKind("ErrSSGenBadGenOuts")

	// ErrSSRtxWrongNumInputs indicates that a given SSRtx contains an
	// invalid number of inputs.
	ErrSSRtxWrongNumInputs = ErrorKind("ErrSSRtxWrongNumInputs")

	// ErrSSRtxTooManyOutputs indicates that a given SSRtx contains too many
	// outputs.
	ErrSSRtxTooManyOutputs = ErrorKind("ErrSSRtxTooManyOutputs")

	// ErrSSRtxNoOutputs indicates that a given SSRtx has no outputs.
	ErrSSRtxNoOutputs = ErrorKind("ErrSSRtxNoOutputs")

	// ErrSSRtxWrongTxTree indicates that a given SSRtx input was not found in
	// the stake tx tree.
	ErrSSRtxWrongTxTree = ErrorKind("ErrSSRtxWrongTxTree")

	// ErrSSRtxBadGenOuts indicates that there was a non-SSRtx tagged output
	// present in an SSRtx.
	ErrSSRtxBadOuts = ErrorKind("ErrSSRtxBadOuts")

	// ErrVerSStxAmts indicates there was an error verifying the calculated
	// SStx out amounts and the actual SStx out amounts.
	ErrVerSStxAmts = ErrorKind("ErrVerSStxAmts")

	// ErrVerifyInput indicates that there was an error in verification
	// function input.
	ErrVerifyInput = ErrorKind("ErrVerifyInput")

	// ErrVerifyOutType indicates that there was a non-equivalence in the
	// output type.
	ErrVerifyOutType = ErrorKind("ErrVerifyOutType")

	// ErrVerifyTooMuchFees indicates that a transaction's output gave
	// too much in fees after taking into accounts the limits imposed
	// by the SStx output's version field.
	ErrVerifyTooMuchFees = ErrorKind("ErrVerifyTooMuchFees")

	// ErrVerifySpendTooMuch indicates that a transaction's output spent more
	// than it was allowed to spend based on the calculated subsidy or return
	// for a vote or revocation.
	ErrVerifySpendTooMuch = ErrorKind("ErrVerifySpendTooMuch")

	// ErrVerifyOutputAmt indicates that for a vote/revocation spend output,
	// the rule was given that it must exactly match the calculated maximum,
	// however the amount in the output did not (e.g. it gave fees).
	ErrVerifyOutputAmt = ErrorKind("ErrVerifyOutputAmt")

	// ErrVerifyOutPkhs indicates that the recipient of the P2PKH or P2SH
	// script was different from that indicated in the SStx input.
	ErrVerifyOutPkhs = ErrorKind("ErrVerifyOutPkhs")

	// ErrDatabaseCorrupt indicates a database inconsistency.
	ErrDatabaseCorrupt = ErrorKind("ErrDatabaseCorrupt")

	// ErrMissingDatabaseTx indicates that a node disconnection failed to
	// pass a database transaction when attempted to remove a very old
	// node.
	ErrMissingDatabaseTx = ErrorKind("ErrMissingDatabaseTx")

	// ErrMemoryCorruption indicates that memory has somehow become corrupt,
	// for example invalid block header serialization from an in memory
	// struct.
	ErrMemoryCorruption = ErrorKind("ErrMemoryCorruption")

	// ErrFindTicketIdxs indicates a failure to find the selected ticket
	// indexes from the block header.
	ErrFindTicketIdxs = ErrorKind("ErrFindTicketIdxs")

	// ErrMissingTicket indicates that a ticket was missing in one of the
	// ticket treaps when it was attempted to be fetched.
	ErrMissingTicket = ErrorKind("ErrMissingTicket")

	// ErrDuplicateTicket indicates that a duplicate ticket was attempted
	// to be inserted into a ticket treap or the database.
	ErrDuplicateTicket = ErrorKind("ErrDuplicateTicket")

	// ErrUnknownTicketSpent indicates that an unknown ticket was spent by
	// the block.
	ErrUnknownTicketSpent = ErrorKind("ErrUnknownTicketSpent")

	// ErrTAddInvalidTxVersion indicates that this transaction has the
	// wrong version.
	ErrTAddInvalidTxVersion = ErrorKind("ErrTAddInvalidTxVersion")

	// ErrTAddInvalidCount indicates that this transaction contains an
	// invalid TADD script count.
	ErrTAddInvalidCount = ErrorKind("ErrTAddInvalidCount")

	// ErrTAddInvalidVersion indicates that this transaction has an invalid
	// version in an output script.
	ErrTAddInvalidVersion = ErrorKind("ErrTAddInvalidVersion")

	// ErrTAddInvalidScriptLength indicates that this transaction has a zero
	// length output script.
	ErrTAddInvalidScriptLength = ErrorKind("ErrTAddInvalidScriptLength")

	// ErrTAddInvalidLength indicates that output 0 is not exactly 1 byte.
	ErrTAddInvalidLength = ErrorKind("ErrTAddInvalidLength")

	// ErrTAddInvalidOpcode indicates that this transaction does not have a
	// TADD opcode.
	ErrTAddInvalidOpcode = ErrorKind("ErrTAddInvalidOpcode")

	// ErrTAddInvalidChange indicates that this transaction contains an
	// invalid change script.
	ErrTAddInvalidChange = ErrorKind("ErrTAddInvalidChange")

	// ErrTSpendInvalidTxVersion indicates that this transaction has
	// the wrong version.
	ErrTSpendInvalidTxVersion = ErrorKind("ErrTSpendInvalidTxVersion")

	// ErrTSpendInvalidLength indicates that this transaction has an
	// invalid number of inputs and/or outputs.
	ErrTSpendInvalidLength = ErrorKind("ErrTSpendInvalidLength")

	// ErrTSpendInvalidVersion indicates that this transaction has an
	// invalid version in an output script.
	ErrTSpendInvalidVersion = ErrorKind("ErrTSpendInvalidVersion")

	// ErrTSpendInvalidScriptLength indicates that the TSPEND transaction
	// has outputs that have a zero length script.
	ErrTSpendInvalidScriptLength = ErrorKind("ErrTSpendInvalidScriptLength")

	// ErrTSpendInvalidPubkey indicates that this transaction contains an
	// invalid public key.
	ErrTSpendInvalidPubkey = ErrorKind("ErrTSpendInvalidPubkey")

	// ErrTSpendInvalidScript indicates that this transaction does not
	// match the required TSPEND transaction signature.
	ErrTSpendInvalidScript = ErrorKind("ErrTSpendInvalidScript")

	// ErrTSpendInvalidTGen indicates that the TSPEND transaction has
	// outputs that are not tagged with OP_TGEN.
	ErrTSpendInvalidTGen = ErrorKind("ErrTSpendInvalidTGen")

	// ErrTSpendInvalidTransaction indicates that a TSPEND transaction
	// output 0 does not have an a valid null data script.
	ErrTSpendInvalidTransaction = ErrorKind("ErrTSpendInvalidTransaction")

	// ErrTSpendInvalidSpendScript indicates that this transaction contains
	// an invalid P2SH or P2PKH script.
	ErrTSpendInvalidSpendScript = ErrorKind("ErrTSpendInvalidSpendScript")

	// ErrTreasuryBaseInvalidTxVersion indicates that this transaction has
	// the wrong version.
	ErrTreasuryBaseInvalidTxVersion = ErrorKind("ErrTreasuryBaseInvalidTxVersion")

	// ErrTreasuryBaseInvalidCount indicates that this transaction contains
	// an invalid treasury base script count.
	ErrTreasuryBaseInvalidCount = ErrorKind("ErrTreasuryBaseInvalidCount")

	// ErrTreasuryBaseInvalidLength indicates that this transaction contains
	// an invalid treasury base input script length.
	ErrTreasuryBaseInvalidLength = ErrorKind("ErrTreasuryBaseInvalidLength")

	// ErrTreasuryBaseInvalidVersion indicates that this transaction has an
	// invalid version in an output script.
	ErrTreasuryBaseInvalidVersion = ErrorKind("ErrTreasuryBaseInvalidVersion")

	// ErrTreasuryBaseInvalidOpcode0 indicates that this transaction does
	// not have a TADD opcode in TxOut[0].
	ErrTreasuryBaseInvalidOpcode0 = ErrorKind("ErrTreasuryBaseInvalidOpcode0")

	// ErrTreasuryBaseInvalidOpcode1 indicates that this transaction does
	// not have an OP_RETURN opcode in TxOut[1].
	ErrTreasuryBaseInvalidOpcode1 = ErrorKind("ErrTreasuryBaseInvalidOpcode1")

	// ErrTreasuryBaseInvalid indicates that this transaction contains
	// invalid treasurybase TxIn constants.
	ErrTreasuryBaseInvalid = ErrorKind("ErrTreasuryBaseInvalid")
)

// Error satisfies the error interface and prints human-readable errors.
func (e ErrorKind) Error() string {
	return string(e)
}

// RuleError identifies a rule violation related to stake transactions. It has
// full support for errors.Is and errors.As, so the caller can ascertain the
// specific reason for the error by checking the underlying error.
type RuleError struct {
	Description string
	Err         error
}

// Error satisfies the error interface and prints human-readable errors.
func (e RuleError) Error() string {
	return e.Description
}

// Unwrap returns the underlying wrapped rule error.
func (e RuleError) Unwrap() error {
	return e.Err
}

// stakeRuleError creates a RuleError given a set of arguments.
func stakeRuleError(kind ErrorKind, desc string) RuleError {
	return RuleError{Err: kind, Description: desc}
}
