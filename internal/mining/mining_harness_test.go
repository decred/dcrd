// Copyright (c) 2020-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/internal/blockchain"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/sign"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/txscript/v4/stdscript"
	"github.com/decred/dcrd/wire"
)

const (
	// singleInputTicketSize is the typical size of a normal P2PKH ticket in bytes
	// when the ticket has one input, rounded up.
	singleInputTicketSize int64 = 300
)

// spendableOutput is a convenience type that houses a particular utxo and the
// amount associated with it.
type spendableOutput struct {
	outPoint wire.OutPoint
	amount   dcrutil.Amount
}

// txOutToSpendableOut returns a spendable output given a transaction and index
// of the output to use.  This is useful as a convenience when creating test
// transactions.
func txOutToSpendableOut(tx *dcrutil.Tx, outputNum uint32, tree int8) spendableOutput {
	return spendableOutput{
		outPoint: wire.OutPoint{Hash: *tx.Hash(), Index: outputNum, Tree: tree},
		amount:   dcrutil.Amount(tx.MsgTx().TxOut[outputNum].Value),
	}
}

// fakeChain is used by the mining harness to provide generated test utxos and
// a faked chain state.  It also allows for mocking the return values of the
// chain related functions that mining depends on.
type fakeChain struct {
	blocks                             map[chainhash.Hash]*dcrutil.Block
	bestState                          blockchain.BestState
	calcNextRequiredDifficulty         uint32
	calcNextRequiredDifficultyErr      error
	calcStakeVersionByHash             uint32
	calcStakeVersionByHashErr          error
	checkConnectBlockTemplateErr       error
	checkTicketExhaustionErr           error
	checkTSpendHasVotesErr             error
	fetchUtxoEntryErr                  error
	fetchUtxoViewErr                   error
	fetchUtxoViewParentTemplateErr     error
	forceHeadReorganizationErr         error
	isHeaderCommitmentsAgendaActive    bool
	isHeaderCommitmentsAgendaActiveErr error
	isTreasuryAgendaActive             bool
	isTreasuryAgendaActiveErr          error
	isAutoRevocationsAgendaActive      bool
	isAutoRevocationsAgendaActiveErr   error
	isSubsidySplitAgendaActive         bool
	isSubsidySplitAgendaActiveErr      error
	isSubsidySplitR2AgendaActive       bool
	isSubsidySplitR2AgendaActiveErr    error
	maxTreasuryExpenditure             int64
	maxTreasuryExpenditureErr          error
	parentUtxos                        *blockchain.UtxoViewpoint
	tipGeneration                      []chainhash.Hash
	utxos                              *blockchain.UtxoViewpoint
}

// determineSubsidySplitVariant returns the subsidy split variant to use based
// on the agendas that are active on the fake chain instance.
func (c *fakeChain) determineSubsidySplitVariant() standalone.SubsidySplitVariant {
	switch {
	case c.isSubsidySplitR2AgendaActive:
		return standalone.SSVDCP0012
	case c.isSubsidySplitAgendaActive:
		return standalone.SSVDCP0010
	}
	return standalone.SSVOriginal
}

// AddBlock adds a block that will be available to the BlockByHash function of
// the fake chain instance.
func (c *fakeChain) AddBlock(block *dcrutil.Block) {
	c.blocks[*block.Hash()] = block
}

// BestSnapshot returns the current best state associated with the fake chain
// instance.
func (c *fakeChain) BestSnapshot() *blockchain.BestState {
	return &c.bestState
}

// BlockByHash returns the block with the given hash from the fake chain
// instance.  Blocks can be added to the instance with the AddBlock function.
func (c *fakeChain) BlockByHash(hash *chainhash.Hash) (*dcrutil.Block, error) {
	block, ok := c.blocks[*hash]
	if !ok {
		return nil, fmt.Errorf("unable to find block %v in fake chain", hash)
	}
	return block, nil
}

// CalcNextRequiredDifficulty returns a mocked required difficulty for the block
// AFTER the provided block hash.
func (c *fakeChain) CalcNextRequiredDifficulty(hash *chainhash.Hash, timestamp time.Time) (uint32, error) {
	return c.calcNextRequiredDifficulty, c.calcNextRequiredDifficultyErr
}

// CalcStakeVersionByHash returns a mocked expected stake version for the block
// AFTER the provided block hash.
func (c *fakeChain) CalcStakeVersionByHash(hash *chainhash.Hash) (uint32, error) {
	return c.calcStakeVersionByHash, c.calcStakeVersionByHashErr
}

// CheckConnectBlockTemplate mocks the function that is used to validate that
// connecting the passed block to either the tip of the main chain or its parent
// does not violate any consensus rules, aside from the proof of work
// requirement.
func (c *fakeChain) CheckConnectBlockTemplate(block *dcrutil.Block) error {
	return c.checkConnectBlockTemplateErr
}

// CheckTicketExhaustion mocks the function that is used to ensure that
// extending the block associated with the provided hash with a block that
// contains the specified number of ticket purchases will not result in a chain
// that is unrecoverable due to inevitable ticket exhaustion.
func (c *fakeChain) CheckTicketExhaustion(hash *chainhash.Hash, ticketPurchases uint8) error {
	return c.checkTicketExhaustionErr
}

// CheckTSpendHasVotes mocks the function that is used to check whether the
// given tspend has enough votes to be included in a block AFTER the specified
// prevHash block.
func (c *fakeChain) CheckTSpendHasVotes(prevHash chainhash.Hash, tspend *dcrutil.Tx) error {
	return c.checkTSpendHasVotesErr
}

// FetchUtxoEntry returns the requested unspent transaction output from the
// mocked utxos.
func (c *fakeChain) FetchUtxoEntry(outpoint wire.OutPoint) (*blockchain.UtxoEntry, error) {
	return c.utxos.LookupEntry(outpoint), c.fetchUtxoEntryErr
}

// FetchUtxoView loads unspent transaction outputs for the inputs referenced by
// the passed transaction from the point of view of the main chain tip while
// taking into account whether or not the transactions in the regular tree of
// the current tip block should be included or not depending on the provided
// flag.  It also attempts to fetch the utxos for the outputs of the transaction
// so the returned view can be examined for duplicate transactions.
func (c *fakeChain) FetchUtxoView(tx *dcrutil.Tx, treeValid bool) (*blockchain.UtxoViewpoint, error) {
	// All entries are cloned to ensure modifications to the returned view
	// do not affect the fake chain's view.

	// Add entries for the outputs of the tx to the new view.
	msgTx := tx.MsgTx()
	viewpoint := blockchain.NewUtxoViewpoint(nil)
	prevOut := wire.OutPoint{Hash: *tx.Hash(), Tree: tx.Tree()}
	for txOutIdx := range msgTx.TxOut {
		prevOut.Index = uint32(txOutIdx)
		entry := c.utxos.LookupEntry(prevOut)
		viewpoint.Entries()[prevOut] = entry.Clone()
	}

	// Add entries for all of the inputs to the tx to the new view.
	for _, txIn := range tx.MsgTx().TxIn {
		entry := c.utxos.LookupEntry(txIn.PreviousOutPoint)
		viewpoint.Entries()[txIn.PreviousOutPoint] = entry.Clone()
	}

	return viewpoint, c.fetchUtxoViewErr
}

// FetchUtxoViewParentTemplate returns mocked unspent transaction output
// information from the point of view of just having connected the given block.
func (c *fakeChain) FetchUtxoViewParentTemplate(block *wire.MsgBlock) (*blockchain.UtxoViewpoint, error) {
	return c.parentUtxos, c.fetchUtxoViewParentTemplateErr
}

