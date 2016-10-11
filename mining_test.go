// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/heap"
	"math/rand"
	"testing"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrutil"
)

// TestTxFeePrioHeap ensures the priority queue for transaction fees and
// priorities works as expected. It doesn't set anything for the stake
// priority, so this test only tests sorting by fee and then priority.
func TestTxFeePrioHeap(t *testing.T) {
	// Create some fake priority items that exercise the expected sort
	// edge conditions.
	testItems := []*txPrioItem{
		{feePerKB: 5678, priority: 1},
		{feePerKB: 5678, priority: 1}, // Duplicate fee and prio
		{feePerKB: 5678, priority: 5},
		{feePerKB: 5678, priority: 2},
		{feePerKB: 1234, priority: 3},
		{feePerKB: 1234, priority: 1},
		{feePerKB: 1234, priority: 5},
		{feePerKB: 1234, priority: 5}, // Duplicate fee and prio
		{feePerKB: 1234, priority: 2},
		{feePerKB: 10000, priority: 0}, // Higher fee, smaller prio
		{feePerKB: 0, priority: 10000}, // Higher prio, lower fee
	}
	numItems := len(testItems)

	// Add random data in addition to the edge conditions already manually
	// specified.
	randSeed := rand.Int63()
	defer func() {
		if t.Failed() {
			t.Logf("Random numbers using seed: %v", randSeed)
		}
	}()
	prng := rand.New(rand.NewSource(randSeed))
	for i := 0; i < 1000; i++ {
		testItems = append(testItems, &txPrioItem{
			feePerKB: prng.Float64() * dcrutil.AtomsPerCoin,
			priority: prng.Float64() * 100,
		})
	}

	// Test sorting by fee per KB then priority.
	var highest *txPrioItem
	priorityQueue := newTxPriorityQueue(len(testItems), txPQByFee)
	for i := 0; i < len(testItems); i++ {
		prioItem := testItems[i]
		if highest == nil {
			highest = prioItem
		}
		if prioItem.feePerKB >= highest.feePerKB {
			highest = prioItem

			if prioItem.feePerKB == highest.feePerKB {
				if prioItem.priority >= highest.priority {
					highest = prioItem
				}
			}
		}
		heap.Push(priorityQueue, prioItem)
	}

	for i := 0; i < len(testItems); i++ {
		prioItem := heap.Pop(priorityQueue).(*txPrioItem)
		feesEqual := false
		switch {
		case prioItem.feePerKB > highest.feePerKB:
			t.Fatalf("priority sort: item (fee per KB: %v, "+
				"priority: %v) higher than than prev "+
				"(fee per KB: %v, priority %v)",
				prioItem.feePerKB, prioItem.priority,
				highest.feePerKB, highest.priority)
		case prioItem.feePerKB == highest.feePerKB:
			feesEqual = true
		default:
		}
		if feesEqual {
			switch {
			case prioItem.priority > highest.priority:
				t.Fatalf("priority sort: item (fee per KB: %v, "+
					"priority: %v) higher than than prev "+
					"(fee per KB: %v, priority %v)",
					prioItem.feePerKB, prioItem.priority,
					highest.feePerKB, highest.priority)
			case prioItem.priority == highest.priority:
			default:
			}
		}

		highest = prioItem
	}

	// Test sorting by priority then fee per KB.
	highest = nil
	priorityQueue = newTxPriorityQueue(numItems, txPQByPriority)
	for i := 0; i < len(testItems); i++ {
		prioItem := testItems[i]
		if highest == nil {
			highest = prioItem
		}
		if prioItem.priority >= highest.priority {
			highest = prioItem

			if prioItem.priority == highest.priority {
				if prioItem.feePerKB >= highest.feePerKB {
					highest = prioItem
				}
			}
		}
		heap.Push(priorityQueue, prioItem)
	}

	for i := 0; i < len(testItems); i++ {
		prioItem := heap.Pop(priorityQueue).(*txPrioItem)
		prioEqual := false
		switch {
		case prioItem.priority > highest.priority:
			t.Fatalf("priority sort: item (fee per KB: %v, "+
				"priority: %v) higher than than prev "+
				"(fee per KB: %v, priority %v)",
				prioItem.feePerKB, prioItem.priority,
				highest.feePerKB, highest.priority)
		case prioItem.priority == highest.priority:
			prioEqual = true
		default:
		}
		if prioEqual {
			switch {
			case prioItem.feePerKB > highest.feePerKB:
				t.Fatalf("priority sort: item (fee per KB: %v, "+
					"priority: %v) higher than than prev "+
					"(fee per KB: %v, priority %v)",
					prioItem.feePerKB, prioItem.priority,
					highest.feePerKB, highest.priority)
			case prioItem.priority == highest.priority:
			default:
			}
		}

		highest = prioItem
	}
}

