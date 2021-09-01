// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpctest

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/rpcclient/v7"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

const (
	// These constants define the minimum and maximum p2p and rpc port
	// numbers used by a test harness.  The min port is inclusive while the
	// max port is exclusive.
	minPeerPort = 10000
	maxPeerPort = 35000
	minRPCPort  = maxPeerPort
	maxRPCPort  = 60000
)

var (
	// XXX these variables are accessed in what should be accessor
	// functions yet it is all global

	// current number of active test nodes.
	numTestInstances = 0

	// processID is the process ID of the current running process.  It is
	// used to calculate ports based upon it when launching an rpc
	// harnesses.  The intent is to allow multiple process to run in
	// parallel without port collisions.
	//
	// It should be noted however that there is still some small probability
	// that there will be port collisions either due to other processes
	// running or simply due to the stars aligning on the process IDs.
	processID = os.Getpid()

	// testInstances is a private package-level slice used to keep track of
	// all active test harnesses. This global can be used to perform
	// various "joins", shutdown several active harnesses after a test,
	// etc.
	testInstances = make(map[string]*Harness)

	// Used to protest concurrent access to above declared variables.
	harnessStateMtx sync.RWMutex

	// pathToDCRD points to the test node. It is supplied through
	// NewWithDCRD or created on the first call to newNode and used
	// throughout the life of this package.
	pathToDCRD    string
	pathToDCRDMtx sync.RWMutex
)

const (
	// BlockVersion is the default block version used when generating
	// blocks.
	BlockVersion = 3
)

// HarnessTestCase represents a test-case which utilizes an instance of the
// Harness to exercise functionality.
type HarnessTestCase func(ctx context.Context, r *Harness, t *testing.T)

// Harness fully encapsulates an active dcrd process to provide a unified
// platform for creating rpc driven integration tests involving dcrd. The
// active dcrd node will typically be run in simnet mode in order to allow for
// easy generation of test blockchains.  The active dcrd process is fully
// managed by Harness, which handles the necessary initialization, and teardown
// of the process along with any temporary directories created as a result.
// Multiple Harness instances may be run concurrently, in order to allow for
// testing complex scenarios involving multiple nodes. The harness also
// includes an in-memory wallet to streamline various classes of tests.
type Harness struct {
	// ActiveNet is the parameters of the blockchain the Harness belongs
	// to.
	ActiveNet *chaincfg.Params

	Node     *rpcclient.Client
	node     *node
	handlers *rpcclient.NotificationHandlers

	wallet *memWallet

	testNodeDir    string
	maxConnRetries int
	nodeNum        int

	t *testing.T

	sync.Mutex
}

// SetPathToDCRD sets the package level dcrd executable. All calls to New will
// use the dcrd located there throughout their life. If not set upon the first
// call to New, a dcrd will be created in a temporary directory and pathToDCRD
// set automatically.
//
// NOTE: This function is safe for concurrent access, but care must be taken
// when setting different paths and using New, as whatever is at pathToDCRD at
// the time will be identified with that node.
func SetPathToDCRD(fnScopePathToDCRD string) {
	pathToDCRDMtx.Lock()
	pathToDCRD = fnScopePathToDCRD
	pathToDCRDMtx.Unlock()
}

