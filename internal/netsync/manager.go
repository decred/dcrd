// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/container/apbf"
	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/internal/blockchain"
	"github.com/decred/dcrd/internal/mempool"
	"github.com/decred/dcrd/internal/progresslog"
	"github.com/decred/dcrd/math/uint256"
	"github.com/decred/dcrd/mixing"
	"github.com/decred/dcrd/mixing/mixpool"
	peerpkg "github.com/decred/dcrd/peer/v3"
	"github.com/decred/dcrd/wire"
)

const (
	// minInFlightBlocks is the minimum number of blocks that should be
	// in the request queue before requesting more.
	minInFlightBlocks = 10

	// maxInFlightBlocks is the maximum number of blocks to allow in the sync
	// peer request queue.
	maxInFlightBlocks = 16

	// maxRejectedTxns specifies the maximum number of recently rejected
	// transactions to track.  This is primarily used to avoid wasting a bunch
	// of bandwidth from requesting transactions that are already known to be
	// invalid again from multiple peers, however, it also doubles as DoS
	// protection against malicious peers.
	//
	// Recall that there are 125 connection slots by default.  Assuming the
	// default setting of 8 outbound connections, which attackers cannot
	// control, that leaves 117 max inbound connections which could potentially
	// be malicious.  maxRejectedTxns is set to target tracking the maximum
	// number of rejected transactions that would result from 120 connections
	// with malicious peers.  120 is used since it is strictly greater than the
	// aforementioned 117 max inbound connections while still providing for the
	// possibility of a few happenstance malicious outbound connections as well.
	//
	// It's also worth noting that even if attackers were to manage to exceed
	// the configured value, the result is not catastrophic as it would only
	// result in increased bandwidth usage versus not exceeding it.
	//
	// rejectedTxnsFPRate is the false positive rate to use for the APBF used to
	// track recently rejected transactions.  It is set to a rate of 1 per 10
	// million to make it incredibly unlikely that any transactions that haven't
	// actually been rejected are incorrectly treated as if they had.
	//
	// These values result in about 568 KiB memory usage including overhead.
	maxRejectedTxns    = 62500
	rejectedTxnsFPRate = 0.0000001

	// maxRejectedMixMsgs specifies the maximum number of recently
	// rejected mixing messages to track, and rejectedMixMsgsFPRate is the
	// false positive rate for the APBF.  These values have not been tuned
	// specifically for the mixing messages, and the equivalent constants
	// for handling rejected transactions are used.
	maxRejectedMixMsgs    = maxRejectedTxns
	rejectedMixMsgsFPRate = rejectedTxnsFPRate

	// maxRequestedBlocks is the maximum number of requested block
	// hashes to store in memory.
	maxRequestedBlocks = wire.MaxInvPerMsg

	// maxRequestedTxns is the maximum number of requested transactions
	// hashes to store in memory.
	maxRequestedTxns = wire.MaxInvPerMsg

	// maxRequestedMixMsgs is the maximum number of hashes of in-flight
	// mixing messages.
	maxRequestedMixMsgs = wire.MaxInvPerMsg

	// maxExpectedHeaderAnnouncementsPerMsg is the maximum number of headers in
	// a single message that is expected when determining when the message
	// appears to be announcing new blocks.
	maxExpectedHeaderAnnouncementsPerMsg = 12

	// maxConsecutiveOrphanHeaders is the maximum number of consecutive header
	// messages that contain headers which do not connect a peer can send before
	// it is deemed to have diverged so far it is no longer useful.
	maxConsecutiveOrphanHeaders = 10

	// headerSyncStallTimeoutSecs is the number of seconds to wait for progress
	// during the header sync process before stalling the sync and disconnecting
	// the peer.
	headerSyncStallTimeoutSecs = (3 + wire.MaxBlockHeadersPerMsg/1000) * 2
)

// zeroHash is the zero value hash (all zeros).  It is defined as a convenience.
var zeroHash chainhash.Hash

// peerConnectedMsg signifies a newly connected peer to the event handler.
type peerConnectedMsg struct {
	peer *Peer
}

// blockMsg packages a Decred block message and the peer it came from together
// so the event handler has access to that information.
type blockMsg struct {
	block *dcrutil.Block
	peer  *Peer
	reply chan struct{}
}

// invMsg packages a Decred inv message and the peer it came from together
// so the event handler has access to that information.
type invMsg struct {
	inv  *wire.MsgInv
	peer *Peer
}

// headersMsg packages a Decred headers message and the peer it came from
// together so the event handler has access to that information.
type headersMsg struct {
	headers *wire.MsgHeaders
	peer    *Peer
}

// notFoundMsg packages a Decred notfound message and the peer it came from
// together so the event handler has access to that information.
type notFoundMsg struct {
	notFound *wire.MsgNotFound
	peer     *Peer
}

// peerDisconnectedMsg signifies a newly disconnected peer to the event handler.
type peerDisconnectedMsg struct {
	peer *Peer
}

// txMsg packages a Decred tx message and the peer it came from together
// so the event handler has access to that information.
type txMsg struct {
	tx    *dcrutil.Tx
	peer  *Peer
	reply chan struct{}
}

// getSyncPeerMsg is a message type to be sent across the message channel for
// retrieving the current sync peer.
type getSyncPeerMsg struct {
	reply chan int32
}

// requestFromPeerMsg is a message type to be sent across the message channel
// for requesting either blocks or transactions from a given peer. It routes
// this through the sync manager so the sync manager doesn't ban the peer
// when it sends this information back.
type requestFromPeerMsg struct {
	peer         *Peer
	blocks       []chainhash.Hash
	voteHashes   []chainhash.Hash
	tSpendHashes []chainhash.Hash
	mixHashes    []chainhash.Hash
	reply        chan requestFromPeerResponse
}

// requestFromPeerResponse is a response sent to the reply channel of a
// requestFromPeerMsg query.
type requestFromPeerResponse struct {
	err error
}

// processBlockResponse is a response sent to the reply channel of a
// processBlockMsg.
type processBlockResponse struct {
	forkLen int64
	err     error
}

// processBlockMsg is a message type to be sent across the message channel
// for requested a block is processed.  Note this call differs from blockMsg
// above in that blockMsg is intended for blocks that came from peers and have
// extra handling whereas this message essentially is just a concurrent safe
// way to call ProcessBlock on the internal block chain instance.
type processBlockMsg struct {
	block *dcrutil.Block
	reply chan processBlockResponse
}

// mixMsg is a message type to be sent across the message channel for requesting
// a message's acceptance to the mixing pool.
type mixMsg struct {
	msg   mixing.Message
	peer  *Peer
	reply chan error
}

// Peer extends a common peer to maintain additional state needed by the sync
// manager.  The internals are intentionally unexported to create an opaque
// type.
type Peer struct {
	*peerpkg.Peer

	syncCandidate    bool
	requestedTxns    map[chainhash.Hash]struct{}
	requestedBlocks  map[chainhash.Hash]struct{}
	requestedMixMsgs map[chainhash.Hash]struct{}

	// requestInitialStateOnce is used to ensure the initial state data is only
	// requested from the peer once.
	requestInitialStateOnce sync.Once

	// numConsecutiveOrphanHeaders tracks the number of consecutive header
	// messages sent by the peer that contain headers which do not connect.  It
	// is used to detect peers that have either diverged so far they are no
	// longer useful or are otherwise being malicious.
	numConsecutiveOrphanHeaders int32

	// These fields are used to track the best known block announced by the peer
	// which in turn provides a means to discover which blocks are available to
	// download from the peer.
	//
	// announcedOrphanBlock is the hash of the most recently announced block
	// that did not connect to any headers known to the local chain at the time
	// of the announcement.  It is tracked because such announcements are
	// typically for newly found blocks whose parent headers will eventually
	// become known and therefore have a fairly good chance of becoming the
	// block with the most cumulative proof of work that the peer has announced.
	//
	// bestAnnouncedBlock is the hash of the block with the most cumulative
	// proof of work that the peer has announced that is also known to the local
	// chain.
	//
	// bestAnnouncedWork is the cumulative proof of work for the associated best
	// announced block hash.
	announcedOrphanBlock *chainhash.Hash
	bestAnnouncedBlock   *chainhash.Hash
	bestAnnouncedWork    *uint256.Uint256
}

// NewPeer returns a new instance of a peer that wraps the provided underlying
// common peer with additional state that is used throughout the package.
func NewPeer(peer *peerpkg.Peer) *Peer {
	isSyncCandidate := peer.Services()&wire.SFNodeNetwork == wire.SFNodeNetwork
	return &Peer{
		Peer:             peer,
		syncCandidate:    isSyncCandidate,
		requestedTxns:    make(map[chainhash.Hash]struct{}),
		requestedBlocks:  make(map[chainhash.Hash]struct{}),
		requestedMixMsgs: make(map[chainhash.Hash]struct{}),
	}
}

// maybeRequestInitialState potentially requests initial state information from
// the peer by sending it an appropriate initial state sync message dependending
// on the protocol version.
//
// The request will not be sent more than once or when the peer is in the
// process of being removed.
//
// This function is safe for concurrent access.
func (peer *Peer) maybeRequestInitialState(includeMiningState bool) {
	// Don't request the initial state more than once or when the peer is in the
	// process of being removed.
	if !peer.Connected() {
		return
	}
	peer.requestInitialStateOnce.Do(func() {
		// Choose which initial state sync p2p messages to use based on the
		// protocol version.
		//
		// Protocol versions prior to the init state version use getminingstate
		// and miningstate while those after use getinitstate and initstate.
		if peer.ProtocolVersion() < wire.InitStateVersion {
			if includeMiningState {
				peer.QueueMessage(wire.NewMsgGetMiningState(), nil)
			}
			return
		}

		// Always request treasury spends for newer protocol versions.
		types := make([]string, 0, 3)
		types = append(types, wire.InitStateTSpends)
		if includeMiningState {
			types = append(types, wire.InitStateHeadBlocks)
			types = append(types, wire.InitStateHeadBlockVotes)
		}
		msg := wire.NewMsgGetInitState()
		if err := msg.AddTypes(types...); err != nil {
			log.Errorf("Failed to build getinitstate msg: %v", err)
			return
		}
		peer.QueueMessage(msg, nil)
	})
}

