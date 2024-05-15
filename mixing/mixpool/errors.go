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
// RuleErrors can be treated as a bannable offense.
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

// Errors wrapped by RuleError.
var (
	// ErrChangeDust is returned by AcceptMessage if a pair request's
	// change amount is dust.
	ErrChangeDust = errors.New("change output is dust")

	// ErrInvalidMessageCount is returned by AcceptMessage if a
	// pair request contains an invalid message count.
	ErrInvalidMessageCount = errors.New("message count must be positive")

	// ErrInvalidScript is returned by AcceptMessage if a pair request
	// contains an invalid script.
	ErrInvalidScript = errors.New("invalid script")

	// ErrInvalidSessionID is returned by AcceptMessage if the message
	// contains an invalid session id.
	ErrInvalidSessionID = errors.New("invalid session ID")

	// ErrInvalidSignature is returned by AcceptMessage if the message is
	// not properly signed for the claimed identity.
	ErrInvalidSignature = errors.New("invalid message signature")

	// ErrInvalidTotalMixAmount is returned by AcceptMessage if a pair
	// request contains the product of the message count and mix amount
	// that exceeds the total input value.
	ErrInvalidTotalMixAmount = errors.New("invalid total mix amount")

	// ErrInvalidUTXOProof is returned by AcceptMessage if a pair request
	// fails to prove ownership of each utxo.
	ErrInvalidUTXOProof = errors.New("invalid UTXO ownership proof")

	// ErrMissingUTXOs is returned by AcceptMessage if a pair request
	// message does not reference any previous UTXOs.
	ErrMissingUTXOs = errors.New("pair request contains no UTXOs")
)

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
