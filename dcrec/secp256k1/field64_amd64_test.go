// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import "testing"

// TestField64AMD64 ensures each of the specialized amd64 [FieldVal64.Mul] and
// [FieldVal64.Square] implementations return the expected results.
//
// Note that any tests which require instruction sets that aren't available on
// the system executing the tests are skipped.
func TestField64AMD64(t *testing.T) {
	// NOTE: This is intentionally not made parallel because it modifies the
	// global feature flags to ensure all supported variants are tested.

	pureGo := true
	type mulVariantTest struct {
		name        string
		featureFlag *bool
	}
	variants := []mulVariantTest{
		{name: "Pure Go", featureFlag: &pureGo},
		{name: "BMI2/ADX", featureFlag: &field64UseBMI2AndADX},
	}

	// Restore the feature flags after the tests complete.
	origFlags := make(map[string]bool)
	for _, variant := range variants {
		origFlags[variant.name] = *variant.featureFlag
	}
	defer func() {
		for _, variant := range variants {
			*variant.featureFlag = origFlags[variant.name]
		}
	}()

	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			// Skip any features that the hardware does not support or have
			// explicitly been disabled.
			//
			// Note that this is intentionally not using t.Skipf because tinygo
			// does not support it.
			if !origFlags[variant.name] {
				t.Logf("Skipping %s tests (disabled or no instruction set "+
					"support)", variant.name)
				return
			}

			// Ensure only the specific feature flag for this test is enabled.
			for _, variant := range variants {
				*variant.featureFlag = false
			}
			*variant.featureFlag = true

			t.Run("Mul", func(t *testing.T) {
				testField64Mul(t)
			})
			t.Run("Square", func(t *testing.T) {
				testField64Square(t)
			})
		})
	}
}
