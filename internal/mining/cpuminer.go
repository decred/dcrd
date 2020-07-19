// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/blockchain/v3"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/wire"
)

const (
	// maxNonce is the maximum value a nonce can be in a block header.
	maxNonce = ^uint32(0) // 2^32 - 1

	// hpsUpdateSecs is the number of seconds to wait in between each
	// update to the hashes per second monitor.
	hpsUpdateSecs = 10

	// hashUpdateSec is the number of seconds each worker waits in between
	// notifying the speed monitor with how many hashes have been completed
	// while they are actively searching for a solution.  This is done to
	// reduce the amount of syncs between the workers that must be done to
	// keep track of the hashes per second.
	hashUpdateSecs = 15

	// maxSimnetToMine is the maximum number of blocks to mine on HEAD~1
	// for simnet so that you don't run out of memory if tickets for
	// some reason run out during simulations.
	maxSimnetToMine uint8 = 4
)

var (
	// defaultNumWorkers is the default number of workers to use for mining
	// and is based on the number of processor cores.  This helps ensure the
	// system stays reasonably responsive under heavy load.
	defaultNumWorkers = uint32(1)

	// littleEndian is a convenience variable since binary.LittleEndian is
	// quite long.
	littleEndian = binary.LittleEndian
)

// speedStats houses tracking information used to monitor the hashing speed of
// the CPU miner.
type speedStats struct {
	sync.Mutex
	totalHashes uint64
}

// AddTotalHashes increments the total number of hashes by the provided number
// of hashes.  It is primarily intended for use by the individual worker
// goroutines to contribute to the overall total number of hashes done.
//
// This function is safe for concurrent access.
func (s *speedStats) AddTotalHashes(numHashes uint64) {
	s.Lock()
	s.totalHashes += numHashes
	s.Unlock()
}

// CPUMinerConfig is a descriptor containing the cpu miner configuration.
type CPUMinerConfig struct {
	// ChainParams identifies which chain parameters the cpu miner is
	// associated with.
	ChainParams *chaincfg.Params

	// PermitConnectionlessMining allows single node mining on simnet.
	PermitConnectionlessMining bool

	// BlockTemplateGenerator identifies the instance to use in order to
	// generate block templates that the miner will attempt to solve.
	BlockTemplateGenerator *BlkTmplGenerator

	// MiningAddrs is a list of payment addresses to use for the generated
	// blocks.  Each generated block will randomly choose one of them.
	MiningAddrs []dcrutil.Address

	// ProcessBlock defines the function to call with any solved blocks.
	// It typically must run the provided block through the same set of
	// rules and handling as any other block coming from the network.
	ProcessBlock func(*dcrutil.Block, blockchain.BehaviorFlags) (bool, error)

	// ConnectedCount defines the function to use to obtain how many other
	// peers the server is connected to.  This is used by the automatic
	// persistent mining routine to determine whether or it should attempt
	// mining.  This is useful because there is no point in mining when not
	// connected to any peers since there would no be anyone to send any
	// found blocks to.
	ConnectedCount func() int32

	// IsCurrent defines the function to use to obtain whether or not the
	// block chain is current.  This is used by the automatic persistent
	// mining routine to determine whether or it should attempt mining.
	// This is useful because there is no point in mining if the chain is
	// not current since any solved blocks would be on a side chain and
	// up orphaned anyways.
	IsCurrent func() bool
}

// CPUMiner provides facilities for solving blocks (mining) using the CPU in
// a concurrency-safe manner.  It consists of two main goroutines -- a speed
// monitor and a controller for worker goroutines which generate and solve
// blocks.  The number of goroutines can be set via the SetMaxGoRoutines
// function, but the default is based on the number of processor cores in the
// system which is typically sufficient.
type CPUMiner struct {
	numWorkers uint32 // update atomically

	sync.Mutex
	g                 *BlkTmplGenerator
	cfg               *CPUMinerConfig
	started           bool
	discreteMining    bool
	submitBlockLock   sync.Mutex
	wg                sync.WaitGroup
	workerWg          sync.WaitGroup
	updateNumWorkers  chan struct{}
	queryHashesPerSec chan float64
	speedStats        speedStats
	quit              chan struct{}

	// This is a map that keeps track of how many blocks have
	// been mined on each parent by the CPUMiner. It is only
	// for use in simulation networks, to diminish memory
	// exhaustion.
	minedOnParents map[chainhash.Hash]uint8
}

