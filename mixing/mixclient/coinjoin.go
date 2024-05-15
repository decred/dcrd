// Copyright (c) 2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixclient

import (
	"crypto/subtle"
	"errors"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrutil/v4/txsort"
	"github.com/decred/dcrd/mixing/utxoproof"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/wire"
)

// msize is the message size of a mixed message (hash160).
const msize = 20

var (
	// errMissingGen indicates one or more dishonest peers in the DC-net
	// that must be removed by revealing secrets, assigning blame, and
	// rerunning with them excluded.
	errMissingGen = errors.New("coinjoin is missing gen output")

	// errSignedWrongTx indicates a peer signed a different transaction
	// than the coinjoin transaction that we constructed.  Peers who sent
	// these invalid CM messages must be removed in the next rerun.
	errSignedWrongTx = errors.New("peer signed incorrect mix transaction")
)

// identity is a peer's secp256k1 compressed public key.
type identity = [33]byte

// CoinJoin tracks the in-progress coinjoin transaction for a single peer as
// the mixing protocol is performed.
type CoinJoin struct {
	txHash chainhash.Hash

	genFunc GenFunc

	tx           *wire.MsgTx
	change       *wire.TxOut
	prevScripts  map[wire.OutPoint][]byte
	peerPRs      map[identity]*wire.MsgMixPairReq
	contributed  map[wire.OutPoint]identity
	prUTXOs      []wire.MixPairReqUTXO
	myInputs     []int
	gens         wire.MixVect
	genScripts   [][]byte
	mixedIndices []int

	mixValue   int64
	inputValue int64
	prExpiry   uint32
	mcount     uint32
}

// GenFunc generates fresh secp256k1 P2PKH hash160s from the wallet.
type GenFunc func(count uint32) (wire.MixVect, error)

// NewCoinJoin creates the initial coinjoin transaction.  Inputs must be
// contributed to the coinjoin by one or more calls to AddInput.
func NewCoinJoin(gen GenFunc, change *wire.TxOut, mixValue int64, prExpiry uint32, mcount uint32) *CoinJoin {
	return &CoinJoin{
		genFunc:     gen,
		tx:          wire.NewMsgTx(),
		change:      change,
		peerPRs:     make(map[identity]*wire.MsgMixPairReq),
		contributed: make(map[wire.OutPoint]identity),
		prevScripts: make(map[wire.OutPoint][]byte),
		mixValue:    mixValue,
		prExpiry:    prExpiry,
		mcount:      mcount,
	}
}

// AddInput adds an contributed input to the coinjoin transaction.
//
// The private key is used to generate a UTXO signature proof demonstrating
// that this wallet is able to spend the UTXO.  The private key is not
// retained by the CoinJoin structure and may be zerod from memory after
// AddInput returns.
func (c *CoinJoin) AddInput(input *wire.TxIn, prevScript []byte, prevScriptVersion uint16,
	privKey *secp256k1.PrivateKey) error {

	pub := privKey.PubKey().SerializeCompressed()
	keyPair := utxoproof.Secp256k1KeyPair{
		Priv: privKey,
		Pub:  pub,
	}
	proofSig, err := keyPair.SignUtxoProof(c.prExpiry)
	if err != nil {
		return err
	}

	var opcode byte
	switch prevScript[0] {
	case txscript.OP_SSGEN, txscript.OP_SSRTX, txscript.OP_TGEN:
		opcode = prevScript[0]
	}

	c.prUTXOs = append(c.prUTXOs, wire.MixPairReqUTXO{
		OutPoint:  input.PreviousOutPoint,
		Script:    nil, // Only for P2SH
		PubKey:    pub,
		Signature: proofSig,
		Opcode:    opcode,
	})

	c.prevScripts[input.PreviousOutPoint] = prevScript
	c.inputValue += input.ValueIn

	return nil
}

// resetUnmixed (re)initializes the coinjoin transaction with all peers'
// unmixed data from their pair request messages.
func (c *CoinJoin) resetUnmixed(prs []*wire.MsgMixPairReq) {
	c.tx.TxIn = c.tx.TxIn[:0]
	c.tx.TxOut = c.tx.TxOut[:0]
	c.mixedIndices = c.mixedIndices[:0]
	c.myInputs = c.myInputs[:0]
	for id := range c.peerPRs {
		delete(c.peerPRs, id)
	}
	for outpoint := range c.contributed {
		delete(c.contributed, outpoint)
	}

	for _, pr := range prs {
		c.peerPRs[pr.Identity] = pr
		for i := range pr.UTXOs {
			prevOutPoint := &pr.UTXOs[i].OutPoint
			in := wire.NewTxIn(prevOutPoint, wire.NullValueIn, nil)
			c.tx.AddTxIn(in)
			c.contributed[*prevOutPoint] = pr.Identity
		}
		if pr.Change != nil {
			c.tx.AddTxOut(pr.Change)
		}
	}
}

