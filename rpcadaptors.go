// Copyright (c) 2017 The btcsuite developers
// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/connmgr/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/internal/blockchain"
	"github.com/decred/dcrd/internal/mempool"
	"github.com/decred/dcrd/internal/mining"
	"github.com/decred/dcrd/internal/mining/cpuminer"
	"github.com/decred/dcrd/internal/netsync"
	"github.com/decred/dcrd/internal/rpcserver"
	"github.com/decred/dcrd/mixing"
	"github.com/decred/dcrd/mixing/mixpool"
	"github.com/decred/dcrd/peer/v4"
	"github.com/decred/dcrd/wire"
)

// rpcPeer provides a peer for use with the RPC server and implements the
// rpcserver.Peer interface.
type rpcPeer serverPeer

// Ensure rpcPeer implements the rpcserver.Peer interface.
var _ rpcserver.Peer = (*rpcPeer)(nil)

// Addr returns the peer address.
//
// This function is safe for concurrent access and is part of the rpcserver.Peer
// interface implementation.
func (p *rpcPeer) Addr() string {
	return (*serverPeer)(p).Peer.Addr()
}

// Connected returns whether or not the peer is currently connected.
//
// This function is safe for concurrent access and is part of the rpcserver.Peer
// interface implementation.
func (p *rpcPeer) Connected() bool {
	return (*serverPeer)(p).Peer.Connected()
}

// ID returns the peer id.
//
// This function is safe for concurrent access and is part of the rpcserver.Peer
// interface implementation.
func (p *rpcPeer) ID() int32 {
	return (*serverPeer)(p).Peer.ID()
}

// Inbound returns whether the peer is inbound.
//
// This function is safe for concurrent access and is part of the rpcserver.Peer
// interface implementation.
func (p *rpcPeer) Inbound() bool {
	return (*serverPeer)(p).Peer.Inbound()
}

// StatsSnapshot returns a snapshot of the current peer flags and statistics.
//
// This function is safe for concurrent access and is part of the rpcserver.Peer
// interface implementation.
func (p *rpcPeer) StatsSnapshot() *peer.StatsSnap {
	return (*serverPeer)(p).Peer.StatsSnapshot()
}

// LocalAddr returns the local address of the connection or nil if the peer is
// not currently connected.
//
// This function is safe for concurrent access and is part of the rpcserver.Peer
// interface implementation.
func (p *rpcPeer) LocalAddr() net.Addr {
	return (*serverPeer)(p).Peer.LocalAddr()
}

// LastPingNonce returns the last ping nonce of the remote peer.
//
// This function is safe for concurrent access and is part of the rpcserver.Peer
// interface implementation.
func (p *rpcPeer) LastPingNonce() uint64 {
	return (*serverPeer)(p).Peer.LastPingNonce()
}

