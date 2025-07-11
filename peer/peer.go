// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2016-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

import (
	"bytes"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/container/lru"
	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/crypto/rand"
	"github.com/decred/dcrd/wire"
	"github.com/decred/go-socks/socks"
	"github.com/decred/slog"
)

const (
	// MaxProtocolVersion is the max protocol version the peer supports.
	MaxProtocolVersion = wire.BatchedCFiltersV2Version

	// outputBufferSize is the number of elements the output channels use.
	outputBufferSize = 5000

	// invTrickleSize is the maximum amount of inventory to send in a single
	// message when trickling inventory to remote peers.
	maxInvTrickleSize = 1000

	// maxKnownInventory is the maximum number of items to keep in the known
	// inventory cache.
	maxKnownInventory = 1000

	// maxKnownInventoryTTL is the duration to keep known inventory vectors in
	// the cache before they are expired.
	maxKnownInventoryTTL = 15 * time.Minute

	// negotiateTimeout is the duration of inactivity before we timeout a
	// peer that hasn't completed the initial version negotiation.
	negotiateTimeout = 30 * time.Second

	// stallTickInterval is the interval of time between each check for
	// stalled peers.
	stallTickInterval = 15 * time.Second

	// stallResponseTimeout is the base maximum amount of time messages that
	// expect a response will wait before disconnecting the peer for
	// stalling.  The deadlines are adjusted for callback running times and
	// only checked on each stall tick interval.
	stallResponseTimeout = 30 * time.Second

	// minInvTrickleSize and maxInvTrickleSize define the lower and upper
	// limits, respectively, of random delay waited while batching
	// inventory before it is trickled to a peer.
	minInvTrickleTimeout = 100 * time.Millisecond
	maxInvTrickleTimeout = 500 * time.Millisecond

	// defaultIdleTimeout is the default duration of inactivity before a peer is
	// timed out when a peer is created with the idle timeout configuration
	// option set to 0.
	defaultIdleTimeout = 120 * time.Second

	// pingInterval is the interval of time to wait in between sending ping
	// messages.
	pingInterval = defaultIdleTimeout - 13*time.Second
)

var (
	// nodeCount is the total number of peer connections made since startup
	// and is used to assign an id to a peer.
	nodeCount int32

	// sentNonces houses the unique nonces that are generated when pushing
	// version messages that are used to detect self connections.
	sentNonces = lru.NewSet[uint64](50)

	// allowSelfConns is only used to allow the tests to bypass the self
	// connection detecting and disconnect logic since they intentionally
	// do so for testing purposes.
	allowSelfConns bool
)

// MessageListeners defines callback function pointers to invoke with message
// listeners for a peer. Any listener which is not set to a concrete callback
// during peer initialization is ignored. Execution of multiple message
// listeners occurs serially, so one callback blocks the execution of the next.
//
// NOTE: Unless otherwise documented, these listeners must NOT directly call any
// blocking calls (such as WaitForShutdown) on the peer instance since the input
// handler goroutine blocks until the callback has completed.  Doing so will
// result in a deadlock.
type MessageListeners struct {
	// OnGetAddr is invoked when a peer receives a getaddr wire message.
	OnGetAddr func(p *Peer, msg *wire.MsgGetAddr)

	// OnAddr is invoked when a peer receives an addr wire message.
	OnAddr func(p *Peer, msg *wire.MsgAddr)

	// OnPing is invoked when a peer receives a ping wire message.
	OnPing func(p *Peer, msg *wire.MsgPing)

	// OnPong is invoked when a peer receives a pong wire message.
	OnPong func(p *Peer, msg *wire.MsgPong)

	// OnMemPool is invoked when a peer receives a mempool wire message.
	OnMemPool func(p *Peer, msg *wire.MsgMemPool)

	// OnGetMiningState is invoked when a peer receives a getminings wire
	// message.
	OnGetMiningState func(p *Peer, msg *wire.MsgGetMiningState)

	// OnMiningState is invoked when a peer receives a miningstate wire
	// message.
	OnMiningState func(p *Peer, msg *wire.MsgMiningState)

	// OnTx is invoked when a peer receives a tx wire message.
	OnTx func(p *Peer, msg *wire.MsgTx)

	// OnBlock is invoked when a peer receives a block wire message.
	OnBlock func(p *Peer, msg *wire.MsgBlock, buf []byte)

	// OnCFilter is invoked when a peer receives a cfilter wire message.
	OnCFilter func(p *Peer, msg *wire.MsgCFilter)

	// OnCFilterV2 is invoked when a peer receives a cfilterv2 wire message.
	OnCFilterV2 func(p *Peer, msg *wire.MsgCFilterV2)

	// OnCFiltersV2 is invoked when a peer receives a cfiltersv2 wire message.
	OnCFiltersV2 func(p *Peer, msg *wire.MsgCFiltersV2)

	// OnCFHeaders is invoked when a peer receives a cfheaders wire
	// message.
	OnCFHeaders func(p *Peer, msg *wire.MsgCFHeaders)

	// OnCFTypes is invoked when a peer receives a cftypes wire message.
	OnCFTypes func(p *Peer, msg *wire.MsgCFTypes)

	// OnInv is invoked when a peer receives an inv wire message.
	OnInv func(p *Peer, msg *wire.MsgInv)

	// OnHeaders is invoked when a peer receives a headers wire message.
	OnHeaders func(p *Peer, msg *wire.MsgHeaders)

	// OnNotFound is invoked when a peer receives a notfound wire message.
	OnNotFound func(p *Peer, msg *wire.MsgNotFound)

	// OnGetData is invoked when a peer receives a getdata wire message.
	OnGetData func(p *Peer, msg *wire.MsgGetData)

	// OnGetBlocks is invoked when a peer receives a getblocks wire message.
	OnGetBlocks func(p *Peer, msg *wire.MsgGetBlocks)

	// OnGetHeaders is invoked when a peer receives a getheaders wire
	// message.
	OnGetHeaders func(p *Peer, msg *wire.MsgGetHeaders)

	// OnGetCFilter is invoked when a peer receives a getcfilter wire
	// message.
	OnGetCFilter func(p *Peer, msg *wire.MsgGetCFilter)

	// OnGetCFilterV2 is invoked when a peer receives a getcfilterv2 wire
	// message.
	OnGetCFilterV2 func(p *Peer, msg *wire.MsgGetCFilterV2)

	// OnGetCFiltersV2 is invoked when a peer receives a getcfsv2 wire
	// message.
	OnGetCFiltersV2 func(p *Peer, msg *wire.MsgGetCFsV2)

	// OnGetCFHeaders is invoked when a peer receives a getcfheaders
	// wire message.
	OnGetCFHeaders func(p *Peer, msg *wire.MsgGetCFHeaders)

	// OnGetCFTypes is invoked when a peer receives a getcftypes wire
	// message.
	OnGetCFTypes func(p *Peer, msg *wire.MsgGetCFTypes)

	// OnFeeFilter is invoked when a peer receives a feefilter wire message.
	OnFeeFilter func(p *Peer, msg *wire.MsgFeeFilter)

	// OnVersion is invoked when a peer receives a version wire message.
	OnVersion func(p *Peer, msg *wire.MsgVersion)

	// OnVerAck is invoked when a peer receives a verack wire message.
	OnVerAck func(p *Peer, msg *wire.MsgVerAck)

	// OnReject is invoked when a peer receives a reject wire message.
	//
	// Deprecated: This will be removed in a future release.  The underlying
	// message is no longer valid and thus this will no longer ever be invoked.
	// Callers should avoid using it.
	OnReject func(p *Peer, msg *wire.MsgReject)

	// OnSendHeaders is invoked when a peer receives a sendheaders wire
	// message.
	OnSendHeaders func(p *Peer, msg *wire.MsgSendHeaders)

	// OnGetInitState is invoked when a peer receives a getinitstate wire
	// message.
	OnGetInitState func(p *Peer, msg *wire.MsgGetInitState)

	// OnInitState is invoked when a peer receives an initstate message.
	OnInitState func(p *Peer, msg *wire.MsgInitState)

	// OnMixPairReq is invoked when a peer receives a mixpairreq message.
	OnMixPairReq func(p *Peer, msg *wire.MsgMixPairReq)

	// OnMixKeyExchange is invoked when a peer receives a mixkeyxchg message.
	OnMixKeyExchange func(p *Peer, msg *wire.MsgMixKeyExchange)

	// OnMixCiphertexts is invoked when a peer receives a mixcphrtxt message.
	OnMixCiphertexts func(p *Peer, msg *wire.MsgMixCiphertexts)

	// OnMixSlotReserve is invoked when a peer receives a mixslotres message.
	OnMixSlotReserve func(p *Peer, msg *wire.MsgMixSlotReserve)

	// OnMixDCNet is invoked when a peer receives a mixdcnet message.
	OnMixDCNet func(p *Peer, msg *wire.MsgMixDCNet)

	// OnMixConfirm is invoked when a peer receives a mixconfirm message.
	OnMixConfirm func(p *Peer, msg *wire.MsgMixConfirm)

	// OnMixFactoredPoly is invoked when a peer receives a mixfactpoly message.
	OnMixFactoredPoly func(p *Peer, msg *wire.MsgMixFactoredPoly)

	// OnMixSecrets is invoked when a peer receives a mixsecrets message.
	OnMixSecrets func(p *Peer, msg *wire.MsgMixSecrets)

	// OnRead is invoked when a peer receives a wire message.  It consists
	// of the number of bytes read, the message, and whether or not an error
	// in the read occurred.  Typically, callers will opt to use the
	// callbacks for the specific message types, however this can be useful
	// for circumstances such as keeping track of server-wide byte counts or
	// working with custom message types for which the peer does not
	// directly provide a callback.
	OnRead func(p *Peer, bytesRead int, msg wire.Message, err error)

	// OnWrite is invoked when we write a wire message to a peer.  It
	// consists of the number of bytes written, the message, and whether or
	// not an error in the write occurred.  This can be useful for
	// circumstances such as keeping track of server-wide byte counts.
	OnWrite func(p *Peer, bytesWritten int, msg wire.Message, err error)
}

