// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package schnorr

import (
	"bytes"
	"math/rand"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec/secp256k1/v3"
)

// TestSchnorrSignAndVerify ensures the Schnorr signing function produces the
// expected signatures for a selected set of private keys, messages, and nonces
// that have been independently verified with the Sage computer algebra system.
// It also ensures verifying the signature works as expected.
func TestSchnorrSignAndVerify(t *testing.T) {
	tests := []struct {
		name     string // test description
		key      string // hex encded private key
		msg      string // hex encoded message to sign before hashing
		hash     string // hex encoded hash of the message to sign
		nonce    string // hex encoded nonce to use in the signature calculation
		rfc6979  bool   // whether or not the nonce is an RFC6979 nonce
		expected string // expected signature
	}{{
		name:    "key 0x1, blake256(0x01020304), rfc6979 nonce",
		key:     "0000000000000000000000000000000000000000000000000000000000000001",
		msg:     "01020304",
		hash:    "c301ba9de5d6053caad9f5eb46523f007702add2c62fa39de03146a36b8026b7",
		nonce:   "4154324ecd4158938f1df8b5b659aeb639c7fbc36005934096e514af7d64bcc2",
		rfc6979: true,
		expected: "c6c4137b0e5fbfc88ae3f293d7e80c8566c43ae20340075d44f75b009c943d09" +
			"fe359056a1f6090fe2599acbe6b315be1e2dc7a675545ec96bb8a12d1a144deb",
	}, {
		name:    "key 0x1, blake256(0x01020304), random nonce",
		key:     "0000000000000000000000000000000000000000000000000000000000000001",
		msg:     "01020304",
		hash:    "c301ba9de5d6053caad9f5eb46523f007702add2c62fa39de03146a36b8026b7",
		nonce:   "a6df66500afeb7711d4c8e2220960855d940a5ed57260d2c98fbf6066cca283e",
		rfc6979: false,
		expected: "b073759a96a835b09b79e7b93c37fdbe48fb82b000c4a0e1404ba5d1fbc15d0a" +
			"299d614b02dec30f8261ae43d09a224b233f3221405c9ffd3d2b00a3d2188fd4",
	}, {
		name:    "key 0x2, blake256(0x01020304), rfc6979 nonce",
		key:     "0000000000000000000000000000000000000000000000000000000000000002",
		msg:     "01020304",
		hash:    "c301ba9de5d6053caad9f5eb46523f007702add2c62fa39de03146a36b8026b7",
		nonce:   "55f96f24cf7531f527edfe3b9222eca12d575367c32a7f593a828dc3651acf49",
		rfc6979: true,
		expected: "e6f137b52377250760cc702e19b7aee3c63b0e7d95a91939b14ab3b5c4771e59" +
			"57341a8bb99ea27ccebee775fe6ea24b560763bb499d03354e739c3cd32d6629",
	}, {
		name:    "key 0x2, blake256(0x01020304), random nonce",
		key:     "0000000000000000000000000000000000000000000000000000000000000002",
		msg:     "01020304",
		hash:    "c301ba9de5d6053caad9f5eb46523f007702add2c62fa39de03146a36b8026b7",
		nonce:   "679a6d36e7fe6c02d7668af86d78186e8f9ccc04371ac1c8c37939d1f5cae07a",
		rfc6979: false,
		expected: "4a090d82f48ca12d9e7aa24b5dcc187ee0db2920496f671d63e86036aaa7997e" +
			"16d33ae10eade4db33dda17873948b4803d6eb9b10781616880a6f66ba2d1b78",
	}, {
		name:    "key 0x1, blake256(0x0102030405), rfc6979 nonce",
		key:     "0000000000000000000000000000000000000000000000000000000000000001",
		msg:     "0102030405",
		hash:    "dc063eba3c8d52a159e725c1a161506f6cb6b53478ad5ef3f08d534efa871d9f",
		nonce:   "aa87a543c68f2568bb107c9946afa5233bf94fb6a7a063544505282621021629",
		rfc6979: true,
		expected: "dda8308cdbda2edf51ccf598b42b42b19597e102eb2ed4a04a16dd57084d3b40" +
			"5798fd44eba62ca35aa7efb27677414b46b7adfe6c523ccc5b18019314acaa71",
	}, {
		name:    "key 0x1, blake256(0x0102030405), random nonce",
		key:     "0000000000000000000000000000000000000000000000000000000000000001",
		msg:     "0102030405",
		hash:    "dc063eba3c8d52a159e725c1a161506f6cb6b53478ad5ef3f08d534efa871d9f",
		nonce:   "65f880c892fdb6e7f74f76b18c7c942cfd037ef9cf97c39c36e08bbc36b41616",
		rfc6979: false,
		expected: "72e5666f4e9d1099447b825cf737ee32112f17a67e2ca7017ae098da31dfbb8b" +
			"c19f5a4f815e9737f1b635075c50b3fa28dbbbebfcb98749b9f3c7b0fa748422",
	}, {
		name:    "key 0x2, blake256(0x0102030405), rfc6979 nonce",
		key:     "0000000000000000000000000000000000000000000000000000000000000002",
		msg:     "0102030405",
		hash:    "dc063eba3c8d52a159e725c1a161506f6cb6b53478ad5ef3f08d534efa871d9f",
		nonce:   "a13d652abd54b6e862548e5d12716df14dc192d93f3fa13536fdf4e56c54f233",
		rfc6979: true,
		expected: "122663fd29e41a132d3c8329cf05d61ebcca9351074cc277dcd868faba58d87d" +
			"e2d0532d74bcff2c8b643d098546590262f42f01826c28b656cf67e89514e655",
	}, {
		name:    "key 0x2, blake256(0x0102030405), random nonce",
		key:     "0000000000000000000000000000000000000000000000000000000000000002",
		msg:     "0102030405",
		hash:    "dc063eba3c8d52a159e725c1a161506f6cb6b53478ad5ef3f08d534efa871d9f",
		nonce:   "026ece4cfb704733dd5eef7898e44c33bd5a0d749eb043f48705e40fa9e9afa0",
		rfc6979: false,
		expected: "3c4c5a2f217ea758113fd4e89eb756314dfad101a300f48e5bd764d3b6e0f8bf" +
			"c29f43beed7d84348386152f1c43fc606d0887fa5b6f5c0b7875687f53b344f0",
	}, {
		name:    "random key 1, blake256(0x01), rfc6979 nonce",
		key:     "a1becef2069444a9dc6331c3247e113c3ee142edda683db8643f9cb0af7cbe33",
		msg:     "01",
		hash:    "4a6c419a1e25c85327115c4ace586decddfe2990ed8f3d4d801871158338501d",
		nonce:   "edb3a01063a0c6ccfc0d77295077cbd322cf364bfa64b7eeea3b20305135d444",
		rfc6979: true,
		expected: "ef392791d87afca8256c4c9c68d981248ee34a09069f50fa8dfc19ae34cd92ce" +
			"14f6553176e5ca434831720f02c4c0c6b6228338cfa00aaae4aa6da5b72aae05",
	}, {
		name:    "random key 2, blake256(0x02), rfc6979 nonce",
		key:     "59930b76d4b15767ec0e8c8e5812aa2e57db30c6af7963e2a6295ba02af5416b",
		msg:     "02",
		hash:    "49af37ab5270015fe25276ea5a3bb159d852943df23919522a202205fb7d175c",
		nonce:   "af2a59085976494567ef0fc2ecede587b2d1d8e9898cc46e72d7f3e33156e057",
		rfc6979: true,
		expected: "886c9cccb356b3e1deafef2c276a4f8717ab73c1244c3f673cfbff5897de0e06" +
			"0ef8e0ed66405379d05cf34934ccb3a38a8d800d35d4f0577b96f3cc316fb09d",
	}, {
		name:    "random key 3, blake256(0x03), rfc6979 nonce",
		key:     "c5b205c36bb7497d242e96ec19a2a4f086d8daa919135cf490d2b7c0230f0e91",
		msg:     "03",
		hash:    "b706d561742ad3671703c247eb927ee8a386369c79644131cdeb2c5c26bf6c5d",
		nonce:   "82d82b696a386d6d7a111c4cb943bfd39de8e5f6195e7eed9d3edb40fe1419fa",
		rfc6979: true,
		expected: "6589d5950cec1fe2e7e20593b5ffa3556de20c176720a1796aa77a0cec1ec5a7" +
			"194f86c06b44bde164fcaaac03452c55dde0ba51a27c6397363b54cb705d3afa",
	}, {
		name:    "random key 4, blake256(0x04), rfc6979 nonce",
		key:     "65b46d4eb001c649a86309286aaf94b18386effe62c2e1586d9b1898ccf0099b",
		msg:     "04",
		hash:    "4c6eb9e38415034f4c93d3304d10bef38bf0ad420eefd0f72f940f11c5857786",
		nonce:   "7afd696a9e770961d2b2eaec77ab7c22c734886fa57bc4a50a9f1946168cd06f",
		rfc6979: true,
		expected: "81db1d6dca08819ad936d3284a359091e57c036648d477b96af9d8326965a7d1" +
			"b5f807b7b4b62ff9f946d802c896d3bdf4eccdfc364f983aa29aa99da68df390",
	}, {
		name:    "random key 5, blake256(0x05), rfc6979 nonce",
		key:     "915cb9ba4675de06a182088b182abcf79fa8ac989328212c6b866fa3ec2338f9",
		msg:     "05",
		hash:    "bdd15db13448905791a70b68137445e607cca06cc71c7a58b9b2e84a06c54d08",
		nonce:   "2a6ae70ea5cf1b932331901d640ece54551f5f33bf9484d5f95c676b5612b527",
		rfc6979: true,
		expected: "47fd51aecbc743477cb59aa29d18d11d75fb206ae1cdd044216e4f294e33d5b6" +
			"58541d8273675ac15d6e2113d56756febdc85c977716422761d4332d508d0ebe",
	}, {
		name:    "random key 6, blake256(0x06), rfc6979 nonce",
		key:     "93e9d81d818f08ba1f850c6dfb82256b035b42f7d43c1fe090804fb009aca441",
		msg:     "06",
		hash:    "19b7506ad9c189a9f8b063d2aee15953d335f5c88480f8515d7d848e7771c4ae",
		nonce:   "0b847a0ae0cbe84dfca66621f04f04b0f2ec190dce10d43ba8c3915c0fcd90ed",
		rfc6979: true,
		expected: "c99800bc7ac7ea11afe5d7a264f4c26edd63ae9c7ecd6d0d19992980bcda1d34" +
			"02ac38658b8102e6ebb35707f550ab106d27e2ce7f20e7ed14f8227b1ca2c6ba",
	}, {
		name:    "random key 7, blake256(0x07), rfc6979 nonce",
		key:     "c249bbd5f533672b7dcd514eb1256854783531c2b85fe60bf4ce6ea1f26afc2b",
		msg:     "07",
		hash:    "53d661e71e47a0a7e416591200175122d83f8af31be6a70af7417ad6f54d0038",
		nonce:   "0f8e20694fe766d7b79e5ac141e3542f2f3c3d2cc6d0f60e0ec263a46dbe6d49",
		rfc6979: true,
		expected: "7a57a5222fb7d615eaa0041193f682262cebfa9b448f9c519d3644d0a3348521" +
			"c2d8d91ba147a3f59557f02465f3e4e29adcc0988cbda169ed4a0e6992b258b1",
	}, {
		name:    "random key 8, blake256(0x08), rfc6979 nonce",
		key:     "ec0be92fcec66cf1f97b5c39f83dfd4ddcad0dad468d3685b5eec556c6290bcc",
		msg:     "08",
		hash:    "9bff7982eab6f7883322edf7bdc86a23c87ca1c07906fbb1584f57b197dc6253",
		nonce:   "ab7df49257d18f5f1b730cc7448f46bd82eb43e6e220f521fa7d23802310e24d",
		rfc6979: true,
		expected: "64f90b09c8b1763a3eeefd156e5d312f80a98c24017811c0163b1c0b01323668" +
			"6f918ac37ead03f06ee7cf0cbe0927a3598874f5cfae087567ff6fd0904609f2",
	}, {
		name:    "random key 9, blake256(0x09), rfc6979 nonce",
		key:     "6847b071a7cba6a85099b26a9c3e57a964e4990620e1e1c346fecc4472c4d834",
		msg:     "09",
		hash:    "4c2231813064f8500edae05b40195416bd543fd3e76c16d6efb10c816d92e8b6",
		nonce:   "48ea6c907e1cda596048d812439ccf416eece9a7de400c8a0e40bd48eb7e613a",
		rfc6979: true,
		expected: "81fc600775d3cdcaa14f8629537299b8226a0c8bfce9320ce64a8d14e3f95bae" +
			"faf7d1cdc07134f72fde4f0e306ed09543344a8c9834a5f134226fb2a12971e5",
	}, {
		name:    "random key 10, blake256(0x0a), rfc6979 nonce",
		key:     "b7548540f52fe20c161a0d623097f827608c56023f50442cc00cc50ad674f6b5",
		msg:     "0a",
		hash:    "e81db4f0d76e02805155441f50c861a8f86374f3ae34c7a3ff4111d3a634ecb1",
		nonce:   "95c07e315cd5457e84270ca01019563c8eeaffb18ab4f23e88a44a0ff01c5f6f",
		rfc6979: true,
		expected: "0d4cbf2da84f7448b083fce9b9c4e1834b5e2e98defcec7ec87e87c739f5fe78" +
			"032b07372963fb913d86e7c911f69aa96a7821191b15210e99b8ce33c2b205a3",
	}}

	for _, test := range tests {
		privKey := hexToBytes(test.key)
		msg := hexToBytes(test.msg)
		hash := hexToBytes(test.hash)
		nonce := hexToBytes(test.nonce)
		wantSig := hexToBytes(test.expected)

		// Ensure the test data is sane by comparing the provided hashed message
		// and nonce, in the case rfc6979 was used, to their calculated values.
		// These values could just be calculated instead of specified in the
		// test data, but it's nice to have all of the calcuated values
		// available in the test data for cross implementation testing and
		// verification.
		calcHash := chainhash.HashB(msg)
		if !bytes.Equal(calcHash, hash) {
			t.Errorf("%s: mismatched test hash -- expected: %x, given: %x",
				test.name, calcHash, hash)
			continue
		}
		if test.rfc6979 {
			calcNonce := secp256k1.NonceRFC6979(privKey, hash, nil, nil, 0)
			calcNonceBytes := calcNonce.Bytes()
			if !bytes.Equal(calcNonceBytes[:], nonce) {
				t.Errorf("%s: mismatched test nonce -- expected: %x, given: %x",
					test.name, calcNonceBytes, nonce)
				continue
			}
		}

		// Sign the hash of the message with the given private key and nonce.
		gotSig, err := schnorrSign(hash, privKey, nonce, chainhash.HashB)
		if err != nil {
			t.Errorf("%s: unexpected error when signing: %v", test.name, err)
			continue
		}

		// Ensure the generated signature is the expected value.
		gotSigBytes := gotSig.Serialize()
		if !bytes.Equal(gotSigBytes, wantSig) {
			t.Errorf("%s: unexpected signature -- got %x, want %x", test.name,
				gotSigBytes, wantSig)
			continue
		}

		// Ensure the produced signature verifies as well.
		pubKey := secp256k1.NewPrivateKey(hexToModNScalar(test.key)).PubKey()
		ok, err := schnorrVerify(gotSigBytes, pubKey, hash, chainhash.HashB)
		if err != nil {
			t.Errorf("%s: signature failed to verify with error: %v", test.name,
				err)
			continue
		}
		if !ok {
			t.Errorf("%s: signature failed to verify", test.name)
			continue
		}
	}
}

