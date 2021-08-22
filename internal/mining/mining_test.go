// Copyright (c) 2020-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"errors"
	"testing"

	"github.com/decred/dcrd/blockchain/v4"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

// TestNewBlockTemplateBasicErrorScenarios tests various basic error scenarios
// that can occur during new block template generation.
func TestNewBlockTemplateBasicErrorScenarios(t *testing.T) {
	t.Parallel()

	// Create a new mining harness instance.
	harness, _, err := newMiningHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("error creating mining harness: %v", err)
	}

	// Create a test address for use in template generation.
	address, err := stdaddr.DecodeAddress("Dsi8CRt85xYyempXs7ZPL1rBxvDdAGZmgsg",
		harness.chainParams)
	if err != nil {
		t.Fatalf("error decoding address: %v", err)
	}

	// Test error retrieving standard verify flags.
	standardVerifyFlags := harness.policy.StandardVerifyFlags
	var errFlags = errors.New("error retrieving standard verify flags")
	harness.policy.StandardVerifyFlags = func() (txscript.ScriptFlags, error) {
		return 0, errFlags
	}
	_, err = harness.generator.NewBlockTemplate(address)
	if !errors.Is(err, errFlags) {
		t.Fatalf("unexpected error retrieving standard verify flags -- got %v, "+
			"want %v", err, errFlags)
	}
	harness.policy.StandardVerifyFlags = standardVerifyFlags

	// Test error retrieving treasury agenda.
	var errTreasuryAgenda = errors.New("error retrieving treasury agenda")
	harness.chain.isTreasuryAgendaActiveErr = errTreasuryAgenda
	_, err = harness.generator.NewBlockTemplate(address)
	if !errors.Is(err, errTreasuryAgenda) {
		t.Fatalf("unexpected error retrieving treasury agenda -- got %v, want %v",
			err, errTreasuryAgenda)
	}
	harness.chain.isTreasuryAgendaActiveErr = nil
}

