// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

type Err string

func (e Err) Error() string { return string(e) }

type Error struct {
	Description string
	Err         error
}

func (e Error) Error() string { return e.Description }
func (e Error) Unwrap() error { return e.Err }

var (
	// ErrDialNil is used to indicate that Dial cannot be nil in
	// the configuration.
	ErrDialNil = Err("ErrDialNil")

	// ErrBothDialsFilled is used to indicate that Dial and DialAddr
	// cannot both be specified in the configuration.
	ErrBothDialsFilled = Err("ErrBothDialsFilled")

	// ErrTorInvalidAddressResponse indicates an invalid address was
	// returned by the Tor DNS resolver.
	ErrTorInvalidAddressResponse = Err("ErrTorInvalidAddressResponse")

	// ErrTorInvalidProxyResponse indicates the Tor proxy returned a
	// response in an unexpected format.
	ErrTorInvalidProxyResponse = Err("ErrTorInvalidProxyResponse")

	// ErrTorUnrecognizedAuthMethod indicates the authentication method
	// provided is not recognized.
	ErrTorUnrecognizedAuthMethod = Err("ErrTorUnrecognizedAuthMethod")

	// ErrTorGeneralError indicates a general tor error.
	ErrTorGeneralError = Err("ErrTorGeneralError")

	// ErrTorNotAllowed indicates tor connections are not allowed.
	ErrTorNotAllowed = Err("ErrTorNotAllowed")

	// ErrTorNetUnreachable indicates the tor network is unreachable.
	ErrTorNetUnreachable = Err("ErrTorNetUnreachable")

	// ErrTorHostUnreachable indicates the tor host is unreachable.
	ErrTorHostUnreachable = Err("ErrTorHostUnreachable")

	// ErrTorConnectionRefused indicates the tor connection was refused.
	ErrTorConnectionRefused = Err("ErrTorConnectionRefused")

	// ErrTorTTLExpired indicates the tor request Time-To-Live (TTL) expired.
	ErrTorTTLExpired = Err("ErrTorTTLExpired")

	// ErrTorCmdNotSupported indicates the tor command is not supported.
	ErrTorCmdNotSupported = Err("ErrTorCmdNotSupported")

	// ErrTorAddrNotSupported indicates the tor address type is not supported.
	ErrTorAddrNotSupported = Err("ErrTorAddrNotSupported")
)
