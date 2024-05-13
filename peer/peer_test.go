// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
	"github.com/decred/go-socks/socks"
)

// conn mocks a network connection by implementing the net.Conn interface.  It
// is used to test peer connection without actually opening a network
// connection.
type conn struct {
	io.ReadCloser
	io.WriteCloser

	// local network, address for the connection.
	lnet, laddr string

	// remote network, address for the connection.
	rnet, raddr string

	// mocks socks proxy if true
	proxy bool
}

// LocalAddr returns the local address for the connection.
func (c conn) LocalAddr() net.Addr {
	return &addr{c.lnet, c.laddr}
}

// Remote returns the remote address for the connection.
func (c conn) RemoteAddr() net.Addr {
	if !c.proxy {
		return &addr{c.rnet, c.raddr}
	}
	host, strPort, _ := net.SplitHostPort(c.raddr)
	port, _ := strconv.Atoi(strPort)
	return &socks.ProxiedAddr{
		Net:  c.rnet,
		Host: host,
		Port: port,
	}
}

// Close handles closing the connection.
func (c conn) Close() error {
	readCloseErr := c.ReadCloser.Close()
	writeCloseErr := c.WriteCloser.Close()
	if readCloseErr != nil {
		return readCloseErr
	}
	return writeCloseErr
}

func (c conn) SetDeadline(t time.Time) error      { return nil }
func (c conn) SetReadDeadline(t time.Time) error  { return nil }
func (c conn) SetWriteDeadline(t time.Time) error { return nil }

// addr mocks a network address.
type addr struct {
	net, address string
}

func (m addr) Network() string { return m.net }
func (m addr) String() string  { return m.address }

// pipe turns two mock connections into a full-duplex connection similar to
// net.Pipe to allow pipe's with (fake) addresses.
func pipe(c1, c2 *conn) (*conn, *conn) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	c1.WriteCloser = w1
	c2.ReadCloser = r1
	c1.ReadCloser = r2
	c2.WriteCloser = w2

	return c1, c2
}

// peerStats holds the expected peer stats used for testing peer.
type peerStats struct {
	wantUserAgent       string
	wantServices        wire.ServiceFlag
	wantProtocolVersion uint32
	wantConnected       bool
	wantVersionKnown    bool
	wantVerAckReceived  bool
	wantLastBlock       int64
	wantStartingHeight  int64
	wantLastPingTime    time.Time
	wantLastPingNonce   uint64
	wantLastPingMicros  int64
	wantTimeOffset      int64
	wantBytesSent       uint64
	wantBytesReceived   uint64
}

