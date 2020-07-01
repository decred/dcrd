// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/addrmgr"
	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/blockchain/v3"
	"github.com/decred/dcrd/blockchain/v3/indexers"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrjson/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/gcs/v2"
	"github.com/decred/dcrd/internal/rpcserver"
	"github.com/decred/dcrd/internal/version"
	"github.com/decred/dcrd/mempool/v4"
	"github.com/decred/dcrd/peer/v2"
	"github.com/decred/dcrd/rpc/jsonrpc/types/v2"
	"github.com/decred/dcrd/wire"
)

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
	blockByHeight                   *dcrutil.Block
	blockHashByHeight               *chainhash.Hash
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
	estimateNextStakeDifficulty     int64
	fetchUtxoEntry                  rpcserver.UtxoEntry
	fetchUtxoStats                  *blockchain.UtxoStats
	filterByBlockHash               *gcs.FilterV2
	getStakeVersions                []blockchain.StakeVersions
	getVoteCounts                   blockchain.VoteCounts
	getVoteInfo                     *blockchain.VoteInfo
	headerByHash                    wire.BlockHeader
	headerByHeight                  wire.BlockHeader
	heightRange                     []chainhash.Hash
	isCurrent                       bool
	liveTickets                     []chainhash.Hash
	locateHeaders                   []wire.BlockHeader
	lotteryDataForBlock             []chainhash.Hash
	mainChainHasBlock               bool
	maxBlockSize                    int64
	maxBlockSizeErr                 error
	missedTickets                   []chainhash.Hash
	nextThresholdState              blockchain.ThresholdStateTuple
	nextThresholdStateErr           error
	stateLastChangedHeight          int64
	stateLastChangedHeightErr       error
	ticketPoolValue                 dcrutil.Amount
	ticketsWithAddress              []chainhash.Hash
	tipGeneration                   []chainhash.Hash
}

// BestSnapshot returns a mocked blockchain.BestState.
func (c *testRPCChain) BestSnapshot() *blockchain.BestState {
	return c.bestSnapshot
}

// BlockByHash returns a mocked block for the given hash.
func (c *testRPCChain) BlockByHash(hash *chainhash.Hash) (*dcrutil.Block, error) {
	return c.blockByHash, nil
}

// BlockByHeight returns a mocked block at the given height.
func (c *testRPCChain) BlockByHeight(height int64) (*dcrutil.Block, error) {
	return c.blockByHeight, nil
}

