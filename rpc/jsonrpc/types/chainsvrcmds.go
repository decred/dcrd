// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a chain server.

package types

import (
	"github.com/decred/dcrd/dcrjson/v3"
)

// AddNodeSubCmd defines the type used in the addnode JSON-RPC command for the
// sub command field.
type AddNodeSubCmd string

const (
	// ANAdd indicates the specified host should be added as a persistent
	// peer.
	ANAdd AddNodeSubCmd = "add"

	// ANRemove indicates the specified peer should be removed.
	ANRemove AddNodeSubCmd = "remove"

	// ANOneTry indicates the specified host should try to connect once,
	// but it should not be made persistent.
	ANOneTry AddNodeSubCmd = "onetry"
)

// NodeSubCmd defines the type used in the node JSON-RPC command for the
// sub command field.
type NodeSubCmd string

const (
	// NConnect indicates the specified host that should be connected to.
	NConnect NodeSubCmd = "connect"

	// NRemove indicates the specified peer that should be removed as a
	// persistent peer.
	NRemove NodeSubCmd = "remove"

	// NDisconnect indicates the specified peer should be disconnected.
	NDisconnect NodeSubCmd = "disconnect"
)

// AddNodeCmd defines the addnode JSON-RPC command.
type AddNodeCmd struct {
	Addr   string
	SubCmd AddNodeSubCmd `jsonrpcusage:"\"add|remove|onetry\""`
}

// NewAddNodeCmd returns a new instance which can be used to issue an addnode
// JSON-RPC command.
func NewAddNodeCmd(addr string, subCmd AddNodeSubCmd) *AddNodeCmd {
	return &AddNodeCmd{
		Addr:   addr,
		SubCmd: subCmd,
	}
}

// SStxInput represents the inputs to an SStx transaction. Specifically a
// transactionsha and output number pair, along with the output amounts.
type SStxInput struct {
	Txid string `json:"txid"`
	Vout uint32 `json:"vout"`
	Tree int8   `json:"tree"`
	Amt  int64  `json:"amt"`
}

// SStxCommitOut represents the output to an SStx transaction. Specifically a
// a commitment address and amount, and a change address and amount.
type SStxCommitOut struct {
	Addr       string `json:"addr"`
	CommitAmt  int64  `json:"commitamt"`
	ChangeAddr string `json:"changeaddr"`
	ChangeAmt  int64  `json:"changeamt"`
}

// CreateRawSStxCmd is a type handling custom marshaling and
// unmarshaling of createrawsstx JSON RPC commands.
type CreateRawSStxCmd struct {
	Inputs []SStxInput
	Amount map[string]int64 `jsonrpcusage:"{\"address\":amount}"` // in atoms
	COuts  []SStxCommitOut
}

// NewCreateRawSStxCmd creates a new CreateRawSStxCmd.
func NewCreateRawSStxCmd(inputs []SStxInput, amount map[string]int64,
	couts []SStxCommitOut) *CreateRawSStxCmd {
	return &CreateRawSStxCmd{
		Inputs: inputs,
		Amount: amount,
		COuts:  couts,
	}
}

// CreateRawSSRtxCmd is a type handling custom marshaling and
// unmarshaling of createrawssrtx JSON RPC commands.
type CreateRawSSRtxCmd struct {
	Inputs []TransactionInput `jsonrpcusage:"[{\"amount\":n.nnn,\"txid\":\"value\",\"vout\":n,\"tree\":n}]"` // only one input is accepted
	Fee    *float64
}

// NewCreateRawSSRtxCmd creates a new CreateRawSSRtxCmd.
func NewCreateRawSSRtxCmd(inputs []TransactionInput, fee *float64) *CreateRawSSRtxCmd {
	return &CreateRawSSRtxCmd{
		Inputs: inputs,
		Fee:    fee,
	}
}

// TransactionInput represents the inputs to a transaction.  Specifically a
// transaction hash and output number pair. Contains Decred additions.
type TransactionInput struct {
	Amount float64 `json:"amount,omitempty"`
	Txid   string  `json:"txid"`
	Vout   uint32  `json:"vout"`
	Tree   int8    `json:"tree"`
}

// CreateRawTransactionCmd defines the createrawtransaction JSON-RPC command.
type CreateRawTransactionCmd struct {
	Inputs   []TransactionInput
	Amounts  map[string]float64 `jsonrpcusage:"{\"address\":amount,...}"` // In DCR
	LockTime *int64
	Expiry   *int64
}

// NewCreateRawTransactionCmd returns a new instance which can be used to issue
// a createrawtransaction JSON-RPC command.
//
// Amounts are in DCR.
func NewCreateRawTransactionCmd(inputs []TransactionInput, amounts map[string]float64,
	lockTime *int64, expiry *int64) *CreateRawTransactionCmd {

	return &CreateRawTransactionCmd{
		Inputs:   inputs,
		Amounts:  amounts,
		LockTime: lockTime,
		Expiry:   expiry,
	}
}

// DebugLevelCmd defines the debuglevel JSON-RPC command.  This command is not a
// standard Bitcoin command.  It is an extension for btcd.
type DebugLevelCmd struct {
	LevelSpec string
}

// NewDebugLevelCmd returns a new DebugLevelCmd which can be used to issue a
// debuglevel JSON-RPC command.  This command is not a standard Bitcoin command.
// It is an extension for btcd.
func NewDebugLevelCmd(levelSpec string) *DebugLevelCmd {
	return &DebugLevelCmd{
		LevelSpec: levelSpec,
	}
}

// DecodeRawTransactionCmd defines the decoderawtransaction JSON-RPC command.
type DecodeRawTransactionCmd struct {
	HexTx string
}

// NewDecodeRawTransactionCmd returns a new instance which can be used to issue
// a decoderawtransaction JSON-RPC command.
func NewDecodeRawTransactionCmd(hexTx string) *DecodeRawTransactionCmd {
	return &DecodeRawTransactionCmd{
		HexTx: hexTx,
	}
}

