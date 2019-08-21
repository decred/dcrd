// Copyright (c) 2018-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gcs

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math/rand"
	"testing"
	"time"

	"github.com/dchest/siphash"
	"github.com/decred/dcrd/chaincfg/chainhash"
)

// TestFilter ensures the filters and all associated methods work as expected by
// using various known parameters and contents along with random keys for
// matching purposes.
func TestFilter(t *testing.T) {
	// Use a random key for each test instance and log it if the tests fail.
	rng := rand.New(rand.NewSource(time.Now().Unix()))
	var randKey [KeySize]byte
	for i := 0; i < KeySize; i += 4 {
		binary.BigEndian.PutUint32(randKey[i:], rng.Uint32())
	}
	defer func(t *testing.T, randKey [KeySize]byte) {
		if t.Failed() {
			t.Logf("random key: %x", randKey)
		}
	}(t, randKey)

	// contents1 defines a set of known elements for use in the tests below.
	contents1 := [][]byte{[]byte("Alex"), []byte("Bob"), []byte("Charlie"),
		[]byte("Dick"), []byte("Ed"), []byte("Frank"), []byte("George"),
		[]byte("Harry"), []byte("Ilya"), []byte("John"), []byte("Kevin"),
		[]byte("Larry"), []byte("Michael"), []byte("Nate"), []byte("Owen"),
		[]byte("Paul"), []byte("Quentin"),
	}

	// contents2 defines a separate set of known elements for use in the tests
	// below.
	contents2 := [][]byte{[]byte("Alice"), []byte("Betty"),
		[]byte("Charmaine"), []byte("Donna"), []byte("Edith"), []byte("Faina"),
		[]byte("Georgia"), []byte("Hannah"), []byte("Ilsbeth"),
		[]byte("Jennifer"), []byte("Kayla"), []byte("Lena"), []byte("Michelle"),
		[]byte("Natalie"), []byte("Ophelia"), []byte("Peggy"), []byte("Queenie"),
	}

	tests := []struct {
		name        string        // test description
		version     uint16        // filter version
		b           uint8         // golomb coding bin size
		m           uint64        // inverse of false positive rate
		matchKey    [KeySize]byte // random filter key for matches
		contents    [][]byte      // data to include in the filter
		wantMatches [][]byte      // expected matches
		fixedKey    [KeySize]byte // fixed filter key for testing serialization
		wantBytes   string        // expected serialized bytes
		wantHash    string        // expected filter hash
	}{{
		name:        "empty filter",
		version:     1,
		b:           20,
		m:           1 << 20,
		matchKey:    randKey,
		contents:    nil,
		wantMatches: nil,
		fixedKey:    [KeySize]byte{},
		wantBytes:   "",
		wantHash:    "0000000000000000000000000000000000000000000000000000000000000000",
	}, {
		name:        "contents1 with B=20, M=1<<20",
		version:     1,
		b:           20,
		m:           1 << 20,
		matchKey:    randKey,
		contents:    contents1,
		wantMatches: contents1,
		fixedKey:    [KeySize]byte{},
		wantBytes:   "00000011ce76b76760b54096a233d504ce55b80600fb072c74893cf306eb0c050f0b3c32e8c23436f8f5e67a986a46470790",
		wantHash:    "a802fbe6f06991877cde8f3d770d8da8cf195816f04874cab045ffccaddd880d",
	}, {
		name:        "contents1 with B=19, M=1<<19",
		version:     1,
		b:           19,
		m:           1 << 19,
		matchKey:    randKey,
		contents:    contents1,
		wantMatches: contents1,
		fixedKey:    [KeySize]byte{},
		wantBytes:   "000000112375937586050f0e9e19689983a3ab9b6f8f0cbc2f204b5233d5099ca0c9fbe9ec6a1f60e76fba3ad6835a28",
		wantHash:    "be9ba34f03ced957e6f5c4d583ddfd34c136b486fbec2a42b4c7588a2d7813c1",
	}, {
		name:        "contents2 with B=19, M=1<<19",
		version:     1,
		b:           19,
		m:           1 << 19,
		matchKey:    randKey,
		contents:    contents2,
		wantMatches: contents2,
		fixedKey:    [KeySize]byte{},
		wantBytes:   "000000114306259e36131a6c9bbd968a6c61dc110804d5ac91d20d6e9314a50332bffed877657c004e2366fcd34cda60",
		wantHash:    "dcbaf452f6de4c82ea506fa551d75876c4979ef388f785509b130de62eeaec23",
	}, {
		name:        "contents2 with B=10, M=1<<10",
		version:     1,
		b:           10,
		m:           1 << 10,
		matchKey:    randKey,
		contents:    contents2,
		wantMatches: contents2,
		fixedKey:    [KeySize]byte{},
		wantBytes:   "000000111ca3aafb023074dc5bf2498df791b7d6e846e9f5016006d600",
		wantHash:    "afa181cd5c4b08eb9c16d1c97c95df1ca7b82e5e444a396cec5e02f2804fbd1a",
	}}

	for _, test := range tests {
		// Create a filter with the match key for all tests not related to
		// testing serialization.
		f, err := newFilter(test.version, test.b, test.m, test.matchKey,
			test.contents)
		if err != nil {
			t.Errorf("%q: unexpected err: %v", test.name, err)
			continue
		}

		// Ensure the parameter values are returned properly.
		resultB := f.b
		if resultB != test.b {
			t.Errorf("%q: unexpected B -- got %d, want %d", test.name,
				resultB, test.b)
			continue
		}
		resultN := f.N()
		if resultN != uint32(len(test.contents)) {
			t.Errorf("%q: unexpected N -- got %d, want %d", test.name,
				resultN, uint32(len(test.contents)))
			continue
		}
		if test.version == 1 {
			v1Filter := &FilterV1{filter: *f}
			resultP := v1Filter.P()
			if resultP != test.b {
				t.Errorf("%q: unexpected P -- got %d, want %d", test.name,
					resultP, test.b)
				continue
			}
		}

		// Ensure empty data never matches.
		if f.Match(test.matchKey, nil) {
			t.Errorf("%q: unexpected match of nil data", test.name)
			continue
		}
		if f.MatchAny(test.matchKey, nil) {
			t.Errorf("%q: unexpected match any of nil data", test.name)
			continue
		}

		// Ensure empty filter never matches data.
		if len(test.contents) == 0 {
			wantMiss := []byte("test")
			if f.Match(test.matchKey, wantMiss) {
				t.Errorf("%q: unexpected match of %q on empty filter",
					test.name, wantMiss)
				continue
			}
			if f.MatchAny(test.matchKey, [][]byte{wantMiss}) {
				t.Errorf("%q: unexpected match any of %q on empty filter",
					test.name, wantMiss)
				continue
			}
		}

		// Ensure all of the expected matches occur individually.
		for _, wantMatch := range test.wantMatches {
			if !f.Match(test.matchKey, wantMatch) {
				t.Errorf("%q: failed match for %q", test.name, wantMatch)
				continue
			}
		}

		// Ensure a subset of the expected matches works in various orders when
		// matching any.
		if len(test.contents) > 0 {
			// Create set of data to attempt to match such that only the final
			// item is an element in the filter.
			matches := make([][]byte, 0, len(test.contents))
			for _, data := range test.contents {
				mutated := make([]byte, len(data))
				copy(mutated, data)
				mutated[0] ^= 0x55
				matches = append(matches, mutated)
			}
			matches[len(matches)-1] = test.contents[len(test.contents)-1]

			if !f.MatchAny(test.matchKey, matches) {
				t.Errorf("%q: failed match for %q", test.name, matches)
				continue
			}

			// Fisher-Yates shuffle the match set and test for matches again.
			for i := 0; i < len(matches); i++ {
				// Pick a number between current index and the end.
				j := rand.Intn(len(matches)-i) + i
				matches[i], matches[j] = matches[j], matches[i]
			}
			if !f.MatchAny(test.matchKey, matches) {
				t.Errorf("%q: failed match for %q", test.name, matches)
				continue
			}
		}

		// Recreate the filter with a fixed key for serialization testing.
		fixedFilter, err := newFilter(test.version, test.b, test.m,
			test.fixedKey, test.contents)
		if err != nil {
			t.Errorf("%q: unexpected err: %v", test.name, err)
			continue
		}

		// Parse the expected serialized bytes and ensure they match.
		wantBytes, err := hex.DecodeString(test.wantBytes)
		if err != nil {
			t.Errorf("%q: unexpected err parsing want bytes hex: %v", test.name,
				err)
			continue
		}
		resultBytes := fixedFilter.Bytes()
		if !bytes.Equal(resultBytes, wantBytes) {
			t.Errorf("%q: mismatched bytes -- got %x, want %x", test.name,
				resultBytes, wantBytes)
			continue
		}

		// Parse the expected hash and ensure it matches.
		wantHash, err := chainhash.NewHashFromStr(test.wantHash)
		if err != nil {
			t.Errorf("%q: unexpected err parsing want hash hex: %v", test.name,
				err)
			continue
		}
		resultHash := fixedFilter.Hash()
		if resultHash != *wantHash {
			t.Errorf("%q: mismatched hash -- got %v, want %v", test.name,
				resultHash, *wantHash)
			continue
		}

		// filterMatcher allows different versions of the filter types to be
		// used for the match testing below.
		type filterMatcher interface {
			Match([KeySize]byte, []byte) bool
		}

		// Deserialize the filter from bytes.
		var f2 filterMatcher
		switch test.version {
		case 1:
			tf2, err := FromBytesV1(test.b, wantBytes)
			if err != nil {
				t.Errorf("%q: unexpected err: %v", test.name, err)
				continue
			}
			f2 = tf2

		default:
			t.Errorf("%q: unsupported filter version: %d", test.name,
				test.version)
			continue
		}

		// Ensure all of the expected matches occur on the deserialized filter.
		for _, wantMatch := range test.wantMatches {
			if !f2.Match(test.fixedKey, wantMatch) {
				t.Errorf("%q: failed match for %q", test.name, wantMatch)
				continue
			}
		}
	}
}

