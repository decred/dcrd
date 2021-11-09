// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpctest

import (
	"bufio"
	"crypto/elliptic"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/decred/dcrd/certgen"
	rpc "github.com/decred/dcrd/rpcclient/v7"
)

// nodeConfig contains all the args, and data required to launch a dcrd process
// and connect the rpc client to it.
type nodeConfig struct {
	rpcUser    string
	rpcPass    string
	listen     string
	rpcListen  string
	rpcConnect string
	dataDir    string
	logDir     string
	profile    string
	debugLevel string
	extra      []string
	prefix     string

	pathToDCRD   string
	endpoint     string
	certFile     string
	keyFile      string
	certificates []byte
}

// newConfig returns a newConfig with all default values.
func newConfig(prefix, certFile, keyFile string, extra []string) (*nodeConfig, error) {
	a := &nodeConfig{
		listen:    "127.0.0.1:18555",
		rpcListen: "127.0.0.1:18556",
		rpcUser:   "user",
		rpcPass:   "pass",
		extra:     extra,
		prefix:    prefix,

		endpoint: "ws",
		certFile: certFile,
		keyFile:  keyFile,
	}
	if err := a.setDefaults(); err != nil {
		return nil, err
	}
	return a, nil
}

// setDefaults sets the default values of the config. It also creates the
// temporary data, and log directories which must be cleaned up with a call to
// cleanup().
func (n *nodeConfig) setDefaults() error {
	n.dataDir = filepath.Join(n.prefix, "data")
	n.logDir = filepath.Join(n.prefix, "logs")
	cert, err := os.ReadFile(n.certFile)
	if err != nil {
		return err
	}
	n.certificates = cert
	return nil
}

// arguments returns an array of arguments that be used to launch the dcrd
// process.
func (n *nodeConfig) arguments() []string {
	args := []string{}
	if n.rpcUser != "" {
		// --rpcuser
		args = append(args, fmt.Sprintf("--rpcuser=%s", n.rpcUser))
	}
	if n.rpcPass != "" {
		// --rpcpass
		args = append(args, fmt.Sprintf("--rpcpass=%s", n.rpcPass))
	}
	if n.listen != "" {
		// --listen
		args = append(args, fmt.Sprintf("--listen=%s", n.listen))
	}
	if n.rpcListen != "" {
		// --rpclisten
		args = append(args, fmt.Sprintf("--rpclisten=%s", n.rpcListen))
	}
	if n.rpcConnect != "" {
		// --rpcconnect
		args = append(args, fmt.Sprintf("--rpcconnect=%s", n.rpcConnect))
	}
	// --rpccert
	args = append(args, fmt.Sprintf("--rpccert=%s", n.certFile))
	// --rpckey
	args = append(args, fmt.Sprintf("--rpckey=%s", n.keyFile))
	// --txindex
	args = append(args, "--txindex")
	// --addrindex
	args = append(args, "--addrindex")
	if n.dataDir != "" {
		// --datadir
		args = append(args, fmt.Sprintf("--datadir=%s", n.dataDir))
	}
	if n.logDir != "" {
		// --logdir
		args = append(args, fmt.Sprintf("--logdir=%s", n.logDir))
	}
	if n.profile != "" {
		// --profile
		args = append(args, fmt.Sprintf("--profile=%s", n.profile))
	}
	if n.debugLevel != "" {
		// --debuglevel
		args = append(args, fmt.Sprintf("--debuglevel=%s", n.debugLevel))
	}
	// --allowunsyncedmining
	args = append(args, "--allowunsyncedmining")
	args = append(args, n.extra...)
	return args
}

// command returns the exec.Cmd which will be used to start the dcrd process.
func (n *nodeConfig) command() *exec.Cmd {
	return exec.Command(n.pathToDCRD, n.arguments()...)
}

// rpcConnConfig returns the rpc connection config that can be used to connect
// to the dcrd process that is launched via Start().
func (n *nodeConfig) rpcConnConfig() rpc.ConnConfig {
	return rpc.ConnConfig{
		Host:                 n.rpcListen,
		Endpoint:             n.endpoint,
		User:                 n.rpcUser,
		Pass:                 n.rpcPass,
		Certificates:         n.certificates,
		DisableAutoReconnect: true,
	}
}

// String returns the string representation of this nodeConfig.
func (n *nodeConfig) String() string {
	return n.prefix
}

// node houses the necessary state required to configure, launch, and manage a
// dcrd process.
type node struct {
	config *nodeConfig

	cmd     *exec.Cmd
	pidFile string
	stderr  io.ReadCloser
	stdout  io.ReadCloser
	wg      sync.WaitGroup
	pid     int

	dataDir string

	t *testing.T
}

// logf is identical to n.t.Logf but it prepends the pid of this  node.
func (n *node) logf(format string, args ...interface{}) {
	pid := strconv.Itoa(n.pid) + " "
	logf(n.t, pid+format, args...)
}

// tracef is identical to debug.go.tracef but it prepends the pid of this
// node.
func (n *node) tracef(format string, args ...interface{}) {
	if !trace {
		return
	}
	pid := strconv.Itoa(n.pid) + " "
	tracef(n.t, pid+format, args...)
}

