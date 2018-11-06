// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"errors"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	// ErrNoConnAvailable indicates no Conn is available for pick().
	ErrNoConnAvailable = errors.New("no connection is available")

	// ErrNoUsableConnAvailable indicates no usable connections available i.e.
	// client is marked as Shutdown.
	ErrNoUsableConnAvailable = errors.New("all connections have been shutdown")

	// ErrBalancerNotFound indicates the balancer type is not found.
	ErrBalancerNotFound = errors.New("not able to find the specified balancer type")
)

// connectionState indicates the current state of connection.
type connectionState int

// String would return a string representation of the said connectionState
func (s connectionState) String() string {
	switch s {
	case idle:
		return "IDLE"
	case connecting:
		return "CONNECTING"
	case ready:
		return "READY"
	case shutdown:
		return "SHUTDOWN"
	case disconnected:
		return "DISCONNECTED"
	default:
		return "Invalid-State"
	}
}

const (
	// idle indicates the conn is idle i.e. not yet tried or not yet successfully connected.
	idle connectionState = iota

	// connecting indicates the conn is connecting.
	connecting

	// disconnected indicates the conn marked to be picked by reconnect handler.
	disconnected

	// ready indicates the conn is ready for work.
	ready

	// shutdown indicates the ClientConn has started shutting down.
	shutdown
)

// HostAddress represents a server the client connects to.
type HostAddress struct {
	// Host is the IP address and port of the RPC server you want to connect
	// to.
	Host string

	// Endpoint is the websocket endpoint on the RPC server.  This is
	// typically "ws".
	Endpoint string

	// retryCount holds the number of times the client has tried to
	// reconnect to the RPC server.
	retryCount int64

	// initialConnectAttemptCount holds the number of times the client has tried to
	// establish initial connection to the RPC server.
	initialConnectAttemptCount int
}

type picker struct {
	conns []HostAddress
	mu    sync.Mutex
	next  int
}

// Pick checks for the next usable connection using the connState map with the balancer.
// It changes the connectionstate to Connecting if the Host picked is not yet used.
// This would error out if there are no HostAddresses available for round-robin.
func (p *picker) Pick(balancer Balancer) (conn *HostAddress, err error) {

	if len(p.conns) == 0 {
		log.Infof(ErrNoConnAvailable.Error())
		return conn, ErrNoConnAvailable
	}

	p.mu.Lock()
	startPos := p.next
	var ok bool

	for {
		conn = &p.conns[p.next]
		var state connectionState
		state, ok = balancer.ConnectionState(conn.Host)
		p.next = (p.next + 1) % len(p.conns)
		if !ok || state == ready {
			// This Host is either not yet tried or its ready to use.
			break
		} else if state == idle && conn.initialConnectAttemptCount < 50 {
			// We are yet to establish a successful connection to this host.
			// If need be, max attempt allowed, can be made configurable.
			break
		}
		// This indicates iteration is complete.
		if startPos == p.next {
			conn = nil
			break
		}
	}
	if conn == nil {
		return conn, ErrNoUsableConnAvailable
	}
	if !ok {
		balancer.NotifyConnStateChange(conn, connecting, true)
	}
	p.mu.Unlock()
	return conn, nil
}

// isStatefulNotification checks if the notification method is
// one of those that changes a server state and hence needs special attention.
func isStatefulNotification(method string) bool {
	switch method {
	case "loadtxfilter",
		"notifyblocks",
		"notifywinningtickets",
		"notifyspentandmissedtickets",
		"notifynewtickets",
		"notifystakedifficulty",
		"notifynewtransactions":
		return true
	}
	return false
}

// wsInHandlerForConn represents the incoming message handler for a
// ws connection. This is invoked as a goroutine, one per ws connection
type wsInHandlerForConn func(*websocket.Conn, string)

