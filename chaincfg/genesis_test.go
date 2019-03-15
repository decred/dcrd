// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// TestSimNetGenesisBlock tests the genesis block of the simulation test network
// for validity by checking the encoded bytes and hashes.
func TestSimNetGenesisBlock(t *testing.T) {
	// Encode the genesis block to raw bytes.
	var buf bytes.Buffer
	err := SimNetParams.GenesisBlock.Serialize(&buf)
	if err != nil {
		t.Fatalf("TestSimNetGenesisBlock: %v", err)
	}

	simNetGenesisBlockBytes, _ := hex.DecodeString("0100000000000000000" +
		"000000000000000000000000000000000000000000000000000000dc101dfc" +
		"3c6a2eb10ca0c5374e10d28feb53f7eabcc850511ceadb99174aa660000000" +
		"00000000000000000000000000000000000000000000000000000000000000" +
		"000000000000000000000000000ffff7f20000000000000000000000000000" +
		"00000450686530000000000000000000000000000000000000000000000000" +
		"00000000000000000000000000000000101000000010000000000000000000" +
		"000000000000000000000000000000000000000000000ffffffff00fffffff" +
		"f0100000000000000000000434104678afdb0fe5548271967f1a67130b7105" +
		"cd6a828e03909a67962e0ea1f61deb649f6bc3f4cef38c4f35504e51ec112d" +
		"e5c384df7ba0b8d578a4c702b6bf11d5fac000000000000000001000000000" +
		"000000000000000000000004d04ffff001d0104455468652054696d6573203" +
		"0332f4a616e2f32303039204368616e63656c6c6f72206f6e206272696e6b2" +
		"06f66207365636f6e64206261696c6f757420666f722062616e6b7300")

	// Ensure the encoded block matches the expected bytes.
	if !bytes.Equal(buf.Bytes(), simNetGenesisBlockBytes) {
		t.Fatalf("TestSimNetGenesisBlock: Genesis block does not "+
			"appear valid - got %v, want %v",
			spew.Sdump(buf.Bytes()),
			spew.Sdump(simNetGenesisBlockBytes))
	}

	// Check hash of the block against expected hash.
	hash := SimNetParams.GenesisBlock.BlockHash()
	if !SimNetParams.GenesisHash.IsEqual(&hash) {
		t.Fatalf("TestSimNetGenesisBlock: Genesis block hash does "+
			"not appear valid - got %v, want %v", spew.Sdump(hash),
			spew.Sdump(SimNetParams.GenesisHash))
	}
}

// TestRegNetGenesisBlock tests the genesis block of the regression test network
// for validity by checking the encoded bytes and hashes.
func TestRegNetGenesisBlock(t *testing.T) {
	// Encode the genesis block to raw bytes.
	var buf bytes.Buffer
	err := RegNetParams.GenesisBlock.Serialize(&buf)
	if err != nil {
		t.Fatalf("TestSimNetGenesisBlock: %v", err)
	}

	regNetGenesisBlockBytes, _ := hex.DecodeString("0100000000000000000" +
		"000000000000000000000000000000000000000000000000000000dc101dfc" +
		"3c6a2eb10ca0c5374e10d28feb53f7eabcc850511ceadb99174aa660000000" +
		"00000000000000000000000000000000000000000000000000000000000000" +
		"000000000000000000000000000ffff7f20000000000000000000000000000" +
		"000008006b45b0000000000000000000000000000000000000000000000000" +
		"00000000000000000000000000000000101000000010000000000000000000" +
		"000000000000000000000000000000000000000000000ffffffff00fffffff" +
		"f010000000000000000000020801679e98561ada96caec2949a5d41c4cab38" +
		"51eb740d951c10ecbcf265c1fd9000000000000000001ffffffffffffffff0" +
		"0000000ffffffff02000000")

	// Ensure the encoded block matches the expected bytes.
	if !bytes.Equal(buf.Bytes(), regNetGenesisBlockBytes) {
		t.Fatalf("TestRegNetGenesisBlock: Genesis block does not "+
			"appear valid - got %v, want %v",
			spew.Sdump(buf.Bytes()),
			spew.Sdump(regNetGenesisBlockBytes))
	}

	// Check hash of the block against expected hash.
	hash := RegNetParams.GenesisBlock.BlockHash()
	if !RegNetParams.GenesisHash.IsEqual(&hash) {
		t.Fatalf("TestRegNetGenesisBlock: Genesis block hash does "+
			"not appear valid - got %v, want %v", spew.Sdump(hash),
			spew.Sdump(RegNetParams.GenesisHash))
	}
}
