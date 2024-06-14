// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcserver

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	stdlog "log"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/crypto/rand"
	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/decred/dcrd/dcrjson/v4"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/internal/blockchain"
	"github.com/decred/dcrd/internal/mempool"
	"github.com/decred/dcrd/internal/mining"
	"github.com/decred/dcrd/internal/version"
	"github.com/decred/dcrd/mixing"
	"github.com/decred/dcrd/rpc/jsonrpc/types/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/txscript/v4/stdscript"
	"github.com/decred/dcrd/wire"
	"github.com/gorilla/websocket"
	"github.com/jrick/bitset"
)

// API version constants
const (
	jsonrpcSemverMajor = 8
	jsonrpcSemverMinor = 3
	jsonrpcSemverPatch = 0
)

const (
	// rpcAuthTimeoutSeconds is the number of seconds a connection to the
	// RPC server is allowed to stay open without authenticating before it
	// is closed.
	rpcAuthTimeoutSeconds = 10

	// rpcReadLimitAuthenticated is the maximum number of bytes allowed for a
	// JSON-RPC message read from a client.
	rpcReadLimitAuthenticated = 1 << 23 // 8 MiB

	// uint256Size is the number of bytes needed to represent an unsigned
	// 256-bit integer.
	uint256Size = 32

	// getworkDataLenBlake256 is the length of the data field of the getwork
	// RPC when providing work for blake256.  It consists of the serialized
	// block header plus the internal blake256 padding.  The internal blake256
	// padding consists of a single 1 bit followed by zeros and a final 1 bit in
	// order to pad the message out to 56 bytes followed by length of the
	// message in bits encoded as a big-endian uint64 (8 bytes).  Thus, the
	// resulting length is a multiple of the blake256 block size (64 bytes).
	// Given the padding requires at least a 1 bit and 64 bits for the padding,
	// the following converts the block header length and hash block size to
	// bits in order to ensure the correct number of hash blocks are calculated
	// and then multiplies the result by the block hash block size in bytes.
	getworkDataLenBlake256 = (1 + ((wire.MaxBlockHeaderPayload*8 + 65) /
		(blake256.BlockSize * 8))) * blake256.BlockSize

	// blake3BlkSize is the internal block size of the blake3 hashing algorithm.
	blake3BlkSize = 64

	// getworkDataLenBlake3 is the length of the data field of the getwork RPC
	// when providing work for blake3.  It consists of the serialized block
	// header plus the internal blake3 padding.  The internal blake3 padding
	// consists of enough zeros to pad the message out to a multiple of the
	// blake3 block size (64 bytes).
	getworkDataLenBlake3 = ((wire.MaxBlockHeaderPayload + (blake3BlkSize - 1)) /
		blake3BlkSize) * blake3BlkSize

	// getworkExpirationDiff is the number of blocks below the current
	// best block in height to begin pruning out old block work from
	// the template pool.
	getworkExpirationDiff = 3

	// sstxCommitmentString is the string to insert when a verbose
	// transaction output's pkscript type is a ticket commitment.
	sstxCommitmentString = "sstxcommitment"

	// merkleRootPairSize is the size in bytes of the merkle root + stake root
	// of a block.
	merkleRootPairSize = 64

	// syncWait is the maximum time in seconds to wait for an index
	// to sync with the main chain.
	syncWait = time.Second * 3
)

var (
	// jsonrpcSemverString is the RPC server's semantic API version formatted as
	// a string.
	jsonrpcSemverString = fmt.Sprintf("%d.%d.%d", jsonrpcSemverMajor,
		jsonrpcSemverMinor, jsonrpcSemverPatch)

	// blake256Pad is the extra blake256 internal padding needed for the
	// data of the getwork RPC.  It is set in the init routine since it is
	// based on the size of the block header and requires a bit of
	// calculation.
	blake256Pad []byte

	// blake3Pad is the extra blake3 internal padding needed for the data of the
	// getwork RPC.  It consists of all zeros.
	blake3Pad = make([]byte, getworkDataLenBlake3-wire.MaxBlockHeaderPayload)

	// JSON 2.0 batched request prefix
	batchedRequestPrefix = []byte("[")

	// zeroHash is the zero value for a chainhash.Hash and is defined as
	// a package level variable to avoid the need to create a new instance
	// every time a check is needed.
	zeroHash chainhash.Hash
)

// Errors
var (
	// ErrRPCUnimplemented is an error returned to RPC clients when the
	// provided command is recognized, but not implemented.
	ErrRPCUnimplemented = &dcrjson.RPCError{
		Code:    dcrjson.ErrRPCUnimplemented,
		Message: "Command unimplemented",
	}

	// ErrRPCNoWallet is an error returned to RPC clients when the provided
	// command is recognized as a wallet command.
	ErrRPCNoWallet = &dcrjson.RPCError{
		Code:    dcrjson.ErrRPCNoWallet,
		Message: "This implementation does not implement wallet commands",
	}
)

type commandHandler func(context.Context, *Server, interface{}) (interface{}, error)

// rpcHandlers maps RPC command strings to appropriate handler functions.
// This is set by init because help references rpcHandlers and thus causes
// a dependency loop.
var rpcHandlers map[types.Method]commandHandler
var rpcHandlersBeforeInit = map[types.Method]commandHandler{
	"addnode":               handleAddNode,
	"createrawsstx":         handleCreateRawSStx,
	"createrawssrtx":        handleCreateRawSSRtx,
	"createrawtransaction":  handleCreateRawTransaction,
	"debuglevel":            handleDebugLevel,
	"decoderawtransaction":  handleDecodeRawTransaction,
	"decodescript":          handleDecodeScript,
	"estimatefee":           handleEstimateFee,
	"estimatesmartfee":      handleEstimateSmartFee,
	"estimatestakediff":     handleEstimateStakeDiff,
	"existsaddress":         handleExistsAddress,
	"existsaddresses":       handleExistsAddresses,
	"existsliveticket":      handleExistsLiveTicket,
	"existslivetickets":     handleExistsLiveTickets,
	"existsmempooltxs":      handleExistsMempoolTxs,
	"generate":              handleGenerate,
	"getaddednodeinfo":      handleGetAddedNodeInfo,
	"getbestblock":          handleGetBestBlock,
	"getbestblockhash":      handleGetBestBlockHash,
	"getblock":              handleGetBlock,
	"getblockchaininfo":     handleGetBlockchainInfo,
	"getblockcount":         handleGetBlockCount,
	"getblockhash":          handleGetBlockHash,
	"getblockheader":        handleGetBlockHeader,
	"getblocksubsidy":       handleGetBlockSubsidy,
	"getcfilterv2":          handleGetCFilterV2,
	"getchaintips":          handleGetChainTips,
	"getcoinsupply":         handleGetCoinSupply,
	"getconnectioncount":    handleGetConnectionCount,
	"getcurrentnet":         handleGetCurrentNet,
	"getdifficulty":         handleGetDifficulty,
	"getgenerate":           handleGetGenerate,
	"gethashespersec":       handleGetHashesPerSec,
	"getheaders":            handleGetHeaders,
	"getinfo":               handleGetInfo,
	"getmempoolinfo":        handleGetMempoolInfo,
	"getmininginfo":         handleGetMiningInfo,
	"getmixmessage":         handleGetMixMessage,
	"getmixpairrequests":    handleGetMixPairRequests,
	"getnettotals":          handleGetNetTotals,
	"getnetworkhashps":      handleGetNetworkHashPS,
	"getnetworkinfo":        handleGetNetworkInfo,
	"getpeerinfo":           handleGetPeerInfo,
	"getrawmempool":         handleGetRawMempool,
	"getrawtransaction":     handleGetRawTransaction,
	"getstakedifficulty":    handleGetStakeDifficulty,
	"getstakeversioninfo":   handleGetStakeVersionInfo,
	"getstakeversions":      handleGetStakeVersions,
	"getticketpoolvalue":    handleGetTicketPoolValue,
	"gettreasurybalance":    handleGetTreasuryBalance,
	"gettreasuryspendvotes": handleGetTreasurySpendVotes,
	"getvoteinfo":           handleGetVoteInfo,
	"gettxout":              handleGetTxOut,
	"gettxoutsetinfo":       handleGetTxOutSetInfo,
	"getwork":               handleGetWork,
	"help":                  handleHelp,
	"invalidateblock":       handleInvalidateBlock,
	"livetickets":           handleLiveTickets,
	"node":                  handleNode,
	"ping":                  handlePing,
	"reconsiderblock":       handleReconsiderBlock,
	"regentemplate":         handleRegenTemplate,
	"sendrawmixmessage":     handleSendRawMixMessage,
	"sendrawtransaction":    handleSendRawTransaction,
	"setgenerate":           handleSetGenerate,
	"startprofiler":         handleStartProfiler,
	"stop":                  handleStop,
	"stopprofiler":          handleStopProfiler,
	"submitblock":           handleSubmitBlock,
	"ticketfeeinfo":         handleTicketFeeInfo,
	"ticketsforaddress":     handleTicketsForAddress,
	"ticketvwap":            handleTicketVWAP,
	"txfeeinfo":             handleTxFeeInfo,
	"validateaddress":       handleValidateAddress,
	"verifychain":           handleVerifyChain,
	"verifymessage":         handleVerifyMessage,
	"version":               handleVersion,
}

// list of commands that we recognize, but for which dcrd has no support because
// it lacks support for wallet functionality. For these commands the user
// should ask a connected instance of dcrwallet.
var rpcAskWallet = map[string]struct{}{
	"abandontransaction":        {},
	"accountaddressindex":       {},
	"accountsyncaddressindex":   {},
	"addmultisigaddress":        {},
	"addtransaction":            {},
	"addticket":                 {},
	"auditreuse":                {},
	"consolidate":               {},
	"createmultisig":            {},
	"createnewaccount":          {},
	"createsignature":           {},
	"createvotingaccount":       {},
	"discoverusage":             {},
	"dumpprivkey":               {},
	"fundrawtransaction":        {},
	"generatevote":              {},
	"getaccount":                {},
	"getaccountaddress":         {},
	"getaddressesbyaccount":     {},
	"getbalance":                {},
	"getcoinjoinsbyacct":        {},
	"getmasterpubkey":           {},
	"getmultisigoutinfo":        {},
	"getnewaddress":             {},
	"getrawchangeaddress":       {},
	"getreceivedbyaccount":      {},
	"getreceivedbyaddress":      {},
	"getstakeinfo":              {},
	"gettickets":                {},
	"gettransaction":            {},
	"getunconfirmedbalance":     {},
	"getvotechoices":            {},
	"getwalletfee":              {},
	"importcfiltersv2":          {},
	"importprivkey":             {},
	"importscript":              {},
	"importxpub":                {},
	"listaccounts":              {},
	"listaddresstransactions":   {},
	"listalltransactions":       {},
	"listlockunspent":           {},
	"listreceivedbyaccount":     {},
	"listreceivedbyaddress":     {},
	"listsinceblock":            {},
	"listtransactions":          {},
	"listunspent":               {},
	"lockunspent":               {},
	"mixoutput":                 {},
	"purchaseticket":            {},
	"redeemmultisigout":         {},
	"redeemmultisigouts":        {},
	"renameaccount":             {},
	"rescanwallet":              {},
	"revoketickets":             {},
	"sendfrom":                  {},
	"sendfromtreasury":          {},
	"sendmany":                  {},
	"sendtoaddress":             {},
	"sendtomultisig":            {},
	"sendtotreasury":            {},
	"setticketfee":              {},
	"settxfee":                  {},
	"setvotechoice":             {},
	"signmessage":               {},
	"signrawtransaction":        {},
	"signrawtransactions":       {},
	"stakepooluserinfo":         {},
	"sweepaccount":              {},
	"ticketinfo":                {},
	"ticketsforaddress":         {},
	"validatepredcp0005cf":      {},
	"verifymessage":             {},
	"walletinfo":                {},
	"walletislocked":            {},
	"walletlock":                {},
	"walletpassphrase":          {},
	"walletpassphrasechange":    {},
	"walletpubpassphrasechange": {},
}

// Commands that are currently unimplemented, but should ultimately be.
var rpcUnimplemented = map[string]struct{}{
	"estimatepriority": {},
}

// Commands that are available to a limited user
var rpcLimited = map[string]struct{}{
	// Websockets commands
	"notifyblocks":          {},
	"notifymixmessages":     {},
	"notifynewtransactions": {},
	"rescan":                {},
	"session":               {},
	"rebroadcastwinners":    {},

	// Websockets AND HTTP/S commands
	"help": {},

	// HTTP/S-only commands
	"createrawsstx":        {},
	"createrawssrtx":       {},
	"createrawtransaction": {},
	"decoderawtransaction": {},
	"decodescript":         {},
	"estimatefee":          {},
	"estimatesmartfee":     {},
	"estimatestakediff":    {},
	"existsaddress":        {},
	"existsaddresses":      {},
	"existsliveticket":     {},
	"existslivetickets":    {},
	"existsmempooltxs":     {},
	"getbestblock":         {},
	"getbestblockhash":     {},
	"getblock":             {},
	"getblockchaininfo":    {},
	"getblockcount":        {},
	"getblockhash":         {},
	"getblockheader":       {},
	"getblocksubsidy":      {},
	"getcfilterv2":         {},
	"getchaintips":         {},
	"getcoinsupply":        {},
	"getcurrentnet":        {},
	"getdifficulty":        {},
	"getheaders":           {},
	"getinfo":              {},
	"getmixmessage":        {},
	"getmixpairrequests":   {},
	"getnettotals":         {},
	"getnetworkhashps":     {},
	"getnetworkinfo":       {},
	"getrawmempool":        {},
	"getstakedifficulty":   {},
	"getstakeversioninfo":  {},
	"getstakeversions":     {},
	"getrawtransaction":    {},
	"gettreasurybalance":   {},
	"gettxout":             {},
	"getvoteinfo":          {},
	"livetickets":          {},
	"regentemplate":        {},
	"sendrawmixmessage":    {},
	"sendrawtransaction":   {},
	"submitblock":          {},
	"ticketfeeinfo":        {},
	"ticketsforaddress":    {},
	"ticketvwap":           {},
	"txfeeinfo":            {},
	"validateaddress":      {},
	"verifymessage":        {},
	"version":              {},
}

// rpcInternalErr is a convenience function to convert an internal error to an
// RPC error with the appropriate code set.  It also logs the error to the RPC
// server subsystem since internal errors really should not occur.  The context
// parameter is only used in the log message and may be empty if it's not
// needed.
func rpcInternalErr(err error, context string) *dcrjson.RPCError {
	errStr := err.Error()
	logStr := errStr
	if context != "" {
		logStr = context + ": " + errStr
	}
	log.Error(logStr)
	return dcrjson.NewRPCError(dcrjson.ErrRPCInternal.Code, errStr)
}

// rpcInvalidError is a convenience function to convert an invalid parameter
// error to an RPC error with the appropriate code set.
func rpcInvalidError(fmtStr string, args ...interface{}) *dcrjson.RPCError {
	return dcrjson.NewRPCError(dcrjson.ErrRPCInvalidParameter,
		fmt.Sprintf(fmtStr, args...))
}

// rpcDeserializationError is a convenience function to convert a
// deserialization error to an RPC error with the appropriate code set.
func rpcDeserializationError(fmtStr string, args ...interface{}) *dcrjson.RPCError {
	return dcrjson.NewRPCError(dcrjson.ErrRPCDeserialization,
		fmt.Sprintf(fmtStr, args...))
}

// rpcRuleError is a convenience function to convert a
// rule error to an RPC error with the appropriate code set.
func rpcRuleError(fmtStr string, args ...interface{}) *dcrjson.RPCError {
	return dcrjson.NewRPCError(dcrjson.ErrRPCMisc,
		fmt.Sprintf(fmtStr, args...))
}

// rpcDuplicateTxError is a convenience function to convert a
// rejected duplicate tx error to an RPC error with the appropriate code set.
func rpcDuplicateTxError(fmtStr string, args ...interface{}) *dcrjson.RPCError {
	return dcrjson.NewRPCError(dcrjson.ErrRPCDuplicateTx,
		fmt.Sprintf(fmtStr, args...))
}

// rpcAddressKeyError is a convenience function to convert an address/key error to
// an RPC error with the appropriate code set.  It also logs the error to the
// RPC server subsystem since internal errors really should not occur.  The
// context parameter is only used in the log message and may be empty if it's
// not needed.
func rpcAddressKeyError(fmtStr string, args ...interface{}) *dcrjson.RPCError {
	return dcrjson.NewRPCError(dcrjson.ErrRPCInvalidAddressOrKey,
		fmt.Sprintf(fmtStr, args...))
}

// rpcDecodeHexError is a convenience function for returning a nicely formatted
// RPC error which indicates the provided hex string failed to decode.
func rpcDecodeHexError(gotHex string) *dcrjson.RPCError {
	return dcrjson.NewRPCError(dcrjson.ErrRPCDecodeHexString,
		fmt.Sprintf("Argument must be hexadecimal string (not %q)",
			gotHex))
}

// rpcNoTxInfoError is a convenience function for returning a nicely formatted
// RPC error which indicates there is no information available for the provided
// transaction hash.
func rpcNoTxInfoError(txHash *chainhash.Hash) *dcrjson.RPCError {
	return dcrjson.NewRPCError(dcrjson.ErrRPCNoTxInfo,
		fmt.Sprintf("No information available about transaction %v",
			txHash))
}

// rpcBlockNotFoundError is a convenience function for returning a nicely
// formatted RPC error which indicates that the provided block was not found in
// the blockchain.
func rpcBlockNotFoundError(blockHash chainhash.Hash) *dcrjson.RPCError {
	return dcrjson.NewRPCError(dcrjson.ErrRPCBlockNotFound,
		fmt.Sprintf("No information available about block %v",
			blockHash))
}

// rpcMixMessageNotFoundError is a convenience function for returning a nicely
// formatted RPC error which indicates that the mix message was not found in
// the mixpool.
func rpcMixMessageNotFoundError(hash *chainhash.Hash) *dcrjson.RPCError {
	return dcrjson.NewRPCError(dcrjson.ErrRPCNoMixMsgInfo,
		fmt.Sprintf("No information available about mix message %v",
			hash))
}

// rpcConnectionClosedError is a convenience function for returning an RPC error
// which indicates the associated connection has been closed, most likely due to
// context cancellation such as when the server is being shutdown.
func rpcConnectionClosedError() *dcrjson.RPCError {
	return dcrjson.NewRPCError(dcrjson.ErrRPCMisc, "Connection closed")
}

// rpcMiscError is a convenience function for returning a nicely formatted RPC
// error which indicates there is an unquantifiable error.  Use this sparingly;
// misc return codes are a cop out.
func rpcMiscError(message string) *dcrjson.RPCError {
	return dcrjson.NewRPCError(dcrjson.ErrRPCMisc, message)
}

// directionString is a helper function that returns a string that represents
// the direction of a connection (inbound or outbound).
func directionString(inbound bool) string {
	if inbound {
		return "inbound"
	}
	return "outbound"
}

// normalizeAddress returns addr with the passed default port appended if
// there is not already a port specified.
//
// If the host portion of the address is an interface name, the first IP address
// associated with the interface is used.  Otherwise, the name is interpreted as
// a hostname.
func normalizeAddress(addr, defaultPort string) string {
	port := defaultPort
	if a, p, err := net.SplitHostPort(addr); err == nil {
		addr = a
		port = p
	}
	iface, _ := net.InterfaceByName(addr)
	if iface == nil {
		return net.JoinHostPort(addr, port)
	}
	ifaceAddrs, err := iface.Addrs()
	if err != nil {
		return net.JoinHostPort(addr, port)
	}
	for _, a := range ifaceAddrs {
		if a, ok := a.(*net.IPNet); ok {
			dialAddr := a.IP.String()
			if a.IP.To4() == nil { // IPv6
				zoned := a.IP.IsLinkLocalUnicast() ||
					a.IP.IsLinkLocalMulticast()
				if zoned {
					dialAddr += "%" + strconv.Itoa(iface.Index)
				}
			}
			return net.JoinHostPort(dialAddr, port)
		}
	}

	return net.JoinHostPort(addr, port)
}

// workState houses state that is used in between multiple RPC invocations to
// getwork.
type workState struct {
	// workSem is a semaphore used to limit multiple RPC invocations for work
	// requests and submission.  This approach is used over a mutex to support
	// early cancellation.
	workSem semaphore

	// These fields are all protected by the embedded mutex.
	//
	// prevBestHash houses the previous best known chain tip.
	//
	// waitForUpdatedTemplate determines whether or not a getwork invocation
	// should block until a new template is received from the background
	// template generator.
	//
	// templatePool houses unique block templates that have been returned to
	// clients so the full block can be reconstructed from it upon successful
	// submission of solved work.  The templates are pruned when the get old
	// enough.
	sync.Mutex
	prevBestHash           *chainhash.Hash
	waitForUpdatedTemplate bool
	templatePool           map[[merkleRootPairSize]byte]*wire.MsgBlock
}

// newWorkState returns a new instance of a workState with all internal fields
// initialized and ready to use.
func newWorkState() *workState {
	return &workState{
		workSem:      makeSemaphore(1),
		templatePool: make(map[[merkleRootPairSize]byte]*wire.MsgBlock),
	}
}

// handleAddNode handles addnode commands.
func handleAddNode(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.AddNodeCmd)

	addr := normalizeAddress(c.Addr, s.cfg.ChainParams.DefaultPort)
	connMgr := s.cfg.ConnMgr
	var err error
	switch c.SubCmd {
	case "add":
		err = connMgr.Connect(addr, true)
	case "remove":
		err = connMgr.RemoveByAddr(addr)
	case "onetry":
		err = connMgr.Connect(addr, false)
	default:
		return nil, rpcInvalidError("Invalid subcommand for addnode")
	}

	if err != nil {
		return nil, rpcInvalidError("%v: %v", c.SubCmd, err)
	}

	// no data returned unless an error.
	return nil, nil
}

// handleNode handles node commands.
func handleNode(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.NodeCmd)

	connMgr := s.cfg.ConnMgr

	var addr string
	var nodeID uint64
	var errN, err error
	params := s.cfg.ChainParams
	switch c.SubCmd {
	case "disconnect":
		// If we have a valid uint disconnect by node id. Otherwise,
		// attempt to disconnect by address, returning an error if a
		// valid IP address is not supplied.
		if nodeID, errN = strconv.ParseUint(c.Target, 10, 32); errN == nil {
			err = connMgr.DisconnectByID(int32(nodeID))
		} else {
			if _, _, errP := net.SplitHostPort(c.Target); errP == nil ||
				net.ParseIP(c.Target) != nil {
				addr = normalizeAddress(c.Target, params.DefaultPort)
				err = connMgr.DisconnectByAddr(addr)
			} else {
				return nil, rpcInvalidError("%v: Invalid "+
					"address or node ID", c.SubCmd)
			}
		}
		if err != nil && peerExists(connMgr, addr, int32(nodeID)) {
			return nil, rpcMiscError("can't disconnect a permanent peer, " +
				"use remove")
		}

	case "remove":
		// If we have a valid uint disconnect by node id. Otherwise,
		// attempt to disconnect by address, returning an error if a
		// valid IP address is not supplied.
		if nodeID, errN = strconv.ParseUint(c.Target, 10, 32); errN == nil {
			err = connMgr.RemoveByID(int32(nodeID))
		} else {
			if _, _, errP := net.SplitHostPort(c.Target); errP == nil ||
				net.ParseIP(c.Target) != nil {
				addr = normalizeAddress(c.Target, params.DefaultPort)
				err = connMgr.RemoveByAddr(addr)
			} else {
				return nil, rpcInvalidError("%v: invalid "+
					"address or node ID", c.SubCmd)
			}
		}
		if err != nil && peerExists(connMgr, addr, int32(nodeID)) {
			return nil, rpcMiscError("can't remove a temporary peer, " +
				"use disconnect")
		}

	case "connect":
		addr = normalizeAddress(c.Target, params.DefaultPort)

		// Default to temporary connections.
		subCmd := "temp"
		if c.ConnectSubCmd != nil {
			subCmd = *c.ConnectSubCmd
		}

		switch subCmd {
		case "perm", "temp":
			err = connMgr.Connect(addr, subCmd == "perm")
		default:
			return nil, rpcInvalidError("%v: invalid subcommand "+
				"for node connect", subCmd)
		}
	default:
		return nil, rpcInvalidError("%v: invalid subcommand for node", c.SubCmd)
	}

	if err != nil {
		return nil, rpcInvalidError("%v: %v", c.SubCmd, err)
	}

	// no data returned unless an error.
	return nil, nil
}

// peerExists determines if a certain peer is currently connected given
// information about all currently connected peers. Peer existence is
// determined using either a target address or node id.
func peerExists(connMgr ConnManager, addr string, nodeID int32) bool {
	for _, p := range connMgr.ConnectedPeers() {
		if p.ID() == nodeID || p.Addr() == addr {
			return true
		}
	}
	return false
}

// messageToHex serializes a message to the wire protocol encoding using the
// latest protocol version and returns a hex-encoded string of the result.
func (s *Server) messageToHex(msg wire.Message) (string, error) {
	var buf bytes.Buffer
	if err := msg.BtcEncode(&buf, s.cfg.MaxProtocolVersion); err != nil {
		context := fmt.Sprintf("Failed to encode msg of type %T", msg)
		return "", rpcInternalErr(err, context)
	}

	return hex.EncodeToString(buf.Bytes()), nil
}

// newTxOut returns a new transaction output with the given parameters.
func newTxOut(amount int64, pkScriptVer uint16, pkScript []byte) *wire.TxOut {
	return &wire.TxOut{
		Value:    amount,
		Version:  pkScriptVer,
		PkScript: pkScript,
	}
}

