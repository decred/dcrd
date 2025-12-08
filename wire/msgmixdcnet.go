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

// MsgMixDCNet is the DC-net broadcast.  It implements the Message interface.
type MsgMixDCNet struct {
	Signature        [64]byte
	Identity         [33]byte
	SessionID        [32]byte
	Run              uint32
	DCNet            []MixVect
	SeenSlotReserves []chainhash.Hash

	// hash records the hash of the message.  It is a member of the
	// message for convenience and performance, but is never automatically
	// set during creation or deserialization.
	hash chainhash.Hash
}

// BtcDecode decodes r using the Decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgMixDCNet) BtcDecode(r io.Reader, pver uint32) error {
	const op = "MsgMixDCNet.BtcDecode"
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

	var dcnet []MixVect
	err = readMixVects(op, r, pver, &dcnet)
	if err != nil {
		return err
	}
	msg.DCNet = dcnet

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
	msg.SeenSlotReserves = seen

	return nil
}

// BtcEncode encodes the receiver to w using the Decred protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgMixDCNet) BtcEncode(w io.Writer, pver uint32) error {
	const op = "MsgMixDCNet.BtcEncode"
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
func (msg *MsgMixDCNet) Hash() chainhash.Hash {
	return msg.hash
}

// WriteHash serializes the message to a hasher and records the sum in the
// message's Hash field.
//
// The hasher's Size() must equal chainhash.HashSize, or this method will
// panic.  This method is designed to work only with hashers returned by
// blake256.New.
func (msg *MsgMixDCNet) WriteHash(h hash.Hash) {
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
func (msg *MsgMixDCNet) writeMessageNoSignature(op string, w io.Writer, pver uint32) error {
	_, hashing := w.(hash.Hash)

	mcount := len(msg.DCNet)
	if !hashing && mcount == 0 {
		msg := fmt.Sprintf("too few mixed messages [%v]", mcount)
		return messageError(op, ErrInvalidMsg, msg)
	}
	if !hashing && mcount > MaxMixMcount {
		msg := fmt.Sprintf("too many total mixed messages [%v]", mcount)
		return messageError(op, ErrInvalidMsg, msg)
	}
	srcount := len(msg.SeenSlotReserves)
	if !hashing && srcount > MaxMixPeers {
		msg := fmt.Sprintf("too many previous referenced messages [count %v, max %v]",
			srcount, MaxMixPeers)
		return messageError(op, ErrTooManyPrevMixMsgs, msg)
	}

	err := writeElements(w, &msg.Identity, &msg.SessionID, &msg.Run)
	if err != nil {
		return err
	}

	err = writeMixVects(w, pver, msg.DCNet)
	if err != nil {
		return err
	}

	err = WriteVarInt(w, pver, uint64(srcount))
	if err != nil {
		return err
	}
	for i := range msg.SeenSlotReserves {
		err = writeElement(w, &msg.SeenSlotReserves[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func writeMixVects(w io.Writer, pver uint32, vecs []MixVect) error {
	// Write dimensions
	err := WriteVarInt(w, pver, uint64(len(vecs)))
	if err != nil {
		return err
	}
	if len(vecs) == 0 {
		return nil
	}
	err = WriteVarInt(w, pver, uint64(len(vecs[0])))
	if err != nil {
		return err
	}
	err = WriteVarInt(w, pver, MixMsgSize)
	if err != nil {
		return err
	}

	// Write messages
	for i := range vecs {
		for j := range vecs[i] {
			err = writeElement(w, &vecs[i][j])
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func readMixVects(op string, r io.Reader, pver uint32, vecs *[]MixVect) error {
	// Read dimensions
	x, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if x == 0 {
		return nil
	}
	y, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	msize, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	if x > MaxMixMcount || y > MaxMixMcount {
		msg := "DC-net mix vector dimensions are too large for maximum message count"
		return messageError(op, ErrInvalidMsg, msg)
	}
	if msize != MixMsgSize {
		msg := fmt.Sprintf("mixed message length must be %d [got: %d]",
			MixMsgSize, msize)
		return messageError(op, ErrInvalidMsg, msg)
	}

	// Read messages
	*vecs = make([]MixVect, x)
	for i := uint64(0); i < x; i++ {
		(*vecs)[i] = make(MixVect, y)
		for j := uint64(0); j < y; j++ {
			err = readElement(r, &(*vecs)[i][j])
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// WriteSignedData writes a tag identifying the message data, followed by all
// message fields excluding the signature.  This is the data committed to when
// the message is signed.
func (msg *MsgMixDCNet) WriteSignedData(h hash.Hash) {
	WriteVarString(h, MixVersion, CmdMixDCNet+"-sig")
	msg.writeMessageNoSignature("", h, MixVersion)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgMixDCNet) Command() string {
	return CmdMixDCNet
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgMixDCNet) MaxPayloadLength(pver uint32) uint32 {
	if pver < MixVersion {
		return 0
	}

	// See tests for this calculation.
	return 20988047
}

// Pub returns the message sender's public key identity.
func (msg *MsgMixDCNet) Pub() []byte {
	return msg.Identity[:]
}

// Sig returns the message signature.
func (msg *MsgMixDCNet) Sig() []byte {
	return msg.Signature[:]
}

// PrevMsgs returns the previous SR messages seen by the peer.
func (msg *MsgMixDCNet) PrevMsgs() []chainhash.Hash {
	return msg.SeenSlotReserves
}

// Sid returns the session ID.
func (msg *MsgMixDCNet) Sid() []byte {
	return msg.SessionID[:]
}

// GetRun returns the run number.
func (msg *MsgMixDCNet) GetRun() uint32 {
	return msg.Run
}

// NewMsgMixDCNet returns a new mixdcnet message that conforms to the Message
// interface using the passed parameters and defaults for the remaining
// fields.
func NewMsgMixDCNet(identity [33]byte, sid [32]byte, run uint32,
	dcnet []MixVect, seenSlotReserves []chainhash.Hash) *MsgMixDCNet {

	return &MsgMixDCNet{
		Identity:         identity,
		SessionID:        sid,
		Run:              run,
		DCNet:            dcnet,
		SeenSlotReserves: seenSlotReserves,
	}
}
