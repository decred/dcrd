// Copyright (c) 2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

import "github.com/decred/dcrd/rpc/jsonrpc/types"

// SessionResult models the data from the session command.
type SessionResult = types.SessionResult

// RescanResult models the result object returned by the rescan RPC.
type RescanResult = types.RescanResult

// RescannedBlock contains the hash and all discovered transactions of a single
// rescanned block.
type RescannedBlock = types.RescannedBlock
