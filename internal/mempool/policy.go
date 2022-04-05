// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2017-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"fmt"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v4"
	"github.com/decred/dcrd/blockchain/v5"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdscript"
	"github.com/decred/dcrd/wire"
)

const (
	// maxStandardP2SHSigOps is the maximum number of signature operations
	// that are considered standard in a pay-to-script-hash script.
	maxStandardP2SHSigOps = 15

	// MaxStandardTxSize is the maximum size allowed for transactions that
	// are considered standard and will therefore be relayed and considered
	// for mining.
	MaxStandardTxSize = 100000

	// maxStandardSigScriptSize is the maximum size allowed for a
	// transaction input signature script to be considered standard.  This
	// value allows for a 15-of-15 CHECKMULTISIG pay-to-script-hash with
	// compressed keys.
	//
	// The form of the overall script is: OP_0 <15 signatures> OP_PUSHDATA2
	// <2 bytes len> [OP_15 <15 pubkeys> OP_15 OP_CHECKMULTISIG]
	//
	// For the p2sh script portion, each of the 15 compressed pubkeys are
	// 33 bytes (plus one for the OP_DATA_33 opcode), and the thus it totals
	// to (15*34)+3 = 513 bytes.  Next, each of the 15 signatures is a max
	// of 73 bytes (plus one for the OP_DATA_73 opcode).  Also, there is one
	// extra byte for the initial extra OP_0 push and 3 bytes for the
	// OP_PUSHDATA2 needed to specify the 513 bytes for the script push.
	// That brings the total to 1+(15*74)+3+513 = 1627.  This value also
	// adds a few extra bytes to provide a little buffer.
	// (1 + 15*74 + 3) + (15*34 + 3) + 23 = 1650
	maxStandardSigScriptSize = 1650

	// DefaultMinRelayTxFee is the minimum fee in atoms that is required for
	// a transaction to be treated as free for relay and mining purposes.
	// It is also used to help determine if a transaction is considered dust
	// and as a base for calculating minimum required fees for larger
	// transactions.  This value is in Atoms/1000 bytes.
	DefaultMinRelayTxFee = dcrutil.Amount(1e4)

	// maxStandardMultiSigKeys is the maximum number of public keys allowed
	// in a multi-signature transaction output script for it to be
	// considered standard.
	maxStandardMultiSigKeys = 3

	// BaseStandardVerifyFlags defines the script flags that should be used
	// when executing transaction scripts to enforce additional checks which
	// are required for the script to be considered standard regardless of
	// the state of any agenda votes.  The full set of standard verification
	// flags must include these flags as well as any additional flags that
	// are conditionally enabled depending on the result of agenda votes.
	BaseStandardVerifyFlags = txscript.ScriptDiscourageUpgradableNops |
		txscript.ScriptVerifyCleanStack |
		txscript.ScriptVerifyCheckLockTimeVerify |
		txscript.ScriptVerifyCheckSequenceVerify
)

// calcMinRequiredTxRelayFee returns the minimum transaction fee required for a
// transaction with the passed serialized size to be accepted into the memory
// pool and relayed.
func calcMinRequiredTxRelayFee(serializedSize int64, minRelayTxFee dcrutil.Amount) int64 {
	// Calculate the minimum fee for a transaction to be allowed into the
	// mempool and relayed by scaling the base fee (which is the minimum
	// free transaction relay fee).  minTxRelayFee is in Atom/KB, so
	// multiply by serializedSize (which is in bytes) and divide by 1000 to
	// get minimum Atoms.
	minFee := (serializedSize * int64(minRelayTxFee)) / 1000

	if minFee == 0 && minRelayTxFee > 0 {
		minFee = int64(minRelayTxFee)
	}

	// Set the minimum fee to the maximum possible value if the calculated
	// fee is not in the valid range for monetary amounts.
	if minFee < 0 || minFee > dcrutil.MaxAmount {
		minFee = dcrutil.MaxAmount
	}

	return minFee
}

