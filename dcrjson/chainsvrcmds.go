// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a chain server.

package dcrjson

import "github.com/decred/dcrd/rpc/jsonrpc/types"

// AddNodeSubCmd defines the type used in the addnode JSON-RPC command for the
// sub command field.
type AddNodeSubCmd = types.AddNodeSubCmd

const (
	// ANAdd indicates the specified host should be added as a persistent
	// peer.
	ANAdd = types.ANAdd

	// ANRemove indicates the specified peer should be removed.
	ANRemove = types.ANRemove

	// ANOneTry indicates the specified host should try to connect once,
	// but it should not be made persistent.
	ANOneTry = types.ANOneTry
)

// NodeSubCmd defines the type used in the addnode JSON-RPC command for the
// sub command field.
type NodeSubCmd = types.NodeSubCmd

const (
	// NConnect indicates the specified host that should be connected to.
	NConnect = types.NConnect

	// NRemove indicates the specified peer that should be removed as a
	// persistent peer.
	NRemove = types.NRemove

	// NDisconnect indicates the specified peer should be disonnected.
	NDisconnect = types.NDisconnect
)

// AddNodeCmd defines the addnode JSON-RPC command.
type AddNodeCmd = types.AddNodeCmd

// NewAddNodeCmd returns a new instance which can be used to issue an addnode
// JSON-RPC command.
func NewAddNodeCmd(addr string, subCmd AddNodeSubCmd) *AddNodeCmd {
	return types.NewAddNodeCmd(addr, subCmd)
}

// SStxInput represents the inputs to an SStx transaction. Specifically a
// transactionsha and output number pair, along with the output amounts.
type SStxInput = types.SStxInput

// SStxCommitOut represents the output to an SStx transaction. Specifically a
// a commitment address and amount, and a change address and amount.
type SStxCommitOut = types.SStxCommitOut

// CreateRawSStxCmd is a type handling custom marshaling and
// unmarshaling of createrawsstx JSON RPC commands.
type CreateRawSStxCmd = types.CreateRawSStxCmd

// NewCreateRawSStxCmd creates a new CreateRawSStxCmd.
func NewCreateRawSStxCmd(inputs []SStxInput, amount map[string]int64,
	couts []SStxCommitOut) *CreateRawSStxCmd {
	return types.NewCreateRawSStxCmd(inputs, amount, couts)
}

// CreateRawSSRtxCmd is a type handling custom marshaling and
// unmarshaling of createrawssrtx JSON RPC commands.
type CreateRawSSRtxCmd = types.CreateRawSSRtxCmd

// NewCreateRawSSRtxCmd creates a new CreateRawSSRtxCmd.
func NewCreateRawSSRtxCmd(inputs []TransactionInput, fee *float64) *CreateRawSSRtxCmd {
	return types.NewCreateRawSSRtxCmd(inputs, fee)
}

// TransactionInput represents the inputs to a transaction.  Specifically a
// transaction hash and output number pair. Contains Decred additions.
type TransactionInput = types.TransactionInput

// CreateRawTransactionCmd defines the createrawtransaction JSON-RPC command.
type CreateRawTransactionCmd = types.CreateRawTransactionCmd

// NewCreateRawTransactionCmd returns a new instance which can be used to issue
// a createrawtransaction JSON-RPC command.
//
// Amounts are in DCR.
func NewCreateRawTransactionCmd(inputs []TransactionInput, amounts map[string]float64,
	lockTime *int64, expiry *int64) *CreateRawTransactionCmd {
	return types.NewCreateRawTransactionCmd(inputs, amounts, lockTime, expiry)
}

// DebugLevelCmd defines the debuglevel JSON-RPC command.  This command is not a
// standard Bitcoin command.  It is an extension for btcd.
type DebugLevelCmd = types.DebugLevelCmd

// NewDebugLevelCmd returns a new DebugLevelCmd which can be used to issue a
// debuglevel JSON-RPC command.  This command is not a standard Bitcoin command.
// It is an extension for btcd.
func NewDebugLevelCmd(levelSpec string) *DebugLevelCmd {
	return types.NewDebugLevelCmd(levelSpec)
}

// DecodeRawTransactionCmd defines the decoderawtransaction JSON-RPC command.
type DecodeRawTransactionCmd = types.DecodeRawTransactionCmd

// NewDecodeRawTransactionCmd returns a new instance which can be used to issue
// a decoderawtransaction JSON-RPC command.
func NewDecodeRawTransactionCmd(hexTx string) *DecodeRawTransactionCmd {
	return types.NewDecodeRawTransactionCmd(hexTx)
}

// DecodeScriptCmd defines the decodescript JSON-RPC command.
type DecodeScriptCmd = types.DecodeScriptCmd

// NewDecodeScriptCmd returns a new instance which can be used to issue a
// decodescript JSON-RPC command.
func NewDecodeScriptCmd(hexScript string) *DecodeScriptCmd {
	return types.NewDecodeScriptCmd(hexScript)
}

// EstimateFeeCmd defines the estimatefee JSON-RPC command.
type EstimateFeeCmd = types.EstimateFeeCmd

// NewEstimateFeeCmd returns a new instance which can be used to issue an
// estimatefee JSON-RPC command.
func NewEstimateFeeCmd(numBlocks int64) *EstimateFeeCmd {
	return types.NewEstimateFeeCmd(numBlocks)
}

