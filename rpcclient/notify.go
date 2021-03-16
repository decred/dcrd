// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v4"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v3"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

var (
	// ErrWebsocketsRequired is an error to describe the condition where the
	// caller is trying to use a websocket-only feature, such as requesting
	// notifications or other websocket requests when the client is
	// configured to run in HTTP POST mode.
	ErrWebsocketsRequired = errors.New("a websocket connection is required " +
		"to use this feature")
)

// notificationState is used to track the current state of successfully
// registered notifications so the state can be automatically re-established on
// reconnect.
type notificationState struct {
	notifyBlocks                bool
	notifyWork                  bool
	notifyTSpend                bool
	notifyWinningTickets        bool
	notifySpentAndMissedTickets bool
	notifyNewTickets            bool
	notifyNewTx                 bool
	notifyNewTxVerbose          bool
}

// Copy returns a deep copy of the receiver.
func (s *notificationState) Copy() *notificationState {
	var stateCopy notificationState
	stateCopy.notifyBlocks = s.notifyBlocks
	stateCopy.notifyWork = s.notifyWork
	stateCopy.notifyTSpend = s.notifyTSpend
	stateCopy.notifyWinningTickets = s.notifyWinningTickets
	stateCopy.notifySpentAndMissedTickets = s.notifySpentAndMissedTickets
	stateCopy.notifyNewTickets = s.notifyNewTickets
	stateCopy.notifyNewTx = s.notifyNewTx
	stateCopy.notifyNewTxVerbose = s.notifyNewTxVerbose

	return &stateCopy
}

// newNotificationState returns a new notification state ready to be populated.
func newNotificationState() *notificationState {
	return &notificationState{}
}

// newNilFutureResult returns a new future result that already has the result
// waiting on the channel with the reply set to nil.  This is useful to ignore
// things such as notifications when the caller didn't specify any notification
// handlers.
func newNilFutureResult(ctx context.Context) *cmdRes {
	responseChan := make(chan *response, 1)
	responseChan <- &response{result: nil, err: nil}
	return &cmdRes{ctx: ctx, c: responseChan}
}

