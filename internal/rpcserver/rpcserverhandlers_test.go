// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcserver

import (
	"bytes"
	"compress/bzip2"
	"context"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/addrmgr"
	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/blockchain/v3"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrjson/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/gcs/v2"
	"github.com/decred/dcrd/gcs/v2/blockcf2"
	"github.com/decred/dcrd/internal/mempool"
	"github.com/decred/dcrd/internal/mining"
	"github.com/decred/dcrd/internal/version"
	"github.com/decred/dcrd/peer/v2"
	"github.com/decred/dcrd/rpc/jsonrpc/types/v2"
	"github.com/decred/dcrd/wire"
)

// testDataPath is the path where all rpcserver test fixtures reside.
const testDataPath = "testdata"

// testRPCUtxoEntry provides a mock utxo entry by implementing the UtxoEntry interface.
type testRPCUtxoEntry struct {
	amountByIndex        int64
	hasExpiry            bool
	height               uint32
	index                uint32
	isCoinBase           bool
	isOutputSpent        bool
	modified             bool
	pkScriptByIndex      []byte
	scriptVersionByIndex uint16
	txType               stake.TxType
	txVersion            uint16
}

// ToUtxoEntry returns a mocked underlying UtxoEntry instance.
func (u *testRPCUtxoEntry) ToUtxoEntry() *blockchain.UtxoEntry {
	return nil
}

// TransactionType returns a mocked txType of the testRPCUtxoEntry.
func (u *testRPCUtxoEntry) TransactionType() stake.TxType {
	return u.txType
}

// IsOutputSpent returns a mocked bool representing whether or not the provided
// output index has been spent.
func (u *testRPCUtxoEntry) IsOutputSpent(outputIndex uint32) bool {
	return u.isOutputSpent
}

// BlockHeight returns a mocked height of the testRPCUtxoEntry.
func (u *testRPCUtxoEntry) BlockHeight() int64 {
	return int64(u.height)
}

// TxVersion returns a mocked txVersion of the testRPCUtxoEntry.
func (u *testRPCUtxoEntry) TxVersion() uint16 {
	return u.txVersion
}

// AmountByIndex returns a mocked amount of the provided output index.
func (u *testRPCUtxoEntry) AmountByIndex(outputIndex uint32) int64 {
	return u.amountByIndex
}

// ScriptVersionByIndex returns a mocked public key script for the provided
// output index.
func (u *testRPCUtxoEntry) ScriptVersionByIndex(outputIndex uint32) uint16 {
	return u.scriptVersionByIndex
}

// PkScriptByIndex returns a mocked public key script for the provided output
// index.
func (u *testRPCUtxoEntry) PkScriptByIndex(outputIndex uint32) []byte {
	return u.pkScriptByIndex
}

// IsCoinBase returns a mocked isCoinBase bool of the testRPCUtxoEntry.
func (u *testRPCUtxoEntry) IsCoinBase() bool {
	return u.isCoinBase
}

// testRPCChain provides a mock block chain by implementing the Chain interface.
type testRPCChain struct {
	bestSnapshot                    *blockchain.BestState
	blockByHash                     *dcrutil.Block
	blockByHashErr                  error
	blockByHeight                   *dcrutil.Block
	blockByHeightErr                error
	blockHashByHeight               *chainhash.Hash
	blockHashByHeightErr            error
	blockHeightByHash               int64
	calcNextRequiredStakeDifficulty int64
	calcWantHeight                  int64
	chainTips                       []blockchain.ChainTipInfo
	chainWork                       *big.Int
	chainWorkErr                    error
	checkExpiredTickets             []bool
	checkLiveTicket                 bool
	checkLiveTickets                []bool
	checkMissedTickets              []bool
	convertUtxosToMinimalOutputs    []*stake.MinimalOutput
	countVoteVersion                uint32
	estimateNextStakeDifficultyFn   func(newTickets int64, useMaxTickets bool) (diff int64, err error)
	fetchUtxoEntry                  UtxoEntry
	fetchUtxoStats                  *blockchain.UtxoStats
	getStakeVersions                []blockchain.StakeVersions
	getStakeVersionsErr             error
	getVoteCounts                   blockchain.VoteCounts
	getVoteInfo                     *blockchain.VoteInfo
	headerByHash                    wire.BlockHeader
	headerByHashErr                 error
	headerByHeight                  wire.BlockHeader
	headerByHeightErr               error
	heightRange                     []chainhash.Hash
	isCurrent                       bool
	liveTickets                     []chainhash.Hash
	liveTicketsErr                  error
	locateHeaders                   []wire.BlockHeader
	lotteryDataForBlock             []chainhash.Hash
	mainChainHasBlock               bool
	maxBlockSize                    int64
	maxBlockSizeErr                 error
	missedTickets                   []chainhash.Hash
	missedTicketsErr                error
	nextThresholdState              blockchain.ThresholdStateTuple
	nextThresholdStateErr           error
	stateLastChangedHeight          int64
	stateLastChangedHeightErr       error
	ticketPoolValue                 dcrutil.Amount
	ticketPoolValueErr              error
	ticketsWithAddress              []chainhash.Hash
	tipGeneration                   []chainhash.Hash
}

// BestSnapshot returns a mocked blockchain.BestState.
func (c *testRPCChain) BestSnapshot() *blockchain.BestState {
	return c.bestSnapshot
}

// BlockByHash returns a mocked block for the given hash.
func (c *testRPCChain) BlockByHash(hash *chainhash.Hash) (*dcrutil.Block, error) {
	return c.blockByHash, c.blockByHashErr
}

// BlockByHeight returns a mocked block at the given height.
func (c *testRPCChain) BlockByHeight(height int64) (*dcrutil.Block, error) {
	return c.blockByHeight, c.blockByHeightErr
}

// BlockHashByHeight returns a mocked hash of the block at the given height.
func (c *testRPCChain) BlockHashByHeight(height int64) (*chainhash.Hash, error) {
	return c.blockHashByHeight, c.blockHashByHeightErr
}

// BlockHeightByHash returns a mocked height of the block with the given hash.
func (c *testRPCChain) BlockHeightByHash(hash *chainhash.Hash) (int64, error) {
	return c.blockHeightByHash, nil
}

// CalcNextRequiredStakeDifficulty returns a mocked required stake difficulty.
func (c *testRPCChain) CalcNextRequiredStakeDifficulty() (int64, error) {
	return c.calcNextRequiredStakeDifficulty, nil
}

// CalcWantHeight returns a mocked height of the final block of the previous
// interval given a block height.
func (c *testRPCChain) CalcWantHeight(interval, height int64) int64 {
	return c.calcWantHeight
}

// ChainTips returns a mocked []blockchain.ChainTipInfo.
func (c *testRPCChain) ChainTips() []blockchain.ChainTipInfo {
	return c.chainTips
}

// ChainWork returns returns a mocked total work up to and including the block
// of the provided block hash.
func (c *testRPCChain) ChainWork(hash *chainhash.Hash) (*big.Int, error) {
	return c.chainWork, c.chainWorkErr
}

// CheckExpiredTickets returns a mocked slice of bools representing
// whether each ticket hash has expired.
func (c *testRPCChain) CheckExpiredTickets(hashes []chainhash.Hash) []bool {
	return c.checkExpiredTickets
}

// CheckLiveTicket returns a mocked result of whether or not a ticket
// exists in the live ticket treap of the best node.
func (c *testRPCChain) CheckLiveTicket(hash chainhash.Hash) bool {
	return c.checkLiveTicket
}

// CheckLiveTickets returns a mocked slice of bools representing
// whether each ticket exists in the live ticket treap of the best node.
func (c *testRPCChain) CheckLiveTickets(hashes []chainhash.Hash) []bool {
	return c.checkLiveTickets
}

// CheckMissedTickets returns a mocked slice of bools representing
// whether each ticket hash has been missed.
func (c *testRPCChain) CheckMissedTickets(hashes []chainhash.Hash) []bool {
	return c.checkMissedTickets
}

// ConvertUtxosToMinimalOutputs returns a mocked MinimalOutput slice.
func (c *testRPCChain) ConvertUtxosToMinimalOutputs(entry UtxoEntry) []*stake.MinimalOutput {
	return c.convertUtxosToMinimalOutputs
}

// CountVoteVersion returns a mocked total number of version votes for the current
// rule change activation interval.
func (c *testRPCChain) CountVoteVersion(version uint32) (uint32, error) {
	return c.countVoteVersion, nil
}

// EstimateNextStakeDifficulty returns a mocked estimated next stake difficulty.
func (c *testRPCChain) EstimateNextStakeDifficulty(newTickets int64, useMaxTickets bool) (int64, error) {
	return c.estimateNextStakeDifficultyFn(newTickets, useMaxTickets)
}

// FetchUtxoEntry returns a mocked UtxoEntry.
func (c *testRPCChain) FetchUtxoEntry(txHash *chainhash.Hash) (UtxoEntry, error) {
	return c.fetchUtxoEntry, nil
}

// FetchUtxoStats returns a mocked blockchain.UtxoStats.
func (c *testRPCChain) FetchUtxoStats() (*blockchain.UtxoStats, error) {
	return c.fetchUtxoStats, nil
}

// GetStakeVersions returns a mocked cooked array of StakeVersions.
func (c *testRPCChain) GetStakeVersions(hash *chainhash.Hash, count int32) ([]blockchain.StakeVersions, error) {
	return c.getStakeVersions, c.getStakeVersionsErr
}

// GetVoteCounts returns a mocked blockchain.VoteCounts for the specified
// version and deployment identifier for the current rule change activation interval.
func (c *testRPCChain) GetVoteCounts(version uint32, deploymentID string) (blockchain.VoteCounts, error) {
	return c.getVoteCounts, nil
}

// GetVoteInfo returns mocked information on consensus deployment agendas and
// their respective states at the provided hash, for the provided deployment
// version.
func (c *testRPCChain) GetVoteInfo(hash *chainhash.Hash, version uint32) (*blockchain.VoteInfo, error) {
	return c.getVoteInfo, nil
}

// HeaderByHash returns a mocked block header identified by the given hash.
func (c *testRPCChain) HeaderByHash(hash *chainhash.Hash) (wire.BlockHeader, error) {
	return c.headerByHash, c.headerByHashErr
}

// HeaderByHeight returns a mocked block header at the given height.
func (c *testRPCChain) HeaderByHeight(height int64) (wire.BlockHeader, error) {
	return c.headerByHeight, c.headerByHeightErr
}

// HeightRange returns a mocked range of block hashes for the given start and
// end heights.
func (c *testRPCChain) HeightRange(startHeight, endHeight int64) ([]chainhash.Hash, error) {
	return c.heightRange, nil
}

// IsCurrent returns a mocked bool representing whether or not the chain
// believes it is current.
func (c *testRPCChain) IsCurrent() bool {
	return c.isCurrent
}

// LiveTickets returns a mocked slice of all currently live tickets.
func (c *testRPCChain) LiveTickets() ([]chainhash.Hash, error) {
	return c.liveTickets, c.liveTicketsErr
}

// LocateHeaders returns a mocked slice of headers of the blocks after the first
// known block in the locator until the provided stop hash is reached, or up to
// a max of wire.MaxBlockHeadersPerMsg headers.
func (c *testRPCChain) LocateHeaders(locator blockchain.BlockLocator, hashStop *chainhash.Hash) []wire.BlockHeader {
	return c.locateHeaders
}

// LotteryDataForBlock returns mocked lottery data for a given block in the
// block chain, including side chain blocks.
func (c *testRPCChain) LotteryDataForBlock(hash *chainhash.Hash) ([]chainhash.Hash, int, [6]byte, error) {
	return c.lotteryDataForBlock, 0, [6]byte{}, nil
}

// MainChainHasBlock returns a mocked bool representing whether or not the block
// with the given hash is in the main chain.
func (c *testRPCChain) MainChainHasBlock(hash *chainhash.Hash) bool {
	return c.mainChainHasBlock
}

// MaxBlockSize returns a mocked maximum permitted block size for the block
// AFTER the end of the current best chain.
func (c *testRPCChain) MaxBlockSize() (int64, error) {
	return c.maxBlockSize, c.maxBlockSizeErr
}

// MissedTickets returns a mocked slice of all currently missed tickets.
func (c *testRPCChain) MissedTickets() ([]chainhash.Hash, error) {
	return c.missedTickets, c.missedTicketsErr
}

// NextThresholdState returns a mocked current rule change threshold state of
// the given deployment ID for the block AFTER the provided block hash.
func (c *testRPCChain) NextThresholdState(hash *chainhash.Hash, version uint32, deploymentID string) (blockchain.ThresholdStateTuple, error) {
	return c.nextThresholdState, c.nextThresholdStateErr
}

// StateLastChangedHeight returns a mocked height at which the provided
// consensus deployment agenda last changed state.
func (c *testRPCChain) StateLastChangedHeight(hash *chainhash.Hash, version uint32, deploymentID string) (int64, error) {
	return c.stateLastChangedHeight, c.stateLastChangedHeightErr
}

// TicketPoolValue returns a mocked current value of all the locked funds in the
// ticket pool.
func (c *testRPCChain) TicketPoolValue() (dcrutil.Amount, error) {
	return c.ticketPoolValue, c.ticketPoolValueErr
}

// TicketsWithAddress returns a mocked slice of ticket hashes that are currently
// live corresponding to the given address.
func (c *testRPCChain) TicketsWithAddress(address dcrutil.Address) ([]chainhash.Hash, error) {
	return c.ticketsWithAddress, nil
}

// TipGeneration returns a mocked slice of the entire generation of blocks
// stemming from the parent of the current tip.
func (c *testRPCChain) TipGeneration() ([]chainhash.Hash, error) {
	return c.tipGeneration, nil
}

// testPeer provides a mock peer by implementing the Peer interface.
type testPeer struct {
	addr              string
	connected         bool
	id                int32
	inbound           bool
	localAddr         net.Addr
	lastPingNonce     uint64
	isTxRelayDisabled bool
	banScore          uint32
	statsSnapshot     *peer.StatsSnap
}

// Addr returns a mocked peer address.
func (p *testPeer) Addr() string {
	return p.addr
}

// Connected returns a mocked bool representing whether or not the peer is
// currently connected.
func (p *testPeer) Connected() bool {
	return p.connected
}

// ID returns a mocked peer id.
func (p *testPeer) ID() int32 {
	return p.id
}

// Inbound returns a mocked bool representing whether the peer is inbound.
func (p *testPeer) Inbound() bool {
	return p.inbound
}

// StatsSnapshot returns a mocked snapshot of the current peer flags and
// statistics.
func (p *testPeer) StatsSnapshot() *peer.StatsSnap {
	return p.statsSnapshot
}

// LocalAddr returns a mocked local address of the connection.
func (p *testPeer) LocalAddr() net.Addr {
	return p.localAddr
}

// LastPingNonce returns a mocked last ping nonce of the remote peer.
func (p *testPeer) LastPingNonce() uint64 {
	return p.lastPingNonce
}

// IsTxRelayDisabled returns a mocked bool representing whether or not the peer
// has disabled transaction relay.
func (p *testPeer) IsTxRelayDisabled() bool {
	return p.isTxRelayDisabled
}

// BanScore returns a mocked current integer value that represents how close
// the peer is to being banned.
func (p *testPeer) BanScore() uint32 {
	return p.banScore
}

// testAddrManager provides a mock address manager by implementing the
// AddrManager interface.
type testAddrManager struct {
	localAddresses []addrmgr.LocalAddr
}

// LocalAddresses returns a mocked summary of local addresses information
// for the getnetworkinfo rpc.
func (c *testAddrManager) LocalAddresses() []addrmgr.LocalAddr {
	return c.localAddresses
}

// testSyncManager provides a mock sync manager by implementing the
// SyncManager interface.
type testSyncManager struct {
	isCurrent          bool
	isOrphan           bool
	submitBlockErr     error
	syncPeerID         int32
	locateBlocks       []chainhash.Hash
	tipGeneration      []chainhash.Hash
	syncHeight         int64
	processTransaction []*dcrutil.Tx
}

// IsCurrent returns a mocked bool representing whether or not the sync manager
// believes the chain is current as compared to the rest of the network.
func (s *testSyncManager) IsCurrent() bool {
	return s.isCurrent
}

// SubmitBlock provides a mock implementation for submitting the provided block
// to the network after processing it locally.
func (s *testSyncManager) SubmitBlock(block *dcrutil.Block, flags blockchain.BehaviorFlags) (bool, error) {
	return s.isOrphan, s.submitBlockErr
}

// SyncPeer returns a mocked id of the current peer being synced with.
func (s *testSyncManager) SyncPeerID() int32 {
	return s.syncPeerID
}

// LocateBlocks returns a mocked slice of hashes of the blocks after the first
// known block in the locator until the provided stop hash is reached, or up to
// the provided max number of block hashes.
func (s *testSyncManager) LocateBlocks(locator blockchain.BlockLocator, hashStop *chainhash.Hash, maxHashes uint32) []chainhash.Hash {
	return s.locateBlocks
}

// TipGeneration returns a mocked entire generation of blocks stemming from the
// parent of the current tip.
func (s *testSyncManager) TipGeneration() ([]chainhash.Hash, error) {
	return s.tipGeneration, nil
}

// SyncHeight returns a mocked latest known block being synced to.
func (s *testSyncManager) SyncHeight() int64 {
	return s.syncHeight
}

// ProcessTransaction provides a mock implementation for relaying the provided
// transaction validation and insertion into the memory pool.
func (s *testSyncManager) ProcessTransaction(tx *dcrutil.Tx, allowOrphans bool,
	rateLimit bool, allowHighFees bool, tag mempool.Tag) ([]*dcrutil.Tx, error) {
	return s.processTransaction, nil
}

// testExistsAddresser provides a mock exists addresser by implementing the
// ExistsAddresser interface.
type testExistsAddresser struct {
	existsAddress      bool
	existsAddressErr   error
	existsAddresses    []bool
	existsAddressesErr error
}

// ExistsAddress returns a mocked bool representing whether or not an address
// has been seen before.
func (e *testExistsAddresser) ExistsAddress(addr dcrutil.Address) (bool, error) {
	return e.existsAddress, e.existsAddressErr
}

// ExistsAddresses returns a mocked bool slice representing whether or not each
// address in a slice of addresses has been seen before.
func (e *testExistsAddresser) ExistsAddresses(addrs []dcrutil.Address) ([]bool, error) {
	return e.existsAddresses, e.existsAddressesErr
}

// testConnManager provides a mock connection manager by implementing the
// ConnManager interface.
type testConnManager struct {
	connectErr          error
	removeByIDErr       error
	removeByAddrErr     error
	disconnectByIDErr   error
	disconnectByAddrErr error
	connectedCount      int32
	netTotalReceived    uint64
	netTotalSent        uint64
	connectedPeers      []Peer
	persistentPeers     []Peer
	addedNodeInfo       []Peer
	lookup              func(host string) ([]net.IP, error)
}

// Connect provides a mock implementation for adding the provided address as a
// new outbound peer.
func (c *testConnManager) Connect(addr string, permanent bool) error {
	return c.connectErr
}

// RemoveByID provides a mock implementation for removing the peer associated
// with the provided id from the list of persistent peers.
func (c *testConnManager) RemoveByID(id int32) error {
	return c.removeByIDErr
}

// RemoveByAddr provides a mock implementation for removing the peer associated
// with the provided address from the list of persistent peers.
func (c *testConnManager) RemoveByAddr(addr string) error {
	return c.removeByAddrErr
}

// DisconnectByID provides a mock implementation for disconnecting the peer
// associated with the provided id.
func (c *testConnManager) DisconnectByID(id int32) error {
	return c.disconnectByIDErr
}

