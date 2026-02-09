// Copyright (c) 2025-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"
	"net"
)

// MaxAddrPerV2Msg is the maximum number of addresses that can be in a single
// Decred addrv2 protocol message.
const MaxAddrPerV2Msg = 1000

// MsgAddrV2 implements the Message interface and represents a wire
// addrv2 message.  It is used to provide a list of known active peers on the
// network.  An active peer is considered one that has transmitted a message
// within the last 3 hours.  Nodes which have not transmitted in that time
// frame should be forgotten.  Each message is limited to a maximum number of
// addresses.
type MsgAddrV2 struct {
	// AddrList contains the addresses that will be sent to or have been
	// received from a peer.  This MUST have a maximum of [MaxAddrPerV2Msg]
	// entries or the message will error during encode and decode.
	AddrList []NetAddressV2
}

// readNetAddressV2 reads an encoded version 2 wire network address from the
// provided reader into the provided NetAddressV2.
func readNetAddressV2(op string, r io.Reader, pver uint32, na *NetAddressV2) error {
	err := readElement(r, (*uint64Time)(&na.Timestamp))
	if err != nil {
		return err
	}

	// Read the service flags.
	err = readElement(r, &na.Services)
	if err != nil {
		return err
	}

	// Read the network id to determine the expected length of the ip field.
	err = readElement(r, &na.Type)
	if err != nil {
		return err
	}

	// Read the ip bytes with a length varying by the network id type.
	switch na.Type {
	case IPv4Address:
		var ip [4]byte
		err := readElement(r, &ip)
		if err != nil {
			return err
		}
		na.EncodedAddr = ip[:]

	case IPv6Address:
		var ip [16]byte
		err := readElement(r, &ip)
		if err != nil {
			return err
		}
		na.EncodedAddr = ip[:]

	case TORv3Address:
		if pver < TORv3Version {
			msg := fmt.Sprintf("TORv3 addresses require protocol version %d "+
				"or higher", TORv3Version)
			return messageError(op, ErrMsgInvalidForPVer, msg)
		}
		var addr [32]byte
		err := readElement(r, &addr)
		if err != nil {
			return err
		}
		na.EncodedAddr = addr[:]

	default:
		msg := fmt.Sprintf("cannot decode unknown network address type %v",
			na.Type)
		return messageError(op, ErrUnknownNetAddrType, msg)
	}

	err = readElement(r, &na.Port)
	if err != nil {
		return err
	}

	return nil
}

// writeNetAddressV2 serializes a version 2 network address to the provided
// writer.
func writeNetAddressV2(op string, w io.Writer, pver uint32, na NetAddressV2) error {
	err := writeElement(w, uint64(na.Timestamp.Unix()))
	if err != nil {
		return err
	}

	err = writeElements(w, na.Services, na.Type)
	if err != nil {
		return err
	}

	encodedAddr := na.EncodedAddr
	addrLen := len(encodedAddr)

	switch na.Type {
	case IPv4Address:
		if addrLen != 4 {
			msg := fmt.Sprintf("invalid IPv4 address length: %d", addrLen)
			return messageError(op, ErrInvalidMsg, msg)
		}
		var ip [4]byte
		copy(ip[:], encodedAddr)
		err = writeElement(w, ip)
		if err != nil {
			return err
		}

	case IPv6Address:
		if addrLen != 16 {
			msg := fmt.Sprintf("invalid IPv6 address length: %d", addrLen)
			return messageError(op, ErrInvalidMsg, msg)
		}
		var ip [16]byte
		copy(ip[:], net.IP(encodedAddr).To16())
		err = writeElement(w, ip)
		if err != nil {
			return err
		}

	case TORv3Address:
		if pver < TORv3Version {
			msg := fmt.Sprintf("TORv3 addresses require protocol version %d "+
				"or higher", TORv3Version)
			return messageError(op, ErrMsgInvalidForPVer, msg)
		}
		if len(encodedAddr) != 32 {
			msg := fmt.Sprintf("invalid TORv3 address length: %d", len(encodedAddr))
			return messageError(op, ErrInvalidMsg, msg)
		}
		var addr [32]byte
		copy(addr[:], encodedAddr)
		err = writeElement(w, addr)
		if err != nil {
			return err
		}

	default:
		msg := fmt.Sprintf("cannot encode unknown network address type %v",
			na.Type)
		return messageError(op, ErrUnknownNetAddrType, msg)
	}

	return writeElement(w, na.Port)
}

