// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package addrmgr

import (
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/wire"
)

// TestIpString verifies that IpString will correctly return the string
// representation of a NetAddress' IP field.
func TestIpString(t *testing.T) {
	// This test only has one test case: for when the NetAddress cannot be
	// represented as a string.
	unsupportedAddr := NetAddress{
		Type:      UnknownAddressType,
		IP:        []byte{0x00},
		Port:      uint16(0),
		Timestamp: time.Now(),
		Services:  wire.SFNodeNetwork,
	}

	addrStr := unsupportedAddr.ipString()
	wantStr := fmt.Sprintf("unsupported IP type %d, %s, %x",
		UnknownAddressType, []byte{0x00}, []byte{0x00})
	if addrStr != wantStr {
		t.Fatalf("problem: %q", addrStr)
	}
}

// TestKey verifies that Key converts a network address to an expected string
// value.
func TestKey(t *testing.T) {
	tests := []struct {
		ip   string
		port uint16
		want string
	}{
		// IPv4
		// Localhost
		{ip: "127.0.0.1", port: 8333, want: "127.0.0.1:8333"},
		{ip: "127.0.0.1", port: 8334, want: "127.0.0.1:8334"},

		// Class A
		{ip: "1.0.0.1", port: 8333, want: "1.0.0.1:8333"},
		{ip: "2.2.2.2", port: 8334, want: "2.2.2.2:8334"},
		{ip: "27.253.252.251", port: 8335, want: "27.253.252.251:8335"},
		{ip: "123.3.2.1", port: 8336, want: "123.3.2.1:8336"},

		// Private Class A
		{ip: "10.0.0.1", port: 8333, want: "10.0.0.1:8333"},
		{ip: "10.1.1.1", port: 8334, want: "10.1.1.1:8334"},
		{ip: "10.2.2.2", port: 8335, want: "10.2.2.2:8335"},
		{ip: "10.10.10.10", port: 8336, want: "10.10.10.10:8336"},

		// Class B
		{ip: "128.0.0.1", port: 8333, want: "128.0.0.1:8333"},
		{ip: "129.1.1.1", port: 8334, want: "129.1.1.1:8334"},
		{ip: "180.2.2.2", port: 8335, want: "180.2.2.2:8335"},
		{ip: "191.10.10.10", port: 8336, want: "191.10.10.10:8336"},

		// Private Class B
		{ip: "172.16.0.1", port: 8333, want: "172.16.0.1:8333"},
		{ip: "172.16.1.1", port: 8334, want: "172.16.1.1:8334"},
		{ip: "172.16.2.2", port: 8335, want: "172.16.2.2:8335"},
		{ip: "172.16.172.172", port: 8336, want: "172.16.172.172:8336"},

		// Class C
		{ip: "193.0.0.1", port: 8333, want: "193.0.0.1:8333"},
		{ip: "200.1.1.1", port: 8334, want: "200.1.1.1:8334"},
		{ip: "205.2.2.2", port: 8335, want: "205.2.2.2:8335"},
		{ip: "223.10.10.10", port: 8336, want: "223.10.10.10:8336"},

		// Private Class C
		{ip: "192.168.0.1", port: 8333, want: "192.168.0.1:8333"},
		{ip: "192.168.1.1", port: 8334, want: "192.168.1.1:8334"},
		{ip: "192.168.2.2", port: 8335, want: "192.168.2.2:8335"},
		{ip: "192.168.192.192", port: 8336, want: "192.168.192.192:8336"},

		// IPv6
		// Localhost
		{ip: "::1", port: 8333, want: "[::1]:8333"},
		{ip: "fe80::1", port: 8334, want: "[fe80::1]:8334"},

		// Link-local
		{ip: "fe80::1:1", port: 8333, want: "[fe80::1:1]:8333"},
		{ip: "fe91::2:2", port: 8334, want: "[fe91::2:2]:8334"},
		{ip: "fea2::3:3", port: 8335, want: "[fea2::3:3]:8335"},
		{ip: "feb3::4:4", port: 8336, want: "[feb3::4:4]:8336"},

		// Site-local
		{ip: "fec0::1:1", port: 8333, want: "[fec0::1:1]:8333"},
		{ip: "fed1::2:2", port: 8334, want: "[fed1::2:2]:8334"},
		{ip: "fee2::3:3", port: 8335, want: "[fee2::3:3]:8335"},
		{ip: "fef3::4:4", port: 8336, want: "[fef3::4:4]:8336"},
	}

	for _, test := range tests {
		host_ip := test.ip
		addrType, addrBytes := EncodeHost(host_ip)

		netAddr, err := NewNetAddressFromParams(addrType, addrBytes, test.port,
			time.Now(), wire.SFNodeNetwork)
		if err != nil {
			t.Fatalf("failed to construct net address from host %q: %v",
				host_ip, err)
		}

		key := netAddr.Key()
		if key != test.want {
			t.Errorf("unexpected network address key -- got %q, want %q",
				key, test.want)
			continue
		}
	}
}

