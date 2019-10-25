// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"

	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database/v2"
)

// maybeFetchNewTickets loads the list of newly maturing tickets for a given
// node by traversing backwards through its parents until it finds the block
// that contains the original tickets to mature if needed.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) maybeFetchNewTickets(node *blockNode) error {
	// Nothing to do if the tickets are already loaded.  It's important to make
	// the distinction here that nil means the value was never looked up, while
	// an empty slice means that there are no new tickets at this height.
	if node.newTickets != nil {
		return nil
	}

	// No tickets in the live ticket pool are possible before stake enabled
	// height.
	if node.height < b.chainParams.StakeEnabledHeight {
		node.newTickets = []chainhash.Hash{}
		return nil
	}

	// Calculate block number for where new tickets matured from and retrieve
	// its block from DB.
	matureNode := node.RelativeAncestor(int64(b.chainParams.TicketMaturity))
	if matureNode == nil {
		return fmt.Errorf("unable to obtain ancestor %d blocks prior to %s "+
			"(height %d)", b.chainParams.TicketMaturity, node.hash, node.height)
	}
	matureBlock, err := b.fetchBlockByNode(matureNode)
	if err != nil {
		return err
	}

	// Extract any ticket purchases from the block and cache them.
	tickets := []chainhash.Hash{}
	for _, stx := range matureBlock.MsgBlock().STransactions {
		if stake.IsSStx(stx) {
			tickets = append(tickets, stx.TxHash())
		}
	}
	node.newTickets = tickets
	return nil
}

// maybeFetchTicketInfo loads and populates prunable ticket information in the
// provided block node if needed.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) maybeFetchTicketInfo(node *blockNode) error {
	// Load and populate the tickets maturing in this block when they are not
	// already loaded.
	if err := b.maybeFetchNewTickets(node); err != nil {
		return err
	}

	// Load and populate the vote and revocation information as needed.
	if node.ticketsVoted == nil || node.ticketsRevoked == nil ||
		node.votes == nil {

		block, err := b.fetchBlockByNode(node)
		if err != nil {
			return err
		}

		node.populateTicketInfo(stake.FindSpentTicketsInBlock(block.MsgBlock()))
	}

	return nil
}

// fetchStakeNode returns the stake node associated with the requested node
// while handling the logic to create the stake node if needed.  In the majority
// of cases, the stake node either already exists and is simply returned, or it
// can be quickly created when the parent stake node is already available.
// However, it should be noted that, since old stake nodes are pruned, this
// function can be quite expensive if a node deep in history or on a long side
// chain is requested since that requires reconstructing all of the intermediate
// nodes along the path from the existing tip to the requested node that have
// not already been pruned.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) fetchStakeNode(node *blockNode) (*stake.Node, error) {
	// Return the cached immutable stake node when it is already loaded.
	if node.stakeNode != nil {
		return node.stakeNode, nil
	}

	// Create the requested stake node from the parent stake node if it is
	// already loaded as an optimization.
	if node.parent.stakeNode != nil {
		// Populate the prunable ticket information as needed.
		if err := b.maybeFetchTicketInfo(node); err != nil {
			return nil, err
		}

		stakeNode, err := node.parent.stakeNode.ConnectNode(node.lotteryIV(),
			node.ticketsVoted, node.ticketsRevoked, node.newTickets)
		if err != nil {
			return nil, err
		}
		node.stakeNode = stakeNode

		return stakeNode, nil
	}

	// -------------------------------------------------------------------------
	// In order to create the stake node, it is necessary to generate a path to
	// the stake node from the current tip, which always has the stake node
	// loaded, and undo the effects of each block back to, and including, the
	// fork point (which might be the requested node itself), and then, in the
	// case the target node is on a side chain, replay the effects of each on
	// the side chain.  In most cases, many of the stake nodes along the path
	// will already be loaded, so, they are only regenerated and populated if
	// they aren't.
	//
	// For example, consider the following scenario:
	//   A -> B  -> C  -> D
	//    \-> B' -> C'
	//
	// Further assume the requested stake node is for C'.  The code that follows
	// will regenerate and populate (only for those not already loaded) the
	// stake nodes for C, B, A, B', and finally, C'.
	// -------------------------------------------------------------------------

	// Start by undoing the effects from the current tip back to, and including
	// the fork point per the above description.
	tip := b.bestChain.Tip()
	fork := b.bestChain.FindFork(node)
	err := b.db.View(func(dbTx database.Tx) error {
		for n := tip; n != nil && n != fork; n = n.parent {
			// No need to load nodes that are already loaded.
			prev := n.parent
			if prev == nil || prev.stakeNode != nil {
				continue
			}

			// Generate the previous stake node by starting with the child stake
			// node and undoing the modifications caused by the stake details in
			// the previous block.
			stakeNode, err := n.stakeNode.DisconnectNode(prev.lotteryIV(), nil,
				nil, dbTx)
			if err != nil {
				return err
			}
			prev.stakeNode = stakeNode
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Nothing more to do if the requested node is the fork point itself.
	if node == fork {
		return node.stakeNode, nil
	}

	// The requested node is on a side chain, so replay the effects of the
	// blocks up to the requested node per the above description.
	//
	// Note that the blocks between the fork point and the requested node are
	// added to the slice from back to front so that they are attached in the
	// appropriate order when iterating the slice.
	attachNodes := make([]*blockNode, node.height-fork.height)
	for n := node; n != nil && n != fork; n = n.parent {
		attachNodes[n.height-fork.height-1] = n
	}
	for _, n := range attachNodes {
		// No need to load nodes that are already loaded.
		if n.stakeNode != nil {
			continue
		}

		// Populate the prunable ticket information as needed.
		if err := b.maybeFetchTicketInfo(n); err != nil {
			return nil, err
		}

		// Generate the stake node by applying the stake details in the current
		// block to the previous stake node.
		stakeNode, err := n.parent.stakeNode.ConnectNode(n.lotteryIV(),
			n.ticketsVoted, n.ticketsRevoked, n.newTickets)
		if err != nil {
			return nil, err
		}
		n.stakeNode = stakeNode
	}

	return node.stakeNode, nil
}
