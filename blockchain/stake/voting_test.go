// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ffldb"
)

func bytesFromHex(s string) []byte {
	b, _ := hex.DecodeString(s)
	return b
}

func TestDecodingAndEncodingVoteBits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   uint16
		out  DecodedVoteBitsPrefix
	}{
		{
			"no and all undefined",
			0x0000,
			DecodedVoteBitsPrefix{
				BlockValid: false,
				Unused:     false,
				Issues: [7]IssueVote{
					IssueVoteUndefined, IssueVoteUndefined,
					IssueVoteUndefined, IssueVoteUndefined,
					IssueVoteUndefined, IssueVoteUndefined,
					IssueVoteUndefined,
				},
			},
		},
		{
			"yes and all undefined",
			0x0001,
			DecodedVoteBitsPrefix{
				BlockValid: true,
				Unused:     false,
				Issues: [7]IssueVote{
					IssueVoteUndefined, IssueVoteUndefined,
					IssueVoteUndefined, IssueVoteUndefined,
					IssueVoteUndefined, IssueVoteUndefined,
					IssueVoteUndefined,
				},
			},
		},
		{
			"no (unused set) and all undefined",
			0x0002,
			DecodedVoteBitsPrefix{
				BlockValid: false,
				Unused:     true,
				Issues: [7]IssueVote{
					IssueVoteUndefined, IssueVoteUndefined,
					IssueVoteUndefined, IssueVoteUndefined,
					IssueVoteUndefined, IssueVoteUndefined,
					IssueVoteUndefined,
				},
			},
		},
		{
			"yes and odd issues yes, even issues no",
			0x6665,
			DecodedVoteBitsPrefix{
				BlockValid: true,
				Unused:     false,
				Issues: [7]IssueVote{
					IssueVoteYes, IssueVoteNo,
					IssueVoteYes, IssueVoteNo,
					IssueVoteYes, IssueVoteNo,
					IssueVoteYes,
				},
			},
		},
		{
			"yes and odd issues no, even issues abstain",
			0xBBB9,
			DecodedVoteBitsPrefix{
				BlockValid: true,
				Unused:     false,
				Issues: [7]IssueVote{
					IssueVoteNo, IssueVoteAbstain,
					IssueVoteNo, IssueVoteAbstain,
					IssueVoteNo, IssueVoteAbstain,
					IssueVoteNo,
				},
			},
		},
		{
			"no and issue 1 yes, rest unused",
			0x0004,
			DecodedVoteBitsPrefix{
				BlockValid: false,
				Unused:     false,
				Issues: [7]IssueVote{
					IssueVoteYes, IssueVoteUndefined,
					IssueVoteUndefined, IssueVoteUndefined,
					IssueVoteUndefined, IssueVoteUndefined,
					IssueVoteUndefined,
				},
			},
		},
		{
			"yes and issue 3 yes, issue 4 no",
			0x0241,
			DecodedVoteBitsPrefix{
				BlockValid: true,
				Unused:     false,
				Issues: [7]IssueVote{
					IssueVoteUndefined, IssueVoteUndefined,
					IssueVoteYes, IssueVoteNo,
					IssueVoteUndefined, IssueVoteUndefined,
					IssueVoteUndefined,
				},
			},
		},
		{
			"yes on blocks, all issues yes",
			0x5555,
			DecodedVoteBitsPrefix{
				BlockValid: true,
				Unused:     false,
				Issues: [7]IssueVote{
					IssueVoteYes, IssueVoteYes,
					IssueVoteYes, IssueVoteYes,
					IssueVoteYes, IssueVoteYes,
					IssueVoteYes,
				},
			},
		},
		{
			"yes on blocks, all issues no",
			0xaaa9,
			DecodedVoteBitsPrefix{
				BlockValid: true,
				Unused:     false,
				Issues: [7]IssueVote{
					IssueVoteNo, IssueVoteNo,
					IssueVoteNo, IssueVoteNo,
					IssueVoteNo, IssueVoteNo,
					IssueVoteNo,
				},
			},
		},
	}

	// Encoding.
	for i := range tests {
		test := tests[i]
		in := EncodeVoteBitsPrefix(test.out)
		if !reflect.DeepEqual(in, test.in) {
			t.Errorf("bad result on EncodeVoteBitsPrefix test %v: got %04x, "+
				"want %04x", test.name, in, test.in)
		}
	}

	// Decoding.
	for i := range tests {
		test := tests[i]
		out := DecodeVoteBitsPrefix(test.in)
		if out != test.out {
			t.Errorf("bad result on DecodeVoteBitsPrefix test %v: got %v, "+
				"want %v", test.name, out, test.out)
		}
	}
}