// ForceHeadReorganization mocks the function that is used to force a
// reorganization of the block chain to the block hash requested.
func (c *fakeChain) ForceHeadReorganization(formerBest chainhash.Hash, newBest chainhash.Hash) error {
	return c.forceHeadReorganizationErr
}

// HeaderByHash returns the header for the block with the given hash from the
// fake chain instance.  Blocks can be added to the instance with the AddBlock
// function.
func (c *fakeChain) HeaderByHash(hash *chainhash.Hash) (wire.BlockHeader, error) {
	block, ok := c.blocks[*hash]
	if !ok {
		return wire.BlockHeader{}, fmt.Errorf("unable to find block %v in fake "+
			"chain", hash)
	}
	return block.MsgBlock().Header, nil
}

// IsHeaderCommitmentsAgendaActive returns a mocked bool representing whether
// the header commitments agenda is active or not for the block AFTER the given
// block.
func (c *fakeChain) IsHeaderCommitmentsAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	return c.isHeaderCommitmentsAgendaActive, c.isHeaderCommitmentsAgendaActiveErr
}

// IsTreasuryAgendaActive returns a mocked bool representing whether the
// treasury agenda is active or not for the block AFTER the given block.
func (c *fakeChain) IsTreasuryAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	return c.isTreasuryAgendaActive, c.isTreasuryAgendaActiveErr
}

// IsAutoRevocationsAgendaActive returns a mocked bool representing whether the
// automatic ticket revocations agenda is active or not for the block AFTER the
// given block.
func (c *fakeChain) IsAutoRevocationsAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	return c.isAutoRevocationsAgendaActive, c.isAutoRevocationsAgendaActiveErr
}

// IsSubsidySplitAgendaActive returns a mocked bool representing whether the
// modified subsidy split agenda is active or not for the block AFTER the given
// block.
func (c *fakeChain) IsSubsidySplitAgendaActive(prevHash *chainhash.Hash) (bool, error) {
	return c.isSubsidySplitAgendaActive, c.isSubsidySplitAgendaActiveErr
}

// IsSubsidySplitR2AgendaActive returns a mocked bool representing whether the
// modified subsidy split round 2 agenda is active or not for the block AFTER
// the given block.
func (c *fakeChain) IsSubsidySplitR2AgendaActive(prevHash *chainhash.Hash) (bool, error) {
	return c.isSubsidySplitR2AgendaActive, c.isSubsidySplitR2AgendaActiveErr
}

// MaxTreasuryExpenditure returns a mocked maximum amount of funds that can be
// spent from the treasury by a set of TSpends for a block that extends the
// given block hash.
func (c *fakeChain) MaxTreasuryExpenditure(preTVIBlock *chainhash.Hash) (int64, error) {
	return c.maxTreasuryExpenditure, c.maxTreasuryExpenditureErr
}

// NewUtxoViewpoint returns a new empty unspent transaction output view.
func (c *fakeChain) NewUtxoViewpoint() *blockchain.UtxoViewpoint {
	return blockchain.NewUtxoViewpoint(nil)
}

// TipGeneration returns a mocked entire generation of blocks stemming from the
// parent of the current tip.
func (c *fakeChain) TipGeneration() []chainhash.Hash {
	return c.tipGeneration
}

// fakeTxSource provides a mocked source of transactions to consider for
// inclusion in new blocks and satisfies the TxSource interface.
//
// It handles the adding and removing of transactions, including adding
// dependent transactions out of order.  It only performs minimal validations
// when transactions are added and does NOT perform all of the validations that
// the real mempool does.
type fakeTxSource struct {
	chain           *fakeChain
	chainParams     *chaincfg.Params
	subsidyCache    *standalone.SubsidyCache
	pool            map[chainhash.Hash]*TxDesc
	outpoints       map[wire.OutPoint]*dcrutil.Tx
	orphans         map[chainhash.Hash]*dcrutil.Tx
	orphansByPrev   map[wire.OutPoint]map[chainhash.Hash]*dcrutil.Tx
	staged          map[chainhash.Hash]*dcrutil.Tx
	stagedOutpoints map[wire.OutPoint]*dcrutil.Tx
	votes           map[chainhash.Hash][]VoteDesc
	tspends         map[chainhash.Hash]*dcrutil.Tx
	miningView      *TxMiningView
	lastUpdated     atomic.Int64
}

// isTransactionInTxSource returns whether or not the passed transaction exists
// in the main pool.
func (p *fakeTxSource) isTransactionInTxSource(hash *chainhash.Hash) bool {
	_, exists := p.pool[*hash]
	return exists
}

// isOrphanInTxSource returns whether or not the passed transaction exists in
// the orphan pool.
func (p *fakeTxSource) isOrphanInTxSource(hash *chainhash.Hash) bool {
	_, exists := p.orphans[*hash]
	return exists
}

// isTransactionStaged determines if the transaction exists in the stage pool.
func (p *fakeTxSource) isTransactionStaged(hash *chainhash.Hash) bool {
	_, exists := p.staged[*hash]
	return exists
}

// LastUpdated returns the last time a transaction was added to or removed from
// the fake tx source.
func (p *fakeTxSource) LastUpdated() time.Time {
	return time.Unix(p.lastUpdated.Load(), 0)
}

// HaveTransaction returns whether or not the passed transaction hash exists in
// the fake tx source.
func (p *fakeTxSource) HaveTransaction(hash *chainhash.Hash) bool {
	return p.isTransactionInTxSource(hash) || p.isOrphanInTxSource(hash) ||
		p.isTransactionStaged(hash)
}

// HaveAllTransactions returns whether or not all of the passed transaction
// hashes exist in the fake tx source.
func (p *fakeTxSource) HaveAllTransactions(hashes []chainhash.Hash) bool {
	haveAllTx := true
	for _, h := range hashes {
		if _, exists := p.pool[h]; !exists {
			haveAllTx = false
			break
		}
	}

	return haveAllTx
}

// VoteHashesForBlock returns the hashes for all votes on the provided block
// hash that are currently available in the fake tx source.
func (p *fakeTxSource) VoteHashesForBlock(hash *chainhash.Hash) []chainhash.Hash {
	vts, exists := p.votes[*hash]

	// Lookup the vote metadata for the block.
	if !exists || len(vts) == 0 {
		return nil
	}

	// Copy the vote hashes from the vote metadata.
	hashes := make([]chainhash.Hash, 0, len(vts))
	for _, vt := range vts {
		hashes = append(hashes, vt.VoteHash)
	}

	return hashes
}

// VotesForBlocks returns a slice of vote descriptors for all votes on the
// provided block hashes that are currently available in the fake tx source.
func (p *fakeTxSource) VotesForBlocks(hashes []chainhash.Hash) [][]VoteDesc {
	result := make([][]VoteDesc, 0, len(hashes))

	for _, hash := range hashes {
		votes := p.votes[hash]
		result = append(result, votes)
	}

	return result
}

// IsRegTxTreeKnownDisapproved returns whether or not the regular transaction
// tree of the block represented by the provided hash is known to be disapproved
// according to the votes currently in the fake tx source.
func (p *fakeTxSource) IsRegTxTreeKnownDisapproved(hash *chainhash.Hash) bool {
	vts := p.votes[*hash]

	// There are not possibly enough votes to tell if the regular transaction
	// tree is approved or not, so assume it's valid.
	if len(vts) <= int(p.chainParams.TicketsPerBlock/2) {
		return false
	}

	// Otherwise, tally the votes and determine if it's approved or not.
	var yes, no int
	for _, vote := range vts {
		if vote.ApprovesParent {
			yes++
		} else {
			no++
		}
	}

	return yes <= no
}

