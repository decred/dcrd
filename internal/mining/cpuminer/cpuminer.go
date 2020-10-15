// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package cpuminer

import (
	"context"
	"encoding/binary"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/blockchain/v3"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/internal/mining"
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

	// maxSimnetToMine is the maximum number of blocks mined on HEAD~1 for
	// simnet that fail to submit to avoid pointlessly mining blocks in
	// situations such as tickets running out during simulations.
	maxSimnetToMine uint8 = 4
)

var (
	// MaxNumWorkers is the maximum number of workers that will be allowed for
	// mining and is based on the number of processor cores.  This helps ensure
	// system stays reasonably responsive under heavy load.
	MaxNumWorkers = uint32(runtime.NumCPU() * 2)

	// defaultNumWorkers is the default number of workers to use for mining.
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

// Config is a descriptor containing the CPU miner configuration.
type Config struct {
	// ChainParams identifies which chain parameters the CPU miner is
	// associated with.
	ChainParams *chaincfg.Params

	// PermitConnectionlessMining allows single node mining.
	PermitConnectionlessMining bool

	// BgBlkTmplGenerator identifies the instance to use in order to
	// generate block templates that the miner will attempt to solve.
	BgBlkTmplGenerator *mining.BgBlkTmplGenerator

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

// CPUMiner provides facilities for solving blocks (mining) using the CPU in a
// concurrency-safe manner.  It consists of two main modes -- a normal mining
// mode that tries to solve blocks continuously and a discrete mining mode,
// which is accessible via GenerateNBlocks, that generates a specific number of
// blocks that extend the main chain.
//
// The normal mining mode consists of two main goroutines -- a speed monitor and
// a controller for additional worker goroutines that generate and solve blocks.
//
// When the CPU miner is first started via the Run method, it will not have any
// workers which means it will be idle.  The number of worker goroutines for the
// normal mining mode can be set via the SetNumWorkers method.
type CPUMiner struct {
	numWorkers uint32 // update atomically

	sync.Mutex
	g                 *mining.BgBlkTmplGenerator
	cfg               *Config
	normalMining      bool
	discreteMining    bool
	discretePrevHash  chainhash.Hash
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
func (m *CPUMiner) speedMonitor(ctx context.Context) {
	log.Trace("CPU miner speed monitor started")

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
				log.Debugf("Hash speed: %6.0f kilohashes/s", hashesPerSec/1000)
			}

		// Request for the number of hashes per second.
		case m.queryHashesPerSec <- hashesPerSec:
			// Nothing to do.

		case <-ctx.Done():
			break out
		}
	}

	m.wg.Done()
	log.Trace("CPU miner speed monitor done")
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
			log.Errorf("Unexpected error while processing block submitted via "+
				"CPU miner: %v", err)
			return false
		}

		// When the reduce min difficulty option is set it is possible that the
		// required difficulty changed while a block was being solved and will
		// therefore result in an error due to having the incorrect required
		// difficulty set in the header.  In that case, only log the error as
		// debug since it is expected to happen from time to time and not really
		// an error.
		if m.cfg.ChainParams.ReduceMinDifficulty &&
			errors.Is(rErr, blockchain.ErrHighHash) {
			log.Debugf("Block submitted via CPU miner rejected because of "+
				"ReduceMinDifficulty time sync failure: %v", err)
			return false
		}

		// Other rule errors should be reported.
		log.Errorf("Block submitted via CPU miner rejected: %v", err)
		return false
	}
	if isOrphan {
		log.Errorf("Block submitted via CPU miner is an orphan building on "+
			"parent %v", block.MsgBlock().Header.PrevBlock)
		return false
	}

	// The block was accepted.
	coinbaseTxOuts := block.MsgBlock().Transactions[0].TxOut
	coinbaseTxGenerated := int64(0)
	for _, out := range coinbaseTxOuts {
		coinbaseTxGenerated += out.Value
	}
	log.Infof("Block submitted via CPU miner accepted (hash %s, height %v, "+
		"amount %v)", block.Hash(), block.Height(),
		dcrutil.Amount(coinbaseTxGenerated))
	return true
}

