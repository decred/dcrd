// Copyright (c) 2023-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package mixpool provides an in-memory pool of mixing messages for full nodes
// that relay these messages and mixing wallets that send and receive them.
package mixpool

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/container/lru"
	"github.com/decred/dcrd/mixing"
	"github.com/decred/dcrd/mixing/utxoproof"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/txscript/v4/stdscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/slog"
)

const minconf = 1
const feeRate = 0.0001e8

type idPubKey = [33]byte

type msgtype int

// Message type constants, for quickly checking looked up entries by message
// hash match the expected type (without performing a type assertion).
// Excludes PR.
const (
	msgtypeKE msgtype = 1 + iota
	msgtypeCT
	msgtypeSR
	msgtypeDC
	msgtypeCM
	msgtypeFP
	msgtypeRS

	nmsgtypes = msgtypeRS
)

const (
	// These fields are used when caching recently removed mix messages.
	//
	// maxRecentlyRemovedMixMsgs specifies the maximum numbr to cache and is
	// set to target about 100 participants per denomination up to typical
	// ticket prices plus an additional 20%.
	//
	// maxRecentMixMsgsTTL is the time to keep items in the recently removed mix
	// messages cache before they are expired.
	//
	// These values result in about 676 KiB memory usage including overhead.
	maxRecentlyRemovedMixMsgs = 8500
	maxRecentMixMsgsTTL       = time.Minute
)

func (m msgtype) String() string {
	switch m {
	case msgtypeKE:
		return "KE"
	case msgtypeCT:
		return "CT"
	case msgtypeSR:
		return "SR"
	case msgtypeDC:
		return "DC"
	case msgtypeCM:
		return "CM"
	case msgtypeFP:
		return "FP"
	case msgtypeRS:
		return "RS"
	default:
		return "?"
	}
}

// entry describes non-PR messages accepted to the pool.
type entry struct {
	hash     chainhash.Hash
	sid      [32]byte
	recvTime time.Time
	msg      mixing.Message
	msgtype  msgtype
}

type orphan struct {
	message  mixing.Message
	accepted time.Time
}

type session struct {
	sid    [32]byte
	prs    []chainhash.Hash
	counts [nmsgtypes]uint32
	hashes map[chainhash.Hash]struct{}
	expiry uint32
	bc     broadcast
}

func (s *session) countFor(t msgtype) uint32 {
	return s.counts[t-1]
}

func (s *session) incrementCountFor(t msgtype) {
	s.counts[t-1]++
}

type broadcast struct {
	ch chan struct{}
	mu sync.Mutex
}

// wait returns the wait channel that is closed whenever a message is received
// for a session.  Waiters must acquire the pool lock before reading messages.
func (b *broadcast) wait() <-chan struct{} {
	b.mu.Lock()
	ch := b.ch
	b.mu.Unlock()

	return ch
}

func (b *broadcast) signal() {
	b.mu.Lock()
	close(b.ch)
	b.ch = make(chan struct{})
	b.mu.Unlock()
}

// Pool records in-memory mix messages that have been broadcast over the
// peer-to-peer network.
type Pool struct {
	mtx                sync.RWMutex
	prs                map[chainhash.Hash]*wire.MsgMixPairReq
	outPoints          map[wire.OutPoint]chainhash.Hash
	pool               map[chainhash.Hash]entry
	orphans            map[chainhash.Hash]*orphan
	orphansByID        map[idPubKey]map[chainhash.Hash]mixing.Message
	messagesByIdentity map[idPubKey][]chainhash.Hash
	latestKE           map[idPubKey]*wire.MsgMixKeyExchange
	sessions           map[[32]byte]*session
	sessionsByTxHash   map[chainhash.Hash]*session
	epoch              time.Duration
	expireHeight       uint32
	expireSem          chan struct{}

	// recentMixMsgs caches mix messages that have recently been removed from
	// the pool.  The cache handles automatic expiration and maximum entry
	// limiting.
	//
	// Maintaining a separate cache of recently removed mix messages increases
	// the probability they are available to serve regardless of whether or not
	// they are still in the pool when a request for the advertisement arrives.
	recentMixMsgs *lru.Map[chainhash.Hash, mixing.Message]

	// The following fields are used to periodically log the total number
	// evicted items from the cache of recently removed mix messages.  They are
	// protected by the pool mutex.
	//
	// totalRecentsEvicted is the total number of items that have been evicted
	// from the cache since the previous report.
	//
	// lastRecentsLogged is the last time the total number of items that have
	// been evicted from the cache was reported.
	totalRecentsEvicted uint64
	lastRecentsLogged   time.Time

	blockchain  BlockChain
	utxoFetcher UtxoFetcher
	feeRate     int64
	params      *chaincfg.Params
}

// UtxoEntry provides details regarding unspent transaction outputs.
type UtxoEntry interface {
	IsSpent() bool
	PkScript() []byte
	ScriptVersion() uint16
	BlockHeight() int64
	Amount() int64
}

// UtxoFetcher defines methods used to validate unspent transaction outputs in
// the pair request message.  It is optional, but should be implemented by full
// nodes that have this capability to detect and stop relay of spam and junk
// messages.
type UtxoFetcher interface {
	// FetchUtxoEntry defines the function to use to fetch unspent
	// transaction output information.
	FetchUtxoEntry(wire.OutPoint) (UtxoEntry, error)
}

// BlockChain queries the current status of the blockchain.  Its methods should
// be able to be implemented by both full nodes and SPV wallets.
type BlockChain interface {
	// ChainParams identifies which chain parameters the mixing pool is
	// associated with.
	ChainParams() *chaincfg.Params

	// CurrentTip returns the hash and height of the current tip block.
	CurrentTip() (chainhash.Hash, int64)
}

// newRecentMixMsgsCache returns a new LRU cache for tracking mix messages that
// have recently been removed from the pool.
func newRecentMixMsgsCache() *lru.Map[chainhash.Hash, mixing.Message] {
	return lru.NewMapWithDefaultTTL[chainhash.Hash, mixing.Message](
		maxRecentlyRemovedMixMsgs, maxRecentMixMsgsTTL)
}