// buildNode creates a new temporary directory and node and saves the location
// to a package level variable where it is used for all tests. pathToDCRDMtx
// must be held for writes.
func buildNode(t *testing.T) error {
	testNodeDir, err := os.MkdirTemp("", "rpctestdcrdnode")
	if err != nil {
		return err
	}
	pathToDCRD = filepath.Join(testNodeDir, "dcrd")
	if runtime.GOOS == "windows" {
		pathToDCRD += ".exe"
	}
	debugf(t, "test node located at: %v\n", pathToDCRD)
	// Determine import path of this package.
	_, rpctestDir, _, ok := runtime.Caller(1)
	if !ok {
		return fmt.Errorf("cannot get path to dcrd source code")
	}
	dcrdPkgPath := filepath.Join(rpctestDir, "..", "..")
	// Build dcrd and output an executable in a static temp path.
	cmd := exec.Command("go", "build", "-o", pathToDCRD, dcrdPkgPath)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to build dcrd: %v", err)
	}
	return nil
}

// newNode creates a new node instance according to the passed config. dataDir
// will be used to hold a file recording the pid of the launched process, and
// as the base for the log and data directories for dcrd. If pathToDCRD has a
// non-zero value, the executable located there is used.
func newNode(t *testing.T, config *nodeConfig, dataDir string) (*node, error) {
	// Create the dcrd node used for tests if not created yet.
	pathToDCRDMtx.Lock()
	if pathToDCRD == "" {
		if err := buildNode(t); err != nil {
			pathToDCRDMtx.Unlock()
			return nil, err
		}
	}
	config.pathToDCRD = pathToDCRD
	pathToDCRDMtx.Unlock()
	return &node{
		config:  config,
		dataDir: dataDir,
		cmd:     config.command(),
		t:       t,
	}, nil
}

// start creates a new dcrd process, and writes its pid in a file reserved for
// recording the pid of the launched process. This file can be used to
// terminate the process in case of a hang, or panic. In the case of a failing
// test case, or panic, it is important that the process be stopped via stop(),
// otherwise, it will persist unless explicitly killed.
func (n *node) start() error {
	var err error

	var pid sync.WaitGroup
	pid.Add(1)

	// Redirect stderr.
	n.stderr, err = n.cmd.StderrPipe()
	if err != nil {
		return err
	}
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		pid.Wait() // Block until pid is available
		r := bufio.NewReader(n.stderr)
		for {
			line, err := r.ReadBytes('\n')
			if errors.Is(err, io.EOF) {
				n.tracef("stderr: EOF")
				return
			}
			n.logf("stderr: %s", line)
		}
	}()

	// Redirect stdout.
	n.stdout, err = n.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		pid.Wait() // Block until pid is available
		r := bufio.NewReader(n.stdout)
		for {
			line, err := r.ReadBytes('\n')
			if errors.Is(err, io.EOF) {
				n.tracef("stdout: EOF")
				return
			}
			n.tracef("stdout: %s", line)
		}
	}()

	// Launch command and store pid.
	if err := n.cmd.Start(); err != nil {
		return err
	}
	n.pid = n.cmd.Process.Pid

	// Unblock pipes now pid is available
	pid.Done()

	f, err := os.Create(filepath.Join(n.config.String(), "dcrd.pid"))
	if err != nil {
		return err
	}

	n.pidFile = f.Name()
	if _, err = fmt.Fprintf(f, "%d\n", n.cmd.Process.Pid); err != nil {
		return err
	}

	return f.Close()
}

// stop interrupts the running dcrd process, and waits until it exits
// properly. On windows, interrupt is not supported, so a kill signal is used
// instead
func (n *node) stop() error {
	n.tracef("stop %p %p", n.cmd, n.cmd.Process)
	defer n.tracef("stop done")

	if n.cmd == nil || n.cmd.Process == nil {
		// return if not properly initialized
		// or error starting the process
		return nil
	}

	// Send kill command
	n.tracef("stop send kill")
	var err error
	if runtime.GOOS == "windows" {
		err = n.cmd.Process.Signal(os.Kill)
	} else {
		err = n.cmd.Process.Signal(os.Interrupt)
	}
	if err != nil {
		n.t.Logf("stop Signal error: %v", err)
	}

	// Wait for pipes.
	n.tracef("stop wg")
	n.wg.Wait()

	// Wait for command to exit.
	n.tracef("stop cmd.Wait")
	err = n.cmd.Wait()
	if err != nil {
		n.t.Logf("stop cmd.Wait error: %v", err)
	}
	return nil
}

// cleanup cleanups process and args files. The file housing the pid of the
// created process will be deleted, as well as any directories created by the
// process.
func (n *node) cleanup() error {
	n.tracef("cleanup")
	defer n.tracef("cleanup done")

	if n.pidFile != "" {
		if err := os.Remove(n.pidFile); err != nil {
			n.t.Logf("unable to remove file %s: %v", n.pidFile,
				err)
			return err
		}
	}

	return nil
}

// shutdown terminates the running dcrd process, and cleans up all
// file/directories created by node.
func (n *node) shutdown() error {
	n.tracef("shutdown")
	defer n.tracef("shutdown done")

	if err := n.stop(); err != nil {
		n.t.Logf("shutdown stop error: %v", err)
		return err
	}
	return n.cleanup()
}

// genCertPair generates a key/cert pair to the paths provided.
func genCertPair(certFile, keyFile string) error {
	org := "rpctest autogenerated cert"
	validUntil := time.Now().Add(10 * 365 * 24 * time.Hour)
	cert, key, err := certgen.NewTLSCertPair(elliptic.P521(), org,
		validUntil, nil)
	if err != nil {
		return err
	}

	// Write cert and key files.
	if err = os.WriteFile(certFile, cert, 0644); err != nil {
		return err
	}
	if err = os.WriteFile(keyFile, key, 0600); err != nil {
		os.Remove(certFile)
		return err
	}

	return nil
}