// NotificationHandlers defines callback function pointers to invoke with
// notifications.  Since all of the functions are nil by default, all
// notifications are effectively ignored until their handlers are set to a
// concrete callback.
//
// NOTE: Unless otherwise documented, these handlers must NOT directly call any
// blocking calls on the client instance since the input reader goroutine blocks
// until the callback has completed.  Doing so will result in a deadlock
// situation.
type NotificationHandlers struct {
	// OnClientConnected is invoked when the client connects or reconnects
	// to the RPC server.  This callback is run async with the rest of the
	// notification handlers, and is safe for blocking client requests.
	OnClientConnected func()

	// OnBlockConnected is invoked when a block is connected to the longest
	// (best) chain.  It will only be invoked if a preceding call to
	// NotifyBlocks has been made to register for the notification and the
	// function is non-nil.
	OnBlockConnected func(blockHeader []byte, transactions [][]byte)

	// OnBlockDisconnected is invoked when a block is disconnected from the
	// longest (best) chain.  It will only be invoked if a preceding call to
	// NotifyBlocks has been made to register for the notification and the
	// function is non-nil.
	OnBlockDisconnected func(blockHeader []byte)

	// OnWork is invoked when a new block template is generated.
	// It will only be invoked if a preceding call to NotifyWork has
	// been made to register for the notification and the function is non-nil.
	OnWork func(data []byte, target []byte, reason string)

	// OnTSpend is invoked when a new tspend arrives in the mempool.  It
	// will only be invoked if a preceding call to NotifyTSpend has been
	// made to register for the notification and the function is non-nil.
	OnTSpend func(tspend []byte)

	// OnRelevantTxAccepted is invoked when an unmined transaction passes
	// the client's transaction filter.
	OnRelevantTxAccepted func(transaction []byte)

	// OnReorganization is invoked when the blockchain begins reorganizing.
	// It will only be invoked if a preceding call to NotifyBlocks has been
	// made to register for the notification and the function is non-nil.
	OnReorganization func(oldHash *chainhash.Hash, oldHeight int32,
		newHash *chainhash.Hash, newHeight int32)

	// OnWinningTickets is invoked when a block is connected and eligible tickets
	// to be voted on for this chain are given.  It will only be invoked if a
	// preceding call to NotifyWinningTickets has been made to register for the
	// notification and the function is non-nil.
	OnWinningTickets func(blockHash *chainhash.Hash,
		blockHeight int64,
		tickets []*chainhash.Hash)

	// OnSpentAndMissedTickets is invoked when a block is connected to the
	// longest (best) chain and tickets are spent or missed.  It will only be
	// invoked if a preceding call to NotifySpentAndMissedTickets has been made to
	// register for the notification and the function is non-nil.
	OnSpentAndMissedTickets func(hash *chainhash.Hash,
		height int64,
		stakeDiff int64,
		tickets map[chainhash.Hash]bool)

	// OnNewTickets is invoked when a block is connected to the longest (best)
	// chain and tickets have matured to become active.  It will only be invoked
	// if a preceding call to NotifyNewTickets has been made to register for the
	// notification and the function is non-nil.
	OnNewTickets func(hash *chainhash.Hash,
		height int64,
		stakeDiff int64,
		tickets []*chainhash.Hash)

	// OnTxAccepted is invoked when a transaction is accepted into the
	// memory pool.  It will only be invoked if a preceding call to
	// NotifyNewTransactions with the verbose flag set to false has been
	// made to register for the notification and the function is non-nil.
	OnTxAccepted func(hash *chainhash.Hash, amount dcrutil.Amount)

	// OnTxAccepted is invoked when a transaction is accepted into the
	// memory pool.  It will only be invoked if a preceding call to
	// NotifyNewTransactions with the verbose flag set to true has been
	// made to register for the notification and the function is non-nil.
	OnTxAcceptedVerbose func(txDetails *chainjson.TxRawResult)

	// OnUnknownNotification is invoked when an unrecognized notification
	// is received.  This typically means the notification handling code
	// for this package needs to be updated for a new notification type or
	// the caller is using a custom notification this package does not know
	// about.
	OnUnknownNotification func(method string, params []json.RawMessage)
}

