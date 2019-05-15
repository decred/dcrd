// Copyright (c) 2017 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/indexers"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/peer"
	"github.com/decred/dcrd/wire"
)

// rpcPeer provides a peer for use with the RPC server and implements the
// rpcserverPeer interface.
type rpcPeer serverPeer

// Ensure rpcPeer implements the rpcserverPeer interface.
var _ rpcserverPeer = (*rpcPeer)(nil)

// ToPeer returns the underlying peer instance.
//
// This function is safe for concurrent access and is part of the rpcserverPeer
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
// This function is safe for concurrent access and is part of the rpcserverPeer
// interface implementation.
func (p *rpcPeer) IsTxRelayDisabled() bool {
	return (*serverPeer)(p).disableRelayTx
}

// BanScore returns the current integer value that represents how close the peer
// is to being banned.
//
// This function is safe for concurrent access and is part of the rpcserverPeer
// interface implementation.
func (p *rpcPeer) BanScore() uint32 {
	return (*serverPeer)(p).banScore.Int()
}

// rpcConnManager provides a connection manager for use with the RPC server and
// implements the rpcserverConnManager interface.
type rpcConnManager struct {
	Query             chan interface{}
	PeerCount         func() int32
	NetworkTotals     func() (uint64, uint64)
	BroadcastMsg      func(msg wire.Message, excluded ...*serverPeer)
	AddRebroadcastInv func(iv *wire.InvVect, data interface{})
	RelayTxns         func(txns []*dcrutil.Tx)
	NodeInfo          func() []*serverPeer
}

// Ensure rpcConnManager implements the rpcserverConnManager interface.
var _ rpcserverConnManager = &rpcConnManager{}

