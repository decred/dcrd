// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"errors"
	"fmt"

	"github.com/decred/dcrd/blockchain/v5"
)

// ErrorKind identifies a kind of error.  It has full support for errors.Is and
// errors.As, so the caller can directly check against an error kind when
// determining the reason for an error.
type ErrorKind string

const (
	// ErrInvalid indicates a mempool transaction is invalid per consensus.
	ErrInvalid = ErrorKind("ErrInvalid")

	// ErrOrphanPolicyViolation indicates that an orphan block violates the
	// prevailing orphan policy.
	ErrOrphanPolicyViolation = ErrorKind("ErrOrphanPolicyViolation")

	// ErrMempoolDoubleSpend indicates a transaction that attempts to to spend
	// coins already spent by other transactions in the pool.
	ErrMempoolDoubleSpend = ErrorKind("ErrMempoolDoubleSpend")

	// ErrAlreadyVoted indicates a ticket already voted.
	ErrAlreadyVoted = ErrorKind("ErrorAlreadyVoted")

	// ErrDuplicate indicates a transaction already exists in the mempool.
	ErrDuplicate = ErrorKind("ErrDuplicate")

	// ErrCoinbase indicates a transaction is a standalone coinbase transaction.
	ErrCoinbase = ErrorKind("ErrCoinbase")

	// ErrExpired indicates a transaction will be expired as of the next block.
	ErrExpired = ErrorKind("ErrExpired")

	// ErrNonStandard indicates a non-standard transaction.
	ErrNonStandard = ErrorKind("ErrNonStandard")

	// ErrDustOutput indicates a transaction has one or more dust outputs.
	ErrDustOutput = ErrorKind("ErrDustOutput")

	// ErrInsufficientFee indicates a transaction cannot does not pay the minimum
	// fee required by the active policy.
	ErrInsufficientFee = ErrorKind("ErrInsufficientFee")

	// ErrTooManyVotes indicates the number of vote double spends exceeds the
	// maximum allowed.
	ErrTooManyVotes = ErrorKind("ErrTooManyVotes")

	// ErrDuplicateRevocation indicates a revocation already exists in the
	// mempool.
	ErrDuplicateRevocation = ErrorKind("ErrDuplicateRevocation")

	// ErrOldVote indicates a ticket votes on a block height lower than
	// the minimum allowed by the mempool.
	ErrOldVote = ErrorKind("ErrOldVote")

	// ErrAlreadyExists indicates a transaction already exists on the
	// main chain and is not fully spent.
	ErrAlreadyExists = ErrorKind("ErrAlreadyExists")

	// ErrSeqLockUnmet indicates a transaction sequence locks are not active.
	ErrSeqLockUnmet = ErrorKind("ErrSeqLockUnmet")

	// ErrInsufficientPriority indicates a non-stake transaction has a
	// priority lower than the minimum allowed.
	ErrInsufficientPriority = ErrorKind("ErrInsufficientPriority")

	// ErrFeeTooHigh indicates a transaction pays fees above the maximum
	// allowed by the active policy.
	ErrFeeTooHigh = ErrorKind("ErrFeeTooHigh")

	// ErrOrphan indicates a transaction is an orphan.
	ErrOrphan = ErrorKind("ErrOrphan")

	// ErrTooManyTSpends indicates the number of treasury spend hashes exceeds
	// the maximum allowed.
	ErrTooManyTSpends = ErrorKind("ErrTooManyTSpends")

	// ErrTSpendMinedOnAncestor indicates a referenced treasury spend was
	// already mined on an ancestor block.
	ErrTSpendMinedOnAncestor = ErrorKind("ErrTSpendMinedOnAncestor")

	// ErrTSpendInvalidExpiry indicates a treasury spend expiry is invalid.
	ErrTSpendInvalidExpiry = ErrorKind("ErrTSpendInvalidExpiry")
)

// Error satisfies the error interface and prints human-readable errors.
func (e ErrorKind) Error() string {
	return string(e)
}

// RuleError identifies a rule violation.  It is used to indicate that
// processing of a transaction failed due to one of the many validation
// rules.  It has full support for errors.Is and errors.As, so the caller
// can ascertain the specific reason for the error by checking the
// underlying error, which will be either an ErrorKind or blockchain.RuleError.
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

// chainRuleError returns a RuleError that encapsulates the given
// blockchain.RuleError.
func chainRuleError(chainErr blockchain.RuleError) RuleError {
	return RuleError{Err: chainErr, Description: chainErr.Description}
}

// txRuleError creates a RuleError given a set of arguments.
func txRuleError(kind ErrorKind, desc string) RuleError {
	return RuleError{Err: kind, Description: desc}
}

// wrapTxRuleError returns a new RuleError with an underlying ErrorKind,
// replacing the description with the provided one while retaining the error
// kind from the original error if it can be determined.
func wrapTxRuleError(kind ErrorKind, desc string, err error) error {
	// Override the passed error kind with the one from the error if it is an
	// ErrorKind.
	var kerr ErrorKind
	if errors.As(err, &kerr) {
		kind = kerr
	}

	// Fill a default error description if empty.
	if desc == "" {
		desc = fmt.Sprintf("rejected: %v", err)
	}

	return txRuleError(kind, desc)
}