// DecodeScriptCmd defines the decodescript JSON-RPC command.
type DecodeScriptCmd struct {
	HexScript string
	Version   *uint16
}

// NewDecodeScriptCmd returns a new instance which can be used to issue a
// decodescript JSON-RPC command.
func NewDecodeScriptCmd(hexScript string) *DecodeScriptCmd {
	return &DecodeScriptCmd{
		HexScript: hexScript,
	}
}

// EstimateFeeCmd defines the estimatefee JSON-RPC command.
type EstimateFeeCmd struct {
	NumBlocks int64
}

// NewEstimateFeeCmd returns a new instance which can be used to issue an
// estimatefee JSON-RPC command.
func NewEstimateFeeCmd(numBlocks int64) *EstimateFeeCmd {
	return &EstimateFeeCmd{
		NumBlocks: numBlocks,
	}
}

// EstimateSmartFeeMode defines estimation mode to be used with
// the estimatesmartfee command.
type EstimateSmartFeeMode string

const (
	// EstimateSmartFeeEconomical returns an
	// economical result.
	EstimateSmartFeeEconomical EstimateSmartFeeMode = "economical"

	// EstimateSmartFeeConservative potentially returns
	// a conservative result.
	EstimateSmartFeeConservative EstimateSmartFeeMode = "conservative"
)

// EstimateSmartFeeCmd defines the estimatesmartfee JSON-RPC command.
type EstimateSmartFeeCmd struct {
	Confirmations int64
	Mode          *EstimateSmartFeeMode `jsonrpcdefault:"\"conservative\""`
}

// NewEstimateSmartFeeCmd returns a new instance which can be used to issue an
// estimatesmartfee JSON-RPC command.
func NewEstimateSmartFeeCmd(confirmations int64, mode *EstimateSmartFeeMode) *EstimateSmartFeeCmd {
	return &EstimateSmartFeeCmd{
		Confirmations: confirmations,
		Mode:          mode,
	}
}

// EstimateStakeDiffCmd defines the eststakedifficulty JSON-RPC command.
type EstimateStakeDiffCmd struct {
	Tickets *uint32
}

// NewEstimateStakeDiffCmd defines the eststakedifficulty JSON-RPC command.
func NewEstimateStakeDiffCmd(tickets *uint32) *EstimateStakeDiffCmd {
	return &EstimateStakeDiffCmd{
		Tickets: tickets,
	}
}

// ExistsAddressCmd defines the existsaddress JSON-RPC command.
type ExistsAddressCmd struct {
	Address string
}

// NewExistsAddressCmd returns a new instance which can be used to issue a
// existsaddress JSON-RPC command.
func NewExistsAddressCmd(address string) *ExistsAddressCmd {
	return &ExistsAddressCmd{
		Address: address,
	}
}

// ExistsAddressesCmd defines the existsaddresses JSON-RPC command.
type ExistsAddressesCmd struct {
	Addresses []string
}

// NewExistsAddressesCmd returns a new instance which can be used to issue an
// existsaddresses JSON-RPC command.
func NewExistsAddressesCmd(addresses []string) *ExistsAddressesCmd {
	return &ExistsAddressesCmd{
		Addresses: addresses,
	}
}

// ExistsMissedTicketsCmd defines the existsmissedtickets JSON-RPC command.
type ExistsMissedTicketsCmd struct {
	TxHashes []string
}

// NewExistsMissedTicketsCmd returns a new instance which can be used to issue an
// existsmissedtickets JSON-RPC command.
func NewExistsMissedTicketsCmd(txHashes []string) *ExistsMissedTicketsCmd {
	return &ExistsMissedTicketsCmd{
		TxHashes: txHashes,
	}
}

// ExistsExpiredTicketsCmd defines the existsexpiredtickets JSON-RPC command.
type ExistsExpiredTicketsCmd struct {
	TxHashes []string
}

// NewExistsExpiredTicketsCmd returns a new instance which can be used to issue an
// existsexpiredtickets JSON-RPC command.
func NewExistsExpiredTicketsCmd(txHashes []string) *ExistsExpiredTicketsCmd {
	return &ExistsExpiredTicketsCmd{
		TxHashes: txHashes,
	}
}

// ExistsLiveTicketCmd defines the existsliveticket JSON-RPC command.
type ExistsLiveTicketCmd struct {
	TxHash string
}

// NewExistsLiveTicketCmd returns a new instance which can be used to issue an
// existsliveticket JSON-RPC command.
func NewExistsLiveTicketCmd(txHash string) *ExistsLiveTicketCmd {
	return &ExistsLiveTicketCmd{
		TxHash: txHash,
	}
}

// ExistsLiveTicketsCmd defines the existslivetickets JSON-RPC command.
type ExistsLiveTicketsCmd struct {
	TxHashes []string
}

// NewExistsLiveTicketsCmd returns a new instance which can be used to issue an
// existslivetickets JSON-RPC command.
func NewExistsLiveTicketsCmd(txHashes []string) *ExistsLiveTicketsCmd {
	return &ExistsLiveTicketsCmd{
		TxHashes: txHashes,
	}
}

// ExistsMempoolTxsCmd defines the existsmempooltxs JSON-RPC command.
type ExistsMempoolTxsCmd struct {
	TxHashes []string
}

// NewExistsMempoolTxsCmd returns a new instance which can be used to issue an
// existsmempooltxs JSON-RPC command.
func NewExistsMempoolTxsCmd(txHashes []string) *ExistsMempoolTxsCmd {
	return &ExistsMempoolTxsCmd{
		TxHashes: txHashes,
	}
}

// GenerateCmd defines the generate JSON-RPC command.
type GenerateCmd struct {
	NumBlocks uint32
}

