// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package secp256k1

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"
)

// hexToBigInt converts the passed hex string into a big integer and will panic
// if there is an error.  This is only provided for the hard-coded constants so
// errors in the source code can be detected. It will only (and must only) be
// called with hard-coded values.
func hexToBigInt(hexStr string) *big.Int {
	val, ok := new(big.Int).SetString(hexStr, 16)
	if !ok {
		panic("failed to parse big integer from hex: " + hexStr)
	}
	return val
}

// hexToBytes converts the passed hex string into bytes and will panic if there
// is an error.  This is only provided for the hard-coded constants so errors in
// the source code can be detected. It will only (and must only) be called with
// hard-coded values.
func hexToBytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	return b
}

// TestSignatureParsing ensures that signatures are properly parsed according
// to both BER and DER rules.  The error paths are tested as well.
func TestSignatureParsing(t *testing.T) {
	tests := []struct {
		name    string
		sig     []byte
		der     bool
		isValid bool
	}{{
		// signatures from bitcoin blockchain tx
		// 0437cd7f8525ceed2324359c2d0ba26006d92d85
		name: "valid signature",
		sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56c" +
			"bbac4622082221a8768d1d09"),
		der:     true,
		isValid: true,
	}, {
		name:    "empty",
		sig:     nil,
		isValid: false,
	}, {
		name: "bad magic",
		sig: hexToBytes("314402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56c" +
			"bbac4622082221a8768d1d09"),
		der:     true,
		isValid: false,
	}, {
		name: "bad 1st int marker magic",
		sig: hexToBytes("304403204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56c" +
			"bbac4622082221a8768d1d09"),
		der:     true,
		isValid: false,
	}, {
		name: "bad 2nd int marker",
		sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd410320181522ec8eca07de4860a4acdd12909d831cc56c" +
			"bbac4622082221a8768d1d09"),
		der:     true,
		isValid: false,
	}, {
		name: "short len",
		sig: hexToBytes("304302204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56c" +
			"bbac4622082221a8768d1d09"),
		der:     true,
		isValid: false,
	}, {
		name: "long len",
		sig: hexToBytes("304502204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56c" +
			"bbac4622082221a8768d1d09"),
		der:     true,
		isValid: false,
	}, {
		name: "long X",
		sig: hexToBytes("304402424e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56c" +
			"bbac4622082221a8768d1d09"),
		der:     true,
		isValid: false,
	}, {
		name: "long Y",
		sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd410221181522ec8eca07de4860a4acdd12909d831cc56c" +
			"bbac4622082221a8768d1d09"),
		der:     true,
		isValid: false,
	}, {
		name: "short Y",
		sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd410219181522ec8eca07de4860a4acdd12909d831cc56c" +
			"bbac4622082221a8768d1d09"),
		der:     true,
		isValid: false,
	}, {
		name: "trailing crap",
		sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56c" +
			"bbac4622082221a8768d1d0901"),
		der: true,

		// This test is now passing (used to be failing) because there are
		// signatures in the blockchain that have trailing zero bytes before
		// the hashtype. So ParseSignature was fixed to permit buffers with
		// trailing nonsense after the actual signature.
		isValid: true,
	}, {
		name: "X == N DER",
		sig: hexToBytes("30440220fffffffffffffffffffffffffffffffebaaedce6af48" +
			"a03bbfd25e8cd03641410220181522ec8eca07de4860a4acdd12909d831cc56c" +
			"bbac4622082221a8768d1d09"),
		der:     true,
		isValid: false,
	}, {
		name: "X == N BER",
		sig: hexToBytes("30440220fffffffffffffffffffffffffffffffebaaedce6af48" +
			"a03bbfd25e8cd03641420220181522ec8eca07de4860a4acdd12909d831cc56c" +
			"bbac4622082221a8768d1d09"),
		der:     false,
		isValid: false,
	}, {
		name: "Y == N",
		sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd410220fffffffffffffffffffffffffffffffebaaedce6" +
			"af48a03bbfd25e8cd0364141"),
		der:     true,
		isValid: false,
	}, {
		name: "Y > N",
		sig: hexToBytes("304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd410220fffffffffffffffffffffffffffffffebaaedce6" +
			"af48a03bbfd25e8cd0364142"),
		der:     false,
		isValid: false,
	}, {
		name: "0 len X",
		sig: hexToBytes("302402000220181522ec8eca07de4860a4acdd12909d831cc56c" +
			"bbac4622082221a8768d1d09"),
		der:     true,
		isValid: false,
	}, {
		name: "0 len Y",
		sig: hexToBytes("302402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd410200"),
		der:     true,
		isValid: false,
	}, {
		name: "extra R padding",
		sig: hexToBytes("30450221004e45e16932b8af514961a1d3a1a25fdf3f4f7732e9" +
			"d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc5" +
			"6cbbac4622082221a8768d1d09"),
		der:     true,
		isValid: false,
	}, {
		name: "extra S padding.",
		sig: hexToBytes("304502204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd41022100181522ec8eca07de4860a4acdd12909d831cc5" +
			"6cbbac4622082221a8768d1d09"),
		der:     true,
		isValid: false,
	}, {
		// Standard checks (in BER format, without checking for 'canonical' DER
		// signatures) don't test for negative numbers here because there isn't
		// a way that is the same between openssl and go that will mark a number
		// as negative. The Go ASN.1 parser marks numbers as negative when
		// openssl does not (it doesn't handle negative numbers that I can tell
		// at all. When not parsing DER signatures, which is done by by bitcoind
		// when accepting transactions into its mempool, we otherwise only check
		// for the coordinates being zero.
		name: "X == 0",
		sig: hexToBytes("30250201000220181522ec8eca07de4860a4acdd12909d831cc5" +
			"6cbbac4622082221a8768d1d09"),
		der:     false,
		isValid: false,
	}, {
		name: "Y == 0",
		sig: hexToBytes("302502204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d6" +
			"24c6c61548ab5fb8cd41020100"),
		der:     false,
		isValid: false,
	}}

	for _, test := range tests {
		var err error
		if test.der {
			_, err = ParseDERSignature(test.sig)
		} else {
			_, err = ParseSignature(test.sig)
		}
		if err != nil {
			if test.isValid {
				t.Errorf("%s signature failed when shouldn't %v", test.name,
					err)
			}
			continue
		}
		if !test.isValid {
			t.Errorf("%s counted as valid when it should fail", test.name)
		}
	}
}