// CountTotalSigOps returns the total number of signature operations for the
// given transaction.
func (p *fakeTxSource) CountTotalSigOps(tx *dcrutil.Tx, txType stake.TxType) (int, error) {
	isVote := txType == stake.TxTypeSSGen
	isStakeBase := txType == stake.TxTypeSSGen
	utxoView, err := p.fetchInputUtxos(tx, p.chain.isTreasuryAgendaActive)
	if err != nil {
		return 0, err
	}

	sigOps := blockchain.CountSigOps(tx, false, isVote,
		p.chain.isTreasuryAgendaActive)
	p2shSigOps, err := blockchain.CountP2SHSigOps(tx, false, isStakeBase,
		utxoView, p.chain.isTreasuryAgendaActive)
	if err != nil {
		return 0, err
	}

	return sigOps + p2shSigOps, nil
}

// fetchRedeemers returns all transactions that reference an outpoint for the
// provided regular transaction `tx`.  Returns nil if a non-regular transaction
// is provided.
func (p *fakeTxSource) fetchRedeemers(outpoints map[wire.OutPoint]*dcrutil.Tx,
	tx *dcrutil.Tx) []*dcrutil.Tx {

	txType := stake.DetermineTxType(tx.MsgTx())
	if txType != stake.TxTypeRegular {
		return nil
	}

	tree := wire.TxTreeRegular
	seen := map[chainhash.Hash]struct{}{}
	redeemers := make([]*dcrutil.Tx, 0)
	outpoint := wire.OutPoint{Hash: *tx.Hash(), Tree: tree}
	for i := uint32(0); i < uint32(len(tx.MsgTx().TxOut)); i++ {
		outpoint.Index = i
		txRedeemer, exists := outpoints[outpoint]
		if !exists {
			continue
		}
		if _, exists := seen[*txRedeemer.Hash()]; exists {
			continue
		}

		seen[*txRedeemer.Hash()] = struct{}{}
		redeemers = append(redeemers, txRedeemer)
	}

	return redeemers
}

// addOrphan adds the passed orphan transaction to the orphan pool.
func (p *fakeTxSource) addOrphan(tx *dcrutil.Tx) {
	p.orphans[*tx.Hash()] = tx
	for _, txIn := range tx.MsgTx().TxIn {
		if _, exists := p.orphansByPrev[txIn.PreviousOutPoint]; !exists {
			p.orphansByPrev[txIn.PreviousOutPoint] =
				make(map[chainhash.Hash]*dcrutil.Tx)
		}
		p.orphansByPrev[txIn.PreviousOutPoint][*tx.Hash()] = tx
	}
}

// removeOrphan removes the passed orphan transaction from the orphan pool.
func (p *fakeTxSource) removeOrphan(tx *dcrutil.Tx, removeRedeemers bool) {
	// Nothing to do if the passed tx does not exist in the orphan pool.
	txHash := tx.Hash()
	tx, exists := p.orphans[*txHash]
	if !exists {
		return
	}

	// Remove the reference from the previous orphan index.
	for _, txIn := range tx.MsgTx().TxIn {
		orphans, exists := p.orphansByPrev[txIn.PreviousOutPoint]
		if exists {
			delete(orphans, *txHash)

			// Remove the map entry altogether if there are no longer any orphans which
			// depend on it.
			if len(orphans) == 0 {
				delete(p.orphansByPrev, txIn.PreviousOutPoint)
			}
		}
	}

	// Remove any orphans that redeem outputs from this one if requested.
	if removeRedeemers {
		prevOut := wire.OutPoint{Hash: *txHash, Tree: tx.Tree()}
		for txOutIdx := range tx.MsgTx().TxOut {
			prevOut.Index = uint32(txOutIdx)
			for _, orphan := range p.orphansByPrev[prevOut] {
				p.removeOrphan(orphan, true)
			}
		}
	}

	// Remove the transaction from the orphan pool.
	delete(p.orphans, *txHash)
}

// removeOrphanDoubleSpends removes all orphans which spend outputs spent by the
// passed transaction from the orphan pool.  Removing those orphans then leads
// to removing all orphans which rely on them, recursively.  This is necessary
// when a transaction is added to the main pool because it may spend outputs
// that orphans also spend.
func (p *fakeTxSource) removeOrphanDoubleSpends(tx *dcrutil.Tx) {
	msgTx := tx.MsgTx()
	for _, txIn := range msgTx.TxIn {
		for _, orphan := range p.orphansByPrev[txIn.PreviousOutPoint] {
			p.removeOrphan(orphan, true)
		}
	}
}

// addTransaction adds the passed transaction to the fake tx source.
func (p *fakeTxSource) addTransaction(tx *dcrutil.Tx, txType stake.TxType, height int64, fee int64, totalSigOps int) {
	// Add the transaction to the pool and mark the referenced outpoints
	// as spent by the pool.
	txDesc := TxDesc{
		Tx:          tx,
		Type:        txType,
		Added:       time.Now(),
		Height:      height,
		Fee:         fee,
		TotalSigOps: totalSigOps,
		TxSize:      int64(tx.MsgTx().SerializeSize()),
	}

	p.pool[*tx.Hash()] = &txDesc
	p.miningView.AddTransaction(&txDesc, p.findTx)

	msgTx := tx.MsgTx()
	for _, txIn := range msgTx.TxIn {
		p.outpoints[txIn.PreviousOutPoint] = tx
	}
	p.lastUpdated.Store(time.Now().Unix())
}

// insertVote inserts a vote into the map of block votes.
func (p *fakeTxSource) insertVote(ssgen *dcrutil.Tx) {
	// Get the block it is voting on; here we're agnostic of height.
	msgTx := ssgen.MsgTx()
	blockHash, _ := stake.SSGenBlockVotedOn(msgTx)

	// If there are currently no votes for this block,
	// start a new buffered slice and store it.
	vts, exists := p.votes[blockHash]
	if !exists {
		vts = make([]VoteDesc, 0, p.chainParams.TicketsPerBlock)
	}

	// Nothing to do if a vote for the ticket is already known.
	ticketHash := &msgTx.TxIn[1].PreviousOutPoint.Hash
	for _, vt := range vts {
		if vt.TicketHash.IsEqual(ticketHash) {
			return
		}
	}

	voteHash := ssgen.Hash()
	voteBits := stake.SSGenVoteBits(msgTx)
	vote := dcrutil.IsFlagSet16(voteBits, dcrutil.BlockValid)
	voteTx := VoteDesc{
		VoteHash:       *voteHash,
		TicketHash:     *ticketHash,
		ApprovesParent: vote,
	}

	// Append the new vote.
	p.votes[blockHash] = append(vts, voteTx)
}

// stageTransaction creates an entry for the provided transaction in the stage
// pool.
func (p *fakeTxSource) stageTransaction(tx *dcrutil.Tx) {
	p.staged[*tx.Hash()] = tx
	for _, txIn := range tx.MsgTx().TxIn {
		p.stagedOutpoints[txIn.PreviousOutPoint] = tx
	}
}

// removeStagedTransaction removes the provided transaction from the stage pool.
func (p *fakeTxSource) removeStagedTransaction(stagedTx *dcrutil.Tx) {
	delete(p.staged, *stagedTx.Hash())
	for _, txIn := range stagedTx.MsgTx().TxIn {
		delete(p.stagedOutpoints, txIn.PreviousOutPoint)
	}
}