// NewGenerateCmd returns a new instance which can be used to issue a generate
// JSON-RPC command.
func NewGenerateCmd(numBlocks uint32) *GenerateCmd {
	return &GenerateCmd{
		NumBlocks: numBlocks,
	}
}

// GetAddedNodeInfoCmd defines the getaddednodeinfo JSON-RPC command.
type GetAddedNodeInfoCmd struct {
	DNS  bool
	Node *string
}

// NewGetAddedNodeInfoCmd returns a new instance which can be used to issue a
// getaddednodeinfo JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetAddedNodeInfoCmd(dns bool, node *string) *GetAddedNodeInfoCmd {
	return &GetAddedNodeInfoCmd{
		DNS:  dns,
		Node: node,
	}
}

// GetBestBlockCmd defines the getbestblock JSON-RPC command.
type GetBestBlockCmd struct{}

// NewGetBestBlockCmd returns a new instance which can be used to issue a
// getbestblock JSON-RPC command.
func NewGetBestBlockCmd() *GetBestBlockCmd {
	return &GetBestBlockCmd{}
}

// GetBestBlockHashCmd defines the getbestblockhash JSON-RPC command.
type GetBestBlockHashCmd struct{}

// NewGetBestBlockHashCmd returns a new instance which can be used to issue a
// getbestblockhash JSON-RPC command.
func NewGetBestBlockHashCmd() *GetBestBlockHashCmd {
	return &GetBestBlockHashCmd{}
}

// GetBlockCmd defines the getblock JSON-RPC command.
type GetBlockCmd struct {
	Hash      string
	Verbose   *bool `jsonrpcdefault:"true"`
	VerboseTx *bool `jsonrpcdefault:"false"`
}

// NewGetBlockCmd returns a new instance which can be used to issue a getblock
// JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetBlockCmd(hash string, verbose, verboseTx *bool) *GetBlockCmd {
	return &GetBlockCmd{
		Hash:      hash,
		Verbose:   verbose,
		VerboseTx: verboseTx,
	}
}

// GetBlockChainInfoCmd defines the getblockchaininfo JSON-RPC command.
type GetBlockChainInfoCmd struct{}

// NewGetBlockChainInfoCmd returns a new instance which can be used to issue a
// getblockchaininfo JSON-RPC command.
func NewGetBlockChainInfoCmd() *GetBlockChainInfoCmd {
	return &GetBlockChainInfoCmd{}
}

// GetBlockCountCmd defines the getblockcount JSON-RPC command.
type GetBlockCountCmd struct{}

// NewGetBlockCountCmd returns a new instance which can be used to issue a
// getblockcount JSON-RPC command.
func NewGetBlockCountCmd() *GetBlockCountCmd {
	return &GetBlockCountCmd{}
}

// GetBlockHashCmd defines the getblockhash JSON-RPC command.
type GetBlockHashCmd struct {
	Index int64
}

// NewGetBlockHashCmd returns a new instance which can be used to issue a
// getblockhash JSON-RPC command.
func NewGetBlockHashCmd(index int64) *GetBlockHashCmd {
	return &GetBlockHashCmd{
		Index: index,
	}
}

// GetBlockHeaderCmd defines the getblockheader JSON-RPC command.
type GetBlockHeaderCmd struct {
	Hash    string
	Verbose *bool `jsonrpcdefault:"true"`
}

// NewGetBlockHeaderCmd returns a new instance which can be used to issue a
// getblockheader JSON-RPC command.
func NewGetBlockHeaderCmd(hash string, verbose *bool) *GetBlockHeaderCmd {
	return &GetBlockHeaderCmd{
		Hash:    hash,
		Verbose: verbose,
	}
}

// GetBlockSubsidyCmd defines the getblocksubsidy JSON-RPC command.
type GetBlockSubsidyCmd struct {
	Height int64
	Voters uint16
}

// NewGetBlockSubsidyCmd returns a new instance which can be used to issue a
// getblocksubsidy JSON-RPC command.
func NewGetBlockSubsidyCmd(height int64, voters uint16) *GetBlockSubsidyCmd {
	return &GetBlockSubsidyCmd{
		Height: height,
		Voters: voters,
	}
}

// GetCFilterCmd defines the getcfilter JSON-RPC command.
type GetCFilterCmd struct {
	Hash       string
	FilterType string
}

// NewGetCFilterCmd returns a new instance which can be used to issue a
// getcfilter JSON-RPC command.
func NewGetCFilterCmd(hash string, filterType string) *GetCFilterCmd {
	return &GetCFilterCmd{
		Hash:       hash,
		FilterType: filterType,
	}
}

// GetCFilterHeaderCmd defines the getcfilterheader JSON-RPC command.
type GetCFilterHeaderCmd struct {
	Hash       string
	FilterType string
}

// NewGetCFilterHeaderCmd returns a new instance which can be used to issue a
// getcfilterheader JSON-RPC command.
func NewGetCFilterHeaderCmd(hash string, filterType string) *GetCFilterHeaderCmd {
	return &GetCFilterHeaderCmd{
		Hash:       hash,
		FilterType: filterType,
	}
}

// GetCFilterV2Cmd defines the getcfilterv2 JSON-RPC command.
type GetCFilterV2Cmd struct {
	BlockHash string
}

// NewGetCFilterV2Cmd returns a new instance which can be used to issue a
// getcfilterv2 JSON-RPC command.
func NewGetCFilterV2Cmd(hash string) *GetCFilterV2Cmd {
	return &GetCFilterV2Cmd{
		BlockHash: hash,
	}
}

// GetChainTipsCmd defines the getchaintips JSON-RPC command.
type GetChainTipsCmd struct{}

// NewGetChainTipsCmd returns a new instance which can be used to issue a
// getchaintips JSON-RPC command.
func NewGetChainTipsCmd() *GetChainTipsCmd {
	return &GetChainTipsCmd{}
}