// TestSignatureSerialize ensures that serializing signatures works as expected.
func TestSignatureSerialize(t *testing.T) {
	tests := []struct {
		name     string
		ecsig    *Signature
		expected []byte
	}{{
		// signature from bitcoin blockchain tx
		// 0437cd7f8525ceed2324359c2d0ba26006d92d85
		"valid 1 - r and s most significant bits are zero",
		&Signature{
			r: hexToBigInt("4e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd41"),
			s: hexToBigInt("181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d09"),
		},
		hexToBytes("304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d62" +
			"4c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc" +
			"56cbbac4622082221a8768d1d09"),
	}, {
		// signature from bitcoin blockchain tx
		// cb00f8a0573b18faa8c4f467b049f5d202bf1101d9ef2633bc611be70376a4b4
		"valid 2 - r most significant bit is one",
		&Signature{
			r: hexToBigInt("0082235e21a2300022738dabb8e1bbd9d19cfb1e7ab8c30a23b0afbb8d178abcf3"),
			s: hexToBigInt("24bf68e256c534ddfaf966bf908deb944305596f7bdcc38d69acad7f9c868724"),
		},
		hexToBytes("304502210082235e21a2300022738dabb8e1bbd9d19cfb1e7ab8c" +
			"30a23b0afbb8d178abcf3022024bf68e256c534ddfaf966bf908deb94430" +
			"5596f7bdcc38d69acad7f9c868724"),
	}, {
		// signature from bitcoin blockchain tx
		// fda204502a3345e08afd6af27377c052e77f1fefeaeb31bdd45f1e1237ca5470
		//
		// Note that signatures with an S component that is > half the group
		// order are neither allowed nor produced in Decred, so this has been
		// modified to expect the equally valid low S signature variant.
		"valid 3 - s most significant bit is one",
		&Signature{
			r: hexToBigInt("1cadddc2838598fee7dc35a12b340c6bde8b389f7bfd19a1252a17c4b5ed2d71"),
			s: hexToBigInt("c1a251bbecb14b058a8bd77f65de87e51c47e95904f4c0e9d52eddc21c1415ac"),
		},
		hexToBytes("304402201cadddc2838598fee7dc35a12b340c6bde8b389f7bfd1" +
			"9a1252a17c4b5ed2d7102203e5dae44134eb4fa757428809a2178199e66f" +
			"38daa53df51eaa380cab4222b95"),
	}, {
		"zero signature",
		&Signature{
			r: big.NewInt(0),
			s: big.NewInt(0),
		},
		hexToBytes("3006020100020100"),
	}}

	for i, test := range tests {
		result := test.ecsig.Serialize()
		if !bytes.Equal(result, test.expected) {
			t.Errorf("Serialize #%d (%s) unexpected result:\n"+
				"got:  %x\nwant: %x", i, test.name, result,
				test.expected)
		}
	}
}

