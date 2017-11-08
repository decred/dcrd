// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
)

const (
	// DefaultBlockPrioritySize is the default size in bytes for high-
	// priority / low-fee transactions.  It is used to help determine which
	// are allowed into the mempool and consequently affects their relay and
	// inclusion when generating block templates.
	DefaultBlockPrioritySize = 20000

	// DefaultMinRelayTxFee is the minimum fee in atoms that is required for
	// a transaction to be treated as free for relay and mining purposes.
	// It is also used to help determine if a transaction is considered dust
	// and as a base for calculating minimum required fees for larger
	// transactions.  This value is in Atoms/1000 bytes.
	DefaultMinRelayTxFee = dcrutil.Amount(1e5)

	// BaseStandardVerifyFlags defines the script flags that should be used
	// when executing transaction scripts to enforce additional checks which
	// are required for the script to be considered standard regardless of
	// the state of any agenda votes.  The full set of standard verification
	// flags must include these flags as well as any additional flags that
	// are conditionally enabled depending on the result of agenda votes.
	BaseStandardVerifyFlags = txscript.ScriptBip16 |
		txscript.ScriptVerifyDERSignatures |
		txscript.ScriptVerifyStrictEncoding |
		txscript.ScriptVerifyMinimalData |
		txscript.ScriptDiscourageUpgradableNops |
		txscript.ScriptVerifyCleanStack |
		txscript.ScriptVerifyCheckLockTimeVerify |
		txscript.ScriptVerifyCheckSequenceVerify |
		txscript.ScriptVerifyLowS
)
