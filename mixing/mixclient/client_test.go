// Copyright (c) 2024-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixclient

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"decred.org/cspp/v2/solverrpc"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/mixing"
	"github.com/decred/dcrd/mixing/mixpool"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/sign"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
	"github.com/decred/slog"
	"golang.org/x/sync/errgroup"
)

func requireCsppsolver(t *testing.T) {
	if err := solverrpc.StartSolver(); err != nil {
		t.Skipf("test requires csppsolver, but it cannot be started: %v", err)
	}
}

var (
	testStartingHeight uint32 = 100
	testStartingBlock         = chainhash.Hash{100}
)

const (
	inputValue = 1 << 22
	mixValue   = 1 << 20
)

func newTestClient(w *testWallet, logger slog.Logger) *Client {
	c := NewClient(w)
	c.testTickC = make(chan struct{})
	c.SetLogger(logger)
	return c
}

var testnetParams = chaincfg.TestNet3Params()

type testBlockchain struct {
	publishedTxs map[chainhash.Hash]*wire.MsgTx
	mu           sync.Mutex
}

func newTestBlockchain() *testBlockchain {
	return &testBlockchain{
		publishedTxs: make(map[chainhash.Hash]*wire.MsgTx),
	}
}

func (b *testBlockchain) CurrentTip() (chainhash.Hash, int64) {
	return testStartingBlock, int64(testStartingHeight)
}

func (b *testBlockchain) ChainParams() *chaincfg.Params {
	return testnetParams
}

type testWallet struct {
	blockchain *testBlockchain
	mixpool    *mixpool.Pool
	keys       map[[20]byte]*secp256k1.PrivateKey
	mu         sync.Mutex
}

func newTestWallet(blockchain *testBlockchain) *testWallet {
	mp := mixpool.NewPool(blockchain)
	return &testWallet{
		blockchain: blockchain,
		mixpool:    mp,
		keys:       make(map[[20]byte]*secp256k1.PrivateKey),
	}
}

func (w *testWallet) BestBlock() (uint32, chainhash.Hash) {
	return testStartingHeight, testStartingBlock
}

func (w *testWallet) Mixpool() *mixpool.Pool {
	return w.mixpool
}

func (w *testWallet) PublishTransaction(ctx context.Context, tx *wire.MsgTx) error {
	bc := w.blockchain
	bc.mu.Lock()
	bc.publishedTxs[tx.TxHash()] = tx
	bc.mu.Unlock()
	return nil
}

func (w *testWallet) SignInput(tx *wire.MsgTx, index int, prevScript []byte) error {
	sigScript, err := sign.SignatureScript(tx, index, prevScript, txscript.SigHashAll,
		w.privForPkScript(prevScript).Serialize(), dcrec.STEcdsaSecp256k1, true)
	if err != nil {
		return err
	}
	tx.TxIn[index].SignatureScript = sigScript
	return nil
}

func (w *testWallet) SubmitMixMessage(ctx context.Context, msg mixing.Message) error {
	_, err := w.mixpool.AcceptMessage(msg, mixpool.ZeroSource)
	return err
}

func (w *testWallet) gen(mcount uint32) (wire.MixVect, error) {
	v := make(wire.MixVect, mcount)
	for i := range v {
		pub, priv, err := generateSecp256k1()
		if err != nil {
			return nil, err
		}
		serializedPub := pub.SerializeCompressed()
		hash160 := *(*[20]byte)(stdaddr.Hash160(serializedPub))

		w.mu.Lock()
		w.keys[hash160] = priv
		w.mu.Unlock()

		copy(v[i][:], hash160[:])
	}
	return v, nil
}

func (w *testWallet) outputScript() ([]byte, error) {
	pub, priv, err := generateSecp256k1()
	if err != nil {
		return nil, err
	}

	serializedPub := pub.SerializeCompressed()
	hash160 := *(*[20]byte)(stdaddr.Hash160(serializedPub))
	pkScript := []byte{
		0:  txscript.OP_DUP,
		1:  txscript.OP_HASH160,
		2:  txscript.OP_DATA_20,
		23: txscript.OP_EQUALVERIFY,
		24: txscript.OP_CHECKSIG,
	}
	copy(pkScript[3:23], hash160[:])

	w.mu.Lock()
	w.keys[hash160] = priv
	w.mu.Unlock()

	return pkScript, nil
}

func (w *testWallet) privForHash160(hash160 [20]byte) *secp256k1.PrivateKey {
	w.mu.Lock()
	priv := w.keys[hash160]
	w.mu.Unlock()
	return priv
}

func (w *testWallet) privForPkScript(p2pkhScript []byte) *secp256k1.PrivateKey {
	hash160 := *(*[20]byte)(p2pkhScript[3:23])
	return w.privForHash160(hash160)
}

