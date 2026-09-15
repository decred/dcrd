// Copyright (c) 2023-2026 The Decred developers
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
// session, and include expiry information to increase resilience to replay and
// denial-of-service attacks.
//
// All mixing messages satisify this interface, however, the pair request
// message returns nil for some fields that do not apply, as it is the first
// message in the protocol.
type Message interface {
	wire.Message

	// Pub returns the message sender's public key identity.
	Pub() []byte
	// Sig returns the message signature.
	Sig() []byte
	WriteHash(hash.Hash)
	Hash() chainhash.Hash
	WriteSignedData(hash.Hash)
	// PrevMsgs returns messages from the previous stage of the mixing session
	// seen by the peer. For example, PrevMsgs of a KE message will return PR
	// messages. PR and FP messages return nil.
	PrevMsgs() []chainhash.Hash
	// Sid returns the session ID. PR messages return nil.
	Sid() []byte
	// GetRun returns the run number. PR messages return 0.
	GetRun() uint32
}
