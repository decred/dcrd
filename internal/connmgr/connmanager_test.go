// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2019-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"math"
	mrand "math/rand/v2"
	"net"
	"net/netip"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/decred/dcrd/addrmgr/v4"
)

// prngSeed is populated when the tests are initialized either by the -seed
// parameter if specified or a source of cryptographic randomness otherwise.
var prngSeed [32]byte

// prngIteration is incremented each time a new test prng seed is requested so
// that each iteration when testing with -count > 1 gets a unique sequence of
// reproducible values.  It can be overridden via the -seed parameter when the
// tests are initialized for easy reproducibility of test failures.
var prngIteration atomic.Uint32

func TestMain(m *testing.M) {
	seedFlag := flag.String("seed", "", "use deterministic PRNG seed")
	flag.Parse()
	if *seedFlag != "" {
		parts := strings.Split(*seedFlag, "/")
		if len(parts) == 0 || len(parts) > 2 {
			fmt.Fprintln(os.Stderr, "invalid -seed: format must be "+
				"<32 byte hex seed> or <32 byte hex seed>/<iteration>")
			os.Exit(1)
		}
		b, err := hex.DecodeString(parts[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, "invalid -seed hex:", err)
			os.Exit(1)
		}
		if len(b) != 32 {
			fmt.Fprintln(os.Stderr, "invalid -seed: must be 32 bytes")
			os.Exit(1)
		}
		copy(prngSeed[:], b)
		if len(parts) > 1 {
			iteration, err := strconv.ParseUint(parts[1], 10, 32)
			if err != nil {
				fmt.Fprintln(os.Stderr, "invalid -seed iteration:", err)
				os.Exit(1)
			}
			prngIteration.Store(uint32(iteration))
		}
	} else {
		globalRand.Read(prngSeed[:])
	}
	os.Exit(m.Run())
}

// newTestPRNGSeed returns a seed to use for the deterministic test prng for the
// given iteration based on the global [prngSeed] variable which is populated in
// [TestMain].
func newTestPRNGSeed(t testing.TB) [32]byte {
	t.Helper()

	// Generate a new determinstic seed based on the test iteration count and
	// global [prngSeed] variable which can be set with flags in [TestMain].
	iteration := prngIteration.Add(1) - 1
	t.Cleanup(func() {
		t.Helper()
		if t.Failed() {
			runFlags := fmt.Sprintf("-run=%s", t.Name())
			if _, ok := t.(*testing.B); ok {
				runFlags = fmt.Sprintf("-run=^$ -bench=%s", t.Name())
			}
			t.Logf("Reproduce with: go test %s -seed=%x/%d -count=1", runFlags,
				prngSeed, iteration)
		}
	})

	// Increment the test seed by the iteration count so each test iteration has
	// a unique seed derived from the overall test seed.
	//
	// This is hacky and not cryptographically sound, but it's is only used for
	// tests, so it doesn't need to be.
	seed := prngSeed
	be := binary.BigEndian
	be.PutUint32(seed[28:], be.Uint32(seed[28:])+iteration)
	return seed
}

const (
	// defaultTestMaxRetryDuration is the default max duration a connection
	// retry backoff is allowed to grow to when running tests.
	defaultTestMaxRetryDuration = 2 * time.Millisecond

	// connTestReceiveTimeout is the default receive timeout used throughout the
	// tests when expecting to receive connections to prevent test hangs.
	connTestReceiveTimeout = 10 * time.Millisecond

	// connTestNonReceiveTimeout is the default timeout used throughout the
	// tests when expecting that a connection will NOT be received.
	connTestNonReceiveTimeout = 20 * time.Millisecond

	// defaultTestP2PPort is the default p2p port to use throughout the test.
	defaultTestP2PPort = 18555
)

// mustParseAddrPort parses the provided address into a [*addrmgr.NetAddress]
// and will panic if there is an error.  It will only (and must only) be called
// with hard-coded, and therefore known good, addresses.
func mustParseAddrPort(addr string) *addrmgr.NetAddress {
	addrPort := netip.MustParseAddrPort(addr)
	return addrmgr.NewNetAddressFromIPPort(addrPort.Addr().AsSlice(),
		addrPort.Port(), 0)
}

// addrGenerator houses state for an address generator used to simplify tests.
type addrGenerator struct {
	mtx                     sync.Mutex
	inboundGroupPrefixBits  uint
	outboundGroupPrefixBits uint
	addr                    netip.Addr
	port                    uint16
}

// newAddrGenerator returns a new address generator configured to start at the
// given base ip:port.
func newAddrGenerator(baseAddrPort string) *addrGenerator {
	addrPort := netip.MustParseAddrPort(baseAddrPort)
	return &addrGenerator{
		inboundGroupPrefixBits:  32,
		outboundGroupPrefixBits: 16,
		addr:                    addrPort.Addr(),
		port:                    addrPort.Port(),
	}
}

// next advances the generator to the next IP and returns the result.  It skips
// all addresses of the form "x.x.x.0".
func (g *addrGenerator) next() *addrmgr.NetAddress {
	// Skip "x.x.x.0".
	g.addr = g.addr.Next()
	if g.addr.As4()[3] == 0 {
		g.addr = g.addr.Next()
	}

	return addrmgr.NewNetAddressFromIPPort(g.addr.AsSlice(), g.port, 0)
}

// Next advances the generator to the next IP and returns the result.  It skips
// all addresses of the form "x.x.x.0".
//
// This provides convenient access to a new unique address with every
// invocation.
func (g *addrGenerator) Next() *addrmgr.NetAddress {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	return g.next()
}

// NextPort advances the generator to the next port of the current IP and
// returns the result.  It skips ports 0-1024.
//
// This provides convenient access to a new endpoint with the same host address
// with every invocation.
func (g *addrGenerator) NextPort() *addrmgr.NetAddress {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	g.port++
	if g.port < 1025 {
		g.port = 1025
	}

	return addrmgr.NewNetAddressFromIPPort(g.addr.AsSlice(), g.port, 0)
}

// nextPrefix advances the generator to the next IP for the given prefix bits
// and returns the result.  It skips all addresses of the form "x.x.x.0".
func (g *addrGenerator) nextPrefix(prefixBits uint) *addrmgr.NetAddress {
	if prefixBits == 32 {
		return g.next()
	}

	// Skip "x.x.x.0".
	ip := g.addr.As4()
	ip32 := binary.BigEndian.Uint32(ip[:])
	if ip32&0xff == 0 {
		ip32++
	}

	// Split the IP into network and host bits based on the number of prefix
	// bits.
	networkMask := ^uint32(0) << (32 - prefixBits)
	networkBits := (ip32 & networkMask)
	hostBits := ip32 & ^networkMask

	// Calculate the next network.
	nextNet := networkBits + (1 << (32 - prefixBits))

	// Calculate and set the next address.
	binary.BigEndian.PutUint32(ip[:], nextNet|hostBits)
	g.addr = netip.AddrFrom4(ip)

	return addrmgr.NewNetAddressFromIPPort(g.addr.AsSlice(), g.port, 0)
}

// SetFlooded sets the address generator to use inbound prefix groups per the
// given flooded state.
func (g *addrGenerator) SetFlooded(active bool) {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	bits := uint(32)
	if active {
		bits = 24
	}
	g.inboundGroupPrefixBits = bits
}

// NextInboundGroup advances the generator to the next inbound group IP and
// returns the result.  It skips all addresses of the form "x.x.x.0".
//
// An inbound group is determined by a certain number of prefix bits.
func (g *addrGenerator) NextInboundGroup() *addrmgr.NetAddress {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	return g.nextPrefix(g.inboundGroupPrefixBits)
}

// NextOutboundGroup advances the generator to the next outbound group IP and
// returns the result.  It skips all addresses of the form "x.x.x.0".
//
// An outbound group is determined by a certain number of prefix bits.
func (g *addrGenerator) NextOutboundGroup() *addrmgr.NetAddress {
	g.mtx.Lock()
	defer g.mtx.Unlock()

	return g.nextPrefix(g.outboundGroupPrefixBits)
}

// defaultAddrGenerator returns an address generator configured with a default
// starting base address and port useful throughout the tests.  The base address
// is a normal routable IPv4 address.
func defaultAddrGenerator() *addrGenerator {
	return newAddrGenerator(fmt.Sprintf("12.1.1.0:%d", defaultTestP2PPort))
}

// defaultTestAddr returns a default address to use throughout the tests.  It is
// a convenient way to get the first address generated by the default address
// generator.
func defaultTestAddr() net.Addr {
	return defaultAddrGenerator().Next()
}

// defaultMockListener returns a default mock listener to use throughout the
// tests.
func defaultMockListener() *mockListener {
	return newMockListener(fmt.Sprintf("127.0.0.1:%d", defaultTestP2PPort))
}

// runConnMgrAsync invokes [ConnManager.Run] on the passed connection manager in
// a separate goroutine and returns a cancelable context and wait group the
// caller can use to shutdown the connection manager and wait for clean
// shutdown.
//
// It also registers a test cleanup func that waits for shutdown and asserts the
// internal state of the connection manager is empty as expected.
func runConnMgrAsync(t *testing.T, cm *ConnManager) (context.Context, context.CancelFunc, *sync.WaitGroup) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		cm.Run(ctx)
		wg.Done()
	}()
	t.Cleanup(func() {
		t.Helper()

		cancel()
		wg.Wait()
		assertConnManagerCleanShutdown(t, cm)
	})

	return ctx, cancel, &wg
}

// newTestConnManager returns a new connection manager with the provided
// configuration and some timeout tweaks so that it is suitable for use in the
// tests.
func newTestConnManager(t *testing.T, cfg *Config) *ConnManager {
	t.Helper()

	cm, err := New(cfg)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	cm.maxRetryDuration = defaultTestMaxRetryDuration
	seed := newTestPRNGSeed(t)
	src := mrand.NewChaCha8(seed)
	cm.csprng = mrand.New(src) // nolint:gosec
	cm.outboundGroups.key[0] = cm.csprng.Uint64()
	cm.outboundGroups.key[1] = cm.csprng.Uint64()
	cm.inboundLimiter.key[0] = cm.csprng.Uint64()
	cm.inboundLimiter.key[1] = cm.csprng.Uint64()
	return cm
}