// BlockHashByHeight returns a mocked hash of the block at the given height.
func (c *testRPCChain) BlockHashByHeight(height int64) (*chainhash.Hash, error) {
	return c.blockHashByHeight, nil
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
func (c *testRPCChain) ConvertUtxosToMinimalOutputs(entry rpcserver.UtxoEntry) []*stake.MinimalOutput {
	return c.convertUtxosToMinimalOutputs
}

// CountVoteVersion returns a mocked total number of version votes for the current
// rule change activation interval.
func (c *testRPCChain) CountVoteVersion(version uint32) (uint32, error) {
	return c.countVoteVersion, nil
}

// EstimateNextStakeDifficulty a mocked estimated next stake difficulty.
func (c *testRPCChain) EstimateNextStakeDifficulty(newTickets int64, useMaxTickets bool) (int64, error) {
	return c.estimateNextStakeDifficulty, nil
}

// FetchUtxoEntry returns a mocked UtxoEntry.
func (c *testRPCChain) FetchUtxoEntry(txHash *chainhash.Hash) (rpcserver.UtxoEntry, error) {
	return c.fetchUtxoEntry, nil
}

// FetchUtxoStats returns a mocked blockchain.UtxoStats.
func (c *testRPCChain) FetchUtxoStats() (*blockchain.UtxoStats, error) {
	return c.fetchUtxoStats, nil
}

// FilterByBlockHash returns a mocked version 2 GCS filter for the given block
// hash when it exists.
func (c *testRPCChain) FilterByBlockHash(hash *chainhash.Hash) (*gcs.FilterV2, error) {
	return c.filterByBlockHash, nil
}

// GetStakeVersions returns a mocked cooked array of StakeVersions.
func (c *testRPCChain) GetStakeVersions(hash *chainhash.Hash, count int32) ([]blockchain.StakeVersions, error) {
	return c.getStakeVersions, nil
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
	return c.headerByHash, nil
}

// HeaderByHeight returns a mocked block header at the given height.
func (c *testRPCChain) HeaderByHeight(height int64) (wire.BlockHeader, error) {
	return c.headerByHeight, nil
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
	return c.liveTickets, nil
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
	return c.missedTickets, nil
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
	return c.ticketPoolValue, nil
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
	submitBlock        bool
	syncPeerID         int32
	locateBlocks       []chainhash.Hash
	existsAddrIndex    *indexers.ExistsAddrIndex
	cfIndex            *indexers.CFIndex
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
	return s.submitBlock, nil
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

// ExistsAddrIndex returns a mocked address index.
func (s *testSyncManager) ExistsAddrIndex() *indexers.ExistsAddrIndex {
	return s.existsAddrIndex
}

// CFIndex returns a mocked committed filter (cf) by hash index.
func (s *testSyncManager) CFIndex() *indexers.CFIndex {
	return s.cfIndex
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
	connectedPeers      []rpcserver.Peer
	persistentPeers     []rpcserver.Peer
	addedNodeInfo       []rpcserver.Peer
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
func (c *testConnManager) ConnectedPeers() []rpcserver.Peer {
	return c.connectedPeers
}

// PersistentPeers returns a mocked slice of all persistent peers.
func (c *testConnManager) PersistentPeers() []rpcserver.Peer {
	return c.persistentPeers
}

// BroadcastMessage provides a mock implementation for sending the provided
// message to all currently connected peers.
func (c *testConnManager) BroadcastMessage(msg wire.Message) {
}

// AddRebroadcastInventory provides a mock implementation for adding the
// provided inventory to the list of inventories to be rebroadcast at random
// intervals until they show up in a block.
func (c *testConnManager) AddRebroadcastInventory(iv *wire.InvVect, data interface{}) {
}

// RelayTransactions provides a mock implementation for generating and relaying
// inventory vectors for all of the passed transactions to all connected peers.
func (c *testConnManager) RelayTransactions(txns []*dcrutil.Tx) {
}

// AddedNodeInfo returns a mocked slice of persistent (added) peers.
func (c *testConnManager) AddedNodeInfo() []rpcserver.Peer {
	return c.addedNodeInfo
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

type rpcTest struct {
	name            string
	handler         commandHandler
	cmd             interface{}
	mockChainParams *chaincfg.Params
	mockChain       *testRPCChain
	mockAddrManager *testAddrManager
	mockSyncManager *testSyncManager
	mockConnManager *testConnManager
	mockClock       *testClock
	mockCfg         *config
	result          interface{}
	wantErr         bool
	errCode         dcrjson.RPCErrorCode
}

func TestHandleAddNode(t *testing.T) {
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleAddNode: ok",
		handler: handleAddNode,
		cmd: &types.AddNodeCmd{
			Addr:   "160.221.215.210:9108",
			SubCmd: "add",
		},
		mockConnManager: &testConnManager{},
		result:          nil,
	}, {
		name:    "handleAddNode: 'add' subcommand error",
		handler: handleAddNode,
		cmd: &types.AddNodeCmd{
			Addr:   "160.221.215.210:9108",
			SubCmd: "add",
		},
		mockConnManager: &testConnManager{
			connectErr: errors.New("peer already connected"),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleAddNode: 'remove' subcommand error",
		handler: handleAddNode,
		cmd: &types.AddNodeCmd{
			Addr:   "160.221.215.210:9108",
			SubCmd: "remove",
		},
		mockConnManager: &testConnManager{
			removeByAddrErr: errors.New("peer not found"),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleAddNode: 'onetry' subcommand error",
		handler: handleAddNode,
		cmd: &types.AddNodeCmd{
			Addr:   "160.221.215.210:9108",
			SubCmd: "onetry",
		},
		mockConnManager: &testConnManager{
			connectErr: errors.New("peer already connected"),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleAddNode: invalid subcommand",
		handler: handleAddNode,
		cmd: &types.AddNodeCmd{
			Addr:   "160.221.215.210:9108",
			SubCmd: "",
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}})
}

func TestHandleCreateRawSStx(t *testing.T) {
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
	defaultCmdInputs := []types.TransactionInput{{
		Amount: 100,
		Txid:   "1189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
		Vout:   0,
		Tree:   1,
	}}
	defaultFee := dcrjson.Float64(1)
	defaultMockFetchUtxoEntry := &testRPCUtxoEntry{
		txType:     stake.TxTypeSStx,
		height:     100000,
		index:      0,
		txVersion:  1,
		isCoinBase: false,
		hasExpiry:  true,
		modified:   false,
	}
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleCreateRawSSRtx: ok",
		handler: handleCreateRawSSRtx,
		cmd: &types.CreateRawSSRtxCmd{
			Inputs: defaultCmdInputs,
			Fee:    defaultFee,
		},
		mockChain: &testRPCChain{
			fetchUtxoEntry: defaultMockFetchUtxoEntry,
			convertUtxosToMinimalOutputs: []*stake.MinimalOutput{{
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
					0x5C, 0x77, 0x0D, 0x00, 0x00, 0x00, 0x00, 0x00, // commitamt
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
			}},
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
		mockChain: &testRPCChain{
			fetchUtxoEntry: defaultMockFetchUtxoEntry,
			convertUtxosToMinimalOutputs: []*stake.MinimalOutput{{
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
			}},
		},
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
		mockChain: &testRPCChain{
			fetchUtxoEntry: nil,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCNoTxInfo,
	}, {
		name:    "handleCreateRawSSRtx: invalid tx type",
		handler: handleCreateRawSSRtx,
		cmd: &types.CreateRawSSRtxCmd{
			Inputs: defaultCmdInputs,
			Fee:    defaultFee,
		},
		mockChain: &testRPCChain{
			fetchUtxoEntry: &testRPCUtxoEntry{
				txType:     stake.TxTypeRegular,
				height:     100000,
				index:      0,
				txVersion:  1,
				isCoinBase: false,
				hasExpiry:  true,
				modified:   false,
			},
		},
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
		mockChain: &testRPCChain{
			fetchUtxoEntry: defaultMockFetchUtxoEntry,
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
		mockChain: &testRPCChain{
			fetchUtxoEntry: defaultMockFetchUtxoEntry,
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
		mockChain: &testRPCChain{
			fetchUtxoEntry: defaultMockFetchUtxoEntry,
			convertUtxosToMinimalOutputs: []*stake.MinimalOutput{{
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
			}},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}})
}

func TestHandleCreateRawTransaction(t *testing.T) {
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
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleDebugLevel: show",
		handler: handleDebugLevel,
		cmd: &types.DebugLevelCmd{
			LevelSpec: "show",
		},
		result: fmt.Sprintf("Supported subsystems %v", supportedSubsystems()),
	}, {
		name:    "handleDebugLevel: invalidDebugLevel",
		handler: handleDebugLevel,
		cmd: &types.DebugLevelCmd{
			LevelSpec: "invalidDebugLevel",
		},
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

func TestHandleEstimateFee(t *testing.T) {
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleEstimateFee: ok",
		handler: handleEstimateFee,
		cmd:     &types.EstimateFeeCmd{},
		mockCfg: &config{
			minRelayTxFee: dcrutil.Amount(int64(10000)),
		},
		result: float64(0.0001),
	}})
}

func TestHandleExistsExpiredTickets(t *testing.T) {
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
		mockChain: &testRPCChain{
			checkExpiredTickets: []bool{true, true},
		},
		result: "03",
	}, {
		name:    "handleExistsExpiredTickets: only first ticket exists",
		handler: handleExistsExpiredTickets,
		cmd: &types.ExistsExpiredTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: &testRPCChain{
			checkExpiredTickets: []bool{true, false},
		},
		result: "01",
	}, {
		name:    "handleExistsExpiredTickets: only second ticket exists",
		handler: handleExistsExpiredTickets,
		cmd: &types.ExistsExpiredTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: &testRPCChain{
			checkExpiredTickets: []bool{false, true},
		},
		result: "02",
	}, {
		name:    "handleExistsExpiredTickets: none of the tickets exist",
		handler: handleExistsExpiredTickets,
		cmd: &types.ExistsExpiredTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: &testRPCChain{
			checkExpiredTickets: []bool{false, false},
		},
		result: "00",
	}, {
		name:    "handleExistsExpiredTickets: invalid hash",
		handler: handleExistsExpiredTickets,
		cmd: &types.ExistsExpiredTicketsCmd{
			TxHashes: []string{
				"g189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
			},
		},
		mockChain: &testRPCChain{
			checkExpiredTickets: []bool{true, true},
		},
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
		mockChain: &testRPCChain{
			checkExpiredTickets: []bool{true, true},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}})
}

func TestHandleExistsLiveTicket(t *testing.T) {
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleExistsLiveTicket: ticket exists",
		handler: handleExistsLiveTicket,
		cmd: &types.ExistsLiveTicketCmd{
			TxHash: "1189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
		},
		mockChain: &testRPCChain{
			checkLiveTicket: true,
		},
		result: true,
	}, {
		name:    "handleExistsLiveTicket: ticket does not exist",
		handler: handleExistsLiveTicket,
		cmd: &types.ExistsLiveTicketCmd{
			TxHash: "1189cbe656c2ef1e0fcb91f107624d9aa8f0db7b28e6a86f694a4cf49abc5e39",
		},
		mockChain: &testRPCChain{
			checkLiveTicket: false,
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
		mockChain: &testRPCChain{
			checkLiveTickets: []bool{true, true},
		},
		result: "03",
	}, {
		name:    "handleExistsLiveTickets: only first ticket exists",
		handler: handleExistsLiveTickets,
		cmd: &types.ExistsLiveTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: &testRPCChain{
			checkLiveTickets: []bool{true, false},
		},
		result: "01",
	}, {
		name:    "handleExistsLiveTickets: only second ticket exists",
		handler: handleExistsLiveTickets,
		cmd: &types.ExistsLiveTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: &testRPCChain{
			checkLiveTickets: []bool{false, true},
		},
		result: "02",
	}, {
		name:    "handleExistsLiveTickets: none of the tickets exist",
		handler: handleExistsLiveTickets,
		cmd: &types.ExistsLiveTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: &testRPCChain{
			checkLiveTickets: []bool{false, false},
		},
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
		mockChain: &testRPCChain{
			checkLiveTickets: []bool{true, true},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}})
}

func TestHandleExistsMissedTickets(t *testing.T) {
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
		mockChain: &testRPCChain{
			checkMissedTickets: []bool{true, true},
		},
		result: "03",
	}, {
		name:    "handleExistsMissedTickets: only first ticket exists",
		handler: handleExistsMissedTickets,
		cmd: &types.ExistsMissedTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: &testRPCChain{
			checkMissedTickets: []bool{true, false},
		},
		result: "01",
	}, {
		name:    "handleExistsMissedTickets: only second ticket exists",
		handler: handleExistsMissedTickets,
		cmd: &types.ExistsMissedTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: &testRPCChain{
			checkMissedTickets: []bool{false, true},
		},
		result: "02",
	}, {
		name:    "handleExistsMissedTickets: none of the tickets exist",
		handler: handleExistsMissedTickets,
		cmd: &types.ExistsMissedTicketsCmd{
			TxHashes: defaultCmdTxHashes,
		},
		mockChain: &testRPCChain{
			checkMissedTickets: []bool{false, false},
		},
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
		mockChain: &testRPCChain{
			checkMissedTickets: []bool{true, true},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}})
}

