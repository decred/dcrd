chaincfg
========

[![Build Status](https://github.com/decred/dcrd/workflows/Build%20and%20Test/badge.svg)](https://github.com/decred/dcrd/actions)
[![ISC License](https://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/github.com/decred/dcrd/chaincfg)

Package chaincfg defines chain configuration parameters for the four standard
Decred networks.

Although this package was primarily written for dcrd, it has intentionally been
designed so it can be used as a standalone package for any projects needing to
use parameters for the standard Decred networks or for projects needing to
define their own network.

## Sample Use

```Go
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/decred/dcrd/dcrutil/v2"
	"github.com/decred/dcrd/chaincfg/v2"
)

var testnet = flag.Bool("testnet", false, "operate on the testnet Decred network")

// By default (without -testnet), use mainnet.
var chainParams = chaincfg.MainNetParams()

func main() {
	flag.Parse()

	// Modify active network parameters if operating on testnet.
	if *testnet {
		chainParams = chaincfg.TestNet3Params()
	}

	// later...

	// Create and print new payment address, specific to the active network.
	pubKeyHash := make([]byte, 20)
	addr, err := btcutil.NewAddressPubKeyHash(pubKeyHash, chainParams)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(addr)
}
```

## Installation and Updating

```bash
$ go get -u github.com/decred/dcrd/chaincfg
```

## License

Package chaincfg is licensed under the [copyfree](http://copyfree.org) ISC
License.
