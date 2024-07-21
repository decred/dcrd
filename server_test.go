// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net"
	"reflect"
	"testing"

	"github.com/decred/dcrd/addrmgr/v3"
	"github.com/decred/dcrd/wire"
)

// TestHostToNetAddress ensures that hostToNetAddress behaves as expected
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
		// 	name:       "valid onion address",
		// 	host:       "a5ccbdkubbr2jlcp.onion",
		// 	port:       8333,
		// 	lookupFunc: nil,
		// 	wantErr:    false,
		// 	want: NewNetAddressFromIPPort(
		// 		net.ParseIP("fd87:d87e:eb43:744:208d:5408:63a4:ac4f"), 8333,
		// 		services),
		// }, {
		// 	name:       "invalid onion address",
		// 	host:       "0000000000000000.onion",
		// 	port:       8333,
		// 	lookupFunc: nil,
		// 	wantErr:    true,
		// 	want:       nil,
		// }, {
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
		want: addrmgr.NewNetAddressFromIPPort(net.ParseIP("127.0.0.1"), 8333,
			services),
	}, {
		name:       "valid IPv4 address",
		host:       "12.1.2.3",
		port:       8333,
		lookupFunc: nil,
		wantErr:    false,
		want: addrmgr.NewNetAddressFromIPPort(net.ParseIP("12.1.2.3"), 8333,
			services),
	}, {
		name:       "valid IPv6 address",
		host:       "2003::1",
		port:       8333,
		lookupFunc: nil,
		wantErr:    false,
		want: addrmgr.NewNetAddressFromIPPort(net.ParseIP("2003::1"), 8333,
			services),
	}}

	for _, test := range tests {
		result, err := hostToNetAddress(test.host, test.port, services, test.lookupFunc)
		if test.wantErr == true && err == nil {
			t.Errorf("%q: expected error but one was not returned", test.name)
		}
		if !reflect.DeepEqual(result, test.want) {
			t.Errorf("%q: unexpected result - got %v, want %v", test.name,
				result, test.want)
		}
	}
}