// handleNotification examines the passed notification type, performs
// conversions to get the raw notification types into higher level types and
// delivers the notification to the appropriate On<X> handler registered with
// the client.
func (c *Client) handleNotification(ntfn *rawNotification) {
	// Ignore the notification if the client is not interested in any
	// notifications.
	if c.ntfnHandlers == nil {
		return
	}

	// Handle chain notifications.
	switch chainjson.Method(ntfn.Method) {
	// OnBlockConnected
	case chainjson.BlockConnectedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnBlockConnected == nil {
			return
		}

		blockHeader, transactions, err := parseBlockConnectedParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid blockconnected "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnBlockConnected(blockHeader, transactions)

	// OnBlockDisconnected
	case chainjson.BlockDisconnectedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnBlockDisconnected == nil {
			return
		}

		blockHeader, err := parseBlockDisconnectedParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid blockdisconnected "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnBlockDisconnected(blockHeader)

	// OnWork
	case chainjson.WorkNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnWork == nil {
			return
		}

		data, target, reason, err := parseWorkParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid work notification: %v", err)
			return
		}

		c.ntfnHandlers.OnWork(data, target, reason)

	// OnTSpend
	case chainjson.TSpendNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnTSpend == nil {
			return
		}

		tspend, err := parseTSpendParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid tspend notification: %v",
				err)
			return
		}

		c.ntfnHandlers.OnTSpend(tspend)

	case chainjson.RelevantTxAcceptedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnRelevantTxAccepted == nil {
			return
		}

		transaction, err := parseRelevantTxAcceptedParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid relevanttxaccepted "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnRelevantTxAccepted(transaction)

	// OnReorganization
	case chainjson.ReorganizationNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnReorganization == nil {
			return
		}

		oldHash, oldHeight, newHash, newHeight, err :=
			parseReorganizationNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid reorganization "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnReorganization(oldHash, oldHeight, newHash, newHeight)

	// OnWinningTickets
	case chainjson.WinningTicketsNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnWinningTickets == nil {
			return
		}

		blockHash, blockHeight, tickets, err :=
			parseWinningTicketsNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid winning tickets "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnWinningTickets(blockHash, blockHeight, tickets)

	// OnSpentAndMissedTickets
	case chainjson.SpentAndMissedTicketsNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnSpentAndMissedTickets == nil {
			return
		}

		blockSha, blockHeight, stakeDifficulty, tickets, err :=
			parseSpentAndMissedTicketsNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid spend and missed tickets "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnSpentAndMissedTickets(blockSha,
			blockHeight,
			stakeDifficulty,
			tickets)

	// OnNewTickets
	case chainjson.NewTicketsNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnNewTickets == nil {
			return
		}

		blockSha, blockHeight, stakeDifficulty, tickets, err :=
			parseNewTicketsNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid new tickets "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnNewTickets(blockSha,
			blockHeight,
			stakeDifficulty,
			tickets)

	// OnTxAccepted
	case chainjson.TxAcceptedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnTxAccepted == nil {
			return
		}

		hash, amt, err := parseTxAcceptedNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid tx accepted "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnTxAccepted(hash, amt)

	// OnTxAcceptedVerbose
	case chainjson.TxAcceptedVerboseNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnTxAcceptedVerbose == nil {
			return
		}

		rawTx, err := parseTxAcceptedVerboseNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid tx accepted verbose "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnTxAcceptedVerbose(rawTx)

	default:
		if c.ntfnHandlers.OnUnknownNotification == nil {
			log.Tracef("unknown notification received")
			return
		}

		c.ntfnHandlers.OnUnknownNotification(ntfn.Method, ntfn.Params)
	}
}

// wrongNumParams is an error type describing an unparseable JSON-RPC
// notification due to an incorrect number of parameters for the
// expected notification type.  The value is the number of parameters
// of the invalid notification.
type wrongNumParams int

// Error satisfies the builtin error interface.
func (e wrongNumParams) Error() string {
	return fmt.Sprintf("wrong number of parameters (%d)", e)
}

func parseHexParam(param json.RawMessage) ([]byte, error) {
	var s string
	err := json.Unmarshal(param, &s)
	if err != nil {
		return nil, err
	}
	return hex.DecodeString(s)
}

// parseBlockConnectedParams parses out the parameters included in a
// blockconnected notification.
func parseBlockConnectedParams(params []json.RawMessage) (blockHeader []byte, transactions [][]byte, err error) {
	if len(params) != 2 {
		return nil, nil, wrongNumParams(len(params))
	}

	blockHeader, err = parseHexParam(params[0])
	if err != nil {
		return nil, nil, err
	}

	var hexTransactions []string
	err = json.Unmarshal(params[1], &hexTransactions)
	if err != nil {
		return nil, nil, err
	}
	transactions = make([][]byte, len(hexTransactions))
	for i, hexTx := range hexTransactions {
		transactions[i], err = hex.DecodeString(hexTx)
		if err != nil {
			return nil, nil, err
		}
	}

	return blockHeader, transactions, nil
}

// parseWorkParams parses out the parameters included in a
// newwork notification.
func parseWorkParams(params []json.RawMessage) (data, target []byte, reason string, err error) {
	if len(params) != 3 {
		return nil, nil, "", wrongNumParams(len(params))
	}

	data, err = parseHexParam(params[0])
	if err != nil {
		return nil, nil, "", err
	}

	target, err = parseHexParam(params[1])
	if err != nil {
		return nil, nil, "", err
	}

	err = json.Unmarshal(params[2], &reason)
	if err != nil {
		return nil, nil, "", err
	}

	return data, target, reason, nil
}

