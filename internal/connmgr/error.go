// Copyright (c) 2020-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

// ErrorKind identifies a kind of error.  It has full support for errors.Is and
// errors.As, so the caller can directly check against an error kind when
// determining the reason for an error.
type ErrorKind string

const (
	// ErrDialNil is used to indicate that Dial cannot be nil in
	// the configuration.
	ErrDialNil = ErrorKind("ErrDialNil")

	// ErrAlreadyPending indicates an attempt to connect to an address that
	// already has a pending connection attempt.
	ErrAlreadyPending = ErrorKind("ErrAlreadyPending")

	// ErrAlreadyConnected indicates an attempt to connect to an address that
	// already has an established connection.
	ErrAlreadyConnected = ErrorKind("ErrAlreadyConnected")

	// ErrMaxNormalConns indicates a connection attempt (inbound or outbound)
	// would exceed the maximum allowed number of normal connections.
	ErrMaxNormalConns = ErrorKind("ErrMaxNormalConns")

	// ErrMaxConnsPerHost indicates a connection attempt (inbound or outbound)
	// would exceed the maximum allowed number of connections per host.
	ErrMaxConnsPerHost = ErrorKind("ErrMaxConnsPerHost")

	// ErrMaxPersistent indicates an attempt to add more than the maximum
	// allowed number of persistent connections.
	ErrMaxPersistent = ErrorKind("ErrMaxPersistent")

	// ErrMaxPersistentPerHost indicates an attempt to add more than the maximum
	// allowed number of persistent connections with the same host address.
	ErrMaxPersistentPerHost = ErrorKind("ErrMaxPersistentPerHost")

	// ErrDuplicatePersistent indicates an attempt to add a persistent
	// connection to an address that already exists.
	ErrDuplicatePersistent = ErrorKind("ErrDuplicatePersistent")

	// ErrNotFound indicates a specified connection ID or address is unknown to
	// the connection manager.
	ErrNotFound = ErrorKind("ErrNotFound")

	// ErrUnsupportedAddr indicates an address is either an unsupported type or
	// an unrecognized type due to being malformed.
	ErrUnsupportedAddr = ErrorKind("ErrUnsupportedAddr")

	// ErrShutdown indicates the connection manager is either in the process of
	// shutting down or has already been shutdown.
	ErrShutdown = ErrorKind("ErrShutdown")

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

// Error identifies an error related to the connection manager error.  It has
// full support for errors.Is and errors.As, so the caller can ascertain the
// specific reason for the error by checking the underlying error.
type Error struct {
	Description string
	Err         error
}

// Error satisfies the error interface and prints human-readable errors.
func (e Error) Error() string {
	return e.Description
}

// Unwrap returns the underlying wrapped error.
func (e Error) Unwrap() error {
	return e.Err
}

// MakeError creates an Error given a set of arguments.
func MakeError(kind ErrorKind, desc string) Error {
	return Error{Err: kind, Description: desc}
}
