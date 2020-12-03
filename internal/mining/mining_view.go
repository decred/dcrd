// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v3"
)

const (
	// ancestorTrackingLimit is the maximum number of ancestors a transaction
	// can have ancestor stats calculated for.
	ancestorTrackingLimit = 25
)

// defaultAncestorStats is the default-state of ancestor stats for a transaction
// that has no ancestors.
var defaultAncestorStats = TxAncestorStats{}

// TxMiningView represents a snapshot of all transactions, as well as the
// hierarchy of transactions, that are ready to be mined.
type TxMiningView struct {
	rejected           map[chainhash.Hash]struct{}
	txGraph            *txDescGraph
	txDescs            []*TxDesc
	trackAncestorStats bool
	ancestorStats      map[chainhash.Hash]*TxAncestorStats
}

// NewTxMiningView creates a new mining view instance.  The forEachRedeemer
// parameter should define the function to be used for finding transactions that
// spend a given transaction.
func NewTxMiningView(enableAncestorTracking bool,
	forEachRedeemer func(tx *dcrutil.Tx, f func(redeemerTx *TxDesc))) *TxMiningView {

	return &TxMiningView{
		rejected:           make(map[chainhash.Hash]struct{}),
		txGraph:            newTxDescGraph(forEachRedeemer),
		txDescs:            nil,
		trackAncestorStats: enableAncestorTracking,
		ancestorStats:      make(map[chainhash.Hash]*TxAncestorStats),
	}
}

// addAncestorTo modifies the TxAncestorStats instance to include the provided
// TxDesc's statistics.
func addAncestorTo(stats *TxAncestorStats, txDesc *TxDesc) {
	stats.Fees += txDesc.Fee
	stats.SizeBytes += txDesc.TxSize
	stats.TotalSigOps += txDesc.TotalSigOps
	stats.NumAncestors++
}

// addDescendantTo modifies the TxAncestorStats instance to account for the
// addition of a newly tracked descendant.
func addDescendantTo(stats *TxAncestorStats) {
	stats.NumDescendants++
}

// removeAncestorFrom modifies the TxAncestorStats instance to stop storing
// the statistics of the provided TxDesc.
func removeAncestorFrom(stats *TxAncestorStats, txDesc *TxDesc) {
	stats.Fees -= txDesc.Fee
	stats.SizeBytes -= txDesc.TxSize
	stats.TotalSigOps -= txDesc.TotalSigOps
	stats.NumAncestors--
}

// removeDescendantFrom modifies the TxAncestorStats instance to account for
// the removal of a descendant.
func removeDescendantFrom(stats *TxAncestorStats) {
	stats.NumDescendants--
}

// ancestors returns a collection of all transactions in the graph that the
// provided transaction hash depends on, and its ancestors' bundle stats.
//
// This function is NOT safe for concurrent access.
func (mv *TxMiningView) ancestors(txHash *chainhash.Hash) []*TxDesc {
	if !mv.trackAncestorStats {
		return nil
	}

	ancestors := make([]*TxDesc, 0, ancestorTrackingLimit)
	var baseTxStats *TxAncestorStats
	if oldStats, hadStats := mv.ancestorStats[*txHash]; hadStats {
		// If the transaction had statistics tracked already, then we should
		// copy the number of descendants since that value is up to date. Use
		// a new instance of TxAncestorStats to avoid mutating references
		// that may be held between calls to this function.
		baseTxStats = &TxAncestorStats{}
		baseTxStats.NumDescendants = oldStats.NumDescendants
	}

	seen := make(map[chainhash.Hash]struct{}, ancestorTrackingLimit)
	mv.txGraph.forEachAncestor(txHash, seen, func(txDesc *TxDesc) {
		if baseTxStats == nil {
			baseTxStats = &TxAncestorStats{}
		}

		addAncestorTo(baseTxStats, txDesc)
		ancestors = append(ancestors, txDesc)
	})

	if baseTxStats == nil {
		delete(mv.ancestorStats, *txHash)
	} else {
		// Update the cache.
		mv.ancestorStats[*txHash] = baseTxStats
	}

	return ancestors
}

// AncestorStats returns the view's cached statistics for all of the provided
// transaction's ancestors.
//
// This function is NOT safe for concurrent access.
func (mv *TxMiningView) AncestorStats(txHash *chainhash.Hash) (*TxAncestorStats, bool) {
	if !mv.trackAncestorStats {
		return &defaultAncestorStats, false
	}

	if stats, exists := mv.ancestorStats[*txHash]; exists {
		return stats, exists
	}

	return &defaultAncestorStats, false
}

