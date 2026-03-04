// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package addrmgr

// ErrorKind identifies a kind of error.  It has full support for errors.Is
// and errors.As, so the caller can directly check against an error kind
// when determining the reason for an error.
type ErrorKind string

// These constants are used to identify a specific RuleError.
const (
	// ErrAddressNotFound indicates that an operation in the address manager
	// failed due to an address lookup failure.
	ErrAddressNotFound = ErrorKind("ErrAddressNotFound")

	// ErrUnknownAddressType indicates that the network address type could not
	// be determined from a network address' bytes.
	ErrUnknownAddressType = ErrorKind("ErrUnknownAddressType")

	// ErrMismatchedAddressType indicates that a network address was expected to
	// be a certain type, but the derived type does not match.
	ErrMismatchedAddressType = ErrorKind("ErrMismatchedAddressType")

	// ErrTorInvalidAddressResponse indicates an invalid address was
	// returned by the Tor DNS resolver.
	ErrTorInvalidAddressResponse = ErrorKind("ErrTorInvalidAddressResponse")

	// ErrTorInvalidProxyResponse indicates the Tor proxy returned a
	// response in an unexpected format.
	ErrTorInvalidProxyResponse = ErrorKind("ErrTorInvalidProxyResponse")

	// ErrTorUnrecognizedAuthMethod indicates the authentication method
	// provided is not recognized.
	ErrTorUnrecognizedAuthMethod = ErrorKind("ErrTorUnrecognizedAuthMethod")

	// ErrTorGeneralError indicates a general tor error.
	ErrTorGeneralError = ErrorKind("ErrTorGeneralError")

	// ErrTorNotAllowed indicates tor connections are not allowed.
	ErrTorNotAllowed = ErrorKind("ErrTorNotAllowed")

	// ErrTorNetUnreachable indicates the tor network is unreachable.
	ErrTorNetUnreachable = ErrorKind("ErrTorNetUnreachable")

	// ErrTorHostUnreachable indicates the tor host is unreachable.
	ErrTorHostUnreachable = ErrorKind("ErrTorHostUnreachable")

	// ErrTorConnectionRefused indicates the tor connection was refused.
	ErrTorConnectionRefused = ErrorKind("ErrTorConnectionRefused")

	// ErrTorTTLExpired indicates the tor request Time-To-Live (TTL) expired.
	ErrTorTTLExpired = ErrorKind("ErrTorTTLExpired")

	// ErrTorCmdNotSupported indicates the tor command is not supported.
	ErrTorCmdNotSupported = ErrorKind("ErrTorCmdNotSupported")

	// ErrTorAddrNotSupported indicates the tor address type is not supported.
	ErrTorAddrNotSupported = ErrorKind("ErrTorAddrNotSupported")
)

// Error satisfies the error interface and prints human-readable errors.
func (e ErrorKind) Error() string {
	return string(e)
}

// Error identifies an address manager error.  It has full support for
// errors.Is and errors.As, so the caller can ascertain the specific reason
// for the error by checking the underlying error.
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