func TestHandleGetAddedNodeInfo(t *testing.T) {
	testPeer1 := &testPeer{
		addr:      "160.221.215.210",
		connected: true,
		inbound:   true,
	}
	testPeer2 := &testPeer{
		addr:      "160.221.215.211:9108",
		connected: true,
		inbound:   false,
	}
	testPeer3 := &testPeer{
		addr:      "mydomain.org:9108",
		connected: true,
		inbound:   false,
	}
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetAddedNodeInfo: ok without DNS and without address filter",
		handler: handleGetAddedNodeInfo,
		cmd: &types.GetAddedNodeInfoCmd{
			DNS: false,
		},
		mockConnManager: &testConnManager{
			addedNodeInfo: []rpcserver.Peer{
				testPeer1,
				testPeer2,
			},
		},
		result: []string{"160.221.215.210", "160.221.215.211:9108"},
	}, {
		name:    "handleGetAddedNodeInfo: found without DNS and with address filter",
		handler: handleGetAddedNodeInfo,
		cmd: &types.GetAddedNodeInfoCmd{
			DNS:  false,
			Node: dcrjson.String("160.221.215.211:9108"),
		},
		mockConnManager: &testConnManager{
			addedNodeInfo: []rpcserver.Peer{
				testPeer1,
				testPeer2,
			},
		},
		result: []string{"160.221.215.211:9108"},
	}, {
		name:    "handleGetAddedNodeInfo: node not found",
		handler: handleGetAddedNodeInfo,
		cmd: &types.GetAddedNodeInfoCmd{
			DNS:  false,
			Node: dcrjson.String("160.221.215.212:9108"),
		},
		mockConnManager: &testConnManager{
			addedNodeInfo: []rpcserver.Peer{
				testPeer1,
				testPeer2,
			},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:    "handleGetAddedNodeInfo: ok with DNS and without address filter",
		handler: handleGetAddedNodeInfo,
		cmd: &types.GetAddedNodeInfoCmd{
			DNS: true,
		},
		mockCfg: &config{
			lookup: func(host string) ([]net.IP, error) {
				if host == "mydomain.org" {
					return []net.IP{net.ParseIP("160.221.215.211")}, nil
				}
				return nil, errors.New("host not found")
			},
		},
		mockConnManager: &testConnManager{
			addedNodeInfo: []rpcserver.Peer{
				testPeer1,
				testPeer3,
			},
		},
		result: []*types.GetAddedNodeInfoResult{{
			AddedNode: "160.221.215.210",
			Connected: dcrjson.Bool(true),
			Addresses: &[]types.GetAddedNodeInfoResultAddr{{
				Address:   "160.221.215.210",
				Connected: "inbound",
			}},
		}, {
			AddedNode: "mydomain.org:9108",
			Connected: dcrjson.Bool(true),
			Addresses: &[]types.GetAddedNodeInfoResultAddr{{
				Address:   "160.221.215.211",
				Connected: "false",
			}},
		}},
	}, {
		name:    "handleGetAddedNodeInfo: found with DNS and with address filter",
		handler: handleGetAddedNodeInfo,
		cmd: &types.GetAddedNodeInfoCmd{
			DNS:  true,
			Node: dcrjson.String("mydomain.org:9108"),
		},
		mockCfg: &config{
			lookup: func(host string) ([]net.IP, error) {
				if host == "mydomain.org" {
					return []net.IP{net.ParseIP("160.221.215.211")}, nil
				}
				return nil, errors.New("host not found")
			},
		},
		mockConnManager: &testConnManager{
			addedNodeInfo: []rpcserver.Peer{
				testPeer1,
				testPeer3,
			},
		},
		result: []*types.GetAddedNodeInfoResult{{
			AddedNode: "mydomain.org:9108",
			Connected: dcrjson.Bool(true),
			Addresses: &[]types.GetAddedNodeInfoResultAddr{{
				Address:   "160.221.215.211",
				Connected: "false",
			}},
		}},
	}, {
		name:    "handleGetAddedNodeInfo: ok with DNS lookup failed",
		handler: handleGetAddedNodeInfo,
		cmd: &types.GetAddedNodeInfoCmd{
			DNS: true,
		},
		mockCfg: &config{
			lookup: func(host string) ([]net.IP, error) {
				return nil, errors.New("host not found")
			},
		},
		mockConnManager: &testConnManager{
			addedNodeInfo: []rpcserver.Peer{
				testPeer1,
				testPeer3,
			},
		},
		result: []*types.GetAddedNodeInfoResult{{
			AddedNode: "160.221.215.210",
			Connected: dcrjson.Bool(true),
			Addresses: &[]types.GetAddedNodeInfoResultAddr{{
				Address:   "160.221.215.210",
				Connected: "inbound",
			}},
		}, {
			AddedNode: "mydomain.org:9108",
			Connected: dcrjson.Bool(true),
			Addresses: &[]types.GetAddedNodeInfoResultAddr{{
				Address:   "mydomain.org",
				Connected: "outbound",
			}},
		}},
	}})
}

