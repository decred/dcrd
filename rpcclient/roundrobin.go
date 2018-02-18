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
}

// Balancer uses Picker to pick connections.
// It collects and maintains the connectivity states.
type Balancer interface {
	//Get gets the next usabel connection
	Get() (wsConn *websocket.Conn, hostAddress *HostAddress, err error)
	// NotifyConnStateChange is called by rpcClient when the ConnectionState
	// for a hostaddress changes.
	NotifyConnStateChange(sc *HostAddress, state ConnectionState)
	// Close closes all connections
	Close()
}

// Picker is used to pick the next connection to be used
type Picker interface {
	// Pick checks for the next usable connection using the connState map
	Pick(connState map[string]ConnectionState, balancer Balancer) (*HostAddress, error)
}

type picker struct {
	conns []HostAddress
	mu    sync.Mutex
	next  int
}

//Pick checks for the next usable connection using the connState map
//It changes the state to Conecting, if the connection picked is not yet used
func (p *picker) Pick(connState map[string]ConnectionState, balancer Balancer) (conn *HostAddress, err error) {

	if len(p.conns) <= 0 {
		log.Infof(ErrNoConnAvailable.Error())
		return conn, ErrNoConnAvailable
	}

	p.mu.Lock()
	startPos := p.next
	var ok bool

	for {
		conn = &p.conns[p.next]
		state, ok := connState[conn.Host]
		p.next = (p.next + 1) % len(p.conns)
		if !ok || (state != Shutdown && state != Connecting) {
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
		balancer.NotifyConnStateChange(conn, Connecting)
	}
	p.mu.Unlock()
	return conn, nil
}

//RoundRobinBalancer represents the balancer implementing Balancer
//and maintaining a Picker
type RoundRobinBalancer struct {
	connPicker    picker
	connConfig    *ConnConfig
	wsConns       map[string]*websocket.Conn
	mu            sync.Mutex
	connState     map[string]ConnectionState
	waitToConnect chan struct{}
	isReady       bool
}

//Get gets the next connection to be used.
//It returns both *websocket.Conn and *HostAddress
//but at a time only one would be valid. i.e. based on
//HttpPostMode
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

	hostAddress, err := rrb.connPicker.Pick(rrb.connState, rrb)
	if err != nil {
		return nil, nil, err
	}
	var wsConn *websocket.Conn
	if !rrb.connConfig.HTTPPostMode {
		_, ok := rrb.wsConns[hostAddress.Host]
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
				rrb.NotifyConnStateChange(hostAddress, Shutdown)
				//try next
				hostAddress, err = rrb.connPicker.Pick(rrb.connState, rrb)
				if err != nil {
					return nil, nil, err
				}
				wsConn, ok = rrb.wsConns[hostAddress.Host]
			} else {
				rrb.mu.Lock()
				rrb.wsConns[hostAddress.Host] = wsConn
				rrb.mu.Unlock()
				rrb.NotifyConnStateChange(hostAddress, Ready)
				log.Infof("Balancer: Established connection to RPC server %s",
					hostAddress.Host)
				break
			}
		}
		wsConn = rrb.wsConns[hostAddress.Host]
	} else {
		rrb.NotifyConnStateChange(hostAddress, Ready)
	}
	log.Infof("Balancer: Connection pick for RPC %s",
		hostAddress.Host)
	return wsConn, hostAddress, err
}

//NotifyConnStateChange updates connState map for the given address.
//Also updates the isReady field to indicate that at least one connection
//with Ready state exists
func (rrb *RoundRobinBalancer) NotifyConnStateChange(sc *HostAddress, state ConnectionState) {
	rrb.mu.Lock()
	if state == Shutdown && !rrb.connConfig.HTTPPostMode {
		delete(rrb.wsConns, sc.Host)
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
	rrb.connState[sc.Host] = state
	rrb.mu.Unlock()
}

//Close closes all the socket connections in the wsConns list
func (rrb *RoundRobinBalancer) Close() {
	rrb.mu.Lock()
	for host := range rrb.wsConns {
		log.Tracef("Balancer: Disconnecting RPC client %s", host)
		rrb.wsConns[host].Close()
	}
	rrb.mu.Unlock()
}

//BuildBalancer is used to setup balancer with required config
//If no items in HostAddresses list, then would add one using *ConnConfig.Host
//else use the *ConnConfig.HostAddresses
func (cn *Client) BuildBalancer(cc *ConnConfig) *RoundRobinBalancer {

	if len(cc.HostAddresses) <= 0 {
		cc.HostAddresses = []HostAddress{HostAddress{Endpoint: cc.Endpoint, Host: cc.Host}}
	}
	balancer := &RoundRobinBalancer{
		connPicker: picker{
			conns: cc.HostAddresses,
			next:  0,
		},
		connConfig: cc,
		connState:  make(map[string]ConnectionState),
		wsConns:    make(map[string]*websocket.Conn),
	}
	return balancer
}
