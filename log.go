// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/decred/dcrd/addrmgr/v2"
	"github.com/decred/dcrd/blockchain/stake/v4"
	"github.com/decred/dcrd/blockchain/v4"
	"github.com/decred/dcrd/blockchain/v4/indexers"
	"github.com/decred/dcrd/connmgr/v3"
	"github.com/decred/dcrd/database/v3"
	"github.com/decred/dcrd/internal/fees"
	"github.com/decred/dcrd/internal/mempool"
	"github.com/decred/dcrd/internal/mining"
	"github.com/decred/dcrd/internal/mining/cpuminer"
	"github.com/decred/dcrd/internal/netsync"
	"github.com/decred/dcrd/internal/rpcserver"
	"github.com/decred/dcrd/peer/v3"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/slog"
	"github.com/jrick/logrotate/rotator"
)

// logWriter implements an io.Writer that outputs to both standard output and
// the write-end pipe of an initialized log rotator.
type logWriter struct{}

func (logWriter) Write(p []byte) (n int, err error) {
	os.Stdout.Write(p)
	if logRotator != nil {
		logRotator.Write(p)
	}
	return len(p), nil
}

// Loggers per subsystem.  A single backend logger is created and all subsystem
// loggers created from it will write to the backend.  When adding new
// subsystems, add the subsystem logger variable here and to the
// subsystemLoggers map.
//
// Loggers can not be used before the log rotator has been initialized with a
// log file.  This must be performed early during application startup by calling
// initLogRotator.
var (
	// backendLog is the logging backend used to create all subsystem loggers.
	// The backend must not be used before the log rotator has been initialized,
	// or data races and/or nil pointer dereferences will occur.
	backendLog = slog.NewBackend(logWriter{})

	// logRotator is one of the logging outputs.  It should be closed on
	// application shutdown.
	logRotator *rotator.Rotator

	adxrLog = backendLog.Logger("ADXR")
	amgrLog = backendLog.Logger("AMGR")
	bcdbLog = backendLog.Logger("BCDB")
	chanLog = backendLog.Logger("CHAN")
	cmgrLog = backendLog.Logger("CMGR")
	dcrdLog = backendLog.Logger("DCRD")
	discLog = backendLog.Logger("DISC")
	feesLog = backendLog.Logger("FEES")
	indxLog = backendLog.Logger("INDX")
	minrLog = backendLog.Logger("MINR")
	peerLog = backendLog.Logger("PEER")
	rpcsLog = backendLog.Logger("RPCS")
	scrpLog = backendLog.Logger("SCRP")
	srvrLog = backendLog.Logger("SRVR")
	stkeLog = backendLog.Logger("STKE")
	syncLog = backendLog.Logger("SYNC")
	txmpLog = backendLog.Logger("TXMP")
	trsyLog = backendLog.Logger("TRSY")
)

// Initialize package-global logger variables.
func init() {
	addrmgr.UseLogger(amgrLog)
	blockchain.UseLogger(chanLog)
	blockchain.UseTreasuryLogger(trsyLog)
	connmgr.UseLogger(cmgrLog)
	database.UseLogger(bcdbLog)
	fees.UseLogger(feesLog)
	indexers.UseLogger(indxLog)
	mempool.UseLogger(txmpLog)
	mining.UseLogger(minrLog)
	cpuminer.UseLogger(minrLog)
	peer.UseLogger(peerLog)
	rpcserver.UseLogger(rpcsLog)
	stake.UseLogger(stkeLog)
	netsync.UseLogger(syncLog)
	txscript.UseLogger(scrpLog)
}

// subsystemLoggers maps each subsystem identifier to its associated logger.
var subsystemLoggers = map[string]slog.Logger{
	"ADXR": adxrLog,
	"AMGR": amgrLog,
	"BCDB": bcdbLog,
	"CHAN": chanLog,
	"CMGR": cmgrLog,
	"DCRD": dcrdLog,
	"DISC": discLog,
	"FEES": feesLog,
	"INDX": indxLog,
	"MINR": minrLog,
	"PEER": peerLog,
	"RPCS": rpcsLog,
	"SCRP": scrpLog,
	"SRVR": srvrLog,
	"STKE": stkeLog,
	"SYNC": syncLog,
	"TXMP": txmpLog,
	"TRSY": trsyLog,
}

// initLogRotator initializes the logging rotator to write logs to logFile and
// create roll files in the same directory.  It must be called before the
// package-global log rotator variables are used.
func initLogRotator(logFile string) {
	logDir, _ := filepath.Split(logFile)
	err := os.MkdirAll(logDir, 0700)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create log directory: %v\n", err)
		os.Exit(1)
	}
	r, err := rotator.New(logFile, 10*1024, false, 3)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create file rotator: %v\n", err)
		os.Exit(1)
	}

	logRotator = r
}

// setLogLevel sets the logging level for provided subsystem.  Invalid
// subsystems are ignored.  Uninitialized subsystems are dynamically created as
// needed.
func setLogLevel(subsystemID string, logLevel string) {
	// Ignore invalid subsystems.
	logger, ok := subsystemLoggers[subsystemID]
	if !ok {
		return
	}

	// Defaults to info if the log level is invalid.
	level, _ := slog.LevelFromString(logLevel)
	logger.SetLevel(level)
}

// setLogLevels sets the log level for all subsystem loggers to the passed
// level.  It also dynamically creates the subsystem loggers as needed, so it
// can be used to initialize the logging system.
func setLogLevels(logLevel string) {
	// Configure all sub-systems with the new logging level.  Dynamically
	// create loggers as needed.
	for subsystemID := range subsystemLoggers {
		setLogLevel(subsystemID, logLevel)
	}
}

// directionString is a helper function that returns a string that represents
// the direction of a connection (inbound or outbound).
func directionString(inbound bool) string {
	if inbound {
		return "inbound"
	}
	return "outbound"
}

// pickNoun returns the singular or plural form of a noun depending
// on the count n.
func pickNoun(n uint64, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// fatalf logs a string, then cleanly exits.
func fatalf(str string) {
	dcrdLog.Errorf("Unable to create profiler: %v", str)
	os.Stdout.Sync()
	if logRotator != nil {
		logRotator.Close()
	}
	os.Exit(1)
}
