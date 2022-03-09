// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"
	"net"
	"time"
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
	// received from a peer.  Instead of manually appending addresses to this
	// field directly, consumers should use the convenience functions on an
	// instance of this message to add addresses.
	AddrList []*NetAddressV2
}

// AddAddress adds a known address to the message.  If the maximum number of
// addresses has been reached, then an error is returned.
func (msg *MsgAddrV2) AddAddress(na *NetAddressV2) error {
	const op = "MsgAddrV2.AddAddress"
	if len(msg.AddrList)+1 > MaxAddrPerV2Msg {
		msg := fmt.Sprintf("too many addresses in message [max %v]",
			MaxAddrPerV2Msg)
		return messageError(op, ErrTooManyAddrs, msg)
	}

	msg.AddrList = append(msg.AddrList, na)
	return nil
}

// AddAddresses adds multiple known addresses to the message.  If the number of
// addresses exceeds the maximum allowed then an error is returned.
func (msg *MsgAddrV2) AddAddresses(netAddrs ...*NetAddressV2) error {
	for _, na := range netAddrs {
		err := msg.AddAddress(na)
		if err != nil {
			return err
		}
	}
	return nil
}

// ClearAddresses removes all addresses from the message.
func (msg *MsgAddrV2) ClearAddresses() {
	msg.AddrList = []*NetAddressV2{}
}

// readNetAddressV2 reads an encoded version 2 wire network address from the
// provided reader.
func readNetAddressV2(op string, r io.Reader, pver uint32) (*NetAddressV2, error) {
	type netAddress struct {
		Timestamp time.Time
		Services  ServiceFlag
		Port      uint16
		Type      NetAddressType
	}
	na := &netAddress{}

	err := readElement(r, (*int64Time)(&na.Timestamp))
	if err != nil {
		return nil, err
	}

	// Read the service flags.
	err = readElement(r, &na.Services)
	if err != nil {
		return nil, err
	}

	// Read the network id to determine the expected length of the ip field.
	err = readElement(r, &na.Type)
	if err != nil {
		return nil, err
	}

	// Read the ip bytes with a length varying by the network id type.
	var ipBytes []byte
	switch {
	case na.Type == IPv4Address:
		var ip [4]byte
		err := readElement(r, &ip)
		if err != nil {
			return nil, err
		}
		ipBytes = ip[:]
	case na.Type == IPv6Address:
		var ip [16]byte
		err := readElement(r, &ip)
		if err != nil {
			return nil, err
		}
		ipBytes = ip[:]
	case na.Type == TORv3Address && pver >= RelayTORv3Version:
		var ip [32]byte
		err := readElement(r, &ip)
		if err != nil {
			return nil, err
		}
		ipBytes = ip[:]
	default:
		msg := fmt.Sprintf("unsupported network address type %v for "+
			"protocol version %d", na.Type, pver)
		return nil, messageError(op, ErrInvalidMsg, msg)
	}

	err = readElement(r, &na.Port)
	if err != nil {
		return nil, err
	}

	return NewNetAddressV2(na.Type, ipBytes, na.Port, na.Timestamp,
		na.Services), nil
}

// writeNetAddressV2 serializes an address manager network address to the
// provided writer.
func writeNetAddressV2(op string, w io.Writer, pver uint32, na *NetAddressV2) error {
	err := writeElement(w, na.Timestamp.Unix())
	if err != nil {
		return err
	}

	err = writeElements(w, na.Services, na.Type)
	if err != nil {
		return err
	}

	netAddrIP := na.IP
	switch {
	case na.Type == IPv4Address:
		var ip [4]byte
		if netAddrIP != nil {
			copy(ip[:], netAddrIP)
		}
		err = writeElement(w, ip)
		if err != nil {
			return err
		}
	case na.Type == IPv6Address:
		var ip [16]byte
		if netAddrIP != nil {
			copy(ip[:], net.IP(netAddrIP).To16())
		}
		err = writeElement(w, ip)
		if err != nil {
			return err
		}
	case na.Type == TORv3Address && pver >= RelayTORv3Version:
		var ip [32]byte
		if len(na.IP) == 32 {
			copy(ip[:], net.IP(na.IP))
		}
		err = writeElement(w, ip)
		if err != nil {
			return err
		}
	default:
		msg := fmt.Sprintf("unsupported network address type %v for "+
			"protocol version %d", na.Type, pver)
		return messageError(op, ErrInvalidMsg, msg)
	}

	return writeElement(w, na.Port)
}

// maxNetAddressPayloadV2 returns the max payload size for an address manager
// network address based on the protocol version.
func maxNetAddressPayloadV2(pver uint32) uint32 {
	if pver < RelayTORv3Version {
		const (
			timestampSize   = 8
			servicesSize    = 8
			addressTypeSize = 1
			maxAddressSize  = 16
			portSize        = 2
		)
		return timestampSize + servicesSize + addressTypeSize + maxAddressSize +
			portSize
	}

	const (
		timestampSize   = 8
		servicesSize    = 8
		addressTypeSize = 1
		maxAddressSize  = 32
		portSize        = 2
	)
	return timestampSize + servicesSize + addressTypeSize + maxAddressSize +
		portSize
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

	msg.AddrList = make([]*NetAddressV2, 0, count)

	for i := uint64(0); i < count; i++ {
		netAddr, err := readNetAddressV2(op, r, pver)
		if err != nil {
			return err
		}
		msg.AddAddress(netAddr)
	}
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

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgAddrV2) MaxPayloadLength(pver uint32) uint32 {
	if pver < AddrV2Version {
		return 0
	}

	// Num addresses (size of varInt for max address per message) + max allowed
	// addresses * max address size.
	return uint32(VarIntSerializeSize(MaxAddrPerV2Msg)) +
		(MaxAddrPerV2Msg * maxNetAddressPayloadV2(pver))
}

// NewMsgAddrV2 returns a new wire addrv2 message that conforms to the
// Message interface.  See MsgAddr for details.
func NewMsgAddrV2() *MsgAddrV2 {
	return &MsgAddrV2{
		AddrList: make([]*NetAddressV2, 0, MaxAddrPerV2Msg),
	}
}
