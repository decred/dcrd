#!/bin/sh

set -ex

# The script does automatic checking on a Go package and its sub-packages,
# including:
# 1. gofmt         (https://golang.org/cmd/gofmt/)
# 2. gosimple      (https://github.com/dominikh/go-simple)
# 3. unconvert     (https://github.com/mdempsky/unconvert)
# 4. ineffassign   (https://github.com/gordonklaus/ineffassign)
# 5. go vet        (https://golang.org/cmd/vet)
# 6. misspell      (https://github.com/client9/misspell)

# golangci-lint (github.com/golangci/golangci-lint) is used to run each
# static checker.

go version

# run tests on all modules
echo "==> test all modules"
ROOTPKG=$(go list)
go test -short -tags rpctest $ROOTPKG/...

# loop all modules
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

echo "------------------------------------------"
echo "Tests completed successfully!"
