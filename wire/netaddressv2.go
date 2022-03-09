// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"time"
)

// NetAddressType is used to indicate which network a network address belongs
// to.
type NetAddressType uint8

const (
	UnknownAddressType NetAddressType = iota
	IPv4Address
	IPv6Address
	TORv3Address
)

// NetAddressV2 defines information about a peer on the network.
type NetAddressV2 struct {
	// Type represents the type of network that the network address belongs to.
	Type NetAddressType

	// IP address of the peer. It is defined as a byte array to support various
	// address types that are not standard to the net module and therefore not
	// entirely appropriate to store as a net.IP.
	IP []byte

	// Port is the port of the remote peer.
	Port uint16

	// Timestamp is the last time the address was seen.
	Timestamp time.Time

	// Services represents the service flags supported by this network address.
	Services ServiceFlag
}

// NewNetAddressV2 creates a new network address using the provided
// parameters without validation.
func NewNetAddressV2(netAddressType NetAddressType, addrBytes []byte, port uint16, timestamp time.Time, services ServiceFlag) *NetAddressV2 {
	return &NetAddressV2{
		Type:      netAddressType,
		IP:        addrBytes,
		Port:      port,
		Services:  services,
		Timestamp: timestamp,
	}
}
