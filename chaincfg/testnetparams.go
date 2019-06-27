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

// TestNet3Params return the network parameters for the test currency network.
// This network is sometimes simply called "testnet".
// This is the third public iteration of testnet.
func TestNet3Params() *Params {
	// testNetPowLimit is the highest proof of work value a Decred block
	// can have for the test network.  It is the value 2^232 - 1.
	testNetPowLimit := new(big.Int).Sub(new(big.Int).Lsh(bigOne, 232), bigOne)

	// genesisBlock defines the genesis block of the block chain which serves as
	// the public transaction ledger for the test network (version 3).
	genesisBlock := wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:   6,
			PrevBlock: chainhash.Hash{},
			// MerkleRoot: Calculated below.
			Timestamp:    time.Unix(1533513600, 0), // 2018-08-06 00:00:00 +0000 UTC
			Bits:         0x1e00ffff,               // Difficulty 1 [000000ffff000000000000000000000000000000000000000000000000000000]
			SBits:        20000000,
			Nonce:        0x18aea41a,
			StakeVersion: 6,
		},
		Transactions: []*wire.MsgTx{{
			SerType: wire.TxSerializeFull,
			Version: 1,
			TxIn: []*wire.TxIn{{
				// Fully null.
				PreviousOutPoint: wire.OutPoint{
					Hash:  chainhash.Hash{},
					Index: 0xffffffff,
					Tree:  0,
				},
				SignatureScript: hexDecode("0000"),
				Sequence:        0xffffffff,
				BlockHeight:     wire.NullBlockHeight,
				BlockIndex:      wire.NullBlockIndex,
				ValueIn:         wire.NullValueIn,
			}},
			TxOut: []*wire.TxOut{{
				Version: 0x0000,
				Value:   0x00000000,
				PkScript: hexDecode("801679e98561ada96caec2949a5d41c4cab3851e" +
					"b740d951c10ecbcf265c1fd9"),
			}},
			LockTime: 0,
			Expiry:   0,
		}},
	}
	// NOTE: This really should be TxHashFull, but it was defined incorrectly.
	//
	// Since the field is not used in any validation code, it does not have any
	// adverse effects, but correcting it would result in changing the block
	// hash which would invalidate the entire test network.  The next test
	// network should set the value properly.
	genesisBlock.Header.MerkleRoot = genesisBlock.Transactions[0].TxHash()

	return &Params{
		Name:        "testnet3",
		Net:         wire.TestNet3,
		DefaultPort: "19108",
		DNSSeeds: []DNSSeed{
			{"testnet-seed.decred.mindcry.org", true},
			{"testnet-seed.decred.netpurgatory.com", true},
			{"testnet-seed.decred.org", true},
		},

		// Chain parameters
		GenesisBlock:             &genesisBlock,
		GenesisHash:              genesisBlock.BlockHash(),
		PowLimit:                 testNetPowLimit,
		PowLimitBits:             0x1e00ffff,
		ReduceMinDifficulty:      true,
		MinDiffReductionTime:     time.Minute * 10, // ~99.3% chance to be mined before reduction
		GenerateSupported:        true,
		MaximumBlockSizes:        []int{1310720},
		MaxTxSize:                1000000,
		TargetTimePerBlock:       time.Minute * 2,
		WorkDiffAlpha:            1,
		WorkDiffWindowSize:       144,
		WorkDiffWindows:          20,
		TargetTimespan:           time.Minute * 2 * 144, // TimePerBlock * WindowSize
		RetargetAdjustmentFactor: 4,

		// Subsidy parameters.
		BaseSubsidy:              2500000000, // 25 Coin
		MulSubsidy:               100,
		DivSubsidy:               101,
		SubsidyReductionInterval: 2048,
		WorkRewardProportion:     6,
		StakeRewardProportion:    3,
		BlockTaxProportion:       1,

		// Checkpoints ordered from oldest to newest.
		Checkpoints: []Checkpoint{
			{83520, newHashFromStr("0000000001e6244d95feae8b598e854905158c7bc781daf874afff88675ef0c8")},
		},

		// Consensus rule change deployments.
		//
		// The miner confirmation window is defined as:
		//   target proof of work timespan / target proof of work spacing
		RuleChangeActivationQuorum:     2520, // 10 % of RuleChangeActivationInterval * TicketsPerBlock
		RuleChangeActivationMultiplier: 3,    // 75%
		RuleChangeActivationDivisor:    4,
		RuleChangeActivationInterval:   5040, // 1 week
		Deployments: map[uint32][]ConsensusDeployment{
			7: {{
				Vote: Vote{
					Id:          VoteIDFixLNSeqLocks,
					Description: "Modify sequence lock handling as defined in DCP0004",
					Mask:        0x0006, // Bits 1 and 2
					Choices: []Choice{{
						Id:          "abstain",
						Description: "abstain voting for change",
						Bits:        0x0000,
						IsAbstain:   true,
						IsNo:        false,
					}, {
						Id:          "no",
						Description: "keep the existing consensus rules",
						Bits:        0x0002, // Bit 1
						IsAbstain:   false,
						IsNo:        true,
					}, {
						Id:          "yes",
						Description: "change to the new consensus rules",
						Bits:        0x0004, // Bit 2
						IsAbstain:   false,
						IsNo:        false,
					}},
				},
				StartTime:  1548633600, // Jan 28th, 2019
				ExpireTime: 1580169600, // Jan 28th, 2020
			}},
		},

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
		NetworkAddressPrefix: "T",
		PubKeyAddrID:         [2]byte{0x28, 0xf7}, // starts with Tk
		PubKeyHashAddrID:     [2]byte{0x0f, 0x21}, // starts with Ts
		PKHEdwardsAddrID:     [2]byte{0x0f, 0x01}, // starts with Te
		PKHSchnorrAddrID:     [2]byte{0x0e, 0xe3}, // starts with TS
		ScriptHashAddrID:     [2]byte{0x0e, 0xfc}, // starts with Tc
		PrivateKeyID:         [2]byte{0x23, 0x0e}, // starts with Pt

		// BIP32 hierarchical deterministic extended key magics
		HDPrivateKeyID: [4]byte{0x04, 0x35, 0x83, 0x97}, // starts with tprv
		HDPublicKeyID:  [4]byte{0x04, 0x35, 0x87, 0xd1}, // starts with tpub

		// BIP44 coin type used in the hierarchical deterministic path for
		// address generation.
		SLIP0044CoinType: 1,  // SLIP0044, Testnet (all coins)
		LegacyCoinType:   11, // for backwards compatibility

		// Decred PoS parameters
		MinimumStakeDiff:        20000000, // 0.2 Coin
		TicketPoolSize:          1024,
		TicketsPerBlock:         5,
		TicketMaturity:          16,
		TicketExpiry:            6144, // 6*TicketPoolSize
		CoinbaseMaturity:        16,
		SStxChangeMaturity:      1,
		TicketPoolSizeWeight:    4,
		StakeDiffAlpha:          1,
		StakeDiffWindowSize:     144,
		StakeDiffWindows:        20,
		StakeVersionInterval:    144 * 2 * 7, // ~1 week
		MaxFreshStakePerBlock:   20,          // 4*TicketsPerBlock
		StakeEnabledHeight:      16 + 16,     // CoinbaseMaturity + TicketMaturity
		StakeValidationHeight:   768,         // Arbitrary
		StakeBaseSigScript:      []byte{0x00, 0x00},
		StakeMajorityMultiplier: 3,
		StakeMajorityDivisor:    4,

		// Decred organization related parameters.
		// Organization address is TcrypGAcGCRVXrES7hWqVZb5oLJKCZEtoL1.
		OrganizationPkScript:        hexDecode("a914d585cd7426d25b4ea5faf1e6987aacfeda3db94287"),
		OrganizationPkScriptVersion: 0,
		BlockOneLedger: []TokenPayout{
			mustPayout("76a914bb56576997d9f7abeebac585821b4d9a79d7ea0a88ac", 80000*1e8),
			mustPayout("76a9147a5c4cca76f2e0b36db4763daacbd6cbb6ee6e7b88ac", 20000*1e8),
		},
	}
}
