// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// Contains a collection of functions that determine what type of stake tx a
// given tx is and does a cursory check for sanity.

package stake

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

// TxType indicates the type of tx (regular or stake type).
type TxType int

// Possible TxTypes.
const (
	TxTypeRegular TxType = iota
	TxTypeSStx
	TxTypeSSGen
	TxTypeSSRtx
	TxTypeTAdd
	TxTypeTSpend
	TxTypeTreasuryBase
)

const (
	// consensusVersion = txscript.consensusVersion
	consensusVersion = 0

	// MaxInputsPerSStx is the maximum number of inputs allowed in an SStx.
	MaxInputsPerSStx = 64

	// MaxOutputsPerSStx is the maximum number of outputs allowed in an SStx;
	// you need +1 for the tagged SStx output.
	MaxOutputsPerSStx = MaxInputsPerSStx*2 + 1

	// NumInputsPerSSGen is the exact number of inputs for an SSGen
	// (stakebase) tx.  Inputs are a tagged SStx output and a stakebase (null)
	// input.
	NumInputsPerSSGen = 2 // SStx and stakebase

	// MaxOutputsPerSSGen is the maximum number of outputs in an SSGen tx,
	// which are all outputs to the addresses specified in the OP_RETURNs of
	// the original SStx referenced as input plus reference and vote
	// OP_RETURN outputs in the zeroeth and first position.
	MaxOutputsPerSSGen = MaxInputsPerSStx + 2

	// NumInputsPerSSRtx is the exact number of inputs for an SSRtx (stake
	// revocation tx); the only input should be the SStx output.
	NumInputsPerSSRtx = 1

	// MaxOutputsPerSSRtx is the maximum number of outputs in an SSRtx, which
	// are all outputs to the addresses specified in the OP_RETURNs of the
	// original SStx referenced as input plus a reference to the block header
	// hash of the block in which voting was missed.
	MaxOutputsPerSSRtx = MaxInputsPerSStx

	// SStxPKHMinOutSize is the minimum size of an OP_RETURN commitment output
	// for an SStx tx.
	// 20 bytes P2SH/P2PKH + 8 byte amount + 4 byte fee range limits
	SStxPKHMinOutSize = 32

	// SStxPKHMaxOutSize is the maximum size of an OP_RETURN commitment output
	// for an SStx tx.
	SStxPKHMaxOutSize = 77

	// SSGenBlockReferenceOutSize is the size of a block reference OP_RETURN
	// output for an SSGen tx.
	SSGenBlockReferenceOutSize = 38

	// SSGenVoteBitsOutputMinSize is the minimum size for a VoteBits push
	// in an SSGen.
	SSGenVoteBitsOutputMinSize = 4

	// SSGenVoteBitsOutputMaxSize is the maximum size for a VoteBits push
	// in an SSGen.
	SSGenVoteBitsOutputMaxSize = 77

	// MaxSingleBytePushLength is the largest maximum push for an
	// SStx commitment or VoteBits push.
	MaxSingleBytePushLength = 75

	// SSGenVoteBitsExtendedMaxSize is the maximum size for a VoteBitsExtended
	// push in an SSGen.
	//
	// The final vote transaction includes a single data push for all vote
	// bits concatenated.  The non-extended vote bits occupy the first 2
	// bytes, thus the max number of extended vote bits is the maximum
	// allow length for a single byte data push minus the 2 bytes required
	// by the non-extended vote bits.
	SSGenVoteBitsExtendedMaxSize = MaxSingleBytePushLength - 2

	// SStxVoteReturnFractionMask extracts the return fraction from a
	// commitment output version.
	// If after applying this mask &0x003f is given, the entire amount of
	// the output is allowed to be spent as fees if the flag to allow fees
	// is set.
	SStxVoteReturnFractionMask = 0x003f

	// SStxRevReturnFractionMask extracts the return fraction from a
	// commitment output version.
	// If after applying this mask &0x3f00 is given, the entire amount of
	// the output is allowed to be spent as fees if the flag to allow fees
	// is set.
	SStxRevReturnFractionMask = 0x3f00

	// SStxVoteFractionFlag is a bitflag mask specifying whether or not to
	// apply a fractional limit to the amount used for fees in a vote.
	// 00000000 00000000 = No fees allowed
	// 00000000 01000000 = Apply fees rule
	SStxVoteFractionFlag = 0x0040

	// SStxRevFractionFlag is a bitflag mask specifying whether or not to
	// apply a fractional limit to the amount used for fees in a vote.
	// 00000000 00000000 = No fees allowed
	// 01000000 00000000 = Apply fees rule
	SStxRevFractionFlag = 0x4000

	// VoteConsensusVersionAbsent is the value of the consensus version
	// for a short read of the voteBits.
	VoteConsensusVersionAbsent = 0

	// MaxDataCarrierSize is the maximum number of bytes allowed in pushed
	// data in the various stake transactions.
	MaxDataCarrierSize = 256
)

var (
	// validSStxAddressOutPrefix is the valid prefix for a 30-byte
	// minimum OP_RETURN push for a commitment for an SStx.
	// Example SStx address out:
	// 0x6a (OP_RETURN)
	// 0x1e (OP_DATA_30, push length: 30 bytes)
	//
	// 0x?? 0x?? 0x?? 0x?? (20 byte public key hash)
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x??
	//
	// 0x?? 0x?? 0x?? 0x?? (8 byte amount)
	// 0x?? 0x?? 0x?? 0x??
	//
	// 0x?? 0x??           (2 byte range limits)
	validSStxAddressOutMinPrefix = []byte{txscript.OP_RETURN, txscript.OP_DATA_30}

	// validSSGenReferenceOutPrefix is the valid prefix for a block
	// reference output for an SSGen tx.
	// Example SStx address out:
	// 0x6a (OP_RETURN)
	// 0x24 (OP_DATA_36, push length: 36 bytes)
	//
	// 0x?? 0x?? 0x?? 0x?? (32 byte block header hash for the block
	// 0x?? 0x?? 0x?? 0x??   you wish to vote on)
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x?? 0x?? 0x??
	//
	// 0x?? 0x?? 0x?? 0x?? (4 byte uint32 for the height of the block
	//                      that you wish to vote on)
	validSSGenReferenceOutPrefix = []byte{txscript.OP_RETURN, txscript.OP_DATA_36}

	// validSSGenVoteOutMinPrefix is the valid prefix for a vote output for an
	// SSGen tx.
	// 0x6a (OP_RETURN)
	// 0x02 (OP_DATA_2 to OP_DATA_75, push length: 2-75 bytes)
	//
	// 0x?? 0x?? (VoteBits) ... 0x??
	validSSGenVoteOutMinPrefix = []byte{txscript.OP_RETURN, txscript.OP_DATA_2}

	// zeroHash is the zero value for a chainhash.Hash and is defined as
	// a package level variable to avoid the need to create a new instance
	// every time a check is needed.
	zeroHash = &chainhash.Hash{}
)

