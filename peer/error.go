// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

// ErrorKind identifies a kind of error.  It has full support for errors.Is
// and errors.As, so the caller can directly check against an error kind
// when determining the reason for an error.
type ErrorKind string

// These constants are used to identify a specific [Error].
const (
	// ErrNotVersionMessage indicates the first message received from a remote
	// peer is not the required version message.
	ErrNotVersionMessage = ErrorKind("ErrNotVersionMessage")

	// ErrSelfConnection indicates a peer attempted to connect to itself.
	ErrSelfConnection = ErrorKind("ErrSelfConnection")

	// ErrProtocolVerTooOld indicates a protocol version is older than the
	// minimum required version.
	ErrProtocolVerTooOld = ErrorKind("ErrProtocolVerTooOld")

	// ErrNotVerAckMessage indicates the second message received from a remote
	// peer is not the required verack message.
	ErrNotVerAckMessage = ErrorKind("ErrNotVerAckMessage")

	// ErrHandshakeTimeout indicates the initial handshake timed out before
	// completing.
	ErrHandshakeTimeout = ErrorKind("ErrHandshakeTimeout")
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

// makeError creates an [Error] given a set of arguments.
func makeError(kind ErrorKind, desc string) Error {
	return Error{Err: kind, Description: desc}
}