// NewPool returns a new mixing pool that accepts and validates mixing messages
// required for distributed transaction mixing.
func NewPool(blockchain BlockChain) *Pool {
	pool := &Pool{
		prs:                make(map[chainhash.Hash]*wire.MsgMixPairReq),
		outPoints:          make(map[wire.OutPoint]chainhash.Hash),
		pool:               make(map[chainhash.Hash]entry),
		orphans:            make(map[chainhash.Hash]*orphan),
		orphansByID:        make(map[idPubKey]map[chainhash.Hash]mixing.Message),
		messagesByIdentity: make(map[idPubKey][]chainhash.Hash),
		latestKE:           make(map[idPubKey]*wire.MsgMixKeyExchange),
		sessions:           make(map[[32]byte]*session),
		sessionsByTxHash:   make(map[chainhash.Hash]*session),
		epoch:              10 * time.Minute, // XXX: mainnet epoch: add to chainparams
		expireHeight:       0,
		expireSem:          make(chan struct{}, 1),
		recentMixMsgs:      newRecentMixMsgsCache(),
		lastRecentsLogged:  time.Now(),
		blockchain:         blockchain,
		feeRate:            feeRate,
		params:             blockchain.ChainParams(),
	}
	// XXX: add epoch to chainparams
	if blockchain.ChainParams().Net == wire.TestNet3 {
		pool.epoch = 3 * time.Minute
	}

	if u, ok := blockchain.(UtxoFetcher); ok {
		pool.utxoFetcher = u
	}
	return pool
}

// Epoch returns the duration between mix epochs.
func (p *Pool) Epoch() time.Duration {
	return p.epoch
}

// Message searches the mixing pool for a message by its hash.
func (p *Pool) Message(query *chainhash.Hash) (mixing.Message, error) {
	p.mtx.RLock()
	pr := p.prs[*query]
	e, ok := p.pool[*query]
	p.mtx.RUnlock()
	if pr != nil {
		return pr, nil
	}
	if !ok || e.msg == nil {
		return nil, errMessageNotFound
	}
	return e.msg, nil
}

// HaveMessage checks whether the mixing pool contains a message by its hash.
func (p *Pool) HaveMessage(query *chainhash.Hash) bool {
	p.mtx.RLock()
	_, ok := p.pool[*query]
	if !ok {
		_, ok = p.prs[*query]
	}
	p.mtx.RUnlock()
	return ok
}

// RecentMessages attempts to find a message by its hash in both the mixing pool
// that contains accepted messages as well as the cache of recently removed
// messages.
func (p *Pool) RecentMessage(query *chainhash.Hash) (mixing.Message, bool) {
	defer p.mtx.RUnlock()
	p.mtx.RLock()

	if pr := p.prs[*query]; pr != nil {
		return pr, true
	}
	if e, ok := p.pool[*query]; ok && e.msg != nil {
		return e.msg, true
	}
	return p.recentMixMsgs.Get(*query)
}

// MixPRs returns all MixPR messages.
//
// Any expired PRs that are still internally tracked by the mixpool for
// ongoing sessions are excluded from the result set.
func (p *Pool) MixPRs() []*wire.MsgMixPairReq {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	p.removeConfirmedSessions()

	res := make([]*wire.MsgMixPairReq, 0, len(p.prs))
	for _, pr := range p.prs {
		// Exclude expired but not yet removed PRs.
		if pr.Expiry <= p.expireHeight {
			continue
		}

		res = append(res, pr)
	}
	return res
}

// CompatiblePRs returns all MixPR messages with pairing descriptions matching
// the parameter.  The order is unspecified.
func (p *Pool) CompatiblePRs(pairing []byte) []*wire.MsgMixPairReq {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	res := make([]*wire.MsgMixPairReq, 0, len(p.prs))
	for _, pr := range p.prs {
		prPairing, _ := pr.Pairing()
		if bytes.Equal(pairing, prPairing) {
			res = append(res, pr)
		}
	}

	// Sort by decreasing expiries and remove any PRs double spending an
	// output with an earlier expiry.
	sort.Slice(res, func(i, j int) bool {
		return res[i].Expiry >= res[j].Expiry
	})
	seen := make(map[wire.OutPoint]uint32)
	for i, pr := range res {
		for _, utxo := range pr.UTXOs {
			prevExpiry, ok := seen[utxo.OutPoint]
			if !ok {
				seen[utxo.OutPoint] = pr.Expiry
			} else if pr.Expiry < prevExpiry {
				res[i] = nil
			}
		}
	}
	filtered := res[:0]
	for i := range res {
		if res[i] != nil {
			filtered = append(filtered, res[i])
		}
	}

	// Sort again lexicographically by hash.
	sort.Slice(filtered, func(i, j int) bool {
		a := filtered[i].Hash()
		b := filtered[j].Hash()
		return bytes.Compare(a[:], b[:]) < 1
	})
	return filtered
}

// ExpireMessagesInBackground will, after the current epoch period ends,
// remove all pair requests that indicate an expiry at or before the height
// parameter and removes all messages that chain back to a removed pair
// request.
//
// If a previous call is still waiting in the background to remove messages,
// this method has no effect, and proper usage to avoid a mixpool memory leak
// requires it to be consistently called as more blocks are processed.
func (p *Pool) ExpireMessagesInBackground(height uint32) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if p.expireHeight == 0 {
		p.expireHeight = height
	}

	select {
	case p.expireSem <- struct{}{}:
		go p.expireMessages()
	default:
	}
}

// waitForExpiry blocks for at least one full epoch, waiting until two epoch
// ticks from now.
func (p *Pool) waitForExpiry() {
	now := time.Now().UTC()
	epoch := now.Truncate(p.epoch).Add(2 * p.epoch)
	duration := epoch.Sub(now)
	time.Sleep(duration)
}

func (p *Pool) expireMessages() {
	p.waitForExpiry()

	p.mtx.Lock()
	defer func() {
		<-p.expireSem
		p.mtx.Unlock()
	}()

	height := p.expireHeight
	p.expireHeight = 0

	p.expireMessagesNow(height)
}

// maybeLogRecentMixMsgsNumEvicted periodically logs the total number of evicted
// items from the recently removed mix messages cache.
//
// This function MUST be called with the pool mutex held (for writes).
func (p *Pool) maybeLogRecentMixMsgsNumEvicted() {
	totalEvicted := p.totalRecentsEvicted
	if totalEvicted == 0 {
		return
	}

	const logInterval = time.Hour
	sinceLastLogged := time.Since(p.lastRecentsLogged)
	if sinceLastLogged < logInterval {
		return
	}

	log.Debugf("Evicted %d recent %s in the last %v (%d remaining, %.2f%% "+
		"hit ratio)", totalEvicted,
		pickNoun(totalEvicted, "mix message", "mix messages"),
		sinceLastLogged.Truncate(time.Second), p.recentMixMsgs.Len(),
		p.recentMixMsgs.HitRatio())

	p.totalRecentsEvicted = 0
	p.lastRecentsLogged = time.Now()
}

