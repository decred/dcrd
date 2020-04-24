// Copyright (c) 2014-2015 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"context"
	"encoding/json"

	walletjson "decred.org/dcrwallet/rpc/jsonrpc/types"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v3"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v2"
	"github.com/decred/dcrd/wire"
)

var (
	// zeroUint32 is the zero value for a uint32.
	zeroUint32 = uint32(0)
)

// FutureDebugLevelResult is a future promise to deliver the result of a
// DebugLevelAsync RPC invocation (or an applicable error).
type FutureDebugLevelResult chan *response

// Receive waits for the response promised by the future and returns the result
// of setting the debug logging level to the passed level specification or the
// list of the available subsystems for the special keyword 'show'.
func (r FutureDebugLevelResult) Receive() (string, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return "", err
	}

	// Unmarshal the result as a string.
	var result string
	err = json.Unmarshal(res, &result)
	if err != nil {
		return "", err
	}
	return result, nil
}

// DebugLevelAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See DebugLevel for the blocking version and more details.
//
// NOTE: This is a dcrd extension.
func (c *Client) DebugLevelAsync(ctx context.Context, levelSpec string) FutureDebugLevelResult {
	cmd := chainjson.NewDebugLevelCmd(levelSpec)
	return c.sendCmd(ctx, cmd)
}

// DebugLevel dynamically sets the debug logging level to the passed level
// specification.
//
// The levelspec can be either a debug level or of the form:
// 	<subsystem>=<level>,<subsystem2>=<level2>,...
//
// Additionally, the special keyword 'show' can be used to get a list of the
// available subsystems.
//
// NOTE: This is a dcrd extension.
func (c *Client) DebugLevel(ctx context.Context, levelSpec string) (string, error) {
	return c.DebugLevelAsync(ctx, levelSpec).Receive()
}

// FutureEstimateStakeDiffResult is a future promise to deliver the result of a
// EstimateStakeDiffAsync RPC invocation (or an applicable error).
type FutureEstimateStakeDiffResult chan *response

// Receive waits for the response promised by the future and returns the hash
// and height of the block in the longest (best) chain.
func (r FutureEstimateStakeDiffResult) Receive() (*chainjson.EstimateStakeDiffResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as an estimatestakediff result object.
	var est chainjson.EstimateStakeDiffResult
	err = json.Unmarshal(res, &est)
	if err != nil {
		return nil, err
	}

	return &est, nil
}

// EstimateStakeDiffAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See EstimateStakeDiff for the blocking version and more details.
//
// NOTE: This is a dcrd extension.
func (c *Client) EstimateStakeDiffAsync(ctx context.Context, tickets *uint32) FutureEstimateStakeDiffResult {
	cmd := chainjson.NewEstimateStakeDiffCmd(tickets)
	return c.sendCmd(ctx, cmd)
}

// EstimateStakeDiff returns the minimum, maximum, and expected next stake
// difficulty.
//
// NOTE: This is a dcrd extension.
func (c *Client) EstimateStakeDiff(ctx context.Context, tickets *uint32) (*chainjson.EstimateStakeDiffResult, error) {
	return c.EstimateStakeDiffAsync(ctx, tickets).Receive()
}

// FutureExistsAddressResult is a future promise to deliver the result
// of a FutureExistsAddressResultAsync RPC invocation (or an applicable error).
type FutureExistsAddressResult chan *response

// Receive waits for the response promised by the future and returns whether or
// not an address exists in the blockchain or mempool.
func (r FutureExistsAddressResult) Receive() (bool, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return false, err
	}

	// Unmarshal the result as a bool.
	var exists bool
	err = json.Unmarshal(res, &exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// ExistsAddressAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
func (c *Client) ExistsAddressAsync(ctx context.Context, address dcrutil.Address) FutureExistsAddressResult {
	cmd := chainjson.NewExistsAddressCmd(address.Address())
	return c.sendCmd(ctx, cmd)
}

// ExistsAddress returns information about whether or not an address has been
// used on the main chain or in mempool.
//
// NOTE: This is a dcrd extension.
func (c *Client) ExistsAddress(ctx context.Context, address dcrutil.Address) (bool, error) {
	return c.ExistsAddressAsync(ctx, address).Receive()
}

// FutureExistsAddressesResult is a future promise to deliver the result
// of a FutureExistsAddressesResultAsync RPC invocation (or an
// applicable error).
type FutureExistsAddressesResult chan *response

// Receive waits for the response promised by the future and returns whether
// or not the addresses exist.
func (r FutureExistsAddressesResult) Receive() (string, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return "", err
	}

	// Unmarshal the result as a string.
	var exists string
	err = json.Unmarshal(res, &exists)
	if err != nil {
		return "", err
	}
	return exists, nil
}

// ExistsAddressesAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
func (c *Client) ExistsAddressesAsync(ctx context.Context, addresses []dcrutil.Address) FutureExistsAddressesResult {
	addrsStr := make([]string, len(addresses))
	for i := range addresses {
		addrsStr[i] = addresses[i].Address()
	}

	cmd := chainjson.NewExistsAddressesCmd(addrsStr)
	return c.sendCmd(ctx, cmd)
}

// ExistsAddresses returns information about whether or not an address exists
// in the blockchain or memory pool.
//
// NOTE: This is a dcrd extension.
func (c *Client) ExistsAddresses(ctx context.Context, addresses []dcrutil.Address) (string, error) {
	return c.ExistsAddressesAsync(ctx, addresses).Receive()
}

// FutureExistsMissedTicketsResult is a future promise to deliver the result of
// an ExistsMissedTicketsAsync RPC invocation (or an applicable error).
type FutureExistsMissedTicketsResult chan *response

// Receive waits for the response promised by the future and returns whether
// or not the ticket exists in the missed ticket database.
func (r FutureExistsMissedTicketsResult) Receive() (string, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return "", err
	}

	// Unmarshal the result as a string.
	var exists string
	err = json.Unmarshal(res, &exists)
	if err != nil {
		return "", err
	}
	return exists, nil
}

// ExistsMissedTicketsAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
func (c *Client) ExistsMissedTicketsAsync(ctx context.Context, hashes []*chainhash.Hash) FutureExistsMissedTicketsResult {
	strHashes := make([]string, len(hashes))
	for i := range hashes {
		strHashes[i] = hashes[i].String()
	}
	cmd := chainjson.NewExistsMissedTicketsCmd(strHashes)
	return c.sendCmd(ctx, cmd)
}

// ExistsMissedTickets returns a hex-encoded bitset describing whether or not
// ticket hashes exists in the missed ticket database.
func (c *Client) ExistsMissedTickets(ctx context.Context, hashes []*chainhash.Hash) (string, error) {
	return c.ExistsMissedTicketsAsync(ctx, hashes).Receive()
}

// FutureExistsExpiredTicketsResult is a future promise to deliver the result
// of a FutureExistsExpiredTicketsResultAsync RPC invocation (or an
// applicable error).
type FutureExistsExpiredTicketsResult chan *response

// Receive waits for the response promised by the future and returns whether
// or not the ticket exists in the live ticket database.
func (r FutureExistsExpiredTicketsResult) Receive() (string, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return "", err
	}

	// Unmarshal the result as a string.
	var exists string
	err = json.Unmarshal(res, &exists)
	if err != nil {
		return "", err
	}
	return exists, nil
}

// ExistsExpiredTicketsAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
func (c *Client) ExistsExpiredTicketsAsync(ctx context.Context, hashes []*chainhash.Hash) FutureExistsExpiredTicketsResult {
	strHashes := make([]string, len(hashes))
	for i := range hashes {
		strHashes[i] = hashes[i].String()
	}
	cmd := chainjson.NewExistsExpiredTicketsCmd(strHashes)
	return c.sendCmd(ctx, cmd)
}

// ExistsExpiredTickets returns information about whether or not a ticket hash exists
// in the expired ticket database.
//
// NOTE: This is a dcrd extension.
func (c *Client) ExistsExpiredTickets(ctx context.Context, hashes []*chainhash.Hash) (string, error) {
	return c.ExistsExpiredTicketsAsync(ctx, hashes).Receive()
}

// FutureExistsLiveTicketResult is a future promise to deliver the result
// of a FutureExistsLiveTicketResultAsync RPC invocation (or an
// applicable error).
type FutureExistsLiveTicketResult chan *response

