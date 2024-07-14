// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// Tests originally written by Dave Collins July 2024.

//go:build !purego

package compress

import (
	"testing"
)

// TestBlocksAMD64 ensures the each of the specialized amd64 block compression
// function implementations return the expected results.
//
// Note that any tests which require instruction sets that aren't available on
// the system executing the tests are skipped.
func TestBlocksAMD64(t *testing.T) {
	// NOTE: This is intentionally not made parallel because it modifies the
	// global feature flags to ensure all supported variants are tested.

	pureGo := true
	type blockVariantTest struct {
		name        string
		featureFlag *bool
	}
	variants := []blockVariantTest{
		{name: "Pure Go", featureFlag: &pureGo},
		{name: "SSE2", featureFlag: &hasSSE2},
		{name: "SSE41", featureFlag: &hasSSE41},
		{name: "AVX", featureFlag: &hasAVX},
	}

	// Skip any features that the hardware does not support or have explicitly
	// been disabled.
	tests := make([]blockVariantTest, 0, len(variants))
	for _, variant := range variants {
		if !*variant.featureFlag {
			t.Logf("Skipping %s tests (disabled or no instruction set support)",
				variant.name)
			continue
		}
		tests = append(tests, variant)
	}

	// Restore the feature flags after the tests complete.
	defer func() {
		for _, test := range tests {
			*test.featureFlag = true
		}
	}()

	for _, test := range tests {
		// Ensure only the specific feature flag for this test is enabled.
		for _, test := range tests {
			*test.featureFlag = false
		}
		*test.featureFlag = true

		t.Run(test.name, func(t *testing.T) {
			for i := range blockVecs {
				test := &blockVecs[i]

				state := State{CV: test.h, S: test.s}
				Blocks(&state, test.msg[:], test.cnt)
				if state.CV != test.want {
					t.Fatalf("%q: unexpected result -- got %08x, want %08x",
						test.name, state.CV, test.want)
				}
			}
		})
	}
}
