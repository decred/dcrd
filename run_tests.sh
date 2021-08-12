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
ROOTPATH=$(go list -m)
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

    # run `go mod download` and `go mod tidy` and fail if the git status of
    # go.mod and/or go.sum changes
    MOD_STATUS=$(git status --porcelain go.mod go.sum)
    go mod download
    go mod tidy
    UPDATED_MOD_STATUS=$(git status --porcelain go.mod go.sum)
    if [ "$UPDATED_MOD_STATUS" != "$MOD_STATUS" ]; then
      echo 'Running `go mod tidy` modified go.mod and/or go.sum'
      exit 1
    fi

    # run linters
    golangci-lint run --build-tags=rpctest --disable-all --deadline=10m \
      --enable=gofmt \
      --enable=gosimple \
      --enable=unconvert \
      --enable=ineffassign \
      --enable=govet \
      --enable=misspell \
      --enable=deadcode \
  )
done

echo "------------------------------------------"
echo "Tests completed successfully!"
