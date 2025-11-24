// Copyright (c) 2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixpool

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/mixing"
	"github.com/decred/dcrd/wire"
)

const strikeLimit = 2

type strikeSet struct {
	set     map[wire.OutPoint]struct{}
	strikes []uint64
}

// sortUniq sorts and removes duplicates of all epoch strike times.
// Strikes must have non-zero length, otherwise this panics.
// For merging strike sets, this is safe as strike sets are always created
// with at least one epoch, and epochs are never removed from a strike set.
func sortUniq(strikes []uint64) []uint64 {
	sort.Slice(strikes, func(i, j int) bool {
		return strikes[i] < strikes[j]
	})
	n := len(strikes)
	strikes = strikes[:1]
	last := strikes[0]
	for _, epoch := range strikes[1:n] {
		if epoch != last {
			strikes = append(strikes, epoch)
			last = epoch
		}
	}
	return strikes
}

func (s *strikeSet) merge(other *strikeSet) {
	if s == other {
		return
	}

	for op := range other.set {
		s.set[op] = struct{}{}
	}

	s.strikes = sortUniq(append(s.strikes, other.strikes...))
}

func mergeStrikeSets(ss map[*strikeSet]struct{}) *strikeSet {
	var s *strikeSet
	for other := range ss {
		if s == nil {
			s = other
			continue
		}
		s.merge(other)
	}
	return s
}

// Observer tracks outpoints that were not included in successful mixes.  This
// provides mempool and voting policy the context necessary to discourage
// denial-of-service where misbehaving mixing peers churn and resubmit
// disruptive pair requests.
type Observer struct {
	mixpool *Pool
	epoch   time.Duration
	strikes map[wire.OutPoint]*strikeSet
	mu      sync.RWMutex
}

// Observer returns the mixpool misbehavior observer.  It must be ran with Run
// to check for misbehavior in previous epochs in the background.
func (p *Pool) Observer() *Observer {
	return p.observer
}

// waitForEpoch blocks until the next epoch, or errors when the context is
// canceled early.  Returns the calculated epoch.
func (o *Observer) waitForEpoch(ctx context.Context) (uint64, error) {
	now := time.Now().UTC()
	epoch := now.Truncate(o.epoch).Add(o.epoch)
	epochUnix := uint64(epoch.Unix())
	duration := epoch.Sub(now)
	timer := time.NewTimer(duration)
	select {
	case <-ctx.Done():
		if !timer.Stop() {
			<-timer.C
		}
		return epochUnix, ctx.Err()
	case <-timer.C:
		return epochUnix, nil
	}
}

// Run waits for every epoch to complete before checking for misbehavior in
// the previous epoch.
func (o *Observer) Run(ctx context.Context) error {
	// A pre-canceled context is used to receive mixpool messages without
	// waiting for a particular count of messages.
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	// Track the previous epoch (as Unix time).  Sessions are particular
	// to the epoch they were formed under, and only messages from the
	// previous finished epoch are considered by the observer.
	var prevEpoch uint64

	for {
		epoch, err := o.waitForEpoch(ctx)
		if err != nil {
			return err
		}
		if prevEpoch == 0 {
			prevEpoch = epoch
			continue
		}

		err = o.checkPrevEpoch(canceledCtx, prevEpoch)
		if err != nil {
			return err
		}

		prevEpoch = epoch
	}
}

// CheckPrevEpoch checks for timeout misbehavior in the previous epoch.
func (o *Observer) CheckPrevEpoch(prevEpoch uint64) error {
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	return o.checkPrevEpoch(canceledCtx, prevEpoch)
}

