// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"bytes"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// TestSimNetGenesisBlock tests the genesis block of the simulation test network
// for validity by checking the encoded bytes and hashes.
func TestSimNetGenesisBlock(t *testing.T) {
	simNetGenesisBlockBytes := hexDecode("010000000000000000000000000000000" +
		"000000000000000000000000000000000000000925629c5582bbfc3609d71a2f4a" +
		"887443c80d54a1fe31e95e95d42f3e288945c00000000000000000000000000000" +
		"000000000000000000000000000000000000000000000000000000000000000000" +
		"0ffff7f20000000000000000000000000000000004506865300000000000000000" +
		"000000000000000000000000000000000000000000000000000000000000000010" +
		"100000001000000000000000000000000000000000000000000000000000000000" +
		"0000000ffffffff00ffffffff0100000000000000000000434104678afdb0fe554" +
		"8271967f1a67130b7105cd6a828e03909a67962e0ea1f61deb649f6bc3f4cef38c" +
		"4f35504e51ec112de5c384df7ba0b8d578a4c702b6bf11d5fac000000000000000" +
		"001000000000000000000000000000000004d04ffff001d0104455468652054696" +
		"d65732030332f4a616e2f32303039204368616e63656c6c6f72206f6e206272696" +
		"e6b206f66207365636f6e64206261696c6f757420666f722062616e6b7300")

	// Encode the genesis block to raw bytes.
	params := SimNetParams()
	var buf bytes.Buffer
	err := params.GenesisBlock.Serialize(&buf)
	if err != nil {
		t.Fatalf("TestSimNetGenesisBlock: %v", err)
	}

	// Ensure the encoded block matches the expected bytes.
	if !bytes.Equal(buf.Bytes(), simNetGenesisBlockBytes) {
		t.Fatalf("TestSimNetGenesisBlock: Genesis block does not "+
			"appear valid - got %v, want %v",
			spew.Sdump(buf.Bytes()),
			spew.Sdump(simNetGenesisBlockBytes))
	}

	// Check hash of the block against expected hash.
	hash := params.GenesisBlock.BlockHash()
	if !params.GenesisHash.IsEqual(&hash) {
		t.Fatalf("TestSimNetGenesisBlock: Genesis block hash does "+
			"not appear valid - got %v, want %v", spew.Sdump(hash),
			spew.Sdump(params.GenesisHash))
	}
}
