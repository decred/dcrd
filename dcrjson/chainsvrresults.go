// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

import "github.com/decred/dcrd/rpc/jsonrpc/types"

// TxRawDecodeResult models the data from the decoderawtransaction command.
type TxRawDecodeResult = types.TxRawDecodeResult

// DecodeScriptResult models the data returned from the decodescript command.
type DecodeScriptResult = types.DecodeScriptResult

// EstimateSmartFeeResult models the data returned from the estimatesmartfee
// command.
type EstimateSmartFeeResult = types.EstimateSmartFeeResult

// EstimateStakeDiffResult models the data returned from the estimatestakediff
// command.
type EstimateStakeDiffResult = types.EstimateStakeDiffResult

// GetAddedNodeInfoResultAddr models the data of the addresses portion of the
// getaddednodeinfo command.
type GetAddedNodeInfoResultAddr = types.GetAddedNodeInfoResultAddr

// GetAddedNodeInfoResult models the data from the getaddednodeinfo command.
type GetAddedNodeInfoResult = types.GetAddedNodeInfoResult

// GetBlockVerboseResult models the data from the getblock command when the
// verbose flag is set.  When the verbose flag is not set, getblock returns a
// hex-encoded string.  Contains Decred additions.
type GetBlockVerboseResult = types.GetBlockVerboseResult

// AgendaInfo provides an overview of an agenda in a consensus deployment.
type AgendaInfo = types.AgendaInfo

// GetBestBlockResult models the data from the getbestblock command.
type GetBestBlockResult = types.GetBestBlockResult

// GetBlockChainInfoResult models the data returned from the getblockchaininfo
// command.
type GetBlockChainInfoResult = types.GetBlockChainInfoResult

// GetBlockHeaderVerboseResult models the data from the getblockheader command when
// the verbose flag is set.  When the verbose flag is not set, getblockheader
// returns a hex-encoded string.
type GetBlockHeaderVerboseResult = types.GetBlockHeaderVerboseResult

// GetBlockSubsidyResult models the data returned from the getblocksubsidy
// command.
type GetBlockSubsidyResult = types.GetBlockSubsidyResult

// GetBlockTemplateResultTx models the transactions field of the
// getblocktemplate command.
type GetBlockTemplateResultTx = types.GetBlockTemplateResultTx

// GetBlockTemplateResultAux models the coinbaseaux field of the
// getblocktemplate command.
type GetBlockTemplateResultAux = types.GetBlockTemplateResultAux

// GetBlockTemplateResult models the data returned from the getblocktemplate
// command.
type GetBlockTemplateResult = types.GetBlockTemplateResult

// GetChainTipsResult models the data returns from the getchaintips command.
type GetChainTipsResult = types.GetChainTipsResult

// GetHeadersResult models the data returned by the chain server getheaders
// command.
type GetHeadersResult = types.GetHeadersResult

// InfoChainResult models the data returned by the chain server getinfo command.
type InfoChainResult = types.InfoChainResult

// GetMempoolInfoResult models the data returned from the getmempoolinfo
// command.
type GetMempoolInfoResult = types.GetMempoolInfoResult

// GetMiningInfoResult models the data from the getmininginfo command.
// Contains Decred additions.
type GetMiningInfoResult = types.GetMiningInfoResult

// LocalAddressesResult models the localaddresses data from the getnetworkinfo
// command.
type LocalAddressesResult = types.LocalAddressesResult

// NetworksResult models the networks data from the getnetworkinfo command.
type NetworksResult = types.NetworksResult

// GetNetworkInfoResult models the data returned from the getnetworkinfo
// command.
type GetNetworkInfoResult = types.GetNetworkInfoResult

// GetNetTotalsResult models the data returned from the getnettotals command.
type GetNetTotalsResult = types.GetNetTotalsResult

// GetPeerInfoResult models the data returned from the getpeerinfo command.
type GetPeerInfoResult = types.GetPeerInfoResult

// GetRawMempoolVerboseResult models the data returned from the getrawmempool
// command when the verbose flag is set.  When the verbose flag is not set,
// getrawmempool returns an array of transaction hashes.
type GetRawMempoolVerboseResult = types.GetRawMempoolVerboseResult

