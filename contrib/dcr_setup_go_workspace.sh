#!/usr/bin/env bash
#
# Copyright (c) 2022 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.
#
# Script to initialize a multi-module workspace that includes all of the modules
# provided by the dcrd repository.

set -e

# Ensure the script is run from either the root of the repo or the contrib dir
SCRIPT=$(basename $0)
MAIN_CODE_FILE="dcrd.go"
if [ -f "../${MAIN_CODE_FILE}" ]; then
  cd ..
fi
if [ ! -f "${MAIN_CODE_FILE}" ]; then
  echo "$SCRIPT: error: ${MAIN_CODE_FILE} not found in the current directory"
  exit 1
fi

# Verify Go is available
if ! type go >/dev/null 2>&1; then
  echo -n "$SCRIPT: error: Unable to find 'go' in the system path."
  exit 1
fi

# Create workspace unless one already exists
if [ ! -f "go.work" ]; then
  go work init
fi

# Add all of the modules as needed
go work use . ./addrmgr ./bech32 ./blockchain ./blockchain/stake
go work use ./blockchain/standalone ./certgen ./chaincfg ./chaincfg/chainhash
go work use ./connmgr ./container/apbf ./crypto/blake256 ./crypto/ripemd160
go work use ./database ./dcrec ./dcrec/edwards ./dcrec/secp256k1 ./dcrjson
go work use ./dcrutil ./gcs ./hdkeychain ./lru ./math/uint256 ./mixing ./peer
go work use ./rpc/jsonrpc/types ./rpcclient ./txscript ./wire
