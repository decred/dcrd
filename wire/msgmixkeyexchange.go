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

const (
	// MaxMixPeers is the maximum number of peers allowed together in a
	// single mix.  This restricts the maximum dimensions of the slot
	// reservation and XOR DC-net matrices and the maximum number of
	// previous messages that may be referenced by mix messages.
	// This value is an high estimate of what a large mix may look like,
	// based on statistics from the centralized mixing server.
	MaxMixPeers = 512
)

// MsgMixKeyExchange implements the Message interface and represents a mixing key
// exchange message.  It includes a commitment to secrets (private keys and
// discarded mixed addresses) in case they must be revealed for blame
// assignment.
type MsgMixKeyExchange struct {
	Signature  [64]byte
	Identity   [33]byte
	SessionID  [32]byte
	Epoch      uint64
	Run        uint32
	Pos        uint32
	ECDH       [33]byte   // Secp256k1 public key
	PQPK       [1218]byte // Sntrup4591761 public key
	Commitment [32]byte
	SeenPRs    []chainhash.Hash

	// hash records the hash of the message.  It is a member of the
	// message for convenience and performance, but is never automatically
	// set during creation or deserialization.
	hash chainhash.Hash
}

// BtcDecode decodes r using the Decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgMixKeyExchange) BtcDecode(r io.Reader, pver uint32) error {
	const op = "MsgMixKeyExchange.BtcDecode"
	if pver < MixVersion {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	err := readElements(r, &msg.Signature, &msg.Identity, &msg.SessionID,
		&msg.Epoch, &msg.Run, &msg.Pos, &msg.ECDH, &msg.PQPK, &msg.Commitment)
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
	msg.SeenPRs = seen

	return nil
}

// BtcEncode encodes the receiver to w using the Decred protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgMixKeyExchange) BtcEncode(w io.Writer, pver uint32) error {
	const op = "MsgMixKeyExchange.BtcEncode"
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
func (msg *MsgMixKeyExchange) Hash() chainhash.Hash {
	return msg.hash
}

// WriteHash serializes the message to a hasher and records the sum in the
// message's Hash field.
//
// The hasher's Size() must equal chainhash.HashSize, or this method will
// panic.  This method is designed to work only with hashers returned by
// blake256.New.
func (msg *MsgMixKeyExchange) WriteHash(h hash.Hash) {
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
func (msg *MsgMixKeyExchange) writeMessageNoSignature(op string, w io.Writer, pver uint32) error {
	_, hashing := w.(hash.Hash)

	// Limit to max previous messages hashes.
	count := len(msg.SeenPRs)
	if !hashing && count > MaxMixPeers {
		msg := fmt.Sprintf("too many previous referenced messages [count %v, max %v]",
			count, MaxMixPeers)
		return messageError(op, ErrTooManyPrevMixMsgs, msg)
	}

	err := writeElements(w, &msg.Identity, &msg.SessionID, msg.Epoch,
		msg.Run, &msg.Pos, &msg.ECDH, &msg.PQPK, &msg.Commitment)
	if err != nil {
		return err
	}

	err = WriteVarInt(w, pver, uint64(count))
	if err != nil {
		return err
	}
	for i := range msg.SeenPRs {
		err := writeElement(w, &msg.SeenPRs[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// WriteSignedData writes a tag identifying the message data, followed by all
// message fields excluding the signature.  This is the data committed to when
// the message is signed.
func (msg *MsgMixKeyExchange) WriteSignedData(h hash.Hash) {
	WriteVarString(h, MixVersion, CmdMixKeyExchange+"-sig")
	msg.writeMessageNoSignature("", h, MixVersion)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgMixKeyExchange) Command() string {
	return CmdMixKeyExchange
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgMixKeyExchange) MaxPayloadLength(pver uint32) uint32 {
	if pver < MixVersion {
		return 0
	}

	// See tests for this calculation.
	return 17815
}

// Pub returns the message sender's public key identity.
func (msg *MsgMixKeyExchange) Pub() []byte {
	return msg.Identity[:]
}

// Sig returns the message signature.
func (msg *MsgMixKeyExchange) Sig() []byte {
	return msg.Signature[:]
}

// PrevMsgs returns the previous PR messages seen by the peer.
func (msg *MsgMixKeyExchange) PrevMsgs() []chainhash.Hash {
	return msg.SeenPRs
}

// Sid returns the session ID.
func (msg *MsgMixKeyExchange) Sid() []byte {
	return msg.SessionID[:]
}

// GetRun returns the run number.
func (msg *MsgMixKeyExchange) GetRun() uint32 {
	return msg.Run
}

// NewMsgMixKeyExchange returns a new mixkeyxchg message that conforms to the
// Message interface using the passed parameters and defaults for the
// remaining fields.
func NewMsgMixKeyExchange(identity [33]byte, sid [32]byte, epoch uint64,
	run uint32, pos uint32, ecdh [33]byte, pqpk [1218]byte, commitment [32]byte,
	seenPRs []chainhash.Hash) *MsgMixKeyExchange {

	return &MsgMixKeyExchange{
		Identity:   identity,
		SessionID:  sid,
		Epoch:      epoch,
		Run:        run,
		Pos:        pos,
		ECDH:       ecdh,
		PQPK:       pqpk,
		Commitment: commitment,
		SeenPRs:    seenPRs,
	}
}
