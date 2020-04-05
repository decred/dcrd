// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package hdkeychain

// References:
//   [BIP32]: BIP0032 - Hierarchical Deterministic Wallets
//   https://github.com/bitcoin/bips/blob/master/bip-0032.mediawiki

import (
	"bytes"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v3"
)

// mockNetParams implements the NetworkParams interface and is used throughout
// the tests to mock multiple networks.
type mockNetParams struct {
	privKeyID [4]byte
	pubKeyID  [4]byte
}

// HDPrivKeyVersion returns the extended private key version bytes associated
// with the mock params.
//
// This is part of the NetworkParams interface.
func (p *mockNetParams) HDPrivKeyVersion() [4]byte {
	return p.privKeyID
}

// HDPubKeyVersion returns the extended public key version bytes associated with
// the mock params.
//
// This is part of the NetworkParams interface.
func (p *mockNetParams) HDPubKeyVersion() [4]byte {
	return p.pubKeyID
}

// mockMainNetParams returns mock mainnet parameters to use throughout the
// tests.  They match the Decred mainnet params as of the time this comment was
// written.
func mockMainNetParams() *mockNetParams {
	return &mockNetParams{
		privKeyID: [4]byte{0x02, 0xfd, 0xa4, 0xe8}, // starts with dprv
		pubKeyID:  [4]byte{0x02, 0xfd, 0xa9, 0x26}, // starts with dpub
	}
}

// mockTestNetParams returns mock testnet parameters to use throughout the
// tests.  They match the Decred testnet version 3 params as of the time this
// comment was written.
func mockTestNetParams() *mockNetParams {
	return &mockNetParams{
		privKeyID: [4]byte{0x04, 0x35, 0x83, 0x97}, // starts with tprv
		pubKeyID:  [4]byte{0x04, 0x35, 0x87, 0xd1}, // starts with tpub
	}
}