// testSignCompact creates a recoverable public key signature over the provided
// data by creating a random private key, signing the data, and ensure the
// public key can be recovered.
func testSignCompact(t *testing.T, tag string, data []byte, isCompressed bool) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}
	signingPubKey := priv.PubKey()

	hashed := []byte("testing")
	sig := SignCompact(priv, hashed, isCompressed)

	pk, wasCompressed, err := RecoverCompact(sig, hashed)
	if err != nil {
		t.Errorf("%s: error recovering: %s", tag, err)
		return
	}
	if !pk.IsEqual(signingPubKey) {
		t.Errorf("%s: recovered pubkey doesn't match original "+
			"(%v,%v) vs (%v,%v) ", tag, pk.X, pk.Y, signingPubKey.X,
			signingPubKey.Y)
		return
	}
	if wasCompressed != isCompressed {
		t.Errorf("%s: recovered pubkey doesn't match compressed state "+
			"(%v vs %v)", tag, isCompressed, wasCompressed)
		return
	}

	// If we change the compressed bit we should get the same key back,
	// but the compressed flag should be reversed.
	if isCompressed {
		sig[0] -= 4
	} else {
		sig[0] += 4
	}

	pk, wasCompressed, err = RecoverCompact(sig, hashed)
	if err != nil {
		t.Errorf("%s: error recovering (2): %s", tag, err)
		return
	}
	if !pk.IsEqual(signingPubKey) {
		t.Errorf("%s: recovered pubkey (2) doesn't match original "+
			"(%v,%v) vs (%v,%v) ", tag, pk.X, pk.Y, signingPubKey.X,
			signingPubKey.Y)
		return
	}
	if wasCompressed == isCompressed {
		t.Errorf("%s: recovered pubkey doesn't match reversed "+
			"compressed state (%v vs %v)", tag, isCompressed,
			wasCompressed)
		return
	}
}

