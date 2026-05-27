// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2019-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/decred/dcrd/addrmgr/v4"
)

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
)

// mustParseAddrPort parses the provided address into a [*addrmgr.NetAddress]
// and will panic if there is an error.  It will only (and must only) be called
// with hard-coded, and therefore known good, addresses.
func mustParseAddrPort(addr string) *addrmgr.NetAddress {
	addrPort := netip.MustParseAddrPort(addr)
	return addrmgr.NewNetAddressFromIPPort(addrPort.Addr().AsSlice(),
		addrPort.Port(), 0)
}

// runConnMgrAsync invokes the Run method on the passed connection manager in a
// separate goroutine and returns a cancelable context and wait group the caller
// can use to shutdown the connection manager and wait for clean shutdown.
func runConnMgrAsync(ctx context.Context, cmgr *ConnManager) (context.Context, context.CancelFunc, *sync.WaitGroup) {
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		cmgr.Run(ctx)
		wg.Done()
	}()
	return ctx, cancel, &wg
}

// mockAddr mocks a network address.
type mockAddr struct {
	net, address string
}

func (m mockAddr) Network() string { return m.net }
func (m mockAddr) String() string  { return m.address }

// mockConn mocks a network connection by implementing the net.Conn interface.
type mockConn struct {
	io.Reader
	io.Writer
	io.Closer

	// local network, address for the connection.
	lnet, laddr string

	// remote network, address for the connection.
	rAddr net.Addr
}

// LocalAddr returns the local address for the connection.
func (c mockConn) LocalAddr() net.Addr {
	return &mockAddr{c.lnet, c.laddr}
}

// RemoteAddr returns the remote address for the connection.
func (c mockConn) RemoteAddr() net.Addr {
	return &mockAddr{c.rAddr.Network(), c.rAddr.String()}
}

// Close handles closing the connection.
func (c mockConn) Close() error {
	return nil
}

