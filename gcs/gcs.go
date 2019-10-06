// Copyright (c) 2016-2017 The btcsuite developers
// Copyright (c) 2016-2017 The Lightning Network Developers
// Copyright (c) 2018-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gcs

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/dchest/siphash"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/wire"
)

// modReduceV1 is the reduction method used in version 1 filters and simply
// consists of x mod N.
func modReduceV1(x, N uint64) uint64 {
	return x % N
}

// chooseReduceFunc chooses which reduction method to use based on the filter
// version and returns the appropriate function to perform it.
func chooseReduceFunc(version uint16) func(uint64, uint64) uint64 {
	if version == 1 {
		return modReduceV1
	}
	return fastReduce
}

// KeySize is the size of the byte array required for key material for the
// SipHash keyed hash function.
const KeySize = 16

// uint64s implements sort.Interface for *[]uint64
type uint64s []uint64

func (s *uint64s) Len() int           { return len(*s) }
func (s *uint64s) Less(i, j int) bool { return (*s)[i] < (*s)[j] }
func (s *uint64s) Swap(i, j int)      { (*s)[i], (*s)[j] = (*s)[j], (*s)[i] }

// filter describes a versioned immutable filter that can be built from a set of
// data elements, serialized, deserialized, and queried in a thread-safe manner.
//
// It is used internally to implement the exported filter version types.
//
// See FilterV1 and FilterV2 for more details.
type filter struct {
	version     uint16
	n           uint32
	b           uint8
	modulusNM   uint64
	filterNData []byte
	filterData  []byte // Slice into filterNData with raw filter bytes.
}