func TestHandleGetBestBlock(t *testing.T) {
	hash := mustParseHash("000000000000000019e76d2f52f39f9245db35eab21741a61ed5bded310f0c87")
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetBestBlock: ok",
		handler: handleGetBestBlock,
		cmd:     &types.GetBestBlockCmd{},
		mockChain: &testRPCChain{
			bestSnapshot: &blockchain.BestState{
				Hash:   *hash,
				Height: 451802,
			},
		},
		result: &types.GetBestBlockResult{
			Hash:   "000000000000000019e76d2f52f39f9245db35eab21741a61ed5bded310f0c87",
			Height: 451802,
		},
	}})
}

func TestHandleGetBestBlockHash(t *testing.T) {
	hash := mustParseHash("000000000000000019e76d2f52f39f9245db35eab21741a61ed5bded310f0c87")
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetBestBlockHash: ok",
		handler: handleGetBestBlockHash,
		cmd:     &types.GetBestBlockHashCmd{},
		mockChain: &testRPCChain{
			bestSnapshot: &blockchain.BestState{
				Hash: *hash,
			},
		},
		result: "000000000000000019e76d2f52f39f9245db35eab21741a61ed5bded310f0c87",
	}})
}

func TestHandleGetBlockchainInfo(t *testing.T) {
	hash := mustParseHash("00000000000000001e6ec1501c858506de1de4703d1be8bab4061126e8f61480")
	prevHash := mustParseHash("00000000000000001a1ec2becd0dd90bfbd0c65f42fdaf608dd9ceac2a3aee1d")
	genesisHash := mustParseHash("298e5cc3d985bfe7f81dc135f360abe089edd4396b86d2de66b0cef42b21d980")
	genesisPrevHash := mustParseHash("0000000000000000000000000000000000000000000000000000000000000000")

	// Explicitly define the params that handleGetBlockchainInfo depends on so that
	// the tests don't break when the values for these change.
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

	testRPCServerHandler(t, []rpcTest{{
		name:            "handleGetBlockchainInfo: ok",
		handler:         handleGetBlockchainInfo,
		cmd:             &types.GetBlockChainInfoCmd{},
		mockChainParams: testChainParams,
		mockChain: &testRPCChain{
			bestSnapshot: &blockchain.BestState{
				Height:   463073,
				Bits:     404696953,
				Hash:     *hash,
				PrevHash: *prevHash,
			},
			chainWork: big.NewInt(0).SetBytes([]byte{0x11, 0x5d, 0x28, 0x33, 0x84,
				0x90, 0x90, 0xb0, 0x02, 0x65, 0x06}),
			isCurrent:    false,
			maxBlockSize: 393216,
			nextThresholdState: blockchain.ThresholdStateTuple{
				State:  blockchain.ThresholdStarted,
				Choice: uint32(0xffffffff),
			},
			stateLastChangedHeight: int64(149248),
		},
		mockSyncManager: &testSyncManager{
			syncHeight: 463074,
		},
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
		name:            "handleGetBlockchainInfo: ok with empty blockchain",
		handler:         handleGetBlockchainInfo,
		cmd:             &types.GetBlockChainInfoCmd{},
		mockChainParams: testChainParams,
		mockChain: &testRPCChain{
			bestSnapshot: &blockchain.BestState{
				Height:   0,
				Bits:     453115903,
				Hash:     *genesisHash,
				PrevHash: *genesisPrevHash,
			},
			chainWork: big.NewInt(0).SetBytes([]byte{0x80, 0x00, 0x40, 0x00, 0x20,
				0x00}),
			isCurrent:    false,
			maxBlockSize: 393216,
			nextThresholdState: blockchain.ThresholdStateTuple{
				State:  blockchain.ThresholdDefined,
				Choice: uint32(0xffffffff),
			},
			stateLastChangedHeight: int64(0),
		},
		mockSyncManager: &testSyncManager{
			syncHeight: 0,
		},
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
		name:            "handleGetBlockchainInfo: could not fetch chain work",
		handler:         handleGetBlockchainInfo,
		cmd:             &types.GetBlockChainInfoCmd{},
		mockChainParams: testChainParams,
		mockChain: &testRPCChain{
			bestSnapshot: &blockchain.BestState{
				Hash: *hash,
			},
			chainWorkErr: errors.New("could not fetch chain work"),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:            "handleGetBlockchainInfo: could not fetch max block size",
		handler:         handleGetBlockchainInfo,
		cmd:             &types.GetBlockChainInfoCmd{},
		mockChainParams: testChainParams,
		mockChain: &testRPCChain{
			bestSnapshot: &blockchain.BestState{
				Hash: *hash,
			},
			chainWork: big.NewInt(0).SetBytes([]byte{0x11, 0x5d, 0x28, 0x33, 0x84,
				0x90, 0x90, 0xb0, 0x02, 0x65, 0x06}),
			maxBlockSizeErr: errors.New("could not fetch max block size"),
		},
		mockSyncManager: &testSyncManager{
			syncHeight: 463074,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:            "handleGetBlockchainInfo: could not fetch threshold state",
		handler:         handleGetBlockchainInfo,
		cmd:             &types.GetBlockChainInfoCmd{},
		mockChainParams: testChainParams,
		mockChain: &testRPCChain{
			bestSnapshot: &blockchain.BestState{
				Height:   463073,
				Bits:     404696953,
				Hash:     *hash,
				PrevHash: *prevHash,
			},
			chainWork: big.NewInt(0).SetBytes([]byte{0x11, 0x5d, 0x28, 0x33, 0x84,
				0x90, 0x90, 0xb0, 0x02, 0x65, 0x06}),
			maxBlockSize:          393216,
			nextThresholdStateErr: errors.New("could not fetch threshold state"),
		},
		mockSyncManager: &testSyncManager{
			syncHeight: 463074,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}, {
		name:            "handleGetBlockchainInfo: could not fetch state last changed",
		handler:         handleGetBlockchainInfo,
		cmd:             &types.GetBlockChainInfoCmd{},
		mockChainParams: testChainParams,
		mockChain: &testRPCChain{
			bestSnapshot: &blockchain.BestState{
				Height:   463073,
				Bits:     404696953,
				Hash:     *hash,
				PrevHash: *prevHash,
			},
			chainWork: big.NewInt(0).SetBytes([]byte{0x11, 0x5d, 0x28, 0x33, 0x84,
				0x90, 0x90, 0xb0, 0x02, 0x65, 0x06}),
			maxBlockSize: 393216,
			nextThresholdState: blockchain.ThresholdStateTuple{
				State:  blockchain.ThresholdStarted,
				Choice: uint32(0xffffffff),
			},
			stateLastChangedHeightErr: errors.New("could not fetch state last changed"),
		},
		mockSyncManager: &testSyncManager{
			syncHeight: 463074,
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInternal.Code,
	}})
}

func TestHandleGetBlockCount(t *testing.T) {
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetBlockCount: ok",
		handler: handleGetBlockCount,
		cmd:     &types.GetBlockCountCmd{},
		mockChain: &testRPCChain{
			bestSnapshot: &blockchain.BestState{
				Height: 451802,
			},
		},
		result: int64(451802),
	}})
}

func TestHandleGetChainTips(t *testing.T) {
	h1 := mustParseHash("000000000000000002e4e275720a511cc4c6e881ac7aa94f6786e496d0901e5c")
	h2 := mustParseHash("00000000000000000cc40fe6f2fe9a0c482281d79f1b49c3c77b976859edd963")
	h3 := mustParseHash("00000000000000000afa4a9c11c4106aac0c73f595182227e78688218f3516f1")
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetChainTips: ok",
		handler: handleGetChainTips,
		cmd:     &types.GetChainTipsCmd{},
		mockChain: &testRPCChain{
			chainTips: []blockchain.ChainTipInfo{{
				Height:    453336,
				Hash:      *h1,
				BranchLen: 0,
				Status:    "active",
			}, {
				Height:    450576,
				Hash:      *h2,
				BranchLen: 1,
				Status:    "valid-headers",
			}, {
				Height:    449982,
				Hash:      *h3,
				BranchLen: 1,
				Status:    "valid-headers",
			}},
		},
		result: []types.GetChainTipsResult{{
			Height:    453336,
			Hash:      "000000000000000002e4e275720a511cc4c6e881ac7aa94f6786e496d0901e5c",
			BranchLen: 0,
			Status:    "active",
		}, {
			Height:    450576,
			Hash:      "00000000000000000cc40fe6f2fe9a0c482281d79f1b49c3c77b976859edd963",
			BranchLen: 1,
			Status:    "valid-headers",
		}, {
			Height:    449982,
			Hash:      "00000000000000000afa4a9c11c4106aac0c73f595182227e78688218f3516f1",
			BranchLen: 1,
			Status:    "valid-headers",
		}},
	}})
}

