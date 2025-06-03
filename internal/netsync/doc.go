// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package netsync implements a concurrency safe block syncing protocol.

The provided implementation of SyncManager communicates with connected peers to
perform an initial chain sync, keep the chain in sync, and announce new blocks
connected to the chain.  Currently the sync manager selects a single sync peer
that it downloads all blocks from until it is up to date with the longest chain
the sync peer is aware of.
*/
package netsync
