// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"fmt"

	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/blockchain/standalone"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/wire"
)

// blockOneCoinbasePaysTokens checks to see if the first block coinbase pays
// out to the network initial token ledger.
func blockOneCoinbasePaysTokens(tx *dcrutil.Tx, params *chaincfg.Params) error {
	// Nothing to do when there is no ledger specified.
	if len(params.BlockOneLedger) == 0 {
		return nil
	}

	if tx.MsgTx().LockTime != 0 {
		str := fmt.Sprintf("block 1 coinbase has invalid locktime")
		return ruleError(ErrBlockOneTx, str)
	}

	if tx.MsgTx().Expiry != wire.NoExpiryValue {
		str := fmt.Sprintf("block 1 coinbase has invalid expiry")
		return ruleError(ErrBlockOneTx, str)
	}

	if tx.MsgTx().TxIn[0].Sequence != wire.MaxTxInSequenceNum {
		str := fmt.Sprintf("block 1 coinbase not finalized")
		return ruleError(ErrBlockOneInputs, str)
	}

	if len(tx.MsgTx().TxOut) == 0 {
		str := fmt.Sprintf("coinbase outputs empty in block 1")
		return ruleError(ErrBlockOneOutputs, str)
	}

	ledger := params.BlockOneLedger
	if len(ledger) != len(tx.MsgTx().TxOut) {
		str := fmt.Sprintf("wrong number of outputs in block 1 coinbase; "+
			"got %v, expected %v", len(tx.MsgTx().TxOut), len(ledger))
		return ruleError(ErrBlockOneOutputs, str)
	}

	// Check the addresses and output amounts against those in the ledger.
	const consensusScriptVersion = 0
	for i, txOut := range tx.MsgTx().TxOut {
		ledgerEntry := &ledger[i]
		if txOut.Version != ledgerEntry.ScriptVersion {
			str := fmt.Sprintf("block one output %d script version %d is not %d",
				i, txOut.Version, consensusScriptVersion)
			return ruleError(ErrBlockOneOutputs, str)
		}

		if !bytes.Equal(txOut.PkScript, ledgerEntry.Script) {
			str := fmt.Sprintf("block one output %d script %x is not %x", i,
				txOut.PkScript, ledgerEntry.Script)
			return ruleError(ErrBlockOneOutputs, str)
		}

		if txOut.Value != ledgerEntry.Amount {
			str := fmt.Sprintf("block one output %d generates %v instead of "+
				"required %v", i, dcrutil.Amount(txOut.Value),
				dcrutil.Amount(ledgerEntry.Amount))
			return ruleError(ErrBlockOneOutputs, str)
		}
	}

	return nil
}

// coinbasePaysTreasury checks to see if a given block's coinbase correctly pays
// the treasury.
func coinbasePaysTreasury(subsidyCache *standalone.SubsidyCache, tx *dcrutil.Tx, height int64, voters uint16, params *chaincfg.Params) error {
	// Treasury subsidy only applies from block 2 onwards.
	if height <= 1 {
		return nil
	}

	// Treasury subsidy is disabled.
	if params.BlockTaxProportion == 0 {
		return nil
	}

	if len(tx.MsgTx().TxOut) == 0 {
		str := fmt.Sprintf("invalid coinbase (no outputs)")
		return ruleError(ErrNoTxOutputs, str)
	}

	treasuryOutput := tx.MsgTx().TxOut[0]
	if treasuryOutput.Version != params.OrganizationPkScriptVersion {
		str := fmt.Sprintf("treasury output version %d is instead of %d",
			treasuryOutput.Version, params.OrganizationPkScriptVersion)
		return ruleError(ErrNoTax, str)
	}
	if !bytes.Equal(treasuryOutput.PkScript, params.OrganizationPkScript) {
		str := fmt.Sprintf("treasury output script is %x instead of %x",
			treasuryOutput.PkScript, params.OrganizationPkScript)
		return ruleError(ErrNoTax, str)
	}

	// Calculate the amount of subsidy that should have been paid out to the
	// Treasury and ensure the subsidy generated is correct.
	orgSubsidy := subsidyCache.CalcTreasurySubsidy(height, voters)
	if orgSubsidy != treasuryOutput.Value {
		str := fmt.Sprintf("treasury output amount is %s instead of %s",
			dcrutil.Amount(treasuryOutput.Value), dcrutil.Amount(orgSubsidy))
		return ruleError(ErrNoTax, str)
	}

	return nil
}

// calculateAddedSubsidy calculates the amount of subsidy added by a block
// and its parent. The blocks passed to this function MUST be valid blocks
// that have already been confirmed to abide by the consensus rules of the
// network, or the function might panic.
func calculateAddedSubsidy(block, parent *dcrutil.Block) int64 {
	var subsidy int64
	if headerApprovesParent(&block.MsgBlock().Header) {
		subsidy += parent.MsgBlock().Transactions[0].TxIn[0].ValueIn
	}

	for _, stx := range block.MsgBlock().STransactions {
		if stake.IsSSGen(stx) {
			subsidy += stx.TxIn[0].ValueIn
		}
	}

	return subsidy
}
