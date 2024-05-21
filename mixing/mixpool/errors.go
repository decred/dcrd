// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixpool

import (
	"errors"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// RuleError represents a mixpool rule violation.
//
// Some RuleErrors should be treated as an automatic bannable offense.  Use
// IsBannable to test for this condition.
type RuleError struct {
	Err error
}

func ruleError(err error) *RuleError {
	return &RuleError{Err: err}
}

func (e *RuleError) Error() string {
	return e.Err.Error()
}

func (e *RuleError) Unwrap() error {
	return e.Err
}

type bannableError struct {
	s string
}

func (e *bannableError) Error() string {
	return e.s
}

func newBannableError(s string) error {
	return &bannableError{
		s: s,
	}
}

// Bannable errors wrapped by RuleError.
var (
	// ErrChangeDust is returned by AcceptMessage if a pair request's
	// change amount is dust.
	ErrChangeDust = newBannableError("change output is dust")

	// ErrLowInput is returned by AcceptMessage when not enough input value
	// is provided by a pair request to cover the mixed output, any change
	// output, and the minimum required fee.
	ErrLowInput = newBannableError("not enough input value, or too low fee")

	// ErrInvalidMessageCount is returned by AcceptMessage if a
	// pair request contains an invalid message count.
	ErrInvalidMessageCount = newBannableError("message count must be positive")

	// ErrInvalidScript is returned by AcceptMessage if a pair request
	// contains an invalid script.
	ErrInvalidScript = newBannableError("invalid script")

	// ErrInvalidSessionID is returned by AcceptMessage if the message
	// contains an invalid session id.
	ErrInvalidSessionID = newBannableError("invalid session ID")

	// ErrInvalidSignature is returned by AcceptMessage if the message is
	// not properly signed for the claimed identity.
	ErrInvalidSignature = newBannableError("invalid message signature")

	// ErrInvalidTotalMixAmount is returned by AcceptMessage if a pair
	// request contains the product of the message count and mix amount
	// that exceeds the total input value.
	ErrInvalidTotalMixAmount = newBannableError("invalid total mix amount")

	// ErrInvalidUTXOProof is returned by AcceptMessage if a pair request
	// fails to prove ownership of each utxo.
	ErrInvalidUTXOProof = newBannableError("invalid UTXO ownership proof")

	// ErrMissingUTXOs is returned by AcceptMessage if a pair request
	// message does not reference any previous UTXOs.
	ErrMissingUTXOs = newBannableError("pair request contains no UTXOs")

	// ErrPeerPositionOutOfBounds is returned by AcceptMessage if the
	// position of a peer's own PR is outside of the possible bounds of
	// the previously seen messages.
	ErrPeerPositionOutOfBounds = newBannableError("peer position cannot be a valid seen PRs index")
)

// IsBannable returns whether the error condition is such that the peer who
// sent the message should be immediately banned for malicious or buggy
// behavior.
func IsBannable(err error) bool {
	var be *bannableError
	return errors.As(err, &be)
}

var (
	// ErrSecretsRevealed is returned by Receive if any peer has
	// unexpectedly revealed their secrets during a run stage
	// (Received.RSs field is nil).  This requires all other peers to quit
	// the run, reveal their secrets, and perform blame assignment.
	ErrSecretsRevealed = errors.New("secrets revealed by peer")
)

// MissingOwnPRError represents the error condition where a key exchange
// message cannot be accepted to the mixpool due to it not referencing the
// owner's own pair request.  The KE is recorded as an orphan and may be
// processed later.  The error contains all unknown PR hashes so they can be
// fetched from another instance.
type MissingOwnPRError struct {
	MissingPR chainhash.Hash
}

func (e *MissingOwnPRError) Error() string {
	return "KE identity's own PR is missing from mixpool"
}
