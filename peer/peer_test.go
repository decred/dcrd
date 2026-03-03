// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

import (
	"context"
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
func pipe(inAddr, outAddr string) (*conn, *conn) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	c1 := &conn{laddr: inAddr, raddr: outAddr, WriteCloser: w1, ReadCloser: r2}
	c2 := &conn{laddr: outAddr, raddr: inAddr, WriteCloser: w2, ReadCloser: r1}
	return c1, c2
}

// mockPeerConfig returns a base mock peer config to use throughout the tests.
func mockPeerConfig() *Config {
	return &Config{
		UserAgentName:    "peer",
		UserAgentVersion: "1.0",
		Net:              wire.MainNet,
		Services:         wire.SFNodeNetwork,
	}
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

// runPeersAsync invokes the [Peer.Start] method on the passed peers in separate
// goroutines and returns a cancelable context and wait group the caller can use
// to shutdown the peers and wait for clean shutdown.
func runPeersAsync(t *testing.T, peers ...*Peer) (context.CancelFunc, *sync.WaitGroup) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(len(peers))
	for _, peer := range peers {
		go func(peer *Peer) {
			if err := peer.Start(); err != nil {
				if !errors.Is(err, io.EOF) {
					t.Logf("failed to start peer %s: %v", peer.remoteAddr, err)
				}
			}
			select {
			case <-ctx.Done():
				peer.Disconnect()
			case <-peer.quit:
			}
			wg.Done()
		}(peer)
	}
	return cancel, &wg
}

// testPeerState ensures the flags and state of the provided peer match the
// given stats.
func testPeerState(t *testing.T, p *Peer, s peerStats) {
	t.Helper()

	if got := p.UserAgent(); got != s.wantUserAgent {
		t.Fatalf("wrong UserAgent - got %v, want %v", got, s.wantUserAgent)
	}

	if got := p.Services(); got != s.wantServices {
		t.Fatalf("wrong Services - got %v, want %v", got, s.wantServices)
	}

	if got := p.LastPingTime(); !got.Equal(s.wantLastPingTime) {
		t.Fatalf("wrong LastPingTime - got %v, want %v", got, s.wantLastPingTime)
	}

	if got := p.LastPingNonce(); got != s.wantLastPingNonce {
		t.Fatalf("wrong LastPingNonce - got %v, want %v", got, s.wantLastPingNonce)
	}

	if got := p.LastPingMicros(); got != s.wantLastPingMicros {
		t.Fatalf("wrong LastPingMicros - got %v, want %v", got, s.wantLastPingMicros)
	}

	if got := p.VerAckReceived(); got != s.wantVerAckReceived {
		t.Fatalf("wrong VerAckReceived - got %v, want %v", got, s.wantVerAckReceived)
	}

	if got := p.VersionKnown(); got != s.wantVersionKnown {
		t.Fatalf("wrong VersionKnown - got %v, want %v", got, s.wantVersionKnown)
	}

	if got := p.ProtocolVersion(); got != s.wantProtocolVersion {
		t.Fatalf("wrong ProtocolVersion - got %v, want %v", got, s.wantProtocolVersion)
	}

	if got := p.LastBlock(); got != s.wantLastBlock {
		t.Fatalf("wrong LastBlock - got %v, want %v", got, s.wantLastBlock)
	}

	// Allow for a deviation of 1s, as the second may tick when the message is
	// in transit and the protocol doesn't support any further precision.
	if got := p.TimeOffset(); got != s.wantTimeOffset && got != s.wantTimeOffset-1 {
		t.Fatalf("wrong TimeOffset - got %v, want %v or %v", got,
			s.wantTimeOffset, s.wantTimeOffset-1)
	}

	if got := p.BytesSent(); got != s.wantBytesSent {
		t.Fatalf("wrong BytesSent - got %v, want %v", got, s.wantBytesSent)
	}

	if got := p.BytesReceived(); got != s.wantBytesReceived {
		t.Fatalf("wrong BytesReceived - got %v, want %v", got, s.wantBytesReceived)
	}

	if got := p.StartingHeight(); got != s.wantStartingHeight {
		t.Fatalf("wrong StartingHeight - got %v, want %v", got, s.wantStartingHeight)
	}

	if got := p.Connected(); got != s.wantConnected {
		t.Fatalf("wrong Connected - got %v, want %v", got, s.wantConnected)
	}

	stats := p.StatsSnapshot()

	if got := p.ID(); got != stats.ID {
		t.Fatalf("wrong ID - got %v, want %v", got, stats.ID)
	}

	if got := p.Addr(); got != stats.Addr {
		t.Fatalf("wrong Addr - got %v, want %v", got, stats.Addr)
	}

	if got := p.LastSend(); got != stats.LastSend {
		t.Fatalf("wrong LastSend - got %v, want %v", got, stats.LastSend)
	}

	if got := p.LastRecv(); got != stats.LastRecv {
		t.Fatalf("wrong LastRecv - got %v, want %v", got, stats.LastRecv)
	}
}

