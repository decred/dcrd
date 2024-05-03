// Copyright (c) 2020 The decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"
)

const (
	// MaxInitStateTypeLen is the maximum length allowable for an
	// individual requested type.
	MaxInitStateTypeLen = 32

	// MaxInitStateTypes is the maximum number of individual types stored
	// in a getinitialstate message.
	MaxInitStateTypes = 32

	// InitStateHeadBlocks is the init state type used to request head
	// blocks for mining.
	InitStateHeadBlocks = "headblocks"

	// InitStateHeadBlockVotes is the init state type used to request votes
	// for the head blocks for mining.
	InitStateHeadBlockVotes = "headblockvotes"

	// InitStateTSpends is the init state type used to request tpends for
	// voting.
	InitStateTSpends = "tspends"
)

// MsgGetInitState implements the Message interface and represents a
// getinitstate message. It is used to request ephemeral, startup state from a
// peer.
//
// The specific desired information is specified in individual entries in the
// Types member.
type MsgGetInitState struct {
	Types []string
}

// AddType adds the specified type to the list of types stored in the message.
// If an attempt to store more than MaxInitStateTypes is made, this function
// returns an error.
//
// Only ascii strings up to MaxInitStateTypeLen may be stored.
func (msg *MsgGetInitState) AddType(typ string) error {
	const op = "MsgGetInitState.AddType"
	lenTyp := len(typ)
	if lenTyp > MaxInitStateTypeLen {
		msg := fmt.Sprintf("individual initial state type is "+
			"too long [len %d, max %d]", lenTyp,
			MaxInitStateTypeLen)
		return messageError(op, ErrInitStateTypeTooLong, msg)
	}

	if !isStrictAscii(typ) {
		msg := "individual initial state type is not strict ASCII"
		return messageError(op, ErrMalformedStrictString, msg)
	}

	nbTypes := uint64(len(msg.Types))
	if nbTypes >= uint64(MaxInitStateTypes) {
		msg := fmt.Sprintf("too many requested types for message "+
			"[count %d, max %d]", nbTypes, MaxInitStateTypes)
		return messageError(op, ErrTooManyInitStateTypes, msg)
	}

	msg.Types = append(msg.Types, typ)
	return nil
}

// AddTypes adds all the specified types or returns the first error. See
// AddType for limitations on individual type strings.
func (msg *MsgGetInitState) AddTypes(types ...string) error {
	for _, typ := range types {
		if err := msg.AddType(typ); err != nil {
			return err
		}
	}
	return nil
}

// BtcDecode decodes r using the Decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgGetInitState) BtcDecode(r io.Reader, pver uint32) error {
	const op = "MsgGetInitState.BtcDecode"
	if pver < InitStateVersion {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	nbTypes, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if nbTypes > MaxInitStateTypes {
		msg := fmt.Sprintf("too many types for message "+
			"[count %d, max %d]", nbTypes, MaxInitStateTypes)
		return messageError(op, ErrTooManyInitStateTypes, msg)
	}

	msg.Types = make([]string, nbTypes)
	for i := 0; i < int(nbTypes); i++ {
		msg.Types[i], err = ReadAsciiVarString(r, pver, MaxInitStateTypeLen)
		if err != nil {
			return err
		}
	}

	return nil
}

// BtcEncode encodes the receiver to w using the Decred protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetInitState) BtcEncode(w io.Writer, pver uint32) error {
	const op = "MsgGetInitState.BtcEncode"
	if pver < InitStateVersion {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	nbTypes := uint64(len(msg.Types))
	if nbTypes > uint64(MaxInitStateTypes) {
		msg := fmt.Sprintf("too many requested types for message "+
			"[count %d max %d]", nbTypes, MaxInitStateTypes)
		return messageError(op, ErrTooManyInitStateTypes, msg)
	}
	err := WriteVarInt(w, pver, nbTypes)
	if err != nil {
		return err
	}

	for _, typ := range msg.Types {
		lenTyp := len(typ)
		if lenTyp > MaxInitStateTypeLen {
			msg := fmt.Sprintf("individual initial state type is "+
				"too long [len %d, max %d]", lenTyp,
				MaxInitStateTypeLen)
			return messageError(op, ErrInitStateTypeTooLong, msg)
		}

		if !isStrictAscii(typ) {
			msg := "individual initial state type is not strict ASCII"
			return messageError(op, ErrMalformedStrictString, msg)
		}
		err := WriteVarString(w, pver, typ)
		if err != nil {
			return err
		}
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgGetInitState) Command() string {
	return CmdGetInitState
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgGetInitState) MaxPayloadLength(pver uint32) uint32 {
	if pver < InitStateVersion {
		return 0
	}

	// maxLenType is the max length of a single serialized type.
	maxLenType := VarIntSerializeSize(MaxInitStateTypeLen) +
		MaxInitStateTypeLen
	return uint32(VarIntSerializeSize(MaxInitStateTypes) +
		MaxInitStateTypes*maxLenType)
}

// NewMsgGetInitState returns a new Decred getinitialstate message that
// conforms to the Message interface.  See MsgGetInitState for details.
func NewMsgGetInitState() *MsgGetInitState {
	return &MsgGetInitState{
		Types: make([]string, 0, MaxInitStateTypes),
	}
}