// TestBIP0032Vectors tests the vectors provided by [BIP32] to ensure the
// derivation works as intended.
func TestBIP0032Vectors(t *testing.T) {
	// The master seeds for each of the two test vectors in [BIP32].
	//
	// Note that the 3rd seed has been changed to ensure the condition it is
	// intended to test applies with the modified hash function used in Decred.
	//
	// In particular, it results in hardened child 0 of the master key having
	// a child key with leading zeroes.
	testVec1MasterHex := "000102030405060708090a0b0c0d0e0f"
	testVec2MasterHex := "fffcf9f6f3f0edeae7e4e1dedbd8d5d2cfccc9c6c3c0bdbab7b4b1aeaba8a5a29f9c999693908d8a8784817e7b7875726f6c696663605d5a5754514e4b484542"
	testVec3MasterHex := "4b381541583be4423346c643850da4b320e46a87ae3d2a4e6da11eba819cd4acba45d239319ac14f863b8d5ab5a0d0c64d2e8a1e7d1457df2e5a3c5100005930"
	hkStart := uint32(0x80000000)

	mainNetParams := mockMainNetParams()
	testNetParams := mockTestNetParams()
	tests := []struct {
		name     string
		master   string
		path     []uint32
		wantPub  string
		wantPriv string
		net      NetworkParams
	}{
		// Test vector 1
		{
			name:     "test vector 1 chain m",
			master:   testVec1MasterHex,
			path:     []uint32{},
			wantPub:  "dpubZ9169KDAEUnyoBhjjmT2VaEodr6pUTDoqCEAeqgbfr2JfkB88BbK77jbTYbcYXb2FVz7DKBdW4P618yd51MwF8DjKVopSbS7Lkgi6bowX5w",
			wantPriv: "dprv3hCznBesA6jBtmoyVFPfyMSZ1qYZ3WdjdebquvkEfmRfxC9VFEFi2YDaJqHnx7uGe75eGSa3Mn3oHK11hBW7KZUrPxwbCPBmuCi1nwm182s",
			net:      mainNetParams,
		},
		{
			name:     "test vector 1 chain m/0H",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart},
			wantPub:  "dpubZCGVaKZBiMo7pMgLaZm1qmchjWenTeVcUdFQkTNsFGFEA6xs4EW8PKiqYqP7HBAitt9Hw16VQkQ1tjsZQSHNWFc6bEK6bLqrbco24FzBTY4",
			wantPriv: "dprv3kUQDBztdyjKuwnaL3hfKYpT7W6X2huYH5d61YSWFBebSYwEBHAXJkCpQ7rvMAxPzKqxVCGLvBqWvGxXjAyMJsV1XwKkfnQCM9KctC8k8bk",
			net:      mainNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1},
			wantPub:  "dpubZEDyZgdnFBMHxqNhfCUwBfAg1UmXHiTmB5jKtzbAZhF8PTzy2PwAicNdkg1CmW6TARxQeUbgC7nAQenJts4YoG3KMiqcjsjgeMvwLc43w6C",
			wantPriv: "dprv3nRtCZ5VAoHW4RUwQgRafSNRPUDFrmsgyY71A5eoZceVfuyL9SbZe2rcbwDW2UwpkEniE4urffgbypegscNchPajWzy9QS4cRxF8QYXsZtq",
			net:      mainNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2},
			wantPub:  "dpubZGLz7gsJAWzUksvtw3opxx5eeLq5fRaUMDABA3bdUVfnGUk5fiS5Cc3kZGTjWtYr3jrEavQQnAF6jv2WCpZtFX4uFgifXqev6ED1TM9rTCB",
			wantPriv: "dprv3pYtkZK168vgrU38gXkUSjHQ2LGpEUzQ9fXrR8fGUR59YviSnm6U82XjQYhpJEUPnVcC9bguJBQU5xVM4VFcDHu9BgScGPA6mQMH4bn5Cth",
			net:      mainNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H/2",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2, 2},
			wantPub:  "dpubZHv6Cfp2XRSWHQXZBo1dLmVM421Zdkc4MePkyBXCLFttVkCmwZkxth4ZV9PzkFP3DtD5xcVq2CPSYpJMWMaoxu1ixz4GNZFVcE2xnHP6chJ",
			wantPriv: "dprv3r7zqYFjT3NiNzdnwGxGpYh6S1TJCp1zA6mSEGaqLBJFnCB94cRMp7YYLR49aTZHZ7ya1CXwQJ6rodKeU9NgQTxkPSK7pzgZRgjYkQ7rgJh",
			net:      mainNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H/2/1000000000",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2, 2, 1000000000},
			wantPub:  "dpubZL6d9amjfRy1zeoZM2zHDU7uoMvwPqtxHRQAiJjeEtQQWjP3retQV1qKJyzUd6ZJNgbJGXjtc5pdoBcTTYTLoxQzvV9JJCzCjB2eCWpRf8T",
			wantPriv: "dprv3tJXnTDSb3uE6Euo6WvvhFKfBMNfxuJt5smqyPoHEoomoBMQyhYoQSKJAHWtWxmuqdUVb8q9J2NaTkF6rYm6XDrSotkJ55bM21fffa7VV97",
			net:      mainNetParams,
		},

		// Test vector 2
		{
			name:     "test vector 2 chain m",
			master:   testVec2MasterHex,
			path:     []uint32{},
			wantPub:  "dpubZ9169KDAEUnynoD4qvXJwmxZt3FFA5UdWn1twnRReE9AxjCKJLNFY1uBoegbFmwzA4Du7yqnu8tLivhrCCH6P3DgBS1HH5vmf8MpNXvvYT9",
			wantPriv: "dprv3hCznBesA6jBtPKJbQTxRZAKG2gyj8tZKEPaCsV4e9YYFBAgRP2eTSPAeu4r8dTMt9q51j2Vdt5zNqj7jbtovvocrP1qLj6WUTLF9xYQt4y",
			net:      mainNetParams,
		},
		{
			name:     "test vector 2 chain m/0",
			master:   testVec2MasterHex,
			path:     []uint32{0},
			wantPub:  "dpubZBA4RCkCybJFaNbqPuBiyfXY1rvmG1XTdCy1AY1U96dxkFqWc2i5KREMh7NYPpy7ZPMhdpFMAesex3JdFDfX4J5FEW3HjSacqEYPfwb9Cj7",
			wantPriv: "dprv3jMy45BuuDETfxi59P8NTSjHPrNVq4wPRfLgRd57923L2hosj5NUEqiLYQ4i7fJtUpiXZLr2wUeToJY2Tm5sCpAJdajEHDmieVJiPQNXwu9",
			net:      mainNetParams,
		},
		{
			name:     "test vector 2 chain m/0/2147483647H",
			master:   testVec2MasterHex,
			path:     []uint32{0, hkStart + 2147483647},
			wantPub:  "dpubZDUNkZEcCRCZEizDGL9sAQbZRKSnaxQLeqN9zpueeqCyq2VY7NUGMXASacsK96S8XzNjq3YgFgwLtj8MJBToW6To9U5zxuazEyh89bjR1xA",
			wantPriv: "dprv3mgHPRgK838mLK6T1p6WeBoJoJtXA1pGTHjqFuyHekcM7UTuER8fGweRRsoLqSuHa98uskVPnJnfWZEBUC1AVmXnSCPDvUFKydXNnnPHTuQ",
			net:      mainNetParams,
		},
		{
			name:     "test vector 2 chain m/0/2147483647H/1",
			master:   testVec2MasterHex,
			path:     []uint32{0, hkStart + 2147483647, 1},
			wantPub:  "dpubZF3wJh7SfggGg74QZW3EE9ei8uQSJEFgd62uyuK5iMgQzUNjpSnprgTpYz3d6Q3fXXtEEXQqpzWcP4LUVuXFsgA8JKt1Hot5kyUk4pPRhDz",
			wantPriv: "dprv3oFqwZZ9bJcUmhAeJyyshvrTWtrAsHfcRYQbEzNiiH5nGvM6wVTDn6woQEz92b2EHTYZBtLi82jKEnxSouA3cVaW8YWBsw5c3f4mwAhA3d2",
			net:      mainNetParams,
		},
		{
			name:     "test vector 2 chain m/0/2147483647H/1/2147483646H",
			master:   testVec2MasterHex,
			path:     []uint32{0, hkStart + 2147483647, 1, hkStart + 2147483646},
			wantPub:  "dpubZH38NEg1CW19dGZs8NdaT4hDkz7wXPstio1mGpHSAXHpSGW3UnTrn25ERT1Mp8ae5GMoQHMbgQiPrChMXQMdx3UqS8YqFkT1pqait8fY92u",
			wantPriv: "dprv3qF3177i87wMirg6sraDvqty8yZg6THpXFPSXuM5AShBiiUQbq8FhSZDGkYmBNR3RKfBrxzkKDBpsRFJfTnQfLsvpPPqRnakat6hHQA43X9",
			net:      mainNetParams,
		},
		{
			name:     "test vector 2 chain m/0/2147483647H/1/2147483646H/2",
			master:   testVec2MasterHex,
			path:     []uint32{0, hkStart + 2147483647, 1, hkStart + 2147483646, 2},
			wantPub:  "dpubZJoBFoQJ35zvEBgsfhJBssnAp8TY5gvruzQFLmyxcqRb7enVtGfSkLo2CkAZJMpa6T2fx6fUtvTgXtUvSVgAZ56bEwGxQsToeZfFV8VadE1",
			wantPriv: "dprv3s15tfqzxhw8Kmo7RBEqMeyvC7uGekLniSmvbs3bckpxQ6ks1KKqfmH144Jgh3PLxkyZRcS367kp7DrtUmnG16NpnsoNhxSXRgKbJJ7MUQR",
			net:      mainNetParams,
		},

		// Test vector 3
		{
			name:     "test vector 3 chain m",
			master:   testVec3MasterHex,
			path:     []uint32{},
			wantPub:  "dpubZ9169KDAEUnynpjjLH5vXBfmaBym5MB4HsyZ4ZbsXtJh8DyFVRP8ucKKgNTNSyUZZC93dnJX5eSZxeURPmzHpnBDBieisUbaMJteHNHJpPK",
			wantPriv: "dprv3hCznBesA6jBtQqy5m2ZzxsWxBRVeQaz6LMEKefWXoi4QfwccU3Xq2oJXgCZvgKQtRXKdgvkyt35PMhyavDyy95dTc1xjbMGd7vJ61LQan8",
			net:      mainNetParams,
		},
		{
			name:     "test vector 3 chain m/0H",
			master:   testVec3MasterHex,
			path:     []uint32{hkStart},
			wantPub:  "dpubZB4aNJNxjimmDnmC3ZCo7zmSW9ue8i6fRMwpwyHzi15X7j2KHXWSDiPr4ymhbQWg6D3HAjSnqSG3PHDaLVAkUCuRU5ohX9y9z6KL768vYm6",
			wantPriv: "dprv3jGV1ApffLhyKNsRo39SbmyBt9MNhmWbDpKWD4MdhvUtQAzgQaAq98spvFaUmYyNJSPGLJ9JzgVDNN9bQP62tUpLwVigWZMjXmojbERDmhk",
			net:      mainNetParams,
		},
		{
			name:     "test vector 3 chain m/0H/0H",
			master:   testVec3MasterHex,
			path:     []uint32{hkStart, hkStart},
			wantPub:  "dpubZESpio8DE1w5E9h5cBwuER5dmfcqZzh19UTstEDJMTfTAbWPJQ8mCFKeqyVgTFL5n4VmrUgbidcH1FmJaPLEWcdyizsdHocfyJTn8Zw8tkB",
			wantPriv: "dprv3nejMfZv9dsHKjoKMftYiCHP9f4a946vwvqZ9KGwMP4pT3UkRSoA7fodhGCqPVBqgC2wijajQYW9ZQKNMdZ7Ac2L5i7xqHcq4tPzcxzAxhf",
			net:      mainNetParams,
		},

		// Test vector 1 - Testnet
		{
			name:     "test vector 1 chain m - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{},
			wantPub:  "tpubVhnMyQmZAhoosedBTX7oacwyCNc5qtdEMoNHudUCW1R6WZTvqCZQoNJHSn4H11puwdk4qyDv2ET637EDap4r8HH3odjBC5nEjmnPcsDfLwm",
			wantPriv: "tprvZUo1ZuEfLLFWfAYiMVaoDV1EeLmbSRuNzaSh7F4awft7dm8nHfFAFZyobWQyV8Qr26r8M2CmNw6nEb35HaECWFGy1vzx2ZGdyfBeaaHudoi",
			net:      testNetParams,
		},
		{
			name:     "test vector 1 chain m/0H - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart},
			wantPub:  "tpubVm3mQR7aeaowtpbnJKRnvpKsJ3A3q5u31EPY1FAU5Re1zvFfmFUE5aHXY4qmjfQcb1uFZf8mvvU1vi89vEzHPQfR5NETLqByzdthaYfQGja",
			wantPriv: "tprvZY4QzuagpDFegLXKCHtnZgP8k1KZRdBBe1TwCrkrX67387vXDi9yXmy3gnz6tBTyNKcSZmu4wLtVsYzbKZhSVZH89uP7VxV4RboFfozTBMQ",
			net:      testNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1 - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1},
			wantPub:  "tpubVo1FPnCBBQN83JJ9Nx9iGhsqa1Gnf9sBhgsT9nNmPrdvEHHmjQuGQrwKjuTsDzLLrZiNH8dxiHrASd2uQfmTgR6dqrkyVN5p3P2crvfgpEQ",
			wantPriv: "tprvZa1tzGfHM2opppDgGvchuZw71ySJFh9LLTwrMPy9qX6wMUxdBsb1s4cqtcLgZVTQ8EZCJeYagpjaw6gkU16ht5Nr8y2WEc9UWQimC4Y6MFs",
			net:      testNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2},
			wantPub:  "tpubVq8FwnRh6k1JqLrLeoUc3znpCsLM2rytspJJQqPEJf4a7J2tNjQAtrcSYVvPyNnjjscCDaShJLK6mtH6idGo8g8Djpe2HL13VFJgygvRmc9",
			wantPriv: "tprvZc8uYGtoGNT1crmsYmwbgrr5eqVrdQG3WbNhcSyckKXbEVhjqC5vM4HxhDpzqEyyAVNgEBKdKLTT3EXQesyhPyhFoeVy6ZExqrpurCCRvrF",
			net:      testNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H/2 - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2, 2},
			wantPub:  "tpubVrhN2mNRTeTLMsSzuYgQRpCWcYWq1C1UtFXtDyJoARHgLZVaeaj4awdFUNrfCjcvv1y3bGY7YNTSanYx2AHir453T7yd83bd1F8eJgtdq3F",
			wantPriv: "tprvZdi1dFqXdGu39PNXoX9Q4gFn4WgLbjHdX2cHRauBc5khTmAS73Qp39Jmd6BL7U4rw7k45nAfRT9qkuMi4Y6mb9ks1QNUfAmRW9DBY5g5ELT",
			net:      testNetParams,
		},
		{
			name:     "test vector 1 chain m/0H/1/2H/2/1000000000 - testnet",
			master:   testVec1MasterHex,
			path:     []uint32{hkStart, 1, hkStart + 2, 2, 1000000000},
			wantPub:  "tpubVtstygL8beyr57j14nf4JWq5MtSCmHJNp2YHy6XF53oCMYfrZfrWBGQ1JDT95aoC4pMFuBnB8Ftdq9s3yMAFh7UKQd4f3hLL8C8KipwSek6",
			wantPriv: "tprvZftYaAoEmHRYrdeXxm83wNtLorbiMpaXSochAi7dWiGDUkLi28YFdU5XSxe53yHVDdEyfiTsKBRZR2HASwVBhueZRroeuFgD6U9JTC1mUyU",
			net:      testNetParams,
		},
	}