// roundRobinBalancer represents a round-robin balancer. It implements Balancer
// interface and maintains a Picker for picking Hosts in a round-robin manner.
type roundRobinBalancer struct {
	// connPicker is the Picker to be used to do round-robin.
	connPicker picker

	// connConfig is the connection config used for making connections.
	// HostAddresses present with the connConfig is used by Picker for
	// round-robin.
	connConfig *ConnConfig

	// wsConns is a map of socket connections for the corresponding HostAddress.Host
	// as the key. It holds only the active connections.
	wsConns map[string]*websocket.Conn

	// hostAddMap is a map of HostAddress with key as the HostAddress.Host.
	hostAddMap map[string]*HostAddress

	mu sync.Mutex

	// connState is a map of HostAddress.Host as key and corresponding
	// connection state as the value.
	connState map[string]connectionState

	// connForNotification is a map of notification name as key and value as
	// HostAddress.Host corresponding to the connection used for this notification.
	connForNotification map[string]string

	waitToConnect chan struct{}

	// isReady indicates that at least one connection is in Ready state.
	isReady bool

	// needsClientRestart is used to decide if client.start() needs to be called during reconnect.
	// It is set to true when all connection are either disconnected or shutdown.
	// It is set to false after client.start() is called during reconnect.
	needsClientRestart bool

	// wsInHandler is receiver for incoming calls for a wsConnection.
	// This is invoked as a goroutine per ws connection.
	wsInHandler wsInHandlerForConn
}

// NextConn gets the next connection to be used.
// It returns both *websocket.Conn and the corresponding *HostAddress.
func (rrb *roundRobinBalancer) NextConn(method string) (*websocket.Conn, *HostAddress, error) {
	if !rrb.isReady {
		select {
		// Wait if any handshake is inprogress.
		// This is to avoid cases where all connections are in connecting mode and
		// consecutive calls will end up with no connection as Picker would skip all and err out
		// complaining no connections available.
		case <-rrb.waitToConnect:
		default:
		}
	}
	if host, ok := rrb.connForNotification[method]; ok {
		wsConn, _ := rrb.WsConnection(host)
		log.Infof("Balancer: Using %s for notification: %s",
			host, method)
		return wsConn, rrb.hostAddMap[host], nil
	}
	hostAddress, err := rrb.connPicker.Pick(rrb)
	if err != nil {
		return nil, nil, err
	}
	var wsConn *websocket.Conn
	if !rrb.connConfig.HTTPPostMode {
		var ok bool
		wsConn, ok = rrb.WsConnection(hostAddress.Host)
		for !ok {
			rrb.waitToConnect = make(chan struct{})
			rrb.connConfig.Host = hostAddress.Host
			rrb.connConfig.Endpoint = hostAddress.Endpoint
			wsConn, err = dial(rrb.connConfig)
			if err != nil {
				// Update the connection state to idle so that we try again
				// as we haven't yet connected to this host successfully.
				rrb.NotifyConnStateChange(hostAddress, idle, true)
				attempt := rrb.updateInitialConnectAttempt(hostAddress)
				log.Infof("Balancer: Failed to make initial connection to %s: %v, attempt:%v",
					rrb.connConfig.Host, err, attempt)
				// Try for the next Host.
				hostAddress, err = rrb.connPicker.Pick(rrb)
				if err != nil {
					return nil, nil, err
				}
				wsConn, ok = rrb.WsConnection(hostAddress.Host)
			} else {
				rrb.mu.Lock()
				rrb.wsConns[hostAddress.Host] = wsConn
				// reset as we successfully connected
				hostAddress.initialConnectAttemptCount = 0
				//invoke the lister for this ws connection
				go rrb.wsInHandler(wsConn, hostAddress.Host)
				rrb.mu.Unlock()
				rrb.NotifyConnStateChange(hostAddress, ready, true)
				log.Infof("Balancer: Established connection to RPC server %s",
					hostAddress.Host)
				break
			}
			close(rrb.waitToConnect)
		}
	} else {
		rrb.NotifyConnStateChange(hostAddress, ready, true)
	}
	log.Infof("Balancer: Connection pick for RPC %s",
		hostAddress.Host)
	if isStatefulNotification(method) {
		rrb.connForNotification[method] = hostAddress.Host
		log.Infof("Balancer: Will be using %s for notification: %s",
			hostAddress.Host, method)
	}
	return wsConn, hostAddress, err
}