// removeTransaction removes the passed transaction from the fake tx source.
func (p *fakeTxSource) RemoveTransaction(tx *dcrutil.Tx, removeRedeemers,
	isTreasuryEnabled, isAutoRevocationsEnabled bool) {

	txHash := tx.Hash()
	if removeRedeemers {
		// Remove any transactions which rely on this one.
		txType := stake.DetermineTxType(tx.MsgTx())
		tree := wire.TxTreeRegular
		if txType != stake.TxTypeRegular {
			tree = wire.TxTreeStake
		}

		prevOut := wire.OutPoint{Hash: *txHash, Tree: tree}
		for i := uint32(0); i < uint32(len(tx.MsgTx().TxOut)); i++ {
			prevOut.Index = i
			if txRedeemer, exists := p.outpoints[prevOut]; exists {
				p.RemoveTransaction(txRedeemer, true,
					isTreasuryEnabled, isAutoRevocationsEnabled)
				continue
			}
			if txRedeemer, exists := p.stagedOutpoints[prevOut]; exists {
				log.Tracef("Removing staged transaction %v", prevOut.Hash)
				p.removeStagedTransaction(txRedeemer)
			}
		}
	}

	// Remove the transaction if needed.
	if txDesc, exists := p.pool[*txHash]; exists {
		log.Tracef("Removing transaction %v", txHash)

		// Mark the referenced outpoints as unspent by the pool.
		for _, txIn := range txDesc.Tx.MsgTx().TxIn {
			delete(p.outpoints, txIn.PreviousOutPoint)
		}

		// Stop tracking this transaction in the mining view.
		// If redeeming transactions are going to be removed from the graph, then do
		// not update their stats.
		updateDescendantStats := !removeRedeemers
		p.miningView.RemoveTransaction(tx.Hash(), updateDescendantStats)

		delete(p.pool, *txHash)

		p.lastUpdated.Store(time.Now().Unix())

		// Stop tracking if it's a tspend.
		delete(p.tspends, *txHash)
	}
}

// hasPoolInput returns true if the provided transaction has an input in the
// main pool.
func (p *fakeTxSource) hasPoolInput(tx *dcrutil.Tx) bool {
	for _, txIn := range tx.MsgTx().TxIn {
		if p.isTransactionInTxSource(&txIn.PreviousOutPoint.Hash) {
			return true
		}
	}

	return false
}

// MaybeAcceptDependents determines if there are any staged dependents of the
// passed transaction and potentially accepts them to the main pool.
//
// It returns a slice of transactions added to the pool.  A nil slice means no
// transactions were moved from the stage pool to the main pool.
func (p *fakeTxSource) MaybeAcceptDependents(tx *dcrutil.Tx) []*dcrutil.Tx {
	var acceptedTxns []*dcrutil.Tx
	for _, redeemer := range p.fetchRedeemers(p.stagedOutpoints, tx) {
		redeemerTxType := stake.DetermineTxType(redeemer.MsgTx())
		if redeemerTxType == stake.TxTypeSStx {
			// Skip tickets with inputs in the main pool.
			if p.hasPoolInput(redeemer) {
				continue
			}

			// Remove the dependent transaction and attempt to add it to the main pool or
			// back to the stage pool.  In the event of an error, the transaction will be
			// discarded.
			p.removeStagedTransaction(redeemer)
			_, err := p.maybeAcceptTransaction(redeemer, true)
			if err != nil {
				log.Debugf("Failed to add previously staged "+
					"ticket %v to pool. %v", *redeemer.Hash(), err)
			}

			if p.isTransactionInTxSource(redeemer.Hash()) {
				acceptedTxns = append(acceptedTxns, redeemer)
			}
		}
	}

	return acceptedTxns
}

// maybeAcceptTransaction handles inserting new transactions into the fake tx
// source
func (p *fakeTxSource) maybeAcceptTransaction(tx *dcrutil.Tx, isNew bool) ([]*chainhash.Hash, error) {
	msgTx := tx.MsgTx()
	txHash := tx.Hash()
	best := p.chain.BestSnapshot()
	height := best.Height
	nextHeight := height + 1
	isTreasuryEnabled := p.chain.isTreasuryAgendaActive
	isAutoRevocationsEnabled := p.chain.isAutoRevocationsAgendaActive
	subsidySplitVariant := p.chain.determineSubsidySplitVariant()

	// Get the best block and header.
	bestHeader, err := p.chain.HeaderByHash(&best.Hash)
	if err != nil {
		str := fmt.Sprintf("unable to get tip block header %v", best.Hash)
		return nil, makeError(ErrGetTopBlock, str)
	}

	// Determine what type of transaction we're dealing with (regular or stake).
	// Then, be sure to set the tx tree correctly as it's possible a user submitted
	// it to the network with TxTreeUnknown.
	txType := stake.DetermineTxType(msgTx)
	if txType == stake.TxTypeRegular {
		tx.SetTree(wire.TxTreeRegular)
	} else {
		tx.SetTree(wire.TxTreeStake)
	}
	isVote := txType == stake.TxTypeSSGen

	var isTreasuryBase, isTSpend bool
	if isTreasuryEnabled {
		isTSpend = txType == stake.TxTypeTSpend
		isTreasuryBase = txType == stake.TxTypeTreasuryBase
	}

	// Fetch all of the unspent transaction outputs referenced by the inputs
	// to this transaction.  This function also attempts to fetch the
	// transaction itself to be used for detecting a duplicate transaction
	// without needing to do a separate lookup.
	utxoView, err := p.fetchInputUtxos(tx, isTreasuryEnabled)
	if err != nil {
		return nil, err
	}

	// Transaction is an orphan if any of the inputs don't exist.
	var missingParents []*chainhash.Hash
	for i, txIn := range msgTx.TxIn {
		if (i == 0 && (isVote || isTreasuryBase)) || isTSpend {
			continue
		}

		entry := utxoView.LookupEntry(txIn.PreviousOutPoint)
		if entry == nil || entry.IsSpent() {
			// Must make a copy of the hash here since the iterator
			// is replaced and taking its address directly would
			// result in all of the entries pointing to the same
			// memory location and thus all be the final hash.
			hashCopy := txIn.PreviousOutPoint.Hash
			missingParents = append(missingParents, &hashCopy)

			// Prevent a panic in the logger by continuing here if the
			// transaction input is nil.
			if entry == nil {
				log.Tracef("Transaction %v uses unknown input %v "+
					"and will be considered an orphan", txHash,
					txIn.PreviousOutPoint.Hash)
				continue
			}
			if entry.IsSpent() {
				log.Tracef("Transaction %v uses spent input %v and will be considered "+
					"an orphan", txHash, txIn.PreviousOutPoint.Hash)
			}
		}
	}

	if len(missingParents) > 0 {
		return missingParents, nil
	}

	txFee, err := blockchain.CheckTransactionInputs(p.subsidyCache, tx, nextHeight,
		utxoView, false, p.chainParams, &bestHeader, isTreasuryEnabled,
		isAutoRevocationsEnabled, subsidySplitVariant)
	if err != nil {
		return nil, err
	}

	// Get the total number of signature operations for the transaction.
	totalSigOps, err := p.CountTotalSigOps(tx, txType)
	if err != nil {
		return nil, err
	}

	// Add the transaction to the tx source.
	p.addTransaction(tx, txType, height, txFee, totalSigOps)

	// A regular transaction that is added back to the pool causes any tickets in
	// the pool that redeem it to leave the main pool and enter the stage pool.
	if !isNew && txType == stake.TxTypeRegular {
		for _, redeemer := range p.fetchRedeemers(p.outpoints, tx) {
			redeemerDesc, exists := p.pool[*redeemer.Hash()]
			if exists && redeemerDesc.Type == stake.TxTypeSStx {
				p.RemoveTransaction(redeemer, true, isTreasuryEnabled,
					isAutoRevocationsEnabled)
				p.stageTransaction(redeemer)
			}
		}
	}

	// Keep track of votes separately.
	if isVote {
		p.insertVote(tx)
	}

	// Keep track of tspends separately.
	if isTSpend {
		p.tspends[*txHash] = tx
	}

	return nil, nil
}