// GetCoinSupplyCmd defines the getcoinsupply JSON-RPC command.
type GetCoinSupplyCmd struct{}

// NewGetCoinSupplyCmd returns a new instance which can be used to issue a
// getcoinsupply JSON-RPC command.
func NewGetCoinSupplyCmd() *GetCoinSupplyCmd {
	return &GetCoinSupplyCmd{}
}

// GetConnectionCountCmd defines the getconnectioncount JSON-RPC command.
type GetConnectionCountCmd struct{}

// NewGetConnectionCountCmd returns a new instance which can be used to issue a
// getconnectioncount JSON-RPC command.
func NewGetConnectionCountCmd() *GetConnectionCountCmd {
	return &GetConnectionCountCmd{}
}

// GetCurrentNetCmd defines the getcurrentnet JSON-RPC command.
type GetCurrentNetCmd struct{}

// NewGetCurrentNetCmd returns a new instance which can be used to issue a
// getcurrentnet JSON-RPC command.
func NewGetCurrentNetCmd() *GetCurrentNetCmd {
	return &GetCurrentNetCmd{}
}

// GetDifficultyCmd defines the getdifficulty JSON-RPC command.
type GetDifficultyCmd struct{}

// NewGetDifficultyCmd returns a new instance which can be used to issue a
// getdifficulty JSON-RPC command.
func NewGetDifficultyCmd() *GetDifficultyCmd {
	return &GetDifficultyCmd{}
}

// GetGenerateCmd defines the getgenerate JSON-RPC command.
type GetGenerateCmd struct{}

// NewGetGenerateCmd returns a new instance which can be used to issue a
// getgenerate JSON-RPC command.
func NewGetGenerateCmd() *GetGenerateCmd {
	return &GetGenerateCmd{}
}

// GetHashesPerSecCmd defines the gethashespersec JSON-RPC command.
type GetHashesPerSecCmd struct{}

// NewGetHashesPerSecCmd returns a new instance which can be used to issue a
// gethashespersec JSON-RPC command.
func NewGetHashesPerSecCmd() *GetHashesPerSecCmd {
	return &GetHashesPerSecCmd{}
}

// GetInfoCmd defines the getinfo JSON-RPC command.
type GetInfoCmd struct{}

// NewGetInfoCmd returns a new instance which can be used to issue a
// getinfo JSON-RPC command.
func NewGetInfoCmd() *GetInfoCmd {
	return &GetInfoCmd{}
}

// GetHeadersCmd defines the getheaders JSON-RPC command.
type GetHeadersCmd struct {
	BlockLocators []string `json:"blocklocators"`
	HashStop      string   `json:"hashstop"`
}

// NewGetHeadersCmd returns a new instance which can be used to issue a
// getheaders JSON-RPC command.
func NewGetHeadersCmd(blockLocators []string, hashStop string) *GetHeadersCmd {
	return &GetHeadersCmd{
		BlockLocators: blockLocators,
		HashStop:      hashStop,
	}
}

// GetMempoolInfoCmd defines the getmempoolinfo JSON-RPC command.
type GetMempoolInfoCmd struct{}

// NewGetMempoolInfoCmd returns a new instance which can be used to issue a
// getmempool JSON-RPC command.
func NewGetMempoolInfoCmd() *GetMempoolInfoCmd {
	return &GetMempoolInfoCmd{}
}

// GetMiningInfoCmd defines the getmininginfo JSON-RPC command.
type GetMiningInfoCmd struct{}

// NewGetMiningInfoCmd returns a new instance which can be used to issue a
// getmininginfo JSON-RPC command.
func NewGetMiningInfoCmd() *GetMiningInfoCmd {
	return &GetMiningInfoCmd{}
}

// GetNetworkInfoCmd defines the getnetworkinfo JSON-RPC command.
type GetNetworkInfoCmd struct{}

// NewGetNetworkInfoCmd returns a new instance which can be used to issue a
// getnetworkinfo JSON-RPC command.
func NewGetNetworkInfoCmd() *GetNetworkInfoCmd {
	return &GetNetworkInfoCmd{}
}

// GetNetTotalsCmd defines the getnettotals JSON-RPC command.
type GetNetTotalsCmd struct{}

// NewGetNetTotalsCmd returns a new instance which can be used to issue a
// getnettotals JSON-RPC command.
func NewGetNetTotalsCmd() *GetNetTotalsCmd {
	return &GetNetTotalsCmd{}
}

// GetNetworkHashPSCmd defines the getnetworkhashps JSON-RPC command.
type GetNetworkHashPSCmd struct {
	Blocks *int `jsonrpcdefault:"120"`
	Height *int `jsonrpcdefault:"-1"`
}

// NewGetNetworkHashPSCmd returns a new instance which can be used to issue a
// getnetworkhashps JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetNetworkHashPSCmd(numBlocks, height *int) *GetNetworkHashPSCmd {
	return &GetNetworkHashPSCmd{
		Blocks: numBlocks,
		Height: height,
	}
}

// GetPeerInfoCmd defines the getpeerinfo JSON-RPC command.
type GetPeerInfoCmd struct{}

// NewGetPeerInfoCmd returns a new instance which can be used to issue a getpeer
// JSON-RPC command.
func NewGetPeerInfoCmd() *GetPeerInfoCmd {
	return &GetPeerInfoCmd{}
}

// GetRawMempoolTxTypeCmd defines the type used in the getrawmempool JSON-RPC
// command for the TxType command field.
type GetRawMempoolTxTypeCmd string

const (
	// GRMAll indicates any type of transaction should be returned.
	GRMAll GetRawMempoolTxTypeCmd = "all"

	// GRMRegular indicates only regular transactions should be returned.
	GRMRegular GetRawMempoolTxTypeCmd = "regular"

	// GRMTickets indicates that only tickets should be returned.
	GRMTickets GetRawMempoolTxTypeCmd = "tickets"

	// GRMVotes indicates that only votes should be returned.
	GRMVotes GetRawMempoolTxTypeCmd = "votes"

	// GRMRevocations indicates that only revocations should be returned.
	GRMRevocations GetRawMempoolTxTypeCmd = "revocations"
)

