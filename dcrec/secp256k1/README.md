secp256k1
=====

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/dcrec/secp256k1/v3)

Package dcrec implements elliptic curve cryptography needed for working with
Decred (secp256k1 only for now). It is designed so that it may be used with the
standard crypto/ecdsa packages provided with go.  A comprehensive suite of test
is provided to ensure proper functionality.  Package dcrec was originally based
on work from ThePiachu which is licensed under the same terms as Go, but it has
signficantly diverged since then.  The Decred developers original is licensed
under the liberal ISC license.

Although this package was primarily written for dcrd, it has intentionally been
designed so it can be used as a standalone package for any projects needing to
use secp256k1 elliptic curve cryptography.

## Installation and Updating

```bash
$ go get -u github.com/decred/dcrd/dcrec
```

## Examples

* [Encryption](https://pkg.go.dev/github.com/decred/dcrd/dcrec/secp256k1/v3#example-package-EncryptMessage)  
  Demonstrates encrypting a message for a public key that is first parsed from
  raw bytes, then decrypting it using the corresponding private key.

* [Decryption](https://pkg.go.dev/github.com/decred/dcrd/dcrec/secp256k1/v3#example-package-DecryptMessage)  
  Demonstrates decrypting a message using a private key that is first parsed
  from raw bytes.

## License

Package dcrec is licensed under the [copyfree](http://copyfree.org) ISC License
except for dcrec.go and dcrec_test.go which is under the same license as Go.