// DisconnectByAddr provides a mock implementation for disconnecting the peer
// associated with the provided address.
func (c *testConnManager) DisconnectByAddr(addr string) error {
	return c.disconnectByAddrErr
}

// ConnectedCount returns a mocked number of currently connected peers.
func (c *testConnManager) ConnectedCount() int32 {
	return c.connectedCount
}

// NetTotals returns a mocked sum of all bytes received and sent across the
// network for all peers.
func (c *testConnManager) NetTotals() (uint64, uint64) {
	return c.netTotalReceived, c.netTotalSent
}

// ConnectedPeers returns a mocked slice of all connected peers.
func (c *testConnManager) ConnectedPeers() []Peer {
	return c.connectedPeers
}

// PersistentPeers returns a mocked slice of all persistent peers.
func (c *testConnManager) PersistentPeers() []Peer {
	return c.persistentPeers
}

// BroadcastMessage provides a mock implementation for sending the provided
// message to all currently connected peers.
func (c *testConnManager) BroadcastMessage(msg wire.Message) {}

// AddRebroadcastInventory provides a mock implementation for adding the
// provided inventory to the list of inventories to be rebroadcast at random
// intervals until they show up in a block.
func (c *testConnManager) AddRebroadcastInventory(iv *wire.InvVect, data interface{}) {}

// RelayTransactions provides a mock implementation for generating and relaying
// inventory vectors for all of the passed transactions to all connected peers.
func (c *testConnManager) RelayTransactions(txns []*dcrutil.Tx) {}

// AddedNodeInfo returns a mocked slice of persistent (added) peers.
func (c *testConnManager) AddedNodeInfo() []Peer {
	return c.addedNodeInfo
}

// Lookup defines a mocked DNS lookup function to be used.
func (c *testConnManager) Lookup(host string) ([]net.IP, error) {
	return c.lookup(host)
}

// testCPUMiner provides a mock CPU miner by implementing the CPUMiner
// interface.
type testCPUMiner struct {
	generatedBlocks    []*chainhash.Hash
	generateNBlocksErr error
	isMining           bool
	hashesPerSecond    float64
	workers            int32
}

// GenerateNBlocks returns a mock implementatation of generating a requested
// number of blocks.
func (c *testCPUMiner) GenerateNBlocks(ctx context.Context, n uint32) ([]*chainhash.Hash, error) {
	return c.generatedBlocks, c.generateNBlocksErr
}

// IsMining returns a mocked mining state of the CPU miner.
func (c *testCPUMiner) IsMining() bool {
	return c.isMining
}

// HashesPerSecond returns a mocked number of hashes per second the CPU miner
// is performing.
func (c *testCPUMiner) HashesPerSecond() float64 {
	return c.hashesPerSecond
}

// HashesPerSecond returns a mocked number of CPU miner workers solving blocks.
func (c *testCPUMiner) NumWorkers() int32 {
	return c.workers
}

// SetNumWorkers sets a mocked number of CPU miner workers.
func (c *testCPUMiner) SetNumWorkers(numWorkers int32) {
	c.workers = numWorkers
}

// testAddr implements the net.Addr interface.
type testAddr struct {
	net, addr string
}

// String returns the address.
func (a testAddr) String() string {
	return a.addr
}

// Network returns the network.
func (a testAddr) Network() string {
	return a.net
}

// testClock provides a mock clock by implementing the Clock interface.
type testClock struct {
	now   time.Time
	since time.Duration
}

// Now returns a mocked time.Time representing the current local time.
func (c *testClock) Now() time.Time {
	return c.now
}

// Since returns a mocked time.Duration representing the time elapsed since t.
func (c *testClock) Since(t time.Time) time.Duration {
	return c.since
}

// testFeeEstimator provides a mock fee estimator by implementing the
// FeeEstimator interface.
type testFeeEstimator struct {
	estimateFeeAmt dcrutil.Amount
	estimateFeeErr error
}

// EstimateFee provides a mock implementation that calculates the
// suggested fee for a transaction.
func (e *testFeeEstimator) EstimateFee(targetConfs int32) (dcrutil.Amount, error) {
	return e.estimateFeeAmt, e.estimateFeeErr
}

// testLogManager provides a mock log manager by implementing the LogManager
// interface.
type testLogManager struct {
	supportedSubsystems       []string
	parseAndSetDebugLevelsErr error
}

// SupportedSubsystems returns a mocked slice of supported subsystems.
func (l *testLogManager) SupportedSubsystems() []string {
	return l.supportedSubsystems
}

// ParseAndSetDebugLevels provides a mock implementation for parsing the
// specified debug level and setting the levels accordingly.
func (l *testLogManager) ParseAndSetDebugLevels(debugLevel string) error {
	return l.parseAndSetDebugLevelsErr
}

// testSanityChecker provides a mock implementation that checks the sanity
// state of a block.
type testSanityChecker struct {
	checkBlockSanityErr error
}

// testSanityChecker returns mock sanity state of the provided block.
func (s *testSanityChecker) CheckBlockSanity(block *dcrutil.Block) error {
	return s.checkBlockSanityErr
}

// testFilterer provides a mock filterer by implementing the Filterer interface.
type testFilterer struct {
	filterByBlockHash          []byte
	filterByBlockHashErr       error
	filterHeaderByBlockHash    []byte
	filterHeaderByBlockHashErr error
}

// FilterByBlockHash returns a mocked regular or extended committed filter for
// the given block hash.
func (f *testFilterer) FilterByBlockHash(h *chainhash.Hash, filterType wire.FilterType) ([]byte, error) {
	return f.filterByBlockHash, f.filterByBlockHashErr
}

// FilterHeaderByBlockHash returns a mocked regular or extended committed filter
// header for the given block hash.
func (f *testFilterer) FilterHeaderByBlockHash(h *chainhash.Hash, filterType wire.FilterType) ([]byte, error) {
	return f.filterHeaderByBlockHash, f.filterHeaderByBlockHashErr
}

// testFiltererV2 provides a mock V2 filterer by implementing the FiltererV2
// interface.
type testFiltererV2 struct {
	filterByBlockHash    *gcs.FilterV2
	filterByBlockHashErr error
}

// FilterByBlockHash returns a mocked version 2 GCS filter for the given block
// hash.
func (f *testFiltererV2) FilterByBlockHash(hash *chainhash.Hash) (*gcs.FilterV2, error) {
	return f.filterByBlockHash, f.filterByBlockHashErr
}

// testMiningState provides a mock mining state.
type testMiningState struct {
	allowUnsyncedMining bool
	miningAddrs         []dcrutil.Address
	workState           *workState
}

// testTemplateSubber provides an implementation of a TemplateSubber for use
// with tests.
type testTemplateSubber struct {
	templater *testBlockTemplater
	c         chan *mining.TemplateNtfn
}

// C returns a channel that produces a stream of block templates as each new
// template is generated.
func (s *testTemplateSubber) C() <-chan *mining.TemplateNtfn {
	return s.c
}

// Stop prevents any future template updates from being delivered and
// unsubscribes the associated subscription.
func (s *testTemplateSubber) Stop() {
	delete(s.templater.subscriptions, s)
}

// PublishTemplateNtfn sends the provided template notification on the channel
// associated with the subscription.
func (s *testTemplateSubber) PublishTemplateNtfn(templateNtfn *mining.TemplateNtfn) {
	s.c <- templateNtfn
}

// testBlockTemplater provides a mock block templater by implementing the
// mining.BlockTemplater interface.
type testBlockTemplater struct {
	subscriptions      map[*testTemplateSubber]struct{}
	regenReason        mining.TemplateUpdateReason
	currTemplate       *mining.BlockTemplate
	currTemplateErr    error
	updateBlockTimeErr error
	simulateNewNtfn    bool
}

// ForceRegen asks the block templater to generate a new template immediately.
func (b *testBlockTemplater) ForceRegen() {}

// Subscribe subscribes a client for block template updates.  The returned
// template subscription contains functions to retrieve a channel that produces
// the stream of block templates and to stop the stream when the caller no
// longer wishes to receive new templates.
func (b *testBlockTemplater) Subscribe() TemplateSubber {
	sub := &testTemplateSubber{
		templater: b,
		c:         make(chan *mining.TemplateNtfn),
	}
	go func() {
		ntfn := &mining.TemplateNtfn{
			Template: b.currTemplate,
			Reason:   b.regenReason,
		}
		sub.PublishTemplateNtfn(ntfn)

		if b.simulateNewNtfn {
			sub.PublishTemplateNtfn(ntfn)
		}
	}()
	b.subscriptions[sub] = struct{}{}
	return sub
}

// CurrentTemplate returns the current template associated with the block
// templater along with any associated error.
func (b *testBlockTemplater) CurrentTemplate() (*mining.BlockTemplate, error) {
	return b.currTemplate, b.currTemplateErr
}

// UpdateBlockTime updates the timestamp in the passed header to the current
// time while taking into account the consensus rules.
func (b *testBlockTemplater) UpdateBlockTime(header *wire.BlockHeader) error {
	return b.updateBlockTimeErr
}

// testTxMempooler provides a mock mempool transaction data source by
// implementing the TxMempooler interface.
type testTxMempooler struct {
	haveTransactions    []bool
	txDescs             []*mempool.TxDesc
	verboseTxDescs      []*mempool.VerboseTxDesc
	count               int
	fetchTransaction    *dcrutil.Tx
	fetchTransactionErr error
}

// HaveTransactions returns a mocked bool slice representing whether or not the
// passed transactions already exist.
func (mp *testTxMempooler) HaveTransactions(hashes []*chainhash.Hash) []bool {
	return mp.haveTransactions
}

// TxDescs returns a mock slice of descriptors for all the transactions in
// the pool.
func (mp *testTxMempooler) TxDescs() []*mempool.TxDesc {
	return mp.txDescs
}

// VerboseTxDescs returns a mock slice of verbose descriptors for all the
// transactions in the pool.
func (mp *testTxMempooler) VerboseTxDescs() []*mempool.VerboseTxDesc {
	return mp.verboseTxDescs
}

// Count returns a mock number of transactions in the main pool.
func (mp *testTxMempooler) Count() int {
	return mp.count
}

// FetchTransaction returns the mocked requested transaction from the
// transaction pool.
func (mp *testTxMempooler) FetchTransaction(txHash *chainhash.Hash) (*dcrutil.Tx, error) {
	return mp.fetchTransaction, mp.fetchTransactionErr
}

// mustParseHash converts the passed big-endian hex string into a
// chainhash.Hash and will panic if there is an error.  It only differs from the
// one available in chainhash in that it will panic so errors in the source code
// be detected.  It will only (and must only) be called with hard-coded, and
// therefore known good, hashes.
func mustParseHash(s string) *chainhash.Hash {
	hash, err := chainhash.NewHashFromStr(s)
	if err != nil {
		panic("invalid hash in source file: " + s)
	}
	return hash
}

// hexToBytes converts the passed hex string into bytes and will panic if there
// is an error.  This is only provided for the hard-coded constants so errors in
// the source code can be detected. It will only (and must only) be called with
// hard-coded values.
func hexToBytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	return b
}

// cloneParams returns a deep copy of the provided parameters so the caller is
// free to modify them without worrying about interfering with other tests.
func cloneParams(params *chaincfg.Params) *chaincfg.Params {
	// Encode via gob.
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	enc.Encode(params)

	// Decode via gob to make a deep copy.
	var paramsCopy chaincfg.Params
	dec := gob.NewDecoder(buf)
	dec.Decode(&paramsCopy)
	return &paramsCopy
}

// block432100 mocks block 432,100 of the block chain.  It is loaded and
// deserialized immediately here and then can be used throughout the tests.
var block432100 = func() wire.MsgBlock {
	// Load and deserialize the test block.
	blockDataFile := filepath.Join(testDataPath, "block432100.bz2")
	fi, err := os.Open(blockDataFile)
	if err != nil {
		panic(err)
	}
	defer fi.Close()
	var block wire.MsgBlock
	err = block.Deserialize(bzip2.NewReader(fi))
	if err != nil {
		panic(err)
	}
	return block
}()

type rpcTest struct {
	name                  string
	handler               commandHandler
	cmd                   interface{}
	mockChainParams       *chaincfg.Params
	mockChain             *testRPCChain
	mockMiningState       *testMiningState
	mockCPUMiner          *testCPUMiner
	mockBlockTemplater    *testBlockTemplater
	setBlockTemplaterNil  bool
	mockSanityChecker     *testSanityChecker
	mockAddrManager       *testAddrManager
	mockFeeEstimator      *testFeeEstimator
	mockSyncManager       *testSyncManager
	mockExistsAddresser   *testExistsAddresser
	setExistsAddresserNil bool
	mockConnManager       *testConnManager
	mockClock             *testClock
	mockLogManager        *testLogManager
	mockFilterer          *testFilterer
	mockFiltererV2        *testFiltererV2
	mockTxMempooler       *testTxMempooler
	result                interface{}
	wantErr               bool
	errCode               dcrjson.RPCErrorCode
}

// defaultChainParams provides a default chaincfg.Params to be used throughout
// the tests.  It should be cloned using cloneParams, updated as necessary, and
// then assigned to rpcTest.mockChainParams if it needs to be overridden by a
// test.
var defaultChainParams = func() *chaincfg.Params {
	testChainParams := cloneParams(chaincfg.MainNetParams())
	testChainParams.Name = "mainnet"
	testChainParams.Deployments = map[uint32][]chaincfg.ConsensusDeployment{
		7: {{
			Vote: chaincfg.Vote{
				Id:          chaincfg.VoteIDHeaderCommitments,
				Description: "Enable header commitments as defined in DCP0005",
				Mask:        0x0006, // Bits 1 and 2
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsAbstain:   true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "keep the existing consensus rules",
					Bits:        0x0002, // Bit 1
					IsAbstain:   false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "change to the new consensus rules",
					Bits:        0x0004, // Bit 2
					IsAbstain:   false,
					IsNo:        false,
				}},
			},
			StartTime:  1567641600, // Sep 5th, 2019
			ExpireTime: 1599264000, // Sep 5th, 2020
		}},
	}
	testChainParams.PowLimitBits = 0x1d00ffff
	testChainParams.StakeDiffWindowSize = 144
	testChainParams.GenesisHash = *mustParseHash("298e5cc3d985bfe7f81dc135f360ab" +
		"e089edd4396b86d2de66b0cef42b21d980")
	return testChainParams
}()

// defaultMockRPCChain provides a default mock chain to be used throughout
// the tests.  Tests can override these defaults by calling defaultMockRPCChain,
// updating fields as necessary on the returned *testRPCChain, and then setting
// rpcTest.mockChain as that *testRPCChain.
func defaultMockRPCChain() *testRPCChain {
	// Define variables related to block432100 to be used as default values for the
	// mock chain.
	blk := dcrutil.NewBlock(&block432100)
	blkHeader := block432100.Header
	blkHash := blk.Hash()
	blkHeight := blk.Height()
	chainWork, _ := new(big.Int).SetString("0e805fb85284503581c57c", 16)
	return &testRPCChain{
		bestSnapshot: &blockchain.BestState{
			Hash:           *blkHash,
			PrevHash:       blkHeader.PrevBlock,
			Height:         blkHeight,
			Bits:           blkHeader.Bits,
			NextPoolSize:   41135,
			NextStakeDiff:  14428162590,
			BlockSize:      uint64(blkHeader.Size),
			NumTxns:        7,
			TotalTxns:      7478697,
			MedianTime:     time.Unix(1584246683, 0), // 2020-03-15 04:31:23 UTC
			TotalSubsidy:   1122503888072909,
			NextFinalState: [6]byte{0xdc, 0x2a, 0x4f, 0x6e, 0x60, 0xb3},
		},
		blockByHash:                     blk,
		blockByHeight:                   blk,
		blockHashByHeight:               blkHash,
		blockHeightByHash:               blkHeight,
		calcNextRequiredStakeDifficulty: 14428162590,
		calcWantHeight:                  431487,
		chainTips: []blockchain.ChainTipInfo{{
			Height:    blkHeight,
			Hash:      *blkHash,
			BranchLen: 500,
			Status:    "active",
		}},
		chainWork: chainWork,
		convertUtxosToMinimalOutputs: []*stake.MinimalOutput{{
			PkScript: hexToBytes("baa914780239ea1231ba67b0c5b82e786b51e21072522187"),
			Value:    100000000,
			Version:  0,
		}, {
			PkScript: hexToBytes("6a1e355c96f48612d57509140e9a049981d5f9970f945c770d00000000000058"),
			Value:    0,
			Version:  0,
		}, {
			PkScript: hexToBytes("bd76a914000000000000000000000000000000000000000088ac"),
			Value:    0,
			Version:  0,
		}},
		estimateNextStakeDifficultyFn: func(int64, bool) (int64, error) {
			return 14336790201, nil
		},
		fetchUtxoEntry: &testRPCUtxoEntry{
			hasExpiry: true,
			height:    100000,
			txType:    stake.TxTypeSStx,
			txVersion: 1,
		},
		fetchUtxoStats: &blockchain.UtxoStats{
			Utxos:          1593879,
			Transactions:   689819,
			Size:           36441617,
			Total:          1154067750680149,
			SerializedHash: *mustParseHash("fe7b32aa188800f07268b17f3bead5f3d8a1b6d18654182066436efce6effa86"),
		},
		getStakeVersions: []blockchain.StakeVersions{{
			Hash:         *blkHash,
			Height:       blkHeight,
			BlockVersion: blkHeader.Version,
			StakeVersion: blkHeader.StakeVersion,
			Votes: []stake.VoteVersionTuple{{
				Version: 7,
				Bits:    1,
			}},
		}},
		getVoteInfo: &blockchain.VoteInfo{
			Agendas: defaultChainParams.Deployments[0],
			AgendaStatus: []blockchain.ThresholdStateTuple{{
				State:  blockchain.ThresholdStarted,
				Choice: uint32(0xffffffff),
			}},
		},
		headerByHash:      blkHeader,
		headerByHeight:    blkHeader,
		isCurrent:         true,
		mainChainHasBlock: true,
		maxBlockSize:      int64(393216),
		nextThresholdState: blockchain.ThresholdStateTuple{
			State:  blockchain.ThresholdStarted,
			Choice: uint32(0xffffffff),
		},
		ticketPoolValue: 570678298669222,
	}
}

// defaultMockSanityChecker provides a default mock sanity checker to be used
// throughout the tests. Tests can override these defaults by calling
// defaultMockSanityChecker, updating fields as necessary on the returned
// *testSanityChecker, and then setting rpcTest.mockSanityChecker as that
// *testSanityChecker.
func defaultMockSanityChecker() *testSanityChecker {
	return &testSanityChecker{}
}

// defaultMockMiningState provides a default mock mining state to be used
// throughout the tests. Tests can override these defaults by calling
// defaultMockMiningState, updating fields as necessary on the returned
// *testMiningState, and then setting rpcTest.mockMiningState as that
// *testMiningState.
func defaultMockMiningState() *testMiningState {
	blk := block432100
	tmplKey := getWorkTemplateKey(&block432100.Header)
	workState := newWorkState()
	workState.templatePool[tmplKey] = &block432100
	workState.prevHash = &blk.Header.PrevBlock
	return &testMiningState{
		workState: workState,
	}
}

// defaultMockBlockTemplater provides a default mock block templater to be used
// throughout the tests. Tests can override these defaults by calling
// defaultMockBlockTemplaterr, updating fields as necessary on the returned
// *testBlockTemplater, and then setting rpcTest.mockBlockTemplater as that
// *testBlockTemplater.
func defaultMockBlockTemplater() *testBlockTemplater {
	return &testBlockTemplater{
		subscriptions: make(map[*testTemplateSubber]struct{}),
		regenReason:   mining.TURNewParent,
		currTemplate:  &mining.BlockTemplate{Block: &block432100},
	}
}

