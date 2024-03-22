// Copyright (c) 2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package mixpool provides an in-memory pool of mixing messages for full nodes
// that relay these messages and mixing wallets that send and receive them.
package mixpool

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"sort"
	"sync"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/mixing"
	"github.com/decred/dcrd/mixing/utxoproof"
	"github.com/decred/dcrd/txscript/v4/stdscript"
	"github.com/decred/dcrd/wire"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

const minconf = 2
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
	msgtypeRS

	nmsgtypes = msgtypeRS
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
	case msgtypeRS:
		return "RS"
	default:
		return "?"
	}
}

// entry describes non-PR messages accepted to the pool.
type entry struct {
	hash    chainhash.Hash
	sid     [32]byte
	msg     mixing.Message
	msgtype msgtype
	run     uint32
}

type session struct {
	sid    [32]byte
	runs   []runstate
	expiry uint32
	bc     broadcast
}

type runstate struct {
	run    uint32
	npeers uint32
	counts [nmsgtypes]uint32
	hashes map[chainhash.Hash]struct{}
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
	messagesByIdentity map[idPubKey][]chainhash.Hash
	sessions           map[[32]byte]*session

	blockchain  BlockChain
	utxoFetcher UtxoFetcher
	feeRate     int64
	params      *chaincfg.Params
}

// UtxoEntry provides details regarding unspent transaction outputs.
type UtxoEntry interface {
	IsSpent() bool
	PkScript() []byte
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

	// BestHeader returns the hash and height of the current tip block.
	BestHeader() (chainhash.Hash, int64)
}

// NewPool returns a new mixing pool that accepts and validates mixing messages
// required for distributed transaction mixing.
func NewPool(blockchain BlockChain) *Pool {
	pool := &Pool{
		prs:                make(map[chainhash.Hash]*wire.MsgMixPairReq),
		outPoints:          make(map[wire.OutPoint]chainhash.Hash),
		pool:               make(map[chainhash.Hash]entry),
		messagesByIdentity: make(map[idPubKey][]chainhash.Hash),
		sessions:           make(map[[32]byte]*session),
		blockchain:         blockchain,
		feeRate:            feeRate,
		params:             blockchain.ChainParams(),
	}
	if u, ok := blockchain.(UtxoFetcher); ok {
		pool.utxoFetcher = u
	}
	return pool
}

// MixPRHashes returns the hashes of all MixPR messages recorded by the pool.
// This data is provided to peers requesting inital state of the mixpool.
func (p *Pool) MixPRHashes() []chainhash.Hash {
	p.mtx.RLock()
	hashes := make([]chainhash.Hash, 0, len(p.prs))
	for hash := range p.prs {
		hashes = append(hashes, hash)
	}
	p.mtx.RUnlock()

	return hashes
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
		return nil, fmt.Errorf("message not found")
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

// MixPR searches the mixing pool for a PR message by its hash.
func (p *Pool) MixPR(query *chainhash.Hash) (*wire.MsgMixPairReq, error) {
	var pr *wire.MsgMixPairReq

	p.mtx.RLock()
	e, ok := p.pool[*query]
	p.mtx.RUnlock()
	if ok {
		pr, _ = e.msg.(*wire.MsgMixPairReq)
	}

	if pr == nil {
		return nil, fmt.Errorf("PR message not found")
	}

	return pr, nil
}

// MixPRs returns all MixPR messages with hashes matching the query.  Unknown
// messages are ignored.
//
// If query is nil, all PRs are returned.
func (p *Pool) MixPRs(query []chainhash.Hash) []*wire.MsgMixPairReq {
	res := make([]*wire.MsgMixPairReq, 0, len(query))

	p.mtx.RLock()
	defer p.mtx.RUnlock()

	if query == nil {
		res = make([]*wire.MsgMixPairReq, 0, len(p.prs))
		for _, pr := range p.prs {
			res = append(res, pr)
		}
		return res
	}

	for i := range query {
		e, ok := p.pool[query[i]]
		if !ok {
			continue
		}

		pr, ok := e.msg.(*wire.MsgMixPairReq)
		if ok {
			res = append(res, pr)
		}
	}

	return res
}

// CompatiblePRs returns all MixPR messages with pairing descriptions matching
// the parameter.
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

// ExpireMessages removes all pair requests that indicate an expiry at or
// before the height parameter and removes all messages that chain back to a
// removed pair request.
func (p *Pool) ExpireMessages(height uint32) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	// Expire sessions and their messages
	for sid, ses := range p.sessions {
		if ses.expiry > height {
			continue
		}

		delete(p.sessions, sid)
		for _, r := range ses.runs {
			for hash := range r.hashes {
				delete(p.pool, hash)
			}
		}
	}

	// Expire PRs and remove identity tracking
	for hash, pr := range p.prs {
		if pr.Expiry > height {
			continue
		}

		log.Printf("mixpool removing PR %s", hash)
		delete(p.prs, hash)
		delete(p.messagesByIdentity, pr.Identity)
	}
}