// Config is the struct to hold configuration options useful to Peer.
type Config struct {
	// NewestBlock specifies a callback which provides the newest block
	// details to the peer as needed.  This can be nil in which case the
	// peer will report a block height of 0, however it is good practice for
	// peers to specify this so their currently best known is accurately
	// reported.
	NewestBlock HashFunc

	// HostToNetAddress returns the netaddress for the given host. This can be
	// nil in which case the host will be parsed as an IP address.
	HostToNetAddress HostToNetAddrFunc

	// Proxy indicates a proxy is being used for connections.  The only
	// effect this has is to prevent leaking the tor proxy address, so it
	// only needs to be specified if using a tor proxy.
	Proxy string

	// UserAgentName specifies the user agent name to advertise.  It is
	// highly recommended to specify this value.
	UserAgentName string

	// UserAgentVersion specifies the user agent version to advertise.  It
	// is highly recommended to specify this value and that it follows the
	// form "major.minor.revision" e.g. "2.6.41".
	UserAgentVersion string

	// UserAgentComments specifies any additional comments to include the in
	// user agent to advertise.  This is optional, so it may be nil.  If
	// specified, the comments should only exist of characters from the
	// semantic alphabet [a-zA-Z0-9-].
	UserAgentComments []string

	// Net identifies the network the peer is associated with.  It is
	// highly recommended to specify this field, but it can be omitted in
	// which case the test network will be used.
	Net wire.CurrencyNet

	// Services specifies which services to advertise as supported by the
	// local peer.  This field can be omitted in which case it will be 0
	// and therefore advertise no supported services.
	Services wire.ServiceFlag

	// ProtocolVersion specifies the maximum protocol version to use and
	// advertise.  This field can be omitted in which case
	// peer.MaxProtocolVersion will be used.
	ProtocolVersion uint32

	// DisableRelayTx specifies if the remote peer should be informed to
	// not send inv messages for transactions.
	DisableRelayTx bool

	// Listeners houses callback functions to be invoked on receiving peer
	// messages.
	Listeners MessageListeners

	// IdleTimeout is the duration of inactivity before a peer is timed
	// out in seconds.
	IdleTimeout time.Duration
}

// minUint32 is a helper function to return the minimum of two uint32s.
// This avoids a math import and the need to cast to floats.
func minUint32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

// newNetAddress attempts to extract the IP address and port from the passed
// net.Addr interface and create a NetAddress structure using that information.
func newNetAddress(addr net.Addr, services wire.ServiceFlag) (*wire.NetAddress, error) {
	// addr will be a net.TCPAddr when not using a proxy.
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		ip := tcpAddr.IP
		port := uint16(tcpAddr.Port)
		na := wire.NewNetAddressIPPort(ip, port, services)
		return na, nil
	}

	// addr will be a socks.ProxiedAddr when using a proxy.
	if proxiedAddr, ok := addr.(*socks.ProxiedAddr); ok {
		ip := net.ParseIP(proxiedAddr.Host)
		if ip == nil {
			ip = net.ParseIP("0.0.0.0")
		}
		port := uint16(proxiedAddr.Port)
		na := wire.NewNetAddressIPPort(ip, port, services)
		return na, nil
	}

	// For the most part, addr should be one of the two above cases, but
	// to be safe, fall back to trying to parse the information from the
	// address string as a last resort.
	host, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}
	na := wire.NewNetAddressIPPort(ip, uint16(port), services)
	return na, nil
}

// outMsg is used to house a message to be sent along with a channel to signal
// when the message has been sent (or won't be sent due to things such as
// shutdown).
type outMsg struct {
	msg      wire.Message
	doneChan chan<- struct{}
}

// stallControlCmd represents the command of a stall control message.
type stallControlCmd uint8

// Constants for the command of a stall control message.
const (
	// sccSendMessage indicates a message is being sent to the remote peer.
	sccSendMessage stallControlCmd = iota

	// sccReceiveMessage indicates a message has been received from the
	// remote peer.
	sccReceiveMessage

	// sccHandlerStart indicates a callback handler is about to be invoked.
	sccHandlerStart

	// sccHandlerStart indicates a callback handler has completed.
	sccHandlerDone
)

// stallControlMsg is used to signal the stall handler about specific events
// so it can properly detect and handle stalled remote peers.
type stallControlMsg struct {
	command stallControlCmd
	message wire.Message
}

// StatsSnap is a snapshot of peer stats at a point in time.
type StatsSnap struct {
	ID             int32
	Addr           string
	Services       wire.ServiceFlag
	LastSend       time.Time
	LastRecv       time.Time
	BytesSent      uint64
	BytesRecv      uint64
	ConnTime       time.Time
	TimeOffset     int64
	Version        uint32
	UserAgent      string
	Inbound        bool
	StartingHeight int64
	LastBlock      int64
	LastPingNonce  uint64
	LastPingTime   time.Time
	LastPingMicros int64
}

// HashFunc is a function which returns a block hash, height and error
// It is used as a callback to get newest block details.
type HashFunc func() (hash *chainhash.Hash, height int64, err error)

// AddrFunc is a func which takes an address and returns a related address.
type AddrFunc func(remoteAddr *wire.NetAddress) *wire.NetAddress

// HostToNetAddrFunc is a func which takes a host, port, services and returns
// the netaddress.
type HostToNetAddrFunc func(host string, port uint16,
	services wire.ServiceFlag) (*wire.NetAddress, error)

// NOTE: The overall data flow of a peer is split into 3 goroutines.  Inbound
// messages are read via the inHandler goroutine and generally dispatched to
// their own handler.  For inbound data-related messages such as blocks,
// transactions, and inventory, the data is handled by the corresponding
// message handlers.  The data flow for outbound messages is split into 2
// goroutines, queueHandler and outHandler.  The first, queueHandler, is used
// as a way for external entities to queue messages, by way of the QueueMessage
// function, quickly regardless of whether the peer is currently sending or not.
// It acts as the traffic cop between the external world and the actual
// goroutine which writes to the network socket.

// Peer provides a basic concurrent safe Decred peer for handling decred
// communications via the peer-to-peer protocol.  It provides full duplex
// reading and writing, automatic handling of the initial handshake process,
// querying of usage statistics and other information about the remote peer such
// as its address, user agent, and protocol version, output message queuing,
// inventory trickling, and the ability to dynamically register and unregister
// callbacks for handling Decred protocol messages.
//
// Outbound messages are typically queued via QueueMessage or QueueInventory.
// QueueMessage is intended for all messages, including responses to data such
// as blocks and transactions.  QueueInventory, on the other hand, is only
// intended for relaying inventory as it employs a trickling mechanism to batch
// the inventory together.  However, some helper functions for pushing messages
// of specific types that typically require common special handling are
// provided as a convenience.
type Peer struct {
	// The following variables must only be used atomically.
	bytesReceived uint64
	bytesSent     uint64
	lastRecv      int64
	lastSend      int64
	disconnect    int32

	conn    net.Conn
	connMtx sync.Mutex

	// blake256Hasher is the hash.Hash object that is used by readMessage
	// to calculate the hash of read mixing messages.  Every peer's hasher
	// is a distinct object and does not require locking.
	blake256Hasher hash.Hash

	// These fields are set at creation time and never modified, so they are
	// safe to read from concurrently without a mutex.
	addr    string
	cfg     Config
	inbound bool

	flagsMtx             sync.Mutex // protects the peer flags below
	na                   *wire.NetAddress
	id                   int32
	userAgent            string
	services             wire.ServiceFlag
	versionKnown         bool
	advertisedProtoVer   uint32 // protocol version advertised by remote
	protocolVersion      uint32 // negotiated protocol version
	sendHeadersPreferred bool   // peer sent a sendheaders message
	versionSent          bool
	verAckReceived       bool

	knownInventory     *lru.Set[wire.InvVect]
	prevGetBlocksMtx   sync.Mutex
	prevGetBlocksBegin *chainhash.Hash
	prevGetBlocksStop  *chainhash.Hash
	prevGetHdrsMtx     sync.Mutex
	prevGetHdrsBegin   *chainhash.Hash
	prevGetHdrsStop    *chainhash.Hash

	// These fields keep track of statistics for the peer and are protected
	// by the statsMtx mutex.
	statsMtx       sync.RWMutex
	timeOffset     int64
	timeConnected  time.Time
	startingHeight int64
	lastBlock      int64
	lastPingNonce  uint64    // Set to nonce if we have a pending ping.
	lastPingTime   time.Time // Time we sent last ping.
	lastPingMicros int64     // Time for last ping to return.

	stallControl  chan stallControlMsg
	outputQueue   chan outMsg
	sendQueue     chan outMsg
	sendDoneQueue chan struct{}
	outputInvChan chan *wire.InvVect
	inQuit        chan struct{}
	queueQuit     chan struct{}
	outQuit       chan struct{}
	quit          chan struct{}
}

