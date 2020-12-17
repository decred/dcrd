// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

// ErrorKind identifies a kind of error.  It has full support for errors.Is and
// errors.As, so the caller can directly check against an error kind when
// determining the reason for an error.  These error codes are NOT used for
// JSON-RPC response errors.
type ErrorKind string

// These constants are used to identify a specific RuleError.
const (
	// ErrDuplicateMethod indicates a command with the specified method
	// already exists.
	ErrDuplicateMethod = ErrorKind("ErrDuplicateMethod")

	// ErrInvalidUsageFlags indicates one or more unrecognized flag bits
	// were specified.
	ErrInvalidUsageFlags = ErrorKind("ErrInvalidUsageFlags")

	// ErrInvalidType indicates a type was passed that is not the required
	// type.
	ErrInvalidType = ErrorKind("ErrInvalidType")

	// ErrEmbeddedType indicates the provided command struct contains an
	// embedded type which is not not supported.
	ErrEmbeddedType = ErrorKind("ErrEmbeddedType")

	// ErrUnexportedField indicates the provided command struct contains an
	// unexported field which is not supported.
	ErrUnexportedField = ErrorKind("ErrUnexportedField")

	// ErrUnsupportedFieldType indicates the type of a field in the provided
	// command struct is not one of the supported types.
	ErrUnsupportedFieldType = ErrorKind("ErrUnsupportedFieldType")

	// ErrNonOptionalField indicates a non-optional field was specified
	// after an optional field.
	ErrNonOptionalField = ErrorKind("ErrNonOptionalField")

	// ErrNonOptionalDefault indicates a 'jsonrpcdefault' struct tag was
	// specified for a non-optional field.
	ErrNonOptionalDefault = ErrorKind("ErrNonOptionalDefault")

	// ErrMismatchedDefault indicates a 'jsonrpcdefault' struct tag contains
	// a value that doesn't match the type of the field.
	ErrMismatchedDefault = ErrorKind("ErrMismatchedDefault")

	// ErrUnregisteredMethod indicates a method was specified that has not
	// been registered.
	ErrUnregisteredMethod = ErrorKind("ErrUnregisteredMethod")

	// ErrMissingDescription indicates a description required to generate
	// help is missing.
	ErrMissingDescription = ErrorKind("ErrMissingDescription")

	// ErrNumParams indicates the number of params supplied do not
	// match the requirements of the associated command.
	ErrNumParams = ErrorKind("ErrNumParams")
)

// Error satisfies the error interface and prints human-readable errors.
func (e ErrorKind) Error() string {
	return string(e)
}

// Error identifies an error related to decred's JSON-RPC APIs. It has
// full support for errors.Is and errors.As, so the caller can ascertain the
// specific reason for the error by checking the underlying error.
type Error struct {
	Err         error
	Description string
}

// Error satisfies the error interface and prints human-readable errors.
func (e Error) Error() string {
	return e.Description
}

// Unwrap returns the underlying wrapped error.
func (e Error) Unwrap() error {
	return e.Err
}

// makeError creates an Error given a set of arguments.
func makeError(kind ErrorKind, desc string) Error {
	return Error{Err: kind, Description: desc}
}
