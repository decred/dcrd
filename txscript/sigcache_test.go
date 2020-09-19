// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"crypto/rand"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec/secp256k1/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v3/ecdsa"
	"github.com/decred/dcrd/wire"
)

// msgTx113875_1 mocks the first transaction from block 113875.
func msgTx113875_1() *wire.MsgTx {
	msgTx := wire.NewMsgTx()
	txIn := wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: 0xffffffff,
			Tree:  wire.TxTreeRegular,
		},
		Sequence:        0xffffffff,
		ValueIn:         5000000000,
		BlockHeight:     0x3f3f3f3f,
		BlockIndex:      0x2e2e2e2e,
		SignatureScript: hexToBytes("0431dc001b0162"),
	}
	txOut := wire.TxOut{
		Value:   5000000000,
		Version: 0xf0f0,
		PkScript: mustParseShortForm("DATA_65 0x04d64bdfd09eb1c5fe295abdeb1dca4281b" +
			"e988e2da0b6c1c6a59dc226c28624e18175e851c96b973d81b01cc31f047834bc06d6d6e" +
			"df620d184241a6aed8b63a6 CHECKSIG"),
	}
	msgTx.AddTxIn(&txIn)
	msgTx.AddTxOut(&txOut)
	msgTx.LockTime = 0
	msgTx.Expiry = 0
	return msgTx
}

// genRandomSig returns a random message, a signature of the message under the
// public key and the public key. This function is used to generate randomized
// test data.
func genRandomSig() (*chainhash.Hash, *ecdsa.Signature, *secp256k1.PublicKey, error) {
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return nil, nil, nil, err
	}
	pub := privKey.PubKey()

	var msgHash chainhash.Hash
	if _, err := rand.Read(msgHash[:]); err != nil {
		return nil, nil, nil, err
	}

	sig := ecdsa.Sign(privKey, msgHash[:])
	return &msgHash, sig, pub, nil
}

// TestSigCacheAddExists tests the ability to add, and later check the
// existence of a signature triplet in the signature cache.
func TestSigCacheAddExists(t *testing.T) {
	sigCache := NewSigCache(200)

	// Generate a random sigCache entry triplet.
	msg1, sig1, key1, err := genRandomSig()
	if err != nil {
		t.Errorf("unable to generate random signature test data")
	}

	// Add the triplet to the signature cache.
	sigCache.Add(*msg1, sig1, key1)

	// The previously added triplet should now be found within the sigcache.
	sig1Copy, _ := ecdsa.ParseDERSignature(sig1.Serialize())
	key1Copy, _ := secp256k1.ParsePubKey(key1.SerializeCompressed())
	if !sigCache.Exists(*msg1, sig1Copy, key1Copy) {
		t.Errorf("previously added item not found in signature cache")
	}
}

// TestSigCacheAddEvictEntry tests the eviction case where a new signature
// triplet is added to a full signature cache which should trigger randomized
// eviction, followed by adding the new element to the cache.
func TestSigCacheAddEvictEntry(t *testing.T) {
	// Create a sigcache that can hold up to 100 entries.
	sigCacheSize := uint(100)
	sigCache := NewSigCache(sigCacheSize)

	// Fill the sigcache up with some random sig triplets.
	for i := uint(0); i < sigCacheSize; i++ {
		msg, sig, key, err := genRandomSig()
		if err != nil {
			t.Fatalf("unable to generate random signature test data")
		}

		sigCache.Add(*msg, sig, key)
		sigCopy, _ := ecdsa.ParseDERSignature(sig.Serialize())
		keyCopy, _ := secp256k1.ParsePubKey(key.SerializeCompressed())
		if !sigCache.Exists(*msg, sigCopy, keyCopy) {
			t.Errorf("previously added item not found in signature " +
				"cache")
		}
	}

	// The sigcache should now have sigCacheSize entries within it.
	if uint(len(sigCache.validSigs)) != sigCacheSize {
		t.Fatalf("sigcache should now have %v entries, instead it has %v",
			sigCacheSize, len(sigCache.validSigs))
	}

	// Add a new entry, this should cause eviction of a randomly chosen
	// previous entry.
	msgNew, sigNew, keyNew, err := genRandomSig()
	if err != nil {
		t.Fatalf("unable to generate random signature test data")
	}
	sigCache.Add(*msgNew, sigNew, keyNew)

	// The sigcache should still have sigCache entries.
	if uint(len(sigCache.validSigs)) != sigCacheSize {
		t.Fatalf("sigcache should now have %v entries, instead it has %v",
			sigCacheSize, len(sigCache.validSigs))
	}

	// The entry added above should be found within the sigcache.
	sigNewCopy, _ := ecdsa.ParseDERSignature(sigNew.Serialize())
	keyNewCopy, _ := secp256k1.ParsePubKey(keyNew.SerializeCompressed())
	if !sigCache.Exists(*msgNew, sigNewCopy, keyNewCopy) {
		t.Fatalf("previously added item not found in signature cache")
	}
}

// TestSigCacheAddMaxEntriesZeroOrNegative tests that if a sigCache is created
// with a max size <= 0, then no entries are added to the sigcache at all.
func TestSigCacheAddMaxEntriesZeroOrNegative(t *testing.T) {
	// Create a sigcache that can hold up to 0 entries.
	sigCache := NewSigCache(0)

	// Generate a random sigCache entry triplet.
	msg1, sig1, key1, err := genRandomSig()
	if err != nil {
		t.Errorf("unable to generate random signature test data")
	}

	// Add the triplet to the signature cache.
	sigCache.Add(*msg1, sig1, key1)

	// The generated triplet should not be found.
	sig1Copy, _ := ecdsa.ParseDERSignature(sig1.Serialize())
	key1Copy, _ := secp256k1.ParsePubKey(key1.SerializeCompressed())
	if sigCache.Exists(*msg1, sig1Copy, key1Copy) {
		t.Errorf("previously added signature found in sigcache, but " +
			"shouldn't have been")
	}

	// There shouldn't be any entries in the sigCache.
	if len(sigCache.validSigs) != 0 {
		t.Errorf("%v items found in sigcache, no items should have "+
			"been added", len(sigCache.validSigs))
	}
}

// TestShortTxHash tests the ability to generate the short hash of a transaction
// accurately.
func TestShortTxHash(t *testing.T) {
	// Create test keys.
	key1 := [shortTxHashKeySize]byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}
	key2 := [shortTxHashKeySize]byte{0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1,
		0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1}

	// Create test tx.
	msgTx := msgTx113875_1()

	// Generate a short tx hash for msgTx with key1.
	hash := shortTxHash(msgTx, key1)

	// Ensure that shortTxHash returns the same short tx hash when given the same
	// key.
	got := shortTxHash(msgTx, key1)
	if hash != got {
		t.Errorf("shortTxHash: wrong hash - got %d, want %d", got, hash)
	}

	// Ensure that shortTxHash returns a different short tx hash when given a
	// different key.
	got = shortTxHash(msgTx, key2)
	if hash == got {
		t.Errorf("shortTxHash: wanted different hash, but got same hash %d", got)
	}
}