// checkInputsStandard performs a series of checks on a transaction's inputs
// to ensure they are "standard".  A standard transaction input within the
// context of this function is one whose referenced public key script is of a
// standard form and, for pay-to-script-hash, does not have more than
// maxStandardP2SHSigOps signature operations.  However, it should also be noted
// that standard inputs also are those which have a clean stack after execution
// and only contain pushed data in their signature scripts.  This function does
// not perform those checks because the script engine already does this more
// accurately and concisely via the txscript.ScriptVerifyCleanStack and
// txscript.ScriptVerifySigPushOnly flags.
//
// Note: all non-nil errors MUST be RuleError with an underlying TxRuleError
// instance.
func checkInputsStandard(tx *dcrutil.Tx, txType stake.TxType, utxoView *blockchain.UtxoViewpoint, isTreasuryEnabled bool) error {
	// NOTE: The reference implementation also does a coinbase check here,
	// but coinbases have already been rejected prior to calling this
	// function so no need to recheck.

	// Ignore the first input if this is a SSGen (vote) or tspend since
	// those inputs are not standard by definition.
	ignoreFirst := txType == stake.TxTypeSSGen ||
		(isTreasuryEnabled && txType == stake.TxTypeTSpend)

	for i, txIn := range tx.MsgTx().TxIn {
		if i == 0 && ignoreFirst {
			continue
		}

		// It is safe to elide existence and index checks here since
		// they have already been checked prior to calling this
		// function.
		entry := utxoView.LookupEntry(txIn.PreviousOutPoint)
		originPkScriptVer := entry.ScriptVersion()
		originPkScript := entry.PkScript()
		switch stdscript.DetermineScriptType(originPkScriptVer, originPkScript) {
		case stdscript.STScriptHash:
			numSigOps := txscript.GetPreciseSigOpCount(txIn.SignatureScript,
				originPkScript, isTreasuryEnabled)
			if numSigOps > maxStandardP2SHSigOps {
				str := fmt.Sprintf("transaction input #%d has "+
					"%d signature operations which is more "+
					"than the allowed max amount of %d",
					i, numSigOps, maxStandardP2SHSigOps)
				return txRuleError(ErrNonStandard, str)
			}

		case stdscript.STNonStandard:
			str := fmt.Sprintf("transaction input #%d has a "+
				"non-standard script form", i)
			return txRuleError(ErrNonStandard, str)
		}
	}

	return nil
}

// checkPkScriptStandard performs a series of checks on a transaction output
// script (public key script) to ensure it is a "standard" public key script.
// A standard public key script is one that is a recognized form, and for
// multi-signature scripts, only contains from 1 to maxStandardMultiSigKeys
// public keys.
//
// Note: all non-nil errors MUST be RuleError with an underlying TxRuleError
// instance.
func checkPkScriptStandard(version uint16, pkScript []byte,
	scriptType stdscript.ScriptType) error {

	// Only version 0 scripts are standard at the current time.
	if version != wire.DefaultPkScriptVersion {
		str := fmt.Sprintf("versions other than default pkscript version " +
			"are currently non-standard except for provably unspendable " +
			"outputs")
		return txRuleError(ErrNonStandard, str)
	}

	switch {
	case scriptType == stdscript.STMultiSig && version == 0:
		// A standard multi-signature public key script must contain
		// from 1 to maxStandardMultiSigKeys public keys.
		details := stdscript.ExtractMultiSigScriptDetailsV0(pkScript, false)
		numPubKeys := details.NumPubKeys
		if numPubKeys < 1 {
			str := "multi-signature script with no pubkeys"
			return txRuleError(ErrNonStandard, str)
		}
		if numPubKeys > maxStandardMultiSigKeys {
			str := fmt.Sprintf("multi-signature script with %d "+
				"public keys which is more than the allowed "+
				"max of %d", numPubKeys, maxStandardMultiSigKeys)
			return txRuleError(ErrNonStandard, str)
		}

		// A standard multi-signature public key script must have at least 1
		// signature and no more signatures than available public keys.
		//
		// NOTE: Due to recent updates in the standardness identification code,
		// the script should not have been identified as standard when there is
		// not at least a 1 signature, but be paranoid and double check it here
		// in case the standardness code changes in the future.
		numSigs := details.RequiredSigs
		if numSigs < 1 {
			return txRuleError(ErrNonStandard,
				"multi-signature script with no signatures")
		}
		if numSigs > numPubKeys {
			str := fmt.Sprintf("multi-signature script with %d "+
				"signatures which is more than the available %d "+
				"public keys", numSigs, numPubKeys)
			return txRuleError(ErrNonStandard, str)
		}

	case scriptType == stdscript.STNonStandard:
		return txRuleError(ErrNonStandard, "non-standard script form")
	}

	return nil
}