// EstimateSmartFeeMode defines estimation mode to be used with
// the estimatesmartfee command.
type EstimateSmartFeeMode = types.EstimateSmartFeeMode

const (
	// EstimateSmartFeeEconomical returns an
	// economical result.
	EstimateSmartFeeEconomical = types.EstimateSmartFeeEconomical

	// EstimateSmartFeeConservative potentially returns
	// a conservative result.
	EstimateSmartFeeConservative = types.EstimateSmartFeeConservative
)

// EstimateSmartFeeCmd defines the estimatesmartfee JSON-RPC command.
type EstimateSmartFeeCmd = types.EstimateSmartFeeCmd

// NewEstimateSmartFeeCmd returns a new instance which can be used to issue an
// estimatesmartfee JSON-RPC command.
func NewEstimateSmartFeeCmd(confirmations int64, mode *EstimateSmartFeeMode) *EstimateSmartFeeCmd {
	return types.NewEstimateSmartFeeCmd(confirmations, mode)
}

// EstimateStakeDiffCmd defines the eststakedifficulty JSON-RPC command.
type EstimateStakeDiffCmd = types.EstimateStakeDiffCmd

// NewEstimateStakeDiffCmd defines the eststakedifficulty JSON-RPC command.
func NewEstimateStakeDiffCmd(tickets *uint32) *EstimateStakeDiffCmd {
	return types.NewEstimateStakeDiffCmd(tickets)
}

// ExistsAddressCmd defines the existsaddress JSON-RPC command.
type ExistsAddressCmd = types.ExistsAddressCmd

// NewExistsAddressCmd returns a new instance which can be used to issue a
// existsaddress JSON-RPC command.
func NewExistsAddressCmd(address string) *ExistsAddressCmd {
	return types.NewExistsAddressCmd(address)
}

// ExistsAddressesCmd defines the existsaddresses JSON-RPC command.
type ExistsAddressesCmd = types.ExistsAddressesCmd

// NewExistsAddressesCmd returns a new instance which can be used to issue an
// existsaddresses JSON-RPC command.
func NewExistsAddressesCmd(addresses []string) *ExistsAddressesCmd {
	return types.NewExistsAddressesCmd(addresses)
}

// ExistsMissedTicketsCmd defines the existsmissedtickets JSON-RPC command.
type ExistsMissedTicketsCmd struct {
	TxHashBlob string
}

// NewExistsMissedTicketsCmd returns a new instance which can be used to issue an
// existsmissedtickets JSON-RPC command.
func NewExistsMissedTicketsCmd(txHashBlob string) *ExistsMissedTicketsCmd {
	return &ExistsMissedTicketsCmd{
		TxHashBlob: txHashBlob,
	}
}

// ExistsExpiredTicketsCmd defines the existsexpiredtickets JSON-RPC command.
type ExistsExpiredTicketsCmd struct {
	TxHashBlob string
}

// NewExistsExpiredTicketsCmd returns a new instance which can be used to issue an
// existsexpiredtickets JSON-RPC command.
func NewExistsExpiredTicketsCmd(txHashBlob string) *ExistsExpiredTicketsCmd {
	return &ExistsExpiredTicketsCmd{
		TxHashBlob: txHashBlob,
	}
}

// ExistsLiveTicketCmd defines the existsliveticket JSON-RPC command.
type ExistsLiveTicketCmd = types.ExistsLiveTicketCmd

// NewExistsLiveTicketCmd returns a new instance which can be used to issue an
// existsliveticket JSON-RPC command.
func NewExistsLiveTicketCmd(txHash string) *ExistsLiveTicketCmd {
	return types.NewExistsLiveTicketCmd(txHash)
}

// ExistsLiveTicketsCmd defines the existslivetickets JSON-RPC command.
type ExistsLiveTicketsCmd struct {
	TxHashBlob string
}

// NewExistsLiveTicketsCmd returns a new instance which can be used to issue an
// existslivetickets JSON-RPC command.
func NewExistsLiveTicketsCmd(txHashBlob string) *ExistsLiveTicketsCmd {
	return &ExistsLiveTicketsCmd{
		TxHashBlob: txHashBlob,
	}
}

// ExistsMempoolTxsCmd defines the existsmempooltxs JSON-RPC command.
type ExistsMempoolTxsCmd struct {
	TxHashBlob string
}

// NewExistsMempoolTxsCmd returns a new instance which can be used to issue an
// existslivetickets JSON-RPC command.
func NewExistsMempoolTxsCmd(txHashBlob string) *ExistsMempoolTxsCmd {
	return &ExistsMempoolTxsCmd{
		TxHashBlob: txHashBlob,
	}
}

// GenerateCmd defines the generate JSON-RPC command.
type GenerateCmd = types.GenerateCmd

// NewGenerateCmd returns a new instance which can be used to issue a generate
// JSON-RPC command.
func NewGenerateCmd(numBlocks uint32) *GenerateCmd {
	return types.NewGenerateCmd(numBlocks)
}

// GetAddedNodeInfoCmd defines the getaddednodeinfo JSON-RPC command.
type GetAddedNodeInfoCmd = types.GetAddedNodeInfoCmd

