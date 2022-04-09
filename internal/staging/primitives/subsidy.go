// Copyright (c) 2015-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package primitives

import (
	"sort"
	"sync"
)

// -----------------------------------------------------------------------------
// Maintainer note:
//
// Since this code is part of the primitives module, major module version bumps
// should be avoided at nearly all costs given the headache major module version
// bumps typically cause consumers.
//
// Although changes to the subsidy are not expected, in light of DCP0010, they
// should not entirely be ruled out.  Therefore, the following discusses the
// preferred approach for introducing new subsidy calculation semantics while
// achieving the goal of avoiding major module version bumps.
//
// Any modifications to the subsidy calculations that involve a consensus change
// will necessarily require some mechanism to allow calculating both the
// historical values and the new values according to new semantics defined by an
// associated DCP.  While not the only possible approach, a good approach is
// typically a flag to specify when the agenda is active.  In the case multiple
// booleans are required, bit flags should be preferred instead.  However, keep
// in mind that adding a new parameter to a method is a major breaking change to
// the API.
//
// Further, ideally, callers should NOT be required to use an older version of
// the cache to calculate values according to those older semantics since that
// would be cumbersome to use for callers as it would require them to keep
// multiple versions of the cache around.  It also would be less efficient
// because cache reuse would not be possible.
//
// Keeping the aforementioned in mind, the preferred way to introduce any such
// changes is to introduce a new type for the cache, such as SubsidyCacheV2,
// with the relevant calculation funcs defined on the new type updated to accept
// the new flags so that the updated version cache can be used for both whatever
// the new semantics are as well as all historical semantics.
//
// Since the cache only stores the full block subsidies prior to any splits or
// reductions, so long as the consensus change does not involve modifying the
// full block subsidies, using the same new version of a cache with flags for
// different splits and/or reduction semantics is straightforward.  On the other
// hand, supporting a consensus change that involves modifying the full block
// subsidies requires greater care, but can still be done efficiently by storing
// additional information regarding which semantics were used to generate the
// cached entries and reacting accordingly.
//
// In the case there are new per-network parameters that must be accepted, a new
// type for the params, such as SubsidyParamsV2 should be introduced and passed
// to the constructor of the new cache type (e.g. NewSubsidyCacheV2) given
// modification of the existing params interface (including adding new methods)
// would be a major breaking change to the API.  Note that the new version of
// the parameters will need all of the existing parameters as well, so that can
// be achieved by embedding the existing params interface in the new one.
//
// Finally, the existing type and methods (e.g. SubsidyCache and all calculation
// methods it provides) that are now handled by the newly introduced versions
// should then be marked as deprecated with a message instructing the caller to
// use the newest version of them instead.
// -----------------------------------------------------------------------------

// SubsidyParams defines an interface that is used to provide the parameters
// required when calculating block and vote subsidies.  These values are
// typically well-defined and unique per network.
type SubsidyParams interface {
	// BlockOneSubsidy returns the total subsidy of block height 1 for the
	// network.  This is separate since it encompasses the initial coin
	// distribution.
	BlockOneSubsidy() int64

	// BaseSubsidyValue returns the starting base max potential subsidy amount
	// for mined blocks.  This value is reduced over time and then split
	// proportionally between PoW, PoS, and the Treasury.  The reduction is
	// controlled by the SubsidyReductionInterval, SubsidyReductionMultiplier,
	// and SubsidyReductionDivisor parameters.
	//
	// NOTE: This must be a max of 140,739,635,871,744 atoms or incorrect
	// results will occur due to int64 overflow.  This value comes from
	// MaxInt64/MaxUint16 = (2^63 - 1)/(2^16 - 1) = 2^47 + 2^31 + 2^15.
	BaseSubsidyValue() int64

	// SubsidyReductionMultiplier returns the multiplier to use when performing
	// the exponential subsidy reduction described by the CalcBlockSubsidy
	// documentation.
	SubsidyReductionMultiplier() int64

	// SubsidyReductionDivisor returns the divisor to use when performing the
	// exponential subsidy reduction described by the CalcBlockSubsidy
	// documentation.
	SubsidyReductionDivisor() int64

	// SubsidyReductionIntervalBlocks returns the reduction interval in number
	// of blocks.
	SubsidyReductionIntervalBlocks() int64

	// StakeValidationBeginHeight returns the height at which votes become
	// required to extend a block.  This height is the first that will be voted
	// on, but will not include any votes itself.
	StakeValidationBeginHeight() int64

	// VotesPerBlock returns the maximum number of votes a block must contain to
	// receive full subsidy once voting begins at StakeValidationBeginHeight.
	VotesPerBlock() uint16
}

