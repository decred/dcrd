// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package blockcf2 provides functions for building committed filters for blocks
using Golomb-coded sets in a way that is useful for light clients such as SPV
wallets.

Committed filters are a reversal of how bloom filters are typically used by a
light client: a consensus-validating full node commits to filters for every
block with a predetermined collision probability and light clients match against
the filters locally rather than uploading personal data to other nodes.  If a
filter matches, the light client should fetch the entire block and further
inspect it for relevant transactions.
*/
package blockcf2

import (
	"fmt"

	"github.com/decred/dcrd/blockchain/stake/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/gcs/v2"
	"github.com/decred/dcrd/txscript/v2"
	"github.com/decred/dcrd/wire"
)

const (
	// B is bits parameter for constructing GCS filters and results in the
	// tunable parameter that is essentially used as the bin size in the
	// underlying Golomb coding having a value of 2^B.
	B = 19

	// M is the inverse of the target false positive rate for constructing GCS
	// filters.  This is the optimal value of M to minimize the size of the
	// filter for B = 19.
	M = 784931
)

const (
	// A ticket commitment output is an OP_RETURN script with a 30-byte data
	// push that consists of a 20-byte hash for the payment hash, 8 bytes
	// for the amount to commit to (with the upper bit flag set to indicate
	// the hash is for a pay-to-script-hash address, otherwise the hash is a
	// pay-to-pubkey-hash), and 2 bytes for the fee limits.  Thus, 1 byte
	// for the OP_RETURN + 1 byte for the data push + 20 bytes for the
	// payment hash means the encoded amount is at offset 22.  Then, 8 bytes
	// for the amount means the encoded fee limits are at offset 30.
	commitHashStartIdx   = 2
	commitHashEndIdx     = commitHashStartIdx + 20
	commitAmountStartIdx = commitHashEndIdx
	commitAmountEndIdx   = commitAmountStartIdx + 8
)

// PrevScriptError represents an error when looking up a previous output script.
// The caller can use type assertions to determine the specific details of the
// previous outpoint and the transaction input that references it.
type PrevScriptError struct {
	PrevOut wire.OutPoint
	TxHash  chainhash.Hash
	TxInIdx int
}

// Error returns the previous script error as a human-readable string and
// satisfies the error interface.
func (e PrevScriptError) Error() string {
	return fmt.Sprintf("unable to find output script %s:%d referenced by %s:%d",
		e.PrevOut, e.PrevOut.Tree, e.TxHash, e.TxInIdx)
}

// Entries describes all of the filter entries used to create a GCS filter and
// provides methods for appending data structures found in blocks.
type Entries [][]byte

// AddRegularPkScript adds the regular tx output script to an entries slice.
// Empty scripts are ignored.
func (e *Entries) AddRegularPkScript(script []byte) {
	if len(script) == 0 {
		return
	}
	*e = append(*e, script)
}

// AddStakePkScript adds the output script without the stake opcode tag to an
// entries slice.  Empty scripts are ignored.
func (e *Entries) AddStakePkScript(script []byte) {
	if len(script) == 0 {
		return
	}
	*e = append(*e, script[1:])
}

// Key creates a block filter key by truncating the Merkle root of a block to
// the key size.
func Key(merkleRoot *chainhash.Hash) [gcs.KeySize]byte {
	var key [gcs.KeySize]byte
	copy(key[:], merkleRoot[:])
	return key
}

// isStakeOutput returns true if a script output is a stake type.
func isStakeOutput(scriptVersion uint16, pkScript []byte) bool {
	class := txscript.GetScriptClass(scriptVersion, pkScript)
	return class == txscript.StakeSubmissionTy ||
		class == txscript.StakeGenTy ||
		class == txscript.StakeRevocationTy ||
		class == txscript.StakeSubChangeTy
}

// extractTicketCommitHash extracts the commitment hash from a ticket output
// commitment script.
//
// NOTE: The caller MUST have already determined that the provided script is
// a commitment output script or the function may panic.
func extractTicketCommitHash(script []byte) []byte {
	return script[commitHashStartIdx:commitHashEndIdx]
}

// isTicketCommitP2SH returns whether or not the passed ticket output commitment
// script commits to a hash which represents a pay-to-script-hash output.  When
// false, it commits to a hash which represents a pay-to-pubkey-hash output.
//
// NOTE: The caller MUST have already determined that the provided script is
// a commitment output script or the function may panic.
func isTicketCommitP2SH(script []byte) bool {
	// The MSB of the encoded amount specifies if the output is P2SH.  Since
	// it is encoded with little endian, the MSB is in final byte in the encoded
	// amount.
	//
	// This is a faster equivalent of:
	//
	//	amtBytes := script[commitAmountStartIdx:commitAmountEndIdx]
	//	amtEncoded := binary.LittleEndian.Uint64(amtBytes)
	//	return (amtEncoded & commitP2SHFlag) != 0
	//
	return script[commitAmountEndIdx-1]&0x80 != 0
}

