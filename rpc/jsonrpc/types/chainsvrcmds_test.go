// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2016-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/decred/dcrd/dcrjson/v4"
)

// TestChainSvrCmds tests all of the chain server commands marshal and unmarshal
// into valid results include handling of optional fields being omitted in the
// marshalled command, while optional fields with defaults have the default
// assigned on unmarshalled commands.
func TestChainSvrCmds(t *testing.T) {
	t.Parallel()

	testID := int(1)
	tests := []struct {
		name         string
		newCmd       func() (interface{}, error)
		staticCmd    func() interface{}
		marshalled   string
		unmarshalled interface{}
	}{
		{
			name: "addnode",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("addnode"), "127.0.0.1", ANRemove)
			},
			staticCmd: func() interface{} {
				return NewAddNodeCmd("127.0.0.1", ANRemove)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"addnode","params":["127.0.0.1","remove"],"id":1}`,
			unmarshalled: &AddNodeCmd{Addr: "127.0.0.1", SubCmd: ANRemove},
		},
		{
			name: "createrawtransaction",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("createrawtransaction"), `[{"amount":0.0123,"txid":"123","vout":1}]`,
					`{"456":0.0123}`)
			},
			staticCmd: func() interface{} {
				txInputs := []TransactionInput{
					{Amount: 0.0123, Txid: "123", Vout: 1},
				}
				amounts := map[string]float64{"456": .0123}
				return NewCreateRawTransactionCmd(txInputs, amounts, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"createrawtransaction","params":[[{"amount":0.0123,"txid":"123","vout":1,"tree":0}],{"456":0.0123}],"id":1}`,
			unmarshalled: &CreateRawTransactionCmd{
				Inputs:  []TransactionInput{{Amount: 0.0123, Txid: "123", Vout: 1}},
				Amounts: map[string]float64{"456": .0123},
			},
		},
		{
			name: "createrawtransaction optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("createrawtransaction"), `[{"amount":0.0123,"txid":"123","vout":1,"tree":0}]`,
					`{"456":0.0123}`, int64(12312333333), int64(12312333333))
			},
			staticCmd: func() interface{} {
				txInputs := []TransactionInput{
					{Amount: 0.0123, Txid: "123", Vout: 1},
				}
				amounts := map[string]float64{"456": .0123}
				return NewCreateRawTransactionCmd(txInputs, amounts, dcrjson.Int64(12312333333), dcrjson.Int64(12312333333))
			},
			marshalled: `{"jsonrpc":"1.0","method":"createrawtransaction","params":[[{"amount":0.0123,"txid":"123","vout":1,"tree":0}],{"456":0.0123},12312333333,12312333333],"id":1}`,
			unmarshalled: &CreateRawTransactionCmd{
				Inputs:   []TransactionInput{{Amount: 0.0123, Txid: "123", Vout: 1}},
				Amounts:  map[string]float64{"456": .0123},
				LockTime: dcrjson.Int64(12312333333),
				Expiry:   dcrjson.Int64(12312333333),
			},
		},
		{
			name: "debuglevel",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("debuglevel"), "trace")
			},
			staticCmd: func() interface{} {
				return NewDebugLevelCmd("trace")
			},
			marshalled: `{"jsonrpc":"1.0","method":"debuglevel","params":["trace"],"id":1}`,
			unmarshalled: &DebugLevelCmd{
				LevelSpec: "trace",
			},
		},
		{
			name: "decoderawtransaction",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("decoderawtransaction"), "123")
			},
			staticCmd: func() interface{} {
				return NewDecodeRawTransactionCmd("123")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"decoderawtransaction","params":["123"],"id":1}`,
			unmarshalled: &DecodeRawTransactionCmd{HexTx: "123"},
		},
		{
			name: "decodescript",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("decodescript"), "00")
			},
			staticCmd: func() interface{} {
				return NewDecodeScriptCmd("00")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"decodescript","params":["00"],"id":1}`,
			unmarshalled: &DecodeScriptCmd{HexScript: "00"},
		},
		{
			name: "decodescript optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("decodescript"), "00", dcrjson.Uint16(1))
			},
			staticCmd: func() interface{} {
				cmd := NewDecodeScriptCmd("00")
				cmd.Version = dcrjson.Uint16(1)
				return cmd
			},
			marshalled:   `{"jsonrpc":"1.0","method":"decodescript","params":["00",1],"id":1}`,
			unmarshalled: &DecodeScriptCmd{HexScript: "00", Version: dcrjson.Uint16(1)},
		},
		{
			name: "estimatefee",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("estimatefee"), 6)
			},
			staticCmd: func() interface{} {
				return NewEstimateFeeCmd(6)
			},
			marshalled: `{"jsonrpc":"1.0","method":"estimatefee","params":[6],"id":1}`,
			unmarshalled: &EstimateFeeCmd{
				NumBlocks: 6,
			},
		},
		{
			name: "estimatesmartfee",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("estimatesmartfee"), 6)
			},
			staticCmd: func() interface{} {
				return NewEstimateSmartFeeCmd(6, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"estimatesmartfee","params":[6],"id":1}`,
			unmarshalled: &EstimateSmartFeeCmd{
				Confirmations: 6,
				Mode:          EstimateSmartFeeModeAddr(EstimateSmartFeeConservative),
			},
		},
		{
			name: "estimatesmartfee optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("estimatesmartfee"), 6, EstimateSmartFeeConservative)
			},
			staticCmd: func() interface{} {
				mode := EstimateSmartFeeConservative
				return NewEstimateSmartFeeCmd(6, &mode)
			},
			marshalled: `{"jsonrpc":"1.0","method":"estimatesmartfee","params":[6,"conservative"],"id":1}`,
			unmarshalled: &EstimateSmartFeeCmd{
				Confirmations: 6,
				Mode:          EstimateSmartFeeModeAddr(EstimateSmartFeeConservative),
			},
		},
		{
			name: "generate",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("generate"), 1)
			},
			staticCmd: func() interface{} {
				return NewGenerateCmd(1)
			},
			marshalled: `{"jsonrpc":"1.0","method":"generate","params":[1],"id":1}`,
			unmarshalled: &GenerateCmd{
				NumBlocks: 1,
			},
		},
		{
			name: "getaddednodeinfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getaddednodeinfo"), true)
			},
			staticCmd: func() interface{} {
				return NewGetAddedNodeInfoCmd(true, nil)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getaddednodeinfo","params":[true],"id":1}`,
			unmarshalled: &GetAddedNodeInfoCmd{DNS: true, Node: nil},
		},
		{
			name: "getaddednodeinfo optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getaddednodeinfo"), true, "127.0.0.1")
			},
			staticCmd: func() interface{} {
				return NewGetAddedNodeInfoCmd(true, dcrjson.String("127.0.0.1"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getaddednodeinfo","params":[true,"127.0.0.1"],"id":1}`,
			unmarshalled: &GetAddedNodeInfoCmd{
				DNS:  true,
				Node: dcrjson.String("127.0.0.1"),
			},
		},
		{
			name: "getbestblock",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getbestblock"))
			},
			staticCmd: func() interface{} {
				return NewGetBestBlockCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getbestblock","params":[],"id":1}`,
			unmarshalled: &GetBestBlockCmd{},
		},
		{
			name: "getbestblockhash",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getbestblockhash"))
			},
			staticCmd: func() interface{} {
				return NewGetBestBlockHashCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getbestblockhash","params":[],"id":1}`,
			unmarshalled: &GetBestBlockHashCmd{},
		},
		{
			name: "getblock",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getblock"), "123")
			},
			staticCmd: func() interface{} {
				return NewGetBlockCmd("123", nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblock","params":["123"],"id":1}`,
			unmarshalled: &GetBlockCmd{
				Hash:      "123",
				Verbose:   dcrjson.Bool(true),
				VerboseTx: dcrjson.Bool(false),
			},
		},
		{
			name: "getblock required optional1",
			newCmd: func() (interface{}, error) {
				// Intentionally use a source param that is
				// more pointers than the destination to
				// exercise that path.
				verbosePtr := dcrjson.Bool(true)
				return dcrjson.NewCmd(Method("getblock"), "123", &verbosePtr)
			},
			staticCmd: func() interface{} {
				return NewGetBlockCmd("123", dcrjson.Bool(true), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblock","params":["123",true],"id":1}`,
			unmarshalled: &GetBlockCmd{
				Hash:      "123",
				Verbose:   dcrjson.Bool(true),
				VerboseTx: dcrjson.Bool(false),
			},
		},
		{
			name: "getblock required optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getblock"), "123", true, true)
			},
			staticCmd: func() interface{} {
				return NewGetBlockCmd("123", dcrjson.Bool(true), dcrjson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblock","params":["123",true,true],"id":1}`,
			unmarshalled: &GetBlockCmd{
				Hash:      "123",
				Verbose:   dcrjson.Bool(true),
				VerboseTx: dcrjson.Bool(true),
			},
		},
		{
			name: "getblockchaininfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getblockchaininfo"))
			},
			staticCmd: func() interface{} {
				return NewGetBlockChainInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockchaininfo","params":[],"id":1}`,
			unmarshalled: &GetBlockChainInfoCmd{},
		},
		{
			name: "getblockcount",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getblockcount"))
			},
			staticCmd: func() interface{} {
				return NewGetBlockCountCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockcount","params":[],"id":1}`,
			unmarshalled: &GetBlockCountCmd{},
		},
		{
			name: "getblockhash",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getblockhash"), 123)
			},
			staticCmd: func() interface{} {
				return NewGetBlockHashCmd(123)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockhash","params":[123],"id":1}`,
			unmarshalled: &GetBlockHashCmd{Index: 123},
		},
		{
			name: "getblockheader",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getblockheader"), "123")
			},
			staticCmd: func() interface{} {
				return NewGetBlockHeaderCmd("123", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblockheader","params":["123"],"id":1}`,
			unmarshalled: &GetBlockHeaderCmd{
				Hash:    "123",
				Verbose: dcrjson.Bool(true),
			},
		},
		{
			name: "getblocksubsidy",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getblocksubsidy"), 123, 256)
			},
			staticCmd: func() interface{} {
				return NewGetBlockSubsidyCmd(123, 256)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblocksubsidy","params":[123,256],"id":1}`,
			unmarshalled: &GetBlockSubsidyCmd{
				Height: 123,
				Voters: 256,
			},
		},
		{
			name: "getcfilterv2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getcfilterv2"), "123")
			},
			staticCmd: func() interface{} {
				return NewGetCFilterV2Cmd("123")
			},
			marshalled: `{"jsonrpc":"1.0","method":"getcfilterv2","params":["123"],"id":1}`,
			unmarshalled: &GetCFilterV2Cmd{
				BlockHash: "123",
			},
		},
		{
			name: "getchaintips",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getchaintips"))
			},
			staticCmd: func() interface{} {
				return NewGetChainTipsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getchaintips","params":[],"id":1}`,
			unmarshalled: &GetChainTipsCmd{},
		},
		{
			name: "getconnectioncount",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getconnectioncount"))
			},
			staticCmd: func() interface{} {
				return NewGetConnectionCountCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getconnectioncount","params":[],"id":1}`,
			unmarshalled: &GetConnectionCountCmd{},
		},
		{
			name: "getcurrentnet",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getcurrentnet"))
			},
			staticCmd: func() interface{} {
				return NewGetCurrentNetCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getcurrentnet","params":[],"id":1}`,
			unmarshalled: &GetCurrentNetCmd{},
		},
		{
			name: "getdifficulty",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getdifficulty"))
			},
			staticCmd: func() interface{} {
				return NewGetDifficultyCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getdifficulty","params":[],"id":1}`,
			unmarshalled: &GetDifficultyCmd{},
		},
		{
			name: "getgenerate",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getgenerate"))
			},
			staticCmd: func() interface{} {
				return NewGetGenerateCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getgenerate","params":[],"id":1}`,
			unmarshalled: &GetGenerateCmd{},
		},
		{
			name: "gethashespersec",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("gethashespersec"))
			},
			staticCmd: func() interface{} {
				return NewGetHashesPerSecCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"gethashespersec","params":[],"id":1}`,
			unmarshalled: &GetHashesPerSecCmd{},
		},
		{
			name: "getinfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getinfo"))
			},
			staticCmd: func() interface{} {
				return NewGetInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getinfo","params":[],"id":1}`,
			unmarshalled: &GetInfoCmd{},
		},
		{
			name: "getmempoolinfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getmempoolinfo"))
			},
			staticCmd: func() interface{} {
				return NewGetMempoolInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getmempoolinfo","params":[],"id":1}`,
			unmarshalled: &GetMempoolInfoCmd{},
		},
		{
			name: "getmininginfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getmininginfo"))
			},
			staticCmd: func() interface{} {
				return NewGetMiningInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getmininginfo","params":[],"id":1}`,
			unmarshalled: &GetMiningInfoCmd{},
		},
		{
			name: "getmixpairrequests",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getmixpairrequests"))
			},
			staticCmd: func() interface{} {
				return NewGetMixPairRequestsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getmixpairrequests","params":[],"id":1}`,
			unmarshalled: &GetMixPairRequestsCmd{},
		},
		{
			name: "getnetworkinfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getnetworkinfo"))
			},
			staticCmd: func() interface{} {
				return NewGetNetworkInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getnetworkinfo","params":[],"id":1}`,
			unmarshalled: &GetNetworkInfoCmd{},
		},
		{
			name: "getnettotals",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getnettotals"))
			},
			staticCmd: func() interface{} {
				return NewGetNetTotalsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getnettotals","params":[],"id":1}`,
			unmarshalled: &GetNetTotalsCmd{},
		},
		{
			name: "getnetworkhashps",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getnetworkhashps"))
			},
			staticCmd: func() interface{} {
				return NewGetNetworkHashPSCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnetworkhashps","params":[],"id":1}`,
			unmarshalled: &GetNetworkHashPSCmd{
				Blocks: dcrjson.Int(120),
				Height: dcrjson.Int(-1),
			},
		},
		{
			name: "getnetworkhashps optional1",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getnetworkhashps"), 200)
			},
			staticCmd: func() interface{} {
				return NewGetNetworkHashPSCmd(dcrjson.Int(200), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnetworkhashps","params":[200],"id":1}`,
			unmarshalled: &GetNetworkHashPSCmd{
				Blocks: dcrjson.Int(200),
				Height: dcrjson.Int(-1),
			},
		},
		{
			name: "getnetworkhashps optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getnetworkhashps"), 200, 123)
			},
			staticCmd: func() interface{} {
				return NewGetNetworkHashPSCmd(dcrjson.Int(200), dcrjson.Int(123))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnetworkhashps","params":[200,123],"id":1}`,
			unmarshalled: &GetNetworkHashPSCmd{
				Blocks: dcrjson.Int(200),
				Height: dcrjson.Int(123),
			},
		},
		{
			name: "getpeerinfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getpeerinfo"))
			},
			staticCmd: func() interface{} {
				return NewGetPeerInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getpeerinfo","params":[],"id":1}`,
			unmarshalled: &GetPeerInfoCmd{},
		},
		{
			name: "getrawmempool",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getrawmempool"))
			},
			staticCmd: func() interface{} {
				return NewGetRawMempoolCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawmempool","params":[],"id":1}`,
			unmarshalled: &GetRawMempoolCmd{
				Verbose: dcrjson.Bool(false),
			},
		},
		{
			name: "getrawmempool optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getrawmempool"), false)
			},
			staticCmd: func() interface{} {
				return NewGetRawMempoolCmd(dcrjson.Bool(false), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawmempool","params":[false],"id":1}`,
			unmarshalled: &GetRawMempoolCmd{
				Verbose: dcrjson.Bool(false),
			},
		},
		{
			name: "getrawmempool optional 2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getrawmempool"), false, "all")
			},
			staticCmd: func() interface{} {
				return NewGetRawMempoolCmd(dcrjson.Bool(false), dcrjson.String("all"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawmempool","params":[false,"all"],"id":1}`,
			unmarshalled: &GetRawMempoolCmd{
				Verbose: dcrjson.Bool(false),
				TxType:  dcrjson.String("all"),
			},
		},
		{
			name: "getrawtransaction",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getrawtransaction"), "123")
			},
			staticCmd: func() interface{} {
				return NewGetRawTransactionCmd("123", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawtransaction","params":["123"],"id":1}`,
			unmarshalled: &GetRawTransactionCmd{
				Txid:    "123",
				Verbose: dcrjson.Int(0),
			},
		},
		{
			name: "getrawtransaction optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getrawtransaction"), "123", 1)
			},
			staticCmd: func() interface{} {
				return NewGetRawTransactionCmd("123", dcrjson.Int(1))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawtransaction","params":["123",1],"id":1}`,
			unmarshalled: &GetRawTransactionCmd{
				Txid:    "123",
				Verbose: dcrjson.Int(1),
			},
		},
		{
			name: "getstakeversions",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getstakeversions"), "deadbeef", 1)
			},
			staticCmd: func() interface{} {
				return NewGetStakeVersionsCmd("deadbeef", 1)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getstakeversions","params":["deadbeef",1],"id":1}`,
			unmarshalled: &GetStakeVersionsCmd{
				Hash:  "deadbeef",
				Count: 1,
			},
		},
		{
			name: "gettxout",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("gettxout"), "123", 1, 0)
			},
			staticCmd: func() interface{} {
				return NewGetTxOutCmd("123", 1, 0, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettxout","params":["123",1,0],"id":1}`,
			unmarshalled: &GetTxOutCmd{
				Txid:           "123",
				Vout:           1,
				Tree:           0,
				IncludeMempool: dcrjson.Bool(true),
			},
		},
		{
			name: "gettxout optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("gettxout"), "123", 1, 0, true)
			},
			staticCmd: func() interface{} {
				return NewGetTxOutCmd("123", 1, 0, dcrjson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettxout","params":["123",1,0,true],"id":1}`,
			unmarshalled: &GetTxOutCmd{
				Txid:           "123",
				Vout:           1,
				Tree:           0,
				IncludeMempool: dcrjson.Bool(true),
			},
		},
		{
			name: "gettxoutsetinfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("gettxoutsetinfo"))
			},
			staticCmd: func() interface{} {
				return NewGetTxOutSetInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"gettxoutsetinfo","params":[],"id":1}`,
			unmarshalled: &GetTxOutSetInfoCmd{},
		},
		{
			name: "getvoteinfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getvoteinfo"), 1)
			},
			staticCmd: func() interface{} {
				return NewGetVoteInfoCmd(1)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getvoteinfo","params":[1],"id":1}`,
			unmarshalled: &GetVoteInfoCmd{
				Version: 1,
			},
		},
		{
			name: "gettreasuryspendvotes",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("gettreasuryspendvotes"))
			},
			staticCmd: func() interface{} {
				return NewGetTreasurySpendVotesCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettreasuryspendvotes","params":[],"id":1}`,
			unmarshalled: &GetTreasurySpendVotesCmd{
				Block:   nil,
				TSpends: nil,
			},
		},
		{
			name: "gettreasuryspendvotes data",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("gettreasuryspendvotes"), "123", []string{"456"})
			},
			staticCmd: func() interface{} {
				return NewGetTreasurySpendVotesCmd(dcrjson.String("123"), &[]string{"456"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettreasuryspendvotes","params":["123",["456"]],"id":1}`,
			unmarshalled: &GetTreasurySpendVotesCmd{
				Block:   dcrjson.String("123"),
				TSpends: &[]string{"456"},
			},
		},
		{
			name: "gettreasuryspendvotes empty block",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("gettreasuryspendvotes"), "", []string{"456"})
			},
			staticCmd: func() interface{} {
				return NewGetTreasurySpendVotesCmd(dcrjson.String(""), &[]string{"456"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettreasuryspendvotes","params":["",["456"]],"id":1}`,
			unmarshalled: &GetTreasurySpendVotesCmd{
				Block:   dcrjson.String(""),
				TSpends: &[]string{"456"},
			},
		},
		{
			name: "getwork",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getwork"))
			},
			staticCmd: func() interface{} {
				return NewGetWorkCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getwork","params":[],"id":1}`,
			unmarshalled: &GetWorkCmd{
				Data: nil,
			},
		},
		{
			name: "getwork optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("getwork"), "00112233")
			},
			staticCmd: func() interface{} {
				return NewGetWorkCmd(dcrjson.String("00112233"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getwork","params":["00112233"],"id":1}`,
			unmarshalled: &GetWorkCmd{
				Data: dcrjson.String("00112233"),
			},
		},
		{
			name: "help",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("help"))
			},
			staticCmd: func() interface{} {
				return NewHelpCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"help","params":[],"id":1}`,
			unmarshalled: &HelpCmd{
				Command: nil,
			},
		},
		{
			name: "help optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("help"), "getblock")
			},
			staticCmd: func() interface{} {
				return NewHelpCmd(dcrjson.String("getblock"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"help","params":["getblock"],"id":1}`,
			unmarshalled: &HelpCmd{
				Command: dcrjson.String("getblock"),
			},
		},
		{
			name: "node option remove",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("node"), NRemove, "1.1.1.1")
			},
			staticCmd: func() interface{} {
				return NewNodeCmd("remove", "1.1.1.1", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"node","params":["remove","1.1.1.1"],"id":1}`,
			unmarshalled: &NodeCmd{
				SubCmd: NRemove,
				Target: "1.1.1.1",
			},
		},
		{
			name: "node option disconnect",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("node"), NDisconnect, "1.1.1.1")
			},
			staticCmd: func() interface{} {
				return NewNodeCmd("disconnect", "1.1.1.1", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"node","params":["disconnect","1.1.1.1"],"id":1}`,
			unmarshalled: &NodeCmd{
				SubCmd: NDisconnect,
				Target: "1.1.1.1",
			},
		},
		{
			name: "node option connect",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("node"), NConnect, "1.1.1.1", "perm")
			},
			staticCmd: func() interface{} {
				return NewNodeCmd("connect", "1.1.1.1", dcrjson.String("perm"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"node","params":["connect","1.1.1.1","perm"],"id":1}`,
			unmarshalled: &NodeCmd{
				SubCmd:        NConnect,
				Target:        "1.1.1.1",
				ConnectSubCmd: dcrjson.String("perm"),
			},
		},
		{
			name: "ping",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("ping"))
			},
			staticCmd: func() interface{} {
				return NewPingCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"ping","params":[],"id":1}`,
			unmarshalled: &PingCmd{},
		},
		{
			name: "sendrawmixmessage",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("sendrawmixmessage"), "mixpairrequest", "1122")
			},
			staticCmd: func() interface{} {
				return NewSendRawMixMessageCmd("mixpairrequest", "1122")
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendrawmixmessage","params":["mixpairrequest","1122"],"id":1}`,
			unmarshalled: &SendRawMixMessageCmd{
				Command: "mixpairrequest",
				Message: "1122",
			},
		},
		{
			name: "sendrawtransaction",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("sendrawtransaction"), "1122")
			},
			staticCmd: func() interface{} {
				return NewSendRawTransactionCmd("1122", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendrawtransaction","params":["1122"],"id":1}`,
			unmarshalled: &SendRawTransactionCmd{
				HexTx:         "1122",
				AllowHighFees: dcrjson.Bool(false),
			},
		},
		{
			name: "sendrawtransaction optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("sendrawtransaction"), "1122", false)
			},
			staticCmd: func() interface{} {
				return NewSendRawTransactionCmd("1122", dcrjson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendrawtransaction","params":["1122",false],"id":1}`,
			unmarshalled: &SendRawTransactionCmd{
				HexTx:         "1122",
				AllowHighFees: dcrjson.Bool(false),
			},
		},
		{
			name: "setgenerate",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("setgenerate"), true)
			},
			staticCmd: func() interface{} {
				return NewSetGenerateCmd(true, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"setgenerate","params":[true],"id":1}`,
			unmarshalled: &SetGenerateCmd{
				Generate:     true,
				GenProcLimit: dcrjson.Int(-1),
			},
		},
		{
			name: "setgenerate optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("setgenerate"), true, 6)
			},
			staticCmd: func() interface{} {
				return NewSetGenerateCmd(true, dcrjson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"setgenerate","params":[true,6],"id":1}`,
			unmarshalled: &SetGenerateCmd{
				Generate:     true,
				GenProcLimit: dcrjson.Int(6),
			},
		},
		{
			name: "stop",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("stop"))
			},
			staticCmd: func() interface{} {
				return NewStopCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"stop","params":[],"id":1}`,
			unmarshalled: &StopCmd{},
		},
		{
			name: "submitblock",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("submitblock"), "112233")
			},
			staticCmd: func() interface{} {
				return NewSubmitBlockCmd("112233", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"submitblock","params":["112233"],"id":1}`,
			unmarshalled: &SubmitBlockCmd{
				HexBlock: "112233",
				Options:  nil,
			},
		},
		{
			name: "submitblock optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("submitblock"), "112233", `{"workid":"12345"}`)
			},
			staticCmd: func() interface{} {
				options := SubmitBlockOptions{
					WorkID: "12345",
				}
				return NewSubmitBlockCmd("112233", &options)
			},
			marshalled: `{"jsonrpc":"1.0","method":"submitblock","params":["112233",{"workid":"12345"}],"id":1}`,
			unmarshalled: &SubmitBlockCmd{
				HexBlock: "112233",
				Options: &SubmitBlockOptions{
					WorkID: "12345",
				},
			},
		},
		{
			name: "validateaddress",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("validateaddress"), "1Address")
			},
			staticCmd: func() interface{} {
				return NewValidateAddressCmd("1Address")
			},
			marshalled: `{"jsonrpc":"1.0","method":"validateaddress","params":["1Address"],"id":1}`,
			unmarshalled: &ValidateAddressCmd{
				Address: "1Address",
			},
		},
		{
			name: "verifychain",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("verifychain"))
			},
			staticCmd: func() interface{} {
				return NewVerifyChainCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifychain","params":[],"id":1}`,
			unmarshalled: &VerifyChainCmd{
				CheckLevel: dcrjson.Int64(3),
				CheckDepth: dcrjson.Int64(288),
			},
		},
		{
			name: "verifychain optional1",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("verifychain"), 2)
			},
			staticCmd: func() interface{} {
				return NewVerifyChainCmd(dcrjson.Int64(2), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifychain","params":[2],"id":1}`,
			unmarshalled: &VerifyChainCmd{
				CheckLevel: dcrjson.Int64(2),
				CheckDepth: dcrjson.Int64(288),
			},
		},
		{
			name: "verifychain optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("verifychain"), 2, 500)
			},
			staticCmd: func() interface{} {
				return NewVerifyChainCmd(dcrjson.Int64(2), dcrjson.Int64(500))
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifychain","params":[2,500],"id":1}`,
			unmarshalled: &VerifyChainCmd{
				CheckLevel: dcrjson.Int64(2),
				CheckDepth: dcrjson.Int64(500),
			},
		},
		{
			name: "verifymessage",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("verifymessage"), "1Address", "301234", "test")
			},
			staticCmd: func() interface{} {
				return NewVerifyMessageCmd("1Address", "301234", "test")
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifymessage","params":["1Address","301234","test"],"id":1}`,
			unmarshalled: &VerifyMessageCmd{
				Address:   "1Address",
				Signature: "301234",
				Message:   "test",
			},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Marshal the command as created by the new static command
		// creation function.
		marshalled, err := dcrjson.MarshalCmd("1.0", testID, test.staticCmd())
		if err != nil {
			t.Errorf("MarshalCmd #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		if !bytes.Equal(marshalled, []byte(test.marshalled)) {
			t.Errorf("Test #%d (%s) unexpected marshalled data - "+
				"got %s, want %s", i, test.name, marshalled,
				test.marshalled)
			t.Errorf("\n%s\n%s", marshalled, test.marshalled)
			continue
		}

		// Ensure the command is created without error via the generic
		// new command creation function.
		cmd, err := test.newCmd()
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected dcrjson.NewCmd error: %v",
				i, test.name, err)
		}

		// Marshal the command as created by the generic new command
		// creation function.
		marshalled, err = dcrjson.MarshalCmd("1.0", testID, cmd)
		if err != nil {
			t.Errorf("MarshalCmd #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		if !bytes.Equal(marshalled, []byte(test.marshalled)) {
			t.Errorf("Test #%d (%s) unexpected marshalled data - "+
				"got %s, want %s", i, test.name, marshalled,
				test.marshalled)
			continue
		}

		var request dcrjson.Request
		if err := json.Unmarshal(marshalled, &request); err != nil {
			t.Errorf("Test #%d (%s) unexpected error while "+
				"unmarshalling JSON-RPC request: %v", i,
				test.name, err)
			continue
		}

		cmd, err = dcrjson.ParseParams(Method(request.Method), request.Params)
		if err != nil {
			t.Errorf("ParseParams #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		if !reflect.DeepEqual(cmd, test.unmarshalled) {
			t.Errorf("Test #%d (%s) unexpected unmarshalled command "+
				"- got %s, want %s", i, test.name,
				fmt.Sprintf("(%T) %+[1]v", cmd),
				fmt.Sprintf("(%T) %+[1]v\n", test.unmarshalled))
			continue
		}
	}
}
