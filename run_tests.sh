#!/bin/sh

set -ex

# This script runs the tests for all packages in all Go modules in the
# repository.
#
# It will also run the linters for all Go modules in the repository when not
# running as a GitHub action.

go version

# run tests on all modules
echo "==> test all modules"
ROOTPKG=$(go list)
go test -short -tags rpctest $ROOTPKG/...

[ -z "$GITHUB_ACTIONS" ] && ./lint.sh

echo "------------------------------------------"
echo "Tests completed successfully!"
