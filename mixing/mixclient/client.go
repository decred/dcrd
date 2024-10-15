// Copyright (c) 2023-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixclient

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"math/bits"
	"runtime"
	"sort"
	"sync"
	"time"

	"decred.org/cspp/v2/solverrpc"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/crypto/rand"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/mixing"
	"github.com/decred/dcrd/mixing/internal/chacha20prng"
	"github.com/decred/dcrd/mixing/mixpool"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
	"github.com/decred/slog"
	"golang.org/x/sync/errgroup"
)

// MinPeers is the minimum number of peers required for a mix run to proceed.
const MinPeers = 4

const pairingFlags byte = 0

const (
	timeoutDuration = 30 * time.Second
	maxJitter       = timeoutDuration / 10
	msgJitter       = 300 * time.Millisecond
	peerJitter      = maxJitter - msgJitter
)

// expiredPRErr indicates that a dicemix session failed to complete due to the
// submitted pair request expiring.
func expiredPRErr(pr *wire.MsgMixPairReq) error {
	return fmt.Errorf("mixing pair request %v by %x expired", pr.Hash(), pr.Pub())
}

var (
	errOnlyKEsBroadcasted = errors.New("session ended without mix occurring")
	errTriggeredBlame     = errors.New("blame required")
)

const (
	ctTimeout = 1 + iota
	srTimeout
	dcTimeout
	cmTimeout
)

func blameTimedOut(sesLog *sessionLogger, sesRun *sessionRun, timeoutMessage int) blamedIdentities {
	var blamed blamedIdentities
	var stage string
	for _, p := range sesRun.peers {
		switch timeoutMessage {
		case ctTimeout:
			stage = "CT"
			if p.ct == nil {
				blamed = append(blamed, *p.id)
			}
		case srTimeout:
			stage = "SR"
			if p.sr == nil {
				blamed = append(blamed, *p.id)
			}
		case dcTimeout:
			stage = "DC"
			if p.dc == nil {
				blamed = append(blamed, *p.id)
			}
		case cmTimeout:
			stage = "CM"
			if p.cm == nil {
				blamed = append(blamed, *p.id)
			}
		}
	}
	sesLog.logf("blaming %x during run (%s timeout)", []identity(blamed), stage)
	return blamed
}

// Wallet signs mix transactions and listens for and broadcasts mixing
// protocol messages.
//
// While wallets are responsible for generating mixed addresses, this duty is
// performed by the generator function provided to NewCoinJoin rather than
// this interface.  This allows each CoinJoin to pass in a generator closure
// for different BIP0032 accounts and branches.
type Wallet interface {
	BestBlock() (uint32, chainhash.Hash)

	// Mixpool returns access to the wallet's mixing message pool.
	//
	// The mixpool should only be used for message access and deletion,
	// but never publishing; SubmitMixMessage must be used instead for
	// message publishing.
	Mixpool() *mixpool.Pool

	// SubmitMixMessage submits a mixing message to the wallet's mixpool
	// and broadcasts it to the network.
	SubmitMixMessage(ctx context.Context, msg mixing.Message) error

	// SignInput adds a signature script to a transaction input.
	SignInput(tx *wire.MsgTx, index int, prevScript []byte) error

	// PublishTransaction adds the transaction to the wallet and publishes
	// it to the network.
	PublishTransaction(ctx context.Context, tx *wire.MsgTx) error
}

type deadlines struct {
	epoch  time.Time
	recvKE time.Time
	recvCT time.Time
	recvSR time.Time
	recvDC time.Time
	recvCM time.Time
}

func (d *deadlines) start(begin time.Time) {
	t := begin
	add := func() time.Time {
		t = t.Add(timeoutDuration)
		return t
	}
	d.recvKE = add()
	d.recvCT = add()
	d.recvSR = add()
	d.recvDC = add()
	d.recvCM = add()
}

func (d *deadlines) shift() {
	d.recvKE = d.recvCT
	d.recvCT = d.recvSR
	d.recvSR = d.recvDC
	d.recvDC = d.recvCM
	d.recvCM = d.recvCM.Add(timeoutDuration)
}

func (d *deadlines) restart() {
	d.start(d.recvCM)
}

// peer represents a participating client in a peer-to-peer mixing session.
// Some fields only pertain to peers created by this wallet, while the rest
// are used during blame assignment.
type peer struct {
	ctx    context.Context
	client *Client
	jitter time.Duration

	res chan error

	pub      *secp256k1.PublicKey
	priv     *secp256k1.PrivateKey
	id       *identity // serialized pubkey
	pr       *wire.MsgMixPairReq
	coinjoin *CoinJoin
	kx       *mixing.KX

	prngSeed [32]byte
	prng     *chacha20prng.Reader

	rs    *wire.MsgMixSecrets
	srMsg []*big.Int   // random numbers for the exponential slot reservation mix
	dcMsg wire.MixVect // anonymized messages to publish in XOR mix

	ke *wire.MsgMixKeyExchange
	ct *wire.MsgMixCiphertexts
	sr *wire.MsgMixSlotReserve
	fp *wire.MsgMixFactoredPoly
	dc *wire.MsgMixDCNet
	cm *wire.MsgMixConfirm

	// Unmixed positions.  May change over multiple sessions/runs.
	myVk    uint32
	myStart uint32

	// Exponential slot reservation mix
	srKP  [][][]byte // shared keys for exp dc-net
	srMix [][]*big.Int

	// XOR DC-net
	dcKP  [][]wire.MixVect
	dcNet []wire.MixVect

	// Whether peer misbehavior was detected by this peer, and initial
	// secrets will be revealed by c.blame().
	triggeredBlame bool

	// Whether this peer represents a remote peer created from revealed secrets;
	// used during blaming.
	remote bool
}

func newRemotePeer(pr *wire.MsgMixPairReq) *peer {
	return &peer{
		id:     &pr.Identity,
		pr:     pr,
		remote: true,
	}
}

func generateSecp256k1() (*secp256k1.PublicKey, *secp256k1.PrivateKey, error) {
	privateKey, err := secp256k1.GeneratePrivateKeyFromRand(rand.Reader())
	if err != nil {
		return nil, nil, err
	}

	publicKey := privateKey.PubKey()

	return publicKey, privateKey, nil
}

// pairedSessions tracks the waiting and in-progress mix sessions performed by
// one or more local peers using compatible pairings.
type pairedSessions struct {
	localPeers map[identity]*peer
	pairing    []byte
	runs       []sessionRun
}

type sessionRun struct {
	sid  [32]byte
	mtot uint32

	// Whether this run must generate fresh KX keys, SR/DC messages.
	freshGen bool

	deadlines

	// Peers sorted by PR hashes.  Each peer's myVk is its index in this
	// slice.
	prs     []*wire.MsgMixPairReq
	peers   []*peer
	mcounts []uint32
	roots   []*big.Int

	// Finalized coinjoin of a successful run.
	cj *CoinJoin
}

type queueWork struct {
	p   *peer
	f   func(p *peer) error
	res chan error
}

// Client manages local mixing client sessions.
type Client struct {
	wallet  Wallet
	mixpool *mixpool.Pool

	// Pending and active sessions and peers (both local and, when
	// blaming, remote).
	pairings map[string]*pairedSessions
	height   uint32
	mu       sync.Mutex

	warming   chan struct{}
	workQueue chan *queueWork

	pairingWG sync.WaitGroup

	blake256Hasher   hash.Hash
	blake256HasherMu sync.Mutex

	epoch   time.Duration
	prFlags byte

	logger slog.Logger

	testTickC chan struct{}
	testHooks map[hook]hookFunc
}

// NewClient creates a wallet's mixing client manager.
func NewClient(w Wallet) *Client {
	var prFlags byte
	err := solverrpc.StartSolver()
	if err == nil {
		prFlags |= mixing.PRFlagCanSolveRoots
	}

	height, _ := w.BestBlock()
	return &Client{
		wallet:         w,
		mixpool:        w.Mixpool(),
		pairings:       make(map[string]*pairedSessions),
		warming:        make(chan struct{}),
		workQueue:      make(chan *queueWork, runtime.NumCPU()),
		blake256Hasher: blake256.New(),
		epoch:          w.Mixpool().Epoch(),
		prFlags:        prFlags,
		height:         height,
	}
}

