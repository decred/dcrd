// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v4"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/gcs/v3"
	"github.com/decred/dcrd/gcs/v3/blockcf2"
	"github.com/decred/dcrd/wire"
)

const (
	// currentDatabaseVersion indicates the current database version.
	currentDatabaseVersion = 12

	// currentBlockIndexVersion indicates the current block index database
	// version.
	currentBlockIndexVersion = 3

	// currentSpendJournalVersion indicates the current spend journal database
	// version.
	currentSpendJournalVersion = 3

	// blockHdrSize is the size of a block header.  This is simply the
	// constant from wire and is only provided here for convenience since
	// wire.MaxBlockHeaderPayload is quite long.
	blockHdrSize = wire.MaxBlockHeaderPayload
)

var (
	// byteOrder is the preferred byte order used for serializing numeric fields
	// for storage in the database.
	byteOrder = binary.LittleEndian

	// bcdbInfoBucketName is the name of the database bucket used to house
	// global versioning and date information for the blockchain database.
	bcdbInfoBucketName = []byte("dbinfo")

	// bcdbInfoVersionKeyName is the name of the database key used to house the
	// database version.  It is itself under the bcdbInfoBucketName bucket.
	bcdbInfoVersionKeyName = []byte("version")

	// bcdbInfoCompressionVerKeyName is the name of the database key used to
	// house the database compression version.  It is itself under the
	// bcdbInfoBucketName bucket.
	bcdbInfoCompressionVerKeyName = []byte("compver")

	// bcdbInfoBlockIndexVerKeyName is the name of the database key used to
	// house the database block index version.  It is itself under the
	// bcdbInfoBucketName bucket.
	bcdbInfoBlockIndexVerKeyName = []byte("bidxver")

	// bcdbInfoCreatedKeyName is the name of the database key used to house
	// date the database was created.  It is itself under the
	// bcdbInfoBucketName bucket.
	bcdbInfoCreatedKeyName = []byte("created")

	// bcdbInfoSpendJournalVerKeyName is the name of the database key used to
	// house the database spend journal version.  It is itself under the
	// bcdbInfoBucketName bucket.
	bcdbInfoSpendJournalVerKeyName = []byte("stxover")

	// chainStateKeyName is the name of the db key used to store the best chain
	// state.
	chainStateKeyName = []byte("chainstate")

	// spendJournalBucketName is the name of the db bucket used to house
	// transactions outputs that are spent in each block.
	spendJournalBucketName = []byte("spendjournalv3")

	// blockIndexBucketName is the name of the db bucket used to house the block
	// index which consists of metadata for all known blocks both in the main
	// chain and on side chains.
	blockIndexBucketName = []byte("blockidxv3")

	// gcsFilterBucketName is the name of the db bucket used to house GCS
	// filters.
	gcsFilterBucketName = []byte("gcsfilters")

	// treasuryBucketName is the name of the db bucket that is used to house
	// TADD/TSPEND additions and subtractions from the treasury account.
	treasuryBucketName = []byte("treasury")

	// treasuryTSpendBucketName is the name of the db bucket that is used to
	// house TSpend transactions which were included in the blockchain.
	treasuryTSpendBucketName = []byte("tspend")
)

// errNotInMainChain signifies that a block hash or height that is not in the
// main chain was requested.
type errNotInMainChain string

// Error implements the error interface.
func (e errNotInMainChain) Error() string {
	return string(e)
}

// errDeserialize signifies that a problem was encountered when deserializing
// data.
type errDeserialize string

// Error implements the error interface.
func (e errDeserialize) Error() string {
	return string(e)
}

// Is implements the interface to work with the standard library's errors.Is.
//
// It returns true in the following cases:
// - The target is errDeserialize
func (e errDeserialize) Is(target error) bool {
	return isDeserializeErr(target)
}

// isDeserializeErr returns whether or not the passed error is an errDeserialize
// error.
func isDeserializeErr(err error) bool {
	var e errDeserialize
	return errors.As(err, &e)
}

// makeDbErr creates a database.Error given a set of arguments.
func makeDbErr(kind database.ErrorKind, desc string) database.Error {
	return database.Error{Err: kind, Description: desc}
}

// -----------------------------------------------------------------------------
// The staking system requires some extra information to be stored for tickets
// to maintain consensus rules. The full set of minimal outputs are thus required
// in order for the chain to work correctly. A 'minimal output' is simply the
// script version, pubkey script, and amount.

// serializeSizeForMinimalOutputs calculates the number of bytes needed to
// serialize a transaction to its minimal outputs.
func serializeSizeForMinimalOutputs(tx *dcrutil.Tx) int {
	sz := serializeSizeVLQ(uint64(len(tx.MsgTx().TxOut)))
	for _, out := range tx.MsgTx().TxOut {
		sz += serializeSizeVLQ(compressTxOutAmount(uint64(out.Value)))
		sz += serializeSizeVLQ(uint64(out.Version))
		sz += serializeSizeVLQ(uint64(len(out.PkScript)))
		sz += len(out.PkScript)
	}

	return sz
}

// putTxToMinimalOutputs serializes a transaction to its minimal outputs.
// It returns the amount of data written. The function will panic if it writes
// beyond the bounds of the passed memory.
func putTxToMinimalOutputs(target []byte, tx *dcrutil.Tx) int {
	offset := putVLQ(target, uint64(len(tx.MsgTx().TxOut)))
	for _, out := range tx.MsgTx().TxOut {
		offset += putVLQ(target[offset:], compressTxOutAmount(uint64(out.Value)))
		offset += putVLQ(target[offset:], uint64(out.Version))
		offset += putVLQ(target[offset:], uint64(len(out.PkScript)))
		copy(target[offset:], out.PkScript)
		offset += len(out.PkScript)
	}

	return offset
}

// deserializeToMinimalOutputs deserializes a series of minimal outputs to their
// decompressed, deserialized state and stores them in a slice. It also returns
// the amount of data read. The function will panic if it reads beyond the bounds
// of the passed memory.
func deserializeToMinimalOutputs(serialized []byte) ([]*stake.MinimalOutput, int) {
	numOutputs, offset := deserializeVLQ(serialized)
	minOuts := make([]*stake.MinimalOutput, int(numOutputs))
	for i := 0; i < int(numOutputs); i++ {
		amountComp, bytesRead := deserializeVLQ(serialized[offset:])
		amount := decompressTxOutAmount(amountComp)
		offset += bytesRead

		version, bytesRead := deserializeVLQ(serialized[offset:])
		offset += bytesRead

		scriptSize, bytesRead := deserializeVLQ(serialized[offset:])
		offset += bytesRead

		pkScript := make([]byte, int(scriptSize))
		copy(pkScript, serialized[offset:offset+int(scriptSize)])
		offset += int(scriptSize)

		minOuts[i] = &stake.MinimalOutput{
			Value:    int64(amount),
			Version:  uint16(version),
			PkScript: pkScript,
		}
	}

	return minOuts, offset
}

