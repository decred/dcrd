// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"

	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/txscript/v3"
	"github.com/decred/dcrd/wire"
)

// utxoOutput houses details about an individual unspent transaction output such
// as whether or not it is spent, its public key script, and how much it pays.
//
// Standard public key scripts are stored in the database using a compressed
// format. Since the vast majority of scripts are of the standard form, a fairly
// significant savings is achieved by discarding the portions of the standard
// scripts that can be reconstructed.
//
// Also, since it is common for only a specific output in a given utxo entry to
// be referenced from a redeeming transaction, the script and amount for a given
// output is not uncompressed until the first time it is accessed.  This
// provides a mechanism to avoid the overhead of needlessly uncompressing all
// outputs for a given utxo entry at the time of load.
//
// The struct is aligned for memory efficiency.
type utxoOutput struct {
	pkScript      []byte // The public key script for the output.
	amount        int64  // The amount of the output.
	scriptVersion uint16 // The script version
	compressed    bool   // The public key script is compressed.
	spent         bool   // Output is spent.
}

// maybeDecompress decompresses the amount and public key script fields of the
// utxo and marks it decompressed if needed.
func (o *utxoOutput) maybeDecompress(compressionVersion uint32) {
	// Nothing to do if it's not compressed.
	if !o.compressed {
		return
	}

	o.pkScript = decompressScript(o.pkScript, compressionVersion)
	o.compressed = false
}

// UtxoEntry contains contextual information about an unspent transaction such
// as whether or not it is a coinbase transaction, which block it was found in,
// and the spent status of its outputs.
//
// The struct is aligned for memory efficiency.
type UtxoEntry struct {
	sparseOutputs map[uint32]*utxoOutput // Sparse map of unspent outputs.
	stakeExtra    []byte                 // Extra data for the staking system.

	txType    stake.TxType // The stake type of the transaction.
	height    uint32       // Height of block containing tx.
	index     uint32       // Index of containing tx in block.
	txVersion uint16       // The tx version of this tx.

	isCoinBase bool // Whether entry is a coinbase tx.
	hasExpiry  bool // Whether entry has an expiry.
	modified   bool // Entry changed since load.
}

// TxVersion returns the version of the transaction the utxo represents.
func (entry *UtxoEntry) TxVersion() uint16 {
	return entry.txVersion
}

// HasExpiry returns the transaction expiry for the transaction that the utxo
// entry represents.
func (entry *UtxoEntry) HasExpiry() bool {
	return entry.hasExpiry
}

// IsCoinBase returns whether or not the transaction the utxo entry represents
// is a coinbase.
func (entry *UtxoEntry) IsCoinBase() bool {
	return entry.isCoinBase
}

// BlockHeight returns the height of the block containing the transaction the
// utxo entry represents.
func (entry *UtxoEntry) BlockHeight() int64 {
	return int64(entry.height)
}

// BlockIndex returns the height of the block containing the transaction the
// utxo entry represents.
func (entry *UtxoEntry) BlockIndex() uint32 {
	return entry.index
}

// TransactionType returns the type of the transaction the utxo entry
// represents.
func (entry *UtxoEntry) TransactionType() stake.TxType {
	return entry.txType
}

// IsOutputSpent returns whether or not the provided output index has been
// spent based upon the current state of the unspent transaction output view
// the entry was obtained from.
//
// Returns true if the output index references an output that does not exist
// either due to it being invalid or because the output is not part of the view
// due to previously being spent/pruned.
func (entry *UtxoEntry) IsOutputSpent(outputIndex uint32) bool {
	output, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return true
	}

	return output.spent
}

// SpendOutput marks the output at the provided index as spent.  Specifying an
// output index that does not exist will not have any effect.
func (entry *UtxoEntry) SpendOutput(outputIndex uint32) {
	output, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return
	}

	// Nothing to do if the output is already spent.
	if output.spent {
		return
	}

	entry.modified = true
	output.spent = true
}

// IsFullySpent returns whether or not the transaction the utxo entry represents
// is fully spent.
func (entry *UtxoEntry) IsFullySpent() bool {
	// The entry is not fully spent if any of the outputs are unspent.
	for _, output := range entry.sparseOutputs {
		if !output.spent {
			return false
		}
	}

	return true
}

// AmountByIndex returns the amount of the provided output index.
//
// Returns 0 if the output index references an output that does not exist
// either due to it being invalid or because the output is not part of the view
// due to previously being spent/pruned.
func (entry *UtxoEntry) AmountByIndex(outputIndex uint32) int64 {
	output, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return 0
	}

	return output.amount
}

// ScriptVersionByIndex returns the public key script for the provided output
// index.
//
// Returns 0 if the output index references an output that does not exist
// either due to it being invalid or because the output is not part of the view
// due to previously being spent/pruned.
func (entry *UtxoEntry) ScriptVersionByIndex(outputIndex uint32) uint16 {
	output, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return 0
	}

	return output.scriptVersion
}

