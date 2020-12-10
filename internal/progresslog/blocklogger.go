// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package progresslog

import (
	"sync"
	"time"

	"github.com/decred/dcrd/wire"
	"github.com/decred/slog"
)

// BlockLogger provides periodic logging for other services in order to show
// users progress of certain "actions" involving some or all current blocks.
// For example, syncing to best chain, indexing all blocks, etc.
type BlockLogger struct {
	sync.Mutex
	subsystemLogger slog.Logger
	progressAction  string

	receivedLogBlocks      int64
	receivedLogTx          int64
	receivedLogVotes       int64
	receivedLogRevocations int64
	receivedLogTickets     int64
	lastBlockLogTime       time.Time
}

// New returns a new block progress logger.
//
// The progress message is templated as follows:
//  {progressAction} {numProcessed} {blocks|block} in the last {timePeriod}
//  ({numTxs} {transactions|transaction}, {numTickets} {tickets|ticket},
//  {numVotes} {votes|vote}, {numRevocations} {revocations|revocation},
//  height {lastBlockHeight}, {lastBlockTimeStamp})
func New(progressMessage string, logger slog.Logger) *BlockLogger {
	return &BlockLogger{
		lastBlockLogTime: time.Now(),
		progressAction:   progressMessage,
		subsystemLogger:  logger,
	}
}

// LogBlockHeight logs a new block height as an information message to show
// progress to the user.  In order to prevent spam, it limits logging to one
// message every 10 seconds with duration and totals included.
func (b *BlockLogger) LogBlockHeight(block *wire.MsgBlock, syncHeight int64) {
	b.Lock()
	defer b.Unlock()

	header := &block.Header
	b.receivedLogBlocks++
	b.receivedLogTx += int64(len(block.Transactions))
	b.receivedLogVotes += int64(header.Voters)
	b.receivedLogRevocations += int64(header.Revocations)
	b.receivedLogTickets += int64(header.FreshStake)
	now := time.Now()
	duration := now.Sub(b.lastBlockLogTime)
	if int64(header.Height) < syncHeight && duration < time.Second*10 {
		return
	}

	// Truncate the duration to 10s of milliseconds.
	durationMillis := int64(duration / time.Millisecond)
	tDuration := 10 * time.Millisecond * time.Duration(durationMillis/10)

	// Log information about new block height.
	blockStr := "blocks"
	if b.receivedLogBlocks == 1 {
		blockStr = "block"
	}
	txStr := "transactions"
	if b.receivedLogTx == 1 {
		txStr = "transaction"
	}
	ticketStr := "tickets"
	if b.receivedLogTickets == 1 {
		ticketStr = "ticket"
	}
	revocationStr := "revocations"
	if b.receivedLogRevocations == 1 {
		revocationStr = "revocation"
	}
	voteStr := "votes"
	if b.receivedLogVotes == 1 {
		voteStr = "vote"
	}
	b.subsystemLogger.Infof("%s %d %s in the last %s (%d %s, %d %s, %d %s, "+
		"%d %s, height %d, %s)", b.progressAction, b.receivedLogBlocks,
		blockStr, tDuration, b.receivedLogTx, txStr, b.receivedLogTickets,
		ticketStr, b.receivedLogVotes, voteStr, b.receivedLogRevocations,
		revocationStr, header.Height, header.Timestamp)

	b.receivedLogBlocks = 0
	b.receivedLogTx = 0
	b.receivedLogVotes = 0
	b.receivedLogTickets = 0
	b.receivedLogRevocations = 0
	b.lastBlockLogTime = now
}

// SetLastLogTime updates the last time data was logged to the provided time.
func (b *BlockLogger) SetLastLogTime(time time.Time) {
	b.Lock()
	b.lastBlockLogTime = time
	b.Unlock()
}
