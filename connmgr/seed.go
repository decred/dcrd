// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"context"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/decred/dcrd/wire"
)

const (
	// These constants are used by the seed code to pick a random last
	// seen time.
	secondsIn3Days int32 = 24 * 60 * 60 * 3
	secondsIn4Days int32 = 24 * 60 * 60 * 4
)

// OnSeed is the signature of the callback function which is invoked when
// seeding is successful.
type OnSeed func(addrs []*wire.NetAddress)

// DialFunc is the signature of the Dialer function.
type DialFunc func(context.Context, string, string) (net.Conn, error)

// node defines a single JSON object returned by the https seeders.
type node struct {
	Host            string `json:"host"`
	Services        uint64 `json:"services"`
	ProtocolVersion uint32 `json:"pver"`
}

// SeedAddrs uses HTTPS seeding to populate the address manager with peers.
func SeedAddrs(ctx context.Context, seeder string, ipversion uint16, pver uint32, services wire.ServiceFlag, dialFn DialFunc, seedFn OnSeed) error {
	req, err := http.NewRequest(http.MethodGet, "https://"+seeder, nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	req.URL.Path = "/api/addrs"
	req.URL.RawQuery = fmt.Sprintf("ipversion=%d&pver=%d&services=%d",
		ipversion, pver, services)

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: dialFn,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var nodes []node
	dec := json.NewDecoder(resp.Body)
	for ctx.Err() == nil && dec.More() {
		var node node
		if err = dec.Decode(&node); err != nil {
			return err
		}
		nodes = append(nodes, node)
	}

	log.Infof("%d addresses found from seeder %s", len(nodes), seeder)

	if len(nodes) == 0 {
		return nil
	}

	randSource := mrand.New(mrand.NewSource(time.Now().UnixNano()))
	addresses := make([]*wire.NetAddress, 0, len(nodes))
	for _, node := range nodes {
		host, portStr, err := net.SplitHostPort(node.Host)
		if err != nil {
			log.Warnf("invalid host '%s'", node.Host)
			continue
		}
		port, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			log.Warnf("invalid port '%s'", node.Host)
			continue
		}
		addresses = append(addresses,
			wire.NewNetAddressTimestamp(
				// bitcoind seeds with addresses from
				// a time randomly selected between 3
				// and 7 days ago.
				time.Now().Add(-1*time.Second*time.Duration(secondsIn3Days+
					randSource.Int31n(secondsIn4Days))),
				0, net.ParseIP(host), uint16(port)))
	}

	if len(addresses) > 0 {
		seedFn(addresses)
	}
	return nil
}