// commitmentConverter provides the ability to convert the commitment output
// script of a ticket purchase to either a pay-to-pubkey-hash or a
// pay-to-script-hash script according to the commitment while making use of a
// single backing array that helps reduce the number of allocations necessary
// to make the conversion.
type commitmentConverter struct {
	allScripts []byte
}

// commitmentToPaymentScript converts a commitment output script of a ticket
// purchase to either a pay-to-pubkey-hash or a pay-to-script-hash script
// according to the commitment.
func (c *commitmentConverter) paymentScript(commitmentScript []byte) []byte {
	commitmentHash := extractTicketCommitHash(commitmentScript)
	if isTicketCommitP2SH(commitmentScript) {
		// A pay-to-script-hash script is of the form:
		//  OP_HASH160 <20-byte hash> OP_EQUAL
		startIdx := len(c.allScripts)
		c.allScripts = append(c.allScripts, txscript.OP_HASH160,
			txscript.OP_DATA_20)
		c.allScripts = append(c.allScripts, commitmentHash...)
		c.allScripts = append(c.allScripts, txscript.OP_EQUAL)
		endIdx := len(c.allScripts)
		return c.allScripts[startIdx:endIdx:endIdx]
	}

	// A pay-to-pubkey-hash script for ecdsa+secp256k1 is of the form:
	//  OP_DUP OP_HASH160 <20-byte hash> OP_EQUALVERIFY OP_CHECKSIG
	startIdx := len(c.allScripts)
	c.allScripts = append(c.allScripts, txscript.OP_DUP, txscript.OP_HASH160,
		txscript.OP_DATA_20)
	c.allScripts = append(c.allScripts, commitmentHash...)
	c.allScripts = append(c.allScripts, txscript.OP_EQUALVERIFY,
		txscript.OP_CHECKSIG)
	endIdx := len(c.allScripts)
	return c.allScripts[startIdx:endIdx:endIdx]
}

// makeCommitmentConverter returns a commitment converter with a pre-allocated
// backing array for performing the conversions based on the provided number
// of tickets.
func makeCommitmentConverter(numTickets uint8) commitmentConverter {
	// The vast majority of ticket purchases only contain commitments to a
	// couple of outputs and p2pkh is the larger of the possible converted
	// scripts, so use that information to calculate a reasonable size hint for
	// all of the converted commitment scripts.
	const p2pkhScriptLen = 25
	sizeHint := int32(numTickets) * p2pkhScriptLen * 2
	return commitmentConverter{
		allScripts: make([]byte, 0, sizeHint),
	}
}

// excludeFromFilter returns whether the passed script version and public key
// script combination should be excluded from filters.  Scripts that are empty
// or larger than the max allowed length and all script versions other than 0
// are excluded.
func excludeFromFilter(scriptVersion uint16, script []byte) bool {
	return scriptVersion != 0 || len(script) == 0 || len(script) >
		txscript.MaxScriptSize
}

// PrevScripter defines an interface that provides access to scripts and their
// associated version keyed by an outpoint.  It is used within this package as a
// generic means to provide the scripts referenced by all of the inputs to
// transactions within a block that are needed to construct a filter.  The
// boolean return indicates whether or not the script and version for the
// provided outpoint was found.
type PrevScripter interface {
	PrevScript(*wire.OutPoint) (uint16, []byte, bool)
}

