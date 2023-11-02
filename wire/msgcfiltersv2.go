// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// MaxCFiltersV2PerBatch is the maximum number of committed filters that may
// be sent in a cfiltersv2 message.
//
// This number has been decided assuming a sequence of blocks filled with
// transactions specifically designed to maximize the filter size, such that
// a cfiltersv2 message will not be larger than the maximum allowed P2P message
// size on any of the currently deployed networks.
const MaxCFiltersV2PerBatch = 100

func init() {
	// The following runtime assertions are included to ensure if any of
	// the input constants used to determine the MaxCFiltersV2PerBatch
	// number are changed from their original values, then that constant is
	// reviewed to still be compatible to the new protocol or consensus
	// constants.
	//
	// In particular, the max number of cfilters in a batched reply has
	// been determined by assuming a sequence of blocks of size
	// MaxBlockPayload (1.25 MiB), filled with a transaction with as many
	// OP_RETURNs as needed to fill a worst-case v2 filter (251581 bytes).
	//
	// At 100 filters per batch, with the added overhead of the commitment
	// proof, the maximum size of a cfiltersv2 message should be 25 MiB,
	// which is less than the maximum P2P message size of 32 MiB.
	//
	// Check the MaxTestSize test from the blockcf2 package for information
	// on how the maximum v2 cfilter size is determined.
	//
	// If any of these assertions break, such that a change to
	// MaxCFiltersV2PerBatch is necessary, then a new protocol version
	// should be introduced to allow a modification to
	// MaxCFiltersV2PerBatch.
	switch {
	case MaxCFiltersV2PerBatch != 100:
		panic("review MaxCFiltersV2PerBatch due to constant change")
	case MaxBlockPayload != 1310720:
		panic("review MaxCFiltersV2PerBatch due to MaxBlockPayload change")
	case MaxCFilterDataSize != 256*1024:
		panic("review MaxCFiltersV2PerBatch due to MaxCFilterDataSize change")
	case MaxMessagePayload != 1024*1024*32:
		panic("review MaxCFiltersV2PerBatch due to MaxMessagePayload change")
	case (&MsgCFiltersV2{}).MaxPayloadLength(BatchedCFiltersV2Version) != 26321001:
		panic("review MaxCFiltersV2PerBatch due to MaxPayloadLength change")
	case (&MsgCFiltersV2{}).MaxPayloadLength(ProtocolVersion) != 26321001:
		panic("review MaxCFiltersV2PerBatch due to MaxPayloadLength change")
	}
}

// MsgCFiltersV2 implements the Message interface and represents a cfiltersv2
// message.  It is used to deliver a batch of version 2 committed gcs filters
// for a given range of blocks, along with a proof that can be used to prove
// the filter is committed to by the block header.
//
// It is delivered in response to a getcfsv2 message (MsgGetCFiltersV2).
type MsgCFiltersV2 struct {
	CFilters []MsgCFilterV2
}

// BtcDecode decodes r using the Decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgCFiltersV2) BtcDecode(r io.Reader, pver uint32) error {
	const op = "MsgCFiltersV2.BtcDecode"
	if pver < BatchedCFiltersV2Version {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	nbCFilters, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	if nbCFilters > MaxCFiltersV2PerBatch {
		msg := fmt.Sprintf("%s too many cfilters sent in batch "+
			"[count %v max %v]", msg.Command(), nbCFilters,
			MaxCFiltersV2PerBatch)
		return messageError(op, ErrTooManyCFilters, msg)
	}

	msg.CFilters = make([]MsgCFilterV2, nbCFilters)
	for i := 0; i < int(nbCFilters); i++ {
		cf := &msg.CFilters[i]
		err := cf.BtcDecode(r, pver)
		if err != nil {
			return err
		}
	}

	return nil
}

// BtcEncode encodes the receiver to w using the Decred protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgCFiltersV2) BtcEncode(w io.Writer, pver uint32) error {
	const op = "MsgCFiltersV2.BtcEncode"
	if pver < BatchedCFiltersV2Version {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	nbCFilters := len(msg.CFilters)
	if nbCFilters > MaxCFiltersV2PerBatch {
		msg := fmt.Sprintf("%s too many cfilters to send in batch "+
			"[count %v max %v]", msg.Command(), nbCFilters,
			MaxCFiltersV2PerBatch)
		return messageError(op, ErrTooManyCFilters, msg)
	}

	err := WriteVarInt(w, pver, uint64(nbCFilters))
	if err != nil {
		return err
	}

	for i := 0; i < nbCFilters; i++ {
		err = msg.CFilters[i].BtcEncode(w, pver)
		if err != nil {
			return err
		}
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgCFiltersV2) Command() string {
	return CmdCFiltersV2
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgCFiltersV2) MaxPayloadLength(pver uint32) uint32 {
	// Varint + n * individual cfilter message:
	// Block hash + max filter data (including varint) +
	// proof index + max num proof hashes (including varint).
	return uint32(VarIntSerializeSize(MaxCFiltersV2PerBatch)) +
		(chainhash.HashSize+
			uint32(VarIntSerializeSize(MaxCFilterDataSize))+
			MaxCFilterDataSize+4+
			uint32(VarIntSerializeSize(MaxHeaderProofHashes))+
			(MaxHeaderProofHashes*chainhash.HashSize))*uint32(MaxCFiltersV2PerBatch)
}

// NewMsgCFiltersV2 returns a new cfiltersv2 message that conforms to the
// Message interface using the passed parameters and defaults for the remaining
// fields.
func NewMsgCFiltersV2(filters []MsgCFilterV2) *MsgCFiltersV2 {
	return &MsgCFiltersV2{
		CFilters: filters,
	}
}
