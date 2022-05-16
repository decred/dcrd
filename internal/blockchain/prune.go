// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math"
	"time"
)

// poissonConfidenceSecs returns the number of seconds it will take to produce
// an event at the provided confidence level given a Poisson distribution with 1
// event occurring in the given target interval.
func poissonConfidenceSecs(targetIntervalSecs int64, confidence float64) int64 {
	// The waiting times between events in a Poisson distribution are
	// exponentially distributed and the CDF for an exponential distribution is:
	//
	// x = 1 - e^-λt
	//
	// Since the goal rate is 1 event per target and time is in terms of
	// targetIntervalSecs, let λ = 1, i = targetIntervalSecs, t = s/i, and x =
	// confidence:
	//   => x = 1 - e^-(s/i)
	//
	// Solve for s:
	//   => e^-(s/i) = 1 - x
	//   => -(s/i) = ln(1 - x)
	//   => s = ln(1 - x) * -i
	//   => s = ln1p(-confidence) * -targetIntervalSecs
	//
	// Extra 0.5 to round up.
	return int64(math.Log1p(-confidence)*-float64(targetIntervalSecs) + 0.5)
}

// chainPruner is used to occasionally prune the blockchain of old nodes that
// can be freed to the garbage collector.
type chainPruner struct {
	chain           *BlockChain
	lastPruneTime   time.Time
	pruningInterval time.Duration

	// prunedPerIntervalHint is the maximum expected number of nodes that will
	// be pruned per pruning interval with a high degree of confidence.
	prunedPerIntervalHint int64
}

// newChainPruner returns a new chain pruner.
func newChainPruner(chain *BlockChain) *chainPruner {
	// Set the pruning interval to match the target time per block.
	targetTimePerBlock := chain.chainParams.TargetTimePerBlock
	pruningInterval := targetTimePerBlock
	pruningIntervalSecs := int64(pruningInterval.Seconds())

	// Calculate the maximum expected number of nodes that will be pruned per
	// interval with a 99% confidence level to use as a hint for reducing the
	// number of allocations.
	//
	// Note that as long as the pruning interval is the same as the target block
	// interval, this will always result in the same value for all networks,
	// but it's better to calculate it properly so it remains accurate if the
	// pruning interval is changed.
	const confidence = 0.99
	targetTimePerBlockSecs := int64(targetTimePerBlock.Seconds())
	confidenceSecs := poissonConfidenceSecs(targetTimePerBlockSecs, confidence)
	pruneHintFloat := float64(confidenceSecs) / float64(pruningIntervalSecs)
	pruneHint := int64(math.Round(pruneHintFloat))

	return &chainPruner{
		chain:                 chain,
		lastPruneTime:         time.Now(),
		pruningInterval:       pruningInterval,
		prunedPerIntervalHint: pruneHint,
	}
}

// pruneChainIfNeeded removes references to old information that should no
// longer be held in memory if the pruning interval has elapsed.
//
// This function MUST be called with the chain lock held (for writes).
func (c *chainPruner) pruneChainIfNeeded() {
	now := time.Now()
	duration := now.Sub(c.lastPruneTime)
	if duration < c.pruningInterval {
		return
	}

	c.lastPruneTime = now
	c.chain.pruneStakeNodes()
}