// VoteBits is a field representing the mandatory 2-byte field of voteBits along
// with the optional 73-byte extended field for votes.
type VoteBits struct {
	Bits         uint16
	ExtendedBits []byte
}

// VoteVersionTuple contains the extracted vote bits and version from votes
// (SSGen).
type VoteVersionTuple struct {
	Version uint32
	Bits    uint16
}

// SpentTicketsInBlock stores the hashes of the spent (both voted and revoked)
// tickets of a given block, along with the vote information.
type SpentTicketsInBlock struct {
	VotedTickets   []chainhash.Hash
	RevokedTickets []chainhash.Hash
	Votes          []VoteVersionTuple
}

// --------------------------------------------------------------------------------
// Accessory Stake Functions
// --------------------------------------------------------------------------------

// isNullOutpoint determines whether or not a previous transaction output point
// is set.
func isNullOutpoint(tx *wire.MsgTx) bool {
	nullInOP := tx.TxIn[0].PreviousOutPoint
	if nullInOP.Index == math.MaxUint32 && nullInOP.Hash.IsEqual(zeroHash) &&
		nullInOP.Tree == wire.TxTreeRegular {
		return true
	}
	return false
}

// isNullFraudProof determines whether or not a previous transaction fraud proof
// is set.
func isNullFraudProof(tx *wire.MsgTx) bool {
	txIn := tx.TxIn[0]
	switch {
	case txIn.BlockHeight != wire.NullBlockHeight:
		return false
	case txIn.BlockIndex != wire.NullBlockIndex:
		return false
	}

	return true
}

// isSmallInt returns whether or not the opcode is considered a small integer,
// which is an OP_0, or OP_1 through OP_16.
//
// NOTE: This function is only valid for version 0 opcodes.
func isSmallInt(op byte) bool {
	return op == txscript.OP_0 || (op >= txscript.OP_1 && op <= txscript.OP_16)
}

// IsNullDataScript returns whether or not the passed script is a null
// data script.
//
// NOTE: This function is only valid for version 0 scripts.  It will always
// return false for other script versions.
func IsNullDataScript(scriptVersion uint16, script []byte) bool {
	// The only supported script version is 0.
	if scriptVersion != 0 {
		return false
	}

	// A null script is of the form:
	//  OP_RETURN <optional data>
	//
	// Thus, it can either be a single OP_RETURN or an OP_RETURN followed by a
	// data push up to MaxDataCarrierSize bytes.

	// The script can't possibly be a null data script if it doesn't start
	// with OP_RETURN.  Fail fast to avoid more work below.
	if len(script) < 1 || script[0] != txscript.OP_RETURN {
		return false
	}

	// Single OP_RETURN.
	if len(script) == 1 {
		return true
	}

	// OP_RETURN followed by data push up to MaxDataCarrierSize bytes.
	tokenizer := txscript.MakeScriptTokenizer(scriptVersion, script[1:])
	return tokenizer.Next() && tokenizer.Done() &&
		(isSmallInt(tokenizer.Opcode()) ||
			tokenizer.Opcode() <= txscript.OP_PUSHDATA4) &&
		len(tokenizer.Data()) <= MaxDataCarrierSize
}

// IsStakeBase returns whether or not a tx could be considered as having a
// topically valid stake base present.
func IsStakeBase(tx *wire.MsgTx) bool {
	// A stake base (SSGen) must only have two transaction inputs.
	if len(tx.TxIn) != 2 {
		return false
	}

	// The previous output of a coin base must have a max value index and
	// a zero hash, as well as null fraud proofs.
	if !isNullOutpoint(tx) {
		return false
	}
	if !isNullFraudProof(tx) {
		return false
	}

	return true
}

// MinimalOutput is a struct encoding a minimally sized output for use in parsing
// stake related information.
type MinimalOutput struct {
	PkScript []byte
	Value    int64
	Version  uint16
}

// ConvertToMinimalOutputs converts a transaction to its minimal outputs
// derivative.
func ConvertToMinimalOutputs(tx *wire.MsgTx) []*MinimalOutput {
	minOuts := make([]*MinimalOutput, len(tx.TxOut))
	for i, txOut := range tx.TxOut {
		minOuts[i] = &MinimalOutput{
			PkScript: txOut.PkScript,
			Value:    txOut.Value,
			Version:  txOut.Version,
		}
	}

	return minOuts
}