// handleCreateRawTransaction handles createrawtransaction commands.
func handleCreateRawTransaction(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.CreateRawTransactionCmd)

	// Validate expiry, if given.
	if c.Expiry != nil && *c.Expiry < 0 {
		return nil, rpcInvalidError("Expiry out of range")
	}

	// Validate the locktime, if given.
	if c.LockTime != nil &&
		(*c.LockTime < 0 ||
			*c.LockTime > int64(wire.MaxTxInSequenceNum)) {
		return nil, rpcInvalidError("Locktime out of range")
	}

	// Add all transaction inputs to a new transaction after performing
	// some validity checks.
	mtx := wire.NewMsgTx()
	for _, input := range c.Inputs {
		txHash, err := chainhash.NewHashFromStr(input.Txid)
		if err != nil {
			return nil, rpcDecodeHexError(input.Txid)
		}

		if !(input.Tree == wire.TxTreeRegular ||
			input.Tree == wire.TxTreeStake) {
			return nil, rpcInvalidError("Tx tree must be regular " +
				"or stake")
		}

		prevOutV := wire.NullValueIn
		if input.Amount > 0 {
			amt, err := dcrutil.NewAmount(input.Amount)
			if err != nil {
				return nil, rpcInvalidError(err.Error())
			}
			prevOutV = int64(amt)
		}

		prevOut := wire.NewOutPoint(txHash, input.Vout, input.Tree)
		txIn := wire.NewTxIn(prevOut, prevOutV, []byte{})
		if c.LockTime != nil && *c.LockTime != 0 {
			txIn.Sequence = wire.MaxTxInSequenceNum - 1
		}
		mtx.AddTxIn(txIn)
	}

	// Add all transaction outputs to the transaction after performing
	// some validity checks.
	for encodedAddr, amount := range c.Amounts {
		atoms, err := dcrutil.NewAmount(amount)
		if err != nil {
			return nil, rpcInternalErr(err, "New amount")
		}

		// Ensure amount is in the valid range for monetary amounts.
		if atoms <= 0 || atoms > dcrutil.MaxAmount {
			return nil, rpcInvalidError("Invalid amount: 0 >= %v "+
				"> %v", amount, dcrutil.MaxAmount)
		}

		// Decode the provided address.  This also ensures the network encoded
		// with the address matches the network the server is currently on.
		addr, err := stdaddr.DecodeAddress(encodedAddr, s.cfg.ChainParams)
		if err != nil {
			return nil, rpcAddressKeyError("Could not decode address: %v", err)
		}

		// Ensure the address is one of the supported types.
		if _, ok := addr.(stdaddr.StakeAddress); !ok {
			return nil, rpcAddressKeyError("Invalid type: %T", addr)
		}

		// Create a new script which pays to the provided address.
		pkScriptVer, pkScript := addr.PaymentScript()
		mtx.AddTxOut(newTxOut(int64(atoms), pkScriptVer, pkScript))
	}

	// Set the Locktime, if given.
	if c.LockTime != nil {
		mtx.LockTime = uint32(*c.LockTime)
	}

	// Set the Expiry, if given.
	if c.Expiry != nil {
		mtx.Expiry = uint32(*c.Expiry)
	}

	// Return the serialized and hex-encoded transaction.  Note that this
	// is intentionally not directly returning because the first return
	// value is a string and it would result in returning an empty string to
	// the client instead of nothing (nil) in the case of an error.
	mtxHex, err := s.messageToHex(mtx)
	if err != nil {
		return nil, err
	}
	return mtxHex, nil
}

// handleCreateRawSStx handles createrawsstx commands.
func handleCreateRawSStx(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.CreateRawSStxCmd)

	// Basic sanity checks for the information coming from the cmd.
	if len(c.Inputs) != len(c.COuts) {
		return nil, rpcInvalidError("Number of inputs should be equal "+
			"to the number of future commitment/change outs for "+
			"any sstx; %v inputs given, but %v COuts",
			len(c.Inputs), len(c.COuts))
	}
	if len(c.Amount) != 1 {
		return nil, rpcInvalidError("Only one SSGen tagged output is "+
			"allowed per sstx; len ssgenout %v", len(c.Amount))
	}

	// Add all transaction inputs to a new transaction after performing
	// some validity checks.
	mtx := wire.NewMsgTx()
	for _, input := range c.Inputs {
		txHash, err := chainhash.NewHashFromStr(input.Txid)
		if err != nil {
			return nil, rpcDecodeHexError(input.Txid)
		}

		if !(input.Tree == wire.TxTreeRegular ||
			input.Tree == wire.TxTreeStake) {
			return nil, rpcInvalidError("Tx tree must be regular or stake")
		}

		prevOut := wire.NewOutPoint(txHash, input.Vout, input.Tree)
		txIn := wire.NewTxIn(prevOut, input.Amt, nil)
		mtx.AddTxIn(txIn)
	}

	// Add all transaction outputs to the transaction after performing
	// some validity checks.
	amtTicket := int64(0)

	for encodedAddr, amount := range c.Amount {
		// Ensure amount is in the valid range for monetary amounts.
		if amount <= 0 || amount > dcrutil.MaxAmount {
			return nil, rpcInvalidError("Invalid SSTx commitment "+
				"amount: 0 >= %v > %v", amount,
				dcrutil.MaxAmount)
		}

		// Decode the provided address.  This also ensures the network encoded
		// with the address matches the network the server is currently on.
		addr, err := stdaddr.DecodeAddress(encodedAddr, s.cfg.ChainParams)
		if err != nil {
			return nil, rpcAddressKeyError("Could not decode address: %v", err)
		}

		// Ensure the address is one of the supported types.
		stakeAddr, ok := addr.(stdaddr.StakeAddress)
		if !ok {
			return nil, rpcAddressKeyError("Invalid address type: %T", addr)
		}

		// Create the necessary voting rights script.
		pkScriptVer, pkScript := stakeAddr.VotingRightsScript()
		mtx.AddTxOut(newTxOut(amount, pkScriptVer, pkScript))

		amtTicket += amount
	}

	// Calculated the commitment amounts, then create the
	// addresses and payout proportions as null data
	// outputs.
	inputAmts := make([]int64, len(c.Inputs))
	for i, input := range c.Inputs {
		inputAmts[i] = input.Amt
	}
	changeAmts := make([]int64, len(c.COuts))
	for i, cout := range c.COuts {
		changeAmts[i] = cout.ChangeAmt
	}

	// Check and make sure none of the change overflows
	// the input amounts.
	for i, amt := range inputAmts {
		if changeAmts[i] >= amt {
			return nil, rpcInvalidError("input %v >= amount %v",
				changeAmts[i], amt)
		}
	}

	// Obtain the commitment amounts.
	_, amountsCommitted, err := stake.SStxNullOutputAmounts(inputAmts,
		changeAmts, amtTicket)
	if err != nil {
		return nil, rpcInternalErr(err, "Invalid SSTx output amounts")
	}

	for i, cout := range c.COuts {
		// Append future commitment output.  This also ensures the network
		// encoded with the address matches the network the server is currently
		// on.
		addr, err := stdaddr.DecodeAddress(cout.Addr, s.cfg.ChainParams)
		if err != nil {
			return nil, rpcAddressKeyError("Could not decode address: %v", err)
		}

		// Ensure the address is one of the supported types.
		stakeAddr, ok := addr.(stdaddr.StakeAddress)
		if !ok {
			return nil, rpcAddressKeyError("Invalid type: %T", addr)
		}

		// Create the reward commitment script.
		//
		// TODO: Allow fee limits to be specified with an argument.
		const voteFeeLimit = 0
		const revokeFeeLimit = 0
		cmtScriptVer, cmtScript := stakeAddr.RewardCommitmentScript(
			amountsCommitted[i], voteFeeLimit, revokeFeeLimit)
		mtx.AddTxOut(newTxOut(0, cmtScriptVer, cmtScript))

		// 2. Append change output.

		// Ensure amount is in the valid range for monetary amounts.
		if cout.ChangeAmt < 0 || cout.ChangeAmt > dcrutil.MaxAmount {
			return nil, rpcInvalidError("Invalid change amount: 0 "+
				"> %v > %v", cout.ChangeAmt, dcrutil.MaxAmount)
		}

		// Decode the provided address.  This also ensures the network encoded
		// with the address matches the network the server is currently on.
		addr, err = stdaddr.DecodeAddress(cout.ChangeAddr, s.cfg.ChainParams)
		if err != nil {
			return nil, rpcAddressKeyError("Wrong network: %v",
				addr)
		}

		// Ensure the address is one of the supported types.
		stakeAddr, ok = addr.(stdaddr.StakeAddress)
		if !ok {
			return nil, rpcAddressKeyError("Invalid type: %T", addr)
		}

		// Create a new script which pays change to the provided address.
		changeScriptVer, changeScript := stakeAddr.StakeChangeScript()
		mtx.AddTxOut(newTxOut(cout.ChangeAmt, changeScriptVer, changeScript))
	}

	// Make sure we generated a valid SStx.
	if err := stake.CheckSStx(mtx); err != nil {
		return nil, rpcInternalErr(err, "Invalid SStx")
	}

	// Return the serialized and hex-encoded transaction.
	mtxHex, err := s.messageToHex(mtx)
	if err != nil {
		return nil, err
	}
	return mtxHex, nil
}

// handleCreateRawSSRtx handles createrawssrtx commands.
func handleCreateRawSSRtx(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.CreateRawSSRtxCmd)

	// Only a single SStx should be given
	if len(c.Inputs) != 1 {
		return nil, rpcInvalidError("SSRtx invalid number of inputs")
	}

	// The input must be in the stake tree.
	input := c.Inputs[0]
	if input.Tree != wire.TxTreeStake {
		return nil, rpcInvalidError("Input tree is not TxTreeStake type")
	}

	// The input must be a ticket submission output.
	const ticketSubmissionOutput = 0
	if input.Vout != ticketSubmissionOutput {
		return nil, rpcInvalidError("Input is not a ticket submission output " +
			"(output index 0)")
	}

	// 1. Fetch the SStx, then calculate all the values we'll need later
	// for the generation of the SSRtx tx outputs.
	//
	// Convert the provided transaction hash hex to a chainhash.Hash.
	txHash, err := chainhash.NewHashFromStr(input.Txid)
	if err != nil {
		return nil, rpcDecodeHexError(input.Txid)
	}

	// Try to fetch the ticket from the block database.
	outpoint := wire.OutPoint{Hash: *txHash, Index: input.Vout, Tree: input.Tree}
	ticketUtxo, err := s.cfg.Chain.FetchUtxoEntry(outpoint)
	if ticketUtxo == nil || err != nil {
		return nil, rpcNoTxInfoError(txHash)
	}
	if t := ticketUtxo.TransactionType(); t != stake.TxTypeSStx {
		return nil, rpcDeserializationError("Invalid Tx type: %v", t)
	}

	// Store the sstx pubkeyhashes and amounts as found in the transaction
	// outputs.
	minimalOutputs := ticketUtxo.TicketMinimalOutputs()
	if minimalOutputs == nil {
		err := errors.New("missing ticket minimal outputs")
		return nil, rpcInternalErr(err, "")
	}

	// The input amount must be the ticket submission amount.
	ticketSubmission := minimalOutputs[ticketSubmissionOutput]
	ticketSubmissionAmount := dcrutil.Amount(ticketSubmission.Value)
	inputAmount, err := dcrutil.NewAmount(input.Amount)
	if err != nil {
		return nil, rpcInvalidError(err.Error())
	}
	if inputAmount != ticketSubmissionAmount {
		return nil, rpcInvalidError("Input amount %v is not equal to ticket "+
			"submission amount %v", inputAmount, ticketSubmissionAmount)
	}

	// Decode the fee as coins.
	var feeAmt dcrutil.Amount
	if c.Fee != nil {
		var err error
		feeAmt, err = dcrutil.NewAmount(*c.Fee)
		if err != nil {
			return nil, rpcInvalidError("Invalid fee amount: %v", err)
		}
	}

	// Determine if the automatic ticket revocations agenda is active.
	prevBlkHash := s.cfg.Chain.BestSnapshot().Hash
	isAutoRevocationsEnabled, err := s.isAutoRevocationsAgendaActive(&prevBlkHash)
	if err != nil {
		return nil, err
	}

	// If the automatic ticket revocations agenda is active, validate that the fee
	// amount is zero and set the transaction version to 2.
	revocationTxVersion := uint16(1)
	if isAutoRevocationsEnabled {
		if feeAmt != 0 {
			return nil, rpcInvalidError("Fee amount must be 0 when the automatic " +
				"ticket revocations agenda is active")
		}
		revocationTxVersion = stake.TxVersionAutoRevocations
	}

	// Get the previous header bytes.
	prevHeader, err := s.cfg.Chain.HeaderByHash(&prevBlkHash)
	if err != nil {
		return nil, rpcBlockNotFoundError(prevBlkHash)
	}
	prevHeaderBytes, err := prevHeader.Bytes()
	if err != nil {
		str := fmt.Sprintf("Failed to serialize header for block %v", prevBlkHash)
		return nil, rpcInternalErr(err, str)
	}

	mtx, err := stake.CreateRevocationFromTicket(txHash, minimalOutputs, feeAmt,
		revocationTxVersion, s.cfg.ChainParams, prevHeaderBytes,
		isAutoRevocationsEnabled)
	if err != nil {
		return nil, rpcInvalidError(err.Error(), "Invalid SSRtx")
	}

	// Check to make sure our SSRtx was created correctly.
	err = stake.CheckSSRtx(mtx)
	if err != nil {
		return nil, rpcInternalErr(err, "Invalid SSRtx")
	}

	// Return the serialized and hex-encoded transaction.
	mtxHex, err := s.messageToHex(mtx)
	if err != nil {
		return nil, err
	}
	return mtxHex, nil
}

// handleDebugLevel handles debuglevel commands.
func handleDebugLevel(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.DebugLevelCmd)

	// Special show command to list supported subsystems.
	if c.LevelSpec == "show" {
		return fmt.Sprintf("Supported subsystems %v",
			s.cfg.LogManager.SupportedSubsystems()), nil
	}

	err := s.cfg.LogManager.ParseAndSetDebugLevels(c.LevelSpec)
	if err != nil {
		return nil, rpcInvalidError("Invalid debug level %v: %v",
			c.LevelSpec, err)
	}

	return "Done.", nil
}

// createVinList returns a slice of JSON objects for the inputs of the passed
// transaction.
func createVinList(mtx *wire.MsgTx, isTreasuryEnabled bool) []types.Vin {
	// Treasurybase transactions only have a single txin by definition.
	//
	// NOTE: This check MUST come before the coinbase check because a
	// treasurybase will be identified as a coinbase as well.
	vinList := make([]types.Vin, len(mtx.TxIn))
	if isTreasuryEnabled && standalone.IsTreasuryBase(mtx) {
		txIn := mtx.TxIn[0]
		vinEntry := &vinList[0]
		vinEntry.Treasurybase = true
		vinEntry.Sequence = txIn.Sequence
		vinEntry.AmountIn = dcrutil.Amount(txIn.ValueIn).ToCoin()
		vinEntry.BlockHeight = txIn.BlockHeight
		vinEntry.BlockIndex = txIn.BlockIndex
		return vinList
	}

	// Coinbase transactions only have a single txin by definition.
	if standalone.IsCoinBaseTx(mtx, isTreasuryEnabled) {
		txIn := mtx.TxIn[0]
		vinEntry := &vinList[0]
		vinEntry.Coinbase = hex.EncodeToString(txIn.SignatureScript)
		vinEntry.Sequence = txIn.Sequence
		vinEntry.AmountIn = dcrutil.Amount(txIn.ValueIn).ToCoin()
		vinEntry.BlockHeight = txIn.BlockHeight
		vinEntry.BlockIndex = txIn.BlockIndex
		return vinList
	}

	// Treasury spend transactions only have a single txin by definition.
	if isTreasuryEnabled && stake.IsTSpend(mtx) {
		txIn := mtx.TxIn[0]
		vinEntry := &vinList[0]
		vinEntry.TreasurySpend = hex.EncodeToString(txIn.SignatureScript)
		vinEntry.Sequence = txIn.Sequence
		vinEntry.AmountIn = dcrutil.Amount(txIn.ValueIn).ToCoin()
		vinEntry.BlockHeight = txIn.BlockHeight
		vinEntry.BlockIndex = txIn.BlockIndex
		return vinList
	}

	// Stakebase transactions (votes) have two inputs: a null stake base
	// followed by an input consuming a ticket's stakesubmission.
	isSSGen := stake.IsSSGen(mtx)

	for i, txIn := range mtx.TxIn {
		// Handle only the null input of a stakebase differently.
		if isSSGen && i == 0 {
			vinEntry := &vinList[0]
			vinEntry.Stakebase = hex.EncodeToString(txIn.SignatureScript)
			vinEntry.Sequence = txIn.Sequence
			vinEntry.AmountIn = dcrutil.Amount(txIn.ValueIn).ToCoin()
			vinEntry.BlockHeight = txIn.BlockHeight
			vinEntry.BlockIndex = txIn.BlockIndex
			continue
		}

		// The disassembled string will contain [error] inline
		// if the script doesn't fully parse, so ignore the
		// error here.
		disbuf, _ := txscript.DisasmString(txIn.SignatureScript)

		vinEntry := &vinList[i]
		vinEntry.Txid = txIn.PreviousOutPoint.Hash.String()
		vinEntry.Vout = txIn.PreviousOutPoint.Index
		vinEntry.Tree = txIn.PreviousOutPoint.Tree
		vinEntry.Sequence = txIn.Sequence
		vinEntry.AmountIn = dcrutil.Amount(txIn.ValueIn).ToCoin()
		vinEntry.BlockHeight = txIn.BlockHeight
		vinEntry.BlockIndex = txIn.BlockIndex
		vinEntry.ScriptSig = &types.ScriptSig{
			Asm: disbuf,
			Hex: hex.EncodeToString(txIn.SignatureScript),
		}
	}

	return vinList
}

// createVoutList returns a slice of JSON objects for the outputs of the passed
// transaction.
func createVoutList(mtx *wire.MsgTx, chainParams *chaincfg.Params,
	filterAddrMap map[string]struct{}) []types.Vout {

	txType := stake.DetermineTxType(mtx)
	voutList := make([]types.Vout, 0, len(mtx.TxOut))
	for i, v := range mtx.TxOut {
		// The disassembled string will contain [error] inline if the
		// script doesn't fully parse, so ignore the error here.
		disbuf, _ := txscript.DisasmString(v.PkScript)

		// Attempt to extract addresses from the public key script.  In
		// the case of stake submission transactions, the odd outputs
		// contain a commitment address, so detect that case
		// accordingly.
		var addrs []stdaddr.Address
		var scriptType string
		var reqSigs uint16
		var commitAmt *dcrutil.Amount
		if txType == stake.TxTypeSStx && (i%2 != 0) {
			scriptType = sstxCommitmentString
			addr, err := stake.AddrFromSStxPkScrCommitment(v.PkScript,
				chainParams)
			if err != nil {
				log.Warnf("failed to decode ticket "+
					"commitment addr output for tx hash "+
					"%v, output idx %v", mtx.TxHash(), i)
			} else {
				addrs = []stdaddr.Address{addr}
			}
			amt, err := stake.AmountFromSStxPkScrCommitment(v.PkScript)
			if err != nil {
				log.Warnf("failed to decode ticket "+
					"commitment amt output for tx hash %v"+
					", output idx %v", mtx.TxHash(), i)
			} else {
				commitAmt = &amt
			}
		} else {
			// Attempt to extract known addresses associated with the script.
			var st stdscript.ScriptType
			st, addrs = stdscript.ExtractAddrs(v.Version, v.PkScript, chainParams)
			scriptType = st.String()

			// Determine the number of required signatures for known standard
			// types.
			reqSigs = stdscript.DetermineRequiredSigs(v.Version, v.PkScript)
		}

		// Encode the addresses while checking if the address passes the
		// filter when needed.
		passesFilter := len(filterAddrMap) == 0
		encodedAddrs := make([]string, len(addrs))
		for j, addr := range addrs {
			encodedAddr := addr.String()
			encodedAddrs[j] = encodedAddr

			// No need to check the map again if the filter already
			// passes.
			if passesFilter {
				continue
			}
			if _, exists := filterAddrMap[encodedAddr]; exists {
				passesFilter = true
			}
		}

		if !passesFilter {
			continue
		}

		var vout types.Vout
		voutSPK := &vout.ScriptPubKey
		vout.N = uint32(i)
		vout.Value = dcrutil.Amount(v.Value).ToCoin()
		vout.Version = v.Version
		voutSPK.Addresses = encodedAddrs
		voutSPK.Asm = disbuf
		voutSPK.Hex = hex.EncodeToString(v.PkScript)
		voutSPK.Type = scriptType
		voutSPK.ReqSigs = int32(reqSigs)
		if commitAmt != nil {
			voutSPK.CommitAmt = dcrjson.Float64(commitAmt.ToCoin())
		}
		voutSPK.Version = v.Version

		voutList = append(voutList, vout)
	}

	return voutList
}

// createTxRawResult converts the passed transaction and associated parameters
// to a raw transaction JSON object.
func (s *Server) createTxRawResult(chainParams *chaincfg.Params,
	mtx *wire.MsgTx, txHash string, blkIdx uint32, blkHeader *wire.BlockHeader,
	blkHash string, blkHeight int64, confirmations int64,
	isTreasuryEnabled bool) (*types.TxRawResult, error) {

	mtxHex, err := s.messageToHex(mtx)
	if err != nil {
		return nil, err
	}

	if txHash != mtx.TxHash().String() {
		return nil, rpcInvalidError("Tx hash does not match: got %v "+
			"expected %v", txHash, mtx.TxHash())
	}

	txReply := &types.TxRawResult{
		Hex:         mtxHex,
		Txid:        txHash,
		Vin:         createVinList(mtx, isTreasuryEnabled),
		Vout:        createVoutList(mtx, chainParams, nil),
		Version:     int32(mtx.Version),
		LockTime:    mtx.LockTime,
		Expiry:      mtx.Expiry,
		BlockHeight: blkHeight,
		BlockIndex:  blkIdx,
	}

	if blkHeader != nil {
		// This is not a typo, they are identical in bitcoind as well.
		txReply.Time = blkHeader.Timestamp.Unix()
		txReply.Blocktime = blkHeader.Timestamp.Unix()
		txReply.BlockHash = blkHash
		txReply.Confirmations = confirmations
	}

	return txReply, nil
}

// handleDecodeRawTransaction handles decoderawtransaction commands.
func handleDecodeRawTransaction(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.DecodeRawTransactionCmd)

	// Deserialize the transaction.
	hexStr := c.HexTx
	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}
	serializedTx, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, rpcDecodeHexError(hexStr)
	}
	var mtx wire.MsgTx
	err = mtx.Deserialize(bytes.NewReader(serializedTx))
	if err != nil {
		return nil, rpcDeserializationError("Could not decode Tx: %v",
			err)
	}

	// Determine if the treasury rules are active as of the current best tip.
	prevBlkHash := s.cfg.Chain.BestSnapshot().Hash
	isTreasuryEnabled, err := s.isTreasuryAgendaActive(&prevBlkHash)
	if err != nil {
		return nil, err
	}

	// Create and return the result.
	txReply := types.TxRawDecodeResult{
		Txid:     mtx.TxHash().String(),
		Version:  int32(mtx.Version),
		Locktime: mtx.LockTime,
		Expiry:   mtx.Expiry,
		Vin:      createVinList(&mtx, isTreasuryEnabled),
		Vout:     createVoutList(&mtx, s.cfg.ChainParams, nil),
	}
	return txReply, nil
}

// handleDecodeScript handles decodescript commands.
func handleDecodeScript(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.DecodeScriptCmd)

	// Convert the hex script to bytes.
	hexStr := c.HexScript
	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}
	script, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, rpcDecodeHexError(hexStr)
	}

	// Fetch the script version if provided.
	scriptVersion := uint16(0)
	if c.Version != nil {
		scriptVersion = *c.Version
	}

	// The disassembled string will contain [error] inline if the script
	// doesn't fully parse, so ignore the error here.
	disbuf, _ := txscript.DisasmString(script)

	// Attempt to extract known addresses associated with the script.
	scriptType, addrs := stdscript.ExtractAddrs(scriptVersion, script,
		s.cfg.ChainParams)
	addresses := make([]string, len(addrs))
	for i, addr := range addrs {
		if pkHasher, ok := addr.(stdaddr.AddressPubKeyHasher); ok {
			addr = pkHasher.AddressPubKeyHash()
		}
		addresses[i] = addr.String()
	}

	// Determine the number of required signatures for known standard types.
	reqSigs := stdscript.DetermineRequiredSigs(scriptVersion, script)

	// Convert the script itself to a pay-to-script-hash address.
	p2sh, err := stdaddr.NewAddressScriptHash(scriptVersion, script,
		s.cfg.ChainParams)
	if err != nil {
		const context = "Failed to convert script to pay-to-script-hash"
		return nil, rpcInternalErr(err, context)
	}

	// Generate and return the reply.
	reply := types.DecodeScriptResult{
		Asm:       disbuf,
		ReqSigs:   int32(reqSigs),
		Type:      scriptType.String(),
		Addresses: addresses,
	}
	if scriptType != stdscript.STScriptHash {
		reply.P2sh = p2sh.String()
	}
	return reply, nil
}

// handleEstimateFee implements the estimatefee command.
// TODO this is a very basic implementation.  It should be
// modified to match the bitcoin-core one.
func handleEstimateFee(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	return s.cfg.MinRelayTxFee.ToCoin(), nil
}

// handleEstimateSmartFee implements the estimatesmartfee command.
//
// The default estimation mode when unset is assumed as "conservative". As of
// 2018-12, the only supported mode is "conservative".
func handleEstimateSmartFee(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.EstimateSmartFeeCmd)

	mode := types.EstimateSmartFeeConservative
	if c.Mode != nil {
		mode = *c.Mode
	}

	if mode != types.EstimateSmartFeeConservative {
		return nil, rpcInvalidError("Only the default and conservative modes " +
			"are supported for smart fee estimation at the moment")
	}

	fee, err := s.cfg.FeeEstimator.EstimateFee(int32(c.Confirmations))
	if err != nil {
		return nil, rpcInternalErr(err, "Could not estimate fee")
	}

	return &types.EstimateSmartFeeResult{
		FeeRate: fee.ToCoin(),
		Blocks:  c.Confirmations,
	}, nil
}

