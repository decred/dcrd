// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"

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
	peerpkg "github.com/decred/dcrd/peer/v4"
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

// Peer extends a common peer to maintain additional state needed by the sync
// manager.  The internals are intentionally unexported to create an opaque
// type.
type Peer struct {
	*peerpkg.Peer

	// This flag is set during creation based on information available from the
	// embedded peer and is immutable making it safe for concurrent access.
	//
	// servesData indicates the peer is capable of serving data.  Currently,
	// this effectively means it is a full node.
	servesData bool

	// requestInitialStateOnce is used to ensure the initial state data is only
	// requested from the peer once.
	requestInitialStateOnce sync.Once

	// numConsecutiveOrphanHeaders tracks the number of consecutive header
	// messages sent by the peer that contain headers which do not connect.  It
	// is used to detect peers that have either diverged so far they are no
	// longer useful or are otherwise being malicious.
	numConsecutiveOrphanHeaders atomic.Int64

	// These fields are used to track the best known block announced by the peer
	// which in turn provides a means to discover which blocks are available to
	// download from the peer.  They are protected by the associated best
	// announced mutex.
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
	bestAnnouncedMtx     sync.Mutex
	announcedOrphanBlock *chainhash.Hash
	bestAnnouncedBlock   *chainhash.Hash
	bestAnnouncedWork    *uint256.Uint256
}

