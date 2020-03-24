// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

// TestNonceRFC6979 ensures that the deterministic nonces generated by
// NonceRFC6979 produces the expected nonces, including things such as when
// providing extra data and version information, short hashes, and multiple
// iterations.
func TestNonceRFC6979(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		hash       string
		extraData  string
		version    string
		iterations uint32
		expected   string
	}{{
		name:       "key 32 bytes, hash 32 bytes, no extra data, no version",
		key:        "0011111111111111111111111111111111111111111111111111111111111111",
		hash:       "0000000000000000000000000000000000000000000000000000000000000001",
		iterations: 0,
		expected:   "154e92760f77ad9af6b547edd6f14ad0fae023eb2221bc8be2911675d8a686a3",
	}, {
		// Should be same as key with 32 bytes due to zero padding.
		name:       "key <32 bytes, hash 32 bytes, no extra data, no version",
		key:        "11111111111111111111111111111111111111111111111111111111111111",
		hash:       "0000000000000000000000000000000000000000000000000000000000000001",
		iterations: 0,
		expected:   "154e92760f77ad9af6b547edd6f14ad0fae023eb2221bc8be2911675d8a686a3",
	}, {
		// Should be same as key with 32 bytes due to truncation.
		name:       "key >32 bytes, hash 32 bytes, no extra data, no version",
		key:        "001111111111111111111111111111111111111111111111111111111111111111",
		hash:       "0000000000000000000000000000000000000000000000000000000000000001",
		iterations: 0,
		expected:   "154e92760f77ad9af6b547edd6f14ad0fae023eb2221bc8be2911675d8a686a3",
	}, {
		name:       "hash <32 bytes (padded), no extra data, no version",
		key:        "0011111111111111111111111111111111111111111111111111111111111111",
		hash:       "00000000000000000000000000000000000000000000000000000000000001",
		iterations: 0,
		expected:   "154e92760f77ad9af6b547edd6f14ad0fae023eb2221bc8be2911675d8a686a3",
	}, {
		name:       "hash >32 bytes (truncated), no extra data, no version",
		key:        "0011111111111111111111111111111111111111111111111111111111111111",
		hash:       "000000000000000000000000000000000000000000000000000000000000000100",
		iterations: 0,
		expected:   "154e92760f77ad9af6b547edd6f14ad0fae023eb2221bc8be2911675d8a686a3",
	}, {
		name:       "hash 32 bytes, extra data <32 bytes (ignored), no version",
		key:        "0011111111111111111111111111111111111111111111111111111111111111",
		hash:       "0000000000000000000000000000000000000000000000000000000000000001",
		extraData:  "00000000000000000000000000000000000000000000000000000000000002",
		iterations: 0,
		expected:   "154e92760f77ad9af6b547edd6f14ad0fae023eb2221bc8be2911675d8a686a3",
	}, {
		name:       "hash 32 bytes, extra data >32 bytes (ignored), no version",
		key:        "0011111111111111111111111111111111111111111111111111111111111111",
		hash:       "0000000000000000000000000000000000000000000000000000000000000001",
		extraData:  "000000000000000000000000000000000000000000000000000000000000000002",
		iterations: 0,
		expected:   "154e92760f77ad9af6b547edd6f14ad0fae023eb2221bc8be2911675d8a686a3",
	}, {
		name:       "hash 32 bytes, no extra data, version <16 bytes (ignored)",
		key:        "0011111111111111111111111111111111111111111111111111111111111111",
		hash:       "0000000000000000000000000000000000000000000000000000000000000001",
		version:    "000000000000000000000000000003",
		iterations: 0,
		expected:   "154e92760f77ad9af6b547edd6f14ad0fae023eb2221bc8be2911675d8a686a3",
	}, {
		name:       "hash 32 bytes, no extra data, version >16 bytes (ignored)",
		key:        "001111111111111111111111111111111111111111111111111111111111111122",
		hash:       "0000000000000000000000000000000000000000000000000000000000000001",
		version:    "0000000000000000000000000000000003",
		iterations: 0,
		expected:   "154e92760f77ad9af6b547edd6f14ad0fae023eb2221bc8be2911675d8a686a3",
	}, {
		name:       "hash 32 bytes, extra data 32 bytes, no version",
		key:        "0011111111111111111111111111111111111111111111111111111111111111",
		hash:       "0000000000000000000000000000000000000000000000000000000000000001",
		extraData:  "0000000000000000000000000000000000000000000000000000000000000002",
		iterations: 0,
		expected:   "67893461ade51cde61824b20bc293b585d058e6b9f40fb68453d5143f15116ae",
	}, {
		name:       "hash 32 bytes, no extra data, version 16 bytes",
		key:        "0011111111111111111111111111111111111111111111111111111111111111",
		hash:       "0000000000000000000000000000000000000000000000000000000000000001",
		version:    "00000000000000000000000000000003",
		iterations: 0,
		expected:   "7b27d6ceff87e1ded1860ca4e271a530e48514b9d3996db0af2bb8bda189007d",
	}, {
		// Should be same as no extra data + version specified due to padding.
		name:       "hash 32 bytes, extra data 32 bytes all zero, version 16 bytes",
		key:        "0011111111111111111111111111111111111111111111111111111111111111",
		hash:       "0000000000000000000000000000000000000000000000000000000000000001",
		extraData:  "0000000000000000000000000000000000000000000000000000000000000000",
		version:    "00000000000000000000000000000003",
		iterations: 0,
		expected:   "7b27d6ceff87e1ded1860ca4e271a530e48514b9d3996db0af2bb8bda189007d",
	}, {
		name:       "hash 32 bytes, extra data 32 bytes, version 16 bytes",
		key:        "0011111111111111111111111111111111111111111111111111111111111111",
		hash:       "0000000000000000000000000000000000000000000000000000000000000001",
		extraData:  "0000000000000000000000000000000000000000000000000000000000000002",
		version:    "00000000000000000000000000000003",
		iterations: 0,
		expected:   "9b5657643dfd4b77d99dfa505ed8a17e1b9616354fc890669b4aabece2170686",
	}, {
		name:       "hash 32 bytes, no extra data, no version, extra iteration",
		key:        "0011111111111111111111111111111111111111111111111111111111111111",
		hash:       "0000000000000000000000000000000000000000000000000000000000000001",
		iterations: 1,
		expected:   "66fca3fe494a6216e4a3f15cfbc1d969c60d9cdefda1a1c193edabd34aa8cd5e",
	}, {
		name:       "hash 32 bytes, no extra data, no version, 2 extra iterations",
		key:        "0011111111111111111111111111111111111111111111111111111111111111",
		hash:       "0000000000000000000000000000000000000000000000000000000000000001",
		iterations: 2,
		expected:   "70da248c92b5d28a52eafca1848b1a37d4cb36526c02553c9c48bb0b895fc77d",
	}}

	for _, test := range tests {
		privKey := hexToBytes(test.key)
		hash := hexToBytes(test.hash)
		extraData := hexToBytes(test.extraData)
		version := hexToBytes(test.version)
		wantNonce := hexToBytes(test.expected)

		// Ensure deterministically generated nonce is the expected value.
		gotNonce := NonceRFC6979(privKey, hash[:], extraData, version,
			test.iterations)
		gotNonceBytes := gotNonce.Bytes()
		if !bytes.Equal(gotNonceBytes[:], wantNonce) {
			t.Errorf("%s: unexpected nonce -- got %x, want %x", test.name,
				gotNonceBytes, wantNonce)
			continue
		}
	}
}

