// Copyright (c) 2023-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixing

import (
	"hash"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// Message is a mixing message.  In addition to implementing wire encoding,
// these messages are signed by an ephemeral mixing participant identity,
// declare the previous messages that have been observed by a peer in a mixing
// session, and include expiry information to increase resilience to replay
// and denial-of-service attacks.
//
// All mixing messages satisify this interface, however, the pair request
// message returns nil for some fields that do not apply, as it is the first
// message in the protocol.
type Message interface {
	wire.Message

	Pub() []byte
	Sig() []byte
	WriteHash(hash.Hash)
	Hash() chainhash.Hash
	WriteSignedData(hash.Hash)
	PrevMsgs() []chainhash.Hash // PR, FP returns nil
	Sid() []byte                // PR returns nil
	GetRun() uint32             // PR returns 0
}
