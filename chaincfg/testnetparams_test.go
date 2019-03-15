// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// TestTestNetGenesisBlock tests the genesis block of the test network (version
// 3) for validity by checking the encoded bytes and hashes.
func TestTestNetGenesisBlock(t *testing.T) {
	testNetGenesisBlockBytes, _ := hex.DecodeString("06000000000000000000" +
		"00000000000000000000000000000000000000000000000000002c0ad603" +
		"d44a16698ac951fa22aab5e7b30293fa1d0ac72560cdfcc9eabcdfe70000" +
		"000000000000000000000000000000000000000000000000000000000000" +
		"00000000000000000000000000000000ffff001e002d3101000000000000" +
		"000000000000808f675b1aa4ae1800000000000000000000000000000000" +
		"000000000000000000000000000000000600000001010000000100000000" +
		"00000000000000000000000000000000000000000000000000000000ffff" +
		"ffff00ffffffff010000000000000000000020801679e98561ada96caec2" +
		"949a5d41c4cab3851eb740d951c10ecbcf265c1fd9000000000000000001" +
		"ffffffffffffffff00000000ffffffff02000000")

	// Encode the genesis block to raw bytes.
	params := TestNet3Params()
	var buf bytes.Buffer
	err := params.GenesisBlock.Serialize(&buf)
	if err != nil {
		t.Fatalf("TestTestNetGenesisBlock: %v", err)
	}

	// Ensure the encoded block matches the expected bytes.
	if !bytes.Equal(buf.Bytes(), testNetGenesisBlockBytes) {
		t.Fatalf("TestTestNetGenesisBlock: Genesis block does not "+
			"appear valid - got %v, want %v",
			spew.Sdump(buf.Bytes()),
			spew.Sdump(testNetGenesisBlockBytes))
	}

	// Check hash of the block against expected hash.
	hash := params.GenesisBlock.BlockHash()
	if !params.GenesisHash.IsEqual(&hash) {
		t.Fatalf("TestTestNetGenesisBlock: Genesis block hash does "+
			"not appear valid - got %v, want %v", spew.Sdump(hash),
			spew.Sdump(params.GenesisHash))
	}
}
