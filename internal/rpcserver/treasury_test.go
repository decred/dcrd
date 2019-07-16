// Copyright (c) 2020 The Decred developers

// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcserver

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/rpcclient/v6"
	"github.com/decred/dcrd/rpctest"
	"github.com/decred/dcrd/txscript/v3"
	"github.com/decred/dcrd/wire"
)

// timeoutCtx returns a context with the given timeout and automatically calls
// cancel() if the test fails to clean up.
func timeoutCtx(t testing.TB, timeout time.Duration) context.Context {
	if timeout <= 0 {
		return context.Background()
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

type tspendPayout struct {
	address dcrutil.Address
	amount  dcrutil.Amount
}

func createTSpend(privKey []byte, payouts []tspendPayout, fee dcrutil.Amount, expiry uint32) *wire.MsgTx {
	// Calculate total payout.
	totalPayout := int64(0)
	for _, v := range payouts {
		totalPayout += int64(v.amount)
	}

	// OP_RETURN <8-byte ValueIn><24 byte random>
	payload := make([]byte, chainhash.HashSize)
	_, err := rand.Read(payload[8:])
	if err != nil {
		panic(err)
	}
	binary.LittleEndian.PutUint64(payload, uint64(totalPayout+int64(fee)))

	builder := txscript.NewScriptBuilder()
	builder.AddOp(txscript.OP_RETURN)
	builder.AddData(payload)
	opretScript, err := builder.Script()
	if err != nil {
		panic(err)
	}
	msgTx := wire.NewMsgTx()
	msgTx.Version = wire.TxVersionTreasury
	msgTx.Expiry = expiry
	msgTx.AddTxOut(wire.NewTxOut(0, opretScript))

	// OP_TGEN
	for _, v := range payouts {
		script, err := txscript.PayToAddrScript(v.address)
		if err != nil {
			panic(err)
		}
		tgenScript := make([]byte, len(script)+1)
		tgenScript[0] = txscript.OP_TGEN
		copy(tgenScript[1:], script)
		msgTx.AddTxOut(wire.NewTxOut(int64(v.amount), tgenScript))
	}

	// Treasury spend transactions have no inputs since the funds are
	// sourced from a special account, so previous outpoint is zero hash
	// and max index.
	msgTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         int64(fee) + totalPayout,
		BlockHeight:     wire.NullBlockHeight,
		BlockIndex:      wire.NullBlockIndex,
		SignatureScript: nil,
	})

	// Calculate TSpend signature without SigHashType.
	sigscript, err := txscript.TSpendSignatureScript(msgTx, privKey)
	if err != nil {
		panic(err)
	}
	msgTx.TxIn[0].SignatureScript = sigscript

	return msgTx
}

func createTAdd(t testing.TB, privKey []byte, prevOut *wire.OutPoint, pkScript []byte,
	amountIn, amountOut, fee dcrutil.Amount,
	changeAddr dcrutil.Address) *wire.MsgTx {

	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *prevOut,
		Sequence:         wire.MaxTxInSequenceNum,
		ValueIn:          int64(amountIn),
	})

	changeScript, err := txscript.PayToSStxChange(changeAddr)
	if err != nil {
		t.Fatal(err)
	}
	changeAmount := amountIn - amountOut - fee
	tx.AddTxOut(wire.NewTxOut(int64(amountOut), []byte{txscript.OP_TADD}))
	if changeAmount > 0 {
		tx.AddTxOut(wire.NewTxOut(int64(changeAmount), changeScript))
	}
	tx.Version = wire.TxVersionTreasury

	sig, err := txscript.SignatureScript(tx, 0, pkScript,
		txscript.SigHashAll, privKey,
		dcrec.STEcdsaSecp256k1, true)
	if err != nil {
		t.Fatalf("unable to generate sig: %v", err)
	}
	tx.TxIn[0].SignatureScript = sig

	return tx
}

