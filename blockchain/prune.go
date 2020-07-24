package blockchain

import (
	"time"
)

// chainPruner is used to occasionally prune the blockchain of old nodes that
// can be freed to the garbage collector.
type chainPruner struct {
	chain           *BlockChain
	lastPruneTime   time.Time
	pruningInterval time.Duration
}

// newChainPruner returns a new chain pruner.
func newChainPruner(chain *BlockChain) *chainPruner {
	return &chainPruner{
		chain:           chain,
		lastPruneTime:   time.Now(),
		pruningInterval: chain.chainParams.TargetTimePerBlock,
	}
}

// pruneChainIfNeeded checks the current time versus the time of the last pruning.
// If the blockchain hasn't been pruned in this time, it initiates a new pruning.
//
// pruneChainIfNeeded must be called with the chainLock held for writes.
func (c *chainPruner) pruneChainIfNeeded() {
	now := time.Now()
	duration := now.Sub(c.lastPruneTime)
	if duration < c.pruningInterval {
		return
	}

	c.lastPruneTime = now
	c.chain.pruneStakeNodes()
}