// NotifyConnStateChange updates connState map for the given Host address.
// It also updates the isReady field to indicate if at least one connection
// with Ready state exists.
func (rrb *roundRobinBalancer) NotifyConnStateChange(hostAdd *HostAddress, state connectionState, sync bool) {
	if sync {
		rrb.mu.Lock()
	}
	if state == shutdown && !rrb.connConfig.HTTPPostMode {
		delete(rrb.wsConns, hostAdd.Host)
	}
	if state == ready {
		rrb.isReady = true
	} else {
		rrb.isReady = false
		for _, state := range rrb.connState {
			if state == ready {
				rrb.isReady = true
				break
			}
		}
	}
	rrb.connState[hostAdd.Host] = state
	log.Infof("Connection state update for host %s: %v", hostAdd.Host, state)
	if sync {
		rrb.mu.Unlock()
	}
}

// Close closes the passed ws connection
func (rrb *roundRobinBalancer) Close(wsConn *websocket.Conn) bool {
	rrb.mu.Lock()

	hostToBeClosed := ""
	for hostEntry, wsConnEntry := range rrb.wsConns {
		if wsConnEntry == wsConn {
			hostToBeClosed = hostEntry
			break
		}
	}

	if rrb.connState[hostToBeClosed] == disconnected || rrb.connState[hostToBeClosed] == shutdown {
		rrb.mu.Unlock()
		return rrb.setForClientRestart()
	}

	log.Tracef("Balancer: Disconnecting current RPC client %s", hostToBeClosed)
	err := wsConn.Close()
	if err != nil {
		log.Errorf("Failed disconnecting to %s: %v", hostToBeClosed, err)
	}

	rrb.NotifyConnStateChange(rrb.hostAddMap[hostToBeClosed], disconnected, false)
	rrb.mu.Unlock()
	return rrb.setForClientRestart()
}

// setForClientRestart sets the flag to mark if client restart is needed during reconnect.
// It returns true if all ws connections are disconnected, false otherwise.
func (rrb *roundRobinBalancer) setForClientRestart() bool {
	if rrb.IsAllDisconnected() {
		rrb.SetClientRestartNeeded(true)
		return true
	}
	return false
}

// CloseAll closes all the socket connections in the wsConns list
// This must be called during shutdown.
func (rrb *roundRobinBalancer) CloseAll() {
	rrb.mu.Lock()
	for host := range rrb.wsConns {
		if rrb.connState[host] != disconnected && rrb.connState[host] != shutdown {
			log.Tracef("Balancer: Disconnecting RPC client %s", host)
			rrb.NotifyConnStateChange(rrb.hostAddMap[host], disconnected, false)
			wsConn, connOk := rrb.wsConns[host]
			if connOk {
				err := wsConn.Close()
				if err != nil {
					log.Errorf("Failed disconnecting to %s: %v", host, err)
				}
			}

		}
	}
	rrb.mu.Unlock()
}

// NotifyReconnect will update the map for wsConns and update the connection state
// for the corresponding host address.
func (rrb *roundRobinBalancer) NotifyReconnect(wsConn *websocket.Conn, hostAdd *HostAddress) {
	rrb.mu.Lock()
	rrb.connState[hostAdd.Host] = ready
	rrb.wsConns[hostAdd.Host] = wsConn
	rrb.hostAddMap[hostAdd.Host].retryCount = 0
	rrb.mu.Unlock()
	go rrb.wsInHandler(wsConn, hostAdd.Host)
}

// UpdateReconnectAttempt increments the connection's retry counter by one
// for corresponding host address.
func (rrb *roundRobinBalancer) UpdateReconnectAttempt(hostAdd *HostAddress) {
	rrb.mu.Lock()
	rrb.hostAddMap[hostAdd.Host].retryCount++
	rrb.mu.Unlock()
}

