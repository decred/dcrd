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

// TestRegNetGenesisBlock tests the genesis block of the regression test network
// for validity by checking the encoded bytes and hashes.
func TestRegNetGenesisBlock(t *testing.T) {
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

	// Encode the genesis block to raw bytes.
	params := RegNetParams()
	var buf bytes.Buffer
	err := params.GenesisBlock.Serialize(&buf)
	if err != nil {
		t.Fatalf("TestSimNetGenesisBlock: %v", err)
	}

	// Ensure the encoded block matches the expected bytes.
	if !bytes.Equal(buf.Bytes(), regNetGenesisBlockBytes) {
		t.Fatalf("TestRegNetGenesisBlock: Genesis block does not "+
			"appear valid - got %v, want %v",
			spew.Sdump(buf.Bytes()),
			spew.Sdump(regNetGenesisBlockBytes))
	}

	// Check hash of the block against expected hash.
	hash := params.GenesisBlock.BlockHash()
	if !params.GenesisHash.IsEqual(&hash) {
		t.Fatalf("TestRegNetGenesisBlock: Genesis block hash does "+
			"not appear valid - got %v, want %v", spew.Sdump(hash),
			spew.Sdump(params.GenesisHash))
	}
}
