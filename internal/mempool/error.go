// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"errors"
	"fmt"

	"github.com/decred/dcrd/blockchain/v3"
	"github.com/decred/dcrd/wire"
)

// ErrorKind identifies a kind of error.  It has full support for errors.Is and
// errors.As, so the caller can directly check against an error kind when
// determining the reason for an error.
type ErrorKind string

const (
	// ErrorInvalid indicates the mempool transaction is invalid per consensus.
	ErrInvalid = ErrorKind("ErrInvalid")

	// ErrorOrphanPolicyViolation indicates that the orphan block violates the
	// prevailing orphan policy.
	ErrOrphanPolicyViolation = ErrorKind("ErrOrphanPolicyViolation")

	// ErrMempoolDoubleSpend indicates the  transaction attempts to
	// double spend already spend outputs.
	ErrMempoolDoubleSpend = ErrorKind("ErrMempoolDoubleSpend")

	// ErrAlreadyVoted indicates the ticket already voted.
	ErrAlreadyVoted = ErrorKind("ErrorAlreadyVoted")

	// ErrDuplicate indicates the transaction already exists in the mempool.
	ErrDuplicate = ErrorKind("ErrDuplicate")

	// ErrCoinbase indicates the transaction is a standalone coinbase transaction.
	ErrCoinbase = ErrorKind("ErrCoinbase")

	// ErrExpired indicates the transaction will expire the next block.
	ErrExpired = ErrorKind("ErrExpired")

	// ErrNontandard indicates a non-standard transaction.
	ErrNonStandard = ErrorKind("ErrNonStandard")

	// ErrDustOutput indicates the transaction has dust outputs.
	ErrDustOutput = ErrorKind("ErrDustOutput")

	// ErrInsufficientFee indicates the transaction cannot settle its fees.
	ErrInsufficientFee = ErrorKind("ErrInsufficientFee")

	// ErrTooManyVotes indicates the number of vote double spends exceeds the
	// maximum allowed.
	ErrTooManyVotes = ErrorKind("ErrTooManyVotes")

	// ErrDuplicateRevocation indicates the revocation already exists in the
	// mempool.
	ErrDuplicateRevocation = ErrorKind("ErrDuplicateRevocation")

	// ErrOldVote indicates the tickets votes on an old block height lower than
	// the minimum allowed by the mempool.
	ErrOldVote = ErrorKind("ErrOldVote")

	// ErrAlreadyExists indicates the transaction already exists on the
	// main chain and is fully spent.
	ErrAlreadyExists = ErrorKind("ErrAlreadyExists")

	// ErrSeqLockUnmet indicates the transaction sequence locks are not active.
	ErrSeqLockUnmet = ErrorKind("ErrSeqLockUnmet")

	// ErrInsufficientPriority indicates the non-stake transaction has a
	// priority lower than the minimum allowed.
	ErrInsufficientPriority = ErrorKind("ErrInsufficientPriority")

	// ErrFeeTooHigh indicates the transaction pays fees above the maximum
	// allowed by the mempool.
	ErrFeeTooHigh = ErrorKind("ErrFeeTooHigh")

	// ErrOrphan indicates the tranaction is an orphan.
	ErrOrphan = ErrorKind("ErrOrphan")
)

// Error satisfies the error interface and prints human-readable errors.
func (e ErrorKind) Error() string {
	return string(e)
}

// RuleError identifies a rule violation.  It is used to indicate that
// processing of a transaction failed due to one of the many validation
// rules.  It has full support for errors.Is and errors.As, so the caller
// can ascertain the specific reason for the error by checking the
// underlying error, which will be either a TxRuleError or a
// blockchain.RuleError.
type RuleError struct {
	Description string
	Err         error
}

// Error satisfies the error interface and prints human-readable errors.
func (e RuleError) Error() string {
	return e.Description
}

// Unwrap returns the underlying wrapped error.
func (e RuleError) Unwrap() error {
	return e.Err
}

// ruleError creates an Error given a set of arguments.
func ruleError(kind ErrorKind, desc string) RuleError {
	return RuleError{Err: kind, Description: desc}
}

// chainRuleError returns a RuleError that encapsulates the given
// blockchain.RuleError.
func chainRuleError(chainErr blockchain.RuleError) RuleError {
	return RuleError{
		Err:         chainErr.Err,
		Description: chainErr.Description,
	}
}