// PkScriptByIndex returns the public key script for the provided output index.
//
// Returns nil if the output index references an output that does not exist
// either due to it being invalid or because the output is not part of the view
// due to previously being spent/pruned.
func (entry *UtxoEntry) PkScriptByIndex(outputIndex uint32) []byte {
	output, ok := entry.sparseOutputs[outputIndex]
	if !ok {
		return nil
	}

	// Ensure the output is decompressed before returning the script.
	output.maybeDecompress(currentCompressionVersion)
	return output.pkScript
}

// Clone returns a deep copy of the utxo entry.
func (entry *UtxoEntry) Clone() *UtxoEntry {
	if entry == nil {
		return nil
	}

	newEntry := &UtxoEntry{
		stakeExtra:    make([]byte, len(entry.stakeExtra)),
		txVersion:     entry.txVersion,
		height:        entry.height,
		index:         entry.index,
		txType:        entry.txType,
		isCoinBase:    entry.isCoinBase,
		hasExpiry:     entry.hasExpiry,
		sparseOutputs: make(map[uint32]*utxoOutput),
	}
	copy(newEntry.stakeExtra, entry.stakeExtra)
	for outputIndex, output := range entry.sparseOutputs {
		newEntry.sparseOutputs[outputIndex] = &utxoOutput{
			pkScript:      output.pkScript,
			amount:        output.amount,
			scriptVersion: output.scriptVersion,
			compressed:    output.compressed,
			spent:         output.spent,
		}
	}
	return newEntry
}

// newUtxoEntry returns a new unspent transaction output entry with the provided
// coinbase flag and block height ready to have unspent outputs added.
func newUtxoEntry(txVersion uint16, height uint32, index uint32, isCoinBase bool, hasExpiry bool, txType stake.TxType) *UtxoEntry {
	return &UtxoEntry{
		sparseOutputs: make(map[uint32]*utxoOutput),
		txVersion:     txVersion,
		height:        height,
		index:         index,
		isCoinBase:    isCoinBase,
		hasExpiry:     hasExpiry,
		txType:        txType,
	}
}

// UtxoViewpoint represents a view into the set of unspent transaction outputs
// from a specific point of view in the chain.  For example, it could be for
// the end of the main chain, some point in the history of the main chain, or
// down a side chain.
//
// The unspent outputs are needed by other transactions for things such as
// script validation and double spend prevention.
type UtxoViewpoint struct {
	entries    map[chainhash.Hash]*UtxoEntry
	bestHash   chainhash.Hash
	blockChain *BlockChain
}

// BestHash returns the hash of the best block in the chain the view currently
// represents.
func (view *UtxoViewpoint) BestHash() *chainhash.Hash {
	return &view.bestHash
}

// SetBestHash sets the hash of the best block in the chain the view currently
// represents.
func (view *UtxoViewpoint) SetBestHash(hash *chainhash.Hash) {
	view.bestHash = *hash
}

// LookupEntry returns information about a given transaction according to the
// current state of the view.  It will return nil if the passed transaction
// hash does not exist in the view or is otherwise not available such as when
// it has been disconnected during a reorg.
func (view *UtxoViewpoint) LookupEntry(txHash *chainhash.Hash) *UtxoEntry {
	entry, ok := view.entries[*txHash]
	if !ok {
		return nil
	}

	return entry
}

// PrevScript returns the script and script version associated with the provided
// previous outpoint along with a bool that indicates whether or not the
// requested entry exists.  This ensures the caller is able to distinguish
// between missing entry and empty v0 scripts.
func (view *UtxoViewpoint) PrevScript(prevOut *wire.OutPoint) (uint16, []byte, bool) {
	entry := view.LookupEntry(&prevOut.Hash)
	if entry == nil {
		return 0, nil, false
	}

	version := entry.ScriptVersionByIndex(prevOut.Index)
	pkScript := entry.PkScriptByIndex(prevOut.Index)
	return version, pkScript, true
}

// PriorityInput returns the block height and amount associated with the
// provided previous outpoint along with a bool that indicates whether or not
// the requested entry exists.  This ensures the caller is able to distinguish
// missing entries from zero values.
func (view *UtxoViewpoint) PriorityInput(prevOut *wire.OutPoint) (int64, int64, bool) {
	originHash := &prevOut.Hash
	originIndex := prevOut.Index
	txEntry := view.LookupEntry(originHash)
	if txEntry != nil && !txEntry.IsOutputSpent(originIndex) {
		return txEntry.BlockHeight(), txEntry.AmountByIndex(originIndex), true
	}

	return 0, 0, false
}