// Receive waits for the response promised by the future and returns whether
// or not the ticket exists in the live ticket database.
func (r FutureExistsLiveTicketResult) Receive() (bool, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return false, err
	}

	// Unmarshal the result as a bool.
	var exists bool
	err = json.Unmarshal(res, &exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// ExistsLiveTicketAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
func (c *Client) ExistsLiveTicketAsync(ctx context.Context, hash *chainhash.Hash) FutureExistsLiveTicketResult {
	cmd := chainjson.NewExistsLiveTicketCmd(hash.String())
	return c.sendCmd(ctx, cmd)
}

// ExistsLiveTicket returns information about whether or not a ticket hash exists
// in the live ticket database.
//
// NOTE: This is a dcrd extension.
func (c *Client) ExistsLiveTicket(ctx context.Context, hash *chainhash.Hash) (bool, error) {
	return c.ExistsLiveTicketAsync(ctx, hash).Receive()
}

// FutureExistsLiveTicketsResult is a future promise to deliver the result
// of a FutureExistsLiveTicketsResultAsync RPC invocation (or an
// applicable error).
type FutureExistsLiveTicketsResult chan *response

// Receive waits for the response promised by the future and returns whether
// or not the ticket exists in the live ticket database.
func (r FutureExistsLiveTicketsResult) Receive() (string, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return "", err
	}

	// Unmarshal the result as a string.
	var exists string
	err = json.Unmarshal(res, &exists)
	if err != nil {
		return "", err
	}
	return exists, nil
}

// ExistsLiveTicketsAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
func (c *Client) ExistsLiveTicketsAsync(ctx context.Context, hashes []*chainhash.Hash) FutureExistsLiveTicketsResult {
	strHashes := make([]string, len(hashes))
	for i := range hashes {
		strHashes[i] = hashes[i].String()
	}
	cmd := chainjson.NewExistsLiveTicketsCmd(strHashes)
	return c.sendCmd(ctx, cmd)
}

// ExistsLiveTickets returns information about whether or not a ticket hash exists
// in the live ticket database.
//
// NOTE: This is a dcrd extension.
func (c *Client) ExistsLiveTickets(ctx context.Context, hashes []*chainhash.Hash) (string, error) {
	return c.ExistsLiveTicketsAsync(ctx, hashes).Receive()
}

// FutureExistsMempoolTxsResult is a future promise to deliver the result
// of a FutureExistsMempoolTxsResultAsync RPC invocation (or an
// applicable error).
type FutureExistsMempoolTxsResult chan *response

// Receive waits for the response promised by the future and returns whether
// or not the ticket exists in the mempool.
func (r FutureExistsMempoolTxsResult) Receive() (string, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return "", err
	}

	// Unmarshal the result as a string.
	var exists string
	err = json.Unmarshal(res, &exists)
	if err != nil {
		return "", err
	}
	return exists, nil
}

// ExistsMempoolTxsAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
func (c *Client) ExistsMempoolTxsAsync(ctx context.Context, hashes []*chainhash.Hash) FutureExistsMempoolTxsResult {
	strHashes := make([]string, len(hashes))
	for i := range hashes {
		strHashes[i] = hashes[i].String()
	}
	cmd := chainjson.NewExistsMempoolTxsCmd(strHashes)
	return c.sendCmd(ctx, cmd)
}

// ExistsMempoolTxs returns information about whether or not a ticket hash exists
// in the live ticket database.
//
// NOTE: This is a dcrd extension.
func (c *Client) ExistsMempoolTxs(ctx context.Context, hashes []*chainhash.Hash) (string, error) {
	return c.ExistsMempoolTxsAsync(ctx, hashes).Receive()
}

// FutureGetBestBlockResult is a future promise to deliver the result of a
// GetBestBlockAsync RPC invocation (or an applicable error).
type FutureGetBestBlockResult chan *response

// Receive waits for the response promised by the future and returns the hash
// and height of the block in the longest (best) chain.
func (r FutureGetBestBlockResult) Receive() (*chainhash.Hash, int64, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, 0, err
	}

	// Unmarshal result as a getbestblock result object.
	var bestBlock chainjson.GetBestBlockResult
	err = json.Unmarshal(res, &bestBlock)
	if err != nil {
		return nil, 0, err
	}

	// Convert to hash from string.
	hash, err := chainhash.NewHashFromStr(bestBlock.Hash)
	if err != nil {
		return nil, 0, err
	}

	return hash, bestBlock.Height, nil
}

// GetBestBlockAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetBestBlock for the blocking version and more details.
//
// NOTE: This is a dcrd extension.
func (c *Client) GetBestBlockAsync(ctx context.Context) FutureGetBestBlockResult {
	cmd := chainjson.NewGetBestBlockCmd()
	return c.sendCmd(ctx, cmd)
}

// GetBestBlock returns the hash and height of the block in the longest (best)
// chain.
//
// NOTE: This is a dcrd extension.
func (c *Client) GetBestBlock(ctx context.Context) (*chainhash.Hash, int64, error) {
	return c.GetBestBlockAsync(ctx).Receive()
}

// FutureGetCurrentNetResult is a future promise to deliver the result of a
// GetCurrentNetAsync RPC invocation (or an applicable error).
type FutureGetCurrentNetResult chan *response

// Receive waits for the response promised by the future and returns the network
// the server is running on.
func (r FutureGetCurrentNetResult) Receive() (wire.CurrencyNet, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return 0, err
	}

	// Unmarshal result as an int64.
	var net int64
	err = json.Unmarshal(res, &net)
	if err != nil {
		return 0, err
	}

	return wire.CurrencyNet(net), nil
}

// GetCurrentNetAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetCurrentNet for the blocking version and more details.
//
// NOTE: This is a dcrd extension.
func (c *Client) GetCurrentNetAsync(ctx context.Context) FutureGetCurrentNetResult {
	cmd := chainjson.NewGetCurrentNetCmd()
	return c.sendCmd(ctx, cmd)
}

// GetCurrentNet returns the network the server is running on.
//
// NOTE: This is a dcrd extension.
func (c *Client) GetCurrentNet(ctx context.Context) (wire.CurrencyNet, error) {
	return c.GetCurrentNetAsync(ctx).Receive()
}

// FutureGetHeadersResult is a future promise to deliver the result of a
// getheaders RPC invocation (or an applicable error).
type FutureGetHeadersResult chan *response

// Receive waits for the response promised by the future and returns the
// getheaders result.
func (r FutureGetHeadersResult) Receive() (*chainjson.GetHeadersResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a getheaders result object.
	var vr chainjson.GetHeadersResult
	err = json.Unmarshal(res, &vr)
	if err != nil {
		return nil, err
	}

	return &vr, nil
}

// GetHeadersAsync returns an instance of a type that can be used to get the result
// of the RPC at some future time by invoking the Receive function on the returned instance.
//
// See GetHeaders for the blocking version and more details.
func (c *Client) GetHeadersAsync(ctx context.Context, blockLocators []*chainhash.Hash, hashStop *chainhash.Hash) FutureGetHeadersResult {
	locators := make([]string, len(blockLocators))
	for i := range blockLocators {
		locators[i] = blockLocators[i].String()
	}

	var hashStopString string
	if hashStop != nil {
		hashStopString = hashStop.String()
	}

	cmd := chainjson.NewGetHeadersCmd(locators, hashStopString)
	return c.sendCmd(ctx, cmd)
}

// GetHeaders mimics the wire protocol getheaders and headers messages by
// returning all headers on the main chain after the first known block in the
// locators, up until a block hash matches hashStop.
func (c *Client) GetHeaders(ctx context.Context, blockLocators []*chainhash.Hash, hashStop *chainhash.Hash) (*chainjson.GetHeadersResult, error) {
	return c.GetHeadersAsync(ctx, blockLocators, hashStop).Receive()
}

// FutureGetStakeDifficultyResult is a future promise to deliver the result of a
// GetStakeDifficultyAsync RPC invocation (or an applicable error).
type FutureGetStakeDifficultyResult chan *response

// Receive waits for the response promised by the future and returns the network
// the server is running on.
func (r FutureGetStakeDifficultyResult) Receive() (*chainjson.GetStakeDifficultyResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a getstakedifficult result object.
	var gsdr chainjson.GetStakeDifficultyResult
	err = json.Unmarshal(res, &gsdr)
	if err != nil {
		return nil, err
	}

	return &gsdr, nil
}

// GetStakeDifficultyAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetStakeDifficulty for the blocking version and more details.
//
// NOTE: This is a dcrd extension.
func (c *Client) GetStakeDifficultyAsync(ctx context.Context) FutureGetStakeDifficultyResult {
	cmd := chainjson.NewGetStakeDifficultyCmd()
	return c.sendCmd(ctx, cmd)
}

// GetStakeDifficulty returns the current and next stake difficulty.
//
// NOTE: This is a dcrd extension.
func (c *Client) GetStakeDifficulty(ctx context.Context) (*chainjson.GetStakeDifficultyResult, error) {
	return c.GetStakeDifficultyAsync(ctx).Receive()
}

// FutureGetStakeVersionsResult is a future promise to deliver the result of a
// GetStakeVersionsAsync RPC invocation (or an applicable error).
type FutureGetStakeVersionsResult chan *response

// Receive waits for the response promised by the future and returns the network
// the server is running on.
func (r FutureGetStakeVersionsResult) Receive() (*chainjson.GetStakeVersionsResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a getstakeversions result object.
	var gsvr chainjson.GetStakeVersionsResult
	err = json.Unmarshal(res, &gsvr)
	if err != nil {
		return nil, err
	}

	return &gsvr, nil
}

// GetStakeVersionInfoAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetStakeVersionInfo for the blocking version and more details.
//
// NOTE: This is a dcrd extension.
func (c *Client) GetStakeVersionInfoAsync(ctx context.Context, count int32) FutureGetStakeVersionInfoResult {
	cmd := chainjson.NewGetStakeVersionInfoCmd(count)
	return c.sendCmd(ctx, cmd)
}

// GetStakeVersionInfo returns the stake versions results for past requested intervals (count).
//
// NOTE: This is a dcrd extension.
func (c *Client) GetStakeVersionInfo(ctx context.Context, count int32) (*chainjson.GetStakeVersionInfoResult, error) {
	return c.GetStakeVersionInfoAsync(ctx, count).Receive()
}

// FutureGetStakeVersionInfoResult is a future promise to deliver the result of a
// GetStakeVersionInfoAsync RPC invocation (or an applicable error).
type FutureGetStakeVersionInfoResult chan *response

// Receive waits for the response promised by the future and returns the network
// the server is running on.
func (r FutureGetStakeVersionInfoResult) Receive() (*chainjson.GetStakeVersionInfoResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a getstakeversioninfo result object.
	var gsvr chainjson.GetStakeVersionInfoResult
	err = json.Unmarshal(res, &gsvr)
	if err != nil {
		return nil, err
	}

	return &gsvr, nil
}

// GetStakeVersionsAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetStakeVersions for the blocking version and more details.
//
// NOTE: This is a dcrd extension.
func (c *Client) GetStakeVersionsAsync(ctx context.Context, hash string, count int32) FutureGetStakeVersionsResult {
	cmd := chainjson.NewGetStakeVersionsCmd(hash, count)
	return c.sendCmd(ctx, cmd)
}

// GetStakeVersions returns the stake versions and vote versions of past requested blocks.
//
// NOTE: This is a dcrd extension.
func (c *Client) GetStakeVersions(ctx context.Context, hash string, count int32) (*chainjson.GetStakeVersionsResult, error) {
	return c.GetStakeVersionsAsync(ctx, hash, count).Receive()
}

// FutureGetTicketPoolValueResult is a future promise to deliver the result of a
// GetTicketPoolValueAsync RPC invocation (or an applicable error).
type FutureGetTicketPoolValueResult chan *response

// Receive waits for the response promised by the future and returns the network
// the server is running on.
func (r FutureGetTicketPoolValueResult) Receive() (dcrutil.Amount, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return 0, err
	}

	// Unmarshal result as a float64.
	var val float64
	err = json.Unmarshal(res, &val)
	if err != nil {
		return 0, err
	}

	// Convert to an amount.
	amt, err := dcrutil.NewAmount(val)
	if err != nil {
		return 0, err
	}

	return amt, nil
}

// GetTicketPoolValueAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetTicketPoolValue for the blocking version and more details.
//
// NOTE: This is a dcrd extension.
func (c *Client) GetTicketPoolValueAsync(ctx context.Context) FutureGetTicketPoolValueResult {
	cmd := chainjson.NewGetTicketPoolValueCmd()
	return c.sendCmd(ctx, cmd)
}