// GetRawMempoolCmd defines the getmempool JSON-RPC command.
type GetRawMempoolCmd struct {
	Verbose *bool `jsonrpcdefault:"false"`
	TxType  *string
}

// NewGetRawMempoolCmd returns a new instance which can be used to issue a
// getrawmempool JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetRawMempoolCmd(verbose *bool, txType *string) *GetRawMempoolCmd {
	return &GetRawMempoolCmd{
		Verbose: verbose,
		TxType:  txType,
	}
}

// GetRawTransactionCmd defines the getrawtransaction JSON-RPC command.
//
// NOTE: This field is an int versus a bool to remain compatible with Bitcoin
// Core even though it really should be a bool.
type GetRawTransactionCmd struct {
	Txid    string
	Verbose *int `jsonrpcdefault:"0"`
}

// NewGetRawTransactionCmd returns a new instance which can be used to issue a
// getrawtransaction JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetRawTransactionCmd(txHash string, verbose *int) *GetRawTransactionCmd {
	return &GetRawTransactionCmd{
		Txid:    txHash,
		Verbose: verbose,
	}
}

// GetStakeDifficultyCmd is a type handling custom marshaling and
// unmarshaling of getstakedifficulty JSON RPC commands.
type GetStakeDifficultyCmd struct{}

// NewGetStakeDifficultyCmd returns a new instance which can be used to
// issue a JSON-RPC getstakedifficulty command.
func NewGetStakeDifficultyCmd() *GetStakeDifficultyCmd {
	return &GetStakeDifficultyCmd{}
}

// GetStakeVersionInfoCmd returns stake version info for the current interval.
// Optionally, Count indicates how many additional intervals to return.
type GetStakeVersionInfoCmd struct {
	Count *int32
}

// NewGetStakeVersionInfoCmd returns a new instance which can be used to
// issue a JSON-RPC getstakeversioninfo command.
func NewGetStakeVersionInfoCmd(count int32) *GetStakeVersionInfoCmd {
	return &GetStakeVersionInfoCmd{
		Count: &count,
	}
}

// GetStakeVersionsCmd returns stake version for a range of blocks.
// Count indicates how many blocks are walked backwards.
type GetStakeVersionsCmd struct {
	Hash  string
	Count int32
}

// NewGetStakeVersionsCmd returns a new instance which can be used to
// issue a JSON-RPC getstakeversions command.
func NewGetStakeVersionsCmd(hash string, count int32) *GetStakeVersionsCmd {
	return &GetStakeVersionsCmd{
		Hash:  hash,
		Count: count,
	}
}

// GetTicketPoolValueCmd defines the getticketpoolvalue JSON-RPC command.
type GetTicketPoolValueCmd struct{}

// NewGetTicketPoolValueCmd returns a new instance which can be used to issue a
// getticketpoolvalue JSON-RPC command.
func NewGetTicketPoolValueCmd() *GetTicketPoolValueCmd {
	return &GetTicketPoolValueCmd{}
}

// GetTxOutCmd defines the gettxout JSON-RPC command.
type GetTxOutCmd struct {
	Txid           string
	Vout           uint32
	IncludeMempool *bool `jsonrpcdefault:"true"`
}

// NewGetTxOutCmd returns a new instance which can be used to issue a gettxout
// JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetTxOutCmd(txHash string, vout uint32, includeMempool *bool) *GetTxOutCmd {
	return &GetTxOutCmd{
		Txid:           txHash,
		Vout:           vout,
		IncludeMempool: includeMempool,
	}
}

// GetTxOutSetInfoCmd defines the gettxoutsetinfo JSON-RPC command.
type GetTxOutSetInfoCmd struct{}

// NewGetTxOutSetInfoCmd returns a new instance which can be used to issue a
// gettxoutsetinfo JSON-RPC command.
func NewGetTxOutSetInfoCmd() *GetTxOutSetInfoCmd {
	return &GetTxOutSetInfoCmd{}
}

// GetVoteInfoCmd returns voting results over a range of blocks.  Count
// indicates how many blocks are walked backwards.
type GetVoteInfoCmd struct {
	Version uint32
}

// NewGetVoteInfoCmd returns a new instance which can be used to
// issue a JSON-RPC getvoteinfo command.
func NewGetVoteInfoCmd(version uint32) *GetVoteInfoCmd {
	return &GetVoteInfoCmd{
		Version: version,
	}
}

// GetWorkCmd defines the getwork JSON-RPC command.
type GetWorkCmd struct {
	Data *string
}

// NewGetWorkCmd returns a new instance which can be used to issue a getwork
// JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetWorkCmd(data *string) *GetWorkCmd {
	return &GetWorkCmd{
		Data: data,
	}
}

// RegenTemplateCmd defines the regentemplate JSON-RPC command.
type RegenTemplateCmd struct{}

// NewRegenTemplateCmd returns a new instance which can be used to issue a
// regentemplate JSON-RPC command.
func NewRegenTemplateCmd() *RegenTemplateCmd {
	return &RegenTemplateCmd{}
}

// HelpCmd defines the help JSON-RPC command.
type HelpCmd struct {
	Command *string
}

// NewHelpCmd returns a new instance which can be used to issue a help JSON-RPC
// command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewHelpCmd(command *string) *HelpCmd {
	return &HelpCmd{
		Command: command,
	}
}

// LiveTicketsCmd is a type handling custom marshaling and
// unmarshaling of livetickets JSON RPC commands.
type LiveTicketsCmd struct{}

// NewLiveTicketsCmd returns a new instance which can be used to issue a JSON-RPC
// livetickets command.
func NewLiveTicketsCmd() *LiveTicketsCmd {
	return &LiveTicketsCmd{}
}

