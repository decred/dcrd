// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/addrmgr/v4"
)

const (
	// MaxPersistent is the maximum number of persistent connections that can be
	// added.  Persistent connections do not count towards the automatic
	// outbound connection limits.
	MaxPersistent = 8
)

const (
	// maxFailedAttempts is the maximum number of successive failed connection
	// attempts after which network failure is assumed and new connections will
	// be delayed by the configured retry duration.
	maxFailedAttempts = 25

	// defaultRetryDuration is the default duration of time for retrying
	// persistent connections.
	defaultRetryDuration = time.Second * 5

	// defaultMaxRetryDuration is the default maximum duration a persistent
	// connection retry backoff is allowed to grow to.  This is necessary since
	// the retry logic uses a backoff mechanism which increases the interval
	// base times the number of retries that have been done.
	defaultMaxRetryDuration = time.Minute * 5

	// defaultTargetOutbound is the default number of outbound connections to
	// maintain.
	defaultTargetOutbound = 8
)

// ConnectionType specifies the different types of supported connections.
type ConnectionType uint8

const (
	// ConnTypeInbound indicates the connection was established by a remote
	// peer.  No further details are known about this connection until a
	// handshake takes place.
	ConnTypeInbound ConnectionType = iota

	// ConnTypeOutbound indicates a normal outbound connection that was
	// established with no additional restrictions imposed on the type of
	// information that the local peer/server is willing to relay.
	//
	// Note that this in no way implies further restrictions may not be
	// negotiated depending on the protocol messages exchanged between the two
	// peers.
	ConnTypeOutbound

	// ConnTypeManual indicates an outbound connection that was manually
	// requested via [ConnManager.Connect] or [ConnManager.AddPersistent].  In
	// practice, this connection type is the result of requesting manual
	// connections via an RPC method (e.g. "node connect") or via command line
	// configuration options (e.g. --addpeer and --connect).
	ConnTypeManual

	// numConnTypes is the number of connection types.  This entry MUST be the
	// last entry in the enum.
	numConnTypes
)

// connTypeStrings is a map of connection types to human-readable names for
// pretty printing.
var connTypeStrings = map[ConnectionType]string{
	ConnTypeInbound:  "inbound",
	ConnTypeOutbound: "outbound",
	ConnTypeManual:   "manual",
}

// String returns the [ConnectionType] in human-readable form.
func (connType ConnectionType) String() string {
	if s, ok := connTypeStrings[connType]; ok {
		return s
	}

	return fmt.Sprintf("Unknown ConnectionType (%d)", uint8(connType))
}

// Conn houses information about a managed connection.  It is the callers
// responsibility to always ensure [Conn.Close] is called when the connection
// is no longer required.
type Conn struct {
	// The following variables are set at the time the instance is created and
	// are safe for concurrent access.
	//
	// net.Conn is the underlying connection.  It is embedded which makes all of
	// its methods immediately available.
	//
	// id is the unique identifier for this connection.
	//
	// connType specifies the connection type.
	//
	// remoteAddr is the remote address associated with the connection.  It is
	// a concrete address manager address.
	//
	// onClose is a callback that will be invoked when the connection is closed.
	net.Conn
	id         uint64
	connType   ConnectionType
	remoteAddr addrmgr.NetAddress
	onClose    func()

	// closed houses whether or not the connection has already been closed.
	closed atomic.Bool
}

// newConn returns a new connection given an underlying [net.Conn], connection
// ID, and connection type.
//
// The returned connection is automatically configured to invoke the provided on
// close handler followed by the [Config.OnDisconnection] that was configured
// when initially creating the connection manager when the connection is closed.
// The on close handler is invoked in the same goroutine as the caller of
// [Conn.Close] and [Config.OnDisconnection] is invoked in a separate goroutine.
func newConn(cm *ConnManager, conn net.Conn, id uint64, connType ConnectionType, remoteAddr *addrmgr.NetAddress, onClose func()) *Conn {
	c := &Conn{Conn: conn, id: id, connType: connType, remoteAddr: *remoteAddr}
	c.onClose = func() {
		onClose()
		if cm.cfg.OnDisconnection != nil {
			go cm.cfg.OnDisconnection(c)
		}
	}
	return c
}

// ID returns a unique identifier for the connection.
//
// This function is safe for concurrent access.
func (c *Conn) ID() uint64 {
	return c.id
}

// Close closes the connection.  The [Config.OnDisconnection] that was
// configured when initially creating the connection manager will be invoked in
// a separate goroutine prior to closing the underlying connection.
//
// Repeated close attempts are ignored.  Closing a connection that has already
// been closed will not return an error.
//
// This function is safe for concurrent access.
func (c *Conn) Close() error {
	// Already closed.
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}

	// Invoke close callback associated with the connection when it's closed.
	if c.onClose != nil {
		c.onClose()
	}

	// Close the underlying connection.
	return c.Conn.Close()
}

