// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2019-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func init() {
	// Override the max retry duration when running tests.
	maxRetryDuration = 2 * time.Millisecond
}

// runConnMgrAsync invokes the Run method on the passed connection manager in a
// separate goroutine and returns a cancelable context and wait group the caller
// can use to shutdown the the connection manager and wait for clean shutdown.
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

// mockAddr mocks a network address
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
	return c, nil
}

// mockDialer mocks the net.Dial interface by returning a mock connection to
// the given address.
func mockDialerAddr(ctx context.Context, addr net.Addr) (net.Conn, error) {
	r, w := io.Pipe()
	c := &mockConn{rAddr: addr}
	c.Reader = r
	c.Writer = w
	return c, nil
}

// TestNewConfig tests that new ConnManager config is validated as expected.
func TestNewConfig(t *testing.T) {
	_, err := New(&Config{})
	if err == nil {
		t.Fatal("New expected error: 'Dial can't be nil', got nil")
	}
	_, err = New(&Config{
		Dial: mockDialer,
	})
	if err != nil {
		t.Fatalf("New unexpected error: %v", err)
	}

	_, err = New(&Config{
		Dial:     mockDialer,
		DialAddr: mockDialerAddr,
	})
	if err == nil {
		t.Fatal("New expected error: 'Dial and DialAddr can't be both nil', got nil")
	}

	_, err = New(&Config{
		DialAddr: mockDialerAddr,
	})
	if err != nil {
		t.Fatalf("New unexpected error: %v", err)
	}
}

// assertConnReqID ensures the provided connection request has the given ID.
func assertConnReqID(t *testing.T, connReq *ConnReq, wantID uint64) {
	t.Helper()

	gotID := connReq.ID()
	if gotID != wantID {
		t.Fatalf("unexpected ID -- got %v, want %v", gotID, wantID)
	}
}

// assertConnReqState ensures the provided connection request has the given
// state.
func assertConnReqState(t *testing.T, connReq *ConnReq, wantState ConnState) {
	t.Helper()

	gotState := connReq.State()
	if gotState != wantState {
		t.Fatalf("unexpected state -- got %v, want %v", gotState, wantState)
	}
}