// TestTxStakeSortingWithSizeHeap ensures the priority queue for stake
// transactions, sorting by stake priority, ticket relative absolute
// fee for small tickets, relative fee per ticket size, then lower
// stake priority by relative fee per transaction size works as expected.
func TestTxStakeSortingWithSizeHeap(t *testing.T) {
	maxPriorityTicketSize := 2500
	configSetTicketPrioSize = 2500

	// Create some fake priority items that exercise the expected sort
	// edge conditions.
	testItems := []*txPrioItem{
		{feePerKB: 5678, priority: 5, txType: stake.TxTypeSStx, txSize: 100},
		{feePerKB: 2840, priority: 2, txType: stake.TxTypeSStx, txSize: 200}, // Higher absolute fees despite relative fees
		{feePerKB: 1234, priority: 3, txType: stake.TxTypeSStx, txSize: 3000},
		{feePerKB: 1235, priority: 1, txType: stake.TxTypeSStx, txSize: 3000}, // Too big size, higher fee per KB
		{feePerKB: 5678, priority: 1, txType: stake.TxTypeSSGen, txSize: 100}, // Votes go first
		{feePerKB: 5678, priority: 1, txType: stake.TxTypeSSGen, txSize: 200},
		{feePerKB: 1234, priority: 2, txType: stake.TxTypeRegular, txSize: 100},
		{feePerKB: 1234, priority: 5, txType: stake.TxTypeRegular, txSize: 100},  // Higher priority ticket all else accounted for
		{feePerKB: 10000, priority: 0, txType: stake.TxTypeRegular, txSize: 100}, // Higher fee, smaller prio
		{feePerKB: 0, priority: 10000, txType: stake.TxTypeRegular, txSize: 100}, // Higher prio, lower fee
	}

	// Add random data in addition to the edge conditions already manually
	// specified.
	randSeed := rand.Int63()
	defer func() {
		if t.Failed() {
			t.Logf("Random numbers using seed: %v", randSeed)
		}
	}()
	prng := rand.New(rand.NewSource(randSeed))
	for i := 0; i < 1000; i++ {
		testItems = append(testItems, &txPrioItem{
			feePerKB: prng.Float64() * dcrutil.AtomsPerCoin,
			priority: prng.Float64() * 100,
			txSize:   prng.Intn(6000),
		})
	}
	for i := range testItems {
		// This should be scaled for kilobyte, but ignore that here for
		// the tests.
		testItems[i].fee = int64(testItems[i].feePerKB) *
			int64(testItems[i].txSize)
	}

	// Make a queue and then pop items off it, checking to see if
	// the highest item we get is also highest priority in a
	// refactor of the actual code.
	priorityQueue := newTxPriorityQueue(len(testItems),
		txPQByStakeSizeAndFeeAndThenPriority)
	for i := 0; i < len(testItems); i++ {
		heap.Push(priorityQueue, testItems[i])
	}

	fatalError := func(where string, i, j *txPrioItem) {
		t.Fatalf("priority sort at %s: item (fee per KB: %v, "+
			"priority: %v, txType %v, txSize %v, fee %v) "+
			"higher than than prev (fee per KB: %v, "+
			"priority %v, txType %v, txSize %v, fee %v)",
			where,
			i.feePerKB, i.priority, i.txType,
			i.txSize, i.fee,
			j.feePerKB, j.priority, j.txType,
			j.txSize, j.fee)
	}

	var highest *txPrioItem
	for i := 0; i < len(testItems); i++ {
		prioItem := heap.Pop(priorityQueue).(*txPrioItem)
		if highest == nil {
			highest = prioItem
			continue
		}

		txTypeEqual := false
		switch {
		case compareStakePriority(prioItem, highest) == 1:
			fatalError("compareStakePriority", prioItem, highest)
		case compareStakePriority(prioItem, highest) == 0:
			txTypeEqual = true
		default:
		}

		txFeesEqual := false
		if txTypeEqual {
			bothAreLowStakePriority :=
				txStakePriority(prioItem.txType) == regOrRevocPriority &&
					txStakePriority(highest.txType) == regOrRevocPriority
			bothAreTickets := txStakePriority(prioItem.txType) == ticketPriority &&
				txStakePriority(highest.txType) == ticketPriority
			bothAreSmall := prioItem.txSize < maxPriorityTicketSize &&
				highest.txSize < maxPriorityTicketSize

			switch {
			case (!bothAreLowStakePriority && bothAreTickets && bothAreSmall):
				if prioItem.fee > highest.fee {
					fatalError("both small tickets", prioItem, highest)
				}
			case !bothAreLowStakePriority:
				if prioItem.feePerKB > highest.feePerKB {
					fatalError("not both small tickets", prioItem, highest)
				}
			case prioItem.feePerKB == highest.feePerKB:
				txFeesEqual = true
			default:
			}
		}

		if txFeesEqual {
			if prioItem.priority > highest.priority {
				fatalError("priority", prioItem, highest)
			}
		}

		highest = prioItem
	}
}