// headerSyncState houses the state used to track the header sync progress and
// related stall handling.
type headerSyncState struct {
	// headersSynced tracks whether or not the headers are synced to a point
	// that is recent enough to start downloading blocks.
	headersSynced bool

	// These fields are used to implement a progress stall timeout that can be
	// reset at any time without needing to create a new one and the associated
	// extra garbage.
	//
	// stallTimer is an underlying timer that is used to implement the timeout.
	//
	// stallChanDrained indicates whether or not the channel for the stall timer
	// has already been read and is used when resetting the timer to ensure the
	// channel is drained when the timer is stopped as described in the timer
	// documentation.
	stallTimer       *time.Timer
	stallChanDrained bool
}

// makeHeaderSyncState returns a header sync state that is ready to use.
func makeHeaderSyncState() headerSyncState {
	stallTimer := time.NewTimer(math.MaxInt64)
	stallTimer.Stop()
	return headerSyncState{
		stallTimer:       stallTimer,
		stallChanDrained: true,
	}
}

// stopStallTimeout stops the progress stall timer while ensuring to read from
// the timer's channel in the case the timer already expired which can happen
// due to the fact the stop happens in between channel reads.  This behavior is
// well documented in the Timer docs.
//
// NOTE: This function must not be called concurrent with any other receives on
// the timer's channel.
func (state *headerSyncState) stopStallTimeout() {
	t := state.stallTimer
	if !t.Stop() && !state.stallChanDrained {
		<-t.C
	}
	state.stallChanDrained = true
}

// resetStallTimeout resets the progress stall timer while ensuring to read from
// the timer's channel in the case the timer already expired which can happen
// due to the fact the reset happens in between channel reads.  This behavior is
// well documented in the Timer docs.
//
// NOTE: This function must not be called concurrent with any other receives on
// the timer's channel.
func (state *headerSyncState) resetStallTimeout() {
	state.stopStallTimeout()
	state.stallTimer.Reset(headerSyncStallTimeoutSecs * time.Second)
	state.stallChanDrained = false
}

// SyncManager provides a concurrency safe sync manager for handling all
// incoming blocks.
type SyncManager struct {
	// quit is used for lifecycle management of the sync manager.
	quit chan struct{}

	// cfg specifies the configuration of the sync manager and is set at
	// creation time and treated as immutable after that.
	cfg Config

	// minKnownWork houses the minimum known work from the associated network
	// params converted to a uint256 so the conversion only needs to be
	// performed once when the sync manager is initialized.  Ideally, the chain
	// params should be updated to use the new type, but that will be a major
	// version bump, so a one-time conversion is a good tradeoff in the mean
	// time.
	minKnownWork *uint256.Uint256

	rejectedTxns     *apbf.Filter
	rejectedMixMsgs  *apbf.Filter
	requestedTxns    map[chainhash.Hash]struct{}
	requestedBlocks  map[chainhash.Hash]*Peer
	requestedMixMsgs map[chainhash.Hash]struct{}
	progressLogger   *progresslog.Logger
	syncPeer         *Peer
	msgChan          chan interface{}
	peers            map[*Peer]struct{}

	// hdrSyncState houses the state used to track the initial header sync
	// process and related stall handling.
	hdrSyncState headerSyncState

	// The following fields are used to track the height being synced to from
	// peers.
	syncHeightMtx sync.Mutex
	syncHeight    int64

	// The following fields are used to track whether or not the manager
	// believes it is fully synced to the network.
	isCurrentMtx sync.Mutex
	isCurrent    bool

	// The following fields are used to track the list of the next blocks to
	// download in the branch leading up to the best known header.
	//
	// nextBlocksHeader is the hash of the best known header when the list was
	// last updated.
	//
	// nextBlocksBuf houses an overall list of blocks needed (up to the size of
	// the array) regardless of whether or not they have been requested and
	// provides what is effectively a reusable lookahead buffer.  Note that
	// since it is a fixed size and acts as a backing array, not all entries
	// will necessarily refer to valid data, especially once the chain is
	// synced.  nextNeededBlocks slices into the valid part of the array.
	//
	// nextNeededBlocks subslices into nextBlocksBuf such that it provides an
	// upper bound on the entries of the backing array that are valid and also
	// acts as a list of needed blocks that are not already known to be in
	// flight.
	nextBlocksHeader chainhash.Hash
	nextBlocksBuf    [512]chainhash.Hash
	nextNeededBlocks []chainhash.Hash
}

// SyncHeight returns latest known block being synced to.
func (m *SyncManager) SyncHeight() int64 {
	m.syncHeightMtx.Lock()
	syncHeight := m.syncHeight
	m.syncHeightMtx.Unlock()
	return syncHeight
}

// chainBlockLocatorToHashes converts a block locator from chain to a slice
// of hashes.
func chainBlockLocatorToHashes(locator blockchain.BlockLocator) []chainhash.Hash {
	if len(locator) == 0 {
		return nil
	}

	result := make([]chainhash.Hash, 0, len(locator))
	for _, hash := range locator {
		result = append(result, *hash)
	}
	return result
}

// maybeUpdateNextNeededBlocks potentially updates the list of the next blocks
// to download in the branch leading up to the best known header.
//
// This function is NOT safe for concurrent access.  It must be called from the
// event handler goroutine.
func (m *SyncManager) maybeUpdateNextNeededBlocks() {
	// Update the list if the best known header changed since the last time it
	// was updated or it is not empty, is getting short, and does not already
	// end at the best known header.
	chain := m.cfg.Chain
	bestHeader, _ := chain.BestHeader()
	numNeeded := len(m.nextNeededBlocks)
	needsUpdate := m.nextBlocksHeader != bestHeader || (numNeeded > 0 &&
		numNeeded < minInFlightBlocks &&
		m.nextNeededBlocks[numNeeded-1] != bestHeader)
	if needsUpdate {
		m.nextNeededBlocks = chain.PutNextNeededBlocks(m.nextBlocksBuf[:])
		m.nextBlocksHeader = bestHeader
	}
}

// isRequestedBlock returns whether or not the given block hash has already been
// requested from any remote peer.
//
// This function is NOT safe for concurrent access.  It must be called from the
// event handler goroutine.
func (m *SyncManager) isRequestedBlock(hash *chainhash.Hash) bool {
	_, ok := m.requestedBlocks[*hash]
	return ok
}

// fetchNextBlocks creates and sends a request to the provided peer for the next
// blocks to be downloaded based on the current headers.
func (m *SyncManager) fetchNextBlocks(peer *Peer) {
	// Nothing to do if the target maximum number of blocks to request from the
	// peer at the same time are already in flight.
	numInFlight := len(peer.requestedBlocks)
	if numInFlight >= maxInFlightBlocks {
		return
	}

	// Potentially update the list of the next blocks to download in the branch
	// leading up to the best known header.
	m.maybeUpdateNextNeededBlocks()

	// Build and send a getdata request for the needed blocks.
	numNeeded := len(m.nextNeededBlocks)
	if numNeeded == 0 {
		return
	}
	maxNeeded := maxInFlightBlocks - numInFlight
	if numNeeded > maxNeeded {
		numNeeded = maxNeeded
	}
	gdmsg := wire.NewMsgGetDataSizeHint(uint(numNeeded))
	for i := 0; i < numNeeded && len(gdmsg.InvList) < wire.MaxInvPerMsg; i++ {
		// The block is either going to be skipped because it has already been
		// requested or it will be requested, but in either case, the block is
		// no longer needed for future iterations.
		hash := &m.nextNeededBlocks[0]
		m.nextNeededBlocks = m.nextNeededBlocks[1:]

		// Skip blocks that have already been requested.  The needed blocks
		// might have been updated above thereby potentially repopulating some
		// blocks that are still in flight.
		if m.isRequestedBlock(hash) {
			continue
		}

		iv := wire.NewInvVect(wire.InvTypeBlock, hash)
		m.requestedBlocks[*hash] = peer
		peer.requestedBlocks[*hash] = struct{}{}
		gdmsg.AddInvVect(iv)
	}
	if len(gdmsg.InvList) > 0 {
		peer.QueueMessage(gdmsg, nil)
	}
}

// fetchNextHeaders requests headers from the provided peer starting from the
// parent of the best known header for the local chain in order to discover any
// blocks that are not already known as well as accurately discover the best
// known block of the remote peer.
//
// Note that the parent is used because the request would otherwise result in an
// empty response when both the local and remote tips are the same.
//
// This function is safe for concurrent access.
func (m *SyncManager) fetchNextHeaders(peer *Peer) {
	chain := m.cfg.Chain
	parentHash, _ := chain.BestHeader()
	header, err := chain.HeaderByHash(&parentHash)
	if err == nil {
		parentHash = header.PrevBlock
	}
	blkLocator := chain.BlockLocatorFromHash(&parentHash)
	locator := chainBlockLocatorToHashes(blkLocator)
	peer.PushGetHeadersMsg(locator, &zeroHash)
}

