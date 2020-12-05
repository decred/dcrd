// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/blockchain/v4"
	"github.com/decred/dcrd/blockchain/v4/indexers"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/internal/mining"
	"github.com/decred/dcrd/txscript/v3"
	"github.com/decred/dcrd/wire"
)

const (
	// DefaultBlockPrioritySize is the default size in bytes for high-
	// priority / low-fee transactions.  It is used to help determine which
	// are allowed into the mempool and consequently affects their relay and
	// inclusion when generating block templates.
	DefaultBlockPrioritySize = 20000

	// maxRelayFeeMultiplier is the factor that we disallow fees / kB above the
	// minimum tx fee.  At the current default minimum relay fee of 0.0001
	// DCR/kB, this results in a maximum allowed high fee of 1 DCR/kB.
	maxRelayFeeMultiplier = 1e4

	// maxVoteDoubleSpends is the maximum number of vote double spends allowed
	// in the pool.
	maxVoteDoubleSpends = 5

	// heightDiffToPruneTicket is the number of blocks to pass by in terms
	// of height before old tickets are pruned.
	// TODO Set this based up the stake difficulty retargeting interval?
	heightDiffToPruneTicket = 288

	// heightDiffToPruneVotes is the number of blocks to pass by in terms
	// of height before SSGen relating to that block are pruned.
	heightDiffToPruneVotes = 10

	// maxNullDataOutputs is the maximum number of OP_RETURN null data
	// pushes in a transaction, after which it is considered non-standard.
	maxNullDataOutputs = 4

	// orphanTTL is the maximum amount of time an orphan is allowed to
	// stay in the orphan pool before it expires and is evicted during the
	// next scan.
	orphanTTL = time.Minute * 15

	// orphanExpireScanInterval is the minimum amount of time in between
	// scans of the orphan pool to evict expired transactions.
	orphanExpireScanInterval = time.Minute * 5

	// MempoolMaxConcurrentTSpends is the maximum number of TSpends that
	// are allowed in the mempool. The number 7 is also the amount of
	// physical space available for TSpend votes and thus is a hard limit.
	MempoolMaxConcurrentTSpends = 7
)

// Tag represents an identifier to use for tagging orphan transactions.  The
// caller may choose any scheme it desires, however it is common to use peer IDs
// so that orphans can be identified by which peer first relayed them.
type Tag uint64

// Config is a descriptor containing the memory pool configuration.
type Config struct {
	// Policy defines the various mempool configuration options related
	// to policy.
	Policy Policy

	// ChainParams identifies which chain parameters the txpool is
	// associated with.
	ChainParams *chaincfg.Params

	// NextStakeDifficulty defines the function to retrieve the stake
	// difficulty for the block after the current best block.
	//
	// This function must be safe for concurrent access.
	NextStakeDifficulty func() (int64, error)

	// FetchUtxoView defines the function to use to fetch unspent
	// transaction output information.
	FetchUtxoView func(*dcrutil.Tx, bool) (*blockchain.UtxoViewpoint, error)

	// BlockByHash defines the function use to fetch the block identified
	// by the given hash.
	BlockByHash func(*chainhash.Hash) (*dcrutil.Block, error)

	// BestHash defines the function to use to access the block hash of
	// the current best chain.
	BestHash func() *chainhash.Hash

	// BestHeight defines the function to use to access the block height of
	// the current best chain.
	BestHeight func() int64

	// PastMedianTime defines the function to use in order to access the
	// median time calculated from the point-of-view of the current chain
	// tip within the best chain.
	PastMedianTime func() time.Time

	// CalcSequenceLock defines the function to use in order to generate
	// the current sequence lock for the given transaction using the passed
	// utxo view.
	CalcSequenceLock func(*dcrutil.Tx, *blockchain.UtxoViewpoint) (*blockchain.SequenceLock, error)

	// SubsidyCache defines a subsidy cache to use.
	SubsidyCache *standalone.SubsidyCache

	// SigCache defines a signature cache to use.
	SigCache *txscript.SigCache

	// AddrIndex defines the optional address index instance to use for
	// indexing the unconfirmed transactions in the memory pool.
	// This can be nil if the address index is not enabled.
	AddrIndex *indexers.AddrIndex

	// ExistsAddrIndex defines the optional exists address index instance
	// to use for indexing the unconfirmed transactions in the memory pool.
	// This can be nil if the address index is not enabled.
	ExistsAddrIndex *indexers.ExistsAddrIndex

	// AddTxToFeeEstimation defines an optional function to be called whenever a
	// new transaction is added to the mempool, which can be used to track fees
	// for the purposes of smart fee estimation.
	AddTxToFeeEstimation func(txHash *chainhash.Hash, fee, size int64, txType stake.TxType)

	// RemoveTxFromFeeEstimation defines an optional function to be called
	// whenever a transaction is removed from the mempool in order to track fee
	// estimation.
	RemoveTxFromFeeEstimation func(txHash *chainhash.Hash)

	// OnVoteReceived defines the function used to signal receiving a new
	// vote in the mempool.
	OnVoteReceived func(voteTx *dcrutil.Tx)

	// IsTreasuryAgendaActive returns if the treasury agenda is active or
	// not.
	IsTreasuryAgendaActive func() (bool, error)

	// OnTSpendReceived defines the function used to signal receiving a new
	// tspend in the mempool.
	OnTSpendReceived func(voteTx *dcrutil.Tx)

	// TSpendMinedOnAncestor returns an error if the provided tspend has
	// been mined in an ancestor block.
	TSpendMinedOnAncestor func(tspend chainhash.Hash) error
}

// Policy houses the policy (configuration parameters) which is used to
// control the mempool.
type Policy struct {
	// MaxTxVersion is the max transaction version that the mempool should
	// accept.  All transactions above this version are rejected as
	// non-standard.
	MaxTxVersion uint16

	// DisableRelayPriority defines whether to relay free or low-fee
	// transactions that do not have enough priority to be relayed.
	DisableRelayPriority bool

	// AcceptNonStd defines whether to accept and relay non-standard
	// transactions to the network. If true, non-standard transactions
	// will be accepted into the mempool and relayed to the rest of the
	// network. Otherwise, all non-standard transactions will be rejected.
	AcceptNonStd bool

	// FreeTxRelayLimit defines the given amount in thousands of bytes
	// per minute that transactions with no fee are rate limited to.
	FreeTxRelayLimit float64

	// MaxOrphanTxs is the maximum number of orphan transactions
	// that can be queued.
	MaxOrphanTxs int

	// MaxOrphanTxSize is the maximum size allowed for orphan transactions.
	// This helps prevent memory exhaustion attacks from sending a lot of
	// of big orphans.
	MaxOrphanTxSize int

	// MaxSigOpsPerTx is the maximum number of signature operations
	// in a single transaction we will relay or mine.  It is a fraction
	// of the max signature operations for a block.
	MaxSigOpsPerTx int

	// MinRelayTxFee defines the minimum transaction fee in DCR/kB to be
	// considered a non-zero fee.
	MinRelayTxFee dcrutil.Amount

	// AllowOldVotes defines whether or not votes on old blocks will be
	// admitted and relayed.
	AllowOldVotes bool

	// MaxVoteAge defines the number of blocks in history from the next block
	// height of the best chain tip for which votes will be accepted.  This only
	// applies when the AllowOldVotes option is false.
	MaxVoteAge uint16

	// StandardVerifyFlags defines the function to retrieve the flags to
	// use for verifying scripts for the block after the current best block.
	// It must set the verification flags properly depending on the result
	// of any agendas that affect them.
	//
	// This function must be safe for concurrent access.
	StandardVerifyFlags func() (txscript.ScriptFlags, error)

	// AcceptSequenceLocks defines the function to determine whether or not
	// to accept transactions with sequence locks.  Typically this will be
	// set depending on the result of the fix sequence locks agenda vote.
	//
	// This function must be safe for concurrent access.
	AcceptSequenceLocks func() (bool, error)

	// EnableAncestorTracking controls whether the mining view tracks
	// transaction relationships in the mempool.
	EnableAncestorTracking bool
}

// TxDesc is a descriptor containing a transaction in the mempool along with
// additional metadata.
type TxDesc struct {
	mining.TxDesc

	// StartingPriority is the priority of the transaction when it was added
	// to the pool.
	StartingPriority float64
}

// VerboseTxDesc is a descriptor containing a transaction in the mempool along
// with additional more expensive to calculate metadata.  Callers should prefer
// working with the more efficient TxDesc unless they specifically need access
// to the additional details provided.
type VerboseTxDesc struct {
	TxDesc

	// CurrentPriority is the current priority of the transaction within the
	// pool.
	CurrentPriority float64

	// Depends enumerates any unconfirmed transactions in the pool used as
	// inputs for the transaction.
	Depends []*TxDesc
}

// orphanTx is a normal transaction that references an ancestor transaction
// that is not yet available.  It also contains additional information related
// to it such as an expiration time to help prevent caching the orphan forever.
type orphanTx struct {
	tx         *dcrutil.Tx
	tag        Tag
	expiration time.Time
}

// TxPool is used as a source of transactions that need to be mined into blocks
// and relayed to other peers.  It is safe for concurrent access from multiple
// peers.
type TxPool struct {
	// The following variables must only be used atomically.
	lastUpdated int64 // last time pool was updated.

	mtx  sync.RWMutex
	cfg  Config
	pool map[chainhash.Hash]*TxDesc

	orphans       map[chainhash.Hash]*orphanTx
	orphansByPrev map[wire.OutPoint]map[chainhash.Hash]*dcrutil.Tx
	outpoints     map[wire.OutPoint]*dcrutil.Tx
	miningView    *mining.TxMiningView

	staged          map[chainhash.Hash]*dcrutil.Tx
	stagedOutpoints map[wire.OutPoint]*dcrutil.Tx

	// Votes on blocks.
	votesMtx sync.RWMutex
	votes    map[chainhash.Hash][]mining.VoteDesc

	// TSpends. Access MUST be protected by the mempool mutex.
	tspends map[chainhash.Hash]*dcrutil.Tx

	pennyTotal    float64 // exponentially decaying total for penny spends.
	lastPennyUnix int64   // unix time of last ``penny spend''

	// nextExpireScan is the time after which the orphan pool will be
	// scanned in order to evict orphans.  This is NOT a hard deadline as
	// the scan will only run when an orphan is added to the pool as opposed
	// to on an unconditional timer.
	nextExpireScan time.Time
}