// TestRollingVotingPrefixTallySerializing tests serializing and deserializing
// for RollingVotingPrefixTally.
func TestRollingVotingPrefixTallySerializing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   RollingVotingPrefixTally
		out  []byte
	}{
		{
			"no and all undefined",
			RollingVotingPrefixTally{
				LastIntervalBlock:  BlockKey{Hash: chainhash.Hash{byte(0x02)}, Height: 38859},
				CurrentBlockHeight: 10200,
				BlockValid:         213,
				Unused:             492,
				Issues: [7]VotingTally{
					VotingTally{123, 321, 324, 2819},
					VotingTally{523, 2355, 0, 0},
					VotingTally{352, 2352, 2442, 44},
					VotingTally{234, 0, 44, 344},
					VotingTally{523, 223, 133, 3444},
					VotingTally{0, 44, 3233, 432},
					VotingTally{867, 1, 444, 33},
				},
			},
			bytesFromHex("0200000000000000000000000000000000000000000000000000000000000000cb970000d8270000d500ec017b0041014401030b0b02330900000000600130098a092c00ea0000002c0058010b02df008500740d00002c00a10cb00163030100bc012100"),
		},
	}

	// Serialize.
	for i := range tests {
		test := tests[i]
		out := test.in.Serialize()
		if !bytes.Equal(out, test.out) {
			t.Errorf("bad result on test.in.Serialize(): got %x, want %x",
				out, test.out)
		}
	}

	// Deserialize.
	for i := range tests {
		test := tests[i]
		var tally RollingVotingPrefixTally
		err := tally.Deserialize(test.out)
		if err != nil {
			t.Errorf("unexpected error %v", err)
		}
		if !reflect.DeepEqual(tally, test.in) {
			t.Errorf("bad result on tally.Deserialize() test %v: got %v, "+
				"want %v", test.name, tally, test.in)
		}
	}

	// Test short read error.
	for i := range tests {
		test := tests[i]
		var tally RollingVotingPrefixTally
		err := tally.Deserialize(test.out[:len(test.out)-1])
		if err == nil ||
			err.(RuleError).ErrorCode != ErrMemoryCorruption {
			t.Errorf("expected ErrMemoryCorruption on test %v, got %v",
				test.name, err)
		}
	}
}