// children returns a set of transactions in the graph that spend from the
// provided transaction hash. The order of elements returned is not guaranteed.
//
// This function is NOT safe for concurrent access.
func (mv *TxMiningView) children(txHash *chainhash.Hash) []*TxDesc {
	childrenOf := mv.txGraph.childrenOf[*txHash]
	if len(childrenOf) == 0 {
		return nil
	}

	children := make([]*TxDesc, 0, len(childrenOf))
	for _, tx := range childrenOf {
		children = append(children, tx)
	}

	return children
}

// Clone makes a deep copy of the mining view and underlying transaction graph.
// fetchTx is the locator functon that is used to find transactions that should
// be included in the mining view.
//
// This function is NOT safe for concurrent access.
func (mv *TxMiningView) Clone(txDescs []*TxDesc, fetchTx TxDescFind) *TxMiningView {
	view := &TxMiningView{
		rejected:           make(map[chainhash.Hash]struct{}),
		txGraph:            mv.txGraph.clone(fetchTx),
		txDescs:            txDescs,
		trackAncestorStats: mv.trackAncestorStats,
		ancestorStats: make(map[chainhash.Hash]*TxAncestorStats,
			len(mv.ancestorStats)),
	}

	for key, value := range mv.ancestorStats {
		view.ancestorStats[key] = &TxAncestorStats{
			Fees:           value.Fees,
			SizeBytes:      value.SizeBytes,
			TotalSigOps:    value.TotalSigOps,
			NumAncestors:   value.NumAncestors,
			NumDescendants: value.NumDescendants,
		}
	}

	return view
}

// descendants returns a collection of transactions in the mining view that
// depend on the provided transaction hash.
//
// This function is NOT safe for concurrent access.
func (mv *TxMiningView) descendants(txHash *chainhash.Hash) []*chainhash.Hash {
	seen := map[chainhash.Hash]struct{}{}
	descendants := make([]*chainhash.Hash, 0)
	mv.txGraph.forEachDescendant(txHash, seen, func(descendant *TxDesc) {
		descendants = append(descendants, descendant.Tx.Hash())
	})
	return descendants
}

// hasParents returns true if the provided transaction hash spends from another
// transaction in the mining view.
//
// This function is NOT safe for concurrent access.
func (mv *TxMiningView) hasParents(txHash *chainhash.Hash) bool {
	return len(mv.txGraph.parentsOf[*txHash]) > 0
}

// isRejected returns true if the given hash has been passed to Reject
// on this instance of the view.
//
// This function is NOT safe for concurrent access.
func (mv *TxMiningView) isRejected(txHash *chainhash.Hash) bool {
	_, exist := mv.rejected[*txHash]
	return exist
}

// parents returns a set of transactions in the graph that the provided
// transaction hash spends from in the view. The order of elements
// returned is not guaranteed.
//
// This function is NOT safe for concurrent access.
func (mv *TxMiningView) parents(txHash *chainhash.Hash) []*TxDesc {
	parentsOf := mv.txGraph.parentsOf[*txHash]
	if len(parentsOf) == 0 {
		return nil
	}

	parents := make([]*TxDesc, 0, len(parentsOf))
	for _, tx := range parentsOf {
		parents = append(parents, tx)
	}

	return parents
}

