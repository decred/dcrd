// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcserver

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/crypto/ripemd160"
	"github.com/decred/dcrd/dcrjson/v4"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/internal/blockchain"
	"github.com/decred/dcrd/internal/mining"
	"github.com/decred/dcrd/rpc/jsonrpc/types/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/txscript/v4/stdscript"
	"github.com/decred/dcrd/wire"
)

const (
	// websocketSendBufferSize is the number of elements the send channel
	// can queue before blocking.  Note that this only applies to requests
	// handled directly in the websocket client input handler or the async
	// handler since notifications have their own queuing mechanism
	// independent of the send channel buffer.
	websocketSendBufferSize = 50

	// websocketReadLimitUnauthenticated is the maximum number of bytes allowed
	// for an unauthenticated JSON-RPC message read from a websocket client.
	websocketReadLimitUnauthenticated = 1 << 12 // 4 KiB

	// websocketReadLimitAuthenticated is the maximum number of bytes allowed
	// for an authenticated JSON-RPC message read from a websocket client.
	websocketReadLimitAuthenticated = 1 << 24 // 16 MiB

	// websocketPongTimeout is the maximum amount of time attempts to respond to
	// websocket ping messages with a pong will wait before giving up.
	websocketPongTimeout = time.Second * 5
)

type semaphore chan struct{}

func makeSemaphore(n int) semaphore {
	return make(chan struct{}, n)
}

func (s semaphore) acquire() { s <- struct{}{} }
func (s semaphore) release() { <-s }

// timeZeroVal is simply the zero value for a time.Time and is used to avoid
// creating multiple instances.
var timeZeroVal time.Time

// wsCommandHandler describes a callback function used to handle a specific
// command.
type wsCommandHandler func(context.Context, *wsClient, interface{}) (interface{}, error)

// wsHandlers maps RPC command strings to appropriate websocket handler
// functions.  This is set by init because help references wsHandlers and thus
// causes a dependency loop.
var wsHandlers map[types.Method]wsCommandHandler
var wsHandlersBeforeInit = map[types.Method]wsCommandHandler{
	"help":                      handleWebsocketHelp,
	"loadtxfilter":              handleLoadTxFilter,
	"notifyblocks":              handleNotifyBlocks,
	"notifywork":                handleNotifyWork,
	"notifytspend":              handleNotifyTSpend,
	"notifywinningtickets":      handleWinningTickets,
	"notifynewtickets":          handleNewTickets,
	"notifynewtransactions":     handleNotifyNewTransactions,
	"rebroadcastwinners":        handleRebroadcastWinners,
	"rescan":                    handleRescan,
	"session":                   handleSession,
	"stopnotifyblocks":          handleStopNotifyBlocks,
	"stopnotifywork":            handleStopNotifyWork,
	"stopnotifytspend":          handleStopNotifyTSpend,
	"stopnotifynewtransactions": handleStopNotifyNewTransactions,
}

// WebsocketHandler handles a new websocket client by creating a new wsClient,
// starting it, and blocking until the connection closes.  Since it blocks, it
// must be run in a separate goroutine.  It should be invoked from the websocket
// server handler which runs each new connection in a new goroutine thereby
// satisfying the requirement.
func (s *Server) WebsocketHandler(ctx context.Context, conn *websocket.Conn, remoteAddr string, authenticated bool, isAdmin bool) {
	// Clear the read deadline that was set before the websocket hijacked
	// the connection.
	conn.SetReadDeadline(timeZeroVal)

	// Limit max number of websocket clients.
	log.Infof("New websocket client %s", remoteAddr)
	if s.ntfnMgr.NumClients()+1 > s.cfg.RPCMaxWebsockets {
		log.Infof("Max websocket clients exceeded [%d] - "+
			"disconnecting client %s", s.cfg.RPCMaxWebsockets,
			remoteAddr)
		conn.Close()
		return
	}

	// Create a new websocket client to handle the new websocket connection
	// and wait for it to shutdown.  Once it has shutdown (and hence
	// disconnected), remove it and any notifications it registered for.
	client, err := newWebsocketClient(s, conn, remoteAddr, authenticated, isAdmin)
	if err != nil {
		log.Errorf("Failed to serve client %s: %v", remoteAddr, err)
		conn.Close()
		return
	}
	s.ntfnMgr.AddClient(client)
	client.Run(ctx)
	s.ntfnMgr.RemoveClient(client)
	log.Infof("Disconnected websocket client %s", remoteAddr)
}

// wsNotificationManager is a connection and notification manager used for
// websockets.  It allows websocket clients to register for notifications they
// are interested in.  When an event happens elsewhere in the code such as
// transactions being added to the memory pool or block connects/disconnects,
// the notification manager is provided with the relevant details needed to
// figure out which websocket clients need to be notified based on what they
// have registered for and notifies them accordingly.  It is also used to keep
// track of all connected websocket clients.
type wsNotificationManager struct {
	// server is the RPC server the notification manager is associated with.
	server *Server

	// queueNotification queues a notification for handling.
	queueNotification chan interface{}

	// notificationMsgs feeds notificationHandler with notifications
	// and client (un)registeration requests from a queue as well as
	// registeration and unregisteration requests from clients.
	notificationMsgs chan interface{}

	// Access channel for current number of connected clients.
	numClients chan int

	// The following fields are used for lifecycle management of the
	// notification manager.
	wg   sync.WaitGroup
	quit chan struct{}
}

// queueHandler maintains a queue of notifications and notification handler
// control messages. The handler stops when the input channel is closed or a
// context cancellation signal is received.
func (m *wsNotificationManager) queueHandler(ctx context.Context) {
	var q []interface{}
	var dequeue chan<- interface{}
	skipQueue := m.notificationMsgs
	var next interface{}

	for {
		select {
		case <-ctx.Done():
			close(m.notificationMsgs)
			m.wg.Done()
			return

		case n := <-m.queueNotification:
			// Either send to out immediately if skipQueue is
			// non-nil (queue is empty) and reader is ready,
			// or append to the queue and send later.
			select {
			case skipQueue <- n:
			default:
				q = append(q, n)
				dequeue = m.notificationMsgs
				skipQueue = nil
				next = q[0]
			}

		case dequeue <- next:
			copy(q, q[1:])
			q[len(q)-1] = nil // avoid leak
			q = q[:len(q)-1]
			if len(q) == 0 {
				dequeue = nil
				skipQueue = m.notificationMsgs
			} else {
				next = q[0]
			}
		}
	}
}

// NotifyBlockConnected passes a block newly-connected to the best chain
// to the notification manager for block and transaction notification
// processing.
func (m *wsNotificationManager) NotifyBlockConnected(block *dcrutil.Block) {
	select {
	case m.queueNotification <- (*notificationBlockConnected)(block):
	case <-m.quit:
	}
}

// NotifyBlockDisconnected passes a block disconnected from the best chain
// to the notification manager for block notification processing.
func (m *wsNotificationManager) NotifyBlockDisconnected(block *dcrutil.Block) {
	select {
	case m.queueNotification <- (*notificationBlockDisconnected)(block):
	case <-m.quit:
	}
}

// NotifyWork passes new mining work to the notification manager
// for block notification processing.
func (m *wsNotificationManager) NotifyWork(templateNtfn *mining.TemplateNtfn) {
	select {
	case m.queueNotification <- (*notificationWork)(templateNtfn):
	case <-m.quit:
	}
}

// NotifyTSpend passes new tspends for mempool additions.
func (m *wsNotificationManager) NotifyTSpend(tx *dcrutil.Tx) {
	select {
	case m.queueNotification <- (*notificationTSpend)(tx):
	case <-m.quit:
	}
}

// NotifyReorganization passes a blockchain reorganization notification for
// reorganization notification processing.
func (m *wsNotificationManager) NotifyReorganization(rd *blockchain.ReorganizationNtfnsData) {
	select {
	case m.queueNotification <- (*notificationReorganization)(rd):
	case <-m.quit:
	}
}

// NotifyWinningTickets passes newly winning tickets for an incoming block
// to the notification manager for further processing.
func (m *wsNotificationManager) NotifyWinningTickets(wtnd *WinningTicketsNtfnData) {
	select {
	case m.queueNotification <- (*notificationWinningTickets)(wtnd):
	case <-m.quit:
	}
}

// NotifyNewTickets passes a new ticket data for an incoming block from the best
// chain to the notification manager for block notification processing.
func (m *wsNotificationManager) NotifyNewTickets(tnd *blockchain.TicketNotificationsData) {
	select {
	case m.queueNotification <- (*notificationNewTickets)(tnd):
	case <-m.quit:
	}
}

// NotifyMempoolTx passes a transaction accepted by mempool to the
// notification manager for transaction notification processing.  If
// isNew is true, the tx is a new transaction, rather than one
// added to the mempool during a reorg.
func (m *wsNotificationManager) NotifyMempoolTx(tx *dcrutil.Tx, isNew bool) {
	n := &notificationTxAcceptedByMempool{
		isNew: isNew,
		tx:    tx,
	}

	select {
	case m.queueNotification <- n:
	case <-m.quit:
	}
}

// WinningTicketsNtfnData is the data that is used to generate
// winning ticket notifications (which indicate a block and
// the tickets eligible to vote on it).
type WinningTicketsNtfnData struct {
	BlockHash   chainhash.Hash
	BlockHeight int64
	Tickets     []chainhash.Hash
}

