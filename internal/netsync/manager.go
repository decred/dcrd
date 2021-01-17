// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime/debug"
	"sync"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v4"
	"github.com/decred/dcrd/blockchain/v4"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/internal/mempool"
	"github.com/decred/dcrd/internal/progresslog"
	"github.com/decred/dcrd/internal/rpcserver"
	"github.com/decred/dcrd/lru"
	peerpkg "github.com/decred/dcrd/peer/v2"
	"github.com/decred/dcrd/wire"
)

const (
	// minInFlightBlocks is the minimum number of blocks that should be
	// in the request queue before requesting more.
	minInFlightBlocks = 10

	// maxInFlightBlocks is the maximum number of blocks to allow in the sync
	// peer request queue.
	maxInFlightBlocks = 16

	// maxRejectedTxns is the maximum number of rejected transactions
	// hashes to store in memory.
	maxRejectedTxns = 1000

	// maxRequestedBlocks is the maximum number of requested block
	// hashes to store in memory.
	maxRequestedBlocks = wire.MaxInvPerMsg

	// maxRequestedTxns is the maximum number of requested transactions
	// hashes to store in memory.
	maxRequestedTxns = wire.MaxInvPerMsg

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

// newPeerMsg signifies a newly connected peer to the event handler.
type newPeerMsg struct {
	peer *peerpkg.Peer
}

// blockMsg packages a Decred block message and the peer it came from together
// so the event handler has access to that information.
type blockMsg struct {
	block *dcrutil.Block
	peer  *peerpkg.Peer
	reply chan struct{}
}

// invMsg packages a Decred inv message and the peer it came from together
// so the event handler has access to that information.
type invMsg struct {
	inv  *wire.MsgInv
	peer *peerpkg.Peer
}

// headersMsg packages a Decred headers message and the peer it came from
// together so the event handler has access to that information.
type headersMsg struct {
	headers *wire.MsgHeaders
	peer    *peerpkg.Peer
}

// notFoundMsg packages a Decred notfound message and the peer it came from
// together so the event handler has access to that information.
type notFoundMsg struct {
	notFound *wire.MsgNotFound
	peer     *peerpkg.Peer
}

// donePeerMsg signifies a newly disconnected peer to the event handler.
type donePeerMsg struct {
	peer *peerpkg.Peer
}

// txMsg packages a Decred tx message and the peer it came from together
// so the event handler has access to that information.
type txMsg struct {
	tx    *dcrutil.Tx
	peer  *peerpkg.Peer
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
	peer         *peerpkg.Peer
	blocks       []*chainhash.Hash
	voteHashes   []*chainhash.Hash
	tSpendHashes []*chainhash.Hash
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
	flags blockchain.BehaviorFlags
	reply chan processBlockResponse
}

// processTransactionResponse is a response sent to the reply channel of a
// processTransactionMsg.
type processTransactionResponse struct {
	acceptedTxs []*dcrutil.Tx
	err         error
}

// processTransactionMsg is a message type to be sent across the message
// channel for requesting a transaction to be processed through the block
// manager.
type processTransactionMsg struct {
	tx            *dcrutil.Tx
	allowOrphans  bool
	rateLimit     bool
	allowHighFees bool
	tag           mempool.Tag
	reply         chan processTransactionResponse
}

// syncMgrPeer extends a peer to maintain additional state maintained by the
// sync manager.
type syncMgrPeer struct {
	*peerpkg.Peer

	syncCandidate   bool
	requestedTxns   map[chainhash.Hash]struct{}
	requestedBlocks map[chainhash.Hash]struct{}

	// numConsecutiveOrphanHeaders tracks the number of consecutive header
	// messages sent by the peer that contain headers which do not connect.  It
	// is used to detect peers that have either diverged so far they are no
	// longer useful or are otherwise being malicious.
	numConsecutiveOrphanHeaders int32
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
	// The following fields are used for lifecycle management of the sync
	// manager.
	wg   sync.WaitGroup
	quit chan struct{}

	// cfg specifies the configuration of the sync manager and is set at
	// creation time and treated as immutable after that.
	cfg Config

	rejectedTxns    map[chainhash.Hash]struct{}
	requestedTxns   map[chainhash.Hash]struct{}
	requestedBlocks map[chainhash.Hash]struct{}
	progressLogger  *progresslog.Logger
	syncPeer        *syncMgrPeer
	msgChan         chan interface{}
	peers           map[*peerpkg.Peer]*syncMgrPeer

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
}

// lookupPeer returns the sync manager peer that maintains additional state for
// a given base peer.  In the event the mapping does not exist, a warning is
// logged and nil is returned.
func lookupPeer(peer *peerpkg.Peer, peers map[*peerpkg.Peer]*syncMgrPeer) *syncMgrPeer {
	sp, ok := peers[peer]
	if !ok {
		log.Warnf("Attempt to lookup unknown peer %s\nStake: %v", peer,
			string(debug.Stack()))
		return nil
	}
	return sp
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

// fetchNextBlocks creates and sends a request to the provided peer for the next
// blocks to be downloaded based on the current headers.
func (m *SyncManager) fetchNextBlocks(peer *syncMgrPeer) {
	// Nothing to do if the target maximum number of blocks to request from the
	// peer at the same time are already in flight.
	numInFlight := len(peer.requestedBlocks)
	if numInFlight >= maxInFlightBlocks {
		return
	}

	// Determine the next blocks to download based on the final block that has
	// already been requested and the next blocks in the branch leading up to
	// best known header.
	chain := m.cfg.Chain
	maxNeeded := uint8(maxInFlightBlocks - numInFlight)
	neededBlocks := chain.NextNeededBlocks(maxNeeded, m.requestedBlocks)
	if len(neededBlocks) == 0 {
		return
	}

	// Ensure the number of needed blocks does not exceed the max inventory
	// per message.  This should never happen because the constants above limit
	// it to relatively small values.  However, the code below relies on this
	// assumption, so assert it.
	if len(neededBlocks) > wire.MaxInvPerMsg {
		log.Warnf("%d needed blocks exceeds max allowed per message",
			len(neededBlocks))
		return
	}

	// Build and send a getdata request for needed blocks.
	gdmsg := wire.NewMsgGetDataSizeHint(uint(len(neededBlocks)))
	for i := range neededBlocks {
		hash := neededBlocks[i]
		iv := wire.NewInvVect(wire.InvTypeBlock, hash)
		m.requestedBlocks[*hash] = struct{}{}
		peer.requestedBlocks[*hash] = struct{}{}
		gdmsg.AddInvVect(iv)
	}
	peer.QueueMessage(gdmsg, nil)

}

// startSync will choose the best peer among the available candidate peers to
// download/sync the blockchain from.  When syncing is already running, it
// simply returns.  It also examines the candidates for any which are no longer
// candidates and removes them as needed.
func (m *SyncManager) startSync() {
	// Nothing more to do when already syncing.
	if m.syncPeer != nil {
		return
	}

	chain := m.cfg.Chain
	best := chain.BestSnapshot()
	var bestPeer *syncMgrPeer
	for _, peer := range m.peers {
		if !peer.syncCandidate {
			continue
		}

		// Remove sync candidate peers that are no longer candidates due
		// to passing their latest known block.  NOTE: The < is
		// intentional as opposed to <=.  While technically the peer
		// doesn't have a later block when it's equal, it will likely
		// have one soon so it is a reasonable choice.  It also allows
		// the case where both are at 0 such as during regression test.
		if peer.LastBlock() < best.Height {
			peer.syncCandidate = false
			continue
		}

		// The best sync candidate is the most updated peer.
		if bestPeer == nil {
			bestPeer = peer
		}
		if bestPeer.LastBlock() < peer.LastBlock() {
			bestPeer = peer
		}
	}

	// Update the state of whether or not the manager believes the chain is
	// fully synced to whatever the chain believes when there is no candidate
	// for a sync peer.
	//
	// Also, return now when there isn't a sync peer candidate as there is
	// nothing more to do without one.
	if bestPeer == nil {
		m.isCurrentMtx.Lock()
		m.isCurrent = chain.IsCurrent()
		m.isCurrentMtx.Unlock()
		log.Warnf("No sync peer candidates available")
		return
	}

	// Start syncing from the best peer.

	// Clear the requestedBlocks if the sync peer changes, otherwise
	// we may ignore blocks we need that the last sync peer failed
	// to send.
	m.requestedBlocks = make(map[chainhash.Hash]struct{})

	syncHeight := bestPeer.LastBlock()

	headersSynced := m.hdrSyncState.headersSynced
	if !headersSynced {
		log.Infof("Syncing headers to block height %d from peer %v", syncHeight,
			bestPeer)
	}

	// The chain is not synced whenever the current best height is less than the
	// height to sync to.
	if best.Height < syncHeight {
		m.isCurrentMtx.Lock()
		m.isCurrent = false
		m.isCurrentMtx.Unlock()
	}

	// Request headers to discover any blocks that are not already known
	// starting from the parent of the best known header for the local chain.
	// The parent is used as a means to accurately discover the best known block
	// of the remote peer in the case both tips are the same where it would
	// otherwise result in an empty response.
	bestHeaderHash, _ := chain.BestHeader()
	parentHash := bestHeaderHash
	header, err := chain.HeaderByHash(&bestHeaderHash)
	if err == nil {
		parentHash = header.PrevBlock
	}
	blkLocator := chain.BlockLocatorFromHash(&parentHash)
	locator := chainBlockLocatorToHashes(blkLocator)
	bestPeer.PushGetHeadersMsg(locator, &zeroHash)

	// Track the sync peer and update the sync height when it is higher than the
	// currently best known value.
	m.syncPeer = bestPeer
	m.syncHeightMtx.Lock()
	if syncHeight > m.syncHeight {
		m.syncHeight = syncHeight
	}
	m.syncHeightMtx.Unlock()

	// Start the header sync progress stall timeout when the initial headers
	// sync is not already done.
	if !headersSynced {
		m.hdrSyncState.resetStallTimeout()
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

// isSyncCandidate returns whether or not the peer is a candidate to consider
// syncing from.
func (m *SyncManager) isSyncCandidate(peer *peerpkg.Peer) bool {
	// The peer is not a candidate for sync if it's not a full node.
	return peer.Services()&wire.SFNodeNetwork == wire.SFNodeNetwork
}

// syncMiningStateAfterSync polls the sync manager for the current sync state
// and once the manager believes the chain is fully synced, it executes a call
// to the peer to sync the mining state.
func (m *SyncManager) syncMiningStateAfterSync(peer *peerpkg.Peer) {
	go func() {
		for {
			select {
			case <-time.After(3 * time.Second):
			case <-m.quit:
				return
			}

			if !peer.Connected() {
				return
			}
			if !m.IsCurrent() {
				continue
			}

			pver := peer.ProtocolVersion()
			var msg wire.Message

			switch {
			case pver < wire.InitStateVersion:
				// Before protocol version InitStateVersion
				// nodes use the GetMiningState/MiningState
				// pair of p2p messages.
				msg = wire.NewMsgGetMiningState()

			default:
				// On the most recent protocol version, nodes
				// use the GetInitState/InitState pair of p2p
				// messages.
				m := wire.NewMsgGetInitState()
				err := m.AddTypes(wire.InitStateHeadBlocks,
					wire.InitStateHeadBlockVotes,
					wire.InitStateTSpends)
				if err != nil {
					log.Errorf("Unexpected error while building getinitstate "+
						"msg: %v", err)
					return
				}
				msg = m
			}

			peer.QueueMessage(msg, nil)
			return
		}
	}()
}

// handleNewPeerMsg deals with new peers that have signalled they may
// be considered as a sync peer (they have already successfully negotiated).  It
// also starts syncing if needed.  It is invoked from the syncHandler goroutine.
func (m *SyncManager) handleNewPeerMsg(ctx context.Context, peer *peerpkg.Peer) {
	select {
	case <-ctx.Done():
	default:
	}

	log.Infof("New valid peer %s (%s)", peer, peer.UserAgent())

	// Initialize the peer state
	isSyncCandidate := m.isSyncCandidate(peer)
	m.peers[peer] = &syncMgrPeer{
		Peer:            peer,
		syncCandidate:   isSyncCandidate,
		requestedTxns:   make(map[chainhash.Hash]struct{}),
		requestedBlocks: make(map[chainhash.Hash]struct{}),
	}

	// Start syncing by choosing the best candidate if needed.
	if isSyncCandidate && m.syncPeer == nil {
		m.startSync()
	}

	// Grab the mining state from this peer once synced when enabled.
	if !m.cfg.NoMiningStateSync {
		m.syncMiningStateAfterSync(peer)
	}
}

// handleDonePeerMsg deals with peers that have signalled they are done.  It
// removes the peer as a candidate for syncing and in the case where it was
// the current sync peer, attempts to select a new best peer to sync from.  It
// is invoked from the syncHandler goroutine.
func (m *SyncManager) handleDonePeerMsg(p *peerpkg.Peer) {
	peer := lookupPeer(p, m.peers)
	if peer == nil {
		return
	}

	// Remove the peer from the list of candidate peers.
	delete(m.peers, p)

	// Remove requested transactions from the global map so that they will
	// be fetched from elsewhere.
	for txHash := range peer.requestedTxns {
		delete(m.requestedTxns, txHash)
	}

	// Remove requested blocks from the global map so that they will be
	// fetched from elsewhere.
	// TODO(oga) we could possibly here check which peers have these blocks
	// and request them now to speed things up a little.
	for blockHash := range peer.requestedBlocks {
		delete(m.requestedBlocks, blockHash)
	}

	// Attempt to find a new peer to sync from and reset the final requested
	// block when the quitting peer is the sync peer.
	if m.syncPeer == peer {
		m.syncPeer = nil
		m.startSync()
	}
}

// errToWireRejectCode determines the wire rejection code and description for a
// given error. This function can convert some select blockchain and mempool
// error types to the historical rejection codes used on the p2p wire protocol.
func errToWireRejectCode(err error) (wire.RejectCode, string) {
	// The default reason to reject a transaction/block is due to it being
	// invalid somehow.
	code := wire.RejectInvalid
	var reason string

	// Convert recognized errors to a reject code.
	switch {
	// Rejected due to duplicate.
	case errors.Is(err, blockchain.ErrDuplicateBlock):
		code = wire.RejectDuplicate
		reason = err.Error()

	// Rejected due to obsolete version.
	case errors.Is(err, blockchain.ErrBlockVersionTooOld):
		code = wire.RejectObsolete
		reason = err.Error()

	// Rejected due to checkpoint.
	case errors.Is(err, blockchain.ErrCheckpointTimeTooOld),
		errors.Is(err, blockchain.ErrDifficultyTooLow),
		errors.Is(err, blockchain.ErrBadCheckpoint),
		errors.Is(err, blockchain.ErrForkTooOld):
		code = wire.RejectCheckpoint
		reason = err.Error()

	// Error codes which map to a duplicate transaction already
	// mined or in the mempool.
	case errors.Is(err, mempool.ErrMempoolDoubleSpend),
		errors.Is(err, mempool.ErrAlreadyVoted),
		errors.Is(err, mempool.ErrDuplicate),
		errors.Is(err, mempool.ErrTooManyVotes),
		errors.Is(err, mempool.ErrDuplicateRevocation),
		errors.Is(err, mempool.ErrAlreadyExists),
		errors.Is(err, mempool.ErrOrphan):
		code = wire.RejectDuplicate
		reason = err.Error()

	// Error codes which map to a non-standard transaction being
	// relayed.
	case errors.Is(err, mempool.ErrOrphanPolicyViolation),
		errors.Is(err, mempool.ErrOldVote),
		errors.Is(err, mempool.ErrSeqLockUnmet),
		errors.Is(err, mempool.ErrNonStandard):
		code = wire.RejectNonstandard
		reason = err.Error()

	// Error codes which map to an insufficient fee being paid.
	case errors.Is(err, mempool.ErrInsufficientFee),
		errors.Is(err, mempool.ErrInsufficientPriority):
		code = wire.RejectInsufficientFee
		reason = err.Error()

	// Error codes which map to an attempt to create dust outputs.
	case errors.Is(err, mempool.ErrDustOutput):
		code = wire.RejectDust
		reason = err.Error()

	default:
		reason = fmt.Sprintf("rejected: %v", err)
	}
	return code, reason
}

// handleTxMsg handles transaction messages from all peers.
func (m *SyncManager) handleTxMsg(tmsg *txMsg) {
	peer := lookupPeer(tmsg.peer, m.peers)
	if peer == nil {
		return
	}

	// NOTE:  BitcoinJ, and possibly other wallets, don't follow the spec of
	// sending an inventory message and allowing the remote peer to decide
	// whether or not they want to request the transaction via a getdata
	// message.  Unfortunately, the reference implementation permits
	// unrequested data, so it has allowed wallets that don't follow the
	// spec to proliferate.  While this is not ideal, there is no check here
	// to disconnect peers for sending unsolicited transactions to provide
	// interoperability.
	txHash := tmsg.tx.Hash()

	// Ignore transactions that we have already rejected.  Do not
	// send a reject message here because if the transaction was already
	// rejected, the transaction was unsolicited.
	if _, exists := m.rejectedTxns[*txHash]; exists {
		log.Debugf("Ignoring unsolicited previously rejected transaction %v "+
			"from %s", txHash, peer)
		return
	}

	// Process the transaction to include validation, insertion in the
	// memory pool, orphan handling, etc.
	allowOrphans := m.cfg.MaxOrphanTxs > 0
	acceptedTxs, err := m.cfg.TxMemPool.ProcessTransaction(tmsg.tx,
		allowOrphans, true, true, mempool.Tag(peer.ID()))

	// Remove transaction from request maps. Either the mempool/chain
	// already knows about it and as such we shouldn't have any more
	// instances of trying to fetch it, or we failed to insert and thus
	// we'll retry next time we get an inv.
	delete(peer.requestedTxns, *txHash)
	delete(m.requestedTxns, *txHash)

	if err != nil {
		// Do not request this transaction again until a new block
		// has been processed.
		limitAdd(m.rejectedTxns, *txHash, maxRejectedTxns)

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

		// Convert the error into an appropriate reject message and
		// send it.
		code, reason := errToWireRejectCode(err)
		peer.PushRejectMsg(wire.CmdTx, code, reason, txHash, false)
		return
	}

	m.cfg.PeerNotifier.AnnounceNewTransactions(acceptedTxs)
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

// processBlock processes the provided block using the internal chain instance.
//
// When no errors occurred during processing, the first return value indicates
// the length of the fork the block extended.  In the case it either extended
// the best chain or is now the tip of the best chain due to causing a
// reorganize, the fork length will be 0.  Orphans are rejected and can be
// detected by checking if the error is blockchain.ErrMissingParent.
func (m *SyncManager) processBlock(block *dcrutil.Block, flags blockchain.BehaviorFlags) (int64, error) {
	// Process the block to include validation, best chain selection, etc.
	forkLen, err := m.cfg.Chain.ProcessBlock(block, flags)
	if err != nil {
		return 0, err
	}
	m.isCurrentMtx.Lock()
	m.maybeUpdateIsCurrent()
	m.isCurrentMtx.Unlock()

	return forkLen, nil
}

// handleBlockMsg handles block messages from all peers.
func (m *SyncManager) handleBlockMsg(bmsg *blockMsg) {
	peer := lookupPeer(bmsg.peer, m.peers)
	if peer == nil {
		return
	}

	// If we didn't ask for this block then the peer is misbehaving.
	blockHash := bmsg.block.Hash()
	if _, exists := peer.requestedBlocks[*blockHash]; !exists {
		log.Warnf("Got unrequested block %v from %s -- disconnecting",
			blockHash, peer)
		peer.Disconnect()
		return
	}

	// Process the block to include validation, best chain selection, etc.
	//
	// Also, remove the block from the request maps once it has been processed.
	// This ensures chain is aware of the block before it is removed from the
	// maps in order to help prevent duplicate requests.
	forkLen, err := m.processBlock(bmsg.block, blockchain.BFNone)
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
		if errors.Is(err, database.ErrCorruption) {
			log.Errorf("Critical failure: %v", err)
		}

		// Convert the error into an appropriate reject message and send it.
		code, reason := errToWireRejectCode(err)
		peer.PushRejectMsg(wire.CmdBlock, code, reason, blockHash, false)
		return
	}

	// Log information about the block and update the chain state.
	msgBlock := bmsg.block.MsgBlock()
	forceLog := int64(msgBlock.Header.Height) >= m.SyncHeight()
	m.progressLogger.LogProgress(msgBlock, forceLog)

	onMainChain := forkLen == 0
	if onMainChain {
		// Prune invalidated transactions.
		best := m.cfg.Chain.BestSnapshot()
		m.cfg.TxMemPool.PruneStakeTx(best.NextStakeDiff, best.Height)
		m.cfg.TxMemPool.PruneExpiredTx()

		// Clear the rejected transactions.
		m.rejectedTxns = make(map[chainhash.Hash]struct{})
	}

	// Update the latest block height for the peer to avoid stale heights when
	// looking for future potential sync node candidacy.
	//
	// Also, when the chain is considered current and the block was accepted to
	// the main chain, update the heights of other peers whose invs may have
	// been ignored when actively syncing while the chain was not yet current or
	// lost the lock announcement race.
	blockHeight := int64(bmsg.block.MsgBlock().Header.Height)
	peer.UpdateLastBlockHeight(blockHeight)
	if onMainChain && m.IsCurrent() {
		for _, p := range m.peers {
			// The height for the sending peer is already updated.
			if p == peer {
				continue
			}

			lastAnnBlock := p.LastAnnouncedBlock()
			if lastAnnBlock != nil && *lastAnnBlock == *blockHash {
				p.UpdateLastBlockHeight(blockHeight)
				p.UpdateLastAnnouncedBlock(nil)
			}
		}
	}

	// Request more blocks using the headers when the request queue is getting
	// short.
	if peer == m.syncPeer && len(peer.requestedBlocks) < minInFlightBlocks {
		m.fetchNextBlocks(peer)
	}
}

// handleHeadersMsg handles headers messages from all peers.
func (m *SyncManager) handleHeadersMsg(hmsg *headersMsg) {
	peer := lookupPeer(hmsg.peer, m.peers)
	if peer == nil {
		return
	}

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
		// Ignore headers that do not connect to any known headers when the
		// initial headers sync is taking place.  It is expected that headers
		// will be announced that are not yet known.
		if !headersSynced {
			return
		}

		// Attempt to detect block announcements which do not connect to any
		// known headers and request any headers starting from the best header
		// the local chain knows in order to (hopefully) discover the missing
		// headers.
		//
		// Meanwhile, also keep track of how many times the peer has
		// consecutively sent a headers message that does not connect and
		// disconnect it once the max allowed threshold has been reached.
		if numHeaders < maxExpectedHeaderAnnouncementsPerMsg {
			peer.numConsecutiveOrphanHeaders++
			if peer.numConsecutiveOrphanHeaders >= maxConsecutiveOrphanHeaders {
				log.Debugf("Received %d consecutive headers messages that do "+
					"not connect from peer %s -- disconnecting",
					peer.numConsecutiveOrphanHeaders, peer)
				peer.Disconnect()
			}

			log.Debugf("Requesting missing parents for header %s (height %d) "+
				"received from peer %s", firstHeaderHash, firstHeader.Height,
				peer)
			bestHeaderHash, _ := chain.BestHeader()
			blkLocator := chain.BlockLocatorFromHash(&bestHeaderHash)
			locator := chainBlockLocatorToHashes(blkLocator)
			peer.PushGetHeadersMsg(locator, &zeroHash)

			return
		}

		// The initial headers sync process is done and this does not appear to
		// be a block announcement, so disconnect the peer.
		log.Debugf("Received orphan header from peer %s -- disconnecting", peer)
		peer.Disconnect()
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
		err := chain.ProcessBlockHeader(header, blockchain.BFNone)
		if err != nil {
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

	// Update the last announced block to the final one in the announced headers
	// above and update the height for the peer too.
	finalHeader := headers[len(headers)-1]
	finalReceivedHash := &headerHashes[len(headerHashes)-1]
	peer.UpdateLastAnnouncedBlock(finalReceivedHash)
	peer.UpdateLastBlockHeight(int64(finalHeader.Height))

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
		minKnownWork := m.cfg.ChainParams.MinKnownChainWork
		if minKnownWork != nil {
			workSum, err := chain.ChainWork(finalReceivedHash)
			if err == nil && workSum.Cmp(minKnownWork) < 0 {
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
	if !headersSynced && peer == m.syncPeer {
		const syncHeightFetchOffset = 6
		if int64(finalHeader.Height)+syncHeightFetchOffset > syncHeight {
			headersSynced = true
			m.hdrSyncState.headersSynced = headersSynced
			m.hdrSyncState.stopStallTimeout()

			log.Infof("Initial headers sync complete (best header hash %s, "+
				"height %d)", newBestHeaderHash, newBestHeaderHeight)
			log.Info("Syncing chain")
			m.progressLogger.SetLastLogTime(time.Now())

			// Potentially update whether the chain believes it is current now
			// that the headers are synced.
			chain.MaybeUpdateIsCurrent()
			isChainCurrent = chain.IsCurrent()
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
	// and associated infrastructure to efficiently determinine which peers have
	// the associated block(s).
	if isChainCurrent {
		gdmsg := wire.NewMsgGetDataSizeHint(uint(len(headers)))
		for i := range headerHashes {
			// Skip the block when it has already been requested or is otherwise
			// already known.
			hash := &headerHashes[i]
			_, isRequestedBlock := m.requestedBlocks[*hash]
			if isRequestedBlock || chain.HaveBlock(hash) {
				continue
			}

			iv := wire.NewInvVect(wire.InvTypeBlock, hash)
			limitAdd(m.requestedBlocks, *hash, maxRequestedBlocks)
			limitAdd(peer.requestedBlocks, *hash, maxRequestedBlocks)
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
	peer := lookupPeer(nfmsg.peer, m.peers)
	if peer == nil {
		return
	}

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
		}
	}
}

// needTx returns whether or not the transaction needs to be downloaded.  For
// example, it does not need to be downloaded when it is already known.
func (m *SyncManager) needTx(hash *chainhash.Hash) bool {
	// No need for transactions that have already been rejected.
	if _, exists := m.rejectedTxns[*hash]; exists {
		return false
	}

	// No need for transactions that are already available in the transaction
	// memory pool (main pool or orphan).
	if m.cfg.TxMemPool.HaveTransaction(hash) {
		return false
	}

	// No need for transactions that were recently confirmed.
	if m.cfg.RecentlyConfirmedTxns.Contains(*hash) {
		return false
	}

	return false
}

// handleInvMsg handles inv messages from all peers.  This entails examining the
// inventory advertised by the remote peer for block and transaction
// announcements and acting accordingly.
func (m *SyncManager) handleInvMsg(imsg *invMsg) {
	peer := lookupPeer(imsg.peer, m.peers)
	if peer == nil {
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
		}
	}

	if lastBlock != nil {
		// Update the last announced block to the final one in the announced
		// inventory above (if any).  In the case the header for that block is
		// already known, use that information to update the height for the peer
		// too.
		peer.UpdateLastAnnouncedBlock(&lastBlock.Hash)
		if isCurrent {
			header, err := m.cfg.Chain.HeaderByHash(&lastBlock.Hash)
			if err == nil {
				peer.UpdateLastBlockHeight(int64(header.Height))
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
			case *newPeerMsg:
				m.handleNewPeerMsg(ctx, msg.peer)

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

			case *invMsg:
				m.handleInvMsg(msg)

			case *headersMsg:
				m.handleHeadersMsg(msg)

			case *notFoundMsg:
				m.handleNotFoundMsg(msg)

			case *donePeerMsg:
				m.handleDonePeerMsg(msg.peer)

			case getSyncPeerMsg:
				var peerID int32
				if m.syncPeer != nil {
					peerID = m.syncPeer.ID()
				}
				msg.reply <- peerID

			case requestFromPeerMsg:
				err := m.requestFromPeer(msg.peer, msg.blocks, msg.voteHashes,
					msg.tSpendHashes)
				msg.reply <- requestFromPeerResponse{
					err: err,
				}

			case processBlockMsg:
				forkLen, err := m.processBlock(msg.block, msg.flags)
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
					m.cfg.TxMemPool.PruneExpiredTx()
				}

				msg.reply <- processBlockResponse{
					err: nil,
				}

			case processTransactionMsg:
				acceptedTxs, err := m.cfg.TxMemPool.ProcessTransaction(msg.tx,
					msg.allowOrphans, msg.rateLimit, msg.allowHighFees, msg.tag)
				msg.reply <- processTransactionResponse{
					acceptedTxs: acceptedTxs,
					err:         err,
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

	m.wg.Done()
	log.Trace("Sync manager event handler done")
}

// NewPeer informs the sync manager of a newly active peer.
func (m *SyncManager) NewPeer(peer *peerpkg.Peer) {
	select {
	case m.msgChan <- &newPeerMsg{peer: peer}:
	case <-m.quit:
	}
}

// QueueTx adds the passed transaction message and peer to the event handling
// queue.
func (m *SyncManager) QueueTx(tx *dcrutil.Tx, peer *peerpkg.Peer, done chan struct{}) {
	select {
	case m.msgChan <- &txMsg{tx: tx, peer: peer, reply: done}:
	case <-m.quit:
		done <- struct{}{}
	}
}

// QueueBlock adds the passed block message and peer to the event handling
// queue.
func (m *SyncManager) QueueBlock(block *dcrutil.Block, peer *peerpkg.Peer, done chan struct{}) {
	select {
	case m.msgChan <- &blockMsg{block: block, peer: peer, reply: done}:
	case <-m.quit:
		done <- struct{}{}
	}
}

// QueueInv adds the passed inv message and peer to the event handling queue.
func (m *SyncManager) QueueInv(inv *wire.MsgInv, peer *peerpkg.Peer) {
	select {
	case m.msgChan <- &invMsg{inv: inv, peer: peer}:
	case <-m.quit:
	}
}

// QueueHeaders adds the passed headers message and peer to the event handling
// queue.
func (m *SyncManager) QueueHeaders(headers *wire.MsgHeaders, peer *peerpkg.Peer) {
	select {
	case m.msgChan <- &headersMsg{headers: headers, peer: peer}:
	case <-m.quit:
	}
}

// QueueNotFound adds the passed notfound message and peer to the event handling
// queue.
func (m *SyncManager) QueueNotFound(notFound *wire.MsgNotFound, peer *peerpkg.Peer) {
	select {
	case m.msgChan <- &notFoundMsg{notFound: notFound, peer: peer}:
	case <-m.quit:
	}
}

// DonePeer informs the sync manager that a peer has disconnected.
func (m *SyncManager) DonePeer(peer *peerpkg.Peer) {
	select {
	case m.msgChan <- &donePeerMsg{peer: peer}:
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
func (m *SyncManager) RequestFromPeer(p *peerpkg.Peer, blocks, voteHashes,
	tSpendHashes []*chainhash.Hash) error {

	reply := make(chan requestFromPeerResponse, 1)
	request := requestFromPeerMsg{
		peer:         p,
		blocks:       blocks,
		voteHashes:   voteHashes,
		tSpendHashes: tSpendHashes,
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

func (m *SyncManager) requestFromPeer(p *peerpkg.Peer, blocks, voteHashes,
	tSpendHashes []*chainhash.Hash) error {

	peer := lookupPeer(p, m.peers)
	if peer == nil {
		return fmt.Errorf("unknown peer %s", p)
	}

	// Add the blocks to the request.
	msgResp := wire.NewMsgGetData()
	for _, bh := range blocks {
		// If we've already requested this block, skip it.
		_, alreadyReqP := peer.requestedBlocks[*bh]
		_, alreadyReqB := m.requestedBlocks[*bh]

		if alreadyReqP || alreadyReqB {
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
		m.requestedBlocks[*bh] = struct{}{}
	}

	addTxsToRequest := func(txs []*chainhash.Hash, txType stake.TxType) error {
		// Return immediately if txs is nil.
		if txs == nil {
			return nil
		}

		for _, tx := range txs {
			// If we've already requested this transaction, skip it.
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

	if len(msgResp.InvList) > 0 {
		p.QueueMessage(msgResp, nil)
	}

	return nil
}

// ProcessBlock makes use of ProcessBlock on an internal instance of a block
// chain.  It is funneled through the sync manager since blockchain is not safe
// for concurrent access.
func (m *SyncManager) ProcessBlock(block *dcrutil.Block, flags blockchain.BehaviorFlags) error {
	reply := make(chan processBlockResponse, 1)
	select {
	case m.msgChan <- processBlockMsg{block: block, flags: flags, reply: reply}:
	case <-m.quit:
	}

	select {
	case response := <-reply:
		return response.err
	case <-m.quit:
		return fmt.Errorf("sync manager stopped")
	}
}

// ProcessTransaction makes use of ProcessTransaction on an internal instance of
// a block chain.  It is funneled through the sync manager since blockchain is
// not safe for concurrent access.
func (m *SyncManager) ProcessTransaction(tx *dcrutil.Tx, allowOrphans bool,
	rateLimit bool, allowHighFees bool, tag mempool.Tag) ([]*dcrutil.Tx, error) {

	reply := make(chan processTransactionResponse, 1)
	select {
	case m.msgChan <- processTransactionMsg{tx, allowOrphans, rateLimit,
		allowHighFees, tag, reply}:
	case <-m.quit:
	}

	select {
	case response := <-reply:
		return response.acceptedTxs, response.err
	case <-m.quit:
		return nil, fmt.Errorf("sync manager stopped")
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
	m.wg.Add(1)
	go m.eventHandler(ctx)

	// Shutdown the sync manager when the context is cancelled.
	m.wg.Add(1)
	go func(ctx context.Context) {
		<-ctx.Done()
		close(m.quit)
		m.wg.Done()
	}(ctx)

	m.wg.Wait()
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

	// TxMemPool specifies the mempool to use for processing transactions.
	TxMemPool *mempool.TxPool

	// RpcServer returns an instance of an RPC server to use for notifications.
	// It may return nil if there is no active RPC server.
	RpcServer func() *rpcserver.Server

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
	RecentlyConfirmedTxns *lru.Cache
}

// New returns a new network chain synchronization manager.  Use Run to begin
// processing asynchronous events.
func New(config *Config) *SyncManager {
	return &SyncManager{
		cfg:             *config,
		rejectedTxns:    make(map[chainhash.Hash]struct{}),
		requestedTxns:   make(map[chainhash.Hash]struct{}),
		requestedBlocks: make(map[chainhash.Hash]struct{}),
		peers:           make(map[*peerpkg.Peer]*syncMgrPeer),
		hdrSyncState:    makeHeaderSyncState(),
		progressLogger:  progresslog.New("Processed", log),
		msgChan:         make(chan interface{}, config.MaxPeers*3),
		quit:            make(chan struct{}),
		syncHeight:      config.Chain.BestSnapshot().Height,
		isCurrent:       config.Chain.IsCurrent(),
	}
}