// maybeUpdateAncestorStats attempts to update the ancestor stats for a given
// transaction. It walks the graph for all ancestors of the provided transaction
// and aggregates statistics for its ancestors. It also accepts a hash for
// an ancestor that should be treated as though it does not exist in the
// graph.
func (mv *TxMiningView) maybeUpdateAncestorStats(txDesc *TxDesc,
	ignoreTxHash *chainhash.Hash) bool {

	baseTxHash := txDesc.Tx.Hash()
	canTrackAncestors := true
	seenAncestors := make(map[chainhash.Hash]*TxDesc,
		ancestorTrackingLimit)
	mv.txGraph.forEachAncestorPreOrder(baseTxHash, seenAncestors,
		func(ancestorTxDesc *TxDesc) bool {
			if !canTrackAncestors {
				// Short circuit the below checks if we know that the provided
				// transaction cannot have ancestor stats aggregated. This also
				// stops adding transactions to the seenAncestors map.
				return false
			}

			ancestorTxHash := *ancestorTxDesc.Tx.Hash()
			if ancestorTxHash == *ignoreTxHash {
				// Skip this ancestor but continue walking others.
				// This accounts for a scenario where a transaction that will be
				// removed from the miningView should cause descendants on the
				// outer edge of the ancestor limit to potentially have ancestor
				// stats calculated. However, in order to walk descendants of
				// the transaction pending removal, it must exist in the graph
				// so ignore it.
				return false
			}

			if len(seenAncestors) >= ancestorTrackingLimit {
				// A transaction with too many ancestors should not have
				// ancestor stats cached.
				canTrackAncestors = false
				return false
			}

			// If any ancestor transaction does not have stats, then the
			// provided one should not either.
			ancestorStats, exists := mv.ancestorStats[ancestorTxHash]
			if !exists {
				canTrackAncestors = false
				return false
			}

			// If any ancestor plus its respective ancestors would exceed the
			// limit for this transaction, then do not track stats.
			if ancestorStats.NumAncestors+1 > ancestorTrackingLimit {
				canTrackAncestors = false
				return false
			}

			// If tracking this transaction would cause any ancestor to have
			// more descendants than the tracking limit, then stats should not
			// be tracked for the base transaction.
			if ancestorStats.NumDescendants+1 > ancestorTrackingLimit {
				canTrackAncestors = false
				return false
			}
			return true
		})

	if !canTrackAncestors {
		delete(mv.ancestorStats, *baseTxHash)
		return false
	}

	// The transaction has passed all checks and will have ancestor statistics
	// tracked and updated. All elements of the seenAncestors map will have
	// ancestor statistics tracked. This is guaranteed by a check performed
	// during a prior walk that limits what enters the seenAncestors map.
	baseTxnAncestorStats := &TxAncestorStats{}
	for ancestorTxHash, ancestorTxDesc := range seenAncestors {
		addAncestorTo(baseTxnAncestorStats, ancestorTxDesc)
		addDescendantTo(mv.ancestorStats[ancestorTxHash])
	}
	mv.ancestorStats[*baseTxHash] = baseTxnAncestorStats
	return true
}

// updateStatsDescendantsAdded adds the provided transaction's stats
// to all transactions in the graph that depend on it.
func (mv *TxMiningView) updateStatsDescendantsAdded(baseTxDesc *TxDesc) {
	if len(mv.txGraph.childrenOf[*baseTxDesc.Tx.Hash()]) == 0 {
		// Return early if the base transaction has no descendants.
		return
	}

	baseTxHash := baseTxDesc.Tx.Hash()
	baseTxStats, baseTxHasStats := mv.ancestorStats[*baseTxHash]
	seenDescendants := make(map[chainhash.Hash]struct{},
		ancestorTrackingLimit+1)

	mv.txGraph.forEachDescendantPreOrder(baseTxHash, seenDescendants,
		func(descendant *TxDesc) bool {
			descendantTxHash := *descendant.Tx.Hash()
			descendantStats, descendantHasStats :=
				mv.ancestorStats[descendantTxHash]
			if !descendantHasStats {
				// Cannot update ancestor stats for a transaction or its
				// descendants if it does not have ancestor stats tracked.
				return false
			}

			if !baseTxHasStats {
				// If the base tx does not have ancestor stats, then remove them
				// for all of its descendants. This can happen during a reorg or
				// disapproval event when two transaction chains are joined by
				// the base transaction.
				delete(mv.ancestorStats, descendantTxHash)
				return true
			}

			if baseTxStats.NumDescendants+1 > ancestorTrackingLimit {
				// The base transaction has enough tracked descendants. Stop
				// tracking this descendant and its descendants.
				delete(mv.ancestorStats, descendantTxHash)
				return true
			}

			if descendantStats.NumAncestors+1 > ancestorTrackingLimit {
				// The descendant has too many tracked ancestors. Stop
				// tracking this descendant and its descendants.
				delete(mv.ancestorStats, descendantTxHash)
				return true
			}

			// Update the stats for this descendant since it is allowed to have
			// ancestor statistics tracked. Also add the descendant to the
			// statistics of the base transaction.
			addAncestorTo(descendantStats, baseTxDesc)
			addDescendantTo(baseTxStats)
			return true
		})
}