func (c *Client) SetLogger(l slog.Logger) {
	c.logger = l
}

func (c *Client) log(args ...interface{}) {
	if c.logger == nil {
		return
	}

	c.logger.Debug(args...)
}

func (c *Client) logf(format string, args ...interface{}) {
	if c.logger == nil {
		return
	}

	c.logger.Debugf(format, args...)
}

func (c *Client) logerrf(format string, args ...interface{}) {
	if c.logger == nil {
		return
	}

	c.logger.Errorf(format, args...)
}

func (c *Client) sessionLog(sid [32]byte) *sessionLogger {
	return &sessionLogger{sid: sid, logger: c.logger}
}

type sessionLogger struct {
	sid    [32]byte
	logger slog.Logger
}

func (l *sessionLogger) logf(format string, args ...interface{}) {
	if l.logger == nil {
		return
	}

	l.logger.Debugf("sid=%x "+format, append([]interface{}{l.sid[:]}, args...)...)
}

// Run runs the client manager, blocking until after the context is
// cancelled.
func (c *Client) Run(ctx context.Context) error {
	c.mu.Lock()
	select {
	case <-c.warming:
		c.warming = make(chan struct{})
	default:
	}
	c.mu.Unlock()

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return c.epochTicker(ctx)
	})
	for i := 0; i < runtime.NumCPU(); i++ {
		g.Go(func() error {
			return c.peerWorker(ctx)
		})
	}
	return g.Wait()
}

func (c *Client) peerWorker(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case w := <-c.workQueue:
			w.res <- w.f(w.p)
		}
	}
}

// forLocalPeers is a helper method that runs a callback on all local peers of
// a session run.  The calls are executed concurrently on peer worker goroutines.
func (c *Client) forLocalPeers(ctx context.Context, s *sessionRun, f func(p *peer) error) error {
	resChans := make([]chan error, 0, len(s.peers))

	for _, p := range s.peers {
		if p.remote {
			continue
		}

		res := make(chan error, 1)
		resChans = append(resChans, res)
		qwork := &queueWork{
			p:   p,
			f:   f,
			res: res,
		}
		select {
		case <-ctx.Done():
			res <- ctx.Err()
		case c.workQueue <- qwork:
		}
	}

	var errs = make([]error, len(resChans))
	for i := range errs {
		errs[i] = <-resChans[i]
	}
	return errors.Join(errs...)
}

type delayedMsg struct {
	t time.Time
	m mixing.Message
	p *peer
}

func (c *Client) sendLocalPeerMsgs(ctx context.Context, s *sessionRun, mayTriggerBlame bool,
	m func(p *peer) mixing.Message) error {

	msgs := make([]delayedMsg, 0, len(s.peers))

	now := time.Now()
	for _, p := range s.peers {
		if p.remote || p.ctx.Err() != nil {
			continue
		}
		msg := m(p)
		if mayTriggerBlame && p.triggeredBlame {
			msg = p.rs
		}
		msgs = append(msgs, delayedMsg{
			t: now.Add(p.msgJitter()),
			m: msg,
			p: p,
		})
	}
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].t.Before(msgs[j].t)
	})

	resChans := make([]chan error, 0, len(s.peers))
	for i := range msgs {
		res := make(chan error, 1)
		resChans = append(resChans, res)
		m := msgs[i]
		time.Sleep(time.Until(m.t))
		qsend := &queueWork{
			p: m.p,
			f: func(p *peer) error {
				return p.signAndSubmit(m.m)
			},
			res: res,
		}
		select {
		case <-ctx.Done():
			res <- ctx.Err()
		case c.workQueue <- qsend:
		}
	}
	var errs = make([]error, len(resChans))
	for i := range errs {
		errs[i] = <-resChans[i]
	}
	return errors.Join(errs...)
}

// waitForEpoch blocks until the next epoch, or errors when the context is
// cancelled early.  Returns the calculated epoch for stage timeout
// calculations.
func (c *Client) waitForEpoch(ctx context.Context) (time.Time, error) {
	now := time.Now().UTC()
	epoch := now.Truncate(c.epoch).Add(c.epoch)
	duration := epoch.Sub(now)
	timer := time.NewTimer(duration)
	select {
	case <-ctx.Done():
		if !timer.Stop() {
			<-timer.C
		}
		return epoch, ctx.Err()
	case <-c.testTickC:
		if !timer.Stop() {
			<-timer.C
		}
		return epoch, nil
	case <-timer.C:
		return epoch, nil
	}
}

func (p *peer) msgJitter() time.Duration {
	return p.jitter + rand.Duration(msgJitter)
}

// prDelay waits until an appropriate time before the PR should be authored
// and published.  PRs will not be sent within +/-30s of the epoch, and a
// small amount of jitter is added to help avoid timing deanonymization
// attacks.
func (c *Client) prDelay(ctx context.Context, p *peer) error {
	now := time.Now().UTC()
	epoch := now.Truncate(c.epoch).Add(c.epoch)
	sendBefore := epoch.Add(-timeoutDuration - maxJitter)
	sendAfter := epoch.Add(timeoutDuration)
	var wait time.Duration
	if !now.Before(sendBefore) {
		wait = sendAfter.Sub(now)
		sendBefore = sendBefore.Add(c.epoch)
	}
	wait += p.msgJitter() + rand.Duration(sendBefore.Sub(now))
	timer := time.NewTimer(wait)
	select {
	case <-ctx.Done():
		if !timer.Stop() {
			<-timer.C
		}
		return ctx.Err()
	case <-c.testTickC:
		if !timer.Stop() {
			<-timer.C
		}
		return nil
	case <-timer.C:
		return nil
	}
}

func (c *Client) testTick() {
	c.testTickC <- struct{}{}
}

func (c *Client) testHook(stage hook, s *sessionRun, p *peer) {
	if hook, ok := c.testHooks[stage]; ok {
		hook(c, s, p)
	}
}

func (p *peer) signAndHash(m mixing.Message) error {
	err := mixing.SignMessage(m, p.priv)
	if err != nil {
		return err
	}

	p.client.blake256HasherMu.Lock()
	m.WriteHash(p.client.blake256Hasher)
	p.client.blake256HasherMu.Unlock()

	return nil
}

func (p *peer) submit(m mixing.Message) error {
	return p.client.wallet.SubmitMixMessage(p.ctx, m)
}

func (p *peer) signAndSubmit(m mixing.Message) error {
	if m == nil {
		return nil
	}
	err := p.signAndHash(m)
	if err != nil {
		return err
	}
	return p.submit(m)
}

func (c *Client) newPairings(pairing []byte, peers map[identity]*peer) *pairedSessions {
	if peers == nil {
		peers = make(map[identity]*peer)
	}
	ps := &pairedSessions{
		localPeers: peers,
		pairing:    pairing,
		runs:       nil,
	}
	return ps
}

func (c *Client) removeUnresponsiveDuringEpoch(prs []*wire.MsgMixPairReq, prevEpoch uint64) {
	pairingPRs := make(map[string][]*wire.MsgMixPairReq)

	for _, pr := range prs {
		pairing, err := pr.Pairing()
		if err != nil {
			continue
		}
		pairingPRs[string(pairing)] = append(pairingPRs[string(pairing)], pr)
	}

	for pairStr, prs := range pairingPRs {
		kes := c.mixpool.ReceiveKEsByPairing([]byte(pairStr), prevEpoch)
		if len(kes) < MinPeers {
			return
		}
		c.mixpool.RemoveUnresponsiveDuringEpoch(prs, prevEpoch)
	}
}

