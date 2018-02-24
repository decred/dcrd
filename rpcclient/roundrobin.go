// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"errors"
	"sync"

	"github.com/btcsuite/websocket"
)

var (
	// ErrNoConnAvailable indicates no Conn is available for pick().
	ErrNoConnAvailable = errors.New("no Connection is available")
	//ErrNoUsableConnAvailable indicates no usable connections available i.e. marked as Shutdown
	ErrNoUsableConnAvailable = errors.New("no Connection is available for use, all are marked as Shutdown")
)

// ConnectionState indicates the current state of connection.
type ConnectionState int

func (s ConnectionState) String() string {
	switch s {
	case Idle:
		return "IDLE"
	case Connecting:
		return "CONNECTING"
	case Ready:
		return "READY"
	case Shutdown:
		return "SHUTDOWN"
	default:
		return "Invalid-State"
	}
}

const (
	// Idle indicates the conn is idle.
	Idle ConnectionState = iota
	// Connecting indicates the conn is connecting.
	Connecting
	// Disconnected indicates the conn marked to be picked by reconnect handler
	Disconnected
	// Ready indicates the conn is ready for work.
	Ready
	// Shutdown indicates the ClientConn has started shutting down.
	Shutdown
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
}

// Balancer uses Picker to pick connections.
// It collects and maintains the connectivity states.
type Balancer interface {
	//Get gets the next usabel connection
	Get() (wsConn *websocket.Conn, hostAddress *HostAddress, err error)
	//GetConnectionState gets the connectionstate of corresponding wsConn
	GetConnectionState(host string) (state ConnectionState, ok bool)
	// GetWsConnection gets the wsConn corresponding to the host
	GetWsConnection(host string) (wsConn *websocket.Conn, connOk bool)
	// NotifyConnStateChange is called by rpcClient when the ConnectionState
	// for a hostaddress changes.
	NotifyConnStateChange(sc *HostAddress, state ConnectionState, sync bool)
	//UpdateReconnectAttempt will increase retryattempt + 1
	UpdateReconnectAttempt(hostAdd *HostAddress)
	//NotifyReconnect will update the map for wsConns and update the connection state
	//for this address
	NotifyReconnect(wsConn *websocket.Conn, hostAdd *HostAddress)
	// Close closes corresponding connection if host is not empty
	// else would close all the WS connections, setting the state as Disconnected
	Close(host string)
}

// Picker is used to pick the next connection to be used
type Picker interface {
	// Pick checks for the next usable connection using the connState map with balancer
	Pick(balancer Balancer) (*HostAddress, error)
}

type picker struct {
	conns []HostAddress
	mu    sync.Mutex
	next  int
}

//Pick checks for the next usable connection using the connState map
//It changes the state to Conecting, if the connection picked is not yet used
func (p *picker) Pick(balancer Balancer) (conn *HostAddress, err error) {

	if len(p.conns) <= 0 {
		log.Infof(ErrNoConnAvailable.Error())
		return conn, ErrNoConnAvailable
	}

	p.mu.Lock()
	startPos := p.next
	var ok bool

	for {
		conn = &p.conns[p.next]
		state, ok := balancer.GetConnectionState(conn.Host)
		p.next = (p.next + 1) % len(p.conns)
		if !ok || state == Ready || state == Idle {
			//either not yet tried or ready to use
			break
		}
		//back to start
		if startPos == p.next {
			break
		}
	}
	if conn == nil {
		return conn, ErrNoUsableConnAvailable
	}
	if !ok {
		balancer.NotifyConnStateChange(conn, Connecting, true)
	}
	p.mu.Unlock()
	return conn, nil
}

// RoundRobinBalancer represents the balancer implementing Balancer
// and maintaining a Picker
type RoundRobinBalancer struct {
	connPicker    picker
	connConfig    *ConnConfig
	wsConns       map[string]*websocket.Conn
	hostAddMap    map[string]*HostAddress
	mu            sync.Mutex
	connState     map[string]ConnectionState
	waitToConnect chan struct{}
	isReady       bool
	// This is used to decide if client.start() needs to be called during reconnect
	// Set to true when all connection are either disconnected or shutdown
	// Set to false after client.start()
	NeedsClientRestart bool
}

//Get gets the next connection to be used.
//It returns both *websocket.Conn and *HostAddress
func (rrb *RoundRobinBalancer) Get() (*websocket.Conn, *HostAddress, error) {

	if !rrb.isReady {
		select {
		//will wait if any handshake inprogress
		//This is to avoid cases where all connections are in connecting mode and
		//consecutive calls woun't get any Address as Pick would skip all and err out saying
		//no connections available
		case <-rrb.waitToConnect:
		default:
		}
	}

	hostAddress, err := rrb.connPicker.Pick(rrb)
	if err != nil {
		return nil, nil, err
	}
	var wsConn *websocket.Conn
	if !rrb.connConfig.HTTPPostMode {
		_, ok := rrb.GetWsConnection(hostAddress.Host)
		for !ok {
			rrb.waitToConnect = make(chan struct{})
			rrb.connConfig.Host = hostAddress.Host
			rrb.connConfig.Endpoint = hostAddress.Endpoint
			wsConn, err = dial(rrb.connConfig)
			close(rrb.waitToConnect)
			if err != nil {
				log.Infof("Balancer: Failed to connect to %s: %v",
					rrb.connConfig.Host, err)
				//change conn state
				rrb.NotifyConnStateChange(hostAddress, Shutdown, true)
				//try next
				hostAddress, err = rrb.connPicker.Pick(rrb)
				if err != nil {
					return nil, nil, err
				}
				wsConn, ok = rrb.GetWsConnection(hostAddress.Host)
			} else {
				rrb.mu.Lock()
				rrb.wsConns[hostAddress.Host] = wsConn
				rrb.mu.Unlock()
				rrb.NotifyConnStateChange(hostAddress, Ready, true)
				log.Infof("Balancer: Established connection to RPC server %s",
					hostAddress.Host)
				break
			}
		}
		wsConn, _ = rrb.GetWsConnection(hostAddress.Host)
	} else {
		rrb.NotifyConnStateChange(hostAddress, Ready, true)
	}
	log.Infof("Balancer: Connection pick for RPC %s",
		hostAddress.Host)
	return wsConn, hostAddress, err
}