// assertInternalConnState ensures the internal connection state of the passed
// connection manager instance is coherent.
func assertInternalConnState(t *testing.T, cm *ConnManager) {
	t.Helper()

	cm.connMtx.Lock()
	defer cm.connMtx.Unlock()

	// Assert established persistent conns have the correct connection type.
	for id, conn := range cm.active {
		if _, ok := cm.persistent[id]; ok {
			want := ConnTypeManual
			if got := conn.Type(); got != want {
				t.Fatalf("bad conn type in active map: %v != %v", got, want)
			}
		}
	}

	// Assert the pending and active maps are mutually exclusive for both conn
	// IDs and addrs.
	//
	// Also build maps of the following data in the pending, active, and
	// persistent maps:
	//
	//   - addrs to conn IDs
	//   - the per host counts
	connIDByAddr := make(map[string]uint64)
	perHostCounts := make(map[string]uint32)
	for id, info := range cm.pending {
		if _, ok := cm.active[id]; ok {
			t.Fatalf("conn ID %d is both pending and active", id)
		}
		connIDByAddr[info.addr.String()] = id
		if _, ok := cm.persistent[id]; !ok {
			perHostCounts[addrHostKey(info.addr)]++
		}
	}
	for id, conn := range cm.active {
		if _, ok := cm.pending[id]; ok {
			t.Fatalf("conn ID %d is both pending and active", id)
		}
		addrStr := conn.remoteAddr.String()
		if _, ok := connIDByAddr[addrStr]; ok {
			t.Fatalf("addr %s is both pending and active", addrStr)
		}
		connIDByAddr[addrStr] = id
		if _, ok := cm.persistent[id]; !ok {
			perHostCounts[addrHostKey(&conn.remoteAddr)]++
		}
	}
	for id, entry := range cm.persistent {
		// Assert the conn ID of established/pending persistent conns matches.
		addrStr := entry.addr.String()
		if existingID, ok := connIDByAddr[addrStr]; ok && existingID != id {
			t.Fatalf("conn ID for addr %s mismatch: %d != %d", addrStr,
				existingID, id)
		}
		connIDByAddr[addrStr] = id
		perHostCounts[addrHostKey(entry.addr)]++
	}

	// Assert the addr to conn ID mappings match the values obtained from
	// manually constructing them.
	if !reflect.DeepEqual(cm.connIDByAddr, connIDByAddr) {
		t.Fatalf("mismatched conn ID by addr maps\ngot: %v\nwant %v",
			cm.connIDByAddr, connIDByAddr)
	}

	// Assert the per host counts match the values obtained from manually
	// tallying them.
	if !reflect.DeepEqual(cm.perHostCounts, perHostCounts) {
		t.Fatalf("mismatched per host count maps\ngot: %v\nwant %v",
			cm.perHostCounts, perHostCounts)
	}
}

// assertInternalOutboundGroupState ensures the internal outbound group state of
// the passed connection manager instance is coherent.
func assertInternalOutboundGroupState(t *testing.T, cm *ConnManager) {
	t.Helper()

	cm.outboundGroups.Lock()
	defer cm.outboundGroups.Unlock()

	// Assert the outbound group counts match a manual tally.
	outboundGroupCounts := make(map[uint64]uint32)
	for addr, count := range cm.outboundGroups.addrs {
		netAddr := mustParseAddrPort(addr)
		outboundGroupCounts[cm.outboundGroups.GroupKey(netAddr)] += count
	}
	if !reflect.DeepEqual(cm.outboundGroups.counts, outboundGroupCounts) {
		t.Fatalf("mismatched outbound group count maps\ngot: %v\nwant %v",
			cm.outboundGroups.counts, outboundGroupCounts)
	}
}

// assertInternalRateLimiterState ensures the internal rate limiting and
// flooding state of the passed connection manager instance is coherent.
func assertInternalRateLimiterState(t *testing.T, cm *ConnManager) {
	t.Helper()

	l := cm.inboundLimiter
	l.floodMtx.Lock()
	defer l.floodMtx.Unlock()

	// Assert the total attempts counts matches the value obtained from manually
	// tallying it.
	var totalAttempts uint64
	for _, attempts := range l.attempts {
		totalAttempts += uint64(attempts)
	}
	if l.totalAttempts != totalAttempts {
		t.Fatalf("mismatched total attempts count: %d != %d", l.totalAttempts,
			totalAttempts)
	}

	// Assert the flooding flag status is correct.
	flooding := l.totalAttempts > floodLow
	if got := l.flooding.Load(); got != flooding {
		t.Fatalf("mismatched flooding flag: %v != %v", got, flooding)
	}
}

// assertConnManagerInternalState ensures the internal state of the passed
// connection manager instance is coherent.
func assertConnManagerInternalState(t *testing.T, cm *ConnManager) {
	t.Helper()

	assertInternalConnState(t, cm)
	assertInternalOutboundGroupState(t, cm)
	assertInternalRateLimiterState(t, cm)
}

// assertConnManagerCleanShutdown ensures the internal state of the passed
// connection manager is fully cleaned up as expected.  It must only be called
// after [ConnManager.Run] returns.
func assertConnManagerCleanShutdown(t *testing.T, cm *ConnManager) {
	t.Helper()

	func() {
		cm.connMtx.Lock()
		defer cm.connMtx.Unlock()

		if count := len(cm.active); count != 0 {
			t.Fatalf("active map is not empty: %d entries", count)
		}
		if count := len(cm.pending); count != 0 {
			t.Fatalf("pending map is not empty: %d entries", count)
		}
		if count := len(cm.persistent); count != 0 {
			t.Fatalf("persistent map is not empty: %d entries", count)
		}
		if count := len(cm.connIDByAddr); count != 0 {
			t.Fatalf("conn ID by addr map not empty: %d entries", count)
		}
		if count := len(cm.perHostCounts); count != 0 {
			t.Fatalf("per host counts map not empty: %d entries", count)
		}
	}()

	func() {
		cm.outboundGroups.Lock()
		defer cm.outboundGroups.Unlock()

		if count := len(cm.outboundGroups.addrs); count != 0 {
			t.Fatalf("outbound group addrs map not empty: %d entries", count)
		}
		if count := len(cm.outboundGroups.counts); count != 0 {
			t.Fatalf("outbound group counts map not empty: %d entries", count)
		}
	}()
}

// TestNewConfig tests that new ConnManager config is validated as expected.
func TestNewConfig(t *testing.T) {
	t.Parallel()

	_, err := New(&Config{})
	if err == nil {
		t.Fatal("New expected error: 'Dial can't be nil', got nil")
	}

	newTestConnManager(t, &Config{
		Dial: mockDialer,
	})
}

// TestIsWhitelisted ensures [ConnManager.IsWhitelisted] works as expected.
func TestIsWhitelisted(t *testing.T) {
	type perManagerTest struct {
		addr        string // address to test against whitelist
		whitelisted bool   // expected whitelisted result
	}

	tests := []struct {
		name            string           // test description
		prefixes        []netip.Prefix   // CIDR prefixes to whitelist
		perManagerTests []perManagerTest // tests to run against the prefixes
	}{{
		name:     "no whitelisted entries",
		prefixes: nil,
		perManagerTests: []perManagerTest{
			{"1.2.3.4:18555", false},
			{"127.0.0.1:18555", false},
		},
	}, {
		name: "single /32 IPv4 entry",
		prefixes: []netip.Prefix{
			netip.MustParsePrefix("1.2.3.4/32"),
		},
		perManagerTests: []perManagerTest{
			{"1.2.3.4:18555", true},
			{"1.2.3.4:9108", true},
			{"[::1.2.3.4]:18555", false}, // IPv4 in IPv6
			{"1.2.3.5:18555", false},
		},
	}, {
		name: "single /128 IPv6 entry",
		prefixes: []netip.Prefix{
			netip.MustParsePrefix("::1.2.3.4/128"),
		},
		perManagerTests: []perManagerTest{
			{"[::1.2.3.4]:18555", true},
			{"[::1.2.3.4]:9108", true},
			{"1.2.3.4:18555", false}, // IPv4 doesn't match IPv4 in IPv6
			{"[::1.2.3.5]:9108", false},
		},
	}, {
		name: "mixed IPv4 and IPv6 with different prefix lengths",
		prefixes: []netip.Prefix{
			netip.MustParsePrefix("12.13.14.0/24"),
			netip.MustParsePrefix("20.21.22.23/8"),
			netip.MustParsePrefix("fe80::/64"),
		},
		perManagerTests: []perManagerTest{
			{"12.13.14.1:18555", true},
			{"12.13.14.255:18555", true},
			{"12.13.15.0:18555", false},
			{"20.0.0.0:18555", true},
			{"20.0.0.0:9108", true},
			{"20.255.255.255:18555", true},
			{"20.255.255.255:9108", true},
			{"21.0.0.0:18555", false},
			{"[fe80::1]:18555", true},
			{"[fe80::1]:9108", true},
			{"[fe80::ffff:ffff:ffff:ffff]:18555", true},
			{"[fe80::ffff:ffff:ffff:ffff]:1234", true},
			{"[fe80::1:ffff:ffff:ffff:ffff]:18555", false},
		},
	}}

	for _, test := range tests {
		// Parse the whitelist entries for the test.
		cm := newTestConnManager(t, &Config{
			Dial:       mockDialer,
			Whitelists: test.prefixes,
		})

		for _, pmTest := range test.perManagerTests {
			mAddr := mockAddr{"tcp", pmTest.addr}
			addr, err := stdlibNetAddrToAddrMgrNetAddr(mAddr)
			if err != nil {
				t.Fatalf("%q-%q: failed to parse address: %v", test.name,
					pmTest.addr, err)
			}
			if got := cm.IsWhitelisted(addr); got != pmTest.whitelisted {
				t.Errorf("%q-%q: mismatched result -- got %v, want %v",
					test.name, pmTest.addr, got, pmTest.whitelisted)
				continue
			}
		}
	}
}

