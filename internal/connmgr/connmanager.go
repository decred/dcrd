// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (

	// maxRetryDuration is the max duration of time retrying of a persistent
	// connection is allowed to grow to.  This is necessary since the retry
	// logic uses a backoff mechanism which increases the interval base times
	// the number of retries that have been done.
	maxRetryDuration = time.Minute * 5
)

const (
	// maxFailedAttempts is the maximum number of successive failed connection
	// attempts after which network failure is assumed and new connections will
	// be delayed by the configured retry duration.
	maxFailedAttempts = 25

	// defaultRetryDuration is the default duration of time for retrying
	// persistent connections.
	defaultRetryDuration = time.Second * 5

	// defaultTargetOutbound is the default number of outbound connections to
	// maintain.
	defaultTargetOutbound = uint32(8)
)

// ConnState represents the state of the requested connection.
type ConnState uint32

// ConnState can be either pending, established, disconnected or failed.  When
// a new connection is requested, it is attempted and categorized as
// established or failed depending on the connection result.  An established
// connection which was disconnected is categorized as disconnected.
const (
	ConnPending ConnState = iota
	ConnEstablished
	ConnDisconnected
	ConnFailed
	ConnCanceled
)

// ConnReq is the connection request to a network address. If permanent, the
// connection will be retried on disconnection.
type ConnReq struct {
	// id is the unique identifier for this connection request.
	id atomic.Uint64

	// state is the current connection state for this connection request.
	state atomic.Uint32

	// The following fields are owned by the connection manager and must not
	// be accessed without its connection mutex held.
	//
	// retryCount is the number of times a permanent connection request that
	// fails to connect has been retried since the last successful connection.
	//
	// conn is the underlying network connection.  It will be nil before a
	// connection has been established.
	retryCount uint32
	conn       net.Conn

	// Addr is the address to connect to.
	Addr net.Addr

	// Permanent specifies whether or not the connection request represents what
	// should be treated as a permanent connection, meaning the connection
	// manager will try to always maintain the connection including retries with
	// increasing backoff timeouts.
	Permanent bool
}

// updateState updates the state of the connection request.
func (c *ConnReq) updateState(state ConnState) {
	c.state.Store(uint32(state))
}

// ID returns a unique identifier for the connection request.
func (c *ConnReq) ID() uint64 {
	return c.id.Load()
}

// State is the connection state of the requested connection.
func (c *ConnReq) State() ConnState {
	return ConnState(c.state.Load())
}

// String returns a human-readable string for the connection request.
func (c *ConnReq) String() string {
	if c.Addr == nil || c.Addr.String() == "" {
		return fmt.Sprintf("reqid %d", c.ID())
	}
	return fmt.Sprintf("%s (reqid %d)", c.Addr, c.ID())
}

// Config holds the configuration options related to the connection manager.
type Config struct {
	// Listeners defines a slice of listeners for which the connection
	// manager will take ownership of and accept connections.  When a
	// connection is accepted, the OnAccept handler will be invoked with the
	// connection.  Since the connection manager takes ownership of these
	// listeners, they will be closed when the connection manager is
	// stopped.
	//
	// This field will not have any effect if the OnAccept field is not
	// also specified.  It may be nil if the caller does not wish to listen
	// for incoming connections.
	Listeners []net.Listener

	// OnAccept is a callback that is fired when an inbound connection is
	// accepted.  It is the caller's responsibility to close the connection.
	// Failure to close the connection will result in the connection manager
	// believing the connection is still active and thus have undesirable
	// side effects such as still counting toward maximum connection limits.
	//
	// This field will not have any effect if the Listeners field is not
	// also specified since there couldn't possibly be any accepted
	// connections in that case.
	OnAccept func(net.Conn)

	// TargetOutbound is the number of outbound network connections to
	// maintain. Defaults to 8.
	TargetOutbound uint32

	// RetryDuration is the duration to wait before retrying connection
	// requests. Defaults to 5s.
	RetryDuration time.Duration

	// OnConnection is a callback that is fired when a new outbound
	// connection is established.
	OnConnection func(*ConnReq, net.Conn)

	// OnDisconnection is a callback that is fired when an outbound
	// connection is disconnected.
	OnDisconnection func(*ConnReq)

	// GetNewAddress is a way to get an address to make a network connection
	// to.  If nil, no new connections will be made automatically.
	GetNewAddress func() (net.Addr, error)

	// Dial connects to the address on the named network. Either Dial or
	// DialAddr need to be specified (but not both).
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)

	// DialAddr is an alternative to Dial which receives a full net.Addr instead
	// of just the protocol family and address. Either DialAddr or Dial need
	// to be specified (but not both).
	DialAddr func(context.Context, net.Addr) (net.Conn, error)

	// Timeout specifies the amount of time to wait for a connection
	// to complete before giving up.
	Timeout time.Duration
}