// String returns the peer's address and directionality as a human-readable
// string.
//
// This function is safe for concurrent access.
func (p *Peer) String() string {
	return fmt.Sprintf("%s (%s)", p.addr, directionString(p.inbound))
}

// UpdateLastBlockHeight updates the last known block for the peer.
//
// This function is safe for concurrent access.
func (p *Peer) UpdateLastBlockHeight(newHeight int64) {
	p.statsMtx.Lock()
	if newHeight <= p.lastBlock {
		p.statsMtx.Unlock()
		return
	}
	log.Tracef("Updating last block height of peer %v from %v to %v",
		p.addr, p.lastBlock, newHeight)
	p.lastBlock = newHeight
	p.statsMtx.Unlock()
}

// AddKnownInventory adds the passed inventory to the cache of known inventory
// for the peer.
//
// This function is safe for concurrent access.
func (p *Peer) AddKnownInventory(invVect *wire.InvVect) {
	p.knownInventory.Put(*invVect)
}

// IsKnownInventory returns whether the passed inventory already exists in
// the known inventory for the peer.
//
// This function is safe for concurrent access.
func (p *Peer) IsKnownInventory(invVect *wire.InvVect) bool {
	return p.knownInventory.Contains(*invVect)
}

// StatsSnapshot returns a snapshot of the current peer flags and statistics.
//
// This function is safe for concurrent access.
func (p *Peer) StatsSnapshot() *StatsSnap {
	p.statsMtx.RLock()

	p.flagsMtx.Lock()
	id := p.id
	addr := p.addr
	userAgent := p.userAgent
	services := p.services
	protocolVersion := p.advertisedProtoVer
	p.flagsMtx.Unlock()

	// Get a copy of all relevant flags and stats.
	statsSnap := &StatsSnap{
		ID:             id,
		Addr:           addr,
		UserAgent:      userAgent,
		Services:       services,
		LastSend:       p.LastSend(),
		LastRecv:       p.LastRecv(),
		BytesSent:      p.BytesSent(),
		BytesRecv:      p.BytesReceived(),
		ConnTime:       p.timeConnected,
		TimeOffset:     p.timeOffset,
		Version:        protocolVersion,
		Inbound:        p.inbound,
		StartingHeight: p.startingHeight,
		LastBlock:      p.lastBlock,
		LastPingNonce:  p.lastPingNonce,
		LastPingMicros: p.lastPingMicros,
		LastPingTime:   p.lastPingTime,
	}

	p.statsMtx.RUnlock()
	return statsSnap
}

// ID returns the peer id.
//
// This function is safe for concurrent access.
func (p *Peer) ID() int32 {
	p.flagsMtx.Lock()
	id := p.id
	p.flagsMtx.Unlock()

	return id
}

// NA returns the peer network address.
//
// This function is safe for concurrent access.
func (p *Peer) NA() *wire.NetAddress {
	p.flagsMtx.Lock()
	if p.na == nil {
		p.flagsMtx.Unlock()
		return nil
	}
	na := *p.na
	p.flagsMtx.Unlock()

	return &na
}

// Addr returns the peer address.
//
// This function is safe for concurrent access.
func (p *Peer) Addr() string {
	// The address doesn't change after initialization, therefore it is not
	// protected by a mutex.
	return p.addr
}

// Inbound returns whether the peer is inbound.
//
// This function is safe for concurrent access.
func (p *Peer) Inbound() bool {
	return p.inbound
}

// Services returns the services flag of the remote peer.
//
// This function is safe for concurrent access.
func (p *Peer) Services() wire.ServiceFlag {
	p.flagsMtx.Lock()
	services := p.services
	p.flagsMtx.Unlock()

	return services
}

// UserAgent returns the user agent of the remote peer.
//
// This function is safe for concurrent access.
func (p *Peer) UserAgent() string {
	p.flagsMtx.Lock()
	userAgent := p.userAgent
	p.flagsMtx.Unlock()

	return userAgent
}

// LastPingNonce returns the last ping nonce of the remote peer.
//
// This function is safe for concurrent access.
func (p *Peer) LastPingNonce() uint64 {
	p.statsMtx.RLock()
	lastPingNonce := p.lastPingNonce
	p.statsMtx.RUnlock()

	return lastPingNonce
}

// LastPingTime returns the last ping time of the remote peer.
//
// This function is safe for concurrent access.
func (p *Peer) LastPingTime() time.Time {
	p.statsMtx.RLock()
	lastPingTime := p.lastPingTime
	p.statsMtx.RUnlock()

	return lastPingTime
}

// LastPingMicros returns the last ping micros of the remote peer.
//
// This function is safe for concurrent access.
func (p *Peer) LastPingMicros() int64 {
	p.statsMtx.RLock()
	lastPingMicros := p.lastPingMicros
	p.statsMtx.RUnlock()

	return lastPingMicros
}

// VersionKnown returns the whether or not the version of a peer is known
// locally.
//
// This function is safe for concurrent access.
func (p *Peer) VersionKnown() bool {
	p.flagsMtx.Lock()
	versionKnown := p.versionKnown
	p.flagsMtx.Unlock()

	return versionKnown
}

// VerAckReceived returns whether or not a verack message was received by the
// peer.
//
// This function is safe for concurrent access.
func (p *Peer) VerAckReceived() bool {
	p.flagsMtx.Lock()
	verAckReceived := p.verAckReceived
	p.flagsMtx.Unlock()

	return verAckReceived
}

// ProtocolVersion returns the negotiated peer protocol version.
//
// This function is safe for concurrent access.
func (p *Peer) ProtocolVersion() uint32 {
	p.flagsMtx.Lock()
	protocolVersion := p.protocolVersion
	p.flagsMtx.Unlock()

	return protocolVersion
}

// LastBlock returns the last block of the peer.
//
// This function is safe for concurrent access.
func (p *Peer) LastBlock() int64 {
	p.statsMtx.RLock()
	lastBlock := p.lastBlock
	p.statsMtx.RUnlock()

	return lastBlock
}

// LastSend returns the last send time of the peer.
//
// This function is safe for concurrent access.
func (p *Peer) LastSend() time.Time {
	return time.Unix(atomic.LoadInt64(&p.lastSend), 0)
}

// LastRecv returns the last recv time of the peer.
//
// This function is safe for concurrent access.
func (p *Peer) LastRecv() time.Time {
	return time.Unix(atomic.LoadInt64(&p.lastRecv), 0)
}

// LocalAddr returns the local address of the connection or nil if the peer is
// not currently connected.
//
// This function is safe for concurrent access.
func (p *Peer) LocalAddr() net.Addr {
	var localAddr net.Addr
	if p.Connected() {
		localAddr = p.conn.LocalAddr()
	}
	return localAddr
}

// BytesSent returns the total number of bytes sent by the peer.
//
// This function is safe for concurrent access.
func (p *Peer) BytesSent() uint64 {
	return atomic.LoadUint64(&p.bytesSent)
}

// BytesReceived returns the total number of bytes received by the peer.
//
// This function is safe for concurrent access.
func (p *Peer) BytesReceived() uint64 {
	return atomic.LoadUint64(&p.bytesReceived)
}

// TimeConnected returns the time at which the peer connected.
//
// This function is safe for concurrent access.
func (p *Peer) TimeConnected() time.Time {
	p.statsMtx.RLock()
	timeConnected := p.timeConnected
	p.statsMtx.RUnlock()

	return timeConnected
}

// TimeOffset returns the number of seconds the local time was offset from the
// time the peer reported during the initial negotiation phase.  Negative values
// indicate the remote peer's time is before the local time.
//
// This function is safe for concurrent access.
func (p *Peer) TimeOffset() int64 {
	p.statsMtx.RLock()
	timeOffset := p.timeOffset
	p.statsMtx.RUnlock()

	return timeOffset
}

// StartingHeight returns the last known height the peer reported during the
// initial negotiation phase.
//
// This function is safe for concurrent access.
func (p *Peer) StartingHeight() int64 {
	p.statsMtx.RLock()
	startingHeight := p.startingHeight
	p.statsMtx.RUnlock()

	return startingHeight
}

// WantsHeaders returns if the peer wants header messages instead of
// inventory vectors for blocks.
//
// This function is safe for concurrent access.
func (p *Peer) WantsHeaders() bool {
	p.flagsMtx.Lock()
	sendHeadersPreferred := p.sendHeadersPreferred
	p.flagsMtx.Unlock()

	return sendHeadersPreferred
}

