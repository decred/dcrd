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
	"errors"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrjson/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/gcs/v3"
	"github.com/decred/dcrd/gcs/v3/blockcf"
	"github.com/decred/dcrd/gcs/v3/blockcf2"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v2"
	"github.com/decred/dcrd/wire"
)

// FutureGetBestBlockHashResult is a future promise to deliver the result of a
// GetBestBlockAsync RPC invocation (or an applicable error).
type FutureGetBestBlockHashResult cmdRes

// Receive waits for the response promised by the future and returns the hash of
// the best block in the longest block chain.
func (r *FutureGetBestBlockHashResult) Receive() (*chainhash.Hash, error) {
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

// GetBestBlockHashAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetBestBlockHash for the blocking version and more details.
func (c *Client) GetBestBlockHashAsync(ctx context.Context) *FutureGetBestBlockHashResult {
	cmd := chainjson.NewGetBestBlockHashCmd()
	return (*FutureGetBestBlockHashResult)(c.sendCmd(ctx, cmd))
}

// GetBestBlockHash returns the hash of the best block in the longest block
// chain.
func (c *Client) GetBestBlockHash(ctx context.Context) (*chainhash.Hash, error) {
	return c.GetBestBlockHashAsync(ctx).Receive()
}

// FutureGetBlockResult is a future promise to deliver the result of a
// GetBlockAsync RPC invocation (or an applicable error).
type FutureGetBlockResult cmdRes

// Receive waits for the response promised by the future and returns the raw
// block requested from the server given its hash.
func (r *FutureGetBlockResult) Receive() (*wire.MsgBlock, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var blockHex string
	err = json.Unmarshal(res, &blockHex)
	if err != nil {
		return nil, err
	}

	// Decode the serialized block hex to raw bytes.
	serializedBlock, err := hex.DecodeString(blockHex)
	if err != nil {
		return nil, err
	}

	// Deserialize the block and return it.
	var msgBlock wire.MsgBlock
	err = msgBlock.Deserialize(bytes.NewReader(serializedBlock))
	if err != nil {
		return nil, err
	}
	return &msgBlock, nil
}

// GetBlockAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetBlock for the blocking version and more details.
func (c *Client) GetBlockAsync(ctx context.Context, blockHash *chainhash.Hash) *FutureGetBlockResult {
	hash := ""
	if blockHash != nil {
		hash = blockHash.String()
	}

	cmd := chainjson.NewGetBlockCmd(hash, dcrjson.Bool(false), nil)
	return (*FutureGetBlockResult)(c.sendCmd(ctx, cmd))
}

// GetBlock returns a raw block from the server given its hash.
//
// See GetBlockVerbose to retrieve a data structure with information about the
// block instead.
func (c *Client) GetBlock(ctx context.Context, blockHash *chainhash.Hash) (*wire.MsgBlock, error) {
	return c.GetBlockAsync(ctx, blockHash).Receive()
}

// FutureGetBlockVerboseResult is a future promise to deliver the result of a
// GetBlockVerboseAsync RPC invocation (or an applicable error).
type FutureGetBlockVerboseResult cmdRes

// Receive waits for the response promised by the future and returns the data
// structure from the server with information about the requested block.
func (r *FutureGetBlockVerboseResult) Receive() (*chainjson.GetBlockVerboseResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal the raw result into a BlockResult.
	var blockResult chainjson.GetBlockVerboseResult
	err = json.Unmarshal(res, &blockResult)
	if err != nil {
		return nil, err
	}
	return &blockResult, nil
}

// GetBlockVerboseAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetBlockVerbose for the blocking version and more details.
func (c *Client) GetBlockVerboseAsync(ctx context.Context, blockHash *chainhash.Hash, verboseTx bool) *FutureGetBlockVerboseResult {
	hash := ""
	if blockHash != nil {
		hash = blockHash.String()
	}

	cmd := chainjson.NewGetBlockCmd(hash, dcrjson.Bool(true), &verboseTx)
	return (*FutureGetBlockVerboseResult)(c.sendCmd(ctx, cmd))
}

// GetBlockVerbose returns a data structure from the server with information
// about a block given its hash.
//
// See GetBlock to retrieve a raw block instead.
func (c *Client) GetBlockVerbose(ctx context.Context, blockHash *chainhash.Hash, verboseTx bool) (*chainjson.GetBlockVerboseResult, error) {
	return c.GetBlockVerboseAsync(ctx, blockHash, verboseTx).Receive()
}

// FutureGetBlockCountResult is a future promise to deliver the result of a
// GetBlockCountAsync RPC invocation (or an applicable error).
type FutureGetBlockCountResult cmdRes

// Receive waits for the response promised by the future and returns the number
// of blocks in the longest block chain.
func (r *FutureGetBlockCountResult) Receive() (int64, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return 0, err
	}

	// Unmarshal the result as an int64.
	var count int64
	err = json.Unmarshal(res, &count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// GetBlockCountAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetBlockCount for the blocking version and more details.
func (c *Client) GetBlockCountAsync(ctx context.Context) *FutureGetBlockCountResult {
	cmd := chainjson.NewGetBlockCountCmd()
	return (*FutureGetBlockCountResult)(c.sendCmd(ctx, cmd))
}

// GetBlockCount returns the number of blocks in the longest block chain.
func (c *Client) GetBlockCount(ctx context.Context) (int64, error) {
	return c.GetBlockCountAsync(ctx).Receive()
}

// FutureGetDifficultyResult is a future promise to deliver the result of a
// GetDifficultyAsync RPC invocation (or an applicable error).
type FutureGetDifficultyResult cmdRes

// Receive waits for the response promised by the future and returns the
// proof-of-work difficulty as a multiple of the minimum difficulty.
func (r *FutureGetDifficultyResult) Receive() (float64, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return 0, err
	}

	// Unmarshal the result as a float64.
	var difficulty float64
	err = json.Unmarshal(res, &difficulty)
	if err != nil {
		return 0, err
	}
	return difficulty, nil
}

// GetDifficultyAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetDifficulty for the blocking version and more details.
func (c *Client) GetDifficultyAsync(ctx context.Context) *FutureGetDifficultyResult {
	cmd := chainjson.NewGetDifficultyCmd()
	return (*FutureGetDifficultyResult)(c.sendCmd(ctx, cmd))
}

// GetDifficulty returns the proof-of-work difficulty as a multiple of the
// minimum difficulty.
func (c *Client) GetDifficulty(ctx context.Context) (float64, error) {
	return c.GetDifficultyAsync(ctx).Receive()
}

// FutureGetBlockChainInfoResult is a future promise to deliver the result of a
// GetBlockChainInfoAsync RPC invocation (or an applicable error).
type FutureGetBlockChainInfoResult cmdRes

// Receive waits for the response promised by the future and returns the info
// provided by the server.
func (r *FutureGetBlockChainInfoResult) Receive() (*chainjson.GetBlockChainInfoResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a getblockchaininfo result object.
	var blockchainInfoRes chainjson.GetBlockChainInfoResult
	err = json.Unmarshal(res, &blockchainInfoRes)
	if err != nil {
		return nil, err
	}

	return &blockchainInfoRes, nil
}

// GetBlockChainInfoAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetBlockChainInfo for the blocking version and more details.
func (c *Client) GetBlockChainInfoAsync(ctx context.Context) *FutureGetBlockChainInfoResult {
	cmd := chainjson.NewGetBlockChainInfoCmd()
	return (*FutureGetBlockChainInfoResult)(c.sendCmd(ctx, cmd))
}

// GetBlockChainInfo returns information about the current state of the block
// chain.
func (c *Client) GetBlockChainInfo(ctx context.Context) (*chainjson.GetBlockChainInfoResult, error) {
	return c.GetBlockChainInfoAsync(ctx).Receive()
}

// FutureGetInfoResult is a future promise to deliver the result of a
// GetInfoAsync RPC invocation (or an applicable error).
type FutureGetInfoResult cmdRes

// Receive waits for the response promised by the future and returns the info
// provided by the server.
func (r *FutureGetInfoResult) Receive() (*chainjson.InfoChainResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a getinfo result object.
	var infoRes chainjson.InfoChainResult
	err = json.Unmarshal(res, &infoRes)
	if err != nil {
		return nil, err
	}

	return &infoRes, nil
}

// GetInfoAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetInfo for the blocking version and more details.
func (c *Client) GetInfoAsync(ctx context.Context) *FutureGetInfoResult {
	cmd := chainjson.NewGetInfoCmd()
	return (*FutureGetInfoResult)(c.sendCmd(ctx, cmd))
}

// GetInfo returns information about the current state of the full node process.
func (c *Client) GetInfo(ctx context.Context) (*chainjson.InfoChainResult, error) {
	return c.GetInfoAsync(ctx).Receive()
}

// FutureGetBlockHashResult is a future promise to deliver the result of a
// GetBlockHashAsync RPC invocation (or an applicable error).
type FutureGetBlockHashResult cmdRes

// Receive waits for the response promised by the future and returns the hash of
// the block in the best block chain at the given height.
func (r *FutureGetBlockHashResult) Receive() (*chainhash.Hash, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal the result as a string-encoded sha.
	var txHashStr string
	err = json.Unmarshal(res, &txHashStr)
	if err != nil {
		return nil, err
	}
	return chainhash.NewHashFromStr(txHashStr)
}

// GetBlockHashAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetBlockHash for the blocking version and more details.
func (c *Client) GetBlockHashAsync(ctx context.Context, blockHeight int64) *FutureGetBlockHashResult {
	cmd := chainjson.NewGetBlockHashCmd(blockHeight)
	return (*FutureGetBlockHashResult)(c.sendCmd(ctx, cmd))
}

// GetBlockHash returns the hash of the block in the best block chain at the
// given height.
func (c *Client) GetBlockHash(ctx context.Context, blockHeight int64) (*chainhash.Hash, error) {
	return c.GetBlockHashAsync(ctx, blockHeight).Receive()
}

// FutureGetBlockHeaderResult is a future promise to deliver the result of a
// GetBlockHeaderAsync RPC invocation (or an applicable error).
type FutureGetBlockHeaderResult cmdRes

// Receive waits for the response promised by the future and returns the
// blockheader requested from the server given its hash.
func (r *FutureGetBlockHeaderResult) Receive() (*wire.BlockHeader, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal the result as a string.
	var bhHex string
	err = json.Unmarshal(res, &bhHex)
	if err != nil {
		return nil, err
	}

	serializedBH, err := hex.DecodeString(bhHex)
	if err != nil {
		return nil, err
	}

	// Deserialize the blockheader and return it.
	var bh wire.BlockHeader
	err = bh.Deserialize(bytes.NewReader(serializedBH))
	if err != nil {
		return nil, err
	}

	return &bh, nil
}

// GetBlockHeaderAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetBlockHeader for the blocking version and more details.
func (c *Client) GetBlockHeaderAsync(ctx context.Context, hash *chainhash.Hash) *FutureGetBlockHeaderResult {
	cmd := chainjson.NewGetBlockHeaderCmd(hash.String(), dcrjson.Bool(false))
	return (*FutureGetBlockHeaderResult)(c.sendCmd(ctx, cmd))
}

// GetBlockHeader returns the hash of the block in the best block chain at the
// given height.
func (c *Client) GetBlockHeader(ctx context.Context, hash *chainhash.Hash) (*wire.BlockHeader, error) {
	return c.GetBlockHeaderAsync(ctx, hash).Receive()
}

// FutureGetBlockHeaderVerboseResult is a future promise to deliver the result of a
// GetBlockHeaderAsync RPC invocation (or an applicable error).
type FutureGetBlockHeaderVerboseResult cmdRes

// Receive waits for the response promised by the future and returns a data
// structure of the block header requested from the server given its hash.
func (r *FutureGetBlockHeaderVerboseResult) Receive() (*chainjson.GetBlockHeaderVerboseResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal the result
	var bh chainjson.GetBlockHeaderVerboseResult
	err = json.Unmarshal(res, &bh)
	if err != nil {
		return nil, err
	}
	return &bh, nil
}

// GetBlockHeaderVerboseAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetBlockHeaderVerbose for the blocking version and more details.
func (c *Client) GetBlockHeaderVerboseAsync(ctx context.Context, hash *chainhash.Hash) *FutureGetBlockHeaderVerboseResult {
	cmd := chainjson.NewGetBlockHeaderCmd(hash.String(), dcrjson.Bool(true))
	return (*FutureGetBlockHeaderVerboseResult)(c.sendCmd(ctx, cmd))
}

// GetBlockHeaderVerbose returns a data structure of the block header from the
// server given its hash.
//
// See GetBlockHeader to retrieve a raw block header instead.
func (c *Client) GetBlockHeaderVerbose(ctx context.Context, hash *chainhash.Hash) (*chainjson.GetBlockHeaderVerboseResult, error) {
	return c.GetBlockHeaderVerboseAsync(ctx, hash).Receive()
}

// FutureGetBlockSubsidyResult is a future promise to deliver the result of a
// GetBlockSubsidyAsync RPC invocation (or an applicable error).
type FutureGetBlockSubsidyResult cmdRes

// Receive waits for the response promised by the future and returns a data
// structure of the block subsidy requested from the server given its height
// and number of voters.
func (r *FutureGetBlockSubsidyResult) Receive() (*chainjson.GetBlockSubsidyResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal the result
	var bs chainjson.GetBlockSubsidyResult
	err = json.Unmarshal(res, &bs)
	if err != nil {
		return nil, err
	}
	return &bs, nil
}

// GetBlockSubsidyAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetBlockSubsidy for the blocking version and more details.
func (c *Client) GetBlockSubsidyAsync(ctx context.Context, height int64, voters uint16) *FutureGetBlockSubsidyResult {
	cmd := chainjson.NewGetBlockSubsidyCmd(height, voters)
	return (*FutureGetBlockSubsidyResult)(c.sendCmd(ctx, cmd))
}

// GetBlockSubsidy returns a data structure of the block subsidy
// from the server given its height and number of voters.
func (c *Client) GetBlockSubsidy(ctx context.Context, height int64, voters uint16) (*chainjson.GetBlockSubsidyResult, error) {
	return c.GetBlockSubsidyAsync(ctx, height, voters).Receive()
}

// FutureGetCoinSupplyResult is a future promise to deliver the result of a
// GetCoinSupplyAsync RPC invocation (or an applicable error).
type FutureGetCoinSupplyResult cmdRes

// Receive waits for the response promised by the future and returns the
// current coin supply
func (r *FutureGetCoinSupplyResult) Receive() (dcrutil.Amount, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return 0, err
	}

	// Unmarshal the result
	var cs int64
	err = json.Unmarshal(res, &cs)
	if err != nil {
		return 0, err
	}
	return dcrutil.Amount(cs), nil
}

// GetCoinSupplyAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetCoinSupply for the blocking version and more details.
func (c *Client) GetCoinSupplyAsync(ctx context.Context) *FutureGetCoinSupplyResult {
	cmd := chainjson.NewGetCoinSupplyCmd()
	return (*FutureGetCoinSupplyResult)(c.sendCmd(ctx, cmd))
}

// GetCoinSupply returns the current coin supply
func (c *Client) GetCoinSupply(ctx context.Context) (dcrutil.Amount, error) {
	return c.GetCoinSupplyAsync(ctx).Receive()
}

// FutureGetRawMempoolResult is a future promise to deliver the result of a
// GetRawMempoolAsync RPC invocation (or an applicable error).
type FutureGetRawMempoolResult cmdRes

// Receive waits for the response promised by the future and returns the hashes
// of all transactions in the memory pool.
func (r *FutureGetRawMempoolResult) Receive() ([]*chainhash.Hash, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal the result as an array of strings.
	var txHashStrs []string
	err = json.Unmarshal(res, &txHashStrs)
	if err != nil {
		return nil, err
	}

	// Create a slice of ShaHash arrays from the string slice.
	txHashes := make([]*chainhash.Hash, 0, len(txHashStrs))
	for _, hashStr := range txHashStrs {
		txHash, err := chainhash.NewHashFromStr(hashStr)
		if err != nil {
			return nil, err
		}
		txHashes = append(txHashes, txHash)
	}

	return txHashes, nil
}

// GetRawMempoolAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetRawMempool for the blocking version and more details.
func (c *Client) GetRawMempoolAsync(ctx context.Context, txType chainjson.GetRawMempoolTxTypeCmd) *FutureGetRawMempoolResult {
	cmd := chainjson.NewGetRawMempoolCmd(dcrjson.Bool(false),
		dcrjson.String(string(txType)))
	return (*FutureGetRawMempoolResult)(c.sendCmd(ctx, cmd))
}

// GetRawMempool returns the hashes of all transactions in the memory pool for
// the given txType.
//
// See GetRawMempoolVerbose to retrieve data structures with information about
// the transactions instead.
func (c *Client) GetRawMempool(ctx context.Context, txType chainjson.GetRawMempoolTxTypeCmd) ([]*chainhash.Hash, error) {
	return c.GetRawMempoolAsync(ctx, txType).Receive()
}

// FutureGetRawMempoolVerboseResult is a future promise to deliver the result of
// a GetRawMempoolVerboseAsync RPC invocation (or an applicable error).
type FutureGetRawMempoolVerboseResult cmdRes

// Receive waits for the response promised by the future and returns a map of
// transaction hashes to an associated data structure with information about the
// transaction for all transactions in the memory pool.
func (r *FutureGetRawMempoolVerboseResult) Receive() (map[string]chainjson.GetRawMempoolVerboseResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal the result as a map of strings (tx shas) to their detailed
	// results.
	var mempoolItems map[string]chainjson.GetRawMempoolVerboseResult
	err = json.Unmarshal(res, &mempoolItems)
	if err != nil {
		return nil, err
	}
	return mempoolItems, nil
}

// GetRawMempoolVerboseAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetRawMempoolVerbose for the blocking version and more details.
func (c *Client) GetRawMempoolVerboseAsync(ctx context.Context, txType chainjson.GetRawMempoolTxTypeCmd) *FutureGetRawMempoolVerboseResult {
	cmd := chainjson.NewGetRawMempoolCmd(dcrjson.Bool(true),
		dcrjson.String(string(txType)))
	return (*FutureGetRawMempoolVerboseResult)(c.sendCmd(ctx, cmd))
}

// GetRawMempoolVerbose returns a map of transaction hashes to an associated
// data structure with information about the transaction for all transactions in
// the memory pool.
//
// See GetRawMempool to retrieve only the transaction hashes instead.
func (c *Client) GetRawMempoolVerbose(ctx context.Context, txType chainjson.GetRawMempoolTxTypeCmd) (map[string]chainjson.GetRawMempoolVerboseResult, error) {
	return c.GetRawMempoolVerboseAsync(ctx, txType).Receive()
}

// FutureValidateAddressResult is a future promise to deliver the result of a
// ValidateAddressAsync RPC invocation (or an applicable error).
type FutureValidateAddressResult cmdRes

// Receive waits for the response promised by the future and returns information
// about the given Decred address.
func (r *FutureValidateAddressResult) Receive() (*chainjson.ValidateAddressChainResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a validateaddress result object.
	var addrResult chainjson.ValidateAddressChainResult
	err = json.Unmarshal(res, &addrResult)
	if err != nil {
		return nil, err
	}

	return &addrResult, nil
}

// ValidateAddressAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See ValidateAddress for the blocking version and more details.
func (c *Client) ValidateAddressAsync(ctx context.Context, address dcrutil.Address) *FutureValidateAddressResult {
	addr := address.Address()
	cmd := chainjson.NewValidateAddressCmd(addr)
	return (*FutureValidateAddressResult)(c.sendCmd(ctx, cmd))
}

// ValidateAddress returns information about the given Decred address.
func (c *Client) ValidateAddress(ctx context.Context, address dcrutil.Address) (*chainjson.ValidateAddressChainResult, error) {
	return c.ValidateAddressAsync(ctx, address).Receive()
}

// FutureVerifyMessageResult is a future promise to deliver the result of a
// VerifyMessageAsync RPC invocation (or an applicable error).
type FutureVerifyMessageResult cmdRes

// Receive waits for the response promised by the future and returns whether or
// not the message was successfully verified.
func (r *FutureVerifyMessageResult) Receive() (bool, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return false, err
	}

	// Unmarshal result as a boolean.
	var verified bool
	err = json.Unmarshal(res, &verified)
	if err != nil {
		return false, err
	}

	return verified, nil
}

// VerifyMessageAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See VerifyMessage for the blocking version and more details.
func (c *Client) VerifyMessageAsync(ctx context.Context, address dcrutil.Address, signature, message string) *FutureVerifyMessageResult {
	addr := address.Address()
	cmd := chainjson.NewVerifyMessageCmd(addr, signature, message)
	return (*FutureVerifyMessageResult)(c.sendCmd(ctx, cmd))
}

// VerifyMessage verifies a signed message.
//
// NOTE: This function requires to the wallet to be unlocked.  See the
// WalletPassphrase function for more details.
func (c *Client) VerifyMessage(ctx context.Context, address dcrutil.Address, signature, message string) (bool, error) {
	return c.VerifyMessageAsync(ctx, address, signature, message).Receive()
}

// FutureVerifyChainResult is a future promise to deliver the result of a
// VerifyChainAsync, VerifyChainLevelAsyncRPC, or VerifyChainBlocksAsync
// invocation (or an applicable error).
type FutureVerifyChainResult cmdRes

// Receive waits for the response promised by the future and returns whether
// or not the chain verified based on the check level and number of blocks
// to verify specified in the original call.
func (r *FutureVerifyChainResult) Receive() (bool, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return false, err
	}

	// Unmarshal the result as a boolean.
	var verified bool
	err = json.Unmarshal(res, &verified)
	if err != nil {
		return false, err
	}
	return verified, nil
}

// FutureGetChainTipsResult is a future promise to deliver the result of a
// GetChainTipsAsync RPC invocation (or an applicable error).
type FutureGetChainTipsResult cmdRes

// Receive waits for the response promised by the future and returns slice of
// all known tips in the block tree.
func (r *FutureGetChainTipsResult) Receive() ([]chainjson.GetChainTipsResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal the result.
	var chainTips []chainjson.GetChainTipsResult
	err = json.Unmarshal(res, &chainTips)
	if err != nil {
		return nil, err
	}
	return chainTips, nil
}

// GetChainTipsAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetChainTips for the blocking version and more details.
func (c *Client) GetChainTipsAsync(ctx context.Context) *FutureGetChainTipsResult {
	cmd := chainjson.NewGetChainTipsCmd()
	return (*FutureGetChainTipsResult)(c.sendCmd(ctx, cmd))
}

// GetChainTips returns all known tips in the block tree.
func (c *Client) GetChainTips(ctx context.Context) ([]chainjson.GetChainTipsResult, error) {
	return c.GetChainTipsAsync(ctx).Receive()
}

// VerifyChainAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See VerifyChain for the blocking version and more details.
func (c *Client) VerifyChainAsync(ctx context.Context) *FutureVerifyChainResult {
	cmd := chainjson.NewVerifyChainCmd(nil, nil)
	return (*FutureVerifyChainResult)(c.sendCmd(ctx, cmd))
}

// VerifyChain requests the server to verify the block chain database using
// the default check level and number of blocks to verify.
//
// See VerifyChainLevel and VerifyChainBlocks to override the defaults.
func (c *Client) VerifyChain(ctx context.Context) (bool, error) {
	return c.VerifyChainAsync(ctx).Receive()
}

// VerifyChainLevelAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See VerifyChainLevel for the blocking version and more details.
func (c *Client) VerifyChainLevelAsync(ctx context.Context, checkLevel int64) *FutureVerifyChainResult {
	cmd := chainjson.NewVerifyChainCmd(&checkLevel, nil)
	return (*FutureVerifyChainResult)(c.sendCmd(ctx, cmd))
}

// VerifyChainLevel requests the server to verify the block chain database using
// the passed check level and default number of blocks to verify.
//
// The check level controls how thorough the verification is with higher numbers
// increasing the amount of checks done as consequently how long the
// verification takes.
//
// See VerifyChain to use the default check level and VerifyChainBlocks to
// override the number of blocks to verify.
func (c *Client) VerifyChainLevel(ctx context.Context, checkLevel int64) (bool, error) {
	return c.VerifyChainLevelAsync(ctx, checkLevel).Receive()
}

// VerifyChainBlocksAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See VerifyChainBlocks for the blocking version and more details.
func (c *Client) VerifyChainBlocksAsync(ctx context.Context, checkLevel, numBlocks int64) *FutureVerifyChainResult {
	cmd := chainjson.NewVerifyChainCmd(&checkLevel, &numBlocks)
	return (*FutureVerifyChainResult)(c.sendCmd(ctx, cmd))
}

// VerifyChainBlocks requests the server to verify the block chain database
// using the passed check level and number of blocks to verify.
//
// The check level controls how thorough the verification is with higher numbers
// increasing the amount of checks done as consequently how long the
// verification takes.
//
// The number of blocks refers to the number of blocks from the end of the
// current longest chain.
//
// See VerifyChain and VerifyChainLevel to use defaults.
func (c *Client) VerifyChainBlocks(ctx context.Context, checkLevel, numBlocks int64) (bool, error) {
	return c.VerifyChainBlocksAsync(ctx, checkLevel, numBlocks).Receive()
}

// FutureGetTxOutResult is a future promise to deliver the result of a
// GetTxOutAsync RPC invocation (or an applicable error).
type FutureGetTxOutResult cmdRes

// Receive waits for the response promised by the future and returns a
// transaction given its hash.
func (r *FutureGetTxOutResult) Receive() (*chainjson.GetTxOutResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// take care of the special case where the output has been spent already
	// it should return the string "null"
	if string(res) == "null" {
		return nil, nil
	}

	// Unmarshal result as a gettxout result object.
	var txOutInfo *chainjson.GetTxOutResult
	err = json.Unmarshal(res, &txOutInfo)
	if err != nil {
		return nil, err
	}

	return txOutInfo, nil
}

// GetTxOutAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetTxOut for the blocking version and more details.
func (c *Client) GetTxOutAsync(ctx context.Context, txHash *chainhash.Hash, index uint32, mempool bool) *FutureGetTxOutResult {
	hash := ""
	if txHash != nil {
		hash = txHash.String()
	}

	cmd := chainjson.NewGetTxOutCmd(hash, index, &mempool)
	return (*FutureGetTxOutResult)(c.sendCmd(ctx, cmd))
}