// assertConnID ensures the provided connection has the given ID.
func assertConnID(t *testing.T, conn *Conn, wantID uint64) {
	t.Helper()

	gotID := conn.ID()
	if gotID != wantID {
		t.Fatalf("unexpected ID -- got %v, want %v", gotID, wantID)
	}
}

// assertConnType ensures the provided connection has the given type.
func assertConnType(t *testing.T, conn *Conn, wantType ConnectionType) {
	t.Helper()

	gotType := conn.Type()
	if gotType != wantType {
		t.Fatalf("unexpected type -- got %v, want %v", gotType, wantType)
	}
}

// pendingAddrConnID returns the connection ID associated with the pending
// connection attempt for the provided address.  The second return value will be
// false if no pending attempt is found.
func pendingAddrConnID(cm *ConnManager, addr net.Addr) (uint64, bool) {
	cm.connMtx.Lock()
	defer cm.connMtx.Unlock()
	addrStr := addr.String()
	for _, info := range cm.pending {
		if info.addr.String() == addrStr {
			return info.id, true
		}
	}
	return 0, false
}

// assertPendingAddr ensures there is a pending connection with the given
// address.
func assertPendingAddr(t *testing.T, cm *ConnManager, addr net.Addr) {
	t.Helper()

	if _, ok := pendingAddrConnID(cm, addr); !ok {
		t.Fatalf("connection %s is not pending", addr)
	}
}

// assertRemovedPersistent ensures there are no persistent conns with the
// provided address.
func assertRemovedPersistent(t *testing.T, cm *ConnManager, addr net.Addr) {
	t.Helper()

	if _, ok := cm.FindPersistentAddrID(addr); ok {
		t.Fatalf("found persistent entry for %s", addr)
	}
}

// assertConnReceivedTimeout ensures a connection with the given type is
// received on the provided channel before the given timeout.  When given a
// non-zero connection ID, it asserts the received connection has that ID.
func assertConnReceivedTimeout(t *testing.T, ch <-chan *Conn, timeout time.Duration, connID uint64, connType ConnectionType) *Conn {
	t.Helper()

	select {
	case conn := <-ch:
		if connID != 0 {
			assertConnID(t, conn, connID)
		}
		assertConnType(t, conn, connType)
		return conn
	case <-time.After(timeout):
		t.Fatal("connection not received before timeout")
	}
	return nil
}

// assertConnReceived ensures a connection with the given type is received on
// the provided channel before the default timeout.  When given a non-zero
// connection ID, it asserts the received connection has that ID.
func assertConnReceived(t *testing.T, ch <-chan *Conn, connID uint64, connType ConnectionType) *Conn {
	t.Helper()

	return assertConnReceivedTimeout(t, ch, connTestReceiveTimeout, connID,
		connType)
}

// assertNoConnReceivedTimeout ensures no connections are received on the
// provided channel before the given timeout.
func assertNoConnReceivedTimeout(t *testing.T, ch <-chan *Conn, timeout time.Duration) {
	t.Helper()

	select {
	case conn := <-ch:
		conn.Close()
		t.Fatalf("got unexpected connection from %v", conn.RemoteAddr())
	case <-time.After(timeout):
		// Connection not received as expected.
	}
}

// assertNoConnReceived ensures no connections are received on the provided
// channel before the default timeout.
func assertNoConnReceived(t *testing.T, ch <-chan *Conn) {
	t.Helper()

	assertNoConnReceivedTimeout(t, ch, connTestNonReceiveTimeout)
}

// assertFlooded ensures the flooding status of the connection manager is the
// given value.
func assertFlooded(t *testing.T, cm *ConnManager, want bool) {
	t.Helper()

	if got := cm.inboundLimiter.flooding.Load(); got != want {
		t.Fatalf("flooding status %v is not %v", got, want)
	}
}

// TestConnectMode tests that the connection manager works in the connect mode.
//
// In connect mode, automatic connections are disabled, so test that connections
// using [ConnManager.Connect] are handled and that no other connections are
// made.
func TestConnectMode(t *testing.T) {
	t.Parallel()

	connected := make(chan *Conn)
	cm := newTestConnManager(t, &Config{
		TargetOutbound: 2,
		Dial:           mockDialer,
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
	})
	ctx, _, _ := runConnMgrAsync(t, cm)

	go cm.Connect(ctx, defaultTestAddr())

	// Ensure that only a single connection is received.
	assertConnReceived(t, connected, 0, ConnTypeManual)
	assertNoConnReceived(t, connected)
	assertConnManagerInternalState(t, cm)
}

// TestDisconnect ensures that [ConnManager.Disconnect] properly disconnects
// pending and established connections for both non-persistent and persistent
// connections.
func TestDisconnect(t *testing.T) {
	t.Parallel()

	// Create a connection manager instance with a dialer that has a few
	// synchronization channels to notify when a dial attempt is made, to keep
	// connection attempts in a pending state, and to notify when the context
	// for the attempt is canceled.  Whether or not to wait/send the signals are
	// controlled by the associated atomic flags.
	connected := make(chan *Conn)
	disconnected := make(chan *Conn)
	dialed := make(chan struct{})
	pending := make(chan struct{})
	canceled := make(chan struct{})
	var notifyDialed, waitForPending, notifyCanceled atomic.Bool
	pendingDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if notifyDialed.Load() {
			dialed <- struct{}{}
		}
		if waitForPending.Load() {
			<-pending
		}
		conn, err := mockDialer(ctx, network, addr)
		if errors.Is(err, context.Canceled) && notifyCanceled.Load() {
			canceled <- struct{}{}
		}
		return conn, err
	}
	cm := newTestConnManager(t, &Config{
		Dial: pendingDialer,
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
		OnDisconnection: func(conn *Conn) {
			disconnected <- conn
		},
	})
	ctx, _, _ := runConnMgrAsync(t, cm)

	// Attempt a connection.
	notifyDialed.Store(true)
	waitForPending.Store(true)
	notifyCanceled.Store(true)
	addr := defaultTestAddr()
	go cm.Connect(ctx, addr)

	// Wait for the connection manager to attempt to dial and ensure the
	// connection is marked as pending while the dialer is blocked.
	select {
	case <-dialed:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting for dial")
	}
	assertPendingAddr(t, cm, addr)
	assertConnManagerInternalState(t, cm)

	// Disconnect the connection attempt while it's still pending.
	connID, _ := pendingAddrConnID(cm, addr)
	if err := cm.Disconnect(connID); err != nil {
		t.Fatalf("unexpected disconnect err: %v", err)
	}
	assertConnManagerInternalState(t, cm)

	// Allow the dialer to proceed with the disconnected connection attempt and
	// then wait for the dialer to signal the context associated with the dial
	// was canceled.  Finally, ensure the internal pending state is removed.
	select {
	case pending <- struct{}{}:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting to signal pending")
	}
	select {
	case <-canceled:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting for cancel")
	}
	if _, ok := pendingAddrConnID(cm, addr); ok {
		t.Fatalf("connection %s is still pending", addr)
	}
	assertConnManagerInternalState(t, cm)

	// Start a connection attempt and wait for it to be established.
	notifyDialed.Store(false)
	waitForPending.Store(false)
	notifyCanceled.Store(false)
	go cm.Connect(ctx, addr)
	conn := assertConnReceived(t, connected, 0, ConnTypeManual)
	assertConnManagerInternalState(t, cm)

	// Disconnect the established connection and wait for the disconnect
	// notification to ensure it is disconnected as intended.
	connID = conn.ID()
	if err := cm.Disconnect(connID); err != nil {
		t.Fatalf("unexpected disconnect err: %v", err)
	}
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cm)

	// Add a persistent connection back to the same address.
	notifyDialed.Store(true)
	waitForPending.Store(true)
	notifyCanceled.Store(true)
	connID, err := cm.AddPersistent(addr)
	if err != nil {
		t.Fatalf("failed to add persistent connection: %v", err)
	}
	assertConnManagerInternalState(t, cm)

	// Wait for the connection manager to attempt to dial and ensure the
	// connection is marked as pending while the dialer is blocked.
	select {
	case <-dialed:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting for dial")
	}
	assertPendingAddr(t, cm, addr)
	assertConnManagerInternalState(t, cm)

	// Disconnect the persistent connection attempt while it's still pending.
	if err := cm.Disconnect(connID); err != nil {
		t.Fatalf("unexpected disconnect err: %v", err)
	}
	assertConnManagerInternalState(t, cm)

	// Allow the dialer to proceed with the disconnected persistent connection
	// attempt and then wait for the dialer to signal the context associated
	// with the dial was canceled.
	select {
	case pending <- struct{}{}:
		// Ensure the reconnect attempt doesn't notify the dialed chan or
		// wait for the pending chan.
		notifyDialed.Store(false)
		waitForPending.Store(false)
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting to signal pending")
	}
	select {
	case <-canceled:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting for cancel")
	}

	// Wait for the retry to be established.
	assertConnReceived(t, connected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cm)

	// Disconnect the established persistent connection and wait for the
	// disconnect notification to ensure it is disconnected as intended.
	if err := cm.Disconnect(connID); err != nil {
		t.Fatalf("unexpected disconnect err: %v", err)
	}
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cm)
}