// speedMonitor handles tracking the number of hashes per second the mining
// process is performing.  It must be run as a goroutine.
func (m *CPUMiner) speedMonitor() {
	log.Tracef("CPU miner speed monitor started")

	var hashesPerSec float64
	ticker := time.NewTicker(time.Second * hpsUpdateSecs)
	defer ticker.Stop()

out:
	for {
		select {
		// Time to update the hashes per second.
		case <-ticker.C:
			stats := &m.speedStats
			stats.Lock()
			totalHashes := stats.totalHashes
			stats.totalHashes = 0
			stats.Unlock()
			curHashesPerSec := float64(totalHashes) / hpsUpdateSecs
			if hashesPerSec == 0 {
				hashesPerSec = curHashesPerSec
			}
			hashesPerSec = (hashesPerSec + curHashesPerSec) / 2
			if hashesPerSec != 0 {
				log.Debugf("Hash speed: %6.0f kilohashes/s",
					hashesPerSec/1000)
			}

		// Request for the number of hashes per second.
		case m.queryHashesPerSec <- hashesPerSec:
			// Nothing to do.

		case <-m.quit:
			break out
		}
	}

	m.wg.Done()
	log.Tracef("CPU miner speed monitor done")
}

// submitBlock submits the passed block to network after ensuring it passes all
// of the consensus validation rules.
func (m *CPUMiner) submitBlock(block *dcrutil.Block) bool {
	m.submitBlockLock.Lock()
	defer m.submitBlockLock.Unlock()

	// Process this block using the same rules as blocks coming from other
	// nodes. This will in turn relay it to the network like normal.
	isOrphan, err := m.cfg.ProcessBlock(block, blockchain.BFNone)
	if err != nil {
		// Anything other than a rule violation is an unexpected error,
		// so log that error as an internal error.
		var rErr blockchain.RuleError
		if !errors.As(err, &rErr) {
			log.Errorf("Unexpected error while processing "+
				"block submitted via CPU miner: %v", err)
			return false
		}
		// Occasionally errors are given out for timing errors with
		// ReduceMinDifficulty and high block works that is above
		// the target. Feed these to debug.
		if m.cfg.ChainParams.ReduceMinDifficulty &&
			rErr.ErrorCode == blockchain.ErrHighHash {
			log.Debugf("Block submitted via CPU miner rejected "+
				"because of ReduceMinDifficulty time sync failure: %v",
				err)
			return false
		}
		// Other rule errors should be reported.
		log.Errorf("Block submitted via CPU miner rejected: %v", err)
		return false
	}
	if isOrphan {
		log.Errorf("Block submitted via CPU miner is an orphan building "+
			"on parent %v", block.MsgBlock().Header.PrevBlock)
		return false
	}

	// The block was accepted.
	coinbaseTxOuts := block.MsgBlock().Transactions[0].TxOut
	coinbaseTxGenerated := int64(0)
	for _, out := range coinbaseTxOuts {
		coinbaseTxGenerated += out.Value
	}
	log.Infof("Block submitted via CPU miner accepted (hash %s, "+
		"height %v, amount %v)", block.Hash(), block.Height(),
		dcrutil.Amount(coinbaseTxGenerated))
	return true
}