// testPeer tests the given peer's flags and stats.
func testPeer(t *testing.T, p *Peer, s peerStats) {
	if p.UserAgent() != s.wantUserAgent {
		t.Errorf("testPeer: wrong UserAgent - got %v, want %v", p.UserAgent(), s.wantUserAgent)
		return
	}

	if p.Services() != s.wantServices {
		t.Errorf("testPeer: wrong Services - got %v, want %v", p.Services(), s.wantServices)
		return
	}

	if !p.LastPingTime().Equal(s.wantLastPingTime) {
		t.Errorf("testPeer: wrong LastPingTime - got %v, want %v", p.LastPingTime(), s.wantLastPingTime)
		return
	}

	if p.LastPingNonce() != s.wantLastPingNonce {
		t.Errorf("testPeer: wrong LastPingNonce - got %v, want %v", p.LastPingNonce(), s.wantLastPingNonce)
		return
	}

	if p.LastPingMicros() != s.wantLastPingMicros {
		t.Errorf("testPeer: wrong LastPingMicros - got %v, want %v", p.LastPingMicros(), s.wantLastPingMicros)
		return
	}

	if p.VerAckReceived() != s.wantVerAckReceived {
		t.Errorf("testPeer: wrong VerAckReceived - got %v, want %v", p.VerAckReceived(), s.wantVerAckReceived)
		return
	}

	if p.VersionKnown() != s.wantVersionKnown {
		t.Errorf("testPeer: wrong VersionKnown - got %v, want %v", p.VersionKnown(), s.wantVersionKnown)
		return
	}

	if p.ProtocolVersion() != s.wantProtocolVersion {
		t.Errorf("testPeer: wrong ProtocolVersion - got %v, want %v", p.ProtocolVersion(), s.wantProtocolVersion)
		return
	}

	if p.LastBlock() != s.wantLastBlock {
		t.Errorf("testPeer: wrong LastBlock - got %v, want %v", p.LastBlock(), s.wantLastBlock)
		return
	}

	// Allow for a deviation of 1s, as the second may tick when the message is
	// in transit and the protocol doesn't support any further precision.
	if p.TimeOffset() != s.wantTimeOffset && p.TimeOffset() != s.wantTimeOffset-1 {
		t.Errorf("testPeer: wrong TimeOffset - got %v, want %v or %v", p.TimeOffset(),
			s.wantTimeOffset, s.wantTimeOffset-1)
		return
	}

	if p.BytesSent() != s.wantBytesSent {
		t.Errorf("testPeer: wrong BytesSent - got %v, want %v", p.BytesSent(), s.wantBytesSent)
		return
	}

	if p.BytesReceived() != s.wantBytesReceived {
		t.Errorf("testPeer: wrong BytesReceived - got %v, want %v", p.BytesReceived(), s.wantBytesReceived)
		return
	}

	if p.StartingHeight() != s.wantStartingHeight {
		t.Errorf("testPeer: wrong StartingHeight - got %v, want %v", p.StartingHeight(), s.wantStartingHeight)
		return
	}

	if p.Connected() != s.wantConnected {
		t.Errorf("testPeer: wrong Connected - got %v, want %v", p.Connected(), s.wantConnected)
		return
	}

	stats := p.StatsSnapshot()

	if p.ID() != stats.ID {
		t.Errorf("testPeer: wrong ID - got %v, want %v", p.ID(), stats.ID)
		return
	}

	if p.Addr() != stats.Addr {
		t.Errorf("testPeer: wrong Addr - got %v, want %v", p.Addr(), stats.Addr)
		return
	}

	if p.LastSend() != stats.LastSend {
		t.Errorf("testPeer: wrong LastSend - got %v, want %v", p.LastSend(), stats.LastSend)
		return
	}

	if p.LastRecv() != stats.LastRecv {
		t.Errorf("testPeer: wrong LastRecv - got %v, want %v", p.LastRecv(), stats.LastRecv)
		return
	}
}

// TestPeerConnection tests connection between inbound and outbound peers.
func TestPeerConnection(t *testing.T) {
	var pause sync.Mutex
	verack := make(chan struct{})
	peerCfg := &Config{
		Listeners: MessageListeners{
			OnVerAck: func(p *Peer, msg *wire.MsgVerAck) {
				verack <- struct{}{}
			},
			OnWrite: func(p *Peer, bytesWritten int, msg wire.Message,
				err error) {
				if _, ok := msg.(*wire.MsgVerAck); ok {
					verack <- struct{}{}
				}
				pause.Lock()
				// Needed to squash empty critical section lint errors.
				_ = p
				pause.Unlock()
			},
		},
		UserAgentName:    "peer",
		UserAgentVersion: "1.0",
		Net:              wire.MainNet,
		Services:         0,
	}
	wantStats := peerStats{
		wantUserAgent:       wire.DefaultUserAgent + "peer:1.0/",
		wantServices:        0,
		wantProtocolVersion: MaxProtocolVersion,
		wantConnected:       true,
		wantVersionKnown:    true,
		wantVerAckReceived:  true,
		wantLastPingTime:    time.Time{},
		wantLastPingNonce:   uint64(0),
		wantLastPingMicros:  int64(0),
		wantTimeOffset:      int64(0),
		wantBytesSent:       158, // 134 version + 24 verack
		wantBytesReceived:   158,
	}
	tests := []struct {
		name  string
		setup func() (*Peer, *Peer, error)
	}{
		{
			"basic handshake",
			func() (*Peer, *Peer, error) {
				inConn, outConn := pipe(
					&conn{raddr: "10.0.0.1:8333"},
					&conn{raddr: "10.0.0.2:8333"},
				)
				inPeer := NewInboundPeer(peerCfg)
				inPeer.AssociateConnection(inConn)

				outPeer, err := NewOutboundPeer(peerCfg, "10.0.0.2:8333")
				if err != nil {
					return nil, nil, err
				}
				outPeer.AssociateConnection(outConn)

				for i := 0; i < 4; i++ {
					select {
					case <-verack:
					case <-time.After(time.Second):
						return nil, nil, errors.New("verack timeout")
					}
				}
				return inPeer, outPeer, nil
			},
		},
		{
			"socks proxy",
			func() (*Peer, *Peer, error) {
				inConn, outConn := pipe(
					&conn{raddr: "10.0.0.1:8333", proxy: true},
					&conn{raddr: "10.0.0.2:8333"},
				)
				inPeer := NewInboundPeer(peerCfg)
				inPeer.AssociateConnection(inConn)

				outPeer, err := NewOutboundPeer(peerCfg, "10.0.0.2:8333")
				if err != nil {
					return nil, nil, err
				}
				outPeer.AssociateConnection(outConn)

				for i := 0; i < 4; i++ {
					select {
					case <-verack:
					case <-time.After(time.Second):
						return nil, nil, errors.New("verack timeout")
					}
				}
				return inPeer, outPeer, nil
			},
		},
	}
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		inPeer, outPeer, err := test.setup()
		if err != nil {
			t.Errorf("TestPeerConnection setup #%d: unexpected err %v", i, err)
			return
		}

		pause.Lock()
		testPeer(t, inPeer, wantStats)
		testPeer(t, outPeer, wantStats)
		pause.Unlock()

		inPeer.Disconnect()
		outPeer.Disconnect()
		inPeer.WaitForDisconnect()
		outPeer.WaitForDisconnect()
	}
}