// TestClone verifies that a new instance of the network address struct is
// created when cloned.
func TestClone(t *testing.T) {
	const port = 0
	netAddr := NewNetAddressFromIPPort(net.ParseIP("1.2.3.4"), port, wire.SFNodeNetwork)
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
	netAddr := NewNetAddressFromIPPort(net.ParseIP("1.2.3.4"), port, wire.SFNodeNetwork)
	netAddr.AddService(wire.SFNodeNetwork)

	if netAddr.Services != wire.SFNodeNetwork {
		t.Fatalf("expected service flag to be set -- got %x, want %x",
			netAddr.Services, wire.SFNodeNetwork)
	}
}

// TestNewNetAddressFromParams verifies that the NewNetAddressFromParams
// constructor correctly creates a network address with expected field values.
func TestNewNetAddressFromParams(t *testing.T) {
	const port = 8345
	const services = wire.SFNodeNetwork
	timestamp := time.Unix(time.Now().Unix(), 0)

	tests := []struct {
		name           string
		addrType       NetAddressType
		addrBytes      []byte
		want           *NetAddress
		error_expected bool
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
			error_expected: false,
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
			error_expected: false,
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
			error_expected: false,
		},
		{
			name:           "Error: Cannot derive net address type",
			addrType:       UnknownAddressType,
			addrBytes:      []byte{0x01, 0x02, 0x03},
			want:           nil,
			error_expected: true,
		},
		{
			name:           "Error: the provided type doesn't match the bytes",
			addrType:       IPv6Address,
			addrBytes:      net.ParseIP("127.0.0.1").To4(),
			want:           nil,
			error_expected: true,
		},
		{
			name:           "Error: no address bytes were provided",
			addrType:       UnknownAddressType,
			addrBytes:      nil,
			want:           nil,
			error_expected: true,
		}}

	for _, test := range tests {
		addr, err := NewNetAddressFromParams(test.addrType, test.addrBytes,
			port, timestamp, services)
		if err != nil && test.error_expected == false {
			t.Fatalf("%q: unexpected error - %v", test.name, err)
		}
		if !reflect.DeepEqual(addr, test.want) {
			t.Errorf("%q: mismatched entries\ngot  %+v\nwant %+v", test.name,
				addr, test.want)
		}
	}
}

// TestNewNetAddressFromString verifies that the newNetAddressFromString
// constructor correctly creates a network address with expected field values.
func TestNewNetAddressFromString(t *testing.T) {
	amgr := New("TestNewNetAddressFromString", nil)
	tests := []struct {
		name          string
		addrString    string
		want          *NetAddress
		errorExpected bool
	}{{
		name:          "Error: cannot split host:port",
		addrString:    "1.2.3.4",
		want:          nil,
		errorExpected: true,
	}, {
		name:          "Error: cannot parse the port as a uint64",
		addrString:    "1.2.3.4:abc",
		want:          nil,
		errorExpected: true,
	},
	// These tests will be added in a future commit, after a few changes to how hosts are parsed.
	// {
	// 	name:           "Error: ParseHost errored because the IP was not base32.StdEncoding",
	// 	addrString:     "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA!@.onion:8345",
	// 	want:           nil,
	// 	errorExpected: true,
	// },
	// {
	// 	name:           "Error: ParseHost did not error, but returned UnknownAddressType",
	// 	addrString:     "abc:8345",
	// 	want:           nil,
	// 	errorExpected: true,
	// },
	}

	for _, test := range tests {
		addr, err := amgr.newNetAddressFromString(test.addrString)
		if err != nil && test.errorExpected == false {
			t.Fatalf("%q: unexpected error - %v", test.name, err)
		}
		if addr != test.want {
			t.Errorf("%q: unexpected NetAddress - got %+v, want %+v",
				test.name, addr, test.want)
		}
	}
}