func TestHandleGetCoinSupply(t *testing.T) {
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetCoinSupply: ok",
		handler: handleGetCoinSupply,
		cmd:     &types.GetCoinSupplyCmd{},
		mockChain: &testRPCChain{
			bestSnapshot: &blockchain.BestState{
				TotalSubsidy: 1152286647709751,
			},
		},
		result: int64(1152286647709751),
	}})
}

func TestHandleGetConnectionCount(t *testing.T) {
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetConnectionCount: ok",
		handler: handleGetConnectionCount,
		cmd:     &types.GetConnectionCountCmd{},
		mockConnManager: &testConnManager{
			connectedCount: 7,
		},
		result: int32(7),
	}})
}

func TestHandleGetCurrentNet(t *testing.T) {
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetCurrentNet: ok",
		handler: handleGetCurrentNet,
		cmd:     &types.GetCurrentNetCmd{},
		result:  wire.MainNet,
	}})
}

func TestHandleGetInfo(t *testing.T) {
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetInfo: ok",
		handler: handleGetInfo,
		cmd:     &types.GetInfoCmd{},
		mockConnManager: &testConnManager{
			connectedCount: 7,
		},
		mockChain: &testRPCChain{
			bestSnapshot: &blockchain.BestState{
				Bits:   493007795,
				Height: 449708,
			},
		},
		mockCfg: &config{
			AddrIndex:     false,
			Proxy:         "",
			TestNet:       false,
			TxIndex:       false,
			minRelayTxFee: dcrutil.Amount(int64(10000)),
		},
		result: &types.InfoChainResult{
			Version: int32(1000000*version.Major + 10000*version.Minor +
				100*version.Patch),
			ProtocolVersion: int32(maxProtocolVersion),
			Blocks:          int64(449708),
			TimeOffset:      int64(0),
			Connections:     int32(7),
			Proxy:           "",
			Difficulty:      float64(0.01013136),
			TestNet:         false,
			RelayFee:        float64(0.0001),
			AddrIndex:       false,
			TxIndex:         false,
		},
	}})
}