tests:
	for i, test := range tests {
		masterSeed, err := hex.DecodeString(test.master)
		if err != nil {
			t.Errorf("DecodeString #%d (%s): unexpected error: %v",
				i, test.name, err)
			continue
		}

		extKey, err := NewMaster(masterSeed, test.net)
		if err != nil {
			t.Errorf("NewMaster #%d (%s): unexpected error when "+
				"creating new master key: %v", i, test.name,
				err)
			continue
		}

		for _, childNum := range test.path {
			var err error
			extKey, err = extKey.Child(childNum)
			if err != nil {
				t.Errorf("err: %v", err)
				continue tests
			}
		}

		privStr := extKey.String()
		if privStr != test.wantPriv {
			t.Errorf("Serialize #%d (%s): mismatched serialized "+
				"private extended key -- got: %s, want: %s", i,
				test.name, privStr, test.wantPriv)
			continue
		}

		pubKey := extKey.Neuter()

		// Neutering a second time should have no effect.
		// Test for referencial equality.
		if pubKey != pubKey.Neuter() {
			t.Errorf("Neuter of extended public key returned " +
				"different object address")
			return
		}

		pubStr := pubKey.String()
		if pubStr != test.wantPub {
			t.Errorf("Neuter #%d (%s): mismatched serialized "+
				"public extended key -- got: %s, want: %s", i,
				test.name, pubStr, test.wantPub)
			continue
		}
	}
}