// insertVote inserts a vote into the map of block votes.
//
// This function MUST be called with the vote mutex locked (for writes).
func (mp *TxPool) insertVote(ssgen *dcrutil.Tx) {
	// Get the block it is voting on; here we're agnostic of height.
	msgTx := ssgen.MsgTx()
	blockHash, blockHeight := stake.SSGenBlockVotedOn(msgTx)

	// If there are currently no votes for this block,
	// start a new buffered slice and store it.
	vts, exists := mp.votes[blockHash]
	if !exists {
		vts = make([]mining.VoteDesc, 0, mp.cfg.ChainParams.TicketsPerBlock)
	}

	// Nothing to do if a vote for the ticket is already known.
	ticketHash := &msgTx.TxIn[1].PreviousOutPoint.Hash
	for _, vt := range vts {
		if vt.TicketHash.IsEqual(ticketHash) {
			return
		}
	}

	voteHash := ssgen.Hash()
	voteBits := stake.SSGenVoteBits(msgTx)
	vote := dcrutil.IsFlagSet16(voteBits, dcrutil.BlockValid)
	voteTx := mining.VoteDesc{
		VoteHash:       *voteHash,
		TicketHash:     *ticketHash,
		ApprovesParent: vote,
	}

	// Append the new vote.
	mp.votes[blockHash] = append(vts, voteTx)

	log.Debugf("Accepted vote %v for block hash %v (height %v), voting "+
		"%v on the transaction tree", voteHash, blockHash, blockHeight,
		vote)
}

// VoteHashesForBlock returns the hashes for all votes on the provided block
// hash that are currently available in the mempool.
//
// This function is safe for concurrent access.
func (mp *TxPool) VoteHashesForBlock(blockHash *chainhash.Hash) []chainhash.Hash {
	mp.votesMtx.RLock()
	vts, exists := mp.votes[*blockHash]
	mp.votesMtx.RUnlock()

	// Lookup the vote metadata for the block.
	if !exists || len(vts) == 0 {
		return nil
	}

	// Copy the vote hashes from the vote metadata.
	hashes := make([]chainhash.Hash, 0, len(vts))
	for _, vt := range vts {
		hashes = append(hashes, vt.VoteHash)
	}

	return hashes
}

// VotesForBlocks returns the vote metadata for all votes on the provided
// block hashes that are currently available in the mempool.
//
// This function is safe for concurrent access.
func (mp *TxPool) VotesForBlocks(hashes []chainhash.Hash) [][]mining.VoteDesc {
	result := make([][]mining.VoteDesc, 0, len(hashes))

	mp.votesMtx.RLock()
	for _, hash := range hashes {
		votes := mp.votes[hash]
		result = append(result, votes)
	}
	mp.votesMtx.RUnlock()

	return result
}

// TODO Pruning of the votes map DECRED

// TSpendHashes returns hashes of all existing tracked tspends. This function
// is safe for concurrent access.
func (mp *TxPool) TSpendHashes() []chainhash.Hash {
	mp.mtx.RLock()
	res := make([]chainhash.Hash, 0, len(mp.tspends))
	for hash := range mp.tspends {
		res = append(res, hash)
	}
	mp.mtx.RUnlock()
	return res
}

// Ensure the TxPool type implements the mining.TxSource interface.
var _ mining.TxSource = (*TxPool)(nil)

// removeOrphan is the internal function which implements the public
// RemoveOrphan.  See the comment for RemoveOrphan for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) removeOrphan(tx *dcrutil.Tx, removeRedeemers bool, isTreasuryEnabled bool) {
	// Nothing to do if the passed tx does not exist in the orphan pool.
	txHash := tx.Hash()
	otx, exists := mp.orphans[*txHash]
	if !exists {
		return
	}

	log.Tracef("Removing orphan transaction %v", txHash)

	// Remove the reference from the previous orphan index.
	for _, txIn := range otx.tx.MsgTx().TxIn {
		orphans, exists := mp.orphansByPrev[txIn.PreviousOutPoint]
		if exists {
			delete(orphans, *txHash)

			// Remove the map entry altogether if there are no
			// longer any orphans which depend on it.
			if len(orphans) == 0 {
				delete(mp.orphansByPrev, txIn.PreviousOutPoint)
			}
		}
	}

	// Remove any orphans that redeem outputs from this one if requested.
	if removeRedeemers {
		txType := stake.DetermineTxType(tx.MsgTx(), isTreasuryEnabled)
		tree := wire.TxTreeRegular
		if txType != stake.TxTypeRegular {
			tree = wire.TxTreeStake
		}

		prevOut := wire.OutPoint{Hash: *txHash, Tree: tree}
		for txOutIdx := range tx.MsgTx().TxOut {
			prevOut.Index = uint32(txOutIdx)
			for _, orphan := range mp.orphansByPrev[prevOut] {
				mp.removeOrphan(orphan, true, isTreasuryEnabled)
			}
		}
	}

	// Remove the transaction from the orphan pool.
	delete(mp.orphans, *txHash)
}

// RemoveOrphan removes the passed orphan transaction from the orphan pool and
// previous orphan index.
//
// This function is safe for concurrent access.
func (mp *TxPool) RemoveOrphan(tx *dcrutil.Tx, isTreasuryEnabled bool) {
	mp.mtx.Lock()
	mp.removeOrphan(tx, false, isTreasuryEnabled)
	mp.mtx.Unlock()
}

// RemoveOrphansByTag removes all orphan transactions tagged with the provided
// identifier.
//
// This function is safe for concurrent access.
func (mp *TxPool) RemoveOrphansByTag(tag Tag, isTreasuryEnabled bool) uint64 {
	var numEvicted uint64
	mp.mtx.Lock()
	for _, otx := range mp.orphans {
		if otx.tag == tag {
			mp.removeOrphan(otx.tx, true, isTreasuryEnabled)
			numEvicted++
		}
	}
	mp.mtx.Unlock()
	return numEvicted
}

// limitNumOrphans limits the number of orphan transactions by evicting a random
// orphan if adding a new one would cause it to overflow the max allowed.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) limitNumOrphans(isTreasuryEnabled bool) {
	// Scan through the orphan pool and remove any expired orphans when it's
	// time.  This is done for efficiency so the scan only happens periodically
	// instead of on every orphan added to the pool.
	if now := time.Now(); now.After(mp.nextExpireScan) {
		origNumOrphans := len(mp.orphans)
		for _, otx := range mp.orphans {
			if now.After(otx.expiration) {
				// Remove redeemers too because the missing parents are very
				// unlikely to ever materialize since the orphan has already
				// been around more than long enough for them to be delivered.
				mp.removeOrphan(otx.tx, true, isTreasuryEnabled)
			}
		}

		// Set next expiration scan to occur after the scan interval.
		mp.nextExpireScan = now.Add(orphanExpireScanInterval)

		numOrphans := len(mp.orphans)
		if numExpired := origNumOrphans - numOrphans; numExpired > 0 {
			log.Debugf("Expired %d %s (remaining: %d)", numExpired,
				pickNoun(numExpired, "orphan", "orphans"), numOrphans)
		}
	}

	// Nothing to do if adding another orphan will not cause the pool to
	// exceed the limit.
	if len(mp.orphans)+1 <= mp.cfg.Policy.MaxOrphanTxs {
		return
	}

	// Remove a random entry from the map.  For most compilers, Go's
	// range statement iterates starting at a random item although
	// that is not 100% guaranteed by the spec.  The iteration order
	// is not important here because an adversary would have to be
	// able to pull off preimage attacks on the hashing function in
	// order to target eviction of specific entries anyways.
	for _, otx := range mp.orphans {
		// Don't remove redeemers in the case of a random eviction since
		// it is quite possible it might be needed again shortly.
		mp.removeOrphan(otx.tx, false, isTreasuryEnabled)
		break
	}
}

// addOrphan adds an orphan transaction to the orphan pool.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) addOrphan(tx *dcrutil.Tx, tag Tag, isTreasuryEnabled bool) {
	// Nothing to do if no orphans are allowed.
	if mp.cfg.Policy.MaxOrphanTxs <= 0 {
		return
	}

	// Limit the number orphan transactions to prevent memory exhaustion.
	// This will periodically remove any expired orphans and evict a random
	// orphan if space is still needed.
	mp.limitNumOrphans(isTreasuryEnabled)

	mp.orphans[*tx.Hash()] = &orphanTx{
		tx:         tx,
		tag:        tag,
		expiration: time.Now().Add(orphanTTL),
	}
	for _, txIn := range tx.MsgTx().TxIn {
		if _, exists := mp.orphansByPrev[txIn.PreviousOutPoint]; !exists {
			mp.orphansByPrev[txIn.PreviousOutPoint] =
				make(map[chainhash.Hash]*dcrutil.Tx)
		}
		mp.orphansByPrev[txIn.PreviousOutPoint][*tx.Hash()] = tx
	}

	log.Debugf("Stored orphan transaction %v (total: %d)", tx.Hash(),
		len(mp.orphans))
}

// maybeAddOrphan potentially adds an orphan to the orphan pool.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) maybeAddOrphan(tx *dcrutil.Tx, tag Tag, isTreasuryEnabled bool) error {
	// Ignore orphan transactions that are too large.  This helps avoid
	// a memory exhaustion attack based on sending a lot of really large
	// orphans.  In the case there is a valid transaction larger than this,
	// it will ultimately be rebroadcast after the parent transactions
	// have been mined or otherwise received.
	//
	// Note that the number of orphan transactions in the orphan pool is
	// also limited, so this equates to a maximum memory used of
	// mp.cfg.Policy.MaxOrphanTxSize * mp.cfg.Policy.MaxOrphanTxs (which is ~5MB
	// using the default values at the time this comment was written).
	serializedLen := tx.MsgTx().SerializeSize()
	if serializedLen > mp.cfg.Policy.MaxOrphanTxSize {
		str := fmt.Sprintf("orphan transaction size of %d bytes is "+
			"larger than max allowed size of %d bytes",
			serializedLen, mp.cfg.Policy.MaxOrphanTxSize)
		return txRuleError(ErrOrphanPolicyViolation, str)
	}

	// Add the orphan if the none of the above disqualified it.
	mp.addOrphan(tx, tag, isTreasuryEnabled)

	return nil
}

