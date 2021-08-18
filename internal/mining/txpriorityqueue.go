// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"container/heap"

	"github.com/decred/dcrd/blockchain/stake/v4"
)

// txPrioItem houses a transaction along with extra information that allows the
// transaction to be prioritized and track dependencies on other transactions
// which have not been mined into a block yet.
type txPrioItem struct {
	txDesc         *TxDesc
	txType         stake.TxType
	autoRevocation bool
	fee            int64
	priority       float64
	feePerKB       float64
}

// txPriorityQueueLessFunc describes a function that can be used as a compare
// function for a transaction priority queue (txPriorityQueue).
type txPriorityQueueLessFunc func(*txPriorityQueue, int, int) bool

// txPriorityQueue implements a priority queue of txPrioItem elements that
// supports an arbitrary compare function as defined by txPriorityQueueLessFunc.
type txPriorityQueue struct {
	lessFunc txPriorityQueueLessFunc
	items    []*txPrioItem
}

// Len returns the number of items in the priority queue.  It is part of the
// heap.Interface implementation.
func (pq *txPriorityQueue) Len() int {
	return len(pq.items)
}

// Less returns whether the item in the priority queue with index i should sort
// before the item with index j by deferring to the assigned less function.  It
// is part of the heap.Interface implementation.
func (pq *txPriorityQueue) Less(i, j int) bool {
	return pq.lessFunc(pq, i, j)
}

// Swap swaps the items at the passed indices in the priority queue.  It is
// part of the heap.Interface implementation.
func (pq *txPriorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
}

// Push pushes the passed item onto the priority queue.  It is part of the
// heap.Interface implementation.
func (pq *txPriorityQueue) Push(x interface{}) {
	pq.items = append(pq.items, x.(*txPrioItem))
}

// Pop removes the highest priority item (according to Less) from the priority
// queue and returns it.  It is part of the heap.Interface implementation.
func (pq *txPriorityQueue) Pop() interface{} {
	n := len(pq.items)
	item := pq.items[n-1]
	pq.items[n-1] = nil
	pq.items = pq.items[0 : n-1]
	return item
}

// SetLessFunc sets the compare function for the priority queue to the provided
// function.  It also invokes heap.Init on the priority queue using the new
// function so it can immediately be used with heap.Push/Pop.
func (pq *txPriorityQueue) SetLessFunc(lessFunc txPriorityQueueLessFunc) {
	pq.lessFunc = lessFunc
	heap.Init(pq)
}

// stakePriority is an integer that is used to sort stake transactions
// by importance when they enter the min heap for block construction.  The
// priority is:
//   - 3 is for votes (highest)
//   - 2 for automatic revocations
//   - 1 for tickets
//   - 0 for regular transactions and revocations (lowest)
type stakePriority int

const (
	regOrRevocPriority stakePriority = iota
	ticketPriority
	autoRevocPriority
	votePriority
)

// stakePriority assigns a stake priority based on a transaction type.
func txStakePriority(txType stake.TxType, autoRevocation bool) stakePriority {
	prio := regOrRevocPriority
	switch {
	case txType == stake.TxTypeSSGen:
		prio = votePriority
	case txType == stake.TxTypeSSRtx && autoRevocation:
		prio = autoRevocPriority
	case txType == stake.TxTypeSStx:
		prio = ticketPriority
	}

	return prio
}

// compareStakePriority compares the stake priority of two transactions.
// It uses votes > tickets > regular transactions or revocations. It
// returns 1 if i > j, 0 if i == j, and -1 if i < j in terms of stake
// priority.
func compareStakePriority(i, j *txPrioItem) int {
	iStakePriority := txStakePriority(i.txType, i.autoRevocation)
	jStakePriority := txStakePriority(j.txType, j.autoRevocation)

	if iStakePriority > jStakePriority {
		return 1
	}
	if iStakePriority < jStakePriority {
		return -1
	}
	return 0
}

// txPQByStakeAndFee sorts a txPriorityQueue by stake priority, followed by
// fees per kilobyte, and then transaction priority.
func txPQByStakeAndFee(pq *txPriorityQueue, i, j int) bool {
	// Sort by stake priority, continue if they're the same stake priority.
	cmp := compareStakePriority(pq.items[i], pq.items[j])
	if cmp == 1 {
		return true
	}
	if cmp == -1 {
		return false
	}

	// Using > here so that pop gives the highest fee item as opposed
	// to the lowest.  Sort by fee first, then priority.
	if pq.items[i].feePerKB == pq.items[j].feePerKB {
		return pq.items[i].priority > pq.items[j].priority
	}

	// The stake priorities are equal, so return based on fees
	// per KB.
	return pq.items[i].feePerKB > pq.items[j].feePerKB
}

// txPQByStakeAndFeeAndThenPriority sorts a txPriorityQueue by stake priority,
// followed by fees per kilobyte, and then if the transaction type is regular
// or a revocation it sorts it by priority.
func txPQByStakeAndFeeAndThenPriority(pq *txPriorityQueue, i, j int) bool {
	// Sort by stake priority, continue if they're the same stake priority.
	cmp := compareStakePriority(pq.items[i], pq.items[j])
	if cmp == 1 {
		return true
	}
	if cmp == -1 {
		return false
	}

	iPrio := txStakePriority(pq.items[i].txType, pq.items[i].autoRevocation)
	jPrio := txStakePriority(pq.items[j].txType, pq.items[j].autoRevocation)
	bothAreLowStakePriority := iPrio == regOrRevocPriority &&
		jPrio == regOrRevocPriority

	// Use fees per KB on high stake priority transactions.
	if !bothAreLowStakePriority {
		return pq.items[i].feePerKB > pq.items[j].feePerKB
	}

	// Both transactions are of low stake importance. Use > here so that
	// pop gives the highest priority item as opposed to the lowest.
	// Sort by priority first, then fee.
	if pq.items[i].priority == pq.items[j].priority {
		return pq.items[i].feePerKB > pq.items[j].feePerKB
	}

	return pq.items[i].priority > pq.items[j].priority
}

// newTxPriorityQueue returns a new transaction priority queue that reserves the
// passed amount of space for the elements.  The new priority queue uses the
// less than function lessFunc to sort the items in the min heap. The priority
// queue can grow larger than the reserved space, but extra copies of the
// underlying array can be avoided by reserving a sane value.
func newTxPriorityQueue(reserve int, lessFunc func(*txPriorityQueue, int, int) bool) *txPriorityQueue {
	pq := &txPriorityQueue{
		items: make([]*txPrioItem, 0, reserve),
	}
	pq.SetLessFunc(lessFunc)
	return pq
}
