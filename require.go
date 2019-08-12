// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file exists to prevent go mod tidy from removing requires for newer
// module versions that are not yet fully integrated and to allow them to be
// automatically discovered by the testing infrastructure.
//
// It is excluded from the build to avoid including unused modules in the final
// binary.

package main

import (
	_ "github.com/decred/dcrd/blockchain/stake/v2"
	_ "github.com/decred/dcrd/blockchain/standalone"
	_ "github.com/decred/dcrd/blockchain/v2"
	_ "github.com/decred/dcrd/chaincfg/v2"
	_ "github.com/decred/dcrd/connmgr/v2"
	_ "github.com/decred/dcrd/crypto/blake256"
	_ "github.com/decred/dcrd/database/v2"
	_ "github.com/decred/dcrd/dcrutil/v2"
	_ "github.com/decred/dcrd/mempool/v3"
	_ "github.com/decred/dcrd/mining/v2"
	_ "github.com/decred/dcrd/peer/v2"
	_ "github.com/decred/dcrd/txscript/v2"
)