// isDust returns whether or not the passed transaction output amount is
// considered dust or not based on the passed minimum transaction relay fee.
// Dust is defined in terms of the minimum transaction relay fee.  In
// particular, if the cost to the network to spend coins is more than 1/3 of the
// minimum transaction relay fee, it is considered dust.
func isDust(txOut *wire.TxOut, minRelayTxFee dcrutil.Amount) bool {
	// Unspendable outputs are considered dust.
	if txscript.IsUnspendable(txOut.Value, txOut.PkScript) {
		return true
	}

	// The total serialized size consists of the output and the associated
	// input script to redeem it.  Since there is no input script
	// to redeem it yet, use the minimum size of a typical input script.
	//
	// Pay-to-pubkey-hash bytes breakdown:
	//
	//  Output to hash (36 bytes):
	//   8 value, 2 script version, 1 script len, 25 script [1 OP_DUP,
	//   1 OP_HASH_160, 1 OP_DATA_20, 20 hash, 1 OP_EQUALVERIFY,
	//   1 OP_CHECKSIG]
	//
	//  Input with compressed pubkey (165 bytes):
	//   37 prev outpoint, 4 sequence, 16 fraud proof, 1 script len,
	//   107 script [1 OP_DATA_72, 72 sig, 1 OP_DATA_33, 33 compressed
	//   pubkey]
	//
	//  Input with uncompressed pubkey (197 bytes):
	//   37 prev outpoint, 4 sequence, 16 fraud proof, 1 script len,
	//   139 script [1 OP_DATA_72, 72 sig, 1 OP_DATA_65, 65 uncompressed
	//   pubkey]
	//
	// Pay-to-pubkey bytes breakdown:
	//
	//  Output to compressed pubkey (46 bytes):
	//   8 value, 2 script version, 1 script len, 35 script [1 OP_DATA_33,
	//   33 compressed pubkey, 1 OP_CHECKSIG]
	//
	//  Output to uncompressed pubkey (78 bytes):
	//   8 value, 2 script version, 1 script len, 67 script [1 OP_DATA_65,
	//   65 uncompressed pubkey, 1 OP_CHECKSIG]
	//
	//  Input (131 bytes):
	//   37 prev outpoint, 4 sequence, 16 fraud proof, 1 script len,
	//   73 script [1 OP_DATA_72, 72 sig]
	//
	// Theoretically this could examine the script type of the output script
	// and use a different size for the typical input script size for
	// pay-to-pubkey vs pay-to-pubkey-hash inputs per the above breakdowns,
	// but the only combination which is less than the value chosen is
	// a pay-to-pubkey script with a compressed pubkey, which is not very
	// common.
	//
	// The most common scripts are pay-to-pubkey-hash, and as per the above
	// breakdown, the minimum size of a p2pkh input script is 165 bytes.  So
	// that figure is used.
	totalSize := txOut.SerializeSize() + 165

	// The output is considered dust if the cost to the network to spend the
	// coins is more than 1/3 of the minimum free transaction relay fee.
	// minFreeTxRelayFee is in Atom/KB, so multiply by 1000 to convert to
	// bytes.
	//
	// Using the typical values for a pay-to-pubkey-hash transaction from
	// the breakdown above and the default minimum free transaction relay
	// fee of 10000, this equates to values less than 6030 atoms being
	// considered dust.
	//
	// The following is equivalent to (value/totalSize) * (1/3) * 1000
	// without needing to do floating point math.
	return txOut.Value*1000/(3*int64(totalSize)) < int64(minRelayTxFee)
}