type wsClientFilter struct {
	mu sync.Mutex

	// Parameter for address decoding.
	params stdaddr.AddressParams

	// Implemented fast paths for address lookup.
	pubKeyHashes      map[[ripemd160.Size]byte]struct{}
	scriptHashes      map[[ripemd160.Size]byte]struct{}
	compressedPubKeys map[[33]byte]struct{}

	// A fallback address lookup map in case a fast path doesn't exist.
	// Only exists for completeness.  If using this shows up in a profile,
	// there's a good chance a fast path should be added.
	otherAddresses map[string]struct{}

	// Outpoints of unspent outputs.
	unspent map[wire.OutPoint]struct{}
}

func makeWSClientFilter(addresses []string, unspentOutPoints []*wire.OutPoint, params stdaddr.AddressParams) *wsClientFilter {
	filter := &wsClientFilter{
		params:            params,
		pubKeyHashes:      map[[ripemd160.Size]byte]struct{}{},
		scriptHashes:      map[[ripemd160.Size]byte]struct{}{},
		compressedPubKeys: map[[33]byte]struct{}{},
		otherAddresses:    map[string]struct{}{},
		unspent:           make(map[wire.OutPoint]struct{}, len(unspentOutPoints)),
	}

	for _, s := range addresses {
		filter.addAddressStr(s)
	}
	for _, op := range unspentOutPoints {
		filter.addUnspentOutPoint(op)
	}

	return filter
}

func (f *wsClientFilter) addAddress(a stdaddr.Address) {
	switch a := a.(type) {
	case *stdaddr.AddressPubKeyHashEcdsaSecp256k1V0:
		f.pubKeyHashes[*a.Hash160()] = struct{}{}
		return
	case *stdaddr.AddressScriptHashV0:
		f.scriptHashes[*a.Hash160()] = struct{}{}
		return
	case *stdaddr.AddressPubKeyEcdsaSecp256k1V0:
		serializedPubKey := a.SerializedPubKey()
		switch len(serializedPubKey) {
		case 33: // compressed
			var compressedPubKey [33]byte
			copy(compressedPubKey[:], serializedPubKey)
			f.compressedPubKeys[compressedPubKey] = struct{}{}
			return
		}
	}

	f.otherAddresses[a.String()] = struct{}{}
}

func (f *wsClientFilter) addAddressStr(s string) {
	a, err := stdaddr.DecodeAddress(s, f.params)
	if err != nil {
		// There is no point in saving the address if it can't be decoded since
		// it should also be impossible to create the address from an inspected
		// transaction output script.
		return
	}
	f.addAddress(a)
}

func (f *wsClientFilter) existsAddress(a stdaddr.Address) bool {
	switch a := a.(type) {
	case *stdaddr.AddressPubKeyHashEcdsaSecp256k1V0:
		_, ok := f.pubKeyHashes[*a.Hash160()]
		return ok
	case *stdaddr.AddressScriptHashV0:
		_, ok := f.scriptHashes[*a.Hash160()]
		return ok
	case *stdaddr.AddressPubKeyEcdsaSecp256k1V0:
		serializedPubKey := a.SerializedPubKey()
		switch len(serializedPubKey) {
		case 33: // compressed
			var compressedPubKey [33]byte
			copy(compressedPubKey[:], serializedPubKey)
			_, ok := f.compressedPubKeys[compressedPubKey]
			if !ok {
				h160 := a.AddressPubKeyHash().(stdaddr.Hash160er).Hash160()
				_, ok = f.pubKeyHashes[*h160]
			}
			return ok
		}
	}

	_, ok := f.otherAddresses[a.String()]
	return ok
}

func (f *wsClientFilter) addUnspentOutPoint(op *wire.OutPoint) {
	f.unspent[*op] = struct{}{}
}

func (f *wsClientFilter) existsUnspentOutPoint(op *wire.OutPoint) bool {
	_, ok := f.unspent[*op]
	return ok
}

// Notification types
type notificationBlockConnected dcrutil.Block
type notificationBlockDisconnected dcrutil.Block
type notificationWork mining.TemplateNtfn
type notificationTSpend dcrutil.Tx
type notificationReorganization blockchain.ReorganizationNtfnsData
type notificationWinningTickets WinningTicketsNtfnData
type notificationNewTickets blockchain.TicketNotificationsData
type notificationTxAcceptedByMempool struct {
	isNew bool
	tx    *dcrutil.Tx
}

// Notification control requests
type notificationRegisterClient wsClient
type notificationUnregisterClient wsClient
type notificationRegisterBlocks wsClient
type notificationUnregisterBlocks wsClient
type notificationRegisterWork wsClient
type notificationUnregisterWork wsClient
type notificationRegisterTSpend wsClient
type notificationUnregisterTSpend wsClient
type notificationRegisterWinningTickets wsClient
type notificationUnregisterWinningTickets wsClient
type notificationRegisterNewTickets wsClient
type notificationUnregisterNewTickets wsClient
type notificationRegisterNewMempoolTxs wsClient
type notificationUnregisterNewMempoolTxs wsClient

// notificationHandler reads notifications and control messages from the queue
// handler and processes one at a time.
func (m *wsNotificationManager) notificationHandler(ctx context.Context) {
	// clients is a map of all currently connected websocket clients.
	clients := make(map[chan struct{}]*wsClient)

	// Maps used to hold lists of websocket clients to be notified on
	// certain events.  Each websocket client also keeps maps for the events
	// which have multiple triggers to make removal from these lists on
	// connection close less horrendously expensive.
	//
	// Where possible, the quit channel is used as the unique id for a client
	// since it is quite a bit more efficient than using the entire struct.
	blockNotifications := make(map[chan struct{}]*wsClient)
	workNotifications := make(map[chan struct{}]*wsClient)
	tspendNotifications := make(map[chan struct{}]*wsClient)
	winningTicketNotifications := make(map[chan struct{}]*wsClient)
	ticketNewNotifications := make(map[chan struct{}]*wsClient)
	txNotifications := make(map[chan struct{}]*wsClient)

out:
	for {
		select {
		case <-ctx.Done():
			// RPC server shutdown.
			break out

		case n, ok := <-m.notificationMsgs:
			if !ok {
				// queueHandler quit.
				break out
			}
			switch n := n.(type) {
			case *notificationBlockConnected:
				m.notifyBlockConnected(blockNotifications, (*dcrutil.Block)(n))

			case *notificationBlockDisconnected:
				m.notifyBlockDisconnected(blockNotifications,
					(*dcrutil.Block)(n))

			case *notificationWork:
				m.notifyWork(workNotifications, (*mining.TemplateNtfn)(n))

			case *notificationTSpend:
				m.notifyTSpend(tspendNotifications, (*dcrutil.Tx)(n))

			case *notificationReorganization:
				m.notifyReorganization(blockNotifications,
					(*blockchain.ReorganizationNtfnsData)(n))

			case *notificationWinningTickets:
				m.notifyWinningTickets(winningTicketNotifications,
					(*WinningTicketsNtfnData)(n))

			case *notificationNewTickets:
				m.notifyNewTickets(ticketNewNotifications,
					(*blockchain.TicketNotificationsData)(n))

			case *notificationTxAcceptedByMempool:
				if n.isNew && len(txNotifications) != 0 {
					m.notifyForNewTx(txNotifications, n.tx)
				}
				m.notifyRelevantTxAccepted(n.tx, clients)

			case *notificationRegisterBlocks:
				wsc := (*wsClient)(n)
				blockNotifications[wsc.quit] = wsc

			case *notificationUnregisterBlocks:
				wsc := (*wsClient)(n)
				delete(blockNotifications, wsc.quit)

			case *notificationRegisterWork:
				wsc := (*wsClient)(n)
				workNotifications[wsc.quit] = wsc

			case *notificationUnregisterWork:
				wsc := (*wsClient)(n)
				delete(workNotifications, wsc.quit)

			case *notificationRegisterTSpend:
				wsc := (*wsClient)(n)
				tspendNotifications[wsc.quit] = wsc

			case *notificationUnregisterTSpend:
				wsc := (*wsClient)(n)
				delete(tspendNotifications, wsc.quit)

			case *notificationRegisterWinningTickets:
				wsc := (*wsClient)(n)
				winningTicketNotifications[wsc.quit] = wsc

			case *notificationUnregisterWinningTickets:
				wsc := (*wsClient)(n)
				delete(winningTicketNotifications, wsc.quit)

			case *notificationRegisterNewTickets:
				wsc := (*wsClient)(n)
				ticketNewNotifications[wsc.quit] = wsc

			case *notificationUnregisterNewTickets:
				wsc := (*wsClient)(n)
				delete(ticketNewNotifications, wsc.quit)

			case *notificationRegisterClient:
				wsc := (*wsClient)(n)
				clients[wsc.quit] = wsc

			case *notificationUnregisterClient:
				wsc := (*wsClient)(n)
				// Remove any requests made by the client as well as
				// the client itself.
				delete(blockNotifications, wsc.quit)
				delete(workNotifications, wsc.quit)
				delete(tspendNotifications, wsc.quit)
				delete(txNotifications, wsc.quit)
				delete(winningTicketNotifications, wsc.quit)
				delete(ticketNewNotifications, wsc.quit)
				delete(clients, wsc.quit)

			case *notificationRegisterNewMempoolTxs:
				wsc := (*wsClient)(n)
				txNotifications[wsc.quit] = wsc

			case *notificationUnregisterNewMempoolTxs:
				wsc := (*wsClient)(n)
				delete(txNotifications, wsc.quit)

			default:
				log.Warnf("Unhandled notification type: %T", n)
			}

		case m.numClients <- len(clients):
		}
	}

	for _, c := range clients {
		c.Disconnect()
	}
	m.wg.Done()
}

