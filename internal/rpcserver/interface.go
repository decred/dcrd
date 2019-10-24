// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcserver

import (
	"github.com/decred/dcrd/peer/v2"
)

// Peer represents a peer for use with the RPC server.
//
// The interface contract requires that all of these methods are safe for
// concurrent access.
type Peer interface {
	// ToPeer returns the underlying peer instance.
	ToPeer() *peer.Peer

	// IsTxRelayDisabled returns whether or not the peer has disabled
	// transaction relay.
	IsTxRelayDisabled() bool

	// BanScore returns the current integer value that represents how close
	// the peer is to being banned.
	BanScore() uint32
}
