// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

// BlockOneLedgerRegNet is the block one output ledger for the regression test
// network.  See "Decred organization related parameters" in regnetparams.go for
// information on how to spend these outputs.
var BlockOneLedgerRegNet = []*TokenPayout{
	{"RsKrWb7Vny1jnzL1sDLgKTAteh9RZcRr5g6", 100000 * 1e8},
	{"Rs8ca5cDALtsMVD4PV3xvFTC7dmuU1juvLv", 100000 * 1e8},
	{"RsHzbGt6YajuHpurtpqXXHz57LmYZK8w9tX", 100000 * 1e8},
}