// NewGetAddedNodeInfoCmd returns a new instance which can be used to issue a
// getaddednodeinfo JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetAddedNodeInfoCmd(dns bool, node *string) *GetAddedNodeInfoCmd {
	return types.NewGetAddedNodeInfoCmd(dns, node)
}

// GetBestBlockCmd defines the getbestblock JSON-RPC command.
type GetBestBlockCmd = types.GetBestBlockCmd

// NewGetBestBlockCmd returns a new instance which can be used to issue a
// getbestblock JSON-RPC command.
func NewGetBestBlockCmd() *GetBestBlockCmd {
	return types.NewGetBestBlockCmd()
}

// GetBestBlockHashCmd defines the getbestblockhash JSON-RPC command.
type GetBestBlockHashCmd = types.GetBestBlockHashCmd

// NewGetBestBlockHashCmd returns a new instance which can be used to issue a
// getbestblockhash JSON-RPC command.
func NewGetBestBlockHashCmd() *GetBestBlockHashCmd {
	return types.NewGetBestBlockHashCmd()
}

// GetBlockCmd defines the getblock JSON-RPC command.
type GetBlockCmd = types.GetBlockCmd

// NewGetBlockCmd returns a new instance which can be used to issue a getblock
// JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetBlockCmd(hash string, verbose, verboseTx *bool) *GetBlockCmd {
	return types.NewGetBlockCmd(hash, verbose, verboseTx)
}

// GetBlockChainInfoCmd defines the getblockchaininfo JSON-RPC command.
type GetBlockChainInfoCmd = types.GetBlockChainInfoCmd

// NewGetBlockChainInfoCmd returns a new instance which can be used to issue a
// getblockchaininfo JSON-RPC command.
func NewGetBlockChainInfoCmd() *GetBlockChainInfoCmd {
	return types.NewGetBlockChainInfoCmd()
}

// GetBlockCountCmd defines the getblockcount JSON-RPC command.
type GetBlockCountCmd = types.GetBlockCountCmd

// NewGetBlockCountCmd returns a new instance which can be used to issue a
// getblockcount JSON-RPC command.
func NewGetBlockCountCmd() *GetBlockCountCmd {
	return types.NewGetBlockCountCmd()
}

// GetBlockHashCmd defines the getblockhash JSON-RPC command.
type GetBlockHashCmd = types.GetBlockHashCmd

// NewGetBlockHashCmd returns a new instance which can be used to issue a
// getblockhash JSON-RPC command.
func NewGetBlockHashCmd(index int64) *GetBlockHashCmd {
	return types.NewGetBlockHashCmd(index)
}

// GetBlockHeaderCmd defines the getblockheader JSON-RPC command.
type GetBlockHeaderCmd = types.GetBlockHeaderCmd

// NewGetBlockHeaderCmd returns a new instance which can be used to issue a
// getblockheader JSON-RPC command.
func NewGetBlockHeaderCmd(hash string, verbose *bool) *GetBlockHeaderCmd {
	return types.NewGetBlockHeaderCmd(hash, verbose)
}

// GetBlockSubsidyCmd defines the getblocksubsidy JSON-RPC command.
type GetBlockSubsidyCmd = types.GetBlockSubsidyCmd

// NewGetBlockSubsidyCmd returns a new instance which can be used to issue a
// getblocksubsidy JSON-RPC command.
func NewGetBlockSubsidyCmd(height int64, voters uint16) *GetBlockSubsidyCmd {
	return types.NewGetBlockSubsidyCmd(height, voters)
}

// TemplateRequest is a request object as defined in BIP22
// (https://en.bitcoin.it/wiki/BIP_0022), it is optionally provided as an
// pointer argument to GetBlockTemplateCmd.
type TemplateRequest = types.TemplateRequest

// GetBlockTemplateCmd defines the getblocktemplate JSON-RPC command.
type GetBlockTemplateCmd = types.GetBlockTemplateCmd

// NewGetBlockTemplateCmd returns a new instance which can be used to issue a
// getblocktemplate JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetBlockTemplateCmd(request *TemplateRequest) *GetBlockTemplateCmd {
	return types.NewGetBlockTemplateCmd(request)
}

// GetCFilterCmd defines the getcfilter JSON-RPC command.
type GetCFilterCmd = types.GetCFilterCmd

// NewGetCFilterCmd returns a new instance which can be used to issue a
// getcfilter JSON-RPC command.
func NewGetCFilterCmd(hash string, filterType string) *GetCFilterCmd {
	return types.NewGetCFilterCmd(hash, filterType)
}

// GetCFilterHeaderCmd defines the getcfilterheader JSON-RPC command.
type GetCFilterHeaderCmd = types.GetCFilterHeaderCmd

// NewGetCFilterHeaderCmd returns a new instance which can be used to issue a
// getcfilterheader JSON-RPC command.
func NewGetCFilterHeaderCmd(hash string, filterType string) *GetCFilterHeaderCmd {
	return types.NewGetCFilterHeaderCmd(hash, filterType)
}

// GetChainTipsCmd defines the getchaintips JSON-RPC command.
type GetChainTipsCmd = types.GetChainTipsCmd

// NewGetChainTipsCmd returns a new instance which can be used to issue a
// getchaintips JSON-RPC command.
func NewGetChainTipsCmd() *GetChainTipsCmd {
	return types.NewGetChainTipsCmd()
}