// TestPrivateDerivation tests several vectors which derive private keys from
// other private keys works as intended.
func TestPrivateDerivation(t *testing.T) {
	// The private extended keys for test vectors in [BIP32].
	testVec1MasterPrivKey := "dprv3hCznBesA6jBucms1ZhyGeFfvJfBSwfs7ZFrxS8tdYzbjDZe2UwSaL7EbYo1qa88DmtyyG5cL9tdGxHkD89JmeZTbz5sVYU4Dgtijiio4Sc"
	testVec2MasterPrivKey := "dprv3hCznBesA6jBtPKJbQTxRZAKG2gyj8tZKEPaCsV4e9YYFBAgRP2eTSPAeu4r8dTMt9q51j2Vdt5zNqj7jbtovvocrP1qLj6WUTLF9xYQt4y"

	tests := []struct {
		name     string
		master   string
		path     []uint32
		wantPriv string
	}{
		// Test vector 1
		{
			name:     "test vector 1 chain m",
			master:   testVec1MasterPrivKey,
			path:     []uint32{},
			wantPriv: "dprv3hCznBesA6jBucms1ZhyGeFfvJfBSwfs7ZFrxS8tdYzbjDZe2UwSaL7EbYo1qa88DmtyyG5cL9tdGxHkD89JmeZTbz5sVYU4Dgtijiio4Sc",
		},
		{
			name:     "test vector 1 chain m/0",
			master:   testVec1MasterPrivKey,
			path:     []uint32{0},
			wantPriv: "dprv3jFfEhxvVxy6NJWopujhfg7syQL71xCRgNoGUpQTtjTpCwzigwtCwssQGbRQsby7PBs1Yp8Wu7isu396qeNof13EZuxbCTJVF1xkoFAQHWj",
		},
		{
			name:     "test vector 1 chain m/0/1",
			master:   testVec1MasterPrivKey,
			path:     []uint32{0, 1},
			wantPriv: "dprv3mWLns1v1fdLhxStaJHh3BqxmTi14RHHeWdNU6oU8sSkTDmAr54yK6La2APy3rAZr9ZJAdm5asTJaqBZ3vBYVSPHqyL8kbcCp5jgqfxBs4x",
		},
		{
			name:     "test vector 1 chain m/0/1/2",
			master:   testVec1MasterPrivKey,
			path:     []uint32{0, 1, 2},
			wantPriv: "dprv3oDxSziXR1rQVWwWWBRKgCQU3vN6dnR3ekzHzvZRdgfVvrYSE35saJh8UdfSxCgtMn7pnbeMXWbyBbwxoncC9LMrnuH1AoJSB259c4XgmnN",
		},
		{
			name:     "test vector 1 chain m/0/1/2/2",
			master:   testVec1MasterPrivKey,
			path:     []uint32{0, 1, 2, 2},
			wantPriv: "dprv3rYHNih25i8MqeRjhFq8mLnK4a3J63a9zkYTCFJd8kaJuNc5aDAAuG1XopkU7h93HvfbNvQaWdQLtwFmUEbDN3GCZ2Mxw6tq5ZSh8d1Chyw",
		},
		{
			name:     "test vector 1 chain m/0/1/2/2/1000000000",
			master:   testVec1MasterPrivKey,
			path:     []uint32{0, 1, 2, 2, 1000000000},
			wantPriv: "dprv3tKkzgLFKaX2VcQTir9JeNHWxikiKokSJtdyj7sYoiDkU3np2rc3DGYPRVmDhb2FFaAk98fnqRotQYTVCRaoyAZiHaoyNoPCFeYA9pEshBT",
		},

		// Test vector 2
		{
			name:     "test vector 2 chain m",
			master:   testVec2MasterPrivKey,
			path:     []uint32{},
			wantPriv: "dprv3hCznBesA6jBtPKJbQTxRZAKG2gyj8tZKEPaCsV4e9YYFBAgRP2eTSPAeu4r8dTMt9q51j2Vdt5zNqj7jbtovvocrP1qLj6WUTLF9xYQt4y",
		},
		{
			name:     "test vector 2 chain m/0",
			master:   testVec2MasterPrivKey,
			path:     []uint32{0},
			wantPriv: "dprv3jMy45BuuDETfxi59P8NTSjHPrNVq4wPRfLgRd57923L2hosj5NUEqiLYQ4i7fJtUpiXZLr2wUeToJY2Tm5sCpAJdajEHDmieVJiPQNXwu9",
		},
		{
			name:     "test vector 2 chain m/0/2147483647",
			master:   testVec2MasterPrivKey,
			path:     []uint32{0, 2147483647},
			wantPriv: "dprv3mgHPRgAnNboAb9edL4RPscKYrNLG77BhPvFe3eiTGPSiigDeXct3WeiZ2QqRrm9TiseBuYWGEG79xkBzazpBGfym1vRXjcEo5KUi4rbhZ1",
		},
		{
			name:     "test vector 2 chain m/0/2147483647/1",
			master:   testVec2MasterPrivKey,
			path:     []uint32{0, 2147483647, 1},
			wantPriv: "dprv3onqUUAjN1xQUAW1BzwiBa5wisDCE8hX8YcAEgVe1DPvbgHVLauDt2NsZPQX6tJs6ozcQSU9GdsffhueTbxTxFMPEMPxM2iHiooMz2oQHWS",
		},
		{
			name:     "test vector 2 chain m/0/2147483647/1/2147483646",
			master:   testVec2MasterPrivKey,
			path:     []uint32{0, 2147483647, 1, 2147483646},
			wantPriv: "dprv3r8NTJgGzAjY5cU7sLo5rhT8o2wdco2iqjks6nJoiDACTucrPMcsciccv9skwGMX69uRa8EaZofskV7YyzBDbVi6v4RXbJ4DyeZ6JpUgdUi",
		},
		{
			name:     "test vector 2 chain m/0/2147483647/1/2147483646/2",
			master:   testVec2MasterPrivKey,
			path:     []uint32{0, 2147483647, 1, 2147483646, 2},
			wantPriv: "dprv3sp4xvFP9mL9UUEddSZUxrtNnhe5UcHs5wrpxdZVEFCXoT4EpYeHZJjCDhvVEQFK2KfSHXFmew6MeBuvtrJfQv1BnkiSV7xxUji66uvWasp",
		},

		// Custom tests to trigger specific conditions.
		{
			// Seed 000000000000000000000000000000da.
			name:     "Derived privkey with zero high byte m/0",
			master:   "dprv3jFfEhxvVxy6NJWopujhfg7syQL71xCRgNoGUpQTtjTpCwzigwtCwssQGbRQsby7PBs1Yp8Wu7isu396qeNof13EZuxbCTJVF1xkoFAQHWj",
			path:     []uint32{0},
			wantPriv: "dprv3mWLns1v1fdLfeu5DKTA6NWQHLF6pFsPSwKCS6q4h4nkjm2DfuH5X2iDnW15jhHTGa3rzxSpvskuXugcbBcUUVWCETKKzjW7ja4V2jL4aw4",
		},
	}

	mainNetParms := mockMainNetParams()
tests:
	for i, test := range tests {
		extKey, err := NewKeyFromString(test.master, mainNetParms)
		if err != nil {
			t.Errorf("NewKeyFromString #%d (%s): unexpected error "+
				"creating extended key: %v", i, test.name,
				err)
			continue
		}

		for _, childNum := range test.path {
			var err error
			extKey, err = extKey.Child(childNum)
			if err != nil {
				t.Errorf("err: %v", err)
				continue tests
			}
		}

		privStr := extKey.String()
		if privStr != test.wantPriv {
			t.Errorf("Child #%d (%s): mismatched serialized "+
				"private extended key -- got: %s, want: %s", i,
				test.name, privStr, test.wantPriv)
			continue
		}
	}
}