// TestStakeTxFeePrioHeap tests the priority heaps including the stake types for
// both transaction fees per KB and transaction priority. It ensures that the
// primary sorting is first by stake type, and then by the latter chosen priority
// type.
func TestStakeTxFeePrioHeap(t *testing.T) {
	numElements := 1000
	ph := newTxPriorityQueue(numElements, txPQByStakeAndFee)

	for i := 0; i < numElements; i++ {
		randType := stake.TxType(rand.Intn(4))
		randPrio := rand.Float64() * 100
		randFeePerKB := rand.Float64() * 10
		prioItem := &txPrioItem{
			tx:       nil,
			txType:   randType,
			priority: randPrio,
			feePerKB: randFeePerKB,
		}
		heap.Push(ph, prioItem)
	}

	// Test sorting by stake and fee per KB.
	last := &txPrioItem{
		tx:       nil,
		txType:   stake.TxTypeSSGen,
		priority: 10000.0,
		feePerKB: 10000.0,
	}
	for i := 0; i < numElements; i++ {
		prioItem := heap.Pop(ph)
		txpi, ok := prioItem.(*txPrioItem)
		if ok {
			if txpi.feePerKB > last.feePerKB &&
				compareStakePriority(txpi, last) >= 0 {
				t.Errorf("bad pop: %v fee per KB was more than last of %v "+
					"while the txtype was %v but last was %v",
					txpi.feePerKB, last.feePerKB, txpi.txType, last.txType)
			}
			last = txpi
		}
	}

	ph = newTxPriorityQueue(numElements, txPQByStakeAndPriority)
	for i := 0; i < numElements; i++ {
		randType := stake.TxType(rand.Intn(4))
		randPrio := rand.Float64() * 100
		randFeePerKB := rand.Float64() * 10
		prioItem := &txPrioItem{
			tx:       nil,
			txType:   randType,
			priority: randPrio,
			feePerKB: randFeePerKB,
		}
		heap.Push(ph, prioItem)
	}

	// Test sorting by stake and priority.
	last = &txPrioItem{
		tx:       nil,
		txType:   stake.TxTypeSSGen,
		priority: 10000.0,
		feePerKB: 10000.0,
	}
	for i := 0; i < numElements; i++ {
		prioItem := heap.Pop(ph)
		txpi, ok := prioItem.(*txPrioItem)
		if ok {
			if txpi.priority > last.priority &&
				compareStakePriority(txpi, last) >= 0 {
				t.Errorf("bad pop: %v fee per KB was more than last of %v "+
					"while the txtype was %v but last was %v",
					txpi.feePerKB, last.feePerKB, txpi.txType, last.txType)
			}
			last = txpi
		}
	}

	ph = newTxPriorityQueue(numElements, txPQByStakeAndFeeAndThenPriority)
	for i := 0; i < numElements; i++ {
		randType := stake.TxType(rand.Intn(4))
		randPrio := rand.Float64() * 100
		randFeePerKB := rand.Float64() * 10
		prioItem := &txPrioItem{
			tx:       nil,
			txType:   randType,
			priority: randPrio,
			feePerKB: randFeePerKB,
		}
		heap.Push(ph, prioItem)
	}

	// Test sorting with fees per KB for high stake priority, then
	// priority for low stake priority.
	last = &txPrioItem{
		tx:       nil,
		txType:   stake.TxTypeSSGen,
		priority: 10000.0,
		feePerKB: 10000.0,
	}
	for i := 0; i < numElements; i++ {
		prioItem := heap.Pop(ph)
		txpi, ok := prioItem.(*txPrioItem)
		if ok {
			bothAreLowStakePriority :=
				txStakePriority(txpi.txType) == regOrRevocPriority &&
					txStakePriority(last.txType) == regOrRevocPriority
			if !bothAreLowStakePriority {
				if txpi.feePerKB > last.feePerKB &&
					compareStakePriority(txpi, last) >= 0 {
					t.Errorf("bad pop: %v fee per KB was more than last of %v "+
						"while the txtype was %v but last was %v",
						txpi.feePerKB, last.feePerKB, txpi.txType, last.txType)
				}
			}
			if bothAreLowStakePriority {
				if txpi.priority > last.priority &&
					compareStakePriority(txpi, last) >= 0 {
					t.Errorf("bad pop: %v priority was more than last of %v "+
						"while the txtype was %v but last was %v",
						txpi.feePerKB, last.feePerKB, txpi.txType, last.txType)
				}
			}
			last = txpi
		}
	}
}
