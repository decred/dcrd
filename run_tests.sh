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
ROOTPATH=$(go list)
ROOTPATHPATTERN=$(echo $ROOTPATH | sed 's/\\/\\\\/g' | sed 's/\//\\\//g')
MODPATHS=$(go list -m all | grep "^$ROOTPATHPATTERN" | cut -d' ' -f1)
for module in $MODPATHS; do
  echo "==> ${module}"
  go test -short -tags rpctest ${module}/...

  # check linters
  MODNAME=$(echo $module | sed -E -e "s/^$ROOTPATHPATTERN//" \
    -e 's,^/,,' -e 's,/v[0-9]+$,,')
  if [ -z "$MODNAME" ]; then
    MODNAME=.
  fi

  # run commands in the module directory as a subshell
  (
    cd $MODNAME

    # run linters
    golangci-lint run --build-tags=rpctest --disable-all --deadline=10m \
      --enable=gofmt \
      --enable=gosimple \
      --enable=unconvert \
      --enable=ineffassign \
      --enable=govet \
      --enable=misspell \
      --enable=unused \
  )
done

echo "------------------------------------------"
echo "Tests completed successfully!"