// TestPublicDerivation tests several vectors which derive public keys from
// other public keys works as intended.
func TestPublicDerivation(t *testing.T) {
	// The public extended keys for test vectors in [BIP32].
	testVec1MasterPubKey := "dpubZF8BRmciAzYoTjXZ3bbRWLVCwUKtTquact3Tr6ye77Rgmw76VyqMb9TB9KpfrvUYEM5d1Au4fQzE2BbtxRjwzGsqnWHmtQP9UV1kxZaqvb6"
	testVec2MasterPubKey := "dpubZF4LSCdF9YKZfNzTVYhz4RBxsjYXqms8AQnMBHXZ8GUKoRSigG7kQnKiJt5pzk93Q8FxcdVBEkQZruSXduGtWnkwXzGnjbSovQ97dCxqaXc"

	tests := []struct {
		name    string
		master  string
		path    []uint32
		wantPub string
	}{
		// Test vector 1
		{
			name:    "test vector 1 chain m",
			master:  testVec1MasterPubKey,
			path:    []uint32{},
			wantPub: "dpubZF8BRmciAzYoTjXZ3bbRWLVCwUKtTquact3Tr6ye77Rgmw76VyqMb9TB9KpfrvUYEM5d1Au4fQzE2BbtxRjwzGsqnWHmtQP9UV1kxZaqvb6",
		},
		{
			name:    "test vector 1 chain m/0",
			master:  testVec1MasterPubKey,
			path:    []uint32{0},
			wantPub: "dpubZHm6cmVU9pvfDCe3BY7iESzsEnV6xfi4DfoYvycnWLM9cryzKA84DqJ2CphYq6cfiEXgo9C3YLJA4ou81mavw9NDtNc3bLCWVqJz8Fx8qxB",
		},
		{
			name:    "test vector 1 chain m/0/1",
			master:  testVec1MasterPubKey,
			path:    []uint32{0, 1},
			wantPub: "dpubZKtA6UTDuxeXV2PcYqoe68u7cgDhbTNbA4dUJoaAvfWzuCcRQCyG5S6dbpDZb2p3B5Y2XxLtD94Nemc8QRV4RspmvGwHvE2FZsfE5Pqpeor",
		},
		{
			name:    "test vector 1 chain m/0/1/2",
			master:  testVec1MasterPubKey,
			path:    []uint32{0, 1, 2},
			wantPub: "dpubZMwLXm5dRVEJRvJHU8gNV7RwHeXMRRUnYFD4f6C8uNFfqksD1FCDARTwNPsQB3Pg4LuoKXkZbPnE6woUyedwNYVPvZToT5x4Kt6rs4GKa9c",
		},
		{
			name:    "test vector 1 chain m/0/1/2/2",
			master:  testVec1MasterPubKey,
			path:    []uint32{0, 1, 2, 2},
			wantPub: "dpubZPfASfojwk6MhtAtkM6wPdQBr1ycVjoyqs3N51zR1keK6FcBhjBTtdW3Wn3kDLBZqgLnGozu8Gh3FV8GrFGpu3knmGVoF1Z6yGdqLU1Rz1S",
		},
		{
			name:    "test vector 1 chain m/0/1/2/2/1000000000",
			master:  testVec1MasterPubKey,
			path:    []uint32{0, 1, 2, 2, 1000000000},
			wantPub: "dpubZR5Pf8cbUGikESevygwydenBaTsgcvoYnRSi7tygu23PxmVEG4GeMQj54oHFoPyRdt7Pg4sMad56yprQszbNyZVewaNEhDkn112C3mqB1fd",
		},

		// Test vector 2
		{
			name:    "test vector 2 chain m",
			master:  testVec2MasterPubKey,
			path:    []uint32{},
			wantPub: "dpubZF4LSCdF9YKZfNzTVYhz4RBxsjYXqms8AQnMBHXZ8GUKoRSigG7kQnKiJt5pzk93Q8FxcdVBEkQZruSXduGtWnkwXzGnjbSovQ97dCxqaXc",
		},
		{
			name:    "test vector 2 chain m/0",
			master:  testVec2MasterPubKey,
			path:    []uint32{0},
			wantPub: "dpubZHJs2Z3PtHbbpaXQCi5wBKPhU8tC5ztBKUYBCYNGKk8eZ1EmBs3MhnLJbxHFMAahGnDnZT7qZxC7AXKP8PB6BDNUZgkG77moNMRmXyQ6s6s",
		},
		{
			name:    "test vector 2 chain m/0/2147483647",
			master:  testVec2MasterPubKey,
			path:    []uint32{0, 2147483647},
			wantPub: "dpubZJgFEUcAZawGaLZdFEX6FfQBQVgU4bUC5qvDERUTD5dfcB2AQPnJ1dKp1R2DrAzC36BznZG43317s2oBJv3PuaZmA6HqmwMu6vNna4Gfumf",
		},
		{
			name:    "test vector 2 chain m/0/2147483647/1",
			master:  testVec2MasterPubKey,
			path:    []uint32{0, 2147483647, 1},
			wantPub: "dpubZLbgtFNyjt3k2cJtg4a3dD2iXPKFTLgNKP8rLC1p5UE3AyfRHLTcYrZ6brg8eUmGvKRrXZ7A3XyVfwGxvYtjfz8514dUoJPkmSnBmC6qQK6",
		},
		{
			name:    "test vector 2 chain m/0/2147483647/1/2147483646",
			master:  testVec2MasterPubKey,
			path:    []uint32{0, 2147483647, 1, 2147483646},
			wantPub: "dpubZNyWTupEG35S6d4uN93vWXpGxQuxtW9zuThQbnWpWTHwRCzxREqSSc9eDYivRGiZnEkEhPece5ciSoHtW6Khc729f6eAxjPnBgU38U9hgYw",
		},
		{
			name:    "test vector 2 chain m/0/2147483647/1/2147483646/2",
			master:  testVec2MasterPubKey,
			path:    []uint32{0, 2147483647, 1, 2147483646, 2},
			wantPub: "dpubZRuRErXqhdJaZWD1AzXB6d5w2zw7UZ7ALxiS1gHbnQbVEohBzQzsVwGRzq97pmuE7ToA6DGn2QTH4DexxzdnMvkiYUpk8Nh2KEuYUM2RCeU",
		},
	}

	mainNetParams := mockMainNetParams()
tests:
	for i, test := range tests {
		extKey, err := NewKeyFromString(test.master, mainNetParams)
		if err != nil {
			t.Errorf("NewKeyFromString #%d (%s): unexpected error "+
				"creating extended key: %v", i, test.name,
				err)
			continue
		}

		for _, childNum := range test.path {
			var err error
			extKey, err = extKey.Child(childNum)
			if err != nil {
				t.Errorf("err: %v", err)
				continue tests
			}
		}

		pubStr := extKey.String()
		if pubStr != test.wantPub {
			t.Errorf("Child #%d (%s): mismatched serialized "+
				"public extended key -- got: %s, want: %s", i,
				test.name, pubStr, test.wantPub)
			continue
		}
	}
}

