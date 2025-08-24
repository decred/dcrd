// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package cpuminer

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/crypto/rand"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/internal/blockchain"
	"github.com/decred/dcrd/internal/mining"
	"github.com/decred/dcrd/internal/staging/primitives"
	"github.com/decred/dcrd/wire"
	"lukechampine.com/blake3"
)

const (
	// maxNonce is the maximum value a nonce can be in a block header.
	maxNonce = ^uint32(0) // 2^32 - 1

	// hpsUpdateSecs is the number of seconds to wait in between each
	// update to the hashes per second monitor.
	hpsUpdateSecs = 10

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

var (
	// ErrCancelDiscreteMining indicates the discrete mining process was
	// cancelled before completing successfully.  For example, this can happen
	// when explicitly requested via calling [CPUMiner.GenerateNBlocks] with 0
	// or when the parent context is cancelled.
	ErrCancelDiscreteMining = errors.New("discrete mining process canceled")
)

// speedStats houses tracking information used to monitor the hashing speed of
// the CPU miner.
type speedStats struct {
	totalHashes   atomic.Uint64
	elapsedMicros atomic.Uint64
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

	// BestSnapshot defines the function to use to access information about the
	// current best block.  The returned instance should be treated as immutable.
	BestSnapshot func() *blockchain.BestState

	// BlockHashByHeight returns the hash of the block at the given height in
	// the main chain.
	BlockHashByHeight func(height int64) (*chainhash.Hash, error)

	// ProcessBlock defines the function to call with any solved blocks.
	// It typically must run the provided block through the same set of
	// rules and handling as any other block coming from the network.
	ProcessBlock func(*dcrutil.Block) error

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

	// IsBlake3PowAgendaActive returns whether or not the agenda to change the
	// proof of work hash function to blake3, as defined in DCP0011, has passed
	// and is now active for the block AFTER the given block.
	IsBlake3PowAgendaActive func(prevHash *chainhash.Hash) (bool, error)
}

// CPUMiner provides facilities for solving blocks (mining) using the CPU in a
// concurrency-safe manner.  It consists of two main modes -- a normal mining
// mode that tries to solve blocks continuously and a discrete mining mode,
// which is accessible via [CPUMiner.GenerateNBlocks], that mines blocks on an
// as needed basis to extend the main chain by a specific number of blocks.
//
// The normal mining mode consists of two main goroutines -- a speed monitor and
// a controller for additional worker goroutines that generate and solve blocks.
//
// When the CPU miner is first started via the Run method, it will not have any
// workers which means it will be idle.  The number of worker goroutines for the
// normal mining mode can be set via the SetNumWorkers method.
type CPUMiner struct {
	numWorkers atomic.Uint32

	sync.Mutex
	g                 *mining.BgBlkTmplGenerator
	cfg               *Config
	normalMining      bool
	discreteMining    bool
	submitBlockLock   sync.Mutex
	updateNumWorkers  chan struct{}
	queryHashesPerSec chan float64
	speedStats        map[uint64]*speedStats
	quit              chan struct{}

	// discretePrevTemplate is the template that was most recently mined by the
	// discrete mining process.  It is used to provide a better user experience
	// for the discrete mining process used in testing.
	discretePrevTemplate atomic.Pointer[mining.BlockTemplate]

	// This is a map that keeps track of how many blocks have been mined on each
	// parent by the CPUMiner. It is only for use in simulation networks, to
	// diminish memory exhaustion.
	//
	// It is protected by the embedded mutex.
	minedOnParents map[chainhash.Hash]uint8

	// generateCancelFn is set when discrete mining is active and may be invoked
	// to cancel the discrete mining process early.
	//
	// It is protected by the embedded mutex.
	generateCancelFn context.CancelFunc
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
			// Update the total overall hashes per second to the sum of the
			// hashes per second of each individual worker.
			hashesPerSec = 0
			m.Lock()
			for _, stats := range m.speedStats {
				totalHashes := stats.totalHashes.Swap(0)
				elapsedMicros := stats.elapsedMicros.Swap(0)
				elapsedSecs := (elapsedMicros / 1000000)
				if totalHashes == 0 || elapsedSecs == 0 {
					continue
				}
				hashesPerSec += float64(totalHashes) / float64(elapsedSecs)
			}
			m.Unlock()
			if hashesPerSec != 0 && !math.IsNaN(hashesPerSec) {
				log.Debugf("Hash speed: %6.0f kilohashes/s", hashesPerSec/1000)
			}

		// Request for the number of hashes per second.
		case m.queryHashesPerSec <- hashesPerSec:
			// Nothing to do.

		case <-ctx.Done():
			break out
		}
	}

	log.Trace("CPU miner speed monitor done")
}