// PushAddrMsg sends an addr message to the connected peer using the provided
// addresses.  This function is useful over manually sending the message via
// QueueMessage since it automatically limits the addresses to the maximum
// number allowed by the message and randomizes the chosen addresses when there
// are too many.  It returns the addresses that were actually sent and no
// message will be sent if there are no entries in the provided addresses slice.
//
// This function is safe for concurrent access.
func (p *Peer) PushAddrMsg(addresses []*wire.NetAddress) ([]*wire.NetAddress, error) {
	// Nothing to send.
	if len(addresses) == 0 {
		return nil, nil
	}

	msg := wire.NewMsgAddr()
	msg.AddrList = make([]*wire.NetAddress, len(addresses))
	copy(msg.AddrList, addresses)

	// Randomize the addresses sent if there are more than the maximum allowed.
	if len(msg.AddrList) > wire.MaxAddrPerMsg {
		// Shuffle the address list.
		rand.ShuffleSlice(msg.AddrList)

		// Truncate it to the maximum size.
		msg.AddrList = msg.AddrList[:wire.MaxAddrPerMsg]
	}

	p.QueueMessage(msg, nil)
	return msg.AddrList, nil
}

// PushGetBlocksMsg sends a getblocks message for the provided block locator
// and stop hash.  It will ignore back-to-back duplicate requests.
//
// This function is safe for concurrent access.
func (p *Peer) PushGetBlocksMsg(locator []chainhash.Hash, stopHash *chainhash.Hash) error {
	// Extract the begin hash from the block locator, if one was specified,
	// to use for filtering duplicate getblocks requests.
	var beginHash *chainhash.Hash
	if len(locator) > 0 {
		beginHash = &locator[0]
	}

	// Filter duplicate getblocks requests.
	p.prevGetBlocksMtx.Lock()
	isDuplicate := p.prevGetBlocksStop != nil && p.prevGetBlocksBegin != nil &&
		beginHash != nil && stopHash.IsEqual(p.prevGetBlocksStop) &&
		beginHash.IsEqual(p.prevGetBlocksBegin)
	p.prevGetBlocksMtx.Unlock()

	if isDuplicate {
		log.Tracef("Filtering duplicate [getblocks] with begin "+
			"hash %v, stop hash %v", beginHash, stopHash)
		return nil
	}

	// Construct the getblocks request and queue it to be sent.
	msg := wire.NewMsgGetBlocks(stopHash)
	for i := range locator {
		err := msg.AddBlockLocatorHash(&locator[i])
		if err != nil {
			return err
		}
	}
	p.QueueMessage(msg, nil)

	// Update the previous getblocks request information for filtering
	// duplicates.
	p.prevGetBlocksMtx.Lock()
	p.prevGetBlocksBegin = beginHash
	p.prevGetBlocksStop = stopHash
	p.prevGetBlocksMtx.Unlock()
	return nil
}

// PushGetHeadersMsg sends a getblocks message for the provided block locator
// and stop hash.  It will ignore back-to-back duplicate requests.
//
// This function is safe for concurrent access.
func (p *Peer) PushGetHeadersMsg(locator []chainhash.Hash, stopHash *chainhash.Hash) error {
	// Extract the begin hash from the block locator, if one was specified,
	// to use for filtering duplicate getheaders requests.
	var beginHash *chainhash.Hash
	if len(locator) > 0 {
		beginHash = &locator[0]
	}

	// Filter duplicate getheaders requests.
	p.prevGetHdrsMtx.Lock()
	isDuplicate := p.prevGetHdrsStop != nil && p.prevGetHdrsBegin != nil &&
		beginHash != nil && stopHash.IsEqual(p.prevGetHdrsStop) &&
		beginHash.IsEqual(p.prevGetHdrsBegin)
	p.prevGetHdrsMtx.Unlock()

	if isDuplicate {
		log.Tracef("Filtering duplicate [getheaders] with begin hash %v",
			beginHash)
		return nil
	}

	// Construct the getheaders request and queue it to be sent.
	msg := wire.NewMsgGetHeaders()
	msg.HashStop = *stopHash
	for i := range locator {
		err := msg.AddBlockLocatorHash(&locator[i])
		if err != nil {
			return err
		}
	}
	p.QueueMessage(msg, nil)

	// Update the previous getheaders request information for filtering
	// duplicates.
	p.prevGetHdrsMtx.Lock()
	p.prevGetHdrsBegin = beginHash
	p.prevGetHdrsStop = stopHash
	p.prevGetHdrsMtx.Unlock()
	return nil
}

// handlePingMsg is invoked when a peer receives a ping wire message.  For
// recent clients (protocol version > BIP0031Version), it replies with a pong
// message.  For older clients, it does nothing and anything other than failure
// is considered a successful ping.
func (p *Peer) handlePingMsg(msg *wire.MsgPing) {
	// Include nonce from ping so pong can be identified.
	p.QueueMessage(wire.NewMsgPong(msg.Nonce), nil)
}

// handlePongMsg is invoked when a peer receives a pong wire message.  It
// updates the ping statistics as required for recent clients (protocol
// version > BIP0031Version).  There is no effect for older clients or when a
// ping was not previously sent.
func (p *Peer) handlePongMsg(msg *wire.MsgPong) {
	// Arguably we could use a buffered channel here sending data
	// in a fifo manner whenever we send a ping, or a list keeping track of
	// the times of each ping. For now we just make a best effort and
	// only record stats if it was for the last ping sent. Any preceding
	// and overlapping pings will be ignored. It is unlikely to occur
	// without large usage of the ping rpc call since we ping infrequently
	// enough that if they overlap we would have timed out the peer.
	p.statsMtx.Lock()
	if p.lastPingNonce != 0 && msg.Nonce == p.lastPingNonce {
		p.lastPingMicros = time.Since(p.lastPingTime).Nanoseconds()
		p.lastPingMicros /= 1000 // convert to usec.
		p.lastPingNonce = 0
	}
	p.statsMtx.Unlock()
}

// hashable is a wire message which can be hashed but requires the hash to be
// calculated upfront after creating any message or (importantly for peer)
// deserializing a message off the wire.
type hashable interface {
	wire.Message
	WriteHash(hash.Hash)
	Hash() chainhash.Hash
}

// readMessage reads the next wire message from the peer with logging.
func (p *Peer) readMessage() (wire.Message, []byte, error) {
	err := p.conn.SetReadDeadline(time.Now().Add(p.cfg.IdleTimeout))
	if err != nil {
		return nil, nil, err
	}
	n, msg, buf, err := wire.ReadMessageN(p.conn, p.ProtocolVersion(),
		p.cfg.Net)
	atomic.AddUint64(&p.bytesReceived, uint64(n))

	// Calculate and store the message hash of any mixing message
	// immediately after deserializing it.
	if err == nil {
		if msg, ok := msg.(hashable); ok {
			msg.WriteHash(p.blake256Hasher)
		}
	}

	if p.cfg.Listeners.OnRead != nil {
		p.cfg.Listeners.OnRead(p, n, msg, err)
	}
	if err != nil { // Check ReadMessageN error
		return nil, nil, err
	}

	// Only construct expensive log strings when the logging level requires it.
	if log.Level() <= slog.LevelDebug {
		// Debug summary of message.
		summary := messageSummary(msg)
		if len(summary) > 0 {
			summary = " (" + summary + ")"
		}
		log.Debugf("Received %s%s from %s", msg.Command(), summary, p)
	}
	if log.Level() <= slog.LevelTrace {
		log.Trace(spew.Sdump(msg))
		log.Trace(spew.Sdump(buf))
	}

	return msg, buf, nil
}

// writeMessage sends a wire message to the peer with logging.
func (p *Peer) writeMessage(msg wire.Message) error {
	// Don't do anything if we're disconnecting.
	if atomic.LoadInt32(&p.disconnect) != 0 {
		return nil
	}

	// Only construct expensive log strings when the logging level requires it.
	if log.Level() <= slog.LevelDebug {
		// Debug summary of message.
		summary := messageSummary(msg)
		if len(summary) > 0 {
			summary = " (" + summary + ")"
		}
		log.Debugf("Sending %v%s to %s", msg.Command(), summary, p)
	}
	if log.Level() <= slog.LevelTrace {
		log.Trace(spew.Sdump(msg))

		var buf bytes.Buffer
		err := wire.WriteMessage(&buf, msg, p.ProtocolVersion(), p.cfg.Net)
		if err == nil {
			log.Trace(spew.Sdump(buf.Bytes()))
		}
	}

	// Write the message to the peer.
	n, err := wire.WriteMessageN(p.conn, msg, p.ProtocolVersion(), p.cfg.Net)
	atomic.AddUint64(&p.bytesSent, uint64(n))
	if p.cfg.Listeners.OnWrite != nil {
		p.cfg.Listeners.OnWrite(p, n, msg, err)
	}
	return err
}

// shouldHandleReadError returns whether or not the passed error, which is
// expected to have come from reading from the remote peer in the inHandler,
// should be logged and responded to with a reject message.
func (p *Peer) shouldHandleReadError(err error) bool {
	// No logging or reject message when the peer is being forcibly
	// disconnected.
	if atomic.LoadInt32(&p.disconnect) != 0 {
		return false
	}

	// No logging or reject message when the remote peer has been
	// disconnected.
	if errors.Is(err, io.EOF) {
		return false
	}

	// Handle all temporary network errors besides timeout errors.
	var opErr *net.OpError
	if errors.As(err, &opErr) &&
		(!opErr.Temporary() || opErr.Timeout()) {
		return false
	}

	return true
}