// TestGenenerateSeed ensures the GenerateSeed function works as intended.
func TestGenenerateSeed(t *testing.T) {
	tests := []struct {
		name   string
		length uint8
		err    error
	}{
		// Test various valid lengths.
		{name: "16 bytes", length: 16},
		{name: "17 bytes", length: 17},
		{name: "20 bytes", length: 20},
		{name: "32 bytes", length: 32},
		{name: "64 bytes", length: 64},

		// Test invalid lengths.
		{name: "15 bytes", length: 15, err: ErrInvalidSeedLen},
		{name: "65 bytes", length: 65, err: ErrInvalidSeedLen},
	}

	for i, test := range tests {
		seed, err := GenerateSeed(test.length)
		if !errors.Is(err, test.err) {
			t.Errorf("GenerateSeed #%d (%s): unexpected error -- "+
				"want %v, got %v", i, test.name, test.err, err)
			continue
		}

		if test.err == nil && len(seed) != int(test.length) {
			t.Errorf("GenerateSeed #%d (%s): length mismatch -- "+
				"got %d, want %d", i, test.name, len(seed),
				test.length)
			continue
		}
	}
}

// TestExtendedKeyAPI ensures the API on the ExtendedKey type works as intended.
func TestExtendedKeyAPI(t *testing.T) {
	tests := []struct {
		name       string
		extKey     string
		isPrivate  bool
		parentFP   uint32
		privKey    string
		privKeyErr error
		pubKey     string
	}{
		{
			name:      "test vector 1 master node private",
			extKey:    "dprv3hCznBesA6jBteCheVrorP4Nix6oEWFwtk79FaLy1rPzedNGg9Jhy9y596C9uCPMeeKJMvtEGZRaxPKxGuCFgR4uo2EySsA29GsnXcrsby8",
			isPrivate: true,
			parentFP:  0,
			privKey:   "33a63922ea4e6686c9fc31daf136888297537f66c1aabe3363df06af0b8274c7",
			pubKey:    "039f2e1d7b50b8451911c64cf745f9ba16193b319212a64096e5679555449d8f37",
		},
		{
			name:       "test vector 2 chain m/0/2147483647/1/2147483646/2",
			extKey:     "dpubZRuRErXqhdJaZWD1AzXB6d5w2zw7UZ7ALxiS1gHbnQbVEohBzQzsVwGRzq97pmuE7ToA6DGn2QTH4DexxzdnMvkiYUpk8Nh2KEuYUM2RCeU",
			isPrivate:  false,
			parentFP:   4220580796,
			privKeyErr: ErrNotPrivExtKey,
			pubKey:     "03dceb0b07698ec3d6ac08ae7297e7f5e63d7fda99d3fce1ded31d36badcdd4d36",
		},
	}

	mainNetParams := mockMainNetParams()
	for i, test := range tests {
		key, err := NewKeyFromString(test.extKey, mainNetParams)
		if err != nil {
			t.Errorf("NewKeyFromString #%d (%s): unexpected "+
				"error: %v", i, test.name, err)
			continue
		}

		if key.IsPrivate() != test.isPrivate {
			t.Errorf("IsPrivate #%d (%s): mismatched key type -- "+
				"want private %v, got private %v", i, test.name,
				test.isPrivate, key.IsPrivate())
			continue
		}

		parentFP := key.ParentFingerprint()
		if parentFP != test.parentFP {
			t.Errorf("ParentFingerprint #%d (%s): mismatched "+
				"parent fingerprint -- want %d, got %d", i,
				test.name, test.parentFP, parentFP)
			continue
		}

		serializedKey := key.String()
		if serializedKey != test.extKey {
			t.Errorf("String #%d (%s): mismatched serialized key "+
				"-- want %s, got %s", i, test.name, test.extKey,
				serializedKey)
			continue
		}

		privKey, err := key.SerializedPrivKey()
		if !errors.Is(err, test.privKeyErr) {
			t.Errorf("SerializedPrivKey #%d (%s): mismatched "+
				"error: want %v, got %v", i, test.name,
				test.privKeyErr, err)
			continue
		}
		if test.privKeyErr == nil {
			privKeyStr := hex.EncodeToString(privKey)
			if privKeyStr != test.privKey {
				t.Errorf("SerializedPrivKey #%d (%s): mismatched "+
					"private key -- want %s, got %s", i,
					test.name, test.privKey, privKeyStr)
				continue
			}
		}

		pubKeyStr := hex.EncodeToString(key.SerializedPubKey())
		if pubKeyStr != test.pubKey {
			t.Errorf("SerializedPubKey #%d (%s): mismatched public "+
				"key -- want %s, got %s", i, test.name, test.pubKey,
				pubKeyStr)
			continue
		}
	}
}

