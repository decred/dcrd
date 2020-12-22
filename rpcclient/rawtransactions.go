// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrjson/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v3"
	"github.com/decred/dcrd/wire"
)

// FutureGetRawTransactionResult is a future promise to deliver the result of a
// GetRawTransactionAsync RPC invocation (or an applicable error).
type FutureGetRawTransactionResult cmdRes

// Receive waits for the response promised by the future and returns a
// transaction given its hash.
func (r *FutureGetRawTransactionResult) Receive() (*dcrutil.Tx, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var txHex string
	err = json.Unmarshal(res, &txHex)
	if err != nil {
		return nil, err
	}

	// Decode the serialized transaction hex to raw bytes.
	serializedTx, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, err
	}

	// Deserialize the transaction and return it.
	var msgTx wire.MsgTx
	if err := msgTx.Deserialize(bytes.NewReader(serializedTx)); err != nil {
		return nil, err
	}
	return dcrutil.NewTx(&msgTx), nil
}

// GetRawTransactionAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetRawTransaction for the blocking version and more details.
func (c *Client) GetRawTransactionAsync(ctx context.Context, txHash *chainhash.Hash) *FutureGetRawTransactionResult {
	hash := ""
	if txHash != nil {
		hash = txHash.String()
	}

	cmd := chainjson.NewGetRawTransactionCmd(hash, dcrjson.Int(0))
	return (*FutureGetRawTransactionResult)(c.sendCmd(ctx, cmd))
}

// GetRawTransaction returns a transaction given its hash.
//
// See GetRawTransactionVerbose to obtain additional information about the
// transaction.
func (c *Client) GetRawTransaction(ctx context.Context, txHash *chainhash.Hash) (*dcrutil.Tx, error) {
	return c.GetRawTransactionAsync(ctx, txHash).Receive()
}

// FutureGetRawTransactionVerboseResult is a future promise to deliver the
// result of a GetRawTransactionVerboseAsync RPC invocation (or an applicable
// error).
type FutureGetRawTransactionVerboseResult cmdRes

// Receive waits for the response promised by the future and returns information
// about a transaction given its hash.
func (r *FutureGetRawTransactionVerboseResult) Receive() (*chainjson.TxRawResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a gettrawtransaction result object.
	var rawTxResult chainjson.TxRawResult
	err = json.Unmarshal(res, &rawTxResult)
	if err != nil {
		return nil, err
	}

	return &rawTxResult, nil
}

// GetRawTransactionVerboseAsync returns an instance of a type that can be used
// to get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetRawTransactionVerbose for the blocking version and more details.
func (c *Client) GetRawTransactionVerboseAsync(ctx context.Context, txHash *chainhash.Hash) *FutureGetRawTransactionVerboseResult {
	hash := ""
	if txHash != nil {
		hash = txHash.String()
	}

	cmd := chainjson.NewGetRawTransactionCmd(hash, dcrjson.Int(1))
	return (*FutureGetRawTransactionVerboseResult)(c.sendCmd(ctx, cmd))
}

// GetRawTransactionVerbose returns information about a transaction given
// its hash.
//
// See GetRawTransaction to obtain only the transaction already deserialized.
func (c *Client) GetRawTransactionVerbose(ctx context.Context, txHash *chainhash.Hash) (*chainjson.TxRawResult, error) {
	return c.GetRawTransactionVerboseAsync(ctx, txHash).Receive()
}

// FutureDecodeRawTransactionResult is a future promise to deliver the result
// of a DecodeRawTransactionAsync RPC invocation (or an applicable error).
type FutureDecodeRawTransactionResult cmdRes

// Receive waits for the response promised by the future and returns information
// about a transaction given its serialized bytes.
func (r *FutureDecodeRawTransactionResult) Receive() (*chainjson.TxRawResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a decoderawtransaction result object.
	var rawTxResult chainjson.TxRawResult
	err = json.Unmarshal(res, &rawTxResult)
	if err != nil {
		return nil, err
	}

	return &rawTxResult, nil
}

// DecodeRawTransactionAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See DecodeRawTransaction for the blocking version and more details.
func (c *Client) DecodeRawTransactionAsync(ctx context.Context, serializedTx []byte) *FutureDecodeRawTransactionResult {
	txHex := hex.EncodeToString(serializedTx)
	cmd := chainjson.NewDecodeRawTransactionCmd(txHex)
	return (*FutureDecodeRawTransactionResult)(c.sendCmd(ctx, cmd))
}