// GetCoinSupplyCmd defines the getcoinsupply JSON-RPC command.
type GetCoinSupplyCmd = types.GetCoinSupplyCmd

// NewGetCoinSupplyCmd returns a new instance which can be used to issue a
// getcoinsupply JSON-RPC command.
func NewGetCoinSupplyCmd() *GetCoinSupplyCmd {
	return types.NewGetCoinSupplyCmd()
}

// GetConnectionCountCmd defines the getconnectioncount JSON-RPC command.
type GetConnectionCountCmd = types.GetConnectionCountCmd

// NewGetConnectionCountCmd returns a new instance which can be used to issue a
// getconnectioncount JSON-RPC command.
func NewGetConnectionCountCmd() *GetConnectionCountCmd {
	return types.NewGetConnectionCountCmd()
}

// GetCurrentNetCmd defines the getcurrentnet JSON-RPC command.
type GetCurrentNetCmd = types.GetCurrentNetCmd

// NewGetCurrentNetCmd returns a new instance which can be used to issue a
// getcurrentnet JSON-RPC command.
func NewGetCurrentNetCmd() *GetCurrentNetCmd {
	return types.NewGetCurrentNetCmd()
}

// GetDifficultyCmd defines the getdifficulty JSON-RPC command.
type GetDifficultyCmd = types.GetDifficultyCmd

// NewGetDifficultyCmd returns a new instance which can be used to issue a
// getdifficulty JSON-RPC command.
func NewGetDifficultyCmd() *GetDifficultyCmd {
	return types.NewGetDifficultyCmd()
}

// GetGenerateCmd defines the getgenerate JSON-RPC command.
type GetGenerateCmd = types.GetGenerateCmd

// NewGetGenerateCmd returns a new instance which can be used to issue a
// getgenerate JSON-RPC command.
func NewGetGenerateCmd() *GetGenerateCmd {
	return types.NewGetGenerateCmd()
}

// GetHashesPerSecCmd defines the gethashespersec JSON-RPC command.
type GetHashesPerSecCmd = types.GetHashesPerSecCmd

// NewGetHashesPerSecCmd returns a new instance which can be used to issue a
// gethashespersec JSON-RPC command.
func NewGetHashesPerSecCmd() *GetHashesPerSecCmd {
	return types.NewGetHashesPerSecCmd()
}

// GetInfoCmd defines the getinfo JSON-RPC command.
type GetInfoCmd = types.GetInfoCmd

// NewGetInfoCmd returns a new instance which can be used to issue a
// getinfo JSON-RPC command.
func NewGetInfoCmd() *GetInfoCmd {
	return types.NewGetInfoCmd()
}

// GetHeadersCmd defines the getheaders JSON-RPC command.
type GetHeadersCmd struct {
	BlockLocators string `json:"blocklocators"`
	HashStop      string `json:"hashstop"`
}

// NewGetHeadersCmd returns a new instance which can be used to issue a
// getheaders JSON-RPC command.
func NewGetHeadersCmd(blockLocators string, hashStop string) *GetHeadersCmd {
	return &GetHeadersCmd{
		BlockLocators: blockLocators,
		HashStop:      hashStop,
	}
}

// GetMempoolInfoCmd defines the getmempoolinfo JSON-RPC command.
type GetMempoolInfoCmd = types.GetMempoolInfoCmd

// NewGetMempoolInfoCmd returns a new instance which can be used to issue a
// getmempool JSON-RPC command.
func NewGetMempoolInfoCmd() *GetMempoolInfoCmd {
	return types.NewGetMempoolInfoCmd()
}

// GetMiningInfoCmd defines the getmininginfo JSON-RPC command.
type GetMiningInfoCmd = types.GetMiningInfoCmd

// NewGetMiningInfoCmd returns a new instance which can be used to issue a
// getmininginfo JSON-RPC command.
func NewGetMiningInfoCmd() *GetMiningInfoCmd {
	return types.NewGetMiningInfoCmd()
}

// GetNetworkInfoCmd defines the getnetworkinfo JSON-RPC command.
type GetNetworkInfoCmd = types.GetNetworkInfoCmd

// NewGetNetworkInfoCmd returns a new instance which can be used to issue a
// getnetworkinfo JSON-RPC command.
func NewGetNetworkInfoCmd() *GetNetworkInfoCmd {
	return types.NewGetNetworkInfoCmd()
}

// GetNetTotalsCmd defines the getnettotals JSON-RPC command.
type GetNetTotalsCmd = types.GetNetTotalsCmd

// NewGetNetTotalsCmd returns a new instance which can be used to issue a
// getnettotals JSON-RPC command.
func NewGetNetTotalsCmd() *GetNetTotalsCmd {
	return types.NewGetNetTotalsCmd()
}

// GetNetworkHashPSCmd defines the getnetworkhashps JSON-RPC command.
type GetNetworkHashPSCmd = types.GetNetworkHashPSCmd

// NewGetNetworkHashPSCmd returns a new instance which can be used to issue a
// getnetworkhashps JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetNetworkHashPSCmd(numBlocks, height *int) *GetNetworkHashPSCmd {
	return types.NewGetNetworkHashPSCmd(numBlocks, height)
}

