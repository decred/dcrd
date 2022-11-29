rpctests
====


[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/internal/integration/rpctests)

Package rpctests provides integration-level tests for dcrd that rely on its RPC
service. The tests in this package exercise features and behaviors of the fully
compiled binary.

Most of the tests in this package are only executed if a corresponding `rpctest`
tag is specified during test execution. For example:

```shell
$ go test -tags rpctest .
```

## License

Package rpctests is licensed under the [copyfree](http://copyfree.org) ISC
License.
