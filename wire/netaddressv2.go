// Copyright (c) 2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"net"
	"time"
)

// NetAddressType is used to indicate the type of a given network address.
type NetAddressType uint8

const (
	UnknownAddressType NetAddressType = 0
	IPv4Address        NetAddressType = 1
	IPv6Address        NetAddressType = 2
	TORv3Address       NetAddressType = 3
)

// NetAddressV2 defines information about a peer on the network.
//
// The field order matches the wire protocol encoding order.
type NetAddressV2 struct {
	// Timestamp is the last time the address was seen.
	Timestamp time.Time

	// Services represents the service flags supported by this network address.
	Services ServiceFlag

	// Type represents the type of network that the network address belongs to.
	Type NetAddressType

	// IP address of the peer. It is defined as a byte array to support various
	// address types that are not standard to the net package and therefore not
	// entirely appropriate to store as a net.IP.
	IP []byte

	// Port is the port of the remote peer.
	Port uint16
}

// NewNetAddressV2 creates a new network address using the provided
// parameters without validation.
func NewNetAddressV2(netAddressType NetAddressType, addrBytes []byte, port uint16, timestamp time.Time, services ServiceFlag) NetAddressV2 {
	return NetAddressV2{
		Timestamp: timestamp,
		Services:  services,
		Type:      netAddressType,
		IP:        addrBytes,
		Port:      port,
	}
}

// NewNetAddressV2IPPort returns a new NetAddressV2 using the provided IP,
// port, and supported services with a current timestamp. The address type is
// automatically determined from the IP (IPv4 or IPv6).
func NewNetAddressV2IPPort(ip net.IP, port uint16, services ServiceFlag) NetAddressV2 {
	var addrType NetAddressType
	var addrBytes []byte

	if ip4 := ip.To4(); ip4 != nil {
		addrType = IPv4Address
		addrBytes = ip4
	} else {
		addrType = IPv6Address
		addrBytes = ip.To16()
	}

	// Limit the timestamp to one second precision since the protocol
	// doesn't support better.
	timestamp := time.Unix(time.Now().Unix(), 0)
	return NetAddressV2{
		Timestamp: timestamp,
		Services:  services,
		Type:      addrType,
		IP:        addrBytes,
		Port:      port,
	}
}
