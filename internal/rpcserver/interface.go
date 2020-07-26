// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcserver

import (
	"context"
	"math/big"
	"net"
	"time"

	"github.com/decred/dcrd/addrmgr"
	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/blockchain/v3"
	"github.com/decred/dcrd/blockchain/v3/indexers"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/gcs/v2"
	"github.com/decred/dcrd/internal/mempool"
	"github.com/decred/dcrd/internal/mining"
	"github.com/decred/dcrd/peer/v2"
	"github.com/decred/dcrd/wire"
)

// Peer represents a peer for use with the RPC server.
//
// The interface contract requires that all of these methods are safe for
// concurrent access.
type Peer interface {
	// Addr returns the peer address.
	Addr() string

	// Connected returns whether or not the peer is currently connected.
	Connected() bool

	// ID returns the peer id.
	ID() int32

	// Inbound returns whether the peer is inbound.
	Inbound() bool

	// StatsSnapshot returns a snapshot of the current peer flags and statistics.
	StatsSnapshot() *peer.StatsSnap

	// LocalAddr returns the local address of the connection.
	LocalAddr() net.Addr

	// LastPingNonce returns the last ping nonce of the remote peer.
	LastPingNonce() uint64

	// IsTxRelayDisabled returns whether or not the peer has disabled
	// transaction relay.
	IsTxRelayDisabled() bool

	// BanScore returns the current integer value that represents how close
	// the peer is to being banned.
	BanScore() uint32
}

// AddrManager represents an address manager for use with the RPC server.
//
// The interface contract requires that all of these methods are safe for
// concurrent access.
type AddrManager interface {
	// LocalAddresses returns a summary of local addresses information for
	// the getnetworkinfo rpc.
	LocalAddresses() []addrmgr.LocalAddr
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

	// Lookup defines the DNS lookup function to be used.
	Lookup(host string) ([]net.IP, error)
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
		allowHighFees bool, tag mempool.Tag) ([]*dcrutil.Tx, error)
}

// UtxoEntry represents a utxo entry for use with the RPC server.
//
// The interface contract does NOT require that these methods are safe for
// concurrent access.
type UtxoEntry interface {
	// ToUtxoEntry returns the underlying UtxoEntry instance.
	ToUtxoEntry() *blockchain.UtxoEntry

	// TransactionType returns the type of the transaction the utxo entry
	// represents.
	TransactionType() stake.TxType

	// IsOutputSpent returns whether or not the provided output index has been
	// spent based upon the current state of the unspent transaction output view
	// the entry was obtained from.
	//
	// Returns true if the output index references an output that does not exist
	// either due to it being invalid or because the output is not part of the view
	// due to previously being spent/pruned.
	IsOutputSpent(outputIndex uint32) bool

	// BlockHeight returns the height of the block containing the transaction the
	// utxo entry represents.
	BlockHeight() int64

	// TxVersion returns the version of the transaction the utxo represents.
	TxVersion() uint16

	// AmountByIndex returns the amount of the provided output index.
	//
	// Returns 0 if the output index references an output that does not exist
	// either due to it being invalid or because the output is not part of the view
	// due to previously being spent/pruned.
	AmountByIndex(outputIndex uint32) int64

	// ScriptVersionByIndex returns the public key script for the provided output
	// index.
	//
	// Returns 0 if the output index references an output that does not exist
	// either due to it being invalid or because the output is not part of the view
	// due to previously being spent/pruned.
	ScriptVersionByIndex(outputIndex uint32) uint16

	// PkScriptByIndex returns the public key script for the provided output index.
	//
	// Returns nil if the output index references an output that does not exist
	// either due to it being invalid or because the output is not part of the view
	// due to previously being spent/pruned.
	PkScriptByIndex(outputIndex uint32) []byte

	// IsCoinBase returns whether or not the transaction the utxo entry represents
	// is a coinbase.
	IsCoinBase() bool
}

