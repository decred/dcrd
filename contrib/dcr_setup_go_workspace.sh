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

# Remove old modules as needed
go work edit -dropuse ./lru

# Add all of the modules as needed
go work use . ./addrmgr ./bech32 ./blockchain ./blockchain/stake
go work use ./blockchain/standalone ./certgen ./chaincfg ./chaincfg/chainhash
go work use ./connmgr ./container/apbf ./container/lru ./crypto/blake256
go work use ./crypto/ripemd160 ./database ./dcrec ./dcrec/edwards
go work use ./dcrec/secp256k1 ./dcrjson ./dcrutil ./gcs ./hdkeychain
go work use ./math/uint256 ./mixing ./peer ./rpc/jsonrpc/types ./rpcclient
go work use ./txscript ./wire