// removeOrphanDoubleSpends removes all orphans which spend outputs spent by the
// passed transaction from the orphan pool.  Removing those orphans then leads
// to removing all orphans which rely on them, recursively.  This is necessary
// when a transaction is added to the main pool because it may spend outputs
// that orphans also spend.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) removeOrphanDoubleSpends(tx *dcrutil.Tx, isTreasuryEnabled bool) {
	msgTx := tx.MsgTx()
	for _, txIn := range msgTx.TxIn {
		for _, orphan := range mp.orphansByPrev[txIn.PreviousOutPoint] {
			mp.removeOrphan(orphan, true, isTreasuryEnabled)
		}
	}
}

// isTransactionInPool returns whether or not the passed transaction already
// exists in the main pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) isTransactionInPool(hash *chainhash.Hash) bool {
	if _, exists := mp.pool[*hash]; exists {
		return true
	}

	return false
}

// IsTransactionInPool returns whether or not the passed transaction already
// exists in the main pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) IsTransactionInPool(hash *chainhash.Hash) bool {
	// Protect concurrent access.
	mp.mtx.RLock()
	inPool := mp.isTransactionInPool(hash)
	mp.mtx.RUnlock()

	return inPool
}

// isOrphanInPool returns whether or not the passed transaction already exists
// in the orphan pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) isOrphanInPool(hash *chainhash.Hash) bool {
	if _, exists := mp.orphans[*hash]; exists {
		return true
	}

	return false
}

// IsOrphanInPool returns whether or not the passed transaction already exists
// in the orphan pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) IsOrphanInPool(hash *chainhash.Hash) bool {
	// Protect concurrent access.
	mp.mtx.RLock()
	inPool := mp.isOrphanInPool(hash)
	mp.mtx.RUnlock()

	return inPool
}

// isTransactionStaged determines if the transaction exists in the
// stage pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) isTransactionStaged(hash *chainhash.Hash) bool {
	_, exists := mp.staged[*hash]
	return exists
}

// stageTransaction creates an entry for the provided
// transaction in the stage pool.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) stageTransaction(tx *dcrutil.Tx) {
	mp.staged[*tx.Hash()] = tx
	for _, txIn := range tx.MsgTx().TxIn {
		mp.stagedOutpoints[txIn.PreviousOutPoint] = tx
	}
}

// removeStagedTransaction removes the provided transaction
// from the stage pool.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) removeStagedTransaction(stagedTx *dcrutil.Tx) {
	delete(mp.staged, *stagedTx.Hash())
	for _, txIn := range stagedTx.MsgTx().TxIn {
		delete(mp.stagedOutpoints, txIn.PreviousOutPoint)
	}
}

// hasMempoolInput returns true if the provided transaction
// has an input in the main pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) hasMempoolInput(tx *dcrutil.Tx) bool {
	for _, txIn := range tx.MsgTx().TxIn {
		if mp.isTransactionInPool(&txIn.PreviousOutPoint.Hash) {
			return true
		}
	}

	return false
}

// fetchRedeemers returns all transactions that reference an outpoint for
// the provided regular transaction `tx`. Returns nil if a non-regular
// transaction is provided.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) fetchRedeemers(outpoints map[wire.OutPoint]*dcrutil.Tx, tx *dcrutil.Tx, isTreasuryEnabled bool) []*dcrutil.Tx {
	txType := stake.DetermineTxType(tx.MsgTx(), isTreasuryEnabled)
	if txType != stake.TxTypeRegular {
		return nil
	}

	tree := wire.TxTreeRegular
	seen := map[chainhash.Hash]struct{}{}
	redeemers := make([]*dcrutil.Tx, 0)
	prevOut := wire.OutPoint{Hash: *tx.Hash(), Tree: tree}
	for i := uint32(0); i < uint32(len(tx.MsgTx().TxOut)); i++ {
		prevOut.Index = i
		txRedeemer, exists := outpoints[prevOut]
		if !exists {
			continue
		}
		if _, exists := seen[*txRedeemer.Hash()]; exists {
			continue
		}

		seen[*txRedeemer.Hash()] = struct{}{}
		redeemers = append(redeemers, txRedeemer)
	}

	return redeemers
}

// MaybeAcceptDependents determines if there are any staged dependents of the
// passed transaction and potentially accepts them to the memory pool.
//
// It returns a slice of transactions added to the mempool.  A nil slice means
// no transactions were moved from the stage pool to the mempool.
//
// This function is safe for concurrent access.
func (mp *TxPool) MaybeAcceptDependents(tx *dcrutil.Tx, isTreasuryEnabled bool) []*dcrutil.Tx {
	mp.mtx.Lock()
	defer mp.mtx.Unlock()

	var acceptedTxns []*dcrutil.Tx
	for _, redeemer := range mp.fetchRedeemers(mp.stagedOutpoints, tx,
		isTreasuryEnabled) {
		redeemerTxType := stake.DetermineTxType(redeemer.MsgTx(),
			isTreasuryEnabled)
		if redeemerTxType == stake.TxTypeSStx {
			// Quick check to skip tickets with mempool inputs.
			if mp.hasMempoolInput(redeemer) {
				continue
			}

			// Remove the dependent transaction and attempt to add it to the
			// mempool or back to the stage pool. In the event of an error, the
			// transaction will be discarded.
			log.Tracef("Removing ticket %v with no mempool dependencies from "+
				"stage pool", *redeemer.Hash())
			mp.removeStagedTransaction(redeemer)
			_, err := mp.maybeAcceptTransaction(
				redeemer, true, true, true, true, isTreasuryEnabled)

			if err != nil {
				log.Debugf("Failed to add previously staged "+
					"ticket %v to pool. %v", *redeemer.Hash(), err)
			}

			if mp.isTransactionInPool(redeemer.Hash()) {
				acceptedTxns = append(acceptedTxns, redeemer)
			}
		}
	}

	return acceptedTxns
}

// haveTransaction returns whether or not the passed transaction already exists
// in the main pool or in the orphan pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) haveTransaction(hash *chainhash.Hash) bool {
	return mp.isTransactionInPool(hash) || mp.isOrphanInPool(hash) ||
		mp.isTransactionStaged(hash)
}

// HaveTransaction returns whether or not the passed transaction already exists
// in the main pool or in the orphan pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) HaveTransaction(hash *chainhash.Hash) bool {
	// Protect concurrent access.
	mp.mtx.RLock()
	haveTx := mp.haveTransaction(hash)
	mp.mtx.RUnlock()

	return haveTx
}

// haveTransactions returns whether or not the passed transactions already exist
// in the main pool or in the orphan pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) haveTransactions(hashes []*chainhash.Hash) []bool {
	have := make([]bool, len(hashes))
	for i := range hashes {
		have[i] = mp.haveTransaction(hashes[i])
	}
	return have
}

// HaveTransactions returns whether or not the passed transactions already exist
// in the main pool or in the orphan pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) HaveTransactions(hashes []*chainhash.Hash) []bool {
	// Protect concurrent access.
	mp.mtx.RLock()
	haveTxns := mp.haveTransactions(hashes)
	mp.mtx.RUnlock()
	return haveTxns
}

// HaveAllTransactions returns whether or not all of the passed transaction
// hashes exist in the mempool.
//
// This function is safe for concurrent access.
func (mp *TxPool) HaveAllTransactions(hashes []chainhash.Hash) bool {
	mp.mtx.RLock()
	inPool := true
	for _, h := range hashes {
		if _, exists := mp.pool[h]; !exists {
			inPool = false
			break
		}
	}
	mp.mtx.RUnlock()
	return inPool
}

// removeTransaction is the internal function which implements the public
// RemoveTransaction.  See the comment for RemoveTransaction for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) removeTransaction(tx *dcrutil.Tx, removeRedeemers bool, isTreasuryEnabled bool) {
	txHash := tx.Hash()
	if removeRedeemers {
		// Remove any transactions which rely on this one.
		txType := stake.DetermineTxType(tx.MsgTx(), isTreasuryEnabled)
		tree := wire.TxTreeRegular
		if txType != stake.TxTypeRegular {
			tree = wire.TxTreeStake
		}

		prevOut := wire.OutPoint{Hash: *txHash, Tree: tree}
		for i := uint32(0); i < uint32(len(tx.MsgTx().TxOut)); i++ {
			prevOut.Index = i
			if txRedeemer, exists := mp.outpoints[prevOut]; exists {
				mp.removeTransaction(txRedeemer, true,
					isTreasuryEnabled)
				continue
			}
			if txRedeemer, exists := mp.stagedOutpoints[prevOut]; exists {
				log.Tracef("Removing staged transaction %v", prevOut.Hash)
				mp.removeStagedTransaction(txRedeemer)
			}
		}
	}

	// Remove the transaction if needed.
	if txDesc, exists := mp.pool[*txHash]; exists {
		log.Tracef("Removing transaction %v", txHash)

		// Remove unconfirmed address index entries associated with the
		// transaction if enabled.
		if mp.cfg.AddrIndex != nil {
			mp.cfg.AddrIndex.RemoveUnconfirmedTx(txHash)
		}

		// Mark the referenced outpoints as unspent by the pool.
		for _, txIn := range txDesc.Tx.MsgTx().TxIn {
			delete(mp.outpoints, txIn.PreviousOutPoint)
		}

		// Stop tracking this transaction in the mining view.
		// If redeeming transactions are going to be removed from the
		// graph, then do not update their stats.
		updateDescendantStats := !removeRedeemers
		mp.miningView.Remove(tx.Hash(), updateDescendantStats)

		delete(mp.pool, *txHash)

		atomic.StoreInt64(&mp.lastUpdated, time.Now().Unix())

		// Inform associated fee estimator that the transaction has been removed
		// from the mempool
		if mp.cfg.RemoveTxFromFeeEstimation != nil {
			mp.cfg.RemoveTxFromFeeEstimation(txHash)
		}

		// Stop tracking if it's a tspend.
		delete(mp.tspends, *txHash)
	}
}

