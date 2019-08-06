// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"fmt"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/blockchain/standalone"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

// The number of values to precalculate on initialization of the subsidy
// cache.
const subsidyCacheInitWidth = 4

// SubsidyCache is a structure that caches calculated values of subsidy so that
// they're not constantly recalculated. The blockchain struct itself possesses a
// pointer to a preinitialized SubsidyCache.
//
// Deprecated: Use standalone.SubsidyCache instead.
type SubsidyCache = standalone.SubsidyCache

// subsidyParams adapts v1 chaincfg.Params to implement the
// standalone.SubsidyParams interface.  It is already implemented by the v2
// chaincfg.Params, but updating to those requires a major version bump since
// the type is used in the public API.
type subsidyParams struct {
	*chaincfg.Params
}

// BaseSubsidyValue returns the starting base max potential subsidy amount for
// mined blocks.
//
// This is part of the standalone.SubsidyParams interface.
func (p *subsidyParams) BaseSubsidyValue() int64 {
	return p.BaseSubsidy
}

// SubsidyReductionMultiplier returns the multiplier to use when performing the
// exponential subsidy reduction.
//
// This is part of the standalone.SubsidyParams interface.
func (p *subsidyParams) SubsidyReductionMultiplier() int64 {
	return p.MulSubsidy
}

// SubsidyReductionDivisor returns the divisor to use when performing the
// exponential subsidy reduction.
//
// This is part of the standalone.SubsidyParams interface.
func (p *subsidyParams) SubsidyReductionDivisor() int64 {
	return p.DivSubsidy
}

// SubsidyReductionIntervalBlocks returns the reduction interval in number of
// blocks.
//
// This is part of the standalone.SubsidyParams interface.
func (p *subsidyParams) SubsidyReductionIntervalBlocks() int64 {
	return p.SubsidyReductionInterval
}

// WorkSubsidyProportion returns the comparative proportion of the subsidy
// generated for creating a block (PoW).
//
// This is part of the standalone.SubsidyParams interface.
func (p *subsidyParams) WorkSubsidyProportion() uint16 {
	return p.WorkRewardProportion
}

// StakeSubsidyProportion returns the comparative proportion of the subsidy
// generated for casting stake votes (collectively, per block).
//
// This is part of the standalone.SubsidyParams interface.
func (p *subsidyParams) StakeSubsidyProportion() uint16 {
	return p.StakeRewardProportion
}

// TreasurySubsidyProportion returns the comparative proportion of the subsidy
// allocated to the project treasury.
//
// This is part of the standalone.SubsidyParams interface.
func (p *subsidyParams) TreasurySubsidyProportion() uint16 {
	return p.BlockTaxProportion
}

// VotesPerBlock returns the maximum number of votes a block must contain to
// receive full subsidy.
//
// This is part of the standalone.SubsidyParams interface.
func (p *subsidyParams) VotesPerBlock() uint16 {
	return p.TicketsPerBlock
}

// StakeValidationBeginHeight returns the height at which votes become required
// to extend a block.  This height is the first that will be voted on, but will
// not include any votes itself.
//
// This is part of the standalone.SubsidyParams interface.
func (p *subsidyParams) StakeValidationBeginHeight() int64 {
	return p.StakeValidationHeight
}

// NewSubsidyCache initializes a new subsidy cache for a given height. It
// precalculates the values of the subsidy that are most likely to be seen by
// the client when it connects to the network.
//
// Deprecated: Use standalone.NewSubsidyCache instead.
func NewSubsidyCache(height int64, params *chaincfg.Params) *SubsidyCache {
	return standalone.NewSubsidyCache(&subsidyParams{params})
}

// CalcBlockWorkSubsidy calculates the proof of work subsidy for a block as a
// proportion of the total subsidy.
//
// Deprecated: Use standalone.SubsidyCache.CalcWorkSubsidy instead.
func CalcBlockWorkSubsidy(subsidyCache *SubsidyCache, height int64, voters uint16, params *chaincfg.Params) int64 {
	return subsidyCache.CalcWorkSubsidy(height, voters)
}

// CalcStakeVoteSubsidy calculates the subsidy for a stake vote based on the height
// of its input SStx.
//
// Safe for concurrent access.
//
// Deprecated: Use standalone.SubsidyCache.CalcStakeVoteSubsidy instead.
func CalcStakeVoteSubsidy(subsidyCache *SubsidyCache, height int64, params *chaincfg.Params) int64 {
	return subsidyCache.CalcStakeVoteSubsidy(height)
}