// removeMessage removes the message associated with the passed hash (when it
// exists) from the pool and adds it to the cache of recently removed mix
// messages.
//
// This MUST be called with the pool mutex held (for writes).
func (p *Pool) removeMessage(hash chainhash.Hash) {
	e, ok := p.pool[hash]
	if !ok {
		return
	}
	delete(p.pool, hash)
	if e.msg == nil {
		return
	}

	// Track recently removed mix messsages for a period of time in order to
	// increase the probability they are still available to serve for a while
	// even though they are no longer in the mixpool.  This helps provide a
	// buffer against the inability to serve any advertisements that take place
	// just prior to removal of the message.
	numEvicted := p.recentMixMsgs.Put(hash, e.msg)
	p.totalRecentsEvicted += uint64(numEvicted)
	p.maybeLogRecentMixMsgsNumEvicted()
}

// ExpireMessages immediately expires all pair requests and sessions built
// from them that indicate expiry at or after a block height.
func (p *Pool) ExpireMessages(height uint32) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	p.expireMessagesNow(height)
	p.expireHeight = 0
}

func (p *Pool) expireMessagesNow(height uint32) {
	// Expire sessions and their messages
	for sid, ses := range p.sessions {
		if ses.expiry > height {
			continue
		}

		delete(p.sessions, sid)
		for hash := range ses.hashes {
			p.removeMessage(hash)
		}
	}

	// Expire PRs and remove identity tracking
	for _, pr := range p.prs {
		if pr.Expiry > height {
			continue
		}

		p.removePR(pr, "expired")
	}

	// Expire orphans with old receive times, and in the case of any
	// orphan KE, expire those with old epochs.
	for hash, o := range p.orphans {
		expire := time.Since(o.accepted) >= 20*time.Minute
		if !expire {
			if ke, ok := o.message.(*wire.MsgMixKeyExchange); ok {
				epoch := time.Unix(int64(ke.Epoch), 0)
				expire = time.Since(epoch) >= 20*time.Minute
			}
		}
		if expire {
			delete(p.orphans, hash)
			delete(p.orphansByID, *(*idPubKey)(o.message.Pub()))
		}
	}
}

// RemoveMessage removes a message that was rejected by the network.
func (p *Pool) RemoveMessage(msg mixing.Message) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	msgHash := msg.Hash()
	p.removeMessage(msgHash)
	if pr, ok := msg.(*wire.MsgMixPairReq); ok {
		p.removePR(pr, "rejected")
	}
	if ke, ok := msg.(*wire.MsgMixKeyExchange); ok {
		delete(p.latestKE, ke.Identity)
	}
}

// RemoveSession removes the PRs and all session messages involving them from
// a completed session.  PR messages of a successful session are also removed.
func (p *Pool) RemoveSession(sid [32]byte) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	p.removeSession(sid, nil, true)
}

func (p *Pool) removeSession(sid [32]byte, txHash *chainhash.Hash, success bool) {
	ses := p.sessions[sid]
	if ses == nil {
		return
	}

	// Delete PRs used to form final run
	var removePRs []chainhash.Hash
	if success {
		removePRs = ses.prs
	}

	if txHash != nil || success {
		if txHash == nil {
			// XXX: may be better to store this in the runstate as
			// a CM is received.
			for h := range ses.hashes {
				if e, ok := p.pool[h]; ok && e.msgtype == msgtypeCM {
					cm := e.msg.(*wire.MsgMixConfirm)
					hash := cm.Mix.TxHash()
					txHash = &hash
					break
				}
			}
		}
		if txHash != nil {
			delete(p.sessionsByTxHash, *txHash)
		}
	}

	delete(p.sessions, sid)
	for hash := range ses.hashes {
		e, ok := p.pool[hash]
		if ok {
			log.Debugf("Removing session %x %T %v by %x",
				sid[:], e.msg, hash, e.msg.Pub())
			p.removeMessage(hash)
		}
	}

	for _, prHash := range removePRs {
		p.removeMessage(prHash)
		if pr := p.prs[prHash]; pr != nil {
			p.removePR(pr, "mixed")
		}
	}
}

// RemoveConfirmedSessions removes all messages including pair requests from
// runs which ended in each peer sending a confirm mix message.
func (p *Pool) RemoveConfirmedSessions() {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	p.removeConfirmedSessions()
}

func (p *Pool) removeConfirmedSessions() {
	for sid, ses := range p.sessions {
		cmCount := ses.countFor(msgtypeCM)
		if uint32(len(ses.prs)) != cmCount {
			continue
		}

		delete(p.sessions, sid)
		for hash := range ses.hashes {
			p.removeMessage(hash)
		}

		for _, hash := range ses.prs {
			p.removeMessage(hash)
			pr := p.prs[hash]
			if pr != nil {
				p.removePR(pr, "confirmed")
			}
		}
	}
}

// RemoveConfirmedMixes removes sessions and messages belonging to a completed
// session that resulted in published or mined transactions.  Transaction
// hashes not associated with a session are ignored.  PRs from the successful
// mix session are removed from the pool.
func (p *Pool) RemoveConfirmedMixes(txHashes []chainhash.Hash) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for i := range txHashes {
		hash := &txHashes[i]
		ses := p.sessionsByTxHash[*hash]
		if ses == nil {
			continue
		}

		p.removeSession(ses.sid, hash, true)
	}
}

// RemoveSpentPRs removes all pair requests that are spent by any transaction
// input.
func (p *Pool) RemoveSpentPRs(txs []*wire.MsgTx) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for _, tx := range txs {
		txHash := tx.TxHash()
		ses, ok := p.sessionsByTxHash[txHash]
		if ok {
			p.removeSession(ses.sid, &txHash, true)
			continue
		}

		for _, in := range tx.TxIn {
			prHash := p.outPoints[in.PreviousOutPoint]
			pr, ok := p.prs[prHash]
			if ok {
				p.removePR(pr, "double spent")
			}
		}
	}
}