func TestHandleGetNetTotals(t *testing.T) {
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetNetTotals: ok",
		handler: handleGetNetTotals,
		cmd:     &types.GetNetTotalsCmd{},
		mockConnManager: &testConnManager{
			netTotalReceived: uint64(9598159),
			netTotalSent:     uint64(4783802),
		},
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
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetNetworkInfo: ok",
		handler: handleGetNetworkInfo,
		cmd:     &types.GetNetworkInfoCmd{},
		mockAddrManager: &testAddrManager{
			localAddresses: []addrmgr.LocalAddr{{
				Address: "45.56.142.184",
				Port:    uint16(19108),
				Score:   int32(0),
			}},
		},
		mockConnManager: &testConnManager{
			connectedCount: 7,
		},
		mockCfg: &config{
			minRelayTxFee: dcrutil.Amount(int64(10000)),
			ipv4NetInfo: types.NetworksResult{
				Name:                      "IPV4",
				Limited:                   false,
				Reachable:                 true,
				Proxy:                     "",
				ProxyRandomizeCredentials: false,
			},
			ipv6NetInfo: types.NetworksResult{
				Name:                      "IPV6",
				Limited:                   false,
				Reachable:                 true,
				Proxy:                     "",
				ProxyRandomizeCredentials: false,
			},
			onionNetInfo: types.NetworksResult{
				Name:                      "Onion",
				Limited:                   false,
				Reachable:                 false,
				Proxy:                     "",
				ProxyRandomizeCredentials: false,
			},
		},
		result: types.GetNetworkInfoResult{
			Version: int32(1000000*version.Major + 10000*version.Minor +
				100*version.Patch),
			SubVersion:      userAgentVersion,
			ProtocolVersion: int32(maxProtocolVersion),
			TimeOffset:      int64(0),
			Connections:     int32(7),
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
				Address: "45.56.142.184",
				Port:    uint16(19108),
				Score:   int32(0),
			}},
			LocalServices: "0000000000000005",
		},
	}})
}

