// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2019-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// DialFunc is the signature of the Dialer function.
type DialFunc func(context.Context, string, string) (net.Conn, error)

// HttpsSeederFilters houses filter parameters for use when making a request to
// an HTTPS seeder.  It can be configured via the various exported functions
// that start with the prefix SeedFilter.
type HttpsSeederFilters struct {
	ipVersion    uint16
	hasIPVersion bool
	pver         uint32
	hasPver      bool
	services     wire.ServiceFlag
	hasServices  bool
}

// SeedFilterIPVersion configures a request to an HTTPS seeder to filter all
// results that are not the provided ip version, which is expected to be either
// 4 or 6, to indicate IPv4 or IPv6 addresses, respectively.  The HTTPS seeder
// may choose to ignore other IP versions.
func SeedFilterIPVersion(ipVersion uint16) func(f *HttpsSeederFilters) {
	return func(f *HttpsSeederFilters) {
		f.ipVersion = ipVersion
		f.hasIPVersion = true
	}
}

// SeedFilterProtocolVersion configures a request to an HTTPS seeder to filter
// all results that are not the provided peer-to-peer protocol version.  This
// can be useful to discover peers that support specific peer versions.
func SeedFilterProtocolVersion(pver uint32) func(f *HttpsSeederFilters) {
	return func(f *HttpsSeederFilters) {
		f.pver = pver
		f.hasPver = true
	}
}

// SeedFilterServices configures a request to an HTTPS seeder to filter all
// results that do not support the provided service flags.  This can be useful
// to discover peers that support specific services such as fully-validating
// nodes.
func SeedFilterServices(services wire.ServiceFlag) func(f *HttpsSeederFilters) {
	return func(f *HttpsSeederFilters) {
		f.services = services
		f.hasServices = true
	}
}

// node defines a single JSON object returned by the https seeders.
type node struct {
	Host            string `json:"host"`
	Services        uint64 `json:"services"`
	ProtocolVersion uint32 `json:"pver"`
}

// SeedAddrs uses HTTPS seeding to return a list of addresses of p2p peers on
// the network.
//
// The seeder parameter specifies the domain name of the seed.  The dial
// function specifies the dialer to use to contact the HTTPS seeder and allows
// the caller to use whatever configuration it deems fit such as using a proxy,
// like Tor.
//
// The variadic filters parameter allows additional query filters to be applied.
// The available filters can be set via the exported functions that start with
// the prefix SeedFilter.  See the documentation for each function for more
// details.
func SeedAddrs(ctx context.Context, seeder string, dialFn DialFunc, filters ...func(f *HttpsSeederFilters)) ([]*wire.NetAddress, error) {
	// Set any caller provided filters.
	var seederFilters HttpsSeederFilters
	for _, f := range filters {
		f(&seederFilters)
	}

	// Setup the HTTPS request.
	url := "https://" + seeder
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.URL.Path = "/api/addrs"

	// Configure the query parameters based on the caller-provided filters.
	queryParams := req.URL.Query()
	if seederFilters.hasIPVersion {
		ipVersionStr := strconv.FormatInt(int64(seederFilters.ipVersion), 10)
		queryParams.Add("ipversion", ipVersionStr)
	}
	if seederFilters.hasPver {
		pverStr := strconv.FormatInt(int64(seederFilters.pver), 10)
		queryParams.Add("pver", pverStr)
	}
	if seederFilters.hasServices {
		servicesStr := strconv.FormatUint(uint64(seederFilters.services), 10)
		queryParams.Add("services", servicesStr)
	}
	req.URL.RawQuery = queryParams.Encode()

	// Make the request.
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: dialFn,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("seeder %s returned invalid status code '%d': %v",
			seeder, resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	// Parse the JSON response.
	const maxNodes = 16
	const maxRespSize = maxNodes * 256
	var nodes []node
	dec := json.NewDecoder(io.LimitReader(resp.Body, maxRespSize))
	for ctx.Err() == nil && dec.More() {
		var node node
		if err = dec.Decode(&node); err != nil {
			return nil, fmt.Errorf("unable to parse response: %w", err)
		}
		nodes = append(nodes, node)
		if len(nodes) >= maxNodes {
			break
		}
	}

	// Nothing more to do when no addresses are returned.
	if len(nodes) == 0 {
		log.Infof("0 addresses found from seeder %s", seeder)
		return nil, nil
	}

	// Convert the response to net addresses.
	randSource := mrand.New(mrand.NewSource(time.Now().UnixNano()))
	addrs := make([]*wire.NetAddress, 0, len(nodes))
	for _, node := range nodes {
		host, portStr, err := net.SplitHostPort(node.Host)
		if err != nil {
			log.Warnf("seeder returned invalid host %q", node.Host)
			continue
		}
		port, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			log.Warnf("seeder returned invalid port %q", node.Host)
			continue
		}
		ip := net.ParseIP(host)
		if ip == nil {
			log.Warnf("seeder returned a hostname that is not an IP address %q",
				host)
			continue
		}

		// Set the timestamp to a value randomly selected between 3 and 7 days
		// ago in order to improve the ranking of peers discovered from seeders
		// since they are a more authoritative source than other random peers.
		offsetSecs := secondsIn3Days + randSource.Int31n(secondsIn4Days)
		ts := time.Now().Add(-1 * time.Second * time.Duration(offsetSecs))
		na := wire.NewNetAddressTimestamp(ts, wire.ServiceFlag(node.Services),
			ip, uint16(port))
		addrs = append(addrs, na)
	}

	if len(addrs) < len(nodes) {
		log.Infof("%d addresses found from seeder %s (excluded %d invalid)",
			len(addrs), seeder, len(nodes)-len(addrs))
	} else {
		log.Infof("%d addresses found from seeder %s", len(addrs), seeder)
	}

	return addrs, nil
}
