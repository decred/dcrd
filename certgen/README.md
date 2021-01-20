Certgen
======

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/certgen)

## Overview

This package contains functions for creating self-signed TLS certificate from
random new key pairs, typically used for encrypting RPC and websocket
communications.

ECDSA certificates are supported on all Go versions.  Beginning with Go 1.13,
this package additionally includes support for Ed25519 certificates.

## Installation and Updating

This package is part of the `github.com/decred/dcrd/certgen` module.  Use the
standard go tooling for working with modules to incorporate it.

## License

Package certgen is licensed under the [copyfree](http://copyfree.org) ISC
License.
