// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:generate go run generateapbftable.go

// Package apbf implements an optimized Age-Partitioned Bloom Filter.
package apbf

import (
	"encoding/binary"
	"math"
	"math/bits"
	"sync"
	"time"

	"github.com/dchest/siphash"
)

// References:
//   [APBF] Age-Partitioned Bloom Filters (Shtul, Baquero, Almeida)
//     https://arxiv.org/pdf/2001.03147.pdf
//
//   [LHSP] Less Hashing, Same Performance: Building a Better Bloom Filter
//      (Kirsch, Mitzenmacher)
//
//   [BFPV] Bloom Filters in Probabilistic Verification (Dillinger, Manolis)

// calcFPRateKey is used as the key when caching results for the false positive
// calculations.
type calcFPRateKey struct {
	a, i uint16
}

// calcFPRateInternal calculates and returns the false positive rate for the
// provided parameters using the given results map to cache intermediate
// results.
func calcFPRateInternal(results map[calcFPRateKey]float64, k, l uint8, a, i uint16) float64 {
	// The false positive rate is calculated according to the following
	// recursively-defined function provided in [APBF]:
	//
	//              {1                                        , if a = k
	// F(k,l,a,i) = {0                                        , if i > l + a
	//              {(r_i)F(k,l,a+1,i+1) + (1-r_i)F(k,l,0,i+1), otherwise

	if a == uint16(k) {
		return 1
	} else if i > uint16(l)+a {
		return 0
	}

	// Return stored results to avoid a bunch of duplicate work.
	resultKey := calcFPRateKey{a, i}
	if result, ok := results[resultKey]; ok {
		return result
	}

	// Calculate the fill ratio for the slice.
	fillRatio := float64(0.5)
	if i < uint16(k) {
		fillRatio = float64(i+1) / float64(2*k)
	}

	firstTerm := fillRatio * calcFPRateInternal(results, k, l, a+1, i+1)
	secondTerm := (1 - fillRatio) * calcFPRateInternal(results, k, l, 0, i+1)
	result := firstTerm + secondTerm
	results[resultKey] = result
	return result
}

// CalcFPRate calculates and returns the false positive rate for an APBF
// created with the given parameters.
//
// NOTE: This involves allocations, so the result should be cached by the caller
// if it intends to use it multiple times.
//
// This function is safe for concurrent access.
func CalcFPRate(k, l uint8) float64 {
	results := make(map[calcFPRateKey]float64, 2*uint16(k)*uint16(l))
	return calcFPRateInternal(results, k, l, 0, 0)
}