// gen calls the message generator function, recording and returning the
// freshly generated messages to be mixed.
func (c *CoinJoin) gen() (wire.MixVect, error) {
	gens, err := c.genFunc(c.mcount)
	if err != nil {
		return nil, err
	}

	genScripts := make([][]byte, len(gens))
	for i, m := range gens {
		script := make([]byte, 25)
		script[0] = txscript.OP_DUP
		script[1] = txscript.OP_HASH160
		script[2] = txscript.OP_DATA_20
		copy(script[3:23], m[:])
		script[23] = txscript.OP_EQUALVERIFY
		script[24] = txscript.OP_CHECKSIG
		genScripts[i] = script
	}

	c.gens = gens
	c.genScripts = genScripts
	return gens, nil
}

// addMixedMessage adds a transaction output paying to the mixed hash160
// message.
func (c *CoinJoin) addMixedMessage(m []byte) {
	if len(m) != msize {
		return
	}

	script := make([]byte, 25)
	script[0] = txscript.OP_DUP
	script[1] = txscript.OP_HASH160
	script[2] = txscript.OP_DATA_20
	copy(script[3:23], m[:])
	script[23] = txscript.OP_EQUALVERIFY
	script[24] = txscript.OP_CHECKSIG

	c.tx.AddTxOut(wire.NewTxOut(c.mixValue, script))
}

// sort performs an in-place sort of the transaction, retaining any and all
// internal bookkeeping about which inputs and outputs are contributed by the
// client for the confirmation checks.  sort must be called before any
// signatures are created or included so that all peers deterministicly agree
// on the trasaction to sign.
func (c *CoinJoin) sort() {
	txsort.InPlaceSort(c.tx)

	c.myInputs = c.myInputs[:0]
	for i, in := range c.tx.TxIn {
		_, ok := c.prevScripts[in.PreviousOutPoint]
		if ok {
			c.myInputs = append(c.myInputs, i)
		}
	}

	c.txHash = c.tx.TxHash()
}

// constantTimeOutputSearch searches for the output indices of mixed outputs to
// verify inclusion in a coinjoin.  It is constant time such that, for each
// searched script, all outputs with equal value, script versions, and script
// lengths matching the searched output are checked in constant time.
func constantTimeOutputSearch(tx *wire.MsgTx, value int64, scriptVer uint16, scripts [][]byte) ([]int, error) {
	var scan []int
	for i, out := range tx.TxOut {
		if out.Value != value {
			continue
		}
		if out.Version != scriptVer {
			continue
		}
		if len(out.PkScript) != len(scripts[0]) {
			continue
		}
		scan = append(scan, i)
	}
	indices := make([]int, 0, len(scan))
	var missing int
	for _, s := range scripts {
		idx := -1
		for _, i := range scan {
			eq := subtle.ConstantTimeCompare(tx.TxOut[i].PkScript, s)
			idx = subtle.ConstantTimeSelect(eq, i, idx)
		}
		indices = append(indices, idx)
		eq := subtle.ConstantTimeEq(int32(idx), -1)
		missing = subtle.ConstantTimeSelect(eq, 1, missing)
	}
	if missing == 1 {
		return nil, errMissingGen
	}
	return indices, nil
}

// confirm ensures that generated messages are present in the transaction and
// signs inputs being contributed by the peer.  returns errMissingGen to
// trigger blame assignment if a message is not found.
func (c *CoinJoin) confirm(wallet Wallet) error {
	genIndices, err := constantTimeOutputSearch(c.tx, c.mixValue, 0, c.genScripts)
	if err != nil {
		return err
	}

	for _, input := range c.myInputs {
		prevOutpoint := &c.tx.TxIn[input].PreviousOutPoint
		err := wallet.SignInput(c.tx, input, c.prevScripts[*prevOutpoint])
		if err != nil {
			return err
		}
	}

	c.mixedIndices = genIndices
	return nil
}

// mergeSignatures adds the signatures from another peer's CM message to the
// coinjoin transaction.  Only those inputs contributed by the peer are
// modified.
//
// Peers must be removed in the next run if mergeSignatures returns an error.
func (c *CoinJoin) mergeSignatures(cm *wire.MsgMixConfirm) error {
	// Signatures may only be used if an identical transaction was signed.
	if cm.Mix.TxHash() != c.txHash {
		return errSignedWrongTx
	}

	for i, in := range cm.Mix.TxIn {
		if cm.Identity != c.contributed[in.PreviousOutPoint] {
			continue
		}
		c.tx.TxIn[i].SignatureScript = in.SignatureScript
	}

	return nil
}

// Tx returns the coinjoin transaction.
func (c *CoinJoin) Tx() *wire.MsgTx {
	return c.tx
}

// MixedIndices returns the peer's mixed transaction output indices.
func (c *CoinJoin) MixedIndices() []int {
	return c.mixedIndices
}
