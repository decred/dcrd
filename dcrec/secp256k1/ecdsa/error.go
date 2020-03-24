// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ecdsa

import (
	"fmt"
)

// ErrorCode identifies a kind of signature error.  It has full support
// for errors.Is and errors.As, so the caller can directly check against an
// error code when determining the reason for an error.
type ErrorCode int

// These constants are used to identify a specific Error.
const (
	// ErrSigTooShort is returned when a signature that should be a DER
	// signature is too short.
	ErrSigTooShort ErrorCode = iota

	// ErrSigTooLong is returned when a signature that should be a DER signature
	// is too long.
	ErrSigTooLong

	// ErrSigInvalidSeqID is returned when a signature that should be a DER
	// signature does not have the expected ASN.1 sequence ID.
	ErrSigInvalidSeqID

	// ErrSigInvalidDataLen is returned when a signature that should be a DER
	// signature does not specify the correct number of remaining bytes for the
	// R and S portions.
	ErrSigInvalidDataLen

	// ErrSigMissingSTypeID is returned when a signature that should be a DER
	// signature does not provide the ASN.1 type ID for S.
	ErrSigMissingSTypeID

	// ErrSigMissingSLen is returned when a signature that should be a DER
	// signature does not provide the length of S.
	ErrSigMissingSLen

	// ErrSigInvalidSLen is returned when a signature that should be a DER
	// signature does not specify the correct number of bytes for the S portion.
	ErrSigInvalidSLen

	// ErrSigInvalidRIntID is returned when a signature that should be a DER
	// signature does not have the expected ASN.1 integer ID for R.
	ErrSigInvalidRIntID

	// ErrSigZeroRLen is returned when a signature that should be a DER
	// signature has an R length of zero.
	ErrSigZeroRLen

	// ErrSigNegativeR is returned when a signature that should be a DER
	// signature has a negative value for R.
	ErrSigNegativeR

	// ErrSigTooMuchRPadding is returned when a signature that should be a DER
	// signature has too much padding for R.
	ErrSigTooMuchRPadding

	// ErrSigRIsZero is returned when a signature has R set to the value zero.
	ErrSigRIsZero

	// ErrSigRTooBig is returned when a signature has R with a value that is
	// greater than or equal to the group order.
	ErrSigRTooBig

	// ErrSigInvalidSIntID is returned when a signature that should be a DER
	// signature does not have the expected ASN.1 integer ID for S.
	ErrSigInvalidSIntID

	// ErrSigZeroSLen is returned when a signature that should be a DER
	// signature has an S length of zero.
	ErrSigZeroSLen

	// ErrSigNegativeS is returned when a signature that should be a DER
	// signature has a negative value for S.
	ErrSigNegativeS

	// ErrSigTooMuchSPadding is returned when a signature that should be a DER
	// signature has too much padding for S.
	ErrSigTooMuchSPadding

	// ErrSigSIsZero is returned when a signature has S set to the value zero.
	ErrSigSIsZero

	// ErrSigSTooBig is returned when a signature has S with a value that is
	// greater than or equal to the group order.
	ErrSigSTooBig

	// numSigErrorCodes is the maximum error code number used in tests.  This
	// entry MUST be the last entry in the enum.
	numSigErrorCodes
)

// Map of ErrorCode values back to their constant names for pretty printing.
var errorCodeStrings = map[ErrorCode]string{
	ErrSigTooShort:        "ErrSigTooShort",
	ErrSigTooLong:         "ErrSigTooLong",
	ErrSigInvalidSeqID:    "ErrSigInvalidSeqID",
	ErrSigInvalidDataLen:  "ErrSigInvalidDataLen",
	ErrSigMissingSTypeID:  "ErrSigMissingSTypeID",
	ErrSigMissingSLen:     "ErrSigMissingSLen",
	ErrSigInvalidSLen:     "ErrSigInvalidSLen",
	ErrSigInvalidRIntID:   "ErrSigInvalidRIntID",
	ErrSigZeroRLen:        "ErrSigZeroRLen",
	ErrSigNegativeR:       "ErrSigNegativeR",
	ErrSigTooMuchRPadding: "ErrSigTooMuchRPadding",
	ErrSigRIsZero:         "ErrSigRIsZero",
	ErrSigRTooBig:         "ErrSigRTooBig",
	ErrSigInvalidSIntID:   "ErrSigInvalidSIntID",
	ErrSigZeroSLen:        "ErrSigZeroSLen",
	ErrSigNegativeS:       "ErrSigNegativeS",
	ErrSigTooMuchSPadding: "ErrSigTooMuchSPadding",
	ErrSigSIsZero:         "ErrSigSIsZero",
	ErrSigSTooBig:         "ErrSigSTooBig",
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
// - The target is a Error and the error codes match
// - The target is a ErrorCode and the error codes match
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
	ErrorCode   ErrorCode
	Description string
}

// Error satisfies the error interface and prints human-readable errors.
func (e Error) Error() string {
	return e.Description
}

// Is implements the interface to work with the standard library's errors.Is.
//
// It returns true in the following cases:
// - The target is a Error and the error codes match
// - The target is a ErrorCode and it the error codes match
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

// signatureError creates a Error given a set of arguments.
func signatureError(c ErrorCode, desc string) Error {
	return Error{ErrorCode: c, Description: desc}
}