// NumClients returns the number of clients actively being served.
func (m *wsNotificationManager) NumClients() int {
	var n int
	select {
	case n = <-m.numClients:
	case <-m.quit: // Use default n (0) if server has shut down.
	}
	return n
}

// RegisterBlockUpdates requests block update notifications to the passed
// websocket client.
func (m *wsNotificationManager) RegisterBlockUpdates(wsc *wsClient) {
	select {
	case m.queueNotification <- (*notificationRegisterBlocks)(wsc):
	case <-m.quit:
	}
}

// UnregisterBlockUpdates removes block update notifications for the passed
// websocket client.
func (m *wsNotificationManager) UnregisterBlockUpdates(wsc *wsClient) {
	select {
	case m.queueNotification <- (*notificationUnregisterBlocks)(wsc):
	case <-m.quit:
	}
}

// RegisterWorkUpdates requests work update notifications to the passed
// websocket client.
func (m *wsNotificationManager) RegisterWorkUpdates(wsc *wsClient) {
	select {
	case m.queueNotification <- (*notificationRegisterWork)(wsc):
	case <-m.quit:
	}
}

// UnregisterWorkUpdates removes work update notifications for the passed
// websocket client.
func (m *wsNotificationManager) UnregisterWorkUpdates(wsc *wsClient) {
	select {
	case m.queueNotification <- (*notificationUnregisterWork)(wsc):
	case <-m.quit:
	}
}

// RegisterTSpendUpdates requests tspend update notifications to the passed
// websocket client.
func (m *wsNotificationManager) RegisterTSpendUpdates(wsc *wsClient) {
	select {
	case m.queueNotification <- (*notificationRegisterTSpend)(wsc):
	case <-m.quit:
	}
}

// UnregisterTSpendUpdates removes tspend update notifications for the passed
// websocket client.
func (m *wsNotificationManager) UnregisterTSpendUpdates(wsc *wsClient) {
	select {
	case m.queueNotification <- (*notificationUnregisterTSpend)(wsc):
	case <-m.quit:
	}
}

// subscribedClients returns the set of all websocket client quit channels that
// are registered to receive notifications regarding tx, either due to tx
// spending a watched output or outputting to a watched address.  Matching
// client's filters are updated based on this transaction's outputs and output
// addresses that may be relevant for a client.
func (m *wsNotificationManager) subscribedClients(tx *dcrutil.Tx, clients map[chan struct{}]*wsClient) map[chan struct{}]struct{} {
	// Use a map of client quit channels as keys to prevent duplicates when
	// multiple inputs and/or outputs are relevant to the client.
	subscribed := make(map[chan struct{}]struct{})

	// Reusable backing array for a slice of a single address.
	var scratchAddress [1]stdaddr.Address

	// Local for convenience.
	params := m.server.cfg.ChainParams

	msgTx := tx.MsgTx()
	var isTicket bool // lazily set
	for q, c := range clients {
		c.Lock()
		f := c.filterData
		c.Unlock()
		if f == nil {
			continue
		}
		f.mu.Lock()

		for _, input := range msgTx.TxIn {
			if f.existsUnspentOutPoint(&input.PreviousOutPoint) {
				subscribed[q] = struct{}{}
			}
		}

		for i, output := range msgTx.TxOut {
			watchOutput := true
			scriptType, addrs := stdscript.ExtractAddrs(output.Version,
				output.PkScript, params)
			if scriptType == stdscript.STNonStandard {
				// Clients are not able to subscribe to nonstandard or
				// non-address outputs.
				continue
			}
			if scriptType == stdscript.STNullData && i&1 == 1 &&
				(isTicket || stake.IsSStx(msgTx)) {

				isTicket = true
				// OP_RETURN ticket commitments may contain relevant
				// P2PKH or P2SH HASH160s.
				// These outputs cannot be spent and do not need to
				// be watched.
				addr, err := stake.AddrFromSStxPkScrCommitment(
					output.PkScript, params)
				if err != nil {
					log.Errorf("Failed to read commitment from "+
						"previously-validated ticket: %v", err)
					continue
				}
				scratchAddress[0] = addr
				addrs = scratchAddress[:]
				watchOutput = false
			}
			for _, a := range addrs {
				if f.existsAddress(a) {
					subscribed[q] = struct{}{}
					if watchOutput {
						op := wire.OutPoint{
							Hash:  *tx.Hash(),
							Index: uint32(i),
							Tree:  tx.Tree(),
						}
						f.addUnspentOutPoint(&op)
					}
				}
			}
		}

		f.mu.Unlock()
	}

	return subscribed
}

// notifyBlockConnected notifies websocket clients that have registered for
// block updates when a block is connected to the main chain.
func (m *wsNotificationManager) notifyBlockConnected(clients map[chan struct{}]*wsClient, block *dcrutil.Block) {
	// Skip notification creation if no clients have requested block connected
	// notifications.
	if len(clients) == 0 {
		return
	}

	// Create the common portion of the notification that is the same for
	// every client.
	headerBytes, err := block.MsgBlock().Header.Bytes()
	if err != nil {
		// This should never error.  The header is written to an
		// in-memory expandable buffer, and given that the block was
		// just accepted, there should be no issues serializing it.
		panic(err)
	}
	ntfn := types.BlockConnectedNtfn{
		Header:        hex.EncodeToString(headerBytes),
		SubscribedTxs: nil, // Set individually for each client
	}

	// Search for relevant transactions for each client and save them
	// serialized in hex encoding for the notification.
	subscribedTxs := make(map[chan struct{}][]string)
	for _, tx := range block.STransactions() {
		var txHex string
		for quitChan := range m.subscribedClients(tx, clients) {
			if txHex == "" {
				txHex = txHexString(tx.MsgTx())
			}
			subscribedTxs[quitChan] = append(subscribedTxs[quitChan], txHex)
		}
	}
	for _, tx := range block.Transactions() {
		var txHex string
		for quitChan := range m.subscribedClients(tx, clients) {
			if txHex == "" {
				txHex = txHexString(tx.MsgTx())
			}
			subscribedTxs[quitChan] = append(subscribedTxs[quitChan], txHex)
		}
	}

	for quitChan, client := range clients {
		// Add all previously discovered relevant transactions for this client,
		// if any.
		ntfn.SubscribedTxs = subscribedTxs[quitChan]

		// Marshal and queue notification.
		marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, &ntfn)
		if err != nil {
			log.Errorf("Failed to marshal block connected "+
				"notification: %v", err)
			continue
		}
		client.QueueNotification(marshalledJSON)
	}
}

// notifyBlockDisconnected notifies websocket clients that have registered for
// block updates when a block is disconnected from the main chain (due to a
// reorganize).
func (*wsNotificationManager) notifyBlockDisconnected(clients map[chan struct{}]*wsClient, block *dcrutil.Block) {
	// Skip notification creation if no clients have requested block
	// connected/disconnected notifications.
	if len(clients) == 0 {
		return
	}

	// Notify interested websocket clients about the disconnected block.
	headerBytes, err := block.MsgBlock().Header.Bytes()
	if err != nil {
		// This should never error.  The header is written to an
		// in-memory expandable buffer, and given that the block was
		// previously accepted, there should be no issues serializing
		// it.
		panic(err)
	}
	ntfn := types.BlockDisconnectedNtfn{
		Header: hex.EncodeToString(headerBytes),
	}
	marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, &ntfn)
	if err != nil {
		log.Errorf("Failed to marshal block disconnected "+
			"notification: %v", err)
		return
	}
	for _, wsc := range clients {
		wsc.QueueNotification(marshalledJSON)
	}
}

// updateReasonToWorkNtfnString converts a template update reason to a string
// which matches the reasons required return values for work notifications.
func updateReasonToWorkNtfnString(reason mining.TemplateUpdateReason) string {
	switch reason {
	case mining.TURNewParent:
		return "newparent"
	case mining.TURNewVotes:
		return "newvotes"
	case mining.TURNewTxns:
		return "newtxns"
	}

	return "unknown"
}