func (c mockConn) SetDeadline(t time.Time) error      { return nil }
func (c mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (c mockConn) SetWriteDeadline(t time.Time) error { return nil }

// mockDialer mocks the net.Dial interface by returning a mock connection to
// the given address.
func mockDialer(ctx context.Context, network, addr string) (net.Conn, error) {
	r, w := io.Pipe()
	c := &mockConn{rAddr: &mockAddr{network, addr}}
	c.Reader = r
	c.Writer = w
	return c, ctx.Err()
}

// newTestConnManager returns a new connection manager with the provided
// configuration and some timeout tweaks so that it is suitable for use in the
// tests.
func newTestConnManager(t *testing.T, cfg *Config) *ConnManager {
	t.Helper()

	cmgr, err := New(cfg)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	cmgr.maxRetryDuration = defaultTestMaxRetryDuration
	return cmgr
}

// assertConnManagerInternalState ensures the internal state of the passed
// connection manager instance is coherent.
func assertConnManagerInternalState(t *testing.T, cm *ConnManager) {
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
	// Also build a map of addrs to conn IDs in the pending, active, and
	// persistent maps for the checks below.
	connIDByAddr := make(map[string]uint64)
	for id, info := range cm.pending {
		if _, ok := cm.active[id]; ok {
			t.Fatalf("conn ID %d is both pending and active", id)
		}
		connIDByAddr[info.addr.String()] = id
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
	}
	for id, entry := range cm.persistent {
		// Assert the conn ID of established/pending persistent conns matches.
		addrStr := entry.addr.String()
		if existingID, ok := connIDByAddr[addrStr]; ok && existingID != id {
			t.Fatalf("conn ID for addr %s mismatch: %d != %d", addrStr,
				existingID, id)
		}
		connIDByAddr[addrStr] = id
	}

	// Assert the addr to conn ID mappings match the values obtained from
	// manually constructing them.
	if !reflect.DeepEqual(cm.connIDByAddr, connIDByAddr) {
		t.Fatalf("mismatched conn ID by addr maps\ngot: %v\nwant %v",
			cm.connIDByAddr, connIDByAddr)
	}
}

// assertConnManagerCleanShutdown ensures the internal state of the passed
// connection manager is fully cleaned up as expected.  It must only be called
// after [ConnManager.Run] returns.
func assertConnManagerCleanShutdown(t *testing.T, cm *ConnManager) {
	t.Helper()

	cm.connMtx.Lock()
	defer cm.connMtx.Unlock()

	if len(cm.active) != 0 {
		t.Fatalf("active map is not empty: %d entries", len(cm.active))
	}
	if len(cm.pending) != 0 {
		t.Fatalf("pending map is not empty: %d entries", len(cm.pending))
	}
	if len(cm.persistent) != 0 {
		t.Fatalf("persistent map is not empty: %d entries", len(cm.persistent))
	}
	if len(cm.connIDByAddr) != 0 {
		t.Fatalf("conn ID by addr map not empty: %d entries",
			len(cm.connIDByAddr))
	}
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
		cmgr := newTestConnManager(t, &Config{
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
			if got := cmgr.IsWhitelisted(addr); got != pmTest.whitelisted {
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
func pendingAddrConnID(cm *ConnManager, addr *addrmgr.NetAddress) (uint64, bool) {
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
func assertPendingAddr(t *testing.T, cm *ConnManager, addr *addrmgr.NetAddress) {
	t.Helper()

	if _, ok := pendingAddrConnID(cm, addr); !ok {
		t.Fatalf("connection %s is not pending", addr)
	}
}

// assertRemovedPersistent ensures there are no persistent conns with the
// provided address.
func assertRemovedPersistent(t *testing.T, cm *ConnManager, addr *addrmgr.NetAddress) {
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

// TestConnectMode tests that the connection manager works in the connect mode.
//
// In connect mode, automatic connections are disabled, so test that connections
// using [ConnManager.Connect] are handled and that no other connections are
// made.
func TestConnectMode(t *testing.T) {
	t.Parallel()

	connected := make(chan *Conn)
	cmgr := newTestConnManager(t, &Config{
		TargetOutbound: 2,
		Dial:           mockDialer,
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
	})
	ctx, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	addr := mustParseAddrPort("127.0.0.1:18555")
	go cmgr.Connect(ctx, addr)

	// Ensure that only a single connection is received.
	assertConnReceived(t, connected, 0, ConnTypeManual)
	assertNoConnReceived(t, connected)
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
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
	cmgr := newTestConnManager(t, &Config{
		Dial: pendingDialer,
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
		OnDisconnection: func(conn *Conn) {
			disconnected <- conn
		},
	})
	ctx, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Attempt a connection to a localhost IP.
	notifyDialed.Store(true)
	waitForPending.Store(true)
	notifyCanceled.Store(true)
	addr := mustParseAddrPort("127.0.0.1:18555")
	go cmgr.Connect(ctx, addr)

	// Wait for the connection manager to attempt to dial and ensure the
	// connection is marked as pending while the dialer is blocked.
	select {
	case <-dialed:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting for dial")
	}
	assertPendingAddr(t, cmgr, addr)
	assertConnManagerInternalState(t, cmgr)

	// Disconnect the connection attempt while it's still pending.
	connID, _ := pendingAddrConnID(cmgr, addr)
	if err := cmgr.Disconnect(connID); err != nil {
		t.Fatalf("unexpected disconnect err: %v", err)
	}
	assertConnManagerInternalState(t, cmgr)

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
	if _, ok := pendingAddrConnID(cmgr, addr); ok {
		t.Fatalf("connection %s is still pending", addr)
	}
	assertConnManagerInternalState(t, cmgr)

	// Start a connection attempt and wait for it to be established.
	notifyDialed.Store(false)
	waitForPending.Store(false)
	notifyCanceled.Store(false)
	go cmgr.Connect(ctx, addr)
	conn := assertConnReceived(t, connected, 0, ConnTypeManual)
	assertConnManagerInternalState(t, cmgr)

	// Disconnect the established connection and wait for the disconnect
	// notification to ensure it is disconnected as intended.
	connID = conn.ID()
	if err := cmgr.Disconnect(connID); err != nil {
		t.Fatalf("unexpected disconnect err: %v", err)
	}
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cmgr)

	// Add a persistent connection back to the same address.
	notifyDialed.Store(true)
	waitForPending.Store(true)
	notifyCanceled.Store(true)
	connID, err := cmgr.AddPersistent(addr)
	if err != nil {
		t.Fatalf("failed to add persistent connection: %v", err)
	}
	assertConnManagerInternalState(t, cmgr)

	// Wait for the connection manager to attempt to dial and ensure the
	// connection is marked as pending while the dialer is blocked.
	select {
	case <-dialed:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting for dial")
	}
	assertPendingAddr(t, cmgr, addr)
	assertConnManagerInternalState(t, cmgr)

	// Disconnect the persistent connection attempt while it's still pending.
	if err := cmgr.Disconnect(connID); err != nil {
		t.Fatalf("unexpected disconnect err: %v", err)
	}
	assertConnManagerInternalState(t, cmgr)

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
	assertConnManagerInternalState(t, cmgr)

	// Disconnect the established persistent connection and wait for the
	// disconnect notification to ensure it is disconnected as intended.
	if err := cmgr.Disconnect(connID); err != nil {
		t.Fatalf("unexpected disconnect err: %v", err)
	}
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
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
	cmgr := newTestConnManager(t, &Config{
		Dial: pendingDialer,
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
		OnDisconnection: func(conn *Conn) {
			disconnected <- conn
		},
	})
	ctx, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Ensure removing an ID that doesn't exist returns the expected error.
	if err := cmgr.Remove(^uint64(0)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("mismatched remove error: got %v, want %v", err, ErrNotFound)
	}

	// Attempt a connection to a localhost IP.
	notifyDialed.Store(true)
	waitForPending.Store(true)
	notifyCanceled.Store(true)
	addr := mustParseAddrPort("127.0.0.1:18555")
	go cmgr.Connect(ctx, addr)

	// Wait for the connection manager to attempt to dial and ensure the
	// connection is marked as pending while the dialer is blocked.
	select {
	case <-dialed:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting for dial")
	}
	assertPendingAddr(t, cmgr, addr)
	assertConnManagerInternalState(t, cmgr)

	// Remove the connection attempt while it's still pending.
	connID, _ := pendingAddrConnID(cmgr, addr)
	if err := cmgr.Remove(connID); err != nil {
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
	if _, ok := pendingAddrConnID(cmgr, addr); ok {
		t.Fatalf("connection %s is still pending", addr)
	}
	assertConnManagerInternalState(t, cmgr)

	// Start a connection attempt and wait for it to be established.
	notifyDialed.Store(false)
	waitForPending.Store(false)
	notifyCanceled.Store(false)
	go cmgr.Connect(ctx, addr)
	conn := assertConnReceived(t, connected, 0, ConnTypeManual)
	assertConnManagerInternalState(t, cmgr)

	// Remove the established connection and wait for the disconnect
	// notification to ensure it is disconnected as intended.
	connID = conn.ID()
	if err := cmgr.Remove(connID); err != nil {
		t.Fatalf("unexpected disconnect err: %v", err)
	}
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cmgr)

	// Add a persistent connection back to the same address.
	notifyDialed.Store(true)
	waitForPending.Store(true)
	notifyCanceled.Store(true)
	connID, err := cmgr.AddPersistent(addr)
	if err != nil {
		t.Fatalf("failed to add persistent connection: %v", err)
	}
	assertConnManagerInternalState(t, cmgr)

	// Wait for the connection manager to attempt to dial and ensure the
	// connection is marked as pending while the dialer is blocked.
	select {
	case <-dialed:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("timeout waiting for dial")
	}
	assertPendingAddr(t, cmgr, addr)

	// Remove the persistent connection attempt while it's still pending.
	if err := cmgr.Remove(connID); err != nil {
		t.Fatalf("unexpected disconnect err: %v", err)
	}
	assertConnManagerInternalState(t, cmgr)

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
	assertConnManagerInternalState(t, cmgr)

	// Add a persistent connection back to the same address and wait for it to
	// be established.
	notifyDialed.Store(false)
	waitForPending.Store(false)
	notifyCanceled.Store(false)
	connID, err = cmgr.AddPersistent(addr)
	if err != nil {
		t.Fatalf("failed to add persistent connection: %v", err)
	}
	conn2 := assertConnReceived(t, connected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cmgr)

	// Remove the established persistent connection and wait for the disconnect
	// notification to ensure it is disconnected as intended.  Also, ensure the
	// persistent connection entry is removed.
	connID = conn2.ID()
	if err := cmgr.Remove(connID); err != nil {
		t.Fatalf("unexpected disconnect err: %v", err)
	}
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
}

// TestTargetOutbound tests the target number of outbound connections
// configuration option by waiting until all connections are established and
// ensuring they are the only connections made.
func TestTargetOutbound(t *testing.T) {
	t.Parallel()

	const targetOutbound = 10
	var nextAddr atomic.Uint32
	connected := make(chan *Conn)
	cmgr := newTestConnManager(t, &Config{
		TargetOutbound: targetOutbound,
		Dial:           mockDialer,
		GetNewAddress: func() (net.Addr, error) {
			addrStr := fmt.Sprintf("127.0.0.%d:18555", nextAddr.Add(1))
			return mustParseAddrPort(addrStr), nil
		},
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
	})
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Ensure only the expected number of target outbound conns are established
	// and no more.
	for range targetOutbound {
		assertConnReceived(t, connected, 0, ConnTypeOutbound)
	}
	assertNoConnReceived(t, connected)
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
}

// TestDoubleClose ensures closing a connection multiple times is a noop after
// the first call.
func TestDoubleClose(t *testing.T) {
	t.Parallel()

	connected := make(chan *Conn)
	cmgr := newTestConnManager(t, &Config{
		TargetOutbound: 1,
		Dial:           mockDialer,
		GetNewAddress: func() (net.Addr, error) {
			return mustParseAddrPort("127.0.0.1:18555"), nil
		},
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
	})
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Wait for the connection to be established.
	conn := assertConnReceived(t, connected, 0, ConnTypeOutbound)
	assertConnManagerInternalState(t, cmgr)

	// Override the close func to cleanly detect closes.
	var numClosed uint32
	origOnClose := conn.onClose
	conn.onClose = func() {
		numClosed++
		origOnClose()
	}

	// Close the connection multiple times and make sure it only happens once.
	for range 3 {
		conn.Close()
	}
	if numClosed != 1 {
		t.Fatal("connection closed more than once")
	}
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
}

// TestRetryPersistent tests that persistent connections are retried.
func TestRetryPersistent(t *testing.T) {
	t.Parallel()

	connected := make(chan *Conn)
	disconnected := make(chan *Conn)
	cmgr := newTestConnManager(t, &Config{
		RetryDuration: time.Millisecond,
		Dial:          mockDialer,
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
		OnDisconnection: func(conn *Conn) {
			disconnected <- conn
		},
	})
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	addr := mustParseAddrPort("127.0.0.1:18555")
	connID, err := cmgr.AddPersistent(addr)
	if err != nil {
		t.Fatalf("failed to add persistent connection: %v", err)
	}
	if !cmgr.IsPersistent(connID) {
		t.Fatal("IsPersistent did not reported true for persistent conn")
	}

	// Wait for the first connection, close it, wait for the disconnect, and
	// ensure the retry succeeds.
	conn := assertConnReceived(t, connected, connID, ConnTypeManual)
	conn.Close()
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertConnReceived(t, connected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cmgr)

	// Remove the persistent connection, wait for it to disconnect, and ensure
	// it is actually removed.
	if err := cmgr.Remove(connID); err != nil {
		t.Fatalf("failed to remove persistent connection: %v", err)
	}
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertRemovedPersistent(t, cmgr, addr)
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
}

// TestMaxPersistent ensures [ConnManager.AddPersistent] limits the maximum
// number of persistent connections including a removal and addition of a new
// one after achieving the max.
func TestMaxPersistent(t *testing.T) {
	t.Parallel()

	connected := make(chan *Conn)
	disconnected := make(chan *Conn)
	cmgr := newTestConnManager(t, &Config{
		Dial: mockDialer,
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
		OnDisconnection: func(conn *Conn) {
			disconnected <- conn
		},
	})
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	var numAddrs uint32
	nextAddr := func() *addrmgr.NetAddress {
		numAddrs++
		addrStr := fmt.Sprintf("127.0.0.%d:18555", numAddrs)
		return mustParseAddrPort(addrStr)
	}

	// Add the maximum allowed number of persistent conns.
	connIDs := make([]uint64, 0, MaxPersistent)
	addrs := make([]*addrmgr.NetAddress, 0, MaxPersistent)
	for range MaxPersistent {
		addr := nextAddr()
		connID, err := cmgr.AddPersistent(addr)
		if err != nil {
			t.Fatalf("failed to add persistent connection %v: %v", addr, err)
		}
		connIDs = append(connIDs, connID)
		addrs = append(addrs, addr)

		// Wait for the connection.
		assertConnReceived(t, connected, connID, ConnTypeManual)
		assertConnManagerInternalState(t, cmgr)
	}

	// Attempting to add more than the max allowed number of persistent conns
	// should be rejected.
	_, err := cmgr.AddPersistent(nextAddr())
	if !errors.Is(err, ErrMaxPersistent) {
		t.Fatalf("did not reject > max persistent, err: %v", err)
	}
	assertConnManagerInternalState(t, cmgr)

	// Ensure disconnecting the persistent conn does not incorrectly decrement
	// the count.
	connID, addr := connIDs[0], addrs[0]
	if err := cmgr.Disconnect(connID); err != nil {
		t.Fatalf("failed to disconnect persistent conn %v: %v", addr, err)
	}
	_, err = cmgr.AddPersistent(nextAddr())
	if !errors.Is(err, ErrMaxPersistent) {
		t.Fatalf("did not reject max persistent after dc, err: %v", err)
	}
	assertConnManagerInternalState(t, cmgr)

	// Remove the first persistent connection, wait for it to disconnect, and
	// ensure it is actually removed.
	if err := cmgr.Remove(connID); err != nil {
		t.Fatalf("failed to remove persistent conn %v: %v", addr, err)
	}
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertRemovedPersistent(t, cmgr, addr)
	assertConnManagerInternalState(t, cmgr)

	// A new persistent conn should now be allowed.
	addr = nextAddr()
	_, err = cmgr.AddPersistent(addr)
	if err != nil {
		t.Fatalf("failed to add persistent connection %v: %v", addr, err)
	}
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
}

// TestMaxRetryDuration tests the maximum retry duration.
//
// We have a timed dialer which initially returns err but after RetryDuration
// hits maxRetryDuration returns a mock conn.
func TestMaxRetryDuration(t *testing.T) {
	t.Parallel()

	// This test relies on the current value of the max retry duration defined
	// in the tests, so assert it.
	if defaultTestMaxRetryDuration != 2*time.Millisecond {
		t.Fatalf("max retry duration of %v is not the required value for test",
			defaultTestMaxRetryDuration)
	}

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
	cmgr := newTestConnManager(t, &Config{
		RetryDuration:  time.Millisecond,
		TargetOutbound: 1,
		Dial:           timedDialer,
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
	})
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	connID, err := cmgr.AddPersistent(mustParseAddrPort("127.0.0.1:18555"))
	if err != nil {
		t.Fatalf("failed to add persistent connection: %v", err)
	}

	// retry in 1ms
	// retry in 2ms - max retry duration reached
	// retry in 2ms - timedDialer returns [mockDialer]
	const networkUpTimeout = 5 * time.Millisecond
	time.AfterFunc(networkUpTimeout, func() {
		close(networkUp)
	})
	const timeout = connTestReceiveTimeout + networkUpTimeout
	assertConnReceivedTimeout(t, connected, timeout, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
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
	var nextAddr atomic.Uint32
	cmgr := newTestConnManager(t, &Config{
		TargetOutbound: targetOutbound,
		RetryDuration:  retryTimeout,
		Dial:           errDialer,
		GetNewAddress: func() (net.Addr, error) {
			addrStr := fmt.Sprintf("127.0.0.%d:18555", nextAddr.Add(1))
			return mustParseAddrPort(addrStr), nil
		},
		OnConnection: func(conn *Conn) {
			t.Fatalf("network failure: got unexpected connection - %v",
				conn.RemoteAddr())
		},
	})
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

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

	assertConnManagerCleanShutdown(t, cmgr)
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
	cmgr := newTestConnManager(t, &Config{
		RetryDuration: maxRetryDuration,
		Dial:          errDialer,
	})
	cmgr.maxRetryDuration = maxRetryDuration
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Establish several connection requests to localhost IPs.
	for i := range targetFailed {
		addr := mustParseAddrPort(fmt.Sprintf("127.0.0.%d:18555", i+1))
		_, err := cmgr.AddPersistent(addr)
		if err != nil {
			t.Fatalf("unexpected add err: %v", err)
		}
	}
	assertConnManagerInternalState(t, cmgr)

	// Wait for the target number of dials and ensure they happen simultaneously
	// by checking it happens before the retry timeout.
	select {
	case <-hitTargetFailed:
	case <-time.After(20 * time.Millisecond):
		t.Fatal("did not reach target number of dials before timeout")
	}
	assertConnManagerInternalState(t, cmgr)

	// Ensure that the connection manager still responds to requests while the
	// failed connections are still retrying.
	disconnected := make(chan struct{})
	go func() {
		const badID = ^uint64(0)
		cmgr.Disconnect(badID)
		close(disconnected)
	}()
	select {
	case <-disconnected:
	case <-time.After(20 * time.Millisecond):
		t.Fatal("timeout servicing connmgr requests")
	}
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
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
	cmgr := newTestConnManager(t, &Config{
		RetryDuration: retryTimeout,
		Dial:          waitDialer,
	})
	cmgr.maxRetryDuration = retryTimeout
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Add a persistent connection.
	addr := mustParseAddrPort("127.0.0.1:18555")
	_, err := cmgr.AddPersistent(addr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertConnManagerInternalState(t, cmgr)

	// Shutdown the connection manager during the retry timeout after a failed
	// dial attempt.
	select {
	case <-dialed:
	case <-time.After(connTestNonReceiveTimeout):
		t.Fatal("timeout waiting for dial")
	}
	time.Sleep(connTestNonReceiveTimeout)
	shutdown()

	// Ensure clean shutdown of connection manager.
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
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
	cmgr := newTestConnManager(t, &Config{
		Dial: indefiniteDialer,
	})
	ctx, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Establish a connection request to a localhost IP.
	addr := mustParseAddrPort("127.0.0.1:18555")
	go cmgr.Connect(ctx, addr)
	assertConnManagerInternalState(t, cmgr)

	// Wait for the connection manager to attempt to dial and ensure the
	// connection is marked as pending while the dialer is blocked.
	select {
	case <-dialed:
	case <-time.After(time.Millisecond * 20):
		t.Fatal("timeout waiting for dial")
	}
	assertPendingAddr(t, cmgr, addr)
	assertConnManagerInternalState(t, cmgr)

	// Cancel the connection attempt while it's still pending.
	connID, _ := pendingAddrConnID(cmgr, addr)
	if err := cmgr.Remove(connID); err != nil {
		t.Fatalf("unexpected remove err: %v", err)
	}
	assertConnManagerInternalState(t, cmgr)

	// Wait for the dialer to signal the context associated with the dial was
	// canceled and ensure the internal pending state is removed.
	select {
	case <-canceled:
	case <-time.After(time.Millisecond * 20):
		t.Fatal("timeout waiting for cancel")
	}
	if _, ok := pendingAddrConnID(cmgr, addr); ok {
		t.Fatalf("connection %s is still pending", addr)
	}
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
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
	cmgr := newTestConnManager(t, &Config{
		Dial:          failingDialer,
		RetryDuration: retryTimeout,
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
	})
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Establish a persistent connection to a localhost IP.
	addr := mustParseAddrPort("127.0.0.1:18555")
	connID, err := cmgr.AddPersistent(addr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertConnManagerInternalState(t, cmgr)

	// Wait for the retry and ensure the connection is pending.
	select {
	case <-retried:
	case <-time.After(20 * time.Millisecond):
		t.Fatalf("did not get retry before timeout")
	}
	assertPendingAddr(t, cmgr, addr)
	assertConnManagerInternalState(t, cmgr)

	// Remove the connection and then immediately allow the next connection to
	// succeed.
	if err := cmgr.Remove(connID); err != nil {
		t.Fatalf("unexpected remove err: %v", err)
	}
	close(connect)

	// Finally, the connection manager should not signal the OnConnection
	// callback, since the request was explicitly canceled.  Give a generous
	// timeout window to ensure the connection manager's backoff is allowed to
	// properly elapse.
	assertNoConnReceivedTimeout(t, connected, 5*retryTimeout)
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
}

// TestDialTimeout ensure [Config.Timeout] works as intended by creating a
// dialer that blocks for three times the configured dial timeout before
// connecting and ensuring the connection fails as expected.
func TestDialTimeout(t *testing.T) {
	t.Parallel()

	// Create a connection manager instance with a dialer that blocks for three
	// times the configured dial timeout before connecting.
	const dialTimeout = time.Millisecond * 20
	cancelled := make(chan struct{})
	timeoutDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		select {
		case <-time.After(dialTimeout * 3):
		case <-ctx.Done():
			close(cancelled)
			return nil, ctx.Err()
		}

		return mockDialer(ctx, network, addr)
	}
	cmgr := newTestConnManager(t, &Config{
		Dial:        timeoutDialer,
		DialTimeout: dialTimeout,
	})
	ctx, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Establish a connection to a localhost IP.
	addr := mustParseAddrPort("127.0.0.1:18555")
	go cmgr.Connect(ctx, addr)
	assertConnManagerInternalState(t, cmgr)

	// Wait to receive the signal that the dialer context was cancelled, which
	// means the dial timeout was hit.
	select {
	case <-cancelled:
	case <-time.After(dialTimeout * 10):
		t.Fatal("timeout waiting for dial cancellation")
	}
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
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
	cmgr := newTestConnManager(t, &Config{
		Dial: indefiniteDialer,
	})
	ctx, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Establish a connection request to a localhost IP with a separate context
	// that can be canceled.
	addr := mustParseAddrPort("127.0.0.1:18555")
	connectCtx, cancelConnect := context.WithCancel(ctx)
	connectErr := make(chan error, 1)
	go func() {
		_, err := cmgr.Connect(connectCtx, addr)
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
	assertPendingAddr(t, cmgr, addr)
	assertConnManagerInternalState(t, cmgr)

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
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
}

// mockListener implements the net.Listener interface and is used to test
// code that deals with net.Listeners without having to actually make any real
// connections.
type mockListener struct {
	localAddr   string
	provideConn chan net.Conn
}

// Accept returns a mock connection when it receives a signal via the Connect
// function.
//
// This is part of the net.Listener interface.
func (m *mockListener) Accept() (net.Conn, error) {
	for conn := range m.provideConn {
		return conn, nil
	}
	return nil, errors.New("network connection closed")
}

// Close closes the mock listener which will cause any blocked Accept
// operations to be unblocked and return errors.
//
// This is part of the net.Listener interface.
func (m *mockListener) Close() error {
	close(m.provideConn)
	return nil
}

// Addr returns the address the mock listener was configured with.
//
// This is part of the net.Listener interface.
func (m *mockListener) Addr() net.Addr {
	return &mockAddr{"tcp", m.localAddr}
}

// Connect fakes a connection to the mock listener from the provided remote
// address.  It will cause the Accept function to return a mock connection
// configured with the provided remote address and the local address for the
// mock listener.
func (m *mockListener) Connect(addr net.Addr) {
	m.provideConn <- &mockConn{
		laddr: m.localAddr,
		lnet:  "tcp",
		rAddr: addr,
	}
}

// newMockListener returns a new mock listener for the provided local address
// and port.  No ports are actually opened.
func newMockListener(localAddr string) *mockListener {
	return &mockListener{
		localAddr:   localAddr,
		provideConn: make(chan net.Conn),
	}
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
	cmgr := newTestConnManager(t, &Config{
		Listeners: listeners,
		OnAccept: func(conn *Conn) {
			receivedConns <- conn
		},
		Dial: mockDialer,
	})
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

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
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
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
	listener := newMockListener("127.0.0.1:18109")
	connected := make(chan *Conn)
	disconnected := make(chan *Conn)
	dialed := make(chan struct{})
	pending := make(chan struct{})
	pendingDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		closeDialedOnce.Do(func() { close(dialed) })
		<-pending
		return mockDialer(ctx, network, addr)
	}
	cmgr := newTestConnManager(t, &Config{
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
	ctx, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Dial a manual connection and wait for it to become pending.
	addr := mustParseAddrPort("127.0.0.1:18555")
	go cmgr.Connect(ctx, addr)
	select {
	case <-dialed:
	case <-time.After(time.Millisecond * 5):
		t.Fatal("did not receive pending dial before timeout")
	}
	assertPendingAddr(t, cmgr, addr)
	assertConnManagerInternalState(t, cmgr)

	// Duplicate connect to the pending address should be rejected.
	if _, err := cmgr.Connect(ctx, addr); !errors.Is(err, ErrAlreadyPending) {
		t.Fatalf("did not reject duplicate pending connection, err: %v", err)
	}

	// Inbound attempts from the pending outbound address should be rejected.
	go listener.Connect(addr)
	assertNoConnReceived(t, inboundConns)
	assertConnManagerInternalState(t, cmgr)

	// Allow the pending connection to complete.
	close(pending)
	conn := assertConnReceived(t, connected, 0, ConnTypeManual)
	assertConnManagerInternalState(t, cmgr)

	// Duplicate connect to the established address should be rejected.
	if _, err := cmgr.Connect(ctx, addr); !errors.Is(err, ErrAlreadyConnected) {
		t.Fatalf("did not reject duplicate active connection, err: %v", err)
	}
	assertConnManagerInternalState(t, cmgr)

	// Inbound attempts from the established outbound address should be
	// rejected.
	go listener.Connect(addr)
	assertNoConnReceived(t, inboundConns)
	assertConnManagerInternalState(t, cmgr)

	// Close the connection and wait for the disconnect.
	conn.Close()
	assertConnReceived(t, disconnected, conn.ID(), ConnTypeManual)
	assertConnManagerInternalState(t, cmgr)

	// Add a persistent connection back to the same address and wait for it to
	// connect since there are no longer any connections to the address.
	connID, err := cmgr.AddPersistent(addr)
	if err != nil {
		t.Fatalf("failed to add persistent connection: %v", err)
	}
	assertConnReceived(t, connected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cmgr)

	// Duplicate persistent connection attempts should be rejected.
	_, err = cmgr.AddPersistent(addr)
	if !errors.Is(err, ErrDuplicatePersistent) {
		t.Fatalf("did not reject duplicate persistent connection, err: %v", err)
	}
	assertConnManagerInternalState(t, cmgr)

	// Manual connection attempts to persistent connection should be rejected.
	_, err = cmgr.Connect(ctx, addr)
	if !errors.Is(err, ErrDuplicatePersistent) {
		t.Fatalf("did not reject manual connection to persistent, err: %v", err)
	}
	assertConnManagerInternalState(t, cmgr)

	// Inbound atempts from the persistent address should be rejected.
	go listener.Connect(addr)
	assertNoConnReceived(t, inboundConns)
	assertConnManagerInternalState(t, cmgr)

	// Remove the persistent connection, wait for it to disconnect, and ensure
	// it is actually removed.
	if err := cmgr.Remove(connID); err != nil {
		t.Fatalf("failed to remove persistent connection: %v", err)
	}
	assertConnReceived(t, disconnected, connID, ConnTypeManual)
	assertRemovedPersistent(t, cmgr, addr)
	assertConnManagerInternalState(t, cmgr)

	// Inbound connections from the same address should now succeed.
	go listener.Connect(addr)
	assertConnReceived(t, inboundConns, 0, ConnTypeInbound)
	assertConnManagerInternalState(t, cmgr)

	// Manual connection attempts to the inbound address should be rejected.
	if _, err := cmgr.Connect(ctx, addr); !errors.Is(err, ErrAlreadyConnected) {
		t.Fatalf("did not reject outbound for existing inbound conn, err: %v",
			err)
	}
	assertConnManagerInternalState(t, cmgr)

	// Attempts to add a persistent connection to an existing inbound should be
	// rejected.
	_, err = cmgr.AddPersistent(addr)
	if !errors.Is(err, ErrAlreadyConnected) {
		t.Fatalf("did not reject persistent conn for existing inbound conn: %v",
			err)
	}
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
}

// TestMaxNormalConns ensures the connection manager limits the total number of
// normal connections to [Config.MaxNormalConns] including automatic outbound,
// manual outbound, and inbound connections.  It also ensures that it is not
// applied to persistent connections.
func TestMaxNormalConns(t *testing.T) {
	t.Parallel()

	// nextAddr is a convenience func to return a new unique address with every
	// invocation.
	var numAddrs atomic.Uint32
	nextAddr := func() *addrmgr.NetAddress {
		addrStr := fmt.Sprintf("10.0.0.%d:18555", numAddrs.Add(1))
		return mustParseAddrPort(addrStr)
	}

	// Create an address that will be whitelisted.
	whitelistedIP := netip.MustParseAddr("220.0.0.1")
	whitelistedAddr := mustParseAddrPort(whitelistedIP.String() + ":1024")

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
	listener := newMockListener("127.0.0.1:9108")
	var pauseTargetOutbound atomic.Bool
	var totalPausedAddrs atomic.Uint32
	hitMaxFailedAttempts := make(chan struct{})
	cmgr := newTestConnManager(t, &Config{
		Listeners:      []net.Listener{listener},
		MaxNormalConns: maxNormalConns,
		TargetOutbound: targetOutbound,
		RetryDuration:  50 * time.Millisecond,
		Dial:           mockDialer,
		Whitelists:     []netip.Prefix{netip.PrefixFrom(whitelistedIP, 32)},
		OnAccept: func(conn *Conn) {
			inboundConns <- conn
		},
		GetNewAddress: func() (net.Addr, error) {
			if pauseTargetOutbound.Load() {
				total := totalPausedAddrs.Add(1)
				if total == maxFailedAttempts {
					hitMaxFailedAttempts <- struct{}{}
				}
				return nil, errors.New("network down")
			}
			return nextAddr(), nil
		},
		OnConnection: func(conn *Conn) {
			connected <- conn
		},
		OnDisconnection: func(conn *Conn) {
			disconnected <- conn
		},
	})
	cmgr.maxRetryDuration = cmgr.cfg.RetryDuration
	ctx, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Wait for the expected number of target outbound conns to be established.
	outbounds := make([]*Conn, 0, targetOutbound)
	for len(outbounds) < targetOutbound {
		conn := assertConnReceived(t, connected, 0, ConnTypeOutbound)
		outbounds = append(outbounds, conn)
	}
	assertConnManagerInternalState(t, cmgr)

	// Establish target number of inbounds to the listener and wait for them to
	// be established.
	go func() {
		for range targetInbound {
			listener.Connect(nextAddr())
		}
	}()
	inbounds := make([]*Conn, 0, targetInbound)
	for len(inbounds) < targetInbound {
		conn := assertConnReceived(t, inboundConns, 0, ConnTypeInbound)
		inbounds = append(inbounds, conn)
	}
	assertConnManagerInternalState(t, cmgr)

	// Establish target number of manual connections and wait for them to be
	// established.
	go func() {
		for range targetManual {
			go cmgr.Connect(ctx, nextAddr())
		}
	}()
	manualConns := make([]*Conn, 0, targetManual+1)
	for len(manualConns) < targetManual {
		conn := assertConnReceived(t, connected, 0, ConnTypeManual)
		manualConns = append(manualConns, conn)
	}
	assertConnManagerInternalState(t, cmgr)

	// Ensure manual connections that would exceed the max allowed normal
	// connections are rejected.
	_, err := cmgr.Connect(ctx, nextAddr())
	if !errors.Is(err, ErrMaxNormalConns) {
		t.Fatalf("did not reject manual connection at max allowed, err: %v", err)
	}
	assertConnManagerInternalState(t, cmgr)

	// Ensure inbound connections that would exceed the max allowed normal
	// connections are rejected.
	go listener.Connect(nextAddr())
	assertNoConnReceived(t, inboundConns)
	assertConnManagerInternalState(t, cmgr)

	// Ensure inbound connections from whitelisted addresses are exempt from the
	// max allowed normal connections by establishing one while at the limit.
	go listener.Connect(whitelistedAddr)
	conn := assertConnReceived(t, inboundConns, 0, ConnTypeInbound)
	assertConnManagerInternalState(t, cmgr)

	// Ensure closing the whitelisted connection does not release a permit it
	// never acquired by asserting inbound connections that would exceed the max
	// allowed normal connections are still rejected after the close.
	conn.Close()
	assertConnReceived(t, disconnected, conn.ID(), ConnTypeInbound)
	go listener.Connect(nextAddr())
	assertNoConnReceived(t, inboundConns)
	assertConnManagerInternalState(t, cmgr)

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
	assertConnManagerInternalState(t, cmgr)

	// Establish another manual connection to take the place of the target
	// outbound connection that was just closed and wait for it to be
	// established.
	go cmgr.Connect(ctx, nextAddr())
	assertConnReceived(t, connected, 0, ConnTypeManual)
	assertConnManagerInternalState(t, cmgr)

	// Unpause the target outbound dials and ensure no additional automatic
	// outbound connections are made despite being under the target outbound due
	// to max total conns.
	pauseTargetOutbound.Store(false)
	assertNoConnReceivedTimeout(t, connected, connTestNonReceiveTimeout+
		cmgr.cfg.RetryDuration)
	assertConnManagerInternalState(t, cmgr)

	// Ensure persistent connections are not subject to the max total normal
	// connections by adding one and waiting for it to be established.
	connID, err := cmgr.AddPersistent(nextAddr())
	if err != nil {
		t.Fatalf("failed to add persistent connection: %v", err)
	}
	assertConnReceived(t, connected, connID, ConnTypeManual)
	assertConnManagerInternalState(t, cmgr)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
	assertConnManagerCleanShutdown(t, cmgr)
}