// defaultMockAddrManager provides a default mock address manager to be used
// throughout the tests. Tests can override these defaults by calling
// defaultMockAddrManager, updating fields as necessary on the returned
// *testAddrManager, and then setting rpcTest.mockAddrManager as that
// *testAddrManager.
func defaultMockAddrManager() *testAddrManager {
	return &testAddrManager{
		localAddresses: []addrmgr.LocalAddr{{
			Address: "127.0.0.184",
			Port:    uint16(19108),
			Score:   int32(0),
		}},
	}
}

// defaultMockExistsAddresser provides a default mock exists addresser to be
// used throughout the tests. Tests can override these defaults by calling
// defaultMockExistsAddresser, updating fields as necessary on the returned
// *testExistsAddresser, and then setting rpcTest.mockExistsAddresser as that
// *testExistsAddresser.
func defaultMockExistsAddresser() *testExistsAddresser {
	return &testExistsAddresser{}
}

// defaultMockSyncManager provides a default mock sync manager to be used
// throughout the tests. Tests can override these defaults by calling
// defaultMockSyncManager, updating fields as necessary on the returned
// *testSyncManager, and then setting rpcTest.mockSyncManager as that
// *testSyncManager.
func defaultMockSyncManager() *testSyncManager {
	return &testSyncManager{
		isOrphan:   false,
		syncHeight: 463074,
	}
}

// defaultMockConnManager provides a default mock connection manager to be used
// throughout the tests. Tests can override these defaults by calling
// defaultMockConnManager, updating fields as necessary on the returned
// *testConnManager, and then setting rpcTest.mockConnManager as that
// *testConnManager.
func defaultMockConnManager() *testConnManager {
	testPeer1 := &testPeer{
		addr:      "127.0.0.210:9108",
		connected: true,
		inbound:   true,
		id:        28,
	}
	testPeer2 := &testPeer{
		addr:      "127.0.0.211:9108",
		connected: true,
		inbound:   false,
		id:        29,
	}
	testPeer3 := &testPeer{
		addr:      "mydomain.org:9108",
		connected: true,
		inbound:   false,
		id:        30,
	}
	testPeer4 := &testPeer{
		addr:      "nonexistentdomain.org:9108",
		connected: true,
		inbound:   false,
		id:        31,
	}
	return &testConnManager{
		connectedCount:   4,
		netTotalReceived: 9598159,
		netTotalSent:     4783802,
		connectedPeers: []Peer{
			testPeer1,
			testPeer2,
			testPeer3,
			testPeer4,
		},
		persistentPeers: []Peer{
			testPeer1,
			testPeer2,
			testPeer3,
			testPeer4,
		},
		addedNodeInfo: []Peer{
			testPeer1,
			testPeer2,
			testPeer3,
			testPeer4,
		},
		lookup: func(host string) ([]net.IP, error) {
			if host == "mydomain.org" {
				return []net.IP{net.ParseIP("127.0.0.211")}, nil
			}
			return nil, errors.New("host not found")
		},
	}
}

// defaultMockFeeEstimator provides a default mock fee estimator to be used
// throughout the tests. Tests can override these defaults by calling
// defaultMockFeeEstimator, updating fields as necessary on the returned
// *testFeeEstimator, and then setting rpcTest.mockFeeEstimator as that
// *testFeeEstimator.
func defaultMockFeeEstimator() *testFeeEstimator {
	return &testFeeEstimator{}
}

// defaultMockLogManager provides a default mock log manager to be used
// throughout the tests. Tests can override these defaults by calling
// defaultMockLogManager, updating fields as necessary on the returned
// *testLogManager, and then setting rpcTest.mockLogManager as that
// *testLogManager.
func defaultMockLogManager() *testLogManager {
	return &testLogManager{
		supportedSubsystems: []string{"DCRD", "PEER", "RPCS"},
	}
}

// defaultMockFiltererV2 provides a default mock V2 filterer to be used
// throughout the tests. Tests can override these defaults by calling
// defaultMockFiltererV2, updating fields as necessary on the returned
// *testFiltererV2, and then setting rpcTest.mockFiltererV2 as that
// *testFiltererV2.
func defaultMockFiltererV2() *testFiltererV2 {
	block432100Filter := hexToBytes("11cdaad289eb092b5fd6ad60c7f7f197c2234dcbc74" +
		"b14e1a477b319eae9d189cfae45f06a225965c7e932fc7600")
	filter, _ := gcs.FromBytesV2(blockcf2.B, blockcf2.M, block432100Filter)
	return &testFiltererV2{
		filterByBlockHash: filter,
	}
}

// defaultMockCPUMiner provides a default mock CPU miner to be used
// throughout the tests. Tests can override these defaults by calling
// defaultMockCPUMiner, updating fields as necessary on the returned
// *testCPUMiner, and then setting rpcTest.mockCPUMiner as that
// *testCPUMiner.
func defaultMockCPUMiner() *testCPUMiner {
	return &testCPUMiner{}
}

// defaultMockTxMempooler provides a default mock mempool transaction source
// to be used throughout the tests. Tests can override these defaults by
// calling defaultMockTxMempooler, updating fields as necessary on the returned
// *testTxMempooler, and then setting rpcTest.mockTxMempooler as that
// *testTxMempooler.
func defaultMockTxMempooler() *testTxMempooler {
	return &testTxMempooler{}
}

// defaultMockConfig provides a default Config that is used throughout
// the tests.  Defaults can be overridden by tests through the rpcTest struct.
func defaultMockConfig(chainParams *chaincfg.Params) *Config {
	return &Config{
		ChainParams:     chainParams,
		Chain:           defaultMockRPCChain(),
		SanityChecker:   defaultMockSanityChecker(),
		BlockTemplater:  defaultMockBlockTemplater(),
		AddrManager:     defaultMockAddrManager(),
		FeeEstimator:    defaultMockFeeEstimator(),
		SyncMgr:         defaultMockSyncManager(),
		ExistsAddresser: defaultMockExistsAddresser(),
		ConnMgr:         defaultMockConnManager(),
		CPUMiner:        defaultMockCPUMiner(),
		TxMempooler:     defaultMockTxMempooler(),
		Clock:           &testClock{},
		LogManager:      defaultMockLogManager(),
		FiltererV2:      defaultMockFiltererV2(),
		TimeSource:      blockchain.NewMedianTime(),
		Services:        wire.SFNodeNetwork | wire.SFNodeCF,
		SubsidyCache:    standalone.NewSubsidyCache(chainParams),
		NetInfo: []types.NetworksResult{{
			Name:                      "IPV4",
			Limited:                   false,
			Reachable:                 true,
			Proxy:                     "",
			ProxyRandomizeCredentials: false,
		}, {
			Name:                      "IPV6",
			Limited:                   false,
			Reachable:                 true,
			Proxy:                     "",
			ProxyRandomizeCredentials: false,
		}, {
			Name:                      "Onion",
			Limited:                   false,
			Reachable:                 false,
			Proxy:                     "",
			ProxyRandomizeCredentials: false,
		}},
		MinRelayTxFee:      dcrutil.Amount(10000),
		MaxProtocolVersion: wire.CFilterV2Version,
		UserAgentVersion: fmt.Sprintf("%d.%d.%d", version.Major, version.Minor,
			version.Patch),
	}
}

func TestHandleAddNode(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleAddNode: ok",
		handler: handleAddNode,
		cmd: &types.AddNodeCmd{
			Addr:   "127.0.0.210:9108",
			SubCmd: "add",
		},
		result: nil,
	}, {
		name:    "handleAddNode: 'add' subcommand error",
		handler: handleAddNode,
		cmd: &types.AddNodeCmd{
			Addr:   "127.0.0.210:9108",
			SubCmd: "add",
		},
		mockConnManager: func() *testConnManager {
			connManager := defaultMockConnManager()
			connManager.connectErr = errors.New("peer already connected")
			return connManager
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleAddNode: 'remove' subcommand error",
		handler: handleAddNode,
		cmd: &types.AddNodeCmd{
			Addr:   "127.0.0.210:9108",
			SubCmd: "remove",
		},
		mockConnManager: func() *testConnManager {
			connManager := defaultMockConnManager()
			connManager.removeByAddrErr = errors.New("peer not found")
			return connManager
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleAddNode: 'onetry' subcommand error",
		handler: handleAddNode,
		cmd: &types.AddNodeCmd{
			Addr:   "127.0.0.210:9108",
			SubCmd: "onetry",
		},
		mockConnManager: func() *testConnManager {
			connManager := defaultMockConnManager()
			connManager.connectErr = errors.New("peer already connected")
			return connManager
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleAddNode: invalid subcommand",
		handler: handleAddNode,
		cmd: &types.AddNodeCmd{
			Addr:   "127.0.0.210:9108",
			SubCmd: "",
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}})
}

func TestHandleCreateRawSStx(t *testing.T) {
	t.Parallel()

	defaultCmdInputs := []types.SStxInput{{
		Txid: "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
		Vout: 0,
		Tree: 0,
		Amt:  100000000,
	}}
	defaultCmdAmount := map[string]int64{"DcuQKx8BES9wU7C6Q5VmLBjw436r27hayjS": 100000000}
	defaultCmdCOuts := []types.SStxCommitOut{{
		Addr:       "DsRah84zx6jdA4nMYboMfLERA5V3KhBr4ru",
		CommitAmt:  100000000,
		ChangeAddr: "DsfkbtrSUr5cFdQYq3WSKo9vvFs5qxZXbgF",
		ChangeAmt:  0,
	}}
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleCreateRawSStx: ok",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: defaultCmdInputs,
			Amount: defaultCmdAmount,
			COuts:  defaultCmdCOuts,
		},
		result: "01000000010d33d3840e9074183dc9a8d82a5031075a98135bfe182840ddaf575" +
			"aa2032fe00000000000ffffffff0300e1f50500000000000018baa914f0b4e851" +
			"00aee1a996f22915eb3c3f764d53779a8700000000000000000000206a1e06c4a" +
			"66cc56478aeaa01744ab8ba0d8cc47110a400e1f5050000000000000000000000" +
			"00000000001abd76a914a23634e90541542fe2ac2a79e6064333a09b558188ac0" +
			"0000000000000000100e1f5050000000000000000ffffffff00",
	}, {
		name:    "handleCreateRawSStx: num inputs != num outputs",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: []types.SStxInput{{
				Txid: "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout: 0,
				Tree: 0,
				Amt:  90000000,
			}, {
				Txid: "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout: 0,
				Tree: 0,
				Amt:  10000000,
			}},
			Amount: defaultCmdAmount,
			COuts:  defaultCmdCOuts,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawSStx: more than one amount specified",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: defaultCmdInputs,
			Amount: map[string]int64{
				"DcuQKx8BES9wU7C6Q5VmLBjw436r27hayjS": 90000000,
				"DcqgK4N4Ccucu2Sq4VDAdu4wH4LASLhzLVp": 10000000,
			},
			COuts: []types.SStxCommitOut{{
				Addr:       "DsRah84zx6jdA4nMYboMfLERA5V3KhBr4ru",
				CommitAmt:  90000000,
				ChangeAddr: "DsfkbtrSUr5cFdQYq3WSKo9vvFs5qxZXbgF",
				ChangeAmt:  10000000,
			}},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawSStx: txid invalid hex",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: []types.SStxInput{{
				Txid: "g02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout: 0,
				Tree: 0,
				Amt:  100000000,
			}},
			Amount: defaultCmdAmount,
			COuts:  defaultCmdCOuts,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}, {
		name:    "handleCreateRawSStx: invalid tx tree",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: []types.SStxInput{{
				Txid: "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout: 0,
				Tree: -1,
				Amt:  100000000,
			}},
			Amount: defaultCmdAmount,
			COuts:  defaultCmdCOuts,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawSStx: invalid amount > dcrutil.MaxAmount",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: defaultCmdInputs,
			Amount: map[string]int64{
				"DcuQKx8BES9wU7C6Q5VmLBjw436r27hayjS": dcrutil.MaxAmount + 1,
			},
			COuts: defaultCmdCOuts,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawSStx: invalid amount < 0",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: defaultCmdInputs,
			Amount: map[string]int64{"DcuQKx8BES9wU7C6Q5VmLBjw436r27hayjS": -1},
			COuts:  defaultCmdCOuts,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawSStx: invalid address",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: defaultCmdInputs,
			Amount: map[string]int64{"DcuqInvalidwU7C6Q5VmLBjw436r27hayjS": 100000000},
			COuts:  defaultCmdCOuts,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidAddressOrKey,
	}, {
		name:    "handleCreateRawSStx: invalid address type",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: defaultCmdInputs,
			Amount: map[string]int64{
				"DkM3EyZ546GghVSkvzb6J47PvGDyntqiDtFgipQhNj78Xm2mUYRpf": 100000000,
			},
			COuts: defaultCmdCOuts,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidAddressOrKey,
	}, {
		name:    "handleCreateRawSStx: unsupported dsa",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: defaultCmdInputs,
			Amount: map[string]int64{"DSXcZv4oSRiEoWL2a9aD8sgfptRo1YEXNKj": 100000000},
			COuts:  defaultCmdCOuts,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleCreateRawSStx: change amount greater than input amount",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: defaultCmdInputs,
			Amount: defaultCmdAmount,
			COuts: []types.SStxCommitOut{{
				Addr:       "DsRah84zx6jdA4nMYboMfLERA5V3KhBr4ru",
				CommitAmt:  100000000,
				ChangeAddr: "DsfkbtrSUr5cFdQYq3WSKo9vvFs5qxZXbgF",
				ChangeAmt:  200000000,
			}},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawSStx: invalid output address",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: defaultCmdInputs,
			Amount: defaultCmdAmount,
			COuts: []types.SStxCommitOut{{
				Addr:       "DsRaInvalidjdA4nMYboMfLERA5V3KhBr4ru",
				CommitAmt:  100000000,
				ChangeAddr: "DsfkbtrSUr5cFdQYq3WSKo9vvFs5qxZXbgF",
				ChangeAmt:  0,
			}},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidAddressOrKey,
	}, {
		name:    "handleCreateRawSStx: invalid output address type",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: defaultCmdInputs,
			Amount: defaultCmdAmount,
			COuts: []types.SStxCommitOut{{
				Addr:       "DkM3EyZ546GghVSkvzb6J47PvGDyntqiDtFgipQhNj78Xm2mUYRpf",
				CommitAmt:  100000000,
				ChangeAddr: "DsfkbtrSUr5cFdQYq3WSKo9vvFs5qxZXbgF",
				ChangeAmt:  0,
			}},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidAddressOrKey,
	}, {
		name:    "handleCreateRawSStx: unsupported output address dsa",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: defaultCmdInputs,
			Amount: defaultCmdAmount,
			COuts: []types.SStxCommitOut{{
				Addr:       "DSXcZv4oSRiEoWL2a9aD8sgfptRo1YEXNKj",
				CommitAmt:  100000000,
				ChangeAddr: "DsfkbtrSUr5cFdQYq3WSKo9vvFs5qxZXbgF",
				ChangeAmt:  0,
			}},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleCreateRawSStx: invalid change amount > dcrutil.MaxAmount",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: []types.SStxInput{{
				Txid: "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout: 0,
				Tree: 0,
				Amt:  dcrutil.MaxAmount + 2,
			}},
			Amount: defaultCmdAmount,
			COuts: []types.SStxCommitOut{{
				Addr:       "DsRah84zx6jdA4nMYboMfLERA5V3KhBr4ru",
				CommitAmt:  100000000,
				ChangeAddr: "DsfkbtrSUr5cFdQYq3WSKo9vvFs5qxZXbgF",
				ChangeAmt:  dcrutil.MaxAmount + 1,
			}},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawSStx: invalid change amount < 0",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: defaultCmdInputs,
			Amount: defaultCmdAmount,
			COuts: []types.SStxCommitOut{{
				Addr:       "DsRah84zx6jdA4nMYboMfLERA5V3KhBr4ru",
				CommitAmt:  100000000,
				ChangeAddr: "DsfkbtrSUr5cFdQYq3WSKo9vvFs5qxZXbgF",
				ChangeAmt:  -1,
			}},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawSStx: invalid change address",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: defaultCmdInputs,
			Amount: defaultCmdAmount,
			COuts: []types.SStxCommitOut{{
				Addr:       "DsRah84zx6jdA4nMYboMfLERA5V3KhBr4ru",
				CommitAmt:  100000000,
				ChangeAddr: "DsfkInvalidcFdQYq3WSKo9vvFs5qxZXbgF",
				ChangeAmt:  0,
			}},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidAddressOrKey,
	}, {
		name:    "handleCreateRawSStx: invalid change address type",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: defaultCmdInputs,
			Amount: defaultCmdAmount,
			COuts: []types.SStxCommitOut{{
				Addr:       "DsRah84zx6jdA4nMYboMfLERA5V3KhBr4ru",
				CommitAmt:  100000000,
				ChangeAddr: "DkM3EyZ546GghVSkvzb6J47PvGDyntqiDtFgipQhNj78Xm2mUYRpf",
				ChangeAmt:  0,
			}},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidAddressOrKey,
	}, {
		name:    "handleCreateRawSStx: unsupported change address dsa",
		handler: handleCreateRawSStx,
		cmd: &types.CreateRawSStxCmd{
			Inputs: defaultCmdInputs,
			Amount: defaultCmdAmount,
			COuts: []types.SStxCommitOut{{
				Addr:       "DsRah84zx6jdA4nMYboMfLERA5V3KhBr4ru",
				CommitAmt:  100000000,
				ChangeAddr: "DSXcZv4oSRiEoWL2a9aD8sgfptRo1YEXNKj",
				ChangeAmt:  0,
			}},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}})
}

