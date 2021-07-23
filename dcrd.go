// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strings"

	"github.com/decred/dcrd/blockchain/v4"
	"github.com/decred/dcrd/blockchain/v4/indexers"
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

	// Enable http profiling server if requested.
	if cfg.Profile != "" {
		go func() {
			listenAddr := cfg.Profile
			dcrdLog.Infof("Creating profiling server "+
				"listening on %s", listenAddr)
			profileRedirect := http.RedirectHandler("/debug/pprof",
				http.StatusSeeOther)
			http.Handle("/", profileRedirect)
			err := http.ListenAndServe(listenAddr, nil)
			if err != nil {
				fatalf(err.Error())
			}
		}()
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

	// Drop indexes and exit if requested.
	//
	// NOTE: The order is important here because dropping the tx index also
	// drops the address index since it relies on it.
	if cfg.DropAddrIndex {
		if err := indexers.DropAddrIndex(ctx, db); err != nil {
			dcrdLog.Errorf("%v", err)
			return err
		}

		return nil
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
	svr, err := newServer(ctx, cfg.Listeners, db, utxoDb, cfg.params.Params,
		cfg.DataDir)
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
	// Use all processor cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Block and transaction processing can cause bursty allocations.  This
	// limits the garbage collector from excessively overallocating during
	// bursts.  This value was arrived at with the help of profiling live
	// usage.
	debug.SetGCPercent(20)

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
