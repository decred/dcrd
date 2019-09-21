// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/list"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/blockchain/standalone"
	"github.com/decred/dcrd/blockchain/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v2"
	"github.com/decred/dcrd/database/v2"
	"github.com/decred/dcrd/dcrutil/v2"
	"github.com/decred/dcrd/fees/v2"
	"github.com/decred/dcrd/mempool/v3"
	"github.com/decred/dcrd/wire"
)

const (
	// minInFlightBlocks is the minimum number of blocks that should be
	// in the request queue for headers-first mode before requesting
	// more.
	minInFlightBlocks = 10

	// blockDbNamePrefix is the prefix for the block database name.  The
	// database type is appended to this value to form the full block
	// database name.
	blockDbNamePrefix = "blocks"

	// maxRejectedTxns is the maximum number of rejected transactions
	// hashes to store in memory.
	maxRejectedTxns = 1000

	// maxRequestedBlocks is the maximum number of requested block
	// hashes to store in memory.
	maxRequestedBlocks = wire.MaxInvPerMsg

	// maxRequestedTxns is the maximum number of requested transactions
	// hashes to store in memory.
	maxRequestedTxns = wire.MaxInvPerMsg

	// maxReorgDepthNotify specifies the maximum reorganization depth for
	// which winning ticket notifications will be sent over RPC.  The reorg
	// depth is the number of blocks that would be reorganized out of the
	// current best chain if a side chain being considered for notifications
	// were to ultimately be extended to be longer than the current one.
	//
	// In effect, this helps to prevent large reorgs by refusing to send the
	// winning ticket information to RPC clients, such as voting wallets,
	// which depend on it to cast votes.
	//
	// This check also doubles to help reduce exhaustion attacks that could
	// otherwise arise from sending old orphan blocks and forcing nodes to
	// do expensive lottery data calculations for them.
	maxReorgDepthNotify = 6
)

// zeroHash is the zero value hash (all zeros).  It is defined as a convenience.
var zeroHash chainhash.Hash

// newPeerMsg signifies a newly connected peer to the block handler.
type newPeerMsg struct {
	peer *serverPeer
}

// blockMsg packages a Decred block message and the peer it came from together
// so the block handler has access to that information.
type blockMsg struct {
	block *dcrutil.Block
	peer  *serverPeer
}

// invMsg packages a Decred inv message and the peer it came from together
// so the block handler has access to that information.
type invMsg struct {
	inv  *wire.MsgInv
	peer *serverPeer
}

// headersMsg packages a Decred headers message and the peer it came from
// together so the block handler has access to that information.
type headersMsg struct {
	headers *wire.MsgHeaders
	peer    *serverPeer
}

// donePeerMsg signifies a newly disconnected peer to the block handler.
type donePeerMsg struct {
	peer *serverPeer
}

// txMsg packages a Decred tx message and the peer it came from together
// so the block handler has access to that information.
type txMsg struct {
	tx   *dcrutil.Tx
	peer *serverPeer
}

// getSyncPeerMsg is a message type to be sent across the message channel for
// retrieving the current sync peer.
type getSyncPeerMsg struct {
	reply chan *serverPeer
}

// requestFromPeerMsg is a message type to be sent across the message channel
// for requesting either blocks or transactions from a given peer. It routes
// this through the block manager so the block manager doesn't ban the peer
// when it sends this information back.
type requestFromPeerMsg struct {
	peer   *serverPeer
	blocks []*chainhash.Hash
	txs    []*chainhash.Hash
	reply  chan requestFromPeerResponse
}

// requestFromPeerResponse is a response sent to the reply channel of a
// requestFromPeerMsg query.
type requestFromPeerResponse struct {
	err error
}

// calcNextReqDifficultyResponse is a response sent to the reply channel of a
// calcNextReqDifficultyMsg query.
type calcNextReqDifficultyResponse struct {
	difficulty uint32
	err        error
}

// calcNextReqDifficultyMsg is a message type to be sent across the message
// channel for requesting the required difficulty of the next block.
type calcNextReqDifficultyMsg struct {
	timestamp time.Time
	reply     chan calcNextReqDifficultyResponse
}

// calcNextReqDiffNodeMsg is a message type to be sent across the message
// channel for requesting the required difficulty for some block building on
// the given block hash.
type calcNextReqDiffNodeMsg struct {
	hash      *chainhash.Hash
	timestamp time.Time
	reply     chan calcNextReqDifficultyResponse
}

// calcNextReqStakeDifficultyResponse is a response sent to the reply channel of a
// calcNextReqStakeDifficultyMsg query.
type calcNextReqStakeDifficultyResponse struct {
	stakeDifficulty int64
	err             error
}

// calcNextReqStakeDifficultyMsg is a message type to be sent across the message
// channel for requesting the required stake difficulty of the next block.
type calcNextReqStakeDifficultyMsg struct {
	reply chan calcNextReqStakeDifficultyResponse
}

// tipGenerationResponse is a response sent to the reply channel of a
// tipGenerationMsg query.
type tipGenerationResponse struct {
	hashes []chainhash.Hash
	err    error
}

// tipGenerationMsg is a message type to be sent across the message
// channel for requesting the required the entire generation of a
// block node.
type tipGenerationMsg struct {
	reply chan tipGenerationResponse
}

// forceReorganizationResponse is a response sent to the reply channel of a
// forceReorganizationMsg query.
type forceReorganizationResponse struct {
	err error
}

// forceReorganizationMsg is a message type to be sent across the message
// channel for requesting that the block on head be reorganized to one of its
// adjacent orphans.
type forceReorganizationMsg struct {
	formerBest chainhash.Hash
	newBest    chainhash.Hash
	reply      chan forceReorganizationResponse
}

// processBlockResponse is a response sent to the reply channel of a
// processBlockMsg.
type processBlockResponse struct {
	forkLen  int64
	isOrphan bool
	err      error
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
	reply         chan processTransactionResponse
}

// isCurrentMsg is a message type to be sent across the message channel for
// requesting whether or not the block manager believes it is synced with
// the currently connected peers.
type isCurrentMsg struct {
	reply chan bool
}

// headerNode is used as a node in a list of headers that are linked together
// between checkpoints.
type headerNode struct {
	height int64
	hash   *chainhash.Hash
}

// PeerNotifier provides an interface for server peer notifications.
type PeerNotifier interface {
	// AnnounceNewTransactions generates and relays inventory vectors and
	// notifies websocket clients of the passed transactions.
	AnnounceNewTransactions(txns []*dcrutil.Tx)

	// UpdatePeerHeights updates the heights of all peers who have
	// announced the latest connected main chain block, or a recognized orphan.
	UpdatePeerHeights(latestBlkHash *chainhash.Hash, latestHeight int64, updateSource *serverPeer)

	// RelayInventory relays the passed inventory vector to all connected peers
	// that are not already known to have it.
	RelayInventory(invVect *wire.InvVect, data interface{}, immediate bool)

	// TransactionConfirmed marks the provided single confirmation transaction
	// as no longer needing rebroadcasting.
	TransactionConfirmed(tx *dcrutil.Tx)
}

// blockManangerConfig is a configuration struct for a blockManager.
type blockManagerConfig struct {
	PeerNotifier PeerNotifier
	TimeSource   blockchain.MedianTimeSource

	// The following fields are for accessing the chain and its configuration.
	Chain        *blockchain.BlockChain
	ChainParams  *chaincfg.Params
	SubsidyCache *standalone.SubsidyCache

	// The following fields provide access to the fee estimator, mempool and
	// the background block template generator.
	FeeEstimator       *fees.Estimator
	TxMemPool          *mempool.TxPool
	BgBlkTmplGenerator *BgBlkTmplGenerator

	// The following fields are blockManager callbacks.
	NotifyWinningTickets      func(*WinningTicketsNtfnData)
	PruneRebroadcastInventory func()
	RpcServer                 func() *rpcServer
}

// blockManager provides a concurrency safe block manager for handling all
// incoming blocks.
type blockManager struct {
	cfg             *blockManagerConfig
	started         int32
	shutdown        int32
	rejectedTxns    map[chainhash.Hash]struct{}
	requestedTxns   map[chainhash.Hash]struct{}
	requestedBlocks map[chainhash.Hash]struct{}
	progressLogger  *blockProgressLogger
	syncPeer        *serverPeer
	msgChan         chan interface{}
	wg              sync.WaitGroup
	quit            chan struct{}

	// The following fields are used for headers-first mode.
	headersFirstMode bool
	headerList       *list.List
	startHeader      *list.Element
	nextCheckpoint   *chaincfg.Checkpoint

	// lotteryDataBroadcastMutex is a mutex protecting the map
	// that checks if block lottery data has been broadcasted
	// yet for any given block, so notifications are never
	// duplicated.
	lotteryDataBroadcast      map[chainhash.Hash]struct{}
	lotteryDataBroadcastMutex sync.RWMutex

	AggressiveMining bool

	// The following fields are used to filter duplicate block announcements.
	announcedBlockMtx sync.Mutex
	announcedBlock    *chainhash.Hash

	// The following fields are used to track the height being synced to from
	// peers.
	syncHeightMtx sync.Mutex
	syncHeight    int64
}

// resetHeaderState sets the headers-first mode state to values appropriate for
// syncing from a new peer.
func (b *blockManager) resetHeaderState(newestHash *chainhash.Hash, newestHeight int64) {
	b.headersFirstMode = false
	b.headerList.Init()
	b.startHeader = nil

	// When there is a next checkpoint, add an entry for the latest known
	// block into the header pool.  This allows the next downloaded header
	// to prove it links to the chain properly.
	if b.nextCheckpoint != nil {
		node := headerNode{height: newestHeight, hash: newestHash}
		b.headerList.PushBack(&node)
	}
}

// SyncHeight returns latest known block being synced to.
func (b *blockManager) SyncHeight() int64 {
	b.syncHeightMtx.Lock()
	defer b.syncHeightMtx.Unlock()
	return b.syncHeight
}