// TestPeerListeners tests that the peer listeners are called as expected.
func TestPeerListeners(t *testing.T) {
	verack := make(chan struct{}, 1)
	ok := make(chan wire.Message, 20)
	peerCfg := &Config{
		Listeners: MessageListeners{
			OnGetAddr: func(p *Peer, msg *wire.MsgGetAddr) {
				ok <- msg
			},
			OnAddr: func(p *Peer, msg *wire.MsgAddr) {
				ok <- msg
			},
			OnPing: func(p *Peer, msg *wire.MsgPing) {
				ok <- msg
			},
			OnPong: func(p *Peer, msg *wire.MsgPong) {
				ok <- msg
			},
			OnMemPool: func(p *Peer, msg *wire.MsgMemPool) {
				ok <- msg
			},
			OnTx: func(p *Peer, msg *wire.MsgTx) {
				ok <- msg
			},
			OnBlock: func(p *Peer, msg *wire.MsgBlock, buf []byte) {
				ok <- msg
			},
			OnInv: func(p *Peer, msg *wire.MsgInv) {
				ok <- msg
			},
			OnHeaders: func(p *Peer, msg *wire.MsgHeaders) {
				ok <- msg
			},
			OnNotFound: func(p *Peer, msg *wire.MsgNotFound) {
				ok <- msg
			},
			OnGetData: func(p *Peer, msg *wire.MsgGetData) {
				ok <- msg
			},
			OnGetBlocks: func(p *Peer, msg *wire.MsgGetBlocks) {
				ok <- msg
			},
			OnGetHeaders: func(p *Peer, msg *wire.MsgGetHeaders) {
				ok <- msg
			},
			OnGetCFilter: func(p *Peer, msg *wire.MsgGetCFilter) {
				ok <- msg
			},
			OnGetCFHeaders: func(p *Peer, msg *wire.MsgGetCFHeaders) {
				ok <- msg
			},
			OnGetCFTypes: func(p *Peer, msg *wire.MsgGetCFTypes) {
				ok <- msg
			},
			OnCFilter: func(p *Peer, msg *wire.MsgCFilter) {
				ok <- msg
			},
			OnCFHeaders: func(p *Peer, msg *wire.MsgCFHeaders) {
				ok <- msg
			},
			OnCFTypes: func(p *Peer, msg *wire.MsgCFTypes) {
				ok <- msg
			},
			OnFeeFilter: func(p *Peer, msg *wire.MsgFeeFilter) {
				ok <- msg
			},
			OnVersion: func(p *Peer, msg *wire.MsgVersion) {
				ok <- msg
			},
			OnVerAck: func(p *Peer, msg *wire.MsgVerAck) {
				verack <- struct{}{}
			},
			OnSendHeaders: func(p *Peer, msg *wire.MsgSendHeaders) {
				ok <- msg
			},
			OnGetCFilterV2: func(p *Peer, msg *wire.MsgGetCFilterV2) {
				ok <- msg
			},
			OnCFilterV2: func(p *Peer, msg *wire.MsgCFilterV2) {
				ok <- msg
			},
			OnGetInitState: func(p *Peer, msg *wire.MsgGetInitState) {
				ok <- msg
			},
			OnInitState: func(p *Peer, msg *wire.MsgInitState) {
				ok <- msg
			},
			OnGetCFiltersV2: func(p *Peer, msg *wire.MsgGetCFsV2) {
				ok <- msg
			},
			OnCFiltersV2: func(p *Peer, msg *wire.MsgCFiltersV2) {
				ok <- msg
			},
		},
		UserAgentName:    "peer",
		UserAgentVersion: "1.0",
		Net:              wire.MainNet,
		Services:         wire.SFNodeBloom,
	}
	inConn, outConn := pipe(
		&conn{raddr: "10.0.0.1:8333"},
		&conn{raddr: "10.0.0.2:8333"},
	)
	inPeer := NewInboundPeer(peerCfg)
	inPeer.AssociateConnection(inConn)

	peerCfg.Listeners = MessageListeners{
		OnVerAck: func(p *Peer, msg *wire.MsgVerAck) {
			verack <- struct{}{}
		},
	}
	outPeer, err := NewOutboundPeer(peerCfg, "10.0.0.1:8333")
	if err != nil {
		t.Errorf("NewOutboundPeer: unexpected err %v\n", err)
		return
	}
	outPeer.AssociateConnection(outConn)

	for i := 0; i < 2; i++ {
		select {
		case <-verack:
		case <-time.After(time.Second * 1):
			t.Error("TestPeerListeners: verack timeout\n")
			return
		}
	}

	select {
	case <-ok:
	case <-time.After(time.Second * 1):
		t.Error("TestPeerListeners: version timeout")
		return
	}

	tests := []struct {
		listener string
		msg      wire.Message
	}{
		{
			"OnGetAddr",
			wire.NewMsgGetAddr(),
		},
		{
			"OnAddr",
			wire.NewMsgAddr(),
		},
		{
			"OnPing",
			wire.NewMsgPing(42),
		},
		{
			"OnPong",
			wire.NewMsgPong(42),
		},
		{
			"OnMemPool",
			wire.NewMsgMemPool(),
		},
		{
			"OnTx",
			wire.NewMsgTx(),
		},
		{
			"OnBlock",
			wire.NewMsgBlock(wire.NewBlockHeader(0, &chainhash.Hash{},
				&chainhash.Hash{}, &chainhash.Hash{}, 1, [6]byte{},
				1, 1, 1, 1, 1, 1, 1, 1, 1, [32]byte{},
				binary.LittleEndian.Uint32([]byte{0xb0, 0x1d, 0xfa, 0xce}))),
		},
		{
			"OnInv",
			wire.NewMsgInv(),
		},
		{
			"OnHeaders",
			wire.NewMsgHeaders(),
		},
		{
			"OnNotFound",
			wire.NewMsgNotFound(),
		},
		{
			"OnGetData",
			wire.NewMsgGetData(),
		},
		{
			"OnGetBlocks",
			wire.NewMsgGetBlocks(&chainhash.Hash{}),
		},
		{
			"OnGetHeaders",
			wire.NewMsgGetHeaders(),
		},
		{
			"OnGetCFilter",
			wire.NewMsgGetCFilter(&chainhash.Hash{},
				wire.GCSFilterRegular),
		},
		{
			"OnGetCFHeaders",
			wire.NewMsgGetCFHeaders(),
		},
		{
			"OnGetCFTypes",
			wire.NewMsgGetCFTypes(),
		},
		{
			"OnCFilter",
			wire.NewMsgCFilter(&chainhash.Hash{},
				wire.GCSFilterRegular, []byte("payload")),
		},
		{
			"OnCFHeaders",
			wire.NewMsgCFHeaders(),
		},
		{
			"OnCFTypes",
			wire.NewMsgCFTypes([]wire.FilterType{
				wire.GCSFilterRegular, wire.GCSFilterExtended,
			}),
		},
		{
			"OnFeeFilter",
			wire.NewMsgFeeFilter(15000),
		},
		{
			"OnGetCFilterV2",
			wire.NewMsgGetCFilterV2(&chainhash.Hash{}),
		},
		{
			"OnCFilterV2",
			wire.NewMsgCFilterV2(&chainhash.Hash{}, nil, 0, nil),
		},
		// only one version message is allowed
		// only one verack message is allowed
		{
			"OnSendHeaders",
			wire.NewMsgSendHeaders(),
		},
		{
			"OnGetInitState",
			wire.NewMsgGetInitState(),
		},
		{
			"OnInitState",
			wire.NewMsgInitState(),
		},
		{
			"OnGetCFiltersV2",
			wire.NewMsgGetCFsV2(&chainhash.Hash{}, &chainhash.Hash{}),
		},
		{
			"OnCFiltersV2",
			wire.NewMsgCFiltersV2([]wire.MsgCFilterV2{}),
		},
	}
	t.Logf("Running %d tests", len(tests))
	for _, test := range tests {
		// Queue the test message
		outPeer.QueueMessage(test.msg, nil)
		select {
		case <-ok:
		case <-time.After(time.Second * 1):
			t.Errorf("TestPeerListeners: %s timeout", test.listener)
			return
		}
	}
	inPeer.Disconnect()
	outPeer.Disconnect()
}