// TestFilterMisses ensures the filter does not match entries with a rate that
// far exceeds the false positive rate.
func TestFilterMisses(t *testing.T) {
	// Create a filter with the lowest supported false positive rate to reduce
	// the chances of a false positive as much as possible.
	var key [KeySize]byte
	f, err := NewFilterV1(32, key, [][]byte{[]byte("entry")})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Since the filter may have false positives, try several queries and track
	// how many matches there are.  Something is very wrong if the filter
	// matched multiple queries for data that are not in the filter with such a
	// low false positive rate.
	const numTries = 5
	var numMatches int
	for i := uint8(0); i < numTries; i++ {
		data := [1]byte{i}
		if f.Match(key, data[:]) {
			numMatches++
		}
	}
	if numMatches == numTries {
		t.Fatalf("filter matched non-existing entries %d times", numMatches)
	}

	// Try again with multi match.
	numMatches = 0
	for i := uint8(0); i < numTries; i++ {
		searchEntry := [1]byte{i}
		data := [][]byte{searchEntry[:]}
		if f.MatchAny(key, data[:]) {
			numMatches++
		}
	}
	if numMatches == numTries {
		t.Fatalf("filter matched non-existing entries %d times", numMatches)
	}
}

// TestFilterCorners ensures a few negative corner cases such as specifying
// parameters that are too large behave as expected.
func TestFilterCorners(t *testing.T) {
	// Attempt to construct filter with parameters too large.
	const largeP = 33
	var key [KeySize]byte
	_, err := NewFilterV1(largeP, key, nil)
	if !IsErrorCode(err, ErrPTooBig) {
		t.Fatalf("did not receive expected err for P too big -- got %v, want %v",
			err, ErrPTooBig)
	}
	_, err = FromBytesV1(largeP, nil)
	if !IsErrorCode(err, ErrPTooBig) {
		t.Fatalf("did not receive expected err for P too big -- got %v, want %v",
			err, ErrPTooBig)
	}

	// Attempt to decode a filter without the N value serialized properly.
	_, err = FromBytesV1(20, []byte{0x00})
	if !IsErrorCode(err, ErrMisserialized) {
		t.Fatalf("did not receive expected err -- got %v, want %v", err,
			ErrMisserialized)
	}
}

