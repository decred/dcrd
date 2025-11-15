// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package banmanager

import (
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/decred/dcrd/peer/v4"
	"github.com/decred/dcrd/wire"
	"github.com/decred/go-socks/socks"
)

// TestBanPeer tests ban manager peer banning functionality.
func TestBanPeer(t *testing.T) {
	bcfg := &Config{
		DisableBanning: false,
		BanThreshold:   100,
		BanDuration:    time.Millisecond * 500,
		MaxPeers:       10,
		WhiteList:      []net.IPNet{},
	}

	bmgr := NewBanManager(bcfg)

	peerCfg := &peer.Config{
		UserAgentName:    "peer",
		UserAgentVersion: "1.0",
		Net:              wire.MainNet,
		Services:         0,
	}

	// Add peer A, B and C.
	pA, err := peer.NewOutboundPeer(peerCfg, "10.0.0.1:8333")
	if err != nil {
		t.Fatalf("NewOutboundPeer: unexpected err - %v\n", err)
	}

	err = bmgr.AddPeer(pA)
	if err != nil {
		t.Fatalf("unexpected err -%v\n", err)
	}

	pB, err := peer.NewOutboundPeer(peerCfg, "10.0.0.2:8333")
	if err != nil {
		t.Fatalf("NewOutboundPeer: unexpected err - %v\n", err)
	}

	err = bmgr.AddPeer(pB)
	if err != nil {
		t.Fatalf("unexpected err -%v\n", err)
	}

	pC, err := peer.NewOutboundPeer(peerCfg, "10.0.0.3:8333")
	if err != nil {
		t.Errorf("NewOutboundPeer: unexpected err - %v\n", err)
		return
	}

	err = bmgr.AddPeer(pC)
	if err != nil {
		t.Fatalf("unexpected err -%v\n", err)
	}

	if len(bmgr.peers) != 3 {
		t.Fatalf("expected 3 tracked peers, got %d", len(bmgr.peers))
	}

	// Remove disconnected peer C.
	bmgr.RemovePeer(pC)

	bmgr.mtx.Lock()
	if len(bmgr.peers) != 2 {
		bmgr.mtx.Unlock()
		t.Fatalf("expected 2 tracked peers, got %d", len(bmgr.peers))
	}
	bmgr.mtx.Unlock()

	// Ensure the ban manager updates the correct peer's ban score.
	expectedBBanScore := uint32(0)
	peerB := bmgr.lookupPeer(pB)
	if bmgr.BanScore(pB) != expectedBBanScore {
		t.Fatalf("expected an unchanged ban score for peer B, got %d",
			peerB.banScore.Int())
	}

	expectedABanScore := uint32(50)
	bmgr.AddBanScore(pA, expectedABanScore, 0, "testing")
	peerA := bmgr.lookupPeer(pA)
	if bmgr.BanScore(pA) != expectedABanScore {
		t.Fatalf("expected a ban score of %d for peer A, got %d",
			expectedABanScore, peerA.banScore.Int())
	}

	// Ban peer A by exceeding the ban threshold.
	bmgr.AddBanScore(pA, 120, 0, "testing")

	peerA = bmgr.lookupPeer(pA)
	if peerA != nil {
		t.Fatal("peer A still exists in the manager")
	}

	// Outrightly ban peer B.
	bmgr.BanPeer(pB)

	peerB = bmgr.lookupPeer(pB)
	if peerB != nil {
		t.Fatal("peer B still exists in the manager")
	}

	bmgr.mtx.Lock()
	if len(bmgr.peers) != 0 {
		bmgr.mtx.Unlock()
		t.Fatalf("expected no tracked peers, got %d", len(bmgr.peers))
	}
	bmgr.mtx.Unlock()

	// Ensure there are two banned peers being tracked by the manager.
	bmgr.mtx.Lock()
	if len(bmgr.banned) != 2 {
		bmgr.mtx.Unlock()
		t.Fatalf("expected two tracked banned peers, got %d", len(bmgr.banned))
	}
	bmgr.mtx.Unlock()

	// Ensure re-adding a banned peer fails if it is before the ban period ends.
	err = bmgr.AddPeer(pA)
	if err == nil {
		t.Fatalf("expected a ban error \n")
	}

	bmgr.mtx.Lock()
	if len(bmgr.peers) != 0 {
		bmgr.mtx.Unlock()
		t.Fatalf("expected no tracked peers, got %d", len(bmgr.peers))
	}
	bmgr.mtx.Unlock()

	// Wait for the ban period to end.
	time.Sleep(time.Millisecond * 500)

	// Ensure re-adding a banned peer succeeds if it is after the ban period.
	err = bmgr.AddPeer(pA)
	if err != nil {
		t.Fatalf("unexpected err -%v\n", err)
	}

	bmgr.mtx.Lock()
	if len(bmgr.peers) != 1 {
		bmgr.mtx.Unlock()
		t.Fatalf("expected a tracked peer, got %d", len(bmgr.peers))
	}
	bmgr.mtx.Unlock()
}