// handleDisconnected is used to remove a connection.
type handleDisconnected struct {
	id    uint64
	retry bool
}

// ConnManager provides a manager to handle network connections.
type ConnManager struct {
	// connReqCount is the number of connection requests that have been made and
	// is primarily used to assign unique connection request IDs.
	connReqCount atomic.Uint64

	// assignIDMtx synchronizes the assignment of an ID to a connection request
	// with overall connection request count above.
	assignIDMtx sync.Mutex

	// quit is used for lifecycle management of the connection manager.
	quit chan struct{}

	// cfg specifies the configuration of the connection manager and is set at
	// creating time and treated as immutable after that.
	cfg Config

	// failedAttempts tracks the total number of failed oubound connection
	// attempts since the last successful connection made by the connection
	// manager.  It is primarily used to detect network outages in order to
	// impose a retry timeout on achieving the target number of outbound
	// connections which prevents runaway failed connection attempt churn.
	//
	// This field is owned by the connection handler and must not be accessed
	// outside of it.
	failedAttempts uint64

	// requests is used internally to interact with the connection handler
	// goroutine.
	requests chan interface{}

	// The following fields are used to track the various connections managed
	// by the connection manager.  They are protected by the associated
	// connection mutex.
	//
	// pending holds all registered connection requests that have yet to
	// succeed.
	//
	// conns represents the set of all active connections.
	connMtx sync.RWMutex
	pending map[uint64]*ConnReq
	conns   map[uint64]*ConnReq
}

// connHandler handles all connection related requests.  It must be run as a
// goroutine.
//
// The connection handler makes sure that we maintain a pool of active outbound
// connections so that we remain connected to the network.  Connection requests
// are processed and mapped by their assigned ids.
func (cm *ConnManager) connHandler(ctx context.Context) {
out:
	for {
		select {
		case req := <-cm.requests:
			switch msg := req.(type) {
			case handleDisconnected:
				cm.connMtx.Lock()
				connReq, ok := cm.conns[msg.id]
				if !ok {
					connReq, ok = cm.pending[msg.id]
					if !ok {
						log.Errorf("Unknown connid=%d",
							msg.id)
						continue
					}

					// Pending connection was found, remove
					// it from pending map if we should
					// ignore a later, successful
					// connection.
					connReq.updateState(ConnCanceled)
					log.Debugf("Canceling: %v", connReq)
					delete(cm.pending, msg.id)
					cm.connMtx.Unlock()
					continue
				}

				// An existing connection was located, mark as
				// disconnected and execute disconnection
				// callback.
				log.Debugf("Disconnected from %v", connReq)
				delete(cm.conns, msg.id)

				if connReq.conn != nil {
					connReq.conn.Close()
				}

				if cm.cfg.OnDisconnection != nil {
					go cm.cfg.OnDisconnection(connReq)
				}

				// All internal state has been cleaned up, if
				// this connection is being removed, we will
				// make no further attempts with this request.
				if !msg.retry {
					connReq.updateState(ConnDisconnected)
					cm.connMtx.Unlock()
					continue
				}

				// Otherwise, attempt a reconnection when there are not already
				// enough outbound peers to satisfy the target number of
				// outbound peers or this is a persistent peer.
				numConns := uint32(len(cm.conns))
				if numConns < cm.cfg.TargetOutbound || connReq.Permanent {
					// The connection request is reused for persistent peers, so
					// add it back to the pending map in that case so that
					// subsequent processing of connections and failures do not
					// ignore the request.
					if connReq.Permanent {
						connReq.updateState(ConnPending)
						log.Debugf("Reconnecting to %v", connReq)
						cm.pending[msg.id] = connReq
					}

					cm.handleFailedConn(ctx, connReq)
				}
				cm.connMtx.Unlock()
			}

		case <-ctx.Done():
			break out
		}
	}

	log.Trace("Connection handler done")
}