// handleEstimateStakeDiff implements the estimatestakediff command.
func handleEstimateStakeDiff(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.EstimateStakeDiffCmd)

	// Minimum possible stake difficulty.
	chain := s.cfg.Chain
	best := chain.BestSnapshot()
	min, err := chain.EstimateNextStakeDifficulty(&best.Hash, 0, false)
	if err != nil {
		const context = "Could not estimate next minimum stake difficulty"
		return nil, rpcInternalErr(err, context)
	}

	// Maximum possible stake difficulty.
	max, err := chain.EstimateNextStakeDifficulty(&best.Hash, 0, true)
	if err != nil {
		const context = "Could not estimate next maximum stake difficulty"
		return nil, rpcInternalErr(err, context)
	}

	// The expected stake difficulty. Average the number of fresh stake
	// since the last retarget to get the number of tickets per block,
	// then use that to estimate the next stake difficulty.
	params := s.cfg.ChainParams
	bestHeight := best.Height
	lastAdjustment := (bestHeight / params.StakeDiffWindowSize) *
		params.StakeDiffWindowSize
	nextAdjustment := ((bestHeight / params.StakeDiffWindowSize) + 1) *
		params.StakeDiffWindowSize
	totalTickets := 0
	for i := lastAdjustment; i <= bestHeight; i++ {
		bh, err := chain.HeaderByHeight(i)
		if err != nil {
			const context = "Could not estimate next stake difficulty"
			return nil, rpcInternalErr(err, context)
		}
		totalTickets += int(bh.FreshStake)
	}
	blocksSince := float64(bestHeight - lastAdjustment + 1)
	remaining := float64(nextAdjustment - bestHeight - 1)
	averagePerBlock := float64(totalTickets) / blocksSince
	expectedTickets := int64(math.Floor(averagePerBlock * remaining))
	expected, err := chain.EstimateNextStakeDifficulty(&best.Hash,
		expectedTickets, false)
	if err != nil {
		const context = "Could not estimate next stake difficulty"
		return nil, rpcInternalErr(err, context)
	}

	// User-specified stake difficulty, if they asked for one.
	var userEstFltPtr *float64
	if c.Tickets != nil {
		userEst, err := chain.EstimateNextStakeDifficulty(&best.Hash,
			int64(*c.Tickets), false)
		if err != nil {
			const context = "Could not estimate next user specified stake " +
				"difficulty"
			return nil, rpcInternalErr(err, context)
		}
		userEstFlt := dcrutil.Amount(userEst).ToCoin()
		userEstFltPtr = &userEstFlt
	}

	return &types.EstimateStakeDiffResult{
		Min:      dcrutil.Amount(min).ToCoin(),
		Max:      dcrutil.Amount(max).ToCoin(),
		Expected: dcrutil.Amount(expected).ToCoin(),
		User:     userEstFltPtr,
	}, nil
}

// handleExistsAddress implements the existsaddress command.
func handleExistsAddress(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	if s.cfg.ExistsAddresser == nil {
		err := errors.New("exists address index disabled")
		return nil, rpcInternalErr(err, "Configuration")
	}

	c := cmd.(*types.ExistsAddressCmd)

	// Decode the provided address.  This also ensures the network encoded with
	// the address matches the network the server is currently on.
	addr, err := stdaddr.DecodeAddress(c.Address, s.cfg.ChainParams)
	if err != nil {
		return nil, rpcAddressKeyError("Could not decode address: %v",
			err)
	}

	// Ensure the exists address index is synced.
	existsAddrIndex := s.cfg.ExistsAddresser
	tHeight, tHash, err := existsAddrIndex.Tip()
	if err != nil {
		return nil, rpcInternalErr(err, "Exists address index tip")
	}

	chain := s.cfg.Chain

	// Return an out-of-sync error if index is lagging a
	// maximum reorg depth (6) blocks or more from the chain tip.
	if chain.BestSnapshot().Height > (tHeight + 5) {
		err := fmt.Errorf("%s: index not synced", existsAddrIndex.Name())
		return nil, rpcInternalErr(err, "Sync")
	}

sync:
	for !chain.BestSnapshot().Hash.IsEqual(tHash) {
		select {
		case <-time.After(syncWait):
			err := fmt.Errorf("%s: index not synced", existsAddrIndex.Name())
			return nil, rpcInternalErr(err, "Sync")
		case <-existsAddrIndex.WaitForSync():
			break sync
		}
	}

	exists, err := existsAddrIndex.ExistsAddress(addr)
	if err != nil {
		return nil, rpcInvalidError("Could not query address: %v", err)
	}

	return exists, nil
}

// handleExistsAddresses implements the existsaddresses command.
//
// TODO: Add an upper bound to the number of addresses that can be checked.
// This will come with a major RPC version bump.
func handleExistsAddresses(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	if s.cfg.ExistsAddresser == nil {
		err := errors.New("exists address index disabled")
		return nil, rpcInternalErr(err, "Configuration")
	}

	c := cmd.(*types.ExistsAddressesCmd)
	addresses := make([]stdaddr.Address, len(c.Addresses))
	for i := range c.Addresses {
		// Decode the provided address.  This also ensures the network encoded
		// with the address matches the network the server is currently on.
		addr, err := stdaddr.DecodeAddress(c.Addresses[i], s.cfg.ChainParams)
		if err != nil {
			return nil, rpcAddressKeyError("Could not decode address: %v", err)
		}
		addresses[i] = addr
	}

	// Ensure the exists address index is synced.
	existsAddrIndex := s.cfg.ExistsAddresser
	tHeight, tHash, err := existsAddrIndex.Tip()
	if err != nil {
		return nil, rpcInternalErr(err, "Tip")
	}

	chain := s.cfg.Chain

	// Return an out-of-sync error if index is lagging a
	// maximum reorg depth (6) blocks or more from the chain tip.
	if chain.BestSnapshot().Height > (tHeight + 5) {
		err := fmt.Errorf("%s: index not synced", existsAddrIndex.Name())
		return nil, rpcInternalErr(err, "Sync")
	}

sync:
	for !chain.BestSnapshot().Hash.IsEqual(tHash) {
		select {
		case <-time.After(syncWait):
			err := fmt.Errorf("%s: index not synced", existsAddrIndex.Name())
			return nil, rpcInternalErr(err, "Sync")
		case <-existsAddrIndex.WaitForSync():
			break sync
		}
	}

	exists, err := existsAddrIndex.ExistsAddresses(addresses)
	if err != nil {
		return nil, rpcInvalidError("Could not query address: %v", err)
	}

	// Convert the slice of bools into a compacted set of bit flags.
	set := bitset.NewBytes(len(c.Addresses))
	for i := range exists {
		if exists[i] {
			set.Set(i)
		}
	}

	return hex.EncodeToString([]byte(set)), nil
}

func decodeHashes(strs []string) ([]chainhash.Hash, error) {
	hashes := make([]chainhash.Hash, len(strs))
	for i, s := range strs {
		if len(s) != 2*chainhash.HashSize {
			return nil, rpcDecodeHexError(s)
		}
		_, err := hex.Decode(hashes[i][:], []byte(s))
		if err != nil {
			return nil, rpcDecodeHexError(s)
		}
		// unreverse hash string bytes
		for j := 0; j < 16; j++ {
			hashes[i][j], hashes[i][31-j] = hashes[i][31-j], hashes[i][j]
		}
	}
	return hashes, nil
}

func decodeHashPointers(strs []string) ([]*chainhash.Hash, error) {
	hashes := make([]*chainhash.Hash, len(strs))
	for i, s := range strs {
		h, err := chainhash.NewHashFromStr(s)
		if err != nil {
			return nil, rpcDecodeHexError(s)
		}
		hashes[i] = h
	}
	return hashes, nil
}

// handleExistsLiveTicket implements the existsliveticket command.
func handleExistsLiveTicket(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.ExistsLiveTicketCmd)

	hash, err := chainhash.NewHashFromStr(c.TxHash)
	if err != nil {
		return nil, rpcDecodeHexError(c.TxHash)
	}

	return s.cfg.Chain.CheckLiveTicket(*hash), nil
}

// handleExistsLiveTickets implements the existslivetickets command.
//
// TODO: Add an upper bound to the number of hashes that can be checked. This
// will come with a major RPC version bump.
func handleExistsLiveTickets(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.ExistsLiveTicketsCmd)

	hashes, err := decodeHashes(c.TxHashes)
	if err != nil {
		return nil, err
	}

	exists := s.cfg.Chain.CheckLiveTickets(hashes)
	if len(exists) != len(hashes) {
		return nil, rpcInvalidError("Invalid live ticket count got "+
			"%v, want %v", len(exists), len(hashes))
	}

	// Convert the slice of bools into a compacted set of bit flags.
	set := bitset.NewBytes(len(hashes))
	for i := range exists {
		if exists[i] {
			set.Set(i)
		}
	}

	return hex.EncodeToString([]byte(set)), nil
}

// handleExistsMempoolTxs implements the existsmempooltxs command.
//
// TODO: Add an upper bound to the number of hashes that can be checked. This
// will come with a major RPC version bump.
func handleExistsMempoolTxs(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.ExistsMempoolTxsCmd)

	hashes, err := decodeHashPointers(c.TxHashes)
	if err != nil {
		return nil, err
	}

	exists := s.cfg.TxMempooler.HaveTransactions(hashes)
	if len(exists) != len(hashes) {
		err := fmt.Errorf("got %v, want %v", len(exists), len(hashes))
		return nil, rpcInternalErr(err, "Invalid mempool Tx ticket count")
	}

	// Convert the slice of bools into a compacted set of bit flags.
	set := bitset.NewBytes(len(hashes))
	for i := range exists {
		if exists[i] {
			set.Set(i)
		}
	}

	return hex.EncodeToString([]byte(set)), nil
}

// handleGenerate handles generate commands.
func handleGenerate(ctx context.Context, s *Server, cmd interface{}) (interface{}, error) {
	// Respond with an error if there are no addresses to pay the
	// created blocks to.
	if len(s.cfg.MiningAddrs) == 0 {
		err := errors.New("no payment addresses specified via --miningaddr")
		return nil, rpcInternalErr(err, "Configuration")
	}

	// Respond with an error if there's virtually 0 chance of CPU-mining a block.
	params := s.cfg.ChainParams
	if !params.GenerateSupported {
		return nil, &dcrjson.RPCError{
			Code: dcrjson.ErrRPCDifficulty,
			Message: fmt.Sprintf("No support for `generate` on the current "+
				"network, %s, as it's unlikely to be possible to mine a block "+
				"with the CPU.", params.Net),
		}
	}

	c := cmd.(*types.GenerateCmd)

	// Respond with an error when no blocks are requested.
	if c.NumBlocks == 0 {
		err := errors.New("invalid number of blocks")
		return nil, rpcInternalErr(err, "Configuration")
	}

	// Mine the correct number of blocks, assigning the hex representation of
	// the hash of each one to its place in the reply.
	blockHashes, err := s.cfg.CPUMiner.GenerateNBlocks(ctx, c.NumBlocks)
	if err != nil {
		return nil, rpcInternalErr(err, "Could not generate blocks")
	}
	reply := make([]string, 0, len(blockHashes))
	for _, hash := range blockHashes {
		reply = append(reply, hash.String())
	}
	return reply, nil
}

// handleGetAddedNodeInfo handles getaddednodeinfo commands.
func handleGetAddedNodeInfo(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetAddedNodeInfoCmd)

	// Retrieve a list of persistent (added) peers from the Decred server
	// and filter the list of peers per the specified address (if any).
	peers := s.cfg.ConnMgr.PersistentPeers()
	if c.Node != nil {
		found := false
		for i, peer := range peers {
			if peer.Addr() == *c.Node {
				peers = peers[i : i+1]
				found = true
			}
		}
		if !found {
			err := errors.New("node not found")
			return nil, rpcInternalErr(err, "")
		}
	}

	// Without the dns flag, the result is just a slice of the addresses as
	// strings.
	if !c.DNS {
		results := make([]string, 0, len(peers))
		for _, peer := range peers {
			results = append(results, peer.Addr())
		}
		return results, nil
	}

	// With the dns flag, the result is an array of JSON objects which
	// include the result of DNS lookups for each peer.
	results := make([]*types.GetAddedNodeInfoResult, 0, len(peers))
	for _, peer := range peers {
		// Set the "address" of the peer which could be an ip address
		// or a domain name.
		var result types.GetAddedNodeInfoResult
		result.AddedNode = peer.Addr()
		result.Connected = dcrjson.Bool(peer.Connected())

		// Split the address into host and port portions so we can do a
		// DNS lookup against the host.  When no port is specified in
		// the address, just use the address as the host.
		host, _, err := net.SplitHostPort(peer.Addr())
		if err != nil {
			host = peer.Addr()
		}

		// Do a DNS lookup for the address.  If the lookup fails, just
		// use the host.
		var ipList []string
		ips, err := s.cfg.ConnMgr.Lookup(host)
		if err == nil {
			ipList = make([]string, 0, len(ips))
			for _, ip := range ips {
				ipList = append(ipList, ip.String())
			}
		} else {
			ipList = make([]string, 1)
			ipList[0] = host
		}

		// Add the addresses and connection info to the result.
		addrs := make([]types.GetAddedNodeInfoResultAddr, 0,
			len(ipList))
		for _, ip := range ipList {
			var addr types.GetAddedNodeInfoResultAddr
			addr.Address = ip
			addr.Connected = "false"
			if ip == host && peer.Connected() {
				addr.Connected = directionString(peer.Inbound())
			}
			addrs = append(addrs, addr)
		}
		result.Addresses = &addrs
		results = append(results, &result)
	}
	return results, nil
}

// handleGetBestBlock implements the getbestblock command.
func handleGetBestBlock(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	// All other "get block" commands give either the height, the hash, or
	// both but require the block SHA.  This gets both for the best block.
	best := s.cfg.Chain.BestSnapshot()
	result := &types.GetBestBlockResult{
		Hash:   best.Hash.String(),
		Height: best.Height,
	}
	return result, nil
}

// handleGetBestBlockHash implements the getbestblockhash command.
func handleGetBestBlockHash(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	best := s.cfg.Chain.BestSnapshot()
	return best.Hash.String(), nil
}

// getDifficultyRatio returns the proof-of-work difficulty as a multiple of the
// minimum difficulty using the passed bits field from the header of a block.
func getDifficultyRatio(bits uint32, params *chaincfg.Params) float64 {
	// The minimum difficulty is the max possible proof-of-work limit bits
	// converted back to a number.  Note this is not the same as the proof
	// of work limit directly because the block difficulty is encoded in a
	// block with the compact form which loses precision.
	max := standalone.CompactToBig(params.PowLimitBits)
	target := standalone.CompactToBig(bits)

	difficulty := new(big.Rat).SetFrac(max, target)
	outString := difficulty.FloatString(8)
	diff, err := strconv.ParseFloat(outString, 64)
	if err != nil {
		log.Errorf("Cannot get difficulty: %v", err)
		return 0
	}
	return diff
}

// handleGetBlock implements the getblock command.
func handleGetBlock(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetBlockCmd)

	// Load the raw block bytes from the database.
	hash, err := chainhash.NewHashFromStr(c.Hash)
	if err != nil {
		return nil, rpcDecodeHexError(c.Hash)
	}

	chain := s.cfg.Chain
	blk, err := chain.BlockByHash(hash)
	if err != nil {
		return nil, &dcrjson.RPCError{
			Code:    dcrjson.ErrRPCBlockNotFound,
			Message: fmt.Sprintf("Block not found: %v", hash),
		}
	}

	// When the verbose flag isn't set, simply return the
	// network-serialized block as a hex-encoded string.
	if c.Verbose != nil && !*c.Verbose {
		blkBytes, err := blk.Bytes()
		if err != nil {
			return nil, rpcInternalErr(err, "Could not serialize block")
		}

		return hex.EncodeToString(blkBytes), nil
	}

	chainWork, err := chain.ChainWork(hash)
	if err != nil {
		return nil, rpcInternalErr(err, "Failed to retrieve work")
	}

	best := chain.BestSnapshot()

	// Get next block hash unless there are none.
	var nextHashString string
	blockHeader := &blk.MsgBlock().Header
	confirmations := int64(-1)
	if chain.MainChainHasBlock(hash) {
		if int64(blockHeader.Height) < best.Height {
			nextHash, err := chain.BlockHashByHeight(int64(blockHeader.Height + 1))
			if err != nil {
				return nil, rpcInternalErr(err, "No next block")
			}
			nextHashString = nextHash.String()
		}
		confirmations = 1 + best.Height - int64(blockHeader.Height)
	}

	sbitsFloat := float64(blockHeader.SBits) / dcrutil.AtomsPerCoin

	medianTime, err := chain.MedianTimeByHash(hash)
	if err != nil {
		return nil, rpcInternalErr(err, "Unable to retrieve median block time")
	}

	isBlake3PowActive, err := s.isBlake3PowAgendaActive(&blockHeader.PrevBlock)
	if err != nil {
		return nil, err
	}
	powHashFn := blockHeader.PowHashV1
	if isBlake3PowActive {
		powHashFn = blockHeader.PowHashV2
	}
	powHash := powHashFn()

	blockReply := types.GetBlockVerboseResult{
		Hash:          c.Hash,
		PoWHash:       powHash.String(),
		Version:       blockHeader.Version,
		MerkleRoot:    blockHeader.MerkleRoot.String(),
		StakeRoot:     blockHeader.StakeRoot.String(),
		PreviousHash:  blockHeader.PrevBlock.String(),
		Nonce:         blockHeader.Nonce,
		VoteBits:      blockHeader.VoteBits,
		FinalState:    hex.EncodeToString(blockHeader.FinalState[:]),
		Voters:        blockHeader.Voters,
		FreshStake:    blockHeader.FreshStake,
		Revocations:   blockHeader.Revocations,
		PoolSize:      blockHeader.PoolSize,
		Time:          blockHeader.Timestamp.Unix(),
		MedianTime:    medianTime.Unix(),
		StakeVersion:  blockHeader.StakeVersion,
		Confirmations: confirmations,
		Height:        int64(blockHeader.Height),
		Size:          int32(blk.MsgBlock().Header.Size),
		Bits:          strconv.FormatInt(int64(blockHeader.Bits), 16),
		SBits:         sbitsFloat,
		Difficulty:    getDifficultyRatio(blockHeader.Bits, s.cfg.ChainParams),
		ChainWork:     fmt.Sprintf("%064x", chainWork),
		ExtraData:     hex.EncodeToString(blockHeader.ExtraData[:]),
		NextHash:      nextHashString,
	}

	// Determine if the treasury rules are active for the block.
	isTreasuryEnabled, err := s.isTreasuryAgendaActive(&blockHeader.PrevBlock)
	if err != nil {
		return nil, err
	}

	if c.VerboseTx == nil || !*c.VerboseTx {
		transactions := blk.Transactions()
		txNames := make([]string, len(transactions))
		for i, tx := range transactions {
			txNames[i] = tx.Hash().String()
		}

		blockReply.Tx = txNames

		stransactions := blk.STransactions()
		stxNames := make([]string, len(stransactions))
		for i, tx := range stransactions {
			stxNames[i] = tx.Hash().String()
		}

		blockReply.STx = stxNames
	} else {
		txns := blk.Transactions()
		chainParams := s.cfg.ChainParams
		rawTxns := make([]types.TxRawResult, len(txns))
		for i, tx := range txns {
			rawTxn, err := s.createTxRawResult(chainParams,
				tx.MsgTx(), tx.Hash().String(), uint32(i),
				blockHeader, blk.Hash().String(),
				int64(blockHeader.Height), confirmations,
				isTreasuryEnabled)
			if err != nil {
				return nil, rpcInternalErr(err, "Could not create transaction")
			}
			rawTxns[i] = *rawTxn
		}
		blockReply.RawTx = rawTxns

		stxns := blk.STransactions()
		rawSTxns := make([]types.TxRawResult, len(stxns))
		for i, tx := range stxns {
			rawSTxn, err := s.createTxRawResult(chainParams,
				tx.MsgTx(), tx.Hash().String(), uint32(i),
				blockHeader, blk.Hash().String(),
				int64(blockHeader.Height), confirmations,
				isTreasuryEnabled)
			if err != nil {
				const context = "Could not create stake transaction"
				return nil, rpcInternalErr(err, context)
			}
			rawSTxns[i] = *rawSTxn
		}
		blockReply.RawSTx = rawSTxns
	}

	return blockReply, nil
}

// thresholdStateToAgendaStatus converts a threshold state tuple to an agenda
// status string suitable for use in the response to the getblockchaininfo
// command.
func thresholdStateToAgendaStatus(state blockchain.ThresholdStateTuple) string {
	switch state.State {
	case blockchain.ThresholdDefined:
		return types.AgendaInfoStatusDefined
	case blockchain.ThresholdStarted:
		return types.AgendaInfoStatusStarted
	case blockchain.ThresholdLockedIn:
		return types.AgendaInfoStatusLockedIn
	case blockchain.ThresholdActive:
		return types.AgendaInfoStatusActive
	case blockchain.ThresholdFailed:
		return types.AgendaInfoStatusFailed
	}

	return types.AgendaInfoStatusDefined
}

// handleGetBlockchainInfo implements the getblockchaininfo command.
func handleGetBlockchainInfo(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	chain := s.cfg.Chain
	best := chain.BestSnapshot()
	_, bestHeaderHeight := chain.BestHeader()

	// Fetch the current chain work using the best block hash.
	chainWork, err := chain.ChainWork(&best.Hash)
	if err != nil {
		return nil, rpcInternalErr(err, "Could not fetch chain work")
	}

	// Estimate the verification progress of the node.
	var verifyProgress float64
	if bestHeaderHeight > 0 {
		progress := float64(best.Height) / float64(bestHeaderHeight)
		verifyProgress = math.Min(progress, 1.0)
	}

	// Fetch the maximum allowed block size for all blocks other than the
	// genesis block.
	params := s.cfg.ChainParams
	maxBlockSize := int64(params.MaximumBlockSizes[0])
	if best.PrevHash != zeroHash {
		maxBlockSize, err = chain.MaxBlockSize(&best.PrevHash)
		if err != nil {
			return nil, rpcInternalErr(err, "Could not fetch max block size")
		}
	}

	// Fetch the agendas of the consensus deployments as well as their
	// threshold states and state activation heights.
	dInfo := make(map[string]types.AgendaInfo)
	defaultStatus := types.AgendaInfoStatusDefined
	for _, deployments := range params.Deployments {
		for _, agenda := range deployments {
			aInfo := types.AgendaInfo{
				StartTime:  agenda.StartTime,
				ExpireTime: agenda.ExpireTime,
				Status:     defaultStatus,
			}

			// If the best block is the genesis block, continue without attempting to
			// query the threshold state or state changed height.
			if best.PrevHash == zeroHash {
				dInfo[agenda.Vote.Id] = aInfo
				continue
			}

			state, err := chain.NextThresholdState(&best.PrevHash,
				agenda.Vote.Id)
			if err != nil {
				context := fmt.Sprintf("Could not fetch threshold state for "+
					"agenda with id (%v)", agenda.Vote.Id)
				return nil, rpcInternalErr(err, context)
			}

			stateChangedHeight, err := chain.StateLastChangedHeight(
				&best.Hash, agenda.Vote.Id)
			if err != nil {
				context := fmt.Sprintf("Could not fetch state last changed "+
					"height for agenda with id (%v)", agenda.Vote.Id)
				return nil, rpcInternalErr(err, context)
			}

			aInfo.Since = stateChangedHeight
			aInfo.Status = thresholdStateToAgendaStatus(state)
			dInfo[agenda.Vote.Id] = aInfo
		}
	}

	// Generate rpc response.
	response := types.GetBlockChainInfoResult{
		Chain:                params.Name,
		Blocks:               best.Height,
		Headers:              bestHeaderHeight,
		SyncHeight:           s.cfg.SyncMgr.SyncHeight(),
		ChainWork:            fmt.Sprintf("%064x", chainWork),
		InitialBlockDownload: !chain.IsCurrent(),
		VerificationProgress: verifyProgress,
		BestBlockHash:        best.Hash.String(),
		Difficulty:           best.Bits,
		DifficultyRatio:      getDifficultyRatio(best.Bits, params),
		MaxBlockSize:         maxBlockSize,
		Deployments:          dInfo,
	}

	return response, nil
}

// handleGetBlockCount implements the getblockcount command.
func handleGetBlockCount(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	best := s.cfg.Chain.BestSnapshot()
	return best.Height, nil
}

// handleGetBlockHash implements the getblockhash command.
func handleGetBlockHash(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetBlockHashCmd)
	hash, err := s.cfg.Chain.BlockHashByHeight(c.Index)
	if err != nil {
		return nil, &dcrjson.RPCError{
			Code: dcrjson.ErrRPCOutOfRange,
			Message: fmt.Sprintf("Block number out of range: %v",
				c.Index),
		}
	}

	return hash.String(), nil
}