// TestSchnorrSignAndVerifyRandom ensures the Schnorr signing and verification
// work as expected for randomly-generated private keys and messages.  It also
// ensures invalid signatures are not improperly verified by mutating the valid
// signature and changing the message the signature covers.
func TestSchnorrSignAndVerifyRandom(t *testing.T) {
	// Use a unique random seed each test instance and log it if the tests fail.
	seed := time.Now().Unix()
	rng := rand.New(rand.NewSource(seed))
	defer func(t *testing.T, seed int64) {
		if t.Failed() {
			t.Logf("random seed: %d", seed)
		}
	}(t, seed)

	for i := 0; i < 100; i++ {
		// Generate a random private key.
		var buf [32]byte
		if _, err := rng.Read(buf[:]); err != nil {
			t.Fatalf("failed to read random private key: %v", err)
		}
		var privKeyScalar secp256k1.ModNScalar
		privKeyScalar.SetBytes(&buf)
		privKey := secp256k1.NewPrivateKey(&privKeyScalar)

		// Generate a random hash to sign.
		var hash [32]byte
		if _, err := rng.Read(hash[:]); err != nil {
			t.Fatalf("failed to read random hash: %v", err)
		}

		// Sign the hash with the private key and then ensure the produced
		// signature is valid for the hash and public key associated with the
		// private key.
		sigR, sigS, err := Sign(privKey, hash[:])
		if err != nil {
			t.Fatalf("failed to sign\nprivate key: %x\nhash: %x",
				privKey.Serialize(), hash)
		}
		sig := NewSignature(sigR, sigS)
		pubKey := privKey.PubKey()
		if !sig.Verify(hash[:], pubKey) {
			t.Fatalf("failed to verify signature\nsig: %x\nhash: %x\n"+
				"private key: %x\npublic key: %x", sig.Serialize(), hash,
				privKey.Serialize(), pubKey.SerializeCompressed())
		}

		// Change a random bit in the signature and ensure the bad signature
		// fails to verify the original message.
		goodSigBytes := sig.Serialize()
		badSigBytes := make([]byte, len(goodSigBytes))
		copy(badSigBytes, goodSigBytes)
		randByte := rng.Intn(len(badSigBytes))
		randBit := rng.Intn(7)
		badSigBytes[randByte] ^= 1 << randBit
		badSig, err := ParseSignature(badSigBytes)
		if err != nil {
			t.Fatalf("failed to create bad signature: %v", err)
		}
		if badSig.Verify(hash[:], pubKey) {
			t.Fatalf("verified bad signature\nsig: %x\nhash: %x\n"+
				"private key: %x\npublic key: %x", badSig.Serialize(), hash,
				privKey.Serialize(), pubKey.SerializeCompressed())
		}

		// Change a random bit in the hash that was originally signed and ensure
		// the original good signature fails to verify the new bad message.
		badHash := make([]byte, len(hash))
		copy(badHash, hash[:])
		randByte = rng.Intn(len(badHash))
		randBit = rng.Intn(7)
		badHash[randByte] ^= 1 << randBit
		if sig.Verify(badHash[:], pubKey) {
			t.Fatalf("verified signature for bad hash\nsig: %x\nhash: %x\n"+
				"pubkey: %x", sig.Serialize(), badHash,
				pubKey.SerializeCompressed())
		}
	}
}