// maybeAddDeadline potentially adds a deadline for the appropriate expected
// response for the passed wire protocol command to the pending responses map.
func (p *Peer) maybeAddDeadline(pendingResponses map[string]time.Time, msgCmd string) {
	// Setup a deadline for each message being sent that expects a response.
	//
	// NOTE: Pings are intentionally ignored here since they are typically sent
	// asynchronously and as a result of a long backlog of messages, such as is
	// typical in the case of the initial chain sync, the response won't be
	// received in time.
	//
	// Also, getheaders is intentionally ignored since there is no guaranteed
	// response if the remote peer does not have any headers for the locator.
	var addedDeadline bool
	deadline := time.Now().Add(stallResponseTimeout)
	switch msgCmd {
	case wire.CmdVersion:
		// Expects a verack message.
		pendingResponses[wire.CmdVerAck] = deadline
		addedDeadline = true

	case wire.CmdMemPool:
		// Expects an inv message.
		pendingResponses[wire.CmdInv] = deadline
		addedDeadline = true

	case wire.CmdGetBlocks:
		// Expects an inv message.
		pendingResponses[wire.CmdInv] = deadline
		addedDeadline = true

	case wire.CmdGetData:
		// Expects a block, tx, mix, or notfound message.
		pendingResponses[wire.CmdBlock] = deadline
		pendingResponses[wire.CmdTx] = deadline
		pendingResponses[wire.CmdMixPairReq] = deadline
		pendingResponses[wire.CmdMixKeyExchange] = deadline
		pendingResponses[wire.CmdMixCiphertexts] = deadline
		pendingResponses[wire.CmdMixSlotReserve] = deadline
		pendingResponses[wire.CmdMixDCNet] = deadline
		pendingResponses[wire.CmdMixFactoredPoly] = deadline
		pendingResponses[wire.CmdMixConfirm] = deadline
		pendingResponses[wire.CmdMixSecrets] = deadline
		pendingResponses[wire.CmdNotFound] = deadline
		addedDeadline = true

	case wire.CmdGetMiningState:
		pendingResponses[wire.CmdMiningState] = deadline
		addedDeadline = true

	case wire.CmdGetInitState:
		pendingResponses[wire.CmdInitState] = deadline
		addedDeadline = true
	}

	if addedDeadline {
		log.Debugf("Adding deadline for command %s for peer %s", msgCmd, p.addr)
	}
}

// stallHandler handles stall detection for the peer.  This entails keeping
// track of expected responses and assigning them deadlines while accounting for
// the time spent in callbacks.  It must be run as a goroutine.
func (p *Peer) stallHandler() {
	// These variables are used to adjust the deadline times forward by the
	// time it takes callbacks to execute.  This is done because new
	// messages aren't read until the previous one is finished processing
	// (which includes callbacks), so the deadline for receiving a response
	// for a given message must account for the processing time as well.
	var handlerActive bool
	var handlersStartTime time.Time
	var deadlineOffset time.Duration

	// pendingResponses tracks the expected response deadline times.
	pendingResponses := make(map[string]time.Time)

	// stallTicker is used to periodically check pending responses that have
	// exceeded the expected deadline and disconnect the peer due to
	// stalling.
	stallTicker := time.NewTicker(stallTickInterval)
	defer stallTicker.Stop()

	// ioStopped is used to detect when both the input and output handler
	// goroutines are done.
	var ioStopped bool
out:
	for {
		select {
		case msg := <-p.stallControl:
			switch msg.command {
			case sccSendMessage:
				// Add a deadline for the expected response
				// message if needed.
				p.maybeAddDeadline(pendingResponses,
					msg.message.Command())

			case sccReceiveMessage:
				// Remove received messages from the expected
				// response map.  Since certain commands expect
				// one of a group of responses, remove
				// everything in the expected group accordingly.
				switch msgCmd := msg.message.Command(); msgCmd {
				case wire.CmdBlock:
					fallthrough
				case wire.CmdTx:
					fallthrough
				case wire.CmdMixPairReq:
					fallthrough
				case wire.CmdMixKeyExchange:
					fallthrough
				case wire.CmdMixCiphertexts:
					fallthrough
				case wire.CmdMixSlotReserve:
					fallthrough
				case wire.CmdMixDCNet:
					fallthrough
				case wire.CmdMixFactoredPoly:
					fallthrough
				case wire.CmdMixConfirm:
					fallthrough
				case wire.CmdMixSecrets:
					fallthrough
				case wire.CmdNotFound:
					delete(pendingResponses, wire.CmdBlock)
					delete(pendingResponses, wire.CmdTx)
					delete(pendingResponses, wire.CmdMixPairReq)
					delete(pendingResponses, wire.CmdMixKeyExchange)
					delete(pendingResponses, wire.CmdMixCiphertexts)
					delete(pendingResponses, wire.CmdMixSlotReserve)
					delete(pendingResponses, wire.CmdMixDCNet)
					delete(pendingResponses, wire.CmdMixFactoredPoly)
					delete(pendingResponses, wire.CmdMixConfirm)
					delete(pendingResponses, wire.CmdMixSecrets)
					delete(pendingResponses, wire.CmdNotFound)

				default:
					delete(pendingResponses, msgCmd)
				}

			case sccHandlerStart:
				// Warn on unbalanced callback signaling.
				if handlerActive {
					log.Warn("Received handler start " +
						"control command while a " +
						"handler is already active")
					continue
				}

				handlerActive = true
				handlersStartTime = time.Now()

			case sccHandlerDone:
				// Warn on unbalanced callback signaling.
				if !handlerActive {
					log.Warn("Received handler done " +
						"control command when a " +
						"handler is not already active")
					continue
				}

				// Extend active deadlines by the time it took
				// to execute the callback.
				duration := time.Since(handlersStartTime)
				deadlineOffset += duration
				handlerActive = false

			default:
				log.Warnf("Unsupported message command %v",
					msg.command)
			}

		case <-stallTicker.C:
			// Calculate the offset to apply to the deadline based
			// on how long the handlers have taken to execute since
			// the last tick.
			now := time.Now()
			offset := deadlineOffset
			if handlerActive {
				offset += now.Sub(handlersStartTime)
			}

			// Disconnect the peer if any of the pending responses
			// don't arrive by their adjusted deadline.
			for command, deadline := range pendingResponses {
				if command == wire.CmdMiningState {
					continue
				}

				if now.Before(deadline.Add(offset)) {
					log.Debugf("Stall ticker rolling over for peer %s on "+
						"cmd %s (deadline for data: %s)", p, command,
						deadline.String())
					continue
				}

				log.Infof("Peer %s appears to be stalled or "+
					"misbehaving, %s timeout -- "+
					"disconnecting", p, command)
				p.Disconnect()
				break
			}

			// Reset the deadline offset for the next tick.
			deadlineOffset = 0

		case <-p.inQuit:
			// The stall handler can exit once both the input and
			// output handler goroutines are done.
			if ioStopped {
				break out
			}
			ioStopped = true

		case <-p.outQuit:
			// The stall handler can exit once both the input and
			// output handler goroutines are done.
			if ioStopped {
				break out
			}
			ioStopped = true
		}
	}

	// Drain any wait channels before going away so there is nothing left
	// waiting on this goroutine.
cleanup:
	for {
		select {
		case <-p.stallControl:
		default:
			break cleanup
		}
	}
	log.Tracef("Peer stall handler done for %s", p)
}