func TestHandleCreateRawSSRtx(t *testing.T) {
	t.Parallel()

	defaultCmdInputs := []types.TransactionInput{{
		Amount: 100,
		Txid:   "1189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
		Vout:   0,
		Tree:   1,
	}}
	defaultFee := dcrjson.Float64(1)
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleCreateRawSSRtx: ok",
		handler: handleCreateRawSSRtx,
		cmd: &types.CreateRawSSRtxCmd{
			Inputs: defaultCmdInputs,
			Fee:    defaultFee,
		},
		result: "0100000001395ebc9af44c4a696fa8e6287bdbf0a89a4d6207f191cb0f1eefc25" +
			"6e6cb89110000000001ffffffff0100e1f5050000000000001abc76a914355c96" +
			"f48612d57509140e9a049981d5f9970f9488ac00000000000000000100e40b540" +
			"200000000000000ffffffff00",
	}, {
		name:    "handleCreateRawSSRtx: ok P2SH",
		handler: handleCreateRawSSRtx,
		cmd: &types.CreateRawSSRtxCmd{
			Inputs: defaultCmdInputs,
			Fee:    defaultFee,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.convertUtxosToMinimalOutputs = []*stake.MinimalOutput{{
				PkScript: []byte{
					0xBA, 0xA9, 0x14, 0x78, 0x02, 0x39, 0xEA, 0x12,
					0x31, 0xBA, 0x67, 0xB0, 0xC5, 0xB8, 0x2E, 0x78,
					0x6B, 0x51, 0xE2, 0x10, 0x72, 0x52, 0x21, 0x87,
				},
				Value:   100000000,
				Version: 0,
			}, {
				PkScript: []byte{
					0x6A, 0x1E, 0x35, 0x5C, 0x96, 0xF4, 0x86, 0x12,
					0xD5, 0x75, 0x09, 0x14, 0x0E, 0x9A, 0x04, 0x99,
					0x81, 0xD5, 0xF9, 0x97, 0x0F, 0x94,
					0x5C, 0x77, 0x0D, 0x00, 0x00, 0x00, 0x00, 0x80, // commitamt (set MSB for P2SH)
					0x00, 0x58,
				},
				Value:   0,
				Version: 0,
			}, {
				PkScript: []byte{
					0xBD, 0x76, 0xA9, 0x14, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x88, 0xAC,
				},
				Value:   0,
				Version: 0,
			}}
			return chain
		}(),
		result: "0100000001395ebc9af44c4a696fa8e6287bdbf0a89a4d6207f191cb0f1eefc25" +
			"6e6cb89110000000001ffffffff0100e1f50500000000000018bca914355c96f4" +
			"8612d57509140e9a049981d5f9970f948700000000000000000100e40b5402000" +
			"00000000000ffffffff00",
	}, {
		name:    "handleCreateRawSSRtx: invalid number of inputs",
		handler: handleCreateRawSSRtx,
		cmd: &types.CreateRawSSRtxCmd{
			Inputs: []types.TransactionInput{},
			Fee:    defaultFee,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawSSRtx: invalid fee amount",
		handler: handleCreateRawSSRtx,
		cmd: &types.CreateRawSSRtxCmd{
			Inputs: defaultCmdInputs,
			Fee:    dcrjson.Float64(math.Inf(1)),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawSSRtx: txid invalid hex",
		handler: handleCreateRawSSRtx,
		cmd: &types.CreateRawSSRtxCmd{
			Inputs: []types.TransactionInput{{
				Amount: 100,
				Txid:   "g189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
				Vout:   0,
				Tree:   1,
			}},
			Fee: defaultFee,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}, {
		name:    "handleCreateRawSSRtx: no tx info",
		handler: handleCreateRawSSRtx,
		cmd: &types.CreateRawSSRtxCmd{
			Inputs: defaultCmdInputs,
			Fee:    defaultFee,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.fetchUtxoEntry = nil
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCNoTxInfo,
	}, {
		name:    "handleCreateRawSSRtx: invalid tx type",
		handler: handleCreateRawSSRtx,
		cmd: &types.CreateRawSSRtxCmd{
			Inputs: defaultCmdInputs,
			Fee:    defaultFee,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.fetchUtxoEntry = &testRPCUtxoEntry{
				txType:     stake.TxTypeRegular,
				height:     100000,
				index:      0,
				txVersion:  1,
				isCoinBase: false,
				hasExpiry:  true,
				modified:   false,
			}
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCDeserialization,
	}, {
		name:    "handleCreateRawSSRtx: input tree wrong type",
		handler: handleCreateRawSSRtx,
		cmd: &types.CreateRawSSRtxCmd{
			Inputs: []types.TransactionInput{{
				Amount: 100,
				Txid:   "1189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
				Vout:   0,
				Tree:   0,
			}},
			Fee: defaultFee,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawSSRtx: invalid input amount",
		handler: handleCreateRawSSRtx,
		cmd: &types.CreateRawSSRtxCmd{
			Inputs: []types.TransactionInput{{
				Amount: math.Inf(1),
				Txid:   "1189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
				Vout:   0,
				Tree:   1,
			}},
			Fee: defaultFee,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawSSRtx: invalid sstx amount",
		handler: handleCreateRawSSRtx,
		cmd: &types.CreateRawSSRtxCmd{
			Inputs: defaultCmdInputs,
			Fee:    defaultFee,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.convertUtxosToMinimalOutputs = []*stake.MinimalOutput{{
				PkScript: []byte{
					0xBA, 0xA9, 0x14, 0x78, 0x02, 0x39, 0xEA, 0x12,
					0x31, 0xBA, 0x67, 0xB0, 0xC5, 0xB8, 0x2E, 0x78,
					0x6B, 0x51, 0xE2, 0x10, 0x72, 0x52, 0x21, 0x87,
				},
				Value:   100000000,
				Version: 0,
			}, {
				PkScript: []byte{
					0x6A, 0x1E, 0x35, 0x5C, 0x96, 0xF4, 0x86, 0x12,
					0xD5, 0x75, 0x09, 0x14, 0x0E, 0x9A, 0x04, 0x99,
					0x81, 0xD5, 0xF9, 0x97, 0x0F, 0x94,
					0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, // commitamt (invalid amount)
					0x00, 0x58,
				},
				Value:   0,
				Version: 0,
			}, {
				PkScript: []byte{
					0xBD, 0x76, 0xA9, 0x14, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x88, 0xAC,
				},
				Value:   0,
				Version: 0,
			}}
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}})
}

func TestHandleCreateRawTransaction(t *testing.T) {
	t.Parallel()

	defaultCmdInputs := []types.TransactionInput{{
		Amount: 1,
		Txid:   "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
		Vout:   0,
		Tree:   0,
	}}
	defaultCmdAmounts := map[string]float64{"DcurAwesomeAddressmqDctW5wJCW1Cn2MF": 1}
	defaultCmdLockTime := dcrjson.Int64(1)
	defaultCmdExpiry := dcrjson.Int64(1)
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleCreateRawTransaction: ok",
		handler: handleCreateRawTransaction,
		cmd: &types.CreateRawTransactionCmd{
			Inputs:   defaultCmdInputs,
			Amounts:  defaultCmdAmounts,
			LockTime: defaultCmdLockTime,
			Expiry:   defaultCmdExpiry,
		},
		result: "01000000010d33d3840e9074183dc9a8d82a5031075a98135bfe182840ddaf575" +
			"aa2032fe00000000000feffffff0100e1f50500000000000017a914f59833f104" +
			"faa3c7fd0c7dc1e3967fe77a9c15238701000000010000000100e1f5050000000" +
			"000000000ffffffff00",
	}, {
		name:    "handleCreateRawTransaction: expiry out of range",
		handler: handleCreateRawTransaction,
		cmd: &types.CreateRawTransactionCmd{
			Inputs:   defaultCmdInputs,
			Amounts:  defaultCmdAmounts,
			LockTime: defaultCmdLockTime,
			Expiry:   dcrjson.Int64(-1),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawTransaction: locktime out of range",
		handler: handleCreateRawTransaction,
		cmd: &types.CreateRawTransactionCmd{
			Inputs:   defaultCmdInputs,
			Amounts:  defaultCmdAmounts,
			LockTime: dcrjson.Int64(-1),
			Expiry:   defaultCmdExpiry,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawTransaction: txid invalid hex",
		handler: handleCreateRawTransaction,
		cmd: &types.CreateRawTransactionCmd{
			Inputs: []types.TransactionInput{{
				Amount: 1,
				Txid:   "g02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout:   0,
				Tree:   0,
			}},
			Amounts:  defaultCmdAmounts,
			LockTime: defaultCmdLockTime,
			Expiry:   defaultCmdExpiry,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}, {
		name:    "handleCreateRawTransaction: invalid tree",
		handler: handleCreateRawTransaction,
		cmd: &types.CreateRawTransactionCmd{
			Inputs: []types.TransactionInput{{
				Amount: 1,
				Txid:   "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout:   0,
				Tree:   2,
			}},
			Amounts:  defaultCmdAmounts,
			LockTime: defaultCmdLockTime,
			Expiry:   defaultCmdExpiry,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawTransaction: output over max amount",
		handler: handleCreateRawTransaction,
		cmd: &types.CreateRawTransactionCmd{
			Inputs: []types.TransactionInput{{
				Amount: (dcrutil.MaxAmount + 1) / 1e8,
				Txid:   "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout:   0,
				Tree:   0,
			}},
			Amounts: map[string]float64{
				"DcurAwesomeAddressmqDctW5wJCW1Cn2MF": (dcrutil.MaxAmount + 1) / 1e8,
			},
			LockTime: defaultCmdLockTime,
			Expiry:   defaultCmdExpiry,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleCreateRawTransaction: address wrong network",
		handler: handleCreateRawTransaction,
		cmd: &types.CreateRawTransactionCmd{
			Inputs:   defaultCmdInputs,
			Amounts:  map[string]float64{"Tsf5Qvq2m7X5KzTZDdSGfa6WrMtikYVRkaL": 1},
			LockTime: defaultCmdLockTime,
			Expiry:   defaultCmdExpiry,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidAddressOrKey,
	}, {
		name:    "handleCreateRawTransaction: address wrong type",
		handler: handleCreateRawTransaction,
		cmd: &types.CreateRawTransactionCmd{
			Inputs: defaultCmdInputs,
			Amounts: map[string]float64{
				"DkRMCQhwDFTRwW6umM59KEJiMvTPke9X7akJJfbzKocNPDqZMAUEq": 1,
			},
			LockTime: defaultCmdLockTime,
			Expiry:   defaultCmdExpiry,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidAddressOrKey,
	}})
}

func TestHandleDebugLevel(t *testing.T) {
	t.Parallel()

	logMgr := defaultMockLogManager()
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleDebugLevel: show",
		handler: handleDebugLevel,
		cmd: &types.DebugLevelCmd{
			LevelSpec: "show",
		},
		result: fmt.Sprintf("Supported subsystems %v", logMgr.supportedSubsystems),
	}, {
		name:    "handleDebugLevel: invalidDebugLevel",
		handler: handleDebugLevel,
		cmd: &types.DebugLevelCmd{
			LevelSpec: "invalidDebugLevel",
		},
		mockLogManager: func() *testLogManager {
			logManager := defaultMockLogManager()
			logManager.parseAndSetDebugLevelsErr = errors.New("invalidDebugLevel")
			return logManager
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleDebugLevel: trace",
		handler: handleDebugLevel,
		cmd: &types.DebugLevelCmd{
			LevelSpec: "trace",
		},
		result: "Done.",
	}})
}

func TestHandleDecodeRawTransaction(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleDecodeRawTransaction: ok",
		handler: handleDecodeRawTransaction,
		cmd: &types.DecodeRawTransactionCmd{
			HexTx: "01000000010d33d3840e9074183dc9a8d82a5031075a98135bfe182840ddaf575a" +
				"a2032fe00000000000feffffff0100e1f50500000000000017a914f59833f104fa" +
				"a3c7fd0c7dc1e3967fe77a9c15238701000000010000000100e1f5050000000000" +
				"000000ffffffff00",
		},
		result: types.TxRawDecodeResult{
			Txid:     "f8e1d2fea09a3ff89c54ddbf4c0f333503afb470fc6bfaa981b8cf5a98165749",
			Version:  1,
			Locktime: 1,
			Expiry:   1,
			Vin: []types.Vin{{
				Txid:        "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout:        0,
				Tree:        0,
				Sequence:    4294967294,
				AmountIn:    1,
				BlockHeight: 0,
				BlockIndex:  4294967295,
				ScriptSig: &types.ScriptSig{
					Asm: "",
					Hex: "",
				},
			}},
			Vout: []types.Vout{{
				Value:   1,
				N:       0,
				Version: 0,
				ScriptPubKey: types.ScriptPubKeyResult{
					Asm:     "OP_HASH160 f59833f104faa3c7fd0c7dc1e3967fe77a9c1523 OP_EQUAL",
					Hex:     "a914f59833f104faa3c7fd0c7dc1e3967fe77a9c152387",
					ReqSigs: 1,
					Type:    "scripthash",
					Addresses: []string{
						"DcurAwesomeAddressmqDctW5wJCW1Cn2MF",
					},
				},
			}},
		},
	}, {
		name:    "handleDecodeRawTransaction: ok with odd length hex",
		handler: handleDecodeRawTransaction,
		cmd: &types.DecodeRawTransactionCmd{
			HexTx: "1000000010d33d3840e9074183dc9a8d82a5031075a98135bfe182840ddaf575aa" +
				"2032fe00000000000feffffff0100e1f50500000000000017a914f59833f104faa" +
				"3c7fd0c7dc1e3967fe77a9c15238701000000010000000100e1f50500000000000" +
				"00000ffffffff00",
		},
		result: types.TxRawDecodeResult{
			Txid:     "f8e1d2fea09a3ff89c54ddbf4c0f333503afb470fc6bfaa981b8cf5a98165749",
			Version:  1,
			Locktime: 1,
			Expiry:   1,
			Vin: []types.Vin{{
				Txid:        "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout:        0,
				Tree:        0,
				Sequence:    4294967294,
				AmountIn:    1,
				BlockHeight: 0,
				BlockIndex:  4294967295,
				ScriptSig: &types.ScriptSig{
					Asm: "",
					Hex: "",
				},
			}},
			Vout: []types.Vout{{
				Value:   1,
				N:       0,
				Version: 0,
				ScriptPubKey: types.ScriptPubKeyResult{
					Asm:     "OP_HASH160 f59833f104faa3c7fd0c7dc1e3967fe77a9c1523 OP_EQUAL",
					Hex:     "a914f59833f104faa3c7fd0c7dc1e3967fe77a9c152387",
					ReqSigs: 1,
					Type:    "scripthash",
					Addresses: []string{
						"DcurAwesomeAddressmqDctW5wJCW1Cn2MF",
					},
				},
			}},
		},
	}, {
		name:    "handleDecodeRawTransaction: invalid hex",
		handler: handleDecodeRawTransaction,
		cmd: &types.DecodeRawTransactionCmd{
			HexTx: "g1000000010d33d3840e9074183dc9a8d82a5031075a98135bfe182840ddaf575a" +
				"a2032fe00000000000feffffff0100e1f50500000000000017a914f59833f104fa" +
				"a3c7fd0c7dc1e3967fe77a9c15238701000000010000000100e1f5050000000000" +
				"000000ffffffff00",
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}, {
		name:    "handleDecodeRawTransaction: deserialization error",
		handler: handleDecodeRawTransaction,
		cmd: &types.DecodeRawTransactionCmd{
			HexTx: "fefefefefefe3d3840e9074183dc9a8d82a5031075a98135bfe182840ddaf575aa" +
				"2032fe00000000000feffffff0100e1f50500000000000017a914f59833f104faa" +
				"3c7fd0c7dc1e3967fe77a9c15238701000000010000000100e1f50500000000000" +
				"00000ffffffff00",
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDeserialization,
	}})
}

func TestHandleDecodeScript(t *testing.T) {
	t.Parallel()

	// This is a pay to stake submission script hash script.
	p2sstxsh := "ba76a914000000000000000000000000000000000000000088ac"
	p2sstxshRes := types.DecodeScriptResult{
		Asm: "OP_SSTX OP_DUP OP_HASH160 0000000000000000000000000000" +
			"000000000000 OP_EQUALVERIFY OP_CHECKSIG",
		ReqSigs:   1,
		Type:      "stakesubmission",
		Addresses: []string{"DsQxuVRvS4eaJ42dhQEsCXauMWjvopWgrVg"},
		P2sh:      "DcaBW1ecMLBzXSS9Q8YRV3aBc5qQeaA1WPo",
	}
	aHex := "0A"
	aHexRes := types.DecodeScriptResult{
		Asm:       "[error]",
		ReqSigs:   0,
		Type:      "nonstandard",
		Addresses: []string{},
		P2sh:      "DcbuYCoW1nJZhFf1ZyGXjoPL6D3ezNwwWjj",
	}
	// This is a 2 of 2 multisig script.
	multiSig := "5221030000000000000000000000000000000000000000000000000" +
		"00000000000000121030000000000000000000000000000000000000000" +
		"00000000000000000000000252ae"
	multiSigRes := types.DecodeScriptResult{
		Asm: "2 0300000000000000000000000000000000000000000000000000" +
			"00000000000001 030000000000000000000000000000000000" +
			"000000000000000000000000000002 2 OP_CHECKMULTISIG",
		ReqSigs: 2,
		Type:    "multisig",
		Addresses: []string{"DsdvMfW6wGbGCXSNWidWtfP1tPmnCLNXQyC",
			"DsSkAQDPhDW3foES4fcfmpkPYYZhnV3R4ws"},
		P2sh: "DcexHKLpqiM49auD2jbxPH6enwm9u1ZFAo6",
	}
	// This is a pay to script hash script.
	p2sh := "a914000000000000000000000000000000000000000087"
	p2shRes := types.DecodeScriptResult{
		Asm: "OP_HASH160 0000000000000000000000000000000000000000 " +
			"OP_EQUAL",
		ReqSigs:   1,
		Type:      "scripthash",
		Addresses: []string{"DcXTb4QtmnyRsnzUVViYQawqFE5PuYTdX2C"},
	}
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleDecodeScript: ok no version",
		handler: handleDecodeScript,
		cmd: &types.DecodeScriptCmd{
			HexScript: p2sstxsh,
		},
		result: p2sstxshRes,
	}, {
		name:    "handleDecodeScript: ok version 0",
		handler: handleDecodeScript,
		cmd: &types.DecodeScriptCmd{
			HexScript: p2sstxsh,
			Version:   dcrjson.Uint16(0),
		},
		result: p2sstxshRes,
	}, {
		name:    "handleDecodeScript: ok asm error",
		handler: handleDecodeScript,
		cmd: &types.DecodeScriptCmd{
			HexScript: aHex,
		},
		result: aHexRes,
	}, {
		name:    "handleDecodeScript: ok incomplete hex",
		handler: handleDecodeScript,
		cmd: &types.DecodeScriptCmd{
			HexScript: aHex[1:],
		},
		result: aHexRes,
	}, {
		name:    "handleDecodeScript: ok multiple addresses",
		handler: handleDecodeScript,
		cmd: &types.DecodeScriptCmd{
			HexScript: multiSig,
		},
		result: multiSigRes,
	}, {
		name:    "handleDecodeScript: ok no p2sh in return",
		handler: handleDecodeScript,
		cmd: &types.DecodeScriptCmd{
			HexScript: p2sh,
		},
		result: p2shRes,
	}, {
		name:    "handleDecodeScript: invalid hex",
		handler: handleDecodeScript,
		cmd: &types.DecodeScriptCmd{
			HexScript: "Q",
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}})
}

func TestHandleEstimateFee(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleEstimateFee: ok",
		handler: handleEstimateFee,
		cmd:     &types.EstimateFeeCmd{},
		result:  float64(0.0001),
	}})
}

func TestHandleEstimateSmartFee(t *testing.T) {
	t.Parallel()

	conservative := types.EstimateSmartFeeConservative
	economical := types.EstimateSmartFeeEconomical
	validFeeEstimator := defaultMockFeeEstimator()
	validFeeEstimator.estimateFeeAmt = 123456789
	validFee := float64(1.23456789)
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleEstimateSmartFee: ok with mode",
		handler: handleEstimateSmartFee,
		cmd: &types.EstimateSmartFeeCmd{
			Confirmations: 0,
			Mode:          &conservative,
		},
		mockFeeEstimator: validFeeEstimator,
		result:           validFee,
	}, {
		name:             "handleEstimateSmartFee: ok no mode",
		handler:          handleEstimateSmartFee,
		cmd:              &types.EstimateSmartFeeCmd{},
		mockFeeEstimator: validFeeEstimator,
		result:           validFee,
	}, {
		name:    "handleEstimateSmartFee: not conservative mode",
		handler: handleEstimateSmartFee,
		cmd: &types.EstimateSmartFeeCmd{
			Mode: &economical,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleEstimateSmartFee: estimate fee error",
		handler: handleEstimateSmartFee,
		cmd:     &types.EstimateSmartFeeCmd{},
		mockFeeEstimator: func() *testFeeEstimator {
			feeEstimator := defaultMockFeeEstimator()
			feeEstimator.estimateFeeErr = errors.New("")
			return feeEstimator
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}})
}