// conn mocks a network connection by implementing the net.Conn interface.  It
// is used to test peer connection without actually opening a network
// connection.
type conn struct {
	io.Reader
	io.Writer
	io.Closer

	// local network, address for the connection.
	lnet, laddr string

	// remote network, address for the connection.
	rnet, raddr string

	// mocks socks proxy if true.
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
	return nil
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

func TestPeerWhitelist(t *testing.T) {
	ip := net.ParseIP("10.0.0.1")
	ipnet := net.IPNet{
		IP:   ip,
		Mask: net.CIDRMask(32, 32),
	}
	whitelist := []net.IPNet{ipnet}

	bcfg := &Config{
		DisableBanning: false,
		BanThreshold:   100,
		MaxPeers:       10,
		WhiteList:      whitelist,
	}

	bmgr := NewBanManager(bcfg)

	peerCfg := &peer.Config{
		UserAgentName:    "peer",
		UserAgentVersion: "1.0",
		Net:              wire.MainNet,
		Services:         0,
	}

	newPeer := func(cfg *peer.Config, addr string) (*peer.Peer, *conn, error) {
		r, w := io.Pipe()
		c := &conn{raddr: addr, Writer: w, Reader: r}
		p, err := peer.NewOutboundPeer(cfg, addr)
		if err != nil {
			return nil, nil, err
		}
		p.AssociateConnection(c)
		return p, c, nil
	}

	// Add two connected peers and one unconnected peer.
	pA, cA, err := newPeer(peerCfg, "10.0.0.1:8333")
	if err != nil {
		t.Errorf("unexpected peer error: %v", err)
	}
	defer cA.Close()
	bmgr.AddPeer(pA)

	pB, cB, err := newPeer(peerCfg, "10.0.0.2:8333")
	if err != nil {
		t.Errorf("unexpected peer error: %v", err)
	}
	defer cB.Close()
	bmgr.AddPeer(pB)

	pC, err := peer.NewOutboundPeer(peerCfg, "10.0.0.3:8333")
	if err != nil {
		t.Errorf("unexpected peer error: %v", err)
	}
	bmgr.AddPeer(pC)

	// Ensure a peer not associated with their connection cannot be
	// whitelisted.
	if bmgr.IsPeerWhitelisted(pC) {
		t.Errorf("Expected an unconnected peer to not be whitelisted")
	}

	// Ensure a peer not whitelisted is not marked as so.
	if bmgr.IsPeerWhitelisted(pB) {
		t.Errorf("Expected peer B not to be whitelisted")
	}

	// Ensure a peer whitelisted to be marked as so.
	if !bmgr.IsPeerWhitelisted(pA) {
		t.Errorf("Expected peer A to be whitelisted")
	}
}
