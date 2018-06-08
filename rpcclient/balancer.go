// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"github.com/gorilla/websocket"
)

// Balancer defines requirements for a load balancer
// using list of hostaddresses as the source.
type Balancer interface {
	// NextConn gets the next usabel connection.
	// If the methodName is one of notifications, then would remember the connection used
	// for this method and would next time pick the same connection.
	// Else would consider normal routine to get the next connection.
	NextConn(methodName string) (wsConn *websocket.Conn, hostAddress *HostAddress, err error)

	// ConnectionState gets the connectionstate of corresponding wsConn.
	ConnectionState(host string) (state ConnectionState, ok bool)

	// WsConnection gets the wsConn corresponding to the host.
	WsConnection(host string) (wsConn *websocket.Conn, connOk bool)

	// NextDisconnectedWsConn will iterate over the connection state map and
	// return the first wsConnection that has its state as Disconnected.
	NextDisconnectedWsConn() *HostAddress

	// NotifyConnStateChange is called by rpcClient when the ConnectionState
	// for a hostaddress changes.
	NotifyConnStateChange(sc *HostAddress, state ConnectionState, sync bool)

	// UpdateReconnectAttempt will increase retryattempt + 1.
	UpdateReconnectAttempt(hostAdd *HostAddress)

	// NotifyReconnect will update the map for wsConns and update the connection state
	// for this address.
	NotifyReconnect(wsConn *websocket.Conn, hostAdd *HostAddress)

	// Close closes the passed ws connection and sets the state as Disconnected
	// by calling NotifyConnStateChange.
	// Returns true if this was the last WS connection to be closed.
	Close(wsConn *websocket.Conn) bool

	// CloseAll closes all the ws connections which are active at present.
	CloseAll()

	// SetClientRestartNeeded sets the flag used to decide if client.start() needs to be called during reconnect.
	SetClientRestartNeeded(isRestartNeeded bool)

	// ClientRestartNeeded gets the flag used to decide if client.start() needs to be called during reconnect.
	ClientRestartNeeded() bool

	// IsAllDisconnected would return true if all hostaddresses are marked with
	// connection state as Disconnected.
	IsAllDisconnected() bool

	// IsReady returns true if there is at least one connection ready to use
	IsReady() bool
}

// Picker is used to pick the next connection to be used.
type Picker interface {
	// Pick checks for the next usable connection using the connState map with balancer.
	Pick(balancer Balancer) (*HostAddress, error)
}