// CalcBlockTaxSubsidy calculates the subsidy for the organization address in the
// coinbase.
//
// Safe for concurrent access.
//
// Deprecated: Use standalone.SubsidyCache.CalcTreasurySubsidy instead.
func CalcBlockTaxSubsidy(subsidyCache *SubsidyCache, height int64, voters uint16, params *chaincfg.Params) int64 {
	return subsidyCache.CalcTreasurySubsidy(height, voters)
}

// blockOneCoinbasePaysTokens checks to see if the first block coinbase pays
// out to the network initial token ledger.
func blockOneCoinbasePaysTokens(tx *dcrutil.Tx, params *chaincfg.Params) error {
	// If no ledger is specified, just return true.
	if len(params.BlockOneLedger) == 0 {
		return nil
	}

	if tx.MsgTx().LockTime != 0 {
		errStr := fmt.Sprintf("block 1 coinbase has invalid locktime")
		return ruleError(ErrBlockOneTx, errStr)
	}

	if tx.MsgTx().Expiry != wire.NoExpiryValue {
		errStr := fmt.Sprintf("block 1 coinbase has invalid expiry")
		return ruleError(ErrBlockOneTx, errStr)
	}

	if tx.MsgTx().TxIn[0].Sequence != wire.MaxTxInSequenceNum {
		errStr := fmt.Sprintf("block 1 coinbase not finalized")
		return ruleError(ErrBlockOneInputs, errStr)
	}

	if len(tx.MsgTx().TxOut) == 0 {
		errStr := fmt.Sprintf("coinbase outputs empty in block 1")
		return ruleError(ErrBlockOneOutputs, errStr)
	}

	ledger := params.BlockOneLedger
	if len(ledger) != len(tx.MsgTx().TxOut) {
		errStr := fmt.Sprintf("wrong number of outputs in block 1 coinbase; "+
			"got %v, expected %v", len(tx.MsgTx().TxOut), len(ledger))
		return ruleError(ErrBlockOneOutputs, errStr)
	}

	// Check the addresses and output amounts against those in the ledger.
	const consensusScriptVersion = 0
	for i, txout := range tx.MsgTx().TxOut {
		if txout.Version != consensusScriptVersion {
			str := fmt.Sprintf("block one output %d script version %d is not %d",
				i, txout.Version, consensusScriptVersion)
			return ruleError(ErrBlockOneOutputs, str)
		}

		// There should only be one address.
		_, addrs, _, err :=
			txscript.ExtractPkScriptAddrs(txout.Version, txout.PkScript, params)
		if err != nil {
			return ruleError(ErrBlockOneOutputs, err.Error())
		}
		if len(addrs) != 1 {
			errStr := fmt.Sprintf("too many addresses in output")
			return ruleError(ErrBlockOneOutputs, errStr)
		}

		addrLedger, err := dcrutil.DecodeAddress(ledger[i].Address)
		if err != nil {
			return err
		}

		if !bytes.Equal(addrs[0].ScriptAddress(), addrLedger.ScriptAddress()) {
			errStr := fmt.Sprintf("address in output %v has non matching "+
				"address; got %v (hash160 %x), want %v (hash160 %x)",
				i,
				addrs[0].EncodeAddress(),
				addrs[0].ScriptAddress(),
				addrLedger.EncodeAddress(),
				addrLedger.ScriptAddress())
			return ruleError(ErrBlockOneOutputs, errStr)
		}

		if txout.Value != ledger[i].Amount {
			errStr := fmt.Sprintf("address in output %v has non matching "+
				"amount; got %v, want %v", i, txout.Value, ledger[i].Amount)
			return ruleError(ErrBlockOneOutputs, errStr)
		}
	}

	return nil
}

// BlockOneCoinbasePaysTokens checks to see if the first block coinbase pays
// out to the network initial token ledger.
//
// Deprecated: This will be removed in the next major version bump.
func BlockOneCoinbasePaysTokens(tx *dcrutil.Tx, params *chaincfg.Params) error {
	return blockOneCoinbasePaysTokens(tx, params)
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

// CoinbasePaysTax checks to see if a given block's coinbase correctly pays
// tax to the developer organization.
//
// Deprecated:  This will be removed in the next major version.
func CoinbasePaysTax(subsidyCache *SubsidyCache, tx *dcrutil.Tx, height int64, voters uint16, params *chaincfg.Params) error {
	return coinbasePaysTreasury(subsidyCache, tx, height, voters, params)
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

// CalculateAddedSubsidy calculates the amount of subsidy added by a block
// and its parent. The blocks passed to this function MUST be valid blocks
// that have already been confirmed to abide by the consensus rules of the
// network, or the function might panic.
//
// Deprecated:  This will no longer be exported in the next major version.
func CalculateAddedSubsidy(block, parent *dcrutil.Block) int64 {
	return calculateAddedSubsidy(block, parent)
}