// solveBlock attempts to find some combination of a nonce, extra nonce, and
// current timestamp which makes the passed block header hash to a value less
// than the target difficulty.  The timestamp is updated periodically and the
// passed block header is modified with all tweaks during this process.  This
// means that when the function returns true, the block is ready for submission.
//
// This function will return early with false when the provided context is
// cancelled or an unexpected error happens.
func (m *CPUMiner) solveBlock(ctx context.Context, header *wire.BlockHeader, stats *speedStats, ticker *time.Ticker) bool {
	// Choose a random extra nonce offset for this block template and
	// worker.
	enOffset, err := wire.RandomUint64()
	if err != nil {
		log.Errorf("Unexpected error while generating random extra nonce "+
			"offset: %v", err)
		enOffset = 0
	}

	// Create some convenience variables.
	targetDifficulty := standalone.CompactToBig(header.Bits)

	// Initial state.
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

// solver is a worker that is controlled by a given generateBlocks goroutine.
//
// It attempts to solve the provided block template and submit the resulting
// solved block.  It also contains some additional logic to handle various
// corner cases such as waiting for connections when connectionless mining is
// disabled and exiting if too many failed blocks building on the same parent
// are mined when on the simulation network.
//
// It must be run as a goroutine.
func (m *CPUMiner) solver(ctx context.Context, template *mining.BlockTemplate, ticker *time.Ticker) {
	defer m.workerWg.Done()

	for {
		if ctx.Err() != nil {
			return
		}

		// Wait until there is a connection to at least one other peer when not
		// in connectionless mode since there is no way to relay a found block
		// or receive transactions to work on when there are no connected peers.
		prevBlock := template.Block.Header.PrevBlock
		for !m.cfg.PermitConnectionlessMining && m.cfg.ConnectedCount() == 0 {
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return
			}
		}

		// Don't try to mine any more blocks when in connectionless mode and the
		// maximum number of alternatives building on the current parent that
		// fail to submit has been reached.  This avoids pointlessly mining
		// blocks in situations such as tickets running out during simulations.
		if m.cfg.PermitConnectionlessMining {
			m.Lock()
			maxBlocksOnParent := m.minedOnParents[prevBlock] >= maxSimnetToMine
			m.Unlock()
			if maxBlocksOnParent {
				log.Infof("too many blocks mined on parent, stopping until " +
					"there are enough votes on these to make a new block")
				return
			}
		}

		// Attempt to solve the block.
		//
		// The function will exit with false if the block was not solved for any
		// reason such as the context being cancelled or an unexpected error, so
		// allow it to loop around to potentially try again in that case.
		//
		// When the return is true, a solution was found, so attempt to submit
		// the solved block and return from the worker if successful since it is
		// done.  In the case the solved block fails to submit, keep track of
		// how many blocks have failed to submit for its parent and try to find
		// another solution.
		//
		// The block in the template is shallow copied to avoid mutating the
		// data of the shared template.
		shallowBlockCopy := *template.Block
		if m.solveBlock(ctx, &shallowBlockCopy.Header, &m.speedStats, ticker) {
			block := dcrutil.NewBlock(&shallowBlockCopy)
			if !m.submitBlock(block) {
				m.Lock()
				m.minedOnParents[prevBlock]++
				m.Unlock()
				continue
			}

			return
		}
	}
}

// generateBlocks is a worker that is controlled by the miningWorkerController.
//
// It is self contained in that it registers for block template updates from the
// background block template generator and launches a goroutine that attempts to
// solve them while automatically switching to new templates as they become
// available.  When a block is solved, it is submitted.
//
// A separate goroutine for the solving is used to ensure template notifications
// can be serviced immediately without slowing down the main mining loop.
//
// It must be run as a goroutine.
func (m *CPUMiner) generateBlocks(ctx context.Context) {
	log.Trace("Starting generate blocks worker")
	defer func() {
		m.workerWg.Done()
		log.Trace("Generate blocks worker done")
	}()

	// Subscribe for block template updates and ensure the subscription is
	// stopped along with the worker.
	templateSub := m.g.Subscribe()
	defer templateSub.Stop()

	// Start a ticker which is used to signal updates to the speed monitor.
	ticker := time.NewTicker(time.Second * hashUpdateSecs)
	defer ticker.Stop()

	var solverCtx context.Context
	var solverCancel context.CancelFunc
	for {
		select {
		case templateNtfn := <-templateSub.C():
			// Clean up the map that tracks the number of blocks mined on a
			// given parent whenever a template is received due to a new parent.
			if m.cfg.PermitConnectionlessMining {
				if templateNtfn.Reason == mining.TURNewParent {
					prevHash := templateNtfn.Template.Block.Header.PrevBlock
					m.Lock()
					for k := range m.minedOnParents {
						if k != prevHash {
							delete(m.minedOnParents, k)
						}
					}
					m.Unlock()
				}
			}

			// Ensure the previous solver goroutine (if any) is stopped and
			// start another one for the new template.
			if solverCancel != nil {
				solverCancel()
			}
			solverCtx, solverCancel = context.WithCancel(ctx)
			m.workerWg.Add(1)
			go m.solver(solverCtx, templateNtfn.Template, ticker)

		case <-ctx.Done():
			// Ensure resources associated with the solver goroutine context are
			// freed as needed.
			if solverCancel != nil {
				solverCancel()
			}

			return
		}
	}
}