// TestPeerConnection tests connection between inbound and outbound peers.
func TestPeerConnection(t *testing.T) {
	var pause sync.Mutex
	verack := make(chan struct{})
	peerCfg := mockPeerConfig()
	peerCfg.Listeners = MessageListeners{
		OnVerAck: func(p *Peer, msg *wire.MsgVerAck) {
			verack <- struct{}{}
		},
		OnWrite: func(p *Peer, bytesWritten int, msg wire.Message, err error) {
			if _, ok := msg.(*wire.MsgVerAck); ok {
				verack <- struct{}{}
			}
			pause.Lock()
			// Needed to squash empty critical section lint errors.
			_ = p
			pause.Unlock()
		},
	}
	peerCfg.Services = 0
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
		setup func() (*Peer, *Peer, context.CancelFunc, *sync.WaitGroup, error)
	}{{
		"basic handshake",
		func() (*Peer, *Peer, context.CancelFunc, *sync.WaitGroup, error) {
			inConn, outConn := pipe("10.0.0.1:9108", "10.0.0.2:9108")
			inPeer := NewInboundPeer(peerCfg, inConn)
			outPeer := NewOutboundPeer(peerCfg, outConn.RemoteAddr(), outConn)
			cancel, wg := runPeersAsync(t, inPeer, outPeer)

			for i := 0; i < 4; i++ {
				select {
				case <-verack:
				case <-time.After(time.Second):
					cancel()
					return nil, nil, nil, nil, errors.New("verack timeout")
				}
			}
			return inPeer, outPeer, cancel, wg, nil
		},
	}, {
		"socks proxy",
		func() (*Peer, *Peer, context.CancelFunc, *sync.WaitGroup, error) {
			inConn, outConn := pipe("10.0.0.1:9108", "10.0.0.2:9108")
			inConn.proxy = true
			inPeer := NewInboundPeer(peerCfg, inConn)
			outPeer := NewOutboundPeer(peerCfg, outConn.RemoteAddr(), outConn)
			cancel, wg := runPeersAsync(t, inPeer, outPeer)

			for i := 0; i < 4; i++ {
				select {
				case <-verack:
				case <-time.After(time.Second):
					cancel()
					return nil, nil, nil, nil, errors.New("verack timeout")
				}
			}
			return inPeer, outPeer, cancel, wg, nil
		},
	}}
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		inPeer, outPeer, cancel, wg, err := test.setup()
		if err != nil {
			t.Fatalf("setup #%d: unexpected err: %v", i, err)
		}

		pause.Lock()
		testPeerState(t, inPeer, wantStats)
		testPeerState(t, outPeer, wantStats)
		pause.Unlock()

		cancel()
		wg.Wait()
	}
}

