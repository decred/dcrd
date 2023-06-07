// Copyright (c) 2019-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package standalone

import (
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// TestHashToBig ensures HashToBig properly converts a hash treated as a little
// endian unsigned 256-bit value to a big integer encoded with big endian.
func TestHashToBig(t *testing.T) {
	tests := []struct {
		name string // test description
		hash string // hash to convert
		want string // expected bit integer bytes in hex
	}{{
		name: "mainnet block 1 hash",
		hash: "000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9",
		want: "000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9",
	}, {
		name: "mainnet block 2 hash",
		hash: "000000000000c41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba",
		want: "000000000000c41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba",
	}}

	for _, test := range tests {
		hash, err := chainhash.NewHashFromStr(test.hash)
		if err != nil {
			t.Errorf("%q: unexpected err parsing test hash: %v", test.name, err)
			continue
		}

		want, success := new(big.Int).SetString(test.want, 16)
		if !success {
			t.Errorf("%q: unexpected err parsing test result", test.name)
			continue
		}

		result := HashToBig(hash)
		if result.Cmp(want) != 0 {
			t.Errorf("%s: unexpected result -- got %x, want %x", test.name,
				result, want)
			continue
		}
	}
}

// TestBigToCompact ensures converting from big integers to the compact
// representation used for target difficulties produces the correct results.
func TestBigToCompact(t *testing.T) {
	tests := []struct {
		name  string // test description
		input string // big integer to test
		want  uint32 // expected compact value
	}{{
		name:  "mainnet block 1",
		input: "0x000000000001ffff000000000000000000000000000000000000000000000000",
		want:  0x1b01ffff,
	}, {
		name:  "mainnet block 288",
		input: "0x000000000001330e000000000000000000000000000000000000000000000000",
		want:  0x1b01330e,
	}, {
		name:  "higher diff (exponent 24, sign bit 0, mantissa 0x5fb28a)",
		input: "0x00000000000000005fb28a000000000000000000000000000000000000000000",
		want:  0x185fb28a,
	}, {
		name:  "zero",
		input: "0",
		want:  0,
	}, {
		name:  "-1 (exponent 1, sign bit 1, mantissa 0x10000)",
		input: "-1",
		want:  0x1810000,
	}, {
		name:  "-128 (exponent 2, sign bit 1, mantissa 0x08000)",
		input: "-128",
		want:  0x2808000,
	}, {
		name:  "-32768 (exponent 3, sign bit 1, mantissa 0x08000)",
		input: "-32768",
		want:  0x3808000,
	}, {
		name:  "-8388608 (exponent 4, sign bit 1, mantissa 0x08000)",
		input: "-8388608",
		want:  0x4808000,
	}}

	for _, test := range tests {
		input, success := new(big.Int).SetString(test.input, 0)
		if !success {
			t.Errorf("%q: unexpected err parsing test input", test.name)
			continue
		}

		result := BigToCompact(input)
		if result != test.want {
			t.Errorf("%q: mismatched result -- got %x, want %x", test.name,
				result, test.want)
			continue
		}
	}
}

// TestCompactToBig ensures converting from the compact representation used for
// target difficulties to big integers produces the correct results.
func TestCompactToBig(t *testing.T) {
	tests := []struct {
		name  string // test description
		input uint32 // compact target difficulty bits to test
		want  string // expected big int
	}{{
		name:  "mainnet block 1",
		input: 0x1b01ffff,
		want:  "0x000000000001ffff000000000000000000000000000000000000000000000000",
	}, {
		name:  "mainnet block 288",
		input: 0x1b01330e,
		want:  "0x000000000001330e000000000000000000000000000000000000000000000000",
	}, {
		name:  "higher diff (exponent 24, sign bit 0, mantissa 0x5fb28a)",
		input: 0x185fb28a,
		want:  "0x00000000000000005fb28a000000000000000000000000000000000000000000",
	}, {
		name:  "zero",
		input: 0,
		want:  "0",
	}, {
		name:  "-1 (exponent 1, sign bit 1, mantissa 0x10000)",
		input: 0x1810000,
		want:  "-1",
	}, {
		name:  "-128 (exponent 2, sign bit 1, mantissa 0x08000)",
		input: 0x2808000,
		want:  "-128",
	}, {
		name:  "-32768 (exponent 3, sign bit 1, mantissa 0x08000)",
		input: 0x3808000,
		want:  "-32768",
	}, {
		name:  "-8388608 (exponent 4, sign bit 1, mantissa 0x08000)",
		input: 0x4808000,
		want:  "-8388608",
	}}

	for _, test := range tests {
		want, success := new(big.Int).SetString(test.want, 0)
		if !success {
			t.Errorf("%q: unexpected err parsing expected value", test.name)
			continue
		}

		result := CompactToBig(test.input)
		if result.Cmp(want) != 0 {
			t.Errorf("%q: mismatched result -- got %x, want %x", test.name,
				result, want)
			continue
		}
	}
}

// TestCalcWork ensures calculating a work value from a compact target
// difficulty produces the correct results.
func TestCalcWork(t *testing.T) {
	tests := []struct {
		name  string // test description
		input uint32 // compact target difficulty bits to test
		want  string // expected big int
	}{{
		name:  "mainnet block 1",
		input: 0x1b01ffff,
		want:  "0x0000000000000000000000000000000000000000000000000000800040002000",
	}, {
		name:  "mainnet block 288",
		input: 0x1b01330e,
		want:  "0x0000000000000000000000000000000000000000000000000000d56f2dcbe105",
	}, {
		name:  "higher diff (exponent 24)",
		input: 0x185fb28a,
		want:  "0x000000000000000000000000000000000000000000000002acd33ddd458512da",
	}, {
		name:  "zero",
		input: 0,
		want:  "0",
	}, {
		name:  "negative target difficulty",
		input: 0x1810000,
		want:  "0",
	}}

	for _, test := range tests {
		want, success := new(big.Int).SetString(test.want, 0)
		if !success {
			t.Errorf("%q: unexpected err parsing expected value", test.name)
			continue
		}

		result := CalcWork(test.input)
		if result.Cmp(want) != 0 {
			t.Errorf("%q: mismatched result -- got %x, want %x", test.name,
				result, want)
			continue
		}
	}
}

// mockMainNetPowLimit returns the pow limit for the main network as of the
// time this comment was written.  It is used to ensure the tests are stable
// independent of any potential changes to chain parameters.
func mockMainNetPowLimit() string {
	return "00000000ffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
}

// TestCheckProofOfWorkRange ensures target difficulties that are outside of
// the acceptable ranges are detected as an error and those inside are not.
func TestCheckProofOfWorkRange(t *testing.T) {
	tests := []struct {
		name     string // test description
		bits     uint32 // compact target difficulty bits to test
		powLimit string // proof of work limit
		err      error  // expected error
	}{{
		name:     "mainnet block 1",
		bits:     0x1b01ffff,
		powLimit: mockMainNetPowLimit(),
		err:      nil,
	}, {
		name:     "mainnet block 288",
		bits:     0x1b01330e,
		powLimit: mockMainNetPowLimit(),
		err:      nil,
	}, {
		name:     "smallest allowed",
		bits:     0x1010000,
		powLimit: mockMainNetPowLimit(),
		err:      nil,
	}, {
		name:     "max allowed (exactly the pow limit)",
		bits:     0x1d00ffff,
		powLimit: mockMainNetPowLimit(),
		err:      nil,
	}, {
		name:     "zero",
		bits:     0,
		powLimit: mockMainNetPowLimit(),
		err:      ErrUnexpectedDifficulty,
	}, {
		name:     "negative",
		bits:     0x1810000,
		powLimit: mockMainNetPowLimit(),
		err:      ErrUnexpectedDifficulty,
	}, {
		name:     "pow limit + 1",
		bits:     0x1d010000,
		powLimit: mockMainNetPowLimit(),
		err:      ErrUnexpectedDifficulty,
	}}

	for _, test := range tests {
		powLimit, success := new(big.Int).SetString(test.powLimit, 16)
		if !success {
			t.Errorf("%q: unexpected err parsing test pow limit", test.name)
			continue
		}

		err := CheckProofOfWorkRange(test.bits, powLimit)
		if !errors.Is(err, test.err) {
			t.Errorf("%q: unexpected err -- got %v, want %v", test.name, err,
				test.err)
			continue
		}
	}
}

// TestCheckProofOfWorkHash ensures hashes that do not satisfy a given target
// difficulty are detected as an error and those that do are not.
func TestCheckProofOfWorkHash(t *testing.T) {
	tests := []struct {
		name string // test description
		hash string // proof of work hash to test
		bits uint32 // compact target difficulty bits to test
		err  error  // expected error
	}{{
		name: "mainnet block 1 pow hash",
		hash: "000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9",
		bits: 0x1b01ffff,
		err:  nil,
	}, {
		name: "mainnet block 288 pow hash",
		hash: "000000000000e0ab546b8fc19f6d94054d47ffa5fe79e17611d170662c8b702b",
		bits: 0x1b01330e,
		err:  nil,
	}, {
		name: "high hash",
		hash: "000000000001ffff000000000000000000000000000000000000000000000001",
		bits: 0x1b01ffff,
		err:  ErrHighHash,
	}}

	for _, test := range tests {
		hash, err := chainhash.NewHashFromStr(test.hash)
		if err != nil {
			t.Errorf("%q: unexpected err parsing test hash: %v", test.name, err)
			continue
		}

		err = CheckProofOfWorkHash(hash, test.bits)
		if !errors.Is(err, test.err) {
			t.Errorf("%q: unexpected err -- got %v, want %v", test.name, err,
				test.err)
			continue
		}
	}
}

// TestCheckProofOfWork ensures hashes and target difficulties that are outside
// of the acceptable ranges are detected as an error and those inside are not.
func TestCheckProofOfWork(t *testing.T) {
	tests := []struct {
		name     string // test description
		hash     string // proof of work hash to test
		bits     uint32 // compact target difficulty bits to test
		powLimit string // proof of work limit
		err      error  // expected error
	}{{
		name:     "mainnet block 1 pow hash",
		hash:     "000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9",
		bits:     0x1b01ffff,
		powLimit: mockMainNetPowLimit(),
		err:      nil,
	}, {
		name:     "mainnet block 288 pow hash",
		hash:     "000000000000e0ab546b8fc19f6d94054d47ffa5fe79e17611d170662c8b702b",
		bits:     0x1b01330e,
		powLimit: mockMainNetPowLimit(),
		err:      nil,
	}, {
		name:     "max allowed (exactly the pow limit)",
		hash:     "0000000000001ffff00000000000000000000000000000000000000000000000",
		bits:     0x1b01ffff,
		powLimit: mockMainNetPowLimit(),
		err:      nil,
	}, {
		name:     "high hash (pow limit + 1)",
		hash:     "000000000001ffff000000000000000000000000000000000000000000000001",
		bits:     0x1b01ffff,
		powLimit: mockMainNetPowLimit(),
		err:      ErrHighHash,
	}, {
		name:     "hash satisfies target, but target too high at pow limit + 1",
		hash:     "0000000000000000000000000000000000000000000000000000000000000001",
		bits:     0x1d010000,
		powLimit: mockMainNetPowLimit(),
		err:      ErrUnexpectedDifficulty,
	}, {
		name:     "zero target difficulty",
		hash:     "0000000000000000000000000000000000000000000000000000000000000001",
		bits:     0,
		powLimit: mockMainNetPowLimit(),
		err:      ErrUnexpectedDifficulty,
	}, {
		name:     "negative target difficulty",
		hash:     "0000000000000000000000000000000000000000000000000000000000000001",
		bits:     0x1810000,
		powLimit: mockMainNetPowLimit(),
		err:      ErrUnexpectedDifficulty,
	}}

	for _, test := range tests {
		hash, err := chainhash.NewHashFromStr(test.hash)
		if err != nil {
			t.Errorf("%q: unexpected err parsing test hash: %v", test.name, err)
			continue
		}

		powLimit, success := new(big.Int).SetString(test.powLimit, 16)
		if !success {
			t.Errorf("%q: unexpected err parsing test pow limit", test.name)
			continue
		}

		err = CheckProofOfWork(hash, test.bits, powLimit)
		if !errors.Is(err, test.err) {
			t.Errorf("%q: unexpected err -- got %v, want %v", test.name, err,
				test.err)
			continue
		}
	}
}

// TestCalcASERTDiff ensures the proof-of-work target difficulty calculation for
// the algorithm defined by DCP0011 works as expected by using the reference
// test vectors.
func TestCalcASERTDiff(t *testing.T) {
	t.Parallel()

	// Read and parse the reference test vectors.
	f, err := os.ReadFile(filepath.Join("testdata", "asert_test_vectors.json"))
	if err != nil {
		t.Fatalf("failed to read test vectors: %v", err)
	}
	var testData struct {
		Comments []string `json:"comments"`
		Params   map[string]struct {
			PowLimit           string `json:"powLimit"`
			PowLimitBits       uint32 `json:"powLimitBits"`
			TargetSecsPerBlock int64  `json:"targetSecsPerBlock"`
			HalfLifeSecs       int64  `json:"halfLifeSecs"`
		} `json:"params"`
		Scenarios []struct {
			Desc          string `json:"description"`
			Params        string `json:"params"`
			StartDiffBits uint32 `json:"startDiffBits"`
			StartHeight   int64  `json:"startHeight"`
			StartTime     int64  `json:"startTime"`
			Tests         []struct {
				Height           uint64 `json:"height"`
				Timestamp        int64  `json:"timestamp"`
				ExpectedDiffBits uint32 `json:"expectedDiffBits"`
			} `json:"tests"`
		} `json:"scenarios"`
	}
	err = json.Unmarshal(f, &testData)
	if err != nil {
		t.Fatal(err)
	}

	// Basic sanity check to ensure scenarios parsed.
	if len(testData.Scenarios) == 0 {
		t.Fatal("No test scenarios found")
	}

	for _, scenario := range testData.Scenarios {
		// Basic sanity check to ensure test cases parsed.
		if len(scenario.Tests) == 0 {
			t.Fatalf("%q: No test cases found", scenario.Desc)
		}

		// Lookup the associated network parameters and parse the proof of work
		// limit hexadecimal to a uint256.
		paramsKey := scenario.Params
		params, ok := testData.Params[paramsKey]
		if !ok {
			t.Errorf("%q: bad network params key %q", scenario.Desc, paramsKey)
			continue
		}
		powLimit, ok := new(big.Int).SetString(params.PowLimit, 16)
		if !ok {
			t.Errorf("%q: malformed pow limit %q", paramsKey, params.PowLimit)
			continue
		}

		for _, test := range scenario.Tests {
			// Calculate the time and height deltas from the test data.
			heightDelta := int64(test.Height - uint64(scenario.StartHeight))
			timeDelta := test.Timestamp - scenario.StartTime

			// Ensure the calculated difficulty matches the expected result.
			gotDiff := CalcASERTDiff(scenario.StartDiffBits, powLimit,
				params.TargetSecsPerBlock, timeDelta, heightDelta,
				params.HalfLifeSecs)
			if gotDiff != test.ExpectedDiffBits {
				t.Errorf("%q@height %d: did not get expected difficulty bits "+
					"-- got %08x, want %08x", scenario.Desc, test.Height,
					gotDiff, test.ExpectedDiffBits)
				continue
			}
		}
	}
}

// TestCalcASERTDiffPanics ensures the proof-of-work target difficulty
// calculation for the algorithm defined by DCP0011 panics when called
// improperly.
func TestCalcASERTDiffPanics(t *testing.T) {
	testPanic := func(fn func()) (paniced bool) {
		// Setup a defer to catch the expected panic and update the return
		// variable.
		defer func() {
			if err := recover(); err != nil {
				paniced = true
			}
		}()

		fn()
		return false
	}

	// Parameters used in the tests below.
	const (
		startDiffBits      = 0x1b00a5a6
		powLimitBits       = 0x1d00ffff
		targetSecsPerBlock = 300
		halfLifeSecs       = 43200
	)
	powLimit := CompactToBig(powLimitBits)

	// Ensure attempting to calculate a target difficulty with an invalid
	// starting target difficulty of 0 panics.
	paniced := testPanic(func() {
		CalcASERTDiff(0, powLimit, targetSecsPerBlock, 0, 0, halfLifeSecs)
	})
	if !paniced {
		t.Fatal("CalcASERTDiff did not panic with zero starting difficulty")
	}

	// Ensure attempting to calculate a target difficulty with a starting target
	// difficulty greater than the proof of work limit panics.
	paniced = testPanic(func() {
		invalidBits := uint32(powLimitBits + 1)
		CalcASERTDiff(invalidBits, powLimit, targetSecsPerBlock, 0, 0,
			halfLifeSecs)
	})
	if !paniced {
		t.Fatal("CalcASERTDiff did not panic with a starting difficulty " +
			"greater than the proof of work limit")
	}

	// Ensure attempting to calculate a target difficulty with a negative height
	// delta panics.
	paniced = testPanic(func() {
		CalcASERTDiff(startDiffBits, powLimit, targetSecsPerBlock, 0, -1,
			halfLifeSecs)
	})
	if !paniced {
		t.Fatal("CalcASERTDiff did not panic with a negative height delta")
	}
}