// RemoveTransaction removes the passed transaction from the mempool. When the
// removeRedeemers flag is set, any transactions that redeem outputs from the
// removed transaction will also be removed recursively from the mempool, as
// they would otherwise become orphans.
//
// This function is safe for concurrent access.
func (mp *TxPool) RemoveTransaction(tx *dcrutil.Tx, removeRedeemers bool, isTreasuryEnabled bool) {
	// Protect concurrent access.
	mp.mtx.Lock()
	mp.removeTransaction(tx, removeRedeemers, isTreasuryEnabled)
	mp.mtx.Unlock()
}

// RemoveDoubleSpends removes all transactions which spend outputs spent by the
// passed transaction from the memory pool.  Removing those transactions then
// leads to removing all transactions which rely on them, recursively.  This is
// necessary when a block is connected to the main chain because the block may
// contain transactions which were previously unknown to the memory pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) RemoveDoubleSpends(tx *dcrutil.Tx, isTreasuryEnabled bool) {
	// Protect concurrent access.
	mp.mtx.Lock()
	for _, txIn := range tx.MsgTx().TxIn {
		if txRedeemer, ok := mp.outpoints[txIn.PreviousOutPoint]; ok {
			if !txRedeemer.Hash().IsEqual(tx.Hash()) {
				mp.removeTransaction(txRedeemer, true,
					isTreasuryEnabled)
			}
		}
		if txRedeemer, ok := mp.stagedOutpoints[txIn.PreviousOutPoint]; ok {
			if !txRedeemer.Hash().IsEqual(tx.Hash()) {
				log.Debugf("Removing double spend transaction %v "+
					"from stage pool", tx.Hash())
				mp.removeStagedTransaction(txRedeemer)
			}
		}
	}
	mp.mtx.Unlock()
}

// findTx returns a transaction from the mempool by hash.  If it does not exist
// in the mempool, a nil pointer is returned.
func (mp *TxPool) findTx(txHash *chainhash.Hash) *mining.TxDesc {
	poolTx := mp.pool[*txHash]
	if poolTx == nil {
		return nil
	}

	return &poolTx.TxDesc
}

// addTransaction adds the passed transaction to the memory pool.  It should
// not be called directly as it doesn't perform any validation.  This is a
// helper for maybeAcceptTransaction.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) addTransaction(utxoView *blockchain.UtxoViewpoint, tx *dcrutil.Tx,
	txType stake.TxType, height int64, fee int64, isTreasuryEnabled bool, totalSigOps int) {

	// Notify callback about vote if requested.
	if mp.cfg.OnVoteReceived != nil && txType == stake.TxTypeSSGen {
		mp.cfg.OnVoteReceived(tx)
	}

	// Add the transaction to the pool and mark the referenced outpoints
	// as spent by the pool.
	msgTx := tx.MsgTx()
	poolTxDesc := &TxDesc{
		TxDesc: mining.TxDesc{
			Tx:          tx,
			Type:        txType,
			Added:       time.Now(),
			Height:      height,
			Fee:         fee,
			TotalSigOps: totalSigOps,
			TxSize:      int64(tx.MsgTx().SerializeSize()),
		},
		StartingPriority: mining.CalcPriority(msgTx, utxoView, height),
	}

	mp.pool[*tx.Hash()] = poolTxDesc
	mp.miningView.AddTransaction(&poolTxDesc.TxDesc, mp.findTx)

	for _, txIn := range msgTx.TxIn {
		mp.outpoints[txIn.PreviousOutPoint] = tx
	}
	atomic.StoreInt64(&mp.lastUpdated, time.Now().Unix())

	// Add unconfirmed address index entries associated with the transaction
	// if enabled.
	if mp.cfg.AddrIndex != nil {
		mp.cfg.AddrIndex.AddUnconfirmedTx(tx, utxoView, isTreasuryEnabled)
	}
	if mp.cfg.ExistsAddrIndex != nil {
		mp.cfg.ExistsAddrIndex.AddUnconfirmedTx(msgTx, isTreasuryEnabled)
	}

	// Inform the associated fee estimator that a new transaction has been added
	// to the mempool
	if mp.cfg.AddTxToFeeEstimation != nil {
		mp.cfg.AddTxToFeeEstimation(tx.Hash(), fee, int64(msgTx.SerializeSize()),
			txType)
	}
}

// checkPoolDoubleSpend checks whether or not the passed transaction is
// attempting to spend coins already spent by other transactions in the pool.
// Note it does not check for double spends against transactions already in the
// main chain.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) checkPoolDoubleSpend(tx *dcrutil.Tx, txType stake.TxType, isTreasuryEnabled bool) error {
	for i, txIn := range tx.MsgTx().TxIn {
		// We don't care about double spends of stake bases.
		if i == 0 && (txType == stake.TxTypeSSGen ||
			txType == stake.TxTypeSSRtx) {
			continue
		}

		// Ignore Treasury bases
		if isTreasuryEnabled {
			if i == 0 && (txType == stake.TxTypeTreasuryBase ||
				txType == stake.TxTypeTSpend) {
				continue
			}
		}

		if txR, exists := mp.outpoints[txIn.PreviousOutPoint]; exists {
			str := fmt.Sprintf("transaction %v in the pool "+
				"already spends the same coins", txR.Hash())
			return txRuleError(ErrMempoolDoubleSpend, str)
		}

		if txR, exists := mp.stagedOutpoints[txIn.PreviousOutPoint]; exists {
			str := fmt.Sprintf("staged transaction %v in the pool "+
				"already spends the same coins", txR.Hash())
			return txRuleError(ErrMempoolDoubleSpend, str)
		}
	}

	return nil
}

// checkVoteDoubleSpend checks whether or not the passed vote is for a block
// that already has a vote that spends the same ticket available.  This is
// necessary because the same ticket might be selected for blocks on candidate
// side chains and thus a more generic check to merely reject double spends of
// tickets is not possible.
//
// This function MUST be called with the mempool lock held (for reads).
// This function MUST NOT be called with the votes mutex held.
func (mp *TxPool) checkVoteDoubleSpend(vote *dcrutil.Tx) error {
	voteTx := vote.MsgTx()
	ticketSpent := voteTx.TxIn[1].PreviousOutPoint.Hash
	hashVotedOn, heightVotedOn := stake.SSGenBlockVotedOn(voteTx)
	mp.votesMtx.RLock()
	for _, existingVote := range mp.votes[hashVotedOn] {
		if existingVote.TicketHash == ticketSpent {
			// Ensure the vote is still actually in the mempool.  This is needed
			// because the votes map is not currently kept in sync with the
			// contents of the pool.
			//
			// TODO(decred): Ideally the votes map and mempool would be kept in
			// sync, which would remove the need for this check, however, there
			// is currently code outside of mempool that relies on being able to
			// look up seen votes by block hash, regardless of their current
			// membership in the pool.
			if _, exists := mp.pool[existingVote.VoteHash]; !exists {
				continue
			}

			mp.votesMtx.RUnlock()
			str := fmt.Sprintf("vote %v spending ticket %v already votes on "+
				"block %s (height %d)", vote.Hash(), ticketSpent, hashVotedOn,
				heightVotedOn)
			return txRuleError(ErrAlreadyVoted, str)
		}
	}
	mp.votesMtx.RUnlock()

	return nil
}

// IsRegTxTreeKnownDisapproved returns whether or not the regular tree of the
// block represented by the provided hash is known to be disapproved according
// to the votes currently in the memory pool.
//
// The function is safe for concurrent access.
func (mp *TxPool) IsRegTxTreeKnownDisapproved(hash *chainhash.Hash) bool {
	mp.votesMtx.RLock()
	vts := mp.votes[*hash]
	mp.votesMtx.RUnlock()

	// There are not possibly enough votes to tell if the regular transaction
	// tree is approved or not, so assume it's valid.
	if len(vts) <= int(mp.cfg.ChainParams.TicketsPerBlock/2) {
		return false
	}

	// Otherwise, tally the votes and determine if it's approved or not.
	var yes, no int
	for _, vote := range vts {
		if vote.ApprovesParent {
			yes++
		} else {
			no++
		}
	}

	return yes <= no
}

// fetchInputUtxos loads utxo details about the input transactions referenced by
// the passed transaction.  First, it loads the details from the viewpoint of
// the main chain, then it adjusts them based upon the contents of the
// transaction pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) fetchInputUtxos(tx *dcrutil.Tx, isTreasuryEnabled bool) (*blockchain.UtxoViewpoint, error) {
	knownDisapproved := mp.IsRegTxTreeKnownDisapproved(mp.cfg.BestHash())
	utxoView, err := mp.cfg.FetchUtxoView(tx, !knownDisapproved)
	if err != nil {
		return nil, err
	}

	// Attempt to populate any missing inputs from the transaction pool.
	for originHash, entry := range utxoView.Entries() {
		if entry != nil && !entry.IsFullySpent() {
			continue
		}

		if poolTxDesc, exists := mp.pool[originHash]; exists {
			utxoView.AddTxOuts(poolTxDesc.Tx, mining.UnminedHeight,
				wire.NullBlockIndex, isTreasuryEnabled)
		}

		if stagedTx, exists := mp.staged[originHash]; exists {
			utxoView.AddTxOuts(stagedTx, mining.UnminedHeight,
				wire.NullBlockIndex, isTreasuryEnabled)
		}
	}

	return utxoView, nil
}

// FetchTransaction returns the requested transaction from the transaction pool.
// This only fetches from the main transaction pool and does not include
// orphans.
//
// This function is safe for concurrent access.
func (mp *TxPool) FetchTransaction(txHash *chainhash.Hash) (*dcrutil.Tx, error) {
	var tx *dcrutil.Tx

	// Protect concurrent access.
	mp.mtx.RLock()
	txDesc, exists := mp.pool[*txHash]
	if exists {
		tx = txDesc.Tx
	} else {
		// Check if the transaction is in the stage pool.
		tx, exists = mp.staged[*txHash]
	}
	mp.mtx.RUnlock()

	if exists {
		return tx, nil
	}

	return nil, fmt.Errorf("transaction is not in the pool")
}