// GetPeerInfoCmd defines the getpeerinfo JSON-RPC command.
type GetPeerInfoCmd = types.GetPeerInfoCmd

// NewGetPeerInfoCmd returns a new instance which can be used to issue a getpeer
// JSON-RPC command.
func NewGetPeerInfoCmd() *GetPeerInfoCmd {
	return types.NewGetPeerInfoCmd()
}

// GetRawMempoolTxTypeCmd defines the type used in the getrawmempool JSON-RPC
// command for the TxType command field.
type GetRawMempoolTxTypeCmd = types.GetRawMempoolTxTypeCmd

const (
	// GRMAll indicates any type of transaction should be returned.
	GRMAll = types.GRMAll

	// GRMRegular indicates only regular transactions should be returned.
	GRMRegular = types.GRMRegular

	// GRMTickets indicates that only tickets should be returned.
	GRMTickets = types.GRMTickets

	// GRMVotes indicates that only votes should be returned.
	GRMVotes = types.GRMVotes

	// GRMRevocations indicates that only revocations should be returned.
	GRMRevocations = types.GRMRevocations
)

// GetRawMempoolCmd defines the getmempool JSON-RPC command.
type GetRawMempoolCmd = types.GetRawMempoolCmd

// NewGetRawMempoolCmd returns a new instance which can be used to issue a
// getrawmempool JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetRawMempoolCmd(verbose *bool, txType *string) *GetRawMempoolCmd {
	return types.NewGetRawMempoolCmd(verbose, txType)
}

// GetRawTransactionCmd defines the getrawtransaction JSON-RPC command.
//
// NOTE: This field is an int versus a bool to remain compatible with Bitcoin
// Core even though it really should be a bool.
type GetRawTransactionCmd = types.GetRawTransactionCmd

// NewGetRawTransactionCmd returns a new instance which can be used to issue a
// getrawtransaction JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetRawTransactionCmd(txHash string, verbose *int) *GetRawTransactionCmd {
	return types.NewGetRawTransactionCmd(txHash, verbose)
}

// GetStakeDifficultyCmd is a type handling custom marshaling and
// unmarshaling of getstakedifficulty JSON RPC commands.
type GetStakeDifficultyCmd = types.GetStakeDifficultyCmd

// NewGetStakeDifficultyCmd returns a new instance which can be used to
// issue a JSON-RPC getstakedifficulty command.
func NewGetStakeDifficultyCmd() *GetStakeDifficultyCmd {
	return types.NewGetStakeDifficultyCmd()
}

// GetStakeVersionInfoCmd returns stake version info for the current interval.
// Optionally, Count indicates how many additional intervals to return.
type GetStakeVersionInfoCmd = types.GetStakeVersionInfoCmd

// NewGetStakeVersionInfoCmd returns a new instance which can be used to
// issue a JSON-RPC getstakeversioninfo command.
func NewGetStakeVersionInfoCmd(count int32) *GetStakeVersionInfoCmd {
	return types.NewGetStakeVersionInfoCmd(count)
}

// GetStakeVersionsCmd returns stake version for a range of blocks.
// Count indicates how many blocks are walked backwards.
type GetStakeVersionsCmd = types.GetStakeVersionsCmd

// NewGetStakeVersionsCmd returns a new instance which can be used to
// issue a JSON-RPC getstakeversions command.
func NewGetStakeVersionsCmd(hash string, count int32) *GetStakeVersionsCmd {
	return types.NewGetStakeVersionsCmd(hash, count)
}

// GetTicketPoolValueCmd defines the getticketpoolvalue JSON-RPC command.
type GetTicketPoolValueCmd = types.GetTicketPoolValueCmd

// NewGetTicketPoolValueCmd returns a new instance which can be used to issue a
// getticketpoolvalue JSON-RPC command.
func NewGetTicketPoolValueCmd() *GetTicketPoolValueCmd {
	return types.NewGetTicketPoolValueCmd()
}

// GetTxOutCmd defines the gettxout JSON-RPC command.
type GetTxOutCmd = types.GetTxOutCmd

// NewGetTxOutCmd returns a new instance which can be used to issue a gettxout
// JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetTxOutCmd(txHash string, vout uint32, includeMempool *bool) *GetTxOutCmd {
	return types.NewGetTxOutCmd(txHash, vout, includeMempool)
}

// GetTxOutSetInfoCmd defines the gettxoutsetinfo JSON-RPC command.
type GetTxOutSetInfoCmd = types.GetTxOutSetInfoCmd

// NewGetTxOutSetInfoCmd returns a new instance which can be used to issue a
// gettxoutsetinfo JSON-RPC command.
func NewGetTxOutSetInfoCmd() *GetTxOutSetInfoCmd {
	return types.NewGetTxOutSetInfoCmd()
}

// GetVoteInfoCmd returns voting results over a range of blocks.  Count
// indicates how many blocks are walked backwards.
type GetVoteInfoCmd = types.GetVoteInfoCmd

// NewGetVoteInfoCmd returns a new instance which can be used to
// issue a JSON-RPC getvoteinfo command.
func NewGetVoteInfoCmd(version uint32) *GetVoteInfoCmd {
	return types.NewGetVoteInfoCmd(version)
}

// GetWorkCmd defines the getwork JSON-RPC command.
type GetWorkCmd = types.GetWorkCmd