// DecodeRawTransaction returns information about a transaction given its
// serialized bytes.
func (c *Client) DecodeRawTransaction(ctx context.Context, serializedTx []byte) (*chainjson.TxRawResult, error) {
	return c.DecodeRawTransactionAsync(ctx, serializedTx).Receive()
}

// FutureCreateRawTransactionResult is a future promise to deliver the result
// of a CreateRawTransactionAsync RPC invocation (or an applicable error).
type FutureCreateRawTransactionResult cmdRes

// Receive waits for the response promised by the future and returns a new
// transaction spending the provided inputs and sending to the provided
// addresses.
func (r *FutureCreateRawTransactionResult) Receive() (*wire.MsgTx, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var txHex string
	err = json.Unmarshal(res, &txHex)
	if err != nil {
		return nil, err
	}

	// Decode the serialized transaction hex to raw bytes.
	serializedTx, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, err
	}

	// Deserialize the transaction and return it.
	var msgTx wire.MsgTx
	if err := msgTx.Deserialize(bytes.NewReader(serializedTx)); err != nil {
		return nil, err
	}
	return &msgTx, nil
}

// CreateRawTransactionAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See CreateRawTransaction for the blocking version and more details.
func (c *Client) CreateRawTransactionAsync(ctx context.Context, inputs []chainjson.TransactionInput,
	amounts map[dcrutil.Address]dcrutil.Amount, lockTime *int64, expiry *int64) *FutureCreateRawTransactionResult {

	convertedAmts := make(map[string]float64, len(amounts))
	for addr, amount := range amounts {
		convertedAmts[addr.String()] = amount.ToCoin()
	}
	cmd := chainjson.NewCreateRawTransactionCmd(inputs, convertedAmts, lockTime, expiry)
	return (*FutureCreateRawTransactionResult)(c.sendCmd(ctx, cmd))
}

// CreateRawTransaction returns a new transaction spending the provided inputs
// and sending to the provided addresses.
func (c *Client) CreateRawTransaction(ctx context.Context, inputs []chainjson.TransactionInput,
	amounts map[dcrutil.Address]dcrutil.Amount, lockTime *int64, expiry *int64) (*wire.MsgTx, error) {

	return c.CreateRawTransactionAsync(ctx, inputs, amounts, lockTime, expiry).Receive()
}

// FutureCreateRawSStxResult is a future promise to deliver the result
// of a CreateRawSStxAsync RPC invocation (or an applicable error).
type FutureCreateRawSStxResult cmdRes

// Receive waits for the response promised by the future and returns a new
// transaction spending the provided inputs and sending to the provided
// addresses.
func (r *FutureCreateRawSStxResult) Receive() (*wire.MsgTx, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var txHex string
	err = json.Unmarshal(res, &txHex)
	if err != nil {
		return nil, err
	}

	// Decode the serialized transaction hex to raw bytes.
	serializedTx, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, err
	}

	// Deserialize the transaction and return it.
	var msgTx wire.MsgTx
	if err := msgTx.Deserialize(bytes.NewReader(serializedTx)); err != nil {
		return nil, err
	}
	return &msgTx, nil
}

// SStxCommitOut represents the output to an SStx transaction. Specifically a
// a commitment address and amount, and a change address and amount. Same
// name as the JSON lib, but different internal structures.
type SStxCommitOut struct {
	Addr       dcrutil.Address
	CommitAmt  dcrutil.Amount
	ChangeAddr dcrutil.Address
	ChangeAmt  dcrutil.Amount
}

// CreateRawSStxAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See CreateRawSStx for the blocking version and more details.
func (c *Client) CreateRawSStxAsync(ctx context.Context, inputs []chainjson.SStxInput,
	amount map[dcrutil.Address]dcrutil.Amount,
	couts []SStxCommitOut) *FutureCreateRawSStxResult {

	convertedAmt := make(map[string]int64, len(amount))
	for addr, amt := range amount {
		convertedAmt[addr.String()] = int64(amt)
	}
	convertedCouts := make([]chainjson.SStxCommitOut, len(couts))
	for i, cout := range couts {
		convertedCouts[i].Addr = cout.Addr.String()
		convertedCouts[i].CommitAmt = int64(cout.CommitAmt)
		convertedCouts[i].ChangeAddr = cout.ChangeAddr.String()
		convertedCouts[i].ChangeAmt = int64(cout.ChangeAmt)
	}

	cmd := chainjson.NewCreateRawSStxCmd(inputs, convertedAmt, convertedCouts)
	return (*FutureCreateRawSStxResult)(c.sendCmd(ctx, cmd))
}

