#!/usr/bin/env bash

set -ex

# This script runs the tests for all packages in all Go modules in the
# repository.
#
# It also runs the linters for all Go modules in the repository by invoking the
# separate linter script when not running as a GitHub action.

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

# run linters on all modules when not running as a GitHub action
[ -z "$GITHUB_ACTIONS" ] && source ./lint.sh

echo "------------------------------------------"
echo "Tests completed successfully!"