func TestHandleGetPeerInfo(t *testing.T) {
	testRPCServerHandler(t, []rpcTest{{
		name:            "handleGetPeerInfo: ok",
		handler:         handleGetPeerInfo,
		cmd:             &types.GetPeerInfoCmd{},
		mockSyncManager: &testSyncManager{},
		mockConnManager: &testConnManager{
			connectedPeers: []rpcserver.Peer{
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
			},
		},
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

func TestHandleGetTxOutSetInfo(t *testing.T) {
	hash := mustParseHash("000000000000000019e76d2f52f39f9245db35eab21741a61ed5bded310f0c87")
	sHash := mustParseHash("fe7b32aa188800f07268b17f3bead5f3d8a1b6d18654182066436efce6effa86")
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleGetTxOutSetInfo: ok",
		handler: handleGetTxOutSetInfo,
		cmd:     &types.GetTxOutSetInfoCmd{},
		mockChain: &testRPCChain{
			bestSnapshot: &blockchain.BestState{
				Hash:   *hash,
				Height: 451802,
			},
			fetchUtxoStats: &blockchain.UtxoStats{
				Utxos:          1593879,
				Transactions:   689819,
				Size:           36441617,
				Total:          1154067750680149,
				SerializedHash: *sHash,
			},
		},
		result: types.GetTxOutSetInfoResult{
			Height:         451802,
			BestBlock:      "000000000000000019e76d2f52f39f9245db35eab21741a61ed5bded310f0c87",
			Transactions:   689819,
			TxOuts:         1593879,
			SerializedHash: "fe7b32aa188800f07268b17f3bead5f3d8a1b6d18654182066436efce6effa86",
			DiskSize:       36441617,
			TotalAmount:    1154067750680149,
		},
	}})
}

func TestHandleNode(t *testing.T) {
	testRPCServerHandler(t, []rpcTest{{
		name:    "handleNode: ok with disconnect by address",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "disconnect",
			Target: "160.221.215.210:9108",
		},
		mockConnManager: &testConnManager{},
		result:          nil,
	}, {
		name:    "handleNode: disconnect by address peer not found",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "disconnect",
			Target: "160.221.215.210:9108",
		},
		mockConnManager: &testConnManager{
			disconnectByAddrErr: errors.New("peer not found"),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleNode: ok with disconnect by id",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "disconnect",
			Target: "28",
		},
		mockConnManager: &testConnManager{},
		result:          nil,
	}, {
		name:    "handleNode: disconnect by id peer not found",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "disconnect",
			Target: "28",
		},
		mockConnManager: &testConnManager{
			disconnectByIDErr: errors.New("peer not found"),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleNode: disconnect invalid address",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "disconnect",
			Target: "invalid_address",
		},
		mockConnManager: &testConnManager{},
		wantErr:         true,
		errCode:         dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleNode: can't disconnect a permanent peer",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "disconnect",
			Target: "160.221.215.210:9108",
		},
		mockConnManager: &testConnManager{
			disconnectByAddrErr: errors.New("peer not found"),
			connectedPeers: []rpcserver.Peer{
				&testPeer{
					id:   28,
					addr: "160.221.215.210:9108",
				},
			},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCMisc,
	}, {
		name:    "handleNode: ok with remove by address",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "remove",
			Target: "160.221.215.210:9108",
		},
		mockConnManager: &testConnManager{},
		result:          nil,
	}, {
		name:    "handleNode: remove by address peer not found",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "remove",
			Target: "160.221.215.210:9108",
		},
		mockConnManager: &testConnManager{
			removeByAddrErr: errors.New("peer not found"),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleNode: ok with remove by id",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "remove",
			Target: "28",
		},
		mockConnManager: &testConnManager{},
		result:          nil,
	}, {
		name:    "handleNode: remove by id peer not found",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "remove",
			Target: "28",
		},
		mockConnManager: &testConnManager{
			removeByIDErr: errors.New("peer not found"),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleNode: remove invalid address",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "remove",
			Target: "invalid_address",
		},
		mockConnManager: &testConnManager{},
		wantErr:         true,
		errCode:         dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleNode: can't remove a temporary peer",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "remove",
			Target: "160.221.215.210:9108",
		},
		mockConnManager: &testConnManager{
			removeByAddrErr: errors.New("peer not found"),
			connectedPeers: []rpcserver.Peer{
				&testPeer{
					id:   28,
					addr: "160.221.215.210:9108",
				},
			},
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCMisc,
	}, {
		name:    "handleNode: ok with connect perm",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd:        "connect",
			Target:        "160.221.215.210:9108",
			ConnectSubCmd: dcrjson.String("perm"),
		},
		mockConnManager: &testConnManager{},
		result:          nil,
	}, {
		name:    "handleNode: ok with connect temp",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd:        "connect",
			Target:        "160.221.215.210:9108",
			ConnectSubCmd: dcrjson.String("temp"),
		},
		mockConnManager: &testConnManager{},
		result:          nil,
	}, {
		name:    "handleNode: invalid connect sub cmd",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd:        "connect",
			Target:        "160.221.215.210:9108",
			ConnectSubCmd: dcrjson.String("invalid"),
		},
		mockConnManager: &testConnManager{},
		wantErr:         true,
		errCode:         dcrjson.ErrRPCInvalidParameter,
	}, {
		name:    "handleNode: invalid sub cmd",
		handler: handleNode,
		cmd: &types.NodeCmd{
			SubCmd: "invalid",
			Target: "160.221.215.210:9108",
		},
		mockConnManager: &testConnManager{},
		wantErr:         true,
		errCode:         dcrjson.ErrRPCInvalidParameter,
	}})
}