// parseTSpendParams parses out the parameters included in a tspend
// notification.
func parseTSpendParams(params []json.RawMessage) ([]byte, error) {
	if len(params) != 1 {
		return nil, wrongNumParams(len(params))
	}

	data, err := parseHexParam(params[0])
	if err != nil {
		return nil, err
	}

	return data, nil
}

// parseBlockDisconnectedParams parses out the parameters included in a
// blockdisconnected notification.
func parseBlockDisconnectedParams(params []json.RawMessage) (blockHeader []byte, err error) {
	if len(params) != 1 {
		return nil, wrongNumParams(len(params))
	}

	return parseHexParam(params[0])
}

// parseRelevantTxAcceptedParams parses out the parameter included in a
// relevanttxaccepted notification.
func parseRelevantTxAcceptedParams(params []json.RawMessage) (transaction []byte, err error) {
	if len(params) != 1 {
		return nil, wrongNumParams(len(params))
	}

	return parseHexParam(params[0])
}

func parseReorganizationNtfnParams(params []json.RawMessage) (*chainhash.Hash,
	int32, *chainhash.Hash, int32, error) {
	errorOut := func(err error) (*chainhash.Hash, int32, *chainhash.Hash,
		int32, error) {
		return nil, 0, nil, 0, err
	}

	if len(params) != 4 {
		return errorOut(wrongNumParams(len(params)))
	}

	// Unmarshal first parameter as a string.
	var oldHashStr string
	err := json.Unmarshal(params[0], &oldHashStr)
	if err != nil {
		return errorOut(err)
	}

	// Unmarshal second parameter as an integer.
	var oldHeight int32
	err = json.Unmarshal(params[1], &oldHeight)
	if err != nil {
		return errorOut(err)
	}

	// Unmarshal first parameter as a string.
	var newHashStr string
	err = json.Unmarshal(params[2], &newHashStr)
	if err != nil {
		return errorOut(err)
	}

	// Unmarshal second parameter as an integer.
	var newHeight int32
	err = json.Unmarshal(params[3], &newHeight)
	if err != nil {
		return errorOut(err)
	}

	// Create hash from block sha string.
	oldHash, err := chainhash.NewHashFromStr(oldHashStr)
	if err != nil {
		return errorOut(err)
	}

	// Create hash from block sha string.
	newHash, err := chainhash.NewHashFromStr(newHashStr)
	if err != nil {
		return errorOut(err)
	}

	return oldHash, oldHeight, newHash, newHeight, nil
}