// New creates and initializes new instance of the rpc test harness.
// Optionally, websocket handlers and a specified configuration may be passed.
// In the case that a nil config is passed, a default configuration will be
// used. If pathToDCRD has not been set and working within the dcrd repository,
// a dcrd executable created from the directory at rpctest/../ (dcrd repo's
// root directory) will be created in a temporary directory. pathToDCRD will be
// set as that file's location. If pathToDCRD has already been set, the
// executable at that location will be used.
//
// NOTE: This function is safe for concurrent access, but care must be taken
// when calling New with different dcrd executables, as whatever is at
// pathToDCRD at the time will be identified with that node.
func New(t *testing.T, activeNet *chaincfg.Params, handlers *rpcclient.NotificationHandlers, extraArgs []string) (*Harness, error) {
	harnessStateMtx.Lock()
	defer harnessStateMtx.Unlock()

	// Add a flag for the appropriate network type based on the provided
	// chain params.
	switch activeNet.Net {
	case wire.MainNet:
		// No extra flags since mainnet is the default
	case wire.TestNet3:
		extraArgs = append(extraArgs, "--testnet")
	case wire.SimNet:
		extraArgs = append(extraArgs, "--simnet")
	case wire.RegNet:
		extraArgs = append(extraArgs, "--regnet")
	default:
		return nil, fmt.Errorf("rpctest.New must be called with one " +
			"of the supported chain networks")
	}

	harnessID := strconv.Itoa(numTestInstances)
	nodeTestData, err := os.MkdirTemp("", "rpctest-"+harnessID)
	if err != nil {
		return nil, err
	}
	debugf(t, "temp dir: %v\n", nodeTestData)

	certFile := filepath.Join(nodeTestData, "rpc.cert")
	keyFile := filepath.Join(nodeTestData, "rpc.key")
	if err := genCertPair(certFile, keyFile); err != nil {
		return nil, err
	}

	wallet, err := newMemWallet(t, activeNet, uint32(numTestInstances))
	if err != nil {
		return nil, err
	}

	miningAddr := fmt.Sprintf("--miningaddr=%s", wallet.coinbaseAddr)
	extraArgs = append(extraArgs, miningAddr)

	config, err := newConfig(nodeTestData, certFile, keyFile, extraArgs)
	if err != nil {
		return nil, err
	}

	// Uncomment and change to enable additional dcrd debug/trace output.
	//config.debugLevel = "TXMP=trace,TRSY=trace,RPCS=trace,PEER=trace"

	// Generate p2p+rpc listening addresses.
	config.listen, config.rpcListen = generateListeningAddresses()

	// Create the testing node bounded to the simnet.
	node, err := newNode(t, config, nodeTestData)
	if err != nil {
		return nil, err
	}
	nodeNum := numTestInstances
	numTestInstances++ // XXX this really should be the length of the harness map.

	if handlers == nil {
		handlers = &rpcclient.NotificationHandlers{}
	}

	// If a handler for the OnBlockConnected/OnBlockDisconnected callback
	// has already been set, then we create a wrapper callback which
	// executes both the currently registered callback, and the mem
	// wallet's callback.
	if handlers.OnBlockConnected != nil {
		obc := handlers.OnBlockConnected
		handlers.OnBlockConnected = func(header []byte, filteredTxns [][]byte) {
			wallet.IngestBlock(header, filteredTxns)
			obc(header, filteredTxns)
		}
	} else {
		// Otherwise, we can claim the callback ourselves.
		handlers.OnBlockConnected = wallet.IngestBlock
	}
	if handlers.OnBlockDisconnected != nil {
		obd := handlers.OnBlockDisconnected
		handlers.OnBlockDisconnected = func(header []byte) {
			wallet.UnwindBlock(header)
			obd(header)
		}
	} else {
		handlers.OnBlockDisconnected = wallet.UnwindBlock
	}

	h := &Harness{
		handlers:       handlers,
		node:           node,
		maxConnRetries: 20,
		testNodeDir:    nodeTestData,
		ActiveNet:      activeNet,
		nodeNum:        nodeNum,
		wallet:         wallet,
		t:              t,
	}

	// Track this newly created test instance within the package level
	// global map of all active test instances.
	testInstances[h.testNodeDir] = h

	return h, nil
}

// SetUp initializes the rpc test state. Initialization includes: starting up a
// simnet node, creating a websockets client and connecting to the started
// node, and finally: optionally generating and submitting a testchain with a
// configurable number of mature coinbase outputs coinbase outputs.
//
// NOTE: This method and TearDown should always be called from the same
// goroutine as they are not concurrent safe.
func (h *Harness) SetUp(createTestChain bool, numMatureOutputs uint32) error {
	// Start the dcrd node itself. This spawns a new process which will be
	// managed
	if err := h.node.start(); err != nil {
		return err
	}
	if err := h.connectRPCClient(); err != nil {
		return err
	}
	ctx := context.Background()
	h.wallet.Start()

	// Filter transactions that pay to the coinbase associated with the
	// wallet.
	filterAddrs := []stdaddr.Address{h.wallet.coinbaseAddr}
	if err := h.Node.LoadTxFilter(ctx, true, filterAddrs, nil); err != nil {
		return err
	}

	// Ensure dcrd properly dispatches our registered call-back for each new
	// block. Otherwise, the memWallet won't function properly.
	if err := h.Node.NotifyBlocks(ctx); err != nil {
		return err
	}

	tracef(h.t, "createTestChain %v numMatureOutputs %v", createTestChain,
		numMatureOutputs)
	// Create a test chain with the desired number of mature coinbase
	// outputs.
	if createTestChain && numMatureOutputs != 0 {
		// Include an extra block to account for the premine block.
		numToGenerate := (uint32(h.ActiveNet.CoinbaseMaturity) +
			numMatureOutputs) + 1
		tracef(h.t, "Generate: %v", numToGenerate)
		_, err := h.Node.Generate(ctx, numToGenerate)
		if err != nil {
			return err
		}
	}

	// Block until the wallet has fully synced up to the tip of the main
	// chain.
	_, height, err := h.Node.GetBestBlock(ctx)
	if err != nil {
		return err
	}
	tracef(h.t, "Best block height: %v", height)
	ticker := time.NewTicker(time.Millisecond * 100)
	for range ticker.C {
		walletHeight := h.wallet.SyncedHeight()
		if walletHeight == height {
			break
		}
	}
	tracef(h.t, "Synced: %v", height)

	return nil
}

