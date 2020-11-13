// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v3"
)

// txDescGraph relates a set of transactions to their respective descendants and
// ancestors. It only stores transactions that have at least one edge relating
// to another.
type txDescGraph struct {
	forEachRedeemer func(tx *dcrutil.Tx, f func(redeemerTx *TxDesc))
	childrenOf      map[chainhash.Hash]map[chainhash.Hash]*TxDesc
	parentsOf       map[chainhash.Hash]map[chainhash.Hash]*TxDesc
}

// TxDescFind is used to inject a transaction repository into the mining view.
// This might either come from the mempool's outpoint map or from the mining
// view itself.
type TxDescFind func(txHash *chainhash.Hash) *TxDesc

// newTxDescGraph creates a new transaction graph instance.  The forEachRedeemer
// parameter should define the function to be used for finding transactions that
// spend a given transaction.
func newTxDescGraph(forEachRedeemer func(tx *dcrutil.Tx, f func(redeemerTx *TxDesc))) *txDescGraph {
	return &txDescGraph{
		forEachRedeemer: forEachRedeemer,
		childrenOf:      make(map[chainhash.Hash]map[chainhash.Hash]*TxDesc),
		parentsOf:       make(map[chainhash.Hash]map[chainhash.Hash]*TxDesc),
	}
}

// addChild adds a child transaction to the graph as a dependent of the
// provided transaction tx.
func (g *txDescGraph) addChild(tx *TxDesc, child *TxDesc) {
	txHash := *tx.Tx.Hash()
	if _, exists := g.childrenOf[txHash]; !exists {
		g.childrenOf[txHash] = make(map[chainhash.Hash]*TxDesc,
			len(tx.Tx.MsgTx().TxOut))
	}

	g.childrenOf[txHash][*child.Tx.Hash()] = child
}

// addParent adds a parent transaction to the graph as a dependency of the
// provided transaction tx.
func (g *txDescGraph) addParent(tx *TxDesc, parent *TxDesc) {
	txHash := *tx.Tx.Hash()
	if _, exists := g.parentsOf[txHash]; !exists {
		g.parentsOf[txHash] = make(map[chainhash.Hash]*TxDesc,
			len(tx.Tx.MsgTx().TxIn))
	}

	g.parentsOf[txHash][*parent.Tx.Hash()] = parent
}

// find returns the TxDesc stored in the graph by its hash. If the transaction
// is not in the graph, then a nil pointer is returned. Since each transaction
// in the graph must have at least one edge connecting it to another
// transaction, scanning for a known hash as a child or parent is sufficient
// to recover its pointer.
func (g *txDescGraph) find(txHash *chainhash.Hash) *TxDesc {
	for parentHash := range g.parentsOf[*txHash] {
		return g.childrenOf[parentHash][*txHash]
	}

	for childHash := range g.childrenOf[*txHash] {
		return g.parentsOf[childHash][*txHash]
	}

	return nil
}

// forEachAncestor iterates over all transactions in the graph that txHash
// depends on and invokes the function f for each transaction,
// in topological order.
func (g *txDescGraph) forEachAncestor(txHash *chainhash.Hash,
	seen map[chainhash.Hash]struct{}, f func(tx *TxDesc)) {

	for parent, parentDesc := range g.parentsOf[*txHash] {
		if _, saw := seen[parent]; saw {
			continue
		}

		seen[parent] = struct{}{}
		g.forEachAncestor(&parent, seen, f)
		f(parentDesc)
	}
}

// forEachAncestorPreOrder iterates over all transactions in the graph that
// txHash depends on and invokes the function f for each, in pre-order.
// If the provided function f returns true then it continues to walk ancestors
// of the respective ancestor. If it returns false, then no additional parents
// of the provided transaction will be visited and the transaction passed to f
// will not be added to the seen map.
func (g *txDescGraph) forEachAncestorPreOrder(txHash *chainhash.Hash,
	seen map[chainhash.Hash]*TxDesc, f func(tx *TxDesc) bool) {

	for parentHash, parentTxDesc := range g.parentsOf[*txHash] {
		if _, saw := seen[parentHash]; saw {
			continue
		}

		moveNext := f(parentTxDesc)
		if !moveNext {
			return
		}

		seen[parentHash] = parentTxDesc
		g.forEachAncestorPreOrder(&parentHash, seen, f)
	}
}

