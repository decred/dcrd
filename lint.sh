#!/bin/sh

set -ex

# The script uses golangci-lint (github.com/golangci/golangci-lint) to run all
# linters defined by the configuration in .golangci.yml on every module in the
# repository.

go version

# loop all modules
ROOTPKG=$(go list)
ROOTPKGPATTERN=$(echo $ROOTPKG | sed 's,\\,\\\\,g' | sed 's,/,\\/,g')
MODPATHS=$(go list -m all | grep "^$ROOTPKGPATTERN" | cut -d' ' -f1)
for module in $MODPATHS; do
  echo "==> lint ${module}"

  # determine module directory
  MODNAME=$(echo $module | sed -E -e "s/^$ROOTPKGPATTERN//" \
    -e 's,^/,,' -e 's,/v[0-9]+$,,')
  if [ -z "$MODNAME" ]; then
    MODNAME=.
  fi

  # run commands in the module directory as a subshell
  (
    cd $MODNAME

    # run linters
    golangci-lint run
  )
done