// CreateRawSStx returns a new transaction spending the provided inputs
// and sending to the provided addresses.
func (c *Client) CreateRawSStx(ctx context.Context, inputs []chainjson.SStxInput,
	amount map[dcrutil.Address]dcrutil.Amount,
	couts []SStxCommitOut) (*wire.MsgTx, error) {

	return c.CreateRawSStxAsync(ctx, inputs, amount, couts).Receive()
}

// FutureCreateRawSSRtxResult is a future promise to deliver the result
// of a CreateRawSSRtxAsync RPC invocation (or an applicable error).
type FutureCreateRawSSRtxResult cmdRes

// Receive waits for the response promised by the future and returns a new
// transaction spending the provided inputs and sending to the provided
// addresses.
func (r *FutureCreateRawSSRtxResult) Receive() (*wire.MsgTx, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var txHex string
	err = json.Unmarshal(res, &txHex)
	if err != nil {
		return nil, err
	}

	// Decode the serialized transaction hex to raw bytes.
	serializedTx, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, err
	}

	// Deserialize the transaction and return it.
	var msgTx wire.MsgTx
	if err := msgTx.Deserialize(bytes.NewReader(serializedTx)); err != nil {
		return nil, err
	}
	return &msgTx, nil
}

// CreateRawSSRtxAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See CreateRawSSRtx for the blocking version and more details.
func (c *Client) CreateRawSSRtxAsync(ctx context.Context, inputs []chainjson.TransactionInput, fee dcrutil.Amount) *FutureCreateRawSSRtxResult {
	feeF64 := fee.ToCoin()
	cmd := chainjson.NewCreateRawSSRtxCmd(inputs, &feeF64)
	return (*FutureCreateRawSSRtxResult)(c.sendCmd(ctx, cmd))
}

// CreateRawSSRtx returns a new SSR transaction (revoking an sstx).
func (c *Client) CreateRawSSRtx(ctx context.Context, inputs []chainjson.TransactionInput, fee dcrutil.Amount) (*wire.MsgTx, error) {
	return c.CreateRawSSRtxAsync(ctx, inputs, fee).Receive()
}

// FutureSendRawTransactionResult is a future promise to deliver the result
// of a SendRawTransactionAsync RPC invocation (or an applicable error).
type FutureSendRawTransactionResult cmdRes

// Receive waits for the response promised by the future and returns the result
// of submitting the encoded transaction to the server which then relays it to
// the network.
func (r *FutureSendRawTransactionResult) Receive() (*chainhash.Hash, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var txHashStr string
	err = json.Unmarshal(res, &txHashStr)
	if err != nil {
		return nil, err
	}

	return chainhash.NewHashFromStr(txHashStr)
}

// SendRawTransactionAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See SendRawTransaction for the blocking version and more details.
func (c *Client) SendRawTransactionAsync(ctx context.Context, tx *wire.MsgTx, allowHighFees bool) *FutureSendRawTransactionResult {
	txHex := ""
	if tx != nil {
		// Serialize the transaction and convert to hex string.
		buf := bytes.NewBuffer(make([]byte, 0, tx.SerializeSize()))
		if err := tx.Serialize(buf); err != nil {
			return (*FutureSendRawTransactionResult)(newFutureError(ctx, err))
		}
		txHex = hex.EncodeToString(buf.Bytes())
	}

	cmd := chainjson.NewSendRawTransactionCmd(txHex, &allowHighFees)
	return (*FutureSendRawTransactionResult)(c.sendCmd(ctx, cmd))
}

// SendRawTransaction submits the encoded transaction to the server which will
// then relay it to the network.
func (c *Client) SendRawTransaction(ctx context.Context, tx *wire.MsgTx, allowHighFees bool) (*chainhash.Hash, error) {
	return c.SendRawTransactionAsync(ctx, tx, allowHighFees).Receive()
}

// FutureSearchRawTransactionsResult is a future promise to deliver the result
// of the SearchRawTransactionsAsync RPC invocation (or an applicable error).
type FutureSearchRawTransactionsResult cmdRes

