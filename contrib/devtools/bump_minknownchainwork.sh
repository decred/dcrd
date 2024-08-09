#!/usr/bin/env bash
#
# Copyright (c) 2024 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.
#
# Script to update the minimum known chain work value for both the main and test
# networks.

set -e

display_usage() {
  echo "Usage: $0"
  echo " Switches:"
  echo "  -h    help"
}

# Parse option flags.
while getopts "h" arg; do
  case $arg in
    h)
      display_usage
      exit 0
      ;;
    *)
      display_usage
      exit 1
      ;;
  esac
done
shift $((OPTIND-1))

# Check arguments and set vars with them.
if [ $# -ne 0 ]; then
  display_usage
  exit 1
fi

# Ensure the script is run from either the root of the repo or the devtools dir.
SCRIPT=$(basename $0)
MAIN_CODE_FILE="dcrd.go"
if [ -f "../../${MAIN_CODE_FILE}" ]; then
  cd ../..
fi
if [ ! -f "${MAIN_CODE_FILE}" ]; then
  echo "$SCRIPT: error: ${MAIN_CODE_FILE} not found in the current directory"
  exit 1
fi

# Ensure chaincfg files exist at the expected paths.
MAINNET_PARAMS_FILE=chaincfg/mainnetparams.go
if [ ! -f "$MAINNET_PARAMS_FILE" ]; then
  echo "$MAINNET_PARAMS_FILE does not exist"
  exit 1
fi
TESTNET_PARAMS_FILE=chaincfg/testnetparams.go
if [ ! -f "$TESTNET_PARAMS_FILE" ]; then
  echo "$TESTNET_PARAMS_FILE does not exist"
  exit 1
fi

# Ensure git, dcrctl, and jq are available and this is running in a git repo.
if ! git status &> /dev/null; then
  echo "git repository not be found"
  exit 1
fi
if ! command -v dcrctl &> /dev/null; then
  echo "dcrctl could not be found"
  exit 1
fi
if ! command -v jq &> /dev/null; then
  echo "jq could not be found"
  exit 1
fi

# Ensure the branch does not already exist.
BRANCH_NAME="chaincfg_update_min_known_chain_work"
if $(git show-ref --verify --quiet refs/heads/${BRANCH_NAME}); then
  echo "$BRANCH_NAME already exists"
  exit 1
fi

# Determine the best minimum known work to use.
dcrctl="dcrctl"
dcrctlt="dcrctl --testnet"
MAINNET_BLOCK_HEIGHT=$($dcrctl getblockcount)
MAINNET_BLOCK_HASH=$($dcrctl getblockhash $MAINNET_BLOCK_HEIGHT)
MAINNET_CHAIN_WORK=$($dcrctl getblockheader $MAINNET_BLOCK_HASH | jq -r .chainwork)
TESTNET_BLOCK_HEIGHT=$($dcrctlt getblockcount)
TESTNET_BLOCK_HASH=$($dcrctlt getblockhash $TESTNET_BLOCK_HEIGHT)
TESTNET_CHAIN_WORK=$($dcrctlt getblockheader $TESTNET_BLOCK_HASH | jq -r .chainwork)

# Create the branch.
git checkout -b "$BRANCH_NAME" master

# Modify the main and test network params files with the updated details.
sed -Ei \
    -e '1h;2,$H;$!d;g' \
    -e "s/(\\t\\t\\/\\/ Block )[0-9a-f]{64}(\\n\\t\\t\\/\\/ Height: )[0-9]+(\\n\\t\\tMinKnownChainWork: hexToBigInt\\(\\\")[0-9a-f]{64}(\\\"\\),)/\\1${MAINNET_BLOCK_HASH}\\2${MAINNET_BLOCK_HEIGHT}\\3${MAINNET_CHAIN_WORK}\\4/" \
    "$MAINNET_PARAMS_FILE"
sed -Ei \
    -e '1h;2,$H;$!d;g' \
    -e "s/(\\t\\t\\/\\/ Block )[0-9a-f]{64}(\\n\\t\\t\\/\\/ Height: )[0-9]+(\\n\\t\\tMinKnownChainWork: hexToBigInt\\(\\\")[0-9a-f]{64}(\\\"\\),)/\\1${TESTNET_BLOCK_HASH}\\2${TESTNET_BLOCK_HEIGHT}\\3${TESTNET_CHAIN_WORK}\\4/" \
    "$TESTNET_PARAMS_FILE"

# Commit the changes with the appropriate message.
git add "$MAINNET_PARAMS_FILE" "$TESTNET_PARAMS_FILE"
git commit -m "chaincfg: Update min known chain work for release.

This updates the minimum known chain work values for the main and test
networks as follows:

mainnet: 0x${MAINNET_CHAIN_WORK}
testnet: 0x${TESTNET_CHAIN_WORK}

The following commands may be used to verify:
\`\`\`
\$ dcrctl getblockhash ${MAINNET_BLOCK_HEIGHT} | dcrctl getblockheader - | jq -r .chainwork
${MAINNET_CHAIN_WORK}
\$ dcrctl --testnet getblockhash ${TESTNET_BLOCK_HEIGHT} | dcrctl --testnet getblockheader - | jq -r .chainwork
${TESTNET_CHAIN_WORK}
\`\`\`
"