func (o *Observer) checkPrevEpoch(cancelledCtx context.Context, prevEpoch uint64) error {
	// Gather all attempted session formations, and those sessions
	// which ended in a pairings mix (number of received
	// confirmation messages equals the number of session
	// participants).
	//
	// The pairings variable maps all pairing IDs -> session hash -> KEs;
	// completed maps only completed session IDs -> KEs.
	pairings := make(map[string]map[chainhash.Hash][]*wire.MsgMixKeyExchange)
	completed := make(map[chainhash.Hash][]*wire.MsgMixKeyExchange)
	prByKE := make(map[chainhash.Hash]*wire.MsgMixPairReq)
	timedOut := make(map[string]map[idPubKey]struct{})
	active := o.mixpool.activeInEpoch(prevEpoch)
	for _, a := range active {
		pairing, err := a.pr.Pairing()
		if err != nil {
			return err
		}
		ses := pairings[string(pairing)]
		if ses == nil {
			ses = make(map[chainhash.Hash][]*wire.MsgMixKeyExchange)
			pairings[string(pairing)] = ses
		}
		for _, ke := range a.kes {
			ses[ke.SessionID] = append(ses[ke.SessionID], ke)
			prByKE[ke.Hash()] = a.pr
		}
	}
	r := &Received{
		ReceiveAll: true,
	}
	for _, ses := range pairings {
		for sid, sesKEs := range ses {
			// Sessions formed with fewer than the
			// required minimum peer count can't be used
			// to discover misbehavior.
			if len(sesKEs) < mixing.MinPeers {
				continue
			}

			r.Sid = sid
			// Capacity must be > 0 in order to receive messages.
			// Capacity does not limit the number of messages initially
			// read with a cancelled context.
			// Receiving into multiple result slices is allowed with
			// ReceiveAll = true.
			if cap(r.CTs) == 0 {
				r.CTs = make([]*wire.MsgMixCiphertexts, 0, len(sesKEs))
				r.SRs = make([]*wire.MsgMixSlotReserve, 0, len(sesKEs))
				r.DCs = make([]*wire.MsgMixDCNet, 0, len(sesKEs))
				r.CMs = make([]*wire.MsgMixConfirm, 0, len(sesKEs))
				r.RSs = make([]*wire.MsgMixSecrets, 0, len(sesKEs))
			} else {
				r.CTs = r.CTs[:0]
				r.SRs = r.SRs[:0]
				r.DCs = r.DCs[:0]
				r.CMs = r.CMs[:0]
				r.RSs = r.RSs[:0]
			}
			_ = o.mixpool.Receive(cancelledCtx, r)

			// When no ciphertext messages were received, a
			// session was not formed, and timeout can not be
			// observed.
			if len(r.CTs) == 0 {
				continue
			}

			// If secrets were revealed, then clients would have
			// blamed peers for non-timeout misbehavior, which is
			// out of scope for this observer.
			if len(r.RSs) > 0 {
				continue
			}

			pairing, err := prByKE[sesKEs[0].Hash()].Pairing()
			if err != nil {
				return err
			}
			if len(r.CMs) == len(sesKEs) {
				completed[sid] = sesKEs
				continue
			}

			// If a session was fully formed (all KEs received by each peer),
			// but later messages in the protocol were never received, peers
			// may have intentionally timed out.  Don't blame peers if all
			// messages are missing, as there is no evidence that only a
			// subset of the peers timed out.
			if len(sesKEs[0].SeenPRs) != len(sesKEs) {
				continue
			}
			ids := make(map[idPubKey]struct{})
			for _, ke := range sesKEs {
				ids[ke.Identity] = struct{}{}
			}
			switch {
			case len(r.CTs) == 0:
				continue
			case len(r.CTs) < len(sesKEs):
				for _, ct := range r.CTs {
					delete(ids, ct.Identity)
				}
			case len(r.SRs) == 0:
				continue
			case len(r.SRs) < len(sesKEs):
				for _, sr := range r.SRs {
					delete(ids, sr.Identity)
				}
			case len(r.DCs) == 0:
				continue
			case len(r.DCs) < len(sesKEs):
				for _, dc := range r.DCs {
					delete(ids, dc.Identity)
				}
			case len(r.CMs) == 0:
				continue
			case len(r.CMs) < len(sesKEs):
				for _, cm := range r.CMs {
					delete(ids, cm.Identity)
				}
			}
			if _, ok := timedOut[string(pairing)]; !ok {
				timedOut[string(pairing)] = make(map[idPubKey]struct{})
			}
			for id := range ids {
				timedOut[string(pairing)][id] = struct{}{}
			}
		}
	}

	// Modify the active map by removing identities that were
	// included in a completed mix.  Those remaining who sent key
	// exchange messages but who (for any reason) were not
	// included in a completed mix are assumed to be misbehaving
	// and trying to disrupt mixing, and restrictions on their
	// submitted UTXOs will be put in place after too many
	// violations.
	// This loop also records the completed pairings for all
	// completed sessions.
	completedPairings := make(map[string]struct{})
	for _, kes := range completed {
		for _, ke := range kes {
			delete(active, ke.Identity)
		}
		pairing, err := prByKE[kes[0].Hash()].Pairing()
		if err != nil {
			return err
		}
		completedPairings[string(pairing)] = struct{}{}
	}

	// Modify the active map by removing identities when no
	// successful mix occurred for the pairing.  If any peers
	// timed out for the pairing, do not exclude them from the
	// misbehaving peer set.
	for id, ap := range active {
		// Active peers will always have at least one KE, and
		// all KEs must be for the same pairing type.
		pairing, err := prByKE[ap.kes[0].Hash()].Pairing()
		if err != nil {
			return err
		}
		if _, ok := completedPairings[string(pairing)]; !ok {
			if timedOutIDs, ok := timedOut[string(pairing)]; ok {
				if _, ok := timedOutIDs[id]; ok {
					continue
				}
			}
			delete(active, id)
		}
	}

	o.updateStrikes(prevEpoch, active, prByKE, completed)

	return nil
}

