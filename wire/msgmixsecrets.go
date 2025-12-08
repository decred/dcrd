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

// MsgMixSecrets reveals secrets of a failed mix run.  After secrets are
// exposed, peers can determine which peers (if any) misbehaved and remove
// them from the next run in the session.
//
// It implements the Message interface.
type MsgMixSecrets struct {
	Signature       [64]byte
	Identity        [33]byte
	SessionID       [32]byte
	Run             uint32
	Seed            [32]byte
	SlotReserveMsgs [][]byte
	DCNetMsgs       MixVect
	SeenSecrets     []chainhash.Hash

	// hash records the hash of the message.  It is a member of the
	// message for convenience and performance, but is never automatically
	// set during creation or deserialization.
	hash chainhash.Hash
}

func writeMixVect(op string, w io.Writer, pver uint32, vec MixVect) error {
	if len(vec) > MaxMixMcount {
		msg := "DC-net mix vector dimensions are too large for maximum message count"
		return messageError(op, ErrInvalidMsg, msg)
	}

	// Write dimensions
	err := WriteVarInt(w, pver, uint64(len(vec)))
	if err != nil {
		return err
	}
	if len(vec) == 0 {
		return nil
	}
	err = WriteVarInt(w, pver, MixMsgSize)
	if err != nil {
		return err
	}

	// Write messages
	for i := range vec {
		_, err = writeElement(w, &vec[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func readMixVect(op string, r io.Reader, pver uint32, vec *MixVect) error {
	// Read dimensions
	n, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if n == 0 {
		*vec = MixVect{}
		return nil
	}
	msize, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	if n > MaxMixMcount {
		msg := "DC-net mix vector dimensions are too large for maximum message count"
		return messageError(op, ErrInvalidMsg, msg)
	}
	if msize != MixMsgSize {
		msg := fmt.Sprintf("mixed message length must be %d [got: %d]",
			MixMsgSize, msize)
		return messageError(op, ErrInvalidMsg, msg)
	}

	// Read messages
	*vec = make(MixVect, n)
	for i := uint64(0); i < n; i++ {
		err = readElement(r, &(*vec)[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// BtcDecode decodes r using the Decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgMixSecrets) BtcDecode(r io.Reader, pver uint32) error {
	const op = "MsgMixSecrets.BtcDecode"
	if pver < MixVersion {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	err := readElements(r, &msg.Signature, &msg.Identity, &msg.SessionID,
		&msg.Run, &msg.Seed)
	if err != nil {
		return err
	}

	numSRs, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if numSRs > MaxMixMcount {
		msg := fmt.Sprintf("too many total mixed messages [count %v, max %v]",
			numSRs, MaxMixMcount)
		return messageError(op, ErrInvalidMsg, msg)
	}
	msg.SlotReserveMsgs = make([][]byte, numSRs)
	for i := uint64(0); i < numSRs; i++ {
		sr, err := ReadVarBytes(r, pver, MaxMixFieldValLen,
			"slot reservation mixed message")
		if err != nil {
			return err
		}
		msg.SlotReserveMsgs[i] = sr
	}

	var dcnetMsgs MixVect
	err = readMixVect(op, r, pver, &dcnetMsgs)
	if err != nil {
		return err
	}
	msg.DCNetMsgs = dcnetMsgs

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
	msg.SeenSecrets = seen

	return nil
}

// BtcEncode encodes the receiver to w using the Decred protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgMixSecrets) BtcEncode(w io.Writer, pver uint32) error {
	const op = "MsgMixSecrets.BtcEncode"
	if pver < MixVersion {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	_, err := writeElement(w, &msg.Signature)
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
func (msg *MsgMixSecrets) Hash() chainhash.Hash {
	return msg.hash
}

// Commitment returns a hash committing to the contents of the reveal secrets
// message without committing to any previous seen messages or the message
// signature.  This is the hash that is referenced by peers' key exchange
// messages.
func (msg *MsgMixSecrets) Commitment(h hash.Hash) chainhash.Hash {
	msgCopy := *msg
	msgCopy.Signature = [64]byte{}
	msgCopy.SeenSecrets = nil
	msgCopy.WriteHash(h)
	return msgCopy.hash
}

// WriteHash serializes the message to a hasher and records the sum in the
// message's Hash field.
//
// The hasher's Size() must equal chainhash.HashSize, or this method will
// panic.  This method is designed to work only with hashers returned by
// blake256.New.
func (msg *MsgMixSecrets) WriteHash(h hash.Hash) {
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
// This method never errors for invalid message construction.
func (msg *MsgMixSecrets) writeMessageNoSignature(op string, w io.Writer, pver uint32) error {
	// Limit to max previous messages hashes.
	count := len(msg.SeenSecrets)
	if count > MaxMixPeers {
		msg := fmt.Sprintf("too many previous referenced messages [count %v, max %v]",
			count, MaxMixPeers)
		return messageError(op, ErrTooManyPrevMixMsgs, msg)
	}

	_, err := writeElements(w, &msg.Identity, &msg.SessionID, &msg.Run, &msg.Seed)
	if err != nil {
		return err
	}

	err = WriteVarInt(w, pver, uint64(len(msg.SlotReserveMsgs)))
	if err != nil {
		return err
	}
	for _, sr := range msg.SlotReserveMsgs {
		err := WriteVarBytes(w, pver, sr)
		if err != nil {
			return err
		}
	}

	err = writeMixVect(op, w, pver, msg.DCNetMsgs)
	if err != nil {
		return err
	}

	err = WriteVarInt(w, pver, uint64(count))
	if err != nil {
		return err
	}
	for i := range msg.SeenSecrets {
		_, err := writeElement(w, &msg.SeenSecrets[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// WriteSignedData writes a tag identifying the message data, followed by all
// message fields excluding the signature.  This is the data committed to when
// the message is signed.
func (msg *MsgMixSecrets) WriteSignedData(h hash.Hash) {
	WriteVarString(h, MixVersion, CmdMixSecrets+"-sig")
	msg.writeMessageNoSignature("", h, MixVersion)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgMixSecrets) Command() string {
	return CmdMixSecrets
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgMixSecrets) MaxPayloadLength(pver uint32) uint32 {
	if pver < MixVersion {
		return 0
	}

	// See tests for this calculation
	return 70831
}

// Pub returns the message sender's public key identity.
func (msg *MsgMixSecrets) Pub() []byte {
	return msg.Identity[:]
}

// Sig returns the message signature.
func (msg *MsgMixSecrets) Sig() []byte {
	return msg.Signature[:]
}

// PrevMsgs returns previously revealed secrets messages by other peers.  An
// honest peer who needs to report blame assignment does not need to reference
// any previous secrets messages, and a secrets message with other referenced
// secrets is necessary to begin blame assignment.  Dishonest peers who
// initially reveal their secrets without blame assignment being necessary are
// themselves removed in future runs.
func (msg *MsgMixSecrets) PrevMsgs() []chainhash.Hash {
	return msg.SeenSecrets
}

// Sid returns the session ID.
func (msg *MsgMixSecrets) Sid() []byte {
	return msg.SessionID[:]
}

// GetRun returns the run number.
func (msg *MsgMixSecrets) GetRun() uint32 {
	return msg.Run
}

// NewMsgMixSecrets returns a new mixsecrets message that conforms to the
// Message interface using the passed parameters and defaults for the
// remaining fields.
func NewMsgMixSecrets(identity [33]byte, sid [32]byte, run uint32,
	seed [32]byte, slotReserveMsgs [][]byte, dcNetMsgs MixVect) *MsgMixSecrets {

	return &MsgMixSecrets{
		Identity:        identity,
		SessionID:       sid,
		Run:             run,
		Seed:            seed,
		SlotReserveMsgs: slotReserveMsgs,
		DCNetMsgs:       dcNetMsgs,
		SeenSecrets:     make([]chainhash.Hash, 0),
	}
}
