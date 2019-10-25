// Copyright (c) 2013, 2014 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil

import (
	"testing"

	"github.com/decred/dcrd/chaincfg/v2/chainec"
	"github.com/decred/dcrd/dcrec/edwards/v2"
	"github.com/decred/dcrd/dcrec/secp256k1/v2"
	"github.com/decred/dcrd/dcrec/v2"
)

func TestEncodeDecodeWIF(t *testing.T) {
	mainNetPrivKeyID := [2]byte{0x22, 0xde} // starts with Pm
	testNetPrivKeyID := [2]byte{0x23, 0x0e} // starts with Pt
	simNetPrivKeyID := [2]byte{0x23, 0x07}  // starts with Ps
	regNetPrivKeyID := [2]byte{0x22, 0xfe}  // starts with Pr
	suites := []dcrec.SignatureType{
		dcrec.STEcdsaSecp256k1,
		dcrec.STEd25519,
		dcrec.STSchnorrSecp256k1,
	}
	for _, suite := range suites {
		var priv1, priv2 chainec.PrivateKey
		switch suite {
		case dcrec.STEcdsaSecp256k1:
			priv1, _ = secp256k1.PrivKeyFromBytes([]byte{
				0x0c, 0x28, 0xfc, 0xa3, 0x86, 0xc7, 0xa2, 0x27,
				0x60, 0x0b, 0x2f, 0xe5, 0x0b, 0x7c, 0xae, 0x11,
				0xec, 0x86, 0xd3, 0xbf, 0x1f, 0xbe, 0x47, 0x1b,
				0xe8, 0x98, 0x27, 0xe1, 0x9d, 0x72, 0xaa, 0x1d})

			priv2, _ = secp256k1.PrivKeyFromBytes([]byte{
				0xdd, 0xa3, 0x5a, 0x14, 0x88, 0xfb, 0x97, 0xb6,
				0xeb, 0x3f, 0xe6, 0xe9, 0xef, 0x2a, 0x25, 0x81,
				0x4e, 0x39, 0x6f, 0xb5, 0xdc, 0x29, 0x5f, 0xe9,
				0x94, 0xb9, 0x67, 0x89, 0xb2, 0x1a, 0x03, 0x98})
		case dcrec.STEd25519:
			priv1, _, _ = edwards.PrivKeyFromScalar(
				[]byte{
					0x0c, 0x28, 0xfc, 0xa3, 0x86, 0xc7, 0xa2, 0x27,
					0x60, 0x0b, 0x2f, 0xe5, 0x0b, 0x7c, 0xae, 0x11,
					0xec, 0x86, 0xd3, 0xbf, 0x1f, 0xbe, 0x47, 0x1b,
					0xe8, 0x98, 0x27, 0xe1, 0x9d, 0x72, 0xaa, 0x1d})

			priv2, _, _ = edwards.PrivKeyFromScalar(
				[]byte{
					0x0c, 0xa3, 0x5a, 0x14, 0x88, 0xfb, 0x97, 0xb6,
					0xeb, 0x3f, 0xe6, 0xe9, 0xef, 0x2a, 0x25, 0x81,
					0x4e, 0x39, 0x6f, 0xb5, 0xdc, 0x29, 0x5f, 0xe9,
					0x94, 0xb9, 0x67, 0x89, 0xb2, 0x1a, 0x03, 0x98})
		case dcrec.STSchnorrSecp256k1:
			priv1, _ = secp256k1.PrivKeyFromBytes([]byte{
				0x0c, 0x28, 0xfc, 0xa3, 0x86, 0xc7, 0xa2, 0x27,
				0x60, 0x0b, 0x2f, 0xe5, 0x0b, 0x7c, 0xae, 0x11,
				0xec, 0x86, 0xd3, 0xbf, 0x1f, 0xbe, 0x47, 0x1b,
				0xe8, 0x98, 0x27, 0xe1, 0x9d, 0x72, 0xaa, 0x1d})

			priv2, _ = secp256k1.PrivKeyFromBytes([]byte{
				0xdd, 0xa3, 0x5a, 0x14, 0x88, 0xfb, 0x97, 0xb6,
				0xeb, 0x3f, 0xe6, 0xe9, 0xef, 0x2a, 0x25, 0x81,
				0x4e, 0x39, 0x6f, 0xb5, 0xdc, 0x29, 0x5f, 0xe9,
				0x94, 0xb9, 0x67, 0x89, 0xb2, 0x1a, 0x03, 0x98})
		}

		wif1 := NewWIF(priv1, mainNetPrivKeyID, suite)
		wif2 := NewWIF(priv2, testNetPrivKeyID, suite)
		wif3 := NewWIF(priv2, simNetPrivKeyID, suite)
		wif4 := NewWIF(priv2, regNetPrivKeyID, suite)

		var tests []struct {
			wif     *WIF
			encoded string
			net     [2]byte
		}

		switch suite {
		case dcrec.STEcdsaSecp256k1:
			tests = []struct {
				wif     *WIF
				encoded string
				net     [2]byte
			}{
				{
					wif1,
					"PmQdMn8xafwaQouk8ngs1CccRCB1ZmsqQxBaxNR4vhQi5a5QB5716",
					mainNetPrivKeyID,
				},
				{
					wif2,
					"PtWVDUidYaiiNT5e2Sfb1Ah4evbaSopZJkkpFBuzkJYcYteugvdFg",
					testNetPrivKeyID,
				},
				{
					wif3,
					"PsURoUb7FMeJQdTYea8pkbUQFBZAsxtfDcfTLGja5sCLZvLZWRtjK",
					simNetPrivKeyID,
				},
				{
					wif4,
					"Pr9D8L8s9nG4AroRjbTGiRuYrweN1T8Dg9grAEeTEStZAPMnjxwCT",
					regNetPrivKeyID,
				},
			}
		case dcrec.STEd25519:
			tests = []struct {
				wif     *WIF
				encoded string
				net     [2]byte
			}{
				{
					wif1,
					"PmQfJXKC2ho1633ZiVbSdCZw1y68BVXYFpyE2UfDcbQN5xa3DByDn",
					mainNetPrivKeyID,
				},
				{
					wif2,
					"PtWVaBGeCfbFQfgqFew8YvdrSH5TH439K7rvpo3aWnSfDvyK8ijbK",
					testNetPrivKeyID,
				},
				{
					wif3,
					"PsUSAB97uSWqSr4jsnQNJMRC2Y33iD7FDymZuss9rM6PExexSPyTQ",
					simNetPrivKeyID,
				},
				{
					wif4,
					"Pr9DV2gsos8bD5QcxoipGBrLeJ8EqhLogWnxjqn2zvnbqRg9fZMHs",
					regNetPrivKeyID,
				},
			}
		case dcrec.STSchnorrSecp256k1:
			tests = []struct {
				wif     *WIF
				encoded string
				net     [2]byte
			}{
				{
					wif1,
					"PmQhFGVRUjeRmGBPJCW2FCXFck1EoDBF6hks6auNJVQ26M4h73W9W",
					mainNetPrivKeyID,
				},
				{
					wif2,
					"PtWZ6y56SeRZiuMHBrUkFAbhrURogF7xzWL6PQQJ86XvZfeE3jf1a",
					testNetPrivKeyID,
				},
				{
					wif3,
					"PsUVgxwa9RM9m5jBoywyzbP3SjPQ7QC4uNEjUVDsTfBeahKkmETvQ",
					simNetPrivKeyID,
				},
				{
					wif4,
					"Pr9H1pVL3qxuXK54u1GRxRpC4VUbEtRdMuG8JT8kcEssBALyTRM7v",
					regNetPrivKeyID,
				},
			}
		}

		for _, test := range tests {
			// Test that encoding the WIF structure matches the expected string.
			s := test.wif.String()
			if s != test.encoded {
				t.Errorf("TestEncodeDecodePrivateKey failed: want '%s', got '%s'",
					test.encoded, s)
				continue
			}

			// Test that decoding the expected string results in the original WIF
			// structure.
			w, err := DecodeWIF(test.encoded, test.net)
			if err != nil {
				t.Error(err)
				continue
			}
			if got := w.String(); got != test.encoded {
				t.Errorf("NewWIF failed: want '%v', got '%v'", test.wif, got)
			}

			w.SerializePubKey()
		}
	}
}