// Filter implements an Age-Partitioned Bloom Filter (APBF) that is safe for
// concurrent access.
//
// An APBF is a probabilistic data structure suitable for use in processing
// unbounded data streams where more recent items are more significant than
// older ones and some false positives are acceptable.  It has similar
// computational costs as traditional Bloom filters and provides space
// efficiency that is competitive with the current best-known, and more complex,
// Dictionary approaches.
//
// Similar to classic Bloom filters, APBFs have a non-zero probability of false
// positives that can be tuned via parameters and are free from false negatives
// up to the capacity of the filter.  However, unlike classic Bloom filters,
// where the false positive rate climbs as items are added until all queries are
// a false positive, APBFs provide a configurable upper bound on the false
// positive rate for an unbounded number of additions.
//
// The unbounded property is achieved by adding and retiring disjoint segments
// over time where each segment is a slice of a partitioned Bloom filter.  The
// slices are conceptually aged over time as new items are added by shifting
// them and discarding the oldest one.
//
// See the NewFilter documentation for details regarding tuning parameters and
// expected approximate total space usage.
type Filter struct {
	// k is the number of slices which need consecutive matches for an item to
	// be considered in the filter.  This is similar to the k parameter of
	// static bloom filters which use exactly k hash functions for k different
	// slices, except here it is the number of consecutive matches in a larger
	// sequence of distinct slices determined by this parameter along with the l
	// parameter below to preserve the false positive rate.
	k uint8

	// l is the additional number of slices which comprise the region of items
	// that are transitioning to expired.
	//
	// The sum of this parameter and k above determines the total number of
	// slices, which in turn determines the false positive rate and also
	// influences the filter capacity.
	l uint8

	// kPlusL is simply the sum of the k and l parameters and is stored to avoid
	// calculating it repeatedly.  It is the total number of slices.
	kPlusL uint16

	// bitsPerSlice is the number of physical bits occupied by each slice that
	// comprise the overall filter.
	bitsPerSlice uint64

	// itemsPerGeneration is the number of items per generation.  Conceptually,
	// the slices are aged every time this number of items is inserted into the
	// filter resulting in items inserted in older generations expiring.  In
	// other words, the items are effectively partitioned by age.
	itemsPerGeneration uint32

	// ****************************************************************
	// The fields below this point are protected by the embedded mutex.
	// ****************************************************************

	mtx sync.Mutex

	// key0 and key1 are used to seed the hash function in order to ensure
	// attackers are not able to intentionally grind false positives that would
	// otherwise affect the entire network if all nodes were calculating the
	// same hash values.
	key0, key1 uint64

	// itemsInCurGeneration is the number of items that have been added to the
	// current generation.  The slices are aged once the number of items per
	// generation is reached by conceptually shifting them to subsequent slices
	// and throwing away the overflow from the final slice.  In practice, the
	// shifting is done via logical shifts in a ring buffer by keeping track of
	// the index of the first logical slice (called the base index).
	itemsInCurGeneration uint32

	// baseIndex is the index of the position of the first slice within the ring
	// buffer.
	baseIndex uint16

	// data is the actual filter data implemented as a packed ring buffer where
	// the individual filter slices (partitions) perform logical shifts.
	data []byte
}

// Capacity returns the max number of items that were most recently added which
// are guaranteed to return true.  Adding more items than the returned value
// will cause the oldest items to be expired.
//
// This function is safe for concurrent access.
func (f *Filter) Capacity() uint32 {
	return uint32(f.l+1) * f.itemsPerGeneration
}

// FPRate calculates and returns the actual false positive rate for the filter.
//
// NOTE: This involves allocations, so the result should be cached by the caller
// if it intends to use it multiple times.
//
// This function is safe for concurrent access.
func (f *Filter) FPRate() float64 {
	return CalcFPRate(f.k, f.l)
}

// Size returns the total bytes occupied by the filter data plus overhead.
//
// This function is safe for concurrent access.
func (f *Filter) Size() int {
	const overhead = 70
	f.mtx.Lock()
	result := len(f.data) + overhead
	f.mtx.Unlock()
	return result
}

// K returns the filter configuration parameter for the number of slices that
// need consecutive matches.  It is one of the tunable parameters that determine
// the overall capacity and false positive rate of the filter.
//
// This function is safe for concurrent access.
func (f *Filter) K() uint8 {
	return f.k
}

// L returns the filter configuration parameter for the number of additional
// slices.  It is one of the tunable parameters that determine the overall
// capacity and false positive rate of the filter.
//
// This function is safe for concurrent access.
func (f *Filter) L() uint8 {
	return f.l
}

// nextGeneration transitions the filter to the next generation which
// effectively ages all items and potentially expires the oldest generation of
// items.
//
// This function MUST be called with the filter mutex held.
func (f *Filter) nextGeneration() {
	// Shift the position of the first slice within the ring buffer to the left
	// by one.
	if f.baseIndex == 0 {
		f.baseIndex = f.kPlusL
	}
	f.baseIndex--

	// Clear the bits associated with the new logical slice.  Note that since
	// the logical slice was just rotated once backwards around the ring buffer
	// above, this clears what was previously the oldest slice so the new
	// entries take its place.
	startBit := uint64(f.baseIndex) * f.bitsPerSlice
	endBit := startBit + f.bitsPerSlice

	// Clear bits up to the next byte boundary.
	byteIdx := startBit >> 3
	if bit := startBit & 7; bit != 0 {
		f.data[byteIdx] &= byte(1<<bit - 1)
		byteIdx++
	}

	// Clear full bytes in one fell swoop when possible.
	if endByteIdx := (endBit >> 3); endByteIdx > byteIdx {
		fullBytes := endByteIdx - byteIdx
		data := f.data[byteIdx : byteIdx+fullBytes]
		for i := range data {
			data[i] = 0
		}
		byteIdx += fullBytes
	}

	// Clear any remaining bits.
	if byteIdx < uint64(len(f.data)) {
		f.data[byteIdx] &^= byte(1<<(endBit&7) - 1)
	}

	f.itemsInCurGeneration = 0
}

