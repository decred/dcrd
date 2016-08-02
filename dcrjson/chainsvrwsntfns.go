// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
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

	// ReorganizationNtfnMethod is the method used for notifications that the
	// block chain is in the process of a reorganization.
	ReorganizationNtfnMethod = "reorganization"

	// NewVoteNtfnMethod is the method used for notifcations that a new vote
	// has been received by the chain server
	NewVoteNtfnMethod = "newvote"

	// RecvTxNtfnMethod is the method used for notifications from the chain
	// server that a transaction which pays to a registered address has been
	// processed.
	RecvTxNtfnMethod = "recvtx"

	// RedeemingTxNtfnMethod is the method used for notifications from the
	// chain server that a transaction which spends a registered outpoint
	// has been processed.
	RedeemingTxNtfnMethod = "redeemingtx"

	// RescanFinishedNtfnMethod is the method used for notifications from
	// the chain server that a rescan operation has finished.
	RescanFinishedNtfnMethod = "rescanfinished"

	// RescanProgressNtfnMethod is the method used for notifications from
	// the chain server that a rescan operation this is underway has made
	// progress.
	RescanProgressNtfnMethod = "rescanprogress"

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

// NewVoteNtfn defines the newvote JSON-RPC notification.
type NewVoteNtfn struct {
	VoteHash  string
	BlockHash string
	Vote      bool
}

// NewVoteNtfn retuns a new instance which can be used to issue a newvote
// JSON-RPC notification.
func NewNewVoteNtfn(voteHash, blockHash string, vote bool) *NewVoteNtfn {
	return &NewVoteNftn{
		VoteHash:  voteHash,
		BlockHash: blockHash,
		Vote:      vote,
	}
}

// BlockDetails describes details of a tx in a block.
type BlockDetails struct {
	Height   int32  `json:"height"`
	Tree     int8   `json:"tree"`
	Hash     string `json:"hash"`
	Index    int    `json:"index"`
	Time     int64  `json:"time"`
	VoteBits uint16 `json:"votebits"`
}

// RecvTxNtfn defines the recvtx JSON-RPC notification.
type RecvTxNtfn struct {
	HexTx string
	Block *BlockDetails
}

// NewRecvTxNtfn returns a new instance which can be used to issue a recvtx
// JSON-RPC notification.
func NewRecvTxNtfn(hexTx string, block *BlockDetails) *RecvTxNtfn {
	return &RecvTxNtfn{
		HexTx: hexTx,
		Block: block,
	}
}

// RedeemingTxNtfn defines the redeemingtx JSON-RPC notification.
type RedeemingTxNtfn struct {
	HexTx string
	Block *BlockDetails
}

// NewRedeemingTxNtfn returns a new instance which can be used to issue a
// redeemingtx JSON-RPC notification.
func NewRedeemingTxNtfn(hexTx string, block *BlockDetails) *RedeemingTxNtfn {
	return &RedeemingTxNtfn{
		HexTx: hexTx,
		Block: block,
	}
}

// RescanFinishedNtfn defines the rescanfinished JSON-RPC notification.
type RescanFinishedNtfn struct {
	Hash   string
	Height int32
	Time   int64
}

// NewRescanFinishedNtfn returns a new instance which can be used to issue a
// rescanfinished JSON-RPC notification.
func NewRescanFinishedNtfn(hash string, height int32, time int64) *RescanFinishedNtfn {
	return &RescanFinishedNtfn{
		Hash:   hash,
		Height: height,
		Time:   time,
	}
}

// RescanProgressNtfn defines the rescanprogress JSON-RPC notification.
type RescanProgressNtfn struct {
	Hash   string
	Height int32
	Time   int64
}

// NewRescanProgressNtfn returns a new instance which can be used to issue a
// rescanprogress JSON-RPC notification.
func NewRescanProgressNtfn(hash string, height int32, time int64) *RescanProgressNtfn {
	return &RescanProgressNtfn{
		Hash:   hash,
		Height: height,
		Time:   time,
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

func init() {
	// The commands in this file are only usable by websockets and are
	// notifications.
	flags := UFWebsocketOnly | UFNotification

	MustRegisterCmd(BlockConnectedNtfnMethod, (*BlockConnectedNtfn)(nil), flags)
	MustRegisterCmd(BlockDisconnectedNtfnMethod, (*BlockDisconnectedNtfn)(nil), flags)
	MustRegisterCmd(ReorganizationNtfnMethod, (*ReorganizationNtfn)(nil), flags)
	MustRegisterCmd(TxAcceptedNtfnMethod, (*TxAcceptedNtfn)(nil), flags)
	MustRegisterCmd(TxAcceptedVerboseNtfnMethod, (*TxAcceptedVerboseNtfn)(nil), flags)
	MustRegisterCmd(RelevantTxAcceptedNtfnMethod, (*RelevantTxAcceptedNtfn)(nil), flags)
}
