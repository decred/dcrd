// Copyright (c) 2024-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/addrmgr/v4"
	"github.com/decred/dcrd/wire"
)

// TestHostToNetAddress ensures that hostToNetAddress behaves as expected
// given valid and invalid host name arguments.
func TestHostToNetAddress(t *testing.T) {
	// Define a hostname that will cause a lookup to be performed using the
	// lookupFunc provided to the address manager instance for each test.
	const hostnameForLookup = "hostname.test"
	const services = wire.SFNodeNetwork
	const torv3Host = "xa4r2iadxm55fbnqgwwi5mymqdcofiu3w6rpbtqn7b2dyn7mgwj64jyd.onion"

	tests := []struct {
		name       string
		host       string
		port       uint16
		lookupFunc func(host string) ([]net.IP, error)
		wantErr    bool
		want       *addrmgr.NetAddress
	}{{
		name:       "valid TorV3 address",
		host:       torv3Host,
		port:       9108,
		lookupFunc: nil,
		wantErr:    false,
		want: func() *addrmgr.NetAddress {
			addrType, addrBytes := addrmgr.EncodeHost(torv3Host)
			now := time.Unix(time.Now().Unix(), 0)
			na, _ := addrmgr.NewNetAddressFromParams(addrType, addrBytes,
				9108, now, services)
			return na
		}(),
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