// deriveIndex uses enhanced double hashing to calculate a unique index for the
// given logical slice number using the closed formula.  In order to support
// more efficient interleaving of the calculation while looping over several
// slices, it also returns the intermediate accumulator which can be fed into
// subsequent iterations of the algorithm to potentially provide significant
// speed improvements.
//
// Note that each slice conceptually uses an independent hash function to
// determine which bit to index.  However, rather than using the more
// traditional approach of a distinct hash function for each one, enhanced
// double hashing is used to effectively generate the necessary hash function
// because it is significantly faster while still coming quite close to the
// theoretical limit of two-index fingerprinting.
//
// It is defined as "f(i) = hash1 + i*hash2 + (i^3 - i)/6 (mod m)", where m is
// the number of bits to index, however, this function does not perform the
// modular reduction and instead leaves it up to the caller.
//
// It is also worth noting that enhanced double hashing is used in this
// implementation over the more common double hashing referenced in [APBF], and
// defined in [LHSP], which generates the necessary hash functions from a linear
// combination of the two independent hash functions as "f(i) = hash1 + i*hash2
// (mod m)" because, as pointed out in [BFPV], that construction results in
// imposing an observable accuracy limit since there are no restrictions placed
// on the second hash to ensure the resulting indices are (almost) all unique.
// Namely, when the second hash function produces 0, all indices are the same,
// and, similarly, when it produces a value that divides m, the number of
// indices probed will be the minimum of the number of slices and the produced
// value, which could be as small as 2.
//
// See section 5.2 of [BFPV] for further details, if desired.
func deriveIndex(slice uint16, hash1, hash2 uint64) (uint64, uint64) {
	s := uint64(slice)
	z := (s*s + s) / 2
	derivedIdx := hash1 + s*hash2 + (z*(s-1))/3
	return derivedIdx, hash2 + z
}

// setBit unconditionally sets the bit at the provided absolute index in the
// underlying filter data.
//
// This function MUST be called with the filter mutex held.
func (f *Filter) setBit(bit uint64) {
	f.data[bit>>3] |= 1 << (bit & 7)
}

// fastReduce calculates a mapping that is more or less equivalent to x mod N.
// However, instead of using a mod operation that can lead to slowness on many
// processors when not using a power of two due to unnecessary division, this
// uses a "multiply-and-shift" trick that eliminates all divisions as described
// in a blog post by Daniel Lemire, located at the following site at the time
// of this writing:
// https://lemire.me/blog/2016/06/27/a-fast-alternative-to-the-modulo-reduction/
//
// Since that link might disappear, the general idea is to multiply by N and
// shift right by log2(N).  Since N is a 64-bit integer in this case, it
// becomes:
//
// (x * N) / 2^64 == (x * N) >> 64
//
// This is a fair map since it maps integers in the range [0,2^64) to multiples
// of N in [0, N*2^64) and then divides by 2^64 to map all multiples of N in
// [0,2^64) to 0, all multiples of N in [2^64, 2*2^64) to 1, etc.  This results
// in either ceil(2^64/N) or floor(2^64/N) multiples of N.
func fastReduce(x, N uint64) uint64 {
	// This uses math/bits to perform the 128-bit multiplication as the compiler
	// will replace it with the relevant intrinsic on most architectures.
	//
	// The high 64 bits in a 128-bit product is the same as shifting the entire
	// product right by 64 bits.
	hi, _ := bits.Mul64(x, N)
	return hi
}