// miningWorkerController launches the worker goroutines that are used to
// subscribe for template updates and solve them.  It also provides the ability
// to dynamically adjust the number of running worker goroutines.
//
// It must be run as a goroutine.
func (m *CPUMiner) miningWorkerController(ctx context.Context) {
	// launchWorker groups common code to launch a worker for subscribing for
	// template updates and solving blocks.
	type workerState struct {
		cancel context.CancelFunc
	}
	var runningWorkers []workerState
	launchWorker := func() {
		wCtx, wCancel := context.WithCancel(ctx)
		runningWorkers = append(runningWorkers, workerState{
			cancel: wCancel,
		})

		m.workerWg.Add(1)
		go m.generateBlocks(wCtx)
	}

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
				numToLaunch := numWorkers - numRunning
				for i := uint32(0); i < numToLaunch; i++ {
					launchWorker()
				}
				log.Debugf("Launched %d %s (%d total running)", numToLaunch,
					pickNoun(uint64(numToLaunch), "worker", "workers"),
					numWorkers)
				continue
			}

			// Signal the most recently created goroutines to exit.
			numToStop := numRunning - numWorkers
			for i := uint32(0); i < numToStop; i++ {
				finalWorkerIdx := numRunning - 1 - i
				runningWorkers[finalWorkerIdx].cancel()
				runningWorkers[finalWorkerIdx].cancel = nil
				runningWorkers = runningWorkers[:finalWorkerIdx]
			}
			log.Debugf("Stopped %d %s (%d total running)", numToStop,
				pickNoun(uint64(numToStop), "worker", "workers"), numWorkers)

		case <-ctx.Done():
			// Signal all of the workers to shut down.
			for _, state := range runningWorkers {
				state.cancel()
			}
			break out
		}
	}

	// Wait until all workers shut down.
	m.workerWg.Wait()
	m.wg.Done()
}

// Run starts the CPU miner with zero workers which means it will be idle. It
// blocks until the provided context is cancelled.
//
// Use the SetNumWorkers method to start solving blocks in the normal mining
// mode.
func (m *CPUMiner) Run(ctx context.Context) {
	log.Trace("Starting CPU miner in idle state")

	m.wg.Add(3)
	go m.speedMonitor(ctx)
	go m.miningWorkerController(ctx)
	go func(ctx context.Context) {
		<-ctx.Done()
		close(m.quit)
		m.wg.Done()
	}(ctx)

	m.wg.Wait()
	log.Trace("CPU miner stopped")
}

// IsMining returns whether or not the CPU miner is currently mining in either
// the normal or discrete mining modes.
//
// This function is safe for concurrent access.
func (m *CPUMiner) IsMining() bool {
	m.Lock()
	defer m.Unlock()

	return m.normalMining || m.discreteMining
}

// HashesPerSecond returns the number of hashes per second the normal mode
// mining process is performing.  0 is returned if the miner is not currently
// mining anything in normal mining mode.
//
// This function is safe for concurrent access.
func (m *CPUMiner) HashesPerSecond() float64 {
	m.Lock()
	defer m.Unlock()

	// Nothing to do if the miner is not currently mining anything.
	if !m.normalMining {
		return 0
	}

	var hashesPerSec float64
	select {
	case hps := <-m.queryHashesPerSec:
		hashesPerSec = hps
	case <-m.quit:
	}

	return hashesPerSec
}

// SetNumWorkers sets the number of workers to create for solving blocks in the
// normal mining mode.  Negative values cause the default number of workers to
// be used, values larger than the max allowed are limited to the max, and a
// value of 0 causes all normal mode CPU mining to be stopped.
//
// NOTE: This will have no effect if discrete mining mode is currently active
// via GenerateNBlocks.
//
// This function is safe for concurrent access.
func (m *CPUMiner) SetNumWorkers(numWorkers int32) {
	m.Lock()
	defer m.Unlock()

	// Ignore when the miner is in discrete mode
	if m.discreteMining {
		return
	}

	// Use default number of workers if the provided value is negative or limit
	// it to the maximum allowed if needed.
	targetNumWorkers := uint32(numWorkers)
	if numWorkers < 0 {
		targetNumWorkers = defaultNumWorkers
	} else if targetNumWorkers > MaxNumWorkers {
		targetNumWorkers = MaxNumWorkers
	}
	atomic.StoreUint32(&m.numWorkers, targetNumWorkers)

	// Set the normal mining state accordingly.
	if targetNumWorkers != 0 {
		m.normalMining = true
	} else {
		m.normalMining = false
	}

	// Notify the controller about the the change.
	select {
	case m.updateNumWorkers <- struct{}{}:
	case <-m.quit:
	}
}

