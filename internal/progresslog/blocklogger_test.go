// Copyright (c) 2020 The Decred developers
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

// TestLogBlockHeight ensures the logging functionality works as expected via
// a test logger.
func TestLogBlockHeight(t *testing.T) {
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
		name                       string
		reset                      bool
		inputBlock                 *wire.MsgBlock
		inputSyncHeight            int64
		inputLastLogTime           time.Time
		wantReceivedLogBlocks      int64
		wantReceivedLogTx          int64
		wantReceivedLogVotes       int64
		wantReceivedLogRevocations int64
		wantReceivedLogTickets     int64
	}{{
		name:                       "round 1, block 0, last log time < 10 secs ago, < sync height",
		inputBlock:                 &testBlocks[0],
		inputSyncHeight:            100002,
		inputLastLogTime:           time.Now(),
		wantReceivedLogBlocks:      1,
		wantReceivedLogTx:          4,
		wantReceivedLogVotes:       2,
		wantReceivedLogRevocations: 1,
		wantReceivedLogTickets:     4,
	}, {
		name:                       "round 1, block 1, last log time < 10 secs ago, < sync height",
		inputBlock:                 &testBlocks[1],
		inputSyncHeight:            100002,
		inputLastLogTime:           time.Now(),
		wantReceivedLogBlocks:      2,
		wantReceivedLogTx:          6,
		wantReceivedLogVotes:       5,
		wantReceivedLogRevocations: 3,
		wantReceivedLogTickets:     6,
	}, {
		name:                       "round 1, block 2, last log time < 10 secs ago, < sync height",
		inputBlock:                 &testBlocks[2],
		inputSyncHeight:            100002,
		inputLastLogTime:           time.Now(),
		wantReceivedLogBlocks:      0,
		wantReceivedLogTx:          0,
		wantReceivedLogVotes:       0,
		wantReceivedLogRevocations: 0,
		wantReceivedLogTickets:     0,
	}, {
		name:                       "round 2, block 0, last log time < 10 secs ago, < sync height",
		reset:                      true,
		inputBlock:                 &testBlocks[0],
		inputSyncHeight:            100002,
		inputLastLogTime:           time.Now(),
		wantReceivedLogBlocks:      1,
		wantReceivedLogTx:          4,
		wantReceivedLogVotes:       2,
		wantReceivedLogRevocations: 1,
		wantReceivedLogTickets:     4,
	}, {
		name:                       "round 2, block 1, last log time > 10 secs ago, < sync height",
		inputBlock:                 &testBlocks[1],
		inputSyncHeight:            100002,
		inputLastLogTime:           time.Now().Add(-11 * time.Second),
		wantReceivedLogBlocks:      0,
		wantReceivedLogTx:          0,
		wantReceivedLogVotes:       0,
		wantReceivedLogRevocations: 0,
		wantReceivedLogTickets:     0,
	}, {
		name:                       "round 2, block 2, last log time > 10 secs ago, == sync height",
		inputBlock:                 &testBlocks[2],
		inputSyncHeight:            100002,
		inputLastLogTime:           time.Now().Add(-11 * time.Second),
		wantReceivedLogBlocks:      0,
		wantReceivedLogTx:          0,
		wantReceivedLogVotes:       0,
		wantReceivedLogRevocations: 0,
		wantReceivedLogTickets:     0,
	}}

	progressLogger := New("Wrote", testLog)
	for _, test := range tests {
		if test.reset {
			progressLogger = New("Wrote", testLog)
		}
		progressLogger.SetLastLogTime(test.inputLastLogTime)
		progressLogger.LogBlockHeight(test.inputBlock, test.inputSyncHeight)
		wantBlockProgressLogger := &BlockLogger{
			receivedLogBlocks:      test.wantReceivedLogBlocks,
			receivedLogTx:          test.wantReceivedLogTx,
			receivedLogVotes:       test.wantReceivedLogVotes,
			receivedLogRevocations: test.wantReceivedLogRevocations,
			receivedLogTickets:     test.wantReceivedLogTickets,
			lastBlockLogTime:       progressLogger.lastBlockLogTime,
			progressAction:         progressLogger.progressAction,
			subsystemLogger:        progressLogger.subsystemLogger,
		}
		if !reflect.DeepEqual(progressLogger, wantBlockProgressLogger) {
			t.Errorf("%s:\nwant: %+v\ngot: %+v\n", test.name,
				wantBlockProgressLogger, progressLogger)
		}
	}
}