// RemoteAddr returns the remote address manager network address associated with
// the connection.  It returns a [net.Addr] to implement the [net.Conn]
// interface, but the underlying type will be a [*addrmgr.NetAddress].
func (c *Conn) RemoteAddr() net.Addr {
	return &c.remoteAddr
}

// Type returns the [ConnectionType] of the connection.
//
// This function is safe for concurrent access.
func (c *Conn) Type() ConnectionType {
	return c.connType
}

// pendingConnInfo houses information about a pending connection attempt.
type pendingConnInfo struct {
	id     uint64
	addr   *addrmgr.NetAddress
	cancel context.CancelFunc
}

// persistentEntry houses information about a persistent connection that has
// been added to the connection manager.  Once an ID has been assigned, all
// future connections established for the persistent connection will have the
// same ID.  This allows it to be uniquely identified and removed later.
type persistentEntry struct {
	id   uint64
	addr *addrmgr.NetAddress

	// cancel shuts down the goroutine that maintains the persistent connection.
	// It is owned by the connection manager and must not be accessed without
	// its connection mutex held.
	cancel context.CancelFunc
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
	OnAccept func(*Conn)

	// TargetOutbound is the number of outbound network connections to maintain
	// automatically.  Defaults to 8.
	//
	// Persistent connections do not count against this value.  They have their
	// own maximum limit defined by [MaxPersistent].
	TargetOutbound uint32

	// RetryDuration is the duration to wait before retrying connection
	// requests. Defaults to 5s.
	RetryDuration time.Duration

	// OnConnection is a callback that is fired when a new outbound
	// connection is established.
	OnConnection func(*Conn)

	// OnDisconnection is a callback that is fired when a connection is closed.
	OnDisconnection func(*Conn)

	// GetNewAddress is a way to get an address to make a network connection
	// to.  If nil, no new connections will be made automatically.
	GetNewAddress func() (net.Addr, error)

	// Dial connects to the address on the named network.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)

	// DialTimeout specifies the amount of time to wait for a connection to
	// complete before giving up.
	DialTimeout time.Duration
}

// ConnManager provides a manager to handle network connections.
type ConnManager struct {
	// nextConnID is used to assign unique connection request IDs.
	nextConnID atomic.Uint64

	// quit is used for lifecycle management of the connection manager.
	quit chan struct{}

	// cfg specifies the configuration of the connection manager and is set at
	// creating time and treated as immutable after that.
	cfg Config

	// maxRetryDuration is the maximum duration a persistent connection retry
	// backoff is allowed to grow to.
	maxRetryDuration time.Duration

	// runPersistentChan is used to signal the persistent connections handler to
	// launch a goroutine that attempts to always maintain an established
	// connection with a given address.
	//
	// It is a buffered channel with size [MaxPersistent].
	runPersistentChan chan *persistentEntry

	// outboundSem limits the number of active outbound connections.  It does
	// not apply to persistent connections which are separately limited to
	// [MaxPersistent].
	activeOutboundsSem semaphore

	// The fields below this point are all protected by the connection mutex.
	connMtx sync.Mutex

	// persistent tracks all registered persistent connection entries.
	//
	// A persistent connection can be in one of three states:
	//
	// - Established with the connection instance in the active map
	// - Pending with an entry in the pending map
	// - Awaiting a retry
	//
	// Regardless of the state, there will always be an entry in this map.
	persistent map[uint64]*persistentEntry

	// pending holds all registered connection attempts that have yet to
	// succeed.
	pending map[uint64]*pendingConnInfo

	// active represents the set of all active connections.
	active map[uint64]*Conn

	// connIDByAddr provides fast O(1) lookup of connection IDs by address
	// (host:port).  It is kept in sync with the persistent, pending, and active
	// maps and is primarily used to efficiently reject duplicate connections.
	connIDByAddr map[string]uint64
}

// checkShutdown returns [ErrShutdown] when the connection manager quit channel
// has been closed.
func (cm *ConnManager) checkShutdown() error {
	select {
	case <-cm.quit:
		const str = "connection manager shutdown"
		return MakeError(ErrShutdown, str)
	default:
	}
	return nil
}

// stdlibNetAddrToAddrMgrNetAddr converts the provided standard lib [net.Addr]
// to a concrete address manager address.
func stdlibNetAddrToAddrMgrNetAddr(addr net.Addr) (*addrmgr.NetAddress, error) {
	host, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		str := fmt.Sprintf("unable to split address %q", addr)
		return nil, MakeError(ErrUnsupportedAddr, str)
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		str := fmt.Sprintf("invalid port for address %q", addr)
		return nil, MakeError(ErrUnsupportedAddr, str)
	}

	addrType, addrBytes := addrmgr.EncodeHost(host)
	if addrType == addrmgr.UnknownAddressType {
		str := fmt.Sprintf("unable to determine address type for %q", addr)
		return nil, MakeError(ErrUnsupportedAddr, str)
	}

	now := time.Unix(time.Now().Unix(), 0)
	netAddr, err := addrmgr.NewNetAddressFromParams(addrType, addrBytes,
		uint16(port), now, 0)
	if err != nil {
		return nil, MakeError(ErrUnsupportedAddr, err.Error())
	}
	return netAddr, nil
}