// RemoveMessage removes a message that was rejected by the network.
func (p *Pool) RemoveMessage(msg mixing.Message) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	msgHash := msg.Hash()
	delete(p.pool, msgHash)
	_, ok := msg.(*wire.MsgMixPairReq)
	if ok {
		if p.prs[msgHash] != nil {
			log.Printf("mixpool removing PR %s", msgHash)
		}
		delete(p.prs, msgHash)
	}
}

// RemoveIdentities removes all messages from the mixpool that were created by
// any of the identities.
func (p *Pool) RemoveIdentities(identities [][33]byte) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for i := range identities {
		id := &identities[i]
		for _, hash := range p.messagesByIdentity[*id] {
			delete(p.pool, hash)
			if p.prs[hash] != nil {
				log.Printf("mixpool removing PR %s", hash)
			}
			delete(p.prs, hash)
		}
		delete(p.messagesByIdentity, *id)
	}
}

// RemoveSession removes all non-PR messages from a completed or errored
// session.  PR messages of a successful run (or rerun) must also be removed.
func (p *Pool) RemoveSession(sid [32]byte, removePRs []*wire.MsgMixPairReq) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	ses := p.sessions[sid]
	if ses == nil {
		return
	}

	delete(p.sessions, sid)
	for _, r := range ses.runs {
		for hash := range r.hashes {
			delete(p.pool, hash)
		}
	}

	for _, pr := range removePRs {
		hash := pr.Hash()
		delete(p.pool, hash)
		if p.prs[hash] != nil {
			log.Printf("mixpool removing PR %s", hash)
		}
		delete(p.prs, hash)
	}
}

// RemoveRun removes all messages from a failed run in a mix session.
func (p *Pool) RemoveRun(sid [32]byte, run uint32) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	ses := p.sessions[sid]
	if ses == nil {
		return
	}

	if run >= uint32(len(ses.runs)) {
		return
	}

	rs := &ses.runs[run]
	for msghash := range rs.hashes {
		delete(p.pool, msghash)
	}
}

// ReceiveKEsByPairing returns all run-0 KE messages in the pool that
// reference PRs of a particular pairing.
func (p *Pool) ReceiveKEsByPairing(pairing []byte) []*wire.MsgMixKeyExchange {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	var kes []*wire.MsgMixKeyExchange
Entries:
	for _, e := range p.pool {
		ke, ok := e.msg.(*wire.MsgMixKeyExchange)
		if !ok {
			continue
		}
		if ke.Run != 0 {
			continue
		}
		for _, prHash := range ke.SeenPRs {
			pr := p.prs[prHash]
			if pr == nil {
				continue Entries
			}
			prPairing, err := pr.Pairing()
			if err != nil {
				continue Entries
			}
			if bytes.Equal(pairing, prPairing) {
				kes = append(kes, ke)
				continue Entries
			}
		}
	}

	return kes
}