// solveBlock attempts to find some combination of a nonce, extra nonce, and
// current timestamp which makes the passed block hash to a value less than the
// target difficulty.  The timestamp is updated periodically and the passed
// block is modified with all tweaks during this process.  This means that
// when the function returns true, the block is ready for submission.
//
// This function will return early with false when conditions that trigger a
// stale block such as a new block showing up or periodically when there are
// new transactions and enough time has elapsed without finding a solution.
func (m *CPUMiner) solveBlock(ctx context.Context, msgBlock *wire.MsgBlock, stats *speedStats, ticker *time.Ticker) bool {
	// Choose a random extra nonce offset for this block template and
	// worker.
	enOffset, err := wire.RandomUint64()
	if err != nil {
		log.Errorf("Unexpected error while generating random "+
			"extra nonce offset: %v", err)
		enOffset = 0
	}

	// Create a couple of convenience variables.
	header := &msgBlock.Header
	targetDifficulty := standalone.CompactToBig(header.Bits)

	// Initial state.
	lastGenerated := time.Now()
	lastTxUpdate := m.g.txSource.LastUpdated()
	hashesCompleted := uint64(0)

	// Note that the entire extra nonce range is iterated and the offset is
	// added relying on the fact that overflow will wrap around 0 as
	// provided by the Go spec.  Furthermore, the break condition has been
	// intentionally omitted such that the loop will continue forever until
	// a solution is found.
	for extraNonce := uint64(0); ; extraNonce++ {
		// Update the extra nonce in the block template header with the
		// new value.
		littleEndian.PutUint64(header.ExtraData[:], extraNonce+enOffset)

		// Search through the entire nonce range for a solution while
		// periodically checking for early quit and stale block
		// conditions along with updates to the speed monitor.
		//
		// This loop differs from the outer one in that it does not run
		// forever, thus allowing the extraNonce field to be updated
		// between each successive iteration of the regular nonce
		// space.  Note that this is achieved by placing the break
		// condition at the end of the code block, as this prevents the
		// infinite loop that would otherwise occur if we let the for
		// statement overflow the nonce value back to 0.
		for nonce := uint32(0); ; nonce++ {
			select {
			case <-ctx.Done():
				return false

			case <-ticker.C:
				stats.AddTotalHashes(hashesCompleted)
				hashesCompleted = 0

				// The current block is stale if the memory pool
				// has been updated since the block template was
				// generated and it has been at least 3 seconds,
				// or if it's been one minute.
				now := time.Now()
				if (lastTxUpdate != m.g.txSource.LastUpdated() &&
					now.After(lastGenerated.Add(3*time.Second))) ||
					now.After(lastGenerated.Add(60*time.Second)) {
					return false
				}

				err = m.g.UpdateBlockTime(header)
				if err != nil {
					log.Warnf("CPU miner unable to update block template "+
						"time: %v", err)
					return false
				}

			default:
				// Non-blocking select to fall through
			}

			// Update the nonce and hash the block header.
			header.Nonce = nonce
			hash := header.BlockHash()
			hashesCompleted++

			// The block is solved when the new block hash is less
			// than the target difficulty.  Yay!
			if standalone.HashToBig(&hash).Cmp(targetDifficulty) <= 0 {
				stats.AddTotalHashes(hashesCompleted)
				return true
			}

			if nonce == maxNonce {
				break
			}
		}
	}
}

// generateBlocks is a worker that is controlled by the miningWorkerController.
// It is self contained in that it creates block templates and attempts to solve
// them while detecting when it is performing stale work and reacting
// accordingly by generating a new block template.  When a block is solved, it
// is submitted.
//
// It must be run as a goroutine.
func (m *CPUMiner) generateBlocks(ctx context.Context) {
	log.Tracef("Starting generate blocks worker")

	// Start a ticker which is used to signal checks for stale work and
	// updates to the speed monitor.
	ticker := time.NewTicker(333 * time.Millisecond)
	defer ticker.Stop()

	for {
		// Quit when the miner is stopped.
		if ctx.Err() != nil {
			break
		}

		// If not in connectionless mode, wait until there is a connection to
		// at least one other peer since there is no way to relay a found
		// block or receive transactions to work on when there are no
		// connected peers.
		if !m.cfg.PermitConnectionlessMining && m.cfg.ConnectedCount() == 0 {
			time.Sleep(time.Second)
			continue
		}

		// No point in searching for a solution before the chain is
		// synced.  Also, grab the same lock as used for block
		// submission, since the current block will be changing and
		// this would otherwise end up building a new block template on
		// a block that is in the process of becoming stale.
		m.submitBlockLock.Lock()
		curHeight := m.g.chain.BestSnapshot().Height
		if curHeight != 0 && !m.cfg.IsCurrent() {
			m.submitBlockLock.Unlock()
			time.Sleep(time.Second)
			continue
		}

		// Choose a payment address at random.
		rand.Seed(time.Now().UnixNano())
		payToAddr := m.cfg.MiningAddrs[rand.Intn(len(m.cfg.MiningAddrs))]

		// Create a new block template using the available transactions
		// in the memory pool as a source of transactions to potentially
		// include in the block.
		template, err := m.g.NewBlockTemplate(payToAddr)
		m.submitBlockLock.Unlock()
		if err != nil {
			errStr := fmt.Sprintf("Failed to create new block "+
				"template: %v", err)
			log.Errorf(errStr)
			continue
		}

		// Not enough voters.
		if template == nil {
			continue
		}

		// This prevents you from causing memory exhaustion issues
		// when mining aggressively in a simulation network.
		if m.cfg.PermitConnectionlessMining {
			prevBlock := template.Block.Header.PrevBlock
			m.Lock()
			maxBlocksOnParent := m.minedOnParents[prevBlock] >= maxSimnetToMine
			m.Unlock()
			if maxBlocksOnParent {
				log.Tracef("too many blocks mined on parent, stopping " +
					"until there are enough votes on these to make a new " +
					"block")
				continue
			}
		}

		// Attempt to solve the block.  The function will exit early
		// with false when conditions that trigger a stale block, so
		// a new block template can be generated.  When the return is
		// true a solution was found, so submit the solved block.
		if m.solveBlock(ctx, template.Block, &m.speedStats, ticker) {
			block := dcrutil.NewBlock(template.Block)
			m.submitBlock(block)

			m.Lock()
			m.minedOnParents[template.Block.Header.PrevBlock]++
			m.Unlock()
		}
	}

	m.workerWg.Done()
	log.Tracef("Generate blocks worker done")
}