// processOrphans determines if there are any orphans which depend on the passed
// transaction hash (it is possible that they are no longer orphans) and
// potentially accepts them to the pool.
func (p *fakeTxSource) processOrphans(acceptedTx *dcrutil.Tx) []*dcrutil.Tx {

	var acceptedTxns []*dcrutil.Tx

	// Start with processing at least the passed transaction.
	processList := []*dcrutil.Tx{acceptedTx}
	for len(processList) > 0 {
		// Pop the transaction to process from the front of the list.
		processItem := processList[0]
		processList[0] = nil
		processList = processList[1:]

		txType := stake.DetermineTxType(processItem.MsgTx())
		tree := wire.TxTreeRegular
		if txType != stake.TxTypeRegular {
			tree = wire.TxTreeStake
		}

		prevOut := wire.OutPoint{Hash: *processItem.Hash(), Tree: tree}
		for txOutIdx := range processItem.MsgTx().TxOut {
			// Look up all orphans that redeem the output that is now available.  This
			// will typically only be one, but it could be multiple if the orphan pool
			// contains double spends.  While it may seem odd that the orphan pool would
			// allow this since there can only possibly ultimately be a single redeemer,
			// it's important to track it this way to prevent malicious actors from being
			// able to purposely construct orphans that would otherwise make outputs
			// unspendable.
			//
			// Skip to the next available output if there are none.
			prevOut.Index = uint32(txOutIdx)
			orphans, exists := p.orphansByPrev[prevOut]
			if !exists {
				continue
			}

			// Potentially accept an orphan into the tx pool.
			for _, tx := range orphans {
				missing, err := p.maybeAcceptTransaction(tx, true)
				if err != nil {
					// The orphan is now invalid, so there is no way any other orphans which
					// redeem any of its outputs can be accepted.  Remove them.
					p.removeOrphan(tx, true)
					break
				}

				// Transaction is still an orphan.  Try the next orphan which redeems this
				// output.
				if len(missing) > 0 {
					continue
				}

				// Transaction was accepted into the main pool.
				//
				// Add it to the list of accepted transactions that are no longer orphans,
				// remove it from the orphan pool, and add it to the list of transactions to
				// process so any orphans that depend on it are handled too.
				acceptedTxns = append(acceptedTxns, tx)
				p.removeOrphan(tx, false)
				processList = append(processList, tx)

				// Only one transaction for this outpoint can be accepted, so the rest are
				// now double spends and are removed later.
				break
			}
		}
	}

	// Recursively remove any orphans that also redeem any outputs redeemed by the
	// accepted transactions since those are now definitive double spends.
	p.removeOrphanDoubleSpends(acceptedTx)
	for _, tx := range acceptedTxns {
		p.removeOrphanDoubleSpends(tx)
	}

	return acceptedTxns
}

// ProcessTransaction is the main entry point for adding new transactions to the
// fake tx source.
func (p *fakeTxSource) ProcessTransaction(tx *dcrutil.Tx) ([]*dcrutil.Tx, error) {
	missingParents, err := p.maybeAcceptTransaction(tx, true)
	if err != nil {
		return nil, err
	}

	// If len(missingParents) == 0 then we know the tx is NOT an orphan.
	if len(missingParents) == 0 {
		// Accept any orphan transactions that depend on this transaction (they may no
		// longer be orphans if all inputs are now available) and repeat for those
		// accepted transactions until there are no more.
		newTxs := p.processOrphans(tx)
		acceptedTxs := make([]*dcrutil.Tx, len(newTxs)+1)

		// Add the parent transaction first so remote nodes do not add orphans.
		acceptedTxs[0] = tx
		copy(acceptedTxs[1:], newTxs)

		return acceptedTxs, nil
	}

	// Add the orphan transaction to the tx source.
	p.addOrphan(tx)

	return nil, err
}

// findTx returns a transaction from the fake tx source by hash.  If it does not
// exist in the fake tx source, a nil pointer is returned.
func (p *fakeTxSource) findTx(txHash *chainhash.Hash) *TxDesc {
	return p.pool[*txHash]
}

// miningDescs returns a slice of mining descriptors for all transactions in the
// fake tx source.
func (p *fakeTxSource) miningDescs() []*TxDesc {
	descs := make([]*TxDesc, len(p.pool))
	i := 0
	for _, desc := range p.pool {
		descs[i] = desc
		i++
	}

	return descs
}

// MiningView returns a snapshot of the underlying TxSource.
func (p *fakeTxSource) MiningView() *TxMiningView {
	return p.miningView.Clone(p.miningDescs(), p.findTx)
}

// fetchInputUtxos loads utxo details about the input transactions referenced by
// the passed transaction.  First, it loads the details from the viewpoint of
// the main chain, then it adjusts them based upon the contents of the
// transaction pool.
func (p *fakeTxSource) fetchInputUtxos(tx *dcrutil.Tx, isTreasuryEnabled bool) (*blockchain.UtxoViewpoint, error) {
	knownDisapproved := p.IsRegTxTreeKnownDisapproved(&p.chain.BestSnapshot().Hash)
	utxoView, err := p.chain.FetchUtxoView(tx, !knownDisapproved)
	if err != nil {
		return nil, err
	}

	// Attempt to populate any missing inputs from the transaction pool.
	for _, txIn := range tx.MsgTx().TxIn {
		prevOut := &txIn.PreviousOutPoint
		entry := utxoView.LookupEntry(*prevOut)
		if entry != nil && !entry.IsSpent() {
			continue
		}

		if poolTxDesc, exists := p.pool[prevOut.Hash]; exists {
			// AddTxOut ignores out of range index values, so it is safe to call without
			// bounds checking here.
			utxoView.AddTxOut(poolTxDesc.Tx, prevOut.Index, UnminedHeight,
				wire.NullBlockIndex, isTreasuryEnabled)
		}

		if stagedTx, exists := p.staged[prevOut.Hash]; exists {
			// AddTxOut ignores out of range index values, so it is safe to call without
			// bounds checking here.
			utxoView.AddTxOut(stagedTx, prevOut.Index, UnminedHeight,
				wire.NullBlockIndex, isTreasuryEnabled)
		}
	}

	return utxoView, nil
}

// miningHarness provides a harness that includes functionality for creating and
// signing transactions, adding/removing transactions to a fake tx source to
// consider for inclusion in new blocks, and a fake chain that provides a mocked
// chain state as well as utxos for use in generating valid transactions.
type miningHarness struct {
	chainParams  *chaincfg.Params
	subsidyCache *standalone.SubsidyCache
	chain        *fakeChain
	policy       *Policy
	txSource     *fakeTxSource

	// signKey is the signing key used for creating transactions throughout
	// the tests.
	//
	// payAddr is the p2sh address for the signing key and is used for the
	// payment address throughout the tests.
	//
	// payScriptVer and payScript are the script version and script to pay the
	// aforementioned payAddr.
	signKey      []byte
	sigType      dcrec.SignatureType
	payAddr      stdaddr.StakeAddress
	payScriptVer uint16
	payScript    []byte

	generator *BlkTmplGenerator
}