// NewGetWorkCmd returns a new instance which can be used to issue a getwork
// JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetWorkCmd(data *string) *GetWorkCmd {
	return types.NewGetWorkCmd(data)
}

// HelpCmd defines the help JSON-RPC command.
type HelpCmd = types.HelpCmd

// NewHelpCmd returns a new instance which can be used to issue a help JSON-RPC
// command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewHelpCmd(command *string) *HelpCmd {
	return types.NewHelpCmd(command)
}

// LiveTicketsCmd is a type handling custom marshaling and
// unmarshaling of livetickets JSON RPC commands.
type LiveTicketsCmd = types.LiveTicketsCmd

// NewLiveTicketsCmd returns a new instance which can be used to issue a JSON-RPC
// livetickets command.
func NewLiveTicketsCmd() *LiveTicketsCmd {
	return types.NewLiveTicketsCmd()
}

// MissedTicketsCmd is a type handling custom marshaling and
// unmarshaling of missedtickets JSON RPC commands.
type MissedTicketsCmd = types.MissedTicketsCmd

// NewMissedTicketsCmd returns a new instance which can be used to issue a JSON-RPC
// missedtickets command.
func NewMissedTicketsCmd() *MissedTicketsCmd {
	return types.NewMissedTicketsCmd()
}

// NodeCmd defines the dropnode JSON-RPC command.
type NodeCmd = types.NodeCmd

// NewNodeCmd returns a new instance which can be used to issue a `node`
// JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewNodeCmd(subCmd NodeSubCmd, target string, connectSubCmd *string) *NodeCmd {
	return types.NewNodeCmd(subCmd, target, connectSubCmd)
}

// PingCmd defines the ping JSON-RPC command.
type PingCmd = types.PingCmd

// NewPingCmd returns a new instance which can be used to issue a ping JSON-RPC
// command.
func NewPingCmd() *PingCmd {
	return types.NewPingCmd()
}

// RebroadcastMissedCmd is a type handling custom marshaling and
// unmarshaling of rebroadcastwinners JSON RPC commands.
type RebroadcastMissedCmd = types.RebroadcastMissedCmd

// NewRebroadcastMissedCmd returns a new instance which can be used to
// issue a JSON-RPC rebroadcastmissed command.
func NewRebroadcastMissedCmd() *RebroadcastMissedCmd {
	return types.NewRebroadcastMissedCmd()
}

// RebroadcastWinnersCmd is a type handling custom marshaling and
// unmarshaling of rebroadcastwinners JSON RPC commands.
type RebroadcastWinnersCmd = types.RebroadcastWinnersCmd

// NewRebroadcastWinnersCmd returns a new instance which can be used to
// issue a JSON-RPC rebroadcastwinners command.
func NewRebroadcastWinnersCmd() *RebroadcastWinnersCmd {
	return types.NewRebroadcastWinnersCmd()
}

// SearchRawTransactionsCmd defines the searchrawtransactions JSON-RPC command.
type SearchRawTransactionsCmd = types.SearchRawTransactionsCmd

// NewSearchRawTransactionsCmd returns a new instance which can be used to issue a
// sendrawtransaction JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSearchRawTransactionsCmd(address string, verbose, skip, count *int, vinExtra *int, reverse *bool, filterAddrs *[]string) *SearchRawTransactionsCmd {
	return types.NewSearchRawTransactionsCmd(address, verbose, skip, count, vinExtra, reverse, filterAddrs)
}

// SendRawTransactionCmd defines the sendrawtransaction JSON-RPC command.
type SendRawTransactionCmd = types.SendRawTransactionCmd

// NewSendRawTransactionCmd returns a new instance which can be used to issue a
// sendrawtransaction JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSendRawTransactionCmd(hexTx string, allowHighFees *bool) *SendRawTransactionCmd {
	return types.NewSendRawTransactionCmd(hexTx, allowHighFees)
}

// SetGenerateCmd defines the setgenerate JSON-RPC command.
type SetGenerateCmd = types.SetGenerateCmd

// NewSetGenerateCmd returns a new instance which can be used to issue a
// setgenerate JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSetGenerateCmd(generate bool, genProcLimit *int) *SetGenerateCmd {
	return types.NewSetGenerateCmd(generate, genProcLimit)
}

// StopCmd defines the stop JSON-RPC command.
type StopCmd = types.StopCmd

// NewStopCmd returns a new instance which can be used to issue a stop JSON-RPC
// command.
func NewStopCmd() *StopCmd {
	return types.NewStopCmd()
}

// SubmitBlockOptions represents the optional options struct provided with a
// SubmitBlockCmd command.
type SubmitBlockOptions = types.SubmitBlockOptions

// SubmitBlockCmd defines the submitblock JSON-RPC command.
type SubmitBlockCmd = types.SubmitBlockCmd

// NewSubmitBlockCmd returns a new instance which can be used to issue a
// submitblock JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSubmitBlockCmd(hexBlock string, options *SubmitBlockOptions) *SubmitBlockCmd {
	return types.NewSubmitBlockCmd(hexBlock, options)
}

// TicketFeeInfoCmd defines the ticketsfeeinfo JSON-RPC command.
type TicketFeeInfoCmd = types.TicketFeeInfoCmd

