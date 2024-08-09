#!/usr/bin/env bash
#
# Copyright (c) 2024 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.
#
# Script to update the image in the Dockerfile to a given image name and
# associated hash.

set -e

display_usage() {
  echo "Usage: $0 <docker_image> <image_hash>"
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
if [ $# -ne 2 ]; then
  display_usage
  exit 1
fi
DOCKER_IMAGE=$1
GO_VER=${DOCKER_IMAGE##golang:}
GO_VER=${GO_VER%%-*}
DOCKER_IMAGE_HASH=$2

# Ensure the script is run from either the root of the repo or the devtools dir.
DOCKER_FILE=contrib/docker/Dockerfile
if [ -f "../../$DOCKER_FILE" ]; then
  cd ../..
fi
if [ ! -f "$DOCKER_FILE" ]; then
  echo "$DOCKER_FILE does not exist"
  exit 1
fi

# Ensure git is available.
if ! command -v git &> /dev/null; then
  echo "git could not be found"
  exit 1
fi

# Ensure the branch does not already exist.
BRANCH_NAME="contrib_docker_go${GO_VER//\./_}"
if $(git show-ref --verify --quiet refs/heads/${BRANCH_NAME}); then
  echo "$BRANCH_NAME already exists"
  exit 1
fi

# Create the branch.
git checkout -b "$BRANCH_NAME" master

# Modify the Dockerfile with the updated image details.
sed -Eie "s/golang:[0-9]\.[0-9]+\.[0-9]+\-alpine[0-9]+\.[0-9]+/${DOCKER_IMAGE}/" "$DOCKER_FILE"
sed -Eie "s/golang@sha256:[0-9a-f]{64}/golang@sha256:${DOCKER_IMAGE_HASH}/" "$DOCKER_FILE"

# Commit the changes with the appropriate message.
git add "$DOCKER_FILE"
git commit -m "docker: Update image to $DOCKER_IMAGE.

This updates the docker image to $DOCKER_IMAGE.

To confirm the new digest:

\`\`\`
$ docker pull $DOCKER_IMAGE
${DOCKER_IMAGE##golang:}: Pulling from library/golang
...
Digest: sha256:${DOCKER_IMAGE_HASH}
...
\`\`\`
"
