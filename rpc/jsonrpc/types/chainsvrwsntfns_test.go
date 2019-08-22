// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/decred/dcrd/dcrjson/v3"
)

// TestChainSvrWsNtfns tests all of the chain server websocket-specific
// notifications marshal and unmarshal into valid results include handling of
// optional fields being omitted in the marshalled command, while optional
// fields with defaults have the default assigned on unmarshalled commands.
func TestChainSvrWsNtfns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		newNtfn      func() (interface{}, error)
		staticNtfn   func() interface{}
		marshalled   string
		unmarshalled interface{}
	}{
		{
			name: "blockconnected",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("blockconnected"), "header", []string{"tx0", "tx1"})
			},
			staticNtfn: func() interface{} {
				return NewBlockConnectedNtfn("header", []string{"tx0", "tx1"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"blockconnected","params":["header",["tx0","tx1"]],"id":null}`,
			unmarshalled: &BlockConnectedNtfn{
				Header:        "header",
				SubscribedTxs: []string{"tx0", "tx1"},
			},
		},
		{
			name: "blockdisconnected",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("blockdisconnected"), "header")
			},
			staticNtfn: func() interface{} {
				return NewBlockDisconnectedNtfn("header")
			},
			marshalled: `{"jsonrpc":"1.0","method":"blockdisconnected","params":["header"],"id":null}`,
			unmarshalled: &BlockDisconnectedNtfn{
				Header: "header",
			},
		},
		{
			name: "newtickets",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("newtickets"), "123", 100, 3, []string{"a", "b"})
			},
			staticNtfn: func() interface{} {
				return NewNewTicketsNtfn("123", 100, 3, []string{"a", "b"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"newtickets","params":["123",100,3,["a","b"]],"id":null}`,
			unmarshalled: &NewTicketsNtfn{
				Hash:      "123",
				Height:    100,
				StakeDiff: 3,
				Tickets:   []string{"a", "b"},
			},
		},
		{
			name: "relevanttxaccepted",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("relevanttxaccepted"), "001122")
			},
			staticNtfn: func() interface{} {
				return NewRelevantTxAcceptedNtfn("001122")
			},
			marshalled: `{"jsonrpc":"1.0","method":"relevanttxaccepted","params":["001122"],"id":null}`,
			unmarshalled: &RelevantTxAcceptedNtfn{
				Transaction: "001122",
			},
		},
		{
			name: "spentandmissedtickets",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("spentandmissedtickets"), "123", 100, 3, map[string]string{"a": "b"})
			},
			staticNtfn: func() interface{} {
				return NewSpentAndMissedTicketsNtfn("123", 100, 3, map[string]string{"a": "b"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"spentandmissedtickets","params":["123",100,3,{"a":"b"}],"id":null}`,
			unmarshalled: &SpentAndMissedTicketsNtfn{
				Hash:      "123",
				Height:    100,
				StakeDiff: 3,
				Tickets:   map[string]string{"a": "b"},
			},
		},
		{
			name: "txaccepted",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("txaccepted"), "123", 1.5)
			},
			staticNtfn: func() interface{} {
				return NewTxAcceptedNtfn("123", 1.5)
			},
			marshalled: `{"jsonrpc":"1.0","method":"txaccepted","params":["123",1.5],"id":null}`,
			unmarshalled: &TxAcceptedNtfn{
				TxID:   "123",
				Amount: 1.5,
			},
		},
		{
			name: "txacceptedverbose",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("txacceptedverbose"), `{"hex":"001122","txid":"123","version":1,"locktime":4294967295,"vin":null,"vout":null,"confirmations":0}`)
			},
			staticNtfn: func() interface{} {
				txResult := TxRawResult{
					Hex:           "001122",
					Txid:          "123",
					Version:       1,
					LockTime:      4294967295,
					Vin:           nil,
					Vout:          nil,
					Confirmations: 0,
				}
				return NewTxAcceptedVerboseNtfn(txResult)
			},
			marshalled: `{"jsonrpc":"1.0","method":"txacceptedverbose","params":[{"hex":"001122","txid":"123","version":1,"locktime":4294967295,"expiry":0,"vin":null,"vout":null}],"id":null}`,
			unmarshalled: &TxAcceptedVerboseNtfn{
				RawTx: TxRawResult{
					Hex:           "001122",
					Txid:          "123",
					Version:       1,
					LockTime:      4294967295,
					Vin:           nil,
					Vout:          nil,
					Confirmations: 0,
				},
			},
		},
		{
			name: "winningtickets",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("winningtickets"), "123", 100, map[string]string{"a": "b"})
			},
			staticNtfn: func() interface{} {
				return NewWinningTicketsNtfn("123", 100, map[string]string{"a": "b"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"winningtickets","params":["123",100,{"a":"b"}],"id":null}`,
			unmarshalled: &WinningTicketsNtfn{
				BlockHash:   "123",
				BlockHeight: 100,
				Tickets:     map[string]string{"a": "b"},
			},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Marshal the notification as created by the new static
		// creation function.  The ID is nil for notifications.
		marshalled, err := dcrjson.MarshalCmd("1.0", nil, test.staticNtfn())
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

		// Ensure the notification is created without error via the
		// generic new notification creation function.
		cmd, err := test.newNtfn()
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected dcrjson.NewCmd error: %v",
				i, test.name, err)
		}

		// Marshal the notification as created by the generic new
		// notification creation function.    The ID is nil for
		// notifications.
		marshalled, err = dcrjson.MarshalCmd("1.0", nil, cmd)
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