// ReceiveKEsByPairing returns the most recently received run-0 KE messages by
// a peer that reference PRs of a particular pairing and epoch.
func (p *Pool) ReceiveKEsByPairing(pairing []byte, epoch uint64) []*wire.MsgMixKeyExchange {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	var kes []*wire.MsgMixKeyExchange
	for id, ke := range p.latestKE {
		if ke.Epoch != epoch {
			continue
		}
		prHash := p.messagesByIdentity[id][0]
		pr := p.prs[prHash]
		prPairing, err := pr.Pairing()
		if err != nil {
			continue
		}
		if bytes.Equal(pairing, prPairing) {
			kes = append(kes, ke)
		}
	}
	return kes
}

// RemoveUnresponsiveDuringEpoch removes pair requests of unresponsive peers
// that did not provide any key exchange messages during the epoch in which a
// mix occurred.
func (p *Pool) RemoveUnresponsiveDuringEpoch(prs []*wire.MsgMixPairReq, epoch uint64) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

PRLoop:
	for _, pr := range prs {
		for _, msgHash := range p.messagesByIdentity[pr.Identity] {
			msg, ok := p.pool[msgHash].msg.(*wire.MsgMixKeyExchange)
			if !ok {
				continue
			}
			if msg.Epoch == epoch {
				continue PRLoop
			}
		}

		p.removePR(pr, "unresponsive")
	}
}

// Received is a parameter for Pool.Receive describing the session to receive
// messages for, and slices for returning results.  A single non-nil slice
// with capacity of the expected number of messages is required and indicates
// which message slice will be will be appended to.  Received messages are
// unsorted.
type Received struct {
	Sid [32]byte
	KEs []*wire.MsgMixKeyExchange
	CTs []*wire.MsgMixCiphertexts
	SRs []*wire.MsgMixSlotReserve
	DCs []*wire.MsgMixDCNet
	CMs []*wire.MsgMixConfirm
	FPs []*wire.MsgMixFactoredPoly
	RSs []*wire.MsgMixSecrets
}

// Receive returns messages matching a session and message type, waiting until
// all described messages have been received, or earlier with the messages
// received so far if the context is cancelled before this point.
//
// Receive only returns results for the session ID in the r parameter.  If no
// such session has any messages currently accepted in the mixpool, the method
// immediately errors.
//
// If any secrets messages are received for the described session, and r.RSs
// is nil, Receive immediately returns ErrSecretsRevealed.  An additional call
// to Receive with a non-nil RSs can be used to receive all of the secrets
// after each peer publishes their own revealed secrets.
func (p *Pool) Receive(ctx context.Context, r *Received) error {
	sid := r.Sid
	var bc *broadcast

	p.mtx.RLock()
	ses, ok := p.sessions[sid]
	if !ok {
		p.mtx.RUnlock()
		return fmt.Errorf("unknown session %x", sid[:])
	}
	bc = &ses.bc

	var capSlices, expectedMessages int
	if cap(r.KEs) != 0 {
		capSlices++
		expectedMessages = cap(r.KEs)
	}
	if cap(r.CTs) != 0 {
		capSlices++
		expectedMessages = cap(r.CTs)
	}
	if cap(r.SRs) != 0 {
		capSlices++
		expectedMessages = cap(r.SRs)
	}
	if cap(r.DCs) != 0 {
		capSlices++
		expectedMessages = cap(r.DCs)
	}
	if cap(r.CMs) != 0 {
		capSlices++
		expectedMessages = cap(r.CMs)
	}
	if cap(r.FPs) != 0 {
		capSlices++
		expectedMessages = cap(r.FPs)
	}
	if cap(r.RSs) != 0 {
		capSlices++
		expectedMessages = cap(r.RSs)
	}
	if capSlices != 1 {
		return fmt.Errorf("mixpool: exactly one Received slice must have non-zero capacity")
	}

Loop:
	for {
		// Pool is locked for reads.  Count if the total number of
		// expected messages have been received.
		received := 0
		for hash := range ses.hashes {
			msgtype := p.pool[hash].msgtype
			switch {
			case msgtype == msgtypeKE && r.KEs != nil:
				received++
			case msgtype == msgtypeCT && r.CTs != nil:
				received++
			case msgtype == msgtypeSR && r.SRs != nil:
				received++
			case msgtype == msgtypeDC && r.DCs != nil:
				received++
			case msgtype == msgtypeCM && r.CMs != nil:
				received++
			case msgtype == msgtypeFP && r.FPs != nil:
				received++
			case msgtype == msgtypeRS:
				if r.RSs == nil {
					// Since initial reporters of secrets
					// need to take the blame for
					// erroneous blame assignment if no
					// issue was detected, we only trigger
					// this for RS messages that do not
					// reference any other previous RS.
					rs := p.pool[hash].msg.(*wire.MsgMixSecrets)
					prev := rs.PrevMsgs()
					if len(prev) == 0 {
						p.mtx.RUnlock()
						return ErrSecretsRevealed
					}
				} else {
					received++
				}
			}
		}
		if received >= expectedMessages {
			break
		}

		// Unlock while waiting for the broadcast channel.
		p.mtx.RUnlock()

		select {
		case <-ctx.Done():
			p.mtx.RLock()
			break Loop
		case <-bc.wait():
		}

		p.mtx.RLock()
	}

	// Pool is locked for reads.  Collect all of the messages.
	for hash := range ses.hashes {
		msg := p.pool[hash].msg
		switch msg := msg.(type) {
		case *wire.MsgMixKeyExchange:
			if r.KEs != nil {
				r.KEs = append(r.KEs, msg)
			}
		case *wire.MsgMixCiphertexts:
			if r.CTs != nil {
				r.CTs = append(r.CTs, msg)
			}
		case *wire.MsgMixSlotReserve:
			if r.SRs != nil {
				r.SRs = append(r.SRs, msg)
			}
		case *wire.MsgMixDCNet:
			if r.DCs != nil {
				r.DCs = append(r.DCs, msg)
			}
		case *wire.MsgMixConfirm:
			if r.CMs != nil {
				r.CMs = append(r.CMs, msg)
			}
		case *wire.MsgMixFactoredPoly:
			if r.FPs != nil {
				r.FPs = append(r.FPs, msg)
			}
		case *wire.MsgMixSecrets:
			if r.RSs != nil {
				r.RSs = append(r.RSs, msg)
			}
		}
	}

	p.mtx.RUnlock()
	return nil
}

