// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

// ErrorKind identifies a kind of error.  It has full support for errors.Is and
// errors.As, so the caller can directly check against an error kind when
// determining the reason for an error.
type ErrorKind string

// These constants are used to identify a specific IndexerError.
const (
	// ErrUnsupportedAddressType indicates an unsupported address type.
	ErrUnsupportedAddressType = ErrorKind("ErrUnsupportedAddressType")

	// ErrInvalidSpendConsumerType indicates an invalid spend consumer type.
	ErrInvalidSpendConsumerType = ErrorKind("ErrInvalidSpendConsumerType")

	// ErrConnectBlock indicates an error indexing a connected block.
	ErrConnectBlock = ErrorKind("ErrConnectBlock")

	// ErrDisconnectBlock indicates an error indexing a disconnected block.
	ErrDisconnectBlock = ErrorKind("ErrDisconnectBlock")

	// ErrRemoveSpendDependency indicates a spend dependency removal error.
	ErrRemoveSpendDependency = ErrorKind("ErrRemoveSpendDependency")

	// ErrInvalidNotificationType indicates an invalid indexer notification type.
	ErrInvalidNotificationType = ErrorKind("ErrInvalidNotificationType")

	// ErrInterruptRequested indicates an operation was cancelled due to
	// a user-requested interrupt.
	ErrInterruptRequested = ErrorKind("ErrInterruptRequested")

	// ErrFetchSubscription indicates an error fetching an index subscription.
	ErrFetchSubscription = ErrorKind("ErrFetchSubscription")

	// ErrFetchTip indicates an error fetching an index tip.
	ErrFetchTip = ErrorKind("ErrFetchTip")

	// ErrMissingNotification indicates a missing index notification.
	ErrMissingNotification = ErrorKind("ErrMissingNotification")

	// ErrBlockNotOnMainChain indicates the provided block is not on the
	// main chain.
	ErrBlockNotOnMainChain = ErrorKind("ErrBlockNotOnMainChain")
)

// Error satisfies the error interface and prints human-readable errors.
func (e ErrorKind) Error() string {
	return string(e)
}

// IndexerError identifies an indexer error. It has full support for
// errors.Is and errors.As, so the caller can ascertain the specific reason
// for the error by checking the underlying error.
type IndexerError struct {
	Description string
	Err         error
}

// Error satisfies the error interface and prints human-readable errors.
func (e IndexerError) Error() string {
	return e.Description
}

// Unwrap returns the underlying wrapped error.
func (e IndexerError) Unwrap() error {
	return e.Err
}

// indexerError creates a IndexerError given a set of arguments.
func indexerError(kind ErrorKind, desc string) IndexerError {
	return IndexerError{Err: kind, Description: desc}
}
