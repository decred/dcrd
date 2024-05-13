// Copyright (c) 2020-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// MaxISBlocksAtHeadPerMsg is the maximum number of block hashes allowed per
// message.
const MaxISBlocksAtHeadPerMsg = 8

// MaxISVotesAtHeadPerMsg is the maximum number of votes at head per message.
const MaxISVotesAtHeadPerMsg = 40 // 8 * 5

// MaxISTSpendsAtHeadPerMsg is the maximum number of tspends at head per
// message.
const MaxISTSpendsAtHeadPerMsg = 7

// MsgInitState implements the Message interface and represents an initial
// state message.  It is used to receive ephemeral startup information from a
// remote peer, such as blocks that can be mined upon, votes for such blocks
// and tspends.
//
// The content of such a message depends upon what the local peer requested
// during a previous GetInitState msg.
type MsgInitState struct {
	BlockHashes  []chainhash.Hash
	VoteHashes   []chainhash.Hash
	TSpendHashes []chainhash.Hash
}

// AddBlockHash adds a new block hash to the message. Up to
// MaxISBlocksAtHeadPerMsg may be added before this function errors out.
func (msg *MsgInitState) AddBlockHash(hash *chainhash.Hash) error {
	const op = "MsgInitState.AddBlockHash"
	if len(msg.BlockHashes)+1 > MaxISBlocksAtHeadPerMsg {
		msg := fmt.Sprintf("too many block hashes for message [max %v]",
			MaxISBlocksAtHeadPerMsg)
		return messageError(op, ErrTooManyHeaders, msg)
	}

	msg.BlockHashes = append(msg.BlockHashes, *hash)
	return nil
}

// AddVoteHash adds a new vote hash to the message. Up to
// MaxISVotesAtHeadPerMsg may be added before this function errors out.
func (msg *MsgInitState) AddVoteHash(hash *chainhash.Hash) error {
	const op = "MsgInitState.AddVoteHash"
	if len(msg.VoteHashes)+1 > MaxISVotesAtHeadPerMsg {
		msg := fmt.Sprintf("too many vote hashes for message [max %v]",
			MaxISVotesAtHeadPerMsg)
		return messageError(op, ErrTooManyVotes, msg)
	}

	msg.VoteHashes = append(msg.VoteHashes, *hash)
	return nil
}

// AddTSpendHash adds a new treasury spend hash to the message.  Up to
// MaxISTSpendsAtHeadPerMsg may be added before this function errors out.
func (msg *MsgInitState) AddTSpendHash(hash *chainhash.Hash) error {
	const op = "MsgInitState.AddTSpendHash"
	if len(msg.TSpendHashes)+1 > MaxISTSpendsAtHeadPerMsg {
		msg := fmt.Sprintf("too many tspend hashes for message [max %v]",
			MaxISTSpendsAtHeadPerMsg)
		return messageError(op, ErrTooManyTSpends, msg)
	}

	msg.TSpendHashes = append(msg.TSpendHashes, *hash)
	return nil
}