// TestOldProtocolVersion ensures that peers with protocol versions older than
// the minimum required version are disconnected.
func TestOldProtocolVersion(t *testing.T) {
	version := make(chan wire.Message, 1)
	verack := make(chan struct{}, 1)
	peerCfg := &Config{
		ProtocolVersion: wire.RemoveRejectVersion - 1,
		Listeners: MessageListeners{
			OnVersion: func(p *Peer, msg *wire.MsgVersion) {
				version <- msg
			},
			OnVerAck: func(p *Peer, msg *wire.MsgVerAck) {
				verack <- struct{}{}
			},
		},
		UserAgentName:    "peer",
		UserAgentVersion: "1.0",
		Net:              wire.MainNet,
		Services:         wire.SFNodeNetwork,
	}
	inConn, outConn := pipe(
		&conn{raddr: "10.0.0.1:8333"},
		&conn{raddr: "10.0.0.2:8333"},
	)
	inPeer := NewInboundPeer(peerCfg)
	inPeer.AssociateConnection(inConn)
	defer inPeer.Disconnect()

	peerCfg.Listeners = MessageListeners{}
	outPeer, err := NewOutboundPeer(peerCfg, "10.0.0.2:8333")
	if err != nil {
		t.Errorf("NewOutboundPeer: unexpected err %v", err)
		return
	}
	outPeer.AssociateConnection(outConn)
	defer outPeer.Disconnect()

	select {
	case <-version:
	case <-time.After(time.Second * 1):
		t.Fatal("version timeout")
	}

	// Ensure the inbound peer is disconnected and does not receive a verack
	// from the outbound side.
	select {
	case <-inPeer.quit:
	case <-verack:
		t.Fatal("unexpected verack from outbound peer")
	case <-time.After(time.Second * 1):
		t.Fatal("inbound peer disconnect timeout")
	}

	// Ensure the outbound peer is disconnected.
	select {
	case <-outPeer.quit:
	case <-time.After(time.Second * 1):
		t.Fatal("outbound peer disconnect timeout")
	}
}

