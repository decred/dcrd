// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
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

// TestChainSvrWsCmds tests all of the chain server websocket-specific commands
// marshal and unmarshal into valid results include handling of optional fields
// being omitted in the marshalled command, while optional fields with defaults
// have the default assigned on unmarshalled commands.
func TestChainSvrWsCmds(t *testing.T) {
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
			name: "authenticate",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("authenticate"), "user", "pass")
			},
			staticCmd: func() interface{} {
				return NewAuthenticateCmd("user", "pass")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"authenticate","params":["user","pass"],"id":1}`,
			unmarshalled: &AuthenticateCmd{Username: "user", Passphrase: "pass"},
		},
		{
			name: "notifywinningtickets",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("notifywinningtickets"))
			},
			staticCmd: func() interface{} {
				return NewNotifyWinningTicketsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"notifywinningtickets","params":[],"id":1}`,
			unmarshalled: &NotifyWinningTicketsCmd{},
		},
		{
			name: "notifynewtickets",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("notifynewtickets"))
			},
			staticCmd: func() interface{} {
				return NewNotifyNewTicketsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"notifynewtickets","params":[],"id":1}`,
			unmarshalled: &NotifyNewTicketsCmd{},
		},
		{
			name: "notifyblocks",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("notifyblocks"))
			},
			staticCmd: func() interface{} {
				return NewNotifyBlocksCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"notifyblocks","params":[],"id":1}`,
			unmarshalled: &NotifyBlocksCmd{},
		},
		{
			name: "notifywork",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("notifywork"))
			},
			staticCmd: func() interface{} {
				return NewNotifyWorkCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"notifywork","params":[],"id":1}`,
			unmarshalled: &NotifyWorkCmd{},
		},
		{
			name: "notifytspend",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("notifytspend"))
			},
			staticCmd: func() interface{} {
				return NewNotifyTSpendCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"notifytspend","params":[],"id":1}`,
			unmarshalled: &NotifyTSpendCmd{},
		},
		{
			name: "stopnotifyblocks",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("stopnotifyblocks"))
			},
			staticCmd: func() interface{} {
				return NewStopNotifyBlocksCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"stopnotifyblocks","params":[],"id":1}`,
			unmarshalled: &StopNotifyBlocksCmd{},
		},
		{
			name: "stopnotifywork",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("stopnotifywork"))
			},
			staticCmd: func() interface{} {
				return NewStopNotifyWorkCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"stopnotifywork","params":[],"id":1}`,
			unmarshalled: &StopNotifyWorkCmd{},
		},
		{
			name: "stopnotifytspend",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("stopnotifytspend"))
			},
			staticCmd: func() interface{} {
				return NewStopNotifyTSpendCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"stopnotifytspend","params":[],"id":1}`,
			unmarshalled: &StopNotifyTSpendCmd{},
		},
		{
			name: "notifynewtransactions",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("notifynewtransactions"))
			},
			staticCmd: func() interface{} {
				return NewNotifyNewTransactionsCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"notifynewtransactions","params":[],"id":1}`,
			unmarshalled: &NotifyNewTransactionsCmd{
				Verbose: dcrjson.Bool(false),
			},
		},
		{
			name: "notifynewtransactions optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("notifynewtransactions"), true)
			},
			staticCmd: func() interface{} {
				return NewNotifyNewTransactionsCmd(dcrjson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"notifynewtransactions","params":[true],"id":1}`,
			unmarshalled: &NotifyNewTransactionsCmd{
				Verbose: dcrjson.Bool(true),
			},
		},
		{
			name: "stopnotifynewtransactions",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("stopnotifynewtransactions"))
			},
			staticCmd: func() interface{} {
				return NewStopNotifyNewTransactionsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"stopnotifynewtransactions","params":[],"id":1}`,
			unmarshalled: &StopNotifyNewTransactionsCmd{},
		},
		{
			name: "rescan",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd(Method("rescan"), []string{"0000000000000000000000000000000000000000000000000000000000000123"})
			},
			staticCmd: func() interface{} {
				return NewRescanCmd([]string{"0000000000000000000000000000000000000000000000000000000000000123"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"rescan","params":[["0000000000000000000000000000000000000000000000000000000000000123"]],"id":1}`,
			unmarshalled: &RescanCmd{
				BlockHashes: []string{"0000000000000000000000000000000000000000000000000000000000000123"},
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
