// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ecdsa

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

//go:generate go run genwycheproof.go

type wycheproofECDSATest struct {
	tcID     int
	msg, sig string
	result   string
}

type wycheproofECDSATestGroup struct {
	publicKey string
	tests     []wycheproofECDSATest
}

// TestWycheproofECDSAVerification runs the secp256k1 ECDSA verification test
// vectors published by Project Wycheproof.
func TestWycheproofECDSAVerification(t *testing.T) {
	t.Run("standard", func(t *testing.T) {
		testWycheproofECDSATestGroups(t, wycheproofECDSATestGroups,
			verifyECDSASignature)
	})
	t.Run("bitcoin", func(t *testing.T) {
		testWycheproofECDSATestGroups(t, wycheproofECDSABitcoinTestGroups,
			verifyBitcoinECDSASignature)
	})
}

// testWycheproofECDSATestGroups checks the provided Wycheproof vectors.
func testWycheproofECDSATestGroups(t *testing.T, groups []wycheproofECDSATestGroup,
	verifySignature func(*Signature, []byte, *secp256k1.PublicKey) bool) {
	t.Helper()

	for groupIndex, group := range groups {
		pubKeyBytes, err := hex.DecodeString(group.publicKey)
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
			sig, err := ParseDERSignature(sigBytes)
			verified := err == nil && verifySignature(sig, digest[:], pubKey)

			switch test.result {
			case "valid":
				if err != nil {
					t.Errorf("%s: failed to parse valid signature: %v",
						testName, err)
					continue
				}
				if !verified {
					t.Errorf("%s: valid signature failed verification",
						testName)
				}
			case "invalid":
				if verified {
					t.Errorf("%s: invalid signature passed verification",
						testName)
				}
			case "acceptable":
				// Either result is permitted for acceptable vectors.
			default:
				t.Errorf("%s: unknown result %q", testName, test.result)
			}
		}
	}
}

func verifyECDSASignature(sig *Signature, digest []byte,
	pubKey *secp256k1.PublicKey) bool {
	return sig.Verify(digest, pubKey)
}

func verifyBitcoinECDSASignature(sig *Signature, digest []byte,
	pubKey *secp256k1.PublicKey) bool {
	if !sig.Verify(digest, pubKey) {
		return false
	}

	s := sig.S()
	return !s.IsOverHalfOrder()
}
