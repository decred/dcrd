// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package addrmgr

import (
	"encoding/base32"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/decred/dcrd/wire"
)

// NetAddress defines information about a peer on the network.
type NetAddress struct {
	// IP address of the peer. It is defined as a byte array to support various
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
	return IsRoutable(netAddr.IP)
}

// ipString returns a string representation of the network address' IP field.
// If the ip is in the range used for TORv2 addresses then it will be
// transformed into the respective .onion address.  It does not include the
// port.
func (netAddr *NetAddress) ipString() string {
	netIP := netAddr.IP
	if isOnionCatTor(netIP) {
		// We know now that na.IP is long enough.
		base32 := base32.StdEncoding.EncodeToString(netIP[6:])
		return strings.ToLower(base32) + ".onion"
	}
	return net.IP(netIP).String()
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

// Clone creates a shallow copy of the NetAddress instance. The IP reference
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

// newAddressFromString creates a new address manager network address from the
// provided string.  The address is expected to be provided in the format
// host:port.
func (a *AddrManager) newAddressFromString(addr string) (*NetAddress, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}

	return a.HostToNetAddress(host, uint16(port), wire.SFNodeNetwork)
}

// NewNetAddressIPPort creates a new address manager network address given an ip,
// port, and the supported service flags for the address.
func NewNetAddressIPPort(ip net.IP, port uint16, services wire.ServiceFlag) *NetAddress {
	timestamp := time.Unix(time.Now().Unix(), 0)
	return &NetAddress{
		IP:        ip,
		Port:      port,
		Services:  services,
		Timestamp: timestamp,
	}
}
