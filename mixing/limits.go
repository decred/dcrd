// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixing

// The wire package's limits on mixing messages are excessively large.  In
// practice, much lower limits are defined below and used by mixpool and
// mixclient.  These limits are enforced by the mixing module to avoid a
// wire protocol bump.
//
// These limits are a tradeoff between mix quality, theoretical maximum
// messages sizes, and the size of the resulting mix transaction.  In the
// largest mix defined by these limits, 64 total peers may each spend five
// P2PKH outputs each while contributing 17 P2PKH outputs (16 mixed outputs
// plus one change output) with the estimated size of the resulting transaction
// equaling 92309 bytes, just under the 100k standard threshold.
//
// These constants may change at any point without a major API bump to the
// mixing module.
const (
	// MaxPeers is the maximum number of peers allowed together in a
	// single mix session.  This restricts the maximum dimensions of the
	// slot reservation and XOR DC-net matrices and the maximum number of
	// previous messages that may be referenced by mix messages.
	MaxPeers = 64

	// MaxMcount is the maximum number of mixed messages that any single
	// peer can contribute to the mix.
	MaxMcount = 16

	// MaxMtot is the maximum number of mixed messages in total that can
	// be created during a session (the sum of all mcounts).
	MaxMtot = 1024

	// MaxMixTxSerializeSize is the maximum size of a mix transaction.
	// This value should not exceed mempool standardness limits.
	MaxMixTxSerializeSize = 100000

	// MaxMixAmount is the maximum value of a mixed output.
	MaxMixAmount = 21000000_00000000 / MinPeers
)