// handleGetBlockHeader implements the getblockheader command.
func handleGetBlockHeader(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetBlockHeaderCmd)

	// Fetch the header from chain.
	hash, err := chainhash.NewHashFromStr(c.Hash)
	if err != nil {
		return nil, rpcDecodeHexError(c.Hash)
	}

	chain := s.cfg.Chain
	blockHeader, err := chain.HeaderByHash(hash)
	if err != nil {
		return nil, &dcrjson.RPCError{
			Code:    dcrjson.ErrRPCBlockNotFound,
			Message: fmt.Sprintf("Block not found: %v", c.Hash),
		}
	}

	// When the verbose flag isn't set, simply return the serialized block
	// header as a hex-encoded string.
	if c.Verbose != nil && !*c.Verbose {
		var headerBuf bytes.Buffer
		err := blockHeader.Serialize(&headerBuf)
		if err != nil {
			return nil, rpcInternalErr(err, "Failed to serialize block header")
		}
		return hex.EncodeToString(headerBuf.Bytes()), nil
	}

	// The verbose flag is set, so generate the JSON object and return it.

	chainWork, err := chain.ChainWork(hash)
	if err != nil {
		return nil, rpcInternalErr(err, "Failed to retrieve work")
	}

	best := chain.BestSnapshot()

	// Get next block hash unless there are none.
	var nextHashString string
	confirmations := int64(-1)
	height := int64(blockHeader.Height)
	if chain.MainChainHasBlock(hash) {
		if height < best.Height {
			nextHash, err := chain.BlockHashByHeight(height + 1)
			if err != nil {
				return nil, rpcInternalErr(err, "No next block")
			}
			nextHashString = nextHash.String()
		}
		confirmations = 1 + best.Height - height
	}

	medianTime, err := chain.MedianTimeByHash(hash)
	if err != nil {
		return nil, rpcInternalErr(err, "Unable to retrieve median block time")
	}

	isBlake3PowActive, err := s.isBlake3PowAgendaActive(&blockHeader.PrevBlock)
	if err != nil {
		return nil, err
	}
	powHashFn := blockHeader.PowHashV1
	if isBlake3PowActive {
		powHashFn = blockHeader.PowHashV2
	}
	powHash := powHashFn()

	blockHeaderReply := types.GetBlockHeaderVerboseResult{
		Hash:          c.Hash,
		PowHash:       powHash.String(),
		Confirmations: confirmations,
		Version:       blockHeader.Version,
		MerkleRoot:    blockHeader.MerkleRoot.String(),
		StakeRoot:     blockHeader.StakeRoot.String(),
		VoteBits:      blockHeader.VoteBits,
		FinalState:    hex.EncodeToString(blockHeader.FinalState[:]),
		Voters:        blockHeader.Voters,
		FreshStake:    blockHeader.FreshStake,
		Revocations:   blockHeader.Revocations,
		PoolSize:      blockHeader.PoolSize,
		Bits:          strconv.FormatInt(int64(blockHeader.Bits), 16),
		SBits:         dcrutil.Amount(blockHeader.SBits).ToCoin(),
		Height:        uint32(height),
		Size:          blockHeader.Size,
		Time:          blockHeader.Timestamp.Unix(),
		MedianTime:    medianTime.Unix(),
		Nonce:         blockHeader.Nonce,
		ExtraData:     hex.EncodeToString(blockHeader.ExtraData[:]),
		StakeVersion:  blockHeader.StakeVersion,
		Difficulty:    getDifficultyRatio(blockHeader.Bits, s.cfg.ChainParams),
		ChainWork:     fmt.Sprintf("%064x", chainWork),
		PreviousHash:  blockHeader.PrevBlock.String(),
		NextHash:      nextHashString,
	}

	return blockHeaderReply, nil
}

// handleGetBlockSubsidy implements the getblocksubsidy command.
func handleGetBlockSubsidy(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetBlockSubsidyCmd)

	height := c.Height
	voters := c.Voters

	// Determine which agendas are active as of the provided height when that
	// height exists in the main chain or as of the current best tip otherwise.
	chain := s.cfg.Chain
	best := chain.BestSnapshot()
	prevBlkHash := best.Hash
	if height <= best.Height {
		header, err := chain.HeaderByHeight(height)
		if err != nil {
			context := fmt.Sprintf("Failed to retrieve header for height %d",
				height)
			return nil, rpcInternalErr(err, context)
		}
		prevBlkHash = header.PrevBlock
	}
	isTreasuryEnabled, err := s.isTreasuryAgendaActive(&prevBlkHash)
	if err != nil {
		return nil, err
	}
	isSubsidyEnabled, err := s.isSubsidySplitAgendaActive(&prevBlkHash)
	if err != nil {
		return nil, err
	}
	isSubsidyR2Enabled, err := s.isSubsidySplitR2AgendaActive(&prevBlkHash)
	if err != nil {
		return nil, err
	}

	// Determine which subsidy split variant to use depending on the active
	// agendas.
	subsidySplitVariant := standalone.SSVOriginal
	switch {
	case isSubsidyR2Enabled:
		subsidySplitVariant = standalone.SSVDCP0012
	case isSubsidyEnabled:
		subsidySplitVariant = standalone.SSVDCP0010
	}

	subsidyCache := s.cfg.SubsidyCache
	dev := subsidyCache.CalcTreasurySubsidy(height, voters, isTreasuryEnabled)
	pos := subsidyCache.CalcStakeVoteSubsidyV3(height-1, subsidySplitVariant) *
		int64(voters)
	pow := subsidyCache.CalcWorkSubsidyV3(height, voters, subsidySplitVariant)
	total := dev + pos + pow

	rep := types.GetBlockSubsidyResult{
		Developer: dev,
		PoS:       pos,
		PoW:       pow,
		Total:     total,
	}

	return rep, nil
}

// handleGetChainTips implements the getchaintips command.
func handleGetChainTips(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	chainTips := s.cfg.Chain.ChainTips()
	result := make([]types.GetChainTipsResult, 0, len(chainTips))
	for _, tip := range chainTips {
		result = append(result, types.GetChainTipsResult{
			Height:    tip.Height,
			Hash:      tip.Hash.String(),
			BranchLen: tip.BranchLen,
			Status:    tip.Status,
		})
	}
	return result, nil
}

// handleGetCoinSupply implements the getcoinsupply command.
func handleGetCoinSupply(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	return s.cfg.Chain.BestSnapshot().TotalSubsidy, nil
}

// handleGetConnectionCount implements the getconnectioncount command.
func handleGetConnectionCount(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	return s.cfg.ConnMgr.ConnectedCount(), nil
}

// handleGetCurrentNet implements the getcurrentnet command.
func handleGetCurrentNet(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	return s.cfg.ChainParams.Net, nil
}

// handleGetDifficulty implements the getdifficulty command.
func handleGetDifficulty(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	best := s.cfg.Chain.BestSnapshot()
	return getDifficultyRatio(best.Bits, s.cfg.ChainParams), nil
}

// handleGetGenerate implements the getgenerate command.
func handleGetGenerate(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	return s.cfg.CPUMiner.IsMining(), nil
}

// handleGetHashesPerSec implements the gethashespersec command.
func handleGetHashesPerSec(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	return int64(s.cfg.CPUMiner.HashesPerSecond()), nil
}

// handleGetHeaders implements the getheaders command.
func handleGetHeaders(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetHeadersCmd)
	blockLocators, err := decodeHashes(c.BlockLocators)
	if err != nil {
		// Already a *dcrjson.RPCError
		return nil, err
	}
	var hashStop chainhash.Hash
	if c.HashStop != "" {
		err := chainhash.Decode(&hashStop, c.HashStop)
		if err != nil {
			return nil, rpcInvalidError("Failed to decode "+
				"hashstop: %v", err)
		}
	}

	// Until wire.MsgGetHeaders uses []Hash instead of the []*Hash, this
	// conversion is necessary.  The wire protocol getheaders is (probably)
	// called much more often than this RPC, so chain.LocateHeaders is
	// optimized for that and this is given the performance penalty.
	locators := make(blockchain.BlockLocator, len(blockLocators))
	for i := range blockLocators {
		locators[i] = &blockLocators[i]
	}

	chain := s.cfg.Chain
	headers := chain.LocateHeaders(locators, &hashStop)

	// Return the serialized block headers as hex-encoded strings.
	hexBlockHeaders := make([]string, len(headers))
	var buf bytes.Buffer
	buf.Grow(wire.MaxBlockHeaderPayload)
	for i, h := range headers {
		err := h.Serialize(&buf)
		if err != nil {
			return nil, rpcInternalErr(err, "Failed to serialize block header")
		}
		hexBlockHeaders[i] = hex.EncodeToString(buf.Bytes())
		buf.Reset()
	}
	return &types.GetHeadersResult{Headers: hexBlockHeaders}, nil
}

// handleGetCFilterV2 implements the getcfilterv2 command.
func handleGetCFilterV2(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetCFilterV2Cmd)
	hash, err := chainhash.NewHashFromStr(c.BlockHash)
	if err != nil {
		return nil, rpcDecodeHexError(c.BlockHash)
	}

	filter, proof, err := s.cfg.FiltererV2.FilterByBlockHash(hash)
	if err != nil {
		if errors.Is(err, blockchain.ErrNoFilter) {
			return nil, &dcrjson.RPCError{
				Code:    dcrjson.ErrRPCBlockNotFound,
				Message: fmt.Sprintf("Block not found: %v", hash),
			}
		}

		context := fmt.Sprintf("Failed to load filter for block %s", hash)
		return nil, rpcInternalErr(err, context)
	}

	var proofHashes []string
	if len(proof.ProofHashes) > 0 {
		proofHashes = make([]string, 0, len(proof.ProofHashes))
		for i := range proof.ProofHashes {
			proofHashes[i] = proof.ProofHashes[i].String()
		}
	}

	result := &types.GetCFilterV2Result{
		BlockHash:   c.BlockHash,
		Data:        hex.EncodeToString(filter.Bytes()),
		ProofIndex:  proof.ProofIndex,
		ProofHashes: proofHashes,
	}
	return result, nil
}

// handleGetInfo implements the getinfo command. We only return the fields
// that are not related to wallet functionality.
func handleGetInfo(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	best := s.cfg.Chain.BestSnapshot()
	ret := &types.InfoChainResult{
		Version: int32(1000000*version.Major + 10000*version.Minor +
			100*version.Patch),
		ProtocolVersion: int32(s.cfg.MaxProtocolVersion),
		Blocks:          best.Height,
		TimeOffset:      int64(s.cfg.TimeSource.Offset().Seconds()),
		Connections:     s.cfg.ConnMgr.ConnectedCount(),
		Proxy:           s.cfg.Proxy,
		Difficulty:      getDifficultyRatio(best.Bits, s.cfg.ChainParams),
		TestNet:         s.cfg.TestNet,
		RelayFee:        s.cfg.MinRelayTxFee.ToCoin(),
		TxIndex:         s.cfg.TxIndexer != nil,
	}

	return ret, nil
}

// handleGetMempoolInfo implements the getmempoolinfo command.
func handleGetMempoolInfo(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	mempoolTxns := s.cfg.TxMempooler.TxDescs()

	var numBytes int64
	for _, txD := range mempoolTxns {
		numBytes += int64(txD.Tx.MsgTx().SerializeSize())
	}

	ret := &types.GetMempoolInfoResult{
		Size:  int64(len(mempoolTxns)),
		Bytes: numBytes,
	}

	return ret, nil
}

// handleGetMiningInfo implements the getmininginfo command. We only return the
// fields that are not related to wallet functionality.
func handleGetMiningInfo(ctx context.Context, s *Server, _ interface{}) (interface{}, error) {
	// Create a default getnetworkhashps command to use defaults and make
	// use of the existing getnetworkhashps handler.
	gnhpsCmd := types.NewGetNetworkHashPSCmd(nil, nil)
	networkHashesPerSecIface, err := handleGetNetworkHashPS(ctx, s, gnhpsCmd)
	if err != nil {
		return nil, err
	}
	networkHashesPerSec, ok := networkHashesPerSecIface.(int64)
	if !ok {
		err := errors.New("invalid network hashes per sec")
		context := fmt.Sprintf("Invalid type: %q", networkHashesPerSecIface)
		return nil, rpcInternalErr(err, context)
	}

	best := s.cfg.Chain.BestSnapshot()
	result := types.GetMiningInfoResult{
		Blocks:           best.Height,
		CurrentBlockSize: best.BlockSize,
		CurrentBlockTx:   best.NumTxns,
		Difficulty:       getDifficultyRatio(best.Bits, s.cfg.ChainParams),
		StakeDifficulty:  best.NextStakeDiff,
		Generate:         s.cfg.CPUMiner.IsMining(),
		GenProcLimit:     s.cfg.CPUMiner.NumWorkers(),
		HashesPerSec:     int64(s.cfg.CPUMiner.HashesPerSecond()),
		NetworkHashPS:    networkHashesPerSec,
		PooledTx:         uint64(s.cfg.TxMempooler.Count()),
		TestNet:          s.cfg.TestNet,
	}
	return &result, nil
}

// handleGetMixMessage implements the getmixmessage command, returning a
// serialized message and its wire command type if it is found in the
// mixpool.
func handleGetMixMessage(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetMixMessageCmd)

	msgHash, err := chainhash.NewHashFromStr(c.Hash)
	if err != nil {
		return nil, rpcDecodeHexError(c.Hash)
	}

	mp := s.cfg.MixPooler
	msg, err := mp.Message(msgHash)
	if err != nil {
		return nil, rpcMixMessageNotFoundError(msgHash)
	}

	buf := new(strings.Builder)
	err = msg.BtcEncode(hex.NewEncoder(buf), wire.MixVersion)
	if err != nil {
		return nil, err
	}

	result := types.GetMixMessageResult{
		Type:    msg.Command(),
		Message: buf.String(),
	}
	return &result, nil
}

// handleGetMixPairRequests implements the getmixpairrequests command,
// returning all current mixing pair requests messages from mixpool.
func handleGetMixPairRequests(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	mp := s.cfg.MixPooler

	prs := mp.MixPRs()

	buf := new(strings.Builder)
	res := make([]string, 0, len(prs))

	const pver = wire.MixVersion
	for _, pr := range prs {
		err := pr.BtcEncode(hex.NewEncoder(buf), pver)
		if err != nil {
			return nil, err
		}
		res = append(res, buf.String())
		buf.Reset()
	}

	return res, nil
}

// handleGetNetTotals implements the getnettotals command.
func handleGetNetTotals(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	totalBytesRecv, totalBytesSent := s.cfg.ConnMgr.NetTotals()
	reply := &types.GetNetTotalsResult{
		TotalBytesRecv: totalBytesRecv,
		TotalBytesSent: totalBytesSent,
		TimeMillis:     s.cfg.Clock.Now().UTC().UnixNano() / int64(time.Millisecond),
	}
	return reply, nil
}

// handleGetNetworkHashPS implements the getnetworkhashps command.
func handleGetNetworkHashPS(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	// Note: All valid error return paths should return an int64.  Literal
	// zeros are inferred as int, and won't coerce to int64 because the
	// return value is an interface{}.

	c := cmd.(*types.GetNetworkHashPSCmd)

	// When the passed height is too high or zero, just return 0 now since
	// we can't reasonably calculate the number of network hashes per
	// second from invalid values.  When it's negative, use the current
	// best block height.
	chain := s.cfg.Chain
	best := chain.BestSnapshot()
	endHeight := int64(-1)
	if c.Height != nil {
		endHeight = int64(*c.Height)
	}
	if endHeight > best.Height || endHeight == 0 {
		return int64(0), nil
	}
	if endHeight < 0 {
		endHeight = best.Height
	}

	// Calculate the starting block height based on the passed number of blocks.
	// When the passed value is negative, use the default.  Also, make sure the
	// starting height is not before the beginning of the chain.
	numBlocks := int64(120)
	if c.Blocks != nil && *c.Blocks >= 0 {
		numBlocks = int64(*c.Blocks)
	}

	startHeight := endHeight - numBlocks
	if startHeight < 0 {
		startHeight = 0
	}
	log.Debugf("Calculating network hashes per second from %d to %d",
		startHeight, endHeight)

	// Find the min and max block timestamps as well as calculate the total
	// amount of work that happened between the start and end blocks.
	var minTimestamp, maxTimestamp time.Time
	totalWork := big.NewInt(0)
	for curHeight := startHeight; curHeight <= endHeight; curHeight++ {
		hash, err := chain.BlockHashByHeight(curHeight)
		if err != nil {
			return nil, rpcInternalErr(err, "Failed to fetch block hash")
		}

		// Fetch the header from chain.
		header, err := chain.HeaderByHash(hash)
		if err != nil {
			return nil, rpcInternalErr(err, "Failed to fetch block header")
		}

		if curHeight == startHeight {
			minTimestamp = header.Timestamp
			maxTimestamp = minTimestamp
		} else {
			totalWork.Add(totalWork, standalone.CalcWork(header.Bits))

			if minTimestamp.After(header.Timestamp) {
				minTimestamp = header.Timestamp
			}
			if maxTimestamp.Before(header.Timestamp) {
				maxTimestamp = header.Timestamp
			}
		}
	}

	// Calculate the difference in seconds between the min and max block
	// timestamps and avoid division by zero in the case where there is no
	// time difference.
	timeDiff := int64(maxTimestamp.Sub(minTimestamp) / time.Second)
	if timeDiff == 0 {
		return int64(0), nil
	}

	hashesPerSec := new(big.Int).Div(totalWork, big.NewInt(timeDiff))
	return hashesPerSec.Int64(), nil
}

// handleGetNetworkInfo implements the getnetworkinfo command.
func handleGetNetworkInfo(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	lAddrs := s.cfg.AddrManager.LocalAddresses()
	localAddrs := make([]types.LocalAddressesResult, len(lAddrs))
	for idx, entry := range lAddrs {
		addr := types.LocalAddressesResult{
			Address: entry.Address,
			Port:    entry.Port,
		}
		localAddrs[idx] = addr
	}

	info := types.GetNetworkInfoResult{
		Version: int32(1000000*version.Major + 10000*version.Minor +
			100*version.Patch),
		SubVersion:      s.cfg.UserAgentVersion,
		ProtocolVersion: int32(s.cfg.MaxProtocolVersion),
		TimeOffset:      int64(s.cfg.TimeSource.Offset().Seconds()),
		Connections:     s.cfg.ConnMgr.ConnectedCount(),
		RelayFee:        s.cfg.MinRelayTxFee.ToCoin(),
		Networks:        s.cfg.NetInfo,
		LocalAddresses:  localAddrs,
		LocalServices:   fmt.Sprintf("%016x", uint64(s.cfg.Services)),
	}

	return info, nil
}

// handleGetPeerInfo implements the getpeerinfo command.
func handleGetPeerInfo(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	peers := s.cfg.ConnMgr.ConnectedPeers()
	syncPeerID := s.cfg.SyncMgr.SyncPeerID()
	infos := make([]*types.GetPeerInfoResult, 0, len(peers))
	for _, p := range peers {
		statsSnap := p.StatsSnapshot()
		var addrLocalStr string
		if addrLocal := p.LocalAddr(); addrLocal != nil {
			addrLocalStr = addrLocal.String()
		}
		info := &types.GetPeerInfoResult{
			ID:             statsSnap.ID,
			Addr:           statsSnap.Addr,
			AddrLocal:      addrLocalStr,
			Services:       fmt.Sprintf("%08d", uint64(statsSnap.Services)),
			RelayTxes:      !p.IsTxRelayDisabled(),
			LastSend:       statsSnap.LastSend.Unix(),
			LastRecv:       statsSnap.LastRecv.Unix(),
			BytesSent:      statsSnap.BytesSent,
			BytesRecv:      statsSnap.BytesRecv,
			ConnTime:       statsSnap.ConnTime.Unix(),
			PingTime:       float64(statsSnap.LastPingMicros),
			TimeOffset:     statsSnap.TimeOffset,
			Version:        statsSnap.Version,
			SubVer:         statsSnap.UserAgent,
			Inbound:        statsSnap.Inbound,
			StartingHeight: statsSnap.StartingHeight,
			CurrentHeight:  statsSnap.LastBlock,
			BanScore:       int32(p.BanScore()),
			SyncNode:       p.ID() == syncPeerID,
		}
		if p.LastPingNonce() != 0 {
			wait := float64(s.cfg.Clock.Since(statsSnap.LastPingTime).Nanoseconds())
			// We actually want microseconds.
			info.PingWait = wait / 1000
		}
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].ID < infos[j].ID
	})
	return infos, nil
}

// handleGetRawMempool implements the getrawmempool command.
func handleGetRawMempool(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetRawMempoolCmd)

	// Choose the type to filter the results by based on the provided param.
	// A filter type of nil means no filtering.
	var filterType *stake.TxType
	if c.TxType != nil {
		switch types.GetRawMempoolTxTypeCmd(*c.TxType) {
		case types.GRMRegular:
			filterType = new(stake.TxType)
			*filterType = stake.TxTypeRegular
		case types.GRMTickets:
			filterType = new(stake.TxType)
			*filterType = stake.TxTypeSStx
		case types.GRMVotes:
			filterType = new(stake.TxType)
			*filterType = stake.TxTypeSSGen
		case types.GRMRevocations:
			filterType = new(stake.TxType)
			*filterType = stake.TxTypeSSRtx
		case types.GRMTSpend:
			filterType = new(stake.TxType)
			*filterType = stake.TxTypeTSpend
		case types.GRMTAdd:
			filterType = new(stake.TxType)
			*filterType = stake.TxTypeTAdd
		case types.GRMAll:
			// Nothing to do
		default:
			supported := []types.GetRawMempoolTxTypeCmd{types.GRMRegular,
				types.GRMTickets, types.GRMVotes, types.GRMRevocations,
				types.GRMTSpend, types.GRMTAdd, types.GRMAll}
			return nil, rpcInvalidError("Invalid transaction type: %s -- "+
				"supported types: %v", *c.TxType, supported)
		}
	}

	// Return verbose results if requested.
	if c.Verbose != nil && *c.Verbose {
		descs := s.cfg.TxMempooler.VerboseTxDescs()
		result := make(map[string]*types.GetRawMempoolVerboseResult, len(descs))
		for i := range descs {
			desc := descs[i]
			if filterType != nil && desc.Type != *filterType {
				continue
			}

			tx := desc.Tx
			mpd := &types.GetRawMempoolVerboseResult{
				Size:             int32(tx.MsgTx().SerializeSize()),
				Fee:              dcrutil.Amount(desc.Fee).ToCoin(),
				Time:             desc.Added.Unix(),
				Height:           desc.Height,
				StartingPriority: 0,
				CurrentPriority:  0,
				Depends:          make([]string, len(desc.Depends)),
			}
			for j, depDesc := range desc.Depends {
				mpd.Depends[j] = depDesc.Tx.Hash().String()
			}

			result[tx.Hash().String()] = mpd
		}

		return result, nil
	}

	// The response is simply an array of the transaction hashes if the
	// verbose flag is not set.
	descs := s.cfg.TxMempooler.TxDescs()
	hashStrings := make([]string, 0, len(descs))
	for i := range descs {
		if filterType != nil && descs[i].Type != *filterType {
			continue
		}
		hashStrings = append(hashStrings, descs[i].Tx.Hash().String())
	}
	return hashStrings, nil
}

// handleGetRawTransaction implements the getrawtransaction command.
func handleGetRawTransaction(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetRawTransactionCmd)

	// Convert the provided transaction hash hex to a Hash.
	txHash, err := chainhash.NewHashFromStr(c.Txid)
	if err != nil {
		return nil, rpcDecodeHexError(c.Txid)
	}

	verbose := false
	if c.Verbose != nil {
		verbose = *c.Verbose != 0
	}

	// Try to fetch the transaction from the memory pool and if that fails,
	// try the block database.
	var mtx *wire.MsgTx
	var blkHash *chainhash.Hash
	var blkHeight int64
	var blkIndex uint32
	chain := s.cfg.Chain
	txIndex := s.cfg.TxIndexer
	tx, err := s.cfg.TxMempooler.FetchTransaction(txHash)
	if err != nil {
		if txIndex == nil {
			err := errors.New("the transaction index must be enabled to " +
				"query the blockchain (specify --txindex)")
			return nil, rpcInternalErr(err, "Configuration")
		}

		// Ensure the tx index is synced.
		tHeight, tHash, err := txIndex.Tip()
		if err != nil {
			return nil, rpcInternalErr(err, "Tip")
		}

		chain := s.cfg.Chain

		// Return an out-of-sync error if index is lagging a
		// maximum reorg depth (6) blocks or more from the chain tip.
		if chain.BestSnapshot().Height > (tHeight + 5) {
			err := fmt.Errorf("%s: index not synced", txIndex.Name())
			return nil, rpcInternalErr(err, "Sync")
		}

	sync:
		for !chain.BestSnapshot().Hash.IsEqual(tHash) {
			select {
			case <-time.After(syncWait):
				err := fmt.Errorf("%s: index not synced", txIndex.Name())
				return nil, rpcInternalErr(err, "Sync")
			case <-txIndex.WaitForSync():
				break sync
			}
		}

		// Look up the location of the transaction.
		idxEntry, err := txIndex.Entry(txHash)
		if err != nil {
			const context = "Failed to retrieve transaction location"
			return nil, rpcInternalErr(err, context)
		}
		if idxEntry == nil {
			return nil, rpcNoTxInfoError(txHash)
		}
		blockRegion := &idxEntry.BlockRegion

		// Load the raw transaction bytes from the database.
		var txBytes []byte
		err = s.cfg.DB.View(func(dbTx database.Tx) error {
			var err error
			txBytes, err = dbTx.FetchBlockRegion(blockRegion)
			return err
		})
		if err != nil {
			return nil, rpcNoTxInfoError(txHash)
		}

		// When the verbose flag isn't set, simply return the serialized
		// transaction as a hex-encoded string.  This is done here to
		// avoid deserializing it only to reserialize it again later.
		if !verbose {
			return hex.EncodeToString(txBytes), nil
		}

		// Grab the block details.
		blkHash = blockRegion.Hash
		blkHeight, err = chain.BlockHeightByHash(blkHash)
		if err != nil {
			return nil, rpcInternalErr(err, "Failed to retrieve block height")
		}
		blkIndex = idxEntry.BlockIndex

		// Deserialize the transaction
		var msgTx wire.MsgTx
		err = msgTx.Deserialize(bytes.NewReader(txBytes))
		if err != nil {
			return nil, rpcInternalErr(err, "Failed to deserialize transaction")
		}
		mtx = &msgTx
	} else {
		// When the verbose flag isn't set, simply return the
		// network-serialized transaction as a hex-encoded string.
		if !verbose {
			// Note that this is intentionally not directly
			// returning because the first return value is a
			// string and it would result in returning an empty
			// string to the client instead of nothing (nil) in the
			// case of an error.
			mtxHex, err := s.messageToHex(tx.MsgTx())
			if err != nil {
				return nil, err
			}
			return mtxHex, nil
		}

		mtx = tx.MsgTx()
	}

	// The verbose flag is set, so generate the JSON object and return it.
	var (
		blkHeader     *wire.BlockHeader
		prevBlkHash   chainhash.Hash
		blkHashStr    string
		confirmations int64
	)
	if blkHash != nil {
		// Fetch the header from chain.
		header, err := chain.HeaderByHash(blkHash)
		if err != nil {
			return nil, rpcInternalErr(err, "Failed to fetch block header")
		}

		blkHeader = &header
		prevBlkHash = header.PrevBlock
		blkHashStr = blkHash.String()
		confirmations = 1 + chain.BestSnapshot().Height - blkHeight
	} else {
		// The transaction was obtained from the mempool when there is no block
		// hash set, so the previous block hash is the current best chain tip in
		// that case.
		prevBlkHash = chain.BestSnapshot().Hash
	}

	// Determine if the treasury rules are active as of either the block the
	// contains the transaction or the current best tip when it is in the
	// mempool.
	isTreasuryEnabled, err := s.isTreasuryAgendaActive(&prevBlkHash)
	if err != nil {
		return nil, rpcInternalErr(err, "Treasury Status")
	}

	rawTxn, err := s.createTxRawResult(s.cfg.ChainParams, mtx, txHash.String(),
		blkIndex, blkHeader, blkHashStr, blkHeight, confirmations,
		isTreasuryEnabled)
	if err != nil {
		return nil, err
	}
	return *rawTxn, nil
}