// TestTreasury performs a test of treasury functionality across the entire
// dcrd stack.
func TestTreasury(t *testing.T) {
	var handlers *rpcclient.NotificationHandlers
	net := chaincfg.SimNetParams()

	defaultFeeRate := dcrutil.Amount(1e4)

	// Setup the log dir for tests to ease debugging after failures.
	logDir := ".dcrdlogs"
	extraArgs := []string{
		"--debuglevel=MINR=trace,TRSY=trace",
		"--logdir=" + logDir,
	}
	info, err := os.Stat(logDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("error stating log dir: %v", err)
	}
	if info != nil {
		if !info.IsDir() {
			t.Fatalf("logdir (%s) is not a dir", logDir)
		}
		err = os.RemoveAll(logDir)
		if err != nil {
			t.Fatalf("error removing logdir: %v", err)
		}
	}

	// Create the rpctest harness and mine outputs for the voting wallet to
	// use.
	hn, err := rpctest.New(t, net, handlers, extraArgs)
	if err != nil {
		t.Fatal(err)
	}
	err = hn.SetUp(false, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer hn.TearDown()
	_, err = rpctest.AdjustedSimnetMiner(timeoutCtx(t, time.Minute), hn.Node, 64)
	if err != nil {
		t.Fatal(err)
	}

	// Create the voting wallet.
	vw, err := rpctest.NewVotingWallet(context.Background(), hn)
	if err != nil {
		t.Fatalf("unable to create voting wallet for test: %v", err)
	}
	err = vw.Start()
	if err != nil {
		t.Fatalf("unable to setup voting wallet: %v", err)
	}
	vw.SetErrorReporting(func(vwerr error) {
		t.Fatalf("voting wallet errored: %v", vwerr)
	})
	vw.SetMiner(func(ctx context.Context, nb uint32) ([]*chainhash.Hash, error) {
		return rpctest.AdjustedSimnetMiner(ctx, hn.Node, nb)
	})

	// Create a privkey and p2pkh addr we control for use in the tests.
	privKey := secp256k1.NewPrivateKey(new(secp256k1.ModNScalar).SetInt(1))
	pubKey := privKey.PubKey().SerializeCompressed()
	pubKeyHash := dcrutil.Hash160(pubKey)
	p2pkhAddr, err := dcrutil.NewAddressPubKeyHash(pubKeyHash, net,
		dcrec.STEcdsaSecp256k1)
	if err != nil {
		t.Fatal(err)
	}
	p2pkhScript, err := txscript.PayToAddrScript(p2pkhAddr)
	if err != nil {
		t.Fatal(err)
	}

	// Generate a p2sh script and addr we control for use in the tests.
	p2shScript := []byte{txscript.OP_TRUE}
	p2shSigScript := []byte{txscript.OP_DATA_1, txscript.OP_TRUE}
	p2shAddr, err := dcrutil.NewAddressScriptHash(p2shScript, net)
	if err != nil {
		t.Fatal(err)
	}

	// Send funds to outputs we control so we can spend it on TAdds.
	nbTAddPrevOuts := 3
	taddInAmt := dcrutil.Amount(1e8) // 1 DCR
	taddPrevOuts := make([]*wire.OutPoint, nbTAddPrevOuts)
	for i := 0; i < nbTAddPrevOuts; i++ {
		txOut := &wire.TxOut{PkScript: p2pkhScript, Value: int64(taddInAmt)}
		txHash, err := hn.SendOutputs([]*wire.TxOut{txOut}, defaultFeeRate)
		if err != nil {
			t.Fatal(err)
		}
		taddPrevOuts[i] = &wire.OutPoint{Hash: *txHash}
	}

	// Advance until SVH.
	_, startHeight, err := hn.Node.GetBestBlock(timeoutCtx(t, time.Second))
	if err != nil {
		t.Fatalf("unable to obtain best block: %v", err)
	}
	targetHeight := net.StakeValidationHeight
	if targetHeight > startHeight {
		nbBlocks := uint32(targetHeight - startHeight)
		_, err = vw.GenerateBlocks(timeoutCtx(t, 5*time.Minute), nbBlocks)
		if err != nil {
			t.Fatalf("unable to mine to SVH: %v", err)
		}
	}

	// Shorter versions of useful params for convenience.
	tvi := net.TreasuryVoteInterval
	mul := net.TreasuryVoteIntervalMultiplier
	piKey, _ := hex.DecodeString("62deae1ab2b1ebd96a28c80e870aee325bed359e83d8db2464ef999e616a9eef")

	// Create a TSpend that pays to a privkey we control and to a P2SH
	// address we know how to redeem.
	expiry := standalone.CalculateTSpendExpiry(targetHeight+1, tvi, mul)
	tspendFee := dcrutil.Amount(5190)
	tspendAmount := dcrutil.Amount(7e8) // 7 DCR
	payouts := []tspendPayout{
		{address: p2pkhAddr, amount: tspendAmount},
		{address: p2shAddr, amount: tspendAmount},
	}
	tspendYes := createTSpend(piKey, payouts, tspendFee, expiry)

	// Create a tspend that will be disapproved (voted no).
	tspendNo := createTSpend(piKey, payouts, tspendFee, expiry)

	// Create a tspend that will never be voted, therefore shouldn't be
	// mined.
	tspendAbstain := createTSpend(piKey, payouts, tspendFee, expiry)

	// Create a very large tspend that will be approved but shouldn't be
	// mined due to spending more than allowed by the expenditure policy.
	largeAmount := dcrutil.Amount(net.TreasuryExpenditureBootstrap * 100)
	largePayout := []tspendPayout{{address: p2pkhAddr, amount: largeAmount}}
	tspendLarge := createTSpend(piKey, largePayout, tspendFee, expiry)

	// Create a TAdd that pays the change back to a privkey we control.
	taddFee := dcrutil.Amount(2550)
	taddChange := taddInAmt - taddInAmt/2 - taddFee
	tadd1 := createTAdd(t, privKey.Serialize(), taddPrevOuts[0], p2pkhScript, taddInAmt,
		taddInAmt/2, taddFee, p2pkhAddr)
	tadd1Hash := tadd1.TxHash()

	// Create a TAdd that pays the change back to a p2sh we control.
	tadd2 := createTAdd(t, privKey.Serialize(), taddPrevOuts[1], p2pkhScript, taddInAmt,
		taddInAmt/2, taddFee, p2shAddr)
	tadd2Hash := tadd2.TxHash()

	// Create a TAdd that doesn't have change.
	tadd3 := createTAdd(t, privKey.Serialize(), taddPrevOuts[2], p2pkhScript, taddInAmt,
		taddInAmt-taddFee, taddFee, p2pkhAddr)
	if len(tadd3.TxOut) > 1 {
		t.Fatalf("tadd3 should not have had change")
	}

	// Set the voting wallet to vote for our tspends.
	tspendYesHash := tspendYes.TxHash()
	tspendNoHash := tspendNo.TxHash()
	tspendLargeHash := tspendLarge.TxHash()
	vw.VoteForTSpends([]*stake.TreasuryVoteTuple{
		{Hash: tspendYesHash, Vote: stake.TreasuryVoteYes},
		{Hash: tspendNoHash, Vote: stake.TreasuryVoteNo},
		{Hash: tspendLargeHash, Vote: stake.TreasuryVoteYes},
	})

	// Publish the tspends so the node will include them once they're
	// approved.
	txs := []*wire.MsgTx{tspendYes, tspendNo, tspendLarge, tspendAbstain}
	for i, tx := range txs {
		_, err = hn.Node.SendRawTransaction(timeoutCtx(t, time.Second), tx, true)
		if err != nil {
			t.Fatalf("unable to publish tspend %d: %v", i, err)
		}
	}

	// Generate one TVI worth of blocks to start voting then TVI*MUL blocks
	// to approve but stop just before the tspend will be mined.
	nbBlocks := uint32(tvi + tvi*mul - 1)
	_, err = vw.GenerateBlocks(timeoutCtx(t, time.Minute), nbBlocks)
	if err != nil {
		t.Fatalf("unable to mine to blocks to approve tspend: %v", err)
	}

	// Publish the tadds so both the tspend and tadds are mined at the same
	// block.
	txs = []*wire.MsgTx{tadd1, tadd2, tadd3}
	for i, tx := range txs {
		_, err = hn.Node.SendRawTransaction(timeoutCtx(t, time.Second), tx, true)
		if err != nil {
			t.Fatalf("unable to publish tadd %d: %v", i, err)
		}
	}

	// Mine the tspend and tadds and then until their funds are mature and
	// spendable and their outputs are reflected in the treasury balance.
	nbBlocks = uint32(1 + net.CoinbaseMaturity + 1)
	_, err = vw.GenerateBlocks(timeoutCtx(t, time.Minute), nbBlocks)
	if err != nil {
		t.Fatalf("unable to mine to blocks to approve tspend: %v", err)
	}

	// Create a tx that spends from the TSPend outputs and the TAdd change
	// outputs.
	tx := wire.NewMsgTx()
	txFee := dcrutil.Amount(5550)
	tx.AddTxIn(&wire.TxIn{ // TSpend P2PKH output
		PreviousOutPoint: wire.OutPoint{
			Hash:  tspendYesHash,
			Index: 1,
			Tree:  1,
		},
		Sequence: wire.MaxTxInSequenceNum,
		ValueIn:  int64(tspendAmount),
	})
	tx.AddTxIn(&wire.TxIn{ // TSpend P2SH output
		PreviousOutPoint: wire.OutPoint{
			Hash:  tspendYesHash,
			Index: 2,
			Tree:  1,
		},
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         int64(tspendAmount),
		SignatureScript: p2shSigScript,
	})
	tx.AddTxIn(&wire.TxIn{ // TAdd P2PKH output
		PreviousOutPoint: wire.OutPoint{
			Hash:  tadd1Hash,
			Index: 1,
			Tree:  1,
		},
		Sequence: wire.MaxTxInSequenceNum,
		ValueIn:  int64(taddChange),
	})
	tx.AddTxIn(&wire.TxIn{ // TAdd P2SH output
		PreviousOutPoint: wire.OutPoint{
			Hash:  tadd2Hash,
			Index: 1,
			Tree:  1,
		},
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         int64(taddChange),
		SignatureScript: p2shSigScript,
	})
	tx.AddTxOut(&wire.TxOut{
		Version:  0,
		Value:    int64(tspendAmount + taddChange - txFee),
		PkScript: p2pkhScript,
	})

	// Generate signatures for the P2PKH inputs.
	sig, err := txscript.SignatureScript(tx, 0, tspendYes.TxOut[1].PkScript,
		txscript.SigHashAll, privKey.Serialize(),
		dcrec.STEcdsaSecp256k1, true)
	if err != nil {
		t.Fatalf("unable to generate sig: %v", err)
	}
	tx.TxIn[0].SignatureScript = sig

	sig, err = txscript.SignatureScript(tx, 2, tadd1.TxOut[1].PkScript,
		txscript.SigHashAll, privKey.Serialize(),
		dcrec.STEcdsaSecp256k1, true)
	if err != nil {
		t.Fatalf("unable to generate sig: %v", err)
	}
	tx.TxIn[2].SignatureScript = sig

	// Publish the spending tx.
	spendTxHash, err := hn.Node.SendRawTransaction(timeoutCtx(t, time.Second), tx, true)
	if err != nil {
		t.Fatalf("unable to publish spend tx: %v", err)
	}

	// Mine it.
	_, err = vw.GenerateBlocks(timeoutCtx(t, time.Minute), 1)
	if err != nil {
		t.Fatalf("unable to mine to blocks to approve tspend: %v", err)
	}

	// Ensure the spending tx output is part of the utxo set and has 1
	// confirmation.
	utxo, err := hn.Node.GetTxOut(timeoutCtx(t, time.Second), spendTxHash, 0, false)
	if err != nil {
		t.Fatalf("unable to fetch spend tx utxo: %v", err)
	}
	if utxo.Confirmations != 1 {
		t.Fatalf("unexpected confirmations in spend tx utxo. want=%d got=%d",
			1, utxo.Confirmations)
	}

	// Ensure the disapproved, large and abstained tspend outputs are not
	// part of the utxo set (i.e. they were not mined).
	tspendAbstainHash := tspendAbstain.TxHash()
	hashes := []*chainhash.Hash{&tspendLargeHash, &tspendNoHash, &tspendAbstainHash}
	for i, h := range hashes {
		utxo, err = hn.Node.GetTxOut(timeoutCtx(t, time.Second), h, 1, false)
		if err != nil {
			t.Fatalf("unable to fetch tspend %d utxo: %v", i, err)
		}
		if utxo != nil {
			t.Fatalf("unexpected confirmations in tspend %d utxo: %v",
				i, utxo)
		}
	}

	// Fetch current block for tbase calculation.
	_, height, err := hn.Node.GetBestBlock(timeoutCtx(t, time.Second))
	if err != nil {
		t.Fatalf("unable to obtain best block: %v", err)
	}

	// Calculate the final expected treasury balance. It should be:
	//      Sum(treasury bases)
	//    + tadd1 amount - tadd1 change - tadd fees   ; p2pkh tadd
	//    + tadd2 amount - tadd2 change - tadd fees   ; p2sh tadd
	//    + tadd3 amount - tadd fees                  ; changeless tadd
	//    - tspend amount                             ; p2pkh out
	//    - tspend amount                             ; p2sh out
	//    - tspend fee
	//
	// Note that since we mine a lot of blocks, we've likely passed subsidy
	// reduction intervals, so we need to account for that when summing
	// treasury bases.

	// Calculate sum of treasurybases. Loop through all subsidy reduction
	// intervals until the last block that affected the treasury balance.
	var tbaseTotal dcrutil.Amount
	lastMatureTbaseBlock := height - int64(net.CoinbaseMaturity) + 1
	subsidy := net.BaseSubsidy
	tbaseSubsidy := subsidy * int64(net.BlockTaxProportion) /
		int64(net.TotalSubsidyProportions())
	sri := net.SubsidyReductionInterval
	for i := int64(0); i < lastMatureTbaseBlock; {
		nbBlocks := sri
		if i == 0 {
			// First SRI needs to ignore blocks 0 and 1 which don't
			// have treasury base.
			nbBlocks -= 2
		}
		if i+nbBlocks > lastMatureTbaseBlock {
			// On the last interval we only sum up to the last
			// mature tbase block.
			nbBlocks = lastMatureTbaseBlock % sri
		}
		tbaseTotal += dcrutil.Amount(nbBlocks * tbaseSubsidy)
		i += sri
		subsidy = subsidy * net.MulSubsidy / net.DivSubsidy
		tbaseSubsidy = subsidy * int64(net.BlockTaxProportion) /
			int64(net.TotalSubsidyProportions())
	}

	// Calculate the final expected balance
	wantBalance := tbaseTotal +
		taddInAmt - taddChange - taddFee + // p2pkh tadd
		taddInAmt - taddChange - taddFee + // p2sh tadd
		taddInAmt - taddFee + // tadd without change
		-tspendAmount*2 - tspendFee // p2pkh+p2sh tspend
	bal, err := hn.Node.GetTreasuryBalance(timeoutCtx(t, time.Second), nil, false)
	if err != nil {
		t.Fatalf("unable to get treasury balance: %v", err)
	}
	gotBalance := dcrutil.Amount(bal.Balance)
	if gotBalance != wantBalance {
		t.Logf("height %d", height)
		t.Logf("tbaseTotal %s   taddInAmt %d   taddChange   %d",
			tbaseTotal, taddInAmt, taddChange)
		t.Logf("taddFee %d   tspendAmount %d   tspendFee %d",
			taddFee, tspendAmount, tspendFee)
		t.Fatalf("unexpected treasury balance. want=%s got=%s",
			wantBalance, gotBalance)
	}

}