// GetTxOut returns the transaction output info if it's unspent and
// nil, otherwise.
func (c *Client) GetTxOut(ctx context.Context, txHash *chainhash.Hash, index uint32, mempool bool) (*chainjson.GetTxOutResult, error) {
	return c.GetTxOutAsync(ctx, txHash, index, mempool).Receive()
}

// FutureRescanResult is a future promise to deliver the result of a
// RescanAsynnc RPC invocation (or an applicable error).
type FutureRescanResult cmdRes

// Receive waits for the response promised by the future and returns the
// discovered rescan data.
func (r *FutureRescanResult) Receive() (*chainjson.RescanResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	var rescanResult *chainjson.RescanResult
	err = json.Unmarshal(res, &rescanResult)
	if err != nil {
		return nil, err
	}

	return rescanResult, nil
}

// RescanAsync returns an instance of a type that can be used to get the result
// of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See Rescan for the blocking version and more details.
func (c *Client) RescanAsync(ctx context.Context, blockHashes []chainhash.Hash) *FutureRescanResult {
	hashes := make([]string, len(blockHashes))
	for i := range blockHashes {
		hashes[i] = blockHashes[i].String()
	}

	cmd := chainjson.NewRescanCmd(hashes)
	return (*FutureRescanResult)(c.sendCmd(ctx, cmd))
}

// Rescan rescans the blocks identified by blockHashes, in order, using the
// client's loaded transaction filter.  The blocks do not need to be on the main
// chain, but they do need to be adjacent to each other.
func (c *Client) Rescan(ctx context.Context, blockHashes []chainhash.Hash) (*chainjson.RescanResult, error) {
	return c.RescanAsync(ctx, blockHashes).Receive()
}

// FutureGetCFilterResult is a future promise to deliver the result of a
// GetCFilterAsync RPC invocation (or an applicable error).
type FutureGetCFilterResult cmdRes

// Receive waits for the response promised by the future and returns the
// discovered rescan data.
func (r *FutureGetCFilterResult) Receive() (*gcs.FilterV1, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	var filterHex string
	err = json.Unmarshal(res, &filterHex)
	if err != nil {
		return nil, err
	}
	filterBytes, err := hex.DecodeString(filterHex)
	if err != nil {
		return nil, err
	}

	return gcs.FromBytesV1(blockcf.P, filterBytes)
}

// GetCFilterAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetCFilter for the blocking version and more details.
func (c *Client) GetCFilterAsync(ctx context.Context, blockHash *chainhash.Hash, filterType wire.FilterType) *FutureGetCFilterResult {
	var ft string
	switch filterType {
	case wire.GCSFilterRegular:
		ft = "regular"
	case wire.GCSFilterExtended:
		ft = "extended"
	default:
		return (*FutureGetCFilterResult)(newFutureError(ctx, errors.New("unknown filter type")))
	}

	cmd := chainjson.NewGetCFilterCmd(blockHash.String(), ft)
	return (*FutureGetCFilterResult)(c.sendCmd(ctx, cmd))
}

// GetCFilter returns the committed filter of type filterType for a block.
func (c *Client) GetCFilter(ctx context.Context, blockHash *chainhash.Hash, filterType wire.FilterType) (*gcs.FilterV1, error) {
	return c.GetCFilterAsync(ctx, blockHash, filterType).Receive()
}

// FutureGetCFilterHeaderResult is a future promise to deliver the result of a
// GetCFilterHeaderAsync RPC invocation (or an applicable error).
type FutureGetCFilterHeaderResult cmdRes

// Receive waits for the response promised by the future and returns the
// discovered rescan data.
func (r *FutureGetCFilterHeaderResult) Receive() (*chainhash.Hash, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	var filterHeaderHex string
	err = json.Unmarshal(res, &filterHeaderHex)
	if err != nil {
		return nil, err
	}

	return chainhash.NewHashFromStr(filterHeaderHex)
}

// GetCFilterHeaderAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetCFilterHeader for the blocking version and more details.
func (c *Client) GetCFilterHeaderAsync(ctx context.Context, blockHash *chainhash.Hash, filterType wire.FilterType) *FutureGetCFilterHeaderResult {
	var ft string
	switch filterType {
	case wire.GCSFilterRegular:
		ft = "regular"
	case wire.GCSFilterExtended:
		ft = "extended"
	default:
		return (*FutureGetCFilterHeaderResult)(newFutureError(ctx, errors.New("unknown filter type")))
	}

	cmd := chainjson.NewGetCFilterHeaderCmd(blockHash.String(), ft)
	return (*FutureGetCFilterHeaderResult)(c.sendCmd(ctx, cmd))
}