// AddTxOuts adds all outputs in the passed transaction which are not provably
// unspendable to the view.  When the view already has entries for any of the
// outputs, they are simply marked unspent.  All fields will be updated for
// existing entries since it's possible it has changed during a reorg.
func (view *UtxoViewpoint) AddTxOuts(tx *dcrutil.Tx, blockHeight int64, blockIndex uint32, isTreasuryEnabled bool) {
	// When there are not already any utxos associated with the transaction,
	// add a new entry for it to the view.
	entry := view.LookupEntry(tx.Hash())
	if entry == nil {
		msgTx := tx.MsgTx()
		txType := stake.DetermineTxType(msgTx, isTreasuryEnabled)
		entry = newUtxoEntry(msgTx.Version, uint32(blockHeight),
			blockIndex, standalone.IsCoinBaseTx(msgTx, isTreasuryEnabled),
			msgTx.Expiry != 0, txType)
		if txType == stake.TxTypeSStx {
			stakeExtra := make([]byte, serializeSizeForMinimalOutputs(tx))
			putTxToMinimalOutputs(stakeExtra, tx)
			entry.stakeExtra = stakeExtra
		}
		view.entries[*tx.Hash()] = entry
	} else {
		entry.height = uint32(blockHeight)
		entry.index = blockIndex
	}
	entry.modified = true

	// Loop all of the transaction outputs and add those which are not
	// provably unspendable.
	for txOutIdx, txOut := range tx.MsgTx().TxOut {
		// TODO allow pruning of stake utxs after all other outputs are spent
		if txscript.IsUnspendable(txOut.Value, txOut.PkScript) {
			continue
		}

		// Update existing entries.  All fields are updated because it's
		// possible (although extremely unlikely) that the existing
		// entry is being replaced by a different transaction with the
		// same hash.  This is allowed so long as the previous
		// transaction is fully spent.
		if output, ok := entry.sparseOutputs[uint32(txOutIdx)]; ok {
			output.spent = false
			output.amount = txOut.Value
			output.scriptVersion = txOut.Version
			output.pkScript = txOut.PkScript
			output.compressed = false
			continue
		}

		// Add the unspent transaction output.
		entry.sparseOutputs[uint32(txOutIdx)] = &utxoOutput{
			spent:         false,
			amount:        txOut.Value,
			scriptVersion: txOut.Version,
			pkScript:      txOut.PkScript,
			compressed:    false,
		}
	}
}

// connectTransaction updates the view by adding all new utxos created by the
// passed transaction and marking all utxos that the transactions spend as
// spent.  In addition, when the 'stxos' argument is not nil, it will be updated
// to append an entry for each spent txout.  An error will be returned if the
// view does not contain the required utxos.
func (view *UtxoViewpoint) connectTransaction(tx *dcrutil.Tx, blockHeight int64, blockIndex uint32, stxos *[]spentTxOut, isTreasuryEnabled bool) error {
	// Coinbase transactions don't have any inputs to spend.
	msgTx := tx.MsgTx()
	if standalone.IsCoinBaseTx(msgTx, isTreasuryEnabled) {
		// Add the transaction's outputs as available utxos.
		view.AddTxOuts(tx, blockHeight, blockIndex, isTreasuryEnabled)
		return nil
	}

	// Treasurybase transactions don't have any inputs to spend.
	if isTreasuryEnabled && standalone.IsTreasuryBase(msgTx) {
		return nil
	}

	// Spend the referenced utxos by marking them spent in the view and,
	// if a slice was provided for the spent txout details, append an entry
	// to it.
	isVote := stake.IsSSGen(msgTx, isTreasuryEnabled)
	var isTSpend bool
	if isTreasuryEnabled {
		isTSpend = stake.IsTSpend(msgTx)
	}
	for txInIdx, txIn := range msgTx.TxIn {
		// Ignore stakebase since it has no input.
		if isVote && txInIdx == 0 {
			continue
		}

		// Ignore TSpend since it has no input.
		if isTSpend {
			continue
		}

		// Ensure the referenced utxo exists in the view.  This should
		// never happen unless there is a bug is introduced in the code.
		originIndex := txIn.PreviousOutPoint.Index
		entry := view.entries[txIn.PreviousOutPoint.Hash]
		if entry == nil {
			return AssertError(fmt.Sprintf("view missing input %v",
				txIn.PreviousOutPoint))
		}
		entry.SpendOutput(originIndex)

		// Don't create the stxo details if not requested.
		if stxos == nil {
			continue
		}

		// Populate the stxo details using the utxo entry.  When the
		// transaction is fully spent, set the additional stxo fields
		// accordingly since those details will no longer be available
		// in the utxo set.
		var stxo = spentTxOut{
			compressed:    false,
			amount:        txIn.ValueIn,
			scriptVersion: entry.ScriptVersionByIndex(originIndex),
			pkScript:      entry.PkScriptByIndex(originIndex),
		}
		if entry.IsFullySpent() {
			stxo.txVersion = entry.TxVersion()
			stxo.height = uint32(entry.BlockHeight())
			stxo.index = entry.BlockIndex()
			stxo.isCoinBase = entry.IsCoinBase()
			stxo.hasExpiry = entry.HasExpiry()
			stxo.txType = entry.TransactionType()
			stxo.txFullySpent = true

			if entry.txType == stake.TxTypeSStx {
				stxo.stakeExtra = entry.stakeExtra
			}
		}

		// Append the entry to the provided spent txouts slice.
		*stxos = append(*stxos, stxo)
	}

	// Add the transaction's outputs as available utxos.
	view.AddTxOuts(tx, blockHeight, blockIndex, isTreasuryEnabled)

	return nil
}

