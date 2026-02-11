// Copyright (c) 2021-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package addrmgr

import (
	"encoding/base32"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/decred/dcrd/wire"
)

// NetAddress defines information about a peer on the network.
type NetAddress struct {
	// Type represents the type of an address (IPv4, IPv6, Tor, etc.).
	Type NetAddressType

	// IP address of the peer.  It is defined as a byte array to support various
	// address types that are not standard to the net module and therefore not
	// entirely appropriate to store as a net.IP.
	IP []byte

	// Port is the port of the remote peer.
	Port uint16

	// Timestamp is the last time the address was seen.
	Timestamp time.Time

	// Services represents the service flags supported by this network address.
	Services wire.ServiceFlag
}

// IsRoutable returns a boolean indicating whether the network address is
// routable.
func (netAddr *NetAddress) IsRoutable() bool {
	if netAddr.Type == TorV3Address {
		return true
	}
	return IsRoutable(netAddr.IP)
}

// ipString returns a string representation of the network address' IP field.
// It does not include the port.
func (netAddr *NetAddress) ipString() string {
	netIP := netAddr.IP
	switch netAddr.Type {
	case IPv6Address:
		return net.IP(netIP).String()
	case IPv4Address:
		return net.IP(netIP).String()
	case TorV3Address:
		var publicKey [32]byte
		copy(publicKey[:], netIP)
		checksum := calcTorV3Checksum(publicKey)
		var torAddressBytes [35]byte
		copy(torAddressBytes[:32], publicKey[:])
		copy(torAddressBytes[32:34], checksum[:])
		torAddressBytes[34] = torV3VersionByte
		return strings.ToLower(base32.StdEncoding.EncodeToString(torAddressBytes[:])) + ".onion"
	}

	// If the netAddr.Type is not recognized in the switch:
	return fmt.Sprintf("unsupported IP type %d, %s, %[2]x", netAddr.Type, netIP)
}

// Key returns a string that can be used to uniquely represent the network
// address and includes the port.
func (netAddr *NetAddress) Key() string {
	portString := strconv.FormatUint(uint64(netAddr.Port), 10)
	return net.JoinHostPort(netAddr.ipString(), portString)
}

// String returns a human-readable string for the network address.  This is
// equivalent to calling Key, but is provided so the type can be used as a
// fmt.Stringer.
func (netAddr *NetAddress) String() string {
	return netAddr.Key()
}

// Clone creates a shallow copy of the NetAddress instance.  The IP reference
// is shared since it is not mutated.
func (netAddr *NetAddress) Clone() *NetAddress {
	netAddrCopy := *netAddr
	return &netAddrCopy
}

// AddService adds the provided service to the set of services that the
// network address supports.
func (netAddr *NetAddress) AddService(service wire.ServiceFlag) {
	netAddr.Services |= service
}

// deriveNetAddressType attempts to determine the network address type from the
// address' raw bytes.  If the type cannot be determined, an error is returned.
// The claimedType parameter provides a hint for ambiguous byte lengths.
func deriveNetAddressType(claimedType NetAddressType, addrBytes []byte) (NetAddressType, error) {
	len := len(addrBytes)
	switch {
	case isIPv4(addrBytes):
		return IPv4Address, nil
	case len == 16:
		return IPv6Address, nil
	case len == 32 && claimedType == TorV3Address:
		return TorV3Address, nil
	}
	str := fmt.Sprintf("unable to determine address type from raw network "+
		"address bytes: %v", addrBytes)
	return UnknownAddressType, makeError(ErrUnknownAddressType, str)
}

// canonicalizeIP converts the provided address' bytes into a standard structure
// based on the type of the network address, if applicable.
func canonicalizeIP(addrType NetAddressType, addrBytes []byte) []byte {
	if addrBytes == nil {
		return nil
	}
	switch {
	case len(addrBytes) == 16 && addrType == IPv4Address:
		return net.IP(addrBytes).To4()
	case addrType == IPv6Address:
		return net.IP(addrBytes).To16()
	}
	// Given a Tor address (or other), the bytes are returned unchanged.
	return addrBytes
}

// checkNetAddressType returns an error if the suggested address type does not
// appear to match the provided address.
func checkNetAddressType(addrType NetAddressType, addrBytes []byte) error {
	derivedAddressType, err := deriveNetAddressType(addrType, addrBytes)
	if err != nil {
		return err
	}
	if addrType != derivedAddressType {
		str := fmt.Sprintf("derived address type does not match expected value"+
			" (got %v, expected %v, address bytes %v).", derivedAddressType,
			addrType, addrBytes)
		return makeError(ErrMismatchedAddressType, str)
	}
	return nil
}

// NewNetAddressFromParams creates a new network address from the given
// parameters. If the provided address type does not appear to match the
// address, an error is returned.
func NewNetAddressFromParams(addrType NetAddressType, addrBytes []byte, port uint16, timestamp time.Time, services wire.ServiceFlag) (*NetAddress, error) {
	canonicalizedIP := canonicalizeIP(addrType, addrBytes)
	err := checkNetAddressType(addrType, canonicalizedIP)
	if err != nil {
		return nil, err
	}
	return &NetAddress{
		Type:      addrType,
		IP:        canonicalizedIP,
		Port:      port,
		Services:  services,
		Timestamp: timestamp,
	}, nil
}

// newNetAddressFromString creates a new network address from the given string.
// The address string is expected to be provided in the format "host:port".
func (a *AddrManager) newNetAddressFromString(addr string) (*NetAddress, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}
	addrType, addrBytes := EncodeHost(host)
	if addrType == UnknownAddressType {
		str := fmt.Sprintf("failed to deserialize address %s", addr)
		return nil, makeError(ErrUnknownAddressType, str)
	}
	timestamp := time.Unix(time.Now().Unix(), 0)
	return NewNetAddressFromParams(addrType, addrBytes, uint16(port), timestamp,
		wire.SFNodeNetwork)
}

// NewNetAddressFromIPPort creates a new network address given an ip, port, and
// the supported service flags for the address.  The provided ip MUST be a valid
// IPv4 or IPv6 address, since this method does not perform error checking on
// the derived network address type.  Furthermore, other types of network
// addresses (like Tor) will not be recognized.
func NewNetAddressFromIPPort(ip net.IP, port uint16, services wire.ServiceFlag) *NetAddress {
	netAddressType, _ := deriveNetAddressType(UnknownAddressType, ip)
	timestamp := time.Unix(time.Now().Unix(), 0)
	canonicalizedIP := canonicalizeIP(netAddressType, ip)
	return &NetAddress{
		Type:      netAddressType,
		IP:        canonicalizedIP,
		Port:      port,
		Services:  services,
		Timestamp: timestamp,
	}
}