// newFilter builds a new GCS filter with the specified version and provided
// tunable parameters that contains every item of the passed data as a member of
// the set.
//
// B is the tunable bits parameter for constructing the filter that is used as
// the bin size in the underlying Golomb coding with a value of 2^B.  The
// optimal value of B to minimize the size of the filter for a given false
// positive rate 1/M is floor(log_2(M) - 0.055256).  The maximum allowed value
// for B is 32.
//
// M is the inverse of the target false positive rate for the filter.  The
// optimal value of M to minimize the size of the filter for a given B is
// ceil(1.497137 * 2^B).
//
// key is a key used in the SipHash function used to hash each data element
// prior to inclusion in the filter.  This helps thwart would be attackers
// attempting to choose elements that intentionally cause false positives.
//
// The general process for determining optimal parameters for B and M to
// minimize the size of the filter is to start with the desired false positive
// rate and calculate B per the aforementioned formula accordingly.  Then, if
// the application permits the false positive rate to be varied, calculate the
// optimal value of M via the formula provided under the description of M.
//
// NOTE: Since this function must only be used internally, it will panic if
// called with a value of B greater than 32.
func newFilter(version uint16, B uint8, M uint64, key [KeySize]byte, data [][]byte) (*filter, error) {
	if B > 32 {
		panic(fmt.Sprintf("B value of %d is greater than max allowed 32", B))
	}
	switch version {
	case 1, 2:
	default:
		panic(fmt.Sprintf("version %d filters are not supported", version))
	}

	// Note that some of the entries might end up hashing to duplicates and
	// being removed below, so it is important to perform this check on the
	// raw data to maintain consensus.
	numEntries := uint64(len(data))
	if numEntries > math.MaxInt32 {
		str := fmt.Sprintf("unable to create filter with %d entries greater "+
			"than max allowed %d", len(data), math.MaxInt32)
		return nil, makeError(ErrNTooBig, str)
	}

	// Insert the hash of each data element into a slice while removing any
	// duplicates in the data for filters after version 1.
	k0 := binary.LittleEndian.Uint64(key[0:8])
	k1 := binary.LittleEndian.Uint64(key[8:16])
	values := make([]uint64, 0, numEntries)
	switch {
	case version == 1:
		for _, d := range data {
			v := siphash.Hash(k0, k1, d)
			values = append(values, v)
		}

	case version > 1:
		seen := make(map[uint64]struct{}, numEntries*2)
		for _, d := range data {
			if len(d) == 0 {
				continue
			}
			v := siphash.Hash(k0, k1, d)
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			values = append(values, v)
		}
	}
	numEntries = uint64(len(values))

	// Create the filter object and insert metadata.
	modBMask := uint64(1<<B) - 1
	f := filter{
		version:   version,
		n:         uint32(numEntries),
		b:         B,
		modulusNM: numEntries * M,
	}

	// Nothing to do for an empty filter.
	if len(values) == 0 {
		return &f, nil
	}

	// Reduce the hash of each data element to the range [0,N*M) and sort it.
	reduceFn := chooseReduceFunc(version)
	for i, v := range values {
		values[i] = reduceFn(v, f.modulusNM)
	}
	sort.Sort((*uint64s)(&values))

	// Every entry will have f.b bits for the remainder portion and a quotient
	// that is expected to be 1 on average with an exponentially decreasing
	// probability for each subsequent value with reasonably optimal parameters.
	// A quotient of 1 takes 2 bits in unary to encode and a quotient of 2 takes
	// 3 bits.  Since the first two terms dominate, a reasonable expected size
	// in bytes is:
	//   (NB + 2N/2 + 3N/2) / 8
	var b bitWriter
	sizeHint := (numEntries*uint64(f.b) + numEntries + 3*numEntries>>1) >> 3
	b.bytes = make([]byte, 0, sizeHint)

	// Write the sorted list of values into the filter bitstream using Golomb
	// coding.
	var quotient, prevValue, remainder uint64
	for _, v := range values {
		delta := v - prevValue
		prevValue = v

		// Calculate the remainder of the difference between this value and the
		// previous when dividing by 2^B.
		//
		// r = d % 2^B
		remainder = delta & modBMask

		// Calculate the quotient of the difference between this value and the
		// previous when dividing by 2^B.
		//
		// q = floor(d / 2^B)
		quotient = (delta - remainder) >> f.b

		// Write the quotient into the bitstream in unary.  The average value
		// will be around 1 for reasonably optimal parameters (which is encoded
		// as 2 bits - 0b10).
		for quotient > 0 {
			b.writeOne()
			quotient--
		}
		b.writeZero()

		// Write the remainder into the bitstream as a big-endian integer with
		// B bits.  Note that Golomb coding typically uses truncated binary
		// encoding in order to support arbitrary bin sizes, however, since 2^B
		// is necessarily fixed to a power of 2, it is equivalent to a regular
		// binary code.
		b.writeNBits(remainder, uint(f.b))
	}

	// Save the filter data internally as n + filter bytes along with the raw
	// filter data as a slice into it.
	switch version {
	case 1:
		ndata := make([]byte, 4+len(b.bytes))
		binary.BigEndian.PutUint32(ndata, f.n)
		copy(ndata[4:], b.bytes)
		f.filterNData = ndata
		f.filterData = ndata[4:]

	case 2:
		var buf bytes.Buffer
		nSize := wire.VarIntSerializeSize(uint64(f.n))
		buf.Grow(nSize + len(b.bytes))

		// The errors are ignored here since they can't realistically fail due
		// to writing into an allocated buffer.
		_ = wire.WriteVarInt(&buf, 0, uint64(f.n))
		_, _ = buf.Write(b.bytes)
		f.filterNData = buf.Bytes()
		f.filterData = f.filterNData[nSize:]
	}

	return &f, nil
}

// Bytes returns the serialized format of the GCS filter which includes N, but
// does not include other parameters such as the false positive rate or the key.
func (f *filter) Bytes() []byte {
	return f.filterNData
}

// N returns the size of the data set used to build the filter.
func (f *filter) N() uint32 {
	return f.n
}

// readFullUint64 reads a value represented by the sum of a unary multiple of
// the Golomb coding bin size (2^B) and a big-endian B-bit remainder.
func (f *filter) readFullUint64(b *bitReader) (uint64, error) {
	v, err := b.readUnary()
	if err != nil {
		return 0, err
	}

	rem, err := b.readNBits(uint(f.b))
	if err != nil {
		return 0, err
	}

	// Add the multiple and the remainder.
	return v<<f.b + rem, nil
}

// Match checks whether a []byte value is likely (within collision probability)
// to be a member of the set represented by the filter.
func (f *filter) Match(key [KeySize]byte, data []byte) bool {
	// An empty filter or empty data can't possibly match anything.
	if len(f.filterData) == 0 || len(data) == 0 {
		return false
	}

	// Hash the search term with the same parameters as the filter.
	reduceFn := chooseReduceFunc(f.version)
	k0 := binary.LittleEndian.Uint64(key[0:8])
	k1 := binary.LittleEndian.Uint64(key[8:16])
	term := siphash.Hash(k0, k1, data)
	term = reduceFn(term, f.modulusNM)

	// Go through the search filter and look for the desired value.
	b := newBitReader(f.filterData)
	var lastValue uint64
	for lastValue <= term {
		// Read the difference between previous and new value from
		// bitstream.
		value, err := f.readFullUint64(&b)
		if err != nil {
			return false
		}

		// Add the previous value to it.
		value += lastValue
		if value == term {
			return true
		}

		lastValue = value
	}

	return false
}

