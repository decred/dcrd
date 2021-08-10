// Copyright (c) 2014-2015 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"context"
	"encoding/json"

	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v3"
)

// AddNodeCommand enumerates the available commands that the AddNode function
// accepts.
type AddNodeCommand string

// Constants used to indicate the command for the AddNode function.
const (
	// ANAdd indicates the specified host should be added as a persistent
	// peer.
	ANAdd AddNodeCommand = "add"

	// ANRemove indicates the specified peer should be removed.
	ANRemove AddNodeCommand = "remove"

	// ANOneTry indicates the specified host should try to connect once,
	// but it should not be made persistent.
	ANOneTry AddNodeCommand = "onetry"
)

// String returns the AddNodeCommand in human-readable form.
func (cmd AddNodeCommand) String() string {
	return string(cmd)
}

// FutureAddNodeResult is a future promise to deliver the result of an
// AddNodeAsync RPC invocation (or an applicable error).
type FutureAddNodeResult cmdRes

// Receive waits for the response promised by the future and returns an error if
// any occurred when performing the specified command.
func (r *FutureAddNodeResult) Receive() error {
	_, err := receiveFuture(r.ctx, r.c)
	return err
}

// AddNodeAsync returns an instance of a type that can be used to get the result
// of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See AddNode for the blocking version and more details.
func (c *Client) AddNodeAsync(ctx context.Context, host string, command AddNodeCommand) *FutureAddNodeResult {
	cmd := chainjson.NewAddNodeCmd(host, chainjson.AddNodeSubCmd(command))
	return (*FutureAddNodeResult)(c.sendCmd(ctx, cmd))
}

// AddNode attempts to perform the passed command on the passed persistent peer.
// For example, it can be used to add or a remove a persistent peer, or to do
// a one time connection to a peer.
//
// It may not be used to remove non-persistent peers.
func (c *Client) AddNode(ctx context.Context, host string, command AddNodeCommand) error {
	return c.AddNodeAsync(ctx, host, command).Receive()
}

// FutureGetAddedNodeInfoResult is a future promise to deliver the result of a
// GetAddedNodeInfoAsync RPC invocation (or an applicable error).
type FutureGetAddedNodeInfoResult cmdRes

// Receive waits for the response promised by the future and returns information
// about manually added (persistent) peers.
func (r *FutureGetAddedNodeInfoResult) Receive() ([]chainjson.GetAddedNodeInfoResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal as an array of getaddednodeinfo result objects.
	var nodeInfo []chainjson.GetAddedNodeInfoResult
	err = json.Unmarshal(res, &nodeInfo)
	if err != nil {
		return nil, err
	}

	return nodeInfo, nil
}

// GetAddedNodeInfoAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetAddedNodeInfo for the blocking version and more details.
func (c *Client) GetAddedNodeInfoAsync(ctx context.Context, peer string) *FutureGetAddedNodeInfoResult {
	cmd := chainjson.NewGetAddedNodeInfoCmd(true, &peer)
	return (*FutureGetAddedNodeInfoResult)(c.sendCmd(ctx, cmd))
}

// GetAddedNodeInfo returns information about manually added (persistent) peers.
//
// See GetAddedNodeInfoNoDNS to retrieve only a list of the added (persistent)
// peers.
func (c *Client) GetAddedNodeInfo(ctx context.Context, peer string) ([]chainjson.GetAddedNodeInfoResult, error) {
	return c.GetAddedNodeInfoAsync(ctx, peer).Receive()
}

// FutureGetAddedNodeInfoNoDNSResult is a future promise to deliver the result
// of a GetAddedNodeInfoNoDNSAsync RPC invocation (or an applicable error).
type FutureGetAddedNodeInfoNoDNSResult cmdRes

// Receive waits for the response promised by the future and returns a list of
// manually added (persistent) peers.
func (r *FutureGetAddedNodeInfoNoDNSResult) Receive() ([]string, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as an array of strings.
	var nodes []string
	err = json.Unmarshal(res, &nodes)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}

// GetAddedNodeInfoNoDNSAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetAddedNodeInfoNoDNS for the blocking version and more details.
func (c *Client) GetAddedNodeInfoNoDNSAsync(ctx context.Context, peer string) *FutureGetAddedNodeInfoNoDNSResult {
	cmd := chainjson.NewGetAddedNodeInfoCmd(false, &peer)
	return (*FutureGetAddedNodeInfoNoDNSResult)(c.sendCmd(ctx, cmd))
}

// GetAddedNodeInfoNoDNS returns a list of manually added (persistent) peers.
// This works by setting the dns flag to false in the underlying RPC.
//
// See GetAddedNodeInfo to obtain more information about each added (persistent)
// peer.
func (c *Client) GetAddedNodeInfoNoDNS(ctx context.Context, peer string) ([]string, error) {
	return c.GetAddedNodeInfoNoDNSAsync(ctx, peer).Receive()
}

// FutureGetConnectionCountResult is a future promise to deliver the result
// of a GetConnectionCountAsync RPC invocation (or an applicable error).
type FutureGetConnectionCountResult cmdRes