// TestRemove ensures that [ConnManager.Remove] properly removes pending and
// established connections for both non-persistent and persistent connections.
//
// It also ensures removal of an invalid ID returns the expected error.
func TestRemove(t *testing.T) {
	t.Parallel()

	// Create a connection manager instance with a dialer that has a few
	// synchronization channels to notify when a dial attempt is made, to keep
	// connection attempts in a pending state, and to notify when the context
	// for the attempt is canceled.  Whether or not to wait/send the signals are
	// controlled by the associated atomic flags.
	connected := make(chan *Conn)
	disconnected := make(chan *Conn)
	dialed := make(chan struct{})
	pending := make(chan struct{})
	canceled := make(chan struct{})
	var notifyDialed, waitForPending, notifyCanceled atomic.Bool
	pendingDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if notifyDialed.Load() {
			dialed <- struct{}{}
		}
		if waitForPending.Load() {
			<-pending
		}
		conn, err := mockDialer(ctx, network, addr)
		if errors.Is(err, context.Canceled) && notifyCanceled.Load() {
			canceled <- struct{}{}
		}
		return conn, err
	}
	cm := newTestConnManager(t, &Config{
		Dial: pendingDialer,
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
		OnDisconnection: func(conn *Conn) {
			disconnected <- conn
		},
	})
	ctx, _, _ := runConnMgrAsync(t, cm)

	// Ensure removing an ID that doesn't exist returns the expected error.
	if err := cm.Remove(^uint64(0)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("mismatched remove error: got %v, want %v", err, ErrNotFound)
	}

	// Attempt a connection.
	notifyDialed.Store(true)
	waitForPending.Store(true)
	notifyCanceled.Store(true)
	addr := defaultTestAddr()
	go cm.Connect(ctx, addr)

	// Wait for the connection manager to attempt to dial and ensure the
	// connection is marked as pending while the dialer is blocked.
	select {
	case <-dialed:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting for dial")
	}
	assertPendingAddr(t, cm, addr)
	assertConnManagerInternalState(t, cm)

	// Remove the connection attempt while it's still pending.
	connID, _ := pendingAddrConnID(cm, addr)
	if err := cm.Remove(connID); err != nil {
		t.Fatalf("unexpected remove err: %v", err)
	}

	// Allow the dialer to proceed with the removed connection attempt and then
	// wait for the dialer to signal the context associated with the dial was
	// canceled.  Finally, ensure the internal pending state is removed.
	select {
	case pending <- struct{}{}:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting to signal pending")
	}
	select {
	case <-canceled:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting for cancel")
	}
	if _, ok := pendingAddrConnID(cm, addr); ok {
		t.Fatalf("connection %s is still pending", addr)
	}
	assertConnManagerInternalState(t, cm)

	// Start a connection attempt and wait for it to be established.
	notifyDialed.Store(false)
	waitForPending.Store(false)
	notifyCanceled.Store(false)
	go cm.Connect(ctx, addr)
	conn := assertConnReceived(t, connected, 0, ConnTypeManual)
	assertConnManagerInternalState(t, cm)

	// Remove the established connection and wait for the disconnect
	// notification to ensure it is disconnected as intended.
	connID = conn.ID()
	if err := cm.Remove(connID); err != nil {
		t.Fatalf("unexpected disconnect err: %v", err)
	}
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cm)

	// Add a persistent connection back to the same address.
	notifyDialed.Store(true)
	waitForPending.Store(true)
	notifyCanceled.Store(true)
	connID, err := cm.AddPersistent(addr)
	if err != nil {
		t.Fatalf("failed to add persistent connection: %v", err)
	}
	assertConnManagerInternalState(t, cm)

	// Wait for the connection manager to attempt to dial and ensure the
	// connection is marked as pending while the dialer is blocked.
	select {
	case <-dialed:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting for dial")
	}
	assertPendingAddr(t, cm, addr)

	// Remove the persistent connection attempt while it's still pending.
	if err := cm.Remove(connID); err != nil {
		t.Fatalf("unexpected disconnect err: %v", err)
	}
	assertConnManagerInternalState(t, cm)

	// Allow the dialer to proceed with the removed persistent connection
	// attempt and then wait for the dialer to signal the context associated
	// with the dial was canceled.
	select {
	case pending <- struct{}{}:
		// Ensure the reconnect attempt doesn't notify the dialed chan or
		// wait for the pending chan.
		notifyDialed.Store(false)
		waitForPending.Store(false)
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting to signal pending")
	}
	select {
	case <-canceled:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting for cancel")
	}
	assertConnManagerInternalState(t, cm)

	// Add a persistent connection back to the same address and wait for it to
	// be established.
	notifyDialed.Store(false)
	waitForPending.Store(false)
	notifyCanceled.Store(false)
	connID, err = cm.AddPersistent(addr)
	if err != nil {
		t.Fatalf("failed to add persistent connection: %v", err)
	}
	conn2 := assertConnReceived(t, connected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cm)

	// Remove the established persistent connection and wait for the disconnect
	// notification to ensure it is disconnected as intended.  Also, ensure the
	// persistent connection entry is removed.
	connID = conn2.ID()
	if err := cm.Remove(connID); err != nil {
		t.Fatalf("unexpected disconnect err: %v", err)
	}
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cm)
}

// TestTargetOutbound tests the target number of outbound connections
// configuration option by waiting until all connections are established and
// ensuring they are the only connections made.
func TestTargetOutbound(t *testing.T) {
	t.Parallel()

	addrGen := defaultAddrGenerator()
	const targetOutbound = 10
	connected := make(chan *Conn)
	cm := newTestConnManager(t, &Config{
		TargetOutbound: targetOutbound,
		Dial:           mockDialer,
		GetNewAddress: func() (*addrmgr.NetAddress, time.Time, error) {
			return addrGen.NextOutboundGroup(), time.Time{}, nil
		},
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
	})
	runConnMgrAsync(t, cm)

	// Ensure only the expected number of target outbound conns are established
	// and no more.
	for range targetOutbound {
		assertConnReceived(t, connected, 0, ConnTypeOutbound)
	}
	assertNoConnReceived(t, connected)
	assertConnManagerInternalState(t, cm)
}

// TestDoubleClose ensures closing a connection multiple times is a noop after
// the first call.
func TestDoubleClose(t *testing.T) {
	t.Parallel()

	addrGen := defaultAddrGenerator()
	connected := make(chan *Conn)
	cm := newTestConnManager(t, &Config{
		TargetOutbound: 1,
		Dial:           mockDialer,
		GetNewAddress: func() (*addrmgr.NetAddress, time.Time, error) {
			return addrGen.NextOutboundGroup(), time.Time{}, nil
		},
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
	})
	runConnMgrAsync(t, cm)

	// Wait for the connection to be established.
	conn := assertConnReceived(t, connected, 0, ConnTypeOutbound)
	assertConnManagerInternalState(t, cm)

	// Override the close func to cleanly detect closes.
	var numClosed uint32
	origOnClose := conn.onClose
	conn.onClose = func() {
		numClosed++
		if numClosed == 1 {
			origOnClose()
		}
	}

	// Close the connection multiple times and make sure it only happens once.
	for range 3 {
		conn.Close()
	}
	if numClosed != 1 {
		t.Fatal("connection closed more than once")
	}
	assertConnManagerInternalState(t, cm)
}

// TestRetryPersistent tests that persistent connections are retried.
func TestRetryPersistent(t *testing.T) {
	t.Parallel()

	connected := make(chan *Conn)
	disconnected := make(chan *Conn)
	cm := newTestConnManager(t, &Config{
		RetryDuration: time.Millisecond,
		Dial:          mockDialer,
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
		OnDisconnection: func(conn *Conn) {
			disconnected <- conn
		},
	})
	runConnMgrAsync(t, cm)

	addr := defaultTestAddr()
	connID, err := cm.AddPersistent(addr)
	if err != nil {
		t.Fatalf("failed to add persistent connection: %v", err)
	}
	if !cm.IsPersistent(connID) {
		t.Fatal("IsPersistent did not reported true for persistent conn")
	}

	// Wait for the first connection, close it, wait for the disconnect, and
	// ensure the retry succeeds.
	conn := assertConnReceived(t, connected, connID, ConnTypeManual)
	conn.Close()
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertConnReceived(t, connected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cm)

	// Remove the persistent connection, wait for it to disconnect, and ensure
	// it is actually removed.
	if err := cm.Remove(connID); err != nil {
		t.Fatalf("failed to remove persistent connection: %v", err)
	}
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertRemovedPersistent(t, cm, addr)
	assertConnManagerInternalState(t, cm)
}

// TestMaxPersistent ensures [ConnManager.AddPersistent] limits the maximum
// number of persistent connections including a removal and addition of a new
// one after achieving the max.
func TestMaxPersistent(t *testing.T) {
	t.Parallel()

	connected := make(chan *Conn)
	disconnected := make(chan *Conn)
	cm := newTestConnManager(t, &Config{
		Dial: mockDialer,
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
		OnDisconnection: func(conn *Conn) {
			disconnected <- conn
		},
	})
	runConnMgrAsync(t, cm)

	// Add the maximum allowed number of persistent conns.
	addrGen := defaultAddrGenerator()
	connIDs := make([]uint64, 0, MaxPersistent)
	addrs := make([]net.Addr, 0, MaxPersistent)
	for range MaxPersistent {
		addr := addrGen.Next()
		connID, err := cm.AddPersistent(addr)
		if err != nil {
			t.Fatalf("failed to add persistent connection %v: %v", addr, err)
		}
		connIDs = append(connIDs, connID)
		addrs = append(addrs, addr)

		// Wait for the connection.
		assertConnReceived(t, connected, connID, ConnTypeManual)
		assertConnManagerInternalState(t, cm)
	}

	// Attempting to add more than the max allowed number of persistent conns
	// should be rejected.
	_, err := cm.AddPersistent(addrGen.Next())
	if !errors.Is(err, ErrMaxPersistent) {
		t.Fatalf("did not reject > max persistent, err: %v", err)
	}
	assertConnManagerInternalState(t, cm)

	// Ensure disconnecting the persistent conn does not incorrectly decrement
	// the count.
	connID, addr := connIDs[0], addrs[0]
	if err := cm.Disconnect(connID); err != nil {
		t.Fatalf("failed to disconnect persistent conn %v: %v", addr, err)
	}
	_, err = cm.AddPersistent(addrGen.Next())
	if !errors.Is(err, ErrMaxPersistent) {
		t.Fatalf("did not reject max persistent after dc, err: %v", err)
	}
	assertConnManagerInternalState(t, cm)

	// Remove the first persistent connection, wait for it to disconnect, and
	// ensure it is actually removed.
	if err := cm.Remove(connID); err != nil {
		t.Fatalf("failed to remove persistent conn %v: %v", addr, err)
	}
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertRemovedPersistent(t, cm, addr)
	assertConnManagerInternalState(t, cm)

	// A new persistent conn should now be allowed.
	addr = addrGen.Next()
	_, err = cm.AddPersistent(addr)
	if err != nil {
		t.Fatalf("failed to add persistent connection %v: %v", addr, err)
	}
	assertConnManagerInternalState(t, cm)
}

