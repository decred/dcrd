// Copyright (c) 2023-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mixpool

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"decred.org/cspp/v2/solverrpc"
	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/mixing"
	"github.com/decred/dcrd/mixing/internal/chacha20prng"
	"github.com/decred/dcrd/mixing/utxoproof"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

var params = chaincfg.SimNetParams()

var seed [32]byte

var prngNonce atomic.Uint32

func testPRNG(t *testing.T) *chacha20prng.Reader {
	t.Helper()
	n := prngNonce.Add(1) - 1
	t.Cleanup(func() {
		t.Helper()
		if t.Failed() {
			t.Logf("Reproduce with -seed=%x/%d", seed[:], n)
		}
	})
	return chacha20prng.New(seed[:], n)
}

var utxoStore struct {
	byName     map[string]*mockUTXO
	byOutpoint map[wire.OutPoint]UtxoEntry
}

func TestMain(m *testing.M) {
	seedFlag := flag.String("seed", "", "use deterministic PRNG seed (32 bytes, hex)")
	flag.Parse()
	if *seedFlag != "" {
		slash := strings.IndexByte(*seedFlag, '/')
		if slash != 64 {
			fmt.Fprintln(os.Stderr, "invalid -seed: must be in form <32 byte hex seed>/iteration")
			os.Exit(1)
		}
		b, err := hex.DecodeString((*seedFlag)[:slash])
		if err != nil {
			fmt.Fprintln(os.Stderr, "invalid -seed:", err)
			os.Exit(1)
		}
		copy(seed[:], b)
		nonce, err := strconv.ParseUint((*seedFlag)[slash+1:], 10, 32)
		if err != nil {
			fmt.Fprintln(os.Stderr, "invalid -seed nonce:", err)
			os.Exit(1)
		}
		prngNonce.Store(uint32(nonce))
	} else {
		cryptorand.Read(seed[:])
	}

	rand := chacha20prng.New(seed[:], 0)

	utxoStore.byName = makeMockUTXOs(rand)
	utxoStore.byOutpoint = make(map[wire.OutPoint]UtxoEntry)
	for _, m := range utxoStore.byName {
		utxoStore.byOutpoint[m.outpoint] = m
	}

	os.Exit(m.Run())
}

func generateSecp256k1(rand io.Reader) (*secp256k1.PublicKey, *secp256k1.PrivateKey, error) {
	if rand == nil {
		rand = cryptorand.Reader
	}

	privateKey, err := secp256k1.GeneratePrivateKeyFromRand(rand)
	if err != nil {
		return nil, nil, err
	}

	publicKey := privateKey.PubKey()

	return publicKey, privateKey, nil
}

type mockUTXO struct {
	outpoint    wire.OutPoint
	tx          *wire.MsgTx
	output      *wire.TxOut
	blockHeight int64
	pubkey      []byte
	privkey     *secp256k1.PrivateKey
}

func (m *mockUTXO) IsSpent() bool         { return false }
func (m *mockUTXO) PkScript() []byte      { return m.output.PkScript }
func (m *mockUTXO) ScriptVersion() uint16 { return 0 }
func (m *mockUTXO) BlockHeight() int64    { return m.blockHeight }
func (m *mockUTXO) Amount() int64         { return m.output.Value }

func makeMockUTXO(rand io.Reader, value int64) *mockUTXO {
	tx := wire.NewMsgTx()

	pub, priv, err := generateSecp256k1(rand)
	if err != nil {
		panic(err)
	}
	pubSerialized := pub.SerializeCompressed()

	p2pkhScript := []byte{
		0:  txscript.OP_DUP,
		1:  txscript.OP_HASH160,
		2:  txscript.OP_DATA_20,
		23: txscript.OP_EQUALVERIFY,
		24: txscript.OP_CHECKSIG,
	}
	hash160 := stdaddr.Hash160(pubSerialized)
	copy(p2pkhScript[3:23], hash160)

	output := wire.NewTxOut(value, p2pkhScript)
	tx.AddTxOut(output)

	return &mockUTXO{
		outpoint: wire.OutPoint{
			Hash:  tx.TxHash(),
			Index: 0,
			Tree:  0,
		},
		tx:      tx,
		output:  output,
		pubkey:  pubSerialized,
		privkey: priv,
	}
}