// Chain represents a chain for use with the RPC server.
//
// The interface contract requires that all of these methods are safe for
// concurrent access.
type Chain interface {
	// BestSnapshot returns information about the current best chain block and
	// related state as of the current point in time.  The returned instance must be
	// treated as immutable since it is shared by all callers.
	BestSnapshot() *blockchain.BestState

	// BlockByHash returns the block for the given hash, regardless of whether the
	// block is part of the main chain or not.
	BlockByHash(hash *chainhash.Hash) (*dcrutil.Block, error)

	// BlockByHeight returns the block at the given height in the main chain.
	BlockByHeight(height int64) (*dcrutil.Block, error)

	// BlockHashByHeight returns the hash of the block at the given height in the
	// main chain.
	BlockHashByHeight(height int64) (*chainhash.Hash, error)

	// BlockHeightByHash returns the height of the block with the given hash in the
	// main chain.
	BlockHeightByHash(hash *chainhash.Hash) (int64, error)

	// CalcNextRequiredStakeDifficulty calculates the required stake difficulty for
	// the block after the end of the current best chain based on the active stake
	// difficulty retarget rules.
	CalcNextRequiredStakeDifficulty() (int64, error)

	// CalcWantHeight calculates the height of the final block of the previous
	// interval given a block height.
	CalcWantHeight(interval, height int64) int64

	// ChainTips returns information, in JSON-RPC format, about all of the currently
	// known chain tips in the block index.
	ChainTips() []blockchain.ChainTipInfo

	// ChainWork returns the total work up to and including the block of the
	// provided block hash.
	ChainWork(hash *chainhash.Hash) (*big.Int, error)

	// CheckExpiredTicket returns whether or not a ticket was ever expired.
	CheckExpiredTickets(hashes []chainhash.Hash) []bool

	// CheckLiveTicket returns whether or not a ticket exists in the live ticket
	// treap of the best node.
	CheckLiveTicket(hash chainhash.Hash) bool

	// CheckLiveTickets returns a slice of bools representing whether each ticket
	// exists in the live ticket treap of the best node.
	CheckLiveTickets(hashes []chainhash.Hash) []bool

	// CheckMissedTickets returns a slice of bools representing whether each ticket
	// hash has been missed in the live ticket treap of the best node.
	CheckMissedTickets(hashes []chainhash.Hash) []bool

	// ConvertUtxosToMinimalOutputs converts the contents of a UTX to a series of
	// minimal outputs. It does this so that these can be passed to stake subpackage
	// functions, where they will be evaluated for correctness.
	ConvertUtxosToMinimalOutputs(entry UtxoEntry) []*stake.MinimalOutput

	// CountVoteVersion returns the total number of version votes for the current
	// rule change activation interval.
	CountVoteVersion(version uint32) (uint32, error)

	// EstimateNextStakeDifficulty estimates the next stake difficulty by pretending
	// the provided number of tickets will be purchased in the remainder of the
	// interval unless the flag to use max tickets is set in which case it will use
	// the max possible number of tickets that can be purchased in the remainder of
	// the interval.
	EstimateNextStakeDifficulty(newTickets int64, useMaxTickets bool) (int64, error)

	// FetchUtxoEntry loads and returns the unspent transaction output entry for the
	// passed hash from the point of view of the end of the main chain.
	//
	// NOTE: Requesting a hash for which there is no data must NOT return an error.
	// Instead both the entry and the error must be nil.  This is done to allow
	// pruning of fully spent transactions.  In practice this means the caller must
	// check if the returned entry is nil before invoking methods on it.
	//
	// This function is safe for concurrent access however the returned entry (if
	// any) is NOT.
	FetchUtxoEntry(txHash *chainhash.Hash) (UtxoEntry, error)

	// FetchUtxoStats returns statistics on the current utxo set.
	FetchUtxoStats() (*blockchain.UtxoStats, error)

	// FilterByBlockHash returns the version 2 GCS filter for the given block hash
	// when it exists.  This function returns the filters regardless of whether or
	// not their associated block is part of the main chain.
	//
	// An error of type blockchain.NoFilterError must be returned when the filter
	// for the given block hash does not exist.
	FilterByBlockHash(hash *chainhash.Hash) (*gcs.FilterV2, error)

	// GetStakeVersions returns a cooked array of StakeVersions.  We do this in
	// order to not bloat memory by returning raw blocks.
	GetStakeVersions(hash *chainhash.Hash, count int32) ([]blockchain.StakeVersions, error)

	// GetVoteCounts returns the vote counts for the specified version and
	// deployment identifier for the current rule change activation interval.
	GetVoteCounts(version uint32, deploymentID string) (blockchain.VoteCounts, error)

	// GetVoteInfo returns information on consensus deployment agendas
	// and their respective states at the provided hash, for the provided
	// deployment version.
	GetVoteInfo(hash *chainhash.Hash, version uint32) (*blockchain.VoteInfo, error)

	// HeaderByHash returns the block header identified by the given hash or an
	// error if it doesn't exist.  Note that this will return headers from both the
	// main chain and any side chains.
	HeaderByHash(hash *chainhash.Hash) (wire.BlockHeader, error)

	// HeaderByHeight returns the block header at the given height in the main
	// chain.
	HeaderByHeight(height int64) (wire.BlockHeader, error)

	// HeightRange returns a range of block hashes for the given start and end
	// heights.  It is inclusive of the start height and exclusive of the end
	// height.  In other words, it is the half open range [startHeight, endHeight).
	//
	// The end height will be limited to the current main chain height.
	HeightRange(startHeight, endHeight int64) ([]chainhash.Hash, error)

	// IsCurrent returns whether or not the chain believes it is current.  Several
	// factors are used to guess, but the key factors that allow the chain to
	// believe it is current are:
	//  - Total amount of cumulative work is more than the minimum known work
	//    specified by the parameters for the network
	//  - Latest block has a timestamp newer than 24 hours ago
	IsCurrent() bool

	// LiveTickets returns all currently live tickets.
	LiveTickets() ([]chainhash.Hash, error)

	// LocateHeaders returns the headers of the blocks after the first known block
	// in the locator until the provided stop hash is reached, or up to a max of
	// wire.MaxBlockHeadersPerMsg headers.
	//
	// In addition, there are two special cases:
	//
	// - When no locators are provided, the stop hash is treated as a request for
	//   that header, so it will either return the header for the stop hash itself
	//   if it is known, or nil if it is unknown
	// - When locators are provided, but none of them are known, headers starting
	//   after the genesis block will be returned
	LocateHeaders(locator blockchain.BlockLocator, hashStop *chainhash.Hash) []wire.BlockHeader

	// LotteryDataForBlock returns lottery data for a given block in the block
	// chain, including side chain blocks.
	LotteryDataForBlock(hash *chainhash.Hash) ([]chainhash.Hash, int, [6]byte, error)

	// MainChainHasBlock returns whether or not the block with the given hash is in
	// the main chain.
	MainChainHasBlock(hash *chainhash.Hash) bool

	// MaxBlockSize returns the maximum permitted block size for the block AFTER
	// the end of the current best chain.
	MaxBlockSize() (int64, error)

	// MissedTickets returns all currently missed tickets.
	MissedTickets() ([]chainhash.Hash, error)

	// NextThresholdState returns the current rule change threshold state of the
	// given deployment ID for the block AFTER the provided block hash.
	NextThresholdState(hash *chainhash.Hash, version uint32, deploymentID string) (blockchain.ThresholdStateTuple, error)

	// StateLastChangedHeight returns the height at which the provided consensus
	// deployment agenda last changed state.  Note that, unlike the
	// NextThresholdState function, this function returns the information as of the
	// passed block hash.
	StateLastChangedHeight(hash *chainhash.Hash, version uint32, deploymentID string) (int64, error)

	// TicketPoolValue returns the current value of all the locked funds in the
	// ticket pool.
	TicketPoolValue() (dcrutil.Amount, error)

	// TicketsWithAddress returns a slice of ticket hashes that are currently live
	// corresponding to the given address.
	TicketsWithAddress(address dcrutil.Address) ([]chainhash.Hash, error)

	// TipGeneration returns the entire generation of blocks stemming from the
	// parent of the current tip.
	TipGeneration() ([]chainhash.Hash, error)
}

