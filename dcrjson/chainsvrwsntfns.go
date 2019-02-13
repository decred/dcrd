// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC websocket notifications that are
// supported by a chain server.

package dcrjson

const (
	// BlockConnectedNtfnMethod is the method used for notifications from
	// the chain server that a block has been connected.
	BlockConnectedNtfnMethod = "blockconnected"

	// BlockDisconnectedNtfnMethod is the method used for notifications from
	// the chain server that a block has been disconnected.
	BlockDisconnectedNtfnMethod = "blockdisconnected"

	// NewTicketsNtfnMethod is the method of the daemon newtickets notification.
	NewTicketsNtfnMethod = "newtickets"

	// ReorganizationNtfnMethod is the method used for notifications that the
	// block chain is in the process of a reorganization.
	ReorganizationNtfnMethod = "reorganization"

	// TxAcceptedNtfnMethod is the method used for notifications from the
	// chain server that a transaction has been accepted into the mempool.
	TxAcceptedNtfnMethod = "txaccepted"

	// TxAcceptedVerboseNtfnMethod is the method used for notifications from
	// the chain server that a transaction has been accepted into the
	// mempool.  This differs from TxAcceptedNtfnMethod in that it provides
	// more details in the notification.
	TxAcceptedVerboseNtfnMethod = "txacceptedverbose"

	// RelevantTxAcceptedNtfnMethod is the method used for notifications
	// from the chain server that inform a client that a relevant
	// transaction was accepted by the mempool.
	RelevantTxAcceptedNtfnMethod = "relevanttxaccepted"

	// SpentAndMissedTicketsNtfnMethod is the method of the daemon
	// spentandmissedtickets notification.
	SpentAndMissedTicketsNtfnMethod = "spentandmissedtickets"

	// StakeDifficultyNtfnMethod is the method of the daemon stakedifficulty
	// notification.
	StakeDifficultyNtfnMethod = "stakedifficulty"

	// WinningTicketsNtfnMethod is the method of the daemon winningtickets
	// notification.
	WinningTicketsNtfnMethod = "winningtickets"
)

// BlockConnectedNtfn defines the blockconnected JSON-RPC notification.
type BlockConnectedNtfn struct {
	Header        string   `json:"header"`
	SubscribedTxs []string `json:"subscribedtxs"`
}

// NewBlockConnectedNtfn returns a new instance which can be used to issue a
// blockconnected JSON-RPC notification.
func NewBlockConnectedNtfn(header string, subscribedTxs []string) *BlockConnectedNtfn {
	return &BlockConnectedNtfn{
		Header:        header,
		SubscribedTxs: subscribedTxs,
	}
}

// BlockDisconnectedNtfn defines the blockdisconnected JSON-RPC notification.
type BlockDisconnectedNtfn struct {
	Header string `json:"header"`
}

// NewBlockDisconnectedNtfn returns a new instance which can be used to issue a
// blockdisconnected JSON-RPC notification.
func NewBlockDisconnectedNtfn(header string) *BlockDisconnectedNtfn {
	return &BlockDisconnectedNtfn{
		Header: header,
	}
}

// NewTicketsNtfn is a type handling custom marshaling and
// unmarshaling of newtickets JSON websocket notifications.
type NewTicketsNtfn struct {
	Hash      string
	Height    int32
	StakeDiff int64
	Tickets   []string
}

// NewNewTicketsNtfn creates a new NewTicketsNtfn.
func NewNewTicketsNtfn(hash string, height int32, stakeDiff int64, tickets []string) *NewTicketsNtfn {
	return &NewTicketsNtfn{
		Hash:      hash,
		Height:    height,
		StakeDiff: stakeDiff,
		Tickets:   tickets,
	}
}

// ReorganizationNtfn defines the reorganization JSON-RPC notification.
type ReorganizationNtfn struct {
	OldHash   string `json:"oldhash"`
	OldHeight int32  `json:"oldheight"`
	NewHash   string `json:"newhash"`
	NewHeight int32  `json:"newheight"`
}

// NewReorganizationNtfn returns a new instance which can be used to issue a
// blockdisconnected JSON-RPC notification.
func NewReorganizationNtfn(oldHash string, oldHeight int32, newHash string,
	newHeight int32) *ReorganizationNtfn {
	return &ReorganizationNtfn{
		OldHash:   oldHash,
		OldHeight: oldHeight,
		NewHash:   newHash,
		NewHeight: newHeight,
	}
}

// SpentAndMissedTicketsNtfn is a type handling custom marshaling and
// unmarshaling of spentandmissedtickets JSON websocket notifications.
type SpentAndMissedTicketsNtfn struct {
	Hash      string
	Height    int32
	StakeDiff int64
	Tickets   map[string]string
}