// SStxStakeOutputInfo takes an SStx as input and scans through its outputs,
// returning the pubkeyhashs and amounts for any null data pushes (future
// commitments to stake generation rewards).
func SStxStakeOutputInfo(outs []*MinimalOutput) ([]bool, [][]byte, []int64,
	[]int64, [][]bool, [][]uint16) {

	expectedInLen := len(outs) / 2
	isP2SH := make([]bool, expectedInLen)
	addresses := make([][]byte, expectedInLen)
	amounts := make([]int64, expectedInLen)
	changeAmounts := make([]int64, expectedInLen)
	allSpendRules := make([][]bool, expectedInLen)
	allSpendLimits := make([][]uint16, expectedInLen)

	// Cycle through the inputs and pull the proportional amounts
	// and commit to PKHs/SHs.
	for idx, out := range outs {
		// We only care about the outputs where we get proportional
		// amounts and the PKHs/SHs to send rewards to, which is all
		// the odd numbered output indexes.
		if (idx > 0) && (idx%2 != 0) {
			// The MSB (sign), not used ever normally, encodes whether
			// or not it is a P2PKH or P2SH for the input.
			amtEncoded := make([]byte, 8)
			copy(amtEncoded, out.PkScript[22:30])
			isP2SH[idx/2] = !(amtEncoded[7]&(1<<7) == 0) // MSB set?
			amtEncoded[7] &= ^uint8(1 << 7)              // Clear bit

			addresses[idx/2] = out.PkScript[2:22]
			amounts[idx/2] = int64(binary.LittleEndian.Uint64(amtEncoded))

			// Get flags and restrictions for the outputs to be
			// make in either a vote or revocation.
			spendRules := make([]bool, 2)
			spendLimits := make([]uint16, 2)

			// This bitflag is true/false.
			feeLimitUint16 := binary.LittleEndian.Uint16(out.PkScript[30:32])
			spendRules[0] = (feeLimitUint16 & SStxVoteFractionFlag) ==
				SStxVoteFractionFlag
			spendRules[1] = (feeLimitUint16 & SStxRevFractionFlag) ==
				SStxRevFractionFlag
			allSpendRules[idx/2] = spendRules

			// This is the fraction to use out of 64.
			spendLimits[0] = feeLimitUint16 & SStxVoteReturnFractionMask
			spendLimits[1] = feeLimitUint16 & SStxRevReturnFractionMask
			spendLimits[1] >>= 8
			allSpendLimits[idx/2] = spendLimits
		}

		// Here we only care about the change amounts, so scan
		// the change outputs (even indices) and save their
		// amounts.
		if (idx > 0) && (idx%2 == 0) {
			changeAmounts[(idx/2)-1] = out.Value
		}
	}

	return isP2SH, addresses, amounts, changeAmounts, allSpendRules,
		allSpendLimits
}

// TxSStxStakeOutputInfo takes an SStx as input and scans through its outputs,
// returning the pubkeyhashs and amounts for any null data pushes (future
// commitments to stake generation rewards).
func TxSStxStakeOutputInfo(tx *wire.MsgTx) ([]bool, [][]byte, []int64, []int64,
	[][]bool, [][]uint16) {

	return SStxStakeOutputInfo(ConvertToMinimalOutputs(tx))
}

// AddrFromSStxPkScrCommitment extracts a P2SH or P2PKH address from a ticket
// commitment pkScript.
func AddrFromSStxPkScrCommitment(pkScript []byte, params stdaddr.AddressParams) (stdaddr.StakeAddress, error) {
	if len(pkScript) < SStxPKHMinOutSize {
		str := "short read of sstx commit pkscript"
		return nil, stakeRuleError(ErrSStxBadCommitAmount, str)
	}

	// The MSB of the encoded amount specifies if the output is P2SH.  Since
	// it is encoded with little endian, the MSB is in final byte in the encoded
	// amount.
	//
	// This is a faster equivalent of:
	//
	//	amtBytes := script[22:30]
	//	amtEncoded := binary.LittleEndian.Uint64(amtBytes)
	//	isP2SH := (amtEncoded & uint64(1<<63)) != 0
	isP2SH := pkScript[29]&0x80 != 0

	// The 20 byte PKH or SH.
	hashBytes := pkScript[2:22]

	// Return the correct address type.
	if isP2SH {
		return stdaddr.NewAddressScriptHashV0FromHash(hashBytes, params)
	}
	return stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(hashBytes, params)
}

// AmountFromSStxPkScrCommitment extracts a commitment amount from a
// ticket commitment pkScript.
func AmountFromSStxPkScrCommitment(pkScript []byte) (dcrutil.Amount, error) {
	if len(pkScript) < SStxPKHMinOutSize {
		str := "short read of sstx commit pkscript"
		return 0, stakeRuleError(ErrSStxBadCommitAmount, str)
	}

	// The MSB (sign), not used ever normally, encodes whether
	// or not it is a P2PKH or P2SH for the input.
	amtEncoded := make([]byte, 8)
	copy(amtEncoded, pkScript[22:30])
	amtEncoded[7] &= ^uint8(1 << 7) // Clear bit for P2SH flag

	return dcrutil.Amount(binary.LittleEndian.Uint64(amtEncoded)), nil
}

// SSGenBlockVotedOn takes an SSGen tx and returns the block voted on in the
// first OP_RETURN by hash and height.
//
// This function is only safe to be called on a transaction that
// has passed IsSSGen.
func SSGenBlockVotedOn(tx *wire.MsgTx) (chainhash.Hash, uint32) {
	// Get the block header hash.  Note that the actual number of bytes is
	// specified here over using chainhash.HashSize in order to statically
	// assert hash sizes have not changed.
	var blockHash [32]byte
	copy(blockHash[:], tx.TxOut[0].PkScript[2:34])

	// Get the block height.
	height := binary.LittleEndian.Uint32(tx.TxOut[0].PkScript[34:38])

	return chainhash.Hash(blockHash), height
}

// SSGenVoteBits takes an SSGen tx as input and scans through its
// outputs, returning the VoteBits of the index 1 output.
//
// This function is only safe to be called on a transaction that
// has passed IsSSGen.
func SSGenVoteBits(tx *wire.MsgTx) uint16 {
	return binary.LittleEndian.Uint16(tx.TxOut[1].PkScript[2:4])
}

// SSGenVersion takes an SSGen tx as input and returns the network
// consensus version from the VoteBits output.  If there is a short
// read, the network consensus version is considered 0 or "unset".
//
// This function is only safe to be called on a transaction that
// has passed IsSSGen.
func SSGenVersion(tx *wire.MsgTx) uint32 {
	if len(tx.TxOut[1].PkScript) < 8 {
		return VoteConsensusVersionAbsent
	}

	return binary.LittleEndian.Uint32(tx.TxOut[1].PkScript[4:8])
}