// TestNewBlockTemplate tests the generation of a new block template containing
// regular and vote transactions.
func TestNewBlockTemplate(t *testing.T) {
	t.Parallel()

	// Create a new mining harness instance.
	harness, spendableOuts, err := newMiningHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("error creating mining harness: %v", err)
	}

	// Create a test address for use in template generation.
	address, err := stdaddr.DecodeAddress("Dsi8CRt85xYyempXs7ZPL1rBxvDdAGZmgsg",
		harness.chainParams)
	if err != nil {
		t.Fatalf("error decoding address: %v", err)
	}

	// Define a munger to apply transaction fees.
	applyTxFee := func(fee int64) func(*wire.MsgTx) {
		return func(tx *wire.MsgTx) {
			tx.TxOut[0].Value -= fee
		}
	}

	// Create additional transactions from the first spendable output provided by
	// the harness.
	const numTxs = 6
	txs := make([]*dcrutil.Tx, numTxs)
	baseTx, err := harness.CreateSignedTx(spendableOuts, uint32(numTxs))
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}
	harness.AddFakeUTXO(baseTx, harness.chain.bestState.Height, 1,
		harness.chain.isTreasuryAgendaActive)
	for i := 0; i < numTxs; i++ {
		tx, err := harness.CreateSignedTx([]spendableOutput{
			txOutToSpendableOut(baseTx, uint32(i), wire.TxTreeRegular)}, 1,
			applyTxFee(5000))
		if err != nil {
			t.Fatalf("unable to create transaction: %v", err)
		}
		txs[i] = tx
	}

	// Create ticket purchase transactions spending the outputs of the prior
	// regular transactions.
	const numVotes = 5
	tickets := make([]*dcrutil.Tx, numVotes)
	ticketHashes := make([]chainhash.Hash, numVotes)
	for i := 0; i < numVotes; i++ {
		ticket, err := harness.CreateTicketPurchase(txs[i], 40000)
		if err != nil {
			t.Fatalf("unable to create ticket purchase transaction: %v", err)
		}
		tickets[i] = ticket
		ticketHashes[i] = ticket.MsgTx().TxHash()
	}

	// Add the ticket outputs as utxos to fake their existence.  Use one after
	// the stake enabled height for the height of the fake utxos to ensure they
	// are mature for the votes cast at stake validation height below.
	harness.chain.bestState.Height = harness.chainParams.StakeEnabledHeight + 1
	for i, ticket := range tickets {
		harness.AddFakeUTXO(ticket, harness.chain.bestState.Height, uint32(i+1),
			harness.chain.isTreasuryAgendaActive)
	}

	// Create votes on a block at stake validation height using the previously
	// created tickets.
	harness.chain.bestState.Height = harness.chainParams.StakeValidationHeight
	harness.chain.bestState.NextWinningTickets = ticketHashes
	votes := make([]*dcrutil.Tx, numVotes)
	for i, ticket := range tickets {
		vote, err := harness.CreateVote(ticket)
		if err != nil {
			t.Fatalf("unable to create vote: %v", err)
		}
		votes[i] = vote
	}

	// Add vote transactions to the tx source.
	for _, vote := range votes {
		_, err = harness.AddTransactionToTxSource(vote)
		if err != nil {
			t.Fatalf("unable to add transaction to the tx source: %v", err)
		}
	}

	// Add remaining regular transactions to the tx source.
	for i := numVotes; i < numTxs; i++ {
		_, err = harness.AddTransactionToTxSource(txs[i])
		if err != nil {
			t.Fatalf("unable to add transaction to the tx source: %v", err)
		}
	}

	// Generate a new block template.
	blockTemplate, err := harness.generator.NewBlockTemplate(address)
	if err != nil {
		t.Fatalf("unexpected err generating block template: %v", err)
	}

	// Validate the number of transactions in the generated block template.
	gotTx := len(blockTemplate.Block.Transactions)
	wantTx := numTxs - numVotes + 1 // + 1 for coinbase.
	if gotTx != wantTx {
		t.Fatalf("unexpected number of transactions in template --  got %v, want %v",
			gotTx, wantTx)
	}

	// Validate the number of stake transactions in the generated block template.
	gotStx := len(blockTemplate.Block.STransactions)
	wantStx := numVotes + 1 // + 1 for stakebase.
	if gotStx != wantStx {
		t.Fatalf("unexpected number of stake transactions in template --  got %v, "+
			"want %v", gotStx, wantStx)
	}

	// Validate that the block is sane.  These checks are context free.
	block := dcrutil.NewBlock(blockTemplate.Block)
	err = blockchain.CheckBlockSanity(block, harness.generator.cfg.TimeSource,
		harness.chainParams)
	if err != nil {
		t.Fatalf("unexpected error when checking block sanity: %v", err)
	}
}