// NewTicketFeeInfoCmd returns a new instance which can be used to issue a
// JSON-RPC ticket fee info command.
func NewTicketFeeInfoCmd(blocks *uint32, windows *uint32) *TicketFeeInfoCmd {
	return types.NewTicketFeeInfoCmd(blocks, windows)
}

// TicketsForAddressCmd defines the ticketsforbucket JSON-RPC command.
type TicketsForAddressCmd = types.TicketsForAddressCmd

// NewTicketsForAddressCmd returns a new instance which can be used to issue a
// JSON-RPC tickets for bucket command.
func NewTicketsForAddressCmd(addr string) *TicketsForAddressCmd {
	return types.NewTicketsForAddressCmd(addr)
}

// TicketVWAPCmd defines the ticketvwap JSON-RPC command.
type TicketVWAPCmd = types.TicketVWAPCmd

// NewTicketVWAPCmd returns a new instance which can be used to issue a
// JSON-RPC ticket volume weight average price command.
func NewTicketVWAPCmd(start *uint32, end *uint32) *TicketVWAPCmd {
	return types.NewTicketVWAPCmd(start, end)
}

// TxFeeInfoCmd defines the ticketsfeeinfo JSON-RPC command.
type TxFeeInfoCmd = types.TxFeeInfoCmd

// NewTxFeeInfoCmd returns a new instance which can be used to issue a
// JSON-RPC ticket fee info command.
func NewTxFeeInfoCmd(blocks *uint32, start *uint32, end *uint32) *TxFeeInfoCmd {
	return types.NewTxFeeInfoCmd(blocks, start, end)
}

// ValidateAddressCmd defines the validateaddress JSON-RPC command.
type ValidateAddressCmd = types.ValidateAddressCmd

// NewValidateAddressCmd returns a new instance which can be used to issue a
// validateaddress JSON-RPC command.
func NewValidateAddressCmd(address string) *ValidateAddressCmd {
	return types.NewValidateAddressCmd(address)
}

// VerifyChainCmd defines the verifychain JSON-RPC command.
type VerifyChainCmd = types.VerifyChainCmd

// NewVerifyChainCmd returns a new instance which can be used to issue a
// verifychain JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewVerifyChainCmd(checkLevel, checkDepth *int64) *VerifyChainCmd {
	return types.NewVerifyChainCmd(checkLevel, checkDepth)
}

// VerifyMessageCmd defines the verifymessage JSON-RPC command.
type VerifyMessageCmd = types.VerifyMessageCmd

// NewVerifyMessageCmd returns a new instance which can be used to issue a
// verifymessage JSON-RPC command.
func NewVerifyMessageCmd(address, signature, message string) *VerifyMessageCmd {
	return types.NewVerifyMessageCmd(address, signature, message)
}

// VersionCmd defines the version JSON-RPC command.
type VersionCmd = types.VersionCmd

// NewVersionCmd returns a new instance which can be used to issue a JSON-RPC
// version command.
func NewVersionCmd() *VersionCmd { return types.NewVersionCmd() }

