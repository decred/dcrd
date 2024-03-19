// Copyright (c) 2023-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"hash"
	"io"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// MsgMixConfirm contains a partially-signed mix transaction, with signatures
// contributed from the peer identity.  When all CM messages are received,
// signatures can be merged and the transaction may be published, ending a
// successful mix session.
//
// It implements the Message interface.
type MsgMixConfirm struct {
	Signature  [64]byte
	Identity   [33]byte
	SessionID  [32]byte
	Run        uint32
	Mix        MsgTx
	SeenDCNets []chainhash.Hash

	// hash records the hash of the message.  It is a member of the
	// message for convenience and performance, but is never automatically
	// set during creation or deserialization.
	hash chainhash.Hash
}

// BtcDecode decodes r using the Decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgMixConfirm) BtcDecode(r io.Reader, pver uint32) error {
	const op = "MsgMixConfirm.BtcDecode"
	if pver < MixVersion {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	err := readElements(r, &msg.Signature, &msg.Identity, &msg.SessionID,
		&msg.Run)
	if err != nil {
		return err
	}

	err = msg.Mix.BtcDecode(r, pver)
	if err != nil {
		return err
	}

	count, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if count > MaxMixPeers {
		msg := fmt.Sprintf("too many previous referenced messages [count %v, max %v]",
			count, MaxMixPeers)
		return messageError(op, ErrTooManyPrevMixMsgs, msg)
	}

	seen := make([]chainhash.Hash, count)
	for i := range seen {
		err := readElement(r, &seen[i])
		if err != nil {
			return err
		}
	}
	msg.SeenDCNets = seen

	return nil
}

// BtcEncode encodes the receiver to w using the Decred protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgMixConfirm) BtcEncode(w io.Writer, pver uint32) error {
	const op = "MsgMixConfirm.BtcEncode"
	if pver < MixVersion {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	err := writeElement(w, &msg.Signature)
	if err != nil {
		return err
	}

	err = msg.writeMessageNoSignature(op, w, pver)
	if err != nil {
		return err
	}

	return nil
}

// Hash returns the message hash calculated by WriteHash.
//
// Hash returns an invalid or zero hash if WriteHash has not been called yet.
//
// This method is not safe while concurrently calling WriteHash.
func (msg *MsgMixConfirm) Hash() chainhash.Hash {
	return msg.hash
}

// WriteHash serializes the message to a hasher and records the sum in the
// message's Hash field.
//
// The hasher's Size() must equal chainhash.HashSize, or this method will
// panic.  This method is designed to work only with hashers returned by
// blake256.New.
func (msg *MsgMixConfirm) WriteHash(h hash.Hash) {
	h.Reset()
	writeElement(h, &msg.Signature)
	msg.writeMessageNoSignature("", h, MixVersion)
	sum := h.Sum(msg.hash[:0])
	if len(sum) != len(msg.hash) {
		s := fmt.Sprintf("hasher type %T has invalid Size() for chainhash.Hash", h)
		panic(s)
	}
}

// writeMessageNoSignature serializes all elements of the message except for
// the signature.  This allows code reuse between message serialization, and
// signing and verifying these message contents.
//
// If w implements hash.Hash, no errors will be returned for invalid message
// construction.
func (msg *MsgMixConfirm) writeMessageNoSignature(op string, w io.Writer, pver uint32) error {
	_, hashing := w.(hash.Hash)

	count := len(msg.SeenDCNets)
	if !hashing && count > MaxMixPeers {
		msg := fmt.Sprintf("too many previous referenced messages [count %v, max %v]",
			count, MaxMixPeers)
		return messageError(op, ErrTooManyPrevMixMsgs, msg)
	}

	err := writeElements(w, &msg.Identity, &msg.SessionID, msg.Run)
	if err != nil {
		return err
	}

	err = msg.Mix.BtcEncode(w, pver)
	if err != nil {
		return err
	}

	err = WriteVarInt(w, pver, uint64(count))
	if err != nil {
		return err
	}
	for i := range msg.SeenDCNets {
		err = writeElement(w, &msg.SeenDCNets[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// WriteSignedData writes a tag identifying the message data, followed by all
// message fields excluding the signature.  This is the data committed to when
// the message is signed.
func (msg *MsgMixConfirm) WriteSignedData(h hash.Hash) {
	WriteVarString(h, MixVersion, CmdMixConfirm+"-sig")
	msg.writeMessageNoSignature("", h, MixVersion)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgMixConfirm) Command() string {
	return CmdMixConfirm
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgMixConfirm) MaxPayloadLength(pver uint32) uint32 {
	if pver < MixVersion {
		return 0
	}

	// See tests for this calculation.
	return 1016520
}

// Pub returns the message sender's public key identity.
func (msg *MsgMixConfirm) Pub() []byte {
	return msg.Identity[:]
}

// Sig returns the message signature.
func (msg *MsgMixConfirm) Sig() []byte {
	return msg.Signature[:]
}

// PrevMsgs returns the previous DC messages seen by the peer.
func (msg *MsgMixConfirm) PrevMsgs() []chainhash.Hash {
	return msg.SeenDCNets
}

// Sid returns the session ID.
func (msg *MsgMixConfirm) Sid() []byte {
	return msg.SessionID[:]
}

// GetRun returns the run number.
func (msg *MsgMixConfirm) GetRun() uint32 {
	return msg.Run
}

// NewMsgMixConfirm returns a new mixconfirm message that conforms to the
// Message interface using the passed parameters and defaults for the
// remaining fields.
func NewMsgMixConfirm(identity [33]byte, sid [32]byte, run uint32,
	mix *MsgTx, seenDCNets []chainhash.Hash) *MsgMixConfirm {

	if mix == nil {
		mix = NewMsgTx()
	}

	return &MsgMixConfirm{
		Identity:   identity,
		SessionID:  sid,
		Run:        run,
		Mix:        *mix,
		SeenDCNets: seenDCNets,
	}
}