// TestOutboundPeer tests that the outbound peer works as expected.
func TestOutboundPeer(t *testing.T) {
	peerCfg := &Config{
		NewestBlock: func() (*chainhash.Hash, int64, error) {
			return nil, 0, errors.New("newest block not found")
		},
		UserAgentName:    "peer",
		UserAgentVersion: "1.0",
		Net:              wire.MainNet,
		Services:         0,
	}

	r, w := io.Pipe()
	c := &conn{raddr: "10.0.0.1:8333", WriteCloser: w, ReadCloser: r}

	p, err := NewOutboundPeer(peerCfg, "10.0.0.1:8333")
	if err != nil {
		t.Errorf("NewOutboundPeer: unexpected err - %v\n", err)
		return
	}

	// Test trying to connect twice.
	p.AssociateConnection(c)
	p.AssociateConnection(c)

	disconnected := make(chan struct{})
	go func() {
		p.WaitForDisconnect()
		disconnected <- struct{}{}
	}()

	select {
	case <-disconnected:
		close(disconnected)
	case <-time.After(time.Second):
		t.Fatal("Peer did not automatically disconnect.")
	}

	if p.Connected() {
		t.Fatalf("Should not be connected as NewestBlock produces error.")
	}

	// Test Queue Inv
	fakeBlockHash := &chainhash.Hash{0: 0x00, 1: 0x01}
	fakeInv := wire.NewInvVect(wire.InvTypeBlock, fakeBlockHash)

	// Should be noops as the peer could not connect.
	p.QueueInventory(fakeInv)
	p.AddKnownInventory(fakeInv)
	p.QueueInventory(fakeInv)

	fakeMsg := wire.NewMsgVerAck()
	p.QueueMessage(fakeMsg, nil)
	done := make(chan struct{})
	p.QueueMessage(fakeMsg, done)
	<-done
	p.Disconnect()

	// Test NewestBlock
	newestBlock := func() (*chainhash.Hash, int64, error) {
		hashStr := "14a0810ac680a3eb3f82edc878cea25ec41d6b790744e5daeef"
		hash, err := chainhash.NewHashFromStr(hashStr)
		if err != nil {
			return nil, 0, err
		}
		return hash, 234439, nil
	}

	peerCfg.NewestBlock = newestBlock
	r1, w1 := io.Pipe()
	c1 := &conn{raddr: "10.0.0.1:8333", WriteCloser: w1, ReadCloser: r1}
	p1, err := NewOutboundPeer(peerCfg, "10.0.0.1:8333")
	if err != nil {
		t.Errorf("NewOutboundPeer: unexpected err - %v\n", err)
		return
	}
	p1.AssociateConnection(c1)

	// Test Queue Inv after connection
	p1.QueueInventory(fakeInv)
	p1.Disconnect()

	// Test testnet
	peerCfg.Net = wire.TestNet3
	peerCfg.Services = wire.SFNodeBloom
	r2, w2 := io.Pipe()
	c2 := &conn{raddr: "10.0.0.1:8333", WriteCloser: w2, ReadCloser: r2}
	p2, err := NewOutboundPeer(peerCfg, "10.0.0.1:8333")
	if err != nil {
		t.Errorf("NewOutboundPeer: unexpected err - %v\n", err)
		return
	}
	p2.AssociateConnection(c2)

	// Test PushXXX
	var addrs []*wire.NetAddress
	for i := 0; i < 5; i++ {
		na := wire.NetAddress{}
		addrs = append(addrs, &na)
	}
	if _, err := p2.PushAddrMsg(addrs); err != nil {
		t.Errorf("PushAddrMsg: unexpected err %v\n", err)
		return
	}
	if err := p2.PushGetBlocksMsg(nil, &chainhash.Hash{}); err != nil {
		t.Errorf("PushGetBlocksMsg: unexpected err %v\n", err)
		return
	}
	if err := p2.PushGetHeadersMsg(nil, &chainhash.Hash{}); err != nil {
		t.Errorf("PushGetHeadersMsg: unexpected err %v\n", err)
		return
	}

	// Test Queue Messages
	p2.QueueMessage(wire.NewMsgGetAddr(), nil)
	p2.QueueMessage(wire.NewMsgPing(1), nil)
	p2.QueueMessage(wire.NewMsgMemPool(), nil)
	p2.QueueMessage(wire.NewMsgGetData(), nil)
	p2.QueueMessage(wire.NewMsgGetHeaders(), nil)
	p2.QueueMessage(wire.NewMsgFeeFilter(20000), nil)

	p2.Disconnect()
}