// readDeserializeSizeOfMinimalOutputs reads the size of the stored set of
// minimal outputs without allocating memory for the structs themselves.
func readDeserializeSizeOfMinimalOutputs(serialized []byte) (int, error) {
	numOutputs, offset := deserializeVLQ(serialized)
	if offset == 0 {
		return offset, errDeserialize("unexpected end of " +
			"data during decoding (num outputs)")
	}
	for i := 0; i < int(numOutputs); i++ {
		// Amount
		_, bytesRead := deserializeVLQ(serialized[offset:])
		if bytesRead == 0 {
			return offset, errDeserialize("unexpected end of " +
				"data during decoding (output amount)")
		}
		offset += bytesRead

		// Script version
		_, bytesRead = deserializeVLQ(serialized[offset:])
		if bytesRead == 0 {
			return offset, errDeserialize("unexpected end of " +
				"data during decoding (output script version)")
		}
		offset += bytesRead

		// Script
		var scriptSize uint64
		scriptSize, bytesRead = deserializeVLQ(serialized[offset:])
		if bytesRead == 0 {
			return offset, errDeserialize("unexpected end of " +
				"data during decoding (output script size)")
		}
		offset += bytesRead

		if uint64(len(serialized[offset:])) < scriptSize {
			return offset, errDeserialize("unexpected end of " +
				"data during decoding (output script)")
		}

		offset += int(scriptSize)
	}

	return offset, nil
}

// -----------------------------------------------------------------------------
// The block index consists of an entry for every known block.  It consists of
// information such as the block header and information about votes.
//
// The serialized key format is:
//
//   <block height><block hash>
//
//   Field           Type              Size
//   block height    uint32            4 bytes
//   block hash      chainhash.Hash    chainhash.HashSize
//
// The serialized value format is:
//
//   <block header><status><num votes><votes info>
//
//   Field              Type                Size
//   block header       wire.BlockHeader    180 bytes
//   status             blockStatus         1 byte
//   num votes          VLQ                 variable
//   vote info
//     vote version     VLQ                 variable
//     vote bits        VLQ                 variable
// -----------------------------------------------------------------------------

// blockIndexEntry represents a block index database entry.
type blockIndexEntry struct {
	header   wire.BlockHeader
	status   blockStatus
	voteInfo []stake.VoteVersionTuple
}

// blockIndexKey generates the binary key for an entry in the block index
// bucket.  The key is composed of the block height encoded as a big-endian
// 32-bit unsigned int followed by the 32 byte block hash.  Big endian is used
// here so the entries can easily be iterated by height.
func blockIndexKey(blockHash *chainhash.Hash, blockHeight uint32) []byte {
	indexKey := make([]byte, chainhash.HashSize+4)
	binary.BigEndian.PutUint32(indexKey[0:4], blockHeight)
	copy(indexKey[4:chainhash.HashSize+4], blockHash[:])
	return indexKey
}

// blockIndexEntrySerializeSize returns the number of bytes it would take to
// serialize the passed block index entry according to the format described
// above.
func blockIndexEntrySerializeSize(entry *blockIndexEntry) int {
	voteInfoSize := 0
	for i := range entry.voteInfo {
		voteInfoSize += serializeSizeVLQ(uint64(entry.voteInfo[i].Version)) +
			serializeSizeVLQ(uint64(entry.voteInfo[i].Bits))
	}

	return blockHdrSize + 1 + serializeSizeVLQ(uint64(len(entry.voteInfo))) +
		voteInfoSize
}

// putBlockIndexEntry serializes the passed block index entry according to the
// format described above directly into the passed target byte slice.  The
// target byte slice must be at least large enough to handle the number of bytes
// returned by the blockIndexEntrySerializeSize function or it will panic.
func putBlockIndexEntry(target []byte, entry *blockIndexEntry) (int, error) {
	// Serialize the entire block header.
	w := bytes.NewBuffer(target[0:0])
	if err := entry.header.Serialize(w); err != nil {
		return 0, err
	}

	// Serialize the status.
	offset := blockHdrSize
	target[offset] = byte(entry.status)
	offset++

	// Serialize the number of votes and associated vote information.
	offset += putVLQ(target[offset:], uint64(len(entry.voteInfo)))
	for i := range entry.voteInfo {
		offset += putVLQ(target[offset:], uint64(entry.voteInfo[i].Version))
		offset += putVLQ(target[offset:], uint64(entry.voteInfo[i].Bits))
	}

	return offset, nil
}

// serializeBlockIndexEntry serializes the passed block index entry into a
// single byte slice according to the format described in detail above.
func serializeBlockIndexEntry(entry *blockIndexEntry) ([]byte, error) {
	serialized := make([]byte, blockIndexEntrySerializeSize(entry))
	_, err := putBlockIndexEntry(serialized, entry)
	return serialized, err
}

// decodeBlockIndexEntry decodes the passed serialized block index entry into
// the passed struct according to the format described above.  It returns the
// number of bytes read.
func decodeBlockIndexEntry(serialized []byte, entry *blockIndexEntry) (int, error) {
	// Ensure there are enough bytes to decode header.
	if len(serialized) < blockHdrSize {
		return 0, errDeserialize("unexpected end of data while " +
			"reading block header")
	}
	hB := serialized[0:blockHdrSize]

	// Deserialize the header.
	var header wire.BlockHeader
	if err := header.Deserialize(bytes.NewReader(hB)); err != nil {
		return 0, err
	}
	offset := blockHdrSize

	// Deserialize the status.
	if offset+1 > len(serialized) {
		return offset, errDeserialize("unexpected end of data while " +
			"reading status")
	}
	status := blockStatus(serialized[offset])
	offset++

	// Deserialize the number of tickets spent.
	var votes []stake.VoteVersionTuple
	numVotes, bytesRead := deserializeVLQ(serialized[offset:])
	if bytesRead == 0 {
		return offset, errDeserialize("unexpected end of data while " +
			"reading num votes")
	}
	offset += bytesRead
	if numVotes > 0 {
		votes = make([]stake.VoteVersionTuple, numVotes)
		for i := uint64(0); i < numVotes; i++ {
			// Deserialize the vote version.
			version, bytesRead := deserializeVLQ(serialized[offset:])
			if bytesRead == 0 {
				return offset, errDeserialize(fmt.Sprintf("unexpected "+
					"end of data while reading vote #%d version",
					i))
			}
			offset += bytesRead

			// Deserialize the vote bits.
			voteBits, bytesRead := deserializeVLQ(serialized[offset:])
			if bytesRead == 0 {
				return offset, errDeserialize(fmt.Sprintf("unexpected "+
					"end of data while reading vote #%d bits",
					i))
			}
			offset += bytesRead

			votes[i].Version = uint32(version)
			votes[i].Bits = uint16(voteBits)
		}
	}

	entry.header = header
	entry.status = status
	entry.voteInfo = votes
	return offset, nil
}

