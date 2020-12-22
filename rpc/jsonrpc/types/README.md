jsonrpc/types
=============

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/rpc/jsonrpc/types/v3)

Package types implements concrete types for marshalling to and from the dcrd
JSON-RPC commands, return values, and notifications.  A comprehensive suite of
tests is provided to ensure proper functionality.

The provided types are automatically registered with
[dcrjson](https://github.com/decred/dcrd/tree/master/dcrjson) when the package
is imported.  Although this package was primarily written for dcrd, it has
intentionally been designed so it can be used as a standalone package for any
projects needing to marshal to and from dcrd JSON-RPC requests and responses.

## Installation and Updating

```bash
$ go get -u github.com/decred/dcrd/rpc/jsonrpc/types
```

## License

Package types is licensed under the [copyfree](http://copyfree.org) ISC License.
