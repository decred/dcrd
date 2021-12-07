// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"github.com/decred/dcrd/blockchain/stake/v4"
)

const (
	// baseEntrySize is the base size of a utxo entry on a 64-bit platform,
	// excluding the contents of the script and ticket minimal outputs.  It is
	// equivalent to what unsafe.Sizeof(UtxoEntry{}) returns on a 64-bit
	// platform.
	baseEntrySize = 56
)

// utxoState defines the in-memory state of a utxo entry.
//
// The bit representation is:
//   bit  0    - transaction output has been spent
//   bit  1    - transaction output has been modified since it was loaded
//   bit  2    - transaction output is fresh
//   bits 3-7  - unused
type utxoState uint8

const (
	// utxoStateSpent indicates that a txout is spent.
	utxoStateSpent utxoState = 1 << iota

	// utxoStateModified indicates that a txout has been modified since it was
	// loaded.
	utxoStateModified

	// utxoStateFresh indicates that a txout is fresh, which means that it
	// exists in the utxo cache but does not exist in the underlying database.
	utxoStateFresh
)

// utxoFlags defines additional information for the containing transaction of a
// utxo entry.
//
// The bit representation is:
//   bit  0    - containing transaction is a coinbase
//   bit  1    - containing transaction has an expiry
//   bits 2-5  - transaction type
type utxoFlags uint8

const (
	// utxoFlagCoinBase indicates that a txout was contained in a coinbase tx.
	utxoFlagCoinBase utxoFlags = 1 << iota

	// utxoFlagHasExpiry indicates that a txout was contained in a tx that
	// included an expiry.
	utxoFlagHasExpiry
)

const (
	// utxoFlagTxTypeBitmask describes the bitmask that yields bits 2-5 from
	// utxoFlags.
	utxoFlagTxTypeBitmask = 0x3c

	// utxoFlagTxTypeShift is the number of bits to shift utxoFlags to the right
	// to yield the correct integer value after applying the bitmask with AND.
	utxoFlagTxTypeShift = 2
)

// encodeUtxoFlags returns utxoFlags representing the passed parameters.
func encodeUtxoFlags(coinbase bool, hasExpiry bool, txType stake.TxType) utxoFlags {
	packedFlags := utxoFlags(txType) << utxoFlagTxTypeShift
	if coinbase {
		packedFlags |= utxoFlagCoinBase
	}
	if hasExpiry {
		packedFlags |= utxoFlagHasExpiry
	}

	return packedFlags
}

// isTicketSubmissionOutput returns true if the output is a ticket submission.
func isTicketSubmissionOutput(txType stake.TxType, txOutIdx uint32) bool {
	return txType == stake.TxTypeSStx && txOutIdx == 0
}

// ticketMinimalOutputs stores the minimal outputs for ticket transactions and
// is used in ticket submission utxo entries.
//
// The minimal outputs for ticket transactions are stored since all outputs of a
// ticket need to be retrieved when validating vote transaction inputs or
// revoking a ticket.
type ticketMinimalOutputs struct {
	data []byte
}

// UtxoEntry houses details about an individual transaction output in a utxo
// view such as whether or not it was contained in a coinbase tx, the height of
// the block that contains the tx, whether or not it is spent, its public key
// script, and how much it pays.
//
// The struct is aligned for memory efficiency.
type UtxoEntry struct {
	amount   int64
	pkScript []byte

	// ticketMinOuts is the minimal outputs for the ticket transaction that the
	// output is contained in.  This is only stored in ticket submission outputs
	// and is nil for all other output types.
	//
	// Note that this is using a pointer rather than a slice in order to occupy
	// less space when it is nil.  It is nil in the vast majority of entries, so
	// this provides a significant overall reduction in memory usage.
	ticketMinOuts *ticketMinimalOutputs

	blockHeight   uint32
	blockIndex    uint32
	scriptVersion uint16

	// state contains info for the in-memory state of the output as defined by
	// utxoState.
	state utxoState

	// packedFlags contains additional info for the containing transaction of
	// the output as defined by utxoFlags.  This approach is used in order to
	// reduce memory usage since there will be a lot of these in memory.
	packedFlags utxoFlags
}

