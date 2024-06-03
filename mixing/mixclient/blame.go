// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/mixing"
	"github.com/decred/dcrd/mixing/internal/chacha20prng"
	"github.com/decred/dcrd/mixing/mixpool"
	"github.com/decred/dcrd/wire"
)

// blamedIdentities identifies detected misbehaving peers.
//
// If a run returns a blamedIdentities error, these peers are immediately
// excluded and the next run is started.  This can only be done in situations
// where all peers observe the misbehavior as the run is performed.
//
// If a run errors but blame requires revealing secrets and blame assignment,
// a blamedIdentities error will be returned by the blame function.
type blamedIdentities []identity

func (e blamedIdentities) Error() string {
	return "blamed peers " + e.String()
}

func (e blamedIdentities) String() string {
	buf := new(bytes.Buffer)
	buf.WriteByte('[')
	for i, id := range e {
		if i != 0 {
			buf.WriteByte(' ')
		}
		fmt.Fprintf(buf, "%x", id[:])
	}
	buf.WriteByte(']')
	return buf.String()
}

func (c *Client) blame(ctx context.Context, sesRun *sessionRun) (err error) {
	c.logf("Blaming for sid=%x", sesRun.sid[:])

	mp := c.mixpool
	prs := sesRun.prs

	identityIndices := make(map[identity]int)
	for i, pr := range prs {
		identityIndices[pr.Identity] = i
	}

	var blamed blamedIdentities
	defer func() {
		if len(blamed) > 0 {
			c.log(blamed)
		}
	}()

	err = c.forLocalPeers(ctx, sesRun, func(p *peer) error {
		// Send initial secrets messages from any peers who detected
		// misbehavior.
		if !p.triggeredBlame {
			return nil
		}
		return p.signAndSubmit(p.rs)
	})
	if err != nil {
		return err
	}

	// Receive currently-revealed secrets
	rcv := new(mixpool.Received)
	rcv.Sid = sesRun.sid
	rcv.RSs = make([]*wire.MsgMixSecrets, 0, len(sesRun.prs))
	_ = mp.Receive(ctx, 0, rcv)
	rsHashes := make([]chainhash.Hash, len(rcv.RSs))
	for _, rs := range rcv.RSs {
		rsHashes = append(rsHashes, rs.Hash())
	}

	// Send remaining secrets messages.
	err = c.forLocalPeers(ctx, sesRun, func(p *peer) error {
		if p.triggeredBlame {
			p.triggeredBlame = false
			return nil
		}
		p.rs.SeenSecrets = rsHashes
		return p.signAndSubmit(p.rs)
	})
	if err != nil {
		return err
	}

	// Wait for all secrets, or timeout.
	rcv.RSs = rcv.RSs[:0]
	_ = mp.Receive(ctx, len(sesRun.prs), rcv)
	rss := rcv.RSs
	for _, rs := range rcv.RSs {
		if idx, ok := identityIndices[rs.Identity]; ok {
			sesRun.peers[idx].rs = rs
		}
	}
	if len(rss) != len(sesRun.peers) {
		// Blame peers who did not send secrets
		c.logf("received %d RSs for %d peers; blaming unresponsive peers",
			len(rss), len(sesRun.peers))

		for _, p := range sesRun.peers {
			if p.rs != nil {
				continue
			}
			c.logf("blaming %x for RS timeout", p.id[:])
			blamed = append(blamed, *p.id)
		}
		return blamed
	}
	sort.Slice(rss, func(i, j int) bool {
		a := identityIndices[rss[i].Identity]
		b := identityIndices[rss[j].Identity]
		return a < b
	})

	// If blame cannot be assigned on a failed mix, blame the peers who
	// reported failure.
	defer func() {
		if err != nil {
			return
		}
		for _, rs := range rss {
			if len(rs.PrevMsgs()) != 0 {
				continue
			}
			id := &rs.Identity
			c.logf("blaming %x for false failure accusation", id[:])
			blamed = append(blamed, *id)
		}
		err = blamed
	}()

	defer c.mu.Unlock()
	c.mu.Lock()

	var start uint32
	starts := make([]uint32, 0, len(prs))
	ecdh := make([]*secp256k1.PublicKey, 0, len(prs))
	pqpk := make([]*mixing.PQPublicKey, 0, len(prs))
	scratch := new(big.Int)
KELoop:
	for _, p := range sesRun.peers {
		if p.ke == nil {
			c.logf("blaming %x for missing messages", p.id[:])
			blamed = append(blamed, *p.id)
			continue
		}

		// Blame when revealed secrets do not match prior commitment to the secrets.
		c.blake256HasherMu.Lock()
		cm := p.rs.Commitment(c.blake256Hasher)
		c.blake256HasherMu.Unlock()
		if cm != p.ke.Commitment {
			c.logf("blaming %x for false commitment, got %x want %x",
				p.id[:], cm[:], p.ke.Commitment[:])
			blamed = append(blamed, *p.id)
			continue
		}

		// Blame peers whose seed is not the correct length (will panic chacha20prng).
		if len(p.rs.Seed) != chacha20prng.SeedSize {
			c.logf("blaming %x for bad seed size in RS message", p.id[:])
			blamed = append(blamed, *p.id)
			continue
		}

		// Blame peers with SR messages outside of the field.
		for _, m := range p.rs.SlotReserveMsgs {
			if mixing.InField(scratch.SetBytes(m)) {
				continue
			}
			c.logf("blaming %x for SR message outside field", p.id[:])
			blamed = append(blamed, *p.id)
			continue KELoop
		}

		// Recover or initialize PRNG from seed and the last run that
		// caused secrets to be generated.
		p.prng = chacha20prng.New(p.rs.Seed[:], 0)

		// Recover derived key exchange from PRNG.
		p.kx, err = mixing.NewKX(p.prng)
		if err != nil {
			c.logf("blaming %x for bad KX", p.id[:])
			blamed = append(blamed, *p.id)
			continue
		}

		// Blame when published ECDH or PQ public keys differ from
		// those recovered from the PRNG.
		switch {
		case !bytes.Equal(p.ke.ECDH[:], p.kx.ECDHPublicKey.SerializeCompressed()):
			fallthrough
		case !bytes.Equal(p.ke.PQPK[:], p.kx.PQPublicKey[:]):
			c.logf("blaming %x for KE public keys not derived from their PRNG",
				p.id[:])
			blamed = append(blamed, *p.id)
			continue KELoop
		}
		publishedECDHPub, err := secp256k1.ParsePubKey(p.ke.ECDH[:])
		if err != nil {
			c.logf("blaming %x for unparsable pubkey")
			blamed = append(blamed, *p.id)
			continue
		}
		ecdh = append(ecdh, publishedECDHPub)
		pqpk = append(pqpk, &p.ke.PQPK)

		mcount := p.pr.MessageCount
		starts = append(starts, start)
		start += mcount

		if uint32(len(p.rs.SlotReserveMsgs)) != mcount || uint32(len(p.rs.DCNetMsgs)) != mcount {
			c.logf("blaming %x for bad message count", p.id[:])
			blamed = append(blamed, *p.id)
			continue
		}
		srMsgInts := make([]*big.Int, len(p.rs.SlotReserveMsgs))
		for i, m := range p.rs.SlotReserveMsgs {
			srMsgInts[i] = new(big.Int).SetBytes(m)
		}
		p.srMsg = srMsgInts
		p.dcMsg = p.rs.DCNetMsgs
	}
	if len(blamed) > 0 {
		return blamed
	}

	// Recreate shared keys and ciphertexts from each peer's PRNG.
	recoveredCTs := make([][]mixing.PQCiphertext, 0, len(sesRun.peers))
	for _, p := range sesRun.peers {
		pqct, err := p.kx.Encapsulate(p.prng, pqpk, int(p.myVk))
		if err != nil {
			blamed = append(blamed, *p.id)
			continue
		}
		recoveredCTs = append(recoveredCTs, pqct)
	}
	if len(blamed) > 0 {
		return blamed
	}
	// Blame peers whose published ciphertexts differ from those recovered
	// from their PRNG.
	for i, p := range sesRun.peers {
		if p.ct == nil {
			c.logf("blaming %x for missing messages", p.id[:])
			blamed = append(blamed, *p.id)
			continue
		}
		if len(recoveredCTs[i]) != len(p.ct.Ciphertexts) {
			c.logf("blaming %x for different ciphertexts count %d != %d",
				p.id[:], len(recoveredCTs[i]), len(p.ct.Ciphertexts))
			blamed = append(blamed, *p.id)
			continue
		}
		for j := range p.ct.Ciphertexts {
			if !bytes.Equal(p.ct.Ciphertexts[j][:], recoveredCTs[i][j][:]) {
				c.logf("blaming %x for different ciphertexts", p.id[:])
				blamed = append(blamed, *p.id)
				break
			}
		}
	}
	if len(blamed) > 0 {
		return blamed
	}

	// Blame peers who share SR messages.
	shared := make(map[string][]identity)
	for _, p := range sesRun.peers {
		for _, m := range p.srMsg {
			key := string(m.Bytes())
			shared[key] = append(shared[key], *p.id)
		}
	}
	for _, pids := range shared {
		if len(pids) > 1 {
			for i := range pids {
				c.logf("blaming %x for shared SR message", pids[i][:])
			}
			blamed = append(blamed, pids...)
		}
	}
	if len(blamed) > 0 {
		return blamed
	}

SRLoop:
	for i, p := range sesRun.peers {
		if p.sr == nil {
			c.logf("blaming %x for missing messages", p.id[:])
			blamed = append(blamed, *p.id)
			continue
		}

		// Recover shared secrets
		revealed := &mixing.RevealedKeys{
			ECDHPublicKeys: ecdh,
			Ciphertexts:    make([]mixing.PQCiphertext, 0, len(prs)),
			MyIndex:        p.myVk,
		}
		for _, ct := range recoveredCTs {
			revealed.Ciphertexts = append(revealed.Ciphertexts, ct[p.myVk])
		}
		sharedSecrets, err := p.kx.SharedSecrets(revealed,
			sesRun.sid[:], 0, sesRun.mcounts)
		var decapErr *mixing.DecapsulateError
		if errors.As(err, &decapErr) {
			submittingID := p.id
			c.logf("blaming %x for unrecoverable secrets", submittingID[:])
			blamed = append(blamed, *submittingID)
			continue
		}
		if err != nil {
			return err
		}
		p.srKP = sharedSecrets.SRSecrets
		p.dcKP = sharedSecrets.DCSecrets

		for j, m := range p.srMsg {
			// Recover SR pads and mix with committed messages
			pads := mixing.SRMixPads(p.srKP[j], starts[i]+uint32(j))
			srMix := mixing.SRMix(m, pads)

			// Blame when committed mix does not match provided.
			for k := range srMix {
				if srMix[k].Cmp(scratch.SetBytes(p.sr.DCMix[j][k])) != 0 {
					c.logf("blaming %x for bad SR mix", p.id[:])
					blamed = append(blamed, *p.id)
					continue SRLoop
				}
			}
		}
	}
	if len(blamed) > 0 {
		return blamed
	}

	// If no roots were solved, but blaming has made it this far,
	// something is quite wrong (or secrets messages were invalidly
	// published).
	if len(sesRun.roots) == 0 {
		c.logerrf("Blame failed: unknown cause of root solving error")
		return nil
	}

	rootSlots := make(map[string]uint32)
	for i, m := range sesRun.roots {
		rootSlots[string(m.Bytes())] = uint32(i)
	}
DCLoop:
	for i, p := range sesRun.peers {
		// This also covers the case of a peer publishing solutions
		// for failing to solve roots, where there is nothing invalid
		// about the roots.  They would not have submitted any DC
		// messages yet in such case.  However, the "blaming $identity
		// for false failure accusation" (handled by an earlier
		// deferred function) if no peers could be assigned blame is
		// not likely to be seen under this situation.
		if p.dc == nil {
			c.logf("blaming %x for missing messages", p.id[:])
			blamed = append(blamed, *p.id)
			continue
		}

		// With the slot reservation successful, no peers should have
		// notified failure to find their slots in the next (DC)
		// message, and there must be mcount DC-net vectors.
		mcount := p.pr.MessageCount
		if uint32(len(p.dc.DCNet)) != mcount {
			c.logf("blaming %x for missing DC mix vectors", p.id[:])
			blamed = append(blamed, *p.id)
			continue
		}

		for j, m := range p.dcMsg {
			srMsg := p.srMsg[j]
			slot, ok := rootSlots[string(srMsg.Bytes())]
			if !ok {
				// Should never get here after a valid SR mix
				return fmt.Errorf("blame failed: no slot for message %v", m)
			}

			// Recover DC pads and mix with committed messages
			pads := mixing.DCMixPads(p.dcKP[j], starts[i]+uint32(j))
			dcMix := mixing.DCMix(pads, m[:], slot)

			// Blame when committed mix does not match provided.
			for k := 0; k < len(dcMix); k++ {
				if !dcMix.Equals(mixing.Vec(p.dc.DCNet[j])) {
					c.logf("blaming %x for bad DC mix", p.id[:])
					blamed = append(blamed, *p.id)
					continue DCLoop
				}
			}
		}
	}
	if len(blamed) > 0 {
		return blamed
	}

	// Blame peers whose unmixed data became invalid since the initial pair
	// request.
	// if j, ok := c.mix.(Joiner); ok {
	// 	// Validation occurs in parallel as it may involve high latency.
	// 	var mu sync.Mutex // protect concurrent appends to blamed
	// 	var wg sync.WaitGroup
	// 	wg.Add(len(c.clients))
	// 	for i := range c.clients {
	// 		i := i
	// 		pr := c.clients[i].pr
	// 		go func() {
	// 			err := j.ValidateUnmixed(pr.Unmixed, pr.MessageCount)
	// 			if err != nil {
	// 				mu.Lock()
	// 				blamed = append(blamed, i)
	// 				mu.Unlock()
	// 			}
	// 			wg.Done()
	// 		}()
	// 	}
	// 	wg.Wait()
	// }
	// if len(blamed) > 0 {
	// 	return blamed
	// }

	return nil
}