// TestConnectMode tests that the connection manager works in the connect mode.
//
// In connect mode, automatic connections are disabled, so we test that
// requests using Connect are handled and that no other connections are made.
func TestConnectMode(t *testing.T) {
	connected := make(chan *ConnReq)
	cmgr, err := New(&Config{
		TargetOutbound: 2,
		Dial:           mockDialer,
		OnConnection: func(c *ConnReq, conn net.Conn) {
			connected <- c
		},
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	ctx, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	cr := &ConnReq{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 18555,
		},
		Permanent: true,
	}
	go cmgr.Connect(ctx, cr)

	// Ensure that the connection was received.
	select {
	case gotConnReq := <-connected:
		assertConnReqID(t, gotConnReq, cr.ID())
		assertConnReqState(t, cr, ConnEstablished)

	case <-time.After(time.Millisecond * 5):
		t.Fatalf("connect mode: connection timeout - %v", cr.Addr)
	}

	// Ensure only a single connection was made.
	select {
	case c := <-connected:
		t.Fatalf("connect mode: got unexpected connection - %v", c.Addr)
	case <-time.After(time.Millisecond * 5):
	}

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
}

// TestTargetOutbound tests the target number of outbound connections
// configuration option by waiting until all connections are established and
// ensuring they are the only connections made.
func TestTargetOutbound(t *testing.T) {
	targetOutbound := uint32(10)
	connected := make(chan *ConnReq)
	cmgr, err := New(&Config{
		TargetOutbound: targetOutbound,
		Dial:           mockDialer,
		GetNewAddress: func() (net.Addr, error) {
			return &net.TCPAddr{
				IP:   net.ParseIP("127.0.0.1"),
				Port: 18555,
			}, nil
		},
		OnConnection: func(c *ConnReq, conn net.Conn) {
			connected <- c
		},
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Wait for the expected number of target outbound conns to be established.
	for i := uint32(0); i < targetOutbound; i++ {
		<-connected
	}

	// Ensure no additional connections are made.
	select {
	case c := <-connected:
		t.Fatalf("target outbound: got unexpected connection - %v", c.Addr)
	case <-time.After(time.Millisecond * 5):
		break
	}

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
}

// TestPassAddrAlongDialAddr tests if when using the DialAddr config option,
// any address object returned by GetNewAddress will be correctly passed along
// to DialAddr to be used for connecting to a host.
func TestPassAddrAlongDialAddr(t *testing.T) {
	connected := make(chan *ConnReq)

	// targetAddr will be the specific address we'll use to connect. It _could_
	// be carrying more info than a standard (tcp/udp) network address, so it
	// needs to be relayed to dialAddr.
	targetAddr := mockAddr{
		net:     "invalid",
		address: "unreachable",
	}

	cmgr, err := New(&Config{
		TargetOutbound: 1,
		DialAddr:       mockDialerAddr,
		GetNewAddress: func() (net.Addr, error) {
			return targetAddr, nil
		},
		OnConnection: func(c *ConnReq, conn net.Conn) {
			connected <- c
		},
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	select {
	case c := <-connected:
		receivedMock, isMockAddr := c.Addr.(mockAddr)
		if !isMockAddr {
			t.Fatal("connected to an address that was not a mockAddr")
		}
		if receivedMock != targetAddr {
			t.Fatal("connected to an address different than the expected target")
		}
	case <-time.After(time.Millisecond * 5):
		t.Fatal("did not get connection to target address before timeout")
	}

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
}

// TestRetryPermanent tests that permanent connection requests are retried.
//
// We make a permanent connection request using Connect, disconnect it using
// Disconnect and we wait for it to be connected back.
func TestRetryPermanent(t *testing.T) {
	connected := make(chan *ConnReq)
	disconnected := make(chan *ConnReq)
	cmgr, err := New(&Config{
		RetryDuration:  time.Millisecond,
		TargetOutbound: 1,
		Dial:           mockDialer,
		OnConnection: func(c *ConnReq, conn net.Conn) {
			connected <- c
		},
		OnDisconnection: func(c *ConnReq) {
			disconnected <- c
		},
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	ctx, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	cr := &ConnReq{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 18555,
		},
		Permanent: true,
	}
	go cmgr.Connect(ctx, cr)
	gotConnReq := <-connected
	assertConnReqID(t, gotConnReq, cr.ID())
	assertConnReqState(t, cr, ConnEstablished)

	cmgr.Disconnect(cr.ID())
	gotConnReq = <-disconnected
	assertConnReqID(t, gotConnReq, cr.ID())
	assertConnReqState(t, cr, ConnPending)

	gotConnReq = <-connected
	assertConnReqID(t, gotConnReq, cr.ID())
	assertConnReqState(t, cr, ConnEstablished)

	cmgr.Remove(cr.ID())
	gotConnReq = <-disconnected
	assertConnReqID(t, gotConnReq, cr.ID())
	assertConnReqState(t, cr, ConnDisconnected)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
}

// TestMaxRetryDuration tests the maximum retry duration.
//
// We have a timed dialer which initially returns err but after RetryDuration
// hits maxRetryDuration returns a mock conn.
func TestMaxRetryDuration(t *testing.T) {
	// This test relies on the current value of the max retry duration defined
	// in the tests, so assert it.
	if maxRetryDuration != 2*time.Millisecond {
		t.Fatalf("max retry duration of %v is not the required value for test",
			maxRetryDuration)
	}

	networkUp := make(chan struct{})
	time.AfterFunc(5*time.Millisecond, func() {
		close(networkUp)
	})
	timedDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		select {
		case <-networkUp:
			return mockDialer(ctx, network, addr)
		default:
			return nil, errors.New("network down")
		}
	}

	connected := make(chan *ConnReq)
	cmgr, err := New(&Config{
		RetryDuration:  time.Millisecond,
		TargetOutbound: 1,
		Dial:           timedDialer,
		OnConnection: func(c *ConnReq, conn net.Conn) {
			connected <- c
		},
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	ctx, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	cr := &ConnReq{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 18555,
		},
		Permanent: true,
	}
	go cmgr.Connect(ctx, cr)
	// retry in 1ms
	// retry in 2ms - max retry duration reached
	// retry in 2ms - timedDialer returns mockDial
	select {
	case <-connected:
	case <-time.After(20 * time.Millisecond):
		t.Fatal("max retry duration: connection timeout")
	}

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
}

// TestNetworkFailure tests that the connection manager handles a network
// failure gracefully.
func TestNetworkFailure(t *testing.T) {
	var closeOnce sync.Once
	const targetOutbound = 5
	const retryTimeout = time.Millisecond * 5
	var dials uint32
	reachedMaxFailedAttempts := make(chan struct{})
	connMgrDone := make(chan struct{})
	errDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		totalDials := atomic.AddUint32(&dials, 1)
		if totalDials >= maxFailedAttempts {
			closeOnce.Do(func() { close(reachedMaxFailedAttempts) })
			<-connMgrDone
		}
		return nil, errors.New("network down")
	}
	cmgr, err := New(&Config{
		TargetOutbound: targetOutbound,
		RetryDuration:  retryTimeout,
		Dial:           errDialer,
		GetNewAddress: func() (net.Addr, error) {
			return &net.TCPAddr{
				IP:   net.ParseIP("127.0.0.1"),
				Port: 18555,
			}, nil
		},
		OnConnection: func(c *ConnReq, conn net.Conn) {
			t.Fatalf("network failure: got unexpected connection - %v", c.Addr)
		},
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Shutdown the connection manager after the max failed attempts is reached
	// and an additional retry duration has passed and then wait for the
	// shutdown to complete.
	<-reachedMaxFailedAttempts
	time.Sleep(retryTimeout)
	shutdown()
	close(connMgrDone)
	wg.Wait()

	// Ensure the number of dial attempts does not exceed the max number of
	// failed attempts plus the number of potential retries during the
	// additional waiting period.
	gotDials := atomic.LoadUint32(&dials)
	wantMaxDials := uint32(maxFailedAttempts + targetOutbound)
	if gotDials > wantMaxDials {
		t.Fatalf("unexpected number of dials - got %v, want <= %v", gotDials,
			wantMaxDials)
	}
}

// TestShutdownFailedConns tests that failed connections are ignored after
// connmgr is shutdown.
//
// We have a dialer which sets the stop flag on the conn manager and returns an
// err so that the handler assumes that the conn manager is stopped and ignores
// the failure.
func TestShutdownFailedConns(t *testing.T) {
	var closeOnce sync.Once
	dialed := make(chan struct{})
	waitDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		closeOnce.Do(func() { close(dialed) })
		return nil, errors.New("network down")
	}
	cmgr, err := New(&Config{
		RetryDuration: maxRetryDuration,
		Dial:          waitDialer,
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	ctx, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Shutdown the connection manager during the retry timeout after a failed
	// dial attempt.
	go func() {
		<-dialed
		time.Sleep(maxRetryDuration / 2)
		shutdown()
	}()

	// Establish a connection request to a localhost IP.
	cr := &ConnReq{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 18555,
		},
		Permanent: true,
	}
	go cmgr.Connect(ctx, cr)

	// Ensure clean shutdown of connection manager.
	wg.Wait()
}

// TestRemovePendingConnection tests that it's possible to cancel a pending
// connection, removing its internal state from the ConnMgr.
func TestRemovePendingConnection(t *testing.T) {
	// Create a ConnMgr instance with an instance of a dialer that'll never
	// succeed.
	dialed := make(chan struct{})
	wait := make(chan struct{})
	indefiniteDialer := func(ctx context.Context, addr net.Addr) (net.Conn, error) {
		close(dialed)
		<-wait
		return nil, errors.New("error")
	}
	cmgr, err := New(&Config{
		DialAddr: indefiniteDialer,
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	ctx, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Establish a connection request to a localhost IP.
	cr := &ConnReq{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 18555,
		},
		Permanent: true,
	}
	go cmgr.Connect(ctx, cr)

	// Wait for the connection manager to attempt to dial the connection request
	// and ensure the connection is marked as pending while the dialer is
	// blocked.
	select {
	case <-dialed:
	case <-time.After(time.Millisecond * 20):
		t.Fatal("timeout waiting for dial")
	}
	assertConnReqState(t, cr, ConnPending)

	// The request launched above will never be able to establish a connection,
	// so cancel it _before_ it's able to be completed.
	cmgr.Remove(cr.ID())

	// Ensure the connection request is now marked as canceled after a short
	// timeout to allow the transition to occur.
	time.Sleep(10 * time.Millisecond)
	assertConnReqState(t, cr, ConnCanceled)

	// Ensure clean shutdown of connection manager.
	close(wait)
	shutdown()
	wg.Wait()
}

// TestCancelIgnoreDelayedConnection tests that a canceled connection request
// will not execute the on connection callback, even if an outstanding retry
// succeeds.
func TestCancelIgnoreDelayedConnection(t *testing.T) {
	const retryTimeout = 10 * time.Millisecond

	// Setup a dialer that will continue to return an error until the
	// connect chan is signaled. The dial attempt immediately after that
	// will succeed in returning a connection.
	connect := make(chan struct{})
	failingDialer := func(ctx context.Context, addr net.Addr) (net.Conn, error) {
		select {
		case <-connect:
			return mockDialerAddr(ctx, addr)
		default:
		}

		return nil, errors.New("error")
	}

	connected := make(chan *ConnReq)
	cmgr, err := New(&Config{
		DialAddr:      failingDialer,
		RetryDuration: retryTimeout,
		OnConnection: func(c *ConnReq, conn net.Conn) {
			connected <- c
		},
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	ctx, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Establish a connection request to a localhost IP.
	cr := &ConnReq{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 18555,
		},
	}
	go cmgr.Connect(ctx, cr)

	// Allow for the first retry timeout to elapse.
	time.Sleep(2 * retryTimeout)

	// Ensure the status of the connection request is marked as failed, even
	// after reattempting to connect.
	assertConnReqState(t, cr, ConnFailed)

	// Remove the connection, and then immediately allow the next connection
	// to succeed.
	cmgr.Remove(cr.ID())
	close(connect)

	// Allow the connection manager to process the removal.
	time.Sleep(5 * time.Millisecond)

	// Ensure the status of the connection request is canceled.
	assertConnReqState(t, cr, ConnCanceled)

	// Finally, the connection manager should not signal the OnConnection
	// callback, since the request was explicitly canceled.  Give a generous
	// timeout window to ensure the connection manager's linear backoff is
	// allowed to properly elapse.
	select {
	case <-connected:
		t.Fatal("on-connect should not be called for canceled req")
	case <-time.After(5 * retryTimeout):
	}

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
}

// TestDialTimeout ensure the Timeout configuration parameter works as intended
// by creating a dialer that blocks for twice the configured dial timeout before
// connecting and ensuring the connection fails as expected.
func TestDialTimeout(t *testing.T) {
	// Create a connection manager instance with a dialer that blocks for twice
	// the configured dial timeout before connecting.
	const dialTimeout = time.Millisecond * 2
	timeoutDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		select {
		case <-time.After(dialTimeout * 2):
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		return mockDialer(ctx, network, addr)
	}
	cmgr, err := New(&Config{
		Dial:    timeoutDialer,
		Timeout: dialTimeout,
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Establish a connection request to a localhost IP.
	cr := &ConnReq{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 18555,
		},
	}
	go cmgr.Connect(context.Background(), cr)

	// Wait for the dial timeout to elapse and ensure the connection request is
	// marked as failed after a short timeout to allow the transition to occur.
	time.Sleep(dialTimeout)
	time.Sleep(10 * time.Millisecond)
	assertConnReqState(t, cr, ConnFailed)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
}

// TestConnectContext ensures the Connect method works as intended when provided
// with a context that times out before a dial attempt succeeds.
func TestConnectContext(t *testing.T) {
	// Create a connection manager instance with a dialer that blocks until its
	// provided context is canceled.
	dialed := make(chan struct{})
	indefiniteDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		close(dialed)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	cmgr, err := New(&Config{
		Dial: indefiniteDialer,
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Establish a connection request to a localhost IP with a separate context
	// that can be canceled.
	cr := &ConnReq{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 18555,
		},
	}
	connectCtx, cancelConnect := context.WithCancel(context.Background())
	go cmgr.Connect(connectCtx, cr)

	// Wait for the connection manager to attempt to dial the connection request
	// and ensure the connection is marked as pending while the dialer is
	// blocked.
	select {
	case <-dialed:
	case <-time.After(time.Millisecond * 20):
		t.Fatal("timeout waiting for dial")
	}
	assertConnReqState(t, cr, ConnPending)

	// Cancel the connection context and ensure the connection request is marked
	// as failed after a short timeout to allow the transition to occur.
	cancelConnect()
	time.Sleep(10 * time.Millisecond)
	assertConnReqState(t, cr, ConnFailed)

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
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
func (m *mockListener) Connect(ip string, port int) {
	m.provideConn <- &mockConn{
		laddr: m.localAddr,
		lnet:  "tcp",
		rAddr: &net.TCPAddr{
			IP:   net.ParseIP(ip),
			Port: port,
		},
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
	// Setup a connection manager with a couple of mock listeners that
	// notify a channel when they receive mock connections.
	receivedConns := make(chan net.Conn)
	listener1 := newMockListener("127.0.0.1:8333")
	listener2 := newMockListener("127.0.0.1:9333")
	listeners := []net.Listener{listener1, listener2}
	cmgr, err := New(&Config{
		Listeners: listeners,
		OnAccept: func(conn net.Conn) {
			receivedConns <- conn
		},
		Dial: mockDialer,
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	_, shutdown, wg := runConnMgrAsync(context.Background(), cmgr)

	// Fake a couple of mock connections to each of the listeners.
	go func() {
		for i, listener := range listeners {
			l := listener.(*mockListener)
			l.Connect("127.0.0.1", 10000+i*2)
			l.Connect("127.0.0.1", 10000+i*2+1)
		}
	}()

	// Tally the receive connections to ensure the expected number are
	// received.  Also, fail the test after a timeout so it will not hang
	// forever should the test not work.
	expectedNumConns := len(listeners) * 2
	var numConns int
out:
	for {
		select {
		case <-receivedConns:
			numConns++
			if numConns == expectedNumConns {
				break out
			}

		case <-time.After(time.Millisecond * 50):
			t.Fatalf("Timeout waiting for %d expected connections",
				expectedNumConns)
		}
	}

	// Ensure clean shutdown of connection manager.
	shutdown()
	wg.Wait()
}