func (m *mockUTXO) mixprutxo(expires uint32) wire.MixPairReqUTXO {
	k := utxoproof.Secp256k1KeyPair{
		Pub:  m.pubkey,
		Priv: m.privkey,
	}
	sig, err := k.SignUtxoProof(expires)
	if err != nil {
		panic(err)
	}

	return wire.MixPairReqUTXO{
		OutPoint:  m.outpoint,
		PubKey:    m.pubkey,
		Signature: sig,
	}
}

func makeMockUTXOs(rand io.Reader) map[string]*mockUTXO {
	return map[string]*mockUTXO{
		"A": makeMockUTXO(rand, 20e8),
	}
}

type fakechain struct {
	hash   chainhash.Hash
	height int64
}

func (c *fakechain) ChainParams() *chaincfg.Params {
	return params
}

func (c *fakechain) CurrentTip() (chainhash.Hash, int64) {
	return c.hash, c.height
}

func (c *fakechain) FetchUtxoEntry(op wire.OutPoint) (UtxoEntry, error) {
	entry := utxoStore.byOutpoint[op]
	if entry == nil {
		return nil, errors.New("no utxo entry")
	}
	return entry, nil
}

func TestAccept(t *testing.T) {
	t.Parallel()
	testRand := testPRNG(t)

	c := new(fakechain)
	c.height = 1000
	p := NewPool(c)

	identityPub, identityPriv, err := generateSecp256k1(testRand)
	if err != nil {
		t.Fatal(err)
	}
	identity := *(*[33]byte)(identityPub.SerializeCompressed())

	h := blake256.New()

	var (
		expires      uint32 = 1010
		mixAmount    int64  = 10e8
		scriptClass         = mixing.ScriptClassP2PKHv0
		txVersion    uint16 = wire.TxVersion
		lockTime     uint32 = 0
		messageCount uint32 = 1
		inputValue   int64  = 20e8
		utxos        []wire.MixPairReqUTXO
		change       *wire.TxOut
		flags        byte = 0
		pairingFlags byte = 0
	)
	utxos = []wire.MixPairReqUTXO{
		utxoStore.byName["A"].mixprutxo(expires),
	}
	pr, err := wire.NewMsgMixPairReq(identity, expires, mixAmount,
		string(scriptClass), txVersion, lockTime, messageCount,
		inputValue, utxos, change, flags, pairingFlags)
	if err != nil {
		t.Fatal(err)
	}
	pr.WriteHash(h)
	err = mixing.SignMessage(pr, identityPriv)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.AcceptMessage(pr, ZeroSource)
	if err != nil {
		t.Fatal(err)
	}

	prngSeed := testRand.Next(32)
	runPRNG := chacha20prng.New(prngSeed, 0)
	kx, err := mixing.NewKX(runPRNG)
	if err != nil {
		t.Fatal(err)
	}

	// Generate unpadded SR and DC messages.
	var msize uint32 = 20
	var mcount uint32 = 2
	srMsg := make([]*big.Int, mcount)
	for i := range srMsg {
		srMsg[i], err = cryptorand.Int(testRand, mixing.F)
		if err != nil {
			t.Fatal(err)
		}
	}
	dcMsg := make([][]byte, mcount)
	for i := range dcMsg {
		dcMsg[i] = testRand.Next(int(msize))
	}
	t.Logf("SR messages %+x\n", srMsg)
	t.Logf("DC messages %+x\n", dcMsg)

	var (
		seenPRs               = []chainhash.Hash{pr.Hash()}
		epoch      uint64     = uint64(time.Now().Unix())
		sid        [32]byte   = mixing.SortPRsForSession([]*wire.MsgMixPairReq{pr}, epoch)
		run        uint32     = 0
		pos        uint32     = 0
		ecdh       [33]byte   = *(*[33]byte)(kx.ECDHPublicKey.SerializeCompressed())
		pqpk       [1218]byte = *kx.PQPublicKey
		commitment [32]byte   // XXX: hash of RS message
	)
	ke := wire.NewMsgMixKeyExchange(identity, sid, epoch, run, pos, ecdh,
		pqpk, commitment, seenPRs)
	ke.WriteHash(h)
	err = mixing.SignMessage(ke, identityPriv)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.AcceptMessage(ke, ZeroSource)
	if err != nil {
		t.Fatal(err)
	}

	pqPubkeys := []*mixing.PQPublicKey{kx.PQPublicKey}
	ciphertexts, err := kx.Encapsulate(runPRNG, pqPubkeys, 0)
	if err != nil {
		t.Fatal(err)
	}

	seenKEs := []chainhash.Hash{ke.Hash()}
	ct := wire.NewMsgMixCiphertexts(identity, sid, run, ciphertexts, seenKEs)
	ct.WriteHash(h)
	err = mixing.SignMessage(ct, identityPriv)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.AcceptMessage(ct, ZeroSource)
	if err != nil {
		t.Fatal(err)
	}

	mcounts := []uint32{mcount}
	revealedKeys := &mixing.RevealedKeys{
		ECDHPublicKeys: []*secp256k1.PublicKey{kx.ECDHPublicKey},
		Ciphertexts:    ciphertexts,
		MyIndex:        0,
	}
	secrets, err := kx.SharedSecrets(revealedKeys, sid[:], run, mcounts)
	if err != nil {
		t.Fatal(err)
	}

	// Pad SR messages
	srmix := make([][]*big.Int, mcount)
	myStart := uint32(0)
	for i := uint32(0); i < mcount; i++ {
		pads := mixing.SRMixPads(secrets.SRSecrets[i], myStart+i)
		padded := mixing.SRMix(srMsg[i], pads)
		srmix[i] = make([]*big.Int, len(padded))
		copy(srmix[i], padded)
	}
	srmixBytes := make([][][]byte, len(srmix))
	for i := range srmix {
		srmixBytes[i] = make([][]byte, len(srmix[i]))
		for j := range srmix[i] {
			srmixBytes[i][j] = srmix[i][j].Bytes()
		}
	}
	t.Logf("SR mix %+x\n", srmix)

	seenCTs := []chainhash.Hash{ct.Hash()}
	sr := wire.NewMsgMixSlotReserve(identity, sid, run, srmixBytes, seenCTs)
	sr.WriteHash(h)
	err = mixing.SignMessage(sr, identityPriv)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.AcceptMessage(sr, ZeroSource)
	if err != nil {
		t.Fatal(err)
	}

	vs := srmix
	powerSums := mixing.AddVectors(vs...)
	coeffs := mixing.Coefficients(powerSums)
	if err := solverrpc.StartSolver(); err == nil {
		roots, err := solverrpc.Roots(coeffs, mixing.F)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("solved roots %+x\n", roots)
	}

	// Pad DC messages
	dcmix := make([]wire.MixVect, mcount)
	var slots []int
	for i := 0; i < int(mcount); i++ {
		slots = append(slots, i)
	}
	for i, slot := range slots {
		my := myStart + uint32(i)
		pads := mixing.DCMixPads(secrets.DCSecrets[i], my)
		dcmix[i] = wire.MixVect(mixing.DCMix(pads, dcMsg[i], uint32(slot)))
	}

	seenSRs := []chainhash.Hash{sr.Hash()}
	dc := wire.NewMsgMixDCNet(identity, sid, run, dcmix, seenSRs)
	dc.WriteHash(h)
	err = mixing.SignMessage(dc, identityPriv)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.AcceptMessage(dc, ZeroSource)
	if err != nil {
		t.Fatal(err)
	}

	dcVecs := make([]mixing.Vec, 0, len(dcmix))
	for _, vec := range dcmix {
		dcVecs = append(dcVecs, mixing.Vec(vec))
	}
	res := mixing.XorVectors(dcVecs)
	t.Logf("recovered message set %v", res.String())

	tx := wire.NewMsgTx()
	for i := range res {
		hash160 := res[i][:]
		pkscript := []byte{
			0:  txscript.OP_DUP,
			1:  txscript.OP_HASH160,
			2:  txscript.OP_DATA_20,
			23: txscript.OP_EQUALVERIFY,
			24: txscript.OP_CHECKSIG,
		}
		copy(pkscript[3:23], hash160)
		tx.AddTxOut(wire.NewTxOut(mixAmount, pkscript))
	}
	t.Logf("mixed tx hash %v", tx.TxHash())

	seenDCs := []chainhash.Hash{dc.Hash()}
	cm := wire.NewMsgMixConfirm(identity, sid, run, tx, seenDCs)
	cm.WriteHash(h)
	err = mixing.SignMessage(cm, identityPriv)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.AcceptMessage(cm, ZeroSource)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("%s", spew.Sdump(tx))
}
