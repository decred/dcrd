txscript
========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![Doc](https://img.shields.io/badge/doc-reference-blue.svg)](https://pkg.go.dev/github.com/decred/dcrd/txscript/v4)

Package txscript implements the Decred transaction script language.  There is
a comprehensive test suite.

This package has intentionally been designed so it can be used as a standalone
package for any projects needing to use or validate Decred transaction scripts.

## Decred Scripts

Decred provides a stack-based, FORTH-like language for the scripts in the Decred
transactions.  This language is not Turing complete although it is still fairly
powerful.

## Installation and Updating

This package is part of the `github.com/decred/dcrd/txscript/v3` module.  Use
the standard go tooling for working with modules to incorporate it.

## Examples

* [Standard Pay-to-pubkey-hash Script](https://pkg.go.dev/github.com/decred/dcrd/txscript/v4#example-PayToAddrScript)
  Demonstrates creating a script which pays to a Decred address.  It also
  prints the created script hex and uses the DisasmString function to display
  the disassembled script.

* [Extracting Details from Standard Scripts](https://pkg.go.dev/github.com/decred/dcrd/txscript/v4#example-ExtractPkScriptAddrs)
  Demonstrates extracting information from a standard public key script.

* [Counting Opcodes in Scripts](https://pkg.go.dev/github.com/decred/dcrd/txscript/v4#example-ScriptTokenizer)
  Demonstrates creating a script tokenizer instance and using it to count the
  number of opcodes a script contains.

## License

Package txscript is licensed under the [copyfree](http://copyfree.org) ISC
License.
