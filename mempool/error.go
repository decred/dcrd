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

// RuleError identifies a rule violation.  It is used to indicate that
// processing of a transaction failed due to one of the many validation
// rules.  The caller can use type assertions to determine if a failure was
// specifically due to a rule violation and use the Err field to access the
// underlying error, which will be either a TxRuleError or a
// blockchain.RuleError.
type RuleError struct {
	Err error
}

// Error satisfies the error interface and prints human-readable errors.
func (e RuleError) Error() string {
	if e.Err == nil {
		return "<nil>"
	}
	return e.Err.Error()
}

// ErrorCode identifies the kind of error.
type ErrorCode int

const (
	ErrOther ErrorCode = iota
	ErrInvalid
	ErrOrphanPolicyViolation
	ErrMempoolDoubleSpend
	ErrAlreadyVoted
	ErrDuplicate
	ErrCoinbase
	ErrExpired
	ErrNonStandard
	ErrDustOutput
	ErrInsufficientFee
	ErrTooManyVotes
	ErrDuplicateRevocation
	ErrOldVote
	ErrAlreadyExists
	ErrSeqLockUnmet
	ErrInsufficientPriority
	ErrFeeTooHigh
	ErrOrphan
)

// TxRuleError identifies a rule violation.  It is used to indicate that
// processing of a transaction failed due to one of the many validation
// rules.  The caller can use type assertions to determine if a failure was
// specifically due to a rule violation and access the ErrorCode field to
// ascertain the specific reason for the rule violation.
type TxRuleError struct {
	// RejectCode is the corresponding rejection code to send when
	// reporting the error via 'reject' wire protocol messages.
	//
	// Deprecated: This will be removed in the next major version. Use
	// ErrorCode instead.
	RejectCode wire.RejectCode

	// ErrorCode is the mempool package error code ID.
	ErrorCode ErrorCode

	// Description is an additional human readable description of the
	// error.
	Description string
}

// Error satisfies the error interface and prints human-readable errors.
func (e TxRuleError) Error() string {
	return e.Description
}

// txRuleError creates an underlying TxRuleError with the given a set of
// arguments and returns a RuleError that encapsulates it.
func txRuleError(c wire.RejectCode, code ErrorCode, desc string) RuleError {
	return RuleError{
		Err: TxRuleError{RejectCode: c, ErrorCode: code, Description: desc},
	}
}

// chainRuleError returns a RuleError that encapsulates the given
// blockchain.RuleError.
func chainRuleError(chainErr blockchain.RuleError) RuleError {
	return RuleError{
		Err: chainErr,
	}
}

// IsErrorCode returns true if the passed error encodes a TxRuleError with the
// given ErrorCode, either directly or embedded in an outer RuleError.
func IsErrorCode(err error, code ErrorCode) bool {
	// Unwrap RuleError if necessary.
	var rerr RuleError
	if errors.As(err, &rerr) {
		err = rerr.Err
	}

	var trerr TxRuleError
	return errors.As(err, &trerr) &&
		trerr.ErrorCode == code
}

// wrapTxRuleError returns a new RuleError with an underlying TxRuleError,
// replacing the description with the provided one while retaining both the
// error code and rejection code from the original error if they can be
// determined.
func wrapTxRuleError(rejectCode wire.RejectCode, errorCode ErrorCode, desc string, err error) error {
	// Unwrap the underlying error if err is a RuleError
	var rerr RuleError
	if errors.As(err, &rerr) {
		err = rerr.Err
	}

	// Override the passed rejectCode and errorCode with the ones from the
	// error, if it is a TxRuleError
	var txerr TxRuleError
	if errors.As(err, &txerr) {
		rejectCode = txerr.RejectCode
		errorCode = txerr.ErrorCode
	}

	// Fill a default error description if empty.
	if desc == "" {
		desc = fmt.Sprintf("rejected: %v", err)
	}

	return txRuleError(rejectCode, errorCode, desc)
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
		return terr.RejectCode, true

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