// findNextHeaderCheckpoint returns the next checkpoint after the passed height.
// It returns nil when there is not one either because the height is already
// later than the final checkpoint or some other reason such as disabled
// checkpoints.
func (b *blockManager) findNextHeaderCheckpoint(height int64) *chaincfg.Checkpoint {
	// There is no next checkpoint if checkpoints are disabled or there are
	// none for this current network.
	if cfg.DisableCheckpoints {
		return nil
	}
	checkpoints := b.cfg.Chain.Checkpoints()
	if len(checkpoints) == 0 {
		return nil
	}

	// There is no next checkpoint if the height is already after the final
	// checkpoint.
	finalCheckpoint := &checkpoints[len(checkpoints)-1]
	if height >= finalCheckpoint.Height {
		return nil
	}

	// Find the next checkpoint.
	nextCheckpoint := finalCheckpoint
	for i := len(checkpoints) - 2; i >= 0; i-- {
		if height >= checkpoints[i].Height {
			break
		}
		nextCheckpoint = &checkpoints[i]
	}
	return nextCheckpoint
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

// startSync will choose the best peer among the available candidate peers to
// download/sync the blockchain from.  When syncing is already running, it
// simply returns.  It also examines the candidates for any which are no longer
// candidates and removes them as needed.
func (b *blockManager) startSync(peers *list.List) {
	// Return now if we're already syncing.
	if b.syncPeer != nil {
		return
	}

	best := b.cfg.Chain.BestSnapshot()
	var bestPeer *serverPeer
	var enext *list.Element
	for e := peers.Front(); e != nil; e = enext {
		enext = e.Next()
		sp := e.Value.(*serverPeer)

		// Remove sync candidate peers that are no longer candidates due
		// to passing their latest known block.  NOTE: The < is
		// intentional as opposed to <=.  While technically the peer
		// doesn't have a later block when it's equal, it will likely
		// have one soon so it is a reasonable choice.  It also allows
		// the case where both are at 0 such as during regression test.
		if sp.LastBlock() < best.Height {
			peers.Remove(e)
			continue
		}

		// the best sync candidate is the most updated peer
		if bestPeer == nil {
			bestPeer = sp
		}
		if bestPeer.LastBlock() < sp.LastBlock() {
			bestPeer = sp
		}
	}

	// Start syncing from the best peer if one was selected.
	if bestPeer != nil {
		// Clear the requestedBlocks if the sync peer changes, otherwise
		// we may ignore blocks we need that the last sync peer failed
		// to send.
		b.requestedBlocks = make(map[chainhash.Hash]struct{})

		blkLocator, err := b.cfg.Chain.LatestBlockLocator()
		if err != nil {
			bmgrLog.Errorf("Failed to get block locator for the "+
				"latest block: %v", err)
			return
		}
		locator := chainBlockLocatorToHashes(blkLocator)

		bmgrLog.Infof("Syncing to block height %d from peer %v",
			bestPeer.LastBlock(), bestPeer.Addr())

		// When the current height is less than a known checkpoint we
		// can use block headers to learn about which blocks comprise
		// the chain up to the checkpoint and perform less validation
		// for them.  This is possible since each header contains the
		// hash of the previous header and a merkle root.  Therefore if
		// we validate all of the received headers link together
		// properly and the checkpoint hashes match, we can be sure the
		// hashes for the blocks in between are accurate.  Further, once
		// the full blocks are downloaded, the merkle root is computed
		// and compared against the value in the header which proves the
		// full block hasn't been tampered with.
		//
		// Once we have passed the final checkpoint, or checkpoints are
		// disabled, use standard inv messages learn about the blocks
		// and fully validate them.  Finally, regression test mode does
		// not support the headers-first approach so do normal block
		// downloads when in regression test mode.
		if b.nextCheckpoint != nil &&
			best.Height < b.nextCheckpoint.Height &&
			!cfg.DisableCheckpoints {

			err := bestPeer.PushGetHeadersMsg(locator, b.nextCheckpoint.Hash)
			if err != nil {
				bmgrLog.Errorf("Failed to push getheadermsg for the "+
					"latest blocks: %v", err)
				return
			}
			b.headersFirstMode = true
			bmgrLog.Infof("Downloading headers for blocks %d to "+
				"%d from peer %s", best.Height+1,
				b.nextCheckpoint.Height, bestPeer.Addr())
		} else {
			err := bestPeer.PushGetBlocksMsg(locator, &zeroHash)
			if err != nil {
				bmgrLog.Errorf("Failed to push getblocksmsg for the "+
					"latest blocks: %v", err)
				return
			}
		}
		b.syncPeer = bestPeer
		b.syncHeightMtx.Lock()
		b.syncHeight = bestPeer.LastBlock()
		b.syncHeightMtx.Unlock()
	} else {
		bmgrLog.Warnf("No sync peer candidates available")
	}
}

// isSyncCandidate returns whether or not the peer is a candidate to consider
// syncing from.
func (b *blockManager) isSyncCandidate(sp *serverPeer) bool {
	// The peer is not a candidate for sync if it's not a full node.
	return sp.Services()&wire.SFNodeNetwork == wire.SFNodeNetwork
}

// syncMiningStateAfterSync polls the blockManager for the current sync
// state; if the manager is synced, it executes a call to the peer to
// sync the mining state to the network.
func (b *blockManager) syncMiningStateAfterSync(sp *serverPeer) {
	go func() {
		for {
			time.Sleep(3 * time.Second)
			if !sp.Connected() {
				return
			}
			if b.IsCurrent() {
				msg := wire.NewMsgGetMiningState()
				sp.QueueMessage(msg, nil)
				return
			}
		}
	}()
}

// handleNewPeerMsg deals with new peers that have signalled they may
// be considered as a sync peer (they have already successfully negotiated).  It
// also starts syncing if needed.  It is invoked from the syncHandler goroutine.
func (b *blockManager) handleNewPeerMsg(peers *list.List, sp *serverPeer) {
	// Ignore if in the process of shutting down.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		return
	}

	bmgrLog.Infof("New valid peer %s (%s)", sp, sp.UserAgent())

	// Ignore the peer if it's not a sync candidate.
	if !b.isSyncCandidate(sp) {
		return
	}

	// Add the peer as a candidate to sync from.
	peers.PushBack(sp)

	// Start syncing by choosing the best candidate if needed.
	b.startSync(peers)

	// Grab the mining state from this peer after we're synced.
	if !cfg.NoMiningStateSync {
		b.syncMiningStateAfterSync(sp)
	}
}

// handleDonePeerMsg deals with peers that have signalled they are done.  It
// removes the peer as a candidate for syncing and in the case where it was
// the current sync peer, attempts to select a new best peer to sync from.  It
// is invoked from the syncHandler goroutine.
func (b *blockManager) handleDonePeerMsg(peers *list.List, sp *serverPeer) {
	// Remove the peer from the list of candidate peers.
	for e := peers.Front(); e != nil; e = e.Next() {
		if e.Value == sp {
			peers.Remove(e)
			break
		}
	}

	bmgrLog.Infof("Lost peer %s", sp)

	// Remove requested transactions from the global map so that they will
	// be fetched from elsewhere next time we get an inv.
	for k := range sp.requestedTxns {
		delete(b.requestedTxns, k)
	}

	// Remove requested blocks from the global map so that they will be
	// fetched from elsewhere next time we get an inv.
	// TODO(oga) we could possibly here check which peers have these blocks
	// and request them now to speed things up a little.
	for k := range sp.requestedBlocks {
		delete(b.requestedBlocks, k)
	}

	// Attempt to find a new peer to sync from if the quitting peer is the
	// sync peer.  Also, reset the headers-first state if in headers-first
	// mode so
	if b.syncPeer != nil && b.syncPeer == sp {
		b.syncPeer = nil
		if b.headersFirstMode {
			best := b.cfg.Chain.BestSnapshot()
			b.resetHeaderState(&best.Hash, best.Height)
		}
		b.startSync(peers)
	}
}

// errToWireRejectCode determines the wire rejection code and description for a
// given error. This function can convert some select blockchain and mempool
// error types to the historical rejection codes used on the p2p wire protocol.
func errToWireRejectCode(err error) (wire.RejectCode, string) {
	// Unwrap mempool errors.
	if rerr, ok := err.(mempool.RuleError); ok {
		err = rerr.Err
	}

	// The default reason to reject a transaction/block is due to it being
	// invalid somehow.
	code := wire.RejectInvalid
	var reason string

	switch err := err.(type) {
	case blockchain.RuleError:
		// Convert the chain error to a reject code.
		switch err.ErrorCode {
		// Rejected due to duplicate.
		case blockchain.ErrDuplicateBlock:
			code = wire.RejectDuplicate

		// Rejected due to obsolete version.
		case blockchain.ErrBlockVersionTooOld:
			code = wire.RejectObsolete

		// Rejected due to checkpoint.
		case blockchain.ErrCheckpointTimeTooOld,
			blockchain.ErrDifficultyTooLow,
			blockchain.ErrBadCheckpoint,
			blockchain.ErrForkTooOld:

			code = wire.RejectCheckpoint
		}

		reason = err.Error()
	case mempool.TxRuleError:
		switch err.ErrorCode {
		// Error codes which map to a duplicate transaction already
		// mined or in the mempool.
		case mempool.ErrMempoolDoubleSpend,
			mempool.ErrAlreadyVoted,
			mempool.ErrDuplicate,
			mempool.ErrTooManyVotes,
			mempool.ErrDuplicateRevocation,
			mempool.ErrAlreadyExists,
			mempool.ErrOrphan:

			code = wire.RejectDuplicate

		// Error codes which map to a non-standard transaction being
		// relayed.
		case mempool.ErrOrphanPolicyViolation,
			mempool.ErrOldVote,
			mempool.ErrSeqLockUnmet,
			mempool.ErrNonStandard:

			code = wire.RejectNonstandard

		// Error codes which map to an insufficient fee being paid.
		case mempool.ErrInsufficientFee,
			mempool.ErrInsufficientPriority:

			code = wire.RejectInsufficientFee

		// Error codes which map to an attempt to create dust outputs.
		case mempool.ErrDustOutput:
			code = wire.RejectDust
		}

		reason = err.Error()
	default:
		reason = fmt.Sprintf("rejected: %v", err)
	}
	return code, reason
}

