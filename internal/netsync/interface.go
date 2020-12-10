// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"github.com/decred/dcrd/blockchain/v4"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/internal/mempool"
	"github.com/decred/dcrd/peer/v2"
	"github.com/decred/dcrd/wire"
)

// PeerNotifier provides an interface to notify peers of status changes related
// to blocks and transactions.
type PeerNotifier interface {
	// AnnounceNewTransactions generates and relays inventory vectors and
	// notifies websocket clients of the passed transactions.
	AnnounceNewTransactions(txns []*dcrutil.Tx)

	// UpdatePeerHeights updates the heights of all peers who have announced the
	// latest connected main chain block, or a recognized orphan.
	UpdatePeerHeights(latestBlkHash *chainhash.Hash, latestHeight int64, updateSource *peer.Peer)
}

// SyncManager is a temporary interface to facilitate cleaner diffs when moving
// the block manager to a new package, so none of its members are documented.
// It will be removed in a future commit.
type SyncManager interface {
	NewPeer(p *peer.Peer)
	IsCurrent() bool
	TipGeneration() ([]chainhash.Hash, error)
	RequestFromPeer(p *peer.Peer, blocks, txs []*chainhash.Hash) error
	QueueTx(tx *dcrutil.Tx, peer *peer.Peer, done chan struct{})
	QueueBlock(block *dcrutil.Block, peer *peer.Peer, done chan struct{})
	QueueInv(inv *wire.MsgInv, peer *peer.Peer)
	QueueHeaders(headers *wire.MsgHeaders, peer *peer.Peer)
	QueueNotFound(notFound *wire.MsgNotFound, peer *peer.Peer)
	DonePeer(peer *peer.Peer)
	Start()
	Stop() error
	ForceReorganization(formerBest, newBest chainhash.Hash) error
	ProcessBlock(block *dcrutil.Block, flags blockchain.BehaviorFlags) (bool, error)
	SyncPeerID() int32
	SyncHeight() int64
	ProcessTransaction(tx *dcrutil.Tx, allowOrphans bool, rateLimit bool,
		allowHighFees bool, tag mempool.Tag) ([]*dcrutil.Tx, error)
}