// addPendingInfo adds information about a pending connection attempt to the
// local state.
//
// This function MUST be called with the connection mutex held (writes).
func (cm *ConnManager) addPendingInfo(info *pendingConnInfo) {
	cm.pending[info.id] = info
	if _, ok := cm.persistent[info.id]; !ok {
		cm.connIDByAddr[info.addr.String()] = info.id
	}
}

// removePendingInfo removes a pending connection attempt from the local state.
//
// This function MUST be called with the connection mutex held (writes).
func (cm *ConnManager) removePendingInfo(info *pendingConnInfo) {
	delete(cm.pending, info.id)
	if _, ok := cm.persistent[info.id]; !ok {
		delete(cm.connIDByAddr, info.addr.String())
	}
}

// addActiveConn adds an established connection to the local state.
//
// This function MUST be called with the connection mutex held (writes).
func (cm *ConnManager) addActiveConn(conn *Conn) {
	cm.active[conn.id] = conn
	if _, ok := cm.persistent[conn.id]; !ok {
		cm.connIDByAddr[conn.remoteAddr.String()] = conn.id
	}
}

// removeActiveConn removes an established connection from the local state.  It
// has no effect if the connection has already been removed from the active map.
//
// This function MUST be called with the connection mutex held (writes).
func (cm *ConnManager) removeActiveConn(conn *Conn) {
	// The active connection might have already been removed before releasing
	// the mutex to call [Conn.Close].
	if _, ok := cm.active[conn.id]; !ok {
		return
	}

	delete(cm.active, conn.id)
	if _, ok := cm.persistent[conn.id]; !ok {
		delete(cm.connIDByAddr, conn.remoteAddr.String())
	}
}

// addPersistentEntry adds a persistent connection entry to the local state.
//
// This function MUST be called with the connection mutex held (writes).
func (cm *ConnManager) addPersistentEntry(entry *persistentEntry) {
	cm.persistent[entry.id] = entry
	cm.connIDByAddr[entry.addr.String()] = entry.id
}

// removePersistentEntry removes a persistent connection entry from the local
// state.
//
// This function MUST be called with the connection mutex held (writes).
func (cm *ConnManager) removePersistentEntry(entry *persistentEntry) {
	delete(cm.persistent, entry.id)
	_, pending := cm.pending[entry.id]
	_, active := cm.active[entry.id]
	if !pending && !active {
		delete(cm.connIDByAddr, entry.addr.String())
	}
}

// rejectConnectedAddr returns an error if there is already either an
// established connection to the provided address or a pending attempt to
// connect to it.  Persistent connections in the retry state are intentionally
// not detected.
//
// This function MUST be called with the connection mutex held (reads).
func (cm *ConnManager) rejectConnectedAddr(addr *addrmgr.NetAddress) error {
	connID, ok := cm.connIDByAddr[addr.String()]
	if !ok {
		return nil
	}

	if _, ok := cm.pending[connID]; ok {
		str := fmt.Sprintf("a pending connection to %s already exists", addr)
		return MakeError(ErrAlreadyPending, str)
	}
	if _, ok := cm.active[connID]; ok {
		str := fmt.Sprintf("a connection to %s is already established", addr)
		return MakeError(ErrAlreadyConnected, str)
	}
	return nil
}

// findPersistentAddrID attempts to find and return the persistent connection ID
// associated with the passed address.  The bool return indicates whether or not
// it was found.
//
// This function MUST be called with the connection mutex held (reads).
func (cm *ConnManager) findPersistentAddrID(addr net.Addr) (uint64, bool) {
	connID, ok := cm.connIDByAddr[addr.String()]
	if !ok {
		return 0, false
	}

	entry, ok := cm.persistent[connID]
	if !ok {
		return 0, false
	}

	return entry.id, true
}

// rejectPersistentAddr returns an error if there is already a persistent
// connection entry for the given address.
//
// This function MUST be called with the connection mutex held (reads).
func (cm *ConnManager) rejectPersistentAddr(addr *addrmgr.NetAddress) error {
	if _, ok := cm.findPersistentAddrID(addr); ok {
		str := fmt.Sprintf("a persistent connection for %s already exists", addr)
		return MakeError(ErrDuplicatePersistent, str)
	}
	return nil
}

// rejectDuplicateAddr returns an error if there is already a persistent
// connection entry, a pending connection attempt, or an established connection
// for the given address.
//
// This function MUST be called with the connection mutex held (reads).
func (cm *ConnManager) rejectDuplicateAddr(addr *addrmgr.NetAddress) error {
	if err := cm.rejectPersistentAddr(addr); err != nil {
		return err
	}
	if err := cm.rejectConnectedAddr(addr); err != nil {
		return err
	}
	return nil
}