var zeroHash chainhash.Hash

// AcceptMessage accepts a mixing message to the pool.
//
// Messages must contain the mixing participant's identity and contain a valid
// signature committing to all non-signature fields.
//
// PR messages will not be accepted if they reference an unknown UTXO or if not
// enough fee is contributed.  Any other message will not be accepted if it
// references previous messages that are not recorded by the pool.
//
// All newly accepted messages, including any orphan key exchange messages
// that were processed after processing missing pair requests, are returned.
func (p *Pool) AcceptMessage(msg mixing.Message) (accepted []mixing.Message, err error) {
	defer func() {
		if err == nil && len(accepted) == 0 {
			// Don't log duplicate messages or non-KE orphans.
			return
		}
		if log.Level() > slog.LevelDebug {
			return
		}
		if err != nil {
			hash := msg.Hash()
			switch msg.(type) {
			case *wire.MsgMixPairReq:
				log.Debugf("Rejected message %T %v by %x: %v",
					msg, hash, msg.Pub(), err)
			default:
				log.Debugf("Rejected message %T %v (session %x) by %x: %v",
					msg, hash, msg.Sid(), msg.Pub(), err)
			}
			return
		}
		for _, msg := range accepted {
			hash := msg.Hash()
			switch msg.(type) {
			case *wire.MsgMixPairReq:
				log.Debugf("Accepted message %T %v by %x", msg, hash, msg.Pub())
			default:
				log.Debugf("Accepted message %T %v (session %x) by %x",
					msg, hash, msg.Sid(), msg.Pub())
			}
		}
	}()

	if msg.GetRun() != 0 {
		return nil, ruleError(fmt.Errorf("nonzero reruns are unsupported"))
	}

	hash := msg.Hash()
	if hash == zeroHash {
		return nil, fmt.Errorf("message of type %T has not been hashed", msg)
	}

	alreadyAccepted := func() bool {
		_, ok := p.pool[hash]
		if !ok {
			_, ok = p.prs[hash]
		}
		return ok
	}

	// Check if already accepted.
	p.mtx.RLock()
	ok := alreadyAccepted()
	p.mtx.RUnlock()
	if ok {
		return nil, nil
	}

	// Require message to be signed by the presented identity.
	if !mixing.VerifySignedMessage(msg) {
		return nil, ruleError(ErrInvalidSignature)
	}
	id := (*idPubKey)(msg.Pub())

	var msgtype msgtype
	switch msg := msg.(type) {
	case *wire.MsgMixPairReq:
		if err := p.checkAcceptPR(msg); err != nil {
			return nil, err
		}

		p.mtx.Lock()
		defer p.mtx.Unlock()

		accepted, err := p.acceptPR(msg, &hash, id)
		if err != nil {
			return nil, err
		}
		// Avoid returning a non-nil mixing.Message in return
		// variable with a nil PR.
		if accepted == nil {
			return nil, nil
		}

		allAccepted := p.reconsiderOrphans(msg, id)
		return allAccepted, nil

	case *wire.MsgMixKeyExchange:
		if err := p.checkAcceptKE(msg); err != nil {
			return nil, err
		}

		p.mtx.Lock()
		defer p.mtx.Unlock()

		accepted, err := p.acceptKE(msg, &hash, id)
		if err != nil {
			return nil, err
		}
		// Avoid returning a non-nil mixing.Message in return
		// variable with a nil KE.
		if accepted == nil {
			return nil, nil
		}
		allAccepted := p.reconsiderOrphans(msg, id)
		return allAccepted, nil

	case *wire.MsgMixCiphertexts:
		msgtype = msgtypeCT
	case *wire.MsgMixSlotReserve:
		msgtype = msgtypeSR
	case *wire.MsgMixDCNet:
		msgtype = msgtypeDC
	case *wire.MsgMixConfirm:
		msgtype = msgtypeCM
	case *wire.MsgMixFactoredPoly:
		msgtype = msgtypeFP
	case *wire.MsgMixSecrets:
		msgtype = msgtypeRS
	default:
		return nil, fmt.Errorf("unknown mix message type %T", msg)
	}

	if len(msg.Sid()) != 32 {
		return nil, ruleError(ErrInvalidSessionID)
	}
	sid := *(*[32]byte)(msg.Sid())

	p.mtx.Lock()
	defer p.mtx.Unlock()

	// Read lock was given up to acquire write lock.  Check if already
	// accepted.
	if alreadyAccepted() {
		return nil, nil
	}

	// Check that a message from this identity does not conflict with a
	// different message of the same type in the session.
	var haveKE bool
	for _, prevHash := range p.messagesByIdentity[*id] {
		e := p.pool[prevHash]
		if e.msgtype == msgtype && bytes.Equal(e.msg.Sid(), msg.Sid()) {
			return nil, ruleError(fmt.Errorf("message %v by identity %x "+
				"in session %x conflicts with already accepted message %v",
				hash, *id, msg.Sid(), prevHash))
		}
		if !haveKE && e.msgtype == msgtypeKE && bytes.Equal(e.msg.Sid(), msg.Sid()) {
			haveKE = true
		}
	}
	// Save as an orphan if their KE is not (yet) accepted.
	if !haveKE {
		orphansByID := p.orphansByID[*id]
		if _, ok := orphansByID[hash]; ok {
			// Already an orphan.
			return nil, nil
		}
		if orphansByID == nil {
			orphansByID = make(map[chainhash.Hash]mixing.Message)
			p.orphansByID[*id] = orphansByID
		}
		p.orphans[hash] = &orphan{
			message:  msg,
			accepted: time.Now(),
		}
		orphansByID[hash] = msg

		// TODO: Consider return an error containing the unknown
		// messages, so they can be getdata'd.
		return nil, nil
	}

	ses := p.sessions[sid]
	if ses == nil {
		return nil, ruleError(fmt.Errorf("%s %s belongs to unknown session %x",
			msgtype, &hash, sid))
	}

	p.acceptEntry(msg, msgtype, &hash, id, ses)
	return []mixing.Message{msg}, nil
}

