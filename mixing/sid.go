// Copyright (c) 2023-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixing

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/wire"
)

// deriveSessionID creates the mix session identifier from an initial sorted
// slice of PR message hashes.
func deriveSessionID(seenPRs []chainhash.Hash, epoch uint64) [32]byte {
	h := blake256.New()
	buf := make([]byte, 8)

	h.Write([]byte("decred-mix-session"))

	binary.BigEndian.PutUint64(buf, epoch)
	h.Write(buf)

	for i := range seenPRs {
		h.Write(seenPRs[i][:])
	}

	return *(*[32]byte)(h.Sum(nil))
}

// SortPRsForSession performs an in-place sort of prs, moving each pair
// request to its original unmixed position in the protocol.  Returns the
// session ID.
func SortPRsForSession(prs []*wire.MsgMixPairReq, epoch uint64) [32]byte {
	// Lexicographical sort PRs to derive the sid.
	sort.Slice(prs, func(i, j int) bool {
		a := prs[i].Hash()
		b := prs[j].Hash()
		return bytes.Compare(a[:], b[:]) == -1
	})

	h := make([]chainhash.Hash, len(prs))
	for i, pr := range prs {
		h[i] = pr.Hash()
	}
	sid := deriveSessionID(h, epoch)

	// XOR the sid into each PR hash.
	for i := range h {
		xor(h[i][:], sid[:])
	}

	// Lexicographical sort PRs by their hash XOR'd with the sid.
	sort.Sort(&sortPRs{prs: prs, prXorSIDs: h})

	return sid
}

func xor(a, b []byte) {
	for i := range a {
		a[i] ^= b[i]
	}
}

type sortPRs struct {
	prs       []*wire.MsgMixPairReq
	prXorSIDs []chainhash.Hash
}

func (s *sortPRs) Len() int {
	return len(s.prs)
}

func (s *sortPRs) Less(i, j int) bool {
	a := s.prXorSIDs[i][:]
	b := s.prXorSIDs[j][:]
	return bytes.Compare(a, b) == -1
}

func (s *sortPRs) Swap(i, j int) {
	s.prs[i], s.prs[j] = s.prs[j], s.prs[i]
	s.prXorSIDs[i], s.prXorSIDs[j] = s.prXorSIDs[j], s.prXorSIDs[i]
}

// ValidateSession checks whether the original unmixed peer order of a key
// exchange's pair request hashes is validly sorted for the session ID, and
// for a run-0 KE, also checks that the session hash is derived from the
// specified pair requests and epoch.
func ValidateSession(ke *wire.MsgMixKeyExchange) error {
	h := make([]chainhash.Hash, len(ke.SeenPRs))
	copy(h, ke.SeenPRs)

	// XOR the sid into each hash.  The result should be sorted in all
	// runs.
	for i := range h {
		xor(h[i][:], ke.SessionID[:])
	}
	sorted := sort.SliceIsSorted(h, func(i, j int) bool {
		return bytes.Compare(h[i][:], h[j][:]) == -1
	})
	if !sorted {
		return errInvalidPROrder
	}

	// If this is a run-0 KE, validate the session hash.
	if ke.Run == 0 {
		copy(h, ke.SeenPRs)
		sort.Slice(h, func(i, j int) bool {
			return bytes.Compare(h[i][:], h[j][:]) == -1
		})
		derivedSID := deriveSessionID(h, ke.Epoch)
		if derivedSID != ke.SessionID {
			return errInvalidSessionID
		}
	}

	return nil
}