// TestSignCompact ensures the public key can be recovered from recoverable
// public key signatures over random data with random private keys.
func TestSignCompact(t *testing.T) {
	for i := 0; i < 256; i++ {
		name := fmt.Sprintf("test %d", i)
		data := make([]byte, 32)
		_, err := rand.Read(data)
		if err != nil {
			t.Errorf("failed to read random data for %s", name)
			continue
		}
		compressed := i%2 != 0
		testSignCompact(t, name, data, compressed)
	}
}

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
// NonceRFC6979 produces the expected nonces and resulting signatures for known
// values from other implementations.
func TestRFC6979Compat(t *testing.T) {
	// Test vectors matching Trezor and CoreBitcoin implementations.
	// - https://github.com/trezor/trezor-crypto/blob/9fea8f8ab377dc514e40c6fd1f7c89a74c1d8dc6/tests.c#L432-L453
	// - https://github.com/oleganza/CoreBitcoin/blob/e93dd71207861b5bf044415db5fa72405e7d8fbc/CoreBitcoin/BTCKey%2BTests.m#L23-L49
	tests := []struct {
		key       string
		msg       string
		nonce     string
		signature string
	}{{
		"cca9fbcc1b41e5a95d369eaa6ddcff73b61a4efaa279cfc6567e8daa39cbaf50",
		"sample",
		"2df40ca70e639d89528a6b670d9d48d9165fdc0febc0974056bdce192b8e16a3",
		"3045022100af340daf02cc15c8d5d08d7735dfe6b98a474ed373bdb5fbecf7571be52b384202205009fb27f37034a9b24b707b7c6b79ca23ddef9e25f7282e8a797efe53a8f124",
	}, {
		// This signature hits the case when S is higher than halforder.
		// If S is not canonicalized (lowered by halforder), this test will fail.
		"0000000000000000000000000000000000000000000000000000000000000001",
		"Satoshi Nakamoto",
		"8f8a276c19f4149656b280621e358cce24f5f52542772691ee69063b74f15d15",
		"3045022100934b1ea10a4b3c1757e2b0c017d0b6143ce3c9a7e6a4a49860d7a6ab210ee3d802202442ce9d2b916064108014783e923ec36b49743e2ffa1c4496f01a512aafd9e5",
	}, {
		"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140",
		"Satoshi Nakamoto",
		"33a19b60e25fb6f4435af53a3d42d493644827367e6453928554f43e49aa6f90",
		"3045022100fd567d121db66e382991534ada77a6bd3106f0a1098c231e47993447cd6af2d002206b39cd0eb1bc8603e159ef5c20a5c8ad685a45b06ce9bebed3f153d10d93bed5",
	}, {
		"f8b8af8ce3c7cca5e300d33939540c10d45ce001b8f252bfbc57ba0342904181",
		"Alan Turing",
		"525a82b70e67874398067543fd84c83d30c175fdc45fdeee082fe13b1d7cfdf1",
		"304402207063ae83e7f62bbb171798131b4a0564b956930092b33b07b395615d9ec7e15c022058dfcc1e00a35e1572f366ffe34ba0fc47db1e7189759b9fb233c5b05ab388ea",
	}, {
		"0000000000000000000000000000000000000000000000000000000000000001",
		"All those moments will be lost in time, like tears in rain. Time to die...",
		"38aa22d72376b4dbc472e06c3ba403ee0a394da63fc58d88686c611aba98d6b3",
		"30450221008600dbd41e348fe5c9465ab92d23e3db8b98b873beecd930736488696438cb6b0220547fe64427496db33bf66019dacbf0039c04199abb0122918601db38a72cfc21",
	}, {
		"e91671c46231f833a6406ccbea0e3e392c76c167bac1cb013f6f1013980455c2",
		"There is a computer disease that anybody who works with computers knows about. It's a very serious disease and it interferes completely with the work. The trouble with computers is that you 'play' with them!",
		"1f4b84c23a86a221d233f2521be018d9318639d5b8bbd6374a8a59232d16ad3d",
		"3045022100b552edd27580141f3b2a5463048cb7cd3e047b97c9f98076c32dbdf85a68718b0220279fa72dd19bfae05577e06c7c0c1900c371fcd5893f7e1d56a37d30174671f6",
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

		// Ensure deterministically generated signature is the expected value.
		gotSig := PrivKeyFromBytes(privKey).Sign(hash[:])
		gotSigBytes := gotSig.Serialize()
		wantSigBytes := hexToBytes(test.signature)
		if !bytes.Equal(gotSigBytes, wantSigBytes) {
			t.Errorf("Sign #%d (%s): mismatched signature: %x (expected %x)", i,
				test.msg, gotSigBytes, wantSigBytes)
			continue
		}
	}
}

// TestSignatureIsEqual ensures that equality testing between to signatures
// works as expected.
func TestSignatureIsEqual(t *testing.T) {
	sig1 := &Signature{
		r: hexToBigInt("0082235e21a2300022738dabb8e1bbd9d19cfb1e7ab8c30a23b0afbb8d178abcf3"),
		s: hexToBigInt("24bf68e256c534ddfaf966bf908deb944305596f7bdcc38d69acad7f9c868724"),
	}
	sig1Copy := &Signature{
		r: hexToBigInt("0082235e21a2300022738dabb8e1bbd9d19cfb1e7ab8c30a23b0afbb8d178abcf3"),
		s: hexToBigInt("24bf68e256c534ddfaf966bf908deb944305596f7bdcc38d69acad7f9c868724"),
	}
	sig2 := &Signature{
		r: hexToBigInt("4e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd41"),
		s: hexToBigInt("181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d09"),
	}

	if !sig1.IsEqual(sig1) {
		t.Fatalf("bad self signature equality check: %v == %v", sig1, sig1Copy)
	}
	if !sig1.IsEqual(sig1Copy) {
		t.Fatalf("bad signature equality check: %v == %v", sig1, sig1Copy)
	}

	if sig1.IsEqual(sig2) {
		t.Fatalf("bad signature equality check: %v != %v", sig1, sig2)
	}
}