// GetScript is the mining harness' implementation of the ScriptDB interface.
// It returns the mining harness' payment redeem script for any address passed
// in.
func (m *miningHarness) GetScript(addr stdaddr.Address) ([]byte, error) {
	return m.payScript, nil
}

// GetKey is the mining harness' implementation of the KeyDB interface. It
// returns the mining harness' signature key for any address passed in.
func (m *miningHarness) GetKey(addr stdaddr.Address) ([]byte, dcrec.SignatureType, bool, error) {
	return m.signKey, m.sigType, true, nil
}

// CreateCoinbaseTx returns a coinbase transaction with the requested number of
// outputs paying an appropriate subsidy based on the passed block height to the
// address associated with the harness.  It automatically uses a standard
// signature script that starts with the required block height.
func (m *miningHarness) CreateCoinbaseTx(blockHeight int64, numOutputs uint32) (*dcrutil.Tx, error) {
	// Create standard coinbase script.
	extraNonce := int64(0)
	coinbaseScript, err := txscript.NewScriptBuilder().
		AddInt64(blockHeight).AddInt64(extraNonce).Script()
	if err != nil {
		return nil, err
	}

	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		// Coinbase transactions have no inputs, so previous outpoint is zero hash and
		// max index.
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
		SignatureScript: coinbaseScript,
		Sequence:        wire.MaxTxInSequenceNum,
	})
	totalInput := m.subsidyCache.CalcBlockSubsidy(blockHeight)
	amountPerOutput := totalInput / int64(numOutputs)
	remainder := totalInput - amountPerOutput*int64(numOutputs)
	for i := uint32(0); i < numOutputs; i++ {
		// Ensure the final output accounts for any remainder that might be left from
		// splitting the input amount.
		amount := amountPerOutput
		if i == numOutputs-1 {
			amount = amountPerOutput + remainder
		}
		tx.AddTxOut(newTxOut(amount, m.payScriptVer, m.payScript))
	}

	return dcrutil.NewTx(tx), nil
}

// CreateTxChain creates a chain of zero-fee transactions (each subsequent
// transaction spends the entire amount from the previous one) with the first
// one spending the provided outpoint.  Each transaction spends the entire
// amount of the previous one and as such does not include any fees.
func (m *miningHarness) CreateTxChain(firstOutput spendableOutput, numTxns uint32) ([]*dcrutil.Tx, error) {
	txChain := make([]*dcrutil.Tx, 0, numTxns)
	prevOutPoint := firstOutput.outPoint
	spendableAmount := firstOutput.amount
	for i := uint32(0); i < numTxns; i++ {
		// Create the transaction using the previous transaction output and paying the
		// full amount to the payment address associated with the harness.
		tx := wire.NewMsgTx()
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: prevOutPoint,
			SignatureScript:  nil,
			Sequence:         wire.MaxTxInSequenceNum,
			ValueIn:          int64(spendableAmount),
		})
		tx.AddTxOut(newTxOut(int64(spendableAmount), m.payScriptVer,
			m.payScript))

		// Sign the new transaction.
		sigScript, err := sign.SignatureScript(tx, 0, m.payScript,
			txscript.SigHashAll, m.signKey, dcrec.STEcdsaSecp256k1, true)
		if err != nil {
			return nil, err
		}
		tx.TxIn[0].SignatureScript = sigScript

		txChain = append(txChain, dcrutil.NewTx(tx))

		// Next transaction uses outputs from this one.
		prevOutPoint = wire.OutPoint{Hash: tx.TxHash(), Index: 0}
	}

	return txChain, nil
}

// CreateTx creates a zero-fee regular transaction from the provided spendable
// output.
func (m *miningHarness) CreateTx(out spendableOutput) (*dcrutil.Tx, error) {
	txns, err := m.CreateTxChain(out, 1)
	if err != nil {
		return nil, err
	}
	return txns[0], err
}

// CreateSignedTx creates a new signed transaction that consumes the provided
// inputs and generates the provided number of outputs by evenly splitting the
// total input amount.  All outputs will be to the payment script associated
// with the harness and all inputs are assumed to do the same.
//
// Additionally, if one or more munge functions are specified, they will be
// invoked with the transaction prior to signing it.  This provides callers with
// the opportunity to modify the transaction which is especially useful for
// testing.
func (m *miningHarness) CreateSignedTx(inputs []spendableOutput, numOutputs uint32, mungers ...func(*wire.MsgTx)) (*dcrutil.Tx, error) {
	// Calculate the total input amount and split it amongst the requested
	// number of outputs.
	var totalInput dcrutil.Amount
	for _, input := range inputs {
		totalInput += input.amount
	}
	amountPerOutput := int64(totalInput) / int64(numOutputs)
	remainder := int64(totalInput) % int64(numOutputs)

	tx := wire.NewMsgTx()
	tx.Expiry = wire.NoExpiryValue
	for _, input := range inputs {
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: input.outPoint,
			SignatureScript:  nil,
			Sequence:         wire.MaxTxInSequenceNum,
			ValueIn:          int64(input.amount),
		})
	}
	for i := uint32(0); i < numOutputs; i++ {
		// Ensure the final output accounts for any remainder that might
		// be left from splitting the input amount.
		amount := amountPerOutput
		if i == numOutputs-1 {
			amount += remainder
		}
		tx.AddTxOut(newTxOut(amount, m.payScriptVer, m.payScript))
	}

	// Perform any transaction munging just before signing.
	for _, f := range mungers {
		f(tx)
	}

	// Sign the new transaction.
	for i := range tx.TxIn {
		sigScript, err := sign.SignatureScript(tx, i, m.payScript,
			txscript.SigHashAll, m.signKey, dcrec.STEcdsaSecp256k1, true)
		if err != nil {
			return nil, err
		}
		tx.TxIn[i].SignatureScript = sigScript
	}

	return dcrutil.NewTx(tx), nil
}

// newTxOut returns a new transaction output with the given parameters.
func newTxOut(amount int64, pkScriptVer uint16, pkScript []byte) *wire.TxOut {
	return &wire.TxOut{
		Value:    amount,
		Version:  pkScriptVer,
		PkScript: pkScript,
	}
}

// CreateTicketPurchase creates a ticket purchase spending the first output of
// the provided transaction.
func (m *miningHarness) CreateTicketPurchase(sourceTx *dcrutil.Tx, cost int64) (*dcrutil.Tx, error) {
	ticketFee := singleInputTicketSize
	ticketPrice := cost

	// Generate the voting rights, commitment, and change scripts of the ticket.
	voteScriptVer, voteScript := m.payAddr.VotingRightsScript()
	commitScriptVer, commitScript := m.payAddr.RewardCommitmentScript(
		ticketPrice+ticketFee, 0, ticketPrice)
	change := sourceTx.MsgTx().TxOut[0].Value - ticketPrice - ticketFee
	changeScriptVer, changeScript := m.payAddr.StakeChangeScript()

	// Generate the ticket purchase.
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  *sourceTx.Hash(),
			Index: 0,
			Tree:  wire.TxTreeRegular,
		},
		Sequence:    wire.MaxTxInSequenceNum,
		ValueIn:     sourceTx.MsgTx().TxOut[0].Value,
		BlockHeight: uint32(m.generator.cfg.BestSnapshot().Height),
	})

	tx.AddTxOut(newTxOut(ticketPrice, voteScriptVer, voteScript))
	tx.AddTxOut(newTxOut(0, commitScriptVer, commitScript))
	tx.AddTxOut(newTxOut(change, changeScriptVer, changeScript))

	// Sign the ticket purchase.
	sigScript, err := sign.SignatureScript(tx, 0,
		sourceTx.MsgTx().TxOut[0].PkScript, txscript.SigHashAll, m.signKey,
		dcrec.STEcdsaSecp256k1, true)
	if err != nil {
		return nil, err
	}
	tx.TxIn[0].SignatureScript = sigScript

	return dcrutil.NewTx(tx), nil
}