// TearDown stops the running rpc test instance. All created processes are
// killed, and temporary directories removed.
//
// NOTE: This method and SetUp should always be called from the same goroutine
// as they are not concurrent safe.
func (h *Harness) TearDown() error {
	tracef(h.t, "TearDown %p %p", h.Node, h.node)
	defer tracef(h.t, "TearDown done")

	if h.Node != nil {
		tracef(h.t, "TearDown: Node")
		h.Node.Shutdown()
	}

	tracef(h.t, "TearDown: node")
	if err := h.node.shutdown(); err != nil {
		return err
	}

	if !(debug || trace) {
		if err := os.RemoveAll(h.testNodeDir); err != nil {
			return err
		}
	}

	tracef(h.t, "TearDown deleting %v", h.node.pid)
	delete(testInstances, h.testNodeDir)

	return nil
}

// connectRPCClient attempts to establish an RPC connection to the created dcrd
// process belonging to this Harness instance. If the initial connection
// attempt fails, this function will retry h.maxConnRetries times, backing off
// the time between subsequent attempts. If after h.maxConnRetries attempts,
// we're not able to establish a connection, this function returns with an
// error.
func (h *Harness) connectRPCClient() error {
	var client *rpcclient.Client
	var err error

	rpcConf := h.node.config.rpcConnConfig()
	for i := 0; i < h.maxConnRetries; i++ {
		if client, err = rpcclient.New(&rpcConf, h.handlers); err != nil {
			time.Sleep(time.Duration(i) * 50 * time.Millisecond)
			continue
		}
		break
	}

	if client == nil {
		return fmt.Errorf("connection timeout")
	}

	h.Node = client
	h.wallet.SetRPCClient(client)
	return nil
}

// NewAddress returns a fresh address spendable by the Harness' internal
// wallet.
//
// This function is safe for concurrent access.
func (h *Harness) NewAddress() (stdaddr.Address, error) {
	return h.wallet.NewAddress()
}

// ConfirmedBalance returns the confirmed balance of the Harness' internal
// wallet.
//
// This function is safe for concurrent access.
func (h *Harness) ConfirmedBalance() dcrutil.Amount {
	return h.wallet.ConfirmedBalance()
}

// SendOutputs creates, signs, and finally broadcasts a transaction spending
// the harness' available mature coinbase outputs creating new outputs
// according to targetOutputs.
//
// This function is safe for concurrent access.
func (h *Harness) SendOutputs(targetOutputs []*wire.TxOut, feeRate dcrutil.Amount) (*chainhash.Hash, error) {
	return h.wallet.SendOutputs(targetOutputs, feeRate)
}

// CreateTransaction returns a fully signed transaction paying to the specified
// outputs while observing the desired fee rate. The passed fee rate should be
// expressed in atoms-per-byte. Any unspent outputs selected as inputs for
// the crafted transaction are marked as unspendable in order to avoid
// potential double-spends by future calls to this method. If the created
// transaction is cancelled for any reason then the selected inputs MUST be
// freed via a call to UnlockOutputs. Otherwise, the locked inputs won't be
// returned to the pool of spendable outputs.
//
// This function is safe for concurrent access.
func (h *Harness) CreateTransaction(targetOutputs []*wire.TxOut, feeRate dcrutil.Amount) (*wire.MsgTx, error) {
	return h.wallet.CreateTransaction(targetOutputs, feeRate)
}

// UnlockOutputs unlocks any outputs which were previously marked as
// unspendable due to being selected to fund a transaction via the
// CreateTransaction method.
//
// This function is safe for concurrent access.
func (h *Harness) UnlockOutputs(inputs []*wire.TxIn) {
	h.wallet.UnlockOutputs(inputs)
}

// RPCConfig returns the harnesses current rpc configuration. This allows other
// potential RPC clients created within tests to connect to a given test
// harness instance.
func (h *Harness) RPCConfig() rpcclient.ConnConfig {
	return h.node.config.rpcConnConfig()
}

// P2PAddress returns the harness node's configured listening address for P2P
// connections.
//
// Note that to connect two different harnesses, it's preferable to use the
// ConnectNode() function, which handles cases like already connected peers and
// ensures the connection actually takes place.
func (h *Harness) P2PAddress() string {
	return h.node.config.listen
}

// generateListeningAddresses returns two strings representing listening
// addresses designated for the current rpc test. If there haven't been any
// test instances created, the default ports are used. Otherwise, in order to
// support multiple test nodes running at once, the p2p and rpc port are
// incremented after each initialization.
func generateListeningAddresses() (string, string) {
	localhost := "127.0.0.1"

	portString := func(minPort, maxPort int) string {
		port := minPort + numTestInstances + ((20 * processID) %
			(maxPort - minPort))
		return strconv.Itoa(port)
	}

	p2p := net.JoinHostPort(localhost, portString(minPeerPort, maxPeerPort))
	rpc := net.JoinHostPort(localhost, portString(minRPCPort, maxRPCPort))
	return p2p, rpc
}