// BtcDecode decodes r using the wire protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgAddrV2) BtcDecode(r io.Reader, pver uint32) error {
	const op = "MsgAddrV2.BtcDecode"

	// Ensure peers sending msgaddrv2 are on the expected minimum version.
	if pver < AddrV2Version {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	// Read the total number of addresses in this message.
	count, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	if count == 0 {
		return messageError(op, ErrTooFewAddrs,
			"no addresses for message [count 0, min 1]")
	}

	// Limit to max addresses per message.
	if count > MaxAddrPerV2Msg {
		msg := fmt.Sprintf("too many addresses for message [count %v, max %v]",
			count, MaxAddrPerV2Msg)
		return messageError(op, ErrTooManyAddrs, msg)
	}

	addrs := make([]NetAddressV2, count)
	for i := uint64(0); i < count; i++ {
		err := readNetAddressV2(op, r, pver, &addrs[i])
		if err != nil {
			return err
		}
	}

	msg.AddrList = addrs
	return nil
}

// BtcEncode encodes the receiver to w using the wire protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgAddrV2) BtcEncode(w io.Writer, pver uint32) error {
	const op = "MsgAddrV2.BtcEncode"
	if pver < AddrV2Version {
		msg := fmt.Sprintf("%s message invalid for protocol version %d",
			msg.Command(), pver)
		return messageError(op, ErrMsgInvalidForPVer, msg)
	}

	count := len(msg.AddrList)
	if count > MaxAddrPerV2Msg {
		msg := fmt.Sprintf("too many addresses for message [count %v, max %v]",
			count, MaxAddrPerV2Msg)
		return messageError(op, ErrTooManyAddrs, msg)
	}

	if count == 0 {
		return messageError(op, ErrTooFewAddrs,
			"no addresses for message [count 0, min 1]")
	}

	err := WriteVarInt(w, pver, uint64(count))
	if err != nil {
		return err
	}

	for _, na := range msg.AddrList {
		err = writeNetAddressV2(op, w, pver, na)
		if err != nil {
			return err
		}
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgAddrV2) Command() string {
	return CmdAddrV2
}

// maxNetAddressPayloadV2 returns the max payload size for an address manager
// network address based on the protocol version.
func maxNetAddressPayloadV2(pver uint32) uint32 {
	const (
		timestampSize   = 8
		servicesSize    = 8
		addressTypeSize = 1
		portSize        = 2
	)

	maxAddressSize := uint32(16) // IPv6 is 16 bytes
	if pver >= TORv3Version {
		maxAddressSize = 32
	}

	return timestampSize + servicesSize + addressTypeSize +
		maxAddressSize + portSize
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgAddrV2) MaxPayloadLength(pver uint32) uint32 {
	if pver < AddrV2Version {
		return 0
	}
	return uint32(VarIntSerializeSize(MaxAddrPerV2Msg)) +
		(MaxAddrPerV2Msg * maxNetAddressPayloadV2(pver))
}

// NewMsgAddrV2 returns a new wire addrv2 message that conforms to the
// Message interface.  See MsgAddrV2 for details.
//
// The provided slice is expected to have a minimum of one address and a maximum
// of [MaxAddrPerV2Msg].  The message will fail to decode and encode if it does
// not satisfy those requirements at the time of decoding and encoding.
func NewMsgAddrV2(addrs []NetAddressV2) *MsgAddrV2 {
	return &MsgAddrV2{addrs}
}