func (o *Observer) updateStrikes(epoch uint64, misbehaving map[idPubKey]activePeer,
	prByKE map[chainhash.Hash]*wire.MsgMixPairReq,
	completed map[chainhash.Hash][]*wire.MsgMixKeyExchange) {

	o.mu.Lock()
	defer o.mu.Unlock()

	// Add a strike for any active identity that was not included in a
	// completed mix last epoch.
	//
	// Strikes are increased for all UTXOs associated with the misbehaving
	// identity.  If a new pair request is created that references a
	// different set of UTXOs, but with at least one shared UTXO, common
	// ownership can be established and all UTXOs from each PR are given
	// new strikes.
	for _, ap := range misbehaving {
		pairing, err := ap.pr.Pairing()
		if err != nil {
			log.Errorf("Pairing marshaling failed: %v", err)
			continue
		}
		log.Debugf("Pair request %v for pairing %x by mixing identity %x "+
			"flagged for misbehavior", ap.pr.Hash(), pairing,
			ap.pr.Identity[:])

		ss := make(map[*strikeSet]struct{})
		for i := range ap.pr.UTXOs {
			outpoint := &ap.pr.UTXOs[i].OutPoint
			s, ok := o.strikes[*outpoint]
			if !ok {
				continue
			}
			ss[s] = struct{}{}
		}
		s := mergeStrikeSets(ss)
		if s == nil {
			s = &strikeSet{
				set: make(map[wire.OutPoint]struct{}),
			}
		}
		s.strikes = append(s.strikes, epoch)
		for i := range ap.pr.UTXOs {
			outpoint := &ap.pr.UTXOs[i].OutPoint
			o.strikes[*outpoint] = s
		}
	}

	// Remove strikes for UTXOs spent by completed mixes.
	for _, kes := range completed {
		for _, ke := range kes {
			pr := prByKE[ke.Hash()]
			for i := range pr.UTXOs {
				outpoint := &pr.UTXOs[i].OutPoint
				delete(o.strikes, *outpoint)
			}
		}
	}

	// Remove strikes if none occurred in the past 24h.
	cutoff := epoch - (60 * 60 * 24)
	for op, s := range o.strikes {
		if s.strikes[len(s.strikes)-1] <= cutoff {
			delete(o.strikes, op)
		}
	}
}