// handleGetStakeDifficulty implements the getstakedifficulty command.
func handleGetStakeDifficulty(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	chain := s.cfg.Chain
	best := chain.BestSnapshot()
	blockHeader, err := chain.HeaderByHeight(best.Height)
	if err != nil {
		log.Errorf("Error getting block: %v", err)
		return nil, &dcrjson.RPCError{
			Code:    dcrjson.ErrRPCDifficulty,
			Message: "Error getting stake difficulty: " + err.Error(),
		}
	}
	result := types.GetStakeDifficultyResult{
		CurrentStakeDifficulty: dcrutil.Amount(blockHeader.SBits).ToCoin(),
		NextStakeDifficulty:    dcrutil.Amount(best.NextStakeDiff).ToCoin(),
	}
	return result, nil
}

// convertVersionMap translates a map[int]int into a sorted array of
// VersionCount that contains the same information.
func convertVersionMap(m map[int]int) []types.VersionCount {
	sorted := make([]types.VersionCount, 0, len(m))
	order := make([]int, 0, len(m))
	for k := range m {
		order = append(order, k)
	}
	sort.Ints(order)

	for _, v := range order {
		sorted = append(sorted, types.VersionCount{Version: uint32(v),
			Count: uint32(m[v])})
	}

	return sorted
}

// handleGetStakeVersionInfo implements the getstakeversioninfo command.
func handleGetStakeVersionInfo(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetStakeVersionInfoCmd)

	chain := s.cfg.Chain
	snapshot := chain.BestSnapshot()
	interval := s.cfg.ChainParams.StakeVersionInterval

	count := int32(1)
	if c.Count != nil {
		count = *c.Count
		if count <= 0 {
			return nil, rpcInvalidError("Count must be > 0")
		}

		// Limit the count to the total possible available intervals.
		totalIntervals := (snapshot.Height + interval - 1) / interval
		if int64(count) > totalIntervals {
			count = int32(totalIntervals)
		}
	}

	// Assemble JSON result.
	result := types.GetStakeVersionInfoResult{
		CurrentHeight: snapshot.Height,
		Hash:          snapshot.Hash.String(),
		Intervals:     make([]types.VersionInterval, 0, count),
	}

	startHeight := snapshot.Height
	endHeight := chain.CalcWantHeight(interval, snapshot.Height) + 1
	hash := &snapshot.Hash
	adjust := int32(1) // We are off by one on the initial iteration.
	for i := int32(0); i < count; i++ {
		numBlocks := int32(startHeight - endHeight)
		if numBlocks <= 0 {
			// Just return what we got.
			break
		}
		sv, err := chain.GetStakeVersions(hash, numBlocks+adjust)
		if err != nil {
			context := fmt.Sprintf("Failed to get stake versions starting "+
				"from hash %v", hash)
			return nil, rpcInternalErr(err, context)
		}

		posVersions := make(map[int]int)
		voteVersions := make(map[int]int)
		for _, v := range sv {
			posVersions[int(v.StakeVersion)]++
			for _, vote := range v.Votes {
				voteVersions[int(vote.Version)]++
			}
		}
		versionInterval := types.VersionInterval{
			StartHeight:  endHeight,
			EndHeight:    startHeight,
			PoSVersions:  convertVersionMap(posVersions),
			VoteVersions: convertVersionMap(voteVersions),
		}
		result.Intervals = append(result.Intervals, versionInterval)

		// Adjust interval.
		endHeight -= interval
		startHeight = endHeight + interval
		adjust = 0

		// Get prior block hash.
		hash, err = chain.BlockHashByHeight(startHeight - 1)
		if err != nil {
			context := fmt.Sprintf("Failed to get block hash for height %d",
				startHeight-1)
			return nil, rpcInternalErr(err, context)
		}
	}

	return result, nil
}

// handleGetStakeVersions implements the getstakeversions command.
func handleGetStakeVersions(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetStakeVersionsCmd)

	hash, err := chainhash.NewHashFromStr(c.Hash)
	if err != nil {
		return nil, rpcDecodeHexError(c.Hash)
	}
	if c.Count <= 0 {
		return nil, rpcInvalidError("Invalid parameter, count must " +
			"be > 0")
	}

	sv, err := s.cfg.Chain.GetStakeVersions(hash, c.Count)
	if err != nil {
		return nil, rpcInternalErr(err, "Could not obtain stake versions")
	}

	result := types.GetStakeVersionsResult{
		StakeVersions: make([]types.StakeVersions, 0, len(sv)),
	}
	for _, v := range sv {
		nsv := types.StakeVersions{
			Hash:         v.Hash.String(),
			Height:       v.Height,
			BlockVersion: v.BlockVersion,
			StakeVersion: v.StakeVersion,
			Votes: make([]types.VersionBits, 0,
				len(v.Votes)),
		}
		for _, vote := range v.Votes {
			nsv.Votes = append(nsv.Votes,
				types.VersionBits{Version: vote.Version, Bits: vote.Bits})
		}

		result.StakeVersions = append(result.StakeVersions, nsv)
	}

	return result, nil
}

// handleGetTicketPoolValue implements the getticketpoolvalue command.
func handleGetTicketPoolValue(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	amt, err := s.cfg.Chain.TicketPoolValue()
	if err != nil {
		return nil, rpcInternalErr(err, "Could not obtain ticket pool value")
	}

	return amt.ToCoin(), nil
}

// handleGetTreasuryBalance implements the gettreasurybalance command.
func handleGetTreasuryBalance(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetTreasuryBalanceCmd)

	// Either parse the provided hash or use the current best tip hash when none
	// is provided.
	var hash chainhash.Hash
	if c.Hash == nil || *c.Hash == "" {
		hash = s.cfg.Chain.BestSnapshot().Hash
	} else {
		parsedHash, err := chainhash.NewHashFromStr(*c.Hash)
		if err != nil {
			return nil, rpcDecodeHexError(*c.Hash)
		}
		hash = *parsedHash
	}

	balanceInfo, err := s.cfg.Chain.TreasuryBalance(&hash)
	if err != nil {
		switch {
		case errors.Is(err, blockchain.ErrUnknownBlock):
			return nil, &dcrjson.RPCError{
				Code:    dcrjson.ErrRPCBlockNotFound,
				Message: fmt.Sprintf("Block not found: %s", hash),
			}

		case errors.Is(err, blockchain.ErrNoTreasuryBalance):
			return nil, &dcrjson.RPCError{
				Code:    dcrjson.ErrRPCNoTreasury,
				Message: fmt.Sprintf("Treasury inactive for block %s", hash),
			}
		}

		return nil, rpcInternalErr(err, "Failed to obtain treasury balance")
	}

	tbr := types.GetTreasuryBalanceResult{
		Hash:    hash.String(),
		Height:  balanceInfo.BlockHeight,
		Balance: balanceInfo.Balance,
	}
	if c.Verbose != nil && *c.Verbose {
		tbr.Updates = balanceInfo.Updates
	}
	return tbr, nil
}

// handleGetTreasurySpendVotes implements the gettreasuryspendvotes command.
func handleGetTreasurySpendVotes(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetTreasurySpendVotesCmd)

	// Shorter version of relevant parameters.
	chain := s.cfg.Chain
	tvi := s.cfg.ChainParams.TreasuryVoteInterval
	mul := s.cfg.ChainParams.TreasuryVoteIntervalMultiplier
	mempool := s.cfg.TxMempooler

	// Either parse the provided hash or use the current best tip hash when
	// none is provided.
	var block chainhash.Hash
	var blockHeight int64
	var checkingMainChain bool
	if c.Block == nil || *c.Block == "" {
		best := s.cfg.Chain.BestSnapshot()
		block = best.Hash
		blockHeight = best.Height
		checkingMainChain = true
	} else {
		if err := chainhash.Decode(&block, *c.Block); err != nil {
			return nil, rpcDecodeHexError(*c.Block)
		}

		// Using HeaderByHash allows querying both the mainchain and
		// any sidechains.
		hdr, err := chain.HeaderByHash(&block)
		if err != nil {
			return nil, rpcBlockNotFoundError(block)
		}

		blockHeight = int64(hdr.Height)
		checkingMainChain = chain.MainChainHasBlock(&block)
	}

	// When tallying votes on mainchain and for mined tspends, we'll only
	// count votes up to when the tspend was mined. Thus we need to
	// maintain some local information to be able to correctly identify the
	// ending block for those types of tspends.
	endBlocks := make(map[chainhash.Hash]chainhash.Hash)

	// Determine whether to use the specified tspends or all the ones in
	// the mempool.
	var tspends []*dcrutil.Tx
	if c.TSpends != nil && len(*c.TSpends) > 0 {
		// Using client-specified tspends, they may be in the mempool,
		// mined, or completely unknown, so handle each case.
		tspends = make([]*dcrutil.Tx, len(*c.TSpends))
		for i, s := range *c.TSpends {
			var hash chainhash.Hash
			if err := chainhash.Decode(&hash, s); err != nil {
				return nil, rpcDecodeHexError(s)
			}

			// Check if this tspend is in the mempool.
			var err error
			tspends[i], err = mempool.FetchTransaction(&hash)
			if err == nil {
				// Sanity check this is actually a tspend.
				if !stake.IsTSpend(tspends[i].MsgTx()) {
					return nil, rpcInvalidError("mempool tx %s "+
						"is not a tspend", hash)
				}
				continue
			}

			// Not in the mempool. Check if it is mined.
			blocks, err := chain.FetchTSpend(hash)
			if err != nil || len(blocks) == 0 {
				// TSpend does not exist mined or in mempool.
				return nil, rpcNoTxInfoError(&hash)
			}

			// TSpend exists mined in at least one block. Fetch the
			// first one and extract the tspend.
			fullBlock, err := chain.BlockByHash(&blocks[0])
			if err != nil {
				// Shouldn't happen unless tspend db is hosed.
				const context = "Block containing mined treasury spend not found"
				return nil, rpcInternalErr(err, context)
			}

			// TSpends live in the stake tree.
			var found bool
			for _, tx := range fullBlock.STransactions() {
				if tx.Hash().IsEqual(&hash) {
					tspends[i] = tx
					found = true
					break
				}
			}
			if !found {
				// Shouldn't happen unless tspend db is hosed
				// or the assumption about tspends living in
				// the stake tree is wrong.
				const context = "Block did not contain treasury spend tx in " +
					"stake tree"
				return nil, rpcInternalErr(err, context)
			}

			// If we're meant to tally main chain votes, figure out
			// which (if any) of the blocks the tspend is found are
			// in the main chain so we can count only up to that
			// block (since it doesn't make sense to count
			// additional votes _after_ the tspend was mined). This
			// doesn't apply if we're not tallying main chain votes
			// because we don't know the relationship between the
			// requested branch and the branches that include the
			// tspend so we just use the requested end block.
			if !checkingMainChain {
				continue
			}
			for i := range blocks {
				blockHash := &blocks[i]
				if !chain.MainChainHasBlock(blockHash) {
					continue
				}

				// Fetch the header to discover this block's
				// height.
				hdr, err := chain.HeaderByHash(blockHash)
				if err != nil {
					// Shouldn't happen.
					const context = "Block without associated header"
					return nil, rpcInternalErr(err, context)
				}

				// Given this tspend was mined in the main
				// chain, it doesn't make sense to count votes
				// after it was mined. So stop early if the
				// target block height is greater than or equal
				// to the tspend's mined height. We need to
				// count votes only up to the block _before_
				// the tspend was mined.
				if blockHeight >= int64(hdr.Height) {
					endBlocks[hash] = hdr.PrevBlock
				}

				break
			}
		}
	} else {
		// Fetch vote counts for all mempool tspends.
		hashes := mempool.TSpendHashes()
		tspends = make([]*dcrutil.Tx, len(hashes))
		for i, h := range hashes {
			var err error
			tspends[i], err = mempool.FetchTransaction(&h)
			if err != nil {
				const context = "Could not fetch treasury spend from mempool"
				return nil, rpcInternalErr(err, context)
			}
		}
	}

	// Fetch the vote counts from the blockchain.
	votes := make([]types.TreasurySpendVotes, len(tspends))
	for i, tx := range tspends {
		txHash := tx.Hash()

		// Early check to ensure this tx has a valid expiry and other
		// functions will behave properly.
		expiry := tx.MsgTx().Expiry
		if !standalone.IsTreasuryVoteInterval(uint64(expiry-2), tvi) {
			err := fmt.Errorf("treasury spend %s has incorrect expiry %d",
				tx.Hash(), expiry)
			const context = "Treasury spend without correct expiry"
			return nil, rpcInternalErr(err, context)
		}

		// We only count votes for tspends that are inside their voting
		// window. Otherwise we just return the appropriate vote start
		// and end heights for it.
		var yes, no int64
		insideWindow := standalone.InsideTSpendWindow(blockHeight, expiry, tvi, mul)
		minedBlock, isMined := endBlocks[*txHash]
		if insideWindow || isMined {
			// Determine whether to use the originally requested
			// stop block or a custom one in case of mainchain
			// mined tspends.
			checkBlock := block
			if isMined {
				checkBlock = minedBlock
			}

			var err error
			yes, no, err = chain.TSpendCountVotes(&checkBlock, tx)
			if err != nil {
				if errors.Is(err, blockchain.ErrUnknownBlock) {
					return nil, rpcBlockNotFoundError(block)
				}

				const context = "Failed to obtain treasury spend votes"
				return nil, rpcInternalErr(err, context)
			}
		}

		// The following error can be ignored because the expiry was verified to
		// be in a TVI earlier.
		start, end, _ := standalone.CalcTSpendWindow(expiry, tvi, mul)

		votes[i] = types.TreasurySpendVotes{
			Hash:      txHash.String(),
			Expiry:    int64(expiry),
			VoteStart: int64(start),
			VoteEnd:   int64(end),
			YesVotes:  yes,
			NoVotes:   no,
		}
	}

	return types.GetTreasurySpendVotesResult{
		Hash:   block.String(),
		Height: blockHeight,
		Votes:  votes,
	}, nil
}

// handleGetVoteInfo implements the getvoteinfo command.
func handleGetVoteInfo(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetVoteInfoCmd)

	// Shorter versions of some parameters for convenience.
	interval := int64(s.cfg.ChainParams.RuleChangeActivationInterval)
	quorum := s.cfg.ChainParams.RuleChangeActivationQuorum
	chain := s.cfg.Chain
	snapshot := chain.BestSnapshot()

	vi, err := chain.GetVoteInfo(&snapshot.Hash, c.Version)
	if err != nil {
		if errors.Is(err, blockchain.ErrUnknownDeploymentVersion) {
			return nil, rpcInvalidError("%d: unrecognized vote version",
				c.Version)
		}
		return nil, rpcInternalErr(err, "Could not obtain vote info")
	}

	// Assemble JSON result.
	result := types.GetVoteInfoResult{
		CurrentHeight: snapshot.Height,
		StartHeight:   chain.CalcWantHeight(interval, snapshot.Height) + 1,
		EndHeight:     chain.CalcWantHeight(interval, snapshot.Height) + interval,
		Hash:          snapshot.Hash.String(),
		VoteVersion:   c.Version,
		Quorum:        quorum,
	}

	// We don't fail, we try to return the totals for this version.
	result.TotalVotes, err = chain.CountVoteVersion(c.Version)
	if err != nil {
		return nil, rpcInternalErr(err, "Could not count voter versions")
	}

	result.Agendas = make([]types.Agenda, 0, len(vi.Agendas))
	for _, agenda := range vi.Agendas {
		// Obtain status of agenda.
		state, err := chain.NextThresholdState(&snapshot.Hash, agenda.Vote.Id)
		if err != nil {
			const context = "Could not fetch next threshold state"
			return nil, rpcInternalErr(err, context)
		}

		a := types.Agenda{
			ID:          agenda.Vote.Id,
			Description: agenda.Vote.Description,
			Mask:        agenda.Vote.Mask,
			Choices:     make([]types.Choice, 0, len(agenda.Vote.Choices)),
			StartTime:   agenda.StartTime,
			ExpireTime:  agenda.ExpireTime,
			Status:      thresholdStateToAgendaStatus(state),
		}

		// Handle choices.
		for _, choice := range agenda.Vote.Choices {
			a.Choices = append(a.Choices, types.Choice{
				ID:          choice.Id,
				Description: choice.Description,
				Bits:        choice.Bits,
				IsAbstain:   choice.IsAbstain,
				IsNo:        choice.IsNo,
			})
		}

		if state.State != blockchain.ThresholdStarted {
			// Append transformed agenda without progress.
			result.Agendas = append(result.Agendas, a)
			continue
		}

		counts, err := s.cfg.Chain.GetVoteCounts(c.Version, agenda.Vote.Id)
		if err != nil {
			return nil, rpcInternalErr(err, "Could not obtain vote count")
		}

		// Calculate quorum.
		qmin := quorum
		totalNonAbstain := counts.Total - counts.TotalAbstain
		if totalNonAbstain < quorum {
			qmin = totalNonAbstain
		}
		a.QuorumProgress = float64(qmin) / float64(quorum)

		// Calculate choice progress.
		for k := range a.Choices {
			a.Choices[k].Count = counts.VoteChoices[k]
			a.Choices[k].Progress = float64(counts.VoteChoices[k]) /
				float64(counts.Total)
		}

		// Append transformed agenda.
		result.Agendas = append(result.Agendas, a)
	}

	return result, nil
}

// bigToLEUint256 returns the passed big integer as an unsigned 256-bit integer
// encoded as little-endian bytes.  Numbers which are larger than the max
// unsigned 256-bit integer are truncated.
func bigToLEUint256(n *big.Int) [uint256Size]byte {
	// Pad or truncate the big-endian big int to correct number of bytes.
	nBytes := n.Bytes()
	nlen := len(nBytes)
	pad := 0
	start := 0
	if nlen <= uint256Size {
		pad = uint256Size - nlen
	} else {
		start = nlen - uint256Size
	}
	var buf [uint256Size]byte
	copy(buf[pad:], nBytes[start:])

	// Reverse the bytes to little endian and return them.
	for i := 0; i < uint256Size/2; i++ {
		buf[i], buf[uint256Size-1-i] = buf[uint256Size-1-i], buf[i]
	}
	return buf
}

// handleGetTxOut handles gettxout commands.
func handleGetTxOut(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.GetTxOutCmd)

	// Convert the provided transaction hash hex to a Hash.
	txHash, err := chainhash.NewHashFromStr(c.Txid)
	if err != nil {
		return nil, rpcDecodeHexError(c.Txid)
	}

	if !(c.Tree == wire.TxTreeRegular || c.Tree == wire.TxTreeStake) {
		return nil, rpcInvalidError("Tx tree must be regular or stake")
	}

	chain := s.cfg.Chain
	best := chain.BestSnapshot()

	// If requested and the tx is available in the mempool try to fetch it
	// from there, otherwise attempt to fetch from the block database.
	var bestBlockHash string
	var confirmations int64
	var value int64
	var scriptVersion uint16
	var pkScript []byte
	var isCoinbase bool
	var isTreasuryEnabled bool
	includeMempool := true
	if c.IncludeMempool != nil {
		includeMempool = *c.IncludeMempool
	}
	var txFromMempool *dcrutil.Tx
	if includeMempool {
		txFromMempool, _ = s.cfg.TxMempooler.FetchTransaction(txHash)

		// Set as nil if the tx tree does not match the tree param that was passed.
		// This is set as nil rather than returning immediately here since it is
		// technically possible (though extremely unlikely) that the tx exists
		// elsewhere, so it should still continue and check elsewhere below.
		if txFromMempool != nil && txFromMempool.Tree() != c.Tree {
			txFromMempool = nil
		}
	}
	if txFromMempool != nil {
		mtx := txFromMempool.MsgTx()
		if c.Vout > uint32(len(mtx.TxOut)-1) {
			return nil, &dcrjson.RPCError{
				Code: dcrjson.ErrRPCInvalidTxVout,
				Message: "Output index number (vout) does not " +
					"exist for transaction.",
			}
		}

		txOut := mtx.TxOut[c.Vout]
		if txOut == nil {
			err := fmt.Errorf("output index: %d for txid: %s does not exist",
				c.Vout, txHash)
			return nil, rpcInternalErr(err, "")
		}

		// The transaction output in question is from the mempool, so determine
		// if the treasury rules are active from the point of view of the
		// current best tip.
		isTreasuryEnabled, err = s.isTreasuryAgendaActive(&best.PrevHash)
		if err != nil {
			return nil, err
		}

		bestBlockHash = best.Hash.String()
		confirmations = 0
		value = txOut.Value
		scriptVersion = txOut.Version
		pkScript = txOut.PkScript
		isCoinbase = standalone.IsCoinBaseTx(mtx, isTreasuryEnabled)
	} else {
		outpoint := wire.OutPoint{Hash: *txHash, Index: c.Vout, Tree: c.Tree}
		entry, err := chain.FetchUtxoEntry(outpoint)
		if err != nil {
			return nil, rpcInternalErr(err, "Failed to retrieve utxo entry")
		}

		// To match the behavior of the reference client, return nil
		// (JSON null) if the transaction output could not be found
		// (never existed or was pruned) or is spent by another
		// transaction already in the main chain.  Mined transactions
		// that are spent by a mempool transaction are not affected by
		// this.
		if entry == nil || entry.IsSpent() {
			return nil, nil
		}

		bestBlockHash = best.Hash.String()
		confirmations = 1 + best.Height - entry.BlockHeight()
		value = entry.Amount()
		scriptVersion = entry.ScriptVersion()
		pkScript = entry.PkScript()
		isCoinbase = entry.IsCoinBase()
	}

	// Disassemble script into single line printable format.  The
	// disassembled string will contain [error] inline if the script
	// doesn't fully parse, so ignore the error here.
	script := pkScript
	disbuf, _ := txscript.DisasmString(script)

	// Attempt to extract known addresses associated with the script.
	scriptType, addrs := stdscript.ExtractAddrs(scriptVersion, script,
		s.cfg.ChainParams)
	addresses := make([]string, len(addrs))
	for i, addr := range addrs {
		addresses[i] = addr.String()
	}

	// Determine the number of required signatures for known standard types.
	reqSigs := stdscript.DetermineRequiredSigs(scriptVersion, script)

	txOutReply := &types.GetTxOutResult{
		BestBlock:     bestBlockHash,
		Confirmations: confirmations,
		Value:         dcrutil.Amount(value).ToUnit(dcrutil.AmountCoin),
		ScriptPubKey: types.ScriptPubKeyResult{
			Asm:       disbuf,
			Hex:       hex.EncodeToString(pkScript),
			ReqSigs:   int32(reqSigs),
			Type:      scriptType.String(),
			Addresses: addresses,
			Version:   scriptVersion,
		},
		Coinbase: isCoinbase,
	}
	return txOutReply, nil
}

// handleGetTxOutSetInfo returns statistics on the current unspent transaction output set.
func handleGetTxOutSetInfo(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	best := s.cfg.Chain.BestSnapshot()
	stats, err := s.cfg.Chain.FetchUtxoStats()
	if err != nil {
		return nil, err
	}

	return types.GetTxOutSetInfoResult{
		Height:         best.Height,
		BestBlock:      best.Hash.String(),
		Transactions:   stats.Transactions,
		TxOuts:         stats.Utxos,
		DiskSize:       stats.Size,
		TotalAmount:    stats.Total,
		SerializedHash: stats.SerializedHash.String(),
	}, nil
}

// pruneOldBlockTemplates prunes all old block templates from the templatePool
// map.
//
// This function MUST be called with the RPC workstate locked.
func (s *workState) pruneOldBlockTemplates(bestHeight int64) {
	pruneHeight := bestHeight - getworkExpirationDiff
	for key, block := range s.templatePool {
		height := int64(block.Header.Height)
		if height < pruneHeight {
			delete(s.templatePool, key)
		}
	}
}

// getWorkTemplateKey returns the key to use for the template pool that houses
// the information necessary to construct full blocks from getwork submissions.
func getWorkTemplateKey(header *wire.BlockHeader) [merkleRootPairSize]byte {
	// Create the new template key which is the MerkleRoot + StakeRoot fields.
	// Note that with DCP0005 active, the stake root field will be merged with
	// the merkle root field and the stake root field will be a commitment
	// root.  Since the commitment root changes with the contents of the block
	// it will still be a good piece of information to add to the key.
	var merkleRootPair [merkleRootPairSize]byte
	copy(merkleRootPair[:chainhash.HashSize], header.MerkleRoot[:])
	copy(merkleRootPair[chainhash.HashSize:], header.StakeRoot[:])
	return merkleRootPair
}

// serializeGetWorkData returns serialized data that represents work to be
// solved and is used in the getwork RPC and notifywork WebSocket notification.
// It consists of the serialized block header along with any internal padding
// that makes the data ready for callers to make use of only the final chunk
// along with the midstate for the rest when solving the block.
func serializeGetWorkData(header *wire.BlockHeader, isBlake3PowActive bool) ([]byte, error) {
	// Choose the full buffer data length as well as internal padding to apply
	// based on the result of the vote for the blake3 proof of work agenda.
	getworkDataLen := getworkDataLenBlake256
	internalHashFuncPad := blake256Pad
	if isBlake3PowActive {
		getworkDataLen = getworkDataLenBlake3
		internalHashFuncPad = blake3Pad
	}

	// Serialize the block header into a buffer large enough to hold the block
	// header and any internal padding that is added and returned as part of the
	// data below.
	//
	// For reference (0-index based, end value is exclusive):
	// data[115:119] --> Bits
	// data[135:139] --> Timestamp
	// data[139:143] --> Nonce
	// data[143:151] --> ExtraNonce
	data := make([]byte, 0, getworkDataLen)
	buf := bytes.NewBuffer(data)
	err := header.Serialize(buf)
	if err != nil {
		return nil, rpcInternalErr(err, "Failed to serialize data")
	}

	// Expand the data slice to include the full data buffer and apply internal
	// padding for the hash function.  This makes the data ready for callers to
	// make use of only the final chunk along with the midstate for the rest.
	data = data[:getworkDataLen]
	copy(data[wire.MaxBlockHeaderPayload:], internalHashFuncPad)
	return data, nil
}