// NewPeer returns a new instance of a peer that wraps the provided underlying
// common peer with additional state that is used throughout the package.
func NewPeer(peer *peerpkg.Peer) *Peer {
	servesData := peer.Services()&wire.SFNodeNetwork == wire.SFNodeNetwork
	return &Peer{
		Peer:       peer,
		servesData: servesData,
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
	// This mutex is used to protect the overall state transition from the
	// initial headers sync mode to the initial chain sync mode.
	sync.Mutex

	// headersSynced tracks whether or not the headers are synced to a point
	// that is recent enough to start downloading blocks.
	//
	// Note that it is an atomic as opposed to a plain boolean flag in order to
	// provide a fast path once the initial headers sync is done.
	//
	// However, the state transition itself, which includes updating this flag
	// to true, is protected by the slower embedded mutex.  In other words, care
	// is required to ensure there are no logic races before the flag is set,
	// but once it is set, callers can safely rely on it without needing the
	// slower mutex.
	//
	// This approach provides a nice performance boost because the initial
	// header sync process is only active for a short period, but its status is
	// needed continuously, such as for every new header.
	headersSynced atomic.Bool

	// stallTimer is used to implement a progress stall timeout that can be
	// reset at any time without needing to create a new one and the associated
	// extra garbage.
	stallTimer *time.Timer
}

// makeHeaderSyncState returns a header sync state that is ready to use.
func makeHeaderSyncState() headerSyncState {
	stallTimer := time.NewTimer(math.MaxInt64)
	stallTimer.Stop()
	return headerSyncState{
		stallTimer: stallTimer,
	}
}

// StopStallTimeout makes a best effort attempt to prevent the progress stall
// timer from firing.  It has no effect if the timer has already fired.
//
// It is only a best effort attempt because stopping the timer concurrently with
// listening on its channel leaves the possibility for the timer to be in the
// process of firing just as it's being stopped which can result in a stale
// time value being delivered on the channel prior to Go 1.23.
//
// That possibility is not a concern because:
//
//	a) The stall timeout logic only depends on the event as opposed to the time
//	   values
//	b) The timeout is sufficiently long that it is almost never hit in practice
//	c) It is even more rare that the timer would just happen to be in the
//	   process of firing (sending to the channel) when data nearly
//	   simultaneously arrives and resets the timer just before it actually fires
//	d) Even if all of that were to happen, the only effect is the standard
//	   stall timeout handling logic which means there are no adverse effects
//
// This function is safe for concurrent access.
func (state *headerSyncState) StopStallTimeout() {
	state.stallTimer.Stop()
}

// ResetStallTimeout resets the progress stall timer.  Note that resetting the
// timer concurrently with listening on its channel leaves the possibility for
// the timer to be in process of firing just as it's being reset which can
// result in a stale time value being delivered on the channel prior to Go 1.23.
//
// The aforementioned possibility is not a concern for the same reasons
// discussed in [headerSyncState.StopStallTimeout].
//
// This function is safe for concurrent access.
func (state *headerSyncState) ResetStallTimeout() {
	state.stallTimer.Reset(headerSyncStallTimeoutSecs * time.Second)
}

// InitialHeaderSyncDone returns whether or not the initial header sync process
// is complete.
func (state *headerSyncState) InitialHeaderSyncDone() bool {
	return state.headersSynced.Load()
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
	//
	// This value is set during creation and is immutable making it safe for
	// concurrent access.
	minKnownWork *uint256.Uint256

	rejectedTxns    *apbf.Filter
	rejectedMixMsgs *apbf.Filter
	progressLogger  *progresslog.Logger

	// These fields track pending requests for data from all peers.  They are
	// protected by the request mutex.
	requestMtx       sync.Mutex
	requestedTxns    map[chainhash.Hash]*Peer
	requestedBlocks  map[chainhash.Hash]*Peer
	requestedMixMsgs map[chainhash.Hash]*Peer

	// The following fields are used to track the current sync peer.  The peer
	// is protected by the associated mutex.
	//
	// warnOnNoSync is used to track whether or not a warning should be logged
	// when there is no suitable sync peer.
	syncPeerMtx  sync.Mutex
	syncPeer     *Peer
	warnOnNoSync bool

	// The following fields are used to track the peers available to the sync
	// manager.  The map is protected by the associated mutex.
	peersMtx sync.Mutex
	peers    map[*Peer]struct{}

	// hdrSyncState houses the state used to track the initial header sync
	// process and related stall handling.
	hdrSyncState headerSyncState

	// The following fields are used to track the state of the initial chain
	// sync process.  The flag and transition to the initial chain synced state
	// is protected by the associated mutex.
	initialChainSyncDoneMtx sync.Mutex
	isInitialChainSyncDone  bool

	// syncHeight tracks the height being synced to from peers.
	syncHeight atomic.Int64

	// isCurrent tracks whether or not the manager believes it is fully synced
	// to the network.
	isCurrent atomic.Bool

	// The following fields are used to track the list of the next blocks to
	// download in the branch leading up to the best known header.  They are
	// protected by the associated mutex.
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
	nextBlocksMtx    sync.Mutex
	nextBlocksHeader chainhash.Hash
	nextBlocksBuf    [512]chainhash.Hash
	nextNeededBlocks []chainhash.Hash
}

// shutdownRequested returns true when the context used to run the sync manager
// has been canceled.  This simplifies checking for shutdown slightly since the
// caller can just use an if statement instead of a select.
func (m *SyncManager) shutdownRequested() bool {
	select {
	case <-m.quit:
		return true
	default:
	}

	return false
}

// maybeUpdateSyncHeight atomically sets [m.syncHeight] to the provided value
// when it is greater than its current value.
//
// This function is safe for concurrent access.
func (m *SyncManager) maybeUpdateSyncHeight(newHeight int64) {
	// NOTE: It's faster to avoid branches that would be introduced by checking
	// the result of the CAS operation since both the success case and failure
	// case of the CAS can be handled cleanly by simply loading the new value.
	//
	// Concretely, if the CAS fails, the conditional response would be to load
	// the new value and loop around to try again.  However, if it succeeds, the
	// conditional response would either be to break out of the for loop or to
	// update the local variable to the new value so the for loop terminates.
	// Both options are relatively expensive as compared to the chosen approach
	// because they both would induce another conditional branch.
	curHeight := m.syncHeight.Load()
	for curHeight < newHeight {
		m.syncHeight.CompareAndSwap(curHeight, newHeight)
		curHeight = m.syncHeight.Load()
	}
}

// SyncHeight returns latest known block being synced to.
func (m *SyncManager) SyncHeight() int64 {
	return m.syncHeight.Load()
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
// This function MUST be called with the next blocks mutex held (writes).
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
// This function MUST be called with the request mutex held (reads).
func (m *SyncManager) isRequestedBlock(hash *chainhash.Hash) bool {
	_, ok := m.requestedBlocks[*hash]
	return ok
}

// isRequestedBlockFromPeer returns whether or not the given block hash has been
// requested from the given remote peer.
//
// This function MUST be called with the request mutex held (reads).
func (m *SyncManager) isRequestedBlockFromPeer(peer *Peer, hash *chainhash.Hash) bool {
	requestedFrom, ok := m.requestedBlocks[*hash]
	return ok && requestedFrom == peer
}

// isRequestedTxFromPeer returns whether or not the given transaction hash has
// been requested from the given remote peer.
//
// This function MUST be called with the request mutex held (reads).
func (m *SyncManager) isRequestedTxFromPeer(peer *Peer, hash *chainhash.Hash) bool {
	requestedFrom, ok := m.requestedTxns[*hash]
	return ok && requestedFrom == peer
}

// isRequestedMixMsgFromPeer returns whether or not the given mix message hash
// has been requested from the given remote peer.
//
// This function MUST be called with the request mutex held (reads).
func (m *SyncManager) isRequestedMixMsgFromPeer(peer *Peer, hash *chainhash.Hash) bool {
	requestedFrom, ok := m.requestedMixMsgs[*hash]
	return ok && requestedFrom == peer
}

// fetchNextBlocks creates and sends a request to the provided peer for the next
// blocks to be downloaded based on the current headers.
//
// This function is safe for concurrent access.
func (m *SyncManager) fetchNextBlocks(peer *Peer) {
	// Nothing to do if the target maximum number of blocks to request from the
	// peer at the same time are already in flight.
	var numInFlight int
	m.requestMtx.Lock()
	for _, requestedFrom := range m.requestedBlocks {
		if requestedFrom != peer {
			continue
		}
		numInFlight++
	}
	m.requestMtx.Unlock()
	if numInFlight >= maxInFlightBlocks {
		return
	}

	defer m.nextBlocksMtx.Unlock()
	m.nextBlocksMtx.Lock()

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
	m.requestMtx.Lock()
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
		gdmsg.AddInvVect(iv)
	}
	m.requestMtx.Unlock()
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

// isSyncPeerCandidate returns whether or not the given peer is a sync peer
// candidate.
//
// Being a sync peer candidate has the following requirements:
//   - The peer must serve data
//   - The peer's latest known block height must be at least as high as the best
//     known block height of the local chain
func isSyncPeerCandidate(peer *Peer, bestHeight int64) bool {
	return peer.servesData && peer.LastBlock() >= bestHeight
}

// updateSyncPeerState updates the sync peer to be the peer that is both a sync
// candidate and has the highest known block height and updates the state of
// whether or not the manager believes the chain is fully synced to whatever the
// chain believes when there are no suitable candidates.
//
// It also potentially warns when no suitable candidate is found depending on
// various factors such as whether or not a warning has already been shown since
// the last time there was a valid sync peer.
//
// This function MUST be called with the sync peer mutex held (writes).
func (m *SyncManager) updateSyncPeerState() {
	chain := m.cfg.Chain
	_, bestHeaderHeight := chain.BestHeader()

	// Determine the best sync peer and number of outbound peers.
	var bestPeer *Peer
	var numOutbound uint64
	m.peersMtx.Lock()
	for peer := range m.peers {
		// Tally total number of outbound peers.
		if !peer.Inbound() {
			numOutbound++
		}

		// Skip peers that are not sync candidates.
		if !isSyncPeerCandidate(peer, bestHeaderHeight) {
			continue
		}

		// The best sync candidate is the most updated peer.
		if bestPeer == nil || bestPeer.LastBlock() < peer.LastBlock() {
			bestPeer = peer
		}
	}
	m.peersMtx.Unlock()

	// Update the state of whether or not the manager believes the chain is
	// fully synced to whatever the chain believes and return after potentially
	// logging a warning when there is not a suitable candidate since there is
	// nothing more to do without one.
	if bestPeer == nil {
		m.isCurrent.Store(chain.IsCurrent())

		// A sync peer already being assigned prior to calling implies it was
		// disconnected or otherwise is no longer a suitable candidate.  On the
		// other hand, no sync peer assigned implies there were no suitable
		// candidates at all.
		//
		// The latter case is expected to happen and thus is only something
		// worthy of a warning when the max expected number of outbound
		// connections has been reached.
		hadSyncPeer := m.syncPeer != nil
		hasMaxOutbound := numOutbound >= m.cfg.MaxOutboundPeers
		if m.warnOnNoSync && (hadSyncPeer || hasMaxOutbound) {
			log.Warnf("No sync peer candidates available")
			m.warnOnNoSync = false
		}
	} else {
		// Ensure future warnings are eligible to be shown when no sync peer
		// candidates are available.
		m.warnOnNoSync = true
	}

	m.syncPeer = bestPeer
}

// startInitialHeaderSync attempts to find the best header sync candidate and,
// so long as one is found, saves it as the sync peer and starts or continues
// the initial header sync process with it.
//
// The initial header sync process consists of downloading all headers that are
// not already known from the sync peer.  As the name implies, this assumes the
// initial header sync process is not already done.
//
// This function MUST be called with the sync peer mutex held (writes).
func (m *SyncManager) startInitialHeaderSync() {
	// Attempt to find and set the best header sync peer candidate and return if
	// no suitable candidates were found since there is nothing more to do
	// without one.
	m.updateSyncPeerState()
	if m.syncPeer == nil {
		return
	}

	syncHeight := m.syncPeer.LastBlock()
	log.Infof("Syncing headers to block height %d from peer %v", syncHeight,
		m.syncPeer)

	// The chain is not synced whenever the current best chain height is not
	// within a couple of blocks of the height to sync to.
	bestHeight := m.cfg.Chain.BestSnapshot().Height
	if bestHeight+2 < syncHeight {
		m.isCurrent.Store(false)
	}

	// Request headers starting from the parent of the best known header for the
	// local chain from the sync peer.
	m.fetchNextHeaders(m.syncPeer)

	// Update the sync height when it is higher than the currently best known
	// value.
	m.maybeUpdateSyncHeight(syncHeight)

	// Start the header sync progress stall timeout.
	m.hdrSyncState.ResetStallTimeout()
}

// startChainSync attempts to find a peer to sync the chain from and, so long as
// one is found, saves it as the sync peer and starts or continues the
// blockchain sync process with it.
//
// This function MUST be called with the sync peer mutex held (writes).
func (m *SyncManager) startChainSync() {
	// Attempt to find and set the best sync peer candidate and return if no
	// suitable candidates were found since there is nothing more to do without
	// one.
	m.updateSyncPeerState()
	if m.syncPeer == nil {
		return
	}

	// Download any blocks needed to catch the local chain up to the best known
	// header (if any).
	//
	// This is done to avoid waiting for a round trip on header discovery when
	// there are still blocks that are needed regardless of the headers
	// response.
	m.fetchNextBlocks(m.syncPeer)
}

// onInitialChainSyncDone is invoked when the initial chain sync process
// completes.
//
// This function MUST be called with the initial sync done mutex held (writes).
func (m *SyncManager) onInitialChainSyncDone() {
	// This is typically only called once in practice currently due to the way
	// the sync process is handled, but that might change in the future with
	// parallel and out of order block processing or possibly parallel header
	// syncing.  Be cautious and prevent multiple invocations just in case.
	//
	// A sync.Once is not used because other code needs to make decisions based
	// on the flag.  It might also be useful in the future to allow the sync
	// state to move back into initial chain sync mode if the chain falls too
	// far behind, which, for example, could happen due to an extended network
	// outage.
	if m.isInitialChainSyncDone {
		return
	}
	m.isInitialChainSyncDone = true

	best := m.cfg.Chain.BestSnapshot()
	log.Infof("Initial chain sync complete (hash %s, height %d)", best.Hash,
		best.Height)

	// Request initial state from all peers that still need it now that the
	// initial chain sync is done.
	m.peersMtx.Lock()
	for peer := range m.peers {
		peer.maybeRequestInitialState(!m.cfg.NoMiningStateSync)
	}
	m.peersMtx.Unlock()
}

// OnPeerConnected should be invoked with peers that have already gone through
// version negotiation and passed other initial validation checks that determine
// it is suitable for participation in staying synced with the network.
//
// This function is safe for concurrent access.
func (m *SyncManager) OnPeerConnected(peer *Peer) {
	if m.shutdownRequested() {
		return
	}

	m.peersMtx.Lock()
	m.peers[peer] = struct{}{}
	m.peersMtx.Unlock()

	// Attempt to find a peer to sync headers from when there isn't already one
	// and the initial headers sync process is still in progress.
	//
	// Technically, this currently could start the process with the peer that is
	// connecting if it is a header sync candidate because the only time the
	// sync peer will not be set due to the current logic is when there are no
	// other candidates.  However, using the same logic to select a new sync
	// peer as when the current one disconnects is more consistent and easier to
	// update if more complex logic is introduced in the future.
	if !m.hdrSyncState.InitialHeaderSyncDone() {
		m.syncPeerMtx.Lock()
		if m.syncPeer == nil {
			m.startInitialHeaderSync()
		}
		m.syncPeerMtx.Unlock()
		return
	}

	// -------------------------------------------------------
	// The initial headers sync process is done at this point.
	// -------------------------------------------------------

	// Request headers starting from the parent of the best known header for the
	// local chain immediately when the initial headers sync process is complete
	// and the peer potentially serves useful data.
	//
	// This primarily serves two purposes:
	//
	// 1) It immediately discovers any blocks that are not already known
	// 2) It provides accurate discovery of the best known block of the peer
	//
	// Note that the parent is used because the request would otherwise result
	// in an empty response when both the local and remote tips are the same.
	if peer.servesData {
		m.fetchNextHeaders(peer)
	}

	// Attempt to find a sync peer and start syncing the chain from it when
	// there isn't already one.
	m.syncPeerMtx.Lock()
	if m.syncPeer == nil {
		m.startChainSync()
	}
	m.syncPeerMtx.Unlock()

	// Potentially request the initial state from this peer now when the manager
	// believes the chain is fully synced.  Otherwise, it will be requested when
	// the initial chain sync process is complete.
	if m.IsCurrent() {
		peer.maybeRequestInitialState(!m.cfg.NoMiningStateSync)
	}
}

// OnPeerDisconnected should be invoked when peers the sync manager was
// previously informed about have disconnected.  It removes the peer as a
// candidate for syncing and, in the case it was the current sync peer, attempts
// to select a new best peer to sync from.
//
// This function is safe for concurrent access.
func (m *SyncManager) OnPeerDisconnected(peer *Peer) {
	if m.shutdownRequested() {
		return
	}

	// Remove the peer from the list of candidate peers.
	m.peersMtx.Lock()
	delete(m.peers, peer)
	m.peersMtx.Unlock()

	// Attempt to find a new peer to sync headers from when the quitting peer is
	// the sync peer and the initial headers sync process is still in progress.
	//
	// Also, skip the rest of the logic below before the headers are synced
	// since no requests for the data being checked are made prior to that point
	// nor can the chain sync be started.
	if !m.hdrSyncState.InitialHeaderSyncDone() {
		m.syncPeerMtx.Lock()
		if m.syncPeer == peer {
			m.startInitialHeaderSync()
		}
		m.syncPeerMtx.Unlock()
		return
	}

	// -------------------------------------------------------
	// The initial headers sync process is done at this point.
	// -------------------------------------------------------

	// Re-request in-flight blocks and transactions that were not received by
	// the disconnected peer if the data was announced by another peer.  Remove
	// the data from the manager's requested data maps if no other peers have
	// announced the data.
	requestQueues := make(map[*Peer][]wire.InvVect)
	var inv wire.InvVect
	inv.Type = wire.InvTypeTx
	m.requestMtx.Lock()
TxHashes:
	for txHash, requestedFrom := range m.requestedTxns {
		if requestedFrom != peer {
			continue
		}
		inv.Hash = txHash
		m.peersMtx.Lock()
		for pp := range m.peers {
			if !pp.IsKnownInventory(&inv) {
				continue
			}
			requestQueues[pp] = append(requestQueues[pp], inv)
			m.requestedTxns[txHash] = pp
			m.peersMtx.Unlock()
			continue TxHashes
		}
		m.peersMtx.Unlock()
		// No peers found that have announced this data.
		delete(m.requestedTxns, txHash)
	}
	inv.Type = wire.InvTypeBlock
BlockHashes:
	for blockHash, requestedFrom := range m.requestedBlocks {
		if requestedFrom != peer {
			continue
		}
		inv.Hash = blockHash
		m.peersMtx.Lock()
		for pp := range m.peers {
			if !pp.IsKnownInventory(&inv) {
				continue
			}
			requestQueues[pp] = append(requestQueues[pp], inv)
			m.requestedBlocks[blockHash] = pp
			m.peersMtx.Unlock()
			continue BlockHashes
		}
		m.peersMtx.Unlock()
		// No peers found that have announced this data.
		delete(m.requestedBlocks, blockHash)
	}
	inv.Type = wire.InvTypeMix
MixHashes:
	for mixHash, requestedFrom := range m.requestedMixMsgs {
		if requestedFrom != peer {
			continue
		}
		inv.Hash = mixHash
		m.peersMtx.Lock()
		for pp := range m.peers {
			if !pp.IsKnownInventory(&inv) {
				continue
			}
			requestQueues[pp] = append(requestQueues[pp], inv)
			m.requestedMixMsgs[mixHash] = pp
			m.peersMtx.Unlock()
			continue MixHashes
		}
		m.peersMtx.Unlock()
		// No peers found that have announced this data.
		delete(m.requestedMixMsgs, mixHash)
	}
	m.requestMtx.Unlock()
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

	// Attempt to find a new peer to sync the chain from when the quitting peer
	// is the sync peer.
	m.syncPeerMtx.Lock()
	if m.syncPeer == peer {
		m.startChainSync()
	}
	m.syncPeerMtx.Unlock()
}

// OnTx should be invoked with transactions that are received from remote peers.
//
// Its primary purpose is processing transactions by validating them and adding
// them to the mempool along with any orphan transactions that depend on it.
//
// It also deals with some other things related to syncing such as marking
// in-flight transactions as received and preventing multiple requests for
// invalid transactions.
//
// It returns a slice of transactions added to the mempool which might include
// the passed transaction itself along with any additional orphan transactions
// that were added as a result of the passed one being accepted.
//
// Ideally, this should be called from the same peer goroutine that received the
// message so the bulk of the processing is done concurrently and further reads
// from the peer are blocked until the message is processed.
//
// This function is safe for concurrent access.
func (m *SyncManager) OnTx(peer *Peer, tx *dcrutil.Tx) []*dcrutil.Tx {
	if m.shutdownRequested() {
		return nil
	}

	// NOTE: The expected sequence of events for inventory propagation is
	// technically sending an inventory message and allowing the remote peer to
	// decide whether or not they want to request the transaction via a getdata
	// message.
	//
	// However, primarily due to inherited legacy behavior, there is no check
	// here to disconnect peers for sending unsolicited transactions.  This
	// behavior is retained in order to provide interoperability since changing
	// it now could potentially cause issues for any wallets that rely on it.
	txHash := tx.Hash()

	// Ignore transactions that have already been rejected.  The transaction was
	// unsolicited if it was already previously rejected.
	if m.rejectedTxns.Contains(txHash[:]) {
		log.Debugf("Ignoring unsolicited previously rejected transaction %v "+
			"from %s", txHash, peer)
		return nil
	}

	// Process the transaction to include validation, insertion in the memory
	// pool, orphan handling, etc.
	allowOrphans := m.cfg.MaxOrphanTxs > 0
	acceptedTxns, err := m.cfg.TxMemPool.ProcessTransaction(tx, allowOrphans,
		true, mempool.Tag(peer.ID()))

	// Remove transaction from request maps. Either the mempool/chain
	// already knows about it and as such we shouldn't have any more
	// instances of trying to fetch it, or we failed to insert and thus
	// we'll retry next time we get an inv.
	m.requestMtx.Lock()
	delete(m.requestedTxns, *txHash)
	m.requestMtx.Unlock()

	if err != nil {
		// Do not request this transaction again until a new block has been
		// processed.
		m.rejectedTxns.Add(txHash[:])

		// When the error is a rule error, it means the transaction was simply
		// rejected as opposed to something actually going wrong, so log it as
		// such.  Otherwise, something really did go wrong, so log it as an
		// actual error.
		var rErr mempool.RuleError
		if errors.As(err, &rErr) {
			log.Debugf("Rejected transaction %v from %s: %v", txHash, peer, err)
		} else {
			log.Errorf("Failed to process transaction %v: %v", txHash, err)
		}
		return nil
	}

	return acceptedTxns
}

// OnMixMsg should be invoked with mix messages that are received from remote
// peers.
//
// Its primary purpose is processing mix messages by validating them and adding
// them to the mixpool along with any orphan messages that depend on them.
//
// It also deals with some other things related to syncing such as marking
// in-flight messages as received and preventing multiple requests for invalid
// messages.
//
// It returns a slice of messages added to the mixpool which might include the
// passed message itself along with any additional orphan messages that were
// added as a result of the passed one being accepted.
//
// Ideally, this should be called from the same peer goroutine that received the
// message so the bulk of the processing is done concurrently and further reads
// from the peer are blocked until the message is processed.
//
// This function is safe for concurrent access.
func (m *SyncManager) OnMixMsg(peer *Peer, msg mixing.Message) ([]mixing.Message, error) {
	if m.shutdownRequested() {
		return nil, nil
	}

	// Ignore mix messages that have already been rejected.  The message was
	// unsolicited if it was already previously rejected.
	mixHash := msg.Hash()
	if m.rejectedMixMsgs.Contains(mixHash[:]) {
		log.Debugf("Ignoring unsolicited previously rejected mix message %v "+
			"from %s", &mixHash, peer)
		return nil, nil
	}

	source := mixpool.Uint64Source(peer.ID())
	accepted, err := m.cfg.MixPool.AcceptMessage(msg, source)

	// Remove message from request maps. Either the mixpool already knows
	// about it and as such we shouldn't have any more instances of trying
	// to fetch it, or we failed to insert and thus we'll retry next time
	// we get an inv.
	m.requestMtx.Lock()
	delete(m.requestedMixMsgs, mixHash)
	m.requestMtx.Unlock()

	if err != nil {
		// Do not request this message again until a new block has been
		// processed.  If the message is an orphan KE, it is tracked internally
		// by mixpool as an orphan; there is no need to request it again after
		// requesting the unknown PR.
		m.rejectedMixMsgs.Add(mixHash[:])

		// When the error is a rule error, it means the message was simply
		// rejected as opposed to something actually going wrong, so log it as
		// such.
		//
		// When the error is an orphan KE with unknown PR, the PR will be
		// requested from the peer submitting the KE.  This is a normal
		// occurrence, and will be logged at debug instead at error level.
		//
		// Otherwise, something really did go wrong, so log it as an actual
		// error.
		var rErr *mixpool.RuleError
		var missingPRErr *mixpool.MissingOwnPRError
		if errors.As(err, &rErr) || errors.As(err, &missingPRErr) {
			log.Debugf("Rejected %T mixing message %v from %s: %v", msg,
				&mixHash, peer, err)
		} else {
			log.Errorf("Failed to process %T mixing message %v: %v", msg,
				&mixHash, err)
		}
		return nil, err
	}

	return accepted, nil
}

// maybeUpdateIsCurrent potentially updates the manager to signal it believes
// the chain is considered synced.
//
// This function is safe for concurrent access.
func (m *SyncManager) maybeUpdateIsCurrent() {
	// Note that not protecting this entire sequence by a mutex doesn't present
	// an overall logic problem because it only ever potentially updates the
	// flag to true and whether or not the manager is considered current is
	// fundamentally a transient property that can change at any time.

	// Nothing to do when already considered synced.
	if m.isCurrent.Load() {
		return
	}

	// The chain is considered synced once both the blockchain believes it is
	// current and the sync height is reached or exceeded.
	best := m.cfg.Chain.BestSnapshot()
	syncHeight := m.SyncHeight()
	if best.Height >= syncHeight && m.cfg.Chain.IsCurrent() {
		m.isCurrent.Store(true)
	}
}

// maybeUpdateBestAnnouncedBlock potentially updates the block with the most
// cumulative proof of work that the given peer has announced which includes its
// associated hash, cumulative work sum, and height.
//
// This function MUST be called with the best announced mutex held (writes).
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
// This function MUST be called with the best announced mutex held (writes).
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
//
// This function is safe for concurrent access.
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
		m.maybeUpdateSyncHeight(int64(block.MsgBlock().Header.Height))
	}

	m.maybeUpdateIsCurrent()

	return forkLen, nil
}

