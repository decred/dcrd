// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"github.com/gorilla/websocket"
	"time"
)

// Balancer defines requirements for a load balancer
// using a list of host addresses as the source.
type Balancer interface {
	// NextConn gets the next usable connection. The balancer remembers connections
	// used for notification methods, these connections are reused on subsequent
	// calls of the notification calls. The default round robin process is used in picking
	// connections for all other methods.
	NextConn(methodName string) (wsConn *websocket.Conn, hostAddress *HostAddress, err error)

	// ConnectionState returns the connection state of corresponding ws connection.
	ConnectionState(host string) (state connectionState, ok bool)

	// WsConnection returns the ws connection corresponding to the host.
	WsConnection(host string) (wsConn *websocket.Conn, connOk bool)

	// AllDisconnectedWsConns returns all the ws connections with
	// a disconnected state.
	AllDisconnectedWsConns() []*HostAddress

	// NotifyConnStateChange is called by rpc client when the connection state
	// for a host address changes.
	NotifyConnStateChange(sc *HostAddress, state connectionState)

	// UpdateReconnectAttempt increments the connection's retry counter by one.
	UpdateReconnectAttempt(hostAdd *HostAddress)

	// NotifyReconnect updates the connection and state of the provided host address
	// on reconnection..
	NotifyReconnect(wsConn *websocket.Conn, hostAdd *HostAddress)

	// Close closes the passed ws connection and sets the state as Disconnected.
	// It returns true if the connection was the last WS connection to be closed.
	Close(wsConn *websocket.Conn) bool

	// CloseAll terminates all active connections.
	CloseAll()

	// SetClientRestartNeeded sets the flag used to decide if client restart needs to be called during reconnect.
	SetClientRestartNeeded(isRestartNeeded bool)

	// ClientRestartNeeded returns the flag used to decide if client restart needs to be called during reconnect.
	ClientRestartNeeded() bool

	// IsAllDisconnected returns true if all host addresses are marked with
	// connection state as Disconnected.
	IsAllDisconnected() bool

	// IsReady returns true if there is at least one connection ready to use.
	IsReady() bool

	// GetNextAttemptInvterval gets the next retry interval scaling by the number of
	// attempts so there is a backoff up to a max of 1 minute and returns the duration.
	GetNextAttemptInvterval(attempt int64) time.Duration
}

// Picker is used to pick the next connection to be used.
type Picker interface {
	// Pick checks for the next usable connection with the balancer.
	Pick(balancer Balancer) (*HostAddress, error)
}