// notifyWork notifies websocket clients that have registered for template
// updates when a new block template is generated.
func (m *wsNotificationManager) notifyWork(clients map[chan struct{}]*wsClient, templateNtfn *mining.TemplateNtfn) {
	// Skip notification creation if no clients have requested work
	// notifications.
	if len(clients) == 0 {
		return
	}

	// Serialize the block header into a buffer large enough to hold the
	// the block header and the internal blake256 padding that is added and
	// retuned as part of the data below.
	//
	// For reference (0-index based, end value is exclusive):
	// data[115:119] --> Bits
	// data[135:139] --> Timestamp
	// data[139:143] --> Nonce
	// data[143:151] --> ExtraNonce
	header := &templateNtfn.Template.Block.Header
	data := make([]byte, 0, getworkDataLen)
	buf := bytes.NewBuffer(data)
	err := header.Serialize(buf)
	if err != nil {
		log.Errorf("Failed to serialize data: %v", err)
		return
	}

	// Expand the data slice to include the full data buffer and apply the
	// internal blake256 padding.  This makes the data ready for callers to
	// make use of only the final chunk along with the midstate for the
	// rest.
	data = data[:getworkDataLen]
	copy(data[wire.MaxBlockHeaderPayload:], blake256Pad)

	// The final result reverses each of the fields to little endian.  In
	// particular, the data, hash1, and midstate fields are treated as
	// arrays of uint32s (per the internal sha256 hashing state) which are
	// in big endian, and thus each 4 bytes is byte swapped.  The target is
	// also in big endian, but it is treated as a uint256 and byte swapped
	// to little endian accordingly.
	//
	// The fact the fields are reversed in this way is rather odd and likey
	// an artifact of some legacy internal state in the reference
	// implementation, but it is required for compatibility.
	target := bigToLEUint256(standalone.CompactToBig(header.Bits))
	ntfn := types.WorkNtfn{
		Data:   hex.EncodeToString(data),
		Target: hex.EncodeToString(target[:]),
		Reason: updateReasonToWorkNtfnString(templateNtfn.Reason),
	}

	// Notify interested websocket clients about new mining work.
	marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, &ntfn)
	if err != nil {
		log.Errorf("Failed to marshal new work notification: %v", err)
		return
	}

	// Prune old templates from the pool when the best block changes and add the
	// template to the template pool.  Since the key is a combination of the
	// merkle and stake root fields, this will not add duplicate entries for the
	// templates with modified timestamps and/or difficulty bits.
	templateKey := getWorkTemplateKey(header)
	state := m.server.workState
	state.Lock()
	if templateNtfn.Reason == mining.TURNewParent {
		best := m.server.cfg.Chain.BestSnapshot()
		state.pruneOldBlockTemplates(best.Height)
	}
	state.templatePool[templateKey] = templateNtfn.Template.Block
	state.Unlock()

	for _, wsc := range clients {
		wsc.QueueNotification(marshalledJSON)
	}
}

// notifyTSpend notifies websocket clients that have registered for mempool
// tspend arrivals.
func (m *wsNotificationManager) notifyTSpend(clients map[chan struct{}]*wsClient,
	tspend *dcrutil.Tx) {
	// Skip notification creation if no clients have requested tspend
	// notifications.
	if len(clients) == 0 {
		return
	}

	data, err := tspend.MsgTx().Bytes()
	if err != nil {
		log.Errorf("Failed to marshal new tspend notification: %v", err)
		return
	}
	ntfn := types.TSpendNtfn{
		TSpend: hex.EncodeToString(data),
	}

	// Notify interested websocket clients about new tspends.
	marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, &ntfn)
	if err != nil {
		log.Errorf("Failed to marshal new tspend notification: %v", err)
		return
	}

	for _, wsc := range clients {
		wsc.QueueNotification(marshalledJSON)
	}
}

// notifyReorganization notifies websocket clients that have registered for
// block updates when the blockchain is beginning a reorganization.
func (m *wsNotificationManager) notifyReorganization(clients map[chan struct{}]*wsClient, rd *blockchain.ReorganizationNtfnsData) {
	// Skip notification creation if no clients have requested block
	// connected/disconnected notifications.
	if len(clients) == 0 {
		return
	}

	// Notify interested websocket clients about the disconnected block.
	ntfn := types.NewReorganizationNtfn(rd.OldHash.String(),
		int32(rd.OldHeight),
		rd.NewHash.String(),
		int32(rd.NewHeight))
	marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, ntfn)
	if err != nil {
		log.Errorf("Failed to marshal reorganization "+
			"notification: %v", err)
		return
	}
	for _, wsc := range clients {
		wsc.QueueNotification(marshalledJSON)
	}
}

// RegisterWinningTickets requests winning tickets update notifications
// to the passed websocket client.
func (m *wsNotificationManager) RegisterWinningTickets(wsc *wsClient) {
	select {
	case m.queueNotification <- (*notificationRegisterWinningTickets)(wsc):
	case <-m.quit:
	}
}

// UnregisterWinningTickets removes winning ticket notifications for
// the passed websocket client.
func (m *wsNotificationManager) UnregisterWinningTickets(wsc *wsClient) {
	select {
	case m.queueNotification <- (*notificationUnregisterWinningTickets)(wsc):
	case <-m.quit:
	}
}

// notifyWinningTickets notifies websocket clients that have registered for
// winning ticket updates.
func (*wsNotificationManager) notifyWinningTickets(
	clients map[chan struct{}]*wsClient, wtnd *WinningTicketsNtfnData) {

	// Create a ticket map to export as JSON.
	ticketMap := make(map[string]string)
	for i, ticket := range wtnd.Tickets {
		ticketMap[strconv.Itoa(i)] = ticket.String()
	}

	// Notify interested websocket clients about the connected block.
	ntfn := types.NewWinningTicketsNtfn(wtnd.BlockHash.String(),
		int32(wtnd.BlockHeight), ticketMap)

	marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, ntfn)
	if err != nil {
		log.Errorf("Failed to marshal winning tickets notification: "+
			"%v", err)
		return
	}

	for _, wsc := range clients {
		wsc.QueueNotification(marshalledJSON)
	}
}

// RegisterNewTickets requests spent/missed tickets update notifications
// to the passed websocket client.
func (m *wsNotificationManager) RegisterNewTickets(wsc *wsClient) {
	select {
	case m.queueNotification <- (*notificationRegisterNewTickets)(wsc):
	case <-m.quit:
	}
}

// UnregisterNewTickets removes spent/missed ticket notifications for
// the passed websocket client.
func (m *wsNotificationManager) UnregisterNewTickets(wsc *wsClient) {
	select {
	case m.queueNotification <- (*notificationUnregisterNewTickets)(wsc):
	case <-m.quit:
	}
}

// notifyNewTickets notifies websocket clients that have registered for
// maturing ticket updates.
func (*wsNotificationManager) notifyNewTickets(clients map[chan struct{}]*wsClient, tnd *blockchain.TicketNotificationsData) {
	// Create a ticket map to export as JSON.
	tickets := make([]string, 0, len(tnd.TicketsNew))
	for _, h := range tnd.TicketsNew {
		tickets = append(tickets, h.String())
	}

	// Notify interested websocket clients about the connected block.
	ntfn := types.NewNewTicketsNtfn(tnd.Hash.String(), int32(tnd.Height),
		tnd.StakeDifficulty, tickets)

	marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, ntfn)
	if err != nil {
		log.Errorf("Failed to marshal new tickets notification: "+
			"%v", err)
		return
	}
	for _, wsc := range clients {
		wsc.QueueNotification(marshalledJSON)
	}
}

// RegisterNewMempoolTxsUpdates requests notifications to the passed websocket
// client when new transactions are added to the memory pool.
func (m *wsNotificationManager) RegisterNewMempoolTxsUpdates(wsc *wsClient) {
	select {
	case m.queueNotification <- (*notificationRegisterNewMempoolTxs)(wsc):
	case <-m.quit:
	}
}

// UnregisterNewMempoolTxsUpdates removes notifications to the passed websocket
// client when new transaction are added to the memory pool.
func (m *wsNotificationManager) UnregisterNewMempoolTxsUpdates(wsc *wsClient) {
	select {
	case m.queueNotification <- (*notificationUnregisterNewMempoolTxs)(wsc):
	case <-m.quit:
	}
}

// notifyForNewTx notifies websocket clients that have registered for updates
// when a new transaction is added to the memory pool.
func (m *wsNotificationManager) notifyForNewTx(clients map[chan struct{}]*wsClient, tx *dcrutil.Tx) {
	txHashStr := tx.Hash().String()
	mtx := tx.MsgTx()

	var amount int64
	for _, txOut := range mtx.TxOut {
		amount += txOut.Value
	}

	ntfn := types.NewTxAcceptedNtfn(txHashStr,
		dcrutil.Amount(amount).ToCoin())
	marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, ntfn)
	if err != nil {
		log.Errorf("Failed to marshal tx notification: %s",
			err.Error())
		return
	}

	// Determine if the treasury rules are active as of the current best tip.
	prevBlkHash := m.server.cfg.Chain.BestSnapshot().Hash
	isTreasuryEnabled, err := m.server.isTreasuryAgendaActive(&prevBlkHash)
	if err != nil {
		log.Errorf("Could not obtain treasury agenda status: %v", err)
		return
	}

	var verboseNtfn *types.TxAcceptedVerboseNtfn
	var marshalledJSONVerbose []byte
	for _, wsc := range clients {
		if wsc.verboseTxUpdates {
			if marshalledJSONVerbose != nil {
				wsc.QueueNotification(marshalledJSONVerbose)
				continue
			}

			net := m.server.cfg.ChainParams
			rawTx, err := m.server.createTxRawResult(net, mtx, txHashStr,
				wire.NullBlockIndex, nil, "", 0, 0, isTreasuryEnabled)
			if err != nil {
				return
			}

			verboseNtfn = types.NewTxAcceptedVerboseNtfn(*rawTx)
			marshalledJSONVerbose, err = dcrjson.MarshalCmd("1.0", nil,
				verboseNtfn)
			if err != nil {
				log.Errorf("Failed to marshal verbose tx "+
					"notification: %s", err.Error())
				return
			}
			wsc.QueueNotification(marshalledJSONVerbose)
		} else {
			wsc.QueueNotification(marshalledJSON)
		}
	}
}

// txHexString returns the serialized transaction encoded in hexadecimal.
func txHexString(tx *wire.MsgTx) string {
	buf := bytes.NewBuffer(make([]byte, 0, tx.SerializeSize()))
	// Ignore Serialize's error, as writing to a bytes.buffer cannot fail.
	tx.Serialize(buf)
	return hex.EncodeToString(buf.Bytes())
}