// Received is a parameter for Pool.Receive describing the session and run to
// receive messages for, and slices for returning results.  Only non-nil slices
// will be appended to.  Received messages are unsorted.
type Received struct {
	Sid [32]byte
	Run uint32
	KEs []*wire.MsgMixKeyExchange
	CTs []*wire.MsgMixCiphertexts
	SRs []*wire.MsgMixSlotReserve
	DCs []*wire.MsgMixDCNet
	CMs []*wire.MsgMixConfirm
	RSs []*wire.MsgMixSecrets
}

// Receive returns messages matching a session, run, and message type, waiting
// until all described messages have been received, or earlier with the
// messages received so far if the context is cancelled before this point.
func (p *Pool) Receive(ctx context.Context, expectedMessages int, r *Received) error {
	sid := r.Sid
	run := r.Run
	var bc *broadcast
	var rs *runstate
	var err error

	p.mtx.RLock()
	ses, ok := p.sessions[sid]
	if !ok {
		p.mtx.RUnlock()
		return fmt.Errorf("unknown session")
	}
	bc = &ses.bc
	if run >= uint32(len(ses.runs)) {
		p.mtx.RUnlock()
		return fmt.Errorf("unknown run")
	}
	rs = &ses.runs[run]

Loop:
	for {
		// Pool is locked.  Count if the total number of expected
		// messages have been received.
		received := 0
		for hash := range rs.hashes {
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
			case msgtype == msgtypeRS && r.RSs != nil:
				received++
			}
		}
		if received >= expectedMessages {
			break
		}

		// Unlock while waiting for the broadcast channel.
		p.mtx.RUnlock()

		select {
		case <-ctx.Done():
			// Set error to be returned, but still lock the pool
			// and collect received messages.
			err = ctx.Err()
			p.mtx.RLock()
			break Loop
		case <-bc.wait():
		}

		p.mtx.RLock()
	}

	// Pool is locked.  Collect all of the messages.
	for hash := range rs.hashes {
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
		case *wire.MsgMixSecrets:
			if r.RSs != nil {
				r.RSs = append(r.RSs, msg)
			}
		}
	}

	p.mtx.RUnlock()
	return err
}