// matchPool pools allocations for match data.
var matchPool sync.Pool

// MatchAny checks whether any []byte value is likely (within collision
// probability) to be a member of the set represented by the filter faster than
// calling Match() for each value individually.
func (f *filter) MatchAny(key [KeySize]byte, data [][]byte) bool {
	// An empty filter or empty data can't possibly match anything.
	if len(f.filterData) == 0 || len(data) == 0 {
		return false
	}

	// Create an uncompressed filter of the search values.
	reduceFn := chooseReduceFunc(f.version)
	var values *[]uint64
	if v := matchPool.Get(); v != nil {
		values = v.(*[]uint64)
		*values = (*values)[:0]
	} else {
		vs := make([]uint64, 0, len(data))
		values = &vs
	}
	defer matchPool.Put(values)
	k0 := binary.LittleEndian.Uint64(key[0:8])
	k1 := binary.LittleEndian.Uint64(key[8:16])
	for _, d := range data {
		v := siphash.Hash(k0, k1, d)
		v = reduceFn(v, f.modulusNM)
		*values = append(*values, v)
	}
	sort.Sort((*uint64s)(values))

	// Zip down the filters, comparing values until we either run out of
	// values to compare in one of the filters or we reach a matching
	// value.
	b := newBitReader(f.filterData)
	searchSize := len(data)
	var searchIdx int
	var filterVal uint64
nextFilterVal:
	for i := uint32(0); i < f.n; i++ {
		// Read the next item to compare from the filter.
		delta, err := f.readFullUint64(&b)
		if err != nil {
			return false
		}
		filterVal += delta

		// Iterate through the values to search until either a match is found
		// or the search value exceeds the current filter value.
		for ; searchIdx < searchSize; searchIdx++ {
			searchVal := (*values)[searchIdx]
			if searchVal == filterVal {
				return true
			}

			// Move to the next filter item once the current search value
			// exceeds it.
			if searchVal > filterVal {
				continue nextFilterVal
			}
		}

		// Exit early when there are no more values to search for.
		break
	}

	return false
}

// Hash returns the BLAKE256 hash of the filter.
func (f *filter) Hash() chainhash.Hash {
	// Empty filters have a hash of all zeroes.
	if len(f.filterNData) == 0 {
		return chainhash.Hash{}
	}

	return chainhash.Hash(blake256.Sum256(f.filterNData))
}

// FilterV1 describes an immutable filter that can be built from a set of data
// elements, serialized, deserialized, and queried in a thread-safe manner.  The
// serialized form is compressed as a Golomb Coded Set (GCS) along with the
// number of members of the set.  The hash function used is SipHash, a keyed
// function.  The key used in building the filter is required in order to match
// filter values and is not included in the serialized form.
type FilterV1 struct {
	filter
}

// P returns the filter's collision probability as a negative power of 2.  For
// example, a collision probability of 1 / 2^20 is represented as 20.
func (f *FilterV1) P() uint8 {
	return f.b
}

// NewFilter builds a new version 1 GCS filter with a collision probability of
// 1 / 2^P for the given key and data.
func NewFilterV1(P uint8, key [KeySize]byte, data [][]byte) (*FilterV1, error) {
	// Basic sanity check.
	if P > 32 {
		str := fmt.Sprintf("P value of %d is greater than max allowed 32", P)
		return nil, makeError(ErrPTooBig, str)
	}

	filter, err := newFilter(1, P, 1<<P, key, data)
	if err != nil {
		return nil, err
	}
	return &FilterV1{filter: *filter}, nil
}

// FromBytesV1 deserializes a version 1 GCS filter from a known P and serialized
// filter as returned by Bytes().
func FromBytesV1(P uint8, d []byte) (*FilterV1, error) {
	// Basic sanity check.
	if P > 32 {
		str := fmt.Sprintf("P value of %d is greater than max allowed 32", P)
		return nil, makeError(ErrPTooBig, str)
	}

	var n uint32
	var filterData []byte
	if len(d) >= 4 {
		n = binary.BigEndian.Uint32(d[:4])
		filterData = d[4:]
	} else if len(d) < 4 && len(d) != 0 {
		str := "number of items serialization missing"
		return nil, makeError(ErrMisserialized, str)
	}

	f := filter{
		version:     1,
		n:           n,
		b:           P,
		modulusNM:   uint64(n) * uint64(1<<P),
		filterNData: d,
		filterData:  filterData,
	}
	return &FilterV1{filter: f}, nil
}