// removePR removes a pair request message and all other messages and sessions
// that the peer sent and was involved in.
//
// This MUST be called with the pool mutex held (for writes).
func (p *Pool) removePR(pr *wire.MsgMixPairReq, reason string) {
	prHash := pr.Hash()

	log.Debugf("Removing %s PR %s by %x", reason, prHash, pr.Identity[:])

	delete(p.prs, prHash)

	// Track recently removed pair requests for a period of time in order to
	// increase the probability they are still available to serve for a while
	// even though they are no longer in the mixpool.  This helps provide a
	// buffer against the inability to serve any advertisements that take place
	// just prior to removal of the message.
	numEvicted := p.recentMixMsgs.Put(prHash, pr)
	p.totalRecentsEvicted += uint64(numEvicted)
	p.maybeLogRecentMixMsgsNumEvicted()

	for _, hash := range p.messagesByIdentity[pr.Identity] {
		e, ok := p.pool[hash]
		if !ok {
			continue
		}
		ke, ok := e.msg.(*wire.MsgMixKeyExchange)
		if ok {
			p.removeSession(ke.SessionID, nil, false)
		}
		p.removeMessage(hash)
	}
	delete(p.messagesByIdentity, pr.Identity)
	delete(p.latestKE, pr.Identity)
	for orphanHash := range p.orphansByID[pr.Identity] {
		delete(p.orphans, orphanHash)
	}
	delete(p.orphansByID, pr.Identity)
	for i := range pr.UTXOs {
		delete(p.outPoints, pr.UTXOs[i].OutPoint)
	}
}