// AcceptMessage accepts a mixing message to the pool.
//
// Messages must contain the mixing participant's identity and contain a valid
// signature committing to all non-signature fields.
//
// PR messages will not be accepted if they reference an unknown UTXO or if not
// enough fee is contributed.  Any other message will not be accepted if it
// references previous messages that are not recorded by the pool.
func (p *Pool) AcceptMessage(msg mixing.Message) (accepted mixing.Message, err error) {
	// Check if already accepted.
	hash := msg.Hash()
	p.mtx.RLock()
	_, ok := p.pool[hash]
	p.mtx.RUnlock()
	if ok {
		return nil, nil
	}

	// Require message to be signed by the presented identity.
	if !mixing.VerifyMessageSignature(msg) {
		return nil, fmt.Errorf("invalid message signature")
	}
	id := (*idPubKey)(msg.Pub())

	var msgtype msgtype
	switch msg := msg.(type) {
	case *wire.MsgMixPairReq:
		accepted, err := p.acceptPR(msg, &hash, id)
		if err != nil {
			return nil, err
		}
		// Avoid returning a non-nil mixing.Message in return
		// variable with a nil PR.
		if accepted == nil {
			return nil, nil
		}
		return accepted, nil

	case *wire.MsgMixKeyExchange:
		accepted, err := p.acceptKE(msg, &hash, id)
		if err != nil {
			return nil, err
		}
		// Avoid returning a non-nil mixing.Message in return
		// variable with a nil KE.
		if accepted == nil {
			return nil, nil
		}
		return accepted, nil

	case *wire.MsgMixCiphertexts:
		msgtype = msgtypeCT
	case *wire.MsgMixSlotReserve:
		msgtype = msgtypeSR
	case *wire.MsgMixDCNet:
		msgtype = msgtypeDC
	case *wire.MsgMixConfirm:
		msgtype = msgtypeCM
	case *wire.MsgMixSecrets:
		msgtype = msgtypeRS
	default:
		return nil, fmt.Errorf("unknown mix message type %T", msg)
	}

	sid := *(*[32]byte)(msg.Sid())

	p.mtx.Lock()
	defer p.mtx.Unlock()

	// Check if already accepted.
	if _, ok := p.pool[hash]; ok {
		return nil, nil
	}

	// Check prior message existence in the pool, and only accept messages
	// that reference other known and accepted messages of the correct type
	// and sid.
	//
	// TODO: Consider return an error containing the unknown messages, so
	// they can be getdata'd, and if they are not received or are garbage,
	// peers can be kicked.
	prevMsgs := msg.PrevMsgs()
	for i := range prevMsgs {
		looktype := msgtype - 1
		if msgtype == msgtypeRS {
			looktype = 0
		}
		_, ok := p.lookupEntry(prevMsgs[i], looktype, &sid)
		if !ok {
			return nil, fmt.Errorf("%s %s references unknown "+
				"previous message %s", msgtype, &hash,
				&prevMsgs[i])
		}
	}

	// Check that a message from this identity does not reuse a run number
	// for the session.
	for _, prevHash := range p.messagesByIdentity[*id] {
		e := p.pool[prevHash]
		if e.msgtype == msgtype && e.msg.GetRun() == msg.GetRun() &&
			bytes.Equal(e.msg.Sid(), msg.Sid()) {
			return nil, fmt.Errorf("reused run number from identity")
		}
	}

	ses := p.sessions[sid]
	if ses == nil {
		return nil, fmt.Errorf("%s %s belongs to unknown session %x",
			msgtype, &hash, &sid)
	}

	err = p.acceptEntry(msg, msgtype, &hash, id, ses)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func (p *Pool) acceptPR(pr *wire.MsgMixPairReq, hash *chainhash.Hash, id *idPubKey) (accepted *wire.MsgMixPairReq, err error) {
	// Check that expiry has not been reached, nor that it is too far
	// into the future.  This limits replay attacks.
	_, curHeight := p.blockchain.BestHeader()
	maxExpiry := mixing.MaxExpiry(uint32(curHeight), p.params)
	switch {
	case uint32(curHeight) >= pr.Expiry:
		return nil, fmt.Errorf("message has expired")
	case pr.Expiry > maxExpiry:
		return nil, fmt.Errorf("expiry is too far into future")
	}

	if len(pr.UTXOs) == 0 {
		return nil, fmt.Errorf("at least one UTXO must be submitted")
	}

	// If able, sanity check UTXOs.
	if p.utxoFetcher != nil {
		err := p.checkUTXOs(pr)
		if err != nil {
			return nil, err
		}
	}

	// Require known script classes.
	switch mixing.ScriptClass(pr.ScriptClass) {
	case mixing.ScriptClassP2PKHv0:
	default:
		return nil, fmt.Errorf("unsupported mixing script class")
	}

	// Require enough fee contributed from this mixing participant.
	// Size estimation assumes mixing.ScriptClassP2PKHv0 outputs and inputs.
	err = checkFee(pr, p.feeRate)
	if err != nil {
		return nil, err
	}

	// Require at least one mixed message.
	if pr.MessageCount == 0 {
		return nil, fmt.Errorf("message count must be positive")
	}

	p.mtx.Lock()
	defer p.mtx.Unlock()

	// Check if already accepted.
	if _, ok := p.prs[*hash]; ok {
		return nil, nil
	}

	// Discourage identity reuse.  PRs should be the first message sent by
	// this identity, and there should only be one PR per identity.
	if len(p.messagesByIdentity[*id]) != 0 {
		return nil, fmt.Errorf("identity reused for a PR message")
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
			err := fmt.Errorf("PR double spends outpoints of " +
				"already-accepted PR message without " +
				"increasing expiry")
			return nil, err
		}
	}

	// XXX: don't allow expiry too far into future

	// Accept the PR
	p.prs[*hash] = pr
	for i := range pr.UTXOs {
		p.outPoints[pr.UTXOs[i].OutPoint] = *hash
	}
	p.messagesByIdentity[*id] = append(make([]chainhash.Hash, 0, 16), *hash)

	return pr, nil
}

