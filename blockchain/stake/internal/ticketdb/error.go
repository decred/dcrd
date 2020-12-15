// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ticketdb

// ErrorKind identifies a kind of error.  It has full support for errors.Is
// and errors.As, so the caller can directly check against an error kind
// when determining the reason for an error.
type ErrorKind string

// These constants are used to identify a specific DBError.
const (
	// ErrUndoDataShortRead indicates that the given undo serialized data
	// was took small.
	ErrUndoDataShortRead = ErrorKind("ErrUndoDataShortRead")

	// ErrUndoDataNoEntries indicates that the data for undoing ticket data
	// in a serialized entry was corrupt.
	ErrUndoDataCorrupt = ErrorKind("ErrUndoDataCorrupt")

	// ErrTicketHashesShortRead indicates that the given ticket hashes
	// serialized data was took small.
	ErrTicketHashesShortRead = ErrorKind("ErrTicketHashesShortRead")

	// ErrTicketHashesCorrupt indicates that the data for ticket hashes
	// in a serialized entry was corrupt.
	ErrTicketHashesCorrupt = ErrorKind("ErrTicketHashesCorrupt")

	// ErrUninitializedBucket indicates that a database bucket was not
	// initialized and therefore could not be written to or read from.
	ErrUninitializedBucket = ErrorKind("ErrUninitializedBucket")

	// ErrMissingKey indicates that a key was not found in a bucket.
	ErrMissingKey = ErrorKind("ErrMissingKey")

	// ErrChainStateShortRead indicates that the given chain state data
	// was too small.
	ErrChainStateShortRead = ErrorKind("ErrChainStateShortRead")

	// ErrDatabaseInfoShortRead indicates that the given database information
	// was too small.
	ErrDatabaseInfoShortRead = ErrorKind("ErrDatabaseInfoShortRead")

	// ErrLoadAllTickets indicates that there was an error loading the tickets
	// from the database, presumably at startup.
	ErrLoadAllTickets = ErrorKind("ErrLoadAllTickets")
)

// Error satisfies the error interface and prints human-readable errors.
func (e ErrorKind) Error() string {
	return string(e)
}

// DBError identifies an error related to the stake database for tickets. It
// has full support for errors.Is and errors.As, so the caller can ascertain
// the specific reason for the error by checking the underlying error.
type DBError struct {
	Description string
	Err         error
}

// Error satisfies the error interface and prints human-readable errors.
func (e DBError) Error() string {
	return e.Description
}

// Unwrap returns the underlying wrapped error.
func (e DBError) Unwrap() error {
	return e.Err
}

// ticketDBError creates an DBError given a set of arguments.
func ticketDBError(kind ErrorKind, desc string) DBError {
	return DBError{Err: kind, Description: desc}
}
