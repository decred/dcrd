// Copyright (c) 2019-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// MaxHeaderProofHashes is the maximum number of header commitment inclusion
// proof hashes that can be in a single message.  It is based on the fact that
// the proofs are logarithmic in nature and hence a value of 32 supports proofs
// of up to 2^32 commitments.
const MaxHeaderProofHashes = 32

// MsgCFilterV2 implements the Message interface and represents a cfilterv2
// message.  It is used to deliver a version 2 committed gcs filter for a given
// block along with a proof that can be used to prove the filter is committed to
// by the block header.  Note that the proof is only useful once the vote to
// enable header commitments is active.
//
// It is delivered in response to a getcfilterv2 message (MsgGetCFilterV2).
// Unknown blocks are ignored.
type MsgCFilterV2 struct {
	BlockHash   chainhash.Hash
	Data        []byte
	ProofIndex  uint32
	ProofHashes []chainhash.Hash
}

// BtcDecode decodes r using the Decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgCFilterV2) BtcDecode(r io.Reader, pver uint32) error {
	const op = "MsgCFilterV2.BtcDecode"
	if pver < CFilterV2Version {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	err := readElement(r, &msg.BlockHash)
	if err != nil {
		return err
	}

	msg.Data, err = ReadVarBytes(r, pver, MaxCFilterDataSize, "cfilterv2 data")
	if err != nil {
		return err
	}

	err = readElement(r, &msg.ProofIndex)
	if err != nil {
		return err
	}

	// Read num proof hashes and limit to max.
	count, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if count > MaxHeaderProofHashes {
		msg := fmt.Sprintf("too many proof hashes for message "+
			"[count %v, max %v]", count, MaxHeaderProofHashes)
		return messageError(op, ErrTooManyProofs, msg)
	}

	// Create a contiguous slice of hashes to deserialize into in order to
	// reduce the number of allocations.
	msg.ProofHashes = make([]chainhash.Hash, count)
	for i := uint64(0); i < count; i++ {
		err := readElement(r, &msg.ProofHashes[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// BtcEncode encodes the receiver to w using the Decred protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgCFilterV2) BtcEncode(w io.Writer, pver uint32) error {
	const op = "MsgCFilterV2.BtcEncode"
	if pver < CFilterV2Version {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	size := len(msg.Data)
	if size > MaxCFilterDataSize {
		msg := fmt.Sprintf("filter size too large for message "+
			"[size %v, max %v]", size, MaxCFilterDataSize)
		return messageError(op, ErrFilterTooLarge, msg)
	}

	numHashes := len(msg.ProofHashes)
	if numHashes > MaxHeaderProofHashes {
		msg := fmt.Sprintf("too many proof hashes for message "+
			"[count %v, max %v]", numHashes, MaxHeaderProofHashes)
		return messageError(op, ErrTooManyProofs, msg)
	}

	err := writeElement(w, &msg.BlockHash)
	if err != nil {
		return err
	}

	err = WriteVarBytes(w, pver, msg.Data)
	if err != nil {
		return err
	}

	err = writeElement(w, msg.ProofIndex)
	if err != nil {
		return err
	}

	err = WriteVarInt(w, pver, uint64(numHashes))
	if err != nil {
		return err
	}

	for _, hash := range msg.ProofHashes {
		err := writeElement(w, hash)
		if err != nil {
			return err
		}
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgCFilterV2) Command() string {
	return CmdCFilterV2
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgCFilterV2) MaxPayloadLength(pver uint32) uint32 {
	// Block hash + max filter data (including varint) +
	// proof index + max num proof hashes (including varint).
	return chainhash.HashSize +
		uint32(VarIntSerializeSize(MaxCFilterDataSize)) +
		MaxCFilterDataSize + 4 +
		uint32(VarIntSerializeSize(MaxHeaderProofHashes)) +
		(MaxHeaderProofHashes * chainhash.HashSize)
}

// NewMsgCFilterV2 returns a new cfilterv2 message that conforms to the Message
// interface using the passed parameters and defaults for the remaining fields.
func NewMsgCFilterV2(blockHash *chainhash.Hash, filterData []byte,
	proofIndex uint32, proofHashes []chainhash.Hash) *MsgCFilterV2 {

	return &MsgCFilterV2{
		BlockHash:   *blockHash,
		Data:        filterData,
		ProofIndex:  proofIndex,
		ProofHashes: proofHashes,
	}
}