// registerPending registers the provided connection request as a pending
// connection attempt.
//
// This function MUST be called with the connection mutex lock held (writes).
func (cm *ConnManager) registerPending(connReq *ConnReq) {
	connReq.updateState(ConnPending)
	cm.pending[connReq.ID()] = connReq
}

// newConnReq creates a new connection request and connects to the corresponding
// address.
func (cm *ConnManager) newConnReq(ctx context.Context) {
	// Ignore during shutdown.
	if ctx.Err() != nil {
		return
	}

	c := &ConnReq{}
	c.id.Store(cm.connReqCount.Add(1))

	// Register the pending connection attempt so it can be canceled via the
	// [ConnManager.Remove] method.
	cm.connMtx.Lock()
	cm.registerPending(c)
	cm.connMtx.Unlock()

	addr, err := cm.cfg.GetNewAddress()
	if err != nil {
		cm.connMtx.Lock()
		cm.handleFailedPending(ctx, c, err)
		cm.connMtx.Unlock()
		return
	}

	c.Addr = addr

	cm.Connect(ctx, c)
}

// handleFailedConn handles a connection failed due to a disconnect or any other
// failure.  Permanent connection requests are retried after the configured
// retry duration.  A new connection request is created if required.
//
// In the event there have been [maxFailedAttempts] failed successive attempts,
// new connections will be retried after the configured retry duration.
//
// This function MUST be called with the connection lock held (writes).
func (cm *ConnManager) handleFailedConn(ctx context.Context, c *ConnReq) {
	// Ignore during shutdown.
	select {
	case <-cm.quit:
		return
	case <-ctx.Done():
		return
	default:
	}

	// Reconnect to permanent connection requests after a retry timeout with
	// an increasing backoff up to a max for repeated failed attempts.
	if c.Permanent {
		c.retryCount++
		retryWait := time.Duration(c.retryCount) * cm.cfg.RetryDuration
		retryWait = min(retryWait, maxRetryDuration)
		log.Debugf("Retrying connection to %v in %v", c, retryWait)
		go func() {
			select {
			case <-time.After(retryWait):
				cm.Connect(ctx, c)
			case <-cm.quit:
			case <-ctx.Done():
			}
		}()
		return
	}

	// Nothing more to do when the method to automatically get new addresses
	// to connect to isn't configured.
	if cm.cfg.GetNewAddress == nil {
		return
	}

	// Wait to attempt new connections when there are too many successive
	// failures.  This prevents massive connection spam when no connections can
	// be made, such as a network outtage.
	cm.failedAttempts++
	if cm.failedAttempts >= maxFailedAttempts {
		log.Debugf("Max failed connection attempts reached: [%d] -- retrying "+
			"connection in: %v", maxFailedAttempts, cm.cfg.RetryDuration)
		go func() {
			select {
			case <-time.After(cm.cfg.RetryDuration):
				cm.newConnReq(ctx)
			case <-cm.quit:
			case <-ctx.Done():
			}
		}()
		return
	}

	// Otherwise, attempt a new connection with a new address now.
	go cm.newConnReq(ctx)
}

// handleFailedPending handles failed pending connection requests.  Connection
// requests that were canceled are ignored.  Otherwise, their state is updated
// to mark it failed and it passed along to [ConnManager.handlFailedConn] to
// possibly retry or be reused for a new connection depending on settings.
//
// This function MUST be called with the connection lock held (writes).
func (cm *ConnManager) handleFailedPending(ctx context.Context, c *ConnReq, failedErr error) {
	if _, ok := cm.pending[c.ID()]; !ok {
		log.Debugf("Ignoring connection for canceled conn req: %v", c)
		return
	}

	c.updateState(ConnFailed)
	log.Debugf("Failed to connect to %v: %v", c, failedErr)
	cm.handleFailedConn(ctx, c)
}