// updateSyncPeerState updates the sync peer to be the peer with the highest
// known block height, and also marks peers as non sync candidates if their
// chain heights have been surpassed.
func (m *SyncManager) updateSyncPeerState(bestHeight int64) {
	// Determine the best sync peer.
	var bestPeer *Peer
	for peer := range m.peers {
		if !peer.syncCandidate {
			continue
		}

		// Remove sync candidate peers that are no longer candidates due to
		// passing their latest known block.  NOTE: The < is intentional as
		// opposed to <=.  While technically the peer doesn't have a later block
		// when it's equal, it will likely have one soon so it is a reasonable
		// choice.  It also allows the case where both are at 0 such as during
		// regression test.
		if peer.LastBlock() < bestHeight {
			peer.syncCandidate = false
			continue
		}

		// The best sync candidate is the most updated peer.
		if bestPeer == nil || bestPeer.LastBlock() < peer.LastBlock() {
			bestPeer = peer
		}
	}
	m.syncPeer = bestPeer
}

// startInitialHeaderSync starts the initial header sync process which consists
// of downloading all headers that are not already known from the sync peer. As
// the name implies, this assumes the initial header sync process is not already
// done.
func (m *SyncManager) startInitialHeaderSync(bestHeight int64) {
	hdrSyncPeer := m.syncPeer
	syncHeight := hdrSyncPeer.LastBlock()
	log.Infof("Syncing headers to block height %d from peer %v", syncHeight,
		hdrSyncPeer)

	// The chain is not synced whenever the current best height is not within a
	// couple of blocks of the height to sync to.
	if bestHeight+2 < syncHeight {
		m.isCurrentMtx.Lock()
		m.isCurrent = false
		m.isCurrentMtx.Unlock()
	}

	// Request headers starting from the parent of the best known header for the
	// local chain from the sync peer.
	m.fetchNextHeaders(hdrSyncPeer)

	// Update the sync height when it is higher than the currently best known
	// value.
	m.syncHeightMtx.Lock()
	if syncHeight > m.syncHeight {
		m.syncHeight = syncHeight
	}
	m.syncHeightMtx.Unlock()

	// Start the header sync progress stall timeout.
	m.hdrSyncState.resetStallTimeout()
}

// startSync updates sync peer candidacy and kicks off the blockchain sync
// process depending on the current overall sync state.
//
// When the initial header sync process has not been completed, it selects the
// peer with the most blocks and begins to download the blockchain headers from
// it.
//
// On the other hand, when the initial header sync process is complete, it
// starts downloading any outstanding blocks that are still needed.
func (m *SyncManager) startSync() {
	// Update sync peer candidacy and determine the best sync peer.
	chain := m.cfg.Chain
	best := chain.BestSnapshot()
	m.updateSyncPeerState(best.Height)

	// Update the state of whether or not the manager believes the chain is
	// fully synced to whatever the chain believes when there is no candidate
	// for a sync peer.
	//
	// Also, return now when there isn't a sync peer candidate as there is
	// nothing more to do without one.
	if m.syncPeer == nil {
		m.isCurrentMtx.Lock()
		m.isCurrent = chain.IsCurrent()
		m.isCurrentMtx.Unlock()
		log.Warnf("No sync peer candidates available")
		return
	}

	// Perform the initial header sync process as needed.
	headersSynced := m.hdrSyncState.headersSynced
	if !headersSynced {
		m.startInitialHeaderSync(best.Height)
	}

	// Download any blocks needed to catch the local chain up to the best
	// known header (if any) when the initial headers sync is already done.
	//
	// This is done in addition to the header request above to avoid waiting
	// for the round trip when there are still blocks that are needed
	// regardless of the headers response.
	if headersSynced {
		m.fetchNextBlocks(m.syncPeer)
	}
}

// onInitialChainSyncDone is invoked when the initial chain sync process
// completes.
func (m *SyncManager) onInitialChainSyncDone() {
	best := m.cfg.Chain.BestSnapshot()
	log.Infof("Initial chain sync complete (hash %s, height %d)",
		best.Hash, best.Height)

	// Request initial state from all peers that still need it now that the
	// initial chain sync is done.
	for peer := range m.peers {
		peer.maybeRequestInitialState(!m.cfg.NoMiningStateSync)
	}
}

// handlePeerConnectedMsg deals with new peers that have signalled they may
// be considered as a sync peer (they have already successfully negotiated).  It
// also starts syncing if needed.  It is invoked from the eventHandler goroutine.
func (m *SyncManager) handlePeerConnectedMsg(ctx context.Context, peer *Peer) {
	select {
	case <-ctx.Done():
	default:
	}

	log.Infof("New valid peer %s (%s)", peer, peer.UserAgent())

	m.peers[peer] = struct{}{}

	// Request headers starting from the parent of the best known header for the
	// local chain immediately when the initial headers sync process is complete
	// and the peer is a sync candidate.
	//
	// This primarily serves two purposes:
	//
	// 1) It immediately discovers any blocks that are not already known
	// 2) It provides accurate discovery of the best known block of the peer
	//
	// Note that the parent is used because the request would otherwise result
	// in an empty response when both the local and remote tips are the same.
	if peer.syncCandidate && m.hdrSyncState.headersSynced {
		m.fetchNextHeaders(peer)
	}

	// Start syncing by choosing the best candidate if needed.
	if peer.syncCandidate && m.syncPeer == nil {
		m.startSync()
	}

	// Potentially request the initial state from this peer now when the manager
	// believes the chain is fully synced.  Otherwise, it will be requested when
	// the initial chain sync process is complete.
	if m.IsCurrent() {
		peer.maybeRequestInitialState(!m.cfg.NoMiningStateSync)
	}
}

// handlePeerDisconnectedMsg deals with peers that have signalled they are done.
// It removes the peer as a candidate for syncing and in the case where it was
// the current sync peer, attempts to select a new best peer to sync from.  It
// is invoked from the eventHandler goroutine.
func (m *SyncManager) handlePeerDisconnectedMsg(peer *Peer) {
	// Remove the peer from the list of candidate peers.
	delete(m.peers, peer)

	// Re-request in-flight blocks and transactions that were not received
	// by the disconnected peer if the data was announced by another peer.
	// Remove the data from the manager's requested data maps if no other
	// peers have announced the data.
	requestQueues := make(map[*Peer][]wire.InvVect)
	var inv wire.InvVect
	inv.Type = wire.InvTypeTx
TxHashes:
	for txHash := range peer.requestedTxns {
		inv.Hash = txHash
		for pp := range m.peers {
			if !pp.IsKnownInventory(&inv) {
				continue
			}
			invs := append(requestQueues[pp], inv)
			requestQueues[pp] = invs
			pp.requestedTxns[txHash] = struct{}{}
			continue TxHashes
		}
		// No peers found that have announced this data.
		delete(m.requestedTxns, txHash)
	}
	inv.Type = wire.InvTypeBlock
BlockHashes:
	for blockHash := range peer.requestedBlocks {
		inv.Hash = blockHash
		for pp := range m.peers {
			if !pp.IsKnownInventory(&inv) {
				continue
			}
			invs := append(requestQueues[pp], inv)
			requestQueues[pp] = invs
			pp.requestedBlocks[blockHash] = struct{}{}
			continue BlockHashes
		}
		// No peers found that have announced this data.
		delete(m.requestedBlocks, blockHash)
	}
	inv.Type = wire.InvTypeMix
MixHashes:
	for mixHash := range peer.requestedMixMsgs {
		inv.Hash = mixHash
		for pp := range m.peers {
			if !pp.IsKnownInventory(&inv) {
				continue
			}
			invs := append(requestQueues[pp], inv)
			requestQueues[pp] = invs
			pp.requestedMixMsgs[mixHash] = struct{}{}
			continue MixHashes
		}
		// No peers found that have announced this data.
		delete(m.requestedMixMsgs, mixHash)
	}
	for pp, requestQueue := range requestQueues {
		var numRequested int32
		gdmsg := wire.NewMsgGetData()
		for i := range requestQueue {
			// Note the copy is intentional here to avoid keeping a reference
			// into the request queue map from the message queue since that
			// reference could potentially prevent the map from being garbage
			// collected for an extended period of time.
			ivCopy := requestQueue[i]
			gdmsg.AddInvVect(&ivCopy)
			numRequested++
			if numRequested == wire.MaxInvPerMsg {
				// Send full getdata message and reset.
				pp.QueueMessage(gdmsg, nil)
				gdmsg = wire.NewMsgGetData()
				numRequested = 0
			}
		}

		if len(gdmsg.InvList) > 0 {
			pp.QueueMessage(gdmsg, nil)
		}
	}

	// Attempt to find a new peer to sync from when the quitting peer is the
	// sync peer.
	if m.syncPeer == peer {
		m.syncPeer = nil
		m.startSync()
	}
}