// SStxNullOutputAmounts takes an array of input amounts, change amounts, and a
// ticket purchase amount, calculates the adjusted proportion from the purchase
// amount, stores it in an array, then returns the array.  That is, for any given
// SStx, this function calculates the proportional outputs that any single user
// should receive.
// Returns: (1) Fees (2) Output Amounts (3) Error
func SStxNullOutputAmounts(amounts []int64,
	changeAmounts []int64,
	amountTicket int64) (int64, []int64, error) {
	lengthAmounts := len(amounts)

	if lengthAmounts != len(changeAmounts) {
		str := "amounts was not equal in length to change amounts!"
		return 0, nil, fmt.Errorf(str)
	}

	if amountTicket <= 0 {
		str := "committed amount was too small!"
		return 0, nil, stakeRuleError(ErrSStxBadCommitAmount, str)
	}

	contribAmounts := make([]int64, lengthAmounts)
	sum := int64(0)

	// Now we want to get the adjusted amounts.  The algorithm is like this:
	// 1 foreach amount
	// 2     subtract change from input, store
	// 3     add this amount to sum
	// 4 check sum against the total committed amount
	for i := 0; i < lengthAmounts; i++ {
		contribAmounts[i] = amounts[i] - changeAmounts[i]
		if contribAmounts[i] < 0 {
			str := fmt.Sprintf("change at idx %v spent more coins than "+
				"allowed (have: %v, spent: %v)", i, amounts[i], changeAmounts[i])
			return 0, nil, stakeRuleError(ErrSStxBadChangeAmts, str)
		}

		sum += contribAmounts[i]
	}

	fees := sum - amountTicket

	return fees, contribAmounts, nil
}

// CalculateRewards takes a list of SStx adjusted output amounts, the amount used
// to purchase that ticket, and the reward for an SSGen tx and subsequently
// generates what the outputs should be in the SSGen tx.  If used for calculating
// the outputs for an SSRtx, pass 0 for subsidy.
func CalculateRewards(amounts []int64, amountTicket int64,
	subsidy int64) []int64 {
	outputsAmounts := make([]int64, len(amounts))

	// SSGen handling
	amountWithStakebase := amountTicket + subsidy

	// Get the sum of the amounts contributed between both fees
	// and contributions to the ticket.
	totalContrib := int64(0)
	for _, amount := range amounts {
		totalContrib += amount
	}

	// Now we want to get the adjusted amounts including the reward.
	// The algorithm is like this:
	// 1 foreach amount
	// 2     amount *= 2^32
	// 3     amount /= amountTicket
	// 4     amount *= amountWithStakebase
	// 5     amount /= 2^32
	amountWithStakebaseBig := big.NewInt(amountWithStakebase)
	totalContribBig := big.NewInt(totalContrib)

	for idx, amount := range amounts {
		amountBig := big.NewInt(amount) // We need > 64 bits

		// mul amountWithStakebase
		amountBig.Mul(amountBig, amountWithStakebaseBig)

		// mul 2^32
		amountBig.Lsh(amountBig, 32)

		// div totalContrib
		amountBig.Div(amountBig, totalContribBig)

		// div 2^32
		amountBig.Rsh(amountBig, 32)

		// make int64
		outputsAmounts[idx] = amountBig.Int64()
	}

	return outputsAmounts
}

// --------------------------------------------------------------------------------
// Stake Transaction Identification Functions
// --------------------------------------------------------------------------------