// maybeAcceptTransaction is the internal function which implements the public
// MaybeAcceptTransaction.  See the comment for MaybeAcceptTransaction for
// more details.
//
// This function MUST be called with the mempool lock held (for writes).
//
// DECRED - TODO
// We need to make sure thing also assigns the TxType after it evaluates the tx,
// so that we can easily pick different stake tx types from the mempool later.
// This should probably be done at the bottom using "IsSStx" etc functions.
// It should also set the dcrutil tree type for the tx as well.
func (mp *TxPool) maybeAcceptTransaction(tx *dcrutil.Tx, isNew, rateLimit, allowHighFees, rejectDupOrphans bool, isTreasuryEnabled bool) ([]*chainhash.Hash, error) {
	msgTx := tx.MsgTx()
	txHash := tx.Hash()
	// Don't accept the transaction if it already exists in the pool.  This
	// applies to orphan transactions as well when the reject duplicate
	// orphans flag is set.  This check is intended to be a quick check to
	// weed out duplicates.
	if mp.isTransactionInPool(txHash) || mp.isTransactionStaged(txHash) ||
		(rejectDupOrphans && mp.isOrphanInPool(txHash)) {
		str := fmt.Sprintf("already have transaction %v", txHash)
		return nil, txRuleError(ErrDuplicate, str)
	}

	// Perform preliminary validation checks on the transaction.  This makes use
	// of blockchain which contains the invariant rules for what transactions
	// are allowed into blocks.
	checkTxFlags := blockchain.AFNone
	if isTreasuryEnabled {
		checkTxFlags |= blockchain.AFTreasuryEnabled
	}
	err := blockchain.CheckTransaction(msgTx, mp.cfg.ChainParams, checkTxFlags)
	if err != nil {
		var cerr blockchain.RuleError
		if errors.As(err, &cerr) {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// A standalone transaction must not be a coinbase transaction.
	if standalone.IsCoinBaseTx(msgTx, isTreasuryEnabled) {
		str := fmt.Sprintf("transaction %v is an individual coinbase",
			txHash)
		return nil, txRuleError(ErrCoinbase, str)
	}

	// Get the current height of the main chain.  A standalone transaction
	// will be mined into the next block at best, so its height is at least
	// one more than the current height.
	bestHeight := mp.cfg.BestHeight()
	nextBlockHeight := bestHeight + 1

	// Don't accept transactions that will be expired as of the next block.
	if blockchain.IsExpired(tx, nextBlockHeight) {
		str := fmt.Sprintf("transaction %v expired at height %d",
			txHash, msgTx.Expiry)
		return nil, txRuleError(ErrExpired, str)
	}

	// Determine what type of transaction we're dealing with (regular or stake).
	// Then, be sure to set the tx tree correctly as it's possible a user submitted
	// it to the network with TxTreeUnknown.
	txType := stake.DetermineTxType(msgTx, isTreasuryEnabled)
	if txType == stake.TxTypeRegular {
		tx.SetTree(wire.TxTreeRegular)
	} else {
		tx.SetTree(wire.TxTreeStake)
	}
	isVote := txType == stake.TxTypeSSGen

	var isTreasuryBase, isTSpend bool
	if isTreasuryEnabled {
		isTSpend = txType == stake.TxTypeTSpend
		isTreasuryBase = txType == stake.TxTypeTreasuryBase
	}

	// Choose whether or not to accept transactions with sequence locks enabled.
	//
	// Typically, this will be set based on the result of the fix sequence locks
	// agenda vote.
	acceptSeqLocks, err := mp.cfg.Policy.AcceptSequenceLocks()
	if err != nil {
		var cerr blockchain.RuleError
		if errors.As(err, &cerr) {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}
	if !acceptSeqLocks {
		if msgTx.Version >= 2 && !isVote {
			for _, txIn := range msgTx.TxIn {
				sequenceNum := txIn.Sequence
				if sequenceNum&wire.SequenceLockTimeDisabled != 0 {
					continue
				}

				str := "violates sequence lock consensus bug"
				return nil, txRuleError(ErrInvalid, str)
			}
		}
	}

	// Reject votes before stake validation height.
	stakeValidationHeight := mp.cfg.ChainParams.StakeValidationHeight
	if (isVote || isTSpend) && nextBlockHeight < stakeValidationHeight {
		strType := "votes"
		if isTSpend {
			strType = "tspends"
		}
		str := fmt.Sprintf("%s are not valid until block height %d (next "+
			"block height %d)", strType, stakeValidationHeight, nextBlockHeight)
		return nil, txRuleError(ErrInvalid, str)
	}

	// Reject revocations before they can possibly be valid.  A vote must be
	// missed for a revocation to be valid and votes are not allowed until stake
	// validation height, so, a revocations can't possibly be valid until one
	// block later.
	isRevocation := txType == stake.TxTypeSSRtx
	if isRevocation && nextBlockHeight < stakeValidationHeight+1 {
		str := fmt.Sprintf("revocations are not valid until block height %d "+
			"(next block height %d)", stakeValidationHeight+1, nextBlockHeight)
		return nil, txRuleError(ErrInvalid, str)
	}

	// Don't allow non-standard transactions if the mempool config forbids
	// their acceptance and relaying.
	medianTime := mp.cfg.PastMedianTime()
	if !mp.cfg.Policy.AcceptNonStd {
		err := checkTransactionStandard(tx, txType, nextBlockHeight,
			medianTime, mp.cfg.Policy.MinRelayTxFee,
			mp.cfg.Policy.MaxTxVersion, isTreasuryEnabled)
		if err != nil {
			str := fmt.Sprintf("transaction %v is not standard: %v",
				txHash, err)
			return nil, wrapTxRuleError(ErrNonStandard, str, err)
		}
	}

	// If the transaction is a ticket, ensure that it meets the next
	// stake difficulty.
	isTicket := txType == stake.TxTypeSStx
	if isTicket {
		sDiff, err := mp.cfg.NextStakeDifficulty()
		if err != nil {
			// This is an unexpected error so don't turn it into a
			// rule error.
			return nil, err
		}

		if msgTx.TxOut[0].Value < sDiff {
			str := fmt.Sprintf("transaction %v has not enough funds "+
				"to meet stake difficulty (ticket diff %v < next diff %v)",
				txHash, msgTx.TxOut[0].Value, sDiff)
			return nil, txRuleError(ErrInsufficientFee, str)
		}
	}

	// Aside from a few exceptions for votes and revocations, the transaction
	// may not use any of the same outputs as other transactions already in the
	// pool as that would ultimately result in a double spend.  This check is
	// intended to be quick and therefore only detects double spends within the
	// transaction pool itself.  The transaction could still be double spending
	// coins from the main chain at this point.  There is a more in-depth check
	// that happens later after fetching the referenced transaction inputs from
	// the main chain which examines the actual spend data and prevents double
	// spends.
	if !isVote && !isRevocation {
		err = mp.checkPoolDoubleSpend(tx, txType, isTreasuryEnabled)
		if err != nil {
			return nil, err
		}

	} else if isVote {
		// Reject votes on blocks that already have a vote that spends the same
		// ticket available.  This is necessary because the same ticket might be
		// selected for blocks on candidate side chains and thus a more generic
		// check to merely reject double spends of tickets is not possible.
		err := mp.checkVoteDoubleSpend(tx)
		if err != nil {
			return nil, err
		}

		voteAlreadyFound := 0
		for _, mpTx := range mp.pool {
			if mpTx.Type == stake.TxTypeSSGen {
				if mpTx.Tx.MsgTx().TxIn[1].PreviousOutPoint ==
					msgTx.TxIn[1].PreviousOutPoint {
					voteAlreadyFound++
				}
			}
			if voteAlreadyFound >= maxVoteDoubleSpends {
				str := fmt.Sprintf("transaction %v in the pool with more than "+
					"%v votes", msgTx.TxIn[1].PreviousOutPoint,
					maxVoteDoubleSpends)
				return nil, txRuleError(ErrTooManyVotes, str)
			}
		}

	} else if isRevocation {
		for _, mpTx := range mp.pool {
			if mpTx.Type == stake.TxTypeSSRtx {
				if mpTx.Tx.MsgTx().TxIn[0].PreviousOutPoint ==
					msgTx.TxIn[0].PreviousOutPoint {
					str := fmt.Sprintf("transaction %v in the pool as a "+
						"revocation. Only one revocation is allowed.",
						msgTx.TxIn[0].PreviousOutPoint)
					return nil, txRuleError(ErrDuplicateRevocation, str)
				}
			}
		}
	}

	// Votes that are on too old of blocks are rejected.
	if isVote {
		_, voteHeight := stake.SSGenBlockVotedOn(msgTx)
		if int64(voteHeight) < nextBlockHeight-int64(mp.cfg.Policy.MaxVoteAge) &&
			!mp.cfg.Policy.AllowOldVotes {
			str := fmt.Sprintf("transaction %v votes on old "+
				"block height of %d which is before the "+
				"current cutoff height of %v", tx.Hash(),
				voteHeight, nextBlockHeight-int64(mp.cfg.Policy.MaxVoteAge))
			return nil, txRuleError(ErrOldVote, str)
		}
	}

	// Fetch all of the unspent transaction outputs referenced by the inputs
	// to this transaction.  This function also attempts to fetch the
	// transaction itself to be used for detecting a duplicate transaction
	// without needing to do a separate lookup.
	utxoView, err := mp.fetchInputUtxos(tx, isTreasuryEnabled)
	if err != nil {
		var cerr blockchain.RuleError
		if errors.As(err, &cerr) {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Don't allow the transaction if it exists in the main chain and is not
	// already fully spent.
	txEntry := utxoView.LookupEntry(txHash)
	if txEntry != nil && !txEntry.IsFullySpent() {
		return nil, txRuleError(ErrAlreadyExists, "transaction already exists")
	}
	delete(utxoView.Entries(), *txHash)

	// Transaction is an orphan if any of the inputs don't exist.
	var missingParents []*chainhash.Hash
	for i, txIn := range msgTx.TxIn {
		if (i == 0 && (isVote || isTreasuryBase)) || isTSpend {
			continue
		}

		entry := utxoView.LookupEntry(&txIn.PreviousOutPoint.Hash)
		if entry == nil || entry.IsFullySpent() {
			// Must make a copy of the hash here since the iterator
			// is replaced and taking its address directly would
			// result in all of the entries pointing to the same
			// memory location and thus all be the final hash.
			hashCopy := txIn.PreviousOutPoint.Hash
			missingParents = append(missingParents, &hashCopy)

			// Prevent a panic in the logger by continuing here if the
			// transaction input is nil.
			if entry == nil {
				log.Tracef("Transaction %v uses unknown input %v "+
					"and will be considered an orphan", txHash,
					txIn.PreviousOutPoint.Hash)
				continue
			}
			if entry.IsFullySpent() {
				log.Tracef("Transaction %v uses full spent input %v "+
					"and will be considered an orphan", txHash,
					txIn.PreviousOutPoint.Hash)
			}
		}
	}

	if len(missingParents) > 0 {
		return missingParents, nil
	}

	// Don't allow the transaction into the mempool unless its sequence
	// lock is active, meaning that it'll be allowed into the next block
	// with respect to its defined relative lock times.
	seqLock, err := mp.cfg.CalcSequenceLock(tx, utxoView)
	if err != nil {
		var cerr blockchain.RuleError
		if errors.As(err, &cerr) {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}
	if !blockchain.SequenceLockActive(seqLock, nextBlockHeight, medianTime) {
		return nil, txRuleError(ErrSeqLockUnmet,
			"transaction sequence locks on inputs not met")
	}

	// Perform several checks on the transaction inputs using the invariant
	// rules in blockchain for what transactions are allowed into blocks.
	// Also returns the fees associated with the transaction which will be
	// used later.  The fraud proof is not checked because it will be
	// filled in by the miner.
	txFee, err := blockchain.CheckTransactionInputs(mp.cfg.SubsidyCache,
		tx, nextBlockHeight, utxoView, false, mp.cfg.ChainParams,
		isTreasuryEnabled)
	if err != nil {
		var cerr blockchain.RuleError
		if errors.As(err, &cerr) {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Don't allow transactions with non-standard inputs if the mempool config
	// forbids their acceptance and relaying.
	if !mp.cfg.Policy.AcceptNonStd {
		err := checkInputsStandard(tx, txType, utxoView,
			isTreasuryEnabled)
		if err != nil {
			str := fmt.Sprintf("transaction %v has a non-standard "+
				"input: %v", txHash, err)
			return nil, wrapTxRuleError(ErrNonStandard, str, err)
		}
	}

	// NOTE: if you modify this code to accept non-standard transactions,
	// you should add code here to check that the transaction does a
	// reasonable number of ECDSA signature verifications.

	// Don't allow transactions with an excessive number of signature
	// operations which would result in making it impossible to mine.  Since
	// the coinbase address itself can contain signature operations, the
	// maximum allowed signature operations per transaction is less than
	// the maximum allowed signature operations per block.
	numP2SHSigOps, err := blockchain.CountP2SHSigOps(tx, false,
		(txType == stake.TxTypeSSGen), utxoView, isTreasuryEnabled)
	if err != nil {
		var cerr blockchain.RuleError
		if errors.As(err, &cerr) {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	numSigOps := blockchain.CountSigOps(tx, false, isVote, isTreasuryEnabled)
	totalSigOps := numP2SHSigOps + numSigOps
	if totalSigOps > mp.cfg.Policy.MaxSigOpsPerTx {
		str := fmt.Sprintf("transaction %v has too many sigops: %d > %d",
			txHash, totalSigOps, mp.cfg.Policy.MaxSigOpsPerTx)
		return nil, txRuleError(ErrNonStandard, str)
	}

	// Don't allow transactions with fees too low to get into a mined block.
	//
	// Most miners allow a free transaction area in blocks they mine to go
	// alongside the area used for high-priority transactions as well as
	// transactions with fees.  A transaction size of up to 1000 bytes is
	// considered safe to go into this section.  Further, the minimum fee
	// calculated below on its own would encourage several small
	// transactions to avoid fees rather than one single larger transaction
	// which is more desirable.  Therefore, as long as the size of the
	// transaction does not exceed 1000 less than the reserved space for
	// high-priority transactions, don't require a fee for it.
	// This applies to non-stake transactions only.
	serializedSize := int64(msgTx.SerializeSize())
	minFee := calcMinRequiredTxRelayFee(serializedSize,
		mp.cfg.Policy.MinRelayTxFee)
	if txType == stake.TxTypeRegular { // Non-stake only
		if serializedSize >= (DefaultBlockPrioritySize-1000) &&
			txFee < minFee {

			str := fmt.Sprintf("transaction %v has %v fees which "+
				"is under the required amount of %v", txHash,
				txFee, minFee)
			return nil, txRuleError(ErrInsufficientFee, str)
		}
	}

	// Require that free transactions have sufficient priority to be mined
	// in the next block.  Transactions which are being added back to the
	// memory pool from blocks that have been disconnected during a reorg
	// are exempted.
	//
	// This applies to non-stake transactions only.
	if isNew && !mp.cfg.Policy.DisableRelayPriority && txFee < minFee &&
		txType == stake.TxTypeRegular {

		currentPriority := mining.CalcPriority(msgTx, utxoView,
			nextBlockHeight)
		if currentPriority <= mining.MinHighPriority {
			str := fmt.Sprintf("transaction %v has insufficient "+
				"priority (%g <= %g)", txHash,
				currentPriority, mining.MinHighPriority)
			return nil, txRuleError(ErrInsufficientPriority, str)
		}
	}

	// Free-to-relay transactions are rate limited here to prevent
	// penny-flooding with tiny transactions as a form of attack.
	// This applies to non-stake transactions only.
	if rateLimit && txFee < minFee && txType == stake.TxTypeRegular {
		nowUnix := time.Now().Unix()
		// Decay passed data with an exponentially decaying ~10 minute
		// window.
		mp.pennyTotal *= math.Pow(1.0-1.0/600.0,
			float64(nowUnix-mp.lastPennyUnix))
		mp.lastPennyUnix = nowUnix

		// Are we still over the limit?
		if mp.pennyTotal >= mp.cfg.Policy.FreeTxRelayLimit*10*1000 {
			str := fmt.Sprintf("transaction %v has been rejected "+
				"by the rate limiter due to low fees", txHash)
			return nil, txRuleError(ErrInsufficientFee, str)
		}
		oldTotal := mp.pennyTotal

		mp.pennyTotal += float64(serializedSize)
		log.Tracef("rate limit: curTotal %v, nextTotal: %v, "+
			"limit %v", oldTotal, mp.pennyTotal,
			mp.cfg.Policy.FreeTxRelayLimit*10*1000)
	}

	// Check that tickets also pay the minimum of the relay fee.  This fee is
	// also performed on regular transactions above, but fees lower than the
	// minimum may be allowed when there is sufficient priority, and these
	// checks aren't desired for ticket purchases.
	if isTicket {
		minTicketFee := calcMinRequiredTxRelayFee(serializedSize,
			mp.cfg.Policy.MinRelayTxFee)
		if txFee < minTicketFee {
			str := fmt.Sprintf("ticket purchase transaction %v has a %v "+
				"fee which is under the required threshold amount of %d",
				txHash, txFee, minTicketFee)
			return nil, txRuleError(ErrInsufficientFee, str)
		}
	}

	// Check whether allowHighFees is set to false (default), if so, then make
	// sure the current fee is sensible.  If people would like to avoid this
	// check then they can AllowHighFees = true
	if !allowHighFees {
		maxFee := calcMinRequiredTxRelayFee(serializedSize*maxRelayFeeMultiplier,
			mp.cfg.Policy.MinRelayTxFee)
		if txFee > maxFee {
			str := fmt.Sprintf("transaction %v has %v fee which is above the "+
				"allowHighFee check threshold amount of %v", txHash,
				txFee, maxFee)
			return nil, txRuleError(ErrFeeTooHigh, str)
		}
	}

	// Verify crypto signatures for each input and reject the transaction if
	// any don't verify.
	flags, err := mp.cfg.Policy.StandardVerifyFlags()
	if err != nil {
		return nil, err
	}
	err = blockchain.ValidateTransactionScripts(tx, utxoView, flags,
		mp.cfg.SigCache)
	if err != nil {
		var cerr blockchain.RuleError
		if errors.As(err, &cerr) {
			return nil, chainRuleError(cerr)
		}
		return nil, err
	}

	// Tickets cannot be included in a block until all inputs have
	// been approved by stakeholders. Consensus rules dictate that stake
	// transactions must precede regular transactions, and that inputs for any
	// transaction must precede its redeemer. As a result, tickets with mempool
	// inputs are placed in a separate `stage` pool rather than the main tx
	// pool since they cannot be included in the next block.
	//
	// Note: The scenario where a mempool ticket spends from a known-disapproved
	// regular transaction that is not in the mempool is accounted for during
	// block template generation.
	if isTicket && mp.hasMempoolInput(tx) {
		mp.stageTransaction(tx)
		log.Debugf("Accepted ticket %v with mempool dependency "+
			"into stage pool", txHash)
		return nil, nil
	}

	// Only allow TSpends that have a valid Expiry.
	if isTreasuryEnabled && isTSpend {
		// Shorter variable names for relevant chain parameters.
		tvi := mp.cfg.ChainParams.TreasuryVoteInterval
		mul := mp.cfg.ChainParams.TreasuryVoteIntervalMultiplier

		// Ensure the TSpend expiry isn't too far in the future, before
		// its voting is supposed to start. We arbitrarily define as
		// "too far in the future" as the vote starting greater than or
		// equal to two full voting windows in the future.
		voteStart, _, err := standalone.CalcTSpendWindow(msgTx.Expiry, tvi, mul)
		if err != nil {
			str := fmt.Sprintf("Invalid tspend expiry %d: %v ",
				msgTx.Expiry, err)
			return nil, txRuleError(ErrTSpendInvalidExpiry, str)
		}
		voteStartThresh := int64(2 * tvi * mul)
		blocksToVoteStart := int64(voteStart) - nextBlockHeight
		voteStartDistantFuture := int64(voteStart) > nextBlockHeight &&
			blocksToVoteStart >= voteStartThresh
		if voteStartDistantFuture {
			str := fmt.Sprintf("Tspend voting too far in the "+
				"future: voting starts in %d blocks while the "+
				"voting threshold is %d blocks",
				blocksToVoteStart, voteStartThresh)
			return nil, txRuleError(ErrTSpendInvalidExpiry, str)
		}

		// Only allow up to MempoolMaxConcurrentTSpends TSpends in the
		// mempool.
		tspends := len(mp.tspends)
		if tspends >= MempoolMaxConcurrentTSpends {
			str := fmt.Sprintf("Mempool can only hold %v "+
				"concurrent TSpend transactions",
				MempoolMaxConcurrentTSpends)
			return nil, txRuleError(ErrTooManyTSpends, str)
		}

		// Verify that this TSpend uses a well-known Pi key and that
		// the signature is valid.
		signature, pubKey, err := stake.CheckTSpend(msgTx)
		if err != nil {
			str := fmt.Sprintf("Mempool invalid TSpend: %v", err)
			return nil, txRuleError(ErrInvalid, str)
		}
		if !mp.cfg.ChainParams.PiKeyExists(pubKey) {
			str := fmt.Sprintf("Unknown Pi Key: %x", pubKey)
			return nil, txRuleError(ErrInvalid, str)
		}
		err = blockchain.VerifyTSpendSignature(msgTx, signature, pubKey)
		if err != nil {
			str := fmt.Sprintf("Mempool invalid TSpend signature: "+
				"%v", err)
			return nil, txRuleError(ErrInvalid, str)
		}

		// Verify that this tspend hash has not been included in an
		// ancestor block yet.
		if err := mp.cfg.TSpendMinedOnAncestor(*txHash); err != nil {
			// err is descriptive and only needs to be wrapped.
			return nil, txRuleError(ErrTSpendMinedOnAncestor, err.Error())
		}

		// Notify that we accepted a TSpend.
		if mp.cfg.OnTSpendReceived != nil {
			mp.cfg.OnTSpendReceived(tx)
		}

		log.Tracef("TSpend allowed in mempool: nbh %v expiry %v "+
			"tvi %v tvim %v tspends %v", nextBlockHeight, msgTx.Expiry,
			tvi, mul, tspends)
	}

	// Add to transaction pool.
	mp.addTransaction(utxoView, tx, txType, bestHeight, txFee, isTreasuryEnabled,
		totalSigOps)

	// A regular transaction that is added back to the mempool causes
	// any mempool tickets that redeem it to leave the main pool and enter the
	// `stage` pool.
	if !isNew && txType == stake.TxTypeRegular {
		for _, redeemer := range mp.fetchRedeemers(mp.outpoints, tx,
			isTreasuryEnabled) {
			redeemerDesc, exists := mp.pool[*redeemer.Hash()]
			if exists && redeemerDesc.Type == stake.TxTypeSStx {
				mp.removeTransaction(redeemer, true,
					isTreasuryEnabled)
				mp.stageTransaction(redeemer)
				log.Debugf("Moved ticket %v dependent on %v into stage pool",
					redeemer.Hash(), tx.Hash())
			}
		}
	}

	// Keep track of votes separately.
	if isVote {
		mp.votesMtx.Lock()
		mp.insertVote(tx)
		mp.votesMtx.Unlock()
	}

	// Keep track of tspends separately.
	if isTSpend {
		mp.tspends[*txHash] = tx
	}

	log.Debugf("Accepted transaction %v (pool size: %v)", txHash,
		len(mp.pool))

	return nil, nil
}

// MaybeAcceptTransaction is the main workhorse for handling insertion of new
// free-standing transactions into a memory pool.  It includes functionality
// such as rejecting duplicate transactions, ensuring transactions follow all
// rules, orphan transaction handling, and insertion into the memory pool.  The
// isOrphan parameter can be nil if the caller does not need to know whether
// or not the transaction is an orphan.
//
// This function is safe for concurrent access.
func (mp *TxPool) MaybeAcceptTransaction(tx *dcrutil.Tx, isNew, rateLimit bool) ([]*chainhash.Hash, error) {
	isTreasuryEnabled, err := mp.cfg.IsTreasuryAgendaActive()
	if err != nil {
		return nil, err
	}

	// Protect concurrent access.
	mp.mtx.Lock()
	hashes, err := mp.maybeAcceptTransaction(tx, isNew, rateLimit, true,
		true, isTreasuryEnabled)
	mp.mtx.Unlock()

	return hashes, err
}

// processOrphans is the internal function which implements the public
// ProcessOrphans.  See the comment for ProcessOrphans for more details.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) processOrphans(acceptedTx *dcrutil.Tx, isTreasuryEnabled bool) []*dcrutil.Tx {
	var acceptedTxns []*dcrutil.Tx

	// Start with processing at least the passed transaction.
	processList := []*dcrutil.Tx{acceptedTx}
	for len(processList) > 0 {
		// Pop the transaction to process from the front of the list.
		processItem := processList[0]
		processList[0] = nil
		processList = processList[1:]

		txType := stake.DetermineTxType(processItem.MsgTx(),
			isTreasuryEnabled)
		tree := wire.TxTreeRegular
		if txType != stake.TxTypeRegular {
			tree = wire.TxTreeStake
		}

		prevOut := wire.OutPoint{Hash: *processItem.Hash(), Tree: tree}
		for txOutIdx := range processItem.MsgTx().TxOut {
			// Look up all orphans that redeem the output that is
			// now available.  This will typically only be one, but
			// it could be multiple if the orphan pool contains
			// double spends.  While it may seem odd that the orphan
			// pool would allow this since there can only possibly
			// ultimately be a single redeemer, it's important to
			// track it this way to prevent malicious actors from
			// being able to purposely construct orphans that
			// would otherwise make outputs unspendable.
			//
			// Skip to the next available output if there are none.
			prevOut.Index = uint32(txOutIdx)
			orphans, exists := mp.orphansByPrev[prevOut]
			if !exists {
				continue
			}

			// Potentially accept an orphan into the tx pool.
			for _, tx := range orphans {
				missing, err := mp.maybeAcceptTransaction(
					tx, true, true, true, false,
					isTreasuryEnabled)
				if err != nil {
					// The orphan is now invalid, so there
					// is no way any other orphans which
					// redeem any of its outputs can be
					// accepted.  Remove them.
					mp.removeOrphan(tx, true,
						isTreasuryEnabled)
					break
				}

				// Transaction is still an orphan.  Try the next
				// orphan which redeems this output.
				if len(missing) > 0 {
					continue
				}

				// Transaction was accepted into the main pool.
				//
				// Add it to the list of accepted transactions
				// that are no longer orphans, remove it from
				// the orphan pool, and add it to the list of
				// transactions to process so any orphans that
				// depend on it are handled too.
				acceptedTxns = append(acceptedTxns, tx)
				mp.removeOrphan(tx, false, isTreasuryEnabled)
				processList = append(processList, tx)

				// Only one transaction for this outpoint can be
				// accepted, so the rest are now double spends
				// and are removed later.
				break
			}
		}
	}

	// Recursively remove any orphans that also redeem any outputs redeemed
	// by the accepted transactions since those are now definitive double
	// spends.
	mp.removeOrphanDoubleSpends(acceptedTx, isTreasuryEnabled)
	for _, tx := range acceptedTxns {
		mp.removeOrphanDoubleSpends(tx, isTreasuryEnabled)
	}

	return acceptedTxns
}

// PruneStakeTx is the function which is called every time a new block is
// processed.  The idea is any outstanding SStx that hasn't been mined in a
// certain period of time (CoinbaseMaturity) and the submitted SStx's
// stake difficulty is below the current required stake difficulty should be
// pruned from mempool since they will never be mined.  The same idea stands
// for SSGen and SSRtx
func (mp *TxPool) PruneStakeTx(requiredStakeDifficulty, height int64) {
	isTreasuryEnabled, err := mp.cfg.IsTreasuryAgendaActive()
	if err != nil {
		return
	}

	// Protect concurrent access.
	mp.mtx.Lock()
	mp.pruneStakeTx(requiredStakeDifficulty, height, isTreasuryEnabled)
	mp.mtx.Unlock()
}

func (mp *TxPool) pruneStakeTx(requiredStakeDifficulty, height int64, isTreasuryEnabled bool) {
	for _, tx := range mp.pool {
		txType := stake.DetermineTxType(tx.Tx.MsgTx(), isTreasuryEnabled)
		if txType == stake.TxTypeSStx &&
			tx.Height+int64(heightDiffToPruneTicket) < height {
			mp.removeTransaction(tx.Tx, true, isTreasuryEnabled)
		}
		if txType == stake.TxTypeSStx &&
			tx.Tx.MsgTx().TxOut[0].Value < requiredStakeDifficulty {
			mp.removeTransaction(tx.Tx, true, isTreasuryEnabled)
		}
		if (txType == stake.TxTypeSSRtx || txType == stake.TxTypeSSGen) &&
			tx.Height+int64(heightDiffToPruneVotes) < height {
			mp.removeTransaction(tx.Tx, true, isTreasuryEnabled)
		}
	}
	for _, tx := range mp.staged {
		txType := stake.DetermineTxType(tx.MsgTx(), isTreasuryEnabled)
		if txType == stake.TxTypeSStx &&
			tx.MsgTx().TxOut[0].Value < requiredStakeDifficulty {
			log.Debugf("Pruning ticket %v with insufficient stake difficulty "+
				"from stage pool", tx.Hash())
			mp.removeStagedTransaction(tx)
		}
	}
}

// pruneExpiredTx prunes expired transactions from the mempool that are no
// longer able to be included into a block.
//
// This function MUST be called with the mempool lock held (for writes).
func (mp *TxPool) pruneExpiredTx(isTreasuryEnabled bool) {
	nextBlockHeight := mp.cfg.BestHeight() + 1

	for _, tx := range mp.pool {
		if blockchain.IsExpired(tx.Tx, nextBlockHeight) {
			log.Debugf("Pruning expired transaction %v from the mempool",
				tx.Tx.Hash())
			mp.removeTransaction(tx.Tx, true, isTreasuryEnabled)
		}
	}

	for _, tx := range mp.staged {
		if blockchain.IsExpired(tx, nextBlockHeight) {
			log.Debugf("Pruning expired transaction %v from the stage pool",
				tx.Hash())
			mp.removeStagedTransaction(tx)
		}
	}
}

// PruneExpiredTx prunes expired transactions from the mempool that may no longer
// be able to be included into a block.
//
// This function is safe for concurrent access.
func (mp *TxPool) PruneExpiredTx() {
	isTreasuryEnabled, err := mp.cfg.IsTreasuryAgendaActive()
	if err != nil {
		return
	}

	// Protect concurrent access.
	mp.mtx.Lock()
	mp.pruneExpiredTx(isTreasuryEnabled)
	mp.mtx.Unlock()
}

// ProcessOrphans determines if there are any orphans which depend on the passed
// transaction hash (it is possible that they are no longer orphans) and
// potentially accepts them to the memory pool.  It repeats the process for the
// newly accepted transactions (to detect further orphans which may no longer be
// orphans) until there are no more.
//
// It returns a slice of transactions added to the mempool.  A nil slice means
// no transactions were moved from the orphan pool to the mempool.
//
// This function is safe for concurrent access.
func (mp *TxPool) ProcessOrphans(acceptedTx *dcrutil.Tx, isTreasuryEnabled bool) []*dcrutil.Tx {
	mp.mtx.Lock()
	acceptedTxns := mp.processOrphans(acceptedTx, isTreasuryEnabled)
	mp.mtx.Unlock()
	return acceptedTxns
}

// ProcessTransaction is the main workhorse for handling insertion of new
// free-standing transactions into the memory pool.  It includes functionality
// such as rejecting duplicate transactions, ensuring transactions follow all
// rules, orphan transaction handling, and insertion into the memory pool.
//
// It returns a slice of transactions added to the mempool.  When the
// error is nil, the list will include the passed transaction itself along
// with any additional orphan transactions that were added as a result of the
// passed one being accepted.
//
// This function is safe for concurrent access.
func (mp *TxPool) ProcessTransaction(tx *dcrutil.Tx, allowOrphan, rateLimit, allowHighFees bool, tag Tag) ([]*dcrutil.Tx, error) {
	isTreasuryEnabled, err := mp.cfg.IsTreasuryAgendaActive()
	if err != nil {
		return nil, err
	}

	// Protect concurrent access.
	mp.mtx.Lock()
	defer mp.mtx.Unlock()
	defer func() {
		if err != nil {
			log.Tracef("Failed to process transaction %v: %s",
				tx.Hash(), err.Error())
		}
	}()

	// Potentially accept the transaction to the memory pool.
	missingParents, err := mp.maybeAcceptTransaction(tx, true, rateLimit,
		allowHighFees, true, isTreasuryEnabled)
	if err != nil {
		return nil, err
	}

	// If len(missingParents) == 0 then we know the tx is NOT an orphan.
	if len(missingParents) == 0 {
		// Accept any orphan transactions that depend on this
		// transaction (they may no longer be orphans if all inputs
		// are now available) and repeat for those accepted
		// transactions until there are no more.
		newTxs := mp.processOrphans(tx, isTreasuryEnabled)
		acceptedTxs := make([]*dcrutil.Tx, len(newTxs)+1)

		// Add the parent transaction first so remote nodes
		// do not add orphans.
		acceptedTxs[0] = tx
		copy(acceptedTxs[1:], newTxs)

		return acceptedTxs, nil
	}

	// The transaction is an orphan (has inputs missing).  Reject
	// it if the flag to allow orphans is not set.
	if !allowOrphan {
		// Only use the first missing parent transaction in
		// the error message.
		//
		// NOTE: RejectDuplicate is really not an accurate
		// reject code here, but it matches the reference
		// implementation and there isn't a better choice due
		// to the limited number of reject codes.  Missing
		// inputs is assumed to mean they are already spent
		// which is not really always the case.
		str := fmt.Sprintf("orphan transaction %v references "+
			"outputs of unknown or fully-spent "+
			"transaction %v", tx.Hash(), missingParents[0])
		return nil, txRuleError(ErrOrphan, str)
	}

	// Potentially add the orphan transaction to the orphan pool.
	err = mp.maybeAddOrphan(tx, tag, isTreasuryEnabled)
	return nil, err
}

// Count returns the number of transactions in the main pool.  It does not
// include the orphan pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) Count() int {
	mp.mtx.RLock()
	count := len(mp.pool)
	mp.mtx.RUnlock()

	return count
}

// TxHashes returns a slice of hashes for all of the transactions in the memory
// pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) TxHashes() []*chainhash.Hash {
	mp.mtx.RLock()
	hashes := make([]*chainhash.Hash, len(mp.pool))
	i := 0
	for hash := range mp.pool {
		hashCopy := hash
		hashes[i] = &hashCopy
		i++
	}
	mp.mtx.RUnlock()

	return hashes
}

// TxDescs returns a slice of descriptors for all the transactions in the pool.
// The descriptors must be treated as read only.
//
// This function is safe for concurrent access.
func (mp *TxPool) TxDescs() []*TxDesc {
	mp.mtx.RLock()
	descs := make([]*TxDesc, len(mp.pool))
	i := 0
	for _, desc := range mp.pool {
		descs[i] = desc
		i++
	}
	mp.mtx.RUnlock()

	return descs
}

// VerboseTxDescs returns a slice of verbose descriptors for all the
// transactions in the pool.  The descriptors must be treated as read only.
//
// Callers should prefer working with the more efficient TxDescs unless they
// specifically need access to the additional details provided.
//
// This function is safe for concurrent access.
func (mp *TxPool) VerboseTxDescs() []*VerboseTxDesc {
	isTreasuryEnabled, err := mp.cfg.IsTreasuryAgendaActive()
	if err != nil {
		return nil
	}

	mp.mtx.RLock()
	defer mp.mtx.RUnlock()

	result := make([]*VerboseTxDesc, 0, len(mp.pool))
	bestHeight := mp.cfg.BestHeight()

	for _, desc := range mp.pool {
		// Calculate the current priority based on inputs to the transaction.
		// Use zero if one or more of the input transactions can't be found for
		// some reason.
		tx := desc.Tx
		var currentPriority float64
		utxos, err := mp.fetchInputUtxos(tx, isTreasuryEnabled)
		if err == nil {
			currentPriority = mining.CalcPriority(tx.MsgTx(), utxos,
				bestHeight+1)
		}

		// Create the descriptor and add dependencies as needed.
		vtxd := &VerboseTxDesc{
			TxDesc:          *desc,
			CurrentPriority: currentPriority,
		}
		for _, txIn := range tx.MsgTx().TxIn {
			hash := &txIn.PreviousOutPoint.Hash
			if depDesc, ok := mp.pool[*hash]; ok {
				vtxd.Depends = append(vtxd.Depends, depDesc)
			}
		}

		result = append(result, vtxd)
	}

	return result
}

// miningDescs returns a slice of mining descriptors for all transactions
// in the pool.
//
// This function MUST be called with the mempool lock held (for reads).
func (mp *TxPool) miningDescs() []*mining.TxDesc {
	descs := make([]*mining.TxDesc, len(mp.pool))
	i := 0
	for _, desc := range mp.pool {
		descs[i] = &desc.TxDesc
		i++
	}

	return descs
}

// LastUpdated returns the last time a transaction was added to or removed from
// the main pool.  It does not include the orphan pool.
//
// This function is safe for concurrent access.
func (mp *TxPool) LastUpdated() time.Time {
	return time.Unix(atomic.LoadInt64(&mp.lastUpdated), 0)
}

// MiningView returns a slice of mining descriptors for all the transactions
// in the pool in addition to a snapshot of the current pool's transaction
// relationships.
//
// This is part of the mining.TxSource interface implementation and is safe for
// concurrent access as required by the interface contract.
func (mp *TxPool) MiningView() *mining.TxMiningView {
	mp.mtx.RLock()
	view := mp.miningView.Clone(mp.miningDescs(), mp.findTx)
	mp.mtx.RUnlock()
	return view
}

// New returns a new memory pool for validating and storing standalone
// transactions until they are mined into a block.
func New(cfg *Config) *TxPool {
	mp := &TxPool{
		cfg:             *cfg,
		pool:            make(map[chainhash.Hash]*TxDesc),
		orphans:         make(map[chainhash.Hash]*orphanTx),
		orphansByPrev:   make(map[wire.OutPoint]map[chainhash.Hash]*dcrutil.Tx),
		outpoints:       make(map[wire.OutPoint]*dcrutil.Tx),
		votes:           make(map[chainhash.Hash][]mining.VoteDesc),
		tspends:         make(map[chainhash.Hash]*dcrutil.Tx),
		nextExpireScan:  time.Now().Add(orphanExpireScanInterval),
		staged:          make(map[chainhash.Hash]*dcrutil.Tx),
		stagedOutpoints: make(map[wire.OutPoint]*dcrutil.Tx),
	}

	// for a given transaction, scan the mempool to find which transactions
	// spend it.
	forEachRedeemer := func(tx *dcrutil.Tx, f func(redeemerTx *mining.TxDesc)) {
		prevOut := wire.OutPoint{Hash: *tx.Hash(), Tree: tx.Tree()}
		txOutLen := uint32(len(tx.MsgTx().TxOut))
		for i := uint32(0); i < txOutLen; i++ {
			prevOut.Index = i
			if txRedeemer, exists := mp.outpoints[prevOut]; exists {
				f(&mp.pool[txRedeemer.MsgTx().TxHash()].TxDesc)
			}
		}
	}

	mp.miningView = mining.NewTxMiningView(cfg.Policy.EnableAncestorTracking,
		forEachRedeemer)

	return mp
}
