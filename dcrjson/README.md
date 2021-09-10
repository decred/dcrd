dcrjson
=======

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/dcrjson/v4)

Package dcrjson implements infrastructure for marshalling to and from the decred
JSON-RPC API via concrete types.  A comprehensive suite of tests is provided to
ensure proper functionality.

Although this package was primarily written for the decred, it has intentionally
been designed so it can be used as a standalone package for any projects needing
to marshal to and from decred JSON-RPC requests and responses.

Note that although it's possible to use this package directly to implement an
RPC client, it is not recommended since it is only intended as an infrastructure
package.  Instead, RPC clients should use the
[rpcclient](https://github.com/decred/dcrd/tree/master/rpcclient) package which
provides a full blown RPC client with many features such as automatic connection
management, websocket support, automatic notification re-registration on
reconnect, and conversion from the raw underlying RPC types (strings, floats,
ints, etc) to higher-level types with many nice and useful properties.

## Installation and Updating

This package is part of the `github.com/decred/dcrd/dcrjson/v4` module.  Use the
standard go tooling for working with modules to incorporate it.

## Examples

* [Marshal Command](https://pkg.go.dev/github.com/decred/dcrd/dcrjson/v4#example-MarshalCmd)
  Demonstrates how to create and marshal a command into a JSON-RPC request.

* [Parse Command](https://pkg.go.dev/github.com/decred/dcrd/dcrjson/v4#example-ParseParams)
  Demonstrates how to unmarshal a JSON-RPC request and then parse the params
  of the concrete request into a concrete command.

* [Marshal Response](https://pkg.go.dev/github.com/decred/dcrd/dcrjson/v4#example-MarshalResponse)
  Demonstrates how to marshal a JSON-RPC response.

* [Unmarshal Response](https://pkg.go.dev/github.com/decred/dcrd/dcrjson/v4#example-package-UnmarshalResponse)
  Demonstrates how to unmarshal a JSON-RPC response and then unmarshal the
  result field in the response to a concrete type.

## License

Package dcrjson is licensed under the [copyfree](http://copyfree.org) ISC
License.