// CheckSStx returns an error if a transaction is not a stake submission
// transaction.  It does some simple validation steps to make sure the number of
// inputs, number of outputs, and the input/output scripts are valid.
//
// SStx transactions are specified as below.
// Inputs:
// untagged output 1 [index 0]
// untagged output 2 [index 1]
// ...
// untagged output MaxInputsPerSStx [index MaxInputsPerSStx-1]
//
// Outputs:
// OP_SSTX tagged output [index 0]
// OP_RETURN push of input 1's address for reward receiving [index 1]
// OP_SSTXCHANGE tagged output for input 1 [index 2]
// OP_RETURN push of input 2's address for reward receiving [index 3]
// OP_SSTXCHANGE tagged output for input 2 [index 4]
// ...
// OP_RETURN push of input MaxInputsPerSStx's address for reward receiving
//     [index (MaxInputsPerSStx*2)-2]
// OP_SSTXCHANGE tagged output [index (MaxInputsPerSStx*2)-1]
//
// The output OP_RETURN pushes should be of size 20 bytes (standard address).
func CheckSStx(tx *wire.MsgTx) error {
	// Check to make sure there aren't too many inputs.
	// CheckTransactionSanity already makes sure that number of inputs is
	// greater than 0, so no need to check that.
	if len(tx.TxIn) > MaxInputsPerSStx {
		str := "SStx has too many inputs"
		return stakeRuleError(ErrSStxTooManyInputs, str)
	}

	// Check to make sure there aren't too many outputs.
	if len(tx.TxOut) > MaxOutputsPerSStx {
		str := "SStx has too many outputs"
		return stakeRuleError(ErrSStxTooManyOutputs, str)
	}

	// Check to make sure there are some outputs.
	if len(tx.TxOut) == 0 {
		str := "SStx has no outputs"
		return stakeRuleError(ErrSStxNoOutputs, str)
	}

	// Check to make sure that all output scripts are the consensus version.
	for idx, txOut := range tx.TxOut {
		if txOut.Version != consensusVersion {
			str := fmt.Sprintf("invalid script version found in "+
				"txOut idx %v", idx)
			return stakeRuleError(ErrSStxInvalidOutputs, str)
		}
	}

	// Ensure that the first output is tagged OP_SSTX.
	if !IsTicketPurchaseScript(tx.TxOut[0].Version, tx.TxOut[0].PkScript) {
		str := "first SStx output should have been OP_SSTX tagged, " +
			"but it was not"
		return stakeRuleError(ErrSStxInvalidOutputs, str)
	}

	// Ensure that the number of outputs is equal to the number of inputs
	// + 1.
	if (len(tx.TxIn)*2 + 1) != len(tx.TxOut) {
		str := "the number of inputs in the SStx tx was not the number " +
			"of outputs/2 - 1"
		return stakeRuleError(ErrSStxInOutProportions, str)
	}

	// Ensure that the rest of the odd outputs are 28-byte OP_RETURN pushes that
	// contain putative pubkeyhashes, and that the rest of the odd outputs are
	// OP_SSTXCHANGE tagged.
	for outTxIndex := 1; outTxIndex < len(tx.TxOut); outTxIndex++ {
		scrVersion := tx.TxOut[outTxIndex].Version
		rawScript := tx.TxOut[outTxIndex].PkScript

		// Check change outputs.
		if outTxIndex%2 == 0 {
			if !IsStakeChangeScript(scrVersion, rawScript) {
				str := fmt.Sprintf("SStx output at output index %d was not "+
					"an sstx change output", outTxIndex)
				return stakeRuleError(ErrSStxInvalidOutputs, str)
			}
			continue
		}

		// Else (odd) check commitment outputs.  The script should be a
		// null data output.
		if !IsNullDataScript(scrVersion, rawScript) {
			str := fmt.Sprintf("SStx output at output index %d was not "+
				"a null data (OP_RETURN) push", outTxIndex)
			return stakeRuleError(ErrSStxInvalidOutputs, str)
		}

		// The length of the output script should be between 32 and 77 bytes long.
		if len(rawScript) < SStxPKHMinOutSize ||
			len(rawScript) > SStxPKHMaxOutSize {
			str := fmt.Sprintf("SStx output at output index %d was a "+
				"null data (OP_RETURN) push of the wrong size", outTxIndex)
			return stakeRuleError(ErrSStxInvalidOutputs, str)
		}

		// The OP_RETURN output script prefix should conform to the standard.
		outputScriptBuffer := bytes.NewBuffer(rawScript)
		outputScriptPrefix := outputScriptBuffer.Next(2)

		minPush := validSStxAddressOutMinPrefix[1]
		maxPush := validSStxAddressOutMinPrefix[1] +
			(MaxSingleBytePushLength - minPush)
		pushLen := outputScriptPrefix[1]
		pushLengthValid := (pushLen >= minPush) && (pushLen <= maxPush)
		// The first byte should be OP_RETURN, while the second byte should be a
		// valid push length.
		if !(outputScriptPrefix[0] == validSStxAddressOutMinPrefix[0]) ||
			!pushLengthValid {
			str := fmt.Sprintf("sstx commitment at output idx %v had "+
				"an invalid prefix", outTxIndex)
			return stakeRuleError(ErrSStxInvalidOutputs, str)
		}
	}

	return nil
}

// IsSStx returns whether or not a transaction is a stake submission transaction.
// These are also known as tickets.
func IsSStx(tx *wire.MsgTx) bool {
	return CheckSStx(tx) == nil
}

// TreasuryVoteT is the type that designates a treasury vote. There are two
// valid bits that may be set, although not simultaneously. 0x01 and 0x02. Any
// other (or lack therefore) bits are considered invalid.
type TreasuryVoteT byte

const (
	TreasuryVoteInvalid TreasuryVoteT = 0x00 // Invalid vote
	TreasuryVoteYes     TreasuryVoteT = 0x01 // Vote YES
	TreasuryVoteNo      TreasuryVoteT = 0x02 // Vote NO
)

// CheckTreasuryVote ensures that the provided treasury vote is valid. If the
// vote is valid the proper value is returned.
func CheckTreasuryVote(vote TreasuryVoteT) (TreasuryVoteT, error) {
	switch vote {
	case TreasuryVoteYes:
	case TreasuryVoteNo:
	default:
		return TreasuryVoteInvalid,
			fmt.Errorf("invalid treasury vote: 0x%x", vote)
	}
	return vote, nil
}

// IsTreasuryVote returns true if the provided vote is valid.
func IsTreasuryVote(vote TreasuryVoteT) bool {
	_, err := CheckTreasuryVote(vote)
	return err == nil
}

// TreasuryVoteTuple is a tuple that groups a TSpend hash and its associated
// vote bits.
type TreasuryVoteTuple struct {
	Hash chainhash.Hash
	Vote TreasuryVoteT
}

