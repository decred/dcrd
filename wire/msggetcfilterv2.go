// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// MsgGetCFilterV2 implements the Message interface and represents a decred
// getcfilterv2 message.  It is used to request a version 2 committed gcs filter
// for a given block along with a proof that can be used to prove the filter is
// committed to by the block header.  Note that the proof is only useful once
// the vote to enable header commitments is active.  The filter is returned via
// a cfilterv2 message (MsgCFilterV2).  Unknown blocks are ignored.
type MsgGetCFilterV2 struct {
	BlockHash chainhash.Hash
}

// BtcDecode decodes r using the Decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgGetCFilterV2) BtcDecode(r io.Reader, pver uint32) error {
	if pver < CFilterV2Version {
		str := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError("MsgGetCFilterV2.BtcDecode", str)
	}

	return readElement(r, &msg.BlockHash)
}

// BtcEncode encodes the receiver to w using the Decred protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetCFilterV2) BtcEncode(w io.Writer, pver uint32) error {
	if pver < CFilterV2Version {
		str := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError("MsgGetCFilterV2.BtcEncode", str)
	}

	return writeElement(w, &msg.BlockHash)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgGetCFilterV2) Command() string {
	return CmdGetCFilterV2
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgGetCFilterV2) MaxPayloadLength(pver uint32) uint32 {
	// Block hash.
	return chainhash.HashSize
}

// NewMsgGetCFilterV2 returns a new Decred getcfilterv2 message that conforms
// to the Message interface using the passed parameters.
func NewMsgGetCFilterV2(blockHash *chainhash.Hash) *MsgGetCFilterV2 {
	return &MsgGetCFilterV2{
		BlockHash: *blockHash,
	}
}
