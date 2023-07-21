#!/usr/bin/env bash

set -ex

# This script runs the tests for all packages in all Go modules in the
# repository and then runs the linters for all Go modules in the repository by
# invoking the separate linter script.

go version

# run tests on all modules
echo "==> test all modules"
ROOTPKG=$(go list)
go test -short -tags rpctest $ROOTPKG/...

# run linters on all modules
. ./lint.sh

echo "------------------------------------------"
echo "Tests completed successfully!"
