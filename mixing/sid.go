// Copyright (c) 2023-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixing

import (
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/crypto/blake256"
)

// DeriveSessionID creates the mix session identifier from an initial sorted
// slice of PR message hashes.
func DeriveSessionID(seenPRs []chainhash.Hash) [32]byte {
	h := blake256.New()
	h.Write([]byte("decred-mix-session"))
	for i := range seenPRs {
		h.Write(seenPRs[i][:])
	}
	return *(*[32]byte)(h.Sum(nil))
}