// updateInitialConnectAttempt increments the connection's initial connection attempt by one
// for corresponding host address.
func (rrb *roundRobinBalancer) updateInitialConnectAttempt(hostAdd *HostAddress) int {
	rrb.mu.Lock()
	rrb.hostAddMap[hostAdd.Host].initialConnectAttemptCount++
	attempt := rrb.hostAddMap[hostAdd.Host].initialConnectAttemptCount
	rrb.mu.Unlock()
	return attempt
}

// IsAllDisconnected would return true if all hostaddresses are marked with
// connection state as Disconnected.
func (rrb *roundRobinBalancer) IsAllDisconnected() bool {
	rrb.mu.Lock()
	for _, state := range rrb.connState {
		if state != disconnected && state != shutdown && state != idle {
			rrb.mu.Unlock()
			return false
		}
	}
	rrb.mu.Unlock()
	return true
}

// IsReady returns true if there is at least one connection ready to use.
func (rrb *roundRobinBalancer) IsReady() bool {
	return rrb.isReady
}

// AllDisconnectedWsConns will iterate over the connection state map and
// return all the wsConnections that have their state as Disconnected.
func (rrb *roundRobinBalancer) AllDisconnectedWsConns() []*HostAddress {
	rrb.mu.Lock()
	var disconnectedWsConns []*HostAddress
	for host, state := range rrb.connState {
		if state == disconnected || state == idle || state == shutdown {
			disconnectedWsConns = append(disconnectedWsConns, rrb.hostAddMap[host])
		}
	}
	rrb.mu.Unlock()
	return disconnectedWsConns
}

// ConnectionState will return the connection state for given host.
func (rrb *roundRobinBalancer) ConnectionState(host string) (state connectionState, ok bool) {
	rrb.mu.Lock()
	state, ok = rrb.connState[host]
	rrb.mu.Unlock()
	return
}

// WsConnection gets the wsConn corresponding to the host.
func (rrb *roundRobinBalancer) WsConnection(host string) (wsConn *websocket.Conn, connOk bool) {
	rrb.mu.Lock()
	wsConn, connOk = rrb.wsConns[host]
	rrb.mu.Unlock()
	return
}

// SetClientRestartNeeded sets the flag used to decide if client.start() needs to be called during reconnect.
func (rrb *roundRobinBalancer) SetClientRestartNeeded(isRestartNeeded bool) {
	rrb.needsClientRestart = isRestartNeeded
}

// ClientRestartNeeded gets the flag used to decide if client.start() needs to be called during reconnect.
func (rrb *roundRobinBalancer) ClientRestartNeeded() bool {
	return rrb.needsClientRestart
}

// BuildBalancer is used to setup balancer with required config.
// If no items in HostAddresses list, then would add one using *ConnConfig.Host
// and *ConnConfig.Endpoint else use the *ConnConfig.HostAddresses for round-robin.
func (c *Client) BuildBalancer(config *ConnConfig) (Balancer, error) {
	if config.Balancer != "" && config.Balancer != "RoundRobinBalancer" {
		return nil, ErrBalancerNotFound
	}
	if len(config.HostAddresses) == 0 {
		config.HostAddresses = []HostAddress{{Endpoint: config.Endpoint, Host: config.Host}}
	}
	hostAddressesMap := make(map[string]*HostAddress)
	for i, val := range config.HostAddresses {
		hostAddressesMap[val.Host] = &config.HostAddresses[i]
	}
	var rrb Balancer = &roundRobinBalancer{
		connPicker: picker{
			conns: config.HostAddresses,
			next:  0,
		},
		connConfig:          config,
		connState:           make(map[string]connectionState),
		wsConns:             make(map[string]*websocket.Conn),
		hostAddMap:          hostAddressesMap,
		wsInHandler:         c.wsInHandler,
		connForNotification: make(map[string]string),
	}
	return rrb, nil
}