// handleTxMsg handles transaction messages from all peers.
func (b *blockManager) handleTxMsg(tmsg *txMsg) {
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
	if _, exists := b.rejectedTxns[*txHash]; exists {
		bmgrLog.Debugf("Ignoring unsolicited previously rejected "+
			"transaction %v from %s", txHash, tmsg.peer)
		return
	}

	// Process the transaction to include validation, insertion in the
	// memory pool, orphan handling, etc.
	allowOrphans := cfg.MaxOrphanTxs > 0
	acceptedTxs, err := b.cfg.TxMemPool.ProcessTransaction(tmsg.tx,
		allowOrphans, true, true)

	// Remove transaction from request maps. Either the mempool/chain
	// already knows about it and as such we shouldn't have any more
	// instances of trying to fetch it, or we failed to insert and thus
	// we'll retry next time we get an inv.
	delete(tmsg.peer.requestedTxns, *txHash)
	delete(b.requestedTxns, *txHash)

	if err != nil {
		// Do not request this transaction again until a new block
		// has been processed.
		b.rejectedTxns[*txHash] = struct{}{}
		b.limitMap(b.rejectedTxns, maxRejectedTxns)

		// When the error is a rule error, it means the transaction was
		// simply rejected as opposed to something actually going wrong,
		// so log it as such.  Otherwise, something really did go wrong,
		// so log it as an actual error.
		if _, ok := err.(mempool.RuleError); ok {
			bmgrLog.Debugf("Rejected transaction %v from %s: %v",
				txHash, tmsg.peer, err)
		} else {
			bmgrLog.Errorf("Failed to process transaction %v: %v",
				txHash, err)
		}

		// Convert the error into an appropriate reject message and
		// send it.
		code, reason := errToWireRejectCode(err)
		tmsg.peer.PushRejectMsg(wire.CmdTx, code, reason, txHash,
			false)
		return
	}

	b.cfg.PeerNotifier.AnnounceNewTransactions(acceptedTxs)
}

// current returns true if we believe we are synced with our peers, false if we
// still have blocks to check
func (b *blockManager) current() bool {
	if !b.cfg.Chain.IsCurrent() {
		return false
	}

	// if blockChain thinks we are current and we have no syncPeer it
	// is probably right.
	if b.syncPeer == nil {
		return true
	}

	// No matter what chain thinks, if we are below the block we are syncing
	// to we are not current.
	if b.cfg.Chain.BestSnapshot().Height < b.syncPeer.LastBlock() {
		return false
	}

	return true
}

// handleBlockMsg handles block messages from all peers.
func (b *blockManager) handleBlockMsg(bmsg *blockMsg) {
	// If we didn't ask for this block then the peer is misbehaving.
	blockHash := bmsg.block.Hash()
	if _, exists := bmsg.peer.requestedBlocks[*blockHash]; !exists {
		bmgrLog.Warnf("Got unrequested block %v from %s -- "+
			"disconnecting", blockHash, bmsg.peer.Addr())
		bmsg.peer.Disconnect()
		return
	}

	// When in headers-first mode, if the block matches the hash of the
	// first header in the list of headers that are being fetched, it's
	// eligible for less validation since the headers have already been
	// verified to link together and are valid up to the next checkpoint.
	// Also, remove the list entry for all blocks except the checkpoint
	// since it is needed to verify the next round of headers links
	// properly.
	isCheckpointBlock := false
	behaviorFlags := blockchain.BFNone
	if b.headersFirstMode {
		firstNodeEl := b.headerList.Front()
		if firstNodeEl != nil {
			firstNode := firstNodeEl.Value.(*headerNode)
			if blockHash.IsEqual(firstNode.hash) {
				behaviorFlags |= blockchain.BFFastAdd
				if firstNode.hash.IsEqual(b.nextCheckpoint.Hash) {
					isCheckpointBlock = true
				} else {
					b.headerList.Remove(firstNodeEl)
				}
			}
		}
	}

	// Remove block from request maps. Either chain will know about it and
	// so we shouldn't have any more instances of trying to fetch it, or we
	// will fail the insert and thus we'll retry next time we get an inv.
	delete(bmsg.peer.requestedBlocks, *blockHash)
	delete(b.requestedBlocks, *blockHash)

	// Process the block to include validation, best chain selection, orphan
	// handling, etc.
	forkLen, isOrphan, err := b.cfg.Chain.ProcessBlock(bmsg.block,
		behaviorFlags)
	if err != nil {
		// When the error is a rule error, it means the block was simply
		// rejected as opposed to something actually going wrong, so log
		// it as such.  Otherwise, something really did go wrong, so log
		// it as an actual error.
		if _, ok := err.(blockchain.RuleError); ok {
			bmgrLog.Infof("Rejected block %v from %s: %v", blockHash,
				bmsg.peer, err)
		} else {
			bmgrLog.Errorf("Failed to process block %v: %v",
				blockHash, err)
		}
		if dbErr, ok := err.(database.Error); ok && dbErr.ErrorCode ==
			database.ErrCorruption {
			bmgrLog.Errorf("Critical failure: %v", dbErr.Error())
		}

		// Convert the error into an appropriate reject message and
		// send it.
		code, reason := errToWireRejectCode(err)
		bmsg.peer.PushRejectMsg(wire.CmdBlock, code, reason,
			blockHash, false)
		return
	}

	// Meta-data about the new block this peer is reporting. We use this
	// below to update this peer's latest block height and the heights of
	// other peers based on their last announced block hash. This allows us
	// to dynamically update the block heights of peers, avoiding stale
	// heights when looking for a new sync peer. Upon acceptance of a block
	// or recognition of an orphan, we also use this information to update
	// the block heights over other peers who's invs may have been ignored
	// if we are actively syncing while the chain is not yet current or
	// who may have lost the lock announcement race.
	var heightUpdate int64
	var blkHashUpdate *chainhash.Hash

	// Request the parents for the orphan block from the peer that sent it.
	if isOrphan {
		// We've just received an orphan block from a peer. In order
		// to update the height of the peer, we try to extract the
		// block height from the scriptSig of the coinbase transaction.
		// Extraction is only attempted if the block's version is
		// high enough (ver 2+).
		header := &bmsg.block.MsgBlock().Header
		cbHeight := header.Height
		heightUpdate = int64(cbHeight)
		blkHashUpdate = blockHash

		orphanRoot := b.cfg.Chain.GetOrphanRoot(blockHash)
		blkLocator, err := b.cfg.Chain.LatestBlockLocator()
		if err != nil {
			bmgrLog.Warnf("Failed to get block locator for the "+
				"latest block: %v", err)
		} else {
			locator := chainBlockLocatorToHashes(blkLocator)
			err = bmsg.peer.PushGetBlocksMsg(locator, orphanRoot)
			if err != nil {
				bmgrLog.Warnf("Failed to push getblocksmsg for the "+
					"latest block: %v", err)
			}
		}
	} else {
		// When the block is not an orphan, log information about it and
		// update the chain state.
		b.progressLogger.logBlockHeight(bmsg.block)

		onMainChain := !isOrphan && forkLen == 0
		if onMainChain {
			// Notify stake difficulty subscribers and prune invalidated
			// transactions.
			best := b.cfg.Chain.BestSnapshot()
			r := b.cfg.RpcServer()
			if r != nil {
				// Update registered websocket clients on the
				// current stake difficulty.
				r.ntfnMgr.NotifyStakeDifficulty(
					&StakeDifficultyNtfnData{
						best.Hash,
						best.Height,
						best.NextStakeDiff,
					})
			}
			b.cfg.TxMemPool.PruneStakeTx(best.NextStakeDiff, best.Height)
			b.cfg.TxMemPool.PruneExpiredTx()

			// Update this peer's latest block height, for future
			// potential sync node candidacy.
			heightUpdate = best.Height
			blkHashUpdate = &best.Hash

			// Clear the rejected transactions.
			b.rejectedTxns = make(map[chainhash.Hash]struct{})
		}
	}

	// Update the block height for this peer. But only send a message to
	// the server for updating peer heights if this is an orphan or our
	// chain is "current". This avoids sending a spammy amount of messages
	// if we're syncing the chain from scratch.
	if blkHashUpdate != nil && heightUpdate != 0 {
		bmsg.peer.UpdateLastBlockHeight(heightUpdate)
		if isOrphan || b.current() {
			go b.cfg.PeerNotifier.UpdatePeerHeights(blkHashUpdate, heightUpdate,
				bmsg.peer)
		}
	}

	// Nothing more to do if we aren't in headers-first mode.
	if !b.headersFirstMode {
		return
	}

	// This is headers-first mode, so if the block is not a checkpoint
	// request more blocks using the header list when the request queue is
	// getting short.
	if !isCheckpointBlock {
		if b.startHeader != nil &&
			len(bmsg.peer.requestedBlocks) < minInFlightBlocks {
			b.fetchHeaderBlocks()
		}
		return
	}

	// This is headers-first mode and the block is a checkpoint.  When
	// there is a next checkpoint, get the next round of headers by asking
	// for headers starting from the block after this one up to the next
	// checkpoint.
	prevHeight := b.nextCheckpoint.Height
	prevHash := b.nextCheckpoint.Hash
	b.nextCheckpoint = b.findNextHeaderCheckpoint(prevHeight)
	if b.nextCheckpoint != nil {
		locator := []chainhash.Hash{*prevHash}
		err := bmsg.peer.PushGetHeadersMsg(locator, b.nextCheckpoint.Hash)
		if err != nil {
			bmgrLog.Warnf("Failed to send getheaders message to "+
				"peer %s: %v", bmsg.peer.Addr(), err)
			return
		}
		bmgrLog.Infof("Downloading headers for blocks %d to %d from "+
			"peer %s", prevHeight+1, b.nextCheckpoint.Height,
			b.syncPeer.Addr())
		return
	}

	// This is headers-first mode, the block is a checkpoint, and there are
	// no more checkpoints, so switch to normal mode by requesting blocks
	// from the block after this one up to the end of the chain (zero hash).
	b.headersFirstMode = false
	b.headerList.Init()
	bmgrLog.Infof("Reached the final checkpoint -- switching to normal mode")
	locator := []chainhash.Hash{*blockHash}
	err = bmsg.peer.PushGetBlocksMsg(locator, &zeroHash)
	if err != nil {
		bmgrLog.Warnf("Failed to send getblocks message to peer %s: %v",
			bmsg.peer.Addr(), err)
		return
	}
}