func (c *Client) epochTicker(ctx context.Context) error {
	prevPRs := c.mixpool.MixPRs()

	// Wait for the next epoch + the KE timeout + extra duration for local
	// clock differences, then remove any previous pair requests that are
	// unresponsive in a pairing that other peers are responsive in.
	firstEpoch, err := c.waitForEpoch(ctx)
	if err != nil {
		return err
	}

	select {
	case <-time.After(timeoutDuration + 2*time.Second):
	case <-c.testTickC:
	case <-ctx.Done():
		return err
	}
	c.mu.Lock()
	c.removeUnresponsiveDuringEpoch(prevPRs, uint64(firstEpoch.Unix()))
	close(c.warming)
	c.mu.Unlock()

	for {
		epoch, err := c.waitForEpoch(ctx)
		if err != nil {
			return ctx.Err()
		}

		c.log("Epoch tick")

		// Wait for any previous pairSession calls to timeout if they
		// have not yet formed a session before the next epoch tick.
		c.pairingWG.Wait()

		c.mu.Lock()

		// XXX: This needs a better solution; it may remove runs that have
		// all confirm messages but with invalid signatures.  This eventually
		// results in "expired PR" errors.
		// Ideally, we would behave like dcrd and only remove sessions that have
		// mined mix transactions or are otherwise double spent in a block.
		c.mixpool.RemoveConfirmedSessions()
		c.expireMessages()

		for _, p := range c.pairings {
			prs := c.mixpool.CompatiblePRs(p.pairing)
			prsMap := make(map[identity]struct{})
			for _, pr := range prs {
				prsMap[pr.Identity] = struct{}{}
			}

			// Clone the p.localPeers map, only including PRs
			// currently accepted to mixpool.  Adding additional
			// waiting local peers must not add more to the map in
			// use by pairSession, and deleting peers in a formed
			// session from the pending map must not inadvertently
			// remove from pairSession's ps.localPeers map.
			localPeers := make(map[identity]*peer)
			for id, peer := range p.localPeers {
				if _, ok := prsMap[id]; ok {
					localPeers[id] = peer
				}
			}
			c.logf("Have %d compatible/%d local PRs waiting for pairing %x",
				len(prs), len(localPeers), p.pairing)

			ps := *p
			ps.localPeers = localPeers
			for id, peer := range p.localPeers {
				ps.localPeers[id] = peer
			}
			// pairSession calls Done once the session is formed
			// and the selected peers have been removed from then
			// pending pairing.
			c.pairingWG.Add(1)
			go c.pairSession(ctx, &ps, prs, epoch)
		}
		c.mu.Unlock()
	}
}