// Add inserts the provided data into the filter.
//
// This function is safe for concurrent access.
func (f *Filter) Add(data []byte) {
	// Transition the filter to the next generation when adding the new item
	// will exceed the number of items per generation.  This effectively ages
	// all existing items and potentially expires the oldest generation of
	// items.
	f.mtx.Lock()
	if f.itemsInCurGeneration == f.itemsPerGeneration {
		f.nextGeneration()
	}
	f.itemsInCurGeneration++

	// Set the relevant bits in the filter for 'k' slices, starting from the
	// first slice (slice 0), as determined by the equivalent of a separate hash
	// (index) function for each slice.  See the comments in deriveIndex for
	// more details about the enhanced double hashing technique used.
	//
	// Note that this implementation uses a ring buffer where the position index
	// of the first slice is modified to simulate shifting, so the code below
	// uses logical slices to map to the correct physical location within the
	// filter data.
	//
	// As a further optimization, rather than fully calculating each independent
	// bit index for each slice via the closed formula for enhanced double
	// hashing, this interleaves the calculation with the loop over the slices
	// which benchmarking shows is about 30% faster.
	logicalSlice := f.baseIndex
	sliceBitOffset := uint64(logicalSlice) * f.bitsPerSlice
	hash1, hash2 := siphash.Hash128(f.key0, f.key1, data)
	derivedIdx, acc := deriveIndex(logicalSlice, hash1, hash2)
	for i := uint8(0); i < f.k; i++ {
		f.setBit(sliceBitOffset + fastReduce(derivedIdx, f.bitsPerSlice))

		// Move to the next logical slice while wrapping around the ring buffer
		// if needed.
		logicalSlice++
		if logicalSlice == f.kPlusL {
			logicalSlice = 0
			sliceBitOffset = 0

			// Reset the derived bit index using enhanced double hashing
			// accordingly.
			derivedIdx, acc = hash1, hash2
			continue
		}
		sliceBitOffset += f.bitsPerSlice

		// Derive the next bit index using enhanced double hashing.
		derivedIdx += acc
		acc += uint64(logicalSlice)
	}
	f.mtx.Unlock()
}

// isBitSet returns whether or not the bit at the provided absolute index in the
// underlying filter data is set.
//
// This function MUST be called with the filter mutex held.
func (f *Filter) isBitSet(bit uint64) bool {
	return f.data[bit>>3]&(1<<(bit&7)) != 0
}

