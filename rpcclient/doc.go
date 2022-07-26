// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2016-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package rpcclient implements a websocket-enabled Decred JSON-RPC client.

This client provides a robust and easy to use client for interfacing
with a Decred RPC server that uses a mostly btcd/bitcoin core
style Decred JSON-RPC API.  This client has been tested with dcrd
(https://github.com/decred/dcrd) and dcrwallet
(https://github.com/decred/dcrwallet).

In addition to the compatible standard HTTP POST JSON-RPC API, dcrd and
dcrwallet provide a websocket interface that is more efficient than the standard
HTTP POST method of accessing RPC.  The section below discusses the differences
between HTTP POST and websockets.

By default, this client assumes the RPC server supports websockets and has
TLS enabled.  In practice, this currently means it assumes you are talking to
dcrd or dcrwallet by default.  However, configuration options are provided to
fall back to HTTP POST and disable TLS to support talking with inferior bitcoin
core style RPC servers.

# Websockets vs HTTP POST

In HTTP POST-based JSON-RPC, every request creates a new HTTP connection,
issues the call, waits for the response, and closes the connection.  This adds
quite a bit of overhead to every call and lacks flexibility for features such as
notifications.

In contrast, the websocket-based JSON-RPC interface provided by dcrd and
dcrwallet only uses a single connection that remains open and allows
asynchronous bi-directional communication.

The websocket interface supports all of the same commands as HTTP POST, but they
can be invoked without having to go through a connect/disconnect cycle for every
call.  In addition, the websocket interface provides other nice features such as
the ability to register for asynchronous notifications of various events.

# Synchronous vs Asynchronous API

The client provides both a synchronous (blocking) and asynchronous API.

The synchronous (blocking) API is typically sufficient for most use cases.  It
works by issuing the RPC and blocking until the response is received.  This
allows  straightforward code where you have the response as soon as the function
returns.

The asynchronous API works on the concept of futures.  When you invoke the async
version of a command, it will quickly return an instance of a type that promises
to provide the result of the RPC at some future time.  In the background, the
RPC call is issued and the result is stored in the returned instance.  Invoking
the Receive method on the returned instance will either return the result
immediately if it has already arrived, or block until it has.  This is useful
since it provides the caller with greater control over concurrency.

# Notifications

The first important part of notifications is to realize that they will only
work when connected via websockets.  This should intuitively make sense
because HTTP POST mode does not keep a connection open!

All notifications provided by dcrd require registration to opt-in.  For example,
if you want to be notified when funds are received by a set of addresses, you
register the addresses via the NotifyReceived (or NotifyReceivedAsync) function.

# Notification Handlers

Notifications are exposed by the client through the use of callback handlers
which are setup via a NotificationHandlers instance that is specified by the
caller when creating the client.

It is important that these notification handlers complete quickly since they
are intentionally in the main read loop and will block further reads until
they complete.  This provides the caller with the flexibility to decide what to
do when notifications are coming in faster than they are being handled.

In particular this means issuing a blocking RPC call from a callback handler
will cause a deadlock as more server responses won't be read until the callback
returns, but the callback would be waiting for a response.   Thus, any
additional RPCs must be issued an a completely decoupled manner.

# Automatic Reconnection

By default, when running in websockets mode, this client will automatically
keep trying to reconnect to the RPC server should the connection be lost.  There
is a back-off in between each connection attempt until it reaches one try per
minute.  Once a connection is re-established, all previously registered
notifications are automatically re-registered and any in-flight commands are
re-issued.  This means from the caller's perspective, the request simply takes
longer to complete.

The caller may invoke the Shutdown method on the client to force the client
to cease reconnect attempts and return ErrClientShutdown for all outstanding
commands.

The automatic reconnection can be disabled by setting the DisableAutoReconnect
flag to true in the connection config when creating the client.

# Interacting with Dcrwallet

This package only provides methods for dcrd RPCs.  Using the websocket
connection and request-response mapping provided by rpcclient with arbitrary
methods or different servers is possible through the generic RawRequest and
RawRequestAsync methods (each of which deal with json.RawMessage for parameters
and return results).

Previous versions of this package provided methods for dcrwallet's JSON-RPC
server in addition to dcrd.  These were removed in major version 6 of this
module.  Projects depending on these calls are advised to use the
decred.org/dcrwallet/rpc/client/dcrwallet package which is able to wrap
rpcclient.Client using the aforementioned RawRequest method:

	var _ *rpcclient.Client = client // Should be connected to dcrwallet
	var _ *chaincfg.Params = params
	var walletClient = dcrwallet.NewClient(dcrwallet.RawRequestCaller(client), params)

Using struct embedding, it is possible to create a single variable with the
combined method sets of both rpcclient.Client and dcrwallet.Client:

	type WalletClient = dcrwallet.Client // Avoids naming clash for selectors
	type MyClient struct {
		*rpcclient.Client
		*WalletClient
	}
	var myClient = MyClient{Client: client, WalletClient: walletClient}

This technique is valuable as dcrwallet (syncing in RPC mode) will passthrough
any unknown RPCs to the backing dcrd server, proxying requests and responses for
the client.

# Errors

There are 3 categories of errors that will be returned throughout this package:

  - Errors related to the client connection such as authentication, endpoint,
    disconnect, and shutdown
  - Errors that occur before communicating with the remote RPC server such as
    command creation and marshaling errors or issues talking to the remote
    server
  - Errors returned from the remote RPC server like unimplemented commands,
    nonexistent requested blocks and transactions, malformed data, and incorrect
    networks

The first category of errors are typically one of ErrInvalidAuth,
ErrInvalidEndpoint, ErrClientDisconnect, or ErrClientShutdown.

NOTE: The ErrClientDisconnect will not be returned unless the
DisableAutoReconnect flag is set since the client automatically handles
reconnect by default as previously described.

The second category of errors typically indicates a programmer error and as such
the type can vary, but usually will be best handled by simply showing/logging
it.

The third category of errors, that is errors returned by the server, can be
detected by type asserting the error in a *dcrjson.RPCError.  For example, to
detect if a command is unimplemented by the remote RPC server:

	block, err := client.GetBlock(ctx, blockHash)
	if err != nil {
		if jerr, ok := err.(*dcrjson.RPCError); ok {
			switch jerr.Code {
			case dcrjson.ErrRPCUnimplemented:
				// Handle not implemented error

			// Handle other specific errors you care about
			}
		}

		// Log or otherwise handle the error knowing it was not one returned
		// from the remote RPC server.
	}

# Example Usage

The following full-blown client examples are in the examples directory:

  - dcrdwebsockets
    Connects to a dcrd RPC server using TLS-secured websockets, registers for
    block connected and block disconnected notifications, and gets the current
    block count
*/
package rpcclient