// NewSpentAndMissedTicketsNtfn creates a new SpentAndMissedTicketsNtfn.
func NewSpentAndMissedTicketsNtfn(hash string, height int32, stakeDiff int64, tickets map[string]string) *SpentAndMissedTicketsNtfn {
	return &SpentAndMissedTicketsNtfn{
		Hash:      hash,
		Height:    height,
		StakeDiff: stakeDiff,
		Tickets:   tickets,
	}
}

// StakeDifficultyNtfn is a type handling custom marshaling and
// unmarshaling of stakedifficulty JSON websocket notifications.
type StakeDifficultyNtfn struct {
	BlockHash   string
	BlockHeight int32
	StakeDiff   int64
}

// NewStakeDifficultyNtfn creates a new StakeDifficultyNtfn.
func NewStakeDifficultyNtfn(hash string, height int32, stakeDiff int64) *StakeDifficultyNtfn {
	return &StakeDifficultyNtfn{
		BlockHash:   hash,
		BlockHeight: height,
		StakeDiff:   stakeDiff,
	}
}

// TxAcceptedNtfn defines the txaccepted JSON-RPC notification.
type TxAcceptedNtfn struct {
	TxID   string  `json:"txid"`
	Amount float64 `json:"amount"`
}

// NewTxAcceptedNtfn returns a new instance which can be used to issue a
// txaccepted JSON-RPC notification.
func NewTxAcceptedNtfn(txHash string, amount float64) *TxAcceptedNtfn {
	return &TxAcceptedNtfn{
		TxID:   txHash,
		Amount: amount,
	}
}

// TxAcceptedVerboseNtfn defines the txacceptedverbose JSON-RPC notification.
type TxAcceptedVerboseNtfn struct {
	RawTx TxRawResult `json:"rawtx"`
}

// NewTxAcceptedVerboseNtfn returns a new instance which can be used to issue a
// txacceptedverbose JSON-RPC notification.
func NewTxAcceptedVerboseNtfn(rawTx TxRawResult) *TxAcceptedVerboseNtfn {
	return &TxAcceptedVerboseNtfn{
		RawTx: rawTx,
	}
}

// RelevantTxAcceptedNtfn defines the parameters to the relevanttxaccepted
// JSON-RPC notification.
type RelevantTxAcceptedNtfn struct {
	Transaction string `json:"transaction"`
}

// NewRelevantTxAcceptedNtfn returns a new instance which can be used to issue a
// relevantxaccepted JSON-RPC notification.
func NewRelevantTxAcceptedNtfn(txHex string) *RelevantTxAcceptedNtfn {
	return &RelevantTxAcceptedNtfn{Transaction: txHex}
}

// WinningTicketsNtfn is a type handling custom marshaling and
// unmarshaling of blockconnected JSON websocket notifications.
type WinningTicketsNtfn struct {
	BlockHash   string
	BlockHeight int32
	Tickets     map[string]string
}

// NewWinningTicketsNtfn creates a new WinningTicketsNtfn.
func NewWinningTicketsNtfn(hash string, height int32, tickets map[string]string) *WinningTicketsNtfn {
	return &WinningTicketsNtfn{
		BlockHash:   hash,
		BlockHeight: height,
		Tickets:     tickets,
	}
}

func init() {
	// The commands in this file are only usable by websockets and are
	// notifications.
	flags := UFWebsocketOnly | UFNotification

	MustRegisterCmd(BlockConnectedNtfnMethod, (*BlockConnectedNtfn)(nil), flags)
	MustRegisterCmd(BlockDisconnectedNtfnMethod, (*BlockDisconnectedNtfn)(nil), flags)
	MustRegisterCmd(NewTicketsNtfnMethod, (*NewTicketsNtfn)(nil), flags)
	MustRegisterCmd(ReorganizationNtfnMethod, (*ReorganizationNtfn)(nil), flags)
	MustRegisterCmd(TxAcceptedNtfnMethod, (*TxAcceptedNtfn)(nil), flags)
	MustRegisterCmd(TxAcceptedVerboseNtfnMethod, (*TxAcceptedVerboseNtfn)(nil), flags)
	MustRegisterCmd(RelevantTxAcceptedNtfnMethod, (*RelevantTxAcceptedNtfn)(nil), flags)
	MustRegisterCmd(SpentAndMissedTicketsNtfnMethod, (*SpentAndMissedTicketsNtfn)(nil), flags)
	MustRegisterCmd(StakeDifficultyNtfnMethod, (*StakeDifficultyNtfn)(nil), flags)
	MustRegisterCmd(WinningTicketsNtfnMethod, (*WinningTicketsNtfn)(nil), flags)
}