// Check that UTXOs exist, have confirmations, sum of UTXO values matches the
// input value, and proof of ownership is valid.
func (p *Pool) checkUTXOs(pr *wire.MsgMixPairReq) error {
	var totalValue int64
	_, curHeight := p.blockchain.BestHeader()

	for i := range pr.UTXOs {
		utxo := &pr.UTXOs[i]
		entry, err := p.utxoFetcher.FetchUtxoEntry(utxo.OutPoint)
		if err != nil {
			return err
		}
		if entry == nil || entry.IsSpent() {
			return fmt.Errorf("output %v is not unspent",
				&utxo.OutPoint)
		}
		height := entry.BlockHeight()
		if !confirmed(minconf, height, curHeight) {
			return fmt.Errorf("output %v is unconfirmed",
				&utxo.OutPoint)
		}

		// Check proof of key ownership and ability to sign coinjoin
		// inputs.
		utxoPkScript := entry.PkScript()
		var valid bool
		switch {
		case stdscript.IsPubKeyHashScriptV0(utxoPkScript):
			valid = validateOwnerProofP2PKHv0(utxoPkScript,
				utxo.PubKey, utxo.Signature, pr.Expires())
		default:
			return fmt.Errorf("unsupported UTXO output script")
		}
		if !valid {
			return fmt.Errorf("invalid UTXO ownership proof")
		}

		totalValue += entry.Amount()
	}

	if totalValue != pr.InputValue {
		return fmt.Errorf("input value does not match sum of UTXO " +
			"values")
	}

	return nil
}

// Tags prepended to signed ownership proof messages.
const (
	ownerproofP2PKHv0 = "mixpr-ownerproof-P2PKH-secp256k1-v0-"
)

func validateOwnerProofP2PKHv0(pkscript, pubkey, sig []byte, expires uint32) bool {
	extractedHash160 := stdscript.ExtractPubKeyHashV0(pkscript)
	pubkeyHash160 := dcrutil.Hash160(pubkey)
	if !bytes.Equal(extractedHash160, pubkeyHash160) {
		return false
	}

	return utxoproof.ValidateSecp256k1P2PKH(pubkey, sig, expires)
}

func (p *Pool) acceptKE(ke *wire.MsgMixKeyExchange, hash *chainhash.Hash, id *idPubKey) (accepted *wire.MsgMixKeyExchange, err error) {
	sid := ke.SessionID

	// In all runs, previous PR messages in the KE must be sorted.
	// This defines the initial unmixed peer positions.
	sorted := sort.SliceIsSorted(ke.SeenPRs, func(i, j int) bool {
		a := ke.SeenPRs[i][:]
		b := ke.SeenPRs[j][:]
		return bytes.Compare(a, b) == -1
	})
	if !sorted {
		err := fmt.Errorf("KE message contains unsorted previous PR " +
			"hashes")
		return nil, err
	}

	// Run-0 KE messages define a session ID by hashing all previously-seen
	// PR message hashes.  This must match the sid also present in the
	// message.  Later runs after a failed run may drop peers from the
	// SeenPRs set, but the sid remains the same.  A sid cannot be conjured
	// out of thin air, and other messages seen from the network for an
	// unknown session are not accepted.
	if ke.Run == 0 {
		derivedSid := mixing.DeriveSessionID(ke.SeenPRs)
		if sid != derivedSid {
			err := fmt.Errorf("invalid session ID for run-0 KE")
			return nil, err
		}
	}

	p.mtx.Lock()
	defer p.mtx.Unlock()

	// Check if already accepted.
	if _, ok := p.pool[*hash]; ok {
		return nil, nil
	}

	// Only accept messages that reference known PRs, and require that these
	// PRs are compatible with each other.
	prevMsgs := ke.PrevMsgs()
	prs := make([]*wire.MsgMixPairReq, 0, len(prevMsgs))
	var pairing []byte
	for _, prevHash := range prevMsgs {
		pr, ok := p.prs[prevHash]
		if !ok {
			continue
		}
		prs = append(prs, pr)
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
				err := fmt.Errorf("referenced PRs are incompatible")
				return nil, err
			}
		}
	}
	if pairing == nil {
		err := fmt.Errorf("KE %s references unknown PRs", hash)
		return nil, err
	}

	ses := p.sessions[sid]

	// Create a session for the first run-0 KE
	if ses == nil {
		if ke.Run != 0 {
			err := fmt.Errorf("unknown session for run-%d KE",
				ke.Run)
			return nil, err
		}

		expiry := ^uint32(0)
		for i := range prs {
			prExpiry := prs[i].Expires()
			if expiry > prExpiry {
				expiry = prExpiry
			}
		}
		ses = &session{
			sid:    sid,
			runs:   make([]runstate, 0, 4),
			expiry: expiry,
			bc:     broadcast{ch: make(chan struct{})},
		}
		p.sessions[sid] = ses
	}

	err = p.acceptEntry(ke, msgtypeKE, hash, id, ses)
	if err != nil {
		return nil, err
	}
	return ke, nil
}

