// Copyright (c) 2016-2017 The btcsuite developers
// Copyright (c) 2016-2017 The Lightning Network Developers
// Copyright (c) 2018-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gcs

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/dchest/siphash"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/crypto/blake256"
)

// Inspired by https://github.com/rasky/gcs

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
// See FilterV1 for more details.
type filter struct {
	version     uint16
	n           uint32
	p           uint8
	modulusNP   uint64
	filterNData []byte
	filterData  []byte // Slice into filterNData with raw filter bytes.
}

// Filter describes an immutable filter that can be built from a set of data
// elements, serialized, deserialized, and queried in a thread-safe manner. The
// serialized form is compressed as a Golomb Coded Set (GCS), but does not
// include N or P to allow the user to encode the metadata separately if
// necessary. The hash function used is SipHash, a keyed function; the key used
// in building the filter is required in order to match filter values and is
// not included in the serialized form.
type FilterV1 struct {
	filter
}

// newFilter builds a new GCS filter of the specified version with the collision
// probability of `1/(2**P)`, key `key`, and including every `[]byte` in `data`
// as a member of the set.
func newFilter(version uint16, P uint8, key [KeySize]byte, data [][]byte) (*filter, error) {
	if len(data) > math.MaxInt32 {
		str := fmt.Sprintf("unable to create filter with %d entries greater "+
			"than max allowed %d", len(data), math.MaxInt32)
		return nil, makeError(ErrNTooBig, str)
	}
	if P > 32 {
		str := fmt.Sprintf("P value of %d is greater than max allowed 32", P)
		return nil, makeError(ErrPTooBig, str)
	}

	// Create the filter object and insert metadata.
	modP := uint64(1 << P)
	modPMask := modP - 1
	f := filter{
		version:   version,
		n:         uint32(len(data)),
		p:         P,
		modulusNP: uint64(len(data)) * modP,
	}

	// Nothing to do for an empty filter.
	if len(data) == 0 {
		return &f, nil
	}

	// Allocate filter data.
	values := make([]uint64, 0, len(data))

	// Insert the hash (modulo N*P) of each data element into a slice and
	// sort the slice.
	k0 := binary.LittleEndian.Uint64(key[0:8])
	k1 := binary.LittleEndian.Uint64(key[8:16])
	for _, d := range data {
		v := siphash.Hash(k0, k1, d) % f.modulusNP
		values = append(values, v)
	}
	sort.Sort((*uint64s)(&values))

	var b bitWriter

	// Write the sorted list of values into the filter bitstream,
	// compressing it using Golomb coding.
	var value, lastValue, remainder uint64
	for _, v := range values {
		// Calculate the difference between this value and the last,
		// modulo P.
		remainder = (v - lastValue) & modPMask

		// Calculate the difference between this value and the last,
		// divided by P.
		value = (v - lastValue - remainder) >> f.p
		lastValue = v

		// Write the P multiple into the bitstream in unary; the
		// average should be around 1 (2 bits - 0b10).
		for value > 0 {
			b.writeOne()
			value--
		}
		b.writeZero()

		// Write the remainder as a big-endian integer with enough bits
		// to represent the appropriate collision probability.
		b.writeNBits(remainder, uint(f.p))
	}

	// Save the filter data internally as n + filter bytes
	ndata := make([]byte, 4+len(b.bytes))
	binary.BigEndian.PutUint32(ndata, f.n)
	copy(ndata[4:], b.bytes)
	f.filterNData = ndata
	f.filterData = ndata[4:]

	return &f, nil
}

// NewFilter builds a new version 1 GCS filter with the collision probability of
// `1/(2**P)`, key `key`, and including every `[]byte` in `data` as a member of
// the set.
func NewFilterV1(P uint8, key [KeySize]byte, data [][]byte) (*FilterV1, error) {
	filter, err := newFilter(1, P, key, data)
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
		p:           P,
		modulusNP:   uint64(n) * uint64(1<<P),
		filterNData: d,
		filterData:  filterData,
	}
	return &FilterV1{filter: f}, nil
}

// Bytes returns the serialized format of the GCS filter which includes N, but
// does not include P (returned by a separate method) or the key used by
// SipHash.
func (f *filter) Bytes() []byte {
	return f.filterNData
}

// P returns the filter's collision probability as a negative power of 2 (that
// is, a collision probability of `1/2**20` is represented as 20).
func (f *filter) P() uint8 {
	return f.p
}

// N returns the size of the data set used to build the filter.
func (f *filter) N() uint32 {
	return f.n
}

// Match checks whether a []byte value is likely (within collision probability)
// to be a member of the set represented by the filter.
func (f *filter) Match(key [KeySize]byte, data []byte) bool {
	// An empty filter or empty data can't possibly match anything.
	if len(f.filterData) == 0 || len(data) == 0 {
		return false
	}

	// Create a filter bitstream.
	b := newBitReader(f.filterData)

	// Hash our search term with the same parameters as the filter.
	k0 := binary.LittleEndian.Uint64(key[0:8])
	k1 := binary.LittleEndian.Uint64(key[8:16])
	term := siphash.Hash(k0, k1, data) % f.modulusNP

	// Go through the search filter and look for the desired value.
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

	// Create a filter bitstream.
	b := newBitReader(f.filterData)

	// Create an uncompressed filter of the search values.
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
		v := siphash.Hash(k0, k1, d) % f.modulusNP
		*values = append(*values, v)
	}
	sort.Sort((*uint64s)(values))

	// Zip down the filters, comparing values until we either run out of
	// values to compare in one of the filters or we reach a matching
	// value.
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

// readFullUint64 reads a value represented by the sum of a unary multiple of
// the filter's P modulus (`2**P`) and a big-endian P-bit remainder.
func (f *filter) readFullUint64(b *bitReader) (uint64, error) {
	v, err := b.readUnary()
	if err != nil {
		return 0, err
	}

	rem, err := b.readNBits(uint(f.p))
	if err != nil {
		return 0, err
	}

	// Add the multiple and the remainder.
	return v<<f.p + rem, nil
}

// Hash returns the BLAKE256 hash of the filter.
func (f *filter) Hash() chainhash.Hash {
	// Empty filters have a hash of all zeroes.
	if len(f.filterNData) == 0 {
		return chainhash.Hash{}
	}

	return chainhash.Hash(blake256.Sum256(f.filterNData))
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
