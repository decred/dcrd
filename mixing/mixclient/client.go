// Copyright (c) 2023-2025 The Decred developers
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
	"sync/atomic"
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

const pairingVersion byte = 1

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

// Constants specifying which peer messages to publish.
const (
	msgKE = 1 << iota
	msgCT
	msgSR
	msgDC
	msgFP
	msgCM
	msgRS
)

func blameTimedOut(sesRun *sessionRun, timeoutMessage int) error {
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
	if len(blamed) > 0 {
		sesRun.logf("blaming %x during run (%s timeout)", []identity(blamed), stage)
		return blamed
	}
	return errBlameFailed
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

// peerRunState describes the peer state that changes across different
// sessions/runs.
type peerRunState struct {
	prngSeed [32]byte
	prng     *chacha20prng.Reader
	kx       *mixing.KX // derived from PRNG

	srMsg []*big.Int   // random (non-PRNG) numbers for the exponential slot reservation mix
	dcMsg wire.MixVect // anonymized messages (HASH160s) to publish in XOR mix

	ke *wire.MsgMixKeyExchange
	ct *wire.MsgMixCiphertexts
	sr *wire.MsgMixSlotReserve
	fp *wire.MsgMixFactoredPoly
	dc *wire.MsgMixDCNet
	cm *wire.MsgMixConfirm
	rs *wire.MsgMixSecrets

	// Unmixed positions.
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
}

// peer represents a participating client in a peer-to-peer mixing session.
// Some fields only pertain to peers created by this wallet, while the rest
// are used during blame assignment.
type peer struct {
	client *Client
	jitter time.Duration

	res chan error

	pub      *secp256k1.PublicKey
	priv     *secp256k1.PrivateKey
	id       *identity // serialized pubkey
	pr       *wire.MsgMixPairReq
	coinjoin *CoinJoin

	peerRunState

	// Whether this peer represents a remote peer created from revealed secrets;
	// used during blaming.
	remote bool
}

// cloneLocalPeer creates a new peer instance representing a local peer
// client, sharing the caller context and result channels, but resetting all
// per-session fields (if freshGen is true), or only copying the PRNG and
// fields directly derived from it (when freshGen is false).
func (p *peer) cloneLocalPeer(freshGen bool) *peer {
	if p.remote {
		panic("cloneLocalPeer: remote peer")
	}

	p2 := *p
	p2.peerRunState = peerRunState{}

	if !freshGen {
		p2.prngSeed = p.prngSeed
		p2.prng = p.prng
		p2.kx = p.kx
		p2.srMsg = p.srMsg
		p2.dcMsg = p.dcMsg
	}

	return &p2
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

type pendingPairing struct {
	localPeers map[identity]*peer
	pairing    []byte
}

// pairedSessions tracks the waiting and in-progress mix sessions performed by
// one or more local peers using compatible pairings.
type pairedSessions struct {
	localPeers map[identity]*peer
	pairing    []byte
	runs       []sessionRun

	epoch time.Time
	deadlines

	// Track state of pairing completion.
	//
	// An agreed-upon pairing means there was agreement on an initial set
	// of peers/KEs.  However, various situations (such as blaming
	// misbehaving peers, or additional session formations after hitting
	// size limits) must not consider the previous sessions with all
	// received KEs.  The peer agreement index clamps down on this by only
	// considering runs in
	// pairedSessions.runs[pairedSessions.peerAgreementRunIdx:]
	peerAgreementRunIdx int
	peerAgreement       bool

	donePairingOnce sync.Once
}

type sessionRun struct {
	sid  [32]byte
	idx  int // in pairedSessions.runs
	mtot uint32

	// Whether this run must generate fresh KX keys, SR/DC messages.
	freshGen bool

	// Peers sorted by PR hashes.  Each peer's myVk is its index in this
	// slice.
	localPeers map[identity]*peer
	prs        []*wire.MsgMixPairReq
	kes        []*wire.MsgMixKeyExchange // set by completePairing
	peers      []*peer
	mcounts    []uint32
	roots      []*big.Int

	// Finalized coinjoin of a successful run.
	cj *CoinJoin

	logger slog.Logger
}

func (s *sessionRun) logf(format string, args ...interface{}) {
	if s.logger == nil {
		return
	}

	s.logger.Debugf("sid=%x/%d "+format, append([]interface{}{s.sid[:], s.idx}, args...)...)
}

func (s *sessionRun) logerrf(format string, args ...interface{}) {
	if s.logger == nil {
		return
	}

	s.logger.Errorf("sid=%x/%d "+format, append([]interface{}{s.sid[:], s.idx}, args...)...)
}

type queueWork struct {
	p   *peer
	f   func(p *peer) error
	res chan error
}

// Client manages local mixing client sessions.
type Client struct {
	// atomics
	atomicPRFlags  uint32
	atomicStopping uint32

	wallet  Wallet
	mixpool *mixpool.Pool

	// Pending and active sessions and peers (both local and, when
	// blaming, remote).
	pendingPairings map[string]*pendingPairing
	height          uint32
	mu              sync.Mutex

	warming   chan struct{}
	workQueue chan *queueWork

	pairingWG sync.WaitGroup
	runWG     sync.WaitGroup

	blake256Hasher   hash.Hash
	blake256HasherMu sync.Mutex

	epoch time.Duration

	stopping chan struct{}

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
		atomicPRFlags:   uint32(prFlags),
		wallet:          w,
		mixpool:         w.Mixpool(),
		pendingPairings: make(map[string]*pendingPairing),
		height:          height,
		warming:         make(chan struct{}),
		workQueue:       make(chan *queueWork, runtime.NumCPU()),
		blake256Hasher:  blake256.New(),
		epoch:           w.Mixpool().Epoch(),
		stopping:        make(chan struct{}),
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

	g, gctx := errgroup.WithContext(context.Background())
	g.Go(func() error {
		<-ctx.Done()
		c.stop()
		return nil
	})
	g.Go(func() error {
		return c.epochTicker(gctx)
	})
	for i := 0; i < runtime.NumCPU(); i++ {
		g.Go(func() error {
			return c.peerWorker(gctx)
		})
	}
	err := g.Wait()

	// Serve errors to all still managed local peers if they have not
	// already received an error.
	c.mu.Lock()
	for _, p := range c.pendingPairings {
		for _, lp := range p.localPeers {
			select {
			case lp.res <- ErrStopping:
			default:
			}
		}
	}
	c.mu.Unlock()

	return err
}

// stop requests clean shutdown of the client.  Any running mixes will be
// completed before Run returns, and no more mixes will be started.
func (c *Client) stop() {
	if atomic.CompareAndSwapUint32(&c.atomicStopping, 0, 1) {
		close(c.stopping)
	}
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
	sendTime time.Time
	deadline time.Time
	m        mixing.Message
	p        *peer
}

func (c *Client) sendLocalPeerMsgs(ctx context.Context, deadline time.Time, s *sessionRun, msgMask uint) error {
	now := time.Now()

	msgs := make([]delayedMsg, 0, len(s.peers)*bits.OnesCount(msgMask))
	for _, p := range s.peers {
		if p.remote {
			continue
		}
		msg := delayedMsg{
			sendTime: now.Add(p.msgJitter()),
			deadline: deadline,
			m:        nil,
			p:        p,
		}
		msgMask := msgMask
		if p.triggeredBlame {
			msgMask |= msgRS
		}
		if msgMask&msgKE == msgKE && p.ke != nil {
			msg.m = p.ke
			msgs = append(msgs, msg)
		}
		if msgMask&msgCT == msgCT && p.ct != nil {
			msg.m = p.ct
			msgs = append(msgs, msg)
		}
		if msgMask&msgSR == msgSR && p.sr != nil {
			msg.m = p.sr
			msgs = append(msgs, msg)
		}
		if msgMask&msgFP == msgFP && p.fp != nil {
			msg.m = p.fp
			msgs = append(msgs, msg)
		}
		if msgMask&msgDC == msgDC && p.dc != nil {
			msg.m = p.dc
			msgs = append(msgs, msg)
		}
		if msgMask&msgCM == msgCM && p.cm != nil {
			msg.m = p.cm
			msgs = append(msgs, msg)
		}
		if msgMask&msgRS == msgRS && p.rs != nil {
			msg.m = p.rs
			msgs = append(msgs, msg)
		}
	}
	sort.SliceStable(msgs, func(i, j int) bool {
		return msgs[i].sendTime.Before(msgs[j].sendTime)
	})

	nilPeerMsg := func(p *peer, msg mixing.Message) {
		switch msg.(type) {
		case *wire.MsgMixKeyExchange:
			p.ke = nil
		case *wire.MsgMixCiphertexts:
			p.ct = nil
		case *wire.MsgMixSlotReserve:
			p.sr = nil
		case *wire.MsgMixFactoredPoly:
			p.fp = nil
		case *wire.MsgMixDCNet:
			p.dc = nil
		case *wire.MsgMixConfirm:
			p.cm = nil
		case *wire.MsgMixSecrets:
			p.rs = nil
		}
	}

	errs := make([]error, 0, len(msgs))

	var sessionCanceledState bool
	sessionCanceled := func() {
		if sessionCanceledState {
			return
		}
		if err := ctx.Err(); err != nil {
			err := fmt.Errorf("session cancelled: %w", err)
			errs = append(errs, err)
			sessionCanceledState = true
		}
	}

	for i := range msgs {
		m := msgs[i]
		select {
		case <-ctx.Done():
			sessionCanceled()
			continue
		case <-time.After(time.Until(m.sendTime)):
		}
		err := m.p.signAndSubmit(m.deadline, m.m)
		if err != nil {
			nilPeerMsg(m.p, m.m)
			errs = append(errs, err)
		}
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
	case <-c.stopping:
		if !timer.Stop() {
			<-timer.C
		}
		c.runWG.Wait()
		return epoch, ErrStopping
	case <-c.testTickC:
		if !timer.Stop() {
			<-timer.C
		}
		return time.Now(), nil
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

func (c *Client) testHook(stage hook, ps *pairedSessions, s *sessionRun, p *peer) {
	if hook, ok := c.testHooks[stage]; ok {
		hook(c, ps, s, p)
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

func (p *peer) submit(deadline time.Time, m mixing.Message) error {
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	return p.client.wallet.SubmitMixMessage(ctx, m)
}

func (p *peer) signAndSubmit(deadline time.Time, m mixing.Message) error {
	if m == nil {
		return nil
	}
	err := p.signAndHash(m)
	if err != nil {
		return err
	}
	return p.submit(deadline, m)
}

func (c *Client) newPendingPairing(pairing []byte) *pendingPairing {
	return &pendingPairing{
		localPeers: make(map[identity]*peer),
		pairing:    pairing,
	}
}

func (c *Client) newPairedSessions(pairing []byte, pendingPeers map[identity]*peer) *pairedSessions {
	if pendingPeers == nil {
		pendingPeers = make(map[identity]*peer)
	}
	ps := &pairedSessions{
		localPeers: pendingPeers,
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
			return err
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

		for _, p := range c.pendingPairings {
			prs := c.mixpool.CompatiblePRs(p.pairing)
			prsMap := make(map[identity]struct{})
			for _, pr := range prs {
				prsMap[pr.Identity] = struct{}{}
			}

			// Clone the pending peers map, only including peers
			// with PRs currently accepted to mixpool.  Adding
			// additional waiting local peers must not add more to
			// the map in use by pairSession, and deleting peers
			// in a formed session from the pending map must not
			// inadvertently remove from pairSession's peers map.
			localPeers := make(map[identity]*peer)
			for id, peer := range p.localPeers {
				if _, ok := prsMap[id]; ok {
					localPeers[id] = peer.cloneLocalPeer(true)
				}
			}

			// Even when we know there are not enough total peers
			// to meet the minimum peer requirement, a run is
			// still formed so that KEs can be broadcast; this
			// must be done so peers can fetch missing PRs.

			c.logf("Have %d compatible/%d local PRs waiting for pairing %x",
				len(prs), len(localPeers), p.pairing)

			// pairSession calls pairingWG.Done once the session
			// is formed and the selected peers have been removed
			// from then pending pairing.
			c.pairingWG.Add(1)
			c.runWG.Add(1)
			ps := c.newPairedSessions(p.pairing, localPeers)
			go func() {
				c.pairSession(ctx, ps, prs, epoch)
				c.runWG.Done()
			}()
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

	prFlags := byte(atomic.LoadUint32(&c.atomicPRFlags))
	pr, err := wire.NewMsgMixPairReq(*p.id, cj.prExpiry, cj.mixValue,
		string(mixing.ScriptClassP2PKHv0), cj.tx.Version,
		cj.tx.LockTime, cj.mcount, cj.inputValue, cj.prUTXOs,
		cj.change, prFlags, pairingVersion)
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
	pending := c.pendingPairings[string(pairingID)]
	if pending == nil {
		pending = c.newPendingPairing(pairingID)
		c.pendingPairings[string(pairingID)] = pending
	}
	pending.localPeers[*p.id] = p
	c.mu.Unlock()

	deadline := time.Now().Add(timeoutDuration)
	err = p.submit(deadline, pr)
	if err != nil {
		c.mu.Lock()
		delete(pending.localPeers, *p.id)
		if len(pending.localPeers) == 0 {
			delete(c.pendingPairings, string(pairingID))
		}
		c.mu.Unlock()
		return err
	}

	return <-p.res
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

	for pairID, p := range c.pendingPairings {
		for id, peer := range p.localPeers {
			prHash := peer.pr.Hash()
			if !c.mixpool.HaveMessage(&prHash) {
				delete(p.localPeers, id)
				// p.res is buffered.  If the write is
				// blocked, we have already served this peer
				// or sent another error.
				select {
				case peer.res <- expiredPRErr(peer.pr):
				default:
				}

			}
		}
		if len(p.localPeers) == 0 {
			delete(c.pendingPairings, pairID)
		}
	}
}

// donePairing decrements the client pairing waitgroup for the paired
// sessions.  This is protected by a sync.Once and safe to call multiple
// times.
func (c *Client) donePairing(ps *pairedSessions) {
	ps.donePairingOnce.Do(c.pairingWG.Done)
}

func (c *Client) pairSession(ctx context.Context, ps *pairedSessions, prs []*wire.MsgMixPairReq, epoch time.Time) {
	defer c.donePairing(ps)

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

		if mixedSession != nil && mixedSession.cj != nil {
			for _, pr := range mixedSession.prs {
				delete(ps.localPeers, pr.Identity)
			}

			// XXX: Removing these later in the background is a hack to
			// work around a race in SPV mode.  Deleting the session too
			// soon may result in our wallet serving notfound messages
			// for CM messages, which will increment ban score.
			go func() {
				time.Sleep(10 * time.Second)
				mixedSession.logf("removing mixed session completed "+
					"with transaction %v", mixedSession.cj.txHash)
				c.mixpool.RemoveSession(mixedSession.sid)
			}()
		}
		if len(ps.localPeers) == 0 {
			return
		}

		c.mu.Lock()
		pendingPairing := c.pendingPairings[string(ps.pairing)]
		if pendingPairing == nil {
			pendingPairing = c.newPendingPairing(ps.pairing)
			c.pendingPairings[string(ps.pairing)] = pendingPairing
		} else {
			for id, p := range ps.localPeers {
				prHash := p.pr.Hash()
				if c.mixpool.HaveMessage(&prHash) {
					pendingPairing.localPeers[id] = p.cloneLocalPeer(true)
				}
			}
		}
		c.mu.Unlock()
	}()

	ps.epoch = epoch
	ps.deadlines.start(epoch)

	sid := mixing.SortPRsForSession(prs, unixEpoch)
	ps.runs = append(ps.runs, sessionRun{
		sid:      sid,
		prs:      prs,
		freshGen: true,
		mcounts:  make([]uint32, 0, len(prs)),
	})
	newRun := &ps.runs[len(ps.runs)-1]

	for {
		if newRun != nil {
			newRun.idx = len(ps.runs) - 1
			newRun.logger = c.logger

			prs = newRun.prs
			prHashes := make([]chainhash.Hash, len(prs))
			newRun.localPeers = make(map[identity]*peer)
			var m uint32
			var localPeerCount int
			for i, pr := range prs {
				prHashes[i] = prs[i].Hash()

				// Peer clones must be made from the previous run's
				// local peer objects (if any) to preserve existing
				// PRNGs and derived secrets and keys.
				peerMap := ps.localPeers
				if newRun.idx > 0 {
					peerMap = ps.runs[newRun.idx-1].localPeers
				}
				p := peerMap[pr.Identity]
				if p != nil {
					p = p.cloneLocalPeer(newRun.freshGen)
					localPeerCount++
					newRun.localPeers[*p.id] = p
				} else {
					p = newRemotePeer(pr)
				}
				p.myVk = uint32(i)
				p.myStart = m

				newRun.peers = append(newRun.peers, p)
				newRun.mcounts = append(newRun.mcounts, p.pr.MessageCount)

				m += p.pr.MessageCount
			}
			newRun.mtot = m

			newRun.logf("created session for pairid=%x from %d total %d local PRs %s",
				ps.pairing, len(prHashes), localPeerCount, prHashes)

			if localPeerCount == 0 {
				newRun.logf("no more active local peers")
				return
			}

			newRun = nil
		}

		c.testHook(hookBeforeRun, ps, &ps.runs[len(ps.runs)-1], nil)
		r, err := c.run(ctx, ps)

		var rerun *sessionRun
		var altses *alternateSession
		var sizeLimitedErr *sizeLimited
		var blamed blamedIdentities
		var revealedSecrets bool
		var requirePeerAgreement bool
		switch {
		case errors.Is(err, errOnlyKEsBroadcasted):
			// When only KEs are broadcasted, the session was not viable
			// due to lack of peers or a peer capable of solving the
			// roots.  Return without setting the mixed session.
			return

		case errors.As(err, &altses):
			// If this errored or has too few peers, keep
			// retrying previous attempts until next epoch,
			// instead of just going away.
			if altses.err != nil {
				r.logf("Unable to recreate session: %v", altses.err)
				ps.deadlines.start(time.Now())
				continue
			}

			if r.sid != altses.sid {
				r.logf("Recreating as session %x (pairid=%x)", altses.sid, ps.pairing)
				unresponsive = append(unresponsive, altses.unresponsive...)
			}

			// Required minimum run index is not incremented for
			// reformed sessions without peer agreement: we must
			// consider receiving KEs from peers who reformed into
			// the same session we previously attempted.
			rerun = &sessionRun{
				sid:      altses.sid,
				prs:      altses.prs,
				freshGen: false,
			}
			err = nil

		case errors.As(err, &sizeLimitedErr):
			if len(sizeLimitedErr.prs) < MinPeers {
				r.logf("Aborting session with too few remaining peers")
				return
			}

			r.logf("Recreating as session %x due to standard tx size limits (pairid=%x)",
				sizeLimitedErr.sid[:], ps.pairing)

			rerun = &sessionRun{
				sid:      sizeLimitedErr.sid,
				prs:      sizeLimitedErr.prs,
				freshGen: false,
			}
			requirePeerAgreement = true
			err = nil

		case errors.Is(err, errTriggeredBlame) || errors.Is(err, mixpool.ErrSecretsRevealed):
			revealedSecrets = true
			err := c.blame(ctx, r)
			if !errors.As(err, &blamed) {
				r.logf("Aborting session for failed blame assignment: %v", err)
				return
			}
			requirePeerAgreement = true
			// err = nil would be an ineffectual assignment here;
			// blamed is non-nil and the following if block will
			// always be entered.
		}

		if blamed != nil || errors.As(err, &blamed) {
			r.logf("Identified %d blamed peers %x", len(blamed), []identity(blamed))

			if len(r.prs)-len(blamed) < MinPeers {
				r.logf("Aborting session with too few remaining peers")
				return
			}

			// Blamed peers were identified, either during the run
			// in a way that all participants could have observed,
			// or following revealing secrets and blame
			// assignment.  Begin a rerun excluding these peers.
			rerun = excludeBlamed(r, unixEpoch, blamed, revealedSecrets)
			requirePeerAgreement = true
			err = nil
		}

		if rerun != nil {
			if ps.runs[len(ps.runs)-1].sid == rerun.sid {
				r := &ps.runs[len(ps.runs)-1]
				r.logf("recreated session matches previous try; " +
					"not creating new run states")
				if ps.peerAgreementRunIdx >= len(ps.runs) {
					// XXX shouldn't happen but fix up anyways
					r.logf("reverting incremented peer agreement index")
					ps.peerAgreementRunIdx = r.idx
				}
			} else {
				ps.runs = append(ps.runs, *rerun)
				newRun = &ps.runs[len(ps.runs)-1]
			}
			ps.deadlines.start(time.Now())
			if requirePeerAgreement {
				ps.peerAgreementRunIdx = len(ps.runs) - 1
			}
			continue
		}

		// Any other run error is not actionable.  Unexpected errors
		// are logged at error level.
		if err != nil {
			r.logerrf("Run error: %v", err)
			return
		}

		mixedSession = r
		return
	}
}

var errIncompletePairing = errors.New("incomplete pairing")

// completePairing waits for all KEs to form a completed session.  Completed
// pairings are checked for in the order the sessions were attempted.
func (c *Client) completePairing(ctx context.Context, ps *pairedSessions) (*sessionRun, error) {
	mp := c.mixpool
	res := make(chan *sessionRun, len(ps.runs))
	errs := make(chan error, len(ps.runs))
	var errCount int

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	wrappedCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	recvKEs := func(ctx context.Context, sesRun *sessionRun) ([]*wire.MsgMixKeyExchange, error) {
		rcv := new(mixpool.Received)
		rcv.Sid = sesRun.sid
		rcv.KEs = make([]*wire.MsgMixKeyExchange, 0, len(sesRun.prs))
		ctx, cancel := context.WithDeadline(ctx, ps.deadlines.recvKE)
		defer cancel()
		err := mp.Receive(ctx, rcv)
		if err != nil {
			return nil, err
		}
		if len(rcv.KEs) != len(sesRun.prs) {
			return nil, errIncompletePairing
		}
		return rcv.KEs, nil
	}

	var wg sync.WaitGroup
	defer func() {
		cancel()
		wg.Wait()
	}()

	for i := ps.peerAgreementRunIdx; i < len(ps.runs); i++ {
		sr := &ps.runs[i]

		// Initially check the mempool with the pre-canceled context
		// to return immediately with KEs received so far.
		kes, err := recvKEs(canceledCtx, sr)
		if err == nil {
			sr.kes = kes
			return sr, nil
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			kes, err := recvKEs(wrappedCtx, sr)
			if err == nil {
				sr.kes = kes
				res <- sr
			} else {
				errs <- err
			}
		}()
	}

	for {
		select {
		case sesRun := <-res:
			return sesRun, nil
		case err := <-errs:
			errCount++
			if errCount == len(ps.runs[ps.peerAgreementRunIdx:]) {
				return nil, err
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (c *Client) run(ctx context.Context, ps *pairedSessions) (sesRun *sessionRun, err error) {
	var blamed blamedIdentities

	mp := c.wallet.Mixpool()
	sesRun = &ps.runs[len(ps.runs)-1]
	prs := sesRun.prs

	d := &ps.deadlines
	unixEpoch := uint64(ps.epoch.Unix())

	// A map of identity public keys to their PR position sort all
	// messages in the same order as the PRs are ordered.
	identityIndices := make(map[identity]int)
	for i, pr := range prs {
		identityIndices[pr.Identity] = i
	}

	freshGen := sesRun.freshGen
	err = c.forLocalPeers(ctx, sesRun, func(p *peer) error {
		if freshGen {
			p.ke = nil

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
				return errors.New("gen returned wrong message count")
			}
			for _, m := range p.dcMsg {
				if len(m) != msize {
					err := fmt.Errorf("gen returned bad message "+
						"length [%v != %v]", len(m), msize)
					return err
				}
			}
		}

		if p.ke == nil {
			seenPRs := make([]chainhash.Hash, len(prs))
			for i := range prs {
				seenPRs[i] = prs[i].Hash()
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
			p.ct = nil
			p.sr = nil
			p.fp = nil
			p.dc = nil
			p.cm = nil
			p.rs = rs
		}
		return nil
	})
	if err != nil {
		return sesRun, err
	}
	sesRun.freshGen = false
	err = c.sendLocalPeerMsgs(ctx, ps.deadlines.recvKE, sesRun, msgKE)
	if err != nil {
		sesRun.logf("%v", err)
	}

	// Only continue attempting to form the session if there are minimum
	// peers available and at least one of them is capable of solving the
	// roots.
	if len(prs) < MinPeers {
		sesRun.logf("pairing %x: minimum peer requirement unmet", ps.pairing)
		return sesRun, errOnlyKEsBroadcasted
	}
	haveSolver := false
	for _, pr := range prs {
		if pr.Flags&mixing.PRFlagCanSolveRoots != 0 {
			haveSolver = true
			break
		}
	}
	if !haveSolver {
		sesRun.logf("pairing %x: no solver available", ps.pairing)
		return sesRun, errOnlyKEsBroadcasted
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
	completedSesRun, err := c.completePairing(ctx, ps)
	if err != nil {
		// Alternate session may need to be attempted.  Do not form an
		// alternate session if we are about to enter into the next
		// epoch.  The session forming will be performed by a new
		// goroutine started by the epoch ticker, possibly with
		// additional PRs.
		nextEpoch := ps.epoch.Add(c.epoch)
		if time.Now().Add(timeoutDuration).After(nextEpoch) {
			c.logf("Aborting session %x after %d attempts",
				sesRun.sid[:], len(ps.runs))
			return sesRun, errOnlyKEsBroadcasted
		}

		// If peer agreement was never established, alternate sessions
		// based on the seen PRs must be formed.
		if !ps.peerAgreement {
			return sesRun, c.alternateSession(ps, sesRun.prs)
		}

		return sesRun, err
	}

	if completedSesRun != sesRun {
		completedSesRun.logf("replacing previous session attempt %x/%d",
			sesRun.sid[:], sesRun.idx)

		// Reset variables created from assuming the
		// final session run.
		sesRun = completedSesRun
		prs = sesRun.prs
		identityIndices = make(map[identity]int)
		for i, pr := range prs {
			identityIndices[pr.Identity] = i
		}
	}
	kes := sesRun.kes
	sesRun.logf("received all %d KEs", len(kes))

	// The coinjoin structure is commonly referenced by all instances of
	// the local peers; reset all of them from the initial paired session
	// attempt, and not only those in the last session run.
	for _, p := range ps.localPeers {
		p.coinjoin.resetUnmixed(prs)
	}

	// Before confirming the pairing, check all of the agreed-upon PRs
	// that they will not result in a coinjoin transaction that exceeds
	// the standard size limits.
	//
	// PRs are randomly ordered in each epoch based on the session ID, so
	// they can be iterated in order to discover any PR that would
	// increase the final coinjoin size above the limits.
	if !ps.peerAgreement {
		ps.peerAgreement = true
		ps.peerAgreementRunIdx = sesRun.idx

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
			return sesRun, &sizeLimited{
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

	// Remove paired local peers from pending pairings.
	//
	// XXX might want to keep these instead of racing to add them back if
	// this mix doesn't run to completion, and we start next epoch without
	// some of our own peers.
	c.mu.Lock()
	if pending := c.pendingPairings[string(ps.pairing)]; pending != nil {
		for id := range sesRun.localPeers {
			delete(pending.localPeers, id)
		}
		if len(pending.localPeers) == 0 {
			delete(c.pendingPairings, string(ps.pairing))
		}
	}
	c.mu.Unlock()

	c.donePairing(ps)

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
		if sesRun.localPeers[ke.Identity] != nil {
			involvesLocalPeers = true
			break
		}
	}
	if !involvesLocalPeers {
		return sesRun, errors.New("excluded all local peers")
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
		sesRun.logf("blaming %x during run (invalid ECDH pubkeys)", []identity(blamed))
		return sesRun, blamed
	}

	err = c.forLocalPeers(ctx, sesRun, func(p *peer) error {
		if p.ct != nil {
			return nil
		}

		// Create shared keys and ciphextexts for each peer
		pqct, err := p.kx.Encapsulate(p.prng, pqpk, int(p.myVk))
		if err != nil {
			return err
		}

		// Send ciphertext messages
		ct := wire.NewMsgMixCiphertexts(*p.id, sesRun.sid, 0, pqct, seenKEs)
		p.ct = ct
		c.testHook(hookBeforePeerCTPublish, ps, sesRun, p)
		return nil
	})
	if err != nil {
		sesRun.logf("%v", err)
	}
	err = c.sendLocalPeerMsgs(ctx, d.recvCT, sesRun, msgCT)
	if err != nil {
		sesRun.logf("%v", err)
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
		return sesRun, err
	}
	if len(cts) != len(prs) {
		// Blame peers
		sesRun.logf("received %d CTs for %d peers; rerunning", len(cts), len(prs))
		return sesRun, blameTimedOut(sesRun, ctTimeout)
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
		if p.sr != nil {
			return nil
		}

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
		c.testHook(hookBeforePeerSRPublish, ps, sesRun, p)
		return nil
	})
	if len(blamedMap) > 0 {
		for id := range blamedMap {
			blamed = append(blamed, id)
		}
		sesRun.logf("blaming %x during run (wrong ciphertext count)", []identity(blamed))
		return sesRun, blamed
	}
	sendErr := c.sendLocalPeerMsgs(ctx, d.recvSR, sesRun, msgSR)
	if sendErr != nil {
		sesRun.logf("%v", sendErr)
	}
	if err != nil {
		sesRun.logf("%v", err)
		return sesRun, err
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
		return sesRun, err
	}
	if len(srs) != len(prs) {
		// Blame peers
		sesRun.logf("received %d SRs for %d peers; rerunning", len(srs), len(prs))
		return sesRun, blameTimedOut(sesRun, srTimeout)
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
	roots, err := c.roots(rcvCtx, seenSRs, sesRun, coeffs, mixing.F)
	rcvCtxCancel()
	if err != nil {
		return sesRun, err
	}
	sesRun.roots = roots

	err = c.forLocalPeers(ctx, sesRun, func(p *peer) error {
		if p.dc != nil {
			return nil
		}

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
		c.testHook(hookBeforePeerDCPublish, ps, sesRun, p)
		return nil
	})
	sendErr = c.sendLocalPeerMsgs(ctx, d.recvDC, sesRun, msgFP|msgDC)
	if sendErr != nil {
		sesRun.logf("%v", err)
	}
	if err != nil {
		sesRun.logf("DC-net error: %v", err)
		return sesRun, err
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
		return sesRun, err
	}
	if len(dcs) != len(prs) {
		// Blame peers
		sesRun.logf("received %d DCs for %d peers; rerunning", len(dcs), len(prs))
		return sesRun, blameTimedOut(sesRun, dcTimeout)
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
		sesRun.logf("blaming %x during run (wrong DC-net count)", []identity(blamed))
		return sesRun, blamed
	}
	mixedMsgs := mixing.XorVectors(dcVecs)
	err = c.forLocalPeers(ctx, sesRun, func(p *peer) error {
		if p.cm != nil {
			return nil
		}

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
			sesRun.logf("missing message; blaming and rerunning")
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
	sendErr = c.sendLocalPeerMsgs(ctx, d.recvCM, sesRun, msgCM)
	if sendErr != nil {
		sesRun.logf("%v", sendErr)
	}
	if err != nil {
		sesRun.logf("confirm error: %v", err)
		return sesRun, err
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
		return sesRun, err
	}
	if len(cms) != len(prs) {
		// Blame peers
		sesRun.logf("received %d CMs for %d peers; rerunning", len(cms), len(prs))
		return sesRun, blameTimedOut(sesRun, cmTimeout)
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
		sesRun.logf("blaming %x during run (confirmed wrong coinjoin)", []identity(blamed))
		return sesRun, blamed
	}
	err = c.validateMergedCoinjoin(cj, prs, utxos)
	if err != nil {
		return sesRun, err
	}

	time.Sleep(lowestJitter + rand.Duration(msgJitter))
	err = c.wallet.PublishTransaction(context.Background(), cj.tx)
	if err != nil {
		return sesRun, err
	}

	c.forLocalPeers(ctx, sesRun, func(p *peer) error {
		select {
		case p.res <- nil:
		default:
		}
		return nil
	})

	sesRun.cj = cj

	return sesRun, nil
}

func (c *Client) solvesRoots() bool {
	prFlags := byte(atomic.LoadUint32(&c.atomicPRFlags))
	return prFlags&mixing.PRFlagCanSolveRoots != 0
}

// roots returns the solutions to the slot reservation polynomial.  If the
// client is capable of solving the roots directly, it does so and publishes
// the result to all peers.  If the client is incapable of solving the roots,
// it waits for a solution.
func (c *Client) roots(ctx context.Context, seenSRs []chainhash.Hash,
	sesRun *sessionRun, a []*big.Int, F *big.Int) ([]*big.Int, error) {

	switch {
	case c.solvesRoots():
		roots, err := solverrpc.Roots(a, F)
		if errors.Is(err, solverrpc.ErrSolverProcessExited) {
			// Unset root-solving capability and future advertisement
			prFlags := byte(atomic.LoadUint32(&c.atomicPRFlags))
			prFlags &^= mixing.PRFlagCanSolveRoots
			atomic.StoreUint32(&c.atomicPRFlags, uint32(prFlags))

			// Wait for other peers to publish roots as a fallback
			// in this run.  If we are the only solver, we will be
			// blamed for not publishing any.
			break
		}
		if err != nil || len(roots) != len(a)-1 {
			c.forLocalPeers(ctx, sesRun, func(p *peer) error {
				p.triggeredBlame = true
				return nil
			})
			return nil, errTriggeredBlame
		}
		sort.Slice(roots, func(i, j int) bool {
			return roots[i].Cmp(roots[j]) == -1
		})
		rootBytes := make([][]byte, len(roots))
		for i, root := range roots {
			rootBytes[i] = root.Bytes()
		}
		c.forLocalPeers(ctx, sesRun, func(p *peer) error {
			if p.fp != nil {
				return nil
			}
			p.fp = wire.NewMsgMixFactoredPoly(*p.id, sesRun.sid,
				0, rootBytes, seenSRs)
			return nil
		})
		return roots, nil
	}

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
		rcvCtx, rcvCtxCancel := context.WithDeadline(ctx, time.Now().Add(timeoutDuration))
		err := c.mixpool.Receive(rcvCtx, rcv)
		rcvCtxCancel()
		if err != nil {
			return nil, err
		}

	FPs:
		for _, fp := range rcv.FPs {
			if _, ok := checkedFPByIdentity[fp.Identity]; ok {
				continue
			}
			checkedFPByIdentity[fp.Identity] = struct{}{}

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
			sorted := sort.SliceIsSorted(roots, func(i, j int) bool {
				return roots[i].Cmp(roots[j]) == -1
			})
			if !sorted {
				continue
			}
			if len(roots) == len(a)-1 {
				return roots, nil
			}
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

func (c *Client) alternateSession(ps *pairedSessions, prs []*wire.MsgMixPairReq) *alternateSession {
	unixEpoch := uint64(ps.epoch.Unix())

	kes := c.mixpool.ReceiveKEsByPairing(ps.pairing, unixEpoch)

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
			err := fmt.Errorf("missing PR %s by %x, but have their KE %s",
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

func excludeBlamed(prevRun *sessionRun, epoch uint64, blamed blamedIdentities, revealedSecrets bool) *sessionRun {
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

	sid := mixing.SortPRsForSession(prs, epoch)

	// mtot, peers, mcounts are all recalculated from the prs before
	// calling run()
	nextRun := &sessionRun{
		sid:      sid,
		idx:      prevRun.idx + 1,
		freshGen: revealedSecrets,
		prs:      prs,
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