// TestRFC6979Compat ensures that the deterministic nonces generated by
// NonceRFC6979 produces the expected nonces for known values from other
// implementations.
func TestRFC6979Compat(t *testing.T) {
	// Test vectors matching Trezor and CoreBitcoin implementations.
	// - https://github.com/trezor/trezor-crypto/blob/9fea8f8ab377dc514e40c6fd1f7c89a74c1d8dc6/tests.c#L432-L453
	// - https://github.com/oleganza/CoreBitcoin/blob/e93dd71207861b5bf044415db5fa72405e7d8fbc/CoreBitcoin/BTCKey%2BTests.m#L23-L49
	tests := []struct {
		key   string
		msg   string
		nonce string
	}{{
		"cca9fbcc1b41e5a95d369eaa6ddcff73b61a4efaa279cfc6567e8daa39cbaf50",
		"sample",
		"2df40ca70e639d89528a6b670d9d48d9165fdc0febc0974056bdce192b8e16a3",
	}, {
		// This signature hits the case when S is higher than halforder.
		// If S is not canonicalized (lowered by halforder), this test will fail.
		"0000000000000000000000000000000000000000000000000000000000000001",
		"Satoshi Nakamoto",
		"8f8a276c19f4149656b280621e358cce24f5f52542772691ee69063b74f15d15",
	}, {
		"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140",
		"Satoshi Nakamoto",
		"33a19b60e25fb6f4435af53a3d42d493644827367e6453928554f43e49aa6f90",
	}, {
		"f8b8af8ce3c7cca5e300d33939540c10d45ce001b8f252bfbc57ba0342904181",
		"Alan Turing",
		"525a82b70e67874398067543fd84c83d30c175fdc45fdeee082fe13b1d7cfdf1",
	}, {
		"0000000000000000000000000000000000000000000000000000000000000001",
		"All those moments will be lost in time, like tears in rain. Time to die...",
		"38aa22d72376b4dbc472e06c3ba403ee0a394da63fc58d88686c611aba98d6b3",
	}, {
		"e91671c46231f833a6406ccbea0e3e392c76c167bac1cb013f6f1013980455c2",
		"There is a computer disease that anybody who works with computers knows about. It's a very serious disease and it interferes completely with the work. The trouble with computers is that you 'play' with them!",
		"1f4b84c23a86a221d233f2521be018d9318639d5b8bbd6374a8a59232d16ad3d",
	}}

	for i, test := range tests {
		privKey := hexToBytes(test.key)
		hash := sha256.Sum256([]byte(test.msg))

		// Ensure deterministically generated nonce is the expected value.
		gotNonce := NonceRFC6979(privKey, hash[:], nil, nil, 0)
		wantNonce := hexToBytes(test.nonce)
		gotNonceBytes := gotNonce.Bytes()
		if !bytes.Equal(gotNonceBytes[:], wantNonce) {
			t.Errorf("NonceRFC6979 #%d (%s): Nonce is incorrect: "+
				"%x (expected %x)", i, test.msg, gotNonce,
				wantNonce)
			continue
		}
	}
}