// notifyRelevantTxAccepted examines the inputs and outputs of the passed
// transaction, notifying websocket clients of outputs spending to a watched
// address and inputs spending a watched outpoint.  Any outputs paying to a
// watched address result in the output being watched as well for future
// notifications.
func (m *wsNotificationManager) notifyRelevantTxAccepted(tx *dcrutil.Tx,
	clients map[chan struct{}]*wsClient) {

	var clientsToNotify map[chan struct{}]*wsClient

	msgTx := tx.MsgTx()
	for q, c := range clients {
		c.Lock()
		f := c.filterData
		c.Unlock()
		if f == nil {
			continue
		}
		f.mu.Lock()

		for _, input := range msgTx.TxIn {
			if f.existsUnspentOutPoint(&input.PreviousOutPoint) {
				if clientsToNotify == nil {
					clientsToNotify = make(map[chan struct{}]*wsClient)
				}
				clientsToNotify[q] = c
			}
		}

		for i, output := range msgTx.TxOut {
			scriptType, addrs := stdscript.ExtractAddrs(output.Version,
				output.PkScript, m.server.cfg.ChainParams)
			if scriptType == stdscript.STNonStandard {
				continue
			}
			for _, a := range addrs {
				if f.existsAddress(a) {
					if clientsToNotify == nil {
						clientsToNotify = make(map[chan struct{}]*wsClient)
					}
					clientsToNotify[q] = c

					op := wire.OutPoint{
						Hash:  *tx.Hash(),
						Index: uint32(i),
						Tree:  tx.Tree(),
					}
					f.addUnspentOutPoint(&op)
				}
			}
		}

		f.mu.Unlock()
	}

	if len(clientsToNotify) != 0 {
		n := types.NewRelevantTxAcceptedNtfn(txHexString(msgTx))
		marshalled, err := dcrjson.MarshalCmd("1.0", nil, n)
		if err != nil {
			log.Errorf("Failed to marshal notification: %v", err)
			return
		}
		for _, c := range clientsToNotify {
			c.QueueNotification(marshalled)
		}
	}
}

// AddClient adds the passed websocket client to the notification manager.
func (m *wsNotificationManager) AddClient(wsc *wsClient) {
	select {
	case m.queueNotification <- (*notificationRegisterClient)(wsc):
	case <-m.quit:
	}
}

// RemoveClient removes the passed websocket client and all notifications
// registered for it.
func (m *wsNotificationManager) RemoveClient(wsc *wsClient) {
	select {
	case m.queueNotification <- (*notificationUnregisterClient)(wsc):
	case <-m.quit:
	}
}

// Run starts the goroutines required for the manager to queue and process
// websocket client notifications.  It blocks until the provided context is
// cancelled.
func (m *wsNotificationManager) Run(ctx context.Context) {
	m.wg.Add(3)
	go m.queueHandler(ctx)
	go m.notificationHandler(ctx)
	go func(ctx context.Context) {
		<-ctx.Done()
		close(m.quit)
		m.wg.Done()
	}(ctx)
	m.wg.Wait()
}

// newWsNotificationManager returns a new notification manager ready for use.
// See wsNotificationManager for more details.
func newWsNotificationManager(server *Server) *wsNotificationManager {
	return &wsNotificationManager{
		server:            server,
		queueNotification: make(chan interface{}),
		notificationMsgs:  make(chan interface{}),
		numClients:        make(chan int),
		quit:              make(chan struct{}),
	}
}

// wsResponse houses a message to send to a connected websocket client as
// well as a channel to reply on when the message is sent.
type wsResponse struct {
	msg      []byte
	doneChan chan bool
}

// wsClient provides an abstraction for handling a websocket client. The overall
// data flow is split into 3 main goroutines. A websocket manager is used to
// allow things such as broadcasting requested notifications to all connected
// websocket clients. Inbound messages are read via the inHandler goroutine and
// generally dispatched to their own handler. There are two outbound message
// types - one for responding to client requests and another for async
// notifications. Responses to client requests use SendMessage which employs a
// buffered channel thereby limiting the number of outstanding requests that can
// be made. Notifications are sent via QueueNotification which implements a
// queue via notificationQueueHandler to ensure sending notifications from other
// subsystems can't block.  Ultimately, all messages are sent via the
// outHandler.
type wsClient struct {
	disconnected atomic.Bool // Websocket client disconnected?

	sync.Mutex

	// server is the RPC server that is servicing the client.
	rpcServer *Server

	// conn is the underlying websocket connection.
	conn *websocket.Conn

	// addr is the remote address of the client.
	addr string

	// authenticated specifies whether a client has been authenticated
	// and therefore is allowed to communicated over the websocket.
	authenticated bool

	// isAdmin specifies whether a client may change the state of the server;
	// false means its access is only to the limited set of RPC calls.
	isAdmin bool

	// sessionID is a random ID generated for each client when connected.
	// These IDs may be queried by a client using the session RPC.  A change
	// to the session ID indicates that the client reconnected.
	sessionID uint64

	// verboseTxUpdates specifies whether a client has requested verbose
	// information about all new transactions.
	verboseTxUpdates bool

	filterData *wsClientFilter

	// Networking infrastructure.
	serviceRequestSem semaphore
	ntfnChan          chan []byte
	sendChan          chan wsResponse
	quit              chan struct{}
	wg                sync.WaitGroup
}

// shouldLogReadError returns whether or not the passed error, which is expected
// to have come from reading from the websocket client in the inHandler, should
// be logged.
func (c *wsClient) shouldLogReadError(err error) bool {
	// No logging when the client is being forcibly disconnected from the server
	// side.
	if c.disconnected.Load() {
		return false
	}

	// No logging when the remote client has disconnected.
	if errors.Is(err, io.EOF) || websocket.IsCloseError(err,
		websocket.CloseNormalClosure, websocket.CloseAbnormalClosure) {

		return false
	}

	return true
}

