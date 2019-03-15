// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// RegNet -------------------------------------------------------------------------

// regNetGenesisMerkleRoot is the hash of the first transaction in the genesis
// block for the regression test network.  It is the same as the merkle root for
// the main network.
var regNetGenesisMerkleRoot = genesisMerkleRoot

// regNetGenesisBlock defines the genesis block of the block chain which serves
// as the public transaction ledger for the regression test network.
var regNetGenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version: 1,
		PrevBlock: chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}),
		MerkleRoot: regNetGenesisMerkleRoot,
		StakeRoot: chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}),
		VoteBits:     0,
		FinalState:   [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Voters:       0,
		FreshStake:   0,
		Revocations:  0,
		Timestamp:    time.Unix(1538524800, 0), // 2018-10-03 00:00:00 +0000 UTC
		PoolSize:     0,
		Bits:         0x207fffff, // 545259519 [7fffff0000000000000000000000000000000000000000000000000000000000]
		SBits:        0,
		Nonce:        0,
		StakeVersion: 0,
		Height:       0,
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTx},
}

// regNetGenesisHash is the hash of the first block in the block chain for the
// simulation test network.
var regNetGenesisHash = regNetGenesisBlock.BlockHash()