// GetSSGenTreasuryVotes pulls out treasury votes for TSpend transactions. This
// function is a convenience function to discourage rolling new versions that
// pull out votes and may be therefore different from consensus.
// This function verifies that the passed in script is of the following form:
// OP_RETURN DATA_PUSH []byte{'T', 'V'} N-votes
// and returns a slice of hashes which represent the TSpend transactions they
// are supposed to vote on.
func GetSSGenTreasuryVotes(PkScript []byte) ([]TreasuryVoteTuple, error) {
	// Verify that there is enough length to contain a discriminator.
	// Expect at least: OP_RETURN DATA_PUSH Y Z
	// The final check that is parenthesis is to prevent crashing when
	// using an illegal script.  This cannot be hit but it is verified
	// anyway.
	if len(PkScript) < 4 || PkScript[1] > txscript.OP_PUSHDATA1 {
		str := fmt.Sprintf("final output of a SSGen does not " +
			"contain a valid type discriminator")
		return nil, stakeRuleError(ErrSSGenInvalidDiscriminatorLength,
			str)
	}

	// Determine start of discriminator based on the opcode in [1].
	start := 2
	if PkScript[1] == txscript.OP_PUSHDATA1 {
		start = 3
	}

	if start+2 > len(PkScript) {
		str := fmt.Sprintf("final output of a SSGen is not a valid " +
			"nullscript")
		return nil, stakeRuleError(ErrSSGenInvalidNullScript,
			str)
	}

	// Ensure discriminator is TV.
	if !bytes.Equal(PkScript[start:start+2], []byte{'T', 'V'}) {
		str := fmt.Sprintf("last SSGen unknown type discriminator: "+
			"0x%x 0x%x", PkScript[start], PkScript[start+1])
		return nil, stakeRuleError(ErrSSGenUnknownDiscriminator, str)
	}

	// Since this is a 'T','V' we expect N hashes and their vote bits.
	const size = chainhash.HashSize + 1
	if len(PkScript[start+2:]) < size ||
		len(PkScript[start+2:])%size != 0 {
		str := fmt.Sprintf("SSGen 'T','V' invalid " +
			"length")
		return nil, stakeRuleError(ErrSSGenInvalidTVLength,
			str)
	}

	// Return hashes, this is the success path.
	const maxVotes = 7
	votes := make([]TreasuryVoteTuple, 0, maxVotes)
	vmap := make(map[chainhash.Hash]struct{}, maxVotes) // Collision detection
	for i := start + 2; ; i += size {

		if len(PkScript[i:]) < size {
			break
		}
		var hash chainhash.Hash
		copy(hash[:], PkScript[i:size+i-1])
		vote := TreasuryVoteT(PkScript[size+i-1])
		if !IsTreasuryVote(vote) {
			str := fmt.Sprintf("SSGen invalid treasury "+
				"vote bits 0x%0x", vote)
			return nil,
				stakeRuleError(ErrSSGenInvalidTreasuryVote,
					str)
		}

		// Ensure there are no duplicate TSpend votes.
		if _, ok := vmap[hash]; ok {
			str := fmt.Sprintf("SSGen duplicate treasury "+
				"vote %v", hash)
			return nil,
				stakeRuleError(ErrSSGenDuplicateTreasuryVote,
					str)
		}
		vmap[hash] = struct{}{}

		// Store votes in order of appearance.
		votes = append(votes, TreasuryVoteTuple{
			Hash: hash,
			Vote: vote,
		})
	}
	return votes, nil
}

