// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"

	"github.com/decred/dcrd/blockchain/stake/v4"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/wire"
)

// UtxoViewpoint represents a view into the set of unspent transaction outputs
// from a specific point of view in the chain.  For example, it could be for
// the end of the main chain, some point in the history of the main chain, or
// down a side chain.
//
// The unspent outputs are needed by other transactions for things such as
// script validation and double spend prevention.
type UtxoViewpoint struct {
	entries  map[wire.OutPoint]*UtxoEntry
	bestHash chainhash.Hash
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

// LookupEntry returns information about a given transaction output according to
// the current state of the view.  It will return nil if the passed output does
// not exist in the view or is otherwise not available such as when it has been
// disconnected during a reorg.
func (view *UtxoViewpoint) LookupEntry(outpoint wire.OutPoint) *UtxoEntry {
	return view.entries[outpoint]
}

// addTxOut adds the specified output to the view if it is not provably
// unspendable.  When the view already has an entry for the output, it will be
// marked unspent.  All fields will be updated for existing entries since it's
// possible it has changed during a reorg.
func (view *UtxoViewpoint) addTxOut(outpoint wire.OutPoint, txOut *wire.TxOut,
	packedFlags utxoFlags, blockHeight int64, blockIndex uint32,
	ticketMinOuts *ticketMinimalOutputs) {

	// Don't add provably unspendable outputs.
	if txscript.IsUnspendable(txOut.Value, txOut.PkScript) {
		return
	}

	// Update existing entries.  All fields are updated because it's possible
	// (although extremely unlikely) that the existing entry is being replaced by
	// a different transaction with the same hash.  This is allowed so long as the
	// previous transaction is fully spent.
	entry := view.LookupEntry(outpoint)
	if entry == nil {
		entry = new(UtxoEntry)
		view.entries[outpoint] = entry
	}

	entry.amount = txOut.Value
	entry.pkScript = txOut.PkScript
	entry.blockHeight = uint32(blockHeight)
	entry.blockIndex = blockIndex
	entry.scriptVersion = txOut.Version
	entry.packedFlags = packedFlags
	entry.ticketMinOuts = ticketMinOuts
}

// AddTxOut adds the specified output of the passed transaction to the view if
// it exists and is not provably unspendable.  When the view already has an
// entry for the output, it will be marked unspent.  All fields will be updated
// for existing entries since it's possible it has changed during a reorg.
func (view *UtxoViewpoint) AddTxOut(tx *dcrutil.Tx, txOutIdx uint32,
	blockHeight int64, blockIndex uint32, isTreasuryEnabled bool) {

	// Can't add an output for an out of bounds index.
	msgTx := tx.MsgTx()
	if txOutIdx >= uint32(len(msgTx.TxOut)) {
		return
	}

	// Set encoded flags for the transaction.
	isCoinBase := standalone.IsCoinBaseTx(msgTx, isTreasuryEnabled)
	const spent = false
	hasExpiry := msgTx.Expiry != wire.NoExpiryValue
	const modified = true
	txType := stake.DetermineTxType(msgTx, isTreasuryEnabled)
	tree := wire.TxTreeRegular
	if txType != stake.TxTypeRegular {
		tree = wire.TxTreeStake
	}
	flags := encodeUtxoFlags(isCoinBase, spent, modified, hasExpiry, txType)

	// Update existing entries.  All fields are updated because it's possible
	// (although extremely unlikely) that the existing entry is being replaced by
	// a different transaction with the same hash.  This is allowed so long as the
	// previous transaction is fully spent.
	outpoint := wire.OutPoint{Hash: *tx.Hash(), Index: txOutIdx, Tree: tree}
	txOut := msgTx.TxOut[txOutIdx]
	var ticketMinOuts *ticketMinimalOutputs
	if isTicketSubmissionOutput(txType, txOutIdx) {
		ticketMinOuts = &ticketMinimalOutputs{
			data: make([]byte, serializeSizeForMinimalOutputs(tx)),
		}
		putTxToMinimalOutputs(ticketMinOuts.data, tx)
	}
	view.addTxOut(outpoint, txOut, flags, blockHeight, blockIndex, ticketMinOuts)
}

// AddTxOuts adds all outputs in the passed transaction which are not provably
// unspendable to the view.  When the view already has entries for any of the
// outputs, they are simply marked unspent.  All fields will be updated for
// existing entries since it's possible it has changed during a reorg.
func (view *UtxoViewpoint) AddTxOuts(tx *dcrutil.Tx, blockHeight int64, blockIndex uint32, isTreasuryEnabled bool) {
	msgTx := tx.MsgTx()

	// Set encoded flags for the transaction.
	isCoinBase := standalone.IsCoinBaseTx(msgTx, isTreasuryEnabled)
	const spent = false
	hasExpiry := msgTx.Expiry != wire.NoExpiryValue
	const modified = true
	txType := stake.DetermineTxType(msgTx, isTreasuryEnabled)
	tree := wire.TxTreeRegular
	if txType != stake.TxTypeRegular {
		tree = wire.TxTreeStake
	}
	flags := encodeUtxoFlags(isCoinBase, spent, modified, hasExpiry, txType)

	// Loop through all of the transaction outputs and add those which are not
	// provably unspendable.
	outpoint := wire.OutPoint{Hash: *tx.Hash(), Tree: tree}
	for txOutIdx, txOut := range msgTx.TxOut {
		// Update existing entries.  All fields are updated because it's possible
		// (although extremely unlikely) that the existing entry is being replaced
		// by a different transaction with the same hash.  This is allowed so long
		// as the previous transaction is fully spent.
		outpoint.Index = uint32(txOutIdx)
		var ticketMinOuts *ticketMinimalOutputs
		if isTicketSubmissionOutput(txType, uint32(txOutIdx)) {
			ticketMinOuts = &ticketMinimalOutputs{
				data: make([]byte, serializeSizeForMinimalOutputs(tx)),
			}
			putTxToMinimalOutputs(ticketMinOuts.data, tx)
		}
		view.addTxOut(outpoint, txOut, flags, blockHeight, blockIndex,
			ticketMinOuts)
	}
}

// PrevScript returns the script and script version associated with the provided
// previous outpoint along with a bool that indicates whether or not the
// requested entry exists.  This ensures the caller is able to distinguish
// between missing entries and empty v0 scripts.
func (view *UtxoViewpoint) PrevScript(prevOut *wire.OutPoint) (uint16, []byte, bool) {
	entry := view.LookupEntry(*prevOut)
	if entry == nil {
		return 0, nil, false
	}

	version := entry.ScriptVersion()
	pkScript := entry.PkScript()
	return version, pkScript, true
}

// PriorityInput returns the block height and amount associated with the
// provided previous outpoint along with a bool that indicates whether or not
// the requested entry exists.  This ensures the caller is able to distinguish
// missing entries from zero values.
func (view *UtxoViewpoint) PriorityInput(prevOut *wire.OutPoint) (int64, int64, bool) {
	entry := view.LookupEntry(*prevOut)
	if entry != nil && !entry.IsSpent() {
		return entry.BlockHeight(), entry.Amount(), true
	}

	return 0, 0, false
}

// connectTransaction updates the view by adding all new utxos created by the
// passed transaction and marking all utxos that the transaction spends as
// spent.  In addition, when the 'stxos' argument is not nil, it will be updated
// to append an entry for each spent txout.  An error will be returned if the
// view does not contain the required utxos.
func (view *UtxoViewpoint) connectTransaction(tx *dcrutil.Tx, blockHeight int64,
	blockIndex uint32, stxos *[]spentTxOut, isTreasuryEnabled bool) error {

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

	// Spend the referenced utxos by marking them spent in the view and, if a
	// slice was provided for the spent txout details, append an entry to it.
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

		// Ensure the referenced utxo exists in the view.  This should never happen
		// unless there is a bug is introduced in the code.
		entry := view.entries[txIn.PreviousOutPoint]
		if entry == nil {
			return AssertError(fmt.Sprintf("view missing input %v",
				txIn.PreviousOutPoint))
		}

		// Only create the stxo details if requested.
		if stxos != nil {
			// Populate the stxo details using the utxo entry.
			var stxo = spentTxOut{
				amount:        entry.Amount(),
				pkScript:      entry.PkScript(),
				ticketMinOuts: entry.ticketMinOuts,
				blockHeight:   uint32(entry.BlockHeight()),
				blockIndex:    entry.BlockIndex(),
				scriptVersion: entry.ScriptVersion(),
				packedFlags: encodeFlags(entry.IsCoinBase(), entry.HasExpiry(),
					entry.TransactionType()),
			}

			*stxos = append(*stxos, stxo)
		}

		// Mark the entry as spent.  This is not done until after the relevant
		// details have been accessed since spending it might clear the fields from
		// memory in the future.
		entry.Spend()
	}

	// Add the transaction's outputs as available utxos.
	view.AddTxOuts(tx, blockHeight, blockIndex, isTreasuryEnabled)

	return nil
}