// GetTicketPoolValue returns the value of the live ticket pool.
//
// NOTE: This is a dcrd extension.
func (c *Client) GetTicketPoolValue(ctx context.Context) (dcrutil.Amount, error) {
	return c.GetTicketPoolValueAsync(ctx).Receive()
}

// FutureGetVoteInfoResult is a future promise to deliver the result of a
// GetVoteInfoAsync RPC invocation (or an applicable error).
type FutureGetVoteInfoResult chan *response

// Receive waits for the response promised by the future and returns the network
// the server is running on.
func (r FutureGetVoteInfoResult) Receive() (*chainjson.GetVoteInfoResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result getvoteinfo result object.
	var gsvr chainjson.GetVoteInfoResult
	err = json.Unmarshal(res, &gsvr)
	if err != nil {
		return nil, err
	}

	return &gsvr, nil
}

// GetVoteInfoAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetVoteInfo for the blocking version and more details.
//
// NOTE: This is a dcrd extension.
func (c *Client) GetVoteInfoAsync(ctx context.Context, version uint32) FutureGetVoteInfoResult {
	cmd := chainjson.NewGetVoteInfoCmd(version)
	return c.sendCmd(ctx, cmd)
}

// GetVoteInfo returns voting information for the specified stake version. This
// includes current voting window, quorum, total votes and agendas.
//
// NOTE: This is a dcrd extension.
func (c *Client) GetVoteInfo(ctx context.Context, version uint32) (*chainjson.GetVoteInfoResult, error) {
	return c.GetVoteInfoAsync(ctx, version).Receive()
}

// FutureListAddressTransactionsResult is a future promise to deliver the result
// of a ListAddressTransactionsAsync RPC invocation (or an applicable error).
type FutureListAddressTransactionsResult chan *response

// Receive waits for the response promised by the future and returns information
// about all transactions associated with the provided addresses.
func (r FutureListAddressTransactionsResult) Receive() ([]walletjson.ListTransactionsResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal the result as an array of listtransactions objects.
	var transactions []walletjson.ListTransactionsResult
	err = json.Unmarshal(res, &transactions)
	if err != nil {
		return nil, err
	}
	return transactions, nil
}

// ListAddressTransactionsAsync returns an instance of a type that can be used
// to get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See ListAddressTransactions for the blocking version and more details.
//
// NOTE: This is a dcrd extension.
func (c *Client) ListAddressTransactionsAsync(ctx context.Context, addresses []dcrutil.Address, account string) FutureListAddressTransactionsResult {
	// Convert addresses to strings.
	addrs := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		addrs = append(addrs, addr.Address())
	}
	cmd := walletjson.NewListAddressTransactionsCmd(addrs, &account)
	return c.sendCmd(ctx, cmd)
}

// ListAddressTransactions returns information about all transactions associated
// with the provided addresses.
//
// NOTE: This is a dcrwallet extension.
func (c *Client) ListAddressTransactions(ctx context.Context, addresses []dcrutil.Address, account string) ([]walletjson.ListTransactionsResult, error) {
	return c.ListAddressTransactionsAsync(ctx, addresses, account).Receive()
}

// FutureLiveTicketsResult is a future promise to deliver the result
// of a FutureLiveTicketsResultAsync RPC invocation (or an applicable error).
type FutureLiveTicketsResult chan *response

// Receive waits for the response promised by the future and returns all
// currently missed tickets from the missed ticket database.
func (r FutureLiveTicketsResult) Receive() ([]*chainhash.Hash, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a live tickets result object.
	var container chainjson.LiveTicketsResult
	err = json.Unmarshal(res, &container)
	if err != nil {
		return nil, err
	}

	liveTickets := make([]*chainhash.Hash, 0, len(container.Tickets))
	for _, ticketStr := range container.Tickets {
		h, err := chainhash.NewHashFromStr(ticketStr)
		if err != nil {
			return nil, err
		}
		liveTickets = append(liveTickets, h)
	}

	return liveTickets, nil
}

// LiveTicketsAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
func (c *Client) LiveTicketsAsync(ctx context.Context) FutureLiveTicketsResult {
	cmd := chainjson.NewLiveTicketsCmd()
	return c.sendCmd(ctx, cmd)
}