// checkTransactionStandard performs a series of checks on a transaction to
// ensure it is a "standard" transaction.  A standard transaction is one that
// conforms to several additional limiting cases over what is considered a
// "sane" transaction such as having a version in the supported range, being
// finalized, conforming to more stringent size constraints, having scripts
// of recognized forms, and not containing "dust" outputs (those that are
// so small it costs more to process them than they are worth).
//
// Note: all non-nil errors MUST be RuleError with an underlying TxRuleError
// instance.
func checkTransactionStandard(tx *dcrutil.Tx, txType stake.TxType, height int64,
	medianTime time.Time, minRelayTxFee dcrutil.Amount) error {

	// The transaction must be a currently supported serialize type.
	msgTx := tx.MsgTx()
	if msgTx.SerType != wire.TxSerializeFull {
		str := fmt.Sprintf("transaction is not serialized with all "+
			"required data -- type %v", msgTx.SerType)
		return txRuleError(ErrNonStandard, str)
	}

	// The transaction must be finalized to be standard and therefore
	// considered for inclusion in a block.
	if !blockchain.IsFinalizedTransaction(tx, height, medianTime) {
		return txRuleError(ErrNonStandard, "transaction is not finalized")
	}

	// Since extremely large transactions with a lot of inputs can cost
	// almost as much to process as the sender fees, limit the maximum
	// size of a transaction.  This also helps mitigate CPU exhaustion
	// attacks.
	serializedLen := msgTx.SerializeSize()
	if serializedLen > MaxStandardTxSize {
		str := fmt.Sprintf("transaction size of %v is larger than max "+
			"allowed size of %v", serializedLen, MaxStandardTxSize)
		return txRuleError(ErrNonStandard, str)
	}

	for i, txIn := range msgTx.TxIn {
		// TSpends should only have one input and that input has a
		// specific format which is checked by IsTSpend, so if this tx
		// is a tspend, skip it.
		if txType == stake.TxTypeTSpend {
			continue
		}

		// Each transaction input signature script must not exceed the
		// maximum size allowed for a standard transaction.  See
		// the comment on maxStandardSigScriptSize for more details.
		sigScriptLen := len(txIn.SignatureScript)
		if sigScriptLen > maxStandardSigScriptSize {
			str := fmt.Sprintf("transaction input %d: signature "+
				"script size of %d bytes is large than max "+
				"allowed size of %d bytes", i, sigScriptLen,
				maxStandardSigScriptSize)
			return txRuleError(ErrNonStandard, str)
		}

		// Each transaction input signature script must only contain
		// opcodes which push data onto the stack.
		if !txscript.IsPushOnlyScript(txIn.SignatureScript) {
			str := fmt.Sprintf("transaction input %d: signature "+
				"script is not push only", i)
			return txRuleError(ErrNonStandard, str)
		}
	}

	// None of the output public key scripts can be a non-standard script or
	// be "dust" (except when the script is a null data script).
	numNullDataOutputs := 0
	for i, txOut := range msgTx.TxOut {
		scriptType := stdscript.DetermineScriptType(txOut.Version,
			txOut.PkScript)
		err := checkPkScriptStandard(txOut.Version, txOut.PkScript, scriptType)
		if err != nil {
			str := fmt.Sprintf("transaction output %d: %v", i, err)
			return wrapTxRuleError(ErrNonStandard, str, err)
		}

		// Accumulate the number of outputs which only carry data.  For
		// all other script types, ensure the output value is not
		// "dust".
		if scriptType == stdscript.STNullData {
			numNullDataOutputs++
		} else if txType == stake.TxTypeRegular && isDust(txOut, minRelayTxFee) {
			str := fmt.Sprintf("transaction output %d: payment "+
				"of %d is dust", i, txOut.Value)
			return txRuleError(ErrDustOutput, str)
		}
	}

	// A standard transaction must not have more than one output script that
	// only carries data. However, certain types of standard stake transactions
	// are allowed to have multiple OP_RETURN outputs, so only throw an error here
	// if the tx is TxTypeRegular.
	if numNullDataOutputs > maxNullDataOutputs && txType == stake.TxTypeRegular {
		str := "more than one transaction output in a nulldata script for a " +
			"regular type tx"
		return txRuleError(ErrNonStandard, str)
	}

	return nil
}