// TestDuplicateVersionMsg ensures that receiving a version message after one
// has already been received results in the peer being disconnected.
func TestDuplicateVersionMsg(t *testing.T) {
	// Create a pair of peers that are connected to each other using a fake
	// connection.
	verack := make(chan struct{})
	peerCfg := &Config{
		Listeners: MessageListeners{
			OnVerAck: func(p *Peer, msg *wire.MsgVerAck) {
				verack <- struct{}{}
			},
		},
		UserAgentName:    "peer",
		UserAgentVersion: "1.0",
		Net:              wire.MainNet,
		Services:         0,
	}
	inConn, outConn := pipe(
		&conn{laddr: "10.0.0.1:9108", raddr: "10.0.0.2:9108"},
		&conn{laddr: "10.0.0.2:9108", raddr: "10.0.0.1:9108"},
	)
	outPeer, err := NewOutboundPeer(peerCfg, inConn.laddr)
	if err != nil {
		t.Fatalf("NewOutboundPeer: unexpected err: %v\n", err)
	}
	outPeer.AssociateConnection(outConn)
	inPeer := NewInboundPeer(peerCfg)
	inPeer.AssociateConnection(inConn)

	// Wait for the veracks from the initial protocol version negotiation.
	for i := 0; i < 2; i++ {
		select {
		case <-verack:
		case <-time.After(time.Second):
			t.Fatal("verack timeout")
		}
	}

	// Queue a duplicate version message from the outbound peer and wait until
	// it is sent.
	done := make(chan struct{})
	outPeer.QueueMessage(&wire.MsgVersion{}, done)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("send duplicate version timeout")
	}

	// Ensure the peer that is the recipient of the duplicate version closes the
	// connection.
	disconnected := make(chan struct{}, 1)
	go func() {
		inPeer.WaitForDisconnect()
		disconnected <- struct{}{}
	}()
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("peer did not disconnect")
	}
}

