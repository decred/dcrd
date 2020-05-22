// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"testing"

	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/blockchain/v3"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrjson/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/gcs/v2"
	"github.com/decred/dcrd/internal/rpcserver"
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
	missedTickets                   []chainhash.Hash
	nextThresholdState              blockchain.ThresholdStateTuple
	stateLastChangedHeight          int64
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
	return c.chainWork, nil
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
	return c.maxBlockSize, nil
}

// MissedTickets returns a mocked slice of all currently missed tickets.
func (c *testRPCChain) MissedTickets() ([]chainhash.Hash, error) {
	return c.missedTickets, nil
}

// NextThresholdState returns a mocked current rule change threshold state of
// the given deployment ID for the block AFTER the provided block hash.
func (c *testRPCChain) NextThresholdState(hash *chainhash.Hash, version uint32, deploymentID string) (blockchain.ThresholdStateTuple, error) {
	return c.nextThresholdState, nil
}

// StateLastChangedHeight returns a mocked height at which the provided
// consensus deployment agenda last changed state.
func (c *testRPCChain) StateLastChangedHeight(hash *chainhash.Hash, version uint32, deploymentID string) (int64, error) {
	return c.stateLastChangedHeight, nil
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

type rpcTest struct {
	name      string
	handler   commandHandler
	cmd       interface{}
	mockChain *testRPCChain
	result    interface{}
	wantErr   bool
	errCode   dcrjson.RPCErrorCode
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

func TestHandleGetBestBlock(t *testing.T) {
	hash, _ := chainhash.NewHashFromStr("000000000000000019e76d2f52f39f9245db35eab21741a61ed5bded310f0c87")
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
	hash, _ := chainhash.NewHashFromStr("000000000000000019e76d2f52f39f9245db35eab21741a61ed5bded310f0c87")
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
	h1, _ := chainhash.NewHashFromStr("000000000000000002e4e275720a511cc4c6e881ac7aa94f6786e496d0901e5c")
	h2, _ := chainhash.NewHashFromStr("00000000000000000cc40fe6f2fe9a0c482281d79f1b49c3c77b976859edd963")
	h3, _ := chainhash.NewHashFromStr("00000000000000000afa4a9c11c4106aac0c73f595182227e78688218f3516f1")
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

func TestHandleGetTxOutSetInfo(t *testing.T) {
	hash, _ := chainhash.NewHashFromStr("000000000000000019e76d2f52f39f9245db35eab21741a61ed5bded310f0c87")
	sHash, _ := chainhash.NewHashFromStr("fe7b32aa188800f07268b17f3bead5f3d8a1b6d18654182066436efce6effa86")
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

func testRPCServerHandler(t *testing.T, tests []rpcTest) {
	t.Helper()

	for _, test := range tests {
		testServer := &rpcServer{
			cfg: rpcserverConfig{
				ChainParams: chaincfg.MainNetParams(),
				Chain:       test.mockChain,
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