// MissedTicketsCmd is a type handling custom marshaling and
// unmarshaling of missedtickets JSON RPC commands.
type MissedTicketsCmd struct{}

// NewMissedTicketsCmd returns a new instance which can be used to issue a JSON-RPC
// missedtickets command.
func NewMissedTicketsCmd() *MissedTicketsCmd {
	return &MissedTicketsCmd{}
}

// NodeCmd defines the dropnode JSON-RPC command.
type NodeCmd struct {
	SubCmd        NodeSubCmd `jsonrpcusage:"\"connect|remove|disconnect\""`
	Target        string
	ConnectSubCmd *string `jsonrpcusage:"\"perm|temp\""`
}

// NewNodeCmd returns a new instance which can be used to issue a node
// JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewNodeCmd(subCmd NodeSubCmd, target string, connectSubCmd *string) *NodeCmd {
	return &NodeCmd{
		SubCmd:        subCmd,
		Target:        target,
		ConnectSubCmd: connectSubCmd,
	}
}

// PingCmd defines the ping JSON-RPC command.
type PingCmd struct{}

// NewPingCmd returns a new instance which can be used to issue a ping JSON-RPC
// command.
func NewPingCmd() *PingCmd {
	return &PingCmd{}
}

// RebroadcastMissedCmd is a type handling custom marshaling and
// unmarshaling of rebroadcastmissed JSON RPC commands.
type RebroadcastMissedCmd struct{}

// NewRebroadcastMissedCmd returns a new instance which can be used to
// issue a JSON-RPC rebroadcastmissed command.
func NewRebroadcastMissedCmd() *RebroadcastMissedCmd {
	return &RebroadcastMissedCmd{}
}

// RebroadcastWinnersCmd is a type handling custom marshaling and
// unmarshaling of rebroadcastwinners JSON RPC commands.
type RebroadcastWinnersCmd struct{}

// NewRebroadcastWinnersCmd returns a new instance which can be used to
// issue a JSON-RPC rebroadcastwinners command.
func NewRebroadcastWinnersCmd() *RebroadcastWinnersCmd {
	return &RebroadcastWinnersCmd{}
}

// SearchRawTransactionsCmd defines the searchrawtransactions JSON-RPC command.
type SearchRawTransactionsCmd struct {
	Address     string
	Verbose     *int  `jsonrpcdefault:"1"`
	Skip        *int  `jsonrpcdefault:"0"`
	Count       *int  `jsonrpcdefault:"100"`
	VinExtra    *int  `jsonrpcdefault:"0"`
	Reverse     *bool `jsonrpcdefault:"false"`
	FilterAddrs *[]string
}

// NewSearchRawTransactionsCmd returns a new instance which can be used to issue a
// searchrawtransactions JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSearchRawTransactionsCmd(address string, verbose, skip, count *int, vinExtra *int, reverse *bool, filterAddrs *[]string) *SearchRawTransactionsCmd {
	return &SearchRawTransactionsCmd{
		Address:     address,
		Verbose:     verbose,
		Skip:        skip,
		Count:       count,
		VinExtra:    vinExtra,
		Reverse:     reverse,
		FilterAddrs: filterAddrs,
	}
}

// SendRawTransactionCmd defines the sendrawtransaction JSON-RPC command.
type SendRawTransactionCmd struct {
	HexTx         string
	AllowHighFees *bool `jsonrpcdefault:"false"`
}

// NewSendRawTransactionCmd returns a new instance which can be used to issue a
// sendrawtransaction JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSendRawTransactionCmd(hexTx string, allowHighFees *bool) *SendRawTransactionCmd {
	return &SendRawTransactionCmd{
		HexTx:         hexTx,
		AllowHighFees: allowHighFees,
	}
}

// SetGenerateCmd defines the setgenerate JSON-RPC command.
type SetGenerateCmd struct {
	Generate     bool
	GenProcLimit *int `jsonrpcdefault:"-1"`
}

// NewSetGenerateCmd returns a new instance which can be used to issue a
// setgenerate JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSetGenerateCmd(generate bool, genProcLimit *int) *SetGenerateCmd {
	return &SetGenerateCmd{
		Generate:     generate,
		GenProcLimit: genProcLimit,
	}
}

// StopCmd defines the stop JSON-RPC command.
type StopCmd struct{}

// NewStopCmd returns a new instance which can be used to issue a stop JSON-RPC
// command.
func NewStopCmd() *StopCmd {
	return &StopCmd{}
}

// SubmitBlockOptions represents the optional options struct provided with a
// SubmitBlockCmd command.
type SubmitBlockOptions struct {
	// must be provided if server provided a workid with template.
	WorkID string `json:"workid,omitempty"`
}

// SubmitBlockCmd defines the submitblock JSON-RPC command.
type SubmitBlockCmd struct {
	HexBlock string
	Options  *SubmitBlockOptions
}

// NewSubmitBlockCmd returns a new instance which can be used to issue a
// submitblock JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSubmitBlockCmd(hexBlock string, options *SubmitBlockOptions) *SubmitBlockCmd {
	return &SubmitBlockCmd{
		HexBlock: hexBlock,
		Options:  options,
	}
}

// TicketFeeInfoCmd defines the ticketfeeinfo JSON-RPC command.
type TicketFeeInfoCmd struct {
	Blocks  *uint32
	Windows *uint32
}

// NewTicketFeeInfoCmd returns a new instance which can be used to issue a
// JSON-RPC ticket fee info command.
func NewTicketFeeInfoCmd(blocks *uint32, windows *uint32) *TicketFeeInfoCmd {
	return &TicketFeeInfoCmd{
		Blocks:  blocks,
		Windows: windows,
	}
}

// TicketsForAddressCmd defines the ticketsforaddress JSON-RPC command.
type TicketsForAddressCmd struct {
	Address string
}