// updateStatsDescendantsRemoved removes the provided transaction's stats
// from all descendant transactions.
func (mv *TxMiningView) updateStatsDescendantsRemoved(baseTxDesc *TxDesc) {
	baseTxHash := baseTxDesc.Tx.Hash()
	if _, hasStats := mv.ancestorStats[*baseTxHash]; !hasStats {
		// If the transaction does not have ancestor tracking enabled, then
		// none of its descendants should either.
		return
	}

	numUntrackedDescendants := 0
	seenDescendants := make(map[chainhash.Hash]struct{}, ancestorTrackingLimit)
	mv.txGraph.forEachDescendantPreOrder(baseTxHash, seenDescendants,
		func(descendantTxDesc *TxDesc) bool {
			descendantTxHash := *descendantTxDesc.Tx.Hash()
			descendantStats, hasStats := mv.ancestorStats[descendantTxHash]

			// Attempt to track the descendant's ancestor stats if it was not
			// tracked previously. This scenario applies to a transaction
			// that once had too many ancestors, but is now eligible to
			// have stats tracked.
			// A check is also performed to avoid testing too many descendants
			// for eligibility to be included in the tracked set, since no
			// descendant can know if any one of its ancestors has too many
			// descendants tracked without visiting the ancestor. Note that
			// although this has a non-ideal behavior of potentially skipping
			// transactions that would otherwise be eligible to have stats
			// tracked, it bounds the number of times ancestors are walked.
			if !hasStats && numUntrackedDescendants < ancestorTrackingLimit {
				numUntrackedDescendants++

				// Note that this call retrieves ancestors while walking
				// descendants of another transaction, and that the time
				// complexity is bound by the maximum number of ancestors
				// allowed to be tracked in the graph and the maximum
				// number of descendants allowed to have ancestor stats tracked.
				mv.maybeUpdateAncestorStats(descendantTxDesc, baseTxHash)

				// Do not walk descendants of this descendant if it did not
				// have ancestor stats previously because it was on the edge of
				// the ancestor limit. Any transactions that descend from it
				// would exceed the limit by at least one.
				return false
			}

			if hasStats {
				removeAncestorFrom(descendantStats, baseTxDesc)
				return true
			}

			// If the transaction does not have statistics tracked then none of
			// its descendants will either.
			return false
		})

	// Update all ancestors to account for the removal of a descendant.
	seenAncestors := make(map[chainhash.Hash]*TxDesc, ancestorTrackingLimit)
	mv.txGraph.forEachAncestorPreOrder(baseTxHash, seenAncestors,
		func(ancestorTxDesc *TxDesc) bool {
			ancestorStats, exists := mv.ancestorStats[*ancestorTxDesc.Tx.Hash()]
			if exists {
				removeDescendantFrom(ancestorStats)
			}
			return true
		})
}

// AddTransaction inserts a TxDesc into the mining view if it has a parent
// or child in the view, or has a relationship with another transaction through
// findTx.
//
// This function is NOT safe for concurrent access.
func (mv *TxMiningView) AddTransaction(txDesc *TxDesc, findTx TxDescFind) {
	// Track the txn in the graph and attempt to relate it to at least one other
	// transaction.
	mv.txGraph.insert(txDesc, findTx)

	if mv.trackAncestorStats {
		mv.maybeUpdateAncestorStats(txDesc, &zeroHash)
		// When a transaction is added back to the view, update the stats of its
		// descendants.
		mv.updateStatsDescendantsAdded(txDesc)
	}
}

// RemoveTransaction stops tracking the transaction in the mining view if it
// exists.  If updateDescendantStats is true, then the statistics for all
// descendant transactions are updated to account for the removal of an
// ancestor.
//
// This function is NOT safe for concurrent access.
func (mv *TxMiningView) RemoveTransaction(txHash *chainhash.Hash, updateDescendantStats bool) {
	if mv.trackAncestorStats && updateDescendantStats {
		txDesc := mv.txGraph.find(txHash)
		if txDesc != nil {
			mv.updateStatsDescendantsRemoved(txDesc)
		}
	}

	mv.txGraph.remove(txHash)
	delete(mv.ancestorStats, *txHash)
}

// reject stops tracking the transaction in the view, if it exists, and all
// of its descendants.  Also flags the provided transaction as rejected and
// tracks the hash as rejected in this instance of the mining view. Rejected
// transactions are not shared between mining view instances, and a new
// mining view is always initialized with an empty set of rejected transactions.
//
// This function is NOT safe for concurrent access.
func (mv *TxMiningView) reject(txHash *chainhash.Hash) {
	seen := make(map[chainhash.Hash]struct{})
	mv.txGraph.forEachDescendant(txHash, seen, func(descendant *TxDesc) {
		mv.RemoveTransaction(descendant.Tx.Hash(), false)
		mv.rejected[*descendant.Tx.Hash()] = struct{}{}
	})

	mv.RemoveTransaction(txHash, false)
	mv.rejected[*txHash] = struct{}{}
}

// TxDescs returns a collection of all transactions available in the view.
func (mv *TxMiningView) TxDescs() []*TxDesc {
	return mv.txDescs
}