// mergeAcceptedPRStrikes merges observed common UTXO ownership of
// newly-accepted mixpool PRs with current strikes recorded for an overlapping
// set of UTXOs, without adding any new strikes.
func (o *Observer) mergeAcceptedPRStrikes(pr *wire.MsgMixPairReq) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for i := range pr.UTXOs {
		s, ok := o.strikes[pr.UTXOs[i].OutPoint]
		if !ok {
			continue
		}
		for j := range pr.UTXOs {
			s.set[pr.UTXOs[j].OutPoint] = struct{}{}
		}
		return
	}
}

func (o *Observer) removeStrikesForMix(tx *wire.MsgTx) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, in := range tx.TxIn {
		delete(o.strikes, in.PreviousOutPoint)
	}
}

// MisbehavingBlock returns whether any transaction in the block spends an
// output that was flagged as submitted by a misbehaving mixing peer.
func (o *Observer) MisbehavingBlock(block *wire.MsgBlock) bool {
	// Lock order: mixpool mutex must be acquired before observer mutex.
	o.mixpool.mtx.RLock()
	defer o.mixpool.mtx.RUnlock()

	o.mu.RLock()
	defer o.mu.RUnlock()

	for _, tx := range block.Transactions {
		if o.misbehavingTx(tx, block) {
			return true
		}
	}
	for _, tx := range block.STransactions {
		if o.misbehavingTx(tx, block) {
			return true
		}
	}
	return false
}

// MisbehavingTx returns whether any transaction output was flagged as
// submitted by a misbehaving mixing peer.
func (o *Observer) MisbehavingTx(tx *wire.MsgTx) bool {
	// Lock order: mixpool mutex must be acquired before observer mutex.
	o.mixpool.mtx.RLock()
	defer o.mixpool.mtx.RUnlock()

	o.mu.RLock()
	defer o.mu.RUnlock()

	return o.misbehavingTx(tx, nil)
}

func (o *Observer) misbehavingTx(tx *wire.MsgTx, block *wire.MsgBlock) bool {
	txHash := tx.TxHash()
	_, ok := o.mixpool.sessionsByTxHash[txHash]
	if ok {
		return false
	}

	for _, in := range tx.TxIn {
		s, ok := o.strikes[in.PreviousOutPoint]
		if !ok {
			continue
		}
		if len(s.strikes) >= strikeLimit {
			if block == nil {
				log.Debugf("Transaction %v spends misbehaving mixing input %v",
					txHash, in.PreviousOutPoint)
			} else {
				log.Debugf("Transaction %v in block %v spends misbehaving mixing input %v",
					txHash, block.Header.BlockHash(), in.PreviousOutPoint)
			}
			return true
		}
	}
	return false
}

// ExcludePRs returns a slice of pair request messages excluding any which
// spend previously-flagged misbehaving outputs.
func (o *Observer) ExcludePRs(prs []*wire.MsgMixPairReq) []*wire.MsgMixPairReq {
	o.mu.RLock()
	defer o.mu.RUnlock()

	l := len(prs)
	prs = prs[:0]
PRs:
	for _, pr := range prs[:l] {
		for i := range pr.UTXOs {
			op := &pr.UTXOs[i].OutPoint
			s, ok := o.strikes[*op]
			if !ok {
				continue
			}
			if len(s.strikes) >= strikeLimit {
				log.Debugf("Excluding PR %v by %x: output %v "+
					"flagged for misbehavior %v times",
					pr.Hash(), pr.Identity[:], op, len(s.strikes))
				continue PRs
			}
		}
		prs = append(prs, pr)
	}
	return prs
}
