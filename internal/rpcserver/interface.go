// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcserver

import (
	"github.com/decred/dcrd/blockchain/v3"
	"github.com/decred/dcrd/blockchain/v3/indexers"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/peer/v2"
	"github.com/decred/dcrd/wire"
)

// Peer represents a peer for use with the RPC server.
//
// The interface contract requires that all of these methods are safe for
// concurrent access.
type Peer interface {
	// ToPeer returns the underlying peer instance.
	ToPeer() *peer.Peer

	// IsTxRelayDisabled returns whether or not the peer has disabled
	// transaction relay.
	IsTxRelayDisabled() bool

	// BanScore returns the current integer value that represents how close
	// the peer is to being banned.
	BanScore() uint32
}

// ConnManager represents a connection manager for use with the RPC server.
//
// The interface contract requires that all of these methods are safe for
// concurrent access.
type ConnManager interface {
	// Connect adds the provided address as a new outbound peer.  The
	// permanent flag indicates whether or not to make the peer persistent
	// and reconnect if the connection is lost.  Attempting to connect to an
	// already existing peer will return an error.
	Connect(addr string, permanent bool) error

	// RemoveByID removes the peer associated with the provided id from the
	// list of persistent peers.  Attempting to remove an id that does not
	// exist will return an error.
	RemoveByID(id int32) error

	// RemoveByAddr removes the peer associated with the provided address
	// from the list of persistent peers.  Attempting to remove an address
	// that does not exist will return an error.
	RemoveByAddr(addr string) error

	// DisconnectByID disconnects the peer associated with the provided id.
	// This applies to both inbound and outbound peers.  Attempting to
	// remove an id that does not exist will return an error.
	DisconnectByID(id int32) error

	// DisconnectByAddr disconnects the peer associated with the provided
	// address.  This applies to both inbound and outbound peers.
	// Attempting to remove an address that does not exist will return an
	// error.
	DisconnectByAddr(addr string) error

	// ConnectedCount returns the number of currently connected peers.
	ConnectedCount() int32

	// NetTotals returns the sum of all bytes received and sent across the
	// network for all peers.
	NetTotals() (uint64, uint64)

	// ConnectedPeers returns an array consisting of all connected peers.
	ConnectedPeers() []Peer

	// PersistentPeers returns an array consisting of all the persistent
	// peers.
	PersistentPeers() []Peer

	// BroadcastMessage sends the provided message to all currently
	// connected peers.
	BroadcastMessage(msg wire.Message)

	// AddRebroadcastInventory adds the provided inventory to the list of
	// inventories to be rebroadcast at random intervals until they show up
	// in a block.
	AddRebroadcastInventory(iv *wire.InvVect, data interface{})

	// RelayTransactions generates and relays inventory vectors for all of
	// the passed transactions to all connected peers.
	RelayTransactions(txns []*dcrutil.Tx)

	// AddedNodeInfo returns information describing persistent (added) nodes.
	AddedNodeInfo() []Peer
}

// SyncManager represents a sync manager for use with the RPC server.
//
// The interface contract requires that all of these methods are safe for
// concurrent access.
type SyncManager interface {
	// IsCurrent returns whether or not the sync manager believes the chain
	// is current as compared to the rest of the network.
	IsCurrent() bool

	// SubmitBlock submits the provided block to the network after
	// processing it locally.
	SubmitBlock(block *dcrutil.Block, flags blockchain.BehaviorFlags) (bool, error)

	// SyncPeerID returns the id of the current peer being synced with.
	SyncPeerID() int32

	// LocateBlocks returns the hashes of the blocks after the first known block
	// in the locator until the provided stop hash is reached, or up to the
	// provided max number of block hashes.
	LocateBlocks(locator blockchain.BlockLocator, hashStop *chainhash.Hash,
		maxHashes uint32) []chainhash.Hash

	// ExistsAddrIndex returns the address index.
	ExistsAddrIndex() *indexers.ExistsAddrIndex

	// CFIndex returns the committed filter (cf) by hash index.
	CFIndex() *indexers.CFIndex

	// TipGeneration returns the entire generation of blocks stemming from the
	// parent of the current tip.
	TipGeneration() ([]chainhash.Hash, error)

	// SyncHeight returns latest known block being synced to.
	SyncHeight() int64

	// ProcessTransaction relays the provided transaction validation and
	// insertion into the memory pool.
	ProcessTransaction(tx *dcrutil.Tx, allowOrphans bool, rateLimit bool,
		allowHighFees bool) ([]*dcrutil.Tx, error)
}
