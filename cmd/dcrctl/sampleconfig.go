// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

// FileContents is a string containing the commented example config for dcrctl.
const FileContents = `[Application Options]

; ------------------------------------------------------------------------------
; Network settings
; ------------------------------------------------------------------------------

; Use testnet (cannot be used with simnet=1).
; testnet=1

; Use simnet (cannot be used with testnet=1).
; simnet=1


; ------------------------------------------------------------------------------
; RPC client settings
; ------------------------------------------------------------------------------

; Connect via a SOCKS5 proxy.
; proxy=127.0.0.1:9050
; proxyuser=
; proxypass=

; Username and password to authenticate connections to a Decred RPC server
; (usually dcrd or dcrwallet)
; rpcuser=
; rpcpass=

; RPC server to connect to
; rpcserver=localhost

; Wallet RPC server to connect to
; walletrpcserver=localhost

; RPC server certificate chain file for validation
; rpccert=~/.dcrd/rpc.cert
`
