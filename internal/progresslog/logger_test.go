// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package progresslog

import (
	"io/ioutil"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/wire"
	"github.com/decred/slog"
)

var (
	backendLog = slog.NewBackend(ioutil.Discard)
	testLog    = backendLog.Logger("TEST")
)

// TestLogProgress ensures the logging functionality works as expected via a
// test logger.
func TestLogProgress(t *testing.T) {
	testBlocks := []wire.MsgBlock{{
		Header: wire.BlockHeader{
			Version:     1,
			Height:      100000,
			Timestamp:   time.Unix(1293623863, 0), // 2010-12-29 11:57:43 +0000 UTC
			Voters:      2,
			Revocations: 1,
			FreshStake:  4,
		},
		Transactions:  make([]*wire.MsgTx, 4),
		STransactions: nil,
	}, {
		Header: wire.BlockHeader{
			Version:     1,
			Height:      100001,
			Timestamp:   time.Unix(1293624163, 0), // 2010-12-29 12:02:43 +0000 UTC
			Voters:      3,
			Revocations: 2,
			FreshStake:  2,
		},
		Transactions:  make([]*wire.MsgTx, 2),
		STransactions: nil,
	}, {
		Header: wire.BlockHeader{
			Version:     1,
			Height:      100002,
			Timestamp:   time.Unix(1293624463, 0), // 2010-12-29 12:07:43 +0000 UTC
			Voters:      1,
			Revocations: 3,
			FreshStake:  1,
		},
		Transactions:  make([]*wire.MsgTx, 3),
		STransactions: nil,
	}}

	tests := []struct {
		name                string
		reset               bool
		inputBlock          *wire.MsgBlock
		forceLog            bool
		inputLastLogTime    time.Time
		wantReceivedBlocks  uint64
		wantReceivedTxns    uint64
		wantReceivedVotes   uint64
		wantReceivedRevokes uint64
		wantReceivedTickets uint64
	}{{
		name:                "round 1, block 0, last log time < 10 secs ago, not forced",
		inputBlock:          &testBlocks[0],
		forceLog:            false,
		inputLastLogTime:    time.Now(),
		wantReceivedBlocks:  1,
		wantReceivedTxns:    4,
		wantReceivedVotes:   2,
		wantReceivedRevokes: 1,
		wantReceivedTickets: 4,
	}, {
		name:                "round 1, block 1, last log time < 10 secs ago, not forced",
		inputBlock:          &testBlocks[1],
		forceLog:            false,
		inputLastLogTime:    time.Now(),
		wantReceivedBlocks:  2,
		wantReceivedTxns:    6,
		wantReceivedVotes:   5,
		wantReceivedRevokes: 3,
		wantReceivedTickets: 6,
	}, {
		name:                "round 1, block 2, last log time < 10 secs ago, forced",
		inputBlock:          &testBlocks[2],
		forceLog:            true,
		inputLastLogTime:    time.Now(),
		wantReceivedBlocks:  0,
		wantReceivedTxns:    0,
		wantReceivedVotes:   0,
		wantReceivedRevokes: 0,
		wantReceivedTickets: 0,
	}, {
		name:                "round 2, block 0, last log time < 10 secs ago, not forced",
		reset:               true,
		inputBlock:          &testBlocks[0],
		forceLog:            false,
		inputLastLogTime:    time.Now(),
		wantReceivedBlocks:  1,
		wantReceivedTxns:    4,
		wantReceivedVotes:   2,
		wantReceivedRevokes: 1,
		wantReceivedTickets: 4,
	}, {
		name:                "round 2, block 1, last log time > 10 secs ago, not forced",
		inputBlock:          &testBlocks[1],
		forceLog:            false,
		inputLastLogTime:    time.Now().Add(-11 * time.Second),
		wantReceivedBlocks:  0,
		wantReceivedTxns:    0,
		wantReceivedVotes:   0,
		wantReceivedRevokes: 0,
		wantReceivedTickets: 0,
	}, {
		name:                "round 2, block 2, last log time > 10 secs ago, forced",
		inputBlock:          &testBlocks[2],
		forceLog:            true,
		inputLastLogTime:    time.Now().Add(-11 * time.Second),
		wantReceivedBlocks:  0,
		wantReceivedTxns:    0,
		wantReceivedVotes:   0,
		wantReceivedRevokes: 0,
		wantReceivedTickets: 0,
	}}

	progressFn := func() float64 { return 0.0 }
	progressLogger := New("Wrote", testLog)
	for _, test := range tests {
		if test.reset {
			progressLogger = New("Wrote", testLog)
		}
		progressLogger.SetLastLogTime(test.inputLastLogTime)
		progressLogger.LogProgress(test.inputBlock, test.forceLog, progressFn)
		wantBlockProgressLogger := &Logger{
			receivedBlocks:  test.wantReceivedBlocks,
			receivedTxns:    test.wantReceivedTxns,
			receivedVotes:   test.wantReceivedVotes,
			receivedRevokes: test.wantReceivedRevokes,
			receivedTickets: test.wantReceivedTickets,
			lastLogTime:     progressLogger.lastLogTime,
			progressAction:  progressLogger.progressAction,
			subsystemLogger: progressLogger.subsystemLogger,
		}
		if !reflect.DeepEqual(progressLogger, wantBlockProgressLogger) {
			t.Errorf("%s:\nwant: %+v\ngot: %+v\n", test.name,
				wantBlockProgressLogger, progressLogger)
		}
	}
}

