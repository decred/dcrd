#!/usr/bin/env bash

set -ex

# This script runs the tests for all packages in all Go modules in the
# repository and then runs the linters for all Go modules in the repository by
# invoking the separate linter script.

go version

# run tests on all modules
MODULES=$(find . -name go.mod -not -path "./playground/*" -not -path "./*/_asm/*")
for module in $MODULES; do
  # determine module name/directory
  MODNAME=$(echo $module | sed -E -e 's,/go\.mod$,,' -e 's,^./,,')
  if [ -z "$MODNAME" ]; then
    MODNAME=.
  fi

  echo "==> test ${MODNAME}"

  # run commands in the module directory as a subshell
  (
    cd $MODNAME

    # run tests
    go test -short -tags rpcserver ./... -- "$@"
  )
done

# run linters on all modules
. ./lint.sh

echo "------------------------------------------"
echo "Tests completed successfully!"
