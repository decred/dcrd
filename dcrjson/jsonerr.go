// Copyright (c) 2013-2014 Conformal Systems LLC.
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

// Standard JSON-RPC 2.0 errors
var (
	ErrInvalidRequest = RPCError{
		Code:    -32600,
		Message: "Invalid request",
	}
	ErrMethodNotFound = RPCError{
		Code:    -32601,
		Message: "Method not found",
	}
	ErrInvalidParams = RPCError{
		Code:    -32602,
		Message: "Invalid parameters",
	}
	ErrInternal = RPCError{
		Code:    -32603,
		Message: "Internal error",
	}
	ErrParse = RPCError{
		Code:    -32700,
		Message: "Parse error",
	}
)

// General application defined JSON errors
var (
	ErrMisc = RPCError{
		Code:    -1,
		Message: "Miscellaneous error",
	}
	ErrForbiddenBySafeMode = RPCError{
		Code:    -2,
		Message: "Server is in safe mode, and command is not allowed in safe mode",
	}
	ErrType = RPCError{
		Code:    -3,
		Message: "Unexpected type was passed as parameter",
	}
	ErrInvalidAddressOrKey = RPCError{
		Code:    -5,
		Message: "Invalid address or key",
	}
	ErrOutOfMemory = RPCError{
		Code:    -7,
		Message: "Ran out of memory during operation",
	}
	ErrInvalidParameter = RPCError{
		Code:    -8,
		Message: "Invalid, missing or duplicate parameter",
	}
	ErrDatabase = RPCError{
		Code:    -20,
		Message: "Database error",
	}
	ErrDeserialization = RPCError{
		Code:    -22,
		Message: "Error parsing or validating structure in raw format",
	}
)

// Peer-to-peer client errors
var (
	ErrClientNotConnected = RPCError{
		Code:    -9,
		Message: "node is not connected",
	}
	ErrClientInInitialDownload = RPCError{
		Code:    -10,
		Message: "node is downloading blocks...",
	}
)

// Wallet JSON errors
var (
	ErrWallet = RPCError{
		Code:    -4,
		Message: "Unspecified problem with wallet",
	}
	ErrWalletInsufficientFunds = RPCError{
		Code:    -6,
		Message: "Not enough funds in wallet or account",
	}
	ErrWalletInvalidAccountName = RPCError{
		Code:    -11,
		Message: "Invalid account name",
	}
	ErrWalletKeypoolRanOut = RPCError{
		Code:    -12,
		Message: "Keypool ran out, call keypoolrefill first",
	}
	ErrWalletUnlockNeeded = RPCError{
		Code:    -13,
		Message: "Enter the wallet passphrase with walletpassphrase first",
	}
	ErrWalletPassphraseIncorrect = RPCError{
		Code:    -14,
		Message: "The wallet passphrase entered was incorrect",
	}
	ErrWalletWrongEncState = RPCError{
		Code:    -15,
		Message: "Command given in wrong wallet encryption state",
	}
	ErrWalletEncryptionFailed = RPCError{
		Code:    -16,
		Message: "Failed to encrypt the wallet",
	}
	ErrWalletAlreadyUnlocked = RPCError{
		Code:    -17,
		Message: "Wallet is already unlocked",
	}
)

// Specific Errors related to commands.  These are the ones a user of the rpc
// server are most likely to see.  Generally, the codes should match one of the
// more general errors above.
var (
	ErrBlockNotFound = RPCError{
		Code:    -5,
		Message: "Block not found",
	}
	ErrBlockCount = RPCError{
		Code:    -5,
		Message: "Error getting block count",
	}
	ErrBestBlockHash = RPCError{
		Code:    -5,
		Message: "Error getting best block hash",
	}
	ErrDifficulty = RPCError{
		Code:    -5,
		Message: "Error getting difficulty",
	}
	ErrOutOfRange = RPCError{
		Code:    -1,
		Message: "Block number out of range",
	}
	ErrNoTxInfo = RPCError{
		Code:    -5,
		Message: "No information available about transaction",
	}
	ErrNoNewestBlockInfo = RPCError{
		Code:    -5,
		Message: "No information about newest block",
	}
	ErrInvalidTxVout = RPCError{
		Code:    -5,
		Message: "Output index number (vout) does not exist for transaction.",
	}
	ErrRawTxString = RPCError{
		Code:    -32602,
		Message: "Raw tx is not a string",
	}
	ErrDecodeHexString = RPCError{
		Code:    -22,
		Message: "Unable to decode hex string",
	}
)

// Errors that are specific to dcrd.
var (
	ErrNoWallet = RPCError{
		Code:    -1,
		Message: "This implementation does not implement wallet commands",
	}
	ErrUnimplemented = RPCError{
		Code:    -1,
		Message: "Command unimplemented",
	}
)