// dial attempts to connect to the provided address and returns a connection
// configured with the provided params on success.
//
// A new globally unique connection ID is assigned unless one is provided by
// passing a non-nil value in the persistent connection ID parameter.  This
// allows persistent connections to retain the same ID across reconnects.
//
// Attempts to dial addresses that are already connected, pending, or (in most
// cases) persistent will return an error as described below.  Only established
// and pending connections are rejected when a non-nil persistent connection ID
// is passed.
//
// On success, the returned connection is configured to remove itself from the
// set of all active connections and invoke the provided on close callback (if
// set) when it is closed.
//
// On failure, the provided on close callback (when non-nil) will be invoked
// prior to returning.
//
// In addition to errors returned by [Config.Dial], the following errors are
// possible:
//
//   - [ErrDuplicatePersistent] when a persistent connection already exists for
//     the address and no persistent connection ID is provided
//   - [ErrAlreadyPending] when there is already a pending connection attempt
//     to the address
//   - [ErrAlreadyConnected] when there is already an established connection to
//     the address
//   - [ErrShutdown] when the connection manager is shutting down
//   - [context.Canceled] or [context.DeadlineExceeded] depending on the
//     provided context or when the dialer fails to establish a connection
//     before the timeout configured for the connection manager
//
// This function is safe for concurrent access.
func (cm *ConnManager) dial(ctx context.Context, addr net.Addr, connType ConnectionType, onClose func(), persistentConnID *uint64) (*Conn, error) {
	var skipOnClose bool
	defer func() {
		if !skipOnClose && onClose != nil {
			onClose()
		}
	}()

	// Ignore during shutdown and when caller provided context is already
	// canceled.
	if err := cm.checkShutdown(); err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	rAddr, err := stdlibNetAddrToAddrMgrNetAddr(addr)
	if err != nil {
		return nil, err
	}

	// Reject attempts to dial addresses that are already connected (or in the
	// process of it).  Additionally, reject attempts to dial existing
	// persistent addresses unless a persistent connection ID was provided
	// indicating the dial is specifically for a persistent connection.
	//
	// This needs to be done under the same lock as adding a pending entry to
	// prevent the possibility of two simultaneous attempts logic racing.
	rejectFn := cm.rejectDuplicateAddr
	if persistentConnID != nil {
		rejectFn = cm.rejectConnectedAddr
	}
	cm.connMtx.Lock()
	if err := rejectFn(rAddr); err != nil {
		cm.connMtx.Unlock()
		log.Debugf("Rejected connection: %v", err)
		return nil, err
	}

	// Apply a dial timeout if requested.  Otherwise, use a regular cancel
	// context to support canceling the pending connection later.
	var cancel context.CancelFunc
	if cm.cfg.DialTimeout != 0 {
		ctx, cancel = context.WithTimeout(ctx, cm.cfg.DialTimeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	// Register the pending connection attempt and defer its removal to ensure
	// it is always removed on failure.
	var connID uint64
	if persistentConnID != nil {
		connID = *persistentConnID
	} else {
		connID = cm.nextConnID.Add(1)
	}
	info := &pendingConnInfo{connID, rAddr, cancel}
	cm.addPendingInfo(info)
	cm.connMtx.Unlock()
	defer func() {
		cm.connMtx.Lock()
		if _, ok := cm.pending[connID]; ok {
			cm.removePendingInfo(info)
		}
		cm.connMtx.Unlock()
	}()

	log.Debugf("Attempting to connect to %v (id: %d, type: %v)", addr, connID,
		connType)

	// Attempt to establish the connection to the address.
	netConn, err := cm.cfg.Dial(ctx, addr.Network(), addr.String())
	if err != nil {
		var logErrStr string
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			logErrStr = fmt.Sprintf("no response for %v", cm.cfg.DialTimeout)
		case errors.Is(err, context.Canceled):
			// Override the error with the shutdown error instead when that is
			// the upstream cause of the context cancel.
			if sErr := cm.checkShutdown(); sErr != nil {
				err = sErr
				break
			}
			logErrStr = "attempt manually canceled"
		}
		if logErrStr == "" {
			logErrStr = err.Error()
		}
		log.Debugf("Failed to connect to %v: %v", addr, logErrStr)
		return nil, err
	}

	// Ignore any connections that succeed after they were manually canceled.
	cm.connMtx.Lock()
	if _, ok := cm.pending[connID]; !ok {
		cm.connMtx.Unlock()
		netConn.Close()
		log.Debugf("Ignoring canceled connection %v (id: %d, type: %v)", addr,
			connID, connType)
		return nil, context.Canceled
	}

	// Remove the pending entry under the lock.  This ensures the maps are
	// mutually exclusive for a given id.
	cm.removePendingInfo(info)

	// Successful return means the on close callback is not invoked until the
	// connection is closed.
	skipOnClose = true

	// Setup a close callback to remove the connection from the map that tracks
	// all active connections when the connection is closed and also to invoke
	// the close callback provided by the caller when specified.
	var conn *Conn
	dialOnClose := func() {
		cm.connMtx.Lock()
		cm.removeActiveConn(conn)
		cm.connMtx.Unlock()
		if onClose != nil {
			onClose()
		}
		log.Debugf("Disconnected from %v (id: %d, type: %v)", addr, connID,
			connType)
	}

	// Create a new connection instance with the connection ID and type and add
	// an entry to the map that tracks all active connections.
	conn = newConn(cm, netConn, connID, connType, rAddr, dialOnClose)
	cm.addActiveConn(conn)
	cm.connMtx.Unlock()

	log.Debugf("Connected to %v (id: %d, type: %v)", addr, connID, connType)
	return conn, nil
}

// Connect assigns an ID and dials a connection to the provided address using
// the provided context and the dial function configured when initially creating
// the connection manager.
//
// Attempts to dial addresses that already have an established, pending, or
// persistent connection will return an error as described below.
//
// The connection will have type [ConnTypeManual].
//
// Note that the context parameter to this function and the lifecycle context
// may be independent.
//
// In addition to errors returned by the underlying dialer, the following errors
// are possible:
//
//   - [ErrDuplicatePersistent] when a persistent connection already exists for
//     the address (regardless of its current state)
//   - [ErrAlreadyPending] when there is already a pending connection attempt
//     to the address
//   - [ErrAlreadyConnected] when there is already an established connection to
//     the address
//   - [ErrShutdown] when the connection manager is shutting down
//   - [context.Canceled] or [context.DeadlineExceeded] depending on the
//     provided context or when the dialer fails to establish a connection
//     before the timeout configured for the connection manager
//
// This function is safe for concurrent access.
func (cm *ConnManager) Connect(ctx context.Context, addr net.Addr) (*Conn, error) {
	conn, err := cm.dial(ctx, addr, ConnTypeManual, nil, nil)
	if err != nil {
		return nil, err
	}
	if cm.cfg.OnConnection != nil {
		go cm.cfg.OnConnection(conn)
	}
	return conn, nil
}

// Disconnect either disconnects the connection corresponding to the given
// connection id or cancels any pending attempts associated with it.  Persistent
// connections will be retried with an increasing backoff duration.
//
// This function is safe for concurrent access.
func (cm *ConnManager) Disconnect(id uint64) error {
	// Cancel and remove pending entries.  Even though the pending entry will be
	// removed from the map regardless by the dialer, doing it now ensures that
	// any connections that are already in progress and later succeed are
	// ignored.
	cm.connMtx.Lock()
	if info, ok := cm.pending[id]; ok {
		info.cancel()
		cm.removePendingInfo(info)
		cm.connMtx.Unlock()
		return nil
	}

	conn := cm.active[id]
	if conn != nil {
		cm.connMtx.Unlock()
		conn.Close() // Close requires the conn mutex.
		return nil
	}
	_, isPersistent := cm.persistent[id]
	cm.connMtx.Unlock()

	// Not found in active or pending, but it might still be a persistent conn
	// waiting to retry.  No error in that case.
	if isPersistent {
		return nil
	}

	str := fmt.Sprintf("no entries with id %d exist", id)
	return MakeError(ErrNotFound, str)
}

// Remove closes, cancels, or removes the connection corresponding to the given
// connection id.
//
// This function may be used for all connections states and types, including
// established, pending, and persistent connections.
//
// Connections that are already established are closed and connection attempts
// that are still pending are canceled.  Persistent connections are additionally
// removed so that no future retries will occur.
//
// This function is safe for concurrent access.
func (cm *ConnManager) Remove(id uint64) error {
	// When the ID is for a persistent connection, cancel the associated context
	// and remove it from the persistent map to prevent future retries.
	cm.connMtx.Lock()
	entry, isPersistent := cm.persistent[id]
	if isPersistent {
		cm.removePersistentEntry(entry)
		if entry.cancel != nil {
			entry.cancel()
		}
		log.Debugf("Removed persistent connection to %v (id %d)", entry.addr,
			entry.id)
	}

	// Cancel and remove pending entries.  Even though the pending entry will be
	// removed from the map regardless by the dialer, doing it now ensures that
	// any connections that are already in progress and later succeed are
	// ignored.
	if info, ok := cm.pending[id]; ok {
		info.cancel()
		cm.removePendingInfo(info)
		cm.connMtx.Unlock()
		return nil
	}

	// Close active connections and remove the entry from the active map.
	//
	// Even though the connection close handler would remove it from the map, it
	// needs to be removed under same lock as removals from the persistent map
	// to prevent the possibility of two simultaneous attempts logic racing.
	if conn, ok := cm.active[id]; ok {
		cm.removeActiveConn(conn)
		cm.connMtx.Unlock()
		conn.Close() // Close requires the conn mutex.
		return nil
	}
	cm.connMtx.Unlock()

	// Not found in active or pending, but no error if it was a removed
	// persistent conn.
	if isPersistent {
		return nil
	}

	str := fmt.Sprintf("no entries with id %d exist", id)
	return MakeError(ErrNotFound, str)
}

// inboundStdlibNetAddrToAddrMgrAddr converts the provided standard library
// [net.Addr] that is expected to be from an inbound connection to a concrete
// address manager address.
func inboundStdlibNetAddrToAddrMgrAddr(addr net.Addr) (*addrmgr.NetAddress, error) {
	// Fast path for inbounds since they will almost always be one of these
	// given they are created by [net.Listener.Accept].
	switch a := addr.(type) {
	case *net.TCPAddr:
		return addrmgr.NewNetAddressFromIPPort(a.IP, uint16(a.Port), 0), nil
	case *net.UDPAddr:
		return addrmgr.NewNetAddressFromIPPort(a.IP, uint16(a.Port), 0), nil
	}

	// Fall back to slower string parsing.
	return stdlibNetAddrToAddrMgrNetAddr(addr)
}

// listenHandler accepts incoming connections on a given listener.  It must be
// run as a goroutine.
func (cm *ConnManager) listenHandler(ctx context.Context, listener net.Listener) {
	log.Infof("Server listening on %s", listener.Addr())
	defer log.Tracef("Listener handler done for %s", listener.Addr())

	for ctx.Err() == nil {
		netConn, err := listener.Accept()
		if err != nil {
			// Only log the error if not forcibly shutting down.
			if ctx.Err() == nil {
				log.Errorf("Can't accept connection: %v", err)
			}
			continue
		}

		rAddr, err := inboundStdlibNetAddrToAddrMgrAddr(netConn.RemoteAddr())
		if err != nil {
			log.Warnf("Dropped connection from %v: failed to parse address",
				netConn.RemoteAddr())
			netConn.Close()
			continue
		}

		// Reject connections with the same host:port as any existing pending,
		// established, or persistent connections.  Note that this does NOT
		// prevent multiple connections from the same host given they typically
		// will be coming from different ports.
		//
		// The aforementioned behavior is intentional as it allows connections
		// from the same host to be independently limited to more than one
		// elsewhere.
		cm.connMtx.Lock()
		if err := cm.rejectDuplicateAddr(rAddr); err != nil {
			cm.connMtx.Unlock()
			log.Debugf("Dropped connection from %v: %v", rAddr, err)
			netConn.Close()
			continue
		}
		cm.connMtx.Unlock()

		go func(netConn net.Conn) {
			// Create a new connection instance with the next globally unique
			// connection ID, add an entry to the map that tracks all active
			// connections, and invoke the configured accept callback with it.
			//
			// Also set a close callback to remove the connection from the map
			// when it is closed.
			id := cm.nextConnID.Add(1)
			const connType = ConnTypeInbound
			var conn *Conn
			onClose := func() {
				cm.connMtx.Lock()
				cm.removeActiveConn(conn)
				cm.connMtx.Unlock()
				log.Debugf("Disconnected from %v (id: %d, type: %v)", rAddr, id,
					connType)
			}
			conn = newConn(cm, netConn, id, connType, rAddr, onClose)
			cm.connMtx.Lock()
			cm.addActiveConn(conn)
			cm.connMtx.Unlock()
			log.Debugf("Accepted connection from %v (id: %d, type: %v)", rAddr,
				id, connType)
			cm.cfg.OnAccept(conn)
		}(netConn)
	}
}

// AddPersistent adds an address the connection manager will attempt to always
// maintain an established connection with until the persistent connection entry
// is removed via [ConnManager.Remove] or the context associated with
// [ConnManager.Run] is canceled.
//
// When the associated connection is dropped, it will be retried with an
// increasing backoff, up to a maximum for repeated failed attempts.
//
// A maximum of [MaxPersistent] connections may be added.  Attempting to add any
// more will return [ErrMaxPersistent].
//
// Adding a duplicate persistent address will return [ErrDuplicatePersistent]
// and adding addresses that already have an established or pending connection
// will return [ErrAlreadyConnected] or [ErrAlreadyPending], respectively.
//
// An ID is returned that uniquely identifies the persistent connection.  All
// future connections established will have the same ID.
//
// Persistent connections do not count against [Config.TargetOutbound].
//
// Note that the actual connections to the address happen asynchronously and
// will have type [ConnTypeManual].  Established connections will invoke the
// [Config.OnConnection] callback that was configured when initially creating
// the connection manager.
//
// Since connections happen asynchronously, the error only indicates issues with
// adding the persistent connection entry.
//
// The persistent connection may be removed by passing the returned connection
// ID to [ConnManager.Remove].
//
// This function is safe for concurrent access.
func (cm *ConnManager) AddPersistent(addr net.Addr) (uint64, error) {
	cm.connMtx.Lock()
	defer cm.connMtx.Unlock()

	if len(cm.persistent)+1 > MaxPersistent {
		str := fmt.Sprintf("a maximum of %d persistent connections is allowed",
			MaxPersistent)
		return 0, MakeError(ErrMaxPersistent, str)
	}

	rAddr, err := stdlibNetAddrToAddrMgrNetAddr(addr)
	if err != nil {
		return 0, err
	}

	if err := cm.rejectDuplicateAddr(rAddr); err != nil {
		return 0, err
	}

	entry := &persistentEntry{id: cm.nextConnID.Add(1), addr: rAddr}
	cm.addPersistentEntry(entry)
	log.Debugf("Added persistent connection to %v (id: %d)", addr, entry.id)

	// The channel is buffered with the max allowed persistent conns, so there
	// is no possibility of blocking here.  This approach allows persistent
	// peers to be added both before and after the connection manager is running
	// without starting the goroutines before it is running.
	cm.runPersistentChan <- entry
	return entry.id, nil
}

// IsPersistent returns whether or not the provided connection id belongs to a
// persistent connection.
//
// This function is safe for concurrent access.
func (cm *ConnManager) IsPersistent(id uint64) bool {
	cm.connMtx.Lock()
	_, ok := cm.persistent[id]
	cm.connMtx.Unlock()
	return ok
}

// FindPersistentAddrID attempts to find and return the persistent connection ID
// associated with the passed address.  The bool return indicates whether or not
// it was found.
//
// This function is safe for concurrent access.
func (cm *ConnManager) FindPersistentAddrID(addr net.Addr) (uint64, bool) {
	cm.connMtx.Lock()
	id, ok := cm.findPersistentAddrID(addr)
	cm.connMtx.Unlock()
	return id, ok
}

// runPersistent attempts to maintain a persistent connection to the provided
// address until the passed context is canceled.
//
// When the associated connection is dropped, it will be retried with an
// increasing backoff, up to a maximum for repeated failed attempts.
//
// This MUST be run as a goroutine.
func (cm *ConnManager) runPersistent(ctx context.Context, connID uint64, addr net.Addr) {
	// Ensure the connection is closed when the goroutine exits.
	var conn *Conn
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	// Setup a callback that notifies a disconnect channel for use below and
	// start with the channel signaled.
	disconnected := make(chan struct{}, 1)
	disconnected <- struct{}{}
	onClose := func() {
		disconnected <- struct{}{}
	}

	var retryCount uint32
	var retryAfter <-chan time.Time
	var lastAttempt time.Time
	for {
		// Wait for disconnect or retry timer when it's set.
		select {
		case <-ctx.Done():
			return
		case <-cm.quit:
			return
		case <-retryAfter:
			retryAfter = nil
		case <-disconnected:
			// Wait to retry any time the connection was not maintained for at
			// least a single retry interval.
			//
			// This approach is used over only incrementing the retry count when
			// the dial fails to effectively rate limit the attempts with an
			// increasing backoff regardless of the reason a stable connection
			// was not maintained.
			//
			// For example, the remote might repeatedly reject the peer for a
			// variety of reasons (max limits, not enough peers of a desired
			// type, etc) after a successful connection is made.
			if !lastAttempt.IsZero() && time.Since(lastAttempt) < cm.cfg.RetryDuration {
				// Reconnect after a retry timeout with an increasing backoff up
				// to a max for repeated failed attempts.
				const maxUint32 = 1<<32 - 1
				if retryCount < maxUint32 {
					retryCount++
				}
				retryWait := time.Duration(retryCount) * cm.cfg.RetryDuration
				retryWait = min(retryWait, cm.maxRetryDuration)
				log.Debugf("Retrying connection to %v in %v (retries %d)", addr,
					retryWait, retryCount)
				retryAfter = time.After(retryWait)
				continue
			}

			// A connection succeeded and was maintained for at least a single
			// retry interval.
			//
			// Clear the retry state.
			retryCount = 0
			retryAfter = nil
		}

		lastAttempt = time.Now()
		var err error
		conn, err = cm.dial(ctx, addr, ConnTypeManual, onClose, &connID)
		if err != nil {
			if ctx.Err() != nil {
				return
			}

			// Retry, potentially after a timeout with backoff.
			continue
		}

		// Successful connection.
		if cm.cfg.OnConnection != nil {
			go cm.cfg.OnConnection(conn)
		}
	}
}

// persistentConnsHandler handles launching individual goroutines for persistent
// connections.
func (cm *ConnManager) persistentConnsHandler(ctx context.Context) {
	for {
		select {
		case entry := <-cm.runPersistentChan:
			pCtx, cancel := context.WithCancel(ctx)
			cm.connMtx.Lock()
			entry.cancel = cancel
			cm.connMtx.Unlock()
			go cm.runPersistent(pCtx, entry.id, entry.addr)

		case <-ctx.Done():
			return
		}
	}
}

// targetOutboundHandler attempts to automatically maintain the target number of
// outbound connections configured via [Config.TargetOutbound] when initially
// creating the connection manager.
//
// This MUST be run as a goroutine.
func (cm *ConnManager) targetOutboundHandler(ctx context.Context) {
	log.Trace("Starting target outbound handler")
	defer log.Trace("Target outbound handler done")

	// failedAttempts tracks the total number of failed outbound connection
	// attempts since the last successful connection.  It is primarily used to
	// detect network outages in order to impose a retry timeout on achieving
	// the target number of outbound connections which prevents runaway failed
	// connection attempt churn.
	//
	// Overflow is not checked since it would be virtually impossible to hit
	// anywhere max uint64 in practice and even if it ever happened, the only
	// consequence would potentially be a few extra retries before it hit the
	// max failures again.
	var failedAttempts atomic.Uint64

	for ctx.Err() == nil {
		// Pause automatic outbound connections for a retry timeout after too
		// many failed connection attempts.  The network very likely has become
		// temporarily unreachable.
		if failedAttempts.Load() >= maxFailedAttempts {
			log.Debugf("Max failed connection attempts reached [%d] -- "+
				"pausing connections for %v", maxFailedAttempts,
				cm.cfg.RetryDuration)

			select {
			case <-time.After(cm.cfg.RetryDuration):
			case <-cm.quit:
				return
			case <-ctx.Done():
				return
			}
		}

		// Wait for a permit to make another outbound connection.
		if !cm.activeOutboundsSem.Acquire(ctx) {
			return
		}

		addr, err := cm.cfg.GetNewAddress()
		if err != nil {
			failedAttempts.Add(1)
			log.Debugf("Failed to get address for outbound connection: %v", err)
			cm.activeOutboundsSem.Release()
			continue
		}

		go func(addr net.Addr) {
			onClose := cm.activeOutboundsSem.Release
			conn, err := cm.dial(ctx, addr, ConnTypeOutbound, onClose, nil)
			if err != nil {
				failedAttempts.Add(1)
				return
			}

			failedAttempts.Store(0)
			if cm.cfg.OnConnection != nil {
				go cm.cfg.OnConnection(conn)
			}
		}(addr)
	}
}

// Run starts the connection manager along with its configured listeners and
// begins connecting to the network.  It blocks until the provided context is
// canceled.
func (cm *ConnManager) Run(ctx context.Context) {
	log.Trace("Starting connection manager")
	defer log.Trace("Connection manager stopped")

	// Start all the listeners so long as the caller requested them and provided
	// a callback to be invoked when connections are accepted.
	var wg sync.WaitGroup
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

	// Start persistent connections handler which starts individual goroutines
	// for each persistent connection already added and any newly added ones
	// later.
	wg.Add(1)
	go func() {
		cm.persistentConnsHandler(ctx)
		wg.Done()
	}()

	// Start outbound connection handler to maintain the target number of
	// normal outbound connections when not in manual connect mode.
	if cm.cfg.GetNewAddress != nil {
		wg.Add(1)
		go func() {
			cm.targetOutboundHandler(ctx)
			wg.Done()
		}()
	}

	// Shutdown the connection manager when the context is canceled.
	<-ctx.Done()
	close(cm.quit)

	// Stop all the listeners.  There will not be any listeners if listening is
	// disabled.
	for _, listener := range listeners {
		// Ignore the error since this is shutdown and there is no way
		// to recover anyways.
		_ = listener.Close()
	}

	// Shutdown persistent conns, cancel pending conns, and close active conns.
	cm.connMtx.Lock()
	totalIDs := len(cm.persistent) + len(cm.pending) + len(cm.active)
	ids := make(map[uint64]struct{}, totalIDs)
	for id := range cm.persistent {
		ids[id] = struct{}{}
	}
	for id := range cm.pending {
		ids[id] = struct{}{}
	}
	for id := range cm.active {
		ids[id] = struct{}{}
	}
	cm.connMtx.Unlock()
	for id := range ids {
		cm.Remove(id)
	}

	wg.Wait()
}

// New returns a new connection manager with the provided configuration.
//
// Use Run to start listening and/or connecting to the network.
func New(cfg *Config) (*ConnManager, error) {
	if cfg.Dial == nil {
		return nil, MakeError(ErrDialNil, "dial cannot be nil")
	}
	// Default to sane values
	if cfg.RetryDuration <= 0 {
		cfg.RetryDuration = defaultRetryDuration
	}
	if cfg.TargetOutbound == 0 {
		cfg.TargetOutbound = defaultTargetOutbound
	}
	cm := ConnManager{
		cfg:                *cfg, // Copy so caller can't mutate
		quit:               make(chan struct{}),
		maxRetryDuration:   defaultMaxRetryDuration,
		runPersistentChan:  make(chan *persistentEntry, MaxPersistent),
		activeOutboundsSem: makeSemaphore(cfg.TargetOutbound),
		persistent:         make(map[uint64]*persistentEntry, MaxPersistent),
		pending:            make(map[uint64]*pendingConnInfo),
		active:             make(map[uint64]*Conn, cfg.TargetOutbound),
		connIDByAddr:       make(map[string]uint64),
	}
	return &cm, nil
}