// miningWorkerController launches the worker goroutines that are used to
// generate block templates and solve them.  It also provides the ability to
// dynamically adjust the number of running worker goroutines.
//
// It must be run as a goroutine.
func (m *CPUMiner) miningWorkerController(ctx context.Context) {
	// launchWorkers groups common code to launch a specified number of
	// workers for generating blocks.
	var runningWorkers []context.CancelFunc
	launchWorkers := func(numWorkers uint32) {
		for i := uint32(0); i < numWorkers; i++ {
			wCtx, wCancel := context.WithCancel(ctx)
			runningWorkers = append(runningWorkers, wCancel)

			m.workerWg.Add(1)
			go m.generateBlocks(wCtx)
		}
	}

	// Launch the current number of workers by default.
	numWorkers := atomic.LoadUint32(&m.numWorkers)
	runningWorkers = make([]context.CancelFunc, 0, numWorkers)
	launchWorkers(numWorkers)

out:
	for {
		select {
		// Update the number of running workers.
		case <-m.updateNumWorkers:
			numRunning := uint32(len(runningWorkers))
			numWorkers := atomic.LoadUint32(&m.numWorkers)

			// No change.
			if numWorkers == numRunning {
				continue
			}

			// Add new workers.
			if numWorkers > numRunning {
				launchWorkers(numWorkers - numRunning)
				continue
			}

			// Signal the most recently created goroutines to exit.
			for i := numRunning - 1; i >= numWorkers; i-- {
				runningWorkers[i]()
				runningWorkers[i] = nil
				runningWorkers = runningWorkers[:i]
			}

		case <-m.quit:
			for _, wCancel := range runningWorkers {
				wCancel()
			}
			break out
		}
	}

	// Wait until all workers shut down.
	m.workerWg.Wait()
	m.wg.Done()
}

// Start begins the CPU mining process as well as the speed monitor used to
// track hashing metrics.  Calling this function when the CPU miner has
// already been started will have no effect.
//
// This function is safe for concurrent access.
func (m *CPUMiner) Start() {
	m.Lock()
	defer m.Unlock()

	// Nothing to do if the miner is already running or if running in discrete
	// mode (using GenerateNBlocks).
	if m.started || m.discreteMining {
		return
	}

	m.quit = make(chan struct{})
	m.wg.Add(2)
	go m.speedMonitor()
	go m.miningWorkerController(context.TODO())

	m.started = true
	log.Infof("CPU miner started")
}

// Wait blocks until the WaitGroup counters added to by a call to
// Start() are zero.
func (m *CPUMiner) Wait() {
	m.wg.Wait()
}

// Stop gracefully stops the mining process by signalling all workers, and the
// speed monitor to quit.  Calling this function when the CPU miner has not
// already been started will have no effect.
//
// This function is safe for concurrent access.
func (m *CPUMiner) Stop() {
	m.Lock()
	defer m.Unlock()

	// Nothing to do if the miner is not currently running or if running in
	// discrete mode (using GenerateNBlocks).
	if !m.started || m.discreteMining {
		return
	}

	close(m.quit)
	m.wg.Wait()
	m.started = false
	log.Infof("CPU miner stopped")
}

// IsMining returns whether or not the CPU miner has been started and is
// therefore currently mining.
//
// This function is safe for concurrent access.
func (m *CPUMiner) IsMining() bool {
	m.Lock()
	defer m.Unlock()

	return m.started
}

// HashesPerSecond returns the number of hashes per second the mining process
// is performing.  0 is returned if the miner is not currently running.
//
// This function is safe for concurrent access.
func (m *CPUMiner) HashesPerSecond() float64 {
	m.Lock()
	defer m.Unlock()

	// Nothing to do if the miner is not currently running.
	if !m.started {
		return 0
	}

	return <-m.queryHashesPerSec
}

