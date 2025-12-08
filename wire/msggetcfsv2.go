// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// MsgGetCFsV2 implements the Message interface and represents a decred
// getcfsv2 message.  It is used to request a batch of version 2 committed
// filters that span a subset of a chain, from StartHash up to (and including)
// EndHash.  The response is sent in a MsgCFiltersV2 message.
//
// At most MaxCFiltersV2PerBatch may be requested by each MsgGetCFsV2
// message, which means the number of blocks between EndHash and StartHash must
// be lesser than or equal to that constant's value.
type MsgGetCFsV2 struct {
	StartHash chainhash.Hash
	EndHash   chainhash.Hash
}

// BtcDecode decodes r using the Decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgGetCFsV2) BtcDecode(r io.Reader, pver uint32) error {
	const op = "MsgGetCFsV2.BtcDecode"
	if pver < BatchedCFiltersV2Version {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	return readElements(r, &msg.StartHash, &msg.EndHash)
}

// BtcEncode encodes the receiver to w using the Decred protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetCFsV2) BtcEncode(w io.Writer, pver uint32) error {
	const op = "MsgGetCFsV2.BtcEncode"
	if pver < BatchedCFiltersV2Version {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	_, err := writeElements(w, &msg.StartHash, &msg.EndHash)
	return err
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgGetCFsV2) Command() string {
	return CmdGetCFiltersV2
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgGetCFsV2) MaxPayloadLength(pver uint32) uint32 {
	// Block hash.
	return chainhash.HashSize * 2
}

// NewMsgGetCFsV2 returns a new Decred getcfiltersv2 message that conforms
// to the Message interface using the passed parameters.
func NewMsgGetCFsV2(startHash, endHash *chainhash.Hash) *MsgGetCFsV2 {
	return &MsgGetCFsV2{
		StartHash: *startHash,
		EndHash:   *endHash,
	}
}
