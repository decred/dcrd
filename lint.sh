#!/usr/bin/env bash

set -ex

# The script uses golangci-lint (github.com/golangci/golangci-lint) to run all
# linters defined by the configuration in .golangci.yml on every module in the
# repository.

go version

# loop all modules
MODULES=$(find . -name go.mod -not -path "./playground/*" -not -path "./*/_asm/*")
for module in $MODULES; do
  # determine module name/directory
  MODNAME=$(echo $module | sed -E -e 's,/go\.mod$,,' -e 's,^./,,')
  if [ -z "$MODNAME" ]; then
    MODNAME=.
  fi

  echo "==> lint ${MODNAME}"

  # run commands in the module directory as a subshell
  (
    cd $MODNAME

    # run linters
    golangci-lint run
  )
done