// IsTxRelayDisabled returns whether or not the peer has disabled transaction
// relay.
//
// This function is safe for concurrent access and is part of the rpcserver.Peer
// interface implementation.
func (p *rpcPeer) IsTxRelayDisabled() bool {
	return (*serverPeer)(p).disableRelayTx.Load()
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
var _ rpcserver.ConnManager = (*rpcConnManager)(nil)

// Connect adds the provided address as a new outbound peer.  The permanent flag
// indicates whether or not to make the peer persistent and reconnect if the
// connection is lost.  Attempting to connect to an already existing peer will
// return an error.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) Connect(addr string, permanent bool) error {
	// Prevent duplicate connections to the same peer.
	connManager := cm.server.connManager
	err := connManager.ForEachConnReq(func(c *connmgr.ConnReq) error {
		if c.Addr != nil && c.Addr.String() == addr {
			if c.Permanent {
				return errors.New("peer exists as a permanent peer")
			}

			switch c.State() {
			case connmgr.ConnPending:
				return errors.New("peer pending connection")
			case connmgr.ConnEstablished:
				return errors.New("peer already connected")

			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	netAddr, err := addrStringToNetAddr(addr)
	if err != nil {
		return err
	}

	// Limit max number of total peers.
	cm.server.peerState.Lock()
	count := cm.server.peerState.count()
	cm.server.peerState.Unlock()
	if count >= cfg.MaxPeers {
		return errors.New("max peers reached")
	}

	go connManager.Connect(context.Background(), &connmgr.ConnReq{
		Addr:      netAddr,
		Permanent: permanent,
	})
	return nil
}

// removeNode removes any peers that the provided compare function return true
// for from the list of persistent peers.
//
// An error will be returned if no matching peers are found (aka the compare
// function returns false for all peers).
func (cm *rpcConnManager) removeNode(cmp func(*serverPeer) bool) error {
	state := &cm.server.peerState
	state.Lock()
	found := disconnectPeer(state.persistentPeers, cmp, func(sp *serverPeer) {
		// Update the group counts since the peer will be removed from the
		// persistent peers just after this func returns.
		remoteAddr := sp.NA()
		state.outboundGroups[remoteAddr.GroupKey()]--

		connReq := sp.connReq.Load()
		peerLog.Debugf("Removing persistent peer %s (reqid %d)", remoteAddr,
			connReq.ID())

		// Mark the peer's connReq as nil to prevent it from scheduling a
		// re-connect attempt.
		sp.connReq.Store(nil)
		cm.server.connManager.Remove(connReq.ID())
	})
	state.Unlock()

	if !found {
		return errors.New("peer not found")
	}
	return nil
}

// RemoveByID removes the peer associated with the provided id from the list of
// persistent peers.  Attempting to remove an id that does not exist will return
// an error.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) RemoveByID(id int32) error {
	cmp := func(sp *serverPeer) bool { return sp.ID() == id }
	return cm.removeNode(cmp)
}

// RemoveByAddr removes the peer associated with the provided address from the
// list of persistent peers.  Attempting to remove an address that does not
// exist will return an error.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) RemoveByAddr(addr string) error {
	cmp := func(sp *serverPeer) bool { return sp.Addr() == addr }
	err := cm.removeNode(cmp)
	if err != nil {
		netAddr, err := addrStringToNetAddr(addr)
		if err != nil {
			return err
		}
		return cm.server.connManager.CancelPending(netAddr)
	}
	return nil
}

// disconnectNode disconnects any peers that the provided compare function
// returns true for.  It applies to both inbound and outbound peers.
//
// An error will be returned if no matching peers are found (aka the compare
// function returns false for all peers).
//
// This function is safe for concurrent access.
func (cm *rpcConnManager) disconnectNode(cmp func(sp *serverPeer) bool) error {
	state := &cm.server.peerState
	defer state.Unlock()
	state.Lock()

	// Check inbound peers.  No callback is passed since there are no additional
	// actions on disconnect for inbound peers.
	found := disconnectPeer(state.inboundPeers, cmp, nil)
	if found {
		return nil
	}

	// Check outbound peers in a loop to ensure all outbound connections to the
	// same ip:port are disconnected when there are multiple.
	var numFound uint32
	for ; ; numFound++ {
		found = disconnectPeer(state.outboundPeers, cmp, func(sp *serverPeer) {
			// Update the group counts since the peer will be removed from the
			// persistent peers just after this func returns.
			remoteAddr := sp.NA()
			state.outboundGroups[remoteAddr.GroupKey()]--
		})
		if !found {
			break
		}
	}

	if numFound == 0 {
		return errors.New("peer not found")
	}
	return nil
}

// DisconnectByID disconnects the peer associated with the provided id.  This
// applies to both inbound and outbound peers.  Attempting to remove an id that
// does not exist will return an error.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) DisconnectByID(id int32) error {
	cmp := func(sp *serverPeer) bool { return sp.ID() == id }
	return cm.disconnectNode(cmp)
}

// DisconnectByAddr disconnects the peer associated with the provided address.
// This applies to both inbound and outbound peers.  Attempting to remove an
// address that does not exist will return an error.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) DisconnectByAddr(addr string) error {
	cmp := func(sp *serverPeer) bool { return sp.Addr() == addr }
	return cm.disconnectNode(cmp)
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
	state := &cm.server.peerState
	state.Lock()
	peers := make([]rpcserver.Peer, 0, state.count())
	state.forAllPeers(func(sp *serverPeer) {
		if !sp.Connected() {
			return
		}
		peers = append(peers, (*rpcPeer)(sp))
	})
	state.Unlock()
	return peers
}

// PersistentPeers returns an array consisting of all the added persistent
// peers.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) PersistentPeers() []rpcserver.Peer {
	// Return a slice of the relevant peers converted to RPC server peers.
	state := &cm.server.peerState
	state.Lock()
	peers := make([]rpcserver.Peer, 0, len(state.persistentPeers))
	for _, sp := range state.persistentPeers {
		peers = append(peers, (*rpcPeer)(sp))
	}
	state.Unlock()
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
func (cm *rpcConnManager) AddRebroadcastInventory(iv *wire.InvVect, data any) {
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

// RelayMixMessages generates and relays inventory vectors for all of the
// passed mixing messages to all connected peers.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (cm *rpcConnManager) RelayMixMessages(msgs []mixing.Message) {
	cm.server.relayMixMessages(msgs)
}