// newVoteScript generates a voting script from the passed VoteBits, for use in
// a vote.
func newVoteScript(voteBits stake.VoteBits) ([]byte, error) {
	b := make([]byte, 2+len(voteBits.ExtendedBits))
	binary.LittleEndian.PutUint16(b[0:2], voteBits.Bits)
	copy(b[2:], voteBits.ExtendedBits)
	return stdscript.ProvablyPruneableScriptV0(b)
}

// CreateVote creates a vote transaction using the provided ticket.  The vote
// will vote on the current best block hash and height associated with the
// harness.
//
// Additionally, if one or more munge functions are specified, they will be
// invoked with the transaction prior to signing it.  This provides callers with
// the opportunity to modify the transaction which is especially useful for
// testing.
func (m *miningHarness) CreateVote(ticket *dcrutil.Tx, mungers ...func(*wire.MsgTx)) (*dcrutil.Tx, error) {
	// Calculate the vote subsidy.
	best := m.chain.BestSnapshot()
	subsidy := m.subsidyCache.CalcStakeVoteSubsidyV3(best.Height,
		m.chain.determineSubsidySplitVariant())
	// Parse the ticket purchase transaction and generate the vote reward.
	ticketPayKinds, ticketHash160s, ticketValues, _, _, _ :=
		stake.TxSStxStakeOutputInfo(ticket.MsgTx())
	voteRewardValues := stake.CalculateRewards(ticketValues,
		ticket.MsgTx().TxOut[0].Value, subsidy)

	// Add the stakebase input.
	vote := wire.NewMsgTx()
	stakebaseOutPoint := wire.NewOutPoint(&chainhash.Hash{}, ^uint32(0),
		wire.TxTreeRegular)
	stakebaseInput := wire.NewTxIn(stakebaseOutPoint, subsidy, nil)
	vote.AddTxIn(stakebaseInput)

	// Add the ticket input.
	spendOut := txOutToSpendableOut(ticket, 0, wire.TxTreeStake)
	ticketInput := wire.NewTxIn(&spendOut.outPoint, int64(spendOut.amount), nil)
	ticketInput.BlockHeight = uint32(best.Height)
	ticketInput.BlockIndex = 5
	vote.AddTxIn(ticketInput)

	// Add the block reference output.
	blockRefScript, _ := txscript.GenerateSSGenBlockRef(best.Hash,
		uint32(best.Height))
	vote.AddTxOut(wire.NewTxOut(0, blockRefScript))

	// Create the vote script.
	voteBits := stake.VoteBits{Bits: uint16(0xff), ExtendedBits: []byte{}}
	voteScript, err := newVoteScript(voteBits)
	if err != nil {
		return nil, err
	}
	vote.AddTxOut(wire.NewTxOut(0, voteScript))

	// Create payment scripts for the ticket commitments.
	params := m.chainParams
	for i, h160 := range ticketHash160s {
		var addr stdaddr.StakeAddress
		if ticketPayKinds[i] { // P2SH
			addr, _ = stdaddr.NewAddressScriptHashV0FromHash(h160, params)
		} else {
			addr, _ = stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(h160, params)
		}

		_, script := addr.PayVoteCommitmentScript()
		vote.AddTxOut(wire.NewTxOut(voteRewardValues[i], script))
	}

	// Perform any transaction munging just before signing.
	for _, f := range mungers {
		f(vote)
	}

	// Sign the input.
	inputToSign := 1
	redeemTicketScript := ticket.MsgTx().TxOut[0].PkScript
	signedScript, err := sign.SignTxOutput(params, vote, inputToSign,
		redeemTicketScript, txscript.SigHashAll, m, m,
		vote.TxIn[inputToSign].SignatureScript, m.chain.isTreasuryAgendaActive)
	if err != nil {
		return nil, err
	}

	vote.TxIn[0].SignatureScript = params.StakeBaseSigScript
	vote.TxIn[1].SignatureScript = signedScript

	return dcrutil.NewTx(vote), nil
}

// CountTotalSigOps returns the total number of signature operations for the
// given transaction.
func (m *miningHarness) CountTotalSigOps(tx *dcrutil.Tx) (int, error) {
	txType := stake.DetermineTxType(tx.MsgTx())
	return m.txSource.CountTotalSigOps(tx, txType)
}

// AddTransactionToTxSource adds the given transaction to the tx source.
func (m *miningHarness) AddTransactionToTxSource(tx *dcrutil.Tx) ([]*dcrutil.Tx, error) {
	return m.txSource.ProcessTransaction(tx)
}

// RemoveTransactionFromTxSource removes the given transaction from the tx
// source.
func (m *miningHarness) RemoveTransactionFromTxSource(tx *dcrutil.Tx, removeRedeemers bool) {
	m.txSource.RemoveTransaction(tx, removeRedeemers,
		m.chain.isTreasuryAgendaActive, m.chain.isAutoRevocationsAgendaActive)
}

// AddFakeUTXO creates a fake mined utxo for the provided transaction.
func (m *miningHarness) AddFakeUTXO(tx *dcrutil.Tx, blockHeight int64, blockIndex uint32, isTreasuryAgendaActive bool) {
	m.chain.utxos.AddTxOuts(tx, blockHeight, blockIndex, isTreasuryAgendaActive)
}