func TestHandlePing(t *testing.T) {
	testRPCServerHandler(t, []rpcTest{{
		name:            "handlePing: ok",
		handler:         handlePing,
		cmd:             &types.PingCmd{},
		mockConnManager: &testConnManager{},
		result:          nil,
	}})
}

func testRPCServerHandler(t *testing.T, tests []rpcTest) {
	t.Helper()

	for _, test := range tests {
		cfg = test.mockCfg
		chainParams := chaincfg.MainNetParams()
		if test.mockChainParams != nil {
			chainParams = test.mockChainParams
		}
		testServer := &rpcServer{
			cfg: rpcserverConfig{
				ChainParams: chainParams,
				Chain:       test.mockChain,
				AddrManager: test.mockAddrManager,
				SyncMgr:     test.mockSyncManager,
				ConnMgr:     test.mockConnManager,
				Clock:       test.mockClock,
				TimeSource:  blockchain.NewMedianTime(),
				Services:    wire.SFNodeNetwork | wire.SFNodeCF,
			},
		}
		result, err := test.handler(nil, testServer, test.cmd)
		if test.wantErr {
			var rpcErr *dcrjson.RPCError
			if !errors.As(err, &rpcErr) || rpcErr.Code != test.errCode {
				t.Errorf("%s\nwant: %+v\n got: %+v\n", test.name, test.errCode, rpcErr.Code)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s\nunexpected error: %+v\n", test.name, err)
		}
		if !reflect.DeepEqual(result, test.result) {
			t.Errorf("%s\nwant: %+v\n got: %+v\n", test.name, test.result, result)
		}
	}
}