// TestErrors performs some negative tests for various invalid cases to ensure
// the errors are handled properly.
func TestErrors(t *testing.T) {
	// Should get an error when seed has too few bytes.
	net := mockMainNetParams()
	_, err := NewMaster(bytes.Repeat([]byte{0x00}, 15), net)
	if !errors.Is(err, ErrInvalidSeedLen) {
		t.Errorf("NewMaster: mismatched error -- got: %v, want: %v",
			err, ErrInvalidSeedLen)
	}

	// Should get an error when seed has too many bytes.
	_, err = NewMaster(bytes.Repeat([]byte{0x00}, 65), net)
	if !errors.Is(err, ErrInvalidSeedLen) {
		t.Errorf("NewMaster: mismatched error -- got: %v, want: %v",
			err, ErrInvalidSeedLen)
	}

	// Generate a new key and neuter it to a public extended key.
	seed, err := GenerateSeed(RecommendedSeedLen)
	if err != nil {
		t.Errorf("GenerateSeed: unexpected error: %v", err)
		return
	}
	extKey, err := NewMaster(seed, net)
	if err != nil {
		t.Errorf("NewMaster: unexpected error: %v", err)
		return
	}
	pubKey := extKey.Neuter()

	// Deriving a hardened child extended key should fail from a public key.
	_, err = pubKey.Child(HardenedKeyStart)
	if !errors.Is(err, ErrDeriveHardFromPublic) {
		t.Errorf("Child: mismatched error -- got: %v, want: %v",
			err, ErrDeriveHardFromPublic)
	}

	// NewKeyFromString failure tests.
	tests := []struct {
		name string
		key  string
		err  error
	}{
		{
			name: "invalid key length",
			key:  "dpub1234",
			err:  ErrInvalidKeyLen,
		},
		{
			name: "bad checksum",
			key:  "dpubZF6AWaFizAuUcbkZSs8cP8Gxzr6Sg5tLYYM7gEjZMC5GDaSHB4rW4F51zkWyo9U19BnXhc99kkEiPg248bYin8m9b8mGss9nxV6N2QpU8vj",
			err:  ErrBadChecksum,
		},
		{
			name: "pubkey not on curve",
			key:  "dpubZ9169KDAEUnyoTzA7pDGtXbxpji5LuUk8johUPVGY2CDsz6S7hahGNL6QkeYrUeAPnaJD1MBmrsUnErXScGZdjL6b2gjCRX1Z1GNhLdVCjv",
			err:  secp256k1.ErrPubKeyNotOnCurve,
		},
		{
			name: "unsupported version",
			key:  "4s9bfpYH9CkJboPNLFC4BhTENPrjfmKwUxesnqxHBjv585bCLzVdQKuKQ5TouA57FkdDskrR695Z5U2wWwDUUVWXPg7V57sLpc9dMgx74LsVZGEB",
			err:  ErrWrongNetwork,
		},
	}

	mainNetParams := mockMainNetParams()
	for i, test := range tests {
		_, err := NewKeyFromString(test.key, mainNetParams)
		if !errors.Is(err, test.err) {
			t.Errorf("NewKeyFromString #%d (%s): mismatched error "+
				"-- got: %v, want: %v", i, test.name, err,
				test.err)
			continue
		}
	}
}

