// Copyright (c) 2017 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"github.com/decred/dcrd/blockchain/v3"
	"github.com/decred/dcrd/blockchain/v3/indexers"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/internal/rpcserver"
	"github.com/decred/dcrd/peer/v2"
	"github.com/decred/dcrd/wire"
)

// rpcPeer provides a peer for use with the RPC server and implements the
// rpcserver.Peer interface.
type rpcPeer serverPeer

// Ensure rpcPeer implements the rpcserver.Peer interface.
var _ rpcserver.Peer = (*rpcPeer)(nil)

// ToPeer returns the underlying peer instance.
//
// This function is safe for concurrent access and is part of the rpcserver.Peer
// interface implementation.
func (p *rpcPeer) ToPeer() *peer.Peer {
	if p == nil {
		return nil
	}
	return (*serverPeer)(p).Peer
}

// IsTxRelayDisabled returns whether or not the peer has disabled transaction
// relay.
//
// This function is safe for concurrent access and is part of the rpcserver.Peer
// interface implementation.
func (p *rpcPeer) IsTxRelayDisabled() bool {
	return (*serverPeer)(p).relayTxDisabled()
}

// BanScore returns the current integer value that represents how close the peer
// is to being banned.
//
// This function is safe for concurrent access and is part of the rpcserver.Peer
// interface implementation.
func (p *rpcPeer) BanScore() uint32 {
	return (*serverPeer)(p).banScore.Int()
}

// rpcConnManager provides a connection manager for use with the RPC server and
// implements the rpcserver.ConnManager interface.
type rpcConnManager struct {
	server *server
}

// Ensure rpcConnManager implements the rpcserver.ConnManager interface.
var _ rpcserver.ConnManager = &rpcConnManager{}

// Connect adds the provided address as a new outbound peer.  The permanent flag
// indicates whether or not to make the peer persistent and reconnect if the
// connection is lost.  Attempting to connect to an already existing peer will
// return an error.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) Connect(addr string, permanent bool) error {
	replyChan := make(chan error)
	cm.server.query <- connectNodeMsg{
		addr:      addr,
		permanent: permanent,
		reply:     replyChan,
	}
	return <-replyChan
}

// RemoveByID removes the peer associated with the provided id from the list of
// persistent peers.  Attempting to remove an id that does not exist will return
// an error.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) RemoveByID(id int32) error {
	replyChan := make(chan error)
	cm.server.query <- removeNodeMsg{
		cmp:   func(sp *serverPeer) bool { return sp.ID() == id },
		reply: replyChan,
	}
	return <-replyChan
}

// RemoveByAddr removes the peer associated with the provided address from the
// list of persistent peers.  Attempting to remove an address that does not
// exist will return an error.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) RemoveByAddr(addr string) error {
	replyChan := make(chan error)
	cm.server.query <- removeNodeMsg{
		cmp:   func(sp *serverPeer) bool { return sp.Addr() == addr },
		reply: replyChan,
	}
	return <-replyChan
}

// DisconnectByID disconnects the peer associated with the provided id.  This
// applies to both inbound and outbound peers.  Attempting to remove an id that
// does not exist will return an error.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) DisconnectByID(id int32) error {
	replyChan := make(chan error)
	cm.server.query <- disconnectNodeMsg{
		cmp:   func(sp *serverPeer) bool { return sp.ID() == id },
		reply: replyChan,
	}
	return <-replyChan
}

// DisconnectByAddr disconnects the peer associated with the provided address.
// This applies to both inbound and outbound peers.  Attempting to remove an
// address that does not exist will return an error.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) DisconnectByAddr(addr string) error {
	replyChan := make(chan error)
	cm.server.query <- disconnectNodeMsg{
		cmp:   func(sp *serverPeer) bool { return sp.Addr() == addr },
		reply: replyChan,
	}
	return <-replyChan
}

// ConnectedCount returns the number of currently connected peers.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) ConnectedCount() int32 {
	return cm.server.ConnectedCount()
}

// NetTotals returns the sum of all bytes received and sent across the network
// for all peers.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) NetTotals() (uint64, uint64) {
	return cm.server.NetTotals()
}

// ConnectedPeers returns an array consisting of all connected peers.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) ConnectedPeers() []rpcserver.Peer {
	replyChan := make(chan []*serverPeer)
	cm.server.query <- getPeersMsg{reply: replyChan}
	serverPeers := <-replyChan

	// Convert to RPC server peers.
	peers := make([]rpcserver.Peer, 0, len(serverPeers))
	for _, sp := range serverPeers {
		peers = append(peers, (*rpcPeer)(sp))
	}
	return peers
}