// TestMaxRetryDuration ensures the maximum retry duration is respected.
func TestMaxRetryDuration(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		networkUp := make(chan struct{})
		timedDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
			select {
			case <-networkUp:
				return mockDialer(ctx, network, addr)
			default:
				return nil, errors.New("network down")
			}
		}

		connected := make(chan *Conn)
		cm := newTestConnManager(t, &Config{
			RetryDuration: time.Second,
			Dial:          timedDialer,
			OnConnection: func(conn *Conn) {
				connected <- conn
			},
		})
		cm.maxRetryDuration = 2 * time.Second
		runConnMgrAsync(t, cm)

		connID, err := cm.AddPersistent(defaultTestAddr())
		if err != nil {
			t.Fatalf("failed to add persistent connection: %v", err)
		}

		// Approximate sequence of events.  The exact number of retries will
		// vary due to jitter.
		//
		// The test is stable regardless since it expects a connection within
		// one max retry duration of the network being brought up and, as shown
		// below, the retry duration without the max imposed would be far
		// greater and not arrive in time.
		//
		//  0s: initial attempt (retry in ~1s)
		// ~1s: retry 1 (retry in ~2s) - max retry duration reached
		// ~3s: retry 2 (retry in ~2s, w/o max would be in ~4s => next at ~7s)
		// ~5s: retry 3 (retry in ~2s, w/o max would be in ~8s => next at ~15s)
		// ~7s: retry 4 (retry in ~2s, w/o max would be in ~16s => next at ~33s)
		// ~9s: retry 5 (retry in ~2s, w/o max would be in ~32s => next at ~65s)
		// ~11s: retry 6 (retry in ~2s, w/o max would be in ~64s => next at ~129s)
		// ~12s: timedDialer returns [mockDialer]
		// ~13s: retry 7 succeeds
		networkUpTimeout := 6 * cm.maxRetryDuration
		time.AfterFunc(networkUpTimeout, func() {
			close(networkUp)
		})
		timeout := networkUpTimeout + cm.maxRetryDuration
		assertConnReceivedTimeout(t, connected, timeout, connID, ConnTypeManual)
		assertConnManagerInternalState(t, cm)
	})
}

// TestNetworkFailure tests that the connection manager handles a network
// failure gracefully.
func TestNetworkFailure(t *testing.T) {
	t.Parallel()

	var closeOnce sync.Once
	const targetOutbound = 5
	const retryTimeout = time.Millisecond * 5
	var dials atomic.Uint32
	reachedMaxFailedAttempts := make(chan struct{})
	connMgrDone := make(chan struct{})
	errDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		totalDials := dials.Add(1)
		if totalDials > maxFailedAttempts {
			closeOnce.Do(func() { close(reachedMaxFailedAttempts) })
			<-connMgrDone
		}
		return nil, errors.New("network down")
	}
	addrGen := defaultAddrGenerator()
	cm := newTestConnManager(t, &Config{
		TargetOutbound: targetOutbound,
		RetryDuration:  retryTimeout,
		Dial:           errDialer,
		GetNewAddress: func() (*addrmgr.NetAddress, time.Time, error) {
			return addrGen.NextOutboundGroup(), time.Time{}, nil
		},
		OnConnection: func(conn *Conn) {
			t.Fatalf("network failure: got unexpected connection - %v",
				conn.RemoteAddr())
		},
	})
	_, shutdown, wg := runConnMgrAsync(t, cm)

	// Shutdown the connection manager after the max failed attempts is reached
	// and an additional retry duration has passed and then wait for the
	// shutdown to complete.
	select {
	case <-reachedMaxFailedAttempts:
	case <-time.After(retryTimeout * maxFailedAttempts * 3):
		t.Fatal("did not reach target number of failed attempts before timeout")
	}
	time.Sleep(retryTimeout)
	shutdown()
	close(connMgrDone)
	wg.Wait()

	// Ensure the number of dial attempts does not exceed the max number of
	// failed attempts plus the number of potential retries during the
	// additional waiting period.
	gotDials := dials.Load()
	wantMaxDials := uint32(maxFailedAttempts + targetOutbound)
	if gotDials > wantMaxDials {
		t.Fatalf("unexpected number of dials - got %v, want <= %v", gotDials,
			wantMaxDials)
	}
}

// TestMultipleFailedConns ensures that the connection manager remains
// responsive when there are multiple simultaneous failed connections for
// persistent conns in the retry state.
func TestMultipleFailedConns(t *testing.T) {
	t.Parallel()

	// Override the max retry duration for this test since it relies on having
	// multiple connections in the retry state.
	const maxRetryDuration = 500 * time.Millisecond

	const targetFailed = 5
	var dials atomic.Uint32
	var closeOnce sync.Once
	hitTargetFailed := make(chan struct{})
	errDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		totalDials := dials.Add(1)
		if totalDials >= targetFailed {
			closeOnce.Do(func() { close(hitTargetFailed) })
		}
		return nil, errors.New("network down")
	}
	cm := newTestConnManager(t, &Config{
		RetryDuration: maxRetryDuration,
		Dial:          errDialer,
	})
	cm.maxRetryDuration = maxRetryDuration
	runConnMgrAsync(t, cm)

	// Establish several persistent connections.
	addrGen := defaultAddrGenerator()
	for range targetFailed {
		_, err := cm.AddPersistent(addrGen.Next())
		if err != nil {
			t.Fatalf("unexpected add err: %v", err)
		}
	}
	assertConnManagerInternalState(t, cm)

	// Wait for the target number of dials and ensure they happen simultaneously
	// by checking it happens before the retry timeout.
	select {
	case <-hitTargetFailed:
	case <-time.After(20 * time.Millisecond):
		t.Fatal("did not reach target number of dials before timeout")
	}
	assertConnManagerInternalState(t, cm)

	// Ensure that the connection manager still responds to requests while the
	// failed connections are still retrying.
	disconnected := make(chan struct{})
	go func() {
		const badID = ^uint64(0)
		cm.Disconnect(badID)
		close(disconnected)
	}()
	select {
	case <-disconnected:
	case <-time.After(20 * time.Millisecond):
		t.Fatal("timeout servicing connmgr requests")
	}
	assertConnManagerInternalState(t, cm)
}

// TestShutdownFailedConns tests that failed connections are ignored after
// connmgr is shutdown.
func TestShutdownFailedConns(t *testing.T) {
	t.Parallel()

	const retryTimeout = time.Second
	var closeOnce sync.Once
	dialed := make(chan struct{})
	waitDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		closeOnce.Do(func() { close(dialed) })
		return nil, errors.New("network down")
	}
	cm := newTestConnManager(t, &Config{
		RetryDuration: retryTimeout,
		Dial:          waitDialer,
	})
	cm.maxRetryDuration = retryTimeout
	runConnMgrAsync(t, cm)

	// Add a persistent connection.
	_, err := cm.AddPersistent(defaultTestAddr())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertConnManagerInternalState(t, cm)

	// Shutdown the connection manager during the retry timeout after a failed
	// dial attempt.
	select {
	case <-dialed:
	case <-time.After(connTestNonReceiveTimeout):
		t.Fatal("timeout waiting for dial")
	}
	time.Sleep(connTestNonReceiveTimeout)
}

// TestRemovePendingConnection ensures that removing a pending outbound
// connection correctly cancels the context used to dial and removes the
// internal state.
func TestRemovePendingConnection(t *testing.T) {
	t.Parallel()

	// Create a conn manager with an instance of a dialer that'll never succeed.
	dialed := make(chan struct{})
	canceled := make(chan struct{})
	indefiniteDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		close(dialed)
		<-ctx.Done()
		close(canceled)
		return nil, errors.New("error")
	}
	cm := newTestConnManager(t, &Config{
		Dial: indefiniteDialer,
	})
	ctx, _, _ := runConnMgrAsync(t, cm)

	// Establish a connection request.
	addr := defaultTestAddr()
	go cm.Connect(ctx, addr)
	assertConnManagerInternalState(t, cm)

	// Wait for the connection manager to attempt to dial and ensure the
	// connection is marked as pending while the dialer is blocked.
	select {
	case <-dialed:
	case <-time.After(time.Millisecond * 20):
		t.Fatal("timeout waiting for dial")
	}
	assertPendingAddr(t, cm, addr)
	assertConnManagerInternalState(t, cm)

	// Cancel the connection attempt while it's still pending.
	connID, _ := pendingAddrConnID(cm, addr)
	if err := cm.Remove(connID); err != nil {
		t.Fatalf("unexpected remove err: %v", err)
	}
	assertConnManagerInternalState(t, cm)

	// Wait for the dialer to signal the context associated with the dial was
	// canceled and ensure the internal pending state is removed.
	select {
	case <-canceled:
	case <-time.After(time.Millisecond * 20):
		t.Fatal("timeout waiting for cancel")
	}
	if _, ok := pendingAddrConnID(cm, addr); ok {
		t.Fatalf("connection %s is still pending", addr)
	}
	assertConnManagerInternalState(t, cm)
}