// fetchHeaderBlocks creates and sends a request to the syncPeer for the next
// list of blocks to be downloaded based on the current list of headers.
func (b *blockManager) fetchHeaderBlocks() {
	// Nothing to do if there is no start header.
	if b.startHeader == nil {
		bmgrLog.Warnf("fetchHeaderBlocks called with no start header")
		return
	}

	// Build up a getdata request for the list of blocks the headers
	// describe.  The size hint will be limited to wire.MaxInvPerMsg by
	// the function, so no need to double check it here.
	gdmsg := wire.NewMsgGetDataSizeHint(uint(b.headerList.Len()))
	numRequested := 0
	for e := b.startHeader; e != nil; e = e.Next() {
		node, ok := e.Value.(*headerNode)
		if !ok {
			bmgrLog.Warn("Header list node type is not a headerNode")
			continue
		}

		iv := wire.NewInvVect(wire.InvTypeBlock, node.hash)
		haveInv, err := b.haveInventory(iv)
		if err != nil {
			bmgrLog.Warnf("Unexpected failure when checking for "+
				"existing inventory during header block "+
				"fetch: %v", err)
			continue
		}
		if !haveInv {
			b.requestedBlocks[*node.hash] = struct{}{}
			b.syncPeer.requestedBlocks[*node.hash] = struct{}{}
			err = gdmsg.AddInvVect(iv)
			if err != nil {
				bmgrLog.Warnf("Failed to add invvect while fetching "+
					"block headers: %v", err)
			}
			numRequested++
		}
		b.startHeader = e.Next()
		if numRequested >= wire.MaxInvPerMsg {
			break
		}
	}
	if len(gdmsg.InvList) > 0 {
		b.syncPeer.QueueMessage(gdmsg, nil)
	}
}

// handleHeadersMsg handles headers messages from all peers.
func (b *blockManager) handleHeadersMsg(hmsg *headersMsg) {
	// The remote peer is misbehaving if we didn't request headers.
	msg := hmsg.headers
	numHeaders := len(msg.Headers)
	if !b.headersFirstMode {
		bmgrLog.Warnf("Got %d unrequested headers from %s -- "+
			"disconnecting", numHeaders, hmsg.peer.Addr())
		hmsg.peer.Disconnect()
		return
	}

	// Nothing to do for an empty headers message.
	if numHeaders == 0 {
		return
	}

	// Process all of the received headers ensuring each one connects to the
	// previous and that checkpoints match.
	receivedCheckpoint := false
	var finalHash *chainhash.Hash
	for _, blockHeader := range msg.Headers {
		blockHash := blockHeader.BlockHash()
		finalHash = &blockHash

		// Ensure there is a previous header to compare against.
		prevNodeEl := b.headerList.Back()
		if prevNodeEl == nil {
			bmgrLog.Warnf("Header list does not contain a previous" +
				" element as expected -- disconnecting peer")
			hmsg.peer.Disconnect()
			return
		}

		// Ensure the header properly connects to the previous one and
		// add it to the list of headers.
		node := headerNode{hash: &blockHash}
		prevNode := prevNodeEl.Value.(*headerNode)
		if prevNode.hash.IsEqual(&blockHeader.PrevBlock) {
			node.height = prevNode.height + 1
			e := b.headerList.PushBack(&node)
			if b.startHeader == nil {
				b.startHeader = e
			}
		} else {
			bmgrLog.Warnf("Received block header that does not "+
				"properly connect to the chain from peer %s "+
				"-- disconnecting", hmsg.peer.Addr())
			hmsg.peer.Disconnect()
			return
		}

		// Verify the header at the next checkpoint height matches.
		if node.height == b.nextCheckpoint.Height {
			if node.hash.IsEqual(b.nextCheckpoint.Hash) {
				receivedCheckpoint = true
				bmgrLog.Infof("Verified downloaded block "+
					"header against checkpoint at height "+
					"%d/hash %s", node.height, node.hash)
			} else {
				bmgrLog.Warnf("Block header at height %d/hash "+
					"%s from peer %s does NOT match "+
					"expected checkpoint hash of %s -- "+
					"disconnecting", node.height,
					node.hash, hmsg.peer.Addr(),
					b.nextCheckpoint.Hash)
				hmsg.peer.Disconnect()
				return
			}
			break
		}
	}

	// When this header is a checkpoint, switch to fetching the blocks for
	// all of the headers since the last checkpoint.
	if receivedCheckpoint {
		// Since the first entry of the list is always the final block
		// that is already in the database and is only used to ensure
		// the next header links properly, it must be removed before
		// fetching the blocks.
		b.headerList.Remove(b.headerList.Front())
		bmgrLog.Infof("Received %v block headers: Fetching blocks",
			b.headerList.Len())
		b.progressLogger.SetLastLogTime(time.Now())
		b.fetchHeaderBlocks()
		return
	}

	// This header is not a checkpoint, so request the next batch of
	// headers starting from the latest known header and ending with the
	// next checkpoint.
	locator := []chainhash.Hash{*finalHash}
	err := hmsg.peer.PushGetHeadersMsg(locator, b.nextCheckpoint.Hash)
	if err != nil {
		bmgrLog.Warnf("Failed to send getheaders message to "+
			"peer %s: %v", hmsg.peer.Addr(), err)
		return
	}
}

// haveInventory returns whether or not the inventory represented by the passed
// inventory vector is known.  This includes checking all of the various places
// inventory can be when it is in different states such as blocks that are part
// of the main chain, on a side chain, in the orphan pool, and transactions that
// are in the memory pool (either the main pool or orphan pool).
func (b *blockManager) haveInventory(invVect *wire.InvVect) (bool, error) {
	switch invVect.Type {
	case wire.InvTypeBlock:
		// Ask chain if the block is known to it in any form (main
		// chain, side chain, or orphan).
		return b.cfg.Chain.HaveBlock(&invVect.Hash)

	case wire.InvTypeTx:
		// Ask the transaction memory pool if the transaction is known
		// to it in any form (main pool or orphan).
		if b.cfg.TxMemPool.HaveTransaction(&invVect.Hash) {
			return true, nil
		}

		// Check if the transaction exists from the point of view of the
		// end of the main chain.
		entry, err := b.cfg.Chain.FetchUtxoEntry(&invVect.Hash)
		if err != nil {
			return false, err
		}
		return entry != nil && !entry.IsFullySpent(), nil
	}

	// The requested inventory is an unsupported type, so just claim
	// it is known to avoid requesting it.
	return true, nil
}

// handleInvMsg handles inv messages from all peers.
// We examine the inventory advertised by the remote peer and act accordingly.
func (b *blockManager) handleInvMsg(imsg *invMsg) {
	// Attempt to find the final block in the inventory list.  There may
	// not be one.
	lastBlock := -1
	invVects := imsg.inv.InvList
	for i := len(invVects) - 1; i >= 0; i-- {
		if invVects[i].Type == wire.InvTypeBlock {
			lastBlock = i
			break
		}
	}

	fromSyncPeer := imsg.peer == b.syncPeer
	isCurrent := b.current()

	// If this inv contains a block announcement, and this isn't coming from
	// our current sync peer or we're current, then update the last
	// announced block for this peer. We'll use this information later to
	// update the heights of peers based on blocks we've accepted that they
	// previously announced.
	if lastBlock != -1 && (!fromSyncPeer || isCurrent) {
		imsg.peer.UpdateLastAnnouncedBlock(&invVects[lastBlock].Hash)
	}

	// Ignore invs from peers that aren't the sync if we are not current.
	// Helps prevent fetching a mass of orphans.
	if !fromSyncPeer && !isCurrent {
		return
	}

	// If our chain is current and a peer announces a block we already
	// know of, then update their current block height.
	if lastBlock != -1 && isCurrent {
		blkHeight, err := b.cfg.Chain.BlockHeightByHash(&invVects[lastBlock].Hash)
		if err == nil {
			imsg.peer.UpdateLastBlockHeight(blkHeight)
		}
	}

	// Request the advertised inventory if we don't already have it.  Also,
	// request parent blocks of orphans if we receive one we already have.
	// Finally, attempt to detect potential stalls due to long side chains
	// we already have and request more blocks to prevent them.
	var requestQueue []*wire.InvVect
	for i, iv := range invVects {
		// Ignore unsupported inventory types.
		if iv.Type != wire.InvTypeBlock && iv.Type != wire.InvTypeTx {
			continue
		}

		// Add the inventory to the cache of known inventory
		// for the peer.
		imsg.peer.AddKnownInventory(iv)

		// Ignore inventory when we're in headers-first mode.
		if b.headersFirstMode {
			continue
		}

		// Request the inventory if we don't already have it.
		haveInv, err := b.haveInventory(iv)
		if err != nil {
			bmgrLog.Warnf("Unexpected failure when checking for "+
				"existing inventory during inv message "+
				"processing: %v", err)
			continue
		}
		if !haveInv {
			if iv.Type == wire.InvTypeTx {
				// Skip the transaction if it has already been
				// rejected.
				if _, exists := b.rejectedTxns[iv.Hash]; exists {
					continue
				}
			}

			// Add it to the request queue.
			requestQueue = append(requestQueue, iv)
			continue
		}

		if iv.Type == wire.InvTypeBlock {
			// The block is an orphan block that we already have.
			// When the existing orphan was processed, it requested
			// the missing parent blocks.  When this scenario
			// happens, it means there were more blocks missing
			// than are allowed into a single inventory message.  As
			// a result, once this peer requested the final
			// advertised block, the remote peer noticed and is now
			// resending the orphan block as an available block
			// to signal there are more missing blocks that need to
			// be requested.
			if b.cfg.Chain.IsKnownOrphan(&iv.Hash) {
				// Request blocks starting at the latest known
				// up to the root of the orphan that just came
				// in.
				orphanRoot := b.cfg.Chain.GetOrphanRoot(&iv.Hash)
				blkLocator, err := b.cfg.Chain.LatestBlockLocator()
				if err != nil {
					bmgrLog.Errorf("PEER: Failed to get block "+
						"locator for the latest block: "+
						"%v", err)
					continue
				}
				locator := chainBlockLocatorToHashes(blkLocator)
				err = imsg.peer.PushGetBlocksMsg(locator, orphanRoot)
				if err != nil {
					bmgrLog.Errorf("PEER: Failed to push getblocksmsg "+
						"for orphan chain: %v", err)
				}
				continue
			}

			// We already have the final block advertised by this
			// inventory message, so force a request for more.  This
			// should only happen if we're on a really long side
			// chain.
			if i == lastBlock {
				// Request blocks after this one up to the
				// final one the remote peer knows about (zero
				// stop hash).
				blkLocator := b.cfg.Chain.BlockLocatorFromHash(&iv.Hash)
				locator := chainBlockLocatorToHashes(blkLocator)
				err = imsg.peer.PushGetBlocksMsg(locator, &zeroHash)
				if err != nil {
					bmgrLog.Errorf("PEER: Failed to push getblocksmsg: "+
						"%v", err)
				}
			}
		}
	}

	// Request as much as possible at once.
	numRequested := 0
	gdmsg := wire.NewMsgGetData()
	for _, iv := range requestQueue {
		switch iv.Type {
		case wire.InvTypeBlock:
			// Request the block if there is not already a pending
			// request.
			if _, exists := b.requestedBlocks[iv.Hash]; !exists {
				b.requestedBlocks[iv.Hash] = struct{}{}
				b.limitMap(b.requestedBlocks, maxRequestedBlocks)
				imsg.peer.requestedBlocks[iv.Hash] = struct{}{}
				gdmsg.AddInvVect(iv)
				numRequested++
			}

		case wire.InvTypeTx:
			// Request the transaction if there is not already a
			// pending request.
			if _, exists := b.requestedTxns[iv.Hash]; !exists {
				b.requestedTxns[iv.Hash] = struct{}{}
				b.limitMap(b.requestedTxns, maxRequestedTxns)
				imsg.peer.requestedTxns[iv.Hash] = struct{}{}
				gdmsg.AddInvVect(iv)
				numRequested++
			}
		}

		if numRequested == wire.MaxInvPerMsg {
			// Send full getdata message and reset.
			//
			// NOTE: There should never be more than wire.MaxInvPerMsg
			// in the inv request, so we could return after the
			// QueueMessage, but this is more safe.
			imsg.peer.QueueMessage(gdmsg, nil)
			gdmsg = wire.NewMsgGetData()
			numRequested = 0
		}
	}

	if len(gdmsg.InvList) > 0 {
		imsg.peer.QueueMessage(gdmsg, nil)
	}
}