// Clock represents a clock for use with the RPC server. The purpose of this
// interface is to allow an alternative implementation to be used for testing.
//
// The interface contract requires that all of these methods are safe for
// concurrent access.
type Clock interface {
	// Now returns the current local time.
	Now() time.Time

	// Since returns the time elapsed since t.
	Since(t time.Time) time.Duration
}

// FeeEstimator provides an interface that tracks historical data for published
// and mined transactions in order to estimate fees to be used in new
// transactions for confirmation within a target block window.
//
// The interface contract requires that all of these methods are safe for
// concurrent access.
type FeeEstimator interface {
	// EstimateFee calculates the suggested fee for a transaction to be
	// confirmed in at most `targetConfs` blocks after publishing with a
	// high degree of certainty.
	EstimateFee(targetConfs int32) (dcrutil.Amount, error)
}

// LogManager represents a log manager for use with the RPC server.
//
// The interface contract does NOT require that these methods are safe for
// concurrent access.
type LogManager interface {
	// SupportedSubsystems returns a sorted slice of the supported subsystems for
	// logging purposes.
	SupportedSubsystems() []string

	// ParseAndSetDebugLevels attempts to parse the specified debug level and set
	// the levels accordingly.  An appropriate error must be returned if anything
	// is invalid.
	ParseAndSetDebugLevels(debugLevel string) error
}

