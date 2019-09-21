// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
)

// MiningErrorCode identifies a kind of error.
type MiningErrorCode int

// These constants are used to identify a specific RuleError.
const (
	// ErrNotEnoughVoters indicates that there were not enough voters to
	// build a block on top of HEAD.
	ErrNotEnoughVoters MiningErrorCode = iota

	// ErrFailedToGetGeneration specifies that the current generation for
	// a block could not be obtained from blockchain.
	ErrFailedToGetGeneration

	// ErrGetStakeDifficulty indicates that the current top block of the
	// blockchain could not be obtained.
	ErrGetTopBlock

	// ErrGettingDifficulty indicates that there was an error getting the
	// PoW difficulty.
	ErrGettingDifficulty

	// ErrTransactionAppend indicates there was a problem adding a msgtx
	// to a msgblock.
	ErrTransactionAppend

	// ErrCheckConnectBlock indicates that a newly created block template
	// failed blockchain.CheckConnectBlock.
	ErrCheckConnectBlock

	// ErrFraudProofIndex indicates that there was an error finding the index
	// for a fraud proof.
	ErrFraudProofIndex

	// ErrFetchTxStore indicates a transaction store failed to fetch.
	ErrFetchTxStore

	// ErrCalcCommitmentRoot indicates that creating the header commitments and
	// calculating the associated commitment root for a newly created block
	// template failed.
	ErrCalcCommitmentRoot
)

// Map of MiningErrorCode values back to their constant names for pretty printing.
var miningErrorCodeStrings = map[MiningErrorCode]string{
	ErrNotEnoughVoters:       "ErrNotEnoughVoters",
	ErrFailedToGetGeneration: "ErrFailedToGetGeneration",
	ErrGetTopBlock:           "ErrGetTopBlock",
	ErrGettingDifficulty:     "ErrGettingDifficulty",
	ErrTransactionAppend:     "ErrTransactionAppend",
	ErrCheckConnectBlock:     "ErrCheckConnectBlock",
	ErrFraudProofIndex:       "ErrFraudProofIndex",
	ErrFetchTxStore:          "ErrFetchTxStore",
	ErrCalcCommitmentRoot:    "ErrCalcCommitmentRoot",
}

// String returns the MiningErrorCode as a human-readable name.
func (e MiningErrorCode) String() string {
	if s := miningErrorCodeStrings[e]; s != "" {
		return s
	}
	return fmt.Sprintf("Unknown MiningErrorCode (%d)", int(e))
}

// MiningRuleError identifies a rule violation.  It is used to indicate that
// processing of a block or transaction failed due to one of the many validation
// rules.  The caller can use type assertions to determine if a failure was
// specifically due to a rule violation and access the MiningErrorCode field to
// ascertain the specific reason for the rule violation.
type MiningRuleError struct {
	ErrorCode   MiningErrorCode // Describes the kind of error
	Description string          // Human readable description of the issue
}

// Error satisfies the error interface and prints human-readable errors.
func (e MiningRuleError) Error() string {
	return e.Description
}

// GetCode satisfies the error interface and prints human-readable errors.
func (e MiningRuleError) GetCode() MiningErrorCode {
	return e.ErrorCode
}

// miningRuleError creates an RuleError given a set of arguments.
func miningRuleError(c MiningErrorCode, desc string) MiningRuleError {
	return MiningRuleError{ErrorCode: c, Description: desc}
}