// PersistentPeers returns an array consisting of all the added persistent
// peers.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) PersistentPeers() []rpcserver.Peer {
	replyChan := make(chan []*serverPeer)
	cm.server.query <- getAddedNodesMsg{reply: replyChan}
	serverPeers := <-replyChan

	// Convert to generic peers.
	peers := make([]rpcserver.Peer, 0, len(serverPeers))
	for _, sp := range serverPeers {
		peers = append(peers, (*rpcPeer)(sp))
	}
	return peers
}

// BroadcastMessage sends the provided message to all currently connected peers.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) BroadcastMessage(msg wire.Message) {
	cm.server.BroadcastMessage(msg)
}

// AddRebroadcastInventory adds the provided inventory to the list of
// inventories to be rebroadcast at random intervals until they show up in a
// block.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) AddRebroadcastInventory(iv *wire.InvVect, data interface{}) {
	cm.server.AddRebroadcastInventory(iv, data)
}

// RelayTransactions generates and relays inventory vectors for all of the
// passed transactions to all connected peers.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) RelayTransactions(txns []*dcrutil.Tx) {
	cm.server.relayTransactions(txns)
}

// AddedNodeInfo returns information describing persistent (added) nodes.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) AddedNodeInfo() []rpcserver.Peer {
	serverPeers := cm.server.AddedNodeInfo()

	// Convert to RPC server peers.
	peers := make([]rpcserver.Peer, 0, len(serverPeers))
	for _, sp := range serverPeers {
		peers = append(peers, (*rpcPeer)(sp))
	}

	return peers
}

// rpcSyncMgr provides a block manager for use with the RPC server and
// implements the rpcserver.SyncManager interface.
type rpcSyncMgr struct {
	server   *server
	blockMgr *blockManager
}

// Ensure rpcSyncMgr implements the rpcserver.SyncManager interface.
var _ rpcserver.SyncManager = (*rpcSyncMgr)(nil)

// IsCurrent returns whether or not the sync manager believes the chain is
// current as compared to the rest of the network.
//
// This function is safe for concurrent access and is part of the
// rpcserver.SyncManager interface implementation.
func (b *rpcSyncMgr) IsCurrent() bool {
	return b.blockMgr.IsCurrent()
}

// SubmitBlock submits the provided block to the network after processing it
// locally.
//
// This function is safe for concurrent access and is part of the
// rpcserver.SyncManager interface implementation.
func (b *rpcSyncMgr) SubmitBlock(block *dcrutil.Block, flags blockchain.BehaviorFlags) (bool, error) {
	return b.blockMgr.ProcessBlock(block, flags)
}

// SyncPeer returns the id of the current peer being synced with.
//
// This function is safe for concurrent access and is part of the
// rpcserver.SyncManager interface implementation.
func (b *rpcSyncMgr) SyncPeerID() int32 {
	return b.blockMgr.SyncPeerID()
}

// LocateBlocks returns the hashes of the blocks after the first known block in
// the locator until the provided stop hash is reached, or up to the provided
// max number of block hashes.
//
// This function is safe for concurrent access and is part of the
// rpcserver.SyncManager interface implementation.
func (b *rpcSyncMgr) LocateBlocks(locator blockchain.BlockLocator, hashStop *chainhash.Hash, maxHashes uint32) []chainhash.Hash {
	return b.server.chain.LocateBlocks(locator, hashStop, maxHashes)
}

// ExistsAddrIndex returns the address index.
//
// This function is safe for concurrent access and is part of the
// rpcserver.SyncManager interface implementation.
func (b *rpcSyncMgr) ExistsAddrIndex() *indexers.ExistsAddrIndex {
	return b.server.existsAddrIndex
}

// CFIndex returns the committed filter (cf) by hash index.
//
// This function is safe for concurrent access and is part of the
// rpcserver.SyncManager interface implementation.
func (b *rpcSyncMgr) CFIndex() *indexers.CFIndex {
	return b.server.cfIndex
}

// TipGeneration returns the entire generation of blocks stemming from the
// parent of the current tip.
func (b *rpcSyncMgr) TipGeneration() ([]chainhash.Hash, error) {
	return b.blockMgr.TipGeneration()
}

// SyncHeight returns latest known block being synced to.
func (b *rpcSyncMgr) SyncHeight() int64 {
	return b.blockMgr.SyncHeight()
}

// ProcessTransaction relays the provided transaction validation and insertion
// into the memory pool.
func (b *rpcSyncMgr) ProcessTransaction(tx *dcrutil.Tx, allowOrphans bool,
	rateLimit bool, allowHighFees bool) ([]*dcrutil.Tx, error) {
	return b.blockMgr.ProcessTransaction(tx, allowOrphans,
		rateLimit, allowHighFees)
}