// SanityChecker represents a block sanity checker for use with the RPC server.
//
// The interface contract requires that all of these methods are safe for
// concurrent access.
type SanityChecker interface {
	// CheckBlockSanity checks the correctness of the provided block
	// per consensus.
	CheckBlockSanity(block *dcrutil.Block) error
}

// CPUMiner represents a CPU miner for use with the RPC server. The purpose of
// this interface is to allow an alternative implementation to be used for
// testing.
//
// The interface contract requires that all of these methods are safe for
// concurrent access.
type CPUMiner interface {
	// GenerateNBlocks generates the requested number of blocks.
	GenerateNBlocks(ctx context.Context, n uint32) ([]*chainhash.Hash, error)

	// IsMining returns whether or not the CPU miner has been started and is
	// therefore currently mining.
	IsMining() bool

	// HashesPerSecond returns the number of hashes per second the mining process
	// is performing.
	HashesPerSecond() float64

	// NumWorkers returns the number of workers which are running to solve blocks.
	NumWorkers() int32

	// SetNumWorkers sets the number of workers to create which solve blocks.
	SetNumWorkers(numWorkers int32)
}

// BlockTemplater represents a source of block templates for use with the
// RPC server.
//
// The interface contract requires that all of these methods are safe for
// concurrent access.
type BlockTemplater interface {
	// ForceRegen asks the block templater to generate a new template immediately.
	ForceRegen()

	// Subscribe subscribes a client for block template updates.  The returned
	// template subscription contains functions to retrieve a channel that produces
	// the stream of block templates and to stop the stream when the caller no
	// longer wishes to receive new templates.
	Subscribe() *mining.TemplateSubscription

	// CurrentTemplate returns the current template associated with the block
	// templater along with any associated error.
	CurrentTemplate() (*mining.BlockTemplate, error)

	// UpdateBlockTime updates the timestamp in the passed header to the current
	// time while taking into account the consensus rules.
	UpdateBlockTime(header *wire.BlockHeader) error
}
