// Copyright (c) 2013, 2014 The btcsuite developers
// Copyright (c) 2015-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/decred/dcrd/dcrec"
)

// TestWIF ensures that WIF-related APIs work as intended including that they
// are properly encoded and decoded, that they produce the expected private and
// public keys as well as DSA, and that error paths fail as expected.
func TestWIF(t *testing.T) {
	t.Parallel()

	mainNetPrivKeyID := [2]byte{0x22, 0xde} // starts with Pm
	testNetPrivKeyID := [2]byte{0x23, 0x0e} // starts with Pt
	simNetPrivKeyID := [2]byte{0x23, 0x07}  // starts with Ps
	regNetPrivKeyID := [2]byte{0x22, 0xfe}  // starts with Pr
	priv1 := "0c28fca386c7a227600b2fe50b7cae11ec86d3bf1fbe471be89827e19d72aa1d"
	priv2 := "dda35a1488fb97b6eb3fe6e9ef2a25814e396fb5dc295fe994b96789b21a0398"
	priv3 := "0ca35a1488fb97b6eb3fe6e9ef2a25814e396fb5dc295fe994b96789b21a0398"
	pub1 := "02d0de0aaeaefad02b8bdc8a01a1b8b11c696bd3d66a2c5f10780d95b7df42645c"
	pub2 := "02eec2540661b0c39d271570742413bd02932dd0093493fd0beced0b7f93addec4"
	pub3 := "544e3399a8df7b99a0454bfd9691050c8bba58f8d685e98cb72277032a0cf3ac"

	// isErr is a convenience func which acts as a limited version of errors.Is
	// until the package is converted to support it at which point this can be
	// removed.
	isErr := func(err error, target error) bool {
		if (err == nil) != (target == nil) {
			return false
		}
		return err == target || (err != nil && err.Error() == target.Error())
	}

	tests := []struct {
		name      string              // test description
		privKey   string              // hex-encoded private key
		net       [2]byte             // network for WIF
		dsa       dcrec.SignatureType // DSA suite
		skipMake  bool                // set to skip new WIF via API
		makeErr   error               // expected error from new WIF func
		wif       string              // WIF to decode and expected WIF
		decodeErr error               // expected error from decode func
		pubKey    string              // expected hex-encoded public key
	}{{
		// ---------------------------------------------------------------------
		// Misc construction error tests.
		// ---------------------------------------------------------------------

		name:    "unsupported signature type",
		privKey: priv1,
		net:     mainNetPrivKeyID,
		dsa:     99999,
		makeErr: fmt.Errorf("unsupported signature type '99999'"),
	}, {
		name:    "invalid ed25519 privkey via constructor",
		privKey: priv2,
		net:     mainNetPrivKeyID,
		dsa:     dcrec.STEd25519,
		makeErr: fmt.Errorf("not on subgroup (>N)"),
	}, {
		// ---------------------------------------------------------------------
		// Misc decoding error tests.
		// ---------------------------------------------------------------------

		name:      "bad checksum",
		skipMake:  true,
		net:       mainNetPrivKeyID,
		wif:       "PmQdMn8xafwaQouk8ngs1CccRCB1ZmsqQxBaxNR4vhQi5a5QB5717",
		decodeErr: ErrChecksumMismatch,
	}, {
		name:      "bad decoded data length",
		skipMake:  true,
		net:       mainNetPrivKeyID,
		wif:       "6A9ymjvs2UDRijKt1s2qfC5i9cvQiZmBDbzmDukTfT3jZS4mStxo",
		decodeErr: ErrMalformedPrivateKey,
	}, {
		name:      "wif for wrong network",
		skipMake:  true,
		net:       mainNetPrivKeyID,
		wif:       "PtWVDUidYaiiNT5e2Sfb1Ah4evbaSopZJkkpFBuzkJYcYteugvdFg",
		decodeErr: ErrWrongWIFNetwork(mainNetPrivKeyID),
	}, {
		name:      "invalid ed25519 privkey via decode",
		skipMake:  true,
		net:       mainNetPrivKeyID,
		wif:       "PmQgtn9ZpDCASuNpjbwDkxHqskf3kTPFFpLZ4SMWtda6PmFyvM5aG",
		decodeErr: fmt.Errorf("not on subgroup (>N)"),
	}, {
		// ---------------------------------------------------------------------
		// Positive path tests.
		// ---------------------------------------------------------------------

		name:    "mainnet ecdsa secp256k1 priv1",
		privKey: priv1,
		net:     mainNetPrivKeyID,
		dsa:     dcrec.STEcdsaSecp256k1,
		wif:     "PmQdMn8xafwaQouk8ngs1CccRCB1ZmsqQxBaxNR4vhQi5a5QB5716",
		pubKey:  pub1,
	}, {
		name:    "testnet ecdsa secp256k1 priv2",
		privKey: priv2,
		net:     testNetPrivKeyID,
		dsa:     dcrec.STEcdsaSecp256k1,
		wif:     "PtWVDUidYaiiNT5e2Sfb1Ah4evbaSopZJkkpFBuzkJYcYteugvdFg",
		pubKey:  pub2,
	}, {
		name:    "simnet ecdsa secp256k1 priv2",
		privKey: priv2,
		net:     simNetPrivKeyID,
		dsa:     dcrec.STEcdsaSecp256k1,
		wif:     "PsURoUb7FMeJQdTYea8pkbUQFBZAsxtfDcfTLGja5sCLZvLZWRtjK",
		pubKey:  pub2,
	}, {
		name:    "regnet ecdsa secp256k1 priv2",
		privKey: priv2,
		net:     regNetPrivKeyID,
		dsa:     dcrec.STEcdsaSecp256k1,
		wif:     "Pr9D8L8s9nG4AroRjbTGiRuYrweN1T8Dg9grAEeTEStZAPMnjxwCT",
		pubKey:  pub2,
	}, {
		name:    "mainnet ed25519 priv1",
		privKey: priv1,
		net:     mainNetPrivKeyID,
		dsa:     dcrec.STEd25519,
		wif:     "PmQfJXKC2ho1633ZiVbSdCZw1y68BVXYFpyE2UfDcbQN5xa3DByDn",
		pubKey:  "667c1c28ec2d59ceb33673a94165a5e51d5cbcef620aec663cd83638643bba3b",
	}, {
		name:    "testnet ed25519 priv3",
		privKey: priv3,
		net:     testNetPrivKeyID,
		dsa:     dcrec.STEd25519,
		wif:     "PtWVaBGeCfbFQfgqFew8YvdrSH5TH439K7rvpo3aWnSfDvyK8ijbK",
		pubKey:  pub3,
	}, {
		name:    "simnet ed25519 priv3",
		privKey: priv3,
		net:     simNetPrivKeyID,
		dsa:     dcrec.STEd25519,
		wif:     "PsUSAB97uSWqSr4jsnQNJMRC2Y33iD7FDymZuss9rM6PExexSPyTQ",
		pubKey:  pub3,
	}, {
		name:    "regnet ed25519 priv3",
		privKey: priv3,
		net:     regNetPrivKeyID,
		dsa:     dcrec.STEd25519,
		wif:     "Pr9DV2gsos8bD5QcxoipGBrLeJ8EqhLogWnxjqn2zvnbqRg9fZMHs",
		pubKey:  pub3,
	}, {
		name:    "mainnet schnorr secp256k1 priv2",
		privKey: priv1,
		net:     mainNetPrivKeyID,
		dsa:     dcrec.STSchnorrSecp256k1,
		wif:     "PmQhFGVRUjeRmGBPJCW2FCXFck1EoDBF6hks6auNJVQ26M4h73W9W",
		pubKey:  pub1,
	}, {
		name:    "testnet schnorr secp256k1 priv2",
		privKey: priv2,
		net:     testNetPrivKeyID,
		dsa:     dcrec.STSchnorrSecp256k1,
		wif:     "PtWZ6y56SeRZiuMHBrUkFAbhrURogF7xzWL6PQQJ86XvZfeE3jf1a",
		pubKey:  pub2,
	}, {
		name:    "simnet schnorr secp256k1 priv2",
		privKey: priv2,
		net:     simNetPrivKeyID,
		dsa:     dcrec.STSchnorrSecp256k1,
		wif:     "PsUVgxwa9RM9m5jBoywyzbP3SjPQ7QC4uNEjUVDsTfBeahKkmETvQ",
		pubKey:  pub2,
	}, {
		name:    "regnet schnorr secp256k1 priv2",
		privKey: priv2,
		net:     regNetPrivKeyID,
		dsa:     dcrec.STSchnorrSecp256k1,
		wif:     "Pr9H1pVL3qxuXK54u1GRxRpC4VUbEtRdMuG8JT8kcEssBALyTRM7v",
		pubKey:  pub2,
	}}

	for _, test := range tests {
		// Decode test data.
		privKey, err := hex.DecodeString(test.privKey)
		if err != nil {
			t.Errorf("%q: invalid private key hex error: %v", test.name, err)
			continue
		}
		pubKey, err := hex.DecodeString(test.pubKey)
		if err != nil {
			t.Errorf("%q: invalid public key hex error: %v", test.name, err)
			continue
		}

		// checkWIF ensures the various methods of the provided WIF return the
		// expected results.
		checkWIF := func(wif *WIF) bool {
			t.Helper()

			// Ensure the WIF encodes to the expected string.
			gotEncoded := wif.String()
			if gotEncoded != test.wif {
				t.Errorf("%q: mismatched encoding -- got %s, want %s",
					test.name, gotEncoded, test.wif)
				return false
			}

			// Ensure the WIF returns the expected private key.
			gotPriv := wif.PrivKey()
			if !bytes.Equal(gotPriv, privKey) {
				t.Errorf("%q: mismatched private key -- got %x, want %x",
					test.name, gotPriv, privKey)
				return false
			}

			// Ensure the WIF returns the expected public key.
			gotPub := wif.PubKey()
			if !bytes.Equal(gotPub, pubKey) {
				t.Errorf("%q: mismatched public key -- got %x, want %x",
					test.name, gotPub, pubKey)
				return false
			}

			// Ensure the WIF returns the expected DSA.
			gotDSA := wif.DSA()
			if gotDSA != test.dsa {
				t.Errorf("%q: mismatched DSA -- got %v, want %v", test.name,
					gotDSA, test.dsa)
				return false
			}

			return true
		}

		// Create the WIF from the test data and ensure it returns all of the
		// expected data.
		if !test.skipMake {
			wif, err := NewWIF(privKey, test.net, test.dsa)
			if !isErr(err, test.makeErr) {
				t.Errorf("%q: unexpected error -- got %v, want %v", test.name,
					err, test.makeErr)
				continue
			}
			if err != nil {
				continue
			}
			if !checkWIF(wif) {
				continue
			}
		}

		// Ensure the WIF decodes as expected and that it returns all of the
		// expected data.
		wif2, err := DecodeWIF(test.wif, test.net)
		if !isErr(err, test.decodeErr) {
			t.Errorf("%q: unexpected error -- got %v, want %v", test.name, err,
				test.decodeErr)
			continue
		}
		if err != nil {
			continue
		}
		if !checkWIF(wif2) {
			continue
		}
	}
}