// disconnectTransactions updates the view by removing all utxos created by
// the transactions in either the regular or stake tree of the block, depending
// on the flag, and unspending all of the txos spent by those same transactions
// by using the provided spent txo information.
func (view *UtxoViewpoint) disconnectTransactions(block *dcrutil.Block, stxos []spentTxOut, stakeTree bool, isTreasuryEnabled bool) error {
	// Choose which transaction tree to use and the appropriate offset into the
	// spent transaction outputs that corresponds to them depending on the flag.
	// Transactions in the stake tree are spent before transactions in the
	// regular tree, thus skip all of the outputs spent by the regular tree when
	// disconnecting stake transactions.
	stxoIdx := len(stxos) - 1
	transactions := block.Transactions()
	if stakeTree {
		stxoIdx = len(stxos) - countSpentRegularOutputs(block) - 1
		transactions = block.STransactions()
	}

	for txIdx := len(transactions) - 1; txIdx > -1; txIdx-- {
		tx := transactions[txIdx]
		msgTx := tx.MsgTx()
		txType := stake.TxTypeRegular
		if stakeTree {
			txType = stake.DetermineTxType(msgTx, isTreasuryEnabled)
		}
		isVote := txType == stake.TxTypeSSGen

		var isTreasuryBase, isTSpend bool

		if isTreasuryEnabled {
			isTSpend = txType == stake.TxTypeTSpend && stakeTree
			isTreasuryBase = txType == stake.TxTypeTreasuryBase &&
				stakeTree && txIdx == 0
		}

		// Clear this transaction from the view if it already exists or create a
		// new empty entry for when it does not.  This is done because the code
		// relies on its existence in the view in order to signal modifications
		// have happened.
		isCoinbase := !stakeTree && txIdx == 0
		entry := view.entries[*tx.Hash()]
		if entry == nil {
			entry = newUtxoEntry(msgTx.Version, uint32(block.Height()),
				uint32(txIdx), isCoinbase, msgTx.Expiry != 0, txType)
			view.entries[*tx.Hash()] = entry
		}
		entry.modified = true
		entry.sparseOutputs = make(map[uint32]*utxoOutput)

		// Loop backwards through all of the transaction inputs (except
		// for the coinbase and treasurybase which have no inputs) and
		// unspend the referenced txos.  This is necessary to match the
		// order of the spent txout entries.
		if isCoinbase || isTreasuryBase || isTSpend {
			continue
		}
		for txInIdx := len(msgTx.TxIn) - 1; txInIdx > -1; txInIdx-- {
			// Ignore stakebase since it has no input.
			if isVote && txInIdx == 0 {
				continue
			}

			// Ensure the spent txout index is decremented to stay in sync with
			// the transaction input.
			stxo := &stxos[stxoIdx]
			stxoIdx--

			// When there is not already an entry for the referenced transaction
			// in the view, it means it was fully spent, so create a new utxo
			// entry in order to resurrect it.
			txIn := msgTx.TxIn[txInIdx]
			originHash := &txIn.PreviousOutPoint.Hash
			originIndex := txIn.PreviousOutPoint.Index
			entry := view.entries[*originHash]
			if entry == nil {
				if !stxo.txFullySpent {
					return AssertError(fmt.Sprintf("tried to revive unspent "+
						"tx %v from non-fully spent stx entry", originHash))
				}
				entry = newUtxoEntry(msgTx.Version, stxo.height, stxo.index,
					stxo.isCoinBase, stxo.hasExpiry, stxo.txType)
				if stxo.txType == stake.TxTypeSStx {
					entry.stakeExtra = stxo.stakeExtra
				}
				view.entries[*originHash] = entry
			}

			// Mark the entry as modified since it is either new or will be
			// changed below.
			entry.modified = true

			// Restore the specific utxo using the stxo data from the spend
			// journal if it doesn't already exist in the view.
			output, ok := entry.sparseOutputs[originIndex]
			if !ok {
				// Add the unspent transaction output.
				entry.sparseOutputs[originIndex] = &utxoOutput{
					compressed:    stxo.compressed,
					spent:         false,
					amount:        txIn.ValueIn,
					scriptVersion: stxo.scriptVersion,
					pkScript:      stxo.pkScript,
				}
				continue
			}

			// Mark the existing referenced transaction output as unspent.
			output.spent = false
		}
	}

	return nil
}

