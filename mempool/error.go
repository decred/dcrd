// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"errors"
	"fmt"

	"github.com/decred/dcrd/blockchain/v3"
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
func txRuleError(code ErrorCode, desc string) RuleError {
	return RuleError{
		Err: TxRuleError{ErrorCode: code, Description: desc},
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
// replacing the description with the provided one while retaining the error
// code from the original error if it can be determined.
func wrapTxRuleError(errorCode ErrorCode, desc string, err error) error {
	// Unwrap the underlying error if err is a RuleError
	var rerr RuleError
	if errors.As(err, &rerr) {
		err = rerr.Err
	}

	// Override the passed error code with the ones from the error if it is a
	// TxRuleError.
	var txerr TxRuleError
	if errors.As(err, &txerr) {
		errorCode = txerr.ErrorCode
	}

	// Fill a default error description if empty.
	if desc == "" {
		desc = fmt.Sprintf("rejected: %v", err)
	}

	return txRuleError(errorCode, desc)
}
