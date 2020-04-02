// Copyright (c) 2014 Conformal Systems LLC.
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package schnorr

import (
	"fmt"
)

// ErrorCode identifies a kind of signature-related error.  It has full support
// for errors.Is and errors.As, so the caller can directly check against an
// error code when determining the reason for an error.
type ErrorCode int

// These constants are used to identify a specific RuleError.
const (
	// ErrInvalidHashLen indicates that the input hash to sign or verify is not
	// the required length.
	ErrInvalidHashLen ErrorCode = iota

	// ErrPrivateKeyIsZero indicates an attempt was made to sign a message with
	// a private key that is equal to zero.
	ErrPrivateKeyIsZero

	// ErrSchnorrHashValue indicates that the hash of (R || m) was too large and
	// so a new nonce should be used.
	ErrSchnorrHashValue

	// ErrPubKeyNotOnCurve indicates that a point was not on the given elliptic
	// curve.
	ErrPubKeyNotOnCurve

	// ErrSigRYIsOdd indicates that the calculated Y value of R was odd.
	ErrSigRYIsOdd

	// ErrSigRNotOnCurve indicates that the calculated or given point R for some
	// signature was not on the curve.
	ErrSigRNotOnCurve

	// ErrUnequalRValues indicates that the calculated point R for some
	// signature was not the same as the given R value for the signature.
	ErrUnequalRValues

	// ErrSigTooShort is returned when a signature that should be a Schnorr
	// signature is too short.
	ErrSigTooShort

	// ErrSigTooLong is returned when a signature that should be a Schnorr
	// signature is too long.
	ErrSigTooLong

	// ErrSigRTooBig is returned when a signature has r with a value that is
	// greater than or equal to the prime of the field underlying the group.
	ErrSigRTooBig

	// ErrSigSTooBig is returned when a signature has s with a value that is
	// greater than or equal to the group order.
	ErrSigSTooBig

	// numErrorCodes is the maximum error code number used in tests.  This entry
	// MUST be the last entry in the enum.
	numErrorCodes
)

// Map of ErrorCode values back to their constant names for pretty printing.
var errorCodeStrings = map[ErrorCode]string{
	ErrInvalidHashLen:   "ErrInvalidHashLen",
	ErrPrivateKeyIsZero: "ErrPrivateKeyIsZero",
	ErrSchnorrHashValue: "ErrSchnorrHashValue",
	ErrPubKeyNotOnCurve: "ErrPubKeyNotOnCurve",
	ErrSigRYIsOdd:       "ErrSigRYIsOdd",
	ErrSigRNotOnCurve:   "ErrSigRNotOnCurve",
	ErrUnequalRValues:   "ErrUnequalRValues",
	ErrSigTooShort:      "ErrSigTooShort",
	ErrSigTooLong:       "ErrSigTooLong",
	ErrSigRTooBig:       "ErrSigRTooBig",
	ErrSigSTooBig:       "ErrSigSTooBig",
}

// String returns the ErrorCode as a human-readable name.
func (e ErrorCode) String() string {
	if s := errorCodeStrings[e]; s != "" {
		return s
	}
	return fmt.Sprintf("Unknown ErrorCode (%d)", int(e))
}

// Error implements the error interface.
func (e ErrorCode) Error() string {
	return e.String()
}

// Is implements the interface to work with the standard library's errors.Is.
//
// It returns true in the following cases:
// - The target is an Error and the error codes match
// - The target is an ErrorCode and the error codes match
func (e ErrorCode) Is(target error) bool {
	switch target := target.(type) {
	case Error:
		return e == target.ErrorCode

	case ErrorCode:
		return e == target
	}

	return false
}

// Error identifies a signature-related error.  It has full support for
// errors.Is and errors.As, so the caller can ascertain the specific reason for
// the error by checking the underlying error code.
type Error struct {
	ErrorCode   ErrorCode // Describes the kind of error
	Description string    // Human readable description of the issue
}

// Error satisfies the error interface and prints human-readable errors.
func (e Error) Error() string {
	return e.Description
}

// Is implements the interface to work with the standard library's errors.Is.
//
// It returns true in the following cases:
// - The target is an Error and the error codes match
// - The target is an ErrorCode and the error codes match
func (e Error) Is(target error) bool {
	switch target := target.(type) {
	case Error:
		return e.ErrorCode == target.ErrorCode

	case ErrorCode:
		return target == e.ErrorCode
	}

	return false
}

// Unwrap returns the underlying wrapped error code.
func (e Error) Unwrap() error {
	return e.ErrorCode
}

// signatureError creates an Error given a set of arguments.
func signatureError(c ErrorCode, desc string) Error {
	return Error{ErrorCode: c, Description: desc}
}