func (p *Pool) acceptEntry(msg mixing.Message, msgtype msgtype, hash *chainhash.Hash,
	id *[33]byte, ses *session) error {
	// XXX: may want to remove expiry for all but PR.
	// if msgtype > msgtypeKE && msg.Expires() > ses.expiry {
	// 	return fmt.Errorf("message has inappropriate expiry")
	// }

	run := msg.GetRun()
	if msg.GetRun() > uint32(len(ses.runs)) {
		return fmt.Errorf("message skips runs")
	}

	var rs *runstate
	if msgtype == msgtypeKE && msg.GetRun() == uint32(len(ses.runs)) {
		// Add a runstate for the next run.
		ses.runs = append(ses.runs, runstate{
			run:    msg.GetRun(),
			npeers: uint32(len(msg.PrevMsgs())),
			hashes: make(map[chainhash.Hash]struct{}),
		})
		rs = &ses.runs[len(ses.runs)-1]
	} else {
		// Add to existing runstate
		rs = &ses.runs[run]
	}

	rs.hashes[*hash] = struct{}{}
	e := entry{
		hash:    *hash,
		sid:     ses.sid,
		msg:     msg,
		msgtype: msgtype,
		run:     msg.GetRun(),
	}
	p.pool[*hash] = e
	p.messagesByIdentity[*id] = append(p.messagesByIdentity[*id], *hash)

	count := &rs.counts[msgtype-1] // msgtypes start at 1
	*count++
	ses.bc.signal()

	return nil
}

// lookupEntry returns the message entry matching a message hash with msgtype
// and session id.  If msgtype is zero, any message type can be looked up.
func (p *Pool) lookupEntry(hash chainhash.Hash, msgtype msgtype, sid *[32]byte) (entry, bool) {
	e, ok := p.pool[hash]
	if !ok {
		return entry{}, false
	}
	if msgtype != 0 && e.msgtype != msgtype {
		return entry{}, false
	}
	if e.sid != *sid {
		return entry{}, false
	}

	return e, true
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

func checkFee(pr *wire.MsgMixPairReq, feeRate int64) error {
	fee := pr.InputValue - int64(pr.MessageCount)*pr.MixAmount
	if pr.Change != nil {
		fee -= pr.Change.Value
	}

	estimatedSize := estimateP2PKHv0SerializeSize(len(pr.UTXOs),
		int(pr.MessageCount), pr.Change != nil)
	requiredFee := feeForSerializeSize(feeRate, estimatedSize)
	if fee < requiredFee {
		return fmt.Errorf("not enough input value, or too low fee")
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