// Receive waits for the response promised by the future and returns the number
// of active connections to other peers.
func (r *FutureGetConnectionCountResult) Receive() (int64, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return 0, err
	}

	// Unmarshal result as an int64.
	var count int64
	err = json.Unmarshal(res, &count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// GetConnectionCountAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetConnectionCount for the blocking version and more details.
func (c *Client) GetConnectionCountAsync(ctx context.Context) *FutureGetConnectionCountResult {
	cmd := chainjson.NewGetConnectionCountCmd()
	return (*FutureGetConnectionCountResult)(c.sendCmd(ctx, cmd))
}

// GetConnectionCount returns the number of active connections to other peers.
func (c *Client) GetConnectionCount(ctx context.Context) (int64, error) {
	return c.GetConnectionCountAsync(ctx).Receive()
}

// FuturePingResult is a future promise to deliver the result of a PingAsync RPC
// invocation (or an applicable error).
type FuturePingResult cmdRes

// Receive waits for the response promised by the future and returns the result
// of queueing a ping to be sent to each connected peer.
func (r *FuturePingResult) Receive() error {
	_, err := receiveFuture(r.ctx, r.c)
	return err
}

// PingAsync returns an instance of a type that can be used to get the result of
// the RPC at some future time by invoking the Receive function on the returned
// instance.
//
// See Ping for the blocking version and more details.
func (c *Client) PingAsync(ctx context.Context) *FuturePingResult {
	cmd := chainjson.NewPingCmd()
	return (*FuturePingResult)(c.sendCmd(ctx, cmd))
}

// Ping queues a ping to be sent to each connected peer.
//
// Use the GetPeerInfo function and examine the PingTime and PingWait fields to
// access the ping times.
func (c *Client) Ping(ctx context.Context) error {
	return c.PingAsync(ctx).Receive()
}

// FutureGetPeerInfoResult is a future promise to deliver the result of a
// GetPeerInfoAsync RPC invocation (or an applicable error).
type FutureGetPeerInfoResult cmdRes

// Receive waits for the response promised by the future and returns data about
// each connected network peer.
func (r *FutureGetPeerInfoResult) Receive() ([]chainjson.GetPeerInfoResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as an array of getpeerinfo result objects.
	var peerInfo []chainjson.GetPeerInfoResult
	err = json.Unmarshal(res, &peerInfo)
	if err != nil {
		return nil, err
	}

	return peerInfo, nil
}

// GetPeerInfoAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetPeerInfo for the blocking version and more details.
func (c *Client) GetPeerInfoAsync(ctx context.Context) *FutureGetPeerInfoResult {
	cmd := chainjson.NewGetPeerInfoCmd()
	return (*FutureGetPeerInfoResult)(c.sendCmd(ctx, cmd))
}

// GetPeerInfo returns data about each connected network peer.
func (c *Client) GetPeerInfo(ctx context.Context) ([]chainjson.GetPeerInfoResult, error) {
	return c.GetPeerInfoAsync(ctx).Receive()
}

// FutureGetNetTotalsResult is a future promise to deliver the result of a
// GetNetTotalsAsync RPC invocation (or an applicable error).
type FutureGetNetTotalsResult cmdRes

// Receive waits for the response promised by the future and returns network
// traffic statistics.
func (r *FutureGetNetTotalsResult) Receive() (*chainjson.GetNetTotalsResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a getnettotals result object.
	var totals chainjson.GetNetTotalsResult
	err = json.Unmarshal(res, &totals)
	if err != nil {
		return nil, err
	}

	return &totals, nil
}

// GetNetTotalsAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetNetTotals for the blocking version and more details.
func (c *Client) GetNetTotalsAsync(ctx context.Context) *FutureGetNetTotalsResult {
	cmd := chainjson.NewGetNetTotalsCmd()
	return (*FutureGetNetTotalsResult)(c.sendCmd(ctx, cmd))
}

// GetNetTotals returns network traffic statistics.
func (c *Client) GetNetTotals(ctx context.Context) (*chainjson.GetNetTotalsResult, error) {
	return c.GetNetTotalsAsync(ctx).Receive()
}

// FutureGetNetworkInfoResult is a future promise to deliver the result of a
// GetNetworkInfo RPC invocation (or an applicable error).
type FutureGetNetworkInfoResult cmdRes

// Receive waits for the response promised by the future and returns data
// related to network of the underlying node instance.
func (r *FutureGetNetworkInfoResult) Receive() (*chainjson.GetNetworkInfoResult, error) {
	res, err := receiveFuture(r.ctx, r.c)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a getnetworkinfo result object.
	var networkInfo chainjson.GetNetworkInfoResult
	err = json.Unmarshal(res, &networkInfo)
	if err != nil {
		return nil, err
	}

	return &networkInfo, nil
}

// GetNetworkInfoAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function
// on the returned instance.
//
// See GetNetworkInfo for the blocking version and more details.
func (c *Client) GetNetworkInfoAsync(ctx context.Context) *FutureGetNetworkInfoResult {
	cmd := chainjson.NewGetNetworkInfoCmd()
	return (*FutureGetNetworkInfoResult)(c.sendCmd(ctx, cmd))
}

// GetNetworkInfo returns network-related data about the underlying node.
func (c *Client) GetNetworkInfo(ctx context.Context) (*chainjson.GetNetworkInfoResult, error) {
	return c.GetNetworkInfoAsync(ctx).Receive()
}
