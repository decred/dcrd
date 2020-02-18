// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package certgen includes a common base for creating a
new TLS certificate key pair.

This package contains functions for creating self-signed TLS certificate from
random new key pairs, typically used for encrypting RPC and websocket
communications.

ECDSA certificates are supported on all Go versions.  Beginning with Go 1.13,
this package additionally includes support for Ed25519 certificates.
*/
package certgen