// Connect assigns an id and dials a connection to the address of the connection
// request using the provided context and the dial function configured when
// initially creating the connection manager.
//
// The connection attempt will be ignored if the connection manager has been
// shutdown by canceling the lifecycle context the Run method was invoked with
// or the provided connection request is already in the failed state.
//
// Note that the context parameter to this function and the lifecycle context
// may be independent.
func (cm *ConnManager) Connect(ctx context.Context, c *ConnReq) {
	// Ignore during shutdown and when caller provided context is already
	// canceled.
	select {
	case <-cm.quit:
		return
	default:
	}
	if ctx.Err() != nil {
		return
	}

	// During the time we wait for retry there is a chance that this
	// connection was already cancelled.
	if c.State() == ConnCanceled {
		log.Debugf("Ignoring connect for canceled connreq=%v", c)
		return
	}

	// Assign an ID and register the pending connection attempt when an ID has
	// not already been assigned so it can be canceled via the
	// [ConnManager.Remove] method.
	//
	// Note that the assignment of the ID and the overall request count need to
	// be synchronized.  So long as this is the only place an existing conn
	// request ID is updated and this method is not called concurrently on the
	// same conn request, no race could occur.  However, those preconditions
	// would be easy to inadvertently violate via updates to the code, so the
	// mutex is added here for additional safety.
	var doRegisterPending bool
	cm.assignIDMtx.Lock()
	if c.ID() == 0 {
		c.id.Store(cm.connReqCount.Add(1))
		doRegisterPending = true
	}
	connReqID := c.ID()
	cm.assignIDMtx.Unlock()
	if doRegisterPending {
		cm.connMtx.Lock()
		cm.registerPending(c)
		cm.connMtx.Unlock()
	}

	log.Debugf("Attempting to connect to %v", c)

	// Attempt to establish the connection to the address associated with the
	// connection request.  Apply a timeout if requested.
	if cm.cfg.Timeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cm.cfg.Timeout)
		defer cancel()
	}
	var conn net.Conn
	var err error
	if cm.cfg.Dial != nil {
		conn, err = cm.cfg.Dial(ctx, c.Addr.Network(), c.Addr.String())
	} else {
		conn, err = cm.cfg.DialAddr(ctx, c.Addr)
	}
	if err != nil {
		cm.connMtx.Lock()
		cm.handleFailedPending(ctx, c, err)
		cm.connMtx.Unlock()
		return
	}

	cm.connMtx.Lock()
	defer cm.connMtx.Unlock()

	if _, ok := cm.pending[connReqID]; !ok {
		conn.Close()
		log.Debugf("Ignoring connection for canceled connreq=%v", c)
		return
	}

	c.updateState(ConnEstablished)
	c.conn = conn
	cm.conns[connReqID] = c
	log.Debugf("Connected to %v", c)
	c.retryCount = 0
	cm.failedAttempts = 0
	delete(cm.pending, connReqID)

	if cm.cfg.OnConnection != nil {
		go cm.cfg.OnConnection(c, conn)
	}
}

// Disconnect disconnects the connection corresponding to the given connection
// id. If permanent, the connection will be retried with an increasing backoff
// duration.
func (cm *ConnManager) Disconnect(id uint64) {
	select {
	case cm.requests <- handleDisconnected{id, true}:
	case <-cm.quit:
	}
}

// Remove removes the connection corresponding to the given connection id from
// known connections.
//
// NOTE: This method can also be used to cancel a lingering connection attempt
// that hasn't yet succeeded.
func (cm *ConnManager) Remove(id uint64) {
	select {
	case cm.requests <- handleDisconnected{id, false}:
	case <-cm.quit:
	}
}

// findPendingByAddr attempts to find and return the pending connection request
// associated with the provided address.  It returns nil if no matching request
// is found.
//
// This function MUST be called with the connection mutex held (writes).
func (cm *ConnManager) findPendingByAddr(addr net.Addr) *ConnReq {
	pendingAddr := addr.String()
	for _, req := range cm.pending {
		if req == nil || req.Addr == nil {
			continue
		}
		if pendingAddr == req.Addr.String() {
			return req
		}
	}
	return nil
}