// totalProportions is the sum of the proportions for the subsidy split between
// PoW, PoS, and the Treasury.
//
// This value comes from the fact that the subsidy split is as follows:
//
// Prior to DCP0010: PoW: 6, PoS: 3, Treasury: 1 => 6+3+1 = 10
//    After DCP0010: PoW: 1, PoS: 8, Treasury: 1 => 1+8+1 = 10
//
// Therefore, the subsidy split percentages are:
//
// Prior to DCP0010: PoW: 6/10 = 60%, PoS: 3/10 = 30%, Treasury: 1/10 = 10%
//    After DCP0010: PoW: 1/10 = 10%, PoS: 8/10 = 80%, Treasury: 1/10 = 10%
const totalProportions = 10

// SubsidyCache provides efficient access to consensus-critical subsidy
// calculations for blocks and votes, including the max potential subsidy for
// given block heights, the proportional proof-of-work subsidy, the proportional
// proof of stake per-vote subsidy, and the proportional treasury subsidy.
//
// It makes use of caching to avoid repeated calculations.
type SubsidyCache struct {
	// This mutex protects the following fields:
	//
	// cache houses the cached subsidies keyed by reduction interval.
	//
	// cachedIntervals contains an ordered list of all cached intervals.  It is
	// used to efficiently track sparsely cached intervals with O(log N)
	// discovery of a prior cached interval.
	mtx             sync.RWMutex
	cache           map[uint64]int64
	cachedIntervals []uint64

	// params stores the subsidy parameters to use during subsidy calculation.
	params SubsidyParams

	// minVotesRequired is the minimum number of votes required for a block to
	// be considered valid by consensus.  It is calculated from the parameters
	// at initialization time and stored in order to avoid repeated calculation.
	minVotesRequired uint16
}

// NewSubsidyCache creates and initializes a new subsidy cache instance.  See
// the SubsidyCache documentation for more details.
func NewSubsidyCache(params SubsidyParams) *SubsidyCache {
	// Initialize the cache with the first interval set to the base subsidy and
	// enough initial space for a few sparse entries for typical usage patterns.
	const prealloc = 5
	baseSubsidy := params.BaseSubsidyValue()
	cache := make(map[uint64]int64, prealloc)
	cache[0] = baseSubsidy

	return &SubsidyCache{
		cache:            cache,
		cachedIntervals:  make([]uint64, 1, prealloc),
		params:           params,
		minVotesRequired: (params.VotesPerBlock() / 2) + 1,
	}
}

// uint64s implements sort.Interface for *[]uint64.
type uint64s []uint64

func (s *uint64s) Len() int           { return len(*s) }
func (s *uint64s) Less(i, j int) bool { return (*s)[i] < (*s)[j] }
func (s *uint64s) Swap(i, j int)      { (*s)[i], (*s)[j] = (*s)[j], (*s)[i] }

// CalcBlockSubsidy returns the max potential subsidy for a block at the
// provided height.  This value is reduced over time based on the height and
// then split proportionally between PoW, PoS, and the Treasury.
//
// Subsidy calculation for exponential reductions:
//
//  subsidy := BaseSubsidyValue()
//  for i := 0; i < (height / SubsidyReductionIntervalBlocks()); i++ {
//    subsidy *= SubsidyReductionMultiplier()
//    subsidy /= SubsidyReductionDivisor()
//  }
//
// This function is safe for concurrent access.
func (c *SubsidyCache) CalcBlockSubsidy(height int64) int64 {
	// Negative block heights are invalid and produce no subsidy.
	// Block 0 is the genesis block and produces no subsidy.
	// Block 1 subsidy is special as it is used for initial token distribution.
	switch {
	case height <= 0:
		return 0
	case height == 1:
		return c.params.BlockOneSubsidy()
	}

	// Calculate the reduction interval associated with the requested height and
	// attempt to look it up in cache.  When it's not in the cache, look up the
	// latest cached interval and subsidy while the mutex is still held for use
	// below.
	reqInterval := uint64(height / c.params.SubsidyReductionIntervalBlocks())
	c.mtx.RLock()
	if cachedSubsidy, ok := c.cache[reqInterval]; ok {
		c.mtx.RUnlock()
		return cachedSubsidy
	}
	lastCachedInterval := c.cachedIntervals[len(c.cachedIntervals)-1]
	lastCachedSubsidy := c.cache[lastCachedInterval]
	c.mtx.RUnlock()

	// When the requested interval is after the latest cached interval, avoid
	// additional work by either determining if the subsidy is already exhausted
	// at that interval or using the interval as a starting point to calculate
	// and store the subsidy for the requested interval.
	//
	// Otherwise, the requested interval is prior to the final cached interval,
	// so use a binary search to find the latest cached interval prior to the
	// requested one and use it as a starting point to calculate and store the
	// subsidy for the requested interval.
	if reqInterval > lastCachedInterval {
		// Return zero for all intervals after the subsidy reaches zero.  This
		// enforces an upper bound on the number of entries in the cache.
		if lastCachedSubsidy == 0 {
			return 0
		}
	} else {
		c.mtx.RLock()
		cachedIdx := sort.Search(len(c.cachedIntervals), func(i int) bool {
			return c.cachedIntervals[i] >= reqInterval
		})
		lastCachedInterval = c.cachedIntervals[cachedIdx-1]
		lastCachedSubsidy = c.cache[lastCachedInterval]
		c.mtx.RUnlock()
	}

	// Finally, calculate the subsidy by applying the appropriate number of
	// reductions per the starting and requested interval.
	reductionMultiplier := c.params.SubsidyReductionMultiplier()
	reductionDivisor := c.params.SubsidyReductionDivisor()
	subsidy := lastCachedSubsidy
	neededIntervals := reqInterval - lastCachedInterval
	for i := uint64(0); i < neededIntervals; i++ {
		subsidy *= reductionMultiplier
		subsidy /= reductionDivisor

		// Stop once no further reduction is possible.  This ensures a bounded
		// computation for large requested intervals and that all future
		// requests for intervals at or after the final reduction interval
		// return 0 without recalculating.
		if subsidy == 0 {
			reqInterval = lastCachedInterval + i + 1
			break
		}
	}

	// Update the cache for the requested interval or the interval in which the
	// subsidy became zero when applicable.  The cached intervals are stored in
	// a map for O(1) lookup and also tracked via a sorted array to support the
	// binary searches for efficient sparse interval query support.
	c.mtx.Lock()
	c.cache[reqInterval] = subsidy
	c.cachedIntervals = append(c.cachedIntervals, reqInterval)
	sort.Sort((*uint64s)(&c.cachedIntervals))
	c.mtx.Unlock()
	return subsidy
}