// NewTicketsForAddressCmd returns a new instance which can be used to issue a
// JSON-RPC tickets for address command.
func NewTicketsForAddressCmd(addr string) *TicketsForAddressCmd {
	return &TicketsForAddressCmd{addr}
}

// TicketVWAPCmd defines the ticketvwap JSON-RPC command.
type TicketVWAPCmd struct {
	Start *uint32
	End   *uint32
}

// NewTicketVWAPCmd returns a new instance which can be used to issue a
// JSON-RPC ticket volume weight average price command.
func NewTicketVWAPCmd(start *uint32, end *uint32) *TicketVWAPCmd {
	return &TicketVWAPCmd{
		Start: start,
		End:   end,
	}
}

// TxFeeInfoCmd defines the txfeeinfo JSON-RPC command.
type TxFeeInfoCmd struct {
	Blocks     *uint32
	RangeStart *uint32
	RangeEnd   *uint32
}

// NewTxFeeInfoCmd returns a new instance which can be used to issue a
// JSON-RPC tx fee info command.
func NewTxFeeInfoCmd(blocks *uint32, start *uint32, end *uint32) *TxFeeInfoCmd {
	return &TxFeeInfoCmd{
		Blocks:     blocks,
		RangeStart: start,
		RangeEnd:   end,
	}
}

// ValidateAddressCmd defines the validateaddress JSON-RPC command.
type ValidateAddressCmd struct {
	Address string
}

// NewValidateAddressCmd returns a new instance which can be used to issue a
// validateaddress JSON-RPC command.
func NewValidateAddressCmd(address string) *ValidateAddressCmd {
	return &ValidateAddressCmd{
		Address: address,
	}
}

// VerifyChainCmd defines the verifychain JSON-RPC command.
type VerifyChainCmd struct {
	CheckLevel *int64 `jsonrpcdefault:"3"`
	CheckDepth *int64 `jsonrpcdefault:"288"` // 0 = all
}

// NewVerifyChainCmd returns a new instance which can be used to issue a
// verifychain JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewVerifyChainCmd(checkLevel, checkDepth *int64) *VerifyChainCmd {
	return &VerifyChainCmd{
		CheckLevel: checkLevel,
		CheckDepth: checkDepth,
	}
}

// VerifyMessageCmd defines the verifymessage JSON-RPC command.
type VerifyMessageCmd struct {
	Address   string
	Signature string
	Message   string
}

// NewVerifyMessageCmd returns a new instance which can be used to issue a
// verifymessage JSON-RPC command.
func NewVerifyMessageCmd(address, signature, message string) *VerifyMessageCmd {
	return &VerifyMessageCmd{
		Address:   address,
		Signature: signature,
		Message:   message,
	}
}

// VersionCmd defines the version JSON-RPC command.
type VersionCmd struct{}

// NewVersionCmd returns a new instance which can be used to issue a JSON-RPC
// version command.
func NewVersionCmd() *VersionCmd { return new(VersionCmd) }