// limitMap is a helper function for maps that require a maximum limit by
// evicting a random transaction if adding a new value would cause it to
// overflow the maximum allowed.
func (b *blockManager) limitMap(m map[chainhash.Hash]struct{}, limit int) {
	if len(m)+1 > limit {
		// Remove a random entry from the map.  For most compilers, Go's
		// range statement iterates starting at a random item although
		// that is not 100% guaranteed by the spec.  The iteration order
		// is not important here because an adversary would have to be
		// able to pull off preimage attacks on the hashing function in
		// order to target eviction of specific entries anyways.
		for txHash := range m {
			delete(m, txHash)
			return
		}
	}
}

// blockHandler is the main handler for the block manager.  It must be run
// as a goroutine.  It processes block and inv messages in a separate goroutine
// from the peer handlers so the block (MsgBlock) messages are handled by a
// single thread without needing to lock memory data structures.  This is
// important because the block manager controls which blocks are needed and how
// the fetching should proceed.
func (b *blockManager) blockHandler() {
	candidatePeers := list.New()
out:
	for {
		select {
		case m := <-b.msgChan:
			switch msg := m.(type) {
			case *newPeerMsg:
				b.handleNewPeerMsg(candidatePeers, msg.peer)

			case *txMsg:
				b.handleTxMsg(msg)
				msg.peer.txProcessed <- struct{}{}

			case *blockMsg:
				b.handleBlockMsg(msg)
				msg.peer.blockProcessed <- struct{}{}

			case *invMsg:
				b.handleInvMsg(msg)

			case *headersMsg:
				b.handleHeadersMsg(msg)

			case *donePeerMsg:
				b.handleDonePeerMsg(candidatePeers, msg.peer)

			case getSyncPeerMsg:
				msg.reply <- b.syncPeer

			case requestFromPeerMsg:
				err := b.requestFromPeer(msg.peer, msg.blocks, msg.txs)
				msg.reply <- requestFromPeerResponse{
					err: err,
				}

			case calcNextReqDiffNodeMsg:
				difficulty, err :=
					b.cfg.Chain.CalcNextRequiredDiffFromNode(msg.hash,
						msg.timestamp)
				msg.reply <- calcNextReqDifficultyResponse{
					difficulty: difficulty,
					err:        err,
				}

			case calcNextReqStakeDifficultyMsg:
				stakeDiff, err := b.cfg.Chain.CalcNextRequiredStakeDifficulty()
				msg.reply <- calcNextReqStakeDifficultyResponse{
					stakeDifficulty: stakeDiff,
					err:             err,
				}

			case forceReorganizationMsg:
				err := b.cfg.Chain.ForceHeadReorganization(
					msg.formerBest, msg.newBest)

				if err == nil {
					// Notify stake difficulty subscribers and prune
					// invalidated transactions.
					best := b.cfg.Chain.BestSnapshot()
					r := b.cfg.RpcServer()
					if r != nil {
						r.ntfnMgr.NotifyStakeDifficulty(
							&StakeDifficultyNtfnData{
								best.Hash,
								best.Height,
								best.NextStakeDiff,
							})
					}
					b.cfg.TxMemPool.PruneStakeTx(best.NextStakeDiff,
						best.Height)
					b.cfg.TxMemPool.PruneExpiredTx()
				}

				msg.reply <- forceReorganizationResponse{
					err: err,
				}

			case tipGenerationMsg:
				g, err := b.cfg.Chain.TipGeneration()
				msg.reply <- tipGenerationResponse{
					hashes: g,
					err:    err,
				}

			case processBlockMsg:
				forkLen, isOrphan, err := b.cfg.Chain.ProcessBlock(
					msg.block, msg.flags)
				if err != nil {
					msg.reply <- processBlockResponse{
						forkLen:  forkLen,
						isOrphan: isOrphan,
						err:      err,
					}
					continue
				}

				r := b.cfg.RpcServer()
				onMainChain := !isOrphan && forkLen == 0
				if onMainChain {
					// Notify stake difficulty subscribers and prune
					// invalidated transactions.
					best := b.cfg.Chain.BestSnapshot()
					if r != nil {
						r.ntfnMgr.NotifyStakeDifficulty(
							&StakeDifficultyNtfnData{
								best.Hash,
								best.Height,
								best.NextStakeDiff,
							})
					}
					b.cfg.TxMemPool.PruneStakeTx(best.NextStakeDiff,
						best.Height)
					b.cfg.TxMemPool.PruneExpiredTx()
				}

				msg.reply <- processBlockResponse{
					isOrphan: isOrphan,
					err:      nil,
				}

			case processTransactionMsg:
				acceptedTxs, err := b.cfg.TxMemPool.ProcessTransaction(msg.tx,
					msg.allowOrphans, msg.rateLimit, msg.allowHighFees)
				msg.reply <- processTransactionResponse{
					acceptedTxs: acceptedTxs,
					err:         err,
				}

			case isCurrentMsg:
				msg.reply <- b.current()

			default:
				bmgrLog.Warnf("Invalid message type in block handler: %T", msg)
			}

		case <-b.quit:
			break out
		}
	}

	b.wg.Done()
	bmgrLog.Trace("Block handler done")
}

// notifiedWinningTickets returns whether or not the winning tickets
// notification for the specified block hash has already been sent.
func (b *blockManager) notifiedWinningTickets(hash *chainhash.Hash) bool {
	b.lotteryDataBroadcastMutex.Lock()
	_, beenNotified := b.lotteryDataBroadcast[*hash]
	b.lotteryDataBroadcastMutex.Unlock()
	return beenNotified
}

// headerApprovesParent returns whether or not the vote bits in the passed
// header indicate the regular transaction tree of the parent block should be
// considered valid.
func headerApprovesParent(header *wire.BlockHeader) bool {
	return dcrutil.IsFlagSet16(header.VoteBits, dcrutil.BlockValid)
}

// isDoubleSpendOrDuplicateError returns whether or not the passed error, which
// is expected to have come from mempool, indicates a transaction was rejected
// either due to containing a double spend or already existing in the pool.
func isDoubleSpendOrDuplicateError(err error) bool {
	merr, ok := err.(mempool.RuleError)
	if !ok {
		return false
	}

	rerr, ok := merr.Err.(mempool.TxRuleError)
	if ok {
		switch rerr.ErrorCode {
		case mempool.ErrDuplicate:
			return true
		case mempool.ErrAlreadyExists:
			return true
		default:
			return false
		}
	}

	cerr, ok := merr.Err.(blockchain.RuleError)
	if ok && cerr.ErrorCode == blockchain.ErrMissingTxOut {
		return true
	}

	return false
}

