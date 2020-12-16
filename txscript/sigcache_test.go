// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"compress/bzip2"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/decred/dcrd/wire"
)

// testDataPath is the path where txscript test fixtures reside.
const testDataPath = "data"

// block432100 mocks block 432,100 of the block chain.  It is loaded and
// deserialized immediately here and then can be used throughout the tests.
var block432100 = func() wire.MsgBlock {
	// Load and deserialize the test block.
	blockDataFile := filepath.Join(testDataPath, "block432100.bz2")
	fi, err := os.Open(blockDataFile)
	if err != nil {
		panic(err)
	}
	defer fi.Close()
	var block wire.MsgBlock
	err = block.Deserialize(bzip2.NewReader(fi))
	if err != nil {
		panic(err)
	}
	return block
}()

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
func genRandomSig(t *testing.T) (*chainhash.Hash, *ecdsa.Signature, *secp256k1.PublicKey) {
	t.Helper()

	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("error generating private key: %v", err)
	}
	pub := privKey.PubKey()

	var msgHash chainhash.Hash
	if _, err := rand.Read(msgHash[:]); err != nil {
		t.Fatalf("error reading random hash: %v", err)
	}

	sig := ecdsa.Sign(privKey, msgHash[:])
	return &msgHash, sig, pub
}

// TestSigCacheAddExists tests the ability to add, and later check the
// existence of a signature triplet in the signature cache.
func TestSigCacheAddExists(t *testing.T) {
	sigCache, err := NewSigCache(200)
	if err != nil {
		t.Fatalf("error creating NewSigCache: %v", err)
	}

	// Generate a random sigCache entry triplet.
	msg1, sig1, key1 := genRandomSig(t)

	// Add the triplet to the signature cache.
	sigCache.Add(*msg1, sig1, key1, msgTx113875_1())

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
	sigCache, err := NewSigCache(sigCacheSize)
	if err != nil {
		t.Fatalf("error creating NewSigCache: %v", err)
	}

	// Create test tx.
	tx := msgTx113875_1()

	// Fill the sigcache up with some random sig triplets.
	for i := uint(0); i < sigCacheSize; i++ {
		msg, sig, key := genRandomSig(t)

		sigCache.Add(*msg, sig, key, tx)
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
	msgNew, sigNew, keyNew := genRandomSig(t)
	sigCache.Add(*msgNew, sigNew, keyNew, tx)

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
	sigCache, err := NewSigCache(0)
	if err != nil {
		t.Fatalf("error creating NewSigCache: %v", err)
	}

	// Generate a random sigCache entry triplet.
	msg1, sig1, key1 := genRandomSig(t)

	// Add the triplet to the signature cache.
	sigCache.Add(*msg1, sig1, key1, msgTx113875_1())

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

// TestEvictEntries tests that evictEntries properly removes all SigCache
// entries related to the given block.
func TestEvictEntries(t *testing.T) {
	// Create a SigCache instance.
	numTxns := len(block432100.Transactions) + len(block432100.STransactions)
	sigCache, err := NewSigCache(uint(numTxns + 1))
	if err != nil {
		t.Fatalf("error creating NewSigCache: %v", err)
	}

	// Add random signatures to the SigCache for each transaction in block432100.
	for _, tx := range block432100.Transactions {
		msg, sig, key := genRandomSig(t)
		sigCache.Add(*msg, sig, key, tx)
	}
	for _, stx := range block432100.STransactions {
		msg, sig, key := genRandomSig(t)
		sigCache.Add(*msg, sig, key, stx)
	}

	// Add another random signature that is not related to a transaction in
	// block432100.
	msg, sig, key := genRandomSig(t)
	sigCache.Add(*msg, sig, key, msgTx113875_1())

	// Validate the number of entries that should exist in the SigCache before
	// eviction.
	wantLength := numTxns + 1
	gotLength := len(sigCache.validSigs)
	if gotLength != wantLength {
		t.Fatalf("Incorrect number of entries before eviction: "+
			"gotLength: %d, wantLength: %d", gotLength, wantLength)
	}

	// Evict entries for block432100.
	sigCache.evictEntries(&block432100)

	// Validate that entries related to block432100 have been removed and that
	// entries unrelated to block432100 have not been removed.
	wantLength = 1
	gotLength = len(sigCache.validSigs)
	if gotLength != wantLength {
		t.Errorf("Incorrect number of entries after eviction: "+
			"gotLength: %d, wantLength: %d", gotLength, wantLength)
	}
	sigCopy, _ := ecdsa.ParseDERSignature(sig.Serialize())
	keyCopy, _ := secp256k1.ParsePubKey(key.SerializeCompressed())
	if !sigCache.Exists(*msg, sigCopy, keyCopy) {
		t.Errorf("previously added item not found in signature cache")
	}
}