// disconnectTransactions updates the view by removing all utxos created by the
// transactions in either the regular or stake tree of the block, depending on
// the flag, and unspending all of the txos spent by those same transactions by
// using the provided spent txo information.
func (view *UtxoViewpoint) disconnectTransactions(block *dcrutil.Block, stxos []spentTxOut, stakeTree bool, isTreasuryEnabled bool) error {
	// Choose which transaction tree to use and the appropriate offset into the
	// spent transaction outputs that corresponds to them depending on the flag.
	// Transactions in the stake tree are spent before transactions in the regular
	// tree, thus skip all of the outputs spent by the regular tree when
	// disconnecting stake transactions.
	stxoIdx := len(stxos) - 1
	transactions := block.Transactions()
	if stakeTree {
		stxoIdx = len(stxos) - countSpentRegularOutputs(block) - 1
		transactions = block.STransactions()
	}

	for txIdx := len(transactions) - 1; txIdx > -1; txIdx-- {
		tx := transactions[txIdx]
		txHash := tx.Hash()
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

		tree := wire.TxTreeRegular
		if txType != stake.TxTypeRegular {
			tree = wire.TxTreeStake
		}

		isCoinBase := !stakeTree && txIdx == 0
		hasExpiry := msgTx.Expiry != wire.NoExpiryValue

		// It is instructive to note that there is no practical difference between
		// a utxo that does not exist and one that has been spent with a pruned
		// utxo set. So, even though the outputs here technically no longer exist,
		// any missing entries are added to the view and marked spent because the
		// code relies on their existence in the view in order to signal
		// modifications have happened.
		outpoint := wire.OutPoint{Hash: *txHash, Tree: tree}
		for txOutIdx, txOut := range msgTx.TxOut {
			// Don't add provably unspendable outputs.
			if txscript.IsUnspendable(txOut.Value, txOut.PkScript) {
				continue
			}

			outpoint.Index = uint32(txOutIdx)
			entry := view.entries[outpoint]
			if entry == nil {
				const spent = false
				const modified = true
				entry = &UtxoEntry{
					amount:        txOut.Value,
					pkScript:      txOut.PkScript,
					blockHeight:   uint32(block.Height()),
					blockIndex:    uint32(txIdx),
					scriptVersion: txOut.Version,
					packedFlags: encodeUtxoFlags(isCoinBase, spent, modified, hasExpiry,
						txType),
				}

				if isTicketSubmissionOutput(txType, uint32(txOutIdx)) {
					entry.ticketMinOuts = &ticketMinimalOutputs{
						data: make([]byte, serializeSizeForMinimalOutputs(tx)),
					}
					putTxToMinimalOutputs(entry.ticketMinOuts.data, tx)
				}

				view.entries[outpoint] = entry
			}

			entry.Spend()
		}

		// Loop backwards through all of the transaction inputs (except for the
		// coinbase and treasurybase which have no inputs) and unspend the
		// referenced txos.  This is necessary to match the order of the spent txout
		// entries.
		if isCoinBase || isTreasuryBase || isTSpend {
			continue
		}
		for txInIdx := len(msgTx.TxIn) - 1; txInIdx > -1; txInIdx-- {
			// Ignore stakebase since it has no input.
			if isVote && txInIdx == 0 {
				continue
			}

			// Ensure the spent txout index is decremented to stay in sync with the
			// transaction input.
			stxo := &stxos[stxoIdx]
			stxoIdx--

			// When there is not already an entry for the referenced output in the
			// view, it means it was previously spent, so create a new utxo entry in
			// order to resurrect it.
			txIn := msgTx.TxIn[txInIdx]
			entry := view.entries[txIn.PreviousOutPoint]
			if entry == nil {
				const spent = false
				const modified = true
				entry = &UtxoEntry{
					amount:        txIn.ValueIn,
					pkScript:      stxo.pkScript,
					ticketMinOuts: stxo.ticketMinOuts,
					blockHeight:   stxo.blockHeight,
					blockIndex:    stxo.blockIndex,
					scriptVersion: stxo.scriptVersion,
					packedFlags: encodeUtxoFlags(stxo.IsCoinBase(), spent, modified,
						stxo.HasExpiry(), stxo.TransactionType()),
				}

				view.entries[txIn.PreviousOutPoint] = entry
			}

			// Mark the existing referenced transaction output as unspent and
			// modified.
			entry.packedFlags &^= utxoFlagSpent
			entry.packedFlags |= utxoFlagModified
		}
	}

	return nil
}