func TestHandleEstimateStakeDiff(t *testing.T) {
	t.Parallel()

	type stakeDiffQueueItem struct {
		diff int64
		err  error
	}
	var low, med, high, user int64 = 123456, 1234567, 12345678, 7345468745783
	validQueueFn := func() []*stakeDiffQueueItem {
		lowQItem := stakeDiffQueueItem{diff: low}
		medQItem := stakeDiffQueueItem{diff: med}
		highQItem := stakeDiffQueueItem{diff: high}
		userQItem := stakeDiffQueueItem{diff: user}
		return []*stakeDiffQueueItem{
			&lowQItem,
			&highQItem,
			&medQItem,
			&userQItem,
		}
	}
	estimateFn := func(queue []*stakeDiffQueueItem) func(int64, bool) (int64, error) {
		return func(int64, bool) (int64, error) {
			defer func() { queue = queue[1:] }()
			return queue[0].diff, queue[0].err
		}
	}
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleEstimateStakeDiff: ok with Tickets arg",
		handler: handleEstimateStakeDiff,
		cmd: &types.EstimateStakeDiffCmd{
			Tickets: dcrjson.Uint32(1),
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.estimateNextStakeDifficultyFn = estimateFn(validQueueFn())
			return chain
		}(),
		result: &types.EstimateStakeDiffResult{
			Min:      dcrutil.Amount(low).ToCoin(),
			Max:      dcrutil.Amount(high).ToCoin(),
			Expected: dcrutil.Amount(med).ToCoin(),
			User:     dcrjson.Float64(dcrutil.Amount(user).ToCoin()),
		},
	}, {
		name:    "handleEstimateStakeDiff: ok no Tickets arg",
		handler: handleEstimateStakeDiff,
		cmd:     &types.EstimateStakeDiffCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.estimateNextStakeDifficultyFn = estimateFn(validQueueFn())
			return chain
		}(),
		result: &types.EstimateStakeDiffResult{
			Min:      dcrutil.Amount(low).ToCoin(),
			Max:      dcrutil.Amount(high).ToCoin(),
			Expected: dcrutil.Amount(med).ToCoin(),
		},
	}, {
		name:    "handleEstimateStakeDiff: HeaderByHeight error",
		handler: handleEstimateStakeDiff,
		cmd:     &types.EstimateStakeDiffCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.estimateNextStakeDifficultyFn = estimateFn(validQueueFn())
			chain.headerByHeightErr = errors.New("")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleEstimateStakeDiff: min diff estimation error",
		handler: handleEstimateStakeDiff,
		cmd:     &types.EstimateStakeDiffCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			queue := validQueueFn()
			queue[0].err = errors.New("")
			chain.estimateNextStakeDifficultyFn = estimateFn(queue)
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleEstimateStakeDiff: high diff estimation error",
		handler: handleEstimateStakeDiff,
		cmd:     &types.EstimateStakeDiffCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			queue := validQueueFn()
			queue[1].err = errors.New("")
			chain.estimateNextStakeDifficultyFn = estimateFn(queue)
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleEstimateStakeDiff: expected diff estimation error",
		handler: handleEstimateStakeDiff,
		cmd:     &types.EstimateStakeDiffCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			queue := validQueueFn()
			queue[2].err = errors.New("")
			chain.estimateNextStakeDifficultyFn = estimateFn(queue)
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleEstimateStakeDiff: user diff estimation error",
		handler: handleEstimateStakeDiff,
		cmd: &types.EstimateStakeDiffCmd{
			Tickets: dcrjson.Uint32(1),
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			queue := validQueueFn()
			queue[3].err = errors.New("")
			chain.estimateNextStakeDifficultyFn = estimateFn(queue)
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}})
}

func TestHandleExistsAddress(t *testing.T) {
	t.Parallel()

	validAddr := "DcurAwesomeAddressmqDctW5wJCW1Cn2MF"
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleExistsAddress: ok",
		handler: handleExistsAddress,
		cmd: &types.ExistsAddressCmd{
			Address: validAddr,
		},
		result: false,
	}, {
		name:    "handleExistsAddress: exist address indexing not enabled",
		handler: handleExistsAddress,
		cmd: &types.ExistsAddressCmd{
			Address: validAddr,
		},
		setExistsAddresserNil: true,
		wantErr:               true,
		errCode:               dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleExistsAddress: bad address",
		handler: handleExistsAddress,
		cmd: &types.ExistsAddressCmd{
			Address: "bad address",
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidAddressOrKey,
	}, {
		name:    "handleExistsAddress: ExistsAddress error",
		handler: handleExistsAddress,
		cmd: &types.ExistsAddressCmd{
			Address: validAddr,
		},
		mockExistsAddresser: func() *testExistsAddresser {
			existsAddrIndexer := defaultMockExistsAddresser()
			existsAddrIndexer.existsAddressErr = errors.New("")
			return existsAddrIndexer
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}})
}

func TestHandleExistsAddresses(t *testing.T) {
	t.Parallel()

	validAddr := "DcurAwesomeAddressmqDctW5wJCW1Cn2MF"
	validAddrs := []string{validAddr, validAddr, validAddr}
	existsSlice := []bool{false, true, true}
	// existsSlice as a bitset is 110₂ which is 6₁₆ in hex.
	existsStr := "06"
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleExistsAddresses: ok",
		handler: handleExistsAddresses,
		cmd: &types.ExistsAddressesCmd{
			Addresses: validAddrs,
		},
		mockExistsAddresser: func() *testExistsAddresser {
			existsAddrIndexer := defaultMockExistsAddresser()
			existsAddrIndexer.existsAddresses = existsSlice
			return existsAddrIndexer
		}(),
		result: existsStr,
	}, {
		name:    "handleExistsAddresses: exist address indexing not enabled",
		handler: handleExistsAddresses,
		cmd: &types.ExistsAddressesCmd{
			Addresses: validAddrs,
		},
		setExistsAddresserNil: true,
		wantErr:               true,
		errCode:               dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleExistsAddresses: bad address",
		handler: handleExistsAddresses,
		cmd: &types.ExistsAddressesCmd{
			Addresses: append(validAddrs, "bad address"),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidAddressOrKey,
	}, {
		name:    "handleExistsAddresses: ExistsAddresses error",
		handler: handleExistsAddresses,
		cmd: &types.ExistsAddressesCmd{
			Addresses: validAddrs,
		},
		mockExistsAddresser: func() *testExistsAddresser {
			existsAddrIndexer := defaultMockExistsAddresser()
			existsAddrIndexer.existsAddressesErr = errors.New("")
			return existsAddrIndexer
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}})
}

func TestHandleExistsExpiredTickets(t *testing.T) {
	t.Parallel()

	defaultCmdTxHashes := []string{
		"1189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
		"2189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
	}
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleExistsExpiredTickets: both tickets exist",
		handler: handleExistsExpiredTickets,
		cmd: &types.ExistsExpiredTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkExpiredTickets = []bool{true, true}
			return chain
		}(),
		result: "03",
	}, {
		name:    "handleExistsExpiredTickets: only first ticket exists",
		handler: handleExistsExpiredTickets,
		cmd: &types.ExistsExpiredTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkExpiredTickets = []bool{true, false}
			return chain
		}(),
		result: "01",
	}, {
		name:    "handleExistsExpiredTickets: only second ticket exists",
		handler: handleExistsExpiredTickets,
		cmd: &types.ExistsExpiredTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkExpiredTickets = []bool{false, true}
			return chain
		}(),
		result: "02",
	}, {
		name:    "handleExistsExpiredTickets: none of the tickets exist",
		handler: handleExistsExpiredTickets,
		cmd: &types.ExistsExpiredTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkExpiredTickets = []bool{false, false}
			return chain
		}(),
		result: "00",
	}, {
		name:    "handleExistsExpiredTickets: invalid hash",
		handler: handleExistsExpiredTickets,
		cmd: &types.ExistsExpiredTicketsCmd{
			TxHashes: []string{
				"g189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
			},
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkExpiredTickets = []bool{true, true}
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}, {
		name:    "handleExistsExpiredTickets: invalid missed ticket count",
		handler: handleExistsExpiredTickets,
		cmd: &types.ExistsExpiredTicketsCmd{
			TxHashes: []string{
				"1189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
			},
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkExpiredTickets = []bool{true, true}
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}})
}

func TestHandleExistsLiveTicket(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleExistsLiveTicket: ticket exists",
		handler: handleExistsLiveTicket,
		cmd: &types.ExistsLiveTicketCmd{
			TxHash: "1189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkLiveTicket = true
			return chain
		}(),
		result: true,
	}, {
		name:    "handleExistsLiveTicket: ticket does not exist",
		handler: handleExistsLiveTicket,
		cmd: &types.ExistsLiveTicketCmd{
			TxHash: "1189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
		},
		result: false,
	}, {
		name:    "handleExistsLiveTicket: invalid hash",
		handler: handleExistsLiveTicket,
		cmd: &types.ExistsLiveTicketCmd{
			TxHash: "g189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}})
}

func TestHandleExistsLiveTickets(t *testing.T) {
	t.Parallel()

	defaultCmdTxHashes := []string{
		"1189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
		"2189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
	}
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleExistsLiveTickets: both tickets exist",
		handler: handleExistsLiveTickets,
		cmd: &types.ExistsLiveTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkLiveTickets = []bool{true, true}
			return chain
		}(),
		result: "03",
	}, {
		name:    "handleExistsLiveTickets: only first ticket exists",
		handler: handleExistsLiveTickets,
		cmd: &types.ExistsLiveTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkLiveTickets = []bool{true, false}
			return chain
		}(),
		result: "01",
	}, {
		name:    "handleExistsLiveTickets: only second ticket exists",
		handler: handleExistsLiveTickets,
		cmd: &types.ExistsLiveTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkLiveTickets = []bool{false, true}
			return chain
		}(),
		result: "02",
	}, {
		name:    "handleExistsLiveTickets: none of the tickets exist",
		handler: handleExistsLiveTickets,
		cmd: &types.ExistsLiveTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkLiveTickets = []bool{false, false}
			return chain
		}(),
		result: "00",
	}, {
		name:    "handleExistsLiveTickets: invalid hash",
		handler: handleExistsLiveTickets,
		cmd: &types.ExistsLiveTicketsCmd{
			TxHashes: []string{
				"g189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
			},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}, {
		name:    "handleExistsLiveTickets: invalid missed ticket count",
		handler: handleExistsLiveTickets,
		cmd: &types.ExistsLiveTicketsCmd{
			TxHashes: []string{
				"1189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
			},
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkLiveTickets = []bool{true, true}
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}})
}

func TestHandleExistsMempoolTxs(t *testing.T) {
	t.Parallel()

	defaultCmdTxHashes := []string{
		"1189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
		"2189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
	}
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleExistsMempoolTxs: ok",
		handler: handleExistsMempoolTxs,
		cmd: &types.ExistsMempoolTxsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockTxMempooler: func() *testTxMempooler {
			mp := defaultMockTxMempooler()
			mp.haveTransactions = []bool{false, true}
			return mp
		}(),
		result: "02",
	}, {
		name:    "handleExistsMempoolTxs: invalid hash",
		handler: handleExistsMempoolTxs,
		cmd: &types.ExistsMempoolTxsCmd{
			TxHashes: []string{
				"g189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
			},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}, {
		name:    "handleExistsMempoolTxs: have transaction count different than number of args",
		handler: handleExistsMempoolTxs,
		cmd: &types.ExistsMempoolTxsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockTxMempooler: func() *testTxMempooler {
			mp := defaultMockTxMempooler()
			mp.haveTransactions = []bool{false}
			return mp
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}})
}

func TestHandleExistsMissedTickets(t *testing.T) {
	t.Parallel()

	defaultCmdTxHashes := []string{
		"1189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
		"2189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
	}
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleExistsMissedTickets: both tickets exist",
		handler: handleExistsMissedTickets,
		cmd: &types.ExistsMissedTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkMissedTickets = []bool{true, true}
			return chain
		}(),
		result: "03",
	}, {
		name:    "handleExistsMissedTickets: only first ticket exists",
		handler: handleExistsMissedTickets,
		cmd: &types.ExistsMissedTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkMissedTickets = []bool{true, false}
			return chain
		}(),
		result: "01",
	}, {
		name:    "handleExistsMissedTickets: only second ticket exists",
		handler: handleExistsMissedTickets,
		cmd: &types.ExistsMissedTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkMissedTickets = []bool{false, true}
			return chain
		}(),
		result: "02",
	}, {
		name:    "handleExistsMissedTickets: none of the tickets exist",
		handler: handleExistsMissedTickets,
		cmd: &types.ExistsMissedTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkMissedTickets = []bool{false, false}
			return chain
		}(),
		result: "00",
	}, {
		name:    "handleExistsMissedTickets: invalid hash",
		handler: handleExistsMissedTickets,
		cmd: &types.ExistsMissedTicketsCmd{
			TxHashes: []string{
				"g189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
			},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}, {
		name:    "handleExistsMissedTickets: invalid missed ticket count",
		handler: handleExistsMissedTickets,
		cmd: &types.ExistsMissedTicketsCmd{
			TxHashes: []string{
				"1189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
			},
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.checkMissedTickets = []bool{true, true}
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}})
}

func TestHandleGetAddedNodeInfo(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetAddedNodeInfo: ok without DNS and without address filter",
		handler: handleGetAddedNodeInfo,
		cmd: &types.GetAddedNodeInfoCmd{
			DNS: false,
		},
		result: []string{"127.0.0.210:9108", "127.0.0.211:9108",
			"mydomain.org:9108", "nonexistentdomain.org:9108"},
	}, {
		name:    "handleGetAddedNodeInfo: found without DNS and with address filter",
		handler: handleGetAddedNodeInfo,
		cmd: &types.GetAddedNodeInfoCmd{
			DNS:  false,
			Node: dcrjson.String("127.0.0.211:9108"),
		},
		result: []string{"127.0.0.211:9108"},
	}, {
		name:    "handleGetAddedNodeInfo: node not found",
		handler: handleGetAddedNodeInfo,
		cmd: &types.GetAddedNodeInfoCmd{
			DNS:  false,
			Node: dcrjson.String("127.0.0.212:9108"),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleGetAddedNodeInfo: ok with DNS and without address filter",
		handler: handleGetAddedNodeInfo,
		cmd: &types.GetAddedNodeInfoCmd{
			DNS: true,
		},
		result: []*types.GetAddedNodeInfoResult{{
			AddedNode: "127.0.0.210:9108",
			Connected: dcrjson.Bool(true),
			Addresses: &[]types.GetAddedNodeInfoResultAddr{{
				Address:   "127.0.0.210",
				Connected: "inbound",
			}},
		}, {
			AddedNode: "127.0.0.211:9108",
			Connected: dcrjson.Bool(true),
			Addresses: &[]types.GetAddedNodeInfoResultAddr{{
				Address:   "127.0.0.211",
				Connected: "outbound",
			}},
		}, {
			AddedNode: "mydomain.org:9108",
			Connected: dcrjson.Bool(true),
			Addresses: &[]types.GetAddedNodeInfoResultAddr{{
				Address:   "127.0.0.211",
				Connected: "false",
			}},
		}, {
			AddedNode: "nonexistentdomain.org:9108",
			Connected: dcrjson.Bool(true),
			Addresses: &[]types.GetAddedNodeInfoResultAddr{{
				Address:   "nonexistentdomain.org",
				Connected: "outbound",
			}},
		}},
	}, {
		name:    "handleGetAddedNodeInfo: found with DNS and with address filter",
		handler: handleGetAddedNodeInfo,
		cmd: &types.GetAddedNodeInfoCmd{
			DNS:  true,
			Node: dcrjson.String("mydomain.org:9108"),
		},
		result: []*types.GetAddedNodeInfoResult{{
			AddedNode: "mydomain.org:9108",
			Connected: dcrjson.Bool(true),
			Addresses: &[]types.GetAddedNodeInfoResultAddr{{
				Address:   "127.0.0.211",
				Connected: "false",
			}},
		}},
	}})
}

func TestHandleGetBestBlock(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetBestBlock: ok",
		handler: handleGetBestBlock,
		cmd:     &types.GetBestBlockCmd{},
		result: &types.GetBestBlockResult{
			Hash:   block432100.BlockHash().String(),
			Height: int64(block432100.Header.Height),
		},
	}})
}

func TestHandleGetBestBlockHash(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetBestBlockHash: ok",
		handler: handleGetBestBlockHash,
		cmd:     &types.GetBestBlockHashCmd{},
		result:  block432100.BlockHash().String(),
	}})
}

func TestHandleGetBlockchainInfo(t *testing.T) {
	t.Parallel()

	hash := mustParseHash("00000000000000001e6ec1501c858506de1de4703d1be8bab4061126e8f61480")
	prevHash := mustParseHash("00000000000000001a1ec2becd0dd90bfbd0c65f42fdaf608dd9ceac2a3aee1d")
	genesisHash := mustParseHash("298e5cc3d985bfe7f81dc135f360abe089edd4396b86d2de66b0cef42b21d980")
	genesisPrevHash := mustParseHash("0000000000000000000000000000000000000000000000000000000000000000")
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetBlockchainInfo: ok",
		handler: handleGetBlockchainInfo,
		cmd:     &types.GetBlockChainInfoCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot = &blockchain.BestState{
				Height:   463073,
				Bits:     404696953,
				Hash:     *hash,
				PrevHash: *prevHash,
			}
			chain.chainWork = big.NewInt(0).SetBytes([]byte{0x11, 0x5d, 0x28, 0x33, 0x84,
				0x90, 0x90, 0xb0, 0x02, 0x65, 0x06})
			chain.isCurrent = false
			chain.maxBlockSize = 393216
			chain.stateLastChangedHeight = int64(149248)
			return chain
		}(),
		result: types.GetBlockChainInfoResult{
			Chain:                "mainnet",
			Blocks:               int64(463073),
			Headers:              int64(463073),
			SyncHeight:           int64(463074),
			ChainWork:            "000000000000000000000000000000000000000000115d2833849090b0026506",
			InitialBlockDownload: true,
			VerificationProgress: float64(0.9999978405179302),
			BestBlockHash:        "00000000000000001e6ec1501c858506de1de4703d1be8bab4061126e8f61480",
			Difficulty:           uint32(404696953),
			DifficultyRatio:      float64(35256672611.3862),
			MaxBlockSize:         int64(393216),
			Deployments: map[string]types.AgendaInfo{
				"headercommitments": {
					Status:     "started",
					Since:      int64(149248),
					StartTime:  uint64(1567641600),
					ExpireTime: uint64(1599264000),
				},
			},
		},
	}, {
		name:    "handleGetBlockchainInfo: ok with empty blockchain",
		handler: handleGetBlockchainInfo,
		cmd:     &types.GetBlockChainInfoCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot = &blockchain.BestState{
				Height:   0,
				Bits:     453115903,
				Hash:     *genesisHash,
				PrevHash: *genesisPrevHash,
			}
			chain.chainWork = big.NewInt(0).SetBytes([]byte{0x80, 0x00, 0x40, 0x00, 0x20,
				0x00})
			chain.isCurrent = false
			chain.maxBlockSize = 393216
			chain.nextThresholdState = blockchain.ThresholdStateTuple{
				State:  blockchain.ThresholdDefined,
				Choice: uint32(0xffffffff),
			}
			chain.stateLastChangedHeight = int64(0)
			return chain
		}(),
		mockSyncManager: func() *testSyncManager {
			syncManager := defaultMockSyncManager()
			syncManager.syncHeight = 0
			return syncManager
		}(),
		result: types.GetBlockChainInfoResult{
			Chain:                "mainnet",
			Blocks:               int64(0),
			Headers:              int64(0),
			SyncHeight:           int64(0),
			ChainWork:            "0000000000000000000000000000000000000000000000000000800040002000",
			InitialBlockDownload: true,
			VerificationProgress: float64(0),
			BestBlockHash:        "298e5cc3d985bfe7f81dc135f360abe089edd4396b86d2de66b0cef42b21d980",
			Difficulty:           uint32(453115903),
			DifficultyRatio:      float64(32767.74999809),
			MaxBlockSize:         int64(393216),
			Deployments: map[string]types.AgendaInfo{
				"headercommitments": {
					Status:     "defined",
					Since:      int64(0),
					StartTime:  uint64(1567641600),
					ExpireTime: uint64(1599264000),
				},
			},
		},
	}, {
		name:    "handleGetBlockchainInfo: could not fetch chain work",
		handler: handleGetBlockchainInfo,
		cmd:     &types.GetBlockChainInfoCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot = &blockchain.BestState{
				Hash: *hash,
			}
			chain.chainWorkErr = errors.New("could not fetch chain work")
			chain.stateLastChangedHeight = int64(149248)
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleGetBlockchainInfo: could not fetch max block size",
		handler: handleGetBlockchainInfo,
		cmd:     &types.GetBlockChainInfoCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot = &blockchain.BestState{
				Hash: *hash,
			}
			chain.chainWork = big.NewInt(0).SetBytes([]byte{0x11, 0x5d, 0x28, 0x33, 0x84,
				0x90, 0x90, 0xb0, 0x02, 0x65, 0x06})
			chain.maxBlockSizeErr = errors.New("could not fetch max block size")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleGetBlockchainInfo: could not fetch threshold state",
		handler: handleGetBlockchainInfo,
		cmd:     &types.GetBlockChainInfoCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot = &blockchain.BestState{
				Height:   463073,
				Bits:     404696953,
				Hash:     *hash,
				PrevHash: *prevHash,
			}
			chain.chainWork = big.NewInt(0).SetBytes([]byte{0x11, 0x5d, 0x28, 0x33, 0x84,
				0x90, 0x90, 0xb0, 0x02, 0x65, 0x06})
			chain.maxBlockSize = 393216
			chain.nextThresholdStateErr = errors.New("could not fetch threshold state")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleGetBlockchainInfo: could not fetch state last changed",
		handler: handleGetBlockchainInfo,
		cmd:     &types.GetBlockChainInfoCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot = &blockchain.BestState{
				Height:   463073,
				Bits:     404696953,
				Hash:     *hash,
				PrevHash: *prevHash,
			}
			chain.chainWork = big.NewInt(0).SetBytes([]byte{0x11, 0x5d, 0x28, 0x33, 0x84,
				0x90, 0x90, 0xb0, 0x02, 0x65, 0x06})
			chain.maxBlockSize = 393216
			chain.stateLastChangedHeightErr = errors.New("could not fetch state last changed")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}})
}