// handleTxMsg handles transaction messages from all peers.
func (m *SyncManager) handleTxMsg(tmsg *txMsg) {
	peer := tmsg.peer

	// NOTE:  BitcoinJ, and possibly other wallets, don't follow the spec of
	// sending an inventory message and allowing the remote peer to decide
	// whether or not they want to request the transaction via a getdata
	// message.  Unfortunately, the reference implementation permits
	// unrequested data, so it has allowed wallets that don't follow the
	// spec to proliferate.  While this is not ideal, there is no check here
	// to disconnect peers for sending unsolicited transactions to provide
	// interoperability.
	txHash := tmsg.tx.Hash()

	// Ignore transactions that have already been rejected.  The transaction was
	// unsolicited if it was already previously rejected.
	if m.rejectedTxns.Contains(txHash[:]) {
		log.Debugf("Ignoring unsolicited previously rejected transaction %v "+
			"from %s", txHash, peer)
		return
	}

	// Process the transaction to include validation, insertion in the
	// memory pool, orphan handling, etc.
	allowOrphans := m.cfg.MaxOrphanTxs > 0
	acceptedTxs, err := m.cfg.TxMemPool.ProcessTransaction(tmsg.tx,
		allowOrphans, true, mempool.Tag(peer.ID()))

	// Remove transaction from request maps. Either the mempool/chain
	// already knows about it and as such we shouldn't have any more
	// instances of trying to fetch it, or we failed to insert and thus
	// we'll retry next time we get an inv.
	delete(peer.requestedTxns, *txHash)
	delete(m.requestedTxns, *txHash)

	if err != nil {
		// Do not request this transaction again until a new block has been
		// processed.
		m.rejectedTxns.Add(txHash[:])

		// When the error is a rule error, it means the transaction was
		// simply rejected as opposed to something actually going wrong,
		// so log it as such.  Otherwise, something really did go wrong,
		// so log it as an actual error.
		var rErr mempool.RuleError
		if errors.As(err, &rErr) {
			log.Debugf("Rejected transaction %v from %s: %v", txHash, peer, err)
		} else {
			log.Errorf("Failed to process transaction %v: %v", txHash, err)
		}
		return
	}

	m.cfg.PeerNotifier.AnnounceNewTransactions(acceptedTxs)
}

// handleMixMsg handles mixing messages from all peers.
func (m *SyncManager) handleMixMsg(mmsg *mixMsg) error {
	peer := mmsg.peer

	mixHash := mmsg.msg.Hash()

	// Ignore transactions that have already been rejected.  The transaction was
	// unsolicited if it was already previously rejected.
	if m.rejectedMixMsgs.Contains(mixHash[:]) {
		log.Debugf("Ignoring unsolicited previously rejected mix message %v "+
			"from %s", &mixHash, peer)
		return nil
	}

	accepted, err := m.cfg.MixPool.AcceptMessage(mmsg.msg)

	// Remove message from request maps. Either the mixpool already knows
	// about it and as such we shouldn't have any more instances of trying
	// to fetch it, or we failed to insert and thus we'll retry next time
	// we get an inv.
	delete(peer.requestedMixMsgs, mixHash)
	delete(m.requestedMixMsgs, mixHash)

	if err != nil {
		// Do not request this message again until a new block has
		// been processed.  If the message is an orphan KE, it is
		// tracked internally by mixpool as an orphan; there is no
		// need to request it again after requesting the unknown PR.
		m.rejectedMixMsgs.Add(mixHash[:])

		// When the error is a rule error, it means the message was
		// simply rejected as opposed to something actually going wrong,
		// so log it as such.
		//
		// When the error is an orphan KE with unknown PR, the PR will be
		// requested from the peer submitting the KE.  This is a normal
		// occurrence, and will be logged at debug instead at error level.
		//
		// Otherwise, something really did go wrong, so log it as an
		// actual error.
		var rErr *mixpool.RuleError
		var missingPRErr *mixpool.MissingOwnPRError
		if errors.As(err, &rErr) || errors.As(err, &missingPRErr) {
			log.Debugf("Rejected %T mixing message %v from %s: %v",
				mmsg.msg, &mixHash, peer, err)
		} else {
			log.Errorf("Failed to process %T mixing message %v: %v",
				mmsg.msg, &mixHash, err)
		}
		return err
	}

	if len(accepted) == 0 {
		return nil
	}

	m.cfg.PeerNotifier.AnnounceMixMessages(accepted)
	return nil
}

// maybeUpdateIsCurrent potentially updates the manager to signal it believes
// the chain is considered synced.
//
// This function MUST be called with the is current mutex held (for writes).
func (m *SyncManager) maybeUpdateIsCurrent() {
	// Nothing to do when already considered synced.
	if m.isCurrent {
		return
	}

	// The chain is considered synced once both the blockchain believes it is
	// current and the sync height is reached or exceeded.
	best := m.cfg.Chain.BestSnapshot()
	syncHeight := m.SyncHeight()
	if best.Height >= syncHeight && m.cfg.Chain.IsCurrent() {
		m.isCurrent = true
	}
}

// maybeUpdateBestAnnouncedBlock potentially updates the block with the most
// cumulative proof of work that the given peer has announced which includes its
// associated hash, cumulative work sum, and height.
//
// This function is NOT safe for concurrent access.  It must be called from the
// event handler goroutine.
func (m *SyncManager) maybeUpdateBestAnnouncedBlock(p *Peer, hash *chainhash.Hash, header *wire.BlockHeader) {
	chain := m.cfg.Chain
	workSum, err := chain.ChainWork(hash)
	if err != nil {
		return
	}

	// Update the best block and associated values when the cumulative work for
	// given block exceeds that of the current best known block for the peer.
	if p.bestAnnouncedWork == nil || workSum.Gt(p.bestAnnouncedWork) {
		p.bestAnnouncedBlock = hash
		p.bestAnnouncedWork = &workSum
		p.UpdateLastBlockHeight(int64(header.Height))
	}
}

// maybeResolveOrphanBlock potentially resolves the most recently announced
// block by the peer that did not connect to any headers known to the local
// chain at the time of the announcement by checking if it is now known and,
// when it is, potentially making it the block with the most cumulative proof of
// work announced by the peer if needed.
//
// This function is NOT safe for concurrent access.  It must be called from the
// event handler goroutine.
func (m *SyncManager) maybeResolveOrphanBlock(p *Peer) {
	// Nothing to do if there isn't a pending orphan block announcement that has
	// not yet been resolved or the block still isn't known.
	chain := m.cfg.Chain
	blockHash := p.announcedOrphanBlock
	if blockHash == nil || !chain.HaveHeader(blockHash) {
		return
	}

	// The block has now been resolved, so potentially make it the block with
	// the most cumulative proof of work announced by the peer.
	header, err := chain.HeaderByHash(blockHash)
	if err != nil {
		log.Warnf("Unable to retrieve known good header %s: %v", blockHash, err)
		return
	}
	m.maybeUpdateBestAnnouncedBlock(p, blockHash, &header)
}

// processBlock processes the provided block using the internal chain instance.
//
// When no errors occurred during processing, the first return value indicates
// the length of the fork the block extended.  In the case it either extended
// the best chain or is now the tip of the best chain due to causing a
// reorganize, the fork length will be 0.  Orphans are rejected and can be
// detected by checking if the error is blockchain.ErrMissingParent.
func (m *SyncManager) processBlock(block *dcrutil.Block) (int64, error) {
	// Process the block to include validation, best chain selection, etc.
	forkLen, err := m.cfg.Chain.ProcessBlock(block)
	if err != nil {
		return 0, err
	}

	// Update the sync height when the block is higher than the currently best
	// known value and it extends the main chain.
	onMainChain := forkLen == 0
	if onMainChain {
		m.syncHeightMtx.Lock()
		blockHeight := int64(block.MsgBlock().Header.Height)
		if blockHeight > m.syncHeight {
			m.syncHeight = blockHeight
		}
		m.syncHeightMtx.Unlock()
	}

	m.isCurrentMtx.Lock()
	m.maybeUpdateIsCurrent()
	m.isCurrentMtx.Unlock()

	return forkLen, nil
}

