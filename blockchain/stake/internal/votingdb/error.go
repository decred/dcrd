// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package votingdb

import (
	"fmt"
)

// ErrorCode identifies a kind of error.
type ErrorCode int

// These constants are used to identify a specific RuleError.
const (
	// ErrUninitializedBucket indicates that a database bucket was not
	// initialized and therefore could not be written to or read from.
	ErrUninitializedBucket = iota

	// ErrMissingKey indicates that a key was not found in a bucket.
	ErrMissingKey

	// ErrChainStateShortRead indicates that the given chain state data
	// was too small.
	ErrChainStateShortRead

	// ErrDatabaseInfoShortRead indicates that the given database information
	// was too small.
	ErrDatabaseInfoShortRead

	// ErrTallyShortRead indicates that the given voting tally information
	// was too small.
	ErrTallyShortRead

	// ErrBlockKeyShortRead indicates that the given voting block key
	// was too small.
	ErrBlockKeyShortRead
)

// Map of ErrorCode values back to their constant names for pretty printing.
var errorCodeStrings = map[ErrorCode]string{
	ErrUninitializedBucket:   "ErrUninitializedBucket",
	ErrMissingKey:            "ErrMissingKey",
	ErrChainStateShortRead:   "ErrChainStateShortRead",
	ErrDatabaseInfoShortRead: "ErrDatabaseInfoShortRead",
	ErrTallyShortRead:        "ErrTallyShortRead",
	ErrBlockKeyShortRead:     "ErrBlockKeyShortRead",
}

// String returns the ErrorCode as a human-readable name.
func (e ErrorCode) String() string {
	if s := errorCodeStrings[e]; s != "" {
		return s
	}
	return fmt.Sprintf("Unknown ErrorCode (%d)", int(e))
}

// DBError identifies a an error in the stake database for tickets.
// The caller can use type assertions to determine if a failure was
// specifically due to a rule violation and access the ErrorCode field to
// ascertain the specific reason for the rule violation.
type DBError struct {
	ErrorCode   ErrorCode // Describes the kind of error
	Description string    // Human readable description of the issue
}

// Error satisfies the error interface and prints human-readable errors.
func (e DBError) Error() string {
	return e.Description
}

// GetCode satisfies the error interface and prints human-readable errors.
func (e DBError) GetCode() ErrorCode {
	return e.ErrorCode
}

// votingDBError creates a DBError given a set of arguments.
func votingDBError(c ErrorCode, desc string) DBError {
	return DBError{ErrorCode: c, Description: desc}
}