// TestCancelIgnoreDelayedConnection tests that a canceled pending persistent
// connection will not execute the on connection callback, even if a pending
// retry succeeds.
func TestCancelIgnoreDelayedConnection(t *testing.T) {
	t.Parallel()

	const retryTimeout = 10 * time.Millisecond

	// Setup a dialer that returns an error on the first attempt and then blocks
	// until the connect chan is signaled.  The dial attempt immediately after
	// that will succeed in returning a connection.
	var numAttempts atomic.Uint32
	connect := make(chan struct{})
	retried := make(chan struct{})
	failingDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if numAttempts.Add(1) == 1 {
			return nil, errors.New("network down")
		}

		close(retried)
		<-connect

		// Override the context to ensure the pending dial succeeds even though
		// the passed context will be canceled.
		ctx = context.Background()
		return mockDialer(ctx, network, addr)
	}

	connected := make(chan *Conn)
	cm := newTestConnManager(t, &Config{
		Dial:          failingDialer,
		RetryDuration: retryTimeout,
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
	})
	runConnMgrAsync(t, cm)

	// Establish a persistent connection.
	addr := defaultTestAddr()
	connID, err := cm.AddPersistent(addr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertConnManagerInternalState(t, cm)

	// Wait for the retry and ensure the connection is pending.
	select {
	case <-retried:
	case <-time.After(20 * time.Millisecond):
		t.Fatalf("did not get retry before timeout")
	}
	assertPendingAddr(t, cm, addr)
	assertConnManagerInternalState(t, cm)

	// Remove the connection and then immediately allow the next connection to
	// succeed.
	if err := cm.Remove(connID); err != nil {
		t.Fatalf("unexpected remove err: %v", err)
	}
	close(connect)

	// Finally, the connection manager should not signal the OnConnection
	// callback, since the request was explicitly canceled.  Give a generous
	// timeout window to ensure the connection manager's backoff is allowed to
	// properly elapse.
	assertNoConnReceivedTimeout(t, connected, 5*retryTimeout)
	assertConnManagerInternalState(t, cm)
}

// TestDialTimeout ensure [Config.Timeout] works as intended by creating a
// dialer that blocks for three times the configured dial timeout before
// connecting and ensuring the connection fails as expected.
func TestDialTimeout(t *testing.T) {
	t.Parallel()

	// Create a connection manager instance with a dialer that blocks for three
	// times the configured dial timeout before connecting.
	const dialTimeout = time.Millisecond * 20
	timeoutDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		select {
		case <-time.After(dialTimeout * 3):
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		return mockDialer(ctx, network, addr)
	}
	cm := newTestConnManager(t, &Config{
		Dial:        timeoutDialer,
		DialTimeout: dialTimeout,
	})
	ctx, _, _ := runConnMgrAsync(t, cm)

	// Establish a connection.
	connectErr := make(chan error, 1)
	go func() {
		_, err := cm.Connect(ctx, defaultTestAddr())
		connectErr <- err
	}()
	assertConnManagerInternalState(t, cm)

	// Wait for the error from connect and ensure it is the expected deadline
	// exceeded (aka dial timeout) error.
	select {
	case err := <-connectErr:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("unexpected connect err: got %v, want %v", err,
				context.Canceled)
		}
	case <-time.After(dialTimeout * 10):
		t.Fatal("timeout waiting for dial cancellation")
	}
	assertConnManagerInternalState(t, cm)
}

// TestConnectContext ensures the [ConnManager.Connect] method works as intended
// when provided with a context that is canceled before a dial attempt succeeds.
func TestConnectContext(t *testing.T) {
	t.Parallel()

	// Create a connection manager instance with a dialer that blocks until its
	// provided context is canceled.
	dialed := make(chan struct{})
	indefiniteDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		close(dialed)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	cm := newTestConnManager(t, &Config{
		Dial: indefiniteDialer,
	})
	ctx, _, _ := runConnMgrAsync(t, cm)

	// Establish a connection request with a separate context that can be
	// canceled.
	addr := defaultTestAddr()
	connectCtx, cancelConnect := context.WithCancel(ctx)
	connectErr := make(chan error, 1)
	go func() {
		_, err := cm.Connect(connectCtx, addr)
		connectErr <- err
	}()

	// Wait for the connection manager to attempt to dial the connection request
	// and ensure the connection is marked as pending while the dialer is
	// blocked.
	select {
	case <-dialed:
	case <-time.After(time.Millisecond * 20):
		t.Fatal("timeout waiting for dial")
	}
	assertPendingAddr(t, cm, addr)
	assertConnManagerInternalState(t, cm)

	// Cancel the connection context, wait for the error from connect, and
	// ensure it is the expected error.
	cancelConnect()
	select {
	case err := <-connectErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected connect err: got %v, want %v", err,
				context.Canceled)
		}
	case <-time.After(10 * time.Millisecond):
		t.Fatal("timeout waiting for dial cancellation")
	}
	assertConnManagerInternalState(t, cm)
}

// TestListeners ensures providing listeners to the connection manager along
// with an accept callback works properly.
func TestListeners(t *testing.T) {
	t.Parallel()

	// Setup a connection manager with a couple of mock listeners that
	// notify a channel when they receive mock connections.
	receivedConns := make(chan *Conn)
	listener1 := newMockListener("127.0.0.1:9108")
	listener2 := newMockListener("127.0.0.1:9208")
	listeners := []net.Listener{listener1, listener2}
	cm := newTestConnManager(t, &Config{
		Listeners: listeners,
		OnAccept: func(conn *Conn) {
			receivedConns <- conn
		},
		Dial: mockDialer,
	})
	runConnMgrAsync(t, cm)

	// Fake a couple of mock connections to each of the listeners.
	go func() {
		for i, listener := range listeners {
			l := listener.(*mockListener)
			l.Connect(mustParseAddrPort(fmt.Sprintf("127.0.0.1:%d", 10000+i*2)))
			l.Connect(mustParseAddrPort(fmt.Sprintf("127.0.0.1:%d", 10000+i*2+1)))
		}
	}()

	// Ensure the expected number of inbound connections are received.
	expectedNumConns := len(listeners) * 2
	for range expectedNumConns {
		assertConnReceived(t, receivedConns, 0, ConnTypeInbound)
	}
	assertConnManagerInternalState(t, cm)
}

// TestRejectDuplicateConns ensures duplicate addresses are rejected.  This
// includes:
//   - Attempts to dial addresses that already have pending, established, and
//     persistent connections (via [ConnManager.Connect]
//   - Attempts to add duplicate persistent conns (via [ConnManager.AddPersistent])
//   - Attempts to receive inbound remote addresses that already have pending,
//     established, and persistent connections
func TestRejectDuplicateConns(t *testing.T) {
	t.Parallel()

	var closeDialedOnce sync.Once
	inboundConns := make(chan *Conn)
	listener := defaultMockListener()
	connected := make(chan *Conn)
	disconnected := make(chan *Conn)
	dialed := make(chan struct{})
	pending := make(chan struct{})
	pendingDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		closeDialedOnce.Do(func() { close(dialed) })
		<-pending
		return mockDialer(ctx, network, addr)
	}
	cm := newTestConnManager(t, &Config{
		Listeners: []net.Listener{listener},
		OnAccept: func(conn *Conn) {
			inboundConns <- conn
		},
		Dial: pendingDialer,
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
		OnDisconnection: func(conn *Conn) {
			disconnected <- conn
		},
	})
	cm.inboundLimiter.burstLimit = 4
	ctx, _, _ := runConnMgrAsync(t, cm)

	// Dial a manual connection and wait for it to become pending.
	addr := defaultTestAddr()
	go cm.Connect(ctx, addr)
	select {
	case <-dialed:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("did not receive pending dial before timeout")
	}
	assertPendingAddr(t, cm, addr)
	assertConnManagerInternalState(t, cm)

	// Duplicate connect to the pending address should be rejected.
	if _, err := cm.Connect(ctx, addr); !errors.Is(err, ErrAlreadyPending) {
		t.Fatalf("did not reject duplicate pending connection, err: %v", err)
	}

	// Inbound attempts from the pending outbound address should be rejected.
	go listener.Connect(addr)
	assertNoConnReceived(t, inboundConns)
	assertConnManagerInternalState(t, cm)

	// Allow the pending connection to complete.
	close(pending)
	conn := assertConnReceived(t, connected, 0, ConnTypeManual)
	assertConnManagerInternalState(t, cm)

	// Duplicate connect to the established address should be rejected.
	if _, err := cm.Connect(ctx, addr); !errors.Is(err, ErrAlreadyConnected) {
		t.Fatalf("did not reject duplicate active connection, err: %v", err)
	}
	assertConnManagerInternalState(t, cm)

	// Inbound attempts from the established outbound address should be
	// rejected.
	go listener.Connect(addr)
	assertNoConnReceived(t, inboundConns)
	assertConnManagerInternalState(t, cm)

	// Close the connection and wait for the disconnect.
	conn.Close()
	assertConnReceived(t, disconnected, conn.ID(), ConnTypeManual)
	assertConnManagerInternalState(t, cm)

	// Add a persistent connection back to the same address and wait for it to
	// connect since there are no longer any connections to the address.
	connID, err := cm.AddPersistent(addr)
	if err != nil {
		t.Fatalf("failed to add persistent connection: %v", err)
	}
	assertConnReceived(t, connected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cm)

	// Duplicate persistent connection attempts should be rejected.
	_, err = cm.AddPersistent(addr)
	if !errors.Is(err, ErrDuplicatePersistent) {
		t.Fatalf("did not reject duplicate persistent connection, err: %v", err)
	}
	assertConnManagerInternalState(t, cm)

	// Manual connection attempts to persistent connection should be rejected.
	_, err = cm.Connect(ctx, addr)
	if !errors.Is(err, ErrDuplicatePersistent) {
		t.Fatalf("did not reject manual connection to persistent, err: %v", err)
	}
	assertConnManagerInternalState(t, cm)

	// Inbound atempts from the persistent address should be rejected.
	go listener.Connect(addr)
	assertNoConnReceived(t, inboundConns)
	assertConnManagerInternalState(t, cm)

	// Remove the persistent connection, wait for it to disconnect, and ensure
	// it is actually removed.
	if err := cm.Remove(connID); err != nil {
		t.Fatalf("failed to remove persistent connection: %v", err)
	}
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertRemovedPersistent(t, cm, addr)
	assertConnManagerInternalState(t, cm)

	// Inbound connections from the same address should now succeed.
	go listener.Connect(addr)
	assertConnReceived(t, inboundConns, 0, ConnTypeInbound)
	assertConnManagerInternalState(t, cm)

	// Manual connection attempts to the inbound address should be rejected.
	if _, err := cm.Connect(ctx, addr); !errors.Is(err, ErrAlreadyConnected) {
		t.Fatalf("did not reject outbound for existing inbound conn, err: %v",
			err)
	}
	assertConnManagerInternalState(t, cm)

	// Attempts to add a persistent connection to an existing inbound should be
	// rejected.
	_, err = cm.AddPersistent(addr)
	if !errors.Is(err, ErrAlreadyConnected) {
		t.Fatalf("did not reject persistent conn for existing inbound conn: %v",
			err)
	}
	assertConnManagerInternalState(t, cm)
}