// disconnectRegularTransactions updates the view by removing all utxos created
// by the transactions in regular tree of the provided block and unspending all
// of the txos spent by those same transactions by using the provided spent txo
// information.
func (view *UtxoViewpoint) disconnectRegularTransactions(block *dcrutil.Block, stxos []spentTxOut, isTreasuryEnabled bool) error {
	return view.disconnectTransactions(block, stxos, false, isTreasuryEnabled)
}

// disconnectStakeTransactions updates the view by removing all utxos created
// by the transactions in stake tree of the provided block and unspending all
// of the txos spent by those same transactions by using the provided spent txo
// information.
func (view *UtxoViewpoint) disconnectStakeTransactions(block *dcrutil.Block, stxos []spentTxOut, isTreasuryEnabled bool) error {
	return view.disconnectTransactions(block, stxos, true, isTreasuryEnabled)
}

// disconnectDisapprovedBlock updates the view by disconnecting all of the
// transactions in the regular tree of the passed block.
//
// Disconnecting a transaction entails removing the utxos created by it and
// restoring the outputs spent by it with the help of the provided spent txo
// information.
//func (view *UtxoViewpoint) disconnectDisapprovedBlock(db database.DB, block *dcrutil.Block, stxos []spentTxOut) error {
func (view *UtxoViewpoint) disconnectDisapprovedBlock(db database.DB, block *dcrutil.Block, isTreasuryEnabled bool) error {
	// Load all of the spent txos for the block from the database spend journal.
	var stxos []spentTxOut
	err := db.View(func(dbTx database.Tx) error {
		var err error
		stxos, err = dbFetchSpendJournalEntry(dbTx, block, isTreasuryEnabled)
		return err
	})
	if err != nil {
		return err
	}

	// Load all of the utxos referenced by the inputs for all transactions in
	// the block that don't already exist in the utxo view from the database.
	err = view.fetchRegularInputUtxos(db, block, isTreasuryEnabled)
	if err != nil {
		return err
	}

	// Sanity check the correct number of stxos are provided.
	if len(stxos) != countSpentOutputs(block, isTreasuryEnabled) {
		panicf("provided %v stxos for block %v (height %v) which spends %v "+
			"outputs", len(stxos), block.Hash(), block.MsgBlock().Header.Height,
			countSpentOutputs(block, isTreasuryEnabled))
	}

	return view.disconnectRegularTransactions(block, stxos, isTreasuryEnabled)
}

// connectBlock updates the view by potentially disconnecting all of the
// transactions in the regular tree of the parent block of the passed block in
// the case the passed block disapproves the parent block, connecting all of
// transactions in both the regular and stake trees of the passed block, and
// setting the best hash for the view to the passed block.
//
// Connecting a transaction entails marking all utxos it spends as spent, and
// adding all of the new utxos created by it.
//
// Disconnecting a transaction entails removing the utxos created by it and
// restoring the outputs spent by it with the help of the provided spent txo
// information.
//
// In addition, when the 'stxos' argument is not nil, it will be updated to
// append an entry for each spent txout.
func (view *UtxoViewpoint) connectBlock(db database.DB, block, parent *dcrutil.Block, stxos *[]spentTxOut, isTreasuryEnabled bool) error {
	// Disconnect the transactions in the regular tree of the parent block if
	// the passed block disapproves it.
	if !headerApprovesParent(&block.MsgBlock().Header) {
		err := view.disconnectDisapprovedBlock(db, parent,
			isTreasuryEnabled)
		if err != nil {
			return err
		}
	}

	// Load all of the utxos referenced by the inputs for all transactions in
	// the block that don't already exist in the utxo view from the database.
	err := view.fetchInputUtxos(db, block, isTreasuryEnabled)
	if err != nil {
		return err
	}

	// Connect all of the transactions in both the regular and stake trees of
	// the block.  Notice that the stake tree is connected before the regular
	// tree.  This means that stake transactions are not able to redeem outputs
	// of transactions created in the regular tree of the same block, which is
	// important since the regular tree may be disapproved by the subsequent
	// block while the stake tree must remain valid.
	for i, stx := range block.STransactions() {
		err := view.connectTransaction(stx, block.Height(), uint32(i),
			stxos, isTreasuryEnabled)
		if err != nil {
			return err
		}
	}
	for i, tx := range block.Transactions() {
		err := view.connectTransaction(tx, block.Height(), uint32(i),
			stxos, isTreasuryEnabled)
		if err != nil {
			return err
		}
	}

	// Update the best hash for view to include this block since all of its
	// transactions have been connected.
	view.SetBestHash(block.Hash())
	return nil
}

