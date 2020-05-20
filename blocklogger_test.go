// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/wire"
)

func TestLogBlockHeight(t *testing.T) {
	tests := []struct {
		name                       string
		reset                      bool
		inputBlock                 *dcrutil.Block
		inputSyncHeight            int64
		inputLastLogTime           time.Time
		wantReceivedLogBlocks      int64
		wantReceivedLogTx          int64
		wantReceivedLogVotes       int64
		wantReceivedLogRevocations int64
		wantReceivedLogTickets     int64
	}{{
		name:                       "TestLogBlockHeight with last log time < 10 seconds ago and sync height not reached",
		inputBlock:                 dcrutil.NewBlock(&TestBlock100000),
		inputSyncHeight:            100002,
		inputLastLogTime:           time.Now(),
		wantReceivedLogBlocks:      1,
		wantReceivedLogTx:          4,
		wantReceivedLogVotes:       2,
		wantReceivedLogRevocations: 1,
		wantReceivedLogTickets:     4,
	}, {
		name:                       "TestLogBlockHeight with last log time < 10 seconds ago and sync height not reached",
		inputBlock:                 dcrutil.NewBlock(&TestBlock100001),
		inputSyncHeight:            100002,
		inputLastLogTime:           time.Now(),
		wantReceivedLogBlocks:      2,
		wantReceivedLogTx:          6,
		wantReceivedLogVotes:       5,
		wantReceivedLogRevocations: 3,
		wantReceivedLogTickets:     6,
	}, {
		name:                       "TestLogBlockHeight with last log time < 10 seconds ago and sync height reached",
		inputBlock:                 dcrutil.NewBlock(&TestBlock100002),
		inputSyncHeight:            100002,
		inputLastLogTime:           time.Now(),
		wantReceivedLogBlocks:      0,
		wantReceivedLogTx:          0,
		wantReceivedLogVotes:       0,
		wantReceivedLogRevocations: 0,
		wantReceivedLogTickets:     0,
	}, {
		name:                       "TestLogBlockHeight with last log time < 10 seconds ago and sync height not reached",
		reset:                      true,
		inputBlock:                 dcrutil.NewBlock(&TestBlock100000),
		inputSyncHeight:            100002,
		inputLastLogTime:           time.Now(),
		wantReceivedLogBlocks:      1,
		wantReceivedLogTx:          4,
		wantReceivedLogVotes:       2,
		wantReceivedLogRevocations: 1,
		wantReceivedLogTickets:     4,
	}, {
		name:                       "TestLogBlockHeight with last log time > 10 seconds ago and sync height not reached",
		inputBlock:                 dcrutil.NewBlock(&TestBlock100001),
		inputSyncHeight:            100002,
		inputLastLogTime:           time.Now().Add(-11 * time.Second),
		wantReceivedLogBlocks:      0,
		wantReceivedLogTx:          0,
		wantReceivedLogVotes:       0,
		wantReceivedLogRevocations: 0,
		wantReceivedLogTickets:     0,
	}, {
		name:                       "TestLogBlockHeight with last log time > 10 seconds ago and sync height reached",
		inputBlock:                 dcrutil.NewBlock(&TestBlock100002),
		inputSyncHeight:            100002,
		inputLastLogTime:           time.Now().Add(-11 * time.Second),
		wantReceivedLogBlocks:      0,
		wantReceivedLogTx:          0,
		wantReceivedLogVotes:       0,
		wantReceivedLogRevocations: 0,
		wantReceivedLogTickets:     0,
	}}

	progressLogger := newBlockProgressLogger("Written", bmgrLog)
	for _, test := range tests {
		if test.reset {
			progressLogger = newBlockProgressLogger("Written", bmgrLog)
		}
		progressLogger.SetLastLogTime(test.inputLastLogTime)
		progressLogger.logBlockHeight(test.inputBlock, test.inputSyncHeight)
		wantBlockProgressLogger := &blockProgressLogger{
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

var TestBlock100000 = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:     1,
		Height:      100000,
		Timestamp:   time.Unix(1293623863, 0), // 2010-12-29 11:57:43 +0000 UTC
		Voters:      2,
		Revocations: 1,
		FreshStake:  4,
	},
	Transactions:  []*wire.MsgTx{{}, {}, {}, {}},
	STransactions: []*wire.MsgTx{},
}

var TestBlock100001 = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:     1,
		Height:      100001,
		Timestamp:   time.Unix(1293624163, 0), // 2010-12-29 12:02:43 +0000 UTC
		Voters:      3,
		Revocations: 2,
		FreshStake:  2,
	},
	Transactions:  []*wire.MsgTx{{}, {}},
	STransactions: []*wire.MsgTx{},
}

var TestBlock100002 = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:     1,
		Height:      100002,
		Timestamp:   time.Unix(1293624463, 0), // 2010-12-29 12:07:43 +0000 UTC
		Voters:      1,
		Revocations: 3,
		FreshStake:  1,
	},
	Transactions:  []*wire.MsgTx{{}, {}, {}},
	STransactions: []*wire.MsgTx{},
}