// TestNewBlockTemplateAutoRevocations tests the generation of a new block with
// automatic ticket revocations enabled.
func TestNewBlockTemplateAutoRevocations(t *testing.T) {
	t.Parallel()

	// Create a new mining harness instance.
	harness, spendableOuts, err := newMiningHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("error creating mining harness: %v", err)
	}

	// Activate the automatic ticket revocations agenda.
	harness.chain.isAutoRevocationsAgendaActive = true

	// Create a test address for use in template generation.
	address, err := stdaddr.DecodeAddress("Dsi8CRt85xYyempXs7ZPL1rBxvDdAGZmgsg",
		harness.chainParams)
	if err != nil {
		t.Fatalf("error decoding address: %v", err)
	}

	// Define a munger to apply transaction fees.
	applyTxFee := func(fee int64) func(*wire.MsgTx) {
		return func(tx *wire.MsgTx) {
			tx.TxOut[0].Value -= fee
		}
	}

	// Create additional transactions from the first spendable output provided by
	// the harness.
	const numTxs = 11
	txs := make([]*dcrutil.Tx, numTxs)
	baseTx, err := harness.CreateSignedTx(spendableOuts, uint32(numTxs))
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}
	harness.AddFakeUTXO(baseTx, harness.chain.bestState.Height, 1,
		harness.chain.isTreasuryAgendaActive)
	for i := 0; i < numTxs; i++ {
		tx, err := harness.CreateSignedTx([]spendableOutput{
			txOutToSpendableOut(baseTx, uint32(i), wire.TxTreeRegular)}, 1,
			applyTxFee(5000))
		if err != nil {
			t.Fatalf("unable to create transaction: %v", err)
		}
		txs[i] = tx
	}

	// Create ticket purchase transactions spending the outputs of the prior
	// regular transactions.
	const numTickets = 10
	tickets := make([]*dcrutil.Tx, numTickets)
	ticketHashes := make([]chainhash.Hash, numTickets)
	for i := 0; i < numTickets; i++ {
		ticket, err := harness.CreateTicketPurchase(txs[i], 40000)
		if err != nil {
			t.Fatalf("unable to create ticket purchase transaction: %v", err)
		}
		tickets[i] = ticket
		ticketHashes[i] = ticket.MsgTx().TxHash()
	}

	// Add the ticket outputs as utxos to fake their existence.  Use one after
	// the stake enabled height for the height of the fake utxos to ensure they
	// are mature for the votes cast at stake validation height below.
	harness.chain.bestState.Height = harness.chainParams.StakeEnabledHeight + 1
	for i, ticket := range tickets {
		harness.AddFakeUTXO(ticket, harness.chain.bestState.Height, uint32(i+1),
			harness.chain.isTreasuryAgendaActive)
	}

	// Create votes on a block at stake validation height using the previously
	// created tickets.
	//
	// Create votes for 3 out of 5 winning tickets.  The 2 remaining winning
	// tickets will be missed and will need to be revoked.
	winningTickets := ticketHashes[:5]
	voteTickets := winningTickets[:3]
	missedThisBlock := winningTickets[3:]
	harness.chain.bestState.Height = harness.chainParams.StakeValidationHeight
	harness.chain.bestState.NextWinningTickets = winningTickets
	numVotes := len(voteTickets)
	votes := make([]*dcrutil.Tx, numVotes)
	for i, ticket := range tickets[:numVotes] {
		vote, err := harness.CreateVote(ticket)
		if err != nil {
			t.Fatalf("unable to create vote: %v", err)
		}
		votes[i] = vote
	}

	// Add vote transactions to the tx source.
	for _, vote := range votes {
		_, err = harness.AddTransactionToTxSource(vote)
		if err != nil {
			t.Fatalf("unable to add transaction to the tx source: %v", err)
		}
	}

	// Add 2 of the remaining non-winning tickets to the list of tickets expiring
	// next block to simulate expiring tickets that will need to be revoked.
	expiredThisBlock := ticketHashes[5:7]
	harness.chain.bestState.NextExpiringTickets = expiredThisBlock

	// Add 2 of the remaining non-winning tickets to the list of tickets that were
	// already missed.
	missedPreviously := ticketHashes[7:9]
	harness.chain.bestState.MissedTickets = missedPreviously

	// Use 1 ticket as a ticket that is ineligible to vote or be revoked.
	ineligibleTicket := ticketHashes[9]

	// Add an additional vote transaction with a bad input.  This ensures that a
	// revocation is still created for a missed ticket due to a vote transaction
	// failing validation.
	vote, err := harness.CreateVote(tickets[numVotes])
	if err != nil {
		t.Fatalf("unable to create vote: %v", err)
	}
	_, err = harness.AddTransactionToTxSource(vote)
	if err != nil {
		t.Fatalf("unable to add transaction to the tx source: %v", err)
	}
	vote.MsgTx().TxIn[0].ValueIn = 0

	// Get the best header bytes.
	bestHeader, err := harness.chain.HeaderByHash(&harness.chain.bestState.Hash)
	if err != nil {
		t.Fatalf("unable to get tip block header %v: %v",
			harness.chain.bestState.Hash, err)
	}
	bestHeaderBytes, err := bestHeader.Bytes()
	if err != nil {
		t.Fatalf("failed to serialize header for block %v: %v",
			bestHeader.BlockHash(), err)
	}

	// Add revocations for tickets that were previously missed.
	for _, ticketHash := range harness.chain.bestState.MissedTickets {
		txDesc, err := harness.generator.createRevocationFromTicket(&ticketHash,
			harness.chain.utxos, bestHeaderBytes,
			harness.chain.isTreasuryAgendaActive)
		if err != nil {
			t.Fatalf("failed to create revocation for ticket %v: %v", ticketHash, err)
		}
		_, err = harness.AddTransactionToTxSource(txDesc.Tx)
		if err != nil {
			t.Fatalf("unable to add transaction to the tx source: %v", err)
		}
	}

	// Add a revocation that tries to spend an ineligible ticket.
	txDesc, err := harness.generator.createRevocationFromTicket(&ineligibleTicket,
		harness.chain.utxos, bestHeaderBytes, harness.chain.isTreasuryAgendaActive)
	if err != nil {
		t.Fatalf("failed to create revocation for ticket %v: %v", ineligibleTicket,
			err)
	}
	_, err = harness.AddTransactionToTxSource(txDesc.Tx)
	if err != nil {
		t.Fatalf("unable to add transaction to the tx source: %v", err)
	}

	// Add remaining regular transactions to the tx source.
	for i := numVotes; i < numTxs; i++ {
		_, err = harness.AddTransactionToTxSource(txs[i])
		if err != nil {
			t.Fatalf("unable to add transaction to the tx source: %v", err)
		}
	}

	// Generate a new block template.
	blockTemplate, err := harness.generator.NewBlockTemplate(address)
	if err != nil {
		t.Fatalf("unexpected err generating block template: %v", err)
	}

	// Validate the number of transactions in the generated block template.
	gotTx := len(blockTemplate.Block.Transactions)
	wantTx := numTxs - numVotes + 1 // + 1 for coinbase.
	if gotTx != wantTx {
		t.Fatalf("unexpected number of transactions in template --  got %v, want %v",
			gotTx, wantTx)
	}

	// Determine the expected number of revocation transactions.
	numAutoRevocations := len(missedThisBlock) + len(expiredThisBlock)
	numTxSourceRevocations := len(missedPreviously)
	numRevocations := numAutoRevocations + numTxSourceRevocations

	// Validate the number of stake transactions in the generated block template.
	gotStx := len(blockTemplate.Block.STransactions)
	wantStx := numVotes + numRevocations + 1 // + 1 for stakebase.
	if gotStx != wantStx {
		t.Fatalf("unexpected number of stake transactions in template --  got %v, "+
			"want %v", gotStx, wantStx)
	}

	// Validate that the block is sane.  These checks are context free.
	block := dcrutil.NewBlock(blockTemplate.Block)
	err = blockchain.CheckBlockSanity(block, harness.generator.cfg.TimeSource,
		harness.chainParams)
	if err != nil {
		t.Fatalf("unexpected error when checking block sanity: %v", err)
	}
}