// deserializeBlockIndexEntry decodes the passed serialized byte slice into a
// block index entry according to the format described above.
func deserializeBlockIndexEntry(serialized []byte) (*blockIndexEntry, error) {
	var entry blockIndexEntry
	if _, err := decodeBlockIndexEntry(serialized, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// dbPutBlockNode stores the information needed to reconstruct the provided
// block node in the block index according to the format described above.
func dbPutBlockNode(dbTx database.Tx, node *blockNode) error {
	serialized, err := serializeBlockIndexEntry(&blockIndexEntry{
		header:   node.Header(),
		status:   node.status,
		voteInfo: node.votes,
	})
	if err != nil {
		return err
	}

	bucket := dbTx.Metadata().Bucket(blockIndexBucketName)
	key := blockIndexKey(&node.hash, uint32(node.height))
	return bucket.Put(key, serialized)
}

// dbMaybeStoreBlock stores the provided block in the database if it's not
// already there.
func dbMaybeStoreBlock(dbTx database.Tx, block *dcrutil.Block) error {
	// Store the block in ffldb if not already done.
	hasBlock, err := dbTx.HasBlock(block.Hash())
	if err != nil {
		return err
	}
	if hasBlock {
		return nil
	}

	return dbTx.StoreBlock(block)
}

// -----------------------------------------------------------------------------
// The transaction spend journal consists of an entry for each block connected
// to the main chain which contains the transaction outputs the block spends
// serialized such that the order is the reverse of the order they were spent.
//
// This is required because reorganizing the chain necessarily entails
// disconnecting blocks to get back to the point of the fork which implies
// unspending all of the transaction outputs that each block previously spent.
// Since the utxo set, by definition, only contains unspent transaction outputs,
// the spent transaction outputs must be resurrected from somewhere.  There is
// more than one way this could be done, however this is the most straight
// forward method that does not require having a transaction index and unpruned
// blockchain.
//
// NOTE: This format is NOT self describing.  The additional details such as
// the number of entries (transaction inputs) are expected to come from the
// block itself and the utxo set.  The rationale in doing this is to save a
// significant amount of space.  This is also the reason the spent outputs are
// serialized in the reverse order they are spent because later transactions
// are allowed to spend outputs from earlier ones in the same block.
//
// The serialized format is:
//
//   [<flags><script version><compressed pk script>],...
//   OPTIONAL: <ticket min outs>
//
//   Field                Type           Size
//   flags                VLQ            byte
//   scriptVersion        uint16         2 bytes
//   pkScript             VLQ+[]byte     variable
//
//   OPTIONAL
//     ticketMinOuts      []byte         variable
//
// The serialized flags format is:
//   bit  0     - containing transaction is a coinbase
//   bit  1     - containing transaction has an expiry
//   bits 2-5   - transaction type
//   bits 6-7   - unused
//
// The ticket min outs field contains minimally encoded outputs for all outputs
// of a ticket transaction. It is only encoded for ticket submission outputs.
//
// -----------------------------------------------------------------------------

// spentTxOut contains a spent transaction output and potentially additional
// contextual information such as whether or not it was contained in a coinbase
// transaction, whether or not the containing transaction has an expiry, and the
// transaction type.
//
// The struct is aligned for memory efficiency.
type spentTxOut struct {
	amount   int64
	pkScript []byte

	// ticketMinOuts is the minimal outputs for the ticket transaction that the
	// output is contained in.  This is only stored in ticket submission outputs
	// and is nil for all other output types.
	//
	// Note that this is using a pointer rather than a slice in order to occupy
	// less space when it is nil.  It is nil in the vast majority of entries, so
	// this provides a significant overall reduction in memory usage.
	ticketMinOuts *ticketMinimalOutputs

	blockHeight   uint32
	blockIndex    uint32
	scriptVersion uint16

	// packedFlags contains additional info about the output as defined by
	// txOutFlags.  This approach is used in order to reduce memory usage since
	// there will be a lot of these in memory.
	packedFlags txOutFlags
}

// IsCoinBase returns whether or not the output was contained in a coinbase
// transaction.
func (stxo *spentTxOut) IsCoinBase() bool {
	return stxo.packedFlags&txOutFlagCoinBase == txOutFlagCoinBase
}

// HasExpiry returns whether or not the output was contained in a transaction
// that included an expiry.
func (stxo *spentTxOut) HasExpiry() bool {
	return stxo.packedFlags&txOutFlagHasExpiry == txOutFlagHasExpiry
}

// TransactionType returns the type of the transaction that the output is
// contained in.
func (stxo *spentTxOut) TransactionType() stake.TxType {
	txType := (stxo.packedFlags & txOutFlagTxTypeBitmask) >> txOutFlagTxTypeShift
	return stake.TxType(txType)
}

// spentTxOutSerializeSize returns the number of bytes it would take to
// serialize the passed stxo according to the format described above.
// The amount is never encoded into spent transaction outputs in Decred
// because they're already encoded into the transactions, so skip them when
// determining the serialization size.
func spentTxOutSerializeSize(stxo *spentTxOut) int {
	flags := encodeFlags(stxo.IsCoinBase(), stxo.HasExpiry(),
		stxo.TransactionType())
	size := serializeSizeVLQ(uint64(flags))

	const hasAmount = false
	size += compressedTxOutSize(uint64(stxo.amount), stxo.scriptVersion,
		stxo.pkScript, hasAmount)

	if stxo.ticketMinOuts != nil {
		size += len(stxo.ticketMinOuts.data)
	}

	return size
}

// putSpentTxOut serializes the passed stxo according to the format described
// above directly into the passed target byte slice.  The target byte slice must
// be at least large enough to handle the number of bytes returned by the
// spentTxOutSerializeSize function or it will panic.
func putSpentTxOut(target []byte, stxo *spentTxOut) int {
	flags := encodeFlags(stxo.IsCoinBase(), stxo.HasExpiry(),
		stxo.TransactionType())
	offset := putVLQ(target, uint64(flags))

	const hasAmount = false
	offset += putCompressedTxOut(target[offset:], 0, stxo.scriptVersion,
		stxo.pkScript, hasAmount)

	if stxo.ticketMinOuts != nil {
		copy(target[offset:], stxo.ticketMinOuts.data)
		offset += len(stxo.ticketMinOuts.data)
	}

	return offset
}

// decodeSpentTxOut decodes the passed serialized stxo entry, possibly followed
// by other data, into the passed stxo struct.  It returns the number of bytes
// read.
func decodeSpentTxOut(serialized []byte, stxo *spentTxOut, amount int64,
	height uint32, index uint32, txOutIndex uint32) (int, error) {

	// Deserialize the flags.
	flags, bytesRead := deserializeVLQ(serialized)
	offset := bytesRead
	if offset >= len(serialized) {
		return offset, errDeserialize("unexpected end of data after flags")
	}

	// Decode the compressed txout. We pass false for the amount flag,
	// since in Decred we only need pkScript at most due to fraud proofs
	// already storing the decompressed amount.
	_, scriptVersion, script, bytesRead, err :=
		decodeCompressedTxOut(serialized[offset:], false)
	offset += bytesRead
	if err != nil {
		return offset, errDeserialize(fmt.Sprintf("unable to decode "+
			"txout: %v", err))
	}

	// Populate the stxo.
	stxo.amount = amount
	stxo.pkScript = script
	stxo.blockHeight = height
	stxo.blockIndex = index
	stxo.scriptVersion = scriptVersion
	stxo.packedFlags = txOutFlags(flags)

	// Copy the minimal outputs if this was a ticket submission output.
	if isTicketSubmissionOutput(stxo.TransactionType(), txOutIndex) {
		sz, err := readDeserializeSizeOfMinimalOutputs(serialized[offset:])
		if err != nil {
			return offset + sz, errDeserialize(fmt.Sprintf("unable to decode "+
				"ticket outputs: %v", err))
		}
		stxo.ticketMinOuts = &ticketMinimalOutputs{
			data: make([]byte, sz),
		}
		copy(stxo.ticketMinOuts.data, serialized[offset:offset+sz])
		offset += sz
	}

	return offset, nil
}

// deserializeSpendJournalEntry decodes the passed serialized byte slice into a
// slice of spent txouts according to the format described in detail above.
//
// Since the serialization format is not self describing, as noted in the
// format comments, this function also requires the transactions that spend the
// txouts and a utxo view that contains any remaining existing utxos in the
// transactions referenced by the inputs to the passed transactions.
func deserializeSpendJournalEntry(serialized []byte, txns []*wire.MsgTx, isTreasuryEnabled bool) ([]spentTxOut, error) {
	// Calculate the total number of stxos.
	var numStxos int
	for _, tx := range txns {
		if stake.IsSSGen(tx, isTreasuryEnabled) {
			numStxos++
			continue
		}
		numStxos += len(tx.TxIn)
	}

	// When a block has no spent txouts there is nothing to serialize.
	if len(serialized) == 0 {
		// Ensure the block actually has no stxos.  This should never
		// happen unless there is database corruption or an empty entry
		// erroneously made its way into the database.
		if numStxos != 0 {
			return nil, AssertError(fmt.Sprintf("mismatched spend "+
				"journal serialization - no serialization for "+
				"expected %d stxos", numStxos))
		}

		return nil, nil
	}

	// Loop backwards through all transactions so everything is read in
	// reverse order to match the serialization order.
	stxoIdx := numStxos - 1
	offset := 0
	stxos := make([]spentTxOut, numStxos)
	for txIdx := len(txns) - 1; txIdx > -1; txIdx-- {
		tx := txns[txIdx]
		isVote := stake.IsSSGen(tx, isTreasuryEnabled)

		// Loop backwards through all of the transaction inputs and read
		// the associated stxo.
		for txInIdx := len(tx.TxIn) - 1; txInIdx > -1; txInIdx-- {
			// Skip stakebase since it has no input.
			if txInIdx == 0 && isVote {
				continue
			}

			txIn := tx.TxIn[txInIdx]
			stxo := &stxos[stxoIdx]
			stxoIdx--

			n, err := decodeSpentTxOut(serialized[offset:], stxo, txIn.ValueIn,
				txIn.BlockHeight, txIn.BlockIndex, txIn.PreviousOutPoint.Index)
			offset += n
			if err != nil {
				return nil, errDeserialize(fmt.Sprintf("unable "+
					"to decode stxo for %v: %v",
					txIn.PreviousOutPoint, err))
			}
		}
	}

	return stxos, nil
}

// serializeSpendJournalEntry serializes all of the passed spent txouts into a
// single byte slice according to the format described in detail above.
func serializeSpendJournalEntry(stxos []spentTxOut) ([]byte, error) {
	if len(stxos) == 0 {
		return nil, nil
	}

	// Calculate the size needed to serialize the entire journal entry.
	var size int
	sizes := make([]int, 0, len(stxos))
	for i := range stxos {
		sz := spentTxOutSerializeSize(&stxos[i])
		sizes = append(sizes, sz)
		size += sz
	}
	serialized := make([]byte, size)

	// Serialize each individual stxo directly into the slice in reverse
	// order one after the other.
	var offset int
	for i := len(stxos) - 1; i > -1; i-- {
		oldOffset := offset
		offset += putSpentTxOut(serialized[offset:], &stxos[i])

		if offset-oldOffset != sizes[i] {
			return nil, AssertError(fmt.Sprintf("bad write; expect sz %v, "+
				"got sz %v (wrote %x)", sizes[i], offset-oldOffset,
				serialized[oldOffset:offset]))
		}
	}

	return serialized, nil
}

// dbFetchSpendJournalEntry fetches the spend journal entry for the passed
// block and deserializes it into a slice of spent txout entries.  The provided
// view MUST have the utxos referenced by all of the transactions available for
// the passed block since that information is required to reconstruct the spent
// txouts.
func dbFetchSpendJournalEntry(dbTx database.Tx, block *dcrutil.Block, isTreasuryEnabled bool) ([]spentTxOut, error) {
	// Exclude the coinbase transaction since it can't spend anything.
	spendBucket := dbTx.Metadata().Bucket(spendJournalBucketName)
	serialized := spendBucket.Get(block.Hash()[:])
	msgBlock := block.MsgBlock()

	blockTxns := make([]*wire.MsgTx, 0, len(msgBlock.STransactions)+
		len(msgBlock.Transactions[1:]))
	if len(msgBlock.STransactions) > 0 && isTreasuryEnabled {
		// Skip treasury base and remove tspends.
		for _, v := range msgBlock.STransactions[1:] {
			if stake.IsTSpend(v) {
				continue
			}
			blockTxns = append(blockTxns, v)
		}
	} else {
		blockTxns = append(blockTxns, msgBlock.STransactions...)
	}
	blockTxns = append(blockTxns, msgBlock.Transactions[1:]...)
	if len(blockTxns) > 0 && len(serialized) == 0 {
		panicf("missing spend journal data for %s", block.Hash())
	}

	stxos, err := deserializeSpendJournalEntry(serialized, blockTxns,
		isTreasuryEnabled)
	if err != nil {
		// Ensure any deserialization errors are returned as database
		// corruption errors.
		if isDeserializeErr(err) {
			str := fmt.Sprintf("corrupt spend information for %v: %v",
				block.Hash(), err)
			return nil, makeDbErr(database.ErrCorruption, str)
		}

		return nil, err
	}

	return stxos, nil
}

// dbPutSpendJournalEntry uses an existing database transaction to update the
// spend journal entry for the given block hash using the provided slice of
// spent txouts.   The spent txouts slice must contain an entry for every txout
// the transactions in the block spend in the order they are spent.
func dbPutSpendJournalEntry(dbTx database.Tx, blockHash *chainhash.Hash, stxos []spentTxOut) error {
	spendBucket := dbTx.Metadata().Bucket(spendJournalBucketName)
	serialized, err := serializeSpendJournalEntry(stxos)
	if err != nil {
		return err
	}
	return spendBucket.Put(blockHash[:], serialized)
}

// dbRemoveSpendJournalEntry uses an existing database transaction to remove the
// spend journal entry for the passed block hash.
func dbRemoveSpendJournalEntry(dbTx database.Tx, blockHash *chainhash.Hash) error {
	spendBucket := dbTx.Metadata().Bucket(spendJournalBucketName)
	return spendBucket.Delete(blockHash[:])
}

// -----------------------------------------------------------------------------
// The GCS filter journal consists of an entry for each block connected to the
// main chain (or has ever been connected to it) which consists of a serialized
// GCS filter.
//
// The serialized key format is:
//
//   <block hash>
//
//   Field           Type              Size
//   block hash      chainhash.Hash    chainhash.HashSize
//
// The serialized value format is:
//
//   <serialized filter>
//
//   Field           Type                     Size
//   filter          []byte (gcs.FilterV2)    variable
//
// -----------------------------------------------------------------------------

// dbFetchGCSFilter fetches the GCS filter for the passed block and deserializes
// it into a slice of spent txout entries.
//
// When there is no entry for the provided hash, nil will be returned for both
// the filter and the error.
func dbFetchGCSFilter(dbTx database.Tx, blockHash *chainhash.Hash) (*gcs.FilterV2, error) {
	filterBucket := dbTx.Metadata().Bucket(gcsFilterBucketName)
	serialized := filterBucket.Get(blockHash[:])
	if serialized == nil {
		return nil, nil
	}

	filter, err := gcs.FromBytesV2(blockcf2.B, blockcf2.M, serialized)
	if err != nil {
		str := fmt.Sprintf("corrupt filter for %v: %v", blockHash, err)
		return nil, makeDbErr(database.ErrCorruption, str)
	}

	return filter, nil
}

// dbPutGCSFilter uses an existing database transaction to update the GCS filter
// for the given block hash using the provided filter.
func dbPutGCSFilter(dbTx database.Tx, blockHash *chainhash.Hash, filter *gcs.FilterV2) error {
	filterBucket := dbTx.Metadata().Bucket(gcsFilterBucketName)
	serialized := filter.Bytes()
	return filterBucket.Put(blockHash[:], serialized)
}

// -----------------------------------------------------------------------------
// The database information contains information about the version and date
// of the blockchain database.
//
// It consists of a separate key for each individual piece of information:
//
//   Key        Value    Size      Description
//   version    uint32   4 bytes   The version of the database
//   compver    uint32   4 bytes   The script compression version of the database
//   bidxver    uint32   4 bytes   The block index version of the database
//   created    uint64   8 bytes   The date of the creation of the database
//   stxover    uint32   4 bytes   The spend journal version of the database
// -----------------------------------------------------------------------------

// databaseInfo is the structure for a database.
type databaseInfo struct {
	version uint32
	compVer uint32
	bidxVer uint32
	created time.Time
	stxoVer uint32
}

// dbPutDatabaseInfo uses an existing database transaction to store the database
// information.
func dbPutDatabaseInfo(dbTx database.Tx, dbi *databaseInfo) error {
	// uint32Bytes is a helper function to convert a uint32 to a byte slice
	// using the byte order specified by the database namespace.
	uint32Bytes := func(ui32 uint32) []byte {
		var b [4]byte
		byteOrder.PutUint32(b[:], ui32)
		return b[:]
	}

	// uint64Bytes is a helper function to convert a uint64 to a byte slice
	// using the byte order specified by the database namespace.
	uint64Bytes := func(ui64 uint64) []byte {
		var b [8]byte
		byteOrder.PutUint64(b[:], ui64)
		return b[:]
	}

	// Store the database version.
	meta := dbTx.Metadata()
	bucket := meta.Bucket(bcdbInfoBucketName)
	err := bucket.Put(bcdbInfoVersionKeyName, uint32Bytes(dbi.version))
	if err != nil {
		return err
	}

	// Store the compression version.
	err = bucket.Put(bcdbInfoCompressionVerKeyName, uint32Bytes(dbi.compVer))
	if err != nil {
		return err
	}

	// Store the block index version.
	err = bucket.Put(bcdbInfoBlockIndexVerKeyName, uint32Bytes(dbi.bidxVer))
	if err != nil {
		return err
	}

	// Store the database creation date.
	err = bucket.Put(bcdbInfoCreatedKeyName,
		uint64Bytes(uint64(dbi.created.Unix())))
	if err != nil {
		return err
	}

	// Store the spend journal version.
	return bucket.Put(bcdbInfoSpendJournalVerKeyName, uint32Bytes(dbi.stxoVer))
}

// dbFetchDatabaseInfo uses an existing database transaction to fetch the
// database versioning and creation information.
func dbFetchDatabaseInfo(dbTx database.Tx) (*databaseInfo, error) {
	meta := dbTx.Metadata()
	bucket := meta.Bucket(bcdbInfoBucketName)

	// Uninitialized state.
	if bucket == nil {
		return nil, nil
	}

	// Load the database version.
	var version uint32
	versionBytes := bucket.Get(bcdbInfoVersionKeyName)
	if versionBytes != nil {
		version = byteOrder.Uint32(versionBytes)
	}

	// Load the database compression version.
	var compVer uint32
	compVerBytes := bucket.Get(bcdbInfoCompressionVerKeyName)
	if compVerBytes != nil {
		compVer = byteOrder.Uint32(compVerBytes)
	}

	// Load the database block index version.
	var bidxVer uint32
	bidxVerBytes := bucket.Get(bcdbInfoBlockIndexVerKeyName)
	if bidxVerBytes != nil {
		bidxVer = byteOrder.Uint32(bidxVerBytes)
	}

	// Load the database creation date.
	var created time.Time
	createdBytes := bucket.Get(bcdbInfoCreatedKeyName)
	if createdBytes != nil {
		ts := byteOrder.Uint64(createdBytes)
		created = time.Unix(int64(ts), 0)
	}

	// Load the database spend journal version.  This is tracked in the database
	// starting in database version 10.
	var stxoVer uint32
	if version >= 10 {
		stxoVerBytes := bucket.Get(bcdbInfoSpendJournalVerKeyName)
		if stxoVerBytes != nil {
			stxoVer = byteOrder.Uint32(stxoVerBytes)
		}
	}

	return &databaseInfo{
		version: version,
		compVer: compVer,
		bidxVer: bidxVer,
		created: created,
		stxoVer: stxoVer,
	}, nil
}

// -----------------------------------------------------------------------------
// The best chain state consists of the best block hash and height, the total
// number of transactions up to and including those in the best block, the
// total coin supply, the subsidy at the current block, the subsidy of the
// block prior (for rollbacks), and the accumulated work sum up to and
// including the best block.
//
// The serialized format is:
//
//   <block hash><block height><total txns><total subsidy><work sum length><work sum>
//
//   Field             Type             Size
//   block hash        chainhash.Hash   chainhash.HashSize
//   block height      uint32           4 bytes
//   total txns        uint64           8 bytes
//   total subsidy     int64            8 bytes
//   work sum length   uint32           4 bytes
//   work sum          big.Int          work sum length
// -----------------------------------------------------------------------------

// bestChainState represents the data to be stored the database for the current
// best chain state.
type bestChainState struct {
	hash         chainhash.Hash
	height       uint32
	totalTxns    uint64
	totalSubsidy int64
	workSum      *big.Int
}

// serializeBestChainState returns the serialization of the passed block best
// chain state.  This is data to be stored in the chain state bucket.
func serializeBestChainState(state bestChainState) []byte {
	// Calculate the full size needed to serialize the chain state.
	workSumBytes := state.workSum.Bytes()
	workSumBytesLen := uint32(len(workSumBytes))
	serializedLen := chainhash.HashSize + 4 + 8 + 8 + 4 + workSumBytesLen

	// Serialize the chain state.
	serializedData := make([]byte, serializedLen)
	copy(serializedData[0:chainhash.HashSize], state.hash[:])
	offset := uint32(chainhash.HashSize)
	byteOrder.PutUint32(serializedData[offset:], state.height)
	offset += 4
	byteOrder.PutUint64(serializedData[offset:], state.totalTxns)
	offset += 8
	byteOrder.PutUint64(serializedData[offset:],
		uint64(state.totalSubsidy))
	offset += 8
	byteOrder.PutUint32(serializedData[offset:], workSumBytesLen)
	offset += 4
	copy(serializedData[offset:], workSumBytes)
	return serializedData
}

// deserializeBestChainState deserializes the passed serialized best chain
// state.  This is data stored in the chain state bucket and is updated after
// every block is connected or disconnected form the main chain.
// block.
func deserializeBestChainState(serializedData []byte) (bestChainState, error) {
	// Ensure the serialized data has enough bytes to properly deserialize
	// the hash, height, total transactions, total subsidy, current subsidy,
	// and work sum length.
	expectedMinLen := chainhash.HashSize + 4 + 8 + 8 + 4
	if len(serializedData) < expectedMinLen {
		str := fmt.Sprintf("corrupt best chain state size; min %v got %v",
			expectedMinLen, len(serializedData))
		return bestChainState{}, makeDbErr(database.ErrCorruption, str)
	}

	state := bestChainState{}
	copy(state.hash[:], serializedData[0:chainhash.HashSize])
	offset := uint32(chainhash.HashSize)
	state.height = byteOrder.Uint32(serializedData[offset : offset+4])
	offset += 4
	state.totalTxns = byteOrder.Uint64(
		serializedData[offset : offset+8])
	offset += 8
	state.totalSubsidy = int64(byteOrder.Uint64(
		serializedData[offset : offset+8]))
	offset += 8
	workSumBytesLen := byteOrder.Uint32(
		serializedData[offset : offset+4])
	offset += 4

	// Ensure the serialized data has enough bytes to deserialize the work
	// sum.
	if uint32(len(serializedData[offset:])) < workSumBytesLen {
		str := fmt.Sprintf("corrupt work sum size; want %v got %v",
			workSumBytesLen, uint32(len(serializedData[offset:])))
		return bestChainState{}, makeDbErr(database.ErrCorruption, str)
	}
	workSumBytes := serializedData[offset : offset+workSumBytesLen]
	state.workSum = new(big.Int).SetBytes(workSumBytes)

	return state, nil
}

// dbPutBestState uses an existing database transaction to update the best chain
// state with the given parameters.
func dbPutBestState(dbTx database.Tx, snapshot *BestState, workSum *big.Int) error {
	// Serialize the current best chain state.
	serializedData := serializeBestChainState(bestChainState{
		hash:         snapshot.Hash,
		height:       uint32(snapshot.Height),
		totalTxns:    snapshot.TotalTxns,
		totalSubsidy: snapshot.TotalSubsidy,
		workSum:      workSum,
	})

	// Store the current best chain state into the database.
	return dbTx.Metadata().Put(chainStateKeyName, serializedData)
}

// dbFetchBestState uses an existing database transaction to fetch the best
// chain state.
func dbFetchBestState(dbTx database.Tx) (bestChainState, error) {
	// Fetch the stored chain state from the database metadata.
	meta := dbTx.Metadata()
	serializedData := meta.Get(chainStateKeyName)
	log.Tracef("Serialized chain state: %x", serializedData)
	return deserializeBestChainState(serializedData)
}

// createChainState initializes both the database and the chain state to the
// genesis block.  This includes creating the necessary buckets and inserting
// the genesis block, so it must only be called on an uninitialized database.
func (b *BlockChain) createChainState() error {
	// Create a new node from the genesis block and set it as the best node.
	genesisBlock := dcrutil.NewBlock(b.chainParams.GenesisBlock)
	header := &genesisBlock.MsgBlock().Header
	node := newBlockNode(header, nil)
	node.status = statusDataStored | statusValidated
	node.isFullyLinked = true

	// Initialize the state related to the best block.  Since it is the
	// genesis block, use its timestamp for the median time.
	numTxns := uint64(len(genesisBlock.MsgBlock().Transactions))
	blockSize := uint64(genesisBlock.MsgBlock().SerializeSize())
	stateSnapshot := newBestState(node, blockSize, numTxns, numTxns,
		time.Unix(node.timestamp, 0), 0, 0, b.chainParams.MinimumStakeDiff,
		nil, nil, nil, earlyFinalState)

	// Create the initial the database chain state including creating the
	// necessary index buckets and inserting the genesis block.
	err := b.db.Update(func(dbTx database.Tx) error {
		meta := dbTx.Metadata()

		// Create the bucket that houses information about the database's
		// creation and version.
		_, err := meta.CreateBucket(bcdbInfoBucketName)
		if err != nil {
			return err
		}

		b.dbInfo = &databaseInfo{
			version: currentDatabaseVersion,
			compVer: currentCompressionVersion,
			bidxVer: currentBlockIndexVersion,
			created: time.Now(),
			stxoVer: currentSpendJournalVersion,
		}
		err = dbPutDatabaseInfo(dbTx, b.dbInfo)
		if err != nil {
			return err
		}

		// Create the bucket that houses the block index data.
		_, err = meta.CreateBucket(blockIndexBucketName)
		if err != nil {
			return err
		}

		// Create the bucket that houses the spend journal data.
		_, err = meta.CreateBucket(spendJournalBucketName)
		if err != nil {
			return err
		}

		// Add the genesis block to the block index.
		err = dbPutBlockNode(dbTx, node)
		if err != nil {
			return err
		}

		// Store the current best chain state into the database.
		err = dbPutBestState(dbTx, stateSnapshot, node.workSum)
		if err != nil {
			return err
		}

		// Initialize the stake buckets in the database, along with
		// the best state for the stake database.
		_, err = stake.InitDatabaseState(dbTx, b.chainParams,
			&b.chainParams.GenesisHash)
		if err != nil {
			return err
		}

		// Store the genesis block into the database.
		err = dbTx.StoreBlock(genesisBlock)
		if err != nil {
			return err
		}

		// Create the bucket that houses the gcs filters.
		_, err = meta.CreateBucket(gcsFilterBucketName)
		if err != nil {
			return err
		}

		// Store the (empty) GCS filter for the genesis block.
		genesisFilter, err := blockcf2.Regular(genesisBlock.MsgBlock(), nil)
		if err != nil {
			return err
		}
		err = dbPutGCSFilter(dbTx, &b.chainParams.GenesisHash, genesisFilter)
		if err != nil {
			return err
		}

		// Create the buckets that house the treasury account and spend
		// transaction information.
		_, err = meta.CreateBucket(treasuryBucketName)
		if err != nil {
			return err
		}
		_, err = meta.CreateBucket(treasuryTSpendBucketName)
		return err
	})
	return err
}

// loadBlockIndex loads all of the block index entries from the database and
// constructs the block index into the provided index parameter.  It is not safe
// for concurrent access as it is only intended to be used during initialization
// and database migration.
func loadBlockIndex(dbTx database.Tx, genesisHash *chainhash.Hash, index *blockIndex) error {
	// Determine how many blocks will be loaded into the index in order to
	// allocate the right amount as a single alloc versus a whole bunch of
	// little ones to reduce pressure on the GC.
	meta := dbTx.Metadata()
	blockIndexBucket := meta.Bucket(blockIndexBucketName)
	var blockCount int32
	cursor := blockIndexBucket.Cursor()
	for ok := cursor.First(); ok; ok = cursor.Next() {
		blockCount++
	}
	blockNodes := make([]blockNode, blockCount)

	// Initialize the best header to the node that will become the genesis block
	// below.
	index.bestHeader = &blockNodes[0]

	// Load all of the block index entries and construct the block index
	// accordingly.
	//
	// NOTE: No locks are used on the block index here since this is
	// initialization code.
	var i int32
	var lastNode *blockNode
	cursor = blockIndexBucket.Cursor()
	for ok := cursor.First(); ok; ok = cursor.Next() {
		entry, err := deserializeBlockIndexEntry(cursor.Value())
		if err != nil {
			return err
		}
		header := &entry.header

		// Determine the parent block node.  Since the block headers are
		// iterated in order of height, there is a very good chance the
		// previous header processed is the parent.
		var parent *blockNode
		if lastNode == nil {
			blockHash := header.BlockHash()
			if blockHash != *genesisHash {
				return AssertError(fmt.Sprintf("loadBlockIndex: expected "+
					"first entry in block index to be genesis block, "+
					"found %s", blockHash))
			}
		} else if header.PrevBlock == lastNode.hash {
			parent = lastNode
		} else {
			parent = index.lookupNode(&header.PrevBlock)
			if parent == nil {
				return AssertError(fmt.Sprintf("loadBlockIndex: could not "+
					"find parent for block %s", header.BlockHash()))
			}
		}

		// Initialize the block node, connect it, and add it to the block
		// index.
		node := &blockNodes[i]
		initBlockNode(node, header, parent)
		node.status = entry.status
		node.isFullyLinked = parent == nil || index.canValidate(parent)
		node.votes = entry.voteInfo
		index.addNodeFromDB(node)

		lastNode = node
		i++
	}

	return nil
}

// initChainState attempts to load and initialize the chain state from the
// database.  When the db does not yet contain any chain state, both it and the
// chain state are initialized to the genesis block.
func (b *BlockChain) initChainState(ctx context.Context,
	utxoBackend UtxoBackend) error {

	// Update database versioning scheme if needed.
	err := b.db.Update(func(dbTx database.Tx) error {
		// No versioning upgrade is needed if the dbinfo bucket does not
		// exist or the legacy key does not exist.
		bucket := dbTx.Metadata().Bucket(bcdbInfoBucketName)
		if bucket == nil {
			return nil
		}
		legacyBytes := bucket.Get(bcdbInfoBucketName)
		if legacyBytes == nil {
			return nil
		}

		// No versioning upgrade is needed if the new version key exists.
		if bucket.Get(bcdbInfoVersionKeyName) != nil {
			return nil
		}

		// Load and deserialize the legacy version information.
		log.Infof("Migrating versioning scheme...")
		dbi, err := deserializeDatabaseInfoV2(legacyBytes)
		if err != nil {
			return err
		}

		// Store the database version info using the new format.
		if err := dbPutDatabaseInfo(dbTx, dbi); err != nil {
			return err
		}

		// Remove the legacy version information.
		return bucket.Delete(bcdbInfoBucketName)
	})
	if err != nil {
		return err
	}

	// Determine the state of the database.
	var isStateInitialized bool
	err = b.db.View(func(dbTx database.Tx) error {
		// Fetch the database versioning information.
		dbInfo, err := dbFetchDatabaseInfo(dbTx)
		if err != nil {
			return err
		}

		// The database bucket for the versioning information is missing.
		if dbInfo == nil {
			return nil
		}

		// Don't allow downgrades of the blockchain database.
		if dbInfo.version > currentDatabaseVersion {
			return fmt.Errorf("the current blockchain database is "+
				"no longer compatible with this version of "+
				"the software (%d > %d)", dbInfo.version,
				currentDatabaseVersion)
		}

		// Don't allow downgrades of the database compression version.
		if dbInfo.compVer > currentCompressionVersion {
			return fmt.Errorf("the current database compression "+
				"version is no longer compatible with this "+
				"version of the software (%d > %d)",
				dbInfo.compVer, currentCompressionVersion)
		}

		// Don't allow downgrades of the block index.
		if dbInfo.bidxVer > currentBlockIndexVersion {
			return fmt.Errorf("the current database block index "+
				"version is no longer compatible with this "+
				"version of the software (%d > %d)",
				dbInfo.bidxVer, currentBlockIndexVersion)
		}

		b.dbInfo = dbInfo
		isStateInitialized = true
		return nil
	})
	if err != nil {
		return err
	}

	// Initialize the database if it has not already been done.
	if !isStateInitialized {
		if err := b.createChainState(); err != nil {
			return err
		}
	}

	// Initialize the UTXO database info.  This must be initialized after the
	// block database info is loaded, but before block database migrations are
	// run, since setting the initial UTXO set version depends on the block
	// database version as that is where it originally resided.
	if err := utxoBackend.InitInfo(b.dbInfo.version); err != nil {
		return err
	}

	// Upgrade the database as needed.
	err = upgradeDB(ctx, b.db, b.chainParams, b.dbInfo)
	if err != nil {
		return err
	}

	// Attempt to load the chain state and block index from the database.
	var tip *blockNode
	err = b.db.View(func(dbTx database.Tx) error {
		// Fetch the stored best chain state from the database.
		state, err := dbFetchBestState(dbTx)
		if err != nil {
			return err
		}

		log.Infof("Loading block index...")
		bidxStart := time.Now()

		// Load all of the block index entries from the database and
		// construct the block index.
		err = loadBlockIndex(dbTx, &b.chainParams.GenesisHash, b.index)
		if err != nil {
			return err
		}

		// Set the best chain to the stored best state.
		tip = b.index.lookupNode(&state.hash)
		if tip == nil {
			return AssertError(fmt.Sprintf("initChainState: cannot find "+
				"chain tip %s in block index", state.hash))
		}
		b.bestChain.SetTip(tip)
		b.index.MaybePruneCachedTips(tip)

		// Add the best chain tip to the set of candidates since it is required
		// to have the current best tip in it at all times.
		b.index.addBestChainCandidate(tip)

		log.Debugf("Block index loaded in %v", time.Since(bidxStart))

		// Exception for version 1 blockchains: skip loading the stake node, as
		// the upgrade path handles ensuring this is correctly set.
		if b.dbInfo.version >= 2 {
			tip.stakeNode, err = stake.LoadBestNode(dbTx, uint32(tip.height),
				tip.hash, tip.Header(), b.chainParams)
			if err != nil {
				return err
			}
			tip.newTickets = tip.stakeNode.NewTickets()
		}

		// Load the best and parent blocks and cache them.
		utilBlock, err := dbFetchBlockByNode(dbTx, tip)
		if err != nil {
			return err
		}
		b.addRecentBlock(utilBlock)
		if tip.parent != nil {
			parentBlock, err := dbFetchBlockByNode(dbTx, tip.parent)
			if err != nil {
				return err
			}
			b.addRecentBlock(parentBlock)
		}

		// Initialize the state related to the best block.
		block := utilBlock.MsgBlock()
		blockSize := uint64(block.SerializeSize())
		numTxns := uint64(len(block.Transactions))

		// Calculate the next stake difficulty.
		nextStakeDiff, err := b.calcNextRequiredStakeDifficulty(tip)
		if err != nil {
			return err
		}

		// Find the most recent checkpoint.
		if b.latestCheckpoint != nil {
			node := b.index.lookupNode(b.latestCheckpoint.Hash)
			if node != nil {
				log.Debugf("Most recent checkpoint is %s (height %d)",
					node.hash, node.height)
				b.checkpointNode = node
			}
		}

		b.stateSnapshot = newBestState(tip, blockSize, numTxns,
			state.totalTxns, tip.CalcPastMedianTime(),
			state.totalSubsidy, uint32(tip.stakeNode.PoolSize()),
			nextStakeDiff, tip.stakeNode.ExpiringNextBlock(), tip.stakeNode.Winners(),
			tip.stakeNode.MissedTickets(), tip.stakeNode.FinalState())

		return nil
	})
	if err != nil {
		return err
	}

	// Upgrade the spend journal as needed.
	return upgradeSpendJournal(ctx, b)
}

// dbFetchBlockByNode uses an existing database transaction to retrieve the raw
// block for the provided node, deserialize it, and return a dcrutil.Block.
func dbFetchBlockByNode(dbTx database.Tx, node *blockNode) (*dcrutil.Block, error) {
	// Load the raw block bytes from the database.
	blockBytes, err := dbTx.FetchBlock(&node.hash)
	if err != nil {
		return nil, err
	}

	// Create the encapsulated block and set the height appropriately.
	block, err := dcrutil.NewBlockFromBytes(blockBytes)
	if err != nil {
		return nil, err
	}

	return block, nil
}
