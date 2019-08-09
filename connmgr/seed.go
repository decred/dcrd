// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"fmt"
	mrand "math/rand"
	"net"
	"time"

	"github.com/decred/dcrd/wire"
)

const (
	// These constants are used by the DNS seed code to pick a random last
	// seen time.
	secondsIn3Days int32 = 24 * 60 * 60 * 3
	secondsIn4Days int32 = 24 * 60 * 60 * 4
)

// OnSeed is the signature of the callback function which is invoked when DNS
// seeding is successful.
type OnSeed func(addrs []*wire.NetAddress)

// LookupFunc is the signature of the DNS lookup function.
type LookupFunc func(string) ([]net.IP, error)

// SeedFromDNS uses DNS seeding to populate the address manager with peers.
func SeedFromDNS(dnsSeeds []string, defaultPort uint16, reqServices wire.ServiceFlag, lookupFn LookupFunc, seedFn OnSeed) {
	for _, seed := range dnsSeeds {
		host := seed
		if reqServices != wire.SFNodeNetwork {
			host = fmt.Sprintf("x%x.%s", uint64(reqServices), seed)
		}

		go func(host string) {
			randSource := mrand.New(mrand.NewSource(time.Now().UnixNano()))

			seedpeers, err := lookupFn(host)
			if err != nil {
				log.Infof("DNS discovery failed on seed %s: %v", host, err)
				return
			}
			numPeers := len(seedpeers)

			log.Infof("%d addresses found from DNS seed %s", numPeers, host)

			if numPeers == 0 {
				return
			}
			addresses := make([]*wire.NetAddress, len(seedpeers))
			// if this errors then we have *real* problems
			for i, peer := range seedpeers {
				addresses[i] = wire.NewNetAddressTimestamp(
					// bitcoind seeds with addresses from
					// a time randomly selected between 3
					// and 7 days ago.
					time.Now().Add(-1*time.Second*time.Duration(secondsIn3Days+
						randSource.Int31n(secondsIn4Days))),
					0, peer, defaultPort)
			}

			seedFn(addresses)
		}(host)
	}
}