// TxRawResult models the data from the getrawtransaction command.
type TxRawResult = types.TxRawResult

// GetStakeDifficultyResult models the data returned from the
// getstakedifficulty command.
type GetStakeDifficultyResult = types.GetStakeDifficultyResult

// VersionCount models a generic version:count tuple.
type VersionCount = types.VersionCount

// VersionInterval models a cooked version count for an interval.
type VersionInterval = types.VersionInterval

// GetStakeVersionInfoResult models the resulting data for getstakeversioninfo
// command.
type GetStakeVersionInfoResult = types.GetStakeVersionInfoResult

// VersionBits models a generic version:bits tuple.
type VersionBits = types.VersionBits

// StakeVersions models the data for GetStakeVersionsResult.
type StakeVersions = types.StakeVersions

// GetStakeVersionsResult models the data returned from the getstakeversions
// command.
type GetStakeVersionsResult = types.GetStakeVersionsResult

// GetTxOutResult models the data from the gettxout command.
type GetTxOutResult = types.GetTxOutResult

// Choice models an individual choice inside an Agenda.
type Choice = types.Choice

// Agenda models an individual agenda including its choices.
type Agenda = types.Agenda

// GetVoteInfoResult models the data returned from the getvoteinfo command.
type GetVoteInfoResult = types.GetVoteInfoResult

// GetWorkResult models the data from the getwork command.
type GetWorkResult = types.GetWorkResult

// Ticket is the structure representing a ticket.
type Ticket = types.Ticket

// LiveTicketsResult models the data returned from the livetickets
// command.
type LiveTicketsResult = types.LiveTicketsResult

// MissedTicketsResult models the data returned from the missedtickets
// command.
type MissedTicketsResult = types.MissedTicketsResult

// FeeInfoBlock is ticket fee information about a block.
type FeeInfoBlock = types.FeeInfoBlock

// FeeInfoMempool is ticket fee information about the mempool.
type FeeInfoMempool = types.FeeInfoMempool

// FeeInfoRange is ticket fee information about a range.
type FeeInfoRange = types.FeeInfoRange

// FeeInfoWindow is ticket fee information about an adjustment window.
type FeeInfoWindow = types.FeeInfoWindow

// TicketFeeInfoResult models the data returned from the ticketfeeinfo command.
// command.
type TicketFeeInfoResult = types.TicketFeeInfoResult

// SearchRawTransactionsResult models the data from the searchrawtransaction
// command.
type SearchRawTransactionsResult = types.SearchRawTransactionsResult

// TxFeeInfoResult models the data returned from the ticketfeeinfo command.
// command.
type TxFeeInfoResult = types.TxFeeInfoResult

// TicketsForAddressResult models the data returned from the ticketforaddress
// command.
type TicketsForAddressResult = types.TicketsForAddressResult

// ValidateAddressChainResult models the data returned by the chain server
// validateaddress command.
type ValidateAddressChainResult = types.ValidateAddressChainResult

// VersionResult models objects included in the version response.  In the actual
// result, these objects are keyed by the program or API name.
type VersionResult = types.VersionResult

// ScriptPubKeyResult models the scriptPubKey data of a tx script.  It is
// defined separately since it is used by multiple commands.
type ScriptPubKeyResult = types.ScriptPubKeyResult

// ScriptSig models a signature script.  It is defined separately since it only
// applies to non-coinbase.  Therefore the field in the Vin structure needs
// to be a pointer.
type ScriptSig = types.ScriptSig

// Vin models parts of the tx data.  It is defined separately since
// getrawtransaction, decoderawtransaction, and searchrawtransaction use the
// same structure.
type Vin = types.Vin

// PrevOut represents previous output for an input Vin.
type PrevOut = types.PrevOut

// VinPrevOut is like Vin except it includes PrevOut.  It is used by searchrawtransaction
type VinPrevOut = types.VinPrevOut

// Vout models parts of the tx data.  It is defined separately since both
// getrawtransaction and decoderawtransaction use the same structure.
type Vout = types.Vout