// GetCFilterHeader returns the committed filter header hash of type filterType
// for a block.
func (c *Client) GetCFilterHeader(ctx context.Context, blockHash *chainhash.Hash, filterType wire.FilterType) (*chainhash.Hash, error) {
	return c.GetCFilterHeaderAsync(ctx, blockHash, filterType).Receive()
}

// CFilterV2Result is the result of calling the GetCFilterV2 and
// GetCFilterV2Async methods.
type CFilterV2Result struct {
	BlockHash   chainhash.Hash
	Filter      *gcs.FilterV2
	ProofIndex  uint32
	ProofHashes []chainhash.Hash
}

// FutureGetCFilterV2Result is a future promise to deliver the result of a
// GetCFilterV2Async RPC invocation (or an applicable error).
type FutureGetCFilterV2Result cmdRes

// Receive waits for the response promised by the future and returns the
// discovered rescan data.
func (r *FutureGetCFilterV2Result) Receive() (*CFilterV2Result, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	var filterResult chainjson.GetCFilterV2Result
	err = json.Unmarshal(res, &filterResult)
	if err != nil {
		return nil, err
	}

	blockHash, err := chainhash.NewHashFromStr(filterResult.BlockHash)
	if err != nil {
		return nil, err
	}

	filterBytes, err := hex.DecodeString(filterResult.Data)
	if err != nil {
		return nil, err
	}
	filter, err := gcs.FromBytesV2(blockcf2.B, blockcf2.M, filterBytes)
	if err != nil {
		return nil, err
	}

	proofHashes := make([]chainhash.Hash, 0, len(filterResult.ProofHashes))
	for _, proofHashStr := range filterResult.ProofHashes {
		proofHash, err := chainhash.NewHashFromStr(proofHashStr)
		if err != nil {
			return nil, err
		}
		proofHashes = append(proofHashes, *proofHash)
	}

	return &CFilterV2Result{
		BlockHash:   *blockHash,
		Filter:      filter,
		ProofIndex:  filterResult.ProofIndex,
		ProofHashes: proofHashes,
	}, nil
}

// GetCFilterV2Async returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetCFilterV2 for the blocking version and more details.
func (c *Client) GetCFilterV2Async(ctx context.Context, blockHash *chainhash.Hash) *FutureGetCFilterV2Result {
	cmd := chainjson.NewGetCFilterV2Cmd(blockHash.String())
	return (*FutureGetCFilterV2Result)(c.sendCmd(ctx, cmd))
}

// GetCFilterV2 returns the version 2 block filter for the given block along
// with a proof that can be used to prove the filter is committed to by the
// block header.
func (c *Client) GetCFilterV2(ctx context.Context, blockHash *chainhash.Hash) (*CFilterV2Result, error) {
	return c.GetCFilterV2Async(ctx, blockHash).Receive()
}

// FutureEstimateSmartFeeResult is a future promise to deliver the result of a
// EstimateSmartFee RPC invocation (or an applicable error).
type FutureEstimateSmartFeeResult cmdRes

// Receive waits for the response promised by the future and returns a fee
// estimation for the given target confirmation window and mode.
func (r *FutureEstimateSmartFeeResult) Receive() (float64, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return 0, err
	}

	// Unmarshal the result as a float64.
	var dcrPerKB float64
	err = json.Unmarshal(res, &dcrPerKB)
	if err != nil {
		return 0, err
	}
	return dcrPerKB, nil
}

// EstimateSmartFeeAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See EstimateSmartFee for the blocking version and more details.
func (c *Client) EstimateSmartFeeAsync(ctx context.Context, confirmations int64, mode chainjson.EstimateSmartFeeMode) *FutureEstimateSmartFeeResult {
	cmd := chainjson.NewEstimateSmartFeeCmd(confirmations, &mode)
	return (*FutureEstimateSmartFeeResult)(c.sendCmd(ctx, cmd))
}

// EstimateSmartFee returns an estimation of a transaction fee rate (in dcr/KB)
// that new transactions should pay if they desire to be mined in up to
// 'confirmations' blocks.
//
// The mode parameter (roughly) selects the different thresholds for accepting
// an estimation as reasonable, allowing users to select different trade-offs
// between probability of the transaction being mined in the given target
// confirmation range and minimization of fees paid.
//
// As of 2019-01, only the default conservative mode is supported by dcrd.
func (c *Client) EstimateSmartFee(ctx context.Context, confirmations int64, mode chainjson.EstimateSmartFeeMode) (float64, error) {
	return c.EstimateSmartFeeAsync(ctx, confirmations, mode).Receive()
}