// calcWorkSubsidy returns the proof of work subsidy for a block for a given
// number of votes using the provided work subsidy proportion which is expected
// to be an integer such that it produces the desired percentage when divided by
// 10.  For example, a proportion of 6 will result in 6/10 = 60%.
//
// The calculated subsidy is further reduced proportionally depending on the
// number of votes once the height at which voting begins has been reached.
//
// Note that passing a number of voters fewer than the minimum required for a
// block to be valid by consensus along with a height greater than or equal to
// the height at which voting begins will return zero.
//
// This is the primary implementation logic used by CalcWorkSubsidy and only
// differs in that the proportion is parameterized.
//
// This function is safe for concurrent access.
func (c *SubsidyCache) calcWorkSubsidy(height int64, voters uint16, proportion uint16) int64 {
	// The first block has special subsidy rules.
	if height == 1 {
		return c.params.BlockOneSubsidy()
	}

	// The subsidy is zero if there are not enough voters once voting begins.  A
	// block without enough voters will fail to validate anyway.
	stakeValidationHeight := c.params.StakeValidationBeginHeight()
	if height >= stakeValidationHeight && voters < c.minVotesRequired {
		return 0
	}

	// Calculate the full block subsidy and reduce it according to the PoW
	// proportion.
	subsidy := c.CalcBlockSubsidy(height)
	subsidy *= int64(proportion)
	subsidy /= int64(totalProportions)

	// Ignore any potential subsidy reductions due to the number of votes prior
	// to the point voting begins.
	if height < stakeValidationHeight {
		return subsidy
	}

	// Adjust for the number of voters.
	return (int64(voters) * subsidy) / int64(c.params.VotesPerBlock())
}

// CalcWorkSubsidy returns the proof of work subsidy for a block for a given
// number of votes using either the original subsidy split that was in effect at
// Decred launch or the modified subsidy split defined in DCP0010 according to
// the provided flag.
//
// It is calculated as a proportion of the total subsidy and further reduced
// proportionally depending on the number of votes once the height at which
// voting begins has been reached.
//
// Note that passing a number of voters fewer than the minimum required for a
// block to be valid by consensus along with a height greater than or equal to
// the height at which voting begins will return zero.
//
// This function is safe for concurrent access.
func (c *SubsidyCache) CalcWorkSubsidy(height int64, voters uint16, useDCP0010 bool) int64 {
	if !useDCP0010 {
		// The work subsidy proportion prior to DCP0010 is 60%.  Thus it is 6
		// since 6/10 = 60%.
		const workSubsidyProportion = 6
		return c.calcWorkSubsidy(height, voters, workSubsidyProportion)
	}

	// The work subsidy proportion defined in DCP0010 is 10%.  Thus it is 1
	// since 1/10 = 10%.
	const workSubsidyProportion = 1
	return c.calcWorkSubsidy(height, voters, workSubsidyProportion)
}