// inHandler handles all incoming messages for the websocket connection.  It
// must be run as a goroutine.
func (c *wsClient) inHandler(ctx context.Context) {
out:
	for !c.disconnected.Load() {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			// Log the error if it's not due to disconnecting.
			if c.shouldLogReadError(err) {
				log.Errorf("Websocket receive error from %s: %v", c.addr, err)
			}
			break out
		}

		var batchedRequest bool

		// Determine request type
		if bytes.HasPrefix(msg, batchedRequestPrefix) {
			batchedRequest = true
		}

		// Process a single request
		if !batchedRequest {
			var req dcrjson.Request
			var reply json.RawMessage
			err = json.Unmarshal(msg, &req)
			if err != nil {
				// only process requests from authenticated clients
				if !c.authenticated {
					break out
				}

				jsonErr := &dcrjson.RPCError{
					Code:    dcrjson.ErrRPCParse.Code,
					Message: "Failed to parse request: " + err.Error(),
				}
				reply, err = createMarshalledReply("1.0", nil, nil, jsonErr)
				if err != nil {
					log.Errorf("Failed to marshal reply: %v", err)
					continue
				}
				c.SendMessage(reply, nil)
				continue
			}

			if req.Method == "" {
				jsonErr := &dcrjson.RPCError{
					Code:    dcrjson.ErrRPCInvalidRequest.Code,
					Message: "Invalid request: malformed",
				}
				reply, err := createMarshalledReply(req.Jsonrpc, req.ID, nil, jsonErr)
				if err != nil {
					log.Errorf("Failed to marshal reply: %v", err)
					continue
				}
				c.SendMessage(reply, nil)
				continue
			}

			// Valid requests with no ID (notifications) must not have a response
			// per the JSON-RPC spec.
			if req.ID == nil {
				if !c.authenticated {
					break out
				}
				continue
			}

			cmd := parseCmd(&req)
			if cmd.err != nil {
				// Only process requests from authenticated clients
				if !c.authenticated {
					break out
				}

				reply, err = createMarshalledReply(cmd.jsonrpc, cmd.id, nil, cmd.err)
				if err != nil {
					log.Errorf("Failed to marshal reply: %v", err)
					continue
				}
				c.SendMessage(reply, nil)
				continue
			}

			log.Debugf("Received command <%s> from %s", cmd.method, c.addr)

			// Check auth.  The client is immediately disconnected if the
			// first request of an unauthenticated websocket client is not
			// the authenticate request, an authenticate request is received
			// when the client is already authenticated, or incorrect
			// authentication credentials are provided in the request.
			switch authCmd, ok := cmd.params.(*types.AuthenticateCmd); {
			case c.authenticated && ok:
				log.Warnf("Websocket client %s is already authenticated",
					c.addr)
				break out
			case !c.authenticated && !ok:
				log.Warnf("Unauthenticated websocket message " +
					"received")
				break out
			case !c.authenticated:
				// Check credentials.
				c.authenticated, c.isAdmin = c.rpcServer.checkAuthUserPass(
					authCmd.Username, authCmd.Passphrase, c.addr)
				if !c.authenticated {
					break out
				}

				// Increase the read limits for authenticated connections.
				c.conn.SetReadLimit(websocketReadLimitAuthenticated)

				// Marshal and send response.
				reply, err = createMarshalledReply(cmd.jsonrpc, cmd.id, nil, nil)
				if err != nil {
					log.Errorf("Failed to marshal authenticate reply: "+
						"%v", err.Error())
					continue
				}
				c.SendMessage(reply, nil)
				continue
			}

			// Check if the client is using limited RPC credentials and
			// error when not authorized to call the supplied RPC.
			if !c.isAdmin {
				if _, ok := rpcLimited[req.Method]; !ok {
					jsonErr := &dcrjson.RPCError{
						Code:    dcrjson.ErrRPCInvalidParams.Code,
						Message: "limited user not authorized for this method",
					}
					// Marshal and send response.
					reply, err = createMarshalledReply("", req.ID, nil, jsonErr)
					if err != nil {
						log.Errorf("Failed to marshal parse failure "+
							"reply: %v", err)
						continue
					}
					c.SendMessage(reply, nil)
					continue
				}
			}

			// Asynchronously handle the request.  A semaphore is used to
			// limit the number of concurrent requests currently being
			// serviced.  If the semaphore can not be acquired, simply wait
			// until a request finished before reading the next RPC request
			// from the websocket client.
			//
			// This could be a little fancier by timing out and erroring
			// when it takes too long to service the request, but if that is
			// done, the read of the next request should not be blocked by
			// this semaphore, otherwise the next request will be read and
			// will probably sit here for another few seconds before timing
			// out as well.  This will cause the total timeout duration for
			// later requests to be much longer than the check here would
			// imply.
			//
			// If a timeout is added, the semaphore acquiring should be
			// moved inside of the new goroutine with a select statement
			// that also reads a time.After channel.  This will unblock the
			// read of the next request from the websocket client and allow
			// many requests to be waited on concurrently.
			c.serviceRequestSem.acquire()
			go func() {
				c.serviceRequest(ctx, cmd)
				c.serviceRequestSem.release()
			}()
		}

		// Process a batched request
		if batchedRequest {
			var batchedRequests []json.RawMessage
			var results []json.RawMessage
			var batchSize int
			var reply json.RawMessage
			c.serviceRequestSem.acquire()
			err = json.Unmarshal(msg, &batchedRequests)
			if err != nil {
				// Only process requests from authenticated clients
				if !c.authenticated {
					break out
				}

				jsonErr := &dcrjson.RPCError{
					Code: dcrjson.ErrRPCParse.Code,
					Message: fmt.Sprintf("Failed to parse request: %v",
						err),
				}
				reply, err = dcrjson.MarshalResponse("2.0", nil, nil, jsonErr)
				if err != nil {
					log.Errorf("Failed to create reply: %v", err)
				}

				if reply != nil {
					results = append(results, reply)
				}
			}

			if err == nil {
				// Response with an empty batch error if the batch size is zero
				if len(batchedRequests) == 0 {
					if !c.authenticated {
						break out
					}

					jsonErr := &dcrjson.RPCError{
						Code:    dcrjson.ErrRPCInvalidRequest.Code,
						Message: "Invalid request: empty batch",
					}
					reply, err = dcrjson.MarshalResponse("2.0", nil, nil, jsonErr)
					if err != nil {
						log.Errorf("Failed to marshal reply: %v", err)
					}

					if reply != nil {
						results = append(results, reply)
					}
				}

				// Process each batch entry individually
				if len(batchedRequests) > 0 {
					batchSize = len(batchedRequests)
					for _, entry := range batchedRequests {
						var req dcrjson.Request
						err := json.Unmarshal(entry, &req)
						if err != nil {
							// Only process requests from authenticated clients
							if !c.authenticated {
								break out
							}

							jsonErr := &dcrjson.RPCError{
								Code: dcrjson.ErrRPCInvalidRequest.Code,
								Message: fmt.Sprintf("Invalid request: %v",
									err),
							}
							reply, err = dcrjson.MarshalResponse("2.0", nil, nil, jsonErr)
							if err != nil {
								log.Errorf("Failed to create reply: %v", err)
								continue
							}

							if reply != nil {
								results = append(results, reply)
							}
							continue
						}

						if req.Method == "" || req.Params == nil {
							jsonErr := &dcrjson.RPCError{
								Code:    dcrjson.ErrRPCInvalidRequest.Code,
								Message: "Invalid request: malformed",
							}
							reply, err := createMarshalledReply(req.Jsonrpc, req.ID, nil, jsonErr)
							if err != nil {
								log.Errorf("Failed to marshal reply: %v", err)
								continue
							}

							if reply != nil {
								results = append(results, reply)
							}
							continue
						}

						// Valid requests with no ID (notifications) must not have a response
						// per the JSON-RPC spec.
						if req.ID == nil {
							if !c.authenticated {
								break out
							}
							continue
						}

						cmd := parseCmd(&req)
						if cmd.err != nil {
							// Only process requests from authenticated clients
							if !c.authenticated {
								break out
							}

							reply, err = createMarshalledReply(cmd.jsonrpc, cmd.id, nil, cmd.err)
							if err != nil {
								log.Errorf("Failed to marshal reply: %v", err)
								continue
							}

							if reply != nil {
								results = append(results, reply)
							}
							continue
						}

						log.Debugf("Received command <%s> from %s", cmd.method, c.addr)

						// Check auth.  The client is immediately disconnected if the
						// first request of an unauthenticated websocket client is not
						// the authenticate request, an authenticate request is received
						// when the client is already authenticated, or incorrect
						// authentication credentials are provided in the request.
						switch authCmd, ok := cmd.params.(*types.AuthenticateCmd); {
						case c.authenticated && ok:
							log.Warnf("Websocket client %s is already authenticated",
								c.addr)
							break out
						case !c.authenticated && !ok:
							log.Warnf("Unauthenticated websocket message " +
								"received")
							break out
						case !c.authenticated:
							// Check credentials.
							c.authenticated, c.isAdmin = c.rpcServer.checkAuthUserPass(
								authCmd.Username, authCmd.Passphrase, c.addr)
							if !c.authenticated {
								break out
							}

							// Marshal and send response.
							reply, err = createMarshalledReply(cmd.jsonrpc, cmd.id, nil, nil)
							if err != nil {
								log.Errorf("Failed to marshal authenticate reply: "+
									"%v", err.Error())
								continue
							}

							if reply != nil {
								results = append(results, reply)
							}
							continue
						}

						// Check if the client is using limited RPC credentials and
						// error when not authorized to call the supplied RPC.
						if !c.isAdmin {
							if _, ok := rpcLimited[req.Method]; !ok {
								jsonErr := &dcrjson.RPCError{
									Code:    dcrjson.ErrRPCInvalidParams.Code,
									Message: "limited user not authorized for this method",
								}
								// Marshal and send response.
								reply, err = createMarshalledReply(req.Jsonrpc, req.ID, nil, jsonErr)
								if err != nil {
									log.Errorf("Failed to marshal parse failure "+
										"reply: %v", err)
									continue
								}

								if reply != nil {
									results = append(results, reply)
								}
								continue
							}
						}

						// Lookup the websocket extension for the command, if it doesn't
						// exist fallback to handling the command as a standard command.
						var resp interface{}
						wsHandler, ok := wsHandlers[cmd.method]
						if ok {
							resp, err = wsHandler(ctx, c, cmd.params)
						} else {
							resp, err = c.rpcServer.standardCmdResult(ctx,
								cmd)
						}

						// Marshal request output.
						reply, err := createMarshalledReply(cmd.jsonrpc, cmd.id, resp, err)
						if err != nil {
							log.Errorf("Failed to marshal reply for <%s> "+
								"command: %v", cmd.method, err)
							return
						}

						if reply != nil {
							results = append(results, reply)
						}
					}
				}
			}

			// generate reply
			var payload = []byte{}
			if batchedRequest && batchSize > 0 {
				if len(results) > 0 {
					// Form the batched response json
					var buffer bytes.Buffer
					buffer.WriteByte('[')
					for idx, marshalledReply := range results {
						if idx == len(results)-1 {
							buffer.Write(marshalledReply)
							buffer.WriteByte(']')
							break
						}
						buffer.Write(marshalledReply)
						buffer.WriteByte(',')
					}
					payload = buffer.Bytes()
				}
			}

			if !batchedRequest || batchSize == 0 {
				// Respond with the first results entry for single requests
				if len(results) > 0 {
					payload = results[0]
				}
			}

			c.SendMessage(payload, nil)
			c.serviceRequestSem.release()
		}
	}

	// Ensure the connection is closed.
	c.Disconnect()
	c.wg.Done()
	log.Tracef("Websocket client input handler done for %s", c.addr)
}

// serviceRequest services a parsed RPC request by looking up and executing the
// appropriate RPC handler.  The response is marshalled and sent to the websocket
// client.
func (c *wsClient) serviceRequest(ctx context.Context, r *parsedRPCCmd) {
	var (
		result interface{}
		err    error
	)

	// Lookup the websocket extension for the command and if it doesn't
	// exist fallback to handling the command as a standard command.
	wsHandler, ok := wsHandlers[r.method]
	if ok {
		result, err = wsHandler(ctx, c, r.params)
	} else {
		result, err = c.rpcServer.standardCmdResult(ctx, r)
	}
	reply, err := createMarshalledReply(r.jsonrpc, r.id, result, err)
	if err != nil {
		log.Errorf("Failed to marshal reply for <%s> "+
			"command: %v", r.method, err)
		return
	}

	c.SendMessage(reply, nil)
}

