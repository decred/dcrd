// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gcs

import (
	"fmt"
)

// ErrorCode identifies a kind of error.
type ErrorCode int

// These constants are used to identify a specific RuleError.
const (
	// ErrNTooBig signifies that the filter can't handle N items.
	ErrNTooBig ErrorCode = iota

	// ErrPTooBig signifies that the filter can't handle `1/2**P`
	// collision probability.
	ErrPTooBig

	// ErrMisserialized signifies a filter was misserialized and is missing the
	// N and/or P parameters of a serialized filter.
	ErrMisserialized

	// numErrorCodes is the maximum error code number used in tests.
	numErrorCodes
)

// Map of ErrorCode values back to their constant names for pretty printing.
var errorCodeStrings = map[ErrorCode]string{
	ErrNTooBig:       "ErrNTooBig",
	ErrPTooBig:       "ErrPTooBig",
	ErrMisserialized: "ErrMisserialized",
}

// String returns the ErrorCode as a human-readable name.
func (e ErrorCode) String() string {
	if s := errorCodeStrings[e]; s != "" {
		return s
	}
	return fmt.Sprintf("Unknown ErrorCode (%d)", int(e))
}

// Error identifies a filter-related error.  The caller can use type assertions
// to access the ErrorCode field to ascertain the specific reason for the
// failure.
type Error struct {
	ErrorCode   ErrorCode // Describes the kind of error
	Description string    // Human readable description of the issue
}

// Error satisfies the error interface and prints human-readable errors.
func (e Error) Error() string {
	return e.Description
}

// makeError creates an Error given a set of arguments.  The error code must
// be one of the error codes provided by this package.
func makeError(c ErrorCode, desc string) Error {
	return Error{ErrorCode: c, Description: desc}
}

// IsError returns whether err is an Error with a matching error code.
func IsErrorCode(err error, c ErrorCode) bool {
	e, ok := err.(Error)
	return ok && e.ErrorCode == c
}