// handleBlockMsg handles block messages from all peers.
func (m *SyncManager) handleBlockMsg(bmsg *blockMsg) {
	peer := bmsg.peer

	// The remote peer is misbehaving when the block was not requested.
	blockHash := bmsg.block.Hash()
	if _, exists := peer.requestedBlocks[*blockHash]; !exists {
		log.Warnf("Got unrequested block %v from %s -- disconnecting",
			blockHash, peer)
		peer.Disconnect()
		return
	}

	// Save whether or not the chain believes it is current prior to processing
	// the block for use below in determining logging behavior.
	chain := m.cfg.Chain
	wasChainCurrent := chain.IsCurrent()
	curBestHeaderHash, _ := chain.BestHeader()
	isBlockForBestHeader := curBestHeaderHash == *blockHash

	// Process the block to include validation, best chain selection, etc.
	//
	// Also, remove the block from the request maps once it has been processed.
	// This ensures chain is aware of the block before it is removed from the
	// maps in order to help prevent duplicate requests.
	forkLen, err := m.processBlock(bmsg.block)
	delete(peer.requestedBlocks, *blockHash)
	delete(m.requestedBlocks, *blockHash)
	if err != nil {
		// Ideally there should never be any requests for duplicate blocks, but
		// ignore any that manage to make it through.
		if errors.Is(err, blockchain.ErrDuplicateBlock) {
			return
		}

		// When the error is a rule error, it means the block was simply
		// rejected as opposed to something actually going wrong, so log it as
		// such.  Otherwise, something really did go wrong, so log it as an
		// actual error.
		//
		// Note that orphan blocks are never requested so there is no need to
		// test for that rule error separately.
		var rErr blockchain.RuleError
		if errors.As(err, &rErr) {
			log.Infof("Rejected block %v from %s: %v", blockHash, peer, err)
		} else {
			log.Errorf("Failed to process block %v: %v", blockHash, err)
		}
		if errors.Is(err, database.ErrCorruption) ||
			errors.Is(err, blockchain.ErrUtxoBackendCorruption) {

			log.Errorf("Critical failure: %v", err)
		}

		// Request headers from all peers that serve data to discover any new
		// blocks that are not already known starting from the parent of the new
		// best known header for the local chain when the header that was
		// previously believed to be the best candidate is rejected.
		//
		// Also, reset the sync height to whatever the chain reports as the new
		// best header height since it is now very likely less than the tip that
		// was just rejected.
		if isBlockForBestHeader {
			_, newBestHeaderHeight := chain.BestHeader()
			m.syncHeightMtx.Lock()
			m.syncHeight = newBestHeaderHeight
			m.syncHeightMtx.Unlock()

			m.isCurrentMtx.Lock()
			m.maybeUpdateIsCurrent()
			m.isCurrentMtx.Unlock()

			for peer := range m.peers {
				if peer.syncCandidate {
					m.fetchNextHeaders(peer)
				}
			}
		}

		return
	}

	// Log information about the block.  Use the progress logger when the chain
	// was not already current prior to processing the block to provide nicer
	// periodic logging with a progress percentage.  Otherwise, log the block
	// individually along with some stats.
	msgBlock := bmsg.block.MsgBlock()
	header := &msgBlock.Header
	if !wasChainCurrent {
		forceLog := int64(header.Height) >= m.SyncHeight()
		m.progressLogger.LogProgress(msgBlock, forceLog, chain.VerifyProgress)
		if chain.IsCurrent() {
			m.onInitialChainSyncDone()
		}
	} else {
		var interval string
		prevBlockHeader, err := chain.HeaderByHash(&header.PrevBlock)
		if err == nil {
			diff := header.Timestamp.Sub(prevBlockHeader.Timestamp)
			interval = ", interval " + diff.Round(time.Second).String()
		}

		numTxns := uint64(len(msgBlock.Transactions))
		numTickets := uint64(header.FreshStake)
		numVotes := uint64(header.Voters)
		numRevokes := uint64(header.Revocations)
		log.Infof("New block %s (%d %s, %d %s, %d %s, %d %s, height %d%s)",
			blockHash, numTxns, pickNoun(numTxns, "transaction", "transactions"),
			numTickets, pickNoun(numTickets, "ticket", "tickets"),
			numVotes, pickNoun(numVotes, "vote", "votes"),
			numRevokes, pickNoun(numRevokes, "revocation", "revocations"),
			header.Height, interval)
	}

	// Perform some additional processing when the block extended the main
	// chain.
	onMainChain := forkLen == 0
	if onMainChain {
		// Prune invalidated transactions.
		best := chain.BestSnapshot()
		m.cfg.TxMemPool.PruneStakeTx(best.NextStakeDiff, best.Height)
		m.cfg.TxMemPool.PruneExpiredTx(best.Height)

		// Clear the rejected transactions.
		m.rejectedTxns.Reset()

		// Remove expired pair requests and completed mixes from
		// mixpool.
		m.cfg.MixPool.RemoveSpentPRs(msgBlock.Transactions)
		m.cfg.MixPool.RemoveSpentPRs(msgBlock.STransactions)
		m.cfg.MixPool.ExpireMessagesInBackground(header.Height)
	}

	// Request more blocks using the headers when the request queue is getting
	// short.
	if peer == m.syncPeer && len(peer.requestedBlocks) < minInFlightBlocks {
		m.fetchNextBlocks(peer)
	}
}

// guessHeaderSyncProgress returns a percentage that is a guess of the progress
// of the header sync progress for the given currently best known header based
// on an algorithm that considers the total number of expected headers based on
// the target time per block of the network.  It should only be used for the
// main and test networks because it relies on relatively consistent mining
// which is not the case for other network such as the simulation test network.
//
// This function is safe for concurrent access.
func (m *SyncManager) guessHeaderSyncProgress(header *wire.BlockHeader) float64 {
	// Calculate the expected total number of blocks to reach the current time
	// by considering the number there already are plus the expected number of
	// remaining ones there should be in the time interval since the provided
	// best known header and the current time given the target block time.
	//
	// This approach is used as opposed to calculating the total expected since
	// the genesis block since it gets more accurate as more headers are
	// processed and thus provide more information.  It is also more robust
	// against networks with dynamic difficulty readjustment such as the test
	// network.
	curTimestamp := m.cfg.TimeSource.AdjustedTime().Unix()
	targetSecsPerBlock := int64(m.cfg.ChainParams.TargetTimePerBlock.Seconds())
	remaining := (curTimestamp - header.Timestamp.Unix()) / targetSecsPerBlock
	expectedTotal := int64(header.Height) + remaining

	// Finally the progress guess is simply the ratio of the current number of
	// known headers to the total expected number of headers.
	return math.Min(float64(header.Height)/float64(expectedTotal), 1.0) * 100
}

// headerSyncProgress returns a percentage that is a guess of the progress of
// the header sync process.
//
// This function is safe for concurrent access.
func (m *SyncManager) headerSyncProgress() float64 {
	hash, _ := m.cfg.Chain.BestHeader()
	header, err := m.cfg.Chain.HeaderByHash(&hash)
	if err != nil {
		return 0.0
	}

	// Use an algorithm that considers the total number of expected headers
	// based on the target time per block of the network for the main and test
	// networks.  This is the preferred approach because, unlike the sync height
	// reported by remote peers, it is difficult to game since it is based on
	// the target proof of work, but it assumes consistent mining, which is not
	// the case on all networks, so limit it to the two where that applies.
	net := m.cfg.ChainParams.Net
	if net == wire.MainNet || net == wire.TestNet3 {
		return m.guessHeaderSyncProgress(&header)
	}

	// Fall back to using the sync height reported by the remote peer otherwise.
	syncHeight := m.SyncHeight()
	if syncHeight == 0 {
		return 0.0
	}
	return math.Min(float64(header.Height)/float64(syncHeight), 1.0) * 100
}