// parseWinningTicketsNtfnParams parses out the list of eligible tickets, block
// hash, and block height from a WinningTickets notification.
func parseWinningTicketsNtfnParams(params []json.RawMessage) (
	*chainhash.Hash,
	int64,
	[]*chainhash.Hash,
	error) {

	if len(params) != 3 {
		return nil, 0, nil, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	var blockHashStr string
	err := json.Unmarshal(params[0], &blockHashStr)
	if err != nil {
		return nil, 0, nil, err
	}

	// Create ShaHash from block sha string.
	bHash, err := chainhash.NewHashFromStr(blockHashStr)
	if err != nil {
		return nil, 0, nil, err
	}

	// Unmarshal second parameter as an integer.
	var blockHeight int32
	err = json.Unmarshal(params[1], &blockHeight)
	if err != nil {
		return nil, 0, nil, err
	}
	bHeight := int64(blockHeight)

	// Unmarshal third parameter as a slice.
	tickets := make(map[string]string)
	err = json.Unmarshal(params[2], &tickets)
	if err != nil {
		return nil, 0, nil, err
	}
	t := make([]*chainhash.Hash, len(tickets))

	for i, ticketHashStr := range tickets {
		// Create and cache Hash from tx hash.
		ticketHash, err := chainhash.NewHashFromStr(ticketHashStr)
		if err != nil {
			return nil, 0, nil, err
		}

		itr, err := strconv.Atoi(i)
		if err != nil {
			return nil, 0, nil, err
		}

		t[itr] = ticketHash
	}

	return bHash, bHeight, t, nil
}

// parseSpentAndMissedTicketsNtfnParams parses out the block header hash, height,
// winner number, and ticket map from a SpentAndMissedTickets notification.
func parseSpentAndMissedTicketsNtfnParams(params []json.RawMessage) (
	*chainhash.Hash,
	int64,
	int64,
	map[chainhash.Hash]bool,
	error) {

	if len(params) != 4 {
		return nil, 0, 0, nil, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	var blockShaStr string
	err := json.Unmarshal(params[0], &blockShaStr)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	// Create ShaHash from block sha string.
	sha, err := chainhash.NewHashFromStr(blockShaStr)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	// Unmarshal second parameter as an integer.
	var blockHeight int32
	err = json.Unmarshal(params[1], &blockHeight)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	bh := int64(blockHeight)

	// Unmarshal third parameter as an integer.
	var stakeDiff int64
	err = json.Unmarshal(params[2], &stakeDiff)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	// Unmarshal fourth parameter as a map[*hash]bool.
	tickets := make(map[string]string)
	err = json.Unmarshal(params[3], &tickets)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	t := make(map[chainhash.Hash]bool)

	for hashStr, spentStr := range tickets {
		isSpent := false
		if spentStr == "spent" {
			isSpent = true
		}

		// Create and cache ShaHash from tx hash.
		ticketSha, err := chainhash.NewHashFromStr(hashStr)
		if err != nil {
			return nil, 0, 0, nil, err
		}

		t[*ticketSha] = isSpent
	}

	return sha, bh, stakeDiff, t, nil
}

// parseNewTicketsNtfnParams parses out the block header hash, height,
// winner number, overflow, and ticket map from a NewTickets notification.
func parseNewTicketsNtfnParams(params []json.RawMessage) (*chainhash.Hash, int64, int64, []*chainhash.Hash, error) {

	if len(params) != 4 {
		return nil, 0, 0, nil, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	var blockShaStr string
	err := json.Unmarshal(params[0], &blockShaStr)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	// Create ShaHash from block sha string.
	sha, err := chainhash.NewHashFromStr(blockShaStr)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	// Unmarshal second parameter as an integer.
	var blockHeight int32
	err = json.Unmarshal(params[1], &blockHeight)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	bh := int64(blockHeight)

	// Unmarshal third parameter as an integer.
	var stakeDiff int64
	err = json.Unmarshal(params[2], &stakeDiff)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	// Unmarshal fourth parameter as a slice.
	var tickets []string
	err = json.Unmarshal(params[3], &tickets)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	t := make([]*chainhash.Hash, len(tickets))

	for i, ticketHashStr := range tickets {
		ticketHash, err := chainhash.NewHashFromStr(ticketHashStr)
		if err != nil {
			return nil, 0, 0, nil, err
		}

		t[i] = ticketHash
	}

	return sha, bh, stakeDiff, t, nil
}

// parseTxAcceptedNtfnParams parses out the transaction hash and total amount
// from the parameters of a txaccepted notification.
func parseTxAcceptedNtfnParams(params []json.RawMessage) (*chainhash.Hash,
	dcrutil.Amount, error) {

	if len(params) != 2 {
		return nil, 0, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	var txHashStr string
	err := json.Unmarshal(params[0], &txHashStr)
	if err != nil {
		return nil, 0, err
	}

	// Unmarshal second parameter as a floating point number.
	var famt float64
	err = json.Unmarshal(params[1], &famt)
	if err != nil {
		return nil, 0, err
	}

	// Bounds check amount.
	amt, err := dcrutil.NewAmount(famt)
	if err != nil {
		return nil, 0, err
	}

	// Decode string encoding of transaction sha.
	txHash, err := chainhash.NewHashFromStr(txHashStr)
	if err != nil {
		return nil, 0, err
	}

	return txHash, amt, nil
}

// parseTxAcceptedVerboseNtfnParams parses out details about a raw transaction
// from the parameters of a txacceptedverbose notification.
func parseTxAcceptedVerboseNtfnParams(params []json.RawMessage) (*chainjson.TxRawResult,
	error) {

	if len(params) != 1 {
		return nil, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a raw transaction result object.
	var rawTx chainjson.TxRawResult
	err := json.Unmarshal(params[0], &rawTx)
	if err != nil {
		return nil, err
	}

	// TODO: change txacceptedverbose notification callbacks to use nicer
	// types for all details about the transaction (i.e. decoding hashes
	// from their string encoding).
	return &rawTx, nil
}

// FutureNotifyBlocksResult is a future promise to deliver the result of a
// NotifyBlocksAsync RPC invocation (or an applicable error).
type FutureNotifyBlocksResult cmdRes

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r *FutureNotifyBlocksResult) Receive() error {
	_, err := receiveFuture(r.ctx, r.c)
	return err
}

// FutureNotifyWorkResult is a future promise to deliver the result of a
// NotifyWorkAsync RPC invocation (or an applicable error).
type FutureNotifyWorkResult cmdRes

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r *FutureNotifyWorkResult) Receive() error {
	_, err := receiveFuture(r.ctx, r.c)
	return err
}

// FutureNotifyTSpendResult is a future promise to deliver the result of a
// NotifyTSpendAsync RPC invocation (or an applicable error).
type FutureNotifyTSpendResult cmdRes

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r *FutureNotifyTSpendResult) Receive() error {
	_, err := receiveFuture(r.ctx, r.c)
	return err
}

// NotifyBlocksAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See NotifyBlocks for the blocking version and more details.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyBlocksAsync(ctx context.Context) *FutureNotifyBlocksResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return (*FutureNotifyBlocksResult)(newFutureError(ctx, ErrWebsocketsRequired))
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return (*FutureNotifyBlocksResult)(newNilFutureResult(ctx))
	}

	cmd := chainjson.NewNotifyBlocksCmd()
	return (*FutureNotifyBlocksResult)(c.sendCmd(ctx, cmd))
}

// NotifyWorkAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See NotifyWork for the blocking version and more details.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyWorkAsync(ctx context.Context) *FutureNotifyWorkResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return (*FutureNotifyWorkResult)(newFutureError(ctx, ErrWebsocketsRequired))
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return (*FutureNotifyWorkResult)(newNilFutureResult(ctx))
	}

	cmd := chainjson.NewNotifyWorkCmd()
	return (*FutureNotifyWorkResult)(c.sendCmd(ctx, cmd))
}

// NotifyTSpendAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See NotifyTSpend for the blocking version and more details.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyTSpendAsync(ctx context.Context) *FutureNotifyTSpendResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return (*FutureNotifyTSpendResult)(newFutureError(ctx, ErrWebsocketsRequired))
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return (*FutureNotifyTSpendResult)(newNilFutureResult(ctx))
	}

	cmd := chainjson.NewNotifyTSpendCmd()
	return (*FutureNotifyTSpendResult)(c.sendCmd(ctx, cmd))
}

// NotifyBlocks registers the client to receive notifications when blocks are
// connected and disconnected from the main chain.  The notifications are
// delivered to the notification handlers associated with the client.  Calling
// this function has no effect if there are no notification handlers and will
// result in an error if the client is configured to run in HTTP POST mode.
//
// The notifications delivered as a result of this call will be via one of
// OnBlockConnected or OnBlockDisconnected.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyBlocks(ctx context.Context) error {
	return c.NotifyBlocksAsync(ctx).Receive()
}

// NotifyWork registers the client to receive notifications when a new block
// template has been generated.
//
// The notifications delivered as a result of this call will be via OnWork.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyWork(ctx context.Context) error {
	return c.NotifyWorkAsync(ctx).Receive()
}

