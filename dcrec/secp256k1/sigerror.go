// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"fmt"
)

// SignatureErrorCode identifies a kind of signature error.  It has full support
// for errors.Is and errors.As, so the caller can directly check against an
// error code when determining the reason for an error.
type SignatureErrorCode int

// These constants are used to identify a specific SignatureError.
const (
	// ErrSigTooShort is returned when a signature that should be a DER
	// signature is too short.
	ErrSigTooShort SignatureErrorCode = iota

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

// Map of SignatureErrorCode values back to their constant names for pretty
// printing.
var errorCodeStrings = map[SignatureErrorCode]string{
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

// String returns the SignatureErrorCode as a human-readable name.
func (e SignatureErrorCode) String() string {
	if s := errorCodeStrings[e]; s != "" {
		return s
	}
	return fmt.Sprintf("Unknown SignatureErrorCode (%d)", int(e))
}

// Error implements the error interface.
func (e SignatureErrorCode) Error() string {
	return e.String()
}

// Is implements the interface to work with the standard library's errors.Is.
//
// It returns true in the following cases:
// - The target is a SignatureError and the error codes match
// - The target is a SignatureErrorCode and the error codes match
func (e SignatureErrorCode) Is(target error) bool {
	switch target := target.(type) {
	case SignatureError:
		return e == target.ErrorCode

	case SignatureErrorCode:
		return e == target
	}

	return false
}

// SignatureError identifies a signature-related error.  It has full support for
// errors.Is and errors.As, so the caller can ascertain the specific reason for
// the error by checking the underlying error code.
type SignatureError struct {
	ErrorCode   SignatureErrorCode
	Description string
}

// Error satisfies the error interface and prints human-readable errors.
func (e SignatureError) Error() string {
	return e.Description
}

// Is implements the interface to work with the standard library's errors.Is.
//
// It returns true in the following cases:
// - The target is a SignatureError and the error codes match
// - The target is a SignatureErrorCode and it the error codes match
func (e SignatureError) Is(target error) bool {
	switch target := target.(type) {
	case SignatureError:
		return e.ErrorCode == target.ErrorCode

	case SignatureErrorCode:
		return target == e.ErrorCode
	}

	return false
}

// Unwrap returns the underlying wrapped error code.
func (e SignatureError) Unwrap() error {
	return e.ErrorCode
}

// signatureError creates a SignatureError given a set of arguments.
func signatureError(c SignatureErrorCode, desc string) SignatureError {
	return SignatureError{ErrorCode: c, Description: desc}
}