// RemoveEntry removes the given transaction output from the current state of
// the view.  It will have no effect if the passed output does not exist in the
// view.
func (view *UtxoViewpoint) RemoveEntry(outpoint wire.OutPoint) {
	delete(view.entries, outpoint)
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
func (view *UtxoViewpoint) Entries() map[wire.OutPoint]*UtxoEntry {
	return view.entries
}

// commit prunes all entries marked modified that are now spent and marks all
// entries as unmodified.
func (view *UtxoViewpoint) commit() {
	for outpoint, entry := range view.entries {
		if entry == nil || (entry.isModified() && entry.IsSpent()) {
			delete(view.entries, outpoint)
			continue
		}

		entry.packedFlags &^= utxoFlagModified
	}
}

// viewFilteredSet represents a set of utxos to fetch from the database that are
// not already in a view.
type viewFilteredSet map[wire.OutPoint]struct{}

// add conditionally adds the provided outpoint to the set if it does not
// already exist in the provided view.
func (set viewFilteredSet) add(view *UtxoViewpoint, outpoint *wire.OutPoint) {
	if _, ok := view.entries[*outpoint]; !ok {
		set[*outpoint] = struct{}{}
	}
}

// fetchUtxosMain fetches unspent transaction output data about the provided
// set of outpoints from the point of view of the main chain tip at the time of
// the call.
//
// Upon completion of this function, the view will contain an entry for each
// requested outpoint.  Spent outputs, or those which otherwise don't exist,
// will result in a nil entry in the view.
func (view *UtxoViewpoint) fetchUtxosMain(db database.DB, filteredSet viewFilteredSet) error {
	// Nothing to do if there are no requested outputs.
	if len(filteredSet) == 0 {
		return nil
	}

	// Load the requested set of unspent transaction outputs from the point
	// of view of the end of the main chain.
	//
	// NOTE: Missing entries are not considered an error here and instead
	// will result in nil entries in the view.  This is intentionally done
	// so other code can use the presence of an entry in the view as a way
	// to unnecessarily avoid attempting to reload it from the database.
	return db.View(func(dbTx database.Tx) error {
		for outpoint := range filteredSet {
			entry, err := dbFetchUtxoEntry(dbTx, outpoint)
			if err != nil {
				return err
			}

			view.entries[outpoint] = entry
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
			filteredSet.add(view, &txIn.PreviousOutPoint)
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

// fetchInputUtxos loads the unspent transaction outputs for the inputs
// referenced by the transactions in both the regular and stake trees of the
// given block into the view from the database as needed.  In the case of the
// regular tree, referenced entries that are earlier in the regular tree of the
// block are added to the view.  In all cases, entries that are already in the
// view are not modified.
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
			filteredSet.add(view, &txIn.PreviousOutPoint)
		}
	}

	// Request the input utxos from the database.
	return view.fetchUtxosMain(db, filteredSet)
}

// clone returns a deep copy of the view.
func (view *UtxoViewpoint) clone() *UtxoViewpoint {
	clonedView := &UtxoViewpoint{
		entries:  make(map[wire.OutPoint]*UtxoEntry),
		bestHash: view.bestHash,
	}

	for outpoint, entry := range view.entries {
		clonedView.entries[outpoint] = entry.Clone()
	}

	return clonedView
}

// NewUtxoViewpoint returns a new empty unspent transaction output view.
func NewUtxoViewpoint() *UtxoViewpoint {
	return &UtxoViewpoint{
		entries: make(map[wire.OutPoint]*UtxoEntry),
	}
}

// FetchUtxoView loads unspent transaction outputs for the inputs referenced by
// the passed transaction from the point of view of the main chain tip while
// taking into account whether or not the transactions in the regular tree of
// the current tip block should be included or not depending on the provided
// flag.  It also attempts to fetch the utxos for the outputs of the transaction
// itself so the returned view can be examined for duplicate transactions.
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
	view := NewUtxoViewpoint()
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
	// inputs of the passed transaction.  Also, add the outputs of the transaction
	// as a way for the caller to detect duplicates that are not fully spent.
	filteredSet := make(viewFilteredSet)
	msgTx := tx.MsgTx()
	txType := stake.DetermineTxType(msgTx, isTreasuryEnabled)
	tree := wire.TxTreeRegular
	if txType != stake.TxTypeRegular {
		tree = wire.TxTreeStake
	}
	outpoint := wire.OutPoint{Hash: *tx.Hash(), Tree: tree}
	for txOutIdx := range msgTx.TxOut {
		outpoint.Index = uint32(txOutIdx)
		filteredSet.add(view, &outpoint)
	}
	if !standalone.IsCoinBaseTx(msgTx, isTreasuryEnabled) {
		isVote := stake.IsSSGen(msgTx, isTreasuryEnabled)
		for txInIdx, txIn := range msgTx.TxIn {
			// Ignore stakebase since it has no input.
			if isVote && txInIdx == 0 {
				continue
			}

			filteredSet.add(view, &txIn.PreviousOutPoint)
		}
	}

	err = view.fetchUtxosMain(b.db, filteredSet)
	return view, err
}

// FetchUtxoEntry loads and returns the requested unspent transaction output
// from the point of view of the the main chain tip.
//
// NOTE: Requesting an output for which there is no data will NOT return an
// error.  Instead both the entry and the error will be nil.  This is done to
// allow pruning of spent transaction outputs.  In practice this means the
// caller must check if the returned entry is nil before invoking methods on it.
//
// This function is safe for concurrent access however the returned entry (if
// any) is NOT.
func (b *BlockChain) FetchUtxoEntry(outpoint wire.OutPoint) (*UtxoEntry, error) {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	var entry *UtxoEntry
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		entry, err = dbFetchUtxoEntry(dbTx, outpoint)
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