// disconnectBlock updates the view by disconnecting all transactions in both
// the regular and stake trees of the passed block, in the case the block
// disapproves the parent block, reconnecting all of the transactions in the
// regular tree of the previous block, and setting the best hash for the view to
// the parent block.
//
// Connecting a transaction entails marking all utxos it spends as spent, and
// adding all of the new utxos created by it.
//
// Disconnecting a transaction entails removing the utxos created by it and
// restoring the outputs spent by it with the help of the provided spent txo
// information.
//
// Note that, unlike block connection, the spent transaction output (stxo)
// information is required and failure to provide it will result in an assertion
// panic.
func (view *UtxoViewpoint) disconnectBlock(db database.DB, block, parent *dcrutil.Block, stxos []spentTxOut, isTreasuryEnabled bool) error {
	// Sanity check the correct number of stxos are provided.
	if len(stxos) != countSpentOutputs(block, isTreasuryEnabled) {
		panicf("provided %v stxos for block %v (height %v) which spends %v "+
			"outputs", len(stxos), block.Hash(), block.MsgBlock().Header.Height,
			countSpentOutputs(block, isTreasuryEnabled))
	}

	// Load all of the utxos referenced by the inputs for all transactions in
	// the block don't already exist in the utxo view from the database.
	err := view.fetchInputUtxos(db, block, isTreasuryEnabled)
	if err != nil {
		return err
	}

	// Disconnect all of the transactions in both the regular and stake trees of
	// the block.  Notice that the regular tree is disconnected before the stake
	// tree since that is the reverse of how they are connected.
	err = view.disconnectRegularTransactions(block, stxos, isTreasuryEnabled)
	if err != nil {
		return err
	}
	err = view.disconnectStakeTransactions(block, stxos, isTreasuryEnabled)
	if err != nil {
		return err
	}

	// Reconnect the transactions in the regular tree of the parent block if the
	// block that is being disconnected disapproves it.
	if !headerApprovesParent(&block.MsgBlock().Header) {
		// Load all of the utxos referenced by the inputs for all transactions
		// in the regular tree of the parent block that don't already exist in
		// the utxo view from the database.
		err := view.fetchRegularInputUtxos(db, parent, isTreasuryEnabled)
		if err != nil {
			return err
		}

		for i, tx := range parent.Transactions() {
			err := view.connectTransaction(tx, parent.Height(),
				uint32(i), nil, isTreasuryEnabled)
			if err != nil {
				return err
			}
		}
	}

	// Update the best hash for view to include this block since all of its
	// transactions have been connected.
	view.SetBestHash(&block.MsgBlock().Header.PrevBlock)
	return nil
}

// Entries returns the underlying map that stores of all the utxo entries.
func (view *UtxoViewpoint) Entries() map[chainhash.Hash]*UtxoEntry {
	return view.entries
}

// commit prunes all entries marked modified that are now fully spent and marks
// all entries as unmodified.
func (view *UtxoViewpoint) commit() {
	for txHash, entry := range view.entries {
		if entry == nil || (entry.modified && entry.IsFullySpent()) {
			delete(view.entries, txHash)
			continue
		}

		entry.modified = false
	}
}

// viewFilteredSet represents a set of utxos to fetch from the database that are
// not already in a view.
type viewFilteredSet map[chainhash.Hash]struct{}

// add conditionally adds the provided utxo hash to the set if it does not
// already exist in the provided view.
func (set viewFilteredSet) add(view *UtxoViewpoint, hash *chainhash.Hash) {
	if _, ok := view.entries[*hash]; !ok {
		set[*hash] = struct{}{}
	}
}

// fetchUtxosMain fetches unspent transaction output data about the provided
// set of transactions from the point of view of the end of the main chain at
// the time of the call.
//
// Upon completion of this function, the view will contain an entry for each
// requested transaction.  Fully spent transactions, or those which otherwise
// don't exist, will result in a nil entry in the view.
func (view *UtxoViewpoint) fetchUtxosMain(db database.DB, filteredSet viewFilteredSet) error {
	// Nothing to do if there are no requested hashes.
	if len(filteredSet) == 0 {
		return nil
	}

	// Load the unspent transaction output information for the requested set
	// of transactions from the point of view of the end of the main chain.
	//
	// NOTE: Missing entries are not considered an error here and instead
	// will result in nil entries in the view.  This is intentionally done
	// since other code uses the presence of an entry in the store as a way
	// to optimize spend and unspend updates to apply only to the specific
	// utxos that the caller needs access to.
	return db.View(func(dbTx database.Tx) error {
		for hash := range filteredSet {
			hashCopy := hash
			entry, err := dbFetchUtxoEntry(dbTx, &hashCopy)
			if err != nil {
				return err
			}

			view.entries[hash] = entry
		}

		return nil
	})
}