func TestHonest(t *testing.T) {
	requireCsppsolver(t)

	l, done := useTestLogger(t)
	t.Cleanup(done)

	bc := newTestBlockchain()
	w := newTestWallet(bc)
	c := newTestClient(w, l)

	type testPeer struct {
		cj *CoinJoin
	}
	peers := []*testPeer{
		{cj: NewCoinJoin(w.gen, nil, mixValue, testStartingHeight+10, 1)},
		{cj: NewCoinJoin(w.gen, nil, mixValue, testStartingHeight+10, 2)},
		{cj: NewCoinJoin(w.gen, nil, mixValue, testStartingHeight+10, 3)},
		{cj: NewCoinJoin(w.gen, nil, mixValue, testStartingHeight+10, 4)},
	}
	inputTx := wire.NewMsgTx()
	for range peers {
		script, err := w.outputScript()
		if err != nil {
			t.Fatal(err)
		}
		inputTx.AddTxOut(wire.NewTxOut(inputValue*4, script))
	}
	inputTxOutpoint := wire.OutPoint{Hash: inputTx.TxHash()}
	for i := range peers {
		p := peers[i]

		inputTxOutpoint.Index = uint32(i)
		input := wire.NewTxIn(&inputTxOutpoint, inputTx.TxOut[i].Value, nil)

		pkScript := inputTx.TxOut[i].PkScript

		p.cj.AddInput(input, pkScript, 0, w.privForPkScript(pkScript))
	}

	ctx, cancel := context.WithCancel(context.Background())
	doneRun := make(chan struct{})
	go func() {
		c.Run(ctx)
		doneRun <- struct{}{}
	}()
	defer func() {
		cancel()
		<-doneRun
	}()

	c.testTick()

	var g errgroup.Group
	for i := range peers {
		p := peers[i]
		g.Go(func() error {
			return c.Dicemix(ctx, p.cj)
		})
	}

	go func() {
		for {
			c.testTick()
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	}()

	err := g.Wait()
	if err != nil {
		t.Error(err)
	}

	for _, tx := range bc.publishedTxs {
		buf := new(bytes.Buffer)
		tx.Serialize(buf)
		t.Logf("published transaction with %d mixed outputs %x",
			len(tx.TxOut), buf.Bytes())
	}
}

func testDisruption(t *testing.T, misbehavingID *identity, h hook, f hookFunc) {
	requireCsppsolver(t)

	l, done := useTestLogger(t)
	t.Cleanup(done)

	bc := newTestBlockchain()
	w := newTestWallet(bc)
	c := newTestClient(w, l)
	c.testHooks = map[hook]hookFunc{
		hookBeforeRun: func(c *Client, ps *pairedSessions, _ *sessionRun, _ *peer) {
			ps.deadlines.recvKE = time.Now().Add(5 * time.Second)
		},
		h: f,
	}
	c2 := newTestClient(w, l)
	c2.testHooks = c.testHooks

	type testPeer struct {
		cj *CoinJoin
	}
	peers := []*testPeer{
		{cj: NewCoinJoin(w.gen, nil, mixValue, testStartingHeight+10, 1)},
		{cj: NewCoinJoin(w.gen, nil, mixValue, testStartingHeight+10, 1)},
		{cj: NewCoinJoin(w.gen, nil, mixValue, testStartingHeight+10, 1)},
		{cj: NewCoinJoin(w.gen, nil, mixValue, testStartingHeight+10, 1)},
		{cj: NewCoinJoin(w.gen, nil, mixValue, testStartingHeight+10, 1)},
		{cj: NewCoinJoin(w.gen, nil, mixValue, testStartingHeight+10, 1)},
		{cj: NewCoinJoin(w.gen, nil, mixValue, testStartingHeight+10, 1)},
		{cj: NewCoinJoin(w.gen, nil, mixValue, testStartingHeight+10, 1)},
		{cj: NewCoinJoin(w.gen, nil, mixValue, testStartingHeight+10, 1)},
		{cj: NewCoinJoin(w.gen, nil, mixValue, testStartingHeight+10, 1)},
	}
	inputTx := wire.NewMsgTx()
	for range peers {
		script, err := w.outputScript()
		if err != nil {
			t.Fatal(err)
		}
		inputTx.AddTxOut(wire.NewTxOut(inputValue, script))
	}
	inputTxOutpoint := wire.OutPoint{Hash: inputTx.TxHash()}
	for i := range peers {
		p := peers[i]

		inputTxOutpoint.Index = uint32(i)
		input := wire.NewTxIn(&inputTxOutpoint, inputTx.TxOut[i].Value, nil)

		pkScript := inputTx.TxOut[i].PkScript

		p.cj.AddInput(input, pkScript, 0, w.privForPkScript(pkScript))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneRun := make(chan struct{})
	go func() {
		c.Run(ctx)
		doneRun <- struct{}{}
	}()
	go func() {
		c2.Run(ctx)
		doneRun <- struct{}{}
	}()
	defer func() {
		cancel()
		<-doneRun
		<-doneRun
	}()

	testTick := func() {
		c.testTick()
		c2.testTick()
	}
	testTick()

	blameErrC := make(chan *testPeerBlamedError, 1)

	var g errgroup.Group
	for _, p := range peers[:len(peers)/2] {
		p := p
		g.Go(func() error {
			err := c.Dicemix(ctx, p.cj)
			var e *testPeerBlamedError
			if errors.As(err, &e) {
				blameErrC <- e
				close(blameErrC)
				return nil
			}
			return err
		})
	}
	for _, p := range peers[len(peers)/2:] {
		p := p
		g.Go(func() error {
			err := c2.Dicemix(ctx, p.cj)
			var e *testPeerBlamedError
			if errors.As(err, &e) {
				blameErrC <- e
				close(blameErrC)
				return nil
			}
			return err
		})
	}

	go func() {
		for {
			testTick()
			select {
			case <-ctx.Done():
				return
			case <-time.After(1000 * time.Millisecond):
			}
		}
	}()

	err := g.Wait()
	if err != nil {
		t.Error(err)
	}

	blamedPeer := <-blameErrC
	blamedID := *blamedPeer.p.id
	t.Logf("blamed peer %x removed from mix", blamedID[:])
	if blamedID != *misbehavingID {
		t.Errorf("blamed incorrect peer, expected to blame %x", misbehavingID[:])
	}

	for _, tx := range bc.publishedTxs {
		buf := new(bytes.Buffer)
		tx.Serialize(buf)
		t.Logf("published transaction with %d mixed outputs: %x",
			len(tx.TxOut), buf.Bytes())
	}
}

func TestCTDisruption(t *testing.T) {
	var misbehavingID identity
	testDisruption(t, &misbehavingID, hookBeforePeerCTPublish,
		func(c *Client, ps *pairedSessions, s *sessionRun, p *peer) {
			if p.myVk != 0 {
				return
			}
			if misbehavingID != [33]byte{} {
				return
			}
			t.Logf("malicious peer %x: flipping CT bit", p.id[:])
			misbehavingID = *p.id
			p.ct.Ciphertexts[1][0] ^= 1
		})
}

func TestCTLength(t *testing.T) {
	var misbehavingID identity
	testDisruption(t, &misbehavingID, hookBeforePeerCTPublish,
		func(c *Client, ps *pairedSessions, s *sessionRun, p *peer) {
			if p.myVk != 0 {
				return
			}
			if misbehavingID != [33]byte{} {
				return
			}
			t.Logf("malicious peer %x: sending too few ciphertexts", p.id[:])
			misbehavingID = *p.id
			p.ct.Ciphertexts = p.ct.Ciphertexts[:len(p.ct.Ciphertexts)-1]
		})

	misbehavingID = identity{}
	testDisruption(t, &misbehavingID, hookBeforePeerCTPublish,
		func(c *Client, ps *pairedSessions, s *sessionRun, p *peer) {
			if p.myVk != 0 {
				return
			}
			if misbehavingID != [33]byte{} {
				return
			}
			t.Logf("malicious peer %x: sending too many ciphertexts", p.id[:])
			misbehavingID = *p.id
			p.ct.Ciphertexts = append(p.ct.Ciphertexts, p.ct.Ciphertexts[0])
		})
}

func TestSRDisruption(t *testing.T) {
	var misbehavingID identity
	testDisruption(t, &misbehavingID, hookBeforePeerSRPublish,
		func(c *Client, ps *pairedSessions, s *sessionRun, p *peer) {
			if p.myVk != 0 {
				return
			}
			if misbehavingID != [33]byte{} {
				return
			}
			t.Logf("malicious peer %x: flipping SR bit", p.id[:])
			misbehavingID = *p.id
			p.sr.DCMix[0][1][0] ^= 1
		})
}

func TestDCDisruption(t *testing.T) {
	var misbehavingID identity
	testDisruption(t, &misbehavingID, hookBeforePeerDCPublish,
		func(c *Client, ps *pairedSessions, s *sessionRun, p *peer) {
			if p.myVk != 0 {
				return
			}
			if misbehavingID != [33]byte{} {
				return
			}
			t.Logf("malicious peer %x: flipping DC bit", p.id[:])
			misbehavingID = *p.id
			p.dc.DCNet[0][1][0] ^= 1
		})
}