// TestNewBlockTemplateAutoRevocationsVotesOnly tests that auto revocations are
// properly added to blocks that only contain vote transactions.
func TestNewBlockTemplateAutoRevocationsVotesOnly(t *testing.T) {
	t.Parallel()

	// Create a new mining harness instance.
	harness, spendableOuts, err := newMiningHarness(chaincfg.MainNetParams())
	if err != nil {
		t.Fatalf("error creating mining harness: %v", err)
	}

	// Activate the automatic ticket revocations agenda.
	harness.chain.isAutoRevocationsAgendaActive = true

	// Create a test address for use in template generation.
	address, err := stdaddr.DecodeAddress("Dsi8CRt85xYyempXs7ZPL1rBxvDdAGZmgsg",
		harness.chainParams)
	if err != nil {
		t.Fatalf("error decoding address: %v", err)
	}

	// Define a munger to apply transaction fees.
	applyTxFee := func(fee int64) func(*wire.MsgTx) {
		return func(tx *wire.MsgTx) {
			tx.TxOut[0].Value -= fee
		}
	}

	// Create additional transactions from the first spendable output provided by
	// the harness.
	const numTxs = 6
	txs := make([]*dcrutil.Tx, numTxs)
	baseTx, err := harness.CreateSignedTx(spendableOuts, uint32(numTxs))
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}
	harness.AddFakeUTXO(baseTx, harness.chain.bestState.Height, 1,
		harness.chain.isTreasuryAgendaActive)
	for i := 0; i < numTxs; i++ {
		tx, err := harness.CreateSignedTx([]spendableOutput{
			txOutToSpendableOut(baseTx, uint32(i), wire.TxTreeRegular)}, 1,
			applyTxFee(5000))
		if err != nil {
			t.Fatalf("unable to create transaction: %v", err)
		}
		txs[i] = tx
	}

	// Create ticket purchase transactions spending the outputs of the prior
	// regular transactions.
	const numTickets = 5
	tickets := make([]*dcrutil.Tx, numTickets)
	ticketHashes := make([]chainhash.Hash, numTickets)
	for i := 0; i < numTickets; i++ {
		ticket, err := harness.CreateTicketPurchase(txs[i], 40000)
		if err != nil {
			t.Fatalf("unable to create ticket purchase transaction: %v", err)
		}
		tickets[i] = ticket
		ticketHashes[i] = ticket.MsgTx().TxHash()
	}

	// Add the ticket outputs as utxos to fake their existence.  Use one after
	// the stake enabled height for the height of the fake utxos to ensure they
	// are mature for the votes cast at stake validation height below.
	harness.chain.bestState.Height = harness.chainParams.StakeEnabledHeight + 1
	for i, ticket := range tickets {
		harness.AddFakeUTXO(ticket, harness.chain.bestState.Height, uint32(i+1),
			harness.chain.isTreasuryAgendaActive)
	}

	// Create votes on a block at stake validation height using the previously
	// created tickets.
	//
	// Create votes for 3 out of 5 winning tickets.  The 2 remaining winning
	// tickets will be missed and will need to be revoked.
	winningTickets := ticketHashes[:5]
	voteTickets := winningTickets[:3]
	missedThisBlock := winningTickets[3:]
	harness.chain.bestState.Height = harness.chainParams.StakeValidationHeight
	harness.chain.bestState.NextWinningTickets = winningTickets
	numVotes := len(voteTickets)
	votes := make([]*dcrutil.Tx, numVotes)
	for i, ticket := range tickets[:numVotes] {
		vote, err := harness.CreateVote(ticket)
		if err != nil {
			t.Fatalf("unable to create vote: %v", err)
		}
		votes[i] = vote
	}

	// Add vote transactions to the tx source.
	for _, vote := range votes {
		_, err = harness.AddTransactionToTxSource(vote)
		if err != nil {
			t.Fatalf("unable to add transaction to the tx source: %v", err)
		}
	}

	// Generate a new block template.
	blockTemplate, err := harness.generator.NewBlockTemplate(address)
	if err != nil {
		t.Fatalf("unexpected err generating block template: %v", err)
	}

	// Determine the expected number of revocation transactions.
	numAutoRevocations := len(missedThisBlock)

	// Validate the number of stake transactions in the generated block template.
	gotStx := len(blockTemplate.Block.STransactions)
	wantStx := numVotes + numAutoRevocations + 1 // + 1 for stakebase.
	if gotStx != wantStx {
		t.Fatalf("unexpected number of stake transactions in template --  got %v, "+
			"want %v", gotStx, wantStx)
	}

	// Validate that the block is sane.  These checks are context free.
	block := dcrutil.NewBlock(blockTemplate.Block)
	err = blockchain.CheckBlockSanity(block, harness.generator.cfg.TimeSource,
		harness.chainParams)
	if err != nil {
		t.Fatalf("unexpected error when checking block sanity: %v", err)
	}
}