func TestHandleGetBlock(t *testing.T) {
	t.Parallel()

	// Define variables related to block432100 to be used throughout the
	// handleGetBlock tests.
	blkHeader := block432100.Header
	blk := dcrutil.NewBlock(&block432100)
	blkHash := blk.Hash()
	blkHashString := blkHash.String()
	blkBytes, err := blk.Bytes()
	if err != nil {
		t.Fatalf("error serializing block: %+v", err)
	}
	blkHexString := hex.EncodeToString(blkBytes)
	bestHeight := int64(432151)
	confirmations := bestHeight - blk.Height() + 1
	nextHash := mustParseHash("000000000000000002e63055e402c823cb86c8258806508d84d6dc2a0790bd49")
	chainWork, _ := new(big.Int).SetString("0e805fb85284503581c57c", 16)

	// Create raw transaction results. This uses createTxRawResult, so ideally
	// createTxRawResult should be tested independently as well.
	txns := blk.Transactions()
	rawTxns := make([]types.TxRawResult, len(txns))
	testServer := &Server{cfg: *defaultMockConfig(defaultChainParams)}
	for i, tx := range txns {
		rawTxn, err := testServer.createTxRawResult(defaultChainParams, tx.MsgTx(),
			tx.Hash().String(), uint32(i), &blkHeader, blk.Hash().String(),
			int64(blkHeader.Height), confirmations)
		if err != nil {
			t.Fatalf("error creating tx raw result: %+v", err)
		}
		rawTxns[i] = *rawTxn
	}
	stxns := blk.STransactions()
	rawSTxns := make([]types.TxRawResult, len(stxns))
	for i, tx := range stxns {
		rawSTxn, err := testServer.createTxRawResult(defaultChainParams, tx.MsgTx(),
			tx.Hash().String(), uint32(i), &blkHeader, blk.Hash().String(),
			int64(blkHeader.Height), confirmations)
		if err != nil {
			t.Fatalf("error creating stx raw result: %+v", err)
		}
		rawSTxns[i] = *rawSTxn
	}

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetBlock: ok",
		handler: handleGetBlock,
		cmd: &types.GetBlockCmd{
			Hash:      blkHashString,
			Verbose:   dcrjson.Bool(false),
			VerboseTx: dcrjson.Bool(false),
		},
		result: blkHexString,
	}, {
		name:    "handleGetBlock: ok verbose",
		handler: handleGetBlock,
		cmd: &types.GetBlockCmd{
			Hash:      blkHashString,
			Verbose:   dcrjson.Bool(true),
			VerboseTx: dcrjson.Bool(false),
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot = &blockchain.BestState{
				Height: bestHeight,
			}
			chain.blockByHash = blk
			chain.blockHashByHeight = nextHash
			return chain
		}(),
		result: types.GetBlockVerboseResult{
			Hash:          blkHashString,
			Version:       blkHeader.Version,
			MerkleRoot:    blkHeader.MerkleRoot.String(),
			StakeRoot:     blkHeader.StakeRoot.String(),
			PreviousHash:  blkHeader.PrevBlock.String(),
			Nonce:         blkHeader.Nonce,
			VoteBits:      blkHeader.VoteBits,
			FinalState:    hex.EncodeToString(blkHeader.FinalState[:]),
			Voters:        blkHeader.Voters,
			FreshStake:    blkHeader.FreshStake,
			Revocations:   blkHeader.Revocations,
			PoolSize:      blkHeader.PoolSize,
			Time:          blkHeader.Timestamp.Unix(),
			StakeVersion:  blkHeader.StakeVersion,
			Confirmations: confirmations,
			Height:        int64(blkHeader.Height),
			Size:          int32(blkHeader.Size),
			Bits:          strconv.FormatInt(int64(blkHeader.Bits), 16),
			SBits:         dcrutil.Amount(blkHeader.SBits).ToCoin(),
			Difficulty:    float64(28147398026.656624),
			ChainWork:     fmt.Sprintf("%064x", chainWork),
			ExtraData:     hex.EncodeToString(blkHeader.ExtraData[:]),
			NextHash:      nextHash.String(),
			Tx: []string{
				"349b3e23b64cb4b71d09b9be4652c9e02e73430daee1285ea03d92aa437dcf37",
				"ea55dfc48f490b112d1e69d196aa47b068a122e0e45000791ebef41ef2f2918f",
			},
			STx: []string{
				"761f22f637f8a7df8fbfa0b411c211e16c40f907afce562ccc6a95e9b992b166",
				"439ea206a41a6d374f0fc88b68af434b58499579850b885e79bc657a2a5f88b8",
				"9e8904d2012875724d35c6d448bda9b6fcdc12b4700806f26a0e50acf52fe7e9",
				"9ce7b38320021d36d67dd68666e56be3d5187da734cee8f7fa8e378efbe17b57",
				"343cfa39bb122171b758edfe378e222ab702d78ca8ac6ad4b797e8353fe70f34",
			},
		},
	}, {
		name:    "handleGetBlock: ok verbose transactions",
		handler: handleGetBlock,
		cmd: &types.GetBlockCmd{
			Hash:      blkHashString,
			Verbose:   dcrjson.Bool(true),
			VerboseTx: dcrjson.Bool(true),
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot = &blockchain.BestState{
				Height: bestHeight,
			}
			chain.blockByHash = blk
			chain.blockHashByHeight = nextHash
			return chain
		}(),
		result: types.GetBlockVerboseResult{
			Hash:          blkHashString,
			Version:       blkHeader.Version,
			MerkleRoot:    blkHeader.MerkleRoot.String(),
			StakeRoot:     blkHeader.StakeRoot.String(),
			PreviousHash:  blkHeader.PrevBlock.String(),
			Nonce:         blkHeader.Nonce,
			VoteBits:      blkHeader.VoteBits,
			FinalState:    hex.EncodeToString(blkHeader.FinalState[:]),
			Voters:        blkHeader.Voters,
			FreshStake:    blkHeader.FreshStake,
			Revocations:   blkHeader.Revocations,
			PoolSize:      blkHeader.PoolSize,
			Time:          blkHeader.Timestamp.Unix(),
			StakeVersion:  blkHeader.StakeVersion,
			Confirmations: confirmations,
			Height:        int64(blkHeader.Height),
			Size:          int32(blkHeader.Size),
			Bits:          strconv.FormatInt(int64(blkHeader.Bits), 16),
			SBits:         dcrutil.Amount(blkHeader.SBits).ToCoin(),
			Difficulty:    float64(28147398026.656624),
			ChainWork:     fmt.Sprintf("%064x", chainWork),
			ExtraData:     hex.EncodeToString(blkHeader.ExtraData[:]),
			NextHash:      nextHash.String(),
			RawTx:         rawTxns,
			RawSTx:        rawSTxns,
		},
	}, {
		name:    "handleGetBlock: invalid hash",
		handler: handleGetBlock,
		cmd: &types.GetBlockCmd{
			Hash:      "invalid",
			Verbose:   dcrjson.Bool(false),
			VerboseTx: dcrjson.Bool(false),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}, {
		name:    "handleGetBlock: block not found",
		handler: handleGetBlock,
		cmd: &types.GetBlockCmd{
			Hash:      blkHashString,
			Verbose:   dcrjson.Bool(false),
			VerboseTx: dcrjson.Bool(false),
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.blockByHashErr = errors.New("block not found")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCBlockNotFound,
	}, {
		name:    "handleGetBlock: could not fetch chain work",
		handler: handleGetBlock,
		cmd: &types.GetBlockCmd{
			Hash:      blkHashString,
			Verbose:   dcrjson.Bool(true),
			VerboseTx: dcrjson.Bool(false),
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.chainWorkErr = errors.New("could not fetch chain work")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleGetBlock: no next block",
		handler: handleGetBlock,
		cmd: &types.GetBlockCmd{
			Hash:      blkHashString,
			Verbose:   dcrjson.Bool(true),
			VerboseTx: dcrjson.Bool(false),
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot = &blockchain.BestState{
				Height: bestHeight,
			}
			chain.blockHashByHeightErr = errors.New("no next block")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}})
}

func TestHandleGetBlockCount(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetBlockCount: ok",
		handler: handleGetBlockCount,
		cmd:     &types.GetBlockCountCmd{},
		result:  int64(block432100.Header.Height),
	}})
}

func TestHandleGetBlockHash(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetBlockHash: ok",
		handler: handleGetBlockHash,
		cmd: &types.GetBlockHashCmd{
			Index: int64(block432100.Header.Height),
		},
		result: block432100.BlockHash().String(),
	}, {
		name:    "handleGetBlockHash: block number out of range",
		handler: handleGetBlockHash,
		cmd: &types.GetBlockHashCmd{
			Index: -1,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.blockHashByHeightErr = errors.New("block number out of range")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCOutOfRange,
	}})
}

func TestHandleGetBlockHeader(t *testing.T) {
	t.Parallel()

	// Define variables related to block432100 to be used throughout the
	// handleGetBlockHeader tests.
	blkHeader := block432100.Header
	blkHeaderBytes, err := blkHeader.Bytes()
	if err != nil {
		t.Fatalf("error serializing block header: %+v", err)
	}
	blkHeaderHexString := hex.EncodeToString(blkHeaderBytes)
	blk := dcrutil.NewBlock(&block432100)
	blkHash := blk.Hash()
	blkHashString := blkHash.String()
	bestHeight := int64(432151)
	confirmations := bestHeight - blk.Height() + 1
	nextHash := mustParseHash("000000000000000002e63055e402c823cb86c8258806508d84d6dc2a0790bd49")
	chainWork, _ := new(big.Int).SetString("0e805fb85284503581c57c", 16)

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetBlockHeader: ok",
		handler: handleGetBlockHeader,
		cmd: &types.GetBlockHeaderCmd{
			Hash:    blkHashString,
			Verbose: dcrjson.Bool(false),
		},
		result: blkHeaderHexString,
	}, {
		name:    "handleGetBlockHeader: ok verbose",
		handler: handleGetBlockHeader,
		cmd: &types.GetBlockHeaderCmd{
			Hash:    blkHashString,
			Verbose: dcrjson.Bool(true),
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot = &blockchain.BestState{
				Height: bestHeight,
			}
			chain.blockHashByHeight = nextHash
			return chain
		}(),
		result: types.GetBlockHeaderVerboseResult{
			Hash:          blkHashString,
			Confirmations: confirmations,
			Version:       blkHeader.Version,
			MerkleRoot:    blkHeader.MerkleRoot.String(),
			StakeRoot:     blkHeader.StakeRoot.String(),
			VoteBits:      blkHeader.VoteBits,
			FinalState:    hex.EncodeToString(blkHeader.FinalState[:]),
			Voters:        blkHeader.Voters,
			FreshStake:    blkHeader.FreshStake,
			Revocations:   blkHeader.Revocations,
			PoolSize:      blkHeader.PoolSize,
			Bits:          strconv.FormatInt(int64(blkHeader.Bits), 16),
			SBits:         dcrutil.Amount(blkHeader.SBits).ToCoin(),
			Height:        blkHeader.Height,
			Size:          blkHeader.Size,
			Time:          blkHeader.Timestamp.Unix(),
			Nonce:         blkHeader.Nonce,
			ExtraData:     hex.EncodeToString(blkHeader.ExtraData[:]),
			StakeVersion:  blkHeader.StakeVersion,
			Difficulty:    float64(28147398026.656624),
			ChainWork:     fmt.Sprintf("%064x", chainWork),
			PreviousHash:  blkHeader.PrevBlock.String(),
			NextHash:      nextHash.String(),
		},
	}, {
		name:    "handleGetBlockHeader: invalid hash",
		handler: handleGetBlockHeader,
		cmd: &types.GetBlockHeaderCmd{
			Hash:    "invalid",
			Verbose: dcrjson.Bool(false),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}, {
		name:    "handleGetBlockHeader: block not found",
		handler: handleGetBlockHeader,
		cmd: &types.GetBlockHeaderCmd{
			Hash:    blkHashString,
			Verbose: dcrjson.Bool(false),
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.headerByHashErr = errors.New("block not found")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCBlockNotFound,
	}, {
		name:    "handleGetBlockHeader: could not fetch chain work",
		handler: handleGetBlockHeader,
		cmd: &types.GetBlockHeaderCmd{
			Hash:    blkHashString,
			Verbose: dcrjson.Bool(true),
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.chainWorkErr = errors.New("could not fetch chain work")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleGetBlockHeader: no next block",
		handler: handleGetBlockHeader,
		cmd: &types.GetBlockHeaderCmd{
			Hash:    blkHashString,
			Verbose: dcrjson.Bool(true),
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot = &blockchain.BestState{
				Height: bestHeight,
			}
			chain.blockHashByHeightErr = errors.New("no next block")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}})
}

func TestHandleGetBlockSubsidy(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetBlockSubsidy: ok",
		handler: handleGetBlockSubsidy,
		cmd: &types.GetBlockSubsidyCmd{
			Height: 463073,
			Voters: 5,
		},
		result: types.GetBlockSubsidyResult{
			Developer: int64(147908610),
			PoS:       int64(443725830),
			PoW:       int64(887451661),
			Total:     int64(1479086101),
		},
	}})
}