// Dicemix performs a new mixing session for a coinjoin mix transaction.
func (c *Client) Dicemix(ctx context.Context, cj *CoinJoin) error {
	select {
	case <-c.warming:
	case <-ctx.Done():
		return ctx.Err()
	}

	pub, priv, err := generateSecp256k1()
	if err != nil {
		return err
	}

	p := &peer{
		ctx:      ctx,
		client:   c,
		jitter:   rand.Duration(peerJitter),
		res:      make(chan error, 1),
		pub:      pub,
		priv:     priv,
		id:       (*[33]byte)(pub.SerializeCompressed()),
		coinjoin: cj,
	}

	err = c.prDelay(ctx, p)
	if err != nil {
		return err
	}

	pr, err := wire.NewMsgMixPairReq(*p.id, cj.prExpiry, cj.mixValue,
		string(mixing.ScriptClassP2PKHv0), cj.tx.Version,
		cj.tx.LockTime, cj.mcount, cj.inputValue, cj.prUTXOs,
		cj.change, c.prFlags, pairingFlags)
	if err != nil {
		return err
	}
	err = p.signAndHash(pr)
	if err != nil {
		return err
	}
	pairingID, err := pr.Pairing()
	if err != nil {
		return err
	}
	p.pr = pr

	c.logf("Created local peer id=%x PR=%s", p.id[:], p.pr.Hash())

	c.mu.Lock()
	pairing := c.pairings[string(pairingID)]
	if pairing == nil {
		pairing = c.newPairings(pairingID, nil)
		c.pairings[string(pairingID)] = pairing
	}
	pairing.localPeers[*p.id] = p
	c.mu.Unlock()

	err = p.submit(pr)
	if err != nil {
		c.mu.Lock()
		delete(pairing.localPeers, *p.id)
		if len(pairing.localPeers) == 0 {
			delete(c.pairings, string(pairingID))
		}
		c.mu.Unlock()
		return err
	}

	select {
	case res := <-p.res:
		return res
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ExpireMessages will cause the removal all mixpool messages and sessions
// that indicate an expiry height at or before the height parameter and
// removes any pending DiceMix sessions that did not complete.
func (c *Client) ExpireMessages(height uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Just mark the current height for expireMessages() to use later,
	// after epoch strikes.  This allows us to keep using PRs during
	// session forming even if they expire, which is required to continue
	// mixing paired sessions that are performing reruns.
	c.height = height
}

func (c *Client) expireMessages() {
	c.mixpool.ExpireMessages(c.height)

	for pairID, ps := range c.pairings {
		for id, p := range ps.localPeers {
			prHash := p.pr.Hash()
			if !c.mixpool.HaveMessage(&prHash) {
				delete(ps.localPeers, id)
				// p.res is buffered.  If the write is
				// blocked, we have already served this peer
				// or sent another error.
				select {
				case p.res <- expiredPRErr(p.pr):
				default:
				}

			}
		}
		if len(ps.localPeers) == 0 {
			delete(c.pairings, pairID)
		}
	}
}

func (c *Client) pairSession(ctx context.Context, ps *pairedSessions, prs []*wire.MsgMixPairReq, epoch time.Time) {
	// This session pairing attempt, and calling pairSession again with
	// fresh PRs, must end before the next call to pairSession for this
	// pairing type.
	unixEpoch := uint64(epoch.Unix())
	nextEpoch := epoch.Add(c.epoch)
	ctx, cancel := context.WithDeadline(ctx, nextEpoch)
	defer cancel()

	// Defer removal of completed mix messages.  Add local peers back to
	// the client to be paired in a later session if the mix was
	// unsuccessful or only some peers were included.
	var mixedSession *sessionRun
	var unresponsive []*wire.MsgMixPairReq
	defer func() {
		c.removeUnresponsiveDuringEpoch(unresponsive, unixEpoch)

		unmixedPeers := ps.localPeers
		if mixedSession != nil && mixedSession.cj != nil {
			for _, pr := range mixedSession.prs {
				delete(unmixedPeers, pr.Identity)
			}

			// XXX: Removing these later in the background is a hack to
			// work around a race in SPV mode.  Deleting the session too
			// soon may result in our wallet serving notfound messages
			// for CM messages, which will increment ban score.
			go func() {
				time.Sleep(10 * time.Second)
				c.logf("sid=%x removing mixed session completed with transaction %v",
					mixedSession.sid[:], mixedSession.cj.txHash)
				c.mixpool.RemoveSession(mixedSession.sid)
			}()
		}
		if len(unmixedPeers) == 0 {
			return
		}

		for _, p := range unmixedPeers {
			p.ke = nil
			p.ct = nil
			p.sr = nil
			p.fp = nil
			p.dc = nil
			p.cm = nil
			p.rs = nil
		}

		c.mu.Lock()
		pendingPairing := c.pairings[string(ps.pairing)]
		if pendingPairing == nil {
			pendingPairing = c.newPairings(ps.pairing, unmixedPeers)
			c.pairings[string(ps.pairing)] = pendingPairing
		} else {
			for id, p := range unmixedPeers {
				prHash := p.pr.Hash()
				if p.ctx.Err() == nil && c.mixpool.HaveMessage(&prHash) {
					pendingPairing.localPeers[id] = p
				}
			}
		}
		c.mu.Unlock()
	}()

	var madePairing bool
	defer func() {
		if !madePairing {
			c.pairingWG.Done()
		}
	}()

	var sesLog *sessionLogger
	var currentRun *sessionRun
	var rerun *sessionRun
	var d deadlines
	d.epoch = epoch
	d.start(epoch)
	for {
		if rerun == nil {
			sid := mixing.SortPRsForSession(prs, unixEpoch)
			sesLog = c.sessionLog(sid)

			sesRun := sessionRun{
				sid:       sid,
				prs:       prs,
				freshGen:  true,
				deadlines: d,
				mcounts:   make([]uint32, 0, len(prs)),
			}
			ps.runs = append(ps.runs, sesRun)
			currentRun = &ps.runs[len(ps.runs)-1]

		} else {
			ps.runs = append(ps.runs, *rerun)
			currentRun = &ps.runs[len(ps.runs)-1]
			sesLog = c.sessionLog(currentRun.sid)
			// rerun is not assigned nil here to please the
			// linter.  All code paths that reenter this loop will
			// set it again.
		}

		prs = currentRun.prs
		prHashes := make([]chainhash.Hash, len(prs))
		for i := range prs {
			prHashes[i] = prs[i].Hash()
		}

		var m uint32
		var localPeerCount, localUncancelledCount int
		for i, pr := range prs {
			p := ps.localPeers[pr.Identity]
			if p != nil {
				localPeerCount++
				if p.ctx.Err() == nil {
					localUncancelledCount++
				}
			} else {
				p = newRemotePeer(pr)
			}
			p.myVk = uint32(i)
			p.myStart = m

			currentRun.peers = append(currentRun.peers, p)
			currentRun.mcounts = append(currentRun.mcounts, p.pr.MessageCount)

			m += p.pr.MessageCount
		}
		currentRun.mtot = m

		sesLog.logf("created session for pairid=%x from %d total %d local PRs %s",
			ps.pairing, len(prHashes), localPeerCount, prHashes)

		if localUncancelledCount == 0 {
			sesLog.logf("no more active local peers")
			return
		}

		sesLog.logf("len(ps.runs)=%d", len(ps.runs))

		c.testHook(hookBeforeRun, currentRun, nil)
		err := c.run(ctx, ps, &madePairing)

		var sizeLimitedErr *sizeLimited
		if errors.As(err, &sizeLimitedErr) {
			if len(sizeLimitedErr.prs) < MinPeers {
				sesLog.logf("Aborting session with too few remaining peers")
				return
			}

			d.shift()

			sesLog.logf("Recreating as session %x due to standard tx size limits (pairid=%x)",
				sizeLimitedErr.sid[:], ps.pairing)

			rerun = &sessionRun{
				sid:       sizeLimitedErr.sid,
				prs:       sizeLimitedErr.prs,
				freshGen:  false,
				deadlines: d,
			}
			continue
		}

		var altses *alternateSession
		if errors.As(err, &altses) {
			if altses.err != nil {
				sesLog.logf("Unable to recreate session: %v", altses.err)
				return
			}

			if len(altses.prs) < MinPeers {
				sesLog.logf("Aborting session with too few remaining peers")
				return
			}

			d.shift()

			if currentRun.sid != altses.sid {
				sesLog.logf("Recreating as session %x (pairid=%x)", altses.sid, ps.pairing)
				unresponsive = append(unresponsive, altses.unresponsive...)
			}

			rerun = &sessionRun{
				sid:       altses.sid,
				prs:       altses.prs,
				freshGen:  false,
				deadlines: d,
			}
			continue
		}

		var blamed blamedIdentities
		revealedSecrets := false
		if errors.Is(err, errTriggeredBlame) || errors.Is(err, mixpool.ErrSecretsRevealed) {
			revealedSecrets = true
			err := c.blame(ctx, currentRun)
			if !errors.As(err, &blamed) {
				sesLog.logf("Aborting session for failed blame assignment: %v", err)
				return
			}
		}
		if blamed != nil || errors.As(err, &blamed) {
			sesLog.logf("Identified %d blamed peers %x", len(blamed), []identity(blamed))

			// Blamed peers were identified, either during the run
			// in a way that all participants could have observed,
			// or following revealing secrets and blame
			// assignment.  Begin a rerun excluding these peers.
			rerun = excludeBlamed(currentRun, blamed, revealedSecrets)
			continue
		}

		// When only KEs are broadcasted, the session was not viable
		// due to lack of peers or a peer capable of solving the
		// roots.  Return without setting the mixed session.
		if errors.Is(err, errOnlyKEsBroadcasted) {
			return
		}

		// Any other run error is not actionable.
		if err != nil {
			sesLog.logf("Run error: %v", err)
			return
		}

		mixedSession = currentRun
		return
	}
}

func (c *Client) run(ctx context.Context, ps *pairedSessions, madePairing *bool) error {
	var blamed blamedIdentities

	mp := c.wallet.Mixpool()
	sesRun := &ps.runs[len(ps.runs)-1]
	prs := sesRun.prs

	d := &sesRun.deadlines
	unixEpoch := uint64(d.epoch.Unix())

	sesLog := c.sessionLog(sesRun.sid)

	// A map of identity public keys to their PR position sort all
	// messages in the same order as the PRs are ordered.
	identityIndices := make(map[identity]int)
	for i, pr := range prs {
		identityIndices[pr.Identity] = i
	}

	seenPRs := make([]chainhash.Hash, len(prs))
	for i := range prs {
		seenPRs[i] = prs[i].Hash()
	}

	err := c.forLocalPeers(ctx, sesRun, func(p *peer) error {
		p.coinjoin.resetUnmixed(prs)

		if sesRun.freshGen {
			// Generate a new PRNG seed
			rand.Read(p.prngSeed[:])
			p.prng = chacha20prng.New(p.prngSeed[:], 0)

			// Generate fresh keys from this run's PRNG
			kx, err := mixing.NewKX(p.prng)
			if err != nil {
				return err
			}
			p.kx = kx

			// Generate fresh SR messages.
			// These must not be created from the PRNG; the Go
			// standard library function is not guaranteed to read
			// the same byte count in all versions.
			p.srMsg = make([]*big.Int, p.pr.MessageCount)
			for i := range p.srMsg {
				p.srMsg[i] = rand.BigInt(mixing.F)
			}

			// Generate fresh DC messages
			p.dcMsg, err = p.coinjoin.gen()
			if err != nil {
				return err
			}
			if len(p.dcMsg) != int(p.pr.MessageCount) {
				return errors.New("Gen returned wrong message count")
			}
			for _, m := range p.dcMsg {
				if len(m) != msize {
					err := fmt.Errorf("Gen returned bad message "+
						"length [%v != %v]", len(m), msize)
					return err
				}
			}
		}

		// Perform key exchange
		srMsgBytes := make([][]byte, len(p.srMsg))
		for i := range p.srMsg {
			srMsgBytes[i] = p.srMsg[i].Bytes()
		}
		rs := wire.NewMsgMixSecrets(*p.id, sesRun.sid, 0,
			p.prngSeed, srMsgBytes, p.dcMsg)
		c.blake256HasherMu.Lock()
		commitment := rs.Commitment(c.blake256Hasher)
		c.blake256HasherMu.Unlock()
		ecdhPub := *(*[33]byte)(p.kx.ECDHPublicKey.SerializeCompressed())
		pqPub := *p.kx.PQPublicKey
		ke := wire.NewMsgMixKeyExchange(*p.id, sesRun.sid, unixEpoch, 0,
			uint32(identityIndices[*p.id]), ecdhPub, pqPub, commitment,
			seenPRs)

		p.ke = ke
		p.rs = rs
		return nil
	})
	if err != nil {
		sesLog.logf("%v", err)
	}
	err = c.sendLocalPeerMsgs(ctx, sesRun, true, func(p *peer) mixing.Message {
		if p.ke == nil {
			return nil
		}
		return p.ke
	})
	if err != nil {
		sesLog.logf("%v", err)
	}

	// Only continue attempting to form the session if there are minimum
	// peers available and at least one of them is capable of solving the
	// roots.
	if len(prs) < MinPeers {
		sesLog.logf("Pairing %x: minimum peer requirement unmet", ps.pairing)
		return errOnlyKEsBroadcasted
	}
	haveSolver := false
	for _, pr := range prs {
		if pr.Flags&mixing.PRFlagCanSolveRoots != 0 {
			haveSolver = true
			break
		}
	}
	if !haveSolver {
		sesLog.logf("Pairing %x: no solver available", ps.pairing)
		return errOnlyKEsBroadcasted
	}

	// Receive key exchange messages.
	//
	// In run 0, it is possible that the attempted session (the session ID
	// used by our PR messages) does not match the same session attempted
	// by all other remote peers, due to not all peers initially agreeing
	// on the same set of PRs.  When there is agreement, the session can
	// be run like normal, and any reruns will only be performed with some
	// of the original peers removed.
	//
	// When there is session disagreement, we attempt to find a new
	// session that the majority of peers will be able to participate in.
	// All KE messages that match the pairing ID are received, and each
	// seen PRs slice is checked.  PRs that were never followed up by a KE
	// are immediately excluded.
	var kes []*wire.MsgMixKeyExchange
	recvKEs := func(sesRun *sessionRun) (kes []*wire.MsgMixKeyExchange, err error) {
		rcv := new(mixpool.Received)
		rcv.Sid = sesRun.sid
		rcv.KEs = make([]*wire.MsgMixKeyExchange, 0, len(sesRun.prs))
		ctx, cancel := context.WithDeadline(ctx, d.recvKE)
		defer cancel()
		err = mp.Receive(ctx, rcv)
		if ctx.Err() != nil {
			err = fmt.Errorf("session %x KE receive context cancelled: %w",
				sesRun.sid[:], ctx.Err())
		}
		return rcv.KEs, err
	}

	switch {
	case !*madePairing:
		// Receive KEs for the last attempted session.  Local
		// peers may have been modified (new keys generated, and myVk
		// indexes changed) if this is a recreated session, and we
		// cannot continue mix using these messages.
		//
		// XXX: do we need to keep peer info available for previous
		// session attempts?  It is possible that previous sessions
		// may be operable now if all wallets have come to agree on a
		// previous session we also tried to form.
		kes, err = recvKEs(sesRun)
		if err == nil && len(kes) == len(sesRun.prs) {
			break
		}

		// Alternate session needs to be attempted.  Do not form an
		// alternate session if we are about to enter into the next
		// epoch.  The session forming will be performed by a new
		// goroutine started by the epoch ticker, possibly with
		// additional PRs.
		nextEpoch := d.epoch.Add(c.epoch)
		if time.Now().Add(timeoutDuration).After(nextEpoch) {
			c.logf("Aborting session %x after %d attempts",
				sesRun.sid[:], len(ps.runs))
			return errOnlyKEsBroadcasted
		}

		return c.alternateSession(ps.pairing, sesRun.prs, d)

	default:
		kes, err = recvKEs(sesRun)
		if err != nil {
			return err
		}
	}

	// Before confirming the pairing, check all of the agreed-upon PRs
	// that they will not result in a coinjoin transaction that exceeds
	// the standard size limits.
	//
	// PRs are randomly ordered in each epoch based on the session ID, so
	// they can be iterated in order to discover any PR that would
	// increase the final coinjoin size above the limits.
	if !*madePairing {
		var sizeExcluded []*wire.MsgMixPairReq
		var cjSize coinjoinSize
		for _, pr := range sesRun.prs {
			contributedInputs := len(pr.UTXOs)
			mcount := int(pr.MessageCount)
			if err := cjSize.join(contributedInputs, mcount, pr.Change); err != nil {
				sizeExcluded = append(sizeExcluded, pr)
			}
		}

		if len(sizeExcluded) > 0 {
			// sizeExcluded and sesRun.prs are in the same order;
			// can zip down both to create the slice of peers that
			// can continue the mix.
			excl := sizeExcluded
			kept := make([]*wire.MsgMixPairReq, 0, len(sesRun.prs))
			for _, pr := range sesRun.prs {
				if pr != excl[0] {
					kept = append(kept, pr)
					continue
				}
				excl = excl[1:]
				if len(excl) == 0 {
					break
				}
			}

			sid := mixing.SortPRsForSession(kept, unixEpoch)
			return &sizeLimited{
				prs:      kept,
				sid:      sid,
				excluded: sizeExcluded,
			}
		}
	}

	for _, ke := range kes {
		if idx, ok := identityIndices[ke.Identity]; ok {
			sesRun.peers[idx].ke = ke
		}
	}

	// Remove paired local peers from waiting pairing.
	if !*madePairing {
		c.mu.Lock()
		if waiting := c.pairings[string(ps.pairing)]; waiting != nil {
			for id := range ps.localPeers {
				delete(waiting.localPeers, id)
			}
			if len(waiting.localPeers) == 0 {
				delete(c.pairings, string(ps.pairing))
			}
		}
		c.mu.Unlock()

		*madePairing = true
		c.pairingWG.Done()
	}

	sort.Slice(kes, func(i, j int) bool {
		a := identityIndices[kes[i].Identity]
		b := identityIndices[kes[j].Identity]
		return a < b
	})
	seenKEs := make([]chainhash.Hash, len(kes))
	for i := range kes {
		seenKEs[i] = kes[i].Hash()
	}

	involvesLocalPeers := false
	for _, ke := range kes {
		if ps.localPeers[ke.Identity] != nil {
			involvesLocalPeers = true
			break
		}
	}
	if !involvesLocalPeers {
		return errors.New("excluded all local peers")
	}

	ecdhPublicKeys := make([]*secp256k1.PublicKey, 0, len(prs))
	pqpk := make([]*mixing.PQPublicKey, 0, len(prs))
	for _, ke := range kes {
		ecdhPub, err := secp256k1.ParsePubKey(ke.ECDH[:])
		if err != nil {
			blamed = append(blamed, ke.Identity)
			continue
		}
		ecdhPublicKeys = append(ecdhPublicKeys, ecdhPub)
		pqpk = append(pqpk, &ke.PQPK)
	}
	if len(blamed) > 0 {
		sesLog.logf("blaming %x during run (invalid ECDH pubkeys)", []identity(blamed))
		return blamed
	}

	err = c.forLocalPeers(ctx, sesRun, func(p *peer) error {
		// Create shared keys and ciphextexts for each peer
		pqct, err := p.kx.Encapsulate(p.prng, pqpk, int(p.myVk))
		if err != nil {
			return err
		}

		// Send ciphertext messages
		ct := wire.NewMsgMixCiphertexts(*p.id, sesRun.sid, 0, pqct, seenKEs)
		p.ct = ct
		c.testHook(hookBeforePeerCTPublish, sesRun, p)
		return nil
	})
	if err != nil {
		sesLog.logf("%v", err)
	}
	err = c.sendLocalPeerMsgs(ctx, sesRun, true, func(p *peer) mixing.Message {
		if p.ct == nil {
			return nil
		}
		return p.ct
	})
	if err != nil {
		sesLog.logf("%v", err)
	}

	// Receive all ciphertext messages
	rcv := new(mixpool.Received)
	rcv.Sid = sesRun.sid
	rcv.KEs = nil
	rcv.CTs = make([]*wire.MsgMixCiphertexts, 0, len(prs))
	rcvCtx, rcvCtxCancel := context.WithDeadline(ctx, d.recvCT)
	err = mp.Receive(rcvCtx, rcv)
	rcvCtxCancel()
	cts := rcv.CTs
	for _, ct := range cts {
		if idx, ok := identityIndices[ct.Identity]; ok {
			sesRun.peers[idx].ct = ct
		}
	}
	if err != nil {
		return err
	}
	if len(cts) != len(prs) {
		// Blame peers
		sesLog.logf("Received %d CTs for %d peers; rerunning", len(cts), len(prs))
		return blameTimedOut(sesLog, sesRun, ctTimeout)
	}
	sort.Slice(cts, func(i, j int) bool {
		a := identityIndices[cts[i].Identity]
		b := identityIndices[cts[j].Identity]
		return a < b
	})
	seenCTs := make([]chainhash.Hash, len(cts))
	for i := range cts {
		seenCTs[i] = cts[i].Hash()
	}

	blamedMap := make(map[identity]struct{})
	var blamedMapMu sync.Mutex
	err = c.forLocalPeers(ctx, sesRun, func(p *peer) error {
		revealed := &mixing.RevealedKeys{
			ECDHPublicKeys: ecdhPublicKeys,
			Ciphertexts:    make([]mixing.PQCiphertext, 0, len(prs)),
			MyIndex:        p.myVk,
		}
		for _, ct := range cts {
			if len(ct.Ciphertexts) != len(prs) {
				// Everyone sees this, can rerun without full blame now.
				blamedMapMu.Lock()
				blamedMap[ct.Identity] = struct{}{}
				blamedMapMu.Unlock()
				return nil
			}
			revealed.Ciphertexts = append(revealed.Ciphertexts, ct.Ciphertexts[p.myVk])
		}

		// Derive shared secret keys
		shared, err := p.kx.SharedSecrets(revealed, sesRun.sid[:], 0, sesRun.mcounts)
		if err != nil {
			p.triggeredBlame = true
			return errTriggeredBlame
		}
		p.srKP = shared.SRSecrets
		p.dcKP = shared.DCSecrets

		// Calculate slot reservation DC-net vectors
		p.srMix = make([][]*big.Int, p.pr.MessageCount)
		for i := range p.srMix {
			pads := mixing.SRMixPads(p.srKP[i], p.myStart+uint32(i))
			p.srMix[i] = mixing.SRMix(p.srMsg[i], pads)
		}
		srMixBytes := mixing.IntVectorsToBytes(p.srMix)

		// Broadcast message commitment and exponential DC-mix vectors for slot
		// reservations.
		sr := wire.NewMsgMixSlotReserve(*p.id, sesRun.sid, 0, srMixBytes, seenCTs)
		p.sr = sr
		c.testHook(hookBeforePeerSRPublish, sesRun, p)
		return nil
	})
	if len(blamedMap) > 0 {
		for id := range blamedMap {
			blamed = append(blamed, id)
		}
		sesLog.logf("blaming %x during run (wrong ciphertext count)", []identity(blamed))
		return blamed
	}
	sendErr := c.sendLocalPeerMsgs(ctx, sesRun, true, func(p *peer) mixing.Message {
		if p.sr == nil {
			return nil
		}
		return p.sr
	})
	if sendErr != nil {
		sesLog.logf("%v", sendErr)
	}
	if err != nil {
		sesLog.logf("%v", err)
		return err
	}

	// Receive all slot reservation messages
	rcv.CTs = nil
	rcv.SRs = make([]*wire.MsgMixSlotReserve, 0, len(prs))
	rcvCtx, rcvCtxCancel = context.WithDeadline(ctx, d.recvSR)
	err = mp.Receive(rcvCtx, rcv)
	rcvCtxCancel()
	srs := rcv.SRs
	for _, sr := range srs {
		if idx, ok := identityIndices[sr.Identity]; ok {
			sesRun.peers[idx].sr = sr
		}
	}
	if err != nil {
		return err
	}
	if len(srs) != len(prs) {
		// Blame peers
		sesLog.logf("Received %d SRs for %d peers; rerunning", len(srs), len(prs))
		return blameTimedOut(sesLog, sesRun, srTimeout)
	}
	sort.Slice(srs, func(i, j int) bool {
		a := identityIndices[srs[i].Identity]
		b := identityIndices[srs[j].Identity]
		return a < b
	})
	seenSRs := make([]chainhash.Hash, len(srs))
	for i := range srs {
		seenSRs[i] = srs[i].Hash()
	}

	// Recover roots
	vs := make([][][]byte, 0, len(prs))
	for _, sr := range srs {
		vs = append(vs, sr.DCMix...)
	}
	powerSums := mixing.AddVectors(mixing.IntVectorsFromBytes(vs)...)
	coeffs := mixing.Coefficients(powerSums)
	rcvCtx, rcvCtxCancel = context.WithDeadline(ctx, d.recvSR)
	publishedRootsC := make(chan struct{})
	defer func() { <-publishedRootsC }()
	roots, err := c.roots(rcvCtx, seenSRs, sesRun, coeffs, mixing.F, publishedRootsC)
	rcvCtxCancel()
	if err != nil {
		return err
	}
	sesRun.roots = roots

	err = c.forLocalPeers(ctx, sesRun, func(p *peer) error {
		// Find reserved slots
		slots := make([]uint32, 0, p.pr.MessageCount)
		for _, m := range p.srMsg {
			slot := constTimeSlotSearch(m, roots)
			if slot == -1 {
				p.triggeredBlame = true
				return errTriggeredBlame
			}
			slots = append(slots, uint32(slot))
		}

		// Calculate XOR DC-net vectors
		p.dcNet = make([]wire.MixVect, p.pr.MessageCount)
		for i, slot := range slots {
			my := p.myStart + uint32(i)
			pads := mixing.DCMixPads(p.dcKP[i], my)
			p.dcNet[i] = wire.MixVect(mixing.DCMix(pads, p.dcMsg[i][:], slot))
		}

		// Broadcast XOR DC-net vectors.
		dc := wire.NewMsgMixDCNet(*p.id, sesRun.sid, 0, p.dcNet, seenSRs)
		p.dc = dc
		c.testHook(hookBeforePeerDCPublish, sesRun, p)
		return nil
	})
	sendErr = c.sendLocalPeerMsgs(ctx, sesRun, true, func(p *peer) mixing.Message {
		if p.dc == nil {
			return nil
		}
		return p.dc
	})
	if sendErr != nil {
		sesLog.logf("%v", err)
	}
	if err != nil {
		sesLog.logf("DC-net error: %v", err)
		return err
	}

	// Receive all DC messages
	rcv.SRs = nil
	rcv.DCs = make([]*wire.MsgMixDCNet, 0, len(prs))
	rcvCtx, rcvCtxCancel = context.WithDeadline(ctx, d.recvDC)
	err = mp.Receive(rcvCtx, rcv)
	rcvCtxCancel()
	dcs := rcv.DCs
	for _, dc := range dcs {
		if idx, ok := identityIndices[dc.Identity]; ok {
			sesRun.peers[idx].dc = dc
		}
	}
	if err != nil {
		return err
	}
	if len(dcs) != len(prs) {
		// Blame peers
		sesLog.logf("Received %d DCs for %d peers; rerunning", len(dcs), len(prs))
		return blameTimedOut(sesLog, sesRun, dcTimeout)
	}
	sort.Slice(dcs, func(i, j int) bool {
		a := identityIndices[dcs[i].Identity]
		b := identityIndices[dcs[j].Identity]
		return a < b
	})
	seenDCs := make([]chainhash.Hash, len(dcs))
	for i := range dcs {
		seenDCs[i] = dcs[i].Hash()
	}

	// Solve XOR dc-net
	dcVecs := make([]mixing.Vec, 0, sesRun.mtot)
	for i, dc := range dcs {
		if uint32(len(dc.DCNet)) != sesRun.mcounts[i] {
			blamed = append(blamed, dc.Identity)
			continue
		}
		for _, vec := range dc.DCNet {
			dcVecs = append(dcVecs, mixing.Vec(vec))
		}
	}
	if len(blamed) > 0 {
		sesLog.logf("blaming %x during run (wrong DC-net count)", []identity(blamed))
		return blamed
	}
	mixedMsgs := mixing.XorVectors(dcVecs)
	err = c.forLocalPeers(ctx, sesRun, func(p *peer) error {
		// Add outputs for each mixed message
		for i := range mixedMsgs {
			mixedMsg := mixedMsgs[i][:]
			p.coinjoin.addMixedMessage(mixedMsg)
		}
		p.coinjoin.sort()

		// Confirm that our messages are present, and sign each peer's
		// provided inputs.
		err := p.coinjoin.confirm(c.wallet)
		if errors.Is(err, errMissingGen) {
			sesLog.logf("Missing message; blaming and rerunning")
			p.triggeredBlame = true
			return errTriggeredBlame
		}
		if err != nil {
			p.res <- err
			return err
		}

		// Broadcast partially signed mix tx
		cm := wire.NewMsgMixConfirm(*p.id, sesRun.sid, 0,
			p.coinjoin.Tx().Copy(), seenDCs)
		p.cm = cm
		return nil
	})
	sendErr = c.sendLocalPeerMsgs(ctx, sesRun, true, func(p *peer) mixing.Message {
		if p.cm == nil {
			return nil
		}
		return p.cm
	})
	if sendErr != nil {
		sesLog.logf("%v", sendErr)
	}
	if err != nil {
		sesLog.logf("Confirm error: %v", err)
		return err
	}

	// Receive all CM messages
	rcv.DCs = nil
	rcv.CMs = make([]*wire.MsgMixConfirm, 0, len(prs))
	rcvCtx, rcvCtxCancel = context.WithDeadline(ctx, d.recvCM)
	err = mp.Receive(rcvCtx, rcv)
	rcvCtxCancel()
	cms := rcv.CMs
	for _, cm := range cms {
		if idx, ok := identityIndices[cm.Identity]; ok {
			sesRun.peers[idx].cm = cm
		}
	}
	if err != nil {
		return err
	}
	if len(cms) != len(prs) {
		// Blame peers
		sesLog.logf("Received %d CMs for %d peers; rerunning", len(cms), len(prs))
		return blameTimedOut(sesLog, sesRun, cmTimeout)
	}
	sort.Slice(cms, func(i, j int) bool {
		a := identityIndices[cms[i].Identity]
		b := identityIndices[cms[j].Identity]
		return a < b
	})

	// Merge and validate all signatures.  Only a single coinjoin is
	// needed at this point.
	var cj *CoinJoin
	var lowestJitter time.Duration
	utxos := make(map[wire.OutPoint]*wire.MixPairReqUTXO)
	for _, p := range sesRun.peers {
		if cj == nil && !p.remote {
			cj = p.coinjoin
		}
		if !p.remote && (lowestJitter == 0 || lowestJitter > p.jitter) {
			lowestJitter = p.jitter
		}
		for i := range p.pr.UTXOs {
			utxo := &p.pr.UTXOs[i]
			utxos[utxo.OutPoint] = utxo
		}
	}
	for _, cm := range cms {
		err := cj.mergeSignatures(cm)
		if err != nil {
			blamed = append(blamed, cm.Identity)
		}
	}
	if len(blamed) > 0 {
		sesLog.logf("blaming %x during run (confirmed wrong coinjoin)", []identity(blamed))
		return blamed
	}
	err = c.validateMergedCoinjoin(cj, prs, utxos)
	if err != nil {
		return err
	}

	time.Sleep(lowestJitter + rand.Duration(msgJitter))
	err = c.wallet.PublishTransaction(context.Background(), cj.tx)
	if err != nil {
		return err
	}

	c.forLocalPeers(ctx, sesRun, func(p *peer) error {
		select {
		case p.res <- nil:
		default:
		}
		return nil
	})

	sesRun.cj = cj

	return nil
}

func (c *Client) solvesRoots() bool {
	return c.prFlags&mixing.PRFlagCanSolveRoots != 0
}

// roots returns the solutions to the slot reservation polynomial.  If the
// client is capable of solving the roots directly, it does so and publishes
// the result to all peers.  If the client is incapable of solving the roots,
// it waits for a solution.
func (c *Client) roots(ctx context.Context, seenSRs []chainhash.Hash,
	sesRun *sessionRun, a []*big.Int, F *big.Int, publishedRoots chan struct{}) ([]*big.Int, error) {

	if c.solvesRoots() {
		roots, err := solverrpc.Roots(a, F)
		if err != nil || len(roots) != len(a)-1 {
			c.forLocalPeers(ctx, sesRun, func(p *peer) error {
				p.triggeredBlame = true
				return nil
			})
			close(publishedRoots)
			return nil, errTriggeredBlame
		}
		rootBytes := make([][]byte, len(roots))
		for i, root := range roots {
			rootBytes[i] = root.Bytes()
		}
		c.forLocalPeers(ctx, sesRun, func(p *peer) error {
			p.fp = wire.NewMsgMixFactoredPoly(*p.id, sesRun.sid,
				0, rootBytes, seenSRs)
			return nil
		})
		// Don't wait for these messages to send.
		go func() {
			err := c.sendLocalPeerMsgs(ctx, sesRun, false, func(p *peer) mixing.Message {
				return p.fp
			})
			if ctx.Err() == nil && err != nil {
				c.logf("sid=%x %v", sesRun.sid[:], err)
			}
			publishedRoots <- struct{}{}
		}()
		return roots, nil
	}

	close(publishedRoots)

	// Clients unable to solve their own roots must wait for solutions.
	// We can return a result as soon as we read any valid factored
	// polynomial message that provides the solutions for this SR
	// polynomial.
	rcv := &mixpool.Received{
		Sid: sesRun.sid,
		FPs: make([]*wire.MsgMixFactoredPoly, 0, 1),
	}
	roots := make([]*big.Int, 0, len(a)-1)
	checkedFPByIdentity := make(map[identity]struct{})
	for {
		rcv.FPs = rcv.FPs[:0]
		err := c.mixpool.Receive(ctx, rcv)
		if err != nil {
			return nil, err
		}

	FPs:
		for _, fp := range rcv.FPs {
			if _, ok := checkedFPByIdentity[fp.Identity]; ok {
				continue
			}

			roots = roots[:0]
			duplicateRoots := make(map[string]struct{})

			// Ignore invalid solutions; we only need one
			// valid one.
			if len(fp.Roots) != len(a)-1 {
				continue
			}
			roots := make([]*big.Int, 0, sesRun.mtot)
			if len(fp.Roots) <= len(roots) {
				continue
			}

			for i := range fp.Roots {
				root := new(big.Int).SetBytes(fp.Roots[i])
				if !mixing.InField(root) || !mixing.IsRoot(root, a) {
					continue FPs
				}
				rootStr := root.String()
				if _, ok := duplicateRoots[rootStr]; !ok {
					duplicateRoots[rootStr] = struct{}{}
					roots = append(roots, root)
				}
			}
			if len(roots) == len(a)-1 {
				return roots, nil
			}

			checkedFPByIdentity[fp.Identity] = struct{}{}
		}

		rcv.FPs = make([]*wire.MsgMixFactoredPoly, 0, len(rcv.FPs)+1)
	}
}

func (c *Client) validateMergedCoinjoin(cj *CoinJoin, prs []*wire.MsgMixPairReq,
	utxos map[wire.OutPoint]*wire.MixPairReqUTXO) error {

	prevScript := []byte{
		0:  0, // Opcode tag (if any needed)
		1:  txscript.OP_DUP,
		2:  txscript.OP_HASH160,
		3:  txscript.OP_DATA_20,
		24: txscript.OP_EQUALVERIFY,
		25: txscript.OP_CHECKSIG,
	}
	opcode := &prevScript[0]
	hash160 := prevScript[4:24]

	prevScriptForUTXO := func(utxo *wire.MixPairReqUTXO) []byte {
		switch utxo.Opcode {
		case 0:
			copy(hash160, stdaddr.Hash160(utxo.PubKey))
			return prevScript[1:]

		case txscript.OP_SSGEN, txscript.OP_SSRTX, txscript.OP_TGEN:
			*opcode = utxo.Opcode
			copy(hash160, stdaddr.Hash160(utxo.PubKey))
			return prevScript

		default:
			return nil
		}
	}

	blamedInputs := make(map[wire.OutPoint]struct{})
	const scriptFlags = txscript.ScriptDiscourageUpgradableNops |
		txscript.ScriptVerifyCleanStack |
		txscript.ScriptVerifyCheckLockTimeVerify |
		txscript.ScriptVerifyCheckSequenceVerify |
		txscript.ScriptVerifyTreasury
	const scriptVersion = 0
	for i, in := range cj.tx.TxIn {
		utxo, ok := utxos[in.PreviousOutPoint]
		if !ok {
			c.logf("warn: prev pubkey for %v not found during tx sig validate",
				&in.PreviousOutPoint)
			blamedInputs[in.PreviousOutPoint] = struct{}{}
			continue
		}

		pkscript := prevScriptForUTXO(utxo)
		engine, err := txscript.NewEngine(pkscript, cj.tx, i,
			scriptFlags, scriptVersion, nil)
		if err != nil {
			c.logf("warn: blaming peer for error creating script engine: %v", err)
			blamedInputs[in.PreviousOutPoint] = struct{}{}
			continue
		}

		if err := engine.Execute(); err != nil {
			txHash := cj.tx.TxHash()
			c.logf("warn: coinjoin %s input %d prev outpoint %v script fails to execute: %v",
				txHash, i, &in.PreviousOutPoint, err)
			blamedInputs[in.PreviousOutPoint] = struct{}{}
		}
	}

	if len(blamedInputs) == 0 {
		return nil
	}

	var blamed blamedIdentities
prs:
	for _, pr := range prs {
		for _, in := range pr.UTXOs {
			if _, ok := blamedInputs[in.OutPoint]; ok {
				c.logf("warn: blaming %x for failed script execute",
					pr.Identity[:])
				blamed = append(blamed, pr.Identity)
				continue prs
			}
		}
	}
	return blamed
}

type sizeLimited struct {
	prs      []*wire.MsgMixPairReq
	sid      [32]byte
	excluded []*wire.MsgMixPairReq
}

func (e *sizeLimited) Error() string {
	return "mix session coinjoin exceeds standard size limits"
}

type alternateSession struct {
	prs          []*wire.MsgMixPairReq
	sid          [32]byte
	unresponsive []*wire.MsgMixPairReq
	err          error
}

func (e *alternateSession) Error() string {
	return "recreated alternate session"
}
func (e *alternateSession) Unwrap() error {
	return e.err
}

func (c *Client) alternateSession(pairing []byte, prs []*wire.MsgMixPairReq, d *deadlines) *alternateSession {
	unixEpoch := uint64(d.epoch.Unix())

	kes := c.mixpool.ReceiveKEsByPairing(pairing, unixEpoch)

	// Sort KEs by identity first (just to group these together) followed
	// by the total referenced PR counts in increasing order (most recent
	// KEs first).  When ranging over KEs below, this will allow us to
	// consider the order in which other peers created their KEs, and how
	// they are forming their sessions.
	sort.Slice(kes, func(i, j int) bool {
		a := kes[i]
		b := kes[j]
		if bytes.Compare(a.Identity[:], b.Identity[:]) == -1 {
			return true
		}
		if len(a.SeenPRs) < len(b.SeenPRs) {
			return true
		}
		return false
	})

	prsByHash := make(map[chainhash.Hash]*wire.MsgMixPairReq)
	prHashByIdentity := make(map[identity]chainhash.Hash)
	for _, pr := range prs {
		prsByHash[pr.Hash()] = pr
		prHashByIdentity[pr.Identity] = pr.Hash()
	}

	// Only one KE per peer identity (the KE that references the least PR
	// hashes) is used for determining session agreement.
	type peerMsgs struct {
		pr *wire.MsgMixPairReq
		ke *wire.MsgMixKeyExchange
	}
	msgsByIdentityMap := make(map[identity]*peerMsgs)
	for i := range kes {
		ke := kes[i]
		if msgsByIdentityMap[ke.Identity] != nil {
			continue
		}
		pr := prsByHash[prHashByIdentity[ke.Identity]]
		if pr == nil {
			err := fmt.Errorf("Missing PR %s by %x, but have their KE %s",
				prHashByIdentity[ke.Identity], ke.Identity[:], ke.Hash())
			c.log(err)
			continue
		}
		msgsByIdentityMap[ke.Identity] = &peerMsgs{
			pr: pr,
			ke: ke,
		}
	}

	// Discover which peers were unresponsive.  Their PRs will not count
	// towards our calculations.
	var unresponsive []*wire.MsgMixPairReq
	unresponsiveIDs := make(map[identity]*wire.MsgMixPairReq)
	unresponsivePRHashes := make(map[chainhash.Hash]*wire.MsgMixPairReq)
	for _, pr := range prs {
		id := pr.Identity
		if _, ok := msgsByIdentityMap[id]; !ok {
			c.logf("Identity %x (PR %s) is unresponsive", id[:], pr.Hash())
			unresponsive = append(unresponsive, pr)
			unresponsiveIDs[id] = pr
			unresponsivePRHashes[pr.Hash()] = pr
		}
	}

	// A total count of all unique PRs/identities (including not just
	// those we have observed PRs for, but also other unknown PR hashes
	// referenced by other peers) is needed.  All participating peers must
	// come to agreement on this number, otherwise the exclusion rules
	// based on majority may result in different peers being removed and
	// agreement never being reached.  Any PRs not seen by a majority of
	// other peers (i.e. without counting peers seeing their own PR) are
	// immediately removed from any formed session attempt.
	prCounts := make(map[chainhash.Hash]int)
	for _, msgs := range msgsByIdentityMap {
		selfPRHash := msgs.pr.Hash()
		for _, prHash := range msgs.ke.SeenPRs {
			// Self-references do not count.
			if selfPRHash == prHash {
				continue
			}
			// Do not count unresponsive peer's PRs.
			if _, ok := unresponsivePRHashes[prHash]; ok {
				continue
			}
			prCounts[prHash]++
		}
	}

	totalPRs := len(prCounts)
	neededForMajority := totalPRs / 2
	remainingPRs := make([]chainhash.Hash, 0, totalPRs)
	for prHash, count := range prCounts {
		if count >= neededForMajority {
			remainingPRs = append(remainingPRs, prHash)
		}
	}

	// Incrementally remove PRs that are not seen by all peers, and
	// recalculate total observance counts after excluding the peer that
	// was dropped.  A session is formed when all remaining PR counts
	// equal one less than the total number of PRs still considered (due
	// to not counting observing one's own PR).
	for {
		if len(remainingPRs) < MinPeers {
			return &alternateSession{
				unresponsive: unresponsive,
				err:          ErrTooFewPeers,
			}
		}

		// Sort remaining PRs by increasing seen counts first, then
		// lexicographically by PR hash.
		sort.Slice(remainingPRs, func(i, j int) bool {
			a := &remainingPRs[i]
			b := &remainingPRs[j]
			switch {
			case prCounts[*a] < prCounts[*b]:
				return true
			case prCounts[*a] > prCounts[*b]:
				return false
			default:
				return bytes.Compare(a[:], b[:]) == -1
			}
		})

		if prCounts[remainingPRs[0]] == len(remainingPRs)-1 {
			prs := make([]*wire.MsgMixPairReq, len(remainingPRs))
			for i, prHash := range remainingPRs {
				prs[i] = prsByHash[prHash]
				if prs[i] == nil {
					// Agreement should be reached, but we
					// won't be able to participate in the
					// formed session.
					return &alternateSession{
						unresponsive: unresponsive,
						err:          ErrUnknownPRs,
					}
				}
			}
			newSessionID := mixing.SortPRsForSession(prs, unixEpoch)
			return &alternateSession{
				prs:          prs,
				sid:          newSessionID,
				unresponsive: unresponsive,
			}
		}

		c.logf("Removing PR %s, was only seen %d times", remainingPRs[0], prCounts[remainingPRs[0]])
		remainingPRs = remainingPRs[1:]

		// Recalculate and reassign prCounts for next iteration.
		prCounts = make(map[chainhash.Hash]int, len(remainingPRs))
		for _, hash := range remainingPRs {
			prCounts[hash] = 0
		}
		for _, hash := range remainingPRs {
			pr := prsByHash[hash]
			if pr == nil {
				continue
			}
			selfPRHash := pr.Hash()
			msgs := msgsByIdentityMap[pr.Identity]
			if msgs == nil {
				continue
			}
			ke := msgs.ke
			for _, prHash := range ke.SeenPRs {
				if prHash == selfPRHash {
					continue
				}
				if _, ok := prCounts[prHash]; ok {
					prCounts[prHash]++
				}
			}
		}
	}

}

func excludeBlamed(prevRun *sessionRun, blamed blamedIdentities, revealedSecrets bool) *sessionRun {
	blamedMap := make(map[identity]struct{})
	for _, id := range blamed {
		blamedMap[id] = struct{}{}
	}

	prs := make([]*wire.MsgMixPairReq, 0, len(prevRun.prs)-1)
	for _, p := range prevRun.peers {
		if _, ok := blamedMap[*p.id]; ok {
			// Should never happen except during tests.
			if !p.remote {
				p.res <- &testPeerBlamedError{p}
			}

			continue
		}

		prs = append(prs, p.pr)
	}

	d := prevRun.deadlines
	d.restart()

	unixEpoch := prevRun.epoch.Unix()
	sid := mixing.SortPRsForSession(prs, uint64(unixEpoch))

	// mtot, peers, mcounts are all recalculated from the prs before
	// calling run()
	nextRun := &sessionRun{
		sid:       sid,
		freshGen:  revealedSecrets,
		prs:       prs,
		deadlines: d,
	}
	return nextRun
}

var fieldLen = uint(len(mixing.F.Bytes()))

// constTimeSlotSearch searches for the index of secret in roots in constant time.
// Returns -1 if the secret is not found.
func constTimeSlotSearch(secret *big.Int, roots []*big.Int) int {
	paddedSecret := make([]byte, fieldLen)
	secretBytes := secret.Bytes()
	off, _ := bits.Sub(fieldLen, uint(len(secretBytes)), 0)
	copy(paddedSecret[off:], secretBytes)

	slot := -1
	buf := make([]byte, fieldLen)
	for i := range roots {
		rootBytes := roots[i].Bytes()
		off, _ := bits.Sub(fieldLen, uint(len(rootBytes)), 0)
		copy(buf[off:], rootBytes)
		cmp := subtle.ConstantTimeCompare(paddedSecret, buf)
		slot = subtle.ConstantTimeSelect(cmp, i, slot)
		for j := range buf {
			buf[j] = 0
		}
	}
	return slot
}
