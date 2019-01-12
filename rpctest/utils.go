// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpctest

import (
	"reflect"
	"time"

	"github.com/decred/dcrd/dcrjson/v2"
	"github.com/decred/dcrd/rpcclient/v2"
)

// JoinType is an enum representing a particular type of "node join". A node
// join is a synchronization tool used to wait until a subset of nodes have a
// consistent state with respect to an attribute.
type JoinType uint8

const (
	// Blocks is a JoinType which waits until all nodes share the same
	// block height.
	Blocks JoinType = iota

	// Mempools is a JoinType which blocks until all nodes have identical
	// mempool.
	Mempools
)

// JoinNodes is a synchronization tool used to block until all passed nodes are
// fully synced with respect to an attribute. This function will block for a
// period of time, finally returning once all nodes are synced according to the
// passed JoinType. This function be used to to ensure all active test
// harnesses are at a consistent state before proceeding to an assertion or
// check within rpc tests.
func JoinNodes(nodes []*Harness, joinType JoinType) error {
	switch joinType {
	case Blocks:
		return syncBlocks(nodes)
	case Mempools:
		return syncMempools(nodes)
	}
	return nil
}

// syncMempools blocks until all nodes have identical mempools.
func syncMempools(nodes []*Harness) error {
	poolsMatch := false

	for !poolsMatch {
	retry:
		firstPool, err := nodes[0].Node.GetRawMempool(dcrjson.GRMAll)
		if err != nil {
			return err
		}

		// If all nodes have an identical mempool with respect to the
		// first node, then we're done. Otherwise, drop back to the top
		// of the loop and retry after a short wait period.
		for _, node := range nodes[1:] {
			nodePool, err := node.Node.GetRawMempool(dcrjson.GRMAll)
			if err != nil {
				return err
			}

			if !reflect.DeepEqual(firstPool, nodePool) {
				time.Sleep(time.Millisecond * 100)
				goto retry
			}
		}

		poolsMatch = true
	}

	return nil
}

// syncBlocks blocks until all nodes report the same block height.
func syncBlocks(nodes []*Harness) error {
	blocksMatch := false

	for !blocksMatch {
	retry:
		blockHeights := make(map[int64]struct{})

		for _, node := range nodes {
			blockHeight, err := node.Node.GetBlockCount()
			if err != nil {
				return err
			}

			blockHeights[blockHeight] = struct{}{}
			if len(blockHeights) > 1 {
				time.Sleep(time.Millisecond * 100)
				goto retry
			}
		}

		blocksMatch = true
	}

	return nil
}

// ConnectNode establishes a new peer-to-peer connection between the "from"
// harness and the "to" harness.  The connection made is flagged as persistent,
// therefore in the case of disconnects, "from" will attempt to reestablish a
// connection to the "to" harness.
func ConnectNode(from *Harness, to *Harness) error {
	peerInfo, err := from.Node.GetPeerInfo()
	if err != nil {
		return err
	}
	numPeers := len(peerInfo)

	targetAddr := to.node.config.listen
	if err := from.Node.AddNode(targetAddr, rpcclient.ANAdd); err != nil {
		return err
	}

	// Block until a new connection has been established.
	peerInfo, err = from.Node.GetPeerInfo()
	if err != nil {
		return err
	}
	for len(peerInfo) <= numPeers {
		peerInfo, err = from.Node.GetPeerInfo()
		if err != nil {
			return err
		}
	}

	return nil
}

// RemoveNode removes the peer-to-peer connection between the "from" harness and
// the "to" harness. The connection is only removed in this direction, therefore
// if the reverse connection exists, the nodes may still be connected.
//
// This function returns an error if the nodes were not previously connected.
func RemoveNode(from *Harness, to *Harness) error {
	targetAddr := to.node.config.listen
	if err := from.Node.AddNode(targetAddr, rpcclient.ANRemove); err != nil {
		// AddNode(..., ANRemove) returns an error if the peer is not found
		return err
	}

	// Block until this particular connection has been dropped.
	for {
		peerInfo, err := from.Node.GetPeerInfo()
		if err != nil {
			return err
		}
		for _, p := range peerInfo {
			if p.Addr == targetAddr {
				// Nodes still connected. Skip and re-fetch the list of nodes.
				continue
			}
		}

		// If this point is reached, then the nodes are not connected anymore.
		break
	}

	return nil
}

// NodesConnected verifies whether there is a connection via the p2p interface
// between the specified nodes. If allowReverse is true, connectivity is also
// checked in the reverse direction (to->from).
func NodesConnected(from, to *Harness, allowReverse bool) (bool, error) {
	peerInfo, err := from.Node.GetPeerInfo()
	if err != nil {
		return false, err
	}

	targetAddr := to.node.config.listen
	for _, p := range peerInfo {
		if p.Addr == targetAddr {
			return true, nil
		}
	}

	if !allowReverse {
		return false, nil
	}

	// Check in the reverse direction.
	peerInfo, err = to.Node.GetPeerInfo()
	if err != nil {
		return false, err
	}

	targetAddr = from.node.config.listen
	for _, p := range peerInfo {
		if p.Addr == targetAddr {
			return true, nil
		}
	}

	return false, nil
}

// TearDownAll tears down all active test harnesses.
func TearDownAll() error {
	harnessStateMtx.Lock()
	defer harnessStateMtx.Unlock()

	for _, harness := range testInstances {
		if err := harness.TearDown(); err != nil {
			return err
		}
	}

	return nil
}

// ActiveHarnesses returns a slice of all currently active test harnesses. A
// test harness if considered "active" if it has been created, but not yet torn
// down.
func ActiveHarnesses() []*Harness {
	harnessStateMtx.RLock()
	defer harnessStateMtx.RUnlock()

	activeNodes := make([]*Harness, 0, len(testInstances))
	for _, harness := range testInstances {
		activeNodes = append(activeNodes, harness)
	}

	return activeNodes
}