// size returns the number of bytes that the entry uses on a 64-bit platform.
func (entry *UtxoEntry) size() uint64 {
	size := baseEntrySize + len(entry.pkScript)
	if entry.ticketMinOuts != nil {
		size += len(entry.ticketMinOuts.data)
	}
	return uint64(size)
}

// isModified returns whether or not the output has been modified since it was
// loaded.
func (entry *UtxoEntry) isModified() bool {
	return entry.state&utxoStateModified == utxoStateModified
}

// isFresh returns whether or not the output is fresh.
func (entry *UtxoEntry) isFresh() bool {
	return entry.state&utxoStateFresh == utxoStateFresh
}

// IsCoinBase returns whether or not the output was contained in a coinbase
// transaction.
func (entry *UtxoEntry) IsCoinBase() bool {
	return entry.packedFlags&utxoFlagCoinBase == utxoFlagCoinBase
}

// IsSpent returns whether or not the output has been spent based upon the
// current state of the unspent transaction output view it was obtained from.
func (entry *UtxoEntry) IsSpent() bool {
	return entry.state&utxoStateSpent == utxoStateSpent
}

// HasExpiry returns whether or not the output was contained in a transaction
// that included an expiry.
func (entry *UtxoEntry) HasExpiry() bool {
	return entry.packedFlags&utxoFlagHasExpiry == utxoFlagHasExpiry
}

// BlockHeight returns the height of the block containing the output.
func (entry *UtxoEntry) BlockHeight() int64 {
	return int64(entry.blockHeight)
}

// BlockIndex returns the index of the transaction that the output is contained
// in.
func (entry *UtxoEntry) BlockIndex() uint32 {
	return entry.blockIndex
}

// TransactionType returns the type of the transaction that the output is
// contained in.
func (entry *UtxoEntry) TransactionType() stake.TxType {
	txType := (entry.packedFlags & utxoFlagTxTypeBitmask) >> utxoFlagTxTypeShift
	return stake.TxType(txType)
}

// Spend marks the output as spent.  Spending an output that is already spent
// has no effect.
func (entry *UtxoEntry) Spend() {
	// Nothing to do if the output is already spent.
	if entry.IsSpent() {
		return
	}

	// Mark the output as spent and modified.
	entry.state |= utxoStateSpent | utxoStateModified
}

// Amount returns the amount of the output.
func (entry *UtxoEntry) Amount() int64 {
	return entry.amount
}

// PkScript returns the public key script for the output.
func (entry *UtxoEntry) PkScript() []byte {
	return entry.pkScript
}

// ScriptVersion returns the public key script version for the output.
func (entry *UtxoEntry) ScriptVersion() uint16 {
	return entry.scriptVersion
}

// TicketMinimalOutputs returns the minimal outputs for the ticket transaction
// that the output is contained in.  Note that the ticket minimal outputs are
// only stored in ticket submission outputs and nil will be returned for all
// other output types.
func (entry *UtxoEntry) TicketMinimalOutputs() []*stake.MinimalOutput {
	if entry.ticketMinOuts == nil {
		return nil
	}

	minOuts, _ := deserializeToMinimalOutputs(entry.ticketMinOuts.data)
	return minOuts
}

// Clone returns a copy of the utxo entry.  It performs a deep copy for any
// fields that are mutable.  Specifically, the script and ticket minimal outputs
// are NOT deep copied since they are immutable.
func (entry *UtxoEntry) Clone() *UtxoEntry {
	if entry == nil {
		return nil
	}

	newEntry := &UtxoEntry{
		amount:        entry.amount,
		pkScript:      entry.pkScript,
		ticketMinOuts: entry.ticketMinOuts,
		blockHeight:   entry.blockHeight,
		blockIndex:    entry.blockIndex,
		scriptVersion: entry.scriptVersion,
		state:         entry.state,
		packedFlags:   entry.packedFlags,
	}

	return newEntry
}