//NotifyConnStateChange updates connState map for the given address.
//Also updates the isReady field to indicate that at least one connection
//with Ready state exists
func (rrb *RoundRobinBalancer) NotifyConnStateChange(hostAdd *HostAddress, state ConnectionState, sync bool) {
	if sync {
		rrb.mu.Lock()
	}
	if state == Shutdown && !rrb.connConfig.HTTPPostMode {
		delete(rrb.wsConns, hostAdd.Host)
	}
	if state == Ready {
		rrb.isReady = true
	} else {
		rrb.isReady = false
		for _, state := range rrb.connState {
			if state == Ready {
				rrb.isReady = true
				break
			}
		}
	}
	rrb.connState[hostAdd.Host] = state
	if sync {
		rrb.mu.Unlock()
	}
}

// Close closes all the socket connections in the wsConns list
// if no host string passed.
// This must be called during shutdown with empty string
func (rrb *RoundRobinBalancer) Close(host string) {
	rrb.mu.Lock()
	if host != "" {
		log.Tracef("Balancer: Disconnecting RPC client %s", host)
		rrb.wsConns[host].Close()
		rrb.NotifyConnStateChange(rrb.hostAddMap[host], Disconnected, false)
		rrb.mu.Unlock()
		return
	}

	for host := range rrb.wsConns {
		if rrb.connState[host] != Shutdown {
			log.Tracef("Balancer: Disconnecting RPC client %s", host)
			rrb.NotifyConnStateChange(rrb.hostAddMap[host], Disconnected, false)
			rrb.wsConns[host].Close()
		}
	}
	rrb.mu.Unlock()
}

// NotifyReconnect will update the map for wsConns and update the connection state
// for this address
func (rrb *RoundRobinBalancer) NotifyReconnect(wsConn *websocket.Conn, hostAdd *HostAddress) {
	rrb.mu.Lock()
	rrb.connState[hostAdd.Host] = Ready
	rrb.wsConns[hostAdd.Host] = wsConn
	rrb.hostAddMap[hostAdd.Host].retryCount = 0
	rrb.mu.Unlock()
}

// UpdateReconnectAttempt will increase retryattempt + 1
// for this address
func (rrb *RoundRobinBalancer) UpdateReconnectAttempt(hostAdd *HostAddress) {
	rrb.mu.Lock()
	rrb.hostAddMap[hostAdd.Host].retryCount++
	rrb.mu.Unlock()
}

// IsAllDisconnected would return true if all hostaddress are marked with
// connection state as Disconnected
func (rrb *RoundRobinBalancer) IsAllDisconnected() bool {
	rrb.mu.Lock()
	for _, state := range rrb.connState {
		if state != Disconnected && state != Shutdown {
			rrb.mu.Unlock()
			return false
		}
	}
	rrb.mu.Unlock()
	return true
}

//GetNextDisconnectedWsConn will iterate over the connection state map and
//return the first wsConnection that has its state as Disconnected
func (rrb *RoundRobinBalancer) GetNextDisconnectedWsConn() *HostAddress {
	rrb.mu.Lock()
	for host, state := range rrb.connState {
		if state == Disconnected {
			rrb.mu.Unlock()
			return rrb.hostAddMap[host]
		}
	}
	rrb.mu.Unlock()
	return nil
}

//GetConnectionState will return the connection state for given host
func (rrb *RoundRobinBalancer) GetConnectionState(host string) (state ConnectionState, ok bool) {
	rrb.mu.Lock()
	state, ok = rrb.connState[host]
	rrb.mu.Unlock()
	return
}

// GetWsConnection gets the wsConn corresponding to the host
func (rrb *RoundRobinBalancer) GetWsConnection(host string) (wsConn *websocket.Conn, connOk bool) {
	rrb.mu.Lock()
	wsConn, connOk = rrb.wsConns[host]
	rrb.mu.Unlock()
	return
}

//BuildBalancer is used to setup balancer with required config
//If no items in HostAddresses list, then would add one using *ConnConfig.Host
//else use the *ConnConfig.HostAddresses
func (cn *Client) BuildBalancer(cc *ConnConfig) *RoundRobinBalancer {

	if len(cc.HostAddresses) <= 0 {
		cc.HostAddresses = []HostAddress{HostAddress{Endpoint: cc.Endpoint, Host: cc.Host}}
	}
	hostAddressesMap := make(map[string]*HostAddress)
	for _, val := range cc.HostAddresses {
		hostAddressesMap[val.Host] = &val
	}
	balancer := &RoundRobinBalancer{
		connPicker: picker{
			conns: cc.HostAddresses,
			next:  0,
		},
		connConfig: cc,
		connState:  make(map[string]ConnectionState),
		wsConns:    make(map[string]*websocket.Conn),
		hostAddMap: hostAddressesMap,
	}
	return balancer
}
