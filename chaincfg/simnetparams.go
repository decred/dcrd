// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"math/big"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// SimNetParams returns the network parameters for the simulation test network.
// This network is similar to the normal test network except it is intended for
// private use within a group of individuals doing simulation testing and full
// integration tests between different applications such as wallets, voting
// service providers, mining pools, block explorers, and other services that
// build on Decred.
//
// The functionality is intended to differ in that the only nodes which are
// specifically specified are used to create the network rather than following
// normal discovery rules.  This is important as otherwise it would just turn
// into another public testnet.
func SimNetParams() *Params {
	// simNetPowLimit is the highest proof of work value a Decred block can have
	// for the simulation test network.  It is the value 2^255 - 1.
	simNetPowLimit := new(big.Int).Sub(new(big.Int).Lsh(bigOne, 255), bigOne)

	// genesisBlock defines the genesis block of the block chain which serves
	// as the public transaction ledger for the simulation test network.
	genesisBlock := wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:   1,
			PrevBlock: chainhash.Hash{}, // All zero.
			// MerkleRoot: Calculated below.
			StakeRoot:    chainhash.Hash{}, // All zero.
			VoteBits:     0,
			FinalState:   [6]byte{}, // All zero.
			Voters:       0,
			FreshStake:   0,
			Revocations:  0,
			Timestamp:    time.Unix(1401292357, 0), // 2009-01-08 20:54:25 -0600 CST
			PoolSize:     0,
			Bits:         0x207fffff, // 545259519 [7fffff0000000000000000000000000000000000000000000000000000000000]
			SBits:        0,
			Nonce:        0,
			StakeVersion: 0,
			Height:       0,
		},
		Transactions: []*wire.MsgTx{&wire.MsgTx{
			SerType: wire.TxSerializeFull,
			Version: 1,
			TxIn: []*wire.TxIn{{
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 0xffffffff,
				},
				SignatureScript: hexDecode("04ffff001d0104455468652054696d65" +
					"732030332f4a616e2f32303039204368616e63656c6c6f72206f6e" +
					"206272696e6b206f66207365636f6e64206261696c6f757420666f" +
					"722062616e6b73"),
				Sequence: 0xffffffff,
			}},
			TxOut: []*wire.TxOut{{
				Value: 0x00000000,
				PkScript: hexDecode("4104678afdb0fe5548271967f1a67130b7105c" +
					"d6a828e03909a67962e0ea1f61deb649f6bc3f4cef38c4f35504e5" +
					"1ec112de5c384df7ba0b8d578a4c702b6bf11d5fac"),
			}},
			LockTime: 0,
			Expiry:   0,
		}},
	}
	genesisBlock.Header.MerkleRoot = genesisBlock.Transactions[0].TxHashFull()

	return &Params{
		Name:        "simnet",
		Net:         wire.SimNet,
		DefaultPort: "18555",
		DNSSeeds:    nil, // NOTE: There must NOT be any seeds.

		// Chain parameters
		GenesisBlock:             &genesisBlock,
		GenesisHash:              genesisBlock.BlockHash(),
		PowLimit:                 simNetPowLimit,
		PowLimitBits:             0x207fffff,
		ReduceMinDifficulty:      false,
		MinDiffReductionTime:     0, // Does not apply since ReduceMinDifficulty false
		GenerateSupported:        true,
		MaximumBlockSizes:        []int{1310720},
		MaxTxSize:                1000000,
		TargetTimePerBlock:       time.Second,
		WorkDiffAlpha:            1,
		WorkDiffWindowSize:       8,
		WorkDiffWindows:          4,
		TargetTimespan:           time.Second * 8, // TimePerBlock * WindowSize
		RetargetAdjustmentFactor: 4,

		// Subsidy parameters.
		BaseSubsidy:              50000000000,
		MulSubsidy:               100,
		DivSubsidy:               101,
		SubsidyReductionInterval: 128,
		WorkRewardProportion:     6,
		StakeRewardProportion:    3,
		BlockTaxProportion:       1,

		// Checkpoints ordered from oldest to newest.
		Checkpoints: nil,

		// Consensus rule change deployments.
		//
		// The miner confirmation window is defined as:
		//   target proof of work timespan / target proof of work spacing
		RuleChangeActivationQuorum:     160, // 10 % of RuleChangeActivationInterval * TicketsPerBlock
		RuleChangeActivationMultiplier: 3,   // 75%
		RuleChangeActivationDivisor:    4,
		RuleChangeActivationInterval:   320, // 320 seconds

		// Enforce current block version once majority of the network has
		// upgraded.
		// 51% (51 / 100)
		// Reject previous block versions once a majority of the network has
		// upgraded.
		// 75% (75 / 100)
		BlockEnforceNumRequired: 51,
		BlockRejectNumRequired:  75,
		BlockUpgradeNumToCheck:  100,

		// AcceptNonStdTxs is a mempool param to either accept and relay non
		// standard txs to the network or reject them
		AcceptNonStdTxs: true,

		// Address encoding magics
		NetworkAddressPrefix: "S",
		PubKeyAddrID:         [2]byte{0x27, 0x6f}, // starts with Sk
		PubKeyHashAddrID:     [2]byte{0x0e, 0x91}, // starts with Ss
		PKHEdwardsAddrID:     [2]byte{0x0e, 0x71}, // starts with Se
		PKHSchnorrAddrID:     [2]byte{0x0e, 0x53}, // starts with SS
		ScriptHashAddrID:     [2]byte{0x0e, 0x6c}, // starts with Sc
		PrivateKeyID:         [2]byte{0x23, 0x07}, // starts with Ps

		// BIP32 hierarchical deterministic extended key magics
		HDPrivateKeyID: [4]byte{0x04, 0x20, 0xb9, 0x03}, // starts with sprv
		HDPublicKeyID:  [4]byte{0x04, 0x20, 0xbd, 0x3d}, // starts with spub

		// BIP44 coin type used in the hierarchical deterministic path for
		// address generation.
		SLIP0044CoinType: 1,   // SLIP0044, Testnet (all coins)
		LegacyCoinType:   115, // ASCII for s, for backwards compatibility

		// Decred PoS parameters
		MinimumStakeDiff:        20000,
		TicketPoolSize:          64,
		TicketsPerBlock:         5,
		TicketMaturity:          16,
		TicketExpiry:            384, // 6*TicketPoolSize
		CoinbaseMaturity:        16,
		SStxChangeMaturity:      1,
		TicketPoolSizeWeight:    4,
		StakeDiffAlpha:          1,
		StakeDiffWindowSize:     8,
		StakeDiffWindows:        8,
		StakeVersionInterval:    8 * 2 * 7,
		MaxFreshStakePerBlock:   20,            // 4*TicketsPerBlock
		StakeEnabledHeight:      16 + 16,       // CoinbaseMaturity + TicketMaturity
		StakeValidationHeight:   16 + (64 * 2), // CoinbaseMaturity + TicketPoolSize*2
		StakeBaseSigScript:      []byte{0xDE, 0xAD, 0xBE, 0xEF},
		StakeMajorityMultiplier: 3,
		StakeMajorityDivisor:    4,

		// Decred organization related parameters
		//
		// Treasury address is a 3-of-3 P2SH going to a wallet with seed:
		// aardvark adroitness aardvark adroitness
		// aardvark adroitness aardvark adroitness
		// aardvark adroitness aardvark adroitness
		// aardvark adroitness aardvark adroitness
		// aardvark adroitness aardvark adroitness
		// aardvark adroitness aardvark adroitness
		// aardvark adroitness aardvark adroitness
		// aardvark adroitness aardvark adroitness
		// briefcase
		// (seed 0x0000000000000000000000000000000000000000000000000000000000000000)
		//
		// This same wallet owns the three ledger outputs for simnet.
		//
		// P2SH details for simnet treasury:
		//
		// redeemScript: 532103e8c60c7336744c8dcc7b85c27789950fc52aa4e48f895ebbfb
		// ac383ab893fc4c2103ff9afc246e0921e37d12e17d8296ca06a8f92a07fbe7857ed1d4
		// f0f5d94e988f21033ed09c7fa8b83ed53e6f2c57c5fa99ed2230c0d38edf53c0340d0f
		// c2e79c725a53ae
		//   (3-of-3 multisig)
		// Pubkeys used:
		//   SkQmxbeuEFDByPoTj41TtXat8tWySVuYUQpd4fuNNyUx51tF1csSs
		//   SkQn8ervNvAUEX5Ua3Lwjc6BAuTXRznDoDzsyxgjYqX58znY7w9e4
		//   SkQkfkHZeBbMW8129tZ3KspEh1XBFC1btbkgzs6cjSyPbrgxzsKqk
		//
		// Organization address is ScuQxvveKGfpG1ypt6u27F99Anf7EW3cqhq
		OrganizationPkScript:        hexDecode("a914cbb08d6ca783b533b2c7d24a51fbca92d937bf9987"),
		OrganizationPkScriptVersion: 0,
		BlockOneLedger: []TokenPayout{
			mustPayout("76a91494ff37a0ee4d48abc45f70474f9b86f9da69a70988ac", 100000*1e8),
			mustPayout("76a914a6753ebbc08e2553e7dd6d64bdead4bcbff4fcf188ac", 100000*1e8),
			mustPayout("76a9147aa3211c2ead810bbf5911c275c69cc196202bd888ac", 100000*1e8),
		},
	}
}
