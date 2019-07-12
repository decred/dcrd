// Copyright (c) 2014-2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a chain server, but are only available via websockets.

package dcrjson

import "github.com/decred/dcrd/rpc/jsonrpc/types"

// AuthenticateCmd defines the authenticate JSON-RPC command.
type AuthenticateCmd = types.AuthenticateCmd

// NewAuthenticateCmd returns a new instance which can be used to issue an
// authenticate JSON-RPC command.
func NewAuthenticateCmd(username, passphrase string) *AuthenticateCmd {
	return types.NewAuthenticateCmd(username, passphrase)
}

// OutPoint describes a transaction outpoint that will be marshalled to and
// from JSON.  Contains Decred addition.
type OutPoint = types.OutPoint

// LoadTxFilterCmd defines the loadtxfilter request parameters to load or
// reload a transaction filter.
type LoadTxFilterCmd = types.LoadTxFilterCmd

// NewLoadTxFilterCmd returns a new instance which can be used to issue a
// loadtxfilter JSON-RPC command.
func NewLoadTxFilterCmd(reload bool, addresses []string, outPoints []OutPoint) *LoadTxFilterCmd {
	return types.NewLoadTxFilterCmd(reload, addresses, outPoints)
}

// NotifyBlocksCmd defines the notifyblocks JSON-RPC command.
type NotifyBlocksCmd = types.NotifyBlocksCmd

// NewNotifyBlocksCmd returns a new instance which can be used to issue a
// notifyblocks JSON-RPC command.
func NewNotifyBlocksCmd() *NotifyBlocksCmd {
	return types.NewNotifyBlocksCmd()
}

// NotifyWinningTicketsCmd is a type handling custom marshaling and
// unmarshaling of notifywinningtickets JSON websocket extension
// commands.
type NotifyWinningTicketsCmd = types.NotifyWinningTicketsCmd

// NewNotifyWinningTicketsCmd creates a new NotifyWinningTicketsCmd.
func NewNotifyWinningTicketsCmd() *NotifyWinningTicketsCmd {
	return types.NewNotifyWinningTicketsCmd()
}

// NotifySpentAndMissedTicketsCmd is a type handling custom marshaling and
// unmarshaling of notifyspentandmissedtickets JSON websocket extension
// commands.
type NotifySpentAndMissedTicketsCmd = types.NotifySpentAndMissedTicketsCmd

// NewNotifySpentAndMissedTicketsCmd creates a new NotifySpentAndMissedTicketsCmd.
func NewNotifySpentAndMissedTicketsCmd() *NotifySpentAndMissedTicketsCmd {
	return types.NewNotifySpentAndMissedTicketsCmd()
}

// NotifyNewTicketsCmd is a type handling custom marshaling and
// unmarshaling of notifynewtickets JSON websocket extension
// commands.
type NotifyNewTicketsCmd = types.NotifyNewTicketsCmd

// NewNotifyNewTicketsCmd creates a new NotifyNewTicketsCmd.
func NewNotifyNewTicketsCmd() *NotifyNewTicketsCmd {
	return types.NewNotifyNewTicketsCmd()
}

// NotifyStakeDifficultyCmd is a type handling custom marshaling and
// unmarshaling of notifystakedifficulty JSON websocket extension
// commands.
type NotifyStakeDifficultyCmd = types.NotifyStakeDifficultyCmd

// NewNotifyStakeDifficultyCmd creates a new NotifyStakeDifficultyCmd.
func NewNotifyStakeDifficultyCmd() *NotifyStakeDifficultyCmd {
	return types.NewNotifyStakeDifficultyCmd()
}

// StopNotifyBlocksCmd defines the stopnotifyblocks JSON-RPC command.
type StopNotifyBlocksCmd = types.StopNotifyBlocksCmd

// NewStopNotifyBlocksCmd returns a new instance which can be used to issue a
// stopnotifyblocks JSON-RPC command.
func NewStopNotifyBlocksCmd() *StopNotifyBlocksCmd {
	return types.NewStopNotifyBlocksCmd()
}

// NotifyNewTransactionsCmd defines the notifynewtransactions JSON-RPC command.
type NotifyNewTransactionsCmd = types.NotifyNewTransactionsCmd

// NewNotifyNewTransactionsCmd returns a new instance which can be used to issue
// a notifynewtransactions JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewNotifyNewTransactionsCmd(verbose *bool) *NotifyNewTransactionsCmd {
	return types.NewNotifyNewTransactionsCmd(verbose)
}

// SessionCmd defines the session JSON-RPC command.
type SessionCmd = types.SessionCmd

// NewSessionCmd returns a new instance which can be used to issue a session
// JSON-RPC command.
func NewSessionCmd() *SessionCmd {
	return types.NewSessionCmd()
}

// StopNotifyNewTransactionsCmd defines the stopnotifynewtransactions JSON-RPC command.
type StopNotifyNewTransactionsCmd = types.StopNotifyNewTransactionsCmd

// NewStopNotifyNewTransactionsCmd returns a new instance which can be used to issue
// a stopnotifynewtransactions JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewStopNotifyNewTransactionsCmd() *StopNotifyNewTransactionsCmd {
	return types.NewStopNotifyNewTransactionsCmd()
}

// RescanCmd defines the rescan JSON-RPC command.
type RescanCmd struct {
	// Concatenated block hashes in non-byte-reversed hex encoding.  Must
	// have length evenly divisible by 2*chainhash.HashSize.
	BlockHashes string
}

// NewRescanCmd returns a new instance which can be used to issue a rescan
// JSON-RPC command.
func NewRescanCmd(blockHashes string) *RescanCmd {
	return &RescanCmd{BlockHashes: blockHashes}
}

func init() {
	// The commands in this file are only usable by websockets.
	flags := UFWebsocketOnly

	MustRegisterCmd("authenticate", (*AuthenticateCmd)(nil), flags)
	MustRegisterCmd("loadtxfilter", (*LoadTxFilterCmd)(nil), flags)
	MustRegisterCmd("notifyblocks", (*NotifyBlocksCmd)(nil), flags)
	MustRegisterCmd("notifynewtransactions", (*NotifyNewTransactionsCmd)(nil), flags)
	MustRegisterCmd("notifynewtickets", (*NotifyNewTicketsCmd)(nil), flags)
	MustRegisterCmd("notifyspentandmissedtickets",
		(*NotifySpentAndMissedTicketsCmd)(nil), flags)
	MustRegisterCmd("notifystakedifficulty",
		(*NotifyStakeDifficultyCmd)(nil), flags)
	MustRegisterCmd("notifywinningtickets",
		(*NotifyWinningTicketsCmd)(nil), flags)
	MustRegisterCmd("session", (*SessionCmd)(nil), flags)
	MustRegisterCmd("stopnotifyblocks", (*StopNotifyBlocksCmd)(nil), flags)
	MustRegisterCmd("stopnotifynewtransactions", (*StopNotifyNewTransactionsCmd)(nil), flags)
	MustRegisterCmd("rescan", (*RescanCmd)(nil), flags)
}