func TestHandleGetCFilter(t *testing.T) {
	t.Parallel()

	blkHashString := block432100.BlockHash().String()
	filterString := "000000112f2f7a8d1b6dd2170afadb2d5f9ae11085487a270d07ccdfb5e" +
		"31796a6162379f000001d43f94851308c4a1534d8"
	filter := hexToBytes(filterString)
	extendedFilterString := "0000000dbd0b842aa0b9bbe14fb5df84cbb2143c32ba4710767" +
		"02fbf52af819387b8216504c8d1"
	extendedFilter := hexToBytes(extendedFilterString)
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetCFilter: ok regular",
		handler: handleGetCFilter,
		cmd: &types.GetCFilterCmd{
			Hash:       blkHashString,
			FilterType: "regular",
		},
		mockFilterer: &testFilterer{
			filterByBlockHash: filter,
		},
		result: filterString,
	}, {
		name:    "handleGetCFilter: ok extended",
		handler: handleGetCFilter,
		cmd: &types.GetCFilterCmd{
			Hash:       blkHashString,
			FilterType: "extended",
		},
		mockFilterer: &testFilterer{
			filterByBlockHash: extendedFilter,
		},
		result: extendedFilterString,
	}, {
		name:    "handleGetCFilter: compact filters not enabled",
		handler: handleGetCFilter,
		cmd: &types.GetCFilterCmd{
			Hash:       blkHashString,
			FilterType: "regular",
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCNoCFIndex,
	}, {
		name:    "handleGetCFilter: invalid hash",
		handler: handleGetCFilter,
		cmd: &types.GetCFilterCmd{
			Hash:       "invalid",
			FilterType: "regular",
		},
		mockFilterer: &testFilterer{
			filterByBlockHash: filter,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}, {
		name:    "handleGetCFilter: unknown filter type",
		handler: handleGetCFilter,
		cmd: &types.GetCFilterCmd{
			Hash:       blkHashString,
			FilterType: "unknown",
		},
		mockFilterer: &testFilterer{
			filterByBlockHash: filter,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleGetCFilter: failed to load filter",
		handler: handleGetCFilter,
		cmd: &types.GetCFilterCmd{
			Hash:       blkHashString,
			FilterType: "regular",
		},
		mockFilterer: &testFilterer{
			filterByBlockHashErr: errors.New("failed to load filter"),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleGetCFilter: block not found",
		handler: handleGetCFilter,
		cmd: &types.GetCFilterCmd{
			Hash:       blkHashString,
			FilterType: "regular",
		},
		mockFilterer: &testFilterer{},
		wantErr:      true,
		errCode:      dcrjson.ErrRPCBlockNotFound,
	}})
}

func TestHandleGetCFilterHeader(t *testing.T) {
	t.Parallel()

	blkHashString := block432100.BlockHash().String()
	filter := hexToBytes("bd60b439d72fad354acc4b47d374ec34b0cca3bb25994b459f01f6" +
		"f84ad6b478")
	filterHash, err := chainhash.NewHash(filter)
	if err != nil {
		t.Fatalf("error creating chainhash.Hash: %+v", err)
	}
	extendedFilter := hexToBytes("72109550799eb4cedaec527fea34a3a2fb42f1e9187da1" +
		"bac1fd8d66274222bd")
	extendedFilterHash, err := chainhash.NewHash(extendedFilter)
	if err != nil {
		t.Fatalf("error creating chainhash.Hash: %+v", err)
	}
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetCFilterHeader: ok regular",
		handler: handleGetCFilterHeader,
		cmd: &types.GetCFilterHeaderCmd{
			Hash:       blkHashString,
			FilterType: "regular",
		},
		mockFilterer: &testFilterer{
			filterHeaderByBlockHash: filter,
		},
		result: filterHash.String(),
	}, {
		name:    "handleGetCFilterHeader: ok extended",
		handler: handleGetCFilterHeader,
		cmd: &types.GetCFilterHeaderCmd{
			Hash:       blkHashString,
			FilterType: "extended",
		},
		mockFilterer: &testFilterer{
			filterHeaderByBlockHash: extendedFilter,
		},
		result: extendedFilterHash.String(),
	}, {
		name:    "handleGetCFilterHeader: ok genesis block",
		handler: handleGetCFilterHeader,
		cmd: &types.GetCFilterHeaderCmd{
			Hash:       defaultChainParams.GenesisHash.String(),
			FilterType: "regular",
		},
		mockFilterer: &testFilterer{
			filterHeaderByBlockHash: zeroHash[:],
		},
		result: zeroHash.String(),
	}, {
		name:    "handleGetCFilterHeader: compact filters not enabled",
		handler: handleGetCFilterHeader,
		cmd: &types.GetCFilterHeaderCmd{
			Hash:       blkHashString,
			FilterType: "regular",
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCNoCFIndex,
	}, {
		name:    "handleGetCFilterHeader: invalid hash",
		handler: handleGetCFilterHeader,
		cmd: &types.GetCFilterHeaderCmd{
			Hash:       "invalid",
			FilterType: "regular",
		},
		mockFilterer: &testFilterer{
			filterHeaderByBlockHash: filter,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}, {
		name:    "handleGetCFilterHeader: unknown filter type",
		handler: handleGetCFilterHeader,
		cmd: &types.GetCFilterHeaderCmd{
			Hash:       blkHashString,
			FilterType: "unknown",
		},
		mockFilterer: &testFilterer{
			filterHeaderByBlockHash: filter,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleGetCFilterHeader: failed to load filter",
		handler: handleGetCFilterHeader,
		cmd: &types.GetCFilterHeaderCmd{
			Hash:       blkHashString,
			FilterType: "regular",
		},
		mockFilterer: &testFilterer{
			filterHeaderByBlockHashErr: errors.New("failed to load filter"),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleGetCFilterHeader: block not found",
		handler: handleGetCFilterHeader,
		cmd: &types.GetCFilterHeaderCmd{
			Hash:       blkHashString,
			FilterType: "regular",
		},
		mockFilterer: &testFilterer{
			filterHeaderByBlockHash: zeroHash[:],
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCBlockNotFound,
	}})
}

func TestHandleGetCFilterV2(t *testing.T) {
	t.Parallel()

	blkHashString := block432100.BlockHash().String()
	filter := hex.EncodeToString(defaultMockFiltererV2().filterByBlockHash.Bytes())
	var noFilterErr blockchain.NoFilterError
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetCFilterV2: ok",
		handler: handleGetCFilterV2,
		cmd: &types.GetCFilterV2Cmd{
			BlockHash: blkHashString,
		},
		result: &types.GetCFilterV2Result{
			BlockHash:   blkHashString,
			Data:        filter,
			ProofIndex:  blockchain.HeaderCmtFilterIndex,
			ProofHashes: nil,
		},
	}, {
		name:    "handleGetCFilterV2: invalid hash",
		handler: handleGetCFilterV2,
		cmd: &types.GetCFilterV2Cmd{
			BlockHash: "invalid",
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}, {
		name:    "handleGetCFilterV2: block not found",
		handler: handleGetCFilterV2,
		cmd: &types.GetCFilterV2Cmd{
			BlockHash: blkHashString,
		},
		mockFiltererV2: func() *testFiltererV2 {
			testFiltererV2 := defaultMockFiltererV2()
			testFiltererV2.filterByBlockHashErr = noFilterErr
			return testFiltererV2
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCBlockNotFound,
	}, {
		name:    "handleGetCFilterV2: failed to load filter",
		handler: handleGetCFilterV2,
		cmd: &types.GetCFilterV2Cmd{
			BlockHash: blkHashString,
		},
		mockFiltererV2: func() *testFiltererV2 {
			testFiltererV2 := defaultMockFiltererV2()
			testFiltererV2.filterByBlockHashErr = errors.New("failed to load filter")
			return testFiltererV2
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}})
}

func TestHandleGetChainTips(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetChainTips: ok",
		handler: handleGetChainTips,
		cmd:     &types.GetChainTipsCmd{},
		result: []types.GetChainTipsResult{{
			Height:    int64(block432100.Header.Height),
			Hash:      block432100.BlockHash().String(),
			BranchLen: 500,
			Status:    "active",
		}},
	}})
}

func TestHandleGetCoinSupply(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetCoinSupply: ok",
		handler: handleGetCoinSupply,
		cmd:     &types.GetCoinSupplyCmd{},
		result:  int64(1122503888072909),
	}})
}

func TestHandleGetConnectionCount(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetConnectionCount: ok",
		handler: handleGetConnectionCount,
		cmd:     &types.GetConnectionCountCmd{},
		result:  int32(4),
	}})
}

func TestHandleGetCurrentNet(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetCurrentNet: ok",
		handler: handleGetCurrentNet,
		cmd:     &types.GetCurrentNetCmd{},
		result:  wire.MainNet,
	}})
}

func TestHandleGetDifficulty(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetDifficulty: ok",
		handler: handleGetDifficulty,
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot.Bits = defaultChainParams.PowLimitBits
			return chain
		}(),
		cmd:    &types.GetDifficultyCmd{},
		result: float64(1.0),
	}})
}

func TestHandleGetInfo(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetInfo: ok",
		handler: handleGetInfo,
		cmd:     &types.GetInfoCmd{},
		result: &types.InfoChainResult{
			Version: int32(1000000*version.Major + 10000*version.Minor +
				100*version.Patch),
			ProtocolVersion: int32(wire.CFilterV2Version),
			Blocks:          int64(block432100.Header.Height),
			TimeOffset:      int64(0),
			Connections:     int32(4),
			Proxy:           "",
			Difficulty:      float64(28147398026.656624),
			TestNet:         false,
			RelayFee:        float64(0.0001),
			AddrIndex:       false,
			TxIndex:         false,
		},
	}})
}

func TestHandleGetNetTotals(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetNetTotals: ok",
		handler: handleGetNetTotals,
		cmd:     &types.GetNetTotalsCmd{},
		mockClock: &testClock{
			now: time.Unix(1592931302, 0),
		},
		result: &types.GetNetTotalsResult{
			TotalBytesRecv: uint64(9598159),
			TotalBytesSent: uint64(4783802),
			TimeMillis:     int64(1592931302000),
		},
	}})
}

func TestHandleGetNetworkInfo(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetNetworkInfo: ok",
		handler: handleGetNetworkInfo,
		cmd:     &types.GetNetworkInfoCmd{},
		result: types.GetNetworkInfoResult{
			Version: int32(1000000*version.Major + 10000*version.Minor +
				100*version.Patch),
			SubVersion: fmt.Sprintf("%d.%d.%d", version.Major, version.Minor,
				version.Patch),
			ProtocolVersion: int32(wire.CFilterV2Version),
			TimeOffset:      int64(0),
			Connections:     int32(4),
			Networks: []types.NetworksResult{{
				Name:                      "IPV4",
				Limited:                   false,
				Reachable:                 true,
				Proxy:                     "",
				ProxyRandomizeCredentials: false,
			}, {
				Name:                      "IPV6",
				Limited:                   false,
				Reachable:                 true,
				Proxy:                     "",
				ProxyRandomizeCredentials: false,
			}, {
				Name:                      "Onion",
				Limited:                   false,
				Reachable:                 false,
				Proxy:                     "",
				ProxyRandomizeCredentials: false,
			}},
			RelayFee: float64(0.0001),
			LocalAddresses: []types.LocalAddressesResult{{
				Address: "127.0.0.184",
				Port:    uint16(19108),
				Score:   int32(0),
			}},
			LocalServices: "0000000000000005",
		},
	}})
}

func TestHandleGetPeerInfo(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetPeerInfo: ok",
		handler: handleGetPeerInfo,
		cmd:     &types.GetPeerInfoCmd{},
		mockConnManager: func() *testConnManager {
			connManager := defaultMockConnManager()
			connManager.connectedPeers = []Peer{
				&testPeer{
					localAddr: testAddr{
						net:  "tcp",
						addr: "172.17.0.2:51060",
					},
					isTxRelayDisabled: false,
					banScore:          uint32(0),
					id:                int32(5),
					addr:              "106.14.238.184:19108",
					lastPingNonce:     uint64(10),
					statsSnapshot: &peer.StatsSnap{
						ID:             int32(5),
						Addr:           "106.14.238.184:19108",
						Services:       wire.SFNodeNetwork | wire.SFNodeCF,
						LastSend:       time.Unix(1592918788, 0),
						LastRecv:       time.Unix(1592918788, 0),
						BytesSent:      uint64(3406),
						BytesRecv:      uint64(2498),
						ConnTime:       time.Unix(1592918784, 0),
						TimeOffset:     int64(-75),
						Version:        uint32(6),
						UserAgent:      "/dcrwire:0.3.0/dcrd:1.5.0(pre)/",
						Inbound:        false,
						StartingHeight: int64(323327),
						LastBlock:      int64(323327),
						LastPingNonce:  uint64(10),
						LastPingTime:   time.Unix(1592918788, 0),
						LastPingMicros: int64(0),
					},
				},
			}
			return connManager
		}(),
		mockClock: &testClock{
			since: time.Duration(2000),
		},
		result: []*types.GetPeerInfoResult{{
			ID:             int32(5),
			Addr:           "106.14.238.184:19108",
			AddrLocal:      "172.17.0.2:51060",
			Services:       "00000005",
			RelayTxes:      true,
			LastSend:       int64(1592918788),
			LastRecv:       int64(1592918788),
			BytesSent:      uint64(3406),
			BytesRecv:      uint64(2498),
			ConnTime:       int64(1592918784),
			TimeOffset:     int64(-75),
			PingTime:       float64(0),
			PingWait:       float64(2),
			Version:        uint32(6),
			SubVer:         "/dcrwire:0.3.0/dcrd:1.5.0(pre)/",
			Inbound:        false,
			StartingHeight: int64(323327),
			CurrentHeight:  int64(323327),
			BanScore:       int32(0),
			SyncNode:       false,
		}},
	}})
}

func TestHandleGetStakeVersionInfo(t *testing.T) {
	t.Parallel()

	blk := dcrutil.NewBlock(&block432100)
	blkHashString := blk.Hash().String()
	blkHeight := blk.Height()
	blk2000Hash := mustParseHash("0000000000000c8a886e3f7c32b1bb08422066dcfd008de596471f11a5aff475")
	mockVersionCount := []types.VersionCount{{
		Version: 7,
		Count:   1,
	}}
	defaultChain := defaultMockRPCChain()
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetStakeVersionInfo: ok without specifying count",
		handler: handleGetStakeVersionInfo,
		cmd:     &types.GetStakeVersionInfoCmd{},
		result: types.GetStakeVersionInfoResult{
			CurrentHeight: blkHeight,
			Hash:          blkHashString,
			Intervals: []types.VersionInterval{{
				StartHeight:  defaultChain.calcWantHeight + 1,
				EndHeight:    blkHeight,
				PoSVersions:  mockVersionCount,
				VoteVersions: mockVersionCount,
			}},
		},
	}, {
		name:    "handleGetStakeVersionInfo: ok with count 2",
		handler: handleGetStakeVersionInfo,
		cmd: &types.GetStakeVersionInfoCmd{
			Count: dcrjson.Int32(2),
		},
		result: types.GetStakeVersionInfoResult{
			CurrentHeight: blkHeight,
			Hash:          blkHashString,
			Intervals: []types.VersionInterval{{
				StartHeight:  defaultChain.calcWantHeight + 1,
				EndHeight:    blkHeight,
				PoSVersions:  mockVersionCount,
				VoteVersions: mockVersionCount,
			}, {
				StartHeight: defaultChain.calcWantHeight + 1 -
					defaultChainParams.StakeVersionInterval,
				EndHeight:    defaultChain.calcWantHeight + 1,
				PoSVersions:  mockVersionCount,
				VoteVersions: mockVersionCount,
			}},
		},
	}, {
		name:    "handleGetStakeVersionInfo: invalid count",
		handler: handleGetStakeVersionInfo,
		cmd: &types.GetStakeVersionInfoCmd{
			Count: dcrjson.Int32(-1),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleGetStakeVersionInfo: ok with count > max intervals",
		handler: handleGetStakeVersionInfo,
		cmd: &types.GetStakeVersionInfoCmd{
			Count: dcrjson.Int32(5),
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot.Hash = *blk2000Hash
			chain.bestSnapshot.Height = 2000
			chain.calcWantHeight = 0
			return chain
		}(),
		result: types.GetStakeVersionInfoResult{
			CurrentHeight: 2000,
			Hash:          blk2000Hash.String(),
			Intervals: []types.VersionInterval{{
				StartHeight:  1,
				EndHeight:    2000,
				PoSVersions:  mockVersionCount,
				VoteVersions: mockVersionCount,
			}},
		},
	}, {
		name:    "handleGetStakeVersionInfo: ok with startHeight - endHeight == 0",
		handler: handleGetStakeVersionInfo,
		cmd:     &types.GetStakeVersionInfoCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.calcWantHeight = blkHeight
			return chain
		}(),
		result: types.GetStakeVersionInfoResult{
			CurrentHeight: blkHeight,
			Hash:          blkHashString,
			Intervals:     []types.VersionInterval{},
		},
	}, {
		name:    "handleGetStakeVersionInfo: failed to get stake versions",
		handler: handleGetStakeVersionInfo,
		cmd:     &types.GetStakeVersionInfoCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.getStakeVersionsErr = errors.New("failed to get stake versions")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleGetStakeVersionInfo: failed to get block hash",
		handler: handleGetStakeVersionInfo,
		cmd:     &types.GetStakeVersionInfoCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.blockHashByHeightErr = errors.New("failed to get block hash")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}})
}

func TestHandleGetStakeVersions(t *testing.T) {
	t.Parallel()

	blk := dcrutil.NewBlock(&block432100)
	blkHashString := blk.Hash().String()
	blkHeight := blk.Height()
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetStakeVersions: ok",
		handler: handleGetStakeVersions,
		cmd: &types.GetStakeVersionsCmd{
			Hash:  blkHashString,
			Count: 1,
		},
		result: types.GetStakeVersionsResult{
			StakeVersions: []types.StakeVersions{{
				Hash:         blkHashString,
				Height:       blkHeight,
				BlockVersion: block432100.Header.Version,
				StakeVersion: block432100.Header.StakeVersion,
				Votes: []types.VersionBits{{
					Version: 7,
					Bits:    1,
				}},
			}},
		},
	}, {
		name:    "handleGetStakeVersions: invalid hash",
		handler: handleGetStakeVersions,
		cmd: &types.GetStakeVersionsCmd{
			Hash:  "invalid",
			Count: 1,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}, {
		name:    "handleGetStakeVersions: invalid count",
		handler: handleGetStakeVersions,
		cmd: &types.GetStakeVersionsCmd{
			Hash:  blkHashString,
			Count: -1,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleGetStakeVersions: could not obtain stake versions",
		handler: handleGetStakeVersions,
		cmd: &types.GetStakeVersionsCmd{
			Hash:  blkHashString,
			Count: 1,
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.getStakeVersionsErr = errors.New("could not obtain stake versions")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}})
}

func TestHandleGetTicketPoolValue(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetTicketPoolValue: ok",
		handler: handleGetTicketPoolValue,
		cmd:     &types.GetTicketPoolValueCmd{},
		result:  defaultMockRPCChain().ticketPoolValue.ToCoin(),
	}, {
		name:    "handleGetTicketPoolValue: could not obtain ticket pool value",
		handler: handleGetTicketPoolValue,
		cmd:     &types.GetTicketPoolValueCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.ticketPoolValueErr = errors.New("could not obtain ticket pool value")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}})
}

func TestHandleGetTxOutSetInfo(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetTxOutSetInfo: ok",
		handler: handleGetTxOutSetInfo,
		cmd:     &types.GetTxOutSetInfoCmd{},
		result: types.GetTxOutSetInfoResult{
			Height:         int64(block432100.Header.Height),
			BestBlock:      block432100.BlockHash().String(),
			Transactions:   689819,
			TxOuts:         1593879,
			SerializedHash: "fe7b32aa188800f07268b17f3bead5f3d8a1b6d18654182066436efce6effa86",
			DiskSize:       36441617,
			TotalAmount:    1154067750680149,
		},
	}})
}

func TestHandleLiveTickets(t *testing.T) {
	t.Parallel()

	tkt1 := mustParseHash("1f6631957b4060d81ba7e760ec9c8150ba028eb051ddadf2b9749a5ccda1a955")
	tkt2 := mustParseHash("eca7e802590df60f7d300b6170f63dfab213b26421ed2e70de3ec2224cb9e460")

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleLiveTickets: no live tickets",
		handler: handleLiveTickets,
		cmd:     &types.LiveTicketsCmd{},
		result: types.LiveTicketsResult{
			Tickets: []string{},
		},
	}, {
		name:    "handleLiveTickets: two live tickets",
		handler: handleLiveTickets,
		cmd:     &types.LiveTicketsCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.liveTickets = []chainhash.Hash{*tkt1, *tkt2}
			return chain
		}(),
		result: types.LiveTicketsResult{
			Tickets: []string{tkt1.String(), tkt2.String()},
		},
	}, {
		name:    "handleLiveTickets: unable to fetch live tickets",
		handler: handleLiveTickets,
		cmd:     &types.LiveTicketsCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.liveTicketsErr = errors.New("unable to fetch live tickets")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}})
}

func TestHandleMissedTickets(t *testing.T) {
	t.Parallel()

	tkt1 := mustParseHash("1f6631957b4060d81ba7e760ec9c8150ba028eb051ddadf2b9749a5ccda1a955")
	tkt2 := mustParseHash("eca7e802590df60f7d300b6170f63dfab213b26421ed2e70de3ec2224cb9e460")

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleMissedTickets: no missed tickets",
		handler: handleMissedTickets,
		cmd:     &types.MissedTicketsCmd{},
		result: types.MissedTicketsResult{
			Tickets: []string{},
		},
	}, {
		name:    "handleMissedTickets: two missed tickets",
		handler: handleMissedTickets,
		cmd:     &types.MissedTicketsCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.missedTickets = []chainhash.Hash{*tkt1, *tkt2}
			return chain
		}(),
		result: types.MissedTicketsResult{
			Tickets: []string{tkt1.String(), tkt2.String()},
		},
	}, {
		name:    "handleMissedTickets: unable to fetch missed tickets",
		handler: handleMissedTickets,
		cmd:     &types.MissedTicketsCmd{},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.missedTicketsErr = errors.New("unable to fetch missed tickets")
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}})
}

