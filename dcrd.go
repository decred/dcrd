// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strings"

	"github.com/decred/dcrd/internal/blockchain"
	"github.com/decred/dcrd/internal/blockchain/indexers"
	"github.com/decred/dcrd/internal/limits"
	"github.com/decred/dcrd/internal/version"
)

var cfg *config

// winServiceMain is only invoked on Windows.  It detects when dcrd is running
// as a service and reacts accordingly.
var winServiceMain func() (bool, error)

// serviceStartOfDayChan is only used by Windows when the code is running as a
// service.  It signals the service code that startup has completed.  Notice
// that it uses a buffered channel so the caller will not be blocked when the
// service is not running.
var serviceStartOfDayChan = make(chan *config, 1)

// dcrdMain is the real main function for dcrd.  It is necessary to work around
// the fact that deferred functions do not run when os.Exit() is called.
func dcrdMain() error {
	// Load configuration and parse command line.  This function also
	// initializes logging and configures it accordingly.
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	tcfg, _, err := loadConfig(appName)
	if err != nil {
		usageMessage := fmt.Sprintf("Use %s -h to show usage", appName)
		fmt.Fprintln(os.Stderr, err)
		var e errSuppressUsage
		if !errors.As(err, &e) {
			fmt.Fprintln(os.Stderr, usageMessage)
		}
		return err
	}
	cfg = tcfg
	defer func() {
		if logRotator != nil {
			logRotator.Close()
		}
	}()

	// Get a context that will be canceled when a shutdown signal has been
	// triggered either from an OS signal such as SIGINT (Ctrl+C) or from
	// another subsystem such as the RPC server.
	ctx := shutdownListener()
	defer dcrdLog.Info("Shutdown complete")

	// Show version and home dir at startup.
	dcrdLog.Infof("Version %s (Go version %s %s/%s)", version.String(),
		runtime.Version(), runtime.GOOS, runtime.GOARCH)
	dcrdLog.Infof("Home dir: %s", cfg.HomeDir)
	if cfg.NoFileLogging {
		dcrdLog.Info("File logging disabled")
	}

	// Block and transaction processing can cause bursty allocations.  This
	// limits the garbage collector from excessively overallocating during
	// bursts.  It does this by tweaking the soft memory limit.
	//
	// A soft upper memory limit is imposed that leaves plenty of headroom for
	// the minimum recommended value and the target GC percentage is left at the
	// default value to significantly reduce the number of GC cycles thereby
	// reducing the amount of CPU time spent doing garbage collection.
	//
	// A limit of 1.8 GiB is used as a baseline, with an increase to the limit
	// when the UTXO cache max size has been increased over the default.
	//
	// In addition, the system administrator may override the limit to increase
	// it further by setting the GOMEMLIMIT environment variable.  In order to
	// prevent extremely poor performance due to a limit that is unreasonably
	// low, the environment variable will be ignored when it is configured with
	// a value less than the minimum required amount as determined per the
	// previous description.
	const memLimitBase = (18 * (1 << 30)) / 10 // 1.8 GiB
	softMemLimit := int64(memLimitBase)
	if cfg.UtxoCacheMaxSize > defaultUtxoCacheMaxSize {
		extra := int64(cfg.UtxoCacheMaxSize) - defaultUtxoCacheMaxSize
		softMemLimit += extra * (1 << 20)
	}
	goMemLimit := debug.SetMemoryLimit(-1)
	if goMemLimit < softMemLimit {
		dcrdLog.Infof("Ignoring GOMEMLIMIT=%q since it is too low and would "+
			"lead to extremely poor performance", humanizeBytes(goMemLimit))
	}
	if _, ok := os.LookupEnv("GOMEMLIMIT"); ok && goMemLimit > softMemLimit {
		softMemLimit = goMemLimit
	}
	debug.SetMemoryLimit(softMemLimit)
	dcrdLog.Infof("Soft memory limit: %s", humanizeBytes(softMemLimit))

	// Enable http profile server if requested.  Note that since the server may
	// be started now or dynamically started and stopped later, the stop call is
	// always deferred to ensure it is always stopped during process shutdown.
	var profiler profileServer
	defer profiler.Stop()
	if cfg.Profile != "" {
		const allowNonLoopback = true
		if err := profiler.Start(cfg.Profile, allowNonLoopback); err != nil {
			dcrdLog.Warnf("unable to start profile server: %v", err)
			return err
		}
	}

	// Write cpu profile if requested.
	if cfg.CPUProfile != "" {
		f, err := os.Create(cfg.CPUProfile)
		if err != nil {
			dcrdLog.Errorf("Unable to create cpu profile: %v", err.Error())
			return err
		}
		pprof.StartCPUProfile(f)
		defer f.Close()
		defer pprof.StopCPUProfile()
	}

	// Write mem profile if requested.
	if cfg.MemProfile != "" {
		f, err := os.Create(cfg.MemProfile)
		if err != nil {
			dcrdLog.Errorf("Unable to create mem profile: %v", err)
			return err
		}
		defer f.Close()
		defer pprof.WriteHeapProfile(f)
	}

	var lifetimeNotifier lifetimeEventServer
	if cfg.LifetimeEvents {
		lifetimeNotifier = newLifetimeEventServer(outgoingPipeMessages)
	}

	if cfg.PipeRx != 0 {
		go serviceControlPipeRx(uintptr(cfg.PipeRx))
	}
	if cfg.PipeTx != 0 {
		go serviceControlPipeTx(uintptr(cfg.PipeTx))
	} else {
		go drainOutgoingPipeMessages()
	}

	// Return now if a shutdown signal was triggered.
	if shutdownRequested(ctx) {
		return nil
	}

	// Load the block database.
	lifetimeNotifier.notifyStartupEvent(lifetimeEventDBOpen)
	db, err := loadBlockDB(cfg.params.Params)
	if err != nil {
		dcrdLog.Errorf("%v", err)
		return err
	}
	defer func() {
		// Ensure the database is sync'd and closed on shutdown.
		lifetimeNotifier.notifyShutdownEvent(lifetimeEventDBOpen)
		dcrdLog.Infof("Gracefully shutting down the block database...")
		db.Close()
	}()

	// Return now if a shutdown signal was triggered.
	if shutdownRequested(ctx) {
		return nil
	}

	// Load the UTXO database.
	utxoDb, err := blockchain.LoadUtxoDB(ctx, cfg.params.Params, cfg.DataDir)
	if err != nil {
		dcrdLog.Errorf("%v", err)
		return err
	}
	defer func() {
		// Ensure the database is sync'd and closed on shutdown.
		dcrdLog.Infof("Gracefully shutting down the UTXO database...")
		utxoDb.Close()
	}()

	// Return now if a shutdown signal was triggered.
	if shutdownRequested(ctx) {
		return nil
	}

	// Always drop the legacy address index if needed and drop any other indexes
	// and exit if requested.
	//
	// NOTE: The order is important here because dropping the tx index also
	// drops the address index since it relies on it.
	if err := indexers.DropAddrIndex(ctx, db); err != nil {
		dcrdLog.Errorf("%v", err)
		return err
	}
	if cfg.DropTxIndex {
		if err := indexers.DropTxIndex(ctx, db); err != nil {
			dcrdLog.Errorf("%v", err)
			return err
		}

		return nil
	}
	if cfg.DropExistsAddrIndex {
		if err := indexers.DropExistsAddrIndex(ctx, db); err != nil {
			dcrdLog.Errorf("%v", err)
			return err
		}

		return nil
	}

	// Drop the legacy v1 committed filter index if needed.
	if err := indexers.DropCfIndex(ctx, db); err != nil {
		dcrdLog.Errorf("%v", err)
		return err
	}

	// Create server.
	lifetimeNotifier.notifyStartupEvent(lifetimeEventP2PServer)
	svr, err := newServer(ctx, &profiler, cfg.Listeners, db, utxoDb,
		cfg.params.Params, cfg.DataDir)
	if err != nil {
		dcrdLog.Errorf("Unable to start server: %v", err)
		return err
	}

	if shutdownRequested(ctx) {
		return nil
	}

	lifetimeNotifier.notifyStartupComplete()
	defer lifetimeNotifier.notifyShutdownEvent(lifetimeEventP2PServer)

	// Signal the Windows service (if running) that startup has completed.
	serviceStartOfDayChan <- cfg

	// Run the server.  This will block until the context is cancelled which
	// happens when the interrupt signal is received from an OS signal or
	// shutdown is requested through one of the subsystems such as the RPC
	// server.
	svr.Run(ctx)
	srvrLog.Infof("Server shutdown complete")
	return nil
}

func main() {
	// Up some limits.
	if err := limits.SetLimits(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set limits: %v\n", err)
		os.Exit(1)
	}

	// Call serviceMain on Windows to handle running as a service.  When
	// the return isService flag is true, exit now since we ran as a
	// service.  Otherwise, just fall through to normal operation.
	if runtime.GOOS == "windows" {
		isService, err := winServiceMain()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if isService {
			os.Exit(0)
		}
	}

	// Work around defer not working after os.Exit()
	if err := dcrdMain(); err != nil {
		os.Exit(1)
	}
}