// TestZero ensures that zeroing an extended key works as intended.
func TestZero(t *testing.T) {
	mainNetParams := mockMainNetParams()
	tests := []struct {
		name   string
		master string
		extKey string
		net    NetworkParams
	}{
		// Test vector 1
		{
			name:   "test vector 1 chain m",
			master: "000102030405060708090a0b0c0d0e0f",
			extKey: "dprv3hCznBesA6jBtmoyVFPfyMSZ1qYZ3WdjdebquvkEfmRfxC9VFEFi2YDaJqHnx7uGe75eGSa3Mn3oHK11hBW7KZUrPxwbCPBmuCi1nwm182s",
			net:    mainNetParams,
		},

		// Test vector 2
		{
			name:   "test vector 2 chain m",
			master: "fffcf9f6f3f0edeae7e4e1dedbd8d5d2cfccc9c6c3c0bdbab7b4b1aeaba8a5a29f9c999693908d8a8784817e7b7875726f6c696663605d5a5754514e4b484542",
			extKey: "dprv3hCznBesA6jBtPKJbQTxRZAKG2gyj8tZKEPaCsV4e9YYFBAgRP2eTSPAeu4r8dTMt9q51j2Vdt5zNqj7jbtovvocrP1qLj6WUTLF9xYQt4y",
			net:    mainNetParams,
		},
	}

	// Use a closure to test that a key is zeroed since the tests create
	// keys in different ways and need to test the same things multiple
	// times.
	testZeroed := func(i int, testName string, key *ExtendedKey) bool {
		// Zeroing a key should result in it no longer being private
		if key.IsPrivate() {
			t.Errorf("IsPrivate #%d (%s): mismatched key type -- "+
				"want private %v, got private %v", i, testName,
				false, key.IsPrivate())
			return false
		}

		parentFP := key.ParentFingerprint()
		if parentFP != 0 {
			t.Errorf("ParentFingerprint #%d (%s): mismatched "+
				"parent fingerprint -- want %d, got %d", i,
				testName, 0, parentFP)
			return false
		}

		wantKey := "zeroed extended key"
		serializedKey := key.String()
		if serializedKey != wantKey {
			t.Errorf("String #%d (%s): mismatched serialized key "+
				"-- want %s, got %s", i, testName, wantKey,
				serializedKey)
			return false
		}

		wantErr := ErrNotPrivExtKey
		_, err := key.SerializedPrivKey()
		if !reflect.DeepEqual(err, wantErr) {
			t.Errorf("SerializedPrivKey #%d (%s): mismatched "+
				"error: want %v, got %v", i, testName, wantErr,
				err)
			return false
		}

		serializedPubKey := key.SerializedPubKey()
		if len(serializedPubKey) != 0 {
			t.Errorf("ECPubKey #%d (%s): mismatched serialized "+
				"pubkey: want nil, got %x", i, testName,
				serializedPubKey)
			return false
		}

		return true
	}

	for i, test := range tests {
		// Create new key from seed and get the neutered version.
		masterSeed, err := hex.DecodeString(test.master)
		if err != nil {
			t.Errorf("DecodeString #%d (%s): unexpected error: %v",
				i, test.name, err)
			continue
		}
		key, err := NewMaster(masterSeed, test.net)
		if err != nil {
			t.Errorf("NewMaster #%d (%s): unexpected error when "+
				"creating new master key: %v", i, test.name,
				err)
			continue
		}
		neuteredKey := key.Neuter()

		// Ensure both non-neutered and neutered keys are zeroed
		// properly.
		key.Zero()
		if !testZeroed(i, test.name+" from seed not neutered", key) {
			continue
		}
		neuteredKey.Zero()
		if !testZeroed(i, test.name+" from seed neutered", key) {
			continue
		}

		// Deserialize key and get the neutered version.
		key, err = NewKeyFromString(test.extKey, mainNetParams)
		if err != nil {
			t.Errorf("NewKeyFromString #%d (%s): unexpected "+
				"error: %v", i, test.name, err)
			continue
		}
		neuteredKey = key.Neuter()

		// Ensure both non-neutered and neutered keys are zeroed
		// properly.
		key.Zero()
		if !testZeroed(i, test.name+" deserialized not neutered", key) {
			continue
		}
		neuteredKey.Zero()
		if !testZeroed(i, test.name+" deserialized neutered", key) {
			continue
		}
	}
}
