// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"hash"
	"io"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// MsgMixFactoredPoly encodes the solution of the factored slot reservation
// polynomial.
type MsgMixFactoredPoly struct {
	Signature        [64]byte
	Identity         [33]byte
	SessionID        [32]byte
	Run              uint32
	Roots            [][]byte
	SeenSlotReserves []chainhash.Hash

	// hash records the hash of the message.  It is a member of the
	// message for convenience and performance, but is never automatically
	// set during creation or deserialization.
	hash chainhash.Hash
}

// BtcDecode decodes r using the Decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgMixFactoredPoly) BtcDecode(r io.Reader, pver uint32) error {
	const op = "MsgMixFactoredPoly.BtcDecode"
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

	count, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if count > MaxMixMcount {
		msg := fmt.Sprintf("too many roots in message [count %v, max %v]",
			count, MaxMixMcount)
		return messageError(op, ErrInvalidMsg, msg)
	}

	roots := make([][]byte, count)
	for i := range roots {
		root, err := ReadVarBytes(r, pver, MaxMixFieldValLen, "MixFactoredPoly.Roots")
		if err != nil {
			return err
		}
		roots[i] = root
	}
	msg.Roots = roots

	count, err = ReadVarInt(r, pver)
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
func (msg *MsgMixFactoredPoly) BtcEncode(w io.Writer, pver uint32) error {
	const op = "MsgMixFactoredPoly.BtcEncode"
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
func (msg *MsgMixFactoredPoly) Hash() chainhash.Hash {
	return msg.hash
}

// WriteHash serializes the message to a hasher and records the sum in the
// message's Hash field.
//
// The hasher's Size() must equal chainhash.HashSize, or this method will
// panic.  This method is designed to work only with hashers returned by
// blake256.New.
func (msg *MsgMixFactoredPoly) WriteHash(h hash.Hash) {
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
func (msg *MsgMixFactoredPoly) writeMessageNoSignature(op string, w io.Writer, pver uint32) error {
	_, hashing := w.(hash.Hash)

	count := len(msg.Roots)
	if !hashing && count > MaxMixMcount {
		msg := fmt.Sprintf("too many solutions to factored polynomial [count %v, max %v]",
			count, MaxMixMcount)
		return messageError(op, ErrInvalidMsg, msg)
	}
	for _, root := range msg.Roots {
		if !hashing && len(root) > MaxMixFieldValLen {
			msg := "root exceeds bytes necessary to represent number in field"
			return messageError(op, ErrInvalidMsg, msg)
		}
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

	err = WriteVarInt(w, pver, uint64(count))
	if err != nil {
		return err
	}
	for _, root := range msg.Roots {
		err := WriteVarBytes(w, pver, root)
		if err != nil {
			return err
		}
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

// WriteSignedData writes a tag identifying the message data, followed by all
// message fields excluding the signature.  This is the data committed to when
// the message is signed.
func (msg *MsgMixFactoredPoly) WriteSignedData(h hash.Hash) {
	WriteVarString(h, MixVersion, CmdMixFactoredPoly+"-sig")
	msg.writeMessageNoSignature("", h, MixVersion)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgMixFactoredPoly) Command() string {
	return CmdMixFactoredPoly
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgMixFactoredPoly) MaxPayloadLength(pver uint32) uint32 {
	if pver < MixVersion {
		return 0
	}

	// See tests for this calculation.
	return 49291
}

// Pub returns the message sender's public key identity.
func (msg *MsgMixFactoredPoly) Pub() []byte {
	return msg.Identity[:]
}

// Sig returns the message signature.
func (msg *MsgMixFactoredPoly) Sig() []byte {
	return msg.Signature[:]
}

// PrevMsgs returns the previous SR messages seen by the peer.
func (msg *MsgMixFactoredPoly) PrevMsgs() []chainhash.Hash {
	return msg.SeenSlotReserves
}

// Sid returns the session ID.
func (msg *MsgMixFactoredPoly) Sid() []byte {
	return msg.SessionID[:]
}

// GetRun returns the run number.
func (msg *MsgMixFactoredPoly) GetRun() uint32 {
	return msg.Run
}

// NewMsgMixFactoredPoly returns a new mixpairreq message that conforms to the
// Message interface using the passed parameters and defaults for the
// remaining fields.
func NewMsgMixFactoredPoly(identity [33]byte, sid [32]byte, run uint32,
	roots [][]byte, seenSlotReserves []chainhash.Hash) *MsgMixFactoredPoly {

	return &MsgMixFactoredPoly{
		Identity:         identity,
		SessionID:        sid,
		Run:              run,
		Roots:            roots,
		SeenSlotReserves: seenSlotReserves,
	}
}