// Lookup defines the DNS lookup function to be used.
//
// This function is safe for concurrent access and is part of the
// rpcserver.ConnManager interface implementation.
func (*rpcConnManager) Lookup(host string) ([]net.IP, error) {
	return dcrdLookup(host)
}

// rpcSyncMgr provides an adaptor for use with the RPC server and implements the
// rpcserver.SyncManager interface.
type rpcSyncMgr struct {
	server  *server
	syncMgr *netsync.SyncManager
}

// Ensure rpcSyncMgr implements the rpcserver.SyncManager interface.
var _ rpcserver.SyncManager = (*rpcSyncMgr)(nil)

// IsCurrent returns whether or not the net sync manager believes the chain is
// current as compared to the rest of the network.
//
// This function is safe for concurrent access and is part of the
// rpcserver.SyncManager interface implementation.
func (b *rpcSyncMgr) IsCurrent() bool {
	return b.syncMgr.IsCurrent()
}

// SubmitBlock submits the provided block to the network after processing it
// locally.
//
// This function is safe for concurrent access and is part of the
// rpcserver.SyncManager interface implementation.
func (b *rpcSyncMgr) SubmitBlock(block *dcrutil.Block) error {
	return b.syncMgr.ProcessBlock(block)
}

// SyncPeer returns the id of the current peer being synced with.
//
// This function is safe for concurrent access and is part of the
// rpcserver.SyncManager interface implementation.
func (b *rpcSyncMgr) SyncPeerID() int32 {
	return b.syncMgr.SyncPeerID()
}

// SyncHeight returns latest known block being synced to.
func (b *rpcSyncMgr) SyncHeight() int64 {
	return b.syncMgr.SyncHeight()
}

// ProcessTransaction relays the provided transaction validation and insertion
// into the memory pool.
func (b *rpcSyncMgr) ProcessTransaction(tx *dcrutil.Tx, allowOrphans bool,
	allowHighFees bool, tag mempool.Tag) ([]*dcrutil.Tx, error) {

	return b.server.txMemPool.ProcessTransaction(tx, allowOrphans,
		allowHighFees, tag)
}

// RecentlyConfirmedTxn returns with high degree of confidence whether a
// transaction has been recently confirmed in a block.
//
// This method may report a false positive, but never a false negative.
func (b *rpcSyncMgr) RecentlyConfirmedTxn(hash *chainhash.Hash) bool {
	return b.server.recentlyConfirmedTxns.Contains(hash[:])
}

// AcceptMixMessage attempts to accept a mixing message to the local mixing
// pool.
func (b *rpcSyncMgr) AcceptMixMessage(msg mixing.Message, src mixpool.Source) error {
	_, err := b.server.mixMsgPool.AcceptMessage(msg, src)
	return err
}

// rpcUtxoEntry represents a utxo entry for use with the RPC server and
// implements the rpcserver.UtxoEntry interface.
type rpcUtxoEntry struct {
	*blockchain.UtxoEntry
}

// Ensure rpcUtxoEntry implements the rpcserver.UtxoEntry interface.
var _ rpcserver.UtxoEntry = (*rpcUtxoEntry)(nil)

// ToUtxoEntry returns the underlying UtxoEntry instance.
func (u *rpcUtxoEntry) ToUtxoEntry() *blockchain.UtxoEntry {
	return u.UtxoEntry
}

// rpcChain provides a chain for use with the RPC server and
// implements the rpcserver.Chain interface.
type rpcChain struct {
	*blockchain.BlockChain
}

// Ensure rpcChain implements the rpcserver.Chain interface.
var _ rpcserver.Chain = (*rpcChain)(nil)

// FetchUtxoEntry loads and returns the requested unspent transaction output
// from the point of view of the main chain tip.
//
// NOTE: Requesting an output for which there is no data will NOT return an
// error.  Instead both the entry and the error will be nil.  This is done to
// allow pruning of spent transaction outputs.  In practice this means the
// caller must check if the returned entry is nil before invoking methods on it.
//
// This function is safe for concurrent access however the returned entry (if
// any) is NOT.
func (c *rpcChain) FetchUtxoEntry(outpoint wire.OutPoint) (rpcserver.UtxoEntry, error) {
	utxo, err := c.BlockChain.FetchUtxoEntry(outpoint)
	if utxo == nil || err != nil {
		return nil, err
	}
	return &rpcUtxoEntry{UtxoEntry: utxo}, nil
}

// rpcClock provides a clock for use with the RPC server and
// implements the rpcserver.Clock interface.
type rpcClock struct{}