func init() {
	// No special flags for commands in this file.
	flags := UsageFlag(0)

	MustRegisterCmd("addnode", (*AddNodeCmd)(nil), flags)
	MustRegisterCmd("createrawssrtx", (*CreateRawSSRtxCmd)(nil), flags)
	MustRegisterCmd("createrawsstx", (*CreateRawSStxCmd)(nil), flags)
	MustRegisterCmd("createrawtransaction", (*CreateRawTransactionCmd)(nil), flags)
	MustRegisterCmd("debuglevel", (*DebugLevelCmd)(nil), flags)
	MustRegisterCmd("decoderawtransaction", (*DecodeRawTransactionCmd)(nil), flags)
	MustRegisterCmd("decodescript", (*DecodeScriptCmd)(nil), flags)
	MustRegisterCmd("estimatefee", (*EstimateFeeCmd)(nil), flags)
	MustRegisterCmd("estimatesmartfee", (*EstimateSmartFeeCmd)(nil), flags)
	MustRegisterCmd("estimatestakediff", (*EstimateStakeDiffCmd)(nil), flags)
	MustRegisterCmd("existsaddress", (*ExistsAddressCmd)(nil), flags)
	MustRegisterCmd("existsaddresses", (*ExistsAddressesCmd)(nil), flags)
	MustRegisterCmd("existsmissedtickets", (*ExistsMissedTicketsCmd)(nil), flags)
	MustRegisterCmd("existsexpiredtickets", (*ExistsExpiredTicketsCmd)(nil), flags)
	MustRegisterCmd("existsliveticket", (*ExistsLiveTicketCmd)(nil), flags)
	MustRegisterCmd("existslivetickets", (*ExistsLiveTicketsCmd)(nil), flags)
	MustRegisterCmd("existsmempooltxs", (*ExistsMempoolTxsCmd)(nil), flags)
	MustRegisterCmd("generate", (*GenerateCmd)(nil), flags)
	MustRegisterCmd("getaddednodeinfo", (*GetAddedNodeInfoCmd)(nil), flags)
	MustRegisterCmd("getbestblock", (*GetBestBlockCmd)(nil), flags)
	MustRegisterCmd("getbestblockhash", (*GetBestBlockHashCmd)(nil), flags)
	MustRegisterCmd("getblock", (*GetBlockCmd)(nil), flags)
	MustRegisterCmd("getblockchaininfo", (*GetBlockChainInfoCmd)(nil), flags)
	MustRegisterCmd("getblockcount", (*GetBlockCountCmd)(nil), flags)
	MustRegisterCmd("getblockhash", (*GetBlockHashCmd)(nil), flags)
	MustRegisterCmd("getblockheader", (*GetBlockHeaderCmd)(nil), flags)
	MustRegisterCmd("getblocksubsidy", (*GetBlockSubsidyCmd)(nil), flags)
	MustRegisterCmd("getblocktemplate", (*GetBlockTemplateCmd)(nil), flags)
	MustRegisterCmd("getcfilter", (*GetCFilterCmd)(nil), flags)
	MustRegisterCmd("getcfilterheader", (*GetCFilterHeaderCmd)(nil), flags)
	MustRegisterCmd("getchaintips", (*GetChainTipsCmd)(nil), flags)
	MustRegisterCmd("getcoinsupply", (*GetCoinSupplyCmd)(nil), flags)
	MustRegisterCmd("getconnectioncount", (*GetConnectionCountCmd)(nil), flags)
	MustRegisterCmd("getcurrentnet", (*GetCurrentNetCmd)(nil), flags)
	MustRegisterCmd("getdifficulty", (*GetDifficultyCmd)(nil), flags)
	MustRegisterCmd("getgenerate", (*GetGenerateCmd)(nil), flags)
	MustRegisterCmd("gethashespersec", (*GetHashesPerSecCmd)(nil), flags)
	MustRegisterCmd("getheaders", (*GetHeadersCmd)(nil), flags)
	MustRegisterCmd("getinfo", (*GetInfoCmd)(nil), flags)
	MustRegisterCmd("getmempoolinfo", (*GetMempoolInfoCmd)(nil), flags)
	MustRegisterCmd("getmininginfo", (*GetMiningInfoCmd)(nil), flags)
	MustRegisterCmd("getnetworkinfo", (*GetNetworkInfoCmd)(nil), flags)
	MustRegisterCmd("getnettotals", (*GetNetTotalsCmd)(nil), flags)
	MustRegisterCmd("getnetworkhashps", (*GetNetworkHashPSCmd)(nil), flags)
	MustRegisterCmd("getpeerinfo", (*GetPeerInfoCmd)(nil), flags)
	MustRegisterCmd("getrawmempool", (*GetRawMempoolCmd)(nil), flags)
	MustRegisterCmd("getrawtransaction", (*GetRawTransactionCmd)(nil), flags)
	MustRegisterCmd("getstakedifficulty", (*GetStakeDifficultyCmd)(nil), flags)
	MustRegisterCmd("getstakeversioninfo", (*GetStakeVersionInfoCmd)(nil), flags)
	MustRegisterCmd("getstakeversions", (*GetStakeVersionsCmd)(nil), flags)
	MustRegisterCmd("getticketpoolvalue", (*GetTicketPoolValueCmd)(nil), flags)
	MustRegisterCmd("gettxout", (*GetTxOutCmd)(nil), flags)
	MustRegisterCmd("gettxoutsetinfo", (*GetTxOutSetInfoCmd)(nil), flags)
	MustRegisterCmd("getvoteinfo", (*GetVoteInfoCmd)(nil), flags)
	MustRegisterCmd("getwork", (*GetWorkCmd)(nil), flags)
	MustRegisterCmd("help", (*HelpCmd)(nil), flags)
	MustRegisterCmd("livetickets", (*LiveTicketsCmd)(nil), flags)
	MustRegisterCmd("missedtickets", (*MissedTicketsCmd)(nil), flags)
	MustRegisterCmd("node", (*NodeCmd)(nil), flags)
	MustRegisterCmd("ping", (*PingCmd)(nil), flags)
	MustRegisterCmd("rebroadcastmissed", (*RebroadcastMissedCmd)(nil), flags)
	MustRegisterCmd("rebroadcastwinners", (*RebroadcastWinnersCmd)(nil), flags)
	MustRegisterCmd("searchrawtransactions", (*SearchRawTransactionsCmd)(nil), flags)
	MustRegisterCmd("sendrawtransaction", (*SendRawTransactionCmd)(nil), flags)
	MustRegisterCmd("setgenerate", (*SetGenerateCmd)(nil), flags)
	MustRegisterCmd("stop", (*StopCmd)(nil), flags)
	MustRegisterCmd("submitblock", (*SubmitBlockCmd)(nil), flags)
	MustRegisterCmd("ticketfeeinfo", (*TicketFeeInfoCmd)(nil), flags)
	MustRegisterCmd("ticketsforaddress", (*TicketsForAddressCmd)(nil), flags)
	MustRegisterCmd("ticketvwap", (*TicketVWAPCmd)(nil), flags)
	MustRegisterCmd("txfeeinfo", (*TxFeeInfoCmd)(nil), flags)
	MustRegisterCmd("validateaddress", (*ValidateAddressCmd)(nil), flags)
	MustRegisterCmd("verifychain", (*VerifyChainCmd)(nil), flags)
	MustRegisterCmd("verifymessage", (*VerifyMessageCmd)(nil), flags)
	MustRegisterCmd("version", (*VersionCmd)(nil), flags)
}