// CheckSSGenVotes returns an error if a transaction is not a stake submission
// generation transaction.  It does some simple validation steps to make sure
// the number of inputs, number of outputs, and the input/output scripts are
// valid. In addition it returns treasury votes to avoid subsequent expensive
// calls.
//
// This does NOT check to see if the subsidy is valid or whether or not the
// value of input[0] + subsidy = value of the outputs.
//
// SSGen transactions are specified as below.
// Inputs:
// Stakebase null input [index 0]
// SStx-tagged output [index 1]
//
// Outputs:
// OP_RETURN push of 40 bytes containing: [index 0]
//     i. 32-byte block header of block being voted on.
//     ii. 8-byte int of this block's height.
// OP_RETURN push of 2 bytes containing votebits [index 1]
// SSGen-tagged output to address from SStx-tagged output's tx index output 1
//     [index 2]
// SSGen-tagged output to address from SStx-tagged output's tx index output 2
//     [index 3]
// ...
// SSGen-tagged output to address from SStx-tagged output's tx index output
//     MaxInputsPerSStx [index MaxOutputsPerSSgen - 1]
// OP_RETURN push of 2 bytes containing opcode designating what the remaining
// data that is pushed is.
// In the case of 'TV` (Treasury Vote) it checks for a <hash><vote> tuple.
// For example: OP_RETURN OP_DATA_X 'T','V' <N hashvote_tuple>
func CheckSSGenVotes(tx *wire.MsgTx, isTreasuryEnabled bool) ([]TreasuryVoteTuple, error) {
	// Check to make sure there aren't too many inputs.
	if len(tx.TxIn) != NumInputsPerSSGen {
		str := "SSgen tx has an invalid number of inputs"
		return nil, stakeRuleError(ErrSSGenWrongNumInputs, str)
	}

	// Check to make sure there aren't too many outputs.
	if len(tx.TxOut) > MaxOutputsPerSSGen {
		str := "SSgen tx has too many outputs"
		return nil, stakeRuleError(ErrSSGenTooManyOutputs, str)
	}

	// Check to make sure there are enough outputs.
	if len(tx.TxOut) < 2 {
		str := "SSgen tx does not have enough outputs"
		return nil, stakeRuleError(ErrSSGenNoOutputs, str)
	}

	// Ensure that the first input is a stake base null input.
	// Also checks to make sure that there aren't too many or too few
	// inputs.
	if !IsStakeBase(tx) {
		str := "SSGen tx did not include a stakebase in the zeroeth " +
			"input position"
		return nil, stakeRuleError(ErrSSGenNoStakebase, str)
	}

	// Check to make sure that the output used as input came from
	// TxTreeStake.
	for i, txin := range tx.TxIn {
		// Skip the stakebase
		if i == 0 {
			continue
		}

		if txin.PreviousOutPoint.Index != 0 {
			str := fmt.Sprintf("SSGen used an invalid input idx (got %v, "+
				"want 0)", txin.PreviousOutPoint.Index)
			return nil, stakeRuleError(ErrSSGenWrongIndex, str)
		}

		if txin.PreviousOutPoint.Tree != wire.TxTreeStake {
			str := "SSGen used a non-stake input"
			return nil, stakeRuleError(ErrSSGenWrongTxTree, str)
		}
	}

	// Check to make sure that all output scripts are the consensus version.
	for _, txOut := range tx.TxOut {
		if txOut.Version != consensusVersion {
			str := "invalid script version found in txOut"
			return nil, stakeRuleError(ErrSSGenBadGenOuts, str)
		}
	}

	// Ensure the number of outputs is equal to the number of inputs found
	// in the original SStx + 3.
	// TODO: Do this in validate, requires DB and valid chain.

	// Ensure that the second input is an SStx tagged output.
	// TODO: Do this in validate, as we don't want to actually lookup
	// old tx here.  This function is for more general sorting.

	// Ensure that the first output is an OP_RETURN push.
	zeroethOutputVersion := tx.TxOut[0].Version
	zeroethOutputScript := tx.TxOut[0].PkScript
	if !IsNullDataScript(zeroethOutputVersion, zeroethOutputScript) {
		str := "first SSGen output should have been an OP_RETURN " +
			"data push, but was not"
		return nil, stakeRuleError(ErrSSGenNoReference, str)
	}

	// Ensure that the first output is the correct size.
	if len(zeroethOutputScript) != SSGenBlockReferenceOutSize {
		str := fmt.Sprintf("first SSGen output has invalid an length (got %d, "+
			"want %d)", len(zeroethOutputScript), SSGenBlockReferenceOutSize)
		return nil, stakeRuleError(ErrSSGenBadReference, str)
	}

	// The OP_RETURN output script prefix for block referencing should
	// conform to the standard.
	zeroethOutputScriptBuffer := bytes.NewBuffer(zeroethOutputScript)

	zeroethOutputScriptPrefix := zeroethOutputScriptBuffer.Next(2)
	if !bytes.Equal(zeroethOutputScriptPrefix,
		validSSGenReferenceOutPrefix) {
		str := "first SSGen output had an invalid prefix"
		return nil, stakeRuleError(ErrSSGenBadReference, str)
	}

	// Ensure that the block header hash given in the first 32 bytes of the
	// OP_RETURN push is a valid block header and found in the main chain.
	// TODO: This is validate level stuff, do this there.

	// Ensure that the second output is an OP_RETURN push.
	firstOutputVersion := tx.TxOut[1].Version
	firstOutputScript := tx.TxOut[1].PkScript
	if !IsNullDataScript(firstOutputVersion, firstOutputScript) {
		str := "second SSGen output should have been an OP_RETURN " +
			"data push, but was not"
		return nil, stakeRuleError(ErrSSGenNoVotePush, str)
	}

	// The length of the output script should be between 4 and 77 bytes
	// long.
	if len(firstOutputScript) < SSGenVoteBitsOutputMinSize ||
		len(firstOutputScript) > SSGenVoteBitsOutputMaxSize {
		str := fmt.Sprintf("SSGen votebits output at output index 1 " +
			"was a NullData (OP_RETURN) push of the wrong size")
		return nil, stakeRuleError(ErrSSGenBadVotePush, str)
	}

	// The OP_RETURN output script prefix for voting should conform to the
	// standard.
	firstOutputScriptBuffer := bytes.NewBuffer(firstOutputScript)
	firstOutputScriptPrefix := firstOutputScriptBuffer.Next(2)

	minPush := validSSGenVoteOutMinPrefix[1]
	maxPush := validSSGenVoteOutMinPrefix[1] +
		(MaxSingleBytePushLength - minPush)
	pushLen := firstOutputScriptPrefix[1]
	pushLengthValid := (pushLen >= minPush) && (pushLen <= maxPush)
	// The first byte should be OP_RETURN, while the second byte should be a
	// valid push length.
	if !(firstOutputScriptPrefix[0] == validSSGenVoteOutMinPrefix[0]) ||
		!pushLengthValid {
		str := "second SSGen output had an invalid prefix"
		return nil, stakeRuleError(ErrSSGenBadVotePush, str)
	}

	// Check to see if the last output is an OP_RETURN followed by a 2 byte
	// data push that designates what the data is that follows the
	// discriminator. In the case of 'T','V' the next data push should be N
	// hashes. If it is we need to decrease the count on OP_SSGEN tests by
	// one. This check is only valid if the treasury agenda is active.
	txOutLen := len(tx.TxOut)
	lastTxOut := tx.TxOut[len(tx.TxOut)-1]
	var votes []TreasuryVoteTuple
	if isTreasuryEnabled && IsNullDataScript(lastTxOut.Version,
		lastTxOut.PkScript) {

		txOutLen--

		// We call this function in order to prevent rolling of
		// additional functions that may not conform 100% to consensus.
		var err error
		votes, err = GetSSGenTreasuryVotes(lastTxOut.PkScript)
		if err != nil {
			return nil, err
		}

		// If there are votes the TxVersion must be TxVersionTreasury.
		// This test is done late in order to allow older versions
		// SSGen if there are no votes.
		if !(len(votes) > 0 && tx.Version == wire.TxVersionTreasury) {
			str := fmt.Sprintf("SSGen invalid tx version %v",
				tx.Version)
			return nil, stakeRuleError(ErrSSGenInvalidTxVersion,
				str)
		}
	}

	// Ensure that the tx height given in the last 8 bytes is StakeMaturity
	// many blocks ahead of the block in which that SStx appears, otherwise
	// this ticket has failed to mature and the SStx must be invalid.
	// TODO: This is validate level stuff, do this there.

	// Ensure that the remaining outputs are OP_SSGEN tagged.
	for outTxIndex := 2; outTxIndex < txOutLen; outTxIndex++ {
		scrVersion := tx.TxOut[outTxIndex].Version
		rawScript := tx.TxOut[outTxIndex].PkScript

		// The script should be a OP_SSGEN tagged output.
		if !IsVoteScript(scrVersion, rawScript) {
			str := fmt.Sprintf("SSGen tx output at output "+
				"index %d was not an OP_SSGEN tagged output",
				outTxIndex)
			return nil, stakeRuleError(ErrSSGenBadGenOuts, str)
		}
	}

	return votes, nil
}

// CheckSSGen wraps CheckSSGenVotes (which is the old CheckSSGen plus it
// returns TSpend votes if there are any) to maintain consistency and backwards
// compatibility.
func CheckSSGen(tx *wire.MsgTx, isTreasuryEnabled bool) error {
	_, err := CheckSSGenVotes(tx, isTreasuryEnabled)
	return err
}

// IsSSGen returns whether or not a transaction is a stake submission generation
// transaction.  These are also known as votes.
func IsSSGen(tx *wire.MsgTx, isTreasuryEnabled bool) bool {
	return CheckSSGen(tx, isTreasuryEnabled) == nil
}

