// Copyright (c) 2019-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package standalone

// ErrorKind identifies a kind of error.  It has full support for errors.Is and
// errors.As, so the caller can directly check against an error kind when
// determining the reason for an error.
type ErrorKind string

// These constants are used to identify a specific RuleError.
const (
	// ErrUnexpectedDifficulty indicates specified bits do not align with
	// the expected value either because it doesn't match the calculated
	// value based on difficulty rules or it is out of the valid range.
	ErrUnexpectedDifficulty = ErrorKind("ErrUnexpectedDifficulty")

	// ErrHighHash indicates the block does not hash to a value which is
	// lower than the required target difficultly.
	ErrHighHash = ErrorKind("ErrHighHash")

	// ErrInvalidTSpendExpiry indicates that an invalid expiry was
	// provided when calculating the treasury spend voting window.
	ErrInvalidTSpendExpiry = ErrorKind("ErrInvalidTSpendExpiry")

	// ErrNoTxInputs indicates a transaction does not have any inputs.  A valid
	// transaction must have at least one input.
	ErrNoTxInputs = ErrorKind("ErrNoTxInputs")

	// ErrNoTxOutputs indicates a transaction does not have any outputs.  A
	// valid transaction must have at least one output.
	ErrNoTxOutputs = ErrorKind("ErrNoTxOutputs")

	// ErrTxTooBig indicates a transaction exceeds the maximum allowed size when
	// serialized.
	ErrTxTooBig = ErrorKind("ErrTxTooBig")

	// ErrBadTxOutValue indicates an output value for a transaction is
	// invalid in some way such as being out of range.
	ErrBadTxOutValue = ErrorKind("ErrBadTxOutValue")

	// ErrDuplicateTxInputs indicates a transaction references the same
	// input more than once.
	ErrDuplicateTxInputs = ErrorKind("ErrDuplicateTxInputs")
)

// Error satisfies the error interface and prints human-readable errors.
func (e ErrorKind) Error() string {
	return string(e)
}

// RuleError identifies a rule violation. It has full support for errors.Is
// and errors.As, so the caller can ascertain the specific reason for the
// error by checking the underlying error.
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

// ruleError creates a RuleError given a set of arguments.
func ruleError(kind ErrorKind, desc string) RuleError {
	return RuleError{Err: kind, Description: desc}
}