// handleBlockchainNotification handles notifications from blockchain.  It does
// things such as request orphan block parents and relay accepted blocks to
// connected peers.
func (b *blockManager) handleBlockchainNotification(notification *blockchain.Notification) {
	switch notification.Type {
	// A block that intends to extend the main chain has passed all sanity and
	// contextual checks and the chain is believed to be current.  Relay it to
	// other peers.
	case blockchain.NTNewTipBlockChecked:
		// WARNING: The chain lock is not released before sending this
		// notification, so care must be taken to avoid calling chain functions
		// which could result in a deadlock.
		block, ok := notification.Data.(*dcrutil.Block)
		if !ok {
			bmgrLog.Warnf("New tip block checked notification is not a block.")
			break
		}

		// Generate the inventory vector and relay it immediately.
		iv := wire.NewInvVect(wire.InvTypeBlock, block.Hash())
		b.cfg.PeerNotifier.RelayInventory(iv, block.MsgBlock().Header, true)
		b.announcedBlockMtx.Lock()
		b.announcedBlock = block.Hash()
		b.announcedBlockMtx.Unlock()

	// A block has been accepted into the block chain.  Relay it to other peers
	// (will be ignored if already relayed via NTNewTipBlockChecked) and
	// possibly notify RPC clients with the winning tickets.
	case blockchain.NTBlockAccepted:
		// Don't relay or notify RPC clients with winning tickets if we
		// are not current. Other peers that are current should already
		// know about it and clients, such as wallets, shouldn't be voting on
		// old blocks.
		if !b.current() {
			return
		}

		band, ok := notification.Data.(*blockchain.BlockAcceptedNtfnsData)
		if !ok {
			bmgrLog.Warnf("Chain accepted notification is not " +
				"BlockAcceptedNtfnsData.")
			break
		}
		block := band.Block

		// Send a winning tickets notification as needed.  The notification will
		// only be sent when the following conditions hold:
		//
		// - The RPC server is running
		// - The block that would build on this one is at or after the height
		//   voting begins
		// - The block that would build on this one would not cause a reorg
		//   larger than the max reorg notify depth
		// - This block is after the final checkpoint height
		// - A notification for this block has not already been sent
		//
		// To help visualize the math here, consider the following two competing
		// branches:
		//
		// 100 -> 101  -> 102  -> 103 -> 104 -> 105 -> 106
		//    \-> 101' -> 102'
		//
		// Further, assume that this is a notification for block 103', or in
		// other words, it is extending the shorter side chain.  The reorg depth
		// would be 106 - (103 - 3) = 6.  This should intuitively make sense,
		// because if the side chain were to be extended enough to become the
		// best chain, it would result in a reorg that would remove 6 blocks,
		// namely blocks 101, 102, 103, 104, 105, and 106.
		blockHash := block.Hash()
		bestHeight := band.BestHeight
		blockHeight := int64(block.MsgBlock().Header.Height)
		reorgDepth := bestHeight - (blockHeight - band.ForkLen)
		if b.cfg.RpcServer() != nil &&
			blockHeight >= b.cfg.ChainParams.StakeValidationHeight-1 &&
			reorgDepth < maxReorgDepthNotify &&
			blockHeight > b.cfg.ChainParams.LatestCheckpointHeight() &&
			!b.notifiedWinningTickets(blockHash) {

			// Obtain the winning tickets for this block.  handleNotifyMsg
			// should be safe for concurrent access of things contained
			// within blockchain.
			wt, _, _, err := b.cfg.Chain.LotteryDataForBlock(blockHash)
			if err != nil {
				bmgrLog.Errorf("Couldn't calculate winning tickets for "+
					"accepted block %v: %v", blockHash, err.Error())
			} else {
				// Notify registered websocket clients of newly
				// eligible tickets to vote on.
				b.cfg.NotifyWinningTickets(&WinningTicketsNtfnData{
					BlockHash:   *blockHash,
					BlockHeight: blockHeight,
					Tickets:     wt,
				})

				b.lotteryDataBroadcastMutex.Lock()
				b.lotteryDataBroadcast[*blockHash] = struct{}{}
				b.lotteryDataBroadcastMutex.Unlock()
			}
		}

		// Generate the inventory vector and relay it immediately if not already
		// known to have been sent in NTNewTipBlockChecked.
		b.announcedBlockMtx.Lock()
		sent := b.announcedBlock != nil && *b.announcedBlock == *blockHash
		b.announcedBlock = nil
		b.announcedBlockMtx.Unlock()
		if !sent {
			iv := wire.NewInvVect(wire.InvTypeBlock, blockHash)
			b.cfg.PeerNotifier.RelayInventory(iv, block.MsgBlock().Header, true)
		}

		// Inform the background block template generator about the accepted
		// block.
		if b.cfg.BgBlkTmplGenerator != nil {
			b.cfg.BgBlkTmplGenerator.BlockAccepted(block)
		}

		if !b.cfg.FeeEstimator.IsEnabled() {
			// fee estimation can only start after we have performed an initial
			// sync, otherwise we'll start adding mempool transactions at the
			// wrong height.
			b.cfg.FeeEstimator.Enable(block.Height())
		}

	// A block has been connected to the main block chain.
	case blockchain.NTBlockConnected:
		blockSlice, ok := notification.Data.([]*dcrutil.Block)
		if !ok {
			bmgrLog.Warnf("Block connected notification is not a block slice.")
			break
		}

		if len(blockSlice) != 2 {
			bmgrLog.Warnf("Block connected notification is wrong size slice.")
			break
		}

		block := blockSlice[0]
		parentBlock := blockSlice[1]

		// Account for transactions mined in the newly connected block for fee
		// estimation. This must be done before attempting to remove
		// transactions from the mempool because the mempool will alert the
		// estimator of the txs that are leaving
		b.cfg.FeeEstimator.ProcessBlock(block)

		// TODO: In the case the new tip disapproves the previous block, any
		// transactions the previous block contains in its regular tree which
		// double spend the same inputs as transactions in either tree of the
		// current tip should ideally be tracked in the pool as eligible for
		// inclusion in an alternative tip (side chain block) in case the
		// current tip block does not get enough votes.  However, the
		// transaction pool currently does not provide any way to distinguish
		// this condition and thus only provides tracking based on the current
		// tip.  In order to handle this condition, the pool would have to
		// provide a way to track and independently query which txns are
		// eligible based on the current tip both approving and disapproving the
		// previous block as well as the previous block itself.

		// Remove all of the regular and stake transactions in the connected
		// block from the transaction pool.  Also, remove any transactions which
		// are now double spends as a result of these new transactions.
		// Finally, remove any transaction that is no longer an orphan.
		// Transactions which depend on a confirmed transaction are NOT removed
		// recursively because they are still valid.  Also, the coinbase of the
		// regular tx tree is skipped because the transaction pool doesn't (and
		// can't) have regular tree coinbase transactions in it.
		//
		// Also, in the case the RPC server is enabled, stop rebroadcasting any
		// transactions in the block that were setup to be rebroadcast.
		txMemPool := b.cfg.TxMemPool
		handleConnectedBlockTxns := func(txns []*dcrutil.Tx) {
			for _, tx := range txns {
				txMemPool.RemoveTransaction(tx, false)
				txMemPool.RemoveDoubleSpends(tx)
				txMemPool.RemoveOrphan(tx)
				acceptedTxs := txMemPool.ProcessOrphans(tx)
				b.cfg.PeerNotifier.AnnounceNewTransactions(acceptedTxs)

				// Now that this block is in the blockchain, mark the
				// transaction (except the coinbase) as no longer needing
				// rebroadcasting.
				b.cfg.PeerNotifier.TransactionConfirmed(tx)
			}
		}
		handleConnectedBlockTxns(block.Transactions()[1:])
		handleConnectedBlockTxns(block.STransactions())

		// In the case the regular tree of the previous block was disapproved,
		// add all of the its transactions, with the exception of the coinbase,
		// back to the transaction pool to be mined in a future block.
		//
		// Notice that some of those transactions might have been included in
		// the current block and others might also be spending some of the same
		// outputs that transactions in the previous originally block spent.
		// This is the expected behavior because disapproval of the regular tree
		// of the previous block essentially makes it as if those transactions
		// never happened.
		//
		// Finally, if transactions fail to add to the pool for some reason
		// other than the pool already having it (a duplicate) or now being a
		// double spend, remove all transactions that depend on it as well.
		// The dependencies are not removed for double spends because the only
		// way a transaction which was not a double spend in the previous block
		// to now be one is due to some transaction in the current block
		// (probably the same one) also spending those outputs, and, in that
		// case, anything that happens to be in the pool which depends on the
		// transaction is still valid.
		if !headerApprovesParent(&block.MsgBlock().Header) {
			for _, tx := range parentBlock.Transactions()[1:] {
				_, err := txMemPool.MaybeAcceptTransaction(tx, false, true)
				if err != nil && !isDoubleSpendOrDuplicateError(err) {
					txMemPool.RemoveTransaction(tx, true)
				}
			}
		}

		if r := b.cfg.RpcServer(); r != nil {
			// Filter and update the rebroadcast inventory.
			b.cfg.PruneRebroadcastInventory()

			// Notify registered websocket clients of incoming block.
			r.ntfnMgr.NotifyBlockConnected(block)
		}

		if b.cfg.BgBlkTmplGenerator != nil {
			b.cfg.BgBlkTmplGenerator.BlockConnected(block)
		}

	// Stake tickets are spent or missed from the most recently connected block.
	case blockchain.NTSpentAndMissedTickets:
		tnd, ok := notification.Data.(*blockchain.TicketNotificationsData)
		if !ok {
			bmgrLog.Warnf("Tickets connected notification is not " +
				"TicketNotificationsData")
			break
		}

		if r := b.cfg.RpcServer(); r != nil {
			r.ntfnMgr.NotifySpentAndMissedTickets(tnd)
		}

	// Stake tickets are matured from the most recently connected block.
	case blockchain.NTNewTickets:
		tnd, ok := notification.Data.(*blockchain.TicketNotificationsData)
		if !ok {
			bmgrLog.Warnf("Tickets connected notification is not " +
				"TicketNotificationsData")
			break
		}

		if r := b.cfg.RpcServer(); r != nil {
			r.ntfnMgr.NotifyNewTickets(tnd)
		}

	// A block has been disconnected from the main block chain.
	case blockchain.NTBlockDisconnected:
		blockSlice, ok := notification.Data.([]*dcrutil.Block)
		if !ok {
			bmgrLog.Warnf("Block disconnected notification is not a block slice.")
			break
		}

		if len(blockSlice) != 2 {
			bmgrLog.Warnf("Block disconnected notification is wrong size slice.")
			break
		}

		block := blockSlice[0]
		parentBlock := blockSlice[1]

		// In the case the regular tree of the previous block was disapproved,
		// disconnecting the current block makes all of those transactions valid
		// again.  Thus, with the exception of the coinbase, remove all of those
		// transactions and any that are now double spends from the transaction
		// pool.  Transactions which depend on a confirmed transaction are NOT
		// removed recursively because they are still valid.
		txMemPool := b.cfg.TxMemPool
		if !headerApprovesParent(&block.MsgBlock().Header) {
			for _, tx := range parentBlock.Transactions()[1:] {
				txMemPool.RemoveTransaction(tx, false)
				txMemPool.RemoveDoubleSpends(tx)
				txMemPool.RemoveOrphan(tx)
				txMemPool.ProcessOrphans(tx)
			}
		}

		// Add all of the regular and stake transactions in the disconnected
		// block, with the exception of the regular tree coinbase, back to the
		// transaction pool to be mined in a future block.
		//
		// Notice that, in the case the previous block was disapproved, some of
		// the transactions in the block being disconnected might have been
		// included in the previous block and others might also have been
		// spending some of the same outputs.  This is the expected behavior
		// because disapproval of the regular tree of the previous block
		// essentially makes it as if those transactions never happened, so
		// disconnecting the block that disapproved those transactions
		// effectively revives them.
		//
		// Finally, if transactions fail to add to the pool for some reason
		// other than the pool already having it (a duplicate) or now being a
		// double spend, remove all transactions that depend on it as well.
		// The dependencies are not removed for double spends because the only
		// way a transaction which was not a double spend in the block being
		// disconnected to now be one is due to some transaction in the previous
		// block (probably the same one), which was disapproved, also spending
		// those outputs, and, in that case, anything that happens to be in the
		// pool which depends on the transaction is still valid.
		handleDisconnectedBlockTxns := func(txns []*dcrutil.Tx) {
			for _, tx := range txns {
				_, err := txMemPool.MaybeAcceptTransaction(tx, false, true)
				if err != nil && !isDoubleSpendOrDuplicateError(err) {
					txMemPool.RemoveTransaction(tx, true)
				}
			}
		}
		handleDisconnectedBlockTxns(block.Transactions()[1:])
		handleDisconnectedBlockTxns(block.STransactions())

		if b.cfg.BgBlkTmplGenerator != nil {
			b.cfg.BgBlkTmplGenerator.BlockDisconnected(block)
		}

		// Notify registered websocket clients.
		if r := b.cfg.RpcServer(); r != nil {
			// Filter and update the rebroadcast inventory.
			b.cfg.PruneRebroadcastInventory()

			// Notify registered websocket clients.
			r.ntfnMgr.NotifyBlockDisconnected(block)
		}

	// Chain reorganization has commenced.
	case blockchain.NTChainReorgStarted:
		if b.cfg.BgBlkTmplGenerator != nil {
			b.cfg.BgBlkTmplGenerator.ChainReorgStarted()
		}

	// Chain reorganization has concluded.
	case blockchain.NTChainReorgDone:
		if b.cfg.BgBlkTmplGenerator != nil {
			b.cfg.BgBlkTmplGenerator.ChainReorgDone()
		}

	// The blockchain is reorganizing.
	case blockchain.NTReorganization:
		rd, ok := notification.Data.(*blockchain.ReorganizationNtfnsData)
		if !ok {
			bmgrLog.Warnf("Chain reorganization notification is malformed")
			break
		}

		// Notify registered websocket clients.
		if r := b.cfg.RpcServer(); r != nil {
			r.ntfnMgr.NotifyReorganization(rd)
		}
	}
}

