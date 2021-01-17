// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package progresslog

import (
	"sync"
	"time"

	"github.com/decred/dcrd/wire"
	"github.com/decred/slog"
)

// pickNoun returns the singular or plural form of a noun depending on the
// provided count.
func pickNoun(n uint64, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// Logger provides periodic logging of progress towards some action such as
// syncing the chain.
type Logger struct {
	sync.Mutex
	subsystemLogger slog.Logger
	progressAction  string

	// lastLogTime tracks the last time a log statement was shown.
	lastLogTime time.Time

	// These fields accumulate information about blocks between log statements.
	receivedBlocks  uint64
	receivedTxns    uint64
	receivedVotes   uint64
	receivedRevokes uint64
	receivedTickets uint64
}

// New returns a new block progress logger.
func New(progressAction string, logger slog.Logger) *Logger {
	return &Logger{
		lastLogTime:     time.Now(),
		progressAction:  progressAction,
		subsystemLogger: logger,
	}
}

// LogProgress accumulates details for the provided block and periodically
// (every 10 seconds) logs an information message to show progress to the user
// along with duration and totals included.
//
// The force flag may be used to force a log message to be shown regardless of
// the time the last one was shown.
//
// The progress message is templated as follows:
//  {progressAction} {numProcessed} {blocks|block} in the last {timePeriod}
//  ({numTxs} {transactions|transaction}, {numTickets} {tickets|ticket},
//  {numVotes} {votes|vote}, {numRevocations} {revocations|revocation},
//  height {lastBlockHeight}, {lastBlockTimeStamp})
func (l *Logger) LogProgress(block *wire.MsgBlock, forceLog bool) {
	l.Lock()
	defer l.Unlock()

	header := &block.Header
	l.receivedBlocks++
	l.receivedTxns += uint64(len(block.Transactions))
	l.receivedVotes += uint64(header.Voters)
	l.receivedRevokes += uint64(header.Revocations)
	l.receivedTickets += uint64(header.FreshStake)
	now := time.Now()
	duration := now.Sub(l.lastLogTime)
	if !forceLog && duration < time.Second*10 {
		return
	}

	// Log information about chain progress.
	l.subsystemLogger.Infof("%s %d %s in the last %0.2fs (%d %s, %d %s, %d %s,"+
		" %d %s, height %d, %s)", l.progressAction,
		l.receivedBlocks, pickNoun(l.receivedBlocks, "block", "blocks"),
		duration.Seconds(),
		l.receivedTxns, pickNoun(l.receivedTxns, "transaction", "transactions"),
		l.receivedTickets, pickNoun(l.receivedTickets, "ticket", "tickets"),
		l.receivedVotes, pickNoun(l.receivedVotes, "vote", "votes"),
		l.receivedRevokes, pickNoun(l.receivedRevokes, "revocation", "revocations"),
		header.Height, header.Timestamp)

	l.receivedBlocks = 0
	l.receivedTxns = 0
	l.receivedVotes = 0
	l.receivedTickets = 0
	l.receivedRevokes = 0
	l.lastLogTime = now
}

// SetLastLogTime updates the last time data was logged to the provided time.
func (l *Logger) SetLastLogTime(time time.Time) {
	l.Lock()
	l.lastLogTime = time
	l.Unlock()
}
