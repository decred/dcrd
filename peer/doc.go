// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package peer provides a common base for creating and managing Decred network
peers.

This package builds upon the wire package, which provides the fundamental
primitives necessary to speak the Decred wire protocol, in order to simplify
the process of creating fully functional peers.  In essence, it provides a
common base for creating concurrent safe fully validating nodes, Simplified
Payment Verification (SPV) nodes, proxies, etc.

A quick overview of the major features peer provides are as follows:

 - Provides a basic concurrent safe Decred peer for handling decred
   communications via the peer-to-peer protocol
 - Full duplex reading and writing of Decred protocol messages
 - Automatic handling of the initial handshake process including protocol
   version negotiation
 - Asynchronous message queuing of outbound messages with optional channel for
   notification when the message is actually sent
 - Flexible peer configuration
   - Caller is responsible for creating outgoing connections and listening for
     incoming connections so they have flexibility to establish connections as
     they see fit (proxies, etc)
   - User agent name and version
   - Decred network
   - Service support signalling (full nodes, etc)
   - Maximum supported protocol version
   - Ability to register callbacks for handling Decred protocol messages
 - Inventory message batching and send trickling with known inventory detection
   and avoidance
 - Automatic periodic keep-alive pinging and pong responses
 - Random nonce generation and self connection detection
 - Snapshottable peer statistics such as the total number of bytes read and
   written, the remote address, user agent, and negotiated protocol version
 - Helper functions pushing addresses, getblocks, getheaders, and reject
   messages
   - These could all be sent manually via the standard message output function,
     but the helpers provide additional nice functionality such as duplicate
     filtering and address randomization
 - Ability to wait for shutdown/disconnect
 - Comprehensive test coverage

Peer Configuration

All peer configuration is handled with the Config struct.  This allows the
caller to specify things such as the user agent name and version, the decred
network to use, which services it supports, and callbacks to invoke when decred
messages are received.  See the documentation for each field of the Config
struct for more details.

Inbound and Outbound Peers

A peer can either be inbound or outbound.  The caller is responsible for
establishing the connection to remote peers and listening for incoming peers.
This provides high flexibility for things such as connecting via proxies, acting
as a proxy, creating bridge peers, choosing whether to listen for inbound peers,
etc.

NewOutboundPeer and NewInboundPeer functions must be followed by calling Connect
with a net.Conn instance to the peer.  This will start all async I/O goroutines
and initiate the protocol negotiation process.  Once finished with the peer call
Disconnect to disconnect from the peer and clean up all resources.
WaitForDisconnect can be used to block until peer disconnection and resource
cleanup has completed.

Callbacks

In order to do anything useful with a peer, it is necessary to react to decred
messages.  This is accomplished by creating an instance of the MessageListeners
struct with the callbacks to be invoke specified and setting the Listeners field
of the Config struct specified when creating a peer to it.

For convenience, a callback hook for all of the currently supported decred
messages is exposed which receives the peer instance and the concrete message
type.  In addition, a hook for OnRead is provided so even custom messages types
for which this package does not directly provide a hook, as long as they
implement the wire.Message interface, can be used.  Finally, the OnWrite hook
is provided, which in conjunction with OnRead, can be used to track server-wide
byte counts.

It is often useful to use closures which encapsulate state when specifying the
callback handlers.  This provides a clean method for accessing that state when
callbacks are invoked.

Queuing Messages and Inventory

The QueueMessage function provides the fundamental means to send messages to the
remote peer.  As the name implies, this employs a non-blocking queue.  A done
channel which will be notified when the message is actually sent can optionally
be specified.  There are certain message types which are better sent using other
functions which provide additional functionality.

Of special interest are inventory messages.  Rather than manually sending MsgInv
messages via Queuemessage, the inventory vectors should be queued using the
QueueInventory function.  It employs batching and trickling along with
intelligent known remote peer inventory detection and avoidance through the use
of a most-recently used algorithm.

Message Sending Helper Functions

In addition to the bare QueueMessage function previously described, the
PushAddrMsg, PushAddrMsgV2, PushGetBlocksMsg, and PushGetHeadersMsg functions
are provided as a convenience.  While it is of course possible to create and
send these messages manually via QueueMessage, these helper functions provide
additional useful functionality that is typically desired.

For example, the PushAddrMsg and PushAddrMsgV2 functions automatically limit the
addresses to the maximum number allowed by the message and randomizes the chosen
addresses when there are too many.  This allows the caller to simply provide a
slice of known addresses, such as that returned by the addrmgr package, without
having to worry about the details.

Finally, the PushGetBlocksMsg and PushGetHeadersMsg functions will construct
proper messages using a block locator and ignore back to back duplicate
requests.

Peer Statistics

A snapshot of the current peer statistics can be obtained with the StatsSnapshot
function.  This includes statistics such as the total number of bytes read and
written, the remote address, user agent, and negotiated protocol version.

Logging

This package provides extensive logging capabilities through the UseLogger
function which allows a slog.Logger to be specified.  For example, logging at
the debug level provides summaries of every message sent and received, and
logging at the trace level provides full dumps of parsed messages as well as the
raw message bytes using a format similar to hexdump -C.

Improvement Proposals

This package supports all improvement proposals supported by the wire package.
*/
package peer