// NewPeer informs the block manager of a newly active peer.
func (b *blockManager) NewPeer(sp *serverPeer) {
	// Ignore if we are shutting down.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		return
	}
	b.msgChan <- &newPeerMsg{peer: sp}
}

// QueueTx adds the passed transaction message and peer to the block handling
// queue.
func (b *blockManager) QueueTx(tx *dcrutil.Tx, sp *serverPeer) {
	// Don't accept more transactions if we're shutting down.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		sp.txProcessed <- struct{}{}
		return
	}

	b.msgChan <- &txMsg{tx: tx, peer: sp}
}

// QueueBlock adds the passed block message and peer to the block handling queue.
func (b *blockManager) QueueBlock(block *dcrutil.Block, sp *serverPeer) {
	// Don't accept more blocks if we're shutting down.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		sp.blockProcessed <- struct{}{}
		return
	}

	b.msgChan <- &blockMsg{block: block, peer: sp}
}

// QueueInv adds the passed inv message and peer to the block handling queue.
func (b *blockManager) QueueInv(inv *wire.MsgInv, sp *serverPeer) {
	// No channel handling here because peers do not need to block on inv
	// messages.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		return
	}

	b.msgChan <- &invMsg{inv: inv, peer: sp}
}

// QueueHeaders adds the passed headers message and peer to the block handling
// queue.
func (b *blockManager) QueueHeaders(headers *wire.MsgHeaders, sp *serverPeer) {
	// No channel handling here because peers do not need to block on
	// headers messages.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		return
	}

	b.msgChan <- &headersMsg{headers: headers, peer: sp}
}

// DonePeer informs the blockmanager that a peer has disconnected.
func (b *blockManager) DonePeer(sp *serverPeer) {
	// Ignore if we are shutting down.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		return
	}

	b.msgChan <- &donePeerMsg{peer: sp}
}

// Start begins the core block handler which processes block and inv messages.
func (b *blockManager) Start() {
	// Already started?
	if atomic.AddInt32(&b.started, 1) != 1 {
		return
	}

	bmgrLog.Trace("Starting block manager")
	b.wg.Add(1)
	go b.blockHandler()
}

// Stop gracefully shuts down the block manager by stopping all asynchronous
// handlers and waiting for them to finish.
func (b *blockManager) Stop() error {
	if atomic.AddInt32(&b.shutdown, 1) != 1 {
		bmgrLog.Warnf("Block manager is already in the process of " +
			"shutting down")
		return nil
	}

	bmgrLog.Infof("Block manager shutting down")
	close(b.quit)
	b.wg.Wait()
	return nil
}

// SyncPeer returns the current sync peer.
func (b *blockManager) SyncPeer() *serverPeer {
	reply := make(chan *serverPeer)
	b.msgChan <- getSyncPeerMsg{reply: reply}
	return <-reply
}

// RequestFromPeer allows an outside caller to request blocks or transactions
// from a peer. The requests are logged in the blockmanager's internal map of
// requests so they do not later ban the peer for sending the respective data.
func (b *blockManager) RequestFromPeer(p *serverPeer, blocks, txs []*chainhash.Hash) error {
	reply := make(chan requestFromPeerResponse)
	b.msgChan <- requestFromPeerMsg{peer: p, blocks: blocks, txs: txs,
		reply: reply}
	response := <-reply

	return response.err
}

func (b *blockManager) requestFromPeer(p *serverPeer, blocks, txs []*chainhash.Hash) error {
	msgResp := wire.NewMsgGetData()

	// Add the blocks to the request.
	for _, bh := range blocks {
		// If we've already requested this block, skip it.
		_, alreadyReqP := p.requestedBlocks[*bh]
		_, alreadyReqB := b.requestedBlocks[*bh]

		if alreadyReqP || alreadyReqB {
			continue
		}

		// Check to see if we already have this block, too.
		// If so, skip.
		exists, err := b.cfg.Chain.HaveBlock(bh)
		if err != nil {
			return err
		}
		if exists {
			continue
		}

		err = msgResp.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, bh))
		if err != nil {
			return fmt.Errorf("unexpected error encountered building request "+
				"for mining state block %v: %v",
				bh, err.Error())
		}

		p.requestedBlocks[*bh] = struct{}{}
		b.requestedBlocks[*bh] = struct{}{}
	}

	// Add the vote transactions to the request.
	for _, vh := range txs {
		// If we've already requested this transaction, skip it.
		_, alreadyReqP := p.requestedTxns[*vh]
		_, alreadyReqB := b.requestedTxns[*vh]

		if alreadyReqP || alreadyReqB {
			continue
		}

		// Ask the transaction memory pool if the transaction is known
		// to it in any form (main pool or orphan).
		if b.cfg.TxMemPool.HaveTransaction(vh) {
			continue
		}

		// Check if the transaction exists from the point of view of the
		// end of the main chain.
		entry, err := b.cfg.Chain.FetchUtxoEntry(vh)
		if err != nil {
			return err
		}
		if entry != nil {
			continue
		}

		err = msgResp.AddInvVect(wire.NewInvVect(wire.InvTypeTx, vh))
		if err != nil {
			return fmt.Errorf("unexpected error encountered building request "+
				"for mining state vote %v: %v",
				vh, err.Error())
		}

		p.requestedTxns[*vh] = struct{}{}
		b.requestedTxns[*vh] = struct{}{}
	}

	if len(msgResp.InvList) > 0 {
		p.QueueMessage(msgResp, nil)
	}

	return nil
}

// CalcNextRequiredDifficulty calculates the required difficulty for the next
// block after the current main chain.  This function makes use of
// CalcNextRequiredDifficulty on an internal instance of a block chain.  It is
// funneled through the block manager since blockchain is not safe for concurrent
// access.
func (b *blockManager) CalcNextRequiredDifficulty(timestamp time.Time) (uint32, error) {
	reply := make(chan calcNextReqDifficultyResponse)
	b.msgChan <- calcNextReqDifficultyMsg{timestamp: timestamp, reply: reply}
	response := <-reply
	return response.difficulty, response.err
}

// CalcNextRequiredDiffNode calculates the required difficulty for the next
// block after the passed block hash.  This function makes use of
// CalcNextRequiredDiffFromNode on an internal instance of a block chain.  It is
// funneled through the block manager since blockchain is not safe for concurrent
// access.
func (b *blockManager) CalcNextRequiredDiffNode(hash *chainhash.Hash, timestamp time.Time) (uint32, error) {
	reply := make(chan calcNextReqDifficultyResponse)
	b.msgChan <- calcNextReqDiffNodeMsg{
		hash:      hash,
		timestamp: timestamp,
		reply:     reply,
	}
	response := <-reply
	return response.difficulty, response.err
}