// forEachDescendant iterates depth-first over all transactions that depend on
// the provided transaction hash and invokes function f with each in post-order.
func (g *txDescGraph) forEachDescendant(txHash *chainhash.Hash,
	seen map[chainhash.Hash]struct{}, f func(*TxDesc)) {

	for child, childDesc := range g.childrenOf[*txHash] {
		if _, saw := seen[child]; saw {
			continue
		}

		seen[child] = struct{}{}
		g.forEachDescendant(&child, seen, f)
		f(childDesc)
	}
}

// forEachDescendantPreOrder attempts to walk all transactions that depend on
// the provided transaction hash by invoking the function f with each in
// pre-order. If the provided function f returns true then the traversal will
// walk descendants of the provided transaction's respective child.
func (g *txDescGraph) forEachDescendantPreOrder(txHash *chainhash.Hash,
	seen map[chainhash.Hash]struct{}, f func(*TxDesc) bool) {

	for child, childDesc := range g.childrenOf[*txHash] {
		if _, saw := seen[child]; saw {
			continue
		}

		seen[child] = struct{}{}
		if f(childDesc) {
			g.forEachDescendantPreOrder(&child, seen, f)
		}
	}
}

// insert adds a transaction to the graph and creates a 2-way association
// between itself and its parents.
func (g *txDescGraph) insert(txDesc *TxDesc, findTx TxDescFind) {
	seen := make(map[chainhash.Hash]struct{})

	// Fetch transactions that spend this one from the graph.
	g.forEachRedeemer(txDesc.Tx, func(child *TxDesc) {
		g.addChild(txDesc, child)
		g.addParent(child, txDesc)
	})

	// Relate self with direct ancestors.
	for _, txIn := range txDesc.Tx.MsgTx().TxIn {
		parentHash := txIn.PreviousOutPoint.Hash
		if _, saw := seen[parentHash]; saw {
			continue
		}

		seen[parentHash] = struct{}{}

		// Find parents using the provided locator function and add them to the graph.
		if parentTx := findTx(&parentHash); parentTx != nil {
			g.addParent(txDesc, parentTx)
			g.addChild(parentTx, txDesc)
		}
	}
}

// remove deletes the provided txn hash from the graph. If it is the only
// relationship held by a related transaction in the graph, that related
// transaction is also removed.
func (g *txDescGraph) remove(txHash *chainhash.Hash) {
	// Remove references to tx from all children.
	for childHash := range g.childrenOf[*txHash] {
		delete(g.parentsOf[childHash], *txHash)

		// If the child has no more parents, remove reference.
		if len(g.parentsOf[childHash]) == 0 {
			delete(g.parentsOf, childHash)
		}
	}

	// Remove references to tx from all parents.
	for parentHash := range g.parentsOf[*txHash] {
		delete(g.childrenOf[parentHash], *txHash)

		// If the parent has no more children, remove reference.
		if len(g.childrenOf[parentHash]) == 0 {
			delete(g.childrenOf, parentHash)
		}
	}

	// Remove reference since we don't need to track this txn's
	// parents or children
	delete(g.parentsOf, *txHash)
	delete(g.childrenOf, *txHash)
}

// clone returns a copy of the current graph.
func (g *txDescGraph) clone(fetchTx TxDescFind) *txDescGraph {
	graph := &txDescGraph{
		parentsOf: make(map[chainhash.Hash]map[chainhash.Hash]*TxDesc,
			len(g.parentsOf)),
		childrenOf: make(map[chainhash.Hash]map[chainhash.Hash]*TxDesc,
			len(g.childrenOf)),
	}

	// Source transactions from within the graph to decouple the cloned
	// mining view instance from the original transaction source.
	graph.forEachRedeemer = func(tx *dcrutil.Tx, f func(redeemerTx *TxDesc)) {
		for _, childTx := range graph.childrenOf[*tx.Hash()] {
			f(childTx)
		}
	}

	// Copy parents and children. Anything tracked by the graph
	// will either be a child or parent of another element in the graph.
	for txHash := range g.parentsOf {
		txDesc := fetchTx(&txHash)
		graph.insert(txDesc, fetchTx)
	}

	for txHash := range g.childrenOf {
		txDesc := fetchTx(&txHash)
		graph.insert(txDesc, fetchTx)
	}

	return graph
}
