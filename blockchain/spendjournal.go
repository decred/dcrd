// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"github.com/decred/dcrd/blockchain/internal/dbnamespace"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
)

type SpendJournal struct {
}

func NewSpendJournal() *SpendJournal {
	return &SpendJournal{}
}

// deserializeSpendJournalEntry decodes the passed serialized byte slice into a
// slice of spent txouts according to the format described in detail above.
//
// Since the serialization format is not self describing, as noted in the
// format comments, this function also requires the transactions that spend the
// txouts and a utxo view that contains any remaining existing utxos in the
// transactions referenced by the inputs to the passed transasctions.
func (s *SpendJournal) deserializeSpendJournalEntry(serialized []byte, txns []*wire.MsgTx) ([]SpentTxOut, error) {
	// Calculate the total number of stxos.
	var numStxos int
	for _, tx := range txns {
		if stake.IsSSGen(tx) {
			numStxos++
			continue
		}
		numStxos += len(tx.TxIn)
	}

	// When a block has no spent txouts there is nothing to serialize.
	if len(serialized) == 0 {
		// Ensure the block actually has no stxos.  This should never
		// happen unless there is database corruption or an empty entry
		// erroneously made its way into the database.
		if numStxos != 0 {
			return nil, AssertError(fmt.Sprintf("mismatched spend "+
				"journal serialization - no serialization for "+
				"expected %d stxos", numStxos))
		}

		return nil, nil
	}

	// Loop backwards through all transactions so everything is read in
	// reverse order to match the serialization order.
	stxoIdx := numStxos - 1
	offset := 0
	stxos := make([]SpentTxOut, numStxos)
	for txIdx := len(txns) - 1; txIdx > -1; txIdx-- {
		tx := txns[txIdx]
		isVote := stake.IsSSGen(tx)

		// Loop backwards through all of the transaction inputs and read
		// the associated stxo.
		for txInIdx := len(tx.TxIn) - 1; txInIdx > -1; txInIdx-- {
			// Skip stakebase since it has no input.
			if isVote && txInIdx == 0 {
				continue
			}

			txIn := tx.TxIn[txInIdx]
			stxo := &stxos[stxoIdx]
			stxoIdx--

			// Get the transaction version for the stxo based on
			// whether or not it should be serialized as a part of
			// the stxo.  Recall that it is only serialized when the
			// stxo spends the final utxo of a transaction.  Since
			// they are deserialized in reverse order, this means
			// the first time an entry for a given containing tx is
			// encountered that is not already in the utxo view it
			// must have been the final spend and thus the extra
			// data will be serialized with the stxo.  Otherwise,
			// the version must be pulled from the utxo entry.
			//
			// Since the view is not actually modified as the stxos
			// are read here and it's possible later entries
			// reference earlier ones, an inflight map is maintained
			// to detect this case and pull the tx version from the
			// entry that contains the version information as just
			// described.
			n, err := decodeSpentTxOut(serialized[offset:], stxo, txIn.ValueIn,
				txIn.BlockHeight, txIn.BlockIndex)
			offset += n
			if err != nil {
				return nil, errDeserialize(fmt.Sprintf("unable "+
					"to decode stxo for %v: %v",
					txIn.PreviousOutPoint, err))
			}
		}
	}

	return stxos, nil
}

// serializeSpendJournalEntry serializes all of the passed spent txouts into a
// single byte slice according to the format described in detail above.
func (s *SpendJournal) serializeSpendJournalEntry(stxos []SpentTxOut) ([]byte, error) {
	if len(stxos) == 0 {
		return nil, nil
	}

	// Calculate the size needed to serialize the entire journal entry.
	var size int
	var sizes []int
	for i := range stxos {
		sz := spentTxOutSerializeSize(&stxos[i])
		sizes = append(sizes, sz)
		size += sz
	}
	serialized := make([]byte, size)

	// Serialize each individual stxo directly into the slice in reverse
	// order one after the other.
	var offset int
	for i := len(stxos) - 1; i > -1; i-- {
		oldOffset := offset
		offset += putSpentTxOut(serialized[offset:], &stxos[i])

		if offset-oldOffset != sizes[i] {
			return nil, AssertError(fmt.Sprintf("bad write; expect sz %v, "+
				"got sz %v (wrote %x)", sizes[i], offset-oldOffset,
				serialized[oldOffset:offset]))
		}
	}

	return serialized, nil
}

// DbFetchSpendJournalEntry fetches the spend journal entry for the passed
// block and deserializes it into a slice of spent txout entries.  The provided
// view MUST have the utxos referenced by all of the transactions available for
// the passed block since that information is required to reconstruct the spent
// txouts.
func (s *SpendJournal) DbFetchSpendJournalEntry(dbTx database.Tx, block *dcrutil.Block) ([]SpentTxOut, error) {
	// Exclude the coinbase transaction since it can't spend anything.
	spendBucket := dbTx.Metadata().Bucket(dbnamespace.SpendJournalBucketName)
	serialized := spendBucket.Get(block.Hash()[:])
	var blockTxns []*wire.MsgTx
	blockTxns = append(blockTxns, block.MsgBlock().STransactions...)
	blockTxns = append(blockTxns, block.MsgBlock().Transactions[1:]...)
	if len(blockTxns) > 0 && len(serialized) == 0 {
		panicf("missing spend journal data for %s", block.Hash())
	}

	stxos, err := s.deserializeSpendJournalEntry(serialized, blockTxns)
	if err != nil {
		// Ensure any deserialization errors are returned as database
		// corruption errors.
		if isDeserializeErr(err) {
			return nil, database.Error{
				ErrorCode: database.ErrCorruption,
				Description: fmt.Sprintf("corrupt spend "+
					"information for %v: %v", block.Hash(),
					err),
			}
		}

		return nil, err
	}

	return stxos, nil
}

// DbPutSpendJournalEntry uses an existing database transaction to update the
// spend journal entry for the given block hash using the provided slice of
// spent txouts.   The spent txouts slice must contain an entry for every txout
// the transactions in the block spend in the order they are spent.
func (s *SpendJournal) DbPutSpendJournalEntry(dbTx database.Tx, blockHash *chainhash.Hash, stxos []SpentTxOut) error {
	spendBucket := dbTx.Metadata().Bucket(dbnamespace.SpendJournalBucketName)
	serialized, err := s.serializeSpendJournalEntry(stxos)
	if err != nil {
		return err
	}
	return spendBucket.Put(blockHash[:], serialized)
}

// DbRemoveSpendJournalEntry uses an existing database transaction to remove the
// spend journal entry for the passed block hash.
func (s *SpendJournal) DbRemoveSpendJournalEntry(dbTx database.Tx, blockHash *chainhash.Hash) error {
	spendBucket := dbTx.Metadata().Bucket(dbnamespace.SpendJournalBucketName)
	return spendBucket.Delete(blockHash[:])
}