// CheckSSRtx returns an error if a transaction is not a stake submission
// revocation transaction.  It does some simple validation steps to make sure
// the number of inputs, number of outputs, and the input/output scripts are
// valid.
//
// SSRtx transactions are specified as below.
// Inputs:
// SStx-tagged output [index 0]
//
// Outputs:
// SSGen-tagged output to address from SStx-tagged output's tx index output 1
//     [index 0]
// SSGen-tagged output to address from SStx-tagged output's tx index output 2
//     [index 1]
// ...
// SSGen-tagged output to address from SStx-tagged output's tx index output
//     MaxInputsPerSStx [index MaxOutputsPerSSRtx - 1]
func CheckSSRtx(tx *wire.MsgTx) error {
	// Check to make sure there is the correct number of inputs.
	if len(tx.TxIn) != NumInputsPerSSRtx {
		str := "SSRtx has an invalid number of inputs"
		return stakeRuleError(ErrSSRtxWrongNumInputs, str)
	}

	// Check to make sure there aren't too many outputs.
	if len(tx.TxOut) > MaxOutputsPerSSRtx {
		str := "SSRtx has too many outputs"
		return stakeRuleError(ErrSSRtxTooManyOutputs, str)
	}

	// Check to make sure there are some outputs.
	if len(tx.TxOut) == 0 {
		str := "SSRtx has no outputs"
		return stakeRuleError(ErrSSRtxNoOutputs, str)
	}

	// Check to make sure that all output scripts are the consensus version.
	for _, txOut := range tx.TxOut {
		if txOut.Version != consensusVersion {
			str := "invalid script version found in txOut"
			return stakeRuleError(ErrSSRtxBadOuts, str)
		}
	}

	// Check to make sure that the output used as input came from TxTreeStake.
	for _, txin := range tx.TxIn {
		if txin.PreviousOutPoint.Tree != wire.TxTreeStake {
			str := "SSRtx used a non-stake input"
			return stakeRuleError(ErrSSRtxWrongTxTree, str)
		}
	}

	// Ensure that the first input is an SStx tagged output.
	// TODO: Do this in validate, needs a DB and chain.

	// Ensure that the tx height given in the last 8 bytes is StakeMaturity
	// many blocks ahead of the block in which that SStx appear, otherwise
	// this ticket has failed to mature and the SStx must be invalid.
	// TODO: Do this in validate, needs a DB and chain.

	// Ensure that the outputs are OP_SSRTX tagged.
	// Ensure that the tx height given in the last 8 bytes is StakeMaturity
	// many blocks ahead of the block in which that SStx appear, otherwise
	// this ticket has failed to mature and the SStx must be invalid.
	// TODO: This is validate level stuff, do this there.

	// Ensure that the outputs are OP_SSRTX tagged.
	for outTxIndex := 0; outTxIndex < len(tx.TxOut); outTxIndex++ {
		scrVersion := tx.TxOut[outTxIndex].Version
		rawScript := tx.TxOut[outTxIndex].PkScript

		// The script should be a OP_SSRTX tagged output.
		if !IsRevocationScript(scrVersion, rawScript) {
			str := fmt.Sprintf("SSRtx output at output index %d was not "+
				"an OP_SSRTX tagged output", outTxIndex)
			return stakeRuleError(ErrSSRtxBadOuts, str)
		}
	}

	// Ensure the number of outputs is equal to the number of inputs found in
	// the original SStx.
	// TODO: Do this in validate, needs a DB and chain.

	return nil
}

// IsSSRtx returns whether or not a transaction is a stake submission revocation
// transaction.  These are also known as revocations.
func IsSSRtx(tx *wire.MsgTx) bool {
	return CheckSSRtx(tx) == nil
}

// DetermineTxType determines the type of stake transaction a transaction is; if
// none, it returns that it is an assumed regular tx.
func DetermineTxType(tx *wire.MsgTx, isTreasuryEnabled bool) TxType {
	if IsSStx(tx) {
		return TxTypeSStx
	}
	if IsSSGen(tx, isTreasuryEnabled) {
		return TxTypeSSGen
	}
	if IsSSRtx(tx) {
		return TxTypeSSRtx
	}
	if isTreasuryEnabled {
		if IsTAdd(tx) {
			return TxTypeTAdd
		}
		if IsTSpend(tx) {
			return TxTypeTSpend
		}
		if IsTreasuryBase(tx) {
			return TxTypeTreasuryBase
		}
	}
	return TxTypeRegular
}

// IsStakeSubmissionTxOut indicates whether the txOut identified by the
// given index is a stake submission output. Stake Submission outputs are
// the odd-numbered outputs of an SStx transaction.
//
// This function is only safe to be called on a transaction that
// has passed IsSStx.
func IsStakeSubmissionTxOut(index int) bool {
	return (index % 2) != 0
}

// FindSpentTicketsInBlock returns information about tickets spent in a given
// block. This includes voted and revoked tickets, and the vote bits of each
// spent ticket. This is faster than calling the individual functions to
// determine ticket state if all information regarding spent tickets is needed.
//
// Note that the returned hashes are of the originally purchased *tickets* and
// **NOT** of the vote/revoke transaction.
//
// The tickets are determined **only** from the STransactions of the provided
// block and no validation is performed.
//
// This function is only safe to be called with a block that has previously
// had all header commitments validated.
func FindSpentTicketsInBlock(block *wire.MsgBlock) *SpentTicketsInBlock {
	votes := make([]VoteVersionTuple, 0, block.Header.Voters)
	voters := make([]chainhash.Hash, 0, block.Header.Voters)
	revocations := make([]chainhash.Hash, 0, block.Header.Revocations)

	for _, stx := range block.STransactions {
		// This can only be called with treasury disabled.
		const noTreasury = false
		if IsSSGen(stx, noTreasury) {
			voters = append(voters, stx.TxIn[1].PreviousOutPoint.Hash)
			votes = append(votes, VoteVersionTuple{
				Version: SSGenVersion(stx),
				Bits:    SSGenVoteBits(stx),
			})
			continue
		}
		if IsSSRtx(stx) {
			revocations = append(revocations, stx.TxIn[0].PreviousOutPoint.Hash)
			continue
		}
	}
	return &SpentTicketsInBlock{
		VotedTickets:   voters,
		Votes:          votes,
		RevokedTickets: revocations,
	}
}