// addRegularInputUtxos adds any outputs of transactions in the regular tree of
// the provided block that are referenced by inputs of transactions that are
// located later in the regular tree of the block and returns a set of the
// referenced outputs that are not already in the view and thus need to be
// fetched from the database.
func (view *UtxoViewpoint) addRegularInputUtxos(block *dcrutil.Block, isTreasuryEnabled bool) viewFilteredSet {
	// Build a map of in-flight transactions because some of the inputs in the
	// regular transaction tree of this block could be referencing other
	// transactions earlier in the block which are not yet in the chain.
	txInFlight := map[chainhash.Hash]int{}
	regularTxns := block.Transactions()
	for i, tx := range regularTxns {
		txInFlight[*tx.Hash()] = i
	}

	// Loop through all of the inputs of the transactions in the regular
	// transaction tree (except for the coinbase which has no inputs) collecting
	// them into sets of what is needed and what is already known (in-flight).
	filteredSet := make(viewFilteredSet)
	for i, tx := range regularTxns[1:] {
		for _, txIn := range tx.MsgTx().TxIn {
			// It is acceptable for a transaction input in the regular tree to
			// reference the output of another transaction in the regular tree
			// of this block only if the referenced transaction comes before the
			// current one in this block.  Add the outputs of the referenced
			// transaction as available utxos when this is the case.  Otherwise,
			// the utxo details are still needed.
			//
			// NOTE: The >= is correct here because i is one less than the
			// actual position of the transaction within the block due to
			// skipping the coinbase.
			originHash := &txIn.PreviousOutPoint.Hash
			if inFlightIndex, ok := txInFlight[*originHash]; ok &&
				i >= inFlightIndex {

				originTx := regularTxns[inFlightIndex]
				view.AddTxOuts(originTx, block.Height(),
					uint32(inFlightIndex), isTreasuryEnabled)
				continue
			}

			// Only request entries that are not already in the view from the
			// database.
			filteredSet.add(view, originHash)
		}
	}

	return filteredSet
}

// fetchRegularInputUtxos loads utxo details about the input transactions
// referenced by the transactions in the regular tree of the given block into
// the view from the database as needed.  In particular, referenced entries that
// are earlier in the block are added to the view and entries that are already
// in the view are not modified.
func (view *UtxoViewpoint) fetchRegularInputUtxos(db database.DB, block *dcrutil.Block, isTreasuryEnabled bool) error {
	// Add any outputs of transactions in the regular tree of the block that are
	// referenced by inputs of transactions that are located later in the tree
	// and fetch any inputs that are not already in the view from the database.
	filteredSet := view.addRegularInputUtxos(block, isTreasuryEnabled)
	return view.fetchUtxosMain(db, filteredSet)
}

// fetchInputUtxos loads utxo details about the input transactions referenced
// by the transactions in both the regular and stake trees of the given block
// into the view from the database as needed.  In the case of regular tree,
// referenced entries that are earlier in the regular tree of the block are
// added to the view.  In all cases, entries that are already in the view are
// not modified.
func (view *UtxoViewpoint) fetchInputUtxos(db database.DB, block *dcrutil.Block, isTreasuryEnabled bool) error {
	// Add any outputs of transactions in the regular tree of the block that are
	// referenced by inputs of transactions that are located later in the tree
	// and, while doing so, determine which inputs are not already in the view
	// and thus need to be fetched from the database.
	filteredSet := view.addRegularInputUtxos(block, isTreasuryEnabled)

	// Loop through all of the inputs of the transaction in the stake tree and
	// add those that aren't already known to the set of what is needed.
	//
	// Note that, unlike in the regular transaction tree, transactions in the
	// stake tree are not allowed to access outputs of transactions earlier in
	// the block.  This applies to both transactions earlier in the stake tree
	// as well as those in the regular tree.
	for _, stx := range block.STransactions() {
		isVote := stake.IsSSGen(stx.MsgTx(), isTreasuryEnabled)
		for txInIdx, txIn := range stx.MsgTx().TxIn {
			// Ignore stakebase since it has no input.
			if isVote && txInIdx == 0 {
				continue
			}

			// Only request entries that are not already in the view
			// from the database.
			originHash := &txIn.PreviousOutPoint.Hash
			filteredSet.add(view, originHash)
		}
	}

	// Request the input utxos from the database.
	return view.fetchUtxosMain(db, filteredSet)
}

// clone returns a deep copy of the view.
func (view *UtxoViewpoint) clone() *UtxoViewpoint {
	clonedView := &UtxoViewpoint{
		entries:    make(map[chainhash.Hash]*UtxoEntry),
		bestHash:   view.bestHash,
		blockChain: view.blockChain,
	}

	for txHash, entry := range view.entries {
		clonedView.entries[txHash] = entry.Clone()
	}

	return clonedView
}