// Connect adds the provided address as a new outbound peer.  The permanent flag
// indicates whether or not to make the peer persistent and reconnect if the
// connection is lost.  Attempting to connect to an already existing peer will
// return an error.
//
// This function is safe for concurrent access and is part of the
// rpcserverConnManager interface implementation.
func (cm *rpcConnManager) Connect(addr string, permanent bool) error {
	replyChan := make(chan error)
	cm.Query <- connectNodeMsg{
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
// rpcserverConnManager interface implementation.
func (cm *rpcConnManager) RemoveByID(id int32) error {
	replyChan := make(chan error)
	cm.Query <- removeNodeMsg{
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
// rpcserverConnManager interface implementation.
func (cm *rpcConnManager) RemoveByAddr(addr string) error {
	replyChan := make(chan error)
	cm.Query <- removeNodeMsg{
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
// rpcserverConnManager interface implementation.
func (cm *rpcConnManager) DisconnectByID(id int32) error {
	replyChan := make(chan error)
	cm.Query <- disconnectNodeMsg{
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
// rpcserverConnManager interface implementation.
func (cm *rpcConnManager) DisconnectByAddr(addr string) error {
	replyChan := make(chan error)
	cm.Query <- disconnectNodeMsg{
		cmp:   func(sp *serverPeer) bool { return sp.Addr() == addr },
		reply: replyChan,
	}
	return <-replyChan
}

// ConnectedCount returns the number of currently connected peers.
//
// This function is safe for concurrent access and is part of the
// rpcserverConnManager interface implementation.
func (cm *rpcConnManager) ConnectedCount() int32 {
	return cm.PeerCount()
}

// NetTotals returns the sum of all bytes received and sent across the network
// for all peers.
//
// This function is safe for concurrent access and is part of the
// rpcserverConnManager interface implementation.
func (cm *rpcConnManager) NetTotals() (uint64, uint64) {
	return cm.NetworkTotals()
}

// ConnectedPeers returns an array consisting of all connected peers.
//
// This function is safe for concurrent access and is part of the
// rpcserverConnManager interface implementation.
func (cm *rpcConnManager) ConnectedPeers() []rpcserverPeer {
	replyChan := make(chan []*serverPeer)
	cm.Query <- getPeersMsg{reply: replyChan}
	serverPeers := <-replyChan

	// Convert to RPC server peers.
	peers := make([]rpcserverPeer, 0, len(serverPeers))
	for _, sp := range serverPeers {
		peers = append(peers, (*rpcPeer)(sp))
	}
	return peers
}

// PersistentPeers returns an array consisting of all the added persistent
// peers.
//
// This function is safe for concurrent access and is part of the
// rpcserverConnManager interface implementation.
func (cm *rpcConnManager) PersistentPeers() []rpcserverPeer {
	replyChan := make(chan []*serverPeer)
	cm.Query <- getAddedNodesMsg{reply: replyChan}
	serverPeers := <-replyChan

	// Convert to generic peers.
	peers := make([]rpcserverPeer, 0, len(serverPeers))
	for _, sp := range serverPeers {
		peers = append(peers, (*rpcPeer)(sp))
	}
	return peers
}

// BroadcastMessage sends the provided message to all currently connected peers.
//
// This function is safe for concurrent access and is part of the
// rpcserverConnManager interface implementation.
func (cm *rpcConnManager) BroadcastMessage(msg wire.Message, excluded ...*serverPeer) {
	cm.BroadcastMsg(msg, excluded...)
}

// AddRebroadcastInventory adds the provided inventory to the list of
// inventories to be rebroadcast at random intervals until they show up in a
// block.
//
// This function is safe for concurrent access and is part of the
// rpcserverConnManager interface implementation.
func (cm *rpcConnManager) AddRebroadcastInventory(iv *wire.InvVect, data interface{}) {
	cm.AddRebroadcastInv(iv, data)
}

// RelayTransactions generates and relays inventory vectors for all of the
// passed transactions to all connected peers.
func (cm *rpcConnManager) RelayTransactions(txns []*dcrutil.Tx) {
	cm.RelayTxns(txns)
}

// AddedNodeInfo returns information describing persistent (added) nodes.
func (cm *rpcConnManager) AddedNodeInfo() []*serverPeer {
	return cm.NodeInfo()
}

// rpcSyncMgr provides a block manager for use with the RPC server and
// implements the rpcserverSyncManager interface.
type rpcSyncMgr struct {
	IsCurrentImpl          func() bool
	SubmitBlockImpl        func(block *dcrutil.Block, flags blockchain.BehaviorFlags) (bool, error)
	SyncPeerIDImpl         func() int32
	LocateBlocksImpl       func(locator blockchain.BlockLocator, hashStop *chainhash.Hash, maxHashes uint32) []chainhash.Hash
	ExistsAddrIndexImpl    *indexers.ExistsAddrIndex
	CFIndexImpl            *indexers.CFIndex
	TipGenerationImpl      func() ([]chainhash.Hash, error)
	SyncHeightImpl         func() int64
	ProcessTransactionImpl func(tx *dcrutil.Tx, allowOrphans bool, rateLimit bool, allowHighFees bool) ([]*dcrutil.Tx, error)
}

// Ensure rpcSyncMgr implements the rpcserverSyncManager interface.
var _ rpcserverSyncManager = (*rpcSyncMgr)(nil)

// IsCurrent returns whether or not the sync manager believes the chain is
// current as compared to the rest of the network.
//
// This function is safe for concurrent access and is part of the
// rpcserverSyncManager interface implementation.
func (b *rpcSyncMgr) IsCurrent() bool {
	return b.IsCurrentImpl()
}

// SubmitBlock submits the provided block to the network after processing it
// locally.
//
// This function is safe for concurrent access and is part of the
// rpcserverSyncManager interface implementation.
func (b *rpcSyncMgr) SubmitBlock(block *dcrutil.Block, flags blockchain.BehaviorFlags) (bool, error) {
	return b.SubmitBlockImpl(block, flags)
}

// Pause pauses the sync manager until the returned channel is closed.
//
// This function is safe for concurrent access and is part of the
// rpcserverSyncManager interface implementation.
// func (b *rpcSyncMgr) Pause() chan<- struct{} {
// 	return b.blockMgr.Pause()
// }

// SyncPeerID returns the peer that is currently the peer being used to sync
// from.
//
// This function is safe for concurrent access and is part of the
// rpcserverSyncManager interface implementation.
func (b *rpcSyncMgr) SyncPeerID() int32 {
	return b.SyncPeerIDImpl()
}

// LocateBlocks returns the hashes of the blocks after the first known block in
// the locator until the provided stop hash is reached, or up to the provided
// max number of block hashes.
//
// This function is safe for concurrent access and is part of the
// rpcserverSyncManager interface implementation.
func (b *rpcSyncMgr) LocateBlocks(locator blockchain.BlockLocator, hashStop *chainhash.Hash, maxHashes uint32) []chainhash.Hash {
	return b.LocateBlocksImpl(locator, hashStop, maxHashes)
}

// ExistsAddrIndex returns the address index.
//
// This function is safe for concurrent access and is part of the
// rpcserverSyncManager interface implementation.
func (b *rpcSyncMgr) ExistsAddrIndex() *indexers.ExistsAddrIndex {
	return b.ExistsAddrIndexImpl
}

// CFIndex returns the committed filter (cf) by hash index.
//
// This function is safe for concurrent access and is part of the
// rpcserverSyncManager interface implementation.
func (b *rpcSyncMgr) CFIndex() *indexers.CFIndex {
	return b.CFIndexImpl
}

// TipGeneration returns the entire generation of blocks stemming from the
// parent of the current tip.
func (b *rpcSyncMgr) TipGeneration() ([]chainhash.Hash, error) {
	return b.TipGenerationImpl()
}

// SyncHeight returns latest known block being synced to.
func (b *rpcSyncMgr) SyncHeight() int64 {
	return b.SyncHeightImpl()
}

// ProcessTransaction relays the provided transaction validation and insertion
// into the memory pool.
func (b *rpcSyncMgr) ProcessTransaction(tx *dcrutil.Tx, allowOrphans bool,
	rateLimit bool, allowHighFees bool) ([]*dcrutil.Tx, error) {
	return b.ProcessTransactionImpl(tx, allowOrphans, rateLimit, allowHighFees)
}