// OnBlock should be invoked with blocks that are received from remote peers.
//
// Its primary purpose is processing blocks by validating them, adding them to
// the blockchain, logging relevant information, and requesting more if needed.
//
// It also deals with some other things related to syncing such as participating
// in determining when the blockchain is initially synced and removing the
// transactions in the block from the transaction and mixing pools.
//
// Ideally, this should be called from the same peer goroutine that received the
// message so the bulk of the processing is done concurrently and further reads
// from the peer are blocked until the message is processed.
//
// This function is safe for concurrent access.
func (m *SyncManager) OnBlock(peer *Peer, block *dcrutil.Block) {
	if m.shutdownRequested() {
		return
	}

	// The remote peer is misbehaving when the block was not requested.
	blockHash := block.Hash()
	m.requestMtx.Lock()
	requested := m.isRequestedBlockFromPeer(peer, blockHash)
	m.requestMtx.Unlock()
	if !requested {
		log.Warnf("Got unrequested block %v from %s -- disconnecting",
			blockHash, peer)
		peer.Disconnect()
		return
	}

	// Save the current best header prior to processing the block for use below.
	chain := m.cfg.Chain
	curBestHeaderHash, _ := chain.BestHeader()

	// Process the block to include validation, best chain selection, etc.
	//
	// Also, remove the block from the request maps once it has been processed.
	// This ensures chain is aware of the block before it is removed from the
	// maps in order to help prevent duplicate requests.
	forkLen, err := m.processBlock(block)
	m.requestMtx.Lock()
	delete(m.requestedBlocks, *blockHash)
	m.requestMtx.Unlock()
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
		isBlockForBestHeader := curBestHeaderHash == *blockHash
		if isBlockForBestHeader {
			_, newBestHeaderHeight := chain.BestHeader()
			m.syncHeight.Store(newBestHeaderHeight)

			m.maybeUpdateIsCurrent()

			m.peersMtx.Lock()
			for peer := range m.peers {
				if peer.servesData {
					m.fetchNextHeaders(peer)
				}
			}
			m.peersMtx.Unlock()
		}

		return
	}

	// Log information about the block.  Use the progress logger when the
	// initial chain sync is not done to provide nicer periodic logging with a
	// progress percentage.  Otherwise, log the block individually along with
	// some stats.
	//
	// Also, transition the initial chain sync state to done when the chain
	// becomes current.
	//
	// Note that the entire logging sequence is protected by the initial chain
	// sync done mutex to ensure proper ordering since it is also dependent on
	// the state of the transition.  Namely, the periodic progress logger must
	// no longer be used once the initial chain sync is done.
	msgBlock := block.MsgBlock()
	header := &msgBlock.Header
	m.initialChainSyncDoneMtx.Lock()
	if !m.isInitialChainSyncDone {
		isCurrent := chain.IsCurrent()
		forceLog := int64(header.Height) >= m.SyncHeight() || isCurrent
		m.progressLogger.LogProgress(msgBlock, forceLog, chain.VerifyProgress)
		if isCurrent {
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
	m.initialChainSyncDoneMtx.Unlock()

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

		// Remove expired pair requests and completed mixes from the mixpool.
		m.cfg.MixPool.RemoveSpentPRs(msgBlock.Transactions)
		m.cfg.MixPool.RemoveSpentPRs(msgBlock.STransactions)
		m.cfg.MixPool.ExpireMessagesInBackground(header.Height)
	}

	// Request more blocks using the headers when the request queue is getting
	// short.
	m.syncPeerMtx.Lock()
	isSyncPeer := peer == m.syncPeer
	m.syncPeerMtx.Unlock()
	if isSyncPeer {
		m.requestMtx.Lock()
		numInFlight := len(m.requestedBlocks)
		m.requestMtx.Unlock()
		if numInFlight < minInFlightBlocks {
			m.fetchNextBlocks(peer)
		}
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

// onInitialHeaderSyncDone is invoked when the initial header sync process
// completes.
//
// This function MUST be called with the header sync state mutex held (writes).
func (m *SyncManager) onInitialHeaderSyncDone(hash *chainhash.Hash, height int64) {
	m.hdrSyncState.StopStallTimeout()

	// This method is typically only called once in practice currently due to
	// the way the sync process is handled, but that might change in the future
	// if parallel header syncing is implemented.  Be cautious and prevent
	// multiple invocations just in case.
	//
	// Check the atomic flag again under the slower mutex to prevent possible
	// logic races during the transition from the initial headers sync mode to
	// the initial chain sync mode.
	//
	// A sync.Once is not used because other code needs to make decisions based
	// on the flag.
	if m.hdrSyncState.InitialHeaderSyncDone() {
		return
	}
	m.hdrSyncState.headersSynced.Store(true)

	log.Infof("Initial headers sync complete (best header hash %s, height %d)",
		hash, height)
	log.Info("Performing initial chain sync...")
	m.progressLogger.SetLastLogTime(time.Now())

	// Request headers starting from the parent of the best known header for the
	// local chain from any peers that potentially serve useful data and have
	// not yet had their best known block discovered now that the initial
	// headers sync process is complete.
	m.peersMtx.Lock()
	for peer := range m.peers {
		peer.bestAnnouncedMtx.Lock()
		m.maybeResolveOrphanBlock(peer)
		needsBestHeader := peer.servesData && peer.bestAnnouncedBlock == nil
		peer.bestAnnouncedMtx.Unlock()
		if !needsBestHeader {
			continue
		}
		m.fetchNextHeaders(peer)
	}
	m.peersMtx.Unlock()

	// Potentially update whether the chain believes it is current now that the
	// headers are synced.
	chain := m.cfg.Chain
	chain.MaybeUpdateIsCurrent()
	if chain.IsCurrent() {
		m.initialChainSyncDoneMtx.Lock()
		m.onInitialChainSyncDone()
		m.initialChainSyncDoneMtx.Unlock()
	}
}

// OnHeaders should be invoked with header messages that are received from
// remote peers.
//
// Its primary purpose is processing headers by ensuring they properly connect,
// adding them to the blockchain, and requesting more if needed.
//
// It also deals with some other things related to syncing such as determining
// when the headers are initially synced, disconnecting outbound peers that have
// less cumulative work compared to the known minimum work during the initial
// chain sync, and requesting blocks associated with the announced headers.
//
// Ideally, this should be called from the same peer goroutine that received the
// message so the bulk of the processing is done concurrently and further reads
// from the peer are blocked until the message is processed.
//
// This function is safe for the concurrent access.
func (m *SyncManager) OnHeaders(peer *Peer, headersMsg *wire.MsgHeaders) {
	if m.shutdownRequested() {
		return
	}

	// Nothing to do for an empty headers message as it means the sending peer
	// does not have any additional headers for the requested block locator.
	headers := headersMsg.Headers
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
	headersSynced := m.hdrSyncState.InitialHeaderSyncDone()
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
			numConsecutive := peer.numConsecutiveOrphanHeaders.Add(1)
			if numConsecutive >= maxConsecutiveOrphanHeaders {
				log.Debugf("Received %d consecutive headers messages that do "+
					"not connect from peer %s -- disconnecting", numConsecutive,
					peer)
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
			finalHeader := headers[len(headers)-1]
			finalHeaderHash := finalHeader.BlockHash()
			peer.bestAnnouncedMtx.Lock()
			m.maybeResolveOrphanBlock(peer)
			peer.announcedOrphanBlock = &finalHeaderHash
			peer.bestAnnouncedMtx.Unlock()

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

	m.syncPeerMtx.Lock()
	isSyncPeer := peer == m.syncPeer
	m.syncPeerMtx.Unlock()

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
			if !headersSynced && isSyncPeer {
				_, newBestHeaderHeight := chain.BestHeader()
				m.syncHeight.Store(newBestHeaderHeight)
			}

			// When the error is a rule error, it means the header was simply
			// rejected as opposed to something actually going wrong, so log it
			// as such.  Otherwise, something really did go wrong, so log it as
			// an actual error.
			//
			// Note that there is no need to check for an orphan header here
			// because they were already verified to connect above.
			var rErr blockchain.RuleError
			if errors.As(err, &rErr) {
				log.Debugf("Rejected block header %s from peer %s: %v -- "+
					"disconnecting", header.BlockHash(), peer, err)
			} else {
				log.Errorf("Failed to process block header %s from peer %s: %v"+
					" -- disconnecting", header.BlockHash(), peer, err)
			}
			if errors.Is(err, database.ErrCorruption) {
				log.Errorf("Criticial failure: %v", err)
			}

			peer.Disconnect()
			return
		}
	}

	// All of the headers were either accepted or already known valid at this
	// point.

	// Reset the header sync progress stall timeout when the headers are not
	// already synced and progress was made.
	newBestHeaderHash, newBestHeaderHeight := chain.BestHeader()
	if !headersSynced && isSyncPeer {
		if newBestHeaderHeight > prevBestHeaderHeight {
			m.hdrSyncState.ResetStallTimeout()
		}
	}

	// Reset the count of consecutive headers messages that contained headers
	// which do not connect.  Note that this is intentionally only done when all
	// of the provided headers are successfully processed above.
	peer.numConsecutiveOrphanHeaders.Store(0)

	// Potentially resolve a previously unknown announced block and then update
	// the block with the most cumulative proof of work the peer has announced
	// to the final announced header if needed.
	finalHeader := headers[len(headers)-1]
	finalReceivedHash := &headerHashes[len(headerHashes)-1]
	peer.bestAnnouncedMtx.Lock()
	m.maybeResolveOrphanBlock(peer)
	m.maybeUpdateBestAnnouncedBlock(peer, finalReceivedHash, finalHeader)
	peer.bestAnnouncedMtx.Unlock()

	// Update the sync height if the new best known header height exceeds it.
	m.maybeUpdateSyncHeight(newBestHeaderHeight)

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
	}

	// Consider the headers synced once the sync peer sends a message with a
	// final header that is within a few blocks of the sync height.
	if !headersSynced && isSyncPeer {
		const syncHeightFetchOffset = 6
		if int64(finalHeader.Height)+syncHeightFetchOffset > m.SyncHeight() {
			headersSynced = true

			m.hdrSyncState.Lock()
			m.progressLogger.LogHeaderProgress(uint64(len(headers)),
				headersSynced, m.headerSyncProgress)
			m.onInitialHeaderSyncDone(&newBestHeaderHash, newBestHeaderHeight)
			m.hdrSyncState.Unlock()

			// Update the local var that tracks whether the chain believes it is
			// current since it might have been updated now that the headers are
			// synced.
			isChainCurrent = chain.IsCurrent()
		} else {
			m.progressLogger.LogHeaderProgress(uint64(len(headers)),
				headersSynced, m.headerSyncProgress)
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
		m.requestMtx.Lock()
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
			iv := wire.NewInvVect(wire.InvTypeBlock, hash)
			gdmsg.AddInvVect(iv)
		}
		m.requestMtx.Unlock()
		if len(gdmsg.InvList) > 0 {
			peer.QueueMessage(gdmsg, nil)
		}
	}

	// Download any blocks needed to catch the local chain up to the best known
	// header (if any) once the initial headers sync is done.
	if headersSynced {
		m.syncPeerMtx.Lock()
		syncPeer := m.syncPeer
		m.syncPeerMtx.Unlock()
		if syncPeer != nil {
			m.fetchNextBlocks(syncPeer)
		}
	}
}

