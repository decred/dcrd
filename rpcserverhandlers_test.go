// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrjson/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/rpc/jsonrpc/types/v2"
)

// testCfg holds the cfg for the test server.
var testCfg = rpcserverConfig{ChainParams: chaincfg.MainNetParams()}

// testServer is a rpcServer stub used for testing handlers.
var testServer = &rpcServer{cfg: testCfg}

func TestHandleCreateRawTransaction(t *testing.T) {
	tests := []struct {
		cmd          *types.CreateRawTransactionCmd
		name, result string
		wantErr      bool
		errCode      dcrjson.RPCErrorCode
	}{{
		name: "ok",
		cmd: &types.CreateRawTransactionCmd{
			Inputs: []types.TransactionInput{{
				Amount: 1,
				Txid:   "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout:   0,
				Tree:   0,
			}},
			Amounts:  map[string]float64{"DcurAwesomeAddressmqDctW5wJCW1Cn2MF": 1},
			LockTime: dcrjson.Int64(1),
			Expiry:   dcrjson.Int64(1),
		},
		wantErr: false,
		result:  "01000000010d33d3840e9074183dc9a8d82a5031075a98135bfe182840ddaf575aa2032fe00000000000feffffff0100e1f50500000000000017a914f59833f104faa3c7fd0c7dc1e3967fe77a9c15238701000000010000000100e1f5050000000000000000ffffffff00",
	}, {
		name: "expiry out of range",
		cmd: &types.CreateRawTransactionCmd{
			Inputs: []types.TransactionInput{{
				Amount: 1,
				Txid:   "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout:   0,
				Tree:   0,
			}},
			Amounts:  map[string]float64{"DcurAwesomeAddressmqDctW5wJCW1Cn2MF": 1},
			LockTime: dcrjson.Int64(1),
			Expiry:   dcrjson.Int64(-1),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name: "locktime out of range",
		cmd: &types.CreateRawTransactionCmd{
			Inputs: []types.TransactionInput{{
				Amount: 1,
				Txid:   "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout:   0,
				Tree:   0,
			}},
			Amounts:  map[string]float64{"DcurAwesomeAddressmqDctW5wJCW1Cn2MF": 1},
			LockTime: dcrjson.Int64(-1),
			Expiry:   dcrjson.Int64(1),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name: "txid invalid hex",
		cmd: &types.CreateRawTransactionCmd{
			Inputs: []types.TransactionInput{{
				Amount: 1,
				Txid:   "g02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout:   0,
				Tree:   0,
			}},
			Amounts:  map[string]float64{"DcurAwesomeAddressmqDctW5wJCW1Cn2MF": 1},
			LockTime: dcrjson.Int64(1),
			Expiry:   dcrjson.Int64(1),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCDecodeHexString,
	}, {
		name: "invalid tree",
		cmd: &types.CreateRawTransactionCmd{
			Inputs: []types.TransactionInput{{
				Amount: 1,
				Txid:   "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout:   0,
				Tree:   2,
			}},
			Amounts:  map[string]float64{"DcurAwesomeAddressmqDctW5wJCW1Cn2MF": 1},
			LockTime: dcrjson.Int64(1),
			Expiry:   dcrjson.Int64(1),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name: "output over max amount",
		cmd: &types.CreateRawTransactionCmd{
			Inputs: []types.TransactionInput{{
				Amount: (dcrutil.MaxAmount + 1) / 1e8,
				Txid:   "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout:   0,
				Tree:   0,
			}},
			Amounts:  map[string]float64{"DcurAwesomeAddressmqDctW5wJCW1Cn2MF": (dcrutil.MaxAmount + 1) / 1e8},
			LockTime: dcrjson.Int64(1),
			Expiry:   dcrjson.Int64(1),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidParameter,
	}, {
		name: "address wrong network",
		cmd: &types.CreateRawTransactionCmd{
			Inputs: []types.TransactionInput{{
				Amount: 1,
				Txid:   "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout:   0,
				Tree:   0,
			}},
			Amounts:  map[string]float64{"Tsf5Qvq2m7X5KzTZDdSGfa6WrMtikYVRkaL": 1},
			LockTime: dcrjson.Int64(1),
			Expiry:   dcrjson.Int64(1),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidAddressOrKey,
	}, {
		name: "address wrong type",
		cmd: &types.CreateRawTransactionCmd{
			Inputs: []types.TransactionInput{{
				Amount: 1,
				Txid:   "e02f03a25a57afdd402818fe5b13985a0731502ad8a8c93d1874900e84d3330d",
				Vout:   0,
				Tree:   0,
			}},
			Amounts:  map[string]float64{"DkRMCQhwDFTRwW6umM59KEJiMvTPke9X7akJJfbzKocNPDqZMAUEq": 1},
			LockTime: dcrjson.Int64(1),
			Expiry:   dcrjson.Int64(1),
		},
		wantErr: true,
		errCode: dcrjson.ErrRPCInvalidAddressOrKey,
	}}

	for _, test := range tests {
		result, err := handleCreateRawTransaction(nil, testServer, test.cmd)
		if test.wantErr {
			var rpcErr *dcrjson.RPCError
			if !errors.As(err, &rpcErr) || rpcErr.Code != test.errCode {
				t.Fatalf("expected error code \"%v\" did not match actual \"%v\"for test \"%s\"", test.errCode, rpcErr.Code, test.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error for test \"%s\": %v", test.name, err)
		}
		if test.result != result {
			t.Fatalf("expected result \"%s\" did not match actual \"%s\" for test \"%s\"", test.result, result, test.name)
		}
	}
}
