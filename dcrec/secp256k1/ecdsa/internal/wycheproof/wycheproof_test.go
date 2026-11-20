// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wycheproof

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

// ecdsaTest is one secp256k1 ECDSA verification vector.
type ecdsaTest struct {
	tcID     int
	msg, sig string
	valid    bool
}

// ecdsaTestGroup groups vectors for one public key.
type ecdsaTestGroup struct {
	pubKey string
	tests  []ecdsaTest
}

// testWycheproofECDSA checks the provided Wycheproof vectors.
func testWycheproofECDSA(t *testing.T, groups []ecdsaTestGroup, enforceLowS bool) {
	t.Helper()

	for groupIndex, group := range groups {
		pubKeyBytes, err := hex.DecodeString(group.pubKey)
		if err != nil {
			t.Fatalf("group %d: invalid public key hex: %v", groupIndex, err)
		}
		pubKey, err := secp256k1.ParsePubKey(pubKeyBytes)
		if err != nil {
			t.Fatalf("group %d: failed to parse public key: %v", groupIndex, err)
		}

		for _, test := range group.tests {
			testName := fmt.Sprintf("tcId %d", test.tcID)
			msg, err := hex.DecodeString(test.msg)
			if err != nil {
				t.Fatalf("%s: invalid message hex: %v", testName, err)
			}
			digest := sha256.Sum256(msg)

			sigBytes, err := hex.DecodeString(test.sig)
			if err != nil {
				t.Fatalf("%s: invalid signature hex: %v", testName, err)
			}

			sig, err := ecdsa.ParseDERSignature(sigBytes)
			if err != nil && test.valid {
				t.Errorf("%s: failed to parse valid signature: %v", testName,
					err)
				continue
			}

			verified := err == nil && sig.Verify(digest[:], pubKey)
			if enforceLowS && verified {
				s := sig.S()
				verified = !s.IsOverHalfOrder()
			}
			if verified != test.valid {
				t.Errorf("%s: unexpected verify result - got %v, want %v",
					testName, verified, test.valid)
			}
		}
	}
}

// TestWycheproofECDSAVerification runs the secp256k1 ECDSA verification test
// vectors published by Project Wycheproof.
func TestWycheproofECDSAVerification(t *testing.T) {
	t.Run("standard", func(t *testing.T) {
		const enforceLowS = false
		testWycheproofECDSA(t, ecdsaTestGroups, enforceLowS)
	})
	t.Run("bitcoin", func(t *testing.T) {
		const enforceLowS = true
		testWycheproofECDSA(t, ecdsaBitcoinTestGroups, enforceLowS)
	})
}