// TestBitsSliceAddingAndSubstracting tests adding and then substracting some vote
// bits to/from a tally.
func TestBitsSliceAddingAndSubstracting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tally    RollingVotingPrefixTally
		votebits []uint16
		out      RollingVotingPrefixTally
	}{
		{
			"simple addition of 5 votebits",
			RollingVotingPrefixTally{
				LastIntervalBlock:  BlockKey{Hash: chainhash.Hash{byte(0x02)}, Height: 38859},
				CurrentBlockHeight: 10200,
				BlockValid:         6,
				Unused:             7,
				Issues: [7]VotingTally{
					VotingTally{5, 4, 3, 2},
					VotingTally{5, 4, 3, 2},
					VotingTally{5, 4, 3, 2},
					VotingTally{5, 4, 3, 2},
					VotingTally{5, 4, 3, 2},
					VotingTally{5, 4, 3, 2},
					VotingTally{5, 4, 3, 2},
				},
			},
			// Add 3x "yes and odd issues yes, even issues no", 1x "yes and odd
			// issues no, even issues abstain", 1x "yes and unused yes,
			// rest undeclared"
			// Total:
			//   +5 BlockValid
			//   +1 Unused
			//   +3 Yes on all odd issues
			//   +3 No on all even issues
			//   +1 No on all odd issues
			//   +1 Abstain on all even issues
			[]uint16{0x6665, 0xBBB9, 0x0003, 0x6665, 0x6665},
			RollingVotingPrefixTally{
				LastIntervalBlock:  BlockKey{Hash: chainhash.Hash{byte(0x02)}, Height: 38859},
				CurrentBlockHeight: 10200,
				BlockValid:         6 + 5,
				Unused:             7 + 1,
				Issues: [7]VotingTally{
					VotingTally{5 + 1, 4 + 3, 3 + 1, 2}, // #1
					VotingTally{5 + 1, 4, 3 + 3, 2 + 1}, // #2
					VotingTally{5 + 1, 4 + 3, 3 + 1, 2}, // #3
					VotingTally{5 + 1, 4, 3 + 3, 2 + 1}, // #4
					VotingTally{5 + 1, 4 + 3, 3 + 1, 2}, // #5
					VotingTally{5 + 1, 4, 3 + 3, 2 + 1}, // #6
					VotingTally{5 + 1, 4 + 3, 3 + 1, 2}, // #7
				},
			},
		},
	}

	for i := range tests {
		testTally := tests[i].tally
		testTally.AddVoteBitsSlice(tests[i].votebits)
		if !reflect.DeepEqual(testTally, tests[i].out) {
			t.Errorf("bad result on AddVoteBitsSlice test %v: got %v, "+
				"want %v", tests[i].name, testTally, tests[i].out)
		}

		testTally.SubtractVoteBitsSlice(tests[i].votebits)
		if !reflect.DeepEqual(testTally, tests[i].tally) {
			t.Errorf("bad result on SubtractVoteBitsSlice test %v: got %v, "+
				"want %v", tests[i].name, testTally, tests[i].tally)
		}
	}
}

// TestAddingTallies tests adding and then substracting some vote
// bits to/from a tally.
func TestAddingTallies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tally1 RollingVotingPrefixTally
		tally2 RollingVotingPrefixTally
		out    RollingVotingPrefixTally
	}{
		{
			"simple addition of 1 or 2 to every field",
			RollingVotingPrefixTally{
				LastIntervalBlock:  BlockKey{Hash: chainhash.Hash{byte(0x02)}, Height: 38859},
				CurrentBlockHeight: 10200,
				BlockValid:         6,
				Unused:             7,
				Issues: [7]VotingTally{
					VotingTally{5, 4, 3, 2},
					VotingTally{5, 4, 3, 2},
					VotingTally{5, 4, 3, 2},
					VotingTally{5, 4, 3, 2},
					VotingTally{5, 4, 3, 2},
					VotingTally{5, 4, 3, 2},
					VotingTally{5, 4, 3, 2},
				},
			},
			RollingVotingPrefixTally{
				LastIntervalBlock:  BlockKey{Hash: chainhash.Hash{byte(0x02)}, Height: 38859},
				CurrentBlockHeight: 10200,
				BlockValid:         1,
				Unused:             2,
				Issues: [7]VotingTally{
					VotingTally{1, 2, 1, 2},
					VotingTally{2, 1, 2, 1},
					VotingTally{1, 2, 1, 2},
					VotingTally{2, 1, 2, 1},
					VotingTally{1, 2, 1, 2},
					VotingTally{2, 1, 2, 1},
					VotingTally{1, 2, 1, 2},
				},
			},
			RollingVotingPrefixTally{
				LastIntervalBlock:  BlockKey{Hash: chainhash.Hash{byte(0x02)}, Height: 38859},
				CurrentBlockHeight: 10200,
				BlockValid:         6 + 1,
				Unused:             7 + 2,
				Issues: [7]VotingTally{
					VotingTally{5 + 1, 4 + 2, 3 + 1, 2 + 2},
					VotingTally{5 + 2, 4 + 1, 3 + 2, 2 + 1},
					VotingTally{5 + 1, 4 + 2, 3 + 1, 2 + 2},
					VotingTally{5 + 2, 4 + 1, 3 + 2, 2 + 1},
					VotingTally{5 + 1, 4 + 2, 3 + 1, 2 + 2},
					VotingTally{5 + 2, 4 + 1, 3 + 2, 2 + 1},
					VotingTally{5 + 1, 4 + 2, 3 + 1, 2 + 2},
				},
			},
		},
	}

	for i := range tests {
		testTally := tests[i].tally1
		testTally.AddTally(tests[i].tally2)
		if !reflect.DeepEqual(testTally, tests[i].out) {
			t.Errorf("bad result on addTally test %v: got %v, "+
				"want %v", tests[i].name, testTally, tests[i].out)
		}
	}
}