// TestLogHeaderProgress ensures the logging functionality for headers works as
// expected via a test logger.
func TestLogHeaderProgress(t *testing.T) {
	tests := []struct {
		name                string
		reset               bool
		numHeaders          uint64
		forceLog            bool
		lastLogTime         time.Time
		wantReceivedHeaders uint64
	}{{
		name:                "round 1, batch 1, last log time < 10 secs ago, not forced",
		numHeaders:          35500,
		forceLog:            false,
		lastLogTime:         time.Now(),
		wantReceivedHeaders: 35500,
	}, {
		name:                "round 1, batch 2, last log time < 10 secs ago, not forced",
		numHeaders:          40000,
		forceLog:            false,
		lastLogTime:         time.Now(),
		wantReceivedHeaders: 75500,
	}, {
		name:                "round 1, batch 3, last log time < 10 secs ago, not forced",
		numHeaders:          66500,
		forceLog:            false,
		lastLogTime:         time.Now(),
		wantReceivedHeaders: 142000,
	}, {
		name:                "round 2, batch 1, last log time < 10 secs ago, not forced",
		reset:               true,
		numHeaders:          40000,
		forceLog:            false,
		lastLogTime:         time.Now(),
		wantReceivedHeaders: 40000,
	}, {
		name:                "round 2, batch 2, last log time > 10 secs ago, not forced",
		numHeaders:          66500,
		forceLog:            false,
		lastLogTime:         time.Now().Add(-11 * time.Second),
		wantReceivedHeaders: 0,
	}, {
		name:                "round 2, batch 1, last log time < 10 secs ago, forced",
		reset:               true,
		numHeaders:          54330,
		forceLog:            true,
		lastLogTime:         time.Now(),
		wantReceivedHeaders: 0,
	}}

	progressFn := func() float64 { return 0.0 }
	logger := New("Wrote", testLog)
	for _, test := range tests {
		if test.reset {
			logger = New("Wrote", testLog)
		}
		logger.SetLastLogTime(test.lastLogTime)
		logger.LogHeaderProgress(test.numHeaders, test.forceLog, progressFn)

		want := &Logger{
			subsystemLogger: logger.subsystemLogger,
			progressAction:  logger.progressAction,
			lastLogTime:     logger.lastLogTime,
			receivedHeaders: test.wantReceivedHeaders,
		}
		if !reflect.DeepEqual(logger, want) {
			t.Errorf("%s:\nwant: %+v\ngot: %+v\n", test.name, want, logger)
		}
	}
}