func init() {
	// No special flags for commands in this file.
	flags := dcrjson.UsageFlag(0)

	dcrjson.MustRegister(Method("addnode"), (*AddNodeCmd)(nil), flags)
	dcrjson.MustRegister(Method("createrawssrtx"), (*CreateRawSSRtxCmd)(nil), flags)
	dcrjson.MustRegister(Method("createrawsstx"), (*CreateRawSStxCmd)(nil), flags)
	dcrjson.MustRegister(Method("createrawtransaction"), (*CreateRawTransactionCmd)(nil), flags)
	dcrjson.MustRegister(Method("debuglevel"), (*DebugLevelCmd)(nil), flags)
	dcrjson.MustRegister(Method("decoderawtransaction"), (*DecodeRawTransactionCmd)(nil), flags)
	dcrjson.MustRegister(Method("decodescript"), (*DecodeScriptCmd)(nil), flags)
	dcrjson.MustRegister(Method("estimatefee"), (*EstimateFeeCmd)(nil), flags)
	dcrjson.MustRegister(Method("estimatesmartfee"), (*EstimateSmartFeeCmd)(nil), flags)
	dcrjson.MustRegister(Method("estimatestakediff"), (*EstimateStakeDiffCmd)(nil), flags)
	dcrjson.MustRegister(Method("existsaddress"), (*ExistsAddressCmd)(nil), flags)
	dcrjson.MustRegister(Method("existsaddresses"), (*ExistsAddressesCmd)(nil), flags)
	dcrjson.MustRegister(Method("existsmissedtickets"), (*ExistsMissedTicketsCmd)(nil), flags)
	dcrjson.MustRegister(Method("existsexpiredtickets"), (*ExistsExpiredTicketsCmd)(nil), flags)
	dcrjson.MustRegister(Method("existsliveticket"), (*ExistsLiveTicketCmd)(nil), flags)
	dcrjson.MustRegister(Method("existslivetickets"), (*ExistsLiveTicketsCmd)(nil), flags)
	dcrjson.MustRegister(Method("existsmempooltxs"), (*ExistsMempoolTxsCmd)(nil), flags)
	dcrjson.MustRegister(Method("generate"), (*GenerateCmd)(nil), flags)
	dcrjson.MustRegister(Method("getaddednodeinfo"), (*GetAddedNodeInfoCmd)(nil), flags)
	dcrjson.MustRegister(Method("getbestblock"), (*GetBestBlockCmd)(nil), flags)
	dcrjson.MustRegister(Method("getbestblockhash"), (*GetBestBlockHashCmd)(nil), flags)
	dcrjson.MustRegister(Method("getblock"), (*GetBlockCmd)(nil), flags)
	dcrjson.MustRegister(Method("getblockchaininfo"), (*GetBlockChainInfoCmd)(nil), flags)
	dcrjson.MustRegister(Method("getblockcount"), (*GetBlockCountCmd)(nil), flags)
	dcrjson.MustRegister(Method("getblockhash"), (*GetBlockHashCmd)(nil), flags)
	dcrjson.MustRegister(Method("getblockheader"), (*GetBlockHeaderCmd)(nil), flags)
	dcrjson.MustRegister(Method("getblocksubsidy"), (*GetBlockSubsidyCmd)(nil), flags)
	dcrjson.MustRegister(Method("getcfilter"), (*GetCFilterCmd)(nil), flags)
	dcrjson.MustRegister(Method("getcfilterheader"), (*GetCFilterHeaderCmd)(nil), flags)
	dcrjson.MustRegister(Method("getcfilterv2"), (*GetCFilterV2Cmd)(nil), flags)
	dcrjson.MustRegister(Method("getchaintips"), (*GetChainTipsCmd)(nil), flags)
	dcrjson.MustRegister(Method("getcoinsupply"), (*GetCoinSupplyCmd)(nil), flags)
	dcrjson.MustRegister(Method("getconnectioncount"), (*GetConnectionCountCmd)(nil), flags)
	dcrjson.MustRegister(Method("getcurrentnet"), (*GetCurrentNetCmd)(nil), flags)
	dcrjson.MustRegister(Method("getdifficulty"), (*GetDifficultyCmd)(nil), flags)
	dcrjson.MustRegister(Method("getgenerate"), (*GetGenerateCmd)(nil), flags)
	dcrjson.MustRegister(Method("gethashespersec"), (*GetHashesPerSecCmd)(nil), flags)
	dcrjson.MustRegister(Method("getheaders"), (*GetHeadersCmd)(nil), flags)
	dcrjson.MustRegister(Method("getinfo"), (*GetInfoCmd)(nil), flags)
	dcrjson.MustRegister(Method("getmempoolinfo"), (*GetMempoolInfoCmd)(nil), flags)
	dcrjson.MustRegister(Method("getmininginfo"), (*GetMiningInfoCmd)(nil), flags)
	dcrjson.MustRegister(Method("getnetworkinfo"), (*GetNetworkInfoCmd)(nil), flags)
	dcrjson.MustRegister(Method("getnettotals"), (*GetNetTotalsCmd)(nil), flags)
	dcrjson.MustRegister(Method("getnetworkhashps"), (*GetNetworkHashPSCmd)(nil), flags)
	dcrjson.MustRegister(Method("getpeerinfo"), (*GetPeerInfoCmd)(nil), flags)
	dcrjson.MustRegister(Method("getrawmempool"), (*GetRawMempoolCmd)(nil), flags)
	dcrjson.MustRegister(Method("getrawtransaction"), (*GetRawTransactionCmd)(nil), flags)
	dcrjson.MustRegister(Method("getstakedifficulty"), (*GetStakeDifficultyCmd)(nil), flags)
	dcrjson.MustRegister(Method("getstakeversioninfo"), (*GetStakeVersionInfoCmd)(nil), flags)
	dcrjson.MustRegister(Method("getstakeversions"), (*GetStakeVersionsCmd)(nil), flags)
	dcrjson.MustRegister(Method("getticketpoolvalue"), (*GetTicketPoolValueCmd)(nil), flags)
	dcrjson.MustRegister(Method("gettxout"), (*GetTxOutCmd)(nil), flags)
	dcrjson.MustRegister(Method("gettxoutsetinfo"), (*GetTxOutSetInfoCmd)(nil), flags)
	dcrjson.MustRegister(Method("getvoteinfo"), (*GetVoteInfoCmd)(nil), flags)
	dcrjson.MustRegister(Method("getwork"), (*GetWorkCmd)(nil), flags)
	dcrjson.MustRegister(Method("help"), (*HelpCmd)(nil), flags)
	dcrjson.MustRegister(Method("livetickets"), (*LiveTicketsCmd)(nil), flags)
	dcrjson.MustRegister(Method("missedtickets"), (*MissedTicketsCmd)(nil), flags)
	dcrjson.MustRegister(Method("node"), (*NodeCmd)(nil), flags)
	dcrjson.MustRegister(Method("ping"), (*PingCmd)(nil), flags)
	dcrjson.MustRegister(Method("rebroadcastmissed"), (*RebroadcastMissedCmd)(nil), flags)
	dcrjson.MustRegister(Method("rebroadcastwinners"), (*RebroadcastWinnersCmd)(nil), flags)
	dcrjson.MustRegister(Method("regentemplate"), (*RegenTemplateCmd)(nil), flags)
	dcrjson.MustRegister(Method("searchrawtransactions"), (*SearchRawTransactionsCmd)(nil), flags)
	dcrjson.MustRegister(Method("sendrawtransaction"), (*SendRawTransactionCmd)(nil), flags)
	dcrjson.MustRegister(Method("setgenerate"), (*SetGenerateCmd)(nil), flags)
	dcrjson.MustRegister(Method("stop"), (*StopCmd)(nil), flags)
	dcrjson.MustRegister(Method("submitblock"), (*SubmitBlockCmd)(nil), flags)
	dcrjson.MustRegister(Method("ticketfeeinfo"), (*TicketFeeInfoCmd)(nil), flags)
	dcrjson.MustRegister(Method("ticketsforaddress"), (*TicketsForAddressCmd)(nil), flags)
	dcrjson.MustRegister(Method("ticketvwap"), (*TicketVWAPCmd)(nil), flags)
	dcrjson.MustRegister(Method("txfeeinfo"), (*TxFeeInfoCmd)(nil), flags)
	dcrjson.MustRegister(Method("validateaddress"), (*ValidateAddressCmd)(nil), flags)
	dcrjson.MustRegister(Method("verifychain"), (*VerifyChainCmd)(nil), flags)
	dcrjson.MustRegister(Method("verifymessage"), (*VerifyMessageCmd)(nil), flags)
	dcrjson.MustRegister(Method("version"), (*VersionCmd)(nil), flags)
}