// pruneTallyCache prunes old blocks from a cache that are more than initCacheSize
// blocks ago.
func pruneTallyCache(cache RollingVotingPrefixTallyCache, height int64, params *chaincfg.Params) {
	toRemove := make([]BlockKey, 0)
	cutoff := uint32(0)
	if height-params.StakeDiffWindowSize*initCacheSize > 0 {
		cutoff = uint32(height - params.StakeDiffWindowSize*initCacheSize)
	}
	for k, _ := range cache {
		if k.Height <= cutoff {
			toRemove = append(toRemove, k)
		}
	}

	for _, k := range toRemove {
		delete(cache, k)
	}
}

// rollingTallyCacheSliceTest is a sortable rolling tally cache used for
// debugging the rolling tally cache.
type rollingTallyCacheSliceTest []RollingVotingPrefixTally

// Len satisfies the sort interface.
func (s rollingTallyCacheSliceTest) Len() int {
	return len(s)
}

// Swap satisfies the sort interface.
func (s rollingTallyCacheSliceTest) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less satisfies the sort interface.
func (s rollingTallyCacheSliceTest) Less(i, j int) bool {
	return s[i].CurrentBlockHeight < s[j].CurrentBlockHeight
}

// numBlocks is the number of "blocks" to add/remove below, including the
// genesis block (added as 1 below) which is automatically skipped.  That is,
// if you want to generate 8064 blocks on top of the genesis block, use
// 8064 below.
const numBlocks = 99934

// sortIntervalCache takes a RollingVotingPrefixTallyCache and returns a sorted
// slice of all the elements, sorting by height.
func sortIntervalCache(cache RollingVotingPrefixTallyCache) []RollingVotingPrefixTally {
	s := make(rollingTallyCacheSliceTest, len(cache))

	i := 0
	for _, v := range cache {
		s[i] = *v
		i++
	}

	sort.Sort(s)

	return s
}