// TestMaxNormalConns ensures the connection manager limits the total number of
// normal connections to [Config.MaxNormalConns] including automatic outbound,
// manual outbound, and inbound connections.  It also ensures that it is not
// applied to persistent connections.
func TestMaxNormalConns(t *testing.T) {
	t.Parallel()

	// Constants for the number of various normal connection types to test
	// overall max normal connection limits.
	const (
		targetOutbound = 3
		targetManual   = 4
		targetInbound  = 5
		maxNormalConns = targetOutbound + targetManual + targetInbound
	)
	connected := make(chan *Conn)
	disconnected := make(chan *Conn)
	inboundConns := make(chan *Conn)
	listener := defaultMockListener()
	var pauseTargetOutbound atomic.Bool
	var totalPausedAddrs atomic.Uint32
	hitMaxFailedAttempts := make(chan struct{})
	addrGen := defaultAddrGenerator()
	cm := newTestConnManager(t, &Config{
		Listeners:      []net.Listener{listener},
		MaxNormalConns: maxNormalConns,
		TargetOutbound: targetOutbound,
		RetryDuration:  50 * time.Millisecond,
		Dial:           mockDialer,
		OnAccept: func(conn *Conn) {
			inboundConns <- conn
		},
		GetNewAddress: func() (*addrmgr.NetAddress, time.Time, error) {
			if pauseTargetOutbound.Load() {
				total := totalPausedAddrs.Add(1)
				if total == maxFailedAttempts {
					hitMaxFailedAttempts <- struct{}{}
				}
				return nil, time.Time{}, errors.New("network down")
			}
			return addrGen.NextOutboundGroup(), time.Time{}, nil
		},
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
		OnDisconnection: func(conn *Conn) {
			disconnected <- conn
		},
	})
	cm.maxRetryDuration = cm.cfg.RetryDuration
	ctx, _, _ := runConnMgrAsync(t, cm)

	// Wait for the expected number of target outbound conns to be established.
	outbounds := make([]*Conn, 0, targetOutbound)
	for len(outbounds) < targetOutbound {
		conn := assertConnReceived(t, connected, 0, ConnTypeOutbound)
		outbounds = append(outbounds, conn)
	}
	assertConnManagerInternalState(t, cm)

	// Establish target number of inbounds to the listener and wait for them to
	// be established.
	go func() {
		for range targetInbound {
			listener.Connect(addrGen.Next())
		}
	}()
	inbounds := make([]*Conn, 0, targetInbound)
	for len(inbounds) < targetInbound {
		conn := assertConnReceived(t, inboundConns, 0, ConnTypeInbound)
		inbounds = append(inbounds, conn)
	}
	assertConnManagerInternalState(t, cm)

	// Establish target number of manual connections and wait for them to be
	// established.
	go func() {
		for range targetManual {
			go cm.Connect(ctx, addrGen.NextOutboundGroup())
		}
	}()
	manualConns := make([]*Conn, 0, targetManual+1)
	for len(manualConns) < targetManual {
		conn := assertConnReceived(t, connected, 0, ConnTypeManual)
		manualConns = append(manualConns, conn)
	}
	assertConnManagerInternalState(t, cm)

	// Ensure manual connections that would exceed the max allowed normal
	// connections are rejected.
	_, err := cm.Connect(ctx, addrGen.NextOutboundGroup())
	if !errors.Is(err, ErrMaxNormalConns) {
		t.Fatalf("did not reject manual connection at max allowed, err: %v", err)
	}
	assertConnManagerInternalState(t, cm)

	// Ensure inbound connections that would exceed the max allowed normal
	// connections are rejected.
	go listener.Connect(addrGen.NextOutboundGroup())
	assertNoConnReceived(t, inboundConns)
	assertConnManagerInternalState(t, cm)

	// Pause the target outbound dials and remove one of the target outbound
	// connections to make room for another manual connection.  Then wait for
	// the max failures to be hit so attempts are paused for a retry timeout.
	pauseTargetOutbound.Store(true)
	outboundConn := outbounds[0]
	outboundConn.Close()
	assertConnReceived(t, disconnected, outboundConn.ID(), ConnTypeOutbound)
	select {
	case <-hitMaxFailedAttempts:
		time.Sleep(connTestReceiveTimeout)
	case <-time.After(maxFailedAttempts * connTestReceiveTimeout):
		t.Fatal("did not reach max failed attempts before timeout")
	}
	assertConnManagerInternalState(t, cm)

	// Establish another manual connection to take the place of the target
	// outbound connection that was just closed and wait for it to be
	// established.
	go cm.Connect(ctx, addrGen.NextOutboundGroup())
	assertConnReceived(t, connected, 0, ConnTypeManual)
	assertConnManagerInternalState(t, cm)

	// Unpause the target outbound dials and ensure no additional automatic
	// outbound connections are made despite being under the target outbound due
	// to max total conns.
	pauseTargetOutbound.Store(false)
	assertNoConnReceivedTimeout(t, connected, connTestNonReceiveTimeout+
		cm.cfg.RetryDuration)
	assertConnManagerInternalState(t, cm)

	// Ensure persistent connections are not subject to the max total normal
	// connections by adding one and waiting for it to be established.
	connID, err := cm.AddPersistent(addrGen.NextOutboundGroup())
	if err != nil {
		t.Fatalf("failed to add persistent connection: %v", err)
	}
	assertConnReceived(t, connected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cm)
}

// TestMaxConnsPerHost ensures the connection manager limits the total number of
// connections with the same host to [Config.MaxConnsPerHost] including
// automatic outbound, manual outbound, inbound, and persistent connections.  It
// also tests whitelisted addresses are exempt.
func TestMaxConnsPerHost(t *testing.T) {
	t.Parallel()

	// nextSameHost is a convenience func to return a new address to the same IP
	// with a different port on every invocation.
	addrGen := defaultAddrGenerator()
	addrGen.Next()
	nextSameHost := addrGen.NextPort

	// nextSameHostWhitelisted is a convenience func to return a new address to
	// the same whitelisted IP with a different port on every invocation.
	allowedIP := netip.MustParseAddr("12.20.0.1")
	addrGenWhitelisted := newAddrGenerator(allowedIP.String() + ":1025")
	nextSameWhitelistedHost := addrGenWhitelisted.NextPort

	const maxConnsPerHost = 3
	connected := make(chan *Conn, 1)
	disconnected := make(chan *Conn, 1)
	inboundConns := make(chan *Conn)
	listener := defaultMockListener()
	var pauseTargetOutbound atomic.Bool
	var totalPausedAddrs atomic.Uint32
	hitMaxFailedAttempts := make(chan struct{})
	cm := newTestConnManager(t, &Config{
		Listeners:       []net.Listener{listener},
		MaxNormalConns:  30, // High enough to not interfere with per-host tests.
		MaxConnsPerHost: maxConnsPerHost,
		TargetOutbound:  maxConnsPerHost,
		RetryDuration:   50 * time.Millisecond,
		Dial:            mockDialer,
		Whitelists:      []netip.Prefix{netip.PrefixFrom(allowedIP, 32)},
		OnAccept: func(conn *Conn) {
			inboundConns <- conn
		},
		GetNewAddress: func() (*addrmgr.NetAddress, time.Time, error) {
			if pauseTargetOutbound.Load() {
				total := totalPausedAddrs.Add(1)
				if total == maxFailedAttempts {
					close(hitMaxFailedAttempts)
				}
				return nil, time.Time{}, errors.New("network down")
			}
			return nextSameHost(), time.Time{}, nil
		},
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
		OnDisconnection: func(conn *Conn) {
			disconnected <- conn
		},
	})
	cm.maxRetryDuration = cm.cfg.RetryDuration
	cm.maxPerOutboundGroup = maxConnsPerHost + 2
	ctx, _, _ := runConnMgrAsync(t, cm)

	// Wait for the maximum allowed non-whitelisted per-host automatic outbound
	// conns.
	outboundConns := make([]*Conn, 0, maxConnsPerHost)
	for len(outboundConns) < maxConnsPerHost {
		conn := assertConnReceived(t, connected, 0, ConnTypeOutbound)
		outboundConns = append(outboundConns, conn)
	}
	assertConnManagerInternalState(t, cm)

	// Ensure non-whitelisted manual connections that would exceed the max
	// allowed per-host connections are rejected.
	_, err := cm.Connect(ctx, nextSameHost())
	if !errors.Is(err, ErrMaxConnsPerHost) {
		t.Fatalf("did not reject manual connection at per-host limit, err: %v",
			err)
	}
	assertConnManagerInternalState(t, cm)

	// Ensure non-whitelisted inbound connections that would exceed the max
	// allowed per-host connections are rejected.
	go listener.Connect(nextSameHost())
	assertNoConnReceived(t, inboundConns)
	assertConnManagerInternalState(t, cm)

	// Ensure whitelisted manual connections are allowed to exceed the per-host
	// limit.
	for range maxConnsPerHost + 1 {
		go cm.Connect(ctx, nextSameWhitelistedHost())
		assertConnReceived(t, connected, 0, ConnTypeManual)
	}

	// Ensure whitelisted inbound connections are allowed to exceed the per-host
	// limit.
	go listener.Connect(nextSameWhitelistedHost())
	assertConnReceived(t, inboundConns, 0, ConnTypeInbound)
	assertConnManagerInternalState(t, cm)

	// Ensure whitelisted persistent connections are allowed to exceed the
	// per-host limit.
	connID, err := cm.AddPersistent(nextSameWhitelistedHost())
	if err != nil {
		t.Fatalf("failed to add persistent connection: %v", err)
	}
	assertConnReceived(t, connected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cm)

	// Pause the target outbound dials and remove one of the target outbound
	// connections to make room for another manual connection with the same
	// host.  Then wait for the max failures to be hit so attempts are paused
	// for a retry timeout.
	pauseTargetOutbound.Store(true)
	outboundConn := outboundConns[0]
	outboundConn.Close()
	assertConnReceived(t, disconnected, outboundConn.ID(), ConnTypeOutbound)
	select {
	case <-hitMaxFailedAttempts:
		time.Sleep(connTestReceiveTimeout)
	case <-time.After(maxFailedAttempts * connTestReceiveTimeout):
		t.Fatal("did not reach max failed attempts before timeout")
	}

	// Ensure a new non-whitelisted manual connection to the same host now
	// succeeds.
	go cm.Connect(ctx, nextSameHost())
	assertConnReceived(t, connected, 0, ConnTypeManual)
	assertConnManagerInternalState(t, cm)

	// Unpause the target outbound dials and ensure no additional automatic
	// outbound connections to the same host are made despite being under the
	// target outbound.
	noConnWaitTimeout := connTestReceiveTimeout + cm.cfg.RetryDuration
	pauseTargetOutbound.Store(false)
	assertNoConnReceivedTimeout(t, connected, noConnWaitTimeout)
	assertConnManagerInternalState(t, cm)

	// Ensure persistent connections are also subject to the max per-host
	// connections by adding one and confirming it is NOT established.
	_, err = cm.AddPersistent(nextSameHost())
	if err != nil {
		t.Fatalf("failed to add persistent connection: %v", err)
	}
	assertNoConnReceivedTimeout(t, connected, noConnWaitTimeout)
	assertConnManagerInternalState(t, cm)
}