// handleGetWorkRequest is a helper for handleGetWork which deals with
// generating and returning work to the caller.
func handleGetWorkRequest(ctx context.Context, s *Server) (interface{}, error) {
	// Return an error immediately in the case of a failed background template.
	//
	// The only time this is expected is due to imposition of certain additional
	// time-based rules such as when the maximum allowed difficulty on testnet
	// is reached and not enough time has passed since the last block.
	bt := s.cfg.BlockTemplater
	if _, err := bt.CurrentTemplate(); err != nil {
		return nil, rpcMiscError(fmt.Sprintf("no work is available: %v", err))
	}

	// Prune old templates when the current best chain tip changes.
	best := s.cfg.Chain.BestSnapshot()
	state := s.workState
	state.Lock()
	tipChanged := state.prevBestHash == nil || *state.prevBestHash != best.Hash
	if tipChanged {
		state.pruneOldBlockTemplates(best.Height)
		state.prevBestHash = &best.Hash
		state.waitForUpdatedTemplate = true
	}
	waitForNewTemplate := state.waitForUpdatedTemplate
	state.Unlock()

	// Wait for updated templates when the current best chain tip changes.
	var template *mining.BlockTemplate
	if waitForNewTemplate {
		// Wait until a new template is generated.  Since the subscription
		// immediately sends the current template, the template might not have
		// been updated yet (for example, it might be waiting on votes).  In
		// that case, wait for the updated template with an eventual timeout
		// in case the new tip never gets enough votes and no other events
		// that trigger a new template have happened.
		templateSub := bt.Subscribe()
		var templateNtfn *mining.TemplateNtfn
		select {
		case templateNtfn = <-templateSub.C():
		case <-ctx.Done():
			templateSub.Stop()
			return nil, rpcConnectionClosedError()
		}
		template = templateNtfn.Template
		templateKey := getWorkTemplateKey(&template.Block.Header)
		state.Lock()
		_, templateKnown := state.templatePool[templateKey]
		state.Unlock()
		if templateKnown {
			const maxTemplateTimeoutDuration = time.Millisecond * 5500
			select {
			case templateNtfn = <-templateSub.C():
				template = templateNtfn.Template

			case <-time.After(maxTemplateTimeoutDuration):
				template = nil

			case <-ctx.Done():
				templateSub.Stop()
				return nil, rpcConnectionClosedError()
			}
		}
		templateSub.Stop()
	}

	// Grab the current template from the background generator immediately when
	// it was not already obtained above.  Return any errors that might have
	// happened when generating the template.
	if template == nil {
		var err error
		template, err = bt.CurrentTemplate()
		if err != nil {
			const context = "Unable to retrieve work due to invalid template"
			return nil, rpcInternalErr(err, context)
		}
		if template == nil {
			return nil, rpcMiscError("no work is available during a chain " +
				"reorganization")
		}
	}

	// Allow future invocations to immediately return the existing background
	// template now that an acceptable template has been received.
	if waitForNewTemplate {
		state.Lock()
		state.waitForUpdatedTemplate = false
		state.Unlock()
	}

	// Update the time of the block template to the current time while
	// accounting for the median time of the past several blocks per the chain
	// consensus rules.  Note that the header is copied to avoid mutating the
	// shared block template.
	headerCopy := template.Block.Header
	bt.UpdateBlockTime(&headerCopy)

	// Serialize the data that represents work to be solved as well as any
	// internal padding that makes the data ready for callers to make use of
	// only the final chunk along with the midstate for the rest when solving
	// the block.
	isBlake3PowActive, err := s.isBlake3PowAgendaActive(&headerCopy.PrevBlock)
	if err != nil {
		return nil, err
	}
	data, err := serializeGetWorkData(&headerCopy, isBlake3PowActive)
	if err != nil {
		return nil, err
	}

	// Add the template to the template pool.  Since the key is a combination
	// of the merkle and stake root fields, this will not add duplicate entries
	// for the templates with modified timestamps and/or difficulty bits.
	templateKey := getWorkTemplateKey(&headerCopy)
	state.Lock()
	state.templatePool[templateKey] = template.Block
	state.Unlock()

	// The target is in big endian, but it is treated as a uint256 and byte
	// swapped to little endian in the final result.  Even though there is
	// really no reason for it to be swapped, it is a holdover from legacy code
	// and is now required for compatibility.
	target := bigToLEUint256(standalone.CompactToBig(headerCopy.Bits))
	reply := &types.GetWorkResult{
		Data:   hex.EncodeToString(data),
		Target: hex.EncodeToString(target[:]),
	}
	return reply, nil
}

// minInt is a helper function to return the minimum of two ints.  This avoids
// the need to cast to floats.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// maxInt is a helper function to return the maximum of two ints.  This avoids
// the need to cast to floats.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// handleGetWorkSubmission is a helper for handleGetWork which deals with
// the calling submitting work to be verified and processed.
func handleGetWorkSubmission(_ context.Context, s *Server, hexData string) (interface{}, error) {
	// Ensure the provided data is sane.
	minDataLen := minInt(getworkDataLenBlake256, getworkDataLenBlake3)
	maxDataLen := maxInt(getworkDataLenBlake256, getworkDataLenBlake3)
	paddedHexDataLen := len(hexData) + len(hexData)%2
	if paddedHexDataLen < minDataLen*2 || paddedHexDataLen > maxDataLen*2 {
		if minDataLen == maxDataLen {
			return nil, rpcInvalidError("Argument must be a hexadecimal string "+
				"with length %d (not %d)", minDataLen*2, len(hexData))
		}
		return nil, rpcInvalidError("Argument must be a hexadecimal string "+
			"with a length between %d and %d (not %d)", minDataLen*2,
			maxDataLen*2, len(hexData))
	}

	// Decode the provided hex data while padding the front with a 0 for
	// odd-length strings since the decoder requires even-length strings.
	if len(hexData)%2 != 0 {
		hexData = "0" + hexData
	}
	data, err := hex.DecodeString(hexData)
	if err != nil {
		return false, rpcDecodeHexError(hexData)
	}

	// Deserialize the block header from the data.
	var submittedHeader wire.BlockHeader
	bhBuf := bytes.NewReader(data[0:wire.MaxBlockHeaderPayload])
	err = submittedHeader.Deserialize(bhBuf)
	if err != nil {
		return false, rpcInvalidError("Invalid block header: %v", err)
	}

	// Reject orphan blocks.  This is done here to provide nicer feedback about
	// why the block was rejected since attempting to determine the state of a
	// voting agenda requires all previous blocks to be known.
	prevBlkHash := &submittedHeader.PrevBlock
	if _, err := s.cfg.Chain.HeaderByHash(prevBlkHash); err != nil {
		log.Infof("Block submitted via getwork rejected: orphan building on "+
			"parent %v", prevBlkHash)
		return false, nil // nolint: nilerr
	}

	// Choose the proof of work mining algorithm based on the result of the vote
	// for the blake3 proof of work agenda.
	isBlake3PowActive, err := s.isBlake3PowAgendaActive(prevBlkHash)
	if err != nil {
		return false, err
	}
	powHashFn := submittedHeader.PowHashV1
	if isBlake3PowActive {
		powHashFn = submittedHeader.PowHashV2
	}

	// Ensure the submitted proof of work hash is less than the target
	// difficulty.
	powHash := powHashFn()
	err = standalone.CheckProofOfWork(&powHash, submittedHeader.Bits,
		s.cfg.ChainParams.PowLimit)
	if err != nil {
		// Anything other than a rule violation is an unexpected error, so
		// return that error as an internal error.
		var rErr standalone.RuleError
		if !errors.As(err, &rErr) {
			const context = "Unexpected error while checking proof of work"
			return false, rpcInternalErr(err, context)
		}

		log.Errorf("Block submitted via getwork does not meet the "+
			"required proof of work: %v", err)
		return false, nil
	}

	// Look up the full block for the provided data based on the merkle and
	// stake roots.  Return false to indicate the solve failed if it's not
	// available.
	templateKey := getWorkTemplateKey(&submittedHeader)
	s.workState.Lock()
	templateBlock, ok := s.workState.templatePool[templateKey]
	s.workState.Unlock()
	if !ok || templateBlock == nil {
		log.Errorf("Block submitted via getwork has no matching template "+
			"for merkle root %s, stake root %s",
			submittedHeader.MerkleRoot, submittedHeader.StakeRoot)
		return false, nil
	}

	// Reconstruct the block using the submitted header stored block info.  Note
	// that the block template is shallow copied to avoid mutating the header
	// of the shared block template.
	msgBlock := *templateBlock
	msgBlock.Header = submittedHeader
	block := dcrutil.NewBlock(&msgBlock)

	// Process this block using the same rules as blocks coming from other
	// nodes.  This will in turn relay it to the network like normal.
	err = s.cfg.SyncMgr.SubmitBlock(block)
	if err != nil {
		// Anything other than a rule violation is an unexpected error,
		// so return that error as an internal error.
		var rErr blockchain.RuleError
		if !errors.As(err, &rErr) {
			const context = "Unexpected error while processing block"
			return false, rpcInternalErr(err, context)
		}

		log.Infof("Block submitted via getwork rejected: %v", err)
		return false, nil
	}

	// The block was accepted.
	var powHashStr string
	blockHash := block.Hash()
	if *blockHash != powHash {
		powHashStr = ", pow hash " + powHash.String()
	}
	log.Infof("Block submitted via getwork accepted: %s (height %d%s)",
		blockHash, msgBlock.Header.Height, powHashStr)
	return true, nil
}

// handleGetWork implements the getwork command.
func handleGetWork(ctx context.Context, s *Server, cmd interface{}) (interface{}, error) {
	if s.cfg.CPUMiner.IsMining() {
		return nil, rpcMiscError("getwork polling is disallowed " +
			"while CPU mining is enabled. Please disable CPU " +
			"mining and try again.")
	}

	// Respond with an error if there are no addresses to pay the created
	// blocks to.
	if len(s.cfg.MiningAddrs) == 0 {
		err := errors.New("no payment addresses specified via --miningaddr")
		return nil, rpcInternalErr(err, "Configuration")
	}

	// Return an error if there are no peers connected since there is no way to
	// relay a found block or receive transactions to work on unless
	// unsynchronized mining has specifically been allowed.
	if !s.cfg.AllowUnsyncedMining && s.cfg.ConnMgr.ConnectedCount() == 0 {
		return nil, &dcrjson.RPCError{
			Code:    dcrjson.ErrRPCClientNotConnected,
			Message: "Decred is not connected",
		}
	}

	// No point in generating or accepting work before the chain is synced
	// unless unsynchronized mining has specifically been allowed.
	chain := s.cfg.Chain
	_, bestHeaderHeight := chain.BestHeader()
	bestHeight := chain.BestSnapshot().Height
	initialChainState := bestHeaderHeight == 0 && bestHeight == 0
	if !s.cfg.AllowUnsyncedMining && !initialChainState && !chain.IsCurrent() {
		return nil, &dcrjson.RPCError{
			Code:    dcrjson.ErrRPCClientInInitialDownload,
			Message: "Decred is downloading blocks...",
		}
	}

	c := cmd.(*types.GetWorkCmd)

	// Protect concurrent access from multiple RPC invocations for work requests
	// and submission.  A single item semaphore is used over a mutex to support
	// early cancellation.
	select {
	case s.workState.workSem <- struct{}{}:
	case <-ctx.Done():
		return nil, rpcConnectionClosedError()
	}
	defer s.workState.workSem.release()

	// When the caller provides data, it is a submission of a supposedly
	// solved block that needs to be checked and submitted to the network
	// if valid.
	if c.Data != nil && *c.Data != "" {
		return handleGetWorkSubmission(ctx, s, *c.Data)
	}

	// No data was provided, so the caller is requesting work.
	return handleGetWorkRequest(ctx, s)
}

// handleHelp implements the help command.
func handleHelp(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.HelpCmd)

	// Provide a usage overview of all commands when no specific command
	// was specified.
	var method types.Method
	if c.Command != nil {
		method = types.Method(*c.Command)
	}
	if method == "" {
		usage, err := s.helpCacher.RPCUsage(false)
		if err != nil {
			return nil, rpcInternalErr(err, "Failed to generate RPC usage")
		}
		return usage, nil
	}

	// Check that the command asked for is supported and implemented.  Only
	// search the main list of handlers since help should not be provided
	// for commands that are unimplemented or related to wallet
	// functionality.
	if _, ok := rpcHandlers[method]; !ok {
		return nil, rpcInvalidError("Unknown method: %v", method)
	}

	// Get the help for the command.
	help, err := s.helpCacher.RPCMethodHelp(method)
	if err != nil {
		return nil, rpcInternalErr(err, "Failed to generate help")
	}
	return help, nil
}

// handleInvalidateBlock implements the invalidateblock command.
func handleInvalidateBlock(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.InvalidateBlockCmd)
	hash, err := chainhash.NewHashFromStr(c.BlockHash)
	if err != nil {
		return nil, rpcDecodeHexError(c.BlockHash)
	}

	chain := s.cfg.Chain
	err = chain.InvalidateBlock(hash)
	if err != nil {
		if errors.Is(err, blockchain.ErrUnknownBlock) {
			return nil, &dcrjson.RPCError{
				Code:    dcrjson.ErrRPCBlockNotFound,
				Message: fmt.Sprintf("Block not found: %v", hash),
			}
		}

		if errors.Is(err, blockchain.ErrInvalidateGenesisBlock) {
			return nil, rpcInvalidError("%v", err)
		}

		context := fmt.Sprintf("Failed to invalidate block %s", hash)
		return nil, rpcInternalErr(err, context)
	}

	return nil, nil
}

// handleLiveTickets implements the livetickets command.
func handleLiveTickets(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	lt, err := s.cfg.Chain.LiveTickets()
	if err != nil {
		return nil, rpcInternalErr(err, "Could not get live tickets")
	}

	ltString := make([]string, len(lt))
	for i := range lt {
		ltString[i] = lt[i].String()
	}

	return types.LiveTicketsResult{Tickets: ltString}, nil
}

// handlePing implements the ping command.
func handlePing(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	// Ask server to ping \o_
	nonce := rand.Uint64()
	s.cfg.ConnMgr.BroadcastMessage(wire.NewMsgPing(nonce))

	return nil, nil
}

// handleReconsiderBlock implements the reconsiderblock command.
func handleReconsiderBlock(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.ReconsiderBlockCmd)
	hash, err := chainhash.NewHashFromStr(c.BlockHash)
	if err != nil {
		return nil, rpcDecodeHexError(c.BlockHash)
	}

	chain := s.cfg.Chain
	err = chain.ReconsiderBlock(hash)
	if err != nil {
		if errors.Is(err, blockchain.ErrUnknownBlock) {
			return nil, &dcrjson.RPCError{
				Code:    dcrjson.ErrRPCBlockNotFound,
				Message: fmt.Sprintf("Block not found: %v", hash),
			}
		}

		// Use separate error code for failed validation.
		allRuleErrs := func(err error) bool {
			var rErr blockchain.RuleError
			if !errors.As(err, &rErr) {
				return false
			}

			var mErr blockchain.MultiError
			if errors.As(err, &mErr) {
				for _, e := range mErr {
					if !errors.As(e, &rErr) {
						return false
					}
				}
			}

			return true
		}
		if allRuleErrs(err) {
			return nil, &dcrjson.RPCError{
				Code: dcrjson.ErrRPCReconsiderFailure,
				Message: fmt.Sprintf("Reconsidering block %s led to one or "+
					"more validation failures: %v", hash, err),
			}
		}

		// Fall back to an internal error.
		context := fmt.Sprintf("Error while reconsidering block %s", hash)
		return nil, rpcInternalErr(err, context)
	}

	return nil, nil
}

// handleRegenTemplate implements the regentemplate command.
func handleRegenTemplate(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	bt := s.cfg.BlockTemplater
	if bt == nil {
		err := errors.New("node is not configured for mining")
		return nil, rpcInternalErr(err, "")
	}
	bt.ForceRegen()
	return nil, nil
}

// handleSendRawMixMessage implements the sendrawmixmessage command.
func handleSendRawMixMessage(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.SendRawMixMessageCmd)

	// Allocate a message of the appropriate type based on the wire
	// command string.
	var msg mixing.Message
	switch c.Command {
	case wire.CmdMixPairReq:
		msg = new(wire.MsgMixPairReq)
	case wire.CmdMixKeyExchange:
		msg = new(wire.MsgMixKeyExchange)
	case wire.CmdMixCiphertexts:
		msg = new(wire.MsgMixCiphertexts)
	case wire.CmdMixSlotReserve:
		msg = new(wire.MsgMixSlotReserve)
	case wire.CmdMixDCNet:
		msg = new(wire.MsgMixDCNet)
	case wire.CmdMixConfirm:
		msg = new(wire.MsgMixConfirm)
	case wire.CmdMixFactoredPoly:
		msg = new(wire.MsgMixFactoredPoly)
	case wire.CmdMixSecrets:
		msg = new(wire.MsgMixSecrets)
	default:
		err := rpcInvalidError("Unrecognized mixing message "+
			"wire command string %q", c.Command)
		return nil, err
	}

	// Deserialize message.
	err := msg.BtcDecode(hex.NewDecoder(strings.NewReader(c.Message)),
		wire.MixVersion)
	if err != nil {
		return nil, rpcDeserializationError("Could not decode mix "+
			"message: %v", err)
	}

	// Message hash of the freshly-deserialized mixing message needs to be
	// pre-calculated.  Calculate this here now; it is required before it
	// can be accepted and notified.
	s.blake256HaserMu.Lock()
	msg.WriteHash(s.blake256Hasher)
	s.blake256HaserMu.Unlock()

	err = s.cfg.SyncMgr.SubmitMixMessage(msg)
	if err != nil {
		// XXX: consider a better error code/function
		str := fmt.Sprintf("Rejected mix message: %s", err)
		return nil, rpcMiscError(str)
	}

	s.cfg.ConnMgr.RelayMixMessages([]mixing.Message{msg})

	s.ntfnMgr.NotifyMixMessage(msg)

	return nil, nil
}

// handleSendRawTransaction implements the sendrawtransaction command.
func handleSendRawTransaction(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.SendRawTransactionCmd)
	// Deserialize and send off to tx relay

	allowHighFees := *c.AllowHighFees
	hexStr := c.HexTx
	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}
	serializedTx, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, rpcDecodeHexError(hexStr)
	}
	msgtx := wire.NewMsgTx()
	err = msgtx.Deserialize(bytes.NewReader(serializedTx))
	if err != nil {
		return nil, rpcDeserializationError("Could not decode Tx: %v",
			err)
	}

	// Use 0 for the tag to represent local node.
	tx := dcrutil.NewTx(msgtx)
	acceptedTxs, err := s.cfg.SyncMgr.ProcessTransaction(tx, false,
		allowHighFees, 0)
	if err != nil {
		// When the error is a rule error, it means the transaction was
		// simply rejected as opposed to something actually going
		// wrong, so log it as such.  Otherwise, something really did
		// go wrong, so log it as an actual error.  In both cases, a
		// JSON-RPC error is returned to the client with the
		// deserialization error code (to match bitcoind behavior).
		var rErr mempool.RuleError
		if errors.As(err, &rErr) {
			hash := tx.Hash()
			err = fmt.Errorf("rejected transaction %v: %w", hash,
				err)
			log.Debugf("%v", err)

			// Use the duplicate tx error code when the transaction
			// is known to already be submitted to the mempool, as
			// well as whenever there is a high certainty that the
			// transaction has been confirmed in a recent block.
			switch {
			case errors.Is(rErr, mempool.ErrDuplicate):
				fallthrough
			case errors.Is(rErr, mempool.ErrAlreadyExists):
				fallthrough
			case s.cfg.SyncMgr.RecentlyConfirmedTxn(hash):
				return nil, rpcDuplicateTxError("%v", err)
			}

			// return a generic rule error
			return nil, rpcRuleError("%v", err)
		}

		err = fmt.Errorf("failed to process transaction %v: %w",
			tx.Hash(), err)
		log.Errorf("%v", err)
		return nil, rpcDeserializationError("rejected: %v", err)
	}

	// Generate and relay inventory vectors for all newly accepted
	// transactions.
	s.cfg.ConnMgr.RelayTransactions(acceptedTxs)

	// Notify websocket clients of all newly accepted transactions.
	s.NotifyNewTransactions(acceptedTxs)

	// Keep track of all the regular sendrawtransaction request txns so that
	// they can be rebroadcast if they don't make their way into a block.
	//
	// Note that votes are only valid for a specific block and are time
	// sensitive, so they should not be added to the rebroadcast logic.
	txType := stake.DetermineTxType(msgtx)
	if txType != stake.TxTypeSSGen {
		iv := wire.NewInvVect(wire.InvTypeTx, tx.Hash())
		s.cfg.ConnMgr.AddRebroadcastInventory(iv, tx)
	}

	return tx.Hash().String(), nil
}

// handleSetGenerate implements the setgenerate command.
func handleSetGenerate(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.SetGenerateCmd)

	// Disable generation regardless of the provided generate flag if the
	// maximum number of threads (goroutines for our purposes) is 0.
	// Otherwise enable or disable it depending on the provided flag.
	generate := c.Generate
	genProcLimit := -1
	if c.GenProcLimit != nil {
		genProcLimit = *c.GenProcLimit
	}
	if genProcLimit == 0 {
		generate = false
	}

	if !generate {
		// Stop CPU mining by setting the number of workers to zero, if needed.
		s.cfg.CPUMiner.SetNumWorkers(0)
	} else {
		// Respond with an error if there are no addresses to pay the
		// created blocks to.
		if len(s.cfg.MiningAddrs) == 0 {
			err := errors.New("no payment addresses specified via --miningaddr")
			return nil, rpcInternalErr(err, "Configuration")
		}

		s.cfg.CPUMiner.SetNumWorkers(int32(genProcLimit))
	}
	return nil, nil
}

// handleStartProfiler implements the startprofiler command.
func handleStartProfiler(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.StartProfilerCmd)

	if listeners := s.cfg.ProfilerMgr.Listeners(); len(listeners) > 0 {
		const str = "profile server is already running"
		return nil, dcrjson.NewRPCError(dcrjson.ErrRPCProfilerState, str)
	}

	var allowNonLoopback bool
	if c.AllowNonLoopback != nil {
		allowNonLoopback = *c.AllowNonLoopback
	}
	if err := s.cfg.ProfilerMgr.Start(c.Addr, allowNonLoopback); err != nil {
		return nil, rpcInvalidError("unable to start profile server: %v", err)
	}

	// Ensure there are active listeners for generating the result.
	//
	// In general, there should always be listeners since the server was just
	// successfully started.  However, although improbable, it's possible that
	// the server was stopped by a separate goroutine in between, so be safe.
	listeners := s.cfg.ProfilerMgr.Listeners()
	if len(listeners) == 0 {
		err := errors.New("profile server started without active listeners")
		return nil, rpcInternalErr(err, "")
	}

	return &types.StartProfilerResult{
		Listeners: listeners,
	}, nil
}

// handleStop implements the stop command.
func handleStop(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	select {
	case s.requestProcessShutdown <- struct{}{}:
	default:
	}
	return "dcrd stopping.", nil
}

// handleStopProfiler implements the stopprofiler command.
func handleStopProfiler(_ context.Context, s *Server, _ interface{}) (interface{}, error) {
	if listeners := s.cfg.ProfilerMgr.Listeners(); len(listeners) == 0 {
		const str = "profile server is not started"
		return nil, dcrjson.NewRPCError(dcrjson.ErrRPCProfilerState, str)
	}

	if err := s.cfg.ProfilerMgr.Stop(); err != nil {
		const context = "unexpected error when stopping profile server"
		return nil, rpcInternalErr(err, context)
	}

	return "profile server stopped", nil
}

// handleSubmitBlock implements the submitblock command.
func handleSubmitBlock(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.SubmitBlockCmd)

	// Deserialize the submitted block.
	hexStr := c.HexBlock
	if len(hexStr)%2 != 0 {
		hexStr = "0" + c.HexBlock
	}
	serializedBlock, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, rpcInternalErr(err, "Block decode")
	}

	block, err := dcrutil.NewBlockFromBytes(serializedBlock)
	if err != nil {
		return nil, rpcInternalErr(err, "Block decode")
	}

	err = s.cfg.SyncMgr.SubmitBlock(block)
	if err != nil {
		return fmt.Sprintf("rejected: %v", err), nil
	}

	log.Infof("Accepted block %s via submitblock", block.Hash())
	return nil, nil
}

// min gets the minimum amount from a slice of amounts.
func min(s []dcrutil.Amount) dcrutil.Amount {
	if len(s) == 0 {
		return 0
	}

	min := s[0]
	for i := range s {
		if s[i] < min {
			min = s[i]
		}
	}

	return min
}

// max gets the maximum amount from a slice of amounts.
func max(s []dcrutil.Amount) dcrutil.Amount {
	max := dcrutil.Amount(0)
	for i := range s {
		if s[i] > max {
			max = s[i]
		}
	}

	return max
}

// mean gets the mean amount from a slice of amounts.
func mean(s []dcrutil.Amount) dcrutil.Amount {
	sum := dcrutil.Amount(0)
	for i := range s {
		sum += s[i]
	}

	if len(s) == 0 {
		return 0
	}

	return sum / dcrutil.Amount(len(s))
}

// median gets the median amount from a slice of amounts.
func median(s []dcrutil.Amount) dcrutil.Amount {
	if len(s) == 0 {
		return 0
	}

	sort.Sort(dcrutil.AmountSorter(s))

	middle := len(s) / 2

	if len(s) == 0 {
		return 0
	} else if (len(s) % 2) != 0 {
		return s[middle]
	}
	return (s[middle] + s[middle-1]) / 2
}