// handleHeadersMsg handles headers messages from all peers.
func (m *SyncManager) handleHeadersMsg(hmsg *headersMsg) {
	peer := hmsg.peer

	// Nothing to do for an empty headers message as it means the sending peer
	// does not have any additional headers for the requested block locator.
	headers := hmsg.headers.Headers
	numHeaders := len(headers)
	if numHeaders == 0 {
		return
	}

	// Handle the case where the first header does not connect to any known
	// headers specially.
	chain := m.cfg.Chain
	firstHeader := headers[0]
	firstHeaderHash := firstHeader.BlockHash()
	firstHeaderConnects := chain.HaveHeader(&firstHeader.PrevBlock)
	headersSynced := m.hdrSyncState.headersSynced
	if !firstHeaderConnects {
		// Attempt to detect block announcements which do not connect to any
		// known headers and request any headers starting from the best header
		// the local chain knows in order to (hopefully) discover the missing
		// headers unless the initial headers sync process is still in progress.
		//
		// Meanwhile, also keep track of how many times the peer has
		// consecutively sent a headers message that looks like an announcement
		// that does not connect and disconnect it once the max allowed
		// threshold has been reached.
		if numHeaders < maxExpectedHeaderAnnouncementsPerMsg {
			peer.numConsecutiveOrphanHeaders++
			if peer.numConsecutiveOrphanHeaders >= maxConsecutiveOrphanHeaders {
				log.Debugf("Received %d consecutive headers messages that do "+
					"not connect from peer %s -- disconnecting",
					peer.numConsecutiveOrphanHeaders, peer)
				peer.Disconnect()
				return
			}

			if headersSynced {
				log.Debugf("Requesting missing parents for header %s (height "+
					"%d) received from peer %s", firstHeaderHash,
					firstHeader.Height, peer)
				bestHeaderHash, _ := chain.BestHeader()
				blkLocator := chain.BlockLocatorFromHash(&bestHeaderHash)
				locator := chainBlockLocatorToHashes(blkLocator)
				peer.PushGetHeadersMsg(locator, &zeroHash)
			}

			// Track the final announced header as the most recently announced
			// block by the peer that does not connect to any headers known to
			// the local chain since there is a good chance it will eventually
			// become known either from this peer or others.
			m.maybeResolveOrphanBlock(peer)
			finalHeader := headers[len(headers)-1]
			finalHeaderHash := finalHeader.BlockHash()
			peer.announcedOrphanBlock = &finalHeaderHash

			// Update the latest block height for the peer to avoid stale
			// heights when looking for future potential header sync node
			// candidacy when the initial headers sync process is still in
			// progess.
			if !headersSynced {
				peer.UpdateLastBlockHeight(int64(finalHeader.Height))
			}
			return
		}

		// Disconnect the peer when the initial headers sync process is done and
		// this does not appear to be a block announcement.
		if headersSynced {
			log.Debugf("Received orphan header from peer %s -- disconnecting",
				peer)
			peer.Disconnect()
			return
		}

		// Ignore headers that do not connect to any known headers when the
		// initial headers sync is taking place.  It is expected that headers
		// will be announced that are not yet known.
		return
	}

	// Ensure all of the received headers connect the previous one before
	// attempting to perform any further processing on any of them.
	headerHashes := make([]chainhash.Hash, 0, len(headers))
	headerHashes = append(headerHashes, firstHeaderHash)
	for prevIdx, header := range headers[1:] {
		prevHash := &headerHashes[prevIdx]
		prevHeight := headers[prevIdx].Height
		if header.PrevBlock != *prevHash || header.Height != prevHeight+1 {
			log.Debugf("Received block header that does not properly connect "+
				"to previous one from peer %s -- disconnecting", peer)
			peer.Disconnect()
			return
		}
		headerHashes = append(headerHashes, header.BlockHash())
	}

	// Save the current best known header height prior to processing the headers
	// so the code later is able to determine if any new useful headers were
	// provided.
	_, prevBestHeaderHeight := chain.BestHeader()

	// Process all of the received headers.
	for _, header := range headers {
		err := chain.ProcessBlockHeader(header)
		if err != nil {
			// Update the sync height when the sync peer fails to process any
			// headers since that chain is invalid from the local point of view
			// and thus whatever the best known good header is becomes the new
			// sync height unless a better one is discovered from the new sync
			// peer.
			if peer == m.syncPeer && !headersSynced {
				_, newBestHeaderHeight := chain.BestHeader()
				m.syncHeightMtx.Lock()
				m.syncHeight = newBestHeaderHeight
				m.syncHeightMtx.Unlock()
			}

			// Note that there is no need to check for an orphan header here
			// because they were already verified to connect above.

			log.Debugf("Failed to process block header %s from peer %s: %v -- "+
				"disconnecting", header.BlockHash(), peer, err)
			peer.Disconnect()
			return
		}
	}

	// All of the headers were either accepted or already known valid at this
	// point.

	// Reset the header sync progress stall timeout when the headers are not
	// already synced and progress was made.
	newBestHeaderHash, newBestHeaderHeight := chain.BestHeader()
	if peer == m.syncPeer && !headersSynced {
		if newBestHeaderHeight > prevBestHeaderHeight {
			m.hdrSyncState.resetStallTimeout()
		}
	}

	// Reset the count of consecutive headers messages that contained headers
	// which do not connect.  Note that this is intentionally only done when all
	// of the provided headers are successfully processed above.
	peer.numConsecutiveOrphanHeaders = 0

	// Potentially resolve a previously unknown announced block and then update
	// the block with the most cumulative proof of work the peer has announced
	// to the final announced header if needed.
	finalHeader := headers[len(headers)-1]
	finalReceivedHash := &headerHashes[len(headerHashes)-1]
	m.maybeResolveOrphanBlock(peer)
	m.maybeUpdateBestAnnouncedBlock(peer, finalReceivedHash, finalHeader)

	// Update the sync height if the new best known header height exceeds it.
	syncHeight := m.SyncHeight()
	if newBestHeaderHeight > syncHeight {
		syncHeight = newBestHeaderHeight
		m.syncHeightMtx.Lock()
		m.syncHeight = syncHeight
		m.syncHeightMtx.Unlock()
	}

	// Disconnect outbound peers that have less cumulative work than the minimum
	// value already known to have been achieved on the network a priori while
	// the initial sync is still underway.  This is determined by noting that a
	// peer only sends fewer than the maximum number of headers per message when
	// it has reached its best known header.
	isChainCurrent := chain.IsCurrent()
	receivedMaxHeaders := len(headers) == wire.MaxBlockHeadersPerMsg
	if !isChainCurrent && !peer.Inbound() && !receivedMaxHeaders {
		if m.minKnownWork != nil {
			workSum, err := chain.ChainWork(finalReceivedHash)
			if err == nil && workSum.Lt(m.minKnownWork) {
				log.Debugf("Best known chain for peer %s has too little "+
					"cumulative work -- disconnecting", peer)
				peer.Disconnect()
				return
			}
		}
	}

	// Request more headers when the peer announced the maximum number of
	// headers that can be sent in a single message since it probably has more.
	if receivedMaxHeaders {
		blkLocator := chain.BlockLocatorFromHash(finalReceivedHash)
		locator := chainBlockLocatorToHashes(blkLocator)
		peer.PushGetHeadersMsg(locator, &zeroHash)

		m.progressLogger.LogHeaderProgress(uint64(len(headers)), headersSynced,
			m.headerSyncProgress)
	}

	// Consider the headers synced once the sync peer sends a message with a
	// final header that is within a few blocks of the sync height.
	if !headersSynced && peer == m.syncPeer {
		const syncHeightFetchOffset = 6
		if int64(finalHeader.Height)+syncHeightFetchOffset > syncHeight {
			headersSynced = true
			m.hdrSyncState.headersSynced = headersSynced
			m.hdrSyncState.stopStallTimeout()

			m.progressLogger.LogHeaderProgress(uint64(len(headers)),
				headersSynced, m.headerSyncProgress)
			log.Infof("Initial headers sync complete (best header hash %s, "+
				"height %d)", newBestHeaderHash, newBestHeaderHeight)
			log.Info("Syncing chain")
			m.progressLogger.SetLastLogTime(time.Now())

			// Request headers starting from the parent of the best known header
			// for the local chain from any sync candidates that have not yet
			// had their best known block discovered now that the initial
			// headers sync process is complete.
			for peer := range m.peers {
				m.maybeResolveOrphanBlock(peer)
				if !peer.syncCandidate || peer.bestAnnouncedBlock != nil {
					continue
				}
				m.fetchNextHeaders(peer)
			}

			// Potentially update whether the chain believes it is current now
			// that the headers are synced.
			chain.MaybeUpdateIsCurrent()
			isChainCurrent = chain.IsCurrent()
			if isChainCurrent {
				m.onInitialChainSyncDone()
			}
		}
	}

	// Immediately download blocks associated with the announced headers once
	// the chain is current.  This allows downloading from whichever peer
	// announces it first and also ensures any side chain blocks are downloaded
	// for vote consideration.
	//
	// Ultimately, it would likely be better for this functionality to be moved
	// to the code which determines the next blocks to request based on the
	// available headers once that code supports downloading from multiple peers
	// and associated infrastructure to efficiently determine which peers have
	// the associated block(s).
	if isChainCurrent {
		gdmsg := wire.NewMsgGetDataSizeHint(uint(len(headers)))
		for i := range headerHashes {
			// Skip the block when it has already been requested or is otherwise
			// already known.
			hash := &headerHashes[i]
			if m.isRequestedBlock(hash) || chain.HaveBlock(hash) {
				continue
			}

			// Stop requesting when the request would exceed the max size of the
			// map used to track requests.
			if len(m.requestedBlocks)+1 > maxRequestedBlocks {
				break
			}

			m.requestedBlocks[*hash] = peer
			peer.requestedBlocks[*hash] = struct{}{}
			iv := wire.NewInvVect(wire.InvTypeBlock, hash)
			gdmsg.AddInvVect(iv)
		}
		if len(gdmsg.InvList) > 0 {
			peer.QueueMessage(gdmsg, nil)
		}
	}

	// Download any blocks needed to catch the local chain up to the best known
	// header (if any) once the initial headers sync is done.
	if headersSynced && m.syncPeer != nil {
		m.fetchNextBlocks(m.syncPeer)
	}
}

// handleNotFoundMsg handles notfound messages from all peers.
func (m *SyncManager) handleNotFoundMsg(nfmsg *notFoundMsg) {
	peer := nfmsg.peer

	for _, inv := range nfmsg.notFound.InvList {
		// verify the hash was actually announced by the peer
		// before deleting from the global requested maps.
		switch inv.Type {
		case wire.InvTypeBlock:
			if _, exists := peer.requestedBlocks[inv.Hash]; exists {
				delete(peer.requestedBlocks, inv.Hash)
				delete(m.requestedBlocks, inv.Hash)
			}
		case wire.InvTypeTx:
			if _, exists := peer.requestedTxns[inv.Hash]; exists {
				delete(peer.requestedTxns, inv.Hash)
				delete(m.requestedTxns, inv.Hash)
			}
		case wire.InvTypeMix:
			if _, exists := peer.requestedMixMsgs[inv.Hash]; exists {
				delete(peer.requestedMixMsgs, inv.Hash)
				delete(m.requestedMixMsgs, inv.Hash)
			}
		}
	}
}

// needTx returns whether or not the transaction needs to be downloaded.  For
// example, it does not need to be downloaded when it is already known.
func (m *SyncManager) needTx(hash *chainhash.Hash) bool {
	// No need for transactions that have already been rejected.
	if m.rejectedTxns.Contains(hash[:]) {
		return false
	}

	// No need for transactions that are already available in the transaction
	// memory pool (main pool or orphan).
	if m.cfg.TxMemPool.HaveTransaction(hash) {
		return false
	}

	// No need for transactions that were recently confirmed.
	if m.cfg.RecentlyConfirmedTxns.Contains(hash[:]) {
		return false
	}

	return true
}

// needMixMsg returns whether or not the mixing message needs to be downloaded.
func (m *SyncManager) needMixMsg(hash *chainhash.Hash) bool {
	if m.rejectedMixMsgs.Contains(hash[:]) {
		return false
	}

	if m.cfg.MixPool.HaveMessage(hash) {
		return false
	}

	// TODO: It would be ideal here to not 'need' previously-observed
	// messages that are known/expected to fail validation, or messages
	// that have already been removed from mixpool.  An LRU of recently
	// removed mixpool messages may work well.

	return true
}