// Contains returns the result of a probabilistic membership test of the
// provided data such that there is a non-zero probability of false positives,
// per the false positive rate of the filter, and a zero probability of false
// negatives for the previous number of items supported by the max capacity of
// the filter.
//
// In other words, the most recent max capacity number of items added to the
// filter will always return true while items that were never added or have been
// expired will only report true with the false positive rate of the filter.
//
// This function is safe for concurrent access.
func (f *Filter) Contains(data []byte) bool {
	// Attempt to find the required 'k' consecutive matches using an algorithm
	// that reduces the average number of tests required.
	//
	// The algorithm works by choosing the starting position such that it leaves
	// exactly 'k' consecutive slices to be tested, accumulates the matching
	// sub sequences, and jumps backwards 'k' slices when a match fails.
	//
	// Recall that this implementation uses a ring buffer where the position
	// index of the first slice is modified to simulate shifting, so the code
	// below uses logical slices to map to the correct physical location within
	// the filter data.
	//
	// As a further optimization, similar to when the items are added, rather
	// than fully calculating each independent bit index for each slice via the
	// closed formula for enhanced double hashing, this interleaves the
	// calculation with the loop over the slices for consecutive matches which
	// benchmarking shows is about 12% faster on average.
	var result = false
	f.mtx.Lock()
	prevMatches, curMatches := uint8(0), uint8(0)
	physicalSlice := uint16(f.l)
	logicalSlice := (f.baseIndex + physicalSlice) % f.kPlusL
	sliceBitOffset := uint64(logicalSlice) * f.bitsPerSlice
	hash1, hash2 := siphash.Hash128(f.key0, f.key1, data)
	derivedIdx, acc := deriveIndex(logicalSlice, hash1, hash2)
	for {
		if f.isBitSet(sliceBitOffset + fastReduce(derivedIdx, f.bitsPerSlice)) {
			// Successful query when the required number of consecutive matches
			// is achieved.
			curMatches++
			if prevMatches+curMatches == f.k {
				result = true
				break
			}

			// Move to the next logical slice while wrapping around the ring
			// buffer if needed.
			physicalSlice++
			logicalSlice++
			if logicalSlice == f.kPlusL {
				logicalSlice = 0
				sliceBitOffset = 0

				// Reset the derived bit index using enhanced double hashing
				// accordingly.
				derivedIdx, acc = hash1, hash2
				continue
			}
			sliceBitOffset += f.bitsPerSlice

			// Derive the next bit index using enhanced double hashing.
			derivedIdx += acc
			acc += uint64(logicalSlice)
			continue
		}

		// Nothing more to do when there are not enough slices left to achieve
		// the required number of consecutive matches.
		if uint16(f.k) > physicalSlice {
			break
		}

		// Skip back the required number of matches while accumulating any
		// matching sub sequence.
		physicalSlice -= uint16(f.k)
		prevMatches = curMatches
		curMatches = 0

		// Reset logical slice and derive the associated bit index using
		// enhanced double hashing.
		logicalSlice = (f.baseIndex + physicalSlice) % f.kPlusL
		sliceBitOffset = uint64(logicalSlice) * f.bitsPerSlice
		derivedIdx, acc = deriveIndex(logicalSlice, hash1, hash2)
	}
	f.mtx.Unlock()

	return result
}

// Reset clears the filter and changes the key used in the internal hashing
// logic to ensure a unique set of false positives versus those prior to the
// reset.
//
// This function is safe for concurrent access.
func (f *Filter) Reset() {
	f.mtx.Lock()
	f.key0, f.key1 = siphash.Hash128(f.key0, f.key1, []byte("reset"))
	f.baseIndex = 0
	f.itemsInCurGeneration = 0
	for i := range f.data {
		f.data[i] = 0
	}
	f.mtx.Unlock()
}

// NewFilterKL returns an Age-Partitioned Bloom Filter (APBF) suitable for use
// in processing unbounded data streams where more recent items are more
// significant than older ones and some false positives are acceptable.  The
// parameters specify the number of most-recently added items that must always
// return true, the number slices which need consecutive matches, k, and the
// number of additional slices, l, to reach a desired target false positive rate
// to maintain.
//
// Every new filter uses a unique key for the internal hashing logic so that
// each one will have a unique set of false positives.  The key is also
// automatically changed by Reset.
//
// Applications might prefer using NewFilter to specify the max capacity and a
// target false positive rate unless they specifically require the additional
// fine tuning provided here.
//
// For convenience, the 'go generate' command may be used in the code directory
// to generate a table of k and l combinations along with the false positive
// rate they maintain and average expected number of accesses for false queries
// to help select appropriate parameters.
//
// Note that, due to rounding, the actual max number of items that can be added
// to the filter before old entries are expired might actually be slightly
// higher than the specified target and can be obtained via the Capacity method
// on the returned filter.
//
// The total space (in bytes) used by a filter that can hold 'n' items for the
// given 'k' and 'l' is approximately:
//
//	ceil(ceil(1.44k * ceil(n / l+1)) * (k+l) / 8)
func NewFilterKL(minCapacity uint32, k, l uint8) *Filter {
	// Calculate the number of items per generation such that the maximum
	// capacity (aka sliding window size) is at least the specified number of
	// items.
	//    w = g*(l+1)
	// => g = w/(l+1)
	// => g = ceil(w/(l+1))
	g := uint32(math.Ceil(float64(minCapacity) / (float64(l) + 1)))

	// Calculate the number of bits needed per slice based the number of items
	// per generation and number of slices used for filter insertions.
	//
	// The fill ratio can be approximated by r = 1 - e^-(n/m), where n is the
	// number of items and m is the bits per slice.  It is well known that
	// optimal filter usage for partitioned bloom filters is asymptotically when
	// r = 1/2.
	//
	// Thus solving r = 1/2 yields:
	//    e^-(n/m) = 1/2
	// => n/m = ln(2)
	// => m = n / ln(2)
	// => bitsPerSlice = k*g / ln(2)
	bitsPerSlice := uint64(math.Ceil(float64(k) * float64(g) / math.Log(2)))

	// The total filter size in bits is thus the total number of slices
	// multiplied by the number of bits per slice.
	kPlusL := uint16(k) + uint16(l)
	filterBytes := ((bitsPerSlice * uint64(kPlusL)) + 7) / 8

	// The key does not need to be cryptographically secure since its purpose is
	// only to ensure filters created at different times (for example, as
	// different nodes come online) produce a different set of false positives.
	var seed [8]byte
	s0 := uint64(time.Now().UnixNano())
	s1 := s0 ^ 0xa5a5a5a5a5a5a5a5
	binary.BigEndian.PutUint64(seed[:], s0^0x5a5a5a5a5a5a5a5a)
	key0, key1 := siphash.Hash128(s0, s1, seed[:])
	return &Filter{
		key0:               key0,
		key1:               key1,
		k:                  k,
		l:                  l,
		kPlusL:             kPlusL,
		bitsPerSlice:       bitsPerSlice,
		itemsPerGeneration: g,
		data:               make([]byte, filterBytes),
	}
}

