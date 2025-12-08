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
	// MaxMixMcount is the maximum number of mixed messages that are allowed
	// in a single mix.  This restricts the total allowed size of the slot
	// reservation and XOR DC-net matrices.
	// This value is an high estimate of what a large mix may look like,
	// based on statistics from the centralized mixing server.
	MaxMixMcount = 1024

	// MaxMixFieldValLen is the maximum number of bytes allowed to represent
	// a value in the slot reservation mix bounded by the field prime.
	MaxMixFieldValLen = 32
)

// MsgMixSlotReserve is the slot reservation broadcast.  It implements the Message
// interface.
type MsgMixSlotReserve struct {
	Signature       [64]byte
	Identity        [33]byte
	SessionID       [32]byte
	Run             uint32
	DCMix           [][][]byte // mcount-by-peers matrix of field numbers
	SeenCiphertexts []chainhash.Hash

	// hash records the hash of the message.  It is a member of the
	// message for convenience and performance, but is never automatically
	// set during creation or deserialization.
	hash chainhash.Hash
}

// BtcDecode decodes r using the Decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgMixSlotReserve) BtcDecode(r io.Reader, pver uint32) error {
	const op = "MsgMixSlotReserve.BtcDecode"
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

	// Read the DCMix
	mcount, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if mcount == 0 {
		msg := fmt.Sprintf("too few mixed messages [%v]", mcount)
		return messageError(op, ErrInvalidMsg, msg)
	}
	if mcount > MaxMixMcount {
		msg := fmt.Sprintf("too many total mixed messages [%v]", mcount)
		return messageError(op, ErrInvalidMsg, msg)
	}
	kpcount, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if kpcount == 0 {
		msg := fmt.Sprintf("too few mixing peers [%v]", kpcount)
		return messageError(op, ErrInvalidMsg, msg)
	}
	if kpcount > MaxMixPeers {
		msg := fmt.Sprintf("too many mixing peers [count %v, max %v]",
			kpcount, MaxMixPeers)
		return messageError(op, ErrInvalidMsg, msg)
	}
	dcmix := make([][][]byte, mcount)
	for i := range dcmix {
		dcmix[i] = make([][]byte, kpcount)
		for j := range dcmix[i] {
			v, err := ReadVarBytes(r, pver, MaxMixFieldValLen,
				"slot reservation field value")
			if err != nil {
				return err
			}
			dcmix[i][j] = v
		}
	}
	msg.DCMix = dcmix

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
	msg.SeenCiphertexts = seen

	return nil
}

// BtcEncode encodes the receiver to w using the Decred protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgMixSlotReserve) BtcEncode(w io.Writer, pver uint32) error {
	const op = "MsgMixSlotReserve.BtcEncode"
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
func (msg *MsgMixSlotReserve) Hash() chainhash.Hash {
	return msg.hash
}

// WriteHash serializes the message to a hasher and records the sum in the
// message's Hash field.
//
// The hasher's Size() must equal chainhash.HashSize, or this method will
// panic.  This method is designed to work only with hashers returned by
// blake256.New.
func (msg *MsgMixSlotReserve) WriteHash(h hash.Hash) {
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
func (msg *MsgMixSlotReserve) writeMessageNoSignature(op string, w io.Writer, pver uint32) error {
	_, hashing := w.(hash.Hash)

	err := writeElements(w, &msg.Identity, &msg.SessionID, &msg.Run)
	if err != nil {
		return err
	}

	// Write the DCMix
	mcount := len(msg.DCMix)
	if !hashing && mcount == 0 {
		msg := fmt.Sprintf("too few mixed messages [%v]", mcount)
		return messageError(op, ErrInvalidMsg, msg)
	}
	if !hashing && mcount > MaxMixMcount {
		msg := fmt.Sprintf("too many total mixed messages [%v]", mcount)
		return messageError(op, ErrInvalidMsg, msg)
	}
	kpcount := len(msg.DCMix[0])
	if !hashing && kpcount == 0 {
		msg := fmt.Sprintf("too few mixing peers [%v]", kpcount)
		return messageError(op, ErrInvalidMsg, msg)
	}
	if !hashing && kpcount > MaxMixPeers {
		msg := fmt.Sprintf("too many mixing peers [%v]", kpcount)
		return messageError(op, ErrInvalidMsg, msg)
	}
	err = WriteVarInt(w, pver, uint64(mcount))
	if err != nil {
		return err
	}
	err = WriteVarInt(w, pver, uint64(kpcount))
	if err != nil {
		return err
	}
	for i := range msg.DCMix {
		if !hashing && len(msg.DCMix[i]) != kpcount {
			msg := "invalid matrix dimensions"
			return messageError(op, ErrInvalidMsg, msg)
		}
		for j := range msg.DCMix[i] {
			v := msg.DCMix[i][j]
			if !hashing && len(v) > MaxMixFieldValLen {
				msg := "value exceeds bytes necessary to represent number in field"
				return messageError(op, ErrInvalidMsg, msg)
			}
			err := WriteVarBytes(w, pver, v)
			if err != nil {
				return err
			}
		}
	}

	count := len(msg.SeenCiphertexts)
	if !hashing && count > MaxMixPeers {
		msg := fmt.Sprintf("too many previous referenced messages [count %v, max %v]",
			count, MaxMixPeers)
		return messageError(op, ErrTooManyPrevMixMsgs, msg)
	}

	err = WriteVarInt(w, pver, uint64(count))
	if err != nil {
		return err
	}
	for i := range msg.SeenCiphertexts {
		err = writeElement(w, &msg.SeenCiphertexts[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// WriteSignedData writes a tag identifying the message data, followed by all
// message fields excluding the signature.  This is the data committed to when
// the message is signed.
func (msg *MsgMixSlotReserve) WriteSignedData(h hash.Hash) {
	WriteVarString(h, MixVersion, CmdMixSlotReserve+"-sig")
	msg.writeMessageNoSignature("", h, MixVersion)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgMixSlotReserve) Command() string {
	return CmdMixSlotReserve
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgMixSlotReserve) MaxPayloadLength(pver uint32) uint32 {
	if pver < MixVersion {
		return 0
	}

	// See tests for this calculation.
	return 17318030
}

// Pub returns the message sender's public key identity.
func (msg *MsgMixSlotReserve) Pub() []byte {
	return msg.Identity[:]
}

// Sig returns the message signature.
func (msg *MsgMixSlotReserve) Sig() []byte {
	return msg.Signature[:]
}

// PrevMsgs returns the previous CT messages seen by the peer.
func (msg *MsgMixSlotReserve) PrevMsgs() []chainhash.Hash {
	return msg.SeenCiphertexts
}

// Sid returns the session ID.
func (msg *MsgMixSlotReserve) Sid() []byte {
	return msg.SessionID[:]
}

// GetRun returns the run number.
func (msg *MsgMixSlotReserve) GetRun() uint32 {
	return msg.Run
}

// NewMsgMixSlotReserve returns a new mixslotres message that conforms to the
// Message interface using the passed parameters and defaults for the
// remaining fields.
func NewMsgMixSlotReserve(identity [33]byte, sid [32]byte, run uint32,
	dcmix [][][]byte, seenCTs []chainhash.Hash) *MsgMixSlotReserve {

	return &MsgMixSlotReserve{
		Identity:        identity,
		SessionID:       sid,
		Run:             run,
		DCMix:           dcmix,
		SeenCiphertexts: seenCTs,
	}
}