// OnNotFound should be invoked from the sync manager with notfound messages
// that are received from remote peers.
//
// Currently, this primarily just removes the items from the request maps so
// they can eventually be requested from elsewhere.  This could be improved in
// the future to immediately request the reported items from another peer that
// has announced them.
//
// Ideally, this should be called from the same peer goroutine that received the
// message so the bulk of the processing is done concurrently and further reads
// from the peer are blocked until the message is processed.
//
// This function is safe for the concurrent access.
func (m *SyncManager) OnNotFound(peer *Peer, notFound *wire.MsgNotFound) {
	if m.shutdownRequested() {
		return
	}

	m.requestMtx.Lock()
	for _, inv := range notFound.InvList {
		// Verify the hash was actually announced by the peer before deleting
		// from the request maps.
		switch inv.Type {
		case wire.InvTypeBlock:
			if m.isRequestedBlockFromPeer(peer, &inv.Hash) {
				delete(m.requestedBlocks, inv.Hash)
			}
		case wire.InvTypeTx:
			if m.isRequestedTxFromPeer(peer, &inv.Hash) {
				delete(m.requestedTxns, inv.Hash)
			}
		case wire.InvTypeMix:
			if m.isRequestedMixMsgFromPeer(peer, &inv.Hash) {
				delete(m.requestedMixMsgs, inv.Hash)
			}
		}
	}
	m.requestMtx.Unlock()
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

	// No need for mix messages that are already available in the mixing pool or
	// were recently removed.
	if _, ok := m.cfg.MixPool.RecentMessage(hash); ok {
		return false
	}

	return true
}