// SetNumWorkers sets the number of workers to create which solve blocks.  Any
// negative values will cause a default number of workers to be used which is
// based on the number of processor cores in the system.  A value of 0 will
// cause all CPU mining to be stopped.
//
// This function is safe for concurrent access.
func (m *CPUMiner) SetNumWorkers(numWorkers int32) {
	if numWorkers == 0 {
		m.Stop()
	}

	// Use default if provided value is negative.
	if numWorkers < 0 {
		atomic.StoreUint32(&m.numWorkers, defaultNumWorkers)
	} else {
		atomic.StoreUint32(&m.numWorkers, uint32(numWorkers))
	}

	// When the miner is already running, notify the controller about the
	// the change.
	if m.started {
		m.updateNumWorkers <- struct{}{}
	}
}

// NumWorkers returns the number of workers which are running to solve blocks.
//
// This function is safe for concurrent access.
func (m *CPUMiner) NumWorkers() int32 {
	return int32(atomic.LoadUint32(&m.numWorkers))
}

// GenerateNBlocks generates the requested number of blocks. It is self
// contained in that it creates block templates and attempts to solve them while
// detecting when it is performing stale work and reacting accordingly by
// generating a new block template.  When a block is solved, it is submitted.
// The function returns a list of the hashes of generated blocks.
func (m *CPUMiner) GenerateNBlocks(ctx context.Context, n uint32) ([]*chainhash.Hash, error) {
	// Respond with an error if server is already mining.
	m.Lock()
	if m.started || m.discreteMining {
		m.Unlock()
		return nil, errors.New("server is already CPU mining. Please call " +
			"`setgenerate 0` before calling discrete `generate` commands")
	}

	m.started = true
	m.discreteMining = true
	m.Unlock()

	log.Tracef("Generating %d blocks", n)

	i := uint32(0)
	blockHashes := make([]*chainhash.Hash, n)

	// Start a ticker which is used to signal checks for stale work.
	ticker := time.NewTicker(time.Second * hashUpdateSecs)
	defer ticker.Stop()

	for {
		// Read updateNumWorkers in case someone tries a `setgenerate` while
		// we're generating. We can ignore it as the `generate` RPC call only
		// uses 1 worker.
		select {
		case <-m.updateNumWorkers:
		default:
		}

		// Grab the lock used for block submission, since the current block will
		// be changing and this would otherwise end up building a new block
		// template on a block that is in the process of becoming stale.
		m.submitBlockLock.Lock()

		// Choose a payment address at random.
		rand.Seed(time.Now().UnixNano())
		payToAddr := m.cfg.MiningAddrs[rand.Intn(len(m.cfg.MiningAddrs))]

		// Create a new block template using the available transactions
		// in the memory pool as a source of transactions to potentially
		// include in the block.
		template, err := m.g.NewBlockTemplate(payToAddr)
		m.submitBlockLock.Unlock()
		if err != nil {
			errStr := fmt.Sprintf("Failed to create new block "+
				"template: %v", err)
			log.Errorf(errStr)
			continue
		}
		if template == nil {
			errStr := fmt.Sprintf("Not enough voters on parent block " +
				"and failed to pull parent template")
			log.Debugf(errStr)
			continue
		}

		// Attempt to solve the block.  The function will exit early
		// with false when conditions that trigger a stale block, so
		// a new block template can be generated.  When the return is
		// true a solution was found, so submit the solved block.
		var stats speedStats
		if m.solveBlock(ctx, template.Block, &stats, ticker) {
			block := dcrutil.NewBlock(template.Block)
			m.submitBlock(block)
			blockHashes[i] = block.Hash()
			i++
			if i == n {
				log.Tracef("Generated %d blocks", i)
				m.wg.Wait()
				m.Lock()
				m.started = false
				m.discreteMining = false
				m.Unlock()
				return blockHashes, nil
			}
		}
	}
}

// NewCPUMiner returns a new instance of a CPU miner for the provided server.
// Use Start to begin the mining process.  See the documentation for CPUMiner
// type for more details.
func NewCPUMiner(cfg *CPUMinerConfig) *CPUMiner {
	return &CPUMiner{
		g:                 cfg.BlockTemplateGenerator,
		cfg:               cfg,
		numWorkers:        defaultNumWorkers,
		updateNumWorkers:  make(chan struct{}),
		queryHashesPerSec: make(chan float64),
		minedOnParents:    make(map[chainhash.Hash]uint8),
	}
}