func TestHandleNode(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleNode: ok with disconnect by address",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "disconnect",
			Target: "127.0.0.210:9108",
		},
		result: nil,
	}, {
		name:    "handleNode: disconnect by address peer not found",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "disconnect",
			Target: "127.0.0.299:9108",
		},
		mockConnManager: func() *testConnManager {
			connManager := defaultMockConnManager()
			connManager.disconnectByAddrErr = errors.New("peer not found")
			return connManager
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleNode: ok with disconnect by id",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "disconnect",
			Target: "28",
		},
		result: nil,
	}, {
		name:    "handleNode: disconnect by id peer not found",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "disconnect",
			Target: "99",
		},
		mockConnManager: func() *testConnManager {
			connManager := defaultMockConnManager()
			connManager.disconnectByIDErr = errors.New("peer not found")
			return connManager
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleNode: disconnect invalid address",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "disconnect",
			Target: "invalid_address",
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleNode: can't disconnect a permanent peer",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "disconnect",
			Target: "127.0.0.210:9108",
		},
		mockConnManager: func() *testConnManager {
			connManager := defaultMockConnManager()
			connManager.disconnectByAddrErr = errors.New("peer not found")
			return connManager
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCMisc,
	}, {
		name:    "handleNode: ok with remove by address",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "remove",
			Target: "127.0.0.210:9108",
		},
		result: nil,
	}, {
		name:    "handleNode: remove by address peer not found",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "remove",
			Target: "127.0.0.299:9108",
		},
		mockConnManager: func() *testConnManager {
			connManager := defaultMockConnManager()
			connManager.removeByAddrErr = errors.New("peer not found")
			return connManager
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleNode: ok with remove by id",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "remove",
			Target: "28",
		},
		result: nil,
	}, {
		name:    "handleNode: remove by id peer not found",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "remove",
			Target: "99",
		},
		mockConnManager: func() *testConnManager {
			connManager := defaultMockConnManager()
			connManager.removeByIDErr = errors.New("peer not found")
			return connManager
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleNode: remove invalid address",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "remove",
			Target: "invalid_address",
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleNode: can't remove a temporary peer",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "remove",
			Target: "127.0.0.210:9108",
		},
		mockConnManager: func() *testConnManager {
			connManager := defaultMockConnManager()
			connManager.removeByAddrErr = errors.New("peer not found")
			return connManager
		}(),

		wantErr: true,
		errCode: dcrjson.ErrRPCMisc,
	}, {
		name:    "handleNode: ok with connect perm",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd:        "connect",
			Target:        "127.0.0.210:9108",
			ConnectSubCmd: dcrjson.String("perm"),
		},
		result: nil,
	}, {
		name:    "handleNode: ok with connect temp",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd:        "connect",
			Target:        "127.0.0.210:9108",
			ConnectSubCmd: dcrjson.String("temp"),
		},
		result: nil,
	}, {
		name:    "handleNode: invalid connect sub cmd",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd:        "connect",
			Target:        "127.0.0.210:9108",
			ConnectSubCmd: dcrjson.String("invalid"),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleNode: invalid sub cmd",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "invalid",
			Target: "127.0.0.210:9108",
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}})
}

func TestHandlePing(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handlePing: ok",
		handler: handlePing,
		cmd:     &types.PingCmd{},
		result:  nil,
	}})
}

func TestHandleSubmitBlock(t *testing.T) {
	t.Parallel()

	blk := dcrutil.NewBlock(&block432100)
	blkBytes, err := blk.Bytes()
	if err != nil {
		t.Fatalf("error serializing block: %+v", err)
	}
	blkHexString := hex.EncodeToString(blkBytes)
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleSubmitBlock: ok",
		handler: handleSubmitBlock,
		cmd: &types.SubmitBlockCmd{
			HexBlock: blkHexString,
		},
		result: nil,
	}, {
		name:    "handleSubmitBlock: ok with odd length hex",
		handler: handleSubmitBlock,
		cmd: &types.SubmitBlockCmd{
			HexBlock: blkHexString[1:],
		},
		result: nil,
	}, {
		name:    "handleSubmitBlock: invalid hex",
		handler: handleSubmitBlock,
		cmd: &types.SubmitBlockCmd{
			HexBlock: "invalid",
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleSubmitBlock: block decode error",
		handler: handleSubmitBlock,
		cmd: &types.SubmitBlockCmd{
			HexBlock: "ffffffff",
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleSubmitBlock: block rejected",
		handler: handleSubmitBlock,
		cmd: &types.SubmitBlockCmd{
			HexBlock: blkHexString,
		},
		mockSyncManager: func() *testSyncManager {
			syncManager := defaultMockSyncManager()
			syncManager.submitBlockErr = errors.New("block rejected")
			return syncManager
		}(),
		result: "rejected: block rejected",
	}})
}

func TestHandleValidateAddress(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleValidateAddress: ok",
		handler: handleValidateAddress,
		cmd: &types.ValidateAddressCmd{
			Address: "DcdacYDf5SUH5dYyDxufRngdiaVhi6n83ka",
		},
		result: types.ValidateAddressChainResult{
			IsValid: true,
			Address: "DcdacYDf5SUH5dYyDxufRngdiaVhi6n83ka",
		},
	}, {
		name:    "handleValidateAddress: invalid address",
		handler: handleValidateAddress,
		cmd: &types.ValidateAddressCmd{
			Address: "invalid",
		},
		result: types.ValidateAddressChainResult{
			IsValid: false,
		},
	}, {
		name:    "handleValidateAddress: wrong network",
		handler: handleValidateAddress,
		cmd: &types.ValidateAddressCmd{
			Address: "Ssn23a3rJaCUxjqXiVSNwU6FxV45sLkiFpz",
		},
		result: types.ValidateAddressChainResult{
			IsValid: false,
		},
	}})
}

func TestHandleVerifyChain(t *testing.T) {
	t.Parallel()

	hash := mustParseHash("00000000000000000c07ff0178ad600db22c18f8f47fa423fa144b2ca919475d")

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleVerifyChain: ok",
		handler: handleVerifyChain,
		cmd:     &types.VerifyChainCmd{},
		result:  true,
	}, {
		name:    "handleVerifyChain: ok with depth=50",
		handler: handleVerifyChain,
		cmd: &types.VerifyChainCmd{
			CheckDepth: dcrjson.Int64(50),
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot = &blockchain.BestState{
				Height: 1,
				Hash:   *hash,
			}
			return chain
		}(),
		result: true,
	}, {
		name:    "handleVerifyChain: invalid block with checklevel=1",
		handler: handleVerifyChain,
		cmd: &types.VerifyChainCmd{
			CheckLevel: dcrjson.Int64(1),
			CheckDepth: dcrjson.Int64(50),
		},
		mockSanityChecker: func() *testSanityChecker {
			checker := defaultMockSanityChecker()
			checker.checkBlockSanityErr = errors.New("invalid block")
			return checker
		}(),
		result: false,
	}, {
		name:    "handleVerifyChain: no block exists",
		handler: handleVerifyChain,
		cmd: &types.VerifyChainCmd{
			CheckDepth: dcrjson.Int64(50),
		},
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot = &blockchain.BestState{
				Height: 100,
				Hash:   *hash,
			}
			chain.blockByHeightErr = errors.New("no block exists")
			return chain
		}(),
		result: false,
	}})
}

func TestHandleGetWork(t *testing.T) {
	t.Parallel()

	data := make([]byte, 0, getworkDataLen)
	buf := bytes.NewBuffer(data)
	err := block432100.Header.Serialize(buf)
	if err != nil {
		t.Fatalf("unexpected serialize error: %v", err)
	}

	data = data[:getworkDataLen]
	copy(data[wire.MaxBlockHeaderPayload:], blake256Pad)

	submissionB := make([]byte, hex.EncodedLen(len(data)))
	hex.Encode(submissionB, data)

	submission := string(submissionB)
	truncatedSubmission := submission[1:]
	lessThanGetWorkDataLen := submission[10:]

	buf = &bytes.Buffer{}
	buf.Write(submissionB[:10])
	buf.WriteRune('g')
	buf.Write(submissionB[10:])
	invalidHexSub := buf.String()

	buf.Reset()
	buf.Write(submissionB[:232])
	buf.WriteString("ffffffff")
	buf.Write(submissionB[240:])
	invalidPOWSub := buf.String()

	miningaddr, err := dcrutil.DecodeAddress("DsRM6qwzT3r85evKvDBJBviTgYcaLKL4ipD", defaultChainParams)
	if err != nil {
		t.Fatalf("[DecodeAddress] unexpected error: %v", err)
	}

	mine := func() *testMiningState {
		ms := defaultMockMiningState()
		ms.miningAddrs = []dcrutil.Address{miningaddr}
		return ms
	}

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetWork: CPU IsMining enabled",
		handler: handleGetWork,
		cmd:     &types.GetWorkCmd{},
		mockCPUMiner: func() *testCPUMiner {
			cpu := defaultMockCPUMiner()
			cpu.isMining = true
			return cpu
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCMisc,
	}, {
		name:    "handleGetWork: no mining address provided",
		handler: handleGetWork,
		cmd:     &types.GetWorkCmd{},
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleGetWork: no connected peers with unsynchronized mining disabled",
		handler: handleGetWork,
		cmd:     &types.GetWorkCmd{},
		mockConnManager: func() *testConnManager {
			connMgr := defaultMockConnManager()
			connMgr.connectedCount = 0
			return connMgr
		}(),
		mockMiningState: mine(),
		wantErr:         true,
		errCode:         dcrjson.ErrRPCClientNotConnected,
	}, {
		name:            "handleGetWork: chain is syncing",
		handler:         handleGetWork,
		cmd:             &types.GetWorkCmd{},
		mockMiningState: mine(),
		mockChain: func() *testRPCChain {
			chain := defaultMockRPCChain()
			chain.bestSnapshot = &blockchain.BestState{
				Height: 100,
			}
			chain.isCurrent = false
			return chain
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCClientInInitialDownload,
	}, {
		name:    "handleGetWork: ok with a workstate entry",
		handler: handleGetWork,
		cmd:     &types.GetWorkCmd{},
		mockBlockTemplater: func() *testBlockTemplater {
			templater := defaultMockBlockTemplater()
			templater.simulateNewNtfn = true
			return templater
		}(),
		mockMiningState: mine(),
		result: &types.GetWorkResult{
			Data: "070000009c3c0efea268c124d46d7daeae2d9667e78daa0523a19725" +
				"00000000000000000bc8a255edde9901ecc4cdb93e4e573cb38ae91e84" +
				"495ecddc0c93c019351d5d7731998be0a78e955f6fb98d2f35479905c3" +
				"279f6257beab42a51d556cef55b9010087ba86bb2e5204000100b1a000" +
				"00e20f27181e4afc5b03000000e4970600de0a0000d2b46d5ef63e4a6d" +
				"d6ab3b0000000000a200ca770000000000000000000000000000000000" +
				"000000070000008000000100000000000005a0",
			Target: "000000000000000000000000000000000000000000e20f27000000" +
				"0000000000",
		},
	}, {
		name:            "handleGetWork: ok with no workstate entries",
		handler:         handleGetWork,
		cmd:             &types.GetWorkCmd{},
		mockMiningState: mine(),
		result: &types.GetWorkResult{
			Data: "070000009c3c0efea268c124d46d7daeae2d9667e78daa0523a19725" +
				"00000000000000000bc8a255edde9901ecc4cdb93e4e573cb38ae91e84" +
				"495ecddc0c93c019351d5d7731998be0a78e955f6fb98d2f35479905c3" +
				"279f6257beab42a51d556cef55b9010087ba86bb2e5204000100b1a000" +
				"00e20f27181e4afc5b03000000e4970600de0a0000d2b46d5ef63e4a6d" +
				"d6ab3b0000000000a200ca770000000000000000000000000000000000" +
				"000000070000008000000100000000000005a0",
			Target: "000000000000000000000000000000000000000000e20f27000000" +
				"0000000000",
		},
	}, {
		name:            "handleGetWork: unable to retrieve template",
		handler:         handleGetWork,
		cmd:             &types.GetWorkCmd{},
		mockMiningState: mine(),
		mockBlockTemplater: func() *testBlockTemplater {
			templater := defaultMockBlockTemplater()
			templater.currTemplateErr = errors.New("unable to retrieve template")
			return templater
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:            "handleGetWork: unable to update block time",
		handler:         handleGetWork,
		cmd:             &types.GetWorkCmd{},
		mockMiningState: mine(),
		mockBlockTemplater: func() *testBlockTemplater {
			templater := defaultMockBlockTemplater()
			templater.updateBlockTimeErr = errors.New("unable to update block time")
			return templater
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleGetWork: data is not equal to getworkDataLen (192 bytes)",
		handler: handleGetWork,
		cmd: &types.GetWorkCmd{
			Data: &lessThanGetWorkDataLen,
		},
		mockMiningState: mine(),
		wantErr:         true,
		errCode:         dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleGetWork: submission has no matching template",
		handler: handleGetWork,
		cmd: &types.GetWorkCmd{
			Data: &submission,
		},
		mockMiningState: func() *testMiningState {
			ms := defaultMockMiningState()
			ms.miningAddrs = []dcrutil.Address{miningaddr}
			ms.workState = newWorkState()
			return ms
		}(),
		result: false,
	}, {
		name:    "handleGetWork: submission is an orphan",
		handler: handleGetWork,
		cmd: &types.GetWorkCmd{
			Data: &truncatedSubmission,
		},
		mockMiningState: mine(),
		mockSyncManager: func() *testSyncManager {
			syncManager := defaultMockSyncManager()
			syncManager.isOrphan = true
			return syncManager
		}(),
		result: false,
	}, {
		name:    "handleGetWork: unable to submit block",
		handler: handleGetWork,
		cmd: &types.GetWorkCmd{
			Data: &submission,
		},
		mockMiningState: mine(),
		mockSyncManager: func() *testSyncManager {
			syncManager := defaultMockSyncManager()
			syncManager.submitBlockErr = errors.New("unable to submit block")
			return syncManager
		}(),
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleGetWork: submission ok",
		handler: handleGetWork,
		cmd: &types.GetWorkCmd{
			Data: &submission,
		},
		mockMiningState: mine(),
		result:          true,
	}, {
		name:    "handleGetWork: invalid submission hex",
		handler: handleGetWork,
		cmd: &types.GetWorkCmd{
			Data: &invalidHexSub,
		},
		mockMiningState: mine(),
		wantErr:         true,
		errCode:         dcrjson.ErrRPCDecodeHexString,
	}, {
		name:    "handleGetWork: invalid proof of work",
		handler: handleGetWork,
		cmd: &types.GetWorkCmd{
			Data: &invalidPOWSub,
		},
		mockMiningState: mine(),
		wantErr:         false,
		result:          false,
	}, {
		name:    "handleGetWork: duplicate block",
		handler: handleGetWork,
		cmd: &types.GetWorkCmd{
			Data: &submission,
		},
		mockMiningState: mine(),
		mockSyncManager: func() *testSyncManager {
			syncManager := defaultMockSyncManager()
			syncManager.submitBlockErr = blockchain.RuleError{
				ErrorCode:   blockchain.ErrDuplicateBlock,
				Description: "Duplicate Block",
			}
			return syncManager
		}(),
		wantErr: false,
		result:  false,
	}})
}

func TestHandleSetGenerate(t *testing.T) {
	t.Parallel()

	procLimit := 2
	zeroProcLimit := 0
	miningaddr, err := dcrutil.DecodeAddress("DsRM6qwzT3r85evKvDBJBviTgYcaLKL4ipD", defaultChainParams)
	if err != nil {
		t.Fatalf("[DecodeAddress] unexpected error: %v", err)
	}

	testRPCServerHandler(t, []rpcTest{{
		name:    "handleSetGenerate: no payment addresses",
		handler: handleSetGenerate,
		cmd: &types.SetGenerateCmd{
			Generate:     true,
			GenProcLimit: &procLimit,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleSetGenerate: ok",
		handler: handleSetGenerate,
		cmd: &types.SetGenerateCmd{
			Generate:     true,
			GenProcLimit: &procLimit,
		},
		mockMiningState: func() *testMiningState {
			ms := defaultMockMiningState()
			ms.miningAddrs = []dcrutil.Address{miningaddr}
			return ms
		}(),
	}, {
		name:    "handleSetGenerate: ok, generate=false",
		handler: handleSetGenerate,
		cmd: &types.SetGenerateCmd{
			Generate: false,
		},
	}, {
		name:    "handleSetGenerate: ok, proclimit=0",
		handler: handleSetGenerate,
		cmd: &types.SetGenerateCmd{
			GenProcLimit: &zeroProcLimit,
		},
	}})
}

func TestHandleRegenTemplate(t *testing.T) {
	t.Parallel()

	testRPCServerHandler(t, []rpcTest{{
		name:                 "handleRegenTemplate: node is not configured for mining",
		handler:              handleRegenTemplate,
		cmd:                  &types.RegenTemplateCmd{},
		setBlockTemplaterNil: true,
		wantErr:              true,
		errCode:              dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleRegenTemplate: ok",
		handler: handleRegenTemplate,
		cmd:     &types.RegenTemplateCmd{},
	}})
}

func testRPCServerHandler(t *testing.T, tests []rpcTest) {
	t.Helper()

	for _, test := range tests {
		test := test // capture range variable
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Create a default rpcserverConfig and override any configurations
			// that are provided by the test.
			chainParams := defaultChainParams
			workState := newWorkState()
			if test.mockChainParams != nil {
				chainParams = test.mockChainParams
			}
			rpcserverConfig := defaultMockConfig(chainParams)
			if test.mockChain != nil {
				rpcserverConfig.Chain = test.mockChain
			}
			if test.mockAddrManager != nil {
				rpcserverConfig.AddrManager = test.mockAddrManager
			}
			if test.mockSyncManager != nil {
				rpcserverConfig.SyncMgr = test.mockSyncManager
			}
			if test.mockExistsAddresser != nil {
				rpcserverConfig.ExistsAddresser = test.mockExistsAddresser
			}
			if test.setExistsAddresserNil {
				rpcserverConfig.ExistsAddresser = nil
			}
			if test.mockConnManager != nil {
				rpcserverConfig.ConnMgr = test.mockConnManager
			}
			if test.mockClock != nil {
				rpcserverConfig.Clock = test.mockClock
			}
			if test.mockFeeEstimator != nil {
				rpcserverConfig.FeeEstimator = test.mockFeeEstimator
			}
			if test.mockLogManager != nil {
				rpcserverConfig.LogManager = test.mockLogManager
			}
			if test.mockSanityChecker != nil {
				rpcserverConfig.SanityChecker = test.mockSanityChecker
			}
			if test.mockFilterer != nil {
				rpcserverConfig.Filterer = test.mockFilterer
			}
			if test.mockFiltererV2 != nil {
				rpcserverConfig.FiltererV2 = test.mockFiltererV2
			}
			if test.mockCPUMiner != nil {
				rpcserverConfig.CPUMiner = test.mockCPUMiner
			}
			if test.mockMiningState != nil {
				ms := test.mockMiningState
				rpcserverConfig.AllowUnsyncedMining = ms.allowUnsyncedMining
				rpcserverConfig.MiningAddrs = ms.miningAddrs
				if ms.workState != nil {
					workState = ms.workState
				}
			}
			if test.mockBlockTemplater != nil {
				rpcserverConfig.BlockTemplater = test.mockBlockTemplater
			}
			if test.setBlockTemplaterNil {
				rpcserverConfig.BlockTemplater = nil
			}
			if test.mockTxMempooler != nil {
				rpcserverConfig.TxMempooler = test.mockTxMempooler
			}

			testServer := &Server{cfg: *rpcserverConfig, workState: workState}
			result, err := test.handler(nil, testServer, test.cmd)
			if test.wantErr {
				var rpcErr *dcrjson.RPCError
				if !errors.As(err, &rpcErr) || rpcErr.Code != test.errCode {
					if rpcErr != nil {
						t.Errorf("%s\nwant: %+v\n got: %+v\n", test.name, test.errCode, rpcErr.Code)
					} else {
						t.Errorf("%s\nwant: %+v\n got: nil\n", test.name, test.errCode)
					}
				}
				return
			}
			if err != nil {
				t.Errorf("%s\nunexpected error: %+v\n", test.name, err)
				return
			}
			if !reflect.DeepEqual(result, test.result) {
				t.Errorf("%s\nwant: %+v\n got: %+v\n", test.name, spew.Sdump(test.result), spew.Sdump(result))
			}
		})
	}
}