func (p *Pool) checkAcceptPR(pr *wire.MsgMixPairReq) error {
	switch {
	case len(pr.UTXOs) == 0: // Require at least one utxo.
		return ruleError(ErrMissingUTXOs)
	case pr.MessageCount == 0: // Require at least one mixed message.
		return ruleError(ErrInvalidMessageCount)
	case pr.InputValue < int64(pr.MessageCount)*pr.MixAmount:
		return ruleError(ErrInvalidTotalMixAmount)
	case pr.Change != nil:
		if isDustAmount(pr.Change.Value, p2pkhv0PkScriptSize, feeRate) {
			return ruleError(ErrChangeDust)
		}
		if !stdscript.IsPubKeyHashScriptV0(pr.Change.PkScript) &&
			!stdscript.IsScriptHashScriptV0(pr.Change.PkScript) {
			return ruleError(ErrInvalidScript)
		}
	}

	// Check that expiry has not been reached, nor that it is too far
	// into the future.  This limits replay attacks.
	_, curHeight := p.blockchain.CurrentTip()
	maxExpiry := mixing.MaxExpiry(uint32(curHeight), p.params)
	switch {
	case uint32(curHeight) >= pr.Expiry:
		return ruleError(fmt.Errorf("message has expired"))
	case pr.Expiry > maxExpiry:
		return ruleError(fmt.Errorf("expiry is too far into future"))
	}

	// Require known script classes.
	switch mixing.ScriptClass(pr.ScriptClass) {
	case mixing.ScriptClassP2PKHv0:
	default:
		return ruleError(fmt.Errorf("unsupported mixing script class"))
	}

	// Require enough fee contributed from this mixing participant.
	// Size estimation assumes mixing.ScriptClassP2PKHv0 outputs and inputs.
	if err := checkFee(pr, p.feeRate); err != nil {
		return err
	}

	// If able, sanity check UTXOs.
	if p.utxoFetcher != nil {
		err := p.checkUTXOs(pr, curHeight)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *Pool) acceptPR(pr *wire.MsgMixPairReq, hash *chainhash.Hash, id *idPubKey) (accepted *wire.MsgMixPairReq, err error) {
	// Check if already accepted.
	if _, ok := p.prs[*hash]; ok {
		return nil, nil
	}

	// Discourage identity reuse.  PRs should be the first message sent by
	// this identity, and there should only be one PR per identity.
	if len(p.messagesByIdentity[*id]) != 0 {
		// XXX: Consider making this a bannable offense.  In the
		// future, it would be better to publish proof of identity
		// reuse when signing different messages.
		return nil, ruleError(fmt.Errorf("identity reused for a PR message"))
	}

	// Only accept PRs that double spend outpoints if they expire later
	// than existing PRs.  Otherwise, reject this PR message.
	for i := range pr.UTXOs {
		otherPRHash := p.outPoints[pr.UTXOs[i].OutPoint]
		otherPR, ok := p.prs[otherPRHash]
		if !ok {
			continue
		}
		if otherPR.Expiry >= pr.Expiry {
			err := ruleError(fmt.Errorf("PR double spends outpoints of " +
				"already-accepted PR message without " +
				"increasing expiry"))
			return nil, err
		}
	}

	// Accept the PR
	p.prs[*hash] = pr
	for i := range pr.UTXOs {
		p.outPoints[pr.UTXOs[i].OutPoint] = *hash
	}
	p.messagesByIdentity[*id] = append(make([]chainhash.Hash, 0, 16), *hash)

	return pr, nil
}

// reconsiderOrphans reconsiders any messages that are currently saved as
// orphans due to missing previous PR message (in the case of KE orphans) or
// missing the identity's KE in matching session (for all other messages).
// The function is recursive: if a reconsidered orphan KE is accepted, other
// orphans by the identity will be considered as well.
func (p *Pool) reconsiderOrphans(accepted mixing.Message, id *idPubKey) []mixing.Message {
	acceptedMessages := []mixing.Message{accepted}

	var kes []*wire.MsgMixKeyExchange
	if ke, ok := accepted.(*wire.MsgMixKeyExchange); ok {
		kes = append(kes, ke)
	}

	// If the accepted message was a PR, there may be KE orphans that can
	// be accepted now.
	if pr, ok := accepted.(*wire.MsgMixPairReq); ok {
		var orphanKEs []*wire.MsgMixKeyExchange
		for _, orphan := range p.orphansByID[*id] {
			orphanKE, ok := orphan.(*wire.MsgMixKeyExchange)
			if !ok {
				continue
			}
			refsAcceptedPR := false
			for _, prHash := range orphanKE.SeenPRs {
				if pr.Hash() == prHash {
					refsAcceptedPR = true
					break
				}
			}
			if !refsAcceptedPR {
				continue
			}

			orphanKEs = append(orphanKEs, orphanKE)
		}

		for _, orphanKE := range orphanKEs {
			orphanKEHash := orphanKE.Hash()
			_, err := p.acceptKE(orphanKE, &orphanKEHash, &orphanKE.Identity)
			if err != nil {
				log.Debugf("orphan KE could not be accepted: %v", err)
				continue
			}

			kes = append(kes, orphanKE)
			delete(p.orphansByID[*id], orphanKEHash)
			delete(p.orphans, orphanKEHash)

			acceptedMessages = append(acceptedMessages, orphanKE)
		}
		if len(p.orphansByID[*id]) == 0 {
			delete(p.orphansByID, *id)
			return acceptedMessages
		}
	}

	// For any KE that has been accepted following reconsideration after
	// accepting a PR, other orphan messages may be potentially accepted
	// as well.
	for _, ke := range kes {
		ses := p.sessions[ke.SessionID]
		if ses == nil {
			log.Errorf("No session %x exists for accepted KE %s",
				ke.SessionID[:], ke.Hash())
			continue
		}

		var acceptedOrphans []mixing.Message
		for orphanHash, orphan := range p.orphansByID[*id] {
			if !bytes.Equal(orphan.Sid(), ke.SessionID[:]) {
				continue
			}

			var msgtype msgtype
			switch orphan.(type) {
			case *wire.MsgMixCiphertexts:
				msgtype = msgtypeCT
			case *wire.MsgMixSlotReserve:
				msgtype = msgtypeSR
			case *wire.MsgMixDCNet:
				msgtype = msgtypeDC
			case *wire.MsgMixConfirm:
				msgtype = msgtypeCM
			case *wire.MsgMixFactoredPoly:
				msgtype = msgtypeFP
			case *wire.MsgMixSecrets:
				msgtype = msgtypeRS
			default:
				log.Errorf("Unknown orphan message %T %s", orphan, orphan.Hash())
				continue
			}

			p.acceptEntry(orphan, msgtype, &orphanHash, id, ses)

			acceptedOrphans = append(acceptedOrphans, orphan)
			acceptedMessages = append(acceptedMessages, orphan)
		}
		for _, orphan := range acceptedOrphans {
			orphanHash := orphan.Hash()
			delete(p.orphansByID[*id], orphanHash)
			delete(p.orphans, orphanHash)
		}
		if len(p.orphansByID[*id]) == 0 {
			delete(p.orphansByID, *id)
			return acceptedMessages
		}
	}

	return acceptedMessages
}

// Check that UTXOs exist, have confirmations, sum of UTXO values matches the
// input value, and proof of ownership is valid.
func (p *Pool) checkUTXOs(pr *wire.MsgMixPairReq, curHeight int64) error {
	var totalValue int64

	for i := range pr.UTXOs {
		utxo := &pr.UTXOs[i]
		entry, err := p.utxoFetcher.FetchUtxoEntry(utxo.OutPoint)
		if err != nil {
			return err
		}
		if entry == nil || entry.IsSpent() {
			return ruleError(fmt.Errorf("output %v is not unspent",
				&utxo.OutPoint))
		}
		height := entry.BlockHeight()
		if !confirmed(minconf, height, curHeight) {
			return ruleError(fmt.Errorf("output %v is unconfirmed",
				&utxo.OutPoint))
		}
		if entry.ScriptVersion() != 0 {
			return ruleError(fmt.Errorf("output %v does not use script version 0",
				&utxo.OutPoint))
		}

		// Check proof of key ownership and ability to sign coinjoin
		// inputs.
		var extractPubKeyHash160 func([]byte) []byte
		switch {
		case utxo.Opcode == 0:
			extractPubKeyHash160 = stdscript.ExtractPubKeyHashV0
		case utxo.Opcode == txscript.OP_SSGEN:
			extractPubKeyHash160 = stdscript.ExtractStakeGenPubKeyHashV0
		case utxo.Opcode == txscript.OP_SSRTX:
			extractPubKeyHash160 = stdscript.ExtractStakeRevocationPubKeyHashV0
		case utxo.Opcode == txscript.OP_TGEN:
			extractPubKeyHash160 = stdscript.ExtractTreasuryGenPubKeyHashV0
		default:
			return ruleError(fmt.Errorf("unsupported output script for UTXO %s", &utxo.OutPoint))
		}
		valid := validateOwnerProofP2PKHv0(extractPubKeyHash160,
			entry.PkScript(), utxo.PubKey, utxo.Signature, pr.Expires())
		if !valid {
			return ruleError(ErrInvalidUTXOProof)
		}

		totalValue += entry.Amount()
	}

	if totalValue != pr.InputValue {
		return ruleError(fmt.Errorf("input value does not match sum of UTXO " +
			"values"))
	}

	return nil
}

func validateOwnerProofP2PKHv0(extractFunc func([]byte) []byte, pkscript, pubkey, sig []byte, expires uint32) bool {
	extractedHash160 := extractFunc(pkscript)
	pubkeyHash160 := stdaddr.Hash160(pubkey)
	if !bytes.Equal(extractedHash160, pubkeyHash160) {
		return false
	}

	return utxoproof.ValidateSecp256k1P2PKH(pubkey, sig, expires)
}

func (p *Pool) checkAcceptKE(ke *wire.MsgMixKeyExchange) error {
	// Validate PR order and session ID.
	if err := mixing.ValidateSession(ke); err != nil {
		return ruleError(err)
	}

	if ke.Pos >= uint32(len(ke.SeenPRs)) {
		return ruleError(ErrPeerPositionOutOfBounds)
	}

	return nil
}

func (p *Pool) acceptKE(ke *wire.MsgMixKeyExchange, hash *chainhash.Hash, id *idPubKey) (accepted *wire.MsgMixKeyExchange, err error) {
	// Check if already accepted.
	if _, ok := p.pool[*hash]; ok {
		return nil, nil
	}

	// While KEs are allowed to reference unknown PRs, they must at least
	// reference the PR submitted by their own identity.  If not, the KE
	// is saved as an orphan and may be processed later.
	// Of all PRs that are known, their pairing types must be compatible.
	var missingOwnPR *chainhash.Hash
	prs := make([]*wire.MsgMixPairReq, 0, len(ke.SeenPRs))
	var pairing []byte
	for i := range ke.SeenPRs {
		seenPR := &ke.SeenPRs[i]
		pr, ok := p.prs[*seenPR]
		if !ok {
			if uint32(i) == ke.Pos {
				missingOwnPR = seenPR
			}
			continue
		}
		if uint32(i) == ke.Pos && pr.Identity != ke.Identity {
			// This cannot be a bannable rule error.  One peer may
			// have sent an orphan KE first, then another peer the
			// PR, and we must not ban the peer who sent only the
			// PR if this is called by reconsiderOrphans.
			err := fmt.Errorf("KE identity does not match own PR " +
				"at unmixed position")
			return nil, ruleError(err)
		}
		if pairing == nil {
			var err error
			pairing, err = pr.Pairing()
			if err != nil {
				return nil, err
			}
		} else {
			pairing2, err := pr.Pairing()
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(pairing, pairing2) {
				// This likewise cannot be a bannable rule
				// error.  Peers may relay a KE without
				// knowing any but the identity's own PR.
				err := fmt.Errorf("referenced PRs are incompatible")
				return nil, ruleError(err)
			}
		}
	}
	if missingOwnPR != nil {
		p.orphans[*hash] = &orphan{
			message:  ke,
			accepted: time.Now(),
		}
		orphansByID := p.orphansByID[*id]
		if orphansByID == nil {
			orphansByID = make(map[chainhash.Hash]mixing.Message)
			p.orphansByID[*id] = orphansByID
		}
		orphansByID[*hash] = ke
		err := &MissingOwnPRError{
			MissingPR: *missingOwnPR,
		}
		return nil, err
	}

	sid := ke.SessionID
	ses := p.sessions[sid]

	// Create a session for the first KE
	if ses == nil {
		expiry := ^uint32(0)
		for i := range prs {
			prExpiry := prs[i].Expires()
			if expiry > prExpiry {
				expiry = prExpiry
			}
		}
		ses = &session{
			sid:    sid,
			prs:    ke.SeenPRs,
			expiry: expiry,
			hashes: make(map[chainhash.Hash]struct{}),
			bc:     broadcast{ch: make(chan struct{})},
		}
		p.sessions[sid] = ses
	}

	p.acceptEntry(ke, msgtypeKE, hash, id, ses)
	p.latestKE[*id] = ke
	return ke, nil
}

func (p *Pool) acceptEntry(msg mixing.Message, msgtype msgtype, hash *chainhash.Hash,
	id *[33]byte, ses *session) {

	ses.hashes[*hash] = struct{}{}
	e := entry{
		hash:     *hash,
		sid:      ses.sid,
		recvTime: time.Now(),
		msg:      msg,
		msgtype:  msgtype,
	}
	p.pool[*hash] = e
	p.messagesByIdentity[*id] = append(p.messagesByIdentity[*id], *hash)

	if cm, ok := msg.(*wire.MsgMixConfirm); ok {
		p.sessionsByTxHash[cm.Mix.TxHash()] = ses
	}

	ses.incrementCountFor(msgtype)
	ses.bc.signal()
}

func confirmed(minConf, txHeight, curHeight int64) bool {
	return confirms(txHeight, curHeight) >= minConf
}

func confirms(txHeight, curHeight int64) int64 {
	switch {
	case txHeight == -1, txHeight > curHeight:
		return 0
	default:
		return curHeight - txHeight + 1
	}
}

// isDustAmount determines whether a transaction output value and script length would
// cause the output to be considered dust.  Transactions with dust outputs are
// not standard and are rejected by mempools with default policies.
func isDustAmount(amount int64, scriptSize int, relayFeePerKb int64) bool {
	// Calculate the total (estimated) cost to the network.  This is
	// calculated using the serialize size of the output plus the serial
	// size of a transaction input which redeems it.  The output is assumed
	// to be compressed P2PKH as this is the most common script type.  Use
	// the average size of a compressed P2PKH redeem input (165) rather than
	// the largest possible (txsizes.RedeemP2PKHInputSize).
	totalSize := 8 + 2 + wire.VarIntSerializeSize(uint64(scriptSize)) +
		scriptSize + 165

	// Dust is defined as an output value where the total cost to the network
	// (output size + input size) is greater than 1/3 of the relay fee.
	return amount*1000/(3*int64(totalSize)) < relayFeePerKb
}

func checkFee(pr *wire.MsgMixPairReq, feeRate int64) error {
	fee := pr.InputValue - int64(pr.MessageCount)*pr.MixAmount
	if pr.Change != nil {
		fee -= pr.Change.Value
	}

	estimatedSize := estimateP2PKHv0SerializeSize(len(pr.UTXOs),
		int(pr.MessageCount), pr.Change != nil)
	requiredFee := feeForSerializeSize(feeRate, estimatedSize)
	if fee < requiredFee {
		return ruleError(ErrLowInput)
	}

	return nil
}

func feeForSerializeSize(relayFeePerKb int64, txSerializeSize int) int64 {
	fee := relayFeePerKb * int64(txSerializeSize) / 1000

	if fee == 0 && relayFeePerKb > 0 {
		fee = relayFeePerKb
	}

	const maxAmount = 21e6 * 1e8
	if fee < 0 || fee > maxAmount {
		fee = maxAmount
	}

	return fee
}

const (
	redeemP2PKHv0SigScriptSize = 1 + 73 + 1 + 33
	p2pkhv0PkScriptSize        = 1 + 1 + 1 + 20 + 1 + 1
)

func estimateP2PKHv0SerializeSize(inputs, outputs int, hasChange bool) int {
	// Sum the estimated sizes of the inputs and outputs.
	txInsSize := inputs * estimateInputSize(redeemP2PKHv0SigScriptSize)
	txOutsSize := outputs * estimateOutputSize(p2pkhv0PkScriptSize)

	changeSize := 0
	if hasChange {
		changeSize = estimateOutputSize(p2pkhv0PkScriptSize)
		outputs++
	}

	// 12 additional bytes are for version, locktime and expiry.
	return 12 + (2 * wire.VarIntSerializeSize(uint64(inputs))) +
		wire.VarIntSerializeSize(uint64(outputs)) +
		txInsSize + txOutsSize + changeSize
}

// estimateInputSize returns the worst case serialize size estimate for a tx input
func estimateInputSize(scriptSize int) int {
	return 32 + // previous tx
		4 + // output index
		1 + // tree
		8 + // amount
		4 + // block height
		4 + // block index
		wire.VarIntSerializeSize(uint64(scriptSize)) + // size of script
		scriptSize + // script itself
		4 // sequence
}

// estimateOutputSize returns the worst case serialize size estimate for a tx output
func estimateOutputSize(scriptSize int) int {
	return 8 + // previous tx
		2 + // version
		wire.VarIntSerializeSize(uint64(scriptSize)) + // size of script
		scriptSize // script itself
}