// BtcDecode decodes r using the protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgInitState) BtcDecode(r io.Reader, pver uint32) error {
	const op = "MsgInitState.BtcDecode"
	if pver < InitStateVersion {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	// Read num block hashes and limit to max.
	count, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if count > MaxISBlocksAtHeadPerMsg {
		msg := fmt.Sprintf("too many block hashes for message "+
			"[count %v, max %v]", count, MaxISBlocksAtHeadPerMsg)
		return messageError(op, ErrTooManyBlocks, msg)
	}

	msg.BlockHashes = make([]chainhash.Hash, count)
	for i := uint64(0); i < count; i++ {
		err := readElement(r, &msg.BlockHashes[i])
		if err != nil {
			return err
		}
	}

	// Read num vote hashes and limit to max.
	count, err = ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if count > MaxISVotesAtHeadPerMsg {
		msg := fmt.Sprintf("too many vote hashes for message "+
			"[count %v, max %v]", count, MaxISVotesAtHeadPerMsg)
		return messageError(op, ErrTooManyVotes, msg)
	}

	msg.VoteHashes = make([]chainhash.Hash, count)
	for i := uint64(0); i < count; i++ {
		err := readElement(r, &msg.VoteHashes[i])
		if err != nil {
			return err
		}
	}

	// Read num tspend hashes and limit to max.
	count, err = ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if count > MaxISTSpendsAtHeadPerMsg {
		msg := fmt.Sprintf("too many tspend hashes for message "+
			"[count %v, max %v]", count, MaxISTSpendsAtHeadPerMsg)
		return messageError(op, ErrTooManyTSpends, msg)
	}

	msg.TSpendHashes = make([]chainhash.Hash, count)
	for i := uint64(0); i < count; i++ {
		err := readElement(r, &msg.TSpendHashes[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// BtcEncode encodes the receiver to w using the protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgInitState) BtcEncode(w io.Writer, pver uint32) error {
	const op = "MsgInitState.BtcEncode"
	if pver < InitStateVersion {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	// Write block hashes.
	count := len(msg.BlockHashes)
	if count > MaxISBlocksAtHeadPerMsg {
		msg := fmt.Sprintf("too many block hashes for message "+
			"[count %v, max %v]", count, MaxISBlocksAtHeadPerMsg)
		return messageError(op, ErrTooManyBlocks, msg)
	}

	err := WriteVarInt(w, pver, uint64(count))
	if err != nil {
		return err
	}

	for i := range msg.BlockHashes {
		err = writeElement(w, &msg.BlockHashes[i])
		if err != nil {
			return err
		}
	}

	// Write vote hashes.
	count = len(msg.VoteHashes)
	if count > MaxISVotesAtHeadPerMsg {
		msg := fmt.Sprintf("too many vote hashes for message "+
			"[count %v, max %v]", count, MaxISVotesAtHeadPerMsg)
		return messageError(op, ErrTooManyVotes, msg)
	}

	err = WriteVarInt(w, pver, uint64(count))
	if err != nil {
		return err
	}

	for i := range msg.VoteHashes {
		err = writeElement(w, &msg.VoteHashes[i])
		if err != nil {
			return err
		}
	}

	// Write tspend hashes.
	count = len(msg.TSpendHashes)
	if count > MaxISTSpendsAtHeadPerMsg {
		msg := fmt.Sprintf("too many tspend hashes for message "+
			"[count %v, max %v]", count, MaxISTSpendsAtHeadPerMsg)
		return messageError(op, ErrTooManyTSpends, msg)
	}

	err = WriteVarInt(w, pver, uint64(count))
	if err != nil {
		return err
	}

	for i := range msg.TSpendHashes {
		err = writeElement(w, &msg.TSpendHashes[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgInitState) Command() string {
	return CmdInitState
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgInitState) MaxPayloadLength(pver uint32) uint32 {
	if pver < InitStateVersion {
		return 0
	}

	return uint32(VarIntSerializeSize(MaxISBlocksAtHeadPerMsg)) +
		(MaxISBlocksAtHeadPerMsg * chainhash.HashSize) +
		uint32(VarIntSerializeSize(MaxISVotesAtHeadPerMsg)) +
		(MaxISVotesAtHeadPerMsg * chainhash.HashSize) +
		uint32(VarIntSerializeSize(MaxISTSpendsAtHeadPerMsg)) +
		(MaxISTSpendsAtHeadPerMsg * chainhash.HashSize)
}

// NewMsgInitState returns a new Decred initstate message that conforms to the
// Message interface using the defaults for the fields.
func NewMsgInitState() *MsgInitState {
	return &MsgInitState{
		BlockHashes:  make([]chainhash.Hash, 0, MaxISBlocksAtHeadPerMsg),
		VoteHashes:   make([]chainhash.Hash, 0, MaxISVotesAtHeadPerMsg),
		TSpendHashes: make([]chainhash.Hash, 0, MaxISTSpendsAtHeadPerMsg),
	}
}

// NewMsgInitStateFilled returns a new Decred initstate message that conforms
// to the Message interface and fills the message with the provided data. This
// is useful in situations where the data slices are already built as it avoids
// performing a second allocation and data copy.
//
// The provided slices are checked for their maximum length.
func NewMsgInitStateFilled(blockHashes []chainhash.Hash, voteHashes []chainhash.Hash,
	tspendHashes []chainhash.Hash) (*MsgInitState, error) {
	const op = "NewMsgInitStateFilled"

	count := len(blockHashes)
	if count > MaxISBlocksAtHeadPerMsg {
		msg := fmt.Sprintf("too many block hashes for message "+
			"[count %v, max %v]", count, MaxISBlocksAtHeadPerMsg)
		return nil, messageError(op, ErrTooManyBlocks, msg)
	}

	count = len(voteHashes)
	if count > MaxISVotesAtHeadPerMsg {
		msg := fmt.Sprintf("too many vote hashes for message "+
			"[count %v, max %v]", count, MaxISVotesAtHeadPerMsg)
		return nil, messageError(op, ErrTooManyVotes, msg)
	}

	count = len(tspendHashes)
	if count > MaxISTSpendsAtHeadPerMsg {
		msg := fmt.Sprintf("too many tspend hashes for message "+
			"[count %v, max %v]", count, MaxISTSpendsAtHeadPerMsg)
		return nil, messageError(op, ErrTooManyTSpends, msg)
	}

	return &MsgInitState{
		BlockHashes:  blockHashes,
		VoteHashes:   voteHashes,
		TSpendHashes: tspendHashes,
	}, nil
}