// handleInvMsg handles inv messages from all peers.  This entails examining the
// inventory advertised by the remote peer for block and transaction
// announcements and acting accordingly.
func (m *SyncManager) handleInvMsg(imsg *invMsg) {
	peer := imsg.peer
	isCurrent := m.IsCurrent()

	// Update state information regarding per-peer known inventory and determine
	// what inventory to request based on factors such as the current sync state
	// and whether or not the data is already available.
	//
	// Also, keep track of the final announced block (when there is one) so the
	// peer can be updated with that information accordingly.
	var lastBlock *wire.InvVect
	var requestQueue []*wire.InvVect
	for _, iv := range imsg.inv.InvList {
		switch iv.Type {
		case wire.InvTypeBlock:
			// NOTE: All block announcements are now made via headers and the
			// decisions regarding which blocks to download are based on those
			// headers.  Therefore, there is no need to request anything here.
			//
			// Also, there realistically should not typically be any inv
			// messages with a type of block for the same reason.  However, it
			// doesn't hurt to update the state accordingly just in case.

			// Add the block to the cache of known inventory for the peer.  This
			// helps avoid sending blocks to the peer that it is already known
			// to have.
			peer.AddKnownInventory(iv)

			// Update the last block in the announced inventory.
			lastBlock = iv

		case wire.InvTypeTx:
			// Add the tx to the cache of known inventory for the peer.  This
			// helps avoid sending transactions to the peer that it is already
			// known to have.
			peer.AddKnownInventory(iv)

			// Ignore transaction announcements before the chain is current or
			// are otherwise not needed, such as when they were recently
			// rejected or are already known.
			//
			// Transaction announcements are based on the state of the fully
			// synced ledger, so they are likely to be invalid before the chain
			// is current.
			if !isCurrent || !m.needTx(&iv.Hash) {
				continue
			}

			// Request the transaction if there is not one already pending.
			if _, exists := m.requestedTxns[iv.Hash]; !exists {
				limitAdd(m.requestedTxns, iv.Hash, maxRequestedTxns)
				limitAdd(peer.requestedTxns, iv.Hash, maxRequestedTxns)
				requestQueue = append(requestQueue, iv)
			}

		case wire.InvTypeMix:
			// Add the mix message to the cache of known inventory
			// for the peer.  This helps avoid sending mix messages
			// to the peer that it is already known to have.
			peer.AddKnownInventory(iv)

			// Ignore mixing messages before the chain is current or
			// if the messages are not needed.  Pair request (PR)
			// messages reference unspent outputs that must be
			// checked to exist and be unspent before they are
			// accepted, and all later messages must reference an
			// existing PR recorded in the mixing pool.
			if !isCurrent || !m.needMixMsg(&iv.Hash) {
				continue
			}

			// Request the mixing message if it is not already pending.
			if _, exists := m.requestedMixMsgs[iv.Hash]; !exists {
				limitAdd(m.requestedMixMsgs, iv.Hash, maxRequestedMixMsgs)
				limitAdd(peer.requestedMixMsgs, iv.Hash, maxRequestedMixMsgs)
				requestQueue = append(requestQueue, iv)
			}
		}
	}

	if lastBlock != nil {
		// Determine if the final announced block is already known to the local
		// chain and then either track it as the most recently announced
		// block by the peer that does not connect to any headers known to the
		// local chain or potentially make it the block with the most cumulative
		// proof of work announced by the peer when it is already known.
		if !m.cfg.Chain.HaveHeader(&lastBlock.Hash) {
			// Notice a copy of the hash is made here to avoid keeping a
			// reference into the inventory vector which would prevent it from
			// being GCd.
			lastBlockHash := lastBlock.Hash
			m.maybeResolveOrphanBlock(peer)
			peer.announcedOrphanBlock = &lastBlockHash
		} else {
			header, err := m.cfg.Chain.HeaderByHash(&lastBlock.Hash)
			if err == nil {
				m.maybeResolveOrphanBlock(peer)
				m.maybeUpdateBestAnnouncedBlock(peer, &lastBlock.Hash, &header)
			}
		}
	}

	// Request as much as possible at once.
	var numRequested int32
	gdmsg := wire.NewMsgGetData()
	for _, iv := range requestQueue {
		gdmsg.AddInvVect(iv)
		numRequested++
		if numRequested == wire.MaxInvPerMsg {
			// Send full getdata message and reset.
			//
			// NOTE: There should never be more than wire.MaxInvPerMsg in the
			// inv request, so this could return after the QueueMessage, but
			// this is safer.
			peer.QueueMessage(gdmsg, nil)
			gdmsg = wire.NewMsgGetData()
			numRequested = 0
		}
	}

	if len(gdmsg.InvList) > 0 {
		peer.QueueMessage(gdmsg, nil)
	}
}

// limitAdd is a helper function for maps that require a maximum limit by
// evicting a random value if adding the new value would cause it to
// overflow the maximum allowed.
func limitAdd(m map[chainhash.Hash]struct{}, hash chainhash.Hash, limit int) {
	// Nothing to do if entry is already in the map.
	if _, exists := m[hash]; exists {
		return
	}
	if len(m)+1 > limit {
		// Remove a random entry from the map.  For most compilers, Go's
		// range statement iterates starting at a random item although
		// that is not 100% guaranteed by the spec.  The iteration order
		// is not important here because an adversary would have to be
		// able to pull off preimage attacks on the hashing function in
		// order to target eviction of specific entries anyways.
		for txHash := range m {
			delete(m, txHash)
			break
		}
	}
	m[hash] = struct{}{}
}

// eventHandler is the main handler for the sync manager.  It must be run as a
// goroutine.  It processes block and inv messages in a separate goroutine from
// the peer handlers so the block (MsgBlock) messages are handled by a single
// thread without needing to lock memory data structures.  This is important
// because the sync manager controls which blocks are needed and how the
// fetching should proceed.
func (m *SyncManager) eventHandler(ctx context.Context) {
out:
	for {
		select {
		case data := <-m.msgChan:
			switch msg := data.(type) {
			case *peerConnectedMsg:
				m.handlePeerConnectedMsg(ctx, msg.peer)

			case *txMsg:
				m.handleTxMsg(msg)
				select {
				case msg.reply <- struct{}{}:
				case <-ctx.Done():
				}

			case *blockMsg:
				m.handleBlockMsg(msg)
				select {
				case msg.reply <- struct{}{}:
				case <-ctx.Done():
				}

			case *mixMsg:
				err := m.handleMixMsg(msg)
				select {
				case msg.reply <- err:
				case <-ctx.Done():
				}

			case *invMsg:
				m.handleInvMsg(msg)

			case *headersMsg:
				m.handleHeadersMsg(msg)

			case *notFoundMsg:
				m.handleNotFoundMsg(msg)

			case *peerDisconnectedMsg:
				m.handlePeerDisconnectedMsg(msg.peer)

			case getSyncPeerMsg:
				var peerID int32
				if m.syncPeer != nil {
					peerID = m.syncPeer.ID()
				}
				msg.reply <- peerID

			case requestFromPeerMsg:
				err := m.requestFromPeer(msg.peer, msg.blocks, msg.voteHashes,
					msg.tSpendHashes, msg.mixHashes)
				msg.reply <- requestFromPeerResponse{
					err: err,
				}

			case processBlockMsg:
				forkLen, err := m.processBlock(msg.block)
				if err != nil {
					msg.reply <- processBlockResponse{
						forkLen: forkLen,
						err:     err,
					}
					continue
				}

				onMainChain := forkLen == 0
				if onMainChain {
					// Prune invalidated transactions.
					best := m.cfg.Chain.BestSnapshot()
					m.cfg.TxMemPool.PruneStakeTx(best.NextStakeDiff,
						best.Height)
					m.cfg.TxMemPool.PruneExpiredTx(best.Height)
				}

				msg.reply <- processBlockResponse{
					err: nil,
				}

			default:
				log.Warnf("Invalid message type in event handler: %T", msg)
			}

		case <-m.hdrSyncState.stallTimer.C:
			// Mark the timer's channel as having been drained so the timer can
			// safely be reset.
			m.hdrSyncState.stallChanDrained = true

			// Disconnect the sync peer due to stalling the header sync process.
			if m.syncPeer != nil {
				log.Debugf("Header sync progress stalled from peer %s -- "+
					"disconnecting", m.syncPeer)
				m.syncPeer.Disconnect()
			}

		case <-ctx.Done():
			break out
		}
	}

	log.Trace("Sync manager event handler done")
}

// PeerConnected informs the sync manager of a newly active peer.
func (m *SyncManager) PeerConnected(peer *Peer) {
	select {
	case m.msgChan <- &peerConnectedMsg{peer: peer}:
	case <-m.quit:
	}
}

// OnTx adds the passed transaction message and peer to the event handling
// queue.
func (m *SyncManager) OnTx(tx *dcrutil.Tx, peer *Peer, done chan struct{}) {
	select {
	case m.msgChan <- &txMsg{tx: tx, peer: peer, reply: done}:
	case <-m.quit:
		done <- struct{}{}
	}
}

// OnBlock adds the passed block message and peer to the event handling
// queue.
func (m *SyncManager) OnBlock(block *dcrutil.Block, peer *Peer, done chan struct{}) {
	select {
	case m.msgChan <- &blockMsg{block: block, peer: peer, reply: done}:
	case <-m.quit:
		done <- struct{}{}
	}
}

// OnInv adds the passed inv message and peer to the event handling queue.
func (m *SyncManager) OnInv(inv *wire.MsgInv, peer *Peer) {
	select {
	case m.msgChan <- &invMsg{inv: inv, peer: peer}:
	case <-m.quit:
	}
}

// OnHeaders adds the passed headers message and peer to the event handling
// queue.
func (m *SyncManager) OnHeaders(headers *wire.MsgHeaders, peer *Peer) {
	select {
	case m.msgChan <- &headersMsg{headers: headers, peer: peer}:
	case <-m.quit:
	}
}

// OnMixMsg adds the passed mixing message and peer to the event handling
// queue.
func (m *SyncManager) OnMixMsg(msg mixing.Message, peer *Peer, done chan error) {
	select {
	case m.msgChan <- &mixMsg{msg: msg, peer: peer, reply: done}:
	case <-m.quit:
	}
}

// OnNotFound adds the passed notfound message and peer to the event handling
// queue.
func (m *SyncManager) OnNotFound(notFound *wire.MsgNotFound, peer *Peer) {
	select {
	case m.msgChan <- &notFoundMsg{notFound: notFound, peer: peer}:
	case <-m.quit:
	}
}

// PeerDisconnected informs the sync manager that a peer has disconnected.
func (m *SyncManager) PeerDisconnected(peer *Peer) {
	select {
	case m.msgChan <- &peerDisconnectedMsg{peer: peer}:
	case <-m.quit:
	}
}

// SyncPeerID returns the ID of the current sync peer, or 0 if there is none.
func (m *SyncManager) SyncPeerID() int32 {
	reply := make(chan int32, 1)
	select {
	case m.msgChan <- getSyncPeerMsg{reply: reply}:
	case <-m.quit:
	}

	select {
	case peerID := <-reply:
		return peerID
	case <-m.quit:
		return 0
	}
}