// TestVotingDbAndSpoofedChain tests block connection, disconnect, and
// a spoofed blockchain.
func TestVotingDbAndSpoofedChain(t *testing.T) {
	// Setup the database.
	params := &chaincfg.MainNetParams
	dbName := "ffldb_votingtest"
	dbPath := filepath.Join(testDbRoot, dbName)
	_ = os.RemoveAll(dbPath)
	testDb, err := database.Create(testDbType, dbPath, params.Net)
	if err != nil {
		t.Fatalf("error creating db: %v", err)
	}

	// Setup a teardown.
	defer os.RemoveAll(dbPath)
	defer os.RemoveAll(testDbRoot)
	defer testDb.Close()

	// Create the buckets and best state for the genesis
	// block.
	var tally *RollingVotingPrefixTally
	err = testDb.Update(func(dbTx database.Tx) error {
		tally, err = InitVotingDatabaseState(dbTx, params)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		t.Fatalf("error initializing voting db: %v", err)
	}

	// Load the cache.
	var cache RollingVotingPrefixTallyCache
	err = testDb.View(func(dbTx database.Tx) error {
		cache, err = InitRollingTallyCache(dbTx, params)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		t.Fatalf("error initializing cache: %v", err)
	}

	// Start adding some "blocks".
	bestTally := *tally
	var talliesForward [numBlocks]RollingVotingPrefixTally
	vbSlice := []uint16{}
	err = testDb.Update(func(dbTx database.Tx) error {
		for i := 1; i <= numBlocks; i++ {
			if int64(i) >= chaincfg.MainNetParams.StakeValidationHeight {
				switch i % 5 {
				case 0:
					vbSlice = []uint16{0x6665, 0x2345, 0x9999, 0xa0a1, 0xc432}
				case 1:
					vbSlice = []uint16{0x6687, 0x6689, 0x66bb, 0x66bb, 0x66e1}
				case 2:
					vbSlice = []uint16{0x3465, 0x5565, 0x6165, 0x65b6, 0xaaaa}
				case 3:
					vbSlice = []uint16{0xffff, 0x0000, 0x2342, 0x6444, 0xa333}
				case 4:
					vbSlice = []uint16{0x231b, 0xa343, 0xff34, 0x90bb}
				}
			}

			bestTally, err = bestTally.ConnectBlockToTally(cache, dbTx,
				chainhash.Hash{byte(i)}, chainhash.Hash{byte(i - 1)}, uint32(i),
				vbSlice, params)
			if err != nil {
				return err
			}

			talliesForward[i-1] = bestTally

			err = WriteConnectedBlockTally(dbTx, chainhash.Hash{byte(i)},
				uint32(i), &bestTally, params)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error adding blocks: %v", err)
	}

	var summedTally *RollingSummedTally
	err = testDb.View(func(dbTx database.Tx) error {
		lastBestInterval, err :=
			FetchIntervalTally(&bestTally.LastIntervalBlock, cache, dbTx,
				params)
		if err != nil {
			return err
		}

		summedTally, err = lastBestInterval.GenerateSummedTally(cache,
			dbTx, chaincfg.MainNetParams.VotingIntervals,
			params)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		t.Fatalf("unexpected fetching summedTally: %v", err)
	}

	// Go backwards, seeing if the state can be reverted.
	var talliesBackward [numBlocks]RollingVotingPrefixTally
	for i := numBlocks; i >= 1; i-- {
		if int64(i) < chaincfg.MainNetParams.StakeValidationHeight {
			vbSlice = []uint16{}
		}
		if int64(i) >= chaincfg.MainNetParams.StakeValidationHeight {
			switch i % 5 {
			case 0:
				vbSlice = []uint16{0x6665, 0x2345, 0x9999, 0xa0a1, 0xc432}
			case 1:
				vbSlice = []uint16{0x6687, 0x6689, 0x66bb, 0x66bb, 0x66e1}
			case 2:
				vbSlice = []uint16{0x3465, 0x5565, 0x6165, 0x65b6, 0xaaaa}
			case 3:
				vbSlice = []uint16{0xffff, 0x0000, 0x2342, 0x6444, 0xa333}
			case 4:
				vbSlice = []uint16{0x231b, 0xa343, 0xff34, 0x90bb}
			}
		}

		talliesBackward[i-1] = bestTally

		bestTally, err = bestTally.DisconnectBlockFromTally(cache, nil,
			chainhash.Hash{byte(i)}, uint32(i), vbSlice, nil,
			params)
		if err != nil {
			t.Fatalf("unexpected error removing blocks: %v", err)
		}
	}

	for i := len(talliesForward) - 1; i >= 0; i-- {
		if talliesForward[i] != talliesBackward[i] {
			t.Fatalf("non-equivalent disconnection tallies at height %v:"+
				" backward %v, forward %v", i, talliesBackward[i],
				talliesForward[i])
		}
	}

	// Do it again, loading from the database and going backwards this
	// time.  Reload the cache and the best tally manually.  Prune the
	// old cache according to height and make sure it's 1:1 with the
	// previously generated cache.
	pruneTallyCache(cache, numBlocks, params)
	oldBestCache := cache
	err = testDb.View(func(dbTx database.Tx) error {
		cache, err = InitRollingTallyCache(dbTx, params)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		t.Fatalf("error initializing cache going backwards: %v", err)
	}
	if !reflect.DeepEqual(oldBestCache, cache) {
		t.Fatalf("caches aren't same: got %v, want %v",
			sortIntervalCache(oldBestCache), sortIntervalCache(cache))
	}

	err = testDb.View(func(dbTx database.Tx) error {
		bestTallyPtr, err := LoadVotingDatabaseState(dbTx)
		if err != nil {
			return err
		}
		bestTally = *bestTallyPtr

		return nil
	})
	if err != nil {
		t.Fatalf("error reloading the best state: %v", err)
	}

	err = testDb.Update(func(dbTx database.Tx) error {
		for i := numBlocks; i >= 1; i-- {
			if int64(i) < chaincfg.MainNetParams.StakeValidationHeight {
				vbSlice = []uint16{}
			}
			if int64(i) >= chaincfg.MainNetParams.StakeValidationHeight {
				switch i % 5 {
				case 0:
					vbSlice = []uint16{0x6665, 0x2345, 0x9999, 0xa0a1, 0xc432}
				case 1:
					vbSlice = []uint16{0x6687, 0x6689, 0x66bb, 0x66bb, 0x66e1}
				case 2:
					vbSlice = []uint16{0x3465, 0x5565, 0x6165, 0x65b6, 0xaaaa}
				case 3:
					vbSlice = []uint16{0xffff, 0x0000, 0x2342, 0x6444, 0xa333}
				case 4:
					vbSlice = []uint16{0x231b, 0xa343, 0xff34, 0x90bb}
				}
			}

			bestTallyCopy := bestTally
			talliesBackward[i-1] = bestTally

			bestTally, err = bestTally.DisconnectBlockFromTally(cache, dbTx,
				chainhash.Hash{byte(i)}, uint32(i), vbSlice, nil,
				params)
			if err != nil {
				return err
			}

			err = WriteDisconnectedBlockTally(dbTx, chainhash.Hash{byte(i)},
				chainhash.Hash{byte(i - 1)}, uint32(i), &bestTallyCopy, vbSlice,
				params)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error removing blocks: %v", err)
	}

	for i := range talliesForward {
		if talliesForward[i] != talliesBackward[i] {
			t.Fatalf("non-equivalent disconnection tallies at height %v:"+
				" backward %v, forward %v", i, talliesBackward[i],
				talliesForward[i])
		}
	}
}

// TestTallyingAndVerdicts tests the tallying and verdict supplying functions
// to ensure that the operate correctly, even in edge cases.
func TestTallyingAndVerdicts(t *testing.T) {
	t.Parallel()
	params := &chaincfg.MainNetParams

	tests := []struct {
		name      string
		numBlocks int64
		intervals int
		votebits  func(int64, int64) []uint16
		verdict   [issuesLen]Verdict
		err       error
	}{
		{
			"issue #1 is no, issue #2 is no, issue #3 is yes, issue #4 is no",
			49968, // 347 intervals
			params.VotingIntervals,
			func(i, numBlocks int64) []uint16 {
				if i >= chaincfg.MainNetParams.StakeValidationHeight {
					switch i % 5 {
					case 0:
						return []uint16{0x0241, 0x0241, 0x0241, 0x0241, 0xc432}
					case 1:
						return []uint16{0x0241, 0x0241, 0x0241, 0x0241, 0x66e1}
					case 2:
						return []uint16{0x0241, 0x0241, 0x0241, 0x0241, 0xaaaa}
					case 3:
						return []uint16{0x0241, 0x0241, 0x0241, 0x0241, 0x0901}
					case 4:
						return []uint16{0x0241, 0x0241, 0x0241, 0x90bb}
					}
				}

				return []uint16{}
			},
			[issuesLen]Verdict{VerdictNo, VerdictNo,
				VerdictYes, VerdictNo, VerdictUndecided,
				VerdictUndecided, VerdictUndecided},
			nil,
		},
		{
			"issue #3 is yes, issue #4 is no by 1 vote",
			49968, // 347 intervals
			params.VotingIntervals,
			func(i, numBlocks int64) []uint16 {
				if i >= chaincfg.MainNetParams.StakeValidationHeight {
					if i == numBlocks-1 {
						return []uint16{0x0141, 0x0141, 0x0141, 0x0241, 0x0241}
					}

					switch i % 4 {
					case 0:
						return []uint16{0x0241, 0x0241, 0x0241, 0x0241, 0x0241}
					case 1:
						return []uint16{0x0241, 0x0241, 0x0241, 0x0241, 0x0241}
					case 2:
						return []uint16{0x0141, 0x0141, 0x0241, 0x0241, 0x0241}
					case 3:
						return []uint16{0x0141, 0x0141, 0x0141, 0x0241, 0x0241}
					}
				}

				return []uint16{}
			},
			[issuesLen]Verdict{VerdictUndecided, VerdictUndecided,
				VerdictYes, VerdictNo, VerdictUndecided,
				VerdictUndecided, VerdictUndecided},
			nil,
		},
		{
			"issue #3 is yes, issue #4 is undecided by 1 vote",
			49968, // 347 intervals
			params.VotingIntervals,
			func(i, numBlocks int64) []uint16 {
				if i >= chaincfg.MainNetParams.StakeValidationHeight {
					if i == numBlocks-1 {
						return []uint16{0x0141, 0x0141, 0x0141, 0x0241, 0x0141}
					}

					switch i % 4 {
					case 0:
						return []uint16{0x0241, 0x0241, 0x0241, 0x0241, 0x0241}
					case 1:
						return []uint16{0x0241, 0x0241, 0x0241, 0x0241, 0x0241}
					case 2:
						return []uint16{0x0141, 0x0141, 0x0241, 0x0241, 0x0241}
					case 3:
						return []uint16{0x0141, 0x0141, 0x0141, 0x0241, 0x0241}
					}
				}

				return []uint16{}
			},
			[issuesLen]Verdict{VerdictUndecided, VerdictUndecided,
				VerdictYes, VerdictUndecided, VerdictUndecided,
				VerdictUndecided, VerdictUndecided},
			nil,
		},
		{
			"issue #3 single yes vote, rest abstain leading to yes",
			49968, // 347 intervals
			params.VotingIntervals,
			func(i, numBlocks int64) []uint16 {
				if i >= chaincfg.MainNetParams.StakeValidationHeight {
					switch i % 144 {
					case 0:
						return []uint16{0x0041, 0x00c1, 0x00c1, 0x00c1, 0x00c1}
					default:
						return []uint16{0x00c1, 0x00c1, 0x00c1, 0x00c1, 0x00c1}
					}
				}

				return []uint16{}
			},
			[issuesLen]Verdict{VerdictUndecided, VerdictUndecided,
				VerdictYes, VerdictUndecided, VerdictUndecided,
				VerdictUndecided, VerdictUndecided},
			nil,
		},
		{
			"issue #3 single no vote, rest abstain leading to no",
			49968, // 347 intervals
			params.VotingIntervals,
			func(i, numBlocks int64) []uint16 {
				if i >= chaincfg.MainNetParams.StakeValidationHeight {
					switch i % 144 {
					case 0:
						return []uint16{0x0081, 0x00c1, 0x00c1, 0x00c1, 0x00c1}
					default:
						return []uint16{0x00c1, 0x00c1, 0x00c1, 0x00c1, 0x00c1}
					}
				}

				return []uint16{}
			},
			[issuesLen]Verdict{VerdictUndecided, VerdictUndecided,
				VerdictNo, VerdictUndecided, VerdictUndecided,
				VerdictUndecided, VerdictUndecided},
			nil,
		},
		{
			"all issues yes",
			49968, // 347 intervals
			params.VotingIntervals,
			func(i, numBlocks int64) []uint16 {
				if i >= chaincfg.MainNetParams.StakeValidationHeight {
					return []uint16{0x5555, 0x5555, 0x5555, 0x5555, 0x5555}
				}

				return []uint16{}
			},
			[issuesLen]Verdict{VerdictYes, VerdictYes,
				VerdictYes, VerdictYes, VerdictYes,
				VerdictYes, VerdictYes},
			nil,
		},
		{
			"all issues no",
			49968, // 347 intervals
			params.VotingIntervals,
			func(i, numBlocks int64) []uint16 {
				if i >= chaincfg.MainNetParams.StakeValidationHeight {
					return []uint16{0xaaa9, 0xaaa9, 0xaaa9, 0xaaa9, 0xaaa9}
				}

				return []uint16{}
			},
			[issuesLen]Verdict{VerdictNo, VerdictNo,
				VerdictNo, VerdictNo, VerdictNo,
				VerdictNo, VerdictNo},
			nil,
		},
		{
			"only 3 of 5 voters, issue #3 is yes, issue #4 is no",
			49968, // 347 intervals
			params.VotingIntervals,
			func(i, numBlocks int64) []uint16 {
				if i >= chaincfg.MainNetParams.StakeValidationHeight {
					switch i % 4 {
					case 0:
						return []uint16{0x0241, 0x0241, 0x0241}
					case 1:
						return []uint16{0x0241, 0x0241, 0x0241}
					case 2:
						return []uint16{0x0141, 0x0241, 0x0241}
					case 3:
						return []uint16{0x0141, 0x0141, 0x0241}
					}
				}

				return []uint16{}
			},
			[issuesLen]Verdict{VerdictUndecided, VerdictUndecided,
				VerdictYes, VerdictNo, VerdictUndecided,
				VerdictUndecided, VerdictUndecided},
			nil,
		},
		{
			"error wrong height",
			49967, // short 1 block, not an interval block
			params.VotingIntervals,
			func(i, numBlocks int64) []uint16 {
				if i >= chaincfg.MainNetParams.StakeValidationHeight {
					return []uint16{0x0241, 0x0241, 0x0241, 0x0241, 0x0241}
				}

				return []uint16{}
			},
			[issuesLen]Verdict{VerdictUndecided, VerdictUndecided,
				VerdictYes, VerdictNo, VerdictUndecided,
				VerdictUndecided, VerdictUndecided},
			stakeRuleError(ErrTallyingIntervals, ""),
		},
		{
			"too many voting intervals",
			49968, // short 1 block, not an interval block
			348,
			func(i, numBlocks int64) []uint16 {
				if i >= chaincfg.MainNetParams.StakeValidationHeight {
					return []uint16{0x0241, 0x0241, 0x0241, 0x0241, 0x0241}
				}

				return []uint16{}
			},
			[issuesLen]Verdict{VerdictUndecided, VerdictUndecided,
				VerdictYes, VerdictNo, VerdictUndecided,
				VerdictUndecided, VerdictUndecided},
			stakeRuleError(ErrTallyingIntervals, ""),
		},
	}

	for _, test := range tests {
		// Set up the cache and genesis block rolling tally.
		cache := make(RollingVotingPrefixTallyCache)
		var bestTally RollingVotingPrefixTally
		bestTally.LastIntervalBlock = BlockKey{*params.GenesisHash, 0}

		for i := int64(1); i < test.numBlocks; i++ {
			// Skip using the database.
			var err error
			bestTally, err = bestTally.ConnectBlockToTally(cache, nil,
				chainhash.Hash{byte(i)}, chainhash.Hash{byte(i - 1)}, uint32(i),
				test.votebits(i, test.numBlocks),
				&chaincfg.MainNetParams)
			if err != nil {
				t.Fatalf("failed connecting tally %v", err)
			}
		}

		verdicts, err := bestTally.GenerateVotingResults(cache,
			nil, test.intervals, &chaincfg.MainNetParams)
		if err != nil && test.err == nil {
			t.Fatalf("failed generating verdicts %v", err)
		}
		if err != nil && test.err != nil {
			if err.(RuleError).ErrorCode != test.err.(RuleError).ErrorCode {
				t.Fatalf("got incorrect error for test %v; got %v, want %v",
					test.name, err.(RuleError).ErrorCode,
					test.err.(RuleError).ErrorCode)
			}
			continue
		}
		if err == nil && test.err != nil {
			t.Fatalf("expecting err %v for test %v, got no error",
				test.err.(RuleError).ErrorCode, test.name)
			continue
		}

		if !reflect.DeepEqual(test.verdict, verdicts.Verdicts) {
			t.Fatalf("unexpected verdicts on test '%v', got %v want %v",
				test.name, verdicts.Verdicts, test.verdict)
		}
	}
}