// calcStakeVoteSubsidy returns the subsidy for a single stake vote for a block
// using the provided stake vote subsidy proportion which is expected to be a
// value in the range [1,10] such that it produces the desired percentage when
// divided by 10.  For example, a proportion of 3 will result in 3/10 = 30%.
//
// It is calculated as the provided proportion of the total subsidy and max
// potential number of votes per block.
//
// Unlike the Proof-of-Work subsidy, the subsidy that votes receive is not
// reduced when a block contains less than the maximum number of votes.
// Consequently, this does not accept the number of votes.  However, it is
// important to note that blocks that do not receive the minimum required number
// of votes for a block to be valid by consensus won't actually produce any vote
// subsidy either since they are invalid.
//
// This is the primary implementation logic used by CalcStakeVoteSubsidy and
// only differs in that the proportion is parameterized.
//
// This function is safe for concurrent access.
func (c *SubsidyCache) calcStakeVoteSubsidy(height int64, proportion uint16) int64 {
	// Votes have no subsidy prior to the point voting begins.  The minus one
	// accounts for the fact that vote subsidy are, unfortunately, based on the
	// height that is being voted on as opposed to the block in which they are
	// included.
	if height < c.params.StakeValidationBeginHeight()-1 {
		return 0
	}

	// Calculate the full block subsidy and reduce it according to the stake
	// proportion.  Then divide it by the number of votes per block to arrive
	// at the amount per vote.
	subsidy := c.CalcBlockSubsidy(height)
	subsidy *= int64(proportion)
	subsidy /= (totalProportions * int64(c.params.VotesPerBlock()))

	return subsidy
}

// CalcStakeVoteSubsidy returns the subsidy for a single stake vote for a block
// using either the original subsidy split that was in effect at Decred launch
// or the modified subsidy split defined in DCP0010 according to the provided
// flag.
//
// It is calculated as a proportion of the total subsidy and max potential
// number of votes per block.
//
// Unlike the Proof-of-Work subsidy, the subsidy that votes receive is not
// reduced when a block contains less than the maximum number of votes.
// Consequently, this does not accept the number of votes.  However, it is
// important to note that blocks that do not receive the minimum required number
// of votes for a block to be valid by consensus won't actually produce any vote
// subsidy either since they are invalid.
//
// This function is safe for concurrent access.
func (c *SubsidyCache) CalcStakeVoteSubsidy(height int64, useDCP0010 bool) int64 {
	if !useDCP0010 {
		// The vote subsidy proportion prior to DCP0010 is 30%.  Thus it is 3
		// since 3/10 = 30%.
		const voteSubsidyProportion = 3
		return c.calcStakeVoteSubsidy(height, voteSubsidyProportion)
	}

	// The stake vote subsidy proportion defined in DCP0010 is 80%.  Thus it is
	// 8 since 8/10 = 80%.
	const voteSubsidyProportion = 8
	return c.calcStakeVoteSubsidy(height, voteSubsidyProportion)
}

// CalcTreasurySubsidy returns the subsidy required to go to the treasury for
// a block.  It is calculated as a proportion of the total subsidy and further
// reduced proportionally depending on the number of votes once the height at
// which voting begins has been reached.
//
// Note that passing a number of voters fewer than the minimum required for a
// block to be valid by consensus along with a height greater than or equal to
// the height at which voting begins will return zero.
//
// When the treasury agenda is active the subsidy rule changes from paying out
// a proportion based on the number of votes to always pay the full subsidy.
//
// This function is safe for concurrent access.
func (c *SubsidyCache) CalcTreasurySubsidy(height int64, voters uint16, useDCP0006 bool) int64 {
	// The first two blocks have special subsidy rules.
	if height <= 1 {
		return 0
	}

	// The subsidy is zero if there are not enough voters once voting
	// begins.  A block without enough voters will fail to validate anyway.
	stakeValidationHeight := c.params.StakeValidationBeginHeight()
	if height >= stakeValidationHeight && voters < c.minVotesRequired {
		return 0
	}

	// Calculate the full block subsidy and reduce it according to the treasury
	// proportion.
	//
	// The treasury proportion is 10% both prior to DCP0010 and after it.  Thus,
	// it is 1 since 1/10 = 10%.
	//
	// However, there is no need to multiply by 1 since the result is the same.
	subsidy := c.CalcBlockSubsidy(height)
	subsidy /= totalProportions

	// Ignore any potential subsidy reductions due to the number of votes
	// prior to the point voting begins or when DCP0006 is active.
	if height < stakeValidationHeight || useDCP0006 {
		return subsidy
	}

	// Adjust for the number of voters.
	return (int64(voters) * subsidy) / int64(c.params.VotesPerBlock())
}