// inHandler handles all incoming messages for the peer.  It must be run as a
// goroutine.
func (p *Peer) inHandler() {
out:
	for atomic.LoadInt32(&p.disconnect) == 0 {
		// Read a message and stop the idle timer as soon as the read
		// is done.  The timer is reset below for the next iteration if
		// needed.
		rmsg, buf, err := p.readMessage()
		if err != nil {
			// Only log the error if the local peer is not forcibly
			// disconnecting and the remote peer has not disconnected.
			if p.shouldHandleReadError(err) {
				log.Errorf("Can't read message from %s: %v", p, err)
			}

			var nErr net.Error
			if errors.As(err, &nErr) && nErr.Timeout() {
				log.Warnf("Peer %s no answer for %s -- disconnecting",
					p, p.cfg.IdleTimeout)
			}

			break out
		}
		atomic.StoreInt64(&p.lastRecv, time.Now().Unix())
		select {
		case p.stallControl <- stallControlMsg{sccReceiveMessage, rmsg}:
		case <-p.quit:
			break out
		}

		// Handle each supported message type.
		select {
		case p.stallControl <- stallControlMsg{sccHandlerStart, rmsg}:
		case <-p.quit:
			break out
		}
		switch msg := rmsg.(type) {
		case *wire.MsgVersion:
			// Limit to one version message per peer.
			log.Debugf("Already received 'version' from peer %s -- "+
				"disconnecting", p)
			break out

		case *wire.MsgVerAck:
			// No read lock is necessary because verAckReceived is not written
			// to in any other goroutine.
			if p.verAckReceived {
				log.Infof("Already received 'verack' from peer %s -- "+
					"disconnecting", p)
				break out
			}
			p.flagsMtx.Lock()
			p.verAckReceived = true
			p.flagsMtx.Unlock()
			if p.cfg.Listeners.OnVerAck != nil {
				p.cfg.Listeners.OnVerAck(p, msg)
			}

		case *wire.MsgGetAddr:
			if p.cfg.Listeners.OnGetAddr != nil {
				p.cfg.Listeners.OnGetAddr(p, msg)
			}

		case *wire.MsgAddr:
			if p.cfg.Listeners.OnAddr != nil {
				p.cfg.Listeners.OnAddr(p, msg)
			}

		case *wire.MsgPing:
			p.handlePingMsg(msg)
			if p.cfg.Listeners.OnPing != nil {
				p.cfg.Listeners.OnPing(p, msg)
			}

		case *wire.MsgPong:
			p.handlePongMsg(msg)
			if p.cfg.Listeners.OnPong != nil {
				p.cfg.Listeners.OnPong(p, msg)
			}

		case *wire.MsgMemPool:
			if p.cfg.Listeners.OnMemPool != nil {
				p.cfg.Listeners.OnMemPool(p, msg)
			}

		case *wire.MsgGetMiningState:
			if p.cfg.Listeners.OnGetMiningState != nil {
				p.cfg.Listeners.OnGetMiningState(p, msg)
			}

		case *wire.MsgMiningState:
			if p.cfg.Listeners.OnMiningState != nil {
				p.cfg.Listeners.OnMiningState(p, msg)
			}

		case *wire.MsgTx:
			if p.cfg.Listeners.OnTx != nil {
				p.cfg.Listeners.OnTx(p, msg)
			}

		case *wire.MsgBlock:
			if p.cfg.Listeners.OnBlock != nil {
				p.cfg.Listeners.OnBlock(p, msg, buf)
			}

		case *wire.MsgInv:
			if p.cfg.Listeners.OnInv != nil {
				p.cfg.Listeners.OnInv(p, msg)
			}

		case *wire.MsgHeaders:
			if p.cfg.Listeners.OnHeaders != nil {
				p.cfg.Listeners.OnHeaders(p, msg)
			}

		case *wire.MsgNotFound:
			if p.cfg.Listeners.OnNotFound != nil {
				p.cfg.Listeners.OnNotFound(p, msg)
			}

		case *wire.MsgGetData:
			if p.cfg.Listeners.OnGetData != nil {
				p.cfg.Listeners.OnGetData(p, msg)
			}

		case *wire.MsgGetBlocks:
			if p.cfg.Listeners.OnGetBlocks != nil {
				p.cfg.Listeners.OnGetBlocks(p, msg)
			}

		case *wire.MsgGetHeaders:
			if p.cfg.Listeners.OnGetHeaders != nil {
				p.cfg.Listeners.OnGetHeaders(p, msg)
			}

		case *wire.MsgGetCFilter:
			if p.cfg.Listeners.OnGetCFilter != nil {
				p.cfg.Listeners.OnGetCFilter(p, msg)
			}

		case *wire.MsgGetCFHeaders:
			if p.cfg.Listeners.OnGetCFHeaders != nil {
				p.cfg.Listeners.OnGetCFHeaders(p, msg)
			}

		case *wire.MsgGetCFTypes:
			if p.cfg.Listeners.OnGetCFTypes != nil {
				p.cfg.Listeners.OnGetCFTypes(p, msg)
			}

		case *wire.MsgCFilter:
			if p.cfg.Listeners.OnCFilter != nil {
				p.cfg.Listeners.OnCFilter(p, msg)
			}

		case *wire.MsgCFHeaders:
			if p.cfg.Listeners.OnCFHeaders != nil {
				p.cfg.Listeners.OnCFHeaders(p, msg)
			}

		case *wire.MsgCFTypes:
			if p.cfg.Listeners.OnCFTypes != nil {
				p.cfg.Listeners.OnCFTypes(p, msg)
			}

		case *wire.MsgFeeFilter:
			if p.cfg.Listeners.OnFeeFilter != nil {
				p.cfg.Listeners.OnFeeFilter(p, msg)
			}

		case *wire.MsgSendHeaders:
			p.flagsMtx.Lock()
			p.sendHeadersPreferred = true
			p.flagsMtx.Unlock()

			if p.cfg.Listeners.OnSendHeaders != nil {
				p.cfg.Listeners.OnSendHeaders(p, msg)
			}

		case *wire.MsgGetCFilterV2:
			if p.cfg.Listeners.OnGetCFilterV2 != nil {
				p.cfg.Listeners.OnGetCFilterV2(p, msg)
			}

		case *wire.MsgCFilterV2:
			if p.cfg.Listeners.OnCFilterV2 != nil {
				p.cfg.Listeners.OnCFilterV2(p, msg)
			}

		case *wire.MsgGetCFsV2:
			if p.cfg.Listeners.OnGetCFiltersV2 != nil {
				p.cfg.Listeners.OnGetCFiltersV2(p, msg)
			}

		case *wire.MsgCFiltersV2:
			if p.cfg.Listeners.OnCFiltersV2 != nil {
				p.cfg.Listeners.OnCFiltersV2(p, msg)
			}

		case *wire.MsgGetInitState:
			if p.cfg.Listeners.OnGetInitState != nil {
				p.cfg.Listeners.OnGetInitState(p, msg)
			}

		case *wire.MsgInitState:
			if p.cfg.Listeners.OnInitState != nil {
				p.cfg.Listeners.OnInitState(p, msg)
			}

		case *wire.MsgMixPairReq:
			if p.cfg.Listeners.OnMixPairReq != nil {
				p.cfg.Listeners.OnMixPairReq(p, msg)
			}

		case *wire.MsgMixKeyExchange:
			if p.cfg.Listeners.OnMixKeyExchange != nil {
				p.cfg.Listeners.OnMixKeyExchange(p, msg)
			}

		case *wire.MsgMixCiphertexts:
			if p.cfg.Listeners.OnMixCiphertexts != nil {
				p.cfg.Listeners.OnMixCiphertexts(p, msg)
			}

		case *wire.MsgMixSlotReserve:
			if p.cfg.Listeners.OnMixSlotReserve != nil {
				p.cfg.Listeners.OnMixSlotReserve(p, msg)
			}

		case *wire.MsgMixDCNet:
			if p.cfg.Listeners.OnMixDCNet != nil {
				p.cfg.Listeners.OnMixDCNet(p, msg)
			}

		case *wire.MsgMixConfirm:
			if p.cfg.Listeners.OnMixConfirm != nil {
				p.cfg.Listeners.OnMixConfirm(p, msg)
			}

		case *wire.MsgMixFactoredPoly:
			if p.cfg.Listeners.OnMixFactoredPoly != nil {
				p.cfg.Listeners.OnMixFactoredPoly(p, msg)
			}

		case *wire.MsgMixSecrets:
			if p.cfg.Listeners.OnMixSecrets != nil {
				p.cfg.Listeners.OnMixSecrets(p, msg)
			}

		default:
			log.Debugf("Received unhandled message of type %v "+
				"from %v", rmsg.Command(), p)
		}
		select {
		case p.stallControl <- stallControlMsg{sccHandlerDone, rmsg}:
		case <-p.quit:
			break out
		}
	}

	// Ensure connection is closed.
	p.Disconnect()

	close(p.inQuit)
	log.Tracef("Peer input handler done for %s", p)
}

// queueHandler handles the queuing of outgoing data for the peer. This runs as
// a muxer for various sources of input so we can ensure that server and peer
// handlers will not block on us sending a message.  That data is then passed on
// to outHandler to be actually written.
func (p *Peer) queueHandler() {
	var pendingMsgs []outMsg
	var invSendQueue []*wire.InvVect

	// We keep the waiting flag so that we know if we have a message queued
	// to the outHandler or not.  We could use the presence of a head of
	// the list for this but then we have rather racy concerns about whether
	// it has gotten it at cleanup time - and thus who sends on the
	// message's done channel.  To avoid such confusion we keep a different
	// flag and pendingMsgs only contains messages that we have not yet
	// passed to outHandler.
	waiting := false

	// To avoid duplication below.
	queuePacket := func(msg outMsg, list *[]outMsg, waiting bool) bool {
		if !waiting {
			p.sendQueue <- msg
		} else {
			*list = append(*list, msg)
		}
		// we are always waiting now.
		return true
	}

	trickleTimeout := func() time.Duration {
		return minInvTrickleTimeout + rand.Duration(
			maxInvTrickleTimeout-minInvTrickleTimeout)
	}

	trickleTimer := time.NewTimer(trickleTimeout())
	defer trickleTimer.Stop()

out:
	for {
		select {
		case msg := <-p.outputQueue:
			waiting = queuePacket(msg, &pendingMsgs, waiting)

		// This channel is notified when a message has been sent across
		// the network socket.
		case <-p.sendDoneQueue:
			// No longer waiting if there are no more messages
			// in the pending messages queue.
			if len(pendingMsgs) == 0 {
				waiting = false
				continue
			}

			// Notify the outHandler about the next item to
			// asynchronously send.
			next := pendingMsgs[0]
			pendingMsgs = pendingMsgs[1:]
			p.sendQueue <- next

		case iv := <-p.outputInvChan:
			// No handshake?  They'll find out soon enough.
			if p.VersionKnown() {
				invSendQueue = append(invSendQueue, iv)
			}

		case <-trickleTimer.C:
			// Don't send anything if we're disconnecting or there
			// is no queued inventory.
			// version is known if send queue has any entries.
			switch {
			case atomic.LoadInt32(&p.disconnect) != 0:
				continue
			case len(invSendQueue) == 0:
				trickleTimer.Reset(trickleTimeout())
				continue
			}

			// Create and send as many inv messages as needed to
			// drain the inventory send queue.
			invMsg := wire.NewMsgInvSizeHint(uint(len(invSendQueue)))

			for _, iv := range invSendQueue {
				// Don't send inventory that became known after
				// the initial check.
				if p.knownInventory.Contains(*iv) {
					continue
				}

				invMsg.AddInvVect(iv)
				if len(invMsg.InvList) >= maxInvTrickleSize {
					waiting = queuePacket(
						outMsg{msg: invMsg},
						&pendingMsgs, waiting)
					invMsg = wire.NewMsgInvSizeHint(uint(len(invSendQueue)))
				}

				// Add the inventory that is being relayed to
				// the known inventory for the peer.
				p.AddKnownInventory(iv)
			}
			if len(invMsg.InvList) > 0 {
				waiting = queuePacket(outMsg{msg: invMsg},
					&pendingMsgs, waiting)
			}
			invSendQueue = nil

			trickleTimer.Reset(trickleTimeout())

		case <-p.quit:
			break out
		}
	}

	// Drain any wait channels before we go away so we don't leave something
	// waiting for us.
	for _, msg := range pendingMsgs {
		if msg.doneChan != nil {
			msg.doneChan <- struct{}{}
		}
	}
cleanup:
	for {
		select {
		case msg := <-p.outputQueue:
			if msg.doneChan != nil {
				msg.doneChan <- struct{}{}
			}
		case <-p.outputInvChan:
			// Just drain channel
		// sendDoneQueue is buffered so doesn't need draining.
		default:
			break cleanup
		}
	}
	close(p.queueQuit)
	log.Tracef("Peer queue handler done for %s", p)
}

