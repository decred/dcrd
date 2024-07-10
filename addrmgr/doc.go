// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package addrmgr implements a concurrency-safe Decred address manager.

# Address Manager Overview

The Decred network relies on fully-validating nodes that relay transactions and
blocks to other nodes around the world.  The network must be dynamic because
nodes will connect and disconnect as they please.  Each node must manage a
source of IP addresses to connect to and share with other nodes.  The Decred
wire protocol provides the `getaddr` and `addr` messages, allowing peers to
request and share known addresses with each other.  Each node needs a way to
store those addresses and select peers from them.  However, it is important to
remember that remote peers cannot be trusted.  A remote peer might send invalid
addresses, or worse, only send addresses they control with malicious intent.

With that in mind, this package provides a concurrency-safe address manager for
caching and selecting peers in a non-deterministic manner.  The general idea is
that the caller adds addresses to the address manager and notifies it when
addresses are connected, known good, and attempted.  The caller also requests
addresses as it needs them.

The address manager internally segregates the addresses into groups and
non-deterministically selects groups in a cryptographically random manner.  This
reduces the chances of selecting multiple addresses from the same network, which
generally helps provide greater peer diversity.  More importantly, it
drastically reduces the chances of an attacker coercing your peer into
connecting only to nodes they control.

The address manager also understands routability, and tries hard to only return
routable addresses.  In addition, it uses the information provided by the caller
about connected, known good, and attempted addresses to periodically purge peers
which no longer appear to be good, as well as to bias the selection toward known
good peers.  The general idea is to make a best effort to only provide usable
addresses.
*/
package addrmgr