// LiveTickets returns all currently missed tickets from the missed
// ticket database in the daemon.
//
// NOTE: This is a dcrd extension.
func (c *Client) LiveTickets(ctx context.Context) ([]*chainhash.Hash, error) {
	return c.LiveTicketsAsync(ctx).Receive()
}

// FutureMissedTicketsResult is a future promise to deliver the result
// of a FutureMissedTicketsResultAsync RPC invocation (or an applicable error).
type FutureMissedTicketsResult chan *response

// Receive waits for the response promised by the future and returns all
// currently missed tickets from the missed ticket database.
func (r FutureMissedTicketsResult) Receive() ([]*chainhash.Hash, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a missed tickets result object.
	var container chainjson.MissedTicketsResult
	err = json.Unmarshal(res, &container)
	if err != nil {
		return nil, err
	}

	missedTickets := make([]*chainhash.Hash, 0, len(container.Tickets))
	for _, ticketStr := range container.Tickets {
		h, err := chainhash.NewHashFromStr(ticketStr)
		if err != nil {
			return nil, err
		}
		missedTickets = append(missedTickets, h)
	}

	return missedTickets, nil
}

// MissedTicketsAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
func (c *Client) MissedTicketsAsync(ctx context.Context) FutureMissedTicketsResult {
	cmd := chainjson.NewMissedTicketsCmd()
	return c.sendCmd(ctx, cmd)
}

// MissedTickets returns all currently missed tickets from the missed
// ticket database in the daemon.
//
// NOTE: This is a dcrd extension.
func (c *Client) MissedTickets(ctx context.Context) ([]*chainhash.Hash, error) {
	return c.MissedTicketsAsync(ctx).Receive()
}

// FutureSessionResult is a future promise to deliver the result of a
// SessionAsync RPC invocation (or an applicable error).
type FutureSessionResult chan *response

// Receive waits for the response promised by the future and returns the
// session result.
func (r FutureSessionResult) Receive() (*chainjson.SessionResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a session result object.
	var session chainjson.SessionResult
	err = json.Unmarshal(res, &session)
	if err != nil {
		return nil, err
	}

	return &session, nil
}

// SessionAsync returns an instance of a type that can be used to get the result
// of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See Session for the blocking version and more details.
//
// NOTE: This is a Decred extension.
func (c *Client) SessionAsync(ctx context.Context) FutureSessionResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return newFutureError(ErrWebsocketsRequired)
	}

	cmd := chainjson.NewSessionCmd()
	return c.sendCmd(ctx, cmd)
}

// Session returns details regarding a websocket client's current connection.
//
// This RPC requires the client to be running in websocket mode.
//
// NOTE: This is a Decred extension.
func (c *Client) Session(ctx context.Context) (*chainjson.SessionResult, error) {
	return c.SessionAsync(ctx).Receive()
}

// FutureTicketFeeInfoResult is a future promise to deliver the result of a
// TicketFeeInfoAsync RPC invocation (or an applicable error).
type FutureTicketFeeInfoResult chan *response

// Receive waits for the response promised by the future and returns the
// ticketfeeinfo result.
func (r FutureTicketFeeInfoResult) Receive() (*chainjson.TicketFeeInfoResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a ticketfeeinfo result object.
	var tfir chainjson.TicketFeeInfoResult
	err = json.Unmarshal(res, &tfir)
	if err != nil {
		return nil, err
	}

	return &tfir, nil
}

// TicketFeeInfoAsync returns an instance of a type that can be used to get the result
// of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See TicketFeeInfo for the blocking version and more details.
//
// NOTE: This is a Decred extension.
func (c *Client) TicketFeeInfoAsync(ctx context.Context, blocks *uint32, windows *uint32) FutureTicketFeeInfoResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return newFutureError(ErrWebsocketsRequired)
	}

	// Avoid passing actual nil values, since they can cause arguments
	// not to pass. Pass zero values instead.
	if blocks == nil {
		blocks = &zeroUint32
	}
	if windows == nil {
		windows = &zeroUint32
	}

	cmd := chainjson.NewTicketFeeInfoCmd(blocks, windows)
	return c.sendCmd(ctx, cmd)
}

