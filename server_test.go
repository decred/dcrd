// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/addrmgr/v2"
	"github.com/decred/dcrd/wire"
)

var zeroTime = time.Time{}

var (
	torV3AddressString = "xa4r2iadxm55fbnqgwwi5mymqdcofiu3w6rpbtqn7b2dyn7mgwj64jyd.onion"
	torV3AddressBytes  = []byte{
		0xb8, 0x39, 0x1d, 0x20, 0x03, 0xbb, 0x3b, 0xd2,
		0x85, 0xb0, 0x35, 0xac, 0x8e, 0xb3, 0x0c, 0x80,
		0xc4, 0xe2, 0xa2, 0x9b, 0xb7, 0xa2, 0xf0, 0xce,
		0x0d, 0xf8, 0x74, 0x3c, 0x37, 0xec, 0x35, 0x93}
)

// TestHostToNetAddress ensures that HostToNetAddress behaves as expected
// given valid and invalid host name arguments.
func TestHostToNetAddress(t *testing.T) {
	// Define a hostname that will cause a lookup to be performed using the
	// lookupFunc provided to the address manager instance for each test.
	const hostnameForLookup = "hostname.test"
	const services = wire.SFNodeNetwork

	tests := []struct {
		name       string
		host       string
		port       uint16
		lookupFunc func(host string) ([]net.IP, error)
		wantErr    bool
		want       *addrmgr.NetAddress
	}{{
		name:       "valid TORv3 onion address",
		host:       torV3AddressString,
		port:       8333,
		lookupFunc: nil,
		wantErr:    false,
		want: (func() *addrmgr.NetAddress {
			addr, _ := addrmgr.NewNetAddressByType(addrmgr.TORv3Address,
				torV3AddressBytes, 8333, zeroTime, services)
			return addr
		})(),
	}, {
		name: "unresolvable host name",
		host: hostnameForLookup,
		port: 8333,
		lookupFunc: func(host string) ([]net.IP, error) {
			return nil, fmt.Errorf("unresolvable host %v", host)
		},
		wantErr: true,
		want:    nil,
	}, {
		name: "not resolved host name",
		host: hostnameForLookup,
		port: 8333,
		lookupFunc: func(host string) ([]net.IP, error) {
			return nil, nil
		},
		wantErr: true,
		want:    nil,
	}, {
		name: "resolved host name",
		host: hostnameForLookup,
		port: 8333,
		lookupFunc: func(host string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		},
		wantErr: false,
		want: addrmgr.NewNetAddressIPPort(net.ParseIP("127.0.0.1"), 8333,
			services),
	}, {
		name:       "valid ip address",
		host:       "12.1.2.3",
		port:       8333,
		lookupFunc: nil,
		wantErr:    false,
		want: addrmgr.NewNetAddressIPPort(net.ParseIP("12.1.2.3"), 8333,
			services),
	}}

	for _, test := range tests {
		cfg := &config{
			lookup:      test.lookupFunc,
			onionlookup: test.lookupFunc,
		}
		result, err := cfg.hostToNetAddress(test.host, test.port, services)
		if test.wantErr == true && err != nil {
			continue
		}
		if test.wantErr == true && err == nil {
			t.Errorf("%q: expected error but one was not returned", test.name)
			return
		}
		if test.wantErr == false && err != nil {
			t.Errorf("%q: failed to convert host to network address", test.name)
			return
		}
		if !reflect.DeepEqual(result.String(), test.want.String()) {
			t.Errorf("%q: unexpected result -- got %v, want %v", test.name,
				result, test.want)
			return
		}
	}
}