// NewUtxoViewpoint returns a new empty unspent transaction output view.
func NewUtxoViewpoint(blockChain *BlockChain) *UtxoViewpoint {
	return &UtxoViewpoint{
		entries:    make(map[chainhash.Hash]*UtxoEntry),
		blockChain: blockChain,
	}
}

// FetchUtxoView loads utxo details about the input transactions referenced by
// the passed transaction from the point of view of the end of the main chain
// while taking into account whether or not the transactions in the regular tree
// of the current tip block should be included or not depending on the provided
// flag.  It also attempts to fetch the utxo details for the transaction itself
// so the returned view can be examined for duplicate unspent transaction
// outputs.
//
// This function is safe for concurrent access however the returned view is NOT.
func (b *BlockChain) FetchUtxoView(tx *dcrutil.Tx, includeRegularTxns bool) (*UtxoViewpoint, error) {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	// The genesis block does not have any spendable transactions, so there
	// can't possibly be any details about it.  This is also necessary
	// because the code below requires the parent block and the genesis
	// block doesn't have one.
	tip := b.bestChain.Tip()
	view := NewUtxoViewpoint(b)
	view.SetBestHash(&tip.hash)
	if tip.height == 0 {
		return view, nil
	}

	isTreasuryEnabled, err := b.isTreasuryAgendaActive(tip)
	if err != nil {
		return nil, err
	}

	// Disconnect the transactions in the regular tree of the tip block if the
	// caller requests it.  In order to avoid the overhead of repeated lookups,
	// only create a view with the changes once and cache it.
	if !includeRegularTxns {
		b.disapprovedViewLock.Lock()
		if b.disapprovedView == nil || *b.disapprovedView.BestHash() !=
			tip.hash {

			// Grab the current tip block.
			tipBlock, err := b.fetchMainChainBlockByNode(tip)
			if err != nil {
				b.disapprovedViewLock.Unlock()
				return nil, err
			}

			// Disconnect the transactions in the regular tree of the tip block.
			err = view.disconnectDisapprovedBlock(b.db, tipBlock,
				isTreasuryEnabled)
			if err != nil {
				b.disapprovedViewLock.Unlock()
				return nil, err
			}

			// Clone the view so the caller can safely mutate it.
			b.disapprovedView = view.clone()
		} else {
			// Clone the view so the caller can safely mutate it.
			view = b.disapprovedView.clone()
		}
		b.disapprovedViewLock.Unlock()
	}

	// Create a set of needed transactions based on those referenced by the
	// inputs of the passed transaction.  Also, add the passed transaction
	// itself as a way for the caller to detect duplicates that are not fully
	// spent.
	filteredSet := make(viewFilteredSet)
	filteredSet.add(view, tx.Hash())
	msgTx := tx.MsgTx()
	if !standalone.IsCoinBaseTx(msgTx, isTreasuryEnabled) {
		isVote := stake.IsSSGen(msgTx, isTreasuryEnabled)
		for txInIdx, txIn := range msgTx.TxIn {
			// Ignore stakebase since it has no input.
			if isVote && txInIdx == 0 {
				continue
			}

			filteredSet.add(view, &txIn.PreviousOutPoint.Hash)
		}
	}

	err = view.fetchUtxosMain(b.db, filteredSet)
	return view, err
}

// FetchUtxoEntry loads and returns the unspent transaction output entry for the
// passed hash from the point of view of the end of the main chain.
//
// NOTE: Requesting a hash for which there is no data will NOT return an error.
// Instead both the entry and the error will be nil.  This is done to allow
// pruning of fully spent transactions.  In practice this means the caller must
// check if the returned entry is nil before invoking methods on it.
//
// This function is safe for concurrent access however the returned entry (if
// any) is NOT.
func (b *BlockChain) FetchUtxoEntry(txHash *chainhash.Hash) (*UtxoEntry, error) {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	var entry *UtxoEntry
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		entry, err = dbFetchUtxoEntry(dbTx, txHash)
		return err
	})
	if err != nil {
		return nil, err
	}

	return entry, nil
}

// UtxoStats represents unspent output statistics on the current utxo set.
type UtxoStats struct {
	Utxos          int64
	Transactions   int64
	Size           int64
	Total          int64
	SerializedHash chainhash.Hash
}

// FetchUtxoStats returns statistics on the current utxo set.
func (b *BlockChain) FetchUtxoStats() (*UtxoStats, error) {
	var stats *UtxoStats
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		stats, err = dbFetchUtxoStats(dbTx)
		return err
	})
	if err != nil {
		return nil, err
	}

	return stats, nil
}