// nearOptimalK calculates a near optimal number of hash functions to use based
// on the desired false positive rate and tradeoff versus the average number of
// queries.  Since the actual false positive rate depends on both the number of
// hash functions and an additional number of slices, the calculated value here
// might be off by one from what is truly optimal, but the difference is minor.
func nearOptimalK(fpRate float64) uint8 {
	return uint8(math.Ceil(-math.Log2(fpRate)))
}

// nearOptimalL calculates a near optimal number of additional slices to use for
// a given number of hash functions in order to maintain the desired false
// positive rate.
func nearOptimalL(k uint8, fpRate float64) uint8 {
	// There is not currently a closed formula for calculating the false
	// positive rate of an APBF, so just brute force it since it is only done
	// once when the filter is created and it's fast enough anyway.
	const maxL = 100
	for l := uint8(1); l <= maxL; l++ {
		result := CalcFPRate(k, l)
		if result > fpRate {
			return l - 1
		}
	}

	return maxL
}

// NewFilter returns an Age-Partitioned Bloom Filter (APBF) suitable for use in
// processing unbounded data streams where more recent items are more
// significant than older ones and some false positives are acceptable.  The
// parameters specify the number of most-recently added items that must always
// return true and the target false positive rate to maintain.
//
// Every new filter uses a unique key for the internal hashing logic so that
// each one will have a unique set of false positives.  The key is also
// automatically changed by Reset.
//
// Note that, due to rounding, the actual max number of items that can be added
// to the filter before old entries are expired might actually be slightly
// higher than the specified target and can be obtained via the Capacity method
// on the returned filter.
//
// Similarly, the actual false positive rate might be slightly better than the
// target rate and can be obtained via the FPRate method on the returned filter.
//
// In other words, both parameters are treated as lower bounds so that the
// returned filter has at least the requested target values.
//
// For most applications, the values will be perfectly acceptable, however,
// applications that desire greater control over the tuning can make use of
// NewFilterKL instead.
//
// The total space (in bytes) used by a filter that can hold 'n' items with a
// false positive rate of 'fpRate' is approximately:
//
//	1.3n * -log(fpRate), where log is base 10
func NewFilter(minCapacity uint32, fpRate float64) *Filter {
	k := nearOptimalK(fpRate)
	l := nearOptimalL(k, fpRate)
	return NewFilterKL(minCapacity, k, l)
}