// notificationQueueHandler handles the queuing of outgoing notifications for
// the websocket client.  This runs as a muxer for various sources of input to
// ensure that queuing up notifications to be sent will not block.  Otherwise,
// slow clients could bog down the other systems (such as the mempool or block
// manager) which are queuing the data.  The data is passed on to outHandler to
// actually be written.  It must be run as a goroutine.
func (c *wsClient) notificationQueueHandler() {
	ntfnSentChan := make(chan bool, 1) // nonblocking sync

	// pendingNtfns is used as a queue for notifications that are ready to
	// be sent once there are no outstanding notifications currently being
	// sent.  The waiting flag is used over simply checking for items in the
	// pending list to ensure cleanup knows what has and hasn't been sent
	// to the outHandler.  Currently no special cleanup is needed, however
	// if something like a done channel is added to notifications in the
	// future, not knowing what has and hasn't been sent to the outHandler
	// (and thus who should respond to the done channel) would be
	// problematic without using this approach.
	var pendingNtfns [][]byte
	waiting := false
out:
	for {
		select {
		// This channel is notified when a message is being queued to
		// be sent across the network socket.  It will either send the
		// message immediately if a send is not already in progress, or
		// queue the message to be sent once the other pending messages
		// are sent.
		case msg := <-c.ntfnChan:
			if !waiting {
				c.SendMessage(msg, ntfnSentChan)
			} else {
				pendingNtfns = append(pendingNtfns, msg)
			}
			waiting = true

		// This channel is notified when a notification has been sent
		// across the network socket.
		case <-ntfnSentChan:
			// No longer waiting if there are no more messages in
			// the pending messages queue.
			if len(pendingNtfns) == 0 {
				waiting = false
				continue
			}
			// Notify the outHandler about the next item to
			// asynchronously send.
			msg := pendingNtfns[0]
			pendingNtfns[0] = nil
			pendingNtfns = pendingNtfns[1:]
			c.SendMessage(msg, ntfnSentChan)

		case <-c.quit:
			break out
		}
	}

	c.wg.Done()
	log.Tracef("Websocket client notification queue handler done "+
		"for %s", c.addr)
}

// outHandler handles all outgoing messages for the websocket connection.  It
// must be run as a goroutine.  It uses a buffered channel to serialize output
// messages while allowing the sender to continue running asynchronously.  It
// must be run as a goroutine.
func (c *wsClient) outHandler() {
out:
	for {
		// Send any messages ready for send until the context is done.
		select {
		case r := <-c.sendChan:
			err := c.conn.WriteMessage(websocket.TextMessage, r.msg)
			if err != nil {
				c.Disconnect()
				break out
			}
			if r.doneChan != nil {
				r.doneChan <- true
			}

		case <-c.quit:
			break out
		}
	}

	c.wg.Done()
	log.Tracef("Websocket client output handler done for %s", c.addr)
}

// SendMessage sends the passed json to the websocket client.  It is backed
// by a buffered channel, so it will not block until the send channel is full.
// Note however that QueueNotification must be used for sending async
// notifications instead of the this function.  This approach allows a limit to
// the number of outstanding requests a client can make without preventing or
// blocking on async notifications.
func (c *wsClient) SendMessage(marshalledJSON []byte, doneChan chan bool) {
	// Don't send the message if disconnected.
	if c.Disconnected() {
		if doneChan != nil {
			doneChan <- false
		}
		return
	}

	// Use select statement to unblock enqueuing the message once the client has
	// begun shutting down.
	select {
	case c.sendChan <- wsResponse{msg: marshalledJSON, doneChan: doneChan}:
	case <-c.quit:
		if doneChan != nil {
			doneChan <- false
		}
	}
}

// ErrClientQuit describes the error where a client send is not processed due
// to the client having already been disconnected or dropped.
var ErrClientQuit = errors.New("client quit")

// QueueNotification queues the passed notification to be sent to the websocket
// client.  This function, as the name implies, is only intended for
// notifications since it has additional logic to prevent other subsystems, such
// as the memory pool and sync manager, from blocking even when the send channel
// is full.
//
// If the client is in the process of shutting down, this function returns
// ErrClientQuit.  This is intended to be checked by long-running notification
// handlers to stop processing if there is no more work needed to be done.
func (c *wsClient) QueueNotification(marshalledJSON []byte) error {
	// Don't queue the message if disconnected.
	if c.Disconnected() {
		return ErrClientQuit
	}

	// Use select statement to unblock enqueuing the message once the client has
	// begun shutting down.
	select {
	case c.ntfnChan <- marshalledJSON:
	case <-c.quit:
		return ErrClientQuit
	}

	return nil
}

// Disconnected returns whether or not the websocket client is disconnected.
func (c *wsClient) Disconnected() bool {
	return c.disconnected.Load()
}

// Disconnect disconnects the websocket client.
func (c *wsClient) Disconnect() {
	// Nothing to do if already disconnected.
	if !c.disconnected.CompareAndSwap(false, true) {
		return
	}

	log.Tracef("Disconnecting websocket client %s", c.addr)
	close(c.quit)
	c.conn.Close()
}

// Run starts the websocket client and all other goroutines necessary for it to
// function properly and blocks until the provided context is cancelled.
func (c *wsClient) Run(ctx context.Context) {
	log.Tracef("Starting websocket client %s", c.addr)

	// Start processing input and output.
	c.wg.Add(3)
	go c.inHandler(ctx)
	go c.notificationQueueHandler()
	go c.outHandler()

	// Forcibly disconnect the websocket client when the context is cancelled
	// which also closes the quit channel and thus ensures all of the above
	// goroutines are shutdown.
	c.wg.Add(1)
	go func(ctx context.Context) {
		// Select across the quit channel as well since the context is not
		// cancelled when the connection is closed due to websocket connection
		// hijacking.
		select {
		case <-ctx.Done():
			c.Disconnect()
		case <-c.quit:
		}
		c.wg.Done()
	}(ctx)

	c.wg.Wait()
}

// newWebsocketClient returns a new websocket client given the notification
// manager, websocket connection, remote address, and whether or not the client
// has already been authenticated (via HTTP Basic access authentication).  The
// returned client is ready to start.  Once started, the client will process
// incoming and outgoing messages in separate goroutines complete with queuing
// and asynchronous handling for long-running operations.
func newWebsocketClient(server *Server, conn *websocket.Conn,
	remoteAddr string, authenticated bool, isAdmin bool) (*wsClient, error) {

	sessionID, err := wire.RandomUint64()
	if err != nil {
		return nil, err
	}

	client := &wsClient{
		conn:              conn,
		addr:              remoteAddr,
		authenticated:     authenticated,
		isAdmin:           isAdmin,
		sessionID:         sessionID,
		rpcServer:         server,
		serviceRequestSem: makeSemaphore(server.cfg.RPCMaxConcurrentReqs),
		ntfnChan:          make(chan []byte, 1), // nonblocking sync
		sendChan:          make(chan wsResponse, websocketSendBufferSize),
		quit:              make(chan struct{}),
	}
	return client, nil
}

// handleWebsocketHelp implements the help command for websocket connections.
func handleWebsocketHelp(_ context.Context, wsc *wsClient, icmd interface{}) (interface{}, error) {
	cmd, ok := icmd.(*types.HelpCmd)
	if !ok {
		return nil, dcrjson.ErrRPCInternal
	}

	// Provide a usage overview of all commands when no specific command
	// was specified.
	var method types.Method
	if cmd.Command != nil {
		method = types.Method(*cmd.Command)
	}
	if method == "" {
		usage, err := wsc.rpcServer.helpCacher.RPCUsage(true)
		if err != nil {
			context := "Failed to generate RPC usage"
			return nil, rpcInternalError(err.Error(), context)
		}
		return usage, nil
	}

	// Check that the command asked for is supported and implemented.
	// Search the list of websocket handlers as well as the main list of
	// handlers since help should only be provided for those cases.
	_, valid := rpcHandlers[method]
	if !valid {
		_, valid = wsHandlers[method]
	}
	if !valid {
		return nil, &dcrjson.RPCError{
			Code:    dcrjson.ErrRPCInvalidParameter,
			Message: "Unknown method: " + string(method),
		}
	}

	// Get the help for the command.
	help, err := wsc.rpcServer.helpCacher.RPCMethodHelp(method)
	if err != nil {
		context := "Failed to generate help"
		return nil, rpcInternalError(err.Error(), context)
	}
	return help, nil
}

// handleLoadTxFilter implements the loadtxfilter command extension for
// websocket connections.
func handleLoadTxFilter(_ context.Context, wsc *wsClient, icmd interface{}) (interface{}, error) {
	cmd := icmd.(*types.LoadTxFilterCmd)

	outPoints := make([]*wire.OutPoint, len(cmd.OutPoints))
	for i := range cmd.OutPoints {
		hash, err := chainhash.NewHashFromStr(cmd.OutPoints[i].Hash)
		if err != nil {
			return nil, &dcrjson.RPCError{
				Code:    dcrjson.ErrRPCInvalidParameter,
				Message: err.Error(),
			}
		}
		outPoints[i] = &wire.OutPoint{
			Hash:  *hash,
			Index: cmd.OutPoints[i].Index,
			Tree:  cmd.OutPoints[i].Tree,
		}
	}

	wsc.Lock()
	if cmd.Reload || wsc.filterData == nil {
		wsc.filterData = makeWSClientFilter(cmd.Addresses, outPoints,
			wsc.rpcServer.cfg.ChainParams)
		wsc.Unlock()
	} else {
		filter := wsc.filterData
		wsc.Unlock()

		filter.mu.Lock()
		for _, a := range cmd.Addresses {
			filter.addAddressStr(a)
		}
		for _, op := range outPoints {
			filter.addUnspentOutPoint(op)
		}
		filter.mu.Unlock()
	}

	return nil, nil
}