// shouldLogWriteError returns whether or not the passed error, which is
// expected to have come from writing to the remote peer in the outHandler,
// should be logged.
func (p *Peer) shouldLogWriteError(err error) bool {
	// No logging when the peer is being forcibly disconnected.
	if atomic.LoadInt32(&p.disconnect) != 0 {
		return false
	}

	// No logging when the remote peer has been disconnected.
	if errors.Is(err, io.EOF) {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && !opErr.Temporary() {
		return false
	}

	return true
}

// outHandler handles all outgoing messages for the peer.  It must be run as a
// goroutine.  It uses a buffered channel to serialize output messages while
// allowing the sender to continue running asynchronously.
func (p *Peer) outHandler() {
	// pingTicker is used to periodically send pings to the remote peer.
	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

out:
	for {
		select {
		case msg := <-p.sendQueue:
			switch m := msg.msg.(type) {
			case *wire.MsgPing:
				// Setup ping statistics.
				p.statsMtx.Lock()
				p.lastPingNonce = m.Nonce
				p.lastPingTime = time.Now()
				p.statsMtx.Unlock()
			}

			select {
			case p.stallControl <- stallControlMsg{sccSendMessage, msg.msg}:
			case <-p.quit:
				break out
			}
			if err := p.writeMessage(msg.msg); err != nil {
				p.Disconnect()
				if p.shouldLogWriteError(err) {
					log.Errorf("Failed to send message to "+
						"%s: %v", p, err)
				}
				if msg.doneChan != nil {
					msg.doneChan <- struct{}{}
				}
				continue
			}

			// At this point, the message was successfully sent, so
			// update the last send time, signal the sender of the
			// message that it has been sent (if requested), and
			// signal the send queue to the deliver the next queued
			// message.
			atomic.StoreInt64(&p.lastSend, time.Now().Unix())
			if msg.doneChan != nil {
				msg.doneChan <- struct{}{}
			}
			p.sendDoneQueue <- struct{}{}

		case <-pingTicker.C:
			nonce := rand.Uint64()
			p.QueueMessage(wire.NewMsgPing(nonce), nil)

		case <-p.quit:
			break out
		}
	}

	<-p.queueQuit

	// Drain any wait channels before we go away so we don't leave something
	// waiting for us. We have waited on queueQuit and thus we can be sure
	// that we will not miss anything sent on sendQueue.
cleanup:
	for {
		select {
		case msg := <-p.sendQueue:
			if msg.doneChan != nil {
				msg.doneChan <- struct{}{}
			}
			// no need to send on sendDoneQueue since queueHandler
			// has been waited on and already exited.
		default:
			break cleanup
		}
	}
	close(p.outQuit)
	log.Tracef("Peer output handler done for %s", p)
}

// QueueMessage adds the passed wire message to the peer send queue.
//
// This function is safe for concurrent access.
func (p *Peer) QueueMessage(msg wire.Message, doneChan chan<- struct{}) {
	// Provide debug information when called with a nil message.  This
	// provides a more useful stack trace to callers than hitting the
	// panic in a long-lived peer goroutine that contains no information
	// about what caller queued the nil message.
	if msg == nil {
		// The semantics of whether a non-nil doneChan should be sent
		// to or not, and the unknown program state this might lead
		// to, doesn't leave a reasonable option to recover from this.
		if doneChan != nil {
			panic("peer: queued nil message")
		}

		log.Warnf("Attempt to send nil message type %T\nStack: %s",
			msg, debug.Stack())
		return
	}

	// Avoid risk of deadlock if goroutine already exited.  The goroutine
	// we will be sending to hangs around until it knows for a fact that
	// it is marked as disconnected and *then* it drains the channels.
	if !p.Connected() {
		if doneChan != nil {
			go func() {
				doneChan <- struct{}{}
			}()
		}
		return
	}
	p.outputQueue <- outMsg{msg: msg, doneChan: doneChan}
}

// QueueInventory adds the passed inventory to the inventory send queue which
// might not be sent right away, rather it is trickled to the peer in batches.
// Inventory that the peer is already known to have is ignored.
//
// This function is safe for concurrent access.
func (p *Peer) QueueInventory(invVect *wire.InvVect) {
	// Don't add the inventory to the send queue if the peer is already
	// known to have it.
	if p.knownInventory.Contains(*invVect) {
		return
	}

	// Avoid risk of deadlock if goroutine already exited.  The goroutine
	// we will be sending to hangs around until it knows for a fact that
	// it is marked as disconnected and *then* it drains the channels.
	if !p.Connected() {
		return
	}

	p.outputInvChan <- invVect
}

// QueueInventoryImmediate adds the passed inventory to the send queue to be
// sent immediately.  This should typically only be used for inventory that is
// time sensitive such as new tip blocks or votes.  Normal inventory should be
// announced via QueueInventory which instead trickles it to the peer in
// batches.  Inventory that the peer is already known to have is ignored.
//
// This function is safe for concurrent access.
func (p *Peer) QueueInventoryImmediate(invVect *wire.InvVect) {
	// Don't announce the inventory if the peer is already known to have it.
	if p.knownInventory.Contains(*invVect) {
		return
	}

	// Avoid risk of deadlock if goroutine already exited.  The goroutine
	// we will be sending to hangs around until it knows for a fact that
	// it is marked as disconnected and *then* it drains the channels.
	if !p.Connected() {
		return
	}

	// Generate and queue a single inv message with the inventory vector.
	invMsg := wire.NewMsgInvSizeHint(1)
	invMsg.AddInvVect(invVect)
	p.AddKnownInventory(invVect)
	p.outputQueue <- outMsg{msg: invMsg, doneChan: nil}
}

// Connected returns whether or not the peer is currently connected.
//
// This function is safe for concurrent access.
func (p *Peer) Connected() bool {
	p.connMtx.Lock()
	defer p.connMtx.Unlock()

	return p.conn != nil && atomic.LoadInt32(&p.disconnect) == 0
}

// Disconnect disconnects the peer by closing the connection.  Calling this
// function when the peer is already disconnected or in the process of
// disconnecting will have no effect.
func (p *Peer) Disconnect() {
	p.connMtx.Lock()
	defer p.connMtx.Unlock()

	log.Tracef("Disconnecting %s", p)
	if p.conn != nil {
		p.conn.Close()
	}

	if atomic.CompareAndSwapInt32(&p.disconnect, 0, 1) {
		close(p.quit)
	}
}

// readRemoteVersionMsg waits for the next message to arrive from the remote
// peer.  If the next message is not a version message or the version is not
// acceptable then return an error.
func (p *Peer) readRemoteVersionMsg() error {
	// Read their version message.
	remoteMsg, _, err := p.readMessage()
	if err != nil {
		return err
	}

	// Disconnect clients if the first message is not a version message.
	msg, ok := remoteMsg.(*wire.MsgVersion)
	if !ok {
		return errors.New("a version message must precede all others")
	}

	// Detect self connections.
	if !allowSelfConns && sentNonces.Contains(msg.Nonce) {
		return errors.New("disconnecting peer connected to self")
	}

	// Negotiate the protocol version and set the services to what the remote
	// peer advertised.
	p.flagsMtx.Lock()
	p.advertisedProtoVer = uint32(msg.ProtocolVersion)
	p.protocolVersion = minUint32(p.protocolVersion, p.advertisedProtoVer)
	p.versionKnown = true
	p.services = msg.Services
	p.na.Services = msg.Services
	p.flagsMtx.Unlock()
	log.Debugf("Negotiated protocol version %d for peer %s",
		p.protocolVersion, p)

	// Updating a bunch of stats.
	p.statsMtx.Lock()
	p.lastBlock = int64(msg.LastBlock)
	p.startingHeight = int64(msg.LastBlock)

	// Set the peer's time offset.
	p.timeOffset = msg.Timestamp.Unix() - time.Now().Unix()
	p.statsMtx.Unlock()

	// Set the peer's ID and user agent.
	p.flagsMtx.Lock()
	p.id = atomic.AddInt32(&nodeCount, 1)
	p.userAgent = msg.UserAgent
	p.flagsMtx.Unlock()

	// Invoke the callback if specified.
	if p.cfg.Listeners.OnVersion != nil {
		p.cfg.Listeners.OnVersion(p, msg)
	}

	// Disconnect clients that have a protocol version that is too old.
	const reqProtocolVersion = int32(wire.RemoveRejectVersion)
	if msg.ProtocolVersion < reqProtocolVersion {
		return fmt.Errorf("protocol version must be %d or greater",
			reqProtocolVersion)
	}

	return nil
}

// localVersionMsg creates a version message that can be used to send to the
// remote peer.
func (p *Peer) localVersionMsg() (*wire.MsgVersion, error) {
	var blockNum int64
	if p.cfg.NewestBlock != nil {
		var err error
		_, blockNum, err = p.cfg.NewestBlock()
		if err != nil {
			return nil, err
		}
	}

	theirNA := p.NA()

	// If we are behind a proxy and the connection comes from the proxy then
	// we return an unroutable address as their address. This is to prevent
	// leaking the tor proxy address.
	if p.cfg.Proxy != "" {
		proxyaddress, _, err := net.SplitHostPort(p.cfg.Proxy)
		// invalid proxy means poorly configured, be on the safe side.
		if err != nil || p.na.IP.String() == proxyaddress {
			theirNA = wire.NewNetAddressIPPort(net.IP([]byte{0, 0, 0, 0}), 0,
				theirNA.Services)
		}
	}

	// Create a wire.NetAddress with only the services set to use as the
	// "addrme" in the version message.
	//
	// Older nodes previously added the IP and port information to the
	// address manager which proved to be unreliable as an inbound
	// connection from a peer didn't necessarily mean the peer itself
	// accepted inbound connections.
	//
	// Also, the timestamp is unused in the version message.
	ourNA := &wire.NetAddress{
		Services: p.cfg.Services,
	}

	// Generate a unique nonce for this peer so self connections can be
	// detected.  This is accomplished by adding it to a size-limited map of
	// recently seen nonces.
	nonce, err := wire.RandomUint64()
	if err != nil {
		return nil, err
	}
	sentNonces.Put(nonce)

	// Version message.
	msg := wire.NewMsgVersion(ourNA, theirNA, nonce, int32(blockNum))
	msg.AddUserAgent(p.cfg.UserAgentName, p.cfg.UserAgentVersion,
		p.cfg.UserAgentComments...)

	// Advertise local services.
	msg.Services = p.cfg.Services

	// Advertise our max supported protocol version.
	msg.ProtocolVersion = int32(p.ProtocolVersion())

	// Advertise if inv messages for transactions are desired.
	msg.DisableRelayTx = p.cfg.DisableRelayTx

	return msg, nil
}

// writeLocalVersionMsg writes our version message to the remote peer.
func (p *Peer) writeLocalVersionMsg() error {
	localVerMsg, err := p.localVersionMsg()
	if err != nil {
		return err
	}

	if err := p.writeMessage(localVerMsg); err != nil {
		return err
	}

	p.flagsMtx.Lock()
	p.versionSent = true
	p.flagsMtx.Unlock()
	return nil
}

// negotiateInboundProtocol waits to receive a version message from the peer
// then sends our version message. If the events do not occur in that order then
// it returns an error.
func (p *Peer) negotiateInboundProtocol() error {
	if err := p.readRemoteVersionMsg(); err != nil {
		return err
	}

	return p.writeLocalVersionMsg()
}

// negotiateOutboundProtocol sends our version message then waits to receive a
// version message from the peer.  If the events do not occur in that order then
// it returns an error.
func (p *Peer) negotiateOutboundProtocol() error {
	if err := p.writeLocalVersionMsg(); err != nil {
		return err
	}

	return p.readRemoteVersionMsg()
}

// start begins processing input and output messages.
func (p *Peer) start() error {
	log.Tracef("Starting peer %s", p)

	negotiateErr := make(chan error, 1)
	go func() {
		if p.inbound {
			negotiateErr <- p.negotiateInboundProtocol()
		} else {
			negotiateErr <- p.negotiateOutboundProtocol()
		}
	}()

	// Negotiate the protocol within the specified negotiateTimeout.
	select {
	case err := <-negotiateErr:
		if err != nil {
			p.Disconnect()
			return err
		}
	case <-time.After(negotiateTimeout):
		p.Disconnect()
		return errors.New("protocol negotiation timeout")
	}
	log.Debugf("Connected to %s", p.Addr())

	// The protocol has been negotiated successfully so start processing input
	// and output messages.
	go p.stallHandler()
	go p.inHandler()
	go p.queueHandler()
	go p.outHandler()

	// Send our verack message now that the IO processing machinery has started.
	p.QueueMessage(wire.NewMsgVerAck(), nil)
	return nil
}

// AssociateConnection associates the given conn to the peer.
// Calling this function when the peer is already connected will
// have no effect.
func (p *Peer) AssociateConnection(conn net.Conn) {
	p.connMtx.Lock()

	// Already connected?
	if p.conn != nil {
		p.connMtx.Unlock()
		return
	}

	p.conn = conn
	p.connMtx.Unlock()

	p.statsMtx.Lock()
	p.timeConnected = time.Now()
	p.statsMtx.Unlock()

	if p.inbound {
		p.addr = p.conn.RemoteAddr().String()

		// Set up a NetAddress for the peer to be used with AddrManager.  We
		// only do this inbound because outbound set this up at connection time
		// and no point recomputing.
		na, err := newNetAddress(p.conn.RemoteAddr(), p.services)
		if err != nil {
			log.Errorf("Cannot create remote net address: %v", err)
			p.Disconnect()
			return
		}
		p.flagsMtx.Lock()
		p.na = na
		p.flagsMtx.Unlock()
	}

	go func(peer *Peer) {
		if err := peer.start(); err != nil {
			log.Debugf("Cannot start peer %v: %v", peer, err)
			peer.Disconnect()
		}
	}(p)
}

// WaitForDisconnect waits until the peer has completely disconnected and all
// resources are cleaned up.  This will happen if either the local or remote
// side has been disconnected or the peer is forcibly disconnected via
// Disconnect.
func (p *Peer) WaitForDisconnect() {
	<-p.quit
}

// newPeerBase returns a new base Decred peer based on the inbound flag.  This
// is used by the NewInboundPeer and NewOutboundPeer functions to perform base
// setup needed by both types of peers.
func newPeerBase(cfgOrig *Config, inbound bool) *Peer {
	// Copy to avoid mutating the caller and so the caller can't mutate.
	cfg := *cfgOrig

	// Default to the max supported protocol version.  Override to the
	// version specified by the caller if configured.
	protocolVersion := MaxProtocolVersion
	if cfg.ProtocolVersion != 0 {
		protocolVersion = cfg.ProtocolVersion
	}

	// Set the network if the caller did not specify one.  The default is
	// testnet.
	if cfg.Net == 0 {
		cfg.Net = wire.TestNet3
	}

	// Set a default idle timeout if the caller did not specify one.
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = defaultIdleTimeout
	}

	p := Peer{
		blake256Hasher: blake256.New(),
		inbound:        inbound,
		knownInventory: lru.NewSetWithDefaultTTL[wire.InvVect](
			maxKnownInventory, maxKnownInventoryTTL),
		stallControl:    make(chan stallControlMsg, 1), // nonblocking sync
		outputQueue:     make(chan outMsg, outputBufferSize),
		sendQueue:       make(chan outMsg, 1),   // nonblocking sync
		sendDoneQueue:   make(chan struct{}, 1), // nonblocking sync
		outputInvChan:   make(chan *wire.InvVect, outputBufferSize),
		inQuit:          make(chan struct{}),
		queueQuit:       make(chan struct{}),
		outQuit:         make(chan struct{}),
		quit:            make(chan struct{}),
		cfg:             cfg,
		services:        cfg.Services,
		protocolVersion: protocolVersion,
	}
	return &p
}

// NewInboundPeer returns a new inbound Decred peer. Use Start to begin
// processing incoming and outgoing messages.
func NewInboundPeer(cfg *Config) *Peer {
	return newPeerBase(cfg, true)
}

// NewOutboundPeer returns a new outbound Decred peer.
func NewOutboundPeer(cfg *Config, addr string) (*Peer, error) {
	p := newPeerBase(cfg, false)
	p.addr = addr

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}

	if cfg.HostToNetAddress != nil {
		na, err := cfg.HostToNetAddress(host, uint16(port), 0)
		if err != nil {
			return nil, err
		}
		p.na = na
	} else {
		p.na = wire.NewNetAddressIPPort(net.ParseIP(host), uint16(port), 0)
	}

	return p, nil
}