// Regular builds a GCS filter from a block and the previous output scripts it
// references as inputs.  The filter will be keyed by the merkle root of the
// block.
//
// The following section describes the items that will be added to the filter,
// however, there are a few special cases that apply:
//
// - Scripts that are not version 0, empty, or otherwise provably unspendable
//   are NOT included
// - Output scripts for transactions in the stake tree do NOT include the
//   initial stake opcode tag (OP_SS*)
//   - This allows users of the filter to only match against a normal P2PKH or
//     P2SH script, instead of many extra matches for each tag
//
// Considering the aforementioned exceptions, the filter will contain the
// following items for transactions in the regular tree:
// - Previous output scripts referenced by the transaction inputs with the
//   exception of the coinbase
// - Output scripts in the transaction outputs
//
// In addition, also considering the aforementioned exceptions, the filter will
// contain the following items for transactions in the stake tree:
// - For ticket purchases:
//   - Previous output scripts referenced by the transaction inputs
//   - Commitment output scripts (converted from the commitment hash and type)
//   - Change output scripts except those that are provably unspendable
// - For votes:
//   - Subsidy generation output scripts
//   - Output scripts that pay the original ticket commitments
// - For revocations:
//   - Output scripts that pay the original ticket commitments
func Regular(block *wire.MsgBlock, prevScripts PrevScripter) (*gcs.FilterV2, error) {
	// There will typically be data entries for at least one output and one
	// input per regular transaction in the block, excepting the coinbase, and
	// an average of two per stake transaction, though stake transactions vary
	// wildly.
	//
	// In practice, most transactions will have more inputs and outputs than
	// this value, but pre-allocate enough space in the backing array for a
	// reasonable minimum value to reduce the number of allocations.
	numEntriesHint := len(block.Transactions)*2 + len(block.STransactions)
	data := make(Entries, 0, numEntriesHint)

	// For regular transactions, add all referenced previous output scripts,
	// except the coinbase, and all output scripts.
	for i, tx := range block.Transactions {
		for _, txOut := range tx.TxOut {
			// Don't add scripts that are empty, larger than the max allowed
			// length, or for an unsupported script version.  Notice that
			// provably unspendable OP_RETURN scripts are included.
			if excludeFromFilter(txOut.Version, txOut.PkScript) {
				continue
			}

			data.AddRegularPkScript(txOut.PkScript)
		}

		// Skip coinbase since it does not have any inputs.
		if i == 0 {
			continue
		}

		for txInIdx, txIn := range tx.TxIn {
			prevOut := &txIn.PreviousOutPoint
			scriptVer, prevOutScript, ok := prevScripts.PrevScript(prevOut)
			if !ok {
				return nil, PrevScriptError{
					PrevOut: *prevOut,
					TxHash:  tx.TxHash(),
					TxInIdx: txInIdx,
				}
			}

			// Don't add scripts that are empty, larger than the max allowed
			// length, or for an unsupported script version.
			if excludeFromFilter(scriptVer, prevOutScript) {
				continue
			}

			// Add the script to be included in the filter while stripping the
			// initial stake opcode for those in the stake tree.
			isStakeTree := prevOut.Tree == wire.TxTreeStake
			isStakeOut := isStakeTree && isStakeOutput(scriptVer, prevOutScript)
			if isStakeOut {
				data.AddStakePkScript(prevOutScript)
			} else {
				data.AddRegularPkScript(prevOutScript)
			}
		}
	}

	// Add data from stake transactions.  For each class of stake transaction,
	// the following data is committed to the regular filter:
	//
	// - Ticket purchases:
	//   - Previous output scripts referenced by the transaction inputs
	//   - Commitment output scripts (converted from the commitment hash and
	//     type)
	//   - Change output scripts except those that are provably unspendable
	//
	// - Votes:
	//   - Subsidy generation output scripts
	//   - Output scripts that pay the original ticket commitments
	//
	// - Revocations:
	//   - Output scripts that pay the original ticket commitments
	//
	// Output scripts are handled specially for stake transactions by slicing
	// off the stake opcode tag (OP_SS*).  This tag always appears as the first
	// byte of the script and removing it allows users of the filter to only
	// match against a normal P2PKH or P2SH script, instead of many extra
	// matches for each tag.
	converter := makeCommitmentConverter(block.Header.FreshStake)
	for _, tx := range block.STransactions {
		switch stake.DetermineTxType(tx) {
		case stake.TxTypeSStx:
			for txInIdx, txIn := range tx.TxIn {
				prevOut := &txIn.PreviousOutPoint
				scriptVer, prevOutScript, ok := prevScripts.PrevScript(prevOut)
				if !ok {
					return nil, PrevScriptError{
						PrevOut: *prevOut,
						TxHash:  tx.TxHash(),
						TxInIdx: txInIdx,
					}
				}

				// Don't add scripts that are empty, larger than the max allowed
				// length, or for an unsupported script version.
				if excludeFromFilter(scriptVer, prevOutScript) {
					continue
				}

				// Add the script to be included in the filter while stripping
				// the initial stake opcode for those in the stake tree.
				isStakeTree := prevOut.Tree == wire.TxTreeStake
				isStakeOut := isStakeTree && isStakeOutput(scriptVer, prevOutScript)
				if isStakeOut {
					data.AddStakePkScript(prevOutScript)
				} else {
					data.AddRegularPkScript(prevOutScript)
				}
			}

			for txOutIdx, txOut := range tx.TxOut {
				// Don't commit to the voting rights output.  This is done to
				// help thwart attempts at SPV voting.  An SPV client does not
				// have access to the full blocks, so they can't reasonably vote
				// on their validity.
				if txOutIdx == 0 {
					continue
				}

				// Commit to the change outputs with the exception of those that
				// are provably unspendable.
				isChangeOuput := txOutIdx%2 == 0
				if isChangeOuput {
					if txOut.Value == 0 {
						continue
					}
					data.AddStakePkScript(txOut.PkScript)
					continue
				}

				// Commit to the final payment scripts that are required by the
				// ticket commitment outputs.
				if len(txOut.PkScript) == 0 {
					continue
				}
				paymentScript := converter.paymentScript(txOut.PkScript)
				data.AddRegularPkScript(paymentScript)
			}

		case stake.TxTypeSSGen:
			// The first two outputs are skipped because they indicate the block
			// voted on and the vote choices, respectively.
			for _, txOut := range tx.TxOut[2:] {
				data.AddStakePkScript(txOut.PkScript)
			}

		case stake.TxTypeSSRtx:
			for _, txOut := range tx.TxOut {
				data.AddStakePkScript(txOut.PkScript)
			}
		}
	}

	// Create the key by truncating the block's merkle root and use it to create
	// the filter.
	key := Key(&block.Header.MerkleRoot)
	return gcs.NewFilterV2(B, M, key, data)
}
