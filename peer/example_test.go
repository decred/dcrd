// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer_test

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/decred/dcrd/crypto/rand"
	"github.com/decred/dcrd/peer/v4"
	"github.com/decred/dcrd/wire"
)

// mockRemotePeer creates a basic inbound peer listening on the simnet port for
// use with Example_peerConnection.  It does not return until the listener is
// active.
func mockRemotePeer(listenAddr string) (net.Listener, error) {
	// Configure peer to act as a simnet node that offers no services.
	peerCfg := &peer.Config{
		UserAgentName:    "peer",  // User agent name to advertise.
		UserAgentVersion: "1.0.0", // User agent version to advertise.
		Net:              wire.SimNet,
		IdleTimeout:      time.Second * 120,
	}

	// Accept connections on the simnet port.
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Accept: error %v\n", err)
			return
		}

		// Create and start the inbound peer.
		go func() {
			p := peer.NewInboundPeer(peerCfg, conn)
			if err := p.Handshake(context.Background(), nil); err != nil {
				fmt.Printf("inbound handshake error: %v\n", err)
				return
			}
			p.Start()
		}()
	}()

	return listener, nil
}

// This example demonstrates the basic process for initializing and creating an
// outbound peer.  Peers negotiate by exchanging version and verack messages.
// For demonstration, a simple handler for version message is attached to the
// peer.
func Example_newOutboundPeer() {
	// Ordinarily this will not be needed since the outbound peer will be
	// connecting to a remote peer, however, since this example is executed
	// and tested, a mock remote peer is needed to listen for the outbound
	// peer.
	listener, err := mockRemotePeer("127.0.0.1:18555")
	if err != nil {
		fmt.Printf("mockRemotePeer: unexpected error %v\n", err)
		return
	}
	defer listener.Close()

	// Establish the connection to the peer address.
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		fmt.Printf("net.Dial: error %v\n", err)
		return
	}

	// Create an outbound peer that is configured to act as a simnet node
	// that offers no services and has a listener for the pong message.
	//
	// Then perform the initial handshake and start the async I/O handling.
	//
	// The pong listener is used here to signal the code below when it arrives
	// in response to an example ping.
	pong := make(chan struct{})
	peerCfg := &peer.Config{
		UserAgentName:    "peer",  // User agent name to advertise.
		UserAgentVersion: "1.0.0", // User agent version to advertise.
		Net:              wire.SimNet,
		Services:         0,
		Listeners: peer.MessageListeners{
			// This uses a simple channel for the purposes of the example, but
			// callers will typically find it much more ergonomic to create a
			// type that houses additional state and exposes methods for the
			// desired listeners.  Then the listeners may be set to a concrete
			// instance of that type so that they close over the additional
			// state.
			OnPong: func(p *peer.Peer, msg *wire.MsgPong) {
				pong <- struct{}{}
			},
		},
		IdleTimeout: time.Second * 120,
	}
	p := peer.NewOutboundPeer(peerCfg, conn.RemoteAddr(), conn)
	if err := p.Handshake(context.Background(), nil); err != nil {
		fmt.Printf("outbound peer handshake error: %v\n", err)
		return
	}
	p.Start()

	// Ping the remote peer aysnchronously.
	p.QueueMessage(wire.NewMsgPing(rand.Uint64()), nil)

	// Wait for the pong message or timeout in case of failure.
	select {
	case <-pong:
		fmt.Println("outbound: received pong")
	case <-time.After(time.Second * 1):
		fmt.Printf("Example_newOutboundPeer: pong timeout")
	}

	// Disconnect the peer.
	p.Disconnect()
	p.WaitForDisconnect()

	// Output:
	// outbound: received pong
}