// newMiningHarness returns a new instance of a mining harness initialized with a
// fake chain and a fake tx source that are bound to it.  Also, the fake chain
// is populated with the returned spendable outputs so that the caller can
// easily create new valid transactions which build off of it.
//
// The returned mining harness instance is NOT safe for concurrent access.  A
// new mining harness instance should be created for each test case to ensure
// that state changes to the underlying fake chain and fake tx source instances
// do not impact other test cases.
func newMiningHarness(chainParams *chaincfg.Params) (*miningHarness, []spendableOutput, error) {
	// Use a hard coded key pair for deterministic results.
	keyBytes, err := hex.DecodeString("700868df1838811ffbdf918fb482c1f7e" +
		"ad62db4b97bd7012c23e726485e577d")
	if err != nil {
		return nil, nil, err
	}
	signPub := secp256k1.PrivKeyFromBytes(keyBytes).PubKey()

	// Generate associated pay-to-script-hash address and resulting payment
	// script.
	pubKeyBytes := signPub.SerializeCompressed()
	h160 := stdaddr.Hash160(pubKeyBytes)
	payAddr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(h160,
		chainParams)
	if err != nil {
		return nil, nil, err
	}
	payScriptVer, payScript := payAddr.PaymentScript()

	// Create a SigCache instance.
	sigCache, err := txscript.NewSigCache(1000)
	if err != nil {
		return nil, nil, err
	}

	// Create a SubsidyCache instance.
	subsidyCache := standalone.NewSubsidyCache(chainParams)

	// Create a fakeChain instance.
	chain := &fakeChain{
		blocks:                          make(map[chainhash.Hash]*dcrutil.Block),
		isHeaderCommitmentsAgendaActive: true,
		isTreasuryAgendaActive:          true,
		parentUtxos:                     blockchain.NewUtxoViewpoint(nil),
		utxos:                           blockchain.NewUtxoViewpoint(nil),
	}

	// Set the proof of work limit and next required difficulty very high by
	// default so that the hash of generated blocks is nearly guaranteed to meet
	// the proof of work requirements when checking the block sanity.
	chainParams.PowLimitBits = 0xff01ffff
	chainParams.PowLimit = standalone.CompactToBig(chainParams.PowLimitBits)
	chain.calcNextRequiredDifficulty = chainParams.PowLimitBits

	// Create a mining policy with defaults suitable for testing.
	policy := &Policy{
		BlockMaxSize:     uint32(375000),
		TxMinFreeFee:     dcrutil.Amount(1e4),
		AggressiveMining: true,
		StandardVerifyFlags: func() (txscript.ScriptFlags, error) {
			scriptFlags := txscript.ScriptDiscourageUpgradableNops |
				txscript.ScriptVerifyCleanStack |
				txscript.ScriptVerifyCheckLockTimeVerify |
				txscript.ScriptVerifyCheckSequenceVerify |
				txscript.ScriptVerifySHA256
			if chain.isTreasuryAgendaActive {
				scriptFlags |= txscript.ScriptVerifyTreasury
			}
			return scriptFlags, nil
		},
	}

	// Create a fakeTxSource instance.
	txSource := &fakeTxSource{
		chain:           chain,
		chainParams:     chainParams,
		subsidyCache:    subsidyCache,
		pool:            make(map[chainhash.Hash]*TxDesc),
		outpoints:       make(map[wire.OutPoint]*dcrutil.Tx),
		orphans:         make(map[chainhash.Hash]*dcrutil.Tx),
		orphansByPrev:   make(map[wire.OutPoint]map[chainhash.Hash]*dcrutil.Tx),
		staged:          make(map[chainhash.Hash]*dcrutil.Tx),
		stagedOutpoints: make(map[wire.OutPoint]*dcrutil.Tx),
		votes:           make(map[chainhash.Hash][]VoteDesc),
		tspends:         make(map[chainhash.Hash]*dcrutil.Tx),
	}

	// Create a mining view instance for the tx source.  forEachRedeemer defines
	// the function to use to scan the tx source to find which transactions spend a
	// given transaction,
	forEachRedeemer := func(tx *dcrutil.Tx, f func(redeemerTx *TxDesc)) {
		prevOut := wire.OutPoint{Hash: *tx.Hash(), Tree: tx.Tree()}
		txOutLen := uint32(len(tx.MsgTx().TxOut))
		for i := uint32(0); i < txOutLen; i++ {
			prevOut.Index = i
			if txRedeemer, exists := txSource.outpoints[prevOut]; exists {
				f(txSource.pool[txRedeemer.MsgTx().TxHash()])
			}
		}
	}
	txSource.miningView = NewTxMiningView(true, forEachRedeemer)

	// Create the mining harness instance.
	harness := &miningHarness{
		chainParams:  chainParams,
		subsidyCache: subsidyCache,
		chain:        chain,
		policy:       policy,
		txSource:     txSource,
		signKey:      keyBytes,
		sigType:      dcrec.STEcdsaSecp256k1,
		payAddr:      payAddr,
		payScriptVer: payScriptVer,
		payScript:    payScript,
		generator: NewBlkTmplGenerator(&Config{
			Policy:                     policy,
			TxSource:                   txSource,
			TimeSource:                 blockchain.NewMedianTime(),
			SubsidyCache:               subsidyCache,
			ChainParams:                chainParams,
			MiningTimeOffset:           0,
			BestSnapshot:               chain.BestSnapshot,
			BlockByHash:                chain.BlockByHash,
			CalcNextRequiredDifficulty: chain.CalcNextRequiredDifficulty,
			CalcStakeVersionByHash:     chain.CalcStakeVersionByHash,
			CheckConnectBlockTemplate:  chain.CheckConnectBlockTemplate,
			CheckTicketExhaustion:      chain.CheckTicketExhaustion,
			CheckTransactionInputs: func(tx *dcrutil.Tx, txHeight int64,
				view *blockchain.UtxoViewpoint, checkFraudProof bool,
				prevHeader *wire.BlockHeader, isTreasuryEnabled,
				isAutoRevocationsEnabled bool,
				subsidySplitVariant standalone.SubsidySplitVariant) (int64, error) {

				return blockchain.CheckTransactionInputs(subsidyCache, tx, txHeight,
					view, checkFraudProof, chainParams, prevHeader, isTreasuryEnabled,
					isAutoRevocationsEnabled, subsidySplitVariant)
			},
			CheckTSpendHasVotes:             chain.CheckTSpendHasVotes,
			CountSigOps:                     blockchain.CountSigOps,
			FetchUtxoEntry:                  chain.FetchUtxoEntry,
			FetchUtxoView:                   chain.FetchUtxoView,
			FetchUtxoViewParentTemplate:     chain.FetchUtxoViewParentTemplate,
			ForceHeadReorganization:         chain.ForceHeadReorganization,
			HeaderByHash:                    chain.HeaderByHash,
			IsFinalizedTransaction:          blockchain.IsFinalizedTransaction,
			IsHeaderCommitmentsAgendaActive: chain.IsHeaderCommitmentsAgendaActive,
			IsTreasuryAgendaActive:          chain.IsTreasuryAgendaActive,
			IsAutoRevocationsAgendaActive:   chain.IsAutoRevocationsAgendaActive,
			IsSubsidySplitAgendaActive:      chain.IsSubsidySplitAgendaActive,
			IsSubsidySplitR2AgendaActive:    chain.IsSubsidySplitR2AgendaActive,
			MaxTreasuryExpenditure:          chain.MaxTreasuryExpenditure,
			NewUtxoViewpoint:                chain.NewUtxoViewpoint,
			TipGeneration:                   chain.TipGeneration,
			ValidateTransactionScripts: func(tx *dcrutil.Tx,
				utxoView *blockchain.UtxoViewpoint, flags txscript.ScriptFlags,
				isAutoRevocationsEnabled bool) error {

				return blockchain.ValidateTransactionScripts(tx, utxoView, flags,
					sigCache, isAutoRevocationsEnabled)
			},
		}),
	}

	// Create a single coinbase transaction and add it to the harness
	// chain's utxo set and set the harness chain height such that the
	// coinbase will mature in the next block.  This ensures the txpool
	// accepts transactions which spend immature coinbases that will become
	// mature in the next block.
	numOutputs := uint32(1)
	outputs := make([]spendableOutput, 0, numOutputs)
	curHeight := chain.bestState.Height
	coinbase, err := harness.CreateCoinbaseTx(curHeight+1, numOutputs)
	if err != nil {
		return nil, nil, err
	}
	harness.AddFakeUTXO(coinbase, curHeight+1, wire.NullBlockIndex,
		chain.isTreasuryAgendaActive)
	for i := uint32(0); i < numOutputs; i++ {
		outputs = append(outputs, txOutToSpendableOut(coinbase, i,
			wire.TxTreeRegular))
	}

	// Mock the chain best block and state.
	mockBestBlock := *dcrutil.NewBlock(&wire.MsgBlock{})
	mockBestHash := mockBestBlock.Hash()
	chain.tipGeneration = []chainhash.Hash{*mockBestHash}
	chain.blocks[*mockBestHash] = &mockBestBlock
	chain.bestState = blockchain.BestState{
		Hash:   *mockBestHash,
		Height: int64(chainParams.CoinbaseMaturity) + curHeight,
	}

	return harness, outputs, nil
}