// TestOutboundGroups ensures the connection manager limits the automatic
// outbound connections to one connection per outbound group.  It includes
// randomized address generation with some addresses in the same group, some
// with non-default ports, and some recently attempted or not to exercise the
// retry logic as well.
func TestOutboundGroups(t *testing.T) {
	t.Parallel()

	addrGen := defaultAddrGenerator()
	defaultPort := addrGen.port
	var cm *ConnManager
	randomizedNewAddr := func() (*addrmgr.NetAddress, time.Time, error) {
		// Only return a new outbound group 10% of the time.
		var addr *addrmgr.NetAddress
		if rv := cm.csprng.Uint64N(10); rv < 1 {
			addr = addrGen.NextOutboundGroup()
		} else {
			addr = addrGen.Next()
		}

		// Return a random port 50% of the time.
		if cm.csprng.Uint64N(10) < 5 {
			const minPort = 1025
			addr.Port = uint16(minPort + cm.csprng.Uint64N(1<<16-1-minPort))
		}

		// Return a recent last attempt 30% of the time.
		var lastAttempt time.Time
		if cm.csprng.Uint64N(10) < 3 {
			lastAttempt = time.Now().Add(-20 * time.Second)
		}

		return addr, lastAttempt, nil
	}

	const targetOutbound = 5
	connected := make(chan *Conn)
	cm = newTestConnManager(t, &Config{
		TargetOutbound: targetOutbound,
		RetryDuration:  50 * time.Millisecond,
		DefaultPort:    defaultPort,
		Dial:           mockDialer,
		GetNewAddress:  randomizedNewAddr,
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
	})
	cm.maxRetryDuration = cm.cfg.RetryDuration
	runConnMgrAsync(t, cm)

	// Wait for the expected number of target outbound conns to be established.
	groups := make(map[uint64]struct{})
	outbounds := make([]*Conn, 0, targetOutbound)
	for len(outbounds) < targetOutbound {
		conn := assertConnReceived(t, connected, 0, ConnTypeOutbound)
		outbounds = append(outbounds, conn)
		groups[cm.outboundGroups.GroupKey(&conn.remoteAddr)] = struct{}{}
	}
	assertConnManagerInternalState(t, cm)

	// Ensure only one address per outbound group was selected.
	if len(groups) != targetOutbound {
		t.Fatalf("unexpected number of outbound groups -- got %d, want %d",
			len(groups), targetOutbound)
	}
}

// TestInboundRateLimiting ensures the connection manager rate limits inbound
// connections behavior as expected.  It includes tests for normal rate
// limiting, bursts, flooding, and flood recovery.
func TestInboundRateLimiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inboundConns := make(chan *Conn)
		listener := defaultMockListener()
		cm := newTestConnManager(t, &Config{
			Listeners:       []net.Listener{listener},
			MaxConnsPerHost: 100,
			OnAccept: func(conn *Conn) {
				inboundConns <- conn
			},
			Dial: mockDialer,
		})
		runConnMgrAsync(t, cm)

		// Ensure exactly the max allowed burst of inbound connections from the
		// same address are accepted.
		addrGen := defaultAddrGenerator()
		addrGen.Next()
		for range groupBurstLimit {
			go listener.Connect(addrGen.NextPort())
			assertConnReceived(t, inboundConns, 0, ConnTypeInbound).Close()
		}
		assertConnManagerInternalState(t, cm)

		// Ensure connections from the same address are now rate limited.
		for range 3 {
			go listener.Connect(addrGen.NextPort())
		}
		assertNoConnReceived(t, inboundConns)
		assertConnManagerInternalState(t, cm)

		// Wait just long enough for the next connection to be allowed and
		// ensure it is.
		perConnSecs := time.Duration(math.Ceil(1/groupRateLimit)) * time.Second
		time.Sleep(perConnSecs - connTestNonReceiveTimeout)
		go listener.Connect(addrGen.NextPort())
		assertConnReceived(t, inboundConns, 0, ConnTypeInbound).Close()
		assertConnManagerInternalState(t, cm)

		// Wait just long enough to reset the burst tokens and ensure another
		// burst of inbound connections from the same address are accepted.
		time.Sleep(groupBurstLimit * perConnSecs)
		for range groupBurstLimit {
			go listener.Connect(addrGen.NextPort())
			assertConnReceived(t, inboundConns, 0, ConnTypeInbound).Close()
		}
		assertConnManagerInternalState(t, cm)

		// Ensure the next inbound group is not rate limited and independently
		// allows the max allowed burst.
		addrGen.NextInboundGroup()
		for range groupBurstLimit {
			go listener.Connect(addrGen.NextPort())
			assertConnReceived(t, inboundConns, 0, ConnTypeInbound).Close()
		}
		assertConnManagerInternalState(t, cm)

		// Ensure connections from the same address in the new inbound group are
		// now rate limited.
		for range 3 {
			go listener.Connect(addrGen.NextPort())
		}
		assertNoConnReceived(t, inboundConns)
		assertConnManagerInternalState(t, cm)

		// Make exactly enough allowed connections to reach one prior to the low
		// intensity flood cutover.  Ensure all connections are accepted and
		// flooding is not detected.
		//
		// Wait long enough to reset the flood state first to simplify the
		// calcs.
		//
		// Then, advance time such that the connections fill up the entire
		// sliding window used to track allowed connections for flood detection.
		time.Sleep(60 * time.Second)
		for i := range floodLow {
			if i%groupBurstLimit == 0 {
				addrGen.NextInboundGroup()
			}
			go listener.Connect(addrGen.NextPort())
			assertConnReceived(t, inboundConns, 0, ConnTypeInbound).Close()
			time.Sleep(60 * time.Second / (floodLow + 1))
		}
		assertConnManagerInternalState(t, cm)
		assertFlooded(t, cm, false)

		// Ensure the next connection activates flooding mode.
		//
		// The current inbound group might be rate limited depending on the
		// actual values above and flooding mode will have coarsened it, so use
		// an address from the next inbound group with the coarsened prefix.
		//
		// The connection may be probabilistically dropped due to flooding, so
		// it may or may not be accepted.
		addrGen.SetFlooded(true)
		go listener.Connect(addrGen.NextInboundGroup())
		select {
		case conn := <-inboundConns:
			assertConnType(t, conn, ConnTypeInbound)
			conn.Close()
		case <-time.After(connTestNonReceiveTimeout):
		}
		assertConnManagerInternalState(t, cm)
		assertFlooded(t, cm, true)

		// Make enough connections from the same address to hit the group burst
		// limit.
		//
		// The connections may be probabilistically dropped due to flooding, so
		// they may or may not be accepted.
		for range groupBurstLimit {
			go listener.Connect(addrGen.NextPort())
			select {
			case conn := <-inboundConns:
				assertConnType(t, conn, ConnTypeInbound)
				conn.Close()
			case <-time.After(connTestNonReceiveTimeout):
			}
		}
		assertConnManagerInternalState(t, cm)
		assertFlooded(t, cm, true)

		// Ensure the next few addresss in the same inbound group are now rate
		// limited.  Use a value high enough to ensure they aren't just being
		// dropped probabilistically.
		for range int(math.Ceil(groupBurstLimit/(1-floodMaxDropProb))) * 2 {
			go listener.Connect(addrGen.Next())
		}
		assertNoConnReceived(t, inboundConns)
		assertConnManagerInternalState(t, cm)

		// Ensure flood mode is deactivated once flooding subsides.
		//
		// Wait for an entire window to pass with no connections and then ensure
		// the same address can connect up to the burst limit again.
		time.Sleep(time.Minute)
		addrGen.SetFlooded(false)
		for range groupBurstLimit {
			go listener.Connect(addrGen.NextPort())
			assertConnReceived(t, inboundConns, 0, ConnTypeInbound).Close()
		}
		assertConnManagerInternalState(t, cm)
		assertFlooded(t, cm, false)
	})
}