// NumWorkers returns the number of workers which are running to solve blocks
// in the normal mining mode.
//
// This function is safe for concurrent access.
func (m *CPUMiner) NumWorkers() int32 {
	return int32(atomic.LoadUint32(&m.numWorkers))
}

// GenerateNBlocks generates the requested number of blocks in the discrete
// mining mode and returns a list of the hashes of generated blocks that were
// added to the main chain.
//
// It makes use of a subscription to the background block template generator to
// obtain the templates and attempts to solve them while automatically switching
// to new templates as they become available as needed.  As a result, it
// supports many of the nice features of the template subscriptions such as
// giving all votes a chance to arrive.
//
// Note that, as the above implies, this will only consider blocks successfully
// added to the main chain in the overall count, so, upon returning, the list of
// hashes will only contain the hashes of those blocks.  This distinction is
// important because it is sometimes possible for a block to be rejected or be
// added to a side chain if it happens to be solved around the same time another
// one shows up.
func (m *CPUMiner) GenerateNBlocks(ctx context.Context, n uint32) ([]*chainhash.Hash, error) {
	// Nothing to do.
	if n == 0 {
		return nil, nil
	}

	// Respond with an error if server is already mining.
	m.Lock()
	if m.normalMining {
		m.Unlock()
		return nil, errors.New("server is already CPU mining -- please call " +
			"`setgenerate 0` before calling discrete `generate` commands")
	}
	if m.discreteMining {
		m.Unlock()
		return nil, errors.New("server is already discrete mining -- please " +
			"wait until the existing call completes or cancel it")
	}

	m.discreteMining = true
	m.Unlock()

	log.Tracef("Generating %d blocks", n)

	// Start a ticker which is used to signal checks for stale work.
	ticker := time.NewTicker(time.Second * hashUpdateSecs)
	defer ticker.Stop()

	templateSub := m.g.Subscribe()
	defer templateSub.Stop()

	blockHashes := make([]*chainhash.Hash, 0, n)
	var stats speedStats
out:
	for {
		// Wait for a new template update notification or early shutdown.
		var templateNtfn *mining.TemplateNtfn
		select {
		case <-ctx.Done():
			break out
		case <-m.quit:
			break out
		case templateNtfn = <-templateSub.C():
		}

		// Since callers might call this method in rapid succession and the
		// subscription immediately sends the current template, the template
		// might not have been updated yet (for example, it might be waiting on
		// votes).  In that case, wait for the updated template.
		m.Lock()
		discretePrev := m.discretePrevHash
		m.Unlock()
		if templateNtfn.Template.Block.Header.PrevBlock == discretePrev {
			continue
		}

		// Attempt to solve the block.
		//
		// The function will exit with false if the block was not solved for any
		// reason such as the context being cancelled or an unexpected error.
		//
		// When the return is true, a solution was found, so attempt to submit
		// the solved block.  Notice that this only records the hash of the
		// block if it was successfully submitted to the main chain.  This means
		// it is theoretically possible for more blocks than requested to be
		// mined if any of them fail to submit, but it results in better
		// behavior that lines up with what a caller would expect such that the
		// requested number of blocks are mined and added to the main chain.
		//
		// The block in the template is shallow copied to avoid mutating the
		// data of the shared template.
		shallowBlockCopy := *templateNtfn.Template.Block
		if m.solveBlock(ctx, &shallowBlockCopy.Header, &stats, ticker) {
			block := dcrutil.NewBlock(&shallowBlockCopy)
			if m.submitBlock(block) {
				m.Lock()
				m.discretePrevHash = shallowBlockCopy.Header.PrevBlock
				m.Unlock()
				blockHashes = append(blockHashes, block.Hash())
			}
		}

		// Done when the requested number of blocks are mined.
		if uint32(len(blockHashes)) == n {
			break out
		}
	}

	// Disable discrete mining mode and return the results.
	log.Tracef("Generated %d blocks", len(blockHashes))
	m.Lock()
	m.discreteMining = false
	m.Unlock()
	return blockHashes, nil
}

// New returns a new instance of a CPU miner for the provided configuration
// options.
//
// Use Run to initialize the CPU miner and then either use SetNumWorkers with a
// non-zero value to start the normal continuous mining mode or use
// GenerateNBlocks to mine a discrete number of blocks.
//
// See the documentation for CPUMiner type for more details.
func New(cfg *Config) *CPUMiner {
	return &CPUMiner{
		g:                 cfg.BgBlkTmplGenerator,
		cfg:               cfg,
		numWorkers:        defaultNumWorkers,
		updateNumWorkers:  make(chan struct{}),
		queryHashesPerSec: make(chan float64),
		minedOnParents:    make(map[chainhash.Hash]uint8),
		quit:              make(chan struct{}),
	}
}