// CancelPending removes the connection corresponding to the given address
// from the list of pending failed connections.
//
// Returns an error if the connection manager is stopped or there is no pending
// connection for the given address.
func (cm *ConnManager) CancelPending(addr net.Addr) error {
	cm.connMtx.Lock()
	defer cm.connMtx.Unlock()

	connReq := cm.findPendingByAddr(addr)
	if connReq == nil {
		str := fmt.Sprintf("no pending connection to %v", addr)
		return MakeError(ErrNotFound, str)
	}

	delete(cm.pending, connReq.ID())
	connReq.updateState(ConnCanceled)
	log.Debugf("Canceled pending connection to %v", addr)
	return nil
}

// ForEachConnReq calls the provided function with each connection request known
// to the connection manager, including pending requests.  Returning an error
// from the provided function will stop the iteration early and return said
// error from this function.
//
// This function is safe for concurrent access.
//
// NOTE: This must not call any other connection manager methods during
// iteration or it will result in a deadlock.
func (cm *ConnManager) ForEachConnReq(f func(c *ConnReq) error) error {
	cm.connMtx.Lock()
	defer cm.connMtx.Unlock()

	var err error
	for _, connReq := range cm.pending {
		err = f(connReq)
		if err != nil {
			return err
		}
	}
	for _, connReq := range cm.conns {
		err = f(connReq)
		if err != nil {
			return err
		}
	}
	return nil
}

// listenHandler accepts incoming connections on a given listener.  It must be
// run as a goroutine.
func (cm *ConnManager) listenHandler(ctx context.Context, listener net.Listener) {
	log.Infof("Server listening on %s", listener.Addr())
	for ctx.Err() == nil {
		conn, err := listener.Accept()
		if err != nil {
			// Only log the error if not forcibly shutting down.
			if ctx.Err() == nil {
				log.Errorf("Can't accept connection: %v", err)
			}
			continue
		}
		go cm.cfg.OnAccept(conn)
	}

	log.Tracef("Listener handler done for %s", listener.Addr())
}

// Run starts the connection manager along with its configured listeners and
// begins connecting to the network.  It blocks until the provided context is
// cancelled.
func (cm *ConnManager) Run(ctx context.Context) {
	log.Trace("Starting connection manager")

	// Start the connection handler goroutine.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		cm.connHandler(ctx)
		wg.Done()
	}()

	// Start all the listeners so long as the caller requested them and provided
	// a callback to be invoked when connections are accepted.
	var listeners []net.Listener
	if cm.cfg.OnAccept != nil {
		listeners = cm.cfg.Listeners
	}
	for _, listener := range cm.cfg.Listeners {
		wg.Add(1)
		go func(listener net.Listener) {
			cm.listenHandler(ctx, listener)
			wg.Done()
		}(listener)
	}

	// Start enough outbound connections to reach the target number when not
	// in manual connect mode.
	if cm.cfg.GetNewAddress != nil {
		curConnReqCount := cm.connReqCount.Load()
		for i := curConnReqCount; i < uint64(cm.cfg.TargetOutbound); i++ {
			go cm.newConnReq(ctx)
		}
	}

	// Stop all the listeners and shutdown the connection manager when the
	// context is cancelled.  There will not be any listeners if listening is
	// disabled.
	<-ctx.Done()
	close(cm.quit)
	for _, listener := range listeners {
		// Ignore the error since this is shutdown and there is no way
		// to recover anyways.
		_ = listener.Close()
	}
	wg.Wait()
	log.Trace("Connection manager stopped")
}

// New returns a new connection manager with the provided configuration.
//
// Use Run to start listening and/or connecting to the network.
func New(cfg *Config) (*ConnManager, error) {
	if cfg.Dial == nil && cfg.DialAddr == nil {
		return nil, MakeError(ErrDialNil, "dial cannot be nil")
	}
	if cfg.Dial != nil && cfg.DialAddr != nil {
		return nil, MakeError(ErrBothDialsFilled,
			"cannot specify both Dial and DialAddr")
	}
	// Default to sane values
	if cfg.RetryDuration <= 0 {
		cfg.RetryDuration = defaultRetryDuration
	}
	if cfg.TargetOutbound == 0 {
		cfg.TargetOutbound = defaultTargetOutbound
	}
	cm := ConnManager{
		cfg:      *cfg, // Copy so caller can't mutate
		requests: make(chan interface{}),
		quit:     make(chan struct{}),
		pending:  make(map[uint64]*ConnReq),
		conns:    make(map[uint64]*ConnReq, cfg.TargetOutbound),
	}
	return &cm, nil
}