// stdDev gets the standard deviation amount from a slice of amounts.
func stdDev(s []dcrutil.Amount) dcrutil.Amount {
	var total float64
	mean := mean(s)

	for i := range s {
		total += math.Pow(s[i].ToCoin()-mean.ToCoin(), 2)
	}
	if len(s)-1 == 0 {
		return 0
	}
	v := total / float64(len(s)-1)

	// Not concerned with an error here, it'll return
	// zero if the amount is too small.
	amt, _ := dcrutil.NewAmount(math.Sqrt(v))

	return amt
}

// feeInfoForMempool returns the fee information for the passed tx type in the
// memory pool.
func feeInfoForMempool(s *Server, txType stake.TxType) *types.FeeInfoMempool {
	txDs := s.cfg.TxMempooler.TxDescs()
	ticketFees := make([]dcrutil.Amount, 0, len(txDs))
	for _, txD := range txDs {
		if txD.Type == txType {
			feePerKb := (dcrutil.Amount(txD.Fee)) * 1000 /
				dcrutil.Amount(txD.Tx.MsgTx().SerializeSize())
			ticketFees = append(ticketFees, feePerKb)
		}
	}

	return &types.FeeInfoMempool{
		Number: uint32(len(ticketFees)),
		Min:    min(ticketFees).ToCoin(),
		Max:    max(ticketFees).ToCoin(),
		Mean:   mean(ticketFees).ToCoin(),
		Median: median(ticketFees).ToCoin(),
		StdDev: stdDev(ticketFees).ToCoin(),
	}
}

// calcFee calculates the fee of a transaction that has its fraud proofs
// properly set.
func calcFeePerKb(tx *dcrutil.Tx) dcrutil.Amount {
	var in dcrutil.Amount
	for _, txIn := range tx.MsgTx().TxIn {
		in += dcrutil.Amount(txIn.ValueIn)
	}
	var out dcrutil.Amount
	for _, txOut := range tx.MsgTx().TxOut {
		out += dcrutil.Amount(txOut.Value)
	}

	return ((in - out) * 1000) / dcrutil.Amount(tx.MsgTx().SerializeSize())
}

// ticketFeeInfoForBlock fetches the ticket fee information for a given tx type
// in a block.
func ticketFeeInfoForBlock(s *Server, height int64, txType stake.TxType) (*types.FeeInfoBlock, error) {
	bl, err := s.cfg.Chain.BlockByHeight(height)
	if err != nil {
		return nil, err
	}

	txNum := 0
	switch txType {
	case stake.TxTypeRegular:
		txNum = len(bl.MsgBlock().Transactions) - 1
	case stake.TxTypeSStx:
		txNum = int(bl.MsgBlock().Header.FreshStake)
	case stake.TxTypeSSGen:
		txNum = int(bl.MsgBlock().Header.Voters)
	case stake.TxTypeSSRtx:
		txNum = int(bl.MsgBlock().Header.Revocations)
	}

	txFees := make([]dcrutil.Amount, txNum)
	itr := 0
	if txType == stake.TxTypeRegular {
		for i, tx := range bl.Transactions() {
			// Skip the coin base.
			if i == 0 {
				continue
			}

			txFees[itr] = calcFeePerKb(tx)
			itr++
		}
	} else {
		for _, stx := range bl.STransactions() {
			thisTxType := stake.DetermineTxType(stx.MsgTx())
			if thisTxType == txType {
				txFees[itr] = calcFeePerKb(stx)
				itr++
			}
		}
	}

	return &types.FeeInfoBlock{
		Height: uint32(height),
		Number: uint32(txNum),
		Min:    min(txFees).ToCoin(),
		Max:    max(txFees).ToCoin(),
		Mean:   mean(txFees).ToCoin(),
		Median: median(txFees).ToCoin(),
		StdDev: stdDev(txFees).ToCoin(),
	}, nil
}

// ticketFeeInfoForRange fetches the ticket fee information for a given range
// from [start, end).
func ticketFeeInfoForRange(s *Server, start int64, end int64, txType stake.TxType) (*types.FeeInfoWindow, error) {
	chain := s.cfg.Chain
	hashes, err := chain.HeightRange(start, end)
	if err != nil {
		return nil, err
	}

	var txFees []dcrutil.Amount
	for i := range hashes {
		bl, err := chain.BlockByHash(&hashes[i])
		if err != nil {
			return nil, err
		}

		if txType == stake.TxTypeRegular {
			for i, tx := range bl.Transactions() {
				// Skip the coin base.
				if i == 0 {
					continue
				}

				txFees = append(txFees, calcFeePerKb(tx))
			}
		} else {
			for _, stx := range bl.STransactions() {
				thisTxType := stake.DetermineTxType(stx.MsgTx())
				if thisTxType == txType {
					txFees = append(txFees, calcFeePerKb(stx))
				}
			}
		}
	}

	return &types.FeeInfoWindow{
		StartHeight: uint32(start),
		EndHeight:   uint32(end),
		Number:      uint32(len(txFees)),
		Min:         min(txFees).ToCoin(),
		Max:         max(txFees).ToCoin(),
		Mean:        mean(txFees).ToCoin(),
		Median:      median(txFees).ToCoin(),
		StdDev:      stdDev(txFees).ToCoin(),
	}, nil
}

// handleTicketFeeInfo implements the ticketfeeinfo command.
func handleTicketFeeInfo(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.TicketFeeInfoCmd)

	bestHeight := s.cfg.Chain.BestSnapshot().Height

	// Memory pool first.
	feeInfoMempool := feeInfoForMempool(s, stake.TxTypeSStx)

	// Blocks requested, descending from the chain tip.
	var feeInfoBlocks []types.FeeInfoBlock
	blocks := uint32(0)
	if c.Blocks != nil {
		blocks = *c.Blocks
	}
	if blocks > 0 {
		start := bestHeight
		end := bestHeight - int64(blocks)

		for i := start; i > end; i-- {
			feeInfo, err := ticketFeeInfoForBlock(s, i, stake.TxTypeSStx)
			if err != nil {
				const context = "Could not obtain ticket fee info"
				return nil, rpcInternalErr(err, context)
			}
			feeInfoBlocks = append(feeInfoBlocks, *feeInfo)
		}
	}

	var feeInfoWindows []types.FeeInfoWindow
	windows := uint32(0)
	if c.Windows != nil {
		windows = *c.Windows
	}
	if windows > 0 {
		// The first window is special because it may not be finished.
		// Perform this first and return if it's the only window the
		// user wants. Otherwise, append and continue.
		winLen := s.cfg.ChainParams.StakeDiffWindowSize
		lastChange := (bestHeight / winLen) * winLen

		feeInfo, err := ticketFeeInfoForRange(s, lastChange, bestHeight+1,
			stake.TxTypeSStx)
		if err != nil {
			return nil, rpcInternalErr(err, "Could not obtain ticket fee info")
		}
		feeInfoWindows = append(feeInfoWindows, *feeInfo)

		// We need data on windows from before this. Start from
		// the last adjustment and move backwards through window
		// lengths, calculating the fees data and appending it
		// each time.
		if windows > 1 {
			// Go down to the last height requested, except
			// in the case that the user has specified to
			// many windows. In that case, just proceed to the
			// first block.
			end := int64(-1)
			if lastChange-int64(windows)*winLen > end {
				end = lastChange - int64(windows)*winLen
			}
			for i := lastChange; i > end+winLen; i -= winLen {
				feeInfo, err := ticketFeeInfoForRange(s, i-winLen, i,
					stake.TxTypeSStx)
				if err != nil {
					const context = "Could not obtain ticket fee info"
					return nil, rpcInternalErr(err, context)
				}
				feeInfoWindows = append(feeInfoWindows, *feeInfo)
			}
		}
	}

	return &types.TicketFeeInfoResult{
		FeeInfoMempool: *feeInfoMempool,
		FeeInfoBlocks:  feeInfoBlocks,
		FeeInfoWindows: feeInfoWindows,
	}, nil
}

// handleTicketsForAddress implements the ticketsforaddress command.
func handleTicketsForAddress(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.TicketsForAddressCmd)

	// Decode the provided address.  This also ensures the network encoded
	// with the address matches the network the server is currently on.
	addr, err := stdaddr.DecodeAddress(c.Address, s.cfg.ChainParams)
	if err != nil {
		return nil, rpcInvalidError("Invalid address: %v", err)
	}

	stakeAddr, ok := addr.(stdaddr.StakeAddress)
	if !ok {
		return nil, rpcInvalidError("Address is not valid for use in the " +
			"staking system")
	}

	chain := s.cfg.Chain
	tickets, err := chain.TicketsWithAddress(stakeAddr)
	if err != nil {
		return nil, rpcInternalErr(err, "Could not obtain tickets")
	}

	ticketStrings := make([]string, len(tickets))
	itr := 0
	for _, ticket := range tickets {
		ticketStrings[itr] = ticket.String()
		itr++
	}

	reply := &types.TicketsForAddressResult{
		Tickets: ticketStrings,
	}
	return reply, nil
}

// handleTicketVWAP implements the ticketvwap command.
func handleTicketVWAP(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.TicketVWAPCmd)

	// The default VWAP is for the past WorkDiffWindows * WorkDiffWindowSize
	// many blocks.
	bestHeight := s.cfg.Chain.BestSnapshot().Height
	var start uint32
	if c.Start == nil {
		params := s.cfg.ChainParams
		toEval := params.WorkDiffWindows * params.WorkDiffWindowSize
		startI64 := bestHeight - toEval

		// Use 1 as the first block if there aren't enough blocks.
		if startI64 <= 0 {
			start = 1
		} else {
			start = uint32(startI64)
		}
	} else {
		start = *c.Start
	}

	end := uint32(bestHeight)
	if c.End != nil {
		end = *c.End
	}
	if start > end {
		return nil, rpcInvalidError("Start height %v is beyond end "+
			"height %v", start, end)
	}
	if end > uint32(bestHeight) {
		return nil, rpcInvalidError("End height %v is beyond "+
			"blockchain tip height %v", end, bestHeight)
	}

	// Calculate the volume weighted average price of a ticket for the
	// given range.
	ticketNum := int64(0)
	totalValue := int64(0)
	for i := start; i <= end; i++ {
		blockHeader, err := s.cfg.Chain.HeaderByHeight(int64(i))
		if err != nil {
			return nil, rpcInternalErr(err, "Could not obtain header")
		}

		ticketNum += int64(blockHeader.FreshStake)
		totalValue += blockHeader.SBits * int64(blockHeader.FreshStake)
	}
	vwap := int64(0)
	if ticketNum > 0 {
		vwap = totalValue / ticketNum
	}

	return dcrutil.Amount(vwap).ToCoin(), nil
}

// handleTxFeeInfo implements the txfeeinfo command.
func handleTxFeeInfo(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.TxFeeInfoCmd)

	bestHeight := s.cfg.Chain.BestSnapshot().Height

	// Memory pool first.
	feeInfoMempool := feeInfoForMempool(s, stake.TxTypeRegular)

	// Blocks requested, descending from the chain tip.
	var feeInfoBlocks []types.FeeInfoBlock
	blocks := uint32(0)
	if c.Blocks != nil {
		blocks = *c.Blocks
	}
	if blocks > 0 {
		start := bestHeight
		end := bestHeight - int64(blocks)

		for i := start; i > end; i-- {
			feeInfo, err := ticketFeeInfoForBlock(s, i,
				stake.TxTypeRegular)
			if err != nil {
				const context = "Could not obtain ticket fee info"
				return nil, rpcInternalErr(err, context)
			}
			feeInfoBlocks = append(feeInfoBlocks, *feeInfo)
		}
	}

	// Get the fee info for the range requested, unless none is given.  The
	// default range is for the past WorkDiffWindowSize many blocks.
	var feeInfoRange types.FeeInfoRange

	var start uint32
	if c.RangeStart == nil {
		toEval := s.cfg.ChainParams.WorkDiffWindowSize
		startI64 := bestHeight - toEval

		// Use 1 as the first block if there aren't enough blocks.
		if startI64 <= 0 {
			start = 1
		} else {
			start = uint32(startI64)
		}
	} else {
		start = *c.RangeStart
	}

	end := uint32(bestHeight)
	if c.RangeEnd != nil {
		end = *c.RangeEnd
	}
	if start > end {
		return nil, rpcInvalidError("Start height %v is beyond end "+
			"height %v", start, end)
	}
	if end > uint32(bestHeight) {
		return nil, rpcInvalidError("End height %v is beyond "+
			"blockchain tip height %v", end, bestHeight)
	}

	feeInfo, err := ticketFeeInfoForRange(s, int64(start), int64(end+1),
		stake.TxTypeRegular)
	if err != nil {
		return nil, rpcInternalErr(err, "Could not obtain ticket fee info")
	}

	feeInfoRange = types.FeeInfoRange{
		Number: feeInfo.Number,
		Min:    feeInfo.Min,
		Max:    feeInfo.Max,
		Mean:   feeInfo.Mean,
		Median: feeInfo.Median,
		StdDev: feeInfo.StdDev,
	}

	return &types.TxFeeInfoResult{
		FeeInfoMempool: *feeInfoMempool,
		FeeInfoBlocks:  feeInfoBlocks,
		FeeInfoRange:   feeInfoRange,
	}, nil
}

// handleValidateAddress implements the validateaddress command.
func handleValidateAddress(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.ValidateAddressCmd)
	result := types.ValidateAddressChainResult{}
	addr, err := stdaddr.DecodeAddress(c.Address, s.cfg.ChainParams)
	if err != nil {
		// Return the default value (false) for IsValid.
		return result, nil // nolint: nilerr
	}

	result.Address = addr.String()
	result.IsValid = true
	return result, nil
}

func verifyChain(_ context.Context, s *Server, level, depth int64) error {
	best := s.cfg.Chain.BestSnapshot()
	finishHeight := best.Height - depth
	if finishHeight < 0 {
		finishHeight = 0
	}
	log.Infof("Verifying chain for %d blocks at level %d",
		best.Height-finishHeight, level)

	for height := best.Height; height > finishHeight; height-- {
		// Level 0 just looks up the block.
		block, err := s.cfg.Chain.BlockByHeight(height)
		if err != nil {
			log.Errorf("Verify is unable to fetch block at "+
				"height %d: %v", height, err)
			return err
		}

		// Level 1 does basic chain sanity checks.
		if level > 0 {
			err := s.cfg.SanityChecker.CheckBlockSanity(block)
			if err != nil {
				log.Errorf("Verify is unable to validate "+
					"block at hash %v height %d: %v",
					block.Hash(), height, err)
				return err
			}
		}
	}
	log.Infof("Chain verify completed successfully")

	return nil
}

// handleVerifyChain implements the verifychain command.
func handleVerifyChain(ctx context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.VerifyChainCmd)

	var checkLevel, checkDepth int64
	if c.CheckLevel != nil {
		checkLevel = *c.CheckLevel
	}
	if c.CheckDepth != nil {
		checkDepth = *c.CheckDepth
	}

	err := verifyChain(ctx, s, checkLevel, checkDepth)
	return err == nil, nil
}

// handleVerifyMessage implements the verifymessage command.
func handleVerifyMessage(_ context.Context, s *Server, cmd interface{}) (interface{}, error) {
	c := cmd.(*types.VerifyMessageCmd)

	// Decode the provided address.  This also ensures the network encoded with
	// the address matches the network the server is currently on.
	params := s.cfg.ChainParams
	addr, err := stdaddr.DecodeAddress(c.Address, params)
	if err != nil {
		return nil, rpcAddressKeyError("Could not decode address: %v",
			err)
	}

	// Only version 0 P2PKH addresses are valid for signing.
	if _, ok := addr.(*stdaddr.AddressPubKeyHashEcdsaSecp256k1V0); !ok {
		return nil, &dcrjson.RPCError{
			Code:    dcrjson.ErrRPCType,
			Message: "Address is not a pay-to-pubkey-hash address",
		}
	}

	// Decode base64 signature.
	sig, err := base64.StdEncoding.DecodeString(c.Signature)
	if err != nil {
		return nil, &dcrjson.RPCError{
			Code:    dcrjson.ErrRPCParse.Code,
			Message: "Malformed base64 encoding: " + err.Error(),
		}
	}

	// Validate the signature - this just shows that it was valid at all.
	// we will compare it with the key next.
	var buf bytes.Buffer
	wire.WriteVarString(&buf, 0, "Decred Signed Message:\n")
	wire.WriteVarString(&buf, 0, c.Message)
	expectedMessageHash := chainhash.HashB(buf.Bytes())
	pk, wasCompressed, err := ecdsa.RecoverCompact(sig, expectedMessageHash)
	if err != nil {
		// Treat errors in RecoverCompact as an invalid signature.
		return false, nil // nolint: nilerr
	}

	// Reconstruct the pubkey hash.
	var pkHash []byte
	if wasCompressed {
		pkHash = stdaddr.Hash160(pk.SerializeCompressed())
	} else {
		pkHash = stdaddr.Hash160(pk.SerializeUncompressed())
	}
	address, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(pkHash, params)
	if err != nil {
		// Treat error in reconstruction as an invalid signature.
		return false, nil // nolint: nilerr
	}

	// Return boolean if addresses match.
	return address.String() == c.Address, nil
}

// handleVersion implements the version command.
func handleVersion(_ context.Context, _ *Server, _ interface{}) (interface{}, error) {
	runtimeVer := strings.ReplaceAll(runtime.Version(), ".", "-")
	buildMeta := version.NormalizeString(runtimeVer)
	build := version.NormalizeString(version.BuildMetadata)
	if build != "" {
		buildMeta = fmt.Sprintf("%s.%s", build, buildMeta)
	}
	result := map[string]types.VersionResult{
		"dcrdjsonrpcapi": {
			VersionString: jsonrpcSemverString,
			Major:         jsonrpcSemverMajor,
			Minor:         jsonrpcSemverMinor,
			Patch:         jsonrpcSemverPatch,
		},
		"dcrd": {
			VersionString: version.String(),
			Major:         version.Major,
			Minor:         version.Minor,
			Patch:         version.Patch,
			Prerelease:    version.NormalizeString(version.PreRelease),
			BuildMetadata: buildMeta,
		},
	}
	return result, nil
}

// Server provides a concurrent safe RPC server to a chain server.
type Server struct {
	numClients atomic.Int32

	cfg                    Config
	hmac                   hash.Hash
	hmacMu                 sync.Mutex
	authsha                [sha256.Size]byte
	limitauthsha           [sha256.Size]byte
	ntfnMgr                NtfnManager
	statusLines            map[int]string
	statusLock             sync.RWMutex
	workState              *workState
	helpCacher             RPCHelpCacher
	requestProcessShutdown chan struct{}

	// blake256Hasher is the hash.Hash object that is used after
	// deserializing mixing messages.  Message handlers may be executed
	// concurrently, and access requires the mutex.
	blake256Hasher  hash.Hash
	blake256HaserMu sync.Mutex
}

// isTreasuryAgendaActive returns if the treasury agenda is active or not for
// the block AFTER the provided block hash.
func (s *Server) isTreasuryAgendaActive(prevBlkHash *chainhash.Hash) (bool, error) {
	chain := s.cfg.Chain
	isTreasuryEnabled, err := chain.IsTreasuryAgendaActive(prevBlkHash)
	if err != nil {
		context := fmt.Sprintf("Could not obtain treasury agenda status for "+
			"block %s", prevBlkHash)
		return false, rpcInternalErr(err, context)
	}
	return isTreasuryEnabled, nil
}

// isAutoRevocationsAgendaActive returns if the automatic ticket revocations
// agenda is active or not for the block AFTER the provided block hash.
func (s *Server) isAutoRevocationsAgendaActive(prevBlkHash *chainhash.Hash) (bool, error) {
	chain := s.cfg.Chain
	isAutoRevocationsEnabled, err := chain.IsAutoRevocationsAgendaActive(prevBlkHash)
	if err != nil {
		context := fmt.Sprintf("Could not obtain automatic ticket revocations "+
			"agenda status for block %s", prevBlkHash)
		return false, rpcInternalErr(err, context)
	}
	return isAutoRevocationsEnabled, nil
}

// isSubsidySplitAgendaActive returns if the modified subsidy split agenda is
// active or not for the block AFTER the provided block hash.
func (s *Server) isSubsidySplitAgendaActive(prevBlkHash *chainhash.Hash) (bool, error) {
	chain := s.cfg.Chain
	isSubsidySplitEnabled, err := chain.IsSubsidySplitAgendaActive(prevBlkHash)
	if err != nil {
		context := fmt.Sprintf("Could not obtain modified subsidy split "+
			"agenda status for block %s", prevBlkHash)
		return false, rpcInternalErr(err, context)
	}
	return isSubsidySplitEnabled, nil
}

// isBlake3PowAgendaActive returns whether or not the agenda to change the proof
// of work hash function to blake3 is active or not for the block AFTER the
// provided block hash.
func (s *Server) isBlake3PowAgendaActive(prevBlkHash *chainhash.Hash) (bool, error) {
	chain := s.cfg.Chain
	isActive, err := chain.IsBlake3PowAgendaActive(prevBlkHash)
	if err != nil {
		context := fmt.Sprintf("Could not obtain blake3 proof of work "+
			"agenda status for block %s", prevBlkHash)
		return false, rpcInternalErr(err, context)
	}
	return isActive, nil
}

// isSubsidySplitR2AgendaActive returns if the modified subsidy split round 2
// agenda is active or not for the block AFTER the provided block hash.
func (s *Server) isSubsidySplitR2AgendaActive(prevBlkHash *chainhash.Hash) (bool, error) {
	chain := s.cfg.Chain
	isActive, err := chain.IsSubsidySplitR2AgendaActive(prevBlkHash)
	if err != nil {
		context := fmt.Sprintf("Could not obtain modified subsidy split "+
			"round 2 agenda status for block %s", prevBlkHash)
		return false, rpcInternalErr(err, context)
	}
	return isActive, nil
}

// httpStatusLine returns a response Status-Line (RFC 2616 Section 6.1) for the
// given request and response status code.  This function was lifted and
// adapted from the standard library HTTP server code since it's not exported.
func (s *Server) httpStatusLine(req *http.Request, code int) string {
	// Fast path:
	key := code
	proto11 := req.ProtoAtLeast(1, 1)
	if !proto11 {
		key = -key
	}
	s.statusLock.RLock()
	line, ok := s.statusLines[key]
	s.statusLock.RUnlock()
	if ok {
		return line
	}

	// Slow path:
	proto := "HTTP/1.0"
	if proto11 {
		proto = "HTTP/1.1"
	}
	codeStr := strconv.Itoa(code)
	text := http.StatusText(code)
	if text != "" {
		line = proto + " " + codeStr + " " + text + "\r\n"
		s.statusLock.Lock()
		s.statusLines[key] = line
		s.statusLock.Unlock()
	} else {
		text = "status code " + codeStr
		line = proto + " " + codeStr + " " + text + "\r\n"
	}

	return line
}

// writeHTTPResponseHeaders writes the necessary response headers prior to
// writing an HTTP body given a request to use for protocol negotiation,
// headers to write, a status code, and a writer.
func (s *Server) writeHTTPResponseHeaders(req *http.Request, headers http.Header, code int, w io.Writer) error {
	_, err := io.WriteString(w, s.httpStatusLine(req, code))
	if err != nil {
		return err
	}

	err = headers.Write(w)
	if err != nil {
		return err
	}

	_, err = io.WriteString(w, "\r\n")
	return err
}

// RequestedProcessShutdown returns a channel that is sent to when an
// authorized RPC client requests the process to shutdown.  If the request can
// not be read immediately, it is dropped.
func (s *Server) RequestedProcessShutdown() <-chan struct{} {
	return s.requestProcessShutdown
}

// NotifyNewTransactions notifies both websocket and getblocktemplate long
// poll clients of the passed transactions.  This function should be called
// whenever new transactions are added to the mempool.
func (s *Server) NotifyNewTransactions(txns []*dcrutil.Tx) {
	for _, tx := range txns {
		// Notify websocket clients about mempool transactions.
		s.ntfnMgr.NotifyMempoolTx(tx, true)
	}
}

// NotifyTSpend notifies websocket clients that have registered to receive new
// tspends in the mempool.
func (s *Server) NotifyTSpend(tx *dcrutil.Tx) {
	s.ntfnMgr.NotifyTSpend(tx)
}

// NotifyMixMessages notifies websocket clients that have registered to
// receive mixing message notifications of newly accepted mix messages.
func (s *Server) NotifyMixMessages(msgs []mixing.Message) {
	for _, msg := range msgs {
		s.ntfnMgr.NotifyMixMessage(msg)
	}
}

// NotifyNewTickets notifies websocket clients that have registered for maturing
// ticket updates.
func (s *Server) NotifyNewTickets(tnd *blockchain.TicketNotificationsData) {
	s.ntfnMgr.NotifyNewTickets(tnd)
}

// NotifyBlockConnected notifies websocket clients that have registered for
// block updates when a block is connected to the main chain.
func (s *Server) NotifyBlockConnected(block *dcrutil.Block) {
	s.ntfnMgr.NotifyBlockConnected(block)
}

// NotifyBlockDisconnected notifies websocket clients that have registered for
// block updates when a block is disconnected from the main chain.
func (s *Server) NotifyBlockDisconnected(block *dcrutil.Block) {
	s.ntfnMgr.NotifyBlockDisconnected(block)
}

// NotifyReorganization notifies websocket clients that have registered for
// block updates when the blockchain is beginning a reorganization.
func (s *Server) NotifyReorganization(rd *blockchain.ReorganizationNtfnsData) {
	s.ntfnMgr.NotifyReorganization(rd)
}

// NotifyWinningTickets notifies websocket clients that have registered for
// winning ticket updates.
func (s *Server) NotifyWinningTickets(wtnd *WinningTicketsNtfnData) {
	s.ntfnMgr.NotifyWinningTickets(wtnd)
}

// limitConnections responds with a 503 service unavailable and returns true if
// adding another client would exceed the maximum allow RPC clients.
//
// This function is safe for concurrent access.
func (s *Server) limitConnections(w http.ResponseWriter, remoteAddr string) bool {
	if int(s.numClients.Load()+1) > s.cfg.RPCMaxClients {
		log.Infof("Max RPC clients exceeded [%d] - "+
			"disconnecting client %s", s.cfg.RPCMaxClients,
			remoteAddr)
		http.Error(w, "503 Too busy.  Try again later.",
			http.StatusServiceUnavailable)
		return true
	}
	return false
}