// FilterV2 describes an immutable filter that can be built from a set of data
// elements, serialized, deserialized, and queried in a thread-safe manner.  The
// serialized form is compressed as a Golomb Coded Set (GCS) along with the
// number of members of the set.  The hash function used is SipHash, a keyed
// function.  The key used in building the filter is required in order to match
// filter values and is not included in the serialized form.
//
// Version 2 filters differ from version 1 filters in four ways:
//
// 1) Support for independently specifying the false positive rate and Golomb
//    coding bin size which allows minimizing the filter size
// 2) A faster (incompatible with version 1) reduction function
// 3) A more compact serialization for the number of members in the set
// 4) Deduplication of all hash collisions prior to reducing and serializing the
//    deltas
type FilterV2 struct {
	filter
}

// B returns the tunable bits parameter that was used to construct the filter.
// It represents the bin size in the underlying Golomb coding with a value of
// 2^B.
func (f *FilterV2) B() uint8 {
	return f.b
}

// NewFilterV2 builds a new GCS filter with the provided tunable parameters that
// contains every item of the passed data as a member of the set.
//
// B is the tunable bits parameter for constructing the filter that is used as
// the bin size in the underlying Golomb coding with a value of 2^B.  The
// optimal value of B to minimize the size of the filter for a given false
// positive rate 1/M is floor(log_2(M) - 0.055256).  The maximum allowed value
// for B is 32.  An error will be returned for larger values.
//
// M is the inverse of the target false positive rate for the filter.  The
// optimal value of M to minimize the size of the filter for a given B is
// ceil(1.497137 * 2^B).
//
// key is a key used in the SipHash function used to hash each data element
// prior to inclusion in the filter.  This helps thwart would be attackers
// attempting to choose elements that intentionally cause false positives.
//
// The general process for determining optimal parameters for B and M to
// minimize the size of the filter is to start with the desired false positive
// rate and calculate B per the aforementioned formula accordingly.  Then, if
// the application permits the false positive rate to be varied, calculate the
// optimal value of M via the formula provided under the description of M.
func NewFilterV2(B uint8, M uint64, key [KeySize]byte, data [][]byte) (*FilterV2, error) {
	// Basic sanity check.
	if B > 32 {
		str := fmt.Sprintf("B value of %d is greater than max allowed 32", B)
		return nil, makeError(ErrBTooBig, str)
	}

	filter, err := newFilter(2, B, M, key, data)
	if err != nil {
		return nil, err
	}
	return &FilterV2{filter: *filter}, nil
}

// FromBytesV2 deserializes a version 2 GCS filter from a known B, M, and
// serialized filter as returned by Bytes().
func FromBytesV2(B uint8, M uint64, d []byte) (*FilterV2, error) {
	if B > 32 {
		str := fmt.Sprintf("B value of %d is greater than max allowed 32", B)
		return nil, makeError(ErrBTooBig, str)
	}

	var n uint64
	var filterData []byte
	if len(d) > 0 {
		var err error
		n, err = wire.ReadVarInt(bytes.NewReader(d), 0)
		if err != nil {
			str := fmt.Sprintf("failed to read number of filter items: %v", err)
			return nil, makeError(ErrMisserialized, str)
		}
		filterData = d[wire.VarIntSerializeSize(n):]
	}

	f := filter{
		version:     2,
		n:           uint32(n),
		b:           B,
		modulusNM:   n * M,
		filterNData: d,
		filterData:  filterData,
	}
	return &FilterV2{filter: f}, nil
}

// MakeHeaderForFilter makes a filter chain header for a filter, given the
// filter and the previous filter chain header.
func MakeHeaderForFilter(filter *FilterV1, prevHeader *chainhash.Hash) chainhash.Hash {
	// In the buffer we created above we'll compute hash || prevHash as an
	// intermediate value.
	var filterTip [2 * chainhash.HashSize]byte
	filterHash := filter.Hash()
	copy(filterTip[:], filterHash[:])
	copy(filterTip[chainhash.HashSize:], prevHeader[:])

	// The final filter hash is the blake256 of the hash computed above.
	return chainhash.Hash(blake256.Sum256(filterTip[:]))
}