// NotifyTSpend registers the client to receive notifications when a new tspend
// arrives in the mempool.
//
// The notifications delivered as a result of this call will be via one of
// OnTSpend.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyTSpend(ctx context.Context) error {
	return c.NotifyTSpendAsync(ctx).Receive()
}

// FutureNotifyWinningTicketsResult is a future promise to deliver the result of a
// NotifyWinningTicketsAsync RPC invocation (or an applicable error).
type FutureNotifyWinningTicketsResult cmdRes

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r *FutureNotifyWinningTicketsResult) Receive() error {
	_, err := receiveFuture(r.ctx, r.c)
	return err
}

// NotifyWinningTicketsAsync returns an instance of a type that can be used
// to get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See NotifyWinningTickets for the blocking version and more details.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyWinningTicketsAsync(ctx context.Context) *FutureNotifyWinningTicketsResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return (*FutureNotifyWinningTicketsResult)(newFutureError(ctx, ErrWebsocketsRequired))
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return (*FutureNotifyWinningTicketsResult)(newNilFutureResult(ctx))
	}

	cmd := chainjson.NewNotifyWinningTicketsCmd()

	return (*FutureNotifyWinningTicketsResult)(c.sendCmd(ctx, cmd))
}

// NotifyWinningTickets registers the client to receive notifications when
// blocks are connected to the main chain and tickets are chosen to vote.  The
// notifications are delivered to the notification handlers associated with the
// client.  Calling this function has no effect if there are no notification
// handlers and will result in an error if the client is configured to run in HTTP
// POST mode.
//
// The notifications delivered as a result of this call will be those from
// OnWinningTickets.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyWinningTickets(ctx context.Context) error {
	return c.NotifyWinningTicketsAsync(ctx).Receive()
}

// FutureNotifySpentAndMissedTicketsResult is a future promise to deliver the result of a
// NotifySpentAndMissedTicketsAsync RPC invocation (or an applicable error).
type FutureNotifySpentAndMissedTicketsResult cmdRes

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r *FutureNotifySpentAndMissedTicketsResult) Receive() error {
	_, err := receiveFuture(r.ctx, r.c)
	return err
}

// NotifySpentAndMissedTicketsAsync returns an instance of a type that can be used
// to get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See NotifySpentAndMissedTickets for the blocking version and more details.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifySpentAndMissedTicketsAsync(ctx context.Context) *FutureNotifySpentAndMissedTicketsResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return (*FutureNotifySpentAndMissedTicketsResult)(newFutureError(ctx, ErrWebsocketsRequired))
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return (*FutureNotifySpentAndMissedTicketsResult)(newNilFutureResult(ctx))
	}

	cmd := chainjson.NewNotifySpentAndMissedTicketsCmd()

	return (*FutureNotifySpentAndMissedTicketsResult)(c.sendCmd(ctx, cmd))
}

// NotifySpentAndMissedTickets registers the client to receive notifications when
// blocks are connected to the main chain and tickets are spent or missed.  The
// notifications are delivered to the notification handlers associated with the
// client.  Calling this function has no effect if there are no notification
// handlers and will result in an error if the client is configured to run in HTTP
// POST mode.
//
// The notifications delivered as a result of this call will be those from
// OnSpentAndMissedTickets.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifySpentAndMissedTickets(ctx context.Context) error {
	return c.NotifySpentAndMissedTicketsAsync(ctx).Receive()
}

// FutureNotifyNewTicketsResult is a future promise to deliver the result of a
// NotifyNewTicketsAsync RPC invocation (or an applicable error).
type FutureNotifyNewTicketsResult cmdRes

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r *FutureNotifyNewTicketsResult) Receive() error {
	_, err := receiveFuture(r.ctx, r.c)
	return err
}

// NotifyNewTicketsAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See NotifyNewTickets for the blocking version and more details.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyNewTicketsAsync(ctx context.Context) *FutureNotifyNewTicketsResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return (*FutureNotifyNewTicketsResult)(newFutureError(ctx, ErrWebsocketsRequired))
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return (*FutureNotifyNewTicketsResult)(newNilFutureResult(ctx))
	}

	cmd := chainjson.NewNotifyNewTicketsCmd()

	return (*FutureNotifyNewTicketsResult)(c.sendCmd(ctx, cmd))
}

// NotifyNewTickets registers the client to receive notifications when blocks are
// connected to the main chain and new tickets have matured.  The notifications are
// delivered to the notification handlers associated with the client.  Calling
// this function has no effect if there are no notification handlers and will
// result in an error if the client is configured to run in HTTP POST mode.
//
// The notifications delivered as a result of this call will be via OnNewTickets.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyNewTickets(ctx context.Context) error {
	return c.NotifyNewTicketsAsync(ctx).Receive()
}

// FutureNotifyNewTransactionsResult is a future promise to deliver the result
// of a NotifyNewTransactionsAsync RPC invocation (or an applicable error).
type FutureNotifyNewTransactionsResult cmdRes

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r *FutureNotifyNewTransactionsResult) Receive() error {
	_, err := receiveFuture(r.ctx, r.c)
	return err
}

// NotifyNewTransactionsAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See NotifyNewTransactions for the blocking version and more details.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyNewTransactionsAsync(ctx context.Context, verbose bool) *FutureNotifyNewTransactionsResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return (*FutureNotifyNewTransactionsResult)(newFutureError(ctx, ErrWebsocketsRequired))
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return (*FutureNotifyNewTransactionsResult)(newNilFutureResult(ctx))
	}

	cmd := chainjson.NewNotifyNewTransactionsCmd(&verbose)
	return (*FutureNotifyNewTransactionsResult)(c.sendCmd(ctx, cmd))
}

// NotifyNewTransactions registers the client to receive notifications every
// time a new transaction is accepted to the memory pool.  The notifications are
// delivered to the notification handlers associated with the client.  Calling
// this function has no effect if there are no notification handlers and will
// result in an error if the client is configured to run in HTTP POST mode.
//
// The notifications delivered as a result of this call will be via one of
// OnTxAccepted (when verbose is false) or OnTxAcceptedVerbose (when verbose is
// true).
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyNewTransactions(ctx context.Context, verbose bool) error {
	return c.NotifyNewTransactionsAsync(ctx, verbose).Receive()
}

// FutureLoadTxFilterResult is a future promise to deliver the result
// of a LoadTxFilterAsync RPC invocation (or an applicable error).
type FutureLoadTxFilterResult cmdRes

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r *FutureLoadTxFilterResult) Receive() error {
	_, err := receiveFuture(r.ctx, r.c)
	return err
}

// LoadTxFilterAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See LoadTxFilter for the blocking version and more details.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) LoadTxFilterAsync(ctx context.Context, reload bool, addresses []stdaddr.Address,
	outPoints []wire.OutPoint) *FutureLoadTxFilterResult {

	addrStrs := make([]string, len(addresses))
	for i, a := range addresses {
		addrStrs[i] = a.Address()
	}
	outPointObjects := make([]chainjson.OutPoint, len(outPoints))
	for i := range outPoints {
		outPointObjects[i] = chainjson.OutPoint{
			Hash:  outPoints[i].Hash.String(),
			Index: outPoints[i].Index,
			Tree:  outPoints[i].Tree,
		}
	}

	cmd := chainjson.NewLoadTxFilterCmd(reload, addrStrs, outPointObjects)
	return (*FutureLoadTxFilterResult)(c.sendCmd(ctx, cmd))
}

// LoadTxFilter loads, reloads, or adds data to a websocket client's transaction
// filter.  The filter is consistently updated based on inspected transactions
// during mempool acceptance, block acceptance, and for all rescanned blocks.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) LoadTxFilter(ctx context.Context, reload bool, addresses []stdaddr.Address, outPoints []wire.OutPoint) error {
	return c.LoadTxFilterAsync(ctx, reload, addresses, outPoints).Receive()
}