// incrementClients adds one to the number of connected RPC clients.  Note this
// only applies to standard clients.  Websocket clients have their own limits
// and are tracked separately.
//
// This function is safe for concurrent access.
func (s *Server) incrementClients() {
	s.numClients.Add(1)
}

// decrementClients subtracts one from the number of connected RPC clients.
// Note this only applies to standard clients.  Websocket clients have their
// own limits and are tracked separately.
//
// This function is safe for concurrent access.
func (s *Server) decrementClients() {
	s.numClients.Add(-1)
}

// authMAC calculates the MAC (currently HMAC-SHA256) of an Authorization
// header, keyed with a random key created during server creation.  The MAC is
// appended to dst, and the appended slice is returned.
func (s *Server) authMAC(dst, auth []byte) []byte {
	s.hmacMu.Lock()
	s.hmac.Reset()
	s.hmac.Write(auth)
	dst = s.hmac.Sum(dst)
	s.hmacMu.Unlock()
	return dst
}

// checkAuthMAC checks the HTTP Basic authentication string by comparing
// it with the already generated hash.
//
// The first bool return value signifies auth success (true if successful) and
// the second bool return value specifies whether the user can change the state
// of the server (true) or whether the user is limited (false).
func (s *Server) checkAuthMAC(auth, remoteAddr string) (bool, bool) {
	mac := make([]byte, 0, sha256.Size)
	mac = s.authMAC(mac, []byte(auth))

	cmp := subtle.ConstantTimeCompare(mac, s.authsha[:])
	limitcmp := subtle.ConstantTimeCompare(mac, s.limitauthsha[:])
	if cmp|limitcmp == 0 {
		// Request's auth doesn't match either user
		log.Warnf("RPC authentication failure from %s", remoteAddr)
		return false, false
	}
	return true, cmp == 1
}

// checkAuthUserPass checks the correctness of username and password by
// generating the corresponding HTTP Basic authentication string then
// compare the string with the already generated hash.
//
// The first bool return value signifies auth success (true if successful) and
// the second bool return value specifies whether the user can change the state
// of the server (true) or whether the user is limited (false).
func (s *Server) checkAuthUserPass(user, pass, remoteAddr string) (bool, bool) {
	login := user + ":" + pass
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(login))
	return s.checkAuthMAC(auth, remoteAddr)
}

// checkAuth checks the HTTP Basic authentication supplied by a wallet or RPC
// client in the HTTP request r.  If the supplied authentication does not match
// the username and password expected, a non-nil error is returned.
//
// This check is time-constant.
//
// The first bool return value signifies auth success (true if successful) and
// the second bool return value specifies whether the user can change the state
// of the server (true) or whether the user is limited (false). The second is
// always false if the first is.
func (s *Server) checkAuth(r *http.Request, require bool) (bool, bool, error) {
	// If admin-level RPC user and pass options are not set, this always
	// succeeds.  This will be the case when TLS client certificates are
	// being used for authentication.
	if s.authsha == ([32]byte{}) {
		return true, true, nil
	}

	authhdr := r.Header["Authorization"]
	if len(authhdr) == 0 {
		if require {
			log.Warnf("RPC authentication failure from %s",
				r.RemoteAddr)
			return false, false, errors.New("auth failure")
		}

		return false, false, nil
	}

	authed, isAdmin := s.checkAuthMAC(authhdr[0], r.RemoteAddr)
	if !authed {
		return false, false, errors.New("auth failure")
	}
	return authed, isAdmin, nil
}

// parsedRPCCmd represents a JSON-RPC request object that has been parsed into
// a known concrete command along with any error that might have happened while
// parsing it.
type parsedRPCCmd struct {
	jsonrpc string
	id      interface{}
	method  types.Method
	params  interface{}
	err     *dcrjson.RPCError
}

// standardCmdResult checks that a parsed command is a standard JSON-RPC command
// and runs the appropriate handler to reply to the command.  Any commands which
// are not recognized or not implemented will return an error suitable for use
// in replies.
func (s *Server) standardCmdResult(ctx context.Context, cmd *parsedRPCCmd) (interface{}, error) {
	handler, ok := rpcHandlers[cmd.method]
	if !ok {
		return nil, dcrjson.ErrRPCMethodNotFound
	}

	return handler(ctx, s, cmd.params)
}

// parseCmd parses a JSON-RPC request object into known concrete command.  The
// err field of the returned parsedRPCCmd struct will contain an RPC error that
// is suitable for use in replies if the command is invalid in some way such as
// an unregistered command or invalid parameters.
func parseCmd(request *dcrjson.Request) *parsedRPCCmd {
	method := types.Method(request.Method)
	parsedCmd := parsedRPCCmd{
		jsonrpc: request.Jsonrpc,
		id:      request.ID,
		method:  method,
	}

	params, err := dcrjson.ParseParams(method, request.Params)
	if err != nil {
		// Produce a relevant error when the requested method is not registered
		// depending on whether or not it is recognized as being a wallet
		// command, recognized as unimplemented, or completely unrecognized.
		if errors.Is(err, dcrjson.ErrUnregisteredMethod) {
			parsedCmd.err = dcrjson.ErrRPCMethodNotFound
			if _, ok := rpcAskWallet[request.Method]; ok {
				parsedCmd.err = ErrRPCNoWallet
			} else if _, ok := rpcUnimplemented[request.Method]; ok {
				parsedCmd.err = ErrRPCUnimplemented
			}

			return &parsedCmd
		}

		// Otherwise, some type of invalid parameters is the cause, so
		// produce the equivalent RPC error.
		parsedCmd.err = rpcInvalidError("Failed to parse request: %v", err)
		return &parsedCmd
	}

	parsedCmd.params = params
	return &parsedCmd
}

// createMarshalledReply returns a new marshalled JSON-RPC response given the
// passed parameters.  It will automatically convert errors that are not of the
// type *dcrjson.RPCError to the appropriate type as needed.
func createMarshalledReply(rpcVersion string, id interface{}, result interface{}, replyErr error) ([]byte, error) {
	var jsonErr *dcrjson.RPCError
	if replyErr != nil && !errors.As(replyErr, &jsonErr) {
		jsonErr = rpcInternalErr(replyErr, "")
	}

	return dcrjson.MarshalResponse(rpcVersion, id, result, jsonErr)
}

// processRequest determines the incoming request type (single or batched),
// parses it and returns a marshalled response.
func (s *Server) processRequest(ctx context.Context, request *dcrjson.Request, isAdmin bool) []byte {
	var result interface{}
	var jsonErr error

	if !isAdmin {
		if _, ok := rpcLimited[request.Method]; !ok {
			jsonErr = rpcInvalidError("limited user not " +
				"authorized for this method")
		}
	}

	if jsonErr == nil {
		if request.Method == "" {
			jsonErr = &dcrjson.RPCError{
				Code:    dcrjson.ErrRPCInvalidRequest.Code,
				Message: "Invalid request: malformed",
			}
			msg, err := createMarshalledReply(request.Jsonrpc, request.ID, result, jsonErr)
			if err != nil {
				log.Errorf("Failed to marshal reply: %v", err)
				return nil
			}
			return msg
		}

		// Valid requests with no ID (notifications) must not have a response
		// per the JSON-RPC spec.
		if request.ID == nil {
			return nil
		}

		// Attempt to parse the JSON-RPC request into a known
		// concrete command.
		parsedCmd := parseCmd(request)
		if parsedCmd.err != nil {
			jsonErr = parsedCmd.err
		} else {
			result, jsonErr = s.standardCmdResult(ctx, parsedCmd)
		}
	}

	// Marshal the response.
	msg, err := createMarshalledReply(request.Jsonrpc, request.ID, result, jsonErr)
	if err != nil {
		log.Errorf("Failed to marshal reply: %v", err)
		return nil
	}
	return msg
}

// jsonRPCRead handles reading and responding to RPC messages.
func (s *Server) jsonRPCRead(sCtx context.Context, w http.ResponseWriter, r *http.Request, isAdmin bool) {
	select {
	case <-sCtx.Done():
		return
	default:
	}

	// Read and close the JSON-RPC request body from the caller.
	bodyReader := io.LimitReader(r.Body, rpcReadLimitAuthenticated)
	body, err := io.ReadAll(bodyReader)
	r.Body.Close()
	if err != nil {
		errMsg := fmt.Sprintf("error reading JSON message: %v", err)
		errCode := http.StatusBadRequest
		http.Error(w, strconv.Itoa(errCode)+" "+errMsg,
			errCode)
		return
	}

	// Unfortunately, the http server doesn't provide the ability to change
	// the read deadline for the new connection and having one breaks long
	// polling.  However, not having a read deadline on the initial
	// connection would mean clients can connect and idle forever.  Thus,
	// hijack the connection from the HTTP server, clear the read deadline,
	// and handle writing the response manually.
	hj, ok := w.(http.Hijacker)
	if !ok {
		errMsg := "webserver doesn't support hijacking"
		log.Warnf(errMsg)
		errCode := http.StatusInternalServerError
		http.Error(w, strconv.Itoa(errCode)+" "+errMsg,
			errCode)
		return
	}

	conn, buf, err := hj.Hijack()
	if err != nil {
		log.Warnf("Failed to hijack HTTP connection: %v", err)
		errCode := http.StatusInternalServerError
		http.Error(w, strconv.Itoa(errCode)+" "+
			err.Error(), errCode)
		return
	}

	defer conn.Close()
	defer buf.Flush()
	conn.SetReadDeadline(timeZeroVal)
	// Setup a close notifier.  Since the connection is hijacked,
	// the CloseNotifier on the ResponseWriter is not available.
	ctx, cancel := context.WithCancel(sCtx)
	defer cancel()

	go func() {
		_, err := conn.Read(make([]byte, 1))
		if err != nil {
			cancel()
		}
	}()

	var results []json.RawMessage
	var batchSize int
	var batchedRequest bool

	// Determine request type
	if bytes.HasPrefix(body, batchedRequestPrefix) {
		batchedRequest = true
	}

	// Process a single request
	if !batchedRequest {
		var req dcrjson.Request
		var resp json.RawMessage
		err = json.Unmarshal(body, &req)
		if err != nil {
			jsonErr := &dcrjson.RPCError{
				Code:    dcrjson.ErrRPCParse.Code,
				Message: fmt.Sprintf("Failed to parse request: %v", err),
			}
			resp, err = dcrjson.MarshalResponse("1.0", nil, nil, jsonErr)
			if err != nil {
				log.Errorf("Failed to create reply: %v", err)
			}
		} else {
			resp = s.processRequest(ctx, &req, isAdmin)
		}

		if resp != nil {
			results = append(results, resp)
		}
	}

	// Process a batched request
	if batchedRequest {
		var batchedRequests []json.RawMessage
		var resp json.RawMessage
		err = json.Unmarshal(body, &batchedRequests)
		if err != nil {
			jsonErr := &dcrjson.RPCError{
				Code:    dcrjson.ErrRPCParse.Code,
				Message: fmt.Sprintf("Failed to parse request: %v", err),
			}
			resp, err = dcrjson.MarshalResponse("2.0", nil, nil, jsonErr)
			if err != nil {
				log.Errorf("Failed to create reply: %v", err)
			}

			if resp != nil {
				results = append(results, resp)
			}
		}

		if err == nil {
			// Response with an empty batch error if the batch size is zero
			if len(batchedRequests) == 0 {
				jsonErr := &dcrjson.RPCError{
					Code:    dcrjson.ErrRPCInvalidRequest.Code,
					Message: "Invalid request: empty batch",
				}
				resp, err = dcrjson.MarshalResponse("2.0", nil, nil, jsonErr)
				if err != nil {
					log.Errorf("Failed to marshal reply: %v", err)
				}

				if resp != nil {
					results = append(results, resp)
				}
			}

			// Process each batch entry individually
			if len(batchedRequests) > 0 {
				batchSize = len(batchedRequests)

				for _, entry := range batchedRequests {
					var req dcrjson.Request
					err := json.Unmarshal(entry, &req)
					if err != nil {
						jsonErr := &dcrjson.RPCError{
							Code: dcrjson.ErrRPCInvalidRequest.Code,
							Message: fmt.Sprintf("Invalid request: %v",
								err),
						}
						resp, err = dcrjson.MarshalResponse("", nil, nil, jsonErr)
						if err != nil {
							log.Errorf("Failed to create reply: %v", err)
						}

						if resp != nil {
							results = append(results, resp)
						}
						continue
					}

					resp = s.processRequest(ctx, &req, isAdmin)
					if resp != nil {
						results = append(results, resp)
					}
				}
			}
		}
	}

	var msg = []byte{}
	if batchedRequest && batchSize > 0 {
		if len(results) > 0 {
			// Form the batched response json
			var buffer bytes.Buffer
			buffer.WriteByte('[')
			for idx, reply := range results {
				if idx == len(results)-1 {
					buffer.Write(reply)
					buffer.WriteByte(']')
					break
				}
				buffer.Write(reply)
				buffer.WriteByte(',')
			}
			msg = buffer.Bytes()
		}
	}

	if !batchedRequest || batchSize == 0 {
		// Respond with the first results entry for single requests
		if len(results) > 0 {
			msg = results[0]
		}
	}

	// Write the response.
	err = s.writeHTTPResponseHeaders(r, w.Header(), http.StatusOK, buf)
	if err != nil {
		log.Error(err)
		return
	}
	if _, err := buf.Write(msg); err != nil {
		log.Errorf("Failed to write marshalled reply: %v", err)
	}

	// Terminate with newline to maintain compatibility with Bitcoin Core.
	if err := buf.WriteByte('\n'); err != nil {
		log.Errorf("Failed to append terminating newline to reply: %v", err)
	}
}

// jsonAuthFail sends a message back to the client if the http auth is rejected.
func jsonAuthFail(w http.ResponseWriter) {
	w.Header().Add("WWW-Authenticate", `Basic realm="dcrd RPC"`)
	http.Error(w, "401 Unauthorized.", http.StatusUnauthorized)
}

// logForwarder provides logic to forward log messages writing to an io.Writer
// to the rpcserver logger.
type logForwarder struct{}

// Write implements the io.Writer interface and forwards the message to the
// active rpcserver logger.
func (logForwarder) Write(p []byte) (int, error) {
	log.Error(strings.TrimRight(string(p), "\r\n"))
	return len(p), nil
}

// equalASCIIFold returns true if s is equal to t with ASCII case folding as
// defined in RFC 4790.  This function was lifted and from the gorilla websocket
// code since it's not exported.
func equalASCIIFold(s, t string) bool {
	for s != "" && t != "" {
		sr, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		tr, size := utf8.DecodeRuneInString(t)
		t = t[size:]
		if sr == tr {
			continue
		}
		if 'A' <= sr && sr <= 'Z' {
			sr = sr + 'a' - 'A'
		}
		if 'A' <= tr && tr <= 'Z' {
			tr = tr + 'a' - 'A'
		}
		if sr != tr {
			return false
		}
	}
	return s == t
}

// route sets up the endpoints of the rpc server.
func (s *Server) route(ctx context.Context) *http.Server {
	rpcServeMux := http.NewServeMux()
	httpServer := &http.Server{
		Handler: rpcServeMux,

		// Use the provided context as the parent context for all requests to
		// ensure handlers are able to react to both client disconnects as well
		// as shutdown via the provided context.
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},

		// Timeout connections which don't complete the initial
		// handshake within the allowed timeframe.
		ReadTimeout: time.Second * rpcAuthTimeoutSeconds,

		// Reroute http server error logging through the rpcserver
		// logger.
		ErrorLog: stdlog.New(logForwarder{}, "", 0),
	}
	rpcServeMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		w.Header().Set("Content-Type", "application/json")
		r.Close = true

		// Limit the number of connections to max allowed.
		if s.limitConnections(w, r.RemoteAddr) {
			return
		}

		// Keep track of the number of connected clients.
		s.incrementClients()
		defer s.decrementClients()
		_, isAdmin, err := s.checkAuth(r, true)
		if err != nil {
			jsonAuthFail(w)
			return
		}

		// Read and respond to the request.
		s.jsonRPCRead(r.Context(), w, r, isAdmin)
	})

	// Websocket endpoint.
	rpcServeMux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		authenticated, isAdmin, err := s.checkAuth(r, false)
		if err != nil {
			jsonAuthFail(w)
			return
		}

		// Attempt to upgrade the connection to a websocket connection using the
		// default size for read/write buffers and impose a read limit that
		// depends on whether or not the connection is authenticated yet.
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Allow requests with no origin header set.
				origin := r.Header["Origin"]
				if len(origin) == 0 {
					return true
				}

				// Reject requests with origin headers that are not valid URLs.
				originURL, err := url.Parse(origin[0])
				if err != nil {
					return false
				}

				// Allow local resources on browsers that set the origin header
				// for them.  In particular:
				// - Firefox which sets it to "null"
				// - Chrome which sets it to "file://"
				// - Edge which sets it to "file://"
				if originURL.Scheme == "file" || originURL.Path == "null" {
					return true
				}

				// Strip the port from both the origin and request hosts.
				originHost := originURL.Host
				requestHost := r.Host
				if host, _, err := net.SplitHostPort(originHost); err != nil {
					originHost = host
				}
				if host, _, err := net.SplitHostPort(requestHost); err != nil {
					requestHost = host
				}

				// Reject mismatched hosts.
				return equalASCIIFold(originHost, requestHost)
			},
		}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			var herr websocket.HandshakeError
			if !errors.As(err, &herr) {
				log.Errorf("Unexpected websocket error: %v", err)
			}
			return
		}
		ws.SetPingHandler(func(payload string) error {
			log.Debugf("ping received: len %d", len(payload))
			log.Tracef("ping payload: %q", payload)
			var netErr net.Error
			err := ws.WriteControl(websocket.PongMessage, []byte(payload),
				time.Now().Add(websocketPongTimeout))
			if err != nil && !errors.Is(err, websocket.ErrCloseSent) &&
				!(errors.As(err, &netErr) && netErr.Timeout()) {

				log.Errorf("Failed to send pong: %v", err)
				return err
			}
			return nil
		})
		ws.SetPongHandler(func(payload string) error {
			log.Debugf("pong received: len %d", len(payload))
			log.Tracef("pong payload: %q", payload)
			return nil
		})
		if !authenticated {
			ws.SetReadLimit(websocketReadLimitUnauthenticated)
		} else {
			ws.SetReadLimit(websocketReadLimitAuthenticated)
		}
		s.WebsocketHandler(r.Context(), ws, r.RemoteAddr, authenticated,
			isAdmin)
	})
	return httpServer
}

// Run starts the rpc server and its listeners. It blocks until the
// provided context is cancelled.
func (s *Server) Run(ctx context.Context) {
	log.Trace("Starting RPC server")
	server := s.route(ctx)
	var wg sync.WaitGroup
	for _, listener := range s.cfg.Listeners {
		wg.Add(1)
		go func(listener net.Listener) {
			log.Infof("RPC server listening on %s", listener.Addr())
			server.Serve(listener)
			log.Tracef("RPC listener done for %s", listener.Addr())
			wg.Done()
		}(listener)
	}

	// Subscribe for async work notifications when background template
	// generation is enabled.
	if len(s.cfg.MiningAddrs) > 0 && s.cfg.BlockTemplater != nil {
		wg.Add(1)
		go func(s *Server, ctx context.Context) {
			templateSub := s.cfg.BlockTemplater.Subscribe()
			defer templateSub.Stop()

			for {
				select {
				case templateNtfn := <-templateSub.C():
					s.ntfnMgr.NotifyWork(templateNtfn)

				case <-ctx.Done():
					wg.Done()
					return
				}
			}
		}(s, ctx)
	}

	// Run the notification manager and wait for it to terminate.
	s.ntfnMgr.Run(ctx)

	// Close all listeners and wait for all goroutines to terminate.
	log.Warnf("RPC server shutting down")
	var hasCloseErr bool
	for _, listener := range s.cfg.Listeners {
		err := listener.Close()
		if err != nil {
			log.Errorf("Failed to close listener %s: %v", listener.Addr(), err)
			hasCloseErr = true
		}
	}
	if !hasCloseErr {
		wg.Wait()
	}
	log.Info("RPC server shutdown complete")
}

// Config is a descriptor containing the RPC server configuration.
type Config struct {
	// Listeners defines a slice of listeners for which the RPC server will
	// take ownership of and accept connections.  Since the RPC server takes
	// ownership of these listeners, they will be closed when the RPC server
	// is stopped.
	Listeners []net.Listener

	// StartupTime is the unix timestamp for when the server that is hosting
	// the RPC server started.
	StartupTime int64

	// ProfilerMgr defines the profile server manager for the RPC server to use.
	// It provides the RPC server with the means to dynamically start and stop
	// an HTTP profile server on a separate listening address.
	ProfilerMgr ProfilerManager

	// ConnMgr defines the connection manager for the RPC server to use.  It
	// provides the RPC server with a means to do things such as add,
	// remove, connect, disconnect, and query peers as well as other
	// connection-related data and tasks.
	ConnMgr ConnManager

	// SyncMgr defines the sync manager for the RPC server to use.
	SyncMgr SyncManager

	// ExistsAddresser defines the exist addresser for the RPC server to
	// use.
	ExistsAddresser ExistsAddresser

	// These fields allow the RPC server to interface with the local block
	// chain data and state.
	TimeSource    blockchain.MedianTimeSource
	Chain         Chain
	SanityChecker SanityChecker
	ChainParams   *chaincfg.Params
	DB            database.DB
	FeeEstimator  FeeEstimator
	Services      wire.ServiceFlag

	// SubsidyCache defines a cache for efficient access to consensus-critical
	// subsidy calculations.
	SubsidyCache *standalone.SubsidyCache

	// AddrManager defines a concurrency safe address manager for caching
	// potential peers on the network.
	AddrManager AddrManager

	// Clock defines the clock for the RPC server to use.
	Clock Clock

	// TxMempooler defines the transaction memory pool to interact with.
	TxMempooler TxMempooler

	// These fields allow the RPC server to interface with mining.
	//
	// BlockTemplater generates block templates, CPUMiner solves
	// templates using the CPU.  CPU mining is typically only useful
	// for test purposes when doing regression or simulation testing.
	BlockTemplater BlockTemplater
	CPUMiner       CPUMiner

	// TxIndexer defines the optional transaction indexer for the RPC server to
	// use.
	TxIndexer TxIndexer

	// NetInfo defines a slice of the available networks.
	NetInfo []types.NetworksResult

	// MinRelayTxFee defines the minimum transaction fee in Atoms/1000 bytes to be
	// considered a non-zero fee.
	MinRelayTxFee dcrutil.Amount

	// Proxy defines the proxy that is being used for connections.
	Proxy string

	// These fields define the username and password for RPC connections and
	// limited RPC connections.
	RPCUser      string
	RPCPass      string
	RPCLimitUser string
	RPCLimitPass string

	// RPCMaxClients defines the max number of RPC clients for standard
	// connections.
	RPCMaxClients int

	// RPCMaxConcurrentReqs defines the max number of RPC requests that may be
	// processed concurrently.
	RPCMaxConcurrentReqs int

	// RPCMaxWebsockets defines the max number of RPC websocket connections.
	RPCMaxWebsockets int

	// TestNet represents whether or not the server is using testnet.
	TestNet bool

	// MiningAddrs is a list of payment addresses to use for the generated blocks.
	MiningAddrs []stdaddr.Address

	// AllowUnsyncedMining indicates whether block templates should be created even
	// when the chain is not fully synced.
	AllowUnsyncedMining bool

	// MaxProtocolVersion is the max protocol version that the server supports.
	MaxProtocolVersion uint32

	// UserAgentVersion is the user agent version and is used to help identify
	// ourselves to other peers.
	UserAgentVersion string

	// LogManager defines the log manager for the RPC server to use.
	LogManager LogManager

	// FiltererV2 defines the V2 filterer for the RPC server to use.
	FiltererV2 FiltererV2

	// MixPooler defines the mixpool for the RPC server to use.
	MixPooler MixPooler
}

// New returns a new instance of the Server struct.
func New(config *Config) (*Server, error) {
	rpc := Server{
		cfg:                    *config,
		statusLines:            make(map[int]string),
		workState:              newWorkState(),
		helpCacher:             newHelpCacher(),
		requestProcessShutdown: make(chan struct{}),
		blake256Hasher:         blake256.New(),
	}
	key := make([]byte, 32)
	rand.Read(key)
	rpc.hmac = hmac.New(sha256.New, key)
	if config.RPCUser != "" && config.RPCPass != "" {
		login := config.RPCUser + ":" + config.RPCPass
		auth := "Basic " +
			base64.StdEncoding.EncodeToString([]byte(login))
		rpc.authMAC(rpc.authsha[:0], []byte(auth))
	}
	if config.RPCLimitUser != "" && config.RPCLimitPass != "" {
		login := config.RPCLimitUser + ":" + config.RPCLimitPass
		auth := "Basic " +
			base64.StdEncoding.EncodeToString([]byte(login))
		rpc.authMAC(rpc.limitauthsha[:0], []byte(auth))
	}
	rpc.ntfnMgr = newWsNotificationManager(&rpc)

	return &rpc, nil
}

func init() {
	rpcHandlers = rpcHandlersBeforeInit

	// blake256Pad is the extra blake256 internal padding needed for the
	// data of the getwork RPC.  The internal blake256 padding consists of
	// a single 1 bit followed by zeros and a final 1 bit in order to pad
	// the message out to 56 bytes followed by length of the message in
	// bits encoded as a big-endian uint64 (8 bytes).  Thus, the resulting
	// length is a multiple of the blake256 block size (64 bytes).  Since
	// the block header is a fixed size, it only needs to be calculated
	// once.
	blake256Pad = make([]byte, getworkDataLenBlake256-wire.MaxBlockHeaderPayload)
	blake256Pad[0] = 0x80
	blake256Pad[len(blake256Pad)-9] |= 0x01
	binary.BigEndian.PutUint64(blake256Pad[len(blake256Pad)-8:],
		wire.MaxBlockHeaderPayload*8)
}