// OnInv should be invoked with inventory announcements that are received from
// remote peers.
//
// Its primary purpose is to learn about the inventory advertised by the remote
// peer and to use that information to guide decisions regarding whether or not
// and when to request the relevant associated data from the peer.
//
// Ideally, this should be called from the same peer goroutine that received the
// message so the bulk of the processing is done concurrently and further reads
// from the peer are blocked until the message is processed.
//
// This function is safe for the concurrent access.
func (m *SyncManager) OnInv(peer *Peer, inv *wire.MsgInv) {
	if m.shutdownRequested() {
		return
	}

	isCurrent := m.IsCurrent()

	// Update state information regarding per-peer known inventory and determine
	// what inventory to request based on factors such as the current sync state
	// and whether or not the data is already available.
	//
	// Also, keep track of the final announced block (when there is one) so the
	// peer can be updated with that information accordingly.
	var lastBlock *wire.InvVect
	var requestQueue []*wire.InvVect
	for _, iv := range inv.InvList {
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
			m.requestMtx.Lock()
			if _, exists := m.requestedTxns[iv.Hash]; !exists {
				limitAdd(m.requestedTxns, iv.Hash, peer, maxRequestedTxns)
				requestQueue = append(requestQueue, iv)
			}
			m.requestMtx.Unlock()

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
			m.requestMtx.Lock()
			if _, exists := m.requestedMixMsgs[iv.Hash]; !exists {
				limitAdd(m.requestedMixMsgs, iv.Hash, peer, maxRequestedMixMsgs)
				requestQueue = append(requestQueue, iv)
			}
			m.requestMtx.Unlock()
		}
	}

	if lastBlock != nil {
		// Determine if the final announced block is already known to the local
		// chain and then either track it as the most recently announced block
		// by the peer that does not connect to any headers known to the local
		// chain or potentially make it the block with the most cumulative proof
		// of work announced by the peer when it is already known.
		if !m.cfg.Chain.HaveHeader(&lastBlock.Hash) {
			// Notice a copy of the hash is made here to avoid keeping a
			// reference into the inventory vector which would prevent it from
			// being GCd.
			lastBlockHash := lastBlock.Hash
			peer.bestAnnouncedMtx.Lock()
			m.maybeResolveOrphanBlock(peer)
			peer.announcedOrphanBlock = &lastBlockHash
			peer.bestAnnouncedMtx.Unlock()
		} else {
			header, err := m.cfg.Chain.HeaderByHash(&lastBlock.Hash)
			if err == nil {
				peer.bestAnnouncedMtx.Lock()
				m.maybeResolveOrphanBlock(peer)
				m.maybeUpdateBestAnnouncedBlock(peer, &lastBlock.Hash, &header)
				peer.bestAnnouncedMtx.Unlock()
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
func limitAdd(m map[chainhash.Hash]*Peer, hash chainhash.Hash, peer *Peer, limit int) {
	// Replace existing entries.
	if _, exists := m[hash]; exists {
		m[hash] = peer
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
	m[hash] = peer
}

// stallHandler monitors the header sync process to detect stalls and disconnect
// the sync peer which ensures clean recovery from stalls.
//
// It must be run as a goroutine.
func (m *SyncManager) stallHandler(ctx context.Context) {
out:
	for {
		select {
		case <-m.hdrSyncState.stallTimer.C:
			// Disconnect the sync peer due to stalling the header sync process.
			m.syncPeerMtx.Lock()
			syncPeer := m.syncPeer
			m.syncPeerMtx.Unlock()
			if syncPeer != nil {
				log.Debugf("Header sync progress stalled from peer %s -- "+
					"disconnecting", syncPeer)
				syncPeer.Disconnect()
			}

		case <-ctx.Done():
			break out
		}
	}

	log.Trace("Sync manager stall handler done")
}

// SyncPeerID returns the ID of the current sync peer, or 0 if there is none.
func (m *SyncManager) SyncPeerID() int32 {
	if m.shutdownRequested() {
		return 0
	}

	m.syncPeerMtx.Lock()
	syncPeer := m.syncPeer
	m.syncPeerMtx.Unlock()
	if syncPeer != nil {
		return syncPeer.ID()
	}

	return 0
}

// RequestFromPeer requests any combination of blocks, votes, and treasury
// spends from the given peer.  It ensures all of the requests are tracked so
// the peer is not banned for sending unrequested data when it responds.
//
// This function is safe for concurrent access.
func (m *SyncManager) RequestFromPeer(peer *Peer, blocks, voteHashes, tSpendHashes []chainhash.Hash) {
	if m.shutdownRequested() {
		return
	}

	defer m.requestMtx.Unlock()
	m.requestMtx.Lock()

	// Request as many needed blocks as possible at once.
	var numRequested uint32
	gdMsg := wire.NewMsgGetData()
	for i := range blocks {
		// Skip the block when it has already been requested or is already
		// known.
		blockHash := &blocks[i]
		if m.isRequestedBlock(blockHash) || m.cfg.Chain.HaveBlock(blockHash) {
			continue
		}

		gdMsg.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, blockHash))
		m.requestedBlocks[*blockHash] = peer
		numRequested++
		if numRequested == wire.MaxInvPerMsg {
			// Send full getdata message and reset.
			peer.QueueMessage(gdMsg, nil)
			gdMsg = wire.NewMsgGetData()
			numRequested = 0
		}
	}

	// Request as many needed votes and treasury spend transactions as possible
	// at once.
	for _, hashes := range [][]chainhash.Hash{voteHashes, tSpendHashes} {
		for i := range hashes {
			// Skip the transaction when it has already been requested or is
			// otherwise not needed.
			txHash := &hashes[i]
			_, alreadyRequested := m.requestedTxns[*txHash]
			if alreadyRequested || !m.needTx(txHash) {
				continue
			}

			gdMsg.AddInvVect(wire.NewInvVect(wire.InvTypeTx, txHash))
			m.requestedTxns[*txHash] = peer
			numRequested++
			if numRequested == wire.MaxInvPerMsg {
				// Send full getdata message and reset.
				peer.QueueMessage(gdMsg, nil)
				gdMsg = wire.NewMsgGetData()
				numRequested = 0
			}
		}
	}

	if len(gdMsg.InvList) > 0 {
		peer.QueueMessage(gdMsg, nil)
	}
}

// RequestMixMsgFromPeer requests the specified mix message from the given peer.
// It ensures all of the requests are tracked so the peer is not banned for
// sending unrequested data when it responds.
//
// This function is safe for concurrent access.
func (m *SyncManager) RequestMixMsgFromPeer(peer *Peer, mixHash *chainhash.Hash) {
	if m.shutdownRequested() {
		return
	}

	defer m.requestMtx.Unlock()
	m.requestMtx.Lock()

	// Skip mix messages that have already been requested or are otherwise not
	// needed.
	_, alreadyRequested := m.requestedMixMsgs[*mixHash]
	if alreadyRequested || !m.needMixMsg(mixHash) {
		return
	}

	gdMsg := wire.NewMsgGetDataSizeHint(1)
	gdMsg.AddInvVect(wire.NewInvVect(wire.InvTypeMix, mixHash))
	m.requestedMixMsgs[*mixHash] = peer
	peer.QueueMessage(gdMsg, nil)
}

// ProcessBlock processes the provided block using the chain instance associated
// with the sync manager.
//
// Blocks from all sources, such as those submitted to the RPC server and those
// found by the CPU miner, are expected to be processed using this method as
// opposed to directly invoking the method on the chain instance.
//
// Passing all blocks through the sync manager ensures they are all processed
// using the same code paths as blocks received from the network which helps
// enforce consistent handling and that the sync manager is always up-to-date in
// regards to the sync status of the chain.
//
// This function is safe for concurrent access.
func (m *SyncManager) ProcessBlock(block *dcrutil.Block) error {
	if m.shutdownRequested() {
		return nil
	}

	forkLen, err := m.processBlock(block)
	if err != nil {
		return err
	}

	onMainChain := forkLen == 0
	if onMainChain {
		// Prune invalidated transactions.
		best := m.cfg.Chain.BestSnapshot()
		m.cfg.TxMemPool.PruneStakeTx(best.NextStakeDiff, best.Height)
		m.cfg.TxMemPool.PruneExpiredTx(best.Height)
	}

	return nil
}

// IsCurrent returns whether or not the sync manager believes it is synced with
// the connected peers.
//
// This function is safe for concurrent access.
func (m *SyncManager) IsCurrent() bool {
	m.maybeUpdateIsCurrent()
	return m.isCurrent.Load()
}

// Run starts the sync manager and all other goroutines necessary for it to
// function properly and blocks until the provided context is cancelled.
func (m *SyncManager) Run(ctx context.Context) {
	log.Trace("Starting sync manager")

	// Start the stall handler goroutine.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		m.stallHandler(ctx)
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

	// MaxOutboundPeers specifies the maximum number of outbound peer the server
	// is expected to be connected with.
	MaxOutboundPeers uint64

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

	mgr := &SyncManager{
		cfg:              *config,
		rejectedTxns:     apbf.NewFilter(maxRejectedTxns, rejectedTxnsFPRate),
		rejectedMixMsgs:  apbf.NewFilter(maxRejectedMixMsgs, rejectedMixMsgsFPRate),
		requestedTxns:    make(map[chainhash.Hash]*Peer),
		requestedBlocks:  make(map[chainhash.Hash]*Peer),
		requestedMixMsgs: make(map[chainhash.Hash]*Peer),
		warnOnNoSync:     true,
		peers:            make(map[*Peer]struct{}),
		minKnownWork:     minKnownWork,
		hdrSyncState:     makeHeaderSyncState(),
		progressLogger:   progresslog.New("Processed", log),
		quit:             make(chan struct{}),
	}
	mgr.syncHeight.Store(config.Chain.BestSnapshot().Height)
	mgr.isCurrent.Store(config.Chain.IsCurrent())
	return mgr
}