// Receive waits for the response promised by the future and returns the
// found raw transactions.
func (r *FutureSearchRawTransactionsResult) Receive() ([]*wire.MsgTx, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal as an array of strings.
	var searchRawTxnsResult []string
	err = json.Unmarshal(res, &searchRawTxnsResult)
	if err != nil {
		return nil, err
	}

	// Decode and deserialize each transaction.
	msgTxns := make([]*wire.MsgTx, 0, len(searchRawTxnsResult))
	for _, hexTx := range searchRawTxnsResult {
		// Decode the serialized transaction hex to raw bytes.
		serializedTx, err := hex.DecodeString(hexTx)
		if err != nil {
			return nil, err
		}

		// Deserialize the transaction and add it to the result slice.
		var msgTx wire.MsgTx
		err = msgTx.Deserialize(bytes.NewReader(serializedTx))
		if err != nil {
			return nil, err
		}
		msgTxns = append(msgTxns, &msgTx)
	}

	return msgTxns, nil
}

// SearchRawTransactionsAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See SearchRawTransactions for the blocking version and more details.
func (c *Client) SearchRawTransactionsAsync(ctx context.Context,
	address dcrutil.Address, skip, count int, reverse bool,
	filterAddrs []string) *FutureSearchRawTransactionsResult {

	addr := address.Address()
	verbose := dcrjson.Int(0)
	prevOut := dcrjson.Int(0)
	cmd := chainjson.NewSearchRawTransactionsCmd(addr, verbose, &skip, &count,
		prevOut, &reverse, &filterAddrs)
	return (*FutureSearchRawTransactionsResult)(c.sendCmd(ctx, cmd))
}

// SearchRawTransactions returns transactions that involve the passed address.
//
// NOTE: Chain servers do not typically provide this capability unless it has
// specifically been enabled.
//
// See SearchRawTransactionsVerbose to retrieve a list of data structures with
// information about the transactions instead of the transactions themselves.
func (c *Client) SearchRawTransactions(ctx context.Context,
	address dcrutil.Address, skip, count int,
	reverse bool, filterAddrs []string) ([]*wire.MsgTx, error) {

	return c.SearchRawTransactionsAsync(ctx, address, skip, count, reverse,
		filterAddrs).Receive()
}

// FutureSearchRawTransactionsVerboseResult is a future promise to deliver the
// result of the SearchRawTransactionsVerboseAsync RPC invocation (or an
// applicable error).
type FutureSearchRawTransactionsVerboseResult cmdRes

// Receive waits for the response promised by the future and returns the
// found raw transactions.
func (r *FutureSearchRawTransactionsVerboseResult) Receive() ([]*chainjson.SearchRawTransactionsResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal as an array of raw transaction results.
	var result []*chainjson.SearchRawTransactionsResult
	err = json.Unmarshal(res, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// SearchRawTransactionsVerboseAsync returns an instance of a type that can be
// used to get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See SearchRawTransactionsVerbose for the blocking version and more details.
func (c *Client) SearchRawTransactionsVerboseAsync(ctx context.Context,
	address dcrutil.Address, skip, count int, includePrevOut bool, reverse bool,
	filterAddrs *[]string) *FutureSearchRawTransactionsVerboseResult {

	addr := address.Address()
	verbose := dcrjson.Int(1)
	prevOut := dcrjson.Int(0)
	if includePrevOut {
		prevOut = dcrjson.Int(1)
	}
	cmd := chainjson.NewSearchRawTransactionsCmd(addr, verbose, &skip, &count,
		prevOut, &reverse, filterAddrs)
	return (*FutureSearchRawTransactionsVerboseResult)(c.sendCmd(ctx, cmd))
}

// SearchRawTransactionsVerbose returns a list of data structures that describe
// transactions which involve the passed address.
//
// NOTE: Chain servers do not typically provide this capability unless it has
// specifically been enabled.
//
// See SearchRawTransactions to retrieve a list of raw transactions instead.
func (c *Client) SearchRawTransactionsVerbose(ctx context.Context,
	address dcrutil.Address, skip, count int, includePrevOut, reverse bool,
	filterAddrs []string) ([]*chainjson.SearchRawTransactionsResult, error) {

	return c.SearchRawTransactionsVerboseAsync(ctx, address, skip, count,
		includePrevOut, reverse, &filterAddrs).Receive()
}