// TestZeroHashMatches ensures that a filter matches search items when their
// internal hash is zero.
func TestZeroHashMatches(t *testing.T) {
	// Choose an item that intentionally hashes to zero for a given set of
	// filter parameters.
	searchItem := []byte("testr")
	contents := [][]byte{searchItem, []byte("test2")}
	const highFPRate = 2
	modVal := ((1 << highFPRate) * uint64(len(contents)))
	var key [KeySize]byte
	term := siphash.Hash(0, 0, searchItem) % modVal
	if term != 0 {
		t.Fatalf("search item must hash to zero -- got %x", term)
	}

	// Create a version 1 filter and ensure a match for the search item when
	// that item hashes to zero with the filters parameters.
	f, err := NewFilterV1(highFPRate, key, contents)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !f.Match(key, searchItem) {
		t.Fatalf("failed to match key with 0 siphash")
	}
	if !f.MatchAny(key, [][]byte{searchItem}) {
		t.Fatalf("failed to match key with 0 siphash")
	}
}

// TestPanics ensures various internal functions panic when called improperly.
func TestPanics(t *testing.T) {
	testPanic := func(fn func()) (paniced bool) {
		// Setup a defer to catch the expected panic and update the
		// return variable.
		defer func() {
			if err := recover(); err != nil {
				paniced = true
			}
		}()

		fn()
		return false
	}

	// Ensure attempting to create a filter with parameters too large panics.
	paniced := testPanic(func() {
		const largeB = 33
		const smallM = 1 << 10
		var key [KeySize]byte
		newFilter(1, largeB, smallM, key, nil)
	})
	if !paniced {
		t.Fatal("newFilter did not panic with too large parameter")
	}

	// Ensure attempting to create and unsupported filter version panics.
	paniced = testPanic(func() {
		const normalB = 19
		const normalM = 784931
		var key [KeySize]byte
		newFilter(65535, normalB, normalM, key, nil)
	})
	if !paniced {
		t.Fatal("newFilter did not panic with unsupported version")
	}
}