// TestPeerListeners tests that the peer listeners are called as expected.
func TestPeerListeners(t *testing.T) {
	verack := make(chan struct{}, 1)
	ok := make(chan wire.Message, 20)
	peerCfg := mockPeerConfig()
	peerCfg.Listeners = MessageListeners{
		OnGetAddr: func(p *Peer, msg *wire.MsgGetAddr) {
			ok <- msg
		},
		OnAddr: func(p *Peer, msg *wire.MsgAddr) {
			ok <- msg
		},
		OnAddrV2: func(p *Peer, msg *wire.MsgAddrV2) {
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
	}
	inConn, outConn := pipe("10.0.0.1:9108", "10.0.0.2:9108")
	inPeer := NewInboundPeer(peerCfg, inConn)
	peerCfg.Listeners = MessageListeners{
		OnVerAck: func(p *Peer, msg *wire.MsgVerAck) {
			verack <- struct{}{}
		},
	}
	outPeer := NewOutboundPeer(peerCfg, outConn.RemoteAddr(), outConn)
	cancel, wg := runPeersAsync(t, inPeer, outPeer)
	defer wg.Wait()
	defer cancel()

	for i := 0; i < 2; i++ {
		select {
		case <-verack:
		case <-time.After(time.Second * 1):
			t.Fatal("TestPeerListeners: verack timeout")
		}
	}

	select {
	case <-ok:
	case <-time.After(time.Second * 1):
		t.Fatal("TestPeerListeners: version timeout")
	}

	const pver = wire.ProtocolVersion
	tests := []struct {
		listener string
		msg      wire.Message
		pver     uint32
	}{{
		listener: "OnGetAddr",
		msg:      wire.NewMsgGetAddr(),
		pver:     pver,
	}, {
		listener: "OnAddr",
		msg:      wire.NewMsgAddr(),
		pver:     wire.AddrV2Version - 1,
	}, {
		listener: "OnAddrV2",
		msg: func() *wire.MsgAddrV2 {
			ipv4Localhost := net.ParseIP("127.0.0.1").To4()
			addr := wire.NewNetAddressV2(wire.IPv4Address, ipv4Localhost, 8333,
				time.Now(), wire.SFNodeNetwork)
			msg := wire.NewMsgAddrV2([]wire.NetAddressV2{addr})
			return msg
		}(),
		pver: pver,
	}, {
		listener: "OnPing",
		msg:      wire.NewMsgPing(42),
		pver:     pver,
	}, {
		listener: "OnPong",
		msg:      wire.NewMsgPong(42),
		pver:     pver,
	}, {
		listener: "OnMemPool",
		msg:      wire.NewMsgMemPool(),
		pver:     pver,
	}, {
		listener: "OnTx",
		msg:      wire.NewMsgTx(),
		pver:     pver,
	}, {
		listener: "OnBlock",
		msg: wire.NewMsgBlock(wire.NewBlockHeader(0, &chainhash.Hash{},
			&chainhash.Hash{}, &chainhash.Hash{}, 1, [6]byte{},
			1, 1, 1, 1, 1, 1, 1, 1, 1, [32]byte{},
			binary.LittleEndian.Uint32([]byte{0xb0, 0x1d, 0xfa, 0xce}))),
		pver: pver,
	}, {
		listener: "OnInv",
		msg:      wire.NewMsgInv(),
		pver:     pver,
	}, {
		listener: "OnHeaders",
		msg:      wire.NewMsgHeaders(),
		pver:     pver,
	}, {
		listener: "OnNotFound",
		msg:      wire.NewMsgNotFound(),
		pver:     pver,
	}, {
		listener: "OnGetData",
		msg:      wire.NewMsgGetData(),
		pver:     pver,
	}, {
		listener: "OnGetBlocks",
		msg:      wire.NewMsgGetBlocks(&chainhash.Hash{}),
		pver:     pver,
	}, {
		listener: "OnGetHeaders",
		msg:      wire.NewMsgGetHeaders(),
		pver:     pver,
	}, {
		listener: "OnGetCFilter",
		msg: wire.NewMsgGetCFilter(&chainhash.Hash{},
			wire.GCSFilterRegular),
		pver: pver,
	}, {
		listener: "OnGetCFHeaders",
		msg:      wire.NewMsgGetCFHeaders(),
		pver:     pver,
	}, {
		listener: "OnGetCFTypes",
		msg:      wire.NewMsgGetCFTypes(),
		pver:     pver,
	}, {
		listener: "OnCFilter",
		msg: wire.NewMsgCFilter(&chainhash.Hash{},
			wire.GCSFilterRegular, []byte("payload")),
		pver: pver,
	}, {
		listener: "OnCFHeaders",
		msg:      wire.NewMsgCFHeaders(),
		pver:     pver,
	}, {
		listener: "OnCFTypes",
		msg: wire.NewMsgCFTypes([]wire.FilterType{
			wire.GCSFilterRegular, wire.GCSFilterExtended,
		}),
		pver: pver,
	}, {
		listener: "OnFeeFilter",
		msg:      wire.NewMsgFeeFilter(15000),
		pver:     pver,
	}, {
		listener: "OnGetCFilterV2",
		msg:      wire.NewMsgGetCFilterV2(&chainhash.Hash{}),
		pver:     pver,
	}, {
		listener: "OnCFilterV2",
		msg:      wire.NewMsgCFilterV2(&chainhash.Hash{}, nil, 0, nil),
		pver:     pver,
	}, {
		// only one version message is allowed
		// only one verack message is allowed
		listener: "OnSendHeaders",
		msg:      wire.NewMsgSendHeaders(),
		pver:     pver,
	}, {
		listener: "OnGetInitState",
		msg:      wire.NewMsgGetInitState(),
		pver:     pver,
	}, {
		listener: "OnInitState",
		msg:      wire.NewMsgInitState(),
		pver:     pver,
	}, {
		listener: "OnGetCFiltersV2",
		msg:      wire.NewMsgGetCFsV2(&chainhash.Hash{}, &chainhash.Hash{}),
		pver:     pver,
	}, {
		listener: "OnCFiltersV2",
		msg:      wire.NewMsgCFiltersV2([]wire.MsgCFilterV2{}),
		pver:     pver,
	}}

	t.Logf("Running %d tests", len(tests))
	prevPver := inPeer.ProtocolVersion()
	changeProtocolVer := func(pver uint32) {
		inPeer.flagsMtx.Lock()
		inPeer.protocolVersion = pver
		inPeer.flagsMtx.Unlock()
		outPeer.flagsMtx.Lock()
		outPeer.protocolVersion = pver
		outPeer.flagsMtx.Unlock()
		// The async read might already be blocking with the previous protocol
		// version.  Send an extra message to guarantee the subsequent reads use
		// the updated one.
		outPeer.QueueMessage(wire.NewMsgPing(0), nil)
		select {
		case <-ok:
		case <-time.After(time.Second * 1):
			t.Fatal("TestPeerListeners: pver change timeout")
		}
	}
	for _, test := range tests {
		if prevPver != test.pver {
			changeProtocolVer(test.pver)
			prevPver = test.pver
		}
		// Queue the test message
		outPeer.QueueMessage(test.msg, nil)
		select {
		case <-ok:
		case <-time.After(time.Second * 1):
			t.Fatalf("%s timeout", test.listener)
		}
	}
}

// TestOldProtocolVersion ensures that peers with protocol versions older than
// the minimum required version are disconnected.
func TestOldProtocolVersion(t *testing.T) {
	const minVer = wire.RemoveRejectVersion
	version := make(chan wire.Message, 1)
	verack := make(chan struct{}, 1)
	peerCfg := mockPeerConfig()
	peerCfg.ProtocolVersion = minVer - 1
	peerCfg.Listeners = MessageListeners{
		OnVersion: func(p *Peer, msg *wire.MsgVersion) {
			version <- msg
		},
		OnVerAck: func(p *Peer, msg *wire.MsgVerAck) {
			verack <- struct{}{}
		},
	}
	inConn, outConn := pipe("10.0.0.1:9108", "10.0.0.2:9108")
	inPeer := NewInboundPeer(peerCfg, inConn)
	peerCfg.Listeners = MessageListeners{}
	outPeer := NewOutboundPeer(peerCfg, outConn.RemoteAddr(), outConn)
	cancel, wg := runPeersAsync(t, inPeer, outPeer)
	defer wg.Wait()
	defer cancel()

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

// TestNoNewestBlock ensures peers are disconnected due to an error returned by
// the caller-provided newest block callback.
func TestNoNewestBlock(t *testing.T) {
	// Create a pair of peers that connect to each other using a fake conn such
	// that the inbound peer has a newest block callback that errors.
	peerCfg := mockPeerConfig()
	inConn, outConn := pipe("10.0.0.1:9108", "10.0.0.1:9108")
	outPeer := NewOutboundPeer(peerCfg, outConn.RemoteAddr(), outConn)
	peerCfg.NewestBlock = func() (*chainhash.Hash, int64, error) {
		return nil, 0, errors.New("newest block not found")
	}
	inPeer := NewInboundPeer(peerCfg, inConn)
	cancel, wg := runPeersAsync(t, inPeer, outPeer)
	defer wg.Wait()
	defer cancel()

	// Ensure the inbound peer disconnects due to the error.
	disconnected := make(chan struct{})
	go func() {
		inPeer.WaitForDisconnect()
		disconnected <- struct{}{}
	}()

	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("peer did not automatically disconnect")
	}

	if inPeer.Connected() {
		t.Fatal("inbound peer should not be connected")
	}

	// Repeat, but in the other direction so the outbound peer has the error.
	inConn, outConn = pipe("10.0.0.1:9108", "10.0.0.1:9108")
	outPeer = NewOutboundPeer(peerCfg, outConn.RemoteAddr(), outConn)
	peerCfg.NewestBlock = nil
	inPeer = NewInboundPeer(peerCfg, inConn)
	cancel, wg = runPeersAsync(t, inPeer, outPeer)
	defer wg.Wait()
	defer cancel()

	// Ensure the outbound peer disconnects due to the error.
	disconnected = make(chan struct{})
	go func() {
		outPeer.WaitForDisconnect()
		disconnected <- struct{}{}
	}()

	select {
	case <-disconnected:
		close(disconnected)
	case <-time.After(time.Second):
		t.Fatal("peer did not automatically disconnect")
	}

	if outPeer.Connected() {
		t.Fatal("outbound peer should not be connected")
	}
}

// TestDuplicateVersionMsg ensures that receiving a version message after one
// has already been received results in the peer being disconnected.
func TestDuplicateVersionMsg(t *testing.T) {
	// Create a pair of peers that are connected to each other using a fake
	// connection.
	verack := make(chan struct{})
	peerCfg := mockPeerConfig()
	peerCfg.Listeners.OnVerAck = func(p *Peer, msg *wire.MsgVerAck) {
		verack <- struct{}{}
	}
	inConn, outConn := pipe("10.0.0.1:9108", "10.0.0.1:9108")
	outPeer := NewOutboundPeer(peerCfg, outConn.RemoteAddr(), outConn)
	inPeer := NewInboundPeer(peerCfg, inConn)
	cancel, wg := runPeersAsync(t, inPeer, outPeer)
	defer wg.Wait()
	defer cancel()

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
		close(disconnected)
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
	inConn, outConn := pipe("10.0.0.1:9108", "10.0.0.2:9108")
	defer inConn.Close()
	defer outConn.Close()

	// Ensure testnet is used when the network is not specified.
	p := NewInboundPeer(&cfg, inConn)
	if p.cfg.Net != wire.TestNet3 {
		t.Fatalf("default network is %v instead of testnet3", p.cfg.Net)
	}

	// Ensure network is set to the explicitly specified value.
	cfg.Net = wire.SimNet
	p = NewInboundPeer(&cfg, inConn)
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
	inConn, outConn := pipe("10.0.0.1:9108", "10.0.0.2:9108")
	localPeer := NewOutboundPeer(&peerCfg, outConn.RemoteAddr(), outConn)
	inPeer := NewInboundPeer(&remotePeerCfg, inConn)
	cancel, wg := runPeersAsync(t, localPeer, inPeer)
	defer wg.Wait()
	defer cancel()

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

// TestPushAddrV2Msg ensures that the PushAddrV2Msg returns the expected
// number of addresses.
func TestPushAddrV2Msg(t *testing.T) {
	addr := wire.NewNetAddressV2(wire.IPv4Address, net.ParseIP("192.168.0.1"),
		8333, time.Now(), wire.SFNodeNetwork)

	tests := []struct {
		name        string
		addrs       []wire.NetAddressV2
		wantSentLen int
	}{{
		name:        "nil address list",
		addrs:       nil,
		wantSentLen: 0,
	}, {
		name:        "empty address list",
		addrs:       []wire.NetAddressV2{},
		wantSentLen: 0,
	}, {
		name:        "single address",
		addrs:       []wire.NetAddressV2{addr},
		wantSentLen: 1,
	}, {
		name: "multiple addresses under limit",
		addrs: func() []wire.NetAddressV2 {
			addrs := make([]wire.NetAddressV2, 10)
			for i := range addrs {
				addrs[i] = addr
			}
			return addrs
		}(),
		wantSentLen: 10,
	}, {
		name: "addresses over MaxAddrPerV2Msg limit",
		addrs: func() []wire.NetAddressV2 {
			addrs := make([]wire.NetAddressV2, wire.MaxAddrPerV2Msg+100)
			for i := range addrs {
				addrs[i] = addr
			}
			return addrs
		}(),
		wantSentLen: wire.MaxAddrPerV2Msg,
	}}

	// Create a mock connection.
	inConn, outConn := pipe("10.0.0.1:9108", "10.0.0.2:9108")

	// Create a peer with the connection.
	peerCfg := mockPeerConfig()
	inPeer := NewInboundPeer(peerCfg, inConn)
	outPeer := NewOutboundPeer(peerCfg, outConn.RemoteAddr(), outConn)
	cancel, wg := runPeersAsync(t, inPeer, outPeer)
	defer wg.Wait()
	defer cancel()

	for _, test := range tests {
		// Test the PushAddrV2Msg function.
		sent := outPeer.PushAddrV2Msg(test.addrs)

		// Check the number of addresses sent.
		if got := len(sent); got != test.wantSentLen {
			t.Errorf("%s: expected %d addresses sent, got %d", test.name,
				test.wantSentLen, got)
		}
	}
}

func init() {
	// Allow self connection when running the tests.
	allowSelfConns = true
}