// submitBlock submits the passed block to network after ensuring it passes all
// of the consensus validation rules.
func (m *CPUMiner) submitBlock(block *dcrutil.Block, isBlake3PowActive bool) bool {
	m.submitBlockLock.Lock()
	defer m.submitBlockLock.Unlock()

	// Process this block using the same rules as blocks coming from other
	// nodes. This will in turn relay it to the network like normal.
	err := m.cfg.ProcessBlock(block)
	if err != nil {
		if errors.Is(err, blockchain.ErrMissingParent) {
			log.Errorf("Block submitted via CPU miner is an orphan building "+
				"on parent %v", block.MsgBlock().Header.PrevBlock)
			return false
		}

		// Anything other than a rule violation is an unexpected error,
		// so log that error as an internal error.
		var rErr blockchain.RuleError
		if !errors.As(err, &rErr) {
			log.Errorf("Unexpected error while processing block submitted via "+
				"CPU miner: %v", err)
			return false
		}

		// Other rule errors should be reported.
		log.Errorf("Block submitted via CPU miner rejected: %v", err)
		return false
	}

	// The block was accepted.
	blockHash := block.Hash()
	var powHashStr string
	powHashFn := block.MsgBlock().PowHashV1
	if isBlake3PowActive {
		powHashFn = block.MsgBlock().PowHashV2
	}
	powHash := powHashFn()
	if powHash != *blockHash {
		powHashStr = ", pow hash " + powHash.String()
	}
	log.Infof("Block submitted via CPU miner accepted (hash %s, height %d%s)",
		blockHash, block.Height(), powHashStr)
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
func (m *CPUMiner) solveBlock(ctx context.Context, header *wire.BlockHeader,
	stats *speedStats, isBlake3PowActive bool) bool {

	// Choose a random extra nonce offset for this block template and
	// worker.
	enOffset := rand.Uint64()

	// Create some convenience variables.
	targetDiff, isNeg, overflows := primitives.DiffBitsToUint256(header.Bits)
	if isNeg || overflows {
		log.Errorf("Unable to convert diff bits %08x to uint256 (negative: %v"+
			", overflows: %v)", header.Bits, isNeg, overflows)
		return false
	}

	// Choose the hash function depending on the active agendas.
	powHashFn := blake256.Sum256
	if isBlake3PowActive {
		powHashFn = blake3.Sum256
	}

	// Serialize the header once so only the specific bytes that need to be
	// updated can be done in the main loops below.
	hdrBytes, err := header.Bytes()
	if err != nil {
		log.Errorf("Unexpected error while serializing header: %v", err)
		return false
	}

	// updateSpeedStats is a convenience func to atomically track and update the
	// speed stats from various branches in the code below.
	hashesCompleted := uint64(0)
	start := time.Now()
	updateSpeedStats := func() {
		stats.totalHashes.Add(hashesCompleted)
		elapsedMicros := time.Since(start).Microseconds()
		stats.elapsedMicros.Add(uint64(elapsedMicros))

		hashesCompleted = 0
		start = time.Now()
	}

	// Note that the entire extra nonce range is iterated and the offset is
	// added relying on the fact that overflow will wrap around 0 as
	// provided by the Go spec.  Furthermore, the break condition has been
	// intentionally omitted such that the loop will continue forever until
	// a solution is found.
	for extraNonce := uint64(0); ; extraNonce++ {
		// Update the extra nonce in the serialized header bytes directly.
		const enSerOffset = 144
		littleEndian.PutUint64(hdrBytes[enSerOffset:], extraNonce+enOffset)

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
			// Periodically update the speed stats and check for cancellation.
			if nonce > 0 && nonce%65535 == 0 {
				updateSpeedStats()

				select {
				case <-ctx.Done():
					return false

				default:
					// Non-blocking select to fall through
				}

				m.g.UpdateBlockTime(header)

				// Update time in the serialized header bytes directly too since
				// it might have changed.
				const timestampOffset = 136
				timestamp := uint32(header.Timestamp.Unix())
				littleEndian.PutUint32(hdrBytes[timestampOffset:], timestamp)
			}

			// Update the nonce in the serialized header bytes directly and
			// compute the block header hash.
			const nonceSerOffset = 140
			littleEndian.PutUint32(hdrBytes[nonceSerOffset:], nonce)
			hash := chainhash.Hash(powHashFn(hdrBytes))
			hashesCompleted++

			// The block is solved when the new block hash is less than the
			// target difficulty.  Yay!
			if n := primitives.HashToUint256(&hash); n.LtEq(&targetDiff) {
				// Update the nonce and extra nonce fields in the block template
				// header to the solution.
				littleEndian.PutUint64(header.ExtraData[:], extraNonce+enOffset)
				header.Nonce = nonce
				updateSpeedStats()
				return true
			}

			if nonce == maxNonce {
				updateSpeedStats()
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
func (m *CPUMiner) solver(ctx context.Context, template *mining.BlockTemplate,
	speedStats *speedStats, isBlake3PowActive bool) {

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
		shallowBlockHdr := &shallowBlockCopy.Header
		if m.solveBlock(ctx, shallowBlockHdr, speedStats, isBlake3PowActive) {
			// Avoid submitting any solutions that might have been found in
			// between the time a worker was signalled to stop and it actually
			// stopping.
			if ctx.Err() != nil {
				return
			}

			block := dcrutil.NewBlock(&shallowBlockCopy)
			if !m.submitBlock(block, isBlake3PowActive) {
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
func (m *CPUMiner) generateBlocks(ctx context.Context, workerID uint64) {
	log.Trace("Starting generate blocks worker")
	defer log.Trace("Generate blocks worker done")

	// Separate waitgroup for solvers to ensure they are stopped prior to
	// terminating the goroutine.
	var solverWg sync.WaitGroup
	defer solverWg.Wait()

	// Subscribe for block template updates and ensure the subscription is
	// stopped along with the worker.
	templateSub := m.g.Subscribe()
	defer templateSub.Stop()

	// Create a new state for tracking speed stats and add it to the global
	// map that the speed monitor periodically polls.
	var speedStats speedStats
	m.Lock()
	m.speedStats[workerID] = &speedStats
	m.Unlock()

	var solverCtx context.Context
	var solverCancel context.CancelFunc
	for {
		select {
		case templateNtfn := <-templateSub.C():
			// Clean up the map that tracks the number of blocks mined on a
			// given parent whenever a template is received due to a new parent.
			template := templateNtfn.Template
			prevHash := template.Block.Header.PrevBlock
			if m.cfg.PermitConnectionlessMining {
				if templateNtfn.Reason == mining.TURNewParent {
					m.Lock()
					for k := range m.minedOnParents {
						if k != prevHash {
							delete(m.minedOnParents, k)
						}
					}
					m.Unlock()
				}
			}

			// Ensure the previous solver goroutine (if any) is stopped.
			if solverCancel != nil {
				solverCancel()
			}

			// Determine the state of the blake3 proof of work agenda.  An error
			// should never really happen here in practice, but just loop around
			// and wait for another template if it does.
			isBlake3PowActive, err := m.cfg.IsBlake3PowAgendaActive(&prevHash)
			if err != nil {
				continue
			}

			// Start another goroutine for the new template.
			solverCtx, solverCancel = context.WithCancel(ctx)
			solverWg.Add(1)
			go func() {
				m.solver(solverCtx, template, &speedStats, isBlake3PowActive)
				solverWg.Done()
			}()

		case <-ctx.Done():
			// Ensure resources associated with the solver goroutine context are
			// freed as needed.
			if solverCancel != nil {
				solverCancel()
			}
			m.Lock()
			delete(m.speedStats, workerID)
			m.Unlock()

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
	// Separate waitgroup for workers to ensure they are stopped prior to
	// terminating the goroutine.
	var workerWg sync.WaitGroup
	defer workerWg.Wait()

	// launchWorker groups common code to launch a worker for subscribing for
	// template updates and solving blocks.
	type workerState struct {
		cancel context.CancelFunc
	}
	var curWorkerID uint64
	var runningWorkers []workerState
	launchWorker := func() {
		wCtx, wCancel := context.WithCancel(ctx)
		runningWorkers = append(runningWorkers, workerState{
			cancel: wCancel,
		})

		workerWg.Add(1)
		go func() {
			m.generateBlocks(wCtx, curWorkerID)
			workerWg.Done()
		}()
		curWorkerID++
	}

out:
	for {
		select {
		// Update the number of running workers.
		case <-m.updateNumWorkers:
			numRunning := uint32(len(runningWorkers))
			numWorkers := m.numWorkers.Load()

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
}

// Run starts the CPU miner with zero workers which means it will be idle. It
// blocks until the provided context is cancelled.
//
// Use the SetNumWorkers method to start solving blocks in the normal mining
// mode.
func (m *CPUMiner) Run(ctx context.Context) {
	log.Trace("Starting CPU miner in idle state")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		m.speedMonitor(ctx)
		wg.Done()
	}()
	go func() {
		m.miningWorkerController(ctx)
		wg.Done()
	}()

	// Shutdown the miner when the context is cancelled.
	<-ctx.Done()
	close(m.quit)
	wg.Wait()
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
// via [CPUMiner.GenerateNBlocks].
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
	m.numWorkers.Store(targetNumWorkers)

	// Set the normal mining state accordingly.
	if targetNumWorkers != 0 {
		m.normalMining = true
	} else {
		m.normalMining = false
	}

	// Notify the controller about the change.
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
	return int32(m.numWorkers.Load())
}

// GenerateNBlocks generates blocks on an as needed basis to extend the main
// chain by the requested number of blocks in the discrete mining mode and
// returns a list of hashes of the blocks that ultimately extended the main
// chain, regardless of their origin.
//
// The aforementioned distinction regarding the origin of the returned hashes is
// important to note because the blocks that ultimately extend the main chain
// may be different from the blocks generated since blocks from other sources
// may arrive while the discrete mining process is underway leading to
// generation of more or less blocks than the target number.
//
// A best effort attempt is made to avoid as many forks as possible by
// discarding any locally-mined blocks that would knowingly cause a fork with
// blocks that arrive from other sources and to only generate as many blocks as
// are necessary to achieve extending the main chain by the specified number of
// blocks.  However, forks may still occur since it is not always possible to
// detect forks beforehand.  Nevertheless, any such forks will not count towards
// the requested number of blocks since they do not extend the main chain.
//
// Only one instance of discrete mining may be active at once.  An active
// discrete mining instance may be canceled by specifying 0 for the number of
// blocks.  A canceled instance will return [ErrCancelDiscreteMining].
//
// Specifying 0 for the number of blocks when there are no active discrete
// mining instances has no effect.
//
// An error will be returned when either normal mining is active or when a
// discrete mining instance is already active and the specified number of blocks
// is not 0.
//
// It makes use of a subscription to the background block template generator to
// obtain the templates and attempts to solve them while automatically switching
// to new templates as they become available as needed.  As a result, it
// supports many of the nice features of the template subscriptions such as
// giving all votes a chance to arrive and automatically switching to templates
// building on new blocks that arrive from other sources.
func (m *CPUMiner) GenerateNBlocks(ctx context.Context, n uint32) ([]chainhash.Hash, error) {
	// Respond with an error if server is already normal mining or when discrete
	// mining and the specified number of blocks is nonzero.
	m.Lock()
	if m.normalMining {
		m.Unlock()
		return nil, errors.New("server is already CPU mining -- please call " +
			"`setgenerate 0` before calling discrete `generate` commands")
	}
	if m.discreteMining && n != 0 {
		m.Unlock()
		return nil, errors.New("server is already discrete mining -- please " +
			"wait until the existing call completes or cancel it")
	}

	// Cancel any outstanding discrete mining calls and return when the caller
	// specified 0 blocks.
	if n == 0 {
		if m.generateCancelFn != nil {
			m.generateCancelFn()
		}
		m.Unlock()
		return nil, nil
	}

	// Create a child context that can be cancelled and activate discrete mining
	// mode.
	genCtx, genCancelFn := context.WithCancel(ctx)
	m.generateCancelFn = genCancelFn
	m.discreteMining = true
	m.Unlock()
	defer func() {
		// Disable discrete mining mode.
		m.Lock()
		m.discreteMining = false
		if m.generateCancelFn != nil {
			m.generateCancelFn()
			m.generateCancelFn = nil
		}
		m.Unlock()
	}()

	log.Tracef("Extending the main chain %d blocks", n)

	// Separate waitgroup for solve goroutine to ensure it's stopped prior to
	// returning.
	var solveWg sync.WaitGroup

	// Subscribe for block template updates and ensure the subscription is
	// stopped when done.
	templateSub := m.g.Subscribe()
	defer templateSub.Stop()

	// Determine the starting and target block heights of the main chain based
	// on the requested number of blocks.
	origBestHeight := m.cfg.BestSnapshot().Height
	targetBestHeight := origBestHeight + int64(n)

	done := make(chan struct{})
	var closeDoneOnce sync.Once
	safeCloseDone := func() { closeDoneOnce.Do(func() { close(done) }) }
	var solveCancel context.CancelFunc
out:
	for {
		// Wait for a new template update notification or early shutdown.
		select {
		case templateNtfn := <-templateSub.C():
			// Stop once the main chain reaches the target best height.
			if m.cfg.BestSnapshot().Height >= targetBestHeight {
				safeCloseDone()
				continue
			}

			// Since callers might call this method in rapid succession and the
			// subscription immediately sends the current template, the template
			// might not have been updated yet (for example, it might be waiting
			// on votes).  In that case, wait for the updated template.
			if templateNtfn.Template == m.discretePrevTemplate.Load() {
				continue
			}
			m.discretePrevTemplate.Store(nil)

			// Ensure the previous solve goroutine (if any) is stopped.
			if solveCancel != nil {
				solveCancel()
			}

			// Determine the state of the blake3 proof of work agenda.  An error
			// should never really happen here in practice, but just loop around
			// and wait for another template if it does.
			prevHash := templateNtfn.Template.Block.Header.PrevBlock
			isBlake3PowActive, err := m.cfg.IsBlake3PowAgendaActive(&prevHash)
			if err != nil {
				continue
			}

			// Attempt to solve the block in a separate goroutine so it can be
			// stopped early when new block templates are notified.
			solveCtx, cancel := context.WithCancel(genCtx)
			solveCancel = cancel
			solveWg.Add(1)
			go func() {
				defer solveWg.Done()

				// The block solving function will exit with false if the block
				// was not solved for any reason such as the context being
				// cancelled or an unexpected error.
				//
				// When the return is true, a solution was found, so attempt to
				// submit the solved block.
				//
				// The block in the template is shallow copied to avoid mutating
				// the data of the shared template.
				var stats speedStats
				shallowBlockCopy := *templateNtfn.Template.Block
				header := &shallowBlockCopy.Header
				if m.solveBlock(solveCtx, header, &stats, isBlake3PowActive) {
					// Avoid submitting any solutions that might have been found
					// in between the time a worker was signalled to stop and it
					// actually stopping.
					if solveCtx.Err() != nil {
						return
					}

					// Avoid submitting any solutions that would extend the main
					// chain beyond the target height and close the done channel
					// immediately to avoid waiting for a new template.
					if m.cfg.BestSnapshot().Height >= targetBestHeight {
						safeCloseDone()
						return
					}

					block := dcrutil.NewBlock(&shallowBlockCopy)
					if m.submitBlock(block, isBlake3PowActive) {
						m.discretePrevTemplate.Store(templateNtfn.Template)

						// Close the done channel immediately once the final
						// block is submitted to avoid waiting for a new
						// template.
						if m.cfg.BestSnapshot().Height >= targetBestHeight {
							safeCloseDone()
						}
					}
				}
			}()

		case <-done:
			break out
		case <-genCtx.Done():
			break out
		case <-m.quit:
			break out
		}
	}

	// Ensure resources associated with the solve goroutine context are freed
	// and wait for the solve groutines to finish if needed.
	if solveCancel != nil {
		solveCancel()
	}
	solveWg.Wait()

	numExtended := m.cfg.BestSnapshot().Height - origBestHeight
	log.Tracef("Extended the main chain %d blocks", numExtended)
	if genCtx.Err() != nil {
		return nil, ErrCancelDiscreteMining
	}

	// Return the block hashes that ultimately extended the main chain
	// regardless of their origin.
	blockHashes := make([]chainhash.Hash, 0, n)
	for height := origBestHeight + 1; height <= targetBestHeight; height++ {
		var blockHash chainhash.Hash
		hash, err := m.cfg.BlockHashByHeight(height)
		if err == nil {
			blockHash = *hash
		}
		blockHashes = append(blockHashes, blockHash)
	}
	return blockHashes, nil
}

// New returns a new instance of a CPU miner for the provided configuration
// options.
//
// Use Run to initialize the CPU miner and then either use SetNumWorkers with a
// non-zero value to start the normal continuous mining mode or use
// [CPUMiner.GenerateNBlocks] to mine a discrete number of blocks.
//
// See the documentation for CPUMiner type for more details.
func New(cfg *Config) *CPUMiner {
	miner := &CPUMiner{
		g:                 cfg.BgBlkTmplGenerator,
		cfg:               cfg,
		updateNumWorkers:  make(chan struct{}),
		queryHashesPerSec: make(chan float64),
		speedStats:        make(map[uint64]*speedStats),
		minedOnParents:    make(map[chainhash.Hash]uint8),
		quit:              make(chan struct{}),
	}
	miner.numWorkers.Store(defaultNumWorkers)
	return miner
}
