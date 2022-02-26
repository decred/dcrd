// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package addrmgr

import (
	"net"
	"reflect"
	"testing"

	"time"

	"github.com/decred/dcrd/wire"
)

// TestNewNetAddressByType verifies that the TestNewNetAddressByType constructor
// converts a network address with expected field values.
func TestNewNetAddressByType(t *testing.T) {
	const port = 8345
	const services = wire.SFNodeNetwork
	timestamp := time.Unix(time.Now().Unix(), 0)

	tests := []struct {
		name      string
		addrType  NetAddressType
		addrBytes []byte
		want      *NetAddress
	}{
		{
			name:      "4 byte ipv4 address stored as 4 byte ip",
			addrType:  IPv4Address,
			addrBytes: net.ParseIP("127.0.0.1").To4(),
			want: &NetAddress{
				IP:        []byte{0x7f, 0x00, 0x00, 0x01},
				Port:      port,
				Services:  services,
				Timestamp: timestamp,
				Type:      IPv4Address,
			},
		},
		{
			name:      "16 byte ipv4 address stored as 4 byte ip",
			addrType:  IPv4Address,
			addrBytes: net.ParseIP("127.0.0.1").To16(),
			want: &NetAddress{
				IP:        []byte{0x7f, 0x00, 0x00, 0x01},
				Port:      port,
				Services:  services,
				Timestamp: timestamp,
				Type:      IPv4Address,
			},
		},
		{
			name:      "16 byte ipv6 address stored in 16 bytes",
			addrType:  IPv6Address,
			addrBytes: net.ParseIP("::1"),
			want: &NetAddress{
				IP: []byte{
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
				Port:      port,
				Services:  services,
				Timestamp: timestamp,
				Type:      IPv6Address,
			},
		},
		{
			name:      "16 byte torv2 address stored in 16 bytes",
			addrType:  TORv2Address,
			addrBytes: net.ParseIP("fd87:d87e:eb43::"),
			want: &NetAddress{
				IP: []byte{
					0xfd, 0x87, 0xd8, 0x7e, 0xeb, 0x43, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				},
				Port:      port,
				Services:  services,
				Timestamp: timestamp,
				Type:      TORv2Address,
			},
		},
		{
			name:     "10 byte torv2 public key stored in 16 bytes with prefix",
			addrType: TORv2Address,
			addrBytes: []byte{
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A,
			},
			want: &NetAddress{
				IP: []byte{
					0xfd, 0x87, 0xd8, 0x7e, 0xeb, 0x43, 0x01, 0x02,
					0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A,
				},
				Port:      port,
				Services:  services,
				Timestamp: timestamp,
				Type:      TORv2Address,
			},
		},
	}

	for _, test := range tests {
		addr, err := NewNetAddressByType(test.addrType, test.addrBytes, port,
			timestamp, services)
		if err != nil {
			t.Fatalf("%q: unexpected error - %v", test.name, err)
		}
		if !reflect.DeepEqual(addr, test.want) {
			t.Errorf("%q: mismatched entries\ngot  %+v\nwant %+v", test.name,
				addr, test.want)
		}
	}
}

// TestKey verifies that Key converts a network address to an expected string
// value.
func TestKey(t *testing.T) {
	tests := []struct {
		host string
		port uint16
		want string
	}{
		// IPv4
		// Localhost
		{host: "127.0.0.1", port: 8333, want: "127.0.0.1:8333"},
		{host: "127.0.0.1", port: 8334, want: "127.0.0.1:8334"},

		// Class A
		{host: "1.0.0.1", port: 8333, want: "1.0.0.1:8333"},
		{host: "2.2.2.2", port: 8334, want: "2.2.2.2:8334"},
		{host: "27.253.252.251", port: 8335, want: "27.253.252.251:8335"},
		{host: "123.3.2.1", port: 8336, want: "123.3.2.1:8336"},

		// Private Class A
		{host: "10.0.0.1", port: 8333, want: "10.0.0.1:8333"},
		{host: "10.1.1.1", port: 8334, want: "10.1.1.1:8334"},
		{host: "10.2.2.2", port: 8335, want: "10.2.2.2:8335"},
		{host: "10.10.10.10", port: 8336, want: "10.10.10.10:8336"},

		// Class B
		{host: "128.0.0.1", port: 8333, want: "128.0.0.1:8333"},
		{host: "129.1.1.1", port: 8334, want: "129.1.1.1:8334"},
		{host: "180.2.2.2", port: 8335, want: "180.2.2.2:8335"},
		{host: "191.10.10.10", port: 8336, want: "191.10.10.10:8336"},

		// Private Class B
		{host: "172.16.0.1", port: 8333, want: "172.16.0.1:8333"},
		{host: "172.16.1.1", port: 8334, want: "172.16.1.1:8334"},
		{host: "172.16.2.2", port: 8335, want: "172.16.2.2:8335"},
		{host: "172.16.172.172", port: 8336, want: "172.16.172.172:8336"},

		// Class C
		{host: "193.0.0.1", port: 8333, want: "193.0.0.1:8333"},
		{host: "200.1.1.1", port: 8334, want: "200.1.1.1:8334"},
		{host: "205.2.2.2", port: 8335, want: "205.2.2.2:8335"},
		{host: "223.10.10.10", port: 8336, want: "223.10.10.10:8336"},

		// Private Class C
		{host: "192.168.0.1", port: 8333, want: "192.168.0.1:8333"},
		{host: "192.168.1.1", port: 8334, want: "192.168.1.1:8334"},
		{host: "192.168.2.2", port: 8335, want: "192.168.2.2:8335"},
		{host: "192.168.192.192", port: 8336, want: "192.168.192.192:8336"},

		// IPv6
		// Localhost
		{host: "::1", port: 8333, want: "[::1]:8333"},
		{host: "fe80::1", port: 8334, want: "[fe80::1]:8334"},

		// Link-local
		{host: "fe80::1:1", port: 8333, want: "[fe80::1:1]:8333"},
		{host: "fe91::2:2", port: 8334, want: "[fe91::2:2]:8334"},
		{host: "fea2::3:3", port: 8335, want: "[fea2::3:3]:8335"},
		{host: "feb3::4:4", port: 8336, want: "[feb3::4:4]:8336"},

		// Site-local
		{host: "fec0::1:1", port: 8333, want: "[fec0::1:1]:8333"},
		{host: "fed1::2:2", port: 8334, want: "[fed1::2:2]:8334"},
		{host: "fee2::3:3", port: 8335, want: "[fee2::3:3]:8335"},
		{host: "fef3::4:4", port: 8336, want: "[fef3::4:4]:8336"},

		// TORv2
		{
			host: "fd87:d87e:eb43::",
			port: 8333,
			want: "aaaaaaaaaaaaaaaa.onion:8333",
		},
		{
			host: "aaaaaaaaaaaaaaaa.onion",
			port: 8334,
			want: "aaaaaaaaaaaaaaaa.onion:8334",
		},

		// TORv3
		{
			host: "xa4r2iadxm55fbnqgwwi5mymqdcofiu3w6rpbtqn7b2dyn7mgwj64jyd.onion",
			port: 8333,
			want: "xa4r2iadxm55fbnqgwwi5mymqdcofiu3w6rpbtqn7b2dyn7mgwj64jyd.onion:8333",
		},
	}

	timeNow := time.Now()
	for _, test := range tests {
		host := test.host
		addrType, addrBytes, err := ParseHost(host)
		if err != nil {
			t.Fatalf("failed to decode host %s: %v", host, err)
		}

		netAddr, err := NewNetAddressByType(addrType, addrBytes, test.port,
			timeNow, wire.SFNodeNetwork)
		if err != nil {
			t.Fatalf("failed to construct network address from host %q: %v",
				host, err)
		}

		key := netAddr.Key()
		if key != test.want {
			t.Errorf("unexpected network address key -- got %s, want %s",
				key, test.want)
			continue
		}
	}
}

// TestClone verifies that a new instance of the network address struct is
// created when cloned.
func TestClone(t *testing.T) {
	const port = 0
	netAddr := NewNetAddressIPPort(net.ParseIP("1.2.3.4"), port, wire.SFNodeNetwork)
	netAddrClone := netAddr.Clone()

	if netAddr == netAddrClone {
		t.Fatal("expected new network address reference")
	}
	if !reflect.DeepEqual(netAddr, netAddrClone) {
		t.Fatalf("unxpected clone result -- got %v, want %v",
			netAddrClone, netAddr)
	}
}

// TestAddService verifies that the service flag is set as expected on a
// network address instance.
func TestAddService(t *testing.T) {
	const port = 0
	netAddr := NewNetAddressIPPort(net.ParseIP("1.2.3.4"), port, wire.SFNodeNetwork)
	netAddr.AddService(wire.SFNodeNetwork)

	if netAddr.Services != wire.SFNodeNetwork {
		t.Fatalf("expected service flag to be set -- got %x, want %x",
			netAddr.Services, wire.SFNodeNetwork)
	}
}