// RequestFromPeer allows an outside caller to request blocks or transactions
// from a peer.  The requests are logged in the internal map of requests so the
// peer is not later banned for sending the respective data.
func (m *SyncManager) RequestFromPeer(p *Peer, blocks, voteHashes,
	tSpendHashes, mixHashes []chainhash.Hash) error {

	reply := make(chan requestFromPeerResponse, 1)
	request := requestFromPeerMsg{
		peer:         p,
		blocks:       blocks,
		voteHashes:   voteHashes,
		tSpendHashes: tSpendHashes,
		mixHashes:    mixHashes,
		reply:        reply,
	}
	select {
	case m.msgChan <- request:
	case <-m.quit:
	}

	select {
	case response := <-reply:
		return response.err
	case <-m.quit:
		return fmt.Errorf("sync manager stopped")
	}
}

func (m *SyncManager) requestFromPeer(peer *Peer, blocks, voteHashes,
	tSpendHashes, mixHashes []chainhash.Hash) error {

	// Add the blocks to the request.
	msgResp := wire.NewMsgGetData()
	for i := range blocks {
		// Skip the block when it has already been requested.
		bh := &blocks[i]
		if m.isRequestedBlock(bh) {
			continue
		}

		// Skip the block when it is already known.
		if m.cfg.Chain.HaveBlock(bh) {
			continue
		}

		err := msgResp.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, bh))
		if err != nil {
			return fmt.Errorf("unexpected error encountered building request "+
				"for mining state block %v: %v",
				bh, err.Error())
		}

		peer.requestedBlocks[*bh] = struct{}{}
		m.requestedBlocks[*bh] = peer
	}

	addTxsToRequest := func(txs []chainhash.Hash, txType stake.TxType) error {
		// Return immediately if txs is nil.
		if txs == nil {
			return nil
		}

		for i := range txs {
			// If we've already requested this transaction, skip it.
			tx := &txs[i]
			_, alreadyReqP := peer.requestedTxns[*tx]
			_, alreadyReqB := m.requestedTxns[*tx]

			if alreadyReqP || alreadyReqB {
				continue
			}

			// Ask the transaction memory pool if the transaction is known
			// to it in any form (main pool or orphan).
			if m.cfg.TxMemPool.HaveTransaction(tx) {
				continue
			}

			// Check if the transaction exists from the point of view of the main
			// chain tip.  Note that this is only a best effort since it is expensive
			// to check existence of every output and the only purpose of this check
			// is to avoid requesting already known transactions.
			//
			// Check for a specific outpoint based on the tx type.
			outpoint := wire.OutPoint{Hash: *tx}
			switch txType {
			case stake.TxTypeSSGen:
				// The first two outputs of vote transactions are OP_RETURN <data>, and
				// therefore never exist as an unspent txo.  Use the third output, as
				// the third output (and subsequent outputs) are OP_SSGEN outputs.
				outpoint.Index = 2
				outpoint.Tree = wire.TxTreeStake
			case stake.TxTypeTSpend:
				// The first output of a tSpend transaction is OP_RETURN <data>, and
				// therefore never exists as an unspent txo.  Use the second output, as
				// the second output (and subsequent outputs) are OP_TGEN outputs.
				outpoint.Index = 1
				outpoint.Tree = wire.TxTreeStake
			}
			entry, err := m.cfg.Chain.FetchUtxoEntry(outpoint)
			if err != nil {
				return err
			}
			if entry != nil {
				continue
			}

			err = msgResp.AddInvVect(wire.NewInvVect(wire.InvTypeTx, tx))
			if err != nil {
				return fmt.Errorf("unexpected error encountered building request "+
					"for mining state vote %v: %v",
					tx, err.Error())
			}

			peer.requestedTxns[*tx] = struct{}{}
			m.requestedTxns[*tx] = struct{}{}
		}

		return nil
	}

	// Add the vote transactions to the request.
	err := addTxsToRequest(voteHashes, stake.TxTypeSSGen)
	if err != nil {
		return err
	}

	// Add the tspend transactions to the request.
	err = addTxsToRequest(tSpendHashes, stake.TxTypeTSpend)
	if err != nil {
		return err
	}

	for i := range mixHashes {
		// If we've already requested this mix message, skip it.
		mh := &mixHashes[i]
		_, alreadyReqP := peer.requestedMixMsgs[*mh]
		_, alreadyReqB := m.requestedMixMsgs[*mh]

		if alreadyReqP || alreadyReqB {
			continue
		}

		// Skip the message when it is already known.
		if m.cfg.MixPool.HaveMessage(mh) {
			continue
		}

		err := msgResp.AddInvVect(wire.NewInvVect(wire.InvTypeMix, mh))
		if err != nil {
			return fmt.Errorf("unexpected error encountered building request "+
				"for inv vect mix hash %v: %v",
				mh, err.Error())
		}

		peer.requestedMixMsgs[*mh] = struct{}{}
		m.requestedMixMsgs[*mh] = struct{}{}
	}

	if len(msgResp.InvList) > 0 {
		peer.QueueMessage(msgResp, nil)
	}

	return nil
}

// ProcessBlock makes use of ProcessBlock on an internal instance of a block
// chain.  It is funneled through the sync manager since blockchain is not safe
// for concurrent access.
func (m *SyncManager) ProcessBlock(block *dcrutil.Block) error {
	reply := make(chan processBlockResponse, 1)
	select {
	case m.msgChan <- processBlockMsg{block: block, reply: reply}:
	case <-m.quit:
	}

	select {
	case response := <-reply:
		return response.err
	case <-m.quit:
		return fmt.Errorf("sync manager stopped")
	}
}

// IsCurrent returns whether or not the sync manager believes it is synced with
// the connected peers.
//
// This function is safe for concurrent access.
func (m *SyncManager) IsCurrent() bool {
	m.isCurrentMtx.Lock()
	m.maybeUpdateIsCurrent()
	isCurrent := m.isCurrent
	m.isCurrentMtx.Unlock()
	return isCurrent
}

// Run starts the sync manager and all other goroutines necessary for it to
// function properly and blocks until the provided context is cancelled.
func (m *SyncManager) Run(ctx context.Context) {
	log.Trace("Starting sync manager")

	// Start the event handler goroutine.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		m.eventHandler(ctx)
		wg.Done()
	}()

	// Shutdown the sync manager when the context is cancelled.
	<-ctx.Done()
	close(m.quit)
	wg.Wait()
	log.Trace("Sync manager stopped")
}

// Config holds the configuration options related to the network chain
// synchronization manager.
type Config struct {
	// PeerNotifier specifies an implementation to use for notifying peers of
	// status changes related to blocks and transactions.
	PeerNotifier PeerNotifier

	// ChainParams identifies which chain parameters the manager is associated
	// with.
	ChainParams *chaincfg.Params

	// Chain specifies the chain instance to use for processing blocks and
	// transactions.
	Chain *blockchain.BlockChain

	// TimeSource defines the median time source which is used to retrieve the
	// current time adjusted by the median time offset.
	TimeSource blockchain.MedianTimeSource

	// TxMemPool specifies the mempool to use for processing transactions.
	TxMemPool *mempool.TxPool

	// NoMiningStateSync indicates whether or not the sync manager should
	// perform an initial mining state synchronization with peers once they are
	// believed to be fully synced.
	NoMiningStateSync bool

	// MaxPeers specifies the maximum number of peers the server is expected to
	// be connected with.  It is primarily used as a hint for more efficient
	// synchronization.
	MaxPeers int

	// MaxOrphanTxs specifies the maximum number of orphan transactions the
	// transaction pool associated with the server supports.
	MaxOrphanTxs int

	// RecentlyConfirmedTxns specifies a size limited set to use for tracking
	// and querying the most recently confirmed transactions.  It is useful for
	// preventing duplicate requests.
	RecentlyConfirmedTxns *apbf.Filter

	// MixPool specifies the mixing pool to use for transient mixing
	// messages broadcast across the network.
	MixPool *mixpool.Pool
}

// New returns a new network chain synchronization manager.  Use Run to begin
// processing asynchronous events.
func New(config *Config) *SyncManager {
	// Convert the minimum known work to a uint256 when it exists.  Ideally, the
	// chain params should be updated to use the new type, but that will be a
	// major version bump, so a one-time conversion is a good tradeoff in the
	// mean time.
	var minKnownWork *uint256.Uint256
	minKnownWorkBig := config.ChainParams.MinKnownChainWork
	if minKnownWorkBig != nil {
		minKnownWork = new(uint256.Uint256).SetBig(minKnownWorkBig)
	}

	return &SyncManager{
		cfg:              *config,
		rejectedTxns:     apbf.NewFilter(maxRejectedTxns, rejectedTxnsFPRate),
		rejectedMixMsgs:  apbf.NewFilter(maxRejectedMixMsgs, rejectedMixMsgsFPRate),
		requestedTxns:    make(map[chainhash.Hash]struct{}),
		requestedBlocks:  make(map[chainhash.Hash]*Peer),
		requestedMixMsgs: make(map[chainhash.Hash]struct{}),
		peers:            make(map[*Peer]struct{}),
		minKnownWork:     minKnownWork,
		hdrSyncState:     makeHeaderSyncState(),
		progressLogger:   progresslog.New("Processed", log),
		msgChan:          make(chan interface{}, config.MaxPeers*3),
		quit:             make(chan struct{}),
		syncHeight:       config.Chain.BestSnapshot().Height,
		isCurrent:        config.Chain.IsCurrent(),
	}
}