// handleNotifyBlocks implements the notifyblocks command extension for
// websocket connections.
func handleNotifyBlocks(_ context.Context, wsc *wsClient, _ interface{}) (interface{}, error) {
	wsc.rpcServer.ntfnMgr.RegisterBlockUpdates(wsc)
	return nil, nil
}

// handleRebroadcastWinners implements the rebroadcastwinners command.
func handleRebroadcastWinners(_ context.Context, wsc *wsClient, _ interface{}) (interface{}, error) {
	cfg := wsc.rpcServer.cfg
	bestHeight := cfg.Chain.BestSnapshot().Height
	blocks, err := cfg.Chain.TipGeneration()
	if err != nil {
		return nil, rpcInternalError("Could not get generation "+
			err.Error(), "")
	}

	for i := range blocks {
		winningTickets, _, _, err := cfg.Chain.LotteryDataForBlock(&blocks[i])
		if err != nil {
			// This can legitimately happen if we have the block
			// header but not the block data, so just log a warning
			// and keep sending notifications.
			log.Warnf("Lottery data for block failed: %v", err)
			continue
		}
		ntfnData := &WinningTicketsNtfnData{
			BlockHash:   blocks[i],
			BlockHeight: bestHeight,
			Tickets:     winningTickets,
		}

		wsc.rpcServer.ntfnMgr.NotifyWinningTickets(ntfnData)
	}

	return nil, nil
}

// handleNotifyWork implements the notifywork command extension for
// websocket connections.
func handleNotifyWork(_ context.Context, wsc *wsClient, _ interface{}) (interface{}, error) {
	wsc.rpcServer.ntfnMgr.RegisterWorkUpdates(wsc)
	return nil, nil
}

// handleNotifyTSpend implements the notifytspend command extension for
// websocket connections.
func handleNotifyTSpend(_ context.Context, wsc *wsClient, _ interface{}) (interface{}, error) {
	wsc.rpcServer.ntfnMgr.RegisterTSpendUpdates(wsc)
	return nil, nil
}

// handleSession implements the session command extension for websocket
// connections.
func handleSession(_ context.Context, wsc *wsClient, icmd interface{}) (interface{}, error) {
	return &types.SessionResult{SessionID: wsc.sessionID}, nil
}

// handleWinningTickets implements the notifywinningtickets command
// extension for websocket connections.
func handleWinningTickets(_ context.Context, wsc *wsClient, _ interface{}) (interface{}, error) {
	wsc.rpcServer.ntfnMgr.RegisterWinningTickets(wsc)
	return nil, nil
}

// handleNewTickets implements the notifynewtickets command extension for
// websocket connections.
func handleNewTickets(_ context.Context, wsc *wsClient, _ interface{}) (interface{}, error) {
	wsc.rpcServer.ntfnMgr.RegisterNewTickets(wsc)
	return nil, nil
}

// handleStopNotifyBlocks implements the stopnotifyblocks command extension for
// websocket connections.
func handleStopNotifyBlocks(_ context.Context, wsc *wsClient, _ interface{}) (interface{}, error) {
	wsc.rpcServer.ntfnMgr.UnregisterBlockUpdates(wsc)
	return nil, nil
}

// handleStopNotifyWork implements the stopnotifywork command extension for
// websocket connections.
func handleStopNotifyWork(_ context.Context, wsc *wsClient, _ interface{}) (interface{}, error) {
	wsc.rpcServer.ntfnMgr.UnregisterWorkUpdates(wsc)
	return nil, nil
}

// handleStopNotifyTSpend implements the stopnotifytspend command extension for
// websocket connections.
func handleStopNotifyTSpend(_ context.Context, wsc *wsClient, _ interface{}) (interface{}, error) {
	wsc.rpcServer.ntfnMgr.UnregisterTSpendUpdates(wsc)
	return nil, nil
}

// handleNotifyNewTransations implements the notifynewtransactions command
// extension for websocket connections.
func handleNotifyNewTransactions(_ context.Context, wsc *wsClient, icmd interface{}) (interface{}, error) {
	cmd, ok := icmd.(*types.NotifyNewTransactionsCmd)
	if !ok {
		return nil, dcrjson.ErrRPCInternal
	}

	wsc.verboseTxUpdates = cmd.Verbose != nil && *cmd.Verbose
	wsc.rpcServer.ntfnMgr.RegisterNewMempoolTxsUpdates(wsc)
	return nil, nil
}

// handleStopNotifyNewTransations implements the stopnotifynewtransactions
// command extension for websocket connections.
func handleStopNotifyNewTransactions(_ context.Context, wsc *wsClient, _ interface{}) (interface{}, error) {
	wsc.rpcServer.ntfnMgr.UnregisterNewMempoolTxsUpdates(wsc)
	return nil, nil
}

// rescanBlock rescans a block for any relevant transactions for the passed
// lookup keys.  Any discovered transactions are returned hex encoded as a
// string slice.
func rescanBlock(filter *wsClientFilter, block *dcrutil.Block, params *chaincfg.Params, isTreasuryEnabled bool) []string {
	var transactions []string

	// Need to iterate over both the stake and regular transactions in a
	// block, but these are two different slices in the MsgTx.  To avoid
	// another allocation to create a single slice to range over, the loop
	// body logic is run from a closure.
	//
	// This makes unsynchronized calls to the filter and thus must only be
	// called with the filter mutex held.
	checkTransaction := func(tx *wire.MsgTx, tree int8) {
		// Keep track of whether the transaction has already been added
		// to the result.  It shouldn't be added twice.
		added := false

		inputs := tx.TxIn
		if tree == wire.TxTreeRegular {
			// Skip previous output checks for coinbase inputs.  These do
			// not reference a previous output.
			if standalone.IsCoinBaseTx(tx, isTreasuryEnabled) {
				goto LoopOutputs
			}
		} else {
			if stake.IsSSGen(tx) {
				// Skip the first stakebase input.  These do not
				// reference a previous output.
				inputs = inputs[1:]
			}
		}
		for _, input := range inputs {
			if !filter.existsUnspentOutPoint(&input.PreviousOutPoint) {
				continue
			}
			if !added {
				transactions = append(transactions, txHexString(tx))
				added = true
			}
		}

	LoopOutputs:
		for i, output := range tx.TxOut {
			scriptType, addrs := stdscript.ExtractAddrs(output.Version,
				output.PkScript, params)
			if scriptType == stdscript.STNonStandard {
				continue
			}
			for _, a := range addrs {
				if !filter.existsAddress(a) {
					continue
				}

				op := wire.OutPoint{
					Hash:  tx.TxHash(),
					Index: uint32(i),
					Tree:  tree,
				}
				filter.addUnspentOutPoint(&op)

				if !added {
					transactions = append(transactions, txHexString(tx))
					added = true
				}
			}
		}
	}

	msgBlock := block.MsgBlock()
	filter.mu.Lock()
	for _, tx := range msgBlock.STransactions {
		checkTransaction(tx, wire.TxTreeStake)
	}
	for _, tx := range msgBlock.Transactions {
		checkTransaction(tx, wire.TxTreeRegular)
	}
	filter.mu.Unlock()

	return transactions
}

// handleRescan implements the rescan command extension for websocket
// connections.
func handleRescan(_ context.Context, wsc *wsClient, icmd interface{}) (interface{}, error) {
	cmd, ok := icmd.(*types.RescanCmd)
	if !ok {
		return nil, dcrjson.ErrRPCInternal
	}

	// Load client's transaction filter.  Must exist in order to continue.
	wsc.Lock()
	filter := wsc.filterData
	wsc.Unlock()
	if filter == nil {
		return nil, &dcrjson.RPCError{
			Code:    dcrjson.ErrRPCMisc,
			Message: "Transaction filter must be loaded before rescanning",
		}
	}

	blockHashes, err := decodeHashes(cmd.BlockHashes)
	if err != nil {
		return nil, err
	}

	discoveredData := make([]types.RescannedBlock, 0, len(blockHashes))

	// Iterate over each block in the request and rescan.  When a block
	// contains relevant transactions, add it to the response.
	rpcServer := wsc.rpcServer
	cfg := rpcServer.cfg
	bc := cfg.Chain
	var lastBlockHash *chainhash.Hash
	for i := range blockHashes {
		block, err := bc.BlockByHash(&blockHashes[i])
		if err != nil {
			return nil, &dcrjson.RPCError{
				Code:    dcrjson.ErrRPCBlockNotFound,
				Message: "Failed to fetch block: " + err.Error(),
			}
		}
		prevBlkHash := block.MsgBlock().Header.PrevBlock
		if lastBlockHash != nil && prevBlkHash != *lastBlockHash {
			return nil, &dcrjson.RPCError{
				Code: dcrjson.ErrRPCInvalidParameter,
				Message: fmt.Sprintf("Block %v is not a child of %v",
					&blockHashes[i], lastBlockHash),
			}
		}
		lastBlockHash = &blockHashes[i]

		// Determine if the treasury rules are active as of the block.
		isTreasuryEnabled, err := rpcServer.isTreasuryAgendaActive(&prevBlkHash)
		if err != nil {
			return nil, err
		}

		transactions := rescanBlock(filter, block, cfg.ChainParams,
			isTreasuryEnabled)
		if len(transactions) != 0 {
			discoveredData = append(discoveredData, types.RescannedBlock{
				Hash:         blockHashes[i].String(),
				Transactions: transactions,
			})
		}
	}

	return &types.RescanResult{DiscoveredData: discoveredData}, nil
}

func init() {
	wsHandlers = wsHandlersBeforeInit
}