// Ensure rpcClock implements the rpcserver.Clock interface.
var _ rpcserver.Clock = (*rpcClock)(nil)

// Now returns the current local time.
//
// This function is safe for concurrent access and is part of the
// rpcserver.Clock interface implementation.
func (*rpcClock) Now() time.Time {
	return time.Now()
}

// Since returns the time elapsed since t.
//
// This function is safe for concurrent access and is part of the
// rpcserver.Clock interface implementation.
func (*rpcClock) Since(t time.Time) time.Duration {
	return time.Since(t)
}

// rpcLogManager provides a log manager for use with the RPC server and
// implements the rpcserver.LogManager interface.
type rpcLogManager struct{}

// Ensure rpcLogManager implements the rpcserver.LogManager interface.
var _ rpcserver.LogManager = (*rpcLogManager)(nil)

// SupportedSubsystems returns a sorted slice of the supported subsystems for
// logging purposes.
//
// This function is part of the rpcserver.LogManager interface implementation.
func (*rpcLogManager) SupportedSubsystems() []string {
	return supportedSubsystems()
}

// ParseAndSetDebugLevels attempts to parse the specified debug level and set
// the levels accordingly.  An appropriate error is returned if anything is
// invalid.
//
// This function is part of the rpcserver.LogManager interface implementation.
func (*rpcLogManager) ParseAndSetDebugLevels(debugLevel string) error {
	return parseAndSetDebugLevels(debugLevel)
}

// rpcSanityChecker provides a block sanity checker for use with the RPC and
// implements the rpcserver.SanityChecker interface.
type rpcSanityChecker struct {
	chain       *blockchain.BlockChain
	timeSource  blockchain.MedianTimeSource
	chainParams *chaincfg.Params
}

// Ensure rpcSanityChecker implements the rpcserver.SanityChecker interface.
var _ rpcserver.SanityChecker = (*rpcSanityChecker)(nil)

// CheckBlockSanity checks the correctness of the provided block
// per consensus.  An appropriate error is returned if anything is
// invalid.
//
// This function is part of the rpcserver.SanityChecker interface implementation.
func (s *rpcSanityChecker) CheckBlockSanity(block *dcrutil.Block) error {
	return blockchain.CheckBlockSanity(block, s.timeSource, s.chainParams)
}

// rpcBlockTemplater provides a block template generator for use with the
// RPC server and implements the rpcserver.BlockTemplater interface.
type rpcBlockTemplater struct {
	*mining.BgBlkTmplGenerator
}

// Ensure rpcBlockTemplater implements the rpcserver.BlockTemplater interface.
var _ rpcserver.BlockTemplater = (*rpcBlockTemplater)(nil)

// Subscribe returns a TemplateSubber which has functions to retrieve
// a channel that produces the stream of block templates and to stop
// the stream when the caller no longer wishes to receive new templates.
func (t *rpcBlockTemplater) Subscribe() rpcserver.TemplateSubber {
	return t.BgBlkTmplGenerator.Subscribe()
}

// rpcCPUMiner provides a CPU miner for use with the RPC and implements the
// rpcserver.CPUMiner interface.
type rpcCPUMiner struct {
	miner *cpuminer.CPUMiner
}

// Ensure rpcCPUMiner implements the rpcserver.CPUMiner interface.
var _ rpcserver.CPUMiner = (*rpcCPUMiner)(nil)

// GenerateNBlocks generates the requested number of blocks.
func (c *rpcCPUMiner) GenerateNBlocks(ctx context.Context, n uint32) ([]chainhash.Hash, error) {
	if c.miner == nil {
		return nil, errors.New("Block generation is disallowed without a " +
			"CPU miner.")
	}

	return c.miner.GenerateNBlocks(ctx, n)
}

// IsMining returns whether or not the CPU miner has been started and is
// therefore currently mining.
func (c *rpcCPUMiner) IsMining() bool {
	if c.miner == nil {
		return false
	}

	return c.miner.IsMining()
}

// HashesPerSecond returns the number of hashes per second the mining process
// is performing.
func (c *rpcCPUMiner) HashesPerSecond() float64 {
	if c.miner == nil {
		return 0
	}

	return c.miner.HashesPerSecond()
}

// NumWorkers returns the number of workers which are running to solve blocks.
func (c *rpcCPUMiner) NumWorkers() int32 {
	if c.miner == nil {
		return 0
	}

	return c.miner.NumWorkers()
}

// SetNumWorkers sets the number of workers to create which solve blocks.
func (c *rpcCPUMiner) SetNumWorkers(numWorkers int32) {
	if c.miner != nil {
		c.miner.SetNumWorkers(numWorkers)
	}
}