// TicketFeeInfo returns information about ticket fees.
//
// This RPC requires the client to be running in websocket mode.
//
// NOTE: This is a Decred extension.
func (c *Client) TicketFeeInfo(ctx context.Context, blocks *uint32, windows *uint32) (*chainjson.TicketFeeInfoResult, error) {
	return c.TicketFeeInfoAsync(ctx, blocks, windows).Receive()
}

// FutureTicketVWAPResult is a future promise to deliver the result of a
// TicketVWAPAsync RPC invocation (or an applicable error).
type FutureTicketVWAPResult chan *response

// Receive waits for the response promised by the future and returns the
// ticketvwap result.
func (r FutureTicketVWAPResult) Receive() (dcrutil.Amount, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return 0, err
	}

	// Unmarshal result as a ticketvwap result object.
	var vwap float64
	err = json.Unmarshal(res, &vwap)
	if err != nil {
		return 0, err
	}

	amt, err := dcrutil.NewAmount(vwap)
	if err != nil {
		return 0, err
	}

	return amt, nil
}

// TicketVWAPAsync returns an instance of a type that can be used to get the result
// of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See TicketVWAP for the blocking version and more details.
//
// NOTE: This is a Decred extension.
func (c *Client) TicketVWAPAsync(ctx context.Context, start *uint32, end *uint32) FutureTicketVWAPResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return newFutureError(ErrWebsocketsRequired)
	}

	cmd := chainjson.NewTicketVWAPCmd(start, end)
	return c.sendCmd(ctx, cmd)
}

// TicketVWAP returns the vwap weighted average price of tickets.
//
// This RPC requires the client to be running in websocket mode.
//
// NOTE: This is a Decred extension.
func (c *Client) TicketVWAP(ctx context.Context, start *uint32, end *uint32) (dcrutil.Amount, error) {
	return c.TicketVWAPAsync(ctx, start, end).Receive()
}

// FutureTxFeeInfoResult is a future promise to deliver the result of a
// TxFeeInfoAsync RPC invocation (or an applicable error).
type FutureTxFeeInfoResult chan *response

// Receive waits for the response promised by the future and returns the
// txfeeinfo result.
func (r FutureTxFeeInfoResult) Receive() (*chainjson.TxFeeInfoResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a txfeeinfo result object.
	var tfir chainjson.TxFeeInfoResult
	err = json.Unmarshal(res, &tfir)
	if err != nil {
		return nil, err
	}

	return &tfir, nil
}

// TxFeeInfoAsync returns an instance of a type that can be used to get the result
// of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See TxFeeInfo for the blocking version and more details.
//
// NOTE: This is a Decred extension.
func (c *Client) TxFeeInfoAsync(ctx context.Context, blocks *uint32, start *uint32, end *uint32) FutureTxFeeInfoResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return newFutureError(ErrWebsocketsRequired)
	}

	cmd := chainjson.NewTxFeeInfoCmd(blocks, start, end)
	return c.sendCmd(ctx, cmd)
}

// TxFeeInfo returns information about tx fees.
//
// This RPC requires the client to be running in websocket mode.
//
// NOTE: This is a Decred extension.
func (c *Client) TxFeeInfo(ctx context.Context, blocks *uint32, start *uint32, end *uint32) (*chainjson.TxFeeInfoResult, error) {
	return c.TxFeeInfoAsync(ctx, blocks, start, end).Receive()
}

// FutureVersionResult is a future promise to deliver the result of a version
// RPC invocation (or an applicable error).
type FutureVersionResult chan *response

// Receive waits for the response promised by the future and returns the version
// result.
func (r FutureVersionResult) Receive() (map[string]chainjson.VersionResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a version result object.
	var vr map[string]chainjson.VersionResult
	err = json.Unmarshal(res, &vr)
	if err != nil {
		return nil, err
	}

	return vr, nil
}

// VersionAsync returns an instance of a type that can be used to get the result
// of the RPC at some future time by invoking the Receive function on the returned instance.
//
// See Version for the blocking version and more details.
func (c *Client) VersionAsync(ctx context.Context) FutureVersionResult {
	cmd := chainjson.NewVersionCmd()
	return c.sendCmd(ctx, cmd)
}

// Version returns information about the server's JSON-RPC API versions.
func (c *Client) Version(ctx context.Context) (map[string]chainjson.VersionResult, error) {
	return c.VersionAsync(ctx).Receive()
}