// TxRuleError identifies a rule violation.  It is used to indicate that
// processing of a transaction failed due to one of the many validation
// rules.  It has full support for errors.Is and errors.As, so the caller
// can ascertain the specific reason for the error by checking the
// underlying error.
type TxRuleError struct {
	Description string
	Err         error
}

// Error satisfies the error interface and prints human-readable errors.
func (e TxRuleError) Error() string {
	return e.Description
}

// Unwrap returns the underlying wrapped error.
func (e TxRuleError) Unwrap() error {
	return e.Err
}

// txRuleError creates an Error given a set of arguments.
func txRuleError(kind ErrorKind, desc string) TxRuleError {
	return TxRuleError{Err: kind, Description: desc}
}

// wrapTxRuleError returns a new RuleError with an underlying TxRuleError,
// replacing the description with the provided one while retaining the error
// kind from the original error if it can be determined.
func wrapTxRuleError(errKind ErrorKind, desc string, err error) error {
	var kind ErrorKind

	// Unwrap the underlying error if err is a RuleError.
	var rerr RuleError
	if errors.As(err, &rerr) {
		kind = rerr.Unwrap().(ErrorKind)
	} else {
		kind = errKind
	}

	// Fill a default error description if empty.
	if desc == "" {
		desc = fmt.Sprintf("rejected: %v", err)
	}

	return txRuleError(kind, desc)
}

// extractRejectCode attempts to return a relevant reject code for a given error
// by examining the error for known types.  It will return true if a code
// was successfully extracted.
func extractRejectCode(err error) (wire.RejectCode, bool) {
	// Pull the underlying error out of a RuleError.
	var rerr RuleError
	if errors.As(err, &rerr) {
		err = rerr.Err
	}

	var berr blockchain.RuleError
	var terr TxRuleError
	switch {
	case errors.As(err, &berr):
		// Convert the chain error to a reject code.
		var code wire.RejectCode
		switch berr.Err {
		// Rejected due to duplicate.
		case blockchain.ErrDuplicateBlock:
			code = wire.RejectDuplicate

		// Rejected due to obsolete version.
		case blockchain.ErrBlockVersionTooOld:
			code = wire.RejectObsolete

		// Rejected due to checkpoint.
		case blockchain.ErrCheckpointTimeTooOld:
			fallthrough
		case blockchain.ErrDifficultyTooLow:
			fallthrough
		case blockchain.ErrBadCheckpoint:
			fallthrough
		case blockchain.ErrForkTooOld:
			code = wire.RejectCheckpoint

		// Everything else is due to the block or transaction being invalid.
		default:
			code = wire.RejectInvalid
		}

		return code, true

	case errors.As(err, &terr):
		// Convert the chain error to a reject code.
		var code wire.RejectCode
		switch terr.Err {
		// Rejected due to insufficient feesq.
		case ErrInsufficientFee:
			code = wire.RejectInvalid

		// Rejected due to non-standardness.
		case ErrNonStandard:
			code = wire.RejectNonstandard

		// Rejected due to dust output.
		case ErrDustOutput:
			code = wire.RejectDust

		// Rejected due to duplicates.
		case ErrDuplicate:
			fallthrough
		case ErrDuplicateRevocation:
			code = wire.RejectDuplicate

		// Everything else is due to the block or transaction being invalid.
		default:
			code = wire.RejectInvalid
		}
		return code, true

	case err == nil:
		return wire.RejectInvalid, false
	}

	return wire.RejectInvalid, false
}

// ErrToRejectErr examines the underlying type of the error and returns a reject
// code and string appropriate to be sent in a wire.MsgReject message.
//
// Deprecated: This will be removed in the next major version of this package.
func ErrToRejectErr(err error) (wire.RejectCode, string) {
	// Return the reject code along with the error text if it can be
	// extracted from the error.
	rejectCode, found := extractRejectCode(err)
	if found {
		return rejectCode, err.Error()
	}

	// Return a generic rejected string if there is no error.  This really
	// should not happen unless the code elsewhere is not setting an error
	// as it should be, but it's best to be safe and simply return a generic
	// string rather than allowing the following code that dereferences the
	// err to panic.
	if err == nil {
		return wire.RejectInvalid, "rejected"
	}

	// When the underlying error is not one of the above cases, just return
	// wire.RejectInvalid with a generic rejected string plus the error
	// text.
	return wire.RejectInvalid, "rejected: " + err.Error()
}
