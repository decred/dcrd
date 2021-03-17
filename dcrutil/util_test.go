// Copyright (c) 2020-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil

import (
	"testing"
)

// mockAddrParams implements the AddressParams interface and is used throughout
// the tests to mock multiple networks.
type mockAddrParams struct {
	pubKeyID     [2]byte
	pkhEcdsaID   [2]byte
	pkhEd25519ID [2]byte
	pkhSchnorrID [2]byte
	scriptHashID [2]byte
	privKeyID    [2]byte
}

// AddrIDPubKeyV0 returns the magic prefix bytes associated with the mock params
// for version 0 pay-to-pubkey addresses.
//
// This is part of the AddressParams interface.
func (p *mockAddrParams) AddrIDPubKeyV0() [2]byte {
	return p.pubKeyID
}

// AddrIDPubKeyHashECDSAV0 returns the magic prefix bytes associated with the
// mock params for version 0 pay-to-pubkey-hash addresses where the underlying
// pubkey is secp256k1 and the signature algorithm is ECDSA.
//
// This is part of the AddressParams interface.
func (p *mockAddrParams) AddrIDPubKeyHashECDSAV0() [2]byte {
	return p.pkhEcdsaID
}

// AddrIDPubKeyHashEd25519V0 returns the magic prefix bytes associated with the
// mock params for version 0 pay-to-pubkey-hash addresses where the underlying
// pubkey and signature algorithm are Ed25519.
//
// This is part of the AddressParams interface.
func (p *mockAddrParams) AddrIDPubKeyHashEd25519V0() [2]byte {
	return p.pkhEd25519ID
}

// AddrIDPubKeyHashSchnorrV0 returns the magic prefix bytes associated with the
// mock params for version 0 pay-to-pubkey-hash addresses where the underlying
// pubkey is secp256k1 and the signature algorithm is Schnorr.
//
// This is part of the AddressParams interface.
func (p *mockAddrParams) AddrIDPubKeyHashSchnorrV0() [2]byte {
	return p.pkhSchnorrID
}

// AddrIDScriptHashV0 returns the magic prefix bytes associated with the mock
// params for version 0 pay-to-script-hash addresses.
//
// This is part of the AddressParams interface.
func (p *mockAddrParams) AddrIDScriptHashV0() [2]byte {
	return p.scriptHashID
}

// mockMainNetParams returns mock mainnet address parameters to use throughout
// the tests.  They match the Decred mainnet params as of the time this comment
// was written.
func mockMainNetParams() *mockAddrParams {
	return &mockAddrParams{
		pubKeyID:     [2]byte{0x13, 0x86}, // starts with Dk
		pkhEcdsaID:   [2]byte{0x07, 0x3f}, // starts with Ds
		pkhEd25519ID: [2]byte{0x07, 0x1f}, // starts with De
		pkhSchnorrID: [2]byte{0x07, 0x01}, // starts with DS
		scriptHashID: [2]byte{0x07, 0x1a}, // starts with Dc
		privKeyID:    [2]byte{0x22, 0xde}, // starts with Pm
	}
}

// mockTestNetParams returns mock testnet address parameters to use throughout
// the tests.  They match the Decred mainnet params as of the time this comment
// was written.
func mockTestNetParams() *mockAddrParams {
	return &mockAddrParams{
		pubKeyID:     [2]byte{0x28, 0xf7}, // starts with Tk
		pkhEcdsaID:   [2]byte{0x0f, 0x21}, // starts with Ts
		pkhEd25519ID: [2]byte{0x0f, 0x01}, // starts with Te
		pkhSchnorrID: [2]byte{0x0e, 0xe3}, // starts with TS
		scriptHashID: [2]byte{0x0e, 0xfc}, // starts with Tc
		privKeyID:    [2]byte{0x23, 0x0e}, // starts with Pt
	}
}

// TestVerifyMessage ensures the verifying a message works as intended.
func TestVerifyMessage(t *testing.T) {
	mainNetParams := mockMainNetParams()
	testNetParams := mockTestNetParams()

	msg := "verifymessage test"

	var tests = []struct {
		name    string
		addr    string
		sig     string
		params  AddressParams
		isValid bool
	}{{
		name:    "valid",
		addr:    "TsdbYVDoh3JsyP6oEg2aHVoTjsFuHzUgGKv",
		sig:     "IITPXfmkfLPULbX9Im3XIyHAKiXRw5N9j6P7qf0MdEP9YQinn51lWjS+8jbTceRxCWckKKssu3ZpQm1xCWKz9GA=",
		params:  testNetParams,
		isValid: true,
	}, {
		name:    "wrong address",
		addr:    "TsWeG3TJzucZgYyMfZFC2GhBvbeNfA48LTo",
		sig:     "IITPXfmkfLPULbX9Im3XIyHAKiXRw5N9j6P7qf0MdEP9YQinn51lWjS+8jbTceRxCWckKKssu3ZpQm1xCWKz9GA=",
		params:  testNetParams,
		isValid: false,
	}, {
		name:    "wrong signature",
		addr:    "TsdbYVDoh3JsyP6oEg2aHVoTjsFuHzUgGKv",
		sig:     "HxzZggzHMljSWpKHnw1Dow84KGWvTRBCG2JqBM5W4Q7iePW0dirZXCggSeXHVQ26D0MbDFffi3yw+x2Z5nQ94gg=",
		params:  testNetParams,
		isValid: false,
	}, {
		name:    "wrong params",
		addr:    "TsdbYVDoh3JsyP6oEg2aHVoTjsFuHzUgGKv",
		sig:     "IITPXfmkfLPULbX9Im3XIyHAKiXRw5N9j6P7qf0MdEP9YQinn51lWjS+8jbTceRxCWckKKssu3ZpQm1xCWKz9GA=",
		params:  mainNetParams,
		isValid: false,
	}}

	for _, test := range tests {
		err := VerifyMessage(test.addr, test.sig, msg, test.params)
		if (test.isValid && err != nil) || (!test.isValid && err == nil) {
			t.Fatalf("%s: failed", test.name)
		}
	}
}
