// Copyright (c) 2021-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package banmanager

import (
	"fmt"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"github.com/decred/dcrd/connmgr/v3"
	"github.com/decred/dcrd/peer/v4"
)

// Config is the configuration struct for the ban manager.
type Config struct {
	// DisableBanning represents the status of disabling banning of
	// misbehaving peers.
	DisableBanning bool

	// BanThreshold represents the maximum allowed ban score before
	// misbehaving peers are disconnecting and banned.
	BanThreshold uint32

	// BanDuration is the duration for which misbehaving peers stay banned for.
	BanDuration time.Duration

	// MaxPeers indicates the maximum number of inbound and outbound
	// peers allowed.
	MaxPeers int

	// Whitelist represents the whitelisted IPs of the server.
	WhiteList []net.IPNet
}

// banMgrPeer extends a peer to maintain additional state maintained by the
// ban manager.
type banMgrPeer struct {
	*peer.Peer

	isWhitelisted bool
	banScore      connmgr.DynamicBanScore
}

// BanManager represents a peer ban score tracking manager.
type BanManager struct {
	cfg    Config
	peers  map[*peer.Peer]*banMgrPeer
	banned map[string]time.Time
	mtx    sync.Mutex
}

// NewBanManager initializes a new peer banning manager.
func NewBanManager(cfg *Config) *BanManager {
	return &BanManager{
		cfg:    *cfg,
		peers:  make(map[*peer.Peer]*banMgrPeer, cfg.MaxPeers),
		banned: make(map[string]time.Time, cfg.MaxPeers),
	}
}

// lookupPeer returns the ban manager peer that maintains additional state for
// a given base peer.  In the event the mapping does not exist, a warning is
// logged and nil is returned.
//
// This function MUST be called with the ban manager mutex locked (for reads).
func (bm *BanManager) lookupPeer(p *peer.Peer) *banMgrPeer {
	bmp, ok := bm.peers[p]
	if !ok {
		log.Warnf("Attempt to lookup unknown peer %s\nStack: %v", p,
			string(debug.Stack()))
		return nil
	}

	return bmp
}

// isPeerWhitelisted checks if the provided peer is whitelisted per the
// provided whitelist.
func (bm *BanManager) isPeerWhitelisted(p *peer.Peer, whitelist []net.IPNet) bool {
	host, _, err := net.SplitHostPort(p.Addr())
	if err != nil {
		log.Errorf("Unable to split peer '%s' IP: %v", p.Addr(), err)
		return false
	}

	ip := net.ParseIP(host)
	if ip == nil {
		log.Errorf("Unable to parse IP '%s'", p.Addr())
		return false
	}

	for _, ipnet := range whitelist {
		if ipnet.Contains(ip) {
			return true
		}
	}

	return false
}

// IsPeerWhitelisted checks if the provided peer is whitelisted.
func (bm *BanManager) IsPeerWhitelisted(p *peer.Peer) bool {
	bm.mtx.Lock()
	bmp := bm.lookupPeer(p)
	bm.mtx.Unlock()
	if bmp == nil {
		return false
	}

	return bmp.isWhitelisted
}

// AddPeer adds the provided peer to the ban manager.
func (bm *BanManager) AddPeer(p *peer.Peer) error {
	host, _, err := net.SplitHostPort(p.Addr())
	if err != nil {
		p.Disconnect()
		return fmt.Errorf("cannot split hostport %w", err)
	}

	bm.mtx.Lock()
	banEnd, ok := bm.banned[host]
	bm.mtx.Unlock()

	if ok {
		if time.Now().Before(banEnd) {
			p.Disconnect()
			return fmt.Errorf("peer %s is banned for another %v - disconnecting",
				host, time.Until(banEnd))
		}

		log.Infof("Peer %s is no longer banned", host)

		bm.mtx.Lock()
		delete(bm.banned, host)
		bm.mtx.Unlock()
	}

	bmp := &banMgrPeer{
		Peer:          p,
		isWhitelisted: bm.isPeerWhitelisted(p, bm.cfg.WhiteList),
	}

	bm.mtx.Lock()
	bm.peers[p] = bmp
	bm.mtx.Unlock()

	return nil
}

// RemovePeer discards the provided peer from the ban manager.
func (bm *BanManager) RemovePeer(p *peer.Peer) {
	bm.mtx.Lock()
	delete(bm.peers, p)
	bm.mtx.Unlock()
}

// BanPeer bans the provided peer.
func (bm *BanManager) BanPeer(p *peer.Peer) {
	// Return immediately if banning is disabled.
	if bm.cfg.DisableBanning {
		return
	}

	bm.mtx.Lock()
	bmp := bm.lookupPeer(p)
	bm.mtx.Unlock()
	if bmp == nil {
		return
	}

	// Return if the peer is whitelisted.
	if bmp.isWhitelisted {
		return
	}

	// Ban and remove the peer.
	host, _, err := net.SplitHostPort(p.Addr())
	if err != nil {
		log.Debugf("can't split ban peer %s %v", p.Addr(), err)
		return
	}

	direction := directionString(p.Inbound())
	log.Infof("Banned peer %s (%s) for %v", host, direction,
		bm.cfg.BanDuration)

	bm.mtx.Lock()
	bm.banned[host] = time.Now().Add(bm.cfg.BanDuration)
	bm.mtx.Unlock()

	p.Disconnect()
	bm.RemovePeer(p)
}

// AddBanScore increases the persistent and decaying ban scores of the
// provided peer by the values passed as parameters. If the resulting score
// exceeds half of the ban threshold, a warning is logged including the reason
// provided. Further, if the score is above the ban threshold, the peer will
// be banned.
func (bm *BanManager) AddBanScore(p *peer.Peer, persistent, transient uint32, reason string) bool {
	// No warning is logged and no score is calculated if banning is disabled.
	if bm.cfg.DisableBanning {
		return false
	}

	bm.mtx.Lock()
	bmp := bm.lookupPeer(p)
	bm.mtx.Unlock()
	if bmp == nil {
		return false
	}

	if bmp.isWhitelisted {
		log.Debugf("Misbehaving whitelisted peer %s: %s", p, reason)
		return false
	}

	banScore := bmp.banScore.Int()
	warnThreshold := bm.cfg.BanThreshold >> 1
	if transient == 0 && persistent == 0 {
		// The score is not being increased, but a warning message is still
		// logged if the score is above the warn threshold.
		if banScore > warnThreshold {
			log.Warnf("Misbehaving peer %s: %s -- ban score is %d, "+
				"it was not increased this time", p, reason, banScore)
		}
		return false
	}

	banScore = bmp.banScore.Increase(persistent, transient)
	if banScore > warnThreshold {
		log.Warnf("Misbehaving peer %s: %s -- ban score increased to %d",
			p, reason, banScore)
		if banScore > bm.cfg.BanThreshold {
			log.Warnf("Misbehaving peer %s -- banning and disconnecting", p)
			bm.BanPeer(p)
			return true
		}
	}

	return false
}

// BanScore returns the ban score of the provided peer.
func (bm *BanManager) BanScore(p *peer.Peer) uint32 {
	bm.mtx.Lock()
	bmp := bm.lookupPeer(p)
	bm.mtx.Unlock()
	if bmp == nil {
		return 0
	}
	return bmp.banScore.Int()
}