// CalcNextRequiredStakeDifficulty calculates the required Stake difficulty for
// the next block after the current main chain.  This function makes use of
// CalcNextRequiredStakeDifficulty on an internal instance of a block chain.  It is
// funneled through the block manager since blockchain is not safe for concurrent
// access.
func (b *blockManager) CalcNextRequiredStakeDifficulty() (int64, error) {
	reply := make(chan calcNextReqStakeDifficultyResponse)
	b.msgChan <- calcNextReqStakeDifficultyMsg{reply: reply}
	response := <-reply
	return response.stakeDifficulty, response.err
}

// ForceReorganization returns the hashes of all the children of a parent for the
// block hash that is passed to the function. It is funneled through the block
// manager since blockchain is not safe for concurrent access.
func (b *blockManager) ForceReorganization(formerBest, newBest chainhash.Hash) error {
	reply := make(chan forceReorganizationResponse)
	b.msgChan <- forceReorganizationMsg{
		formerBest: formerBest,
		newBest:    newBest,
		reply:      reply}
	response := <-reply
	return response.err
}

// TipGeneration returns the hashes of all the children of the current best
// chain tip.  It is funneled through the block manager since blockchain is not
// safe for concurrent access.
func (b *blockManager) TipGeneration() ([]chainhash.Hash, error) {
	reply := make(chan tipGenerationResponse)
	b.msgChan <- tipGenerationMsg{reply: reply}
	response := <-reply
	return response.hashes, response.err
}

// ProcessBlock makes use of ProcessBlock on an internal instance of a block
// chain.  It is funneled through the block manager since blockchain is not safe
// for concurrent access.
func (b *blockManager) ProcessBlock(block *dcrutil.Block, flags blockchain.BehaviorFlags) (bool, error) {
	reply := make(chan processBlockResponse, 1)
	b.msgChan <- processBlockMsg{block: block, flags: flags, reply: reply}
	response := <-reply
	return response.isOrphan, response.err
}

// ProcessTransaction makes use of ProcessTransaction on an internal instance of
// a block chain.  It is funneled through the block manager since blockchain is
// not safe for concurrent access.
func (b *blockManager) ProcessTransaction(tx *dcrutil.Tx, allowOrphans bool,
	rateLimit bool, allowHighFees bool) ([]*dcrutil.Tx, error) {
	reply := make(chan processTransactionResponse, 1)
	b.msgChan <- processTransactionMsg{tx, allowOrphans, rateLimit,
		allowHighFees, reply}
	response := <-reply
	return response.acceptedTxs, response.err
}

// IsCurrent returns whether or not the block manager believes it is synced with
// the connected peers.
func (b *blockManager) IsCurrent() bool {
	reply := make(chan bool)
	b.msgChan <- isCurrentMsg{reply: reply}
	return <-reply
}

// TicketPoolValue returns the current value of the total stake in the ticket
// pool.
func (b *blockManager) TicketPoolValue() (dcrutil.Amount, error) {
	return b.cfg.Chain.TicketPoolValue()
}

// newBlockManager returns a new Decred block manager.
// Use Start to begin processing asynchronous block and inv updates.
func newBlockManager(config *blockManagerConfig) (*blockManager, error) {
	bm := blockManager{
		cfg:              config,
		rejectedTxns:     make(map[chainhash.Hash]struct{}),
		requestedTxns:    make(map[chainhash.Hash]struct{}),
		requestedBlocks:  make(map[chainhash.Hash]struct{}),
		progressLogger:   newBlockProgressLogger("Processed", bmgrLog),
		msgChan:          make(chan interface{}, cfg.MaxPeers*3),
		headerList:       list.New(),
		AggressiveMining: !cfg.NonAggressive,
		quit:             make(chan struct{}),
	}

	best := bm.cfg.Chain.BestSnapshot()
	bm.cfg.Chain.DisableCheckpoints(cfg.DisableCheckpoints)
	if !cfg.DisableCheckpoints {
		// Initialize the next checkpoint based on the current height.
		bm.nextCheckpoint = bm.findNextHeaderCheckpoint(best.Height)
		if bm.nextCheckpoint != nil {
			bm.resetHeaderState(&best.Hash, best.Height)
		}
	} else {
		bmgrLog.Info("Checkpoints are disabled")
	}

	// Dump the blockchain here if asked for it, and quit.
	if cfg.DumpBlockchain != "" {
		err := dumpBlockChain(bm.cfg.Chain, best.Height)
		if err != nil {
			return nil, err
		}

		return nil, fmt.Errorf("closing after dumping blockchain")
	}

	bm.lotteryDataBroadcast = make(map[chainhash.Hash]struct{})
	bm.syncHeightMtx.Lock()
	bm.syncHeight = best.Height
	bm.syncHeightMtx.Unlock()

	return &bm, nil
}

// removeRegressionDB removes the existing regression test database if running
// in regression test mode and it already exists.
func removeRegressionDB(dbPath string) error {
	// Don't do anything if not in regression test mode.
	if !cfg.RegNet {
		return nil
	}

	// Remove the old regression test database if it already exists.
	fi, err := os.Stat(dbPath)
	if err == nil {
		dcrdLog.Infof("Removing regression test database from '%s'", dbPath)
		if fi.IsDir() {
			err := os.RemoveAll(dbPath)
			if err != nil {
				return err
			}
		} else {
			err := os.Remove(dbPath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// blockDbPath returns the path to the block database given a database type.
func blockDbPath(dbType string) string {
	// The database name is based on the database type.
	dbName := blockDbNamePrefix + "_" + dbType
	if dbType == "sqlite" {
		dbName = dbName + ".db"
	}
	dbPath := filepath.Join(cfg.DataDir, dbName)
	return dbPath
}

// warnMultipleDBs shows a warning if multiple block database types are detected.
// This is not a situation most users want.  It is handy for development however
// to support multiple side-by-side databases.
func warnMultipleDBs() {
	// This is intentionally not using the known db types which depend
	// on the database types compiled into the binary since we want to
	// detect legacy db types as well.
	dbTypes := []string{"ffldb", "leveldb", "sqlite"}
	duplicateDbPaths := make([]string, 0, len(dbTypes)-1)
	for _, dbType := range dbTypes {
		if dbType == cfg.DbType {
			continue
		}

		// Store db path as a duplicate db if it exists.
		dbPath := blockDbPath(dbType)
		if fileExists(dbPath) {
			duplicateDbPaths = append(duplicateDbPaths, dbPath)
		}
	}

	// Warn if there are extra databases.
	if len(duplicateDbPaths) > 0 {
		selectedDbPath := blockDbPath(cfg.DbType)
		dcrdLog.Warnf("WARNING: There are multiple block chain databases "+
			"using different database types.\nYou probably don't "+
			"want to waste disk space by having more than one.\n"+
			"Your current database is located at [%v].\nThe "+
			"additional database is located at %v", selectedDbPath,
			duplicateDbPaths)
	}
}

// loadBlockDB loads (or creates when needed) the block database taking into
// account the selected database backend and returns a handle to it.  It also
// contains additional logic such warning the user if there are multiple
// databases which consume space on the file system and ensuring the regression
// test database is clean when in regression test mode.
func loadBlockDB() (database.DB, error) {
	// The memdb backend does not have a file path associated with it, so
	// handle it uniquely.  We also don't want to worry about the multiple
	// database type warnings when running with the memory database.
	if cfg.DbType == "memdb" {
		dcrdLog.Infof("Creating block database in memory.")
		db, err := database.Create(cfg.DbType)
		if err != nil {
			return nil, err
		}
		return db, nil
	}

	warnMultipleDBs()

	// The database name is based on the database type.
	dbPath := blockDbPath(cfg.DbType)

	// The regression test is special in that it needs a clean database for
	// each run, so remove it now if it already exists.
	removeRegressionDB(dbPath)

	dcrdLog.Infof("Loading block database from '%s'", dbPath)
	db, err := database.Open(cfg.DbType, dbPath, activeNetParams.Net)
	if err != nil {
		// Return the error if it's not because the database doesn't
		// exist.
		if dbErr, ok := err.(database.Error); !ok || dbErr.ErrorCode !=
			database.ErrDbDoesNotExist {

			return nil, err
		}

		// Create the db if it does not exist.
		err = os.MkdirAll(cfg.DataDir, 0700)
		if err != nil {
			return nil, err
		}
		db, err = database.Create(cfg.DbType, dbPath, activeNetParams.Net)
		if err != nil {
			return nil, err
		}
	}

	dcrdLog.Info("Block database loaded")
	return db, nil
}

// dumpBlockChain dumps a map of the blockchain blocks as serialized bytes.
func dumpBlockChain(b *blockchain.BlockChain, height int64) error {
	bmgrLog.Infof("Writing the blockchain to disk as a flat file, " +
		"please wait...")

	progressLogger := newBlockProgressLogger("Written", bmgrLog)

	file, err := os.Create(cfg.DumpBlockchain)
	if err != nil {
		return err
	}
	defer file.Close()

	// Store the network ID in an array for later writing.
	var net [4]byte
	binary.LittleEndian.PutUint32(net[:], uint32(activeNetParams.Net))

	// Write the blocks sequentially, excluding the genesis block.
	var sz [4]byte
	for i := int64(1); i <= height; i++ {
		bl, err := b.BlockByHeight(i)
		if err != nil {
			return err
		}

		// Serialize the block for writing.
		blB, err := bl.Bytes()
		if err != nil {
			return err
		}

		// Write the network ID first.
		_, err = file.Write(net[:])
		if err != nil {
			return err
		}

		// Write the size of the block as a little endian uint32,
		// then write the block itself serialized.
		binary.LittleEndian.PutUint32(sz[:], uint32(len(blB)))
		_, err = file.Write(sz[:])
		if err != nil {
			return err
		}

		_, err = file.Write(blB)
		if err != nil {
			return err
		}

		progressLogger.logBlockHeight(bl)
	}

	bmgrLog.Infof("Successfully dumped the blockchain (%v blocks) to %v.",
		height, cfg.DumpBlockchain)

	return nil
}