// TestNetFallback ensures the network is set to the expected value in
// accordance with the parameters.
func TestNetFallback(t *testing.T) {
	cfg := Config{
		NewestBlock: func() (*chainhash.Hash, int64, error) {
			return nil, 0, errors.New("newest block not found")
		},
		UserAgentName:    "peer",
		UserAgentVersion: "1.0",
		Services:         0,
	}

	// Ensure testnet is used when the network is not specified.
	p := NewInboundPeer(&cfg)
	if p.cfg.Net != wire.TestNet3 {
		t.Fatalf("default network is %v instead of testnet3", p.cfg.Net)
	}

	// Ensure network is set to the explicitly specified value.
	cfg.Net = wire.SimNet
	p = NewInboundPeer(&cfg)
	if p.cfg.Net != wire.SimNet {
		t.Fatalf("explicit network is %v instead of %v", p.cfg.Net, wire.SimNet)
	}
}

// TestUpdateLastBlockHeight ensures the last block height is set properly
// during the initial version negotiation and is only allowed to advance to
// higher values via the associated update function.
func TestUpdateLastBlockHeight(t *testing.T) {
	// Create a pair of peers that are connected to each other using a fake
	// connection and the remote peer starting at height 100.
	const remotePeerHeight = 100
	verack := make(chan struct{})
	peerCfg := Config{
		Listeners: MessageListeners{
			OnVerAck: func(p *Peer, msg *wire.MsgVerAck) {
				verack <- struct{}{}
			},
		},
		UserAgentName:    "peer",
		UserAgentVersion: "1.0",
		Net:              wire.MainNet,
		Services:         0,
	}
	remotePeerCfg := peerCfg
	remotePeerCfg.NewestBlock = func() (*chainhash.Hash, int64, error) {
		return &chainhash.Hash{}, remotePeerHeight, nil
	}
	inConn, outConn := pipe(
		&conn{laddr: "10.0.0.1:9108", raddr: "10.0.0.2:9108"},
		&conn{laddr: "10.0.0.2:9108", raddr: "10.0.0.1:9108"},
	)
	localPeer, err := NewOutboundPeer(&peerCfg, inConn.laddr)
	if err != nil {
		t.Fatalf("NewOutboundPeer: unexpected err: %v\n", err)
	}
	localPeer.AssociateConnection(outConn)
	inPeer := NewInboundPeer(&remotePeerCfg)
	inPeer.AssociateConnection(inConn)

	// Wait for the veracks from the initial protocol version negotiation.
	for i := 0; i < 2; i++ {
		select {
		case <-verack:
		case <-time.After(time.Second):
			t.Fatal("verack timeout")
		}
	}

	// Ensure the latest block height starts at the value reported by the remote
	// peer via its version message.
	if height := localPeer.LastBlock(); height != remotePeerHeight {
		t.Fatalf("wrong starting height - got %d, want %d", height,
			remotePeerHeight)
	}

	// Ensure the latest block height is not allowed to go backwards.
	localPeer.UpdateLastBlockHeight(remotePeerHeight - 1)
	if height := localPeer.LastBlock(); height != remotePeerHeight {
		t.Fatalf("height allowed to go backwards - got %d, want %d", height,
			remotePeerHeight)
	}

	// Ensure the latest block height is allowed to advance.
	localPeer.UpdateLastBlockHeight(remotePeerHeight + 1)
	if height := localPeer.LastBlock(); height != remotePeerHeight+1 {
		t.Fatalf("height not allowed to advance - got %d, want %d", height,
			remotePeerHeight+1)
	}
}

func init() {
	// Allow self connection when running the tests.
	allowSelfConns = true
}
